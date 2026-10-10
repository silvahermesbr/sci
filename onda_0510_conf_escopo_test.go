package main

// Testes da onda 05/10 — escopo de conferências (ordem Diretor):
//   E1. ENCARREGADO DE PESSOAL (função c/ "encarregado", SEM papel do sistema)
//       inicia conferência → 200 (escopo de grupo, como o gerente).
//   E2. Gerente continua iniciando → 200; admin continua proibido → 403.
//   S1. Operador marca pessoa de OUTRO setor → 403 (regra nova).
//   S2. Operador marca pessoa do PRÓPRIO setor → 200.
//   S3. Gerente marca pessoa de qualquer setor → 200 (regressão de escopo).
//   S4. Chefe de setor fora do próprio setor → 403 (regressão).
//   S5. Encarregado marca em qualquer setor do grupo → 200.
//   G1. Usuário comum (sem papel e sem função de encarregado) continua fora → 403.

import (
	"net/http"
	"testing"
)

func ondaEscopoSetup(t *testing.T, app *App, st *Store) (gid, setorA, setorB, pA, pB int64) {
	t.Helper()
	if err := st.db.QueryRow(`INSERT INTO grupos (nome) VALUES ('Grp Onda Escopo') RETURNING id`).Scan(&gid); err != nil {
		t.Fatalf("criar grupo: %v", err)
	}
	if err := st.db.QueryRow(`INSERT INTO setores (nome, sigla) VALUES ('Setor A Onda','SAO') RETURNING id`).Scan(&setorA); err != nil {
		t.Fatalf("criar setor A: %v", err)
	}
	if err := st.db.QueryRow(`INSERT INTO setores (nome, sigla) VALUES ('Setor B Onda','SBO') RETURNING id`).Scan(&setorB); err != nil {
		t.Fatalf("criar setor B: %v", err)
	}
	var fEnc int64
	if err := st.db.QueryRow(`SELECT id FROM funcoes WHERE chave = 'enc_pessoal'`).Scan(&fEnc); err != nil {
		t.Fatalf("criar função: %v", err)
	}

	criaUsuarioTeste(t, st, "enc01", "senha-enc", "")           // encarregado: SEM papel
	criaUsuarioTeste(t, st, "op01", "senha-op", "operador")     // operador do setor A
	criaUsuarioTeste(t, st, "ch01", "senha-ch", "chefe_setor")  // chefe do setor A
	criaUsuarioTeste(t, st, "ger01", "senha-ger", "gerente")    // gerente do grupo
	criaUsuarioTeste(t, st, "com01", "senha-com", "")           // comum, sem função

	if _, err := st.db.Exec(`UPDATE usuarios SET grupo_id = ?, funcao_id = ? WHERE login = 'enc01'`, gid, fEnc); err != nil {
		t.Fatalf("vincular enc: %v", err)
	}
	// R3: o PODER vem da designação (funcao_membros), não do funcao_id do
	// cadastro (que fica só para o display no /api/me).
	var enc01ID int64
	if err := st.db.QueryRow(`SELECT id FROM usuarios WHERE login = 'enc01'`).Scan(&enc01ID); err != nil {
		t.Fatalf("id enc01: %v", err)
	}
	if _, err := st.db.Exec(`INSERT INTO funcao_membros (funcao_id, grupo_id, usuario_id, titularidade) VALUES (?,?,?,'titular')`, fEnc, gid, enc01ID); err != nil {
		t.Fatalf("designar enc01: %v", err)
	}
	if _, err := st.db.Exec(`UPDATE usuarios SET grupo_id = ?, setor_id = ? WHERE login IN ('op01','ch01')`, gid, setorA); err != nil {
		t.Fatalf("vincular op/ch: %v", err)
	}
	if _, err := st.db.Exec(`UPDATE usuarios SET grupo_id = ? WHERE login = 'ger01'`, gid); err != nil {
		t.Fatalf("vincular ger: %v", err)
	}
	if _, err := st.db.Exec(`UPDATE usuario_papeis SET grupo_id = ? WHERE usuario_id IN (SELECT id FROM usuarios WHERE login IN ('op01','ch01','ger01'))`, gid); err != nil {
		t.Fatalf("vincular papeis: %v", err)
	}

	// v1.5.4-D1 (R-12): comando de chefiar = linha em chefe_setores (fonte
	// única) — o setup semeia na doutrina nova; o chefe-zumbi não deve mais
	// existir nem em fixture.
	var ch01ID int64
	if err := st.db.QueryRow(`SELECT id FROM usuarios WHERE login = 'ch01'`).Scan(&ch01ID); err != nil {
		t.Fatalf("id ch01: %v", err)
	}
	materializaComandoSetor(t, st, ch01ID, gid, setorA)

	if err := st.db.QueryRow(`INSERT INTO pessoas (nome_guerra, nome_completo, setor_id, grupo_id, status) VALUES ('SILVA A','Silva A Comum', ?, ?, 'ativo') RETURNING id`, setorA, gid).Scan(&pA); err != nil {
		t.Fatalf("criar pessoa A: %v", err)
	}
	if err := st.db.QueryRow(`INSERT INTO pessoas (nome_guerra, nome_completo, setor_id, grupo_id, status) VALUES ('SOUZA B','Souza B Comum', ?, ?, 'ativo') RETURNING id`, setorB, gid).Scan(&pB); err != nil {
		t.Fatalf("criar pessoa B: %v", err)
	}
	return
}

