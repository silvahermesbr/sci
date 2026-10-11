package main

import (
	"fmt"
	"net/http"
	"testing"
)

// Onda UX 0510 (item 4): comentário de aviso NUNCA pode chegar sem "texto" —
// o front renderiza c.texto || c.comentario; antes a API só mandava "text"
// (o comentário virava "undefined" na tela). funcao_nome também deve estar
// sempre presente no payload (string vazia quando não há função).
func TestOndaUxAvisoComentarioCampos(t *testing.T) {
	app, st, cleanup := setupTestApp(t)
	defer cleanup()

	resG, err := st.db.Exec(`INSERT INTO grupos (nome, codigo) VALUES ('Cia UX', 'UX0001')`)
	if err != nil {
		t.Fatalf("criar grupo: %v", err)
	}
	gid, _ := resG.LastInsertId()

	// gerente com grupo vinculado (ordem: vincula grupo ANTES do login;
	// vinculaGrupoDoLogin loga com a senha-padrão "senha-gerente")
	criaUsuarioTeste(t, st, "ux_ger", "senha-gerente", "gerente")
	ckGer := vinculaGrupoDoLogin(t, app, st, "ux_ger", gid)

	// comentarista (v1.6.0 Fase 5: comunicação é por PAPEL — a conta sem
	// função não comenta mural; a persona aqui é o operador do grupo, que
	// conserva o caso de funcao_nome vazio do bug original)
	criaUsuarioTeste(t, st, "ux_op", "senha-ux-123", "operador")
	if _, err := st.db.Exec(`UPDATE usuarios SET grupo_id = ? WHERE login = 'ux_op'`, gid); err != nil {
		t.Fatalf("vincular comentarista: %v", err)
	}
	ckCru := loginAs(t, app, "ux_op", "senha-ux-123")

	// gerente publica o aviso
	rr, res := doJSONReq(app, "POST", "/api/avisos", map[string]any{
		"titulo":   "Aviso UX",
		"conteudo": "Conteúdo do aviso UX.",
	}, ckGer)
	if rr.Code != http.StatusOK {
		t.Fatalf("falha ao publicar aviso: %v", res)
	}
	avisoID := int64(res["id"].(float64))

	// comentarista comenta
	rr, resC := doJSONReq(app, "POST", fmt.Sprintf("/api/avisos/%d/comentar", avisoID), map[string]any{
		"texto": "Comentário de conta sem papel.",
	}, ckCru)
	if rr.Code != http.StatusOK {
		t.Fatalf("falha ao comentar (sem papel): %v", resC)
	}

	// detalhes: comentário deve ter texto NÃO-VAZIO e funcao_nome presente
	rr, res = doJSONReq(app, "GET", fmt.Sprintf("/api/avisos/%d/detalhes", avisoID), nil, ckGer)
	if rr.Code != http.StatusOK {
		t.Fatalf("falha ao obter detalhes: %v", res)
	}
	comps, ok := res["comentarios"].([]any)
	if !ok || len(comps) != 1 {
		t.Fatalf("esperado 1 comentário, obtido: %v", res["comentarios"])
	}
	c := comps[0].(map[string]any)
	txt, _ := c["texto"].(string)
	if txt == "" {
		t.Fatalf("campo 'texto' ausente/vazio (regressão do bug undefined): %v", c)
	}
	if _, presente := c["funcao_nome"]; !presente {
		t.Fatalf("campo 'funcao_nome' ausente no payload do comentário: %v", c)
	}
}
