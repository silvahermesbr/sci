package main

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// ---------- Calendário Operacional & Mesh Institucional (Fase 3 v1.2) ----------

// checarAcessoEvento valida permissão de leitura ou edição sobre um evento.
func (a *App) checarAcessoEvento(u *Usuario, eventoID int64, precisaEdicao bool) (bool, map[string]any, error) {
	var titulo, tipo, cor, dataInicio, criadoEm string
	var descricao, dataFim *string
	var diaInteiro int
	var grupoID *int64
	var autorUsuarioID, autorPapelID int64

	err := a.st.db.QueryRow(`
		SELECT titulo, descricao, tipo, cor, data_inicio, data_fim, dia_inteiro,
		       grupo_id, autor_usuario_id, autor_papel_id, criado_em
		FROM calendario_eventos WHERE id = ?
	`, eventoID).Scan(&titulo, &descricao, &tipo, &cor, &dataInicio, &dataFim, &diaInteiro,
		&grupoID, &autorUsuarioID, &autorPapelID, &criadoEm)
	if err != nil {
		return false, nil, err
	}

	meta := map[string]any{
		"id":               eventoID,
		"titulo":           titulo,
		"descricao":        descricao,
		"tipo":             tipo,
		"cor":              cor,
		"data_inicio":      dataInicio,
		"data_fim":         dataFim,
		"dia_inteiro":      diaInteiro == 1,
		"grupo_id":         grupoID,
		"autor_usuario_id": autorUsuarioID,
		"autor_papel_id":   autorPapelID,
		"criado_em":        criadoEm,
	}

	if u.Papel == "admin" {
		return false, meta, fmt.Errorf("administrador não tem acesso a eventos de grupo")
	}
	if u.ID == autorUsuarioID {
		return true, meta, nil
	}

	// Se for evento corporativo geral (grupo_id IS NULL)
	if grupoID == nil {
		if !precisaEdicao {
			return true, meta, nil
		}
		return false, meta, nil
	}

	// Gerente da Unidade ou Unidade Superior
	if u.Papel == "gerente" && u.GrupoID != nil {
		if *grupoID == *u.GrupoID || int64Contem(a.gruposSubordinadosAtivos(*u.GrupoID), *grupoID) {
			return true, meta, nil
		}
	}

	// Compartilhamento Granular estilo Google Calendar
	var podeEditar int
	var qArgs []any
	q := `SELECT pode_editar FROM calendario_compartilhamentos WHERE evento_id = ? AND (`
	qArgs = append(qArgs, eventoID)
	conds := []string{"alvo_usuario_id = ?"}
	qArgs = append(qArgs, u.ID)
	if u.PapelAtivoID != nil {
		conds = append(conds, "alvo_papel_id = ?")
		qArgs = append(qArgs, *u.PapelAtivoID)
	}
	if u.GrupoID != nil {
		conds = append(conds, "alvo_grupo_id = ?")
		qArgs = append(qArgs, *u.GrupoID)
	}
	q += strings.Join(conds, " OR ") + `) ORDER BY pode_editar DESC LIMIT 1`

	errComp := a.st.db.QueryRow(q, qArgs...).Scan(&podeEditar)
	if errComp == nil {
		if !precisaEdicao || podeEditar == 1 {
			return true, meta, nil
		}
	}

	// Mesma Unidade: leitura por padrão
	if u.GrupoID != nil && *u.GrupoID == *grupoID {
		if !precisaEdicao {
			return true, meta, nil
		}
	}

	return false, meta, nil
}

