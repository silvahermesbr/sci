package main

// Onda 05/10 — testes de regressão do Drive (ordem do Diretor, 4 itens):
//   1. MOVER (muda pasta_id) e COPIAR (novo registro, mesmo físico).
//   2. (front — ellipsis/wrap cobertos por CSS/JS, verificados no smoke E2E.)
//   3. PROPRIEDADES: nome completo, criado_em (+fmt), autor (JOIN usuarios),
//      tamanho e TABELA de acessos com alvo resolvido e desde quando.
//   4. MODELO DE ACESSO: hDriveItens devolve exatamente próprios + compartilhados
//      (usuário/papel/grupo); sem grant e sem gerência = fora da listagem.

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"testing"
)

// montaDrive: grupo + gerente dono + arquivo na raiz (via SQL, como nos testes de anexos).
func montaDrive(t *testing.T, app *App, st *Store) (int64, *http.Cookie) {
	t.Helper()
	ckGer := loginAsPapel(t, app, st, "gerdr", "gerente")
	var gid int64
	if err := st.db.QueryRow(`INSERT INTO grupos (nome) VALUES ('Grp Drive') RETURNING id`).Scan(&gid); err != nil {
		t.Fatalf("criar grupo: %v", err)
	}
	if _, err := st.db.Exec(`UPDATE usuarios SET grupo_id = ? WHERE login = 'gerdr'`, gid); err != nil {
		t.Fatalf("vincular grupo: %v", err)
	}
	if _, err := st.db.Exec(`UPDATE usuario_papeis SET grupo_id = ? WHERE usuario_id = (SELECT id FROM usuarios WHERE login = 'gerdr')`, gid); err != nil {
		t.Fatalf("papel no grupo: %v", err)
	}
	return gid, ckGer
}

// papelIDDe: id da linha de papel (usuario_papeis) do login — padrão da casa
// quando o teste precisa de alvo_tipo=papel/usuario com alvo_id real. Falha o
// teste se a linha não existir (silenciar aqui virava alvo_id=0 → 400).
func papelIDDe(t *testing.T, st *Store, login string) int64 {
	t.Helper()
	var id int64
	if err := st.db.QueryRow(`SELECT up.id FROM usuario_papeis up JOIN usuarios u ON u.id = up.usuario_id WHERE u.login = ?`, login).Scan(&id); err != nil {
		t.Fatalf("papel de %s: %v", login, err)
	}
	return id
}

func criaArquivoDriveEm(t *testing.T, st *Store, gid int64, nome, nomeArm, autor string, pastaID *int64) int64 {
	t.Helper()
	var id int64
	if err := st.db.QueryRow(`
		INSERT INTO drive_arquivos (pasta_id, grupo_id, nome_original, nome_armazenado, tipo, tamanho, autor_usuario_id, autor_papel_id)
		VALUES (?, ?, ?, ?, 'application/pdf', 2048,
		        (SELECT id FROM usuarios WHERE login = ?),
		        (SELECT up.id FROM usuario_papeis up JOIN usuarios u ON u.id = up.usuario_id WHERE u.login = ?))
		RETURNING id`, pastaID, gid, nome, nomeArm, autor, autor).Scan(&id); err != nil {
		t.Fatalf("criar arquivo: %v", err)
	}
	return id
}

func criaPastaDrive(t *testing.T, st *Store, gid int64, nome string, paiID *int64, autor string) int64 {
	t.Helper()
	var id int64
	if err := st.db.QueryRow(`
		INSERT INTO drive_pastas (nome, grupo_id, pai_id, autor_usuario_id, autor_papel_id)
		VALUES (?, ?, ?, (SELECT id FROM usuarios WHERE login = ?),
		        (SELECT up.id FROM usuario_papeis up JOIN usuarios u ON u.id = up.usuario_id WHERE u.login = ?))
		RETURNING id`, nome, gid, paiID, autor, autor).Scan(&id); err != nil {
		t.Fatalf("criar pasta: %v", err)
	}
	return id
}

// itemNaListagem: procura id na resposta de GET /api/drive/itens.
func itemNaListagem(t *testing.T, rr map[string]any, chave string, id int64) bool {
	t.Helper()
	ars, _ := rr[chave].([]any)
	for _, it := range ars {
		m, _ := it.(map[string]any)
		if m == nil {
			continue
		}
		if idv, _ := m["id"].(float64); int64(idv) == id {
			return true
		}
	}
	return false
}

