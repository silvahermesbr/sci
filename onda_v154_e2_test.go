package main

// onda_v154_e2_test.go — pacote E2 da onda v1.5.4 (defeitos R-5, R-6, R-8,
// R-11, R-15 e R-21 do ARQUITETURA.md §14).
//
//   R-5  — GET /api/usuarios/{id}/foto sem escopo (LGPD): própria grupo 200,
//          outro grupo/sem-grupo 403, admin global 200.
//   R-11 — Drive: MIME do cliente servido inline era stored XSS; agora só a
//          allowlist (PDF/PNG/JPEG/WEBP) desce inline, resto attachment +
//          nosniff SEMPRE + filename sanitizado.
//   R-6  — NUKE de grupo "rico" (uma linha em cada tabela do domínio) sai sem
//          erro de FK e sem órfãos; idem exclusão de setor rico.
//   R-15 — exclusão de conferência arquivada não deixa conferencia_escalas órfã.
//   R-21 — mural (detalhes/ciente) e compartilhamentos de calendário do
//          objeto de OUTRO grupo → 403.
//   R-8  — arquivar/descartar é ato do gerente/encarregado de pessoal;
//          operador 403, gerente e enc 200.
//
// Padrão persona da casa: setupTestApp + loginAs + doJSONReq/doRawReqH,
// positivo E negativo. Helper de multipart só aqui (helpers_test.go intocado).

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/textproto"
	"strings"
	"testing"
)

// ---------- fixtures locais (nomes e2* — nada de helpers_test.go) ----------

func e2CriaGrupo(t *testing.T, st *Store, nome string) int64 {
	t.Helper()
	var gid int64
	if err := st.db.QueryRow(`INSERT INTO grupos (nome) VALUES (?) RETURNING id`, nome).Scan(&gid); err != nil {
		t.Fatalf("criar grupo %s: %v", nome, err)
	}
	return gid
}

func e2CriaSetor(t *testing.T, st *Store, nome string, gid int64) int64 {
	t.Helper()
	var sid int64
	if err := st.db.QueryRow(`INSERT INTO setores (nome, sigla, grupo_id) VALUES (?, 'E2', ?) RETURNING id`, nome, gid).Scan(&sid); err != nil {
		t.Fatalf("criar setor %s: %v", nome, err)
	}
	return sid
}

func e2CriaPessoa(t *testing.T, st *Store, guerra, completo string, gid, sid int64) int64 {
	t.Helper()
	var setor any
	if sid > 0 {
		setor = sid
	}
	var pid int64
	if err := st.db.QueryRow(`INSERT INTO pessoas (nome_guerra, nome_completo, grupo_id, setor_id, status)
		VALUES (?, ?, ?, ?, 'ativo') RETURNING id`, guerra, completo, gid, setor).Scan(&pid); err != nil {
		t.Fatalf("criar pessoa %s: %v", guerra, err)
	}
	return pid
}

// e2CriaConta: conta ativa do grupo COM linha em usuario_papeis (sessão nasce
// com papel/escopo do grupo; avisos exigem autor_papel_id).
func e2CriaConta(t *testing.T, st *Store, login, senha, papel string, gid int64) int64 {
	t.Helper()
	criaUsuarioTeste(t, st, login, senha, papel)
	var uid int64
	if err := st.db.QueryRow(`SELECT id FROM usuarios WHERE login = ?`, login).Scan(&uid); err != nil {
		t.Fatalf("id de %s: %v", login, err)
	}
	var grupo any
	if gid > 0 {
		grupo = gid
	}
	if _, err := st.db.Exec(`UPDATE usuarios SET grupo_id = ? WHERE id = ?`, grupo, uid); err != nil {
		t.Fatalf("vincular grupo de %s: %v", login, err)
	}
	if gid > 0 {
		if _, err := st.db.Exec(`INSERT INTO usuario_papeis (usuario_id, grupo_id, papel) VALUES (?, ?, ?)`, uid, gid, papel); err != nil {
			t.Fatalf("papel de %s: %v", login, err)
		}
	}
	return uid
}

func e2PapelID(t *testing.T, st *Store, uid int64) int64 {
	t.Helper()
	var pid int64
	if err := st.db.QueryRow(`SELECT id FROM usuario_papeis WHERE usuario_id = ? LIMIT 1`, uid).Scan(&pid); err != nil {
		t.Fatalf("papel do usuário %d: %v", uid, err)
	}
	return pid
}

func e2Contar(t *testing.T, st *Store, q string, args ...any) int {
	t.Helper()
	var n int
	if err := st.db.QueryRow(q, args...).Scan(&n); err != nil {
		t.Fatalf("contar (%s): %v", q, err)
	}
	return n
}

// e2UploadDrive: multipart com Content-Type EXPLÍCITO da parte (o fluxo real
// do drive: o MIME gravado é o que o CLIENTE manda) e nome hostil possível.
func e2UploadDrive(t *testing.T, app *App, ck *http.Cookie, nomeArquivo, mime string, conteudo []byte) int64 {
	t.Helper()
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	esc := strings.NewReplacer("\\", "\\\\", "\"", "\\\"").Replace(nomeArquivo)
	h := textproto.MIMEHeader{}
	h.Set("Content-Disposition", fmt.Sprintf(`form-data; name="arquivo"; filename="%s"`, esc))
	h.Set("Content-Type", mime)
	fw, err := mw.CreatePart(h)
	if err != nil {
		t.Fatalf("criar parte multipart: %v", err)
	}
	if _, err := fw.Write(conteudo); err != nil {
		t.Fatalf("gravar parte multipart: %v", err)
	}
	if err := mw.Close(); err != nil {
		t.Fatalf("fechar multipart: %v", err)
	}
	req := httptest.NewRequest("POST", "/api/drive/upload", &buf)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	req.Header.Set("X-SCI", "1")
	req.AddCookie(ck)
	rr := httptest.NewRecorder()
	app.mux.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("upload drive %s: %d (%s)", nomeArquivo, rr.Code, rr.Body.String())
	}
	var res map[string]any
	_ = json.Unmarshal(rr.Body.Bytes(), &res)
	id, _ := res["id"].(float64)
	if id <= 0 {
		t.Fatalf("upload sem id: %s", rr.Body.String())
	}
	return int64(id)
}

// ---------- R-5: foto de usuário com escopo ----------

