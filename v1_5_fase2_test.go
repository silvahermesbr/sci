package main

import (
	"net/http"
	"testing"
	"time"
)

func TestV15_Fase2_CalendarioDuplicidadeEConferenciaRegras(t *testing.T) {
	app, _, cleanup := setupTestApp(t)
	defer cleanup()

	admin := loginAs(t, app, "admin", "admin123")

	// 1. Criar grupo e gerente
	_, resG := doJSONReq(app, "POST", "/api/grupos", map[string]any{
		"nome": "Pelotao Alfa", "login": "chefe_alfa", "senha": "senha12345", "nome_guerra": "Cap Alfa",
	}, admin)
	if resG["id"] == nil {
		t.Fatalf("falha ao criar grupo: %v", resG)
	}
	gid := int64(resG["id"].(float64))

	ger := loginAs(t, app, "chefe_alfa", "senha12345")

	// Criar operador
	resOp, errOp := app.st.db.Exec(`INSERT INTO usuarios (login, senha_hash, papel, grupo_id, ativo) VALUES ('op_alfa', 'hash', 'operador', ?, 1)`, gid)
	if errOp != nil {
		t.Fatalf("falha ao criar operador: %v", errOp)
	}
	opUID, _ := resOp.LastInsertId()

	// Criar militar
	var pesID int64
	resP, errIns := app.st.db.Exec(`INSERT INTO pessoas (nome_guerra, nome_completo, grupo_id, status) VALUES ('Sd Souza', 'Souza Costa', ?, 'ativo')`, gid)
	if errIns != nil {
		t.Fatalf("inserir pessoa: %v", errIns)
	}
	pesID, _ = resP.LastInsertId()

	// 2. Criar Calendário e testar compartilhamento sem duplicidade
	_, resCal := doJSONReq(app, "POST", "/api/calendarios", map[string]any{
		"nome": "Agenda Alfa", "cor": "#10b981",
	}, ger)
	if resCal["id"] == nil {
		t.Fatalf("falha ao criar calendário: %v", resCal)
	}
	calID := int64(resCal["id"].(float64))

	// 1º compartilhamento: pode_editar = false
	rec1, resC1 := doJSONReq(app, "POST", "/api/calendarios/"+int64ToStr(calID)+"/compartilhar", map[string]any{
		"alvo_tipo":   "usuario",
		"alvo_id":     opUID,
		"pode_editar": false,
	}, ger)
	if rec1.Code != http.StatusOK {
		t.Fatalf("falha no 1º compartilhamento: %d - %v", rec1.Code, resC1)
	}

	// 2º compartilhamento com mesmo alvo: pode_editar = true (deve atualizar, não duplicar)
	rec2, resC2 := doJSONReq(app, "POST", "/api/calendarios/"+int64ToStr(calID)+"/compartilhar", map[string]any{
		"alvo_tipo":   "usuario",
		"alvo_id":     opUID,
		"pode_editar": true,
	}, ger)
	if rec2.Code != http.StatusOK {
		t.Fatalf("falha no 2º compartilhamento: %d - %v", rec2.Code, resC2)
	}

	var countComp, podeEd int
	err := app.st.db.QueryRow(`SELECT count(*), pode_editar FROM calendario_compartilhamentos WHERE calendario_id = ? AND alvo_usuario_id = ?`, calID, opUID).Scan(&countComp, &podeEd)
	if err != nil || countComp != 1 || podeEd != 1 {
		t.Fatalf("esperava 1 compartilhamento com pode_editar=1, obteve count=%d, pode_editar=%d (err: %v)", countComp, podeEd, err)
	}

	// 3. Testar Regras de Conferência v1.5
	// Catálogo de destino
	_, resD := doJSONReq(app, "POST", "/api/catalogo/destinos", map[string]any{"nome": "Hospital Geral"}, ger)
	destID := int64(resD["id"].(float64))

	// Abrir conferência
	_, resConf := doJSONReq(app, "POST", "/api/conferencia/iniciar", map[string]any{"local": "Pátio"}, ger)
	confID := int64(resConf["id"].(float64))

	// 3a. Marcar falta justificada com destino e observação
	obsJust := "Consulta médica"
	recJust, _ := doJSONReq(app, "POST", "/api/conferencia/marcar?id="+int64ToStr(confID), map[string]any{
		"pessoa_id":  pesID,
		"situacao":   "justificada",
		"destino_id": destID,
		"observacao": obsJust,
	}, ger)
	if recJust.Code != http.StatusOK {
		t.Fatalf("falha ao marcar justificada: %d", recJust.Code)
	}

	var sitBanco, obsBanco string
	var destBanco *int64
	_ = app.st.db.QueryRow(`SELECT situacao, observacao, destino_id FROM presencas WHERE conferencia_id = ? AND pessoa_id = ?`, confID, pesID).
		Scan(&sitBanco, &obsBanco, &destBanco)
	if sitBanco != "justificada" || obsBanco != obsJust || destBanco == nil || *destBanco != destID {
		t.Fatalf("esperava justificada salva, obteve sit=%s, obs=%s, dest=%v", sitBanco, obsBanco, destBanco)
	}

	// 3b. Mudar para presente (sem informar nova observação e tentando enviar destino)
	// A observação deve ser resetada para vazio e o destino forçado a null (v1.5)
	recPres, _ := doJSONReq(app, "POST", "/api/conferencia/marcar?id="+int64ToStr(confID), map[string]any{
		"pessoa_id":  pesID,
		"situacao":   "presente",
		"destino_id": destID,
	}, ger)
	if recPres.Code != http.StatusOK {
		t.Fatalf("falha ao marcar presente: %d", recPres.Code)
	}

	_ = app.st.db.QueryRow(`SELECT situacao, observacao, destino_id FROM presencas WHERE conferencia_id = ? AND pessoa_id = ?`, confID, pesID).
		Scan(&sitBanco, &obsBanco, &destBanco)
	if sitBanco != "presente" || obsBanco != "" || destBanco != nil {
		t.Fatalf("esperava presente com obs='' e destino=nil, obteve sit=%s, obs='%s', dest=%v", sitBanco, obsBanco, destBanco)
	}

	// 4. Testar sigla do setor no PDF da conferência
	_, resSet := doJSONReq(app, "POST", "/api/catalogo/setores", map[string]any{
		"nome": "Primeiro Pelotão Alfa", "sigla": "1º PEL",
	}, ger)
	setorID := int64(resSet["id"].(float64))
	_, _ = app.st.db.Exec(`UPDATE pessoas SET setor_id = ? WHERE id = ?`, setorID, pesID)

	lancamentos, _, errLanc := app.montarLancamentosPDFConferencia(confID, "todos")
	if errLanc != nil || len(lancamentos) == 0 {
		t.Fatalf("nenhum lançamento retornado no PDF: %v", errLanc)
	}
	if lancamentos[0]["setor"].(string) != "1º PEL" {
		t.Fatalf("esperava sigla '1º PEL', obteve '%s'", lancamentos[0]["setor"])
	}

	// 5. Testar geração de PDF com assinatura física
	hoje := time.Now().Format("2006-01-02")
	pdfBytes, err := app.gerarConferenciaPDF(ConferenciaPDF{
		ID:             confID,
		Data:           hoje,
		Status:         "fechada",
		CriadoPor:      "chefe_alfa",
		GeradoPor:      "chefe_alfa",
		FechadoPorNome: "Capitão Silva Alfa",
		Lancamentos:    lancamentos,
	})
	if err != nil || len(pdfBytes) == 0 {
		t.Fatalf("falha ao gerar PDF de conferência: %v", err)
	}
}
