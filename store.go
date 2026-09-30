package main

// Persistência do SCI — SQLite em arquivo, pragmas de durabilidade, schema + migrações.
// Doutrina: TODA a informação vive em arquivo (banco + backups); zero estado
// fonte-de-verdade fora do filesystem.

import (
	crand "crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

type Store struct {
	db      *sql.DB
	dataDir string
	arquivo string // caminho absoluto do sci.db (swap do importar-backup)
	dsn     string
}

func AbrirStore(dataDir string) (*Store, error) {
	// caminho ABSOLUTO: VACUUM INTO e logs resolvem contra CWD do processo
	abs, err := filepath.Abs(dataDir)
	if err != nil {
		return nil, err
	}
	dataDir = abs
	if err := os.MkdirAll(dataDir, 0o750); err != nil {
		return nil, err
	}
	dsn := "file:" + filepath.Join(dataDir, "sci.db") +
		"?_pragma=busy_timeout(5000)" +
		"&_pragma=foreign_keys(1)" +
		"&_pragma=journal_mode(WAL)" +
		"&_pragma=synchronous(FULL)" +
		"&_txlock=immediate"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1) // escritor serializado — volume ínfimo, zero SQLITE_BUSY
	db.SetMaxIdleConns(1)
	if err := db.Ping(); err != nil {
		return nil, err
	}
	s := &Store{db: db, dataDir: dataDir, arquivo: filepath.Join(dataDir, "sci.db"), dsn: dsn}
	if err := s.migrar(); err != nil {
		return nil, err
	}
	if err := s.migrarV4(); err != nil {
		return nil, err
	}
	if err := s.migrarV5(); err != nil {
		return nil, err
	}
	if err := s.migrarV7(); err != nil {
		return nil, err
	}
	if err := s.migrarV8(); err != nil {
		return nil, err
	}
	if err := s.migrarV9(); err != nil {
		return nil, err
	}
	if err := s.migrarV10(); err != nil {
		return nil, err
	}
	if err := s.migrarV11(); err != nil {
		return nil, err
	}
	if err := s.migrarV12(); err != nil {
		return nil, err
	}
	if err := s.migrarV13(); err != nil {
		return nil, err
	}
	if err := s.migrarV14(); err != nil {
		return nil, err
	}
	if err := s.migrarV15(); err != nil {
		return nil, err
	}
	if err := s.migrarV16(); err != nil {
		return nil, err
	}
	if err := s.migrarV17(); err != nil {
		return nil, err
	}
	if err := s.migrarV18(); err != nil {
		return nil, err
	}
	if err := s.migrarV19(); err != nil {
		return nil, err
	}
	return s, nil
}

func (s *Store) Close() error { return s.db.Close() }

func (s *Store) migrar() error {
	ddl := []string{
		`CREATE TABLE IF NOT EXISTS schema_migrations (
			versao INTEGER PRIMARY KEY,
			aplicada_em TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
		)`,
		`CREATE TABLE IF NOT EXISTS grupos (
			id INTEGER PRIMARY KEY,
			nome TEXT NOT NULL UNIQUE,
			criado_em TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
		)`,
		`CREATE TABLE IF NOT EXISTS setores (
			id INTEGER PRIMARY KEY, nome TEXT NOT NULL UNIQUE, sigla TEXT,
			ativo INTEGER NOT NULL DEFAULT 1 CHECK (ativo IN (0,1))
		)`,
		`CREATE TABLE IF NOT EXISTS funcoes (
			id INTEGER PRIMARY KEY, nome TEXT NOT NULL UNIQUE,
			ativo INTEGER NOT NULL DEFAULT 1 CHECK (ativo IN (0,1))
		)`,
		`CREATE TABLE IF NOT EXISTS destinos (
			id INTEGER PRIMARY KEY, nome TEXT NOT NULL UNIQUE,
			ativo INTEGER NOT NULL DEFAULT 1 CHECK (ativo IN (0,1))
		)`,
		`CREATE TABLE IF NOT EXISTS tags (
			id INTEGER PRIMARY KEY, nome TEXT NOT NULL UNIQUE, cor TEXT,
			ativo INTEGER NOT NULL DEFAULT 1 CHECK (ativo IN (0,1))
		)`,
		`CREATE TABLE IF NOT EXISTS conferencia_tipos (
			id INTEGER PRIMARY KEY, nome TEXT NOT NULL UNIQUE,
			ativo INTEGER NOT NULL DEFAULT 1 CHECK (ativo IN (0,1))
		)`,
		`CREATE TABLE IF NOT EXISTS pessoas (
			id INTEGER PRIMARY KEY,
			nome_guerra TEXT NOT NULL,
			nome_completo TEXT NOT NULL,
			setor_id INTEGER REFERENCES setores(id),
			funcao_id INTEGER REFERENCES funcoes(id),
			grupo_id INTEGER REFERENCES grupos(id),
			status TEXT NOT NULL DEFAULT 'ativo',
			criado_em TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
			atualizado_em TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
		)`,
		`CREATE INDEX IF NOT EXISTS idx_pessoas_setor ON pessoas(setor_id)`,
		`CREATE INDEX IF NOT EXISTS idx_pessoas_status ON pessoas(status)`,
		`CREATE TABLE IF NOT EXISTS usuarios (
			id INTEGER PRIMARY KEY,
			login TEXT NOT NULL UNIQUE COLLATE NOCASE,
			senha_hash TEXT NOT NULL,
			papel TEXT NOT NULL, -- admin | gerente | operador (validação no app)
			pessoa_id INTEGER UNIQUE REFERENCES pessoas(id),
			grupo_id INTEGER REFERENCES grupos(id), -- NULL = admin global
			ativo INTEGER NOT NULL DEFAULT 1 CHECK (ativo IN (0,1)),
			criado_em TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
			ultimo_login TEXT,
			senhas TEXT NOT NULL DEFAULT '[]'
		)`,
		`CREATE TABLE IF NOT EXISTS sessoes (
			token_hash TEXT PRIMARY KEY,
			usuario_id INTEGER NOT NULL REFERENCES usuarios(id) ON DELETE CASCADE,
			criada_em TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
			expira_em TEXT NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS conferencias (
			id INTEGER PRIMARY KEY,
			data TEXT NOT NULL,
			hora TEXT,
			tipo_id INTEGER NOT NULL REFERENCES conferencia_tipos(id),
			local TEXT,
			grupo_id INTEGER REFERENCES grupos(id),
			status TEXT NOT NULL DEFAULT 'aberta' CHECK (status IN ('aberta','fechada')),
			observacao TEXT,
			criado_por INTEGER NOT NULL REFERENCES usuarios(id),
			criado_em TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
			fechada_em TEXT,
			UNIQUE (data, tipo_id)
		)`,
		`CREATE INDEX IF NOT EXISTS idx_conferencias_data ON conferencias(data)`,
		`CREATE TABLE IF NOT EXISTS presencas (
			id INTEGER PRIMARY KEY,
			conferencia_id INTEGER NOT NULL REFERENCES conferencias(id) ON DELETE CASCADE,
			pessoa_id INTEGER NOT NULL REFERENCES pessoas(id),
			situacao TEXT NOT NULL CHECK (situacao IN ('presente','atraso','falta','justificada')),
			destino_id INTEGER REFERENCES destinos(id),
			tag_id INTEGER REFERENCES tags(id),
			observacao TEXT,
			marcado_por INTEGER NOT NULL REFERENCES usuarios(id),
			marcado_em TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
			alterado_por INTEGER REFERENCES usuarios(id),
			alterado_em TEXT,
			CHECK (situacao <> 'justificada' OR destino_id IS NOT NULL),
			UNIQUE (conferencia_id, pessoa_id)
		)`,
		`CREATE INDEX IF NOT EXISTS idx_presencas_pessoa ON presencas(pessoa_id)`,
		`CREATE INDEX IF NOT EXISTS idx_presencas_conf ON presencas(conferencia_id)`,
		`CREATE TABLE IF NOT EXISTS status_pessoal (
			id INTEGER PRIMARY KEY, nome TEXT NOT NULL UNIQUE,
			ativo INTEGER NOT NULL DEFAULT 1 CHECK (ativo IN (0,1))
		)`,
		`CREATE TABLE IF NOT EXISTS comentarios (
			id INTEGER PRIMARY KEY,
			ordem INTEGER,
			conferencia_id INTEGER NOT NULL REFERENCES conferencias(id) ON DELETE CASCADE,
			pessoa_id INTEGER NOT NULL REFERENCES pessoas(id),
			operador_id INTEGER NOT NULL REFERENCES usuarios(id),
			comentario TEXT NOT NULL,
			criado_em TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
		)`,
		`CREATE INDEX IF NOT EXISTS idx_comentarios_conf ON comentarios(conferencia_id)`,
		`CREATE TABLE IF NOT EXISTS auditoria (
			id INTEGER PRIMARY KEY,
			usuario_id INTEGER REFERENCES usuarios(id),
			acao TEXT NOT NULL,
			entidade TEXT NOT NULL,
			registro_id INTEGER,
			detalhes TEXT,
			ip TEXT,
			em TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
		)`,
		`CREATE INDEX IF NOT EXISTS idx_auditoria_em ON auditoria(em)`,
	}
	for _, q := range ddl {
		if _, err := s.db.Exec(q); err != nil {
			return fmt.Errorf("ddl: %w", err)
		}
	}
	_, err := s.db.Exec(
		`INSERT INTO schema_migrations (versao) SELECT 1
		 WHERE NOT EXISTS (SELECT 1 FROM schema_migrations WHERE versao = 1)`)
	return err
}

