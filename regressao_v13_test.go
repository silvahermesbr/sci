package main

// Regressão da rodada v1.3 (fixes do Sargento sobre cb36e4e) — um teste por fix:
// router P0, sanitização XSS (aviso+comentário+unit), contrato de comentário/cientes,
// escopo de repostar, revogação de coleção, cor allowlist, permissão de coleção,
// reservaAtivo fail-closed sem quebrar banco-zerado.

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

func TestRegressaoSanitizaRichText(t *testing.T) {
	limpo := sanitizaRichText(`<p>Texto <b>negrito</b></p><img src=x onerror="alert(1)"><script>evil()</script>`)
	if strings.Contains(limpo, "<img") || strings.Contains(limpo, "<script") || strings.Contains(limpo, "onerror") {
		t.Fatalf("sanitizador deixou passar vetor de XSS: %q", limpo)
	}
	if !strings.Contains(limpo, "<p>Texto <b>negrito</b></p>") {
		t.Fatalf("sanitizador destruiu formatação legítima: %q", limpo)
	}
	alinhado := sanitizaRichText(`<p style="text-align:center">c</p>`)
	if !strings.Contains(alinhado, `<p style="text-align:center">`) {
		t.Fatalf("align legítimo do editor foi perdido: %q", alinhado)
	}
	injetado := sanitizaRichText(`<p style="text-align:center;background-image:url(x)">c</p>`)
	if strings.Contains(injetado, "background-image") {
		t.Fatalf("atributo style além de align passou: %q", injetado)
	}
	evento := sanitizaRichText(`<b onclick="x()">n</b>`)
	if strings.Contains(evento, "onclick") {
		t.Fatalf("on* passou: %q", evento)
	}
	idempotente := sanitizaRichText(limpo)
	if idempotente != limpo {
		t.Fatalf("sanitizador não é idempotente:\n1: %q\n2: %q", limpo, idempotente)
	}
}

func TestRegressaoRouterAssetsV13(t *testing.T) {
	// O P0 do router (apagou #/hoje e #/mensagens) é de JS — a prova de regressão
	// ao nível do asset embutido: core.js servido PRECISA conter as ligações.
	app, _, cleanup := setupTestApp(t)
	defer cleanup()
	rr, _ := doJSONReq(app, "GET", "/core.js", nil, nil)
	if rr.Code != http.StatusOK {
		t.Fatalf("core.js: %d", rr.Code)
	}
	corpo := rr.Body.String()
	for _, marca := range []string{
		"if (h === '#/hoje') { chamarView('ViewHoje'); return; }",
		"if (h === '#/mensagens') { chamarView('ViewMensagens'); return; }",
	} {
		if !strings.Contains(corpo, marca) {
			t.Fatalf("router sem a ligação de rota (P0 regressão): %q", marca)
		}
	}
}

