package main

import (
	"bytes"
	"encoding/base64"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// -----------------------------------------------------------------------------
// Testes de Regressão Milestone M1 (R1: Os 3 P0s de Segurança e Integridade)
// Item A: Fechamento de escopo -1 (contas sem grupo) em todas as rotas de dados
// Item B: Blindagem de foto_base64 contra XSS armazenado
// Item C: Swap atômico de backup para sci.db, migrações unificadas e rollback seguro
// -----------------------------------------------------------------------------

func TestR1_ScopeClosure_FiltroGrupoSQL(t *testing.T) {
	// 1. Escopo -1 (não-admin sem grupo): deve retornar " AND 1 = 0" (fail-closed)
	if got := filtroGrupoSQL(-1, "t"); got != " AND 1 = 0" {
		t.Errorf("filtroGrupoSQL(-1, 't') = %q, esperado ' AND 1 = 0'", got)
	}
	if got := filtroGrupoSQL(-1, ""); got != " AND 1 = 0" {
		t.Errorf("filtroGrupoSQL(-1, '') = %q, esperado ' AND 1 = 0'", got)
	}

	// 2. Escopo 0 (admin global): deve retornar string vazia ""
	if got := filtroGrupoSQL(0, "t"); got != "" {
		t.Errorf("filtroGrupoSQL(0, 't') = %q, esperado ''", got)
	}
	if got := filtroGrupoSQL(0, ""); got != "" {
		t.Errorf("filtroGrupoSQL(0, '') = %q, esperado ''", got)
	}

	// 3. Escopo > 0: deve filtrar com alias e placeholder
	if got := filtroGrupoSQL(5, "t"); got != " AND t.grupo_id = ?" {
		t.Errorf("filtroGrupoSQL(5, 't') = %q, esperado ' AND t.grupo_id = ?'", got)
	}
	if got := filtroGrupoSQL(5, ""); got != " AND grupo_id = ?" {
		t.Errorf("filtroGrupoSQL(5, '') = %q, esperado ' AND grupo_id = ?'", got)
	}
}

func TestR1_ScopeClosure_Matrix(t *testing.T) {
	app, st, cleanup := setupTestApp(t)
	defer cleanup()

	// Cria grupo e conta operacional para contraste
	var gid int64
	if err := st.db.QueryRow(`INSERT INTO grupos (nome) VALUES ('Grupo Base R1') RETURNING id`).Scan(&gid); err != nil {
		t.Fatalf("criar grupo: %v", err)
	}

	// Cria conta NÃO-ADMIN SEM GRUPO (escopo == -1)
	criaUsuarioTeste(t, st, "sem_grupo_op", "senha123", "operador")
	ckSemGrupo := loginAs(t, app, "sem_grupo_op", "senha123")

	// Cria conta GERENTE SEM GRUPO (escopo == -1)
	criaUsuarioTeste(t, st, "sem_grupo_ger", "senha123", "gerente")
	ckGerSemGrupo := loginAs(t, app, "sem_grupo_ger", "senha123")

	rotasOperador := []struct {
		metodo string
		path   string
		corpo  map[string]any
	}{
		{"GET", "/api/conferencia/hoje", nil},
		{"GET", "/api/conferencias", nil},
		{"GET", "/api/conferencia/1/relatorio.pdf", nil},
		{"GET", "/api/pessoas", nil},
		{"GET", "/api/material/itens", nil},
		{"GET", "/api/material/cautelas", nil},
		{"GET", "/api/material/inventario/pdf", nil},
		{"GET", "/api/escalas/hoje", nil},
		{"GET", "/api/escalas/turnos", nil},
		{"GET", "/api/escalas/tipos", nil},
		{"GET", "/api/efetivo_atual", nil},
		{"GET", "/api/presenca/periodo", nil},
		{"GET", "/api/setores/sugestoes", nil},
		{"GET", "/api/avisos", nil},
		{"GET", "/api/notificacoes", nil},
		{"GET", "/api/grupos", nil},
		{"POST", "/api/comentarios", map[string]any{"conferencia_id": 1, "comentario": "teste"}},
		{"POST", "/api/conferencia/marcar", map[string]any{"pessoa_id": 1, "situacao": "presente"}},
	}

	for _, r := range rotasOperador {
		rr, res := doJSONReq(app, r.metodo, r.path, r.corpo, ckSemGrupo)
		if rr.Code != http.StatusForbidden {
			t.Errorf("Rota %s %s com conta sem grupo deveria retornar 403 Forbidden, recebeu %d (res=%v)",
				r.metodo, r.path, rr.Code, res)
		}
	}

	rotasGerente := []struct {
		metodo string
		path   string
		corpo  map[string]any
	}{
		{"POST", "/api/conferencia/fechar", map[string]any{"id": 1, "lancamentos": []any{}}},
		{"POST", "/api/escalas/limpar-dia", map[string]any{"data": "2026-10-10"}},
		{"GET", "/api/grupo/funcoes/membros", nil},
	}

	for _, r := range rotasGerente {
		rr, res := doJSONReq(app, r.metodo, r.path, r.corpo, ckGerSemGrupo)
		if rr.Code != http.StatusForbidden {
			t.Errorf("Rota Gerente %s %s com conta sem grupo deveria retornar 403 Forbidden, recebeu %d (res=%v)",
				r.metodo, r.path, rr.Code, res)
		}
	}
}

func TestR1_ScopeClosure_AdminAndNormalGroupAccess(t *testing.T) {
	app, st, cleanup := setupTestApp(t)
	defer cleanup()

	// 1. Admin (escopo == 0) pode acessar rotas globais sem bloqueio de escopo
	admin := loginAs(t, app, "admin", "admin123")
	rrAdm, _ := doJSONReq(app, "GET", "/api/conferencias", nil, admin)
	if rrAdm.Code != http.StatusOK {
		t.Errorf("Admin em /api/conferencias esperado 200, veio %d", rrAdm.Code)
	}

	// 2. Operador COM grupo (escopo > 0) tem acesso liberado
	var gid int64
	if err := st.db.QueryRow(`INSERT INTO grupos (nome) VALUES ('Grupo Op Normal') RETURNING id`).Scan(&gid); err != nil {
		t.Fatalf("criar grupo: %v", err)
	}
	criaUsuarioTeste(t, st, "op_normal", "senha123", "operador")
	if _, err := st.db.Exec(`UPDATE usuarios SET grupo_id = ? WHERE login = 'op_normal'`, gid); err != nil {
		t.Fatalf("vincular grupo: %v", err)
	}
	if _, err := st.db.Exec(`INSERT INTO usuario_papeis (usuario_id, grupo_id, papel) VALUES ((SELECT id FROM usuarios WHERE login = 'op_normal'), ?, 'operador')`, gid); err != nil {
		t.Fatalf("vincular papel: %v", err)
	}

	ckOp := loginAs(t, app, "op_normal", "senha123")
	rrOp, _ := doJSONReq(app, "GET", "/api/pessoas", nil, ckOp)
	if rrOp.Code != http.StatusOK {
		t.Errorf("Operador com grupo em /api/pessoas esperado 200, veio %d", rrOp.Code)
	}
}

func TestR1_FotoBase64_Validation(t *testing.T) {
	app, _, cleanup := setupTestApp(t)
	defer cleanup()

	admin := loginAs(t, app, "admin", "admin123")

	// 1. Imagem PNG válida mínima (1x1 transparente)
	pngBytes := []byte{
		0x89, 0x50, 0x4E, 0x4E, // corrupt: not 0x47
	}
	_ = pngBytes

	realPNG := "data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mNk+M9QDwADhgGAWjR9awAAAABJRU5ErkJggg=="
	realJPEG := "data:image/jpeg;base64,/9j/4AAQSkZJRgABAQEASABIAAD/2wBDAP//////////////////////////////////////////////////////////////////////////////////////wgALCAABAAEBAREA/8QAFBABAAAAAAAAAAAAAAAAAAAAAP/aAAgBAQABPxA="
	realWebP := "data:image/webp;base64,UklGRhoAAABXRUJQVlA4TA0AAAAvAAAAEAcQERGIiP4HAA=="

	// Teste A: PNG válido no perfil deve ser aceito (200 OK)
	rr, res := doJSONReq(app, "PATCH", "/api/perfil", map[string]any{
		"nome_guerra":   "ADMIN",
		"nome_completo": "Administrador do Sistema",
		"foto_base64":   realPNG,
	}, admin)
	if rr.Code != http.StatusOK {
		t.Fatalf("PNG válido em /api/perfil rejeitado: status=%d res=%v", rr.Code, res)
	}

	// Teste B: JPEG válido deve ser aceito
	rr, res = doJSONReq(app, "PATCH", "/api/perfil", map[string]any{
		"nome_guerra":   "ADMIN",
		"nome_completo": "Administrador do Sistema",
		"foto_base64":   realJPEG,
	}, admin)
	if rr.Code != http.StatusOK {
		t.Fatalf("JPEG válido em /api/perfil rejeitado: status=%d res=%v", rr.Code, res)
	}

	// Teste C: WebP válido deve ser aceito
	rr, res = doJSONReq(app, "PATCH", "/api/perfil", map[string]any{
		"nome_guerra":   "ADMIN",
		"nome_completo": "Administrador do Sistema",
		"foto_base64":   realWebP,
	}, admin)
	if rr.Code != http.StatusOK {
		t.Fatalf("WebP válido em /api/perfil rejeitado: status=%d res=%v", rr.Code, res)
	}

	// Teste D: Payload XSS com data:text/html deve ser rejeitado (400 Bad Request)
	xssHTML := "data:text/html;base64,PHNjcmlwdD5hbGVydCgxKTwvc2NyaXB0Pg=="
	rr, _ = doJSONReq(app, "PATCH", "/api/perfil", map[string]any{
		"nome_guerra":   "ADMIN",
		"nome_completo": "Administrador do Sistema",
		"foto_base64":   xssHTML,
	}, admin)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("XSS data:text/html deveria ser rejeitado com 400, veio %d", rr.Code)
	}

	// Teste E: Payload XSS com SVG onload deve ser rejeitado (400 Bad Request)
	xssSVG := "data:image/svg+xml;base64,PHN2ZyBvbmxvYWQ9YWxlcnQoMSk+"
	rr, _ = doJSONReq(app, "PATCH", "/api/perfil", map[string]any{
		"nome_guerra":   "ADMIN",
		"nome_completo": "Administrador do Sistema",
		"foto_base64":   xssSVG,
	}, admin)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("XSS SVG deveria ser rejeitado com 400, veio %d", rr.Code)
	}

	// Teste F: Payload javascript: deve ser rejeitado (400 Bad Request)
	rr, _ = doJSONReq(app, "PATCH", "/api/perfil", map[string]any{
		"nome_guerra":   "ADMIN",
		"nome_completo": "Administrador do Sistema",
		"foto_base64":   "javascript:alert(1)",
	}, admin)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("URI javascript: deveria ser rejeitada com 400, veio %d", rr.Code)
	}

	// Teste G: Magic bytes falsos (prefixo PNG com conteúdo não-PNG) -> 400
	fakePNG := "data:image/png;base64," + base64.StdEncoding.EncodeToString([]byte("NAO E PNG MAS SIM TEXTO"))
	rr, _ = doJSONReq(app, "PATCH", "/api/perfil", map[string]any{
		"nome_guerra":   "ADMIN",
		"nome_completo": "Administrador do Sistema",
		"foto_base64":   fakePNG,
	}, admin)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("PNG com magic bytes inválidos deveria ser rejeitado com 400, veio %d", rr.Code)
	}

	// Teste H: Base64 corrompido -> 400
	badB64 := "data:image/png;base64,!!!NAO_BASE64!!!"
	rr, _ = doJSONReq(app, "PATCH", "/api/perfil", map[string]any{
		"nome_guerra":   "ADMIN",
		"nome_completo": "Administrador do Sistema",
		"foto_base64":   badB64,
	}, admin)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("Base64 malformado deveria ser rejeitado com 400, veio %d", rr.Code)
	}

	// Teste I: Tamanho maior que 512 KB -> 400 Bad Request
	bigPayload := "data:image/png;base64," + strings.Repeat("A", 513*1024)
	rr, _ = doJSONReq(app, "PATCH", "/api/perfil", map[string]any{
		"nome_guerra":   "ADMIN",
		"nome_completo": "Administrador do Sistema",
		"foto_base64":   bigPayload,
	}, admin)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("Foto > 512 KB deveria ser rejeitada com 400, veio %d", rr.Code)
	}
}

