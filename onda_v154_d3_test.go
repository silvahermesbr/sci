package main

// onda_v154_d3_test.go — v1.5.4-D3 (causa-raiz R-7): a ordenação do modo
// antiguidade tem que usar as MESMAS 3 fontes da tag do filtro (pessoa, conta,
// papel) e quem ficou fora do filtro tem que vir nomeado (sem_tag), não sumir
// em silêncio. Padrão do harness: setupTestApp + loginAs + doJSONReq.
//
// Cenário: os nomes foram escolhidos para que a ordem ALFABÉTICA (Aldo, Nadim,
// Quim, Ykki, Zub) seja DIFERENTE da ordem de ANTIGUIDADE (Zub → Ykki → Aldo) —
// assim é a listagem que prova a ordem, não a coincidência.
//   Zub  — tag SÓ na CONTA (usuarios.funcao_id) — antiguidade 1
//   Ykki — tag SÓ em usuario_papeis            — antiguidade 2
//   Aldo — tag na PESSOA (pessoas.funcao_id)   — antiguidade 3
//   Nadim — sem tag em nenhuma fonte, mesmo setor → entra no sem_tag
//   Quim  — sem tag em nenhuma fonte, OUTRO setor → entra no sem_tag só sem recorte

import (
	"bytes"
	"net/http"
	"strings"
	"testing"
)

type cenarioD3 struct {
	grupoID, setorID, outroSetorID int64
	ids                            map[string]int64
}

// semeiaCenarioD3: grupo com 3 tags de antiguidade, 2 setores e 5 militares.
func semeiaCenarioD3(t *testing.T, app *App, st *Store) cenarioD3 {
	t.Helper()

	res, err := st.db.Exec(`INSERT INTO grupos (nome, codigo) VALUES ('Cia D3', 'CD301')`)
	if err != nil {
		t.Fatalf("grupo: %v", err)
	}
	grupoID, _ := res.LastInsertId()

	tagTen := criaTagAntig(t, st, grupoID, "Tenente Gr D3", 1)
	tagSgt := criaTagAntig(t, st, grupoID, "Sargento Gr D3", 2)
	tagCb := criaTagAntig(t, st, grupoID, "Cabo Gr D3", 3)

	res, err = st.db.Exec(`INSERT INTO setores (nome, grupo_id) VALUES ('Comunicações D3', ?)`, grupoID)
	if err != nil {
		t.Fatalf("setor: %v", err)
	}
	setorID, _ := res.LastInsertId()
	res, err = st.db.Exec(`INSERT INTO setores (nome, grupo_id) VALUES ('Manutenção D3', ?)`, grupoID)
	if err != nil {
		t.Fatalf("outro setor: %v", err)
	}
	outroSetorID, _ := res.LastInsertId()

	pessoa := func(ng, nc string, setor, funcao any) int64 {
		t.Helper()
		var pid int64
		if err := st.db.QueryRow(`INSERT INTO pessoas (nome_guerra, nome_completo, grupo_id, setor_id, funcao_id, status)
			VALUES (?, ?, ?, ?, ?, 'ativo') RETURNING id`, ng, nc, grupoID, setor, funcao).Scan(&pid); err != nil {
			t.Fatalf("pessoa %s: %v", ng, err)
		}
		return pid
	}

	// Zub: tag SÓ na CONTA (usuarios.funcao_id) — a pessoa não tem funcao_id
	zub := pessoa("Zub", "Zub da Antiga", setorID, nil)
	hash, _ := hashSenha("senha12345")
	if _, err := st.db.Exec(`INSERT INTO usuarios (login, senha_hash, papel, grupo_id, pessoa_id, funcao_id, precisa_setup)
		VALUES ('zub_d3', ?, 'operador', ?, ?, ?, 0)`, hash, grupoID, zub, tagTen); err != nil {
		t.Fatalf("conta do Zub: %v", err)
	}

	// Ykki: tag SÓ em usuario_papeis — conta sem funcao_id + papel do grupo com a tag
	ykki := pessoa("Ykki", "Ykki Papel", setorID, nil)
	if _, err := st.db.Exec(`INSERT INTO usuarios (login, senha_hash, papel, grupo_id, pessoa_id, precisa_setup)
		VALUES ('ykki_d3', ?, 'operador', ?, ?, 0)`, hash, grupoID, ykki); err != nil {
		t.Fatalf("conta da Ykki: %v", err)
	}
	var ykkiUID int64
	if err := st.db.QueryRow(`SELECT id FROM usuarios WHERE login = 'ykki_d3'`).Scan(&ykkiUID); err != nil {
		t.Fatalf("conta ykki: %v", err)
	}
	if _, err := st.db.Exec(`INSERT INTO usuario_papeis (usuario_id, grupo_id, papel, funcao_id) VALUES (?, ?, 'operador', ?)`, ykkiUID, grupoID, tagSgt); err != nil {
		t.Fatalf("papel da Ykki: %v", err)
	}

	// Aldo: tag na PESSOA (fonte clássica) — sem conta
	aldo := pessoa("Aldo", "Aldo Pessoa", setorID, tagCb)

	// Nadim e Quim: sem tag em NENHUMA fonte — fora do filtro (sem_tag)
	nadim := pessoa("Nadim", "Nadim Sem Tag", setorID, nil)
	quim := pessoa("Quim", "Quim Outro Setor", outroSetorID, nil)

	// gerente do grupo
	var gerUID int64
	if err := st.db.QueryRow(`INSERT INTO usuarios (login, senha_hash, papel, grupo_id, precisa_setup)
		VALUES ('ger_d3', ?, 'gerente', ?, 0) RETURNING id`, hash, grupoID).Scan(&gerUID); err != nil {
		t.Fatalf("gerente: %v", err)
	}
	_, _ = st.db.Exec(`INSERT INTO usuario_papeis (usuario_id, grupo_id, papel) VALUES (?, ?, 'gerente')`, gerUID, grupoID)

	return cenarioD3{
		grupoID: grupoID, setorID: setorID, outroSetorID: outroSetorID,
		ids: map[string]int64{
			"zub": zub, "ykki": ykki, "aldo": aldo, "nadim": nadim, "quim": quim,
			"tag_ten": tagTen, "tag_sgt": tagSgt, "tag_cb": tagCb,
		},
	}
}