// GET /api/calendario/visao?mes=YYYY-MM&inicio=YYYY-MM-DD&fim=YYYY-MM-DD
// Visão Unificada Mesh: Agrega Eventos, Escalas de Serviço e Prazos/Despachos
func (a *App) hCalendarioVisao(w http.ResponseWriter, r *http.Request) {
	u := usuarioDoCtx(r)
	if u.Papel == "admin" {
		jsonErro(w, http.StatusForbidden, "administrador não possui acesso ao calendário operacional")
		return
	}

	mes := r.URL.Query().Get("mes")
	inicio := r.URL.Query().Get("inicio")
	fim := r.URL.Query().Get("fim")

	if inicio == "" || fim == "" {
		if mes != "" {
			partes := strings.Split(mes, "-")
			if len(partes) == 2 {
				ano, _ := strconv.Atoi(partes[0])
				m, _ := strconv.Atoi(partes[1])
				if ano > 2000 && m >= 1 && m <= 12 {
					tIni := time.Date(ano, time.Month(m), 1, 0, 0, 0, 0, a.horaLocal)
					tFim := tIni.AddDate(0, 1, -1)
					inicio = tIni.Format("2006-01-02")
					fim = tFim.Format("2006-01-02")
				}
			}
		}
		if inicio == "" || fim == "" {
			agora := time.Now().In(a.horaLocal)
			tIni := time.Date(agora.Year(), agora.Month(), 1, 0, 0, 0, 0, a.horaLocal)
			tFim := tIni.AddDate(0, 1, -1)
			inicio = tIni.Format("2006-01-02")
			fim = tFim.Format("2006-01-02")
		}
	}

	// 1. EVENTOS DO CALENDÁRIO
	type EventoVisao struct {
		ID            int64   `json:"id"`
		Titulo        string  `json:"titulo"`
		Descricao     string  `json:"descricao"`
		Tipo          string  `json:"tipo"`
		Cor           string  `json:"cor"`
		DataInicio    string  `json:"data_inicio"`
		DataFim       *string `json:"data_fim"`
		DiaInteiro    bool    `json:"dia_inteiro"`
		GrupoID       *int64  `json:"grupo_id"`
		GrupoNome     string  `json:"grupo_nome"`
		AutorNome     string  `json:"autor_nome"`
		PodeEditar    bool    `json:"pode_editar"`
		Compartilhado bool    `json:"compartilhado"`
	}

	eventos := []EventoVisao{}

	// Condições de visibilidade do usuário
	var condVis []string
	var argsVis []any

	condVis = append(condVis, "ce.grupo_id IS NULL") // Globais
	condVis = append(condVis, "ce.autor_usuario_id = ?")
	argsVis = append(argsVis, u.ID)

	if u.GrupoID != nil {
		condVis = append(condVis, "ce.grupo_id = ?")
		argsVis = append(argsVis, *u.GrupoID)

		if u.Papel == "gerente" {
			subs := a.gruposSubordinadosAtivos(*u.GrupoID)
			if len(subs) > 0 {
				ph := strings.TrimSuffix(strings.Repeat("?,", len(subs)), ",")
				condVis = append(condVis, fmt.Sprintf("ce.grupo_id IN (%s)", ph))
				for _, s := range subs {
					argsVis = append(argsVis, s)
				}
			}
		}
	}

	// Compartilhados
	var condComp []string
	condComp = append(condComp, "cc.alvo_usuario_id = ?")
	argsVis = append(argsVis, u.ID)
	if u.PapelAtivoID != nil {
		condComp = append(condComp, "cc.alvo_papel_id = ?")
		argsVis = append(argsVis, *u.PapelAtivoID)
	}
	if u.GrupoID != nil {
		condComp = append(condComp, "cc.alvo_grupo_id = ?")
		argsVis = append(argsVis, *u.GrupoID)
	}
	condVis = append(condVis, fmt.Sprintf("ce.id IN (SELECT evento_id FROM calendario_compartilhamentos cc WHERE %s)", strings.Join(condComp, " OR ")))

	qEventos := fmt.Sprintf(`
		SELECT DISTINCT ce.id, ce.titulo, COALESCE(ce.descricao, ''), ce.tipo, ce.cor,
		       ce.data_inicio, ce.data_fim, ce.dia_inteiro, ce.grupo_id,
		       COALESCE(g.nome, 'Geral'), COALESCE(u.nome_guerra, u.login, '—'),
		       ce.autor_usuario_id
		FROM calendario_eventos ce
		LEFT JOIN grupos g ON g.id = ce.grupo_id
		LEFT JOIN usuarios u ON u.id = ce.autor_usuario_id
		WHERE (substr(ce.data_inicio, 1, 10) <= ? AND (ce.data_fim IS NULL OR substr(ce.data_fim, 1, 10) >= ?))
		  AND (%s)
		ORDER BY ce.data_inicio ASC
	`, strings.Join(condVis, " OR "))

	argsFinal := append([]any{fim, inicio}, argsVis...)
	evRows, err := a.st.db.Query(qEventos, argsFinal...)
	if err == nil {
		defer evRows.Close()
		for evRows.Next() {
			var ev EventoVisao
			var dFim *string
			var dInt int
			var autorUID int64
			_ = evRows.Scan(&ev.ID, &ev.Titulo, &ev.Descricao, &ev.Tipo, &ev.Cor,
				&ev.DataInicio, &dFim, &dInt, &ev.GrupoID, &ev.GrupoNome, &ev.AutorNome, &autorUID)
			ev.DataFim = dFim
			ev.DiaInteiro = dInt == 1
			ev.PodeEditar = (u.ID == autorUID)
			if !ev.PodeEditar && u.Papel == "gerente" && u.GrupoID != nil && ev.GrupoID != nil && *ev.GrupoID == *u.GrupoID {
				ev.PodeEditar = true
			}
			eventos = append(eventos, ev)
		}
	}

	// 2. ESCALAS DE SERVIÇO NO PERÍODO
	type EscalaMilitar struct {
		NomeGuerra string `json:"nome_guerra"`
		Funcao     string `json:"funcao"`
	}

	type EscalaVisao struct {
		TurnoID     int64           `json:"turno_id"`
		DataInicio  string          `json:"data_inicio"`
		DataFim     string          `json:"data_fim"`
		TipoNome    string          `json:"tipo_nome"`
		GrupoNome   string          `json:"grupo_nome"`
		Observacao  string          `json:"observacao"`
		Militares   []EscalaMilitar `json:"militares"`
		TotalEfetivo int            `json:"total_efetivo"`
	}

	escalas := []EscalaVisao{}

	var escGrupCond string
	var escGrupArgs []any
	if u.GrupoID != nil {
		if u.Papel == "gerente" {
			grupos := append([]int64{*u.GrupoID}, a.gruposSubordinadosAtivos(*u.GrupoID)...)
			ph := strings.TrimSuffix(strings.Repeat("?,", len(grupos)), ",")
			escGrupCond = fmt.Sprintf("et.grupo_id IN (%s)", ph)
			for _, g := range grupos {
				escGrupArgs = append(escGrupArgs, g)
			}
		} else {
			escGrupCond = "et.grupo_id = ?"
			escGrupArgs = append(escGrupArgs, *u.GrupoID)
		}
	} else {
		escGrupCond = "1=0"
	}

	qEscalas := fmt.Sprintf(`
		SELECT et.id, et.data_inicio, et.data_fim, etp.nome, COALESCE(g.nome, '—'), COALESCE(et.observacao, '')
		FROM escala_turnos et
		JOIN escala_tipos etp ON etp.id = et.tipo_id
		LEFT JOIN grupos g ON g.id = et.grupo_id
		WHERE (substr(et.data_inicio, 1, 10) <= ? AND substr(et.data_fim, 1, 10) >= ?)
		  AND (%s)
		ORDER BY et.data_inicio ASC
	`, escGrupCond)

	argsEscalasFinal := append([]any{fim, inicio}, escGrupArgs...)
	escRows, errEsc := a.st.db.Query(qEscalas, argsEscalasFinal...)
	if errEsc == nil {
		for escRows.Next() {
			var escItem EscalaVisao
			_ = escRows.Scan(&escItem.TurnoID, &escItem.DataInicio, &escItem.DataFim,
				&escItem.TipoNome, &escItem.GrupoNome, &escItem.Observacao)
			escalas = append(escalas, escItem)
		}
		escRows.Close()

		// Militares alocados em cada turno (após fechar escRows)
		for i := range escalas {
			pRows, errP := a.st.db.Query(`
				SELECT p.nome_guerra, COALESCE(ep.funcao_escala, 'Serviço')
				FROM escala_pessoas ep
				JOIN pessoas p ON p.id = ep.pessoa_id
				WHERE ep.turno_id = ?
				ORDER BY p.antiguidade ASC, p.nome_guerra ASC
			`, escalas[i].TurnoID)
			if errP == nil {
				for pRows.Next() {
					var m EscalaMilitar
					_ = pRows.Scan(&m.NomeGuerra, &m.Funcao)
					escalas[i].Militares = append(escalas[i].Militares, m)
				}
				pRows.Close()
			}
			escalas[i].TotalEfetivo = len(escalas[i].Militares)
		}
	}

	// 3. DESPACHOS & PRAZOS PENDENTES (Fase 2)
	type DespachoVisao struct {
		ID            int64  `json:"id"`
		Assunto       string `json:"assunto"`
		Data          string `json:"data"`
		Pendente      bool   `json:"pendente"`
		RemetenteNome string `json:"remetente_nome"`
	}

	despachos := []DespachoVisao{}
	if u.PapelAtivoID != nil {
		dRows, errD := a.st.db.Query(`
			SELECT m.id, m.assunto, substr(m.criada_em, 1, 10),
			       (md.respondido_em IS NULL), COALESCE(u.nome_guerra, u.login, '—')
			FROM mensagens m
			JOIN mensagem_destinatarios md ON md.mensagem_id = m.id
			LEFT JOIN usuarios u ON u.id = m.remetente_usuario_id
			WHERE md.destinatario_papel_id = ?
			  AND m.tipo = 'despacho' AND m.exige_resposta = 1
			  AND substr(m.criada_em, 1, 10) >= ? AND substr(m.criada_em, 1, 10) <= ?
			ORDER BY m.criada_em DESC
		`, *u.PapelAtivoID, inicio, fim)
		if errD == nil {
			defer dRows.Close()
			for dRows.Next() {
				var d DespachoVisao
				_ = dRows.Scan(&d.ID, &d.Assunto, &d.Data, &d.Pendente, &d.RemetenteNome)
				despachos = append(despachos, d)
			}
		}
	}

	jsonOK(w, map[string]any{
		"periodo": map[string]string{
			"inicio": inicio,
			"fim":    fim,
		},
		"eventos":   eventos,
		"escalas":   escalas,
		"despachos": despachos,
	})
}

