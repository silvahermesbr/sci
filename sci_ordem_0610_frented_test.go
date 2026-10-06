package main

// sci_ordem_0610_frented_test.go — prova do relatório DETALHADO (ordem SCI
// 06/10, Frente D): seed próprio com 2 grupos (próprio + subordinado ativo)
// e dados; extração de texto do PDF (padrão zlib+regex do repo) provando:
//   SIMPLES   = só o resumo (sem folha de grupo, sem nomes do registro);
//   DETALHADO = resumo + 1 folha por grupo (páginas == grupos+1), quebra de
//               folha entre grupos, coluna SETOR no grupo próprio e SEM setor
//               no subordinado (cada um com seus dados).
// Guardas: 401 sem sessão, 400 modo inválido, 400 admin sem grupo, 403 rival.

import (
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"
)

// semeiaFrenteD: Cia Alfa (escopo, setor CSET) + Pel Beta (subordinada ATIVA,
// setor PBST) + Cia Gama (rival, sem vínculo). Lançamentos: Alfa (presente) e
// Bravo (falta) na conferência da Alfa; Charlie (justificada c/ destino) na
// 1ª da Beta e (atraso c/ tag) na 2ª.
func semeiaFrenteD(t *testing.T) (app *App, cleanup func(), gidAlfa, gidBeta, gidGama int64, tokGer, tokAdmin *http.Cookie) {
	t.Helper()
	app, _, cleanup = setupTestApp(t)
	st := app.st

	if res, err := st.db.Exec(`INSERT INTO grupos (nome, codigo) VALUES ('Cia Alfa', 'CA0001')`); err != nil {
		t.Fatalf("grupo alfa: %v", err)
	} else {
		gidAlfa, _ = res.LastInsertId()
	}
	if res, err := st.db.Exec(`INSERT INTO grupos (nome, codigo) VALUES ('Pel Beta', 'PB0001')`); err != nil {
		t.Fatalf("grupo beta: %v", err)
	} else {
		gidBeta, _ = res.LastInsertId()
	}
	if res, err := st.db.Exec(`INSERT INTO grupos (nome, codigo) VALUES ('Cia Gama', 'CG0001')`); err != nil {
		t.Fatalf("grupo gama: %v", err)
	} else {
		gidGama, _ = res.LastInsertId()
	}

	var s1, s2 int64
	if res, err := st.db.Exec(`INSERT INTO setores (nome, sigla) VALUES ('Comando Alfa', 'CSET')`); err != nil {
		t.Fatalf("setor alfa: %v", err)
	} else {
		s1, _ = res.LastInsertId()
	}
	if res, err := st.db.Exec(`INSERT INTO setores (nome, sigla) VALUES ('Setor Beta', 'PBST')`); err != nil {
		t.Fatalf("setor beta: %v", err)
	} else {
		s2, _ = res.LastInsertId()
	}

	var pAlfa, pBravo, pCharlie int64
	if res, err := st.db.Exec(`INSERT INTO pessoas (grupo_id, nome_guerra, nome_completo, setor_id, status)
		VALUES (?, 'Alfa', 'Alfa Maciel', ?, 'ativo')`, gidAlfa, s1); err != nil {
		t.Fatalf("pessoa alfa: %v", err)
	} else {
		pAlfa, _ = res.LastInsertId()
	}
	if res, err := st.db.Exec(`INSERT INTO pessoas (grupo_id, nome_guerra, nome_completo, setor_id, status)
		VALUES (?, 'Bravo', 'Bravo e Souza', ?, 'ativo')`, gidAlfa, s1); err != nil {
		t.Fatalf("pessoa bravo: %v", err)
	} else {
		pBravo, _ = res.LastInsertId()
	}
	if res, err := st.db.Exec(`INSERT INTO pessoas (grupo_id, nome_guerra, nome_completo, setor_id, status)
		VALUES (?, 'Charlie', 'Charlie Lopes', ?, 'ativo')`, gidBeta, s2); err != nil {
		t.Fatalf("pessoa charlie: %v", err)
	} else {
		pCharlie, _ = res.LastInsertId()
	}

	criaUsuarioTeste(t, st, "ger_det", "senha-gerente", "gerente")
	if _, err := st.db.Exec(`UPDATE usuarios SET grupo_id = ? WHERE login = 'ger_det'`, gidAlfa); err != nil {
		t.Fatalf("grupo do gerente: %v", err)
	}
	if _, err := st.db.Exec(`INSERT OR IGNORE INTO usuario_papeis (usuario_id, grupo_id, papel)
		SELECT id, grupo_id, papel FROM usuarios WHERE login = 'ger_det'`); err != nil {
		t.Fatalf("papel do gerente: %v", err)
	}
	tokGer = loginAs(t, app, "ger_det", "senha-gerente")
	tokAdmin = loginAs(t, app, "admin", "admin123")

	// vínculo bilateral ATIVO: Alfa superiora Beta (gruposSubordinadosAtivos)
	if _, err := st.db.Exec(`INSERT INTO grupo_vinculos (superior_id, subordinado_id, criado_por_superior, criado_por_subordinado)
		VALUES (?, ?, 1, 1)`, gidAlfa, gidBeta); err != nil {
		t.Fatalf("vinculo alfa>beta: %v", err)
	}

	var destID, tagID int64
	if res, err := st.db.Exec(`INSERT INTO destinos (nome) VALUES ('Servico externo')`); err != nil {
		t.Fatalf("destino: %v", err)
	} else {
		destID, _ = res.LastInsertId()
	}
	if res, err := st.db.Exec(`INSERT INTO tags (nome, cor, ativo) VALUES ('COMBATE', '#333333', 1)`); err != nil {
		t.Fatalf("tag: %v", err)
	} else {
		tagID, _ = res.LastInsertId()
	}

	tipoID := int64(0)
	if err := st.db.QueryRow(`SELECT id FROM conferencia_tipos ORDER BY id LIMIT 1`).Scan(&tipoID); err != nil {
		t.Fatalf("tipo de conferência no seed: %v", err)
	}

	var uidGer int64
	_ = st.db.QueryRow(`SELECT id FROM usuarios WHERE login='ger_det'`).Scan(&uidGer)
	agora := time.Now().UTC().Format("2006-01-02T15:04:05.000Z")
	hoje := time.Now().In(app.horaLocal)
	d3 := hoje.AddDate(0, 0, -3).Format("2006-01-02")
	d2 := hoje.AddDate(0, 0, -2).Format("2006-01-02")
	d1 := hoje.AddDate(0, 0, -1).Format("2006-01-02")

	insConf := func(data, hora string, gid int64) int64 {
		t.Helper()
		res, err := st.db.Exec(`INSERT INTO conferencias (data, hora, tipo_id, local, grupo_id, status, criado_por, criado_em, fechada_em)
			VALUES (?, ?, ?, 'Pátio', ?, 'fechada', ?, ?, ?)`, data, hora, tipoID, gid, uidGer, agora, agora)
		if err != nil {
			t.Fatalf("conferência %s: %v", data, err)
		}
		id, _ := res.LastInsertId()
		return id
	}
	insPres := func(confID, pID int64, sit string, dest, tag int64, obs string) {
		t.Helper()
		var d, tg any // destino/tag 0 → NULL (FK: REFERENCES destinos/tags)
		if dest > 0 {
			d = dest
		}
		if tag > 0 {
			tg = tag
		}
		if _, err := st.db.Exec(`INSERT OR IGNORE INTO presencas (conferencia_id, pessoa_id, situacao, destino_id, tag_id, observacao, marcado_por, marcado_em)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?)`, confID, pID, sit, d, tg, obs, uidGer, agora); err != nil {
			t.Fatalf("presença %d: %v", pID, err)
		}
	}

	confA := insConf(d3, "08:00:00", gidAlfa)
	insPres(confA, pAlfa, "presente", 0, 0, "Sem observacao")
	insPres(confA, pBravo, "falta", 0, 0, "")
	confB := insConf(d2, "09:30:00", gidBeta)
	insPres(confB, pCharlie, "justificada", destID, 0, "Consulta medica")
	confC := insConf(d1, "07:15:00", gidBeta)
	insPres(confC, pCharlie, "atraso", 0, tagID, "")

	return app, cleanup, gidAlfa, gidBeta, gidGama, tokGer, tokAdmin
}