func TestRegressaoAvisosXSSeContratos(t *testing.T) {
	app, st, cleanup := setupTestApp(t)
	defer cleanup()
	admin := loginAs(t, app, "admin", "admin123")

	rr, res := doJSONReq(app, "POST", "/api/grupos", map[string]any{
		"nome": "G Reg XSS", "login": "ger_reg_xss", "senha": "senha12345", "nome_guerra": "GerXSS",
	}, admin)
	if rr.Code != http.StatusOK {
		t.Fatalf("grupo: %v", res)
	}
	grupoID := int64(res["id"].(float64))
	ger := loginAs(t, app, "ger_reg_xss", "senha12345")
	if rrOp, resOp := doJSONReq(app, "POST", "/api/usuarios", map[string]any{
		"login": "op_reg_xss", "senha": "senha12345", "papel": "operador",
		"grupo_id": grupoID, "nome_guerra": "OpXSS",
	}, admin); rrOp.Code != http.StatusOK {
		t.Fatalf("operador: %v", resOp)
	}
	op := loginAs(t, app, "op_reg_xss", "senha12345")

	// 1) publicar aviso com payload → sai sanitizado
	payload := `<p>Aviso <b>oficial</b></p><img src=x onerror="fetch('/x')"><script>alert(1)</script>`
	if rrA, resA := doJSONReq(app, "POST", "/api/avisos", map[string]any{
		"titulo": "Reg", "conteudo": payload,
	}, ger); rrA.Code != http.StatusOK {
		t.Fatalf("aviso: %v", resA)
	}
	rrL, _ := doJSONReq(app, "GET", "/api/avisos", nil, op)
	if rrL.Code != http.StatusOK {
		t.Fatalf("lista: %d", rrL.Code)
	}
	var avisos []map[string]any
	if err := json.Unmarshal(rrL.Body.Bytes(), &avisos); err != nil {
		t.Fatalf("lista de avisos não é array raiz: %v", err)
	}
	var conteudo string
	var avisoID float64
	for _, am := range avisos {
		if am["titulo"] == "Reg" {
			conteudo = am["conteudo"].(string)
			avisoID = am["id"].(float64)
		}
	}
	if conteudo == "" {
		t.Fatalf("aviso não listado")
	}
	if strings.Contains(conteudo, "<img") || strings.Contains(conteudo, "<script") || strings.Contains(conteudo, "onerror") {
		t.Fatalf("XSS armazenado no aviso: %q", conteudo)
	}
	if !strings.Contains(conteudo, "<p>Aviso <b>oficial</b></p>") {
		t.Fatalf("rich text legítimo destruído: %q", conteudo)
	}

	// 2) comentário pelo CONTRATO NOVO do front ({texto}) com payload → 200 + sanitizado
	rrC, resC := doJSONReq(app, "POST", `/api/avisos/`+intToString(int(avisoID))+`/comentar`, map[string]any{"texto": `<i>ciente</i><script>x()</script>`}, op)
	if rrC.Code != http.StatusOK {
		t.Fatalf("comentário com campo texto deveria 200 (contrato front), obtido %d: %v", rrC.Code, resC)
	}
	// 3) comentário pelo campo antigo também funciona
	if rrC2, _ := doJSONReq(app, "POST", `/api/avisos/`+intToString(int(avisoID))+`/comentar`, map[string]any{"comentario": `<u>ok</u><img onerror=y src=x>`}, ger); rrC2.Code != http.StatusOK {
		t.Fatalf("comentário campo antigo deveria 200, obtido %d", rrC2.Code)
	}

	// 4) detalhes: comentário sanitizado + alias text + cientes com contrato do front
	rrD, resD := doJSONReq(app, "GET", `/api/avisos/`+intToString(int(avisoID))+`/detalhes`, nil, ger)
	if rrD.Code != http.StatusOK {
		t.Fatalf("detalhes: %d", rrD.Code)
	}
	comps := resD["comentarios"].([]any)
	if len(comps) != 2 {
		t.Fatalf("esperado 2 comentários, obtido %d", len(comps))
	}
	c0 := comps[0].(map[string]any)
	if _, temTexto := c0["text"]; !temTexto {
		t.Fatalf("alias 'text' ausente no JSON de comentários (front lê c.texto)")
	}
	if strings.Contains(c0["text"].(string), "<script") {
		t.Fatalf("XSS armazenado em comentário: %q", c0["text"])
	}
	if !strings.Contains(c0["text"].(string), "<i>ciente</i>") {
		t.Fatalf("formatação legítima do comentário perdida: %q", c0["text"])
	}

	// 5) operador registra ciente → contrato registrado_em/funcao_nome/grupo_nome presente
	if rrCi, resCi := doJSONReq(app, "POST", `/api/avisos/`+intToString(int(avisoID))+`/ciente`, map[string]any{}, op); rrCi.Code != http.StatusOK {
		t.Fatalf("ciente: %v", resCi)
	}
	_, resD2 := doJSONReq(app, "GET", `/api/avisos/`+intToString(int(avisoID))+`/detalhes`, nil, ger)
	cientList := resD2["cientes"].([]any)
	if len(cientList) == 0 {
		t.Fatalf("ciente não listado")
	}
	ci := cientList[0].(map[string]any)
	for _, campo := range []string{"registrado_em", "funcao_nome", "grupo_nome"} {
		if _, ok := ci[campo]; !ok {
			t.Fatalf("campo %q ausente no JSON de cientes (contrato front)", campo)
		}
	}
	_ = st
}

