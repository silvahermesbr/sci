package main

// onda_r3_poderes_test.go — R3 (fix/r3-poderes-designacao, ordem Diretor 08/10):
//   Poderes de gestão de pessoal passam a vir da DESIGNAÇÃO (funcao_membros),
//   não do nome da função no cadastro do usuário.
//   (a) usuário SEM função no cadastro, DESIGNADO titular 'Encarregado de Pessoal'
//       → podeGestaoPessoal true
//   (b) usuário com função 'encarregado' no cadastro SEM designação → false
//   (c) designado 'auxiliar' → true
//   (d) migrarV37 em banco populado estilo-velho cria designações,
//       e é idempotente

import (
	"net/http"
	"testing"
)

// setupR3: grupo, funções, usuários e designações de teste.
func setupR3(t *testing.T, app *App, st *Store) (gid int64) {
	t.Helper()
	if err := st.db.QueryRow(`INSERT INTO grupos (nome) VALUES ('Grp R3') RETURNING id`).Scan(&gid); err != nil {
		t.Fatalf("criar grupo: %v", err)
	}
	return gid
}

// TestR3DesignacaoEncarregado: (a) usuário SEM função no cadastro, DESIGNADO
// titular de função 'Encarregado de Pessoal' → podeGestaoPessoal true.
func TestR3DesignacaoEncarregado(t *testing.T) {
	app, st, cleanup := setupTestApp(t)
	defer cleanup()
	gid := setupR3(t, app, st)

	var fEnc int64
	if err := st.db.QueryRow(`INSERT INTO funcoes (nome, grupo_id) VALUES ('Encarregado de Pessoal', ?) RETURNING id`, gid).Scan(&fEnc); err != nil {
		t.Fatalf("criar função encarregado: %v", err)
	}

	criaUsuarioTeste(t, st, "r3enc01", "senha-r3", "")
	var uid int64
	if err := st.db.QueryRow(`SELECT id FROM usuarios WHERE login='r3enc01'`).Scan(&uid); err != nil {
		t.Fatalf("id r3enc01: %v", err)
	}
	if _, err := st.db.Exec(`UPDATE usuarios SET grupo_id = ? WHERE id = ?`, gid, uid); err != nil {
		t.Fatalf("vincular grupo: %v", err)
	}
	// Designação DIRETA (sem funcao_id no cadastro)
	if _, err := st.db.Exec(`INSERT INTO funcao_membros (funcao_id, grupo_id, usuario_id, titularidade) VALUES (?,?,?,'titular')`, fEnc, gid, uid); err != nil {
		t.Fatalf("designar encarregado: %v", err)
	}

	ck := loginAs(t, app, "r3enc01", "senha-r3")

	// (a) podeGestaoPessoal true — acesso a rota de gestão
	if rr, _ := doJSONReq(app, "POST", "/api/pessoas", map[string]any{"nome_guerra": "R3ENC", "nome_completo": "R3 Encarregado Teste"}, ck); rr.Code != http.StatusOK {
		t.Fatalf("(a) encarregado designado cadastra pessoa deve 200, veio %d", rr.Code)
	}
	if rr, _ := doJSONReq(app, "GET", "/api/usuarios", nil, ck); rr.Code != http.StatusOK {
		t.Fatalf("(a) encarregado designado lista contas deve 200, veio %d", rr.Code)
	}
}

