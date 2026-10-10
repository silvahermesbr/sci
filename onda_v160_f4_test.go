package main

// onda_v160_f4_test.go — v1.6.0 Fase 4: EXTINÇÃO DO OPERADOR DE GRUPO.
// Operador é de SETOR: a criação (hUsuariosAdd) e a atribuição de papel
// (hUsuarioPapelAdd) EXIGEM setor válido no grupo; contas de operador sem
// setor (legado, SQL direto) ficam BLOQUEADAS nos módulos operacionais pela
// guarda central exigeSetorOperador (403 "conta sem setor atribuído") até
// atribuição — sem auto-adivinhação. Leituras de conferência (hoje) ficam
// FORA (corte vazio já protege); mensagens/mural/drive fora (comunicação
// por papel — interpretação registrada).
//
// Cenários (persona: setupTestApp + loginAs + doJSONReq; positivo E negativo):
//   C1. criação de operador sem setor → 400; setor de OUTRO grupo → 400;
//       com setor do grupo → 200 e usuarios.setor_id gravado (admin/enc);
//   C2. chefe_setor cria operador: herança do setor do chefe permanece
//       (com setor → herda; sem setor no cadastro → 400);
//   C3. hUsuarioPapelAdd: operador sem setor → 400; setor alheio → 400;
//       com setor → 200 + cadastro gravado (SEM linha em chefe_setores);
//       re-atribuição = TROCA de setor idempotente (200, cadastro atualiza);
//   C4. operador legado SEM setor (SQL direto): login 200; marcar → 403 com
//       a mensagem da guarda; material itens → 403; escalas pdf → 403;
//       relatório → 403; leitura /conferencia/hoje → 200 (fora da guarda);
//   C5. reparo: PATCH de setor via hUsuarioEdit devolve a conta ao operacional
//       (mesma sessão — UsuarioDaSessao cai no cadastro).

import (
	"net/http"
	"strings"
	"testing"
)

// f4Setup: grupo principal + grupo fora + setores em ambos + gerente do
// grupo + designado na cadeira enc_pessoal (contexto enc após v45).
func f4Setup(t *testing.T, app *App, st *Store, sfx string) (gid, gidFora, setorA, setorB, setorFora int64, ckGer, ckEnc *http.Cookie) {
	t.Helper()
	if err := st.db.QueryRow(`INSERT INTO grupos (nome) VALUES (?) RETURNING id`, "Grp F4 "+sfx).Scan(&gid); err != nil {
		t.Fatalf("criar grupo: %v", err)
	}
	if err := st.db.QueryRow(`INSERT INTO grupos (nome) VALUES (?) RETURNING id`, "Grp F4 Fora "+sfx).Scan(&gidFora); err != nil {
		t.Fatalf("criar grupo fora: %v", err)
	}
	if err := st.db.QueryRow(`INSERT INTO setores (nome, grupo_id, ativo) VALUES (?,?,1) RETURNING id`, "S1 "+sfx, gid).Scan(&setorA); err != nil {
		t.Fatalf("criar setor A: %v", err)
	}
	if err := st.db.QueryRow(`INSERT INTO setores (nome, grupo_id, ativo) VALUES (?,?,1) RETURNING id`, "S2 "+sfx, gid).Scan(&setorB); err != nil {
		t.Fatalf("criar setor B: %v", err)
	}
	if err := st.db.QueryRow(`INSERT INTO setores (nome, grupo_id, ativo) VALUES (?,?,1) RETURNING id`, "SFora "+sfx, gidFora).Scan(&setorFora); err != nil {
		t.Fatalf("criar setor fora: %v", err)
	}

	criaUsuarioTeste(t, st, "f4_ger_"+sfx, "senha-ger", "gerente")
	if _, err := st.db.Exec(`UPDATE usuarios SET grupo_id = ? WHERE login = 'f4_ger_`+sfx+`'`, gid); err != nil {
		t.Fatalf("vincular gerente: %v", err)
	}
	ckGer = loginAs(t, app, "f4_ger_"+sfx, "senha-ger")

	// designado na cadeira enc_pessoal (contexto enc após materialização v45)
	var fPess int64
	if err := st.db.QueryRow(`SELECT id FROM funcoes WHERE chave = 'enc_pessoal'`).Scan(&fPess); err != nil {
		t.Fatalf("cadeira enc_pessoal ausente: %v", err)
	}
	criaUsuarioTeste(t, st, "f4_enc_"+sfx, "senha-enc", "")
	if _, err := st.db.Exec(`UPDATE usuarios SET grupo_id = ? WHERE login = 'f4_enc_`+sfx+`'`, gid); err != nil {
		t.Fatalf("vincular enc: %v", err)
	}
	var idEnc int64
	if err := st.db.QueryRow(`SELECT id FROM usuarios WHERE login = 'f4_enc_`+sfx+`'`).Scan(&idEnc); err != nil {
		t.Fatalf("id enc: %v", err)
	}
	if _, err := st.db.Exec(`INSERT INTO funcao_membros (funcao_id, grupo_id, usuario_id, titularidade) VALUES (?,?,?,'titular')`, fPess, gid, idEnc); err != nil {
		t.Fatalf("designar enc: %v", err)
	}
	v45Reexecuta(t, st)
	ckEnc = loginAs(t, app, "f4_enc_"+sfx, "senha-enc")
	return
}

