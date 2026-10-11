package main

// onda_v160_migracao_test.go — v1.6.0 Fase 1: migração v45 (cadeiras enc_*
// materializam em usuario_papeis como CONTEXTO — fundação da onda contextos).
//
// Padrão persona da casa (setupTestApp/loginAs/doJSONReq). Estado PRÉ-migração
// é semeado por SQL direto no store e a migração é re-executada removendo a
// marca de schema_migrations (padrão do TestD1MigracaoV44MaterializaLegado).
//
// Cenários:
//   a. rebuild de usuario_papeis preserva dados, ids e FKs (mensagens/
//      mensagem_destinatarios/avisos/aviso_cientes continuam resolvendo por JOIN);
//   b. titular + auxiliar designados na enc_material → 2 linhas com funcao_id da
//      cadeira; re-designação (remove + designa outro) pré-migração → a
//      materialização reflete o ESTADO FINAL;
//   c. sessão de designado puro (papel_ativo_id NULL) é re-keyada para a linha
//      enc e o /api/me passa a resolver o contexto novo (contrato do front);
//   d. idempotência (re-execução não duplica) + cadeia/binário em 45.

import (
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"strings"
	"testing"
	"time"
)

// v45SeedGrupo: grupo + cadeira enc_material do catálogo semeado pela v39.
func v45SeedGrupo(t *testing.T, st *Store, nome string) (gid, fMat, fPess int64) {
	t.Helper()
	if err := st.db.QueryRow(`INSERT INTO grupos (nome) VALUES (?) RETURNING id`, nome).Scan(&gid); err != nil {
		t.Fatalf("criar grupo %s: %v", nome, err)
	}
	if err := st.db.QueryRow(`SELECT id FROM funcoes WHERE chave = 'enc_material'`).Scan(&fMat); err != nil {
		t.Fatalf("cadeira enc_material ausente (seed v39): %v", err)
	}
	if err := st.db.QueryRow(`SELECT id FROM funcoes WHERE chave = 'enc_pessoal'`).Scan(&fPess); err != nil {
		t.Fatalf("cadeira enc_pessoal ausente (seed v39): %v", err)
	}
	return
}

// v45Conta: conta SEM linha em usuario_papeis e SEM papel de sistema
// (designado puro pré-v45 — o mundo que a migração encontra).
func v45Conta(t *testing.T, st *Store, login string, gid int64) int64 {
	t.Helper()
	criaUsuarioTeste(t, st, login, "senha-v45", "")
	if _, err := st.db.Exec(`UPDATE usuarios SET grupo_id = ? WHERE login = ?`, gid, login); err != nil {
		t.Fatalf("vincular grupo (%s): %v", login, err)
	}
	var uid int64
	if err := st.db.QueryRow(`SELECT id FROM usuarios WHERE login = ?`, login).Scan(&uid); err != nil {
		t.Fatalf("id de %s: %v", login, err)
	}
	return uid
}

// v45Desigina: designação direta na cadeira (estado pré-migração, sem handler).
func v45Designa(t *testing.T, st *Store, funcaoID, grupoID, usuarioID int64, titularidade string) {
	t.Helper()
	if _, err := st.db.Exec(`INSERT INTO funcao_membros (funcao_id, grupo_id, usuario_id, titularidade) VALUES (?,?,?,?)`,
		funcaoID, grupoID, usuarioID, titularidade); err != nil {
		t.Fatalf("designar %d em %d (%s): %v", usuarioID, funcaoID, titularidade, err)
	}
}

// v45Reexecuta: limpa a marca de versão e roda a migração de novo.
func v45Reexecuta(t *testing.T, st *Store) {
	t.Helper()
	if _, err := st.db.Exec(`DELETE FROM schema_migrations WHERE versao = 45`); err != nil {
		t.Fatalf("limpar marcador v45: %v", err)
	}
	if err := st.migrarV45(); err != nil {
		t.Fatalf("migrarV45: %v", err)
	}
}

