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
// Cobertura (atualizada na v1.6.0 Fase 2 — o PODER segue o CONTEXTO ATIVO,
// linha materializada em usuario_papeis que a v45 criou):
//   T1  Matriz completa de poder por designação MATERIALIZADA (titular/
//       auxiliar, pessoal/material, leitura/escrita; R4: revogada a linha →
//       403 na MESMA sessão e re-materializar não traz o poder de volta).
//   T2  Cadeira de grupo legada SEM chave + designação → migrarV42 crava a
//       chave, a v45 materializa e a MESMA sessão resolve o contexto.
//   T3  Operador designado NO CONTEXTO operador é operador; a troca para a
//       linha da cadeira abre o poder do cargo (sem vazar para a outra
//       cadeira); v42 NÃO apaga papel REAL de operador.
//   T4  Linha-fantasia 'operador' + sessão presa: com a fantasia ativa o
//       designado é operador (403); v42 apaga a fantasia e a v45 devolve o
//       CONTEXTO da cadeira à MESMA sessão; designação materializa 1 linha.

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

// revogaLinhaMaterializada: apaga a linha de papel ESPELHO da cadeira e
// RE-CHAVEIA as sessões presas nela (para outra linha do usuário; se não há,
// para NULL) — exatamente o que o hFuncaoMembrosDel passa a fazer na Fase 3,
// no padrão do hUsuarioPapelDel. Idempotente: sem linha, no-op.
// v1.6.0 Fase 2: prova que o PODER é do CONTEXTO (a linha), não da designação.
func revogaLinhaMaterializada(t *testing.T, st *Store, usuarioID int64, papel string) {
	t.Helper()
	var linhas []int64
	rows, err := st.db.Query(`SELECT id FROM usuario_papeis WHERE usuario_id = ? AND papel = ?`, usuarioID, papel)
	if err != nil {
		t.Fatalf("ler linhas %s do usuário %d: %v", papel, usuarioID, err)
	}
	for rows.Next() {
		var id int64
		if rows.Scan(&id) == nil {
			linhas = append(linhas, id)
		}
	}
	rows.Close()
	for _, id := range linhas {
		if _, err := st.db.Exec(`DELETE FROM usuario_papeis WHERE id = ? AND usuario_id = ?`, id, usuarioID); err != nil {
			t.Fatalf("revogar linha %d: %v", id, err)
		}
		var outro int64
		_ = st.db.QueryRow(`SELECT id FROM usuario_papeis WHERE usuario_id = ? AND id != ? LIMIT 1`, usuarioID, id).Scan(&outro)
		if outro > 0 {
			if _, err := st.db.Exec(`UPDATE sessoes SET papel_ativo_id = ? WHERE papel_ativo_id = ?`, outro, id); err != nil {
				t.Fatalf("re-chavear sessões da linha %d: %v", id, err)
			}
		} else if _, err := st.db.Exec(`UPDATE sessoes SET papel_ativo_id = NULL WHERE papel_ativo_id = ?`, id); err != nil {
			t.Fatalf("soltar sessões da linha %d: %v", id, err)
		}
	}
}