// colunaExiste: triagem para migrações idempotentes.
func (s *Store) colunaExiste(tabela, coluna string) bool {
	rows, err := s.db.Query(`PRAGMA table_info(` + tabela + `)`)
	if err != nil {
		return false
	}
	defer rows.Close()
	for rows.Next() {
		var cid int
		var nome, tipo string
		var notNull int
		var dflt any
		var pk int
		if rows.Scan(&cid, &nome, &tipo, &notNull, &dflt, &pk) == nil && nome == coluna {
			return true
		}
	}
	return false
}

func (s *Store) marcarVersao(v int) error {
	_, err := s.db.Exec(`INSERT INTO schema_migrations (versao) SELECT ? WHERE NOT EXISTS
		(SELECT 1 FROM schema_migrations WHERE versao = ?)`, v, v)
	return err
}

// migrarV4: grupos + comentários + status_pessoal + grupo_id nos granulares.
// Idempotente: detecta colunas/tabelas já criadas pelo DDL novo.
func (s *Store) migrarV4() error {
	var n int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM schema_migrations WHERE versao = 4`).Scan(&n); err != nil {
		return err
	}
	if n > 0 {
		return nil
	}
	steps := []string{}
	if !s.colunaExiste("pessoas", "grupo_id") {
		steps = append(steps,
			`ALTER TABLE pessoas ADD COLUMN grupo_id INTEGER REFERENCES grupos(id)`)
	}
	if !s.colunaExiste("conferencias", "grupo_id") {
		steps = append(steps,
			`ALTER TABLE conferencias ADD COLUMN grupo_id INTEGER REFERENCES grupos(id)`)
	}
	// várias conferências por dia (ordem Tenente 28/09): rebuild p/ remover UNIQUE(data,tipo_id)
	// CRÍTICO (revisão TAKEDA): DROP com FK ativa cascata em presencas/comentarios —
	// desliga FK apenas nesta conexão, rebuild, checa integridade e religa.
	ddlConferencias := ""
	if err := s.db.QueryRow(`SELECT COALESCE(sql,'') FROM sqlite_master WHERE type='table' AND name='conferencias'`).Scan(&ddlConferencias); err != nil {
		return err
	}
	if strings.Contains(strings.ToUpper(ddlConferencias), "UNIQUE") {
		if _, err := s.db.Exec(`PRAGMA foreign_keys=OFF`); err != nil {
			return err
		}
		steps = append(steps,
			`CREATE TABLE conferencias_v4 (
				id INTEGER PRIMARY KEY,
				data TEXT NOT NULL,
				hora TEXT,
				tipo_id INTEGER NOT NULL REFERENCES conferencia_tipos(id),
				local TEXT,
				grupo_id INTEGER REFERENCES grupos(id),
				status TEXT NOT NULL DEFAULT 'aberta' CHECK (status IN ('aberta','fechada')),
				observacao TEXT,
				criado_por INTEGER NOT NULL REFERENCES usuarios(id),
				criado_em TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
				fechada_em TEXT
			)`,
			`INSERT INTO conferencias_v4 (id, data, hora, tipo_id, local, grupo_id, status,
				observacao, criado_por, criado_em, fechada_em)
			 SELECT id, data, hora, tipo_id, local, grupo_id, status,
				observacao, criado_por, criado_em, fechada_em FROM conferencias`,
			`DROP TABLE conferencias`,
			`ALTER TABLE conferencias_v4 RENAME TO conferencias`,
			`CREATE INDEX IF NOT EXISTS idx_conferencias_data ON conferencias(data)`,
		)
	}
	if !s.colunaExiste("usuarios", "grupo_id") {
		steps = append(steps,
			`ALTER TABLE usuarios ADD COLUMN grupo_id INTEGER REFERENCES grupos(id)`)
	}
	if !s.colunaExiste("usuarios", "senhas") {
		steps = append(steps,
			`ALTER TABLE usuarios ADD COLUMN senhas TEXT NOT NULL DEFAULT '[]'`)
	}
	// CHECK do papel: em legado era IN('admin','usuario'); rebuild só se necessário
	steps = append(steps,
		`CREATE INDEX IF NOT EXISTS idx_comentarios_conf ON comentarios(conferencia_id)`,
		`CREATE INDEX IF NOT EXISTS idx_conferencias_data ON conferencias(data)`,
		`INSERT OR IGNORE INTO status_pessoal (nome) VALUES ('Ativo'),('Inativo')`,
		`INSERT OR IGNORE INTO conferencia_tipos (nome) VALUES ('Conferência de pessoal')`,
	)
	for _, q := range steps {
		if _, err := s.db.Exec(q); err != nil {
			return fmt.Errorf("migração v4: %w", err)
		}
	}
	// religa FK e prova integridade (revisão TAKEDA)
	if _, err := s.db.Exec(`PRAGMA foreign_keys=ON`); err != nil {
		return err
	}
	var fkViol int
	if err := s.db.QueryRow(`PRAGMA foreign_key_check`).Scan(&fkViol); err == nil && fkViol > 0 {
		return fmt.Errorf("migração v4: foreign_key_check detectou %d violações", fkViol)
	}
	return s.marcarVersao(4)
}

// migrarV5: hierarquia de grupos (ordem Tenente, 28/09 noite) — código de 6 dígitos,
// vínculo BILATERAL (superior cadastra o inferior, inferior confirma) e perfil do usuário.
func (s *Store) migrarV5() error {
	var n int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM schema_migrations WHERE versao = 5`).Scan(&n); err != nil {
		return err
	}
	if n > 0 {
		return nil
	}
	steps := []string{
		// 6 dígitos (gerados em Go abaixo, antes do índice único)
		`ALTER TABLE grupos ADD COLUMN codigo TEXT`,
		// vínculos de hierarquia: bilateral, sem auto-vínculo, sem duplicata
		`CREATE TABLE IF NOT EXISTS grupo_vinculos (
			id INTEGER PRIMARY KEY,
			superior_id INTEGER NOT NULL REFERENCES grupos(id),
			subordinado_id INTEGER NOT NULL REFERENCES grupos(id),
			criado_por_superior INTEGER NOT NULL DEFAULT 0,
			criado_por_subordinado INTEGER NOT NULL DEFAULT 0,
			criado_em TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
			CHECK (superior_id <> subordinado_id),
			UNIQUE (superior_id, subordinado_id)
		)`,
		`CREATE INDEX IF NOT EXISTS idx_gv_sup ON grupo_vinculos(superior_id)`,
		`CREATE INDEX IF NOT EXISTS idx_gv_sub ON grupo_vinculos(subordinado_id)`,
		// catálogos por grupo (NULL = global/admin): subordinado herda os do superior
		`ALTER TABLE setores ADD COLUMN grupo_id INTEGER REFERENCES grupos(id)`,
		`ALTER TABLE funcoes ADD COLUMN grupo_id INTEGER REFERENCES grupos(id)`,
		`ALTER TABLE destinos ADD COLUMN grupo_id INTEGER REFERENCES grupos(id)`,
		`ALTER TABLE tags ADD COLUMN grupo_id INTEGER REFERENCES grupos(id)`,
		`ALTER TABLE conferencia_tipos ADD COLUMN grupo_id INTEGER REFERENCES grupos(id)`,
		// perfil do usuário (aba Meu usuário): dados espelham o banco de pessoal
		`ALTER TABLE usuarios ADD COLUMN nome_guerra TEXT`,
		`ALTER TABLE usuarios ADD COLUMN nome_completo TEXT`,
		`ALTER TABLE usuarios ADD COLUMN setor_id INTEGER REFERENCES setores(id)`,
		`ALTER TABLE usuarios ADD COLUMN funcao_id INTEGER REFERENCES funcoes(id)`,
	}
	for _, q := range steps {
		if _, err := s.db.Exec(q); err != nil {
			// falha parcial anterior reexecuta o ALTER — inofensivo
			if strings.Contains(err.Error(), "duplicate column name") {
				continue
			}
			return fmt.Errorf("migração v5: %w", err)
		}
	}
	// códigos para grupos existentes (sem SQL frágil: rand em Go, com retry de colisão)
	ids := []int64{}
	rows, err := s.db.Query(`SELECT id FROM grupos WHERE codigo IS NULL OR codigo = ''`)
	if err != nil {
		return err
	}
	for rows.Next() {
		var id int64
		if rows.Scan(&id) == nil {
			ids = append(ids, id)
		}
	}
	rows.Close()
	for _, id := range ids {
		for tent := 0; tent < 8; tent++ {
			var existe int
			cod := gerarCodigoGrupo()
			_ = s.db.QueryRow(`SELECT COUNT(*) FROM grupos WHERE codigo = ?`, cod).Scan(&existe)
			if existe > 0 {
				continue
			}
			if _, err := s.db.Exec(`UPDATE grupos SET codigo = ? WHERE id = ?`, cod, id); err == nil {
				break
			}
		}
	}
	if _, err := s.db.Exec(`CREATE UNIQUE INDEX IF NOT EXISTS uq_grupos_codigo ON grupos(codigo)`); err != nil {
		return err
	}
	return s.marcarVersao(5)
}

