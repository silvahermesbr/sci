package main

// onda_v154_d1_test.go — v1.5.4-D1 (defeito R-12, "chefe-zumbi").
//
// O bug: A é nomeado chefe do setor S; B é nomeado no lugar (ou A é
// destituído) — o COMANDO (chefe_setores) sai, mas o CONTEXTO
// (usuarios.setor_id) de A fica órfão da chefia, e os guardas de
// concluir/reabrir/marcar/leitura confiamavam no contexto com fallbacks
// (u.SetorID → pessoas.setor_id). A seguia exercendo poderes invisíveis.
//
// Correção sob teste (decisão D-2): chefe_setores é a FONTE ÚNICA —
//   • chefeComandaSetor sem fallbacks;
//   • setorAtivoComandado: o contexto da sessão só vale se o chefe AINDA
//     comanda o setor;
//   • hUsuarioPapelAdd (chefe_setor + setor_id) materializa o comando;
//     hUsuarioPapelDel (chefe_setor) apaga os comandos do grupo;
//   • migrarV44 materializa os legados pendentes antes da troca.
//
// Aceite do ROADMAP: nomear A, trocar p/ B → A perde concluir/reabrir/marcar
// do setor IMEDIATAMENTE (mesma sessão), B exerce com contexto certo.
// Padrão persona da casa: setupTestApp + loginAs + doJSONReq, positivo E negativo.

import (
	"net/http"
	"testing"
)

// d1Setup: grupo com 2 setores + gerente + 2 contas (A e B) + 1 militar no
// setor S. A nasce com papel BASE chefe_setor (o chefe real de campo); B nasce
// operador. O COMANDO só existe depois do nomear via API (que materializa).
func d1Setup(t *testing.T, app *App, st *Store) (gid, setorS, uidA, uidB, uidGer, pA int64) {
	t.Helper()
	if err := st.db.QueryRow(`INSERT INTO grupos (nome) VALUES ('Grp D1') RETURNING id`).Scan(&gid); err != nil {
		t.Fatalf("criar grupo: %v", err)
	}
	if err := st.db.QueryRow(`INSERT INTO setores (nome, sigla, grupo_id) VALUES ('Setor D1 S','SD1', ?) RETURNING id`, gid).Scan(&setorS); err != nil {
		t.Fatalf("criar setor S: %v", err)
	}

	criaUsuarioTeste(t, st, "ger_d1", "senha-gerente", "gerente")
	criaUsuarioTeste(t, st, "chefe_d1_a", "senha-gerente", "chefe_setor")
	criaUsuarioTeste(t, st, "chefe_d1_b", "senha-gerente", "operador")
	if _, err := st.db.Exec(`UPDATE usuarios SET grupo_id = ? WHERE login IN ('ger_d1','chefe_d1_a','chefe_d1_b')`, gid); err != nil {
		t.Fatalf("vincular grupo: %v", err)
	}
	if err := st.db.QueryRow(`SELECT id FROM usuarios WHERE login = 'chefe_d1_a'`).Scan(&uidA); err != nil {
		t.Fatalf("id A: %v", err)
	}
	if err := st.db.QueryRow(`SELECT id FROM usuarios WHERE login = 'chefe_d1_b'`).Scan(&uidB); err != nil {
		t.Fatalf("id B: %v", err)
	}
	if err := st.db.QueryRow(`SELECT id FROM usuarios WHERE login = 'ger_d1'`).Scan(&uidGer); err != nil {
		t.Fatalf("id ger: %v", err)
	}
	if err := st.db.QueryRow(`INSERT INTO pessoas (nome_guerra, nome_completo, grupo_id, setor_id, status)
		VALUES ('D1 SILVA','Silva D1 Completo', ?, ?, 'ativo') RETURNING id`, gid, setorS).Scan(&pA); err != nil {
		t.Fatalf("criar militar do setor S: %v", err)
	}
	return
}

func d1IniciaConf(t *testing.T, app *App, ck *http.Cookie) int64 {
	t.Helper()
	rr, res := doJSONReq(app, "POST", "/api/conferencia/iniciar", map[string]any{"nome": "Conf D1"}, ck)
	if rr.Code != http.StatusOK {
		t.Fatalf("iniciar conferência: %d (%v)", rr.Code, res)
	}
	id, _ := res["id"].(float64)
	if id <= 0 {
		t.Fatalf("iniciar sem id de conferência: %v", res)
	}
	return int64(id)
}

