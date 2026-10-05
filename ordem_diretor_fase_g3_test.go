package main

// ordem_diretor_fase_g3_test.go — Fase G3 (ordem 04/10): o operador NÃO é
// criado — é SELECIONADO pelo chefe de setor dentre as contas do SEU setor.

import (
	"net/http"
	"strings"
	"testing"
)

func setupG3(t *testing.T) (*App, *Store, *http.Cookie, string) {
	t.Helper()
	app, st, cleanup := setupTestApp(t)
	t.Cleanup(cleanup)
	admin := loginAs(t, app, "admin", "admin123")
	gid, _ := criaGrupo(t, app, admin, "G G3 Setor", "ger_g3")

	if _, err := st.db.Exec(`INSERT OR IGNORE INTO setores (id, nome) VALUES (55, 'Setor G3')`); err != nil {
		t.Fatalf("criar setor: %v", err)
	}
	// chefe vinculado ao grupo+setor ANTES do login (sessão congela papel)
	criaUsuarioTeste(t, st, "chefe_g3", "senha12345", "chefe_setor")
	if _, err := st.db.Exec(`UPDATE usuarios SET grupo_id = ?, setor_id = 55 WHERE login = ?`, gid, "chefe_g3"); err != nil {
		t.Fatalf("vincular chefe: %v", err)
	}
	chefe := loginAs(t, app, "chefe_g3", "senha12345")
	return app, st, chefe, "chefe_g3"
}

// conta "civil" no setor aparece na lista; designação vira papel operador
// (usuario_papeis) SEM mudar o papel principal da conta.
func TestG3ListaEDesignaOperador(t *testing.T) {
	app, st, chefe, _ := setupG3(t)
	// conta do mesmo setor, papel principal "normal"
	criaUsuarioTeste(t, st, "cand_g3", "senha12345", "normal")
	if _, err := st.db.Exec(`UPDATE usuarios SET grupo_id = ?, setor_id = 55 WHERE login = ?`, 1, "cand_g3"); err != nil {
		t.Fatalf("vincular candidato: %v", err)
	}

	rr, res := doJSONReq(app, "GET", "/api/operadores-do-setor", nil, chefe)
	if rr.Code != http.StatusOK {
		t.Fatalf("listar candidatos: %d (%v)", rr.Code, res)
	}
	corpo := rr.Body.String()
	if !strings.Contains(corpo, "cand_g3") {
		t.Fatalf("candidato do setor deve aparecer na lista: %s", corpo)
	}

	rr, res = doJSONReq(app, "POST", "/api/operadores-do-setor", map[string]any{"login": "cand_g3"}, chefe)
	if rr.Code != http.StatusOK {
		t.Fatalf("designar operador: %d (%v)", rr.Code, res)
	}
	var papel *string
	var gid *int64
	if err := st.db.QueryRow(`SELECT papel, grupo_id FROM usuario_papeis WHERE usuario_id = (SELECT id FROM usuarios WHERE login = 'cand_g3')`).Scan(&papel, &gid); err != nil {
		t.Fatalf("ler papel atribuído: %v", err)
	}
	if papel == nil || *papel != "operador" || gid == nil || *gid != 1 {
		t.Fatalf("papel operador esperado no grupo 1; veio %v %v", papel, gid)
	}
	var papelPrincipal string
	_ = st.db.QueryRow(`SELECT papel FROM usuarios WHERE login = 'cand_g3'`).Scan(&papelPrincipal)
	if papelPrincipal != "normal" {
		t.Fatalf("papel PRINCIPAL da conta não deve mudar; veio %s", papelPrincipal)
	}
}

// chefe NÃO designa conta de outro setor (mesmo do mesmo grupo).
func TestG3RecusaContaDeOutroSetor(t *testing.T) {
	app, st, chefe, _ := setupG3(t)
	if _, err := st.db.Exec(`INSERT OR IGNORE INTO setores (id, nome) VALUES (66, 'Setor G3 B')`); err != nil {
		t.Fatalf("criar setor 66: %v", err)
	}
	criaUsuarioTeste(t, st, "outro_setor_g3", "senha12345", "normal")
	if _, err := st.db.Exec(`UPDATE usuarios SET grupo_id = 1, setor_id = 66 WHERE login = ?`, "outro_setor_g3"); err != nil {
		t.Fatalf("vincular fora do setor: %v", err)
	}

	rr, res := doJSONReq(app, "POST", "/api/operadores-do-setor", map[string]any{"login": "outro_setor_g3"}, chefe)
	if rr.Code != http.StatusForbidden {
		t.Fatalf("conta de outro setor deve dar 403, veio %d (%v)", rr.Code, res)
	}
}

// gerente e operador não operam o endpoint.
func TestG3SoChefeOpera(t *testing.T) {
	app, st, _, _ := setupG3(t)
	ger := loginAs(t, app, "ger_g3", "senha12345")
	rr, res := doJSONReq(app, "GET", "/api/operadores-do-setor", nil, ger)
	if rr.Code != http.StatusForbidden {
		t.Fatalf("gerente no endpoint do chefe deve dar 403, veio %d (%v)", rr.Code, res)
	}
	op := loginAsPapel(t, app, st, "op_g3", "operador", "senha12345")
	rr, res = doJSONReq(app, "GET", "/api/operadores-do-setor", nil, op)
	if rr.Code != http.StatusForbidden {
		t.Fatalf("operador no endpoint do chefe deve dar 403, veio %d (%v)", rr.Code, res)
	}
}