// (C1) criação de operador EXIGE setor do grupo — admin e encarregado.
func TestF4OperadorExigeSetorNaCriacao(t *testing.T) {
	app, st, cleanup := setupTestApp(t)
	defer cleanup()
	gid, _, setorA, _, setorFora, _, ckEnc := f4Setup(t, app, st, "c1")
	ckAdm := loginAs(t, app, "admin", "admin123")

	// ADMIN: operador sem setor → 400 com a mensagem da doutrina
	rr, res := doJSONReq(app, "POST", "/api/usuarios", map[string]any{
		"login": "f4.op.a1", "senha": "SenhaF4a1", "papel": "operador", "grupo_id": gid}, ckAdm)
	if rr.Code != http.StatusBadRequest || !strings.Contains(rr.Body.String(), "operador deve nascer vinculado a um setor do grupo") {
		t.Fatalf("C1 admin sem setor: quer 400 'operador deve nascer…', veio %d (%v)", rr.Code, res["erro"])
	}
	// ADMIN: setor de OUTRO grupo → 400 (setor não pertence ao grupo)
	rr, _ = doJSONReq(app, "POST", "/api/usuarios", map[string]any{
		"login": "f4.op.a2", "senha": "SenhaF4a2", "papel": "operador", "grupo_id": gid, "setor_id": setorFora}, ckAdm)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("C1 admin setor alheio: quer 400, veio %d (%s)", rr.Code, rr.Body.String())
	}
	// ADMIN: com setor do grupo → 200 e cadastro gravado
	rr, res = doJSONReq(app, "POST", "/api/usuarios", map[string]any{
		"login": "f4.op.a3", "senha": "SenhaF4a3", "papel": "operador", "grupo_id": gid, "setor_id": setorA}, ckAdm)
	if rr.Code != http.StatusOK {
		t.Fatalf("C1 admin com setor: quer 200, veio %d (%v)", rr.Code, res["erro"])
	}
	var sid *int64
	if err := st.db.QueryRow(`SELECT setor_id FROM usuarios WHERE login = 'f4.op.a3'`).Scan(&sid); err != nil || sid == nil || *sid != setorA {
		t.Fatalf("C1: setor_id do operador novo devia ser %d, veio (%v, %v)", setorA, sid, err)
	}

	// ENCARREGADO de pessoal (contexto enc): sem setor → 400; com setor → 200
	rr, _ = doJSONReq(app, "POST", "/api/usuarios", map[string]any{"login": "f4.op.e1", "papel": "operador"}, ckEnc)
	if rr.Code != http.StatusBadRequest || !strings.Contains(rr.Body.String(), "operador deve nascer vinculado a um setor do grupo") {
		t.Fatalf("C1 enc sem setor: quer 400 'operador deve nascer…', veio %d (%s)", rr.Code, rr.Body.String())
	}
	rr, _ = doJSONReq(app, "POST", "/api/usuarios", map[string]any{"login": "f4.op.e2", "papel": "operador", "setor_id": setorA}, ckEnc)
	if rr.Code != http.StatusOK {
		t.Fatalf("C1 enc com setor: quer 200, veio %d (%s)", rr.Code, rr.Body.String())
	}
	var gidNovo int64
	if err := st.db.QueryRow(`SELECT COALESCE(grupo_id,0) FROM usuarios WHERE login = 'f4.op.e2'`).Scan(&gidNovo); err != nil || gidNovo != gid {
		t.Fatalf("C1: operador do enc devia nascer no grupo do enc (%d), veio %d (%v)", gid, gidNovo, err)
	}
}

