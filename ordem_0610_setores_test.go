package main

// ordem_0610_setores_test.go — Frente C, ITEM 8c (ordem 06/10): exclusão de
// setor com remanejamento. Prova executada:
//   1. setor com 3 pessoas (2 no banco de pessoal + 1 conta de usuário) →
//      exclusão zera setor_id de todos e apaga o setor;
//   2. setor referenciado por histórico de conferência (conferencia_setores)
//      → 409 "possui histórico — desative" (imutabilidade preservada);
//   3. gerente de outro grupo → 403 (fora da hierarquia).

import (
	"net/http"
	"testing"
)

// monta cenário comum: grupo A (gerente com sessão), grupo B (gerente rival),
// setor-alvo no grupo A e setor com histórico de conferência.
func setupExclusaoSetor(t *testing.T) (*App, *Store, *http.Cookie, int64, int64, int64) {
	t.Helper()
	app, st, teardown := setupTestApp(t)
	t.Cleanup(teardown)

	// grupos
	resA, err := st.db.Exec(`INSERT INTO grupos (nome, codigo) VALUES ('Grupo Alfa', 'ALFA')`)
	if err != nil {
		t.Fatalf("grupo A: %v", err)
	}
	gA, _ := resA.LastInsertId()
	resB, err := st.db.Exec(`INSERT INTO grupos (nome, codigo) VALUES ('Grupo Bravo', 'BRAVO')`)
	if err != nil {
		t.Fatalf("grupo B: %v", err)
	}
	gB, _ := resB.LastInsertId()

	// setores
	resS, _ := st.db.Exec(`INSERT INTO setores (nome, grupo_id) VALUES ('Comunicação Social', ?)`, gA)
	sCom, _ := resS.LastInsertId()
	resSH, _ := st.db.Exec(`INSERT INTO setores (nome, grupo_id) VALUES ('Setor com Histórico', ?)`, gA)
	sHist, _ := resSH.LastInsertId()

	// pessoal do setor: 3 registros (2 pessoas + 1 conta de usuário)
	for _, nome := range []string{"Silva", "Souza"} {
		if _, e := st.db.Exec(`INSERT INTO pessoas (nome_guerra, nome_completo, grupo_id, setor_id, status) VALUES (?, ?, ?, ?, 'ativo')`, nome, nome+" Completo", gA, sCom); e != nil {
			t.Fatalf("pessoa %s: %v", nome, e)
		}
	}

	// gerente do grupo A (com linha em usuario_papeis — doutrina da suíte)
	hashA, _ := hashSenha("senha-gerente")
	resU, _ := st.db.Exec(`INSERT INTO usuarios (login, senha_hash, papel, grupo_id, setor_id, precisa_setup) VALUES ('ger_alfa', ?, 'gerente', ?, NULL, 0)`, hashA, gA)
	uGer, _ := resU.LastInsertId()
	if _, e := st.db.Exec(`INSERT INTO usuario_papeis (usuario_id, grupo_id, papel) VALUES (?, ?, 'gerente')`, uGer, gA); e != nil {
		t.Fatalf("usuario_papeis gerente: %v", e)
	}
	// conta de usuário alocada ao setor (3º "pessoal" remanejado)
	resUC, _ := st.db.Exec(`INSERT INTO usuarios (login, senha_hash, papel, grupo_id, setor_id, precisa_setup) VALUES ('op_com', ?, 'operador', ?, ?, 0)`, hashA, gA, sCom)
	uOp, _ := resUC.LastInsertId()
	if _, e := st.db.Exec(`INSERT INTO usuario_papeis (usuario_id, grupo_id, papel) VALUES (?, ?, 'operador')`, uOp, gA); e != nil {
		t.Fatalf("usuario_papeis operador: %v", e)
	}

	// gerente do grupo B (rival)
	hashB, _ := hashSenha("senha-gerente")
	resUB, _ := st.db.Exec(`INSERT INTO usuarios (login, senha_hash, papel, grupo_id, precisa_setup) VALUES ('ger_bravo', ?, 'gerente', ?, 0)`, hashB, gB)
	uGerB, _ := resUB.LastInsertId()
	if _, e := st.db.Exec(`INSERT INTO usuario_papeis (usuario_id, grupo_id, papel) VALUES (?, ?, 'gerente')`, uGerB, gB); e != nil {
		t.Fatalf("usuario_papeis rival: %v", e)
	}

	tok, _, err := st.CriarSessao(uGer, ttlSessao)
	if err != nil {
		t.Fatalf("sessão gerente A: %v", err)
	}
	ck := &http.Cookie{Name: cookieSessao, Value: tok}
	return app, st, ck, gA, sCom, sHist
}

