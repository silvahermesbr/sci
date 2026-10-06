package main

// Testes da onda Escalas + Nomeação (05/10):
//   X1. Gerente designa chefe e operador na escala da conferência → 200.
//   X2. Chefe designa operador DE OUTRO setor → 403.
//   X3. Admin na escala → 403 (regra de ouro dos handlers escopados).
//   X4. nomear_chefe por não-gerente (admin) → 403.
//   X5. Destituir chefe remove o papel (segunda vez → 404).
//   X6. GET da conferência devolve "escala" com login/nome_guerra.
//   X7. Remoção de designação (DELETE) → 200; repetir → 404.
//   X8. Operador não designa ninguém → 403.

import (
	"net/http"
	"strconv"
	"testing"
)

func escalasSetup(t *testing.T, app *App, st *Store) (gid, setorA, setorB, uidGer, uidChefeA, uidOpA, uidOpB int64) {
	t.Helper()
	if err := st.db.QueryRow(`INSERT INTO grupos (nome) VALUES ('Grp Escalas') RETURNING id`).Scan(&gid); err != nil {
		t.Fatalf("criar grupo: %v", err)
	}
	if err := st.db.QueryRow(`INSERT INTO setores (nome, sigla) VALUES ('Setor X Esc','SXE') RETURNING id`).Scan(&setorA); err != nil {
		t.Fatalf("criar setor A: %v", err)
	}
	if err := st.db.QueryRow(`INSERT INTO setores (nome, sigla) VALUES ('Setor Y Esc','SYE') RETURNING id`).Scan(&setorB); err != nil {
		t.Fatalf("criar setor B: %v", err)
	}

	criaUsuarioTeste(t, st, "ger_esc", "senha-gerente", "gerente")
	criaUsuarioTeste(t, st, "ch_esc", "senha-gerente", "chefe_setor")
	criaUsuarioTeste(t, st, "opa_esc", "senha-gerente", "operador")
	criaUsuarioTeste(t, st, "opb_esc", "senha-gerente", "operador")

	if _, err := st.db.Exec(`UPDATE usuarios SET nome_guerra = CASE login
		WHEN 'ger_esc' THEN 'Gerência' WHEN 'ch_esc' THEN 'Chefe X'
		WHEN 'opa_esc' THEN 'Op A' ELSE 'Op B' END
		WHERE login IN ('ger_esc','ch_esc','opa_esc','opb_esc')`); err != nil {
		t.Fatalf("nome_guerra dos usuários de teste: %v", err)
	}
	if _, err := st.db.Exec(`UPDATE usuarios SET grupo_id = ? WHERE login IN ('ger_esc','ch_esc','opa_esc','opb_esc')`, gid); err != nil {
		t.Fatalf("vincular grupo: %v", err)
	}
	if _, err := st.db.Exec(`UPDATE usuarios SET setor_id = ? WHERE login = 'ch_esc'`, setorA); err != nil {
		t.Fatalf("setor chefe: %v", err)
	}
	if _, err := st.db.Exec(`UPDATE usuarios SET setor_id = ? WHERE login = 'opa_esc'`, setorA); err != nil {
		t.Fatalf("setor opA: %v", err)
	}
	if _, err := st.db.Exec(`UPDATE usuarios SET setor_id = ? WHERE login = 'opb_esc'`, setorB); err != nil {
		t.Fatalf("setor opB: %v", err)
	}

	var uid int64
	if err := st.db.QueryRow(`SELECT id FROM usuarios WHERE login = 'ger_esc'`).Scan(&uid); err != nil {
		t.Fatalf("id ger: %v", err)
	}
	if _, err := st.db.Exec(`DELETE FROM usuario_papeis WHERE usuario_id = ? AND grupo_id IS NULL`, uid); err != nil {
		t.Fatalf("remover papel sem grupo: %v", err)
	}
	if _, err := st.db.Exec(`UPDATE usuario_papeis SET grupo_id = ? WHERE usuario_id IN (SELECT id FROM usuarios WHERE login IN ('ger_esc','ch_esc','opa_esc','opb_esc'))`, gid); err != nil {
		t.Fatalf("vincular papeis: %v", err)
	}

	var chefeA, opA, opB int64
	if err := st.db.QueryRow(`SELECT id FROM usuarios WHERE login = 'ch_esc'`).Scan(&chefeA); err != nil {
		t.Fatalf("id chefe: %v", err)
	}
	if err := st.db.QueryRow(`SELECT id FROM usuarios WHERE login = 'opa_esc'`).Scan(&opA); err != nil {
		t.Fatalf("id opA: %v", err)
	}
	if err := st.db.QueryRow(`SELECT id FROM usuarios WHERE login = 'opb_esc'`).Scan(&opB); err != nil {
		t.Fatalf("id opB: %v", err)
	}
	return gid, setorA, setorB, uid, chefeA, opA, opB
}

