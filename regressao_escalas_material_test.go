package main

// Regressão da rodada de liberação Escalas/Material (fixes do Sargento):
// P0 cross-group no turno, P1-2 update-sem-efeito, P1-1 escopo de cautela,
// P1-3 teto de anexo. Padrão de seeding do v1_test.go.

import (
	"fmt"
	"net/http"
	"strings"
	"testing"
)

func TestRegressaoTurnoCrossGroup(t *testing.T) {
	app, st, cleanup := setupTestApp(t)
	defer cleanup()
	admin := loginAs(t, app, "admin", "admin123")

	// Grupo A (vítima) com gerente e turno criado pelo gerente de A
	_, resA := doJSONReq(app, "POST", "/api/grupos", map[string]any{
		"nome": "G A Turno", "login": "ger_a_t", "senha": "senha12345", "nome_guerra": "GerAT",
	}, admin)
	if rrA := resA; false {
		_ = rrA
	}
	grupoA := int64(resA["id"].(float64))
	gerA := loginAs(t, app, "ger_a_t", "senha12345")

	// Grupo B (atacante)
	_, resB := doJSONReq(app, "POST", "/api/grupos", map[string]any{
		"nome": "G B Turno", "login": "ger_b_t", "senha": "senha12345", "nome_guerra": "GerBT",
	}, admin)
	grupoB := int64(resB["id"].(float64))
	_ = grupoB
	gerB := loginAs(t, app, "ger_b_t", "senha12345")

	// tipo de escala (seed global)
	var tipoID int64
	if err := st.db.QueryRow(`SELECT id FROM escala_tipos LIMIT 1`).Scan(&tipoID); err != nil {
		t.Fatalf("seed tipos: %v", err)
	}

	// pessoa sintética no grupo A para o efetivo do turno
	resPes, _ := st.db.Exec(`INSERT INTO pessoas (nome_guerra, nome_completo, grupo_id, status) VALUES ('PESSOA-A', 'Pessoa A', ?, 'ativo')`, grupoA)
	pesA, _ := resPes.LastInsertId()

	// gerente A cria turno no próprio grupo (sem grupo_id no corpo)
	payloadA := map[string]any{
		"tipo_id": tipoID, "data_inicio": "2026-11-01T08:00", "data_fim": "2026-11-01T20:00",
		"observacao": "turno legitimo A", "pessoas": []map[string]any{{"pessoa_id": pesA, "funcao_escala": "Cmd"}},
	}
	rrC, resC := doJSONReq(app, "POST", "/api/escalas/turnos", payloadA, gerA)
	if rrC.Code != http.StatusOK {
		t.Fatalf("criar turno A: %v", resC)
	}
	turnoA := int64(resC["id"].(float64))

	//gerente B tenta EDITAR o turno de A com grupo forjado = A no corpo (o P0)
	payloadForjado := map[string]any{
		"id": turnoA, "grupo_id": grupoA,
		"tipo_id": tipoID, "data_inicio": "2026-11-02T08:00", "data_fim": "2026-11-02T20:00",
		"observacao": "INVASAO", "pessoas": []map[string]any{},
	}
	rrX, _ := doJSONReq(app, "POST", "/api/escalas/turnos", payloadForjado, gerB)
	if rrX.Code == http.StatusOK {
		t.Fatalf("P0 VIVO: gerente B alterou turno de A com 200")
	}
	// prova de estado: o turno de A NÃO foi tocado nem zerei o efetivo dele
	var obs string
	var nPess int
	if err := st.db.QueryRow(`SELECT observacao FROM escala_turnos WHERE id = ?`, turnoA).Scan(&obs); err != nil {
		t.Fatalf("ler turno A: %v", err)
	}
	if err := st.db.QueryRow(`SELECT COUNT(*) FROM escala_pessoas WHERE turno_id = ?`, turnoA).Scan(&nPess); err != nil {
		t.Fatalf("ler efetivo: %v", err)
	}
	if obs != "turno legitimo A" || nPess != 1 {
		t.Fatalf("turno de A foi alterado: obs=%q pessoas=%d", obs, nPess)
	}

	// mesmo com grupo do corpo IGUAL ao próprio (grupoB), UPDATE sem match no escopo => 404
	payloadSelf := map[string]any{
		"id": turnoA, "grupo_id": grupoB,
		"tipo_id": tipoID, "data_inicio": "2026-11-03T08:00", "data_fim": "2026-11-03T20:00",
		"observacao": "x", "pessoas": []map[string]any{},
	}
	rrY, _ := doJSONReq(app, "POST", "/api/escalas/turnos", payloadSelf, gerB)
	if rrY.Code != http.StatusNotFound {
		t.Fatalf("update sem match deveria 404, obtido %d", rrY.Code)
	}

	// gerente A edita o próprio turno: 200 e estado muda
	payloadSelfA := map[string]any{
		"id": turnoA,
		"tipo_id": tipoID, "data_inicio": "2026-11-04T08:00", "data_fim": "2026-11-04T20:00",
		"observacao": "editado pelo dono", "pessoas": []map[string]any{{"pessoa_id": pesA, "funcao_escala": "Cmd"}},
	}
	rrO, resO := doJSONReq(app, "POST", "/api/escalas/turnos", payloadSelfA, gerA)
	if rrO.Code != http.StatusOK {
		t.Fatalf("dono deveria editar: %v", resO)
	}
	if err := st.db.QueryRow(`SELECT observacao FROM escala_turnos WHERE id = ?`, turnoA).Scan(&obs); err != nil || obs != "editado pelo dono" {
		t.Fatalf("dono não conseguiu editar: obs=%q err=%v", obs, err)
	}
}