func TestE2FotoUsuarioComEscopo(t *testing.T) {
	app, st, cleanup := setupTestApp(t)
	defer cleanup()

	g1 := e2CriaGrupo(t, st, "E2 Foto G1")
	g2 := e2CriaGrupo(t, st, "E2 Foto G2")
	uidGer1 := e2CriaConta(t, st, "e2f_ger1", "senha-g1", "gerente", g1)
	uidOp1 := e2CriaConta(t, st, "e2f_op1", "senha-o1", "operador", g1)
	uidGer2 := e2CriaConta(t, st, "e2f_ger2", "senha-g2", "gerente", g2)
	criaUsuarioTeste(t, st, "e2f_semgrupo", "senha-sg", "operador") // conta SEM grupo

	foto := "data:image/png;base64," + base64.StdEncoding.EncodeToString([]byte("PNG-E2-FOTO"))
	if _, err := st.db.Exec(`UPDATE usuarios SET foto_base64 = ? WHERE id IN (?, ?, ?)`, foto, uidGer1, uidOp1, uidGer2); err != nil {
		t.Fatalf("semear fotos: %v", err)
	}

	ckOp := loginAs(t, app, "e2f_op1", "senha-o1")

	// POSITIVO: conta do grupo lê foto de conta do PRÓPRIO grupo (e a própria)
	rr := doRawReqH(app, "GET", fmt.Sprintf("/api/usuarios/%d/foto", uidGer1), nil, "", ckOp)
	if rr.Code != http.StatusOK {
		t.Fatalf("foto do próprio grupo devia ser 200, veio %d (%s)", rr.Code, rr.Body.String())
	}
	if ct := rr.Header().Get("Content-Type"); ct != "image/png" {
		t.Fatalf("foto devia sair image/png, veio %q", ct)
	}
	if rr := doRawReqH(app, "GET", fmt.Sprintf("/api/usuarios/%d/foto", uidOp1), nil, "", ckOp); rr.Code != http.StatusOK {
		t.Fatalf("foto própria devia ser 200, veio %d", rr.Code)
	}

	// NEGATIVO: foto de conta de OUTRO grupo → 403 (op e gerente de grupo)
	rr = doRawReqH(app, "GET", fmt.Sprintf("/api/usuarios/%d/foto", uidGer2), nil, "", ckOp)
	if rr.Code != http.StatusForbidden {
		t.Fatalf("foto de outro grupo devia ser 403, veio %d (%s)", rr.Code, rr.Body.String())
	}
	ckGer1 := loginAs(t, app, "e2f_ger1", "senha-g1")
	if rr := doRawReqH(app, "GET", fmt.Sprintf("/api/usuarios/%d/foto", uidGer2), nil, "", ckGer1); rr.Code != http.StatusForbidden {
		t.Fatalf("gerente de grupo NÃO é global: foto de outro grupo devia ser 403, veio %d", rr.Code)
	}

	// NEGATIVO: conta sem grupo → 403 (foto ALHEIA); a PRÓPRIA foto é o avatar
	// do perfil e segue 200 (contrato legado do perfil)
	var uidSem int64
	if err := st.db.QueryRow(`SELECT id FROM usuarios WHERE login = 'e2f_semgrupo'`).Scan(&uidSem); err != nil {
		t.Fatalf("id semgrupo: %v", err)
	}
	if _, err := st.db.Exec(`UPDATE usuarios SET foto_base64 = ? WHERE id = ?`, foto, uidSem); err != nil {
		t.Fatalf("semear foto do semgrupo: %v", err)
	}
	ckSem := loginAs(t, app, "e2f_semgrupo", "senha-sg")
	if rr := doRawReqH(app, "GET", fmt.Sprintf("/api/usuarios/%d/foto", uidGer1), nil, "", ckSem); rr.Code != http.StatusForbidden {
		t.Fatalf("conta sem grupo devia levar 403 na foto alheia, veio %d", rr.Code)
	}
	if rr := doRawReqH(app, "GET", fmt.Sprintf("/api/usuarios/%d/foto", uidSem), nil, "", ckSem); rr.Code != http.StatusOK {
		t.Fatalf("foto PRÓPRIA (sem grupo) é o avatar do perfil e devia ser 200, veio %d", rr.Code)
	}

	// admin (escopo 0) continua vendo tudo
	ckAdm := loginAs(t, app, "admin", "admin123")
	for _, uid := range []int64{uidGer1, uidGer2} {
		if rr := doRawReqH(app, "GET", fmt.Sprintf("/api/usuarios/%d/foto", uid), nil, "", ckAdm); rr.Code != http.StatusOK {
			t.Fatalf("admin devia ler a foto de %d, veio %d", uid, rr.Code)
		}
	}

	// inexistente continua 404 (sem virar 500/403)
	if rr := doRawReqH(app, "GET", "/api/usuarios/999999/foto", nil, "", ckOp); rr.Code != http.StatusNotFound {
		t.Fatalf("foto inexistente devia ser 404, veio %d", rr.Code)
	}
}

// ---------- R-11: drive só inline na allowlist, filename saneado ----------

