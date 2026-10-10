package main

// onda_v154_d2_test.go — v1.5.4-D2 (defeito R-2): rotas de PDF/dados que
// estavam a.auth(false) SEM guarda de papel/escopo passam a ter o MESMO guard
// do módulo (o portão do rotear() no front é cosmético; o servidor é a guarda):
//
//	escalas:     /api/escalas/pdf, /api/escalas/relatorio-dia.pdf (+ duplicata
//	             /relatorio-dia/pdf) e /api/escalas/minhas → reservaAuth
//	             {gerente, operador, chefe_setor; admin 403} + conta COM grupo
//	             (prólogo exigeEscopo em `minhas`; abertura ampla pende da D-3/M5).
//	material:    /api/material/inventario/pdf, /api/material/cautelas/{id}/recibo.pdf
//	             e /api/material/conferencias/{id}/pronto.pdf → authMaterial
//	             {gerente, operador, enc_material; admin 403} + escopo do objeto.
//	conferência: /api/conferencia/{id}/relatorio.pdf → confPDFAuth (gerente/
//	             operador/enc_pessoal/auxiliar; chefe_setor SÓ se a conferência
//	             envolve setor que AINDA comanda; admin PROIBIDO) + escopo
//	             estrito do handler (próprio grupo/subordinados).
//
// Matriz por persona (padrão da casa: setupTestApp + loginAs + doJSONReq/
// doRawReqH), casos positivos E negativos. Nota de doutrina: as rotas de
// escalas SEM parâmetro de grupo (pdf, relatorio-dia, minhas, inventário) são
// confinadas ao escopo da SESSÃO — um operador de outro grupo recebe o PDF do
// PRÓPRIO grupo (200), não 403; o teste prova a não-vazamento pelo CONTEÚDO
// (texto do PDF não menciona o grupo alvo). 403 de papel/escopo é cobrado
// onde a rota endereça objeto de outro grupo e para as personas fora da
// doutrina (sem-grupo, admin, chefe sem comando).

import (
	"bytes"
	"net/http"
	"strings"
	"testing"
)

