package main

// fix_encarregado_test.go — ordem do dono 09/10: "ao escalar um encarregado de
// pessoal e de material, o escalado deve ter acesso a TODOS os módulos que o
// respectivo encarregado tem. Atualmente eles não têm acesso."
//
// Causa raiz provada: o sync do POST /api/grupo/funcoes/membros fabricava
// linha em usuario_papeis com papel='operador' para conta SEM papel do sistema;
// o login seguinte adotava essa linha como papel ATIVO da sessão e o portão
// anti-escalação (papel do sistema trava) negava tudo ao designado.
//
// Cobertura:
//   T1  Matriz completa de poder por designação (titular/auxiliar, pessoal/
//       material, leitura/escrita, R4 remoção → 403 na mesma sessão).
//   T2  Cadeira de grupo legada SEM chave + designação → migrarV42 crava a
//       chave e o poder passa a conceder (R3 sem SQL manual).
//   T3  Papel de sistema trava (R5): operador designado segue 403; v42 NÃO
//       destrava conta com operador real em usuarios.papel.
//   T4  v42 destrava o estrangulado: linha-fantasia apagada, sessão presa
//       solta (papel_ativo_id NULL), poder concedido na MESMA sessão;
//       idempotente; sync novo não fabrica mais linha.

import (
	"encoding/json"
	"net/http"
	"testing"
)

// encSetupBase: grupo + contas + gerente logado. Retorna ids úteis.
func encSetupBase(t *testing.T, app *App, st *Store, sufixo string) (gid, fPess, fMat, idGer, idComum int64, ckGer *http.Cookie) {
	t.Helper()
	if err := st.db.QueryRow(`INSERT INTO grupos (nome) VALUES (?) RETURNING id`, "Grp Enc "+sufixo).Scan(&gid); err != nil {
		t.Fatalf("criar grupo: %v", err)
	}
	if err := st.db.QueryRow(`SELECT id FROM funcoes WHERE chave = 'enc_pessoal'`).Scan(&fPess); err != nil {
		t.Fatalf("cadeira enc_pessoal ausente: %v", err)
	}
	if err := st.db.QueryRow(`SELECT id FROM funcoes WHERE chave = 'enc_material'`).Scan(&fMat); err != nil {
		t.Fatalf("cadeira enc_material ausente: %v", err)
	}
	criaUsuarioTeste(t, st, "enc_ger_"+sufixo, "senha-ger", "gerente")
	criaUsuarioTeste(t, st, "enc_comum_"+sufixo, "senha-x", "")
	if _, err := st.db.Exec(`UPDATE usuarios SET grupo_id = ? WHERE login IN ('enc_ger_`+sufixo+`','enc_comum_`+sufixo+`')`, gid); err != nil {
		t.Fatalf("vincular grupo: %v", err)
	}
	if err := st.db.QueryRow(`SELECT id FROM usuarios WHERE login = 'enc_ger_`+sufixo+`'`).Scan(&idGer); err != nil {
		t.Fatalf("gerente: %v", err)
	}
	if err := st.db.QueryRow(`SELECT id FROM usuarios WHERE login = 'enc_comum_`+sufixo+`'`).Scan(&idComum); err != nil {
		t.Fatalf("comum: %v", err)
	}
	ckGer = loginAs(t, app, "enc_ger_"+sufixo, "senha-ger")
	return
}

// designarViaAPI: designa pelo endpoint real da UI (POST /api/grupo/funcoes/membros).
func designarViaAPI(t *testing.T, app *App, ckGer *http.Cookie, funcaoID, usuarioID int64, titularidade string) {
	t.Helper()
	rr, res := doJSONReq(app, "POST", "/api/grupo/funcoes/membros", map[string]any{
		"funcao_id": funcaoID, "usuario_id": usuarioID, "titularidade": titularidade,
	}, ckGer)
	if rr.Code != http.StatusOK {
		t.Fatalf("designar via API: esperado 200, veio %d (%v)", rr.Code, res)
	}
}

