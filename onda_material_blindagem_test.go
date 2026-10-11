package main

import (
	"encoding/base64"
	"fmt"
	"net/http"
	"regexp"
	"strings"
	"testing"
)

// setupPersonasMaterial cria Grupos A e B com suas respectivas personas:
// Admin, Gerente A, Gerente B, Encarregado de Material A, Operador A, Chefe de Setor A.
func setupPersonasMaterial(t *testing.T, app *App, st *Store) (
	adminCk *http.Cookie,
	gidA int64, gerACk *http.Cookie,
	gidB int64, gerBCk *http.Cookie,
	encACk *http.Cookie,
	opACk *http.Cookie,
	chefeACk *http.Cookie,
) {
	t.Helper()
	adminCk = loginAs(t, app, "admin", "admin123")
	gidA, gerACk = criaGrupo(t, app, adminCk, "Grupo Blindagem A", "ger_mat_a")
	gidB, gerBCk = criaGrupo(t, app, adminCk, "Grupo Blindagem B", "ger_mat_b")

	// Resolver função enc_material
	var fMat int64
	if err := st.db.QueryRow(`SELECT id FROM funcoes WHERE chave = 'enc_material'`).Scan(&fMat); err != nil {
		t.Fatalf("buscar funcao enc_material: %v", err)
	}

	// Criar encarregado de material do Grupo A
	criaUsuarioTeste(t, st, "enc_mat_a", "senha123", "")
	var uidEnc int64
	_ = st.db.QueryRow(`SELECT id FROM usuarios WHERE login='enc_mat_a'`).Scan(&uidEnc)
	if _, err := st.db.Exec(`UPDATE usuarios SET grupo_id = ? WHERE id = ?`, gidA, uidEnc); err != nil {
		t.Fatalf("atualizar grupo_id de enc_mat_a: %v", err)
	}
	if _, err := st.db.Exec(`INSERT INTO funcao_membros (funcao_id, grupo_id, usuario_id, titularidade) VALUES (?, ?, ?, 'titular')`, fMat, gidA, uidEnc); err != nil {
		t.Fatalf("designar enc_mat_a: %v", err)
	}
	// v1.6.0 Fase 2: enc_mat agora é CONTEXTO — a designação semeda é
	// MATERIALIZADA pela v45 e o login resolve a linha (papel='enc_material';
	// poderes do material iguais, pessoais/conferência seguem fechados).
	v45Reexecuta(t, st)
	encACk = loginAs(t, app, "enc_mat_a", "senha123")

	// Criar operador do Grupo A
	criaUsuarioTeste(t, st, "op_mat_a", "senha123", "operador")
	var uidOp int64
	_ = st.db.QueryRow(`SELECT id FROM usuarios WHERE login='op_mat_a'`).Scan(&uidOp)
	if _, err := st.db.Exec(`UPDATE usuarios SET grupo_id = ? WHERE id = ?`, gidA, uidOp); err != nil {
		t.Fatalf("atualizar grupo_id de op_mat_a: %v", err)
	}
	opACk = loginAs(t, app, "op_mat_a", "senha123")

	// Criar setor e chefe_setor no Grupo A
	resSet, err := st.db.Exec(`INSERT INTO setores (nome, grupo_id) VALUES ('Setor Blindagem A', ?)`, gidA)
	if err != nil {
		t.Fatalf("criar setor: %v", err)
	}
	setorID, _ := resSet.LastInsertId()

	criaUsuarioTeste(t, st, "chefe_mat_a", "senha123", "chefe_setor")
	var uidChefe int64
	_ = st.db.QueryRow(`SELECT id FROM usuarios WHERE login='chefe_mat_a'`).Scan(&uidChefe)
	if _, err := st.db.Exec(`UPDATE usuarios SET grupo_id = ?, setor_id = ? WHERE id = ?`, gidA, setorID, uidChefe); err != nil {
		t.Fatalf("atualizar grupo/setor chefe_mat_a: %v", err)
	}
	chefeACk = loginAs(t, app, "chefe_mat_a", "senha123")

	return
}