func TestE2DriveMIMEInlineSeguro(t *testing.T) {
	app, st, cleanup := setupTestApp(t)
	defer cleanup()

	g1 := e2CriaGrupo(t, st, "E2 Drive G1")
	e2CriaConta(t, st, "e2d_op", "senha-op", "operador", g1)
	ckOp := loginAs(t, app, "e2d_op", "senha-op")

	// HTML com nome hostil (aspas + ângulos) sobe com MIME do cliente
	hostil := `relatório "final" <x>.html`
	idHTML := e2UploadDrive(t, app, ckOp, hostil, "text/html; charset=utf-8", []byte("<html><script>alert(1)</script></html>"))

	// NEGATIVO: pede inline → attachment + nosniff + octet-stream
	rr := doRawReqH(app, "GET", fmt.Sprintf("/api/drive/download/%d?inline=1", idHTML), nil, "", ckOp)
	if rr.Code != http.StatusOK {
		t.Fatalf("download html: %d (%s)", rr.Code, rr.Body.String())
	}
	disp := rr.Header().Get("Content-Disposition")
	if !strings.HasPrefix(disp, "attachment;") {
		t.Fatalf("HTML NÃO pode ser inline: Content-Disposition = %q", disp)
	}
	if rr.Header().Get("X-Content-Type-Options") != "nosniff" {
		t.Fatalf("download de drive devia levar nosniff (headers=%v)", rr.Header())
	}
	if ct := rr.Header().Get("Content-Type"); ct != "application/octet-stream" {
		t.Fatalf("MIME fora da allowlist devia descer octet-stream, veio %q", ct)
	}
	// filename sem aspas internas, sem quebras/controles e sem caminho
	nome := strings.TrimPrefix(disp, "attachment; filename=")
	if len(nome) < 2 || !strings.HasPrefix(nome, `"`) || !strings.HasSuffix(nome, `"`) || strings.Count(nome, `"` ) != 2 {
		t.Fatalf("filename devia vir aspas-seguro e inteiro: %q", disp)
	}
	interior := nome[1 : len(nome)-1]
	if strings.ContainsAny(interior, "\"\r\n\x00\t") {
		t.Fatalf("filename hostil não foi sanitizado: %q", interior)
	}
	if !strings.Contains(interior, ".html") {
		t.Fatalf("filename devia preservar o nome/extension: %q", interior)
	}

	// sem ?inline segue attachment (comportamento)
	if rr := doRawReqH(app, "GET", fmt.Sprintf("/api/drive/download/%d", idHTML), nil, "", ckOp); !strings.HasPrefix(rr.Header().Get("Content-Disposition"), "attachment;") {
		t.Fatalf("sem inline devia ser attachment: %q", rr.Header().Get("Content-Disposition"))
	}

	// POSITIVO: PDF da allowlist continua inline (com nosniff)
	idPDF := e2UploadDrive(t, app, ckOp, "ordem do dia.pdf", "application/pdf", []byte("%PDF-1.4 e2"))
	rr = doRawReqH(app, "GET", fmt.Sprintf("/api/drive/download/%d?inline=1", idPDF), nil, "", ckOp)
	if rr.Code != http.StatusOK {
		t.Fatalf("download pdf: %d (%s)", rr.Code, rr.Body.String())
	}
	if disp = rr.Header().Get("Content-Disposition"); !strings.HasPrefix(disp, "inline;") {
		t.Fatalf("PDF da allowlist devia poder inline, veio %q", disp)
	}
	if rr.Header().Get("X-Content-Type-Options") != "nosniff" {
		t.Fatalf("inline também leva nosniff (headers=%v)", rr.Header())
	}
	if ct := rr.Header().Get("Content-Type"); ct != "application/pdf" {
		t.Fatalf("PDF devia preservar o MIME, veio %q", ct)
	}

	// defesa em profundidade: tipo hostil gravado direto no banco (legado) NÃO
	// vira inline nem renderiza — o gate é no serve, não no upload
	if _, err := st.db.Exec(`UPDATE drive_arquivos SET tipo = 'text/html' WHERE id = ?`, idPDF); err != nil {
		t.Fatalf("corromper tipo legado: %v", err)
	}
	rr = doRawReqH(app, "GET", fmt.Sprintf("/api/drive/download/%d?inline=1", idPDF), nil, "", ckOp)
	if rr.Header().Get("Content-Type") != "application/octet-stream" ||
		!strings.HasPrefix(rr.Header().Get("Content-Disposition"), "attachment;") {
		t.Fatalf("tipo hostil legado devia descer attachment/octet-stream (CT=%q CD=%q)",
			rr.Header().Get("Content-Type"), rr.Header().Get("Content-Disposition"))
	}
}

// ---------- R-6: NUKE de grupo rico sem erro e sem órfãos ----------

