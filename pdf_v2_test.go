package main

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
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
	rrCaut, _ := doRawReq(app, "GET", "/api/material/cautelas/1/recibo.pdf", nil, adminCookie)
	if rrCaut.Code != http.StatusOK {
		t.Fatalf("esperado 200 em /api/material/cautelas/1/recibo.pdf, obtido: %d - %s", rrCaut.Code, rrCaut.Body.String())
	}
	if !bytes.HasPrefix(rrCaut.Body.Bytes(), []byte("%PDF-1.")) {
		t.Fatalf("resposta de recibo de cautela não começa com %%PDF-1.")
	}

	// 4. Testar Escalas PDF (/api/escalas/pdf)
	rrEsc, _ := doRawReq(app, "GET", "/api/escalas/pdf?mes=2026-09", nil, adminCookie)
	if rrEsc.Code != http.StatusOK {
		t.Fatalf("esperado 200 em /api/escalas/pdf, obtido: %d - %s", rrEsc.Code, rrEsc.Body.String())
	}
	if !bytes.HasPrefix(rrEsc.Body.Bytes(), []byte("%PDF-1.")) {
		t.Fatalf("resposta de escalas PDF não começa com %%PDF-1.")
	}

	// 5. Testar Inventário de Material PDF (/api/material/inventario/pdf)
	rrInv, _ := doRawReq(app, "GET", "/api/material/inventario/pdf", nil, adminCookie)
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
