package main

import (
	"bytes"
	"fmt"
	"net/http"
	"testing"
)

func setupDespachoFixture(t *testing.T, app *App, st *Store) (gid, s1, s2, s3, p1, p2, p3 int64) {
	t.Helper()
	if err := st.db.QueryRow(`INSERT INTO grupos (nome) VALUES ('Grupo Despacho Teste') RETURNING id`).Scan(&gid); err != nil {
		t.Fatalf("criar grupo: %v", err)
	}
	if err := st.db.QueryRow(`INSERT INTO setores (nome, sigla, grupo_id, ativo) VALUES ('Setor Alfa', 'SA', ?, 1) RETURNING id`, gid).Scan(&s1); err != nil {
		t.Fatalf("criar setor 1: %v", err)
	}
	if err := st.db.QueryRow(`INSERT INTO setores (nome, sigla, grupo_id, ativo) VALUES ('Setor Bravo', 'SB', ?, 1) RETURNING id`, gid).Scan(&s2); err != nil {
		t.Fatalf("criar setor 2: %v", err)
	}
	if err := st.db.QueryRow(`INSERT INTO setores (nome, sigla, grupo_id, ativo) VALUES ('Setor Charlie', 'SC', ?, 1) RETURNING id`, gid).Scan(&s3); err != nil {
		t.Fatalf("criar setor 3: %v", err)
	}

	criaUsuarioTeste(t, st, "ger_desp", "senha-ger", "gerente")
	criaUsuarioTeste(t, st, "adm_desp", "senha-adm", "admin")
	criaUsuarioTeste(t, st, "chefe_s1", "senha-c1", "chefe_setor")
	criaUsuarioTeste(t, st, "chefe_s2", "senha-c2", "chefe_setor")
	criaUsuarioTeste(t, st, "chefe_s3", "senha-c3", "chefe_setor")
	criaUsuarioTeste(t, st, "op_s1", "senha-op1", "operador")

	_, _ = st.db.Exec(`UPDATE usuarios SET grupo_id = ? WHERE login = 'ger_desp'`, gid)
	_, _ = st.db.Exec(`UPDATE usuarios SET grupo_id = ?, setor_id = ? WHERE login = 'chefe_s1'`, gid, s1)
	_, _ = st.db.Exec(`UPDATE usuarios SET grupo_id = ?, setor_id = ? WHERE login = 'chefe_s2'`, gid, s2)
	_, _ = st.db.Exec(`UPDATE usuarios SET grupo_id = ?, setor_id = ? WHERE login = 'chefe_s3'`, gid, s3)
	_, _ = st.db.Exec(`UPDATE usuarios SET grupo_id = ?, setor_id = ? WHERE login = 'op_s1'`, gid, s1)

	var u1, u2, u3 int64
	_ = st.db.QueryRow(`SELECT id FROM usuarios WHERE login = 'chefe_s1'`).Scan(&u1)
	_ = st.db.QueryRow(`SELECT id FROM usuarios WHERE login = 'chefe_s2'`).Scan(&u2)
	_ = st.db.QueryRow(`SELECT id FROM usuarios WHERE login = 'chefe_s3'`).Scan(&u3)

	_, _ = st.db.Exec(`INSERT INTO chefe_setores (usuario_id, setor_id, grupo_id) VALUES (?, ?, ?)`, u1, s1, gid)
	_, _ = st.db.Exec(`INSERT INTO chefe_setores (usuario_id, setor_id, grupo_id) VALUES (?, ?, ?)`, u2, s2, gid)
	_, _ = st.db.Exec(`INSERT INTO chefe_setores (usuario_id, setor_id, grupo_id) VALUES (?, ?, ?)`, u3, s3, gid)

	if err := st.db.QueryRow(`INSERT INTO pessoas (nome_guerra, nome_completo, grupo_id, setor_id, status) VALUES ('ALFA', 'Mil Alfa', ?, ?, 'ativo') RETURNING id`, gid, s1).Scan(&p1); err != nil {
		t.Fatalf("criar p1: %v", err)
	}
	if err := st.db.QueryRow(`INSERT INTO pessoas (nome_guerra, nome_completo, grupo_id, setor_id, status) VALUES ('BRAVO', 'Mil Bravo', ?, ?, 'ativo') RETURNING id`, gid, s2).Scan(&p2); err != nil {
		t.Fatalf("criar p2: %v", err)
	}
	if err := st.db.QueryRow(`INSERT INTO pessoas (nome_guerra, nome_completo, grupo_id, setor_id, status) VALUES ('CHARLIE', 'Mil Charlie', ?, ?, 'ativo') RETURNING id`, gid, s3).Scan(&p3); err != nil {
		t.Fatalf("criar p3: %v", err)
	}

	return
}

