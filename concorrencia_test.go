package main

// concorrencia_test.go — F4 (DELIB-0008): corrida de requests simultâneos contra
// o App in-process (padrão setupTestApp da F2), com -race ligado no gate.
//
// Cenários exigidos pela fatia:
//  1. marcação simultânea — MESMA pessoa marcada por 2 gerentes ao mesmo tempo
//     (upsert de presencas não pode duplicar linha nem corromper estado);
//  2. fechar duplo — 2 POST /api/conferencia/fechar concorrentes: um grava,
//     o outro DEVE levar 409 ("conferência já está fechada"); nunca 2 fechamentos;
//  3. backup concorrente — N POST /api/backup ao mesmo tempo (nomes únicos, sha256
//     válido) + fechamento que dispara backupAssincrono (goroutine × VACUUM).
//
// Observação de arquitetura: o SQLite roda com MaxOpenConns(1) + busy_timeout,
// então a escrita é serializada no banco — o que -race expõe aqui é corrida de
// memória no App (mux, limiter, sessões, handlers, camada de backup em goroutine).

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

// doJSONAsync: dispara N requests simultâneos com o MESMO corpo/cookie e espera
// todos terminarem. Recorders ficam na ordem de disparo (índice = goroutine).
func doJSONAsync(app *App, n int, method, path string, body any, cookie *http.Cookie) []*httptest.ResponseRecorder {
	recs := make([]*httptest.ResponseRecorder, n)
	var wg sync.WaitGroup
	start := make(chan struct{}) // trava de largada: todos os requests nascem juntos
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start
			recs[i], _ = doJSONReq(app, method, path, body, cookie)
		}(i)
	}
	close(start)
	wg.Wait()
	return recs
}

// vinculaGrupoDoLogin: gerente/operador de teste criado por criaUsuarioTeste nasce
// SEM grupo. ORDEM OBRIGATÓRIA: 1) vincula grupo no banco (usuarios + usuario_papeis,
// DELETE+INSERT porque UNIQUE(usuario_id,grupo_id,papel) + índice parcial de gerente
// único trai UPDATE NULL→gid); 2) SÓ ENTÃO loga — sessoes.papel_ativo_id referencia
// usuario_papeis(id) ON DELETE CASCADE com foreign_keys=1 no DSN: apagar/recriar o
// papel DEPOIS do login apaga a linha pai e a SESSÃO INTEIRA morre junto (probe:
// sessoes=0, /api/me → 401 "sessão expirada").
func vinculaGrupoDoLogin(t *testing.T, app *App, st *Store, login string, gid int64) *http.Cookie {
	t.Helper()
	if _, err := st.db.Exec(`UPDATE usuarios SET grupo_id = ? WHERE login = ?`, gid, login); err != nil {
		t.Fatalf("grupo do usuário %s: %v", login, err)
	}
	var uid int64
	if err := st.db.QueryRow(`SELECT id FROM usuarios WHERE login = ?`, login).Scan(&uid); err != nil {
		t.Fatalf("usuário %s: %v", login, err)
	}
	if _, err := st.db.Exec(`DELETE FROM usuario_papeis WHERE usuario_id = ? AND grupo_id IS NULL`, uid); err != nil {
		t.Fatalf("remover papel antigo de %s: %v", login, err)
	}
	if _, err := st.db.Exec(`INSERT INTO usuario_papeis (usuario_id, grupo_id, papel)
		SELECT id, ?, papel FROM usuarios WHERE id = ?`, gid, uid); err != nil {
		t.Fatalf("vincular papel de %s ao grupo: %v", login, err)
	}
	// login POR ÚLTIMO: a sessão nasce já apontando para o papel com grupo
	return loginAs(t, app, login, "senha-gerente")
}

