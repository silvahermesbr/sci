package main

// onda_v154_e1_test.go — v1.5.4-E1: regressão por persona (positivo E
// negativo) dos 4 defeitos da bula E:
//
//	R-9  — contexto de sessão: setor fora do grupo do papel ativo / fora do
//	       comando do chefe → 403; setor comandado → 200 e usuarios.setor_id
//	       NÃO é reescrito (o contexto vive em sessoes.setor_ativo_id).
//	R-10 — gestão de papéis: Del fail-open extinto (gerente de outro grupo e
//	       chefe → 403, alvo intacto); allowlist cobre Add (operador puro
//	       barrado; enc de pessoal passa no próprio grupo).
//	R-14 — envio de mensagens: conta sem grupo não envia (403); papel do
//	       próprio grupo e de subordinado → 200; grupo não-relacionado → 403.
//	R-16 — senha invalida sessões (outras morrem; a corrente da própria troca
//	       fica; redefinição por outrem derruba tudo) e admin semeado com a
//	       senha-PADRÃO nasce com troca obrigatória (precisa_setup).

import (
	"fmt"
	"net/http"
	"os"
	"strings"
	"testing"
)

// ---------- helpers da onda (só aqui — helpers_test.go é intocável) ----------

// e1CriaGrupo: grupo cru via SQL (a bula é de guarda; a persona não depende
// do CRUD de grupos).
func e1CriaGrupo(t *testing.T, st *Store, nome string) int64 {
	t.Helper()
	var gid int64
	if err := st.db.QueryRow(`INSERT INTO grupos (nome) VALUES (?) RETURNING id`, nome).Scan(&gid); err != nil {
		t.Fatalf("criar grupo %s: %v", nome, err)
	}
	return gid
}

// e1VinculaConta: doutrina R-14 (conta sem grupo não envia) — vincula a conta
// e o papel da sessão ao grupo, PRESERVANDO o id da linha (UPDATE in-place,
// padrão do ctxDrive): a sessão continua apontando para o mesmo papel com
// grupo. Idempotente; funciona antes ou depois do login.
func e1VinculaConta(t *testing.T, st *Store, login string, gid int64) {
	t.Helper()
	if _, err := st.db.Exec(`UPDATE usuarios SET grupo_id = ? WHERE login = ?`, gid, login); err != nil {
		t.Fatalf("grupo do usuário %s: %v", login, err)
	}
	var uid int64
	if err := st.db.QueryRow(`SELECT id FROM usuarios WHERE login = ?`, login).Scan(&uid); err != nil {
		t.Fatalf("usuário %s: %v", login, err)
	}
	if _, err := st.db.Exec(`UPDATE usuario_papeis SET grupo_id = ? WHERE usuario_id = ? AND grupo_id IS NULL`, gid, uid); err != nil {
		t.Fatalf("papel de %s no grupo: %v", login, err)
	}
}

// e1LoginVinculado: vincula ao grupo e loga com a senha informada.
func e1LoginVinculado(t *testing.T, app *App, st *Store, login string, gid int64, senha string) *http.Cookie {
	t.Helper()
	e1VinculaConta(t, st, login, gid)
	return loginAs(t, app, login, senha)
}

// e1UsuarioID: id da CONTA (usuarios.id) pelo login.
func e1UsuarioID(t *testing.T, st *Store, login string) int64 {
	t.Helper()
	var uid int64
	if err := st.db.QueryRow(`SELECT id FROM usuarios WHERE login = ?`, login).Scan(&uid); err != nil {
		t.Fatalf("usuário %s: %v", login, err)
	}
	return uid
}

// e1PapelID: id da linha usuario_papeis da conta (por papel, opcional).
func e1PapelID(t *testing.T, st *Store, login, papel string) int64 {
	t.Helper()
	var pid int64
	q := `SELECT up.id FROM usuario_papeis up JOIN usuarios u ON u.id = up.usuario_id WHERE u.login = ?`
	args := []any{login}
	if papel != "" {
		q += ` AND up.papel = ?`
		args = append(args, papel)
	}
	if err := st.db.QueryRow(q, args...).Scan(&pid); err != nil {
		t.Fatalf("papel de %s (%s): %v", login, papel, err)
	}
	return pid
}