func TestDespachoGerenteDoisSetoresEVisualizacaoChefes(t *testing.T) {
	app, st, cleanup := setupTestApp(t)
	defer cleanup()
	_, s1, s2, _, _, _, _ := setupDespachoFixture(t, app, st)

	ckGer := loginAs(t, app, "ger_desp", "senha-ger")
	ckC1 := loginAs(t, app, "chefe_s1", "senha-c1")
	ckC3 := loginAs(t, app, "chefe_s3", "senha-c3")

	// 1. Gerente despacha 2 setores (s1 e s2)
	body := map[string]any{
		"nome":        "Conf Despacho Alfa e Bravo",
		"prazo_final": "17:30",
		"setores":     []int64{s1, s2},
	}
	rr, res := doJSONReq(app, "POST", "/api/conferencia/despachar", body, ckGer)
	if rr.Code != http.StatusOK {
		t.Fatalf("gerente despachar 2 setores esperado 200, veio %d: %v", rr.Code, res)
	}
	cidF, ok := res["conferencia_id"].(float64)
	if !ok || cidF <= 0 {
		t.Fatalf("conferencia_id inválido na resposta: %v", res)
	}
	cid := int64(cidF)
	if despF, ok := res["despachados"].(float64); !ok || int(despF) != 2 {
		t.Fatalf("esperado despachados = 2, veio %v", res["despachados"])
	}

	// Verifica se nasceu aberta e com linhas em conferencia_despachos
	var status string
	if err := st.db.QueryRow(`SELECT status FROM conferencias WHERE id = ?`, cid).Scan(&status); err != nil || status != "aberta" {
		t.Fatalf("conferência não nasceu aberta: status=%s, err=%v", status, err)
	}
	var countDesp int
	_ = st.db.QueryRow(`SELECT COUNT(*) FROM conferencia_despachos WHERE conferencia_id = ?`, cid).Scan(&countDesp)
	if countDesp != 2 {
		t.Fatalf("esperado 2 linhas em conferencia_despachos, obteve %d", countDesp)
	}

	// 2. Chefe de um dos setores despachados (s1) -> GET /hoje mostra só o setor dele + pct_conferido
	rrH1, resH1 := doJSONReq(app, "GET", "/api/conferencia/hoje?id="+fmt.Sprintf("%d", cid), nil, ckC1)
	if rrH1.Code != http.StatusOK {
		t.Fatalf("chefe s1 GET /hoje esperado 200, veio %d: %v", rrH1.Code, resH1)
	}
	if resH1["conferencia"] == nil {
		t.Fatalf("chefe s1 deveria ver conferência, mas conferencia é null")
	}
	stStatus, ok := resH1["setores_status"].([]any)
	if !ok || len(stStatus) != 1 {
		t.Fatalf("chefe s1 deveria ver apenas 1 setor despachado, viu %d (%v)", len(stStatus), resH1["setores_status"])
	}
	item0 := stStatus[0].(map[string]any)
	if int64(item0["setor_id"].(float64)) != s1 {
		t.Fatalf("esperado setor %d, veio %v", s1, item0["setor_id"])
	}
	if item0["pct_conferido"] == nil {
		t.Fatalf("pct_conferido não encontrado no item do setor: %v", item0)
	}
	if desp, ok := item0["despachado"].(bool); !ok || !desp {
		t.Fatalf("despachado deveria ser true, veio %v", item0["despachado"])
	}
	if resH1["total_setores"] == nil || resH1["total_banco"] == nil {
		t.Fatalf("campos de topo total_setores/total_banco ausentes: %v", resH1)
	}

	// 3. Chefe de setor FORA do despacho (s3) -> conferencia: null
	rrH3, resH3 := doJSONReq(app, "GET", "/api/conferencia/hoje?id="+fmt.Sprintf("%d", cid), nil, ckC3)
	if rrH3.Code != http.StatusOK {
		t.Fatalf("chefe s3 GET /hoje esperado 200, veio %d: %v", rrH3.Code, resH3)
	}
	if resH3["conferencia"] != nil {
		t.Fatalf("chefe fora do despacho deveria receber conferencia: null, obteve: %v", resH3["conferencia"])
	}
	if st3, ok := resH3["setores_status"].([]any); !ok || len(st3) != 0 {
		t.Fatalf("chefe fora do despacho deveria receber setores_status vazio, obteve %d itens", len(st3))
	}
	if p3Arr, ok := resH3["pessoas"].([]any); !ok || len(p3Arr) != 0 {
		t.Fatalf("chefe fora do despacho deveria receber pessoas vazia, obteve %d itens", len(p3Arr))
	}
	if resH3["total_banco"] == nil {
		t.Fatalf("total_banco deveria vir calculado no funil vazio")
	}
}

