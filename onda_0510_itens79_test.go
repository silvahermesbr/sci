package main

// Testes da onda itens 7+9 (DELIB-0010, fechamento da fila):
//
//   ITEM 7 — "Arquivo do Grupo" (GET /api/drive/arquivo_grupo):
//     I1. Gerente SEM grupo → 403 (guarda de escopo).
//     I2. Admin → 403 (como em todo o drive operacional; reg escopada).
//     I3. Operador e chefe_setor → 403.
//     I4. Gerente com grupo → 200 e METADADOS corretos (totais, grupo de
//         origem, função v34, autor) — e o download NÃO é concedido aqui
//         (quem não tem acesso ao conteúdo continua sem a rota de download).
//     I5. Escopo inclui GRUPOS SUBORDINADOS ativos (vinculado dos dois lados).
//
//   ITEM 9 — Visão agregada de setores (GET /api/setores/agregado):
//     I6.  Admin → 200 com TODOS os grupos ativos (globais e de grupo).
//     I7.  Gerente → 200 com o PRÓPRIO grupo (escopo); sem grupo → 403.
//     I8.  Contagens: pessoas ativas por setor, contas ativas, chefe atual
//          (papel chefe_setor + usuarios.setor_id), tem_chefe coerente.
//     I9.  hUsuarioPapelAdd: gerente atribui chefe_setor COM setor_id → 200 e
//          usuarios.setor_id gravado (efeito real no banco).
//     I10. hUsuarioPapelAdd: setor de OUTRO grupo → 400 (validação de escopo).

import (
	"fmt"
	"net/http"
	"testing"
)

// itens79Setup: grupo pai + grupo subordinado (vinculação nos DOIS lados, como
// exige gruposSubordinadosAtivos), 2 setores do grupo pai + 1 do subordinado,
// pessoas/contas de teste e o gerente do pai (ainda SEM login vinculado).
func itens79Setup(t *testing.T, app *App, st *Store) (gidPai, gidSub, setA, setB, setSub int64) {
	t.Helper()
	if err := st.db.QueryRow(`INSERT INTO grupos (nome) VALUES ('Grp Pai I79') RETURNING id`).Scan(&gidPai); err != nil {
		t.Fatalf("criar grupo pai: %v", err)
	}
	if err := st.db.QueryRow(`INSERT INTO grupos (nome) VALUES ('Grp Sub I79') RETURNING id`).Scan(&gidSub); err != nil {
		t.Fatalf("criar grupo sub: %v", err)
	}
	// vínculo de subordinação precisa existir nos dois lados (padrão da onda 04/10)
	if _, err := st.db.Exec(`INSERT INTO grupo_vinculos (superior_id, subordinado_id, criado_por_superior, criado_por_subordinado)
		VALUES (?, ?, 1, 1)`, gidPai, gidSub); err != nil {
		t.Fatalf("vincular grupos: %v", err)
	}
	if err := st.db.QueryRow(`INSERT INTO setores (nome, sigla, grupo_id) VALUES ('Setor A79','SA7', ?) RETURNING id`, gidPai).Scan(&setA); err != nil {
		t.Fatalf("criar setor A: %v", err)
	}
	if err := st.db.QueryRow(`INSERT INTO setores (nome, sigla, grupo_id) VALUES ('Setor B79','SB7', ?) RETURNING id`, gidPai).Scan(&setB); err != nil {
		t.Fatalf("criar setor B: %v", err)
	}
	if err := st.db.QueryRow(`INSERT INTO setores (nome, sigla, grupo_id) VALUES ('Setor Sub79','SS7', ?) RETURNING id`, gidSub).Scan(&setSub); err != nil {
		t.Fatalf("criar setor sub: %v", err)
	}

	criaUsuarioTeste(t, st, "ger79", "senha-gerente", "gerente")
	criaUsuarioTeste(t, st, "op79", "senha-gerente", "operador")
	criaUsuarioTeste(t, st, "ch79", "senha-gerente", "chefe_setor")
	if _, err := st.db.Exec(`UPDATE usuarios SET grupo_id = ? WHERE login IN ('ger79','op79','ch79')`, gidPai); err != nil {
		t.Fatalf("vincular grupo: %v", err)
	}
	// SEM linhas em usuario_papeis aqui: quem loga com grupo passa por
	// vinculaGrupoDoLogin(Papel) ANTES do login (DELETE NULL + INSERT) ou a
	// sessão sintetiza o papel já com o grupo (CriarSessaoComPapel).
	// Pré-criar linha grupo-NULL dava UNIQUE 2067 / sessão sem escopo. I6
	// insere a linha do chefe na hora p/ provar chefe real no escopo.

	// pessoas no banco de pessoal: 2 ativas no setor A, 1 inativa no setor A,
	// 1 ativa no setor do subordinado (prova a contagem por escopo)
	for i, s := range []struct {
		guerra string
		setor  int64
		status string
	}{{"Pessoal A1", setA, "ativo"}, {"Pessoal A2", setA, "ativo"}, {"Pessoal A3", setA, "inativo"}, {"Pessoal S1", setSub, "ativo"}} {
		_ = i
		if _, err := st.db.Exec(`INSERT INTO pessoas (nome_guerra, nome_completo, setor_id, grupo_id, status) VALUES (?, ?, ?, ?, ?)`,
			s.guerra, s.guerra+" Completo", s.setor, gidPai, s.status); err != nil {
			t.Fatalf("criar pessoa %s: %v", s.guerra, err)
		}
	}
	// conta de chefe JÁ no setor A (chefe visível na visão agregada)
	if _, err := st.db.Exec(`UPDATE usuarios SET setor_id = ? WHERE login = 'ch79'`, setA); err != nil {
		t.Fatalf("setor do ch79: %v", err)
	}
	return gidPai, gidSub, setA, setB, setSub
}

