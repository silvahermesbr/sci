package main

import (
	"fmt"
	"net/http"
	"testing"
)

// TestFase5WorkflowSugestoesSetor: testa criação de sugestão por auxiliar e aprovação por chefe de setor
func TestFase5WorkflowSugestoesSetor(t *testing.T) {
	app, st, cleanup := setupTestApp(t)
	defer cleanup()

	admin := loginAs(t, app, "admin", "admin123")

	// 1. Criar grupo e militar
	_, resG := doJSONReq(app, "POST", "/api/grupos", map[string]any{
		"nome": "Pelotão de Operações", "login": "chefe_pessoal", "senha": "senha12345", "nome_guerra": "Cap Chefe",
	}, admin)
	gid := int64(resG["id"].(float64))

	resP, _ := st.db.Exec(`INSERT INTO pessoas (nome_guerra, nome_completo, grupo_id, status) VALUES ('SD FULANO', 'Fulano da Silva', ?, 'ativo')`, gid)
	pid, _ := resP.LastInsertId()

	// Criar Auxiliar de Pessoal
	hash, _ := hashSenha("senha12345")
	_, _ = st.db.Exec(`INSERT INTO usuarios (login, senha_hash, papel, grupo_id, precisa_setup, nome_guerra) VALUES ('aux_pessoal', ?, 'operador', ?, 0, 'SGT AUXILIAR')`, hash, gid)

	aux := loginAs(t, app, "aux_pessoal", "senha12345")
	chefe := loginAs(t, app, "chefe_pessoal", "senha12345")

	// 2. Auxiliar envia sugestão de alteração de militar
	sugReq := map[string]any{
		"setor_tipo": "pessoal",
		"tipo_acao":  "alterar_status_militar",
		"dados": map[string]any{
			"pessoa_id":   pid,
			"novo_status": "inativo",
			"motivo":      "Transferência interna",
		},
	}
	recCriar, respCriar := doJSONReq(app, "POST", "/api/setores/sugestoes", sugReq, aux)
	if recCriar.Code != http.StatusOK {
		t.Fatalf("esperava 200 ao criar sugestão, obteve %d", recCriar.Code)
	}
	sugID := int64(respCriar["id"].(float64))

	// 3. Listar sugestões pendentes
	recList, respList := doJSONReq(app, "GET", "/api/setores/sugestoes?status=pendente", nil, chefe)
	if recList.Code != http.StatusOK {
		t.Fatalf("esperava 200 ao listar sugestões, obteve %d", recList.Code)
	}
	sugestoes := respList["sugestoes"].([]any)
	if len(sugestoes) == 0 {
		t.Fatalf("esperava ao menos 1 sugestão pendente")
	}

	// 4. Chefe de Setor aprova sugestão -> ação executada e nome do chefe registrado
	avalReq := map[string]any{
		"acao":          "aprovar",
		"justificativa": "Conforme plano de movimentação homologado",
	}
	recAval, _ := doJSONReq(app, "POST", fmt.Sprintf("/api/setores/sugestoes/%d/avaliar", sugID), avalReq, chefe)
	if recAval.Code != http.StatusOK {
		t.Fatalf("esperava 200 ao avaliar sugestão, obteve %d", recAval.Code)
	}

	// Verificar se militar foi alterado para inativo
	var stMilitar string
	_ = st.db.QueryRow(`SELECT status FROM pessoas WHERE id = ?`, pid).Scan(&stMilitar)
	if stMilitar != "inativo" {
		t.Fatalf("esperava militar com status inativo após aprovação, obteve %s", stMilitar)
	}

	// Verificar se registro oficial de aprovação está preenchido
	var aprPor int64
	_ = st.db.QueryRow(`SELECT aprovado_por FROM setor_sugestoes WHERE id = ?`, sugID).Scan(&aprPor)
	if aprPor <= 0 {
		t.Fatalf("esperava aprovado_por > 0 (Chefe), obteve %d", aprPor)
	}
}

// TestFase5ConscienciaSituacionalResumo: testa agregação consolidada de subunidades
func TestFase5ConscienciaSituacionalResumo(t *testing.T) {
	app, st, cleanup := setupTestApp(t)
	defer cleanup()

	admin := loginAs(t, app, "admin", "admin123")

	// 1. Criar grupo superior e subordinado
	_, resGSup := doJSONReq(app, "POST", "/api/grupos", map[string]any{
		"nome": "Comando da Unidade", "login": "comandante", "senha": "senha12345", "nome_guerra": "TC COMANDANTE",
	}, admin)
	gidSup := int64(resGSup["id"].(float64))

	_, resGSub := doJSONReq(app, "POST", "/api/grupos", map[string]any{
		"nome": "1ª Companhia Operacional", "login": "ger_cia1", "senha": "senha12345", "nome_guerra": "Cap Cia",
	}, admin)
	gidSub := int64(resGSub["id"].(float64))

	// Vínculo bilateral ativo
	_, _ = st.db.Exec(`INSERT INTO grupo_vinculos (superior_id, subordinado_id, criado_por_superior, criado_por_subordinado) VALUES (?, ?, 1, 1)`, gidSup, gidSub)

	// Efetivo no subordinado
	if _, err := st.db.Exec(`INSERT INTO pessoas (nome_guerra, nome_completo, grupo_id, status) VALUES ('SD ALPHA', 'Alpha da Silva', ?, 'ativo')`, gidSub); err != nil {
		t.Fatalf("erro ao inserir pessoa: %v", err)
	}
	if _, err := st.db.Exec(`INSERT INTO pessoas (nome_guerra, nome_completo, grupo_id, status) VALUES ('SD BRAVO', 'Bravo Souza', ?, 'ativo')`, gidSub); err != nil {
		t.Fatalf("erro ao inserir pessoa: %v", err)
	}

	com := loginAs(t, app, "comandante", "senha12345")

	// Consultar API de consciência situacional
	recResumo, res := doJSONReq(app, "GET", "/api/consciencia/resumo", nil, com)
	if recResumo.Code != http.StatusOK {
		t.Fatalf("esperava 200 em /api/consciencia/resumo, obteve %d", recResumo.Code)
	}

	t.Logf("RESUMO RESULT: %+v", res)

	totalEf := int(res["total_efetivo"].(float64))
	if totalEf != 2 {
		t.Fatalf("esperava total_efetivo = 2, obteve %d", totalEf)
	}

	subs := res["subordinados"].([]any)
	if len(subs) != 2 { // superior + subordinado
		t.Fatalf("esperava 2 grupos monitorados, obteve %d", len(subs))
	}
}
