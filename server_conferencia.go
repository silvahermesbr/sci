// server_conferencia.go — extraído de server.go (onda de modularização; recorte puro).

package main

import (
	"database/sql"
	"errors"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"
)

func (a *App) hConferenciaHoje(w http.ResponseWriter, r *http.Request) {
	a.ensureTabelaDespachos()
	u := usuarioDoCtx(r)
	escopo, err := a.exigeEscopo(u)
	if err != nil {
		jsonErro(w, http.StatusForbidden, "conta sem grupo definido")
		return
	}
	var f struct {
		ID       int64
		Status   string
		Data     string
		CriadaEm string
	}
	// admin não tem grupo: nunca há "conferência do admin" — a área fica em modo leitura
	if escopo == 0 && u.Papel == "admin" {
		pessoas := a.pessoasAtivas(0)
		jsonOK(w, map[string]any{
			"conferencia":      nil,
			"pessoas":          pessoas,
			"setores_status":   []map[string]any{},
			"total_banco":      len(pessoas),
			"total_verificado": 0,
			"setores_fechados": 0,
			"total_setores":    0,
		})
		return
	}
	// v9.14.2: ?id=N abre conferência específica (várias simultâneas); senão a mais recente
	idQ := r.URL.Query().Get("id")
	qHoje := `SELECT id, status, data, criado_em FROM conferencias
		 WHERE status = 'aberta' AND grupo_id = ?`
	argsHoje := []any{escopo}
	if idQ != "" {
		if cid, e := strconv.ParseInt(idQ, 10, 64); e == nil {
			qHoje += ` AND id = ?`
			argsHoje = append(argsHoje, cid)
		}
	}
	qHoje += ` ORDER BY id DESC LIMIT 1`
	err = a.st.db.QueryRow(qHoje, argsHoje...).
		Scan(&f.ID, &f.Status, &f.Data, &f.CriadaEm)
	var form *map[string]any
	if err == nil {
		estados := map[int64]map[string]any{}
		rows, e := a.st.db.Query(
			`SELECT pessoa_id, situacao, destino_id, COALESCE(observacao,''), verificado
			 FROM presencas WHERE conferencia_id = ?`, f.ID)
		if e == nil {
			for rows.Next() {
				var pid int64
				var sit string
				var did *int64
				var obs string
				var verificado int
				if rows.Scan(&pid, &sit, &did, &obs, &verificado) == nil {
					estados[pid] = map[string]any{"situacao": sit, "destino_id": did, "observacao": obs,
						"verificado": verificado == 1}
				}
			}
			rows.Close()
		}
		form = &map[string]any{"id": f.ID, "status": f.Status, "data": f.Data,
			"criada_em": f.CriadaEm, "estados": estados}
	}

	despachadosMap := make(map[int64]bool)
	if f.ID > 0 {
		rowsD, errD := a.st.db.Query(`SELECT setor_id FROM conferencia_despachos WHERE conferencia_id = ?`, f.ID)
		if errD == nil {
			for rowsD.Next() {
				var sid int64
				if rowsD.Scan(&sid) == nil {
					despachadosMap[sid] = true
				}
			}
			rowsD.Close()
		}
	}
	temDespachos := len(despachadosMap) > 0

	// Filtro por funções (antiguidade)
	var filtroFuncoes []int64
	var temFiltroFuncoes bool
	var predFuncoes string
	var argsPredFuncoes []any
	if f.ID > 0 {
		filtroFuncoes = a.funcoesFiltroDaConferencia(f.ID)
		temFiltroFuncoes = len(filtroFuncoes) > 0
		if temFiltroFuncoes {
			predFuncoes, argsPredFuncoes = a.predicadoAntiguidade(f.ID)
		}
	}

	var setoresStatus []map[string]any
	if f.ID > 0 {
		qSetores := `SELECT s.id, s.nome, COALESCE(s.sigla, ''),
		       COALESCE(cs.status, 'nao_iniciada'),
		       cs.concluido_por, COALESCE(NULLIF(u.nome_guerra,''), NULLIF(u.nome_completo,''), '—'), cs.concluido_em,
		       COUNT(DISTINCT p.id) AS total_efetivo,
		       COUNT(DISTINCT CASE WHEN pr.verificado = 1 THEN p.id ELSE NULL END) AS total_verificados
		FROM setores s
		JOIN pessoas p ON p.setor_id = s.id AND p.status = 'ativo' AND (? = 0 OR p.grupo_id = ?)`
		if temFiltroFuncoes {
			qSetores += predFuncoes
		}
		qSetores += `
		LEFT JOIN conferencia_setores cs ON cs.setor_id = s.id AND cs.conferencia_id = ?
		LEFT JOIN usuarios u ON u.id = cs.concluido_por
		LEFT JOIN presencas pr ON pr.conferencia_id = ? AND pr.pessoa_id = p.id
		WHERE s.ativo = 1 AND (? = 0 OR s.grupo_id = ? OR s.grupo_id IS NULL)
		GROUP BY s.id, s.nome, s.sigla, cs.status, cs.concluido_por, u.nome_guerra, u.login, cs.concluido_em
		ORDER BY s.nome ASC`
		qArgs := []any{escopo, escopo}
		for _, a := range argsPredFuncoes {
			qArgs = append(qArgs, a)
		}
		qArgs = append(qArgs, f.ID, f.ID, escopo, escopo)
		sRows, sErr := a.st.db.Query(qSetores, qArgs...)
		if sErr == nil {
			for sRows.Next() {
				var sid int64
				var sNome, sSigla, sStatus, concNome string
				var concPor *int64
				var concEm *string
				var totEf, totVer int
				if sRows.Scan(&sid, &sNome, &sSigla, &sStatus, &concPor, &concNome, &concEm, &totEf, &totVer) == nil {
					pct := 0
					if totEf > 0 {
						pct = (totVer * 100) / totEf
					}
					item := map[string]any{
						"setor_id":           sid,
						"setor_nome":         sNome,
						"setor_sigla":        sSigla,
						"status":             sStatus,
						"total_efetivo":      totEf,
						"total_pessoas":      totEf,
						"total_verificados":  totVer,
						"verificados":        totVer,
						"concluido_por_id":   concPor,
						"concluido_por_nome": concNome,
						"concluido_em":       concEm,
						"pct_conferido":      pct,
						"despachado":         despachadosMap[sid],
					}
					setoresStatus = append(setoresStatus, item)
				}
			}
			sRows.Close()
		}
	}
	if setoresStatus == nil {
		setoresStatus = []map[string]any{}
	}

	dataHoje := time.Now().In(a.horaLocal).Format("2006-01-02")
	if f.Data != "" {
		dataHoje = f.Data
	}
	escalados := a.escaladosNaData(escopo, dataHoje)
	tHoje, _ := time.Parse("2006-01-02", dataHoje)
	dataOntem := tHoje.AddDate(0, 0, -1).Format("2006-01-02")
	escaladosOntem := a.escaladosNaData(escopo, dataOntem)

	pessoasGrupo := a.pessoasAtivasOpt(escopo, filtroFuncoes)
	totalBanco := len(pessoasGrupo)

	// ordem 08/10 — corte por CONTEXTO ATIVO: chefe_setor e operador só
	// veem/lançam o setor ATIVO na sessão (sessoes.setor_ativo_id → u.SetorID;
	// setorDoUsuario mantém o fallback legado da pessoa vinculada). Antes,
	// chefeComandaSetor devolvia TODOS os setores comandados — o chefe via e
	// operava o efetivo dos outros setores mesmo com outro contexto ativo.
	ehChefeOuOper := (u.Papel == "chefe_setor" || u.Papel == "operador") &&
		!a.ehEncarregado(u) && !a.ehAuxiliarDePessoal(u)
	var ativo *int64
	if ehChefeOuOper {
		ativo = setorDoUsuario(a, u)
		if ativo != nil {
			somenteAtivo := []map[string]any{}
			for _, p := range pessoasGrupo {
				if sid, ok := p["setor_id"].(int64); ok && sid == *ativo {
					somenteAtivo = append(somenteAtivo, p)
				}
			}
			pessoasGrupo = somenteAtivo
		} else {
			pessoasGrupo = []map[string]any{}
		}
	}

	deveFiltrar := f.ID > 0 && temDespachos && ehChefeOuOper

	// ordem 08/10: estados de lançamento (presenças) também seguem o contexto —
	// só entram no fio os de pessoas do setor ativo (sem cruzamento de contadores).
	if ehChefeOuOper && form != nil && ativo != nil {
		if est, ok := (*form)["estados"].(map[int64]map[string]any); ok {
			estFiltrado := map[int64]map[string]any{}
			for pid := range est {
				var pSetor *int64
				_ = a.st.db.QueryRow(`SELECT setor_id FROM pessoas WHERE id = ?`, pid).Scan(&pSetor)
				if pSetor != nil && *pSetor == *ativo {
					estFiltrado[pid] = est[pid]
				}
			}
			(*form)["estados"] = estFiltrado
		}
	}

	if deveFiltrar {
		var filtrados []map[string]any
		for _, st := range setoresStatus {
			sid, _ := st["setor_id"].(int64)
			if !despachadosMap[sid] {
				continue
			}
			if ativo != nil && *ativo == sid {
				filtrados = append(filtrados, st)
			}
		}
		if len(filtrados) == 0 {
			jsonOK(w, map[string]any{
				"conferencia":      nil,
				"pessoas":          []any{},
				"setores_status":   []map[string]any{},
				"total_banco":      totalBanco,
				"total_verificado": 0,
				"setores_fechados": 0,
				"total_setores":    0,
				"escalados":        escalados,
				"escalados_ontem":  escaladosOntem,
				"escala":           a.escalaDaConferencia(f.ID),
			})
			return
		}
		setoresStatus = filtrados
	}

	totalVerificado := 0
	setoresFechados := 0
	for _, st := range setoresStatus {
		if v, ok := st["verificados"].(int); ok {
			totalVerificado += v
		}
		if s, ok := st["status"].(string); ok && s == "concluida" {
			setoresFechados++
		}
	}
	totalSetores := len(setoresStatus)

	// onda 09/10: modo da conferência + nomes das funções do filtro (antiguidade)
	modoConf := "setores"
	nomesFiltro := []string{}
	if temFiltroFuncoes {
		modoConf = "antiguidade"
		ph := ""
		argsNome := []any{}
		for i, fid := range filtroFuncoes {
			if i > 0 {
				ph += ","
			}
			ph += "?"
			argsNome = append(argsNome, fid)
		}
		rowsN, eN := a.st.db.Query(`SELECT nome FROM funcoes WHERE id IN (`+ph+`)
			ORDER BY CASE WHEN tipo = 'antiguidade' THEN COALESCE(antiguidade, 999) ELSE 999 END, nome`, argsNome...)
		if eN == nil {
			for rowsN.Next() {
				var nm string
				if rowsN.Scan(&nm) == nil {
					nomesFiltro = append(nomesFiltro, nm)
				}
			}
			rowsN.Close()
		}
	}

	jsonOK(w, map[string]any{
		"conferencia":      form,
		"setores_status":   setoresStatus,
		"pessoas":          pessoasGrupo,
		"escalados":        escalados,
		"escalados_ontem":  escaladosOntem,
		"escala":           a.escalaDaConferencia(f.ID),
		"total_banco":      totalBanco,
		"total_verificado": totalVerificado,
		"setores_fechados": setoresFechados,
		"total_setores":    totalSetores,
		"modo":             modoConf,
		"funcoes_filtro":   nomesFiltro,
	})
}