// buscaSetorAgreg: devolve o mapa do setor pelo id dentro da resposta agregada.
func buscaSetorAgreg(t *testing.T, res map[string]any, id int64) map[string]any {
	t.Helper()
	lista, ok := res["setores"].([]any)
	if !ok {
		t.Fatalf("payload sem 'setores': %v", res)
	}
	for _, it := range lista {
		if m, ok := it.(map[string]any); ok && int64(m["id"].(float64)) == id {
			return m
		}
	}
	t.Fatalf("setor %d não veio na visão agregada: %v", id, res)
	return nil
}

// ---------- ITEM 7: arquivo do grupo ----------

func TestI1ArquivoGrupoGerenteSemGrupo403(t *testing.T) {
	app, st, cleanup := setupTestApp(t)
	defer cleanup()
	// gerente SEM grupo: guarda → 403
	criaUsuarioTeste(t, st, "ger79solto", "senha-gerente", "gerente")
	ck := loginAs(t, app, "ger79solto", "senha-gerente")
	rr, res := doJSONReq(app, "GET", "/api/drive/arquivo_grupo", nil, ck)
	if rr.Code != http.StatusForbidden {
		t.Fatalf("gerente sem grupo: esperado 403, veio %d %v", rr.Code, res)
	}
}

func TestI2ArquivoGrupoAdmin403(t *testing.T) {
	app, st, cleanup := setupTestApp(t)
	defer cleanup()
	itens79Setup(t, app, st)
	ckAdm := loginAs(t, app, "admin", "admin123")
	rr, res := doJSONReq(app, "GET", "/api/drive/arquivo_grupo", nil, ckAdm)
	if rr.Code != http.StatusForbidden {
		t.Fatalf("admin no arquivo do grupo: esperado 403, veio %d %v", rr.Code, res)
	}
}