// TestMaterialBlindagem_AnexosItemEscopoEValidacoes testa:
// - Grupo B NÃO lista anexo de item do grupo A (403)
// - GET/DELETE por id alheio retorna 403
// - ADD com mime inválido (text/html) retorna 400
// - ADD com path traversal sanitiza nome
// - Item/anexo inexistente retorna 404
func TestMaterialBlindagem_AnexosItemEscopoEValidacoes(t *testing.T) {
	app, st, cleanup := setupTestApp(t)
	defer cleanup()

	adminCk, gidA, gerACk, _, gerBCk, encACk, _, _ := setupPersonasMaterial(t, app, st)
	_ = adminCk

	// Categoria e Item no Grupo A
	var catIDA int64
	if err := st.db.QueryRow(`INSERT INTO material_categorias (nome, grupo_id) VALUES ('Cat A', ?) RETURNING id`, gidA).Scan(&catIDA); err != nil {
		t.Fatalf("criar categoria A: %v", err)
	}

	rrItem, resItem := doJSONReq(app, "POST", "/api/material/itens", map[string]any{
		"nome":              "Fuzil Grupo A",
		"categoria_id":      catIDA,
		"codigo_patrimonio": "FAL-A-001",
		"status":            "disponivel",
		"sensibilidade":     "controlado",
	}, gerACk)
	if rrItem.Code != http.StatusOK {
		t.Fatalf("criar item A: %d (%v)", rrItem.Code, resItem)
	}
	itemIDA := int64(resItem["id"].(float64))

	// 1. ADD com MIME inválido -> 400
	rrAddBadMime, _ := doJSONReq(app, "POST", fmt.Sprintf("/api/material/itens/%d/anexos", itemIDA), map[string]any{
		"nome_arquivo": "script.html",
		"tipo_mime":    "text/html",
		"tamanho":      10,
		"dados_base64": base64.StdEncoding.EncodeToString([]byte("<h1>test</h1>")),
	}, gerACk)
	if rrAddBadMime.Code != http.StatusBadRequest {
		t.Fatalf("add anexo mime text/html esperado 400, veio %d", rrAddBadMime.Code)
	}

	// 2. ADD com nome com path traversal -> sanitizado gravado com sucesso
	b64Png := base64.StdEncoding.EncodeToString([]byte("\x89PNG\r\n\x1a\n"))
	rrAddSanitize, resAddSanitize := doJSONReq(app, "POST", fmt.Sprintf("/api/material/itens/%d/anexos", itemIDA), map[string]any{
		"nome_arquivo": "../../etc/passwd.png",
		"tipo_mime":    "image/png",
		"tamanho":      len(b64Png),
		"dados_base64": b64Png,
	}, encACk)
	if rrAddSanitize.Code != http.StatusOK {
		t.Fatalf("add anexo enc_mat_a esperado 200, veio %d (%v)", rrAddSanitize.Code, resAddSanitize)
	}
	anexoIDA := int64(resAddSanitize["id"].(float64))

	// Verificar nome sanitizado no banco
	var nomeGravado string
	if err := st.db.QueryRow(`SELECT nome_arquivo FROM material_item_anexos WHERE id = ?`, anexoIDA).Scan(&nomeGravado); err != nil {
		t.Fatalf("consultar anexo gravado: %v", err)
	}
	if nomeGravado == "../../etc/passwd.png" || regexp.MustCompile(`[/\\.]+/`).MatchString(nomeGravado) {
		t.Fatalf("nome_arquivo não foi devidamente sanitizado: %q", nomeGravado)
	}

	// 3. Grupo B tentando listar anexos do item do Grupo A -> 403
	rrListB, _ := doJSONReq(app, "GET", fmt.Sprintf("/api/material/itens/%d/anexos", itemIDA), nil, gerBCk)
	if rrListB.Code != http.StatusForbidden {
		t.Fatalf("gerB listar anexos item A esperado 403, veio %d", rrListB.Code)
	}

	// 4. Grupo B tentando GET do anexo por ID -> 403
	rrGetB, _ := doJSONReq(app, "GET", fmt.Sprintf("/api/material/item-anexos/%d", anexoIDA), nil, gerBCk)
	if rrGetB.Code != http.StatusForbidden {
		t.Fatalf("gerB GET anexo A esperado 403, veio %d", rrGetB.Code)
	}

	// 5. Grupo B tentando DELETE do anexo por ID -> 403
	rrDelB, _ := doJSONReq(app, "DELETE", fmt.Sprintf("/api/material/item-anexos/%d", anexoIDA), nil, gerBCk)
	if rrDelB.Code != http.StatusForbidden {
		t.Fatalf("gerB DELETE anexo A esperado 403, veio %d", rrDelB.Code)
	}

	// 6. Grupo A (dono) lista com sucesso -> 200
	rrListA, resListA := doJSONReq(app, "GET", fmt.Sprintf("/api/material/itens/%d/anexos", itemIDA), nil, gerACk)
	if rrListA.Code != http.StatusOK {
		t.Fatalf("gerA listar anexos item A esperado 200, veio %d (%v)", rrListA.Code, resListA)
	}

	// 7. Grupo A (encarregado) GET do anexo -> 200
	rrGetA, _ := doJSONReq(app, "GET", fmt.Sprintf("/api/material/item-anexos/%d", anexoIDA), nil, encACk)
	if rrGetA.Code != http.StatusOK {
		t.Fatalf("encA GET anexo A esperado 200, veio %d", rrGetA.Code)
	}

	// 8. 404 em item inexistente / anexo inexistente
	rrInexItem, _ := doJSONReq(app, "GET", "/api/material/itens/999999/anexos", nil, gerACk)
	if rrInexItem.Code != http.StatusNotFound {
		t.Fatalf("item inexistente list anexos esperado 404, veio %d", rrInexItem.Code)
	}
	rrInexAnexo, _ := doJSONReq(app, "GET", "/api/material/item-anexos/999999", nil, gerACk)
	if rrInexAnexo.Code != http.StatusNotFound {
		t.Fatalf("anexo inexistente get esperado 404, veio %d", rrInexAnexo.Code)
	}

	// 9. Grupo A exclui anexo -> 200
	rrDelA, _ := doJSONReq(app, "DELETE", fmt.Sprintf("/api/material/item-anexos/%d", anexoIDA), nil, gerACk)
	if rrDelA.Code != http.StatusOK {
		t.Fatalf("gerA DELETE anexo esperado 200, veio %d", rrDelA.Code)
	}
}