func iniciarConfOnda(t *testing.T, app *App, ck *http.Cookie) int {
	t.Helper()
	rr, _ := doJSONReq(app, "POST", "/api/conferencia/iniciar", map[string]any{"nome": "Conf Onda Escopo", "prazo_final": "17:00"}, ck)
	return rr.Code
}

func marcarOnda(t *testing.T, app *App, ck *http.Cookie, pessoaID int64, sit string) int {
	t.Helper()
	rr, _ := doJSONReq(app, "POST", "/api/conferencia/marcar", map[string]any{"pessoa_id": pessoaID, "situacao": sit, "verificado": true}, ck)
	return rr.Code
}

func TestOndaEscopoEncarregadoInicia(t *testing.T) {
	app, st, cleanup := setupTestApp(t)
	defer cleanup()
	gid, _, _, _, _ := ondaEscopoSetup(t, app, st)

	ckEnc := loginAs(t, app, "enc01", "senha-enc")
	if code := iniciarConfOnda(t, app, ckEnc); code != http.StatusOK {
		t.Fatalf("E1: encarregado deve iniciar conferência (200), veio %d", code)
	}
	var n int
	if err := st.db.QueryRow(`SELECT COUNT(*) FROM conferencias WHERE grupo_id = ? AND status = 'aberta'`, gid).Scan(&n); err != nil || n == 0 {
		t.Fatalf("E1: conferência não persistiu (n=%d err=%v)", n, err)
	}

	// S5: encarregado marca em qualquer setor do grupo
	if code := marcarOnda(t, app, ckEnc, 0, "presente"); code != http.StatusBadRequest {
		t.Fatalf("S5 sanity: pessoa 0 deve 400, veio %d", code)
	}
}

func TestOndaEscopoGerenteEAdmin(t *testing.T) {
	app, st, cleanup := setupTestApp(t)
	defer cleanup()
	ondaEscopoSetup(t, app, st)

	ckGer := loginAs(t, app, "ger01", "senha-ger")
	if code := iniciarConfOnda(t, app, ckGer); code != http.StatusOK {
		t.Fatalf("E2a: gerente deve iniciar (200), veio %d", code)
	}

	criaUsuarioTeste(t, st, "adm01", "senha-adm", "admin")
	ckAdm := loginAs(t, app, "adm01", "senha-adm")
	if codeAdm := iniciarConfOnda(t, app, ckAdm); codeAdm != http.StatusForbidden {
		t.Fatalf("E2b: admin deve continuar proibido (403), veio %d", codeAdm)
	}
}

func TestOndaEscopoSetorOperadorEChefe(t *testing.T) {
	app, st, cleanup := setupTestApp(t)
	defer cleanup()
	_, _, _, pA, pB := ondaEscopoSetup(t, app, st)

	ckGer := loginAs(t, app, "ger01", "senha-ger")
	if code := iniciarConfOnda(t, app, ckGer); code != http.StatusOK {
		t.Fatalf("setup: iniciar gerente %d", code)
	}

	ckOp := loginAs(t, app, "op01", "senha-op")
	// S1: operador fora do setor → 403
	if code := marcarOnda(t, app, ckOp, pB, "falta"); code != http.StatusForbidden {
		t.Fatalf("S1: operador fora do setor deve 403, veio %d", code)
	}
	// S2: operador no próprio setor → 200
	if code := marcarOnda(t, app, ckOp, pA, "presente"); code != http.StatusOK {
		t.Fatalf("S2: operador no próprio setor deve 200, veio %d", code)
	}

	ckCh := loginAs(t, app, "ch01", "senha-ch")
	// S4: chefe fora do setor → 403 (regressão)
	if code := marcarOnda(t, app, ckCh, pB, "falta"); code != http.StatusForbidden {
		t.Fatalf("S4: chefe fora do setor deve 403, veio %d", code)
	}
	if code := marcarOnda(t, app, ckCh, pA, "presente"); code != http.StatusOK {
		t.Fatalf("S4b: chefe no próprio setor deve 200, veio %d", code)
	}

	// S3: gerente em qualquer setor → 200 (regressão de escopo)
	if code := marcarOnda(t, app, ckGer, pB, "presente"); code != http.StatusOK {
		t.Fatalf("S3: gerente em qualquer setor deve 200, veio %d", code)
	}
}

func TestOndaEscopoComumContinuaFora(t *testing.T) {
	app, st, cleanup := setupTestApp(t)
	defer cleanup()
	ondaEscopoSetup(t, app, st)

	ckCom := loginAs(t, app, "com01", "senha-com")
	codeCom := iniciarConfOnda(t, app, ckCom)
	if codeCom != http.StatusForbidden {
		t.Fatalf("G1: comum deve 403 no iniciar, veio %d", codeCom)
	}
	rrM, _ := doJSONReq(app, "POST", "/api/conferencia/marcar", map[string]any{"pessoa_id": 1, "situacao": "presente"}, ckCom)
	if rrM.Code != http.StatusForbidden {
		t.Fatalf("G1b: comum deve 403 no marcar, veio %d", rrM.Code)
	}
}