func TestE2NukeGrupoRicoSemOrfaos(t *testing.T) {
	app, st, cleanup := setupTestApp(t)
	defer cleanup()

	g := e2CriaGrupo(t, st, "E2 Rico")
	s := e2CriaSetor(t, st, "E2 Rico Setor", g)
	uidGer := e2CriaConta(t, st, "e2n_ger", "senha-g", "gerente", g)
	uidChefe := e2CriaConta(t, st, "e2n_chefe", "senha-c", "chefe_setor", g)
	uidOp := e2CriaConta(t, st, "e2n_op", "senha-o", "operador", g)
	p1 := e2CriaPessoa(t, st, "E2 RICO", "Rico Completo", g, s)

	pGer, pOp := e2PapelID(t, st, uidGer), e2PapelID(t, st, uidOp)

	// chefe_setores
	if _, err := st.db.Exec(`INSERT INTO chefe_setores (grupo_id, setor_id, usuario_id) VALUES (?,?,?)`, g, s, uidChefe); err != nil {
		t.Fatalf("chefe_setores: %v", err)
	}
	// funcao_membros (cadeira de grupo própria)
	var fGrupo int64
	if err := st.db.QueryRow(`INSERT INTO funcoes (nome, grupo_id, tipo, ativo) VALUES ('E2 Fun Grupo', ?, 'grupo', 1) RETURNING id`, g).Scan(&fGrupo); err != nil {
		t.Fatalf("função de grupo: %v", err)
	}
	if _, err := st.db.Exec(`INSERT INTO funcao_membros (funcao_id, grupo_id, usuario_id, titularidade) VALUES (?,?,?,'titular')`, fGrupo, g, uidOp); err != nil {
		t.Fatalf("funcao_membros: %v", err)
	}
	// mural: aviso + ciente + comentário
	var idAv int64
	if err := st.db.QueryRow(`INSERT INTO avisos (titulo, conteudo, autor_usuario_id, autor_papel_id, grupo_id)
		VALUES ('Aviso E2', '<p>ordem</p>', ?, ?, ?) RETURNING id`, uidGer, pGer, g).Scan(&idAv); err != nil {
		t.Fatalf("aviso: %v", err)
	}
	if _, err := st.db.Exec(`INSERT INTO aviso_cientes (aviso_id, usuario_id, papel_id) VALUES (?,?,?)`, idAv, uidOp, pOp); err != nil {
		t.Fatalf("aviso_cientes: %v", err)
	}
	if _, err := st.db.Exec(`INSERT INTO aviso_comentarios (aviso_id, usuario_id, papel_id, comentario) VALUES (?,?,?,'<p>ok</p>')`, idAv, uidOp, pOp); err != nil {
		t.Fatalf("aviso_comentarios: %v", err)
	}
	// material: item + conferência de material com item
	var idItem int64
	if err := st.db.QueryRow(`INSERT INTO material_itens (grupo_id, nome, codigo_patrimonio) VALUES (?,?,?) RETURNING id`, g, "Fuzil E2", "E2-0001").Scan(&idItem); err != nil {
		t.Fatalf("material_itens: %v", err)
	}
	var idMC int64
	if err := st.db.QueryRow(`INSERT INTO material_conferencias (grupo_id, setor_id, data, status, aberta_por) VALUES (?,?, '2026-10-10', 'aberta', ?) RETURNING id`, g, s, uidGer).Scan(&idMC); err != nil {
		t.Fatalf("material_conferencias: %v", err)
	}
	if _, err := st.db.Exec(`INSERT INTO material_conferencia_itens (conferencia_id, item_id, status) VALUES (?,?, 'presente')`, idMC, idItem); err != nil {
		t.Fatalf("material_conferencia_itens: %v", err)
	}
	// escalas: tipo + modelo + posto + apto
	var idTipo int64
	if err := st.db.QueryRow(`INSERT INTO escala_tipos (grupo_id, nome, ativo) VALUES (?,?,1) RETURNING id`, g, "E2 Tipo").Scan(&idTipo); err != nil {
		t.Fatalf("escala_tipos: %v", err)
	}
	var idMod int64
	if err := st.db.QueryRow(`INSERT INTO escala_modelos (grupo_id, nome) VALUES (?, 'E2 Modelo') RETURNING id`, g).Scan(&idMod); err != nil {
		t.Fatalf("escala_modelos: %v", err)
	}
	if _, err := st.db.Exec(`INSERT INTO escala_modelo_postos (modelo_id, tipo_id) VALUES (?,?)`, idMod, idTipo); err != nil {
		t.Fatalf("escala_modelo_postos: %v", err)
	}
	if _, err := st.db.Exec(`INSERT INTO escala_modelo_aptos (modelo_id, pessoa_id) VALUES (?,?)`, idMod, p1); err != nil {
		t.Fatalf("escala_modelo_aptos: %v", err)
	}
	// sugestões de setor e responsáveis de material
	if _, err := st.db.Exec(`INSERT INTO setor_sugestoes (grupo_id, setor_tipo, autor_id, tipo_acao, dados_json)
		VALUES (?, 'pessoal', ?, 'exemplo_e2', '{}')`, g, uidGer); err != nil {
		t.Fatalf("setor_sugestoes: %v", err)
	}
	if _, err := st.db.Exec(`INSERT INTO grupo_setor_responsaveis (grupo_id, setor_id, encarregado_id) VALUES (?,?,?)`, g, s, p1); err != nil {
		t.Fatalf("grupo_setor_responsaveis: %v", err)
	}
	// conferência do grupo com escala de guarda vinculada (R-15)
	var idCT int64
	if err := st.db.QueryRow(`INSERT INTO conferencia_tipos (nome, ativo) VALUES ('E2 Tipo Conf', 1) RETURNING id`).Scan(&idCT); err != nil {
		t.Fatalf("conferencia_tipos: %v", err)
	}
	var idConf int64
	if err := st.db.QueryRow(`INSERT INTO conferencias (data, tipo_id, grupo_id, status, criado_por) VALUES ('2026-10-09', ?, ?, 'aberta', ?) RETURNING id`, idCT, g, uidGer).Scan(&idConf); err != nil {
		t.Fatalf("conferencias: %v", err)
	}
	if _, err := st.db.Exec(`INSERT INTO conferencia_escalas (conferencia_id, usuario_id, papel_na_escala, designado_por) VALUES (?,?, 'chefe', ?)`, idConf, uidChefe, uidGer); err != nil {
		t.Fatalf("conferencia_escalas: %v", err)
	}

	// GRUPO VIZINHO: repost apontando para o aviso daqui + ciente cruzado do op
	g2 := e2CriaGrupo(t, st, "E2 Rico Vizinho")
	uidGer2 := e2CriaConta(t, st, "e2n_ger2", "senha-g2", "gerente", g2)
	pGer2 := e2PapelID(t, st, uidGer2)
	var idRep int64
	if err := st.db.QueryRow(`INSERT INTO avisos (titulo, conteudo, autor_usuario_id, autor_papel_id, grupo_id, grupo_origem_id, aviso_origem_id)
		VALUES ('Repost E2', '<p>repost</p>', ?, ?, ?, ?, ?) RETURNING id`, uidGer2, pGer2, g2, g, idAv).Scan(&idRep); err != nil {
		t.Fatalf("aviso repost: %v", err)
	}
	if _, err := st.db.Exec(`INSERT INTO aviso_cientes (aviso_id, usuario_id, papel_id) VALUES (?,?,?)`, idRep, uidOp, pOp); err != nil {
		t.Fatalf("ciente cruzado: %v", err)
	}

	// NUKE via API (admin + senha)
	ckAdm := loginAs(t, app, "admin", "admin123")
	rr, res := doJSONReq(app, "DELETE", fmt.Sprintf("/api/grupos/%d?nuke=1", g), map[string]string{"senha": "admin123"}, ckAdm)
	if rr.Code != http.StatusOK {
		t.Fatalf("NUKE do grupo rico falhou: %d (%v)", rr.Code, res)
	}

	// sem erro e sem órfãos: COUNT = 0 em TODA tabela do domínio
	for nome, q := range map[string]string{
		"grupos":                        `SELECT COUNT(*) FROM grupos WHERE id = ?`,
		"usuarios":                      `SELECT COUNT(*) FROM usuarios WHERE grupo_id = ?`,
		"pessoas":                       `SELECT COUNT(*) FROM pessoas WHERE grupo_id = ?`,
		"chefe_setores":                 `SELECT COUNT(*) FROM chefe_setores WHERE grupo_id = ?`,
		"funcao_membros":                `SELECT COUNT(*) FROM funcao_membros WHERE grupo_id = ?`,
		"avisos":                        `SELECT COUNT(*) FROM avisos WHERE grupo_id = ?`,
		"aviso_cientes (do grupo)":      `SELECT COUNT(*) FROM aviso_cientes WHERE aviso_id = ?`,
		"aviso_comentarios (do grupo)":  `SELECT COUNT(*) FROM aviso_comentarios WHERE aviso_id = ?`,
		"material_conferencias":         `SELECT COUNT(*) FROM material_conferencias WHERE grupo_id = ?`,
		"material_conferencia_itens":    `SELECT COUNT(*) FROM material_conferencia_itens WHERE conferencia_id = ?`,
		"material_itens":                `SELECT COUNT(*) FROM material_itens WHERE grupo_id = ?`,
		"escala_modelos":                `SELECT COUNT(*) FROM escala_modelos WHERE grupo_id = ?`,
		"escala_modelo_postos":          `SELECT COUNT(*) FROM escala_modelo_postos WHERE modelo_id = ?`,
		"escala_modelo_aptos":           `SELECT COUNT(*) FROM escala_modelo_aptos WHERE modelo_id = ?`,
		"escala_tipos":                  `SELECT COUNT(*) FROM escala_tipos WHERE grupo_id = ?`,
		"setor_sugestoes":               `SELECT COUNT(*) FROM setor_sugestoes WHERE grupo_id = ?`,
		"grupo_setor_responsaveis":      `SELECT COUNT(*) FROM grupo_setor_responsaveis WHERE grupo_id = ?`,
		"conferencia_escalas":           `SELECT COUNT(*) FROM conferencia_escalas WHERE conferencia_id = ?`,
		"conferencias":                  `SELECT COUNT(*) FROM conferencias WHERE grupo_id = ?`,
		"setores":                       `SELECT COUNT(*) FROM setores WHERE grupo_id = ?`,
		"funcoes":                       `SELECT COUNT(*) FROM funcoes WHERE grupo_id = ?`,
	} {
		var alvo any = g
		switch nome {
		case "aviso_cientes (do grupo)", "aviso_comentarios (do grupo)":
			alvo = idAv
		case "material_conferencia_itens":
			alvo = idMC
		case "escala_modelo_postos", "escala_modelo_aptos":
			alvo = idMod
		case "conferencia_escalas":
			alvo = idConf
		}
		if n := e2Contar(t, st, q, alvo); n != 0 {
			t.Fatalf("NUKE deixou %d linha(s) em %s", n, nome)
		}
	}

	// vizinho sobrevive, SEM órfãos nem ponteiro pendente
	if n := e2Contar(t, st, `SELECT COUNT(*) FROM avisos WHERE id = ?`, idRep); n != 1 {
		t.Fatalf("repost do grupo vizinho devia sobreviver (n=%d)", n)
	}
	var origem, grupoOrig any
	if err := st.db.QueryRow(`SELECT aviso_origem_id, grupo_origem_id FROM avisos WHERE id = ?`, idRep).Scan(&origem, &grupoOrig); err != nil {
		t.Fatalf("ler repost: %v", err)
	}
	if origem != nil || grupoOrig != nil {
		t.Fatalf("repost devia perder a origem apagada (aviso_origem=%v grupo_origem=%v)", origem, grupoOrig)
	}
	if n := e2Contar(t, st, `SELECT COUNT(*) FROM aviso_cientes WHERE aviso_id = ?`, idRep); n != 0 {
		t.Fatalf("ciente cruzado do usuário NUKEado devia ter saído (n=%d)", n)
	}
}

