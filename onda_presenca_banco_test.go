package main

import (
	"fmt"
	"net/http"
	"testing"
)

func ondaPresencaBancoSetup(t *testing.T, app *App, st *Store) (gid, gidFora, p1, pFora, fEnc, fAux int64) {
	t.Helper()
	if err := st.db.QueryRow(`INSERT INTO grupos (nome) VALUES ('Grp Presenca 1') RETURNING id`).Scan(&gid); err != nil {
		t.Fatalf("criar grupo 1: %v", err)
	}
	if err := st.db.QueryRow(`INSERT INTO grupos (nome) VALUES ('Grp Presenca 2') RETURNING id`).Scan(&gidFora); err != nil {
		t.Fatalf("criar grupo 2: %v", err)
	}
	if err := st.db.QueryRow(`INSERT INTO funcoes (nome, grupo_id) VALUES ('Encarregado de Pessoal Teste', ?) RETURNING id`, gid).Scan(&fEnc); err != nil {
		t.Fatalf("criar função encarregado: %v", err)
	}
	if err := st.db.QueryRow(`INSERT INTO funcoes (nome, grupo_id) VALUES ('Auxiliar de Pessoal Teste', ?) RETURNING id`, gid).Scan(&fAux); err != nil {
		t.Fatalf("criar função auxiliar: %v", err)
	}

	criaUsuarioTeste(t, st, "ger01_p", "senha-ger", "gerente")
	criaUsuarioTeste(t, st, "enc01_p", "senha-enc", "")
	criaUsuarioTeste(t, st, "aux01_p", "senha-aux", "")
	criaUsuarioTeste(t, st, "chefe01_p", "senha-chefe", "chefe_setor")
	criaUsuarioTeste(t, st, "op01_p", "senha-op", "operador")
	criaUsuarioTeste(t, st, "gerFora_p", "senha-gerf", "gerente")
	criaUsuarioTeste(t, st, "adm01_p", "senha-adm", "admin")

	vincula := `UPDATE usuarios SET grupo_id = ?, funcao_id = ? WHERE login = ?`
	if _, err := st.db.Exec(vincula, gid, fEnc, "enc01_p"); err != nil {
		t.Fatalf("vincular enc01_p: %v", err)
	}
	if _, err := st.db.Exec(vincula, gid, fAux, "aux01_p"); err != nil {
		t.Fatalf("vincular aux01_p: %v", err)
	}
	if _, err := st.db.Exec(`UPDATE usuarios SET grupo_id = ? WHERE login IN ('ger01_p','chefe01_p','op01_p')`, gid); err != nil {
		t.Fatalf("vincular usuarios gid: %v", err)
	}
	if _, err := st.db.Exec(`UPDATE usuarios SET grupo_id = ? WHERE login = 'gerFora_p'`, gidFora); err != nil {
		t.Fatalf("vincular gerFora_p: %v", err)
	}

	if err := st.db.QueryRow(`INSERT INTO pessoas (nome_guerra, nome_completo, grupo_id, status) VALUES ('ALFA', 'Alfa Teste', ?, 'ativo') RETURNING id`, gid).Scan(&p1); err != nil {
		t.Fatalf("criar pessoa 1: %v", err)
	}
	if err := st.db.QueryRow(`INSERT INTO pessoas (nome_guerra, nome_completo, grupo_id, status) VALUES ('BETA', 'Beta Fora', ?, 'ativo') RETURNING id`, gidFora).Scan(&pFora); err != nil {
		t.Fatalf("criar pessoa 2: %v", err)
	}

	return
}