func TestI3ArquivoGrupoOpEChefe403(t *testing.T) {
	app, st, cleanup := setupTestApp(t)
	defer cleanup()
	gidPai, _, _, _, _ := itens79Setup(t, app, st)
	ckOp := vinculaGrupoDoLoginPapel(t, app, st, "op79", gidPai, "senha-gerente")
	rr, res := doJSONReq(app, "GET", "/api/drive/arquivo_grupo", nil, ckOp)
	if rr.Code != http.StatusForbidden {
		t.Fatalf("operador: esperado 403, veio %d %v", rr.Code, res)
	}
	ckCh := vinculaGrupoDoLoginPapel(t, app, st, "ch79", gidPai, "senha-gerente")
	rr, res = doJSONReq(app, "GET", "/api/drive/arquivo_grupo", nil, ckCh)
	if rr.Code != http.StatusForbidden {
		t.Fatalf("chefe_setor: esperado 403, veio %d %v", rr.Code, res)
	}
}

func TestI4ArquivoGrupoGerente200Metadados(t *testing.T) {
	app, st, cleanup := setupTestApp(t)
	defer cleanup()
	gidPai, _, _, _, _ := itens79Setup(t, app, st)
	ckGer := vinculaGrupoDoLogin(t, app, st, "ger79", gidPai)

	// pasta + arquivo do grupo pai, com função v34, e 1 item no subordinado
	var fid int64
	if err := st.db.QueryRow(`INSERT INTO funcoes (nome) VALUES ('Cmdo I79') RETURNING id`).Scan(&fid); err != nil {
		t.Fatalf("criar função: %v", err)
	}
	criaPastaDrive(t, st, gidPai, "Pasta Pai I79", nil, "ger79")
	if _, err := st.db.Exec(`UPDATE drive_pastas SET funcao_id = ? WHERE nome = 'Pasta Pai I79'`, fid); err != nil {
		t.Fatalf("função na pasta: %v", err)
	}
	criaArquivoDriveEm(t, st, gidPai, "doc79.pdf", "doc79_arm.pdf", "ger79", nil)

	rr, res := doJSONReq(app, "GET", "/api/drive/arquivo_grupo", nil, ckGer)
	if rr.Code != http.StatusOK {
		t.Fatalf("gerente no arquivo do grupo: esperado 200, veio %d %v", rr.Code, res)
	}
	if tot, _ := res["total_pastas"].(float64); tot != 1 {
		t.Fatalf("total_pastas: esperado 1, veio %v", res["total_pastas"])
	}
	if tot, _ := res["total_arquivos"].(float64); tot != 1 {
		t.Fatalf("total_arquivos: esperado 1, veio %v", res["total_arquivos"])
	}
	pastas, _ := res["pastas"].([]any)
	if len(pastas) != 1 {
		t.Fatalf("pastas: esperado 1, veio %v", res["pastas"])
	}
	p := pastas[0].(map[string]any)
	if p["funcao_nome"] != "Cmdo I79" {
		t.Fatalf("funcao_nome v34: esperado 'Cmdo I79', veio %v", p["funcao_nome"])
	}
	if p["grupo_nome"] != "Grp Pai I79" {
		t.Fatalf("grupo_nome: esperado 'Grp Pai I79', veio %v", p["grupo_nome"])
	}

	// o download NÃO é concedido pela visão: guarda normal (checarAcessoArquivo)
	// — gerente de OUTRO grupo não baixa o arquivo só porque existe a rota nova
	if err := st.db.QueryRow(`INSERT INTO grupos (nome) VALUES ('Grp Alheio I79') RETURNING id`).Scan(&fid); err != nil {
		t.Fatalf("criar grupo alheio: %v", err)
	}
	criaUsuarioTeste(t, st, "ger79fora", "senha-gerente", "gerente")
	if _, err := st.db.Exec(`UPDATE usuarios SET grupo_id = ? WHERE login = 'ger79fora'`, fid); err != nil {
		t.Fatalf("vincular ger79fora: %v", err)
	}
	if _, err := st.db.Exec(`UPDATE usuario_papeis SET grupo_id = ? WHERE usuario_id = (SELECT id FROM usuarios WHERE login = 'ger79fora')`, fid); err != nil {
		t.Fatalf("papel ger79fora: %v", err)
	}
	ckFora := loginAs(t, app, "ger79fora", "senha-gerente")
	var arqID int64
	_ = st.db.QueryRow(`SELECT id FROM drive_arquivos WHERE nome_original = 'doc79.pdf'`).Scan(&arqID)
	rr2, _ := doJSONReq(app, "GET", fmt.Sprintf("/api/drive/download/%d", arqID), nil, ckFora)
	if rr2.Code == http.StatusOK {
		t.Fatalf("download vazou p/ gerente de outro grupo (código %d)", rr2.Code)
	}
}