// TestMaterialBlindagem_ComentariosItemEscopo testa:
// - Comentário em item do grupo alheio -> 403
// - Listagem de comentários de item alheio -> 403
// - Gerente e encarregado do próprio grupo comentam e listam -> 200
func TestMaterialBlindagem_ComentariosItemEscopo(t *testing.T) {
	app, st, cleanup := setupTestApp(t)
	defer cleanup()

	_, gidA, gerACk, _, gerBCk, encACk, _, _ := setupPersonasMaterial(t, app, st)

	var catIDA int64
	if err := st.db.QueryRow(`INSERT INTO material_categorias (nome, grupo_id) VALUES ('Cat A Coment', ?) RETURNING id`, gidA).Scan(&catIDA); err != nil {
		t.Fatalf("criar categoria A: %v", err)
	}

	rrItem, resItem := doJSONReq(app, "POST", "/api/material/itens", map[string]any{
		"nome":              "Item A Comentarios",
		"categoria_id":      catIDA,
		"codigo_patrimonio": "COM-A-001",
		"status":            "disponivel",
	}, gerACk)
	if rrItem.Code != http.StatusOK {
		t.Fatalf("criar item A: %d", rrItem.Code)
	}
	itemIDA := int64(resItem["id"].(float64))

	// Grupo B tenta comentar em item do Grupo A -> 403
	rrComB, _ := doJSONReq(app, "POST", fmt.Sprintf("/api/material/itens/%d/comentarios", itemIDA), map[string]any{
		"texto": "Tentativa de comentário invasor",
	}, gerBCk)
	if rrComB.Code != http.StatusForbidden {
		t.Fatalf("gerB comentar item A esperado 403, veio %d", rrComB.Code)
	}

	// Grupo B tenta listar comentários de item do Grupo A -> 403
	rrListB, _ := doJSONReq(app, "GET", fmt.Sprintf("/api/material/itens/%d/comentarios", itemIDA), nil, gerBCk)
	if rrListB.Code != http.StatusForbidden {
		t.Fatalf("gerB listar comentários item A esperado 403, veio %d", rrListB.Code)
	}

	// enc_mat_a comenta -> 200
	rrComEncA, _ := doJSONReq(app, "POST", fmt.Sprintf("/api/material/itens/%d/comentarios", itemIDA), map[string]any{
		"texto": "Comentário do encarregado de material A",
	}, encACk)
	if rrComEncA.Code != http.StatusOK {
		t.Fatalf("encA comentar item A esperado 200, veio %d", rrComEncA.Code)
	}

	// gerA lista comentários -> 200
	rrListA, resListA := doJSONReq(app, "GET", fmt.Sprintf("/api/material/itens/%d/comentarios", itemIDA), nil, gerACk)
	if rrListA.Code != http.StatusOK {
		t.Fatalf("gerA listar comentários item A esperado 200, veio %d (%v)", rrListA.Code, resListA)
	}
}