func escalasConfAberta(t *testing.T, app *App, ck *http.Cookie) int64 {
	t.Helper()
	rr, res := doJSONReq(app, "POST", "/api/conferencia/iniciar", map[string]any{"nome": "Conf Escalas", "prazo_final": "17:00"}, ck)
	if rr.Code != http.StatusOK {
		t.Fatalf("iniciar conferência: %d %v", rr.Code, res)
	}
	if id, ok := res["id"].(float64); ok {
		return int64(id)
	}
	var cid int64
	if err := app.st.db.QueryRow(`SELECT id FROM conferencias WHERE status = 'aberta' ORDER BY id DESC LIMIT 1`).Scan(&cid); err != nil {
		t.Fatalf("id da conferência: %v", err)
	}
	return cid
}

// i64: helper local de formatação de id em path.
func i64(v int64) string {
	return strconv.FormatInt(v, 10)
}

func TestX1GerenteDesignaEscala(t *testing.T) {
	app, st, cleanup := setupTestApp(t)
	defer cleanup()
	gid, _, _, _, uidChefe, uidOp, _ := escalasSetup(t, app, st)
	_ = gid
	ckGer := vinculaGrupoDoLogin(t, app, st, "ger_esc", gid)
	cid := escalasConfAberta(t, app, ckGer)

	rr, res := doJSONReq(app, "POST", "/api/conferencia/"+i64(cid)+"/escala", map[string]any{"usuario_id": uidChefe, "papel_na_escala": "chefe"}, ckGer)
	if rr.Code != http.StatusOK {
		t.Fatalf("X1a: gerente designa chefe → 200, veio %d %v", rr.Code, res)
	}
	rr, res = doJSONReq(app, "POST", "/api/conferencia/"+i64(cid)+"/escala", map[string]any{"usuario_id": uidOp, "papel_na_escala": "operador"}, ckGer)
	if rr.Code != http.StatusOK {
		t.Fatalf("X1b: gerente designa operador → 200, veio %d %v", rr.Code, res)
	}
}

func TestX2ChefeOutroSetor403(t *testing.T) {
	app, st, cleanup := setupTestApp(t)
	defer cleanup()
	_, _, _, uidGer, _, _, uidOpB := escalasSetup(t, app, st)
	// sessão DIRETA (CriarSessao): logar como gerente religaria o papel_ativo
	// à primeira linha de usuario_papeis (gerente), escondendo o papel do chefe.
	tokGer, _, _ := st.CriarSessao(uidGer, ttlSessao)
	cid := escalasConfAberta(t, app, &http.Cookie{Name: cookieSessao, Value: tokGer})
	var uidCh int64
	if err := st.db.QueryRow(`SELECT id FROM usuarios WHERE login = 'ch_esc'`).Scan(&uidCh); err != nil {
		t.Fatalf("id chefe: %v", err)
	}
	tokCh, _, _ := st.CriarSessao(uidCh, ttlSessao)
	ckCh := &http.Cookie{Name: cookieSessao, Value: tokCh}

	rr, res := doJSONReq(app, "POST", "/api/conferencia/"+i64(cid)+"/escala", map[string]any{"usuario_id": uidOpB, "papel_na_escala": "operador"}, ckCh)
	if rr.Code != http.StatusForbidden {
		t.Fatalf("X2: chefe designa operador de outro setor → 403, veio %d %v", rr.Code, res)
	}
}