// ---------- R-6: exclusão de setor rico sem erro e sem órfãos ----------

func TestE2ExclusaoSetorRicoSemOrfaos(t *testing.T) {
	app, st, cleanup := setupTestApp(t)
	defer cleanup()

	g := e2CriaGrupo(t, st, "E2 Setor Rico")
	s := e2CriaSetor(t, st, "E2 SR Setor", g)
	uidGer := e2CriaConta(t, st, "e2s_ger", "senha-g", "gerente", g)
	uidChefe := e2CriaConta(t, st, "e2s_chefe", "senha-c", "chefe_setor", g)
	p1 := e2CriaPessoa(t, st, "E2 SR P1", "P1 Completo", g, s)
	p2 := e2CriaPessoa(t, st, "E2 SR P2", "P2 Completo", g, s)

	if _, err := st.db.Exec(`INSERT INTO chefe_setores (grupo_id, setor_id, usuario_id) VALUES (?,?,?)`, g, s, uidChefe); err != nil {
		t.Fatalf("chefe_setores: %v", err)
	}
	if _, err := st.db.Exec(`INSERT INTO grupo_setor_responsaveis (grupo_id, setor_id, encarregado_id) VALUES (?,?,?)`, g, s, p1); err != nil {
		t.Fatalf("grupo_setor_responsaveis: %v", err)
	}
	var idItem int64
	if err := st.db.QueryRow(`INSERT INTO material_itens (grupo_id, nome, codigo_patrimonio, setor_id) VALUES (?, 'Rádio E2', 'E2-S-001', ?) RETURNING id`, g, s).Scan(&idItem); err != nil {
		t.Fatalf("material_itens: %v", err)
	}
	if _, err := st.db.Exec(`INSERT INTO material_cautelas (item_id, pessoa_id, responsavel_entrega_id, data_saida, status, setor_id)
		VALUES (?, ?, ?, '2026-10-01', 'ativa', ?)`, idItem, p2, uidGer, s); err != nil {
		t.Fatalf("material_cautelas: %v", err)
	}
	var idMC int64
	if err := st.db.QueryRow(`INSERT INTO material_conferencias (grupo_id, setor_id, data, status, aberta_por) VALUES (?, ?, '2026-10-10', 'fechada', ?) RETURNING id`, g, s, uidGer).Scan(&idMC); err != nil {
		t.Fatalf("material_conferencias: %v", err)
	}
	if _, err := st.db.Exec(`INSERT INTO material_conferencia_itens (conferencia_id, item_id, status) VALUES (?, ?, 'presente')`, idMC, idItem); err != nil {
		t.Fatalf("material_conferencia_itens: %v", err)
	}

	ckGer := loginAs(t, app, "e2s_ger", "senha-g")

	// NEGATIVO: setor de OUTRO grupo → 403
	g2 := e2CriaGrupo(t, st, "E2 SR Outro")
	s2 := e2CriaSetor(t, st, "E2 SR Alheio", g2)
	if rr, res := doJSONReq(app, "DELETE", "/api/setores/"+i64(s2), nil, ckGer); rr.Code != http.StatusForbidden {
		t.Fatalf("setor alheio devia ser 403, veio %d (%v)", rr.Code, res)
	}

	// POSITIVO: setor rico sai sem erro
	if rr, res := doJSONReq(app, "DELETE", "/api/setores/"+i64(s), nil, ckGer); rr.Code != http.StatusOK {
		t.Fatalf("exclusão de setor rico falhou: %d (%v)", rr.Code, res)
	}

	if n := e2Contar(t, st, `SELECT COUNT(*) FROM setores WHERE id = ?`, s); n != 0 {
		t.Fatalf("setor devia ter saído (n=%d)", n)
	}
	if n := e2Contar(t, st, `SELECT COUNT(*) FROM chefe_setores WHERE setor_id = ?`, s); n != 0 {
		t.Fatalf("comando do setor devia ter saído (n=%d)", n)
	}
	if n := e2Contar(t, st, `SELECT COUNT(*) FROM usuario_papeis WHERE usuario_id = ? AND papel = 'chefe_setor'`, uidChefe); n != 0 {
		t.Fatalf("chefe do setor apagado ficou com papel chefe_setor órfão (chefe-zumbi) (n=%d)", n)
	}
	if n := e2Contar(t, st, `SELECT COUNT(*) FROM grupo_setor_responsaveis WHERE setor_id = ?`, s); n != 0 {
		t.Fatalf("responsáveis do setor deviam ter saído (n=%d)", n)
	}
	// material do grupo é REMANEJADO (sem setor), nunca órfão nem apagado
	var setorItem, setorCautela, setorMC any
	if err := st.db.QueryRow(`SELECT setor_id FROM material_itens WHERE id = ?`, idItem).Scan(&setorItem); err != nil || setorItem != nil {
		t.Fatalf("item devia sobreviver SEM setor (setor=%v err=%v)", setorItem, err)
	}
	if err := st.db.QueryRow(`SELECT setor_id FROM material_cautelas WHERE item_id = ?`, idItem).Scan(&setorCautela); err != nil || setorCautela != nil {
		t.Fatalf("cautela devia sobreviver SEM setor (setor=%v err=%v)", setorCautela, err)
	}
	if err := st.db.QueryRow(`SELECT setor_id FROM material_conferencias WHERE id = ?`, idMC).Scan(&setorMC); err != nil || setorMC != nil {
		t.Fatalf("conferência de material devia sobreviver SEM setor (setor=%v err=%v)", setorMC, err)
	}
	if n := e2Contar(t, st, `SELECT COUNT(*) FROM material_conferencia_itens WHERE conferencia_id = ?`, idMC); n != 1 {
		t.Fatalf("itens da conferência de material deviam sobreviver (n=%d)", n)
	}
	// pessoal remanejado para SEM SETOR (comportamento da ordem 06/10)
	if n := e2Contar(t, st, `SELECT COUNT(*) FROM pessoas WHERE id IN (?, ?) AND setor_id IS NULL`, p1, p2); n != 2 {
		t.Fatalf("pessoal devia ter sido remanejado para SEM SETOR (n=%d)", n)
	}

	// NEGATIVO (doutrina existente): setor com histórico de conferência → 409
	s3 := e2CriaSetor(t, st, "E2 SR Historico", g)
	var idCT int64
	if err := st.db.QueryRow(`INSERT INTO conferencia_tipos (nome, ativo) VALUES ('E2 SR Tipo', 1) RETURNING id`).Scan(&idCT); err != nil {
		t.Fatalf("tipo conf: %v", err)
	}
	var idConf int64
	if err := st.db.QueryRow(`INSERT INTO conferencias (data, tipo_id, grupo_id, status, criado_por) VALUES ('2026-10-08', ?, ?, 'aberta', ?) RETURNING id`, idCT, g, uidGer).Scan(&idConf); err != nil {
		t.Fatalf("conferência: %v", err)
	}
	if _, err := st.db.Exec(`INSERT OR IGNORE INTO conferencia_setores (conferencia_id, setor_id, status) VALUES (?, ?, 'em_andamento')`, idConf, s3); err != nil {
		t.Fatalf("conferencia_setores: %v", err)
	}
	if rr, res := doJSONReq(app, "DELETE", "/api/setores/"+i64(s3), nil, ckGer); rr.Code != http.StatusConflict {
		t.Fatalf("setor com histórico devia ser 409, veio %d (%v)", rr.Code, res)
	}
}

