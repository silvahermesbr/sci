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
	"fmt"
	"net/http"
	"strconv"
	"strings"
)

// guardaEscala: quem pode designar/remover na escala de UMA conferência.
//   - gerente do grupo (escopo) → qualquer papel na escala, alvo = membro do grupo;
//   - chefe_setor → SOMENTE papel 'operador', alvo do SEU setor;
//   - operador/encarregado/admin → 403.
//
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
// só gerente do PRÓPRIO grupo (admin → 403). Ordem 06/10 (item 14, REVISÃO do
// item 1): cada setor tem UM chefe (linha em chefe_setores), cada USUÁRIO pode
// chefiar VÁRIOS — nomear o chefe do setor S substitui SÓ o comando daquele
// setor; quem perde S mas comanda outro mantém o papel de sessão chefe_setor.
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
	tx, err := a.st.db.Begin()
	if err != nil {
		jsonErro(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer tx.Rollback()
	// chefe único (ordem 06/10): remove o papel chefe_setor de QUEM quer que
	// esteja com este setor como setor corrente (o chefe anterior), exceto o
	// próprio nomeado (re-nomear o mesmo chefe não pode apagar o papel dele
	// antes do INSERT OR IGNORE — a UNIQUE (usuario_id, grupo_id, papel)
	// ignoraria o re-INSERT e o chefe perderia o papel).
	if _, e := tx.Exec(`INSERT OR IGNORE INTO usuario_papeis (usuario_id, grupo_id, papel) VALUES (?,?, 'chefe_setor')`, req.UsuarioID, escopo); e != nil {
		jsonErro(w, http.StatusInternalServerError, "falha ao nomear: "+e.Error())
		return
	}
	// Multi-chefia (ordem 06/10, item 14): o COMANDO do setor S é a linha em
	// chefe_setores — SUBSTITUIÇÃO 1:1 (setor_id UNIQUE). O papel chefe_setor em
	// usuario_papeis (acesso de sessão) permanece: quem perde S só deixa de
	// comandar S; a purga do papel é automática em quem ficar SEM NENHUM comando.
	if _, e := tx.Exec(`DELETE FROM chefe_setores WHERE setor_id = ?`, req.SetorID); e != nil {
		jsonErro(w, http.StatusInternalServerError, "falha ao liberar comando anterior: "+e.Error())
		return
	}
	if _, e := tx.Exec(`INSERT INTO chefe_setores (grupo_id, setor_id, usuario_id) VALUES (?,?,?)`, escopo, req.SetorID, req.UsuarioID); e != nil {
		jsonErro(w, http.StatusInternalServerError, "falha ao gravar comando: "+e.Error())
		return
	}
	if _, e := tx.Exec(`DELETE FROM usuario_papeis WHERE papel = 'chefe_setor' AND grupo_id = ? AND usuario_id NOT IN (
		SELECT usuario_id FROM chefe_setores WHERE grupo_id = ?
	)`, escopo, escopo); e != nil {
		jsonErro(w, http.StatusInternalServerError, "falha ao purgar papéis sem comando: "+e.Error())
		return
	}
	if _, e := tx.Exec(`UPDATE usuarios SET setor_id = ? WHERE id = ?`, req.SetorID, req.UsuarioID); e != nil {
		jsonErro(w, http.StatusInternalServerError, "falha ao vincular setor: "+e.Error())
		return
	}
	if e := tx.Commit(); e != nil {
		jsonErro(w, http.StatusInternalServerError, e.Error())
		return
	}
	a.st.Auditoria(&u.ID, "nomear_chefe", "usuarios", &req.UsuarioID, login+" → setor "+strconv.FormatInt(req.SetorID, 10), ipDe(r))
	jsonOK(w, map[string]any{"ok": true})
}

// hGrupoDestituirChefe: POST /api/grupos/{id}/destituir_chefe {usuario_id}
// só gerente do PRÓPRIO grupo; multi-chefia (ordem 06/10 item 14): apaga os
// COMANDOS do usuário (chefe_setores do grupo); o PAPEL chefe_setor em
// usuario_papeis (acesso de sessão) só sai se ele ficar SEM NENHUM comando.
// usuarios.setor_id NÃO é mexido aqui.
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
		SetorID   int64 `json:"setor_id"` // opcional (ordem 06/10 item 14): presente → derruba SÓ o comando daquele setor
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
	// Multi-chefia (ordem 06/10, item 14): a fonte do comando é chefe_setores.
	// Com setor_id: apaga SÓ o comando daquele setor — quem comanda outro segue
	// chefe (papel chefe_setor fica). Sem setor_id (legado/X5): apaga TODOS os
	// comandos do grupo; o papel chefe_setor só sai se ele ficar SEM NENHUM.
	if req.SetorID > 0 {
		var nomeSetor string
		if e := a.st.db.QueryRow(`SELECT COALESCE(nome,'') FROM setores WHERE id = ?`, req.SetorID).Scan(&nomeSetor); e != nil || nomeSetor == "" {
			jsonErro(w, http.StatusBadRequest, "setor inexistente")
			return
		}
		resCmd, err := a.st.db.Exec(`DELETE FROM chefe_setores WHERE usuario_id = ? AND grupo_id = ? AND setor_id = ?`, req.UsuarioID, escopo, req.SetorID)
		if err != nil {
			jsonErro(w, http.StatusInternalServerError, err.Error())
			return
		}
		if n, _ := resCmd.RowsAffected(); n == 0 {
			jsonErro(w, http.StatusNotFound, "este usuário não comanda este setor no grupo")
			return
		}
		// papel de sessão só sai se ele ficar SEM NENHUM comando no grupo
		var comandos int
		if e := a.st.db.QueryRow(`SELECT COUNT(*) FROM chefe_setores WHERE usuario_id = ? AND grupo_id = ?`, req.UsuarioID, escopo).Scan(&comandos); e == nil && comandos == 0 {
			if _, err := a.st.db.Exec(`DELETE FROM usuario_papeis WHERE usuario_id = ? AND grupo_id = ? AND papel = 'chefe_setor'`, req.UsuarioID, escopo); err != nil {
				jsonErro(w, http.StatusInternalServerError, err.Error())
				return
			}
		}
		a.st.Auditoria(&u.ID, "destituir_chefe", "usuarios", &req.UsuarioID, fmt.Sprintf("setor_id=%d", req.SetorID), ipDe(r))
		jsonOK(w, map[string]any{"ok": true})
		return
	}
	// guarda do legado: sem setor_id, só destitui quem TEM comando (404 antes apagava só papel)
	var temComando int
	if e := a.st.db.QueryRow(`SELECT COUNT(*) FROM chefe_setores WHERE usuario_id = ? AND grupo_id = ?`, req.UsuarioID, escopo).Scan(&temComando); e == nil && temComando == 0 {
		jsonErro(w, http.StatusNotFound, "este usuário não é chefe de setor no grupo")
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
	// Multi-chefia (ordem 06/10, item 14): destituir apaga SÓ os COMANDOS do
	// usuário (linhas em chefe_setores do grupo) — se ele ainda comanda outro
	// setor, o papel chefe_setor (acesso de sessão) PERMANECE; sai só se ficar
	// sem NENHUM comando no grupo.
	if _, err := a.st.db.Exec(`DELETE FROM chefe_setores WHERE usuario_id = ? AND grupo_id = ?`, req.UsuarioID, escopo); err != nil {
		jsonErro(w, http.StatusInternalServerError, err.Error())
		return
	}
	var comandos int
	if e := a.st.db.QueryRow(`SELECT COUNT(*) FROM chefe_setores WHERE usuario_id = ? AND grupo_id = ?`, req.UsuarioID, escopo).Scan(&comandos); e == nil && comandos == 0 {
		if _, err := a.st.db.Exec(`DELETE FROM usuario_papeis WHERE usuario_id = ? AND grupo_id = ? AND papel = 'chefe_setor'`, req.UsuarioID, escopo); err != nil {
			jsonErro(w, http.StatusInternalServerError, err.Error())
			return
		}
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
