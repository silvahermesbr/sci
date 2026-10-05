package main

import (
	"net/http"
)

// GET /api/drive/seletor — lista leve de arquivos ACESSÍVEIS do usuário para o
// seletor de anexos (ordem 04/10: anexar por referência ao drive, sem duplicar).
// Devolve raiz do próprio grupo + compartilhados com o usuário/papel/grupo.
func (a *App) hDriveSeletor(w http.ResponseWriter, r *http.Request) {
	u := usuarioDoCtx(r)
	if u.Papel == "admin" {
		jsonErro(w, http.StatusForbidden, "administrador não possui acesso ao drive operacional")
		return
	}

	q := `
		SELECT DISTINCT da.id, da.nome_original, da.tipo, da.tamanho, da.criado_em,
		       COALESCE(g.nome, '')
		FROM drive_arquivos da
		LEFT JOIN grupos g ON g.id = da.grupo_id
		LEFT JOIN drive_compartilhamentos dc ON dc.arquivo_id = da.id
		WHERE (
			da.grupo_id = ?
			OR dc.alvo_usuario_id = ?
			OR (dc.alvo_papel_id IS NOT NULL AND dc.alvo_papel_id = ?)
			OR (dc.alvo_grupo_id IS NOT NULL AND dc.alvo_grupo_id = ?)
		)
		ORDER BY da.id DESC LIMIT 200`

	args := []any{}
	if u.GrupoID != nil {
		args = append(args, *u.GrupoID)
	} else {
		args = append(args, -1)
	}
	args = append(args, u.ID)
	if u.PapelAtivoID != nil {
		args = append(args, *u.PapelAtivoID)
	} else {
		args = append(args, -1)
	}
	if u.GrupoID != nil {
		args = append(args, *u.GrupoID)
	} else {
		args = append(args, -1)
	}

	rows, err := a.st.db.Query(q, args...)
	if err != nil {
		jsonErro(w, http.StatusInternalServerError, "falha ao listar arquivos do drive: "+err.Error())
		return
	}
	defer rows.Close()

	out := []map[string]any{}
	for rows.Next() {
		var id int64
		var nome, tipo, criadoEm, grupoNome string
		var tamanho int64
		if err := rows.Scan(&id, &nome, &tipo, &tamanho, &criadoEm, &grupoNome); err == nil {
			out = append(out, map[string]any{
				"drive_arquivo_id": id,
				"nome":             nome,
				"tipo":             tipo,
				"tamanho":          tamanho,
				"criado_em":        criadoEm,
				"grupo_nome":       grupoNome,
			})
		}
	}
	jsonOK(w, out)
}
