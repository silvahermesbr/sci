package main

// api_backup_test.go — F2 (frente F, P0): ciclo de vida do backup definitivo
// (server.go §backup): POST /api/backup → download → importar (R9).
// Lacuna mapeada na F1: lifecycle de backup sem NENHUM teste — risco de perda de dados.

import (
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

var reNomeBackupResposta = regexp.MustCompile(`^backups/sci_[0-9]{8}_[0-9]{6}(_[0-9]{2})?\.db$`)

// sha256Hex: digest hex de um bloco de bytes (conferência download × resposta).
func sha256Hex(b []byte) string {
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}

// TestBackupCicloCompleto: cria backup, valida artefatos em disco + MANIFEST,
// baixa e confere sha256, importa o arquivo baixado e prova a semântica de
// restauração: estado criado DEPOIS do backup desaparece; sessão anterior a ele
// sobrevive; sessão posterior morre.
func TestBackupCicloCompleto(t *testing.T) {
	app, st, cleanup := setupTestApp(t)
	defer cleanup()

	admin := loginAs(t, app, "admin", "admin123")

	// 1) POST /api/backup → 200 + arquivo + sha256
	rr, res := doJSONReq(app, "POST", "/api/backup", nil, admin)
	if rr.Code != http.StatusOK {
		t.Fatalf("POST /api/backup: status=%d, corpo=%s", rr.Code, rr.Body.String())
	}
	arquivo, _ := res["arquivo"].(string)
	shaResp, _ := res["sha256"].(string)
	if !reNomeBackupResposta.MatchString(arquivo) {
		t.Fatalf("nome de arquivo inesperado: %q", arquivo)
	}
	if len(shaResp) != 64 {
		t.Fatalf("sha256 inesperado: %q", shaResp)
	}

	// 2) artefatos em disco: .db, .sha256, linha no MANIFEST
	caminhoDB := filepath.Join(st.dataDir, filepath.FromSlash(arquivo))
	if _, err := os.Stat(caminhoDB); err != nil {
		t.Fatalf("arquivo de backup não existe: %v", err)
	}
	sidecar, err := os.ReadFile(caminhoDB + ".sha256")
	if err != nil {
		t.Fatalf("sidecar .sha256 ausente: %v", err)
	}
	if !strings.HasPrefix(string(sidecar), shaResp+"  ") {
		t.Errorf("sidecar .sha256 (%q) diverge do sha reportado (%q)", strings.TrimSpace(string(sidecar)), shaResp)
	}
	manifest, err := os.ReadFile(filepath.Join(st.dataDir, "backups", "MANIFEST.txt"))
	if err != nil || !strings.Contains(string(manifest), filepath.Base(arquivo)) {
		t.Errorf("MANIFEST.txt sem entrada do backup %q (err=%v)", arquivo, err)
	}

	// 3) download → bytes conferem com o sha256 reportado
	pathDown := "/api/backup/download?nome=" + url.QueryEscape(filepath.Base(arquivo))
	rrDown := doRawReqH(app, http.MethodGet, pathDown, nil, "", admin)
	if rrDown.Code != http.StatusOK {
		t.Fatalf("download: status=%d, corpo=%s", rrDown.Code, rrDown.Body.String())
	}
	if got := sha256Hex(rrDown.Body.Bytes()); got != shaResp {
		t.Errorf("sha do download (%s) diverge do reportado (%s)", got, shaResp)
	}
	if cd := rrDown.Header().Get("Content-Disposition"); !strings.Contains(cd, filepath.Base(arquivo)) {
		t.Errorf("Content-Disposition sem o nome do arquivo: %q", cd)
	}

	// 4) estado criado DEPOIS do backup: usuário fantasma + sua sessão
	criaUsuarioTeste(t, st, "fantasma", "senha-fantasma", "operador")
	fantasma := loginAs(t, app, "fantasma", "senha-fantasma")

	// 5) importar o MESMO arquivo baixado
	rrImp := doMultipartCampo(t, app, "/api/backup/importar", "arquivo", filepath.Base(arquivo), rrDown.Body.Bytes(), admin)
	if rrImp.Code != http.StatusOK {
		t.Fatalf("importar: status=%d, corpo=%s", rrImp.Code, rrImp.Body.String())
	}
	resImp := jsonDe(t, rrImp)
	if resImp["ok"] != true {
		t.Errorf("importar: ok != true: %v", resImp)
	}
	if s, _ := resImp["schema"].(float64); int(s) != versaoSchemaBinario {
		t.Errorf("importar: schema=%v, quer=%d", resImp["schema"], versaoSchemaBinario)
	}
	if seg, _ := resImp["seguranca"].(string); seg == "" {
		t.Errorf("importar: backup de segurança não reportado: %v", resImp)
	}

	// 6) semântica de restauração
	var n int
	if err := st.db.QueryRow(`SELECT COUNT(*) FROM usuarios WHERE login = 'fantasma'`).Scan(&n); err != nil {
		t.Fatalf("contar fantasma pós-swap: %v", err)
	}
	if n != 0 {
		t.Errorf("usuário criado pós-backup sobreviveu ao import (n=%d) — restauração não veio do arquivo", n)
	}
	rrGhost := doRawReqH(app, "GET", "/api/me", nil, "", fantasma)
	if rrGhost.Code != http.StatusUnauthorized {
		t.Errorf("sessão pós-backup sobreviveu ao swap: status=%d, quer=401", rrGhost.Code)
	}
	rrMe := doRawReqH(app, "GET", "/api/me", nil, "", admin)
	if rrMe.Code != http.StatusOK {
		t.Errorf("sessão anterior ao backup deveria sobreviver (estava no arquivo): status=%d, corpo=%s",
			rrMe.Code, rrMe.Body.String())
	}
}

// TestBackupImportarRejeicoes: importação só entra se o arquivo passar nas
// três validações (magic SQLite, integrity_check, schema <= binário) — e qualquer
// rejeição NÃO pode tocar o banco vivo.
func TestBackupImportarRejeicoes(t *testing.T) {
	app, st, cleanup := setupTestApp(t)
	defer cleanup()

	admin := loginAs(t, app, "admin", "admin123")

	// a) lixo não-SQLite
	rrLixo := doMultipartCampo(t, app, "/api/backup/importar", "arquivo", "lixo.db", []byte("isto não é sqlite"), admin)
	if rrLixo.Code != http.StatusBadRequest || !strings.Contains(rrLixo.Body.String(), "não é um banco SQLite") {
		t.Errorf("lixo: status=%d, corpo=%s; quer 400 'não é um banco SQLite'", rrLixo.Code, rrLixo.Body.String())
	}
	bancoVivoOK(t, app, admin)

	// cópia do banco para gerar variantes inválidas (VACUUM INTO → arquivo novo)
	copia := filepath.Join(t.TempDir(), "copia.db")
	if _, err := st.db.Exec(`VACUUM INTO ?`, copia); err != nil {
		t.Fatalf("VACUUM INTO cópia: %v", err)
	}

	// b) SQLite legítimo SEM schema_migrations
	sem := filepath.Join(t.TempDir(), "sem_migrations.db")
	copiarERemoverMigrations(t, copia, sem)
	rrSem := doMultipartCampo(t, app, "/api/backup/importar", "arquivo", "sem_migrations.db", lerArquivo(t, sem), admin)
	if rrSem.Code != http.StatusBadRequest || !strings.Contains(rrSem.Body.String(), "sem schema_migrations") {
		t.Errorf("sem migrations: status=%d, corpo=%s; quer 400 'sem schema_migrations'", rrSem.Code, rrSem.Body.String())
	}
	bancoVivoOK(t, app, admin)

	// c) schema MAIS NOVO que o binário
	novo := filepath.Join(t.TempDir(), "schema_novo.db")
	copiarArquivo(t, copia, novo)
	subirSchema(t, novo)
	rrNovo := doMultipartCampo(t, app, "/api/backup/importar", "arquivo", "schema_novo.db", lerArquivo(t, novo), admin)
	if rrNovo.Code != http.StatusBadRequest || !strings.Contains(rrNovo.Body.String(), "mais novo que o sistema") {
		t.Errorf("schema novo: status=%d, corpo=%s; quer 400 'mais novo que o sistema'", rrNovo.Code, rrNovo.Body.String())
	}
	bancoVivoOK(t, app, admin)
}

// bancoVivoOK: após cada rejeição, o banco vivo precisa continuar de pé.
func bancoVivoOK(t *testing.T, app *App, admin *http.Cookie) {
	t.Helper()
	rr := doRawReqH(app, "GET", "/api/me", nil, "", admin)
	if rr.Code != http.StatusOK {
		t.Errorf("banco vivo afetado por importação rejeitada: /api/me status=%d, corpo=%s", rr.Code, rr.Body.String())
	}
}

// copiarERemoverMigrations: abre a cópia e derruba schema_migrations.
func copiarERemoverMigrations(t *testing.T, origem, destino string) {
	t.Helper()
	copiarArquivo(t, origem, destino)
	db, err := sql.Open("sqlite", "file:"+destino+"?_pragma=foreign_keys(0)")
	if err != nil {
		t.Fatalf("abrir cópia: %v", err)
	}
	defer db.Close()
	if _, err := db.Exec(`DROP TABLE schema_migrations`); err != nil {
		t.Fatalf("drop schema_migrations na cópia: %v", err)
	}
}

// subirSchema: na cópia, cria linha de versão futura (> binário).
func subirSchema(t *testing.T, caminho string) {
	t.Helper()
	db, err := sql.Open("sqlite", "file:"+caminho+"?_pragma=foreign_keys(0)")
	if err != nil {
		t.Fatalf("abrir cópia p/ schema: %v", err)
	}
	defer db.Close()
	// versao é PRIMARY KEY: zerar e reinserir (UPDATE em PK com múltiplas
	// linhas tenta duplicar o valor no meio do statement → erro NULL/UNIQUE)
	if _, err := db.Exec(`DELETE FROM schema_migrations`); err != nil {
		t.Fatalf("limpar schema_migrations na cópia: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO schema_migrations (versao) VALUES (?)`, versaoSchemaBinario+1); err != nil {
		t.Fatalf("subir schema na cópia: %v", err)
	}
}

func copiarArquivo(t *testing.T, origem, destino string) {
	t.Helper()
	b, err := os.ReadFile(origem)
	if err != nil {
		t.Fatalf("ler %s: %v", origem, err)
	}
	if err := os.WriteFile(destino, b, 0o600); err != nil {
		t.Fatalf("gravar %s: %v", destino, err)
	}
}

func lerArquivo(t *testing.T, caminho string) []byte {
	t.Helper()
	b, err := os.ReadFile(caminho)
	if err != nil {
		t.Fatalf("ler %s: %v", caminho, err)
	}
	return b
}

// TestBackupImportarMalformado: multipart sem campo 'arquivo' e corpo não-multipart.
func TestBackupImportarMalformado(t *testing.T) {
	app, _, cleanup := setupTestApp(t)
	defer cleanup()

	admin := loginAs(t, app, "admin", "admin123")

	// multipart válido, mas sem o campo 'arquivo'
	rr := doMultipartCampo(t, app, "/api/backup/importar", "outro_campo", "x.db", []byte("x"), admin)
	if rr.Code != http.StatusBadRequest || !strings.Contains(rr.Body.String(), "arquivo") {
		t.Errorf("sem campo: status=%d, corpo=%s; quer 400 mencionando 'arquivo'", rr.Code, rr.Body.String())
	}

	// corpo não-multipart
	rr2 := doRawReqH(app, "POST", "/api/backup/importar", strings.NewReader("corpo solto"), "application/json", admin)
	if rr2.Code != http.StatusBadRequest || !strings.Contains(rr2.Body.String(), "upload inválido") {
		t.Errorf("não multipart: status=%d, corpo=%s; quer 400 'upload inválido'", rr2.Code, rr2.Body.String())
	}
}

// TestBackupDownloadParametros: regex de nome barra path traversal e 404 de inexistente.
func TestBackupDownloadParametros(t *testing.T) {
	app, _, cleanup := setupTestApp(t)
	defer cleanup()

	admin := loginAs(t, app, "admin", "admin123")

	for _, nome := range []string{"../sci.db", "sci.db", "x", "sci_1.db"} {
		rr := doRawReqH(app, "GET", "/api/backup/download?nome="+url.QueryEscape(nome), nil, "", admin)
		if rr.Code != http.StatusBadRequest || !strings.Contains(rr.Body.String(), "nome de backup inválido") {
			t.Errorf("nome %q: status=%d, corpo=%s; quer 400 'nome de backup inválido'", nome, rr.Code, rr.Body.String())
		}
	}
	rr := doRawReqH(app, "GET", "/api/backup/download?nome=sci_99999999_999999.db", nil, "", admin)
	if rr.Code != http.StatusNotFound || !strings.Contains(rr.Body.String(), "não encontrado") {
		t.Errorf("inexistente: status=%d, corpo=%s; quer 404 'não encontrado'", rr.Code, rr.Body.String())
	}
}