// preparaConferenciaAberta: app de teste com grupo, 2 pessoas, gerente logado com
// grupo vinculado e 1 conferência ABERTA criada direto no banco (tipo vem do seed).
func preparaConferenciaAberta(t *testing.T) (app *App, st *Store, cleanup func(), gid, p1, p2 int64, tokGer *http.Cookie) {
	t.Helper()
	app, st, cleanup = setupTestApp(t)

	resG, err := st.db.Exec(`INSERT INTO grupos (nome, codigo) VALUES ('Cia Concorrência', 'CC0001')`)
	if err != nil {
		t.Fatalf("criar grupo: %v", err)
	}
	gid, _ = resG.LastInsertId()

	resP1, err := st.db.Exec(`INSERT INTO pessoas (grupo_id, nome_guerra, nome_completo, status) VALUES (?, 'Alfa', 'Alfa da Silva', 'ativo')`, gid)
	if err != nil {
		t.Fatalf("criar pessoa 1: %v", err)
	}
	p1, _ = resP1.LastInsertId()

	resP2, err := st.db.Exec(`INSERT INTO pessoas (grupo_id, nome_guerra, nome_completo, status) VALUES (?, 'Bravo', 'Bravo Souza', 'ativo')`, gid)
	if err != nil {
		t.Fatalf("criar pessoa 2: %v", err)
	}
	p2, _ = resP2.LastInsertId()

	criaUsuarioTeste(t, st, "ger_conc_a", "senha-gerente", "gerente")
	tokGer = vinculaGrupoDoLogin(t, app, st, "ger_conc_a", gid)

	tipoID := int64(0)
	if err := st.db.QueryRow(`SELECT id FROM conferencia_tipos ORDER BY id LIMIT 1`).Scan(&tipoID); err != nil {
		t.Fatalf("tipo de conferência no seed: %v", err)
	}
	hoje := time.Now().In(app.horaLocal).Format("2006-01-02")
	resC, err := st.db.Exec(`INSERT INTO conferencias (data, tipo_id, local, grupo_id, criado_por, criado_em) VALUES (?, ?, 'Pátio', ?, ?, ?)`,
		hoje, tipoID, gid, usuarioID(t, st, "ger_conc_a"), time.Now().UTC().Format("2006-01-02T15:04:05.000Z"))
	if err != nil {
		t.Fatalf("criar conferência aberta: %v", err)
	}
	confID, _ := resC.LastInsertId()
	_ = confID // os testes leem a conferência aberta pelo status
	return app, st, cleanup, gid, p1, p2, tokGer
}

// Cenário 1 — mesma pessoa, 2 marcações simultâneas (gerentes distintos):
// exatamente 1 linha em presencas, estado final ∈ {presente, atraso}.
func TestConcorrenciaMarcacaoSimultaneaMesmaPessoa(t *testing.T) {
	app, st, cleanup, _, p1, _, tok := preparaConferenciaAberta(t)
	defer cleanup()

	criaUsuarioTeste(t, st, "ger_conc_b", "senha-gerente", "operador")
	tokB := vinculaGrupoDoLogin(t, app, st, "ger_conc_b", gidDe(t, st))

	var wg sync.WaitGroup
	chA := make(chan *httptest.ResponseRecorder, 1)
	chB := make(chan *httptest.ResponseRecorder, 1)
	start := make(chan struct{})
	wg.Add(2)
	go func() { // gerente A marca PRESENTE
		defer wg.Done()
		<-start
		rr, _ := doJSONReq(app, "POST", "/api/conferencia/marcar", map[string]any{"pessoa_id": p1, "situacao": "presente"}, tok)
		chA <- rr
	}()
	go func() { // gerente B marca ATRASO na MESMA pessoa no mesmo instante
		defer wg.Done()
		<-start
		rr, _ := doJSONReq(app, "POST", "/api/conferencia/marcar", map[string]any{"pessoa_id": p1, "situacao": "atraso"}, tokB)
		chB <- rr
	}()
	close(start)
	wg.Wait()
	ra, rb := <-chA, <-chB

	if ra.Code != http.StatusOK {
		t.Fatalf("marcação gerente A: %d (%s)", ra.Code, ra.Body.String())
	}
	if rb.Code != http.StatusOK {
		t.Fatalf("marcação gerente B: %d (%s)", rb.Code, rb.Body.String())
	}

	// invariante forte: UMA linha por (conferência, pessoa) — o upsert não pode duplicar
	var confID int64
	if err := st.db.QueryRow(`SELECT id FROM conferencias WHERE status='aberta'`).Scan(&confID); err != nil {
		t.Fatalf("conferência aberta: %v", err)
	}
	var n, situacaoDistinct int
	var situacao string
	if err := st.db.QueryRow(`SELECT COUNT(*), COUNT(DISTINCT situacao), COALESCE(MAX(situacao),'') FROM presencas WHERE conferencia_id=? AND pessoa_id=?`, confID, p1).
		Scan(&n, &situacaoDistinct, &situacao); err != nil {
		t.Fatalf("ler presencas: %v", err)
	}
	if n != 1 {
		t.Fatalf("linha duplicada: esperado 1 lançamento para (conf,pessoa), obtido %d", n)
	}
	if situacaoDistinct != 1 || (situacao != "presente" && situacao != "atraso") {
		t.Fatalf("estado final incoerente: situacao=%q (distinct=%d)", situacao, situacaoDistinct)
	}
}

