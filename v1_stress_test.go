package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"sync"
	"testing"
	"time"
)

// TestQRCodeGeneration: valida encoding do gerador próprio de QR Code sem pacotes externos.
func TestQRCodeGeneration(t *testing.T) {
	payloads := []string{
		"sci://p:1:SILVA",
		"sci://m:42:FUZIL-IA2-762",
		"sci://kiosk:checkin:2026-09-30",
	}

	for _, p := range payloads {
		qr, err := GerarQRCode(p)
		if err != nil {
			t.Fatalf("falha ao gerar QRCode para payload '%s': %v", p, err)
		}
		if qr.Size < 21 || qr.Size > 40 {
			t.Fatalf("tamanho de matriz inesperado para QRCode: %d", qr.Size)
		}

		pngBytes, err := qr.RenderPNG(6, 4)
		if err != nil || len(pngBytes) < 50 {
			t.Fatalf("falha ao renderizar PNG: %v (len %d)", err, len(pngBytes))
		}
		// Assinatura PNG: \x89PNG\r\n\x1a\n
		if string(pngBytes[:4]) != "\x89PNG" {
			t.Fatalf("formato PNG inválido para payload '%s'", p)
		}
	}
}

// TestNotificacoesSLA: valida o cálculo de SLA e hub de notificações para materiais em atraso.
func TestNotificacoesSLA(t *testing.T) {
	app, st, cleanup := setupTestApp(t)
	defer cleanup()

	adminCookie := loginAs(t, app, "admin", "admin123")

	// 1. Criar grupo
	resG, err := st.db.Exec(`INSERT INTO grupos (nome, codigo) VALUES ('1ª Cia Teste', 'G99999')`)
	if err != nil {
		t.Fatalf("criar grupo: %v", err)
	}
	gID, _ := resG.LastInsertId()

	// 2. Criar militar vinculado ao grupo
	resP, err := st.db.Exec(`INSERT INTO pessoas (nome_guerra, nome_completo, grupo_id) VALUES ('ALERTA_TESTE', 'Militar Alerta Teste', ?)`, gID)
	if err != nil {
		t.Fatalf("criar pessoa: %v", err)
	}
	pID, _ := resP.LastInsertId()

	// 3. Criar item com nível de sensibilidade alto
	resI, err := st.db.Exec(`INSERT INTO material_itens (grupo_id, nome, codigo_patrimonio, status, nivel_sensibilidade)
		VALUES (?, 'Rádio Tático VHF', 'RAD-999', 'acautelado', 'sensivel')`, gID)
	if err != nil {
		t.Fatalf("criar item: %v", err)
	}
	iID, _ := resI.LastInsertId()

	// 4. Forjar cautela ativa com saída há 72 horas atrás
	dataAntiga := time.Now().UTC().Add(-72 * time.Hour).Format(time.RFC3339)
	_, err = st.db.Exec(`INSERT INTO material_cautelas (item_id, pessoa_id, responsavel_entrega_id, data_saida, status)
		VALUES (?, ?, 1, ?, 'ativa')`, iID, pID, dataAntiga)
	if err != nil {
		t.Fatalf("forjar cautela: %v", err)
	}

	// 5. Executar requisição GET /api/notificacoes
	req := httptest.NewRequest("GET", "/api/notificacoes", nil)
	req.AddCookie(adminCookie)
	w := httptest.NewRecorder()
	app.mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("esperado 200 em /api/notificacoes, obtido %d: %s", w.Code, w.Body.String())
	}

	var resp struct {
		TotalAtrasadas    int `json:"total_atrasadas"`
		PrazoHoras        int `json:"prazo_horas"`
		CautelasAtrasadas []struct {
			CautelaID          int64   `json:"cautela_id"`
			ItemNome           string  `json:"item_nome"`
			CodigoPatrimonio   string  `json:"codigo_patrimonio"`
			PessoaNomeGuerra   string  `json:"pessoa_nome_guerra"`
			NivelSensibilidade string  `json:"nivel_sensibilidade"`
			HorasEmUso         float64 `json:"horas_em_uso"`
		} `json:"cautelas_atrasadas"`
	}

	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decodificar resposta: %v", err)
	}

	if resp.TotalAtrasadas < 1 {
		t.Errorf("esperado ao menos 1 cautela atrasada, obtido: %d", resp.TotalAtrasadas)
	}
	if len(resp.CautelasAtrasadas) > 0 {
		c := resp.CautelasAtrasadas[0]
		if c.CodigoPatrimonio != "RAD-999" || c.NivelSensibilidade != "sensivel" {
			t.Errorf("dados incorretos da cautela atrasada: %+v", c)
		}
		if c.HorasEmUso < 70 {
			t.Errorf("esperado horas em uso > 70h, obtido: %.1f", c.HorasEmUso)
		}
	}
}