func TestDespachoChefeIniciaProprioSetor(t *testing.T) {
	app, st, cleanup := setupTestApp(t)
	defer cleanup()
	_, s1, _, _, _, _, _ := setupDespachoFixture(t, app, st)

	ckC1 := loginAs(t, app, "chefe_s1", "senha-c1")

	// Chefe inicia conferência do próprio setor (iniciar com setores: [s1])
	body := map[string]any{
		"nome":    "Conf Próprio Setor Chefe 1",
		"setores": []int64{s1},
	}
	rr, res := doJSONReq(app, "POST", "/api/conferencia/iniciar", body, ckC1)
	if rr.Code != http.StatusOK {
		t.Fatalf("chefe iniciando próprio setor esperado 200, veio %d: %v", rr.Code, res)
	}
	cidF, ok := res["id"].(float64)
	if !ok || cidF <= 0 {
		cidF, ok = res["conferencia_id"].(float64)
	}
	if !ok || cidF <= 0 {
		t.Fatalf("id da conferência não retornado: %v", res)
	}
	cid := int64(cidF)

	// Verifica se a linha de despacho foi criada
	var despCount int
	_ = st.db.QueryRow(`SELECT COUNT(*) FROM conferencia_despachos WHERE conferencia_id = ? AND setor_id = ?`, cid, s1).Scan(&despCount)
	if despCount != 1 {
		t.Fatalf("despacho do setor %d não foi criado para conferência %d", s1, cid)
	}
}

func TestDespachoOperadorChefeSetorAlheio(t *testing.T) {
	app, st, cleanup := setupTestApp(t)
	defer cleanup()
	_, _, s2, _, _, _, _ := setupDespachoFixture(t, app, st)

	ckC1 := loginAs(t, app, "chefe_s1", "senha-c1")
	ckOp1 := loginAs(t, app, "op_s1", "senha-op1")

	// 1. Chefe do setor 1 tenta iniciar para setor 2 -> 403
	bodyC := map[string]any{
		"nome":    "Tentativa Chefe Alheio",
		"setores": []int64{s2},
	}
	rrC, resC := doJSONReq(app, "POST", "/api/conferencia/iniciar", bodyC, ckC1)
	if rrC.Code != http.StatusForbidden {
		t.Fatalf("chefe iniciando setor alheio esperado 403, obteve %d: %v", rrC.Code, resC)
	}

	// 2. Operador do setor 1 tenta iniciar para setor 2 -> 403
	bodyOp := map[string]any{
		"nome":    "Tentativa Operador Alheio",
		"setores": []int64{s2},
	}
	rrOp, resOp := doJSONReq(app, "POST", "/api/conferencia/iniciar", bodyOp, ckOp1)
	if rrOp.Code != http.StatusForbidden {
		t.Fatalf("operador iniciando setor alheio esperado 403, obteve %d: %v", rrOp.Code, resOp)
	}
}

func TestDespachoValidacoesGerenteEAdmin(t *testing.T) {
	app, st, cleanup := setupTestApp(t)
	defer cleanup()
	_, s1, _, _, _, _, _ := setupDespachoFixture(t, app, st)

	ckGer := loginAs(t, app, "ger_desp", "senha-ger")
	ckAdm := loginAs(t, app, "adm_desp", "senha-adm")

	// 1. Gerente despacha sem setores (array vazio) -> 400
	rr1, _ := doJSONReq(app, "POST", "/api/conferencia/despachar", map[string]any{
		"nome":    "Sem Setores Vazio",
		"setores": []int64{},
	}, ckGer)
	if rr1.Code != http.StatusBadRequest {
		t.Fatalf("despachar com setores vazio esperado 400, veio %d", rr1.Code)
	}

	// 2. Gerente despacha sem o campo setores -> 400
	rr2, _ := doJSONReq(app, "POST", "/api/conferencia/despachar", map[string]any{
		"nome": "Sem Campo Setores",
	}, ckGer)
	if rr2.Code != http.StatusBadRequest {
		t.Fatalf("despachar sem campo setores esperado 400, veio %d", rr2.Code)
	}

	// 3. Admin despacha -> 403
	rrAdm, _ := doJSONReq(app, "POST", "/api/conferencia/despachar", map[string]any{
		"nome":    "Admin Despachando",
		"setores": []int64{s1},
	}, ckAdm)
	if rrAdm.Code != http.StatusForbidden {
		t.Fatalf("admin despachar esperado 403, veio %d", rrAdm.Code)
	}
}