func TestD2_R2_PDFsComGuardaDoModulo(t *testing.T) {
	app, st, cleanup := setupTestApp(t)
	defer cleanup()

	// ---------- grupos: ALVO (A) e FORA (B) ----------
	var gidA, gidB int64
	if err := st.db.QueryRow(`INSERT INTO grupos (nome) VALUES ('Grupo D2 Alvo') RETURNING id`).Scan(&gidA); err != nil {
		t.Fatalf("criar grupo A: %v", err)
	}
	if err := st.db.QueryRow(`INSERT INTO grupos (nome) VALUES ('Grupo D2 Fora') RETURNING id`).Scan(&gidB); err != nil {
		t.Fatalf("criar grupo B: %v", err)
	}

	// ---------- setores do grupo A (nomes UNIQUE) ----------
	var s1, s2 int64
	if err := st.db.QueryRow(`INSERT INTO setores (grupo_id, nome, sigla, ativo) VALUES (?, 'D2 Setor Comandado', 'D2C', 1) RETURNING id`, gidA).Scan(&s1); err != nil {
		t.Fatalf("criar setor S1: %v", err)
	}
	if err := st.db.QueryRow(`INSERT INTO setores (grupo_id, nome, sigla, ativo) VALUES (?, 'D2 Setor Livre', 'D2L', 1) RETURNING id`, gidA).Scan(&s2); err != nil {
		t.Fatalf("criar setor S2: %v", err)
	}

	// ---------- contas (grupo na CONTA ANTES do login — lição v1.5.4-A) ----------
	criaConta := func(login, papel string, gid *int64) int64 {
		t.Helper()
		criaUsuarioTeste(t, st, login, "senha-d2", papel)
		var id int64
		if gid != nil {
			if _, err := st.db.Exec(`UPDATE usuarios SET grupo_id = ? WHERE login = ?`, *gid, login); err != nil {
				t.Fatalf("grupo da conta %s: %v", login, err)
			}
		}
		if err := st.db.QueryRow(`SELECT id FROM usuarios WHERE login = ?`, login).Scan(&id); err != nil {
			t.Fatalf("id da conta %s: %v", login, err)
		}
		return id
	}
	gpA, gpB := gidA, gidB
	gerAID := criaConta("d2_ger", "gerente", &gpA)
	criaConta("d2_op", "operador", &gpA)
	criaConta("d2_opB", "operador", &gpB)
	criaConta("d2_semgrupo", "operador", nil)
	chefeAID := criaConta("d2_chefe", "chefe_setor", &gpA)

	ckGer := loginAs(t, app, "d2_ger", "senha-d2")
	ckOp := loginAs(t, app, "d2_op", "senha-d2")
	ckOpB := loginAs(t, app, "d2_opB", "senha-d2")
	ckSemGrupo := loginAs(t, app, "d2_semgrupo", "senha-d2")
	ckChefe := loginAs(t, app, "d2_chefe", "senha-d2")
	ckAdmin := loginAs(t, app, "admin", "admin123")

	// ---------- comando vigente do chefe: SOMENTE o setor S1 (chefe_setores) ----------
	materializaComandoSetor(t, st, chefeAID, gidA, s1)

	// ---------- dados do grupo A: pessoa, material, cautela, conf. material ----------
	var pA int64
	if err := st.db.QueryRow(`INSERT INTO pessoas (nome_guerra, nome_completo, grupo_id, setor_id, status) VALUES ('D2 SOLDADO', 'Soldado D2 Alvo', ?, ?, 'ativo') RETURNING id`, gidA, s1).Scan(&pA); err != nil {
		t.Fatalf("criar pessoa: %v", err)
	}
	var catID, itemID int64
	if err := st.db.QueryRow(`INSERT INTO material_categorias (grupo_id, nome, ativo) VALUES (?, 'D2 Categoria', 1) RETURNING id`, gidA).Scan(&catID); err != nil {
		t.Fatalf("criar categoria: %v", err)
	}
	if err := st.db.QueryRow(`INSERT INTO material_itens (grupo_id, categoria_id, nome, codigo_patrimonio, status) VALUES (?, ?, 'Fuzil D2', 'D2-0001', 'acautelado') RETURNING id`, gidA, catID).Scan(&itemID); err != nil {
		t.Fatalf("criar item: %v", err)
	}
	var cautID int64
	if err := st.db.QueryRow(`INSERT INTO material_cautelas (item_id, pessoa_id, responsavel_entrega_id, data_saida, status) VALUES (?, ?, ?, '2026-10-08T10:00:00Z', 'ativa') RETURNING id`, itemID, pA, gerAID).Scan(&cautID); err != nil {
		t.Fatalf("criar cautela: %v", err)
	}
	var matConfID int64
	if err := st.db.QueryRow(`INSERT INTO material_conferencias (grupo_id, setor_id, data, status, aberta_por, fechada_por, fechada_em) VALUES (?, ?, '2026-10-09', 'fechada', ?, ?, '2026-10-09T12:00:00Z') RETURNING id`, gidA, s1, gerAID, gerAID).Scan(&matConfID); err != nil {
		t.Fatalf("criar conferência de material: %v", err)
	}

	// ---------- conferências de pessoal FECHADAS do grupo A ----------
	// conf1 envolve S1 (setor COMANDADO pelo chefe) · conf2 envolve S2 (não comandado).
	var t1, t2 int64
	if err := st.db.QueryRow(`INSERT INTO conferencia_tipos (nome, ativo) VALUES ('D2 Tipo Comandado', 1) RETURNING id`).Scan(&t1); err != nil {
		t.Fatalf("criar tipo 1: %v", err)
	}
	if err := st.db.QueryRow(`INSERT INTO conferencia_tipos (nome, ativo) VALUES ('D2 Tipo Livre', 1) RETURNING id`).Scan(&t2); err != nil {
		t.Fatalf("criar tipo 2: %v", err)
	}
	var conf1, conf2 int64
	if err := st.db.QueryRow(`INSERT INTO conferencias (data, tipo_id, grupo_id, status, criado_por, fechada_em) VALUES ('2026-10-09', ?, ?, 'fechada', ?, '2026-10-09T12:00:00Z') RETURNING id`, t1, gidA, gerAID).Scan(&conf1); err != nil {
		t.Fatalf("criar conferência 1: %v", err)
	}
	if err := st.db.QueryRow(`INSERT INTO conferencias (data, tipo_id, grupo_id, status, criado_por, fechada_em) VALUES ('2026-10-09', ?, ?, 'fechada', ?, '2026-10-09T12:00:00Z') RETURNING id`, t2, gidA, gerAID).Scan(&conf2); err != nil {
		t.Fatalf("criar conferência 2: %v", err)
	}
	if _, err := st.db.Exec(`INSERT INTO conferencia_setores (conferencia_id, setor_id, status) VALUES (?, ?, 'concluida')`, conf1, s1); err != nil {
		t.Fatalf("CS conf1: %v", err)
	}
	if _, err := st.db.Exec(`INSERT INTO conferencia_setores (conferencia_id, setor_id, status) VALUES (?, ?, 'concluida')`, conf2, s2); err != nil {
		t.Fatalf("CS conf2: %v", err)
	}
	for _, c := range []int64{conf1, conf2} {
		if _, err := st.db.Exec(`INSERT INTO presencas (conferencia_id, pessoa_id, situacao, marcado_por, verificado) VALUES (?, ?, 'presente', ?, 1)`, c, pA, gerAID); err != nil {
			t.Fatalf("presença: %v", err)
		}
	}

	// ---------- helpers ----------
	cod := func(ck *http.Cookie, url string) int {
		rr := doRawReqH(app, "GET", url, nil, "", ck)
		return rr.Code
	}
	corpo := func(ck *http.Cookie, url string) (int, string) {
		rr := doRawReqH(app, "GET", url, nil, "", ck)
		return rr.Code, extrairTextoPDF(t, rr.Body.Bytes())
	}
	espera := func(ck *http.Cookie, url string, quero int, quem string) {
		t.Helper()
		if got := cod(ck, url); got != quero {
			t.Errorf("[%s] GET %s: esperado %d, veio %d", quem, url, quero, got)
		}
	}

	// ---------- GERENTE do grupo: vê os PDFs do PRÓPRIO grupo (200) ----------
	posRotasGer := []string{
		"/api/escalas/pdf?mes=2026-10",
		"/api/escalas/relatorio-dia.pdf?data=2026-10-09",
		"/api/escalas/relatorio-dia/pdf?data=2026-10-09",
		"/api/escalas/minhas",
		"/api/material/inventario/pdf",
		"/api/material/cautelas/" + int64ToStr(cautID) + "/recibo.pdf",
		"/api/material/conferencias/" + int64ToStr(matConfID) + "/pronto.pdf",
		"/api/conferencia/" + int64ToStr(conf1) + "/relatorio.pdf",
		"/api/conferencia/" + int64ToStr(conf2) + "/relatorio.pdf",
	}
	for _, url := range posRotasGer {
		espera(ckGer, url, http.StatusOK, "gerente A")
	}

	// ---------- OPERADOR do próprio grupo: papel do módulo, escopo próprio (200) ----------
	for _, url := range posRotasGer {
		espera(ckOp, url, http.StatusOK, "operador A")
	}

	// ---------- CONTA SEM GRUPO (-1): 403 em TODAS as rotas tocadas ----------
	negRotasSemGrupo := []string{
		"/api/escalas/pdf?mes=2026-10",
		"/api/escalas/relatorio-dia.pdf?data=2026-10-09",
		"/api/escalas/relatorio-dia/pdf?data=2026-10-09",
		"/api/escalas/minhas",
		"/api/material/inventario/pdf",
		"/api/material/cautelas/" + int64ToStr(cautID) + "/recibo.pdf",
		"/api/material/conferencias/" + int64ToStr(matConfID) + "/pronto.pdf",
		"/api/conferencia/" + int64ToStr(conf1) + "/relatorio.pdf",
	}
	for _, url := range negRotasSemGrupo {
		espera(ckSemGrupo, url, http.StatusForbidden, "conta sem grupo")
	}

	// ---------- ADMIN: doutrina dos módulos — proibido (403) ----------
	negRotasAdmin := []string{
		// conferência: admin PROIBIDO (matriz §4)
		"/api/conferencia/" + int64ToStr(conf1) + "/relatorio.pdf",
		// escalas: reservaAuth não inclui admin
		"/api/escalas/pdf?mes=2026-10",
		"/api/escalas/relatorio-dia.pdf?data=2026-10-09",
		"/api/escalas/minhas",
		// material: authMaterial não inclui admin
		"/api/material/inventario/pdf",
		"/api/material/cautelas/" + int64ToStr(cautID) + "/recibo.pdf",
		"/api/material/conferencias/" + int64ToStr(matConfID) + "/pronto.pdf",
	}
	for _, url := range negRotasAdmin {
		espera(ckAdmin, url, http.StatusForbidden, "admin")
	}

	// ---------- CHEFE de setor: relatório só da conferência que envolve ----------
	// ---------- setor que AINDA comanda (fonte única chefe_setores)  ----------
	espera(ckChefe, "/api/conferencia/"+int64ToStr(conf1)+"/relatorio.pdf", http.StatusOK, "chefe conf1 (setor comandado)")
	espera(ckChefe, "/api/conferencia/"+int64ToStr(conf2)+"/relatorio.pdf", http.StatusForbidden, "chefe conf2 (setor não comandado)")
	// chefe é papel do módulo escalas, mas NÃO do módulo material
	espera(ckChefe, "/api/escalas/pdf?mes=2026-10", http.StatusOK, "chefe escalas pdf")
	espera(ckChefe, "/api/material/inventario/pdf", http.StatusForbidden, "chefe material inventario")
	espera(ckChefe, "/api/material/conferencias/"+int64ToStr(matConfID)+"/pronto.pdf", http.StatusForbidden, "chefe material pronto")

	// ---------- OPERADOR de OUTRO grupo: nada do grupo alvo escopa ----------
	// Rotas que ENDEREÇAM objeto do grupo alvo → 403 (escopo estrito).
	espera(ckOpB, "/api/material/cautelas/"+int64ToStr(cautID)+"/recibo.pdf", http.StatusForbidden, "operador B recibo alheio")
	espera(ckOpB, "/api/material/conferencias/"+int64ToStr(matConfID)+"/pronto.pdf", http.StatusForbidden, "operador B pronto alheio")
	espera(ckOpB, "/api/conferencia/"+int64ToStr(conf1)+"/relatorio.pdf", http.StatusForbidden, "operador B conferência alheia")
	espera(ckOpB, "/api/conferencia/"+int64ToStr(conf2)+"/relatorio.pdf", http.StatusForbidden, "operador B conferência alheia 2")

	// Rotas confinadas ao ESCOPO DA SESSÃO (sem parâmetro de grupo): o operador
	// B recebe o PDF do PRÓPRIO grupo (200) — e o conteúdo NÃO vaza o grupo
	// alvo. Prova de não-vazamento pelo texto extraído do PDF.
	if c, txt := corpo(ckOpB, "/api/escalas/pdf?mes=2026-10"); c != http.StatusOK || !strings.Contains(txt, "Grupo D2 Fora") || strings.Contains(txt, "Grupo D2 Alvo") {
		t.Errorf("[operador B] escalas/pdf: code=%d, vazamento=%v (texto não mostra 'Grupo D2 Fora' ou menciona 'Grupo D2 Alvo')", c, strings.Contains(txt, "Grupo D2 Alvo"))
	}
	if c, txt := corpo(ckOpB, "/api/escalas/relatorio-dia.pdf?data=2026-10-09"); c != http.StatusOK || strings.Contains(txt, "GRUPO D2 ALVO") {
		t.Errorf("[operador B] relatorio-dia.pdf: code=%d, vazamento=%v", c, strings.Contains(txt, "GRUPO D2 ALVO"))
	}
	if c := cod(ckOpB, "/api/escalas/minhas"); c != http.StatusOK {
		t.Errorf("[operador B] minhas (dados da própria pessoa): esperado 200, veio %d", c)
	}
	if c := cod(ckOpB, "/api/material/inventario/pdf"); c != http.StatusOK {
		t.Errorf("[operador B] inventário (escopo próprio): esperado 200, veio %d", c)
	}
}