func d1Nomear(t *testing.T, app *App, ck *http.Cookie, gid, uid, sid int64) {
	t.Helper()
	rr, res := doJSONReq(app, "POST", "/api/grupos/"+i64(gid)+"/nomear_chefe", map[string]any{"usuario_id": uid, "setor_id": sid}, ck)
	if rr.Code != http.StatusOK {
		t.Fatalf("nomear chefe %d no setor %d: %d (%v)", uid, sid, rr.Code, res)
	}
}

// comandosDoGrupo: setores comandados pelo usuário NO grupo (chefe_setores).
func comandosDoGrupo(t *testing.T, st *Store, uid, gid int64) map[int64]bool {
	t.Helper()
	rows, err := st.db.Query(`SELECT setor_id FROM chefe_setores WHERE usuario_id = ? AND grupo_id = ?`, uid, gid)
	if err != nil {
		t.Fatalf("ler chefe_setores: %v", err)
	}
	defer rows.Close()
	m := map[int64]bool{}
	for rows.Next() {
		var sid int64
		if rows.Scan(&sid) == nil {
			m[sid] = true
		}
	}
	return m
}

// (a) ACEITE DO ROADMAP — substituição 1:1 mata o zumbi na HORA:
// A nomeado via API, exercendo (200); B nomeado no lugar; A na MESMA sessão
// recebe 403 em concluir/reabrir/marcar e perde a leitura do setor;
// B recebe 200 nas mesmas operações com o contexto certo.
func TestD1SubstituicaoMataChefeZumbi(t *testing.T) {
	app, st, cleanup := setupTestApp(t)
	defer cleanup()
	gid, setorS, uidA, uidB, _, pA := d1Setup(t, app, st)

	ckGer := loginAs(t, app, "ger_d1", "senha-gerente")
	confID := d1IniciaConf(t, app, ckGer)
	d1Nomear(t, app, ckGer, gid, uidA, setorS)

	// A loga DEPOIS de nomeado: sessão com papel chefe_setor e contexto S.
	ckA := loginAs(t, app, "chefe_d1_a", "senha-gerente")
	// sanity positivo: chefe legítimo lança e conclui/reabre o próprio setor
	if rr, res := doJSONReq(app, "POST", "/api/conferencia/marcar", map[string]any{"pessoa_id": pA, "situacao": "presente", "verificado": true}, ckA); rr.Code != http.StatusOK {
		t.Fatalf("sanity: chefe A marca no próprio setor: %d (%v)", rr.Code, res)
	}
	if rr, _ := doJSONReq(app, "POST", "/api/conferencia/"+i64(confID)+"/setor/"+i64(setorS)+"/concluir", nil, ckA); rr.Code != http.StatusOK {
		t.Fatalf("sanity: chefe A conclui o próprio setor: %d (%s)", rr.Code, rr.Body.String())
	}
	if rr, _ := doJSONReq(app, "POST", "/api/conferencia/"+i64(confID)+"/setor/"+i64(setorS)+"/reabrir", nil, ckA); rr.Code != http.StatusOK {
		t.Fatalf("sanity: chefe A reabre o próprio setor: %d (%s)", rr.Code, rr.Body.String())
	}

	// SUBSTITUIÇÃO 1:1 — B toma o setor S.
	d1Nomear(t, app, ckGer, gid, uidB, setorS)

	// comando trocou de mãos na fonte única
	if cmd := comandosDoGrupo(t, st, uidA, gid); len(cmd) != 0 {
		t.Fatalf("A substituído ainda tem comando: %v", cmd)
	}
	if cmd := comandosDoGrupo(t, st, uidB, gid); !cmd[setorS] {
		t.Fatalf("B não ficou com o comando do setor %d: %v", setorS, cmd)
	}

	// NEGATIVO — o zumbi (mesma sessão de A, contexto órfão S) perde TUDO já já:
	if rr, res := doJSONReq(app, "POST", "/api/conferencia/marcar", map[string]any{"pessoa_id": pA, "situacao": "presente"}, ckA); rr.Code != http.StatusForbidden {
		t.Fatalf("chefe-zumbi NÃO devia marcar: %d (%v)", rr.Code, res)
	}
	if rr, _ := doJSONReq(app, "POST", "/api/conferencia/"+i64(confID)+"/setor/"+i64(setorS)+"/concluir", nil, ckA); rr.Code != http.StatusForbidden {
		t.Fatalf("chefe-zumbi NÃO devia concluir: %d (%s)", rr.Code, rr.Body.String())
	}
	if rr, _ := doJSONReq(app, "POST", "/api/conferencia/"+i64(confID)+"/setor/"+i64(setorS)+"/reabrir", nil, ckA); rr.Code != http.StatusForbidden {
		t.Fatalf("chefe-zumbi NÃO devia reabrir: %d (%s)", rr.Code, rr.Body.String())
	}

	// POSITIVO — B, nomeado no lugar, exerce com o contexto certo:
	ckB := loginAs(t, app, "chefe_d1_b", "senha-gerente")
	if rr, res := doJSONReq(app, "POST", "/api/conferencia/marcar", map[string]any{"pessoa_id": pA, "situacao": "presente", "verificado": true}, ckB); rr.Code != http.StatusOK {
		t.Fatalf("novo chefe B devia marcar: %d (%v)", rr.Code, res)
	}
	if rr, _ := doJSONReq(app, "POST", "/api/conferencia/"+i64(confID)+"/setor/"+i64(setorS)+"/concluir", nil, ckB); rr.Code != http.StatusOK {
		t.Fatalf("novo chefe B devia concluir: %d (%s)", rr.Code, rr.Body.String())
	}
	if rr, _ := doJSONReq(app, "POST", "/api/conferencia/"+i64(confID)+"/setor/"+i64(setorS)+"/reabrir", nil, ckB); rr.Code != http.StatusOK {
		t.Fatalf("novo chefe B devia reabrir: %d (%s)", rr.Code, rr.Body.String())
	}
}

