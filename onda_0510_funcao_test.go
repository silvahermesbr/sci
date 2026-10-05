package main

// Testes da onda "Drive/Email POR FUNÇÃO" (DELIB-0010 B + Q1/Q3, 05/10).
//
// Princípio: a FUNÇÃO é a dona do registro; o titular atual VÊ o que a função
// possui e o EX-TITULAR deixa de ver (mas mantém o que é dele). Mensagem da
// função chega ao novo titular na Caixa da Função e NÃO é arquivável.
//
//   FV1. Titular publica no drive → item nasce carimbado com funcao_id.
//   FV2. Titular vê o item carimbado via /api/drive/da_funcao; usuário da
//        mesma função (legado/auxiliar-caminho) e SEM relação não vê.
//   FV3. Troca de titular: novo titular vê; ex-titular NÃO vê mais (mas vê
//        os seus); NULL-legado preservado (não aparece para ninguém da função).
//   FV4. Mensagem enviada pelo titular nasce funcao_id → chega ao NOVO titular
//        no inbox com da_funcao=true (UNION, sem duplicar para o remetente).
//   FV5. Arquivar item da função exercida → 400 (trava); arquivar pessoal → 200.

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

// fnSetup: grupo + função + titular/operador ligados por funcao_membros.
// O titular é inserido DIRETO no banco (mesma doutrina de onda_0510_c2_test.go).
// Senha padrão "senha-gerente": é a que o vinculaGrupoDoLogin usa no login final.
func fnSetup(t *testing.T, app *App, st *Store) (gid, funcaoID, titUID, opUID, titPapelID int64) {
	t.Helper()
	if err := st.db.QueryRow(`INSERT INTO grupos (nome) VALUES ('Grp FN') RETURNING id`).Scan(&gid); err != nil {
		t.Fatalf("criar grupo: %v", err)
	}
	if err := st.db.QueryRow(`INSERT INTO funcoes (nome) VALUES ('Oficial de Dia FN') RETURNING id`).Scan(&funcaoID); err != nil {
		t.Fatalf("criar função: %v", err)
	}
	criaUsuarioTeste(t, st, "fntit", "senha-gerente", "gerente")
	criaUsuarioTeste(t, st, "fnop", "senha-gerente", "operador")
	vinculaGrupoDoLogin(t, app, st, "fntit", gid)
	vinculaGrupoDoLogin(t, app, st, "fnop", gid)
	if err := st.db.QueryRow(`SELECT id FROM usuarios WHERE login='fntit'`).Scan(&titUID); err != nil {
		t.Fatalf("id titular: %v", err)
	}
	if err := st.db.QueryRow(`SELECT id FROM usuarios WHERE login='fnop'`).Scan(&opUID); err != nil {
		t.Fatalf("id operador: %v", err)
	}
	if _, err := st.db.Exec(`INSERT INTO funcao_membros (funcao_id, grupo_id, usuario_id, titularidade) VALUES (?,?,?,'titular')`, funcaoID, gid, titUID); err != nil {
		t.Fatalf("designar titular: %v", err)
	}
	if err := st.db.QueryRow(`SELECT id FROM usuario_papeis WHERE usuario_id=? AND grupo_id=?`, titUID, gid).Scan(&titPapelID); err != nil {
		t.Fatalf("papel do titular: %v", err)
	}
	return
}