func TestDriveMoverArquivo(t *testing.T) {
	app, st, cleanup := setupTestApp(t)
	defer cleanup()

	gid, ckGer := montaDrive(t, app, st)
	arqID := criaArquivoDriveEm(t, st, gid, "relat.pdf", "arq_relat", "gerdr", nil)
	pastaID := criaPastaDrive(t, st, gid, "Operação", nil, "gerdr")

	rr, res := doJSONReq(app, "POST", fmt.Sprintf("/api/drive/arquivos/%d/mover", arqID), map[string]any{"pasta_id": pastaID}, ckGer)
	if rr.Code != http.StatusOK {
		t.Fatalf("mover deve 200, veio %d: %s", rr.Code, rr.Body.String())
	}
	if idv, _ := res["id"].(float64); int64(idv) != arqID {
		t.Fatalf("mover preserva o id do item, veio %v", res)
	}
	var pastaBanco *int64
	if err := st.db.QueryRow(`SELECT pasta_id FROM drive_arquivos WHERE id = ?`, arqID).Scan(&pastaBanco); err != nil {
		t.Fatalf("ler pasta_id: %v", err)
	}
	if pastaBanco == nil || *pastaBanco != pastaID {
		t.Fatalf("mover deve setar pasta_id=%d, veio %v", pastaID, pastaBanco)
	}
	// Exatamente 1 registro (mover não duplica).
	var n int
	_ = st.db.QueryRow(`SELECT COUNT(*) FROM drive_arquivos WHERE nome_armazenado = 'arq_relat'`).Scan(&n)
	if n != 1 {
		t.Fatalf("mover não deve duplicar registro, há %d", n)
	}
	// Listagem reflete o novo local.
	rrL, resL := doJSONReq(app, "GET", fmt.Sprintf("/api/drive/itens?pasta_id=%d", pastaID), nil, ckGer)
	if rrL.Code != http.StatusOK || !itemNaListagem(t, resL, "arquivos", arqID) {
		t.Fatalf("arquivo deve aparecer na pasta destino")
	}
}

func TestDriveMoverParaRaizETristes(t *testing.T) {
	app, st, cleanup := setupTestApp(t)
	defer cleanup()

	gid, ckGer := montaDrive(t, app, st)
	pastaID := criaPastaDrive(t, st, gid, "Docs", nil, "gerdr")
	arqID := criaArquivoDriveEm(t, st, gid, "b.pdf", "arq_b", "gerdr", &pastaID)

	// Para a raiz (pasta_id 0 = NULL).
	rr, _ := doJSONReq(app, "POST", fmt.Sprintf("/api/drive/arquivos/%d/mover", arqID), map[string]any{"pasta_id": 0}, ckGer)
	if rr.Code != http.StatusOK {
		t.Fatalf("mover para raiz deve 200, veio %d: %s", rr.Code, rr.Body.String())
	}
	var pastaBanco *int64
	_ = st.db.QueryRow(`SELECT pasta_id FROM drive_arquivos WHERE id = ?`, arqID).Scan(&pastaBanco)
	if pastaBanco != nil {
		t.Fatalf("pasta_id deve virar NULL na raiz, veio %v", pastaBanco)
	}

	// pasta_id negativo = 400.
	rrN, _ := doJSONReq(app, "POST", fmt.Sprintf("/api/drive/arquivos/%d/mover", arqID), map[string]any{"pasta_id": -3}, ckGer)
	if rrN.Code != http.StatusBadRequest {
		t.Fatalf("pasta_id negativo deve 400, veio %d", rrN.Code)
	}

	// Destino de outro grupo = 403 (físico continua na pasta original).
	var gid2 int64
	if err := st.db.QueryRow(`INSERT INTO grupos (nome) VALUES ('Outro') RETURNING id`).Scan(&gid2); err != nil {
		t.Fatalf("grupo 2: %v", err)
	}
	pastaAlheia := criaPastaDrive(t, st, gid2, "Alheia", nil, "admin")
	rrF, _ := doJSONReq(app, "POST", fmt.Sprintf("/api/drive/arquivos/%d/mover", arqID), map[string]any{"pasta_id": pastaAlheia}, ckGer)
	if rrF.Code != http.StatusForbidden {
		t.Fatalf("destino sem permissão deve 403, veio %d: %s", rrF.Code, rrF.Body.String())
	}

	// Admin sem drive = 403 (marca de papel).
	ckAdm := loginAs(t, app, "admin", "admin123")
	rrA, _ := doJSONReq(app, "POST", fmt.Sprintf("/api/drive/arquivos/%d/mover", arqID), map[string]any{"pasta_id": 0}, ckAdm)
	if rrA.Code != http.StatusForbidden {
		t.Fatalf("admin deve 403, veio %d", rrA.Code)
	}
}