// (b) hUsuarioPapelAdd (chefe_setor + setor_id) MATERIALIZA o comando em
// chefe_setores (o chefe anterior do setor perde a linha); hUsuarioPapelDel do
// papel chefe_setor apaga os comandos do grupo.
func TestD1PapelAddDelMaterializaComando(t *testing.T) {
	app, st, cleanup := setupTestApp(t)
	defer cleanup()
	gid, setorS, uidA, uidB, _, _ := d1Setup(t, app, st)
	ckGer := loginAs(t, app, "ger_d1", "senha-gerente")

	// ADD: chefe_setor COM setor_id → comando materializado + setor na conta
	rr, res := doJSONReq(app, "POST", "/api/usuarios/"+i64(uidB)+"/papeis",
		map[string]any{"papel": "chefe_setor", "grupo_id": gid, "setor_id": setorS}, ckGer)
	if rr.Code != http.StatusOK {
		t.Fatalf("papel add chefe_setor+setor: %d (%v)", rr.Code, res)
	}
	if cmd := comandosDoGrupo(t, st, uidB, gid); !cmd[setorS] {
		t.Fatalf("ADD não materializou comando em chefe_setores: %v", cmd)
	}
	var setorConta *int64
	if err := st.db.QueryRow(`SELECT setor_id FROM usuarios WHERE id = ?`, uidB).Scan(&setorConta); err != nil || setorConta == nil || *setorConta != setorS {
		t.Fatalf("ADD devia vincular usuarios.setor_id=%d (veio %v, err=%v)", setorS, setorConta, err)
	}

	// troca de setor via PAPEIS: A ganha o MESMO setor → substituição 1:1 na
	// fonte única (chefe anterior perde a linha), igual ao nomear_chefe
	rr, res = doJSONReq(app, "POST", "/api/usuarios/"+i64(uidA)+"/papeis",
		map[string]any{"papel": "chefe_setor", "grupo_id": gid, "setor_id": setorS}, ckGer)
	if rr.Code != http.StatusOK {
		t.Fatalf("papel add (troca p/ A): %d (%v)", rr.Code, res)
	}
	if cmd := comandosDoGrupo(t, st, uidA, gid); !cmd[setorS] {
		t.Fatalf("A devia ficar com o comando do setor %d: %v", setorS, cmd)
	}
	if cmd := comandosDoGrupo(t, st, uidB, gid); len(cmd) != 0 {
		t.Fatalf("chefe anterior (B) devia PERDER a linha do setor: %v", cmd)
	}

	// DEL: remover o papel chefe_setor apaga os COMANDOS do grupo.
	// (o endpoint recusa remover o ÚNICO papel → A ganha um operador antes)
	if rr, res = doJSONReq(app, "POST", "/api/usuarios/"+i64(uidA)+"/papeis",
		map[string]any{"papel": "operador", "grupo_id": gid}, ckGer); rr.Code != http.StatusOK {
		t.Fatalf("papel add operador (para poder remover o chefe): %d (%v)", rr.Code, res)
	}
	var papelChefeID int64
	if err := st.db.QueryRow(`SELECT id FROM usuario_papeis WHERE usuario_id = ? AND grupo_id = ? AND papel = 'chefe_setor'`, uidA, gid).Scan(&papelChefeID); err != nil {
		t.Fatalf("papel chefe_setor de A: %v", err)
	}
	if rr, res = doJSONReq(app, "DELETE", "/api/usuarios/"+i64(uidA)+"/papeis/"+i64(papelChefeID), nil, ckGer); rr.Code != http.StatusOK {
		t.Fatalf("papel del chefe_setor: %d (%v)", rr.Code, res)
	}
	if cmd := comandosDoGrupo(t, st, uidA, gid); len(cmd) != 0 {
		t.Fatalf("DEL devia apagar os comandos do grupo: %v", cmd)
	}
	var nPapel int
	if err := st.db.QueryRow(`SELECT COUNT(*) FROM usuario_papeis WHERE id = ?`, papelChefeID).Scan(&nPapel); err != nil || nPapel != 0 {
		t.Fatalf("papel chefe_setor devia ter saído (n=%d err=%v)", nPapel, err)
	}
}

