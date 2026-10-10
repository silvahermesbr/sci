package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestChefeSetorEConferencia(t *testing.T) {
	app, st, teardown := setupTestApp(t)
	defer teardown()

	// 1. Testar endpoint de ping (conectividade)
	recPing := httptest.NewRecorder()
	reqPing := httptest.NewRequest(http.MethodGet, "/api/ping", nil)
	app.mux.ServeHTTP(recPing, reqPing)
	if recPing.Code != http.StatusOK {
		t.Fatalf("esperava 200 no ping, obteve %d", recPing.Code)
	}

	// 2. Criar Grupo, Setor A, Setor B
	resG, err := st.db.Exec(`INSERT INTO grupos (nome, codigo) VALUES ('Pelotão de Comando', 'PELCMD')`)
	if err != nil {
		t.Fatalf("erro ao criar grupo: %v", err)
	}
	grupoID, _ := resG.LastInsertId()

	resSA, _ := st.db.Exec(`INSERT INTO setores (nome, grupo_id) VALUES ('Comunicações', ?)`, grupoID)
	setorAID, _ := resSA.LastInsertId()

	resSB, _ := st.db.Exec(`INSERT INTO setores (nome, grupo_id) VALUES ('Operações', ?)`, grupoID)
	setorBID, _ := resSB.LastInsertId()

	// 3. Criar Pessoas: Militar A (Setor Comunicações), Militar B (Setor Operações)
	resPA, _ := st.db.Exec(`INSERT INTO pessoas (nome_guerra, nome_completo, grupo_id, setor_id, status) VALUES ('Silva', 'Silva Santos', ?, ?, 'ativo')`, grupoID, setorAID)
	pessoaAID, _ := resPA.LastInsertId()

	resPB, _ := st.db.Exec(`INSERT INTO pessoas (nome_guerra, nome_completo, grupo_id, setor_id, status) VALUES ('Souza', 'Souza Lima', ?, ?, 'ativo')`, grupoID, setorBID)
	pessoaBID, _ := resPB.LastInsertId()

	// 4. Criar Usuários:
	// - Gerente do Grupo
	// - Chefe do Setor A (Silva)
	hashPadrao, _ := hashSenha("senha12345")
	resU制定, _ := st.db.Exec(`INSERT INTO usuarios (login, senha_hash, papel, grupo_id, precisa_setup) VALUES ('gerente1', ?, 'gerente', ?, 0)`, hashPadrao, grupoID)
	gerenteUID, _ := resU制定.LastInsertId()
	_, _ = st.db.Exec(`INSERT INTO usuario_papeis (usuario_id, grupo_id, papel) VALUES (?, ?, 'gerente')`, gerenteUID, grupoID)

	resUChefe, _ := st.db.Exec(`INSERT INTO usuarios (login, senha_hash, papel, grupo_id, pessoa_id, setor_id, precisa_setup) VALUES ('chefe_com', ?, 'chefe_setor', ?, ?, ?, 0)`, hashPadrao, grupoID, pessoaAID, setorAID)
	chefeUID, _ := resUChefe.LastInsertId()
	_, _ = st.db.Exec(`INSERT INTO usuario_papeis (usuario_id, grupo_id, papel) VALUES (?, ?, 'chefe_setor')`, chefeUID, grupoID)
	// v1.5.4-D1 (R-12): comando materializado em chefe_setores (fonte única) —
	// fixture antiga vivia do fallback usuarios.setor_id, removido do produto.
	materializaComandoSetor(t, st, chefeUID, grupoID, setorAID)

	tokenGerente, _, _ := st.CriarSessao(gerenteUID, ttlSessao)
	tokenChefe, _, _ := st.CriarSessao(chefeUID, ttlSessao)

	// 5. Chefe de Setor TENTA iniciar conferência (deve ser PROIBIDO)
	recIniChefe := httptest.NewRecorder()
	reqIniChefe := httptest.NewRequest(http.MethodPost, "/api/conferencia/iniciar", bytes.NewBufferString(`{}`))
	reqIniChefe.AddCookie(&http.Cookie{Name: cookieSessao, Value: tokenChefe})
	reqIniChefe.Header.Set("X-SCI", "1")
	reqIniChefe.Header.Set("Origin", "http://example.com")
	reqIniChefe.Host = "example.com"
	app.mux.ServeHTTP(recIniChefe, reqIniChefe)
	if recIniChefe.Code != http.StatusForbidden {
		t.Fatalf("chefe de setor NÃO deveria conseguir iniciar conferência, obteve: %d", recIniChefe.Code)
	}

	// 6. Gerente inicia conferência (sucesso)
	recIniGer := httptest.NewRecorder()
	reqIniGer := httptest.NewRequest(http.MethodPost, "/api/conferencia/iniciar", bytes.NewBufferString(`{}`))
	reqIniGer.AddCookie(&http.Cookie{Name: cookieSessao, Value: tokenGerente})
	reqIniGer.Header.Set("X-SCI", "1")
	reqIniGer.Header.Set("Origin", "http://example.com")
	reqIniGer.Host = "example.com"
	app.mux.ServeHTTP(recIniGer, reqIniGer)
	if recIniGer.Code != http.StatusOK {
		t.Fatalf("gerente deveria iniciar conferência com sucesso, obteve: %d (%s)", recIniGer.Code, recIniGer.Body.String())
	}

	// 7. Chefe do Setor A tenta marcar militar do Setor B (Operações) -> DEVE SER RECUSADO (403 Forbidden)
	payloadInvalido, _ := json.Marshal(map[string]any{
		"pessoa_id":  pessoaBID,
		"situacao":   "presente",
		"verificado": true,
	})
	recMarcarInvalido := httptest.NewRecorder()
	reqMarcarInvalido := httptest.NewRequest(http.MethodPost, "/api/conferencia/marcar", bytes.NewBuffer(payloadInvalido))
	reqMarcarInvalido.AddCookie(&http.Cookie{Name: cookieSessao, Value: tokenChefe})
	reqMarcarInvalido.Header.Set("X-SCI", "1")
	reqMarcarInvalido.Header.Set("Origin", "http://example.com")
	reqMarcarInvalido.Host = "example.com"
	app.mux.ServeHTTP(recMarcarInvalido, reqMarcarInvalido)
	if recMarcarInvalido.Code != http.StatusForbidden {
		t.Fatalf("chefe de setor não deveria conseguir marcar militar de outro setor! código: %d, body: %s", recMarcarInvalido.Code, recMarcarInvalido.Body.String())
	}

	// 8. Chefe do Setor A marca militar do SEU próprio setor (Setor A) -> DEVE FUNCIONAR (200 OK)
	payloadValido, _ := json.Marshal(map[string]any{
		"pessoa_id":  pessoaAID,
		"situacao":   "presente",
		"verificado": true,
	})
	recMarcarValido := httptest.NewRecorder()
	reqMarcarValido := httptest.NewRequest(http.MethodPost, "/api/conferencia/marcar", bytes.NewBuffer(payloadValido))
	reqMarcarValido.AddCookie(&http.Cookie{Name: cookieSessao, Value: tokenChefe})
	reqMarcarValido.Header.Set("X-SCI", "1")
	reqMarcarValido.Header.Set("Origin", "http://example.com")
	reqMarcarValido.Host = "example.com"
	app.mux.ServeHTTP(recMarcarValido, reqMarcarValido)
	if recMarcarValido.Code != http.StatusOK {
		t.Fatalf("chefe de setor deveria conseguir marcar militar do seu próprio setor! código: %d, body: %s", recMarcarValido.Code, recMarcarValido.Body.String())
	}

	// 9. Chefe de Setor TENTA fechar conferência -> DEVE SER PROIBIDO (403 Forbidden)
	var confID int64
	_ = st.db.QueryRow(`SELECT id FROM conferencias WHERE status = 'aberta' AND grupo_id = ?`, grupoID).Scan(&confID)
	payloadFechar, _ := json.Marshal(map[string]any{
		"id": confID,
		"lancamentos": []map[string]any{
			{"pessoa_id": pessoaAID, "situacao": "presente", "verificado": true},
		},
	})
	recFecharChefe := httptest.NewRecorder()
	reqFecharChefe := httptest.NewRequest(http.MethodPost, "/api/conferencia/fechar", bytes.NewBuffer(payloadFechar))
	reqFecharChefe.AddCookie(&http.Cookie{Name: cookieSessao, Value: tokenChefe})
	reqFecharChefe.Header.Set("X-SCI", "1")
	reqFecharChefe.Header.Set("Origin", "http://example.com")
	reqFecharChefe.Host = "example.com"
	app.mux.ServeHTTP(recFecharChefe, reqFecharChefe)
	if recFecharChefe.Code != http.StatusForbidden {
		t.Fatalf("chefe de setor NÃO deveria conseguir fechar a conferência! obteve: %d", recFecharChefe.Code)
	}

	// 10. Gerente fecha a conferência com sucesso
	recFecharGer := httptest.NewRecorder()
	reqFecharGer := httptest.NewRequest(http.MethodPost, "/api/conferencia/fechar", bytes.NewBuffer(payloadFechar))
	reqFecharGer.AddCookie(&http.Cookie{Name: cookieSessao, Value: tokenGerente})
	reqFecharGer.Header.Set("X-SCI", "1")
	reqFecharGer.Header.Set("Origin", "http://example.com")
	reqFecharGer.Host = "example.com"
	app.mux.ServeHTTP(recFecharGer, reqFecharGer)
	if recFecharGer.Code != http.StatusOK {
		t.Fatalf("gerente deveria conseguir fechar a conferência! obteve: %d, body: %s", recFecharGer.Code, recFecharGer.Body.String())
	}
}