func TestI5ArquivoGrupoEscopoSubordinados(t *testing.T) {
	app, st, cleanup := setupTestApp(t)
	defer cleanup()
	gidPai, gidSub, _, _, _ := itens79Setup(t, app, st)
	ckGer := vinculaGrupoDoLogin(t, app, st, "ger79", gidPai)

	criaPastaDrive(t, st, gidSub, "Pasta Sub I79", nil, "ger79")

	rr, res := doJSONReq(app, "GET", "/api/drive/arquivo_grupo", nil, ckGer)
	if rr.Code != http.StatusOK {
		t.Fatalf("esperado 200, veio %d %v", rr.Code, res)
	}
	pastas, _ := res["pastas"].([]any)
	achouSub := false
	for _, it := range pastas {
		if p := it.(map[string]any); p["grupo_nome"] == "Grp Sub I79" {
			achouSub = true
		}
	}
	if !achouSub {
		t.Fatalf("pasta do grupo SUBORDINADO não veio no escopo: %v", res["pastas"])
	}
	if gs, _ := res["grupos"].(float64); gs < 2 {
		t.Fatalf("grupos no escopo: esperado >= 2 (próprio + subordinado), veio %v", res["grupos"])
	}
}

// ---------- ITEM 9: visão agregada de setores ----------

func TestI6SetoresAgregadoAdminTodos(t *testing.T) {
	app, st, cleanup := setupTestApp(t)
	defer cleanup()
	gidPai, _, setA, setB, _ := itens79Setup(t, app, st)
	// chefe REAL p/ a visão: linha de papel chefe_setor NO grupo + COMANDO
	// materializado em chefe_setores — v1.5.4-D1 (R-12): a agregada leu
	// usuario_papeis+usuarios.setor_id (fonte divergente da UI de catálogo);
	// agora lê chefe_setores, igual ao catálogo (fonte única).
	if _, err := st.db.Exec(`INSERT OR IGNORE INTO usuario_papeis (usuario_id, grupo_id, papel)
		SELECT id, ?, 'chefe_setor' FROM usuarios WHERE login = 'ch79'`, gidPai); err != nil {
		t.Fatalf("papel do chefe no grupo: %v", err)
	}
	var ch79ID int64
	if err := st.db.QueryRow(`SELECT id FROM usuarios WHERE login = 'ch79'`).Scan(&ch79ID); err != nil {
		t.Fatalf("id ch79: %v", err)
	}
	materializaComandoSetor(t, st, ch79ID, gidPai, setA)
	ckAdm := loginAs(t, app, "admin", "admin123")

	rr, res := doJSONReq(app, "GET", "/api/setores/agregado", nil, ckAdm)
	if rr.Code != http.StatusOK {
		t.Fatalf("admin: esperado 200, veio %d %v", rr.Code, res)
	}
	mA := buscaSetorAgreg(t, res, setA)
	mB := buscaSetorAgreg(t, res, setB)

	// pessoas ativas: A tem 2 ativas (1 inativa NÃO conta); B tem 0
	if ps, _ := mA["pessoas"].(float64); ps != 2 {
		t.Fatalf("pessoas ativas do setor A: esperado 2, veio %v", mA["pessoas"])
	}
	if ps, _ := mB["pessoas"].(float64); ps != 0 {
		t.Fatalf("pessoas ativas do setor B: esperado 0, veio %v", mB["pessoas"])
	}
	// ch79 está no setor A com papel chefe_setor → tem_chefe
	if mA["tem_chefe"] != true {
		t.Fatalf("setor A devia ter chefe: %v", mA)
	}
	if mn, _ := mA["chefe_nome"].(string); mn == "" {
		t.Fatalf("chefe_nome vazio no setor A: %v", mA)
	}
	if mB["tem_chefe"] != false {
		t.Fatalf("setor B NÃO devia ter chefe: %v", mB)
	}
}

