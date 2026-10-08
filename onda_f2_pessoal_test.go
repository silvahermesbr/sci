package main

// onda_f2_pessoal_test.go — F1 do módulo PESSOAL (branch f2/modulo-pessoal):
//   1. DELETE /api/pessoas/{id} passa a exigir gestão de pessoal — um OPERADOR
//      do próprio grupo recebe 403 (antes: só auth + checagem de grupo, buraco).
//   2. DELETE /api/setores/{id}: enc/aux de pessoal (R3: DESIGNAÇÃO em
//      funcao_membros) excluem setor do PRÓPRIO grupo → 200; setor de grupo
//      alheio → 403; operador → 403. Gerente mantém subordinados.
// Padrão cookiejar por persona (mesmo da suíte).

import (
	"fmt"
	"net/http"
	"testing"
)

func f2PessoalSetup(t *testing.T, app *App, st *Store) (gid, gidFora, setorID, setorForaID, pAlvo, fEnc, fAux int64) {
	t.Helper()
	if err := st.db.QueryRow(`INSERT INTO grupos (nome) VALUES ('Grp F2 Pessoal') RETURNING id`).Scan(&gid); err != nil {
		t.Fatalf("criar grupo: %v", err)
	}
	if err := st.db.QueryRow(`INSERT INTO grupos (nome) VALUES ('Grp F2 Fora') RETURNING id`).Scan(&gidFora); err != nil {
		t.Fatalf("criar grupo fora: %v", err)
	}
	if err := st.db.QueryRow(`INSERT INTO setores (nome, grupo_id) VALUES ('Seção F2', ?) RETURNING id`, gid).Scan(&setorID); err != nil {
		t.Fatalf("criar setor: %v", err)
	}
	if err := st.db.QueryRow(`INSERT INTO setores (nome, grupo_id) VALUES ('Seção F2 Fora', ?) RETURNING id`, gidFora).Scan(&setorForaID); err != nil {
		t.Fatalf("criar setor fora: %v", err)
	}
	if err := st.db.QueryRow(`INSERT INTO funcoes (nome, grupo_id) VALUES ('Encarregado de Pessoal F2', ?) RETURNING id`, gid).Scan(&fEnc); err != nil {
		t.Fatalf("criar função encarregado: %v", err)
	}
	if err := st.db.QueryRow(`INSERT INTO funcoes (nome, grupo_id) VALUES ('Auxiliar de Pessoal F2', ?) RETURNING id`, gid).Scan(&fAux); err != nil {
		t.Fatalf("criar função auxiliar: %v", err)
	}

	criaUsuarioTeste(t, st, "ger_f2", "senha-gerente", "gerente")
	criaUsuarioTeste(t, st, "enc_f2", "senha-enc", "")
	criaUsuarioTeste(t, st, "aux_f2", "senha-aux", "")
	criaUsuarioTeste(t, st, "op_f2", "senha-op", "operador")
	criaUsuarioTeste(t, st, "gerfora_f2", "senha-gerf", "gerente")

	vincula := `UPDATE usuarios SET grupo_id = ? WHERE login = ?`
	for _, lg := range []string{"ger_f2", "enc_f2", "aux_f2", "op_f2"} {
		if _, err := st.db.Exec(vincula, gid, lg); err != nil {
			t.Fatalf("vincular %s: %v", lg, err)
		}
	}
	if _, err := st.db.Exec(vincula, gidFora, "gerfora_f2"); err != nil {
		t.Fatalf("vincular gerfora: %v", err)
	}

	// R3: poder de enc/aux vem da DESIGNAÇÃO (funcao_membros), não do funcao_id.
	designa := func(login string, fid int64, tit string) {
		var uid int64
		if err := st.db.QueryRow(`SELECT id FROM usuarios WHERE login = ?`, login).Scan(&uid); err != nil {
			t.Fatalf("id %s: %v", login, err)
		}
		if _, err := st.db.Exec(`INSERT INTO funcao_membros (funcao_id, grupo_id, usuario_id, titularidade) VALUES (?,?,?,?)`, fid, gid, uid, tit); err != nil {
			t.Fatalf("designar %s: %v", login, err)
		}
	}
	designa("enc_f2", fEnc, "titular")
	designa("aux_f2", fAux, "auxiliar")

	if err := st.db.QueryRow(`INSERT INTO pessoas (nome_guerra, nome_completo, grupo_id, status) VALUES ('ALVOF2', 'Alvo F2', ?, 'ativo') RETURNING id`, gid).Scan(&pAlvo); err != nil {
		t.Fatalf("criar pessoa alvo: %v", err)
	}
	return
}

