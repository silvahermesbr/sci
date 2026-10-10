package main

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
)

func TestPDFEndpointsV2(t *testing.T) {
	app, st, cleanup := setupTestApp(t)
	defer cleanup()

	adminCookie := loginAs(t, app, "admin", "admin123")

	// 1. Criar dados de teste (Grupo, Pessoa, Material, Cautela, Escala)
	resG, err := st.db.Exec(`INSERT INTO grupos (nome, codigo) VALUES ('1ª Companhia', 'CIA01')`)
	if err != nil {
		t.Fatalf("erro ao criar grupo: %v", err)
	}
	gid, _ := resG.LastInsertId()

	// v1.5.4-D2 (R-2): os PDFs de escalas/material/conferência passaram a exigir
	// o guard do módulo (reservaAuth/authMaterial/confPDFAuth — admin 403). O
	// leitor destes PDFs aqui é o GERENTE do grupo dono dos dados (grupo na
	// CONTA antes do login — a sessão sintetiza o papel com o grupo da conta).
	criaUsuarioTeste(t, st, "ger_pdfv2", "senha-ger-pdfv2", "gerente")
	if _, err := st.db.Exec(`UPDATE usuarios SET grupo_id = ? WHERE login = 'ger_pdfv2'`, gid); err != nil {
		t.Fatalf("grupo do gerente: %v", err)
	}
	gerCookie := loginAs(t, app, "ger_pdfv2", "senha-ger-pdfv2")

	resP, err := st.db.Exec(`INSERT INTO pessoas (nome_guerra, nome_completo, grupo_id, status) VALUES ('SILVA', 'Carlos Silva', ?, 'ativo')`, gid)
	if err != nil {
		t.Fatalf("erro ao criar pessoa: %v", err)
	}
	pid, _ := resP.LastInsertId()

	resCat, _ := st.db.Exec(`INSERT INTO material_categorias (nome, grupo_id) VALUES ('Armamento', ?)`, gid)
	catID, _ := resCat.LastInsertId()

	resItem, _ := st.db.Exec(`INSERT INTO material_itens (nome, codigo_patrimonio, categoria_id, grupo_id, status) VALUES ('Fuzil IA2 5.56', 'BR001', ?, ?, 'acautelado')`, catID, gid)
	itemID, _ := resItem.LastInsertId()

	resCaut, _ := st.db.Exec(`INSERT INTO material_cautelas (item_id, pessoa_id, responsavel_entrega_id, data_saida, status) VALUES (?, ?, 1, '2026-09-30T10:00:00Z', 'ativa')`, itemID, pid)
	cautID, _ := resCaut.LastInsertId()

	resTipo, _ := st.db.Exec(`INSERT INTO escala_tipos (nome, grupo_id) VALUES ('Oficial de Dia', ?)`, gid)
	tipoID, _ := resTipo.LastInsertId()

	resTurno, _ := st.db.Exec(`INSERT INTO escala_turnos (tipo_id, grupo_id, data_inicio, data_fim) VALUES (?, ?, '2026-09-30T08:00:00Z', '2026-10-01T08:00:00Z')`, tipoID, gid)
	turnoID, _ := resTurno.LastInsertId()

	_, _ = st.db.Exec(`INSERT INTO escala_pessoas (turno_id, pessoa_id, funcao_escala) VALUES (?, ?, 'Oficial de Dia')`, turnoID, pid)

	// 2. Testar Ficha Individual PDF (/api/pessoas/{id}/pdf)
	rr, _ := doRawReq(app, "GET", "/api/pessoas/1/pdf", nil, adminCookie)
	if rr.Code != http.StatusOK {
		t.Fatalf("esperado 200 em /api/pessoas/1/pdf, obtido: %d - %s", rr.Code, rr.Body.String())
	}
	if !bytes.HasPrefix(rr.Body.Bytes(), []byte("%PDF-1.")) {
		t.Fatalf("resposta de /api/pessoas/1/pdf não começa com %%PDF-1.")
	}

	// 3. Testar Recibo de Cautela PDF (/api/material/cautelas/{id}/recibo.pdf)
	rrCaut, _ := doRawReq(app, "GET", "/api/material/cautelas/1/recibo.pdf", nil, gerCookie)
	if rrCaut.Code != http.StatusOK {
		t.Fatalf("esperado 200 em /api/material/cautelas/1/recibo.pdf, obtido: %d - %s", rrCaut.Code, rrCaut.Body.String())
	}
	if !bytes.HasPrefix(rrCaut.Body.Bytes(), []byte("%PDF-1.")) {
		t.Fatalf("resposta de recibo de cautela não começa com %%PDF-1.")
	}

	// 4. Testar Escalas PDF (/api/escalas/pdf) — guard do módulo (reservaAuth)
	rrEsc, _ := doRawReq(app, "GET", "/api/escalas/pdf?mes=2026-09", nil, gerCookie)
	if rrEsc.Code != http.StatusOK {
		t.Fatalf("esperado 200 em /api/escalas/pdf, obtido: %d - %s", rrEsc.Code, rrEsc.Body.String())
	}
	if !bytes.HasPrefix(rrEsc.Body.Bytes(), []byte("%PDF-1.")) {
		t.Fatalf("resposta de escalas PDF não começa com %%PDF-1.")
	}

	// 5. Testar Inventário de Material PDF (/api/material/inventario/pdf)
	rrInv, _ := doRawReq(app, "GET", "/api/material/inventario/pdf", nil, gerCookie)
	if rrInv.Code != http.StatusOK {
		t.Fatalf("esperado 200 em /api/material/inventario/pdf, obtido: %d - %s", rrInv.Code, rrInv.Body.String())
	}
	if !bytes.HasPrefix(rrInv.Body.Bytes(), []byte("%PDF-1.")) {
		t.Fatalf("resposta de inventario PDF não começa com %%PDF-1.")
	}

	// 6. Testar Relatório Geral de Efetivo (/api/relatorio.pdf)
	rrRel, _ := doRawReq(app, "GET", "/api/relatorio.pdf?de=2026-09-01&ate=2026-09-30", nil, adminCookie)
	if rrRel.Code != http.StatusOK {
		t.Fatalf("esperado 200 em /api/relatorio.pdf, obtido: %d - %s", rrRel.Code, rrRel.Body.String())
	}
	if !bytes.HasPrefix(rrRel.Body.Bytes(), []byte("%PDF-1.")) {
		t.Fatalf("resposta de relatorio geral não começa com %%PDF-1.")
	}

	// 7. Testar Relatório de Conferência com Função (/api/conferencia/{id} e /api/conferencia/{id}/relatorio.pdf)
	resF, _ := st.db.Exec(`INSERT INTO funcoes (nome, grupo_id, ativo) VALUES ('Comandante de Pelotão', ?, 1)`, gid)
	fID, _ := resF.LastInsertId()
	_, _ = st.db.Exec(`UPDATE pessoas SET funcao_id = ? WHERE id = ?`, fID, pid)

	var tipoConfID int64
	_ = st.db.QueryRow(`SELECT id FROM conferencia_tipos LIMIT 1`).Scan(&tipoConfID)
	if tipoConfID == 0 {
		resTipo, _ := st.db.Exec(`INSERT INTO conferencia_tipos (nome) VALUES ('Conferência Diária')`)
		tipoConfID, _ = resTipo.LastInsertId()
	}

	resConf, err := st.db.Exec(`INSERT INTO conferencias (tipo_id, data, status, criado_por, grupo_id, fechada_em) VALUES (?, '2026-09-30', 'fechada', 1, ?, '2026-09-30T10:00:00Z')`, tipoConfID, gid)
	if err != nil {
		t.Fatalf("falha ao criar conferencia: %v", err)
	}
	confID, _ := resConf.LastInsertId()
	_, _ = st.db.Exec(`INSERT INTO presencas (conferencia_id, pessoa_id, situacao, marcado_por, verificado) VALUES (?, ?, 'presente', 1, 1)`, confID, pid)

	rrConfJSON, resConfJSON := doJSONReq(app, "GET", "/api/conferencia/"+strconv.FormatInt(confID, 10), nil, adminCookie)
	if rrConfJSON.Code != http.StatusOK {
		t.Fatalf("esperado 200 em /api/conferencia/%d, obtido %d", confID, rrConfJSON.Code)
	}
	lancamentos, ok := resConfJSON["lancamentos"].([]any)
	if !ok || len(lancamentos) == 0 {
		t.Fatalf("esperado lancamentos em /api/conferencia/%d, body: %s", confID, rrConfJSON.Body.String())
	}
	primeiro := lancamentos[0].(map[string]any)
	if primeiro["funcao"] != "Comandante de Pelotão" {
		t.Fatalf("esperado funcao 'Comandante de Pelotão', obtido %v", primeiro["funcao"])
	}

	rrConfPDF, _ := doRawReq(app, "GET", "/api/conferencia/"+strconv.FormatInt(confID, 10)+"/relatorio.pdf", nil, gerCookie)
	if rrConfPDF.Code != http.StatusOK {
		t.Fatalf("esperado 200 em /api/conferencia/%d/relatorio.pdf, obtido: %d - %s", confID, rrConfPDF.Code, rrConfPDF.Body.String())
	}
	if !bytes.HasPrefix(rrConfPDF.Body.Bytes(), []byte("%PDF-1.")) {
		t.Fatalf("resposta de conferencia PDF não começa com %%PDF-1.")
	}

	_ = cautID
	_ = turnoID
}

func doRawReq(app *App, method, path string, body io.Reader, cookie *http.Cookie) (*httptest.ResponseRecorder, []byte) {
	req := httptest.NewRequest(method, path, body)
	req.Header.Set("X-SCI", "1")
	if cookie != nil {
		req.AddCookie(cookie)
	}
	rr := httptest.NewRecorder()
	app.mux.ServeHTTP(rr, req)
	return rr, rr.Body.Bytes()
}