// e1PapelExiste: a linha de papel ainda existe? (falso = removida).
func e1PapelExiste(t *testing.T, st *Store, pid int64) bool {
	t.Helper()
	var n int
	if err := st.db.QueryRow(`SELECT COUNT(*) FROM usuario_papeis WHERE id = ?`, pid).Scan(&n); err != nil {
		t.Fatalf("papel %d: %v", pid, err)
	}
	return n > 0
}

// e1SetupComSenha: app com seed de senha informada (R-16b compara a
// senha-PADRÃO 'admin' com senhas próprias no 1º boot).
func e1SetupComSenha(t *testing.T, senhaSeed string) (*App, *Store, func()) {
	t.Helper()
	tmpDir, err := os.MkdirTemp("", "sci_e1_*")
	if err != nil {
		t.Fatalf("tmpDir: %v", err)
	}
	st, err := AbrirStore(tmpDir)
	if err != nil {
		os.RemoveAll(tmpDir)
		t.Fatalf("store: %v", err)
	}
	if err := st.SeedIfEmpty(senhaSeed); err != nil {
		st.Close()
		os.RemoveAll(tmpDir)
		t.Fatalf("seed: %v", err)
	}
	app := NovaApp(st)
	return app, st, func() {
		st.Close()
		os.RemoveAll(tmpDir)
	}
}

// ---------- R-9 — contexto no grupo do papel ativo ----------

func TestE1R9ContextoNoGrupoDoPapel(t *testing.T) {
	app, st, cleanup := setupTestApp(t)
	defer cleanup()

	gidA := e1CriaGrupo(t, st, "Grp E1 A")
	gidB := e1CriaGrupo(t, st, "Grp E1 B")
	var sa1, sa2, sb1 int64
	if err := st.db.QueryRow(`INSERT INTO setores (grupo_id, nome, ativo) VALUES (?, 'Setor A1', 1) RETURNING id`, gidA).Scan(&sa1); err != nil {
		t.Fatalf("setor A1: %v", err)
	}
	if err := st.db.QueryRow(`INSERT INTO setores (grupo_id, nome, ativo) VALUES (?, 'Setor A2', 1) RETURNING id`, gidA).Scan(&sa2); err != nil {
		t.Fatalf("setor A2: %v", err)
	}
	if err := st.db.QueryRow(`INSERT INTO setores (grupo_id, nome, ativo) VALUES (?, 'Setor B1', 1) RETURNING id`, gidB).Scan(&sb1); err != nil {
		t.Fatalf("setor B1: %v", err)
	}

	// chefe comanda SA1 no grupo A; cadastro (usuarios.setor_id) aponta SA2 —
	// NADA de chefe_setores para SA2 (era o chefe-zumbi da D1).
	criaUsuarioTeste(t, st, "chefe_e1", "senha-gerente", "chefe_setor")
	e1VinculaConta(t, st, "chefe_e1", gidA)
	var uid int64
	if err := st.db.QueryRow(`SELECT id FROM usuarios WHERE login='chefe_e1'`).Scan(&uid); err != nil {
		t.Fatalf("chefe: %v", err)
	}
	if _, err := st.db.Exec(`INSERT INTO chefe_setores (grupo_id, setor_id, usuario_id) VALUES (?,?,?)`, gidA, sa1, uid); err != nil {
		t.Fatalf("comando SA1: %v", err)
	}
	if _, err := st.db.Exec(`UPDATE usuarios SET setor_id = ? WHERE id = ?`, sa2, uid); err != nil {
		t.Fatalf("cadastro SA2: %v", err)
	}
	ck := loginAs(t, app, "chefe_e1", "senha-gerente")
	pidChefe := e1PapelID(t, st, "chefe_e1", "chefe_setor")

	// 1) setor de OUTRO grupo → 403
	rr, res := doJSONReq(app, "POST", "/api/sessao/contexto", map[string]any{"papel_id": pidChefe, "setor_id": sb1}, ck)
	if rr.Code != http.StatusForbidden {
		t.Fatalf("R-9: setor de outro grupo deveria 403, veio %d (%v)", rr.Code, res)
	}

	// 2) setor do próprio grupo que NÃO comanda → 403
	rr, res = doJSONReq(app, "POST", "/api/sessao/contexto", map[string]any{"papel_id": pidChefe, "setor_id": sa2}, ck)
	if rr.Code != http.StatusForbidden {
		t.Fatalf("R-9: setor não comandado deveria 403, veio %d (%v)", rr.Code, res)
	}

	// 3) setor que comanda → 200; usuarios.setor_id NÃO é reescrito e o
	// contexto segue na SESSÃO (/api/me devolve o setor ativo).
	rr, res = doJSONReq(app, "POST", "/api/sessao/contexto", map[string]any{"papel_id": pidChefe, "setor_id": sa1}, ck)
	if rr.Code != http.StatusOK {
		t.Fatalf("R-9: setor comandado deveria 200, veio %d (%v)", rr.Code, res)
	}
	var cadastro *int64
	if err := st.db.QueryRow(`SELECT setor_id FROM usuarios WHERE id = ?`, uid).Scan(&cadastro); err != nil {
		t.Fatalf("ler cadastro: %v", err)
	}
	if cadastro == nil || *cadastro != sa2 {
		t.Fatalf("R-9: usuarios.setor_id NÃO devia ser reescrito pelo contexto (quer %d, veio %v)", sa2, cadastro)
	}
	_, resMe := doJSONReq(app, "GET", "/api/me", nil, ck)
	uMe := resMe["usuario"].(map[string]any)
	if int64(uMe["setor_id"].(float64)) != sa1 {
		t.Fatalf("R-9: contexto da sessão deveria ser o setor comandado %d, veio %v", sa1, uMe["setor_id"])
	}

	// 4) controle: admin segue livre (setor de qualquer grupo → 200)
	var pidAdmin int64
	if err := st.db.QueryRow(`SELECT id FROM usuario_papeis WHERE papel='admin' LIMIT 1`).Scan(&pidAdmin); err != nil {
		t.Fatalf("papel admin: %v", err)
	}
	ckAdmin := loginAs(t, app, "admin", "admin123")
	rr, res = doJSONReq(app, "POST", "/api/sessao/contexto", map[string]any{"papel_id": pidAdmin, "setor_id": sb1}, ckAdmin)
	if rr.Code != http.StatusOK {
		t.Fatalf("R-9: admin é global e deveria assumir qualquer setor, veio %d (%v)", rr.Code, res)
	}
}