// idOrdemD3 extrai a sequência de pessoa_id dos itens (listagem/pré-fechamento).
func idOrdemD3(t *testing.T, itens []any) []int64 {
	t.Helper()
	out := []int64{}
	for _, it := range itens {
		m, ok := it.(map[string]any)
		if !ok {
			t.Fatalf("item inesperado: %v", it)
		}
		id, _ := m["id"].(float64)
		if id == 0 {
			id, _ = m["pessoa_id"].(float64)
		}
		out = append(out, int64(id))
	}
	return out
}

func iguaisOrdemD3(a, b []int64) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// (a) militar tagueado só na CONTA e outro só em usuario_papeis aparecem na
// listagem e na ordem de antiguidade correta — ante o tagueado na pessoa — na
// tela da conferência E no pré-fechamento.
func TestD3OrdemAntiguidadeUnificadaNasListagens(t *testing.T) {
	app, st, cleanup := setupTestApp(t)
	defer cleanup()

	c := semeiaCenarioD3(t, app, st)
	ckGer := loginAs(t, app, "ger_d3", "senha12345")
	filtro := []int64{c.ids["tag_ten"], c.ids["tag_sgt"], c.ids["tag_cb"]}
	escada := []int64{c.ids["zub"], c.ids["ykki"], c.ids["aldo"]}

	// tela da conferência (GET hoje): modo antiguidade, 3 convocados na escada
	rr, res := doJSONReq(app, "POST", "/api/conferencia/iniciar", map[string]any{
		"nome": "Conf D3 ordem", "funcao_ids": filtro,
	}, ckGer)
	if rr.Code != http.StatusOK {
		t.Fatalf("iniciar falhou: %d %v", rr.Code, res)
	}
	rr, res = doJSONReq(app, "GET", "/api/conferencia/hoje", nil, ckGer)
	if rr.Code != http.StatusOK {
		t.Fatalf("hoje falhou: %d", rr.Code)
	}
	if res["modo"] != "antiguidade" {
		t.Fatalf("esperava modo=antiguidade, obtive %v", res["modo"])
	}
	pessoas, _ := res["pessoas"].([]any)
	ordem := idOrdemD3(t, pessoas)
	if !iguaisOrdemD3(ordem, escada) {
		t.Fatalf("listagem fora da escada de antiguidade: esperava %v, veio %v (Zub só-conta → Ykki só-papel → Aldo pessoa)", escada, ordem)
	}

	// pré-fechamento do setor: MESMA escada (antes ordenava só por p.funcao_id)
	rr, res = doJSONReq(app, "POST", "/api/conferencia/despachar", map[string]any{
		"nome": "Conf D3 prefech", "setores": []int64{c.setorID}, "funcao_ids": filtro,
	}, ckGer)
	if rr.Code != http.StatusOK {
		t.Fatalf("despachar falhou: %d %v", rr.Code, res)
	}
	cid := int64(res["conferencia_id"].(float64))
	rr, res = doJSONReq(app, "GET", "/api/conferencia/"+itoa(cid)+"/setor/"+itoa(c.setorID)+"/pre_fechamento", nil, ckGer)
	if rr.Code != http.StatusOK {
		t.Fatalf("pre_fechamento falhou: %d %v", rr.Code, res)
	}
	itens, _ := res["itens"].([]any)
	if !iguaisOrdemD3(idOrdemD3(t, itens), escada) {
		t.Fatalf("pré-fechamento fora da escada: esperava %v, veio %v", escada, idOrdemD3(t, itens))
	}
}

