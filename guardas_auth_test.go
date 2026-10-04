package main

// guardas_auth_test.go — F2 (frente F, P0): tabela rota×cenário para os guardas
// de auth (auth.go): 401 sem sessão, 403 sem X-SCI, 403 Origin estranha,
// 403 papel errado, passagem livre com tudo certo.
// Lacunas mapeadas na F1: zero testes de 401/auth; guardas X-SCI/Origin sem suíte.
// Ordem real do middleware (auth.go): Origin → X-SCI → sessão → precisa_setup → papel.

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// cenarioGuarda: uma quebra proposital de um requisito do guarda (modelada em
// campos, não em mutação de cookie — a ordem cookie×mutação já queimou um
// rascunho: cookie válido entrava no cenário "sem cookie" e o 401 nunca vinha).
type cenarioGuarda struct {
	nome           string
	semCookie      bool
	cookieValor    string                // se != "", substitui o cookie da credencial
	mutacao        func(r *http.Request) // mexe só em headers
	querStatus     int                   // -1 = guarda passou (ver negócío da rota)
	querBodyContem string                // substring esperada no corpo quando status >= 400
}

// passagemGuarda: o guarda deixou passar — resposta 2xx (ok) ou 4xx de NEGÓCIO
// (rota existe, handler decidiu). 401/403 aqui significaria guarda falhando.
func passagemGuarda(code int) bool {
	if code >= 200 && code < 300 {
		return true
	}
	return code >= 400 && code < 500 && code != http.StatusUnauthorized && code != http.StatusForbidden
}

// rodarCenarios executa a matriz cenário×papel para uma rota.
// rotaAdmin=true: com passagem de guarda, operador deve tomar 403 do papel.
// Host fixado para o teste de Origin ter dente: "origem estranha" não contém
// o host; a própria origem contém.
func rodarCenarios(t *testing.T, app *App, st *Store, metodo, rota string, rotaAdmin bool) {
	t.Helper()

	admin := loginAs(t, app, "admin", "admin123")
	oper := loginAsPapel(t, app, st, "oper_guarda", "operador")

	cenarios := []cenarioGuarda{
		{
			nome:           "sem cookie → 401",
			semCookie:      true,
			querStatus:     http.StatusUnauthorized,
			querBodyContem: "não autenticado",
		},
		{
			nome:           "cookie inválido → 401",
			cookieValor:    "token-falso",
			querStatus:     http.StatusUnauthorized,
			querBodyContem: "sessão expirada",
		},
		{
			nome:           "sem X-SCI → 403",
			mutacao:        func(r *http.Request) { r.Header.Del("X-SCI") },
			querStatus:     http.StatusForbidden,
			querBodyContem: "cabeçalho ausente",
		},
		{
			nome:           "X-SCI errado → 403",
			mutacao:        func(r *http.Request) { r.Header.Set("X-SCI", "sim") },
			querStatus:     http.StatusForbidden,
			querBodyContem: "cabeçalho ausente",
		},
		{
			nome:           "Origin estranha → 403",
			mutacao:        func(r *http.Request) { r.Header.Set("Origin", "https://site-malicioso.exemplo") },
			querStatus:     http.StatusForbidden,
			querBodyContem: "origem estranha",
		},
		{
			nome: "Origin da própria origem → guarda passa",
			mutacao: func(r *http.Request) {
				r.Header.Set("Origin", "http://"+r.Host)
			},
			querStatus: -1,
		},
	}

	for _, c := range cenarios {
		for _, cred := range []struct {
			papel  string
			cookie *http.Cookie
		}{
			{"admin", admin},
			{"operador", oper},
		} {
			req, _ := http.NewRequest(metodo, rota, nil)
			req.Host = "sci.teste.local"
			req.Header.Set("X-SCI", "1")
			if c.mutacao != nil {
				c.mutacao(req)
			}
			// cookie por último e de forma exclusiva: o cenário decide qual
			// cookie (ou nenhum) segue na request.
			switch {
			case c.semCookie:
			case c.cookieValor != "":
				req.AddCookie(&http.Cookie{Name: cookieSessao, Value: c.cookieValor})
			case cred.cookie != nil:
				req.AddCookie(cred.cookie)
			}

			rr := httptest.NewRecorder()
			app.mux.ServeHTTP(rr, req)

			// resolução do esperado
			if c.querStatus == -1 {
				// guarda passou: o que vale é NÃO ser 401/403 de guarda
				if rotaAdmin && cred.papel == "operador" {
					if rr.Code != http.StatusForbidden {
						t.Errorf("[%s %s | %s | %s] status=%d, quer=403 (restrito ao admin), corpo=%s",
							metodo, rota, cred.papel, c.nome, rr.Code, rr.Body.String())
					}
					continue
				}
				if !passagemGuarda(rr.Code) {
					t.Errorf("[%s %s | %s | %s] guarda deveria passar, status=%d, corpo=%s",
						metodo, rota, cred.papel, c.nome, rr.Code, rr.Body.String())
				}
				continue
			}
			if rr.Code != c.querStatus {
				t.Errorf("[%s %s | %s | %s] status=%d, quer=%d, corpo=%s",
					metodo, rota, cred.papel, c.nome, rr.Code, c.querStatus, rr.Body.String())
				continue
			}
			if c.querBodyContem != "" && rr.Code >= 400 && !strings.Contains(rr.Body.String(), c.querBodyContem) {
				t.Errorf("[%s %s | %s | %s] corpo=%q não contém %q",
					metodo, rota, cred.papel, c.nome, rr.Body.String(), c.querBodyContem)
			}
		}
	}
}

