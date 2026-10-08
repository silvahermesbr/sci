package main

import (
	"net/http"
	"strings"
	"testing"
)

// TestF3Migracao38IdempotenteEBackfill testa a migração 37->38 e o backfill idempotente.
func TestF3Migracao38IdempotenteEBackfill(t *testing.T) {
	app, st, cleanup := setupTestApp(t)
	defer cleanup()

	// Inserir funções estilo antigo (sem tipo/chave especificados, caem no default)
	var fPess, fMat, fGer, fOutra int64
	if err := st.db.QueryRow(`SELECT id FROM funcoes WHERE chave = 'enc_pessoal'`).Scan(&fPess); err != nil {
		t.Fatalf("cadeira enc_pessoal semeda ausente: %v", err)
	}
	if err := st.db.QueryRow(`SELECT id FROM funcoes WHERE chave = 'enc_material'`).Scan(&fMat); err != nil {
		t.Fatalf("cadeira enc_material semeda ausente: %v", err)
	}
	if err := st.db.QueryRow(`INSERT INTO funcoes (nome) VALUES ('gerente') RETURNING id`).Scan(&fGer); err != nil {
		t.Fatalf("inserir gerente: %v", err)
	}
	if err := st.db.QueryRow(`INSERT INTO funcoes (nome) VALUES ('Soldado EV') RETURNING id`).Scan(&fOutra); err != nil {
		t.Fatalf("inserir soldado: %v", err)
	}

	// Forçar reexecução da migração v38 removendo o marcador
	if _, err := st.db.Exec(`DELETE FROM schema_migrations WHERE versao = 38`); err != nil {
		t.Fatalf("deletar marcador v38: %v", err)
	}
	// Resetar colunas para antiguidade/NULL para testar o backfill
	if _, err := st.db.Exec(`UPDATE funcoes SET tipo = 'antiguidade', chave = NULL WHERE id IN (?, ?, ?, ?)`, fPess, fMat, fGer, fOutra); err != nil {
		t.Fatalf("resetar colunas: %v", err)
	}

	if err := st.migrarV38(); err != nil {
		t.Fatalf("migrarV38 falhou: %v", err)
	}

	// Verificar backfill
	var tipoPess, tipoMat, tipoGer, tipoOutra string
	var chavePess, chaveMat, chaveGer, chaveOutra *string

	_ = st.db.QueryRow(`SELECT tipo, chave FROM funcoes WHERE id = ?`, fPess).Scan(&tipoPess, &chavePess)
	_ = st.db.QueryRow(`SELECT tipo, chave FROM funcoes WHERE id = ?`, fMat).Scan(&tipoMat, &chaveMat)
	_ = st.db.QueryRow(`SELECT tipo, chave FROM funcoes WHERE id = ?`, fGer).Scan(&tipoGer, &chaveGer)
	_ = st.db.QueryRow(`SELECT tipo, chave FROM funcoes WHERE id = ?`, fOutra).Scan(&tipoOutra, &chaveOutra)

	if tipoPess != "grupo" || chavePess == nil || *chavePess != "enc_pessoal" {
		t.Fatalf("fPess esperado grupo/enc_pessoal, veio tipo=%q chave=%v", tipoPess, chavePess)
	}
	if tipoMat != "grupo" || chaveMat == nil || *chaveMat != "enc_material" {
		t.Fatalf("fMat esperado grupo/enc_material, veio tipo=%q chave=%v", tipoMat, chaveMat)
	}
	if tipoGer != "grupo" || chaveGer != nil {
		t.Fatalf("fGer esperado grupo/nil, veio tipo=%q chave=%v", tipoGer, chaveGer)
	}
	if tipoOutra != "antiguidade" || chaveOutra != nil {
		t.Fatalf("fOutra esperado antiguidade/nil, veio tipo=%q chave=%v", tipoOutra, chaveOutra)
	}

	// v39 (ordem Diretor): as 3 cadeiras HARDCODED devem existir com as chaves certas
	var nEncP, nEncM, nGer int
	_ = st.db.QueryRow(`SELECT COUNT(*) FROM funcoes WHERE chave = 'enc_pessoal'`).Scan(&nEncP)
	_ = st.db.QueryRow(`SELECT COUNT(*) FROM funcoes WHERE chave = 'enc_material'`).Scan(&nEncM)
	_ = st.db.QueryRow(`SELECT COUNT(*) FROM funcoes WHERE tipo = 'grupo' AND LOWER(nome) = 'gerente'`).Scan(&nGer)
	if nEncP != 1 || nEncM != 1 || nGer < 1 {
		t.Fatalf("cadeiras hardcoded ausentes/duplicadas: enc_pessoal=%d enc_material=%d gerente=%d", nEncP, nEncM, nGer)
	}

	// Provar idempotência rodando migrarV38 novamente
	if err := st.migrarV38(); err != nil {
		t.Fatalf("migrarV38 idempotência falhou: %v", err)
	}
	_ = app
}