// TestExportacaoGranular: valida extração dual-mode (JSON e SQLite) com filtros finos.
func TestExportacaoGranular(t *testing.T) {
	app, st, cleanup := setupTestApp(t)
	defer cleanup()

	adminCookie := loginAs(t, app, "admin", "admin123")

	resG, _ := st.db.Exec(`INSERT INTO grupos (nome, codigo) VALUES ('1ª Cia Export', 'GEX123')`)
	gID, _ := resG.LastInsertId()

	// Inserir conferências de teste
	if _, err := st.db.Exec(`INSERT INTO conferencias (grupo_id, data, tipo_id, status, criado_por) VALUES (?, '2026-05-10', 1, 'fechada', 1)`, gID); err != nil {
		t.Fatalf("erro ao inserir conf 2026: %v", err)
	}
	if _, err := st.db.Exec(`INSERT INTO conferencias (grupo_id, data, tipo_id, status, criado_por) VALUES (?, '2025-11-20', 1, 'fechada', 1)`, gID); err != nil {
		t.Fatalf("erro ao inserir conf 2025: %v", err)
	}

	// 1. Exportação JSON com filtro de ano 2026
	reqJSON := httptest.NewRequest("GET", "/api/export?tipo=json&ano=2026", nil)
	reqJSON.AddCookie(adminCookie)
	wJSON := httptest.NewRecorder()
	app.mux.ServeHTTP(wJSON, reqJSON)

	if wJSON.Code != http.StatusOK {
		t.Fatalf("esperado 200 no export JSON, obtido %d: %s", wJSON.Code, wJSON.Body.String())
	}
	var dataJSON map[string]any
	if err := json.Unmarshal(wJSON.Body.Bytes(), &dataJSON); err != nil {
		t.Fatalf("json inválido retornado pelo export: %v", err)
	}
	confs, ok := dataJSON["conferencias"].([]any)
	if !ok || len(confs) != 1 {
		t.Errorf("esperado exatamente 1 conferência do ano 2026, obtido: %d", len(confs))
	}

	// 2. Exportação SQLite com filtro de tabela
	reqSQL := httptest.NewRequest("GET", "/api/export?tipo=sqlite&tabela=conferencias&ano=2026", nil)
	reqSQL.AddCookie(adminCookie)
	wSQL := httptest.NewRecorder()
	app.mux.ServeHTTP(wSQL, reqSQL)

	if wSQL.Code != http.StatusOK {
		t.Fatalf("esperado 200 no export SQLite, obtido %d: %s", wSQL.Code, wSQL.Body.String())
	}
	rawBytes := wSQL.Body.Bytes()
	if len(rawBytes) < 512 {
		t.Fatalf("arquivo SQLite gerado muito pequeno: %d bytes", len(rawBytes))
	}
	// Cabeçalho de arquivo SQLite3 padrão
	if string(rawBytes[:15]) != "SQLite format 3" {
		t.Fatalf("arquivo exportado não possui assinatura SQLite válida")
	}

	// Testar leitura da base exportada
	tempF, _ := os.CreateTemp("", "test_export_read_*.db")
	tempF.Write(rawBytes)
	tempF.Close()
	defer os.Remove(tempF.Name())

	dbCheck, err := sql.Open("sqlite", tempF.Name())
	if err != nil {
		t.Fatalf("abrir banco exportado: %v", err)
	}
	defer dbCheck.Close()

	var totalConf int
	_ = dbCheck.QueryRow(`SELECT COUNT(*) FROM conferencias`).Scan(&totalConf)
	if totalConf != 1 {
		t.Errorf("esperado 1 conferência no banco exportado, obtido: %d", totalConf)
	}
}

// TestStressVolumeDB: teste de alta carga com inserção massiva em lote e concorrência no SQLite WAL.
func TestStressVolumeDB(t *testing.T) {
	if testing.Short() {
		t.Skip("pulando teste de stress em modo curto")
	}
	_, st, cleanup := setupTestApp(t)
	defer cleanup()

	resG, _ := st.db.Exec(`INSERT INTO grupos (nome, codigo) VALUES ('1ª Cia Stress', 'GSTR01')`)
	gID, _ := resG.LastInsertId()

	// Inserir 2.000 pessoas em lote via transação única
	tx, err := st.db.Begin()
	if err != nil {
		t.Fatalf("iniciar transação: %v", err)
	}
	stmt, err := tx.Prepare(`INSERT INTO pessoas (nome_guerra, nome_completo, grupo_id) VALUES (?, ?, ?)`)
	if err != nil {
		t.Fatalf("prepare: %v", err)
	}
	defer stmt.Close()

	numPessoas := 2000
	for i := 1; i <= numPessoas; i++ {
		ng := fmt.Sprintf("MILITAR_%04d", i)
		nc := fmt.Sprintf("Soldado de Teste Concorrente %04d", i)
		if _, err := stmt.Exec(ng, nc, gID); err != nil {
			tx.Rollback()
			t.Fatalf("inserir militar %d: %v", i, err)
		}
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("commit lote pessoas: %v", err)
	}

	// Concorrência: 20 goroutines executando leituras e gravações simultâneas em WAL mode
	var wg sync.WaitGroup
	workers := 20
	repeticoes := 15

	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			for r := 0; r < repeticoes; r++ {
				// 1. Leitura
				var count int
				err := st.db.QueryRow(`SELECT COUNT(*) FROM pessoas WHERE grupo_id = ?`, gID).Scan(&count)
				if err != nil {
					t.Errorf("worker %d leitura erro: %v", workerID, err)
				}
				// 2. Escrita pontual de log ou auditoria
				st.Auditoria(nil, "stress_test_ping", "teste", nil, fmt.Sprintf("w=%d r=%d", workerID, r), "127.0.0.1")
			}
		}(w)
	}

	wg.Wait()

	var auditCount int
	_ = st.db.QueryRow(`SELECT COUNT(*) FROM auditoria WHERE acao = 'stress_test_ping'`).Scan(&auditCount)
	esperadoAudit := workers * repeticoes
	if auditCount != esperadoAudit {
		t.Errorf("esperado %d logs de auditoria concorrentes, obtido: %d", esperadoAudit, auditCount)
	}
}