// v45LinhasEnc: linhas materializadas (id, usuario_id, papel, funcao_id) do grupo.
func v45LinhasEnc(t *testing.T, st *Store, grupoID int64) map[int64]int64 {
	t.Helper()
	rows, err := st.db.Query(`SELECT id, COALESCE(funcao_id,0) FROM usuario_papeis WHERE grupo_id = ? AND papel IN ('enc_pessoal','enc_material')`, grupoID)
	if err != nil {
		t.Fatalf("ler usuario_papeis enc: %v", err)
	}
	defer rows.Close()
	m := map[int64]int64{}
	for rows.Next() {
		var id, fid int64
		if err := rows.Scan(&id, &fid); err != nil {
			t.Fatalf("scan usuario_papeis enc: %v", err)
		}
		m[id] = fid
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("rows usuario_papeis enc: %v", err)
	}
	return m
}

// v45Contagem: COUNT(*) de uma tabela (helper de leitura isolada no pool=1).
func v45Contagem(t *testing.T, st *Store, query string, args ...any) int {
	t.Helper()
	var n int
	if err := st.db.QueryRow(query, args...).Scan(&n); err != nil {
		t.Fatalf("contar (%s): %v", query, err)
	}
	return n
}

// (a) REBUILD: banco legado (CHECK sem enc_material) é reconstruído preservando
// TODAS as linhas, ids e as FKs — mensagens/mensagem_destinatarios/avisos/
// aviso_cientes continuam resolvendo por JOIN após DROP/RENAME.
func TestV45RebuildPreservaDadosEFKs(t *testing.T) {
	_, st, cleanup := setupTestApp(t)
	defer cleanup()
	gid, _, _ := v45SeedGrupo(t, st, "Grp V45 Rebuild")

	// contas COM papel de sistema + linha em usuario_papeis (o dado que sobrevive)
	criaUsuarioTeste(t, st, "v45r_ger", "senha-v45", "gerente")
	criaUsuarioTeste(t, st, "v45r_op", "senha-v45", "operador")
	if _, err := st.db.Exec(`UPDATE usuarios SET grupo_id = ? WHERE login IN ('v45r_ger','v45r_op')`, gid); err != nil {
		t.Fatalf("vincular grupo: %v", err)
	}
	var uGer, uOp int64
	if err := st.db.QueryRow(`SELECT id FROM usuarios WHERE login='v45r_ger'`).Scan(&uGer); err != nil {
		t.Fatalf("id ger: %v", err)
	}
	if err := st.db.QueryRow(`SELECT id FROM usuarios WHERE login='v45r_op'`).Scan(&uOp); err != nil {
		t.Fatalf("id op: %v", err)
	}
	var pGer, pOp int64
	if err := st.db.QueryRow(`INSERT INTO usuario_papeis (usuario_id, grupo_id, papel) VALUES (?,?,'gerente') RETURNING id`, uGer, gid).Scan(&pGer); err != nil {
		t.Fatalf("papel gerente: %v", err)
	}
	if err := st.db.QueryRow(`INSERT INTO usuario_papeis (usuario_id, grupo_id, papel) VALUES (?,?,'operador') RETURNING id`, uOp, gid).Scan(&pOp); err != nil {
		t.Fatalf("papel operador: %v", err)
	}

	// devolve a tabela ao DDL LEGADO (o que a v45 encontra em produção:
	// CHECK da v24 — sem 'enc_material') — mesmo mecanismo do rebuild da v24
	legado := []string{
		`PRAGMA foreign_keys=OFF`,
		`CREATE TABLE usuario_papeis_legado_v45 (
			id INTEGER PRIMARY KEY,
			usuario_id INTEGER NOT NULL REFERENCES usuarios(id) ON DELETE CASCADE,
			grupo_id INTEGER REFERENCES grupos(id),
			papel TEXT NOT NULL CHECK (papel IN ('admin', 'gerente', 'operador', 'chefe_setor')),
			funcao_id INTEGER REFERENCES funcoes(id),
			nome_exibicao TEXT,
			criado_em TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
			UNIQUE (usuario_id, grupo_id, papel)
		)`,
		`INSERT INTO usuario_papeis_legado_v45 (id, usuario_id, grupo_id, papel, funcao_id, nome_exibicao, criado_em)
			SELECT id, usuario_id, grupo_id, papel, funcao_id, nome_exibicao, criado_em FROM usuario_papeis`,
		`DROP TABLE usuario_papeis`,
		`ALTER TABLE usuario_papeis_legado_v45 RENAME TO usuario_papeis`,
		`CREATE UNIQUE INDEX IF NOT EXISTS idx_usuario_papeis_unico_gerente ON usuario_papeis(grupo_id) WHERE papel = 'gerente'`,
		`PRAGMA foreign_keys=ON`,
	}
	for _, q := range legado {
		if _, err := st.db.Exec(q); err != nil {
			t.Fatalf("shape legado: %v — %s", err, q)
		}
	}
	var ddlLegado string
	if err := st.db.QueryRow(`SELECT COALESCE(sql,'') FROM sqlite_master WHERE type='table' AND name='usuario_papeis'`).Scan(&ddlLegado); err != nil {
		t.Fatalf("ler ddl legado: %v", err)
	}
	if ddlLegado == "" || !strings.Contains(ddlLegado, "CHECK") || strings.Contains(ddlLegado, "enc_material") {
		t.Fatalf("cenário: DDL devia ser o legado SEM enc_material: %s", ddlLegado)
	}

	// dependentes por FK nas duas linhas (remetente/autor NO ACTION; destino/ciente CASCADE)
	if _, err := st.db.Exec(`INSERT INTO mensagens (assunto, corpo, remetente_papel_id, remetente_usuario_id) VALUES ('Msg V45','corpo',?,?)`, pGer, uGer); err != nil {
		t.Fatalf("mensagem: %v", err)
	}
	var mID int64
	if err := st.db.QueryRow(`SELECT id FROM mensagens WHERE assunto='Msg V45'`).Scan(&mID); err != nil {
		t.Fatalf("id mensagem: %v", err)
	}
	if _, err := st.db.Exec(`INSERT INTO mensagem_destinatarios (mensagem_id, destinatario_papel_id) VALUES (?,?)`, mID, pOp); err != nil {
		t.Fatalf("destinatário: %v", err)
	}
	if _, err := st.db.Exec(`INSERT INTO avisos (titulo, conteudo, autor_usuario_id, autor_papel_id, grupo_id) VALUES ('Aviso V45','corpo',?,?,?)`, uGer, pGer, gid); err != nil {
		t.Fatalf("aviso: %v", err)
	}
	var aID int64
	if err := st.db.QueryRow(`SELECT id FROM avisos WHERE titulo='Aviso V45'`).Scan(&aID); err != nil {
		t.Fatalf("id aviso: %v", err)
	}
	if _, err := st.db.Exec(`INSERT INTO aviso_cientes (aviso_id, usuario_id, papel_id) VALUES (?,?,?)`, aID, uOp, pOp); err != nil {
		t.Fatalf("ciente: %v", err)
	}

	antes := map[string]int{
		"papeis":    v45Contagem(t, st, `SELECT COUNT(*) FROM usuario_papeis`),
		"mensagens": v45Contagem(t, st, `SELECT COUNT(*) FROM mensagens`),
		"destins":   v45Contagem(t, st, `SELECT COUNT(*) FROM mensagem_destinatarios`),
		"avisos":    v45Contagem(t, st, `SELECT COUNT(*) FROM avisos`),
		"cientes":   v45Contagem(t, st, `SELECT COUNT(*) FROM aviso_cientes`),
		"id_ger":    v45Contagem(t, st, `SELECT COUNT(*) FROM usuario_papeis WHERE id = ? AND papel='gerente'`, pGer),
		"id_op":     v45Contagem(t, st, `SELECT COUNT(*) FROM usuario_papeis WHERE id = ? AND papel='operador'`, pOp),
	}

	v45Reexecuta(t, st)

	depois := map[string]int{
		"papeis":    v45Contagem(t, st, `SELECT COUNT(*) FROM usuario_papeis`),
		"mensagens": v45Contagem(t, st, `SELECT COUNT(*) FROM mensagens`),
		"destins":   v45Contagem(t, st, `SELECT COUNT(*) FROM mensagem_destinatarios`),
		"avisos":    v45Contagem(t, st, `SELECT COUNT(*) FROM avisos`),
		"cientes":   v45Contagem(t, st, `SELECT COUNT(*) FROM aviso_cientes`),
		"id_ger":    v45Contagem(t, st, `SELECT COUNT(*) FROM usuario_papeis WHERE id = ? AND papel='gerente'`, pGer),
		"id_op":     v45Contagem(t, st, `SELECT COUNT(*) FROM usuario_papeis WHERE id = ? AND papel='operador'`, pOp),
	}
	for k, v := range antes {
		if depois[k] != v {
			t.Fatalf("rebuild perdeu/duplicou %s: antes=%d depois=%d", k, v, depois[k])
		}
	}

	// FKs resolvem: remetente, destinatário, autor e ciente continuam nomeados
	var papelDe string
	if err := st.db.QueryRow(`SELECT up.papel FROM mensagens m JOIN usuario_papeis up ON up.id = m.remetente_papel_id WHERE m.id = ?`, mID).Scan(&papelDe); err != nil || papelDe != "gerente" {
		t.Fatalf("mensagens.remetente_papel_id não resolve após rebuild (papel=%q err=%v)", papelDe, err)
	}
	if err := st.db.QueryRow(`SELECT up.papel FROM mensagem_destinatarios d JOIN usuario_papeis up ON up.id = d.destinatario_papel_id WHERE d.mensagem_id = ?`, mID).Scan(&papelDe); err != nil || papelDe != "operador" {
		t.Fatalf("mensagem_destinatarios não resolve após rebuild (papel=%q err=%v)", papelDe, err)
	}
	if err := st.db.QueryRow(`SELECT up.papel FROM avisos a JOIN usuario_papeis up ON up.id = a.autor_papel_id WHERE a.id = ?`, aID).Scan(&papelDe); err != nil || papelDe != "gerente" {
		t.Fatalf("avisos.autor_papel_id não resolve após rebuild (papel=%q err=%v)", papelDe, err)
	}
	if err := st.db.QueryRow(`SELECT up.papel FROM aviso_cientes c JOIN usuario_papeis up ON up.id = c.papel_id WHERE c.aviso_id = ?`, aID).Scan(&papelDe); err != nil || papelDe != "operador" {
		t.Fatalf("aviso_cientes não resolve após rebuild (papel=%q err=%v)", papelDe, err)
	}

	// DDL novo conhece enc_material e o índice do gerente único voltou
	var ddlNovo string
	if err := st.db.QueryRow(`SELECT COALESCE(sql,'') FROM sqlite_master WHERE type='table' AND name='usuario_papeis'`).Scan(&ddlNovo); err != nil {
		t.Fatalf("ler ddl novo: %v", err)
	}
	if !strings.Contains(ddlNovo, "enc_material") || !strings.Contains(ddlNovo, "enc_pessoal") {
		t.Fatalf("DDL pós-v45 devia conhecer os papéis enc_*: %s", ddlNovo)
	}
	if n := v45Contagem(t, st, `SELECT COUNT(*) FROM sqlite_master WHERE type='index' AND name='idx_usuario_papeis_unico_gerente' AND tbl_name='usuario_papeis'`); n != 1 {
		t.Fatalf("índice idx_usuario_papeis_unico_gerente devia existir pós-rebuild (n=%d)", n)
	}
	// integridade de FK global (nenhuma referência pendurada)
	if n := v45Contagem(t, st, `SELECT COUNT(*) FROM pragma_foreign_key_check`); n != 0 {
		t.Fatalf("foreign_key_check devia vir vazio pós-rebuild, veio %d", n)
	}
	// sem sobra da tabela de trabalho
	if n := v45Contagem(t, st, `SELECT COUNT(*) FROM sqlite_master WHERE name = 'usuario_papeis_v45'`); n != 0 {
		t.Fatalf("tabela de trabalho usuario_papeis_v45 devia ter sido renomeada (n=%d)", n)
	}
}

