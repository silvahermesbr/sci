package main

// Regressão dos fixes P1 da onda v1.5 (rodada 04/10/26):
//   P1-1 anexos de material: allowlist de MIME no upload + defesa em profundidade
//        no download (octet-stream + attachment para mime legado hostil, nunca inline);
//   P1-2 sugestões de setor: efeito revalida escopo/allowlist na APLICAÇÃO (tx única),
//        TOCTOU morto (WHERE status='pendente' + RowsAffected), allowlist de tipo_acao
//        na entrada;
//   P1-3 delegar turno: RowsAffected==0 → 404 (200 fantasma morto);
//   P1-4 cobertura das rotas novas: DELETE modelos (destrutiva), GET minhas, GET modelos.
// Padrões da suíte: setupTestApp/loginAs/doJSONReq; 2 grupos reais p/ escopo;
// tríade de escopo: forjado recusado + vítima intacta + dono edita normal.

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// criaGrupo cria grupo novo (nasce COM gerente) e devolve id do grupo + cookie do gerente.
func criaGrupo(t *testing.T, app *App, admin *http.Cookie, nome, login string) (int64, *http.Cookie) {
	t.Helper()
	_, res := doJSONReq(app, "POST", "/api/grupos", map[string]any{
		"nome": nome, "login": login, "senha": "senha12345", "nome_guerra": strings.ToUpper(login),
	}, admin)
	grupoID, _ := res["id"].(float64)
	if grupoID == 0 {
		t.Fatalf("grupo %s não criado: %v", nome, res)
	}
	return int64(grupoID), loginAs(t, app, login, "senha12345")
}

func TestP1DelegarTurnoInexistente(t *testing.T) {
	app, st, cleanup := setupTestApp(t)
	defer cleanup()
	admin := loginAs(t, app, "admin", "admin123")
	grupoA, gerA := criaGrupo(t, app, admin, "G Del Fantasma", "ger_fant")
	grupoA1, _ := criaGrupo(t, app, admin, "G Del Fantasma Sub", "ger_fant_sub")
	// A1 subordinado de A (vínculo bilateral ativo) — senão o 403 de subordinação
	// dispara ANTES do 404 de turno inexistente.
	if _, err := st.db.Exec(`INSERT INTO grupo_vinculos (superior_id, subordinado_id, criado_por_superior, criado_por_subordinado)
		VALUES (?, ?, 1, 1)`, grupoA, grupoA1); err != nil {
		t.Fatalf("vínculo A→A1: %v", err)
	}

	rr, res := doJSONReq(app, "POST", "/api/escalas/turnos/999999/delegar",
		map[string]any{"grupo_delegado_id": grupoA1}, gerA)
	if rr.Code != http.StatusNotFound {
		t.Fatalf("delegar turno inexistente: esperado 404, veio %d (%v)", rr.Code, res)
	}
}

