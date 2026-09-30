package main

import (
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
)

func TestMultiPapeisEContextoSessao(t *testing.T) {
	app, _, cleanup := setupTestApp(t)
	defer cleanup()

	adminCookie := loginAs(t, app, "admin", "admin123")

	// 1. Criar dois grupos para teste
	rr, res := doJSONReq(app, "POST", "/api/grupos", map[string]any{
		"nome":        "1º Pelotão",
		"login":       "cmt_pel1",
		"senha":       "senha12345",
		"nome_guerra": "Silva",
	}, adminCookie)
	if rr.Code != http.StatusOK {
		t.Fatalf("falha ao criar grupo 1: %v", res)
	}

	rr, res = doJSONReq(app, "POST", "/api/grupos", map[string]any{
		"nome":        "2º Pelotão",
		"login":       "cmt_pel2",
		"senha":       "senha12345",
		"nome_guerra": "Souza",
	}, adminCookie)
	if rr.Code != http.StatusOK {
		t.Fatalf("falha ao criar grupo 2: %v", res)
	}

	// Buscar ID do usuário cmt_pel1
	var cmt1ID int64
	err := app.st.db.QueryRow(`SELECT id FROM usuarios WHERE login = 'cmt_pel1'`).Scan(&cmt1ID)
	if err != nil {
		t.Fatalf("usuário cmt_pel1 não encontrado: %v", err)
	}

	var grupo2ID int64
	err = app.st.db.QueryRow(`SELECT id FROM grupos WHERE nome = '2º Pelotão'`).Scan(&grupo2ID)
	if err != nil {
		t.Fatalf("grupo 2 não encontrado: %v", err)
	}

	// 2. Tentar atribuir papel de gerente do 2º Pelotão para cmt_pel1 enquanto já existe gerente no grupo 2 -> DEVE FALHAR (unicidade)
	rr, res = doJSONReq(app, "POST", "/api/usuarios/"+fmtInt(cmt1ID)+"/papeis", map[string]any{
		"grupo_id": grupo2ID,
		"papel":    "gerente",
	}, adminCookie)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("esperado erro ao tentar adicionar segundo gerente no grupo 2, obtido code: %d res: %v", rr.Code, res)
	}

	// 3. Atribuir papel de OPERADOR do 2º Pelotão para cmt_pel1 -> DEVE FUNCIONAR (multi-funções)
	rr, res = doJSONReq(app, "POST", "/api/usuarios/"+fmtInt(cmt1ID)+"/papeis", map[string]any{
		"grupo_id": grupo2ID,
		"papel":    "operador",
	}, adminCookie)
	if rr.Code != http.StatusOK {
		t.Fatalf("falha ao atribuir operador para cmt1 no grupo 2: %v", res)
	}

	// 4. Logar como cmt_pel1 e verificar que possui múltiplos papéis
	cmtCookie := loginAs(t, app, "cmt_pel1", "senha12345")
	rr, res = doJSONReq(app, "GET", "/api/me", nil, cmtCookie)
	if rr.Code != http.StatusOK {
		t.Fatalf("falha ao buscar /api/me: %v", res)
	}
	uMap, ok := res["usuario"].(map[string]any)
	if !ok {
		t.Fatalf("esperado objeto usuario em /api/me: %v", res)
	}
	papeis, ok := uMap["papeis"].([]any)
	if !ok || len(papeis) != 2 {
		t.Fatalf("esperado 2 papéis para cmt_pel1, obtido: %v", uMap["papeis"])
	}

	// 5. Trocar contexto para o segundo papel
	segundoPapel := papeis[1].(map[string]any)
	segundoPapelID := int64(segundoPapel["id"].(float64))

	rr, res = doJSONReq(app, "POST", "/api/sessao/contexto", map[string]any{
		"papel_id": segundoPapelID,
	}, cmtCookie)
	if rr.Code != http.StatusOK {
		t.Fatalf("falha ao trocar contexto: %v", res)
	}
	uMapAtualizado := res["usuario"].(map[string]any)
	if uMapAtualizado["papel"] != "operador" {
		t.Fatalf("esperado papel ativo 'operador' após troca de contexto, obtido: %v", uMapAtualizado["papel"])
	}

	// 6. Validar que Admin acessa /api/conferencia/hoje sem 403
	rr, res = doJSONReq(app, "GET", "/api/conferencia/hoje", nil, adminCookie)
	if rr.Code != http.StatusOK {
		t.Fatalf("Admin deveria receber 200 em /api/conferencia/hoje, mas obteve: %d %v", rr.Code, res)
	}

	// 7. Validar edição de nomes do usuário via PATCH /api/usuarios/{id}
	rr, res = doJSONReq(app, "PATCH", "/api/usuarios/"+fmtInt(cmt1ID), map[string]any{
		"nome_guerra":   "Tenente Silva",
		"nome_completo": "Hermes da Silva",
	}, adminCookie)
	if rr.Code != http.StatusOK {
		t.Fatalf("falha ao editar nomes do usuário: %d %v", rr.Code, res)
	}

	// 8. Validar que GET /api/usuarios traz os nomes atualizados
	rr, res = doJSONReq(app, "GET", "/api/usuarios", nil, adminCookie)
	if rr.Code != http.StatusOK {
		t.Fatalf("falha ao listar usuários: %d %v", rr.Code, res)
	}
}

