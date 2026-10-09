package main

// Onda 09/10 — testes da conferência POR ANTIGUIDADE + pré-fechamento de setor.
// Padrão do harness: setupTestApp + doJSONReq/loginAs (v1_test.go).

import (
	"net/http"
	"testing"
)

// semeiaCenarioAntiguidade: grupo, setor, pessoas de graduações distintas,
// gerente, chefe do setor e operador. Retorna os ids necessários.
func semeiaCenarioAntiguidade(t *testing.T, app *App, st *Store) (grupoID, setorID, p3SgtID, pSdID, gerUID, chefeUID int64) {
	t.Helper()

	var funcaoSgt, funcaoSd int64
	if err := st.db.QueryRow(`SELECT id FROM funcoes WHERE nome = '3º Sargento' AND tipo = 'antiguidade'`).Scan(&funcaoSgt); err != nil {
		t.Fatalf("migração v40 não semeou 3º Sargento: %v", err)
	}
	if err := st.db.QueryRow(`SELECT id FROM funcoes WHERE nome = 'Soldado EV' AND tipo = 'antiguidade'`).Scan(&funcaoSd); err != nil {
		t.Fatalf("migração v40 não semeou Soldado EV: %v", err)
	}

	res, err := st.db.Exec(`INSERT INTO grupos (nome, codigo) VALUES ('Cia Conf Antiguidade', 'CCA1')`)
	if err != nil {
		t.Fatalf("grupo: %v", err)
	}
	grupoID, _ = res.LastInsertId()

	res, err = st.db.Exec(`INSERT INTO setores (nome, grupo_id) VALUES ('Comunicações', ?)`, grupoID)
	if err != nil {
		t.Fatalf("setor: %v", err)
	}
	setorID, _ = res.LastInsertId()

	// pessoa com funcao_id DIRETO na pessoa (3º Sargento)
	res, err = st.db.Exec(`INSERT INTO pessoas (nome_guerra, nome_completo, grupo_id, setor_id, funcao_id, status) VALUES ('Tavares', 'Tavares da Silva', ?, ?, ?, 'ativo')`, grupoID, setorID, funcaoSgt)
	if err != nil {
		t.Fatalf("pessoa 3Sgt: %v", err)
	}
	p3SgtID, _ = res.LastInsertId()

	// pessoa de outra graduação (Soldado EV) — NÃO deve entrar no filtro
	res, err = st.db.Exec(`INSERT INTO pessoas (nome_guerra, nome_completo, grupo_id, setor_id, funcao_id, status) VALUES ('Rocha', 'Rocha Lima', ?, ?, ?, 'ativo')`, grupoID, setorID, funcaoSd)
	if err != nil {
		t.Fatalf("pessoa Sd: %v", err)
	}
	pSdID, _ = res.LastInsertId()

	hashPadrao, _ := hashSenha("senha12345")
	res, err = st.db.Exec(`INSERT INTO usuarios (login, senha_hash, papel, grupo_id, precisa_setup) VALUES ('ger_conf_a', ?, 'gerente', ?, 0)`, hashPadrao, grupoID)
	if err != nil {
		t.Fatalf("usuario gerente: %v", err)
	}
	gerUID, _ = res.LastInsertId()
	_, _ = st.db.Exec(`INSERT INTO usuario_papeis (usuario_id, grupo_id, papel) VALUES (?, ?, 'gerente')`, gerUID, grupoID)

	res, err = st.db.Exec(`INSERT INTO usuarios (login, senha_hash, papel, grupo_id, pessoa_id, setor_id, precisa_setup) VALUES ('chefe_conf_a', ?, 'chefe_setor', ?, NULL, ?, 0)`, hashPadrao, grupoID, setorID)
	if err != nil {
		t.Fatalf("usuario chefe: %v", err)
	}
	chefeUID, _ = res.LastInsertId()
	_, _ = st.db.Exec(`INSERT INTO usuario_papeis (usuario_id, grupo_id, papel) VALUES (?, ?, 'chefe_setor')`, chefeUID, grupoID)

	return
}

func TestV40MigraSemeiaFuncoesAntiguidade(t *testing.T) {
	_, st, cleanup := setupTestApp(t)
	defer cleanup()

	var n int
	if err := st.db.QueryRow(`SELECT COUNT(*) FROM funcoes WHERE tipo = 'antiguidade'`).Scan(&n); err != nil {
		t.Fatalf("count funcoes: %v", err)
	}
	if n < 14 {
		t.Fatalf("esperava >= 14 funções de antiguidade semeadas, obtive %d", n)
	}
	var paiID int64
	var paiNome string
	if err := st.db.QueryRow(`SELECT f.pai_id, COALESCE(p.nome,'') FROM funcoes f LEFT JOIN funcoes p ON p.id = f.pai_id WHERE f.nome = '3º Sargento' AND f.tipo = 'antiguidade'`).Scan(&paiID, &paiNome); err != nil {
		t.Fatalf("3º Sargento não semeada: %v", err)
	}
	if paiID == 0 || paiNome != "Praças" {
		t.Fatalf("3º Sargento deveria ser filha do nó 'Praças', pai=%d (%s)", paiID, paiNome)
	}
	// idempotência: rodar de novo não duplica
	if err := st.migrarV40(); err != nil {
		t.Fatalf("migrarV40 idempotente: %v", err)
	}
	var n2 int
	_ = st.db.QueryRow(`SELECT COUNT(*) FROM funcoes WHERE nome = '3º Sargento' AND tipo = 'antiguidade'`).Scan(&n2)
	if n2 != 1 {
		t.Fatalf("seed não é idempotente: %d ocorrências de 3º Sargento", n2)
	}
}

