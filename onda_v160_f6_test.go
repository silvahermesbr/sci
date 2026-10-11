package main

// onda_v160_f6_test.go — v1.6.0 Fase 6 (Furo 1): as LEITURAS do módulo de
// conferência ganham porta de PAPEL (confLeituraAuth). Até aqui GET hoje/
// estado/lista · /api/conferencias · {id} · funcoes-antiguidade · stream
// rodavam com a.auth(false) pelada: no CONTEXTO enc_material o usuário lia o
// efetivo do grupo inteiro sem ter poder nenhum no módulo (a ESCRITA já o
// barrava via authConfCom — a leitura era o furo).
//
// Persona (padrão da casa: setupTestApp + loginAs + doJSONReq; positivo E
// negativo):
//   (a) enc_material PURO → 403 "papel sem acesso ao módulo de conferência"
//       nas 7 leituras; GET /api/material/itens segue 200 (não-regressão do
//       módulo DELE).
//   (b) operador COM cadeira enc_material: no CONTEXTO enc → 403 nas leituras;
//       troca para o CONTEXTO operador → hoje 200 (a segregação por contexto
//       da Fase 2 dá o recorte de graça).
//   (c) operador (com setor) no próprio contexto → hoje 200 com o efetivo só
//       do PRÓPRIO setor (prova de ausência do setor alheio na resposta).
//   (d) gerente → 200 com o grupo inteiro; admin → 200 leitura vazia
//       (conferencia nil); conta sem grupo → 403 (guarda central r1 segue).

import (
	"net/http"
	"strings"
	"testing"
)

// f6LeiturasConferencia: as 7 rotas de leitura do módulo (rota oficial
// singular — /api/conferencias/hoje, plural, é SPA/404 e não é API).
func f6LeiturasConferencia() []struct{ metodo, path string } {
	return []struct{ metodo, path string }{
		{"GET", "/api/conferencia/hoje"},
		{"GET", "/api/conferencia/estado"},
		{"GET", "/api/conferencia/lista"},
		{"GET", "/api/conferencia/1"},
		{"GET", "/api/conferencia/1/stream"},
		{"GET", "/api/conferencia/funcoes-antiguidade"},
		{"GET", "/api/conferencias"},
	}
}

// f6SetupGrupo: grupo + setores A/B + pessoas + gerente. Retorna ids úteis.
func f6SetupGrupo(t *testing.T, app *App, st *Store, sufixo string) (gid, sA, sB, pA, pB, idGer int64, ckGer *http.Cookie) {
	t.Helper()
	if err := st.db.QueryRow(`INSERT INTO grupos (nome) VALUES (?) RETURNING id`, "Grp F6 "+sufixo).Scan(&gid); err != nil {
		t.Fatalf("criar grupo: %v", err)
	}
	if err := st.db.QueryRow(`INSERT INTO setores (grupo_id, nome) VALUES (?, 'F6 Setor A') RETURNING id`, gid).Scan(&sA); err != nil {
		t.Fatalf("criar setor A: %v", err)
	}
	if err := st.db.QueryRow(`INSERT INTO setores (grupo_id, nome) VALUES (?, 'F6 Setor B') RETURNING id`, gid).Scan(&sB); err != nil {
		t.Fatalf("criar setor B: %v", err)
	}
	if err := st.db.QueryRow(`INSERT INTO pessoas (nome_guerra, nome_completo, grupo_id, setor_id, status) VALUES ('F6 AA','Pessoa A F6', ?, ?, 'ativo') RETURNING id`, gid, sA).Scan(&pA); err != nil {
		t.Fatalf("criar pessoa A: %v", err)
	}
	if err := st.db.QueryRow(`INSERT INTO pessoas (nome_guerra, nome_completo, grupo_id, setor_id, status) VALUES ('F6 BB','Pessoa B F6', ?, ?, 'ativo') RETURNING id`, gid, sB).Scan(&pB); err != nil {
		t.Fatalf("criar pessoa B: %v", err)
	}
	criaUsuarioTeste(t, st, "f6_ger_"+sufixo, "senha-ger", "gerente")
	if _, err := st.db.Exec(`UPDATE usuarios SET grupo_id = ? WHERE login = 'f6_ger_`+sufixo+`'`, gid); err != nil {
		t.Fatalf("vincular gerente: %v", err)
	}
	if err := st.db.QueryRow(`SELECT id FROM usuarios WHERE login = 'f6_ger_`+sufixo+`'`).Scan(&idGer); err != nil {
		t.Fatalf("id gerente: %v", err)
	}
	ckGer = loginAs(t, app, "f6_ger_"+sufixo, "senha-ger")
	return
}