// (b) iniciar conferência por antiguidade devolve sem_tag com os nomes dos
// militares ATIVOS que ficaram fora do filtro (nenhuma das 3 fontes), com o
// mesmo recorte de setor do despacho; conferência sem filtro não traz a chave.
func TestD3SemTagAvisaExcluidosAoIniciar(t *testing.T) {
	app, st, cleanup := setupTestApp(t)
	defer cleanup()

	c := semeiaCenarioD3(t, app, st)
	ckGer := loginAs(t, app, "ger_d3", "senha12345")
	filtro := []int64{c.ids["tag_ten"], c.ids["tag_sgt"], c.ids["tag_cb"]}

	// iniciar SEM setores: universo = grupo → excluídos Nadim (mesmo setor) e Quim (outro)
	rr, res := doJSONReq(app, "POST", "/api/conferencia/iniciar", map[string]any{
		"nome": "Conf D3 semtag", "funcao_ids": filtro,
	}, ckGer)
	if rr.Code != http.StatusOK {
		t.Fatalf("iniciar falhou: %d %v", rr.Code, res)
	}
	sem, _ := res["sem_tag"].([]any)
	var nomes []string
	for _, n := range sem {
		nomes = append(nomes, n.(string))
	}
	if len(nomes) != 2 || nomes[0] != "Nadim" || nomes[1] != "Quim" {
		t.Fatalf("sem_tag esperado [Nadim Quim], veio %v", res["sem_tag"])
	}

	// despachar COM setores: recorte = setor despachado → Quim (outro setor) sai do aviso
	rr, res = doJSONReq(app, "POST", "/api/conferencia/despachar", map[string]any{
		"nome": "Conf D3 semtag setor", "setores": []int64{c.setorID}, "funcao_ids": filtro,
	}, ckGer)
	if rr.Code != http.StatusOK {
		t.Fatalf("despachar falhou: %d %v", rr.Code, res)
	}
	sem, _ = res["sem_tag"].([]any)
	if len(sem) != 1 || sem[0] != "Nadim" {
		t.Fatalf("sem_tag do despacho (recorte de setor) esperado [Nadim], veio %v", res["sem_tag"])
	}

	// conferência por SETORES (sem filtro) não tem quem avisar — sem a chave
	rr, res = doJSONReq(app, "POST", "/api/conferencia/iniciar", map[string]any{
		"nome": "Conf D3 normal",
	}, ckGer)
	if rr.Code != http.StatusOK {
		t.Fatalf("iniciar sem filtro falhou: %d %v", rr.Code, res)
	}
	if _, tem := res["sem_tag"]; tem {
		t.Fatalf("conferência sem filtro não deveria trazer sem_tag: %v", res["sem_tag"])
	}
	_ = st
}

// (c) negativo: grupo sem NENHUMA tag → comportamento atual preservado
// (avisoSemTagsAntiguidade no picker + 400 ao tentar criar por antiguidade).
func TestD3GrupoSemTagsPreservado(t *testing.T) {
	app, st, cleanup := setupTestApp(t)
	defer cleanup()

	c := semeiaCenarioD3(t, app, st) // só p/ ter uma tag de OUTRO grupo à mão
	hash, _ := hashSenha("senha12345")
	res, err := st.db.Exec(`INSERT INTO grupos (nome, codigo) VALUES ('Cia D3 Vazia', 'CD302')`)
	if err != nil {
		t.Fatalf("grupo vazio: %v", err)
	}
	gVazio, _ := res.LastInsertId()
	var gerUID int64
	if err := st.db.QueryRow(`INSERT INTO usuarios (login, senha_hash, papel, grupo_id, precisa_setup)
		VALUES ('ger_d3_vazio', ?, 'gerente', ?, 0) RETURNING id`, hash, gVazio).Scan(&gerUID); err != nil {
		t.Fatalf("gerente vazio: %v", err)
	}
	_, _ = st.db.Exec(`INSERT INTO usuario_papeis (usuario_id, grupo_id, papel) VALUES (?, ?, 'gerente')`, gerUID, gVazio)
	ckVazio := loginAs(t, app, "ger_d3_vazio", "senha12345")

	// picker: lista vazia + aviso canônico
	rr, res2 := doJSONReq(app, "GET", "/api/conferencia/funcoes-antiguidade", nil, ckVazio)
	if rr.Code != http.StatusOK {
		t.Fatalf("picker falhou: %d", rr.Code)
	}
	if res2["tem_tags"] != false {
		t.Fatalf("esperava tem_tags=false: %v", res2)
	}
	if av, _ := res2["aviso"].(string); av != avisoSemTagsAntiguidade {
		t.Fatalf("aviso esperado %q, veio %q", avisoSemTagsAntiguidade, res2["aviso"])
	}

	// criar por antiguidade continua bloqueado (tag de outro grupo não vale)
	rr, _ = doJSONReq(app, "POST", "/api/conferencia/iniciar", map[string]any{
		"nome": "Conf sem tags", "funcao_ids": []int64{c.ids["tag_ten"]},
	}, ckVazio)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("grupo sem tags: criar por antiguidade deveria bloquear (400), obtive %d", rr.Code)
	}
}