// TestFuncaoDriveCarimboEVISAO: FV1 + FV2.
func TestFuncaoDriveCarimboEVISAO(t *testing.T) {
	app, st, cleanup := setupTestApp(t)
	defer cleanup()
	gid, funcaoID, _, _, _ := fnSetup(t, app, st)

	// Sanity: login da sessão enxerga a função exercida (titular).
	ckTit := loginAs(t, app, "fntit", "senha-gerente")

	// FV1: titular cria pasta via API → nasce carimbada com funcao_id.
	rrP, resP := doJSONReq(app, "POST", "/api/drive/pastas", map[string]any{"nome": "Pasta da OD"}, ckTit)
	if rrP.Code != http.StatusOK {
		t.Fatalf("FV1: criar pasta 200, veio %d (%v)", rrP.Code, resP)
	}
	var fID *int64
	if err := st.db.QueryRow(`SELECT funcao_id FROM drive_pastas WHERE id = ?`, int64(resP["id"].(float64))).Scan(&fID); err != nil || fID == nil || *fID != funcaoID {
		t.Fatalf("FV1: pasta não carimbada com funcao_id (v=%v err=%v)", fID, err)
	}

	// FV2: titular vê o item em /api/drive/da_funcao.
	rrL, _ := doJSONReq(app, "GET", "/api/drive/da_funcao", nil, ckTit)
	if rrL.Code != http.StatusOK {
		t.Fatalf("FV2: da_funcao 200, veio %d", rrL.Code)
	}
	var lista struct {
		Pastas []map[string]any `json:"pastas"`
	}
	if err := json.Unmarshal(rrL.Body.Bytes(), &lista); err != nil {
		t.Fatalf("FV2: payload inválido: %v", err)
	}
	if len(lista.Pastas) != 1 || int64(lista.Pastas[0]["id"].(float64)) != int64(resP["id"].(float64)) {
		t.Fatalf("FV2: titular deveria ver exatamente a pasta da função; veio %v", lista.Pastas)
	}

	// Operador SEM função não vê nada (mesmo grupo).
	ckOp := loginAs(t, app, "fnop", "senha-gerente")
	rrO, _ := doJSONReq(app, "GET", "/api/drive/da_funcao", nil, ckOp)
	var listaO struct {
		Pastas []map[string]any `json:"pastas"`
	}
	_ = json.Unmarshal(rrO.Body.Bytes(), &listaO)
	if len(listaO.Pastas) != 0 {
		t.Fatalf("FV2: operador sem função não deveria ver itens; veio %v", listaO.Pastas)
	}

	// Admin → 403 (drive operacional não é do admin).
	criaUsuarioTeste(t, st, "fnadm", "senha-adm", "admin")
	ckAdm := loginAs(t, app, "fnadm", "senha-adm")
	if rrA, _ := doJSONReq(app, "GET", "/api/drive/da_funcao", nil, ckAdm); rrA.Code != http.StatusForbidden {
		t.Fatalf("FV2: admin deve 403 no da_funcao, veio %d", rrA.Code)
	}

	_ = gid
}

