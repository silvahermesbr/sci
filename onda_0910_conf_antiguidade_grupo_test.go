package main

// CORREÇÃO 09/10 (ordem do dono): a conferência por antiguidade usa a escada
// de antiguidade DO GRUPO (tags mantidas pelo gerente no catálogo do grupo) —
// nunca a seed global inventada pela v40.
//   T1: grupo A com tags próprias → picker devolve EXATAMENTE essas, ordenadas.
//   T2: grupo B com outras tags → picker da conferência de B só devolve as de B.
//   T3: grupo sem tags → lista vazia + flag de aviso; front compilado serve o aviso.
//   T4: conferência pré-existente apontando p/ id da seed global abre sem erro (compat).

import (
	"net/http"
	"os"
	"strings"
	"testing"
)

// criaTagAntig: helper do teste — cadastra tag de antiguidade num grupo.
func criaTagAntig(t *testing.T, st *Store, grupoID int64, nome string, antig int) int64 {
	t.Helper()
	var id int64
	if err := st.db.QueryRow(`INSERT INTO funcoes (nome, grupo_id, tipo, antiguidade) VALUES (?, ?, 'antiguidade', ?) RETURNING id`, nome, grupoID, antig).Scan(&id); err != nil {
		t.Fatalf("tag %s: %v", nome, err)
	}
	return id
}

