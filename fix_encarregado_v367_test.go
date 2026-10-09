package main

// fix_encarregado_v367_test.go — ordem do dono 09/10 (v367): "ao nomear o
// encarregado de pessoal ou material, o usuário não recebe a função no dropdown
// de funções dele para seleção, nem ocorre a liberação do módulo necessário."
//
// Causa raiz provada (CEO): o trava-papel (operador/chefe_setor → false ANTES
// de consultar a designação) bloqueava o poder do cargo — paliativo do
// bug-fantasia morto na fonte na v366 (sync não fabrica mais linha).
// Causa 2: o sync de display só preenchia funcao_id NULO — linha com função
// STALE nunca atualizava, e a remoção da designação não limpa o funcao_id.
//
// Cobertura v367:
//   T2  Display ida-e-volta: designar sobre linha com funcao_id STALE sobrescreve
//       (dropdown mostra a cadeira certa); remover → NULL; designado puro sem
//       linha em usuario_papeis → NADA inventado.
//   T5  chefe_setor designado: poder do cargo (pessoal 200) + poderes de chefe
//       PRESERVADOS (cria operador do setor) + hierarquia de criação mantida
//       (não cria gerente); rotas exclusivas de cargo do material sem cadeira → 403.
//   T6  Setores: operador designado exclui setor do PRÓPRIO grupo (ramo restrito
//       por designação); gerente NÃO cai no ramo restrito (segue com subordinados).
//   T7  Fail-closed: sem designação → 403 (qualquer papel); designação em UMA
//       cadeira não abre a outra.

import (
	"database/sql"
	"net/http"
	"testing"
)