// (c) Migração v44: legados pendentes (papel chefe_setor + setor na conta ou
// só na pessoa) viram comando em chefe_setores; setor de OUTRO grupo, setor
// INATIVO e setor já ocupado NÃO mudam de dono; idempotente.
func TestD1MigracaoV44MaterializaLegado(t *testing.T) {
	_, st, cleanup := setupTestApp(t)
	defer cleanup()

	var gid1, gid2, sA, sB, sC, sD int64
	if err := st.db.QueryRow(`INSERT INTO grupos (nome) VALUES ('Grp D1 M1') RETURNING id`).Scan(&gid1); err != nil {
		t.Fatalf("grupo 1: %v", err)
	}
	if err := st.db.QueryRow(`INSERT INTO grupos (nome) VALUES ('Grp D1 M2') RETURNING id`).Scan(&gid2); err != nil {
		t.Fatalf("grupo 2: %v", err)
	}
	criaSetor := func(nome string, gid int64) int64 {
		t.Helper()
		var sid int64
		if err := st.db.QueryRow(`INSERT INTO setores (nome, sigla, grupo_id) VALUES (?, 'D1M', ?) RETURNING id`, nome, gid).Scan(&sid); err != nil {
			t.Fatalf("criar %s: %v", nome, err)
		}
		return sid
	}
	sA = criaSetor("D1M A", gid1)
	sB = criaSetor("D1M B", gid1)
	sC = criaSetor("D1M C", gid2) // setor de OUTRO grupo
	sD = criaSetor("D1M D", gid1)

	criaLegado := func(login string, gid int64, setor *int64, pessoaID *int64) int64 {
		t.Helper()
		var uid int64
		if err := st.db.QueryRow(`INSERT INTO usuarios (login, senha_hash, papel, grupo_id, setor_id, pessoa_id, precisa_setup)
			VALUES (?, 'hash-d1', 'chefe_setor', ?, ?, ?, 0) RETURNING id`, login, gid, setor, pessoaID).Scan(&uid); err != nil {
			t.Fatalf("criar %s: %v", login, err)
		}
		if _, err := st.db.Exec(`INSERT INTO usuario_papeis (usuario_id, grupo_id, papel) VALUES (?, ?, 'chefe_setor')`, uid, gid); err != nil {
			t.Fatalf("papel legado %s: %v", login, err)
		}
		return uid
	}
	u1 := criaLegado("d1m_u1", gid1, &sA, nil)                 // fonte A: usuarios.setor_id
	var p2 int64
	if err := st.db.QueryRow(`INSERT INTO pessoas (nome_guerra, nome_completo, grupo_id, setor_id, status)
		VALUES ('D1M P2','P2 Completo', ?, ?, 'ativo') RETURNING id`, gid1, sB).Scan(&p2); err != nil {
		t.Fatalf("pessoa p2: %v", err)
	}
	u2 := criaLegado("d1m_u2", gid1, nil, &p2)  // fonte B: só pessoas.setor_id
	u3 := criaLegado("d1m_u3", gid1, &sC, nil) // conta do grupo 1 com setor do grupo 2 → NÃO materializa
	var u4 int64
	if err := st.db.QueryRow(`INSERT INTO usuarios (login, senha_hash, papel, grupo_id, setor_id, precisa_setup)
		VALUES ('d1m_u4', 'hash-d1', 'chefe_setor', ?, ?, 0) RETURNING id`, gid1, sD).Scan(&u4); err != nil {
		t.Fatalf("criar d1m_u4: %v", err)
	}
	// u4 NÃO ganha papel legado (sem papel, a migração não cria comando).
	// setor D já OCUPADO pelo comando vigente → OR IGNORE preserva o vigente.
	var uVigente int64
	if err := st.db.QueryRow(`INSERT INTO usuarios (login, senha_hash, papel, grupo_id, precisa_setup)
		VALUES ('d1m_vigente', 'hash-d1', 'operador', ?, 0) RETURNING id`, gid1).Scan(&uVigente); err != nil {
		t.Fatalf("criar vigente: %v", err)
	}
	materializaComandoSetor(t, st, uVigente, gid1, sD)

	// estado-legado pronto; força a re-execução da v44 (só o marcador sai —
	// as tabelas já existem; padrão do TestX13MigracaoV35Backfill)
	if _, err := st.db.Exec(`DELETE FROM schema_migrations WHERE versao = 44`); err != nil {
		t.Fatalf("limpar marcador v44: %v", err)
	}
	if err := st.migrarV44(); err != nil {
		t.Fatalf("migrarV44: %v", err)
	}

	if cmd := comandosDoGrupo(t, st, u1, gid1); !cmd[sA] || len(cmd) != 1 {
		t.Fatalf("v44: legado usuarios.setor_id devia virar comando do setor %d, veio %v", sA, cmd)
	}
	if cmd := comandosDoGrupo(t, st, u2, gid1); !cmd[sB] || len(cmd) != 1 {
		t.Fatalf("v44: legado só em pessoas.setor_id devia virar comando do setor %d, veio %v", sB, cmd)
	}
	if cmd := comandosDoGrupo(t, st, u3, gid1); len(cmd) != 0 {
		t.Fatalf("v44: setor de OUTRO grupo NÃO devia materializar: %v", cmd)
	}
	if cmd := comandosDoGrupo(t, st, u4, gid1); len(cmd) != 0 {
		t.Fatalf("v44: conta SEM papel legado não ganha comando: %v", cmd)
	}
	// UNIQUE(setor_id): o comando VIGENTE de sD permanece, sem duplicar
	if cmd := comandosDoGrupo(t, st, uVigente, gid1); !cmd[sD] {
		t.Fatalf("v44: setor ocupado devia PRESERVAR o comando vigente %d", sD)
	}
	var nD int
	if err := st.db.QueryRow(`SELECT COUNT(*) FROM chefe_setores WHERE setor_id = ?`, sD).Scan(&nD); err != nil || nD != 1 {
		t.Fatalf("v44: setor ocupado devia ter EXATAMENTE 1 comando (n=%d err=%v)", nD, err)
	}

	// idempotência: re-executar (marker removido de novo) não duplica nem troca dono
	if _, err := st.db.Exec(`DELETE FROM schema_migrations WHERE versao = 44`); err != nil {
		t.Fatalf("limpar marcador v44 (2ª): %v", err)
	}
	if err := st.migrarV44(); err != nil {
		t.Fatalf("migrarV44 re-execução: %v", err)
	}
	if cmd := comandosDoGrupo(t, st, u1, gid1); len(cmd) != 1 || !cmd[sA] {
		t.Fatalf("v44 idempotência: comando de u1 mudou: %v", cmd)
	}
	if n := len(comandosDoGrupo(t, st, uVigente, gid1)); n != 1 {
		t.Fatalf("v44 idempotência: comandos do vigente duplicaram (%d)", n)
	}
	var v44 int
	if err := st.db.QueryRow(`SELECT COUNT(*) FROM schema_migrations WHERE versao = 44`).Scan(&v44); err != nil || v44 != 1 {
		t.Fatalf("v44 devia ficar marcada (n=%d err=%v)", v44, err)
	}
}

