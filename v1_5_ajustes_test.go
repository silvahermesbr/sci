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

func TestEscalaEdicaoPostoEDescansoNullSafe(t *testing.T) {
	app, st, cleanup := setupTestApp(t)
	defer cleanup()

	admin := loginAs(t, app, "admin", "admin123")

	_, resG := doJSONReq(app, "POST", "/api/grupos", map[string]any{
		"nome": "Companhia Alpha", "login": "cap_alpha", "senha": "alpha_password", "nome_guerra": "Cap Alpha",
	}, admin)
	gid := int64(resG["id"].(float64))
	ger := loginAs(t, app, "cap_alpha", "alpha_password")

	resTipo, _ := st.db.Exec(`INSERT INTO escala_tipos (nome, grupo_id) VALUES ('Oficial de Dia', ?)`, gid)
	tipoID, _ := resTipo.LastInsertId()

	resPes, _ := st.db.Exec(`INSERT INTO pessoas (nome_guerra, nome_completo, grupo_id, status) VALUES ('Moura', 'Lucas Moura', ?, 'ativo')`, gid)
	pesID, _ := resPes.LastInsertId()

	// 1. Inserir turno com data_fim vazia para testar robustez de descanso
	resT1, errT1 := st.db.Exec(`INSERT INTO escala_turnos (grupo_id, tipo_id, data_inicio, data_fim) VALUES (?, ?, '2026-10-20T07:00:00', '')`, gid, tipoID)
	if errT1 != nil {
		t.Fatalf("erro ao inserir turno 1: %v", errT1)
	}
	t1ID, _ := resT1.LastInsertId()
	_, _ = st.db.Exec(`INSERT INTO escala_pessoas (turno_id, pessoa_id) VALUES (?, ?)`, t1ID, pesID)

	desc := app.validarDescansoEscala(pesID, 99999, "2026-10-20T10:00:00", "2026-10-20T18:00:00")
	// Turno 1 vai das 07:00 até 07:00 (vazia). O novo é 10:00. Deve calcular folga de 3 horas (< 24h = critico)
	if desc.Nivel != "critico" {
		t.Fatalf("esperava nível critico para intervalo de 3h, obteve %s (msg: %s)", desc.Nivel, desc.Mensagem)
	}

	// 2. Testar escaladosNaData com formato ISO completo (YYYY-MM-DDTHH:MM:SS)
	escalados := app.escaladosNaData(gid, "2026-10-20")
	if len(escalados) == 0 {
		t.Fatalf("esperava encontrar militar escalado em 2026-10-20 via escaladosNaData")
	}

	// 3. Testar criação e edição de posto avulso com preservação de pessoas alocadas
	recCria, resCria := doJSONReq(app, "POST", "/api/escalas/turnos", map[string]any{
		"tipo_id":     tipoID,
		"data_inicio": "2026-10-25T07:00:00",
		"data_fim":    "2026-10-26T07:00:00",
		"observacao":  "Posto Inicial",
		"pessoas": []map[string]any{
			{"pessoa_id": pesID, "funcao_escala": "Comandante da Guarda"},
		},
	}, ger)
	if recCria.Code != http.StatusOK {
		t.Fatalf("falha ao criar turno: %d", recCria.Code)
	}
	turnoAvulsoID := int64(resCria["id"].(float64))

	// Editar o posto (horários e observação) sem passar o campo "pessoas"
	recEdit, resEdit := doJSONReq(app, "POST", "/api/escalas/turnos", map[string]any{
		"id":          turnoAvulsoID,
		"tipo_id":     tipoID,
		"data_inicio": "2026-10-25T08:00:00",
		"data_fim":    "2026-10-26T08:00:00",
		"observacao":  "Horário alterado para 08h",
	}, ger)
	if recEdit.Code != http.StatusOK {
		t.Fatalf("falha ao atualizar turno: %d (%v)", recEdit.Code, resEdit)
	}

	// Verificar se a pessoa continua alocada ao turno
	var countPes int
	errQ := st.db.QueryRow(`SELECT COUNT(*) FROM escala_pessoas WHERE turno_id = ? AND pessoa_id = ?`, turnoAvulsoID, pesID).Scan(&countPes)
	if errQ != nil || countPes != 1 {
		t.Fatalf("militar alocado foi indevidamente removido ao atualizar propriedades do turno: count=%d, err=%v", countPes, errQ)
	}
}
