package main

// Regressão dos 9 bugs da Cia de Teste (rodada 04/10/26 — BUGS_s2.md / BUGS_s3.md):
//   F1 anexos INLINE do cautelar: allowlist MIME + sanitizarNomeArquivo (bypass morto);
//   F2 IDOR anexos: Get/List/Del exigem escopo da cautela (via item→grupo);
//   F3 POST modelos com id alheio reescrevia postos/aptos cross-group → 403 + vítima intacta;
//   F4 reaplicar modelo na mesma data duplicava turnos → 409;
//   F5 DELETE modelo referenciado por turnos → 500 FK cru → 409; livre → 200+404 pós;
//   F6 PATCH fase por não-dono → 200 fantasma {atualizados:0} → 404;
//   F7 anexo em cautela inexistente → 500 FK cru → 404.
//   (F8 GET modelos/{id} escopado e F9 slices não-nil são cobertos incidentalmente:
//   bravo no GET/{id} do modelo alheio → 403; listagem vazia Marshaliza [].)
// Padrões da suíte: setupTestApp/loginAs/doJSONReq/criaGrupo; tríade de escopo:
// forjado recusado + vítima intacta + dono edita normal.

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// cautelaTeste cria grupo+item+pessoa e devolve id da cautela feita pelo gerente.
func cautelaTeste(t *testing.T, app *App, st *Store, admin *http.Cookie, nomeGrupo, login string) (int64, *http.Cookie, int64, int64) {
	t.Helper()
	grupoID, ger := criaGrupo(t, app, admin, nomeGrupo, login)
	var catID int64
	if err := st.db.QueryRow(`SELECT id FROM material_categorias LIMIT 1`).Scan(&catID); err != nil {
		t.Fatalf("seed categorias: %v", err)
	}
	rrI, resI := doJSONReq(app, "POST", "/api/material/itens", map[string]any{
		"categoria_id": catID, "nome": "Rádio " + nomeGrupo, "codigo_patrimonio": "CIA-" + login,
	}, ger)
	if rrI.Code != http.StatusOK {
		t.Fatalf("criar item: %d (%v)", rrI.Code, resI)
	}
	itemID := int64(resI["id"].(float64))
	resPes, err := st.db.Exec(`INSERT INTO pessoas (nome_guerra, nome_completo, grupo_id, status)
		VALUES ('MUNIZ', 'Muniz de Castro', ?, 'ativo')`, grupoID)
	if err != nil {
		t.Fatalf("pessoa: %v", err)
	}
	pesID, _ := resPes.LastInsertId()
	rrC, resC := doJSONReq(app, "POST", "/api/material/cautelar", map[string]any{
		"item_id": itemID, "pessoa_id": pesID, "obs_saida": "serviço",
	}, ger)
	if rrC.Code != http.StatusOK {
		t.Fatalf("cautelar: %d (%v)", rrC.Code, resC)
	}
	return int64(resC["cautela_id"].(float64)), ger, itemID, grupoID
}

// tipoEscala garante um escala_tipos e devolve o id.
func tipoEscala(t *testing.T, app *App) int64 {
	t.Helper()
	var tipoID int64
	if err := app.st.db.QueryRow(`SELECT id FROM escala_tipos LIMIT 1`).Scan(&tipoID); err != nil {
		resTp, errTp := app.st.db.Exec(`INSERT INTO escala_tipos (nome) VALUES ('Cia Teste')`)
		if errTp != nil {
			t.Fatalf("criar tipo: %v", errTp)
		}
		tipoID, _ = resTp.LastInsertId()
	}
	return tipoID
}

// modeloTeste cria um modelo com 1 posto (07:00→19:00, qtd 1) e devolve o id.
func modeloTeste(t *testing.T, app *App, ger *http.Cookie, tipoID int64, nome string) int64 {
	t.Helper()
	rr, res := doJSONReq(app, "POST", "/api/escalas/modelos", map[string]any{
		"nome":   nome,
		"postos": []map[string]any{{"tipo_id": tipoID, "hora_inicio": "07:00", "hora_fim": "19:00", "quantidade": 1, "ordem": 1}},
	}, ger)
	if rr.Code != http.StatusOK {
		t.Fatalf("criar modelo %s: %d (%v)", nome, rr.Code, res)
	}
	return int64(res["id"].(float64))
}

