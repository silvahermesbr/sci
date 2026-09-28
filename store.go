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
	if err := s.migrarV5(); err != nil {
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
