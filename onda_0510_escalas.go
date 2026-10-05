package main

// onda_0510_escalas.go — onda Escalas + Nomeação (ordem Tenente 05/10):
//   • ESCALA DE GUARDA da conferência (v33 conferencia_escalas): gerente designa
//     chefe(s) e operador(es) da escala; chefe de setor designa APENAS operadores
//     do SEU setor. Admin → 403 (regra de ouro dos handlers escopados).
//   • NOMEAR/DESTITUIR CHEFE DE SETOR: gerente escolhe um membro do grupo,
//     atribui o papel 'chefe_setor' e vincula o setor (usuario_papeis + setor_id).
//     Destituir remove o papel (setor fica livre para nova nomeação).
//
// Armadilhas respeitadas: SELECT-primeiro + INSERT OR IGNORE (sem confiar em
// LastInsertId para detectar duplicado); escopo SEMPRE via escopoDoUsuario;
// auditoria a.st.Auditoria com ipDe(r).

import (
	"net/http"
	"strconv"
	"strings"
)

// guardaEscala: quem pode designar/remover na escala de UMA conferência.
//   - gerente do grupo (escopo) → qualquer papel na escala, alvo = membro do grupo;
//   - chefe_setor → SOMENTE papel 'operador', alvo do SEU setor;
//   - operador/encarregado/admin → 403.
// Devolve (autorizado, papelDoDesignador). Escreve o erro HTTP quando reprova.
func guardaEscala(a *App, w http.ResponseWriter, u *Usuario) (bool, string) {
	if u == nil || u.Papel == "admin" {
		jsonErro(w, http.StatusForbidden, "admin não opera a escala de conferência")
		return false, ""
	}
	escopo := escopoDoUsuario(u)
	switch u.Papel {
	case "gerente":
		if escopo <= 0 {
			jsonErro(w, http.StatusForbidden, "gerente sem grupo ativo na sessão")
			return false, ""
		}
		return true, "gerente"
	case "chefe_setor":
		if escopo <= 0 {
			jsonErro(w, http.StatusForbidden, "chefe de setor sem grupo ativo na sessão")
			return false, ""
		}
		if setorDoUsuario(a, u) == nil {
			jsonErro(w, http.StatusForbidden, "sua conta não tem setor atribuído no cadastro — solicite ao gerente")
			return false, ""
		}
		return true, "chefe_setor"
	}
	jsonErro(w, http.StatusForbidden, "papel sem permissão para designar escala")
	return false, ""
}

// validaAlvoEscala: alvo deve ser conta ATIVA; gerente → membro do MESMO grupo
// (usuarios.grupo_id = escopo); chefe → conta do SEU setor (usuarios.setor_id).
func (a *App) validaAlvoEscala(w http.ResponseWriter, u *Usuario, alvoID int64, papelEscala string) bool {
	var grupo, setor *int64
	var login string
	err := a.st.db.QueryRow(`SELECT grupo_id, setor_id, COALESCE(login,'') FROM usuarios WHERE id = ? AND ativo = 1`, alvoID).
		Scan(&grupo, &setor, &login)
	if err != nil {
		jsonErro(w, http.StatusBadRequest, "usuário alvo inexistente ou inativo")
		return false
	}
	escopo := escopoDoUsuario(u)
	if grupo == nil || *grupo != escopo {
		jsonErro(w, http.StatusForbidden, "usuário alvo não pertence ao seu grupo")
		return false
	}
	if u.Papel == "chefe_setor" {
		if papelEscala != "operador" {
			jsonErro(w, http.StatusForbidden, "chefe de setor só designa operador na escala")
			return false
		}
		meuSetor := setorDoUsuario(a, u)
		if meuSetor == nil || setor == nil || *setor != *meuSetor {
			jsonErro(w, http.StatusForbidden, "você só pode designar operadores do seu próprio setor")
			return false
		}
	}
	return true
}

