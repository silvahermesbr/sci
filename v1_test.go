package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"
)

func setupTestApp(t *testing.T) (*App, *Store, func()) {
	tmpDir, err := os.MkdirTemp("", "sci_test_*")
	if err != nil {
		t.Fatalf("falha ao criar tmpDir: %v", err)
	}
	st, err := AbrirStore(tmpDir)
	if err != nil {
		os.RemoveAll(tmpDir)
		t.Fatalf("falha ao abrir store: %v", err)
	}
	if err := st.SeedIfEmpty("admin123"); err != nil {
		st.Close()
		os.RemoveAll(tmpDir)
		t.Fatalf("falha ao rodar seed: %v", err)
	}
	app := NovaApp(st)
	cleanup := func() {
		st.Close()
		os.RemoveAll(tmpDir)
	}
	return app, st, cleanup
}

func doJSONReq(app *App, method, path string, body any, cookie *http.Cookie) (*httptest.ResponseRecorder, map[string]any) {
	var bodyReader *bytes.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		bodyReader = bytes.NewReader(b)
	} else {
		bodyReader = bytes.NewReader([]byte{})
	}
	req := httptest.NewRequest(method, path, bodyReader)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-SCI", "1")
	if cookie != nil {
		req.AddCookie(cookie)
	}
	rr := httptest.NewRecorder()
	app.mux.ServeHTTP(rr, req)

	var res map[string]any
	_ = json.Unmarshal(rr.Body.Bytes(), &res)
	return rr, res
}

func loginAs(t *testing.T, app *App, login, senha string) *http.Cookie {
	rr, res := doJSONReq(app, "POST", "/api/login", map[string]string{"login": login, "senha": senha}, nil)
	if rr.Code != http.StatusOK {
		t.Fatalf("login falhou para %s: %v", login, res)
	}
	for _, c := range rr.Result().Cookies() {
		if c.Name == cookieSessao {
			return c
		}
	}
	t.Fatalf("cookie de sessão não retornado")
	return nil
}