// F1: anexo inline text/html no cautelar → 400 (allowlist), nada gravado.
func TestCiaF1AnexoInlineCautelar(t *testing.T) {
	app, st, cleanup := setupTestApp(t)
	defer cleanup()
	admin := loginAs(t, app, "admin", "admin123")
	cautelaID, gerA, itemID, _ := cautelaTeste(t, app, st, admin, "G Cia F1", "ger_f1")
	_ = cautelaID

	// (a) html inline → 400 (validação do anexo roda ANTES da tx/estoque)
	rr, res := doJSONReq(app, "POST", "/api/material/cautelar", map[string]any{
		"item_id": itemID, "pessoa_id": 1,
		"anexos": []map[string]any{{"nome_arquivo": "malicioso.html", "tipo_mime": "text/html", "tamanho": 20, "dados_base64": "PGh0bWw+eDwvaHRtbD4="}},
	}, gerA)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("inline html: esperado 400, veio %d (%v)", rr.Code, res)
	}
	// (b) nome cruel + mime ok → 200, nome SANEADO gravado (item novo, disponível)
	var catID int64
	if err := st.db.QueryRow(`SELECT id FROM material_categorias LIMIT 1`).Scan(&catID); err != nil {
		t.Fatalf("seed categorias: %v", err)
	}
	rrI2, resI2 := doJSONReq(app, "POST", "/api/material/itens", map[string]any{
		"categoria_id": catID, "nome": "Câmera F1", "codigo_patrimonio": "CIA-F1-B",
	}, gerA)
	if rrI2.Code != http.StatusOK {
		t.Fatalf("criar item B: %d (%v)", rrI2.Code, resI2)
	}
	itemB := int64(resI2["id"].(float64))
	rrB, resB := doJSONReq(app, "POST", "/api/material/cautelar", map[string]any{
		"item_id": itemB, "pessoa_id": 1,
		"anexos": []map[string]any{{"nome_arquivo": "../../etc/evil\".pdf", "tipo_mime": "application/pdf", "tamanho": 4, "dados_base64": "dGVzdA=="}},
	}, gerA)
	if rrB.Code != http.StatusOK {
		t.Fatalf("inline pdf com nome cruel: esperado 200, veio %d (%v)", rrB.Code, resB)
	}
	cautelaB := int64(resB["cautela_id"].(float64))
	var nomeDB, mimeDB string
	if err := st.db.QueryRow(`SELECT nome_arquivo, tipo_mime FROM material_cautela_anexos WHERE cautela_id = ? ORDER BY id DESC LIMIT 1`, cautelaB).Scan(&nomeDB, &mimeDB); err != nil {
		t.Fatalf("ler anexo inline: %v", err)
	}
	if strings.ContainsAny(nomeDB, `/"'`) || strings.Contains(nomeDB, "..") || nomeDB != "evil.pdf" {
		t.Fatalf("nome não saneado: %q", nomeDB)
	}
	if mimeDB != "application/pdf" {
		t.Fatalf("mime inesperado: %q", mimeDB)
	}
	// (c) recusa do (a) NÃO criou cautela: total = helper + passo b = 2
	var n int
	_ = st.db.QueryRow(`SELECT COUNT(*) FROM material_cautelas`).Scan(&n)
	if n != 2 {
		t.Fatalf("recusa de anexo hostil gerou efeito colateral: %d cautelas", n)
	}
}

