// server_relatorios.go — extraído de server.go (onda de modularização; recorte puro).

package main

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"
)

func (a *App) hEfetivoAtual(w http.ResponseWriter, r *http.Request) {
	u := usuarioDoCtx(r)
	escopo, err := a.exigeEscopo(u)
	if err != nil {
		jsonErro(w, http.StatusForbidden, "conta sem grupo definido")
		return
	}
	q := `
		SELECT p.id, p.nome_guerra, COALESCE(s.nome,''), COALESCE(fu.nome,''),
		       COALESCE((SELECT g.nome FROM grupos g WHERE g.id = p.grupo_id),'—'),
		       ult.data, ult.situacao, ult.conferencia_id
		FROM pessoas p
		LEFT JOIN setores s ON s.id = p.setor_id
		LEFT JOIN funcoes fu ON fu.id = p.funcao_id
		LEFT JOIN (
			SELECT pr.pessoa_id, c.data, pr.situacao, pr.conferencia_id,
			       ROW_NUMBER() OVER (PARTITION BY pr.pessoa_id ORDER BY c.data DESC, c.id DESC) rn
			FROM presencas pr
			JOIN conferencias c ON c.id = pr.conferencia_id AND c.status = 'fechada'
		) ult ON ult.pessoa_id = p.id AND ult.rn = 1
		WHERE p.status = 'ativo'`
	var args []any
	if escopo > 0 {
		q += ` AND p.grupo_id = ?`
		args = append(args, escopo)
	} else if escopo == -1 {
		// Conta corrompida sem grupo: não vê ninguém
		jsonOK(w, map[string]any{"pessoas": []map[string]any{}, "hoje": time.Now().In(a.horaLocal).Format("2006-01-02")})
		return
	}
	q += ` ORDER BY COALESCE(s.nome,''), p.nome_guerra`
	rows, err := a.st.db.Query(q, args...)
	if err != nil {
		jsonErro(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer rows.Close()
	out := []map[string]any{}
	for rows.Next() {
		var id int64
		var ng, setor, funcao, grupo string
		var data, sit *string
		var confID *int64
		if rows.Scan(&id, &ng, &setor, &funcao, &grupo, &data, &sit, &confID) == nil {
			m := map[string]any{"id": id, "nome_guerra": ng, "setor": setor, "funcao": funcao, "grupo": grupo}
			if data != nil {
				m["ultima_data"] = *data
				m["situacao"] = *sit
				m["conferencia_id"] = *confID
			} else {
				m["ultima_data"] = nil
				m["situacao"] = nil
			}
			out = append(out, m)
		}
	}
	jsonOK(w, map[string]any{"pessoas": out, "hoje": time.Now().In(a.horaLocal).Format("2006-01-02")})
}

func (a *App) hRegistrosBusca(w http.ResponseWriter, r *http.Request) {
	u := usuarioDoCtx(r)
	escopo, err := a.exigeEscopo(u)
	if err != nil {
		jsonErro(w, http.StatusForbidden, "conta sem grupo definido")
		return
	}
	de, ate := a.periodoPadrao(r)
	filtrosJSON := r.URL.Query().Get("filtros")
	pessoaQ := r.URL.Query().Get("pessoa")

	cond := []string{"f.status='fechada'", "f.data BETWEEN ? AND ?"}
	args := []any{de, ate}
	if escopo > 0 {
		ft := a.filtroArvore(escopo, "f")
		cond = append(cond, strings.TrimPrefix(ft.clause, " AND "))
		args = append(args, ft.args...)
	}
	var pessoaID int64
	if pessoaQ != "" {
		p, err := strconv.ParseInt(pessoaQ, 10, 64)
		if err != nil || p <= 0 {
			jsonErro(w, http.StatusBadRequest, "pessoa inválida")
			return
		}
		pessoaID = p
		cond = append(cond, "pr.pessoa_id = ?")
		args = append(args, pessoaID)
	}
	where := " WHERE " + strings.Join(cond, " AND ")

	if filtrosJSON != "" {
		// modo filtros: regras (tag × período) em OU; retorna REGISTROS DE COMENTÁRIO
		var regras []struct {
			Tag int64  `json:"tag"`
			De  string `json:"de"`
			Ate string `json:"ate"`
		}
		if err := json.Unmarshal([]byte(filtrosJSON), &regras); err != nil || len(regras) == 0 {
			jsonErro(w, http.StatusBadRequest, "filtros inválidos")
			return
		}
		ou := []string{}
		oargs := []any{}
		for _, rg := range regras {
			if rg.Tag <= 0 || rg.De == "" || rg.Ate == "" {
				jsonErro(w, http.StatusBadRequest, "cada filtro exige tag, de e ate")
				return
			}
			ou = append(ou, "(t.tag_id = ? AND f.data BETWEEN ? AND ?)")
			oargs = append(oargs, rg.Tag, rg.De, rg.Ate)
		}
		q := `
		SELECT f.data, f.id, p.nome_guerra, COALESCE(tt.nome,''), t.comentario,
		       COALESCE(NULLIF(u.nome_guerra,''), NULLIF(u.nome_completo,''), '—'), t.criado_em, t.tag_id
		FROM comentarios t
		JOIN tags tt ON tt.id = t.tag_id
		JOIN conferencias f ON f.id = t.conferencia_id
		JOIN pessoas p ON p.id = t.pessoa_id
		JOIN usuarios u ON u.id = t.operador_id
		WHERE ( ` + strings.Join(ou, " OR ") + ` ) AND f.status='fechada'`
		qargs := oargs
		if escopo > 0 {
			ft := a.filtroArvore(escopo, "f")
			q += ft.clause
			qargs = append(qargs, ft.args...)
		}
		q += ` ORDER BY f.data DESC, f.id DESC, t.ordem DESC LIMIT 400`
		rows, err := a.st.db.Query(q, qargs...)
		if err != nil {
			jsonErro(w, http.StatusInternalServerError, err.Error())
			return
		}
		defer rows.Close()
		out := []map[string]any{}
		for rows.Next() {
			var data, nome, tag, comentario, por, em string
			var confID, tagID int64
			if rows.Scan(&data, &confID, &nome, &tag, &comentario, &por, &em, &tagID) == nil {
				out = append(out, map[string]any{
					"data": data, "conferencia_id": confID, "pessoa": nome, "tag": tag,
					"tag_id": tagID, "comentario": comentario, "operador": por,
					"datahora": em, "tipo": "comentario",
				})
			}
		}
		jsonOK(w, map[string]any{"registros": out, "modo": "filtros"})
		return
	}

	// modo padrão: lançamentos (com a TAG associada) da pessoa no período
	q := `
	SELECT f.data, f.id, COALESCE(f.local,''), pr.situacao, COALESCE(d.nome,''),
	       COALESCE(pr.observacao,''), COALESCE(tg.nome,''), pr.id
	FROM presencas pr
	JOIN conferencias f ON f.id = pr.conferencia_id
	LEFT JOIN destinos d ON d.id = pr.destino_id
	LEFT JOIN tags tg ON tg.id = pr.tag_id` + where + `
	ORDER BY f.data DESC, f.id DESC LIMIT 400`
	if pessoaID == 0 {
		jsonErro(w, http.StatusBadRequest, "informe pessoa=ID (ou filtros=...)")
		return
	}
	rows, err := a.st.db.Query(q, args...)
	if err != nil {
		jsonErro(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer rows.Close()
	out := []map[string]any{}
	for rows.Next() {
		var data, local, sit, destino, obs string
		var tags *string
		var confID, prID int64
		if rows.Scan(&data, &confID, &local, &sit, &destino, &obs, &tags, &prID) == nil {
			tagsOut := ""
			if tags != nil {
				tagsOut = *tags
			}
			out = append(out, map[string]any{
				"data": data, "conferencia_id": confID, "local": local, "situacao": sit,
				"destino": destino, "observacao": obs, "tags": tagsOut, "presenca_id": prID,
			})
		}
	}
	// ficha mínima da pessoa para o modal
	var nome, nomeCompleto string
	var setor, funcao *string
	_ = a.st.db.QueryRow(`SELECT p.nome_guerra, COALESCE(p.nome_completo,''),
		s.nome, fu.nome FROM pessoas p
		LEFT JOIN setores s ON s.id = p.setor_id
		LEFT JOIN funcoes fu ON fu.id = p.funcao_id WHERE p.id = ?`, pessoaID).
		Scan(&nome, &nomeCompleto, &setor, &funcao)
	ficha := map[string]any{"id": pessoaID, "nome_guerra": nome, "nome_completo": nomeCompleto,
		"setor": setor, "funcao": funcao}
	jsonOK(w, map[string]any{"pessoa": ficha, "registros": out, "de": de, "ate": ate})
}

func (a *App) escopoRelatorio(r *http.Request, u *Usuario) (int64, bool) {
	esc, err := a.exigeEscopo(u)
	if err != nil {
		return -1, false
	}
	q := r.URL.Query().Get("grupo")
	if esc == 0 {
		if q == "" {
			return 0, true
		}
		gid, err := strconv.ParseInt(q, 10, 64)
		if err != nil {
			return 0, true
		}
		return gid, true
	}
	if q == "" {
		return esc, true
	}
	gid, err := strconv.ParseInt(q, 10, 64)
	if err != nil {
		return esc, true
	}
	if gid == esc || int64Contem(a.gruposSubordinadosAtivos(esc), gid) {
		return gid, true
	}
	return esc, false
}

func (a *App) hPresencaPeriodo(w http.ResponseWriter, r *http.Request) {
	de, ate := a.periodoPadrao(r)
	u := usuarioDoCtx(r)
	if _, err := a.exigeEscopo(u); err != nil {
		jsonErro(w, http.StatusForbidden, "conta sem grupo definido")
		return
	}
	esc, ok := a.escopoRelatorio(r, u)
	if !ok {
		jsonErro(w, http.StatusForbidden, "grupo fora do seu escopo")
		return
	}
	// v9.15.2: relatório sempre fresco — sem cache HTTP
	w.Header().Set("Cache-Control", "no-store, no-cache, must-revalidate")
	w.Header().Set("Pragma", "no-cache")
	jsonOK(w, a.montarBundle(de, ate, esc))
}

func (a *App) hRelatorioJSON(w http.ResponseWriter, r *http.Request) {
	de, ate := a.periodoPadrao(r)
	u := usuarioDoCtx(r)
	if _, err := a.exigeEscopo(u); err != nil {
		jsonErro(w, http.StatusForbidden, "conta sem grupo definido")
		return
	}
	esc, ok := a.escopoRelatorio(r, u)
	if !ok {
		jsonErro(w, http.StatusForbidden, "grupo fora do seu escopo")
		return
	}
	jsonOK(w, a.montarBundle(de, ate, esc))
}

func (a *App) hRelatorioPDF(w http.ResponseWriter, r *http.Request) {
	de, ate := a.periodoPadrao(r)
	u := usuarioDoCtx(r)
	if _, err := a.exigeEscopo(u); err != nil {
		jsonErro(w, http.StatusForbidden, "conta sem grupo definido")
		return
	}
	esc, ok := a.escopoRelatorio(r, u)
	if !ok {
		jsonErro(w, http.StatusForbidden, "grupo fora do seu escopo")
		return
	}
	pdf, err := a.gerarRelatorioPDF(a.montarBundle(de, ate, esc))
	if err != nil {
		jsonErro(w, http.StatusInternalServerError, "falha ao gerar PDF: "+err.Error())
		return
	}
	a.st.Auditoria(&u.ID, "exportar", "relatorio", nil, de+" a "+ate, ipDe(r))
	// v9.15.2 (ordem Tenente): relatório SEMPRE on demand — proibir cache do navegador
	w.Header().Set("Cache-Control", "no-store, no-cache, must-revalidate")
	w.Header().Set("Pragma", "no-cache")
	w.Header().Set("Content-Type", "application/pdf")
	w.Header().Set("Content-Disposition",
		fmt.Sprintf("inline; filename=SCI_relatorio_%s_%s.pdf", de, ate))
	_, _ = w.Write(pdf)
}

func (a *App) hRelatorioDetalhadoPDF(w http.ResponseWriter, r *http.Request) {
	modo := r.URL.Query().Get("modo")
	if modo != "simples" && modo != "detalhado" {
		jsonErro(w, http.StatusBadRequest, "modo inválido (simples|detalhado)")
		return
	}
	de, ate := a.periodoPadrao(r)
	u := usuarioDoCtx(r)
	if _, err := a.exigeEscopo(u); err != nil {
		jsonErro(w, http.StatusForbidden, "conta sem grupo definido")
		return
	}
	esc, ok := a.escopoRelatorio(r, u)
	if !ok {
		jsonErro(w, http.StatusForbidden, "grupo fora do seu escopo")
		return
	}
	if esc == 0 {
		// Admin (escopo global): TODAS as grupos, uma folha cada
		// (modelo do Diretor: folha por grupo). Monta a lista aqui e segue.
		rows, qerr := a.st.db.Query(`SELECT id FROM grupos ORDER BY id`)
		if qerr != nil {
			jsonErro(w, http.StatusInternalServerError, "falha ao listar grupos")
			return
		}
		var ids []int64
		for rows.Next() {
			var gid int64
			if rows.Scan(&gid) == nil {
				ids = append(ids, gid)
			}
		}
		rows.Close()
		if len(ids) == 0 {
			jsonErro(w, http.StatusBadRequest, "nenhum grupo ativo no sistema")
			return
		}
		pdf, err := a.relatorioDetalhadoMulti(de, ate, modo, ids)
		if err != nil {
			jsonErro(w, http.StatusInternalServerError, "falha ao gerar PDF: "+err.Error())
			return
		}
		a.st.Auditoria(&u.ID, "exportar", "relatorio_detalhado", nil, de+" a "+ate+" modo="+modo+" admin-global", ipDe(r))
		w.Header().Set("Cache-Control", "no-store, no-cache, must-revalidate")
		w.Header().Set("Pragma", "no-cache")
		w.Header().Set("Content-Type", "application/pdf")
		w.Header().Set("Content-Disposition",
			fmt.Sprintf("inline; filename=SCI_relatorio_%s_%s.pdf", de, ate))
		_, _ = w.Write(pdf)
		return
	}
	pdf, err := a.relatorioDetalhado(de, ate, modo, esc)
	if err != nil {
		jsonErro(w, http.StatusInternalServerError, "falha ao gerar PDF: "+err.Error())
		return
	}
	a.st.Auditoria(&u.ID, "exportar", "relatorio_detalhado", nil, de+" a "+ate+" modo="+modo, ipDe(r))
	w.Header().Set("Cache-Control", "no-store, no-cache, must-revalidate")
	w.Header().Set("Pragma", "no-cache")
	w.Header().Set("Content-Type", "application/pdf")
	w.Header().Set("Content-Disposition",
		fmt.Sprintf("inline; filename=SCI_relatorio_%s_%s.pdf", de, ate))
	_, _ = w.Write(pdf)
}

func (a *App) hExportCSV(w http.ResponseWriter, r *http.Request) {
	t := r.PathValue("t")
	u := usuarioDoCtx(r)
	escopo, err := a.exigeEscopo(u)
	if err != nil {
		jsonErro(w, http.StatusForbidden, "conta sem grupo definido")
		return
	}
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", "attachment; filename=sci_"+t+".csv")
	_, _ = w.Write([]byte{0xEF, 0xBB, 0xBF}) // BOM p/ Excel
	cw := csv.NewWriter(w)
	switch t {
	case "pessoas":
		_ = cw.Write([]string{"id", "nome_guerra", "nome_completo", "setor", "funcao", "status", "criado_em"})
		rows, err := a.st.db.Query(`SELECT p.id, p.nome_guerra, p.nome_completo,
			COALESCE(s.nome,''), COALESCE(fu.nome,''), p.status, p.criado_em
			FROM pessoas p LEFT JOIN setores s ON s.id=p.setor_id
			LEFT JOIN funcoes fu ON fu.id=p.funcao_id
			WHERE 1=1`+filtroGrupoSQL(escopo, "p")+` ORDER BY p.id`,
			filtroGrupoArgs(escopo)...)
		if err == nil {
			defer rows.Close()
			for rows.Next() {
				var id int64
				var v [6]string
				if rows.Scan(&id, &v[0], &v[1], &v[2], &v[3], &v[4], &v[5]) == nil {
					_ = cw.Write([]string{strconv.FormatInt(id, 10), v[0], v[1], v[2], v[3], v[4], v[5]})
				}
			}
		}
	case "presencas":
		_ = cw.Write([]string{"data", "conferencia_tipo", "nome_guerra", "situacao", "destino", "marcado_em"})
		rows, err := a.st.db.Query(`SELECT f.data, ft.nome, p.nome_guerra, pr.situacao,
			COALESCE(d.nome,''), pr.marcado_em
			FROM presencas pr JOIN conferencias f ON f.id=pr.conferencia_id
			JOIN conferencia_tipos ft ON ft.id=f.tipo_id
			JOIN pessoas p ON p.id=pr.pessoa_id LEFT JOIN destinos d ON d.id=pr.destino_id
			WHERE 1=1`+filtroGrupoSQL(escopo, "f")+` ORDER BY f.data, p.nome_guerra`,
			filtroGrupoArgs(escopo)...)
		if err == nil {
			defer rows.Close()
			for rows.Next() {
				var v [6]string
				if rows.Scan(&v[0], &v[1], &v[2], &v[3], &v[4], &v[5]) == nil {
					_ = cw.Write(v[:])
				}
			}
		}
	case "conferencias":
		_ = cw.Write([]string{"data", "hora", "tipo", "local", "status"})
		rows, err := a.st.db.Query(`SELECT f.data, COALESCE(f.hora,''), ft.nome,
			COALESCE(f.local,''), f.status FROM conferencias f
			JOIN conferencia_tipos ft ON ft.id=f.tipo_id
			WHERE 1=1`+filtroGrupoSQL(escopo, "f")+` ORDER BY f.data`,
			filtroGrupoArgs(escopo)...)
		if err == nil {
			defer rows.Close()
			for rows.Next() {
				var v [5]string
				if rows.Scan(&v[0], &v[1], &v[2], &v[3], &v[4]) == nil {
					_ = cw.Write(v[:])
				}
			}
		}
	default:
		_ = cw.Write([]string{"erro", "entidade inválida"})
	}
	cw.Flush()
}

// ---------- rotas rotasRelatorios ----------
func (a *App) rotasRelatorios() {
	m := a.mux

	// Busca individual nos relatórios (ordem Tenente 30/09): registros por pessoa+período
	// e filtros complexos multi-seleção (TAG × período em OU).
	m.Handle("GET /api/relatorio/registros", a.auth(false, a.hRegistrosBusca))
	m.Handle("GET /api/relatorio/tags", a.auth(false, a.hTagsDisponiveis))

	m.Handle("GET /api/efetivo_atual", a.auth(false, a.hEfetivoAtual)) // todos os papéis: admin vê todos, demais veem o escopo
	m.Handle("GET /api/presenca/periodo", a.auth(false, a.hPresencaPeriodo))

	m.Handle("GET /api/relatorio", a.auth(false, a.hRelatorioJSON))
	m.Handle("GET /api/relatorio.pdf", a.auth(false, a.hRelatorioPDF))
	m.Handle("GET /api/relatorio/detalhado.pdf", a.auth(false, a.hRelatorioDetalhadoPDF))
	m.Handle("GET /api/export/{t}", a.auth(false, a.hExportCSV))
	m.Handle("GET /api/export", a.auth(false, a.hExportarDados))
}