// TestF3AuxiliarEncPessoalTemPoderGestaoPessoal testa que auxiliar tem o mesmo poder do titular.
func TestF3AuxiliarEncPessoalTemPoderGestaoPessoal(t *testing.T) {
	app, st, cleanup := setupTestApp(t)
	defer cleanup()

	var gid int64
	if err := st.db.QueryRow(`INSERT INTO grupos (nome) VALUES ('Grp F3 Pessoal') RETURNING id`).Scan(&gid); err != nil {
		t.Fatalf("criar grupo: %v", err)
	}
	var fEnc int64
	if err := st.db.QueryRow(`SELECT id FROM funcoes WHERE chave = 'enc_pessoal'`, gid).Scan(&fEnc); err != nil {
		t.Fatalf("criar funcao: %v", err)
	}

	criaUsuarioTeste(t, st, "f3aux_pess", "senha-aux", "")
	var uid int64
	_ = st.db.QueryRow(`SELECT id FROM usuarios WHERE login='f3aux_pess'`).Scan(&uid)
	_, _ = st.db.Exec(`UPDATE usuarios SET grupo_id = ? WHERE id = ?`, gid, uid)

	// Designar como AUXILIAR
	if _, err := st.db.Exec(`INSERT INTO funcao_membros (funcao_id, grupo_id, usuario_id, titularidade) VALUES (?, ?, ?, 'auxiliar')`, fEnc, gid, uid); err != nil {
		t.Fatalf("designar auxiliar: %v", err)
	}

	ck := loginAs(t, app, "f3aux_pess", "senha-aux")

	// Auxiliar tem poder de cadastrar pessoa
	rr, res := doJSONReq(app, "POST", "/api/pessoas", map[string]any{"nome_guerra": "AUXPESSOA", "nome_completo": "Auxiliar Pessoa Teste"}, ck)
	if rr.Code != http.StatusOK {
		t.Fatalf("auxiliar de pessoal deve conseguir cadastrar pessoa (200), veio %d (%v)", rr.Code, res)
	}

	// Auxiliar tem poder de listar contas
	rrU, _ := doJSONReq(app, "GET", "/api/usuarios", nil, ck)
	if rrU.Code != http.StatusOK {
		t.Fatalf("auxiliar de pessoal deve conseguir listar contas (200), veio %d", rrU.Code)
	}
}