func TestModificacoesUltimaAuditoriaEListagem(t *testing.T) {
	app, st, cleanup := setupTestApp(t)
	defer cleanup()

	gid, _, p1, _, _, _ := ondaPresencaBancoSetup(t, app, st)
	_ = gid
	ckGer := loginAs(t, app, "ger01_p", "senha-ger")

	// 1. Gerente lista /api/pessoas e cada item tem os campos ultima_mod_* (null no início)
	rr, res := doJSONReq(app, "GET", "/api/pessoas", nil, ckGer)
	if rr.Code != http.StatusOK {
		t.Fatalf("esperado 200 na listagem de pessoas, veio %d", rr.Code)
	}
	pessoas, ok := res["pessoas"].([]any)
	if !ok || len(pessoas) == 0 {
		t.Fatalf("esperado lista de pessoas, veio %v", res)
	}

	var pAlfa map[string]any
	for _, raw := range pessoas {
		pMap := raw.(map[string]any)
		if int64(pMap["id"].(float64)) == p1 {
			pAlfa = pMap
			break
		}
	}
	if pAlfa == nil {
		t.Fatalf("pessoa ALFA não encontrada na listagem")
	}
	if pAlfa["ultima_mod_por"] != nil {
		t.Fatalf("esperado ultima_mod_por null no início, veio %v", pAlfa["ultima_mod_por"])
	}
	if pAlfa["ultima_mod_em"] != nil {
		t.Fatalf("esperado ultima_mod_em null no início, veio %v", pAlfa["ultima_mod_em"])
	}

	// 2. Gerente edita pessoa (PATCH) → ultima_mod_em/por deixam de ser null
	patchBody := map[string]any{
		"nome_guerra":   "ALFA2",
		"nome_completo": "Alfa Editado",
		"status":        "ativo",
	}
	rrPatch, resPatch := doJSONReq(app, "PATCH", fmt.Sprintf("/api/pessoas/%d", p1), patchBody, ckGer)
	if rrPatch.Code != http.StatusOK {
		t.Fatalf("esperado 200 no PATCH de pessoas, veio %d (%v)", rrPatch.Code, resPatch["erro"])
	}

	rr2, res2 := doJSONReq(app, "GET", "/api/pessoas", nil, ckGer)
	if rr2.Code != http.StatusOK {
		t.Fatalf("esperado 200 na listagem de pessoas após patch, veio %d", rr2.Code)
	}
	pessoas2 := res2["pessoas"].([]any)
	var pAlfaEditado map[string]any
	for _, raw := range pessoas2 {
		pMap := raw.(map[string]any)
		if int64(pMap["id"].(float64)) == p1 {
			pAlfaEditado = pMap
			break
		}
	}
	if pAlfaEditado == nil {
		t.Fatalf("pessoa ALFA não encontrada após patch")
	}
	if pAlfaEditado["ultima_mod_por"] == nil || pAlfaEditado["ultima_mod_por"] == "" {
		t.Fatalf("esperado ultima_mod_por preenchido, veio %v", pAlfaEditado["ultima_mod_por"])
	}
	if pAlfaEditado["ultima_mod_em"] == nil || pAlfaEditado["ultima_mod_em"] == "" {
		t.Fatalf("esperado ultima_mod_em preenchido, veio %v", pAlfaEditado["ultima_mod_em"])
	}
	if pAlfaEditado["ultima_mod_por"] != "ger01_p" {
		t.Fatalf("esperado ultima_mod_por ger01_p, veio %v", pAlfaEditado["ultima_mod_por"])
	}
}

