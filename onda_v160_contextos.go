package main

// onda_v160_contextos.go — v1.6.0 Fase 1 (fundação da onda "contextos").
//
// O comando pediu que as cadeiras Encarregado de Pessoal/Material virem
// CONTEXTO PRÓPRIO no dropdown. Doutrina espelhada na D1 do chefe (R-12):
// `funcao_membros` é a DESIGNAÇÃO; a linha em `usuario_papeis` é o ACESSO —
// o dropdown de contexto só lê usuario_papeis (PapeisDoUsuario, store.go), e
// UsuarioDaSessao sobrepõe o papel da sessão pela linha apontada por
// sessoes.papel_ativo_id. Hoje a cadeira nunca vira contexto: designado puro
// loga com papel_ativo_id NULL (CriarSessaoComPapel) e nenhum papel enc existe
// na tabela (CHECK recusa).
//
// migrarV45 fecha isso em 3 passos (template: migrarV24, store.go — o rebuild
// de usuario_papeis já nasceu lá):
//   1. REBUILD de usuario_papeis com o CHECK estendido
//      ('admin','gerente','operador','chefe_setor','enc_pessoal','enc_material')
//      preservando TODAS as colunas e ids — as FKs que apontam para a tabela
//      (mensagens.remetente_papel_id, mensagem_destinatarios,
//      avisos.autor_papel_id, aviso_cientes/aviso_comentarios.papel_id,
//      calendario_compartilhamentos.alvo_papel_id) continuam resolvendo por
//      nome de tabela + ids preservados; `sem_funcao` fica FORA do CHECK por
//      design (ausência de contexto não é papel — Fase 5).
//      Guard DUPLA: marca em schema_migrations (idempotência) + DDL em
//      sqlite_master (só reconstrói se o CHECK ainda não conhece
//      'enc_material' — banco já migrado por re-execução não refaz).
//      SEM transação: PRAGMA foreign_keys é no-op dentro de tx (v24 provou o
//      padrão); pool = 1 conexão, statements isolados, nenhum rows aberto.
//   2. MATERIALIZAÇÃO: toda designação em cadeira enc (funcao_membros ×
//      funcoes tipo='grupo') vira linha de papel. INSERT OR IGNORE — a
//      UNIQUE(usuario_id,grupo_id,papel) deduplica e titular/auxiliar da
//      MESMA cadeira não colidem (papel distingue). funcao_id guarda a
//      cadeira: é o rótulo do dropdown (FuncaoNome).
//   3. RE-KEY de sessões: sessão de designado puro (papel_ativo_id NULL)
//      passa a apontar para a PRIMEIRA linha enc da conta (subquery escalar
//      ORDER BY id — mesma convenção do CriarSessaoComPapel). Fases seguintes
//      fazem o sync nas designações/remoções via handler; aqui é o one-shot.

import (
	"fmt"
	"strings"
)

func (s *Store) migrarV45() error {
	var v int
	_ = s.db.QueryRow(`SELECT versao FROM schema_migrations WHERE versao = 45`).Scan(&v)
	if v == 45 {
		return nil
	}

	// defesa sqlite_master (estilo v44): banco velho pode não ter o domínio
	var temTabela func(string) bool
	temTabela = func(nome string) bool {
		var n int
		_ = s.db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name = ?`, nome).Scan(&n)
		return n > 0
	}
	if !temTabela("usuario_papeis") || !temTabela("funcoes") || !temTabela("funcao_membros") || !temTabela("usuarios") || !temTabela("sessoes") {
		return s.marcarVersao(45)
	}

	// GUARD DE REBUILD: reconstrói só se o DDL atual tem CHECK que ainda não
	// conhece 'enc_material'.
	var ddl string
	if err := s.db.QueryRow(`SELECT COALESCE(sql,'') FROM sqlite_master WHERE type='table' AND name='usuario_papeis'`).Scan(&ddl); err != nil {
		return err
	}
	if strings.Contains(ddl, "CHECK") && !strings.Contains(ddl, "enc_material") {
		if _, err := s.db.Exec(`PRAGMA foreign_keys=OFF`); err != nil {
			return err
		}
		steps := []string{
			`CREATE TABLE usuario_papeis_v45 (
				id INTEGER PRIMARY KEY,
				usuario_id INTEGER NOT NULL REFERENCES usuarios(id) ON DELETE CASCADE,
				grupo_id INTEGER REFERENCES grupos(id),
				papel TEXT NOT NULL CHECK (papel IN ('admin', 'gerente', 'operador', 'chefe_setor', 'enc_pessoal', 'enc_material')),
				funcao_id INTEGER REFERENCES funcoes(id),
				nome_exibicao TEXT,
				criado_em TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
				UNIQUE (usuario_id, grupo_id, papel)
			)`,
			`INSERT INTO usuario_papeis_v45 (id, usuario_id, grupo_id, papel, funcao_id, nome_exibicao, criado_em)
				SELECT id, usuario_id, grupo_id, papel, funcao_id, nome_exibicao, criado_em FROM usuario_papeis`,
			`DROP TABLE usuario_papeis`,
			`ALTER TABLE usuario_papeis_v45 RENAME TO usuario_papeis`,
			`CREATE UNIQUE INDEX IF NOT EXISTS idx_usuario_papeis_unico_gerente ON usuario_papeis(grupo_id) WHERE papel = 'gerente'`,
		}
		for _, q := range steps {
			if _, err := s.db.Exec(q); err != nil {
				return fmt.Errorf("migração v45 rebuild usuario_papeis: %w", err)
			}
		}
		if _, err := s.db.Exec(`PRAGMA foreign_keys=ON`); err != nil {
			return err
		}
	}

	// 2) materialização das cadeiras (titular E auxiliar; OR IGNORE idempotente)
	if _, err := s.db.Exec(`
		INSERT OR IGNORE INTO usuario_papeis (usuario_id, grupo_id, papel, funcao_id)
		SELECT fm.usuario_id, fm.grupo_id, f.chave, f.id
		FROM funcao_membros fm
		JOIN funcoes f ON f.id = fm.funcao_id
		WHERE f.tipo = 'grupo' AND f.chave IN ('enc_pessoal', 'enc_material')
	`); err != nil {
		return fmt.Errorf("migração v45 materializar cadeiras: %w", err)
	}

	// 3) re-key: sessão aberta de designado puro assume a primeira linha enc
	if _, err := s.db.Exec(`
		UPDATE sessoes SET papel_ativo_id = (
			SELECT up.id FROM usuario_papeis up
			WHERE up.usuario_id = sessoes.usuario_id
			  AND up.papel IN ('enc_pessoal', 'enc_material')
			ORDER BY up.id ASC LIMIT 1
		)
		WHERE papel_ativo_id IS NULL
		  AND EXISTS (
			SELECT 1 FROM usuario_papeis up2
			WHERE up2.usuario_id = sessoes.usuario_id
			  AND up2.papel IN ('enc_pessoal', 'enc_material')
		  )
	`); err != nil {
		return fmt.Errorf("migração v45 re-key sessões: %w", err)
	}

	return s.marcarVersao(45)
}