// 1. excluir setor com 3 pessoas → pessoas/contas com setor_id NULL + setor sumiu.
func TestExcluirSetorRemanejaPessoal(t *testing.T) {
	app, st, ck, _, sCom, _ := setupExclusaoSetor(t)

	rr, res := doJSONReq(app, "DELETE", "/api/setores/"+itoa64(sCom), nil, ck)
	if rr.Code != http.StatusOK {
		t.Fatalf("esperava 200 na exclusão, veio %d: %v", rr.Code, res)
	}
	if ok, _ := res["ok"].(bool); !ok {
		t.Fatalf("corpo sem ok=true: %v", res)
	}

	var nPessoasNoSetor, nContasNoSetor, nSetor int
	if e := st.db.QueryRow(`SELECT COUNT(*) FROM pessoas WHERE setor_id = ?`, sCom).Scan(&nPessoasNoSetor); e != nil {
		t.Fatalf("count pessoas: %v", e)
	}
	if e := st.db.QueryRow(`SELECT COUNT(*) FROM usuarios WHERE setor_id = ?`, sCom).Scan(&nContasNoSetor); e != nil {
		t.Fatalf("count usuarios: %v", e)
	}
	if e := st.db.QueryRow(`SELECT COUNT(*) FROM setores WHERE id = ?`, sCom).Scan(&nSetor); e != nil {
		t.Fatalf("count setores: %v", e)
	}
	if nPessoasNoSetor != 0 || nContasNoSetor != 0 || nSetor != 0 {
		t.Fatalf("remanejamento incompleto: pessoas_no_setor=%d contas_no_setor=%d setor=%d",
			nPessoasNoSetor, nContasNoSetor, nSetor)
	}

	// remanejados de fato existem (com setor_id NULL) — prova do "→ NULL"
	var nNull int
	if e := st.db.QueryRow(`SELECT COUNT(*) FROM pessoas WHERE nome_guerra IN ('Silva','Souza') AND setor_id IS NULL`).Scan(&nNull); e != nil {
		t.Fatalf("count NULL: %v", e)
	}
	if nNull != 2 {
		t.Fatalf("esperava 2 pessoas com setor_id NULL, obtive %d", nNull)
	}
}

// 2. setor com histórico de conferência (conferencia_setores) → 409 e NADA muda.
func TestExcluirSetorComHistoricoDa409(t *testing.T) {
	app, st, ck, gA, _, sHist := setupExclusaoSetor(t)

	// histórico: conferência fechada com o setor participante (FK real)
	resConf, err := st.db.Exec(`INSERT INTO conferencias (data, tipo_id, grupo_id, status, criado_por) VALUES ('2026-10-05', 1, ?, 'fechada', 1)`, gA)
	if err != nil {
		// seed pode não ter tipo_id=1 — cria um tipo antes de tentar de novo
		if _, e2 := st.db.Exec(`INSERT INTO conferencia_tipos (nome) VALUES ('C Pand')`); e2 != nil {
			t.Fatalf("tipo conferência: %v / %v", err, e2)
		}
		resConf, err = st.db.Exec(`INSERT INTO conferencias (data, tipo_id, grupo_id, status, criado_por) VALUES ('2026-10-05', 1, ?, 'fechada', 1)`, gA)
		if err != nil {
			t.Fatalf("conferência: %v", err)
		}
	}
	cID, _ := resConf.LastInsertId()
	if _, e := st.db.Exec(`INSERT INTO conferencia_setores (conferencia_id, setor_id, status) VALUES (?, ?, 'concluida')`, cID, sHist); e != nil {
		t.Fatalf("conferencia_setores: %v", e)
	}

	rr, res := doJSONReq(app, "DELETE", "/api/setores/"+itoa64(sHist), nil, ck)
	if rr.Code != http.StatusConflict {
		t.Fatalf("esperava 409, veio %d: %v", rr.Code, res)
	}
	if msg, _ := res["erro"].(string); !contains(msg, "histórico") || !contains(msg, "desative") {
		t.Fatalf("mensagem do 409 sem a doutrina: %q", msg)
	}
	var nSetor int
	if e := st.db.QueryRow(`SELECT COUNT(*) FROM setores WHERE id = ?`, sHist).Scan(&nSetor); e != nil {
		t.Fatalf("count setores: %v", e)
	}
	if nSetor != 1 {
		t.Fatalf("setor com histórico foi apagado (deveria permanecer)")
	}
}

// 3. gerente de OUTRO grupo → 403 (setor fora da hierarquia).
func TestExcluirSetorGerenteAlheio403(t *testing.T) {
	app, _, _, _, sCom, _ := setupExclusaoSetor(t)

	hashB, _ := hashSenha("senha-gerente")
	var uID int64
	if e := app.st.db.QueryRow(`SELECT id FROM usuarios WHERE login = 'ger_bravo'`).Scan(&uID); e != nil {
		t.Fatalf("gerente rival: %v", e)
	}
	tok, _, err := app.st.CriarSessao(uID, ttlSessao)
	if err != nil {
		t.Fatalf("sessão rival: %v", err)
	}
	ckB := &http.Cookie{Name: cookieSessao, Value: tok}
	_ = hashB

	rr, res := doJSONReq(app, "DELETE", "/api/setores/"+itoa64(sCom), nil, ckB)
	if rr.Code != http.StatusForbidden && rr.Code != http.StatusNotFound {
		t.Fatalf("esperava 403 (ou 404), veio %d: %v", rr.Code, res)
	}
}

func itoa64(n int64) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (func() bool {
		for i := 0; i+len(sub) <= len(s); i++ {
			if s[i:i+len(sub)] == sub {
				return true
			}
		}
		return false
	})()
}