// ---------- R-10 — allowlist de solicitantes em papéis ----------

func TestE1R10PapelDelSemFailOpen(t *testing.T) {
	app, st, cleanup := setupTestApp(t)
	defer cleanup()

	gidA := e1CriaGrupo(t, st, "Grp E1 Del A")
	gidB := e1CriaGrupo(t, st, "Grp E1 Del B")

	criaUsuarioTeste(t, st, "ger_del_a", "senha-gerente", "gerente")
	ckGerA := e1LoginVinculado(t, app, st, "ger_del_a", gidA, "senha-gerente")
	criaUsuarioTeste(t, st, "ger_del_b", "senha-gerente", "gerente")
	ckGerB := e1LoginVinculado(t, app, st, "ger_del_b", gidB, "senha-gerente")
	criaUsuarioTeste(t, st, "chefe_del", "senha-gerente", "chefe_setor")
	ckChefe := e1LoginVinculado(t, app, st, "chefe_del", gidA, "senha-gerente")

	// alvo com DOIS papéis no grupo A (a trava do único papel segue viva)
	criaUsuarioTeste(t, st, "alvo_del", "senha-gerente", "operador")
	e1VinculaConta(t, st, "alvo_del", gidA)
	loginAs(t, app, "alvo_del", "senha-gerente") // sintetiza o papel operador@A
	uidAlvo := e1UsuarioID(t, st, "alvo_del")
	_, res := doJSONReq(app, "POST", fmt.Sprintf("/api/usuarios/%d/papeis", uidAlvo),
		map[string]any{"papel": "chefe_setor", "grupo_id": gidA}, ckGerA)
	if res["ok"] != true {
		t.Fatalf("setup: atribuir chefe_setor ao alvo falhou: %v", res)
	}
	pidChefeAlvo := e1PapelID(t, st, "alvo_del", "chefe_setor")
	pidOpAlvo := e1PapelID(t, st, "alvo_del", "operador")

	// 1) gerente do PRÓPRIO grupo remove papel → 200
	rr, res := doJSONReq(app, "DELETE", fmt.Sprintf("/api/usuarios/%d/papeis/%d", uidAlvo, pidChefeAlvo), nil, ckGerA)
	if rr.Code != http.StatusOK {
		t.Fatalf("R-10: gerente do próprio grupo deveria remover (200), veio %d (%v)", rr.Code, res)
	}

	// 2) gerente de OUTRO grupo (sem vínculo) → 403 e o papel fica intacto
	rr, res = doJSONReq(app, "DELETE", fmt.Sprintf("/api/usuarios/%d/papeis/%d", uidAlvo, pidOpAlvo), nil, ckGerB)
	if rr.Code != http.StatusForbidden {
		t.Fatalf("R-10: gerente de outro grupo deveria 403, veio %d (%v)", rr.Code, res)
	}
	if !e1PapelExiste(t, st, pidOpAlvo) {
		t.Fatalf("R-10: papel do alvo NÃO devia sair com solicitante fora do escopo")
	}

	// 3) chefe tentando remover papel → 403 (era o fail-open: passava e
	// re-chaveava as sessões do alvo)
	rr, res = doJSONReq(app, "DELETE", fmt.Sprintf("/api/usuarios/%d/papeis/%d", uidAlvo, pidOpAlvo), nil, ckChefe)
	if rr.Code != http.StatusForbidden {
		t.Fatalf("R-10: chefe deveria 403 no papel-del, veio %d (%v)", rr.Code, res)
	}
	if !e1PapelExiste(t, st, pidOpAlvo) {
		t.Fatalf("R-10: papel do alvo não sobreviveu à tentativa ilegítima")
	}

	// 4) trava do único papel segue viva: alvo agora só tem o operador
	rr, res = doJSONReq(app, "DELETE", fmt.Sprintf("/api/usuarios/%d/papeis/%d", uidAlvo, pidOpAlvo), nil, ckGerA)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("R-10: remover o ÚNICO papel deveria seguir 400, veio %d (%v)", rr.Code, res)
	}
}

