package main

// ordem_diretor_0410_test.go — testes da rodada de alterações do Diretor
// (email "Alterações SCI", 04/10/2026 23:01). Fases com superfície de API:
//
//   A. Meu Perfil — troca de senha própria: atual + nova + confirmação
//      (workflow hTrocarSenha via POST /api/senha). O bug do front chamava
//      /api/usuarios/undefined/senha; aqui se prova o CONTRATO do endpoint
//      que o modal corrigido consome (motivo, confirmação e rejeições).
//   B. Relatórios — assinatura por NOME COMPLETO (nunca login), nome em
//      negrito e função específica do grupo logo abaixo (se houver).
//   C. Grupos — gerente pode ser QUALQUER conta indicada pelo admin,
//      sem exigência de já ser membro do grupo.
//
// Sem superfície de API (só front): janelas rich-text, anexos em comentários,
// painel de consciência (revertido), módulos em modo desenvolvimento,
// dropdown de abas, mensageria renomeada (UI), roadmap.

import (
	"fmt"
	"net/http"
	"strings"
	"testing"
)

// ============================ FASE A — PERFIL ============================

func TestTrocaSenhaPropriaWorkflow(t *testing.T) {
	app, st, cleanup := setupTestApp(t)
	defer cleanup()

	// usuário comum com senha conhecida (criado direto no banco)
	criaUsuarioTeste(t, st, "opperfil", "senha-antiga-xyz", "operador")
	ck := loginAs(t, app, "opperfil", "senha-antiga-xyz")

	// 1) troca com workflow completo: atual correta + nova + confirmação implícita
	rr, res := doJSONReq(app, "POST", "/api/senha", map[string]string{"atual": "senha-antiga-xyz", "nova": "nova-senha-9876"}, ck)
	if rr.Code != http.StatusOK {
		t.Fatalf("troca de senha válida deve 200, veio %d: %v", rr.Code, res)
	}

	// senha NOVA abre sessão; senha ANTIGA é recusada — prova que gravou
	if _, res := doJSONReq(app, "POST", "/api/login", map[string]string{"login": "opperfil", "senha": "nova-senha-9876"}, nil); res["erro"] != nil {
		t.Fatalf("login com a nova senha deveria funcionar: %v", res)
	}
	rr2, res2 := doJSONReq(app, "POST", "/api/login", map[string]string{"login": "opperfil", "senha": "senha-antiga-xyz"}, nil)
	if rr2.Code == http.StatusOK {
		t.Fatalf("login com senha antiga deveria ser recusado após a troca: %v", res2)
	}

	// 2) senha ATUAL errada → 401 (o "preflight" do modal)
	ck = loginAs(t, app, "opperfil", "nova-senha-9876")
	rr3, _ := doJSONReq(app, "POST", "/api/senha", map[string]string{"atual": "errada", "nova": "outra-senha-123"}, ck)
	if rr3.Code != http.StatusUnauthorized {
		t.Fatalf("senha atual errada deve 401, veio %d", rr3.Code)
	}

	// 3) nova senha curta → 400
	rr4, _ := doJSONReq(app, "POST", "/api/senha", map[string]string{"atual": "nova-senha-9876", "nova": "curta"}, ck)
	if rr4.Code != http.StatusBadRequest {
		t.Fatalf("nova senha curta deve 400, veio %d", rr4.Code)
	}

	// 4) sem senha atual → 400
	rr5, _ := doJSONReq(app, "POST", "/api/senha", map[string]string{"nova": "so-nova-12345"}, ck)
	if rr5.Code != http.StatusBadRequest {
		t.Fatalf("sem senha atual deve 400, veio %d", rr5.Code)
	}
}

// ========================= FASE B — RELATÓRIOS ==========================

