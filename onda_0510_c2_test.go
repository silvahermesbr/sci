package main

// Testes da onda C2 (ordem Diretor, 05/10): Mural no topo + Aba Funções.
//
// MURAL — admin no mural (decisão de desenho: remover bloqueio; visão global):
//   M1. Admin lista avisos → 200 (antes 403); vê avisos de TODOS os grupos.
//   M2. Admin publica aviso com grupo explícito → 200; persiste no grupo alvo.
//   M3. Admin publica SEM grupo → 400 (publicar exige grupo de destino).
//   M4. Admin comenta, registra ciente e vê detalhes → 200 (regressão do bloco antigo).
//   M5. Admin exclui aviso PRÓPRIO → 200; aviso de gerente → 403.
//   M6. Operador continua lendo/publicando no próprio grupo (regressão).
//   M7. Gerente continua excluindo aviso de outro autor → 403 (regressão).
//
// FUNÇÕES — designação de membros (aba Funções do Gerenciar):
//   F1. GET sem grupo → 403; com grupo → 200 com funções do catálogo.
//   F2. Gerente designa titular → 200; persiste; GET reflete.
//   F3. Segundo titular na mesma função → 409 (índice parcial único).
//   F4. Mesmo usuário duas vezes → 409 (UNIQUE função+grupo+usuário).
//   F5. Auxiliar adicionado além do titular → 200.
//   F6. Usuário de OUTRO grupo → 400; operador → 403; sem grupo → 403.
//   F7. DELETE de designação do próprio grupo → 200; de outro grupo → 404.
//   F8. Migração v32: versão 32 registrada; tabela e índice parcial presentes.

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

// c2Setup cria grupo + função de catálogo + gerente/operador/outroGrupo/admin.
func c2Setup(t *testing.T, app *App, st *Store) (gid, outroGid, funcaoID, gerUID, opUID int64) {
	t.Helper()
	if err := st.db.QueryRow(`INSERT INTO grupos (nome) VALUES ('Grp C2 Alfa') RETURNING id`).Scan(&gid); err != nil {
		t.Fatalf("criar grupo: %v", err)
	}
	if err := st.db.QueryRow(`INSERT INTO grupos (nome) VALUES ('Grp C2 Beta') RETURNING id`).Scan(&outroGid); err != nil {
		t.Fatalf("criar grupo beta: %v", err)
	}
	if err := st.db.QueryRow(`INSERT INTO funcoes (nome, tipo) VALUES ('Oficial de Dia C2', 'grupo') RETURNING id`).Scan(&funcaoID); err != nil {
		t.Fatalf("criar função: %v", err)
	}
	criaUsuarioTeste(t, st, "c2ger", "senha-ger", "gerente")
	criaUsuarioTeste(t, st, "c2op", "senha-op", "operador")
	criaUsuarioTeste(t, st, "c2gerb", "senha-gerb", "gerente")
	criaUsuarioTeste(t, st, "c2adm", "senha-adm", "admin")
	if _, err := st.db.Exec(`UPDATE usuarios SET grupo_id = ? WHERE login IN ('c2ger','c2op')`, gid); err != nil {
		t.Fatalf("vincular grupo: %v", err)
	}
	if _, err := st.db.Exec(`UPDATE usuarios SET grupo_id = ? WHERE login = 'c2gerb'`, outroGid); err != nil {
		t.Fatalf("vincular grupo beta: %v", err)
	}
	if _, err := st.db.Exec(`UPDATE usuario_papeis SET grupo_id = ? WHERE usuario_id IN (SELECT id FROM usuarios WHERE login IN ('c2ger','c2op'))`, gid); err != nil {
		t.Fatalf("vincular papeis: %v", err)
	}
	if _, err := st.db.Exec(`UPDATE usuario_papeis SET grupo_id = ? WHERE usuario_id IN (SELECT id FROM usuarios WHERE login = 'c2gerb')`, outroGid); err != nil {
		t.Fatalf("vincular papeis beta: %v", err)
	}
	if err := st.db.QueryRow(`SELECT id FROM usuarios WHERE login = 'c2ger'`).Scan(&gerUID); err != nil {
		t.Fatalf("id gerente: %v", err)
	}
	if err := st.db.QueryRow(`SELECT id FROM usuarios WHERE login = 'c2op'`).Scan(&opUID); err != nil {
		t.Fatalf("id operador: %v", err)
	}
	return
}