// TestE1R10PapelAddFailClosed: a mesma allowlist no Add — operador puro e
// chefe sem designação não gerem papéis; enc de pessoal gere no próprio
// grupo; gerente fora da árvore não atribui.
func TestE1R10PapelAddFailClosed(t *testing.T) {
	app, st, cleanup := setupTestApp(t)
	defer cleanup()

	gidA := e1CriaGrupo(t, st, "Grp E1 Add A")
	gidC := e1CriaGrupo(t, st, "Grp E1 Add C")

	criaUsuarioTeste(t, st, "ger_add", "senha-gerente", "gerente")
	ckGer := e1LoginVinculado(t, app, st, "ger_add", gidA, "senha-gerente")
	criaUsuarioTeste(t, st, "op_add", "senha-gerente", "operador")
	ckOp := e1LoginVinculado(t, app, st, "op_add", gidA, "senha-gerente")

	// enc de pessoal (base operador — o cargo manda, v367)
	criaUsuarioTeste(t, st, "enc_add", "senha-gerente", "operador")
	e1VinculaConta(t, st, "enc_add", gidA)
	var fid int64
	if err := st.db.QueryRow(`SELECT id FROM funcoes WHERE chave = 'enc_pessoal' AND ativo = 1
		AND (grupo_id = ? OR grupo_id IS NULL) ORDER BY (grupo_id IS NULL) ASC LIMIT 1`, gidA).Scan(&fid); err != nil {
		if err := st.db.QueryRow(`INSERT INTO funcoes (nome, grupo_id, tipo, chave)
			VALUES ('Encarregado E1', ?, 'grupo', 'enc_pessoal') RETURNING id`, gidA).Scan(&fid); err != nil {
			t.Fatalf("função enc: %v", err)
		}
	}
	var uidEnc int64
	if err := st.db.QueryRow(`SELECT id FROM usuarios WHERE login='enc_add'`).Scan(&uidEnc); err != nil {
		t.Fatalf("enc: %v", err)
	}
	if _, err := st.db.Exec(`INSERT INTO funcao_membros (funcao_id, grupo_id, usuario_id, titularidade) VALUES (?,?,?,'titular')`, fid, gidA, uidEnc); err != nil {
		t.Fatalf("designar enc: %v", err)
	}
	// v1.6.0 Fase 2: o enc gere papéis pelo CONTEXTO 'enc_pessoal' — a
	// designação semeda é MATERIALIZADA pela v45; com a única linha da conta
	// sendo a da cadeira, o login do enc_add resolve o contexto enc.
	v45Reexecuta(t, st)
	ckEnc := loginAs(t, app, "enc_add", "senha-gerente")

	criaUsuarioTeste(t, st, "alvo_add", "senha-gerente", "operador")
	e1VinculaConta(t, st, "alvo_add", gidA)
	uidAlvo := e1UsuarioID(t, st, "alvo_add")

	// operador puro (fail-open antigo caía fora do ramo gerente) → 403
	rr, res := doJSONReq(app, "POST", fmt.Sprintf("/api/usuarios/%d/papeis", uidAlvo),
		map[string]any{"papel": "operador", "grupo_id": gidA}, ckOp)
	if rr.Code != http.StatusForbidden {
		t.Fatalf("R-10 Add: operador deveria 403, veio %d (%v)", rr.Code, res)
	}

	// gerente no próprio grupo → 200
	rr, res = doJSONReq(app, "POST", fmt.Sprintf("/api/usuarios/%d/papeis", uidAlvo),
		map[string]any{"papel": "operador", "grupo_id": gidA}, ckGer)
	if rr.Code != http.StatusOK {
		t.Fatalf("R-10 Add: gerente no próprio grupo deveria 200, veio %d (%v)", rr.Code, res)
	}

	// gerente atribui em grupo SEM relação → 403
	rr, res = doJSONReq(app, "POST", fmt.Sprintf("/api/usuarios/%d/papeis", uidAlvo),
		map[string]any{"papel": "operador", "grupo_id": gidC}, ckGer)
	if rr.Code != http.StatusForbidden {
		t.Fatalf("R-10 Add: gerente em grupo não-relacionado deveria 403, veio %d (%v)", rr.Code, res)
	}

	// enc de pessoal no próprio grupo → 200 (doutrina hUsuariosAdd)
	rr, res = doJSONReq(app, "POST", fmt.Sprintf("/api/usuarios/%d/papeis", uidAlvo),
		map[string]any{"papel": "chefe_setor", "grupo_id": gidA}, ckEnc)
	if rr.Code != http.StatusOK {
		t.Fatalf("R-10 Add: enc de pessoal no próprio grupo deveria 200, veio %d (%v)", rr.Code, res)
	}
}

