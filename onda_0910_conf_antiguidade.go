package main

// Onda 09/10 — M1: conferência POR ANTIGUIDADE.
// Filtro de funções (postos/graduações) na conferência + endpoint pre_fechamento.
// Tabela conferencia_funcoes + migração v40 com seed das funções canônicas.

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"
)

func (s *Store) migrarV40() error {
	var v int
	_ = s.db.QueryRow(`SELECT versao FROM schema_migrations WHERE versao = 40`).Scan(&v)
	if v == 40 {
		// Defesa (revisão 09/10): marca 40 pode ter sido gravada em banco de dev
		// por binário que usava v40 para o índice de material — só pular se a
		// tabela desta migração realmente existir.
		var n int
		_ = s.db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='conferencia_funcoes'`).Scan(&n)
		if n > 0 {
			return nil
		}
	}

	if _, err := s.db.Exec(`CREATE TABLE IF NOT EXISTS conferencia_funcoes (
		conferencia_id INTEGER NOT NULL REFERENCES conferencias(id) ON DELETE CASCADE,
		funcao_id INTEGER NOT NULL REFERENCES funcoes(id),
		PRIMARY KEY (conferencia_id, funcao_id)
	)`); err != nil {
		return fmt.Errorf("migracao v40 create conferencia_funcoes: %w", err)
	}

	// Seed idempotente POR NOME: oficiais + Praças (pai=NULL)
	oficiais := []struct {
		nome        string
		antiguidade int
	}{
		{"Coronel", 1},
		{"Tenente-Coronel", 2},
		{"Major", 3},
		{"Capitão", 4},
		{"1º Tenente", 5},
		{"2º Tenente", 6},
		{"Aspirante-a-Oficial", 7},
	}

	for _, f := range oficiais {
		var id int64
		if err := s.db.QueryRow(`SELECT id FROM funcoes WHERE nome = ? AND tipo = 'antiguidade'`, f.nome).Scan(&id); err != nil {
			if _, e := s.db.Exec(`INSERT INTO funcoes (nome, grupo_id, tipo, antiguidade, pai_id) VALUES (?, NULL, 'antiguidade', ?, NULL)`, f.nome, f.antiguidade); e != nil {
				return fmt.Errorf("migracao v40 seed %s: %w", f.nome, e)
			}
		}
	}

	// Nó Praças
	var pID int64
	if err := s.db.QueryRow(`SELECT id FROM funcoes WHERE nome = 'Praças' AND tipo = 'antiguidade'`).Scan(&pID); err != nil {
		res, e := s.db.Exec(`INSERT INTO funcoes (nome, grupo_id, tipo, antiguidade, pai_id) VALUES ('Praças', NULL, 'antiguidade', 8, NULL)`)
		if e != nil {
			return fmt.Errorf("migracao v40 seed Pracas: %w", e)
		}
		if lastID, _ := res.LastInsertId(); lastID > 0 {
			pID = lastID
		}
	}

	if pID <= 0 {
		_ = s.db.QueryRow(`SELECT id FROM funcoes WHERE nome = 'Praças' AND tipo = 'antiguidade'`).Scan(&pID)
	}

	// Praças children com pai = Praças.id
	pracasSub := []struct {
		nome        string
		antiguidade int
	}{
		{"1º Sargento", 9},
		{"2º Sargento", 10},
		{"3º Sargento", 11},
		{"Cabo", 12},
		{"Soldado EV", 13},
		{"Soldado 2ª Classe", 14},
	}

	for _, f := range pracasSub {
		var id int64
		if err := s.db.QueryRow(`SELECT id FROM funcoes WHERE nome = ? AND tipo = 'antiguidade'`, f.nome).Scan(&id); err != nil {
			if _, e := s.db.Exec(`INSERT INTO funcoes (nome, grupo_id, tipo, antiguidade, pai_id) VALUES (?, NULL, 'antiguidade', ?, ?)`, f.nome, f.antiguidade, pID); e != nil {
				return fmt.Errorf("migracao v40 seed %s: %w", f.nome, e)
			}
		}
	}

	return s.marcarVersao(40)
}

func (a *App) funcoesFiltroDaConferencia(cid int64) []int64 {
	out := []int64{}
	rows, err := a.st.db.Query(`SELECT funcao_id FROM conferencia_funcoes WHERE conferencia_id = ? ORDER BY funcao_id`, cid)
	if err != nil {
		return out
	}
	defer rows.Close()
	for rows.Next() {
		var fid int64
		if rows.Scan(&fid) == nil {
			out = append(out, fid)
		}
	}
	return out
}

// ---------- v1.5.4-D3 (R-7): fonte ÚNICA das expressões SQL de antiguidade ----------
// O filtro da conferência por antiguidade casa o militar em 3 fontes (tag na
// PESSOA, na CONTA ou num PAPEL da conta). Filtro, ORDENAÇÃO e aviso de
// excluídos têm que sair da mesma doutrina — por isso o texto SQL vive AQUI e
// os pontos de consulta (hoje/pessoasAtivasOpt, pré-fechamento, relatório em
// tela e PDF) só o referenciam, sem replicar SQL.

// filtroAntiguidadeTresFontes: corpo da condição (sem o "AND " inicial) que
// casa o militar com as tags da conferência (conferencia_funcoes) nas 3
// fontes. Requer o alias p (pessoas). Placeholders: 3× conferencia_id, nesta
// ordem.
const filtroAntiguidadeTresFontes = `(p.funcao_id IN (SELECT funcao_id FROM conferencia_funcoes WHERE conferencia_id = ?)
OR EXISTS (SELECT 1 FROM usuarios u2 WHERE u2.pessoa_id = p.id AND u2.funcao_id IN (SELECT funcao_id FROM conferencia_funcoes WHERE conferencia_id = ?))
OR EXISTS (SELECT 1 FROM usuario_papeis up2 JOIN usuarios u3 ON u3.id = up2.usuario_id WHERE u3.pessoa_id = p.id AND (up2.grupo_id = p.grupo_id OR up2.grupo_id IS NULL) AND up2.funcao_id IN (SELECT funcao_id FROM conferencia_funcoes WHERE conferencia_id = ?)))`

// exprAntiguidadeTresFontes: antiguidade UNIFICADA do militar — a tag vale na
// primeira fonte que a tiver, na mesma precedência do filtro e do nome de
// função exibido: PESSOA (fu) → CONTA (fu_u) → PAPEL (fu_up). Sem tag em
// nenhuma fonte → 999 (por último). Requer os aliases fu/fu_u/fu_up (os mesmos
// JOINs que as consultas do modo antiguidade já fazem).
const exprAntiguidadeTresFontes = `COALESCE(fu.antiguidade, fu_u.antiguidade, fu_up.antiguidade, 999)`

// ordemAntiguidadeTresFontes: sufixo ORDER BY do modo antiguidade — antiguidade
// unificada, depois nome, depois id (estável e determinística mesmo com nomes
// de guerra repetidos). colNome é a coluna de nome da consulta.
func ordemAntiguidadeTresFontes(colNome string) string {
	return exprAntiguidadeTresFontes + ", " + colNome + ", p.id"
}

// conferenciaEmModoAntiguidade: true se a conferência tem filtro de tags
// (linhas em conferencia_funcoes) — o modo "por antiguidade".
func (a *App) conferenciaEmModoAntiguidade(cid int64) bool {
	var n int
	_ = a.st.db.QueryRow(`SELECT COUNT(*) FROM conferencia_funcoes WHERE conferencia_id = ?`, cid).Scan(&n)
	return n > 0
}

// militaresSemTagAntiguidade (R-7): nomes (em ordem alfabética) dos militares
// ATIVOS do universo da conferência — grupo da conferência, recortado aos
// setores despachados quando houver — que ficaram FORA do filtro por não
// terem a tag em NENHUMA das 3 fontes. Alimenta `sem_tag` na resposta do
// iniciar/despachar para o front avisar, em vez de o militar sumir em silêncio.
// Chamar APÓS criar a conferência (lê as tags já semeadas em CF).
func (a *App) militaresSemTagAntiguidade(grupoID, cid int64, setores []int64) []string {
	out := []string{}
	q := `SELECT p.nome_guerra FROM pessoas p WHERE p.status = 'ativo' AND p.grupo_id = ?`
	args := []any{grupoID}
	if len(setores) > 0 {
		ph := strings.TrimSuffix(strings.Repeat("?,", len(setores)), ",")
		q += ` AND p.setor_id IN (` + ph + `)`
		for _, sid := range setores {
			args = append(args, sid)
		}
	}
	// COALESCE(...,0): lógica tri-estados — pessoa com funcao_id NULL (nem
	// conta/papel) deixa o predicado em NULL, e NOT NULL não é TRUE; sem o
	// COALESCE o militar sem tag nenhuma é justamente quem some do aviso.
	q += ` AND NOT COALESCE(` + filtroAntiguidadeTresFontes + `, 0) ORDER BY p.nome_guerra`
	args = append(args, cid, cid, cid)
	rows, err := a.st.db.Query(q, args...)
	if err != nil {
		return out
	}
	defer rows.Close()
	for rows.Next() {
		var ng string
		if rows.Scan(&ng) == nil {
			out = append(out, ng)
		}
	}
	return out
}

// predicadoAntiguidade devolve clausula SQL + args para filtrar pessoas que
// tenham funcao na conferencia_funcoes (p.funcao_id, u2.funcao_id ou up2.funcao_id).
// Se nao ha filtro, devolve vazio.
func (a *App) predicadoAntiguidade(cid int64) (string, []any) {
	var n int
	if err := a.st.db.QueryRow(`SELECT COUNT(*) FROM conferencia_funcoes WHERE conferencia_id = ?`, cid).Scan(&n); err != nil || n == 0 {
		return "", nil
	}
	return " AND " + filtroAntiguidadeTresFontes, []any{cid, cid, cid}
}

func (a *App) hSetorPreFechamento(w http.ResponseWriter, r *http.Request) {
	u := usuarioDoCtx(r)
	cid, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || cid <= 0 {
		jsonErro(w, http.StatusBadRequest, "id de conferencia invalido")
		return
	}
	sid, err := strconv.ParseInt(r.PathValue("setor_id"), 10, 64)
	if err != nil || sid <= 0 {
		jsonErro(w, http.StatusBadRequest, "setor_id invalido")
		return
	}

	var confStatus string
	var confGrupoID int64
	if err := a.st.db.QueryRow(`SELECT status, COALESCE(grupo_id, 0) FROM conferencias WHERE id = ?`, cid).Scan(&confStatus, &confGrupoID); err != nil {
		jsonErro(w, http.StatusNotFound, "conferencia nao encontrada")
		return
	}
	if confStatus != "aberta" {
		jsonErro(w, http.StatusBadRequest, "conferencia ja esta fechada")
		return
	}
	esc, err := a.exigeEscopo(u)
	if err != nil {
		jsonErro(w, http.StatusForbidden, "conta sem grupo definido")
		return
	}
	if esc > 0 && confGrupoID != esc {
		jsonErro(w, http.StatusForbidden, "conferencia fora do seu escopo")
		return
	}

	if u.Papel == "chefe_setor" {
		sAtivo := setorDoUsuario(a, u)
		if sAtivo == nil || *sAtivo != sid {
			jsonErro(w, http.StatusForbidden, "setor ativo no seu contexto e outro -- troque a funcao no menu de contexto antes de acessar este setor")
			return
		}
	}

	// Nome do setor
	var setorNome string
	if err := a.st.db.QueryRow(`SELECT nome FROM setores WHERE id = ?`, sid).Scan(&setorNome); err != nil {
		jsonErro(w, http.StatusNotFound, "setor nao encontrado")
		return
	}

	pred, argsPred := a.predicadoAntiguidade(cid)

	q := `SELECT p.id, p.nome_guerra,
	             COALESCE(NULLIF(fu.nome,''), COALESCE(NULLIF(fu_u.nome,''), COALESCE(NULLIF(fu_up.nome,''), ''))) AS funcao_nome,
	             COALESCE(pr.situacao, '') AS situacao, pr.verificado, COALESCE(pr.observacao, '')
	      FROM pessoas p
	      LEFT JOIN funcoes fu ON fu.id = p.funcao_id
	      LEFT JOIN usuarios u2 ON u2.pessoa_id = p.id
	      LEFT JOIN funcoes fu_u ON fu_u.id = u2.funcao_id
	      LEFT JOIN usuario_papeis up2 ON up2.usuario_id = u2.id AND (up2.grupo_id = p.grupo_id OR up2.grupo_id IS NULL)
	      LEFT JOIN funcoes fu_up ON fu_up.id = up2.funcao_id
	      LEFT JOIN presencas pr ON pr.conferencia_id = ? AND pr.pessoa_id = p.id
	      WHERE p.setor_id = ? AND p.status = 'ativo'`
	queryArgs := []any{cid, sid}
	if esc > 0 {
		q += ` AND p.grupo_id = ?`
		queryArgs = append(queryArgs, esc)
	}
	q += pred
	queryArgs = append(queryArgs, argsPred...)
	// v1.5.4-D3 (R-7): ordem pela antiguidade UNIFICADA das 3 fontes da tag —
	// antes usava só p.funcao_id e quem só tem tag na conta/papel caía pro fim.
	q += ` ORDER BY ` + ordemAntiguidadeTresFontes("p.nome_guerra")

	rows, errQ := a.st.db.Query(q, queryArgs...)
	if errQ != nil {
		jsonErro(w, http.StatusInternalServerError, errQ.Error())
		return
	}
	defer rows.Close()

	total := 0
	verificados := 0
	itens := []map[string]any{}
	for rows.Next() {
		var pid int64
		var ng, fnome, situacao, obs string
		var verificado *int
		if err := rows.Scan(&pid, &ng, &fnome, &situacao, &verificado, &obs); err != nil {
			continue
		}
		isVerificado := verificado != nil && *verificado == 1
		if isVerificado {
			verificados++
		}
		sit := situacao
		if sit == "" {
			sit = "nao_verificado"
		}
		itens = append(itens, map[string]any{
			"pessoa_id":   pid,
			"nome_guerra": ng,
			"funcao":      fnome,
			"situacao":    sit,
			"verificado":  isVerificado,
			"observacao":  obs,
		})
		total++
	}

	jsonOK(w, map[string]any{
		"setor_id":    sid,
		"setor":       setorNome,
		"total":       total,
		"verificados": verificados,
		"itens":       itens,
	})
}

// ---------- CORREÇÃO 09/10 (ordem do dono): antiguidade é a DO GRUPO --------
// A conferência por antiguidade NÃO inventa postos/graduações: usa as tags
// mantidas pelo gerente no catálogo do grupo (funcoes.grupo_id = grupo). A
// seed global da v40 (grupo_id NULL) sai do cardápio: o picker tem endpoint
// próprio escopado, a validação de criação exige grupo, e a v42 apaga a seed
// quando nada a referencia (linhas referenciadas ficam inertes — compat R4).

// avisoSemTagsAntiguidade: mensagem única front/back (R3).
const avisoSemTagsAntiguidade = "Grupo sem tags de antiguidade — cadastre no catálogo do grupo (módulo Pessoal)"

// FuncaoAntig: item da escada de antiguidade de um grupo (tag do gerente).
type FuncaoAntig struct {
	ID          int64
	Nome        string
	PaiID       *int64
	Antiguidade int
}

// funcoesAntiguidadeDoGrupo devolve as tags de antiguidade DO GRUPO
// (funcoes.grupo_id = grupo), ordenadas pelo campo antiguidade. soAtivas=true
// filtra ativo=1 (criação de conferência). Sem herança de grupo superior:
// cada grupo confere pela escada que o próprio gerente cadastrou.
func (a *App) funcoesAntiguidadeDoGrupo(grupoID int64, soAtivas bool) []FuncaoAntig {
	out := []FuncaoAntig{}
	q := `SELECT id, nome, pai_id, COALESCE(antiguidade, 999) FROM funcoes
	      WHERE grupo_id = ? AND (tipo = 'antiguidade' OR tipo IS NULL)`
	if soAtivas {
		q += ` AND ativo = 1`
	}
	q += ` ORDER BY COALESCE(antiguidade, 999), id`
	rows, err := a.st.db.Query(q, grupoID)
	if err != nil {
		return out
	}
	defer rows.Close()
	for rows.Next() {
		var f FuncaoAntig
		if rows.Scan(&f.ID, &f.Nome, &f.PaiID, &f.Antiguidade) == nil {
			out = append(out, f)
		}
	}
	return out
}

// GET /api/conferencia/funcoes-antiguidade — alimenta o picker "por
// antiguidade" do modal. Escopo = grupo da conferência, que nasce no grupo do
// usuário logado (escopoDoUsuario). Admin (sem grupo) recebe lista vazia +
// flag de aviso — nunca a lista global.
func (a *App) hConferenciaFuncoesAntiguidade(w http.ResponseWriter, r *http.Request) {
	u := usuarioDoCtx(r)
	escopo, err := a.exigeEscopo(u)
	if err != nil {
		jsonErro(w, http.StatusForbidden, "conta sem grupo definido")
		return
	}
	if escopo <= 0 {
		jsonOK(w, map[string]any{
			"funcoes":  []map[string]any{},
			"total":    0,
			"tem_tags": false,
			"aviso":    avisoSemTagsAntiguidade,
		})
		return
	}
	lista := a.funcoesAntiguidadeDoGrupo(escopo, true)
	its := []map[string]any{}
	for _, f := range lista {
		pai := int64(0)
		if f.PaiID != nil {
			pai = *f.PaiID
		}
		its = append(its, map[string]any{
			"id":          f.ID,
			"nome":        f.Nome,
			"pai_id":      pai,
			"antiguidade": f.Antiguidade,
		})
	}
	resp := map[string]any{
		"funcoes":  its,
		"total":    len(its),
		"tem_tags": len(its) > 0,
	}
	if len(its) == 0 {
		resp["aviso"] = avisoSemTagsAntiguidade
	}
	jsonOK(w, resp)
}

// migrarV43 (correção 09/10): a conferência por antiguidade usa a escada DO
// GRUPO (tags do gerente), não a seed global inventada pela v40.
//   - Novas conferências só aceitam funcao com grupo_id = grupo da conferência
//     (validação nos handlers) — o picker não oferece mais a seed global.
//   - A seed global (funcoes.grupo_id IS NULL AND tipo 'antiguidade') é APAGADA
//     SOMENTE se nada referenciar (pessoas, usuarios, usuario_papeis,
//     conferencia_funcoes). Linhas referenciadas ficam no banco: conferências
//     já criadas continuam abrindo e conferindo (compat), mas deixam de ser
//     oferecidas para novas conferências. Idempotente; nunca renumera v40/v41.
func (s *Store) migrarV43() error {
	var v int
	_ = s.db.QueryRow(`SELECT versao FROM schema_migrations WHERE versao = 43`).Scan(&v)
	if v == 43 {
		return nil
	}

	ids := []int64{}
	rows, err := s.db.Query(`SELECT id FROM funcoes WHERE grupo_id IS NULL AND (tipo = 'antiguidade' OR tipo IS NULL)`)
	if err != nil {
		return fmt.Errorf("migração v43 select seed global: %w", err)
	}
	for rows.Next() {
		var id int64
		if rows.Scan(&id) == nil {
			ids = append(ids, id)
		}
	}
	rows.Close()

	if len(ids) > 0 {
		ph := strings.TrimSuffix(strings.Repeat("?,", len(ids)), ",")
		args := make([]any, len(ids))
		for i, id := range ids {
			args[i] = id
		}
		referencias := []string{
			`SELECT COUNT(*) FROM pessoas WHERE funcao_id IN (` + ph + `)`,
			`SELECT COUNT(*) FROM usuarios WHERE funcao_id IN (` + ph + `)`,
			`SELECT COUNT(*) FROM usuario_papeis WHERE funcao_id IN (` + ph + `)`,
			`SELECT COUNT(*) FROM conferencia_funcoes WHERE funcao_id IN (` + ph + `)`,
		}
		total := 0
		for _, rq := range referencias {
			var n int
			if err := s.db.QueryRow(rq, args...).Scan(&n); err != nil {
				return fmt.Errorf("migração v42 checagem de referências: %w", err)
			}
			total += n
		}
		if total == 0 {
			if _, e := s.db.Exec(`DELETE FROM funcoes WHERE grupo_id IS NULL AND (tipo = 'antiguidade' OR tipo IS NULL)`, args...); e != nil {
				return fmt.Errorf("migração v42 delete seed global: %w", e)
			}
		}
		// Com referências: mantém as linhas (compat R4) — picker e validação já
		// não as oferecem/aceitam para conferências novas.
	}
	return s.marcarVersao(43)
}
