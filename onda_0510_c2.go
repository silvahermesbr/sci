package main

// onda_0510_c2.go — Onda C2 (ordem Diretor, 05/10): Mural no topo + Aba Funções.
//
// PARTE 2 — FUNÇÕES × MEMBROS (aba "Funções" do Gerenciar): o gerente designa
// os membros de cada função do catálogo (titular/auxiliar), com 1 titular por
// (função, grupo) garantido por índice parcial único (migração v32).
// CRUD em /api/grupo/funcoes/membros, escopo do gerente (grupo da sessão via
// escopoDoUsuario). A PARTE 1 (admin no mural) vive em mensagens.go — removidos
// os guardas de admin em hAvisosList/Add/Del/Ciente/Comentar/Detalhes.

import (
	"database/sql"
	"net/http"
	"strconv"
	"strings"
)

// podeVerMural: admin (visão global) ou qualquer papel com grupo na sessão.
// Ordem "mural em todos os papéis" não alcança conta sem grupo: o mural é
// estruturado por grupo (avisos.grupo_id) — sem grupo não há o que ler.
func podeVerMural(u *Usuario) bool {
	return u != nil && (u.Papel == "admin" || u.GrupoID != nil)
}

// hFuncaoMembrosGet: GET /api/grupo/funcoes/membros
// Lista SOMENTE funções tipo='grupo' com as designações do grupo da sessão.
// Escopo: gerente e encarregado vêem o próprio grupo; admin=0 vê tudo; sem grupo=-1 → 403.
func (a *App) hFuncaoMembrosGet(w http.ResponseWriter, r *http.Request) {
	u := usuarioDoCtx(r)
	escopo := escopoDoUsuario(u)
	if escopo < 0 || (escopo == 0 && u.Papel != "admin") {
		jsonErro(w, http.StatusForbidden, "sem grupo ativo na sessão")
		return
	}

	var rows *sql.Rows
	var err error
	if u.Papel == "admin" && escopo == 0 {
		rows, err = a.st.db.Query(`
			SELECT f.id, f.nome,
			       COALESCE(tm.id, 0), COALESCE(tm.usuario_id, 0),
			       COALESCE(tm.titularidade, ''),
			       COALESCE(mu.login, ''), COALESCE(mu.nome_guerra, ''), COALESCE(mu.nome_completo, '')
			FROM funcoes f
			LEFT JOIN funcao_membros tm ON tm.funcao_id = f.id
			LEFT JOIN usuarios mu ON mu.id = tm.usuario_id
			WHERE f.tipo = 'grupo' AND f.chave IS NOT NULL
			ORDER BY CASE f.chave WHEN 'enc_pessoal' THEN 0 WHEN 'enc_material' THEN 1 ELSE 2 END, f.nome ASC, CASE tm.titularidade WHEN 'titular' THEN 0 ELSE 1 END, tm.id ASC`)
	} else {
		ids := append([]int64{escopo}, a.gruposSuperioresAtivos(escopo)...)
		ph := strings.TrimSuffix(strings.Repeat("?,", len(ids)), ",")
		args := make([]any, 0, len(ids)+1)
		args = append(args, escopo) // LEFT JOIN tm.grupo_id = ?
		for _, id := range ids {
			args = append(args, id)
		}
		rows, err = a.st.db.Query(`
			SELECT f.id, f.nome,
			       COALESCE(tm.id, 0), COALESCE(tm.usuario_id, 0),
			       COALESCE(tm.titularidade, ''),
			       COALESCE(mu.login, ''), COALESCE(mu.nome_guerra, ''), COALESCE(mu.nome_completo, '')
			FROM funcoes f
			LEFT JOIN funcao_membros tm ON tm.funcao_id = f.id AND tm.grupo_id = ?
			LEFT JOIN usuarios mu ON mu.id = tm.usuario_id
			WHERE f.tipo = 'grupo' AND f.chave IS NOT NULL AND (f.grupo_id IS NULL OR f.grupo_id IN (`+ph+`))
			ORDER BY CASE f.chave WHEN 'enc_pessoal' THEN 0 WHEN 'enc_material' THEN 1 ELSE 2 END, f.nome ASC, CASE tm.titularidade WHEN 'titular' THEN 0 ELSE 1 END, tm.id ASC`, args...)
	}
	if err != nil {
		jsonErro(w, http.StatusInternalServerError, "falha ao listar funções: "+err.Error())
		return
	}
	defer rows.Close()
	out := []map[string]any{}
	for rows.Next() {
		var fid, tmID, uid int64
		var nome, tit, login, guerra, completo string
		if err := rows.Scan(&fid, &nome, &tmID, &uid, &tit, &login, &guerra, &completo); err != nil {
			continue
		}
		out = append(out, map[string]any{
			"funcao_id":     fid,
			"funcao_nome":   nome,
			"membro_id":     tmID,
			"usuario_id":    uid,
			"titularidade":  tit,
			"login":         login,
			"nome_guerra":   guerra,
			"nome_completo": completo,
		})
	}
	jsonOK(w, out)
}

