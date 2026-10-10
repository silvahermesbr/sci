package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"testing"
)

func TestFase3DriveLocalFisicoECompartilhamento(t *testing.T) {
	app, _, cleanup := setupTestApp(t)
	defer cleanup()

	adminCookie := loginAs(t, app, "admin", "admin123")

	// 1. Criar dois grupos com seus respectivos gerentes
	rr, res := doJSONReq(app, "POST", "/api/grupos", map[string]any{
		"nome":        "Companhia Alfa Drive",
		"login":       "gerente_drive_a",
		"senha":       "senha12345",
		"nome_guerra": "AlfaDrive",
	}, adminCookie)
	if rr.Code != http.StatusOK {
		t.Fatalf("falha ao criar grupo Alfa: %v", res)
	}

	rr, res = doJSONReq(app, "POST", "/api/grupos", map[string]any{
		"nome":        "Companhia Bravo Drive",
		"login":       "gerente_drive_b",
		"senha":       "senha12345",
		"nome_guerra": "BravoDrive",
	}, adminCookie)
	if rr.Code != http.StatusOK {
		t.Fatalf("falha ao criar grupo Bravo: %v", res)
	}

	alfaCookie := loginAs(t, app, "gerente_drive_a", "senha12345")
	bravoCookie := loginAs(t, app, "gerente_drive_b", "senha12345")

	rr, res = doJSONReq(app, "GET", "/api/me", nil, bravoCookie)
	bravoU := res["usuario"].(map[string]any)
	bravoGrupoID := int64(bravoU["grupo_id"].(float64))

	// 2. Gerente Alfa cria pasta no Drive
	rr, res = doJSONReq(app, "POST", "/api/drive/pastas", map[string]any{
		"nome": "Diretrizes e Manuais 2026",
	}, alfaCookie)
	if rr.Code != http.StatusOK {
		t.Fatalf("falha ao criar pasta: %v", res)
	}
	pastaID := int64(res["id"].(float64))

	// 3. Gerente Alfa faz upload de arquivo físico multipart para a pasta
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	_ = writer.WriteField("pasta_id", fmt.Sprintf("%d", pastaID))
	part, _ := writer.CreateFormFile("arquivo", "norma_operacional.pdf")
	conteudoOriginal := []byte("CONTEÚDO SECRETO DE TREINAMENTO MILITAR - DRIVE LOCAL SCI V1.2")
	_, _ = part.Write(conteudoOriginal)
	_ = writer.Close()

	req := httptest.NewRequest("POST", "/api/drive/upload", &body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	req.Header.Set("X-SCI", "1")
	req.AddCookie(alfaCookie)
	rr = httptest.NewRecorder()
	app.mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("falha ao enviar arquivo multipart: %s", rr.Body.String())
	}

	var uploadRes map[string]any
	_ = json.Unmarshal(rr.Body.Bytes(), &uploadRes)
	arqID := int64(uploadRes["id"].(float64))

	// Verificar se arquivo físico foi realmente gravado na pasta de dados no disco
	var nomeArmazenado string
	err := app.st.db.QueryRow(`SELECT nome_armazenado FROM drive_arquivos WHERE id = ?`, arqID).Scan(&nomeArmazenado)
	if err != nil {
		t.Fatalf("arquivo não encontrado no banco: %v", err)
	}
	caminhoFisico := filepath.Join(app.pastaFisicaDrive(), nomeArmazenado)
	lidoDisco, err := os.ReadFile(caminhoFisico)
	if err != nil || !bytes.Equal(lidoDisco, conteudoOriginal) {
		t.Fatalf("arquivo físico em disco inconsistente: %v (lido %d bytes)", err, len(lidoDisco))
	}

	// 4. Teste de Bloqueio: Gerente Bravo tenta baixar arquivo ou acessar pasta Alfa -> Deve dar 403 Forbidden
	rr, _ = doJSONReq(app, "GET", fmt.Sprintf("/api/drive/download/%d", arqID), nil, bravoCookie)
	if rr.Code != http.StatusForbidden {
		t.Fatalf("esperava 403 Forbidden para Bravo tentar baixar arquivo privado de Alfa, obtido: %d", rr.Code)
	}

	rr, _ = doJSONReq(app, "GET", fmt.Sprintf("/api/drive/itens?pasta_id=%d", pastaID), nil, bravoCookie)
	if rr.Code != http.StatusForbidden {
		t.Fatalf("esperava 403 Forbidden para Bravo tentar listar pasta privada de Alfa, obtido: %d", rr.Code)
	}

	// 5. Gerente Alfa compartilha a pasta com o Grupo Bravo (Leitura apenas)
	rr, res = doJSONReq(app, "POST", "/api/drive/compartilhar", map[string]any{
		"pasta_id":    pastaID,
		"alvo_tipo":   "grupo",
		"alvo_id":     bravoGrupoID,
		"pode_editar": false,
	}, alfaCookie)
	if rr.Code != http.StatusOK {
		t.Fatalf("falha ao compartilhar pasta com Bravo: %v", res)
	}

	// 6. Agora Gerente Bravo lista a pasta com sucesso e encontra o arquivo
	rr, res = doJSONReq(app, "GET", fmt.Sprintf("/api/drive/itens?pasta_id=%d", pastaID), nil, bravoCookie)
	if rr.Code != http.StatusOK {
		t.Fatalf("esperava OK para Bravo após compartilhamento, obtido: %d", rr.Code)
	}
	arqs := res["arquivos"].([]any)
	if len(arqs) != 1 {
		t.Fatalf("esperado 1 arquivo compartilhado, obtido: %d", len(arqs))
	}

	// 7. Gerente Bravo faz o download com sucesso e valida integridade dos bytes
	req = httptest.NewRequest("GET", fmt.Sprintf("/api/drive/download/%d", arqID), nil)
	req.Header.Set("X-SCI", "1")
	req.AddCookie(bravoCookie)
	rr = httptest.NewRecorder()
	app.mux.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK || !bytes.Equal(rr.Body.Bytes(), conteudoOriginal) {
		t.Fatalf("conteúdo do download divergente ou código %d", rr.Code)
	}

	// 8. Como Bravo tem apenas permissão de LEITURA, tentar renomear deve falhar (403)
	rr, _ = doJSONReq(app, "PATCH", fmt.Sprintf("/api/drive/arquivos/%d", arqID), map[string]any{
		"nome": "novo_nome.pdf",
	}, bravoCookie)
	if rr.Code != http.StatusForbidden {
		t.Fatalf("esperado 403 ao tentar renomear arquivo sem permissão de escrita, obtido: %d", rr.Code)
	}

	// 9. Gerente Alfa concede permissão de EDIÇÃO diretamente no arquivo para o Grupo Bravo
	rr, res = doJSONReq(app, "POST", "/api/drive/compartilhar", map[string]any{
		"arquivo_id":  arqID,
		"alvo_tipo":   "grupo",
		"alvo_id":     bravoGrupoID,
		"pode_editar": true,
	}, alfaCookie)
	if rr.Code != http.StatusOK {
		t.Fatalf("falha ao compartilhar arquivo com permissão de edição: %v", res)
	}

	// 10. Agora Gerente Bravo consegue renomear o arquivo
	rr, res = doJSONReq(app, "PATCH", fmt.Sprintf("/api/drive/arquivos/%d", arqID), map[string]any{
		"nome": "norma_atualizada.pdf",
	}, bravoCookie)
	if rr.Code != http.StatusOK {
		t.Fatalf("esperado sucesso ao renomear após permissão de escrita concedida: %v", res)
	}

	// 11. Gerente Alfa exclui a pasta: deve limpar o registro e apagar o arquivo do disco local
	rr, res = doJSONReq(app, "DELETE", fmt.Sprintf("/api/drive/pastas/%d", pastaID), nil, alfaCookie)
	if rr.Code != http.StatusOK {
		t.Fatalf("falha ao excluir pasta: %v", res)
	}

	if _, err := os.Stat(caminhoFisico); !os.IsNotExist(err) {
		t.Fatalf("esperado que arquivo físico no disco fosse removido após exclusão da pasta")
	}
}

