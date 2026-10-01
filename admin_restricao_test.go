package main

import (
	"net/http"
	"testing"
)

func TestAdminRestricaoOperacional(t *testing.T) {
	app, _, cleanup := setupTestApp(t)
	defer cleanup()

	adminCookie := loginAs(t, app, "admin", "admin123")

	// 1. Módulo DRIVE: Admin deve ser terminantemente proibido (HTTP 403)
	t.Run("Drive Restrito ao Admin", func(t *testing.T) {
		rr, _ := doJSONReq(app, "GET", "/api/drive/itens", nil, adminCookie)
		if rr.Code != http.StatusForbidden {
			t.Errorf("GET /api/drive/itens deveria retornar 403, obteve %d", rr.Code)
		}

		rr, _ = doJSONReq(app, "POST", "/api/drive/pastas", map[string]any{"nome": "Pasta Proibida"}, adminCookie)
		if rr.Code != http.StatusForbidden {
			t.Errorf("POST /api/drive/pastas deveria retornar 403, obteve %d", rr.Code)
		}

		rr, _ = doJSONReq(app, "GET", "/api/drive/download/1", nil, adminCookie)
		if rr.Code != http.StatusForbidden {
			t.Errorf("GET /api/drive/download/1 deveria retornar 403, obteve %d", rr.Code)
		}

		rr, _ = doJSONReq(app, "POST", "/api/drive/compartilhar", map[string]any{"pasta_id": 1, "alvo_id": 1, "alvo_tipo": "usuario"}, adminCookie)
		if rr.Code != http.StatusForbidden {
			t.Errorf("POST /api/drive/compartilhar deveria retornar 403, obteve %d", rr.Code)
		}

		rr, _ = doJSONReq(app, "GET", "/api/drive/compartilhamentos?pasta_id=1", nil, adminCookie)
		if rr.Code != http.StatusForbidden {
			t.Errorf("GET /api/drive/compartilhamentos deveria retornar 403, obteve %d", rr.Code)
		}
	})

	// 2. Módulo CALENDÁRIO: Admin deve ser terminantemente proibido (HTTP 403)
	t.Run("Calendário Restrito ao Admin", func(t *testing.T) {
		rr, _ := doJSONReq(app, "GET", "/api/calendario/visao", nil, adminCookie)
		if rr.Code != http.StatusForbidden {
			t.Errorf("GET /api/calendario/visao deveria retornar 403, obteve %d", rr.Code)
		}

		rr, _ = doJSONReq(app, "POST", "/api/calendario/eventos", map[string]any{
			"titulo":      "Evento Admin Proibido",
			"data_inicio": "2026-10-15T08:00",
		}, adminCookie)
		if rr.Code != http.StatusForbidden {
			t.Errorf("POST /api/calendario/eventos deveria retornar 403, obteve %d", rr.Code)
		}

		rr, _ = doJSONReq(app, "DELETE", "/api/calendario/eventos/1", nil, adminCookie)
		if rr.Code != http.StatusForbidden {
			t.Errorf("DELETE /api/calendario/eventos/1 deveria retornar 403, obteve %d", rr.Code)
		}

		rr, _ = doJSONReq(app, "POST", "/api/calendario/compartilhar", map[string]any{"evento_id": 1, "alvo_id": 1, "alvo_tipo": "usuario"}, adminCookie)
		if rr.Code != http.StatusForbidden {
			t.Errorf("POST /api/calendario/compartilhar deveria retornar 403, obteve %d", rr.Code)
		}

		rr, _ = doJSONReq(app, "GET", "/api/calendario/compartilhamentos?evento_id=1", nil, adminCookie)
		if rr.Code != http.StatusForbidden {
			t.Errorf("GET /api/calendario/compartilhamentos deveria retornar 403, obteve %d", rr.Code)
		}
	})

	// 3. Módulo MENSAGENS E AVISOS: Admin deve ser terminantemente proibido (HTTP 403)
	t.Run("Mensagens e Avisos Restritos ao Admin", func(t *testing.T) {
		rr, _ := doJSONReq(app, "GET", "/api/mensagens/inbox", nil, adminCookie)
		if rr.Code != http.StatusForbidden {
			t.Errorf("GET /api/mensagens/inbox deveria retornar 403, obteve %d", rr.Code)
		}

		rr, _ = doJSONReq(app, "GET", "/api/mensagens/enviadas", nil, adminCookie)
		if rr.Code != http.StatusForbidden {
			t.Errorf("GET /api/mensagens/enviadas deveria retornar 403, obteve %d", rr.Code)
		}

		rr, _ = doJSONReq(app, "POST", "/api/mensagens", map[string]any{
			"assunto": "Teste",
			"corpo":   "Mensagem",
		}, adminCookie)
		if rr.Code != http.StatusForbidden {
			t.Errorf("POST /api/mensagens deveria retornar 403, obteve %d", rr.Code)
		}

		rr, _ = doJSONReq(app, "GET", "/api/mensagens/contador", nil, adminCookie)
		if rr.Code != http.StatusForbidden {
			t.Errorf("GET /api/mensagens/contador deveria retornar 403, obteve %d", rr.Code)
		}

		rr, _ = doJSONReq(app, "GET", "/api/avisos", nil, adminCookie)
		if rr.Code != http.StatusForbidden {
			t.Errorf("GET /api/avisos deveria retornar 403, obteve %d", rr.Code)
		}

		rr, _ = doJSONReq(app, "POST", "/api/avisos", map[string]any{
			"titulo":   "Aviso Proibido",
			"conteudo": "Conteudo",
		}, adminCookie)
		if rr.Code != http.StatusForbidden {
			t.Errorf("POST /api/avisos deveria retornar 403, obteve %d", rr.Code)
		}
	})

	// 4. Módulo ADMIN & SISTEMA: Admin DEVE continuar tendo pleno acesso gerencial global
	t.Run("Gestão Global Permitida ao Admin", func(t *testing.T) {
		rr, res := doJSONReq(app, "GET", "/api/usuarios", nil, adminCookie)
		if rr.Code != http.StatusOK {
			t.Errorf("GET /api/usuarios deveria retornar 200, obteve %d (%v)", rr.Code, res)
		}

		rr, res = doJSONReq(app, "GET", "/api/grupos", nil, adminCookie)
		if rr.Code != http.StatusOK {
			t.Errorf("GET /api/grupos deveria retornar 200, obteve %d (%v)", rr.Code, res)
		}

		rr, res = doJSONReq(app, "GET", "/api/relatorio/dados", nil, adminCookie)
		if rr.Code != http.StatusOK {
			t.Errorf("GET /api/relatorio/dados deveria retornar 200, obteve %d (%v)", rr.Code, res)
		}

		rr, res = doJSONReq(app, "POST", "/api/configuracoes", map[string]string{
			"MODO_RESERVA": "0",
		}, adminCookie)
		if rr.Code != http.StatusOK {
			t.Errorf("POST /api/configuracoes deveria retornar 200, obteve %d (%v)", rr.Code, res)
		}
	})
}