// TestF3EncMaterialAcessaMaterialENaoPessoal testa acesso exclusivo a material pelo enc_material.
func TestF3EncMaterialAcessaMaterialENaoPessoal(t *testing.T) {
	app, st, cleanup := setupTestApp(t)
	defer cleanup()

	var gid int64
	if err := st.db.QueryRow(`INSERT INTO grupos (nome) VALUES ('Grp F3 Mat') RETURNING id`).Scan(&gid); err != nil {
		t.Fatalf("criar grupo: %v", err)
	}
	var fMat int64
	if err := st.db.QueryRow(`SELECT id FROM funcoes WHERE chave = 'enc_material'`, gid).Scan(&fMat); err != nil {
		t.Fatalf("criar funcao material: %v", err)
	}
	var catID int64
	if err := st.db.QueryRow(`INSERT INTO material_categorias (nome, grupo_id) VALUES ('Armamento F3', ?) RETURNING id`, gid).Scan(&catID); err != nil {
		t.Fatalf("criar categoria: %v", err)
	}

	criaUsuarioTeste(t, st, "f3tit_mat", "senha-mat", "")
	criaUsuarioTeste(t, st, "f3aux_mat", "senha-mat-aux", "")
	var uidTit, uidAux int64
	_ = st.db.QueryRow(`SELECT id FROM usuarios WHERE login='f3tit_mat'`).Scan(&uidTit)
	_ = st.db.QueryRow(`SELECT id FROM usuarios WHERE login='f3aux_mat'`).Scan(&uidAux)
	_, _ = st.db.Exec(`UPDATE usuarios SET grupo_id = ? WHERE id IN (?, ?)`, gid, uidTit, uidAux)

	// Designar titular e auxiliar de material
	if _, err := st.db.Exec(`INSERT INTO funcao_membros (funcao_id, grupo_id, usuario_id, titularidade) VALUES (?, ?, ?, 'titular')`, fMat, gid, uidTit); err != nil {
		t.Fatalf("designar titular material: %v", err)
	}
	if _, err := st.db.Exec(`INSERT INTO funcao_membros (funcao_id, grupo_id, usuario_id, titularidade) VALUES (?, ?, ?, 'auxiliar')`, fMat, gid, uidAux); err != nil {
		t.Fatalf("designar auxiliar material: %v", err)
	}

	ckTit := loginAs(t, app, "f3tit_mat", "senha-mat")
	ckAux := loginAs(t, app, "f3aux_mat", "senha-mat-aux")

	for _, tc := range []struct {
		nome   string
		cookie *http.Cookie
	}{
		{"titular", ckTit},
		{"auxiliar", ckAux},
	} {
		// Acesso a Material -> 200
		rrM, resM := doJSONReq(app, "POST", "/api/material/itens", map[string]any{
			"nome":              "Fuzil Fal 7.62 " + tc.nome,
			"categoria_id":      catID,
			"codigo_patrimonio": "FAL-F3-" + tc.nome,
			"status":            "disponivel",
			"sensibilidade":     "controlado",
		}, tc.cookie)
		if rrM.Code != http.StatusOK {
			t.Fatalf("%s de material deve conseguir cadastrar item (200), veio %d (%v)", tc.nome, rrM.Code, resM)
		}

		// Listar itens de material -> 200
		rrL, _ := doJSONReq(app, "GET", "/api/material/itens", nil, tc.cookie)
		if rrL.Code != http.StatusOK {
			t.Fatalf("%s de material deve listar itens (200), veio %d", tc.nome, rrL.Code)
		}

		// Acesso a Pessoal -> 403
		rrP, _ := doJSONReq(app, "POST", "/api/pessoas", map[string]any{"nome_guerra": "TENTATIVAPES", "nome_completo": "Tentativa Pessoal"}, tc.cookie)
		if rrP.Code != http.StatusForbidden {
			t.Fatalf("%s de material NÃO deve ter acesso a pessoal (403), veio %d", tc.nome, rrP.Code)
		}
	}
}

