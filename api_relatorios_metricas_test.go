package main

// api_relatorios_metricas_test.go — F4 (DELIB-0008): fecha 2 lacunas P1 do
// inventário F1 que a F2 não cobriu:
//   - GET /api/admin/sistema/metricas (auth(true) → hAdminSistemaMetricas):
//     caminho feliz com todos os campos numéricos + 401 sem sessão + 403 papel comum;
//   - relatórios de efetivo e export: GET /api/efetivo_atual (caminho feliz com
//     estados herdados de conferência fechada; escopo por grupo) e GET /api/export/{t}
//     (pessoas/presencas/conferencias com BOM UTF-8 e cabeçalho CSV), sempre com 401.
//
// Padrão da casa: setupTestApp in-process, doJSONReq/loginAs dos helpers da F2.

import (
	"encoding/csv"
	"net/http"
	"strings"
	"testing"
	"time"
)

// semeiaGrupoRelatorio: grupo + 2 pessoas + gerente com grupo + conferência FECHADA
// com 1 lançamento — estado mínimo p/ efetivo_atual e export de presenças.
func semeiaGrupoRelatorio(t *testing.T) (app *App, st *Store, cleanup func(), gid, p1, p2 int64, tokGer, tokAdmin *http.Cookie) {
	t.Helper()
	app, st, cleanup = setupTestApp(t)

	resG, err := st.db.Exec(`INSERT INTO grupos (nome, codigo) VALUES ('Cia Relatórios', 'CR0001')`)
	if err != nil {
		t.Fatalf("criar grupo: %v", err)
	}
	gid, _ = resG.LastInsertId()

	resP1, err := st.db.Exec(`INSERT INTO pessoas (grupo_id, nome_guerra, nome_completo, status) VALUES (?, 'Azevedo', 'Azevedo Alves', 'ativo')`, gid)
	if err != nil {
		t.Fatalf("criar pessoa 1: %v", err)
	}
	p1, _ = resP1.LastInsertId()

	resP2, err := st.db.Exec(`INSERT INTO pessoas (grupo_id, nome_guerra, nome_completo, status) VALUES (?, 'Barros', 'Barros Nunes', 'ativo')`, gid)
	if err != nil {
		t.Fatalf("criar pessoa 2: %v", err)
	}
	p2, _ = resP2.LastInsertId()

	criaUsuarioTeste(t, st, "ger_rel", "senha-gerente", "gerente")
	if _, err := st.db.Exec(`UPDATE usuarios SET grupo_id = ? WHERE login = 'ger_rel'`, gid); err != nil {
		t.Fatalf("grupo do gerente: %v", err)
	}
	if _, err := st.db.Exec(`INSERT OR IGNORE INTO usuario_papeis (usuario_id, grupo_id, papel)
		SELECT id, grupo_id, papel FROM usuarios WHERE login = 'ger_rel'`); err != nil {
		t.Fatalf("papel do gerente: %v", err)
	}
	tokGer = loginAs(t, app, "ger_rel", "senha-gerente")

	// conferência fechada com lançamentos de ontem (efetivo_atual só conta FECHADA)
	tipoID := int64(0)
	if err := st.db.QueryRow(`SELECT id FROM conferencia_tipos ORDER BY id LIMIT 1`).Scan(&tipoID); err != nil {
		t.Fatalf("tipo de conferência no seed: %v", err)
	}
	ontem := time.Now().In(app.horaLocal).AddDate(0, 0, -1).Format("2006-01-02")
	resC, err := st.db.Exec(`INSERT INTO conferencias (data, tipo_id, local, grupo_id, status, criado_por, criado_em, fechada_em)
		VALUES (?, ?, 'Pátio', ?, 'fechada', (SELECT id FROM usuarios WHERE login='ger_rel'), ?, ?)`,
		ontem, tipoID, gid, time.Now().UTC().Format("2006-01-02T15:04:05.000Z"), time.Now().UTC().Format("2006-01-02T15:04:05.000Z"))
	if err != nil {
		t.Fatalf("criar conferência fechada: %v", err)
	}
	confID, _ := resC.LastInsertId()
	for _, lp := range []struct {
		id  int64
		sit string
	}{
		{p1, "presente"},
		{p2, "falta"},
	} {
		if _, err := st.db.Exec(`INSERT OR IGNORE INTO presencas (conferencia_id, pessoa_id, situacao, marcado_por, marcado_em)
			VALUES (?, ?, ?, (SELECT id FROM usuarios WHERE login='ger_rel'), ?)`,
			confID, lp.id, lp.sit, time.Now().UTC().Format("2006-01-02T15:04:05.000Z")); err != nil {
			t.Fatalf("lançar presença %d: %v", lp.id, err)
		}
	}

	tokAdmin = loginAs(t, app, "admin", "admin123")
	return app, st, cleanup, gid, p1, p2, tokGer, tokAdmin
}