// TestMaterialBlindagem_ConferenciaEscopoEUnicidadeEData testa:
// - Data gravada confere ^\d{4}-\d{2}-\d{2}$
// - Iniciar conferência com grupo alheio -> 403
// - Iniciar conferência duplicada no mesmo grupo/setor/data (aberta) -> 409
// - Bipar / fechar conferência do grupo alheio -> 403
// - Fechar e reabrir após fechamento funciona
func TestMaterialBlindagem_ConferenciaEscopoEUnicidadeEData(t *testing.T) {
	app, st, cleanup := setupTestApp(t)
	defer cleanup()

	_, gidA, gerACk, gidB, gerBCk, encACk, _, _ := setupPersonasMaterial(t, app, st)

	var catIDA int64
	if err := st.db.QueryRow(`INSERT INTO material_categorias (nome, grupo_id) VALUES ('Cat Conf', ?) RETURNING id`, gidA).Scan(&catIDA); err != nil {
		t.Fatalf("criar categoria: %v", err)
	}

	rrItem, resItem := doJSONReq(app, "POST", "/api/material/itens", map[string]any{
		"nome":              "Item Conf 1",
		"categoria_id":      catIDA,
		"codigo_patrimonio": "CONF-001",
		"status":            "disponivel",
	}, gerACk)
	if rrItem.Code != http.StatusOK {
		t.Fatalf("criar item: %d", rrItem.Code)
	}
	_ = resItem

	// 1. Gerente A tenta iniciar conferência forjando grupo B -> 403
	rrForja, _ := doJSONReq(app, "POST", "/api/material/conferencias/iniciar", map[string]any{
		"grupo_id": gidB,
	}, gerACk)
	if rrForja.Code != http.StatusForbidden {
		t.Fatalf("iniciar conferencia grupo alheio esperado 403, veio %d", rrForja.Code)
	}

	// 2. Gerente A inicia conferência válida do Grupo A -> 200
	rrIniA, resIniA := doJSONReq(app, "POST", "/api/material/conferencias/iniciar", map[string]any{
		"grupo_id": gidA,
	}, gerACk)
	if rrIniA.Code != http.StatusOK {
		t.Fatalf("iniciar conferencia grupo A esperado 200, veio %d (%v)", rrIniA.Code, resIniA)
	}
	confIDA := int64(resIniA["id"].(float64))

	// 3. Verificar formato da data gravada ^\d{4}-\d{2}-\d{2}$
	var dataGravada string
	if err := st.db.QueryRow(`SELECT data FROM material_conferencias WHERE id = ?`, confIDA).Scan(&dataGravada); err != nil {
		t.Fatalf("consultar data conferencia: %v", err)
	}
	matchDate, err := regexp.MatchString(`^\d{4}-\d{2}-\d{2}$`, dataGravada)
	if err != nil || !matchDate {
		t.Fatalf("data gravada não está no formato YYYY-MM-DD: %q", dataGravada)
	}

	// 4. Iniciar conferência DUPLICADA (mesmo grupo, mesma data, status aberta) -> 409
	rrDup, _ := doJSONReq(app, "POST", "/api/material/conferencias/iniciar", map[string]any{
		"grupo_id": gidA,
	}, encACk)
	if rrDup.Code != http.StatusConflict {
		t.Fatalf("conferencia duplicada aberta esperado 409, veio %d", rrDup.Code)
	}

	// 5. Grupo B tenta GET / BIPAR / FECHAR conferência do Grupo A -> 403
	rrGetConfB, _ := doJSONReq(app, "GET", fmt.Sprintf("/api/material/conferencias/%d", confIDA), nil, gerBCk)
	if rrGetConfB.Code != http.StatusForbidden {
		t.Fatalf("gerB GET conf A esperado 403, veio %d", rrGetConfB.Code)
	}

	rrBiparB, _ := doJSONReq(app, "POST", fmt.Sprintf("/api/material/conferencias/%d/bipar", confIDA), map[string]any{
		"codigo_patrimonio": "CONF-001",
	}, gerBCk)
	if rrBiparB.Code != http.StatusForbidden {
		t.Fatalf("gerB bipar conf A esperado 403, veio %d", rrBiparB.Code)
	}

	rrFecharB, _ := doJSONReq(app, "POST", fmt.Sprintf("/api/material/conferencias/%d/fechar", confIDA), nil, gerBCk)
	if rrFecharB.Code != http.StatusForbidden {
		t.Fatalf("gerB fechar conf A esperado 403, veio %d", rrFecharB.Code)
	}

	// 6. enc_mat_a bipa item -> 200
	rrBiparA, _ := doJSONReq(app, "POST", fmt.Sprintf("/api/material/conferencias/%d/bipar", confIDA), map[string]any{
		"codigo_patrimonio": "CONF-001",
	}, encACk)
	if rrBiparA.Code != http.StatusOK {
		t.Fatalf("encA bipar conf A esperado 200, veio %d", rrBiparA.Code)
	}

	// 7. enc_mat_a fecha conferência -> 200
	rrFecharA, _ := doJSONReq(app, "POST", fmt.Sprintf("/api/material/conferencias/%d/fechar", confIDA), nil, encACk)
	if rrFecharA.Code != http.StatusOK {
		t.Fatalf("encA fechar conf A esperado 200, veio %d", rrFecharA.Code)
	}
}