func TestI7SetoresAgregadoGerenteEscopo(t *testing.T) {
	app, st, cleanup := setupTestApp(t)
	defer cleanup()
	gidPai, _, setA, _, setSub := itens79Setup(t, app, st)
	ckGer := vinculaGrupoDoLogin(t, app, st, "ger79", gidPai)

	rr, res := doJSONReq(app, "GET", "/api/setores/agregado", nil, ckGer)
	if rr.Code != http.StatusOK {
		t.Fatalf("gerente: esperado 200, veio %d %v", rr.Code, res)
	}
	// escopo = próprio grupo (setores A e B); o setor do SUBORDINADO só entra
	// se houver vínculo — que existe, então setSub DEVE vir
	buscaSetorAgreg(t, res, setA)
	buscaSetorAgreg(t, res, setSub)

	// gerente SEM grupo → 403
	criaUsuarioTeste(t, st, "ger79solto", "senha-gerente", "gerente")
	ckSolto := loginAs(t, app, "ger79solto", "senha-gerente")
	rr, res = doJSONReq(app, "GET", "/api/setores/agregado", nil, ckSolto)
	if rr.Code != http.StatusForbidden {
		t.Fatalf("gerente sem grupo: esperado 403, veio %d %v", rr.Code, res)
	}
}

func TestI7bSetoresAgregadoContasDropdown(t *testing.T) {
	app, st, cleanup := setupTestApp(t)
	defer cleanup()
	gidPai, _, _, _, _ := itens79Setup(t, app, st)
	ckGer := vinculaGrupoDoLogin(t, app, st, "ger79", gidPai)

	rr, res := doJSONReq(app, "GET", "/api/setores/agregado", nil, ckGer)
	if rr.Code != http.StatusOK {
		t.Fatalf("esperado 200, veio %d %v", rr.Code, res)
	}
	contas, ok := res["contas"].([]any)
	if !ok || len(contas) < 2 {
		t.Fatalf("contas p/ dropdown (op79+ch79): esperado >= 2, veio %v", res["contas"])
	}
}

// ---------- ITEM 9: hUsuarioPapelAdd com setor_id ----------