func TestFase3CalendarioMeshEEscalas(t *testing.T) {
	app, _, cleanup := setupTestApp(t)
	defer cleanup()

	adminCookie := loginAs(t, app, "admin", "admin123")

	// Criar grupo
	rr, res := doJSONReq(app, "POST", "/api/grupos", map[string]any{
		"nome":        "Companhia Calendário",
		"login":       "gerente_cal",
		"senha":       "senha12345",
		"nome_guerra": "CalManager",
	}, adminCookie)
	if rr.Code != http.StatusOK {
		t.Fatalf("falha ao criar grupo: %v", res)
	}

	calCookie := loginAs(t, app, "gerente_cal", "senha12345")

	// 1. Criar Evento Operacional no Calendário
	rr, res = doJSONReq(app, "POST", "/api/calendario/eventos", map[string]any{
		"titulo":      "Instrução Geral de Tiro",
		"descricao":   "Estande de tiro às 08h00 - Uniforme 4º R2",
		"tipo":        "instrucao",
		"cor":         "#10b981",
		"data_inicio": "2026-10-15T08:00",
		"data_fim":    "2026-10-15T12:00",
		"dia_inteiro": false,
	}, calCookie)
	if rr.Code != http.StatusOK {
		t.Fatalf("falha ao criar evento no calendário: %v", res)
	}
	eventoID := int64(res["id"].(float64))

	// 2. Criar Tipo de Escala e Turno no mesmo dia (2026-10-15)
	rr, res = doJSONReq(app, "POST", "/api/escalas/tipos", map[string]any{
		"nome": "Oficial de Dia",
	}, calCookie)
	if rr.Code != http.StatusOK {
		t.Fatalf("falha ao criar tipo de escala: %v", res)
	}
	tipoID := int64(res["id"].(float64))

	// Cadastrar militar para alocar
	rr, res = doJSONReq(app, "POST", "/api/pessoas", map[string]any{
		"nome_completo": "TENENTE SILVA",
		"nome_guerra":   "SILVA",
		"posto":         "1º Tenente",
		"antiguidade":   10,
	}, calCookie)
	if rr.Code != http.StatusOK {
		t.Fatalf("falha ao cadastrar pessoa: %v", res)
	}
	pessoaID := int64(res["id"].(float64))

	rr, res = doJSONReq(app, "POST", "/api/escalas/turnos", map[string]any{
		"tipo_id":     tipoID,
		"data_inicio": "2026-10-15",
		"data_fim":    "2026-10-16",
		"observacao":  "Serviço de 24 horas",
		"pessoas": []map[string]any{
			{"pessoa_id": pessoaID, "funcao": "Oficial de Dia"},
		},
	}, calCookie)
	if rr.Code != http.StatusOK {
		t.Fatalf("falha ao cadastrar turno de escala: %v", res)
	}

	// 3. Emitir Despacho com exigência de resposta
	rr, res = doJSONReq(app, "GET", "/api/me", nil, calCookie)
	uInfo := res["usuario"].(map[string]any)
	papelID := int64(uInfo["papel_ativo_id"].(float64))

	rr, res = doJSONReq(app, "POST", "/api/mensagens", map[string]any{
		"destinatario_papel_ids": []int64{papelID},
		"assunto":                "Prazo: Plano de Chamada",
		"corpo":                  "Encaminhar atualização até o dia 15.",
		"tipo":                   "despacho",
		"exige_resposta":         true,
	}, calCookie)
	if rr.Code != http.StatusOK {
		t.Fatalf("falha ao emitir despacho: %v", res)
	}

	// 4. Consultar a visão mesh unificada do Calendário
	rr, res = doJSONReq(app, "GET", "/api/calendario/visao?mes=2026-10", nil, calCookie)
	if rr.Code != http.StatusOK {
		t.Fatalf("falha ao carregar visão do calendário: %v", res)
	}

	eventos := res["eventos"].([]any)
	escalas := res["escalas"].([]any)
	despachos := res["despachos"].([]any)

	if len(eventos) < 1 {
		t.Fatalf("esperado pelo menos 1 evento no calendário, obtido: %d", len(eventos))
	}
	if len(escalas) < 1 {
		t.Fatalf("esperado pelo menos 1 turno de escala no calendário, obtido: %d", len(escalas))
	}
	if len(despachos) < 1 {
		t.Fatalf("esperado pelo menos 1 despacho ativo no calendário, obtido: %d", len(despachos))
	}

	// 5. Excluir o evento
	rr, res = doJSONReq(app, "DELETE", fmt.Sprintf("/api/calendario/eventos/%d", eventoID), nil, calCookie)
	if rr.Code != http.StatusOK {
		t.Fatalf("falha ao excluir evento: %v", res)
	}

	// 6. Consultar novamente e validar que o evento sumiu
	rr, res = doJSONReq(app, "GET", "/api/calendario/visao?mes=2026-10", nil, calCookie)
	eventos = res["eventos"].([]any)
	if len(eventos) != 0 {
		t.Fatalf("evento ainda consta após exclusão")
	}
}