// TestMaterialBlindagem_ResponsaveisPortaDePapelEEscopo testa:
// - Operador comum do Grupo A -> 403
// - Chefe de setor do Grupo A -> 403
// - Gerente do Grupo A define responsável no Grupo A -> 200
// - Gerente do Grupo A tenta definir responsável no Grupo B -> 403
// - Encarregado de material define responsável no próprio grupo -> 200
func TestMaterialBlindagem_ResponsaveisPortaDePapelEEscopo(t *testing.T) {
	app, st, cleanup := setupTestApp(t)
	defer cleanup()

	_, gidA, gerACk, gidB, _, encACk, opACk, chefeACk := setupPersonasMaterial(t, app, st)

	// Pessoa no Grupo A para ser encarregado
	resPes, err := st.db.Exec(`INSERT INTO pessoas (nome_guerra, nome_completo, grupo_id, status) VALUES ('RESPA', 'Responsavel A', ?, 'ativo')`, gidA)
	if err != nil {
		t.Fatalf("criar pessoa: %v", err)
	}
	pesIDA, _ := resPes.LastInsertId()

	// 1. Operador comum tenta salvar responsáveis -> 403
	rrOp, _ := doJSONReq(app, "POST", "/api/material/responsaveis", map[string]any{
		"grupo_id":               gidA,
		"encarregado_id":         pesIDA,
		"auxiliar_encarregado_id": nil,
	}, opACk)
	if rrOp.Code != http.StatusForbidden {
		t.Fatalf("operador salvar responsaveis esperado 403, veio %d", rrOp.Code)
	}

	// 2. Chefe de setor tenta salvar responsáveis -> 403
	rrChefe, _ := doJSONReq(app, "POST", "/api/material/responsaveis", map[string]any{
		"grupo_id":       gidA,
		"encarregado_id": pesIDA,
	}, chefeACk)
	if rrChefe.Code != http.StatusForbidden {
		t.Fatalf("chefe_setor salvar responsaveis esperado 403, veio %d", rrChefe.Code)
	}

	// 3. Gerente do Grupo A tenta salvar responsáveis no Grupo B -> 403
	rrGerForja, _ := doJSONReq(app, "POST", "/api/material/responsaveis", map[string]any{
		"grupo_id":       gidB,
		"encarregado_id": pesIDA,
	}, gerACk)
	if rrGerForja.Code != http.StatusForbidden {
		t.Fatalf("gerente salvar responsaveis grupo alheio esperado 403, veio %d", rrGerForja.Code)
	}

	// 4. Gerente do Grupo A define responsáveis no Grupo A -> 200
	rrGerA, _ := doJSONReq(app, "POST", "/api/material/responsaveis", map[string]any{
		"grupo_id":       gidA,
		"encarregado_id": pesIDA,
	}, gerACk)
	if rrGerA.Code != http.StatusOK {
		t.Fatalf("gerente salvar responsaveis grupo A esperado 200, veio %d", rrGerA.Code)
	}

	// 5. Encarregado de material define responsáveis no próprio grupo -> 200
	rrEncA, _ := doJSONReq(app, "POST", "/api/material/responsaveis", map[string]any{
		"grupo_id":       gidA,
		"encarregado_id": pesIDA,
	}, encACk)
	if rrEncA.Code != http.StatusOK {
		t.Fatalf("enc_material salvar responsaveis próprio grupo esperado 200, veio %d", rrEncA.Code)
	}
}