func TestPickerAntiguidadeSomenteDoGrupo(t *testing.T) {
	app, st, cleanup := setupTestApp(t)
	defer cleanup()

	// Grupo A com tags [Sd EV, Sd EP, Cb] NESTA ordem de antiguidade
	res, err := st.db.Exec(`INSERT INTO grupos (nome, codigo) VALUES ('Cia A', 'CPKA')`)
	if err != nil {
		t.Fatalf("grupo A: %v", err)
	}
	gA, _ := res.LastInsertId()
	sdEV := criaTagAntig(t, st, gA, "Sd EV", 1)
	sdEP := criaTagAntig(t, st, gA, "Sd EP", 2)
	criaTagAntig(t, st, gA, "Cb", 3)

	hashPadrao, _ := hashSenha("senha12345")
	var gerA int64
	if err := st.db.QueryRow(`INSERT INTO usuarios (login, senha_hash, papel, grupo_id, precisa_setup) VALUES ('ger_picker_a', ?, 'gerente', ?, 0) RETURNING id`, hashPadrao, gA).Scan(&gerA); err != nil {
		t.Fatalf("gerente A: %v", err)
	}
	_, _ = st.db.Exec(`INSERT INTO usuario_papeis (usuario_id, grupo_id, papel) VALUES (?, ?, 'gerente')`, gerA, gA)
	ckA := loginAs(t, app, "ger_picker_a", "senha12345")

	// T1: endpoint do picker retorna EXATAMENTE as 3 tags do grupo, ordenadas
	rr, res2 := doJSONReq(app, "GET", "/api/conferencia/funcoes-antiguidade", nil, ckA)
	if rr.Code != http.StatusOK {
		t.Fatalf("picker antiguidade falhou: %d %v", rr.Code, res2)
	}
	if res2["tem_tags"] != true {
		t.Fatalf("esperava tem_tags=true: %v", res2)
	}
	fl, _ := res2["funcoes"].([]any)
	if len(fl) != 3 {
		t.Fatalf("esperava 3 funções do grupo, obtive %d: %v", len(fl), res2["funcoes"])
	}
	ordem := []int64{sdEV, sdEP}
	for i, exp := range ordem {
		m, _ := fl[i].(map[string]any)
		if int64(m["id"].(float64)) != exp {
			t.Fatalf("ordem errada na pos %d: esperava id %d, veio %v", i, exp, m["id"])
		}
	}
	for _, it := range fl {
		m, _ := it.(map[string]any)
		if m["nome"].(string) == "Coronel" || m["nome"].(string) == "Praças" {
			t.Fatalf("seed global vazou no picker: %v", m)
		}
	}

	// T2: grupo B com outras tags → picker da conferência de B só devolve as de B
	resB, err := st.db.Exec(`INSERT INTO grupos (nome, codigo) VALUES ('Cia B', 'CPKB')`)
	if err != nil {
		t.Fatalf("grupo B: %v", err)
	}
	gB, _ := resB.LastInsertId()
	// nomes únicos globais (funcoes.nome é UNIQUE — restrito global já existente)
	cbB := criaTagAntig(t, st, gB, "Cb Gr B", 1)
	criaTagAntig(t, st, gB, "Ten Gr B", 2)
	var gerB int64
	if err := st.db.QueryRow(`INSERT INTO usuarios (login, senha_hash, papel, grupo_id, precisa_setup) VALUES ('ger_picker_b', ?, 'gerente', ?, 0) RETURNING id`, hashPadrao, gB).Scan(&gerB); err != nil {
		t.Fatalf("gerente B: %v", err)
	}
	_, _ = st.db.Exec(`INSERT INTO usuario_papeis (usuario_id, grupo_id, papel) VALUES (?, ?, 'gerente')`, gerB, gB)
	ckB := loginAs(t, app, "ger_picker_b", "senha12345")

	rr, resB2 := doJSONReq(app, "GET", "/api/conferencia/funcoes-antiguidade", nil, ckB)
	if rr.Code != http.StatusOK {
		t.Fatalf("picker B falhou: %d", rr.Code)
	}
	flB, _ := resB2["funcoes"].([]any)
	if len(flB) != 2 {
		t.Fatalf("grupo B esperava 2 funções, obtive %d", len(flB))
	}
	m0, _ := flB[0].(map[string]any)
	if int64(m0["id"].(float64)) != cbB {
		t.Fatalf("grupo B: ordem errada, esperava Cb=%d primeiro, veio %v", cbB, flB)
	}
	// a conferência de B NÃO vê as tags de A
	rr, _ = doJSONReq(app, "POST", "/api/conferencia/iniciar", map[string]any{
		"nome":       "Conf B",
		"funcao_ids": []int64{sdEV}, // tag do grupo A
	}, ckB)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("tag de outro grupo deveria ser rejeitada (400), obtive %d", rr.Code)
	}

	// T3: grupo C sem nenhuma tag → lista vazia + aviso no payload
	resC, err := st.db.Exec(`INSERT INTO grupos (nome, codigo) VALUES ('Cia C', 'CPKC')`)
	if err != nil {
		t.Fatalf("grupo C: %v", err)
	}
	gC, _ := resC.LastInsertId()
	var gerC int64
	if err := st.db.QueryRow(`INSERT INTO usuarios (login, senha_hash, papel, grupo_id, precisa_setup) VALUES ('ger_picker_c', ?, 'gerente', ?, 0) RETURNING id`, hashPadrao, gC).Scan(&gerC); err != nil {
		t.Fatalf("gerente C: %v", err)
	}
	_, _ = st.db.Exec(`INSERT INTO usuario_papeis (usuario_id, grupo_id, papel) VALUES (?, ?, 'gerente')`, gerC, gC)
	ckC := loginAs(t, app, "ger_picker_c", "senha12345")

	rr, resC2 := doJSONReq(app, "GET", "/api/conferencia/funcoes-antiguidade", nil, ckC)
	if rr.Code != http.StatusOK {
		t.Fatalf("picker C falhou: %d", rr.Code)
	}
	if resC2["tem_tags"] != false {
		t.Fatalf("esperava tem_tags=false: %v", resC2)
	}
	flC, _ := resC2["funcoes"].([]any)
	if len(flC) != 0 {
		t.Fatalf("grupo sem tags deveria devolver lista vazia, veio %v", flC)
	}
	if av, _ := resC2["aviso"].(string); !strings.Contains(av, "Grupo sem tags de antiguidade") {
		t.Fatalf("aviso ausente no payload: %v", resC2["aviso"])
	}
	// e o BACK bloqueia criar conferência por antiguidade sem tags
	rr, _ = doJSONReq(app, "POST", "/api/conferencia/iniciar", map[string]any{
		"nome":       "Conf C sem tags",
		"funcao_ids": []int64{sdEV}, // tag de OUTRO grupo → não vale
	}, ckC)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("grupo sem tags: criar por antiguidade deveria bloquear (400), obtive %d", rr.Code)
	}
}

// T3 (prova estática): o front compilado/servido contém o aviso — a SPA embutida
// serve web/views_conf.js direto do binário (go:embed), então o arquivo embutido
// É o asset servido.
func TestFrontCompiladoContemAvisoSemTags(t *testing.T) {
	b, err := os.ReadFile("web/views_conf.js")
	if err != nil {
		t.Fatalf("asset do front: %v", err)
	}
	src := string(b)
	if !strings.Contains(src, "Grupo sem tags de antiguidade") {
		t.Fatalf("views_conf.js não contém o aviso R3")
	}
	if !strings.Contains(src, "/api/conferencia/funcoes-antiguidade") {
		t.Fatalf("views_conf.js não consome o endpoint escopado do picker")
	}
	// picker NÃO pode mais beber o catálogo completo (fonte do vazamento global)
	if strings.Contains(src, "let funcoes = [];\n    try { funcoes = await api('/api/catalogo/funcoes'); }") {
		t.Fatalf("modal ainda carrega o picker do catálogo global")
	}
}