// ---------- R-14 — hierarquia de envio ----------

func TestE1R14EscopoDeEnvio(t *testing.T) {
	app, st, cleanup := setupTestApp(t)
	defer cleanup()

	gidA := e1CriaGrupo(t, st, "Grp E1 Msg A")
	gidB := e1CriaGrupo(t, st, "Grp E1 Msg B")
	gidC := e1CriaGrupo(t, st, "Grp E1 Msg C")
	if _, err := st.db.Exec(`INSERT INTO grupo_vinculos (superior_id, subordinado_id, criado_por_superior, criado_por_subordinado) VALUES (?, ?, 1, 1)`, gidA, gidB); err != nil {
		t.Fatalf("vínculo A→B: %v", err)
	}

	criaUsuarioTeste(t, st, "ger_msg_a", "senha-gerente", "gerente")
	ckGerA := e1LoginVinculado(t, app, st, "ger_msg_a", gidA, "senha-gerente")
	criaUsuarioTeste(t, st, "ger_msg_c", "senha-gerente", "gerente")
	e1LoginVinculado(t, app, st, "ger_msg_c", gidC, "senha-gerente")
	criaUsuarioTeste(t, st, "op_msg_a", "senha-gerente", "operador")
	ckOpA := e1LoginVinculado(t, app, st, "op_msg_a", gidA, "senha-gerente")
	pidOpA := e1PapelID(t, st, "op_msg_a", "operador")
	criaUsuarioTeste(t, st, "op_msg_b", "senha-gerente", "operador")
	e1LoginVinculado(t, app, st, "op_msg_b", gidB, "senha-gerente")
	pidOpB := e1PapelID(t, st, "op_msg_b", "operador")
	pidGerC := e1PapelID(t, st, "ger_msg_c", "gerente")

	// operador SEM grupo: não envia a NINGUÉM (403) — era o pior ramo do R-14
	criaUsuarioTeste(t, st, "op_sem_grupo", "senha-gerente", "operador")
	ckSemGrupo := loginAs(t, app, "op_sem_grupo", "senha-gerente")
	rr, res := doJSONReq(app, "POST", "/api/mensagens", map[string]any{
		"destinatario_papel_ids": []int64{pidOpA},
		"assunto":                "fora do escopo", "corpo": "<p>x</p>",
	}, ckSemGrupo)
	if rr.Code != http.StatusForbidden {
		t.Fatalf("R-14: conta sem grupo deveria 403 no envio, veio %d (%v)", rr.Code, res)
	}

	// gerente → papel do PRÓPRIO grupo → 200
	rr, res = doJSONReq(app, "POST", "/api/mensagens", map[string]any{
		"destinatario_papel_ids": []int64{pidOpA},
		"assunto":                "próprio grupo", "corpo": "<p>x</p>",
	}, ckGerA)
	if rr.Code != http.StatusOK {
		t.Fatalf("R-14: envio ao próprio grupo deveria 200, veio %d (%v)", rr.Code, res)
	}

	// gerente → papel de grupo SUBORDINADO → 200 (árvore, vínculo bilateral)
	rr, res = doJSONReq(app, "POST", "/api/mensagens", map[string]any{
		"destinatario_papel_ids": []int64{pidOpB},
		"assunto":                "subordinado", "corpo": "<p>x</p>",
	}, ckGerA)
	if rr.Code != http.StatusOK {
		t.Fatalf("R-14: envio a subordinado deveria 200, veio %d (%v)", rr.Code, res)
	}

	// gerente → papel de grupo NÃO-relacionado → 403
	rr, res = doJSONReq(app, "POST", "/api/mensagens", map[string]any{
		"destinatario_papel_ids": []int64{pidGerC},
		"assunto":                "grupo alheio", "corpo": "<p>x</p>",
	}, ckGerA)
	if rr.Code != http.StatusForbidden {
		t.Fatalf("R-14: envio a grupo não-relacionado deveria 403, veio %d (%v)", rr.Code, res)
	}

	// operador do grupo → caixa admin (destino global de infraestrutura) → 200
	var pidAdmin int64
	if err := st.db.QueryRow(`SELECT id FROM usuario_papeis WHERE papel='admin' LIMIT 1`).Scan(&pidAdmin); err != nil {
		t.Fatalf("papel admin: %v", err)
	}
	rr, res = doJSONReq(app, "POST", "/api/mensagens", map[string]any{
		"destinatario_papel_ids": []int64{pidAdmin},
		"assunto":                "para a administração", "corpo": "<p>x</p>",
	}, ckOpA)
	if rr.Code != http.StatusOK {
		t.Fatalf("R-14: envio à caixa admin deveria 200, veio %d (%v)", rr.Code, res)
	}
}