// migrarV7 (v9.7): safeguard anti-duplicata de pessoa — mesmo militar cadastrado
// duas vezes no MESMO grupo (nome de guerra + nome completo idênticos, case-insensitive).
// Funde duplicatas existentes (mantém a mais antiga, remigra lançamentos) e cria índice único.
func (s *Store) migrarV7() error {
	var n int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM schema_migrations WHERE versao = 7`).Scan(&n); err != nil {
		return err
	}
	if n > 0 {
		return nil
	}
	// 1) fundir duplicatas: para cada par (grupo_id, nome_guerra, nome_completo) com >1,
	// preserva o id mais antigo e aponta os lançamentos para ele antes de apagar os demais.
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	idsManter := []int64{}
	rows, err := tx.Query(`
		SELECT MIN(id) FROM pessoas
		GROUP BY grupo_id, LOWER(TRIM(nome_guerra)), LOWER(TRIM(nome_completo))
		HAVING COUNT(*) > 1`)
	if err != nil {
		tx.Rollback()
		return err
	}
	for rows.Next() {
		var manter int64
		if rows.Scan(&manter) == nil {
			idsManter = append(idsManter, manter)
		}
	}
	rows.Close()
	for _, manter := range idsManter {
		if _, err = tx.Exec(`
			UPDATE presencas SET pessoa_id = ? WHERE pessoa_id IN (
				SELECT p2.id FROM pessoas p1 JOIN pessoas p2
				  ON p2.id <> ? AND p1.id = ?
				 AND p2.grupo_id IS p1.grupo_id
				 AND LOWER(TRIM(p2.nome_guerra)) = LOWER(TRIM(p1.nome_guerra))
				 AND LOWER(TRIM(p2.nome_completo)) = LOWER(TRIM(p1.nome_completo))
			)`, manter, manter, manter); err != nil {
			tx.Rollback()
			return fmt.Errorf("fusão de duplicatas (pessoa %d): %w", manter, err)
		}
		if _, err = tx.Exec(`
			DELETE FROM pessoas WHERE id <> ? AND id NOT IN (
				SELECT pessoa_id FROM presencas
			) AND id IN (
				SELECT p2.id FROM pessoas p1 JOIN pessoas p2
				  ON p2.id <> ? AND p1.id = ?
				 AND p2.grupo_id IS p1.grupo_id
				 AND LOWER(TRIM(p2.nome_guerra)) = LOWER(TRIM(p1.nome_guerra))
				 AND LOWER(TRIM(p2.nome_completo)) = LOWER(TRIM(p1.nome_completo))
			)`, manter, manter, manter); err != nil {
			tx.Rollback()
			return fmt.Errorf("remoção de duplicatas (pessoa %d): %w", manter, err)
		}
	}
	if err = tx.Commit(); err != nil {
		return err
	}
	// 2) índice único (NULL grupo_id conta como grupo distinto — coluna com IS NULL)
	if _, err := s.db.Exec(`CREATE UNIQUE INDEX IF NOT EXISTS uq_pessoas_identidade
		ON pessoas(grupo_id, LOWER(TRIM(nome_guerra)), LOWER(TRIM(nome_completo)))`); err != nil {
		// se ainda houver duplicata (caso raríssimo com lançamentos em ambas), segue sem índice —
		// a checagem no hPessoasAdd continua bloqueando novas duplicatas
		log.Printf("sci migrarV7: índice único adiado (duplicatas com histórico): %v", err)
		return s.marcarVersao(7)
	}
	return s.marcarVersao(7)
}

// migrarV8 (v9.9, ordem Tenente 29/09): catálogos de FUNÇÃO, SETOR e TAG nascem ZEROS —
// todo grupo começa sem nenhum item; herança é só de LEITURA (o que o grupo de cima cria
// desce para os de baixo; nunca o contrário). Remove os seeds globais de setores/funcoes/
// tags. Mantidos: destinos, conferencia_tipos e status_pessoal (operacionais, não de
// organização). Ordem: apagar seeds globais (só os SEM lançamento/setor referenciando;
// os em uso viram órfãos ativos preservando histórico), marca versão 8.
func (s *Store) migrarV8() error {
	var n int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM schema_migrations WHERE versao = 8`).Scan(&n); err != nil {
		return err
	}
	if n > 0 {
		return nil
	}
	// seeds globais de organização (grupo_id IS NULL) saem — exceto os em uso por
	// pessoas/lançamentos (imutabilidade histórica preservada: ficam mas invisíveis
	// para novos grupos pois a herança só desce; mais limpo: desativa em vez de apagar)
	// v9.11: DESTINOS também — tudo de organização nasce zerado.
	for _, q := range []string{
		`UPDATE setores SET ativo = 0 WHERE grupo_id IS NULL AND id NOT IN (SELECT DISTINCT setor_id FROM pessoas WHERE setor_id IS NOT NULL)`,
		`UPDATE funcoes SET ativo = 0 WHERE grupo_id IS NULL AND id NOT IN (SELECT DISTINCT funcao_id FROM pessoas WHERE funcao_id IS NOT NULL)`,
		`UPDATE tags SET ativo = 0 WHERE grupo_id IS NULL`,
		`UPDATE destinos SET ativo = 0 WHERE grupo_id IS NULL`,
	} {
		if _, err := s.db.Exec(q); err != nil {
			return fmt.Errorf("migração v8: %w", err)
		}
	}
	return s.marcarVersao(8)
}