// T4 (compat R4): conferência pré-existente com filtro apontando p/ id da seed
// global (funcoes.grupo_id NULL preservado pela v42) abre e confere sem erro.
func TestConferenciaLegadaSeedGlobalAbreSemErro(t *testing.T) {
	app, st, cleanup := setupTestApp(t)
	defer cleanup()

	grupoID, _, _, _, _, _ := semeiaCenarioAntiguidade(t, app, st)
	ckGer := loginAs(t, app, "ger_conf_a", "senha12345")

	// simula banco LEGADO: seed global da v40 presente (grupo_id NULL) e
	// conferência existente referenciando um desses ids via conferencia_funcoes.
	var fLegada int64
	if err := st.db.QueryRow(`INSERT INTO funcoes (nome, grupo_id, tipo, antiguidade) VALUES ('Cabo (legado)', NULL, 'antiguidade', 50) RETURNING id`).Scan(&fLegada); err != nil {
		t.Fatalf("funcao legada: %v", err)
	}
	var tipoID int64
	if err := st.db.QueryRow(`INSERT INTO conferencia_tipos (nome) VALUES ('Tipo Legado T4') RETURNING id`).Scan(&tipoID); err != nil {
		t.Fatalf("tipo: %v", err)
	}
	var cid int64
	if err := st.db.QueryRow(`INSERT INTO conferencias (data, tipo_id, local, grupo_id, criado_por, criado_em, nome, status) VALUES (date('now'), ?, '', ?, 1, strftime('%Y-%m-%dT%H:%M:%fZ','now'), 'Conf legada', 'aberta') RETURNING id`, tipoID, grupoID).Scan(&cid); err != nil {
		t.Fatalf("conferencia legada: %v", err)
	}
	if _, err := st.db.Exec(`INSERT INTO conferencia_funcoes (conferencia_id, funcao_id) VALUES (?, ?)`, cid, fLegada); err != nil {
		t.Fatalf("conferencia_funcoes legada: %v", err)
	}

	// v42 NÃO pode ter apagado a linha referenciada
	var existe int
	_ = st.db.QueryRow(`SELECT COUNT(*) FROM funcoes WHERE id = ? AND grupo_id IS NULL`, fLegada).Scan(&existe)
	if existe != 1 {
		t.Fatalf("v42 apagou funcao referenciada por conferência existente (quebrou compat)")
	}

	// abre a conferência legada sem erro (GET ?id=N)
	rr, res := doJSONReq(app, "GET", "/api/conferencia/hoje?id="+itoa(cid), nil, ckGer)
	if rr.Code != http.StatusOK {
		t.Fatalf("conferência legada não abre: %d %v", rr.Code, res)
	}
	if res["modo"] != "antiguidade" {
		t.Fatalf("esperava modo=antiguidade na legada, obtive %v", res["modo"])
	}
	filtro, _ := res["funcoes_filtro"].([]any)
	if len(filtro) != 1 || filtro[0] != "Cabo (legado)" {
		t.Fatalf("badge devia mostrar a função legada: %v", res["funcoes_filtro"])
	}

	// marca presença numa pessoa DO GRUPO cuja função é a legada → confere sem erro
	var pid int64
	if err := st.db.QueryRow(`INSERT INTO pessoas (nome_guerra, nome_completo, grupo_id, funcao_id, status) VALUES ('Neves', 'Neves Alves', ?, ?, 'ativo') RETURNING id`, grupoID, fLegada).Scan(&pid); err != nil {
		t.Fatalf("pessoa legada: %v", err)
	}
	rr, _ = doJSONReq(app, "POST", "/api/conferencia/marcar?id=", map[string]any{
		"pessoa_id":  pid,
		"situacao":   "presente",
		"verificado": true,
	}, ckGer)
	if rr.Code != http.StatusOK {
		t.Fatalf("marcar na conferência legada falhou: %d", rr.Code)
	}

	// ...mas o PICKER não oferece mais a seed global p/ novas conferências
	rr, resP := doJSONReq(app, "GET", "/api/conferencia/funcoes-antiguidade", nil, ckGer)
	if rr.Code != http.StatusOK {
		t.Fatalf("picker falhou: %d", rr.Code)
	}
	flP, _ := resP["funcoes"].([]any)
	for _, it := range flP {
		m, _ := it.(map[string]any)
		if m["nome"].(string) == "Cabo (legado)" || int64(m["id"].(float64)) == fLegada {
			t.Fatalf("seed global (mesmo preservada p/ compat) não pode aparecer no picker: %v", m)
		}
	}
}
