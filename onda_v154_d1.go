package main

// onda_v154_d1.go — v1.5.4-D1 (defeito R-12, "chefe-zumbi"): UMA fonte de
// verdade para o comando de setor.
//
// O bug relatado pelo comando ("2 chefes de setor simultâneos"): A é nomeado
// chefe de S (linha em chefe_setores + usuarios.setor_id = S, efeito colateral
// de hGrupoNomearChefe); B é nomeado no lugar (chefe_setores: B↔S), mas o
// usuarios.setor_id de A NUNCA é limpo (hGrupoDestituirChefe não o mexe) — e
// os guardas de concluir/reabrir/marcar confiam no CONTEXTO (u.SetorID →
// fallback pessoas.setor_id) sem conferir o COMANDO. A segue exercendo poderes
// invisíveis de S. Fabricas adicionais do zumbi: hUsuarioPapelAdd gravava o
// papel chefe_setor + usuarios.setor_id SEM linha em chefe_setores (o comando
// só existia via fallback).
//
// Correção (decisão D-2: MATERIALIZAR legados e remover os fallbacks):
//   1. migrarV44 — one-shot que materializa em chefe_setores os comandos
//      legados ainda pendentes (fontes usuarios.setor_id → pessoas.setor_id;
//      OR IGNORE — UNIQUE(setor_id) preserva o comando vigente);
//   2. chefeComandaSetor passa a consultar SOMENTE chefe_setores;
//   3. novo setorAtivoComandado — o contexto da sessão só vale se o chefe
//      AINDA comanda o setor (mata o poder via contexto órfão);
//   4. hUsuarioPapelAdd (chefe_setor + setor) materializa o comando;
//      hUsuarioPapelDel (chefe_setor) apaga os comandos do grupo.
// usuarios.setor_id deixa de ser fonte de autorização (passa a ser só o
// contexto/cadastro exibido — decisão D-5 proposta no ARQUITETURA §Chefias).

import (
	"fmt"
)

// migrarV44 (v1.5.4-D1): materializa comandos legados em chefe_setores.
// Idempotente (marca versão + INSERT OR IGNORE). Nunca apaga comando vigente:
// a UNIQUE(setor_id) deduplica, preservando quem já está na tabela.
func (s *Store) migrarV44() error {
	var v int
	_ = s.db.QueryRow(`SELECT versao FROM schema_migrations WHERE versao = 44`).Scan(&v)
	if v == 44 {
		return nil
	}

	// defesa sqlite_master: banco velho pode não ter as tabelas do domínio
	var temTabela func(string) bool
	temTabela = func(nome string) bool {
		var n int
		_ = s.db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name = ?`, nome).Scan(&n)
		return n > 0
	}
	if !temTabela("chefe_setores") || !temTabela("usuario_papeis") || !temTabela("usuarios") {
		return s.marcarVersao(44)
	}

	// 1) legado fonte A: conta com papel chefe_setor, SEM comando materializado,
	//    cujo usuarios.setor_id é um setor ATIVO DO PRÓPRIO grupo ainda VAGO.
	if _, err := s.db.Exec(`
		INSERT OR IGNORE INTO chefe_setores (grupo_id, setor_id, usuario_id)
		SELECT u.grupo_id, u.setor_id, u.id
		FROM usuarios u
		JOIN usuario_papeis up ON up.usuario_id = u.id
		       AND up.grupo_id IS u.grupo_id AND up.papel = 'chefe_setor'
		WHERE u.ativo = 1 AND u.grupo_id IS NOT NULL AND u.setor_id IS NOT NULL
		  AND u.setor_id IN (SELECT id FROM setores WHERE ativo = 1 AND grupo_id IS u.grupo_id)
	`); err != nil {
		return fmt.Errorf("migração v44 materializar (usuarios.setor_id): %w", err)
	}

	// 2) legado fonte B: mesmo critério, setor vindo da pessoa vinculada
	//    (contas criadas com setor só no cadastro de pessoal).
	if temTabela("pessoas") {
		if _, err := s.db.Exec(`
			INSERT OR IGNORE INTO chefe_setores (grupo_id, setor_id, usuario_id)
			SELECT u.grupo_id, p.setor_id, u.id
			FROM usuarios u
			JOIN pessoas p ON p.id = u.pessoa_id
			JOIN usuario_papeis up ON up.usuario_id = u.id
			       AND up.grupo_id IS u.grupo_id AND up.papel = 'chefe_setor'
			WHERE u.ativo = 1 AND u.grupo_id IS NOT NULL AND p.setor_id IS NOT NULL
			  AND p.setor_id IN (SELECT id FROM setores WHERE ativo = 1 AND grupo_id IS u.grupo_id)
		`); err != nil {
			return fmt.Errorf("migração v44 materializar (pessoas.setor_id): %w", err)
		}
	}

	// NOTA (R-12): SEM purge global de papel chefe_setor sem comando — o papel
	// é acesso de sessão e a purga já acontece por grupo a cada nomear/
	// destituir/remover-papel. Sobra cosmética (menu visível, ação 403) não
	// concede poder: toda autorização consulta chefe_setores.
	return s.marcarVersao(44)
}

// setorAtivoComandado: o setor ATIVO no contexto da sessão, DESDE QUE o chefe
// ainda o comande (chefe_setores). Guarda do chefe-zumbi: contexto órfão —
// usuarios.setor_id que ficou de uma chefia anterior — não autoriza NADA
// (concluir, reabrir, marcar, leitura do setor). Somente papel chefe_setor;
// operador tem doutrina própria (setor de cadastro).
func (a *App) setorAtivoComandado(u *Usuario) *int64 {
	if u == nil || u.Papel != "chefe_setor" {
		return nil
	}
	sAtivo := setorDoUsuario(a, u)
	if sAtivo == nil || !a.chefeComandaSetor(u, *sAtivo) {
		return nil
	}
	return sAtivo
}
