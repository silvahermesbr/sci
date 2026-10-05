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
// Lista as funções com as designações DO GRUPO DA SESSÃO (escopoDoUsuario:
// gerente vê o próprio; admin=0 vê tudo; sem grupo=-1 → 403). Funções sem
// designação (LEFT JOIN) vêm com membro_id=0 — o front renderiza os dropdowns
// de designação em cima delas.
func (a *App) hFuncaoMembrosGet(w http.ResponseWriter, r *http.Request) {
	u := usuarioDoCtx(r)
	escopo := escopoDoUsuario(u)
	if escopo <= 0 {
		jsonErro(w, http.StatusForbidden, "sem grupo ativo na sessão")
		return
	}
	rows, err := a.st.db.Query(`
		SELECT f.id, f.nome,
		       COALESCE(tm.id, 0), COALESCE(tm.usuario_id, 0),
		       COALESCE(tm.titularidade, ''),
		       COALESCE(mu.login, ''), COALESCE(mu.nome_guerra, ''), COALESCE(mu.nome_completo, '')
		FROM funcoes f
		LEFT JOIN funcao_membros tm ON tm.funcao_id = f.id AND tm.grupo_id = ?
		LEFT JOIN usuarios mu ON mu.id = tm.usuario_id
		ORDER BY f.nome ASC, CASE tm.titularidade WHEN 'titular' THEN 0 ELSE 1 END, tm.id ASC`, escopo)
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
// Validações: gerente com grupo na sessão; função existente no catálogo;
// usuário ATIVO do PRÓPRIO grupo (usuarios.grupo_id); 1 titular por
// (função, grupo) via índice parcial único — segundo titular → 409.
func (a *App) hFuncaoMembrosSet(w http.ResponseWriter, r *http.Request) {
	u := usuarioDoCtx(r)
	escopo := escopoDoUsuario(u)
	if u.Papel != "gerente" || escopo <= 0 {
		jsonErro(w, http.StatusForbidden, "apenas gerentes designam membros de função")
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

	var funcaoExiste int
	if err := a.st.db.QueryRow(`SELECT COUNT(*) FROM funcoes WHERE id = ?`, req.FuncaoID).Scan(&funcaoExiste); err != nil || funcaoExiste == 0 {
		jsonErro(w, http.StatusBadRequest, "função inexistente")
		return
	}
	var loginMembro string
	if err := a.st.db.QueryRow(`SELECT COALESCE(login,'') FROM usuarios WHERE id = ? AND grupo_id = ? AND ativo = 1`, req.UsuarioID, escopo).Scan(&loginMembro); err != nil {
		jsonErro(w, http.StatusBadRequest, "usuário não pertence ao seu grupo")
		return
	}

	res, err := a.st.db.Exec(`INSERT INTO funcao_membros (funcao_id, grupo_id, usuario_id, titularidade) VALUES (?,?,?,?)`,
		req.FuncaoID, escopo, req.UsuarioID, tit)
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
	jsonOK(w, map[string]any{"ok": true, "id": id})
}

// hFuncaoMembrosDel: DELETE /api/grupo/funcoes/membros/{id}
// id = funcao_membros.id. Gerente só apaga designação DO PRÓPRIO grupo
// (filtro grupo_id no WHERE; 404 se não é dele).
func (a *App) hFuncaoMembrosDel(w http.ResponseWriter, r *http.Request) {
	u := usuarioDoCtx(r)
	escopo := escopoDoUsuario(u)
	if u.Papel != "gerente" || escopo <= 0 {
		jsonErro(w, http.StatusForbidden, "apenas gerentes designam membros de função")
		return
	}
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id <= 0 {
		jsonErro(w, http.StatusBadRequest, "id inválido")
		return
	}
	res, err := a.st.db.Exec(`DELETE FROM funcao_membros WHERE id = ? AND grupo_id = ?`, id, escopo)
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
	jsonOK(w, map[string]any{"ok": true})
}