// POST /api/calendario/eventos - Criar ou Atualizar Evento
func (a *App) hCalendarioEventosSave(w http.ResponseWriter, r *http.Request) {
	u := usuarioDoCtx(r)
	if u.Papel == "admin" {
		jsonErro(w, http.StatusForbidden, "administrador não possui acesso ao calendário operacional")
		return
	}

	var req struct {
		ID          int64   `json:"id"`
		Titulo      string  `json:"titulo"`
		Descricao   string  `json:"descricao"`
		Tipo        string  `json:"tipo"`
		Cor         string  `json:"cor"`
		DataInicio  string  `json:"data_inicio"`
		DataFim     *string `json:"data_fim"`
		DiaInteiro  bool    `json:"dia_inteiro"`
		GrupoID     *int64  `json:"grupo_id"`
	}
	if err := decodificar(r, &req); err != nil || strings.TrimSpace(req.Titulo) == "" || strings.TrimSpace(req.DataInicio) == "" {
		jsonErro(w, http.StatusBadRequest, "título e data_inicio são obrigatórios")
		return
	}

	req.Titulo = strings.TrimSpace(req.Titulo)
	if req.Tipo == "" {
		req.Tipo = "evento"
	}
	if req.Cor == "" {
		req.Cor = "#2563eb"
	}
	diaIntVal := 0
	if req.DiaInteiro {
		diaIntVal = 1
	}

	var grupoID *int64
	if u.GrupoID != nil {
		grupoID = u.GrupoID
	}

	if req.ID > 0 {
		// Atualizar evento existente
		ok, _, errAcesso := a.checarAcessoEvento(u, req.ID, true)
		if errAcesso != nil || !ok {
			jsonErro(w, http.StatusForbidden, "sem permissão para editar este evento")
			return
		}

		_, err := a.st.db.Exec(`
			UPDATE calendario_eventos 
			SET titulo = ?, descricao = ?, tipo = ?, cor = ?, data_inicio = ?, 
			    data_fim = ?, dia_inteiro = ?, grupo_id = ?
			WHERE id = ?
		`, req.Titulo, req.Descricao, req.Tipo, req.Cor, req.DataInicio, req.DataFim, diaIntVal, grupoID, req.ID)
		if err != nil {
			jsonErro(w, http.StatusInternalServerError, "falha ao atualizar evento: "+err.Error())
			return
		}

		a.st.Auditoria(&u.ID, "calendario_atualizar_evento", "calendario_eventos", &req.ID, req.Titulo, ipDe(r))
		jsonOK(w, map[string]any{"id": req.ID, "ok": true})
		return
	}

	// Criar novo evento
	res, err := a.st.db.Exec(`
		INSERT INTO calendario_eventos (titulo, descricao, tipo, cor, data_inicio, data_fim, dia_inteiro, grupo_id, autor_usuario_id, autor_papel_id)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, req.Titulo, req.Descricao, req.Tipo, req.Cor, req.DataInicio, req.DataFim, diaIntVal, grupoID, u.ID, u.PapelAtivoID)
	if err != nil {
		jsonErro(w, http.StatusInternalServerError, "falha ao criar evento: "+err.Error())
		return
	}
	id, _ := res.LastInsertId()

	a.st.Auditoria(&u.ID, "calendario_criar_evento", "calendario_eventos", &id, req.Titulo, ipDe(r))
	jsonOK(w, map[string]any{"id": id, "ok": true})
}

// DELETE /api/calendario/eventos/{id} - Excluir Evento
func (a *App) hCalendarioEventosDel(w http.ResponseWriter, r *http.Request) {
	u := usuarioDoCtx(r)
	if u.Papel == "admin" {
		jsonErro(w, http.StatusForbidden, "administrador não possui acesso ao calendário operacional")
		return
	}
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id <= 0 {
		jsonErro(w, http.StatusBadRequest, "id inválido")
		return
	}

	ok, meta, errAcesso := a.checarAcessoEvento(u, id, true)
	if errAcesso != nil || !ok {
		jsonErro(w, http.StatusForbidden, "sem permissão para excluir este evento")
		return
	}

	_, err = a.st.db.Exec(`DELETE FROM calendario_eventos WHERE id = ?`, id)
	if err != nil {
		jsonErro(w, http.StatusInternalServerError, "falha ao excluir evento: "+err.Error())
		return
	}

	a.st.Auditoria(&u.ID, "calendario_excluir_evento", "calendario_eventos", &id, meta["titulo"].(string), ipDe(r))
	jsonOK(w, map[string]any{"ok": true})
}

// POST /api/calendario/compartilhar - Conceder Permissão a Evento
func (a *App) hCalendarioCompartilhar(w http.ResponseWriter, r *http.Request) {
	u := usuarioDoCtx(r)
	if u.Papel == "admin" {
		jsonErro(w, http.StatusForbidden, "administrador não possui acesso ao calendário operacional")
		return
	}

	var req struct {
		EventoID   int64  `json:"evento_id"`
		AlvoTipo   string `json:"alvo_tipo"` // "grupo", "usuario", "papel"
		AlvoID     int64  `json:"alvo_id"`
		PodeEditar bool   `json:"pode_editar"`
	}
	if err := decodificar(r, &req); err != nil || req.EventoID <= 0 || req.AlvoID <= 0 {
		jsonErro(w, http.StatusBadRequest, "parâmetros de compartilhamento inválidos")
		return
	}

	ok, _, err := a.checarAcessoEvento(u, req.EventoID, true)
	if err != nil || !ok {
		jsonErro(w, http.StatusForbidden, "sem permissão para compartilhar este evento")
		return
	}

	var alvoGrupoID *int64
	var alvoUsuarioID *int64
	var alvoPapelID *int64

	switch req.AlvoTipo {
	case "grupo":
		alvoGrupoID = &req.AlvoID
	case "usuario":
		alvoUsuarioID = &req.AlvoID
	case "papel":
		alvoPapelID = &req.AlvoID
	default:
		jsonErro(w, http.StatusBadRequest, "alvo_tipo deve ser 'grupo', 'usuario' ou 'papel'")
		return
	}

	podeEdVal := 0
	if req.PodeEditar {
		podeEdVal = 1
	}

	res, err := a.st.db.Exec(`
		INSERT INTO calendario_compartilhamentos (evento_id, alvo_grupo_id, alvo_usuario_id, alvo_papel_id, pode_editar)
		VALUES (?, ?, ?, ?, ?)
	`, req.EventoID, alvoGrupoID, alvoUsuarioID, alvoPapelID, podeEdVal)
	if err != nil {
		jsonErro(w, http.StatusInternalServerError, "falha ao registrar compartilhamento: "+err.Error())
		return
	}
	compId, _ := res.LastInsertId()

	a.st.Auditoria(&u.ID, "calendario_compartilhar", "calendario_compartilhamentos", &compId,
		fmt.Sprintf("alvo=%s:%d evento=%d", req.AlvoTipo, req.AlvoID, req.EventoID), ipDe(r))

	jsonOK(w, map[string]any{"id": compId, "ok": true})
}

// GET /api/calendario/compartilhamentos?evento_id={id} - Listar Compartilhamentos
func (a *App) hCalendarioCompartilhamentosList(w http.ResponseWriter, r *http.Request) {
	u := usuarioDoCtx(r)
	if u.Papel == "admin" {
		jsonErro(w, http.StatusForbidden, "administrador não possui acesso ao calendário operacional")
		return
	}
	evStr := r.URL.Query().Get("evento_id")
	evID, _ := strconv.ParseInt(evStr, 10, 64)
	if evID <= 0 {
		jsonErro(w, http.StatusBadRequest, "evento_id é obrigatório")
		return
	}

	ok, _, err := a.checarAcessoEvento(u, evID, false)
	if err != nil || !ok {
		jsonErro(w, http.StatusForbidden, "sem permissão")
		return
	}

	type CompItem struct {
		ID         int64  `json:"id"`
		AlvoTipo   string `json:"alvo_tipo"`
		AlvoNome   string `json:"alvo_nome"`
		PodeEditar bool   `json:"pode_editar"`
		CriadoEm   string `json:"criado_em"`
	}

	var itens []CompItem
	rows, err := a.st.db.Query(`
		SELECT cc.id, cc.pode_editar, cc.criado_em,
		       CASE 
		         WHEN cc.alvo_grupo_id IS NOT NULL THEN 'grupo'
		         WHEN cc.alvo_usuario_id IS NOT NULL THEN 'usuario'
		         WHEN cc.alvo_papel_id IS NOT NULL THEN 'papel'
		         ELSE 'outro'
		       END as alvo_tipo,
		       COALESCE(g.nome, u.nome_guerra, u.login, up.papel, '—') as alvo_nome
		FROM calendario_compartilhamentos cc
		LEFT JOIN grupos g ON g.id = cc.alvo_grupo_id
		LEFT JOIN usuarios u ON u.id = cc.alvo_usuario_id
		LEFT JOIN usuario_papeis up ON up.id = cc.alvo_papel_id
		WHERE cc.evento_id = ?
	`, evID)
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var it CompItem
			var pEd int
			_ = rows.Scan(&it.ID, &pEd, &it.CriadoEm, &it.AlvoTipo, &it.AlvoNome)
			it.PodeEditar = pEd == 1
			itens = append(itens, it)
		}
	}

	jsonOK(w, map[string]any{"compartilhamentos": itens})
}

// DELETE /api/calendario/compartilhamentos/{id} - Revogar Compartilhamento
func (a *App) hCalendarioCompartilhamentosDel(w http.ResponseWriter, r *http.Request) {
	u := usuarioDoCtx(r)
	if u.Papel == "admin" {
		jsonErro(w, http.StatusForbidden, "administrador não possui acesso ao calendário operacional")
		return
	}
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id <= 0 {
		jsonErro(w, http.StatusBadRequest, "id inválido")
		return
	}

	var evID int64
	err = a.st.db.QueryRow(`SELECT evento_id FROM calendario_compartilhamentos WHERE id = ?`, id).Scan(&evID)
	if err != nil {
		jsonErro(w, http.StatusNotFound, "compartilhamento não encontrado")
		return
	}

	ok, _, errE := a.checarAcessoEvento(u, evID, true)
	if errE != nil || !ok {
		jsonErro(w, http.StatusForbidden, "sem permissão para revogar compartilhamento deste evento")
		return
	}

	_, err = a.st.db.Exec(`DELETE FROM calendario_compartilhamentos WHERE id = ?`, id)
	if err != nil {
		jsonErro(w, http.StatusInternalServerError, "falha ao revogar: "+err.Error())
		return
	}

	a.st.Auditoria(&u.ID, "calendario_revogar_compartilhamento", "calendario_compartilhamentos", &id, "", ipDe(r))
	jsonOK(w, map[string]any{"ok": true})
}
