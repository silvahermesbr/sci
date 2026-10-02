package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"
)

func TestV15_MaterialSensibilidade(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "sci-mat-sens-*")
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

	// Cria grupo, setor e militar
	resG, err := st.db.Exec(`INSERT INTO grupos (nome, codigo) VALUES ('1ª Companhia Material', 'CIAMAT')`)
	if err != nil {
		t.Fatal(err)
	}
	gid, _ := resG.LastInsertId()

	resS, err := st.db.Exec(`INSERT INTO setores (grupo_id, nome, sigla, ativo) VALUES (?, 'Logística', 'LOG', 1)`, gid)
	if err != nil {
		t.Fatal(err)
	}
	sID, _ := resS.LastInsertId()

	resP, _ := st.db.Exec(`INSERT INTO pessoas (grupo_id, setor_id, nome_guerra, nome_completo, status) VALUES (?, ?, 'Oliveira', 'Marcos Oliveira', 'ativo')`, gid, sID)
	pID, _ := resP.LastInsertId()

	resUChefeMat, _ := st.db.Exec(`INSERT INTO usuarios (login, senha_hash, papel, grupo_id, setor_id, ativo) VALUES ('chefe_mat', 'hash', 'chefe_setor', ?, ?, 1)`, gid, sID)
	uID, _ := resUChefeMat.LastInsertId()
	_, _ = st.db.Exec(`INSERT INTO usuario_papeis (usuario_id, grupo_id, papel) VALUES (?, ?, 'chefe_setor')`, uID, gid)

	tok, _, err := st.CriarSessao(uID, time.Hour)
	if err != nil {
		t.Fatal(err)
	}

	doReq := func(method, url string, body any) (*httptest.ResponseRecorder, map[string]any) {
		var rdr *bytes.Reader
		if body != nil {
			b, _ := json.Marshal(body)
			rdr = bytes.NewReader(b)
		} else {
			rdr = bytes.NewReader([]byte{})
		}
		req, _ := http.NewRequest(method, url, rdr)
		if body != nil {
			req.Header.Set("Content-Type", "application/json")
		}
		req.Header.Set("X-SCI", "1")
		req.AddCookie(&http.Cookie{Name: "sci_sessao", Value: tok})
		w := httptest.NewRecorder()
		app.mux.ServeHTTP(w, req)

		var res map[string]any
		_ = json.Unmarshal(w.Body.Bytes(), &res)
		return w, res
	}

	// 1. Cadastrar Material Convencional com quantidade = 50
	w, resConv := doReq("POST", "/api/material/itens", map[string]any{
		"nome":          "Cobertor de Lã",
		"sensibilidade": "convencional",
		"quantidade":    50,
		"setor_id":      sID,
		"status":        "disponivel",
	})
	if w.Code != http.StatusOK {
		t.Fatalf("erro ao criar material convencional: %d %s", w.Code, w.Body.String())
	}
	convID := int64(resConv["id"].(float64))

	// 2. Listar itens e verificar campos
	w, resList := doReq("GET", "/api/material/itens", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("erro ao listar itens: %d", w.Code)
	}
	itens := resList["itens"].([]any)
	if len(itens) != 1 {
		t.Fatalf("esperava 1 item, obteve %d", len(itens))
	}
	it0 := itens[0].(map[string]any)
	if it0["sensibilidade"] != "convencional" {
		t.Fatalf("esperava sensibilidade convencional, obteve %v", it0["sensibilidade"])
	}
	if int(it0["quantidade"].(float64)) != 50 || int(it0["quantidade_disponivel"].(float64)) != 50 {
		t.Fatalf("esperava 50 quantidade e 50 disponivel, obteve qty=%v disp=%v", it0["quantidade"], it0["quantidade_disponivel"])
	}

	// 3. Fazer cautela parcial de 10 unidades do material convencional
	w, resCautela := doReq("POST", "/api/material/cautelar", map[string]any{
		"item_id":    convID,
		"pessoa_id":  pID,
		"quantidade": 10,
		"obs_saida":  "Instrução no Campo",
	})
	if w.Code != http.StatusOK {
		t.Fatalf("erro ao acautelar 10 itens: %d %s", w.Code, w.Body.String())
	}
	cautelaID := int64(resCautela["cautela_id"].(float64))

	// Verifica saldo após cautela: disponível deve ser 40, acautelada 10
	_, resList = doReq("GET", "/api/material/itens", nil)
	it0 = resList["itens"].([]any)[0].(map[string]any)
	if int(it0["quantidade_disponivel"].(float64)) != 40 || int(it0["quantidade_acautelada"].(float64)) != 10 {
		t.Fatalf("esperava saldo disp=40, acautelado=10, obteve disp=%v acaut=%v", it0["quantidade_disponivel"], it0["quantidade_acautelada"])
	}

	// 4. Tentar acautelar 45 unidades (quando só restam 40) -> deve falhar com 400
	w, _ = doReq("POST", "/api/material/cautelar", map[string]any{
		"item_id":    convID,
		"pessoa_id":  pID,
		"quantidade": 45,
		"obs_saida":  "Excesso",
	})
	if w.Code != http.StatusBadRequest {
		t.Fatalf("esperava 400 para cautela excedente, obteve %d", w.Code)
	}

	// 5. Devolver a cautela de 10 unidades -> saldo volta para 50 disponível
	w, _ = doReq("POST", "/api/material/devolver", map[string]any{
		"cautela_id":    cautelaID,
		"obs_devolucao": "Tudo devolvido em perfeito estado",
	})
	if w.Code != http.StatusOK {
		t.Fatalf("erro ao devolver cautela: %d", w.Code)
	}

	_, resList = doReq("GET", "/api/material/itens", nil)
	it0 = resList["itens"].([]any)[0].(map[string]any)
	if int(it0["quantidade_disponivel"].(float64)) != 50 || int(it0["quantidade_acautelada"].(float64)) != 0 {
		t.Fatalf("esperava saldo restaurado disp=50, acaut=0, obteve disp=%v acaut=%v", it0["quantidade_disponivel"], it0["quantidade_acautelada"])
	}

	// 6. Cadastrar Material Controlado -> deve forçar quantidade = 1
	w, resCtrl := doReq("POST", "/api/material/itens", map[string]any{
		"nome":          "Rádio Transceptor VHF",
		"sensibilidade": "controlado",
		"quantidade":    5, // mesmo enviando 5, deve forçar 1
		"numero_serie":  "VHF-10928",
		"patrimonio":    "PAT-991",
		"setor_id":      sID,
		"status":        "disponivel",
	})
	if w.Code != http.StatusOK {
		t.Fatalf("erro ao cadastrar material controlado: %d %s", w.Code, w.Body.String())
	}
	ctrlID := int64(resCtrl["id"].(float64))

	_, resList = doReq("GET", "/api/material/itens", nil)
	var ctrlItem map[string]any
	for _, it := range resList["itens"].([]any) {
		m := it.(map[string]any)
		if int64(m["id"].(float64)) == ctrlID {
			ctrlItem = m
			break
		}
	}
	if ctrlItem == nil {
		t.Fatalf("material controlado não encontrado na lista")
	}
	if ctrlItem["sensibilidade"] != "controlado" {
		t.Fatalf("esperava sensibilidade controlado, obteve %v", ctrlItem["sensibilidade"])
	}
	if int(ctrlItem["quantidade"].(float64)) != 1 {
		t.Fatalf("esperava quantidade forçada para 1 em material controlado, obteve %v", ctrlItem["quantidade"])
	}
}