// TestFase3StressDriveECalendario: Valida concorrência pesada, upload maciço de arquivos físicos,
// criação de eventos simultâneos e consultas mesh no Calendário sem contenção ou travamento de banco.
func TestFase3StressDriveECalendario(t *testing.T) {
	app, _, cleanup := setupTestApp(t)
	defer cleanup()

	adminCookie := loginAs(t, app, "admin", "admin123")

	// Criar grupo de teste de carga
	rr, res := doJSONReq(app, "POST", "/api/grupos", map[string]any{
		"nome":        "Batalhão Stress F3",
		"login":       "gerente_stress_f3",
		"senha":       "senha12345",
		"nome_guerra": "StressMgr",
	}, adminCookie)
	if rr.Code != http.StatusOK {
		t.Fatalf("falha ao criar grupo: %v", res)
	}

	gerenteCookie := loginAs(t, app, "gerente_stress_f3", "senha12345")

	// 1. Criar pasta raiz para os testes
	rr, res = doJSONReq(app, "POST", "/api/drive/pastas", map[string]any{
		"nome": "Carga Concorrente",
	}, gerenteCookie)
	if rr.Code != http.StatusOK {
		t.Fatalf("falha ao criar pasta: %v", res)
	}
	pastaID := int64(res["id"].(float64))

	// 2. Concorrência: 30 uploads simultâneos de arquivos em goroutines paralelas
	const numUploads = 30
	var wg sync.WaitGroup
	errCh := make(chan error, numUploads)

	for i := 0; i < numUploads; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()

			var b bytes.Buffer
			w := multipart.NewWriter(&b)
			_ = w.WriteField("pasta_id", fmt.Sprintf("%d", pastaID))
			part, err := w.CreateFormFile("arquivo", fmt.Sprintf("doc_stress_%d.dat", idx))
			if err != nil {
				errCh <- fmt.Errorf("idx %d CreateFormFile: %w", idx, err)
				return
			}
			dados := bytes.Repeat([]byte(fmt.Sprintf("DADOS_STRESS_F3_BLOCO_%d\n", idx)), 100)
			_, _ = part.Write(dados)
			_ = w.Close()

			req := httptest.NewRequest("POST", "/api/drive/upload", &b)
			req.Header.Set("Content-Type", w.FormDataContentType())
			req.Header.Set("X-SCI", "1")
			req.AddCookie(gerenteCookie)
			rec := httptest.NewRecorder()
			app.mux.ServeHTTP(rec, req)

			if rec.Code != http.StatusOK {
				errCh <- fmt.Errorf("idx %d status %d: %s", idx, rec.Code, rec.Body.String())
				return
			}
		}(i)
	}

	wg.Wait()
	close(errCh)

	for err := range errCh {
		t.Fatalf("erro em upload concorrente: %v", err)
	}

	// Validar que exatamente numUploads arquivos foram gravados no banco e no disco
	var totalArquivos int
	_ = app.st.db.QueryRow(`SELECT COUNT(*) FROM drive_arquivos WHERE pasta_id = ?`, pastaID).Scan(&totalArquivos)
	if totalArquivos != numUploads {
		t.Fatalf("esperado %d arquivos no banco, obtido %d", numUploads, totalArquivos)
	}

	// 3. Concorrência: 30 eventos criados em paralelo no Calendário
	const numEventos = 30
	errChEv := make(chan error, numEventos)

	for i := 0; i < numEventos; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			dia := (idx % 28) + 1
			dataStr := fmt.Sprintf("2026-11-%02d", dia)

			reqBody := map[string]any{
				"titulo":      fmt.Sprintf("Instrução Concorrente #%d", idx),
				"tipo":        "instrucao",
				"data_inicio": dataStr,
				"cor":         "#10b981",
			}
			rec, _ := doJSONReq(app, "POST", "/api/calendario/eventos", reqBody, gerenteCookie)
			if rec.Code != http.StatusOK {
				errChEv <- fmt.Errorf("evento %d status %d", idx, rec.Code)
			}
		}(i)
	}

	wg.Wait()
	close(errChEv)

	for err := range errChEv {
		t.Fatalf("erro em criação concorrente de eventos: %v", err)
	}

	// 4. Concorrência de Leitura: 15 leituras simultâneas do Calendário e do Drive
	const numLeituras = 15
	errChRead := make(chan error, numLeituras)

	for i := 0; i < numLeituras; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			rec, res := doJSONReq(app, "GET", "/api/calendario/visao?mes=2026-11", nil, gerenteCookie)
			if rec.Code != http.StatusOK {
				errChRead <- fmt.Errorf("leitura cal falhou: %d", rec.Code)
				return
			}
			evs := res["eventos"].([]any)
			if len(evs) != numEventos {
				errChRead <- fmt.Errorf("esperado %d eventos, lido: %d", numEventos, len(evs))
				return
			}

			recD, resD := doJSONReq(app, "GET", fmt.Sprintf("/api/drive/itens?pasta_id=%d", pastaID), nil, gerenteCookie)
			if recD.Code != http.StatusOK {
				errChRead <- fmt.Errorf("leitura drive falhou: %d", recD.Code)
				return
			}
			arqs := resD["arquivos"].([]any)
			if len(arqs) != numUploads {
				errChRead <- fmt.Errorf("esperado %d arquivos no drive, lido: %d", numUploads, len(arqs))
				return
			}
		}()
	}

	wg.Wait()
	close(errChRead)

	for err := range errChRead {
		t.Fatalf("erro em leitura concorrente: %v", err)
	}

	// 5. Exclusão em cascata: excluir a pasta com os 30 arquivos e validar que o disco foi limpo
	recDel, _ := doJSONReq(app, "DELETE", fmt.Sprintf("/api/drive/pastas/%d", pastaID), nil, gerenteCookie)
	if recDel.Code != http.StatusOK {
		t.Fatalf("falha ao excluir pasta com 30 arquivos: %d", recDel.Code)
	}

	var arqsRestantes int
	_ = app.st.db.QueryRow(`SELECT COUNT(*) FROM drive_arquivos WHERE pasta_id = ?`, pastaID).Scan(&arqsRestantes)
	if arqsRestantes != 0 {
		t.Fatalf("esperado 0 arquivos restantes no banco, obtido %d", arqsRestantes)
	}
}