// ---------- métricas admin (P1) ----------

func TestMetricasAdminCamposFeliz(t *testing.T) {
	app, _, cleanup, _, _, _, _, tokAdmin := semeiaGrupoRelatorio(t)
	defer cleanup()

	rr, res := doJSONReq(app, "GET", "/api/admin/sistema/metricas", nil, tokAdmin)
	if rr.Code != http.StatusOK {
		t.Fatalf("metricas: status %d (%s)", rr.Code, rr.Body.String())
	}
	// contrato: todos os campos numéricos do MetricasSistema presentes e ≥ 0
	numerico := map[string]bool{
		"cpu_percent": true, "num_cpu": true, "goroutines": true,
		"ram_processo_mb": true, "ram_sistema_mb": true, "ram_heap_mb": true,
		"banco_bytes": true, "banco_mb": true, "dados_bytes": true, "dados_mb": true,
		"disco_total_gb": true, "disco_livre_gb": true, "disco_usado_pct": true,
	}
	for campo := range numerico {
		v, ok := res[campo]
		if !ok {
			t.Errorf("metricas sem campo %q", campo)
			continue
		}
		n, ok := v.(float64)
		if !ok || n < 0 {
			t.Errorf("campo %q não é numérico ≥ 0: %v", campo, v)
		}
	}
	if _, ok := res["timestamp"].(string); !ok {
		t.Errorf("metricas sem timestamp string")
	}
	if n := res["num_cpu"].(float64); n < 1 {
		t.Errorf("num_cpu < 1: %v", n)
	}
	if goroutines := res["goroutines"].(float64); goroutines < 1 {
		t.Errorf("goroutines < 1: %v", goroutines)
	}
	// banco semeado tem bytes > 0
	if banco := res["banco_bytes"].(float64); banco <= 0 {
		t.Errorf("banco_bytes deveria ser > 0 com banco semeado: %v", banco)
	}
}

func TestMetricasAdmin401SemSessao(t *testing.T) {
	app, _, cleanup, _, _, _, _, _ := semeiaGrupoRelatorio(t)
	defer cleanup()

	rr, _ := doJSONReq(app, "GET", "/api/admin/sistema/metricas", nil, nil)
	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("metricas sem sessão: esperado 401, obtido %d (%s)", rr.Code, rr.Body.String())
	}
}

func TestMetricasAdmin403PapelComum(t *testing.T) {
	app, st, cleanup, gid, _, _, tokGer, _ := semeiaGrupoRelatorio(t)
	defer cleanup()

	// operador do grupo (login com papel já vinculado no banco)
	criaUsuarioTeste(t, st, "op_rel", "senha-operador", "operador")
	if _, err := st.db.Exec(`UPDATE usuarios SET grupo_id = ? WHERE login = 'op_rel'`, gid); err != nil {
		t.Fatalf("grupo do operador: %v", err)
	}
	if _, err := st.db.Exec(`INSERT OR IGNORE INTO usuario_papeis (usuario_id, grupo_id, papel)
		SELECT id, grupo_id, papel FROM usuarios WHERE login = 'op_rel'`); err != nil {
		t.Fatalf("papel do operador: %v", err)
	}
	tokOp := loginAs(t, app, "op_rel", "senha-operador")

	rr, _ := doJSONReq(app, "GET", "/api/admin/sistema/metricas", nil, tokOp)
	if rr.Code != http.StatusForbidden {
		t.Fatalf("metricas com operador: esperado 403, obtido %d (%s)", rr.Code, rr.Body.String())
	}
	_ = tokGer
}

// ---------- efetivo_atual (P1) ----------

func TestEfetivoAtualCaminhoFeliz(t *testing.T) {
	app, _, cleanup, _, _, _, tokGer, _ := semeiaGrupoRelatorio(t)
	defer cleanup()

	rr, res := doJSONReq(app, "GET", "/api/efetivo_atual", nil, tokGer)
	if rr.Code != http.StatusOK {
		t.Fatalf("efetivo_atual: status %d (%s)", rr.Code, rr.Body.String())
	}
	pessoas, ok := res["pessoas"].([]any)
	if !ok {
		t.Fatalf("efetivo_atual sem lista de pessoas: %v", res)
	}
	if len(pessoas) != 2 {
		t.Fatalf("esperado 2 pessoas no escopo do grupo, obtido %d", len(pessoas))
	}
	if _, ok := res["hoje"].(string); !ok {
		t.Errorf("efetivo_atual sem campo hoje (data)")
	}
	// estados herdados da conferência fechada de ontem
	sits := map[string]bool{}
	for _, p := range pessoas {
		m, _ := p.(map[string]any)
		if m["situacao"] != nil {
			sits[m["situacao"].(string)] = true
			if m["ultima_data"] == nil {
				t.Errorf("pessoa com situacao sem ultima_data: %v", m)
			}
		}
	}
	if !sits["presente"] || !sits["falta"] {
		t.Errorf("estados herdados errados: %v (esperado presente+falta)", sits)
	}
}

