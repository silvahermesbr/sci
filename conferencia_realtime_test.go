package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestConferenciaRealtimeStream(t *testing.T) {
	app, st, teardown := setupTestApp(t)
	defer teardown()

	// 1. Criar grupo e militar
	resG, err := st.db.Exec(`INSERT INTO grupos (nome, codigo) VALUES ('Pelotão de Operações', 'PELOPS')`)
	if err != nil {
		t.Fatalf("erro ao criar grupo: %v", err)
	}
	grupoID, _ := resG.LastInsertId()

	resP, err := st.db.Exec(`INSERT INTO pessoas (nome_guerra, nome_completo, grupo_id, status) VALUES ('Oliveira', 'Oliveira Santos', ?, 'ativo')`, grupoID)
	if err != nil {
		t.Fatalf("erro ao criar pessoa: %v", err)
	}
	pessoaID, _ := resP.LastInsertId()

	// 2. Criar usuário gerente e sessão
	hashPadrao, _ := hashSenha("senha12345")
	resU, err := st.db.Exec(`INSERT INTO usuarios (login, senha_hash, papel, grupo_id, precisa_setup) VALUES ('gerente_ops', ?, 'gerente', ?, 0)`, hashPadrao, grupoID)
	if err != nil {
		t.Fatalf("erro ao criar usuario: %v", err)
	}
	gerenteUID, _ := resU.LastInsertId()
	_, _ = st.db.Exec(`INSERT INTO usuario_papeis (usuario_id, grupo_id, papel) VALUES (?, ?, 'gerente')`, gerenteUID, grupoID)

	tokenGerente, _, err := st.CriarSessao(gerenteUID, ttlSessao)
	if err != nil {
		t.Fatalf("erro ao criar sessao: %v", err)
	}

	// 3. Iniciar conferência
	recIni := httptest.NewRecorder()
	reqIni := httptest.NewRequest(http.MethodPost, "/api/conferencia/iniciar", bytes.NewBufferString(`{}`))
	reqIni.AddCookie(&http.Cookie{Name: cookieSessao, Value: tokenGerente})
	reqIni.Header.Set("X-SCI", "1")
	reqIni.Header.Set("Origin", "http://example.com")
	app.mux.ServeHTTP(recIni, reqIni)
	if recIni.Code != http.StatusOK {
		t.Fatalf("esperava 200 ao iniciar conf, obteve %d: %s", recIni.Code, recIni.Body.String())
	}

	var iniResp struct {
		ID int64 `json:"id"`
	}
	if err := json.Unmarshal(recIni.Body.Bytes(), &iniResp); err != nil || iniResp.ID <= 0 {
		t.Fatalf("falha ao obter ID da conferencia: %v, corpo=%s", err, recIni.Body.String())
	}
	confID := iniResp.ID

	// 4. Iniciar servidor de teste HTTP real para streaming SSE
	ts := httptest.NewServer(app.mux)
	defer ts.Close()

	// 5. Conectar cliente SSE
	reqStream, err := http.NewRequest(http.MethodGet, ts.URL+"/api/conferencia/"+strings.TrimSpace(recIni.Body.String()), nil)
	// rota oficial com ID
	streamURL := fmt.Sprintf("%s/api/conferencia/%d/stream", ts.URL, confID)
	reqStream, err = http.NewRequest(http.MethodGet, streamURL, nil)
	if err != nil {
		t.Fatalf("falha ao criar request do stream: %v", err)
	}
	reqStream.AddCookie(&http.Cookie{Name: cookieSessao, Value: tokenGerente})

	respStream, err := http.DefaultClient.Do(reqStream)
	if err != nil {
		t.Fatalf("falha ao conectar stream SSE: %v", err)
	}
	defer respStream.Body.Close()

	if respStream.StatusCode != http.StatusOK {
		t.Fatalf("esperava status 200 no SSE, obteve: %d", respStream.StatusCode)
	}

	contentType := respStream.Header.Get("Content-Type")
	if !strings.HasPrefix(contentType, "text/event-stream") {
		t.Fatalf("esperava Content-Type text/event-stream, obteve: %s", contentType)
	}

	reader := bufio.NewReader(respStream.Body)

	// Ler evento inicial {"tipo":"conectado"}
	line, err := reader.ReadString('\n')
	if err != nil {
		t.Fatalf("falha ao ler primeira linha SSE: %v", err)
	}
	if !strings.Contains(line, "conectado") {
		t.Fatalf("esperava evento conectado, obteve: %s", line)
	}

	// 6. Em outra requisição, marcar falta para o militar
	recMarcar := httptest.NewRecorder()
	corpoMarcar := fmt.Sprintf(`{"pessoa_id": %d, "situacao": "falta", "verificado": true}`, pessoaID)
	reqMarcar := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/conferencia/marcar?id=%d", confID), bytes.NewBufferString(corpoMarcar))
	reqMarcar.AddCookie(&http.Cookie{Name: cookieSessao, Value: tokenGerente})
	reqMarcar.Header.Set("X-SCI", "1")
	reqMarcar.Header.Set("Origin", "http://example.com")
	app.mux.ServeHTTP(recMarcar, reqMarcar)
	if recMarcar.Code != http.StatusOK {
		t.Fatalf("esperava 200 ao marcar, obteve: %d: %s", recMarcar.Code, recMarcar.Body.String())
	}

	// 7. Ler evento de 'marcar' no stream SSE
	canalMsg := make(chan string, 1)
	go func() {
		for {
			l, e := reader.ReadString('\n')
			if e != nil {
				return
			}
			if strings.HasPrefix(l, "data: ") {
				canalMsg <- l
				return
			}
		}
	}()

	select {
	case msg := <-canalMsg:
		if !strings.Contains(msg, `"tipo":"marcar"`) || !strings.Contains(msg, `"situacao":"falta"`) {
			t.Fatalf("mensagem SSE recebida não contém os dados esperados: %s", msg)
		}
	case <-time.After(3 * time.Second):
		t.Fatalf("timeout aguardando evento de marcar no SSE")
	}

	// 8. Fechar a conferência e verificar se evento 'fechada' chega no stream
	recFechar := httptest.NewRecorder()
	corpoFechar := fmt.Sprintf(`{"id": %d, "lancamentos": [{"pessoa_id": %d, "situacao": "falta", "verificado": true}]}`, confID, pessoaID)
	reqFechar := httptest.NewRequest(http.MethodPost, "/api/conferencia/fechar", bytes.NewBufferString(corpoFechar))
	reqFechar.AddCookie(&http.Cookie{Name: cookieSessao, Value: tokenGerente})
	reqFechar.Header.Set("X-SCI", "1")
	reqFechar.Header.Set("Origin", "http://example.com")
	app.mux.ServeHTTP(recFechar, reqFechar)
	if recFechar.Code != http.StatusOK {
		t.Fatalf("esperava 200 ao fechar conferencia, obteve: %d: %s", recFechar.Code, recFechar.Body.String())
	}

	canalFechar := make(chan string, 1)
	go func() {
		for {
			l, e := reader.ReadString('\n')
			if e != nil {
				return
			}
			if strings.HasPrefix(l, "data: ") && strings.Contains(l, `"tipo":"fechada"`) {
				canalFechar <- l
				return
			}
		}
	}()

	select {
	case msg := <-canalFechar:
		if !strings.Contains(msg, `"tipo":"fechada"`) {
			t.Fatalf("esperava evento tipo fechada, obteve: %s", msg)
		}
	case <-time.After(3 * time.Second):
		t.Fatalf("timeout aguardando evento fechada no SSE")
	}
}
