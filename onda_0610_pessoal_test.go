package main

// Testes da onda 06/10 — itens 3 e 15 da ordem do Diretor (gestão de pessoal):
//   E1. Encarregado gerencia o PRÓPRIO grupo: usuarios/pessoas/catálogos/
//       conferência/nomear chefe → 200; cria conta operador com grupo forçado.
//   E2. Encarregado NÃO escala: criar gerente 403, senha 403, mover 403,
//       rota admin (backup/arquivada) 403.
//   E3. Encarregado de grupo ALHEIO → 403 em pessoa/conta/catálogo/chefe.
//   R1. Regressão: gerente 200 em tudo do pessoal; operador lê mas não escreve;
//       comum sem grupo → 403 na lista de contas; admin continua dono.
//   A1. AUXILIAR espelha o encarregado (mesmos 200); auxiliar alheio 403.
//   F1. Nomeação de função (item 15): encarregado/auxiliar definem/desfazem
//       SÓ a função "auxiliar" do grupo; outra função 403; gerente qualquer.
//   M1. /api/me traz grupo_nome do papel ativo (P3).
//   V1. hMoverConta: operador/comum não movem (buraco fechado); admin move.

import (
	"net/http"
	"strconv"
	"testing"
)

// onda0610Setup: fixture fail-fast — 2 grupos, funções de pessoal em ambos,
// contas de todos os papeis e 1 pessoa no grupo principal.
// R3: poder encarregado/auxiliar vem de DESIGNAÇÃO (funcao_membros), não de
// funcao_id no cadastro.
func onda0610Setup(t *testing.T, app *App, st *Store) (gid, gidFora, fEnc, fAux, fOutra, pSilva, op01ID int64) {
	t.Helper()
	if err := st.db.QueryRow(`INSERT INTO grupos (nome) VALUES ('Grp 0610') RETURNING id`).Scan(&gid); err != nil {
		t.Fatalf("criar grupo: %v", err)
	}
	if err := st.db.QueryRow(`INSERT INTO grupos (nome) VALUES ('Grp 0610 Fora') RETURNING id`).Scan(&gidFora); err != nil {
		t.Fatalf("criar grupo fora: %v", err)
	}
	if err := st.db.QueryRow(`INSERT INTO funcoes (nome, grupo_id, tipo, chave) VALUES ('Encarregado de Pessoal', ?, 'grupo', 'enc_pessoal') RETURNING id`, gid).Scan(&fEnc); err != nil {
		t.Fatalf("criar função encarregado: %v", err)
	}
	if err := st.db.QueryRow(`INSERT INTO funcoes (nome, grupo_id) VALUES ('Auxiliar de Pessoal', ?) RETURNING id`, gid).Scan(&fAux); err != nil {
		t.Fatalf("criar função auxiliar: %v", err)
	}
	if err := st.db.QueryRow(`INSERT INTO funcoes (nome, grupo_id) VALUES ('Furriel', ?) RETURNING id`, gid).Scan(&fOutra); err != nil {
		t.Fatalf("criar função outra: %v", err)
	}

	criaUsuarioTeste(t, st, "enc01", "senha-enc", "")          // encarregado: SEM papel
	criaUsuarioTeste(t, st, "aux01", "senha-aux", "")          // auxiliar: SEM papel
	criaUsuarioTeste(t, st, "encFora", "senha-encf", "")       // encarregado do grupo alheio
	criaUsuarioTeste(t, st, "auxFora", "senha-auxf", "")       // auxiliar do grupo alheio
	criaUsuarioTeste(t, st, "ger01", "senha-ger", "gerente")   // gerente do grupo
	criaUsuarioTeste(t, st, "op01", "senha-op", "operador")    // operador do grupo
	criaUsuarioTeste(t, st, "com01", "senha-com", "")          // comum SEM grupo
	criaUsuarioTeste(t, st, "adm01", "senha-adm", "admin")     // admin

	// Resolve IDs (precisa ser ANTES das designações)
	var enc01ID, aux01ID, encForaID, auxForaID int64
	if err := st.db.QueryRow(`SELECT id FROM usuarios WHERE login='enc01'`).Scan(&enc01ID); err != nil {
		t.Fatalf("id enc01: %v", err)
	}
	if err := st.db.QueryRow(`SELECT id FROM usuarios WHERE login='aux01'`).Scan(&aux01ID); err != nil {
		t.Fatalf("id aux01: %v", err)
	}
	if err := st.db.QueryRow(`SELECT id FROM usuarios WHERE login='encFora'`).Scan(&encForaID); err != nil {
		t.Fatalf("id encFora: %v", err)
	}
	if err := st.db.QueryRow(`SELECT id FROM usuarios WHERE login='auxFora'`).Scan(&auxForaID); err != nil {
		t.Fatalf("id auxFora: %v", err)
	}

	// Vincular grupo (sem funcao_id — R3: poder vem da designação)
	if _, err := st.db.Exec(`UPDATE usuarios SET grupo_id = ? WHERE login IN ('enc01','aux01','ger01','op01')`, gid); err != nil {
		t.Fatalf("vincular grupo gid: %v", err)
	}
	if _, err := st.db.Exec(`UPDATE usuarios SET grupo_id = ? WHERE login IN ('encFora','auxFora')`, gidFora); err != nil {
		t.Fatalf("vincular grupo gidFora: %v", err)
	}

	// R3: designações em funcao_membros (NÃO em usuarios.funcao_id)
	designa := `INSERT INTO funcao_membros (funcao_id, grupo_id, usuario_id, titularidade) VALUES (?,?,?,?)`
	if _, err := st.db.Exec(designa, fEnc, gid, enc01ID, "titular"); err != nil {
		t.Fatalf("designar enc01: %v", err)
	}
	if _, err := st.db.Exec(designa, fEnc, gid, aux01ID, "auxiliar"); err != nil {
		t.Fatalf("designar aux01: %v", err)
	}
	if _, err := st.db.Exec(designa, fEnc, gidFora, encForaID, "titular"); err != nil {
		t.Fatalf("designar encFora: %v", err)
	}
	if _, err := st.db.Exec(designa, fEnc, gidFora, auxForaID, "auxiliar"); err != nil {
		t.Fatalf("designar auxFora: %v", err)
	}

	if err := st.db.QueryRow(`SELECT id FROM usuarios WHERE login = 'op01'`).Scan(&op01ID); err != nil {
		t.Fatalf("id op01: %v", err)
	}
	if err := st.db.QueryRow(`INSERT INTO pessoas (nome_guerra, nome_completo, grupo_id, status) VALUES ('SILVA','Silva Teste 0610', ?, 'ativo') RETURNING id`, gid).Scan(&pSilva); err != nil {
		t.Fatalf("criar pessoa: %v", err)
	}
	return
}