// F2: operador de outro grupo não lê/lista/apaga anexo alheio; dono opera normal.
func TestCiaF2IDORAnexos(t *testing.T) {
	app, st, cleanup := setupTestApp(t)
	defer cleanup()
	admin := loginAs(t, app, "admin", "admin123")
	cautelaA, gerA, _, grupoA := cautelaTeste(t, app, st, admin, "G Cia F2 A", "ger_f2a")

	// anexo legítimo no grupo A
	pdf := "JVBERi0xLjQK" // %PDF-1.4
	rrAnx, resAnx := doJSONReq(app, "POST", fmt.Sprintf("/api/material/cautelas/%d/anexos", cautelaA), map[string]any{
		"nome_arquivo": "ficha.pdf", "tipo_mime": "application/pdf", "tamanho": 8, "dados_base64": pdf,
	}, gerA)
	if rrAnx.Code != http.StatusOK {
		t.Fatalf("anexo A: %d (%v)", rrAnx.Code, resAnx)
	}
	anexoID := int64(resAnx["id"].(float64))

	// gerente B + operador B (grupo alheio)
	// (ordem 04/10: gerente não cria operador — cria chefe de setor, que cria
	// o operador)
	grupoB, gerB := criaGrupo(t, app, admin, "G Cia F2 B", "ger_f2b")
	rrChefe, resChefe := doJSONReq(app, "POST", "/api/usuarios", map[string]any{
		"login": "chefe_f2b", "senha": "senha12345", "papel": "chefe_setor",
	}, gerB)
	if rrChefe.Code != http.StatusOK {
		t.Fatalf("criar chefe B: %d (%v)", rrChefe.Code, resChefe)
	}
	// v1.6.0 F4: o chefe precisa de setor no cadastro — a herança do ramo do
	// chefe (hUsuariosAdd) é o que vincula o setor do operador que ele cria.
	var setorF2B int64
	if err := st.db.QueryRow(`INSERT INTO setores (nome, grupo_id, ativo) VALUES ('Setor F2 B', ?, 1) RETURNING id`, grupoB).Scan(&setorF2B); err != nil {
		t.Fatalf("criar setor B: %v", err)
	}
	if _, err := st.db.Exec(`UPDATE usuarios SET setor_id = ? WHERE login = 'chefe_f2b'`, setorF2B); err != nil {
		t.Fatalf("setor do chefe B: %v", err)
	}
	chefeB := loginAs(t, app, "chefe_f2b", "senha12345")
	rrOp, resOp := doJSONReq(app, "POST", "/api/usuarios", map[string]any{
		"login": "op_f2b", "senha": "senha12345", "papel": "operador",
	}, chefeB)
	if rrOp.Code != http.StatusOK {
		t.Fatalf("criar operador B: %d (%v)", rrOp.Code, resOp)
	}
	opB := loginAs(t, app, "op_f2b", "senha12345")

	// (a) GET bytes → 403
	rrG, _ := doJSONReq(app, "GET", fmt.Sprintf("/api/material/anexos/%d", anexoID), nil, opB)
	if rrG.Code != http.StatusForbidden {
		t.Fatalf("GET anexo cross-group: esperado 403, veio %d", rrG.Code)
	}
	// (b) LIST metadados → 403
	rrL, _ := doJSONReq(app, "GET", fmt.Sprintf("/api/material/cautelas/%d/anexos", cautelaA), nil, opB)
	if rrL.Code != http.StatusForbidden {
		t.Fatalf("LIST anexos cross-group: esperado 403, veio %d", rrL.Code)
	}
	// (c) DELETE destrutivo → 403 e anexo SOBREVIVE
	rrD, _ := doJSONReq(app, "DELETE", fmt.Sprintf("/api/material/anexos/%d", anexoID), nil, opB)
	if rrD.Code != http.StatusForbidden {
		t.Fatalf("DELETE anexo cross-group: esperado 403, veio %d", rrD.Code)
	}
	var vive int
	if err := st.db.QueryRow(`SELECT COUNT(*) FROM material_cautela_anexos WHERE id = ?`, anexoID).Scan(&vive); err != nil || vive != 1 {
		t.Fatalf("anexo vítima apagado (err %v, count %d)", err, vive)
	}
	// (d) dono lista, baixa e apaga: 200 / 200 / 200 + 404 pós
	if rrLD, _ := doJSONReq(app, "GET", fmt.Sprintf("/api/material/cautelas/%d/anexos", cautelaA), nil, gerA); rrLD.Code != http.StatusOK {
		t.Fatalf("dono LIST: %d", rrLD.Code)
	}
	req := httptest.NewRequest("GET", fmt.Sprintf("/api/material/anexos/%d", anexoID), nil)
	req.AddCookie(gerA)
	rrGD := httptest.NewRecorder()
	app.mux.ServeHTTP(rrGD, req)
	if rrGD.Code != http.StatusOK {
		t.Fatalf("dono GET anexo: %d (%s)", rrGD.Code, rrGD.Body.String())
	}
	if rrDD, _ := doJSONReq(app, "DELETE", fmt.Sprintf("/api/material/anexos/%d", anexoID), nil, gerA); rrDD.Code != http.StatusOK {
		t.Fatalf("dono DELETE: %d", rrDD.Code)
	}
	if rrG2, _ := doJSONReq(app, "GET", fmt.Sprintf("/api/material/anexos/%d", anexoID), nil, gerA); rrG2.Code != http.StatusNotFound {
		t.Fatalf("GET pós-delete dono: esperado 404, veio %d", rrG2.Code)
	}
	// (e) admin (escopo 0) vê tudo
	rrAdm, _ := doJSONReq(app, "GET", fmt.Sprintf("/api/material/cautelas/%d/anexos", cautelaA), nil, admin)
	_ = rrAdm
	_ = grupoA
}

