package main

// onda_v160_f2_test.go — v1.6.0 Fase 2: os poderes de encarregado passam a
// seguir o CONTEXTO ATIVO da sessão (u.Papel == 'enc_pessoal'/'enc_material' —
// a linha materializada em usuario_papeis; a query por request em
// funcao_membros saiu). A segregação pedida: no contexto operador, operador é
// operador (setor); no contexto enc_*, é encarregado. Quem tem os dois
// contextos troca no dropdown (POST /api/sessao/contexto, mesmo cookie).
//
// Persona (padrão da casa: setupTestApp + loginAs + doJSONReq; positivo E negativo):
//   (a) operador com cadeira enc_material: no CONTEXTO operador, material 403
//       e pessoal 403; troca de contexto → 200 e /api/me resolve 'enc_material'.
//   (b) no CONTEXTO enc_material: material 200; conferência/marcar 403 e
//       pessoal 403 (enc_material NÃO é pessoal — conferência segue dele).
//   (c) enc_material PURO: login resolve o contexto (papel='enc_material');
//       material 200; pessoal 403.
//   (d) chefe_setor com cadeira enc_pessoal: no contexto chefe, poder de setor
//       comandado (marca no próprio setor) SEM poder de pessoal; troca para
//       enc_pessoal → pessoal 200 (e marca continua, agora sem corte de setor).
//
// Fixtures: designação semeada por SQL + MATERIALIZAÇÃO re-executando a v45
// (padrão v45Reexecuta da Fase 1 — o mundo que a migração encontra). A Fase 3
// sincroniza o handler (set/del) e a re-execução vira no-op idempotente.

import (
	"net/http"
	"testing"
)

// f2TrocaContexto: POST /api/sessao/contexto com o mesmo cookie (a troca NUNCA
// re-login) — a sessão re-chaveia e /api/me passa a resolver o novo papel.
func f2TrocaContexto(t *testing.T, app *App, ck *http.Cookie, papelID int64) {
	t.Helper()
	rr, res := doJSONReq(app, "POST", "/api/sessao/contexto", map[string]any{"papel_id": papelID}, ck)
	if rr.Code != http.StatusOK {
		t.Fatalf("troca de contexto p/ papel %d: esperado 200, veio %d (%v)", papelID, rr.Code, res)
	}
}

// f2LinhaPapel: id da linha de papel do usuário (fixture de grupo único).
func f2LinhaPapel(t *testing.T, st *Store, usuarioID int64, papel string) int64 {
	t.Helper()
	var id int64
	if err := st.db.QueryRow(`SELECT id FROM usuario_papeis WHERE usuario_id = ? AND papel = ?`, usuarioID, papel).Scan(&id); err != nil {
		t.Fatalf("linha %s do usuário %d ausente: %v", papel, usuarioID, err)
	}
	return id
}

// f2MePapel: papel ativo resolvido pelo /api/me (contrato do front).
func f2MePapel(t *testing.T, app *App, ck *http.Cookie) string {
	t.Helper()
	rr, res := doJSONReq(app, "GET", "/api/me", nil, ck)
	if rr.Code != http.StatusOK {
		t.Fatalf("/api/me: esperado 200, veio %d (%v)", rr.Code, res)
	}
	u, _ := res["usuario"].(map[string]any)
	if u == nil {
		t.Fatalf("/api/me sem usuario: %v", res)
	}
	p, _ := u["papel"].(string)
	return p
}

