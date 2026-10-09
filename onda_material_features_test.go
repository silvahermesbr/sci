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

func TestOndaMaterialFeatures(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "sci-mat-feat-*")
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

	// Criação de Grupo
	resG, err := st.db.Exec(`INSERT INTO grupos (nome, codigo) VALUES ('1ª Companhia de Fuzileiros', '1CIA')`)
	if err != nil {
		t.Fatal(err)
	}
	gid, _ := resG.LastInsertId()

	// Criação de Categorias
	resCatViat, err := st.db.Exec(`INSERT INTO material_categorias (grupo_id, nome, ativo) VALUES (?, 'Viaturas & Transportes', 1)`, gid)
	if err != nil {
		t.Fatal(err)
	}
	catViatID, _ := resCatViat.LastInsertId()

	resCatArm, err := st.db.Exec(`INSERT INTO material_categorias (grupo_id, nome, ativo) VALUES (?, 'Armamento Leve', 1)`, gid)
	if err != nil {
		t.Fatal(err)
	}
	catArmID, _ := resCatArm.LastInsertId()

	// Criação de Setores
	resS1, err := st.db.Exec(`INSERT INTO setores (grupo_id, nome, sigla, ativo) VALUES (?, 'Pelotão de Apoio', 'PELAP', 1)`, gid)
	if err != nil {
		t.Fatal(err)
	}
	s1ID, _ := resS1.LastInsertId()

	resS2, err := st.db.Exec(`INSERT INTO setores (grupo_id, nome, sigla, ativo) VALUES (?, 'Comando da Subunidade', 'SUBCMD', 1)`, gid)
	if err != nil {
		t.Fatal(err)
	}
	s2ID, _ := resS2.LastInsertId()
	_ = s2ID

	// Criação de Pessoas (Padrinhos)
	resPadTit, err := st.db.Exec(`INSERT INTO pessoas (grupo_id, setor_id, nome_guerra, nome_completo, status) VALUES (?, ?, 'Sgt Farias', 'Roberto Farias', 'ativo')`, gid, s1ID)
	if err != nil {
		t.Fatal(err)
	}
	padTitID, _ := resPadTit.LastInsertId()

	resPadSub, err := st.db.Exec(`INSERT INTO pessoas (grupo_id, setor_id, nome_guerra, nome_completo, status) VALUES (?, ?, 'Cb Nogueira', 'Marcos Nogueira', 'ativo')`, gid, s1ID)
	if err != nil {
		t.Fatal(err)
	}
	padSubID, _ := resPadSub.LastInsertId()

	// Criação de Usuário Gerente/Operador
	resU, err := st.db.Exec(`INSERT INTO usuarios (login, senha_hash, papel, grupo_id, setor_id, ativo) VALUES ('gerente_mat', 'hash', 'gerente', ?, ?, 1)`, gid, s1ID)
	if err != nil {
		t.Fatal(err)
	}
	uID, _ := resU.LastInsertId()
	_, _ = st.db.Exec(`INSERT INTO usuario_papeis (usuario_id, grupo_id, papel) VALUES (?, ?, 'gerente')`, uID, gid)

	tok, _, err := st.CriarSessao(uID, 2*time.Hour)
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

	var viaturaItemID int64
	var armamentoItemID int64
	var itemSemSetorID int64
	var itemBaixadoID int64

	// TAREFA 1 & 2: Salvar item com viatura e setor_id
	t.Run("1. Salvar item com campos de viatura e setor_id", func(t *testing.T) {
		rec, resp := doReq("POST", "/api/material/itens", map[string]any{
			"nome":                   "Viatura Blindada Guarani",
			"codigo_patrimonio":      "VBR-001",
			"categoria_id":           catViatID,
			"setor_id":               s1ID,
			"sensibilidade":          "controlado",
			"status":                 "disponivel",
			"placa":                  "EB-1001",
			"renavam":                "98765432100",
			"padrinho_titular_id":    padTitID,
			"padrinho_substituto_id": padSubID,
			"hodometro_atual":        5400,
			"combustivel_atual":      "3/4",
		})
		if rec.Code != http.StatusOK || resp["ok"] != true {
			t.Fatalf("falha ao salvar viatura: code=%d resp=%v", rec.Code, resp)
		}
		viaturaItemID = int64(resp["id"].(float64))

		// Verificar persistência no banco em material_viaturas
		var placa, renavam, combustivel string
		var ptID, psID, hodometro int64
		err := st.db.QueryRow(`
			SELECT placa, renavam, padrinho_titular_id, padrinho_substituto_id, hodometro_atual, combustivel_atual
			FROM material_viaturas WHERE item_id = ?`, viaturaItemID).
			Scan(&placa, &renavam, &ptID, &psID, &hodometro, &combustivel)
		if err != nil {
			t.Fatalf("erro ao consultar material_viaturas: %v", err)
		}
		if placa != "EB-1001" || renavam != "98765432100" || ptID != padTitID || psID != padSubID || hodometro != 5400 || combustivel != "3/4" {
			t.Fatalf("dados de viatura divergentes: placa=%s renavam=%s pt=%d ps=%d hod=%d comb=%s", placa, renavam, ptID, psID, hodometro, combustivel)
		}

		// Verificar setor_id em material_itens
		var sidVal int64
		_ = st.db.QueryRow(`SELECT setor_id FROM material_itens WHERE id = ?`, viaturaItemID).Scan(&sidVal)
		if sidVal != s1ID {
			t.Fatalf("esperava setor_id=%d, obteve %d", s1ID, sidVal)
		}
	})

	t.Run("2. Editar item com campos de viatura (upsert sem duplicar)", func(t *testing.T) {
		rec, resp := doReq("POST", "/api/material/itens", map[string]any{
			"id":                     viaturaItemID,
			"nome":                   "Viatura Blindada Guarani - Atualizada",
			"codigo_patrimonio":      "VBR-001",
			"categoria_id":           catViatID,
			"setor_id":               s1ID,
			"sensibilidade":          "controlado",
			"status":                 "disponivel",
			"placa":                  "EB-1001",
			"renavam":                "98765432100",
			"padrinho_titular_id":    padTitID,
			"padrinho_substituto_id": padSubID,
			"hodometro_atual":        6200,
			"combustivel_atual":      "cheio",
		})
		if rec.Code != http.StatusOK || resp["ok"] != true {
			t.Fatalf("falha ao editar viatura: code=%d resp=%v", rec.Code, resp)
		}

		var count int
		_ = st.db.QueryRow(`SELECT COUNT(*) FROM material_viaturas WHERE item_id = ?`, viaturaItemID).Scan(&count)
		if count != 1 {
			t.Fatalf("esperava exatamente 1 registro em material_viaturas, obteve %d", count)
		}

		var hodometro int64
		var combustivel string
		_ = st.db.QueryRow(`SELECT hodometro_atual, combustivel_atual FROM material_viaturas WHERE item_id = ?`, viaturaItemID).Scan(&hodometro, &combustivel)
		if hodometro != 6200 || combustivel != "cheio" {
			t.Fatalf("valores de viatura não atualizados: hod=%d comb=%s", hodometro, combustivel)
		}
	})

	t.Run("3. Salvar item normal sem campos de viatura (sem linha em material_viaturas)", func(t *testing.T) {
		rec, resp := doReq("POST", "/api/material/itens", map[string]any{
			"nome":              "Fuzil de Assalto IA2 5,56mm",
			"codigo_patrimonio": "ARM-101",
			"categoria_id":      catArmID,
			"setor_id":          s1ID,
			"sensibilidade":     "controlado",
			"status":            "disponivel",
		})
		if rec.Code != http.StatusOK || resp["ok"] != true {
			t.Fatalf("falha ao cadastrar armamento: code=%d resp=%v", rec.Code, resp)
		}
		armamentoItemID = int64(resp["id"].(float64))

		var count int
		_ = st.db.QueryRow(`SELECT COUNT(*) FROM material_viaturas WHERE item_id = ?`, armamentoItemID).Scan(&count)
		if count != 0 {
			t.Fatalf("item comum não deve ter registro em material_viaturas (count=%d)", count)
		}
	})

	t.Run("4. Cadastrar item sem setor (Carga Geral) e item baixado", func(t *testing.T) {
		// Item sem setor
		rec1, resp1 := doReq("POST", "/api/material/itens", map[string]any{
			"nome":              "Gerador de Campanha 5kVA",
			"codigo_patrimonio": "GER-01",
			"categoria_id":      catArmID,
			"sensibilidade":     "convencional",
			"quantidade":        2,
			"status":            "disponivel",
		})
		if rec1.Code != http.StatusOK {
			t.Fatalf("falha ao cadastrar item sem setor: code=%d", rec1.Code)
		}
		itemSemSetorID = int64(resp1["id"].(float64))

		// Item baixado
		rec2, resp2 := doReq("POST", "/api/material/itens", map[string]any{
			"nome":              "Rádio Antigo Sucata",
			"codigo_patrimonio": "RAD-SUC",
			"categoria_id":      catArmID,
			"setor_id":          s1ID,
			"sensibilidade":     "controlado",
			"status":            "baixado",
		})
		if rec2.Code != http.StatusOK {
			t.Fatalf("falha ao cadastrar item baixado: code=%d", rec2.Code)
		}
		itemBaixadoID = int64(resp2["id"].(float64))
		_ = itemBaixadoID
	})

	t.Run("5. Validação de FK de setor_id inexistente retorna HTTP 400", func(t *testing.T) {
		rec, _ := doReq("POST", "/api/material/itens", map[string]any{
			"nome":              "Barraca 10 Lugares",
			"codigo_patrimonio": "BAR-01",
			"setor_id":          999999, // inexistente
			"sensibilidade":     "convencional",
		})
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("esperava HTTP 400 para setor inexistente, obteve %d", rec.Code)
		}
	})

	t.Run("6. Listagem de itens: objeto viatura, padrinhos, setor_nome e filtro garagem=1", func(t *testing.T) {
		rec, resp := doReq("GET", "/api/material/itens", nil)
		if rec.Code != http.StatusOK {
			t.Fatalf("falha ao listar itens: code=%d", rec.Code)
		}
		itens, _ := resp["itens"].([]any)
		if len(itens) < 3 {
			t.Fatalf("esperava ao menos 3 itens na listagem, obteve %d", len(itens))
		}

		var achouViatura, achouArmamento, achouSemSetor bool
		for _, raw := range itens {
			it := raw.(map[string]any)
			id := int64(it["id"].(float64))
			if id == viaturaItemID {
				achouViatura = true
				if it["setor_nome"] != "Pelotão de Apoio" {
					t.Fatalf("setor_nome incorreto para viatura: %v", it["setor_nome"])
				}
				viat, ok := it["viatura"].(map[string]any)
				if !ok {
					t.Fatalf("esperava objeto viatura no item de viatura")
				}
				if viat["placa"] != "EB-1001" || viat["padrinho_titular_nome"] != "Sgt Farias" || viat["padrinho_substituto_nome"] != "Cb Nogueira" {
					t.Fatalf("dados do objeto viatura divergentes: %v", viat)
				}
			}
			if id == armamentoItemID {
				achouArmamento = true
				if _, ok := it["viatura"]; ok {
					t.Fatalf("item normal não deve conter chave viatura")
				}
				if it["setor_nome"] != "Pelotão de Apoio" {
					t.Fatalf("setor_nome incorreto para armamento: %v", it["setor_nome"])
				}
			}
			if id == itemSemSetorID {
				achouSemSetor = true
				if it["setor_id"] != nil {
					t.Fatalf("item sem setor deve ter setor_id nil")
				}
				if it["setor_nome"] != "" {
					t.Fatalf("item sem setor deve ter setor_nome vazio, obteve %v", it["setor_nome"])
				}
			}
		}
		if !achouViatura || !achouArmamento || !achouSemSetor {
			t.Fatalf("nem todos os itens esperados foram encontrados na listagem")
		}

		// Teste com ?garagem=1
		recG, respG := doReq("GET", "/api/material/itens?garagem=1", nil)
		if recG.Code != http.StatusOK {
			t.Fatalf("falha ao filtrar garagem: code=%d", recG.Code)
		}
		itensG, _ := respG["itens"].([]any)
		if len(itensG) != 1 {
			t.Fatalf("esperava exatamente 1 item na garagem, obteve %d", len(itensG))
		}
		itG := itensG[0].(map[string]any)
		if int64(itG["id"].(float64)) != viaturaItemID {
			t.Fatalf("item retornado na garagem não é a viatura esperada")
		}
	})

	var confSetorID int64
	var confCargaGeralID int64

	t.Run("7. Iniciar conferência de setor e carga geral com checklist automático", func(t *testing.T) {
		// Conferência do setor PELAP (s1ID)
		rec1, resp1 := doReq("POST", "/api/material/conferencias/iniciar", map[string]any{
			"setor_id": s1ID,
			"data":     "2026-10-15",
		})
		if rec1.Code != http.StatusOK || resp1["ok"] != true {
			t.Fatalf("falha ao iniciar conferência do setor: code=%d resp=%v", rec1.Code, resp1)
		}
		confSetorID = int64(resp1["id"].(float64))

		// Verificar que material_conferencia_itens nasceu com os itens do setor (viatura e armamento), e SEM o baixado
		var countSetorItens int
		_ = st.db.QueryRow(`SELECT COUNT(*) FROM material_conferencia_itens WHERE conferencia_id = ?`, confSetorID).Scan(&countSetorItens)
		if countSetorItens != 2 {
			t.Fatalf("esperava 2 itens no checklist do setor (excluindo baixado), obteve %d", countSetorItens)
		}

		// Checar detalhes via GET
		recGet, respGet := doReq("GET", "/api/material/conferencias/"+testIntToStr(confSetorID), nil)
		if recGet.Code != http.StatusOK {
			t.Fatalf("falha ao obter dados da conferência: code=%d", recGet.Code)
		}
		if respGet["setor_nome"] != "Pelotão de Apoio" {
			t.Fatalf("setor_nome da conferência divergente: %v", respGet["setor_nome"])
		}
		confItens, _ := respGet["itens"].([]any)
		if len(confItens) != 2 {
			t.Fatalf("esperava 2 itens no GET da conferência, obteve %d", len(confItens))
		}

		// Conferência de Carga Geral (setor_id = 0)
		rec2, resp2 := doReq("POST", "/api/material/conferencias/iniciar", map[string]any{
			"setor_id": 0,
			"data":     "2026-10-15",
		})
		if rec2.Code != http.StatusOK || resp2["ok"] != true {
			t.Fatalf("falha ao iniciar conferência de carga geral: code=%d resp=%v", rec2.Code, resp2)
		}
		confCargaGeralID = int64(resp2["id"].(float64))

		var countGeralItens int
		_ = st.db.QueryRow(`SELECT COUNT(*) FROM material_conferencia_itens WHERE conferencia_id = ?`, confCargaGeralID).Scan(&countGeralItens)
		if countGeralItens != 1 {
			t.Fatalf("esperava 1 item na conferência de carga geral (o gerador sem setor), obteve %d", countGeralItens)
		}

		// Duplicar conferência aberta no mesmo grupo/setor/data -> HTTP 409
		recDup, _ := doReq("POST", "/api/material/conferencias/iniciar", map[string]any{
			"setor_id": s1ID,
			"data":     "2026-10-15",
		})
		if recDup.Code != http.StatusConflict {
			t.Fatalf("esperava HTTP 409 ao duplicar conferência aberta, obteve %d", recDup.Code)
		}
	})

	t.Run("8. Emissão de PDF do Pronto de Material com gofpdf", func(t *testing.T) {
		// Bipar um item da conferência
		recB, respB := doReq("POST", "/api/material/conferencias/"+testIntToStr(confSetorID)+"/bipar", map[string]any{
			"codigo_patrimonio":    "VBR-001",
			"status":               "presente",
			"quantidade_conferida": 1,
			"observacao":           "Viatura em perfeitas condições operacionais",
		})
		if recB.Code != http.StatusOK || respB["ok"] != true {
			t.Fatalf("falha ao bipar item: code=%d resp=%v", recB.Code, respB)
		}

		// Obter Pronto em PDF
		recPDF, _ := doReq("GET", "/api/material/conferencias/"+testIntToStr(confSetorID)+"/pronto.pdf", nil)
		if recPDF.Code != http.StatusOK {
			t.Fatalf("esperava HTTP 200 no PDF do pronto, obteve %d", recPDF.Code)
		}
		if recPDF.Header().Get("Content-Type") != "application/pdf" {
			t.Fatalf("Content-Type esperado 'application/pdf', obteve '%s'", recPDF.Header().Get("Content-Type"))
		}

		pdfBytes := recPDF.Body.Bytes()
		if len(pdfBytes) < 1024 {
			t.Fatalf("PDF gerado é menor que 1 KB (tamanho = %d bytes)", len(pdfBytes))
		}
		if !bytes.HasPrefix(pdfBytes, []byte("%PDF")) {
			t.Fatalf("arquivo gerado não inicia com cabeçalho %%PDF válido")
		}
	})
}