func TestOnda0610EncarregadoGestaoProprioGrupo(t *testing.T) {
	app, st, cleanup := setupTestApp(t)
	defer cleanup()
	gid, _, _, _, fOutra, pSilva, op01ID := onda0610Setup(t, app, st)
	ck := loginAs(t, app, "enc01", "senha-enc")

	// lista de contas do próprio grupo
	if rr, _ := doJSONReq(app, "GET", "/api/usuarios", nil, ck); rr.Code != http.StatusOK {
		t.Fatalf("E1: encarregado lista contas (200), veio %d", rr.Code)
	}
	// cria pessoa no próprio grupo (grupo forçado)
	rr, res := doJSONReq(app, "POST", "/api/pessoas", map[string]any{"nome_guerra": "NOVO06", "nome_completo": "Novo Pessoa 0610"}, ck)
	if rr.Code != http.StatusOK {
		t.Fatalf("E1: encarregado cadastra pessoa (200), veio %d (%v)", rr.Code, res["erro"])
	}
	var gidPessoa int64
	if err := st.db.QueryRow(`SELECT COALESCE(grupo_id,0) FROM pessoas WHERE nome_guerra = 'NOVO06'`).Scan(&gidPessoa); err != nil || gidPessoa != gid {
		t.Fatalf("E1: pessoa nasceu no grupo errado (gid=%d err=%v)", gidPessoa, err)
	}
	// edita pessoa do grupo
	if rr, _ := doJSONReq(app, "PATCH", "/api/pessoas/"+idi(pSilva), map[string]any{"nome_guerra": "SILVA2", "nome_completo": "Silva Editada", "status": "ativo"}, ck); rr.Code != http.StatusOK {
		t.Fatalf("E1: encarregado edita pessoa (200), veio %d", rr.Code)
	}
	// cria conta operador — grupo forçado
	if rr, _ := doJSONReq(app, "POST", "/api/usuarios", map[string]any{"login": "op.novo06", "papel": "operador"}, ck); rr.Code != http.StatusOK {
		t.Fatalf("E1: encarregado cria operador (200), veio %d", rr.Code)
	}
	var gidConta int64
	if err := st.db.QueryRow(`SELECT COALESCE(grupo_id,0) FROM usuarios WHERE login = 'op.novo06'`).Scan(&gidConta); err != nil || gidConta != gid {
		t.Fatalf("E1: conta criada fora do grupo do encarregado (gid=%d err=%v)", gidConta, err)
	}
	// catálogo: renomear função do grupo
	if rr, _ := doJSONReq(app, "PATCH", "/api/catalogo/funcoes/"+idi(fOutra), map[string]any{"nome": "Furriel Renomeada"}, ck); rr.Code != http.StatusOK {
		t.Fatalf("E1: encarregado edita catálogo (200), veio %d", rr.Code)
	}
	// conferência: inicia no próprio grupo (onda 0510 mantida)
	if rr, _ := doJSONReq(app, "POST", "/api/conferencia/iniciar", map[string]any{"nome": "Conf 0610"}, ck); rr.Code != http.StatusOK {
		t.Fatalf("E1: encarregado inicia conferência (200), veio %d", rr.Code)
	}
	// nomear chefe de setor no próprio grupo
	var setorID int64
	if err := st.db.QueryRow(`INSERT INTO setores (nome, grupo_id) VALUES ('S1 0610', ?) RETURNING id`, gid).Scan(&setorID); err != nil {
		t.Fatalf("criar setor: %v", err)
	}
	if rr, _ := doJSONReq(app, "POST", "/api/grupos/"+idi(gid)+"/nomear_chefe", map[string]any{"usuario_id": op01ID, "setor_id": setorID}, ck); rr.Code != http.StatusOK {
		t.Fatalf("E1: encarregado nomeia chefe (200), veio %d", rr.Code)
	}
}