// contaPaginasPDF: nº de objetos página no PDF bruto (dicionários não comprimidos).
func contaPaginasPDF(t *testing.T, pdf []byte) int {
	t.Helper()
	re := regexp.MustCompile(`/Type\s*/Page[^s]`)
	return len(re.FindAllSubmatchIndex(pdf, -1))
}

// normalizaTextoPDF: remove marcas voláteis do rodapé/cabeçalho (hash de
// autenticidade por nanos e horário de emissão) p/ comparar 2 PDFs no texto.
func normalizaTextoPDF(txt string) string {
	re := regexp.MustCompile(`Autenticidade: #[0-9A-F]{8}`)
	txt = re.ReplaceAllString(txt, "")
	re = regexp.MustCompile(`Emitido em: \d{2}/\d{2}/\d{4} \d{2}:\d{2}`)
	txt = re.ReplaceAllString(txt, "")
	return txt
}

// TestFrenteDSimples: modo=simples é EXATAMENTE o legado /api/relatorio.pdf —
// mesmo nº de páginas e mesmo texto (a página 1 do detalhado herda isso).
func TestFrenteDSimples(t *testing.T) {
	app, cleanup, _, _, _, tokGer, _ := semeiaFrenteD(t)
	defer cleanup()

	de := time.Now().In(app.horaLocal).AddDate(0, 0, -3).Format("2006-01-02")
	ate := time.Now().In(app.horaLocal).Format("2006-01-02")
	rr := doRawReqH(app, "GET", "/api/relatorio/detalhado.pdf?de="+de+"&ate="+ate+"&modo=simples", nil, "", tokGer)
	if rr.Code != http.StatusOK {
		t.Fatalf("modo=simples: status %d (%s)", rr.Code, rr.Body.String())
	}
	pdf := rr.Body.Bytes()
	if string(pdf[:4]) != "%PDF" {
		t.Fatalf("resposta não é PDF: %.20q", pdf)
	}
	// mesmo conteúdo do legado: nº de páginas e texto (menos voláteis)
	rrLeg := doRawReqH(app, "GET", "/api/relatorio.pdf?de="+de+"&ate="+ate, nil, "", tokGer)
	if rrLeg.Code != http.StatusOK || string(rrLeg.Body.Bytes()[:4]) != "%PDF" {
		t.Fatalf("legado /api/relatorio.pdf: status %d", rrLeg.Code)
	}
	if nS, nL := contaPaginasPDF(t, pdf), contaPaginasPDF(t, rrLeg.Body.Bytes()); nS != nL {
		t.Errorf("modo=simples: %d páginas; legado tem %d", nS, nL)
	}
	txtS := normalizaTextoPDF(extrairTextoPDF(t, pdf))
	txtL := normalizaTextoPDF(extrairTextoPDF(t, rrLeg.Body.Bytes()))
	if txtS != txtL {
		t.Errorf("modo=simples difere do legado no texto (simples=%d chars, legado=%d chars)", len(txtS), len(txtL))
	}
	// resumo presente; folha de grupo ausente
	for _, esperado := range []string{"RESUMO GERAL DO PERIODO", "PROPORCAO DE SITUACOES"} {
		if !strings.Contains(txtS, esperado) {
			t.Errorf("simples sem o bloco do resumo %q", esperado)
		}
	}
	for _, proibido := range []string{"GRUPO:", "Charlie Lopes", "Alfa Maciel"} {
		if strings.Contains(txtS, proibido) {
			t.Errorf("simples não deveria conter registro detalhado %q", proibido)
		}
	}
}

