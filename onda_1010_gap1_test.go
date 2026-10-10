package main

import (
	"fmt"
	"net/http"
	"testing"
)

// TestGap1_DesignacaoPeloEncarregado: validação da Onda 10/10 Frente A
// Encarregado de pessoal designa/remove membros na cadeira enc_material do próprio grupo,
// bloqueado na cadeira enc_pessoal (403), em usuários alheios (400/403), e regressões
// de operador (403) e gerente (200).
func TestGap1_DesignacaoPeloEncarregado(t *testing.T) {
	app, st, cleanup := setupTestApp(t)
	defer cleanup()

	// 1. Grupos (principal e alheio)
	var gid, gidAlheio int64
	if err := st.db.QueryRow(`INSERT INTO grupos (nome) VALUES ('Grp Gap1') RETURNING id`).Scan(&gid); err != nil {
		t.Fatalf("criar grupo: %v", err)
	}
	if err := st.db.QueryRow(`INSERT INTO grupos (nome) VALUES ('Grp Gap1 Alheio') RETURNING id`).Scan(&gidAlheio); err != nil {
		t.Fatalf("criar grupo alheio: %v", err)
	}

	// 2. Cadeiras fixas
	var fPess, fMat int64
	if err := st.db.QueryRow(`SELECT id FROM funcoes WHERE chave = 'enc_pessoal'`).Scan(&fPess); err != nil {
		t.Fatalf("obter funcao enc_pessoal: %v", err)
	}
	if err := st.db.QueryRow(`SELECT id FROM funcoes WHERE chave = 'enc_material'`).Scan(&fMat); err != nil {
		t.Fatalf("obter funcao enc_material: %v", err)
	}

	// 3. Usuários
	criaUsuarioTeste(t, st, "gap1_enc", "senha-enc", "")        // encarregado de pessoal: SEM papel
	criaUsuarioTeste(t, st, "gap1_membro", "senha-mem", "")     // usuário do próprio grupo: SEM papel
	criaUsuarioTeste(t, st, "gap1_alheio", "senha-alh", "")     // usuário de grupo alheio: SEM papel
	criaUsuarioTeste(t, st, "gap1_op", "senha-op", "operador")  // operador do grupo
	criaUsuarioTeste(t, st, "gap1_ger", "senha-ger", "gerente") // gerente do grupo

	var idEnc, idMembro, idAlheio, idOp, idGer int64
	if err := st.db.QueryRow(`SELECT id FROM usuarios WHERE login = 'gap1_enc'`).Scan(&idEnc); err != nil {
		t.Fatalf("id enc: %v", err)
	}
	if err := st.db.QueryRow(`SELECT id FROM usuarios WHERE login = 'gap1_membro'`).Scan(&idMembro); err != nil {
		t.Fatalf("id membro: %v", err)
	}
	if err := st.db.QueryRow(`SELECT id FROM usuarios WHERE login = 'gap1_alheio'`).Scan(&idAlheio); err != nil {
		t.Fatalf("id alheio: %v", err)
	}
	if err := st.db.QueryRow(`SELECT id FROM usuarios WHERE login = 'gap1_op'`).Scan(&idOp); err != nil {
		t.Fatalf("id op: %v", err)
	}
	if err := st.db.QueryRow(`SELECT id FROM usuarios WHERE login = 'gap1_ger'`).Scan(&idGer); err != nil {
		t.Fatalf("id ger: %v", err)
	}

	// 4. Vincular grupos (ANTES de qualquer login)
	if _, err := st.db.Exec(`UPDATE usuarios SET grupo_id = ? WHERE id IN (?, ?, ?, ?)`, gid, idEnc, idMembro, idOp, idGer); err != nil {
		t.Fatalf("vincular grupo principal: %v", err)
	}
	if _, err := st.db.Exec(`UPDATE usuarios SET grupo_id = ? WHERE id = ?`, gidAlheio, idAlheio); err != nil {
		t.Fatalf("vincular grupo alheio: %v", err)
	}

	// 5. Designação inicial do encarregado de pessoal (ANTES do login para congelar na sessão)
	var idDesigEncPess int64
	if err := st.db.QueryRow(`INSERT INTO funcao_membros (funcao_id, grupo_id, usuario_id, titularidade) VALUES (?, ?, ?, 'titular') RETURNING id`, fPess, gid, idEnc).Scan(&idDesigEncPess); err != nil {
		t.Fatalf("designar encarregado: %v", err)
	}

	// 6. Logins (ÚLTIMO passo de cada persona)
	ckEnc := loginAs(t, app, "gap1_enc", "senha-enc")
	ckOp := loginAs(t, app, "gap1_op", "senha-op")
	ckGer := loginAs(t, app, "gap1_ger", "senha-ger")

	// Caso 1: Encarregado designa usuário do próprio grupo na cadeira enc_material, titularidade "auxiliar" → 200
	rr1, res1 := doJSONReq(app, "POST", "/api/grupo/funcoes/membros", map[string]any{
		"funcao_id":    fMat,
		"usuario_id":   idMembro,
		"titularidade": "auxiliar",
	}, ckEnc)
	if rr1.Code != http.StatusOK {
		t.Fatalf("Caso 1 falhou: esperado 200, veio %d (%v)", rr1.Code, res1)
	}
	idDesigMat, ok := res1["id"].(float64)
	if !ok || int64(idDesigMat) <= 0 {
		t.Fatalf("Caso 1 falhou: id retornado inválido: %v", res1["id"])
	}

	// Caso 2: Encarregado remove essa designação (DELETE) → 200
	rr2, res2 := doJSONReq(app, "DELETE", fmt.Sprintf("/api/grupo/funcoes/membros/%d", int64(idDesigMat)), nil, ckEnc)
	if rr2.Code != http.StatusOK {
		t.Fatalf("Caso 2 falhou: esperado 200, veio %d (%v)", rr2.Code, res2)
	}
	var countMat int
	if err := st.db.QueryRow(`SELECT COUNT(*) FROM funcao_membros WHERE id = ?`, int64(idDesigMat)).Scan(&countMat); err != nil || countMat != 0 {
		t.Fatalf("Caso 2 falhou: registro %d ainda consta no banco (count=%d)", int64(idDesigMat), countMat)
	}

	// Caso 3: Encarregado tenta designar na cadeira enc_pessoal → 403
	rr3, res3 := doJSONReq(app, "POST", "/api/grupo/funcoes/membros", map[string]any{
		"funcao_id":    fPess,
		"usuario_id":   idMembro,
		"titularidade": "auxiliar",
	}, ckEnc)
	if rr3.Code != http.StatusForbidden {
		t.Fatalf("Caso 3 (POST enc_pessoal) falhou: esperado 403, veio %d (%v)", rr3.Code, res3)
	}
	// Encarregado também é bloqueado ao tentar remover cadeira enc_pessoal (a própria designação)
	rr3Del, res3Del := doJSONReq(app, "DELETE", fmt.Sprintf("/api/grupo/funcoes/membros/%d", idDesigEncPess), nil, ckEnc)
	if rr3Del.Code != http.StatusForbidden {
		t.Fatalf("Caso 3 (DELETE enc_pessoal) falhou: esperado 403, veio %d (%v)", rr3Del.Code, res3Del)
	}

	// Caso 4: Encarregado designa usuário de grupo alheio na enc_material → 400/403 (fora do escopo)
	rr4, res4 := doJSONReq(app, "POST", "/api/grupo/funcoes/membros", map[string]any{
		"funcao_id":    fMat,
		"usuario_id":   idAlheio,
		"titularidade": "auxiliar",
	}, ckEnc)
	if rr4.Code != http.StatusBadRequest && rr4.Code != http.StatusForbidden {
		t.Fatalf("Caso 4 falhou: esperado 400 ou 403, veio %d (%v)", rr4.Code, res4)
	}

	// Caso 5: Operador SEM cadeira enc_pessoal: POST e DELETE → 403 (regressão da guarda)
	rr5Post, res5Post := doJSONReq(app, "POST", "/api/grupo/funcoes/membros", map[string]any{
		"funcao_id":    fMat,
		"usuario_id":   idMembro,
		"titularidade": "auxiliar",
	}, ckOp)
	if rr5Post.Code != http.StatusForbidden {
		t.Fatalf("Caso 5 (POST) falhou: esperado 403, veio %d (%v)", rr5Post.Code, res5Post)
	}
	rr5Del, res5Del := doJSONReq(app, "DELETE", fmt.Sprintf("/api/grupo/funcoes/membros/%d", idDesigEncPess), nil, ckOp)
	if rr5Del.Code != http.StatusForbidden {
		t.Fatalf("Caso 5 (DELETE) falhou: esperado 403, veio %d (%v)", rr5Del.Code, res5Del)
	}

	// Caso 6: Gerente designa na cadeira enc_pessoal → 200 (regressão do caminho antigo)
	rr6, res6 := doJSONReq(app, "POST", "/api/grupo/funcoes/membros", map[string]any{
		"funcao_id":    fPess,
		"usuario_id":   idMembro,
		"titularidade": "auxiliar",
	}, ckGer)
	if rr6.Code != http.StatusOK {
		t.Fatalf("Caso 6 falhou: esperado 200, veio %d (%v)", rr6.Code, res6)
	}
	idDesigGer, okGer := res6["id"].(float64)
	if !okGer || int64(idDesigGer) <= 0 {
		t.Fatalf("Caso 6 falhou: id retornado inválido: %v", res6["id"])
	}

	// Encarregado tentando remover essa nova designação de enc_pessoal feita pelo gerente → 403
	rr6EncDel, res6EncDel := doJSONReq(app, "DELETE", fmt.Sprintf("/api/grupo/funcoes/membros/%d", int64(idDesigGer)), nil, ckEnc)
	if rr6EncDel.Code != http.StatusForbidden {
		t.Fatalf("Caso 6 (encarregado tenta DELETE enc_pessoal criado por gerente) falhou: esperado 403, veio %d (%v)", rr6EncDel.Code, res6EncDel)
	}

	// Gerente remove a designação na cadeira enc_pessoal → 200 (regressão)
	rr6GerDel, res6GerDel := doJSONReq(app, "DELETE", fmt.Sprintf("/api/grupo/funcoes/membros/%d", int64(idDesigGer)), nil, ckGer)
	if rr6GerDel.Code != http.StatusOK {
		t.Fatalf("Caso 6 (gerente DELETE) falhou: esperado 200, veio %d (%v)", rr6GerDel.Code, res6GerDel)
	}
}