func TestOnda0610EncarregadoSemPoderesAdmin(t *testing.T) {
	app, st, cleanup := setupTestApp(t)
	defer cleanup()
	_, _, _, _, _, _, op01ID := onda0610Setup(t, app, st)
	ck := loginAs(t, app, "enc01", "senha-enc")

	// criar gerente/admin é poder de gerente/admin
	if rr, _ := doJSONReq(app, "POST", "/api/usuarios", map[string]any{"login": "ger.novo", "papel": "gerente"}, ck); rr.Code != http.StatusForbidden {
		t.Fatalf("E2: encarregado cria gerente deve 403, veio %d", rr.Code)
	}
	if rr, _ := doJSONReq(app, "POST", "/api/usuarios", map[string]any{"login": "adm.novo", "papel": "admin"}, ck); rr.Code != http.StatusForbidden {
		t.Fatalf("E2: encarregado cria admin deve 403, veio %d", rr.Code)
	}
	// senha de conta (poder credencial) segue gerente/admin
	if rr, _ := doJSONReq(app, "POST", "/api/usuarios/"+idi(op01ID)+"/senha", map[string]any{"senha": "NovaSenha1"}, ck); rr.Code != http.StatusForbidden {
		t.Fatalf("E2: encarregado redefine senha deve 403, veio %d", rr.Code)
	}
	// mover conta não é dele
	if rr, _ := doJSONReq(app, "PATCH", "/api/usuarios/"+idi(op01ID)+"/mover", map[string]any{"grupo_id": 1}, ck); rr.Code != http.StatusForbidden {
		t.Fatalf("E2: encarregado move conta deve 403, veio %d", rr.Code)
	}
	// rotas admin (auth(true)): backup e excluir arquivada
	if rr, _ := doJSONReq(app, "POST", "/api/backup", map[string]any{}, ck); rr.Code != http.StatusForbidden {
		t.Fatalf("E2: encarregado em rota admin (backup) deve 403, veio %d", rr.Code)
	}
	if rr, _ := doJSONReq(app, "DELETE", "/api/conferencia/arquivada/1", nil, ck); rr.Code != http.StatusForbidden {
		t.Fatalf("E2: encarregado exclui arquivada deve 403, veio %d", rr.Code)
	}
	if rr, _ := doJSONReq(app, "DELETE", "/api/grupos/1", nil, ck); rr.Code != http.StatusForbidden {
		t.Fatalf("E2: encarregado exclui grupo deve 403, veio %d", rr.Code)
	}
	// hash de senhas não vem na lista
	_, res := doJSONReqAny(app, "GET", "/api/usuarios", nil, ck)
	if lista, ok := res.([]any); ok && len(lista) > 0 {
		primeiro := lista[0].(map[string]any)
		if _, tem := primeiro["senhas"]; tem {
			t.Fatalf("E2: lista de contas para encarregado NÃO deve trazer hash de senhas")
		}
	}
}

