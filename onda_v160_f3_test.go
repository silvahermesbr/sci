package main

// onda_v160_f3_test.go — v1.6.0 Fase 3: a designação de cadeira SINCRONIZA a
// linha de papel (set/del) com re-key de sessão. Doutrina D1 da cadeira:
// funcao_membros é a DESIGNAÇÃO; a linha em usuario_papeis é o ACESSO/CONTEXTO
// — o dropdown só lê usuario_papeis e os poderes seguem o contexto ativo
// (Fase 2). Sem o sync, a designação nova só valeria após a próxima migração.
//
// Cenários (persona: setupTestApp + loginAs + doJSONReq; positivo E negativo):
//   S1. designar via API MATERIALIZA a linha com funcao_id da cadeira e o
//       grupo da designação — e o login do designado resolve o contexto;
//   S2. remover apaga a linha e RE-CHAVEIA a sessão ATIVA (prova: GET /api/me
//       muda o papel na hora; designado puro volta a sem-contexto) e
//       re-designar depois de remover reabre a linha (OR IGNORE dedup);
//   S3. titular + auxiliar na MESMA cadeira → 2 linhas; remover só o auxiliar
//       mantém a do titular (e o titular continua no contexto);
//   S4. operador com cadeira: remover o enc conserva a linha OPERADOR e a
//       sessão ativa re-chaveia para ela na hora.
//
// A designação é sempre pela UI (POST /api/grupo/funcoes/membros) e a remoção
// pelo DELETE /api/grupo/funcoes/membros/{id} — o caminho real do produto.

import (
	"net/http"
	"testing"
)

// f3Designa: designa pela API real e devolve o id da designação.
func f3Designa(t *testing.T, app *App, ck *http.Cookie, funcaoID, usuarioID int64, tit string) int64 {
	t.Helper()
	rr, res := doJSONReq(app, "POST", "/api/grupo/funcoes/membros", map[string]any{
		"funcao_id": funcaoID, "usuario_id": usuarioID, "titularidade": tit,
	}, ck)
	if rr.Code != http.StatusOK {
		t.Fatalf("designar via API: esperado 200, veio %d (%v)", rr.Code, res)
	}
	id, _ := res["id"].(float64)
	if id <= 0 {
		t.Fatalf("designar via API: id inválido: %v", res["id"])
	}
	return int64(id)
}

// f3LinhaCadeira: id da linha de papel da cadeira do usuário (papel+grupo).
func f3LinhaCadeira(t *testing.T, st *Store, usuarioID, grupoID int64, papel string) int64 {
	t.Helper()
	var id int64
	if err := st.db.QueryRow(`SELECT id FROM usuario_papeis WHERE usuario_id = ? AND grupo_id = ? AND papel = ?`, usuarioID, grupoID, papel).Scan(&id); err != nil {
		t.Fatalf("linha %s do usuário %d no grupo %d: %v", papel, usuarioID, grupoID, err)
	}
	return id
}

// f3ContaLinhas: COUNT(*) com query e args (leitura isolada no pool=1).
func f3ContaLinhas(t *testing.T, st *Store, query string, args ...any) int {
	t.Helper()
	var n int
	if err := st.db.QueryRow(query, args...).Scan(&n); err != nil {
		t.Fatalf("contar (%s): %v", query, err)
	}
	return n
}