// TestEncMatrizPoderPorDesignacao (T1): matriz completa — sem designação 403;
// titular e auxiliar de pessoal com leitura+escrita de pessoal (e NÃO material);
// titular e auxiliar de material com leitura+escrita de material (e NÃO
// pessoal); designação revogada → 403 de novo na MESMA sessão (R4).
// v1.6.0 Fase 2: o PODER segue o CONTEXTO ATIVO — a designação vale quando a
// linha materializada existe (v45 materializa; Fase 3 sincroniza o handler) e
// a sessão resolve o papel enc (re-key da migração ou troca no dropdown).
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

	// --- titular de PESSOAL: designação + MATERIALIZAÇÃO — a MESMA sessão
	// (designado puro, papel_ativo_id NULL) é re-keyada pela v45 para o contexto
	designarViaAPI(t, app, ckGer, fPess, idComum, "titular")
	v45Reexecuta(t, st)
	if p := f2MePapel(t, app, ck); p != "enc_pessoal" {
		t.Fatalf("designado titular devia resolver o CONTEXTO enc_pessoal na mesma sessão, veio %q", p)
	}
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
	v45Reexecuta(t, st)
	ckAux := loginAs(t, app, "enc_aux_m1", "senha-y")
	if rr, _ := doJSONReq(app, "POST", "/api/pessoas", map[string]any{"nome_guerra": "AUXPESS", "nome_completo": "Auxiliar Pessoal", "status": "ativo"}, ckAux); rr.Code != http.StatusOK {
		t.Fatalf("enc pessoal auxiliar: POST /api/pessoas deve 200, veio %d", rr.Code)
	}

	// --- designação REMOVIDA → o PODER some na MESMA sessão (R4). A revogação
	// real é do espelho: apagar a linha materializada (o que o del-sync da Fase 3
	// faz) derruba o contexto — e re-materializar NÃO traz o poder de volta,
	// porque a designação não existe mais.
	var memID int64
	if err := st.db.QueryRow(`SELECT id FROM funcao_membros WHERE funcao_id=? AND grupo_id=? AND usuario_id=?`, fPess, gid, idComum).Scan(&memID); err != nil {
		t.Fatalf("designação do titular não encontrada: %v", err)
	}
	if rr, _ := doJSONReq(app, "DELETE", "/api/grupo/funcoes/membros/"+idi(memID), nil, ckGer); rr.Code != http.StatusOK {
		t.Fatalf("remover designação: deve 200, veio %d", rr.Code)
	}
	revogaLinhaMaterializada(t, st, idComum, "enc_pessoal")
	if rr, _ := doJSONReq(app, "POST", "/api/pessoas", map[string]any{"nome_guerra": "PODERFOI", "nome_completo": "Poder Foi"}, ck); rr.Code != http.StatusForbidden {
		t.Fatalf("R4: designação revogada → POST /api/pessoas deve 403 na mesma sessão, veio %d", rr.Code)
	}
	if rr, _ := doJSONReq(app, "GET", "/api/material/itens", nil, ck); rr.Code != http.StatusForbidden {
		t.Fatalf("R4: designação revogada → material segue 403, veio %d", rr.Code)
	}
	v45Reexecuta(t, st)
	if rr, _ := doJSONReq(app, "POST", "/api/pessoas", map[string]any{"nome_guerra": "PODERFOI2", "nome_completo": "Poder Nao Volta"}, ck); rr.Code != http.StatusForbidden {
		t.Fatalf("R4: sem designação não há linha a materializar — segue 403, veio %d", rr.Code)
	}

	// --- titular e auxiliar de MATERIAL: leitura+escrita de material, pessoal negado
	designarViaAPI(t, app, ckGer, fMat, idComum, "titular")
	v45Reexecuta(t, st)
	if p := f2MePapel(t, app, ck); p != "enc_material" {
		t.Fatalf("designado em material devia resolver o CONTEXTO enc_material, veio %q", p)
	}
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
	// auxiliar de material: mesmo poder — no CONTEXTO da cadeira (a sessão do
	// aux estava no contexto enc_pessoal; a troca é a da UI, mesmo cookie)
	designarViaAPI(t, app, ckGer, fMat, idAux, "auxiliar")
	v45Reexecuta(t, st)
	f2TrocaContexto(t, app, ckAux, f2LinhaPapel(t, st, idAux, "enc_material"))
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

	// DEPOIS: a v42 crava a chave e REAPONTA a designação; a v45 MATERIALIZA a
	// cadeira e re-chaveia a MESMA sessão (designado puro, papel_ativo_id NULL
	// → 1ª linha enc) — o poder concede sem re-login (UsuarioDaSessao relê).
	v45Reexecuta(t, st)
	if p := f2MePapel(t, app, ck); p != "enc_pessoal" {
		t.Fatalf("pós-v42+v45: designado devia resolver o CONTEXTO enc_pessoal na mesma sessão, veio %q", p)
	}
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

