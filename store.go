package main

// Persistência do SCI — SQLite em arquivo, pragmas de durabilidade, schema + migrações.
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
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

type Store struct {
	db      *sql.DB
	dataDir string
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
	s := &Store{db: db, dataDir: dataDir}
	if err := s.migrar(); err != nil {
		return nil, err
	}
	if err := s.migrarV4(); err != nil {
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
			papel TEXT NOT NULL, -- admin | gerente | usuario (validação no app)
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
	// condição correta: o DDL da tabela ainda contém "UNIQUE" (banco novo já nasce sem)
	ddlConferencias := ""
	if err := s.db.QueryRow(`SELECT COALESCE(sql,'') FROM sqlite_master WHERE type='table' AND name='conferencias'`).Scan(&ddlConferencias); err != nil {
		return err
	}
	if strings.Contains(strings.ToUpper(ddlConferencias), "UNIQUE") {
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
		`INSERT OR IGNORE INTO destinos (nome) VALUES
			('Serviço'),('SSV - saindo de serviço'),('Missão externa'),('Curso'),('Hospital'),
			('Licença'),('Trânsito'),('CMA')`,
		`INSERT OR IGNORE INTO conferencia_tipos (nome) VALUES ('Conferência de pessoal')`,
	)
	for _, q := range steps {
		if _, err := s.db.Exec(q); err != nil {
			return fmt.Errorf("migração v4: %w", err)
		}
	}
	return s.marcarVersao(4)
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
			('Serviço'),('SSV - saindo de serviço'),('Missão externa'),('Curso'),('Hospital'),
			('Licença'),('Trânsito'),('CMA')`,
		`INSERT OR IGNORE INTO tags (nome, cor) VALUES
			('Sd','#8ea9c1'),('Cb','#7fb069'),('3º Sgt','#d4a24e'),('2º Sgt','#d4a24e'),
			('1º Sgt','#d4a24e'),('Asp','#9b7fb0'),('Of','#c14e4e')`,
		`INSERT OR IGNORE INTO conferencia_tipos (nome) VALUES
			('Conferência de pessoal'),('Café Coletivo'),('Instrução'),('Inspeção')`,
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
		`SELECT u.id, u.login, u.papel, u.pessoa_id, u.grupo_id
		 FROM sessoes se JOIN usuarios u ON u.id = se.usuario_id
		 WHERE se.token_hash = ? AND se.expira_em > ? AND u.ativo = 1`,
		hash, time.Now().UTC().Format(time.RFC3339)).
		Scan(&u.ID, &u.Login, &u.Papel, &u.PessoaID, &u.GrupoID)
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