// ---------- R-15: excluir arquivada apaga a escala de guarda ----------

func TestE2ExcluirArquivadaApagaEscala(t *testing.T) {
	app, st, cleanup := setupTestApp(t)
	defer cleanup()

	g := e2CriaGrupo(t, st, "E2 Arq")
	uidGer := e2CriaConta(t, st, "e2a_ger", "senha-g", "gerente", g)

	var idCT int64
	if err := st.db.QueryRow(`INSERT INTO conferencia_tipos (nome, ativo) VALUES ('E2 Arq Tipo', 1) RETURNING id`).Scan(&idCT); err != nil {
		t.Fatalf("tipo: %v", err)
	}
	var idConf int64
	if err := st.db.QueryRow(`INSERT INTO conferencias (data, tipo_id, grupo_id, status, criado_por, arquivada_em)
		VALUES ('2026-10-07', ?, ?, 'fechada', ?, strftime('%Y-%m-%dT%H:%M:%fZ','now')) RETURNING id`, idCT, g, uidGer).Scan(&idConf); err != nil {
		t.Fatalf("conferência arquivada: %v", err)
	}
	if _, err := st.db.Exec(`INSERT INTO conferencia_escalas (conferencia_id, usuario_id, papel_na_escala, designado_por) VALUES (?, ?, 'operador', ?)`, idConf, uidGer, uidGer); err != nil {
		t.Fatalf("escala: %v", err)
	}

	ckAdm := loginAs(t, app, "admin", "admin123")
	if rr, res := doJSONReq(app, "DELETE", "/api/conferencia/arquivada/"+i64(idConf), nil, ckAdm); rr.Code != http.StatusOK {
		t.Fatalf("excluir arquivada com escala: %d (%v)", rr.Code, res)
	}
	if n := e2Contar(t, st, `SELECT COUNT(*) FROM conferencia_escalas WHERE conferencia_id = ?`, idConf); n != 0 {
		t.Fatalf("R-15: escala de guarda ficou ÓRFÃ (n=%d)", n)
	}
	if n := e2Contar(t, st, `SELECT COUNT(*) FROM conferencias WHERE id = ?`, idConf); n != 0 {
		t.Fatalf("conferência devia ter saído (n=%d)", n)
	}

	// NEGATIVO: não-arquivada não é excluída (doutrina)
	var idConf2 int64
	if err := st.db.QueryRow(`INSERT INTO conferencias (data, tipo_id, grupo_id, status, criado_por) VALUES ('2026-10-06', ?, ?, 'fechada', ?) RETURNING id`, idCT, g, uidGer).Scan(&idConf2); err != nil {
		t.Fatalf("conferência não arquivada: %v", err)
	}
	if rr, res := doJSONReq(app, "DELETE", "/api/conferencia/arquivada/"+i64(idConf2), nil, ckAdm); rr.Code != http.StatusConflict {
		t.Fatalf("excluir NÃO arquivada devia ser 409, veio %d (%v)", rr.Code, res)
	}
}

// ---------- R-21 (mural): detalhes/ciente só no escopo ----------