// (S1) designar via API materializa a linha da cadeira com funcao_id certo.
func TestF3DesignacaoMaterializaLinhaDaCadeira(t *testing.T) {
	app, st, cleanup := setupTestApp(t)
	defer cleanup()
	gid, fPess, _, _, ckGer := f2SetupGrupo(t, app, st, "s1")

	criaUsuarioTeste(t, st, "f3_membro_s1", "senha-m", "")
	if _, err := st.db.Exec(`UPDATE usuarios SET grupo_id = ? WHERE login = 'f3_membro_s1'`, gid); err != nil {
		t.Fatalf("vincular membro: %v", err)
	}
	var idM int64
	if err := st.db.QueryRow(`SELECT id FROM usuarios WHERE login = 'f3_membro_s1'`).Scan(&idM); err != nil {
		t.Fatalf("id membro: %v", err)
	}

	f3Designa(t, app, ckGer, fPess, idM, "titular")

	// a linha materializada carrega o PAPEL da cadeira, o GRUPO da designação
	// e o funcao_id da CADEIRA (rótulo do dropdown)
	var papel string
	var gidLinha, fidLinha int64
	if err := st.db.QueryRow(`SELECT papel, grupo_id, COALESCE(funcao_id,0) FROM usuario_papeis WHERE usuario_id = ?`, idM).Scan(&papel, &gidLinha, &fidLinha); err != nil {
		t.Fatalf("linha materializada ausente: %v", err)
	}
	if papel != "enc_pessoal" || gidLinha != gid || fidLinha != fPess {
		t.Fatalf("S1: linha devia ser (enc_pessoal, gid=%d, funcao_id=%d), veio (%q, %d, %d)", gid, fPess, papel, gidLinha, fidLinha)
	}
	// NADA de papel de sistema inventado
	if n := f3ContaLinhas(t, st, `SELECT COUNT(*) FROM usuario_papeis WHERE usuario_id = ? AND papel NOT IN ('enc_pessoal','enc_material')`, idM); n != 0 {
		t.Fatalf("S1: sync inventou linha de sistema (n=%d)", n)
	}

	// o designado puro que LOGA depois já nasce NO CONTEXTO da cadeira
	ck := loginAs(t, app, "f3_membro_s1", "senha-m")
	if p := f2MePapel(t, app, ck); p != "enc_pessoal" {
		t.Fatalf("S1: designado devia logar no CONTEXTO enc_pessoal, veio %q", p)
	}
	// e o dropdown (/api/me papeis) lista a linha com o rótulo da cadeira
	_, resMe := doJSONReq(app, "GET", "/api/me", nil, ck)
	uMap, _ := resMe["usuario"].(map[string]any)
	if uMap == nil {
		t.Fatalf("S1: /api/me sem usuario: %v", resMe)
	}
	papeis, _ := uMap["papeis"].([]any)
	achou := false
	for _, p := range papeis {
		if pm, _ := p.(map[string]any); pm != nil && pm["papel"] == "enc_pessoal" {
			achou = true
		}
	}
	if !achou {
		t.Fatalf("S1: dropdown devia listar a linha enc_pessoal: %v", papeis)
	}
}

// (S2) remover apaga a linha e RE-CHAVEIA a sessão ATIVA — o /api/me muda o
// papel NA HORA; re-designar depois de remover reabre a linha.
func TestF3RemocaoApagaLinhaEReChaveiaSessaoAtiva(t *testing.T) {
	app, st, cleanup := setupTestApp(t)
	defer cleanup()
	gid, fPess, _, _, ckGer := f2SetupGrupo(t, app, st, "s2")

	criaUsuarioTeste(t, st, "f3_membro_s2", "senha-m", "")
	if _, err := st.db.Exec(`UPDATE usuarios SET grupo_id = ? WHERE login = 'f3_membro_s2'`, gid); err != nil {
		t.Fatalf("vincular membro: %v", err)
	}
	var idM int64
	if err := st.db.QueryRow(`SELECT id FROM usuarios WHERE login = 'f3_membro_s2'`).Scan(&idM); err != nil {
		t.Fatalf("id membro: %v", err)
	}

	// 1ª designação → linha + contexto
	memID := f3Designa(t, app, ckGer, fPess, idM, "titular")
	ck := loginAs(t, app, "f3_membro_s2", "senha-m")
	if p := f2MePapel(t, app, ck); p != "enc_pessoal" {
		t.Fatalf("S2 cenário: designado devia estar no contexto enc_pessoal, veio %q", p)
	}

	// remover a designação → a linha sai E a SESSÃO ATIVA re-chaveia (NULL:
	// era a única linha — o designado puro volta a sem-contexto NA HORA)
	if rr, _ := doJSONReq(app, "DELETE", "/api/grupo/funcoes/membros/"+idi(memID), nil, ckGer); rr.Code != http.StatusOK {
		t.Fatalf("S2: remover designação deve 200, veio %d", rr.Code)
	}
	if n := f3ContaLinhas(t, st, `SELECT COUNT(*) FROM usuario_papeis WHERE usuario_id = ? AND papel = 'enc_pessoal'`, idM); n != 0 {
		t.Fatalf("S2: linha da cadeira devia ter sido apagada (n=%d)", n)
	}
	if p := f2MePapel(t, app, ck); p != "" {
		t.Fatalf("S2: sessão ATIVA devia voltar a sem-contexto na hora (papel ''), veio %q", p)
	}
	// e o poder saiu junto (Fase 2: sem contexto, sem poder)
	if rr, _ := doJSONReq(app, "POST", "/api/pessoas", map[string]any{"nome_guerra": "F3SEMCTX", "nome_completo": "Sem Contexto"}, ck); rr.Code != http.StatusForbidden {
		t.Fatalf("S2: sem contexto, POST /api/pessoas deve 403, veio %d", rr.Code)
	}

	// re-designar → a linha volta (OR IGNORE deduplica; nada duplicado)
	f3Designa(t, app, ckGer, fPess, idM, "titular")
	if n := f3ContaLinhas(t, st, `SELECT COUNT(*) FROM usuario_papeis WHERE usuario_id = ? AND papel = 'enc_pessoal'`, idM); n != 1 {
		t.Fatalf("S2: re-designação devia reabrir EXATAMENTE 1 linha (n=%d)", n)
	}
	if p := f2MePapel(t, app, ck); p != "" {
		t.Fatalf("S2: re-designação NÃO re-chaveia sessão (vale no próximo login/troca) — papel segue '', veio %q", p)
	}
	if p := f2MePapel(t, app, func() *http.Cookie { ck2 := loginAs(t, app, "f3_membro_s2", "senha-m"); return ck2 }()); p != "enc_pessoal" {
		t.Fatalf("S2: re-login devia resolver o contexto de novo, veio %q", p)
	}
}