// (b) MATERIALIZAÇÃO: titular + auxiliar designados na enc_material → 2 linhas
// com funcao_id da cadeira; re-designação pré-migração (sai um, entra outro) →
// a materialização reflete o ESTADO FINAL.
func TestV45MaterializaCadeirasTitularEAuxiliar(t *testing.T) {
	_, st, cleanup := setupTestApp(t)
	defer cleanup()
	gid, fMat, _ := v45SeedGrupo(t, st, "Grp V45 Mat")
	uTit := v45Conta(t, st, "v45m_tit", gid)
	uAux := v45Conta(t, st, "v45m_aux", gid)
	uNovo := v45Conta(t, st, "v45m_novo", gid)

	// pré: nenhum dos designados tem linha
	if n := v45Contagem(t, st, `SELECT COUNT(*) FROM usuario_papeis WHERE usuario_id IN (?,?,?)`, uTit, uAux, uNovo); n != 0 {
		t.Fatalf("cenário: designados puros não deviam ter linha em usuario_papeis (n=%d)", n)
	}

	v45Designa(t, st, fMat, gid, uTit, "titular")
	v45Designa(t, st, fMat, gid, uAux, "auxiliar")

	v45Reexecuta(t, st)

	linhas := v45LinhasEnc(t, st, gid)
	if len(linhas) != 2 {
		t.Fatalf("esperado 2 linhas materializadas (titular+auxiliar), veio %d: %v", len(linhas), linhas)
	}
	porUsuario := map[int64]int64{}
	for id, fid := range linhas {
		var uid int64
		if err := st.db.QueryRow(`SELECT usuario_id FROM usuario_papeis WHERE id = ?`, id).Scan(&uid); err != nil {
			t.Fatalf("ler usuario_id da linha %d: %v", id, err)
		}
		porUsuario[uid] = id
		if fid != fMat {
			t.Fatalf("linha %d devia carregar funcao_id da cadeira (%d), veio %d", id, fMat, fid)
		}
	}
	if _, ok := porUsuario[uTit]; !ok {
		t.Fatalf("titular %d não materializou: %v", uTit, porUsuario)
	}
	if _, ok := porUsuario[uAux]; !ok {
		t.Fatalf("auxiliar %d não materializou: %v", uAux, porUsuario)
	}
	for uid := range porUsuario {
		var papel string
		var gidLinha *int64
		if err := st.db.QueryRow(`SELECT papel, grupo_id FROM usuario_papeis WHERE usuario_id = ?`, uid).Scan(&papel, &gidLinha); err != nil || papel != "enc_material" || gidLinha == nil || *gidLinha != gid {
			t.Fatalf("linha de %d devia ser enc_material do grupo %d (papel=%q gid=%v err=%v)", uid, gid, papel, gidLinha, err)
		}
	}

	// RE-DESIGNAÇÃO no estado pré-migração: titular sai, outro entra; a
	// materialização (re-executada) reflete o estado FINAL da cadeira.
	if _, err := st.db.Exec(`DELETE FROM usuario_papeis WHERE grupo_id = ? AND papel = 'enc_material'`, gid); err != nil {
		t.Fatalf("limpar materialização p/ simular pré-migração: %v", err)
	}
	if _, err := st.db.Exec(`DELETE FROM funcao_membros WHERE funcao_id = ? AND grupo_id = ? AND usuario_id = ?`, fMat, gid, uTit); err != nil {
		t.Fatalf("remover titular: %v", err)
	}
	v45Designa(t, st, fMat, gid, uNovo, "titular")

	v45Reexecuta(t, st)

	linhas = v45LinhasEnc(t, st, gid)
	if len(linhas) != 2 {
		t.Fatalf("estado final devia ter 2 linhas (novo titular + auxiliar), veio %d", len(linhas))
	}
	donos := map[int64]bool{}
	for id := range linhas {
		var uid int64
		if err := st.db.QueryRow(`SELECT usuario_id FROM usuario_papeis WHERE id = ?`, id).Scan(&uid); err != nil {
			t.Fatalf("ler dono da linha %d: %v", id, err)
		}
		donos[uid] = true
	}
	if !donos[uNovo] || !donos[uAux] || donos[uTit] {
		t.Fatalf("materialização devia refletir o estado final (novo titular %d + auxiliar %d, sem o removido %d): %v", uNovo, uAux, uTit, donos)
	}
}

