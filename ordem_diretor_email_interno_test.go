package main

// Testes da Fase D da ordem Diretor 04/10 — "Email Interno" (mensageria):
//
//   D1. Abas separadas: caixa CONVENCIONAL não mostra despacho em aberto;
//       aba de despachos mostra. Despacho FINALIZADO vira mensagem comum
//       (sai da aba despachos-pendentes, entra na caixa convencional).
//   D2. FINALIZAR despacho: destinatário finaliza SEM responder; exige_resposta
//       cai, pendência baixada, contador de despachos pendentes zera.
//   D3. Arquivamento de despacho NÃO respondido continua BLOQUEADO (a trava
//       do arquivo sobrevive à finalização? NÃO — finalizado é comum, pode
//       arquivar; o bloqueio vale enquanto não respondido E não finalizado).
//   D4. Guardas: não-destinatário não finaliza; mensagem comum não finaliza.

import (
	"fmt"
	"net/http"
	"strings"
	"testing"
)

// ctxDespacho removido — enviarDespachoTeste cobre o contexto.

func enviarDespachoTeste(t *testing.T, app *App, st *Store, ckRemetente *http.Cookie, loginDest string) int64 {
	t.Helper()
	// papel-ativo do destinatário
	var papelDestID int64
	if err := st.db.QueryRow(`SELECT up.id FROM usuario_papeis up JOIN usuarios u ON u.id = up.usuario_id WHERE u.login = ?`, loginDest).Scan(&papelDestID); err != nil {
		t.Fatalf("papel do destinatário %s: %v", loginDest, err)
	}
	rr, res := doJSONReq(app, "POST", "/api/mensagens", map[string]any{
		"destinatario_papel_ids": []int64{papelDestID},
		"assunto":                "Despacho de prova D",
		"corpo":                  "<p>Cumpra-se.</p>",
		"tipo":                   "despacho",
	}, ckRemetente)
	if rr.Code != http.StatusOK {
		t.Fatalf("enviar despacho deve 200, veio %d: %v", rr.Code, res)
	}
	idv, _ := res["id"].(float64)
	return int64(idv)
}

// D1+D2: despacho em aberto fica na aba de despachos; FINALIZAR sem responder
// o transforma em mensagem comum na caixa convencional e zera a pendência.
func TestFinalizarDespachoViraMensagemComum(t *testing.T) {
	app, st, cleanup := setupTestApp(t)
	defer cleanup()

	ckGer := loginAsPapel(t, app, st, "gerd", "gerente")
	ckOp := loginAsPapel(t, app, st, "opd", "operador")

	msgID := enviarDespachoTeste(t, app, st, ckGer, "opd")

	// 1) despacho em aberto aparece na ABA despachos e NÃO na caixa convencional
	rrD, _ := doJSONReq(app, "GET", "/api/mensagens/inbox?despacho=1", nil, ckOp)
	if rrD.Code != http.StatusOK || !strings.Contains(rrD.Body.String(), "Despacho de prova D") {
		t.Fatalf("despacho em aberto deve listar na aba despachos: %d %s", rrD.Code, rrD.Body.String())
	}
	rrC, _ := doJSONReq(app, "GET", "/api/mensagens/inbox", nil, ckOp)
	if strings.Contains(rrC.Body.String(), "Despacho de prova D") {
		t.Fatalf("despacho em aberto NÃO deve aparecer na caixa convencional (ordem 04/10): %s", rrC.Body.String())
	}

	// 2) pendência existe antes de finalizar
	rrCnt, resCnt := doJSONReq(app, "GET", "/api/mensagens/contador", nil, ckOp)
	if rrCnt.Code != http.StatusOK {
		t.Fatalf("contador: %d", rrCnt.Code)
	}
	if pend, _ := resCnt["despachos_pendentes"].(float64); pend < 1 {
		t.Fatalf("esperado >=1 despacho pendente, veio %v", resCnt)
	}

	// 3) FINALIZAR sem responder — destinatário
	rrF, resF := doJSONReq(app, "POST", fmt.Sprintf("/api/mensagens/%d/finalizar", msgID), nil, ckOp)
	if rrF.Code != http.StatusOK {
		t.Fatalf("finalizar deve 200, veio %d: %v", rrF.Code, resF)
	}

	// 4) saiu da aba despachos (lista mostra só pendentes? não — mas pendência baixou)
	rrCnt2, resCnt2 := doJSONReq(app, "GET", "/api/mensagens/contador", nil, ckOp)
	if rrCnt2.Code != http.StatusOK {
		t.Fatalf("contador pós-finalizar: %d", rrCnt2.Code)
	}
	if pend, _ := resCnt2["despachos_pendentes"].(float64); pend != 0 {
		t.Fatalf("despachos_pendentes deve zerar após finalizar, veio %v", resCnt2)
	}

	// 5) agora aparece na caixa CONVENCIONAL (virou mensagem comum)
	rrC2, _ := doJSONReq(app, "GET", "/api/mensagens/inbox", nil, ckOp)
	if !strings.Contains(rrC2.Body.String(), "Despacho de prova D") {
		t.Fatalf("despacho finalizado deve aparecer na caixa convencional: %s", rrC2.Body.String())
	}

	// 6) despacho finalizado PODE ser arquivado (não é mais pendência)
	rrA, resA := doJSONReq(app, "POST", fmt.Sprintf("/api/mensagens/%d/arquivar", msgID), nil, ckOp)
	if rrA.Code != http.StatusOK {
		t.Fatalf("arquivar despacho finalizado deve 200, veio %d: %v", rrA.Code, resA)
	}
}

