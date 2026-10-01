package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

func doJSONReqAny(app *App, method, path string, body any, cookie *http.Cookie) (*httptest.ResponseRecorder, any) {
	var bodyReader *bytes.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		bodyReader = bytes.NewReader(b)
	} else {
		bodyReader = bytes.NewReader([]byte{})
	}
	req := httptest.NewRequest(method, path, bodyReader)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-SCI", "1")
	if cookie != nil {
		req.AddCookie(cookie)
	}
	rr := httptest.NewRecorder()
	app.mux.ServeHTTP(rr, req)

	var res any
	_ = json.Unmarshal(rr.Body.Bytes(), &res)
	return rr, res
}

func TestFase2DespachosEAvisos(t *testing.T) {
	app, _, cleanup := setupTestApp(t)
	defer cleanup()

	adminCookie := loginAs(t, app, "admin", "admin123")

	// 1. Criar dois grupos com seus respectivos gerentes
	rr, res := doJSONReq(app, "POST", "/api/grupos", map[string]any{
		"nome":        "Companhia Alfa",
		"login":       "gerente_alfa",
		"senha":       "senha12345",
		"nome_guerra": "Alfa",
	}, adminCookie)
	if rr.Code != http.StatusOK {
		t.Fatalf("falha ao criar grupo Alfa: %v", res)
	}

	rr, res = doJSONReq(app, "POST", "/api/grupos", map[string]any{
		"nome":        "Companhia Bravo",
		"login":       "gerente_bravo",
		"senha":       "senha12345",
		"nome_guerra": "Bravo",
	}, adminCookie)
	if rr.Code != http.StatusOK {
		t.Fatalf("falha ao criar grupo Bravo: %v", res)
	}

	// Login como gerente Alfa e Bravo
	alfaCookie := loginAs(t, app, "gerente_alfa", "senha12345")
	bravoCookie := loginAs(t, app, "gerente_bravo", "senha12345")

	rr, res = doJSONReq(app, "GET", "/api/me", nil, alfaCookie)
	alfaU := res["usuario"].(map[string]any)
	alfaPapelID := int64(alfaU["papel_ativo_id"].(float64))

	rr, res = doJSONReq(app, "GET", "/api/me", nil, bravoCookie)
	bravoU := res["usuario"].(map[string]any)
	bravoPapelID := int64(bravoU["papel_ativo_id"].(float64))

	// 2. Gerente Alfa envia um DESPACHO com retorno exigido para Gerente Bravo
	rr, res = doJSONReq(app, "POST", "/api/mensagens", map[string]any{
		"destinatario_papel_ids": []int64{bravoPapelID},
		"assunto":                "Diligência Urgente - Efetivo",
		"corpo":                  "Favor informar militares aptos para a missão até 18:00.",
		"tipo":                   "despacho",
		"exige_resposta":         true,
		"anexos": []map[string]any{
			{"nome": "diretriz.pdf", "tamanho": 1024, "dados_base64": "data:application/pdf;base64,AAAA"},
		},
	}, alfaCookie)
	if rr.Code != http.StatusOK {
		t.Fatalf("falha ao emitir despacho: %v", res)
	}
	despachoID := int64(res["id"].(float64))

	// 3. Gerente Bravo verifica inbox e vê o despacho
	rrAny, resAny := doJSONReqAny(app, "GET", "/api/mensagens/inbox?despacho=1", nil, bravoCookie)
	if rrAny.Code != http.StatusOK {
		t.Fatalf("falha ao listar despachos: %v", resAny)
	}
	inbox := resAny.([]any)
	if len(inbox) != 1 {
		t.Fatalf("esperado 1 despacho na caixa de Bravo, obtido: %d", len(inbox))
	}
	msgItem := inbox[0].(map[string]any)
	if msgItem["exige_resposta"] != true {
		t.Fatalf("esperado exige_resposta=true no despacho")
	}

	// 4. Teste de Bloqueio: Gerente Bravo tenta arquivar ou excluir sem responder
	rr, _ = doJSONReq(app, "POST", fmt.Sprintf("/api/mensagens/%d/arquivar", despachoID), nil, bravoCookie)
	if rr.Code == http.StatusOK {
		t.Fatalf("esperado erro ao tentar arquivar despacho com resposta pendente")
	}

	rr, _ = doJSONReq(app, "POST", fmt.Sprintf("/api/mensagens/%d/excluir", despachoID), nil, bravoCookie)
	if rr.Code == http.StatusOK {
		t.Fatalf("esperado erro ao tentar excluir despacho com resposta pendente")
	}

	// 5. Gerente Bravo responde ao despacho (informando pai_id)
	rr, res = doJSONReq(app, "POST", "/api/mensagens", map[string]any{
		"destinatario_papel_ids": []int64{alfaPapelID},
		"assunto":                "Re: Diligência Urgente - Efetivo",
		"corpo":                  "Militares destacados: CB Silva, SD Souza.",
		"tipo":                   "comum",
		"pai_id":                 despachoID,
	}, bravoCookie)
	if rr.Code != http.StatusOK {
		t.Fatalf("falha ao responder despacho: %v", res)
	}

	// 6. Agora o despacho deve constar como atendido (respondido_em preenchido)
	rrAny, resAny = doJSONReqAny(app, "GET", "/api/mensagens/inbox", nil, bravoCookie)
	inbox = resAny.([]any)
	msgItem = inbox[0].(map[string]any)
	if msgItem["respondido_em"] == nil {
		t.Fatalf("esperado respondido_em não-nulo após envio da resposta ao despacho")
	}

	// 7. Agora Gerente Bravo PODE arquivar o despacho
	rr, res = doJSONReq(app, "POST", fmt.Sprintf("/api/mensagens/%d/arquivar", despachoID), nil, bravoCookie)
	if rr.Code != http.StatusOK {
		t.Fatalf("falha ao arquivar despacho atendido: %v", res)
	}

	// 8. Fórum de Avisos: Gerente Alfa publica aviso no mural
	rr, res = doJSONReq(app, "POST", "/api/avisos", map[string]any{
		"titulo":   "Reunião de Oficiais",
		"conteudo": "Apresentação no auditório amanhã às 08h00.",
		"fixado":   true,
	}, alfaCookie)
	if rr.Code != http.StatusOK {
		t.Fatalf("falha ao publicar aviso: %v", res)
	}
	avisoID := int64(res["id"].(float64))

	// 9. Gerente Alfa registra comentário
	rr, res = doJSONReq(app, "POST", fmt.Sprintf("/api/avisos/%d/comentar", avisoID), map[string]any{
		"comentario": "Uniforme 4º R2.",
	}, alfaCookie)
	if rr.Code != http.StatusOK {
		t.Fatalf("falha ao comentar aviso: %v", res)
	}

	// 10. Consulta detalhes do aviso
	rr, res = doJSONReq(app, "GET", fmt.Sprintf("/api/avisos/%d/detalhes", avisoID), nil, alfaCookie)
	if rr.Code != http.StatusOK {
		t.Fatalf("falha ao obter detalhes do aviso: %v", res)
	}
	comentarios := res["comentarios"].([]any)
	if len(comentarios) != 1 {
		t.Fatalf("esperado 1 comentário no aviso, obtido: %d", len(comentarios))
	}
}
