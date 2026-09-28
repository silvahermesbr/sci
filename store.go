package main

// Persistência do SCI — SQLite em arquivo, pragmas de durabilidade, schema v2.
// Doutrina: TODA a informação vive em arquivo (banco + backups); zero estado
// fonte-de-verdade fora do filesystem.

import (
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	_ "modernc.org/sqlite"
)

type Store struct {
	db      *sql.DB
	dataDir string
}

func AbrirStore(dataDir string) (*Store, error) {
	// caminho ABSOLUTO: VACUUM INTO e logs resolvem contra CWD do processo
	// (revisão SHORYU §1.1) — nohup/systemd iniciado de outro dir não desvia backup
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
	s := &Store{db: db, dataDir: dataDir}
	if err := s.migrar(); err != nil {
		return nil, err
	}
	if err := s.migrarPresencasV2(); err != nil {
		return nil, err
	}
	if err := s.migrarV3Conferencias(); err != nil {
		return nil, err
	}
	return s, nil
}

// migrarV3Conferencias: renomeia formaturas→conferencias, formatura_tipos→conferencia_tipos
// e a coluna presencas.formatura_id→conferencia_id. Em banco NOVO (já criado com o schema
// novo) não faz nada.
func (s *Store) migrarV3Conferencias() error {
	var n int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM schema_migrations WHERE versao = 3`).Scan(&n); err != nil {
		return err
	}
	if n > 0 {
		return nil
	}
	// banco novo: presencas já tem conferencia_id → nada a fazer
	temV3 := false
	rows, err := s.db.Query(`PRAGMA table_info(presencas)`)
	if err != nil {
		return err
	}
	for rows.Next() {
		var cid int
		var nome, tipo string
		var notNull int
		var dflt any
		var pk int
		if rows.Scan(&cid, &nome, &tipo, &notNull, &dflt, &pk) == nil && nome == "conferencia_id" {
			temV3 = true
		}
	}
	rows.Close()
	if temV3 {
		_, err = s.db.Exec(`INSERT INTO schema_migrations (versao) SELECT 3
			WHERE NOT EXISTS (SELECT 1 FROM schema_migrations WHERE versao = 3)`)
		return err
	}
	// banco legado: existe tabela formaturas?
	var temLegado int
	if err = s.db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='formaturas'`).Scan(&temLegado); err != nil {
		return err
	}
	if temLegado == 0 {
		_, err = s.db.Exec(`INSERT INTO schema_migrations (versao) SELECT 3
			WHERE NOT EXISTS (SELECT 1 FROM schema_migrations WHERE versao = 3)`)
		return err
	}
	steps := []string{
		`ALTER TABLE formatura_tipos RENAME TO conferencia_tipos`,
		`ALTER TABLE formaturas RENAME TO conferencias`,
		`CREATE TABLE presencas_v3 (
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
		`INSERT INTO presencas_v3 (id, conferencia_id, pessoa_id, situacao, destino_id, tag_id,
			observacao, marcado_por, marcado_em, alterado_por, alterado_em)
		 SELECT id, formatura_id, pessoa_id, situacao, destino_id, tag_id,
			observacao, marcado_por, marcado_em, alterado_por, alterado_em FROM presencas`,
		`DROP TABLE presencas`,
		`ALTER TABLE presencas_v3 RENAME TO presencas`,
		`CREATE INDEX IF NOT EXISTS idx_presencas_pessoa ON presencas(pessoa_id)`,
		`CREATE INDEX IF NOT EXISTS idx_presencas_conf ON presencas(conferencia_id)`,
		`CREATE INDEX IF NOT EXISTS idx_conferencias_data ON conferencias(data)`,
		`INSERT INTO schema_migrations (versao) SELECT 3
		 WHERE NOT EXISTS (SELECT 1 FROM schema_migrations WHERE versao = 3)`,
	}
	for _, q := range steps {
		if _, err = s.db.Exec(q); err != nil {
			return fmt.Errorf("migração v3 conferencias: %w", err)
		}
	}
	return nil
}

// migrarPresencasV2: ordem do Tenente (28/09) — falta NÃO exige destino.
// Rebuild da tabela presencas para relaxar o CHECK (só 'justificada' exige).
func (s *Store) migrarPresencasV2() error {
	var n int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM schema_migrations WHERE versao = 2`).Scan(&n); err != nil {
		return err
	}
	if n > 0 {
		return nil
	}
	// se presencas não tem a coluna legado formatura_id (banco novo ou já v3), só registra
	temLegado := false
	rows, err := s.db.Query(`PRAGMA table_info(presencas)`)
	if err != nil {
		return err
	}
	for rows.Next() {
		var cid int
		var nome, tipo string
		var notNull int
		var dflt any
		var pk int
		if rows.Scan(&cid, &nome, &tipo, &notNull, &dflt, &pk) == nil && nome == "formatura_id" {
			temLegado = true
		}
	}
	rows.Close()
	if !temLegado {
		_, err = s.db.Exec(`INSERT INTO schema_migrations (versao) SELECT 2
			WHERE NOT EXISTS (SELECT 1 FROM schema_migrations WHERE versao = 2)`)
		return err
	}
	steps := []string{
		`CREATE TABLE presencas_v2 (
			id INTEGER PRIMARY KEY,
			formatura_id INTEGER NOT NULL REFERENCES formaturas(id) ON DELETE CASCADE,
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
			UNIQUE (formatura_id, pessoa_id)
		)`,
		`INSERT INTO presencas_v2 (id, formatura_id, pessoa_id, situacao, destino_id, tag_id,
			observacao, marcado_por, marcado_em, alterado_por, alterado_em)
		 SELECT id, formatura_id, pessoa_id, situacao, destino_id, tag_id,
			observacao, marcado_por, marcado_em, alterado_por, alterado_em FROM presencas`,
		`DROP TABLE presencas`,
		`ALTER TABLE presencas_v2 RENAME TO presencas`,
		`CREATE INDEX IF NOT EXISTS idx_presencas_pessoa ON presencas(pessoa_id)`,
		`INSERT INTO schema_migrations (versao) SELECT 2 WHERE NOT EXISTS (SELECT 1 FROM schema_migrations WHERE versao = 2)`,
	}
	for _, q := range steps {
		if _, err := s.db.Exec(q); err != nil {
			return fmt.Errorf("migração v2 presencas: %w", err)
		}
	}
	return nil
}