// D4a: arquivar despacho EM ABERTO (não respondido, não finalizado) continua proibido.
func TestArquivarDespachoEmAbertoBloqueado(t *testing.T) {
	app, st, cleanup := setupTestApp(t)
	defer cleanup()

	ckGer := loginAsPapel(t, app, st, "gerd2", "gerente")
	ckOp := loginAsPapel(t, app, st, "opd2", "operador")
	msgID := enviarDespachoTeste(t, app, st, ckGer, "opd2")

	rr, res := doJSONReq(app, "POST", fmt.Sprintf("/api/mensagens/%d/arquivar", msgID), nil, ckOp)
	if rr.Code == http.StatusOK {
		t.Fatalf("arquivar despacho NÃO respondido deve ser bloqueado; veio 200: %v", res)
	}
}

// D4b: guardas de finalização.
func TestFinalizarGuardas(t *testing.T) {
	app, st, cleanup := setupTestApp(t)
	defer cleanup()

	ckGer := loginAsPapel(t, app, st, "gerg", "gerente")
	ckOp := loginAsPapel(t, app, st, "opg", "operador")
	ckOutro := loginAsPapel(t, app, st, "fora", "operador", "senha-outro")

	// mensagem COMUM não finaliza
	var papelGerID, papelOpID int64
	_ = st.db.QueryRow(`SELECT up.id FROM usuario_papeis up JOIN usuarios u ON u.id = up.usuario_id WHERE u.login='gerg'`).Scan(&papelGerID)
	_ = st.db.QueryRow(`SELECT up.id FROM usuario_papeis up JOIN usuarios u ON u.id = up.usuario_id WHERE u.login='opg'`).Scan(&papelOpID)
	rrC, resC := doJSONReq(app, "POST", "/api/mensagens", map[string]any{
		"destinatario_papel_ids": []int64{papelOpID},
		"assunto":                "Comum D",
		"corpo":                  "<p>informativo</p>",
		"tipo":                   "comum",
	}, ckGer)
	if rrC.Code != http.StatusOK {
		t.Fatalf("enviar comum: %d %v", rrC.Code, resC)
	}
	idv, _ := resC["id"].(float64)
	rr, _ := doJSONReq(app, "POST", fmt.Sprintf("/api/mensagens/%d/finalizar", int64(idv)), nil, ckOp)
	if rr.Code != http.StatusConflict {
		t.Fatalf("finalizar mensagem comum deve 409, veio %d", rr.Code)
	}

	// não-destinatário não finaliza despacho alheio
	msgID := enviarDespachoTeste(t, app, st, ckGer, "opg")
	rr2, _ := doJSONReq(app, "POST", fmt.Sprintf("/api/mensagens/%d/finalizar", msgID), nil, ckOutro)
	if rr2.Code != http.StatusForbidden {
		t.Fatalf("finalizar por não-destinatário deve 403, veio %d", rr2.Code)
	}

	// remetente não é quem finaliza (a ordem dá o botão a quem RECEBEU)
	rr3, _ := doJSONReq(app, "POST", fmt.Sprintf("/api/mensagens/%d/finalizar", msgID), nil, ckGer)
	if rr3.Code != http.StatusForbidden {
		t.Fatalf("finalizar pelo remetente deve 403, veio %d", rr3.Code)
	}
}