// ---------- R-16 — senha invalida sessões; admin com troca obrigatória ----------

func TestE1R16SenhaInvalidaSessoes(t *testing.T) {
	app, st, cleanup := setupTestApp(t)
	defer cleanup()

	gidA := e1CriaGrupo(t, st, "Grp E1 Senha A")
	criaUsuarioTeste(t, st, "ger_senha", "senha-gerente", "gerente")
	ckGer := e1LoginVinculado(t, app, st, "ger_senha", gidA, "senha-gerente")

	// (a1) troca PELA PRÓPRIA conta: outras sessões morrem, a corrente fica
	criaUsuarioTeste(t, st, "usr_senha", "SenhaAntiga1", "operador")
	ckA := loginAs(t, app, "usr_senha", "SenhaAntiga1")
	ckB := loginAs(t, app, "usr_senha", "SenhaAntiga1")

	rr, res := doJSONReq(app, "POST", "/api/senha", map[string]any{"atual": "SenhaAntiga1", "nova": "SenhaNova123"}, ckA)
	if rr.Code != http.StatusOK {
		t.Fatalf("R-16: troca de senha própria deveria 200, veio %d (%v)", rr.Code, res)
	}

	// sessão usada na troca segue viva (decisão documentada §3.C)
	if rrMe, _ := doJSONReq(app, "GET", "/api/me", nil, ckA); rrMe.Code != http.StatusOK {
		t.Fatalf("R-16: sessão CORRENTE da própria troca deveria permanecer, veio %d", rrMe.Code)
	}
	// as demais morreram
	if rrMe, _ := doJSONReq(app, "GET", "/api/me", nil, ckB); rrMe.Code != http.StatusUnauthorized {
		t.Fatalf("R-16: sessão ANTIGA deveria morrer com a troca (401), veio %d", rrMe.Code)
	}
	// senha nova loga; a antiga não
	if rrL, _ := doJSONReq(app, "POST", "/api/login", map[string]string{"login": "usr_senha", "senha": "SenhaNova123"}, nil); rrL.Code != http.StatusOK {
		t.Fatalf("R-16: senha nova deveria logar, veio %d", rrL.Code)
	}
	if rrL, resL := doJSONReq(app, "POST", "/api/login", map[string]string{"login": "usr_senha", "senha": "SenhaAntiga1"}, nil); rrL.Code != http.StatusUnauthorized {
		t.Fatalf("R-16: senha antiga deveria ser recusada, veio %d (%v)", rrL.Code, resL)
	}

	// (a2) redefinição POR OUTREM (gerente): TODAS as sessões do alvo caem
	criaUsuarioTeste(t, st, "op_senha", "SenhaDoOp123", "operador")
	e1VinculaConta(t, st, "op_senha", gidA)
	ckOp := loginAs(t, app, "op_senha", "SenhaDoOp123")
	uidOp := e1UsuarioID(t, st, "op_senha")
	rr, res = doJSONReq(app, "POST", fmt.Sprintf("/api/usuarios/%d/senha", uidOp), map[string]any{"senha": "Redefinida789"}, ckGer)
	if rr.Code != http.StatusOK {
		t.Fatalf("R-16: redefinição pelo gerente deveria 200, veio %d (%v)", rr.Code, res)
	}
	if rrMe, _ := doJSONReq(app, "GET", "/api/me", nil, ckOp); rrMe.Code != http.StatusUnauthorized {
		t.Fatalf("R-16: redefinição por outrem deveria derrubar a sessão do alvo (401), veio %d", rrMe.Code)
	}
}