// TestGuardasMatrizAdmin: rota admin-only contra toda a matriz de cenários.
// Última célula (guarda passa × admin) cria um backup real — em diretório de teste.
func TestGuardasMatrizAdmin(t *testing.T) {
	app, st, cleanup := setupTestApp(t)
	defer cleanup()
	rodarCenarios(t, app, st, "POST", "/api/backup", true)
}

// TestGuardasMatrizComum: rota autenticada comum contra a mesma matriz
// (/api/logout é barata e aceita qualquer papel; última célula mata a própria
// sessão do admin — depois dela ninguém mais usa esse cookie).
func TestGuardasMatrizComum(t *testing.T) {
	app, st, cleanup := setupTestApp(t)
	defer cleanup()
	rodarCenarios(t, app, st, "POST", "/api/logout", false)
}

// TestGuardas401MetodosInseguros: rota protegida sem cookie → 401 sempre,
// mesmo com X-SCI presente (Origin/X-SCI passam; a sessão é que falta).
func TestGuardas401MetodosInseguros(t *testing.T) {
	app, _, cleanup := setupTestApp(t)
	defer cleanup()

	rotas := []struct{ metodo, rota string }{
		{"POST", "/api/backup"},
		{"POST", "/api/logout"},
		{"POST", "/api/senha"},
		{"POST", "/api/pessoas"},
		{"PATCH", "/api/pessoas/999"},
		{"DELETE", "/api/pessoas/999"},
		{"GET", "/api/me"}, // método seguro: guarda é só de sessão
	}

	for _, rt := range rotas {
		req, _ := http.NewRequest(rt.metodo, rt.rota, nil)
		req.Header.Set("X-SCI", "1")
		rr := httptest.NewRecorder()
		app.mux.ServeHTTP(rr, req)
		if rr.Code != http.StatusUnauthorized {
			t.Errorf("[%s %s] sem cookie: status=%d, quer=401, corpo=%s",
				rt.metodo, rt.rota, rr.Code, rr.Body.String())
		}
	}
}

// TestGuardasAdminSomente: operador autenticado, X-SCI e sem Origin (curl local)
// em rota admin-only → 403 "restrito ao admin", nunca 404/500.
func TestGuardasAdminSomente(t *testing.T) {
	app, st, cleanup := setupTestApp(t)
	defer cleanup()

	oper := loginAsPapel(t, app, st, "oper_admin", "operador")

	for _, rt := range []struct{ metodo, rota string }{
		{"POST", "/api/backup"},
		{"GET", "/api/backup/download?nome=x.db"}, // rota GET-only: 405 do mux viria antes do guarda
	} {
		rr := doRawReqH(app, rt.metodo, rt.rota, nil, "", oper)
		if rr.Code != http.StatusForbidden || !strings.Contains(rr.Body.String(), "restrito ao admin") {
			t.Errorf("[%s] operador: status=%d, corpo=%s; quer 403 'restrito ao admin'",
				rt, rr.Code, rr.Body.String())
		}
	}
	// importar: forma multipart, campo 'arquivo'
	rr := doMultipartCampo(t, app, "/api/backup/importar", "arquivo", "x.db", []byte("lixo"), oper)
	if rr.Code != http.StatusForbidden || !strings.Contains(rr.Body.String(), "restrito ao admin") {
		t.Errorf("[/api/backup/importar] operador: status=%d, corpo=%s; quer 403 'restrito ao admin'",
			rr.Code, rr.Body.String())
	}
}

// TestGuardasGETSemXSCI: método seguro não exige X-SCI/Origin — só sessão.
func TestGuardasGETSemXSCI(t *testing.T) {
	app, _, cleanup := setupTestApp(t)
	defer cleanup()

	admin := loginAs(t, app, "admin", "admin123")
	req, _ := http.NewRequest("GET", "/api/me", nil)
	req.AddCookie(admin)
	rr := httptest.NewRecorder()
	app.mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("GET /api/me sem X-SCI: status=%d, quer=200, corpo=%s", rr.Code, rr.Body.String())
	}
}