func TestR1_BackupRestore_AtomicAndRollback(t *testing.T) {
	app, st, cleanup := setupTestApp(t)
	defer cleanup()

	admin := loginAs(t, app, "admin", "admin123")

	// 1. Gera backup íntegro do sistema
	rr, res := doJSONReq(app, "POST", "/api/backup", nil, admin)
	if rr.Code != http.StatusOK {
		t.Fatalf("POST /api/backup: %d (%v)", rr.Code, res)
	}
	arqNome := res["arquivo"].(string)

	pathDown := "/api/backup/download?nome=" + filepath.Base(arqNome)
	rrDown := doRawReqH(app, http.MethodGet, pathDown, nil, "", admin)
	if rrDown.Code != http.StatusOK {
		t.Fatalf("download backup: %d", rrDown.Code)
	}
	backupBytes := rrDown.Body.Bytes()

	// 2. Modifica o banco atual inserindo dado para verificar restauração
	if _, err := st.db.Exec(`INSERT INTO grupos (nome) VALUES ('Grupo Temporario Pre-Restore')`); err != nil {
		t.Fatalf("inserir grupo temporario: %v", err)
	}

	// 3. Importa o backup gerado
	rrImp := doMultipartCampo(t, app, "/api/backup/importar", "arquivo", filepath.Base(arqNome), backupBytes, admin)
	if rrImp.Code != http.StatusOK {
		t.Fatalf("importar backup: %d (%s)", rrImp.Code, rrImp.Body.String())
	}
	resImp := jsonDe(t, rrImp)
	if resImp["ok"] != true {
		t.Fatalf("importar ok != true: %v", resImp)
	}

	// 4. Verifica que sci.db é o arquivo canônico (NUNCA .novo)
	canonicalDB := filepath.Join(st.dataDir, "sci.db")
	novoDB := canonicalDB + ".novo"
	if _, err := os.Stat(canonicalDB); os.IsNotExist(err) {
		t.Errorf("arquivo sci.db canônico não existe após restauração")
	}
	if _, err := os.Stat(novoDB); err == nil {
		t.Errorf("arquivo sci.db.novo ainda existe após restauração (deveria ter sido removido)")
	}

	// 5. Verifica que todas as migrações até versaoSchemaBinario (43) estão registradas
	var maxVersao int
	if err := st.db.QueryRow(`SELECT COALESCE(MAX(versao), 0) FROM schema_migrations`).Scan(&maxVersao); err != nil {
		t.Fatalf("consultar schema_migrations: %v", err)
	}
	if maxVersao != versaoSchemaBinario {
		t.Errorf("maxVersao após restauração = %d, esperado %d", maxVersao, versaoSchemaBinario)
	}

	// 6. Confirma que o dado adicionado depois do backup foi descartado
	var cntTemp int
	_ = st.db.QueryRow(`SELECT COUNT(*) FROM grupos WHERE nome = 'Grupo Temporario Pre-Restore'`).Scan(&cntTemp)
	if cntTemp != 0 {
		t.Errorf("dado criado pós-backup sobreviveu ao restore (cnt=%d)", cntTemp)
	}

	// 7. Teste de ROLLBACK: tentar importar arquivo SQLite corrompido
	// Cria arquivo com cabeçalho SQLite válido mas corpo truncado/corrompido
	corruptBytes := append([]byte("SQLite format 3\x00"), bytes.Repeat([]byte{0xFF}, 1024)...)
	rrCorrupt := doMultipartCampo(t, app, "/api/backup/importar", "arquivo", "corrupt.db", corruptBytes, admin)
	if rrCorrupt.Code != http.StatusBadRequest {
		t.Errorf("importar banco corrompido deveria retornar 400 Bad Request, veio %d (%s)",
			rrCorrupt.Code, rrCorrupt.Body.String())
	}

	// Garante que o banco canônico sci.db continua vivo e operacional após a falha
	var cntVivo int
	if err := st.db.QueryRow(`SELECT COUNT(*) FROM usuarios WHERE login = 'admin'`).Scan(&cntVivo); err != nil || cntVivo == 0 {
		t.Fatalf("banco sci.db ficou inacessível ou corrompido após tentativa de importação falha: %v", err)
	}
}
