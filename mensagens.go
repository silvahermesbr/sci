package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// ---------- Contexto de Sessão (Multi-Funções) ----------

func (a *App) hMudarContexto(w http.ResponseWriter, r *http.Request) {
	var req struct {
		PapelID int64 `json:"papel_id"`
	}
	if err := decodificar(r, &req); err != nil || req.PapelID <= 0 {
		jsonErro(w, http.StatusBadRequest, "papel_id inválido")
		return
	}
	u := usuarioDoCtx(r)
	c, err := r.Cookie(cookieSessao)
	if err != nil || c.Value == "" {
		jsonErro(w, http.StatusUnauthorized, "não autenticado")
		return
	}

	// Validar se o papel_id pertence ao usuário
	var donoID int64
	err = a.st.db.QueryRow(`SELECT usuario_id FROM usuario_papeis WHERE id = ?`, req.PapelID).Scan(&donoID)
	if err != nil || donoID != u.ID {
		jsonErro(w, http.StatusForbidden, "este papel não pertence a este usuário")
		return
	}

	// Atualizar a sessão ativa
	h := sha256.Sum256([]byte(c.Value))
	hash := hex.EncodeToString(h[:])
	_, err = a.st.db.Exec(`UPDATE sessoes SET papel_ativo_id = ? WHERE token_hash = ?`, req.PapelID, hash)
	if err != nil {
		jsonErro(w, http.StatusInternalServerError, "falha ao atualizar contexto da sessão")
		return
	}

	novoU, err := a.st.UsuarioDaSessao(c.Value)
	if err != nil || novoU == nil {
		jsonErro(w, http.StatusInternalServerError, "falha ao recarregar usuário")
		return
	}

	a.st.Auditoria(&u.ID, "trocar_contexto", "usuario_papeis", &req.PapelID,
		fmt.Sprintf("para papel=%s grupo=%v", novoU.Papel, novoU.GrupoID), ipDe(r))

	jsonOK(w, map[string]any{"usuario": novoU})
}

// ---------- Gestão de Papéis de Usuários ----------

func (a *App) hUsuarioPapelAdd(w http.ResponseWriter, r *http.Request) {
	usuarioID, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || usuarioID <= 0 {
		jsonErro(w, http.StatusBadRequest, "id de usuário inválido")
		return
	}

	var req struct {
		GrupoID      *int64 `json:"grupo_id"`
		Papel        string `json:"papel"`
		FuncaoID     *int64 `json:"funcao_id"`
		NomeExibicao string `json:"nome_exibicao"`
	}
	if err := decodificar(r, &req); err != nil {
		jsonErro(w, http.StatusBadRequest, "corpo da requisição inválido")
		return
	}

	u := usuarioDoCtx(r)
	papel := strings.ToLower(strings.TrimSpace(req.Papel))
	switch papel {
	case "admin", "gerente", "operador", "chefe_setor":
	default:
		jsonErro(w, http.StatusBadRequest, "papel inválido (admin | gerente | operador | chefe_setor)")
		return
	}

	// Permissões: Admin cria qualquer papel. Gerente só cria operador/chefe_setor no seu próprio grupo.
	if u.Papel == "operador" || u.Papel == "chefe_setor" {
		jsonErro(w, http.StatusForbidden, "operador ou chefe de setor não gerencia papéis")
		return
	}
	if u.Papel == "gerente" {
		if papel != "operador" && papel != "chefe_setor" {
			jsonErro(w, http.StatusForbidden, "gerente só pode atribuir papel de operador ou chefe de setor")
			return
		}
		if u.GrupoID == nil {
			jsonErro(w, http.StatusForbidden, "gerente sem grupo definido")
			return
		}
		req.GrupoID = u.GrupoID
	}
	if papel == "admin" {
		req.GrupoID = nil
	}

	// Regra de cardinalidade: cada grupo só pode ter UM gerente ativo
	if papel == "gerente" {
		if req.GrupoID == nil || *req.GrupoID <= 0 {
			jsonErro(w, http.StatusBadRequest, "gerente deve obrigatoriamente estar vinculado a um grupo")
			return
		}
		var count int
		_ = a.st.db.QueryRow(`SELECT COUNT(*) FROM usuario_papeis WHERE grupo_id = ? AND papel = 'gerente'`, *req.GrupoID).Scan(&count)
		if count > 0 {
			jsonErro(w, http.StatusBadRequest, "Este grupo já possui um Gerente cadastrado. Cada grupo só pode ter um Gerente.")
			return
		}
	}

	// Regra de funções: o usuário não precisa estar no banco de pessoal de um grupo inicialmente,
	// mas para ser colocado em uma função militar daquele grupo, ele deve fazer parte do banco de pessoal do grupo.
	if req.FuncaoID != nil && *req.FuncaoID > 0 {
		if req.GrupoID == nil || *req.GrupoID <= 0 {
			jsonErro(w, http.StatusBadRequest, "Para atribuição de função, é necessário vincular ao grupo correspondente")
			return
		}
		var pID *int64
		_ = a.st.db.QueryRow(`SELECT pessoa_id FROM usuarios WHERE id = ?`, usuarioID).Scan(&pID)
		if pID == nil || *pID <= 0 {
			jsonErro(w, http.StatusBadRequest, "Para ser colocado em uma função, o militar deve fazer parte do banco de pessoal deste grupo.")
			return
		}
		var pGrupoID *int64
		_ = a.st.db.QueryRow(`SELECT grupo_id FROM pessoas WHERE id = ?`, *pID).Scan(&pGrupoID)
		if pGrupoID == nil || *pGrupoID != *req.GrupoID {
			jsonErro(w, http.StatusBadRequest, "Para ser colocado em uma função neste grupo, o militar deve fazer parte do banco de pessoal deste grupo.")
			return
		}
	}

	res, err := a.st.db.Exec(`
		INSERT INTO usuario_papeis (usuario_id, grupo_id, papel, funcao_id, nome_exibicao)
		VALUES (?, ?, ?, ?, ?)`,
		usuarioID, req.GrupoID, papel, req.FuncaoID, strings.TrimSpace(req.NomeExibicao))
	if err != nil {
		jsonErro(w, http.StatusBadRequest, "não foi possível adicionar papel (já cadastrado?): "+err.Error())
		return
	}

	id, _ := res.LastInsertId()
	if req.FuncaoID != nil {
		var pID *int64
		_ = a.st.db.QueryRow(`SELECT pessoa_id FROM usuarios WHERE id = ?`, usuarioID).Scan(&pID)
		if pID != nil {
			_, _ = a.st.db.Exec(`UPDATE pessoas SET funcao_id = ? WHERE id = ?`, req.FuncaoID, *pID)
		}
		_, _ = a.st.db.Exec(`UPDATE usuarios SET funcao_id = ? WHERE id = ?`, req.FuncaoID, usuarioID)
	}
	a.st.Auditoria(&u.ID, "adicionar_papel", "usuario_papeis", &id,
		fmt.Sprintf("usuario_id=%d papel=%s grupo_id=%v", usuarioID, papel, req.GrupoID), ipDe(r))

	jsonOK(w, map[string]any{"ok": true, "id": id})
}