// (a) enc_material puro: NEM LEITURA — 403 nas 7; material segue dele (200).
func TestF6EncMaterialPuroSemLeituraDeConferencia(t *testing.T) {
	app, st, cleanup := setupTestApp(t)
	defer cleanup()
	gid, _, fMat, _, _ := f2SetupGrupo(t, app, st, "f6pm")

	criaUsuarioTeste(t, st, "f6_puro_pm", "senha-puro", "")
	if _, err := st.db.Exec(`UPDATE usuarios SET grupo_id = ? WHERE login = 'f6_puro_pm'`, gid); err != nil {
		t.Fatalf("vincular puro: %v", err)
	}
	var idPuro int64
	if err := st.db.QueryRow(`SELECT id FROM usuarios WHERE login = 'f6_puro_pm'`).Scan(&idPuro); err != nil {
		t.Fatalf("id puro: %v", err)
	}
	if _, err := st.db.Exec(`INSERT INTO funcao_membros (funcao_id, grupo_id, usuario_id, titularidade) VALUES (?,?,?,'titular')`, fMat, gid, idPuro); err != nil {
		t.Fatalf("designar puro: %v", err)
	}
	v45Reexecuta(t, st)

	ck := loginAs(t, app, "f6_puro_pm", "senha-puro")
	if p := f2MePapel(t, app, ck); p != "enc_material" {
		t.Fatalf("(a) cenário: designado puro devia logar NO CONTEXTO enc_material, veio %q", p)
	}

	for _, r := range f6LeiturasConferencia() {
		rr, res := doJSONReq(app, r.metodo, r.path, nil, ck)
		if rr.Code != http.StatusForbidden {
			t.Errorf("(a) %s %s no contexto enc_material: esperado 403, veio %d (%v)", r.metodo, r.path, rr.Code, res)
			continue
		}
		if !strings.Contains(rr.Body.String(), "papel sem acesso ao módulo de conferência") {
			t.Errorf("(a) %s %s: mensagem da guarda ausente: %s", r.metodo, r.path, rr.Body.String())
		}
	}

	// não-regressão do módulo DELE: material segue inteiro no contexto enc_material
	if rr, res := doJSONReq(app, "GET", "/api/material/itens", nil, ck); rr.Code != http.StatusOK {
		t.Fatalf("(a) não-regressão: GET /api/material/itens deve 200 no enc_material, veio %d (%v)", rr.Code, res)
	}
}

// (b) operador com cadeira enc_material: o CONTEXTO decide — no contexto enc
// nem leitura; no contexto operador a leitura abre (cortada ao setor, (c)).
func TestF6LeituraSegueOContextoAtivo(t *testing.T) {
	app, st, cleanup := setupTestApp(t)
	defer cleanup()
	gid, _, fMat, _, ckGer := f2SetupGrupo(t, app, st, "f6cx")

	criaUsuarioTeste(t, st, "f6_op_cx", "senha-op", "operador")
	if _, err := st.db.Exec(`UPDATE usuarios SET grupo_id = ? WHERE login = 'f6_op_cx'`, gid); err != nil {
		t.Fatalf("vincular operador: %v", err)
	}
	var idOp int64
	if err := st.db.QueryRow(`SELECT id FROM usuarios WHERE login = 'f6_op_cx'`).Scan(&idOp); err != nil {
		t.Fatalf("id operador: %v", err)
	}
	// LOGA PRIMEIRO (padrão F2): a linha de sistema nasce antes (id menor) e o
	// login NUNCA aterrissa no contexto enc (CriarSessaoComPapel ORDER BY id).
	ckOp := loginAs(t, app, "f6_op_cx", "senha-op")
	if p := f2MePapel(t, app, ckOp); p != "operador" {
		t.Fatalf("(b) cenário: operador devia logar NO CONTEXTO operador, veio %q", p)
	}
	if _, err := st.db.Exec(`INSERT INTO funcao_membros (funcao_id, grupo_id, usuario_id, titularidade) VALUES (?,?,?,'titular')`, fMat, gid, idOp); err != nil {
		t.Fatalf("designar operador na cadeira: %v", err)
	}
	v45Reexecuta(t, st)

	// troca para o contexto enc_material → leitura do módulo FECHA
	f2TrocaContexto(t, app, ckOp, f2LinhaPapel(t, st, idOp, "enc_material"))
	if rr, _ := doJSONReq(app, "GET", "/api/conferencia/hoje", nil, ckOp); rr.Code != http.StatusForbidden {
		t.Fatalf("(b) contexto enc_material: GET hoje deve 403, veio %d", rr.Code)
	}

	// volta ao contexto operador → leitura ABRE (guarda de papel passa)
	f2TrocaContexto(t, app, ckOp, f2LinhaPapel(t, st, idOp, "operador"))
	if rr, res := doJSONReq(app, "GET", "/api/conferencia/hoje", nil, ckOp); rr.Code != http.StatusOK {
		t.Fatalf("(b) contexto operador: GET hoje deve 200, veio %d (%v)", rr.Code, res)
	}
	_ = ckGer
}