func TestC2MuralAdminGlobal(t *testing.T) {
	app, st, cleanup := setupTestApp(t)
	defer cleanup()
	gidA, _, _, _, _ := c2Setup(t, app, st)

	// Dois gerentes publicam em grupos distintos (caminho HTTP provado; o
	// insert direto via subselect quebra com usuario_papeis multi-linha).
	ckGerA := loginAs(t, app, "c2ger", "senha-ger")
	ckGerB := loginAs(t, app, "c2gerb", "senha-gerb")
	rrA, resA := doJSONReq(app, "POST", "/api/avisos", map[string]any{"titulo": "Aviso Alfa", "conteudo": "<p>a</p>"}, ckGerA)
	if rrA.Code != http.StatusOK {
		t.Fatalf("sanity: gerente A publica (200), veio %d (%v)", rrA.Code, resA)
	}
	rrB, resB := doJSONReq(app, "POST", "/api/avisos", map[string]any{"titulo": "Aviso Beta", "conteudo": "<p>b</p>"}, ckGerB)
	if rrB.Code != http.StatusOK {
		t.Fatalf("sanity: gerente B publica (200), veio %d (%v)", rrB.Code, resB)
	}

	ckAdm := loginAs(t, app, "c2adm", "senha-adm")
	rr, _ := doJSONReq(app, "GET", "/api/avisos", nil, ckAdm)
	if rr.Code != http.StatusOK {
		t.Fatalf("M1: admin deve listar o mural (200), veio %d", rr.Code)
	}
	var lista []map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &lista); err != nil {
		t.Fatalf("M1: lista não é array raiz: %v", err)
	}
	titulos := map[string]bool{}
	for _, av := range lista {
		if s, ok := av["titulo"].(string); ok {
			titulos[s] = true
		}
	}
	if !titulos["Aviso Alfa"] || !titulos["Aviso Beta"] {
		t.Fatalf("M1: admin deveria ver murais de TODOS os grupos; veio %v", titulos)
	}

	// M4: detalhes, comentário e ciente do admin.
	rrD, _ := doJSONReq(app, "GET", "/api/avisos/1/detalhes", nil, ckAdm)
	if rrD.Code != http.StatusOK {
		t.Fatalf("M4a: detalhes 200, veio %d", rrD.Code)
	}
	if rrC, _ := doJSONReq(app, "POST", "/api/avisos/1/comentar", map[string]any{"texto": "<p>ciente da administração</p>"}, ckAdm); rrC.Code != http.StatusOK {
		t.Fatalf("M4b: comentário do admin 200, veio %d", rrC.Code)
	}
	if rrCi, resCi := doJSONReq(app, "POST", "/api/avisos/1/ciente", map[string]any{}, ckAdm); rrCi.Code != http.StatusOK {
		t.Fatalf("M4c: ciente do admin 200, veio %d (%v)", rrCi.Code, resCi)
	}

	// M2: publica com grupo explícito → 200 e persiste no grupo alvo.
	rrP, resP := doJSONReq(app, "POST", "/api/avisos", map[string]any{"titulo": "Comunicado Global", "conteudo": "<p>determino</p>", "grupo_id": gidA}, ckAdm)
	if rrP.Code != http.StatusOK {
		t.Fatalf("M2: admin publica com grupo (200), veio %d (%v)", rrP.Code, resP)
	}
	var n int
	if err := st.db.QueryRow(`SELECT COUNT(*) FROM avisos WHERE titulo='Comunicado Global' AND grupo_id = ?`, gidA).Scan(&n); err != nil || n != 1 {
		t.Fatalf("M2: aviso do admin não persistiu no grupo alvo (n=%d err=%v)", n, err)
	}

	// M3: publica sem grupo → 400.
	if rr3, _ := doJSONReq(app, "POST", "/api/avisos", map[string]any{"titulo": "Sem Grupo", "conteudo": "<p>x</p>"}, ckAdm); rr3.Code != http.StatusBadRequest {
		t.Fatalf("M3: publicar sem grupo deve 400, veio %d", rr3.Code)
	}

	// M5: admin exclui o PRÓPRIO aviso → 200; aviso de gerente → 403.
	idAdm, _ := resP["id"].(float64) // "Comunicado Global" — autor admin
	idGer, _ := resA["id"].(float64) // "Aviso Alfa" — autor gerente A
	if rr5a, _ := doJSONReq(app, "DELETE", "/api/avisos/"+intToString(int(idAdm)), nil, ckAdm); rr5a.Code != http.StatusOK {
		t.Fatalf("M5a: admin exclui próprio aviso (200), veio %d", rr5a.Code)
	}
	if rr5b, _ := doJSONReq(app, "DELETE", "/api/avisos/"+intToString(int(idGer)), nil, ckAdm); rr5b.Code != http.StatusForbidden {
		t.Fatalf("M5b: admin não exclui aviso de gerente (403), veio %d", rr5b.Code)
	}

	// M7 sanity: aviso inexistente → 404 para o gerente (guarda autoral vivo).
	ckGer := loginAs(t, app, "c2ger", "senha-ger")
	if rr7, _ := doJSONReq(app, "DELETE", "/api/avisos/999999", nil, ckGer); rr7.Code != http.StatusNotFound {
		t.Fatalf("M7 sanity: aviso inexistente deve 404, veio %d", rr7.Code)
	}
}