// f2SetupGrupo: grupo + cadeiras + gerente. Retorna ids úteis.
func f2SetupGrupo(t *testing.T, app *App, st *Store, sufixo string) (gid, fPess, fMat, idGer int64, ckGer *http.Cookie) {
	t.Helper()
	if err := st.db.QueryRow(`INSERT INTO grupos (nome) VALUES (?) RETURNING id`, "Grp F2 "+sufixo).Scan(&gid); err != nil {
		t.Fatalf("criar grupo: %v", err)
	}
	if err := st.db.QueryRow(`SELECT id FROM funcoes WHERE chave = 'enc_pessoal'`).Scan(&fPess); err != nil {
		t.Fatalf("cadeira enc_pessoal ausente: %v", err)
	}
	if err := st.db.QueryRow(`SELECT id FROM funcoes WHERE chave = 'enc_material'`).Scan(&fMat); err != nil {
		t.Fatalf("cadeira enc_material ausente: %v", err)
	}
	criaUsuarioTeste(t, st, "f2_ger_"+sufixo, "senha-ger", "gerente")
	if _, err := st.db.Exec(`UPDATE usuarios SET grupo_id = ? WHERE login = 'f2_ger_`+sufixo+`'`, gid); err != nil {
		t.Fatalf("vincular gerente: %v", err)
	}
	if err := st.db.QueryRow(`SELECT id FROM usuarios WHERE login = 'f2_ger_`+sufixo+`'`).Scan(&idGer); err != nil {
		t.Fatalf("id gerente: %v", err)
	}
	ckGer = loginAs(t, app, "f2_ger_"+sufixo, "senha-ger")
	return
}

// (a)+(b) operador com cadeira enc_material: o CONTEXTO decide.
func TestF2OperadorComCadeiraTrocaContexto(t *testing.T) {
	app, st, cleanup := setupTestApp(t)
	defer cleanup()
	gid, _, fMat, _, ckGer := f2SetupGrupo(t, app, st, "om")

	// Operador LOGA PRIMEIRO: a linha de sistema nasce antes (id menor) e o
	// login NUNCA aterrissa no contexto enc (CriarSessaoComPapel ORDER BY id).
	criaUsuarioTeste(t, st, "f2_op_om", "senha-op", "operador")
	if _, err := st.db.Exec(`UPDATE usuarios SET grupo_id = ? WHERE login = 'f2_op_om'`, gid); err != nil {
		t.Fatalf("vincular operador: %v", err)
	}
	var idOp int64
	if err := st.db.QueryRow(`SELECT id FROM usuarios WHERE login = 'f2_op_om'`).Scan(&idOp); err != nil {
		t.Fatalf("id operador: %v", err)
	}
	ckOp := loginAs(t, app, "f2_op_om", "senha-op")
	if p := f2MePapel(t, app, ckOp); p != "operador" {
		t.Fatalf("(a) cenário: operador sem cadeira devia logar 'operador', veio %q", p)
	}

	// designado na cadeira enc_material + materialização (v45 materializa o legado)
	if _, err := st.db.Exec(`INSERT INTO funcao_membros (funcao_id, grupo_id, usuario_id, titularidade) VALUES (?,?,?,'titular')`, fMat, gid, idOp); err != nil {
		t.Fatalf("designar operador na cadeira: %v", err)
	}
	v45Reexecuta(t, st)

	// (a) CONTEXTO operador: operador é operador — gestão de pessoal e a rota
	// de CARGO do material (authMaterial: admin/gerente/enc_material) NEGADAS.
	// (GET /api/material/itens segue 200 no contexto operador — acesso de
	// operador PRÉ-EXISTENTE ao módulo, matriz §4 "⚠ ✓ API"; a Fase 7 recorta
	// por setor — fora do escopo desta fase.)
	if rr, _ := doJSONReq(app, "POST", "/api/material/responsaveis", map[string]any{"grupo_id": gid, "setor_id": 0, "encarregado_id": 0, "auxiliar_encarregado_id": 0}, ckOp); rr.Code != http.StatusForbidden {
		t.Fatalf("(a) contexto operador: POST /api/material/responsaveis deve 403, veio %d", rr.Code)
	}
	if rr, _ := doJSONReq(app, "POST", "/api/pessoas", map[string]any{"nome_guerra": "F2OPCTX", "nome_completo": "Operador No Contexto"}, ckOp); rr.Code != http.StatusForbidden {
		t.Fatalf("(a) contexto operador: POST /api/pessoas deve 403, veio %d", rr.Code)
	}

	// (a) troca de contexto pelo dropdown (mesmo cookie) → 200
	f2TrocaContexto(t, app, ckOp, f2LinhaPapel(t, st, idOp, "enc_material"))
	if p := f2MePapel(t, app, ckOp); p != "enc_material" {
		t.Fatalf("(a) pós-troca: /api/me devia resolver 'enc_material', veio %q", p)
	}

	// (b) CONTEXTO enc_material: material ABRE (inclui a rota de cargo); conferência
	// e pessoal NEGADOS
	if rr, res := doJSONReq(app, "GET", "/api/material/itens", nil, ckOp); rr.Code != http.StatusOK {
		t.Fatalf("(b) contexto enc_material: GET /api/material/itens deve 200, veio %d (%v)", rr.Code, res)
	}
	if rr, res := doJSONReq(app, "POST", "/api/material/responsaveis", map[string]any{"grupo_id": gid, "setor_id": 0, "encarregado_id": 0, "auxiliar_encarregado_id": 0}, ckOp); rr.Code != http.StatusOK {
		t.Fatalf("(b) contexto enc_material: POST /api/material/responsaveis deve 200, veio %d (%v)", rr.Code, res)
	}
	if rr, _ := doJSONReq(app, "POST", "/api/conferencia/marcar", map[string]any{"pessoa_id": 1, "situacao": "presente", "verificado": true}, ckOp); rr.Code != http.StatusForbidden {
		t.Fatalf("(b) contexto enc_material: POST /api/conferencia/marcar deve 403, veio %d", rr.Code)
	}
	if rr, _ := doJSONReq(app, "POST", "/api/pessoas", map[string]any{"nome_guerra": "F2ENCMAT", "nome_completo": "Enc Mat No Contexto"}, ckOp); rr.Code != http.StatusForbidden {
		t.Fatalf("(b) contexto enc_material: POST /api/pessoas deve 403, veio %d", rr.Code)
	}

	// (b) volta ao contexto operador: o poder de CARGO some na mesma sessão
	f2TrocaContexto(t, app, ckOp, f2LinhaPapel(t, st, idOp, "operador"))
	if rr, _ := doJSONReq(app, "POST", "/api/material/responsaveis", map[string]any{"grupo_id": gid, "setor_id": 0, "encarregado_id": 0, "auxiliar_encarregado_id": 0}, ckOp); rr.Code != http.StatusForbidden {
		t.Fatalf("(b) volta ao contexto operador: rota de cargo deve 403 de novo, veio %d", rr.Code)
	}
	_ = ckGer
}

