package main

// ordem_diretor_hierarquia_test.go — Fase G da ordem 04/10 (hierarquia de papéis).
// Prova executada, não alegada: criação de usuários por papel, escopo de
// visibilidade do gerente (grupo + subordinados) e designação de operador
// pelo chefe de setor (com herança de setor).

import (
	"net/http"
	"strings"
	"testing"
)

// admin cria chefe de setor SEM grupo (default da ordem: sem grupo).
func TestG1AdminCriaSemGrupo(t *testing.T) {
	app, st, cleanup := setupTestApp(t)
	defer cleanup()
	admin := loginAs(t, app, "admin", "admin123")

	rr, res := doJSONReq(app, "POST", "/api/usuarios", map[string]any{
		"login": "chefe_sem_grupo", "senha": "senha12345", "papel": "chefe_setor",
	}, admin)
	if rr.Code != http.StatusOK {
		t.Fatalf("criar chefe via admin: %d (%v)", rr.Code, res)
	}
	var grupo int64
	if err := st.db.QueryRow(`SELECT COALESCE(grupo_id,0) FROM usuarios WHERE login = ?`, "chefe_sem_grupo").Scan(&grupo); err != nil || grupo != 0 {
		t.Fatalf("usuário criado por admin deve nascer SEM grupo (grupo=%d, err=%v)", grupo, err)
	}
}

// admin provisiona gerente com grupo explícito (vínculo deliberado permitido).
func TestG1AdminProvisionaGerenteComGrupo(t *testing.T) {
	app, st, cleanup := setupTestApp(t)
	defer cleanup()
	admin := loginAs(t, app, "admin", "admin123")

	gid, _ := criaGrupo(t, app, admin, "G G1 Prov", "ger_g1prov")
	rr, res := doJSONReq(app, "POST", "/api/usuarios", map[string]any{
		"login": "ger_g1novo", "senha": "senha12345", "papel": "gerente", "grupo_id": gid,
	}, admin)
	if rr.Code != http.StatusOK {
		t.Fatalf("admin provisionar gerente c/ grupo: %d (%v)", rr.Code, res)
	}
	var grupo int64
	_ = st.db.QueryRow(`SELECT COALESCE(grupo_id,0) FROM usuarios WHERE login = ?`, "ger_g1novo").Scan(&grupo)
	if grupo != gid {
		t.Fatalf("gerente provisionado deve ficar no grupo %d, ficou %d", gid, grupo)
	}
}

// gerente cria chefe de setor — força o próprio grupo; NÃO cria operador nem gerente.
func TestG1GerenteCriaChefeNoProprioGrupo(t *testing.T) {
	app, st, cleanup := setupTestApp(t)
	defer cleanup()
	admin := loginAs(t, app, "admin", "admin123")
	gid, ger := criaGrupo(t, app, admin, "G G1 Chefe", "ger_g1chefe")

	rr, res := doJSONReq(app, "POST", "/api/usuarios", map[string]any{
		"login": "chefe_g1", "senha": "senha12345", "papel": "chefe_setor", "grupo_id": 99999,
	}, ger)
	if rr.Code != http.StatusOK {
		t.Fatalf("gerente criar chefe: %d (%v)", rr.Code, res)
	}
	// grupo 99999 no corpo deve ser IGNORADO — fica no grupo do gerente
	var grupo int64
	_ = st.db.QueryRow(`SELECT COALESCE(grupo_id,0) FROM usuarios WHERE login = ?`, "chefe_g1").Scan(&grupo)
	if grupo != gid {
		t.Fatalf("chefe criado pelo gerente deve herdar o grupo do gerente (%d), ficou %d", gid, grupo)
	}

	// gerente NÃO cria operador (ordem: ele seleciona chefes; chefes criam operadores)
	rrOp, resOp := doJSONReq(app, "POST", "/api/usuarios", map[string]any{
		"login": "op_negado_g1", "senha": "senha12345", "papel": "operador",
	}, ger)
	if rrOp.Code != http.StatusForbidden {
		t.Fatalf("gerente criando operador deve dar 403, veio %d (%v)", rrOp.Code, resOp)
	}
	// gerente NÃO cria gerente
	rrGer, resGer := doJSONReq(app, "POST", "/api/usuarios", map[string]any{
		"login": "ger_negado_g1", "senha": "senha12345", "papel": "gerente",
	}, ger)
	if rrGer.Code != http.StatusForbidden {
		t.Fatalf("gerente criando gerente deve dar 403, veio %d (%v)", rrGer.Code, resGer)
	}
}