// TestEncMatrizPoderPorDesignacao (T1): matriz completa — sem designação 403;
// titular e auxiliar de pessoal com leitura+escrita de pessoal (e NÃO material);
// titular e auxiliar de material com leitura+escrita de material (e NÃO
// pessoal); designação removida → 403 de novo na MESMA sessão (R4).
func TestEncMatrizPoderPorDesignacao(t *testing.T) {
	app, st, cleanup := setupTestApp(t)
	defer cleanup()
	gid, fPess, fMat, _, idComum, ckGer := encSetupBase(t, app, st, "m1")

	// --- sem designação: 403 em pessoal (escrita) e material (leitura/escrita)
	ck := loginAs(t, app, "enc_comum_m1", "senha-x")
	if rr, _ := doJSONReq(app, "POST", "/api/pessoas", map[string]any{"nome_guerra": "SEMPODER", "nome_completo": "Sem Poder"}, ck); rr.Code != http.StatusForbidden {
		t.Fatalf("sem designação: POST /api/pessoas deve 403, veio %d", rr.Code)
	}
	if rr, _ := doJSONReq(app, "GET", "/api/material/itens", nil, ck); rr.Code != http.StatusForbidden {
		t.Fatalf("sem designação: GET /api/material/itens deve 403, veio %d", rr.Code)
	}
	if rr, _ := doJSONReq(app, "POST", "/api/material/itens", map[string]any{"nome": "Item Sem Poder"}, ck); rr.Code != http.StatusForbidden {
		t.Fatalf("sem designação: POST /api/material/itens deve 403, veio %d", rr.Code)
	}

	// --- titular de PESSOAL designado pela UI: pessoal ok, material negado
	designarViaAPI(t, app, ckGer, fPess, idComum, "titular")
	if rr, res := doJSONReq(app, "POST", "/api/pessoas", map[string]any{"nome_guerra": "TITPESS", "nome_completo": "Titular Pessoal", "status": "ativo"}, ck); rr.Code != http.StatusOK {
		t.Fatalf("enc pessoal titular: POST /api/pessoas deve 200, veio %d (%v)", rr.Code, res)
	}
	if rr, _ := doJSONReq(app, "GET", "/api/pessoas", nil, ck); rr.Code != http.StatusOK {
		t.Fatalf("enc pessoal titular: GET /api/pessoas deve 200, veio %d", rr.Code)
	}
	var pid float64
	rrP, _ := doJSONReq(app, "GET", "/api/pessoas", nil, ck)
	var pessoasObj map[string]any
	_ = json.Unmarshal(rrP.Body.Bytes(), &pessoasObj)
	listaPess, _ := pessoasObj["pessoas"].([]any)
	if len(listaPess) == 0 {
		t.Fatalf("enc pessoal: listagem vazia — não há o que editar")
	}
	linha0, _ := listaPess[0].(map[string]any)
	pid, _ = linha0["id"].(float64)
	if rr, _ := doJSONReq(app, "PATCH", "/api/pessoas/"+idi(int64(pid)), map[string]any{"nome_guerra": "TITPESS2"}, ck); rr.Code != http.StatusOK {
		t.Fatalf("enc pessoal titular: PATCH /api/pessoas deve 200, veio %d", rr.Code)
	}
	if rr, _ := doJSONReq(app, "GET", "/api/material/itens", nil, ck); rr.Code != http.StatusForbidden {
		t.Fatalf("enc pessoal titular: material deve continuar 403, veio %d", rr.Code)
	}

	// /api/me traz funcoes_grupo com a chave (fonte da sidebar do front)
	_, resMe := doJSONReq(app, "GET", "/api/me", nil, ck)
	uMap, _ := resMe["usuario"].(map[string]any)
	if uMap == nil {
		t.Fatalf("/api/me sem usuario: %v", resMe)
	}
	fg, _ := uMap["funcoes_grupo"].([]any)
	achou := false
	for _, it := range fg {
		if m, _ := it.(map[string]any); m != nil {
			if ch, _ := m["chave"].(string); ch == "enc_pessoal" {
				achou = true
			}
		}
	}
	if !achou {
		t.Fatalf("/api/me não trouxe funcoes_grupo[chave=enc_pessoal]: %v", fg)
	}

	// --- auxiliar de PESSOAL: mesmo poder
	criaUsuarioTeste(t, st, "enc_aux_m1", "senha-y", "")
	var idAux int64
	_ = st.db.QueryRow(`SELECT id FROM usuarios WHERE login='enc_aux_m1'`).Scan(&idAux)
	_, _ = st.db.Exec(`UPDATE usuarios SET grupo_id = ? WHERE id = ?`, gid, idAux)
	designarViaAPI(t, app, ckGer, fPess, idAux, "auxiliar")
	ckAux := loginAs(t, app, "enc_aux_m1", "senha-y")
	if rr, _ := doJSONReq(app, "POST", "/api/pessoas", map[string]any{"nome_guerra": "AUXPESS", "nome_completo": "Auxiliar Pessoal", "status": "ativo"}, ckAux); rr.Code != http.StatusOK {
		t.Fatalf("enc pessoal auxiliar: POST /api/pessoas deve 200, veio %d", rr.Code)
	}

	// --- designação REMOVIDA → poder some na mesma sessão (R4)
	var memID int64
	if err := st.db.QueryRow(`SELECT id FROM funcao_membros WHERE funcao_id=? AND grupo_id=? AND usuario_id=?`, fPess, gid, idComum).Scan(&memID); err != nil {
		t.Fatalf("designação do titular não encontrada: %v", err)
	}
	if rr, _ := doJSONReq(app, "DELETE", "/api/grupo/funcoes/membros/"+idi(memID), nil, ckGer); rr.Code != http.StatusOK {
		t.Fatalf("remover designação: deve 200, veio %d", rr.Code)
	}
	if rr, _ := doJSONReq(app, "POST", "/api/pessoas", map[string]any{"nome_guerra": "PODERFOI", "nome_completo": "Poder Foi"}, ck); rr.Code != http.StatusForbidden {
		t.Fatalf("R4: designação removida → POST /api/pessoas deve 403 na mesma sessão, veio %d", rr.Code)
	}
	if rr, _ := doJSONReq(app, "GET", "/api/material/itens", nil, ck); rr.Code != http.StatusForbidden {
		t.Fatalf("R4: designação removida → material segue 403, veio %d", rr.Code)
	}

	// --- titular e auxiliar de MATERIAL: leitura+escrita de material, pessoal negado
	designarViaAPI(t, app, ckGer, fMat, idComum, "titular")
	if rr, res := doJSONReq(app, "POST", "/api/material/itens", map[string]any{"nome": "Item do Enc Mat", "status": "disponivel", "sensibilidade": "convencional"}, ck); rr.Code != http.StatusOK {
		t.Fatalf("enc material titular: POST /api/material/itens deve 200, veio %d (%v)", rr.Code, res)
	}
	if rr, _ := doJSONReq(app, "GET", "/api/material/itens", nil, ck); rr.Code != http.StatusOK {
		t.Fatalf("enc material titular: GET /api/material/itens deve 200, veio %d", rr.Code)
	}
	if rr, _ := doJSONReq(app, "GET", "/api/material/categorias", nil, ck); rr.Code != http.StatusOK {
		t.Fatalf("enc material titular: GET categorias deve 200, veio %d", rr.Code)
	}
	if rr, _ := doJSONReq(app, "POST", "/api/pessoas", map[string]any{"nome_guerra": "MATNAOPESS", "nome_completo": "Mat Não Pessoal"}, ck); rr.Code != http.StatusForbidden {
		t.Fatalf("enc material: pessoal deve 403, veio %d", rr.Code)
	}
	// auxiliar de material: mesmo poder
	designarViaAPI(t, app, ckGer, fMat, idAux, "auxiliar")
	if rr, _ := doJSONReq(app, "POST", "/api/material/itens", map[string]any{"nome": "Item do Aux Mat", "status": "disponivel", "sensibilidade": "convencional"}, ckAux); rr.Code != http.StatusOK {
		t.Fatalf("enc material auxiliar: POST /api/material/itens deve 200, veio %d", rr.Code)
	}
}