// TestR3SemDesignacao: (b) usuário com função 'encarregado' no cadastro
// (funcao_id) SEM designação → podeGestaoPessoal false.
func TestR3SemDesignacao(t *testing.T) {
	app, st, cleanup := setupTestApp(t)
	defer cleanup()
	gid := setupR3(t, app, st)

	var fEnc int64
	if err := st.db.QueryRow(`INSERT INTO funcoes (nome, grupo_id) VALUES ('Encarregado de Pessoal', ?) RETURNING id`, gid).Scan(&fEnc); err != nil {
		t.Fatalf("criar função encarregado: %v", err)
	}

	criaUsuarioTeste(t, st, "r3nodis", "senha-r3", "")
	var uid int64
	if err := st.db.QueryRow(`SELECT id FROM usuarios WHERE login='r3nodis'`).Scan(&uid); err != nil {
		t.Fatalf("id r3nodis: %v", err)
	}
	// funcao_id no cadastro, MAS SEM designação em funcao_membros
	if _, err := st.db.Exec(`UPDATE usuarios SET grupo_id = ?, funcao_id = ? WHERE id = ?`, gid, fEnc, uid); err != nil {
		t.Fatalf("vincular grupo+funcao: %v", err)
	}

	ck := loginAs(t, app, "r3nodis", "senha-r3")

	// (b) sem designação → 403 em rotas GUARDADAS (POST pessoa, POST usuario — GET
	// usuarios é aberto a todos do grupo, não passa por guardaGestaoPessoal)
	if rr, _ := doJSONReq(app, "POST", "/api/pessoas", map[string]any{"nome_guerra": "R3ND", "nome_completo": "R3 Sem Designacao"}, ck); rr.Code != http.StatusForbidden {
		t.Fatalf("(b) sem designação POST pessoa deve 403, veio %d", rr.Code)
	}
	if rr, _ := doJSONReq(app, "POST", "/api/usuarios", map[string]any{"login": "r3nopoder", "papel": "operador"}, ck); rr.Code != http.StatusForbidden {
		t.Fatalf("(b) sem designação POST usuario deve 403, veio %d", rr.Code)
	}
}

// TestR3DesignacaoAuxiliar: (c) designado 'auxiliar' → true.
func TestR3DesignacaoAuxiliar(t *testing.T) {
	app, st, cleanup := setupTestApp(t)
	defer cleanup()
	gid := setupR3(t, app, st)

	var fAux int64
	if err := st.db.QueryRow(`INSERT INTO funcoes (nome, grupo_id) VALUES ('Auxiliar de Pessoal', ?) RETURNING id`, gid).Scan(&fAux); err != nil {
		t.Fatalf("criar função auxiliar: %v", err)
	}

	criaUsuarioTeste(t, st, "r3aux01", "senha-r3", "")
	var uid int64
	if err := st.db.QueryRow(`SELECT id FROM usuarios WHERE login='r3aux01'`).Scan(&uid); err != nil {
		t.Fatalf("id r3aux01: %v", err)
	}
	if _, err := st.db.Exec(`UPDATE usuarios SET grupo_id = ? WHERE id = ?`, gid, uid); err != nil {
		t.Fatalf("vincular grupo: %v", err)
	}
	// Designação 'auxiliar'
	if _, err := st.db.Exec(`INSERT INTO funcao_membros (funcao_id, grupo_id, usuario_id, titularidade) VALUES (?,?,?,'auxiliar')`, fAux, gid, uid); err != nil {
		t.Fatalf("designar auxiliar: %v", err)
	}

	ck := loginAs(t, app, "r3aux01", "senha-r3")

	// (c) auxiliar → acesso
	if rr, _ := doJSONReq(app, "POST", "/api/pessoas", map[string]any{"nome_guerra": "R3AUX", "nome_completo": "R3 Auxiliar Teste"}, ck); rr.Code != http.StatusOK {
		t.Fatalf("(c) auxiliar designado cadastra pessoa deve 200, veio %d", rr.Code)
	}
	if rr, _ := doJSONReq(app, "GET", "/api/usuarios", nil, ck); rr.Code != http.StatusOK {
		t.Fatalf("(c) auxiliar designado lista contas deve 200, veio %d", rr.Code)
	}
}

