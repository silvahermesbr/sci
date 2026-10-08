package main

import (
	"fmt"
	"net/http"
	"testing"
)

func TestMultiSetorChefeNoMesmoGrupo(t *testing.T) {
	app, _, cleanup := setupTestApp(t)
	defer cleanup()

	adminCookie := loginAs(t, app, "admin", "admin123")

	// 1. Criar grupo "Companhia Alpha"
	rr, res := doJSONReq(app, "POST", "/api/grupos", map[string]any{
		"nome":        "Companhia Alpha",
		"login":       "gerente_alpha",
		"senha":       "senha12345",
		"nome_guerra": "Capitão Alpha",
	}, adminCookie)
	if rr.Code != http.StatusOK {
		t.Fatalf("falha ao criar grupo: %v", res)
	}
	var gid int64
	err := app.st.db.QueryRow(`SELECT id FROM grupos WHERE nome = 'Companhia Alpha'`).Scan(&gid)
	if err != nil {
		t.Fatalf("grupo não encontrado: %v", err)
	}

	gerenteCookie := loginAs(t, app, "gerente_alpha", "senha12345")

	// 2. Criar 3 setores no grupo: "Comando", "1º Pelotão", "2º Pelotão"
	setores := []string{"Comando", "1º Pelotão", "2º Pelotão"}
	var setorIDs []int64
	for _, sNome := range setores {
		rrS, resS := doJSONReq(app, "POST", "/api/catalogo/setores", map[string]any{
			"nome":     sNome,
			"sigla":    sNome[:2],
			"grupo_id": gid,
		}, gerenteCookie)
		if rrS.Code != http.StatusOK {
			t.Fatalf("falha ao criar setor %s: %v", sNome, resS)
		}
		var sid int64
		if err := app.st.db.QueryRow(`SELECT id FROM setores WHERE grupo_id = ? AND nome = ?`, gid, sNome).Scan(&sid); err != nil {
			t.Fatalf("setor %s não encontrado: %v", sNome, err)
		}
		setorIDs = append(setorIDs, sid)
	}

	// 3. Criar usuário "chefe_multi" no grupo
	rrU, resU := doJSONReq(app, "POST", "/api/usuarios", map[string]any{
		"login":       "chefe_multi",
		"senha":       "senha12345",
		"nome_guerra": "Sargento Multi",
		"papel":       "chefe_setor",
		"grupo_id":    gid,
	}, gerenteCookie)
	if rrU.Code != http.StatusOK {
		t.Fatalf("falha ao criar usuário: %v", resU)
	}
	var uID int64
	if err := app.st.db.QueryRow(`SELECT id FROM usuarios WHERE login = 'chefe_multi'`).Scan(&uID); err != nil {
		t.Fatalf("usuário não encontrado: %v", err)
	}

	// 4. Nomear "chefe_multi" para cada um dos 3 setores
	for _, sid := range setorIDs {
		rrN, resN := doJSONReq(app, "POST", fmt.Sprintf("/api/grupos/%d/nomear_chefe", gid), map[string]any{
			"usuario_id": uID,
			"setor_id":   sid,
		}, gerenteCookie)
		if rrN.Code != http.StatusOK {
			t.Fatalf("falha ao nomear chefe para setor %d: %v", sid, resN)
		}
	}

	// 5. Logar como "chefe_multi" e verificar resposta imediata de login
	rrL, resL := doJSONReq(app, "POST", "/api/login", map[string]any{
		"login": "chefe_multi",
		"senha": "senha12345",
	}, nil)
	if rrL.Code != http.StatusOK {
		t.Fatalf("falha no login: %v", resL)
	}
	uLogin, ok := resL["usuario"].(map[string]any)
	if !ok {
		t.Fatalf("esperado objeto usuario no login: %v", resL)
	}
	if uLogin["papel"] != "chefe_setor" {
		t.Fatalf("esperado papel chefe_setor no login, obtido: %v", uLogin["papel"])
	}
	if uLogin["grupo_nome"] != "Companhia Alpha" {
		t.Fatalf("esperado grupo_nome 'Companhia Alpha', obtido: %v", uLogin["grupo_nome"])
	}
	if uLogin["setor_nome"] == nil || uLogin["setor_nome"] == "" {
		t.Fatalf("esperado setor_nome presente no login, obtido: %v", uLogin["setor_nome"])
	}

	// Verificar lista de papéis retornada
	papeis, ok := uLogin["papeis"].([]any)
	if !ok {
		t.Fatalf("esperado array papeis: %v", uLogin["papeis"])
	}

	// Deve ter 3 papéis de chefe_setor (um para cada setor) + possível operador original se mantido
	chefeSetorCount := 0
	setorNomesVistos := map[string]bool{}
	var papelID int64
	for _, pItem := range papeis {
		pMap := pItem.(map[string]any)
		if pMap["papel"] == "chefe_setor" {
			chefeSetorCount++
			papelID = int64(pMap["id"].(float64))
			if sn, ok := pMap["setor_nome"].(string); ok {
				setorNomesVistos[sn] = true
			}
		}
	}

	if chefeSetorCount != 3 {
		t.Fatalf("esperava 3 funções de chefe_setor no dropdown/papeis, obteve %d: %v", chefeSetorCount, papeis)
	}
	for _, sNome := range setores {
		if !setorNomesVistos[sNome] {
			t.Fatalf("setor %s não encontrado na lista de funções: %v", sNome, setorNomesVistos)
		}
	}

	// 6. Testar troca de contexto para o terceiro setor ("2º Pelotão")
	chefeCookie := loginAs(t, app, "chefe_multi", "senha12345")
	alvoSetorID := setorIDs[2] // 2º Pelotão
	rrCtx, resCtx := doJSONReq(app, "POST", "/api/sessao/contexto", map[string]any{
		"papel_id": papelID,
		"setor_id": alvoSetorID,
	}, chefeCookie)
	if rrCtx.Code != http.StatusOK {
		t.Fatalf("falha ao trocar contexto de setor: %v", resCtx)
	}

	uCtx, ok := resCtx["usuario"].(map[string]any)
	if !ok {
		t.Fatalf("esperado usuario na resposta de contexto: %v", resCtx)
	}
	if int64(uCtx["setor_id"].(float64)) != alvoSetorID {
		t.Fatalf("esperava setor_id %d, obteve %v", alvoSetorID, uCtx["setor_id"])
	}
	if uCtx["setor_nome"] != "2º Pelotão" {
		t.Fatalf("esperava setor_nome '2º Pelotão', obteve %v", uCtx["setor_nome"])
	}

	// 7. Validar GET /api/me reflete a troca sem F5
	rrMe, resMe := doJSONReq(app, "GET", "/api/me", nil, chefeCookie)
	if rrMe.Code != http.StatusOK {
		t.Fatalf("falha ao chamar /api/me: %v", resMe)
	}
	uMe := resMe["usuario"].(map[string]any)
	if int64(uMe["setor_id"].(float64)) != alvoSetorID {
		t.Fatalf("esperava setor_id %d em /api/me, obteve %v", alvoSetorID, uMe["setor_id"])
	}
	if uMe["setor_nome"] != "2º Pelotão" {
		t.Fatalf("esperava setor_nome '2º Pelotão' em /api/me, obteve %v", uMe["setor_nome"])
	}
}