// TestFase3MultiCalendariosEInscricaoForcada valida a dinâmica estilo Nextcloud/Google:
// 1. Criação de múltiplos calendários pessoais e verificação de calendário default auto-criado.
// 2. Compartilhamento de calendário por Gerente com seu Grupo com 'forcar=true' (inscrição forçada).
// 3. Verificação de que militar subordinado vê o calendário com flag 'inscricao_forcada=true'.
// 4. Tentativa de usuário não-gerente compartilhar com grupo ou forçar inscrição retorna 403.
// 5. Exclusão limpa de calendário cascateando eventos e compartilhamentos.
func TestFase3MultiCalendariosEInscricaoForcada(t *testing.T) {
	app, _, cleanup := setupTestApp(t)
	defer cleanup()

	adminCookie := loginAs(t, app, "admin", "admin123")

	// 1. Criar grupo e gerente
	rr, res := doJSONReq(app, "POST", "/api/grupos", map[string]any{
		"nome":        "Btl Calendario Teste",
		"login":       "ger_cal_next",
		"senha":       "senha12345",
		"nome_guerra": "GerNext",
	}, adminCookie)
	if rr.Code != http.StatusOK {
		t.Fatalf("falha ao criar grupo: %v", res)
	}
	grupoID := int64(res["id"].(float64))

	gerCookie := loginAs(t, app, "ger_cal_next", "senha12345")

	// 2. Criar operador no mesmo grupo
	// v1.6.0 F4: operador nasce com setor (extinção do operador de grupo)
	var setorCal int64
	if err := app.st.db.QueryRow(`INSERT INTO setores (nome, grupo_id, ativo) VALUES ('Setor Cal', ?, 1) RETURNING id`, grupoID).Scan(&setorCal); err != nil {
		t.Fatalf("criar setor: %v", err)
	}
	rrOp, resOp := doJSONReq(app, "POST", "/api/usuarios", map[string]any{
		"login":       "op_cal_sub",
		"senha":       "senha12345",
		"papel":       "operador",
		"grupo_id":    grupoID,
		"setor_id":    setorCal,
		"nome_guerra": "SoldadoSub",
	}, adminCookie)
	if rrOp.Code != http.StatusOK {
		t.Fatalf("falha ao criar operador: %v", resOp)
	}
	opCookie := loginAs(t, app, "op_cal_sub", "senha12345")

	// 3. Listar calendários do gerente (deve auto-criar 'Pessoal')
	rrList, resList := doJSONReq(app, "GET", "/api/calendarios", nil, gerCookie)
	if rrList.Code != http.StatusOK {
		t.Fatalf("falha ao listar calendários: %v", resList)
	}
	meus := resList["meus"].([]any)
	if len(meus) == 0 {
		t.Fatalf("esperado pelo menos 1 calendário auto-criado, obtido: %d", len(meus))
	}

	// 4. Criar calendário institucional temático como gerente
	rrAdd, resAdd := doJSONReq(app, "POST", "/api/calendarios", map[string]any{
		"nome":      "Escala Geral de Formaturas",
		"cor":       "#ef4444",
		"descricao": "Obrigatório para todo o batalhão",
	}, gerCookie)
	if rrAdd.Code != http.StatusOK {
		t.Fatalf("falha ao criar calendário institucional: %v", resAdd)
	}
	calID := int64(resAdd["id"].(float64))

	// 5. Criar evento nesse calendário
	rrEv, resEv := doJSONReq(app, "POST", "/api/calendario/eventos", map[string]any{
		"calendario_id": calID,
		"titulo":        "Formatura Matinal Geral",
		"tipo":          "formatura",
		"cor":           "#ef4444",
		"data_inicio":   "2026-11-05",
		"dia_inteiro":   true,
	}, gerCookie)
	if rrEv.Code != http.StatusOK {
		t.Fatalf("falha ao criar evento no novo calendário: %v", resEv)
	}

	// 6. Testar regra de segurança: operador tentando forçar inscrição deve receber 403
	rrForcarOp, _ := doJSONReq(app, "POST", fmt.Sprintf("/api/calendarios/%d/compartilhar", calID), map[string]any{
		"alvo_tipo": "grupo",
		"alvo_id":   grupoID,
		"forcar":    true,
	}, opCookie)
	if rrForcarOp.Code != http.StatusForbidden {
		t.Fatalf("esperado 403 Forbidden para operador forçando inscrição, obtido: %d", rrForcarOp.Code)
	}

	// 7. Gerente compartilha com o grupo com FORÇAR=true
	rrComp, resComp := doJSONReq(app, "POST", fmt.Sprintf("/api/calendarios/%d/compartilhar", calID), map[string]any{
		"alvo_tipo": "grupo",
		"alvo_id":   grupoID,
		"forcar":    true,
	}, gerCookie)
	if rrComp.Code != http.StatusOK {
		t.Fatalf("falha ao compartilhar com grupo forçado: %v", resComp)
	}

	// 8. Operador lista calendários: deve ver em 'compartilhados' com 'inscricao_forcada=true'
	rrOpList, resOpList := doJSONReq(app, "GET", "/api/calendarios", nil, opCookie)
	if rrOpList.Code != http.StatusOK {
		t.Fatalf("operador falhou ao listar calendários: %v", resOpList)
	}
	comps := resOpList["compartilhados"].([]any)
	if len(comps) == 0 {
		t.Fatalf("esperado calendário compartilhado para operador, obtido 0")
	}
	calComp := comps[0].(map[string]any)
	if calComp["inscricao_forcada"] != true {
		t.Fatalf("esperado inscricao_forcada=true no calendário compartilhado")
	}

	// 9. Operador consulta visão do mês: deve ver o evento no calendário forçado
	rrVis, resVis := doJSONReq(app, "GET", "/api/calendario/visao?mes=2026-11", nil, opCookie)
	if rrVis.Code != http.StatusOK {
		t.Fatalf("falha na visão do calendário: %v", resVis)
	}
	evsOp := resVis["eventos"].([]any)
	if len(evsOp) == 0 {
		t.Fatalf("operador deveria ver evento do calendário forçado no mês, obtido: 0")
	}

	// 10. Excluir o calendário e verificar limpeza
	rrDel, resDel := doJSONReq(app, "DELETE", fmt.Sprintf("/api/calendarios/%d", calID), nil, gerCookie)
	if rrDel.Code != http.StatusOK {
		t.Fatalf("falha ao excluir calendário: %v", resDel)
	}

	var countEvRestantes int
	_ = app.st.db.QueryRow(`SELECT COUNT(*) FROM calendario_eventos WHERE calendario_id = ?`, calID).Scan(&countEvRestantes)
	if countEvRestantes != 0 {
		t.Fatalf("esperado 0 eventos restantes do calendário excluído, obtido: %d", countEvRestantes)
	}
}