func TestOnda0610EncarregadoGrupoAlheio403(t *testing.T) {
	app, st, cleanup := setupTestApp(t)
	defer cleanup()
	_, _, fOutra, _, _, pSilva, op01ID := onda0610Setup(t, app, st)
	_ = st
	ck := loginAs(t, app, "encFora", "senha-encf")

	if rr, _ := doJSONReq(app, "PATCH", "/api/pessoas/"+idi(pSilva), map[string]any{"nome_guerra": "X", "nome_completo": "Y", "status": "ativo"}, ck); rr.Code != http.StatusForbidden {
		t.Fatalf("E3: encarregado alheio edita pessoa deve 403, veio %d", rr.Code)
	}
	if rr, _ := doJSONReq(app, "PATCH", "/api/usuarios/"+idi(op01ID), map[string]any{"nome_guerra": "X"}, ck); rr.Code != http.StatusForbidden {
		t.Fatalf("E3: encarregado alheio edita conta deve 403, veio %d", rr.Code)
	}
	if rr, _ := doJSONReq(app, "PATCH", "/api/catalogo/funcoes/"+idi(fOutra), map[string]any{"nome": "Roubada"}, ck); rr.Code != http.StatusForbidden {
		t.Fatalf("E3: encarregado alheio edita catálogo deve 403, veio %d", rr.Code)
	}
}

func TestOnda0610RegressaoGerenteOperadorComumAdmin(t *testing.T) {
	app, st, cleanup := setupTestApp(t)
	defer cleanup()
	gid, _, _, _, fOutra, pSilva, op01ID := onda0610Setup(t, app, st)
	_ = op01ID
	_ = st

	// GERENTE: dono das rotas — segue 200
	ckGer := loginAs(t, app, "ger01", "senha-ger")
	if rr, _ := doJSONReq(app, "GET", "/api/usuarios", nil, ckGer); rr.Code != http.StatusOK {
		t.Fatalf("R1: gerente lista contas (200), veio %d", rr.Code)
	}
	if rr, _ := doJSONReq(app, "POST", "/api/pessoas", map[string]any{"nome_guerra": "GER06", "nome_completo": "Pessoa do Gerente"}, ckGer); rr.Code != http.StatusOK {
		t.Fatalf("R1: gerente cadastra pessoa (200), veio %d", rr.Code)
	}
	if rr, _ := doJSONReq(app, "PATCH", "/api/catalogo/funcoes/"+idi(fOutra), map[string]any{"nome": "Furriel G"}, ckGer); rr.Code != http.StatusOK {
		t.Fatalf("R1: gerente edita catálogo (200), veio %d", rr.Code)
	}
	// OPERADOR: lê a lista (telas de designação), não escreve
	ckOp := loginAs(t, app, "op01", "senha-op")
	if rr, _ := doJSONReq(app, "GET", "/api/usuarios", nil, ckOp); rr.Code != http.StatusOK {
		t.Fatalf("R1: operador lê contas do grupo (200), veio %d", rr.Code)
	}
	if rr, _ := doJSONReq(app, "POST", "/api/pessoas", map[string]any{"nome_guerra": "OP06", "nome_completo": "Não Pode"}, ckOp); rr.Code != http.StatusForbidden {
		t.Fatalf("R1: operador cadastra pessoa deve 403, veio %d", rr.Code)
	}
	if rr, _ := doJSONReq(app, "POST", "/api/usuarios", map[string]any{"login": "x.y", "papel": "operador"}, ckOp); rr.Code != http.StatusForbidden {
		t.Fatalf("R1: operador cria conta deve 403, veio %d", rr.Code)
	}
	// COMUM sem grupo: lista de contas é 403 (escalação fechada)
	ckCom := loginAs(t, app, "com01", "senha-com")
	if rr, _ := doJSONReq(app, "GET", "/api/usuarios", nil, ckCom); rr.Code != http.StatusForbidden {
		t.Fatalf("R1: comum sem grupo lê contas deve 403, veio %d", rr.Code)
	}
	// ADMIN: continua dono de tudo
	ckAdm := loginAs(t, app, "adm01", "senha-adm")
	if rr, _ := doJSONReq(app, "GET", "/api/usuarios", nil, ckAdm); rr.Code != http.StatusOK {
		t.Fatalf("R1: admin lista contas (200), veio %d", rr.Code)
	}
	if rr, _ := doJSONReq(app, "POST", "/api/pessoas", map[string]any{"nome_guerra": "ADM06", "nome_completo": "Pessoa do Admin", "grupo_id": gid}, ckAdm); rr.Code != http.StatusOK {
		t.Fatalf("R1: admin cadastra pessoa (200), veio %d", rr.Code)
	}
	_ = pSilva
}

