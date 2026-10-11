package main

// onda_v160_f5_test.go — v1.6.0 Fase 5: CONTA SEM FUNÇÃO + página de bloqueio.
// usuarios.papel='sem_funcao' é AUSÊNCIA de contexto: SEM linha em
// usuario_papeis (o CHECK da v45 deliberadamente não o inclui); a sessão nasce
// com papel_ativo_id NULL e UsuarioDaSessao cai no usuarios.papel. O escopo
// central (lista única papelTemEscopoDeDados consumida por exigeEscopo e
// escopoDoUsuario) devolve -1 para papel fora da lista — a guarda central da
// v1.5.4-A (403 + filtroGrupoSQL AND 1=0) barra TODO dado de grupo, MESMO COM
// grupo no cadastro.
//
// Matriz da conta (persona: setupTestApp + loginAs + doJSONReq):
//   login/me/logout/perfil GET-PATCH/senha própria/foto própria → 200;
//   mural, mensagens enviar, pessoas, conferência hoje, material itens,
//   escalas pdf, relatórios, grupos, catálogo → 403.
// Criação: enc_pessoal (contexto enc) e gerente criam 'sem_funcao' → 200;
// chefe → 403; admin exige grupo; hUsuarioPapelAdd rejeita (não é papel de
// linha). Reparo: gerente edita/redefine senha da conta do próprio grupo.
// Designação posterior de cadeira devolve o acesso (login novo resolve o
// contexto pela linha materializada — Fase 3).

import (
	"net/http"
	"strings"
	"testing"
)

// f5Setup: grupo + gerente + designado na cadeira enc_pessoal (contexto enc).
func f5Setup(t *testing.T, app *App, st *Store, sfx string) (gid int64, ckGer, ckEnc *http.Cookie) {
	t.Helper()
	if err := st.db.QueryRow(`INSERT INTO grupos (nome) VALUES (?) RETURNING id`, "Grp F5 "+sfx).Scan(&gid); err != nil {
		t.Fatalf("criar grupo: %v", err)
	}
	criaUsuarioTeste(t, st, "f5_ger_"+sfx, "senha-ger", "gerente")
	if _, err := st.db.Exec(`UPDATE usuarios SET grupo_id = ? WHERE login = 'f5_ger_`+sfx+`'`, gid); err != nil {
		t.Fatalf("vincular gerente: %v", err)
	}
	ckGer = loginAs(t, app, "f5_ger_"+sfx, "senha-ger")

	var fPess int64
	if err := st.db.QueryRow(`SELECT id FROM funcoes WHERE chave = 'enc_pessoal'`).Scan(&fPess); err != nil {
		t.Fatalf("cadeira enc_pessoal ausente: %v", err)
	}
	criaUsuarioTeste(t, st, "f5_enc_"+sfx, "senha-enc", "")
	if _, err := st.db.Exec(`UPDATE usuarios SET grupo_id = ? WHERE login = 'f5_enc_`+sfx+`'`, gid); err != nil {
		t.Fatalf("vincular enc: %v", err)
	}
	var idEnc int64
	if err := st.db.QueryRow(`SELECT id FROM usuarios WHERE login = 'f5_enc_`+sfx+`'`).Scan(&idEnc); err != nil {
		t.Fatalf("id enc: %v", err)
	}
	if _, err := st.db.Exec(`INSERT INTO funcao_membros (funcao_id, grupo_id, usuario_id, titularidade) VALUES (?,?,?,'titular')`, fPess, gid, idEnc); err != nil {
		t.Fatalf("designar enc: %v", err)
	}
	v45Reexecuta(t, st)
	ckEnc = loginAs(t, app, "f5_enc_"+sfx, "senha-enc")
	return
}