func TestX3Admin403NaEscala(t *testing.T) {
	app, st, cleanup := setupTestApp(t)
	defer cleanup()
	gid, _, _, _, _, uidOp, _ := escalasSetup(t, app, st)
	_ = gid
	ckGer := vinculaGrupoDoLogin(t, app, st, "ger_esc", gid)
	cid := escalasConfAberta(t, app, ckGer)
	ckAdm := loginAs(t, app, "admin", "admin123")

	rr, res := doJSONReq(app, "POST", "/api/conferencia/"+i64(cid)+"/escala", map[string]any{"usuario_id": uidOp, "papel_na_escala": "operador"}, ckAdm)
	if rr.Code != http.StatusForbidden {
		t.Fatalf("X3: admin → 403, veio %d %v", rr.Code, res)
	}
}

func TestX4NomearChefeSoGerente(t *testing.T) {
	app, st, cleanup := setupTestApp(t)
	defer cleanup()
	gid, _, setorA, _, uidOp, _, _ := escalasSetup(t, app, st)
	_ = setorA
	ckAdm := loginAs(t, app, "admin", "admin123")

	rr, res := doJSONReq(app, "POST", "/api/grupos/"+i64(gid)+"/nomear_chefe", map[string]any{"usuario_id": uidOp, "setor_id": setorA}, ckAdm)
	if rr.Code != http.StatusForbidden {
		t.Fatalf("X4: não-gerente nomeia chefe → 403, veio %d %v", rr.Code, res)
	}
}

func TestX5NomearEDestituirChefe(t *testing.T) {
	app, st, cleanup := setupTestApp(t)
	defer cleanup()
	gid, _, setorA, _, uidOpA, _, _ := escalasSetup(t, app, st)
	ckGer := vinculaGrupoDoLogin(t, app, st, "ger_esc", gid)

	rr, res := doJSONReq(app, "POST", "/api/grupos/"+i64(gid)+"/nomear_chefe", map[string]any{"usuario_id": uidOpA, "setor_id": setorA}, ckGer)
	if rr.Code != http.StatusOK {
		t.Fatalf("X5a: gerente nomeia chefe → 200, veio %d %v", rr.Code, res)
	}
	var papelExiste int
	if err := app.st.db.QueryRow(`SELECT COUNT(*) FROM usuario_papeis WHERE usuario_id = ? AND grupo_id = ? AND papel = 'chefe_setor'`, uidOpA, gid).Scan(&papelExiste); err != nil || papelExiste == 0 {
		t.Fatalf("X5a: papel chefe_setor não gravado (n=%d err=%v)", papelExiste, err)
	}
	var setorGravado *int64
	if err := app.st.db.QueryRow(`SELECT setor_id FROM usuarios WHERE id = ?`, uidOpA).Scan(&setorGravado); err != nil || setorGravado == nil || *setorGravado != setorA {
		t.Fatalf("X5a: setor_id não vinculado")
	}

	rr, res = doJSONReq(app, "POST", "/api/grupos/"+i64(gid)+"/destituir_chefe", map[string]any{"usuario_id": uidOpA}, ckGer)
	if rr.Code != http.StatusOK {
		t.Fatalf("X5b: destituir → 200, veio %d %v", rr.Code, res)
	}
	if err := app.st.db.QueryRow(`SELECT COUNT(*) FROM usuario_papeis WHERE usuario_id = ? AND grupo_id = ? AND papel = 'chefe_setor'`, uidOpA, gid).Scan(&papelExiste); err != nil || papelExiste != 0 {
		t.Fatalf("X5b: papel chefe_setor ainda existe")
	}
	// repetir → 404
	rr, res = doJSONReq(app, "POST", "/api/grupos/"+i64(gid)+"/destituir_chefe", map[string]any{"usuario_id": uidOpA}, ckGer)
	if rr.Code != http.StatusNotFound {
		t.Fatalf("X5c: destituir de novo → 404, veio %d %v", rr.Code, res)
	}
}