func TestE1R16AdminSembradoExigeTroca(t *testing.T) {
	// senha-PADRÃO 'admin' no 1º boot → troca obrigatória (precisa_setup)
	app, _, cleanup := e1SetupComSenha(t, "admin")
	defer cleanup()

	ck := loginAs(t, app, "admin", "admin")
	rr, _ := doJSONReq(app, "GET", "/api/usuarios", nil, ck)
	if rr.Code != http.StatusForbidden || !strings.Contains(rr.Body.String(), "req_setup") {
		t.Fatalf("R-16: admin semeado com senha-padrão deveria estar BLOQUEADO (403 req_setup), veio %d %s", rr.Code, rr.Body.String())
	}

	// troca obrigatória via /api/setup (doutrina existente — gate central)
	rr, res := doJSONReq(app, "POST", "/api/setup", map[string]any{"novo_login": "admin", "nova_senha": "TrocaForte123"}, ck)
	if rr.Code != http.StatusOK {
		t.Fatalf("R-16: setup do admin deveria 200, veio %d (%v)", rr.Code, res)
	}

	ck2 := loginAs(t, app, "admin", "TrocaForte123")
	if rr, _ := doJSONReq(app, "GET", "/api/usuarios", nil, ck2); rr.Code != http.StatusOK {
		t.Fatalf("R-16: após a troca o admin deveria operar (200), veio %d", rr.Code)
	}
}

func TestE1R16AdminComSenhaPropriaNasceLivre(t *testing.T) {
	// deploy que define SCI_ADMIN_SENHA no 1º boot: sem marca de troca
	app, _, cleanup := e1SetupComSenha(t, "senha-do-comando-1")
	defer cleanup()

	ck := loginAs(t, app, "admin", "senha-do-comando-1")
	if rr, _ := doJSONReq(app, "GET", "/api/usuarios", nil, ck); rr.Code != http.StatusOK {
		t.Fatalf("R-16: admin com senha própria no seed NÃO deveria nascer bloqueado, veio %d", rr.Code)
	}
}