// f5CriaSemFuncao: cria a conta sem_funcao pelo gerente (senha própria → sem
// setup pendente) e devolve o id.
func f5CriaSemFuncao(t *testing.T, app *App, ckGer *http.Cookie, login string) int64 {
	t.Helper()
	rr, res := doJSONReq(app, "POST", "/api/usuarios", map[string]any{
		"login": login, "senha": "SenhaF5" + login, "papel": "sem_funcao"}, ckGer)
	if rr.Code != http.StatusOK {
		t.Fatalf("criar conta sem_funcao (%s): quer 200, veio %d (%v)", login, rr.Code, res["erro"])
	}
	id, _ := res["id"].(float64)
	if id <= 0 {
		t.Fatalf("criar conta sem_funcao: id inválido: %v", res)
	}
	return int64(id)
}

// (C1) quem cria 'sem_funcao': enc no contexto (200), gerente (200), chefe
// (403), admin exige grupo; a conta NÃO ganha linha em usuario_papeis.
func TestF5CriacaoContaSemFuncao(t *testing.T) {
	app, st, cleanup := setupTestApp(t)
	defer cleanup()
	gid, ckGer, ckEnc := f5Setup(t, app, st, "c1")
	ckAdm := loginAs(t, app, "admin", "admin123")

	// ENCARREGADO (contexto enc_pessoal): grupo forçado ao próprio
	idEnc := f5CriaSemFuncao(t, app, ckEnc, "f5sf.enc1")
	var papel string
	var gidConta int64
	var nLinhas int
	if err := st.db.QueryRow(`SELECT papel, COALESCE(grupo_id,0) FROM usuarios WHERE id = ?`, idEnc).Scan(&papel, &gidConta); err != nil {
		t.Fatalf("ler conta do enc: %v", err)
	}
	if papel != "sem_funcao" || gidConta != gid {
		t.Fatalf("C1: conta do enc devia ser (sem_funcao, gid=%d), veio (%q, %d)", gid, papel, gidConta)
	}
	if err := st.db.QueryRow(`SELECT COUNT(*) FROM usuario_papeis WHERE usuario_id = ?`, idEnc).Scan(&nLinhas); err != nil || nLinhas != 0 {
		t.Fatalf("C1: sem_funcao NÃO ganha linha em usuario_papeis (n=%d, %v)", nLinhas, err)
	}

	// GERENTE: 200
	f5CriaSemFuncao(t, app, ckGer, "f5sf.ger1")

	// CHEFE: 403 (chefe só designa operadores)
	criaUsuarioTeste(t, st, "f5_chefe_c1", "senha-chefe", "chefe_setor")
	if _, err := st.db.Exec(`UPDATE usuarios SET grupo_id = ? WHERE login = 'f5_chefe_c1'`, gid); err != nil {
		t.Fatalf("vincular chefe: %v", err)
	}
	ckChefe := loginAs(t, app, "f5_chefe_c1", "senha-chefe")
	if rr, _ := doJSONReq(app, "POST", "/api/usuarios", map[string]any{"login": "f5sf.chefe1", "papel": "sem_funcao"}, ckChefe); rr.Code != http.StatusForbidden {
		t.Fatalf("C1: chefe criando sem_funcao deve 403, veio %d", rr.Code)
	}

	// ADMIN: sem grupo → 400; com grupo → 200
	rr, _ := doJSONReq(app, "POST", "/api/usuarios", map[string]any{"login": "f5sf.adm1", "senha": "SenhaF5adm1", "papel": "sem_funcao"}, ckAdm)
	if rr.Code != http.StatusBadRequest || !strings.Contains(rr.Body.String(), "conta sem função exige grupo de vinculação") {
		t.Fatalf("C1: admin sem grupo deve 400 'exige grupo', veio %d (%s)", rr.Code, rr.Body.String())
	}
	rr, _ = doJSONReq(app, "POST", "/api/usuarios", map[string]any{"login": "f5sf.adm2", "senha": "SenhaF5adm2", "papel": "sem_funcao", "grupo_id": gid}, ckAdm)
	if rr.Code != http.StatusOK {
		t.Fatalf("C1: admin com grupo deve 200, veio %d (%s)", rr.Code, rr.Body.String())
	}

	// hUsuarioPapelAdd REJEITA: 'sem_funcao' não é papel de linha
	rr, _ = doJSONReq(app, "POST", "/api/usuarios/"+idi(idEnc)+"/papeis", map[string]any{"papel": "sem_funcao", "grupo_id": gid}, ckGer)
	if rr.Code != http.StatusBadRequest || !strings.Contains(rr.Body.String(), "não é papel de linha") {
		t.Fatalf("C1: papel add sem_funcao deve 400 'não é papel de linha', veio %d (%s)", rr.Code, rr.Body.String())
	}
}