func TestP1DelegarTurnoCrossGroup(t *testing.T) {
	app, st, cleanup := setupTestApp(t)
	defer cleanup()
	admin := loginAs(t, app, "admin", "admin123")
	grupoA, gerA := criaGrupo(t, app, admin, "G Del A", "ger_del_a")
	grupoA1, _ := criaGrupo(t, app, admin, "G Del A Sub", "ger_del_a_sub")
	grupoB, gerB := criaGrupo(t, app, admin, "G Del B", "ger_del_b")
	grupoB1, _ := criaGrupo(t, app, admin, "G Del B Sub", "ger_del_b_sub")
	for _, v := range [][2]int64{{grupoA, grupoA1}, {grupoB, grupoB1}} {
		if _, err := st.db.Exec(`INSERT INTO grupo_vinculos (superior_id, subordinado_id, criado_por_superior, criado_por_subordinado)
			VALUES (?, ?, 1, 1)`, v[0], v[1]); err != nil {
			t.Fatalf("vínculo %d→%d: %v", v[0], v[1], err)
		}
	}

	var tipoID int64
	if err := st.db.QueryRow(`SELECT id FROM escala_tipos LIMIT 1`).Scan(&tipoID); err != nil {
		t.Fatalf("seed tipos: %v", err)
	}
	resPes, err := st.db.Exec(`INSERT INTO pessoas (nome_guerra, nome_completo, grupo_id, status)
		VALUES ('DEL-A', 'Del A', ?, 'ativo')`, grupoA)
	if err != nil {
		t.Fatalf("pessoa A: %v", err)
	}
	pesA, _ := resPes.LastInsertId()

	rrC, resC := doJSONReq(app, "POST", "/api/escalas/turnos", map[string]any{
		"tipo_id": tipoID, "data_inicio": "2026-11-01T08:00", "data_fim": "2026-11-01T20:00",
		"pessoas": []map[string]any{{"pessoa_id": pesA, "funcao_escala": "Cmd"}},
	}, gerA)
	if rrC.Code != http.StatusOK {
		t.Fatalf("criar turno A: %d (%v)", rrC.Code, resC)
	}
	turnoA := int64(resC["id"].(float64))

	// Gerente B delega o turno de A para subordinado legítimo SEU (B1): a checagem de
	// subordinação passa, mas o UPDATE com WHERE grupo_id=B não casa → 404 honesto
	// (antes: 200 fantasma), turno vítima intacto.
	rr, res := doJSONReq(app, "POST", fmt.Sprintf("/api/escalas/turnos/%d/delegar", turnoA),
		map[string]any{"grupo_delegado_id": grupoB1}, gerB)
	if rr.Code != http.StatusNotFound {
		t.Fatalf("delegar cross-group: esperado 404, veio %d (%v)", rr.Code, res)
	}
	var gid int64
	var delegado *int64
	var statusDel string
	if err := st.db.QueryRow(`SELECT grupo_id, grupo_delegado_id, status_delegacao
		FROM escala_turnos WHERE id = ?`, turnoA).Scan(&gid, &delegado, &statusDel); err != nil {
		t.Fatalf("ler turno: %v", err)
	}
	if gid != grupoA || delegado != nil || statusDel == "delegado" {
		t.Fatalf("turno vítima ALTERADO: grupo=%d delegado=%v status=%s", gid, delegado, statusDel)
	}

	// Dono honesto delega para o próprio subordinado (triade: forjado recusado +
	// vítima intacta + dono edita).
	rrD, resD := doJSONReq(app, "POST", fmt.Sprintf("/api/escalas/turnos/%d/delegar", turnoA),
		map[string]any{"grupo_delegado_id": grupoA1}, gerA)
	if rrD.Code != http.StatusOK {
		t.Fatalf("dono delega próprio turno: %d (%v)", rrD.Code, resD)
	}
}

func TestP1SugestaoCrossGroup(t *testing.T) {
	app, st, cleanup := setupTestApp(t)
	defer cleanup()
	admin := loginAs(t, app, "admin", "admin123")
	grupoA, _ := criaGrupo(t, app, admin, "G Sug A", "ger_sug_a")
	grupoB, gerB := criaGrupo(t, app, admin, "G Sug B", "ger_sug_b")

	resVit, err := st.db.Exec(`INSERT INTO pessoas (nome_guerra, nome_completo, grupo_id, status)
		VALUES ('VITIMA', 'Vítima Cross', ?, 'ativo')`, grupoA)
	if err != nil {
		t.Fatalf("pessoa vítima: %v", err)
	}
	vitimaID, _ := resVit.LastInsertId()
	resAlg, err := st.db.Exec(`INSERT INTO pessoas (nome_guerra, nome_completo, grupo_id, status)
		VALUES ('ALIADO', 'Aliado B', ?, 'ativo')`, grupoB)
	if err != nil {
		t.Fatalf("pessoa aliada: %v", err)
	}
	aliadoID, _ := resAlg.LastInsertId()

	// Sugestão criada honestamente no grupo B…
	rrAdd, resAdd := doJSONReq(app, "POST", "/api/setores/sugestoes", map[string]any{
		"setor_tipo": "pessoal", "tipo_acao": "alterar_status_militar",
		"dados": map[string]any{"pessoa_id": aliadoID, "novo_status": "inativo"},
	}, gerB)
	if rrAdd.Code != http.StatusOK {
		t.Fatalf("criar sugestão: %d (%v)", rrAdd.Code, resAdd)
	}
	sugB := int64(resAdd["id"].(float64))
	// …mas o chefe do B forja dados_json apontando para a vítima do grupo A.
	if _, err := st.db.Exec(`UPDATE setor_sugestoes SET dados_json = ? WHERE id = ?`,
		fmt.Sprintf(`{"pessoa_id":%d,"novo_status":"inativo"}`, vitimaID), sugB); err != nil {
		t.Fatalf("forjar dados_json: %v", err)
	}

	rr, res := doJSONReq(app, "POST", fmt.Sprintf("/api/setores/sugestoes/%d/avaliar", sugB),
		map[string]any{"acao": "aprovar"}, gerB)
	if rr.Code != http.StatusConflict && rr.Code != http.StatusForbidden {
		t.Fatalf("aprovar cross-group: esperado 409/403, veio %d (%v)", rr.Code, res)
	}
	var status string
	if err := st.db.QueryRow(`SELECT status FROM pessoas WHERE id = ?`, vitimaID).Scan(&status); err != nil {
		t.Fatalf("ler vítima: %v", err)
	}
	if status != "ativo" {
		t.Fatalf("vítima ALTERADA: status=%s", status)
	}
	// Sugestão NÃO vira aprovada em falha de efeito.
	var sugStatus string
	if err := st.db.QueryRow(`SELECT status FROM setor_sugestoes WHERE id = ?`, sugB).Scan(&sugStatus); err != nil {
		t.Fatalf("ler sugestão: %v", err)
	}
	if sugStatus != "pendente" {
		t.Fatalf("sugestão virou %s em falha de efeito", sugStatus)
	}
}