func TestRegressaoRepostarForaEscopo(t *testing.T) {
	app, _, cleanup := setupTestApp(t)
	defer cleanup()
	admin := loginAs(t, app, "admin", "admin123")

	// Grupo A (origem do aviso) e Grupo B (sem relação com A — criado solto)
	rrA, resA := doJSONReq(app, "POST", "/api/grupos", map[string]any{
		"nome": "G A", "login": "ger_a", "senha": "senha12345", "nome_guerra": "GerA",
	}, admin)
	if rrA.Code != http.StatusOK {
		t.Fatalf("grupo A: %v", resA)
	}
	gerA := loginAs(t, app, "ger_a", "senha12345")
	rrB, resB := doJSONReq(app, "POST", "/api/grupos", map[string]any{
		"nome": "G B", "login": "ger_b", "senha": "senha12345", "nome_guerra": "GerB",
	}, admin)
	if rrB.Code != http.StatusOK {
		t.Fatalf("grupo B: %v", resB)
	}
	gerB := loginAs(t, app, "ger_b", "senha12345")

	if rrAv, resAv := doJSONReq(app, "POST", "/api/avisos", map[string]any{
		"titulo": "Confidencial A", "conteudo": "<p>interna do A</p>",
	}, gerA); rrAv.Code != http.StatusOK {
		t.Fatalf("aviso A: %v", resAv)
	}
	rrL, _ := doJSONReq(app, "GET", "/api/avisos", nil, gerA)
	var avisos []map[string]any
	if err := json.Unmarshal(rrL.Body.Bytes(), &avisos); err != nil {
		t.Fatalf("lista de avisos não é array raiz: %v", err)
	}
	var avisoID int64
	for _, am := range avisos {
		if am["titulo"] == "Confidencial A" {
			avisoID = int64(am["id"].(float64))
		}
	}
	if avisoID == 0 {
		t.Fatalf("aviso de origem não encontrado")
	}

	// gerente de grupo SEM relação não extrai nem clona (antes: 200)
	rrR, _ := doJSONReq(app, "POST", `/api/avisos/`+intToString(int(avisoID))+`/repostar`, map[string]any{}, gerB)
	if rrR.Code != http.StatusForbidden {
		t.Fatalf("repostar fora do escopo deveria 403, obtido %d", rrR.Code)
	}
}