// TestV367DisplaySyncIdaVolta (T2/R3/R4): o dropdown de contexto do front lista
// usuario.papeis com funcao_nome vindo de usuario_papeis.funcao_id — prova
// ida-e-volta do sync de display pelos dados reais do /api/me.
func TestV367DisplaySyncIdaVolta(t *testing.T) {
	app, st, cleanup := setupTestApp(t)
	defer cleanup()
	gid, fPess, fMat, _, idComum, ckGer := encSetupBase(t, app, st, "d1")

	// STALE (estado do caso real 'teste'): linha do usuário no grupo aponta para
	// enc_pessoal, mas a designação verdadeira vai ser em enc_material.
	if _, err := st.db.Exec(`INSERT INTO usuario_papeis (usuario_id, grupo_id, papel, funcao_id) VALUES (?, ?, 'operador', ?)`, idComum, gid, fPess); err != nil {
		t.Fatalf("semear linha com funcao_id stale: %v", err)
	}
	designarViaAPI(t, app, ckGer, fMat, idComum, "titular")

	var fid int64
	if err := st.db.QueryRow(`SELECT funcao_id FROM usuario_papeis WHERE usuario_id = ? AND grupo_id = ?`, idComum, gid).Scan(&fid); err != nil {
		t.Fatalf("ler funcao_id pós-designação: %v", err)
	}
	if fid != fMat {
		t.Fatalf("R3 ida: funcao_id deve virar %d (enc_material), segue %d — stale nunca atualiza", fMat, fid)
	}

	// dropdown de contexto: funcao_nome do papel reflete a CADEIRA CERTA
	ck := loginAs(t, app, "enc_comum_d1", "senha-x")
	_, resMe := doJSONReq(app, "GET", "/api/me", nil, ck)
	uMap, _ := resMe["usuario"].(map[string]any)
	if uMap == nil {
		t.Fatalf("/api/me sem usuario: %v", resMe)
	}
	var nomeEsperado string
	if err := st.db.QueryRow(`SELECT nome FROM funcoes WHERE id = ?`, fMat).Scan(&nomeEsperado); err != nil {
		t.Fatalf("nome da cadeira: %v", err)
	}
	papeis, _ := uMap["papeis"].([]any)
	var fnome string
	for _, p := range papeis {
		pm, _ := p.(map[string]any)
		if pm == nil {
			continue
		}
		if g, _ := pm["grupo_id"].(float64); int64(g) == gid {
			fnome, _ = pm["funcao_nome"].(string)
		}
	}
	if fnome != nomeEsperado {
		t.Fatalf("R4: dropdown mostraria %q; esperado %q", fnome, nomeEsperado)
	}

	// volta: remover a designação → funcao_id da linha volta a NULL
	var memID int64
	if err := st.db.QueryRow(`SELECT id FROM funcao_membros WHERE funcao_id = ? AND grupo_id = ? AND usuario_id = ?`, fMat, gid, idComum).Scan(&memID); err != nil {
		t.Fatalf("designação não encontrada: %v", err)
	}
	if rr, _ := doJSONReq(app, "DELETE", "/api/grupo/funcoes/membros/"+idi(memID), nil, ckGer); rr.Code != http.StatusOK {
		t.Fatalf("remover designação: deve 200, veio %d", rr.Code)
	}
	var fidNull sql.NullInt64
	if err := st.db.QueryRow(`SELECT funcao_id FROM usuario_papeis WHERE usuario_id = ? AND grupo_id = ?`, idComum, gid).Scan(&fidNull); err != nil {
		t.Fatalf("ler funcao_id pós-remoção: %v", err)
	}
	if fidNull.Valid {
		t.Fatalf("R3 volta: funcao_id deveria ser NULL após remover a designação, segue %d", fidNull.Int64)
	}

	// designado PURO (sem linha em usuario_papeis): designar NÃO inventa linha
	// (conta 'd2' sem nenhuma linha prévia)
	criaUsuarioTeste(t, st, "enc_puro_d1", "senha-p", "")
	var idPuro int64
	_ = st.db.QueryRow(`SELECT id FROM usuarios WHERE login = 'enc_puro_d1'`).Scan(&idPuro)
	_, _ = st.db.Exec(`UPDATE usuarios SET grupo_id = ? WHERE id = ?`, gid, idPuro)
	designarViaAPI(t, app, ckGer, fPess, idPuro, "titular")
	var nLinhas int
	_ = st.db.QueryRow(`SELECT COUNT(*) FROM usuario_papeis WHERE usuario_id = ?`, idPuro).Scan(&nLinhas)
	if nLinhas != 0 {
		t.Fatalf("R3: designado puro ganhou linha em usuario_papeis (n=%d) — sync inventou", nLinhas)
	}
	// e o poder do cargo vale para o designado puro (regressão v366 mantida)
	ckPuro := loginAs(t, app, "enc_puro_d1", "senha-p")
	if rr, res := doJSONReq(app, "POST", "/api/pessoas", map[string]any{"nome_guerra": "PURO367", "nome_completo": "Designado Puro", "status": "ativo"}, ckPuro); rr.Code != http.StatusOK {
		t.Fatalf("designado puro: POST /api/pessoas deve 200, veio %d (%v)", rr.Code, res)
	}
}

