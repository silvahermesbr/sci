package main

// Teste de regressão da ordem 08/10 — CONFERÊNCIA POR CONTEXTO ATIVO (governador):
// o chefe de múltiplos setores vê, lança e conclui APENAS o setor ativo no
// contexto da sessão; trocar de setor = trocar o contexto no dropdown.
// Prova o ciclo completo: contexto S1 → só S1 → contexto S2 → só S2,
// com 403 em toda ação cruzada (marcar/concluir setor alheio).

import (
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
)

func TestContextoGovemaConferenciaMultiSetor(t *testing.T) {
	app, _, cleanup := setupTestApp(t)
	defer cleanup()

	adminCookie := loginAs(t, app, "admin", "admin123")

	// 1. Grupo + gerente
	rr, res := doJSONReq(app, "POST", "/api/grupos", map[string]any{
		"nome": "G Govena", "login": "ger_gov", "senha": "senha12345",
		"nome_guerra": "Cap Govena",
	}, adminCookie)
	if rr.Code != http.StatusOK {
		t.Fatalf("falha ao criar grupo: %v", res)
	}
	var gid int64
	if err := app.st.db.QueryRow(`SELECT id FROM grupos WHERE nome = 'G Govena'`).Scan(&gid); err != nil {
		t.Fatalf("grupo não encontrado: %v", err)
	}

	gerCookie := loginAs(t, app, "ger_gov", "senha12345")

	// 2. Dois setores
	setores := map[string]int64{}
	for _, ns := range []struct{ nome, sigla string }{
		{"S1 Comando", "SC1"}, {"S2 Operacoes", "SO2"},
	} {
		rrS, resS := doJSONReq(app, "POST", "/api/catalogo/setores", map[string]any{
			"nome": ns.nome, "sigla": ns.sigla,
		}, gerCookie)
		if rrS.Code != http.StatusOK {
			t.Fatalf("falha ao criar setor %s: %v", ns.nome, resS)
		}
		var sid int64
		if err := app.st.db.QueryRow(`SELECT id FROM setores WHERE grupo_id = ? AND nome = ?`, gid, ns.nome).Scan(&sid); err != nil {
			t.Fatalf("setor %s não encontrado: %v", ns.nome, err)
		}
		setores[ns.sigla] = sid
	}
	s1, s2 := setores["SC1"], setores["SO2"]

	// 3. Chefe multi + nomeações
	rrU, resU := doJSONReq(app, "POST", "/api/usuarios", map[string]any{
		"login": "chefe_gov", "senha": "senha12345", "nome_guerra": "Sgt Govena",
		"papel": "chefe_setor", "grupo_id": gid,
	}, gerCookie)
	if rrU.Code != http.StatusOK {
		t.Fatalf("falha ao criar chefe: %v", resU)
	}
	var uid int64
	if err := app.st.db.QueryRow(`SELECT id FROM usuarios WHERE login = 'chefe_gov'`).Scan(&uid); err != nil {
		t.Fatalf("usuário não encontrado: %v", err)
	}
	for _, sid := range []int64{s1, s2} {
		rrN, resN := doJSONReq(app, "POST", fmt.Sprintf("/api/grupos/%d/nomear_chefe", gid), map[string]any{
			"usuario_id": uid, "setor_id": sid,
		}, gerCookie)
		if rrN.Code != http.StatusOK {
			t.Fatalf("falha ao nomear chefe no setor %d: %v", sid, resN)
		}
	}

	// 4. Pessoas: A1/A2 no S1, B1/B2 no S2
	pID := map[string]int64{}
	for _, p := range []struct{ guerra, completo string; setor int64 }{
		{"ALPHA UM", "ALPHA UM", s1}, {"ALPHA DOIS", "ALPHA DOIS", s1},
		{"BRAVO UM", "BRAVO UM", s2}, {"BRAVO DOIS", "BRAVO DOIS", s2},
	} {
		rrP, resP := doJSONReq(app, "POST", "/api/pessoas", map[string]any{
			"nome_guerra": p.guerra, "nome_completo": p.completo, "setor_id": p.setor,
		}, gerCookie)
		if rrP.Code != http.StatusOK {
			t.Fatalf("falha ao criar pessoa %s: %v", p.guerra, resP)
		}
		var outP struct {
			ID int64 `json:"id"`
		}
		_ = json.Unmarshal(rrP.Body.Bytes(), &outP)
		pID[p.guerra] = outP.ID
	}

	// 5. Conferência despachada para AMBOS os setores
	rrD, resD := doJSONReq(app, "POST", "/api/conferencia/despachar", map[string]any{
		"nome": "Conf Govena", "prazo_final": "23:59", "setores": []int64{s1, s2},
	}, gerCookie)
	if rrD.Code != http.StatusOK {
		t.Fatalf("falha ao despachar: %v", resD)
	}
	var outD struct {
		CID int64 `json:"conferencia_id"`
	}
	_ = json.Unmarshal(rrD.Body.Bytes(), &outD)
	if outD.CID <= 0 {
		t.Fatalf("conferencia_id inválido: %v", resD)
	}

	// 6. Login do chefe + papel IDs via /api/me
	chefeCookie := loginAs(t, app, "chefe_gov", "senha12345")
	rrMe, resMe := doJSONReq(app, "GET", "/api/me", nil, chefeCookie)
	if rrMe.Code != http.StatusOK {
		t.Fatalf("falha no /api/me: %v", resMe)
	}
	var me struct {
		Usuario struct {
			PapelAtivoID int64   `json:"papel_ativo_id"`
			SetorID      int64   `json:"setor_id"`
			Papeis       [] struct {
				ID      int64  `json:"id"`
				Papel   string `json:"papel"`
				SetorID int64  `json:"setor_id"`
			} `json:"papeis"`
		} `json:"usuario"`
	}
	_ = json.Unmarshal(rrMe.Body.Bytes(), &me)
	var pidS1, pidS2 int64
	for _, p := range me.Usuario.Papeis {
		if p.Papel != "chefe_setor" {
			continue
		}
		if p.SetorID == s1 {
			pidS1 = p.ID
		}
		if p.SetorID == s2 {
			pidS2 = p.ID
		}
	}
	if pidS1 == 0 || pidS2 == 0 {
		t.Fatalf("esperava papéis p/ S1 e S2 em papeis, obteve: %+v", me.Usuario.Papeis)
	}

	// fixa o contexto em S1 explicitamente
	rrC1, resC1 := doJSONReq(app, "POST", "/api/sessao/contexto", map[string]any{
		"papel_id": pidS1, "setor_id": s1,
	}, chefeCookie)
	if rrC1.Code != http.StatusOK {
		t.Fatalf("falha ao ativar contexto S1: %v", resC1)
	}

	tipoHoje := func() (sids []int64, pessoas []struct {
		Nome    string `json:"nome_guerra"`
		SetorID int64  `json:"setor_id"`
	}) {
		rrH, resH := doJSONReq(app, "GET", fmt.Sprintf("/api/conferencia/hoje?id=%d", outD.CID), nil, chefeCookie)
		if rrH.Code != http.StatusOK {
			t.Fatalf("falha no GET hoje: %v", resH)
		}
		var hoje struct {
			SetoresStatus []struct {
				SetorID int64 `json:"setor_id"`
			} `json:"setores_status"`
			Pessoas []struct {
				Nome    string `json:"nome_guerra"`
				SetorID int64  `json:"setor_id"`
			} `json:"pessoas"`
		}
		_ = json.Unmarshal(rrH.Body.Bytes(), &hoje)
		for _, s := range hoje.SetoresStatus {
			sids = append(sids, s.SetorID)
		}
		return sids, hoje.Pessoas
	}

	marcar := func(pessoa int64) int {
		rrM, _ := doJSONReq(app, "POST", "/api/conferencia/marcar", map[string]any{
			"pessoa_id": pessoa, "situacao": "presente", "verificado": true,
		}, chefeCookie)
		return rrM.Code
	}
	concluir := func(sid int64) int {
		rrC, _ := doJSONReq(app, "POST", fmt.Sprintf("/api/conferencia/%d/setor/%d/concluir", outD.CID, sid), map[string]any{}, chefeCookie)
		return rrC.Code
	}

	// ===== CONTEXTO S1 =====
	sids, pessoas := tipoHoje()
	if len(sids) != 1 || sids[0] != s1 {
		t.Fatalf("CTX S1: esperava setores_status=[S1], obteve %v", sids)
	}
	for _, p := range pessoas {
		if p.SetorID != s1 {
			t.Fatalf("CTX S1: pessoa %s do setor %d vazou no fio (esperava só S1)", p.Nome, p.SetorID)
		}
	}
	if len(pessoas) != 2 {
		t.Fatalf("CTX S1: esperava 2 pessoas (ALPHA*), obteve %d: %+v", len(pessoas), pessoas)
	}
	if code := marcar(pID["BRAVO UM"]); code != http.StatusForbidden {
		t.Fatalf("CTX S1: marcar pessoa do S2 deveria 403, veio %d", code)
	}
	if code := marcar(pID["ALPHA UM"]); code != http.StatusOK {
		t.Fatalf("CTX S1: marcar pessoa do S1 deveria 200, veio %d", code)
	}
	if code := concluir(s2); code != http.StatusForbidden {
		t.Fatalf("CTX S1: concluir setor S2 deveria 403, veio %d", code)
	}
	// estados (presenças) também seguem o contexto: o check do ALPHA UM vem no
	// fio; nenhum estado do S2 aparece mesmo com lançamento pré-existente.
	rrH1, _ := doJSONReq(app, "GET", fmt.Sprintf("/api/conferencia/hoje?id=%d", outD.CID), nil, chefeCookie)
	var hoje1 struct {
		Conferencia *struct {
			Estados map[string]struct {
				Verificado bool `json:"verificado"`
			} `json:"estados"`
		} `json:"conferencia"`
	}
	_ = json.Unmarshal(rrH1.Body.Bytes(), &hoje1)
	if hoje1.Conferencia == nil {
		t.Fatalf("CTX S1: conferencia não veio no fio")
	}
	if n := len(hoje1.Conferencia.Estados); n != 1 {
		t.Fatalf("CTX S1: esperava 1 estado no fio (ALPHA UM), obteve %d: %+v", n, hoje1.Conferencia.Estados)
	}

	// ===== TROCA: contexto S2 =====
	rrC2, resC2 := doJSONReq(app, "POST", "/api/sessao/contexto", map[string]any{
		"papel_id": pidS2, "setor_id": s2,
	}, chefeCookie)
	if rrC2.Code != http.StatusOK {
		t.Fatalf("falha ao ativar contexto S2: %v", resC2)
	}

	sids, pessoas = tipoHoje()
	if len(sids) != 1 || sids[0] != s2 {
		t.Fatalf("CTX S2: esperava setores_status=[S2], obteve %v", sids)
	}
	for _, p := range pessoas {
		if p.SetorID != s2 {
			t.Fatalf("CTX S2: pessoa %s do setor %d vazou no fio (esperava só S2)", p.Nome, p.SetorID)
		}
	}
	if len(pessoas) != 2 {
		t.Fatalf("CTX S2: esperava 2 pessoas (BRAVO*), obteve %d: %+v", len(pessoas), pessoas)
	}
	// CTX S2: o check do ALPHA UM (S1) NÃO pode vazar no fio de estados.
	rrH2, _ := doJSONReq(app, "GET", fmt.Sprintf("/api/conferencia/hoje?id=%d", outD.CID), nil, chefeCookie)
	var hoje2 struct {
		Conferencia *struct {
			Estados map[string]struct {
				Verificado bool `json:"verificado"`
			} `json:"estados"`
		} `json:"conferencia"`
	}
	_ = json.Unmarshal(rrH2.Body.Bytes(), &hoje2)
	if hoje2.Conferencia == nil {
		t.Fatalf("CTX S2: conferencia não veio no fio")
	}
	if n := len(hoje2.Conferencia.Estados); n != 0 {
		t.Fatalf("CTX S2: estados do S1 vazaram no fio (%d): %+v", n, hoje2.Conferencia.Estados)
	}
	if code := marcar(pID["ALPHA UM"]); code != http.StatusForbidden {
		t.Fatalf("CTX S2: marcar pessoa do S1 deveria 403, veio %d", code)
	}
	if code := marcar(pID["BRAVO UM"]); code != http.StatusOK {
		t.Fatalf("CTX S2: marcar pessoa do S2 deveria 200, veio %d", code)
	}
	if code := concluir(s1); code != http.StatusForbidden {
		t.Fatalf("CTX S2: concluir setor S1 deveria 403, veio %d", code)
	}
	if code := concluir(s2); code != http.StatusOK {
		t.Fatalf("CTX S2: concluir o PRÓPRIO setor ativo S2 deveria 200, veio %d", code)
	}
}
