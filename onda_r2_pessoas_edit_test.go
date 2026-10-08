package main

// onda_r2_pessoas_edit_test.go — FIX F3 (R2): validação de função do grupo
// no hPessoasEdit via funcaoValidaParaPessoa. Padrão cookiejar por persona.
//
// Cenários:
//   (a) gerente edita pessoa setando função de OUTRO grupo → 400 e dado intacto
//   (b) gerente edita pessoa setando função PRÓPRIA do grupo → 200
//   (c) gerente edita sem tocar função → 200
//   (d) admin move pessoa de grupo com função do grupo destino → 200

import (
	"fmt"
	"net/http"
	"testing"
)

func ondaR2EditSetup(t *testing.T, app *App, st *Store) (gidA, gidB, funcaoA, funcaoB, pessoaA int64, ckGer, ckAdmin *http.Cookie) {
	t.Helper()

	if err := st.db.QueryRow(`INSERT INTO grupos (nome) VALUES ('Grupo A R2') RETURNING id`).Scan(&gidA); err != nil {
		t.Fatalf("criar grupo A: %v", err)
	}
	if err := st.db.QueryRow(`INSERT INTO grupos (nome) VALUES ('Grupo B R2') RETURNING id`).Scan(&gidB); err != nil {
		t.Fatalf("criar grupo B: %v", err)
	}

	// função DO grupo A
	if err := st.db.QueryRow(`INSERT INTO funcoes (nome, grupo_id) VALUES ('Funcao So Do A', ?) RETURNING id`, gidA).Scan(&funcaoA); err != nil {
		t.Fatalf("criar funcao A: %v", err)
	}
	// função DO grupo B
	if err := st.db.QueryRow(`INSERT INTO funcoes (nome, grupo_id) VALUES ('Funcao So Do B', ?) RETURNING id`, gidB).Scan(&funcaoB); err != nil {
		t.Fatalf("criar funcao B: %v", err)
	}

	// gerente do grupo A
	criaUsuarioTeste(t, st, "ger_r2", "senha12345", "gerente")
	if _, err := st.db.Exec(`UPDATE usuarios SET grupo_id = ? WHERE login = 'ger_r2'`, gidA); err != nil {
		t.Fatalf("vincular ger_r2: %v", err)
	}
	ckGer = loginAs(t, app, "ger_r2", "senha12345")

	// admin (papel do seed) — já existe como "admin"
	ckAdmin = loginAs(t, app, "admin", "admin123")

	// pessoa no grupo A, SEM função inicialmente
	if err := st.db.QueryRow(`INSERT INTO pessoas (nome_guerra, nome_completo, grupo_id, status) VALUES ('PESSOA_R2', 'Pessoa R2 Teste', ?, 'ativo') RETURNING id`, gidA).Scan(&pessoaA); err != nil {
		t.Fatalf("criar pessoa A: %v", err)
	}

	return
}

// (a) gerente tenta editar pessoa com função de OUTRO grupo → 400 e dado intacto
func TestR2PessoasEditFuncaoDeOutroGrupo(t *testing.T) {
	app, st, cleanup := setupTestApp(t)
	defer cleanup()

	gidA, _, funcaoA, funcaoB, pessoaA, ckGer, _ := ondaR2EditSetup(t, app, st)
	_ = funcaoA
	_ = gidA

	// tenta PATCH com funcao_id do grupo B
	patchBody := map[string]any{
		"nome_guerra":   "PESSOA_R2",
		"nome_completo": "Pessoa R2 Teste",
		"status":        "ativo",
		"funcao_id":     funcaoB,
	}
	rr, res := doJSONReq(app, "PATCH", fmt.Sprintf("/api/pessoas/%d", pessoaA), patchBody, ckGer)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("(a) esperado 400 ao setar função de outro grupo, veio %d (%v)", rr.Code, res)
	}
	msg, _ := res["erro"].(string)
	if msg != "função não pertence ao grupo da pessoa" {
		t.Fatalf("(a) esperado mensagem 'função não pertence ao grupo da pessoa', veio %q", msg)
	}

	// dado INTACTO: a pessoa continua sem função
	var funcaoAtual *int64
	if err := st.db.QueryRow(`SELECT funcao_id FROM pessoas WHERE id = ?`, pessoaA).Scan(&funcaoAtual); err != nil {
		t.Fatalf("(a) ler funcao_id: %v", err)
	}
	if funcaoAtual != nil {
		t.Fatalf("(a) funcao_id deveria permanecer NULL, veio %d", *funcaoAtual)
	}
}

