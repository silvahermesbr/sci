package main

// Regressão: destino órfão no PDF de conferência + filtros do relatório.

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// monta grupo+gerente+conferência aberta e devolve o que os testes precisam
func setupConfDestino(t *testing.T) (app *App, gerCookie *http.Cookie, confID int64, pessoaID int64, destinoID int64) {
	t.Helper()
	app, _, cleanup := setupTestApp(t)
	t.Cleanup(cleanup)
	admin := loginAs(t, app, "admin", "admin123")
	_, resG := doJSONReq(app, "POST", "/api/grupos", map[string]any{
		"nome": "G Dest", "login": "ger_dest", "senha": "senha12345", "nome_guerra": "GerDest",
	}, admin)
	if rr := resG["id"]; rr == nil {
		t.Fatalf("grupo: %v", resG)
	}
	grupoID := int64(resG["id"].(float64))
	ger := loginAs(t, app, "ger_dest", "senha12345")
	// pessoa sintética no grupo (para o lançamento)
	var pesID int64
	if err := app.st.db.QueryRow(`SELECT id FROM pessoas WHERE grupo_id = ? LIMIT 1`, grupoID).Scan(&pesID); err != nil {
		resP, errIns := app.st.db.Exec(`INSERT INTO pessoas (nome_guerra, nome_completo, grupo_id, status) VALUES ('PDEST', 'Pessoa Dest', ?, 'ativo')`, grupoID)
		if errIns != nil {
			t.Fatalf("inserir pessoa: %v", errIns)
		}
		pesID, _ = resP.LastInsertId()
	}
	// catálogo de destino criado pelo gerente
	rrD, resD := doJSONReq(app, "POST", "/api/catalogo/destinos", map[string]any{"nome": "Hospital"}, ger)
	if rrD.Code != http.StatusOK {
		t.Fatalf("destino: %v", resD)
	}
	destID := int64(resD["id"].(float64))
	// conferência aberta
	rrC, resC := doJSONReq(app, "POST", "/api/conferencia/iniciar", map[string]any{"local": "QG"}, ger)
	if rrC.Code != http.StatusOK {
		t.Fatalf("iniciar: %v", resC)
	}
	confID = int64(resC["id"].(float64))
	return app, ger, confID, pesID, destID
}

func marcar(app *App, cookie *http.Cookie, confID, pessoaID int64, sit string, dest *int64) *httptest.ResponseRecorder {
	body := map[string]any{"pessoa_id": pessoaID, "situacao": sit}
	if dest != nil {
		body["destino_id"] = *dest
	}
	rr, _ := doJSONReq(app, "POST", "/api/conferencia/marcar?id="+int64ToStr(confID), body, cookie)
	return rr
}

func buscarPresenca(t *testing.T, app *App, confID, pessoaID int64) (sit string, dest *int64) {
	t.Helper()
	var d any
	err := app.st.db.QueryRow(`SELECT situacao, destino_id FROM presencas WHERE conferencia_id = ? AND pessoa_id = ?`,
		confID, pessoaID).Scan(&sit, &d)
	if err != nil {
		t.Fatalf("ler presenca: %v", err)
	}
	if dd, ok := d.(int64); ok {
		dest = &dd
	}
	return sit, dest
}

func int64ToStr(n int64) string {
	if n == 0 {
		return "0"
	}
	var b [20]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	return string(b[i:])
}

func TestDestinoJustificadaParaPresenteSome(t *testing.T) {
	app, ger, confID, pesID, destID := setupConfDestino(t)
	// 1) justificada COM destino → grava os dois
	if rr := marcar(app, ger, confID, pesID, "justificada", &destID); rr.Code != http.StatusOK {
		t.Fatalf("justificada: %d", rr.Code)
	}
	sit, dest := buscarPresenca(t, app, confID, pesID)
	if sit != "justificada" || dest == nil || *dest != destID {
		t.Fatalf("passo 1 falhou: sit=%s dest=%v", sit, dest)
	}
	// 2) troca para PRESENTE (front reenvia destino) → destino DEVE ser anulado
	if rr := marcar(app, ger, confID, pesID, "presente", &destID); rr.Code != http.StatusOK {
		t.Fatalf("presente: %d", rr.Code)
	}
	sit, dest = buscarPresenca(t, app, confID, pesID)
	if sit != "presente" {
		t.Fatalf("situação: %s", sit)
	}
	if dest != nil {
		t.Fatalf("BUG VIVO: presente carregando destino %d", *dest)
	}
}