// chefe de setor designa operador — herda o setor do chefe; chefe NÃO cria chefe/gerente.
func TestG1ChefeDesignaOperadorHerdaSetor(t *testing.T) {
	app, st, cleanup := setupTestApp(t)
	defer cleanup()
	admin := loginAs(t, app, "admin", "admin123")
	gid, ger := criaGrupo(t, app, admin, "G G1 Op", "ger_g1op")
	// vínculos do chefe ANTES do login: a sessão congela o papel ativo
	// (usuario_papeis) no momento do login.
	chefeCookie := func() *http.Cookie {
		criaUsuarioTeste(t, st, "chefe_g1op", "senha12345", "chefe_setor")
		if _, err := st.db.Exec(`INSERT OR IGNORE INTO setores (id, nome) VALUES (77, 'Setor G1')`); err != nil {
			t.Fatalf("criar setor 77: %v", err)
		}
		if _, err := st.db.Exec(`UPDATE usuarios SET grupo_id = ?, setor_id = 77 WHERE login = ?`, gid, "chefe_g1op"); err != nil {
			t.Fatalf("vincular chefe: %v", err)
		}
		return loginAs(t, app, "chefe_g1op", "senha12345")
	}()
	_ = ger

	rr, res := doJSONReq(app, "POST", "/api/usuarios", map[string]any{
		"login": "op_g1", "senha": "senha12345", "papel": "operador",
	}, chefeCookie)
	if rr.Code != http.StatusOK {
		t.Fatalf("chefe designar operador: %d (%v)", rr.Code, res)
	}
	var setor, grupo int64
	_ = st.db.QueryRow(`SELECT COALESCE(setor_id,0), COALESCE(grupo_id,0) FROM usuarios WHERE login = ?`, "op_g1").Scan(&setor, &grupo)
	if setor != 77 {
		t.Fatalf("operador deve herdar setor 77 do chefe, ficou %d", setor)
	}
	if grupo != gid {
		t.Fatalf("operador designado pelo chefe deve ficar no grupo do chefe (%d), ficou %d", gid, grupo)
	}

	// chefe NÃO cria chefe de setor nem gerente
	rrCh, resCh := doJSONReq(app, "POST", "/api/usuarios", map[string]any{
		"login": "chefe_negado", "senha": "senha12345", "papel": "chefe_setor",
	}, chefeCookie)
	if rrCh.Code != http.StatusForbidden {
		t.Fatalf("chefe criando chefe deve dar 403, veio %d (%v)", rrCh.Code, resCh)
	}
	rrGer2, resGer2 := doJSONReq(app, "POST", "/api/usuarios", map[string]any{
		"login": "ger_negado2", "senha": "senha12345", "papel": "gerente",
	}, chefeCookie)
	if rrGer2.Code != http.StatusForbidden {
		t.Fatalf("chefe criando gerente deve dar 403, veio %d (%v)", rrGer2.Code, resGer2)
	}
}

// operador não cria conta nenhuma.
func TestG1OperadorNaoCriaNada(t *testing.T) {
	app, st, cleanup := setupTestApp(t)
	defer cleanup()
	admin := loginAs(t, app, "admin", "admin123")
	gid, _ := criaGrupo(t, app, admin, "G G1 NegOp", "ger_g1negop")
	opCookie := loginAsPapel(t, app, st, "op_g1neg", "operador", "senha12345")
	if _, err := st.db.Exec(`UPDATE usuarios SET grupo_id = ? WHERE login = ?`, gid, "op_g1neg"); err != nil {
		t.Fatalf("vincular operador: %v", err)
	}

	rr, res := doJSONReq(app, "POST", "/api/usuarios", map[string]any{
		"login": "alguem", "senha": "senha12345", "papel": "operador",
	}, opCookie)
	if rr.Code != http.StatusForbidden {
		t.Fatalf("operador criando conta deve dar 403, veio %d (%v)", rr.Code, res)
	}
}

// gerente vê contas do próprio grupo E dos grupos subordinados; admin vê tudo.
func TestG1GerenteVeSubordinados(t *testing.T) {
	app, st, cleanup := setupTestApp(t)
	defer cleanup()
	admin := loginAs(t, app, "admin", "admin123")
	gidA, gerA := criaGrupo(t, app, admin, "G G1 Pai", "ger_g1pai")
	gidB, _ := criaGrupo(t, app, admin, "G G1 Filho", "ger_g1filho")
	if _, err := st.db.Exec(`INSERT INTO grupo_vinculos (superior_id, subordinado_id, criado_por_superior, criado_por_subordinado)
		VALUES (?, ?, 1, 1)`, gidA, gidB); err != nil {
		t.Fatalf("vínculo A→B: %v", err)
	}
	// conta no grupo filho
	if _, err := st.db.Exec(`INSERT INTO usuarios (login, senha_hash, papel, grupo_id, precisa_setup)
		VALUES ('membro_filho', 'x', 'operador', ?, 0)`, gidB); err != nil {
		t.Fatalf("conta no grupo filho: %v", err)
	}

	rr, _ := doJSONReq(app, "GET", "/api/usuarios", nil, gerA)
	if rr.Code != http.StatusOK {
		t.Fatalf("lista usuários p/ gerente: %d", rr.Code)
	}
	corpo := rr.Body.String()
	if !strings.Contains(corpo, "membro_filho") {
		t.Fatalf("gerente do grupo pai deve ver conta do grupo subordinado; corpo: %s", corpo[:min(len(corpo), 400)])
	}
}
