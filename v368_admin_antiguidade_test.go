package main

// Missão v368 — Admin: catálogo nos modais é SÓ ANTIGUIDADE (never cadeiras).
//
// O endpoint /api/catalogo/funcoes já filtra (tipo='antiguidade' OR tipo IS NULL)
// desde 3c538b7 (f3). Estes testes CRAVAM esse contrato: se alguém remover o
// filtro, os modais do Admin voltam a listar cadeiras de grupo (Encarregado de
// Pessoal/Material, Gerente) como se fossem grau de antiguidade — o defeito
// reportado pelo Diretor em 09/10.
//
// Padrão do harness: setupTestApp + doJSONReq/loginAs (v1_test.go).

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

// TestCatalogoFuncoesSomenteAntiguidade: o envelope do catálogo NUNCA contém
// cadeiras de grupo — com seed completa (3 cadeiras v39 + 2 tags de antiguidade).
func TestCatalogoFuncoesSomenteAntiguidade(t *testing.T) {
	app, st, cleanup := setupTestApp(t)
	defer cleanup()

	// seed: 2 itens de antiguidade (ordem de exibição 2 e 1 — fora da ordem de
	// inserção, para provar o ORDER BY de antiguidade) + as 3 cadeiras do sistema.
	var fCabo, fSd int64
	if err := st.db.QueryRow(`INSERT INTO funcoes (nome, grupo_id, tipo, antiguidade) VALUES ('Cabo (v368)', NULL, 'antiguidade', 2) RETURNING id`).Scan(&fCabo); err != nil {
		t.Fatalf("funcao cabo: %v", err)
	}
	if err := st.db.QueryRow(`INSERT INTO funcoes (nome, grupo_id, tipo, antiguidade) VALUES ('Soldado EV (v368)', NULL, 'antiguidade', 1) RETURNING id`).Scan(&fSd); err != nil {
		t.Fatalf("funcao sd: %v", err)
	}
	// cadeiras do sistema: já SEMEADAS pela v39 (chave UNIQUE) — reutiliza as linhas.
	var encPes, ger int64
	if err := st.db.QueryRow(`SELECT id FROM funcoes WHERE chave = 'enc_pessoal'`).Scan(&encPes); err != nil {
		t.Fatalf("cadeira enc_pessoal semeada não encontrada: %v", err)
	}
	if err := st.db.QueryRow(`SELECT id FROM funcoes WHERE tipo = 'grupo' AND LOWER(nome) = 'gerente'`).Scan(&ger); err != nil {
		t.Fatalf("cadeira gerente semeada não encontrada: %v", err)
	}
	// legado v367: tipo NULL = antiguidade (não pode sumir do catálogo)
	var legado int64
	if err := st.db.QueryRow(`INSERT INTO funcoes (nome, grupo_id) VALUES ('Legado Sem Tipo (v368)', NULL) RETURNING id`).Scan(&legado); err != nil {
		t.Fatalf("funcao legada: %v", err)
	}

	ck := loginAs(t, app, "admin", "admin123")
	rr, res := doJSONReq(app, "GET", "/api/catalogo/funcoes", nil, ck)
	if rr.Code != http.StatusOK {
		t.Fatalf("catálogo: %d", rr.Code)
	}

	// aceita envelope lista (legado) ou objeto {funcoes:[…]} (v368)
	var itens []map[string]any
	switch env := res["funcoes"].(type) {
	case []any:
		for _, e := range env {
			if m, ok := e.(map[string]any); ok {
				itens = append(itens, m)
			}
		}
	case nil:
		if rr.Body != nil && strings.Contains(rr.Body.String(), "[") {
			var lista []map[string]any
			if err := json.Unmarshal(rr.Body.Bytes(), &lista); err == nil {
				itens = lista
			}
		}
	}

	nomes := []string{}
	idsOrdem := []int64{}
	for _, it := range itens {
		nome, _ := it["nome"].(string)
		nomes = append(nomes, nome)
		if f, ok := it["id"].(float64); ok {
			idsOrdem = append(idsOrdem, int64(f))
		}
	}

	// cadeiras NUNCA aparecem
	proibidos := []string{"Encarregado de Pessoal", "Encarregado de Material", "Gerente"}
	for _, n := range nomes {
		for _, p := range proibidos {
			if strings.Contains(n, p) {
				t.Fatalf("CADEIRA vazou no catálogo de antiguidade: %q em %v", n, nomes)
			}
		}
	}
	_ = encPes
	_ = ger

	// antiguidade tipo='antiguidade' presente
	viuCabo, viuSd := false, false
	for _, n := range nomes {
		if strings.Contains(n, "Cabo (v368)") {
			viuCabo = true
		}
		if strings.Contains(n, "Soldado EV (v368)") {
			viuSd = true
		}
	}
	if !viuCabo || !viuSd {
		t.Fatalf("antiguidade sumiu do catálogo: %v", nomes)
	}

	// legado tipo NULL presente (compat v367)
	viuLegado := false
	for _, n := range nomes {
		if strings.Contains(n, "Legado Sem Tipo (v368)") {
			viuLegado = true
		}
	}
	if !viuLegado {
		t.Fatalf("função legada (tipo NULL) não pode sumir do catálogo: %v", nomes)
	}

	// ordem de antiguidade: Sd (1) ANTES de Cabo (2)
	iSd, iCabo := -1, -1
	for i, id := range idsOrdem {
		if id == fSd {
			iSd = i
		}
		if id == fCabo {
			iCabo = i
		}
	}
	if iSd == -1 || iCabo == -1 || iSd > iCabo {
		t.Fatalf("ordem de antiguidade violada (Sd deve vir antes de Cabo): ids=%v", idsOrdem)
	}
}

// TestCatalogoFuncoesNaoListaCadeirasSeed: com as cadeiras SEMEADAS pela v39
// (store.go) e ZERO antiguidade no grupo, o catálogo vem VAZIO — nunca as 3 cadeiras.
func TestCatalogoFuncoesNaoListaCadeirasSeed(t *testing.T) {
	app, _, cleanup := setupTestApp(t)
	defer cleanup()

	ck := loginAs(t, app, "admin", "admin123")
	rr, res := doJSONReq(app, "GET", "/api/catalogo/funcoes", nil, ck)
	if rr.Code != http.StatusOK {
		t.Fatalf("catálogo: %d", rr.Code)
	}

	// aceita envelope lista (legado) ou objeto {funcoes:[…]} (v368)
	var itens []map[string]any
	switch env := res["funcoes"].(type) {
	case []any:
		for _, e := range env {
			if m, ok := e.(map[string]any); ok {
				itens = append(itens, m)
			}
		}
	case nil:
		if rr.Body != nil && strings.Contains(rr.Body.String(), "[") {
			var lista []map[string]any
			if err := json.Unmarshal(rr.Body.Bytes(), &lista); err == nil {
				itens = lista
			}
		}
	}
	if len(itens) != 0 {
		nomes := []string{}
		for _, it := range itens {
			n, _ := it["nome"].(string)
			nomes = append(nomes, n)
		}
		t.Fatalf("banco SEM antiguidade: catálogo devia vir vazio, veio %d item(ns): %v", len(itens), nomes)
	}
}