// TestF2OperadorNaoExcluiPessoa: operador do próprio grupo → 403 no DELETE de
// pessoa (buraco fechado pelo guardaGestaoPessoal).
func TestF2OperadorNaoExcluiPessoa(t *testing.T) {
	app, st, cleanup := setupTestApp(t)
	defer cleanup()
	gid, _, _, _, pAlvo, _, _ := f2PessoalSetup(t, app, st)
	_ = gid

	ckOp := loginAs(t, app, "op_f2", "senha-op")
	if rr, res := doJSONReq(app, "DELETE", fmt.Sprintf("/api/pessoas/%d", pAlvo), nil, ckOp); rr.Code != http.StatusForbidden {
		t.Fatalf("operador excluindo pessoa deve 403, veio %d (%v)", rr.Code, res["erro"])
	}

	// leitura continua aberta a qualquer papel autenticado
	if rr, _ := doJSONReq(app, "GET", "/api/pessoas", nil, ckOp); rr.Code != http.StatusOK {
		t.Fatalf("operador lê pessoas deve 200, veio %d", rr.Code)
	}

	// gerente do grupo exclui → 200 (regressão do guarda)
	ckGer := loginAs(t, app, "ger_f2", "senha-gerente")
	if rr, res := doJSONReq(app, "DELETE", fmt.Sprintf("/api/pessoas/%d", pAlvo), nil, ckGer); rr.Code != http.StatusOK {
		t.Fatalf("gerente excluindo pessoa deve 200, veio %d (%v)", rr.Code, res["erro"])
	}
}

