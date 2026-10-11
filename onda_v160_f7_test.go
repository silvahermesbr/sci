package main

// onda_v160_f7_test.go — v1.6.0 Fase 7 ("setor em tudo" para o OPERADOR):
// material e conferência recortados ao PRÓPRIO setor do contexto operador.
//
// Onda v1.6.0-contextos (doutrina de NÍVEL do comando): o teste do C provava
// operador CRUANDO item no próprio setor (200) — INVERTIDO: operador SÓ
// cautela/descautela (save/del 403); chefe de setor ADICIONA/EDITA o próprio
// setor e NÃO exclui/baixa; gerente/enc_material seguem de grupo.
//
// Matriz por persona (padrão da casa: setupTestApp + loginAs + doJSONReq;
// positivo E negativo):
//   - operador (com setor): lista/cautela/descautela/conferência/PDF só do
//     PRÓPRIO setor (positivo 200 no próprio + prova de AUSÊNCIA do alheio na
//     lista); cadastrar/editar/excluir/baixar item → 403 (ANTES do recorte —
//     o papel veta primeiro); iniciar de material com setor alheio no CORPO
//     usa o PRÓPRIO setor (200 com setor_id efetivo devolvido); conferência
//     de pessoal: concluir/reabrir/pré-fechar setor alheio → 403, próprio → 200.
//   - operador SEM setor → 403 "conta sem setor atribuído — solicite ao
//     gerente/encarregado" nas rotas do módulo (a MESMA mensagem do guarda
//     central do Agente B; as camadas coexistem na integração). Escopo de
//     grupo segue onde a onda manteve (responsáveis).
//   - chefe_setor (comando materializado em chefe_setores + setor de cadastro):
//     save próprio setor 200 (nascendo SEMPRE no próprio, corpo ignorado),
//     item de outro setor não é alcançado (404 honesto do P1-2), Del/baixa
//     403 "exclusão e baixa de material é ato do gerente ou encarregado de
//     material", cautela/descautela do próprio setor 200 (mantida).
//   - gerente/enc_material seguem GRUPO (vêem o que o operador não vê; editam
//     e excluem item de qualquer setor); escrita de categorias vira
//     gerente/enc_material (catálogo de grupo) com leitura aberta.