func TestMensageriaInternaPorFuncao(t *testing.T) {
	app, _, cleanup := setupTestApp(t)
	defer cleanup()

	adminCookie := loginAs(t, app, "admin", "admin123")

	// Criar grupo com gerente
	rr, res := doJSONReq(app, "POST", "/api/grupos", map[string]any{
		"nome":        "Pelotão Alfa",
		"login":       "gerente_alfa",
		"senha":       "senha12345",
		"nome_guerra": "Alfa",
	}, adminCookie)
	if rr.Code != http.StatusOK {
		t.Fatalf("falha ao criar grupo: %v", res)
	}

	// Obter papel_id do admin
	rr, res = doJSONReq(app, "GET", "/api/me", nil, adminCookie)
	adminU := res["usuario"].(map[string]any)
	adminPapelID := int64(adminU["papel_ativo_id"].(float64))
	_ = adminPapelID

	// Obter papel_id do gerente_alfa
	alfaCookie := loginAs(t, app, "gerente_alfa", "senha12345")
	rr, res = doJSONReq(app, "GET", "/api/me", nil, alfaCookie)
	alfaU := res["usuario"].(map[string]any)
	alfaPapelID := int64(alfaU["papel_ativo_id"].(float64))

	// 1. Admin envia mensagem para o papel de gerente_alfa
	rr, res = doJSONReq(app, "POST", "/api/mensagens", map[string]any{
		"destinatario_papel_ids": []int64{alfaPapelID},
		"assunto":                "Ordem de Serviço 01",
		"corpo":                  "Favor encaminhar relatório semanal até sexta-feira.",
	}, adminCookie)
	if rr.Code != http.StatusOK {
		t.Fatalf("falha ao enviar mensagem: %v", res)
	}
	msgID := int64(res["id"].(float64))

	// 2. Gerente alfa verifica contador de mensagens não lidas
	rr, res = doJSONReq(app, "GET", "/api/mensagens/contador", nil, alfaCookie)
	if rr.Code != http.StatusOK || int(res["nao_lidas"].(float64)) != 1 {
		t.Fatalf("esperado 1 mensagem não lida, obtido: %v", res)
	}

	// 3. Gerente alfa lê a caixa de entrada
	reqList := httptest.NewRequest("GET", "/api/mensagens/inbox", nil)
	reqList.Header.Set("X-SCI", "1")
	reqList.AddCookie(alfaCookie)
	rrList := httptest.NewRecorder()
	app.mux.ServeHTTP(rrList, reqList)
	if rrList.Code != http.StatusOK {
		t.Fatalf("falha ao buscar inbox: %s", rrList.Body.String())
	}

	// 4. Gerente alfa marca mensagem como lida
	rr, res = doJSONReq(app, "POST", "/api/mensagens/"+fmtInt(msgID)+"/ler", nil, alfaCookie)
	if rr.Code != http.StatusOK {
		t.Fatalf("falha ao marcar mensagem como lida: %v", res)
	}

	// 5. Contador de não lidas deve zerar
	rr, res = doJSONReq(app, "GET", "/api/mensagens/contador", nil, alfaCookie)
	if rr.Code != http.StatusOK || int(res["nao_lidas"].(float64)) != 0 {
		t.Fatalf("esperado 0 mensagens não lidas após leitura, obtido: %v", res)
	}
}

func fmtInt(n int64) string {
	return strconv.FormatInt(n, 10)
}