// TestEncFuncaoSemChaveGanhaPoderAposV42 (T2/R3): banco legado com cadeira de
// grupo SEM chave (nome canônico, janela pré-v39) e designação pendente sobre
// ela — migrarV42 crava a chave e o designado passa a ter poder. Prova também
// pela UI: a designação nova na cadeira resolvida concede (200).
func TestEncFuncaoSemChaveGanhaPoderAposV42(t *testing.T) {
	app, st, cleanup := setupTestApp(t)
	defer cleanup()
	gid, _, _, _, idComum, ckGerM2 := encSetupBase(t, app, st, "m2")
	_ = ckGerM2

	// cadeira legada: tipo=grupo, chave NULL, nome canônico variante
	var fLegada int64
	if err := st.db.QueryRow(`INSERT INTO funcoes (nome, grupo_id, tipo) VALUES ('Encarregado De Pessoal', ?, 'grupo') RETURNING id`, gid).Scan(&fLegada); err != nil {
		t.Fatalf("criar cadeira legada: %v", err)
	}
	// designação no banco legado sobre a cadeira sem chave
	if _, err := st.db.Exec(`INSERT INTO funcao_membros (funcao_id, grupo_id, usuario_id, titularidade) VALUES (?,?,?,'titular')`, fLegada, gid, idComum); err != nil {
		t.Fatalf("designar na cadeira sem chave: %v", err)
	}

	// ANTES da v42: sem chave → sem poder nenhum
	ck := loginAs(t, app, "enc_comum_m2", "senha-x")
	if rr, _ := doJSONReq(app, "POST", "/api/pessoas", map[string]any{"nome_guerra": "ANTES42", "nome_completo": "Antes 42"}, ck); rr.Code != http.StatusForbidden {
		t.Fatalf("antes da v42: designado em cadeira sem chave deve 403, veio %d", rr.Code)
	}

	// upgrade: roda a v42 (simula arranque do binário novo)
	if _, err := st.db.Exec(`DELETE FROM schema_migrations WHERE versao = 42`); err != nil {
		t.Fatalf("resetar marcador v42: %v", err)
	}
	if err := st.migrarV42(); err != nil {
		t.Fatalf("migrarV42: %v", err)
	}

	// DEPOIS: poder concedido na MESMA sessão (UsuarioDaSessao relê a cada request)
	if rr, res := doJSONReq(app, "POST", "/api/pessoas", map[string]any{"nome_guerra": "DEPOIS42", "nome_completo": "Depois 42", "status": "ativo"}, ck); rr.Code != http.StatusOK {
		t.Fatalf("depois da v42: POST /api/pessoas deve 200, veio %d (%v)", rr.Code, res)
	}
	// a v42 desativou a legada e REAPOU as designações para a cadeira oficial
	var ativa int
	_ = st.db.QueryRow(`SELECT ativo FROM funcoes WHERE id = ?`, fLegada).Scan(&ativa)
	if ativa != 0 {
		t.Fatalf("v42 deveria ter desativado a cadeira legada duplicada (ativo=%d)", ativa)
	}
	var nFM int
	_ = st.db.QueryRow(`SELECT COUNT(*) FROM funcao_membros fm JOIN funcoes f ON f.id = fm.funcao_id WHERE f.chave = 'enc_pessoal' AND fm.usuario_id = ?`, idComum).Scan(&nFM)
	if nFM != 1 {
		t.Fatalf("designação não reapontada para a cadeira oficial (n=%d)", nFM)
	}

	// idempotência: rodar de novo não muda nada e não erra
	if err := st.migrarV42(); err != nil {
		t.Fatalf("migrarV42 idempotência: %v", err)
	}
	var nChave int
	_ = st.db.QueryRow(`SELECT COUNT(*) FROM funcoes WHERE chave = 'enc_pessoal'`).Scan(&nChave)
	if nChave != 1 {
		t.Fatalf("idempotência: esperado 1 cadeira enc_pessoal, veio %d", nChave)
	}
}