// (C2) matriz da conta sem_funcao COM grupo: o próprio universo (login, me,
// perfil, senha, foto) responde 200; TODO dado de grupo responde 403.
func TestF5MatrizContaSemFuncao(t *testing.T) {
	app, st, cleanup := setupTestApp(t)
	defer cleanup()
	gid, ckGer, _ := f5Setup(t, app, st, "c2")

	// setor do grupo (para o reparo de setor do gerente)
	var setorID int64
	if err := st.db.QueryRow(`INSERT INTO setores (nome, grupo_id, ativo) VALUES ('S F5', ?, 1) RETURNING id`, gid).Scan(&setorID); err != nil {
		t.Fatalf("criar setor: %v", err)
	}
	idSF := f5CriaSemFuncao(t, app, ckGer, "f5sf.mat")
	ckSF := loginAs(t, app, "f5sf.mat", "SenhaF5f5sf.mat")

	// --- o próprio universo: 200 ---
	if rr, _ := doJSONReq(app, "GET", "/api/me", nil, ckSF); rr.Code != http.StatusOK {
		t.Fatalf("C2: /api/me deve 200, veio %d", rr.Code)
	}
	if _, res := doJSONReq(app, "GET", "/api/me", nil, ckSF); res["usuario"].(map[string]any)["papel"] != "sem_funcao" {
		t.Fatalf("C2: papel ativo da sessão devia ser sem_funcao (papel_ativo_id NULL → usuarios.papel)")
	}
	if rr, _ := doJSONReq(app, "GET", "/api/perfil", nil, ckSF); rr.Code != http.StatusOK {
		t.Fatalf("C2: GET /api/perfil deve 200, veio %d", rr.Code)
	}
	if rr, _ := doJSONReq(app, "PATCH", "/api/perfil", map[string]any{"nome_guerra": "SF", "nome_completo": "Sem Função"}, ckSF); rr.Code != http.StatusOK {
		t.Fatalf("C2: PATCH /api/perfil deve 200, veio %d (%s)", rr.Code, rr.Body.String())
	}
	if rr, _ := doJSONReq(app, "POST", "/api/senha", map[string]any{"atual": "SenhaF5f5sf.mat", "nova": "SenhaF5nova1"}, ckSF); rr.Code != http.StatusOK {
		t.Fatalf("C2: POST /api/senha (própria) deve 200, veio %d (%s)", rr.Code, rr.Body.String())
	}
	// senha trocada: re-login com a nova (a sessão corrente foi preservada, mas
	// provamos que a credencial nova vale)
	if rres, _, _ := doLoginOK(t, app, "f5sf.mat", "SenhaF5nova1"); rres.StatusCode != http.StatusOK {
		t.Fatalf("C2: login com a senha nova deve 200, veio %d", rres.StatusCode)
	}
	if rr, _ := doJSONReq(app, "GET", "/api/usuarios/"+idi(idSF)+"/foto", nil, ckSF); rr.Code != http.StatusOK && rr.Code != http.StatusNotFound {
		t.Fatalf("C2: GET foto própria deve 200 (ou 404 sem foto), veio %d", rr.Code)
	}
	// logout por último dentro do bloco 200 não cabe (mata a sessão) — testado à parte abaixo.

	// --- todo dado de grupo: 403 ---
	negadas := []struct{ nome, metodo, rota string }{
		{"mural", "GET", "/api/avisos"},
		{"mensagens enviar", "POST", "/api/mensagens"},
		{"pessoas", "GET", "/api/pessoas"},
		{"conferência hoje", "GET", "/api/conferencia/hoje"},
		{"material itens", "GET", "/api/material/itens"},
		{"escalas pdf", "GET", "/api/escalas/relatorio-dia.pdf?data=2026-10-10"},
		{"relatórios", "GET", "/api/relatorio"},
		{"grupos", "GET", "/api/grupos"},
		{"catálogo", "GET", "/api/catalogo/setores"},
		{"contas", "GET", "/api/usuarios"},
	}
	for _, n := range negadas {
		rr, _ := doJSONReq(app, n.metodo, n.rota, map[string]any{"destinatario_papel_ids": []int64{1}, "assunto": "x", "corpo": "<p>y</p>"}, ckSF)
		if rr.Code != http.StatusForbidden {
			t.Fatalf("C2: %s (%s %s) deve 403, veio %d (%s)", n.nome, n.metodo, n.rota, rr.Code, rr.Body.String())
		}
	}

	// logout fecha a sessão da conta sem função (contrato da página de bloqueio)
	if rr, _ := doJSONReq(app, "POST", "/api/logout", map[string]any{}, ckSF); rr.Code != http.StatusOK {
		t.Fatalf("C2: logout deve 200, veio %d", rr.Code)
	}
	_ = setorID
}

