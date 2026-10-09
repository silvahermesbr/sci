package main

// Regressão 09/10: o envelope {funcoes,total} da v368 estava no FIM do
// hCatalogoList sem escopo do parâmetro {t} e embrulhava TODOS os catálogos
// (setores, tags, destinos, …). O front consome esses catálogos como array
// (ativosDe/forEach) → TypeError → toast "Falha ao carregar a tela" ao abrir
// o módulo Pessoal como gerente (repro: roteiro_repro_0910.py).
// Contrato cravado aqui: envelope SOMENTE em /api/catalogo/funcoes (v368);
// os demais catálogos voltam a devolver array cru.

import (
	"encoding/json"
	"net/http"
	"testing"
)

func catálogoArray(t *testing.T, app *App, caminho string, ck *http.Cookie) []map[string]any {
	t.Helper()
	rr, _ := doJSONReq(app, "GET", caminho, nil, ck)
	if rr.Code != http.StatusOK {
		t.Fatalf("%s: %d", caminho, rr.Code)
	}
	corpo := rr.Body.String()
	if len(corpo) > 0 && corpo[0] == '{' {
		t.Fatalf("%s voltou OBJETO (envelope vazando): %s", caminho, corpo)
	}
	var lista []map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &lista); err != nil {
		t.Fatalf("%s não é array: %v — corpo: %s", caminho, err, corpo)
	}
	return lista
}

func TestCatalogoEnvelopeSoEmFuncoes(t *testing.T) {
	app, st, cleanup := setupTestApp(t)
	defer cleanup()

	// seed: um item em cada catálogo de fronteira do bug
	if _, err := st.db.Exec(`INSERT INTO setores (nome) VALUES ('Seção Env (v369)')`); err != nil {
		t.Fatalf("setor: %v", err)
	}
	if _, err := st.db.Exec(`INSERT INTO funcoes (nome, tipo, antiguidade) VALUES ('Cabo Env (v369)', 'antiguidade', 1)`); err != nil {
		t.Fatalf("funcao: %v", err)
	}
	if _, err := st.db.Exec(`INSERT INTO tags (nome) VALUES ('tag-env-v369')`); err != nil {
		t.Fatalf("tag: %v", err)
	}

	ck := loginAs(t, app, "admin", "admin123")

	// funções: envelope {funcoes,total} PRESERVADO (contrato v368)
	rr, _ := doJSONReq(app, "GET", "/api/catalogo/funcoes", nil, ck)
	if rr.Code != http.StatusOK {
		t.Fatalf("funcoes: %d", rr.Code)
	}
	var env struct {
		Funcoes []map[string]any `json:"funcoes"`
		Total   int              `json:"total"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &env); err != nil {
		t.Fatalf("funcoes sem envelope {funcoes,total}: %v — corpo: %s", err, rr.Body.String())
	}
	achou := false
	for _, f := range env.Funcoes {
		if f["nome"] == "Cabo Env (v369)" {
			achou = true
		}
	}
	if !achou || env.Total < 1 || len(env.Funcoes) != env.Total {
		t.Fatalf("funcoes: item não veio no envelope (achou=%v, total=%d, len=%d)", achou, env.Total, len(env.Funcoes))
	}

	// setores: array cru — é o que a ViewPessoal (ativosDe) consome
	setores := catálogoArray(t, app, "/api/catalogo/setores", ck)
	achouSet := false
	for _, s := range setores {
		if s["nome"] == "Seção Env (v369)" {
			achouSet = true
			if at, ok := s["ativo"].(float64); !ok || at != 1 {
				t.Fatalf("setores: ativo inesperado: %v", s["ativo"])
			}
		}
	}
	if !achouSet {
		t.Fatalf("setores: item semeado ausente (%d linhas)", len(setores))
	}

	// tags: array cru
	tags := catálogoArray(t, app, "/api/catalogo/tags", ck)
	achouTag := false
	for _, tg := range tags {
		if tg["nome"] == "tag-env-v369" {
			achouTag = true
		}
	}
	if !achouTag {
		t.Fatalf("tags: item semeado ausente (%d linhas)", len(tags))
	}
}