// (S3) titular + auxiliar na MESMA cadeira → 2 linhas; remover só o auxiliar
// mantém a do titular (e o titular segue no contexto).
func TestF3TitularEAuxiliarViramDuasLinhas(t *testing.T) {
	app, st, cleanup := setupTestApp(t)
	defer cleanup()
	gid, fPess, _, _, ckGer := f2SetupGrupo(t, app, st, "s3")

	criaUsuarioTeste(t, st, "f3_tit_s3", "senha-t", "")
	criaUsuarioTeste(t, st, "f3_aux_s3", "senha-a", "")
	if _, err := st.db.Exec(`UPDATE usuarios SET grupo_id = ? WHERE login IN ('f3_tit_s3','f3_aux_s3')`, gid); err != nil {
		t.Fatalf("vincular contas: %v", err)
	}
	var idTit, idAux int64
	if err := st.db.QueryRow(`SELECT id FROM usuarios WHERE login = 'f3_tit_s3'`).Scan(&idTit); err != nil {
		t.Fatalf("id titular: %v", err)
	}
	if err := st.db.QueryRow(`SELECT id FROM usuarios WHERE login = 'f3_aux_s3'`).Scan(&idAux); err != nil {
		t.Fatalf("id auxiliar: %v", err)
	}

	f3Designa(t, app, ckGer, fPess, idTit, "titular")
	f3Designa(t, app, ckGer, fPess, idAux, "auxiliar")

	if n := f3ContaLinhas(t, st, `SELECT COUNT(*) FROM usuario_papeis WHERE grupo_id = ? AND papel = 'enc_pessoal'`, gid); n != 2 {
		t.Fatalf("S3: titular+auxiliar deviam materializar 2 linhas (n=%d)", n)
	}

	// remover SÓ o auxiliar: a linha dele sai, a do titular fica
	auxMem := f3DesignaID(t, st, fPess, gid, idAux)
	if rr, _ := doJSONReq(app, "DELETE", "/api/grupo/funcoes/membros/"+idi(auxMem), nil, ckGer); rr.Code != http.StatusOK {
		t.Fatalf("S3: remover auxiliar deve 200, veio %d", rr.Code)
	}
	if n := f3ContaLinhas(t, st, `SELECT COUNT(*) FROM usuario_papeis WHERE usuario_id = ? AND papel = 'enc_pessoal'`, idAux); n != 0 {
		t.Fatalf("S3: linha do auxiliar devia ter sido apagada (n=%d)", n)
	}
	if n := f3ContaLinhas(t, st, `SELECT COUNT(*) FROM usuario_papeis WHERE usuario_id = ? AND papel = 'enc_pessoal'`, idTit); n != 1 {
		t.Fatalf("S3: linha do TITULAR devia permanecer (n=%d)", n)
	}
	// titular logado continua resolvendo o contexto
	ckTit := loginAs(t, app, "f3_tit_s3", "senha-t")
	if p := f2MePapel(t, app, ckTit); p != "enc_pessoal" {
		t.Fatalf("S3: titular devia continuar no CONTEXTO enc_pessoal, veio %q", p)
	}
}