// TestF2EncAuxExcluemSetorProprio: designados excluem setor do próprio grupo;
// setor de grupo alheio → 403; operador → 403. Gerente segue com subordinados.
func TestF2EncAuxExcluemSetorProprio(t *testing.T) {
	app, st, cleanup := setupTestApp(t)
	defer cleanup()
	gid, gidFora, setorID, setorForaID, _, _, _ := f2PessoalSetup(t, app, st)

	// setor extra no próprio grupo p/ o caso do auxiliar
	var setorAux int64
	if err := st.db.QueryRow(`INSERT INTO setores (nome, grupo_id) VALUES ('Seção F2 Aux', ?) RETURNING id`, gid).Scan(&setorAux); err != nil {
		t.Fatalf("criar setor aux: %v", err)
	}

	// gerente de outro grupo é coberto por TestExcluirSetorGerenteAlheio403
	ckEnc := loginAs(t, app, "enc_f2", "senha-enc")
	ckAux := loginAs(t, app, "aux_f2", "senha-aux")
	ckOp := loginAs(t, app, "op_f2", "senha-op")

	// enc exclui setor do próprio grupo → 200
	if rr, res := doJSONReq(app, "DELETE", fmt.Sprintf("/api/setores/%d", setorID), nil, ckEnc); rr.Code != http.StatusOK {
		t.Fatalf("enc exclui setor próprio deve 200, veio %d (%v)", rr.Code, res["erro"])
	}
	// aux exclui setor do próprio grupo → 200
	if rr, res := doJSONReq(app, "DELETE", fmt.Sprintf("/api/setores/%d", setorAux), nil, ckAux); rr.Code != http.StatusOK {
		t.Fatalf("aux exclui setor próprio deve 200, veio %d (%v)", rr.Code, res["erro"])
	}
	// enc NÃO exclui setor de grupo alheio → 403
	if rr, res := doJSONReq(app, "DELETE", fmt.Sprintf("/api/setores/%d", setorForaID), nil, ckEnc); rr.Code != http.StatusForbidden {
		t.Fatalf("enc exclui setor de outro grupo deve 403, veio %d (%v)", rr.Code, res["erro"])
	}
	// operador → 403
	if rr, _ := doJSONReq(app, "DELETE", fmt.Sprintf("/api/setores/%d", setorForaID), nil, ckOp); rr.Code != http.StatusForbidden {
		t.Fatalf("operador excluindo setor deve 403, veio %d", rr.Code)
	}

	// ramo da hierarquia: gidFora vira SUBORDINADO de gid (vínculo bilateral) —
	// gerente passa a excluir setor do subordinado; enc/aux NÃO (restrito ao próprio).
	if _, err := st.db.Exec(`INSERT INTO grupo_vinculos (superior_id, subordinado_id, criado_por_superior, criado_por_subordinado)
		VALUES (?, ?, 1, 1)`, gid, gidFora); err != nil {
		t.Fatalf("vincular subordinado: %v", err)
	}
	var setorSub int64
	if err := st.db.QueryRow(`INSERT INTO setores (nome, grupo_id) VALUES ('Seção F2 Sub', ?) RETURNING id`, gidFora).Scan(&setorSub); err != nil {
		t.Fatalf("criar setor sub: %v", err)
	}
	// gerente loga DEPOIS do vínculo: a sessão congela o papel com grupo (doutrina
	// vinculaGrupoDoLogin — sessão aponta p/ usuario_papeis.grupo_id).
	// doutrina vinculaGrupoDoLogin: a linha do gerente em usuario_papeis JÁ EXISTE
	// (criada no 1º login do teste de pessoas) — o índice parcial único
	// (1 gerente por grupo, store.go:1435) proíbe reinseri-la; a linha ganha o
	// grupo ANTES do login e a NOVA sessão já nasce apontando p/ ela.
	var uidGer int64
	if err := st.db.QueryRow(`SELECT id FROM usuarios WHERE login='ger_f2'`).Scan(&uidGer); err != nil {
		t.Fatalf("id ger_f2: %v", err)
	}
	if _, err := st.db.Exec(`UPDATE usuario_papeis SET grupo_id = ? WHERE usuario_id = ? AND grupo_id IS NULL`, gid, uidGer); err != nil {
		t.Fatalf("vincular papel do gerente: %v", err)
	}
	ckGer := loginAs(t, app, "ger_f2", "senha-gerente")
	if rr, res := doJSONReq(app, "DELETE", fmt.Sprintf("/api/setores/%d", setorSub), nil, ckGer); rr.Code != http.StatusOK {
		t.Fatalf("gerente exclui setor de subordinado deve 200, veio %d (%v)", rr.Code, res["erro"])
	}
	if rr, _ := doJSONReq(app, "DELETE", fmt.Sprintf("/api/setores/%d", setorForaID), nil, ckEnc); rr.Code != http.StatusForbidden {
		t.Fatalf("enc excluindo setor de subordinado deve 403, veio %d", rr.Code)
	}

	// gerente LOCAL exclui setor próprio (ramo clássico intacto)
	var setorGer int64
	if err := st.db.QueryRow(`INSERT INTO setores (nome, grupo_id) VALUES ('Seção F2 Ger', ?) RETURNING id`, gid).Scan(&setorGer); err != nil {
		t.Fatalf("criar setor ger: %v", err)
	}
	if rr, res := doJSONReq(app, "DELETE", fmt.Sprintf("/api/setores/%d", setorGer), nil, ckGer); rr.Code != http.StatusOK {
		t.Fatalf("gerente exclui setor próprio deve 200, veio %d (%v)", rr.Code, res["erro"])
	}
	_ = gidFora
}