// TestF3EncarregadoNaoDesignaMembros testa que encarregados não conseguem designar membros (403).
func TestF3EncarregadoNaoDesignaMembros(t *testing.T) {
	app, st, cleanup := setupTestApp(t)
	defer cleanup()

	var gid int64
	if err := st.db.QueryRow(`INSERT INTO grupos (nome) VALUES ('Grp F3 Desig') RETURNING id`).Scan(&gid); err != nil {
		t.Fatalf("criar grupo: %v", err)
	}
	var fEnc, fOutra int64
	if err := st.db.QueryRow(`SELECT id FROM funcoes WHERE chave = 'enc_pessoal'`, gid).Scan(&fEnc); err != nil {
		t.Fatalf("criar enc pessoal: %v", err)
	}
	if err := st.db.QueryRow(`INSERT INTO funcoes (nome, grupo_id, tipo) VALUES ('Funcao Alvo', ?, 'grupo') RETURNING id`, gid).Scan(&fOutra); err != nil {
		t.Fatalf("criar funcao alvo: %v", err)
	}

	criaUsuarioTeste(t, st, "f3enc_user", "senha-enc", "")
	criaUsuarioTeste(t, st, "f3membro_alvo", "senha-alvo", "operador")
	var uidEnc, uidAlvo int64
	_ = st.db.QueryRow(`SELECT id FROM usuarios WHERE login='f3enc_user'`).Scan(&uidEnc)
	_ = st.db.QueryRow(`SELECT id FROM usuarios WHERE login='f3membro_alvo'`).Scan(&uidAlvo)
	_, _ = st.db.Exec(`UPDATE usuarios SET grupo_id = ? WHERE id IN (?, ?)`, gid, uidEnc, uidAlvo)

	if _, err := st.db.Exec(`INSERT INTO funcao_membros (funcao_id, grupo_id, usuario_id, titularidade) VALUES (?, ?, ?, 'titular')`, fEnc, gid, uidEnc); err != nil {
		t.Fatalf("designar enc: %v", err)
	}

	ck := loginAs(t, app, "f3enc_user", "senha-enc")

	// Tentativa de POST designação -> 403
	rr, _ := doJSONReq(app, "POST", "/api/grupo/funcoes/membros", map[string]any{
		"funcao_id":    fOutra,
		"usuario_id":   uidAlvo,
		"titularidade": "titular",
	}, ck)
	if rr.Code != http.StatusForbidden {
		t.Fatalf("encarregado não pode designar membros (403), veio %d", rr.Code)
	}

	// Criar uma designação pelo banco para tentar excluir
	var memID int64
	if err := st.db.QueryRow(`INSERT INTO funcao_membros (funcao_id, grupo_id, usuario_id, titularidade) VALUES (?, ?, ?, 'titular') RETURNING id`, fOutra, gid, uidAlvo).Scan(&memID); err != nil {
		t.Fatalf("seed designacao: %v", err)
	}

	// Tentativa de DELETE designação -> 403
	rrDel, _ := doJSONReq(app, "DELETE", "/api/grupo/funcoes/membros/"+idi(memID), nil, ck)
	if rrDel.Code != http.StatusForbidden {
		t.Fatalf("encarregado não pode excluir designação (403), veio %d", rrDel.Code)
	}
}

// TestF3AntiEscalacaoRenomearItemAntiguidade testa que renomear item 'antiguidade' NÃO concede poder.
func TestF3AntiEscalacaoRenomearItemAntiguidade(t *testing.T) {
	app, st, cleanup := setupTestApp(t)
	defer cleanup()

	var gid int64
	if err := st.db.QueryRow(`INSERT INTO grupos (nome) VALUES ('Grp AntiEscala') RETURNING id`).Scan(&gid); err != nil {
		t.Fatalf("criar grupo: %v", err)
	}

	// Item do catálogo de antiguidade
	var fAntiga int64
	if err := st.db.QueryRow(`INSERT INTO funcoes (nome, grupo_id, tipo) VALUES ('Soldado Antigo', ?, 'antiguidade') RETURNING id`, gid).Scan(&fAntiga); err != nil {
		t.Fatalf("criar funcao antiguidade: %v", err)
	}

	criaUsuarioTeste(t, st, "f3adm", "senha-adm", "admin")
	criaUsuarioTeste(t, st, "f3golpe", "senha-golpe", "")
	var uidGolpe int64
	_ = st.db.QueryRow(`SELECT id FROM usuarios WHERE login='f3golpe'`).Scan(&uidGolpe)
	_, _ = st.db.Exec(`UPDATE usuarios SET grupo_id = ? WHERE id = ?`, gid, uidGolpe)

	// Admin renomeia o item de antiguidade para "Encarregado de Pessoal"
	ckAdm := loginAs(t, app, "f3adm", "senha-adm")
	rrR, _ := doJSONReq(app, "PATCH", "/api/catalogo/funcoes/"+idi(fAntiga), map[string]any{"nome": "Encarregado de Pessoal Falso"}, ckAdm)
	if rrR.Code != http.StatusOK {
		t.Fatalf("admin renomeia item de catalogo (200), veio %d", rrR.Code)
	}

	// Mesmo que o usuário seja designado nessa função renomeada:
	if _, err := st.db.Exec(`INSERT INTO funcao_membros (funcao_id, grupo_id, usuario_id, titularidade) VALUES (?, ?, ?, 'titular')`, fAntiga, gid, uidGolpe); err != nil {
		t.Fatalf("designar golpe: %v", err)
	}

	ckGolpe := loginAs(t, app, "f3golpe", "senha-golpe")

	// Usuário NÃO deve ganhar poderes de gestão de pessoal (chave='enc_pessoal' ausente)
	rrP, _ := doJSONReq(app, "POST", "/api/pessoas", map[string]any{"nome_guerra": "GOLPE", "nome_completo": "Golpe Nao Passa"}, ckGolpe)
	if rrP.Code != http.StatusForbidden {
		t.Fatalf("anti-escalação: renomear item antiguidade NÃO pode conceder poder (403), veio %d", rrP.Code)
	}
}

