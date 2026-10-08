package main

// Testes da onda R1 (ordem CEO, 08/10): escopo de funções — correção F1+F2.
//
//   R1a. Gerente tenta designar função EXCLUSIVA de OUTRO grupo → 400; nada gravado.
//   R1b. Gerente de grupo SUBORDINADO designa função do grupo SUPERIOR (herdada) → 200.
//   R1c. GET do gerente NÃO contém função exclusiva de outro grupo.
//   R1d. Gerente designa função do PRÓPRIO grupo → 200 (regressão).

import (
	"encoding/json"
	"net/http"
	"testing"
)

// r1EscopoSetup cria hierarquia de 2 grupos (A=superior, B=subordinado a A)
// com funções em cada escopo + funções globais. Retorna IDs.
func r1EscopoSetup(t *testing.T, app *App, st *Store) (gidA, gidB, funcGlobalID, funcAID, funcBID, gerAUID, gerBUID int64) {
	t.Helper()

	// Grupo A (superior)
	if err := st.db.QueryRow(`INSERT INTO grupos (nome) VALUES ('Grupo R1 Alfa') RETURNING id`).Scan(&gidA); err != nil {
		t.Fatalf("criar grupo A: %v", err)
	}
	// Grupo B (subordinado a A)
	if err := st.db.QueryRow(`INSERT INTO grupos (nome) VALUES ('Grupo R1 Beta') RETURNING id`).Scan(&gidB); err != nil {
		t.Fatalf("criar grupo B: %v", err)
	}
	// Vínculo: B é subordinado de A
	if _, err := st.db.Exec(`INSERT INTO grupo_vinculos (superior_id, subordinado_id) VALUES (?, ?)`, gidA, gidB); err != nil {
		t.Fatalf("vincular B subordinado a A: %v", err)
	}

	// Função global (sem grupo — visível a todos)
	if err := st.db.QueryRow(`INSERT INTO funcoes (nome, grupo_id) VALUES ('Funcao R1 Global', NULL) RETURNING id`).Scan(&funcGlobalID); err != nil {
		t.Fatalf("criar função global: %v", err)
	}
	// Função exclusiva de A
	if err := st.db.QueryRow(`INSERT INTO funcoes (nome, grupo_id) VALUES ('Funcao R1 Exclusiva A', ?) RETURNING id`, gidA).Scan(&funcAID); err != nil {
		t.Fatalf("criar função A: %v", err)
	}
	// Função exclusiva de B
	if err := st.db.QueryRow(`INSERT INTO funcoes (nome, grupo_id) VALUES ('Funcao R1 Exclusiva B', ?) RETURNING id`, gidB).Scan(&funcBID); err != nil {
		t.Fatalf("criar função B: %v", err)
	}

	// Gerente de A
	criaUsuarioTeste(t, st, "r1gerA", "senha-gerA", "gerente")
	if _, err := st.db.Exec(`UPDATE usuarios SET grupo_id = ? WHERE login = 'r1gerA'`, gidA); err != nil {
		t.Fatalf("vincular gerA ao grupo A: %v", err)
	}
	if _, err := st.db.Exec(`UPDATE usuario_papeis SET grupo_id = ? WHERE usuario_id = (SELECT id FROM usuarios WHERE login = 'r1gerA')`, gidA); err != nil {
		t.Fatalf("vincular papel do gerA: %v", err)
	}
	if err := st.db.QueryRow(`SELECT id FROM usuarios WHERE login = 'r1gerA'`).Scan(&gerAUID); err != nil {
		t.Fatalf("id gerA: %v", err)
	}

	// Gerente de B
	criaUsuarioTeste(t, st, "r1gerB", "senha-gerB", "gerente")
	if _, err := st.db.Exec(`UPDATE usuarios SET grupo_id = ? WHERE login = 'r1gerB'`, gidB); err != nil {
		t.Fatalf("vincular gerB ao grupo B: %v", err)
	}
	if _, err := st.db.Exec(`UPDATE usuario_papeis SET grupo_id = ? WHERE usuario_id = (SELECT id FROM usuarios WHERE login = 'r1gerB')`, gidB); err != nil {
		t.Fatalf("vincular papel do gerB: %v", err)
	}
	if err := st.db.QueryRow(`SELECT id FROM usuarios WHERE login = 'r1gerB'`).Scan(&gerBUID); err != nil {
		t.Fatalf("id gerB: %v", err)
	}

	return
}