// TestEncPapelSistemaTrava (T3/R5): doutrina 08/10 — papel do sistema trava.
// Operador designado (mesmo titular) segue SEM poder; e a v42 NÃO destrava a
// conta cujo usuarios.papel já é operador (papel real permanece).
func TestEncPapelSistemaTrava(t *testing.T) {
	app, st, cleanup := setupTestApp(t)
	defer cleanup()
	gid, fPess, _, _, _, ckGerM3 := encSetupBase(t, app, st, "m3")

	criaUsuarioTeste(t, st, "enc_op_m3", "senha-op", "operador")
	var idOp int64
	_ = st.db.QueryRow(`SELECT id FROM usuarios WHERE login='enc_op_m3'`).Scan(&idOp)
	_, _ = st.db.Exec(`UPDATE usuarios SET grupo_id = ? WHERE id = ?`, gid, idOp)
	designarViaAPI(t, app, ckGerM3, fPess, idOp, "titular")

	ck := loginAs(t, app, "enc_op_m3", "senha-op")
	if rr, _ := doJSONReq(app, "POST", "/api/pessoas", map[string]any{"nome_guerra": "OPTRAVA", "nome_completo": "Operador Trava"}, ck); rr.Code != http.StatusForbidden {
		t.Fatalf("R5: operador designado deve seguir 403, veio %d", rr.Code)
	}
	// leitura de material: operador JÁ tem por desenho (authMaterial libera
	// operador) — o teste de trava é no domínio de PESSOAL do designado.
	if rr, _ := doJSONReq(app, "POST", "/api/material/itens", map[string]any{"nome": "Item de Operador"}, ck); rr.Code != http.StatusOK {
		t.Fatalf("sanidade: operador comum lê/escreve material por desenho do authMaterial (200), veio %d", rr.Code)
	}
	// a linha de papel REAL do operador sobrevive à v42 (não é fantasia)
	if _, err := st.db.Exec(`DELETE FROM schema_migrations WHERE versao = 42`); err != nil {
		t.Fatalf("resetar marcador v42: %v", err)
	}
	if err := st.migrarV42(); err != nil {
		t.Fatalf("migrarV42: %v", err)
	}
	var n int
	_ = st.db.QueryRow(`SELECT COUNT(*) FROM usuario_papeis WHERE usuario_id = ? AND papel = 'operador'`, idOp).Scan(&n)
	if n != 1 {
		t.Fatalf("v42 apagou papel REAL de operador (n=%d) — regressão", n)
	}
	ck2 := loginAs(t, app, "enc_op_m3", "senha-op")
	if rr, _ := doJSONReq(app, "POST", "/api/pessoas", map[string]any{"nome_guerra": "OPTRAVA2", "nome_completo": "Operador Trava 2"}, ck2); rr.Code != http.StatusForbidden {
		t.Fatalf("R5: após v42 operador designado deve seguir 403, veio %d", rr.Code)
	}
}

