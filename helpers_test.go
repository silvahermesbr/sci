package main

// helpers_test.go — F2 (frente F, P0): infra comum da suíte profissional.
// Regras da casa: teste é parte do código; evidência executada, não alegada.
// Reaproveita setupTestApp/loginAs/doJSONReq (v1_test.go); adiciona só o que a
// suíte nova precisa: usuários por papel, request cru (multipart/binário) e
// tabela rota×cenário para os guardas de auth.

import (
	"bytes"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"
)

// criaUsuarioTeste insere usuário ativo com senha e papel — via SQL direto
// (validarCredenciais lê usuarios.senha_hash/ativo/precisa_setup). SEM linha
// em usuario_papeis: a sessão sintetiza o papel NO LOGIN já com o grupo da
// conta (CriarSessaoComPapel). Linha pré-criada grupo-NULL virava a PRIMEIRA
// linha (ORDER BY id) e prendia a sessão num papel sem escopo — 403 em
// G1/G3 e efetivo vazio (regressão provada na suíte completa).
func criaUsuarioTeste(t *testing.T, st *Store, login, senha, papel string) {
	t.Helper()
	hash, err := hashSenha(senha)
	if err != nil {
		t.Fatalf("hashSenha: %v", err)
	}
	if _, err := st.db.Exec(`INSERT INTO usuarios (login, senha_hash, papel, precisa_setup) VALUES (?, ?, ?, 0)`, login, hash, papel); err != nil {
		t.Fatalf("criar usuário %s: %v", login, err)
	}
}

// loginAsPapel: cria usuário do papel e loga — atalho para os testes de guarda.
// Senha opcional (ordem Diretor 04/10): sem senha usa o padrão "senha-<papel>".
func loginAsPapel(t *testing.T, app *App, st *Store, login, papel string, senha ...string) *http.Cookie {
	t.Helper()
	s := "senha-" + papel
	if len(senha) > 0 && senha[0] != "" {
		s = senha[0]
	}
	criaUsuarioTeste(t, st, login, s, papel)
	return loginAs(t, app, login, s)
}

// materializaComandoSetor: grava a linha de COMANDO em chefe_setores — a fonte
// ÚNICA de autorização de chefia desde a v1.5.4-D1 (R-12, chefe-zumbi; decisão
// D-2). Setups que criavam chefe via SQL direto (papel chefe_setor +
// usuarios.setor_id, SEM a linha) dependiam dos fallbacks removidos do produto
// — aqui o teste semeia o estado na doutrina NOVA, nunca re-adicionando
// fallback no produto. (Setups que nomeiam via API já materializam sozinhos.)
func materializaComandoSetor(t *testing.T, st *Store, usuarioID, grupoID, setorID int64) {
	t.Helper()
	if _, err := st.db.Exec(`INSERT OR IGNORE INTO chefe_setores (grupo_id, setor_id, usuario_id) VALUES (?,?,?)`,
		grupoID, setorID, usuarioID); err != nil {
		t.Fatalf("materializar comando (usuario %d, setor %d): %v", usuarioID, setorID, err)
	}
}

// doRawReq: request com corpo arbitrário (multipart, binário) e headers explícitos.
func doRawReqH(app *App, method, path string, body io.Reader, contentType string, cookie *http.Cookie) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, body)
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	req.Header.Set("X-SCI", "1")
	if cookie != nil {
		req.AddCookie(cookie)
	}
	rr := httptest.NewRecorder()
	app.mux.ServeHTTP(rr, req)
	return rr
}

// doMultipartCampo: POST multipart com um campo de arquivo — forma do importar.
func doMultipartCampo(t *testing.T, app *App, path, campo, nomeArq string, conteudo []byte, cookie *http.Cookie) *httptest.ResponseRecorder {
	t.Helper()
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	fw, err := mw.CreateFormFile(campo, nomeArq)
	if err != nil {
		t.Fatalf("CreateFormFile: %v", err)
	}
	if _, err := fw.Write(conteudo); err != nil {
		t.Fatalf("gravar multipart: %v", err)
	}
	if err := mw.Close(); err != nil {
		t.Fatalf("fechar multipart: %v", err)
	}
	return doRawReqH(app, "POST", path, &buf, mw.FormDataContentType(), cookie)
}

// jsonDe: decodifica corpo JSON do recorder; falha o teste se vier quebrado.
func jsonDe(t *testing.T, rr *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &m); err != nil {
		t.Fatalf("corpo não é JSON (%v): %q", err, rr.Body.String())
	}
	return m
}