// TestFuncaoTrocaTitular: FV3 + FV4 — herança pelo SUCESSOR; ex-titular perde
// a visão da função mas mantém os próprios itens; NULL-legado fica preserved.
func TestFuncaoTrocaTitular(t *testing.T) {
	app, st, cleanup := setupTestApp(t)
	defer cleanup()
	gid, funcaoID, titUID, _, titPapelID := fnSetup(t, app, st)
	ckTit := loginAs(t, app, "fntit", "senha-gerente")

	// Titular publica pasta + mensagem carimbadas.
	rrP, resP := doJSONReq(app, "POST", "/api/drive/pastas", map[string]any{"nome": "Pasta OD"}, ckTit)
	if rrP.Code != http.StatusOK {
		t.Fatalf("sanity pasta: %d (%v)", rrP.Code, resP)
	}
	pastaID := int64(resP["id"].(float64))
	rrM, resM := doJSONReq(app, "POST", "/api/mensagens", map[string]any{
		"assunto": "Ordem de serviço da função", "corpo": "<p>cumpra-se</p>",
		"tipo": "comum", "destinatario_papel_ids": []int64{titPapelID},
	}, ckTit)
	if rrM.Code != http.StatusOK {
		t.Fatalf("sanity mensagem: %d (%v)", rrM.Code, resM)
	}
	msgID := int64(resM["id"].(float64))
	var fMsg *int64
	if err := st.db.QueryRow(`SELECT funcao_id FROM mensagens WHERE id=?`, msgID).Scan(&fMsg); err != nil || fMsg == nil || *fMsg != funcaoID {
		t.Fatalf("mensagem não carimbada (v=%v err=%v)", fMsg, err)
	}

	// Mensagem pessoal legada (sem função) para o ex-titular continuar vendo.
	rrL, resL := doJSONReq(app, "POST", "/api/mensagens", map[string]any{
		"assunto": "Recado pessoal", "corpo": "<p>seu</p>",
		"tipo": "comum", "destinatario_papel_ids": []int64{titPapelID},
	}, ckTit)
	if rrL.Code != http.StatusOK {
		t.Fatalf("sanity msg pessoal: %d (%v)", rrL.Code, resL)
	}
	msgPessoalID := int64(resL["id"].(float64))
	// Força a pessoal como legado (NULL) — a do titular carimba pela resolução.
	if _, err := st.db.Exec(`UPDATE mensagens SET funcao_id = NULL WHERE id=?`, msgPessoalID); err != nil {
		t.Fatalf("zerar funcao_id da pessoal: %v", err)
	}

	// TROCA DE TITULAR: operador assume a função.
	opUID := int64(0)
	if err := st.db.QueryRow(`SELECT id FROM usuarios WHERE login='fnop'`).Scan(&opUID); err != nil {
		t.Fatalf("id operador: %v", err)
	}
	if _, err := st.db.Exec(`DELETE FROM funcao_membros WHERE funcao_id=? AND grupo_id=? AND usuario_id=?`, funcaoID, gid, titUID); err != nil {
		t.Fatalf("remover titular antigo: %v", err)
	}
	if _, err := st.db.Exec(`INSERT INTO funcao_membros (funcao_id, grupo_id, usuario_id, titularidade) VALUES (?,?,?,'titular')`, funcaoID, gid, opUID); err != nil {
		t.Fatalf("designar novo titular: %v", err)
	}

	// FV3: NOVO titular vê a pasta da função.
	ckOp := loginAs(t, app, "fnop", "senha-gerente")
	rrNovo, _ := doJSONReq(app, "GET", "/api/drive/da_funcao", nil, ckOp)
	var listaN struct {
		Pastas []map[string]any `json:"pastas"`
	}
	if err := json.Unmarshal(rrNovo.Body.Bytes(), &listaN); err != nil {
		t.Fatalf("payload novo titular: %v", err)
	}
	achou := false
	for _, p := range listaN.Pastas {
		if int64(p["id"].(float64)) == pastaID {
			achou = true
		}
	}
	if !achou {
		t.Fatalf("FV3: novo titular deveria ver a pasta da função; veio %v", listaN.Pastas)
	}

	// EX-titular NÃO vê mais a pasta da função…
	rrEx, _ := doJSONReq(app, "GET", "/api/drive/da_funcao", nil, ckTit)
	var listaE struct {
		Pastas []map[string]any `json:"pastas"`
	}
	_ = json.Unmarshal(rrEx.Body.Bytes(), &listaE)
	for _, p := range listaE.Pastas {
		if int64(p["id"].(float64)) == pastaID {
			t.Fatalf("FV3: ex-titular NÃO deveria ver mais a pasta da função")
		}
	}

	// NULL-legado preservado: pasta de usuário (sem função) não vaza para ninguém.
	if _, err := st.db.Exec(`INSERT INTO drive_pastas (nome, grupo_id, autor_usuario_id, autor_papel_id) VALUES ('Legado do ex', ?, ?, ?)`, gid, titUID, titPapelID); err != nil {
		t.Fatalf("criar pasta legado: %v", err)
	}
	rrN2, _ := doJSONReq(app, "GET", "/api/drive/da_funcao", nil, ckOp)
	var listaN2 struct {
		Pastas []map[string]any `json:"pastas"`
	}
	_ = json.Unmarshal(rrN2.Body.Bytes(), &listaN2)
	for _, p := range listaN2.Pastas {
		if s, _ := p["nome"].(string); strings.Contains(s, "Legado") {
			t.Fatalf("FV3: pasta NULL-legado não deveria aparecer na caixa da função")
		}
	}

	// FV4: inbox do NOVO titular traz a mensagem da função com da_funcao=true…
	rrIn, _ := doJSONReq(app, "GET", "/api/mensagens/inbox", nil, ckOp)
	// (o operador precisa ser destinatário? NÃO — a da função entra pelo funcao_id.)
	// O inbox do novo titular: braço da função (funcao_id exercida).
	var inbox []map[string]any
	if err := json.Unmarshal(rrIn.Body.Bytes(), &inbox); err != nil {
		t.Fatalf("inbox não é array: %v", err)
	}
	achouFn, achouPessoal := false, false
	for _, m := range inbox {
		id := int64(m["id"].(float64))
		if id == msgID {
			if v, _ := m["da_funcao"].(bool); !v {
				t.Fatalf("FV4: mensagem da função sem da_funcao=true")
			}
			achouFn = true
		}
		if id == msgPessoalID {
			achouPessoal = true // destinatário é o papel do TITULAR, não do op — não deve vir
		}
	}
	if !achouFn {
		t.Fatalf("FV4: mensagem carimbada deveria chegar ao novo titular (funcao_id exercida)")
	}
	if achouPessoal {
		t.Fatalf("FV4: mensagem pessoal do ex-titular não deveria vazar ao novo titular")
	}

	// …e continua chegando ao ex-titular como PESSOAL (linha em destinatários).
	rrInEx, _ := doJSONReq(app, "GET", "/api/mensagens/inbox", nil, ckTit)
	var inboxEx []map[string]any
	_ = json.Unmarshal(rrInEx.Body.Bytes(), &inboxEx)
	for _, m := range inboxEx {
		if int64(m["id"].(float64)) == msgID {
			if v, _ := m["da_funcao"].(bool); v {
				t.Fatalf("FV4: ex-titular não deveria mais ver a msg como da função")
			}
		}
		if int64(m["id"].(float64)) == msgPessoalID {
			if v, _ := m["da_funcao"].(bool); v {
				t.Fatalf("FV4: msg pessoal marcada como da função")
			}
		}
	}

	_ = titPapelID
}