func (a *App) hUsuarioPapelDel(w http.ResponseWriter, r *http.Request) {
	usuarioID, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || usuarioID <= 0 {
		jsonErro(w, http.StatusBadRequest, "id de usuário inválido")
		return
	}
	papelID, err := strconv.ParseInt(r.PathValue("papel_id"), 10, 64)
	if err != nil || papelID <= 0 {
		jsonErro(w, http.StatusBadRequest, "id de papel inválido")
		return
	}

	u := usuarioDoCtx(r)
	if u.Papel == "operador" {
		jsonErro(w, http.StatusForbidden, "operador não gerencia papéis")
		return
	}

	// Não permitir remover o único papel do usuário
	var totalPapeis int
	_ = a.st.db.QueryRow(`SELECT COUNT(*) FROM usuario_papeis WHERE usuario_id = ?`, usuarioID).Scan(&totalPapeis)
	if totalPapeis <= 1 {
		jsonErro(w, http.StatusBadRequest, "não é possível remover o único papel do usuário")
		return
	}

	// Se o gerente está excluindo, garantir que o papel pertence ao seu grupo e é operador
	if u.Papel == "gerente" {
		var pGrupoID *int64
		var pPapel string
		err := a.st.db.QueryRow(`SELECT grupo_id, papel FROM usuario_papeis WHERE id = ? AND usuario_id = ?`, papelID, usuarioID).Scan(&pGrupoID, &pPapel)
		if err != nil || pPapel != "operador" || pGrupoID == nil || u.GrupoID == nil || *pGrupoID != *u.GrupoID {
			jsonErro(w, http.StatusForbidden, "permissão insuficiente para remover este papel")
			return
		}
	}

	// Se alguma sessão estava usando este papel, chavear para outro papel válido do usuário
	var outroPapelID int64
	_ = a.st.db.QueryRow(`SELECT id FROM usuario_papeis WHERE usuario_id = ? AND id != ? LIMIT 1`, usuarioID, papelID).Scan(&outroPapelID)
	if outroPapelID > 0 {
		_, _ = a.st.db.Exec(`UPDATE sessoes SET papel_ativo_id = ? WHERE papel_ativo_id = ?`, outroPapelID, papelID)
	}

	_, err = a.st.db.Exec(`DELETE FROM usuario_papeis WHERE id = ? AND usuario_id = ?`, papelID, usuarioID)
	if err != nil {
		jsonErro(w, http.StatusInternalServerError, "falha ao remover papel: "+err.Error())
		return
	}

	a.st.Auditoria(&u.ID, "remover_papel", "usuario_papeis", &papelID,
		fmt.Sprintf("usuario_id=%d papel_id=%d", usuarioID, papelID), ipDe(r))

	jsonOK(w, map[string]any{"ok": true})
}

// ---------- Módulo de Mensageria Interna, Despachos & Pastas (v1.2 Fase 2) ----------

func (a *App) hMensagensInbox(w http.ResponseWriter, r *http.Request) {
	u := usuarioDoCtx(r)
	if u.PapelAtivoID == nil || *u.PapelAtivoID <= 0 {
		jsonOK(w, []any{})
		return
	}

	soArquivadas := r.URL.Query().Get("arquivadas") == "1"
	pastaIDStr := r.URL.Query().Get("pasta_id")
	soDespachos := r.URL.Query().Get("despacho") == "1"

	var filtroExtra string
	var args []any
	args = append(args, *u.PapelAtivoID)

	if soArquivadas {
		filtroExtra += " AND md.arquivada = 1"
	} else {
		filtroExtra += " AND md.arquivada = 0"
		if pastaIDStr != "" {
			if pid, err := strconv.ParseInt(pastaIDStr, 10, 64); err == nil && pid > 0 {
				filtroExtra += " AND md.pasta_id = ?"
				args = append(args, pid)
			}
		}
	}

	if soDespachos {
		filtroExtra += " AND m.tipo = 'despacho'"
	}

	q := `
		SELECT m.id, m.assunto, m.corpo, m.criada_em,
		       COALESCE(m.tipo, 'comum'), COALESCE(m.exige_resposta, 0),
		       COALESCE(m.anexos, '[]'), m.pai_id,
		       md.id, md.lida_em, md.lida_por_usuario_id,
		       COALESCE(u_lida.nome_guerra, ''),
		       md.visualizado_em, md.respondido_em, md.pasta_id, md.arquivada,
		       m.remetente_papel_id, m.remetente_usuario_id,
		       u_rem.login, COALESCE(u_rem.nome_guerra, ''), COALESCE(u_rem.nome_completo, ''),
		       up_rem.papel, up_rem.grupo_id, COALESCE(g_rem.nome, ''),
		       up_rem.funcao_id, COALESCE(f_rem.nome, ''), COALESCE(up_rem.nome_exibicao, '')
		FROM mensagem_destinatarios md
		JOIN mensagens m ON m.id = md.mensagem_id
		JOIN usuario_papeis up_rem ON up_rem.id = m.remetente_papel_id
		JOIN usuarios u_rem ON u_rem.id = m.remetente_usuario_id
		LEFT JOIN grupos g_rem ON g_rem.id = up_rem.grupo_id
		LEFT JOIN funcoes f_rem ON f_rem.id = up_rem.funcao_id
		LEFT JOIN usuarios u_lida ON u_lida.id = md.lida_por_usuario_id
		WHERE md.destinatario_papel_id = ? AND md.excluida = 0` + filtroExtra + `
		ORDER BY m.id DESC LIMIT 150`

	rows, err := a.st.db.Query(q, args...)
	if err != nil {
		jsonErro(w, http.StatusInternalServerError, "falha ao buscar mensagens: "+err.Error())
		return
	}
	defer rows.Close()

	out := []map[string]any{}
	for rows.Next() {
		var msgID, destID, remPapelID, remUsuarioID int64
		var assunto, corpo, criadaEm, remLogin, remNomeGuerra, remNomeCompleto string
		var remPapel, remGrupoNome, remFuncaoNome, remNomeExibicao string
		var lidaPorGuerra, tipo, anexosJSON string
		var exigeResposta, arquivada int
		var lidaEm, visualizadoEm, respondidoEm *string
		var lidaPorID, remGrupoID, remFuncaoID, paiID, pastaID *int64

		if err := rows.Scan(
			&msgID, &assunto, &corpo, &criadaEm,
			&tipo, &exigeResposta, &anexosJSON, &paiID,
			&destID, &lidaEm, &lidaPorID, &lidaPorGuerra,
			&visualizadoEm, &respondidoEm, &pastaID, &arquivada,
			&remPapelID, &remUsuarioID,
			&remLogin, &remNomeGuerra, &remNomeCompleto,
			&remPapel, &remGrupoID, &remGrupoNome,
			&remFuncaoID, &remFuncaoNome, &remNomeExibicao,
		); err == nil {
			var anexosList []any
			_ = json.Unmarshal([]byte(anexosJSON), &anexosList)
			if anexosList == nil {
				anexosList = []any{}
			}

			item := map[string]any{
				"id":               msgID,
				"destinatario_id":  destID,
				"assunto":          assunto,
				"corpo":            corpo,
				"criada_em":        criadaEm,
				"tipo":             tipo,
				"exige_resposta":   exigeResposta == 1,
				"anexos":           anexosList,
				"pai_id":           paiID,
				"lida_em":          lidaEm,
				"lida_por_id":      lidaPorID,
				"lida_por_nome":    lidaPorGuerra,
				"visualizado_em":   visualizadoEm,
				"respondido_em":    respondidoEm,
				"pasta_id":         pastaID,
				"arquivada":        arquivada == 1,
				"remetente": map[string]any{
					"usuario_id":    remUsuarioID,
					"login":         remLogin,
					"nome_completo": remNomeCompleto,
					"nome_guerra":   remNomeGuerra,
					"papel":         remPapel,
					"grupo_id":      remGrupoID,
					"grupo_nome":    remGrupoNome,
					"funcao_id":     remFuncaoID,
					"funcao_nome":   remFuncaoNome,
					"nome_exibicao": remNomeExibicao,
				},
			}
			out = append(out, item)
		}
	}
	jsonOK(w, out)
}