func TestOnda0610AuxiliarEspelhaEncarregado(t *testing.T) {
	app, st, cleanup := setupTestApp(t)
	defer cleanup()
	_, _, _, _, _, pSilva, op01ID := onda0610Setup(t, app, st)
	_ = st
	ck := loginAs(t, app, "aux01", "senha-aux")

	if rr, _ := doJSONReq(app, "GET", "/api/usuarios", nil, ck); rr.Code != http.StatusOK {
		t.Fatalf("A1: auxiliar lista contas (200), veio %d", rr.Code)
	}
	if rr, _ := doJSONReq(app, "POST", "/api/pessoas", map[string]any{"nome_guerra": "AUX06", "nome_completo": "Pessoa do Auxiliar"}, ck); rr.Code != http.StatusOK {
		t.Fatalf("A1: auxiliar cadastra pessoa (200), veio %d", rr.Code)
	}
	if rr, _ := doJSONReq(app, "POST", "/api/usuarios", map[string]any{"login": "op.aux06", "papel": "chefe_setor"}, ck); rr.Code != http.StatusOK {
		t.Fatalf("A1: auxiliar cria chefe de setor (200), veio %d", rr.Code)
	}
	if rr, _ := doJSONReq(app, "POST", "/api/conferencia/iniciar", map[string]any{"nome": "Conf Aux"}, ck); rr.Code != http.StatusOK {
		t.Fatalf("A1: auxiliar inicia conferência (200), veio %d", rr.Code)
	}
	if rr, _ := doJSONReq(app, "PATCH", "/api/pessoas/"+idi(pSilva), map[string]any{"nome_guerra": "SILVA", "nome_completo": "Silva via Aux", "status": "ativo"}, ck); rr.Code != http.StatusOK {
		t.Fatalf("A1: auxiliar edita pessoa (200), veio %d", rr.Code)
	}
	// auxiliar do grupo ALHEIO continua fora
	ckF := loginAs(t, app, "auxFora", "senha-auxf")
	if rr, _ := doJSONReq(app, "PATCH", "/api/pessoas/"+idi(pSilva), map[string]any{"nome_guerra": "X", "nome_completo": "Y", "status": "ativo"}, ckF); rr.Code != http.StatusForbidden {
		t.Fatalf("A1: auxiliar alheio edita pessoa deve 403, veio %d", rr.Code)
	}
	_ = op01ID
}