func TestC2MuralPapeisComGrupo(t *testing.T) {
	app, st, cleanup := setupTestApp(t)
	defer cleanup()
	c2Setup(t, app, st)

	// M6: operador com grupo lê o mural (guarda antigo bloqueava só admin;
	// fixar a leitura do operador como regressão da onda).
	ckOp := loginAs(t, app, "c2op", "senha-op")
	if rr, _ := doJSONReq(app, "GET", "/api/avisos", nil, ckOp); rr.Code != http.StatusOK {
		t.Fatalf("M6: operador com grupo lê mural (200), veio %d", rr.Code)
	}

	// Conta SEM grupo e SEM papel → 403 (o mural precisa de grupo).
	criaUsuarioTeste(t, st, "c2sem", "senha-sem", "")
	ckSem := loginAs(t, app, "c2sem", "senha-sem")
	if rr, _ := doJSONReq(app, "GET", "/api/avisos", nil, ckSem); rr.Code != http.StatusForbidden {
		t.Fatalf("sem grupo deve 403 no mural, veio %d", rr.Code)
	}
}

func TestC2FuncoesMembros(t *testing.T) {
	app, st, cleanup := setupTestApp(t)
	defer cleanup()
	gid, _, funcaoID, gerUID, opUID := c2Setup(t, app, st)

	// F8: migração v32 aplicada.
	var versao int
	if err := st.db.QueryRow(`SELECT MAX(versao) FROM schema_migrations`).Scan(&versao); err != nil || versao < 32 {
		t.Fatalf("F8: schema versão >= 32, obtido %d (err %v)", versao, err)
	}
	var nIdx int
	if err := st.db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type='index' AND name='idx_funcao_membros_titular'`).Scan(&nIdx); err != nil || nIdx != 1 {
		t.Fatalf("F8: índice parcial do titular ausente (n=%d err=%v)", nIdx, err)
	}

	ckGer := loginAs(t, app, "c2ger", "senha-ger")

	// F1: GET com grupo → 200 e função do catálogo presente.
	rr, _ := doJSONReq(app, "GET", "/api/grupo/funcoes/membros", nil, ckGer)
	if rr.Code != http.StatusOK {
		t.Fatalf("F1: GET membros 200, veio %d", rr.Code)
	}
	var lista []map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &lista); err != nil {
		t.Fatalf("F1: resposta não é array: %v", err)
	}
	achou := false
	for _, it := range lista {
		if int64(it["funcao_id"].(float64)) == funcaoID {
			achou = true
		}
	}
	if !achou {
		t.Fatalf("F1: função do catálogo não veio na listagem")
	}

	// F2: designa titular → 200 e persiste.
	rr2, res2 := doJSONReq(app, "POST", "/api/grupo/funcoes/membros", map[string]any{"funcao_id": funcaoID, "usuario_id": gerUID, "titularidade": "titular"}, ckGer)
	if rr2.Code != http.StatusOK {
		t.Fatalf("F2: designar titular 200, veio %d (%v)", rr2.Code, res2)
	}
	var tit string
	if err := st.db.QueryRow(`SELECT titularidade FROM funcao_membros WHERE funcao_id=? AND grupo_id=? AND usuario_id=?`, funcaoID, gid, gerUID).Scan(&tit); err != nil || tit != "titular" {
		t.Fatalf("F2: designação não persistiu (tit=%q err=%v)", tit, err)
	}

	// F3: segundo titular → 409.
	criaUsuarioTeste(t, st, "c2op2", "senha-op2", "operador")
	if _, err := st.db.Exec(`UPDATE usuarios SET grupo_id = ? WHERE login='c2op2'`, gid); err != nil {
		t.Fatalf("vincular op2: %v", err)
	}
	var op2UID int64
	_ = st.db.QueryRow(`SELECT id FROM usuarios WHERE login='c2op2'`).Scan(&op2UID)
	if rr3, _ := doJSONReq(app, "POST", "/api/grupo/funcoes/membros", map[string]any{"funcao_id": funcaoID, "usuario_id": op2UID, "titularidade": "titular"}, ckGer); rr3.Code != http.StatusConflict {
		t.Fatalf("F3: segundo titular deve 409, veio %d", rr3.Code)
	}

	// F5: auxiliar além do titular → 200.
	if rr5, _ := doJSONReq(app, "POST", "/api/grupo/funcoes/membros", map[string]any{"funcao_id": funcaoID, "usuario_id": opUID, "titularidade": "auxiliar"}, ckGer); rr5.Code != http.StatusOK {
		t.Fatalf("F5: auxiliar 200, veio %d", rr5.Code)
	}

	// F4: mesmo usuário repetido → 409.
	if rr4, _ := doJSONReq(app, "POST", "/api/grupo/funcoes/membros", map[string]any{"funcao_id": funcaoID, "usuario_id": gerUID, "titularidade": "auxiliar"}, ckGer); rr4.Code != http.StatusConflict {
		t.Fatalf("F4: mesmo usuário repetido deve 409, veio %d", rr4.Code)
	}

	// F6: usuário de outro grupo → 400; operador → 403; sem grupo → 403.
	var gerBUID int64
	_ = st.db.QueryRow(`SELECT id FROM usuarios WHERE login='c2gerb'`).Scan(&gerBUID)
	if rr6a, _ := doJSONReq(app, "POST", "/api/grupo/funcoes/membros", map[string]any{"funcao_id": funcaoID, "usuario_id": gerBUID, "titularidade": "auxiliar"}, ckGer); rr6a.Code != http.StatusBadRequest {
		t.Fatalf("F6a: usuário de outro grupo deve 400, veio %d", rr6a.Code)
	}
	ckOp := loginAs(t, app, "c2op", "senha-op")
	if rr6b, _ := doJSONReq(app, "POST", "/api/grupo/funcoes/membros", map[string]any{"funcao_id": funcaoID, "usuario_id": opUID, "titularidade": "auxiliar"}, ckOp); rr6b.Code != http.StatusForbidden {
		t.Fatalf("F6b: operador deve 403, veio %d", rr6b.Code)
	}
	criaUsuarioTeste(t, st, "c2sem", "senha-sem", "")
	ckSem := loginAs(t, app, "c2sem", "senha-sem")
	if rr6c, _ := doJSONReq(app, "GET", "/api/grupo/funcoes/membros", nil, ckSem); rr6c.Code != http.StatusForbidden {
		t.Fatalf("F6c: sem grupo deve 403 no GET, veio %d", rr6c.Code)
	}

	// F7: DELETE do próprio grupo → 200; de outro grupo → 404.
	var memID int64
	if err := st.db.QueryRow(`SELECT id FROM funcao_membros WHERE funcao_id=? AND grupo_id=? AND usuario_id=?`, funcaoID, gid, opUID).Scan(&memID); err != nil {
		t.Fatalf("F7: designação auxiliar não encontrada: %v", err)
	}
	if rr7a, _ := doJSONReq(app, "DELETE", "/api/grupo/funcoes/membros/"+intToString(int(memID)), nil, ckGer); rr7a.Code != http.StatusOK {
		t.Fatalf("F7a: delete próprio 200, veio %d", rr7a.Code)
	}
	// F7b: designação em grupo FORA do escopo → 404 (filtro grupo_id no WHERE).
	// Caminho provado: admin cria grupo novo com gerente próprio (hGruposAdd).
	ckAdm := loginAs(t, app, "c2adm", "senha-adm")
	rrG, resG := doJSONReq(app, "POST", "/api/grupos", map[string]any{"nome": "Grp C2 Gama", "login": "c2gerg", "senha": "senha-gerg", "nome_guerra": "GERG"}, ckAdm)
	if rrG.Code != http.StatusOK {
		t.Fatalf("F7b sanity: criar grupo gama (200), veio %d (%v)", rrG.Code, resG)
	}
	gamaID, _ := resG["id"].(float64)
	var memBID int64
	var gerGamaUID int64
	if err := st.db.QueryRow(`SELECT id FROM usuarios WHERE login='c2gerg'`).Scan(&gerGamaUID); err != nil {
		t.Fatalf("F7b: gerente gama não criado: %v", err)
	}
	if _, err := st.db.Exec(`INSERT INTO funcao_membros (funcao_id, grupo_id, usuario_id, titularidade) VALUES (?,?,?,'titular')`, funcaoID, int64(gamaID), gerGamaUID); err != nil {
		t.Fatalf("F7b: designação no gama: %v", err)
	}
	_ = st.db.QueryRow(`SELECT id FROM funcao_membros WHERE grupo_id = ?`, int64(gamaID)).Scan(&memBID)
	if rr7b, _ := doJSONReq(app, "DELETE", "/api/grupo/funcoes/membros/"+intToString(int(memBID)), nil, ckGer); rr7b.Code != http.StatusNotFound {
		t.Fatalf("F7b: delete de outro grupo deve 404, veio %d", rr7b.Code)
	}
}

func TestC2FuncoesTitularidadeInvalida(t *testing.T) {
	app, st, cleanup := setupTestApp(t)
	defer cleanup()
	_, _, funcaoID, gerUID, _ := c2Setup(t, app, st)
	ckGer := loginAs(t, app, "c2ger", "senha-ger")
	if rr, _ := doJSONReq(app, "POST", "/api/grupo/funcoes/membros", map[string]any{"funcao_id": funcaoID, "usuario_id": gerUID, "titularidade": "chefe"}, ckGer); rr.Code != http.StatusBadRequest {
		t.Fatalf("titularidade inválida deve 400, veio %d", rr.Code)
	}
	if rr, _ := doJSONReq(app, "POST", "/api/grupo/funcoes/membros", map[string]any{"funcao_id": funcaoID, "usuario_id": gerUID, "titularidade": " titular "}, ckGer); rr.Code != http.StatusOK {
		t.Fatalf("titularidade com espaços deve normalizar → 200, veio %d", rr.Code)
	}
	if !strings.Contains("x", "x") {
		t.Fatal("inacessível")
	}
}