func (a *App) hConferenciaMarcar(w http.ResponseWriter, r *http.Request) {
	u := usuarioDoCtx(r)
	escopo, err := a.exigeEscopo(u)
	if err != nil {
		jsonErro(w, http.StatusForbidden, "conta sem grupo definido")
		return
	}
	var req struct {
		PessoaID   int64   `json:"pessoa_id"`
		Situacao   string  `json:"situacao"`
		DestinoID  *int64  `json:"destino_id"`
		Observacao *string `json:"observacao"`
		Verificado *bool   `json:"verificado"`
	}
	if err := decodificar(r, &req); err != nil || req.PessoaID == 0 {
		jsonErro(w, http.StatusBadRequest, "pessoa_id obrigatório")
		return
	}
	var confID int64
	var status string
	qMark := `SELECT id, status FROM conferencias
		WHERE status = 'aberta' AND grupo_id = ?`
	argsMark := []any{escopo}
	if idQ := r.URL.Query().Get("id"); idQ != "" {
		if cid, e := strconv.ParseInt(idQ, 10, 64); e == nil {
			qMark += ` AND id = ?`
			argsMark = append(argsMark, cid)
		}
	}
	qMark += ` ORDER BY id DESC LIMIT 1`
	err = a.st.db.QueryRow(qMark, argsMark...).Scan(&confID, &status)
	if err != nil {
		jsonErro(w, http.StatusConflict, "nenhuma conferência aberta")
		return
	}
	// pessoa precisa pertencer ao escopo
	var n int
	_ = a.st.db.QueryRow(`SELECT COUNT(*) FROM pessoas WHERE id = ? AND grupo_id = ? AND status='ativo'`,
		req.PessoaID, escopo).Scan(&n)
	if n == 0 {
		jsonErro(w, http.StatusForbidden, "pessoa fora do seu escopo")
		return
	}

	// Onda 05/10 (ordem Diretor): chefe_setor E operador só lançam no PRÓPRIO setor;
	// gerente e encarregado de pessoal lançam no grupo inteiro (mesma conferência).
	if !a.guardaSetorNaMarcar(w, u, req.PessoaID) {
		return
	}
	if req.Situacao == "" {
		// "desmarcar" o check (ordem Tenente 30/09): REMOVE a verificação — grava
		// verificado=0 no lançamento existente (antes o uncheck não persistia e o ✅
		// ressuscitava no reload); sem lançamento, nada a gravar.
		var jahExiste int
		_ = a.st.db.QueryRow(`SELECT COUNT(*) FROM presencas WHERE conferencia_id = ? AND pessoa_id = ?`,
			confID, req.PessoaID).Scan(&jahExiste)
		if jahExiste > 0 {
			_, _ = a.st.db.Exec(`UPDATE presencas SET verificado = 0, alterado_por = ?, alterado_em = strftime('%Y-%m-%dT%H:%M:%fZ','now')
				WHERE conferencia_id = ? AND pessoa_id = ?`, u.ID, confID, req.PessoaID)
		}
		jsonOK(w, map[string]any{"ok": true, "gravado": jahExiste > 0})
		return
	}
	switch req.Situacao {
	case "presente", "atraso", "falta", "justificada", "nao_verificado":
	default:
		jsonErro(w, http.StatusBadRequest, "situação inválida")
		return
	}
	if req.Situacao == "justificada" && req.DestinoID == nil {
		jsonErro(w, http.StatusBadRequest, "justificada exige destino")
		return
	}
	// v1.5: Estados de presente, falta e atraso não têm destino (destino zerado); somente justificada aceita destino.
	if req.Situacao != "justificada" {
		req.DestinoID = nil
	}

	// v1.5: Caso o estado mude em relação ao anterior, a observação é resetada (salvo nova observação explícita).
	var sitAnt, obsAnt string
	_ = a.st.db.QueryRow(`SELECT situacao, COALESCE(observacao,'') FROM presencas WHERE conferencia_id = ? AND pessoa_id = ?`,
		confID, req.PessoaID).Scan(&sitAnt, &obsAnt)
	if sitAnt != "" && sitAnt != req.Situacao && req.Observacao == nil {
		vazio := ""
		req.Observacao = &vazio
	}
	// v9.16.4: ✅ de verificação persiste separado (carry over inicia zerado)
	verificadoFlag := 0
	if req.Verificado != nil && *req.Verificado {
		verificadoFlag = 1
	}
	// v9.15.3: marcado_em em UTC REAL (exibição converte p/ Brasília)
	marcadoEm := time.Now().UTC().Format("2006-01-02T15:04:05.000Z")
	_, err = a.st.db.Exec(`INSERT INTO presencas (conferencia_id, pessoa_id, situacao, destino_id, observacao, marcado_por, marcado_em, verificado)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(conferencia_id, pessoa_id) DO UPDATE SET
		  situacao = excluded.situacao,
		  destino_id = excluded.destino_id,
		  observacao = excluded.observacao,
		  alterado_por = excluded.marcado_por,
		  alterado_em = excluded.marcado_em,
		  verificado = excluded.verificado`,
		confID, req.PessoaID, req.Situacao, req.DestinoID, req.Observacao, u.ID, marcadoEm, verificadoFlag)
	if err != nil {
		jsonErro(w, http.StatusInternalServerError, err.Error())
		return
	}

	// v1.5: Atualização automática da conferência setorial para 'em_andamento'
	var pSetorID *int64
	_ = a.st.db.QueryRow(`SELECT setor_id FROM pessoas WHERE id = ?`, req.PessoaID).Scan(&pSetorID)
	if pSetorID != nil && *pSetorID > 0 {
		_, _ = a.st.db.Exec(`
			INSERT INTO conferencia_setores (conferencia_id, setor_id, status)
			VALUES (?, ?, 'em_andamento')
			ON CONFLICT(conferencia_id, setor_id) DO UPDATE SET
			  status = 'em_andamento'
		`, confID, *pSetorID)
	}

	a.st.Auditoria(&u.ID, "marcar_parcial", "presencas", &confID,
		"pessoa "+fmt.Sprintf("%d", req.PessoaID)+" → "+req.Situacao, ipDe(r))
	// v1.5 SSE: notifica clientes conectados à conferência sobre a mudança
	a.confHub.Broadcast(confID, map[string]any{
		"tipo":       "marcar",
		"pessoa_id":  req.PessoaID,
		"situacao":   req.Situacao,
		"verificado": req.Verificado,
	})
	jsonOK(w, map[string]any{"ok": true, "gravado": true, "conferencia_id": confID})
}

