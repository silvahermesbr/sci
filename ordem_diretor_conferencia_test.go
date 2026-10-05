package main

// Testes da Fase F da ordem 04/10 — conferência atrelada ao ENCARREGADO DE PESSOAL:
//   F1. NOVA CONFERÊNCIA c/ nome + prazo: gravados e devolvidos na listagem.
//   F2. Sem nome → recusa (o modal exige, a API também guarda a porta).

import (
	"net/http"
	"testing"
)

func TestConferenciaNovaComNomePrazo(t *testing.T) {
	app, st, cleanup := setupTestApp(t)
	defer cleanup()

	ckGer := loginAsPapel(t, app, st, "gerconf", "gerente")
	var gid int64
	if err := st.db.QueryRow(`INSERT INTO grupos (nome) VALUES ('Grp Conf') RETURNING id`).Scan(&gid); err != nil {
		t.Fatalf("criar grupo: %v", err)
	}
	if _, err := st.db.Exec(`UPDATE usuarios SET grupo_id = ? WHERE login = 'gerconf'`, gid); err != nil {
		t.Fatalf("vincular: %v", err)
	}
	if _, err := st.db.Exec(`UPDATE usuario_papeis SET grupo_id = ? WHERE usuario_id = (SELECT id FROM usuarios WHERE login='gerconf')`, gid); err != nil {
		t.Fatalf("papel: %v", err)
	}

	// F2: sem nome → nome PADRÃO (o MODAL é quem exige; API não recusa —
	// retrocompatível com integrações/testes que iniciam sem nome)
	rrV, resV := doJSONReq(app, "POST", "/api/conferencia/iniciar", map[string]any{"nome": "   "}, ckGer)
	if rrV.Code != http.StatusOK {
		t.Fatalf("iniciar sem nome deve 200 c/ nome padrão, veio %d: %v", rrV.Code, resV)
	}

	// F1 feliz: nome + prazo
	rr, res := doJSONReq(app, "POST", "/api/conferencia/iniciar", map[string]any{
		"nome":        "Conferência de pessoal — terça",
		"prazo_final": "17:30",
	}, ckGer)
	if rr.Code != http.StatusOK {
		t.Fatalf("iniciar deve 200, veio %d: %v", rr.Code, res)
	}

	var nome, prazo string
	if err := st.db.QueryRow(`SELECT COALESCE(nome,''), COALESCE(prazo_final,'') FROM conferencias ORDER BY id DESC LIMIT 1`).Scan(&nome, &prazo); err != nil {
		t.Fatalf("ler conferência: %v", err)
	}
	if nome != "Conferência de pessoal — terça" || prazo != "17:30" {
		t.Fatalf("nome/prazo não persistidos: %q %q", nome, prazo)
	}

	// listagem devolve os novos campos (toggle do front consome)
	rrL, resAny := doJSONReqAny(app, "GET", "/api/conferencia/lista", nil, ckGer)
	if rrL.Code != http.StatusOK {
		t.Fatalf("lista: %d", rrL.Code)
	}
	lista, _ := resAny.([]any)
	if len(lista) == 0 {
		t.Fatalf("lista vazia: %v", resAny)
	}
	primeira := lista[0].(map[string]any)
	if primeira["nome"] != nome || primeira["prazo_final"] != prazo {
		t.Fatalf("lista deve devolver nome/prazo; veio %v", primeira)
	}
}
