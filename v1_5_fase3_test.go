package main

import (
	"fmt"
	"net/http"
	"strings"
	"testing"
)

func TestV15_Fase3_EtiquetasMaterialLote(t *testing.T) {
	app, _, cleanup := setupTestApp(t)
	defer cleanup()

	admin := loginAs(t, app, "admin", "admin123")

	// Criar grupo e gerente
	_, resG := doJSONReq(app, "POST", "/api/grupos", map[string]any{
		"nome": "Companhia Logística", "login": "ger_log", "senha": "senha12345", "nome_guerra": "Cap Log",
	}, admin)
	gid := int64(resG["id"].(float64))

	ger := loginAs(t, app, "ger_log", "senha12345")

	// Criar categoria de material
	resCat, errCat := app.st.db.Exec(`INSERT INTO material_categorias (nome, grupo_id) VALUES ('Armamento Leve', ?)`, gid)
	if errCat != nil {
		t.Fatalf("falha ao criar categoria: %v", errCat)
	}
	catID, _ := resCat.LastInsertId()

	// Inserir 12 itens no banco para testar lote e paginação (10 por folha A4)
	var ids []string
	var itensEtq []MaterialItemEtiqueta
	for i := 1; i <= 12; i++ {
		cod := fmt.Sprintf("FAL-762-%03d", i)
		nome := fmt.Sprintf("Fuzil Automático Leve M964 #%02d", i)
		resItem, err := app.st.db.Exec(`
			INSERT INTO material_itens (nome, codigo_patrimonio, categoria_id, grupo_id, numero_serie, nivel_sensibilidade, tipo_material, classe_material, status)
			VALUES (?, ?, ?, ?, ?, 'restrito', 'Bélico', 'Classe V', 'disponivel')`,
			nome, cod, catID, gid, fmt.Sprintf("SER-%d", 1000+i))
		if err != nil {
			t.Fatalf("erro ao inserir item %d: %v", i, err)
		}
		id, _ := resItem.LastInsertId()
		ids = append(ids, int64ToStr(id))
		itensEtq = append(itensEtq, MaterialItemEtiqueta{
			ID:                 id,
			Nome:               nome,
			CodigoPatrimonio:   cod,
			CategoriaNome:      "Armamento Leve",
			NumeroSerie:        fmt.Sprintf("SER-%d", 1000+i),
			NivelSensibilidade: "restrito",
			TipoMaterial:       "Bélico",
			ClasseMaterial:     "Classe V",
		})
	}

	// 1. Testar endpoint HTTP GET /api/material/etiquetas-lote.pdf
	urlLote := "/api/material/etiquetas-lote.pdf?ids=" + strings.Join(ids, ",")
	rec, _ := doJSONReq(app, "GET", urlLote, nil, ger)
	if rec.Code != http.StatusOK {
		t.Fatalf("esperava 200 OK na rota de etiquetas em lote, obteve %d - corpo: %s", rec.Code, rec.Body.String())
	}
	if rec.Header().Get("Content-Type") != "application/pdf" {
		t.Fatalf("esperava Content-Type application/pdf, obteve %s", rec.Header().Get("Content-Type"))
	}
	if len(rec.Body.Bytes()) < 1000 {
		t.Fatalf("PDF gerado muito pequeno ou corrompido: %d bytes", len(rec.Body.Bytes()))
	}

	// 2. Testar chamada direta do gerador de 10 por folha A4
	pdfBytes, err := app.gerarEtiquetasLotePDF(itensEtq)
	if err != nil {
		t.Fatalf("erro ao gerar etiquetas em lote PDF: %v", err)
	}
	if len(pdfBytes) < 1000 {
		t.Fatalf("esperava PDF substancial, obteve %d bytes", len(pdfBytes))
	}

	// Validar assinatura inicial do PDF (%PDF-)
	if !strings.HasPrefix(string(pdfBytes[:8]), "%PDF-") {
		t.Fatalf("conteúdo retornado não inicia com cabeçalho PDF válido: %s", string(pdfBytes[:8]))
	}
}
