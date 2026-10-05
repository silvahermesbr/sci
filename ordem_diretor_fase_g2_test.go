package main

// ordem_diretor_fase_g2_test.go — Fase G2 (ordem 04/10): a FUNÇÃO do usuário
// determina os módulos de gestão. Prova: /api/me devolve funcao_nome da função
// global e do papel ativo; create de usuário aceita funcao_id do catálogo do
// grupo e recusa função de outro grupo.

import (
	"net/http"
	"testing"
)

func TestG2MeDevolveFuncaoGlobal(t *testing.T) {
	app, st, cleanup := setupTestApp(t)
	defer cleanup()
	admin := loginAs(t, app, "admin", "admin123")
	gid, _ := criaGrupo(t, app, admin, "G G2 Funcao", "ger_g2fun")

	var fid int64
	if err := st.db.QueryRow(`INSERT INTO funcoes (nome, grupo_id) VALUES ('Encarregado de Pessoal G2', ?) RETURNING id`, gid).Scan(&fid); err != nil {
		t.Fatalf("criar função: %v", err)
	}
	// conta criada com a função (vínculos ANTES do login: sessão congela papel)
	criaUsuarioTeste(t, st, "enc_g2", "senha12345", "operador")
	if _, err := st.db.Exec(`UPDATE usuarios SET grupo_id = ?, funcao_id = ? WHERE login = ?`, gid, fid, "enc_g2"); err != nil {
		t.Fatalf("vincular função: %v", err)
	}
	cookie := loginAs(t, app, "enc_g2", "senha12345")

	_, res := doJSONReq(app, "GET", "/api/me", nil, cookie)
	u := res["usuario"].(map[string]any)
	if u["funcao_nome"] == nil || u["funcao_nome"].(string) != "Encarregado de Pessoal G2" {
		t.Fatalf("/api/me deve devolver funcao_nome global; veio %v", u["funcao_nome"])
	}
}

func TestG2CreateComFuncaoDoGrupo(t *testing.T) {
	app, st, cleanup := setupTestApp(t)
	defer cleanup()
	admin := loginAs(t, app, "admin", "admin123")
	gid, ger := criaGrupo(t, app, admin, "G G2 Create", "ger_g2create")

	var fid int64
	if err := st.db.QueryRow(`INSERT INTO funcoes (nome, grupo_id) VALUES ('Chefe Seo Pessoal G2', ?) RETURNING id`, gid).Scan(&fid); err != nil {
		t.Fatalf("criar função: %v", err)
	}

	rr, res := doJSONReq(app, "POST", "/api/usuarios", map[string]any{
		"login": "chefe_g2", "senha": "senha12345", "papel": "chefe_setor", "funcao_id": fid,
	}, ger)
	if rr.Code != http.StatusOK {
		t.Fatalf("gerente criar chefe c/ função: %d (%v)", rr.Code, res)
	}
	var papelGrupo, papelFuncao *int64
	if err := st.db.QueryRow(`SELECT grupo_id, funcao_id FROM usuario_papeis WHERE usuario_id = (SELECT id FROM usuarios WHERE login = 'chefe_g2')`).Scan(&papelGrupo, &papelFuncao); err != nil {
		t.Fatalf("ler usuario_papeis: %v", err)
	}
	if papelGrupo == nil || *papelGrupo != gid || papelFuncao == nil || *papelFuncao != fid {
		t.Fatalf("usuario_papeis deve ter grupo=%d funcao=%d; veio grupo=%v funcao=%v", gid, fid, papelGrupo, papelFuncao)
	}
}

func TestG2CreateRecusaFuncaoDeOutroGrupo(t *testing.T) {
	app, st, cleanup := setupTestApp(t)
	defer cleanup()
	admin := loginAs(t, app, "admin", "admin123")
	_, ger := criaGrupo(t, app, admin, "G G2 A", "ger_g2a")
	gidB, _ := criaGrupo(t, app, admin, "G G2 B", "ger_g2b")

	var fidB int64
	if err := st.db.QueryRow(`INSERT INTO funcoes (nome, grupo_id) VALUES ('Funcao So Do B', ?) RETURNING id`, gidB).Scan(&fidB); err != nil {
		t.Fatalf("criar função B: %v", err)
	}

	rr, res := doJSONReq(app, "POST", "/api/usuarios", map[string]any{
		"login": "chefe_fora", "senha": "senha12345", "papel": "chefe_setor", "funcao_id": fidB,
	}, ger)
	if rr.Code != http.StatusForbidden {
		t.Fatalf("função de outro grupo deve dar 403, veio %d (%v)", rr.Code, res)
	}
	// e a conta NÃO pode ter sido criada
	var n int
	_ = st.db.QueryRow(`SELECT COUNT(*) FROM usuarios WHERE login = 'chefe_fora'`).Scan(&n)
	if n != 0 {
		t.Fatalf("conta não deve existir após recusa de função estranha (n=%d)", n)
	}
}