// TestD2_R2_PDFMagicBytes: os PDFs liberados são PDFs de verdade.
func TestD2_R2_PDFMagicBytes(t *testing.T) {
	app, st, cleanup := setupTestApp(t)
	defer cleanup()

	var gid int64
	if err := st.db.QueryRow(`INSERT INTO grupos (nome) VALUES ('Grupo D2 Magia') RETURNING id`).Scan(&gid); err != nil {
		t.Fatalf("criar grupo: %v", err)
	}
	criaUsuarioTeste(t, st, "d2_ger2", "senha-d2", "gerente")
	if _, err := st.db.Exec(`UPDATE usuarios SET grupo_id = ? WHERE login = 'd2_ger2'`, gid); err != nil {
		t.Fatalf("grupo do gerente: %v", err)
	}
	ckGer := loginAs(t, app, "d2_ger2", "senha-d2")

	for _, url := range []string{
		"/api/escalas/pdf?mes=2026-10",
		"/api/escalas/relatorio-dia.pdf?data=2026-10-09",
		"/api/material/inventario/pdf",
	} {
		rr := doRawReqH(app, "GET", url, nil, "", ckGer)
		if rr.Code != http.StatusOK {
			t.Fatalf("GET %s: esperado 200, veio %d (%s)", url, rr.Code, rr.Body.String())
		}
		if !bytes.HasPrefix(rr.Body.Bytes(), []byte("%PDF")) {
			t.Fatalf("GET %s: corpo não começa com %%PDF", url)
		}
	}
}