// gidDe: id do grupo padrão destes testes (evita ler LastInsertId em closures).
func gidDe(t *testing.T, st *Store) int64 {
	t.Helper()
	var gid int64
	if err := st.db.QueryRow(`SELECT id FROM grupos WHERE codigo='CC0001'`).Scan(&gid); err != nil {
		t.Fatalf("grupo CC0001: %v", err)
	}
	return gid
}

// usuarioID: id do usuário pelo login (para criado_por e afins).
func usuarioID(t *testing.T, st *Store, login string) int64 {
	t.Helper()
	var id int64
	if err := st.db.QueryRow(`SELECT id FROM usuarios WHERE login = ?`, login).Scan(&id); err != nil {
		t.Fatalf("usuário %s: %v", login, err)
	}
	return id
}

// Cenário 2 — fechar duplo: 2 fechar SIMULTÂNEOS → 1×200 + 1×409; conferência
// fecha 1× e o lançamento não duplica (transação serializada por MaxOpenConns(1)).
func TestConcorrenciaFecharDuplo(t *testing.T) {
	app, st, cleanup, _, _, p2, tok := preparaConferenciaAberta(t)
	defer cleanup()

	var confID int64
	if err := st.db.QueryRow(`SELECT id FROM conferencias WHERE status='aberta'`).Scan(&confID); err != nil {
		t.Fatalf("conferência aberta: %v", err)
	}
	body := map[string]any{
		"id": confID,
		"lancamentos": []map[string]any{
			{"pessoa_id": p2, "situacao": "presente", "verificado": true},
		},
	}
	recs := doJSONAsync(app, 2, "POST", "/api/conferencia/fechar", body, tok)

	oks, conflitos := 0, 0
	for i, rr := range recs {
		switch rr.Code {
		case http.StatusOK:
			oks++
		case http.StatusConflict:
			conflitos++
		default:
			t.Fatalf("fechar #%d: status inesperado %d (%s)", i, rr.Code, rr.Body.String())
		}
	}
	if oks != 1 || conflitos != 1 {
		t.Fatalf("fechar duplo: esperado 1×200 + 1×409, obtido %d×200 + %d×409", oks, conflitos)
	}

	var status string
	if err := st.db.QueryRow(`SELECT status FROM conferencias WHERE id=?`, confID).Scan(&status); err != nil || status != "fechada" {
		t.Fatalf("conferência não ficou fechada (err=%v status=%q)", err, status)
	}
	var n int
	_ = st.db.QueryRow(`SELECT COUNT(*) FROM presencas WHERE conferencia_id=? AND pessoa_id=?`, confID, p2).Scan(&n)
	if n != 1 {
		t.Fatalf("lançamento duplicado no fechar duplo: %d linhas", n)
	}
}

// Cenário 2b — fechar duplo com SESSÕES diferentes (gerente A × gerente C),
// sequenciais: 2º fechar sobre conferência fechada tem que dar 409.
func TestConcorrenciaFecharDuploSessoesDistintas(t *testing.T) {
	app, st, cleanup, _, _, p2, tok := preparaConferenciaAberta(t)
	defer cleanup()

	criaUsuarioTeste(t, st, "ger_conc_c", "senha-gerente", "operador")
	tokB := vinculaGrupoDoLogin(t, app, st, "ger_conc_c", gidDe(t, st))

	var confID int64
	if err := st.db.QueryRow(`SELECT id FROM conferencias WHERE status='aberta'`).Scan(&confID); err != nil {
		t.Fatalf("conferência aberta: %v", err)
	}
	gera := func() map[string]any {
		return map[string]any{
			"id":          confID,
			"lancamentos": []map[string]any{{"pessoa_id": p2, "situacao": "presente", "verificado": true}},
		}
	}
	recA, _ := doJSONReq(app, "POST", "/api/conferencia/fechar", gera(), tok)
	recB, _ := doJSONReq(app, "POST", "/api/conferencia/fechar", gera(), tokB)

	if recA.Code != http.StatusOK || recB.Code != http.StatusConflict {
		t.Fatalf("fechar A/B: esperado 200+409, obtido %d+%d (%s | %s)", recA.Code, recB.Code, recA.Body.String(), recB.Body.String())
	}
}