// (C2) herança do ramo do chefe permanece: chefe com setor no cadastro cria
// operador que herda; chefe sem setor no cadastro → 400 (não há de onde herdar
// e o operador não nasce mais sem setor).
func TestF4ChefeHerdandoSetor(t *testing.T) {
	app, st, cleanup := setupTestApp(t)
	defer cleanup()
	gid, _, setorA, _, _, _, _ := f4Setup(t, app, st, "c2")

	// chefe COM setor no cadastro (padrão itens79: papel + setor materializam comando)
	criaUsuarioTeste(t, st, "f4_chefe_c2", "senha-chefe", "chefe_setor")
	if _, err := st.db.Exec(`UPDATE usuarios SET grupo_id = ?, setor_id = ? WHERE login = 'f4_chefe_c2'`, gid, setorA); err != nil {
		t.Fatalf("vincular chefe: %v", err)
	}
	var idChefe int64
	if err := st.db.QueryRow(`SELECT id FROM usuarios WHERE login = 'f4_chefe_c2'`).Scan(&idChefe); err != nil {
		t.Fatalf("id chefe: %v", err)
	}
	ckChefe := loginAs(t, app, "f4_chefe_c2", "senha-chefe")

	// herança: sem setor_id no corpo → herda o do chefe
	rr, _ := doJSONReq(app, "POST", "/api/usuarios", map[string]any{"login": "f4.op.h1", "papel": "operador"}, ckChefe)
	if rr.Code != http.StatusOK {
		t.Fatalf("C2 herança: quer 200, veio %d (%s)", rr.Code, rr.Body.String())
	}
	var sid *int64
	if err := st.db.QueryRow(`SELECT setor_id FROM usuarios WHERE login = 'f4.op.h1'`).Scan(&sid); err != nil || sid == nil || *sid != setorA {
		t.Fatalf("C2: operador devia HERDAR o setor do chefe (%d), veio (%v, %v)", setorA, sid, err)
	}

	// chefe SEM setor no cadastro: herança não produz setor → 400
	criaUsuarioTeste(t, st, "f4_chefe_ns", "senha-chefe2", "chefe_setor")
	if _, err := st.db.Exec(`UPDATE usuarios SET grupo_id = ?, setor_id = NULL WHERE login = 'f4_chefe_ns'`, gid); err != nil {
		t.Fatalf("vincular chefe sem setor: %v", err)
	}
	ckChefeNs := loginAs(t, app, "f4_chefe_ns", "senha-chefe2")
	rr, _ = doJSONReq(app, "POST", "/api/usuarios", map[string]any{"login": "f4.op.h2", "papel": "operador"}, ckChefeNs)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("C2 chefe sem setor: quer 400, veio %d (%s)", rr.Code, rr.Body.String())
	}

	// idChefe em uso para evitar "declared and not used" em builds futuros
	_ = idChefe
}