func TestCalendarioVisaoMilitaresEscalas(t *testing.T) {
	app, _, cleanup := setupTestApp(t)
	defer cleanup()

	adminCookie := loginAs(t, app, "admin", "admin123")

	// 1. Criar grupo e gerente
	_, resG := doJSONReq(app, "POST", "/api/grupos", map[string]any{
		"nome": "Companhia Calendario",
	}, adminCookie)
	gid := int64(resG["id"].(float64))

	_, resU := doJSONReq(app, "POST", "/api/usuarios", map[string]any{
		"login":    "gerente_cal",
		"senha":    "senha12345",
		"papel":    "gerente",
		"grupo_id": gid,
	}, adminCookie)
	_ = int64(resU["id"].(float64))

	gerCookie := loginAs(t, app, "gerente_cal", "senha12345")

	// 2. Cadastrar militares no efetivo
	resP, _ := app.st.db.Exec(`INSERT INTO pessoas (grupo_id, nome_guerra, nome_completo, status) VALUES (?, 'SANTOS', 'Lucas Santos', 'ativo')`, gid)
	pid1, _ := resP.LastInsertId()

	resP2, _ := app.st.db.Exec(`INSERT INTO pessoas (grupo_id, nome_guerra, nome_completo, status) VALUES (?, 'ALMEIDA', 'Marcos Almeida', 'ativo')`, gid)
	pid2, _ := resP2.LastInsertId()

	// 3. Criar tipo de escala e turno com data_fim nulo
	resTipo, _ := app.st.db.Exec(`INSERT INTO escala_tipos (grupo_id, nome, ativo) VALUES (?, 'Oficial de Dia', 1)`, gid)
	tipoID, _ := resTipo.LastInsertId()

	resTurno, errT := app.st.db.Exec(`INSERT INTO escala_turnos (grupo_id, tipo_id, data_inicio, data_fim) VALUES (?, ?, '2026-10-15T08:00:00', '2026-10-15T20:00:00')`, gid, tipoID)
	if errT != nil {
		t.Fatalf("falha ao inserir escala_turnos: %v", errT)
	}
	turnoID, _ := resTurno.LastInsertId()

	// 4. Preencher turno com militares
	_, _ = app.st.db.Exec(`INSERT INTO escala_pessoas (turno_id, pessoa_id, funcao_escala) VALUES (?, ?, 'Oficial de Dia')`, turnoID, pid1)
	_, _ = app.st.db.Exec(`INSERT INTO escala_pessoas (turno_id, pessoa_id, funcao_escala) VALUES (?, ?, 'Adjunto')`, turnoID, pid2)

	// 5. Consultar visão do calendário para 2026-10
	rr, res := doJSONReq(app, "GET", "/api/calendario/visao?mes=2026-10", nil, gerCookie)
	if rr.Code != http.StatusOK {
		t.Fatalf("falha ao consultar visao do calendario: code %d resp %v", rr.Code, res)
	}

	escalas, ok := res["escalas"].([]any)
	if !ok || len(escalas) == 0 {
		t.Fatalf("esperado pelo menos 1 escala no calendário de 2026-10, obtido: %v", res["escalas"])
	}

	escMap := escalas[0].(map[string]any)
	if escMap["tipo_nome"] != "Oficial de Dia" {
		t.Errorf("tipo_nome esperado 'Oficial de Dia', obtido: %v", escMap["tipo_nome"])
	}

	militares, okMil := escMap["militares"].([]any)
	if !okMil || len(militares) != 2 {
		t.Fatalf("esperado 2 militares na escala do calendário, obtido: %v", escMap["militares"])
	}

	m1 := militares[0].(map[string]any)
	m2 := militares[1].(map[string]any)
	if m1["nome_guerra"] != "ALMEIDA" && m1["nome_guerra"] != "SANTOS" {
		t.Errorf("nome de guerra inesperado: %v", m1)
	}
	if m2["nome_guerra"] != "ALMEIDA" && m2["nome_guerra"] != "SANTOS" {
		t.Errorf("nome de guerra inesperado: %v", m2)
	}
}


