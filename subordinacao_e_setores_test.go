package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestSubordinacaoESetores(t *testing.T) {
	app, st, teardown := setupTestApp(t)
	defer teardown()

	// 1. Criar Grupo Superior (G1) e Grupo Subordinado (G2) e Grupo Alheio (G3)
	resG1, err := st.db.Exec(`INSERT INTO grupos (nome, codigo) VALUES ('Batalhão', 'BATALHAO')`)
	if err != nil {
		t.Fatalf("erro ao criar G1: %v", err)
	}
	g1ID, _ := resG1.LastInsertId()

	resG2, _ := st.db.Exec(`INSERT INTO grupos (nome, codigo) VALUES ('1ª Companhia', '1CIA')`)
	g2ID, _ := resG2.LastInsertId()

	resG3, _ := st.db.Exec(`INSERT INTO grupos (nome, codigo) VALUES ('Outro Batalhão', 'OUTRO')`)
	g3ID, _ := resG3.LastInsertId()

	// Criar vínculo bilateral G1 -> G2 (G2 é subordinado a G1)
	_, _ = st.db.Exec(`INSERT INTO grupo_vinculos (superior_id, subordinado_id, criado_por_superior, criado_por_subordinado) VALUES (?, ?, 1, 1)`, g1ID, g2ID)

	// 2. Criar Gerente de G1
	hash, _ := hashSenha("senha12345")
	resU, _ := st.db.Exec(`INSERT INTO usuarios (login, senha_hash, papel, grupo_id, precisa_setup) VALUES ('gerente_batalhao', ?, 'gerente', ?, 0)`, hash, g1ID)
	gerenteID, _ := resU.LastInsertId()
	_, _ = st.db.Exec(`INSERT INTO usuario_papeis (usuario_id, grupo_id, papel) VALUES (?, ?, 'gerente')`, gerenteID, g1ID)
	tokenGerente, _, _ := st.CriarSessao(gerenteID, ttlSessao)

	// Helper para requisições autenticadas do gerente
	fazerReq := func(metodo, rota string, corpo any) *httptest.ResponseRecorder {
		var buf *bytes.Buffer
		if corpo != nil {
			b, _ := json.Marshal(corpo)
			buf = bytes.NewBuffer(b)
		} else {
			buf = bytes.NewBuffer(nil)
		}
		req := httptest.NewRequest(metodo, rota, buf)
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-SCI", "1")
		req.Header.Set("Origin", "http://example.com")
		req.AddCookie(&http.Cookie{Name: "sci_sessao", Value: tokenGerente})
		rec := httptest.NewRecorder()
		app.mux.ServeHTTP(rec, req)
		return rec
	}

	// 3. Gerente de G1 cria Operador diretamente em G2 (subordinado) -> DEVE FUNCIONAR (200)
	recAddOp := fazerReq(http.MethodPost, "/api/usuarios", map[string]any{
		"login":    "op_subordinado",
		"senha":    "senha12345",
		"papel":    "operador",
		"grupo_id": g2ID,
	})
	if recAddOp.Code != http.StatusOK {
		t.Fatalf("esperava 200 ao criar operador no grupo subordinado, obteve %d: %s", recAddOp.Code, recAddOp.Body.String())
	}
	var resOp map[string]any
	_ = json.Unmarshal(recAddOp.Body.Bytes(), &resOp)
	opID := int64(resOp["id"].(float64))

	// 4. Gerente de G1 tenta criar operador em G3 (alheio) -> DEVE SER 403 FORBIDDEN
	recAddG3 := fazerReq(http.MethodPost, "/api/usuarios", map[string]any{
		"login":    "op_alheio",
		"senha":    "senha12345",
		"papel":    "operador",
		"grupo_id": g3ID,
	})
	if recAddG3.Code != http.StatusForbidden {
		t.Fatalf("esperava 403 ao tentar criar usuário em grupo alheio, obteve %d", recAddG3.Code)
	}

	// 5. Gerente de G1 cria Setor em G2 (subordinado) -> DEVE FUNCIONAR
	recAddSetor := fazerReq(http.MethodPost, "/api/catalogo/setores", map[string]any{
		"nome":     "Pelotão de Operações Especiais",
		"sigla":    "PELOP",
		"grupo_id": g2ID,
	})
	if recAddSetor.Code != http.StatusOK {
		t.Fatalf("esperava 200 ao criar setor em grupo subordinado, obteve %d: %s", recAddSetor.Code, recAddSetor.Body.String())
	}
	var resSetor map[string]any
	_ = json.Unmarshal(recAddSetor.Body.Bytes(), &resSetor)
	setorID := int64(resSetor["id"].(float64))

	// 6. Gerente de G1 atribui papel de Chefe de Setor e vincula setor ao operador do subgrupo
	recEditUser := fazerReq(http.MethodPatch, "/api/usuarios/"+httpMethodID(opID), map[string]any{
		"setor_id": setorID,
	})
	if recEditUser.Code != http.StatusOK {
		t.Fatalf("esperava 200 ao atualizar setor_id do usuário, obteve %d: %s", recEditUser.Code, recEditUser.Body.String())
	}

	recAddPapel := fazerReq(http.MethodPost, "/api/usuarios/"+httpMethodID(opID)+"/papeis", map[string]any{
		"papel":    "chefe_setor",
		"grupo_id": g2ID,
	})
	if recAddPapel.Code != http.StatusOK {
		t.Fatalf("esperava 200 ao atribuir chefe_setor em grupo subordinado, obteve %d: %s", recAddPapel.Code, recAddPapel.Body.String())
	}

	// 7. Gerente de G1 redefine senha do usuário no grupo subordinado
	recSenha := fazerReq(http.MethodPost, "/api/usuarios/"+httpMethodID(opID)+"/senha", map[string]any{
		"senha": "novaSenha888",
	})
	if recSenha.Code != http.StatusOK {
		t.Fatalf("esperava 200 ao redefinir senha no subordinado, obteve %d: %s", recSenha.Code, recSenha.Body.String())
	}

	// 8. Checar listagem de usuários com GET /api/usuarios para o gerente:
	// Deve conter tanto o gerente quanto o operador do grupo subordinado com o setor_id preenchido
	recListU := fazerReq(http.MethodGet, "/api/usuarios", nil)
	if recListU.Code != http.StatusOK {
		t.Fatalf("esperava 200 na listagem de usuários, obteve %d", recListU.Code)
	}
	var listaU []map[string]any
	_ = json.Unmarshal(recListU.Body.Bytes(), &listaU)
	achouOpSub := false
	for _, u := range listaU {
		if u["login"] == "op_subordinado" {
			achouOpSub = true
			if u["setor_id"] == nil || int64(u["setor_id"].(float64)) != setorID {
				t.Errorf("esperava setor_id %d para op_subordinado, obteve %v", setorID, u["setor_id"])
			}
		}
	}
	if !achouOpSub {
		t.Errorf("op_subordinado não foi retornado na listagem de usuários para o gerente")
	}

	// 9. Gerente exclui o papel de chefe_setor e a conta do operador no grupo subordinado
	recDelUser := fazerReq(http.MethodDelete, "/api/usuarios/"+httpMethodID(opID), nil)
	if recDelUser.Code != http.StatusOK {
		t.Fatalf("esperava 200 ao excluir usuário no grupo subordinado, obteve %d: %s", recDelUser.Code, recDelUser.Body.String())
	}
}

func httpMethodID(id int64) string {
	b, _ := json.Marshal(id)
	return string(b)
}
