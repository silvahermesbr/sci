// server_escalas.go — extraído de server.go (onda de modularização; recorte puro).

package main

import (
	"database/sql"
	"fmt"
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"
)

func (a *App) hEscalasPDF(w http.ResponseWriter, r *http.Request) {
	if a.reservaAtivo() {
		jsonErro(w, http.StatusLocked, "módulo em reserva (indisponível nesta instalação)")
		return
	}
	u := usuarioDoCtx(r)
	escopo, err := a.exigeEscopo(u)
	if err != nil {
		jsonErro(w, http.StatusForbidden, "conta sem grupo definido")
		return
	}

	mes := strings.TrimSpace(r.URL.Query().Get("mes"))
	de := strings.TrimSpace(r.URL.Query().Get("de"))
	ate := strings.TrimSpace(r.URL.Query().Get("ate"))

	if mes != "" && de == "" && ate == "" {
		de = mes + "-01"
		ate = mes + "-31"
	}
	if de == "" || ate == "" {
		hoje := time.Now().In(a.horaLocal)
		de = hoje.Format("2006-01") + "-01"
		ate = hoje.Format("2006-01") + "-31"
	}

	var esc EscalasRelatorioPDF
	esc.Periodo = fmt.Sprintf("%s a %s", de, ate)

	var grupoNome string
	if escopo > 0 {
		_ = a.st.db.QueryRow(`SELECT nome FROM grupos WHERE id = ?`, escopo).Scan(&grupoNome)
	} else {
		grupoNome = "Todas as Subunidades"
	}
	esc.Grupo = grupoNome

	qTurnos := `SELECT et.id, et.data_inicio, et.data_fim, etp.nome, COALESCE(et.observacao, ''), COALESCE(g.nome, '')
	            FROM escala_turnos et
	            JOIN escala_tipos etp ON etp.id = et.tipo_id
	            LEFT JOIN grupos g ON g.id = et.grupo_id
	            WHERE substr(et.data_inicio, 1, 10) <= ? AND substr(et.data_fim, 1, 10) >= ?`
	args := []any{ate, de}
	if escopo > 0 {
		qTurnos += ` AND et.grupo_id = ?`
		args = append(args, escopo)
	}
	qTurnos += ` ORDER BY et.data_inicio ASC, et.id ASC`

	rows, err := a.st.db.Query(qTurnos, args...)
	if err != nil {
		jsonErro(w, http.StatusInternalServerError, "falha ao buscar turnos: "+err.Error())
		return
	}

	var turnoIDs []int64
	for rows.Next() {
		var tid int64
		var dIni, dFim, tNome, obs, gNome string
		if rows.Scan(&tid, &dIni, &dFim, &tNome, &obs, &gNome) == nil {
			turnoIDs = append(turnoIDs, tid)
			esc.Turnos = append(esc.Turnos, map[string]any{
				"id":          tid,
				"data_inicio": dIni,
				"data_fim":    dFim,
				"tipo_nome":   tNome,
				"observacao":  obs,
				"grupo_nome":  gNome,
				"militares":   "",
			})
		}
	}
	rows.Close()

	if len(turnoIDs) > 0 {
		ph := strings.TrimSuffix(strings.Repeat("?,", len(turnoIDs)), ",")
		qP := fmt.Sprintf(`
			SELECT ep.turno_id, p.nome_guerra, COALESCE(ep.funcao_escala, '')
			FROM escala_pessoas ep
			JOIN pessoas p ON p.id = ep.pessoa_id
			WHERE ep.turno_id IN (%s)
			ORDER BY p.nome_guerra ASC`, ph)

		argsT := make([]any, len(turnoIDs))
		for i, id := range turnoIDs {
			argsT[i] = id
		}

		pRows, pErr := a.st.db.Query(qP, argsT...)
		if pErr == nil {
			pPorTurno := map[int64][]string{}
			for pRows.Next() {
				var tid int64
				var ng, fEsc string
				if pRows.Scan(&tid, &ng, &fEsc) == nil {
					info := ng
					if fEsc != "" {
						info += " (" + fEsc + ")"
					}
					pPorTurno[tid] = append(pPorTurno[tid], info)
				}
			}
			pRows.Close()

			for i := range esc.Turnos {
				tid := esc.Turnos[i]["id"].(int64)
				if mils, ok := pPorTurno[tid]; ok {
					esc.Turnos[i]["militares"] = strings.Join(mils, ", ")
				}
			}
		}
	}

	pdf, err := a.gerarEscalasPDF(esc, u.Login)
	if err != nil {
		jsonErro(w, http.StatusInternalServerError, "falha ao gerar PDF de escalas: "+err.Error())
		return
	}

	a.st.Auditoria(&u.ID, "exportar", "escalas_pdf", nil, de+" a "+ate, ipDe(r))
	w.Header().Set("Cache-Control", "no-store, no-cache, must-revalidate")
	w.Header().Set("Pragma", "no-cache")
	w.Header().Set("Content-Type", "application/pdf")
	w.Header().Set("Content-Disposition", "inline; filename=escala_servico.pdf")
	_, _ = w.Write(pdf)
}