// (C3) hUsuarioPapelAdd: operador trata setor COMO o ramo do chefe — exige
// setor válido no grupo e grava usuarios.setor_id; re-atribuição troca o setor;
// NADA de comando em chefe_setores.
func TestF4OperadorExigeSetorNoPapelAdd(t *testing.T) {
	app, st, cleanup := setupTestApp(t)
	defer cleanup()
	gid, gidFora, setorA, setorB, setorFora, ckGer, ckEnc := f4Setup(t, app, st, "c3")

	criaUsuarioTeste(t, st, "f4_alvo_c3", "senha-alvo", "")
	if _, err := st.db.Exec(`UPDATE usuarios SET grupo_id = ? WHERE login = 'f4_alvo_c3'`, gid); err != nil {
		t.Fatalf("vincular alvo: %v", err)
	}
	var idAlvo int64
	if err := st.db.QueryRow(`SELECT id FROM usuarios WHERE login = 'f4_alvo_c3'`).Scan(&idAlvo); err != nil {
		t.Fatalf("id alvo: %v", err)
	}

	// GERENTE: sem setor → 400
	rr, _ := doJSONReq(app, "POST", "/api/usuarios/"+idi(idAlvo)+"/papeis", map[string]any{"papel": "operador", "grupo_id": gid}, ckGer)
	if rr.Code != http.StatusBadRequest || !strings.Contains(rr.Body.String(), "operador deve nascer vinculado a um setor do grupo") {
		t.Fatalf("C3 gerente sem setor: quer 400 'operador deve nascer…', veio %d (%s)", rr.Code, rr.Body.String())
	}
	// GERENTE: setor de outro grupo → 400
	rr, _ = doJSONReq(app, "POST", "/api/usuarios/"+idi(idAlvo)+"/papeis", map[string]any{"papel": "operador", "grupo_id": gid, "setor_id": setorFora}, ckGer)
	if rr.Code != http.StatusBadRequest || !strings.Contains(rr.Body.String(), "setor não pertence ao grupo do papel") {
		t.Fatalf("C3 gerente setor alheio: quer 400 'setor não pertence…', veio %d (%s)", rr.Code, rr.Body.String())
	}
	// GERENTE: com setor do grupo → 200 e CADASTRO gravado
	rr, _ = doJSONReq(app, "POST", "/api/usuarios/"+idi(idAlvo)+"/papeis", map[string]any{"papel": "operador", "grupo_id": gid, "setor_id": setorA}, ckGer)
	if rr.Code != http.StatusOK {
		t.Fatalf("C3 gerente com setor: quer 200, veio %d (%s)", rr.Code, rr.Body.String())
	}
	var sid *int64
	if err := st.db.QueryRow(`SELECT setor_id FROM usuarios WHERE id = ?`, idAlvo).Scan(&sid); err != nil || sid == nil || *sid != setorA {
		t.Fatalf("C3: usuarios.setor_id devia ser %d, veio (%v, %v)", setorA, sid, err)
	}
	// NENHUM comando fabricado (operador não comanda)
	var nCmd int
	if err := st.db.QueryRow(`SELECT COUNT(*) FROM chefe_setores WHERE usuario_id = ?`, idAlvo).Scan(&nCmd); err != nil || nCmd != 0 {
		t.Fatalf("C3: operador não pode ganhar linha em chefe_setores (n=%d, %v)", nCmd, err)
	}

	// re-atribuição (UNIQUE usuario+grupo+papel) = TROCA de setor idempotente
	rr, _ = doJSONReq(app, "POST", "/api/usuarios/"+idi(idAlvo)+"/papeis", map[string]any{"papel": "operador", "grupo_id": gid, "setor_id": setorB}, ckGer)
	if rr.Code != http.StatusOK {
		t.Fatalf("C3 troca de setor: quer 200, veio %d (%s)", rr.Code, rr.Body.String())
	}
	if err := st.db.QueryRow(`SELECT setor_id FROM usuarios WHERE id = ?`, idAlvo).Scan(&sid); err != nil || sid == nil || *sid != setorB {
		t.Fatalf("C3: troca de setor devia gravar %d, veio (%v, %v)", setorB, sid, err)
	}

	// ENCARREGADO (contexto enc): sem setor → 400; com setor → 200
	criaUsuarioTeste(t, st, "f4_alvo2_c3", "senha-alvo2", "")
	if _, err := st.db.Exec(`UPDATE usuarios SET grupo_id = ? WHERE login = 'f4_alvo2_c3'`, gid); err != nil {
		t.Fatalf("vincular alvo2: %v", err)
	}
	var idAlvo2 int64
	if err := st.db.QueryRow(`SELECT id FROM usuarios WHERE login = 'f4_alvo2_c3'`).Scan(&idAlvo2); err != nil {
		t.Fatalf("id alvo2: %v", err)
	}
	rr, _ = doJSONReq(app, "POST", "/api/usuarios/"+idi(idAlvo2)+"/papeis", map[string]any{"papel": "operador", "grupo_id": gid}, ckEnc)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("C3 enc sem setor: quer 400, veio %d (%s)", rr.Code, rr.Body.String())
	}
	rr, _ = doJSONReq(app, "POST", "/api/usuarios/"+idi(idAlvo2)+"/papeis", map[string]any{"papel": "operador", "grupo_id": gid, "setor_id": setorA}, ckEnc)
	if rr.Code != http.StatusOK {
		t.Fatalf("C3 enc com setor: quer 200, veio %d (%s)", rr.Code, rr.Body.String())
	}
	_ = gidFora
}