func TestDespachoPDFComFiltroAtrasos(t *testing.T) {
	app, st, cleanup := setupTestApp(t)
	defer cleanup()
	_, _, _, _, p1, p2, _ := setupDespachoFixture(t, app, st)

	ckGer := loginAs(t, app, "ger_desp", "senha-ger")

	// 1. Inicia conferência
	rrIni, resIni := doJSONReq(app, "POST", "/api/conferencia/iniciar", map[string]any{"nome": "Conf PDF Atrasos"}, ckGer)
	if rrIni.Code != http.StatusOK {
		t.Fatalf("iniciar conf para PDF esperado 200, veio %d", rrIni.Code)
	}
	cid := int64(resIni["id"].(float64))

	// 2. Fecha conferência com 1 presente e 1 atraso
	bodyFechar := map[string]any{
		"id": cid,
		"lancamentos": []map[string]any{
			{"pessoa_id": p1, "situacao": "presente"},
			{"pessoa_id": p2, "situacao": "atraso"},
		},
	}
	rrF, resF := doJSONReq(app, "POST", "/api/conferencia/fechar", bodyFechar, ckGer)
	if rrF.Code != http.StatusOK {
		t.Fatalf("fechar conf esperado 200, veio %d: %v", rrF.Code, resF)
	}

	// 3. GET /relatorio.pdf?filtro=atrasos
	urlPDF := fmt.Sprintf("/api/conferencia/%d/relatorio.pdf?filtro=atrasos", cid)
	rrPDF := doRawReqH(app, "GET", urlPDF, nil, "", ckGer)
	if rrPDF.Code != http.StatusOK {
		t.Fatalf("GET PDF atrasos esperado 200, veio %d: %s", rrPDF.Code, rrPDF.Body.String())
	}
	ct := rrPDF.Header().Get("Content-Type")
	if ct != "application/pdf" {
		t.Fatalf("esperado Content-Type application/pdf, veio %s", ct)
	}
	bodyBytes := rrPDF.Body.Bytes()
	if !bytes.HasPrefix(bodyBytes, []byte("%PDF-")) {
		t.Fatalf("corpo do relatório não começa com %%PDF-")
	}
}

func TestDespachoRegressaoSemDespachos(t *testing.T) {
	app, st, cleanup := setupTestApp(t)
	defer cleanup()
	setupDespachoFixture(t, app, st)

	ckGer := loginAs(t, app, "ger_desp", "senha-ger")
	ckC1 := loginAs(t, app, "chefe_s1", "senha-c1")

	// Gerente inicia conferência SEM setores (fluxo aberto a todo o grupo)
	rrIni, resIni := doJSONReq(app, "POST", "/api/conferencia/iniciar", map[string]any{"nome": "Conf Global Aberta"}, ckGer)
	if rrIni.Code != http.StatusOK {
		t.Fatalf("iniciar conferência sem setores esperado 200, veio %d", rrIni.Code)
	}
	cid := int64(resIni["id"].(float64))

	// Chefe s1 acessa /hoje: conferência deve estar visível e trazer todos os setores
	rrH, resH := doJSONReq(app, "GET", "/api/conferencia/hoje?id="+fmt.Sprintf("%d", cid), nil, ckC1)
	if rrH.Code != http.StatusOK {
		t.Fatalf("chefe s1 GET /hoje esperado 200, veio %d", rrH.Code)
	}
	if resH["conferencia"] == nil {
		t.Fatalf("conferência sem despachos DEVE ser visível para chefe, mas veio null")
	}
	stArr, ok := resH["setores_status"].([]any)
	if !ok || len(stArr) < 3 {
		t.Fatalf("esperado ver todos os 3 setores na conferência sem despacho, veio %d", len(stArr))
	}
	// Verifica se despachado é false para setores de conferência sem despacho
	for _, it := range stArr {
		item := it.(map[string]any)
		if desp, _ := item["despachado"].(bool); desp {
			t.Fatalf("setor em conferência sem despacho não deveria estar despachado: %v", item)
		}
	}
}