func (s *Store) Close() error { return s.db.Close() }

func (s *Store) migrar() error {
	ddl := []string{
		`CREATE TABLE IF NOT EXISTS schema_migrations (
			versao INTEGER PRIMARY KEY,
			aplicada_em TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
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
			status TEXT NOT NULL DEFAULT 'ativo' CHECK (status IN ('ativo','inativo','movido')),
			criado_em TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
			atualizado_em TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
		)`,
		`CREATE INDEX IF NOT EXISTS idx_pessoas_setor ON pessoas(setor_id)`,
		`CREATE INDEX IF NOT EXISTS idx_pessoas_status ON pessoas(status)`,
		`CREATE TABLE IF NOT EXISTS usuarios (
			id INTEGER PRIMARY KEY,
			login TEXT NOT NULL UNIQUE COLLATE NOCASE,
			senha_hash TEXT NOT NULL,
			papel TEXT NOT NULL CHECK (papel IN ('admin','usuario')),
			pessoa_id INTEGER UNIQUE REFERENCES pessoas(id),
			ativo INTEGER NOT NULL DEFAULT 1 CHECK (ativo IN (0,1)),
			criado_em TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
			ultimo_login TEXT
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
		// (revisão TAKEDA: UNIQUE já indexa o prefixo conferencia — índice extra dispensável, mas barato e explícito)
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
		`INSERT OR IGNORE INTO setores (nome, sigla) VALUES
			('Comando','Cmdo'),('Escola','Esco'),('Serviços','Svc'),('Manutenção','Manut'),('Telemática','TLM')`,
		`INSERT OR IGNORE INTO funcoes (nome) VALUES
			('Operador'),('Motorista'),('Mecânico'),('Bombeiro'),('Auxiliar Adm')`,
		`INSERT OR IGNORE INTO destinos (nome) VALUES
			('Serviço'),('Curso'),('Hospital'),('Licença'),('Trânsito'),('CMA')`,
		`INSERT OR IGNORE INTO tags (nome, cor) VALUES
			('Sd','#8ea9c1'),('Cb','#7fb069'),('3º Sgt','#d4a24e'),('2º Sgt','#d4a24e'),
			('1º Sgt','#d4a24e'),('Asp','#9b7fb0'),('Of','#c14e4e')`,
		`INSERT OR IGNORE INTO conferencia_tipos (nome) VALUES
			('Conferência de pessoal'),('Café Coletivo'),('Instrução'),('Inspeção')`,
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
	if _, err = rand.Read(b); err != nil {
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
		`SELECT u.id, u.login, u.papel, u.pessoa_id
		 FROM sessoes se JOIN usuarios u ON u.id = se.usuario_id
		 WHERE se.token_hash = ? AND se.expira_em > ? AND u.ativo = 1`,
		hash, time.Now().UTC().Format(time.RFC3339)).
		Scan(&u.ID, &u.Login, &u.Papel, &u.PessoaID)
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
	"tags": true, "conferencia_tipos": true,
}

func tabelaDeCatalogo(tab string) (string, error) {
	if catalogosValidos[tab] {
		return tab, nil
	}
	return "", fmt.Errorf("catálogo inválido: %s", tab)
}