func TestE2MuralAvisoForaDoEscopo(t *testing.T) {
	app, st, cleanup := setupTestApp(t)
	defer cleanup()

	g1 := e2CriaGrupo(t, st, "E2 Mural G1")
	g2 := e2CriaGrupo(t, st, "E2 Mural G2")
	e2CriaConta(t, st, "e2m_ger1", "senha-g1", "gerente", g1)
	e2CriaConta(t, st, "e2m_op1", "senha-o1", "operador", g1)
	e2CriaConta(t, st, "e2m_ger2", "senha-g2", "gerente", g2)

	ckGer1 := loginAs(t, app, "e2m_ger1", "senha-g1")
	ckOp1 := loginAs(t, app, "e2m_op1", "senha-o1")
	ckGer2 := loginAs(t, app, "e2m_ger2", "senha-g2")

	// aviso publicado no G2
	rr, res := doJSONReq(app, "POST", "/api/avisos", map[string]any{"titulo": "Aviso G2", "conteudo": "<p>interno da outra unidade</p>"}, ckGer2)
	if rr.Code != http.StatusOK {
		t.Fatalf("publicar aviso: %d (%v)", rr.Code, res)
	}
	idAviso := int64(res["id"].(float64))

	// NEGATIVO: fora do escopo → 403 (leitura de PII e escrita de ciente)
	if rr, res := doJSONReq(app, "GET", fmt.Sprintf("/api/avisos/%d/detalhes", idAviso), nil, ckOp1); rr.Code != http.StatusForbidden {
		t.Fatalf("detalhes de aviso de outro grupo devia ser 403, veio %d (%v)", rr.Code, res)
	}
	if rr, res := doJSONReq(app, "POST", fmt.Sprintf("/api/avisos/%d/ciente", idAviso), map[string]any{}, ckOp1); rr.Code != http.StatusForbidden {
		t.Fatalf("ciente em aviso de outro grupo devia ser 403, veio %d (%v)", rr.Code, res)
	}
	if rr, _ := doJSONReq(app, "GET", fmt.Sprintf("/api/avisos/%d/detalhes", idAviso), nil, ckGer1); rr.Code != http.StatusForbidden {
		t.Fatalf("gerente de outro grupo devia levar 403 nos detalhes, veio %d", rr.Code)
	}
	if n := e2Contar(t, st, `SELECT COUNT(*) FROM aviso_cientes WHERE aviso_id = ? AND usuario_id = (SELECT id FROM usuarios WHERE login = 'e2m_op1')`, idAviso); n != 0 {
		t.Fatalf("ciente cross-group NÃO devia ter sido gravado (n=%d)", n)
	}

	// inexistente → 404 (não 500 de FK)
	if rr, _ := doJSONReq(app, "GET", "/api/avisos/999999/detalhes", nil, ckGer1); rr.Code != http.StatusNotFound {
		t.Fatalf("aviso inexistente devia ser 404, veio %d", rr.Code)
	}

	// POSITIVO: do próprio grupo continua tudo de pé
	if rr, res := doJSONReq(app, "GET", fmt.Sprintf("/api/avisos/%d/detalhes", idAviso), nil, ckGer2); rr.Code != http.StatusOK {
		t.Fatalf("detalhes do próprio grupo devia ser 200, veio %d (%v)", rr.Code, res)
	}
	if rr, res := doJSONReq(app, "POST", fmt.Sprintf("/api/avisos/%d/ciente", idAviso), map[string]any{}, ckGer2); rr.Code != http.StatusOK {
		t.Fatalf("ciente do próprio grupo devia ser 200, veio %d (%v)", rr.Code, res)
	}
	// regressão: admin é global no mural
	ckAdm := loginAs(t, app, "admin", "admin123")
	if rr, _ := doJSONReq(app, "GET", fmt.Sprintf("/api/avisos/%d/detalhes", idAviso), nil, ckAdm); rr.Code != http.StatusOK {
		t.Fatalf("admin devia continuar lendo detalhes (global), veio %d", rr.Code)
	}
}

// ---------- R-21 (calendário): compartilhamentos só para autor/gerente-dono ----------

func TestE2CalendarioCompartilhamentosEscopo(t *testing.T) {
	app, st, cleanup := setupTestApp(t)
	defer cleanup()

	g1 := e2CriaGrupo(t, st, "E2 Cal G1")
	g2 := e2CriaGrupo(t, st, "E2 Cal G2")
	uidGer1 := e2CriaConta(t, st, "e2c_ger1", "senha-g1", "gerente", g1)
	e2CriaConta(t, st, "e2c_op1", "senha-o1", "operador", g1)
	uidGer2 := e2CriaConta(t, st, "e2c_ger2", "senha-g2", "gerente", g2)

	ckGer1 := loginAs(t, app, "e2c_ger1", "senha-g1")
	ckGer2 := loginAs(t, app, "e2c_ger2", "senha-g2")

	// calendário do G1 criado pela API (autor = gerente do G1)
	rr, res := doJSONReq(app, "POST", "/api/calendarios", map[string]any{"nome": "Cal E2 G1"}, ckGer1)
	if rr.Code != http.StatusOK {
		t.Fatalf("criar calendário: %d (%v)", rr.Code, res)
	}
	idCal := int64(res["id"].(float64))

	// NEGATIVO: gerente de outro grupo → 403
	if rr, _ := doJSONReq(app, "GET", fmt.Sprintf("/api/calendarios/%d/compartilhamentos", idCal), nil, ckGer2); rr.Code != http.StatusForbidden {
		t.Fatalf("compartilhamentos de calendário alheio devia ser 403, veio %d", rr.Code)
	}
	// inexistente → 404
	if rr, _ := doJSONReq(app, "GET", "/api/calendarios/999999/compartilhamentos", nil, ckGer1); rr.Code != http.StatusNotFound {
		t.Fatalf("calendário inexistente devia ser 404, veio %d", rr.Code)
	}
	// POSITIVO: autor vê
	if rr, _ := doJSONReq(app, "GET", fmt.Sprintf("/api/calendarios/%d/compartilhamentos", idCal), nil, ckGer1); rr.Code != http.StatusOK {
		t.Fatalf("autor devia ver os compartilhamentos, veio %d", rr.Code)
	}

	// calendário de TERCEIRO do mesmo grupo: gerente do grupo-dono também vê
	// (mesma régua do compartilhar/revogar)
	var idCal2 int64
	if err := st.db.QueryRow(`INSERT INTO calendarios (nome, cor, descricao, autor_usuario_id, grupo_id)
		SELECT 'Cal E2 Terceiro', '#123456', '', (SELECT id FROM usuarios WHERE login = 'e2c_op1'), ?
		RETURNING id`, g1).Scan(&idCal2); err != nil {
		t.Fatalf("calendário de terceiro: %v", err)
	}
	if rr, _ := doJSONReq(app, "GET", fmt.Sprintf("/api/calendarios/%d/compartilhamentos", idCal2), nil, ckGer1); rr.Code != http.StatusOK {
		t.Fatalf("gerente do grupo-dono devia ver, veio %d", rr.Code)
	}
	if rr, _ := doJSONReq(app, "GET", fmt.Sprintf("/api/calendarios/%d/compartilhamentos", idCal2), nil, ckGer2); rr.Code != http.StatusForbidden {
		t.Fatalf("gerente de fora devia levar 403 (calendário de terceiro do G1), veio %d", rr.Code)
	}
	_ = uidGer1
	_ = uidGer2
}