// (c)+(d) matriz de escopo na leitura liberada: operador (com setor) → efetivo
// só do próprio setor; gerente → grupo; admin → leitura vazia; sem grupo → 403.
func TestF6LeituraEscopoPorPersona(t *testing.T) {
	app, st, cleanup := setupTestApp(t)
	defer cleanup()
	gid, sA, _, pA, pB, _, ckGer := f6SetupGrupo(t, app, st, "esc")

	// operador COM setor (o cenário da Fase 7: sem setor é 403 do recorte)
	criaUsuarioTeste(t, st, "f6_op_esc", "senha-op", "operador")
	if _, err := st.db.Exec(`UPDATE usuarios SET grupo_id = ?, setor_id = ? WHERE login = 'f6_op_esc'`, gid, sA); err != nil {
		t.Fatalf("vincular operador: %v", err)
	}
	ckOp := loginAs(t, app, "f6_op_esc", "senha-op")

	// conferência aberta do grupo (poder de gerente)
	if rr, res := doJSONReq(app, "POST", "/api/conferencia/iniciar", map[string]any{"nome": "Conf F6"}, ckGer); rr.Code != http.StatusOK {
		t.Fatalf("cenário: gerente inicia conferência (200), veio %d (%v)", rr.Code, res)
	}

	// (c) operador: hoje 200 e SÓ o efetivo do próprio setor
	rrOp, resOp := doJSONReq(app, "GET", "/api/conferencia/hoje", nil, ckOp)
	if rrOp.Code != http.StatusOK {
		t.Fatalf("(c) operador hoje: esperado 200, veio %d (%v)", rrOp.Code, resOp)
	}
	pessoasOp, _ := resOp["pessoas"].([]any)
	if len(pessoasOp) != 1 {
		t.Fatalf("(c) operador devia ver 1 pessoa (próprio setor), veio %d: %v", len(pessoasOp), resOp["pessoas"])
	}
	if pm, _ := pessoasOp[0].(map[string]any); pm == nil || pm["id"].(float64) != float64(pA) {
		t.Fatalf("(c) pessoa visível ao operador devia ser a do setor A (%d), veio %v", pA, pessoasOp[0])
	}

	// (d) gerente: hoje 200 com o grupo inteiro (as duas pessoas)
	rrGer, resGer := doJSONReq(app, "GET", "/api/conferencia/hoje", nil, ckGer)
	if rrGer.Code != http.StatusOK {
		t.Fatalf("(d) gerente hoje: esperado 200, veio %d (%v)", rrGer.Code, resGer)
	}
	if n := len(resGer["pessoas"].([]any)); n != 2 {
		t.Fatalf("(d) gerente devia ver as 2 pessoas do grupo, veio %d", n)
	}

	// (d) admin: leitura vazia (conferencia nil, setores vazios) — doutrina §4
	ckAdm := loginAs(t, app, "admin", "admin123")
	rrAdm, resAdm := doJSONReq(app, "GET", "/api/conferencia/hoje", nil, ckAdm)
	if rrAdm.Code != http.StatusOK {
		t.Fatalf("(d) admin hoje: esperado 200 (leitura vazia), veio %d (%v)", rrAdm.Code, resAdm)
	}
	if resAdm["conferencia"] != nil {
		t.Fatalf("(d) admin: conferencia devia vir nil, veio %v", resAdm["conferencia"])
	}
	if n := len(resAdm["setores_status"].([]any)); n != 0 {
		t.Fatalf("(d) admin: setores_status devia vir vazio, veio %d", n)
	}

	// (d) conta sem grupo: a guarda central r1 segue devolvendo 403 no handler
	criaUsuarioTeste(t, st, "f6_semgrupo", "senha-sg", "operador")
	ckSg := loginAs(t, app, "f6_semgrupo", "senha-sg")
	if rr, _ := doJSONReq(app, "GET", "/api/conferencia/hoje", nil, ckSg); rr.Code != http.StatusForbidden {
		t.Fatalf("(d) sem grupo: GET hoje deve 403, veio %d", rr.Code)
	}

	// sanidade: pB (setor B) existe e NÃO apareceu para o operador
	if pB <= 0 {
		t.Fatalf("(c) fixture: pessoa B inválida")
	}
}