func TestRegressaoRevogacaoColecaoECor(t *testing.T) {
	app, _, cleanup := setupTestApp(t)
	defer cleanup()
	admin := loginAs(t, app, "admin", "admin123")

	rr, res := doJSONReq(app, "POST", "/api/grupos", map[string]any{
		"nome": "G Rev", "login": "ger_rev", "senha": "senha12345", "nome_guerra": "GerRev",
	}, admin)
	if rr.Code != http.StatusOK {
		t.Fatalf("grupo: %v", res)
	}
	grupoID := int64(res["id"].(float64))
	ger := loginAs(t, app, "ger_rev", "senha12345")
	if rrOp, resOp := doJSONReq(app, "POST", "/api/usuarios", map[string]any{
		"login": "op_rev", "senha": "senha12345", "papel": "operador",
		"grupo_id": grupoID, "nome_guerra": "OpRev",
	}, admin); rrOp.Code != http.StatusOK {
		t.Fatalf("operador: %v", resOp)
	}
	op := loginAs(t, app, "op_rev", "senha12345")

	// cor inválida → 400 (allowlist)
	rrBad, _ := doJSONReq(app, "POST", "/api/calendarios", map[string]any{
		"nome": "Cal Má", "cor": "red;background-image:url(javascript:alert(1))",
	}, ger)
	if rrBad.Code != http.StatusBadRequest {
		t.Fatalf("cor inválida deveria 400, obtido %d", rrBad.Code)
	}
	// cor válida → cria
	rrOk, resOk := doJSONReq(app, "POST", "/api/calendarios", map[string]any{
		"nome": "Cal Rev", "cor": "#10b981",
	}, ger)
	if rrOk.Code != http.StatusOK {
		t.Fatalf("calendário válido: %v", resOk)
	}
	calID := int64(resOk["id"].(float64))

	// compartilha com o próprio grupo (pode_editar=0)
	if rrSh, resSh := doJSONReq(app, "POST", `/api/calendarios/`+intToString(int(calID))+`/compartilhar`, map[string]any{
		"alvo_tipo": "grupo", "alvo_id": grupoID,
	}, ger); rrSh.Code != http.StatusOK {
		t.Fatalf("compartilhar: %v", resSh)
	}

	// OPERADOR não grava evento em coleção alheia (antes: 200 com pode_editar=0)
	rrEv, _ := doJSONReq(app, "POST", "/api/calendario/eventos", map[string]any{
		"calendario_id": calID, "titulo": "Invasão", "data_inicio": "2026-11-01",
	}, op)
	if rrEv.Code != http.StatusForbidden {
		t.Fatalf("evento em coleção sem pode_editar deveria 403, obtido %d", rrEv.Code)
	}
	// DONO grava normalmente
	rrEv2, resEv2 := doJSONReq(app, "POST", "/api/calendario/eventos", map[string]any{
		"calendario_id": calID, "titulo": "Legítimo", "data_inicio": "2026-11-02",
	}, ger)
	if rrEv2.Code != http.StatusOK {
		t.Fatalf("evento do dono: %v", resEv2)
	}

	// revogação de COLEÇÃO (antes: 404 sempre — evento_id NULL)
	rrList, resList := doJSONReq(app, "GET", `/api/calendarios/`+intToString(int(calID))+`/compartilhamentos`, nil, ger)
	if rrList.Code != http.StatusOK {
		t.Fatalf("listar compartilhamentos: %d", rrList.Code)
	}
	comps := resList["compartilhamentos"].([]any)
	if len(comps) == 0 {
		t.Fatalf("compartilhamento não listado")
	}
	compID := int64(comps[0].(map[string]any)["id"].(float64))
	rrDel, _ := doJSONReq(app, "DELETE", `/api/calendario/compartilhamentos/`+intToString(int(compID)), nil, ger)
	if rrDel.Code != http.StatusOK {
		t.Fatalf("revogação de coleção deveria 200 (P1), obtido %d", rrDel.Code)
	}
	rrList2, resList2 := doJSONReq(app, "GET", `/api/calendarios/`+intToString(int(calID))+`/compartilhamentos`, nil, ger)
	raw2, ok2 := resList2["compartilhamentos"]
	if !ok2 {
		t.Fatalf("chave 'compartilhamentos' ausente; corpo cru: %s", rrList2.Body.String())
	}
	comps2, _ := raw2.([]any) // lista vazia pode virar null (slice nil do Go)
	if len(comps2) != 0 {
		t.Fatalf("compartilhamento sobrou após revogação")
	}
}

func TestRegressaoReservaAtivoFailClosed(t *testing.T) {
	app, st, cleanup := setupTestApp(t)
	defer cleanup()

	// banco zerado SEM flag: módulos ativos por desenho (não 423 em massa)
	if app.reservaAtivo() {
		t.Fatalf("flag ausente deveria ser módulos ativos (desenho v1.3)")
	}
	// flag '1': em reserva
	if _, err := st.db.Exec(`INSERT INTO configuracoes (chave, valor) VALUES ('MODO_RESERVA', '1')
		ON CONFLICT(chave) DO UPDATE SET valor='1'`); err != nil {
		t.Fatalf("seed flag: %v", err)
	}
	if !app.reservaAtivo() {
		t.Fatalf("flag '1' deveria travar os módulos")
	}
	// flag '0': liberado
	if _, err := st.db.Exec(`UPDATE configuracoes SET valor='0' WHERE chave='MODO_RESERVA'`); err != nil {
		t.Fatalf("update flag: %v", err)
	}
	if app.reservaAtivo() {
		t.Fatalf("flag '0' deveria liberar os módulos")
	}
}

func TestRegressaoCatalogoGrupoInexistente(t *testing.T) {
	app, _, cleanup := setupTestApp(t)
	defer cleanup()
	admin := loginAs(t, app, "admin", "admin123")

	rr, _ := doJSONReq(app, "POST", "/api/catalogo/setores", map[string]any{
		"nome": "S Fantasma", "grupo_id": 99999,
	}, admin)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("catálogo em grupo inexistente deveria 400, obtido %d", rr.Code)
	}
}

// intToString: helper local sem importar strconv só para isso.
func intToString(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var b [20]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		b[i] = '-'
	}
	return string(b[i:])
}
