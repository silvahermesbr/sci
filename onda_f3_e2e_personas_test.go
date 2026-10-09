package main

import (
	"net/http"
	"testing"
)

// TestF3E2EPersonasEMatrizPermissoes cobre as 6 personas do sistema e tentativas de bypass.
func TestF3E2EPersonasEMatrizPermissoes(t *testing.T) {
	app, st, cleanup := setupTestApp(t)
	defer cleanup()

	// 1. Criar grupo e categorias de teste
	var gid int64
	if err := st.db.QueryRow(`INSERT INTO grupos (nome) VALUES ('Grupo E2E Teste') RETURNING id`).Scan(&gid); err != nil {
		t.Fatalf("criar grupo: %v", err)
	}
	var fPess, fMat int64
	if err := st.db.QueryRow(`SELECT id FROM funcoes WHERE chave = 'enc_pessoal'`).Scan(&fPess); err != nil {
		t.Fatalf("cadeira enc_pessoal ausente: %v", err)
	}
	if err := st.db.QueryRow(`SELECT id FROM funcoes WHERE chave = 'enc_material'`).Scan(&fMat); err != nil {
		t.Fatalf("cadeira enc_material ausente: %v", err)
	}
	var matCatID int64
	if err := st.db.QueryRow(`INSERT INTO material_categorias (nome, grupo_id) VALUES ('Cat E2E', ?) RETURNING id`, gid).Scan(&matCatID); err != nil {
		t.Fatalf("criar categoria mat: %v", err)
	}

	// 2. Criar contas para as 6 personas
	// Persona 1: Admin (sem grupo)
	criaUsuarioTeste(t, st, "e2e_admin", "senha", "admin")
	// Persona 2: Gerente
	criaUsuarioTeste(t, st, "e2e_gerente", "senha", "gerente")
	// Persona 3: Encarregado de Pessoal (Titular)
	criaUsuarioTeste(t, st, "e2e_enc_pess_tit", "senha", "")
	// Persona 4: Auxiliar de Pessoal
	criaUsuarioTeste(t, st, "e2e_enc_pess_aux", "senha", "")
	// Persona 5a: Encarregado de Material (Titular)
	criaUsuarioTeste(t, st, "e2e_enc_mat_tit", "senha", "")
	// Persona 5b: Auxiliar de Material
	criaUsuarioTeste(t, st, "e2e_enc_mat_aux", "senha", "")
	// Persona 6a: Operador
	criaUsuarioTeste(t, st, "e2e_operador", "senha", "operador")
	// Persona 6b: Chefe de setor
	criaUsuarioTeste(t, st, "e2e_chefe", "senha", "chefe_setor")

	// Vincular contas ao grupo
	_, _ = st.db.Exec(`UPDATE usuarios SET grupo_id = ? WHERE login != 'e2e_admin'`, gid)

	// Obter IDs dos usuários
	var idTitPess, idAuxPess, idTitMat, idAuxMat, idOperador int64
	_ = st.db.QueryRow(`SELECT id FROM usuarios WHERE login='e2e_enc_pess_tit'`).Scan(&idTitPess)
	_ = st.db.QueryRow(`SELECT id FROM usuarios WHERE login='e2e_enc_pess_aux'`).Scan(&idAuxPess)
	_ = st.db.QueryRow(`SELECT id FROM usuarios WHERE login='e2e_enc_mat_tit'`).Scan(&idTitMat)
	_ = st.db.QueryRow(`SELECT id FROM usuarios WHERE login='e2e_enc_mat_aux'`).Scan(&idAuxMat)
	_ = st.db.QueryRow(`SELECT id FROM usuarios WHERE login='e2e_operador'`).Scan(&idOperador)

	// Designar Pessoal (titular e auxiliar)
	if _, err := st.db.Exec(`INSERT INTO funcao_membros (funcao_id, grupo_id, usuario_id, titularidade) VALUES (?, ?, ?, 'titular')`, fPess, gid, idTitPess); err != nil {
		t.Fatalf("designar titular pess: %v", err)
	}
	if _, err := st.db.Exec(`INSERT INTO funcao_membros (funcao_id, grupo_id, usuario_id, titularidade) VALUES (?, ?, ?, 'auxiliar')`, fPess, gid, idAuxPess); err != nil {
		t.Fatalf("designar aux pess: %v", err)
	}
	// Designar Material (titular e auxiliar)
	if _, err := st.db.Exec(`INSERT INTO funcao_membros (funcao_id, grupo_id, usuario_id, titularidade) VALUES (?, ?, ?, 'titular')`, fMat, gid, idTitMat); err != nil {
		t.Fatalf("designar titular mat: %v", err)
	}
	if _, err := st.db.Exec(`INSERT INTO funcao_membros (funcao_id, grupo_id, usuario_id, titularidade) VALUES (?, ?, ?, 'auxiliar')`, fMat, gid, idAuxMat); err != nil {
		t.Fatalf("designar aux mat: %v", err)
	}

	// Login das personas
	ckAdmin := loginAs(t, app, "e2e_admin", "senha")
	ckGerente := loginAs(t, app, "e2e_gerente", "senha")
	ckTitPess := loginAs(t, app, "e2e_enc_pess_tit", "senha")
	ckAuxPess := loginAs(t, app, "e2e_enc_pess_aux", "senha")
	ckTitMat := loginAs(t, app, "e2e_enc_mat_tit", "senha")
	ckAuxMat := loginAs(t, app, "e2e_enc_mat_aux", "senha")
	ckOperador := loginAs(t, app, "e2e_operador", "senha")
	ckChefe := loginAs(t, app, "e2e_chefe", "senha")

	// ==========================================
	// PERSONA 1: Admin (sem grupo)
	// ==========================================
	// Mural global: 200
	rr, _ := doJSONReq(app, "GET", "/api/avisos", nil, ckAdmin)
	if rr.Code != http.StatusOK {
		t.Fatalf("Admin deve acessar avisos (200), veio %d", rr.Code)
	}
	// Rota operacional de material (authMaterial): 403
	rr, _ = doJSONReq(app, "GET", "/api/material/itens", nil, ckAdmin)
	if rr.Code != http.StatusForbidden {
		t.Fatalf("Admin sem grupo deve receber 403 em /api/material/itens, veio %d", rr.Code)
	}
	// Iniciar conferência operacional de grupo: 403
	rr, _ = doJSONReq(app, "POST", "/api/conferencia/iniciar", map[string]any{}, ckAdmin)
	if rr.Code != http.StatusForbidden {
		t.Fatalf("Admin sem grupo deve receber 403 ao iniciar conferência operacional, veio %d", rr.Code)
	}

	// ==========================================
	// PERSONA 2: Gerente
	// ==========================================
	// Conferência: 200
	rr, _ = doJSONReq(app, "GET", "/api/conferencias/hoje", nil, ckGerente)
	if rr.Code != http.StatusOK {
		t.Fatalf("Gerente deve acessar conferencias/hoje (200), veio %d", rr.Code)
	}
	// Pessoal: 200
	rr, _ = doJSONReq(app, "GET", "/api/pessoas", nil, ckGerente)
	if rr.Code != http.StatusOK {
		t.Fatalf("Gerente deve acessar pessoas (200), veio %d", rr.Code)
	}
	// Material: 200
	rr, _ = doJSONReq(app, "GET", "/api/material/itens", nil, ckGerente)
	if rr.Code != http.StatusOK {
		t.Fatalf("Gerente deve acessar material/itens (200), veio %d", rr.Code)
	}
	// Gerenciar Grupo: 200
	rr, _ = doJSONReq(app, "GET", "/api/grupos/arvore", nil, ckGerente)
	if rr.Code != http.StatusOK {
		t.Fatalf("Gerente deve acessar grupos/arvore (200), veio %d", rr.Code)
	}
	// Mural Avisos: 200
	rr, _ = doJSONReq(app, "GET", "/api/avisos", nil, ckGerente)
	if rr.Code != http.StatusOK {
		t.Fatalf("Gerente deve acessar avisos (200), veio %d", rr.Code)
	}
	// v39: funções de grupo são HARDCODED — API NÃO cria (400); gerente só designa
	rr, _ = doJSONReq(app, "POST", "/api/catalogo/funcoes", map[string]any{"nome": "Nova Função Gerente", "tipo": "grupo"}, ckGerente)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("Criar função de grupo via API deve ser 400 (hardcoded), veio %d", rr.Code)
	}
	// Designa membro: 200
	rr, _ = doJSONReq(app, "POST", "/api/grupo/funcoes/membros", map[string]any{"funcao_id": fPess, "usuario_id": idOperador, "titularidade": "auxiliar"}, ckGerente)
	if rr.Code != http.StatusOK && rr.Code != http.StatusCreated {
		t.Fatalf("Gerente deve conseguir designar membro (200/201), veio %d", rr.Code)
	}

	// ==========================================
	// PERSONA 3: Encarregado de Pessoal (Titular)
	// ==========================================
	// Conferência: 200
	rr, _ = doJSONReq(app, "GET", "/api/conferencias/hoje", nil, ckTitPess)
	if rr.Code != http.StatusOK {
		t.Fatalf("Enc Pessoal deve acessar conferencias/hoje (200), veio %d", rr.Code)
	}
	// Pessoal (gestor - pode criar/editar): 200
	rr, _ = doJSONReq(app, "POST", "/api/pessoas", map[string]any{"nome_guerra": "Novo TitPess", "nome_completo": "Militar Titular", "status": "ativo"}, ckTitPess)
	if rr.Code != http.StatusOK && rr.Code != http.StatusCreated {
		t.Fatalf("Enc Pessoal deve conseguir criar pessoa (200/201), veio %d", rr.Code)
	}
	// Material: 403
	rr, _ = doJSONReq(app, "GET", "/api/material/itens", nil, ckTitPess)
	if rr.Code != http.StatusForbidden {
		t.Fatalf("Enc Pessoal NÃO deve acessar material (403), veio %d", rr.Code)
	}
	// Tenta designar membro: 403
	rr, _ = doJSONReq(app, "POST", "/api/grupo/funcoes/membros", map[string]any{"funcao_id": fPess, "usuario_id": idOperador, "titularidade": "auxiliar"}, ckTitPess)
	if rr.Code != http.StatusForbidden {
		t.Fatalf("Enc Pessoal NÃO pode designar membros (403), veio %d", rr.Code)
	}

	// ==========================================
	// PERSONA 4: Auxiliar de Pessoal
	// ==========================================
	// Mesmos poderes e restrições do titular:
	rr, _ = doJSONReq(app, "GET", "/api/conferencias/hoje", nil, ckAuxPess)
	if rr.Code != http.StatusOK {
		t.Fatalf("Aux Pessoal deve acessar conferencias/hoje (200), veio %d", rr.Code)
	}
	rr, _ = doJSONReq(app, "POST", "/api/pessoas", map[string]any{"nome_guerra": "Novo AuxPess", "nome_completo": "Militar Auxiliar", "status": "ativo"}, ckAuxPess)
	if rr.Code != http.StatusOK && rr.Code != http.StatusCreated {
		t.Fatalf("Aux Pessoal deve conseguir criar pessoa (200/201), veio %d", rr.Code)
	}
	rr, _ = doJSONReq(app, "GET", "/api/material/itens", nil, ckAuxPess)
	if rr.Code != http.StatusForbidden {
		t.Fatalf("Aux Pessoal NÃO deve acessar material (403), veio %d", rr.Code)
	}
	rr, _ = doJSONReq(app, "POST", "/api/grupo/funcoes/membros", map[string]any{"funcao_id": fPess, "usuario_id": idOperador, "titularidade": "auxiliar"}, ckAuxPess)
	if rr.Code != http.StatusForbidden {
		t.Fatalf("Aux Pessoal NÃO pode designar membros (403), veio %d", rr.Code)
	}

	// ==========================================
	// PERSONA 5: Encarregado de Material (Titular & Auxiliar)
	// ==========================================
	for nome, ck := range map[string]*http.Cookie{"Titular Mat": ckTitMat, "Auxiliar Mat": ckAuxMat} {
		// Conferência: 200
		rr, _ = doJSONReq(app, "GET", "/api/conferencias/hoje", nil, ck)
		if rr.Code != http.StatusOK {
			t.Fatalf("%s deve acessar conferencias/hoje (200), veio %d", nome, rr.Code)
		}
		// Material: 200
		rr, _ = doJSONReq(app, "GET", "/api/material/itens", nil, ck)
		if rr.Code != http.StatusOK {
			t.Fatalf("%s deve acessar material (200), veio %d", nome, rr.Code)
		}
		// Gestão de Pessoal: 403
		rr, _ = doJSONReq(app, "POST", "/api/pessoas", map[string]any{"nome_guerra": "Invasor Mat"}, ck)
		if rr.Code != http.StatusForbidden {
			t.Fatalf("%s NÃO deve gerenciar pessoas (403), veio %d", nome, rr.Code)
		}
		// Tenta designar: 403
		rr, _ = doJSONReq(app, "POST", "/api/grupo/funcoes/membros", map[string]any{"funcao_id": fMat, "usuario_id": idOperador, "titularidade": "auxiliar"}, ck)
		if rr.Code != http.StatusForbidden {
			t.Fatalf("%s NÃO pode designar membros (403), veio %d", nome, rr.Code)
		}
	}

	// ==========================================
	// PERSONA 6: Operador & Chefe de Setor (doutrina v367)
	// O Operador FOI designado AUXILIAR de Enc. Pessoal no setup (linha do
	// ckGerente) — v367: designação manda em qualquer papel, então ele TEM
	// gestão de pessoal. O Chefe de Setor NÃO tem designação → segue 403
	// (fail-closed preservado).
	// ==========================================
	// Operador (aux de enc_pessoal designado): conferência 200...
	rr, _ = doJSONReq(app, "GET", "/api/conferencias/hoje", nil, ckOperador)
	if rr.Code != http.StatusOK {
		t.Fatalf("Operador deve acessar conferencias/hoje (200), veio %d", rr.Code)
	}
	// ...e gestão de pessoal ABERTA (cria no próprio grupo, payload válido → 200)
	rr, _ = doJSONReq(app, "POST", "/api/pessoas", map[string]any{"nome_guerra": "Criado Pelo Operador", "nome_completo": "Criado Pelo Operador Aux"}, ckOperador)
	if rr.Code != http.StatusOK {
		t.Fatalf("Operador designado aux de Enc. Pessoal deve gerenciar pessoas (200), veio %d", rr.Code)
	}
	// Operador NÃO pode designar membros (designação é exclusiva de gerente/admin)
	rr, _ = doJSONReq(app, "POST", "/api/grupo/funcoes/membros", map[string]any{"funcao_id": fPess, "usuario_id": idOperador, "titularidade": "auxiliar"}, ckOperador)
	if rr.Code != http.StatusForbidden {
		t.Fatalf("Operador não pode designar membros (403), veio %d", rr.Code)
	}
	// Chefe de Setor SEM designação: conferência 200, gestão de pessoal 403
	rr, _ = doJSONReq(app, "GET", "/api/conferencias/hoje", nil, ckChefe)
	if rr.Code != http.StatusOK {
		t.Fatalf("Chefe deve acessar conferencias/hoje (200), veio %d", rr.Code)
	}
	rr, _ = doJSONReq(app, "POST", "/api/pessoas", map[string]any{"nome_guerra": "Invasor Chefe"}, ckChefe)
	if rr.Code != http.StatusForbidden {
		t.Fatalf("Chefe sem designação NÃO deve gerenciar pessoas (403), veio %d", rr.Code)
	}

	// ==========================================
	// TENTATIVAS DE BYPASS (Anti-Escalação)
	// ==========================================
	// 1. Operador cria/renomeia item de catálogo de antiguidade para "Encarregado de Pessoal"
	var fFalsa int64
	if err := st.db.QueryRow(`INSERT INTO funcoes (nome, grupo_id, tipo) VALUES ('Funcao Normal Ev', ?, 'antiguidade') RETURNING id`, gid).Scan(&fFalsa); err != nil {
		t.Fatalf("criar funcao falsa: %v", err)
	}
	// Atribui essa função a um militar vinculado ao operador
	criaUsuarioTeste(t, st, "e2e_malicioso", "senha", "")
	var idMalicioso int64
	_ = st.db.QueryRow(`SELECT id FROM usuarios WHERE login='e2e_malicioso'`).Scan(&idMalicioso)
	_, _ = st.db.Exec(`UPDATE usuarios SET grupo_id = ?, funcao_id = ? WHERE id = ?`, gid, fFalsa, idMalicioso)
	ckMalicioso := loginAs(t, app, "e2e_malicioso", "senha")

	// Renomear a função para "Encarregado de Pessoal"
	_, _ = st.db.Exec(`UPDATE funcoes SET nome = 'Encarregado de Pessoal' WHERE id = ?`, fFalsa)

	// Conta maliciosa tenta gerenciar /api/pessoas -> DEVE SER 403 (detecção é por chave, não por nome)
	rr, _ = doJSONReq(app, "POST", "/api/pessoas", map[string]any{"nome_guerra": "Bypass"}, ckMalicioso)
	if rr.Code != http.StatusForbidden {
		t.Fatalf("Anti-escalação violada! Renomear item deu poder indevido (esperado 403, veio %d)", rr.Code)
	}

	// 2. Operador tenta criar função com tipo='grupo' -> JAMIS cria (v367:
	// designado passa pela guarda de middleware e morre no gate hardcoded 400;
	// sem designação morre 403 no middleware — o invariante é NÃO CRIAR).
	rr, _ = doJSONReq(app, "POST", "/api/catalogo/funcoes", map[string]any{"nome": "Tentativa Hacker", "tipo": "grupo"}, ckOperador)
	if rr.Code == http.StatusOK || rr.Code == http.StatusCreated {
		t.Fatalf("Operador NÃO pode criar função tipo=grupo (veio %d)", rr.Code)
	}
	var nHack int
	if err := st.db.QueryRow(`SELECT COUNT(*) FROM funcoes WHERE nome='Tentativa Hacker'`).Scan(&nHack); err != nil || nHack != 0 {
		t.Fatalf("função tipo=grupo do operador NÃO pode ser criada (count=%d, err=%v)", nHack, err)
	}
}
