package main

import (
	"crypto/sha256"
	"encoding/hex"
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
	case "admin", "gerente", "operador":
	default:
		jsonErro(w, http.StatusBadRequest, "papel inválido (admin | gerente | operador)")
		return
	}

	// Permissões: Admin cria qualquer papel. Gerente só cria operador no seu próprio grupo.
	if u.Papel == "operador" {
		jsonErro(w, http.StatusForbidden, "operador não gerencia papéis")
		return
	}
	if u.Papel == "gerente" {
		if papel != "operador" {
			jsonErro(w, http.StatusForbidden, "gerente só pode atribuir papel de operador")
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

// ---------- Módulo de Mensageria Interna por Função ----------

func (a *App) hMensagensInbox(w http.ResponseWriter, r *http.Request) {
	u := usuarioDoCtx(r)
	if u.PapelAtivoID == nil || *u.PapelAtivoID <= 0 {
		jsonOK(w, []any{})
		return
	}

	rows, err := a.st.db.Query(`
		SELECT m.id, m.assunto, m.corpo, m.criada_em,
		       md.id, md.lida_em, md.lida_por_usuario_id,
		       COALESCE(u_lida.nome_guerra, ''),
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
		WHERE md.destinatario_papel_id = ? AND md.excluida = 0
		ORDER BY m.id DESC LIMIT 100`, *u.PapelAtivoID)
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
		var lidaPorGuerra string
		var lidaEm *string
		var lidaPorID, remGrupoID, remFuncaoID *int64

		if err := rows.Scan(
			&msgID, &assunto, &corpo, &criadaEm,
			&destID, &lidaEm, &lidaPorID, &lidaPorGuerra,
			&remPapelID, &remUsuarioID,
			&remLogin, &remNomeGuerra, &remNomeCompleto,
			&remPapel, &remGrupoID, &remGrupoNome,
			&remFuncaoID, &remFuncaoNome, &remNomeExibicao,
		); err == nil {
			item := map[string]any{
				"id":               msgID,
				"destinatario_id":  destID,
				"assunto":          assunto,
				"corpo":            corpo,
				"criada_em":        criadaEm,
				"lida_em":          lidaEm,
				"lida_por_id":      lidaPorID,
				"lida_por_nome":    lidaPorGuerra,
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
		SELECT m.id, m.assunto, m.corpo, m.criada_em
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
		var assunto, corpo, criadaEm string
		if err := rows.Scan(&id, &assunto, &corpo, &criadaEm); err == nil {
			msgs = append(msgs, map[string]any{
				"id":        id,
				"assunto":   assunto,
				"corpo":     corpo,
				"criada_em": criadaEm,
			})
		}
	}
	rows.Close() // FIXED: explicitly close the outer query before executing inner queries

	out := []map[string]any{}
	for _, msg := range msgs {
		id := msg["id"].(int64)
		destRows, _ := a.st.db.Query(`
			SELECT md.destinatario_papel_id, up.papel, COALESCE(g.nome, ''),
			       COALESCE(f.nome, ''), md.lida_em, COALESCE(u_lida.nome_guerra, '')
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
				var lidaEm *string
				if err := destRows.Scan(&papelID, &papel, &gNome, &fNome, &lidaEm, &lidaPorGuerra); err == nil {
					dests = append(dests, map[string]any{
						"papel_id":      papelID,
						"papel":         papel,
						"grupo_nome":    gNome,
						"funcao_nome":   fNome,
						"lida_em":       lidaEm,
						"lida_por_nome": lidaPorGuerra,
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

	res, err := a.st.db.Exec(`
		INSERT INTO mensagens (assunto, corpo, remetente_papel_id, remetente_usuario_id)
		VALUES (?, ?, ?, ?)`,
		req.Assunto, req.Corpo, *u.PapelAtivoID, u.ID)
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

	a.st.Auditoria(&u.ID, "enviar_mensagem", "mensagens", &msgID,
		fmt.Sprintf("assunto=%s destinatarios=%d", req.Assunto, len(req.DestinatarioPapelIDs)), ipDe(r))

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
		SET lida_em = ?, lida_por_usuario_id = ?
		WHERE mensagem_id = ? AND destinatario_papel_id = ? AND lida_em IS NULL`,
		agora, u.ID, msgID, *u.PapelAtivoID)
	if err != nil {
		jsonErro(w, http.StatusInternalServerError, "falha ao marcar como lida: "+err.Error())
		return
	}

	jsonOK(w, map[string]any{"ok": true, "lida_em": agora})
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

func (a *App) hMensagensContador(w http.ResponseWriter, r *http.Request) {
	u := usuarioDoCtx(r)
	if u.PapelAtivoID == nil || *u.PapelAtivoID <= 0 {
		jsonOK(w, map[string]any{"nao_lidas": 0})
		return
	}

	var count int
	_ = a.st.db.QueryRow(`
		SELECT COUNT(*)
		FROM mensagem_destinatarios
		WHERE destinatario_papel_id = ? AND lida_em IS NULL AND excluida = 0`, *u.PapelAtivoID).Scan(&count)

	jsonOK(w, map[string]any{"nao_lidas": count})
}

func (a *App) hMensagensDestinatarios(w http.ResponseWriter, r *http.Request) {
	u := usuarioDoCtx(r)
	var papelAtivoID int64
	if u.PapelAtivoID != nil {
		papelAtivoID = *u.PapelAtivoID
	}

	rows, err := a.st.db.Query(`
		SELECT up.id, up.usuario_id, u.login, COALESCE(u.nome_guerra, ''), COALESCE(u.nome_completo, ''),
		       up.papel, up.grupo_id, COALESCE(g.nome, ''),
		       up.funcao_id, COALESCE(f.nome, ''), COALESCE(up.nome_exibicao, '')
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