func TestDriveCopiarArquivo(t *testing.T) {
	app, st, cleanup := setupTestApp(t)
	defer cleanup()

	gid, ckGer := montaDrive(t, app, st)
	arqID := criaArquivoDriveEm(t, st, gid, "ordem.pdf", "arq_ordem", "gerdr", nil)
	pastaID := criaPastaDrive(t, st, gid, "Cópias", nil, "gerdr")

	// Conteúdo físico real para provar a duplicação byte a byte.
	conteudo := []byte("CONTEUDO SECRETO DA ORDEM 05/10")
	dirFis := filepath.Join(app.st.dataDir, "drive")
	if err := os.MkdirAll(dirFis, 0o750); err != nil {
		t.Fatalf("mkdir drive: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dirFis, "arq_ordem"), conteudo, 0o640); err != nil {
		t.Fatalf("gravar físico: %v", err)
	}

	rr, res := doJSONReq(app, "POST", fmt.Sprintf("/api/drive/arquivos/%d/copiar", arqID), map[string]any{"pasta_id": pastaID}, ckGer)
	if rr.Code != http.StatusOK {
		t.Fatalf("copiar deve 200, veio %d: %s", rr.Code, rr.Body.String())
	}
	novoID, _ := res["id"].(float64)
	if int64(novoID) == arqID {
		t.Fatalf("cópia deve ser NOVO item (id diferente de %d)", arqID)
	}

	// Cópia = NOVO nome_armazenado (UNIQUE), MESMO conteúdo físico; autor = quem copiou.
	var nomeArm, autor string
	var tam int64
	if err := st.db.QueryRow(`SELECT nome_armazenado, tamanho, (SELECT login FROM usuarios WHERE id = autor_usuario_id) FROM drive_arquivos WHERE id = ?`, int64(novoID)).Scan(&nomeArm, &tam, &autor); err != nil {
		t.Fatalf("ler cópia: %v", err)
	}
	if nomeArm == "arq_ordem" || nomeArm == "" {
		t.Fatalf("cópia deve ter nome_armazenado PRÓPRIO, veio %q", nomeArm)
	}
	if tam != int64(len(conteudo)) {
		t.Fatalf("tamanho da cópia deve bater com o físico, veio %d", tam)
	}
	if autor != "gerdr" {
		t.Fatalf("autor da cópia deve ser quem copiou, veio %q", autor)
	}
	got, err := os.ReadFile(filepath.Join(dirFis, nomeArm))
	if err != nil {
		t.Fatalf("físico da cópia deve existir em dados/drive: %v", err)
	}
	if string(got) != string(conteudo) {
		t.Fatalf("conteúdo da cópia deve ser idêntico ao original")
	}
	// Original intocado.
	if _, err := os.Stat(filepath.Join(dirFis, "arq_ordem")); err != nil {
		t.Fatalf("original deve continuar no disco: %v", err)
	}

	// Listagens: original na raiz, cópia na pasta.
	rrL, resL := doJSONReq(app, "GET", "/api/drive/itens?pasta_id=0", nil, ckGer)
	if rrL.Code != http.StatusOK || !itemNaListagem(t, resL, "arquivos", arqID) {
		t.Fatalf("original deve seguir na raiz")
	}
	rrL2, resL2 := doJSONReq(app, "GET", fmt.Sprintf("/api/drive/itens?pasta_id=%d", pastaID), nil, ckGer)
	if rrL2.Code != http.StatusOK || !itemNaListagem(t, resL2, "arquivos", int64(novoID)) {
		t.Fatalf("cópia deve aparecer na pasta destino")
	}

	// Itens independentes: excluir o ORIGINAL não afeta a cópia (e vice-versa).
	rrD, _ := doJSONReq(app, "DELETE", fmt.Sprintf("/api/drive/arquivos/%d", arqID), nil, ckGer)
	if rrD.Code != http.StatusOK {
		t.Fatalf("delete do original deve 200, veio %d: %s", rrD.Code, rrD.Body.String())
	}
	if _, err := os.Stat(filepath.Join(dirFis, nomeArm)); err != nil {
		t.Fatalf("físico da CÓPIA não pode ser afetado pelo delete do original: %v", err)
	}
	rrDl, _ := doJSONReq(app, "GET", fmt.Sprintf("/api/drive/download/%d", int64(novoID)), nil, ckGer)
	if rrDl.Code != http.StatusOK {
		t.Fatalf("cópia deve continuar baixável após delete do original, veio %d", rrDl.Code)
	}
}

func TestDrivePropriedades(t *testing.T) {
	app, st, cleanup := setupTestApp(t)
	defer cleanup()

	gid, ckGer := montaDrive(t, app, st)
	// item 2 (ec8f1cb): autor exibe nome_guerra→nome_completo (login NUNCA).
	// O teste pedia o login 'gerdr' — contrato antigo; dá nome de guerra ao
	// autor e cobra por ele (a prova do JOIN continua de pé).
	if _, err := st.db.Exec(`UPDATE usuarios SET nome_guerra = 'Ger Dr' WHERE login = 'gerdr'`); err != nil {
		t.Fatalf("nome de guerra do autor: %v", err)
	}
	arqID := criaArquivoDriveEm(t, st, gid, "Relatorio_Diario_De_Operacao_Edicao_Extremamente_Longa.pdf", "arq_prop", "gerdr", nil)

	// Grants: para o operador (usuário) e para o próprio grupo.
	// Garante usuário + linha de papel do opdr (login sincroniza usuario_papeis);
	// sem isto o lookup de alvo_id morre em "no rows" e o grant ia com alvo_id=0.
	loginAsPapel(t, app, st, "opdr", "operador")
	papelOpID := papelIDDe(t, st, "opdr")
	rrC, _ := doJSONReq(app, "POST", "/api/drive/compartilhar", map[string]any{"arquivo_id": arqID, "alvo_tipo": "usuario", "alvo_id": papelOpID, "pode_editar": true}, ckGer)
	if rrC.Code != http.StatusOK {
		t.Fatalf("compartilhar deve 200, veio %d: %s", rrC.Code, rrC.Body.String())
	}
	rrC2, _ := doJSONReq(app, "POST", "/api/drive/compartilhar", map[string]any{"arquivo_id": arqID, "alvo_tipo": "grupo", "alvo_id": gid, "pode_editar": false}, ckGer)
	if rrC2.Code != http.StatusOK {
		t.Fatalf("compartilhar grupo deve 200, veio %d", rrC2.Code)
	}

	rr, res := doJSONReq(app, "GET", fmt.Sprintf("/api/drive/arquivos/%d/propriedades", arqID), nil, ckGer)
	if rr.Code != http.StatusOK {
		t.Fatalf("propriedades deve 200, veio %d: %s", rr.Code, rr.Body.String())
	}

	if res["nome_original"] != "Relatorio_Diario_De_Operacao_Edicao_Extremamente_Longa.pdf" {
		t.Fatalf("nome COMPLETO obrigatório, veio %v", res["nome_original"])
	}
	if res["autor_nome"] != "Ger Dr" {
		t.Fatalf("autor via JOIN usuarios (nome de guerra, item 2), veio %v", res["autor_nome"])
	}
	if tsz, _ := res["tamanho"].(float64); int64(tsz) != 2048 {
		t.Fatalf("tamanho, veio %v", res["tamanho"])
	}
	ce, _ := res["criado_em"].(string)
	if ce == "" {
		t.Fatalf("criado_em não pode vir vazio")
	}
	if res["criado_em_fmt"] == "" || res["criado_em_fmt"] == ce {
		t.Fatalf("criado_em_fmt deve ser data formatada em Brasília, veio %q (cru %q)", res["criado_em_fmt"], ce)
	}

	acs, _ := res["acessos"].([]any)
	if len(acs) != 2 {
		t.Fatalf("tabela de acessos deve ter 2 linhas, veio %v", res["acessos"])
	}
	tipos := map[string]bool{}
	datas := 0
	for _, a := range acs {
		m, _ := a.(map[string]any)
		tipos[m["alvo_tipo"].(string)] = true
		if m["alvo_nome"] == "" || m["alvo_nome"] == nil {
			t.Fatalf("alvo deve vir resolvido com nome, veio %v", m)
		}
		if fmtv, _ := m["criado_em_fmt"].(string); fmtv != "" {
			datas++
		}
	}
	if !tipos["usuario"] || !tipos["grupo"] {
		t.Fatalf("acessos devem cobrir usuario e grupo, veio %v", tipos)
	}
	if datas != 2 {
		t.Fatalf("cada acesso deve ter 'desde quando' (criado_em_fmt), veio %d/2", datas)
	}
}

func TestDriveAcessoItensVisibilidade(t *testing.T) {
	app, st, cleanup := setupTestApp(t)
	defer cleanup()

	gid, ckGer := montaDrive(t, app, st)

	// Operador no MESMO grupo.
	loginAsPapel(t, app, st, "opdr", "operador")
	if _, err := st.db.Exec(`UPDATE usuarios SET grupo_id = ? WHERE login = 'opdr'`, gid); err != nil {
		t.Fatalf("grupo do op: %v", err)
	}
	if _, err := st.db.Exec(`UPDATE usuario_papeis SET grupo_id = ? WHERE usuario_id = (SELECT id FROM usuarios WHERE login = 'opdr')`, gid); err != nil {
		t.Fatalf("papel do op: %v", err)
	}
	ckOp := loginAs(t, app, "opdr", "senha-operador")

	// Arquivos: próprio (gerdr), compartilhado com o grupo, sem acesso nenhum.
	proprio := criaArquivoDriveEm(t, st, gid, "meu.pdf", "arq_meu", "gerdr", nil)
	compart := criaArquivoDriveEm(t, st, gid, "comp.pdf", "arq_comp", "gerdr", nil)
	_, _ = st.db.Exec(`INSERT INTO drive_compartilhamentos (arquivo_id, alvo_usuario_id, pode_editar) VALUES (?, (SELECT id FROM usuarios WHERE login='opdr'), 0)`, compart)
	semAcesso := criaArquivoDriveEm(t, st, gid, "protegido.pdf", "arq_prot", "gerdr", nil)

	// Listagem do GERENTE: vê o próprio e os dois que ele criou (autor).
	rrG, resG := doJSONReq(app, "GET", "/api/drive/itens?pasta_id=0", nil, ckGer)
	if rrG.Code != http.StatusOK {
		t.Fatalf("listagem gerente: %d", rrG.Code)
	}
	if !itemNaListagem(t, resG, "arquivos", proprio) || !itemNaListagem(t, resG, "arquivos", compart) || !itemNaListagem(t, resG, "arquivos", semAcesso) {
		t.Fatalf("gerente (autor) deve ver os 3 arquivos próprios")
	}

	// Listagem do OPERADOR: só o compartilhado com ele.
	rrO, resO := doJSONReq(app, "GET", "/api/drive/itens?pasta_id=0", nil, ckOp)
	if rrO.Code != http.StatusOK {
		t.Fatalf("listagem operador: %d", rrO.Code)
	}
	if !itemNaListagem(t, resO, "arquivos", compart) {
		t.Fatalf("operador deve ver o arquivo compartilhado com ele")
	}
	if itemNaListagem(t, resO, "arquivos", proprio) || itemNaListagem(t, resO, "arquivos", semAcesso) {
		t.Fatalf("operador NÃO deve ver arquivos sem grant (modelo: acesso = lista de compartilhamentos)")
	}

	// Compartilhado com o GRUPO: passa a aparecer para o operador.
	_, _ = st.db.Exec(`INSERT INTO drive_compartilhamentos (arquivo_id, alvo_grupo_id, pode_editar) VALUES (?, ?, 0)`, semAcesso, gid)
	rrO2, resO2 := doJSONReq(app, "GET", "/api/drive/itens?pasta_id=0", nil, ckOp)
	if rrO2.Code != http.StatusOK || !itemNaListagem(t, resO2, "arquivos", semAcesso) {
		t.Fatalf("grant de grupo deve incluir o arquivo na listagem do membro")
	}

	// Aba Compartilhados Comigo: dedup — mesmo arquivo com 2 grants aparece 1 vez.
	_, _ = st.db.Exec(`INSERT INTO drive_compartilhamentos (arquivo_id, alvo_grupo_id, pode_editar) VALUES (?, ?, 1)`, compart, gid)
	rrO3, resO3 := doJSONReq(app, "GET", "/api/drive/itens?pasta_id=0&compartilhados=1", nil, ckOp)
	if rrO3.Code != http.StatusOK {
		t.Fatalf("aba compartilhados: %d", rrO3.Code)
	}
	ars, _ := resO3["arquivos"].([]any)
	nComp := 0
	for _, a := range ars {
		m, _ := a.(map[string]any)
		if idv, _ := m["id"].(float64); int64(idv) == compart {
			nComp++
		}
	}
	if nComp != 1 {
		t.Fatalf("dedup na aba compartilhados: esperado 1 ocorrência, veio %d", nComp)
	}

	// Download sem grant continua 403 (item isolado + lista de acesso).
	rrDl, _ := doJSONReq(app, "GET", fmt.Sprintf("/api/drive/download/%d", proprio), nil, ckOp)
	if rrDl.Code != http.StatusForbidden {
		t.Fatalf("download sem grant deve 403, veio %d", rrDl.Code)
	}
	_ = json.Marshal // mantém import usado em refactors futuros
}