func TestI9PapelAddChefeComSetor(t *testing.T) {
	app, st, cleanup := setupTestApp(t)
	defer cleanup()
	gidPai, _, setA, setB, _ := itens79Setup(t, app, st)
	ckGer := vinculaGrupoDoLogin(t, app, st, "ger79", gidPai)

	// operador promovido a chefe_setor JÁ com setor
	var uidOp int64
	if err := st.db.QueryRow(`SELECT id FROM usuarios WHERE login = 'op79'`).Scan(&uidOp); err != nil {
		t.Fatalf("id op79: %v", err)
	}
	rr, res := doJSONReq(app, "POST", fmt.Sprintf("/api/usuarios/%d/papeis", uidOp),
		map[string]any{"papel": "chefe_setor", "grupo_id": gidPai, "setor_id": setA}, ckGer)
	if rr.Code != http.StatusOK {
		t.Fatalf("papel chefe_setor com setor: esperado 200, veio %d %v", rr.Code, res)
	}
	var setorGravado int64
	if err := st.db.QueryRow(`SELECT COALESCE(setor_id,0) FROM usuarios WHERE id = ?`, uidOp).Scan(&setorGravado); err != nil {
		t.Fatalf("ler setor gravado: %v", err)
	}
	if setorGravado != setA {
		t.Fatalf("usuarios.setor_id: esperado %d, veio %d (efeito não aplicado)", setA, setorGravado)
	}

	// setor de OUTRO grupo → 400
	var gidOutro int64
	if err := st.db.QueryRow(`INSERT INTO grupos (nome) VALUES ('Grp Outro I79') RETURNING id`).Scan(&gidOutro); err != nil {
		t.Fatalf("criar grupo outro: %v", err)
	}
	var setorOutro int64
	if err := st.db.QueryRow(`INSERT INTO setores (nome, grupo_id) VALUES ('Setor Outro I79', ?) RETURNING id`, gidOutro).Scan(&setorOutro); err != nil {
		t.Fatalf("criar setor outro: %v", err)
	}
	rr, res = doJSONReq(app, "POST", fmt.Sprintf("/api/usuarios/%d/papeis", uidOp),
		map[string]any{"papel": "chefe_setor", "grupo_id": gidPai, "setor_id": setorOutro}, ckGer)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("setor de outro grupo: esperado 400, veio %d %v", rr.Code, res)
	}
	// setor B (mesmo grupo, sem chefe) também aceita — troca de setor
	rr, res = doJSONReq(app, "POST", fmt.Sprintf("/api/usuarios/%d/papeis", uidOp),
		map[string]any{"papel": "chefe_setor", "grupo_id": gidPai, "setor_id": setB}, ckGer)
	if rr.Code != http.StatusOK {
		t.Fatalf("segunda atribuição (troca de setor): esperado 200, veio %d %v", rr.Code, res)
	}
	if err := st.db.QueryRow(`SELECT COALESCE(setor_id,0) FROM usuarios WHERE id = ?`, uidOp).Scan(&setorGravado); err != nil {
		t.Fatalf("ler setor gravado 2: %v", err)
	}
	if setorGravado != setB {
		t.Fatalf("usuarios.setor_id pós-troca: esperado %d, veio %d", setB, setorGravado)
	}
}

func TestI10PapelAddSetorInexistente400(t *testing.T) {
	app, st, cleanup := setupTestApp(t)
	defer cleanup()
	gidPai, _, _, _, _ := itens79Setup(t, app, st)
	ckGer := vinculaGrupoDoLogin(t, app, st, "ger79", gidPai)

	var uidOp int64
	if err := st.db.QueryRow(`SELECT id FROM usuarios WHERE login = 'op79'`).Scan(&uidOp); err != nil {
		t.Fatalf("id op79: %v", err)
	}
	rr, res := doJSONReq(app, "POST", fmt.Sprintf("/api/usuarios/%d/papeis", uidOp),
		map[string]any{"papel": "chefe_setor", "grupo_id": gidPai, "setor_id": int64(999999)}, ckGer)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("setor inexistente: esperado 400, veio %d %v", rr.Code, res)
	}
}

// vinculaGrupoDoLoginPapel: variante de vinculaGrupoDoLogin p/ logins com senha
// padrão diferente (mantém o fluxo idêntico ao helper da casa).
func vinculaGrupoDoLoginPapel(t *testing.T, app *App, st *Store, login string, gid int64, senha string) *http.Cookie {
	t.Helper()
	if _, err := st.db.Exec(`UPDATE usuarios SET grupo_id = ? WHERE login = ?`, gid, login); err != nil {
		t.Fatalf("grupo do usuário %s: %v", login, err)
	}
	var uid int64
	if err := st.db.QueryRow(`SELECT id FROM usuarios WHERE login = ?`, login).Scan(&uid); err != nil {
		t.Fatalf("usuário %s: %v", login, err)
	}
	if _, err := st.db.Exec(`DELETE FROM usuario_papeis WHERE usuario_id = ? AND grupo_id IS NULL`, uid); err != nil {
		t.Fatalf("remover papel antigo de %s: %v", login, err)
	}
	if _, err := st.db.Exec(`INSERT INTO usuario_papeis (usuario_id, grupo_id, papel)
		SELECT id, ?, papel FROM usuarios WHERE id = ?`, gid, uid); err != nil {
		t.Fatalf("vincular papel de %s ao grupo: %v", login, err)
	}
	return loginAs(t, app, login, senha)
}