func TestConferenciaPorAntiguidadeFiltro(t *testing.T) {
	app, st, cleanup := setupTestApp(t)
	defer cleanup()

	grupoID, _, p3SgtID, pSdID, _, _ := semeiaCenarioAntiguidade(t, app, st)
	ckGer := loginAs(t, app, "ger_conf_a", "senha12345")

	var fSgt int64
	if err := st.db.QueryRow(`SELECT id FROM funcoes WHERE nome = '3º Sargento' AND tipo = 'antiguidade'`).Scan(&fSgt); err != nil {
		t.Fatalf("3º Sargento: %v", err)
	}

	// T2a: iniciar com funcao_ids (sem setores) → 200 + conferência criada
	rr, res := doJSONReq(app, "POST", "/api/conferencia/iniciar", map[string]any{
		"nome":       "Conf 3Sgt",
		"funcao_ids": []int64{fSgt},
	}, ckGer)
	if rr.Code != http.StatusOK {
		t.Fatalf("iniciar com funcao_ids falhou: %d %v", rr.Code, res)
	}
	cid, _ := res["conferencia_id"].(float64)
	if cid == 0 {
		t.Fatalf("conferencia_id ausente: %v", res)
	}

	// T2b: hoje traz modo=antiguidade, funcoes_filtro com 3º Sargento e só a pessoa do filtro
	rr, res = doJSONReq(app, "GET", "/api/conferencia/hoje", nil, ckGer)
	if rr.Code != http.StatusOK {
		t.Fatalf("hoje falhou: %d", rr.Code)
	}
	if res["modo"] != "antiguidade" {
		t.Fatalf("esperava modo=antiguidade, obtive %v", res["modo"])
	}
	filtro, _ := res["funcoes_filtro"].([]any)
	if len(filtro) != 1 || filtro[0] != "3º Sargento" {
		t.Fatalf("funcoes_filtro inesperado: %v", res["funcoes_filtro"])
	}
	pessoas, _ := res["pessoas"].([]any)
	if len(pessoas) != 1 {
		t.Fatalf("esperava 1 pessoa no filtro (3º Sgt), obtive %d", len(pessoas))
	}
	p0, _ := pessoas[0].(map[string]any)
	if p0["id"].(float64) != float64(p3SgtID) {
		t.Fatalf("pessoa errada no filtro: %v", p0)
	}

	// T2c: pessoa fora do filtro não é afetada — segue ativa no banco (só sai da conferência)
	var status string
	_ = st.db.QueryRow(`SELECT status FROM pessoas WHERE id = ?`, pSdID).Scan(&status)
	if status != "ativo" {
		t.Fatalf("pessoa fora do filtro não deveria mudar de status")
	}
	_ = grupoID
}

func TestConferenciaSemFiltroRegressao(t *testing.T) {
	app, st, cleanup := setupTestApp(t)
	defer cleanup()

	_, _, p3SgtID, pSdID, _, _ := semeiaCenarioAntiguidade(t, app, st)
	ckGer := loginAs(t, app, "ger_conf_a", "senha12345")

	// conferência SEM funcao_ids → modo=setores, pessoas completas (via iniciar)
	rr, res := doJSONReq(app, "POST", "/api/conferencia/iniciar", map[string]any{
		"nome": "Conf normal",
	}, ckGer)
	if rr.Code != http.StatusOK {
		t.Fatalf("despachar simples falhou: %d %v", rr.Code, res)
	}
	rr, res = doJSONReq(app, "GET", "/api/conferencia/hoje", nil, ckGer)
	if rr.Code != http.StatusOK {
		t.Fatalf("hoje falhou: %d", rr.Code)
	}
	if res["modo"] != "setores" {
		t.Fatalf("esperava modo=setores (regressão), obtive %v", res["modo"])
	}
	pessoas, _ := res["pessoas"].([]any)
	if len(pessoas) != 2 {
		t.Fatalf("sem filtro esperava 2 pessoas, obtive %d", len(pessoas))
	}
	_ = p3SgtID
	_ = pSdID
}