// hFuncaoMembrosSet: POST /api/grupo/funcoes/membros
// {funcao_id, usuario_id, titularidade: titular|auxiliar}
// Designação exclusiva de gerente e admin (encarregado/auxiliar → 403).
func (a *App) hFuncaoMembrosSet(w http.ResponseWriter, r *http.Request) {
	u := usuarioDoCtx(r)
	if u == nil || (u.Papel != "gerente" && u.Papel != "admin") {
		jsonErro(w, http.StatusForbidden, "designação de membros de função é restrita a gerente e administrador")
		return
	}
	escopo := escopoDoUsuario(u)
	if u.Papel == "gerente" && escopo <= 0 {
		jsonErro(w, http.StatusForbidden, "sem grupo ativo na sessão")
		return
	}

	var req struct {
		FuncaoID     int64  `json:"funcao_id"`
		UsuarioID    int64  `json:"usuario_id"`
		Titularidade string `json:"titularidade"`
	}
	if err := decodificar(r, &req); err != nil || req.FuncaoID <= 0 || req.UsuarioID <= 0 {
		jsonErro(w, http.StatusBadRequest, "dados inválidos")
		return
	}
	tit := strings.ToLower(strings.TrimSpace(req.Titularidade))
	if tit != "titular" && tit != "auxiliar" {
		jsonErro(w, http.StatusBadRequest, "titularidade deve ser titular|auxiliar")
		return
	}

	var grupoAlvo int64
	var loginMembro string
	if u.Papel == "gerente" {
		grupoAlvo = escopo
		if err := a.st.db.QueryRow(`SELECT COALESCE(login,'') FROM usuarios WHERE id = ? AND grupo_id = ? AND ativo = 1`, req.UsuarioID, escopo).Scan(&loginMembro); err != nil {
			jsonErro(w, http.StatusBadRequest, "usuário não pertence ao seu grupo")
			return
		}
	} else {
		// Admin: resolve o grupo do usuário alvo
		if err := a.st.db.QueryRow(`SELECT COALESCE(grupo_id, 0), COALESCE(login,'') FROM usuarios WHERE id = ? AND ativo = 1`, req.UsuarioID).Scan(&grupoAlvo, &loginMembro); err != nil || grupoAlvo <= 0 {
			jsonErro(w, http.StatusBadRequest, "usuário não pertence a nenhum grupo")
			return
		}
	}

	// Validar que a função é do tipo 'grupo' e pertence ao escopo do grupoAlvo
	var funcaoExiste int
	superiores := a.gruposSuperioresAtivos(grupoAlvo)
	ph := strings.TrimSuffix(strings.Repeat("?,", len(superiores)), ",")
	args := make([]any, 0, len(superiores)+2)
	args = append(args, req.FuncaoID, grupoAlvo)
	for _, s := range superiores {
		args = append(args, s)
	}
	if err := a.st.db.QueryRow(`SELECT COUNT(*) FROM funcoes WHERE id=? AND tipo='grupo' AND chave IS NOT NULL AND (grupo_id IS NULL OR grupo_id=? OR grupo_id IN (`+ph+`))`, args...).Scan(&funcaoExiste); err != nil || funcaoExiste == 0 {
		jsonErro(w, http.StatusBadRequest, "só é possível designar nas cadeiras fixas: Encarregado de Pessoal e Encarregado de Material")
		return
	}

	res, err := a.st.db.Exec(`INSERT INTO funcao_membros (funcao_id, grupo_id, usuario_id, titularidade) VALUES (?,?,?,?)`,
		req.FuncaoID, grupoAlvo, req.UsuarioID, tit)
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE constraint") {
			jsonErro(w, http.StatusConflict, "esta função já possui titular no grupo")
			return
		}
		jsonErro(w, http.StatusInternalServerError, "falha ao designar: "+err.Error())
		return
	}
	id, _ := res.LastInsertId()
	a.st.Auditoria(&u.ID, "designar_funcao_membro", "funcao_membros", &id, loginMembro+" ["+tit+"]", ipDe(r))

	// Sincroniza o DISPLAY da sessão (funcao_nome do papel ativo) — v367, R3
	// ida-e-volta: a linha do designado NAQUELE grupo recebe a função designada
	// INCONDICIONALMENTE (um funcao_id STALE mostrava cadeira errada no dropdown
	// de contexto — pior que sobrescrever). NUNCA cria linha em usuario_papeis
	// (bug 09/10: o sync antigo fabricava papel='operador' e o CHECK/INSERT
	// virava a SESSÃO do designado no próximo login). Designado puro sem linha
	// tem o display vindo de funcao_membros (FuncoesGrupoDoUsuario → /api/me).
	_, _ = a.st.db.Exec(`UPDATE usuario_papeis SET funcao_id = ? WHERE usuario_id = ? AND grupo_id = ?`, req.FuncaoID, req.UsuarioID, grupoAlvo)
	jsonOK(w, map[string]any{"ok": true, "id": id})
}