// (c) enc_material PURO: login resolve a linha materializada — nasce NO CONTEXTO.
func TestF2EncMaterialPuroNasceNoContexto(t *testing.T) {
	app, st, cleanup := setupTestApp(t)
	defer cleanup()
	gid, _, fMat, _, _ := f2SetupGrupo(t, app, st, "pm")

	criaUsuarioTeste(t, st, "f2_puro_pm", "senha-puro", "")
	if _, err := st.db.Exec(`UPDATE usuarios SET grupo_id = ? WHERE login = 'f2_puro_pm'`, gid); err != nil {
		t.Fatalf("vincular puro: %v", err)
	}
	var idPuro int64
	if err := st.db.QueryRow(`SELECT id FROM usuarios WHERE login = 'f2_puro_pm'`).Scan(&idPuro); err != nil {
		t.Fatalf("id puro: %v", err)
	}
	if _, err := st.db.Exec(`INSERT INTO funcao_membros (funcao_id, grupo_id, usuario_id, titularidade) VALUES (?,?,?,'titular')`, fMat, gid, idPuro); err != nil {
		t.Fatalf("designar puro: %v", err)
	}
	v45Reexecuta(t, st)

	ck := loginAs(t, app, "f2_puro_pm", "senha-puro")
	if p := f2MePapel(t, app, ck); p != "enc_material" {
		t.Fatalf("(c) designado puro devia logar NO CONTEXTO enc_material, veio %q", p)
	}
	// poder de material OK…
	if rr, res := doJSONReq(app, "GET", "/api/material/itens", nil, ck); rr.Code != http.StatusOK {
		t.Fatalf("(c) enc_material puro: GET /api/material/itens deve 200, veio %d (%v)", rr.Code, res)
	}
	// …e pessoal/conferência NEGADOS (cadeira errada)
	if rr, _ := doJSONReq(app, "POST", "/api/pessoas", map[string]any{"nome_guerra": "F2PURO", "nome_completo": "Puro Material"}, ck); rr.Code != http.StatusForbidden {
		t.Fatalf("(c) enc_material puro: POST /api/pessoas deve 403, veio %d", rr.Code)
	}
	if rr, _ := doJSONReq(app, "POST", "/api/conferencia/marcar", map[string]any{"pessoa_id": 1, "situacao": "presente", "verificado": true}, ck); rr.Code != http.StatusForbidden {
		t.Fatalf("(c) enc_material puro: POST /api/conferencia/marcar deve 403, veio %d", rr.Code)
	}
}