func TestR1EscopoFuncoes(t *testing.T) {
	app, st, cleanup := setupTestApp(t)
	defer cleanup()
	gidA, gidB, funcGlobalID, funcAID, funcBID, gerAUID, gerBUID := r1EscopoSetup(t, app, st)
	_ = gidA
	_ = gidB
	_ = funcGlobalID

	ckGerA := loginAs(t, app, "r1gerA", "senha-gerA")
	ckGerB := loginAs(t, app, "r1gerB", "senha-gerB")

	// ---- R1d: designar função do PRÓPRIO grupo → 200 (regressão) ----
	rr, res := doJSONReq(app, "POST", "/api/grupo/funcoes/membros",
		map[string]any{"funcao_id": funcAID, "usuario_id": gerAUID, "titularidade": "titular"}, ckGerA)
	if rr.Code != http.StatusOK {
		t.Fatalf("R1d: designar função própria (A) deve 200, veio %d (%v)", rr.Code, res)
	}
	// Confirma que gravou
	var tit string
	if err := st.db.QueryRow(`SELECT titularidade FROM funcao_membros WHERE funcao_id=? AND grupo_id=? AND usuario_id=?`, funcAID, gidA, gerAUID).Scan(&tit); err != nil || tit != "titular" {
		t.Fatalf("R1d: designação não persistiu (tit=%q err=%v)", tit, err)
	}

	// ---- R1a: designar função EXCLUSIVA de OUTRO grupo → 400 e NADA gravado ----
	rr, res = doJSONReq(app, "POST", "/api/grupo/funcoes/membros",
		map[string]any{"funcao_id": funcBID, "usuario_id": gerAUID, "titularidade": "titular"}, ckGerA)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("R1a: designar função exclusiva de B como gerente A deve 400, veio %d (%v)", rr.Code, res)
	}
	// Verifica que NADA foi gravado na tabela para essa função+grupo
	var cnt int
	_ = st.db.QueryRow(`SELECT COUNT(*) FROM funcao_membros WHERE funcao_id=? AND grupo_id=?`, funcBID, gidA).Scan(&cnt)
	if cnt != 0 {
		t.Fatalf("R1a: designação de função alheia não deveria ter sido gravada (cnt=%d)", cnt)
	}

	// ---- R1c: GET do gerente A NÃO contém função exclusiva de B ----
	rr, _ = doJSONReq(app, "GET", "/api/grupo/funcoes/membros", nil, ckGerA)
	if rr.Code != http.StatusOK {
		t.Fatalf("R1c: GET membros 200, veio %d", rr.Code)
	}
	var lista []map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &lista); err != nil {
		t.Fatalf("R1c: resposta não é array: %v", err)
	}
	temGlobal := false
	temExclusivaA := false
	temExclusivaB := false
	for _, it := range lista {
		nome, _ := it["funcao_nome"].(string)
		fid := int64(it["funcao_id"].(float64))
		if fid == funcGlobalID {
			temGlobal = true
		}
		if fid == funcAID {
			temExclusivaA = true
		}
		if fid == funcBID {
			temExclusivaB = true
		}
		_ = nome
	}
	if !temGlobal {
		t.Fatalf("R1c: GET gerente A deveria conter função global (ID=%d), não veio", funcGlobalID)
	}
	if !temExclusivaA {
		t.Fatalf("R1c: GET gerente A deveria conter função exclusiva A (ID=%d), não veio", funcAID)
	}
	if temExclusivaB {
		t.Fatalf("R1c: GET gerente A NÃO deveria conter função exclusiva B (ID=%d), mas veio", funcBID)
	}

	// ---- R1b: gerente B designa função do grupo SUPERIOR A (herdada) → 200 ----
	rr, res = doJSONReq(app, "POST", "/api/grupo/funcoes/membros",
		map[string]any{"funcao_id": funcAID, "usuario_id": gerBUID, "titularidade": "titular"}, ckGerB)
	if rr.Code != http.StatusOK {
		t.Fatalf("R1b: gerente B designar função superior A deve 200, veio %d (%v)", rr.Code, res)
	}
	// Confirma que gravou no grupo B (designação é sempre do próprio grupo)
	if err := st.db.QueryRow(`SELECT titularidade FROM funcao_membros WHERE funcao_id=? AND grupo_id=? AND usuario_id=?`, funcAID, gidB, gerBUID).Scan(&tit); err != nil || tit != "titular" {
		t.Fatalf("R1b: designação herdada não persistiu (tit=%q err=%v)", tit, err)
	}

	// ---- R1b variante: gerente B também consegue designar função própria e global ----
	rr, _ = doJSONReq(app, "POST", "/api/grupo/funcoes/membros",
		map[string]any{"funcao_id": funcBID, "usuario_id": gerBUID, "titularidade": "auxiliar"}, ckGerB)
	if rr.Code != http.StatusOK {
		t.Fatalf("R1b variante: gerente B designar exclusiva B deve 200, veio %d", rr.Code)
	}
}