// TestF3ApiMeTrazFuncoesGrupo testa que GET /api/me inclui funcoes_grupo corretamente.
func TestF3ApiMeTrazFuncoesGrupo(t *testing.T) {
	app, st, cleanup := setupTestApp(t)
	defer cleanup()

	var gid int64
	if err := st.db.QueryRow(`INSERT INTO grupos (nome) VALUES ('Grp Me F3') RETURNING id`).Scan(&gid); err != nil {
		t.Fatalf("criar grupo: %v", err)
	}
	var fPess, fMat int64
	if err := st.db.QueryRow(`SELECT id FROM funcoes WHERE chave = 'enc_pessoal'`, gid).Scan(&fPess); err != nil {
		t.Fatalf("criar enc pessoal: %v", err)
	}
	if err := st.db.QueryRow(`SELECT id FROM funcoes WHERE chave = 'enc_material'`, gid).Scan(&fMat); err != nil {
		t.Fatalf("criar enc mat: %v", err)
	}

	criaUsuarioTeste(t, st, "f3user_me", "senha-me", "")
	var uid int64
	_ = st.db.QueryRow(`SELECT id FROM usuarios WHERE login='f3user_me'`).Scan(&uid)
	_, _ = st.db.Exec(`UPDATE usuarios SET grupo_id = ? WHERE id = ?`, gid, uid)

	// Designado como titular de pessoal e auxiliar de material
	_, _ = st.db.Exec(`INSERT INTO funcao_membros (funcao_id, grupo_id, usuario_id, titularidade) VALUES (?, ?, ?, 'titular')`, fPess, gid, uid)
	_, _ = st.db.Exec(`INSERT INTO funcao_membros (funcao_id, grupo_id, usuario_id, titularidade) VALUES (?, ?, ?, 'auxiliar')`, fMat, gid, uid)

	ck := loginAs(t, app, "f3user_me", "senha-me")
	rr, res := doJSONReq(app, "GET", "/api/me", nil, ck)
	if rr.Code != http.StatusOK {
		t.Fatalf("GET /api/me deve dar 200, veio %d", rr.Code)
	}

	uMap, ok := res["usuario"].(map[string]any)
	if !ok {
		t.Fatalf("res[usuario] ausente ou inválido: %v", res)
	}
	fgRaw, ok := uMap["funcoes_grupo"].([]any)
	if !ok {
		t.Fatalf("usuario.funcoes_grupo não é lista: %v", uMap["funcoes_grupo"])
	}
	if len(fgRaw) != 2 {
		t.Fatalf("esperado 2 funcoes_grupo, obtido %d", len(fgRaw))
	}

	achouPess := false
	achouMat := false
	for _, it := range fgRaw {
		item := it.(map[string]any)
		ch, _ := item["chave"].(string)
		tit, _ := item["titularidade"].(string)
		if ch == "enc_pessoal" && tit == "titular" {
			achouPess = true
		}
		if ch == "enc_material" && tit == "auxiliar" {
			achouMat = true
		}
	}
	if !achouPess || !achouMat {
		t.Fatalf("funcoes_grupo não trouxe os itens corretos: %v", fgRaw)
	}
}