func TestMigrationsAndSeeds(t *testing.T) {
	_, st, cleanup := setupTestApp(t)
	defer cleanup()

	// 1. Validar versão de schema
	var versao int
	err := st.db.QueryRow(`SELECT MAX(versao) FROM schema_migrations`).Scan(&versao)
	if err != nil || versao != 16 {
		t.Fatalf("esperado schema versão 16, obtido: %d (err: %v)", versao, err)
	}

	// 2. Validar que as tabelas de Escalas, Material e Configurações existem
	tabelas := []string{"escala_tipos", "escala_turnos", "escala_pessoas", "material_categorias", "material_itens", "material_cautelas", "material_cautela_anexos", "configuracoes"}
	for _, tab := range tabelas {
		var n int
		err = st.db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name=?`, tab).Scan(&n)
		if err != nil || n != 1 {
			t.Fatalf("tabela %s não encontrada no banco", tab)
		}
	}

	// 3. Validar seeds padrão
	var tiposCount, catCount, cfgCount int
	_ = st.db.QueryRow(`SELECT COUNT(*) FROM escala_tipos`).Scan(&tiposCount)
	_ = st.db.QueryRow(`SELECT COUNT(*) FROM material_categorias`).Scan(&catCount)
	_ = st.db.QueryRow(`SELECT COUNT(*) FROM configuracoes`).Scan(&cfgCount)

	if tiposCount < 8 {
		t.Errorf("esperado ao menos 8 tipos de escala padrão, obtido: %d", tiposCount)
	}
	if catCount < 7 {
		t.Errorf("esperado ao menos 7 categorias de material, obtido: %d", catCount)
	}
	if cfgCount < 10 {
		t.Errorf("esperado ao menos 10 configurações globais, obtido: %d", cfgCount)
	}
}

func TestEscalasAndConferenciaIntegration(t *testing.T) {
	app, st, cleanup := setupTestApp(t)
	defer cleanup()

	adminCookie := loginAs(t, app, "admin", "admin123")

	// 1. Criar um grupo e gerente
	resGrupo, _ := st.db.Exec(`INSERT INTO grupos (nome, codigo) VALUES ('1ª Companhia', 'ABC123')`)
	grupoID, _ := resGrupo.LastInsertId()

	hashGer, _ := hashSenha("gerente123")
	_, _ = st.db.Exec(`INSERT INTO usuarios (login, senha_hash, papel, grupo_id, ativo) VALUES ('gerente1', ?, 'gerente', ?, 1)`, hashGer, grupoID)
	gerenteCookie := loginAs(t, app, "gerente1", "gerente123")

	// 2. Cadastrar militares no grupo
	resP1, _ := st.db.Exec(`INSERT INTO pessoas (nome_guerra, nome_completo, grupo_id, status) VALUES ('SILVA', 'João Silva', ?, 'ativo')`, grupoID)
	p1ID, _ := resP1.LastInsertId()

	resP2, _ := st.db.Exec(`INSERT INTO pessoas (nome_guerra, nome_completo, grupo_id, status) VALUES ('SOUZA', 'Carlos Souza', ?, 'ativo')`, grupoID)
	p2ID, _ := resP2.LastInsertId()

	// 3. Criar turno de serviço para SILVA na data de hoje
	hoje := time.Now().Format("2006-01-02")
	turnoPayload := map[string]any{
		"tipo_id":     1, // Oficial de Dia
		"data_inicio": hoje,
		"data_fim":    hoje,
		"observacao":  "Guarda Principal",
		"pessoas": []map[string]any{
			{"pessoa_id": p1ID, "funcao_escala": "Sentinela 1"},
		},
	}
	rr, respTurno := doJSONReq(app, "POST", "/api/escalas/turnos", turnoPayload, gerenteCookie)
	if rr.Code != http.StatusOK || respTurno["ok"] != true {
		t.Fatalf("falha ao criar turno de escala: %v", respTurno)
	}

	// 4. Verificar endpoint /api/escalas/hoje
	rr, respHoje := doJSONReq(app, "GET", "/api/escalas/hoje", nil, gerenteCookie)
	if rr.Code != http.StatusOK {
		t.Fatalf("erro ao consultar /api/escalas/hoje: %v", respHoje)
	}
	escalados, _ := respHoje["escalados"].([]any)
	if len(escalados) != 1 {
		t.Fatalf("esperado 1 militar escalado hoje, obtido: %d", len(escalados))
	}

	// 5. Iniciar Conferência e verificar INTEGRAÇÃO (Motor Inteligente)
	rrConf, respConf := doJSONReq(app, "POST", "/api/conferencia/iniciar", map[string]string{"local": "Pátio"}, gerenteCookie)
	if rrConf.Code != http.StatusOK {
		t.Fatalf("erro ao iniciar conferência: %v", respConf)
	}
	confID := int64(respConf["id"].(float64))

	// 6. Verificar que SILVA (escalado) foi automaticamente atribuído como Justificada / Serviço de Escala
	var sitSilva, obsSilva string
	var destID *int64
	err := st.db.QueryRow(`SELECT situacao, destino_id, observacao FROM presencas WHERE conferencia_id = ? AND pessoa_id = ?`, confID, p1ID).
		Scan(&sitSilva, &destID, &obsSilva)
	if err != nil {
		t.Fatalf("presença de SILVA não encontrada na conferência: %v", err)
	}
	if sitSilva != "justificada" || destID == nil || *destID <= 0 {
		t.Fatalf("SILVA deveria estar com situação 'justificada' e destino_id preenchido. Obtido: sit=%s, dest=%v", sitSilva, destID)
	}

	// 7. Verificar que SOUZA (não escalado) não foi forçado indevidamente
	var countSouza int
	_ = st.db.QueryRow(`SELECT COUNT(*) FROM presencas WHERE conferencia_id = ? AND pessoa_id = ?`, confID, p2ID).Scan(&countSouza)
	// SOUZA ainda não foi verificado (começa vazio no carry-over novo)
	if countSouza > 0 {
		var sitSouza string
		_ = st.db.QueryRow(`SELECT situacao FROM presencas WHERE conferencia_id = ? AND pessoa_id = ?`, confID, p2ID).Scan(&sitSouza)
		if sitSouza == "justificada" {
			t.Errorf("SOUZA não estava escalado e não deveria estar como justificada")
		}
	}
	_ = adminCookie
}

func TestMaterialAndCautelasLifecycle(t *testing.T) {
	app, st, cleanup := setupTestApp(t)
	defer cleanup()

	adminCookie := loginAs(t, app, "admin", "admin123")

	// 1. Cadastrar Grupo, Gerente e Militar
	resGrupo, _ := st.db.Exec(`INSERT INTO grupos (nome, codigo) VALUES ('2ª Cia Fuz', 'XYZ789')`)
	grupoID, _ := resGrupo.LastInsertId()

	hashGer, _ := hashSenha("gerente456")
	_, _ = st.db.Exec(`INSERT INTO usuarios (login, senha_hash, papel, grupo_id, ativo) VALUES ('gerente2', ?, 'gerente', ?, 1)`, hashGer, grupoID)
	gerenteCookie := loginAs(t, app, "gerente2", "gerente456")

	resPes, _ := st.db.Exec(`INSERT INTO pessoas (nome_guerra, nome_completo, grupo_id, status) VALUES ('OLIVEIRA', 'Lucas Oliveira', ?, 'ativo')`, grupoID)
	pesID, _ := resPes.LastInsertId()

	// 1.1. Validar que Admin NÃO tem acesso direto a Material e Escalas (HTTP 403 Forbidden)
	itemPayload := map[string]any{
		"grupo_id":          grupoID,
		"categoria_id":      1,
		"nome":              "Pistola 9mm Beretta",
		"codigo_patrimonio": "BER-9001",
		"numero_serie":      "BER7721",
		"status":            "disponivel",
		"observacao":        "Reserva do Oficial de Dia",
	}
	rrAdminMat, _ := doJSONReq(app, "POST", "/api/material/itens", itemPayload, adminCookie)
	if rrAdminMat.Code != http.StatusForbidden {
		t.Fatalf("Admin deveria receber 403 Forbidden no módulo de material, obtido: %d", rrAdminMat.Code)
	}

	rrAdminEsc, _ := doJSONReq(app, "GET", "/api/escalas/hoje", nil, adminCookie)
	if rrAdminEsc.Code != http.StatusForbidden {
		t.Fatalf("Admin deveria receber 403 Forbidden no módulo de escalas, obtido: %d", rrAdminEsc.Code)
	}

	// 2. Cadastrar Item de Material como Gerente do Grupo
	rrItem, respItem := doJSONReq(app, "POST", "/api/material/itens", itemPayload, gerenteCookie)
	if rrItem.Code != http.StatusOK || respItem["ok"] != true {
		t.Fatalf("falha ao cadastrar item de material: %v", respItem)
	}
	itemID := int64(respItem["id"].(float64))

	// 3. Cautelar Item com anexo inicial
	cautPayload := map[string]any{
		"item_id":   itemID,
		"pessoa_id": pesID,
		"obs_saida": "Para serviço de guarda",
		"anexos": []map[string]any{
			{
				"nome_arquivo": "termo_assinado.pdf",
				"tipo_mime":    "application/pdf",
				"tamanho":      1234,
				"dados_base64": "data:application/pdf;base64,SlZCRVJpMHhMakV4TVRFPQ==",
			},
		},
	}
	rrCaut, respCaut := doJSONReq(app, "POST", "/api/material/cautelar", cautPayload, gerenteCookie)
	if rrCaut.Code != http.StatusOK || respCaut["ok"] != true {
		t.Fatalf("falha ao cautelar material: %v", respCaut)
	}
	cautelaID := int64(respCaut["cautela_id"].(float64))

	// 3.1. Validar listagem de anexos da cautela
	rrAnexos, respAnexos := doJSONReq(app, "GET", fmt.Sprintf("/api/material/cautelas/%d/anexos", cautelaID), nil, gerenteCookie)
	if rrAnexos.Code != http.StatusOK {
		t.Fatalf("falha ao listar anexos: %v", respAnexos)
	}
	anexosList, ok := respAnexos["anexos"].([]any)
	if !ok || len(anexosList) != 1 {
		t.Fatalf("esperado 1 anexo inicial na cautela, obtido: %v", respAnexos)
	}
	primeiroAnexo := anexosList[0].(map[string]any)
	anexoID := int64(primeiroAnexo["id"].(float64))

	// 3.2. Adicionar segundo anexo pós-abertura
	novoAnexoPayload := map[string]any{
		"nome_arquivo": "foto_cautela_estado.jpg",
		"tipo_mime":    "image/jpeg",
		"tamanho":      5678,
		"dados_base64": "data:image/jpeg;base64,iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mNk+A8AAQUBAScY44YAAAAASUVORK5CYII=",
	}
	rrAddAnexo, respAddAnexo := doJSONReq(app, "POST", fmt.Sprintf("/api/material/cautelas/%d/anexos", cautelaID), novoAnexoPayload, gerenteCookie)
	if rrAddAnexo.Code != http.StatusOK || respAddAnexo["ok"] != true {
		t.Fatalf("falha ao adicionar segundo anexo: %v", respAddAnexo)
	}

	// 3.3. Testar download/recuperação do anexo
	rrGetAnexo, _ := doJSONReq(app, "GET", fmt.Sprintf("/api/material/anexos/%d", anexoID), nil, gerenteCookie)
	if rrGetAnexo.Code != http.StatusOK {
		t.Fatalf("falha ao buscar anexo por id: %d (body: %s)", rrGetAnexo.Code, rrGetAnexo.Body.String())
	}
	if rrGetAnexo.Header().Get("Content-Type") != "application/pdf" {
		t.Fatalf("esperado Content-Type application/pdf, obtido: %s", rrGetAnexo.Header().Get("Content-Type"))
	}
	if len(rrGetAnexo.Body.Bytes()) == 0 {
		t.Fatalf("esperado corpo do anexo não vazio")
	}

	// 4. Validar que o status do item mudou para 'acautelado' no banco
	var stItem string
	_ = st.db.QueryRow(`SELECT status FROM material_itens WHERE id = ?`, itemID).Scan(&stItem)
	if stItem != "acautelado" {
		t.Fatalf("esperado status 'acautelado', obtido: %s", stItem)
	}

	// 5. Testar proteção contra dupla-cautela (deve falhar se tentar cautelar de novo)
	rrDupla, _ := doJSONReq(app, "POST", "/api/material/cautelar", cautPayload, gerenteCookie)
	if rrDupla.Code != http.StatusBadRequest {
		t.Fatalf("tentativa de cautelar item já em uso deveria retornar 400 Bad Request, obtido: %d", rrDupla.Code)
	}

	// 6. Devolver Item
	devPayload := map[string]any{
		"cautela_id":    cautelaID,
		"obs_devolucao": "Devolvido limpo e com carregador cheio",
	}
	rrDev, respDev := doJSONReq(app, "POST", "/api/material/devolver", devPayload, gerenteCookie)
	if rrDev.Code != http.StatusOK || respDev["ok"] != true {
		t.Fatalf("falha ao devolver material: %v", respDev)
	}

	// 7. Validar que o status do item voltou para 'disponivel' e a cautela está 'devolvida'
	_ = st.db.QueryRow(`SELECT status FROM material_itens WHERE id = ?`, itemID).Scan(&stItem)
	if stItem != "disponivel" {
		t.Fatalf("esperado status 'disponivel' após devolução, obtido: %s", stItem)
	}

	var stCautela, dtDev string
	_ = st.db.QueryRow(`SELECT status, data_devolucao FROM material_cautelas WHERE id = ?`, cautelaID).Scan(&stCautela, &dtDev)
	if stCautela != "devolvida" || dtDev == "" {
		t.Fatalf("cautela deveria estar marcada como 'devolvida' com data_devolucao preenchida. st=%s, dt=%s", stCautela, dtDev)
	}

	// 8. Testar exclusão de anexo
	rrDelAnexo, respDelAnexo := doJSONReq(app, "DELETE", fmt.Sprintf("/api/material/anexos/%d", anexoID), nil, gerenteCookie)
	if rrDelAnexo.Code != http.StatusOK || respDelAnexo["ok"] != true {
		t.Fatalf("falha ao deletar anexo: %v", respDelAnexo)
	}

	// 9. Testar dar baixa no item (modo=baixar)
	rrBaixar, respBaixar := doJSONReq(app, "DELETE", fmt.Sprintf("/api/material/itens/%d?modo=baixar", itemID), nil, gerenteCookie)
	if rrBaixar.Code != http.StatusOK || respBaixar["ok"] != true || respBaixar["acao"] != "baixado" {
		t.Fatalf("falha ao dar baixa no item: %v", respBaixar)
	}
	_ = st.db.QueryRow(`SELECT status FROM material_itens WHERE id = ?`, itemID).Scan(&stItem)
	if stItem != "baixado" {
		t.Fatalf("esperado status 'baixado' após dar baixa, obtido: %s", stItem)
	}

	// 10. Testar exclusão definitiva do item
	rrDelItem, respDelItem := doJSONReq(app, "DELETE", fmt.Sprintf("/api/material/itens/%d", itemID), nil, gerenteCookie)
	if rrDelItem.Code != http.StatusOK || respDelItem["ok"] != true || respDelItem["acao"] != "excluido" {
		t.Fatalf("falha ao excluir item definitivamente: %v", respDelItem)
	}
	var countItem int
	_ = st.db.QueryRow(`SELECT COUNT(*) FROM material_itens WHERE id = ?`, itemID).Scan(&countItem)
	if countItem != 0 {
		t.Fatalf("item ainda consta no banco após exclusão definitiva")
	}
}

func TestWhiteLabelAndConfiguracoes(t *testing.T) {
	app, st, cleanup := setupTestApp(t)
	defer cleanup()

	adminCookie := loginAs(t, app, "admin", "admin123")

	// 1. Ler configurações públicas (sem auth)
	rrGet, respGet := doJSONReq(app, "GET", "/api/configuracoes", nil, nil)
	if rrGet.Code != http.StatusOK {
		t.Fatalf("erro ao ler /api/configuracoes: %v", respGet)
	}
	cfgMap, ok := respGet["configuracoes"].(map[string]any)
	if !ok || cfgMap["NOME_SISTEMA"] != "SCI" {
		t.Fatalf("configuração inicial incorreta: %v", cfgMap)
	}

	// 2. Modificar configurações para White-Label (ex.: Hospital Militar)
	novasConfigs := map[string]string{
		"NOME_SISTEMA":       "SIG-HOSP",
		"TITULO_ORGANIZACAO": "Hospital Militar de Área",
		"ROTULO_PESSOA":      "Médico / Servidor",
		"COR_PRIMARIA":       "#4a8cdb",
	}
	rrSet, respSet := doJSONReq(app, "POST", "/api/configuracoes", novasConfigs, adminCookie)
	if rrSet.Code != http.StatusOK || respSet["ok"] != true {
		t.Fatalf("falha ao salvar configurações: %v", respSet)
	}

	// 3. Verificar persistência
	var valNome, valOrg, valCor string
	_ = st.db.QueryRow(`SELECT valor FROM configuracoes WHERE chave = 'NOME_SISTEMA'`).Scan(&valNome)
	_ = st.db.QueryRow(`SELECT valor FROM configuracoes WHERE chave = 'TITULO_ORGANIZACAO'`).Scan(&valOrg)
	_ = st.db.QueryRow(`SELECT valor FROM configuracoes WHERE chave = 'COR_PRIMARIA'`).Scan(&valCor)

	if valNome != "SIG-HOSP" || valOrg != "Hospital Militar de Área" || valCor != "#4a8cdb" {
		t.Fatalf("valores gravados incorretos: nome=%s, org=%s, cor=%s", valNome, valOrg, valCor)
	}
}