// Assinatura por NOME COMPLETO (nunca login), nome em negrito e função do
// grupo logo abaixo (se houver). O PDF da conferência fechada é a peça
// central; extrairTextoPDF prova o conteúdo real dos streams.
func TestAssinaturaNomeCompletoConferencia(t *testing.T) {
	app, st, cleanup := setupTestApp(t)
	defer cleanup()

	// gerente com nome completo + função (catálogo do grupo)
	ck := loginAsPapel(t, app, st, "seedger", "gerente")
	var gid int64
	if err := st.db.QueryRow(`INSERT INTO grupos (nome) VALUES ('Grp Assin') RETURNING id`).Scan(&gid); err != nil {
		t.Fatalf("criar grupo: %v", err)
	}
	var fid int64
	if err := st.db.QueryRow(`INSERT INTO funcoes (nome, grupo_id) VALUES ('Chefe Seção Comunicação', ?) RETURNING id`, gid).Scan(&fid); err != nil {
		t.Fatalf("criar função: %v", err)
	}
	if _, err := st.db.Exec(`UPDATE usuarios SET nome_completo = 'Capitão Silva Alfa Completo', nome_guerra = 'Silva', grupo_id = ?, funcao_id = ? WHERE login = 'seedger'`, gid, fid); err != nil {
		t.Fatalf("dados do gerente: %v", err)
	}

	// tipo + conferência JÁ FECHADA (relatório exige fechada)
	var tid int64
	if err := st.db.QueryRow(`INSERT INTO conferencia_tipos (nome) VALUES ('Ordem Unica') RETURNING id`).Scan(&tid); err != nil {
		t.Fatalf("criar tipo: %v", err)
	}
	var cid int64
	if err := st.db.QueryRow(`INSERT INTO conferencias (data, tipo_id, grupo_id, status, criado_por, criado_em, fechada_em)
		VALUES ('2026-10-04', ?, ?, 'fechada', (SELECT id FROM usuarios WHERE login='seedger'), '2026-10-04T10:00:00Z', '2026-10-04T12:00:00Z') RETURNING id`, tid, gid).Scan(&cid); err != nil {
		t.Fatalf("criar conferência fechada: %v", err)
	}

	rr := doRawReqH(app, "GET", fmt.Sprintf("/api/conferencia/%d/relatorio.pdf", cid), nil, "", ck)
	if rr.Code != http.StatusOK {
		t.Fatalf("PDF da conferência fechada deve 200, veio %d: %s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Header().Get("Content-Type"), "pdf") {
		t.Fatalf("esperado PDF, veio %q", rr.Header().Get("Content-Type"))
	}
	pdf := extrairTextoPDF(t, rr.Body.Bytes())
	if !strings.Contains(pdf, "Capitao Silva Alfa Completo") {
		t.Errorf("assinatura deve trazer NOME COMPLETO (negrito); extrato: %.400s", pdf)
	}
	if !strings.Contains(pdf, "Chefe Secao Comunicacao") {
		t.Errorf("assinatura deve trazer a função do grupo abaixo do nome; extrato: %.400s", pdf)
	}
	if strings.Contains(pdf, "seedger") {
		t.Errorf("assinatura NUNCA deve mostrar o login; 'seedger' apareceu no PDF")
	}
}

// =========================== FASE C — GRUPOS ============================

// Gerente pode ser QUALQUER conta indicada pelo admin (ordem 04/10: reverte
// a exigência de o gerente ser membro do grupo).
func TestGerentePodeSerExternoAoGrupo(t *testing.T) {
	app, st, cleanup := setupTestApp(t)
	defer cleanup()

	ckAdmin := loginAs(t, app, "admin", "admin123")

	// grupo e uma conta SEM grupo (forasteiro)
	ckOutro := loginAsPapel(t, app, st, "forasteiro", "operador", "senha-forasteiro")
	var gid int64
	if err := st.db.QueryRow(`INSERT INTO grupos (nome) VALUES ('Grp Destino') RETURNING id`).Scan(&gid); err != nil {
		t.Fatalf("criar grupo: %v", err)
	}

	rr, res := doJSONReq(app, "POST", fmt.Sprintf("/api/grupos/%d/trocar-gerente", gid), map[string]string{"login": "forasteiro"}, ckAdmin)
	if rr.Code != http.StatusOK {
		t.Fatalf("admin deve poder indicar gerente EXTERNO ao grupo (ordem 04/10); veio %d: %v", rr.Code, res)
	}

	// prova de efeito: a conta foi promovida e vinculada ao grupo
	var papel string
	var grupo *int64
	if err := st.db.QueryRow(`SELECT papel, grupo_id FROM usuarios WHERE login = 'forasteiro'`).Scan(&papel, &grupo); err != nil {
		t.Fatalf("ler usuário: %v", err)
	}
	if papel != "gerente" || grupo == nil || *grupo != gid {
		t.Fatalf("esperado gerente do grupo %d; papel=%s grupo=%v", gid, papel, grupo)
	}
	_ = ckOutro
}