// ---------- R-8: arquivar/descartar é do gerente/encarregado ----------

func TestE2ArquivarDescartarSomenteGerenteEnc(t *testing.T) {
	app, st, cleanup := setupTestApp(t)
	defer cleanup()

	g := e2CriaGrupo(t, st, "E2 Arq Papel")
	e2CriaConta(t, st, "e2r8_ger", "senha-g", "gerente", g)
	uidOp := e2CriaConta(t, st, "e2r8_op", "senha-o", "operador", g)
	e2CriaConta(t, st, "e2r8_enc", "senha-e", "operador", g) // ganhará a cadeira
	p1 := e2CriaPessoa(t, st, "E2 R8 P1", "P1 Completo", g, 0)

	// cadeira enc_pessoal global (v39) → o segundo operador vira encarregado
	var fEnc int64
	if err := st.db.QueryRow(`SELECT id FROM funcoes WHERE chave = 'enc_pessoal'`).Scan(&fEnc); err != nil {
		t.Fatalf("cadeira enc_pessoal ausente: %v", err)
	}
	var uidEnc int64
	if err := st.db.QueryRow(`SELECT id FROM usuarios WHERE login = 'e2r8_enc'`).Scan(&uidEnc); err != nil {
		t.Fatalf("id enc: %v", err)
	}
	if _, err := st.db.Exec(`INSERT INTO funcao_membros (funcao_id, grupo_id, usuario_id, titularidade) VALUES (?,?,?,'titular')`, fEnc, g, uidEnc); err != nil {
		t.Fatalf("designar enc: %v", err)
	}

	ckGer := loginAs(t, app, "e2r8_ger", "senha-g")
	ckOp := loginAs(t, app, "e2r8_op", "senha-o")
	ckEnc := loginAs(t, app, "e2r8_enc", "senha-e")

	// conf 1 (via API) é fechada → alvo do ARQUIVAR
	rr, res := doJSONReq(app, "POST", "/api/conferencia/iniciar", map[string]any{"nome": "Conf E2 R8"}, ckGer)
	if rr.Code != http.StatusOK {
		t.Fatalf("iniciar conf: %d (%v)", rr.Code, res)
	}
	idFechada := int64(res["id"].(float64))
	if rr, res = doJSONReq(app, "POST", "/api/conferencia/fechar", map[string]any{
		"id":          idFechada,
		"lancamentos": []map[string]any{{"pessoa_id": p1, "situacao": "presente", "verificado": true}},
	}, ckGer); rr.Code != http.StatusOK {
		t.Fatalf("fechar conf: %d (%v)", rr.Code, res)
	}

	// conf 2 (via SQL, tipo próprio por causa do UNIQUE(data,tipo_id)) fica ABERTA → alvo do DESCARTAR
	var idCT int64
	if err := st.db.QueryRow(`INSERT INTO conferencia_tipos (nome, ativo) VALUES ('E2 R8 Tipo', 1) RETURNING id`).Scan(&idCT); err != nil {
		t.Fatalf("tipo: %v", err)
	}
	var uidGerID int64
	if err := st.db.QueryRow(`SELECT id FROM usuarios WHERE login = 'e2r8_ger'`).Scan(&uidGerID); err != nil {
		t.Fatalf("id ger: %v", err)
	}
	var idAberta int64
	if err := st.db.QueryRow(`INSERT INTO conferencias (data, tipo_id, grupo_id, status, criado_por) VALUES ('2026-10-05', ?, ?, 'aberta', ?) RETURNING id`, idCT, g, uidGerID).Scan(&idAberta); err != nil {
		t.Fatalf("conf aberta: %v", err)
	}

	// NEGATIVO: operador (sem cadeira) não arquiva nem descarta
	if rr, res := doJSONReq(app, "POST", fmt.Sprintf("/api/conferencia/%d/arquivar", idFechada), nil, ckOp); rr.Code != http.StatusForbidden {
		t.Fatalf("operador NÃO devia arquivar (R-8), veio %d (%v)", rr.Code, res)
	}
	if n := e2Contar(t, st, `SELECT COUNT(*) FROM conferencias WHERE id = ? AND arquivada_em IS NOT NULL`, idFechada); n != 0 {
		t.Fatalf("operador arquivou de fato?!")
	}
	if rr, res := doJSONReq(app, "DELETE", "/api/conferencia/"+i64(idAberta), nil, ckOp); rr.Code != http.StatusForbidden {
		t.Fatalf("operador NÃO devia descartar (R-8), veio %d (%v)", rr.Code, res)
	}
	if n := e2Contar(t, st, `SELECT COUNT(*) FROM conferencias WHERE id = ?`, idAberta); n != 1 {
		t.Fatalf("operador descartou de fato?!")
	}

	// POSITIVO: encarregado de pessoal arquiva (mesma régua do fechar)
	if rr, res := doJSONReq(app, "POST", fmt.Sprintf("/api/conferencia/%d/arquivar", idFechada), nil, ckEnc); rr.Code != http.StatusOK {
		t.Fatalf("encarregado devia arquivar, veio %d (%v)", rr.Code, res)
	}

	// POSITIVO: gerente arquiva (regressão) e descarta a aberta
	var idConf3 int64
	if err := st.db.QueryRow(`INSERT INTO conferencias (data, tipo_id, grupo_id, status, criado_por) VALUES ('2026-10-04', ?, ?, 'fechada', ?) RETURNING id`, idCT, g, uidGerID).Scan(&idConf3); err != nil {
		t.Fatalf("conf 3: %v", err)
	}
	if rr, res := doJSONReq(app, "POST", fmt.Sprintf("/api/conferencia/%d/arquivar", idConf3), nil, ckGer); rr.Code != http.StatusOK {
		t.Fatalf("gerente devia continuar arquivando, veio %d (%v)", rr.Code, res)
	}
	if rr, res := doJSONReq(app, "DELETE", "/api/conferencia/"+i64(idAberta), nil, ckGer); rr.Code != http.StatusOK {
		t.Fatalf("gerente devia descartar a aberta, veio %d (%v)", rr.Code, res)
	}
	if n := e2Contar(t, st, `SELECT COUNT(*) FROM conferencias WHERE id = ?`, idAberta); n != 0 {
		t.Fatalf("descarte do gerente não gravou (n=%d)", n)
	}
	_ = uidOp
}
