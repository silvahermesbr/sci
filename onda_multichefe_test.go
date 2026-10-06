package main

// Testes multi-chefia (ordem Diretor 06/10, item 14 — "cada setor tem UM chefe,
// cada USUÁRIO pode chefiar VÁRIOS"):
//  X10. chefe de 2 setores ganha o 3º → mantém os 3 (linha 1:1 por setor).
//  X11. chefe de 2 setores é substituído SÓ no setor 1 → perde só o 1, mantém
//       o papel chefe_setor (acesso de sessão) pelo setor 2.
//  X12. destituir num grupo/comando não derruba o outro setor que comanda
//       (papel permanece enquanto houver comando; sair SEM comando purga o papel).
//  X13. migração v35: backfill do modelo item 1 (papel + usuarios.setor_id) e
//       idempotência (re-executar não duplica).
// Guarda CRAVADA: 'chefe do setor S' = linha em chefe_setores (setor_id UNIQUE).

import (
	"net/http"
	"testing"
)

// multiChefeSetup: grupo + 3 setores + gerente + chefe (multi) — via API real.
func multiChefeSetup(t *testing.T, app *App, st *Store) (gid, s1, s2, s3, uidCh int64, ckGer *http.Cookie) {
	t.Helper()
	if err := st.db.QueryRow(`INSERT INTO grupos (nome) VALUES ('Grp MultiChefe') RETURNING id`).Scan(&gid); err != nil {
		t.Fatalf("criar grupo: %v", err)
	}
	for _, n := range []string{"Setor MC 1", "Setor MC 2", "Setor MC 3"} {
		if _, err := st.db.Exec(`INSERT INTO setores (nome, sigla) VALUES (?, 'MCX')`, n); err != nil {
			t.Fatalf("criar %s: %v", n, err)
		}
	}
	if err := st.db.QueryRow(`SELECT id FROM setores WHERE nome = 'Setor MC 1'`).Scan(&s1); err != nil {
		t.Fatalf("id setor 1: %v", err)
	}
	if err := st.db.QueryRow(`SELECT id FROM setores WHERE nome = 'Setor MC 2'`).Scan(&s2); err != nil {
		t.Fatalf("id setor 2: %v", err)
	}
	if err := st.db.QueryRow(`SELECT id FROM setores WHERE nome = 'Setor MC 3'`).Scan(&s3); err != nil {
		t.Fatalf("id setor 3: %v", err)
	}
	criaUsuarioTeste(t, st, "ger_multi", "senha-gerente", "gerente")
	criaUsuarioTeste(t, st, "ch_multi", "senha-gerente", "chefe_setor")
	criaUsuarioTeste(t, st, "op_multi", "senha-gerente", "operador")
	if _, err := st.db.Exec(`UPDATE usuarios SET nome_guerra = CASE login WHEN 'ch_multi' THEN 'CH MULTI' ELSE 'OP MULTI' END WHERE login IN ('ch_multi','op_multi')`); err != nil {
		t.Fatalf("nome_guerra: %v", err)
	}
	if err := st.db.QueryRow(`SELECT id FROM usuarios WHERE login = 'ch_multi'`).Scan(&uidCh); err != nil {
		t.Fatalf("id chefe: %v", err)
	}
	// chefe e operador pertencem ao grupo novo (nomear_chefe exige do grupo)
	if _, err := st.db.Exec(`UPDATE usuarios SET grupo_id = ? WHERE login IN ('ch_multi','op_multi')`, gid); err != nil {
		t.Fatalf("vincular grupo do chefe/op: %v", err)
	}
	// gerente PRÓPRIO deste setup (não o ger_esc do escalasSetup), logado e
	// vinculado ao grupo novo — X5 fica dono do grupo dele, sem colisão.
	ckGer = vinculaGrupoDoLogin(t, app, st, "ger_multi", gid)
	return
}