func TestP1SugestaoStatusInvalido(t *testing.T) {
	app, st, cleanup := setupTestApp(t)
	defer cleanup()
	admin := loginAs(t, app, "admin", "admin123")
	grupoA, gerA := criaGrupo(t, app, admin, "G Sug Inv", "ger_sug_inv")

	resPes, err := st.db.Exec(`INSERT INTO pessoas (nome_guerra, nome_completo, grupo_id, status)
		VALUES ('INV', 'Inválido', ?, 'ativo')`, grupoA)
	if err != nil {
		t.Fatalf("pessoa: %v", err)
	}
	pesID, _ := resPes.LastInsertId()

	rrAdd, resAdd := doJSONReq(app, "POST", "/api/setores/sugestoes", map[string]any{
		"setor_tipo": "pessoal", "tipo_acao": "alterar_status_militar",
		"dados": map[string]any{"pessoa_id": pesID, "novo_status": "xyz"},
	}, gerA)
	if rrAdd.Code != http.StatusOK {
		t.Fatalf("criar sugestão xyz: %d (%v)", rrAdd.Code, resAdd)
	}
	sug := int64(resAdd["id"].(float64))

	rr, res := doJSONReq(app, "POST", fmt.Sprintf("/api/setores/sugestoes/%d/avaliar", sug),
		map[string]any{"acao": "aprovar"}, gerA)
	if rr.Code != http.StatusConflict {
		t.Fatalf("aprovar novo_status 'xyz': esperado 409, veio %d (%v)", rr.Code, res)
	}
	var status string
	if err := st.db.QueryRow(`SELECT status FROM pessoas WHERE id = ?`, pesID).Scan(&status); err != nil {
		t.Fatalf("ler pessoa: %v", err)
	}
	if status != "ativo" {
		t.Fatalf("pessoa ALTERADA por status inválido: status=%s", status)
	}
}

func TestP1SugestaoHonestoFunciona(t *testing.T) {
	app, st, cleanup := setupTestApp(t)
	defer cleanup()
	admin := loginAs(t, app, "admin", "admin123")
	grupoA, gerA := criaGrupo(t, app, admin, "G Sug Hon", "ger_sug_hon")

	resPes, err := st.db.Exec(`INSERT INTO pessoas (nome_guerra, nome_completo, grupo_id, status)
		VALUES ('HON', 'Honesto', ?, 'ativo')`, grupoA)
	if err != nil {
		t.Fatalf("pessoa: %v", err)
	}
	pesID, _ := resPes.LastInsertId()

	rrAdd, resAdd := doJSONReq(app, "POST", "/api/setores/sugestoes", map[string]any{
		"setor_tipo": "pessoal", "tipo_acao": "alterar_status_militar",
		"dados": map[string]any{"pessoa_id": pesID, "novo_status": "inativo"},
	}, gerA)
	if rrAdd.Code != http.StatusOK {
		t.Fatalf("criar sugestão honesta: %d (%v)", rrAdd.Code, resAdd)
	}
	sug := int64(resAdd["id"].(float64))

	rr, res := doJSONReq(app, "POST", fmt.Sprintf("/api/setores/sugestoes/%d/avaliar", sug),
		map[string]any{"acao": "aprovar"}, gerA)
	if rr.Code != http.StatusOK {
		t.Fatalf("aprovar honesta: %d (%v)", rr.Code, res)
	}
	var status string
	if err := st.db.QueryRow(`SELECT status FROM pessoas WHERE id = ?`, pesID).Scan(&status); err != nil {
		t.Fatalf("ler pessoa: %v", err)
	}
	if status != "inativo" {
		t.Fatalf("efeito honesto não aplicado: status=%s", status)
	}
	var sugStatus string
	if err := st.db.QueryRow(`SELECT status FROM setor_sugestoes WHERE id = ?`, sug).Scan(&sugStatus); err != nil {
		t.Fatalf("ler sugestão: %v", err)
	}
	if sugStatus != "aprovado" {
		t.Fatalf("sugestão honesta deveria estar aprovada, está %s", sugStatus)
	}
}