// TestEncPapelSistemaDesignadoTemPoder (T3, v367): o CARGO concede quando o
// CONTEXTO dele está ativo (v1.6.0 Fase 2): operador designado NO CONTEXTO
// operador é operador (sem poder do cargo); trocando para a linha da cadeira o
// poder abre — e a designação em UMA cadeira não vaza para a outra. Operador
// SEM designação segue sem poder; v42 NÃO apaga papel REAL de operador (a linha
// em usuario_papeis é a cadeira do sistema, não fantasia).
func TestEncPapelSistemaDesignadoTemPoder(t *testing.T) {
	app, st, cleanup := setupTestApp(t)
	defer cleanup()
	gid, fPess, fMat, _, _, ckGerM3 := encSetupBase(t, app, st, "m3")

	criaUsuarioTeste(t, st, "enc_op_m3", "senha-op", "operador")
	var idOp int64
	_ = st.db.QueryRow(`SELECT id FROM usuarios WHERE login='enc_op_m3'`).Scan(&idOp)
	_, _ = st.db.Exec(`UPDATE usuarios SET grupo_id = ? WHERE id = ?`, gid, idOp)

	// ANTES da designação: operador sem cadeira → 403 (fail-closed, v367 mantém)
	ck := loginAs(t, app, "enc_op_m3", "senha-op")
	if rr, _ := doJSONReq(app, "POST", "/api/pessoas", map[string]any{"nome_guerra": "OPSEMCARGO", "nome_completo": "Operador Sem Cargo"}, ck); rr.Code != http.StatusForbidden {
		t.Fatalf("operador sem designação deve 403, veio %d", rr.Code)
	}
	if rr, _ := doJSONReq(app, "POST", "/api/material/responsaveis", map[string]any{"grupo_id": gid, "setor_id": 0, "encarregado_id": 0, "auxiliar_encarregado_id": 0}, ck); rr.Code != http.StatusForbidden {
		t.Fatalf("operador sem designação: rota de cargo do material deve 403, veio %d", rr.Code)
	}

	// designado em enc_pessoal: a linha materializa DEPOIS da do sistema
	// (ORDER BY id) → o login default CONTINUA 'operador' e, nele, operador é
	// operador — o poder do cargo NÃO vaza para o contexto errado.
	designarViaAPI(t, app, ckGerM3, fPess, idOp, "titular")
	v45Reexecuta(t, st)
	if p := f2MePapel(t, app, ck); p != "operador" {
		t.Fatalf("contexto default devia continuar 'operador' (linha de sistema nasceu antes), veio %q", p)
	}
	if rr, _ := doJSONReq(app, "POST", "/api/pessoas", map[string]any{"nome_guerra": "OPCTXOP", "nome_completo": "Operador No Contexto Operador"}, ck); rr.Code != http.StatusForbidden {
		t.Fatalf("Fase 2: no CONTEXTO operador, POST /api/pessoas deve 403, veio %d", rr.Code)
	}

	// troca de contexto para a cadeira (dropdown) → o poder do cargo abre
	f2TrocaContexto(t, app, ck, f2LinhaPapel(t, st, idOp, "enc_pessoal"))
	if rr, res := doJSONReq(app, "POST", "/api/pessoas", map[string]any{"nome_guerra": "OPCARGO", "nome_completo": "Operador Com Cargo", "status": "ativo"}, ck); rr.Code != http.StatusOK {
		t.Fatalf("contexto enc_pessoal: operador designado deve 200 em pessoal, veio %d (%v)", rr.Code, res)
	}
	if rr, _ := doJSONReq(app, "POST", "/api/material/responsaveis", map[string]any{"grupo_id": gid, "setor_id": 0, "encarregado_id": 0, "auxiliar_encarregado_id": 0}, ck); rr.Code != http.StatusForbidden {
		t.Fatalf("designado SÓ em enc_pessoal: rota de cargo do material deve 403, veio %d", rr.Code)
	}

	// designado TAMBÉM em enc_material (auxiliar): no CONTEXTO enc_material a
	// rota de cargo do material passa — e a pessoal fecha de novo
	designarViaAPI(t, app, ckGerM3, fMat, idOp, "auxiliar")
	v45Reexecuta(t, st)
	f2TrocaContexto(t, app, ck, f2LinhaPapel(t, st, idOp, "enc_material"))
	if rr, res := doJSONReq(app, "POST", "/api/material/responsaveis", map[string]any{"grupo_id": gid, "setor_id": 0, "encarregado_id": 0, "auxiliar_encarregado_id": 0}, ck); rr.Code != http.StatusOK {
		t.Fatalf("contexto enc_material: rota de cargo deve 200, veio %d (%v)", rr.Code, res)
	}
	if rr, _ := doJSONReq(app, "POST", "/api/pessoas", map[string]any{"nome_guerra": "MATNAOPESS3", "nome_completo": "Mat Não Pessoal 3"}, ck); rr.Code != http.StatusForbidden {
		t.Fatalf("contexto enc_material: pessoal deve 403, veio %d", rr.Code)
	}

	// a v42 não apaga papel REAL de operador (não é fantasia) — e o login
	// seguinte nasce NO CONTEXTO operador; a cadeira segue disponível no dropdown
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
	if p := f2MePapel(t, app, ck2); p != "operador" {
		t.Fatalf("pós-v42: login devia nascer no CONTEXTO operador, veio %q", p)
	}
	f2TrocaContexto(t, app, ck2, f2LinhaPapel(t, st, idOp, "enc_pessoal"))
	if rr, _ := doJSONReq(app, "POST", "/api/pessoas", map[string]any{"nome_guerra": "OPCARGO2", "nome_completo": "Operador Com Cargo 2", "status": "ativo"}, ck2); rr.Code != http.StatusOK {
		t.Fatalf("pós-v42: o contexto enc_pessoal segue concedendo o poder do cargo, veio %d", rr.Code)
	}
}

