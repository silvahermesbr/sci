package main

import (
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestV15_Fase4_Escalas20_ModelosAplicacaoDelegacaoERelatorio(t *testing.T) {
	app, _, cleanup := setupTestApp(t)
	defer cleanup()

	admin := loginAs(t, app, "admin", "admin123")

	// 1. Criar Grupo Superior e Grupo Subordinado
	_, resGSup := doJSONReq(app, "POST", "/api/grupos", map[string]any{
		"nome": "Companhia Alpha", "login": "ger_alpha", "senha": "senha12345", "nome_guerra": "Cap Alpha",
	}, admin)
	gSupID := int64(resGSup["id"].(float64))

	_, resGSub := doJSONReq(app, "POST", "/api/grupos", map[string]any{
		"nome": "1º Pelotão Alpha", "login": "ger_pel1", "senha": "senha12345", "nome_guerra": "Ten Pel1",
	}, admin)
	gSubID := int64(resGSub["id"].(float64))

	// Criar vínculo bilateral ativo (superior -> subordinado)
	_, _ = app.st.db.Exec(`
		INSERT INTO grupo_vinculos (superior_id, subordinado_id, criado_por_superior, criado_por_subordinado)
		VALUES (?, ?, 1, 1)`, gSupID, gSubID)

	gerSup := loginAs(t, app, "ger_alpha", "senha12345")
	gerSub := loginAs(t, app, "ger_pel1", "senha12345")

	// Criar militar no Grupo Superior
	resPSup, _ := app.st.db.Exec(`INSERT INTO pessoas (nome_guerra, nome_completo, grupo_id, status) VALUES ('Sgt Borges', 'Borges Lima', ?, 'ativo')`, gSupID)
	pSupID, _ := resPSup.LastInsertId()

	// Criar militar no Grupo Subordinado
	resPSub, _ := app.st.db.Exec(`INSERT INTO pessoas (nome_guerra, nome_completo, grupo_id, status) VALUES ('Sd Ferreira', 'Ferreira Santos', ?, 'ativo')`, gSubID)
	pSubID, _ := resPSub.LastInsertId()

	// Criar Tipo de Serviço
	resTipo, _ := app.st.db.Exec(`INSERT INTO escala_tipos (nome, grupo_id, ativo) VALUES ('Oficial de Dia', ?, 1)`, gSupID)
	tipoOfDia, _ := resTipo.LastInsertId()

	resTipo2, _ := app.st.db.Exec(`INSERT INTO escala_tipos (nome, grupo_id, ativo) VALUES ('Sentinela da Guarda', ?, 1)`, gSupID)
	tipoSentinela, _ := resTipo2.LastInsertId()

	// 2. Criar Modelo de Escala (POST /api/escalas/modelos)
	modeloPayload := map[string]any{
		"nome":      "Serviço de Guarda e Plantão",
		"descricao": "Escala padrão de 24h",
		"postos": []map[string]any{
			{"tipo_id": tipoOfDia, "hora_inicio": "07:00", "hora_fim": "07:00", "quantidade": 1, "ordem": 0},
			{"tipo_id": tipoSentinela, "hora_inicio": "07:00", "hora_fim": "07:00", "quantidade": 2, "ordem": 1},
		},
		"aptos_ids": []int64{pSupID},
	}
	recMod, resMod := doJSONReq(app, "POST", "/api/escalas/modelos", modeloPayload, gerSup)
	if recMod.Code != http.StatusOK {
		t.Fatalf("falha ao criar modelo de escala: %d - %v", recMod.Code, resMod)
	}
	modeloID := int64(resMod["id"].(float64))

	// Obter modelo (GET /api/escalas/modelos/{id})
	recGetMod, resGetMod := doJSONReq(app, "GET", "/api/escalas/modelos/"+int64ToStr(modeloID), nil, gerSup)
	if recGetMod.Code != http.StatusOK {
		t.Fatalf("falha ao obter modelo: %d", recGetMod.Code)
	}
	postosList := resGetMod["postos"].([]any)
	if len(postosList) != 2 {
		t.Fatalf("esperava 2 postos no modelo, obteve %d", len(postosList))
	}

	// 3. Aplicar Modelo no Dia (POST /api/escalas/aplicar-modelo)
	dataTeste := time.Now().AddDate(0, 0, 3).Format("2006-01-02")
	recAp, resAp := doJSONReq(app, "POST", "/api/escalas/aplicar-modelo", map[string]any{
		"modelo_id": modeloID,
		"data":      dataTeste,
	}, gerSup)
	if recAp.Code != http.StatusOK {
		t.Fatalf("falha ao aplicar modelo: %d - %v", recAp.Code, resAp)
	}
	turnosCriados := int(resAp["turnos_criados"].(float64))
	if turnosCriados != 3 { // 1 Oficial de Dia + 2 Sentinelas
		t.Fatalf("esperava 3 turnos criados (1+2), obteve %d", turnosCriados)
	}

	// Consultar turnos gerados (GET /api/escalas/turnos?data=...)
	_, resTurnos := doJSONReq(app, "GET", "/api/escalas/turnos?data="+dataTeste, nil, gerSup)
	turnos := resTurnos["turnos"].([]any)
	if len(turnos) != 3 {
		t.Fatalf("esperava 3 turnos no banco, obteve %d", len(turnos))
	}
	primeiroTurno := turnos[0].(map[string]any)
	turno1ID := int64(primeiroTurno["id"].(float64))
	if primeiroTurno["fase"].(string) != "aberto" {
		t.Fatalf("esperava fase inicial 'aberto', obteve '%s'", primeiroTurno["fase"])
	}

	// 4. Alocar Militar no 1º Turno (POST /api/escalas/turnos/{id}/alocar)
	recAloc, _ := doJSONReq(app, "POST", "/api/escalas/turnos/"+int64ToStr(turno1ID)+"/alocar", map[string]any{
		"pessoa_id":     pSupID,
		"funcao_escala": "Oficial de Dia",
	}, gerSup)
	if recAloc.Code != http.StatusOK {
		t.Fatalf("falha ao alocar militar: %d", recAloc.Code)
	}

	// 5. Delegar 2º Turno para o Grupo Subordinado (POST /api/escalas/turnos/{id}/delegar)
	segundoTurno := turnos[1].(map[string]any)
	turno2ID := int64(segundoTurno["id"].(float64))
	recDeleg, _ := doJSONReq(app, "POST", "/api/escalas/turnos/"+int64ToStr(turno2ID)+"/delegar", map[string]any{
		"grupo_delegado_id": gSubID,
	}, gerSup)
	if recDeleg.Code != http.StatusOK {
		t.Fatalf("falha ao delegar turno: %d", recDeleg.Code)
	}

	// Grupo Subordinado consulta e enxerga o turno delegado
	_, resTurnosSub := doJSONReq(app, "GET", "/api/escalas/turnos?data="+dataTeste, nil, gerSub)
	turnosSub := resTurnosSub["turnos"].([]any)
	if len(turnosSub) == 0 {
		t.Fatalf("grupo subordinado deveria enxergar o turno delegado para ele")
	}

	// Grupo Subordinado preenche o posto delegado com o militar dele
	recSubAloc, _ := doJSONReq(app, "POST", "/api/escalas/turnos/"+int64ToStr(turno2ID)+"/alocar", map[string]any{
		"pessoa_id":     pSubID,
		"funcao_escala": "Sentinela 1 (Pelotão 1)",
	}, gerSub)
	if recSubAloc.Code != http.StatusOK {
		t.Fatalf("subordinado falhou ao preencher posto delegado: %d", recSubAloc.Code)
	}

	// Grupo Superior agora verifica que o turno delegado foi preenchido com o militar do subordinado
	_, resTurnosSupRecheck := doJSONReq(app, "GET", "/api/escalas/turnos?data="+dataTeste, nil, gerSup)
	turnosSupRecheck := resTurnosSupRecheck["turnos"].([]any)
	var t2Found map[string]any
	for _, tItem := range turnosSupRecheck {
		tm := tItem.(map[string]any)
		if int64(tm["id"].(float64)) == turno2ID {
			t2Found = tm
			break
		}
	}
	if t2Found == nil || len(t2Found["pessoas"].([]any)) == 0 {
		t.Fatalf("dono da escala deveria ver militar alocado pelo subordinado em tempo real")
	}

	// 6. Alterar Fase da Escala (PATCH /api/escalas/fase)
	recFase, resFase := doJSONReq(app, "PATCH", "/api/escalas/fase", map[string]any{
		"data": dataTeste,
		"fase": "publicado",
	}, gerSup)
	if recFase.Code != http.StatusOK {
		t.Fatalf("falha ao alterar fase: %d - %v", recFase.Code, resFase)
	}
	if resFase["fase"].(string) != "publicado" {
		t.Fatalf("esperava fase publicado, obteve '%s'", resFase["fase"])
	}

	// 7. Relatório Diário de Escala em PDF (GET /api/escalas/relatorio-dia.pdf?data=...)
	recPDF, _ := doJSONReq(app, "GET", "/api/escalas/relatorio-dia.pdf?data="+dataTeste, nil, gerSup)
	if recPDF.Code != http.StatusOK {
		t.Fatalf("falha ao gerar relatório diário de escala: %d", recPDF.Code)
	}
	if recPDF.Header().Get("Content-Type") != "application/pdf" {
		t.Fatalf("esperava Content-Type application/pdf, obteve %s", recPDF.Header().Get("Content-Type"))
	}
	if len(recPDF.Body.Bytes()) < 1000 || !strings.HasPrefix(string(recPDF.Body.Bytes()[:8]), "%PDF-") {
		t.Fatalf("PDF gerado inválido")
	}

	// 8. Testar Limpar Dia (POST /api/escalas/limpar-dia)
	recLimp, resLimp := doJSONReq(app, "POST", "/api/escalas/limpar-dia", map[string]any{
		"data": dataTeste,
	}, gerSup)
	if recLimp.Code != http.StatusOK {
		t.Fatalf("falha ao limpar dia: %d", recLimp.Code)
	}
	removidos := int(resLimp["removidos"].(float64))
	if removidos != 3 {
		t.Fatalf("esperava 3 turnos removidos ao limpar o dia, obteve %d", removidos)
	}

	// Confirmar que não há mais turnos no dia
	_, resVazio := doJSONReq(app, "GET", "/api/escalas/turnos?data="+dataTeste, nil, gerSup)
	if len(resVazio["turnos"].([]any)) != 0 {
		t.Fatalf("dia deveria estar completamente limpo")
	}
}