func TestP1SugestaoTipoAcaoInvalido(t *testing.T) {
	app, _, cleanup := setupTestApp(t)
	defer cleanup()
	admin := loginAs(t, app, "admin", "admin123")
	_, gerA := criaGrupo(t, app, admin, "G Sug Tipo", "ger_sug_tipo")

	rr, res := doJSONReq(app, "POST", "/api/setores/sugestoes", map[string]any{
		"setor_tipo": "pessoal", "tipo_acao": "excluir_tudo",
		"dados":      map[string]any{"pessoa_id": 1, "novo_status": "inativo"},
	}, gerA)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("tipo_acao fora da allowlist: esperado 400, veio %d (%v)", rr.Code, res)
	}
}

func TestP1AnexoMimeHostil(t *testing.T) {
	app, st, cleanup := setupTestApp(t)
	defer cleanup()
	admin := loginAs(t, app, "admin", "admin123")
	grupoA, gerA := criaGrupo(t, app, admin, "G Anexo", "ger_anexo")

	var adminID int64
	if err := st.db.QueryRow(`SELECT id FROM usuarios WHERE login='admin'`).Scan(&adminID); err != nil {
		t.Fatalf("admin user: %v", err)
	}
	resItem, err := st.db.Exec(`INSERT INTO material_itens (grupo_id, nome, codigo_patrimonio, status, nivel_sensibilidade)
		VALUES (?, 'Rádio X', 'PX-1', 'disponivel', 'padrao')`, grupoA)
	if err != nil {
		t.Fatalf("item: %v", err)
	}
	itemID, _ := resItem.LastInsertId()
	resPes, err := st.db.Exec(`INSERT INTO pessoas (nome_guerra, nome_completo, grupo_id, status)
		VALUES ('CAUT', 'Cautelado', ?, 'ativo')`, grupoA)
	if err != nil {
		t.Fatalf("pessoa cautela: %v", err)
	}
	pesID, _ := resPes.LastInsertId()
	resCaut, err := st.db.Exec(`INSERT INTO material_cautelas (item_id, pessoa_id, responsavel_entrega_id, data_saida, status)
		VALUES (?, ?, ?, '2026-11-01', 'ativa')`, itemID, pesID, adminID)
	if err != nil {
		t.Fatalf("cautela: %v", err)
	}
	cautelaID, _ := resCaut.LastInsertId()

	// (t6) upload text/html → 400
	rr, res := doJSONReq(app, "POST", fmt.Sprintf("/api/material/cautelas/%d/anexos", cautelaID), map[string]any{
		"nome_arquivo": "pag.html", "tipo_mime": "text/html",
		"tamanho": 20, "dados_base64": "PGh0bWw+PHNjcmlwdD48L3NjcmlwdD48L2h0bWw+",
	}, gerA)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("upload text/html: esperado 400, veio %d (%v)", rr.Code, res)
	}

	// (t7) anexo LEGADO com mime hostil gravado direto no banco → download serve
	// attachment + octet-stream, nunca inline/html.
	conteudo := []byte("<html><script>alert(1)</script></html>")
	b64 := base64.StdEncoding.EncodeToString(conteudo)
	if _, err := st.db.Exec(`INSERT INTO material_cautela_anexos (cautela_id, nome_arquivo, tipo_mime, tamanho, dados_base64)
		VALUES (?, "../../evil.html", "text/html", ?, ?)`, cautelaID, len(conteudo), b64); err != nil {
		t.Fatalf("insert legado: %v", err)
	}
	var anexoID int64
	if err := st.db.QueryRow(`SELECT MAX(id) FROM material_cautela_anexos`).Scan(&anexoID); err != nil {
		t.Fatalf("ler anexo: %v", err)
	}

	req := httptest.NewRequest("GET", fmt.Sprintf("/api/material/anexos/%d", anexoID), nil)
	req.AddCookie(gerA)
	req.Header.Set("X-SCI", "1")
	rrG := httptest.NewRecorder()
	app.mux.ServeHTTP(rrG, req)
	if rrG.Code != http.StatusOK {
		t.Fatalf("GET anexo legado: %d (%s)", rrG.Code, rrG.Body.String())
	}
	cd := rrG.Header().Get("Content-Disposition")
	ct := rrG.Header().Get("Content-Type")
	if !strings.Contains(cd, "attachment") || strings.Contains(cd, "inline") {
		t.Fatalf("Content-Disposition hostil: %q", cd)
	}
	if ct != "application/octet-stream" {
		t.Fatalf("Content-Type hostil: %q", ct)
	}
	// O fix está nos HEADERS (attachment + octet-stream + nosniff): o conteúdo legado
	// continua vindo (dados do usuário), mas nunca renderizável na origem. O filename
	// tem que sair saneado (basename, sem aspas/controles/path traversal).
	if !strings.Contains(cd, `filename="evil.html"`) || strings.Contains(cd, "../") {
		t.Fatalf("filename não saneado p/ basename: %q", cd)
	}
}