// (d) relatório em tela e PDF saem na MESMA escada do modo antiguidade
// (antes: tela ordenava situacao/nome e o PDF por caminho de função).
func TestD3RelatorioTelaEPdfNaEscada(t *testing.T) {
	app, st, cleanup := setupTestApp(t)
	defer cleanup()

	c := semeiaCenarioD3(t, app, st)
	ckGer := loginAs(t, app, "ger_d3", "senha12345")
	filtro := []int64{c.ids["tag_ten"], c.ids["tag_sgt"], c.ids["tag_cb"]}
	escada := []string{"Zub", "Ykki", "Aldo"}

	rr, res := doJSONReq(app, "POST", "/api/conferencia/iniciar", map[string]any{
		"nome": "Conf D3 relatorio", "funcao_ids": filtro,
	}, ckGer)
	if rr.Code != http.StatusOK {
		t.Fatalf("iniciar falhou: %d %v", rr.Code, res)
	}
	cid := int64(res["conferencia_id"].(float64))

	for _, ng := range escada {
		var pid int64
		if err := st.db.QueryRow(`SELECT id FROM pessoas WHERE nome_guerra = ? AND grupo_id = ?`, ng, c.grupoID).Scan(&pid); err != nil {
			t.Fatalf("pessoa %s: %v", ng, err)
		}
		rr, res = doJSONReq(app, "POST", "/api/conferencia/marcar?id="+itoa(cid), map[string]any{
			"pessoa_id": pid, "situacao": "presente", "verificado": true,
		}, ckGer)
		if rr.Code != http.StatusOK {
			t.Fatalf("marcar %s falhou: %d %v", ng, rr.Code, res)
		}
	}

	// fecha com os 3 lançamentos verificados
	lancs := []map[string]any{}
	for _, ng := range escada {
		var pid int64
		_ = st.db.QueryRow(`SELECT id FROM pessoas WHERE nome_guerra = ? AND grupo_id = ?`, ng, c.grupoID).Scan(&pid)
		lancs = append(lancs, map[string]any{"pessoa_id": pid, "situacao": "presente", "verificado": true})
	}
	rr, res = doJSONReq(app, "POST", "/api/conferencia/fechar", map[string]any{
		"id": cid, "lancamentos": lancs,
	}, ckGer)
	if rr.Code != http.StatusOK {
		t.Fatalf("fechar falhou: %d %v", rr.Code, res)
	}

	// relatório em tela na escada
	rr, res = doJSONReq(app, "GET", "/api/conferencia/"+itoa(cid), nil, ckGer)
	if rr.Code != http.StatusOK {
		t.Fatalf("relatório em tela falhou: %d", rr.Code)
	}
	lanc, _ := res["lancamentos"].([]any)
	var emTela []string
	for _, l := range lanc {
		m, _ := l.(map[string]any)
		emTela = append(emTela, m["nome_guerra"].(string))
	}
	if len(emTela) != 3 || !iguaisStrD3(emTela, escada) {
		t.Fatalf("relatório em tela fora da escada: esperava %v, veio %v", escada, emTela)
	}

	// PDF na mesma escada
	rrPdf, corpo := doRawReq(app, "GET", "/api/conferencia/"+itoa(cid)+"/relatorio.pdf", nil, ckGer)
	if rrPdf.Code != http.StatusOK {
		t.Fatalf("pdf falhou: %d", rrPdf.Code)
	}
	if !bytes.HasPrefix(corpo, []byte("%PDF-1.")) {
		t.Fatalf("resposta não é PDF")
	}
	txt := extrairTextoPDF(t, corpo)
	iz := strings.Index(txt, "Zub")
	iy := strings.Index(txt, "Ykki")
	ia := strings.Index(txt, "Aldo")
	if iz < 0 || iy < 0 || ia < 0 {
		t.Fatalf("nomes ausentes no PDF (Zub=%d Ykki=%d Aldo=%d): %.200q", iz, iy, ia, txt)
	}
	if !(iz < iy && iy < ia) {
		t.Fatalf("PDF fora da escada (Zub@%d, Ykki@%d, Aldo@%d)", iz, iy, ia)
	}
}

func iguaisStrD3(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