func TestRegressaoCautelaEscopoETeto(t *testing.T) {
	app, st, cleanup := setupTestApp(t)
	defer cleanup()
	admin := loginAs(t, app, "admin", "admin123")

	_, resA := doJSONReq(app, "POST", "/api/grupos", map[string]any{
		"nome": "G Mat A", "login": "ger_ma", "senha": "senha12345", "nome_guerra": "GerMA",
	}, admin)
	grupoA := int64(resA["id"].(float64))
	gerA := loginAs(t, app, "ger_ma", "senha12345")
	_, resB := doJSONReq(app, "POST", "/api/grupos", map[string]any{
		"nome": "G Mat B", "login": "ger_mb", "senha": "senha12345", "nome_guerra": "GerMB",
	}, admin)
	_ = resB
	gerB := loginAs(t, app, "ger_mb", "senha12345")

	var catID int64
	if err := st.db.QueryRow(`SELECT id FROM material_categorias LIMIT 1`).Scan(&catID); err != nil {
		t.Fatalf("seed categorias: %v", err)
	}

	// item do grupo A (gerente cria; grupo do corpo ignorado p/ não-admin)
	rrI, resI := doJSONReq(app, "POST", "/api/material/itens", map[string]any{
		"categoria_id": catID, "nome": "Rádio A", "codigo_patrimonio": "RAD-A1",
	}, gerA)
	if rrI.Code != http.StatusOK {
		t.Fatalf("criar item: %v", resI)
	}
	itemID := int64(resI["id"].(float64))
	var itemGrupo int64
	if err := st.db.QueryRow(`SELECT grupo_id FROM material_itens WHERE id = ?`, itemID).Scan(&itemGrupo); err != nil || itemGrupo != grupoA {
		t.Fatalf("item deveria nascer no grupo do gerente (%d), veio %d (err %v)", grupoA, itemGrupo, err)
	}

	// gerente B tenta EDITAR item de A (P1-2): update-sem-match => 404
	rrE, _ := doJSONReq(app, "POST", "/api/material/itens", map[string]any{
		"id": itemID, "categoria_id": catID, "nome": "HACKEADO", "codigo_patrimonio": "RAD-A1",
	}, gerB)
	if rrE.Code != http.StatusNotFound {
		t.Fatalf("edição fora do escopo deveria 404, obtido %d", rrE.Code)
	}
	var nome string
	_ = st.db.QueryRow(`SELECT nome FROM material_itens WHERE id = ?`, itemID).Scan(&nome)
	if nome != "Rádio A" {
		t.Fatalf("item de A foi alterado: %q", nome)
	}

	// gerente B tenta CAUTELAR item de A (P1-1): 403
	rrCau, _ := doJSONReq(app, "POST", "/api/material/cautelar", map[string]any{
		"item_id": itemID, "pessoa_id": 1, "obs_saida": "x",
	}, gerB)
	if rrCau.Code != http.StatusForbidden {
		t.Fatalf("cautela fora do escopo deveria 403, obtido %d", rrCau.Code)
	}

	// pessoa sintética no grupo A para a cautela legítima
	resPesM, _ := st.db.Exec(`INSERT INTO pessoas (nome_guerra, nome_completo, grupo_id, status) VALUES ('PESSOA-MA', 'Pessoa MA', ?, 'ativo')`, grupoA)
	pesMA, _ := resPesM.LastInsertId()

	// gerente A cautela o próprio: 200
	rrCauA, resCauA := doJSONReq(app, "POST", "/api/material/cautelar", map[string]any{
		"item_id": itemID, "pessoa_id": pesMA, "obs_saida": "serviço",
	}, gerA)
	if rrCauA.Code != http.StatusOK {
		t.Fatalf("cautela legítima: %v", resCauA)
	}
	cautelaID := int64(resCauA["cautela_id"].(float64))

	// anexo acima do teto (P1-3): 413
	grande := strings.Repeat("QUJD", 250000) // ~1 MB de base64
	rrAnx, _ := doJSONReq(app, "POST", fmt.Sprintf("/api/material/cautelas/%d/anexos", cautelaID), map[string]any{
		"nome_arquivo": "g.pdf", "dados_base64": grande,
	}, gerA)
	if rrAnx.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("anexo acima do teto deveria 413, obtido %d", rrAnx.Code)
	}
}
