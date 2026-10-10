package main

import (
	"database/sql"
	"net/http"
	"strconv"
	"strings"
)

var estadosApresentacaoValidos = map[string]bool{
	"presente":          true,
	"dispensado":        true,
	"descompensado":     true,
	"a serviço externo": true,
	"atrasado":          true,
	"falta":             true,
}

// hPessoaApresentacaoGet devolve o estado corrente de apresentação das pessoas
// no escopo do usuário autenticado.
func (a *App) hPessoaApresentacaoGet(w http.ResponseWriter, r *http.Request) {
	_ = a.ensureTabelaApresentacao()
	u := usuarioDoCtx(r)
	esc, err := a.exigeEscopo(u)
	if err != nil {
		jsonErro(w, http.StatusForbidden, "conta sem grupo definido")
		return
	}

	q := `SELECT pa.pessoa_id, pa.estado, COALESCE(pa.motivo, ''),
	             COALESCE(NULLIF(u.nome_guerra, ''), COALESCE(u.login, '')),
	             pa.definido_em,
	             COALESCE(p.atualizado_em, p.criado_em, '')
	      FROM pessoas_apresentacao pa
	      JOIN pessoas p ON p.id = pa.pessoa_id
	      LEFT JOIN usuarios u ON u.id = pa.definido_por`

	var (
		rows *sql.Rows
	)
	if esc > 0 {
		rows, err = a.st.db.Query(q+` WHERE p.grupo_id = ? ORDER BY pa.definido_em DESC, pa.pessoa_id ASC`, esc)
	} else {
		rows, err = a.st.db.Query(q + ` ORDER BY pa.definido_em DESC, pa.pessoa_id ASC`)
	}
	if err != nil {
		jsonErro(w, http.StatusInternalServerError, "erro ao consultar apresentação: "+err.Error())
		return
	}
	defer rows.Close()

	out := []map[string]any{}
	for rows.Next() {
		var pessoaID int64
		var estado, motivo, defNome, defEm, atEm string
		if err := rows.Scan(&pessoaID, &estado, &motivo, &defNome, &defEm, &atEm); err == nil {
			out = append(out, map[string]any{
				"pessoa_id":         pessoaID,
				"estado":            estado,
				"motivo":            motivo,
				"definido_por_nome": defNome,
				"definido_em":       defEm,
				"atualizado_em":     atEm,
			})
		}
	}
	jsonOK(w, map[string]any{"apresentacao": out})
}

// hPessoaApresentacaoSet registra o estado de apresentação corrente de uma pessoa.
func (a *App) hPessoaApresentacaoSet(w http.ResponseWriter, r *http.Request) {
	_ = a.ensureTabelaApresentacao()

	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		jsonErro(w, http.StatusBadRequest, "id inválido")
		return
	}

	var req struct {
		Estado string `json:"estado"`
		Motivo string `json:"motivo"`
	}
	if err := decodificar(r, &req); err != nil {
		jsonErro(w, http.StatusBadRequest, "JSON inválido")
		return
	}
	req.Estado = strings.TrimSpace(req.Estado)
	req.Motivo = strings.TrimSpace(req.Motivo)

	if !estadosApresentacaoValidos[req.Estado] {
		jsonErro(w, http.StatusBadRequest, "estado inválido")
		return
	}

	u := usuarioDoCtx(r)
	var gid *int64
	if err := a.st.db.QueryRow(`SELECT grupo_id FROM pessoas WHERE id = ?`, id).Scan(&gid); err != nil {
		jsonErro(w, http.StatusNotFound, "pessoa inexistente")
		return
	}

	if u.Papel != "admin" {
		esc, err := a.exigeEscopo(u)
		if err != nil {
			jsonErro(w, http.StatusForbidden, "conta sem grupo definido")
			return
		}
		if gid == nil || *gid != esc {
			jsonErro(w, http.StatusForbidden, "pessoa de outro grupo")
			return
		}
	}

	upsert := `INSERT INTO pessoas_apresentacao (pessoa_id, estado, motivo, definido_por, definido_em)
	VALUES (?, ?, ?, ?, strftime('%Y-%m-%dT%H:%M:%fZ','now'))
	ON CONFLICT(pessoa_id) DO UPDATE SET
	    estado = excluded.estado,
	    motivo = excluded.motivo,
	    definido_por = excluded.definido_por,
	    definido_em = strftime('%Y-%m-%dT%H:%M:%fZ','now')
	RETURNING definido_em`

	var definidoEm string
	if err := a.st.db.QueryRow(upsert, id, req.Estado, req.Motivo, u.ID).Scan(&definidoEm); err != nil {
		jsonErro(w, http.StatusInternalServerError, "erro ao registrar apresentação: "+err.Error())
		return
	}

	detalhes := req.Estado
	if req.Motivo != "" {
		detalhes = req.Estado + " " + req.Motivo
	} else {
		detalhes = req.Estado + " "
	}
	a.st.Auditoria(&u.ID, "apresentacao", "pessoas", &id, detalhes, ipDe(r))

	jsonOK(w, map[string]any{
		"ok":          true,
		"pessoa_id":   id,
		"estado":      req.Estado,
		"definido_em": definidoEm,
	})
}