// hConferenciaEscalaSet: POST /api/conferencia/{id}/escala
// {usuario_id, papel_na_escala: chefe|operador}
func (a *App) hConferenciaEscalaSet(w http.ResponseWriter, r *http.Request) {
	u := usuarioDoCtx(r)
	ok, _ := guardaEscala(a, w, u)
	if !ok {
		return
	}
	var req struct {
		UsuarioID     int64  `json:"usuario_id"`
		PapelNaEscala string `json:"papel_na_escala"`
	}
	if err := decodificar(r, &req); err != nil || req.UsuarioID <= 0 {
		jsonErro(w, http.StatusBadRequest, "dados inválidos")
		return
	}
	papel := strings.ToLower(strings.TrimSpace(req.PapelNaEscala))
	if papel != "chefe" && papel != "operador" {
		jsonErro(w, http.StatusBadRequest, "papel_na_escala deve ser chefe|operador")
		return
	}
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id <= 0 {
		jsonErro(w, http.StatusBadRequest, "id inválido")
		return
	}
	// conferência tem que existir e ser do grupo do designador (escopo)
	var gid int64
	if e := a.st.db.QueryRow(`SELECT COALESCE(grupo_id,0) FROM conferencias WHERE id = ?`, id).Scan(&gid); e != nil {
		jsonErro(w, http.StatusNotFound, "conferência inexistente")
		return
	}
	if gid != escopoDoUsuario(u) {
		jsonErro(w, http.StatusForbidden, "conferência de outro grupo")
		return
	}
	if !a.validaAlvoEscala(w, u, req.UsuarioID, papel) {
		return
	}
	// SELECT-primeiro + INSERT OR IGNORE (sem LastInsertId p/ duplicado)
	var existente int
	_ = a.st.db.QueryRow(`SELECT COUNT(*) FROM conferencia_escalas WHERE conferencia_id = ? AND usuario_id = ?`, id, req.UsuarioID).Scan(&existente)
	if _, err := a.st.db.Exec(`INSERT OR IGNORE INTO conferencia_escalas (conferencia_id, usuario_id, papel_na_escala, designado_por) VALUES (?,?,?,?)`,
		id, req.UsuarioID, papel, u.ID); err != nil {
		jsonErro(w, http.StatusInternalServerError, "falha ao designar escala: "+err.Error())
		return
	}
	detalhe := loginDoUsuario(a, req.UsuarioID) + " [" + papel + "]"
	a.st.Auditoria(&u.ID, "designar_escala", "conferencia_escalas", &id, detalhe, ipDe(r))
	jsonOK(w, map[string]any{"ok": true, "ja_designado": existente > 0})
}

// hConferenciaEscalaDel: DELETE /api/conferencia/{id}/escala/{usuario_id}
// mesma guarda; remove a linha da escala.
func (a *App) hConferenciaEscalaDel(w http.ResponseWriter, r *http.Request) {
	u := usuarioDoCtx(r)
	if ok, _ := guardaEscala(a, w, u); !ok {
		return
	}
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id <= 0 {
		jsonErro(w, http.StatusBadRequest, "id inválido")
		return
	}
	alvoID, err := strconv.ParseInt(r.PathValue("usuario_id"), 10, 64)
	if err != nil || alvoID <= 0 {
		jsonErro(w, http.StatusBadRequest, "usuario_id inválido")
		return
	}
	var gid int64
	if e := a.st.db.QueryRow(`SELECT COALESCE(grupo_id,0) FROM conferencias WHERE id = ?`, id).Scan(&gid); e != nil {
		jsonErro(w, http.StatusNotFound, "conferência inexistente")
		return
	}
	if gid != escopoDoUsuario(u) {
		jsonErro(w, http.StatusForbidden, "conferência de outro grupo")
		return
	}
	if u.Papel == "chefe_setor" {
		// chefe só remove operador DO SEU setor
		var papel string
		var setorAlvo, meuSetor *int64
		if e := a.st.db.QueryRow(`SELECT papel_na_escala, (SELECT setor_id FROM usuarios WHERE id = ?) FROM conferencia_escalas WHERE conferencia_id = ? AND usuario_id = ?`,
			alvoID, id, alvoID).Scan(&papel, &setorAlvo); e != nil {
			jsonErro(w, http.StatusNotFound, "designação inexistente")
			return
		}
		meuSetor = setorDoUsuario(a, u)
		if papel != "operador" || meuSetor == nil || setorAlvo == nil || *setorAlvo != *meuSetor {
			jsonErro(w, http.StatusForbidden, "chefe de setor só remove operador do próprio setor")
			return
		}
	}
	res, err := a.st.db.Exec(`DELETE FROM conferencia_escalas WHERE conferencia_id = ? AND usuario_id = ?`, id, alvoID)
	if err != nil {
		jsonErro(w, http.StatusInternalServerError, err.Error())
		return
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		jsonErro(w, http.StatusNotFound, "designação inexistente")
		return
	}
	a.st.Auditoria(&u.ID, "remover_escala", "conferencia_escalas", &id, loginDoUsuario(a, alvoID), ipDe(r))
	jsonOK(w, map[string]any{"ok": true})
}

// escalaDaConferencia: linhas enriquecidas da escala (ordenadas papel, nome).
// Reuso em hConferenciaGet e hConferenciaHoje.
func (a *App) escalaDaConferencia(id int64) []map[string]any {
	out := []map[string]any{}
	rows, err := a.st.db.Query(`
		SELECT ce.id, ce.usuario_id, ce.papel_na_escala, COALESCE(ce.designado_por,0), ce.criado_em,
		       COALESCE(u.login,''), COALESCE(u.nome_guerra,'')
		FROM conferencia_escalas ce
		JOIN usuarios u ON u.id = ce.usuario_id
		WHERE ce.conferencia_id = ?
		ORDER BY ce.papel_na_escala, u.nome_guerra`, id)
	if err != nil {
		return out
	}
	defer rows.Close()
	for rows.Next() {
		var id, uid, por int64
		var papel, criado, login, guerra string
		if rows.Scan(&id, &uid, &papel, &por, &criado, &login, &guerra) == nil {
			out = append(out, map[string]any{
				"id": id, "usuario_id": uid, "papel_na_escala": papel,
				"designado_por": por, "criado_em": criado,
				"login": login, "nome_guerra": guerra,
			})
		}
	}
	return out
}

