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
// (validarCredenciais lê usuarios.senha_hash/ativo/precisa_setup).
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
func loginAsPapel(t *testing.T, app *App, st *Store, login, papel string) *http.Cookie {
	t.Helper()
	criaUsuarioTeste(t, st, login, "senha-"+papel, papel)
	return loginAs(t, app, login, "senha-"+papel)
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