// hPessoaModificacoes lista os últimos 30 registros que envolvem a pessoa
// (auditoria de pessoas e presenças alteradas).
func (a *App) hPessoaModificacoes(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		jsonErro(w, http.StatusBadRequest, "id inválido")
		return
	}

	u := usuarioDoCtx(r)
	var gid *int64
	if err := a.st.db.QueryRow(`SELECT grupo_id FROM pessoas WHERE id = ?`, id).Scan(&gid); err != nil {
		jsonErro(w, http.StatusNotFound, "pessoa inexistente")
		return
	}

	if u.Papel != "admin" {
		esc, err := a.exigeEscopo(u)
		if err != nil {
			jsonErro(w, http.StatusForbidden, "conta sem grupo definido")
			return
		}
		if gid == nil || *gid != esc {
			jsonErro(w, http.StatusForbidden, "pessoa de outro grupo")
			return
		}
	}

	q := `SELECT quando, quem, acao, fonte FROM (
		SELECT ad.em AS quando,
		       COALESCE(NULLIF(u.nome_guerra, ''), COALESCE(u.login, '')) AS quem,
		       ad.acao AS acao,
		       'auditoria' AS fonte
		FROM auditoria ad
		LEFT JOIN usuarios u ON u.id = ad.usuario_id
		WHERE ad.entidade = 'pessoas' AND ad.registro_id = ?

		UNION ALL

		SELECT pr.alterado_em AS quando,
		       COALESCE(NULLIF(u.nome_guerra, ''), COALESCE(u.login, '')) AS quem,
		       'presença alterada: ' || pr.situacao AS acao,
		       'presenca' AS fonte
		FROM presencas pr
		LEFT JOIN usuarios u ON u.id = pr.alterado_por
		WHERE pr.pessoa_id = ? AND pr.alterado_por IS NOT NULL AND pr.alterado_em IS NOT NULL AND pr.alterado_em != ''
	)
	ORDER BY quando DESC
	LIMIT 30`

	rows, err := a.st.db.Query(q, id, id)
	if err != nil {
		jsonErro(w, http.StatusInternalServerError, "erro ao consultar modificações: "+err.Error())
		return
	}
	defer rows.Close()

	mods := []map[string]any{}
	for rows.Next() {
		var quando, quem, acao, fonte string
		if err := rows.Scan(&quando, &quem, &acao, &fonte); err == nil {
			mods = append(mods, map[string]any{
				"quando": quando,
				"quem":   quem,
				"acao":   acao,
				"fonte":  fonte,
			})
		}
	}

	jsonOK(w, map[string]any{"modificacoes": mods})
}