func (a *App) hMensagensEnviadas(w http.ResponseWriter, r *http.Request) {
	u := usuarioDoCtx(r)
	if u.PapelAtivoID == nil || *u.PapelAtivoID <= 0 {
		jsonOK(w, []any{})
		return
	}

	rows, err := a.st.db.Query(`
		SELECT m.id, m.assunto, m.corpo, m.criada_em,
		       COALESCE(m.tipo, 'comum'), COALESCE(m.exige_resposta, 0),
		       COALESCE(m.anexos, '[]'), m.pai_id
		FROM mensagens m
		WHERE m.remetente_papel_id = ?
		ORDER BY m.id DESC LIMIT 100`, *u.PapelAtivoID)
	if err != nil {
		jsonErro(w, http.StatusInternalServerError, "falha ao buscar enviadas: "+err.Error())
		return
	}

	var msgs []map[string]any
	for rows.Next() {
		var id int64
		var assunto, corpo, criadaEm, tipo, anexosJSON string
		var exigeResposta int
		var paiID *int64
		if err := rows.Scan(&id, &assunto, &corpo, &criadaEm, &tipo, &exigeResposta, &anexosJSON, &paiID); err == nil {
			var anexosList []any
			_ = json.Unmarshal([]byte(anexosJSON), &anexosList)
			if anexosList == nil {
				anexosList = []any{}
			}

			msgs = append(msgs, map[string]any{
				"id":             id,
				"assunto":        assunto,
				"corpo":          corpo,
				"criada_em":      criadaEm,
				"tipo":           tipo,
				"exige_resposta": exigeResposta == 1,
				"anexos":         anexosList,
				"pai_id":         paiID,
			})
		}
	}
	rows.Close()

	out := []map[string]any{}
	for _, msg := range msgs {
		id := msg["id"].(int64)
		destRows, _ := a.st.db.Query(`
			SELECT md.destinatario_papel_id, up.papel, COALESCE(g.nome, ''),
			       COALESCE(f.nome, ''), md.lida_em, COALESCE(u_lida.nome_guerra, ''),
			       md.visualizado_em, md.respondido_em
			FROM mensagem_destinatarios md
			JOIN usuario_papeis up ON up.id = md.destinatario_papel_id
			LEFT JOIN grupos g ON g.id = up.grupo_id
			LEFT JOIN funcoes f ON f.id = up.funcao_id
			LEFT JOIN usuarios u_lida ON u_lida.id = md.lida_por_usuario_id
			WHERE md.mensagem_id = ?`, id)

		dests := []map[string]any{}
		if destRows != nil {
			for destRows.Next() {
				var papelID int64
				var papel, gNome, fNome, lidaPorGuerra string
				var lidaEm, visEm, respEm *string
				if err := destRows.Scan(&papelID, &papel, &gNome, &fNome, &lidaEm, &lidaPorGuerra, &visEm, &respEm); err == nil {
					dests = append(dests, map[string]any{
						"papel_id":       papelID,
						"papel":          papel,
						"grupo_nome":     gNome,
						"funcao_nome":    fNome,
						"lida_em":        lidaEm,
						"lida_por_nome":  lidaPorGuerra,
						"visualizado_em": visEm,
						"respondido_em":  respEm,
					})
				}
			}
			destRows.Close()
		}
		msg["destinatarios"] = dests
		out = append(out, msg)
	}
	jsonOK(w, out)
}