// D4c: finalizar é ÚNICO — re-finalizar despacho já finalizado → 409.
func TestFinalizarDuplicadoBloqueado(t *testing.T) {
	app, st, cleanup := setupTestApp(t)
	defer cleanup()

	ckGer := loginAsPapel(t, app, st, "gerfd", "gerente")
	ckOp := loginAsPapel(t, app, st, "opfd", "operador")
	msgID := enviarDespachoTeste(t, app, st, ckGer, "opfd")

	rr1, res1 := doJSONReq(app, "POST", fmt.Sprintf("/api/mensagens/%d/finalizar", msgID), nil, ckOp)
	if rr1.Code != http.StatusOK {
		t.Fatalf("1ª finalização deve 200, veio %d: %v", rr1.Code, res1)
	}
	rr2, res2 := doJSONReq(app, "POST", fmt.Sprintf("/api/mensagens/%d/finalizar", msgID), nil, ckOp)
	if rr2.Code != http.StatusConflict {
		t.Fatalf("2ª finalização deve 409, veio %d: %v", rr2.Code, res2)
	}

	// Selo único: finalizado_em gravado uma vez (hora explícita no banco).
	var conta int
	if err := st.db.QueryRow(`SELECT COUNT(*) FROM mensagens WHERE id = ? AND finalizado_em IS NOT NULL`, msgID).Scan(&conta); err != nil || conta != 1 {
		t.Fatalf("finalizado_em deve existir e ser único, conta=%d err=%v", conta, err)
	}
}

// D4d: resposta SEM finalizar continua possível — despacho respondido segue
// exigido até a finalização (ordem 05/10: resposta e finalização independentes).
func TestResponderSemFinalizarMantemPendencia(t *testing.T) {
	app, st, cleanup := setupTestApp(t)
	defer cleanup()

	ckGer := loginAsPapel(t, app, st, "gerrf", "gerente")
	ckOp := loginAsPapel(t, app, st, "oprf", "operador")
	msgID := enviarDespachoTeste(t, app, st, ckGer, "oprf")

	// responde SEM finalizar
	rrR, resR := doJSONReq(app, "POST", fmt.Sprintf("/api/mensagens/%d/responder", msgID), map[string]any{
		"corpo": "<p>Ciente, em execução.</p>",
	}, ckOp)
	if rrR.Code != http.StatusOK {
		t.Fatalf("responder deve 200, veio %d: %v", rrR.Code, resR)
	}

	// pendência BAIXOU para o destinatário (respondido_em gravado)…
	rrCnt, resCnt := doJSONReq(app, "GET", "/api/mensagens/contador", nil, ckOp)
	if rrCnt.Code != http.StatusOK {
		t.Fatalf("contador: %d", rrCnt.Code)
	}
	if pend, _ := resCnt["despachos_pendentes"].(float64); pend != 0 {
		t.Fatalf("após resposta a pendência do destinatário baixa, veio %v", resCnt)
	}

	// …mas o despacho NÃO foi finalizado: thread segue exigida até FINALIZAR.
	var exigeResp int
	var finalizado *string
	if err := st.db.QueryRow(`SELECT COALESCE(exige_resposta,0), finalizado_em FROM mensagens WHERE id = ?`, msgID).Scan(&exigeResp, &finalizado); err != nil {
		t.Fatalf("ler despacho: %v", err)
	}
	if exigeResp != 1 || finalizado != nil {
		t.Fatalf("resposta sem finalizar NÃO deve baixar exige_resposta nem selar finalizado_em: exige=%d fin=%v", exigeResp, finalizado)
	}

	// thread expõe o estado p/ o front: pendência minha baixada + não finalizado
	rrT, resT := doJSONReq(app, "GET", fmt.Sprintf("/api/mensagens/%d/thread", msgID), nil, ckOp)
	if rrT.Code != http.StatusOK {
		t.Fatalf("thread: %d", rrT.Code)
	}
	if fin, _ := resT["mensagem"].(map[string]any)["finalizado_em"]; fin != nil {
		t.Fatalf("thread não deveria reportar finalizado_em, veio %v", fin)
	}

	// agora FINALIZAR depois de responder deve 200 (ordem: resposta não impede finalização)
	rrF, resF := doJSONReq(app, "POST", fmt.Sprintf("/api/mensagens/%d/finalizar", msgID), nil, ckOp)
	if rrF.Code != http.StatusOK {
		t.Fatalf("finalizar pós-resposta deve 200, veio %d: %v", rrF.Code, resF)
	}
}