func nomearMC(t *testing.T, app *App, ckGer *http.Cookie, gid, uid, sid int64) {
	t.Helper()
	if rr, res := doJSONReq(app, "POST", "/api/grupos/"+i64(gid)+"/nomear_chefe", map[string]any{"usuario_id": uid, "setor_id": sid}, ckGer); rr.Code != http.StatusOK {
		t.Fatalf("nomear chefe setor %d: veio %d %v", sid, rr.Code, res)
	}
}

func comandosDe(t *testing.T, st *Store, uid int64) map[int64]bool {
	t.Helper()
	rows, err := st.db.Query(`SELECT setor_id FROM chefe_setores WHERE usuario_id = ?`, uid)
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

// X10: chefe de 2 setores ganha o 3º → mantém os 3; o comando é 1 linha por
// setor (setor_id UNIQUE) e o papel de sessão permanece único.
func TestX10ChefeDe2SetoresGanha3MantemTodos(t *testing.T) {
	app, st, cleanup := setupTestApp(t)
	defer cleanup()
	gid, s1, s2, s3, uidCh, ckGer := multiChefeSetup(t, app, st)
	_ = gid

	nomearMC(t, app, ckGer, gid, uidCh, s1)
	nomearMC(t, app, ckGer, gid, uidCh, s2)
	nomearMC(t, app, ckGer, gid, uidCh, s3)

	cmd := comandosDe(t, st, uidCh)
	if len(cmd) != 3 || !cmd[s1] || !cmd[s2] || !cmd[s3] {
		t.Fatalf("X10: chefe devia comandar os 3 setores, veio %v", cmd)
	}
	var papel int
	if err := st.db.QueryRow(`SELECT COUNT(*) FROM usuario_papeis WHERE usuario_id = ? AND papel = 'chefe_setor'`, uidCh).Scan(&papel); err != nil || papel != 1 {
		t.Fatalf("X10: papel chefe_setor devia ser único (n=%d err=%v)", papel, err)
	}
}

// X11: chefe dos setores 1 e 2 é substituído SÓ no 1 → perde o comando do 1,
// mantém o do 2 E mantém o papel chefe_setor (acesso de sessão).
func TestX11SubstituicaoNumSetorMantemOutro(t *testing.T) {
	app, st, cleanup := setupTestApp(t)
	defer cleanup()
	gid, s1, s2, _, uidCh, ckGer := multiChefeSetup(t, app, st)
	_ = s2

	var uidOpMulti int64
	if err := st.db.QueryRow(`SELECT id FROM usuarios WHERE login = 'op_multi'`).Scan(&uidOpMulti); err != nil {
		t.Fatalf("id op: %v", err)
	}

	nomearMC(t, app, ckGer, gid, uidCh, s1)
	nomearMC(t, app, ckGer, gid, uidCh, s2)
	// substituição SÓ no setor 1
	nomearMC(t, app, ckGer, gid, uidOpMulti, s1)

	cmd := comandosDe(t, st, uidCh)
	if cmd[s1] {
		t.Fatalf("X11a: chefe antigo NÃO devia mais comandar o setor 1")
	}
	if !cmd[s2] {
		t.Fatalf("X11b: chefe antigo devia CONTINUAR comandando o setor 2")
	}
	var papel int
	if err := st.db.QueryRow(`SELECT COUNT(*) FROM usuario_papeis WHERE usuario_id = ? AND grupo_id = ? AND papel = 'chefe_setor'`, uidCh, gid).Scan(&papel); err != nil || papel != 1 {
		t.Fatalf("X11c: papel chefe_setor devia permanecer (comanda setor 2), n=%d err=%v", papel, err)
	}
	// novo chefe do setor 1: 1 linha 1:1
	if !comandosDe(t, st, uidOpMulti)[s1] {
		t.Fatalf("X11d: novo chefe sem comando do setor 1")
	}
}

// X12: destituir em 1 não derruba o outro — a via real de "perder um setor" é
// a SUBSTITUIÇÃO naquele setor (nomear outro chefe). O chefe dos setores 1 e 2
// perde o 1 por substituição, mantém o 2 e segue logando como chefe_setor;
// só quando perde o ÚLTIMO comando o papel de sessão sai (X12c usa X11 já
// provado p/ substituição; aqui a purga sem comando é provada direto).
func TestX12DestituirUmNaoDerrubaOutro(t *testing.T) {
	app, st, cleanup := setupTestApp(t)
	defer cleanup()
	gid, s1, s2, _, uidCh, ckGer := multiChefeSetup(t, app, st)

	var uidOpMulti int64
	if err := st.db.QueryRow(`SELECT id FROM usuarios WHERE login = 'op_multi'`).Scan(&uidOpMulti); err != nil {
		t.Fatalf("id op: %v", err)
	}

	nomearMC(t, app, ckGer, gid, uidCh, s1)
	nomearMC(t, app, ckGer, gid, uidCh, s2)
	// "destituir em 1" = substituir o chefe SÓ no setor 1
	nomearMC(t, app, ckGer, gid, uidOpMulti, s1)

	cmd := comandosDe(t, st, uidCh)
	if cmd[s1] || !cmd[s2] {
		t.Fatalf("X12a: esperado perder só o setor 1 (comandos: %v)", cmd)
	}
	// papel de sessão permanece (ainda comanda o setor 2)
	var papel int
	if err := st.db.QueryRow(`SELECT COUNT(*) FROM usuario_papeis WHERE usuario_id = ? AND grupo_id = ? AND papel = 'chefe_setor'`, uidCh, gid).Scan(&papel); err != nil || papel != 1 {
		t.Fatalf("X12b: papel devia permanecer (comanda setor 2), n=%d err=%v", papel, err)
	}
	// sessão nova do chefe: papel ativo segue chefe_setor
	tokCh, _, _ := st.CriarSessao(uidCh, ttlSessao)
	ckCh := &http.Cookie{Name: cookieSessao, Value: tokCh}
	if rr, res := doJSONReq(app, "GET", "/api/me", nil, ckCh); rr.Code != http.StatusOK {
		t.Fatalf("X12c: /api/me → 200, veio %d %v", rr.Code, res)
	} else if um, _ := res["usuario"].(map[string]any); um == nil || um["papel"] != "chefe_setor" {
		t.Fatalf("X12d: papel ativo devia continuar chefe_setor: %v", res)
	}
	// e a lista de setores chefiados (guarda do FECHAR SETOR) só tem o setor 2
	if rr, res := doJSONReq(app, "GET", "/api/me", nil, ckCh); rr.Code == http.StatusOK {
		lst, _ := res["setores_chefiados"].([]any)
		if len(lst) != 1 || int(lst[0].(float64)) != int(s2) {
			t.Fatalf("X12e: setores_chefiados devia ser [%d], veio %v", s2, res["setores_chefiados"])
		}
	}
}

// X14: destituir COM setor_id derruba SÓ o comando daquele setor (ordem
// 06/10 item 14: "destituir apaga só o comando do setor S") — o chefe dos
// setores 1 e 2 destituído do 1 mantém o comando do 2 E o papel de sessão.
func TestX14DestituirComSetorDerrubaSoAquele(t *testing.T) {
	app, st, cleanup := setupTestApp(t)
	defer cleanup()
	gid, s1, s2, _, uidCh, ckGer := multiChefeSetup(t, app, st)

	nomearMC(t, app, ckGer, gid, uidCh, s1)
	nomearMC(t, app, ckGer, gid, uidCh, s2)

	// destituição CIRÚRGICA: só o setor 1
	if rr, res := doJSONReq(app, "POST", "/api/grupos/"+i64(gid)+"/destituir_chefe", map[string]any{"usuario_id": uidCh, "setor_id": s1}, ckGer); rr.Code != http.StatusOK {
		t.Fatalf("X14a: destituir c/ setor → 200, veio %d %v", rr.Code, res)
	}
	cmd := comandosDe(t, st, uidCh)
	if cmd[s1] {
		t.Fatalf("X14b: comando do setor 1 devia ter caído")
	}
	if !cmd[s2] {
		t.Fatalf("X14c: comando do setor 2 devia PERMANECER")
	}
	var papel int
	if err := st.db.QueryRow(`SELECT COUNT(*) FROM usuario_papeis WHERE usuario_id = ? AND grupo_id = ? AND papel = 'chefe_setor'`, uidCh, gid).Scan(&papel); err != nil || papel != 1 {
		t.Fatalf("X14d: papel chefe_setor devia permanecer (comanda setor 2), n=%d err=%v", papel, err)
	}
	// destituir de novo o mesmo setor → 404 (não tem mais o comando)
	if rr, _ := doJSONReq(app, "POST", "/api/grupos/"+i64(gid)+"/destituir_chefe", map[string]any{"usuario_id": uidCh, "setor_id": s1}, ckGer); rr.Code != http.StatusNotFound {
		t.Fatalf("X14e: destituir de novo o mesmo setor → 404, veio %d", rr.Code)
	}
	// destituição SEM setor (legado): derruba o ÚLTIMO comando → purga o papel
	if rr, res := doJSONReq(app, "POST", "/api/grupos/"+i64(gid)+"/destituir_chefe", map[string]any{"usuario_id": uidCh}, ckGer); rr.Code != http.StatusOK {
		t.Fatalf("X14f: destituir sem setor → 200, veio %d %v", rr.Code, res)
	}
	if n := len(comandosDe(t, st, uidCh)); n != 0 {
		t.Fatalf("X14g: sem setor, todos os comandos deviam cair (ficaram %d)", n)
	}
	var papel2 int
	if err := st.db.QueryRow(`SELECT COUNT(*) FROM usuario_papeis WHERE usuario_id = ? AND grupo_id = ? AND papel = 'chefe_setor'`, uidCh, gid).Scan(&papel2); err != nil || papel2 != 0 {
		t.Fatalf("X14h: papel chefe_setor devia ser purgado sem NENHUM comando, n=%d err=%v", papel2, err)
	}
}

// X13: migração v35 — backfill do estado do item 1 (papel + usuarios.setor_id)
// vira linha de comando; re-executar é idempotente (1 linha por setor).
func TestX13MigracaoV35Backfill(t *testing.T) {
	app, st, cleanup := setupTestApp(t)
	defer cleanup()
	gid, s1, _, _, uidCh, _ := multiChefeSetup(t, app, st)
	_ = gid

	// estado MODELO DO ITEM 1: papel chefe_setor + usuarios.setor_id = s1,
	// SEM linha em chefe_setores (simula banco da onda anterior).
	if _, err := st.db.Exec(`DELETE FROM chefe_setores`); err != nil {
		t.Fatalf("limpar chefe_setores: %v", err)
	}
	// e SEM o marcador v35: o guard de versão sairia cedo e o backfill não rodaria
	if _, err := st.db.Exec(`DELETE FROM schema_migrations WHERE versao = 35`); err != nil {
		t.Fatalf("limpar marcador v35: %v", err)
	}
	if _, err := st.db.Exec(`INSERT INTO usuario_papeis (usuario_id, grupo_id, papel) VALUES (?, (SELECT grupo_id FROM usuarios WHERE id = ?), 'chefe_setor')`, uidCh, uidCh); err != nil {
		t.Fatalf("papel legado: %v", err)
	}
	if _, err := st.db.Exec(`UPDATE usuarios SET setor_id = ? WHERE id = ?`, s1, uidCh); err != nil {
		t.Fatalf("setor legado: %v", err)
	}

	// re-executa o corpo idempotente da v35 (mesmos passos da migração)
	if err := st.migrarV35(); err != nil {
		t.Fatalf("migrarV35 re-execução: %v", err)
	}
	cmd := comandosDe(t, st, uidCh)
	if len(cmd) != 1 || !cmd[s1] {
		t.Fatalf("X13a: backfill devia criar comando do setor %d, veio %v", s1, cmd)
	}
	// idempotência: re-executar NÃO duplica
	if err := st.migrarV35(); err != nil {
		t.Fatalf("migrarV35 2ª re-execução: %v", err)
	}
	if n := len(comandosDe(t, st, uidCh)); n != 1 {
		t.Fatalf("X13b: re-execução duplicou comandos (%d)", n)
	}
}