func TestEfetivoAtual401SemSessao(t *testing.T) {
	app, _, cleanup, _, _, _, _, _ := semeiaGrupoRelatorio(t)
	defer cleanup()

	rr, _ := doJSONReq(app, "GET", "/api/efetivo_atual", nil, nil)
	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("efetivo_atual sem sessão: esperado 401, obtido %d", rr.Code)
	}
}

// ---------- export CSV (P1) ----------

func TestExportCSVCaminhoFeliz(t *testing.T) {
	app, _, cleanup, _, _, _, tokGer, _ := semeiaGrupoRelatorio(t)
	defer cleanup()

	casos := []struct {
		t       string
		cab     []string
		linhas  int
		contem  string // substring que tem que aparecer em alguma linha de dados
	}{
		{t: "pessoas", cab: []string{"id", "nome_guerra", "nome_completo", "setor", "funcao", "status", "criado_em"}, linhas: 2, contem: "Azevedo"},
		{t: "presencas", cab: []string{"data", "conferencia_tipo", "nome_guerra", "situacao", "destino", "marcado_em"}, linhas: 2, contem: "falta"},
		{t: "conferencias", cab: []string{"data", "hora", "tipo", "local", "status"}, linhas: 1, contem: "fechada"},
	}
	for _, c := range casos {
		rr, _ := doJSONReq(app, "GET", "/api/export/"+c.t, nil, tokGer)
		if rr.Code != http.StatusOK {
			t.Fatalf("export/%s: status %d (%s)", c.t, rr.Code, rr.Body.String())
		}
		if ct := rr.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/csv") {
			t.Errorf("export/%s: Content-Type %q", c.t, ct)
		}
		if cd := rr.Header().Get("Content-Disposition"); !strings.Contains(cd, "sci_"+c.t+".csv") {
			t.Errorf("export/%s: Content-Disposition %q", c.t, cd)
		}
		corpo := rr.Body.String()
		if !strings.HasPrefix(corpo, "\ufeff") {
			t.Errorf("export/%s: sem BOM UTF-8", c.t)
		}
		recs, err := csv.NewReader(strings.NewReader(strings.TrimPrefix(corpo, "\ufeff"))).ReadAll()
		if err != nil {
			t.Fatalf("export/%s: CSV malformado: %v", c.t, err)
		}
		if len(recs) != 1+c.linhas { // cabeçalho + dados
			t.Fatalf("export/%s: esperado %d linhas (1 cab + %d dados), obtido %d", c.t, 1+c.linhas, c.linhas, len(recs))
		}
		for i, want := range c.cab {
			if recs[0][i] != want {
				t.Errorf("export/%s: cabeçalho col %d = %q, esperado %q", c.t, i, recs[0][i], want)
			}
		}
		achou := false
		for _, linha := range recs[1:] {
			if strings.Join(linha, ";") != "" && strings.Contains(strings.Join(linha, ";"), c.contem) {
				achou = true
			}
		}
		if !achou {
			t.Errorf("export/%s: dados não contêm %q: %v", c.t, c.contem, recs[1:])
		}
	}
}

func TestExportCSVEntidadeInvalida(t *testing.T) {
	app, _, cleanup, _, _, _, tokGer, _ := semeiaGrupoRelatorio(t)
	defer cleanup()

	// contrato atual: entidade desconhecida NÃO é 400/404 — devolve CSV com linha de erro
	rr, _ := doJSONReq(app, "GET", "/api/export/inexistente", nil, tokGer)
	if rr.Code != http.StatusOK {
		t.Fatalf("export inexistente: status %d", rr.Code)
	}
	corpo := rr.Body.String()
	if !strings.Contains(corpo, "erro") || !strings.Contains(corpo, "entidade inválida") {
		t.Errorf("export inexistente sem linha de erro: %q", corpo)
	}
}

func TestExportCSV401SemSessao(t *testing.T) {
	app, _, cleanup, _, _, _, _, _ := semeiaGrupoRelatorio(t)
	defer cleanup()

	for _, caminho := range []string{"/api/export/pessoas", "/api/export/presencas", "/api/export/conferencias"} {
		rr, _ := doJSONReq(app, "GET", caminho, nil, nil)
		if rr.Code != http.StatusUnauthorized {
			t.Errorf("%s sem sessão: esperado 401, obtido %d", caminho, rr.Code)
		}
	}
}