// Cenário 3 — backup concorrente: N POST /api/backup ao mesmo tempo. Nomes de
// arquivo têm que ser únicos (sufixo anti-colisão no mesmo segundo) e sha256
// presente; depois, um fechamento dispara backupAssincrono (goroutine × VACUUM).
func TestConcorrenciaBackupSimultaneo(t *testing.T) {
	app, st, cleanup, _, p1, _, tok := preparaConferenciaAberta(t)
	defer cleanup()

	tokAdmin := loginAs(t, app, "admin", "admin123") // backup é admin-only (auth(true))
	_ = tok

	const n = 4
	recs := doJSONAsync(app, n, "POST", "/api/backup", nil, tokAdmin)

	nomes := map[string]bool{}
	for i, rr := range recs {
		if rr.Code != http.StatusOK {
			t.Fatalf("backup #%d: status %d (%s)", i, rr.Code, rr.Body.String())
		}
		m := jsonDe(t, rr)
		arq, _ := m["arquivo"].(string)
		sha, _ := m["sha256"].(string)
		if !strings.HasPrefix(arq, "backups/sci_") || !strings.HasSuffix(arq, ".db") {
			t.Fatalf("backup #%d: arquivo inválido %q", i, arq)
		}
		if len(sha) != 64 {
			t.Fatalf("backup #%d: sha256 inválido %q", i, sha)
		}
		if nomes[arq] {
			t.Fatalf("backup #%d: nome de arquivo duplicado %q — colisão de segundo sem sufixo", i, arq)
		}
		nomes[arq] = true
	}
	if len(nomes) != n {
		t.Fatalf("esperado %d arquivos distintos, obtido %d", n, len(nomes))
	}
	for arq := range nomes {
		if _, err := os.Stat(filepath.Join(st.dataDir, filepath.FromSlash(arq))); err != nil {
			t.Fatalf("arquivo de backup ausente: %s (%v)", arq, err)
		}
	}

	// fechamento dispara backupAssincrono — o -race vigia a memória compartilhada
	var confID int64
	if err := st.db.QueryRow(`SELECT id FROM conferencias WHERE status='aberta'`).Scan(&confID); err != nil {
		t.Fatalf("conferência aberta: %v", err)
	}
	body := map[string]any{
		"id":          confID,
		"lancamentos": []map[string]any{{"pessoa_id": p1, "situacao": "presente", "verificado": true}},
	}
	if rr := doJSONAsync(app, 1, "POST", "/api/conferencia/fechar", body, tok)[0]; rr.Code != http.StatusOK {
		t.Fatalf("fechar pós-backups: status %d (%s)", rr.Code, rr.Body.String())
	}
	// espera o goroutine do backup assíncrono terminar (ou falhar com flag)
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(filepath.Join(st.dataDir, "FLAG_BACKUP.txt")); err == nil {
			t.Fatalf("backup assíncrono pós-fechamento falhou (FLAG_BACKUP.txt presente)")
		}
		time.Sleep(100 * time.Millisecond)
	}
}

// Cenário 3b — tempestade mista: logins, marcações, leituras e métricas cruzadas
// no mesmo App. O objetivo é o -race: nenhum acesso de memória sem guarda.
func TestConcorrenciaTempestadeMista(t *testing.T) {
	app, st, cleanup, _, p1, _, tok := preparaConferenciaAberta(t)
	defer cleanup()

	tokAdmin := loginAs(t, app, "admin", "admin123")
	criaUsuarioTeste(t, st, "ger_conc_d", "senha-gerente", "operador")
	tokB := vinculaGrupoDoLogin(t, app, st, "ger_conc_d", gidDe(t, st))

	sits := []string{"presente", "atraso", "presente", "atraso"}
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			switch i % 4 {
			case 0: // login concorrente (sessões novas no mesmo App)
				doJSONReq(app, "POST", "/api/login", map[string]string{"login": "ger_conc_d", "senha": "senha-gerente"}, nil)
			case 1: // marcações na mesma pessoa
				doJSONReq(app, "POST", "/api/conferencia/marcar", map[string]any{"pessoa_id": p1, "situacao": sits[i%len(sits)]}, tok)
			case 2: // leitura de efetivo + health
				doJSONReq(app, "GET", "/api/efetivo_atual", nil, tok)
				doJSONReq(app, "GET", "/api/health", nil, nil)
			case 3: // métricas admin (coletarMetricas tem estado global amostrado)
				doJSONReq(app, "GET", "/api/admin/sistema/metricas", nil, tokAdmin)
				doJSONReq(app, "GET", "/api/efetivo_atual", nil, tokB)
			}
		}(i)
	}
	wg.Wait()

	// nada pegou fogo: efetivo continua respondendo 200 com a pessoa do grupo
	rr, res := doJSONReq(app, "GET", "/api/efetivo_atual", nil, tok)
	if rr.Code != http.StatusOK {
		t.Fatalf("efetivo_atual pós-tempestade: %d (%s)", rr.Code, rr.Body.String())
	}
	pessoas, _ := res["pessoas"].([]any)
	if len(pessoas) != 2 {
		t.Fatalf("efetivo_atual pós-tempestade: esperado 2 pessoas, obtido %d", len(pessoas))
	}
}