func TestX6GetDevolveEscala(t *testing.T) {
	app, st, cleanup := setupTestApp(t)
	defer cleanup()
	gid, _, _, _, uidChefe, _, _ := escalasSetup(t, app, st)
	ckGer := vinculaGrupoDoLogin(t, app, st, "ger_esc", gid)
	cid := escalasConfAberta(t, app, ckGer)

	if rr, _ := doJSONReq(app, "POST", "/api/conferencia/"+i64(cid)+"/escala", map[string]any{"usuario_id": uidChefe, "papel_na_escala": "chefe"}, ckGer); rr.Code != http.StatusOK {
		t.Fatalf("setup designar: %d", rr.Code)
	}
	rr, res := doJSONReq(app, "GET", "/api/conferencia/"+i64(cid), nil, ckGer)
	if rr.Code != http.StatusOK {
		t.Fatalf("GET conferência: %d", rr.Code)
	}
	escala, ok := res["escala"].([]any)
	if !ok || len(escala) != 1 {
		t.Fatalf("X6: GET não devolveu escala com 1 linha: %v", res["escala"])
	}
	linha, _ := escala[0].(map[string]any)
	if linha["papel_na_escala"] != "chefe" || linha["login"] != "ch_esc" {
		t.Fatalf("X6: linha da escala incorreta: %v", linha)
	}
	if linha["nome_guerra"] == "" || linha["nome_guerra"] == nil {
		t.Fatalf("X6: nome_guerra ausente: %v", linha)
	}
}

func TestX7RemoverDesignacao(t *testing.T) {
	app, st, cleanup := setupTestApp(t)
	defer cleanup()
	gid, _, _, _, uidChefe, _, _ := escalasSetup(t, app, st)
	ckGer := vinculaGrupoDoLogin(t, app, st, "ger_esc", gid)
	cid := escalasConfAberta(t, app, ckGer)

	if rr, _ := doJSONReq(app, "POST", "/api/conferencia/"+i64(cid)+"/escala", map[string]any{"usuario_id": uidChefe, "papel_na_escala": "chefe"}, ckGer); rr.Code != http.StatusOK {
		t.Fatalf("setup designar: %d", rr.Code)
	}
	rr, res := doJSONReq(app, "DELETE", "/api/conferencia/"+i64(cid)+"/escala/"+i64(uidChefe), nil, ckGer)
	if rr.Code != http.StatusOK {
		t.Fatalf("X7a: remover → 200, veio %d %v", rr.Code, res)
	}
	rr, res = doJSONReq(app, "DELETE", "/api/conferencia/"+i64(cid)+"/escala/"+i64(uidChefe), nil, ckGer)
	if rr.Code != http.StatusNotFound {
		t.Fatalf("X7b: remover de novo → 404, veio %d %v", rr.Code, res)
	}
}

func TestX8OperadorNaoDesigna(t *testing.T) {
	app, st, cleanup := setupTestApp(t)
	defer cleanup()
	gid, _, _, uidGer, _, uidOpA, _ := escalasSetup(t, app, st)
	_ = gid
	tokGer, _, _ := st.CriarSessao(uidGer, ttlSessao)
	cid := escalasConfAberta(t, app, &http.Cookie{Name: cookieSessao, Value: tokGer})
	tokOp, _, _ := st.CriarSessao(uidOpA, ttlSessao)
	ckOp := &http.Cookie{Name: cookieSessao, Value: tokOp}

	rr, res := doJSONReq(app, "POST", "/api/conferencia/"+i64(cid)+"/escala", map[string]any{"usuario_id": uidOpA, "papel_na_escala": "operador"}, ckOp)
	if rr.Code != http.StatusForbidden {
		t.Fatalf("X8: operador designa → 403, veio %d %v", rr.Code, res)
	}
}