import (
	"encoding/base64"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// f7Cenario: fixtures do módulo material em dois setores + personas.
type f7Cenario struct {
	gid, s1, s2           int64
	pA, pB                int64
	itemS1, itemS2, itemG int64
	ckGer, ckEnc, ckOp    *http.Cookie // ckOp = operador do setor s1
	ckOpSem               *http.Cookie // operador SEM setor
}

// f7SetupMaterial: grupo, setores Alpha (s1)/Bravo (s2), itens nos dois setores
// + Carga Geral, cautelas do gerente em cada item de setor, e as personas.
func f7SetupMaterial(t *testing.T, app *App, st *Store) *f7Cenario {
	t.Helper()
	c := &f7Cenario{}
	if err := st.db.QueryRow(`INSERT INTO grupos (nome) VALUES ('Grp F7 Mat') RETURNING id`).Scan(&c.gid); err != nil {
		t.Fatalf("criar grupo: %v", err)
	}
	if err := st.db.QueryRow(`INSERT INTO setores (grupo_id, nome) VALUES (?, 'F7 Alpha') RETURNING id`, c.gid).Scan(&c.s1); err != nil {
		t.Fatalf("criar setor s1: %v", err)
	}
	if err := st.db.QueryRow(`INSERT INTO setores (grupo_id, nome) VALUES (?, 'F7 Bravo') RETURNING id`, c.gid).Scan(&c.s2); err != nil {
		t.Fatalf("criar setor s2: %v", err)
	}
	if err := st.db.QueryRow(`INSERT INTO pessoas (nome_guerra, nome_completo, grupo_id, setor_id, status) VALUES ('F7 PA','Pessoa Alpha', ?, ?, 'ativo') RETURNING id`, c.gid, c.s1).Scan(&c.pA); err != nil {
		t.Fatalf("criar pessoa A: %v", err)
	}
	if err := st.db.QueryRow(`INSERT INTO pessoas (nome_guerra, nome_completo, grupo_id, setor_id, status) VALUES ('F7 PB','Pessoa Bravo', ?, ?, 'ativo') RETURNING id`, c.gid, c.s2).Scan(&c.pB); err != nil {
		t.Fatalf("criar pessoa B: %v", err)
	}

	// gerente
	criaUsuarioTeste(t, st, "f7_ger", "senha-ger", "gerente")
	if _, err := st.db.Exec(`UPDATE usuarios SET grupo_id = ? WHERE login = 'f7_ger'`, c.gid); err != nil {
		t.Fatalf("vincular gerente: %v", err)
	}
	c.ckGer = loginAs(t, app, "f7_ger", "senha-ger")

	// enc_material puro (contexto materializado pela v45)
	var fMat int64
	if err := st.db.QueryRow(`SELECT id FROM funcoes WHERE chave = 'enc_material'`).Scan(&fMat); err != nil {
		t.Fatalf("cadeira enc_material ausente: %v", err)
	}
	criaUsuarioTeste(t, st, "f7_enc", "senha-enc", "")
	if _, err := st.db.Exec(`UPDATE usuarios SET grupo_id = ? WHERE login = 'f7_enc'`, c.gid); err != nil {
		t.Fatalf("vincular enc: %v", err)
	}
	var idEnc int64
	if err := st.db.QueryRow(`SELECT id FROM usuarios WHERE login = 'f7_enc'`).Scan(&idEnc); err != nil {
		t.Fatalf("id enc: %v", err)
	}
	if _, err := st.db.Exec(`INSERT INTO funcao_membros (funcao_id, grupo_id, usuario_id, titularidade) VALUES (?,?,?,'titular')`, fMat, c.gid, idEnc); err != nil {
		t.Fatalf("designar enc: %v", err)
	}
	v45Reexecuta(t, st)
	c.ckEnc = loginAs(t, app, "f7_enc", "senha-enc")
	if p := f2MePapel(t, app, c.ckEnc); p != "enc_material" {
		t.Fatalf("cenário: enc devia logar NO CONTEXTO enc_material, veio %q", p)
	}

	// operador COM setor (contexto operador) e operador SEM setor
	criaUsuarioTeste(t, st, "f7_op", "senha-op", "operador")
	if _, err := st.db.Exec(`UPDATE usuarios SET grupo_id = ?, setor_id = ? WHERE login = 'f7_op'`, c.gid, c.s1); err != nil {
		t.Fatalf("vincular operador: %v", err)
	}
	c.ckOp = loginAs(t, app, "f7_op", "senha-op")
	criaUsuarioTeste(t, st, "f7_op_sem", "senha-ops", "operador")
	if _, err := st.db.Exec(`UPDATE usuarios SET grupo_id = ? WHERE login = 'f7_op_sem'`, c.gid); err != nil {
		t.Fatalf("vincular operador sem setor: %v", err)
	}
	c.ckOpSem = loginAs(t, app, "f7_op_sem", "senha-ops")

	// itens: próprio setor, setor alheio e Carga Geral (setor NULL)
	if err := st.db.QueryRow(`INSERT INTO material_itens (grupo_id, setor_id, nome, codigo_patrimonio, status, quantidade) VALUES (?, ?, 'Fuzil Alpha', 'F7-0001', 'disponivel', 5) RETURNING id`, c.gid, c.s1).Scan(&c.itemS1); err != nil {
		t.Fatalf("criar item s1: %v", err)
	}
	if err := st.db.QueryRow(`INSERT INTO material_itens (grupo_id, setor_id, nome, codigo_patrimonio, status, quantidade) VALUES (?, ?, 'Fuzil Bravo', 'F7-0002', 'disponivel', 5) RETURNING id`, c.gid, c.s2).Scan(&c.itemS2); err != nil {
		t.Fatalf("criar item s2: %v", err)
	}
	if err := st.db.QueryRow(`INSERT INTO material_itens (grupo_id, setor_id, nome, codigo_patrimonio, status, quantidade) VALUES (?, NULL, 'Carga Geral F7', 'F7-0003', 'disponivel', 5) RETURNING id`, c.gid).Scan(&c.itemG); err != nil {
		t.Fatalf("criar item carga geral: %v", err)
	}

	// cautelas do gerente (escopo grupo) em cada item de setor
	rr, res := doJSONReq(app, "POST", "/api/material/cautelar", map[string]any{"item_id": c.itemS1, "pessoa_id": c.pA}, c.ckGer)
	if rr.Code != http.StatusOK {
		t.Fatalf("cenário: gerente cautela item s1 (200), veio %d (%v)", rr.Code, res)
	}
	rr, res = doJSONReq(app, "POST", "/api/material/cautelar", map[string]any{"item_id": c.itemS2, "pessoa_id": c.pB}, c.ckGer)
	if rr.Code != http.StatusOK {
		t.Fatalf("cenário: gerente cautela item s2 (200), veio %d (%v)", rr.Code, res)
	}
	return c
}

// f7IDs: ids (float64 do JSON) presentes na lista.
func f7IDs(res map[string]any, chave string) map[int64]bool {
	out := map[int64]bool{}
	lista, _ := res[chave].([]any)
	for _, it := range lista {
		if m, ok := it.(map[string]any); ok {
			if id, ok := m["id"].(float64); ok {
				out[int64(id)] = true
			}
		}
	}
	return out
}

func TestF7MaterialOperadorRecortePorSetor(t *testing.T) {
	app, st, cleanup := setupTestApp(t)
	defer cleanup()
	c := f7SetupMaterial(t, app, st)

	// ---------- LEITURAS: só o próprio setor ----------
	rr, res := doJSONReq(app, "GET", "/api/material/itens", nil, c.ckOp)
	if rr.Code != http.StatusOK {
		t.Fatalf("op itens: esperado 200, veio %d (%v)", rr.Code, res)
	}
	ids := f7IDs(res, "itens")
	if !ids[c.itemS1] || ids[c.itemS2] || ids[c.itemG] {
		t.Fatalf("op devia ver SÓ o item do próprio setor: vistos=%v (s1=%d s2=%d geral=%d)", ids, c.itemS1, c.itemS2, c.itemG)
	}

	rr, res = doJSONReq(app, "GET", "/api/material/cautelas", nil, c.ckOp)
	if rr.Code != http.StatusOK {
		t.Fatalf("op cautelas: esperado 200, veio %d (%v)", rr.Code, res)
	}
	var cautS1, cautS2 int64
	if err := st.db.QueryRow(`SELECT id FROM material_cautelas WHERE item_id = ? AND status = 'ativa'`, c.itemS1).Scan(&cautS1); err != nil {
		t.Fatalf("cautela s1: %v", err)
	}
	if err := st.db.QueryRow(`SELECT id FROM material_cautelas WHERE item_id = ? AND status = 'ativa'`, c.itemS2).Scan(&cautS2); err != nil {
		t.Fatalf("cautela s2: %v", err)
	}
	if got := f7IDs(res, "cautelas"); !got[cautS1] || got[cautS2] {
		t.Fatalf("op devia ver SÓ a cautela do próprio setor: vistos=%v (s1=%d s2=%d)", got, cautS1, cautS2)
	}

	// ---------- CONFERÊNCIA DE MATERIAL: iniciar força o próprio setor ----------
	// operador inicia com setor ALHEIO no corpo → usa o PRÓPRIO (200 + setor_id
	// efetivo devolvido — mais honesto p/ o front; divergência anotada).
	rr, res = doJSONReq(app, "POST", "/api/material/conferencias/iniciar", map[string]any{"setor_id": c.s2}, c.ckOp)
	if rr.Code != http.StatusOK {
		t.Fatalf("op iniciar (corpo setor alheio): esperado 200, veio %d (%v)", rr.Code, res)
	}
	if sid, _ := res["setor_id"].(float64); int64(sid) != c.s1 {
		t.Fatalf("op iniciar devia FORÇAR o próprio setor (%d), veio setor_id=%v", c.s1, res["setor_id"])
	}
	confS1 := int64(res["id"].(float64))
	var confS1Setor *int64
	if err := st.db.QueryRow(`SELECT setor_id FROM material_conferencias WHERE id = ?`, confS1).Scan(&confS1Setor); err != nil {
		t.Fatalf("conf s1: %v", err)
	}
	if confS1Setor == nil || *confS1Setor != c.s1 {
		t.Fatalf("conf do operador devia nascer no setor próprio (%d), veio %v", c.s1, confS1Setor)
	}

	// gerente segue de GRUPO: inicia no setor que QUISER (corpo honrado)
	rr, res = doJSONReq(app, "POST", "/api/material/conferencias/iniciar", map[string]any{"setor_id": c.s2}, c.ckGer)
	if rr.Code != http.StatusOK {
		t.Fatalf("gerente iniciar s2: esperado 200, veio %d (%v)", rr.Code, res)
	}
	confS2 := int64(res["id"].(float64))

	// GET unitário: próprio 200, alheia 403
	if rr, _ = doJSONReq(app, "GET", fmt.Sprintf("/api/material/conferencias/%d", confS1), nil, c.ckOp); rr.Code != http.StatusOK {
		t.Fatalf("op GET conf própria: esperado 200, veio %d", rr.Code)
	}
	if rr, _ = doJSONReq(app, "GET", fmt.Sprintf("/api/material/conferencias/%d", confS2), nil, c.ckOp); rr.Code != http.StatusForbidden {
		t.Fatalf("op GET conf alheia: esperado 403, veio %d", rr.Code)
	}

	// ---------- BIPAR / FECHAR ----------
	// bipar na conf ALHEIA → 403 (conferência fora do recorte)
	rr, _ = doJSONReq(app, "POST", fmt.Sprintf("/api/material/conferencias/%d/bipar", confS2), map[string]any{"item_id": c.itemS1}, c.ckOp)
	if rr.Code != http.StatusForbidden {
		t.Fatalf("op bipar conf alheia: esperado 403, veio %d", rr.Code)
	}
	// bipar ITEM ALHEIO na conf PRÓPRIA → 403 (item fora do recorte)
	rr, _ = doJSONReq(app, "POST", fmt.Sprintf("/api/material/conferencias/%d/bipar", confS1), map[string]any{"item_id": c.itemS2}, c.ckOp)
	if rr.Code != http.StatusForbidden {
		t.Fatalf("op bipar item alheio na própria: esperado 403, veio %d", rr.Code)
	}
	// bipar o próprio → 200; fechar alheia 403; fechar própria 200
	if rr, res = doJSONReq(app, "POST", fmt.Sprintf("/api/material/conferencias/%d/bipar", confS1), map[string]any{"item_id": c.itemS1}, c.ckOp); rr.Code != http.StatusOK {
		t.Fatalf("op bipar próprio: esperado 200, veio %d (%v)", rr.Code, res)
	}
	if rr, _ = doJSONReq(app, "POST", fmt.Sprintf("/api/material/conferencias/%d/fechar", confS2), nil, c.ckOp); rr.Code != http.StatusForbidden {
		t.Fatalf("op fechar conf alheia: esperado 403, veio %d", rr.Code)
	}
	if rr, _ = doJSONReq(app, "POST", fmt.Sprintf("/api/material/conferencias/%d/fechar", confS1), nil, c.ckOp); rr.Code != http.StatusOK {
		t.Fatalf("op fechar conf própria: esperado 200, veio %d", rr.Code)
	}
	// gerente fecha a dele (a listagem só resolve linhas com fechada_em —
	// comportamento pré-existente do Scan; o recorte se prova em FECHADAS)
	if rr, _ = doJSONReq(app, "POST", fmt.Sprintf("/api/material/conferencias/%d/fechar", confS2), nil, c.ckGer); rr.Code != http.StatusOK {
		t.Fatalf("gerente fechar conf s2: esperado 200, veio %d", rr.Code)
	}

	// listagem de conferências: operador só o próprio setor
	rr, res = doJSONReq(app, "GET", "/api/material/conferencias", nil, c.ckOp)
	if rr.Code != http.StatusOK {
		t.Fatalf("op conferencias: esperado 200, veio %d (%v)", rr.Code, res)
	}
	if got := f7IDs(res, "conferencias"); !got[confS1] || got[confS2] {
		t.Fatalf("op devia listar SÓ a conferência do próprio setor: vistos=%v (s1=%d s2=%d)", got, confS1, confS2)
	}

	// ---------- ESCRITAS DE ITEM (doutrina de NÍVEL: operador SÓ cautela) ----------
	// O teste da fase 7 provava operador CRUANDO item no próprio setor (200);
	// a doutrina do comando INVERTEU: operador não cadastra nem edita — o 403
	// do checkpoint vem ANTES do recorte (o papel veta, depois o escopo fala).
	rr, _ = doJSONReq(app, "POST", "/api/material/itens", map[string]any{"nome": "Item Novo Op", "codigo_patrimonio": "F7-OP-1", "setor_id": c.s2}, c.ckOp)
	if rr.Code != http.StatusForbidden || !strings.Contains(rr.Body.String(), "operador não tem permissão para cadastrar ou editar materiais") {
		t.Fatalf("op criar item: esperado 403 com a mensagem do checkpoint, veio %d (%s)", rr.Code, rr.Body.String())
	}
	// editar item ALHEIO e o PRÓPRIO → 403 (mesma régua; sem exceção ao próprio)
	rr, _ = doJSONReq(app, "POST", "/api/material/itens", map[string]any{"id": c.itemS2, "nome": "Hackeado", "codigo_patrimonio": "F7-0002"}, c.ckOp)
	if rr.Code != http.StatusForbidden {
		t.Fatalf("op editar item alheio: esperado 403, veio %d", rr.Code)
	}
	rr, _ = doJSONReq(app, "POST", "/api/material/itens", map[string]any{"id": c.itemS1, "nome": "Fuzil Alpha Editado", "codigo_patrimonio": "F7-0001", "quantidade": 5}, c.ckOp)
	if rr.Code != http.StatusForbidden {
		t.Fatalf("op editar próprio: esperado 403, veio %d", rr.Code)
	}
	// excluir/baixar: ALHEIO, Carga Geral E PRÓPRIO → 403 (só cautela/descautela)
	if rr, _ = doJSONReq(app, "DELETE", fmt.Sprintf("/api/material/itens/%d", c.itemS2), nil, c.ckOp); rr.Code != http.StatusForbidden {
		t.Fatalf("op excluir item alheio: esperado 403, veio %d", rr.Code)
	}
	if rr, _ = doJSONReq(app, "DELETE", fmt.Sprintf("/api/material/itens/%d", c.itemG), nil, c.ckOp); rr.Code != http.StatusForbidden {
		t.Fatalf("op excluir carga geral: esperado 403, veio %d", rr.Code)
	}
	if rr, _ = doJSONReq(app, "DELETE", fmt.Sprintf("/api/material/itens/%d?modo=baixar", c.itemS1), nil, c.ckOp); rr.Code != http.StatusForbidden {
		t.Fatalf("op baixar próprio: esperado 403, veio %d", rr.Code)
	}
	if rr, _ = doJSONReq(app, "DELETE", fmt.Sprintf("/api/material/itens/%d", c.itemS1), nil, c.ckOp); rr.Code != http.StatusForbidden {
		t.Fatalf("op excluir próprio: esperado 403, veio %d", rr.Code)
	}

	// ---------- CAUTELAR / DEVOLVER ----------
	// cautelar item ALHEIO → 403
	rr, _ = doJSONReq(app, "POST", "/api/material/cautelar", map[string]any{"item_id": c.itemS2, "pessoa_id": c.pB}, c.ckOp)
	if rr.Code != http.StatusForbidden {
		t.Fatalf("op cautelar item alheio: esperado 403, veio %d", rr.Code)
	}
	// cautelar o PRÓPRIO → 200 e devolver → 200
	rr, res = doJSONReq(app, "POST", "/api/material/cautelar", map[string]any{"item_id": c.itemS1, "pessoa_id": c.pA}, c.ckOp)
	if rr.Code != http.StatusOK {
		t.Fatalf("op cautelar próprio: esperado 200, veio %d (%v)", rr.Code, res)
	}
	cautOp := int64(res["cautela_id"].(float64))
	rr, _ = doJSONReq(app, "POST", "/api/material/devolver", map[string]any{"cautela_id": cautS2}, c.ckOp)
	if rr.Code != http.StatusForbidden {
		t.Fatalf("op devolver cautela alheia: esperado 403, veio %d", rr.Code)
	}
	if rr, _ = doJSONReq(app, "POST", "/api/material/devolver", map[string]any{"cautela_id": cautOp}, c.ckOp); rr.Code != http.StatusOK {
		t.Fatalf("op devolver própria: esperado 200, veio %d", rr.Code)
	}

	// ---------- ANEXOS/COMENTÁRIOS por item e por cautela ----------
	b64 := base64.StdEncoding.EncodeToString([]byte("\x89PNG\r\n\x1a\n"))
	if rr, _ = doJSONReq(app, "POST", fmt.Sprintf("/api/material/itens/%d/anexos", c.itemS2), map[string]any{"nome_arquivo": "x.png", "tipo_mime": "image/png", "dados_base64": b64}, c.ckOp); rr.Code != http.StatusForbidden {
		t.Fatalf("op anexo item alheio: esperado 403, veio %d", rr.Code)
	}
	if rr, _ = doJSONReq(app, "GET", fmt.Sprintf("/api/material/itens/%d/comentarios", c.itemS2), nil, c.ckOp); rr.Code != http.StatusForbidden {
		t.Fatalf("op comentarios item alheio: esperado 403, veio %d", rr.Code)
	}
	rr, res = doJSONReq(app, "POST", fmt.Sprintf("/api/material/itens/%d/anexos", c.itemS1), map[string]any{"nome_arquivo": "proprio.png", "tipo_mime": "image/png", "dados_base64": b64}, c.ckOp)
	if rr.Code != http.StatusOK {
		t.Fatalf("op anexo próprio: esperado 200, veio %d (%v)", rr.Code, res)
	}
	anexoItem := int64(res["id"].(float64))
	if rr, _ = doJSONReq(app, "GET", fmt.Sprintf("/api/material/item-anexos/%d", anexoItem), nil, c.ckOp); rr.Code != http.StatusOK {
		t.Fatalf("op GET anexo próprio: esperado 200, veio %d", rr.Code)
	}
	if rr, _ = doJSONReq(app, "POST", fmt.Sprintf("/api/material/itens/%d/comentarios", c.itemS1), map[string]any{"texto": "do operador"}, c.ckOp); rr.Code != http.StatusOK {
		t.Fatalf("op comentário próprio: esperado 200, veio %d", rr.Code)
	}
	// cautela alheia: anexo add/list/get/del → 403
	rrAnx, _ := doJSONReq(app, "POST", fmt.Sprintf("/api/material/cautelas/%d/anexos", cautS2), map[string]any{"nome_arquivo": "r.png", "tipo_mime": "image/png", "dados_base64": b64}, c.ckOp)
	if rrAnx.Code != http.StatusForbidden {
		t.Fatalf("op anexo cautela alheia: esperado 403, veio %d", rrAnx.Code)
	}
	if rr, _ = doJSONReq(app, "GET", fmt.Sprintf("/api/material/cautelas/%d/anexos", cautS2), nil, c.ckOp); rr.Code != http.StatusForbidden {
		t.Fatalf("op list anexos cautela alheia: esperado 403, veio %d", rr.Code)
	}
	// e na PRÓPRIA: add + get → 200
	rr, res = doJSONReq(app, "POST", fmt.Sprintf("/api/material/cautelas/%d/anexos", cautOp), map[string]any{"nome_arquivo": "proprio.pdf", "tipo_mime": "application/pdf", "dados_base64": b64}, c.ckOp)
	if rr.Code != http.StatusOK {
		t.Fatalf("op anexo cautela própria: esperado 200, veio %d (%v)", rr.Code, res)
	}
	anexoCaut := int64(res["id"].(float64))
	if rr, _ = doJSONReq(app, "GET", fmt.Sprintf("/api/material/anexos/%d", anexoCaut), nil, c.ckOp); rr.Code != http.StatusOK {
		t.Fatalf("op GET anexo cautela própria: esperado 200, veio %d", rr.Code)
	}

	// ---------- PDFs ----------
	codPDF := func(path string) int {
		rq := doRawReqH(app, "GET", path, nil, "", c.ckOp)
		return rq.Code
	}
	if got := codPDF("/api/material/inventario/pdf"); got != http.StatusOK {
		t.Fatalf("op inventário: esperado 200, veio %d", got)
	}
	if got := codPDF(fmt.Sprintf("/api/material/etiquetas-lote.pdf?ids=%d", c.itemS1)); got != http.StatusOK {
		t.Fatalf("op etiquetas próprio: esperado 200, veio %d", got)
	}
	if got := codPDF(fmt.Sprintf("/api/material/etiquetas-lote.pdf?ids=%d", c.itemS2)); got != http.StatusNotFound {
		t.Fatalf("op etiquetas alheias (ids ∩ setor vazio): esperado 404, veio %d", got)
	}
	if got := codPDF(fmt.Sprintf("/api/material/cautelas/%d/recibo.pdf", cautS1)); got != http.StatusOK {
		t.Fatalf("op recibo cautela própria: esperado 200, veio %d", got)
	}
	if got := codPDF(fmt.Sprintf("/api/material/cautelas/%d/recibo.pdf", cautS2)); got != http.StatusForbidden {
		t.Fatalf("op recibo cautela alheia: esperado 403, veio %d", got)
	}
	if got := codPDF(fmt.Sprintf("/api/material/conferencias/%d/pronto.pdf", confS2)); got != http.StatusForbidden {
		t.Fatalf("op pronto conf alheia: esperado 403, veio %d", got)
	}

	// ---------- QR ----------
	if rr, _ = doJSONReq(app, "GET", fmt.Sprintf("/api/material/itens/%d/qr", c.itemS1), nil, c.ckOp); rr.Code != http.StatusOK {
		t.Fatalf("op QR próprio: esperado 200, veio %d", rr.Code)
	}
	if rr, _ = doJSONReq(app, "GET", fmt.Sprintf("/api/material/itens/%d/qr", c.itemS2), nil, c.ckOp); rr.Code != http.StatusForbidden {
		t.Fatalf("op QR alheio: esperado 403, veio %d", rr.Code)
	}

	// ---------- CATEGORIAS: escrita vira gerente/enc_material; leitura segue ----------
	if rr, res = doJSONReq(app, "GET", "/api/material/categorias", nil, c.ckOp); rr.Code != http.StatusOK {
		t.Fatalf("op categorias (leitura): esperado 200, veio %d (%v)", rr.Code, res)
	}
	if rr, _ = doJSONReq(app, "POST", "/api/material/categorias", map[string]any{"nome": "Cat Pelo Operador"}, c.ckOp); rr.Code != http.StatusForbidden {
		t.Fatalf("op criar categoria: esperado 403, veio %d", rr.Code)
	}
	if rr, _ = doJSONReq(app, "DELETE", "/api/material/categorias/999999", nil, c.ckOp); rr.Code != http.StatusForbidden {
		t.Fatalf("op excluir categoria: esperado 403, veio %d", rr.Code)
	}
	if rr, res = doJSONReq(app, "POST", "/api/material/categorias", map[string]any{"nome": "Cat Pelo Enc"}, c.ckEnc); rr.Code != http.StatusOK {
		t.Fatalf("enc criar categoria: esperado 200, veio %d (%v)", rr.Code, res)
	}

	// ---------- GERENTE/ENC seguem GRUPO (não-regressão) ----------
	rr, res = doJSONReq(app, "GET", "/api/material/itens", nil, c.ckGer)
	if rr.Code != http.StatusOK {
		t.Fatalf("gerente itens: esperado 200, veio %d", rr.Code)
	}
	if got := f7IDs(res, "itens"); !got[c.itemS1] || !got[c.itemS2] || !got[c.itemG] {
		t.Fatalf("gerente devia ver o GRUPO inteiro (s1+s2+carga geral): %v", got)
	}
	rr, res = doJSONReq(app, "GET", "/api/material/itens", nil, c.ckEnc)
	if rr.Code != http.StatusOK {
		t.Fatalf("enc itens: esperado 200, veio %d", rr.Code)
	}
	if got := f7IDs(res, "itens"); !got[c.itemS2] {
		t.Fatalf("enc devia ver o GRUPO inteiro (item s2 presente): %v", got)
	}
	if rr, _ = doJSONReq(app, "POST", "/api/material/itens", map[string]any{"id": c.itemS2, "nome": "Fuzil Bravo Ger", "codigo_patrimonio": "F7-0002"}, c.ckGer); rr.Code != http.StatusOK {
		t.Fatalf("gerente edita item de qualquer setor: esperado 200, veio %d", rr.Code)
	}

	// ---------- OPERADOR SEM SETOR: 403 com a mensagem única ----------
	for _, r := range []struct{ metodo, path string }{
		{"GET", "/api/material/itens"},
		{"GET", "/api/material/cautelas"},
		{"GET", "/api/material/conferencias"},
		{"GET", "/api/material/inventario/pdf"},
		{"POST", "/api/material/itens"},
		{"POST", "/api/material/cautelar"},
	} {
		rr, res := doJSONReq(app, r.metodo, r.path, map[string]any{"item_id": c.itemS1, "pessoa_id": c.pA}, c.ckOpSem)
		if rr.Code != http.StatusForbidden {
			t.Errorf("op sem setor %s %s: esperado 403, veio %d (%v)", r.metodo, r.path, rr.Code, res)
			continue
		}
		if !strings.Contains(rr.Body.String(), "conta sem setor atribuído — solicite ao gerente/encarregado") {
			t.Errorf("op sem setor %s %s: mensagem única ausente: %s", r.metodo, r.path, rr.Body.String())
		}
	}
	// Integração F4×F7: a guarda de MIDDLEWARE (exigeSetorOperador no
	// authMaterial, onda B) vence para operador sem setor — TODA rota do
	// módulo 403, inclusive responsáveis (o "escopo grupo" do dado vale no
	// recorte, não no acesso; acesso sem setor é bloqueio, doutrina do comando).
	rr, res = doJSONReq(app, "GET", "/api/material/responsaveis", nil, c.ckOpSem)
	if rr.Code != http.StatusForbidden || !strings.Contains(rr.Body.String(), "conta sem setor atribuído") {
		t.Fatalf("op sem setor GET responsaveis: esperado 403 com mensagem da guarda, veio %d (%v)", rr.Code, res)
	}
	// Operador COM setor mantém a leitura (recorte de grupo da designação).
	if rr, _ = doJSONReq(app, "GET", "/api/material/responsaveis", nil, c.ckOp); rr.Code != http.StatusOK {
		t.Fatalf("op com setor GET responsaveis: esperado 200, veio %d", rr.Code)
	}
}

// TestF7ConferenciaOperadorSetorProprio: concluir/reabrir/pré-fechamento — o
// operador passa a ser setor-bound (furo espelhado do chefe).
func TestF7ConferenciaOperadorSetorProprio(t *testing.T) {
	app, st, cleanup := setupTestApp(t)
	defer cleanup()
	gid, sA, sB, _, _, _, ckGer := f6SetupGrupo(t, app, st, "f7cf")

	criaUsuarioTeste(t, st, "f7_cf_op", "senha-op", "operador")
	if _, err := st.db.Exec(`UPDATE usuarios SET grupo_id = ?, setor_id = ? WHERE login = 'f7_cf_op'`, gid, sA); err != nil {
		t.Fatalf("vincular operador: %v", err)
	}
	ckOp := loginAs(t, app, "f7_cf_op", "senha-op")
	criaUsuarioTeste(t, st, "f7_cf_opsem", "senha-ops", "operador")
	if _, err := st.db.Exec(`UPDATE usuarios SET grupo_id = ? WHERE login = 'f7_cf_opsem'`, gid); err != nil {
		t.Fatalf("vincular operador sem setor: %v", err)
	}
	ckOpSem := loginAs(t, app, "f7_cf_opsem", "senha-ops")

	if rr, res := doJSONReq(app, "POST", "/api/conferencia/iniciar", map[string]any{"nome": "Conf F7"}, ckGer); rr.Code != http.StatusOK {
		t.Fatalf("cenário: gerente inicia conferência (200), veio %d (%v)", rr.Code, res)
	}
	var cid int64
	if err := st.db.QueryRow(`SELECT id FROM conferencias WHERE status = 'aberta' AND grupo_id = ? ORDER BY id DESC LIMIT 1`, gid).Scan(&cid); err != nil {
		t.Fatalf("id conferência: %v", err)
	}

	// setor ALHEIO → 403 nas três
	var rr *httptest.ResponseRecorder
	rr, _ = doJSONReq(app, "POST", fmt.Sprintf("/api/conferencia/%d/setor/%d/concluir", cid, sB), nil, ckOp)
	if rr.Code != http.StatusForbidden {
		t.Fatalf("op concluir setor alheio: esperado 403, veio %d", rr.Code)
	}
	rr, _ = doJSONReq(app, "GET", fmt.Sprintf("/api/conferencia/%d/setor/%d/pre_fechamento", cid, sB), nil, ckOp)
	if rr.Code != http.StatusForbidden {
		t.Fatalf("op pré-fechamento setor alheio: esperado 403, veio %d", rr.Code)
	}
	rr, _ = doJSONReq(app, "POST", fmt.Sprintf("/api/conferencia/%d/setor/%d/reabrir", cid, sB), nil, ckOp)
	if rr.Code != http.StatusForbidden {
		t.Fatalf("op reabrir setor alheio: esperado 403, veio %d", rr.Code)
	}

	// PRÓPRIO setor → 200 nas três
	var res map[string]any
	rr, res = doJSONReq(app, "GET", fmt.Sprintf("/api/conferencia/%d/setor/%d/pre_fechamento", cid, sA), nil, ckOp)
	if rr.Code != http.StatusOK {
		t.Fatalf("op pré-fechamento próprio: esperado 200, veio %d (%v)", rr.Code, res)
	}
	rr, _ = doJSONReq(app, "POST", fmt.Sprintf("/api/conferencia/%d/setor/%d/concluir", cid, sA), nil, ckOp)
	if rr.Code != http.StatusOK {
		t.Fatalf("op concluir próprio: esperado 200, veio %d", rr.Code)
	}
	rr, _ = doJSONReq(app, "POST", fmt.Sprintf("/api/conferencia/%d/setor/%d/reabrir", cid, sA), nil, ckOp)
	if rr.Code != http.StatusOK {
		t.Fatalf("op reabrir próprio: esperado 200, veio %d", rr.Code)
	}

	// operador SEM setor → 403 com a mensagem única
	rr, _ = doJSONReq(app, "POST", fmt.Sprintf("/api/conferencia/%d/setor/%d/concluir", cid, sA), nil, ckOpSem)
	if rr.Code != http.StatusForbidden || !strings.Contains(rr.Body.String(), "conta sem setor atribuído — solicite ao gerente/encarregado") {
		t.Fatalf("op sem setor concluir: esperado 403 com mensagem única, veio %d (%s)", rr.Code, rr.Body.String())
	}
	rr, _ = doJSONReq(app, "GET", fmt.Sprintf("/api/conferencia/%d/setor/%d/pre_fechamento", cid, sA), nil, ckOpSem)
	if rr.Code != http.StatusForbidden || !strings.Contains(rr.Body.String(), "conta sem setor atribuído — solicite ao gerente/encarregado") {
		t.Fatalf("op sem setor pré-fechamento: esperado 403 com mensagem única, veio %d (%s)", rr.Code, rr.Body.String())
	}

	// não-regressão do gerente (grupo inteiro)
	if rr, _ := doJSONReq(app, "POST", fmt.Sprintf("/api/conferencia/%d/setor/%d/concluir", cid, sB), nil, ckGer); rr.Code != http.StatusOK {
		t.Fatalf("gerente concluir setor alheio a si: esperado 200, veio %d", rr.Code)
	}
}

// TestF7MaterialChefeNivel: doutrina de NÍVEL (onda v1.6.0-contextos) — o
// chefe de setor ADICIONA/CRIA/ATUALIZA o material do PRÓPRIO setor (visível a
// nível grupo pelo enc/gerente) e NÃO exclui nem baixa (ato do gerente ou
// encarregado de material); cautela/descautela do próprio setor MANTIDAS
// (interpretação registrada no hMaterialCautelar/hMaterialDevolver).
func TestF7MaterialChefeNivel(t *testing.T) {
	app, st, cleanup := setupTestApp(t)
	defer cleanup()
	c := f7SetupMaterial(t, app, st)

	// chefe comandando o setor s1: papel + setor de cadastro + linha de COMANDO
	// em chefe_setores (fonte única da chefia, v1.5.4-D1/R-12).
	criaUsuarioTeste(t, st, "f7_chefe", "senha-chefe", "chefe_setor")
	if _, err := st.db.Exec(`UPDATE usuarios SET grupo_id = ?, setor_id = ? WHERE login = 'f7_chefe'`, c.gid, c.s1); err != nil {
		t.Fatalf("vincular chefe: %v", err)
	}
	var idChefe int64
	if err := st.db.QueryRow(`SELECT id FROM usuarios WHERE login = 'f7_chefe'`).Scan(&idChefe); err != nil {
		t.Fatalf("id chefe: %v", err)
	}
	materializaComandoSetor(t, st, idChefe, c.gid, c.s1)
	ckChefe := loginAs(t, app, "f7_chefe", "senha-chefe")

	// ---------- LEITURA: 200 COM recorte (o "chefe 403 no material" da era
	// blindagem virou acesso recortado — onda_material_blindagem_test ajustado
	// na mesma onda).
	rr, res := doJSONReq(app, "GET", "/api/material/itens", nil, ckChefe)
	if rr.Code != http.StatusOK {
		t.Fatalf("chefe itens: esperado 200 com recorte, veio %d (%v)", rr.Code, res)
	}
	if ids := f7IDs(res, "itens"); !ids[c.itemS1] || ids[c.itemS2] || ids[c.itemG] {
		t.Fatalf("chefe devia ver SÓ o item do próprio setor: vistos=%v (s1=%d s2=%d geral=%d)", ids, c.itemS1, c.itemS2, c.itemG)
	}

	// ---------- SAVE: próprio setor 200 — nasce SEMPRE no próprio (corpo
	// com setor alheio é IGNORADO, mesma régua do operador).
	rr, res = doJSONReq(app, "POST", "/api/material/itens", map[string]any{"nome": "Item do Chefe", "codigo_patrimonio": "F7-CH-1", "setor_id": c.s2}, ckChefe)
	if rr.Code != http.StatusOK {
		t.Fatalf("chefe criar item (corpo setor alheio): esperado 200, veio %d (%v)", rr.Code, res)
	}
	itemChefe := int64(res["id"].(float64))
	var setorNasc *int64
	if err := st.db.QueryRow(`SELECT setor_id FROM material_itens WHERE id = ?`, itemChefe).Scan(&setorNasc); err != nil || setorNasc == nil || *setorNasc != c.s1 {
		t.Fatalf("item do chefe devia nascer no setor próprio (%d), veio %v (%v)", c.s1, setorNasc, err)
	}
	// editar o PRÓPRIO → 200
	if rr, _ = doJSONReq(app, "POST", "/api/material/itens", map[string]any{"id": itemChefe, "nome": "Item do Chefe Editado", "codigo_patrimonio": "F7-CH-1", "quantidade": 5}, ckChefe); rr.Code != http.StatusOK {
		t.Fatalf("chefe editar próprio: esperado 200, veio %d", rr.Code)
	}
	// item de OUTRO setor → não é alcançado (o UPDATE não bate no WHERE do
	// recorte; 404 honesto do P1-2 — a recusa explícita da doutrina, o 403,
	// fica no Del/baixa logo abaixo).
	if rr, _ = doJSONReq(app, "POST", "/api/material/itens", map[string]any{"id": c.itemS2, "nome": "Hackeado Pelo Chefe", "codigo_patrimonio": "F7-0002"}, ckChefe); rr.Code != http.StatusNotFound {
		t.Fatalf("chefe editar item alheio: esperado 404 (P1-2), veio %d", rr.Code)
	}

	// ---------- DEL / BAIXA: 403 MESMO no próprio setor (ato do gerente/enc) ----------
	rr, _ = doJSONReq(app, "DELETE", fmt.Sprintf("/api/material/itens/%d", itemChefe), nil, ckChefe)
	if rr.Code != http.StatusForbidden || !strings.Contains(rr.Body.String(), "exclusão e baixa de material é ato do gerente ou encarregado de material") {
		t.Fatalf("chefe excluir próprio: esperado 403 com a mensagem da doutrina, veio %d (%s)", rr.Code, rr.Body.String())
	}
	if rr, _ = doJSONReq(app, "DELETE", fmt.Sprintf("/api/material/itens/%d?modo=baixar", itemChefe), nil, ckChefe); rr.Code != http.StatusForbidden {
		t.Fatalf("chefe baixar próprio: esperado 403, veio %d", rr.Code)
	}
	// não-regressão do gerente: exclui item de qualquer setor do grupo (200)
	if rr, _ = doJSONReq(app, "DELETE", fmt.Sprintf("/api/material/itens/%d", itemChefe), nil, c.ckGer); rr.Code != http.StatusOK {
		t.Fatalf("gerente excluir item do setor do chefe: esperado 200, veio %d", rr.Code)
	}

	// ---------- CAUTELA/DESCAUTELA: chefe MANTÉM acesso (próprio setor) ----------
	if rr, res = doJSONReq(app, "POST", "/api/material/cautelar", map[string]any{"item_id": c.itemS1, "pessoa_id": c.pA}, ckChefe); rr.Code != http.StatusOK {
		t.Fatalf("chefe cautelar próprio: esperado 200, veio %d (%v)", rr.Code, res)
	}
	cautChefe := int64(res["cautela_id"].(float64))
	if rr, _ = doJSONReq(app, "POST", "/api/material/cautelar", map[string]any{"item_id": c.itemS2, "pessoa_id": c.pB}, ckChefe); rr.Code != http.StatusForbidden {
		t.Fatalf("chefe cautelar alheio: esperado 403, veio %d", rr.Code)
	}
	if rr, _ = doJSONReq(app, "POST", "/api/material/devolver", map[string]any{"cautela_id": cautChefe}, ckChefe); rr.Code != http.StatusOK {
		t.Fatalf("chefe descautelar própria: esperado 200, veio %d", rr.Code)
	}

	// ---------- DESIGNAÇÃO DE RESPONSÁVEIS: segue fechada ao chefe ----------
	rr, _ = doJSONReq(app, "POST", "/api/material/responsaveis", map[string]any{"grupo_id": c.gid, "encarregado_id": c.pA}, ckChefe)
	if rr.Code != http.StatusForbidden {
		t.Fatalf("chefe salvar responsaveis: esperado 403, veio %d", rr.Code)
	}
}