func (a *App) hEscalasTiposList(w http.ResponseWriter, r *http.Request) {
	u := usuarioDoCtx(r)
	escopo, err := a.exigeEscopo(u)
	if err != nil {
		jsonErro(w, http.StatusForbidden, "conta sem grupo definido")
		return
	}
	rows, err := a.st.db.Query(
		`SELECT id, COALESCE(grupo_id, 0), nome, COALESCE(descricao, ''), ativo, criado_em
		 FROM escala_tipos
		 WHERE (grupo_id IS NULL OR grupo_id = ? OR ? = 0)
		 ORDER BY nome`, escopo, escopo)
	if err != nil {
		jsonErro(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer rows.Close()
	var lista []map[string]any
	for rows.Next() {
		var id, gid int64
		var nome, desc, criada string
		var ativo int
		if rows.Scan(&id, &gid, &nome, &desc, &ativo, &criada) == nil {
			lista = append(lista, map[string]any{
				"id": id, "grupo_id": gid, "nome": nome, "descricao": desc,
				"ativo": ativo == 1, "criado_em": criada,
			})
		}
	}
	jsonOK(w, map[string]any{"tipos": lista})
}

func (a *App) hEscalasTiposAdd(w http.ResponseWriter, r *http.Request) {
	u := usuarioDoCtx(r)
	if _, err := a.exigeEscopo(u); err != nil {
		jsonErro(w, http.StatusForbidden, "conta sem grupo definido")
		return
	}
	var req struct {
		ID        int64  `json:"id"`
		Nome      string `json:"nome"`
		Descricao string `json:"descricao"`
		Ativo     *bool  `json:"ativo"`
	}
	if err := decodificar(r, &req); err != nil || strings.TrimSpace(req.Nome) == "" {
		jsonErro(w, http.StatusBadRequest, "Nome do tipo de escala é obrigatório")
		return
	}
	req.Nome = strings.TrimSpace(req.Nome)
	ativo := 1
	if req.Ativo != nil && !*req.Ativo {
		ativo = 0
	}
	if req.ID > 0 {
		_, err := a.st.db.Exec(`UPDATE escala_tipos SET nome = ?, descricao = ?, ativo = ? WHERE id = ?`,
			req.Nome, req.Descricao, ativo, req.ID)
		if err != nil {
			jsonErro(w, http.StatusInternalServerError, err.Error())
			return
		}
		a.st.Auditoria(&u.ID, "editar", "escala_tipos", &req.ID, req.Nome, ipDe(r))
		jsonOK(w, map[string]any{"ok": true, "id": req.ID})
		return
	}
	res, err := a.st.db.Exec(`INSERT INTO escala_tipos (grupo_id, nome, descricao, ativo) VALUES (?, ?, ?, ?)`,
		u.GrupoID, req.Nome, req.Descricao, ativo)
	if err != nil {
		jsonErro(w, http.StatusInternalServerError, err.Error())
		return
	}
	newID, _ := res.LastInsertId()
	a.st.Auditoria(&u.ID, "criar", "escala_tipos", &newID, req.Nome, ipDe(r))
	jsonOK(w, map[string]any{"ok": true, "id": newID})
}

func (a *App) hEscalasTiposDel(w http.ResponseWriter, r *http.Request) {
	u := usuarioDoCtx(r)
	if _, err := a.exigeEscopo(u); err != nil {
		jsonErro(w, http.StatusForbidden, "conta sem grupo definido")
		return
	}
	idStr := r.PathValue("id")
	id, _ := strconv.ParseInt(idStr, 10, 64)
	if id <= 0 {
		jsonErro(w, http.StatusBadRequest, "ID inválido")
		return
	}
	var turnosCount int
	_ = a.st.db.QueryRow(`SELECT COUNT(*) FROM escala_turnos WHERE tipo_id = ?`, id).Scan(&turnosCount)
	if turnosCount > 0 {
		_, _ = a.st.db.Exec(`UPDATE escala_tipos SET ativo = 0 WHERE id = ?`, id)
		jsonOK(w, map[string]any{"ok": true, "desativado": true})
		return
	}
	_, err := a.st.db.Exec(`DELETE FROM escala_tipos WHERE id = ?`, id)
	if err != nil {
		jsonErro(w, http.StatusInternalServerError, err.Error())
		return
	}
	a.st.Auditoria(&u.ID, "excluir", "escala_tipos", &id, "", ipDe(r))
	jsonOK(w, map[string]any{"ok": true})
}

func parseDataHoraTurno(s string) (time.Time, error) {
	s = strings.TrimSpace(s)
	formatos := []string{
		"2006-01-02T15:04:05",
		"2006-01-02T15:04",
		"2006-01-02 15:04:05",
		"2006-01-02 15:04",
		time.RFC3339,
	}
	for _, f := range formatos {
		if t, err := time.Parse(f, s); err == nil {
			return t, nil
		}
	}
	return time.Time{}, fmt.Errorf("formato de data/hora inválido: %s", s)
}

func (a *App) validarDescansoEscala(pessoaID int64, turnoID int64, dataInicioStr, dataFimStr string) InfoDescanso {
	info := InfoDescanso{Nivel: "ok", HorasFolga: 999, Mensagem: "Descanso adequado"}
	tIni, err1 := parseDataHoraTurno(dataInicioStr)
	tFim, err2 := parseDataHoraTurno(dataFimStr)
	if err1 != nil || err2 != nil {
		return info
	}

	rows, err := a.st.db.Query(`
		SELECT et.id, etp.nome, et.data_inicio, COALESCE(NULLIF(et.data_fim, ''), et.data_inicio)
		FROM escala_pessoas ep
		JOIN escala_turnos et ON et.id = ep.turno_id
		JOIN escala_tipos etp ON etp.id = et.tipo_id
		WHERE ep.pessoa_id = ? AND et.id != ?
	`, pessoaID, turnoID)
	if err != nil {
		return info
	}
	defer rows.Close()

	minFolga := 999999.0
	temOutro := false

	for rows.Next() {
		var oID int64
		var oNome, oIniStr, oFimStr string
		if rows.Scan(&oID, &oNome, &oIniStr, &oFimStr) == nil {
			oIni, e1 := parseDataHoraTurno(oIniStr)
			oFim, e2 := parseDataHoraTurno(oFimStr)
			if e1 != nil || e2 != nil {
				continue
			}

			// 1. Verificação estrita de sobreposição simultânea:
			// Dois intervalos [A_ini, A_fim] e [B_ini, B_fim] colidem se A_ini < B_fim E A_fim > B_ini
			if tIni.Before(oFim) && tFim.After(oIni) {
				info.Conflito = true
				info.ConflitoErro = fmt.Sprintf("Militar já escalado simultaneamente no posto '%s' (%s às %s)",
					oNome, oIni.Format("15:04"), oFim.Format("15:04"))
				info.Nivel = "conflito"
				info.HorasFolga = 0
				info.Mensagem = info.ConflitoErro
				return info
			}

			// 2. Cálculo do descanso (intervalo de folga entre escalas):
			var gap float64 = -1
			if !tIni.Before(oFim) { // este turno é após o outro
				gap = tIni.Sub(oFim).Hours()
			} else if !oIni.Before(tFim) { // este turno é antes do outro
				gap = oIni.Sub(tFim).Hours()
			}

			if gap >= 0 {
				temOutro = true
				if gap < minFolga {
					minFolga = gap
				}
			}
		}
	}

	if !temOutro {
		info.Nivel = "ok"
		info.HorasFolga = 999
		info.Mensagem = "Sem outros serviços próximos registrados"
		return info
	}

	info.HorasFolga = math.Round(minFolga*10) / 10
	if minFolga < 24.0 {
		info.Nivel = "critico"
		info.Mensagem = fmt.Sprintf("🔴 Alerta Crítico: Folga de apenas %.1fh (< 24h) em relação a outro serviço", info.HorasFolga)
	} else if minFolga < 48.0 {
		info.Nivel = "alerta"
		info.Mensagem = fmt.Sprintf("🟠 Alerta: Folga de %.1fh (< 48h) em relação a outro serviço", info.HorasFolga)
	} else if minFolga < 72.0 {
		info.Nivel = "atencao"
		info.Mensagem = fmt.Sprintf("🟡 Atenção: Folga de %.1fh (< 72h) em relação a outro serviço", info.HorasFolga)
	} else {
		info.Nivel = "ok"
		info.Mensagem = fmt.Sprintf("🟢 Descanso adequado (%.1fh)", info.HorasFolga)
	}

	return info
}

func (a *App) obterFuncoesOrdenadas(grupoID int64) []int64 {
	rows, err := a.st.db.Query(`
		SELECT id FROM funcoes 
		WHERE ativo = 1 AND (grupo_id IS NULL OR grupo_id = ?)
		ORDER BY antiguidade ASC, id ASC`, grupoID)
	if err != nil {
		return nil
	}
	defer rows.Close()
	var ids []int64
	for rows.Next() {
		var id int64
		if rows.Scan(&id) == nil {
			ids = append(ids, id)
		}
	}
	return ids
}

func (a *App) verificarFaixaPostoGrad(funcaoID *int64, minID *int64, maxID *int64, funcoesOrdenadas []int64) bool {
	if (minID == nil || *minID <= 0) && (maxID == nil || *maxID <= 0) {
		return true
	}
	if funcaoID == nil || *funcaoID <= 0 {
		return false
	}
	fID := *funcaoID

	posMap := make(map[int64]int)
	for i, id := range funcoesOrdenadas {
		posMap[id] = i
	}

	pPos, okP := posMap[fID]
	if !okP {
		return false
	}

	hasMin := minID != nil && *minID > 0
	hasMax := maxID != nil && *maxID > 0

	if hasMin && hasMax {
		minPos, okMin := posMap[*minID]
		maxPos, okMax := posMap[*maxID]
		if okMin && okMax {
			startPos := minPos
			endPos := maxPos
			if minPos > maxPos {
				startPos = maxPos
				endPos = minPos
			}
			return pPos >= startPos && pPos <= endPos
		}
	}

	if hasMin {
		minPos, okMin := posMap[*minID]
		if okMin && pPos < minPos {
			return false
		}
	}

	if hasMax {
		maxPos, okMax := posMap[*maxID]
		if okMax && pPos > maxPos {
			return false
		}
	}

	return true
}

func (a *App) hEscalasTurnosList(w http.ResponseWriter, r *http.Request) {
	u := usuarioDoCtx(r)
	escopo, err := a.exigeEscopo(u)
	if err != nil {
		jsonErro(w, http.StatusForbidden, "conta sem grupo definido")
		return
	}
	de := r.URL.Query().Get("de")
	ate := r.URL.Query().Get("ate")
	mes := r.URL.Query().Get("mes")
	dataQ := r.URL.Query().Get("data")

	var condEscopo string
	var args []any
	if escopo <= 0 {
		condEscopo = "1=1"
	} else {
		// Subordinados também enxergam escalas dos grupos superiores (v1.5)
		superiores := a.gruposSuperioresAtivos(escopo)
		gids := append([]int64{escopo}, superiores...)
		ph := strings.TrimSuffix(strings.Repeat("?,", len(gids)), ",")
		condEscopo = fmt.Sprintf("(et.grupo_id IN (%s) OR et.grupo_delegado_id = ?)", ph)
		for _, gid := range gids {
			args = append(args, gid)
		}
		args = append(args, escopo)
	}

	q := fmt.Sprintf(`SELECT et.id, et.grupo_id, et.tipo_id, etp.nome, et.data_inicio, et.data_fim, COALESCE(et.observacao,''),
	             COALESCE(u.login,''), et.criado_em, COALESCE(g.nome, ''),
	             COALESCE(et.fase, 'aberto'), et.modelo_id, et.grupo_delegado_id, COALESCE(et.status_delegacao, 'proprio'),
	             COALESCE(gd.nome, ''), et.posto_grad_min_id, et.posto_grad_max_id,
	             COALESCE(fgmin.nome, ''), COALESCE(fgmax.nome, '')
	      FROM escala_turnos et
	      JOIN escala_tipos etp ON etp.id = et.tipo_id
	      LEFT JOIN usuarios u ON u.id = et.criado_por
	      LEFT JOIN grupos g ON g.id = et.grupo_id
	      LEFT JOIN grupos gd ON gd.id = et.grupo_delegado_id
	      LEFT JOIN funcoes fgmin ON fgmin.id = et.posto_grad_min_id
	      LEFT JOIN funcoes fgmax ON fgmax.id = et.posto_grad_max_id
	      WHERE %s`, condEscopo)

	if mes != "" {
		q += ` AND (et.data_inicio LIKE ? OR et.data_fim LIKE ?)`
		args = append(args, mes+"%", mes+"%")
	} else if de != "" && ate != "" {
		q += ` AND et.data_fim >= ? AND et.data_inicio <= ?`
		args = append(args, de, ate)
	} else if dataQ != "" {
		q += ` AND (et.data_inicio LIKE ? OR (et.data_inicio <= ? AND et.data_fim >= ?))`
		args = append(args, dataQ+"%", dataQ+"T23:59:59", dataQ+"T00:00:00")
	}
	q += ` ORDER BY et.data_inicio DESC, et.id DESC`

	rows, err := a.st.db.Query(q, args...)
	if err != nil {
		jsonErro(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer rows.Close()

	type TurnoItem struct {
		ID                int64            `json:"id"`
		GrupoID           int64            `json:"grupo_id"`
		GrupoNome         string           `json:"grupo_nome"`
		TipoID            int64            `json:"tipo_id"`
		TipoNome          string           `json:"tipo_nome"`
		DataInicio        string           `json:"data_inicio"`
		DataFim           string           `json:"data_fim"`
		Observacao        string           `json:"observacao"`
		CriadoPor         string           `json:"criado_por"`
		CriadoEm          string           `json:"criado_em"`
		Fase              string           `json:"fase"`
		ModeloID          *int64           `json:"modelo_id"`
		GrupoDelegadoID   *int64           `json:"grupo_delegado_id"`
		StatusDelegacao   string           `json:"status_delegacao"`
		GrupoDelegadoNome string           `json:"grupo_delegado_nome"`
		PostoGradMinID    *int64           `json:"posto_grad_min_id"`
		PostoGradMaxID    *int64           `json:"posto_grad_max_id"`
		PostoGradMinNome  string           `json:"posto_grad_min_nome"`
		PostoGradMaxNome  string           `json:"posto_grad_max_nome"`
		Pessoas           []map[string]any `json:"pessoas"`
	}
	turnos := []TurnoItem{}
	var turnoIDs []any
	for rows.Next() {
		var t TurnoItem
		if rows.Scan(&t.ID, &t.GrupoID, &t.TipoID, &t.TipoNome, &t.DataInicio, &t.DataFim, &t.Observacao, &t.CriadoPor, &t.CriadoEm, &t.GrupoNome,
			&t.Fase, &t.ModeloID, &t.GrupoDelegadoID, &t.StatusDelegacao, &t.GrupoDelegadoNome,
			&t.PostoGradMinID, &t.PostoGradMaxID, &t.PostoGradMinNome, &t.PostoGradMaxNome) == nil {
			t.Pessoas = []map[string]any{}
			turnos = append(turnos, t)
			turnoIDs = append(turnoIDs, t.ID)
		}
	}
	rows.Close()

	if len(turnoIDs) > 0 {
		ph := strings.TrimSuffix(strings.Repeat("?,", len(turnoIDs)), ",")
		qP := fmt.Sprintf(`
			SELECT ep.turno_id, ep.pessoa_id, COALESCE(ep.funcao_escala, ''),
			       p.nome_guerra, p.nome_completo, COALESCE(s.nome, ''), COALESCE(fu.nome, '')
			FROM escala_pessoas ep
			JOIN pessoas p ON p.id = ep.pessoa_id
			LEFT JOIN setores s ON s.id = p.setor_id
			LEFT JOIN funcoes fu ON fu.id = p.funcao_id
			WHERE ep.turno_id IN (%s)
			ORDER BY p.nome_guerra`, ph)
		pRows, pErr := a.st.db.Query(qP, turnoIDs...)
		if pErr == nil {
			pessoasPorTurno := map[int64][]map[string]any{}
			for pRows.Next() {
				var tid, pid int64
				var fEscala, ng, nc, setor, funcao string
				if pRows.Scan(&tid, &pid, &fEscala, &ng, &nc, &setor, &funcao) == nil {
					pessoasPorTurno[tid] = append(pessoasPorTurno[tid], map[string]any{
						"pessoa_id":     pid,
						"funcao_escala": fEscala,
						"nome_guerra":   ng,
						"nome_completo": nc,
						"setor":         setor,
						"funcao":        funcao,
					})
				}
			}
			pRows.Close()
			for i := range turnos {
				if pes, ok := pessoasPorTurno[turnos[i].ID]; ok {
					turnos[i].Pessoas = pes
				}
			}
		}
	}
	jsonOK(w, map[string]any{"turnos": turnos})
}

func (a *App) hEscalasTurnosSave(w http.ResponseWriter, r *http.Request) {
	u := usuarioDoCtx(r)
	esc, err := a.exigeEscopo(u)
	if err != nil {
		jsonErro(w, http.StatusForbidden, "conta sem grupo definido")
		return
	}
	var req struct {
		ID             int64  `json:"id"`
		GrupoID        *int64 `json:"grupo_id"`
		TipoID         int64  `json:"tipo_id"`
		DataInicio     string `json:"data_inicio"`
		DataFim        string `json:"data_fim"`
		Observacao     string `json:"observacao"`
		PostoGradMinID *int64 `json:"posto_grad_min_id"`
		PostoGradMaxID *int64 `json:"posto_grad_max_id"`
		Pessoas        *[]struct {
			PessoaID     int64  `json:"pessoa_id"`
			FuncaoEscala string `json:"funcao_escala"`
		} `json:"pessoas"`
	}
	if err := decodificar(r, &req); err != nil || req.TipoID <= 0 || req.DataInicio == "" || req.DataFim == "" {
		jsonErro(w, http.StatusBadRequest, "Tipo de escala e datas de início/fim são obrigatórios")
		return
	}
	grupoID := int64(0)
	if u.GrupoID != nil {
		grupoID = *u.GrupoID
	}
	if req.GrupoID != nil && *req.GrupoID > 0 && u.Papel == "admin" {
		grupoID = *req.GrupoID
	}
	if grupoID <= 0 {
		_ = a.st.db.QueryRow(`SELECT id FROM grupos ORDER BY id LIMIT 1`).Scan(&grupoID)
	}
	if grupoID <= 0 {
		resG, errG := a.st.db.Exec(`INSERT INTO grupos (nome, codigo) VALUES ('1ª Cia (Geral)', ?)`, gerarCodigoGrupo())
		if errG == nil {
			grupoID, _ = resG.LastInsertId()
		}
	}
	if grupoID <= 0 {
		jsonErro(w, http.StatusBadRequest, "Grupo é obrigatório para o turno de serviço")
		return
	}

	// Validação de sobreposição para pessoas alocadas
	if req.Pessoas != nil {
		for _, p := range *req.Pessoas {
			if p.PessoaID > 0 {
				desc := a.validarDescansoEscala(p.PessoaID, req.ID, req.DataInicio, req.DataFim)
				if desc.Conflito {
					jsonErro(w, http.StatusBadRequest, desc.ConflitoErro)
					return
				}
			}
		}
	}

	tx, err := a.st.db.Begin()
	if err != nil {
		jsonErro(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer tx.Rollback()

	var turnoID = req.ID
	if turnoID > 0 {
		resUpd, err := tx.Exec(`
			UPDATE escala_turnos
			SET tipo_id = ?, data_inicio = ?, data_fim = ?, observacao = ?, posto_grad_min_id = ?, posto_grad_max_id = ?
			WHERE id = ? AND (? = 0 OR grupo_id = ?)`,
			req.TipoID, req.DataInicio, req.DataFim, req.Observacao, req.PostoGradMinID, req.PostoGradMaxID,
			turnoID, esc, grupoID)
		if err != nil {
			jsonErro(w, http.StatusInternalServerError, err.Error())
			return
		}
		if n, _ := resUpd.RowsAffected(); n == 0 {
			jsonErro(w, http.StatusNotFound, "turno não encontrado no seu escopo")
			return
		}
		if req.Pessoas != nil {
			_, _ = tx.Exec(`DELETE FROM escala_pessoas WHERE turno_id = ?`, turnoID)
		}
	} else {
		res, err := tx.Exec(`
			INSERT INTO escala_turnos (grupo_id, tipo_id, data_inicio, data_fim, observacao, criado_por, posto_grad_min_id, posto_grad_max_id)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
			grupoID, req.TipoID, req.DataInicio, req.DataFim, req.Observacao, u.ID, req.PostoGradMinID, req.PostoGradMaxID)
		if err != nil {
			jsonErro(w, http.StatusInternalServerError, err.Error())
			return
		}
		turnoID, _ = res.LastInsertId()
	}

	if req.Pessoas != nil {
		for _, p := range *req.Pessoas {
			if p.PessoaID > 0 {
				_, _ = tx.Exec(`
					INSERT INTO escala_pessoas (turno_id, pessoa_id, funcao_escala)
					VALUES (?, ?, ?)`, turnoID, p.PessoaID, p.FuncaoEscala)
			}
		}
	}

	if err := tx.Commit(); err != nil {
		jsonErro(w, http.StatusInternalServerError, err.Error())
		return
	}
	nPessoas := 0
	if req.Pessoas != nil {
		nPessoas = len(*req.Pessoas)
	}
	a.st.Auditoria(&u.ID, "salvar_turno", "escala_turnos", &turnoID,
		fmt.Sprintf("tipo=%d de=%s ate=%s pessoas=%d", req.TipoID, req.DataInicio, req.DataFim, nPessoas), ipDe(r))
	jsonOK(w, map[string]any{"ok": true, "id": turnoID})
}

func (a *App) hEscalasTurnosDel(w http.ResponseWriter, r *http.Request) {
	u := usuarioDoCtx(r)
	idStr := r.PathValue("id")
	id, _ := strconv.ParseInt(idStr, 10, 64)
	if id <= 0 {
		jsonErro(w, http.StatusBadRequest, "ID inválido")
		return
	}
	escopo, err := a.exigeEscopo(u)
	if err != nil {
		jsonErro(w, http.StatusForbidden, "conta sem grupo definido")
		return
	}
	res, err := a.st.db.Exec(`DELETE FROM escala_turnos WHERE id = ? AND (? = 0 OR grupo_id = ?)`, id, escopo, escopo)
	if err != nil {
		jsonErro(w, http.StatusInternalServerError, err.Error())
		return
	}
	ra, _ := res.RowsAffected()
	if ra == 0 {
		jsonErro(w, http.StatusNotFound, "Turno não encontrado ou sem permissão")
		return
	}
	a.st.Auditoria(&u.ID, "excluir", "escala_turnos", &id, "", ipDe(r))
	jsonOK(w, map[string]any{"ok": true})
}

func (a *App) hEscalasHoje(w http.ResponseWriter, r *http.Request) {
	u := usuarioDoCtx(r)
	escopo, err := a.exigeEscopo(u)
	if err != nil {
		jsonErro(w, http.StatusForbidden, "conta sem grupo definido")
		return
	}
	dataHoje := time.Now().In(a.horaLocal).Format("2006-01-02")
	escalados := a.escaladosNaData(escopo, dataHoje)
	jsonOK(w, map[string]any{"data": dataHoje, "escalados": escalados})
}

func (a *App) hEscalasModelosList(w http.ResponseWriter, r *http.Request) {
	u := usuarioDoCtx(r)
	escopo, err := a.exigeEscopo(u)
	if err != nil {
		jsonErro(w, http.StatusForbidden, "conta sem grupo definido")
		return
	}
	q := `SELECT em.id, em.nome, COALESCE(em.descricao,''), em.ativo, em.criado_em,
	             (SELECT COUNT(*) FROM escala_modelo_postos emp WHERE emp.modelo_id = em.id) AS total_postos,
	             (SELECT COUNT(*) FROM escala_modelo_aptos ema WHERE ema.modelo_id = em.id) AS total_aptos
	      FROM escala_modelos em
	      WHERE (em.grupo_id = ? OR ? = 0)
	      ORDER BY em.nome ASC`
	rows, err := a.st.db.Query(q, escopo, escopo)
	if err != nil {
		jsonErro(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer rows.Close()
	var lista []map[string]any
	if lista == nil {
		lista = make([]map[string]any, 0) // fix cia-F9: nil marshaliza null — contrato front
	}
	for rows.Next() {
		var id int64
		var nome, desc, criadoEm string
		var ativo, postos, aptos int
		if rows.Scan(&id, &nome, &desc, &ativo, &criadoEm, &postos, &aptos) == nil {
			lista = append(lista, map[string]any{
				"id":           id,
				"nome":         nome,
				"descricao":    desc,
				"ativo":        ativo == 1,
				"criado_em":    criadoEm,
				"total_postos": postos,
				"total_aptos":  aptos,
			})
		}
	}
	jsonOK(w, map[string]any{"modelos": lista})
}

func (a *App) hEscalasModelosGet(w http.ResponseWriter, r *http.Request) {
	u := usuarioDoCtx(r)
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id <= 0 {
		jsonErro(w, http.StatusBadRequest, "ID inválido")
		return
	}
	var mod struct {
		ID        int64  `json:"id"`
		GrupoID   int64  `json:"grupo_id"`
		Nome      string `json:"nome"`
		Descricao string `json:"descricao"`
		Ativo     bool   `json:"ativo"`
	}
	var ativoInt int
	err = a.st.db.QueryRow(`SELECT id, grupo_id, nome, COALESCE(descricao,''), ativo FROM escala_modelos WHERE id = ?`, id).
		Scan(&mod.ID, &mod.GrupoID, &mod.Nome, &mod.Descricao, &ativoInt)
	if err != nil {
		jsonErro(w, http.StatusNotFound, "Modelo não encontrado")
		return
	}
	mod.Ativo = ativoInt == 1

	// Fix cia-F8: o DETALHE não era escopado — bravo lia o modelo completo do
	// alpha com PII (nome_completo dos aptos) enquanto a LISTAGEM é escopada.
	// Mesmo guard de dono do Save (cia-F3): admin (escopo<=0) livre.
	var modeloGrupo int64
	if err := a.st.db.QueryRow(`SELECT COALESCE(grupo_id,0) FROM escala_modelos WHERE id = ?`, id).Scan(&modeloGrupo); err != nil {
		jsonErro(w, http.StatusNotFound, "Modelo não encontrado")
		return
	}
	esc, err := a.exigeEscopo(u)
	if err != nil {
		jsonErro(w, http.StatusForbidden, "conta sem grupo definido")
		return
	}
	if esc > 0 && modeloGrupo != esc {
		jsonErro(w, http.StatusForbidden, "modelo fora do seu escopo")
		return
	}

	// Postos
	pRows, _ := a.st.db.Query(`
		SELECT emp.id, emp.tipo_id, etp.nome, emp.hora_inicio, emp.hora_fim, emp.quantidade, emp.ordem,
		       emp.posto_grad_min_id, emp.posto_grad_max_id,
		       COALESCE(fgmin.nome, ''), COALESCE(fgmax.nome, '')
		FROM escala_modelo_postos emp
		JOIN escala_tipos etp ON etp.id = emp.tipo_id
		LEFT JOIN funcoes fgmin ON fgmin.id = emp.posto_grad_min_id
		LEFT JOIN funcoes fgmax ON fgmax.id = emp.posto_grad_max_id
		WHERE emp.modelo_id = ?
		ORDER BY emp.ordem ASC, emp.id ASC`, id)
	var postos []map[string]any
	if postos == nil {
		postos = make([]map[string]any, 0) // fix cia-F9: nil marshaliza null — contrato front
	}
	if pRows != nil {
		defer pRows.Close()
		for pRows.Next() {
			var pid, tid int64
			var tnome, hi, hf string
			var qtd, ord int
			var pgMinID, pgMaxID *int64
			var pgMinNome, pgMaxNome string
			if pRows.Scan(&pid, &tid, &tnome, &hi, &hf, &qtd, &ord, &pgMinID, &pgMaxID, &pgMinNome, &pgMaxNome) == nil {
				postos = append(postos, map[string]any{
					"id":                  pid,
					"tipo_id":             tid,
					"tipo_nome":           tnome,
					"hora_inicio":         hi,
					"hora_fim":            hf,
					"quantidade":          qtd,
					"ordem":               ord,
					"posto_grad_min_id":   pgMinID,
					"posto_grad_max_id":   pgMaxID,
					"posto_grad_min_nome": pgMinNome,
					"posto_grad_max_nome": pgMaxNome,
				})
			}
		}
	}

	// Aptos
	aRows, _ := a.st.db.Query(`
		SELECT ema.pessoa_id, p.nome_guerra, p.nome_completo, COALESCE(s.nome,''), COALESCE(fu.nome, '')
		FROM escala_modelo_aptos ema
		JOIN pessoas p ON p.id = ema.pessoa_id
		LEFT JOIN setores s ON s.id = p.setor_id
		LEFT JOIN funcoes fu ON fu.id = p.funcao_id
		WHERE ema.modelo_id = ?
		ORDER BY p.nome_guerra ASC`, id)
	var aptos []map[string]any
	if aptos == nil {
		aptos = make([]map[string]any, 0) // fix cia-F9: nil marshaliza null — contrato front
	}
	if aRows != nil {
		defer aRows.Close()
		for aRows.Next() {
			var pid int64
			var ng, nc, setor, funcao string
			if aRows.Scan(&pid, &ng, &nc, &setor, &funcao) == nil {
				aptos = append(aptos, map[string]any{
					"pessoa_id":     pid,
					"nome_guerra":   ng,
					"nome_completo": nc,
					"setor":         setor,
					"funcao":        funcao,
				})
			}
		}
	}

	jsonOK(w, map[string]any{
		"modelo": mod,
		"postos": postos,
		"aptos":  aptos,
	})
}

func (a *App) hEscalasModelosSave(w http.ResponseWriter, r *http.Request) {
	u := usuarioDoCtx(r)
	escopo, err := a.exigeEscopo(u)
	if err != nil {
		jsonErro(w, http.StatusForbidden, "conta sem grupo definido")
		return
	}
	if escopo <= 0 {
		jsonErro(w, http.StatusBadRequest, "Usuário deve pertencer a um grupo")
		return
	}
	var req struct {
		ID        int64  `json:"id"`
		Nome      string `json:"nome"`
		Descricao string `json:"descricao"`
		Postos    []struct {
			TipoID         int64  `json:"tipo_id"`
			HoraInicio     string `json:"hora_inicio"`
			HoraFim        string `json:"hora_fim"`
			Quantidade     int    `json:"quantidade"`
			Ordem          int    `json:"ordem"`
			PostoGradMinID *int64 `json:"posto_grad_min_id"`
			PostoGradMaxID *int64 `json:"posto_grad_max_id"`
		} `json:"postos"`
		AptosIDs []int64 `json:"aptos_ids"`
	}
	if err := decodificar(r, &req); err != nil || strings.TrimSpace(req.Nome) == "" {
		jsonErro(w, http.StatusBadRequest, "Nome do modelo é obrigatório")
		return
	}

	tx, err := a.st.db.Begin()
	if err != nil {
		jsonErro(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer tx.Rollback()

	modeloID := req.ID
	if modeloID > 0 {
		// Fix cia-F3: com id alheio, o UPDATE (escopado) não fazia nada (200
		// fantasma) mas os DELETE+INSERT de postos/aptos SEM filtro de grupo
		// REESCREVIAM o modelo da vítima. Checar dono antes de qualquer
		// DELETE/INSERT: dono = grupo_id do modelo == escopo do usuário (admin
		// escopo<=0 livre). A partir daqui os DELETEs operam sobre modelo próprio.
		var modeloGrupo int64
		if err := tx.QueryRow(`SELECT COALESCE(grupo_id,0) FROM escala_modelos WHERE id = ?`, modeloID).Scan(&modeloGrupo); err != nil {
			jsonErro(w, http.StatusNotFound, "Modelo não encontrado")
			return
		}
		if escopo > 0 && modeloGrupo != escopo {
			jsonErro(w, http.StatusForbidden, "modelo fora do seu escopo")
			return
		}
		ra, err := tx.Exec(`UPDATE escala_modelos SET nome = ?, descricao = ? WHERE id = ? AND grupo_id = ?`,
			req.Nome, req.Descricao, modeloID, escopo)
		if err != nil {
			jsonErro(w, http.StatusInternalServerError, err.Error())
			return
		}
		if n, _ := ra.RowsAffected(); n == 0 {
			jsonErro(w, http.StatusNotFound, "Modelo não encontrado no seu escopo")
			return
		}
		_, _ = tx.Exec(`DELETE FROM escala_modelo_postos WHERE modelo_id = ?`, modeloID)
		_, _ = tx.Exec(`DELETE FROM escala_modelo_aptos WHERE modelo_id = ?`, modeloID)
	} else {
		res, err := tx.Exec(`INSERT INTO escala_modelos (grupo_id, nome, descricao, ativo) VALUES (?, ?, ?, 1)`,
			escopo, req.Nome, req.Descricao)
		if err != nil {
			jsonErro(w, http.StatusInternalServerError, err.Error())
			return
		}
		modeloID, _ = res.LastInsertId()
	}

	for _, p := range req.Postos {
		qtd := p.Quantidade
		if qtd <= 0 {
			qtd = 1
		}
		hi := p.HoraInicio
		if hi == "" {
			hi = "07:00"
		}
		hf := p.HoraFim
		if hf == "" {
			hf = "07:00"
		}
		_, err = tx.Exec(`
			INSERT INTO escala_modelo_postos (modelo_id, tipo_id, hora_inicio, hora_fim, quantidade, ordem, posto_grad_min_id, posto_grad_max_id)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
			modeloID, p.TipoID, hi, hf, qtd, p.Ordem, p.PostoGradMinID, p.PostoGradMaxID)
		if err != nil {
			jsonErro(w, http.StatusInternalServerError, err.Error())
			return
		}
	}

	for _, pid := range req.AptosIDs {
		_, _ = tx.Exec(`INSERT OR IGNORE INTO escala_modelo_aptos (modelo_id, pessoa_id) VALUES (?, ?)`, modeloID, pid)
	}

	if err := tx.Commit(); err != nil {
		jsonErro(w, http.StatusInternalServerError, err.Error())
		return
	}

	jsonOK(w, map[string]any{"id": modeloID, "ok": true})
}

func (a *App) hEscalasModelosDel(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id <= 0 {
		jsonErro(w, http.StatusBadRequest, "ID inválido")
		return
	}
	u := usuarioDoCtx(r)
	escopo, err := a.exigeEscopo(u)
	if err != nil {
		jsonErro(w, http.StatusForbidden, "conta sem grupo definido")
		return
	}
	// Fix cia-F5: modelo referenciado por turnos explodia em 500 FK cru
	// (e ficava permanentemente indeletável). Pre-check: turnos apontando o
	// modelo → 409 com instrução; sem turnos → DELETE normal (dono/escopo).
	var turnos int
	if err := a.st.db.QueryRow(`SELECT COUNT(*) FROM escala_turnos WHERE modelo_id = ?`, id).Scan(&turnos); err != nil {
		jsonErro(w, http.StatusInternalServerError, err.Error())
		return
	}
	if turnos > 0 {
		jsonErro(w, http.StatusConflict, "modelo aplicado em turnos; exclua os turnos primeiro")
		return
	}
	res, err := a.st.db.Exec(`DELETE FROM escala_modelos WHERE id = ? AND (? = 0 OR grupo_id = ?)`, id, escopo, escopo)
	if err != nil {
		jsonErro(w, http.StatusInternalServerError, err.Error())
		return
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		jsonErro(w, http.StatusNotFound, "Modelo não encontrado")
		return
	}
	jsonOK(w, map[string]any{"ok": true})
}

func (a *App) hEscalasAplicarModelo(w http.ResponseWriter, r *http.Request) {
	u := usuarioDoCtx(r)
	escopo, err := a.exigeEscopo(u)
	if err != nil {
		jsonErro(w, http.StatusForbidden, "conta sem grupo definido")
		return
	}
	var req struct {
		ModeloID int64  `json:"modelo_id"`
		Data     string `json:"data"` // YYYY-MM-DD
		GrupoID  *int64 `json:"grupo_id"`
	}
	if err := decodificar(r, &req); err != nil || req.ModeloID <= 0 || req.Data == "" {
		jsonErro(w, http.StatusBadRequest, "modelo_id e data são obrigatórios")
		return
	}
	if escopo == 0 && req.GrupoID != nil && *req.GrupoID > 0 {
		escopo = *req.GrupoID
	}
	var modeloGrupo int64
	if err := a.st.db.QueryRow(`SELECT COALESCE(grupo_id, 0) FROM escala_modelos WHERE id = ?`, req.ModeloID).Scan(&modeloGrupo); err != nil {
		jsonErro(w, http.StatusNotFound, "modelo não encontrado")
		return
	}
	if escopo > 0 && modeloGrupo != escopo {
		jsonErro(w, http.StatusForbidden, "modelo fora do seu escopo")
		return
	}
	if escopo <= 0 {
		escopo = modeloGrupo
	}
	if escopo <= 0 {
		_ = a.st.db.QueryRow(`SELECT id FROM grupos ORDER BY id LIMIT 1`).Scan(&escopo)
	}

	// Buscar postos do modelo com faixas de posto/graduação
	pRows, err := a.st.db.Query(`
		SELECT tipo_id, hora_inicio, hora_fim, quantidade, posto_grad_min_id, posto_grad_max_id
		FROM escala_modelo_postos
		WHERE modelo_id = ?
		ORDER BY ordem ASC, id ASC`, req.ModeloID)
	if err != nil {
		jsonErro(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer pRows.Close()

	type postoDef struct {
		tipoID  int64
		hi, hf  string
		qtd     int
		pgMinID *int64
		pgMaxID *int64
	}
	var postos []postoDef
	for pRows.Next() {
		var p postoDef
		if pRows.Scan(&p.tipoID, &p.hi, &p.hf, &p.qtd, &p.pgMinID, &p.pgMaxID) == nil {
			postos = append(postos, p)
		}
	}
	pRows.Close()

	if len(postos) == 0 {
		jsonErro(w, http.StatusBadRequest, "o modelo selecionado não possui postos cadastrados")
		return
	}

	tBase, errDate := time.Parse("2006-01-02", req.Data)
	if errDate != nil {
		jsonErro(w, http.StatusBadRequest, "formato de data inválido (esperado YYYY-MM-DD)")
		return
	}

	tx, err := a.st.db.Begin()
	if err != nil {
		jsonErro(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer tx.Rollback()

	// Fix cia-F4: reaplicar o modelo na MESMA data duplicava turnos idênticos
	// (BUGS_s2 B3: 2 turnos, mesma data/tipo/modelo, sem aviso). MENOR mudança
	// segura = bloquear: turnos do MESMO modelo nesta data → 409 (o chefe tem
	// "limpar-dia" para substituir por decisão própria).
	var jaExistem int
	if err := tx.QueryRow(`SELECT COUNT(*) FROM escala_turnos
		WHERE modelo_id = ? AND substr(data_inicio,1,10) = ?`, req.ModeloID, req.Data).Scan(&jaExistem); err != nil {
		jsonErro(w, http.StatusInternalServerError, err.Error())
		return
	}
	if jaExistem > 0 {
		jsonErro(w, http.StatusConflict, "turnos já existentes para esta data (modelo já aplicado); use limpar-dia antes de reaplicar")
		return
	}

	criados := 0
	for _, p := range postos {
		dataIni := req.Data + "T" + p.hi + ":00"
		dataFimDia := req.Data
		if p.hf <= p.hi {
			dataFimDia = tBase.AddDate(0, 0, 1).Format("2006-01-02")
		}
		dataFim := dataFimDia + "T" + p.hf + ":00"

		for q := 0; q < p.qtd; q++ {
			_, err = tx.Exec(`
				INSERT INTO escala_turnos (grupo_id, tipo_id, data_inicio, data_fim, modelo_id, fase, status_delegacao, criado_por, posto_grad_min_id, posto_grad_max_id)
				VALUES (?, ?, ?, ?, ?, 'aberto', 'proprio', ?, ?, ?)`,
				escopo, p.tipoID, dataIni, dataFim, req.ModeloID, u.ID, p.pgMinID, p.pgMaxID)
			if err != nil {
				jsonErro(w, http.StatusInternalServerError, err.Error())
				return
			}
			criados++
		}
	}

	if err := tx.Commit(); err != nil {
		jsonErro(w, http.StatusInternalServerError, err.Error())
		return
	}

	a.st.Auditoria(&u.ID, "aplicar_modelo", "escala_turnos", &req.ModeloID,
		fmt.Sprintf("data=%s turnos_criados=%d grupo=%d", req.Data, criados, escopo), ipDe(r))
	jsonOK(w, map[string]any{"ok": true, "turnos_criados": criados, "fase": "aberto"})
}

func (a *App) hEscalasLimparDia(w http.ResponseWriter, r *http.Request) {
	u := usuarioDoCtx(r)
	escopo, err := a.exigeEscopo(u)
	if err != nil {
		jsonErro(w, http.StatusForbidden, "conta sem grupo definido")
		return
	}
	var req struct {
		Data    string `json:"data"` // YYYY-MM-DD
		GrupoID *int64 `json:"grupo_id"`
	}
	if err := decodificar(r, &req); err != nil || req.Data == "" {
		jsonErro(w, http.StatusBadRequest, "data obrigatória")
		return
	}
	if escopo == 0 && req.GrupoID != nil && *req.GrupoID > 0 {
		escopo = *req.GrupoID
	}

	res, err := a.st.db.Exec(`
		DELETE FROM escala_turnos
		WHERE (? = 0 OR grupo_id = ?) AND data_inicio LIKE ?`,
		escopo, escopo, req.Data+"%")
	if err != nil {
		jsonErro(w, http.StatusInternalServerError, err.Error())
		return
	}
	n, _ := res.RowsAffected()
	a.st.Auditoria(&u.ID, "limpar_dia", "escala_turnos", nil,
		fmt.Sprintf("data=%s grupo=%d removidos=%d", req.Data, escopo, n), ipDe(r))
	jsonOK(w, map[string]any{"ok": true, "removidos": n})
}

func (a *App) hEscalasTurnoAlocar(w http.ResponseWriter, r *http.Request) {
	turnoID, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || turnoID <= 0 {
		jsonErro(w, http.StatusBadRequest, "ID do turno inválido")
		return
	}
	var req struct {
		PessoaID     *int64 `json:"pessoa_id"` // se nil, desocupa o posto
		FuncaoEscala string `json:"funcao_escala"`
	}
	if err := decodificar(r, &req); err != nil {
		jsonErro(w, http.StatusBadRequest, "requisição inválida")
		return
	}

	u := usuarioDoCtx(r)
	escopo, err := a.exigeEscopo(u)
	if err != nil {
		jsonErro(w, http.StatusForbidden, "conta sem grupo definido")
		return
	}

	var turno struct {
		ID              int64
		GrupoID         int64
		TipoID          int64
		DataInicio      string
		DataFim         string
		ModeloID        *int64
		GrupoDelegadoID *int64
		PostoGradMinID  *int64
		PostoGradMaxID  *int64
	}
	err = a.st.db.QueryRow(`
		SELECT id, grupo_id, tipo_id, data_inicio, data_fim, modelo_id, grupo_delegado_id, posto_grad_min_id, posto_grad_max_id
		FROM escala_turnos WHERE id = ?`, turnoID).
		Scan(&turno.ID, &turno.GrupoID, &turno.TipoID, &turno.DataInicio, &turno.DataFim,
			&turno.ModeloID, &turno.GrupoDelegadoID, &turno.PostoGradMinID, &turno.PostoGradMaxID)
	if err != nil {
		jsonErro(w, http.StatusNotFound, "Turno não encontrado")
		return
	}

	// Permissão: admin, próprio grupo dono do turno, grupo delegado, ou grupo subordinado visualizando escala superior
	podeAlocar := false
	if u.Papel == "admin" || turno.GrupoID == escopo {
		podeAlocar = true
	} else if turno.GrupoDelegadoID != nil && *turno.GrupoDelegadoID == escopo {
		podeAlocar = true
	} else {
		superiores := a.gruposSuperioresAtivos(escopo)
		if int64Contem(superiores, turno.GrupoID) {
			podeAlocar = true
		}
	}
	if !podeAlocar {
		jsonErro(w, http.StatusForbidden, "Você não tem permissão para gerenciar este posto")
		return
	}

	var alertaDescanso InfoDescanso
	if req.PessoaID != nil && *req.PessoaID > 0 {
		pid := *req.PessoaID

		// 1. Aptos enforcement se turno oriundo de modelo com aptos definidos
		// Nota: Se o posto for delegado para um subgrupo, o subgrupo escala membros da sua própria fração
		ehDelegadoParaSub := turno.GrupoDelegadoID != nil && *turno.GrupoDelegadoID != turno.GrupoID
		if !ehDelegadoParaSub && turno.ModeloID != nil && *turno.ModeloID > 0 {
			var countAptos int
			_ = a.st.db.QueryRow(`SELECT count(*) FROM escala_modelo_aptos WHERE modelo_id = ?`, *turno.ModeloID).Scan(&countAptos)
			if countAptos > 0 {
				var estaApto int
				_ = a.st.db.QueryRow(`SELECT count(*) FROM escala_modelo_aptos WHERE modelo_id = ? AND pessoa_id = ?`, *turno.ModeloID, pid).Scan(&estaApto)
				if estaApto == 0 {
					jsonErro(w, http.StatusBadRequest, "O militar selecionado não está na lista de habilitados/aptos desta escala")
					return
				}
			}
		}

		// 2. Faixa de Posto/Graduação (mínima e máxima)
		if turno.PostoGradMinID != nil || turno.PostoGradMaxID != nil {
			var pFuncaoID *int64
			_ = a.st.db.QueryRow(`SELECT funcao_id FROM pessoas WHERE id = ?`, pid).Scan(&pFuncaoID)
			ord := a.obterFuncoesOrdenadas(turno.GrupoID)
			if !a.verificarFaixaPostoGrad(pFuncaoID, turno.PostoGradMinID, turno.PostoGradMaxID, ord) {
				jsonErro(w, http.StatusBadRequest, "O militar não atende à faixa de Posto/Graduação definida para este posto")
				return
			}
		}

		// 3. Algoritmo de conferência de permanência: sobreposição simultânea e descanso
		alertaDescanso = a.validarDescansoEscala(pid, turno.ID, turno.DataInicio, turno.DataFim)
		if alertaDescanso.Conflito {
			jsonErro(w, http.StatusBadRequest, alertaDescanso.ConflitoErro)
			return
		}
	}

	tx, err := a.st.db.Begin()
	if err != nil {
		jsonErro(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer tx.Rollback()

	_, _ = tx.Exec(`DELETE FROM escala_pessoas WHERE turno_id = ?`, turnoID)

	if req.PessoaID != nil && *req.PessoaID > 0 {
		_, err = tx.Exec(`INSERT INTO escala_pessoas (turno_id, pessoa_id, funcao_escala) VALUES (?, ?, ?)`,
			turnoID, *req.PessoaID, req.FuncaoEscala)
		if err != nil {
			jsonErro(w, http.StatusInternalServerError, err.Error())
			return
		}
		_, _ = tx.Exec(`UPDATE escala_turnos SET status_delegacao = 'preenchido' WHERE id = ?`, turnoID)
	} else {
		_, _ = tx.Exec(`UPDATE escala_turnos SET status_delegacao = 'proprio' WHERE id = ?`, turnoID)
	}

	if err := tx.Commit(); err != nil {
		jsonErro(w, http.StatusInternalServerError, err.Error())
		return
	}

	jsonOK(w, map[string]any{
		"ok":              true,
		"alerta_descanso": alertaDescanso,
	})
}

func (a *App) hEscalasTurnoDelegar(w http.ResponseWriter, r *http.Request) {
	turnoID, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || turnoID <= 0 {
		jsonErro(w, http.StatusBadRequest, "ID do turno inválido")
		return
	}
	var req struct {
		GrupoDelegadoID *int64 `json:"grupo_delegado_id"` // se nil, revoga delegação
	}
	if err := decodificar(r, &req); err != nil {
		jsonErro(w, http.StatusBadRequest, "requisição inválida")
		return
	}

	u := usuarioDoCtx(r)
	escopo, err := a.exigeEscopo(u)
	if err != nil {
		jsonErro(w, http.StatusForbidden, "conta sem grupo definido")
		return
	}

	if req.GrupoDelegadoID != nil && *req.GrupoDelegadoID > 0 {
		subs := a.gruposSubordinadosAtivos(escopo)
		if !int64Contem(subs, *req.GrupoDelegadoID) {
			jsonErro(w, http.StatusForbidden, "o grupo destino não é subordinado direto ou ativo do seu grupo")
			return
		}
		ra, err := a.st.db.Exec(`
			UPDATE escala_turnos
			SET grupo_delegado_id = ?, status_delegacao = 'delegado'
			WHERE id = ? AND (? = 0 OR grupo_id = ?)`,
			*req.GrupoDelegadoID, turnoID, escopo, escopo)
		if err != nil {
			jsonErro(w, http.StatusInternalServerError, err.Error())
			return
		}
		// Fix P1 (rodada 04/10): 200 fantasma — UPDATE com WHERE escopo TEM que checar
		// RowsAffected==0 → 404 (lição bd6a7af: 200-sem-efeito esconde falha silenciosa)
		if n, _ := ra.RowsAffected(); n == 0 {
			jsonErro(w, http.StatusNotFound, "turno não encontrado no seu escopo")
			return
		}
	} else {
		ra, err := a.st.db.Exec(`
			UPDATE escala_turnos
			SET grupo_delegado_id = NULL, status_delegacao = 'proprio'
			WHERE id = ? AND (? = 0 OR grupo_id = ?)`,
			turnoID, escopo, escopo)
		if err != nil {
			jsonErro(w, http.StatusInternalServerError, err.Error())
			return
		}
		if n, _ := ra.RowsAffected(); n == 0 {
			jsonErro(w, http.StatusNotFound, "turno não encontrado no seu escopo")
			return
		}
	}

	jsonOK(w, map[string]any{"ok": true})
}

func (a *App) hEscalasTurnoCandidatos(w http.ResponseWriter, r *http.Request) {
	turnoID, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || turnoID <= 0 {
		jsonErro(w, http.StatusBadRequest, "ID do turno inválido")
		return
	}
	u := usuarioDoCtx(r)
	escopo, err := a.exigeEscopo(u)
	if err != nil {
		jsonErro(w, http.StatusForbidden, "conta sem grupo definido")
		return
	}

	var turno struct {
		ID              int64  `json:"id"`
		GrupoID         int64  `json:"grupo_id"`
		TipoID          int64  `json:"tipo_id"`
		TipoNome        string `json:"tipo_nome"`
		DataInicio      string `json:"data_inicio"`
		DataFim         string `json:"data_fim"`
		ModeloID        *int64 `json:"modelo_id"`
		GrupoDelegadoID *int64 `json:"grupo_delegado_id"`
		PostoGradMinID  *int64 `json:"posto_grad_min_id"`
		PostoGradMaxID  *int64 `json:"posto_grad_max_id"`
	}
	err = a.st.db.QueryRow(`
		SELECT et.id, et.grupo_id, et.tipo_id, etp.nome, et.data_inicio, et.data_fim,
		       et.modelo_id, et.grupo_delegado_id, et.posto_grad_min_id, et.posto_grad_max_id
		FROM escala_turnos et
		JOIN escala_tipos etp ON etp.id = et.tipo_id
		WHERE et.id = ?`, turnoID).
		Scan(&turno.ID, &turno.GrupoID, &turno.TipoID, &turno.TipoNome, &turno.DataInicio, &turno.DataFim,
			&turno.ModeloID, &turno.GrupoDelegadoID, &turno.PostoGradMinID, &turno.PostoGradMaxID)
	if err != nil {
		jsonErro(w, http.StatusNotFound, "Turno não encontrado")
		return
	}

	// Permissão
	podeVer := false
	if u.Papel == "admin" || turno.GrupoID == escopo {
		podeVer = true
	} else if turno.GrupoDelegadoID != nil && *turno.GrupoDelegadoID == escopo {
		podeVer = true
	} else {
		superiores := a.gruposSuperioresAtivos(escopo)
		if int64Contem(superiores, turno.GrupoID) {
			podeVer = true
		}
	}
	if !podeVer {
		jsonErro(w, http.StatusForbidden, "Acesso não autorizado a este turno")
		return
	}

	// Grupo do qual os militares serão alocados:
	grupoMilitares := escopo
	if grupoMilitares <= 0 {
		grupoMilitares = turno.GrupoID
	}

	// Carregar aptos do modelo (se houver e não for delegado a subgrupo)
	aptosSet := make(map[int64]bool)
	temFiltroAptos := false
	ehDelegadoParaSub := turno.GrupoDelegadoID != nil && *turno.GrupoDelegadoID != turno.GrupoID
	if !ehDelegadoParaSub && turno.ModeloID != nil && *turno.ModeloID > 0 {
		aRows, aErr := a.st.db.Query(`SELECT pessoa_id FROM escala_modelo_aptos WHERE modelo_id = ?`, *turno.ModeloID)
		if aErr == nil {
			for aRows.Next() {
				var pid int64
				if aRows.Scan(&pid) == nil {
					aptosSet[pid] = true
					temFiltroAptos = true
				}
			}
			aRows.Close()
		}
	}

	// Carregar funcoes ordenadas para validação de faixa
	funcoesOrd := a.obterFuncoesOrdenadas(grupoMilitares)

	// Consultar militares ativos do grupo
	pRows, err := a.st.db.Query(`
		SELECT p.id, p.nome_guerra, p.nome_completo, p.funcao_id, COALESCE(fu.nome, ''), COALESCE(s.nome, '')
		FROM pessoas p
		LEFT JOIN funcoes fu ON fu.id = p.funcao_id
		LEFT JOIN setores s ON s.id = p.setor_id
		WHERE p.status = 'ativo' AND (? = 0 OR p.grupo_id = ?)
		ORDER BY fu.antiguidade ASC, p.nome_guerra ASC`, grupoMilitares, grupoMilitares)
	if err != nil {
		jsonErro(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer pRows.Close()

	type CandidatoItem struct {
		ID                  int64        `json:"id"`
		NomeGuerra          string       `json:"nome_guerra"`
		NomeCompleto        string       `json:"nome_completo"`
		FuncaoID            *int64       `json:"funcao_id"`
		Funcao              string       `json:"funcao"`
		Setor               string       `json:"setor"`
		Apto                bool         `json:"apto"`
		CompativelPostoGrad bool         `json:"compativel_posto_grad"`
		Descanso            InfoDescanso `json:"descanso"`
	}

	var candidatos []CandidatoItem
	for pRows.Next() {
		var c CandidatoItem
		if pRows.Scan(&c.ID, &c.NomeGuerra, &c.NomeCompleto, &c.FuncaoID, &c.Funcao, &c.Setor) == nil {
			if temFiltroAptos {
				c.Apto = aptosSet[c.ID]
			} else {
				c.Apto = true
			}
			c.CompativelPostoGrad = a.verificarFaixaPostoGrad(c.FuncaoID, turno.PostoGradMinID, turno.PostoGradMaxID, funcoesOrd)
			candidatos = append(candidatos, c)
		}
	}
	pRows.Close()

	for i := range candidatos {
		candidatos[i].Descanso = a.validarDescansoEscala(candidatos[i].ID, turno.ID, turno.DataInicio, turno.DataFim)
	}

	jsonOK(w, map[string]any{
		"turno":      turno,
		"candidatos": candidatos,
	})
}

func (a *App) hEscalasAlterarFase(w http.ResponseWriter, r *http.Request) {
	u := usuarioDoCtx(r)
	escopo, err := a.exigeEscopo(u)
	if err != nil {
		jsonErro(w, http.StatusForbidden, "conta sem grupo definido")
		return
	}
	var req struct {
		Data string `json:"data"` // YYYY-MM-DD
		Fase string `json:"fase"` // aberto | preenchido | aprovado | publicado
	}
	if err := decodificar(r, &req); err != nil || req.Data == "" || req.Fase == "" {
		jsonErro(w, http.StatusBadRequest, "data e fase são obrigatórios")
		return
	}
	switch req.Fase {
	case "aberto", "preenchido", "aprovado", "publicado":
	default:
		jsonErro(w, http.StatusBadRequest, "fase inválida (aberto | preenchido | aprovado | publicado)")
		return
	}

	res, err := a.st.db.Exec(`
		UPDATE escala_turnos
		SET fase = ?
		WHERE grupo_id = ? AND data_inicio LIKE ?`,
		req.Fase, escopo, req.Data+"%")
	if err != nil {
		jsonErro(w, http.StatusInternalServerError, err.Error())
		return
	}
	n, _ := res.RowsAffected()
	// Fix cia-F6: 200 fantasma {"atualizados":0} para não-dono (mesmo padrão do
	// fix de delegar, bd6a7af) — UPDATE sem match TEM que recusar.
	if n == 0 {
		jsonErro(w, http.StatusNotFound, "escala não encontrada no seu escopo")
		return
	}
	jsonOK(w, map[string]any{"ok": true, "atualizados": n, "fase": req.Fase})
}

func (a *App) hEscalasRelatorioDiaPDF(w http.ResponseWriter, r *http.Request) {
	u := usuarioDoCtx(r)
	escopo, err := a.exigeEscopo(u)
	if err != nil {
		jsonErro(w, http.StatusForbidden, "conta sem grupo definido")
		return
	}
	data := r.URL.Query().Get("data")
	if data == "" {
		data = time.Now().In(a.horaLocal).Format("2006-01-02")
	}

	var grupoNome string
	if escopo > 0 {
		_ = a.st.db.QueryRow(`SELECT nome FROM grupos WHERE id = ?`, escopo).Scan(&grupoNome)
	}
	if grupoNome == "" {
		grupoNome = "Comando Geral"
	}

	var fase string = "aberto"
	_ = a.st.db.QueryRow(`
		SELECT COALESCE(fase, 'aberto')
		FROM escala_turnos
		WHERE (? = 0 OR grupo_id = ?) AND data_inicio LIKE ?
		ORDER BY id DESC LIMIT 1`, escopo, escopo, data+"%").Scan(&fase)

	q := `SELECT et.id, etp.nome, et.data_inicio, COALESCE(NULLIF(et.data_fim, ''), et.data_inicio),
	             COALESCE(p.nome_guerra, ''), COALESCE(p.nome_completo, ''),
	             COALESCE(s.nome, ''), COALESCE(et.status_delegacao, 'proprio'), COALESCE(gd.nome, ''),
	             COALESCE(fu.nome, '')
	      FROM escala_turnos et
	      JOIN escala_tipos etp ON etp.id = et.tipo_id
	      LEFT JOIN escala_pessoas ep ON ep.turno_id = et.id
	      LEFT JOIN pessoas p ON p.id = ep.pessoa_id
	      LEFT JOIN funcoes fu ON fu.id = p.funcao_id
	      LEFT JOIN setores s ON s.id = p.setor_id
	      LEFT JOIN grupos gd ON gd.id = et.grupo_delegado_id
	      WHERE (? = 0 OR et.grupo_id = ?)
	        AND (et.data_inicio LIKE ? OR (substr(et.data_inicio, 1, 10) <= ? AND substr(COALESCE(NULLIF(et.data_fim, ''), et.data_inicio), 1, 10) >= ?))
	      ORDER BY et.data_inicio ASC, et.id ASC`

	rows, err := a.st.db.Query(q, escopo, escopo, data+"%", data, data)
	if err != nil {
		jsonErro(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer rows.Close()

	var turnosPDF []EscalaTurnoPDF
	for rows.Next() {
		var id int64
		var posto, di, df, ng, nc, setor, stDeleg, gDeleg, fuNome string
		if rows.Scan(&id, &posto, &di, &df, &ng, &nc, &setor, &stDeleg, &gDeleg, &fuNome) == nil {
			horario := ""
			if len(di) >= 16 && len(df) >= 16 {
				horario = di[11:16] + " às " + df[11:16]
			}
			origem := setor
			if stDeleg == "delegado" {
				if gDeleg != "" {
					origem = "Delegado: " + gDeleg
				} else {
					origem = "Delegado"
				}
			}
			militarNomeCompleto := nc
			militarNomeGuerra := ng
			if fuNome != "" {
				if militarNomeGuerra != "" {
					militarNomeGuerra = fuNome + " " + militarNomeGuerra
				}
				if militarNomeCompleto != "" {
					militarNomeCompleto = fuNome + " " + militarNomeCompleto
				}
			}
			turnosPDF = append(turnosPDF, EscalaTurnoPDF{
				ID:            id,
				PostoNome:     posto,
				Horario:       horario,
				MilitarNome:   militarNomeCompleto,
				MilitarGuerra: militarNomeGuerra,
				SetorOuOrigem: origem,
				Status:        stDeleg,
			})
		}
	}

	pdfData, err := a.gerarEscalaDiaPDF(EscalaDiaPDF{
		Data:      data,
		GrupoNome: grupoNome,
		Fase:      fase,
		GeradoPor: u.Login,
		Turnos:    turnosPDF,
	})
	if err != nil {
		jsonErro(w, http.StatusInternalServerError, "erro ao gerar PDF de escala: "+err.Error())
		return
	}

	w.Header().Set("Content-Type", "application/pdf")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`inline; filename="escala_%s.pdf"`, data))
	w.Header().Set("Content-Length", strconv.Itoa(len(pdfData)))
	_, _ = w.Write(pdfData)
}

func (a *App) hEscalasMinhas(w http.ResponseWriter, r *http.Request) {
	u := usuarioDoCtx(r)
	if u.PessoaID == nil {
		jsonOK(w, map[string]any{
			"escalas_aptas":   []any{},
			"proximos_turnos": []any{},
			"historico":       []any{},
		})
		return
	}
	pid := *u.PessoaID
	hoje := time.Now().In(a.horaLocal).Format("2006-01-02")

	// Escalas aptas
	mRows, _ := a.st.db.Query(`
		SELECT em.id, em.nome, COALESCE(em.descricao,''), COALESCE(g.nome,'')
		FROM escala_modelo_aptos ema
		JOIN escala_modelos em ON em.id = ema.modelo_id
		LEFT JOIN grupos g ON g.id = em.grupo_id
		WHERE ema.pessoa_id = ? AND em.ativo = 1
		ORDER BY em.nome ASC`, pid)
	var escalasAptas []map[string]any
	if mRows != nil {
		defer mRows.Close()
		for mRows.Next() {
			var id int64
			var nome, desc, gNome string
			if mRows.Scan(&id, &nome, &desc, &gNome) == nil {
				escalasAptas = append(escalasAptas, map[string]any{
					"id":         id,
					"nome":       nome,
					"descricao":  desc,
					"grupo_nome": gNome,
				})
			}
		}
	}

	// Próximos turnos escalados
	pRows, _ := a.st.db.Query(`
		SELECT et.id, etp.nome, et.data_inicio, et.data_fim, COALESCE(ep.funcao_escala,''), COALESCE(et.fase,'aberto'), COALESCE(g.nome,'')
		FROM escala_pessoas ep
		JOIN escala_turnos et ON et.id = ep.turno_id
		JOIN escala_tipos etp ON etp.id = et.tipo_id
		LEFT JOIN grupos g ON g.id = et.grupo_id
		WHERE ep.pessoa_id = ? AND et.data_fim >= ?
		ORDER BY et.data_inicio ASC LIMIT 20`, pid, hoje)
	var proximosTurnos []map[string]any
	if pRows != nil {
		defer pRows.Close()
		for pRows.Next() {
			var id int64
			var posto, di, df, fEscala, fase, gNome string
			if pRows.Scan(&id, &posto, &di, &df, &fEscala, &fase, &gNome) == nil {
				proximosTurnos = append(proximosTurnos, map[string]any{
					"turno_id":      id,
					"posto_nome":    posto,
					"data_inicio":   di,
					"data_fim":      df,
					"funcao_escala": fEscala,
					"fase":          fase,
					"grupo_nome":    gNome,
				})
			}
		}
	}

	// Histórico recente (passados)
	hRows, _ := a.st.db.Query(`
		SELECT et.id, etp.nome, et.data_inicio, et.data_fim, COALESCE(ep.funcao_escala,''), COALESCE(g.nome,'')
		FROM escala_pessoas ep
		JOIN escala_turnos et ON et.id = ep.turno_id
		JOIN escala_tipos etp ON etp.id = et.tipo_id
		LEFT JOIN grupos g ON g.id = et.grupo_id
		WHERE ep.pessoa_id = ? AND et.data_fim < ?
		ORDER BY et.data_inicio DESC LIMIT 20`, pid, hoje)
	var historico []map[string]any
	if hRows != nil {
		defer hRows.Close()
		for hRows.Next() {
			var id int64
			var posto, di, df, fEscala, gNome string
			if hRows.Scan(&id, &posto, &di, &df, &fEscala, &gNome) == nil {
				historico = append(historico, map[string]any{
					"turno_id":      id,
					"posto_nome":    posto,
					"data_inicio":   di,
					"data_fim":      df,
					"funcao_escala": fEscala,
					"grupo_nome":    gNome,
				})
			}
		}
	}

	jsonOK(w, map[string]any{
		"escalas_aptas":   escalasAptas,
		"proximos_turnos": proximosTurnos,
		"historico":       historico,
	})
}

func (a *App) reservaAtivo() bool {
	var v string
	// Fix P1: erro REAL de banco => fail-closed (em reserva). Flag AUSENTE
	// (sql.ErrNoRows) é estado válido por desenho — banco zerado nasce com
	// módulos ativos (contrato provado pela suíte: admin leva 403, não 423).
	if err := a.st.db.QueryRow(`SELECT valor FROM configuracoes WHERE chave = 'MODO_RESERVA'`).Scan(&v); err != nil && err != sql.ErrNoRows {
		return true
	}
	return v == "1"
}

// ---------- rotas rotasEscalas ----------
func (a *App) rotasEscalas() {
	m := a.mux

	// Módulos ESCALA e MATERIAL EM RESERVA (ordem Tenente 30/09): fora do frontend e
	// Módulos de Escalas e Material desbloqueados para apreciação (respeita MODO_RESERVA=1 se configurado)
	// Fix P1 (fail-closed): erro de banco => módulo em reserva (423), JAMais liberado; delega ao helper único.

	// Módulo de Escalas e Serviços Integrados (v1.0) — EM RESERVA (ordem Tenente 30/09)
	m.Handle("GET /api/escalas/tipos", a.reservaAuth(a.hEscalasTiposList))
	m.Handle("POST /api/escalas/tipos", a.reservaAuth(a.hEscalasTiposAdd))
	m.Handle("DELETE /api/escalas/tipos/{id}", a.reservaAuth(a.hEscalasTiposDel))
	m.Handle("GET /api/escalas/turnos", a.reservaAuth(a.hEscalasTurnosList))
	m.Handle("POST /api/escalas/turnos", a.reservaAuth(a.hEscalasTurnosSave))
	m.Handle("GET /api/escalas/hoje", a.reservaAuth(a.hEscalasHoje))
	m.Handle("GET /api/escalas/pdf", a.auth(false, a.hEscalasPDF))

	// Escalas 2.0 (v1.5) — Modelos, Fases, Delegação e Minhas Escalas
	m.Handle("GET /api/escalas/modelos", a.reservaAuth(a.hEscalasModelosList))
	m.Handle("POST /api/escalas/modelos", a.reservaAuth(a.hEscalasModelosSave))
	m.Handle("GET /api/escalas/modelos/{id}", a.reservaAuth(a.hEscalasModelosGet))
	m.Handle("DELETE /api/escalas/modelos/{id}", a.reservaAuth(a.hEscalasModelosDel))
	m.Handle("POST /api/escalas/aplicar-modelo", a.reservaAuth(a.hEscalasAplicarModelo))
	m.Handle("POST /api/escalas/limpar-dia", a.reservaAuth(a.hEscalasLimparDia))
	m.Handle("POST /api/escalas/turnos/{id}/alocar", a.reservaAuth(a.hEscalasTurnoAlocar))
	m.Handle("POST /api/escalas/turnos/{id}/delegar", a.reservaAuth(a.hEscalasTurnoDelegar))
	m.Handle("GET /api/escalas/turnos/{id}/candidatos", a.reservaAuth(a.hEscalasTurnoCandidatos))
	m.Handle("PATCH /api/escalas/fase", a.reservaAuth(a.hEscalasAlterarFase))
	m.Handle("GET /api/escalas/relatorio-dia.pdf", a.auth(false, a.hEscalasRelatorioDiaPDF))
	m.Handle("GET /api/escalas/relatorio-dia/pdf", a.auth(false, a.hEscalasRelatorioDiaPDF))
	m.Handle("GET /api/escalas/minhas", a.auth(false, a.hEscalasMinhas))
}

func (a *App) reservaAuth(next http.HandlerFunc) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if a.reservaAtivo() {
			jsonErro(w, http.StatusLocked, "módulo em reserva operacional")
			return
		}
		a.authPapeis([]string{"gerente", "operador", "chefe_setor"}, next).ServeHTTP(w, r)
	})
}