// TestFrenteDDetalhado: resumo + UMA FOLHA POR GRUPO (páginas == grupos+1 ==
// 3), quebra de folha entre os grupos (12+ linhas do 1º não o empurram para a
// folha do 2º — sem AddPage por grupo caberiam na mesma folha), setor no
// próprio grupo e SEM coluna de setor no subordinado.
func TestFrenteDDetalhado(t *testing.T) {
	app, cleanup, gidAlfa, gidBeta, _, tokGer, _ := semeiaFrenteD(t)
	defer cleanup()

	// reforço de continuação: +10 presenças do grupo próprio (linhas extras)
	// para que a folha do 1º grupo tenha volume real — a quebra p/ o 2º grupo
	// é por ORDEM (AddPage antes do cabeçalho), não por estouro.
	var uidGer int64
	_ = app.st.db.QueryRow(`SELECT id FROM usuarios WHERE login='ger_det'`).Scan(&uidGer)
	var pAlfa int64
	_ = app.st.db.QueryRow(`SELECT id FROM pessoas WHERE nome_guerra='Alfa'`).Scan(&pAlfa)
	agora := time.Now().UTC().Format("2006-01-02T15:04:05.000Z")
	tipoID := int64(0)
	_ = app.st.db.QueryRow(`SELECT id FROM conferencia_tipos ORDER BY id LIMIT 1`).Scan(&tipoID)
	hoje := time.Now().In(app.horaLocal)
	de := hoje.AddDate(0, 0, -19).Format("2006-01-02")
	ate := hoje.Format("2006-01-02")
	for i := 10; i >= 1; i-- {
		dia := hoje.AddDate(0, 0, -10-i).Format("2006-01-02")
		var confID int64
		res, err := app.st.db.Exec(`INSERT INTO conferencias (data, hora, tipo_id, local, grupo_id, status, criado_por, criado_em, fechada_em)
			VALUES (?, '07:00:00', ?, 'Pátio', ?, 'fechada', ?, ?, ?)`, dia, tipoID, gidAlfa, uidGer, agora, agora)
		if err != nil {
			t.Fatalf("conferência extra %d: %v", i, err)
		}
		confID, _ = res.LastInsertId()
		if _, err := app.st.db.Exec(`INSERT OR IGNORE INTO presencas (conferencia_id, pessoa_id, situacao, marcado_por, marcado_em)
			VALUES (?, ?, 'presente', ?, ?)`, confID, pAlfa, uidGer, agora); err != nil {
			t.Fatalf("presença extra %d: %v", i, err)
		}
	}
	_ = gidBeta

	rr := doRawReqH(app, "GET", "/api/relatorio/detalhado.pdf?de="+de+"&ate="+ate+"&modo=detalhado&grupo="+strconv.FormatInt(gidAlfa, 10), nil, "", tokGer)
	if rr.Code != http.StatusOK {
		t.Fatalf("modo=detalhado: status %d (%s)", rr.Code, rr.Body.String())
	}
	pdf := rr.Body.Bytes()
	// prova relacional: folhas = folhas do resumo (idêntico ao simples) + UMA
	// por grupo (2). Cada grupo começa em folha PRÓPRIA (AddPage antes do
	// cabeçalho) — nunca dividindo folha com o resumo ou com o outro grupo.
	rrSimples := doRawReqH(app, "GET", "/api/relatorio/detalhado.pdf?de="+de+"&ate="+ate+"&modo=simples&grupo="+strconv.FormatInt(gidAlfa, 10), nil, "", tokGer)
	if rrSimples.Code != http.StatusOK {
		t.Fatalf("modo=simples (base de comparação): status %d", rrSimples.Code)
	}
	nResumo := contaPaginasPDF(t, rrSimples.Body.Bytes())
	if n := contaPaginasPDF(t, pdf); n != nResumo+2 {
		t.Errorf("detalhado: esperado %d páginas (resumo %d + 1 folha por grupo), obtido %d", nResumo+2, nResumo, n)
	}
	txt := extrairTextoPDF(t, pdf)

	// página 1 = resumo idêntico ao simples
	if !strings.Contains(txt, "RESUMO GERAL DO PERIODO") {
		t.Errorf("detalhado sem o resumo na página 1")
	}
	// cabeçalho de folha com nome do grupo + período
	if !strings.Contains(txt, "GRUPO: Cia Alfa") || !strings.Contains(txt, "(GRUPO DO ESCOPO)") {
		t.Errorf("detalhado sem folha do grupo próprio: %s", trecho(txt, "GRUPO:"))
	}
	if !strings.Contains(txt, "GRUPO: Pel Beta") {
		t.Errorf("detalhado sem folha do grupo subordinado")
	}
	diaA := hoje.AddDate(0, 0, -19).Format("02/01/2006")
	diaB := hoje.Format("02/01/2006")
	if !strings.Contains(txt, "PERIODO: "+diaA+" A "+diaB) {
		t.Errorf("cabeçalho de folha sem período %s a %s", diaA, diaB)
	}
	// registro linha a linha — próprio grupo
	if !strings.Contains(txt, "Alfa Maciel") || !strings.Contains(txt, "Bravo e Souza") {
		t.Errorf("folha do próprio grupo sem os militares (nome completo conforme cadastro)")
	}
	// registro linha a linha — subordinado
	if !strings.Contains(txt, "Charlie Lopes") {
		t.Errorf("folha do subordinado sem o militar")
	}
	// coluna SETOR (seção) para o pessoal do PRÓPRIO grupo: sigla presente
	if !strings.Contains(txt, "CSET") {
		t.Errorf("grupo próprio sem a coluna setor (CSET)")
	}
	// subordinado: folha PRÓPRIA e SEM coluna de setor — a partir do cabeçalho
	// do subordinado não pode aparecer a sigla do setor DELE (PBST)
	idxBeta := strings.Index(txt, "GRUPO: Pel Beta")
	if idxBeta < 0 {
		t.Fatalf("cabeçalho do subordinado ausente")
	}
	cauda := txt[idxBeta:]
	if !strings.Contains(cauda, "Charlie Lopes") {
		t.Errorf("folha do subordinado não contém os registros dele")
	}
	if strings.Contains(cauda, "PBST") {
		t.Errorf("subordinado exibiu coluna setor — só o grupo próprio tem SETOR")
	}
	// ordem das folhas: próprio primeiro, subordinado depois (folhas distintas)
	idxAlfa := strings.Index(txt, "GRUPO: Cia Alfa")
	if idxAlfa < 0 || idxAlfa > idxBeta {
		t.Errorf("folhas fora de ordem (próprio deve preceder subordinado)")
	}
	// dados específicos: destino da justificada, tag do atraso, situações
	for _, esperado := range []string{"Servico externo", "COMBATE", "Consulta medica", "Justificada", "Atraso"} {
		if !strings.Contains(txt, esperado) {
			t.Errorf("detalhado sem %q", esperado)
		}
	}
	// horas exibidas (com .In(loc), nunca cru): 08:00 e 09:30 presentes
	if !strings.Contains(txt, "08:00") || !strings.Contains(txt, "09:30") {
		t.Errorf("detalhado sem as horas das conferências")
	}
	// quebra de folha: 13 linhas do próprio + cabeçalhos ocupam a folha inteira;
	// se NÃO houvesse AddPage por grupo, as 2 folhas colapsariam (2 páginas).
	if n := contaPaginasPDF(t, pdf); n < 3 {
		t.Errorf("quebra de folha entre grupos não ocorreu (%d páginas)", n)
	}
}