func (a *App) hConferenciaIniciar(w http.ResponseWriter, r *http.Request) {
	a.ensureTabelaDespachos()
	var req confIniciarReq
	// fix PM5: JSON malformado não pode engolir silenciosamente (req ficava com
	// defaults e caía no fluxo "sem setores"); body vazio é tolerado (contrato legado).
	if err := decodificar(r, &req); err != nil && err.Error() != "EOF" {
		jsonErro(w, http.StatusBadRequest, "JSON inválido")
		return
	}

	u := usuarioDoCtx(r)
	if u.Papel == "admin" {
		jsonErro(w, http.StatusForbidden, "o admin não inicia conferências — quem inicia é o gerente/operador de um grupo")
		return
	}
	if u.GrupoID == nil {
		jsonErro(w, http.StatusForbidden, "conta sem grupo definido")
		return
	}

	temSetores := len(req.Setores) > 0
	if !temSetores {
		if u.Papel != "gerente" && !a.ehEncarregado(u) && !a.ehAuxiliarDePessoal(u) {
			jsonErro(w, http.StatusForbidden, "a conferência é iniciada pelo gerente ou pelo encarregado/auxiliar de pessoal")
			return
		}
	} else {
		if u.Papel == "chefe_setor" {
			for _, sid := range req.Setores {
				if !a.chefeComandaSetor(u, sid) {
					jsonErro(w, http.StatusForbidden, "você só pode iniciar conferência do seu próprio setor")
					return
				}
			}
		} else if u.Papel == "operador" {
			opSid := setorDoUsuario(a, u)
			if opSid == nil {
				jsonErro(w, http.StatusForbidden, "você só pode iniciar conferência do seu próprio setor")
				return
			}
			for _, sid := range req.Setores {
				if *opSid != sid {
					jsonErro(w, http.StatusForbidden, "você só pode iniciar conferência do seu próprio setor")
					return
				}
			}
		} else if u.Papel != "gerente" && !a.ehEncarregado(u) && !a.ehAuxiliarDePessoal(u) {
			jsonErro(w, http.StatusForbidden, "papel sem acesso a esta área")
			return
		}
	}

	var setoresUnicos []int64
	if temSetores {
		var err error
		setoresUnicos, err = a.validarSetoresDoGrupo(*u.GrupoID, req.Setores)
		if err != nil {
			jsonErro(w, http.StatusBadRequest, "setor inválido ou inexistente")
			return
		}
	}

	// onda 09/10: funcao_ids inválido → 400 ANTES de criar a conferência
	// (correção 09/10: só vale função de antiguidade DO GRUPO da conferência)
	if len(req.FuncaoIDs) > 0 {
		if err := a.validarFuncoesAntiguidadeDoGrupo(*u.GrupoID, req.FuncaoIDs); err != nil {
			jsonErro(w, http.StatusBadRequest, "posto/graduação inválido no filtro (precisa ser tag de antiguidade do grupo)")
			return
		}
	}

	id, err := a.criarConferenciaBase(u, req)
	if err != nil {
		jsonErro(w, http.StatusInternalServerError, err.Error())
		return
	}

	if temSetores {
		for _, sid := range setoresUnicos {
			_, _ = a.st.db.Exec(`INSERT OR IGNORE INTO conferencia_despachos (conferencia_id, setor_id, criado_por) VALUES (?, ?, ?)`, id, sid, u.ID)
		}
	}

	data := time.Now().In(a.horaLocal).Format("2006-01-02")
	a.st.Auditoria(&u.ID, "iniciar", "conferencias", &id, "data="+data+" (carry over e escalas aplicados)", ipDe(r))
	jsonOK(w, map[string]any{"id": id, "data": data, "conferencia_id": id})
}