func TestAtrasoMantemDestino(t *testing.T) {
	app, ger, confID, pesID, destID := setupConfDestino(t)
	if rr := marcar(app, ger, confID, pesID, "atraso", &destID); rr.Code != http.StatusOK {
		t.Fatalf("atraso: %d", rr.Code)
	}
	sit, dest := buscarPresenca(t, app, confID, pesID)
	if sit != "atraso" || dest == nil || *dest != destID {
		t.Fatalf("atraso deveria manter destino: sit=%s dest=%v", sit, dest)
	}
}

func TestFaltaEVivoSemDestino(t *testing.T) {
	app, ger, confID, pesID, destID := setupConfDestino(t)
	if rr := marcar(app, ger, confID, pesID, "falta", &destID); rr.Code != http.StatusOK {
		t.Fatalf("falta: %d", rr.Code)
	}
	sit, dest := buscarPresenca(t, app, confID, pesID)
	if sit != "falta" || dest != nil {
		t.Fatalf("falta deveria nascer sem destino: sit=%s dest=%v", sit, dest)
	}
}

func TestBundleNaoReportaDestinoOorfo(t *testing.T) {
	app, ger, confID, pesID, destID := setupConfDestino(t)
	// grava resíduo COMO ESTÁ HOJE (simula conferência antiga): presente com destino,
	// via SQL direto — escrita nova já impediria, mas fechadas imutáveis têm o resíduo.
	if rr := marcar(app, ger, confID, pesID, "justificada", &destID); rr.Code != http.StatusOK {
		t.Fatalf("setup: %d", rr.Code)
	}
	if _, err := app.st.db.Exec(`UPDATE presencas SET situacao='presente' WHERE conferencia_id=? AND pessoa_id=?`, confID, pesID); err != nil {
		t.Fatalf("residuo: %v", err)
	}
	// fecha a conferência diretamente no banco (simula conferência fechada antiga)
	if _, err := app.st.db.Exec(`UPDATE conferencias SET status='fechada', fechada_em=strftime('%Y-%m-%dT%H:%M:%fZ','now') WHERE id=?`, confID); err != nil {
		t.Fatalf("fechar: %v", err)
	}
	lanc, _, err := app.montarLancamentosPDFConferencia(confID, "")
	if err != nil {
		t.Fatalf("montar: %v", err)
	}
	for _, l := range lanc {
		if l["situacao"] == "presente" && l["destino"] != "" {
			t.Fatalf("BUG LEITURA VIVO: presente com destino %q", l["destino"])
		}
	}
}

func TestPDFConferenciaFiltroFaltas(t *testing.T) {
	app, ger, confID, pesID, _ := setupConfDestino(t)
	// cria 2ª pessoa para ter falta e presente na mesma conferência
	resP2, errP2 := app.st.db.Exec(`INSERT INTO pessoas (nome_guerra, nome_completo, grupo_id, status) SELECT 'PDEST2','Pessoa Dest 2', grupo_id, 'ativo' FROM pessoas WHERE id = ?`, pesID)
	if errP2 != nil {
		t.Fatalf("inserir pessoa 2: %v", errP2)
	}
	pes2, _ := resP2.LastInsertId()
	marcar(app, ger, confID, pesID, "falta", nil)
	marcar(app, ger, confID, pes2, "presente", nil)
	if _, err := app.st.db.Exec(`UPDATE conferencias SET status='fechada', fechada_em=strftime('%Y-%m-%dT%H:%M:%fZ','now') WHERE id=?`, confID); err != nil {
		t.Fatalf("fechar: %v", err)
	}
	// PDF filtrado: HTTP 200 e content-type PDF
	rrPdf, _ := doRawReq(app, "GET", "/api/conferencia/"+int64ToStr(confID)+"/relatorio.pdf?filtro=faltas", nil, ger)
	if rrPdf.Code != http.StatusOK {
		t.Fatalf("pdf filtrado: %d", rrPdf.Code)
	}
	if ct := rrPdf.Header().Get("Content-Type"); !strings.Contains(ct, "application/pdf") {
		t.Fatalf("content-type: %s", ct)
	}
	if !strings.Contains(rrPdf.Header().Get("Content-Disposition"), "_SO_FALTAS") {
		t.Fatalf("selo de filtro ausente no filename: %s", rrPdf.Header().Get("Content-Disposition"))
	}
}