// TestF3PatchCatalogoNaoAlteraChaveEBloqueiaNaoAdmin testa regras de integridade do catálogo.
func TestF3PatchCatalogoNaoAlteraChaveEBloqueiaNaoAdmin(t *testing.T) {
	app, st, cleanup := setupTestApp(t)
	defer cleanup()

	var gid int64
	if err := st.db.QueryRow(`INSERT INTO grupos (nome) VALUES ('Grp F3 Patch') RETURNING id`).Scan(&gid); err != nil {
		t.Fatalf("criar grupo: %v", err)
	}
	var fGrupo, fAntiga int64
	if err := st.db.QueryRow(`SELECT id FROM funcoes WHERE chave = 'enc_pessoal'`).Scan(&fGrupo); err != nil {
		t.Fatalf("cadeira enc_pessoal ausente: %v", err)
	}
	if err := st.db.QueryRow(`INSERT INTO funcoes (nome, grupo_id, tipo) VALUES ('Funcao Antiga F3', ?, 'antiguidade') RETURNING id`, gid).Scan(&fAntiga); err != nil {
		t.Fatalf("criar funcao antiga: %v", err)
	}

	criaUsuarioTeste(t, st, "f3adm2", "senha-adm", "admin")
	criaUsuarioTeste(t, st, "f3ger", "senha-ger", "gerente")
	_, _ = st.db.Exec(`UPDATE usuarios SET grupo_id = ? WHERE login='f3ger'`, gid)

	ckAdm := loginAs(t, app, "f3adm2", "senha-adm")
	ckGer := loginAs(t, app, "f3ger", "senha-ger")

	// Gerente tenta alterar ou excluir função tipo='grupo' -> 403
	rrGerPatch, _ := doJSONReq(app, "PATCH", "/api/catalogo/funcoes/"+idi(fGrupo), map[string]any{"nome": "Tentativa Ger"}, ckGer)
	if rrGerPatch.Code != http.StatusForbidden {
		t.Fatalf("gerente não pode editar função tipo=grupo (403), veio %d", rrGerPatch.Code)
	}
	rrGerDel, _ := doJSONReq(app, "DELETE", "/api/catalogo/funcoes/"+idi(fGrupo), nil, ckGer)
	if rrGerDel.Code != http.StatusForbidden {
		t.Fatalf("gerente não pode excluir função tipo=grupo (403), veio %d", rrGerDel.Code)
	}

	// Admin tenta passar campo chave no PATCH de antiguidade -> chave permanece NULL
	rrAdmPatch, _ := doJSONReq(app, "PATCH", "/api/catalogo/funcoes/"+idi(fAntiga), map[string]any{"nome": "Antiga Atualizada", "chave": "enc_material"}, ckAdm)
	if rrAdmPatch.Code != http.StatusOK {
		t.Fatalf("admin edita antiguidade (200), veio %d", rrAdmPatch.Code)
	}
	var chAfter *string
	_ = st.db.QueryRow(`SELECT chave FROM funcoes WHERE id = ?`, fAntiga).Scan(&chAfter)
	if chAfter != nil {
		t.Fatalf("campo chave não pode ser alterado via API: %v", *chAfter)
	}
}

// TestF3SegundaLinhaMesmaChaveViolaIndiceSemPanico testa o índice parcial único em chave.
func TestF3SegundaLinhaMesmaChaveViolaIndiceSemPanico(t *testing.T) {
	_, st, cleanup := setupTestApp(t)
	defer cleanup()

	// Inserir primeira com chave de teste (inexistente)
	if _, err := st.db.Exec(`INSERT INTO funcoes (nome, tipo, chave) VALUES ('Enc 1', 'grupo', 'enc_teste')`); err != nil {
		t.Fatalf("inserir enc 1: %v", err)
	}

	// Inserir segunda com a mesma chave -> erro UNIQUE sem pânico
	_, err := st.db.Exec(`INSERT INTO funcoes (nome, tipo, chave) VALUES ('Enc 2', 'grupo', 'enc_teste')`)
	if err == nil {
		t.Fatalf("esperava erro de UNIQUE constraint no índice parcial idx_funcoes_chave, veio nil")
	}
	if !strings.Contains(err.Error(), "UNIQUE constraint") {
		t.Fatalf("esperava erro de UNIQUE constraint, veio: %v", err)
	}
}