func (a *App) hConferenciaFechar(w http.ResponseWriter, r *http.Request) {
	u := usuarioDoCtx(r)
	esc, err := a.exigeEscopo(u)
	if err != nil {
		jsonErro(w, http.StatusForbidden, "conta sem grupo definido")
		return
	}
	if u.Papel != "gerente" && !a.ehEncarregado(u) {
		jsonErro(w, http.StatusForbidden, "fechar a conferência é ato do gerente ou do encarregado de pessoal")
		return
	}
	var req struct {
		ID          int64           `json:"id"`
		Lancamentos []lancamentoReq `json:"lancamentos"`
	}
	if err := decodificar(r, &req); err != nil {
		jsonErro(w, http.StatusBadRequest, "JSON inválido")
		return
	}
	if req.ID == 0 || len(req.Lancamentos) == 0 {
		jsonErro(w, http.StatusBadRequest, "id e lançamentos obrigatórios")
		return
	}
	for _, l := range req.Lancamentos {
		switch l.Situacao {
		case "presente", "atraso", "falta", "justificada", "nao_verificado":
		default:
			jsonErro(w, http.StatusBadRequest, "situação inválida: "+l.Situacao)
			return
		}
		if l.Situacao == "justificada" && l.DestinoID == nil {
			jsonErro(w, http.StatusBadRequest, "justificada exige destino")
			return
		}
	}
	// REGRA (28/09): só quem pertence ao grupo da conferência a fecha
	if esc > 0 {
		var gid *int64
		qerr := a.st.db.QueryRow(`SELECT grupo_id FROM conferencias WHERE id = ?`, req.ID).Scan(&gid)
		if qerr != nil {
			jsonErro(w, http.StatusNotFound, "conferência inexistente")
			return
		}
		if gid == nil || *gid != esc {
			jsonErro(w, http.StatusForbidden, "conferência de outro grupo")
			return
		}
	}

	tx, err := a.st.db.Begin()
	if err != nil {
		jsonErro(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer tx.Rollback()
	// a conferência tem que estar ABERTA
	var status string
	if err = tx.QueryRow(`SELECT status FROM conferencias WHERE id = ?`, req.ID).Scan(&status); err != nil {
		jsonErro(w, http.StatusNotFound, "conferência inexistente")
		return
	}
	if status != "aberta" {
		jsonErro(w, http.StatusConflict, "conferência já está fechada")
		return
	}
	gravados := 0
	for _, l := range req.Lancamentos {
		// ordem Tenente 30/09: conferência NÃO fecha com membro "presente" implícito —
		// quem está sem ✅ (verificado=0 no corpo do front) grava NAO_VERIFICADO
		// (pune no % de presença como tudo que não é presente). Destino cai fora:
		// NÃO VERIFICADO não tem destino (lançamento efetivo não aconteceu).
		if !l.Verificado {
			l.Situacao = "nao_verificado"
			l.DestinoID = nil
		}
		_, err = tx.Exec(`
			INSERT INTO presencas (conferencia_id, pessoa_id, situacao, destino_id, tag_id, observacao, marcado_por)
			VALUES (?,?,?,?,?,?,?)
			ON CONFLICT (conferencia_id, pessoa_id) DO UPDATE SET
			  situacao = excluded.situacao,
			  destino_id = excluded.destino_id,
			  tag_id = excluded.tag_id,
			  observacao = excluded.observacao,
			  marcado_por = excluded.marcado_por,
			  alterado_por = excluded.marcado_por,
			  alterado_em = strftime('%Y-%m-%dT%H:%M:%fZ','now')`,
			req.ID, l.PessoaID, l.Situacao, l.DestinoID, l.TagID, l.Observacao, u.ID)
		if err != nil {
			jsonErro(w, http.StatusInternalServerError, "lançamento falhou (nada gravado): "+err.Error())
			return
		}
		gravados++
	}
	if _, err = tx.Exec(
		`UPDATE conferencias SET status = 'fechada', fechada_em = ? WHERE id = ?`,
		time.Now().UTC().Format("2006-01-02T15:04:05.000Z"), req.ID); err != nil {
		jsonErro(w, http.StatusInternalServerError, err.Error())
		return
	}
	_, _ = tx.Exec(`UPDATE conferencia_setores SET status = 'concluida' WHERE conferencia_id = ? AND status != 'concluida'`, req.ID)
	if err = tx.Commit(); err != nil {
		jsonErro(w, http.StatusInternalServerError, err.Error())
		return
	}
	a.st.Auditoria(&u.ID, "fechar", "conferencias", &req.ID,
		fmt.Sprintf("lancamentos=%d", gravados), ipDe(r))
	a.backupAssincrono("fechar") // zero-perda: cópia consistente a cada fechamento
	a.confHub.Broadcast(req.ID, map[string]any{
		"tipo":     "fechada",
		"gravados": gravados,
	})
	jsonOK(w, map[string]any{"conferencia_id": req.ID, "gravados": gravados})
}

func (a *App) hConferenciaSetorConcluir(w http.ResponseWriter, r *http.Request) {
	u := usuarioDoCtx(r)
	cid, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || cid <= 0 {
		jsonErro(w, http.StatusBadRequest, "id de conferência inválido")
		return
	}
	sid, err := strconv.ParseInt(r.PathValue("setor_id"), 10, 64)
	if err != nil || sid <= 0 {
		jsonErro(w, http.StatusBadRequest, "setor_id inválido")
		return
	}

	var confStatus string
	var confGrupoID int64
	if err := a.st.db.QueryRow(`SELECT status, COALESCE(grupo_id, 0) FROM conferencias WHERE id = ?`, cid).Scan(&confStatus, &confGrupoID); err != nil {
		jsonErro(w, http.StatusNotFound, "conferência não encontrada")
		return
	}
	if confStatus != "aberta" {
		jsonErro(w, http.StatusBadRequest, "conferência já está fechada")
		return
	}
	esc, err := a.exigeEscopo(u)
	if err != nil {
		jsonErro(w, http.StatusForbidden, "conta sem grupo definido")
		return
	}
	if esc > 0 && confGrupoID != esc {
		jsonErro(w, http.StatusForbidden, "conferência fora do seu escopo")
		return
	}

	if u.Papel == "chefe_setor" {
		// ordem 08/10 — contexto-govena: o chefe só conclui o setor ATIVO no
		// contexto da sessão (u.SetorID → fallback pessoa vinculada). A
		// multi-chefia passa a ser exercida TROCANDO o contexto no dropdown,
		// um setor por vez — sem cruzamento de dados entre setores.
		sAtivo := setorDoUsuario(a, u)
		if sAtivo == nil || *sAtivo != sid {
			jsonErro(w, http.StatusForbidden, "setor ativo no seu contexto é outro — troque a função no menu de contexto antes de concluir este setor")
			return
		}
	}

	concluidoEm := time.Now().UTC().Format("2006-01-02T15:04:05.000Z")
	_, err = a.st.db.Exec(`
		INSERT INTO conferencia_setores (conferencia_id, setor_id, status, concluido_por, concluido_em)
		VALUES (?, ?, 'concluida', ?, ?)
		ON CONFLICT(conferencia_id, setor_id) DO UPDATE SET
		  status = 'concluida',
		  concluido_por = excluded.concluido_por,
		  concluido_em = excluded.concluido_em
	`, cid, sid, u.ID, concluidoEm)
	if err != nil {
		jsonErro(w, http.StatusInternalServerError, err.Error())
		return
	}

	a.st.Auditoria(&u.ID, "concluir_setor", "conferencia_setores", &sid, fmt.Sprintf("conf_id=%d setor_id=%d", cid, sid), ipDe(r))
	jsonOK(w, map[string]any{"ok": true, "status": "concluida", "concluido_em": concluidoEm})
}

func (a *App) hConferenciaSetorReabrir(w http.ResponseWriter, r *http.Request) {
	u := usuarioDoCtx(r)
	cid, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || cid <= 0 {
		jsonErro(w, http.StatusBadRequest, "id de conferência inválido")
		return
	}
	sid, err := strconv.ParseInt(r.PathValue("setor_id"), 10, 64)
	if err != nil || sid <= 0 {
		jsonErro(w, http.StatusBadRequest, "setor_id inválido")
		return
	}

	var confStatus string
	var confGrupoID int64
	if err := a.st.db.QueryRow(`SELECT status, COALESCE(grupo_id, 0) FROM conferencias WHERE id = ?`, cid).Scan(&confStatus, &confGrupoID); err != nil {
		jsonErro(w, http.StatusNotFound, "conferência não encontrada")
		return
	}
	if confStatus != "aberta" {
		jsonErro(w, http.StatusBadRequest, "conferência já está fechada")
		return
	}
	esc, err := a.exigeEscopo(u)
	if err != nil {
		jsonErro(w, http.StatusForbidden, "conta sem grupo definido")
		return
	}
	if esc > 0 && confGrupoID != esc {
		jsonErro(w, http.StatusForbidden, "conferência fora do seu escopo")
		return
	}

	if u.Papel == "chefe_setor" {
		// ordem 08/10 — contexto-govena: só reabre o setor ATIVO no contexto
		// (mesma regra da conclusão; multi-chefia = trocar o contexto).
		sAtivo := setorDoUsuario(a, u)
		if sAtivo == nil || *sAtivo != sid {
			jsonErro(w, http.StatusForbidden, "setor ativo no seu contexto é outro — troque a função no menu de contexto antes de reabrir este setor")
			return
		}
	}

	_, err = a.st.db.Exec(`
		UPDATE conferencia_setores
		SET status = 'em_andamento', concluido_por = NULL, concluido_em = NULL
		WHERE conferencia_id = ? AND setor_id = ?
	`, cid, sid)
	if err != nil {
		jsonErro(w, http.StatusInternalServerError, err.Error())
		return
	}

	a.st.Auditoria(&u.ID, "reabrir_setor", "conferencia_setores", &sid, fmt.Sprintf("conf_id=%d setor_id=%d", cid, sid), ipDe(r))
	jsonOK(w, map[string]any{"ok": true, "status": "em_andamento"})
}

func (a *App) hConferenciaArquivar(w http.ResponseWriter, r *http.Request) {
	u := usuarioDoCtx(r)
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		jsonErro(w, http.StatusBadRequest, "id inválido")
		return
	}
	var gid *int64
	var status string
	if e := a.st.db.QueryRow(`SELECT grupo_id, status FROM conferencias WHERE id = ?`, id).Scan(&gid, &status); e != nil {
		jsonErro(w, http.StatusNotFound, "conferência inexistente")
		return
	}
	esc, err := a.exigeEscopo(u)
	if err != nil {
		jsonErro(w, http.StatusForbidden, "conta sem grupo definido")
		return
	}
	if esc > 0 {
		if gid == nil || *gid != esc {
			jsonErro(w, http.StatusForbidden, "conferência de outro grupo")
			return
		}
	} else if u.Papel != "admin" && u.Papel != "gerente" {
		jsonErro(w, http.StatusForbidden, "sem acesso")
		return
	}
	if status != "fechada" {
		jsonErro(w, http.StatusConflict, "só conferência FECHADA pode ser arquivada")
		return
	}
	if _, err := a.st.db.Exec(`UPDATE conferencias SET arquivada_em = strftime('%Y-%m-%dT%H:%M:%fZ','now') WHERE id = ?`, id); err != nil {
		jsonErro(w, http.StatusInternalServerError, err.Error())
		return
	}
	a.st.Auditoria(&u.ID, "arquivar", "conferencias", &id, "", ipDe(r))
	a.backupAssincrono("arquivar")
	jsonOK(w, map[string]any{"ok": true, "arquivada": id})
}

func (a *App) hConferenciaExcluirArquivada(w http.ResponseWriter, r *http.Request) {
	u := usuarioDoCtx(r) // a.auth(true) já garantiu papel admin
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		jsonErro(w, http.StatusBadRequest, "id inválido")
		return
	}
	var gid *int64
	var arq *string
	if e := a.st.db.QueryRow(`SELECT grupo_id, arquivada_em FROM conferencias WHERE id = ?`, id).Scan(&gid, &arq); e != nil {
		jsonErro(w, http.StatusNotFound, "conferência inexistente")
		return
	}
	if arq == nil {
		jsonErro(w, http.StatusConflict, "conferência não está arquivada — arquive antes de excluir")
		return
	}
	tx, err := a.st.db.Begin()
	if err != nil {
		jsonErro(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer tx.Rollback()
	for _, q := range []string{
		`DELETE FROM comentarios WHERE conferencia_id = ?`,
		`DELETE FROM presencas WHERE conferencia_id = ?`,
		`DELETE FROM conferencias WHERE id = ?`,
	} {
		if _, e := tx.Exec(q, id); e != nil {
			jsonErro(w, http.StatusInternalServerError, e.Error())
			return
		}
	}
	if err = tx.Commit(); err != nil {
		jsonErro(w, http.StatusInternalServerError, err.Error())
		return
	}
	a.st.Auditoria(&u.ID, "excluir_arquivada", "conferencias", &id, "", ipDe(r))
	a.backupAssincrono("excluir_arquivada")
	jsonOK(w, map[string]any{"ok": true, "excluida": id})
}

func (a *App) hConferenciaList(w http.ResponseWriter, r *http.Request) {
	u := usuarioDoCtx(r)
	escopo, err := a.exigeEscopo(u)
	if err != nil {
		jsonErro(w, http.StatusForbidden, "conta sem grupo definido")
		return
	}
	q := `
		SELECT c.id, c.data, COALESCE(c.hora,''), COALESCE(c.local,''), c.status,
		       COALESCE(NULLIF(u.nome_guerra,''), NULLIF(u.nome_completo,''), '—'), c.criado_em, c.fechada_em,
		       (SELECT COUNT(*) FROM presencas p WHERE p.conferencia_id = c.id) AS lanc,
		       COALESCE(c.grupo_id,0), COALESCE((SELECT g.nome FROM grupos g WHERE g.id = c.grupo_id),'—'),
		       c.arquivada_em,
		       COALESCE(c.nome,''), COALESCE(c.prazo_final,''),
		       COALESCE((SELECT COALESCE(NULLIF(u2.nome_completo,''), NULLIF(u2.nome_guerra,''), u2.login)
		                 FROM usuarios u2 WHERE u2.id = c.encarregado_usuario_id),'')
		FROM conferencias c
		LEFT JOIN usuarios u ON u.id = c.criado_por`
	var rows *sql.Rows
	// ordem Tenente 30/09: aba CONFERÊNCIAS tem filtro de período (padrão: últimos 7 dias);
	// arquivadas vêm SÓ na aba ARQUIVO (?arq=1, sem limite de período)
	if r.URL.Query().Get("arq") == "1" {
		if escopo > 0 {
			rows, err = a.st.db.Query(q+` WHERE c.grupo_id = ? AND c.arquivada_em IS NOT NULL
				ORDER BY c.data DESC, c.id DESC`, escopo)
		} else {
			rows, err = a.st.db.Query(q + ` WHERE c.arquivada_em IS NOT NULL
				ORDER BY c.data DESC, c.id DESC`)
		}
	} else {
		de, ate := a.periodoPadrao(r)
		if escopo > 0 {
			rows, err = a.st.db.Query(q+` WHERE c.grupo_id = ? AND c.arquivada_em IS NULL
				AND c.data BETWEEN ? AND ?
				ORDER BY c.data DESC, c.id DESC`, escopo, de, ate)
		} else {
			rows, err = a.st.db.Query(q+` WHERE c.arquivada_em IS NULL
				AND c.data BETWEEN ? AND ?
				ORDER BY c.data DESC, c.id DESC`, de, ate)
		}
	}
	if err != nil {
		jsonErro(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer rows.Close()
	out := []map[string]any{}
	for rows.Next() {
		var id, grupoID int64
		var data, hora, local, status, criado, criadaEm string
		var fechada *string
		var lanc int
		var grupoNome string
		var arquivada *string
		var nome, prazo, encarregado string
		if rows.Scan(&id, &data, &hora, &local, &status, &criado, &criadaEm, &fechada, &lanc, &grupoID, &grupoNome, &arquivada, &nome, &prazo, &encarregado) == nil {
			out = append(out, map[string]any{
				"id": id, "data": data, "hora": hora, "local": local, "status": status,
				"criado_por": criado, "criada_em": criadaEm, "fechada_em": fechada, "lancamentos": lanc,
				"grupo_id": grupoID, "grupo": grupoNome, "arquivada_em": arquivada,
				"nome": nome, "prazo_final": prazo, "encarregado_nome": encarregado,
			})
		}
	}
	jsonOK(w, out)
}

func (a *App) hConferenciaDescartar(w http.ResponseWriter, r *http.Request) {
	u := usuarioDoCtx(r)
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		jsonErro(w, http.StatusBadRequest, "id inválido")
		return
	}
	var gid *int64
	var status string
	if e := a.st.db.QueryRow(`SELECT grupo_id, status FROM conferencias WHERE id = ?`, id).Scan(&gid, &status); e != nil {
		jsonErro(w, http.StatusNotFound, "conferência inexistente")
		return
	}
	esc, err := a.exigeEscopo(u)
	if err != nil {
		jsonErro(w, http.StatusForbidden, "conta sem grupo definido")
		return
	}
	if esc > 0 {
		if gid == nil || *gid != esc {
			jsonErro(w, http.StatusForbidden, "conferência de outro grupo")
			return
		}
	}
	if status != "aberta" {
		jsonErro(w, http.StatusConflict, "conferência fechada é histórico — não pode ser descartada")
		return
	}
	tx, err := a.st.db.Begin()
	if err != nil {
		jsonErro(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer tx.Rollback()
	for _, q := range []string{
		`DELETE FROM comentarios WHERE conferencia_id = ?`,
		`DELETE FROM presencas WHERE conferencia_id = ?`,
		`DELETE FROM conferencia_escalas WHERE conferencia_id = ?`,
		`DELETE FROM conferencias WHERE id = ?`,
	} {
		if _, e := tx.Exec(q, id); e != nil {
			jsonErro(w, http.StatusInternalServerError, e.Error())
			return
		}
	}
	if err = tx.Commit(); err != nil {
		jsonErro(w, http.StatusInternalServerError, err.Error())
		return
	}
	a.st.Auditoria(&u.ID, "descartar", "conferencias", &id, "conferência aberta descartada", ipDe(r))
	jsonOK(w, map[string]any{"ok": true, "descartada": id})
}

func (a *App) hConferenciaGet(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		jsonErro(w, http.StatusBadRequest, "id inválido")
		return
	}
	var (
		data, status, criadaEm string
		hora, local            *string
		fechada                *string
		criadoPor              string
	)
	err = a.st.db.QueryRow(`
		SELECT c.data, c.status, c.criado_em, c.hora, c.local, c.fechada_em, COALESCE(u.login,'')
		FROM conferencias c LEFT JOIN usuarios u ON u.id = c.criado_por
		WHERE c.id = ?`, id).
		Scan(&data, &status, &criadaEm, &hora, &local, &fechada, &criadoPor)
	if errors.Is(err, sql.ErrNoRows) {
		jsonErro(w, http.StatusNotFound, "conferência inexistente")
		return
	}
	if err != nil {
		jsonErro(w, http.StatusInternalServerError, err.Error())
		return
	}
	// IDOR + HERANÇA (ordem Tenente 28/09 noite): grupo acessa a PRÓPRIA conferência;
	// superior acessa TAMBÉM as de subordinados com vínculo ativo (relatório fechado).
	uCtx := usuarioDoCtx(r)
	esc, err := a.exigeEscopo(uCtx)
	if err != nil {
		jsonErro(w, http.StatusForbidden, "conta sem grupo definido")
		return
	}
	if esc > 0 {
		var gid int64
		qerr := a.st.db.QueryRow(`SELECT COALESCE(grupo_id,0) FROM conferencias WHERE id = ?`, id).Scan(&gid)
		if qerr != nil || (gid != esc && !int64Contem(a.gruposSubordinadosAtivos(esc), gid)) {
			jsonErro(w, http.StatusForbidden, "conferência de outro grupo")
			return
		}
	}
	rows, e := a.st.db.Query(`
		SELECT p.nome_guerra, COALESCE(s.nome,'INDEFINIDO'), pr.situacao,
		       COALESCE(d.nome,''), COALESCE(pr.observacao,''), COALESCE(u.login,''), COALESCE(pr.marcado_em,''),
		       COALESCE(NULLIF(fu.nome,''), COALESCE(NULLIF(fu_u.nome,''), COALESCE(NULLIF(fu_up.nome,''), '—'))),
		       COALESCE(pr.verificado, 0)
		FROM presencas pr
		JOIN pessoas p ON p.id = pr.pessoa_id
		LEFT JOIN setores s ON s.id = p.setor_id
		LEFT JOIN funcoes fu ON fu.id = p.funcao_id
		LEFT JOIN usuarios u2 ON u2.pessoa_id = p.id
		LEFT JOIN funcoes fu_u ON fu_u.id = u2.funcao_id
		LEFT JOIN usuario_papeis up2 ON up2.usuario_id = u2.id AND (up2.grupo_id = p.grupo_id OR up2.grupo_id IS NULL)
		LEFT JOIN funcoes fu_up ON fu_up.id = up2.funcao_id
		LEFT JOIN destinos d ON d.id = pr.destino_id
		LEFT JOIN usuarios u ON u.id = pr.marcado_por
		WHERE pr.conferencia_id = ?
		ORDER BY pr.situacao, p.nome_guerra`, id)
	if e != nil {
		jsonErro(w, http.StatusInternalServerError, e.Error())
		return
	}
	defer rows.Close()
	lanc := []map[string]any{}
	cont := map[string]int{}
	for rows.Next() {
		var ng, setor, sit, destino, obs, por, em, funcao string
		var verificado int
		if rows.Scan(&ng, &setor, &sit, &destino, &obs, &por, &em, &funcao, &verificado) == nil {
			lanc = append(lanc, map[string]any{
				"nome_guerra": ng, "funcao": funcao, "setor": setor, "situacao": sit,
				"destino": destino, "observacao": obs, "marcado_por": por, "marcado_em": em,
				"verificado": verificado == 1,
			})
			cont[sit]++
		}
	}
	cMap := map[string]any{
		"id": id, "data": data, "status": status, "hora": hora, "local": local,
		"criada_em": criadaEm, "fechada_em": fechada, "criado_por": criadoPor,
	}
	res := map[string]any{
		"id": id, "data": data, "status": status, "hora": hora, "local": local,
		"criada_em": criadaEm, "fechada_em": fechada, "criado_por": criadoPor,
		"conferencia": cMap,
		"lancamentos": lanc,
		"presencas":   lanc,
		"escala":      a.escalaDaConferencia(id),
		"resumo": map[string]any{
			"presentes": cont["presente"], "atrasos": cont["atraso"],
			"faltas": cont["falta"], "justificadas": cont["justificada"],
		},
	}
	jsonOK(w, res)
}

func (a *App) montarLancamentosPDFConferencia(id int64, filtro string) ([]map[string]any, map[string]int, error) {
	rows, e := a.st.db.Query(`
		WITH RECURSIVE cam_setor(id, caminho) AS (
		  SELECT id, nome FROM setores WHERE pai_id IS NULL
		  UNION ALL
		  SELECT f.id, cs.caminho || ' > ' || f.nome FROM setores f JOIN cam_setor cs ON f.pai_id = cs.id
		),
		cam_funcao(id, caminho) AS (
		  SELECT id, nome FROM funcoes WHERE pai_id IS NULL
		  UNION ALL
		  SELECT f.id, cf.caminho || ' > ' || f.nome FROM funcoes f JOIN cam_funcao cf ON f.pai_id = cf.id
		)
		SELECT p.nome_guerra, COALESCE(NULLIF(s.sigla,''), s.nome, 'INDEFINIDO'), pr.situacao,
		       CASE WHEN pr.situacao = 'justificada' THEN COALESCE(d.nome,'') ELSE '' END AS destino, COALESCE(pr.observacao,''), u.login,
		       COALESCE(NULLIF(fu.nome,''), COALESCE(NULLIF(fu_u.nome,''), COALESCE(NULLIF(fu_up.nome,''), '—'))),
		       COALESCE(cf2.caminho,'~sem função'), COALESCE(cs2.caminho,'~sem setor')
		FROM presencas pr
		JOIN pessoas p ON p.id = pr.pessoa_id
		LEFT JOIN setores s ON s.id = p.setor_id
		LEFT JOIN cam_setor cs2 ON cs2.id = s.id
		LEFT JOIN funcoes fu ON fu.id = p.funcao_id
		LEFT JOIN cam_funcao cf2 ON cf2.id = fu.id
		LEFT JOIN usuarios u2 ON u2.pessoa_id = p.id
		LEFT JOIN funcoes fu_u ON fu_u.id = u2.funcao_id
		LEFT JOIN usuario_papeis up2 ON up2.usuario_id = u2.id AND (up2.grupo_id = p.grupo_id OR up2.grupo_id IS NULL)
		LEFT JOIN funcoes fu_up ON fu_up.id = up2.funcao_id
		LEFT JOIN destinos d ON d.id = pr.destino_id
		JOIN usuarios u ON u.id = pr.marcado_por
		WHERE pr.conferencia_id = ?
		ORDER BY COALESCE(cf2.caminho,'~sem função'), COALESCE(cs2.caminho,'~sem setor'), p.nome_guerra COLLATE NOCASE`, id)
	if e != nil {
		return nil, nil, e
	}
	defer rows.Close()
	lanc := []map[string]any{}
	resumo := map[string]int{"presentes": 0, "atrasos": 0, "faltas": 0, "justificadas": 0, "nao_verificados": 0}
	ord := 0
	for rows.Next() {
		var ng, setor, sit, destino, obs, por, funcao, camF, camS string
		if rows.Scan(&ng, &setor, &sit, &destino, &obs, &por, &funcao, &camF, &camS) == nil {
			switch sit {
			case "presente":
				resumo["presentes"]++
			case "atraso":
				resumo["atrasos"]++
			case "falta":
				resumo["faltas"]++
			case "nao_verificado":
				resumo["nao_verificados"]++
			case "justificada":
				resumo["justificadas"]++
			}
			if filtro == "faltas" && sit != "falta" {
				continue
			}
			if filtro == "justificados" && sit != "justificada" {
				continue
			}
			if filtro == "atrasos" && sit != "atraso" {
				continue
			}
			ord++
			lanc = append(lanc, map[string]any{
				"ord": ord, "nome_guerra": ng, "funcao": funcao, "setor": setor, "situacao": sit,
				"destino": destino, "observacao": obs,
			})
		}
	}
	return lanc, resumo, nil
}

func (a *App) hConferenciaPDF(w http.ResponseWriter, r *http.Request) {
	uCtx := usuarioDoCtx(r)
	esc, err := a.exigeEscopo(uCtx)
	if err != nil {
		jsonErro(w, http.StatusForbidden, "conta sem grupo definido")
		return
	}
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		jsonErro(w, http.StatusBadRequest, "id inválido")
		return
	}
	filtro := r.URL.Query().Get("filtro")
	if filtro != "" && filtro != "todos" && filtro != "faltas" && filtro != "justificados" && filtro != "atrasos" {
		jsonErro(w, http.StatusBadRequest, "filtro inválido (todos|faltas|justificados|atrasos)")
		return
	}
	var (
		data, status, criadaEm string
		fechada                *string
		criadoPor              string
	)
	err = a.st.db.QueryRow(`
		SELECT c.data, c.status, c.criado_em, c.fechada_em, COALESCE(u.login,'')
		FROM conferencias c LEFT JOIN usuarios u ON u.id = c.criado_por
		WHERE c.id = ?`, id).
		Scan(&data, &status, &criadaEm, &fechada, &criadoPor)
	if errors.Is(err, sql.ErrNoRows) {
		jsonErro(w, http.StatusNotFound, "conferência inexistente")
		return
	}
	if err != nil {
		jsonErro(w, http.StatusInternalServerError, err.Error())
		return
	}
	if status != "fechada" {
		jsonErro(w, http.StatusConflict, "só é possível gerar relatório de conferência FECHADA")
		return
	}
	// IDOR + HERANÇA (28/09 noite): superior gera o relatório FECHADO do subordinado.
	if esc > 0 {
		var gid int64
		qerr := a.st.db.QueryRow(`SELECT COALESCE(grupo_id,0) FROM conferencias WHERE id = ?`, id).Scan(&gid)
		if qerr != nil || (gid != esc && !int64Contem(a.gruposSubordinadosAtivos(esc), gid)) {
			jsonErro(w, http.StatusForbidden, "conferência de outro grupo")
			return
		}
	}
	lanc, resumo, e := a.montarLancamentosPDFConferencia(id, filtro)
	if e != nil {
		jsonErro(w, http.StatusInternalServerError, e.Error())
		return
	}
	u := usuarioDoCtx(r)
	selo := ""
	if filtro == "faltas" {
		selo = "_SO_FALTAS"
	} else if filtro == "justificados" {
		selo = "_SO_JUSTIFICADOS"
	} else if filtro == "atrasos" {
		selo = "_SO_ATRASOS"
	}
	var fechadoPorNome string
	_ = a.st.db.QueryRow(`
		SELECT COALESCE(NULLIF(u.nome_completo,''), NULLIF(u.nome_guerra,''), u.login)
		FROM conferencias c
		JOIN usuarios u ON u.id = COALESCE(c.fechada_por, c.criado_por)
		WHERE c.id = ?`, id).Scan(&fechadoPorNome)

	// "Iniciada por" também por NOME COMPLETO (ordem Diretor 04/10): o login
	// nunca aparece no PDF, nem no banner nem na assinatura.
	var criadoPorNome string
	_ = a.st.db.QueryRow(`
		SELECT COALESCE(NULLIF(u.nome_completo,''), NULLIF(u.nome_guerra,''), u.login)
		FROM conferencias c
		JOIN usuarios u ON u.id = c.criado_por
		WHERE c.id = ?`, id).Scan(&criadoPorNome)

	// Assinatura por NOME COMPLETO (ordem Diretor 04/10): o PDF nunca mostra o
	// login — quem assina é identificado pelo nome completo (fallback nome de
	// guerra) e, quando houver, a função específica do grupo logo abaixo.
	var quemAssina string
	_ = a.st.db.QueryRow(`
		SELECT COALESCE(NULLIF(u2.nome_completo,''), NULLIF(u2.nome_guerra,''), u2.login)
		FROM usuarios u2 WHERE u2.id = ?`, u.ID).Scan(&quemAssina)
	if strings.TrimSpace(quemAssina) == "" {
		quemAssina = u.Login
	}
	var funcaoAssina string
	_ = a.st.db.QueryRow(`
		SELECT f.nome FROM usuarios u2
		JOIN funcoes f ON f.id = u2.funcao_id AND f.grupo_id IS NOT DISTINCT FROM u2.grupo_id
		WHERE u2.id = ?`, u.ID).Scan(&funcaoAssina)

	cp := ConferenciaPDF{
		ID: id, Data: data, Status: status, CriadaEm: criadaEm, FechadaEm: fechada,
		CriadoPor: criadoPorNome, GeradoPor: quemAssina, Resumo: resumo, Lancamentos: lanc,
		Filtro: filtro, FechadoPorNome: fechadoPorNome, FuncaoGeradoPor: funcaoAssina,
	}
	pdf, err := a.gerarConferenciaPDF(cp)
	if err != nil {
		jsonErro(w, http.StatusInternalServerError, "falha ao gerar PDF: "+err.Error())
		return
	}
	a.st.Auditoria(&u.ID, "exportar", "conferencia", &id, "pdf", ipDe(r))
	w.Header().Set("Cache-Control", "no-store, no-cache, must-revalidate")
	w.Header().Set("Pragma", "no-cache")
	w.Header().Set("Content-Type", "application/pdf")
	w.Header().Set("Content-Disposition",
		fmt.Sprintf("inline; filename=SCI_conferencia_%s_%d%s.pdf", data, id, selo))
	_, _ = w.Write(pdf)
}

func (a *App) montarBundle(de, ate string, escopo int64) Bundle {
	b := Bundle{De: de, Ate: ate}
	// filtro de escopo (revisão TAKEDA/SHORYU): grupo só agrega o próprio grupo
	// + hierarquia (ordem 28/09 noite): superior agrega subordinados ativos
	filtro := ` WHERE f.status='fechada' AND f.data BETWEEN ? AND ?`
	args := []any{de, ate}
	if escopo > 0 {
		ft := a.filtroArvore(escopo, "f")
		filtro += ft.clause
		args = append(args, ft.args...)
	}
	_ = a.st.db.QueryRow(`
		WITH ultima_presenca AS (
		  SELECT p.situacao,
		         ROW_NUMBER() OVER(PARTITION BY p.pessoa_id ORDER BY f.data DESC, f.hora DESC, f.id DESC) as rn
		  FROM presencas p JOIN conferencias f ON f.id = p.conferencia_id`+filtro+`
		)
		SELECT COUNT(*),
		       COALESCE(SUM(situacao='presente'),0), COALESCE(SUM(situacao='atraso'),0),
		       COALESCE(SUM(situacao='falta'),0), COALESCE(SUM(situacao='justificada'),0),
		       COALESCE(SUM(situacao='nao_verificado'),0)
		FROM ultima_presenca WHERE rn = 1`,
		args...).Scan(&b.TotalLanc, &b.Presentes, &b.Atrasos, &b.Faltas, &b.Justificadas, &b.NaoVerificados)
	// "efetivo pronto" = presentes SEM ressalva (ordem do Tenente, 28/09)
	// "efetivo pronto" = presentes SEM ressalva (ordem do Tenente, 28/09)
	_ = a.st.db.QueryRow(`
		WITH ultima_presenca AS (
		  SELECT p.situacao, p.observacao,
		         ROW_NUMBER() OVER(PARTITION BY p.pessoa_id ORDER BY f.data DESC, f.hora DESC, f.id DESC) as rn
		  FROM presencas p JOIN conferencias f ON f.id = p.conferencia_id`+filtro+`
		)
		SELECT COALESCE(SUM(situacao='presente' AND (observacao IS NULL OR observacao = '')),0)
		FROM ultima_presenca WHERE rn = 1`,
		args...).Scan(&b.PresentesPuros)
	// FIX S4-P0 (verif5): conferencias precisa do MESMO alias "f" da cláusula de
	// árvore — sem alias, "no such column: f.grupo_id" era engolido pelo `_ =`
	// e convocacoes ficava 0 (zerando todos os % do relatório).
	qConv := `SELECT COUNT(*) FROM conferencias f WHERE f.status='fechada' AND f.data BETWEEN ? AND ?`
	qcArgs := append([]any{de, ate}, a.argsArvore(escopo)...)
	if escopo > 0 {
		qConv += a.clSetor(escopo)
	}
	// erros de contagem-base NUNCA silenciosos (lição v9): logar.
	if err := a.st.db.QueryRow(qConv, qcArgs...).Scan(&b.Convocacoes); err != nil {
		log.Printf("sci relatorio: convocacoes escopo=%d: %v", escopo, err)
	}
	// FIX S4-P1: efetivo ativo conta a ÁRVORE do escopo (mesma base dos lançamentos);
	// antes era só o grupo próprio → denominador misturava escopos.
	qEfetivo := `SELECT COUNT(*) FROM pessoas p WHERE p.status='ativo'`
	var efetArgs []any
	if escopo > 0 {
		ftP := a.filtroArvore(escopo, "p")
		qEfetivo += ftP.clause
		efetArgs = ftP.args
	}
	if err := a.st.db.QueryRow(qEfetivo, efetArgs...).Scan(&b.EfetivoAtivo); err != nil {
		log.Printf("sci relatorio: efetivo escopo=%d: %v", escopo, err)
	}
	// decisão Tenente 28/09: JUSTIFICADA = FALTA justificada → tudo que não é presente
	// pune o % de presença (exibição separa faltas justificadas x não justificadas)
	validas := b.Presentes + b.Atrasos
	denom := b.Convocacoes * b.EfetivoAtivo
	// FIX S4-P1: total_faltas é contagem de lançamentos — independe do denominador;
	// antes ficava 0 junto com os % quando denom=0 (falta real escondida).
	b.TotalFaltas = b.Faltas + b.Justificadas + b.NaoVerificados // ordem Tenente 30/09: NÃO VERIFICADO pune como falta
	if denom > 0 {
		b.PctGeral = round1(100 * float64(validas) / float64(denom))
		b.PctPronto = round1(100 * float64(b.PresentesPuros) / float64(denom))
		b.PctPresencaEstrita = round1(100 * float64(b.PresentesPuros) / float64(denom))
	}

	rows, err := a.st.db.Query(`
		WITH ultima_presenca AS (
		  SELECT pr.situacao, p.setor_id,
		         ROW_NUMBER() OVER(PARTITION BY pr.pessoa_id ORDER BY f.data DESC, f.hora DESC, f.id DESC) as rn
		  FROM presencas pr
		  JOIN pessoas p ON p.id = pr.pessoa_id
		  JOIN conferencias f ON f.id = pr.conferencia_id
		  WHERE f.status='fechada' AND f.data BETWEEN ? AND ?`+a.clSetor(escopo)+`
		)
		SELECT COALESCE(s.nome,'INDEFINIDO') AS setor_nome,
		       COALESCE(SUM(upr.situacao IN ('presente','atraso')),0) AS pres,
		       COALESCE(SUM(upr.situacao='falta'),0) AS faltas,
		       COALESCE(SUM(upr.situacao='justificada'),0) AS just,
		       COUNT(upr.situacao) AS lanc
		FROM ultima_presenca upr
		LEFT JOIN setores s ON s.id = upr.setor_id
		WHERE upr.rn = 1
		GROUP BY setor_nome ORDER BY pres DESC`, append([]any{de, ate}, a.argsArvore(escopo)...)...)
	if err == nil {
		for rows.Next() {
			var r SetorStat
			if rows.Scan(&r.Setor, &r.Presencas, &r.Faltas, &r.Justificadas, &r.Lancados) == nil {
				if r.Lancados > 0 {
					r.Pct = round1(100 * float64(r.Presencas) / float64(r.Lancados))
				}
				b.PorSetor = append(b.PorSetor, r)
			}
		}
		rows.Close()
	}

	rows, err = a.st.db.Query(`
		WITH ultima_presenca AS (
		  SELECT pr.situacao, pr.destino_id,
		         ROW_NUMBER() OVER(PARTITION BY pr.pessoa_id ORDER BY f.data DESC, f.hora DESC, f.id DESC) as rn
		  FROM presencas pr
		  JOIN conferencias f ON f.id = pr.conferencia_id
		  WHERE f.status='fechada' AND f.data BETWEEN ? AND ?`+a.clSetor(escopo)+`
		)
		SELECT COALESCE(d.nome,'(sem destino)'), COUNT(*)
		FROM ultima_presenca upr
		LEFT JOIN destinos d ON d.id = upr.destino_id
		WHERE upr.rn = 1 AND upr.situacao IN ('falta','justificada')
		GROUP BY d.id ORDER BY 2 DESC`, append([]any{de, ate}, a.argsArvore(escopo)...)...)
	if err == nil {
		for rows.Next() {
			var r DestinoStat
			if rows.Scan(&r.Destino, &r.Quantidade) == nil {
				b.PorDestino = append(b.PorDestino, r)
			}
		}
		rows.Close()
	}

	ftP2 := a.filtroArvore(escopo, "f2")
	// v9.7 (ordem Tenente): relatórios organizados por ANTIGUIDADE DE FUNÇÃO
	// (funcao_id menor = mais antigo), depois alfabetica. ID da função visível no relatório.
	// v9.11.1 (ordem Tenente 29/09): efetivo do relatório em ORDEM ALFABÉTICA (por hora).
	// v9.17 (ordem Tenente 29/09): TODOS os relatórios ordenam por HIERARQUIA DE SETOR
	// (árvore pai>filho, raiz antes, alfabético por nível), depois HIERARQUIA DE FUNÇÃO
	// (mesma regra), depois nome de guerra. Caminho construído com CTE recursiva.
	rows, err = a.st.db.Query(`
		WITH RECURSIVE cam_setor(id, caminho) AS (
		  SELECT id, nome FROM setores WHERE pai_id IS NULL
		  UNION ALL
		  SELECT f.id, cs.caminho || ' > ' || f.nome FROM setores f JOIN cam_setor cs ON f.pai_id = cs.id
		),
		cam_funcao(id, caminho) AS (
		  SELECT id, nome FROM funcoes WHERE pai_id IS NULL
		  UNION ALL
		  SELECT f.id, cf.caminho || ' > ' || f.nome FROM funcoes f JOIN cam_funcao cf ON f.pai_id = cf.id
		),
		ultima_presenca AS (
		  SELECT pr.pessoa_id, pr.situacao,
		         ROW_NUMBER() OVER(PARTITION BY pr.pessoa_id ORDER BY f.data DESC, f.hora DESC, f.id DESC) as rn
		  FROM presencas pr
		  JOIN conferencias f ON f.id = pr.conferencia_id
		  WHERE f.status='fechada' AND f.data BETWEEN ? AND ? `+ftP2.clause+`
		)
		SELECT p.id, p.nome_guerra, COALESCE(s.nome,'INDEFINIDO'),
		       CASE WHEN upr.situacao IN ('presente','atraso') THEN 1 ELSE 0 END,
		       CASE WHEN upr.situacao = 'atraso' THEN 1 ELSE 0 END,
		       CASE WHEN upr.situacao = 'falta' THEN 1 ELSE 0 END,
		       CASE WHEN upr.situacao = 'justificada' THEN 1 ELSE 0 END,
		       CASE WHEN upr.situacao = 'nao_verificado' THEN 1 ELSE 0 END,
		       CASE WHEN upr.situacao IS NOT NULL THEN 1 ELSE 0 END,
		       COALESCE(p.funcao_id, u2.funcao_id, up2.funcao_id),
		       COALESCE(NULLIF(fu.nome,''), COALESCE(NULLIF(fu_u.nome,''), COALESCE(NULLIF(fu_up.nome,''), ''))),
		       ROW_NUMBER() OVER (ORDER BY COALESCE(cf2.caminho,'~sem função'),
		                                  COALESCE(cs2.caminho,'~sem setor'),
		                                  p.nome_guerra COLLATE NOCASE) AS antig,
		       COALESCE(g2.nome,'—')
		FROM pessoas p
		LEFT JOIN setores s ON s.id = p.setor_id
		LEFT JOIN cam_setor cs2 ON cs2.id = s.id
		LEFT JOIN funcoes fu ON fu.id = p.funcao_id
		LEFT JOIN cam_funcao cf2 ON cf2.id = fu.id
		LEFT JOIN usuarios u2 ON u2.pessoa_id = p.id
		LEFT JOIN funcoes fu_u ON fu_u.id = u2.funcao_id
		LEFT JOIN usuario_papeis up2 ON up2.usuario_id = u2.id AND (up2.grupo_id = p.grupo_id OR up2.grupo_id IS NULL)
		LEFT JOIN funcoes fu_up ON fu_up.id = up2.funcao_id
		LEFT JOIN grupos g2 ON g2.id = p.grupo_id
		LEFT JOIN ultima_presenca upr ON upr.pessoa_id = p.id AND upr.rn = 1
		WHERE p.status='ativo'`+a.filtroArvore(escopo, "p").clause+`
		ORDER BY COALESCE(cf2.caminho,'~sem função'),
		         COALESCE(cs2.caminho,'~sem setor'),
		         p.nome_guerra COLLATE NOCASE`,
		append(append([]any{de, ate}, ftP2.args...), a.filtroArvore(escopo, "p").args...)...)
	if err == nil {
		for rows.Next() {
			var r PessoaStat
			var funcaoID *int64
			var antig int
			if rows.Scan(&r.ID, &r.NomeGuerra, &r.Setor, &r.Presencas, &r.Atrasos,
				&r.Faltas, &r.Justificadas, &r.NaoVerificados, &r.Lancados, &funcaoID, &r.Funcao, &antig, &r.Grupo) == nil {
				r.FuncaoID = funcaoID
				r.Antiguidade = antig
				if b.Convocacoes > 0 {
					r.Pct = round1(100 * float64(r.Presencas) / float64(b.Convocacoes))
				}
				b.Pessoas = append(b.Pessoas, r)
			}
		}
		rows.Close()
	}

	rows, err = a.st.db.Query(`
		SELECT f.data, COALESCE(ft.nome,''), COALESCE(f.hora,''), f.status,
		       COALESCE(SUM(pr.situacao IN ('presente','atraso')),0),
		       COALESCE(SUM(pr.situacao IN ('falta','nao_verificado')),0)
		FROM conferencias f
		JOIN conferencia_tipos ft ON ft.id = f.tipo_id
		LEFT JOIN presencas pr ON pr.conferencia_id = f.id
		WHERE f.data BETWEEN ? AND ?`+
		a.clSetor(escopo)+`
		GROUP BY f.id ORDER BY f.data DESC`, append([]any{de, ate}, a.argsArvore(escopo)...)...)
	if err == nil {
		for rows.Next() {
			var r FormaturaStat
			if rows.Scan(&r.Data, &r.Tipo, &r.Hora, &r.Status, &r.Presentes, &r.Faltas) == nil {
				b.Formaturas = append(b.Formaturas, r)
			}
		}
		rows.Close()
	}
	return b
}

func (a *App) hComentariosAdd(w http.ResponseWriter, r *http.Request) {
	u := usuarioDoCtx(r)
	esc, err := a.exigeEscopo(u)
	if err != nil {
		jsonErro(w, http.StatusForbidden, "conta sem grupo definido")
		return
	}
	var req struct {
		ConferenciaID int64  `json:"conferencia_id"`
		PessoaID      int64  `json:"pessoa_id"`
		Comentario    string `json:"comentario"`
		TagID         *int64 `json:"tag_id"` // ordem Tenente 30/09: TAGs do catálogo nos comentários
	}
	if err := decodificar(r, &req); err != nil || req.ConferenciaID == 0 || req.PessoaID == 0 ||
		strings.TrimSpace(req.Comentario) == "" {
		jsonErro(w, http.StatusBadRequest, "conferencia_id, pessoa_id e comentario obrigatórios")
		return
	}
	// IDOR (revisão TAKEDA/SHORYU): comentário só na conferência DO PRÓPRIO grupo
	if esc > 0 {
		var gid *int64
		if err := a.st.db.QueryRow(`SELECT grupo_id FROM conferencias WHERE id = ?`, req.ConferenciaID).Scan(&gid); err != nil || gid == nil || *gid != esc {
			jsonErro(w, http.StatusForbidden, "conferência de outro grupo")
			return
		}
	}
	// ordem incremental por conferência
	var ord int64
	_ = a.st.db.QueryRow(`SELECT COALESCE(MAX(ordem),0)+1 FROM comentarios WHERE conferencia_id = ?`,
		req.ConferenciaID).Scan(&ord)
	// TAG (ordem Tenente 30/09): se informada, precisa existir e ser visível no escopo
	if req.TagID != nil && *req.TagID > 0 {
		var n int
		var q2 string
		args2 := []any{*req.TagID}
		if esc > 0 {
			ids := append([]int64{esc}, a.gruposSuperioresAtivos(esc)...)
			marks := strings.TrimRight(strings.Repeat("?,", len(ids)), ",")
			q2 = `SELECT COUNT(*) FROM tags WHERE id = ? AND ativo = 1 AND (grupo_id IS NULL OR grupo_id IN (` + marks + `))`
			for _, v := range ids {
				args2 = append(args2, v)
			}
		} else {
			q2 = `SELECT COUNT(*) FROM tags WHERE id = ? AND ativo = 1`
		}
		_ = a.st.db.QueryRow(q2, args2...).Scan(&n)
		if n == 0 {
			jsonErro(w, http.StatusBadRequest, "tag inválida")
			return
		}
	}
	res, err := a.st.db.Exec(
		`INSERT INTO comentarios (ordem, conferencia_id, pessoa_id, operador_id, comentario, tag_id) VALUES (?,?,?,?,?,?)`,
		ord, req.ConferenciaID, req.PessoaID, u.ID, strings.TrimSpace(sanitizaRichText(req.Comentario)), req.TagID)
	if err != nil {
		jsonErro(w, http.StatusInternalServerError, err.Error())
		return
	}
	id, _ := res.LastInsertId()
	a.st.Auditoria(&u.ID, "comentar", "comentarios", &id,
		fmt.Sprintf("conf=%d pessoa=%d", req.ConferenciaID, req.PessoaID), ipDe(r))
	jsonOK(w, map[string]any{"id": id, "ordem": ord})
}

func (a *App) hComentariosList(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		jsonErro(w, http.StatusBadRequest, "id inválido")
		return
	}
	// IDOR: grupo só lista comentários da própria conferência
	u := usuarioDoCtx(r)
	esc, err := a.exigeEscopo(u)
	if err != nil {
		jsonErro(w, http.StatusForbidden, "conta sem grupo definido")
		return
	}
	if esc > 0 {
		var gid *int64
		if qerr := a.st.db.QueryRow(`SELECT grupo_id FROM conferencias WHERE id = ?`, id).Scan(&gid); qerr != nil || gid == nil || *gid != esc {
			jsonErro(w, http.StatusForbidden, "conferência de outro grupo")
			return
		}
	}
	rows, err := a.st.db.Query(`
		SELECT c.ordem, c.criado_em, p.nome_guerra, u.login, c.comentario, COALESCE(tt.nome,'')
		FROM comentarios c
		JOIN pessoas p ON p.id = c.pessoa_id
		JOIN usuarios u ON u.id = c.operador_id
		LEFT JOIN tags tt ON tt.id = c.tag_id
		WHERE c.conferencia_id = ? ORDER BY c.ordem`, id)
	if err != nil {
		jsonErro(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer rows.Close()
	out := []map[string]any{}
	for rows.Next() {
		var ord int64
		var em, nome, por, comentario, tag string
		if rows.Scan(&ord, &em, &nome, &por, &comentario, &tag) == nil {
			out = append(out, map[string]any{
				"ordem": ord, "datahora": em, "pessoa": nome,
				"operador": por, "comentario": comentario, "tag": tag,
			})
		}
	}
	jsonOK(w, out)
}

func (a *App) escaladosNaData(grupoID int64, data string) []map[string]any {
	if grupoID < 0 {
		return []map[string]any{}
	}
	q := `SELECT ep.pessoa_id, et.id, et.tipo_id, COALESCE(etp.nome, ''), COALESCE(ep.funcao_escala, ''), et.data_inicio, et.data_fim,
	             p.nome_guerra, p.nome_completo, COALESCE(s.nome, ''), COALESCE(f.nome, '')
	      FROM escala_pessoas ep
	      JOIN escala_turnos et ON et.id = ep.turno_id
	      JOIN escala_tipos etp ON etp.id = et.tipo_id
	      JOIN pessoas p ON p.id = ep.pessoa_id
	      LEFT JOIN setores s ON s.id = p.setor_id
	      LEFT JOIN funcoes f ON f.id = p.funcao_id
	      WHERE (? = 0 OR et.grupo_id = ?)
	        AND substr(et.data_inicio, 1, 10) <= ? AND substr(COALESCE(NULLIF(et.data_fim, ''), et.data_inicio), 1, 10) >= ?
	      ORDER BY et.id, p.nome_guerra`
	rows, err := a.st.db.Query(q, grupoID, grupoID, data, data)
	if err != nil {
		return []map[string]any{}
	}
	defer rows.Close()
	var res []map[string]any
	for rows.Next() {
		var pid, tid, tipoID int64
		var tipoNome, funcaoEscala, dtIni, dtFim, ng, nc, setor, funcao string
		if err := rows.Scan(&pid, &tid, &tipoID, &tipoNome, &funcaoEscala, &dtIni, &dtFim, &ng, &nc, &setor, &funcao); err == nil {
			res = append(res, map[string]any{
				"pessoa_id":     pid,
				"turno_id":      tid,
				"tipo_id":       tipoID,
				"tipo_nome":     tipoNome,
				"funcao_escala": funcaoEscala,
				"data_inicio":   dtIni,
				"data_fim":      dtFim,
				"nome_guerra":   ng,
				"nome_completo": nc,
				"setor":         setor,
				"funcao":        funcao,
			})
		}
	}
	return res
}

// ---------- rotas rotasConferencia ----------
func (a *App) rotasConferencia() {
	m := a.mux

	// abas de conferência/presença: GERENTE e OPERADOR apenas (R2/R11 — admin tem nav própria).
	// Onda 05/10 (ordem Diretor): + ENCARREGADO DE PESSOAL (função c/ "encarregado"
	// no nome, mesmo sem papel do sistema) em TODAS as áreas de conferência.
	confAuth := func(h http.HandlerFunc) http.Handler { return a.authConf(h) }
	confMarcarAuth := func(h http.HandlerFunc) http.Handler {
		return a.authConfCom([]string{"gerente", "operador", "chefe_setor"}, h)
	}

	// Arquivo de conferências + filtro de período (ordem Tenente 30/09):
	// FECHADAS × ARQUIVADAS; gerente arquiva, admin-only exclui arquivada.
	m.Handle("POST /api/conferencia/{id}/arquivar", confAuth(a.hConferenciaArquivar))
	m.Handle("DELETE /api/conferencia/arquivada/{id}", a.auth(true, a.hConferenciaExcluirArquivada))
	m.Handle("GET /api/conferencia/hoje", a.auth(false, a.hConferenciaHoje))
	m.Handle("GET /api/conferencia/estado", a.auth(false, a.hConferenciaEstado))
	m.Handle("POST /api/conferencia/iniciar", confMarcarAuth(a.hConferenciaIniciar))
	m.Handle("POST /api/conferencia/despachar", confAuth(a.hConferenciaDespachar))
	m.Handle("POST /api/conferencia/fechar", confAuth(a.hConferenciaFechar))
	m.Handle("POST /api/conferencia/marcar", confMarcarAuth(a.hConferenciaMarcar))
	m.Handle("GET /api/conferencia/lista", a.auth(false, a.hConferenciaList))
	m.Handle("GET /api/conferencia/{id}", a.auth(false, a.hConferenciaGet))
	m.Handle("DELETE /api/conferencia/{id}", confAuth(a.hConferenciaDescartar))
	m.Handle("POST /api/conferencia/{id}/setor/{setor_id}/concluir", confMarcarAuth(a.hConferenciaSetorConcluir))
	m.Handle("POST /api/conferencia/{id}/setor/{setor_id}/reabrir", confMarcarAuth(a.hConferenciaSetorReabrir))
	// onda 09/10: pré-fechamento — lista rápida do setor p/ conferência no modal antes do Despachar
	m.Handle("GET /api/conferencia/{id}/setor/{setor_id}/pre_fechamento", confMarcarAuth(a.hSetorPreFechamento))
	// correção 09/10: escada de antiguidade DO GRUPO p/ o picker do modal (sem seed global)
	m.Handle("GET /api/conferencia/funcoes-antiguidade", a.auth(false, a.hConferenciaFuncoesAntiguidade))
	m.Handle("GET /api/conferencia/{id}/relatorio.pdf", a.auth(false, a.hConferenciaPDF))

	// Escala de guarda (onda 05/10): gerente designa chefe/operador; chefe
	// designa operador do próprio setor; admin → 403 no handler (regra escopada).
	m.Handle("POST /api/conferencia/{id}/escala", a.auth(false, a.hConferenciaEscalaSet))
	m.Handle("DELETE /api/conferencia/{id}/escala/{usuario_id}", a.auth(false, a.hConferenciaEscalaDel))
	m.Handle("GET /api/conferencias", a.auth(false, a.hConferenciaList))
	m.Handle("POST /api/comentarios", confAuth(a.hComentariosAdd))
	m.Handle("GET /api/comentarios/{id}", confAuth(a.hComentariosList))
	m.Handle("GET /api/pessoas/{id}/comentarios", confAuth(a.hPessoaComentarios))

	// Conferência: stream SSE em tempo real (feat/v1.5-evolucao)
	m.Handle("GET /api/conferencia/{id}/stream", a.auth(false, a.hConferenciaStream))
}