// hFuncaoMembrosDel: DELETE /api/grupo/funcoes/membros/{id}
// Designação exclusiva de gerente e admin (encarregado/auxiliar → 403).
func (a *App) hFuncaoMembrosDel(w http.ResponseWriter, r *http.Request) {
	u := usuarioDoCtx(r)
	if u == nil || (u.Papel != "gerente" && u.Papel != "admin") {
		jsonErro(w, http.StatusForbidden, "designação de membros de função é restrita a gerente e administrador")
		return
	}
	escopo := escopoDoUsuario(u)
	if u.Papel == "gerente" && escopo <= 0 {
		jsonErro(w, http.StatusForbidden, "sem grupo ativo na sessão")
		return
	}

	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id <= 0 {
		jsonErro(w, http.StatusBadRequest, "id inválido")
		return
	}

	// v367 (R3): dados da designação ANTES do DELETE — o cleanup do display
	// precisa de quem e qual função estavam na linha removida.
	var uidAlvo, fidAlvo, gidAlvo sql.NullInt64
	if u.Papel == "admin" {
		_ = a.st.db.QueryRow(`SELECT usuario_id, funcao_id, grupo_id FROM funcao_membros WHERE id = ?`, id).Scan(&uidAlvo, &fidAlvo, &gidAlvo)
	} else {
		_ = a.st.db.QueryRow(`SELECT usuario_id, funcao_id, grupo_id FROM funcao_membros WHERE id = ? AND grupo_id = ?`, id, escopo).Scan(&uidAlvo, &fidAlvo, &gidAlvo)
	}

	var res sql.Result
	if u.Papel == "admin" {
		res, err = a.st.db.Exec(`DELETE FROM funcao_membros WHERE id = ?`, id)
	} else {
		res, err = a.st.db.Exec(`DELETE FROM funcao_membros WHERE id = ? AND grupo_id = ?`, id, escopo)
	}
	if err != nil {
		jsonErro(w, http.StatusInternalServerError, err.Error())
		return
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		jsonErro(w, http.StatusNotFound, "designação inexistente")
		return
	}
	a.st.Auditoria(&u.ID, "remover_funcao_membro", "funcao_membros", &id, "", ipDe(r))
	// v367, R3 (volta): limpar o display — se o funcao_id da linha do usuário
	// NAQUELE grupo apontava para a função removida, volta a NULL. (Era o outro
	// lado do stale: removida a designação, o dropdown continuava mostrando a
	// cadeira antiga.) Sem linha em usuario_papeis → nada a fazer (não inventa).
	if uidAlvo.Valid && fidAlvo.Valid && fidAlvo.Int64 > 0 {
		if u.Papel == "admin" {
			_, _ = a.st.db.Exec(`UPDATE usuario_papeis SET funcao_id = NULL WHERE usuario_id = ? AND COALESCE(grupo_id,-1) = ? AND funcao_id = ?`, uidAlvo.Int64, gidAlvo.Int64, fidAlvo.Int64)
		} else {
			_, _ = a.st.db.Exec(`UPDATE usuario_papeis SET funcao_id = NULL WHERE usuario_id = ? AND grupo_id = ? AND funcao_id = ?`, uidAlvo.Int64, escopo, fidAlvo.Int64)
		}
	}
	jsonOK(w, map[string]any{"ok": true})
}