func TestApresentacaoFluxoEGuardas(t *testing.T) {
	app, st, cleanup := setupTestApp(t)
	defer cleanup()

	_, _, p1, pFora, _, _ := ondaPresencaBancoSetup(t, app, st)

	ckGer := loginAs(t, app, "ger01_p", "senha-ger")
	ckEnc := loginAs(t, app, "enc01_p", "senha-enc")
	ckAux := loginAs(t, app, "aux01_p", "senha-aux")
	ckChefe := loginAs(t, app, "chefe01_p", "senha-chefe")
	ckOp := loginAs(t, app, "op01_p", "senha-op")

	// 1. chefe_setor e operador tentam POST apresentacao → 403
	rr, _ := doJSONReq(app, "POST", fmt.Sprintf("/api/pessoas/%d/apresentacao", p1), map[string]any{"estado": "dispensado"}, ckChefe)
	if rr.Code != http.StatusForbidden {
		t.Fatalf("chefe_setor POST apresentacao deve dar 403, veio %d", rr.Code)
	}
	rr, _ = doJSONReq(app, "POST", fmt.Sprintf("/api/pessoas/%d/apresentacao", p1), map[string]any{"estado": "dispensado"}, ckOp)
	if rr.Code != http.StatusForbidden {
		t.Fatalf("operador POST apresentacao deve dar 403, veio %d", rr.Code)
	}

	// 2. estado inválido ("feriado") → 400
	rr, resErr := doJSONReq(app, "POST", fmt.Sprintf("/api/pessoas/%d/apresentacao", p1), map[string]any{"estado": "feriado"}, ckGer)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("estado feriado deve dar 400, veio %d (%v)", rr.Code, resErr)
	}

	// 3. pessoa de outro grupo → 403
	rr, _ = doJSONReq(app, "POST", fmt.Sprintf("/api/pessoas/%d/apresentacao", pFora), map[string]any{"estado": "dispensado"}, ckGer)
	if rr.Code != http.StatusForbidden {
		t.Fatalf("pessoa de outro grupo deve dar 403, veio %d", rr.Code)
	}

	// 4. gerente define "dispensado" com motivo → 200 + estado persistido (GET devolve)
	rr, resPost := doJSONReq(app, "POST", fmt.Sprintf("/api/pessoas/%d/apresentacao", p1), map[string]any{
		"estado": "dispensado",
		"motivo": "dispensa medica",
	}, ckGer)
	if rr.Code != http.StatusOK {
		t.Fatalf("gerente POST apresentacao deve dar 200, veio %d (%v)", rr.Code, resPost["erro"])
	}
	if resPost["ok"] != true || resPost["estado"] != "dispensado" {
		t.Fatalf("resposta POST inválida: %v", resPost)
	}
	if resPost["definido_em"] == nil || resPost["definido_em"] == "" {
		t.Fatalf("definido_em ausente na resposta: %v", resPost)
	}

	// GET devolve estado persistido
	rrGet, resGet := doJSONReq(app, "GET", "/api/pessoas/apresentacao", nil, ckGer)
	if rrGet.Code != http.StatusOK {
		t.Fatalf("GET apresentacao deve dar 200, veio %d", rrGet.Code)
	}
	aprs, ok := resGet["apresentacao"].([]any)
	if !ok || len(aprs) == 0 {
		t.Fatalf("esperado apresentacao com registros, veio %v", resGet)
	}
	var aprItem map[string]any
	for _, it := range aprs {
		m := it.(map[string]any)
		if int64(m["pessoa_id"].(float64)) == p1 {
			aprItem = m
			break
		}
	}
	if aprItem == nil {
		t.Fatalf("registro de apresentacao de p1 não retornado no GET")
	}
	if aprItem["estado"] != "dispensado" || aprItem["motivo"] != "dispensa medica" {
		t.Fatalf("campos de apresentacao incorretos: %v", aprItem)
	}
	if aprItem["definido_por_nome"] != "ger01_p" {
		t.Fatalf("definido_por_nome incorreto: %v", aprItem["definido_por_nome"])
	}

	// 5. encarregado (função) define estado → 200
	rr, resEnc := doJSONReq(app, "POST", fmt.Sprintf("/api/pessoas/%d/apresentacao", p1), map[string]any{
		"estado": "atrasado",
		"motivo": "transito",
	}, ckEnc)
	if rr.Code != http.StatusOK {
		t.Fatalf("encarregado define estado deve dar 200, veio %d (%v)", rr.Code, resEnc["erro"])
	}

	// 6. auxiliar também define estado → 200
	rr, resAux := doJSONReq(app, "POST", fmt.Sprintf("/api/pessoas/%d/apresentacao", p1), map[string]any{
		"estado": "presente",
		"motivo": "",
	}, ckAux)
	if rr.Code != http.StatusOK {
		t.Fatalf("auxiliar define estado deve dar 200, veio %d (%v)", rr.Code, resAux["erro"])
	}
}