// (c) RE-KEY: sessão de designado puro (papel_ativo_id NULL) aponta para a
// primeira linha enc da conta; sessão NULL de conta SEM designação segue NULL;
// sessão já chaveada não é mexida; /api/me da sessão re-keyada resolve o
// CONTEXTO novo (contrato que o front mínimo consuma: papel = enc_material).
func TestV45RekeySessaoDesignadoPuro(t *testing.T) {
	app, st, cleanup := setupTestApp(t)
	defer cleanup()
	gid, fMat, _ := v45SeedGrupo(t, st, "Grp V45 Rekey")
	uEnc := v45Conta(t, st, "v45k_enc", gid)
	uSem := v45Conta(t, st, "v45k_sem", gid)

	hashDe := func(cru string) string {
		h := sha256.Sum256([]byte(cru))
		return hex.EncodeToString(h[:])
	}
	insereSessao := func(token string, uid int64, papelAtivo *int64) {
		t.Helper()
		expira := time.Now().UTC().Add(2 * time.Hour).Format(time.RFC3339)
		if _, err := st.db.Exec(`INSERT INTO sessoes (token_hash, usuario_id, papel_ativo_id, setor_ativo_id, expira_em) VALUES (?,?,?,?,?)`,
			hashDe(token), uid, papelAtivo, nil, expira); err != nil {
			t.Fatalf("criar sessão: %v", err)
		}
	}

	// sessão JÁ chaveada (controle: não pode ser mexida)
	criaUsuarioTeste(t, st, "v45k_ger", "senha-v45", "gerente")
	if _, err := st.db.Exec(`UPDATE usuarios SET grupo_id = ? WHERE login = 'v45k_ger'`, gid); err != nil {
		t.Fatalf("vincular ger: %v", err)
	}
	var uGer int64
	if err := st.db.QueryRow(`SELECT id FROM usuarios WHERE login='v45k_ger'`).Scan(&uGer); err != nil {
		t.Fatalf("id ger: %v", err)
	}
	var pGer int64
	if err := st.db.QueryRow(`INSERT INTO usuario_papeis (usuario_id, grupo_id, papel) VALUES (?,?,'gerente') RETURNING id`, uGer, gid).Scan(&pGer); err != nil {
		t.Fatalf("papel ger: %v", err)
	}
	insereSessao("tok-v45k-ger", uGer, &pGer)

	insereSessao("tok-v45k-enc", uEnc, nil) // designado puro: NULL
	insereSessao("tok-v45k-sem", uSem, nil) // sem designação: NULL

	v45Designa(t, st, fMat, gid, uEnc, "titular")

	v45Reexecuta(t, st)

	var pEnc int64
	if err := st.db.QueryRow(`SELECT id FROM usuario_papeis WHERE usuario_id = ? AND papel = 'enc_material'`, uEnc).Scan(&pEnc); err != nil {
		t.Fatalf("linha enc do designado: %v", err)
	}
	var ativo *int64
	if err := st.db.QueryRow(`SELECT papel_ativo_id FROM sessoes WHERE token_hash = ?`, hashDe("tok-v45k-enc")).Scan(&ativo); err != nil {
		t.Fatalf("ler sessão do designado: %v", err)
	}
	if ativo == nil || *ativo != pEnc {
		t.Fatalf("sessão do designado devia ser re-keyada p/ linha %d, veio %v", pEnc, ativo)
	}
	// NULL sem designação segue NULL (nada de chaveamento inventado)
	if err := st.db.QueryRow(`SELECT papel_ativo_id FROM sessoes WHERE token_hash = ?`, hashDe("tok-v45k-sem")).Scan(&ativo); err != nil || ativo != nil {
		t.Fatalf("sessão sem designação devia continuar NULL (veio %v err=%v)", ativo, err)
	}
	// sessão já chaveada permanece
	if err := st.db.QueryRow(`SELECT papel_ativo_id FROM sessoes WHERE token_hash = ?`, hashDe("tok-v45k-ger")).Scan(&ativo); err != nil || ativo == nil || *ativo != pGer {
		t.Fatalf("sessão já chaveada não devia mudar (veio %v err=%v)", ativo, err)
	}

	// persona: a MESMA sessão re-keyada resolve o contexto novo no /api/me
	ck := &http.Cookie{Name: cookieSessao, Value: "tok-v45k-enc"}
	rr, res := doJSONReq(app, "GET", "/api/me", nil, ck)
	if rr.Code != http.StatusOK {
		t.Fatalf("/api/me da sessão re-keyada: %d (%v)", rr.Code, res)
	}
	u, ok := res["usuario"].(map[string]any)
	if !ok {
		t.Fatalf("/api/me sem usuario: %v", res)
	}
	if u["papel"] != "enc_material" {
		t.Fatalf("contexto ativo devia resolver enc_material (front: rotuloPapel/rotaInicial), veio %v", u["papel"])
	}
	if fid, _ := u["funcao_id"].(float64); int64(fid) != fMat {
		t.Fatalf("funcao_id do contexto devia ser a cadeira %d, veio %v", fMat, u["funcao_id"])
	}
	papeis, _ := u["papeis"].([]any)
	achou := false
	for _, pp := range papeis {
		if p, _ := pp.(map[string]any); p != nil && p["papel"] == "enc_material" {
			achou = true
		}
	}
	if !achou {
		t.Fatalf("dropdown (/api/me papeis) devia listar a linha enc_material: %v", papeis)
	}

	// e o designado puro que LOGA depois da migração já nasce no contexto enc
	ckLogin := loginAs(t, app, "v45k_enc", "senha-v45")
	if rr, res := doJSONReq(app, "GET", "/api/me", nil, ckLogin); rr.Code != http.StatusOK {
		t.Fatalf("login do designado puro pós-v45: %d (%v)", rr.Code, res)
	} else if u, ok := res["usuario"].(map[string]any); !ok || u["papel"] != "enc_material" {
		t.Fatalf("designado puro pós-v45 devia logar NO CONTEXTO enc_material, veio %v", u["papel"])
	}
}