// TestEncV42DestravaFantasiaESoltaSessao (T4): o estado exato que o bug de
// produção deixava no banco — designado sem papel + linha-fantasia
// usuario_papeis('operador') + sessão presa nela — migra para o estado são:
// fantasia apagada, sessão com papel_ativo_id NULL, poder concedido na mesma
// sessão. E o sync NOVO não fabrica mais linha.
func TestEncV42DestravaFantasiaESoltaSessao(t *testing.T) {
	app, st, cleanup := setupTestApp(t)
	defer cleanup()
	gid, fPess, _, _, idComum, ckGer := encSetupBase(t, app, st, "m4")

	// designação pela UI NOVA (sync não fabrica mais linha em usuario_papeis)
	designarViaAPI(t, app, ckGer, fPess, idComum, "titular")
	var nLinha int
	_ = st.db.QueryRow(`SELECT COUNT(*) FROM usuario_papeis WHERE usuario_id = ?`, idComum).Scan(&nLinha)
	if nLinha != 0 {
		t.Fatalf("sync novo fabricou linha em usuario_papeis (n=%d) — regressão do bug", nLinha)
	}

	// reconstrói o estrago legado: fantasia 'operador' + sessão presa nela
	var fantasiaID int64
	if err := st.db.QueryRow(`INSERT INTO usuario_papeis (usuario_id, grupo_id, papel, funcao_id) VALUES (?, ?, 'operador', ?) RETURNING id`, idComum, gid, fPess).Scan(&fantasiaID); err != nil {
		t.Fatalf("semear fantasia: %v", err)
	}
	ck := loginAs(t, app, "enc_comum_m4", "senha-x") // sessão pega a fantasia (única linha)
	// prova do bug: com a fantasia ativa, o designado está travado
	if rr, _ := doJSONReq(app, "POST", "/api/pessoas", map[string]any{"nome_guerra": "PRESO", "nome_completo": "Designado Preso"}, ck); rr.Code != http.StatusForbidden {
		t.Fatalf("estado estrangulado: designado deve estar travado (403), veio %d", rr.Code)
	}

	// upgrade: v42 destrava
	if _, err := st.db.Exec(`DELETE FROM schema_migrations WHERE versao = 42`); err != nil {
		t.Fatalf("resetar marcador v42: %v", err)
	}
	if err := st.migrarV42(); err != nil {
		t.Fatalf("migrarV42: %v", err)
	}
	_ = st.db.QueryRow(`SELECT COUNT(*) FROM usuario_papeis WHERE id = ?`, fantasiaID).Scan(&nLinha)
	if nLinha != 0 {
		t.Fatalf("v42 não apagou a linha-fantasia (n=%d)", nLinha)
	}
	// mesma sessão: solta (papel_ativo_id NULL) + poder concedido
	if rr, res := doJSONReq(app, "POST", "/api/pessoas", map[string]any{"nome_guerra": "SOLTO", "nome_completo": "Designado Solto", "status": "ativo"}, ck); rr.Code != http.StatusOK {
		t.Fatalf("pós-v42: designado destravado deve 200, veio %d (%v)", rr.Code, res)
	}
	_, resMe := doJSONReq(app, "GET", "/api/me", nil, ck)
	uMap, _ := resMe["usuario"].(map[string]any)
	if uMap == nil {
		t.Fatalf("/api/me sem usuario pós-v42: %v", resMe)
	}
	if p, _ := uMap["papel"].(string); p != "" {
		t.Fatalf("pós-v42: papel da sessão do designado deve ser vazio, veio %q", p)
	}
	// gerente (linha usuario_papeis 'gerente' legítima) NÃO é tocado
	var nGer int
	_ = st.db.QueryRow(`SELECT COUNT(*) FROM usuario_papeis WHERE papel = 'gerente' AND grupo_id = ?`, gid).Scan(&nGer)
	if nGer < 1 {
		t.Fatalf("v42 apagou papel de gerente — regressão")
	}
}