// (d) chefe_setor com cadeira enc_pessoal: no contexto CHEFE manda o setor
// comandado (sem pessoal); trocando para enc_pessoal, a gestão de pessoal abre.
func TestF2ChefeComCadeiraEncPessoal(t *testing.T) {
	app, st, cleanup := setupTestApp(t)
	defer cleanup()
	gid, fPess, _, _, ckGer := f2SetupGrupo(t, app, st, "ce")

	var setorID, pA int64
	if err := st.db.QueryRow(`INSERT INTO setores (nome, grupo_id) VALUES ('Setor F2 CE', ?) RETURNING id`, gid).Scan(&setorID); err != nil {
		t.Fatalf("criar setor: %v", err)
	}
	if err := st.db.QueryRow(`INSERT INTO pessoas (nome_guerra, nome_completo, setor_id, grupo_id, status) VALUES ('SILVA F2','Silva F2 Chefe', ?, ?, 'ativo') RETURNING id`, setorID, gid).Scan(&pA); err != nil {
		t.Fatalf("criar pessoa: %v", err)
	}
	criaUsuarioTeste(t, st, "f2_chefe_ce", "senha-ch", "chefe_setor")
	if _, err := st.db.Exec(`UPDATE usuarios SET grupo_id = ?, setor_id = ? WHERE login = 'f2_chefe_ce'`, gid, setorID); err != nil {
		t.Fatalf("vincular chefe: %v", err)
	}
	var idCh int64
	if err := st.db.QueryRow(`SELECT id FROM usuarios WHERE login = 'f2_chefe_ce'`).Scan(&idCh); err != nil {
		t.Fatalf("id chefe: %v", err)
	}
	materializaComandoSetor(t, st, idCh, gid, setorID)

	// chefe LOGA PRIMEIRO (contexto chefe é o default)…
	ckCh := loginAs(t, app, "f2_chefe_ce", "senha-ch")
	// …depois ganha a cadeira enc_pessoal (materializada)
	if _, err := st.db.Exec(`INSERT INTO funcao_membros (funcao_id, grupo_id, usuario_id, titularidade) VALUES (?,?,?,'titular')`, fPess, gid, idCh); err != nil {
		t.Fatalf("designar chefe na cadeira: %v", err)
	}
	v45Reexecuta(t, st)

	// conferência do grupo aberta
	if rr, res := doJSONReq(app, "POST", "/api/conferencia/iniciar", map[string]any{"nome": "Conf F2 CE"}, ckGer); rr.Code != http.StatusOK {
		t.Fatalf("cenário: gerente inicia conferência (200), veio %d (%v)", rr.Code, res)
	}

	// (d) CONTEXTO chefe: marca no PRÓPRIO setor (poder de setor comandado)…
	if rr, res := doJSONReq(app, "POST", "/api/conferencia/marcar", map[string]any{"pessoa_id": pA, "situacao": "presente", "verificado": true}, ckCh); rr.Code != http.StatusOK {
		t.Fatalf("(d) contexto chefe: marcar no próprio setor deve 200, veio %d (%v)", rr.Code, res)
	}
	// …e SEM poder de pessoal (a cadeira não está ativa)
	if rr, _ := doJSONReq(app, "POST", "/api/pessoas", map[string]any{"nome_guerra": "F2CHEFECTX", "nome_completo": "Chefe No Contexto"}, ckCh); rr.Code != http.StatusForbidden {
		t.Fatalf("(d) contexto chefe: POST /api/pessoas deve 403, veio %d", rr.Code)
	}

	// (d) troca para o contexto enc_pessoal → pessoal ABRE
	f2TrocaContexto(t, app, ckCh, f2LinhaPapel(t, st, idCh, "enc_pessoal"))
	if p := f2MePapel(t, app, ckCh); p != "enc_pessoal" {
		t.Fatalf("(d) pós-troca: /api/me devia resolver 'enc_pessoal', veio %q", p)
	}
	if rr, res := doJSONReq(app, "POST", "/api/pessoas", map[string]any{"nome_guerra": "F2CHEFEENC", "nome_completo": "Chefe No Contexto Enc", "status": "ativo"}, ckCh); rr.Code != http.StatusOK {
		t.Fatalf("(d) contexto enc_pessoal: POST /api/pessoas deve 200, veio %d (%v)", rr.Code, res)
	}
	// e no contexto enc a marcação não tem corte de setor (régua de gerente)
	if rr, _ := doJSONReq(app, "POST", "/api/conferencia/marcar", map[string]any{"pessoa_id": pA, "situacao": "atraso", "verificado": true}, ckCh); rr.Code != http.StatusOK {
		t.Fatalf("(d) contexto enc_pessoal: marcar deve continuar 200, veio %d", rr.Code)
	}
}