// (S4) operador com cadeira: remover o enc conserva a linha OPERADOR e a
// sessão ativa re-chaveia para ela na hora (prova pelo /api/me).
func TestF3OperadorComCadeiraRemoveEncConservaOperador(t *testing.T) {
	app, st, cleanup := setupTestApp(t)
	defer cleanup()
	gid, _, fMat, _, ckGer := f2SetupGrupo(t, app, st, "s4")

	criaUsuarioTeste(t, st, "f3_op_s4", "senha-o", "operador")
	if _, err := st.db.Exec(`UPDATE usuarios SET grupo_id = ? WHERE login = 'f3_op_s4'`, gid); err != nil {
		t.Fatalf("vincular operador: %v", err)
	}
	var idOp int64
	if err := st.db.QueryRow(`SELECT id FROM usuarios WHERE login = 'f3_op_s4'`).Scan(&idOp); err != nil {
		t.Fatalf("id operador: %v", err)
	}
	// login ANTES da designação: a linha de sistema nasce primeiro (contexto default)
	ckOp := loginAs(t, app, "f3_op_s4", "senha-o")

	f3Designa(t, app, ckGer, fMat, idOp, "titular")
	if n := f3ContaLinhas(t, st, `SELECT COUNT(*) FROM usuario_papeis WHERE usuario_id = ?`, idOp); n != 2 {
		t.Fatalf("S4: operador+enc deviam coexistir (2 linhas), veio %d", n)
	}

	// assume o CONTEXTO enc_material (dropdown)
	f2TrocaContexto(t, app, ckOp, f3LinhaCadeira(t, st, idOp, gid, "enc_material"))
	if p := f2MePapel(t, app, ckOp); p != "enc_material" {
		t.Fatalf("S4 cenário: operador devia estar no contexto enc_material, veio %q", p)
	}

	// gerente remove a designação → linha enc sai, a OPERADOR fica e a sessão
	// ativa re-chaveia para ela NA HORA
	encMem := f3DesignaID(t, st, fMat, gid, idOp)
	if rr, _ := doJSONReq(app, "DELETE", "/api/grupo/funcoes/membros/"+idi(encMem), nil, ckGer); rr.Code != http.StatusOK {
		t.Fatalf("S4: remover designação deve 200, veio %d", rr.Code)
	}
	if n := f3ContaLinhas(t, st, `SELECT COUNT(*) FROM usuario_papeis WHERE usuario_id = ? AND papel = 'enc_material'`, idOp); n != 0 {
		t.Fatalf("S4: linha enc_material devia ter sido apagada (n=%d)", n)
	}
	if n := f3ContaLinhas(t, st, `SELECT COUNT(*) FROM usuario_papeis WHERE usuario_id = ? AND papel = 'operador'`, idOp); n != 1 {
		t.Fatalf("S4: linha OPERADOR devia ser conservada (n=%d)", n)
	}
	if p := f2MePapel(t, app, ckOp); p != "operador" {
		t.Fatalf("S4: sessão ativa devia re-chavear para 'operador' na hora, veio %q", p)
	}
}

// f3DesignaID: id da designação (funcao_membros) de um usuário numa cadeira.
func f3DesignaID(t *testing.T, st *Store, funcaoID, grupoID, usuarioID int64) int64 {
	t.Helper()
	var id int64
	if err := st.db.QueryRow(`SELECT id FROM funcao_membros WHERE funcao_id = ? AND grupo_id = ? AND usuario_id = ?`, funcaoID, grupoID, usuarioID).Scan(&id); err != nil {
		t.Fatalf("designação (f=%d g=%d u=%d) não encontrada: %v", funcaoID, grupoID, usuarioID, err)
	}
	return id
}