func TestPreFechamentoSetor(t *testing.T) {
	app, st, cleanup := setupTestApp(t)
	defer cleanup()

	_, setorID, p3SgtID, pSdID, _, chefeUID := semeiaCenarioAntiguidade(t, app, st)
	ckGer := loginAs(t, app, "ger_conf_a", "senha12345")

	var fSgt int64
	_ = st.db.QueryRow(`SELECT id FROM funcoes WHERE nome = '3º Sargento' AND tipo = 'antiguidade'`).Scan(&fSgt)

	// despacha conferência com filtro + setor
	rr, res := doJSONReq(app, "POST", "/api/conferencia/despachar", map[string]any{
		"nome":       "Conf PreFech",
		"setores":    []int64{setorID},
		"funcao_ids": []int64{fSgt},
	}, ckGer)
	if rr.Code != http.StatusOK {
		t.Fatalf("despachar falhou: %d %v", rr.Code, res)
	}
	cid := int64(res["conferencia_id"].(float64))

	// gerente marca a pessoa do filtro como presente+verificada
	rr, res = doJSONReq(app, "POST", "/api/conferencia/marcar?id=", map[string]any{
		"pessoa_id":  p3SgtID,
		"situacao":   "presente",
		"verificado": true,
	}, ckGer)
	if rr.Code != http.StatusOK {
		t.Fatalf("marcar falhou: %d %v", rr.Code, res)
	}

	// T4: chefe do setor consulta o pré-fechamento — itens só do filtro
	ckChefe := loginAs(t, app, "chefe_conf_a", "senha12345")
	_ = chefeUID
	rr, res = doJSONReq(app, "GET", "/api/conferencia/"+itoa(cid)+"/setor/"+itoa(setorID)+"/pre_fechamento", nil, ckChefe)
	if rr.Code != http.StatusOK {
		t.Fatalf("pre_fechamento falhou: %d %v", rr.Code, res)
	}
	itens, _ := res["itens"].([]any)
	if len(itens) != 1 {
		t.Fatalf("esperava 1 item no pré-fechamento (filtro 3º Sgt), obtive %d", len(itens))
	}
	it0, _ := itens[0].(map[string]any)
	if it0["pessoa_id"].(float64) != float64(p3SgtID) {
		t.Fatalf("item errado: %v", it0)
	}
	if it0["situacao"] != "presente" || it0["verificado"] != true {
		t.Fatalf("estado da pessoa não refletido: %v", it0)
	}
	if res["total"].(float64) != 1 || res["verificados"].(float64) != 1 {
		t.Fatalf("contagens erradas: total=%v verificados=%v", res["total"], res["verificados"])
	}
	// pessoa fora do filtro (Sd EV) não aparece
	for _, it := range itens {
		m, _ := it.(map[string]any)
		if m["pessoa_id"].(float64) == float64(pSdID) {
			t.Fatalf("pessoa fora do filtro vazou no pré-fechamento")
		}
	}
}

func TestDespacharFuncaoInvalida400(t *testing.T) {
	app, st, cleanup := setupTestApp(t)
	defer cleanup()

	_, _, _, _, _, _ = semeiaCenarioAntiguidade(t, app, st)
	ckGer := loginAs(t, app, "ger_conf_a", "senha12345")

	// T5: funcao_id inexistente → 400 e nenhuma conferência criada
	rr, _ := doJSONReq(app, "POST", "/api/conferencia/iniciar", map[string]any{
		"nome":       "Conf inválida",
		"funcao_ids": []int64{999999},
	}, ckGer)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("esperava 400 p/ funcao inexistente, obtive %d", rr.Code)
	}
	var n int
	_ = app.st.db.QueryRow(`SELECT COUNT(*) FROM conferencias`).Scan(&n)
	if n != 0 {
		t.Fatalf("conferência órfã criada apesar do 400 (%d)", n)
	}

	// funcao_id que é cadeira de grupo (tipo 'grupo') → 400 também
	var gerFun int64
	_ = app.st.db.QueryRow(`SELECT id FROM funcoes WHERE nome = 'Gerente' AND tipo = 'grupo'`).Scan(&gerFun)
	if gerFun > 0 {
		rr, _ = doJSONReq(app, "POST", "/api/conferencia/iniciar", map[string]any{
			"nome":       "Conf cadeira",
			"funcao_ids": []int64{gerFun},
		}, ckGer)
		if rr.Code != http.StatusBadRequest {
			t.Fatalf("esperava 400 p/ cadeira de grupo, obtive %d", rr.Code)
		}
	}
}

func itoa(v int64) string {
	if v == 0 {
		return "0"
	}
	neg := v < 0
	if neg {
		v = -v
	}
	digits := []byte{}
	for v > 0 {
		digits = append([]byte{byte('0' + v%10)}, digits...)
		v /= 10
	}
	if neg {
		return "-" + string(digits)
	}
	return string(digits)
}