// Segurança: a troca de contexto NÃO escala — papel não pertencente ao usuário
// é recusado (guarda R-9 do hMudarContexto).
func TestF2TrocaContextoNaoEscala(t *testing.T) {
	app, st, cleanup := setupTestApp(t)
	defer cleanup()
	gid, fPess, _, _, _ := f2SetupGrupo(t, app, st, "ne")

	criaUsuarioTeste(t, st, "f2_a_ne", "senha-a", "")
	if _, err := st.db.Exec(`UPDATE usuarios SET grupo_id = ? WHERE login = 'f2_a_ne'`, gid); err != nil {
		t.Fatalf("vincular a: %v", err)
	}
	var idA int64
	if err := st.db.QueryRow(`SELECT id FROM usuarios WHERE login = 'f2_a_ne'`).Scan(&idA); err != nil {
		t.Fatalf("id a: %v", err)
	}
	if _, err := st.db.Exec(`INSERT INTO funcao_membros (funcao_id, grupo_id, usuario_id, titularidade) VALUES (?,?,?,'auxiliar')`, fPess, gid, idA); err != nil {
		t.Fatalf("designar a: %v", err)
	}
	v45Reexecuta(t, st)
	ckA := loginAs(t, app, "f2_a_ne", "senha-a")

	// linha de papel de OUTREM (gerente do grupo) → 403
	linhaGer := f2LinhaPapel(t, st, func() int64 { var id int64; _ = st.db.QueryRow(`SELECT id FROM usuarios WHERE login = 'f2_ger_ne'`).Scan(&id); return id }(), "gerente")
	if rr, res := doJSONReq(app, "POST", "/api/sessao/contexto", map[string]any{"papel_id": linhaGer}, ckA); rr.Code != http.StatusForbidden {
		t.Fatalf("troca para papel alheio deve 403, veio %d (%v)", rr.Code, res)
	}
	// própria linha enc → 200
	if rr, _ := doJSONReq(app, "POST", "/api/sessao/contexto", map[string]any{"papel_id": f2LinhaPapel(t, st, idA, "enc_pessoal")}, ckA); rr.Code != http.StatusOK {
		t.Fatalf("troca para a própria linha enc deve 200, veio %d", rr.Code)
	}
}