// TestX9ChefeUnicoPorSetor (ordem 06/10, item 1): nomear chefe 2x no MESMO setor —
// o anterior é demitido AUTOMATICAMENTE na mesma transação: só 1 linha
// chefe_setor para aquele setor; o antigo não mantém o papel; o novo responde
// como chefe (sessão com papel ativo chefe_setor). O novo chefe assume com
// usuarios.setor_id = S (pré-condição do modelo: papel não tem coluna de setor;
// o setor corrente do usuário é que liga a linha chefe_setor ao setor).
func TestX9ChefeUnicoPorSetor(t *testing.T) {
	app, st, cleanup := setupTestApp(t)
	defer cleanup()
	gid, setorA, _, _, _, uidOpA, uidOpB := escalasSetup(t, app, st)
	ckGer := vinculaGrupoDoLogin(t, app, st, "ger_esc", gid)

	// 1ª nomeação: opA vira chefe do setor A
	rr, res := doJSONReq(app, "POST", "/api/grupos/"+i64(gid)+"/nomear_chefe", map[string]any{"usuario_id": uidOpA, "setor_id": setorA}, ckGer)
	if rr.Code != http.StatusOK {
		t.Fatalf("X9a: 1ª nomeação → 200, veio %d %v", rr.Code, res)
	}
	// 2ª nomeação: opB substitui opA NO MESMO setor
	rr, res = doJSONReq(app, "POST", "/api/grupos/"+i64(gid)+"/nomear_chefe", map[string]any{"usuario_id": uidOpB, "setor_id": setorA}, ckGer)
	if rr.Code != http.StatusOK {
		t.Fatalf("X9b: 2ª nomeação (substituição) → 200, veio %d %v", rr.Code, res)
	}

	// (i) o ANTIGO não mantém o papel
	var papelAntigo int
	if err := app.st.db.QueryRow(`SELECT COUNT(*) FROM usuario_papeis WHERE usuario_id = ? AND grupo_id = ? AND papel = 'chefe_setor'`, uidOpA, gid).Scan(&papelAntigo); err != nil || papelAntigo != 0 {
		t.Fatalf("X9c: chefe antigo manteve o papel (n=%d err=%v)", papelAntigo, err)
	}
	// (ii) o NOVO tem o papel E está vinculado ao setor
	var papelNovo int
	if err := app.st.db.QueryRow(`SELECT COUNT(*) FROM usuario_papeis WHERE usuario_id = ? AND grupo_id = ? AND papel = 'chefe_setor'`, uidOpB, gid).Scan(&papelNovo); err != nil || papelNovo != 1 {
		t.Fatalf("X9d: papel chefe_setor do novo não gravado (n=%d err=%v)", papelNovo, err)
	}
	var setorNovo *int64
	if err := app.st.db.QueryRow(`SELECT setor_id FROM usuarios WHERE id = ?`, uidOpB).Scan(&setorNovo); err != nil || setorNovo == nil || *setorNovo != setorA {
		t.Fatalf("X9e: novo chefe sem setor_id = setor A (err=%v)", err)
	}
	// (iii) apenas UMA linha chefe_setor aponta para o setor A no grupo
	// (modelo: setor corrente em usuarios.setor_id — quem tem o setor tem o papel)
	var totalSetor int
	if err := app.st.db.QueryRow(`SELECT COUNT(*) FROM usuario_papeis up
		JOIN usuarios u ON u.id = up.usuario_id
		WHERE up.papel = 'chefe_setor' AND up.grupo_id = ? AND u.setor_id = ? AND u.grupo_id = ?`, gid, setorA, gid).Scan(&totalSetor); err != nil || totalSetor != 1 {
		t.Fatalf("X9f: esperado EXATAMENTE 1 chefe para o setor A, veio %d (err=%v)", totalSetor, err)
	}

	// (iv) o NOVO responde como chefe: sessão nova com papel ativo = chefe_setor
	tokNovo, _, _ := st.CriarSessao(uidOpB, ttlSessao)
	ckNovo := &http.Cookie{Name: cookieSessao, Value: tokNovo}
	rr2, res2 := doJSONReq(app, "GET", "/api/me", nil, ckNovo)
	if rr2.Code != http.StatusOK {
		t.Fatalf("X9g: /api/me do novo chefe → 200, veio %d %v", rr2.Code, res2)
	}
	uMap, _ := res2["usuario"].(map[string]any)
	if uMap == nil || uMap["papel"] != "chefe_setor" {
		t.Fatalf("X9h: novo chefe responde com papel ativo errado: %v", res2)
	}
}