// doLoginOK: login simples sem fatal (usado quando 200 não é garantia).
func doLoginOK(t *testing.T, app *App, login, senha string) (*http.Response, map[string]any, *http.Cookie) {
	t.Helper()
	rr, res := doJSONReq(app, "POST", "/api/login", map[string]string{"login": login, "senha": senha}, nil)
	var ck *http.Cookie
	for _, c := range rr.Result().Cookies() {
		if c.Name == cookieSessao {
			ck = c
		}
	}
	return rr.Result(), res, ck
}

// (C3) reparo pelo gerente: edição de nomes e redefinição de senha alcançam a
// conta sem_funcao do PRÓPRIO grupo; fora do grupo segue 403.
func TestF5ReparoPeloGerente(t *testing.T) {
	app, st, cleanup := setupTestApp(t)
	defer cleanup()
	_, ckGer, _ := f5Setup(t, app, st, "c3")

	// grupo alheio com a própria conta sem_funcao
	var gidFora int64
	if err := st.db.QueryRow(`INSERT INTO grupos (nome) VALUES ('Grp F3 Fora') RETURNING id`).Scan(&gidFora); err != nil {
		t.Fatalf("criar grupo fora: %v", err)
	}
	rr, res := doJSONReq(app, "POST", "/api/usuarios", map[string]any{"login": "f5sf.fora", "senha": "SenhaF5fora1", "papel": "sem_funcao", "grupo_id": gidFora}, loginAs(t, app, "admin", "admin123"))
	if rr.Code != http.StatusOK {
		t.Fatalf("conta sem_funcao de grupo alheio: %d (%v)", rr.Code, res["erro"])
	}
	var idFora int64
	if err := st.db.QueryRow(`SELECT id FROM usuarios WHERE login = 'f5sf.fora'`).Scan(&idFora); err != nil {
		t.Fatalf("id conta fora: %v", err)
	}

	idSF := f5CriaSemFuncao(t, app, ckGer, "f5sf.rep")

	// edição de nomes/setor (reparo) → 200
	if rr, _ := doJSONReq(app, "PATCH", "/api/usuarios/"+idi(idSF), map[string]any{"nome_guerra": "REPARO", "nome_completo": "Conta Em Reparo"}, ckGer); rr.Code != http.StatusOK {
		t.Fatalf("C3: gerente edita sem_funcao do próprio grupo (200), veio %d", rr.Code)
	}
	// redefinição de senha → 200
	if rr, _ := doJSONReq(app, "POST", "/api/usuarios/"+idi(idSF)+"/senha", map[string]any{"senha": "NovaSenhaF51"}, ckGer); rr.Code != http.StatusOK {
		t.Fatalf("C3: gerente redefine senha de sem_funcao (200), veio %d (%s)", rr.Code, rr.Body.String())
	}
	// credencial nova vale
	if rres, _, _ := doLoginOK(t, app, "f5sf.rep", "NovaSenhaF51"); rres.StatusCode != http.StatusOK {
		t.Fatalf("C3: login com senha redefinida deve 200, veio %d", rres.StatusCode)
	}
	// conta de OUTRO grupo: 403 nos dois
	if rr, _ := doJSONReq(app, "PATCH", "/api/usuarios/"+idi(idFora), map[string]any{"nome_guerra": "X"}, ckGer); rr.Code != http.StatusForbidden {
		t.Fatalf("C3: gerente edita sem_funcao de outro grupo deve 403, veio %d", rr.Code)
	}
	if rr, _ := doJSONReq(app, "POST", "/api/usuarios/"+idi(idFora)+"/senha", map[string]any{"senha": "SenhaAlheia1"}, ckGer); rr.Code != http.StatusForbidden {
		t.Fatalf("C3: gerente redefine senha de sem_funcao de outro grupo deve 403, veio %d", rr.Code)
	}
}