func TestP1ModelosLifecycle(t *testing.T) {
	app, _, cleanup := setupTestApp(t)
	defer cleanup()
	admin := loginAs(t, app, "admin", "admin123")
	_, gerA := criaGrupo(t, app, admin, "G Modelos", "ger_modelos")

	var tipoID int64
	// escala_tipos é seed global; se vazio, cria um.
	if err := app.st.db.QueryRow(`SELECT id FROM escala_tipos LIMIT 1`).Scan(&tipoID); err != nil {
		resTp, errTp := app.st.db.Exec(`INSERT INTO escala_tipos (nome) VALUES ('Serviço Teste')`)
		if errTp != nil {
			t.Fatalf("criar tipo: %v", errTp)
		}
		tipoID, _ = resTp.LastInsertId()
	}

	// (t10) cria modelo com 1 posto
	rrC, resC := doJSONReq(app, "POST", "/api/escalas/modelos", map[string]any{
		"nome":   "Modelo P1",
		"postos": []map[string]any{{"tipo_id": tipoID, "hora_inicio": "07:00", "hora_fim": "19:00", "quantidade": 1, "ordem": 1}},
	}, gerA)
	if rrC.Code != http.StatusOK {
		t.Fatalf("criar modelo: %d (%v)", rrC.Code, resC)
	}
	modeloID := int64(resC["id"].(float64))

	// (t9) GET /api/escalas/minhas → 200
	rrM, resM := doJSONReq(app, "GET", "/api/escalas/minhas", nil, gerA)
	if rrM.Code != http.StatusOK {
		t.Fatalf("GET minhas: %d (%v)", rrM.Code, resM)
	}

	// (t10) GET list contém o criado
	rrL, resL := doJSONReq(app, "GET", "/api/escalas/modelos", nil, gerA)
	if rrL.Code != http.StatusOK {
		t.Fatalf("GET modelos: %d (%v)", rrL.Code, resL)
	}
	var lista struct {
		Modelos []struct {
			ID   int64  `json:"id"`
			Nome string `json:"nome"`
		} `json:"modelos"`
	}
	if err := json.Unmarshal(rrL.Body.Bytes(), &lista); err != nil {
		t.Fatalf("unmarshal lista: %v", err)
	}
	achou := false
	for _, m := range lista.Modelos {
		if m.ID == modeloID && m.Nome == "Modelo P1" {
			achou = true
		}
	}
	if !achou {
		t.Fatalf("modelo criado não veio na listagem: %s", rrL.Body.String())
	}

	// (t8) DELETE com postos: apagar → GET/{id} 404
	rrD, resD := doJSONReq(app, "DELETE", fmt.Sprintf("/api/escalas/modelos/%d", modeloID), nil, gerA)
	if rrD.Code != http.StatusOK {
		t.Fatalf("DELETE modelo: %d (%v)", rrD.Code, resD)
	}
	rrG, resG := doJSONReq(app, "GET", fmt.Sprintf("/api/escalas/modelos/%d", modeloID), nil, gerA)
	if rrG.Code != http.StatusNotFound {
		t.Fatalf("GET pós-DELETE: esperado 404, veio %d (%v)", rrG.Code, resG)
	}
}