// TestMaterialBlindagem_CategoriasEscopo testa:
// - Categoria global editada por gerente -> 403
// - Categoria global excluída por gerente -> 403
// - Categoria do grupo A editada por gerente B -> 403
// - Categoria do próprio grupo editada por gerente A -> 200
// - Admin edita categoria global -> 200
func TestMaterialBlindagem_CategoriasEscopo(t *testing.T) {
	app, st, cleanup := setupTestApp(t)
	defer cleanup()

	adminCk, gidA, gerACk, _, gerBCk, _, _, _ := setupPersonasMaterial(t, app, st)
	_ = adminCk

	// Inserir categoria global
	var catGlobalID int64
	if err := st.db.QueryRow(`INSERT INTO material_categorias (nome, grupo_id) VALUES ('Global Teste', NULL) RETURNING id`).Scan(&catGlobalID); err != nil {
		t.Fatalf("criar cat global: %v", err)
	}

	// Inserir categoria Grupo A
	var catGrupoAID int64
	if err := st.db.QueryRow(`INSERT INTO material_categorias (nome, grupo_id) VALUES ('Cat Exclusiva A', ?) RETURNING id`, gidA).Scan(&catGrupoAID); err != nil {
		t.Fatalf("criar cat grupo A: %v", err)
	}

	// 1. Gerente A tenta editar categoria global -> 403
	rrEditGlobGer, _ := doJSONReq(app, "POST", "/api/material/categorias", map[string]any{
		"id":   catGlobalID,
		"nome": "Global Modificada por Gerente",
	}, gerACk)
	if rrEditGlobGer.Code != http.StatusForbidden {
		t.Fatalf("gerente editar categoria global esperado 403, veio %d", rrEditGlobGer.Code)
	}

	// 2. Gerente A tenta excluir categoria global -> 403
	rrDelGlobGer, _ := doJSONReq(app, "DELETE", fmt.Sprintf("/api/material/categorias/%d", catGlobalID), nil, gerACk)
	if rrDelGlobGer.Code != http.StatusForbidden {
		t.Fatalf("gerente excluir categoria global esperado 403, veio %d", rrDelGlobGer.Code)
	}

	// 3. Gerente B tenta editar categoria do Grupo A -> 403
	rrEditAGerB, _ := doJSONReq(app, "POST", "/api/material/categorias", map[string]any{
		"id":   catGrupoAID,
		"nome": "Cat A Modificada por B",
	}, gerBCk)
	if rrEditAGerB.Code != http.StatusForbidden {
		t.Fatalf("gerB editar categoria A esperado 403, veio %d", rrEditAGerB.Code)
	}

	// 4. Gerente B tenta excluir categoria do Grupo A -> 403
	rrDelAGerB, _ := doJSONReq(app, "DELETE", fmt.Sprintf("/api/material/categorias/%d", catGrupoAID), nil, gerBCk)
	if rrDelAGerB.Code != http.StatusForbidden {
		t.Fatalf("gerB excluir categoria A esperado 403, veio %d", rrDelAGerB.Code)
	}

	// 5. Gerente A edita categoria do próprio grupo -> 200
	rrEditAGerA, _ := doJSONReq(app, "POST", "/api/material/categorias", map[string]any{
		"id":   catGrupoAID,
		"nome": "Cat A Modificada por Dono",
	}, gerACk)
	if rrEditAGerA.Code != http.StatusOK {
		t.Fatalf("gerA editar categoria A esperado 200, veio %d", rrEditAGerA.Code)
	}
}