func TestModificacoesHistoricoUnion(t *testing.T) {
	app, st, cleanup := setupTestApp(t)
	defer cleanup()

	_, _, p1, pFora, _, _ := ondaPresencaBancoSetup(t, app, st)
	ckGer := loginAs(t, app, "ger01_p", "senha-ger")
	ckGerFora := loginAs(t, app, "gerFora_p", "senha-gerf")

	// 1. Gerente faz PATCH na pessoa
	rrPatch, _ := doJSONReq(app, "PATCH", fmt.Sprintf("/api/pessoas/%d", p1), map[string]any{
		"nome_guerra":   "ALFA_MOD",
		"nome_completo": "Alfa Completo",
		"status":        "ativo",
	}, ckGer)
	if rrPatch.Code != http.StatusOK {
		t.Fatalf("PATCH falhou: %d", rrPatch.Code)
	}

	// 2. Gerente faz POST na apresentacao
	rrApr, _ := doJSONReq(app, "POST", fmt.Sprintf("/api/pessoas/%d/apresentacao", p1), map[string]any{
		"estado": "a serviço externo",
		"motivo": "missão oficial",
	}, ckGer)
	if rrApr.Code != http.StatusOK {
		t.Fatalf("POST apresentacao falhou: %d", rrApr.Code)
	}

	// 3. Simula alteração de presença histórica em presencas
	var uGerID int64
	_ = st.db.QueryRow(`SELECT id FROM usuarios WHERE login = 'ger01_p'`).Scan(&uGerID)
	// criar tipo de conf e conf para inserir presenca
	var tipoID, confID int64
	_ = st.db.QueryRow(`INSERT INTO conferencia_tipos (nome) VALUES ('Matinal Teste') RETURNING id`).Scan(&tipoID)
	_ = st.db.QueryRow(`INSERT INTO conferencias (data, tipo_id, criado_por) VALUES ('2026-10-07', ?, ?) RETURNING id`, tipoID, uGerID).Scan(&confID)
	_, err := st.db.Exec(`INSERT INTO presencas (conferencia_id, pessoa_id, situacao, marcado_por, alterado_por, alterado_em)
		VALUES (?, ?, 'atraso', ?, ?, '2026-10-07T12:00:00.000Z')`, confID, p1, uGerID, uGerID)
	if err != nil {
		t.Fatalf("inserir presenca alterada teste: %v", err)
	}

	// 4. GET /api/pessoas/{id}/modificacoes traz o PATCH, a apresentacao e a presenca alterada com quem fez
	rrMod, resMod := doJSONReq(app, "GET", fmt.Sprintf("/api/pessoas/%d/modificacoes", p1), nil, ckGer)
	if rrMod.Code != http.StatusOK {
		t.Fatalf("GET modificacoes deve dar 200, veio %d (%v)", rrMod.Code, resMod["erro"])
	}
	mods, ok := resMod["modificacoes"].([]any)
	if !ok || len(mods) < 3 {
		t.Fatalf("esperado ao menos 3 modificações, veio %d (%v)", len(mods), resMod)
	}

	var temAlterar, temApresentacao, temPresenca bool
	for _, raw := range mods {
		m := raw.(map[string]any)
		acao := fmt.Sprintf("%v", m["acao"])
		quem := fmt.Sprintf("%v", m["quem"])
		fonte := fmt.Sprintf("%v", m["fonte"])

		if acao == "alterar" && quem == "ger01_p" && fonte == "auditoria" {
			temAlterar = true
		}
		if acao == "apresentacao" && quem == "ger01_p" && fonte == "auditoria" {
			temApresentacao = true
		}
		if acao == "presença alterada: atraso" && quem == "ger01_p" && fonte == "presenca" {
			temPresenca = true
		}
	}

	if !temAlterar {
		t.Fatalf("modificações não incluiu o PATCH (ação alterar): %v", mods)
	}
	if !temApresentacao {
		t.Fatalf("modificações não incluiu a apresentacao: %v", mods)
	}
	if !temPresenca {
		t.Fatalf("modificações não incluiu a presenca alterada: %v", mods)
	}

	// 5. Usuário de outro grupo tentando acessar modificacoes de p1 → 403
	rrFora, _ := doJSONReq(app, "GET", fmt.Sprintf("/api/pessoas/%d/modificacoes", p1), nil, ckGerFora)
	if rrFora.Code != http.StatusForbidden {
		t.Fatalf("gerente de fora em modificacoes deve dar 403, veio %d", rrFora.Code)
	}

	// 6. Pessoa inexistente → 404
	rr404, _ := doJSONReq(app, "GET", "/api/pessoas/999999/modificacoes", nil, ckGer)
	if rr404.Code != http.StatusNotFound {
		t.Fatalf("pessoa inexistente deve dar 404, veio %d", rr404.Code)
	}
	_ = pFora
}