// TestFuncaoArquivarBloqueado: FV5 — titular NÃO arquiva item da função
// exercida (400); mensagem pessoal arquivam normal (200, regressão).
func TestFuncaoArquivarBloqueado(t *testing.T) {
	app, st, cleanup := setupTestApp(t)
	defer cleanup()
	_, funcaoID, _, _, titPapelID := fnSetup(t, app, st)
	ckTit := loginAs(t, app, "fntit", "senha-gerente")

	// Mensagem carimbada da função (destinatário = o próprio papel do titular).
	rrM, resM := doJSONReq(app, "POST", "/api/mensagens", map[string]any{
		"assunto": "OS da função", "corpo": "<p>x</p>",
		"tipo": "comum", "destinatario_papel_ids": []int64{titPapelID},
	}, ckTit)
	if rrM.Code != http.StatusOK {
		t.Fatalf("sanity msg função: %d (%v)", rrM.Code, resM)
	}
	msgFnID := int64(resM["id"].(float64))
	// Braço pessoal: linha em mensagem_destinatarios para o papel do titular.
	// (o carimbo NÃO cria destinatário; aqui os dois existem — trava vale igual)

	// Mensagem pessoal legada.
	rrP, resP := doJSONReq(app, "POST", "/api/mensagens", map[string]any{
		"assunto": "Pessoal", "corpo": "<p>y</p>",
		"tipo": "comum", "destinatario_papel_ids": []int64{titPapelID},
	}, ckTit)
	if rrP.Code != http.StatusOK {
		t.Fatalf("sanity msg pessoal: %d (%v)", rrP.Code, resP)
	}
	msgPessoalID := int64(resP["id"].(float64))
	if _, err := st.db.Exec(`UPDATE mensagens SET funcao_id=NULL WHERE id=?`, msgPessoalID); err != nil {
		t.Fatalf("zerar carimbo pessoal: %v", err)
	}
	var nFn *int64
	_ = st.db.QueryRow(`SELECT funcao_id FROM mensagens WHERE id=?`, msgFnID).Scan(&nFn)
	if nFn == nil || *nFn != funcaoID {
		t.Fatalf("sanity: msg da função sem carimbo (v=%v)", nFn)
	}

	// FV5a: arquivar a da função → 400 com mensagem clara.
	rrA, resA := doJSONReq(app, "POST", "/api/mensagens/"+intToString(int(msgFnID))+"/arquivar", map[string]any{}, ckTit)
	if rrA.Code != http.StatusBadRequest {
		t.Fatalf("FV5a: arquivar item da função deve 400, veio %d (%v)", rrA.Code, resA)
	}
	if !strings.Contains(rrA.Body.String(), "Caixa da Função") {
		t.Fatalf("FV5a: mensagem de bloqueio sem 'Caixa da Função': %s", rrA.Body.String())
	}

	// FV5b: arquivar a pessoal → 200 (regressão da trava de despacho).
	rrB, resB := doJSONReq(app, "POST", "/api/mensagens/"+intToString(int(msgPessoalID))+"/arquivar", map[string]any{}, ckTit)
	if rrB.Code != http.StatusOK {
		t.Fatalf("FV5b: arquivar pessoal deve 200, veio %d (%v)", rrB.Code, resB)
	}
	var arq int
	if err := st.db.QueryRow(`SELECT arquivada FROM mensagem_destinatarios WHERE mensagem_id=? AND destinatario_papel_id=?`, msgPessoalID, titPapelID).Scan(&arq); err != nil || arq != 1 {
		t.Fatalf("FV5b: arquivamento não persistiu (arq=%d err=%v)", arq, err)
	}
}