// TestMaterialBlindagem_ChefeSetorRecorte testa a doutrina de NÍVEL da onda
// v1.6.0-contextos (o chefe de setor ENTROU no authMaterial com setor —
// checkpoint do comando; o antigo "chefe 403 no material" virou acesso
// RECORTE):
//   - leituras do módulo → 200 com recorte (só o próprio setor na lista);
//   - save do próprio setor → 200 (nasce no próprio, corpo com setor alheio
//     é ignorado); item de outro setor não é alcançado (404 honesto do P1-2);
//   - Del/baixa patrimonial → 403 "exclusão e baixa de material é ato do
//     gerente ou encarregado de material" (mesmo no próprio setor);
//   - designação de responsáveis segue 403 (gerente/enc_material/admin);
//   - gerente não regride: exclui item de qualquer setor (200).
func TestMaterialBlindagem_ChefeSetorRecorte(t *testing.T) {
	app, st, cleanup := setupTestApp(t)
	defer cleanup()

	_, gidA, gerACk, _, _, _, _, chefeACk := setupPersonasMaterial(t, app, st)

	// Materializar o COMANDO do chefe (fonte única chefe_setores, v1.5.4-D1) —
	// o setup cria papel + usuarios.setor_id; a linha de comando é a doutrina.
	var idChefe, setorChefe int64
	if err := st.db.QueryRow(`SELECT id, setor_id FROM usuarios WHERE login = 'chefe_mat_a'`).Scan(&idChefe, &setorChefe); err != nil {
		t.Fatalf("ler chefe_mat_a: %v", err)
	}
	materializaComandoSetor(t, st, idChefe, gidA, setorChefe)

	// Item em OUTRO setor do MESMO grupo (fora do recorte do chefe)
	var outroSetor int64
	if err := st.db.QueryRow(`INSERT INTO setores (nome, grupo_id) VALUES ('Setor Blindagem Outro', ?) RETURNING id`, gidA).Scan(&outroSetor); err != nil {
		t.Fatalf("criar setor alheio: %v", err)
	}
	var itemAlheio int64
	if err := st.db.QueryRow(`INSERT INTO material_itens (grupo_id, setor_id, nome, codigo_patrimonio, status, quantidade) VALUES (?, ?, 'Fuzil Setor Outro', 'BLD-OUT-1', 'disponivel', 1) RETURNING id`, gidA, outroSetor).Scan(&itemAlheio); err != nil {
		t.Fatalf("criar item alheio: %v", err)
	}

	// 1. Leituras → 200 (antes eram 403 no gate)
	for _, r := range []struct{ metodo, path string }{
		{"GET", "/api/material/itens"},
		{"GET", "/api/material/cautelas"},
		{"GET", "/api/material/conferencias"},
		{"GET", "/api/material/categorias"},
	} {
		rr, _ := doJSONReq(app, r.metodo, r.path, nil, chefeACk)
		if rr.Code != http.StatusOK {
			t.Errorf("chefe_setor em %s %s: esperado 200 (recorte), veio %d", r.metodo, r.path, rr.Code)
		}
	}

	// 2. Recorte na lista: só o próprio setor (vazio aqui — nenhum item no setor
	// do chefe ainda); o item alheio NUNCA aparece.
	rrList, resList := doJSONReq(app, "GET", "/api/material/itens", nil, chefeACk)
	if rrList.Code != http.StatusOK {
		t.Fatalf("chefe listar itens: esperado 200, veio %d", rrList.Code)
	}
	listaChefe, _ := resList["itens"].([]any)
	for _, it := range listaChefe {
		if m, ok := it.(map[string]any); ok && int64(m["id"].(float64)) == itemAlheio {
			t.Fatalf("chefe NÃO devia ver item de outro setor (%v)", m["id"])
		}
	}

	// 3. Save do próprio setor → 200 (corpo com setor alheio é ignorado)
	rrSave, resSave := doJSONReq(app, "POST", "/api/material/itens", map[string]any{
		"nome":              "Item do Chefe Blindagem",
		"codigo_patrimonio": "BLD-CH-1",
		"setor_id":          outroSetor,
	}, chefeACk)
	if rrSave.Code != http.StatusOK {
		t.Fatalf("chefe criar item: esperado 200, veio %d (%v)", rrSave.Code, resSave)
	}
	itemChefe := int64(resSave["id"].(float64))
	var setorNascido *int64
	if err := st.db.QueryRow(`SELECT setor_id FROM material_itens WHERE id = ?`, itemChefe).Scan(&setorNascido); err != nil || setorNascido == nil || *setorNascido != setorChefe {
		t.Fatalf("item do chefe devia nascer no próprio setor (%d), veio %v (%v)", setorChefe, setorNascido, err)
	}
	// e a lista agora mostra o próprio (recorte positivo)
	rrList2, resList2 := doJSONReq(app, "GET", "/api/material/itens", nil, chefeACk)
	if rrList2.Code != http.StatusOK {
		t.Fatalf("chefe listar itens pós-save: esperado 200, veio %d", rrList2.Code)
	}
	viuProprio := false
	listaChefe2, _ := resList2["itens"].([]any)
	for _, it := range listaChefe2 {
		if m, ok := it.(map[string]any); ok {
			if int64(m["id"].(float64)) == itemChefe {
				viuProprio = true
			}
			if int64(m["id"].(float64)) == itemAlheio {
				t.Fatalf("chefe NÃO devia ver item de outro setor (%v)", m["id"])
			}
		}
	}
	if !viuProprio {
		t.Fatalf("chefe devia ver o item do próprio setor (%d) na lista", itemChefe)
	}

	// 4. Del / baixa patrimonial → 403 com a mensagem da doutrina
	rrDel, _ := doJSONReq(app, "DELETE", fmt.Sprintf("/api/material/itens/%d", itemChefe), nil, chefeACk)
	if rrDel.Code != http.StatusForbidden || !strings.Contains(rrDel.Body.String(), "exclusão e baixa de material é ato do gerente ou encarregado de material") {
		t.Fatalf("chefe excluir próprio: esperado 403 da doutrina, veio %d (%s)", rrDel.Code, rrDel.Body.String())
	}
	if rrB, _ := doJSONReq(app, "DELETE", fmt.Sprintf("/api/material/itens/%d?modo=baixar", itemChefe), nil, chefeACk); rrB.Code != http.StatusForbidden {
		t.Fatalf("chefe baixar próprio: esperado 403, veio %d", rrB.Code)
	}

	// 5. Designação de responsáveis segue fechada (porta de papel do handler)
	rrResp, _ := doJSONReq(app, "POST", "/api/material/responsaveis", map[string]any{"grupo_id": gidA, "encarregado_id": 0}, chefeACk)
	if rrResp.Code != http.StatusForbidden {
		t.Fatalf("chefe salvar responsaveis: esperado 403, veio %d", rrResp.Code)
	}

	// 6. Não-regressão do gerente: exclui item de qualquer setor (200)
	if rrGer, _ := doJSONReq(app, "DELETE", fmt.Sprintf("/api/material/itens/%d", itemChefe), nil, gerACk); rrGer.Code != http.StatusOK {
		t.Fatalf("gerente excluir item do setor do chefe: esperado 200, veio %d", rrGer.Code)
	}
}