// (C4) operador legado SEM setor (SQL direto): loga, mas os módulos
// operacionais negam com a mensagem da guarda; leitura /conferencia/hoje segue
// 200 (fora da guarda — corte vazio).
func TestF4OperadorSemSetorBloqueado(t *testing.T) {
	app, st, cleanup := setupTestApp(t)
	defer cleanup()
	gid, _, _, _, _, _, _ := f4Setup(t, app, st, "c4")

	criaUsuarioTeste(t, st, "f4_op_semsetor", "senha-op", "operador")
	if _, err := st.db.Exec(`UPDATE usuarios SET grupo_id = ? WHERE login = 'f4_op_semsetor'`, gid); err != nil {
		t.Fatalf("vincular operador legado: %v", err)
	}
	ckOp := loginAs(t, app, "f4_op_semsetor", "senha-op")
	if rr, _ := doJSONReq(app, "GET", "/api/me", nil, ckOp); rr.Code != http.StatusOK {
		t.Fatalf("C4: login/me do operador sem setor devia ser 200, veio %d", rr.Code)
	}

	// marcar (confMarcarAuth → authConfCom) → 403 com a mensagem da guarda
	rr, _ := doJSONReq(app, "POST", "/api/conferencia/marcar", map[string]any{"pessoa_id": 1, "situacao": "falta"}, ckOp)
	if rr.Code != http.StatusForbidden || !strings.Contains(rr.Body.String(), "conta sem setor atribuído — solicite ao gerente/encarregado") {
		t.Fatalf("C4 marcar: quer 403 'conta sem setor atribuído…', veio %d (%s)", rr.Code, rr.Body.String())
	}
	// material itens (authMaterial) → 403
	rr, _ = doJSONReq(app, "GET", "/api/material/itens", nil, ckOp)
	if rr.Code != http.StatusForbidden || !strings.Contains(rr.Body.String(), "conta sem setor atribuído") {
		t.Fatalf("C4 material itens: quer 403 'conta sem setor atribuído', veio %d (%s)", rr.Code, rr.Body.String())
	}
	// escalas pdf (reservaAuth) → 403
	rr, _ = doJSONReq(app, "GET", "/api/escalas/relatorio-dia.pdf?de=2026-01-01&ate=2026-01-31", nil, ckOp)
	if rr.Code != http.StatusForbidden || !strings.Contains(rr.Body.String(), "conta sem setor atribuído") {
		t.Fatalf("C4 escalas pdf: quer 403 'conta sem setor atribuído', veio %d (%s)", rr.Code, rr.Body.String())
	}
	// relatório (handler de dados) → 403
	rr, _ = doJSONReq(app, "GET", "/api/relatorio", nil, ckOp)
	if rr.Code != http.StatusForbidden || !strings.Contains(rr.Body.String(), "conta sem setor atribuído") {
		t.Fatalf("C4 relatório: quer 403 'conta sem setor atribuído', veio %d (%s)", rr.Code, rr.Body.String())
	}
	// leitura de conferência FORA da guarda: /hoje segue 200 (corte vazio)
	rr, _ = doJSONReq(app, "GET", "/api/conferencia/hoje", nil, ckOp)
	if rr.Code != http.StatusOK {
		t.Fatalf("C4: leitura /conferencia/hoje devia seguir 200, veio %d (%s)", rr.Code, rr.Body.String())
	}
}

// (C5) reparo: PATCH de setor via hUsuarioEdit devolve a conta ao operacional —
// a MESMA sessão volta a passar (UsuarioDaSessao cai no cadastro quando o
// setor ativo da sessão é NULL).
func TestF4ReparoViaPatchSetor(t *testing.T) {
	app, st, cleanup := setupTestApp(t)
	defer cleanup()
	gid, _, setorA, _, _, ckGer, _ := f4Setup(t, app, st, "c5")

	criaUsuarioTeste(t, st, "f4_op_c5", "senha-op", "operador")
	if _, err := st.db.Exec(`UPDATE usuarios SET grupo_id = ? WHERE login = 'f4_op_c5'`, gid); err != nil {
		t.Fatalf("vincular operador: %v", err)
	}
	var idOp int64
	if err := st.db.QueryRow(`SELECT id FROM usuarios WHERE login = 'f4_op_c5'`).Scan(&idOp); err != nil {
		t.Fatalf("id operador: %v", err)
	}
	ckOp := loginAs(t, app, "f4_op_c5", "senha-op")

	if rr, _ := doJSONReq(app, "GET", "/api/material/itens", nil, ckOp); rr.Code != http.StatusForbidden {
		t.Fatalf("C5 pré-reparo: material devia negar 403, veio %d", rr.Code)
	}

	// GERENTE dá o setor (hUsuarioEdit — alvo operador do próprio grupo)
	rr, _ := doJSONReq(app, "PATCH", "/api/usuarios/"+idi(idOp), map[string]any{"setor_id": setorA}, ckGer)
	if rr.Code != http.StatusOK {
		t.Fatalf("C5 PATCH setor: quer 200, veio %d (%s)", rr.Code, rr.Body.String())
	}
	if rr, _ := doJSONReq(app, "GET", "/api/material/itens", nil, ckOp); rr.Code != http.StatusOK {
		t.Fatalf("C5 pós-reparo: material devia passar 200, veio %d (%s)", rr.Code, rr.Body.String())
	}
	if rr, _ := doJSONReq(app, "GET", "/api/relatorio", nil, ckOp); rr.Code != http.StatusOK {
		t.Fatalf("C5 pós-reparo: relatório devia passar 200, veio %d (%s)", rr.Code, rr.Body.String())
	}
}