// TestV367ChefeSetorDesignado (T5): chefe_setor com cadeira 'enc_pessoal' exerce
// o poder do cargo E mantém os poderes do próprio papel (cria operador do setor);
// hierarquia de criação segue (não cria gerente).
func TestV367ChefeSetorDesignado(t *testing.T) {
	app, st, cleanup := setupTestApp(t)
	defer cleanup()
	gid, fPess, _, _, _, ckGer := encSetupBase(t, app, st, "cs1")

	criaUsuarioTeste(t, st, "enc_cs_cs1", "senha-cs", "chefe_setor")
	var idCs int64
	_ = st.db.QueryRow(`SELECT id FROM usuarios WHERE login = 'enc_cs_cs1'`).Scan(&idCs)
	_, _ = st.db.Exec(`UPDATE usuarios SET grupo_id = ? WHERE id = ?`, gid, idCs)

	// sem designação: 403 em escrita de pessoal (fail-closed)
	ckCs := loginAs(t, app, "enc_cs_cs1", "senha-cs")
	if rr, _ := doJSONReq(app, "POST", "/api/pessoas", map[string]any{"nome_guerra": "CSSEMCARGO", "nome_completo": "Chefe Sem Cargo"}, ckCs); rr.Code != http.StatusForbidden {
		t.Fatalf("chefe_setor sem designação deve 403, veio %d", rr.Code)
	}

	// designado: poder do cargo na MESMA sessão
	designarViaAPI(t, app, ckGer, fPess, idCs, "titular")
	if rr, res := doJSONReq(app, "POST", "/api/pessoas", map[string]any{"nome_guerra": "CSCARGO", "nome_completo": "Chefe Com Cargo", "status": "ativo"}, ckCs); rr.Code != http.StatusOK {
		t.Fatalf("v367: chefe_setor designado deve 200 em pessoal, veio %d (%v)", rr.Code, res)
	}

	// poderes de chefe PRESERVADOS: cria operador do próprio grupo
	if rr, res := doJSONReq(app, "POST", "/api/usuarios", map[string]any{"login": "op_do_cs_367", "senha": "senha-opx", "papel": "operador"}, ckCs); rr.Code != http.StatusOK {
		t.Fatalf("chefe designado mantém criação de operador (200), veio %d (%v)", rr.Code, res)
	}

	// hierarquia de criação: NEM chefe nem designado cria GERENTE
	if rr, _ := doJSONReq(app, "POST", "/api/usuarios", map[string]any{"login": "ger_do_cs_367", "senha": "senha-gerx", "papel": "gerente"}, ckCs); rr.Code != http.StatusForbidden {
		t.Fatalf("chefe (mesmo designado) não cria gerente — deve 403, veio %d", rr.Code)
	}
	_ = gid
}
// TestV367SetorRamoRestritoPorDesignacao (T6): o ramo restrito do DELETE
// /api/setores/{id} passa a ser por designação — operador designado exclui setor
// do PRÓPRIO grupo; gerente NÃO cai no ramo restrito (segue excluindo setor de
// grupo SUBORDINADO).
func TestV367SetorRamoRestritoPorDesignacao(t *testing.T) {
	app, st, cleanup := setupTestApp(t)
	defer cleanup()
	gid, fPess, _, _, _, ckGer := encSetupBase(t, app, st, "st1")

	criaUsuarioTeste(t, st, "enc_op_st1", "senha-op", "operador")
	var idOp int64
	_ = st.db.QueryRow(`SELECT id FROM usuarios WHERE login = 'enc_op_st1'`).Scan(&idOp)
	_, _ = st.db.Exec(`UPDATE usuarios SET grupo_id = ? WHERE id = ?`, gid, idOp)

	var sProprio, sFora int64
	var gidFora int64
	if err := st.db.QueryRow(`INSERT INTO grupos (nome) VALUES ('Grp 367 Alheio') RETURNING id`).Scan(&gidFora); err != nil {
		t.Fatalf("criar grupo alheio: %v", err)
	}
	if err := st.db.QueryRow(`INSERT INTO setores (nome, grupo_id) VALUES ('Setor 367 Proprio', ?) RETURNING id`, gid).Scan(&sProprio); err != nil {
		t.Fatalf("criar setor próprio: %v", err)
	}
	if err := st.db.QueryRow(`INSERT INTO setores (nome, grupo_id) VALUES ('Setor 367 Alheio', ?) RETURNING id`, gidFora).Scan(&sFora); err != nil {
		t.Fatalf("criar setor alheio: %v", err)
	}

	// designado: exclui setor do PRÓPRIO grupo (ramo restrito por designação)
	designarViaAPI(t, app, ckGer, fPess, idOp, "titular")
	ckOp := loginAs(t, app, "enc_op_st1", "senha-op")
	if rr, res := doJSONReq(app, "DELETE", "/api/setores/"+idi(sProprio), nil, ckOp); rr.Code != http.StatusOK {
		t.Fatalf("v367: operador designado exclui setor do próprio grupo (200), veio %d (%v)", rr.Code, res)
	}
	// e NÃO exclui setor de grupo alheio (restrito ao próprio)
	if rr, _ := doJSONReq(app, "DELETE", "/api/setores/"+idi(sFora), nil, ckOp); rr.Code != http.StatusForbidden {
		t.Fatalf("operador designado não exclui setor de grupo alheio — deve 403, veio %d", rr.Code)
	}

	// gerente NÃO cai no ramo restrito: grupo subordinado continua excluível
	var gidSub int64
	if err := st.db.QueryRow(`INSERT INTO grupos (nome) VALUES ('Grp 367 Sub') RETURNING id`).Scan(&gidSub); err != nil {
		t.Fatalf("criar grupo subordinado: %v", err)
	}
	if _, err := st.db.Exec(`INSERT INTO grupo_vinculos (superior_id, subordinado_id, criado_por_superior, criado_por_subordinado) VALUES (?, ?, 1, 1)`, gid, gidSub); err != nil {
		t.Fatalf("vínculo subordinado: %v", err)
	}
	var sSub int64
	if err := st.db.QueryRow(`INSERT INTO setores (nome, grupo_id) VALUES ('Setor 367 Sub', ?) RETURNING id`, gidSub).Scan(&sSub); err != nil {
		t.Fatalf("criar setor do subordinado: %v", err)
	}
	if rr, res := doJSONReq(app, "DELETE", "/api/setores/"+idi(sSub), nil, ckGer); rr.Code != http.StatusOK {
		t.Fatalf("gerente mantém hierarquia: exclui setor de subordinado (200), veio %d (%v)", rr.Code, res)
	}
}