// TestEncV42DestravaFantasiaESoltaSessao (T4): o estado que o bug de produção
// deixava no banco — designado sem papel + linha-fantasia usuario_papeis
// ('operador') + sessão presa nela. v1.6.0 Fase 2: com a fantasia ativa o
// designado É operador — a cadeira não está ativa e o poder NÃO vale (a
// segregação por contexto é exatamente essa); a v42 apaga a fantasia, solta a
// sessão e a v45 devolve o CONTEXTO da cadeira À MESMA sessão.
func TestEncV42DestravaFantasiaESoltaSessao(t *testing.T) {
	app, st, cleanup := setupTestApp(t)
	defer cleanup()
	gid, fPess, _, _, idComum, ckGer := encSetupBase(t, app, st, "m4")

	// fantasia ANTES de tudo: a linha menor é a que o login resolve (ORDER BY id)
	var fantasiaID int64
	if err := st.db.QueryRow(`INSERT INTO usuario_papeis (usuario_id, grupo_id, papel, funcao_id) VALUES (?, ?, 'operador', ?) RETURNING id`, idComum, gid, fPess).Scan(&fantasiaID); err != nil {
		t.Fatalf("semear fantasia: %v", err)
	}

	// designação pela UI + materialização da cadeira (v45 materializa o legado;
	// a Fase 3 sincroniza o handler — em ambos os estados resta EXATAMENTE 1
	// linha enc além da fantasia)
	designarViaAPI(t, app, ckGer, fPess, idComum, "titular")
	v45Reexecuta(t, st)
	var nEnc int
	_ = st.db.QueryRow(`SELECT COUNT(*) FROM usuario_papeis WHERE usuario_id = ? AND papel = 'enc_pessoal'`, idComum).Scan(&nEnc)
	if nEnc != 1 {
		t.Fatalf("designação devia materializar EXATAMENTE 1 linha enc_pessoal (n=%d)", nEnc)
	}

	// sessão pega a FANTASIA (linha de id menor — a mesma prioridade do login
	// real): no contexto 'operador' o designado é operador, sem poder do cargo
	ck := loginAs(t, app, "enc_comum_m4", "senha-x")
	if rr, _ := doJSONReq(app, "POST", "/api/pessoas", map[string]any{"nome_guerra": "PRESO", "nome_completo": "Designado Preso"}, ck); rr.Code != http.StatusForbidden {
		t.Fatalf("Fase 2: com a fantasia 'operador' ativa o cargo NÃO vale (403), veio %d", rr.Code)
	}

	// upgrade: v42 destrava (apaga a fantasia, solta a sessão)…
	if _, err := st.db.Exec(`DELETE FROM schema_migrations WHERE versao = 42`); err != nil {
		t.Fatalf("resetar marcador v42: %v", err)
	}
	if err := st.migrarV42(); err != nil {
		t.Fatalf("migrarV42: %v", err)
	}
	var nLinha int
	_ = st.db.QueryRow(`SELECT COUNT(*) FROM usuario_papeis WHERE id = ?`, fantasiaID).Scan(&nLinha)
	if nLinha != 0 {
		t.Fatalf("v42 não apagou a linha-fantasia (n=%d)", nLinha)
	}
	// …e a v45 devolve o CONTEXTO da cadeira À MESMA sessão (re-key NULL→enc)
	v45Reexecuta(t, st)
	if p := f2MePapel(t, app, ck); p != "enc_pessoal" {
		t.Fatalf("pós-v42+v45: a mesma sessão devia resolver 'enc_pessoal', veio %q", p)
	}
	if rr, res := doJSONReq(app, "POST", "/api/pessoas", map[string]any{"nome_guerra": "SOLTO", "nome_completo": "Designado Solto", "status": "ativo"}, ck); rr.Code != http.StatusOK {
		t.Fatalf("pós-v42: designado destravado deve 200, veio %d (%v)", rr.Code, res)
	}
	// gerente (linha usuario_papeis 'gerente' legítima) NÃO é tocado
	var nGer int
	_ = st.db.QueryRow(`SELECT COUNT(*) FROM usuario_papeis WHERE papel = 'gerente' AND grupo_id = ?`, gid).Scan(&nGer)
	if nGer < 1 {
		t.Fatalf("v42 apagou papel de gerente — regressão")
	}
}