// TestR3MigrarV37: (d) migrarV37 em banco populado estilo-velho cria
// designações e é idempotente.
func TestR3MigrarV37(t *testing.T) {
	_, st, cleanup := setupTestApp(t)
	defer cleanup()

	// Cria grupo e função estilo-velho (sem funcao_membros)
	var gid1, gid2 int64
	if err := st.db.QueryRow(`INSERT INTO grupos (nome) VALUES ('Grp R3 V37 A') RETURNING id`).Scan(&gid1); err != nil {
		t.Fatalf("criar grupo A: %v", err)
	}
	if err := st.db.QueryRow(`INSERT INTO grupos (nome) VALUES ('Grp R3 V37 B') RETURNING id`).Scan(&gid2); err != nil {
		t.Fatalf("criar grupo B: %v", err)
	}
	var fEnc, fOutra int64
	if err := st.db.QueryRow(`INSERT INTO funcoes (nome, grupo_id) VALUES ('Encarregado de Pessoal', ?) RETURNING id`, gid1).Scan(&fEnc); err != nil {
		t.Fatalf("criar função encarregado: %v", err)
	}
	if err := st.db.QueryRow(`INSERT INTO funcoes (nome, grupo_id) VALUES ('Furriel', ?) RETURNING id`, gid1).Scan(&fOutra); err != nil {
		t.Fatalf("criar função furriel: %v", err)
	}

	// Seed: 2 usuários no estilo-velho (funcao_id no cadastro, sem funcao_membros)
	criaUsuarioTeste(t, st, "velho_enc", "senha-r3", "")
	criaUsuarioTeste(t, st, "velho_outro", "senha-r3", "")
	var uidEnc, uidOutro int64
	if err := st.db.QueryRow(`SELECT id FROM usuarios WHERE login='velho_enc'`).Scan(&uidEnc); err != nil {
		t.Fatalf("id velho_enc: %v", err)
	}
	if err := st.db.QueryRow(`SELECT id FROM usuarios WHERE login='velho_outro'`).Scan(&uidOutro); err != nil {
		t.Fatalf("id velho_outro: %v", err)
	}
	// estilo-velho: funcao_id no cadastro, grupo, SEM designação
	if _, err := st.db.Exec(`UPDATE usuarios SET grupo_id = ?, funcao_id = ? WHERE id = ?`, gid1, fEnc, uidEnc); err != nil {
		t.Fatalf("vincular velho_enc: %v", err)
	}
	if _, err := st.db.Exec(`UPDATE usuarios SET grupo_id = ? WHERE id = ?`, gid1, uidOutro); err != nil {
		t.Fatalf("vincular velho_outro: %v", err)
	}

	// Antes da migração: zero linhas em funcao_membros
	var antes int
	if err := st.db.QueryRow(`SELECT COUNT(*) FROM funcao_membros`).Scan(&antes); err != nil || antes != 0 {
		t.Fatalf("antes da migração: funcao_membros deve estar vazio (n=%d err=%v)", antes, err)
	}

	// Remove o marcador v37 (setado por setupTestApp → AbrirStore → migrarV37)
	// para que a migração RODE DE NOVO com os dados seed inseridos agora.
	if _, err := st.db.Exec(`DELETE FROM schema_migrations WHERE versao = 37`); err != nil {
		t.Fatalf("remover guarda v37: %v", err)
	}

	// Roda migrarV37
	if err := st.migrarV37(); err != nil {
		t.Fatalf("migrarV37: %v", err)
	}

	// Depois: 1 linha em funcao_membros (só velho_enc, que tinha 'encarregado')
	var depois int
	if err := st.db.QueryRow(`SELECT COUNT(*) FROM funcao_membros`).Scan(&depois); err != nil || depois != 1 {
		t.Fatalf("após migração: esperado 1 designação, veio %d (err=%v)", depois, err)
	}
	var fmFuncaoID int64
	var fmTitularidade string
	if err := st.db.QueryRow(`SELECT funcao_id, titularidade FROM funcao_membros WHERE usuario_id = ?`, uidEnc).Scan(&fmFuncaoID, &fmTitularidade); err != nil {
		t.Fatalf("designação velho_enc não encontrada: %v", err)
	}
	if fmFuncaoID != fEnc {
		t.Fatalf("esperado funcao_id %d, veio %d", fEnc, fmFuncaoID)
	}
	if fmTitularidade != "titular" {
		t.Fatalf("esperado titularidade 'titular', veio %q", fmTitularidade)
	}

	// IDEMPOTÊNCIA: segunda execução não quebra nem duplica
	if err := st.migrarV37(); err != nil {
		t.Fatalf("migrarV37 idempotente: %v", err)
	}
	var contado int
	if err := st.db.QueryRow(`SELECT COUNT(*) FROM funcao_membros`).Scan(&contado); err != nil || contado != 1 {
		t.Fatalf("idempotência: esperado 1 designação, veio %d (err=%v)", contado, err)
	}
	// Schema_migrations registra v37
	var ver int
	if err := st.db.QueryRow(`SELECT versao FROM schema_migrations WHERE versao=37`).Scan(&ver); err != nil || ver != 37 {
		t.Fatalf("schema_migrations não tem v37 (v=%d err=%v)", ver, err)
	}
}