// hGrupoNomearChefe: POST /api/grupos/{id}/nomear_chefe {usuario_id, setor_id}
// só gerente do PRÓPRIO grupo (admin → 403). INSERT OR IGNORE em usuario_papeis
// ('chefe_setor') + UPDATE usuarios.setor_id.
func (a *App) hGrupoNomearChefe(w http.ResponseWriter, r *http.Request) {
	u := usuarioDoCtx(r)
	escopo := escopoDoUsuario(u)
	if u.Papel != "gerente" || escopo <= 0 {
		jsonErro(w, http.StatusForbidden, "apenas gerentes nomeiam chefe de setor")
		return
	}
	gid, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || gid <= 0 {
		jsonErro(w, http.StatusBadRequest, "id inválido")
		return
	}
	if gid != escopo {
		jsonErro(w, http.StatusForbidden, "grupo fora do seu escopo")
		return
	}
	var req struct {
		UsuarioID int64 `json:"usuario_id"`
		SetorID   int64 `json:"setor_id"`
	}
	if err := decodificar(r, &req); err != nil || req.UsuarioID <= 0 || req.SetorID <= 0 {
		jsonErro(w, http.StatusBadRequest, "dados inválidos")
		return
	}
	var login string
	if e := a.st.db.QueryRow(`SELECT COALESCE(login,'') FROM usuarios WHERE id = ? AND grupo_id = ? AND ativo = 1`, req.UsuarioID, escopo).Scan(&login); e != nil {
		jsonErro(w, http.StatusBadRequest, "usuário não pertence ao seu grupo")
		return
	}
	var setorExiste int
	if e := a.st.db.QueryRow(`SELECT COUNT(*) FROM setores WHERE id = ? AND ativo = 1`, req.SetorID).Scan(&setorExiste); e != nil || setorExiste == 0 {
		jsonErro(w, http.StatusBadRequest, "setor inexistente")
		return
	}
	if _, err := a.st.db.Exec(`INSERT OR IGNORE INTO usuario_papeis (usuario_id, grupo_id, papel) VALUES (?,?, 'chefe_setor')`, req.UsuarioID, escopo); err != nil {
		jsonErro(w, http.StatusInternalServerError, "falha ao nomear: "+err.Error())
		return
	}
	if _, err := a.st.db.Exec(`UPDATE usuarios SET setor_id = ? WHERE id = ?`, req.SetorID, req.UsuarioID); err != nil {
		jsonErro(w, http.StatusInternalServerError, "falha ao vincular setor: "+err.Error())
		return
	}
	a.st.Auditoria(&u.ID, "nomear_chefe", "usuarios", &req.UsuarioID, login+" → setor "+strconv.FormatInt(req.SetorID, 10), ipDe(r))
	jsonOK(w, map[string]any{"ok": true})
}

// hGrupoDestituirChefe: POST /api/grupos/{id}/destituir_chefe {usuario_id}
// só gerente do PRÓPRIO grupo; remove a linha chefe_setor do grupo (setor fica
// livre — usuarios.setor_id NÃO é mexido aqui).
func (a *App) hGrupoDestituirChefe(w http.ResponseWriter, r *http.Request) {
	u := usuarioDoCtx(r)
	escopo := escopoDoUsuario(u)
	if u.Papel != "gerente" || escopo <= 0 {
		jsonErro(w, http.StatusForbidden, "apenas gerentes destituem chefe de setor")
		return
	}
	gid, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || gid <= 0 {
		jsonErro(w, http.StatusBadRequest, "id inválido")
		return
	}
	if gid != escopo {
		jsonErro(w, http.StatusForbidden, "grupo fora do seu escopo")
		return
	}
	var req struct {
		UsuarioID int64 `json:"usuario_id"`
	}
	if err := decodificar(r, &req); err != nil || req.UsuarioID <= 0 {
		jsonErro(w, http.StatusBadRequest, "dados inválidos")
		return
	}
	var login string
	if e := a.st.db.QueryRow(`SELECT COALESCE(login,'') FROM usuarios WHERE id = ? AND grupo_id = ?`, req.UsuarioID, escopo).Scan(&login); e != nil {
		jsonErro(w, http.StatusBadRequest, "usuário não pertence ao seu grupo")
		return
	}
	res, err := a.st.db.Exec(`DELETE FROM usuario_papeis WHERE usuario_id = ? AND grupo_id = ? AND papel = 'chefe_setor'`, req.UsuarioID, escopo)
	if err != nil {
		jsonErro(w, http.StatusInternalServerError, err.Error())
		return
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		jsonErro(w, http.StatusNotFound, "este usuário não é chefe de setor no grupo")
		return
	}
	a.st.Auditoria(&u.ID, "destituir_chefe", "usuarios", &req.UsuarioID, login, ipDe(r))
	jsonOK(w, map[string]any{"ok": true})
}

// loginDoUsuario: detalhe de auditoria legível (best-effort).
func loginDoUsuario(a *App, id int64) string {
	var login string
	_ = a.st.db.QueryRow(`SELECT COALESCE(login,'') FROM usuarios WHERE id = ?`, id).Scan(&login)
	return login
}
