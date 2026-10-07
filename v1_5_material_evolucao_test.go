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

func TestV15_MaterialEvolucao_SetorGaragemEConferencia(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "sci-mat-evol-*")
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

	// Criação de Grupo, Setor, Pessoas (Encarregado, Auxiliar, Padrinhos)
	resG, err := st.db.Exec(`INSERT INTO grupos (nome, codigo) VALUES ('3º Btl Com', '3BCOM')`)
	if err != nil {
		t.Fatal(err)
	}
	gid, _ := resG.LastInsertId()

	resS, err := st.db.Exec(`INSERT INTO setores (grupo_id, nome, sigla, ativo) VALUES (?, 'Pelotão de Manutenção', 'PELMAN', 1)`, gid)
	if err != nil {
		t.Fatal(err)
	}
	sID, _ := resS.LastInsertId()

	resEnc, _ := st.db.Exec(`INSERT INTO pessoas (grupo_id, setor_id, nome_guerra, nome_completo, status) VALUES (?, ?, 'Sgt Alves', 'Carlos Alves', 'ativo')`, gid, sID)
	encID, _ := resEnc.LastInsertId()

	resAux, _ := st.db.Exec(`INSERT INTO pessoas (grupo_id, setor_id, nome_guerra, nome_completo, status) VALUES (?, ?, 'Cb Silva', 'Lucas Silva', 'ativo')`, gid, sID)
	auxID, _ := resAux.LastInsertId()

	resPadTit, _ := st.db.Exec(`INSERT INTO pessoas (grupo_id, setor_id, nome_guerra, nome_completo, status) VALUES (?, ?, 'Sd Santos', 'Pedro Santos', 'ativo')`, gid, sID)
	padTitID, _ := resPadTit.LastInsertId()

	resU, _ := st.db.Exec(`INSERT INTO usuarios (login, senha_hash, papel, grupo_id, setor_id, ativo) VALUES ('operador_mat', 'hash', 'operador', ?, ?, 1)`, gid, sID)
	uID, _ := resU.LastInsertId()
	_, _ = st.db.Exec(`INSERT INTO usuario_papeis (usuario_id, grupo_id, papel) VALUES (?, ?, 'operador')`, uID, gid)

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
		rec := httptest.NewRecorder()
		app.mux.ServeHTTP(rec, req)

		var resp map[string]any
		_ = json.Unmarshal(rec.Body.Bytes(), &resp)
		return rec, resp
	}

	// 1. Salvar Designação de Encarregado e Auxiliar de Material
	t.Run("1. Encarregados e Auxiliares de Material", func(t *testing.T) {
		rec, resp := doReq("POST", "/api/material/responsaveis", map[string]any{
			"setor_id":                sID,
			"encarregado_id":         encID,
			"auxiliar_encarregado_id": auxID,
		})
		if rec.Code != http.StatusOK || resp["ok"] != true {
			t.Fatalf("falha ao salvar encarregados: code=%d resp=%v", rec.Code, resp)
		}

		recL, respL := doReq("GET", "/api/material/responsaveis", nil)
		if recL.Code != http.StatusOK {
			t.Fatalf("falha ao listar responsáveis: code=%d", recL.Code)
		}
		resps, _ := respL["responsaveis"].([]any)
		if len(resps) == 0 {
			t.Fatalf("esperava ao menos 1 responsável cadastrado")
		}
	})

	// 2. Cadastrar Viatura com Padrinho, Placa e Hodômetro
	var viaturaID int64
	t.Run("2. Cadastro de Viatura e Garagem", func(t *testing.T) {
		rec, resp := doReq("POST", "/api/material/itens", map[string]any{
			"nome":                  "Viatura Agrale Marruá",
			"codigo_patrimonio":     "VT-01",
			"setor_id":              sID,
			"sensibilidade":         "controlado",
			"status":                "disponivel",
			"placa":                 "EB-5001",
			"renavam":               "1234567890",
			"padrinho_titular_id":   padTitID,
			"hodometro_atual":       12500,
			"combustivel_atual":     "cheio",
		})
		if rec.Code != http.StatusOK || resp["ok"] != true {
			t.Fatalf("falha ao cadastrar viatura: code=%d resp=%v", rec.Code, resp)
		}
		viaturaID = int64(resp["id"].(float64))

		// Listar na Garagem
		recG, respG := doReq("GET", "/api/material/itens?garagem=1", nil)
		if recG.Code != http.StatusOK {
			t.Fatalf("falha ao listar garagem: code=%d", recG.Code)
		}
		itens, _ := respG["itens"].([]any)
		if len(itens) == 0 {
			t.Fatalf("esperava viatura na garagem")
		}
	})

	// 3. Dossiê da Viatura: Anexo e Comentário
	t.Run("3. Dossiê da Viatura (Anexo e Comentários)", func(t *testing.T) {
		// Comentário
		recC, respC := doReq("POST", "/api/material/itens/"+testIntToStr(viaturaID)+"/comentarios", map[string]any{
			"texto": "Troca de óleo aos 12.500 km realizada com filtro novo.",
		})
		if recC.Code != http.StatusOK || respC["ok"] != true {
			t.Fatalf("falha ao adicionar comentário: code=%d resp=%v", recC.Code, respC)
		}

		// Listar comentários
		recCL, respCL := doReq("GET", "/api/material/itens/"+testIntToStr(viaturaID)+"/comentarios", nil)
		if recCL.Code != http.StatusOK {
			t.Fatalf("falha ao listar comentários: code=%d", recCL.Code)
		}
		coms, _ := respCL["comentarios"].([]any)
		if len(coms) == 0 {
			t.Fatalf("esperava comentário salvo na viatura")
		}

		// Anexo
		recA, respA := doReq("POST", "/api/material/itens/"+testIntToStr(viaturaID)+"/anexos", map[string]any{
			"nome_arquivo": "manual_revisao.txt",
			"tipo_mime":    "text/plain",
			"tamanho":      14,
			"dados_base64": "SGVsbG8gV29ybGQ=",
		})
		if recA.Code != http.StatusOK || respA["ok"] != true {
			t.Fatalf("falha ao anexar documento: code=%d resp=%v", recA.Code, respA)
		}
		anexoID := int64(respA["id"].(float64))

		// Obter Anexo
		recAG, _ := doReq("GET", "/api/material/item-anexos/"+testIntToStr(anexoID), nil)
		if recAG.Code != http.StatusOK {
			t.Fatalf("falha ao baixar anexo: code=%d", recAG.Code)
		}
	})

	// 4. Conferência Diária de Material & Emissão do Pronto Oficial
	t.Run("4. Check Diário de Material e Emissão de Pronto", func(t *testing.T) {
		// Iniciar conferência
		recI, respI := doReq("POST", "/api/material/conferencias/iniciar", map[string]any{
			"data":     "2026-10-06",
			"setor_id": sID,
		})
		if recI.Code != http.StatusOK || respI["ok"] != true {
			t.Fatalf("falha ao iniciar conferência diária: code=%d resp=%v", recI.Code, respI)
		}
		confID := int64(respI["id"].(float64))

		// Bipar o item como presente
		recB, respB := doReq("POST", "/api/material/conferencias/"+testIntToStr(confID)+"/bipar", map[string]any{
			"codigo_patrimonio": "VT-01",
			"status":            "presente",
		})
		if recB.Code != http.StatusOK || respB["ok"] != true {
			t.Fatalf("falha ao bipar item na conferência: code=%d resp=%v", recB.Code, respB)
		}

		// Fechar conferência
		recF, respF := doReq("POST", "/api/material/conferencias/"+testIntToStr(confID)+"/fechar", map[string]any{
			"observacao": "Check de passagem de serviço concluído sem alterações.",
		})
		if recF.Code != http.StatusOK || respF["ok"] != true {
			t.Fatalf("falha ao fechar conferência: code=%d resp=%v", recF.Code, respF)
		}

		// Obter Pronto em PDF
		recPDF, _ := doReq("GET", "/api/material/conferencias/"+testIntToStr(confID)+"/pronto.pdf", nil)
		if recPDF.Code != http.StatusOK || recPDF.Header().Get("Content-Type") != "application/pdf" {
			t.Fatalf("esperava PDF do pronto militar: code=%d type=%s", recPDF.Code, recPDF.Header().Get("Content-Type"))
		}
		if len(recPDF.Body.Bytes()) < 500 {
			t.Fatalf("PDF gerado é muito pequeno (%d bytes)", len(recPDF.Body.Bytes()))
		}
	})
}

func testIntToStr(n int64) string {
	b, _ := json.Marshal(n)
	return string(b)
}