// (d) Contexto órfão pela DESTITUIÇÃO: gerente destitui A; usuarios.setor_id
// de A FICA apontando para o setor (documentado — decisão D-5 pendente), a
// sessão continua "olhando" chefe (papel base), e NEM a leitura do setor
// (hConferenciaHoje: pessoas vazias) NEM o marcar (403) passam.
func TestD1ContextoOrfaoPerdeLeituraEMarcar(t *testing.T) {
	app, st, cleanup := setupTestApp(t)
	defer cleanup()
	gid, setorS, uidA, _, _, pA := d1Setup(t, app, st)

	ckGer := loginAs(t, app, "ger_d1", "senha-gerente")
	d1IniciaConf(t, app, ckGer)
	d1Nomear(t, app, ckGer, gid, uidA, setorS)

	ckA := loginAs(t, app, "chefe_d1_a", "senha-gerente")
	if rr, res := doJSONReq(app, "POST", "/api/conferencia/marcar", map[string]any{"pessoa_id": pA, "situacao": "presente"}, ckA); rr.Code != http.StatusOK {
		t.Fatalf("sanity: chefe A marca no próprio setor: %d (%v)", rr.Code, res)
	}

	// DESTITUIÇÃO — sai o comando; o contexto (usuarios.setor_id) fica órfão
	rr, res := doJSONReq(app, "POST", "/api/grupos/"+i64(gid)+"/destituir_chefe", map[string]any{"usuario_id": uidA}, ckGer)
	if rr.Code != http.StatusOK {
		t.Fatalf("destituir chefe: %d (%v)", rr.Code, res)
	}
	var setorOrfao *int64
	if err := st.db.QueryRow(`SELECT setor_id FROM usuarios WHERE id = ?`, uidA).Scan(&setorOrfao); err != nil || setorOrfao == nil || *setorOrfao != setorS {
		t.Fatalf("cenário: usuarios.setor_id devia ficar órfão apontando pro setor %d (veio %v err=%v)", setorS, setorOrfao, err)
	}
	if cmd := comandosDoGrupo(t, st, uidA, gid); len(cmd) != 0 {
		t.Fatalf("cenário: destituído não devia ter comando: %v", cmd)
	}

	// LEITURA: sem comando, o contexto órfão não mostra mais o efetivo do setor
	rrHoje, resHoje := doJSONReq(app, "GET", "/api/conferencia/hoje", nil, ckA)
	if rrHoje.Code != http.StatusOK {
		t.Fatalf("GET conferencia/hoje do destituído: %d (%v)", rrHoje.Code, resHoje)
	}
	pessoas, _ := resHoje["pessoas"].([]any)
	if len(pessoas) != 0 {
		t.Fatalf("contexto órfão NÃO devia ler o efetivo do setor (pessoas=%v)", pessoas)
	}

	// ESCRITA: marcar militar do setor órfão → 403
	if rr, res := doJSONReq(app, "POST", "/api/conferencia/marcar", map[string]any{"pessoa_id": pA, "situacao": "falta"}, ckA); rr.Code != http.StatusForbidden {
		t.Fatalf("contexto órfão NÃO devia marcar: %d (%v)", rr.Code, res)
	}
}