func (a *App) hMensagensEnviar(w http.ResponseWriter, r *http.Request) {
	u := usuarioDoCtx(r)
	if u.PapelAtivoID == nil || *u.PapelAtivoID <= 0 {
		jsonErro(w, http.StatusBadRequest, "usuário sem papel ativo na sessão")
		return
	}

	var req struct {
		DestinatarioPapelIDs []int64 `json:"destinatario_papel_ids"`
		Assunto              string  `json:"assunto"`
		Corpo                string  `json:"corpo"`
		Tipo                 string  `json:"tipo"`
		ExigeResposta        bool    `json:"exige_resposta"`
		PaiID                *int64  `json:"pai_id"`
		Anexos               any     `json:"anexos"`
	}
	if err := decodificar(r, &req); err != nil {
		jsonErro(w, http.StatusBadRequest, "dados inválidos")
		return
	}

	req.Assunto = strings.TrimSpace(req.Assunto)
	req.Corpo = strings.TrimSpace(req.Corpo)
	if req.Assunto == "" || req.Corpo == "" {
		jsonErro(w, http.StatusBadRequest, "assunto e mensagem são obrigatórios")
		return
	}
	if len(req.DestinatarioPapelIDs) == 0 {
		jsonErro(w, http.StatusBadRequest, "ao menos um destinatário deve ser selecionado")
		return
	}

	tipo := strings.ToLower(strings.TrimSpace(req.Tipo))
	if tipo != "despacho" {
		tipo = "comum"
	}

	exigeRespInt := 0
	if req.ExigeResposta || tipo == "despacho" {
		exigeRespInt = 1
	}

	anexosJSON := "[]"
	if req.Anexos != nil {
		if b, err := json.Marshal(req.Anexos); err == nil {
			anexosJSON = string(b)
		}
	}

	// Validação de Hierarquia de Envio:
	// Operador só pode enviar para o mesmo grupo, para seu gerente, ou responder a quem enviou (pai_id).
	if u.Papel == "operador" && u.GrupoID != nil {
		var permitidoPara []int64
		// Busca quem o operador pode enviar
		rowsP, _ := a.st.db.Query(`
			SELECT id FROM usuario_papeis
			WHERE grupo_id = ? OR papel = 'admin' OR (grupo_id = ? AND papel = 'gerente')`,
			*u.GrupoID, *u.GrupoID)
		if rowsP != nil {
			for rowsP.Next() {
				var pID int64
				if rowsP.Scan(&pID) == nil {
					permitidoPara = append(permitidoPara, pID)
				}
			}
			rowsP.Close()
		}

		// Se tem pai_id, também pode responder ao remetente da mensagem pai
		if req.PaiID != nil {
			var remetentePaiPapelID int64
			if a.st.db.QueryRow(`SELECT remetente_papel_id FROM mensagens WHERE id = ?`, *req.PaiID).Scan(&remetentePaiPapelID) == nil {
				permitidoPara = append(permitidoPara, remetentePaiPapelID)
			}
		}

		mapaPerm := map[int64]bool{}
		for _, pid := range permitidoPara {
			mapaPerm[pid] = true
		}

		for _, dID := range req.DestinatarioPapelIDs {
			if !mapaPerm[dID] {
				jsonErro(w, http.StatusForbidden, "operadores só podem enviar mensagens para membros da própria equipe ou gerência")
				return
			}
		}
	}

	res, err := a.st.db.Exec(`
		INSERT INTO mensagens (assunto, corpo, remetente_papel_id, remetente_usuario_id, tipo, exige_resposta, anexos, pai_id)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		req.Assunto, req.Corpo, *u.PapelAtivoID, u.ID, tipo, exigeRespInt, anexosJSON, req.PaiID)
	if err != nil {
		jsonErro(w, http.StatusInternalServerError, "falha ao salvar mensagem: "+err.Error())
		return
	}

	msgID, _ := res.LastInsertId()
	for _, destID := range req.DestinatarioPapelIDs {
		if destID <= 0 {
			continue
		}
		_, _ = a.st.db.Exec(`
			INSERT INTO mensagem_destinatarios (mensagem_id, destinatario_papel_id)
			VALUES (?, ?)`, msgID, destID)
	}

	// Se for resposta a um despacho que exigia resposta, baixa o despacho marcando respondido_em!
	if req.PaiID != nil {
		agora := time.Now().UTC().Format(time.RFC3339)
		_, _ = a.st.db.Exec(`
			UPDATE mensagem_destinatarios
			SET respondido_em = ?
			WHERE mensagem_id = ? AND destinatario_papel_id = ? AND respondido_em IS NULL`,
			agora, *req.PaiID, *u.PapelAtivoID)
	}

	a.st.Auditoria(&u.ID, "enviar_mensagem", "mensagens", &msgID,
		fmt.Sprintf("assunto=%s tipo=%s exige_resp=%d dests=%d", req.Assunto, tipo, exigeRespInt, len(req.DestinatarioPapelIDs)), ipDe(r))

	jsonOK(w, map[string]any{"ok": true, "id": msgID})
}

func (a *App) hMensagensMarcarLida(w http.ResponseWriter, r *http.Request) {
	u := usuarioDoCtx(r)
	if u.PapelAtivoID == nil || *u.PapelAtivoID <= 0 {
		jsonErro(w, http.StatusBadRequest, "usuário sem papel ativo")
		return
	}

	msgID, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || msgID <= 0 {
		jsonErro(w, http.StatusBadRequest, "id inválido")
		return
	}

	agora := time.Now().UTC().Format(time.RFC3339)
	_, err = a.st.db.Exec(`
		UPDATE mensagem_destinatarios
		SET lida_em = COALESCE(lida_em, ?),
		    visualizado_em = COALESCE(visualizado_em, ?),
		    lida_por_usuario_id = COALESCE(lida_por_usuario_id, ?)
		WHERE mensagem_id = ? AND destinatario_papel_id = ?`,
		agora, agora, u.ID, msgID, *u.PapelAtivoID)
	if err != nil {
		jsonErro(w, http.StatusInternalServerError, "falha ao marcar como lida: "+err.Error())
		return
	}

	jsonOK(w, map[string]any{"ok": true, "lida_em": agora, "visualizado_em": agora})
}

func (a *App) hMensagensArquivar(w http.ResponseWriter, r *http.Request) {
	u := usuarioDoCtx(r)
	if u.PapelAtivoID == nil || *u.PapelAtivoID <= 0 {
		jsonErro(w, http.StatusBadRequest, "usuário sem papel ativo")
		return
	}

	msgID, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || msgID <= 0 {
		jsonErro(w, http.StatusBadRequest, "id inválido")
		return
	}

	// Bloqueio de Despacho com resposta pendente
	var exigeResp int
	var respEm *string
	err = a.st.db.QueryRow(`
		SELECT COALESCE(m.exige_resposta,0), md.respondido_em
		FROM mensagem_destinatarios md
		JOIN mensagens m ON m.id = md.mensagem_id
		WHERE md.mensagem_id = ? AND md.destinatario_papel_id = ?`,
		msgID, *u.PapelAtivoID).Scan(&exigeResp, &respEm)
	if err != nil {
		jsonErro(w, http.StatusNotFound, "mensagem não encontrada")
		return
	}
	if exigeResp == 1 && respEm == nil {
		jsonErro(w, http.StatusBadRequest, "Despacho com resposta exigida. É obrigatório responder antes de arquivar.")
		return
	}

	_, err = a.st.db.Exec(`
		UPDATE mensagem_destinatarios
		SET arquivada = 1
		WHERE mensagem_id = ? AND destinatario_papel_id = ?`,
		msgID, *u.PapelAtivoID)
	if err != nil {
		jsonErro(w, http.StatusInternalServerError, "falha ao arquivar: "+err.Error())
		return
	}

	jsonOK(w, map[string]any{"ok": true})
}

func (a *App) hMensagensDesarquivar(w http.ResponseWriter, r *http.Request) {
	u := usuarioDoCtx(r)
	if u.PapelAtivoID == nil || *u.PapelAtivoID <= 0 {
		jsonErro(w, http.StatusBadRequest, "usuário sem papel ativo")
		return
	}

	msgID, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || msgID <= 0 {
		jsonErro(w, http.StatusBadRequest, "id inválido")
		return
	}

	_, err = a.st.db.Exec(`
		UPDATE mensagem_destinatarios
		SET arquivada = 0
		WHERE mensagem_id = ? AND destinatario_papel_id = ?`,
		msgID, *u.PapelAtivoID)
	if err != nil {
		jsonErro(w, http.StatusInternalServerError, "falha ao desarquivar: "+err.Error())
		return
	}

	jsonOK(w, map[string]any{"ok": true})
}

func (a *App) hMensagensExcluir(w http.ResponseWriter, r *http.Request) {
	u := usuarioDoCtx(r)
	if u.PapelAtivoID == nil || *u.PapelAtivoID <= 0 {
		jsonErro(w, http.StatusBadRequest, "usuário sem papel ativo")
		return
	}

	msgID, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || msgID <= 0 {
		jsonErro(w, http.StatusBadRequest, "id inválido")
		return
	}

	// Bloqueio de Despacho com resposta pendente
	var exigeResp int
	var respEm *string
	err = a.st.db.QueryRow(`
		SELECT COALESCE(m.exige_resposta,0), md.respondido_em
		FROM mensagem_destinatarios md
		JOIN mensagens m ON m.id = md.mensagem_id
		WHERE md.mensagem_id = ? AND md.destinatario_papel_id = ?`,
		msgID, *u.PapelAtivoID).Scan(&exigeResp, &respEm)
	if err == nil && exigeResp == 1 && respEm == nil {
		jsonErro(w, http.StatusBadRequest, "Despacho com resposta exigida. É obrigatório responder antes de excluir.")
		return
	}

	_, err = a.st.db.Exec(`
		UPDATE mensagem_destinatarios
		SET excluida = 1
		WHERE mensagem_id = ? AND destinatario_papel_id = ?`,
		msgID, *u.PapelAtivoID)
	if err != nil {
		jsonErro(w, http.StatusInternalServerError, "falha ao excluir mensagem: "+err.Error())
		return
	}

	jsonOK(w, map[string]any{"ok": true})
}

func (a *App) hMensagensPastasList(w http.ResponseWriter, r *http.Request) {
	u := usuarioDoCtx(r)
	rows, err := a.st.db.Query(`
		SELECT p.id, p.nome, p.criada_em, COUNT(md.id)
		FROM mensagem_pastas p
		LEFT JOIN mensagem_destinatarios md 
		  ON md.pasta_id = p.id 
		  AND md.destinatario_papel_id = ? 
		  AND md.excluida = 0
		WHERE p.usuario_id = ? 
		GROUP BY p.id, p.nome, p.criada_em
		ORDER BY p.nome`, u.PapelAtivoID, u.ID)
	if err != nil {
		jsonErro(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer rows.Close()

	out := []map[string]any{}
	for rows.Next() {
		var id int64
		var nome, em string
		var total int
		if rows.Scan(&id, &nome, &em, &total) == nil {
			out = append(out, map[string]any{
				"id":        id,
				"nome":      nome,
				"criada_em": em,
				"total":     total,
			})
		}
	}
	jsonOK(w, out)
}

func (a *App) hMensagensPastasAdd(w http.ResponseWriter, r *http.Request) {
	u := usuarioDoCtx(r)
	var req struct {
		Nome string `json:"nome"`
	}
	if err := decodificar(r, &req); err != nil || strings.TrimSpace(req.Nome) == "" {
		jsonErro(w, http.StatusBadRequest, "nome da pasta obrigatório")
		return
	}
	res, err := a.st.db.Exec(`INSERT INTO mensagem_pastas (usuario_id, nome) VALUES (?, ?)`, u.ID, strings.TrimSpace(req.Nome))
	if err != nil {
		jsonErro(w, http.StatusBadRequest, "pasta já existente ou erro: "+err.Error())
		return
	}
	id, _ := res.LastInsertId()
	jsonOK(w, map[string]any{"id": id, "nome": req.Nome})
}

func (a *App) hMensagensPastasDel(w http.ResponseWriter, r *http.Request) {
	u := usuarioDoCtx(r)
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id <= 0 {
		jsonErro(w, http.StatusBadRequest, "id inválido")
		return
	}
	// Limpa vínculo nas mensagens
	_, _ = a.st.db.Exec(`UPDATE mensagem_destinatarios SET pasta_id = NULL WHERE pasta_id = ?`, id)
	_, _ = a.st.db.Exec(`DELETE FROM mensagem_pastas WHERE id = ? AND usuario_id = ?`, id, u.ID)
	jsonOK(w, map[string]any{"ok": true})
}

func (a *App) hMensagensMoverPasta(w http.ResponseWriter, r *http.Request) {
	u := usuarioDoCtx(r)
	if u.PapelAtivoID == nil || *u.PapelAtivoID <= 0 {
		jsonErro(w, http.StatusBadRequest, "usuário sem papel ativo")
		return
	}
	msgID, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || msgID <= 0 {
		jsonErro(w, http.StatusBadRequest, "id inválido")
		return
	}
	var req struct {
		PastaID *int64 `json:"pasta_id"`
	}
	_ = decodificar(r, &req)

	_, err = a.st.db.Exec(`
		UPDATE mensagem_destinatarios
		SET pasta_id = ?, arquivada = 0
		WHERE mensagem_id = ? AND destinatario_papel_id = ?`,
		req.PastaID, msgID, *u.PapelAtivoID)
	if err != nil {
		jsonErro(w, http.StatusInternalServerError, "falha ao mover mensagem: "+err.Error())
		return
	}
	jsonOK(w, map[string]any{"ok": true})
}

func (a *App) hMensagensThread(w http.ResponseWriter, r *http.Request) {
	u := usuarioDoCtx(r)
	if u.PapelAtivoID == nil || *u.PapelAtivoID <= 0 {
		jsonErro(w, http.StatusBadRequest, "usuário sem papel ativo")
		return
	}
	msgID, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || msgID <= 0 {
		jsonErro(w, http.StatusBadRequest, "id inválido")
		return
	}

	// 1. Dados da mensagem raiz
	var mID, remPapelID, remUID int64
	var assunto, corpo, criadaEm, tipo, anexosJSON string
	var exigeResp int
	err = a.st.db.QueryRow(`
		SELECT id, assunto, corpo, criada_em, remetente_papel_id, remetente_usuario_id,
		       COALESCE(tipo, 'comum'), COALESCE(exige_resposta, 0), COALESCE(anexos, '[]')
		FROM mensagens WHERE id = ?`, msgID).Scan(
		&mID, &assunto, &corpo, &criadaEm, &remPapelID, &remUID,
		&tipo, &exigeResp, &anexosJSON)
	if err != nil {
		jsonErro(w, http.StatusNotFound, "mensagem não encontrada")
		return
	}

	// 2. Destinatários da mensagem
	dRows, err := a.st.db.Query(`
		SELECT md.id, md.destinatario_papel_id, up.usuario_id, u.login,
		       COALESCE(u.nome_guerra,''), COALESCE(u.nome_completo,''),
		       up.papel, up.grupo_id, COALESCE(g.nome,''),
		       up.funcao_id, COALESCE(f.nome,''),
		       md.lida_em, md.visualizado_em, md.respondido_em, md.arquivada
		FROM mensagem_destinatarios md
		JOIN usuario_papeis up ON up.id = md.destinatario_papel_id
		JOIN usuarios u ON u.id = up.usuario_id
		LEFT JOIN grupos g ON g.id = up.grupo_id
		LEFT JOIN funcoes f ON f.id = up.funcao_id
		WHERE md.mensagem_id = ? AND md.excluida = 0`, msgID)
	if err != nil {
		jsonErro(w, http.StatusInternalServerError, err.Error())
		return
	}

	destinatarios := []map[string]any{}
	ehDestinatario := false
	var meuRespondidoEm *string
	var meuLidaEm *string
	var meuVisualizadoEm *string
	minhaRespostaPendente := false

	for dRows.Next() {
		var mdID, destPapelID, destUID int64
		var dLogin, dGuerra, dCompleto, dPapel, dGNome, dFNome string
		var lidaEm, visEm, respEm *string
		var dGID, dFID *int64
		var arq int
		if dRows.Scan(&mdID, &destPapelID, &destUID, &dLogin, &dGuerra, &dCompleto,
			&dPapel, &dGID, &dGNome, &dFID, &dFNome,
			&lidaEm, &visEm, &respEm, &arq) == nil {

			if destPapelID == *u.PapelAtivoID {
				ehDestinatario = true
				meuRespondidoEm = respEm
				meuLidaEm = lidaEm
				meuVisualizadoEm = visEm
				if exigeResp == 1 && respEm == nil {
					minhaRespostaPendente = true
				}
			}

			destinatarios = append(destinatarios, map[string]any{
				"id":             mdID,
				"papel_id":       destPapelID,
				"usuario_id":     destUID,
				"login":          dLogin,
				"nome_guerra":    dGuerra,
				"nome_completo":  dCompleto,
				"papel":          dPapel,
				"grupo_id":       dGID,
				"grupo_nome":     dGNome,
				"funcao_id":      dFID,
				"funcao_nome":    dFNome,
				"lida_em":        lidaEm,
				"visualizado_em": visEm,
				"respondido_em":  respEm,
				"arquivada":      arq == 1,
			})
		}
	}
	dRows.Close()

	ehRemetente := (remPapelID == *u.PapelAtivoID)
	if !ehRemetente && !ehDestinatario {
		jsonErro(w, http.StatusForbidden, "acesso não autorizado a este despacho")
		return
	}

	// Se for destinatário e ainda não tinha visualizado, marca visualizado_em e lida_em agora!
	if ehDestinatario {
		agora := time.Now().UTC().Format(time.RFC3339)
		if meuVisualizadoEm == nil || meuLidaEm == nil {
			_, _ = a.st.db.Exec(`
				UPDATE mensagem_destinatarios
				SET visualizado_em = COALESCE(visualizado_em, ?),
				    lida_em = COALESCE(lida_em, ?),
				    lida_por_usuario_id = COALESCE(lida_por_usuario_id, ?)
				WHERE mensagem_id = ? AND destinatario_papel_id = ?`,
				agora, agora, u.ID, msgID, *u.PapelAtivoID)
			if meuVisualizadoEm == nil {
				meuVisualizadoEm = &agora
			}
			if meuLidaEm == nil {
				meuLidaEm = &agora
			}
		}
	}

	// 3. Remetente da mensagem raiz
	var remLogin, remGuerra, remCompleto, remPapel, remGNome, remFNome, remExibicao string
	var remGID, remFID *int64
	_ = a.st.db.QueryRow(`
		SELECT u.login, COALESCE(u.nome_guerra,''), COALESCE(u.nome_completo,''),
		       up.papel, up.grupo_id, COALESCE(g.nome,''),
		       up.funcao_id, COALESCE(f.nome,''), COALESCE(up.nome_exibicao,'')
		FROM usuario_papeis up
		JOIN usuarios u ON u.id = up.usuario_id
		LEFT JOIN grupos g ON g.id = up.grupo_id
		LEFT JOIN funcoes f ON f.id = up.funcao_id
		WHERE up.id = ?`, remPapelID).Scan(
		&remLogin, &remGuerra, &remCompleto,
		&remPapel, &remGID, &remGNome,
		&remFID, &remFNome, &remExibicao)

	var anexosList []any
	_ = json.Unmarshal([]byte(anexosJSON), &anexosList)
	if anexosList == nil {
		anexosList = []any{}
	}

	// 4. Posts / Respostas da Thread
	rRows, err := a.st.db.Query(`
		SELECT mr.id, mr.corpo, mr.anexos, mr.criada_em,
		       mr.remetente_papel_id, mr.remetente_usuario_id,
		       u.login, COALESCE(u.nome_guerra,''), COALESCE(u.nome_completo,''),
		       up.papel, up.grupo_id, COALESCE(g.nome,''),
		       up.funcao_id, COALESCE(f.nome,''), COALESCE(up.nome_exibicao,'')
		FROM mensagem_respostas mr
		JOIN usuario_papeis up ON up.id = mr.remetente_papel_id
		JOIN usuarios u ON u.id = mr.remetente_usuario_id
		LEFT JOIN grupos g ON g.id = up.grupo_id
		LEFT JOIN funcoes f ON f.id = up.funcao_id
		WHERE mr.mensagem_id = ?
		ORDER BY mr.id ASC`, msgID)

	respostas := []map[string]any{}
	if err == nil {
		for rRows.Next() {
			var respID, respPapelID, respUID int64
			var rCorpo, rAnxJSON, rCriadaEm, rLogin, rGuerra, rCompleto, rPapel, rGNome, rFNome, rExib string
			var rGID, rFID *int64
			if rRows.Scan(&respID, &rCorpo, &rAnxJSON, &rCriadaEm,
				&respPapelID, &respUID,
				&rLogin, &rGuerra, &rCompleto,
				&rPapel, &rGID, &rGNome,
				&rFID, &rFNome, &rExib) == nil {

				var rAnxList []any
				_ = json.Unmarshal([]byte(rAnxJSON), &rAnxList)
				if rAnxList == nil {
					rAnxList = []any{}
				}

				respostas = append(respostas, map[string]any{
					"id":        respID,
					"corpo":     rCorpo,
					"anexos":    rAnxList,
					"criada_em": rCriadaEm,
					"remetente": map[string]any{
						"papel_id":      respPapelID,
						"usuario_id":    respUID,
						"login":         rLogin,
						"nome_guerra":   rGuerra,
						"nome_completo": rCompleto,
						"papel":         rPapel,
						"grupo_id":      rGID,
						"grupo_nome":    rGNome,
						"funcao_id":     rFID,
						"funcao_nome":   rFNome,
						"nome_exibicao": rExib,
					},
				})
			}
		}
		rRows.Close()
	}

	jsonOK(w, map[string]any{
		"mensagem": map[string]any{
			"id":             mID,
			"assunto":        assunto,
			"corpo":          corpo,
			"criada_em":      criadaEm,
			"tipo":           tipo,
			"exige_resposta": exigeResp == 1,
			"anexos":         anexosList,
			"remetente": map[string]any{
				"papel_id":      remPapelID,
				"usuario_id":    remUID,
				"login":         remLogin,
				"nome_guerra":   remGuerra,
				"nome_completo": remCompleto,
				"papel":         remPapel,
				"grupo_id":      remGID,
				"grupo_nome":    remGNome,
				"funcao_id":     remFID,
				"funcao_nome":   remFNome,
				"nome_exibicao": remExibicao,
			},
		},
		"destinatarios":            destinatarios,
		"respostas":                respostas,
		"eh_remetente":             ehRemetente,
		"eh_destinatario":          ehDestinatario,
		"minha_resposta_pendente":  minhaRespostaPendente,
		"meu_visualizado_em":       meuVisualizadoEm,
		"meu_respondido_em":        meuRespondidoEm,
	})
}

func (a *App) hMensagensResponderThread(w http.ResponseWriter, r *http.Request) {
	u := usuarioDoCtx(r)
	if u.PapelAtivoID == nil || *u.PapelAtivoID <= 0 {
		jsonErro(w, http.StatusBadRequest, "usuário sem papel ativo")
		return
	}
	msgID, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || msgID <= 0 {
		jsonErro(w, http.StatusBadRequest, "id inválido")
		return
	}

	var req struct {
		Corpo  string `json:"corpo"`
		Anexos any    `json:"anexos"`
	}
	if err := decodificar(r, &req); err != nil || strings.TrimSpace(req.Corpo) == "" {
		jsonErro(w, http.StatusBadRequest, "o texto da resposta é obrigatório")
		return
	}

	// 1. Validar se a mensagem raiz existe e se o usuário tem acesso (é remetente ou destinatário)
	var remPapelID int64
	var exigeResp int
	err = a.st.db.QueryRow(`
		SELECT remetente_papel_id, COALESCE(exige_resposta, 0)
		FROM mensagens WHERE id = ?`, msgID).Scan(&remPapelID, &exigeResp)
	if err != nil {
		jsonErro(w, http.StatusNotFound, "mensagem não encontrada")
		return
	}

	ehRemetente := (remPapelID == *u.PapelAtivoID)

	var isDestinatario int
	var respEm *string
	_ = a.st.db.QueryRow(`
		SELECT 1, respondido_em
		FROM mensagem_destinatarios
		WHERE mensagem_id = ? AND destinatario_papel_id = ?`,
		msgID, *u.PapelAtivoID).Scan(&isDestinatario, &respEm)

	if !ehRemetente && isDestinatario != 1 {
		jsonErro(w, http.StatusForbidden, "apenas os participantes desta mensagem podem postar na thread")
		return
	}

	anexosJSON := "[]"
	if req.Anexos != nil {
		if b, err := json.Marshal(req.Anexos); err == nil {
			anexosJSON = string(b)
		}
	}

	// 2. Inserir resposta na thread
	res, err := a.st.db.Exec(`
		INSERT INTO mensagem_respostas (mensagem_id, remetente_usuario_id, remetente_papel_id, corpo, anexos)
		VALUES (?, ?, ?, ?, ?)`,
		msgID, u.ID, *u.PapelAtivoID, strings.TrimSpace(req.Corpo), anexosJSON)
	if err != nil {
		jsonErro(w, http.StatusInternalServerError, "falha ao registrar resposta: "+err.Error())
		return
	}
	respID, _ := res.LastInsertId()

	agora := time.Now().UTC().Format(time.RFC3339)

	// 3. REGRA CRÍTICA (Bugfix #3):
	// Apenas se o usuário atual for o DESTINATÁRIO do despacho é que seu respondido_em é marcado!
	// Se o remetente original postar na thread, ele adiciona o acompanhamento, mas o despacho do alvo NÃO é atendido!
	if isDestinatario == 1 {
		_, _ = a.st.db.Exec(`
			UPDATE mensagem_destinatarios
			SET respondido_em = ?,
			    visualizado_em = COALESCE(visualizado_em, ?),
			    lida_em = COALESCE(lida_em, ?)
			WHERE mensagem_id = ? AND destinatario_papel_id = ? AND respondido_em IS NULL`,
			agora, agora, agora, msgID, *u.PapelAtivoID)
	}

	a.st.Auditoria(&u.ID, "responder_thread", "mensagem_respostas", &respID,
		fmt.Sprintf("msg_id=%d is_dest=%d", msgID, isDestinatario), ipDe(r))

	jsonOK(w, map[string]any{
		"ok":                true,
		"id":                respID,
		"criada_em":         agora,
		"atendido":          isDestinatario == 1,
		"despacho_atendido": isDestinatario == 1,
	})
}

func (a *App) hMensagensContador(w http.ResponseWriter, r *http.Request) {
	u := usuarioDoCtx(r)
	if u.PapelAtivoID == nil || *u.PapelAtivoID <= 0 {
		jsonOK(w, map[string]any{"nao_lidas": 0, "despachos_pendentes": 0, "arquivadas": 0})
		return
	}

	var naoLidas, despachosPendentes, arquivadas int
	_ = a.st.db.QueryRow(`
		SELECT COUNT(*)
		FROM mensagem_destinatarios
		WHERE destinatario_papel_id = ? AND lida_em IS NULL AND excluida = 0 AND arquivada = 0`, *u.PapelAtivoID).Scan(&naoLidas)

	_ = a.st.db.QueryRow(`
		SELECT COUNT(*)
		FROM mensagem_destinatarios md
		JOIN mensagens m ON m.id = md.mensagem_id
		WHERE md.destinatario_papel_id = ? AND m.exige_resposta = 1 AND md.respondido_em IS NULL AND md.excluida = 0`, *u.PapelAtivoID).Scan(&despachosPendentes)

	_ = a.st.db.QueryRow(`
		SELECT COUNT(*)
		FROM mensagem_destinatarios
		WHERE destinatario_papel_id = ? AND arquivada = 1 AND excluida = 0`, *u.PapelAtivoID).Scan(&arquivadas)

	jsonOK(w, map[string]any{
		"nao_lidas":           naoLidas,
		"despachos_pendentes": despachosPendentes,
		"arquivadas":          arquivadas,
	})
}

func (a *App) hMensagensDestinatarios(w http.ResponseWriter, r *http.Request) {
	u := usuarioDoCtx(r)
	var papelAtivoID int64
	if u.PapelAtivoID != nil {
		papelAtivoID = *u.PapelAtivoID
	}

	rows, err := a.st.db.Query(`
		SELECT up.id, up.usuario_id, u.login, COALESCE(u.nome_guerra, ''), COALESCE(u.nome_completo, ''),
		       up.papel, up.grupo_id, COALESCE(NULLIF(g.nome, ''), 'Administração Geral'),
		       up.funcao_id, COALESCE(NULLIF(f.nome, ''), CASE WHEN up.papel = 'admin' THEN 'Administrador do Sistema' ELSE '' END),
		       COALESCE(NULLIF(up.nome_exibicao, ''), CASE WHEN up.papel = 'admin' THEN 'Administração do Sistema' ELSE '' END)
		FROM usuario_papeis up
		JOIN usuarios u ON u.id = up.usuario_id
		LEFT JOIN grupos g ON g.id = up.grupo_id
		LEFT JOIN funcoes f ON f.id = up.funcao_id
		WHERE u.ativo = 1 AND up.id != ?
		ORDER BY up.papel = 'admin' DESC, g.nome ASC, up.papel ASC`, papelAtivoID)
	if err != nil {
		jsonErro(w, http.StatusInternalServerError, "falha ao listar destinatários: "+err.Error())
		return
	}
	defer rows.Close()

	out := []map[string]any{}
	for rows.Next() {
		var id, usuarioID int64
		var login, nomeGuerra, nomeCompleto, papel, gNome, fNome, exibicao string
		var gID, fID *int64
		if err := rows.Scan(&id, &usuarioID, &login, &nomeGuerra, &nomeCompleto,
			&papel, &gID, &gNome, &fID, &fNome, &exibicao); err == nil {
			out = append(out, map[string]any{
				"id":            id,
				"usuario_id":    usuarioID,
				"login":         login,
				"nome_guerra":   nomeGuerra,
				"nome_completo": nomeCompleto,
				"papel":         papel,
				"grupo_id":      gID,
				"grupo_nome":    gNome,
				"funcao_id":     fID,
				"funcao_nome":   fNome,
				"nome_exibicao": exibicao,
			})
		}
	}
	jsonOK(w, out)
}

// ---------- Módulo de Fórum & Avisos Gerenciais (v1.2 Fase 2) ----------

func (a *App) hAvisosList(w http.ResponseWriter, r *http.Request) {
	u := usuarioDoCtx(r)
	if u.Papel == "admin" {
		jsonErro(w, http.StatusForbidden, "administrador não possui acesso aos avisos operacionais")
		return
	}
	escopo := escopoDoUsuario(u)

	var filtroGrupo string
	var args []any

	if escopo > 0 {
		filtroGrupo = "WHERE a.grupo_id = ? OR a.grupo_id IN (SELECT id FROM grupos WHERE id = ?)"
		args = append(args, escopo, escopo)
	}

	q := `
		SELECT a.id, a.titulo, a.conteudo, a.fixado, a.criado_em,
		       a.autor_usuario_id, u.login, COALESCE(u.nome_guerra,''), COALESCE(u.nome_completo,''),
		       a.autor_papel_id, up.papel, COALESCE(up.nome_exibicao,''),
		       a.grupo_id, COALESCE(g.nome,''),
		       a.grupo_origem_id, COALESCE(g_orig.nome,''), a.aviso_origem_id,
		       (SELECT COUNT(*) FROM aviso_cientes ac WHERE ac.aviso_id = a.id) as total_cientes,
		       (SELECT COUNT(*) FROM aviso_comentarios comm WHERE comm.aviso_id = a.id) as total_comentarios,
		       (SELECT COUNT(*) FROM aviso_cientes ac_me WHERE ac_me.aviso_id = a.id AND ac_me.usuario_id = ?) as meu_ciente
		FROM avisos a
		JOIN usuarios u ON u.id = a.autor_usuario_id
		JOIN usuario_papeis up ON up.id = a.autor_papel_id
		JOIN grupos g ON g.id = a.grupo_id
		LEFT JOIN grupos g_orig ON g_orig.id = a.grupo_origem_id
		` + filtroGrupo + `
		ORDER BY a.fixado DESC, a.id DESC LIMIT 100`

	qArgs := append([]any{u.ID}, args...)
	rows, err := a.st.db.Query(q, qArgs...)
	if err != nil {
		jsonErro(w, http.StatusInternalServerError, "falha ao listar avisos: "+err.Error())
		return
	}
	defer rows.Close()

	out := []map[string]any{}
	for rows.Next() {
		var id, autorUID, autorPID, grupoID int64
		var titulo, conteudo, criadoEm, login, guerra, completo, papel, exibicao, gNome string
		var gOrigNome string
		var fixado, totalCientes, totalComentarios, meuCiente int
		var gOrigID, avisoOrigID *int64

		if err := rows.Scan(
			&id, &titulo, &conteudo, &fixado, &criadoEm,
			&autorUID, &login, &guerra, &completo,
			&autorPID, &papel, &exibicao,
			&grupoID, &gNome,
			&gOrigID, &gOrigNome, &avisoOrigID,
			&totalCientes, &totalComentarios, &meuCiente,
		); err == nil {
			out = append(out, map[string]any{
				"id":                id,
				"titulo":            titulo,
				"conteudo":          conteudo,
				"fixado":            fixado == 1,
				"criado_em":         criadoEm,
				"total_cientes":     totalCientes,
				"total_comentarios": totalComentarios,
				"meu_ciente":        meuCiente > 0,
				"grupo_id":          grupoID,
				"grupo_nome":        gNome,
				"origem": map[string]any{
					"grupo_id":   gOrigID,
					"grupo_nome": gOrigNome,
					"aviso_id":   avisoOrigID,
				},
				"autor": map[string]any{
					"usuario_id":    autorUID,
					"login":         login,
					"nome_guerra":   guerra,
					"nome_completo": completo,
					"papel":         papel,
					"nome_exibicao": exibicao,
				},
			})
		}
	}
	jsonOK(w, out)
}

func (a *App) hAvisosAdd(w http.ResponseWriter, r *http.Request) {
	u := usuarioDoCtx(r)
	if u.Papel != "gerente" {
		jsonErro(w, http.StatusForbidden, "apenas gerentes de grupo podem publicar avisos")
		return
	}
	if u.PapelAtivoID == nil || *u.PapelAtivoID <= 0 {
		jsonErro(w, http.StatusBadRequest, "sem papel ativo na sessão")
		return
	}

	var req struct {
		Titulo   string `json:"titulo"`
		Conteudo string `json:"conteudo"`
		Fixado   bool   `json:"fixado"`
		GrupoID  *int64 `json:"grupo_id"`
	}
	if err := decodificar(r, &req); err != nil {
		jsonErro(w, http.StatusBadRequest, "dados inválidos")
		return
	}

	req.Titulo = strings.TrimSpace(req.Titulo)
	req.Conteudo = strings.TrimSpace(req.Conteudo)
	if req.Titulo == "" || req.Conteudo == "" {
		jsonErro(w, http.StatusBadRequest, "título e conteúdo são obrigatórios")
		return
	}

	grupoID := u.GrupoID
	if grupoID == nil || *grupoID <= 0 {
		jsonErro(w, http.StatusBadRequest, "aviso deve estar vinculado a um grupo")
		return
	}

	fixadoInt := 0
	if req.Fixado {
		fixadoInt = 1
	}

	res, err := a.st.db.Exec(`
		INSERT INTO avisos (titulo, conteudo, autor_usuario_id, autor_papel_id, grupo_id, fixado)
		VALUES (?, ?, ?, ?, ?, ?)`,
		req.Titulo, req.Conteudo, u.ID, *u.PapelAtivoID, *grupoID, fixadoInt)
	if err != nil {
		jsonErro(w, http.StatusInternalServerError, "falha ao publicar aviso: "+err.Error())
		return
	}

	id, _ := res.LastInsertId()
	// O próprio autor já dá o ciente automático
	_, _ = a.st.db.Exec(`INSERT OR IGNORE INTO aviso_cientes (aviso_id, usuario_id, papel_id) VALUES (?, ?, ?)`, id, u.ID, *u.PapelAtivoID)

	a.st.Auditoria(&u.ID, "publicar_aviso", "avisos", &id, req.Titulo, ipDe(r))
	jsonOK(w, map[string]any{"ok": true, "id": id})
}

func (a *App) hAvisosDel(w http.ResponseWriter, r *http.Request) {
	u := usuarioDoCtx(r)
	if u.Papel == "admin" {
		jsonErro(w, http.StatusForbidden, "administrador não possui acesso aos avisos operacionais")
		return
	}
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id <= 0 {
		jsonErro(w, http.StatusBadRequest, "id inválido")
		return
	}

	var autorUID int64
	var gID int64
	err = a.st.db.QueryRow(`SELECT autor_usuario_id, grupo_id FROM avisos WHERE id = ?`, id).Scan(&autorUID, &gID)
	if err != nil {
		jsonErro(w, http.StatusNotFound, "aviso inexistente")
		return
	}

	if u.ID != autorUID && (u.Papel != "gerente" || u.GrupoID == nil || *u.GrupoID != gID) {
		jsonErro(w, http.StatusForbidden, "sem permissão para excluir este aviso")
		return
	}

	_, _ = a.st.db.Exec(`DELETE FROM aviso_comentarios WHERE aviso_id = ?`, id)
	_, _ = a.st.db.Exec(`DELETE FROM aviso_cientes WHERE aviso_id = ?`, id)
	_, err = a.st.db.Exec(`DELETE FROM avisos WHERE id = ?`, id)
	if err != nil {
		jsonErro(w, http.StatusInternalServerError, err.Error())
		return
	}

	a.st.Auditoria(&u.ID, "excluir_aviso", "avisos", &id, "", ipDe(r))
	jsonOK(w, map[string]any{"ok": true})
}

func (a *App) hAvisosCiente(w http.ResponseWriter, r *http.Request) {
	u := usuarioDoCtx(r)
	if u.Papel == "admin" {
		jsonErro(w, http.StatusForbidden, "administrador não possui acesso aos avisos operacionais")
		return
	}
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id <= 0 {
		jsonErro(w, http.StatusBadRequest, "id inválido")
		return
	}

	agora := time.Now().UTC().Format(time.RFC3339)
	_, err = a.st.db.Exec(`
		INSERT INTO aviso_cientes (aviso_id, usuario_id, papel_id, ciente_em)
		VALUES (?, ?, ?, ?)
		ON CONFLICT(aviso_id, usuario_id) DO UPDATE SET ciente_em = excluded.ciente_em`,
		id, u.ID, u.PapelAtivoID, agora)
	if err != nil {
		jsonErro(w, http.StatusInternalServerError, "falha ao registrar ciente: "+err.Error())
		return
	}

	a.st.Auditoria(&u.ID, "ciente_aviso", "avisos", &id, "", ipDe(r))
	jsonOK(w, map[string]any{"ok": true, "ciente_em": agora})
}

func (a *App) hAvisosComentar(w http.ResponseWriter, r *http.Request) {
	u := usuarioDoCtx(r)
	if u.Papel == "admin" {
		jsonErro(w, http.StatusForbidden, "administrador não possui acesso aos avisos operacionais")
		return
	}
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id <= 0 {
		jsonErro(w, http.StatusBadRequest, "id inválido")
		return
	}

	var req struct {
		Comentario string `json:"comentario"`
	}
	if err := decodificar(r, &req); err != nil || strings.TrimSpace(req.Comentario) == "" {
		jsonErro(w, http.StatusBadRequest, "comentário não pode estar vazio")
		return
	}

	res, err := a.st.db.Exec(`
		INSERT INTO aviso_comentarios (aviso_id, usuario_id, papel_id, comentario)
		VALUES (?, ?, ?, ?)`,
		id, u.ID, u.PapelAtivoID, strings.TrimSpace(req.Comentario))
	if err != nil {
		jsonErro(w, http.StatusInternalServerError, "falha ao inserir comentário: "+err.Error())
		return
	}

	comID, _ := res.LastInsertId()
	jsonOK(w, map[string]any{"ok": true, "id": comID})
}

func (a *App) hAvisosDetalhes(w http.ResponseWriter, r *http.Request) {
	u := usuarioDoCtx(r)
	if u.Papel == "admin" {
		jsonErro(w, http.StatusForbidden, "administrador não possui acesso aos avisos operacionais")
		return
	}
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id <= 0 {
		jsonErro(w, http.StatusBadRequest, "id inválido")
		return
	}

	// Comentários
	cRows, _ := a.st.db.Query(`
		SELECT ac.id, ac.comentario, ac.criado_em,
		       u.id, u.login, COALESCE(u.nome_guerra,''), COALESCE(u.nome_completo,''),
		       COALESCE(up.papel,''), COALESCE(up.nome_exibicao,'')
		FROM aviso_comentarios ac
		JOIN usuarios u ON u.id = ac.usuario_id
		LEFT JOIN usuario_papeis up ON up.id = ac.papel_id
		WHERE ac.aviso_id = ? ORDER BY ac.id ASC`, id)

	comentarios := []map[string]any{}
	if cRows != nil {
		for cRows.Next() {
			var cid, uid int64
			var com, dts, login, guerra, completo, papel, exibicao string
			if cRows.Scan(&cid, &com, &dts, &uid, &login, &guerra, &completo, &papel, &exibicao) == nil {
				comentarios = append(comentarios, map[string]any{
					"id":            cid,
					"comentario":    com,
					"criado_em":     dts,
					"usuario_id":    uid,
					"login":         login,
					"nome_guerra":   guerra,
					"nome_completo": completo,
					"papel":         papel,
					"nome_exibicao": exibicao,
				})
			}
		}
		cRows.Close()
	}

	// Cientes
	ciRows, _ := a.st.db.Query(`
		SELECT ac.id, ac.ciente_em,
		       u.id, u.login, COALESCE(u.nome_guerra,''), COALESCE(u.nome_completo,''),
		       COALESCE(up.papel,''), COALESCE(up.nome_exibicao,'')
		FROM aviso_cientes ac
		JOIN usuarios u ON u.id = ac.usuario_id
		LEFT JOIN usuario_papeis up ON up.id = ac.papel_id
		WHERE ac.aviso_id = ? ORDER BY ac.ciente_em ASC`, id)

	cientes := []map[string]any{}
	if ciRows != nil {
		for ciRows.Next() {
			var cid, uid int64
			var dts, login, guerra, completo, papel, exibicao string
			if ciRows.Scan(&cid, &dts, &uid, &login, &guerra, &completo, &papel, &exibicao) == nil {
				cientes = append(cientes, map[string]any{
					"id":            cid,
					"ciente_em":     dts,
					"usuario_id":    uid,
					"login":         login,
					"nome_guerra":   guerra,
					"nome_completo": completo,
					"papel":         papel,
					"nome_exibicao": exibicao,
				})
			}
		}
		ciRows.Close()
	}

	jsonOK(w, map[string]any{
		"comentarios": comentarios,
		"cientes":     cientes,
	})
}

func (a *App) hAvisosRepostar(w http.ResponseWriter, r *http.Request) {
	u := usuarioDoCtx(r)
	if u.Papel != "gerente" {
		jsonErro(w, http.StatusForbidden, "apenas gerentes podem repostar comunicados")
		return
	}
	if u.PapelAtivoID == nil || u.GrupoID == nil {
		jsonErro(w, http.StatusBadRequest, "sem grupo ativo para repostar")
		return
	}

	avisoID, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || avisoID <= 0 {
		jsonErro(w, http.StatusBadRequest, "id inválido")
		return
	}

	var titulo, conteudo string
	var grupoOrigID int64
	err = a.st.db.QueryRow(`SELECT titulo, conteudo, grupo_id FROM avisos WHERE id = ?`, avisoID).Scan(&titulo, &conteudo, &grupoOrigID)
	if err != nil {
		jsonErro(w, http.StatusNotFound, "aviso original inexistente")
		return
	}

	if grupoOrigID == *u.GrupoID {
		jsonErro(w, http.StatusBadRequest, "aviso já pertence ao seu próprio grupo")
		return
	}

	res, err := a.st.db.Exec(`
		INSERT INTO avisos (titulo, conteudo, autor_usuario_id, autor_papel_id, grupo_id, grupo_origem_id, aviso_origem_id)
		VALUES (?, ?, ?, ?, ?, ?, ?)`,
		titulo, conteudo, u.ID, *u.PapelAtivoID, *u.GrupoID, grupoOrigID, avisoID)
	if err != nil {
		jsonErro(w, http.StatusInternalServerError, "falha ao repostar: "+err.Error())
		return
	}

	novoID, _ := res.LastInsertId()
	_, _ = a.st.db.Exec(`INSERT OR IGNORE INTO aviso_cientes (aviso_id, usuario_id, papel_id) VALUES (?, ?, ?)`, novoID, u.ID, *u.PapelAtivoID)

	a.st.Auditoria(&u.ID, "repostar_aviso", "avisos", &novoID, fmt.Sprintf("origem=%d", avisoID), ipDe(r))
	jsonOK(w, map[string]any{"ok": true, "id": novoID})
}