// (d) IDEMPOTÊNCIA + CADEIA: banco novo nasce em v45 == versaoSchemaBinario;
// re-executar a migração (marca removida) não duplica linha nem troca id;
// executar com a marca presente é no-op; sem sobra de tabela de rebuild.
func TestV45IdempotenteECadeiaBinario(t *testing.T) {
	_, st, cleanup := setupTestApp(t)
	defer cleanup()

	// (e) cadeia completa do setupTestApp termina exatamente no binário
	var maxVersao int
	if err := st.db.QueryRow(`SELECT COALESCE(MAX(versao),0) FROM schema_migrations`).Scan(&maxVersao); err != nil {
		t.Fatalf("ler schema_migrations: %v", err)
	}
	if maxVersao != versaoSchemaBinario || versaoSchemaBinario != 45 {
		t.Fatalf("cadeia devia terminar em versaoSchemaBinario=45 (max=%d binário=%d)", maxVersao, versaoSchemaBinario)
	}

	gid, fMat, _ := v45SeedGrupo(t, st, "Grp V45 Idem")
	uEnc := v45Conta(t, st, "v45i_enc", gid)
	v45Designa(t, st, fMat, gid, uEnc, "titular")

	v45Reexecuta(t, st)

	linhas1 := v45LinhasEnc(t, st, gid)
	if len(linhas1) != 1 {
		t.Fatalf("1ª execução devia materializar 1 linha, veio %d", len(linhas1))
	}

	// re-execução (marca removida): rebuild pulado pelo guard do DDL, nada duplica
	v45Reexecuta(t, st)
	linhas2 := v45LinhasEnc(t, st, gid)
	if len(linhas2) != len(linhas1) {
		t.Fatalf("re-execução duplicou materialização: %d → %d", len(linhas1), len(linhas2))
	}
	for id, fid := range linhas1 {
		if fid2, ok := linhas2[id]; !ok || fid2 != fid {
			t.Fatalf("re-execução mudou a linha %d (%d → %d)", id, fid, fid2)
		}
	}

	// execução com a marca PRESENTE: no-op sem erro
	if err := st.migrarV45(); err != nil {
		t.Fatalf("migrarV45 com marca presente devia ser no-op: %v", err)
	}
	if n := len(v45LinhasEnc(t, st, gid)); n != 1 {
		t.Fatalf("execução com marca presente devia manter 1 linha (n=%d)", n)
	}

	// marca presente EXATAMENTE 1×; sem tabela de trabalho sobrando
	if n := v45Contagem(t, st, `SELECT COUNT(*) FROM schema_migrations WHERE versao = 45`); n != 1 {
		t.Fatalf("marca v45 devia existir 1× (n=%d)", n)
	}
	if n := v45Contagem(t, st, `SELECT COUNT(*) FROM sqlite_master WHERE name LIKE 'usuario_papeis_%' AND name != 'usuario_papeis'`); n != 0 {
		t.Fatalf("sobras de rebuild em sqlite_master: %d", n)
	}
	var ddl string
	if err := st.db.QueryRow(`SELECT COALESCE(sql,'') FROM sqlite_master WHERE type='table' AND name='usuario_papeis'`).Scan(&ddl); err != nil {
		t.Fatalf("ler ddl: %v", err)
	}
	if !strings.Contains(ddl, "enc_material") {
		t.Fatalf("DDL final devia conhecer enc_material: %s", ddl)
	}
}