func TestOnda0610NomeacaoFuncaoAuxiliar(t *testing.T) {
	app, st, cleanup := setupTestApp(t)
	defer cleanup()
	_, _, _, fAux, fOutra, pSilva, _ := onda0610Setup(t, app, st)
	_ = st
	ckEnc := loginAs(t, app, "enc01", "senha-enc")
	ckAux := loginAs(t, app, "aux01", "senha-aux")
	ckGer := loginAs(t, app, "ger01", "senha-ger")

	// encarregado DEFINE a auxiliar do grupo
	if rr, _ := doJSONReq(app, "PATCH", "/api/pessoas/"+idi(pSilva), map[string]any{"nome_guerra": "SILVA", "nome_completo": "Silva Teste 0610", "status": "ativo", "funcao_id": fAux}, ckEnc); rr.Code != http.StatusOK {
		t.Fatalf("F1: encarregado define auxiliar (200), veio %d", rr.Code)
	}
	// encarregado NÃO define outra função
	if rr, _ := doJSONReq(app, "PATCH", "/api/pessoas/"+idi(pSilva), map[string]any{"nome_guerra": "SILVA", "nome_completo": "Silva Teste 0610", "status": "ativo", "funcao_id": fOutra}, ckEnc); rr.Code != http.StatusForbidden {
		t.Fatalf("F1: encarregado define função qualquer deve 403, veio %d", rr.Code)
	}
	// auxiliar REENVIA a mesma função (edição de campos) → passa
	if rr, _ := doJSONReq(app, "PATCH", "/api/pessoas/"+idi(pSilva), map[string]any{"nome_guerra": "SILVA", "nome_completo": "Silva Editada", "status": "ativo", "funcao_id": fAux}, ckAux); rr.Code != http.StatusOK {
		t.Fatalf("F1: auxiliar reenvia mesma função (200), veio %d", rr.Code)
	}
	// auxiliar troca para outra função → 403
	if rr, _ := doJSONReq(app, "PATCH", "/api/pessoas/"+idi(pSilva), map[string]any{"nome_guerra": "SILVA", "nome_completo": "Silva Editada", "status": "ativo", "funcao_id": fOutra}, ckAux); rr.Code != http.StatusForbidden {
		t.Fatalf("F1: auxiliar define função qualquer deve 403, veio %d", rr.Code)
	}
	// gerente define QUALQUER função
	if rr, _ := doJSONReq(app, "PATCH", "/api/pessoas/"+idi(pSilva), map[string]any{"nome_guerra": "SILVA", "nome_completo": "Silva Teste 0610", "status": "ativo", "funcao_id": fOutra}, ckGer); rr.Code != http.StatusOK {
		t.Fatalf("F1: gerente define função qualquer (200), veio %d", rr.Code)
	}
	// encarregado define a auxiliar de novo (volta) — gerente deixou outra função
	if rr, _ := doJSONReq(app, "PATCH", "/api/pessoas/"+idi(pSilva), map[string]any{"nome_guerra": "SILVA", "nome_completo": "Silva Teste 0610", "status": "ativo", "funcao_id": fAux}, ckEnc); rr.Code != http.StatusOK {
		t.Fatalf("F1: encarregado volta p/ auxiliar (200), veio %d", rr.Code)
	}
	// persistência: função da pessoa é a auxiliar
	var fid int64
	if err := st.db.QueryRow(`SELECT COALESCE(funcao_id,0) FROM pessoas WHERE id = ?`, pSilva).Scan(&fid); err != nil || fid != fAux {
		t.Fatalf("F1: função da pessoa não persistiu como auxiliar (fid=%d err=%v)", fid, err)
	}
}

func TestOnda0610MeTrazGrupoNome(t *testing.T) {
	app, st, cleanup := setupTestApp(t)
	defer cleanup()
	onda0610Setup(t, app, st)
	_ = st
	ck := loginAs(t, app, "ger01", "senha-ger")
	_, res := doJSONReqAny(app, "GET", "/api/me", nil, ck)
	if res == nil {
		t.Fatalf("M1: /api/me sem corpo")
	}
	m := res.(map[string]any)
	if gn, ok := m["grupo_nome"].(string); !ok || gn != "Grp 0610" {
		t.Fatalf("M1: /api/me deve trazer grupo_nome='Grp 0610', veio %v", m["grupo_nome"])
	}
}

func TestOnda0610MoverContasFix(t *testing.T) {
	app, st, cleanup := setupTestApp(t)
	defer cleanup()
	gid, gidFora, _, _, _, _, op01ID := onda0610Setup(t, app, st)
	_ = gid
	_ = st

	// operador não move (buraco fechado)
	ckOp := loginAs(t, app, "op01", "senha-op")
	if rr, _ := doJSONReq(app, "PATCH", "/api/usuarios/"+idi(op01ID)+"/mover", map[string]any{"grupo_id": gidFora}, ckOp); rr.Code != http.StatusForbidden {
		t.Fatalf("V1: operador move conta deve 403, veio %d", rr.Code)
	}
	// admin move
	ckAdm := loginAs(t, app, "adm01", "senha-adm")
	if rr, _ := doJSONReq(app, "PATCH", "/api/usuarios/"+idi(op01ID)+"/mover", map[string]any{"grupo_id": gidFora}, ckAdm); rr.Code != http.StatusOK {
		t.Fatalf("V1: admin move conta (200), veio %d", rr.Code)
	}
}

// idi: helper de formatação de id em path (legibilidade dos testes) — nome
// distinto do i64 da suíte 0510 para não colidir no pacote de testes.
func idi(n int64) string {
	return strconv.FormatInt(n, 10)
}
