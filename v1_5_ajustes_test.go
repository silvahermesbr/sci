package main

import (
	"fmt"
	"net/http"
	"strconv"
	"testing"
)

func TestAjustesEscalas(t *testing.T) {
	app, st, cleanup := setupTestApp(t)
	defer cleanup()

	admin := loginAs(t, app, "admin", "admin123")

	// 1. Criar Grupo Superior e Grupo Subordinado
	_, resGSup := doJSONReq(app, "POST", "/api/grupos", map[string]any{
		"nome": "Batalhão Pai", "login": "chefe_pai", "senha": "senha12345", "nome_guerra": "Chefe Pai",
	}, admin)
	paiID := int64(resGSup["id"].(float64))

	_, resGSub := doJSONReq(app, "POST", "/api/grupos", map[string]any{
		"nome": "1ª Cia Filha", "login": "op_sub", "senha": "senha12345", "nome_guerra": "Op Sub",
	}, admin)
	filhoID := int64(resGSub["id"].(float64))

	// Vínculo bilateral superior -> subordinado
	_, _ = st.db.Exec(`
		INSERT INTO grupo_vinculos (superior_id, subordinado_id, criado_por_superior, criado_por_subordinado)
		VALUES (?, ?, 1, 1)`, paiID, filhoID)

	gerPai := loginAs(t, app, "chefe_pai", "senha12345")
	gerSub := loginAs(t, app, "op_sub", "senha12345")

	// Criar postos/graduações (funcoes) ordenadas por antiguidade
	resSd, _ := st.db.Exec(`INSERT INTO funcoes (nome, grupo_id, antiguidade) VALUES ('Soldado', ?, 10)`, paiID)
	sdID, _ := resSd.LastInsertId()
	resCb, _ := st.db.Exec(`INSERT INTO funcoes (nome, grupo_id, antiguidade) VALUES ('Cabo', ?, 20)`, paiID)
	cbID, _ := resCb.LastInsertId()
	resTen, _ := st.db.Exec(`INSERT INTO funcoes (nome, grupo_id, antiguidade) VALUES ('1º Tenente', ?, 40)`, paiID)
	tenID, _ := resTen.LastInsertId()

	// Criar pessoas no Batalhão Pai
	resP1, _ := st.db.Exec(`INSERT INTO pessoas (nome_guerra, nome_completo, grupo_id, funcao_id, status) VALUES ('Silva', 'José Silva', ?, ?, 'ativo')`, paiID, sdID)
	p1SilvaID, _ := resP1.LastInsertId()
	resP2, _ := st.db.Exec(`INSERT INTO pessoas (nome_guerra, nome_completo, grupo_id, funcao_id, status) VALUES ('Souza', 'Carlos Souza', ?, ?, 'ativo')`, paiID, cbID)
	p2SouzaID, _ := resP2.LastInsertId()
	resP3, _ := st.db.Exec(`INSERT INTO pessoas (nome_guerra, nome_completo, grupo_id, funcao_id, status) VALUES ('Costa', 'Marcos Costa', ?, ?, 'ativo')`, paiID, tenID)
	p3CostaID, _ := resP3.LastInsertId()

	// Criar tipo de escala
	resTipo, _ := st.db.Exec(`INSERT INTO escala_tipos (nome, grupo_id) VALUES ('Guarda ao Quartel', ?)`, paiID)
	tipoID, _ := resTipo.LastInsertId()

	// 2. Criar Modelo de Escala com postos e aptos (apenas Silva e Souza são aptos)
	// Posto 1 exige entre Soldado e Cabo (inclusivo)
	bodyMod := map[string]any{
		"nome": "Guarda 24h",
		"postos": []map[string]any{
			{
				"tipo_id":           tipoID,
				"hora_inicio":       "07:00",
				"hora_fim":          "07:00",
				"quantidade":        1,
				"posto_grad_min_id": sdID,
				"posto_grad_max_id": cbID,
			},
		},
		"aptos_ids": []int64{p1SilvaID, p2SouzaID},
	}
	recMod, resMod := doJSONReq(app, "POST", "/api/escalas/modelos", bodyMod, gerPai)
	if recMod.Code != http.StatusOK {
		t.Fatalf("criar modelo falhou: %d %v", recMod.Code, resMod)
	}
	modeloID := int64(resMod["id"].(float64))

	// Aplicar modelo no dia 2026-10-10
	recApl, resApl := doJSONReq(app, "POST", "/api/escalas/aplicar-modelo", map[string]any{
		"modelo_id": modeloID,
		"data":      "2026-10-10",
	}, gerPai)
	if recApl.Code != http.StatusOK {
		t.Fatalf("aplicar modelo falhou: %d %v", recApl.Code, resApl)
	}

	// Buscar turno criado
	var turnoID int64
	err := st.db.QueryRow(`SELECT id FROM escala_turnos WHERE modelo_id = ? AND data_inicio LIKE '2026-10-10%'`, modeloID).Scan(&turnoID)
	if err != nil {
		t.Fatalf("buscar turno criado: %v", err)
	}

	// 1. TESTE: Subordinado enxerga escala do grupo superior (Requisito 3)
	recSubList, resSubList := doJSONReq(app, "GET", "/api/escalas/turnos?data=2026-10-10", nil, gerSub)
	if recSubList.Code != http.StatusOK {
		t.Fatalf("subordinado listar turnos falhou: %d", recSubList.Code)
	}
	turnosArr := resSubList["turnos"].([]any)
	encontrou := false
	for _, it := range turnosArr {
		tr := it.(map[string]any)
		if int64(tr["id"].(float64)) == turnoID {
			encontrou = true
			break
		}
	}
	if !encontrou {
		t.Fatalf("subordinado deveria ver escala do grupo superior")
	}

	// 2. TESTE: Alocar Tenente Costa (NÃO APTO no modelo) -> deve ser REJEITADO com 400 (Requisito 2)
	recAlocCosta, _ := doJSONReq(app, "POST", fmt.Sprintf("/api/escalas/turnos/%d/alocar", turnoID), map[string]any{
		"pessoa_id": p3CostaID,
	}, gerPai)
	if recAlocCosta.Code != http.StatusBadRequest {
		t.Fatalf("esperava erro 400 para militar não apto, obteve %d", recAlocCosta.Code)
	}

	// 3. TESTE: Alocar Soldado Silva (APTO e faixa válida) -> SUCESSO (Requisito 2 e 8)
	recAlocSilva, _ := doJSONReq(app, "POST", fmt.Sprintf("/api/escalas/turnos/%d/alocar", turnoID), map[string]any{
		"pessoa_id": p1SilvaID,
	}, gerPai)
	if recAlocSilva.Code != http.StatusOK {
		t.Fatalf("alocar Silva falhou: %d", recAlocSilva.Code)
	}

	// 4. TESTE: Criar segundo turno no MESMO HORÁRIO e tentar alocar Soldado Silva -> BLOQUEIO POR SOBREPOSIÇÃO (Requisito 10)
	resT2, _ := st.db.Exec(`INSERT INTO escala_turnos (grupo_id, tipo_id, data_inicio, data_fim, fase, status_delegacao)
		VALUES (?, ?, '2026-10-10T07:00:00', '2026-10-11T07:00:00', 'aberto', 'proprio')`, paiID, tipoID)
	t2ID, _ := resT2.LastInsertId()

	recAlocT2, _ := doJSONReq(app, "POST", fmt.Sprintf("/api/escalas/turnos/%d/alocar", t2ID), map[string]any{
		"pessoa_id": p1SilvaID,
	}, gerPai)
	if recAlocT2.Code != http.StatusBadRequest {
		t.Fatalf("esperava bloqueio de sobreposição simultânea com 400, obteve %d", recAlocT2.Code)
	}

	// 5. TESTE: Alocar Soldado Silva em turno com folga de 12 horas (< 24h) -> Retorna Alerta Crítico (Vermelho)
	resT3, _ := st.db.Exec(`INSERT INTO escala_turnos (grupo_id, tipo_id, data_inicio, data_fim, fase, status_delegacao)
		VALUES (?, ?, '2026-10-11T19:00:00', '2026-10-12T07:00:00', 'aberto', 'proprio')`, paiID, tipoID)
	t3ID, _ := resT3.LastInsertId()

	recAlocT3, resAlocT3 := doJSONReq(app, "POST", fmt.Sprintf("/api/escalas/turnos/%d/alocar", t3ID), map[string]any{
		"pessoa_id": p1SilvaID,
	}, gerPai)
	if recAlocT3.Code != http.StatusOK {
		t.Fatalf("alocar t3 com descanso < 24h falhou: %d", recAlocT3.Code)
	}
	alertaDescanso := resAlocT3["alerta_descanso"].(map[string]any)
	if alertaDescanso["nivel"].(string) != "critico" {
		t.Fatalf("esperava nivel critico (<24h), obteve: %v", alertaDescanso["nivel"])
	}

	// 6. TESTE: Endpoint GET /api/escalas/turnos/{id}/candidatos
	recCand, resCand := doJSONReq(app, "GET", fmt.Sprintf("/api/escalas/turnos/%d/candidatos", turnoID), nil, gerPai)
	if recCand.Code != http.StatusOK {
		t.Fatalf("endpoint candidatos falhou: %d", recCand.Code)
	}
	candidatos := resCand["candidatos"].([]any)
	if len(candidatos) == 0 {
		t.Fatalf("esperava candidatos retornados")
	}
	for _, it := range candidatos {
		c := it.(map[string]any)
		cid := int64(c["id"].(float64))
		apto := c["apto"].(bool)
		if cid == p1SilvaID && !apto {
			t.Fatalf("Silva deveria estar apto")
		}
		if cid == p3CostaID && apto {
			t.Fatalf("Costa não deveria estar apto")
		}
	}

	_ = strconv.Itoa
}