// trecho: fatia de debug em volta da 1ª ocorrência (mensagens de erro curtas).
func trecho(txt, marcador string) string {
	i := strings.Index(txt, marcador)
	if i < 0 {
		return "(marcador ausente)"
	}
	fim := i + 120
	if fim > len(txt) {
		fim = len(txt)
	}
	return txt[i:fim]
}

// TestFrenteDGuardas: parâmetros e escopo da rota nova — legado intocado.
func TestFrenteDGuardas(t *testing.T) {
	app, cleanup, gidAlfa, _, gidGama, tokGer, tokAdmin := semeiaFrenteD(t)
	defer cleanup()

	// 401 sem sessão
	rr := doRawReqH(app, "GET", "/api/relatorio/detalhado.pdf?modo=simples", nil, "", nil)
	if rr.Code != http.StatusUnauthorized {
		t.Errorf("sem sessão: esperado 401, obtido %d", rr.Code)
	}
	// 400 modo inválido
	rr = doRawReqH(app, "GET", "/api/relatorio/detalhado.pdf?modo=gigante", nil, "", tokGer)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("modo inválido: esperado 400, obtido %d", rr.Code)
	}
	// 403 gerente pede grupo rival (fora da árvore)
	rr = doRawReqH(app, "GET", "/api/relatorio/detalhado.pdf?modo=simples&grupo="+strconv.FormatInt(gidGama, 10), nil, "", tokGer)
	if rr.Code != http.StatusForbidden {
		t.Errorf("grupo rival: esperado 403, obtido %d", rr.Code)
	}
	// 400 admin sem grupo (escopo global não gera folha por grupo)
	rr = doRawReqH(app, "GET", "/api/relatorio/detalhado.pdf?modo=simples", nil, "", tokAdmin)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("admin sem grupo: esperado 400, obtido %d", rr.Code)
	}
	// grupo do próprio escopo segue OK
	rr = doRawReqH(app, "GET", "/api/relatorio/detalhado.pdf?modo=simples&grupo="+strconv.FormatInt(gidAlfa, 10), nil, "", tokGer)
	if rr.Code != http.StatusOK {
		t.Errorf("grupo do escopo: esperado 200, obtido %d", rr.Code)
	}
	// legado intacto: /api/relatorio.pdf continua de pé
	rr = doRawReqH(app, "GET", "/api/relatorio.pdf", nil, "", tokGer)
	if rr.Code != http.StatusOK || string(rr.Body.Bytes()[:4]) != "%PDF" {
		t.Errorf("legado /api/relatorio.pdf quebrado: %d", rr.Code)
	}
}