// TestV367PoderNaoVazaSemDesignacao: matriz fail-closed — operador e chefe_setor
// SEM designação não ganham nada novo; designação em UMA cadeira não abre a outra.
func TestV367PoderNaoVazaSemDesignacao(t *testing.T) {
	app, st, cleanup := setupTestApp(t)
	defer cleanup()
	gid, fPess, fMat, _, _, ckGer := encSetupBase(t, app, st, "nv1")

	criaUsuarioTeste(t, st, "enc_op_nv1", "senha-op", "operador")
	var idOp int64
	_ = st.db.QueryRow(`SELECT id FROM usuarios WHERE login = 'enc_op_nv1'`).Scan(&idOp)
	_, _ = st.db.Exec(`UPDATE usuarios SET grupo_id = ? WHERE id = ?`, gid, idOp)

	ckOp := loginAs(t, app, "enc_op_nv1", "senha-op")
	if rr, _ := doJSONReq(app, "POST", "/api/pessoas", map[string]any{"nome_guerra": "NVSEM", "nome_completo": "Sem Designacao"}, ckOp); rr.Code != http.StatusForbidden {
		t.Fatalf("operador sem designação deve 403 em pessoal, veio %d", rr.Code)
	}
	if rr, _ := doJSONReq(app, "POST", "/api/material/responsaveis", map[string]any{"grupo_id": gid, "setor_id": 0, "encarregado_id": 0, "auxiliar_encarregado_id": 0}, ckOp); rr.Code != http.StatusForbidden {
		t.Fatalf("operador sem designação deve 403 na rota de cargo do material, veio %d", rr.Code)
	}

	// designado SÓ em enc_pessoal: material (rota de cargo) segue 403
	designarViaAPI(t, app, ckGer, fPess, idOp, "titular")
	if rr, _ := doJSONReq(app, "POST", "/api/material/responsaveis", map[string]any{"grupo_id": gid, "setor_id": 0, "encarregado_id": 0, "auxiliar_encarregado_id": 0}, ckOp); rr.Code != http.StatusForbidden {
		t.Fatalf("designado só em enc_pessoal: cargo do material segue 403, veio %d", rr.Code)
	}

	// designado TAMBÉM em enc_material (auxiliar): rota de cargo do material abre
	designarViaAPI(t, app, ckGer, fMat, idOp, "auxiliar")
	if rr, res := doJSONReq(app, "POST", "/api/material/responsaveis", map[string]any{"grupo_id": gid, "setor_id": 0, "encarregado_id": 0, "auxiliar_encarregado_id": 0}, ckOp); rr.Code != http.StatusOK {
		t.Fatalf("v367: designado em enc_material deve 200 na rota de cargo, veio %d (%v)", rr.Code, res)
	}
}