// F3: bravo envia POST /modelos com id do modelo do alpha → 403, postos da vítima intactos;
// dono edita 200.
func TestCiaF3ModeloCrossGroup(t *testing.T) {
	app, st, cleanup := setupTestApp(t)
	defer cleanup()
	admin := loginAs(t, app, "admin", "admin123")
	_, gerA := criaGrupo(t, app, admin, "G Cia F3 A", "ger_f3a")
	_, gerB := criaGrupo(t, app, admin, "G Cia F3 B", "ger_f3b")
	tipoID := tipoEscala(t, app)
	modeloA := modeloTeste(t, app, gerA, tipoID, "Modelo Alpha F3")

	// ataque: bravo manda o id alheio com postos 23:00/qty9
	rr, res := doJSONReq(app, "POST", "/api/escalas/modelos", map[string]any{
		"id":     modeloA,
		"nome":   "HACKEADO",
		"postos": []map[string]any{{"tipo_id": tipoID, "hora_inicio": "23:00", "hora_fim": "23:59", "quantidade": 9, "ordem": 1}},
	}, gerB)
	if rr.Code != http.StatusForbidden {
		t.Fatalf("edição cross-group: esperado 403, veio %d (%v)", rr.Code, res)
	}
	// vítima intacta
	var hi string
	var qtd int
	if err := st.db.QueryRow(`SELECT hora_inicio, quantidade FROM escala_modelo_postos WHERE modelo_id = ?`, modeloA).Scan(&hi, &qtd); err != nil {
		t.Fatalf("ler posto vítima: %v", err)
	}
	if hi != "07:00" || qtd != 1 {
		t.Fatalf("modelo vítima REESCRITO: %s qty=%d", hi, qtd)
	}
	var nomeModelo string
	_ = st.db.QueryRow(`SELECT nome FROM escala_modelos WHERE id = ?`, modeloA).Scan(&nomeModelo)
	if nomeModelo == "HACKEADO" {
		t.Fatalf("nome do modelo vítima alterado")
	}
	// dono edita: 200 e postos reescritos
	rrD, resD := doJSONReq(app, "POST", "/api/escalas/modelos", map[string]any{
		"id":     modeloA,
		"nome":   "Modelo Alpha F3 v2",
		"postos": []map[string]any{{"tipo_id": tipoID, "hora_inicio": "08:00", "hora_fim": "20:00", "quantidade": 2, "ordem": 1}},
	}, gerA)
	if rrD.Code != http.StatusOK {
		t.Fatalf("dono edita modelo: %d (%v)", rrD.Code, resD)
	}
	if err := st.db.QueryRow(`SELECT hora_inicio, quantidade FROM escala_modelo_postos WHERE modelo_id = ?`, modeloA).Scan(&hi, &qtd); err != nil || hi != "08:00" || qtd != 2 {
		t.Fatalf("dono não reescreveu postos: %s qty=%d (err %v)", hi, qtd, err)
	}
}

// F4: reaplicar modelo na mesma data → 409, sem duplicar turnos.
func TestCiaF4ReaplicarModelo(t *testing.T) {
	app, _, cleanup := setupTestApp(t)
	defer cleanup()
	admin := loginAs(t, app, "admin", "admin123")
	_, gerA := criaGrupo(t, app, admin, "G Cia F4", "ger_f4")
	tipoID := tipoEscala(t, app)
	modelo := modeloTeste(t, app, gerA, tipoID, "Modelo F4")

	rr1, res1 := doJSONReq(app, "POST", "/api/escalas/aplicar-modelo", map[string]any{
		"modelo_id": modelo, "data": "2026-11-10",
	}, gerA)
	if rr1.Code != http.StatusOK {
		t.Fatalf("aplicar 1a vez: %d (%v)", rr1.Code, res1)
	}
	rr2, res2 := doJSONReq(app, "POST", "/api/escalas/aplicar-modelo", map[string]any{
		"modelo_id": modelo, "data": "2026-11-10",
	}, gerA)
	if rr2.Code != http.StatusConflict {
		t.Fatalf("reaplicar: esperado 409, veio %d (%v)", rr2.Code, res2)
	}
	var n int
	if err := app.st.db.QueryRow(`SELECT COUNT(*) FROM escala_turnos WHERE modelo_id = ?`, modelo).Scan(&n); err != nil || n != 1 {
		t.Fatalf("turnos duplicados ou errados: %d (err %v)", n, err)
	}
	// data DIFERENTE aplica normal (não é reaplicação)
	rr3, res3 := doJSONReq(app, "POST", "/api/escalas/aplicar-modelo", map[string]any{
		"modelo_id": modelo, "data": "2026-11-11",
	}, gerA)
	if rr3.Code != http.StatusOK {
		t.Fatalf("aplicar outra data: %d (%v)", rr3.Code, res3)
	}
}

