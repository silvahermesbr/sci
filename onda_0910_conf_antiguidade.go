package main

// Onda 09/10 — M1: conferência POR ANTIGUIDADE.
// Filtro de funções (postos/graduações) na conferência + endpoint pre_fechamento.
// Tabela conferencia_funcoes + migração v40 com seed das funções canônicas.

import (
	"fmt"
	"net/http"
	"strconv"
)

func (s *Store) migrarV40() error {
	var v int
	_ = s.db.QueryRow(`SELECT versao FROM schema_migrations WHERE versao = 40`).Scan(&v)
	if v == 40 {
		return nil
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

// predicadoAntiguidade devolve clausula SQL + args para filtrar pessoas que
// tenham funcao na conferencia_funcoes (p.funcao_id, u2.funcao_id ou up2.funcao_id).
// Se nao ha filtro, devolve vazio.
func (a *App) predicadoAntiguidade(cid int64) (string, []any) {
	rows, err := a.st.db.Query(`SELECT COUNT(*) FROM conferencia_funcoes WHERE conferencia_id = ?`, cid)
	if err != nil {
		return "", nil
	}
	defer rows.Close()
	var n int
	if rows.Next() {
		_ = rows.Scan(&n)
	}
	if n == 0 {
		return "", nil
	}
	clause := ` AND (p.funcao_id IN (SELECT funcao_id FROM conferencia_funcoes WHERE conferencia_id = ?)
OR EXISTS (SELECT 1 FROM usuarios u2 WHERE u2.pessoa_id = p.id AND u2.funcao_id IN (SELECT funcao_id FROM conferencia_funcoes WHERE conferencia_id = ?))
OR EXISTS (SELECT 1 FROM usuario_papeis up2 JOIN usuarios u3 ON u3.id = up2.usuario_id WHERE u3.pessoa_id = p.id AND (up2.grupo_id = p.grupo_id OR up2.grupo_id IS NULL) AND up2.funcao_id IN (SELECT funcao_id FROM conferencia_funcoes WHERE conferencia_id = ?)))`
	return clause, []any{cid, cid, cid}
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
	if esc := escopoDoUsuario(u); esc > 0 && confGrupoID != esc {
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
	             pr.situacao, pr.verificado, COALESCE(pr.observacao, '')
	      FROM pessoas p
	      LEFT JOIN funcoes fu ON fu.id = p.funcao_id
	      LEFT JOIN usuarios u2 ON u2.pessoa_id = p.id
	      LEFT JOIN funcoes fu_u ON fu_u.id = u2.funcao_id
	      LEFT JOIN usuario_papeis up2 ON up2.usuario_id = u2.id AND (up2.grupo_id = p.grupo_id OR up2.grupo_id IS NULL)
	      LEFT JOIN funcoes fu_up ON fu_up.id = up2.funcao_id
	      LEFT JOIN presencas pr ON pr.conferencia_id = ? AND pr.pessoa_id = p.id
	      WHERE p.setor_id = ? AND p.status = 'ativo'`
	queryArgs := []any{cid, sid}
	if esc := escopoDoUsuario(u); esc > 0 {
		q += ` AND p.grupo_id = ?`
		queryArgs = append(queryArgs, esc)
	}
	q += pred
	queryArgs = append(queryArgs, argsPred...)
	q += ` ORDER BY COALESCE(fu.antiguidade, 999), p.nome_guerra`

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