// (C4) designação posterior de cadeira devolve a conta: a linha materializada
// é o contexto do PRÓXIMO login (Fase 3) — e o escopo de dados volta.
func TestF5DesignacaoTiraDoBloqueio(t *testing.T) {
	app, st, cleanup := setupTestApp(t)
	defer cleanup()
	_, ckGer, _ := f5Setup(t, app, st, "c4")

	idSF := f5CriaSemFuncao(t, app, ckGer, "f5sf.des")
	ckSF := loginAs(t, app, "f5sf.des", "SenhaF5f5sf.des")
	if rr, _ := doJSONReq(app, "GET", "/api/grupos", nil, ckSF); rr.Code != http.StatusForbidden {
		t.Fatalf("C4: antes da designação, grupos deve 403, veio %d", rr.Code)
	}

	var fPess int64
	if err := st.db.QueryRow(`SELECT id FROM funcoes WHERE chave = 'enc_pessoal'`).Scan(&fPess); err != nil {
		t.Fatalf("cadeira enc_pessoal: %v", err)
	}
	// designa a cadeira pela API real (o sync da Fase 3 materializa a linha;
	// 'auxiliar' — a cadeira já tem titular: o f5_enc do setup)
	rr, res := doJSONReq(app, "POST", "/api/grupo/funcoes/membros", map[string]any{
		"funcao_id": fPess, "usuario_id": idSF, "titularidade": "auxiliar"}, ckGer)
	if rr.Code != http.StatusOK {
		t.Fatalf("C4: designar cadeira na conta sem_funcao deve 200, veio %d (%v)", rr.Code, res["erro"])
	}

	// a sessão ANTIGA continua bloqueada (escopo central lê o papel da sessão)
	if rr, _ := doJSONReq(app, "GET", "/api/grupos", nil, ckSF); rr.Code != http.StatusForbidden {
		t.Fatalf("C4: sessão antiga (papel_ativo NULL → sem_funcao) deve seguir 403, veio %d", rr.Code)
	}
	// LOGIN NOVO resolve o contexto enc_pessoal e o escopo volta
	ckNovo := loginAs(t, app, "f5sf.des", "SenhaF5f5sf.des")
	if _, resMe := doJSONReq(app, "GET", "/api/me", nil, ckNovo); resMe["usuario"].(map[string]any)["papel"] != "enc_pessoal" {
		t.Fatalf("C4: login novo devia resolver o contexto enc_pessoal, veio %v", resMe["usuario"].(map[string]any)["papel"])
	}
	if rr, _ := doJSONReq(app, "GET", "/api/grupos", nil, ckNovo); rr.Code != http.StatusOK {
		t.Fatalf("C4: depois da designação, dados do grupo voltam (200), veio %d", rr.Code)
	}
}