// F5: DELETE modelo referenciado por turnos → 409; sem turnos → 200 e 404 pós.
func TestCiaF5DeleteModeloReferenciado(t *testing.T) {
	app, _, cleanup := setupTestApp(t)
	defer cleanup()
	admin := loginAs(t, app, "admin", "admin123")
	_, gerA := criaGrupo(t, app, admin, "G Cia F5", "ger_f5")
	tipoID := tipoEscala(t, app)
	modelo := modeloTeste(t, app, gerA, tipoID, "Modelo F5")
	if rr, res := doJSONReq(app, "POST", "/api/escalas/aplicar-modelo", map[string]any{
		"modelo_id": modelo, "data": "2026-11-12",
	}, gerA); rr.Code != http.StatusOK {
		t.Fatalf("aplicar: %d (%v)", rr.Code, res)
	}
	// referenciado → 409 (não 500)
	rr, res := doJSONReq(app, "DELETE", fmt.Sprintf("/api/escalas/modelos/%d", modelo), nil, gerA)
	if rr.Code != http.StatusConflict {
		t.Fatalf("delete referenciado: esperado 409, veio %d (%v)", rr.Code, res)
	}
	// modelo continua lá
	rrGet, _ := doJSONReq(app, "GET", fmt.Sprintf("/api/escalas/modelos/%d", modelo), nil, gerA)
	if rrGet.Code != http.StatusOK {
		t.Fatalf("modelo sumiu após 409: %d", rrGet.Code)
	}
	// limpa turnos (rota real do front/chefe) → agora apaga: 200 e 404 pós
	if rrL, resL := doJSONReq(app, "POST", "/api/escalas/limpar-dia", map[string]any{"data": "2026-11-12"}, gerA); rrL.Code != http.StatusOK {
		t.Fatalf("limpar-dia: %d (%v)", rrL.Code, resL)
	}
	rrOK, resOK := doJSONReq(app, "DELETE", fmt.Sprintf("/api/escalas/modelos/%d", modelo), nil, gerA)
	if rrOK.Code != http.StatusOK {
		t.Fatalf("delete livre: esperado 200, veio %d (%v)", rrOK.Code, resOK)
	}
	if rrG, _ := doJSONReq(app, "GET", fmt.Sprintf("/api/escalas/modelos/%d", modelo), nil, gerA); rrG.Code != http.StatusNotFound {
		t.Fatalf("pós-delete: esperado 404, veio %d", rrG.Code)
	}
}