// (b) gerente edita pessoa com função PRÓPRIA do grupo → 200
func TestR2PessoasEditFuncaoPropria(t *testing.T) {
	app, st, cleanup := setupTestApp(t)
	defer cleanup()

	gidA, _, funcaoA, _, pessoaA, ckGer, _ := ondaR2EditSetup(t, app, st)
	_ = gidA

	patchBody := map[string]any{
		"nome_guerra":   "PESSOA_R2",
		"nome_completo": "Pessoa R2 Teste",
		"status":        "ativo",
		"funcao_id":     funcaoA,
	}
	rr, res := doJSONReq(app, "PATCH", fmt.Sprintf("/api/pessoas/%d", pessoaA), patchBody, ckGer)
	if rr.Code != http.StatusOK {
		t.Fatalf("(b) esperado 200 ao setar função do próprio grupo, veio %d (%v)", rr.Code, res)
	}

	// verifica que a função foi gravada
	var funcaoAtual *int64
	if err := st.db.QueryRow(`SELECT funcao_id FROM pessoas WHERE id = ?`, pessoaA).Scan(&funcaoAtual); err != nil {
		t.Fatalf("(b) ler funcao_id: %v", err)
	}
	if funcaoAtual == nil || *funcaoAtual != funcaoA {
		t.Fatalf("(b) funcao_id deveria ser %d, veio %v", funcaoA, funcaoAtual)
	}
}

// (c) gerente edita pessoa sem tocar no campo função → 200
func TestR2PessoasEditSemTocarFuncao(t *testing.T) {
	app, st, cleanup := setupTestApp(t)
	defer cleanup()

	_, _, funcaoA, _, pessoaA, ckGer, _ := ondaR2EditSetup(t, app, st)

	// primeiro seta uma função válida
	if _, err := st.db.Exec(`UPDATE pessoas SET funcao_id = ? WHERE id = ?`, funcaoA, pessoaA); err != nil {
		t.Fatalf("(c) setup funcao: %v", err)
	}

	// edita SEM funcao_id no corpo
	patchBody := map[string]any{
		"nome_guerra":   "PESSOA_R2_EDIT",
		"nome_completo": "Pessoa R2 Editada",
		"status":        "ativo",
	}
	rr, res := doJSONReq(app, "PATCH", fmt.Sprintf("/api/pessoas/%d", pessoaA), patchBody, ckGer)
	if rr.Code != http.StatusOK {
		t.Fatalf("(c) esperado 200 ao editar sem função, veio %d (%v)", rr.Code, res)
	}

	// nome_guerra foi atualizado (campos de texto mudam)
	var nome string
	if err := st.db.QueryRow(`SELECT nome_guerra FROM pessoas WHERE id = ?`, pessoaA).Scan(&nome); err != nil {
		t.Fatalf("(c) ler nome_guerra: %v", err)
	}
	if nome != "PESSOA_R2_EDIT" {
		t.Fatalf("(c) nome_guerra deveria ser PESSOA_R2_EDIT, veio %q", nome)
	}
}

// (d) admin move pessoa de grupo COM função do grupo destino → 200
func TestR2PessoasEditAdminMoveGrupoComFuncao(t *testing.T) {
	app, st, cleanup := setupTestApp(t)
	defer cleanup()

	gidA, gidB, funcaoA, funcaoB, pessoaA, _, ckAdmin := ondaR2EditSetup(t, app, st)
	_ = funcaoA

	// admin define função A (do grupo A) na pessoa
	if _, err := st.db.Exec(`UPDATE pessoas SET funcao_id = ? WHERE id = ?`, funcaoA, pessoaA); err != nil {
		t.Fatalf("(d) setup funcao A: %v", err)
	}

	// admin move a pessoa para grupo B E troca a função para a do grupo B
	patchBody := map[string]any{
		"nome_guerra":   "PESSOA_R2",
		"nome_completo": "Pessoa R2 Teste",
		"status":        "ativo",
		"grupo_id":      gidB,
		"funcao_id":     funcaoB,
	}
	rr, res := doJSONReq(app, "PATCH", fmt.Sprintf("/api/pessoas/%d", pessoaA), patchBody, ckAdmin)
	if rr.Code != http.StatusOK {
		t.Fatalf("(d) esperado 200 admin move grupo+função, veio %d (%v)", rr.Code, res)
	}

	// verifica grupo e função atualizados
	var gidAtual *int64
	var funcaoAtual *int64
	if err := st.db.QueryRow(`SELECT grupo_id, funcao_id FROM pessoas WHERE id = ?`, pessoaA).Scan(&gidAtual, &funcaoAtual); err != nil {
		t.Fatalf("(d) ler dados: %v", err)
	}
	if gidAtual == nil || *gidAtual != gidB {
		t.Fatalf("(d) grupo_id deveria ser %d, veio %v", gidB, gidAtual)
	}
	if funcaoAtual == nil || *funcaoAtual != funcaoB {
		t.Fatalf("(d) funcao_id deveria ser %d, veio %v", funcaoB, funcaoAtual)
	}

	_ = gidA
}