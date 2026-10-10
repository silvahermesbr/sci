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

func TestV15_ConferenciaPorSetor(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "sci-conf-setor-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	st, err := AbrirStore(tmpDir)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	if err := st.SeedIfEmpty("admin123"); err != nil {
		t.Fatal(err)
	}

	app := NovaApp(st)

	// Cria grupo e setores
	resG, err := st.db.Exec(`INSERT INTO grupos (nome, codigo) VALUES ('1ª Companhia Teste', 'CIA1')`)
	if err != nil {
		t.Fatal(err)
	}
	gid, _ := resG.LastInsertId()

	resS1, err := st.db.Exec(`INSERT INTO setores (grupo_id, nome, sigla, ativo) VALUES (?, 'Comando', 'CMDO', 1)`, gid)
	if err != nil {
		t.Fatal(err)
	}
	s1ID, _ := resS1.LastInsertId()

	resS2, err := st.db.Exec(`INSERT INTO setores (grupo_id, nome, sigla, ativo) VALUES (?, '1º Pelotão', '1PEL', 1)`, gid)
	if err != nil {
		t.Fatal(err)
	}
	s2ID, _ := resS2.LastInsertId()

	// Militares no setor 1 e setor 2
	resP1, _ := st.db.Exec(`INSERT INTO pessoas (grupo_id, setor_id, nome_guerra, nome_completo, status) VALUES (?, ?, 'Silva', 'José Silva', 'ativo')`, gid, s1ID)
	p1ID, _ := resP1.LastInsertId()

	resP2, _ := st.db.Exec(`INSERT INTO pessoas (grupo_id, setor_id, nome_guerra, nome_completo, status) VALUES (?, ?, 'Souza', 'Carlos Souza', 'ativo')`, gid, s2ID)
	_ = resP2

	// Cria gerente e chefe do setor 1
	resUMan, _ := st.db.Exec(`INSERT INTO usuarios (login, senha_hash, papel, grupo_id, ativo) VALUES ('gerente1', 'hash', 'gerente', ?, 1)`, gid)
	uManID, _ := resUMan.LastInsertId()
	_, _ = st.db.Exec(`INSERT INTO usuario_papeis (usuario_id, grupo_id, papel) VALUES (?, ?, 'gerente')`, uManID, gid)

	resUChefe1, _ := st.db.Exec(`INSERT INTO usuarios (login, senha_hash, papel, grupo_id, setor_id, ativo) VALUES ('chefe_cmdo', 'hash', 'chefe_setor', ?, ?, 1)`, gid, s1ID)
	uChefe1ID, _ := resUChefe1.LastInsertId()
	_, _ = st.db.Exec(`INSERT INTO usuario_papeis (usuario_id, grupo_id, papel) VALUES (?, ?, 'chefe_setor')`, uChefe1ID, gid)
	// v1.5.4-D1 (R-12): comando materializado em chefe_setores (fonte única) —
	// fixture antiga vivia do fallback usuarios.setor_id, removido do produto.
	materializaComandoSetor(t, st, uChefe1ID, gid, s1ID)

	loginToken := func(uid int64) string {
		tok, _, e := st.CriarSessao(uid, time.Hour)
		if e != nil {
			t.Fatal(e)
		}
		return tok
	}

	tokGer := loginToken(uManID)
	tokChefe1 := loginToken(uChefe1ID)

	doReq := func(method, url, tok string, body []byte) *httptest.ResponseRecorder {
		var rdr *bytes.Reader
		if body != nil {
			rdr = bytes.NewReader(body)
		} else {
			rdr = bytes.NewReader([]byte{})
		}
		req, _ := http.NewRequest(method, url, rdr)
		if body != nil {
			req.Header.Set("Content-Type", "application/json")
		}
		req.Header.Set("X-SCI", "1")
		if tok != "" {
			req.AddCookie(&http.Cookie{Name: "sci_sessao", Value: tok})
		}
		w := httptest.NewRecorder()
		app.mux.ServeHTTP(w, req)
		return w
	}

	// 1. Iniciar Conferência pelo Gerente
	w := doReq("POST", "/api/conferencia/iniciar", tokGer, []byte(`{"local": "Pátio Principal"}`))
	if w.Code != http.StatusOK {
		t.Fatalf("esperava 200 ao iniciar conferência, obteve %d: %s", w.Code, w.Body.String())
	}
	var resConf struct {
		ID int64 `json:"id"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &resConf)
	confID := resConf.ID
	if confID <= 0 {
		t.Fatalf("confID inválido: %d", confID)
	}

	// 2. Verificar que ambos os setores nasceram com status 'nao_iniciada'
	w = doReq("GET", fmt.Sprintf("/api/conferencia/hoje?id=%d", confID), tokGer, nil)
	if w.Code != http.StatusOK {
		t.Fatalf("erro ao buscar conferencia hoje: %d %s", w.Code, w.Body.String())
	}
	var resHoje struct {
		SetoresStatus []map[string]any `json:"setores_status"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &resHoje)
	if len(resHoje.SetoresStatus) < 2 {
		t.Fatalf("esperava pelo menos 2 setores no status, obteve %d", len(resHoje.SetoresStatus))
	}
	for _, s := range resHoje.SetoresStatus {
		if s["status"] != "nao_iniciada" {
			t.Fatalf("esperava status nao_iniciada para setor %v, obteve %v", s["setor_nome"], s["status"])
		}
	}

	// 3. Marcar presença em militar do Setor 1 -> transiciona para 'em_andamento'
	bodyMarcar := []byte(fmt.Sprintf(`{"pessoa_id": %d, "situacao": "presente", "verificado": true}`, p1ID))
	w = doReq("POST", fmt.Sprintf("/api/conferencia/marcar?id=%d", confID), tokChefe1, bodyMarcar)
	if w.Code != http.StatusOK {
		t.Fatalf("erro ao marcar presenca: %d %s", w.Code, w.Body.String())
	}

	// Consulta e confirma que Setor 1 virou 'em_andamento' e Setor 2 continua 'nao_iniciada'
	w = doReq("GET", fmt.Sprintf("/api/conferencia/hoje?id=%d", confID), tokGer, nil)
	_ = json.Unmarshal(w.Body.Bytes(), &resHoje)
	for _, s := range resHoje.SetoresStatus {
		sid := int64(s["setor_id"].(float64))
		if sid == s1ID && s["status"] != "em_andamento" {
			t.Fatalf("esperava status em_andamento para setor 1, obteve %v", s["status"])
		}
		if sid == s2ID && s["status"] != "nao_iniciada" {
			t.Fatalf("esperava status nao_iniciada para setor 2, obteve %v", s["status"])
		}
	}

	// 4. Chefe do Setor 1 tenta concluir o Setor 2 -> deve retornar 403 Forbidden!
	w = doReq("POST", fmt.Sprintf("/api/conferencia/%d/setor/%d/concluir", confID, s2ID), tokChefe1, nil)
	if w.Code != http.StatusForbidden {
		t.Fatalf("esperava 403 para chefe do setor 1 tentar concluir setor 2, obteve %d: %s", w.Code, w.Body.String())
	}

	// 5. Chefe do Setor 1 conclui o Setor 1 -> deve retornar 200 OK
	w = doReq("POST", fmt.Sprintf("/api/conferencia/%d/setor/%d/concluir", confID, s1ID), tokChefe1, nil)
	if w.Code != http.StatusOK {
		t.Fatalf("esperava 200 ao concluir setor 1, obteve %d: %s", w.Code, w.Body.String())
	}

	// 6. Validar que Setor 1 está como 'concluida'
	w = doReq("GET", fmt.Sprintf("/api/conferencia/hoje?id=%d", confID), tokGer, nil)
	_ = json.Unmarshal(w.Body.Bytes(), &resHoje)
	for _, s := range resHoje.SetoresStatus {
		sid := int64(s["setor_id"].(float64))
		if sid == s1ID && s["status"] != "concluida" {
			t.Fatalf("esperava status concluida para setor 1, obteve %v", s["status"])
		}
	}

	// 7. Chefe do Setor 1 reabre o Setor 1 -> deve retornar 'em_andamento'
	w = doReq("POST", fmt.Sprintf("/api/conferencia/%d/setor/%d/reabrir", confID, s1ID), tokChefe1, nil)
	if w.Code != http.StatusOK {
		t.Fatalf("esperava 200 ao reabrir setor 1, obteve %d: %s", w.Code, w.Body.String())
	}

	w = doReq("GET", fmt.Sprintf("/api/conferencia/hoje?id=%d", confID), tokGer, nil)
	_ = json.Unmarshal(w.Body.Bytes(), &resHoje)
	for _, s := range resHoje.SetoresStatus {
		sid := int64(s["setor_id"].(float64))
		if sid == s1ID {
			if s["status"] != "em_andamento" {
				t.Fatalf("esperava status em_andamento após reabertura, obteve %v", s["status"])
			}
			if s["total_pessoas"] == nil || s["verificados"] == nil {
				t.Fatalf("esperava aliases total_pessoas e verificados no payload do setor: %v", s)
			}
		}
	}
}