// D4e: abas separadas também nos ENVIADOS — ?despacho=1 devolve só despachos;
// sem o parâmetro, só mensagens convencionais (contrato espelha o inbox).
func TestEnviadasAbasDespacho(t *testing.T) {
	app, st, cleanup := setupTestApp(t)
	defer cleanup()

	ckGer := loginAsPapel(t, app, st, "gerenv", "gerente")
	_ = loginAsPapel(t, app, st, "openv", "operador")
	var papelOpID int64
	if err := st.db.QueryRow(`SELECT up.id FROM usuario_papeis up JOIN usuarios u ON u.id = up.usuario_id WHERE u.login='openv'`).Scan(&papelOpID); err != nil {
		t.Fatalf("papel operador: %v", err)
	}

	// 1 despacho + 1 comum enviados pelo gerente
	_ = enviarDespachoTeste(t, app, st, ckGer, "openv")
	rrC, resC := doJSONReq(app, "POST", "/api/mensagens", map[string]any{
		"destinatario_papel_ids": []int64{papelOpID},
		"assunto":                "Comum enviadas",
		"corpo":                  "<p>informativo</p>",
		"tipo":                   "comum",
	}, ckGer)
	if rrC.Code != http.StatusOK {
		t.Fatalf("enviar comum: %d %v", rrC.Code, resC)
	}

	rrD, resD := doJSONReq(app, "GET", "/api/mensagens/enviadas?despacho=1", nil, ckGer)
	if rrD.Code != http.StatusOK {
		t.Fatalf("enviadas?despacho=1: %d", rrD.Code)
	}
	if strings.Contains(rrD.Body.String(), "Comum enviadas") || !strings.Contains(rrD.Body.String(), "Despacho de prova D") {
		t.Fatalf("aba enviados-despachos deve trazer só despachos: %s", resD)
	}

	rrN, resN := doJSONReq(app, "GET", "/api/mensagens/enviadas", nil, ckGer)
	if rrN.Code != http.StatusOK {
		t.Fatalf("enviadas: %d", rrN.Code)
	}
	if strings.Contains(rrN.Body.String(), "Despacho de prova D") || !strings.Contains(rrN.Body.String(), "Comum enviadas") {
		t.Fatalf("aba enviados convencionais deve trazer só mensagens comuns: %s", resN)
	}
}

// Teste do assunto sem tags (ordem 04/10: "Título às vezes chega como
// <strong>alguma coisa</strong>, com marks — remediar").
func TestAssuntoSemTags(t *testing.T) {
	app, st, cleanup := setupTestApp(t)
	defer cleanup()

	ckGer := loginAsPapel(t, app, st, "geras", "gerente")
	_ = loginAsPapel(t, app, st, "opas", "operador")
	var papelOpID int64
	if err := st.db.QueryRow(`SELECT up.id FROM usuario_papeis up JOIN usuarios u ON u.id = up.usuario_id WHERE u.login='opas'`).Scan(&papelOpID); err != nil {
		t.Fatalf("papel operador: %v", err)
	}

	rr, res := doJSONReq(app, "POST", "/api/mensagens", map[string]any{
		"destinatario_papel_ids": []int64{papelOpID},
		"assunto":                "<strong>Ordem de Serviço</strong> &nbsp; 2/2026",
		"corpo":                  "<p>corpo normal</p>",
		"tipo":                   "comum",
	}, ckGer)
	if rr.Code != http.StatusOK {
		t.Fatalf("envio deve 200, veio %d: %v", rr.Code, res)
	}

	// Gravado SEM marcações: título puro
	var assunto string
	if err := st.db.QueryRow(`SELECT assunto FROM mensagens ORDER BY id DESC LIMIT 1`).Scan(&assunto); err != nil {
		t.Fatalf("ler assunto: %v", err)
	}
	if strings.ContainsAny(assunto, "<>&") || strings.Contains(assunto, "nbsp") {
		t.Fatalf("assunto deve vir limpo (sem tags/entidades), veio %q", assunto)
	}
	if !strings.Contains(assunto, "Ordem de Serviço") {
		t.Fatalf("assunto deve preservar o texto, veio %q", assunto)
	}
}