// migrarV9 (v9.11, ordem Tenente 29/09): HIERARQUIA nos catálogos — pai_id (self-FK)
// em setores/funcoes/tags/destinos. NULL = raiz. Facilita filtragem/ordenação em
// queries (herança visual pai→filho dentro do próprio grupo). Idempotente.
func (s *Store) migrarV9() error {
	var n int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM schema_migrations WHERE versao = 9`).Scan(&n); err != nil {
		return err
	}
	if n > 0 {
		return nil
	}
	for _, tab := range []string{"setores", "funcoes", "tags", "destinos"} {
		if _, err := s.db.Exec(`ALTER TABLE ` + tab + ` ADD COLUMN pai_id INTEGER REFERENCES ` + tab + `(id)`); err != nil {
			if !strings.Contains(err.Error(), "duplicate column name") {
				return fmt.Errorf("migração v9 (%s): %w", tab, err)
			}
		}
		if _, err := s.db.Exec(`CREATE INDEX IF NOT EXISTS idx_` + tab + `_pai ON ` + tab + `(pai_id)`); err != nil {
			return fmt.Errorf("migração v9 idx (%s): %w", tab, err)
		}
	}
	return s.marcarVersao(9)
}

// migrarV10 (ordem Tenente 29/09: "destinos parecem hardcoded"): APAGA as seeds globais
// de destinos (grupo_id IS NULL) que NÃO são referenciadas por lançamentos — bancos novos
// já nascem sem o INSERT (removido do schema base). As referenciadas por histórico ficam
// (imutabilidade), mas invisíveis para grupos que não as usam.
func (s *Store) migrarV10() error {
	var v int
	_ = s.db.QueryRow(`SELECT versao FROM schema_migrations WHERE versao = 10`).Scan(&v)
	if v == 10 {
		return nil
	}
	if _, err := s.db.Exec(`DELETE FROM destinos WHERE grupo_id IS NULL AND id NOT IN
		(SELECT DISTINCT destino_id FROM presencas WHERE destino_id IS NOT NULL)`); err != nil {
		return err
	}
	return s.marcarVersao(10)
}

// migrarV11 (v9.16.4, ordem Tenente 29/09): checkbox de VERIFICAÇÃO passa a persistir
// separado da situação. Carry over insere verificado=0 (checkbox começa zerado); o ✅ do
// operador grava verificado=1 via /marcar.
func (s *Store) migrarV11() error {
	var v int
	_ = s.db.QueryRow(`SELECT versao FROM schema_migrations WHERE versao = 11`).Scan(&v)
	if v == 11 {
		return nil
	}
	var col int
	_ = s.db.QueryRow(`SELECT COUNT(*) FROM pragma_table_info('presencas') WHERE name='verificado'`).Scan(&col)
	if col == 0 {
		if _, err := s.db.Exec(`ALTER TABLE presencas ADD COLUMN verificado INTEGER NOT NULL DEFAULT 0`); err != nil {
			return err
		}
	}
	return s.marcarVersao(11)
}

// migrarV12 (v9.16.8, ordem Tenente 29/09): ANTIGUIDADE nos catálogos de organização —
// definida por drag & drop no painel do gerente; herdadas sempre anteriores às do grupo.
func (s *Store) migrarV12() error {
	var v int
	_ = s.db.QueryRow(`SELECT versao FROM schema_migrations WHERE versao = 12`).Scan(&v)
	if v == 12 {
		return nil
	}
	for _, t := range []string{"setores", "funcoes", "tags", "destinos"} {
		var col int
		_ = s.db.QueryRow(`SELECT COUNT(*) FROM pragma_table_info('` + t + `') WHERE name='antiguidade'`).Scan(&col)
		if col == 0 {
			if _, err := s.db.Exec(`ALTER TABLE ` + t + ` ADD COLUMN antiguidade INTEGER NOT NULL DEFAULT 0`); err != nil {
				return err
			}
		}
	}
	return s.marcarVersao(12)
}

// migrarV13 (v1.0): Módulo de Escalas e Serviços Integrados.
func (s *Store) migrarV13() error {
	var v int
	_ = s.db.QueryRow(`SELECT versao FROM schema_migrations WHERE versao = 13`).Scan(&v)
	if v == 13 {
		return nil
	}
	ddl := []string{
		`CREATE TABLE IF NOT EXISTS escala_tipos (
			id INTEGER PRIMARY KEY,
			grupo_id INTEGER REFERENCES grupos(id),
			nome TEXT NOT NULL,
			descricao TEXT,
			ativo INTEGER NOT NULL DEFAULT 1 CHECK (ativo IN (0,1)),
			criado_em TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
		)`,
		`CREATE TABLE IF NOT EXISTS escala_turnos (
			id INTEGER PRIMARY KEY,
			grupo_id INTEGER NOT NULL REFERENCES grupos(id),
			tipo_id INTEGER NOT NULL REFERENCES escala_tipos(id),
			data_inicio TEXT NOT NULL,
			data_fim TEXT NOT NULL,
			observacao TEXT,
			criado_por INTEGER REFERENCES usuarios(id),
			criado_em TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
		)`,
		`CREATE INDEX IF NOT EXISTS idx_escala_turnos_grupo ON escala_turnos(grupo_id)`,
		`CREATE INDEX IF NOT EXISTS idx_escala_turnos_datas ON escala_turnos(data_inicio, data_fim)`,
		`CREATE TABLE IF NOT EXISTS escala_pessoas (
			id INTEGER PRIMARY KEY,
			turno_id INTEGER NOT NULL REFERENCES escala_turnos(id) ON DELETE CASCADE,
			pessoa_id INTEGER NOT NULL REFERENCES pessoas(id) ON DELETE CASCADE,
			funcao_escala TEXT,
			UNIQUE(turno_id, pessoa_id)
		)`,
		`CREATE INDEX IF NOT EXISTS idx_escala_pessoas_pessoa ON escala_pessoas(pessoa_id)`,
		`CREATE INDEX IF NOT EXISTS idx_escala_pessoas_turno ON escala_pessoas(turno_id)`,
	}
	for _, q := range ddl {
		if _, err := s.db.Exec(q); err != nil {
			return fmt.Errorf("migração v13 ddl: %w", err)
		}
	}
	// Seeds de tipos padrão de serviço
	seeds := []string{
		"Oficial de Dia", "Adjunto ao Oficial de Dia", "Sargento de Dia",
		"Cabo da Guarda", "Sentinela / Guarda do Quartel", "Plantonista",
		"Permanência", "Motorista de Dia",
	}
	for _, sNome := range seeds {
		_, _ = s.db.Exec(`INSERT INTO escala_tipos (nome, ativo)
			SELECT ?, 1 WHERE NOT EXISTS (SELECT 1 FROM escala_tipos WHERE nome = ? AND grupo_id IS NULL)`,
			sNome, sNome)
	}
	return s.marcarVersao(13)
}

// migrarV14 (v1.0): Módulo de Material, Reserva de Armamento e Cautelas.
func (s *Store) migrarV14() error {
	var v int
	_ = s.db.QueryRow(`SELECT versao FROM schema_migrations WHERE versao = 14`).Scan(&v)
	if v == 14 {
		return nil
	}
	ddl := []string{
		`CREATE TABLE IF NOT EXISTS material_categorias (
			id INTEGER PRIMARY KEY,
			grupo_id INTEGER REFERENCES grupos(id),
			nome TEXT NOT NULL,
			ativo INTEGER NOT NULL DEFAULT 1 CHECK (ativo IN (0,1))
		)`,
		`CREATE TABLE IF NOT EXISTS material_itens (
			id INTEGER PRIMARY KEY,
			grupo_id INTEGER NOT NULL REFERENCES grupos(id),
			categoria_id INTEGER REFERENCES material_categorias(id),
			nome TEXT NOT NULL,
			codigo_patrimonio TEXT NOT NULL,
			numero_serie TEXT,
			status TEXT NOT NULL DEFAULT 'disponivel' CHECK (status IN ('disponivel', 'acautelado', 'manutencao', 'baixado')),
			observacao TEXT,
			criado_em TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
			UNIQUE(grupo_id, codigo_patrimonio)
		)`,
		`CREATE INDEX IF NOT EXISTS idx_material_itens_grupo ON material_itens(grupo_id)`,
		`CREATE INDEX IF NOT EXISTS idx_material_itens_status ON material_itens(status)`,
		`CREATE TABLE IF NOT EXISTS material_cautelas (
			id INTEGER PRIMARY KEY,
			item_id INTEGER NOT NULL REFERENCES material_itens(id) ON DELETE CASCADE,
			pessoa_id INTEGER NOT NULL REFERENCES pessoas(id),
			responsavel_entrega_id INTEGER NOT NULL REFERENCES usuarios(id),
			responsavel_recebimento_id INTEGER REFERENCES usuarios(id),
			data_saida TEXT NOT NULL,
			data_devolucao TEXT,
			obs_saida TEXT,
			obs_devolucao TEXT,
			status TEXT NOT NULL DEFAULT 'ativa' CHECK (status IN ('ativa', 'devolvida'))
		)`,
		`CREATE INDEX IF NOT EXISTS idx_cautelas_item ON material_cautelas(item_id)`,
		`CREATE INDEX IF NOT EXISTS idx_cautelas_pessoa ON material_cautelas(pessoa_id)`,
		`CREATE INDEX IF NOT EXISTS idx_cautelas_status ON material_cautelas(status)`,
	}
	for _, q := range ddl {
		if _, err := s.db.Exec(q); err != nil {
			return fmt.Errorf("migração v14 ddl: %w", err)
		}
	}
	// Seeds de categorias padrão de materiais
	cats := []string{
		"Armamento", "Munição & Carregadores", "Comunicação / Rádios",
		"Viaturas & Chaves", "Equipamentos de Proteção / EPI", "Informática & TI",
		"Ferramental / Diversos",
	}
	for _, cNome := range cats {
		_, _ = s.db.Exec(`INSERT INTO material_categorias (nome, ativo)
			SELECT ?, 1 WHERE NOT EXISTS (SELECT 1 FROM material_categorias WHERE nome = ? AND grupo_id IS NULL)`,
			cNome, cNome)
	}
	return s.marcarVersao(14)
}

// migrarV15 (v1.0): Configurações do Sistema e Suporte White-Label (Multi-Instituição).
func (s *Store) migrarV15() error {
	var v int
	_ = s.db.QueryRow(`SELECT versao FROM schema_migrations WHERE versao = 15`).Scan(&v)
	if v == 15 {
		return nil
	}
	ddl := []string{
		`CREATE TABLE IF NOT EXISTS configuracoes (
			chave TEXT PRIMARY KEY,
			valor TEXT NOT NULL,
			atualizado_em TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
		)`,
	}
	for _, q := range ddl {
		if _, err := s.db.Exec(q); err != nil {
			return fmt.Errorf("migração v15 ddl: %w", err)
		}
	}
	defaults := map[string]string{
		"NOME_SISTEMA":          "SCI",
		"SUBTITULO_SISTEMA":     "Controle Interno",
		"TITULO_ORGANIZACAO":    "Organização",
		"ROTULO_GRUPO":          "Companhia / Subunidade",
		"ROTULO_SETOR":          "Pelotão / Seção",
		"ROTULO_FUNCAO":         "Função",
		"ROTULO_PESSOA":         "Militar",
		"ROTULO_IDENTIFICADOR":  "Nome de Guerra",
		"COR_PRIMARIA":          "#10b981",
		"COR_PRIMARIA_CLARO":    "#34d399",
		"COR_PRIMARIA_ESCURO":   "#065f46",
	}
	for k, v := range defaults {
		_, _ = s.db.Exec(`INSERT INTO configuracoes (chave, valor)
			SELECT ?, ? WHERE NOT EXISTS (SELECT 1 FROM configuracoes WHERE chave = ?)`,
			k, v, k)
	}
	return s.marcarVersao(15)
}

// migrarV16 (v1.0): Anexos e Escaneamentos em Cautelas de Material.
func (s *Store) migrarV16() error {
	var v int
	_ = s.db.QueryRow(`SELECT versao FROM schema_migrations WHERE versao = 16`).Scan(&v)
	if v == 16 {
		return nil
	}
	ddl := []string{
		`CREATE TABLE IF NOT EXISTS material_cautela_anexos (
			id INTEGER PRIMARY KEY,
			cautela_id INTEGER NOT NULL REFERENCES material_cautelas(id) ON DELETE CASCADE,
			nome_arquivo TEXT NOT NULL,
			tipo_mime TEXT NOT NULL,
			tamanho INTEGER NOT NULL,
			dados_base64 TEXT NOT NULL,
			criado_em TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
		)`,
		`CREATE INDEX IF NOT EXISTS idx_cautela_anexos_cautela ON material_cautela_anexos(cautela_id)`,
	}
	for _, q := range ddl {
		if _, err := s.db.Exec(q); err != nil {
			return fmt.Errorf("migração v16 ddl: %w", err)
		}
	}
	return s.marcarVersao(16)
}

// migrarV17 (ordem Tenente 30/09): situação NAO_VERIFICADO — o CHECK legado de
// `presencas` ('presente','atraso','falta','justificada') REJEITARIA o novo valor.
// Rebuild da tabela SEM CHECK de situação (governança pelo app), cópia de TODAS as
// linhas por interseção de colunas (bancos pré-v11 não têm `verificado`). Ídem v4:
// inspeção de DDL antes, FK off no rebuild, FK on + prova depois. Idempotente.
func (s *Store) migrarV17() error {
	var v int
	_ = s.db.QueryRow(`SELECT versao FROM schema_migrations WHERE versao = 17`).Scan(&v)
	if v == 17 {
		return nil
	}
	var ddl string
	if err := s.db.QueryRow(`SELECT COALESCE(sql,'') FROM sqlite_master WHERE type='table' AND name='presencas'`).Scan(&ddl); err != nil {
		return err
	}
	if strings.Contains(strings.ToUpper(ddl), "CHECK") {
		if _, err := s.db.Exec(`PRAGMA foreign_keys=OFF`); err != nil {
			return err
		}
		cols := []string{"id", "conferencia_id", "pessoa_id", "situacao", "destino_id",
			"tag_id", "observacao", "marcado_por", "marcado_em", "alterado_por", "alterado_em"}
		if s.colunaExiste("presencas", "verificado") {
			cols = append(cols, "verificado")
		}
		lista := strings.Join(cols, ", ")
		steps := []string{
			`CREATE TABLE presencas_v17 (
				id INTEGER PRIMARY KEY,
				conferencia_id INTEGER NOT NULL REFERENCES conferencias(id) ON DELETE CASCADE,
				pessoa_id INTEGER NOT NULL REFERENCES pessoas(id),
				situacao TEXT NOT NULL,
				destino_id INTEGER REFERENCES destinos(id),
				tag_id INTEGER REFERENCES tags(id),
				observacao TEXT,
				marcado_por INTEGER NOT NULL REFERENCES usuarios(id),
				marcado_em TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
				alterado_por INTEGER REFERENCES usuarios(id),
				alterado_em TEXT,
				verificado INTEGER NOT NULL DEFAULT 0,
				UNIQUE (conferencia_id, pessoa_id)
			)`,
			`INSERT INTO presencas_v17 (` + lista + `) SELECT ` + lista + ` FROM presencas`,
			`DROP TABLE presencas`,
			`ALTER TABLE presencas_v17 RENAME TO presencas`,
			`CREATE INDEX IF NOT EXISTS idx_presencas_pessoa ON presencas(pessoa_id)`,
			`CREATE INDEX IF NOT EXISTS idx_presencas_conf ON presencas(conferencia_id)`,
		}
		for _, q := range steps {
			if _, err := s.db.Exec(q); err != nil {
				return fmt.Errorf("migração v17: %w", err)
			}
		}
		if _, err := s.db.Exec(`PRAGMA foreign_keys=ON`); err != nil {
			return err
		}
	}
	return s.marcarVersao(17)
}

// gerarCodigoGrupo: 6 caracteres sem ambiguidade (sem 0/O, 1/I/L, 2/S óbvios? mantemos
// 32 símbolos legíveis) — revisão de leitura humana em campo.
func gerarCodigoGrupo() string {
	const alfabeto = "23456789ABCDEFGHJKMNPQRSTUVWXYZ"
	b := make([]byte, 6)
	if _, err := crand.Read(b); err != nil {
		// fallback determinístico por tempo — improvável de ocorrer
		return fmt.Sprintf("%06d", time.Now().UnixNano()%1000000)
	}
	for i := range b {
		b[i] = alfabeto[int(b[i])%len(alfabeto)]
	}
	return string(b)
}

// SeedIfEmpty cria o admin inicial e catálogos padrão na primeira execução.
func (s *Store) SeedIfEmpty(senhaAdmin string) error {
	var n int
	err := s.db.QueryRow(`SELECT COUNT(*) FROM usuarios`).Scan(&n)
	if err != nil {
		return err
	}
	if n > 0 {
		return nil
	}
	hash, err := hashSenha(senhaAdmin)
	if err != nil {
		return err
	}
	if _, err := s.db.Exec(
		`INSERT INTO usuarios (login, senha_hash, papel) VALUES ('admin', ?, 'admin')`, hash); err != nil {
		return err
	}
	for _, q := range []string{
		// v9.11 (ordem Tenente 29/09): TUDO de organização nasce ZERADO — setores,
		// funções, tags E DESTINOS. Herança só desce (leitura); edição do dono.
		// Destinos default migrados para desativados (preserva histórico de lançamentos).
		`UPDATE destinos SET ativo = 0 WHERE grupo_id IS NULL`,
		`INSERT OR IGNORE INTO status_pessoal (nome) VALUES ('Ativo'),('Inativo')`,
	} {
		if _, err := s.db.Exec(q); err != nil {
			return err
		}
	}
	s.Auditoria(nil, "seed", "usuarios", nil, "admin inicial + catálogos padrão", "")
	return nil
}

// ---------- auditoria ----------

func (s *Store) Auditoria(usuarioID *int64, acao, entidade string, registroID *int64, detalhes, ip string) {
	_, _ = s.db.Exec(
		`INSERT INTO auditoria (usuario_id, acao, entidade, registro_id, detalhes, ip)
		 VALUES (?,?,?,?,?,?)`,
		usuarioID, acao, entidade, registroID, detalhes, ip)
}

// ---------- sessões ----------

func novoToken() (cru, hash string, err error) {
	b := make([]byte, 32)
	if _, err = crand.Read(b); err != nil {
		return
	}
	cru = hex.EncodeToString(b)
	h := sha256.Sum256([]byte(cru))
	hash = hex.EncodeToString(h[:])
	return
}

func (s *Store) CriarSessao(usuarioID int64, ttl time.Duration) (token string, expira time.Time, err error) {
	cru, hash, err := novoToken()
	if err != nil {
		return
	}
	expira = time.Now().UTC().Add(ttl)
	_, err = s.db.Exec(`INSERT INTO sessoes (token_hash, usuario_id, expira_em) VALUES (?,?,?)`,
		hash, usuarioID, expira.UTC().Format(time.RFC3339))
	if err != nil {
		return
	}
	return cru, expira, nil
}

func (s *Store) UsuarioDaSessao(tokenCru string) (*Usuario, error) {
	h := sha256.Sum256([]byte(tokenCru))
	hash := hex.EncodeToString(h[:])
	var u Usuario
	err := s.db.QueryRow(
		`SELECT u.id, u.login, u.papel, u.pessoa_id, u.grupo_id,
		        COALESCE(u.nome_guerra,''), COALESCE(u.nome_completo,''), u.setor_id, u.funcao_id
		 FROM sessoes se JOIN usuarios u ON u.id = se.usuario_id
		 WHERE se.token_hash = ? AND se.expira_em > ? AND u.ativo = 1`,
		hash, time.Now().UTC().Format(time.RFC3339)).
		Scan(&u.ID, &u.Login, &u.Papel, &u.PessoaID, &u.GrupoID,
			&u.NomeGuerra, &u.NomeCompleto, &u.SetorID, &u.FuncaoID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &u, nil
}

func (s *Store) EncerrarSessao(tokenCru string) {
	h := sha256.Sum256([]byte(tokenCru))
	_, _ = s.db.Exec(`DELETE FROM sessoes WHERE token_hash = ?`, hex.EncodeToString(h[:]))
}

func (s *Store) LimparSessoesExpiradas() {
	_, _ = s.db.Exec(`DELETE FROM sessoes WHERE expira_em <= ?`, time.Now().UTC().Format(time.RFC3339))
}

// ---------- catálogos (whitelist de tabelas gerenciadas pelo admin) ----------

var catalogosValidos = map[string]bool{
	"setores": true, "funcoes": true, "destinos": true,
	"tags": true, "conferencia_tipos": true, "status_pessoal": true,
}

func tabelaDeCatalogo(tab string) (string, error) {
	if catalogosValidos[tab] {
		return tab, nil
	}
	return "", fmt.Errorf("catálogo inválido: %s", tab)
}

// ReabrirComArquivo: fecha o pool atual e reabre o banco sobre outro arquivo
// (swap atômico do IMPORTAR backup). Migrações v2/v4/v5/v6 rodam na reabertura.
func (s *Store) ReabrirComArquivo(novoArquivo string) error {
	if err := s.db.Close(); err != nil {
		return fmt.Errorf("fechar pool: %w", err)
	}
	s.arquivo = novoArquivo
	dsn := "file:" + novoArquivo +
		"?_pragma=busy_timeout(5000)" +
		"&_pragma=foreign_keys(1)" +
		"&_pragma=journal_mode(WAL)" +
		"&_pragma=synchronous(FULL)" +
		"&_txlock=immediate"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return err
	}
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	if err := db.Ping(); err != nil {
		return err
	}
	s.db = db
	s.dsn = dsn
	if err := s.migrarV2(); err != nil {
		return err
	}
	if err := s.migrar(); err != nil {
		return err
	}
	if err := s.migrarV4(); err != nil {
		return err
	}
	if err := s.migrarV5(); err != nil {
		return err
	}
	if err := s.migrarV6(); err != nil {
		return err
	}
	if err := s.migrarV7(); err != nil {
		return err
	}
	if err := s.migrarV8(); err != nil {
		return err
	}
	if err := s.migrarV9(); err != nil {
		return err
	}
	if err := s.migrarV10(); err != nil {
		return err
	}
	if err := s.migrarV11(); err != nil {
		return err
	}
	return s.migrarV12()
}

// migrarV6: papéis limpos — 'usuario' passa a se chamar 'operador' (v9.3).
// Idempotente: UPDATE só bate com linhas antigas; versão marcada uma vez.
func (s *Store) migrarV6() error {
	if _, err := s.db.Exec(`UPDATE usuarios SET papel = 'operador' WHERE papel = 'usuario'`); err != nil {
		return err
	}
	return s.marcarVersao(6)
}

// migrarV19: nível de sensibilidade em itens de material + SLA de devolução e Webhooks
func (s *Store) migrarV19() error {
	var v int
	_ = s.db.QueryRow(`SELECT versao FROM schema_migrations WHERE versao = 19`).Scan(&v)
	if v == 19 {
		return nil
	}
	if !s.colunaExiste("material_itens", "nivel_sensibilidade") {
		if _, err := s.db.Exec(`ALTER TABLE material_itens ADD COLUMN nivel_sensibilidade TEXT NOT NULL DEFAULT 'padrao'`); err != nil {
			if !strings.Contains(err.Error(), "duplicate column") {
				return fmt.Errorf("migração v19 nivel_sensibilidade: %w", err)
			}
		}
	}
	_, _ = s.db.Exec(`INSERT INTO configuracoes (chave, valor)
		SELECT 'CAUTELA_PRAZO_PADRAO_HORAS', '24'
		WHERE NOT EXISTS (SELECT 1 FROM configuracoes WHERE chave = 'CAUTELA_PRAZO_PADRAO_HORAS')`)
	_, _ = s.db.Exec(`INSERT INTO configuracoes (chave, valor)
		SELECT 'WEBHOOK_ATRASOS_URL', ''
		WHERE NOT EXISTS (SELECT 1 FROM configuracoes WHERE chave = 'WEBHOOK_ATRASOS_URL')`)
	_, _ = s.db.Exec(`UPDATE configuracoes SET valor = 'Organização' WHERE chave = 'TITULO_ORGANIZACAO' AND valor = '3º B Com GE'`)
	_, _ = s.db.Exec(`UPDATE configuracoes SET valor = '#10b981' WHERE chave = 'COR_PRIMARIA' AND valor = '#57a173'`)
	_, _ = s.db.Exec(`UPDATE configuracoes SET valor = '#34d399' WHERE chave = 'COR_PRIMARIA_CLARO' AND valor = '#8fd2a9'`)
	_, _ = s.db.Exec(`UPDATE configuracoes SET valor = '#065f46' WHERE chave = 'COR_PRIMARIA_ESCURO' AND valor = '#275e42'`)

	return s.marcarVersao(19)
}