// F6: PATCH fase por não-dono → 404 (não 200 {atualizados:0}); dono muda de verdade.
func TestCiaF6FasePorNaoDono(t *testing.T) {
	app, st, cleanup := setupTestApp(t)
	defer cleanup()
	admin := loginAs(t, app, "admin", "admin123")
	_, gerA := criaGrupo(t, app, admin, "G Cia F6 A", "ger_f6a")
	_, gerB := criaGrupo(t, app, admin, "G Cia F6 B", "ger_f6b")
	tipoID := tipoEscala(t, app)
	modelo := modeloTeste(t, app, gerA, tipoID, "Modelo F6")
	if rr, res := doJSONReq(app, "POST", "/api/escalas/aplicar-modelo", map[string]any{
		"modelo_id": modelo, "data": "2026-11-13",
	}, gerA); rr.Code != http.StatusOK {
		t.Fatalf("aplicar: %d (%v)", rr.Code, res)
	}
	// bravo tenta mudar fase dos turnos do alpha → 404
	rr, res := doJSONReq(app, "PATCH", "/api/escalas/fase", map[string]any{
		"data": "2026-11-13", "fase": "publicado",
	}, gerB)
	if rr.Code != http.StatusNotFound {
		t.Fatalf("fase cross-group: esperado 404, veio %d (%v)", rr.Code, res)
	}
	// turnos intactos (fase aberto)
	var fase string
	if err := st.db.QueryRow(`SELECT fase FROM escala_turnos WHERE modelo_id = ?`, modelo).Scan(&fase); err != nil || fase != "aberto" {
		t.Fatalf("fase da vítima alterada: %q (err %v)", fase, err)
	}
	// dono: 200 com atualizados=1
	rrD, resD := doJSONReq(app, "PATCH", "/api/escalas/fase", map[string]any{
		"data": "2026-11-13", "fase": "publicado",
	}, gerA)
	if rrD.Code != http.StatusOK {
		t.Fatalf("dono muda fase: %d (%v)", rrD.Code, resD)
	}
	if v, _ := resD["atualizados"].(float64); int(v) != 1 {
		t.Fatalf("atualizados esperado 1, veio %v", resD["atualizados"])
	}
}

// F7: anexo em cautela inexistente → 404 (não 500 FK cru).
func TestCiaF7AnexoCautelaInexistente(t *testing.T) {
	app, _, cleanup := setupTestApp(t)
	defer cleanup()
	admin := loginAs(t, app, "admin", "admin123")
	_, gerA := criaGrupo(t, app, admin, "G Cia F7", "ger_f7")
	rr, res := doJSONReq(app, "POST", "/api/material/cautelas/999999/anexos", map[string]any{
		"nome_arquivo": "x.pdf", "tipo_mime": "application/pdf", "tamanho": 4, "dados_base64": "dGVzdA==",
	}, gerA)
	if rr.Code != http.StatusNotFound {
		t.Fatalf("anexo cautela inexistente: esperado 404, veio %d (%v)", rr.Code, res)
	}
}

// F8+F9: bravo não lê GET /modelos/{id} do alpha (403, sem PII);
// listagem de escopo vazio Marshaliza [] (não null).
func TestCiaF8F9ModeloGetEscopoEListaVazia(t *testing.T) {
	app, _, cleanup := setupTestApp(t)
	defer cleanup()
	admin := loginAs(t, app, "admin", "admin123")
	_, gerA := criaGrupo(t, app, admin, "G Cia F8 A", "ger_f8a")
	_, gerB := criaGrupo(t, app, admin, "G Cia F8 B", "ger_f8b")
	tipoID := tipoEscala(t, app)
	modelo := modeloTeste(t, app, gerA, tipoID, "Modelo F8")

	// bravo no GET/{id} alheio → 403 (antes: 200 com PII)
	rr, res := doJSONReq(app, "GET", fmt.Sprintf("/api/escalas/modelos/%d", modelo), nil, gerB)
	if rr.Code != http.StatusForbidden {
		t.Fatalf("GET modelo cross-group: esperado 403, veio %d (%v)", rr.Code, res)
	}
	// dono lê normal: postos/aptos são [] (nunca null)
	rrD, resD := doJSONReq(app, "GET", fmt.Sprintf("/api/escalas/modelos/%d", modelo), nil, gerA)
	if rrD.Code != http.StatusOK {
		t.Fatalf("dono GET modelo: %d (%v)", rrD.Code, resD)
	}
	var out struct {
		Postos []map[string]any `json:"postos"`
		Aptos  []map[string]any `json:"aptos"`
	}
	if err := json.Unmarshal(rrD.Body.Bytes(), &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if out.Postos == nil || out.Aptos == nil {
		t.Fatalf("postos/aptos null (cia-F9): %s", rrD.Body.String())
	}
	// listagem vazia para B: modelos é [] (nunca null)
	rrL, _ := doJSONReq(app, "GET", "/api/escalas/modelos", nil, gerB)
	var lista struct {
		Modelos []map[string]any `json:"modelos"`
	}
	if err := json.Unmarshal(rrL.Body.Bytes(), &lista); err != nil {
		t.Fatalf("unmarshal lista: %v", err)
	}
	if lista.Modelos == nil {
		t.Fatalf("modelos:null para escopo vazio (cia-F9): %s", rrL.Body.String())
	}
}
