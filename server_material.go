package main

import (
	"database/sql"
	"encoding/base64"
	"fmt"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// =====================================================================
// MÓDULO DE MATERIAL (v1.5) — Handlers ÚNICOS da feat/v1.5-evolucao
// =====================================================================

func strVal(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}



// -----------------------------------------------------------------
// hMaterialResponsaveisList
// -----------------------------------------------------------------
func (a *App) hMaterialResponsaveisList(w http.ResponseWriter, r *http.Request) {
	u := usuarioDoCtx(r)
	escopo, err := a.exigeEscopo(u)
	if err != nil {
		jsonErro(w, http.StatusForbidden, "conta sem grupo definido")
		return
	}

	q := `SELECT gsr.grupo_id, g.nome, gsr.setor_id, COALESCE(s.nome, 'Carga Geral'),
	             gsr.encarregado_id, COALESCE(pe.nome_guerra, ''), COALESCE(pe.nome_completo, ''),
	             gsr.auxiliar_encarregado_id, COALESCE(pa.nome_guerra, ''), COALESCE(pa.nome_completo, '')
	      FROM grupo_setor_responsaveis gsr
	      JOIN grupos g ON g.id = gsr.grupo_id
	      LEFT JOIN setores s ON s.id = gsr.setor_id
	      LEFT JOIN pessoas pe ON pe.id = gsr.encarregado_id
	      LEFT JOIN pessoas pa ON pa.id = gsr.auxiliar_encarregado_id
	      WHERE (? = 0 OR gsr.grupo_id = ?)
	      ORDER BY g.nome, COALESCE(s.nome, 'Carga Geral')`
	rows, err := a.st.db.Query(q, escopo, escopo)
	if err != nil {
		jsonErro(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer rows.Close()
	var lista []map[string]any
	for rows.Next() {
		var gid, sid, eid, aid int64
		var gn, sn, eng, enc, aug, auc string
		if rows.Scan(&gid, &gn, &sid, &sn, &eid, &eng, &enc, &aid, &aug, &auc) == nil {
			var item map[string]any = map[string]any{
				"grupo_id": gid, "grupo_nome": gn, "setor_id": sid, "setor_nome": sn,
			}
			item["encarregado"] = map[string]any{
				"id": eid, "nome_guerra": eng, "nome_completo": enc,
			}
			item["auxiliar"] = map[string]any{
				"id": aid, "nome_guerra": aug, "nome_completo": auc,
			}
			lista = append(lista, item)
		}
	}
	jsonOK(w, map[string]any{"responsaveis": lista})
}

// -----------------------------------------------------------------
// hMaterialResponsaveisSave
// -----------------------------------------------------------------
func (a *App) hMaterialResponsaveisSave(w http.ResponseWriter, r *http.Request) {
	u := usuarioDoCtx(r)
	if !(u.Papel == "admin" || u.Papel == "gerente" || a.ehEncarregadoDeMaterial(u)) {
		jsonErro(w, http.StatusForbidden, "somente gerente ou encarregado de material define responsáveis")
		return
	}
	esc, err := a.exigeEscopo(u)
	if err != nil {
		jsonErro(w, http.StatusForbidden, "conta sem grupo definido")
		return
	}
	var req struct {
		GrupoID               int64 `json:"grupo_id"`
		SetorID               int64 `json:"setor_id"`
		EncarregadoID         int64 `json:"encarregado_id"`
		AuxiliarEncarregadoID int64 `json:"auxiliar_encarregado_id"`
	}
	if err := decodificar(r, &req); err != nil {
		jsonErro(w, http.StatusBadRequest, "JSON inválido: "+err.Error())
		return
	}
	if a.ehEncarregadoDeMaterial(u) && u.Papel != "admin" {
		if u.GrupoID != nil && *u.GrupoID > 0 {
			req.GrupoID = *u.GrupoID
		}
	}
	if req.GrupoID <= 0 {
		if u.GrupoID != nil && *u.GrupoID > 0 {
			req.GrupoID = *u.GrupoID
		} else {
			jsonErro(w, http.StatusBadRequest, "grupo_id é obrigatório")
			return
		}
	}
	if esc > 0 && req.GrupoID != esc {
		jsonErro(w, http.StatusForbidden, "recurso fora do seu escopo")
		return
	}
	var setorVal, encVal, auxVal *int64
	if req.SetorID > 0 {
		setorVal = &req.SetorID
	}
	if req.EncarregadoID > 0 {
		encVal = &req.EncarregadoID
	}
	if req.AuxiliarEncarregadoID > 0 {
		auxVal = &req.AuxiliarEncarregadoID
	}
	if _, err := a.st.db.Exec(`INSERT INTO grupo_setor_responsaveis (grupo_id, setor_id, encarregado_id, auxiliar_encarregado_id, atualizado_em)
		VALUES (?, ?, ?, ?, strftime('%Y-%m-%dT%H:%M:%fZ','now'))
		ON CONFLICT(grupo_id, setor_id) DO UPDATE SET
			encarregado_id = excluded.encarregado_id,
			auxiliar_encarregado_id = excluded.auxiliar_encarregado_id,
			atualizado_em = strftime('%Y-%m-%dT%H:%M:%fZ','now')`,
		req.GrupoID, setorVal, encVal, auxVal); err != nil {
		jsonErro(w, http.StatusInternalServerError, err.Error())
		return
	}
	jsonOK(w, map[string]any{"ok": true})
}

// -----------------------------------------------------------------
// hMaterialItemAnexosList
// -----------------------------------------------------------------
func (a *App) hMaterialItemAnexosList(w http.ResponseWriter, r *http.Request) {
	itemID, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || itemID <= 0 {
		jsonErro(w, http.StatusBadRequest, "item_id inválido")
		return
	}
	u := usuarioDoCtx(r)
	var itemGrupo int64
	if err := a.st.db.QueryRow(`SELECT COALESCE(grupo_id,0) FROM material_itens WHERE id = ?`, itemID).Scan(&itemGrupo); err != nil {
		jsonErro(w, http.StatusNotFound, "item não encontrado")
		return
	}
	esc, err := a.exigeEscopo(u)
	if err != nil {
		jsonErro(w, http.StatusForbidden, "conta sem grupo definido")
		return
	}
	if esc > 0 && itemGrupo != esc {
		jsonErro(w, http.StatusForbidden, "item fora do seu escopo")
		return
	}
	rows, err := a.st.db.Query(
		`SELECT id, nome_arquivo, tipo_mime, tamanho, criado_em
		 FROM material_item_anexos WHERE item_id = ? ORDER BY criado_em DESC`, itemID)
	if err != nil {
		jsonErro(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer rows.Close()
	var lista []map[string]any
	if lista == nil {
		lista = make([]map[string]any, 0)
	}
	for rows.Next() {
		var id int64
		var nome, mime, criadoEm string
		var tam int64
		if rows.Scan(&id, &nome, &mime, &tam, &criadoEm) == nil {
			lista = append(lista, map[string]any{
				"id": id, "nome_arquivo": nome, "tipo_mime": mime,
				"tamanho": tam, "criado_em": criadoEm,
			})
		}
	}
	jsonOK(w, map[string]any{"anexos": lista})
}

// -----------------------------------------------------------------
// hMaterialItemAnexoAdd
// -----------------------------------------------------------------
func (a *App) hMaterialItemAnexoAdd(w http.ResponseWriter, r *http.Request) {
	itemID, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || itemID <= 0 {
		jsonErro(w, http.StatusBadRequest, "item_id inválido")
		return
	}
	u := usuarioDoCtx(r)
	var itemGrupo int64
	if err := a.st.db.QueryRow(`SELECT COALESCE(grupo_id,0) FROM material_itens WHERE id = ?`, itemID).Scan(&itemGrupo); err != nil {
		jsonErro(w, http.StatusNotFound, "item não encontrado")
		return
	}
	esc, err := a.exigeEscopo(u)
	if err != nil {
		jsonErro(w, http.StatusForbidden, "conta sem grupo definido")
		return
	}
	if esc > 0 && itemGrupo != esc {
		jsonErro(w, http.StatusForbidden, "item fora do seu escopo")
		return
	}

	var req struct {
		NomeArquivo string `json:"nome_arquivo"`
		TipoMIME    string `json:"tipo_mime"`
		DadosBase64 string `json:"dados_base64"`
		Tamanho     int64  `json:"tamanho"`
	}
	if err := decodificar(r, &req); err != nil {
		jsonErro(w, http.StatusBadRequest, "JSON inválido: "+err.Error())
		return
	}
	if strings.TrimSpace(req.NomeArquivo) == "" || strings.TrimSpace(req.DadosBase64) == "" {
		jsonErro(w, http.StatusBadRequest, "Nome do arquivo e dados em base64 são obrigatórios")
		return
	}
	const maxAnexoBase64 = 800 * 1024
	if len(req.DadosBase64) > maxAnexoBase64 {
		jsonErro(w, http.StatusRequestEntityTooLarge, "anexo acima do teto (máx. ~600 KB)")
		return
	}
	mime := strings.ToLower(strings.TrimSpace(req.TipoMIME))
	switch mime {
	case "application/pdf", "image/png", "image/jpeg", "image/webp":
		// permitido
	default:
		jsonErro(w, http.StatusBadRequest, "tipo não permitido (use PDF, PNG, JPEG ou WEBP)")
		return
	}
	nome := sanitizarNomeArquivo(req.NomeArquivo)
	if nome == "" {
		jsonErro(w, http.StatusBadRequest, "nome de arquivo inválido")
		return
	}
	res, err := a.st.db.Exec(
		`INSERT INTO material_item_anexos (item_id, nome_arquivo, tipo_mime, tamanho, dados_base64, criado_em)
		 VALUES (?, ?, ?, ?, ?, strftime('%Y-%m-%dT%H:%M:%fZ','now'))`,
		itemID, nome, mime,
		req.Tamanho, req.DadosBase64)
	if err != nil {
		jsonErro(w, http.StatusInternalServerError, err.Error())
		return
	}
	var anexoID int64
	anexoID, _ = res.LastInsertId()
	jsonOK(w, map[string]any{"ok": true, "id": anexoID, "nome_arquivo": nome})
}

// -----------------------------------------------------------------
// hMaterialItemAnexoGet
// -----------------------------------------------------------------
func (a *App) hMaterialItemAnexoGet(w http.ResponseWriter, r *http.Request) {
	anexoID, err := strconv.ParseInt(r.PathValue("anexo_id"), 10, 64)
	if err != nil || anexoID <= 0 {
		jsonErro(w, http.StatusBadRequest, "anexo_id inválido")
		return
	}
	u := usuarioDoCtx(r)
	var id, itemID int64
	var nome, mime string
	var tam int64
	var dados, criadoEm string
	if err := a.st.db.QueryRow(
		`SELECT id, item_id, nome_arquivo, tipo_mime, tamanho, dados_base64, criado_em
		 FROM material_item_anexos WHERE id = ?`, anexoID).
		Scan(&id, &itemID, &nome, &mime, &tam, &dados, &criadoEm); err != nil {
		jsonErro(w, http.StatusNotFound, "anexo não encontrado")
		return
	}
	var itemGrupo int64
	if err := a.st.db.QueryRow(`SELECT COALESCE(grupo_id,0) FROM material_itens WHERE id = ?`, itemID).Scan(&itemGrupo); err != nil {
		jsonErro(w, http.StatusNotFound, "item não encontrado")
		return
	}
	esc, err := a.exigeEscopo(u)
	if err != nil {
		jsonErro(w, http.StatusForbidden, "conta sem grupo definido")
		return
	}
	if esc > 0 && itemGrupo != esc {
		jsonErro(w, http.StatusForbidden, "item fora do seu escopo")
		return
	}
	jsonOK(w, map[string]any{
		"id": id, "item_id": itemID, "nome_arquivo": nome,
		"tipo_mime": mime, "tamanho": tam, "criado_em": criadoEm,
		"dados_base64": dados,
	})
}

// -----------------------------------------------------------------
// hMaterialItemAnexoDel
// -----------------------------------------------------------------
func (a *App) hMaterialItemAnexoDel(w http.ResponseWriter, r *http.Request) {
	anexoID, err := strconv.ParseInt(r.PathValue("anexo_id"), 10, 64)
	if err != nil || anexoID <= 0 {
		jsonErro(w, http.StatusBadRequest, "anexo_id inválido")
		return
	}
	u := usuarioDoCtx(r)
	var itemID int64
	if err := a.st.db.QueryRow(`SELECT item_id FROM material_item_anexos WHERE id = ?`, anexoID).Scan(&itemID); err != nil {
		jsonErro(w, http.StatusNotFound, "anexo não encontrado")
		return
	}
	var itemGrupo int64
	if err := a.st.db.QueryRow(`SELECT COALESCE(grupo_id,0) FROM material_itens WHERE id = ?`, itemID).Scan(&itemGrupo); err != nil {
		jsonErro(w, http.StatusNotFound, "item não encontrado")
		return
	}
	esc, err := a.exigeEscopo(u)
	if err != nil {
		jsonErro(w, http.StatusForbidden, "conta sem grupo definido")
		return
	}
	if esc > 0 && itemGrupo != esc {
		jsonErro(w, http.StatusForbidden, "item fora do seu escopo")
		return
	}
	_, err = a.st.db.Exec(`DELETE FROM material_item_anexos WHERE id = ?`, anexoID)
	if err != nil {
		jsonErro(w, http.StatusInternalServerError, err.Error())
		return
	}
	jsonOK(w, map[string]any{"ok": true})
}

// -----------------------------------------------------------------
// hMaterialItemComentariosList
// -----------------------------------------------------------------
func (a *App) hMaterialItemComentariosList(w http.ResponseWriter, r *http.Request) {
	itemID, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || itemID <= 0 {
		jsonErro(w, http.StatusBadRequest, "item_id inválido")
		return
	}
	u := usuarioDoCtx(r)
	var itemGrupo int64
	if err := a.st.db.QueryRow(`SELECT COALESCE(grupo_id,0) FROM material_itens WHERE id = ?`, itemID).Scan(&itemGrupo); err != nil {
		jsonErro(w, http.StatusNotFound, "item não encontrado")
		return
	}
	esc, err := a.exigeEscopo(u)
	if err != nil {
		jsonErro(w, http.StatusForbidden, "conta sem grupo definido")
		return
	}
	if esc > 0 && itemGrupo != esc {
		jsonErro(w, http.StatusForbidden, "item fora do seu escopo")
		return
	}
	rows, err := a.st.db.Query(
		`SELECT mic.id, mic.texto, mic.criado_em, mic.operador_id, COALESCE(u.nome_guerra, u.login, '')
		 FROM material_item_comentarios mic
		 JOIN usuarios u ON u.id = mic.operador_id
		 WHERE mic.item_id = ?
		 ORDER BY mic.criado_em DESC`, itemID)
	if err != nil {
		jsonErro(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer rows.Close()
	var lista []map[string]any
	if lista == nil {
		lista = make([]map[string]any, 0)
	}
	for rows.Next() {
		var id, opID int64
		var texto, criadoEm, opNome string
		if rows.Scan(&id, &texto, &criadoEm, &opID, &opNome) == nil {
			lista = append(lista, map[string]any{
				"id": id, "texto": texto, "criado_em": criadoEm,
				"operador_id": opID, "operador_nome": opNome,
			})
		}
	}
	jsonOK(w, map[string]any{"comentarios": lista})
}

// -----------------------------------------------------------------
// hMaterialItemComentarioAdd
// -----------------------------------------------------------------
func (a *App) hMaterialItemComentarioAdd(w http.ResponseWriter, r *http.Request) {
	itemID, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || itemID <= 0 {
		jsonErro(w, http.StatusBadRequest, "item_id inválido")
		return
	}
	u := usuarioDoCtx(r)
	var itemGrupo int64
	if err := a.st.db.QueryRow(`SELECT COALESCE(grupo_id,0) FROM material_itens WHERE id = ?`, itemID).Scan(&itemGrupo); err != nil {
		jsonErro(w, http.StatusNotFound, "item não encontrado")
		return
	}
	esc, err := a.exigeEscopo(u)
	if err != nil {
		jsonErro(w, http.StatusForbidden, "conta sem grupo definido")
		return
	}
	if esc > 0 && itemGrupo != esc {
		jsonErro(w, http.StatusForbidden, "item fora do seu escopo")
		return
	}
	var req struct {
		Texto string `json:"texto"`
	}
	if err := decodificar(r, &req); err != nil {
		jsonErro(w, http.StatusBadRequest, "JSON inválido: "+err.Error())
		return
	}
	if strings.TrimSpace(req.Texto) == "" {
		jsonErro(w, http.StatusBadRequest, "texto é obrigatório")
		return
	}
	if _, err := a.st.db.Exec(
		`INSERT INTO material_item_comentarios (item_id, operador_id, texto, criado_em)
		 VALUES (?, ?, ?, strftime('%Y-%m-%dT%H:%M:%fZ','now'))`,
		itemID, u.ID, strings.TrimSpace(req.Texto)); err != nil {
		jsonErro(w, http.StatusInternalServerError, err.Error())
		return
	}
	jsonOK(w, map[string]any{"ok": true})
}

// -----------------------------------------------------------------
// hMaterialConferenciasList
// -----------------------------------------------------------------
func (a *App) hMaterialConferenciasList(w http.ResponseWriter, r *http.Request) {
	u := usuarioDoCtx(r)
	escopo, err := a.exigeEscopo(u)
	if err != nil {
		jsonErro(w, http.StatusForbidden, "conta sem grupo definido")
		return
	}
	q := `SELECT mc.id, mc.grupo_id, g.nome, COALESCE(mc.setor_id, 0), COALESCE(s.nome, 'Geral'),
	             mc.data, mc.status, mc.aberta_por, COALESCE(ua.nome_guerra, ua.login, ''),
	             mc.aberta_em, mc.fechada_em
	      FROM material_conferencias mc
	      JOIN grupos g ON g.id = mc.grupo_id
	      LEFT JOIN setores s ON s.id = mc.setor_id
	      LEFT JOIN usuarios ua ON ua.id = mc.aberta_por
	      WHERE (? = 0 OR mc.grupo_id = ?)
	      ORDER BY mc.aberta_em DESC`
	rows, err := a.st.db.Query(q, escopo, escopo)
	if err != nil {
		jsonErro(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer rows.Close()
	var lista []map[string]any
	for rows.Next() {
		var id, gid, sid, apID int64
		var gn, sn, data, st, apNome, abertaEm, fechadaEm string
		if rows.Scan(&id, &gid, &gn, &sid, &sn, &data, &st, &apID, &apNome, &abertaEm, &fechadaEm) == nil {
			lista = append(lista, map[string]any{
				"id": id, "grupo_id": gid, "grupo_nome": gn,
				"setor_id": sid, "setor_nome": sn,
				"data": data, "status": st,
				"aberta_por": apID, "aberta_por_nome": apNome,
				"aberta_em": abertaEm, "fechada_em": fechadaEm,
			})
		}
	}
	jsonOK(w, map[string]any{"conferencias": lista})
}

// -----------------------------------------------------------------
// hMaterialConferenciaIniciar
// -----------------------------------------------------------------
func (a *App) hMaterialConferenciaIniciar(w http.ResponseWriter, r *http.Request) {
	u := usuarioDoCtx(r)
	var req struct {
		GrupoID int64  `json:"grupo_id"`
		SetorID int64  `json:"setor_id"`
		Data    string `json:"data"`
	}
	if err := decodificar(r, &req); err != nil {
		jsonErro(w, http.StatusBadRequest, "JSON inválido: "+err.Error())
		return
	}
	esc, err := a.exigeEscopo(u)
	if err != nil {
		jsonErro(w, http.StatusForbidden, "conta sem grupo definido")
		return
	}
	if req.GrupoID > 0 {
		if esc > 0 && req.GrupoID != esc {
			jsonErro(w, http.StatusForbidden, "conferência fora do seu escopo")
			return
		}
	} else {
		if u.GrupoID != nil && *u.GrupoID > 0 {
			req.GrupoID = *u.GrupoID
		} else {
			jsonErro(w, http.StatusBadRequest, "grupo_id é obrigatório")
			return
		}
	}
	if req.Data == "" {
		req.Data = time.Now().In(a.horaLocal).Format("2006-01-02")
	}

	tx, err := a.st.db.Begin()
	if err != nil {
		jsonErro(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer tx.Rollback()

	var setorVal *int64
	if req.SetorID > 0 {
		setorVal = &req.SetorID
	}

	var countAberta int
	if req.SetorID > 0 {
		_ = tx.QueryRow(`SELECT COUNT(*) FROM material_conferencias WHERE grupo_id = ? AND setor_id = ? AND data = ? AND status = 'aberta'`, req.GrupoID, req.SetorID, req.Data).Scan(&countAberta)
	} else {
		_ = tx.QueryRow(`SELECT COUNT(*) FROM material_conferencias WHERE grupo_id = ? AND setor_id IS NULL AND data = ? AND status = 'aberta'`, req.GrupoID, req.Data).Scan(&countAberta)
	}
	if countAberta > 0 {
		jsonErro(w, http.StatusConflict, "Já existe conferência aberta para este grupo/setor/data")
		return
	}

	res, err := tx.Exec(
		`INSERT INTO material_conferencias (grupo_id, setor_id, data, status, aberta_por, aberta_em)
		 VALUES (?, ?, ?, 'aberta', ?, strftime('%Y-%m-%dT%H:%M:%fZ','now'))`,
		req.GrupoID, setorVal, req.Data, u.ID)
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE") {
			jsonErro(w, http.StatusConflict, "Já existe conferência aberta para este grupo/setor/data")
		} else {
			jsonErro(w, http.StatusInternalServerError, err.Error())
		}
		return
	}
	confID, _ := res.LastInsertId()
	if confID <= 0 {
		jsonErro(w, http.StatusInternalServerError, "falha ao obter ID da conferência")
		return
	}

	// Popula checklist com itens do inventário no escopo da conferência (excluindo baixados)
	if req.SetorID > 0 {
		_, err = tx.Exec(`
			INSERT INTO material_conferencia_itens (conferencia_id, item_id, status, quantidade_esperada, quantidade_conferida, conferido_em)
			SELECT ?, id, 'presente', COALESCE(quantidade, 1), 0, strftime('%Y-%m-%dT%H:%M:%fZ','now')
			FROM material_itens
			WHERE grupo_id = ? AND status <> 'baixado' AND setor_id = ?`,
			confID, req.GrupoID, req.SetorID)
	} else {
		_, err = tx.Exec(`
			INSERT INTO material_conferencia_itens (conferencia_id, item_id, status, quantidade_esperada, quantidade_conferida, conferido_em)
			SELECT ?, id, 'presente', COALESCE(quantidade, 1), 0, strftime('%Y-%m-%dT%H:%M:%fZ','now')
			FROM material_itens
			WHERE grupo_id = ? AND status <> 'baixado' AND setor_id IS NULL`,
			confID, req.GrupoID)
	}
	if err != nil {
		jsonErro(w, http.StatusInternalServerError, "falha ao popular itens da conferência: "+err.Error())
		return
	}

	if err := tx.Commit(); err != nil {
		jsonErro(w, http.StatusInternalServerError, err.Error())
		return
	}

	jsonOK(w, map[string]any{"id": confID, "ok": true})
}

// -----------------------------------------------------------------
// hMaterialConferenciaGet
// -----------------------------------------------------------------
func (a *App) hMaterialConferenciaGet(w http.ResponseWriter, r *http.Request) {
	confID, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || confID <= 0 {
		jsonErro(w, http.StatusBadRequest, "id inválido")
		return
	}
	u := usuarioDoCtx(r)
	var id, gid, apID int64
	var sid *int64
	var data, st, apNome, abertaEm, fechadaEm, obs, gNome, sNome string
	if err := a.st.db.QueryRow(
		`SELECT mc.id, mc.grupo_id, COALESCE(g.nome, ''), mc.setor_id, COALESCE(s.nome, 'Carga Geral'),
		        mc.data, mc.status, mc.aberta_por, COALESCE(ua.nome_guerra, ua.login, ''),
		        mc.aberta_em, COALESCE(mc.fechada_em, ''), COALESCE(mc.observacao, '')
		 FROM material_conferencias mc
		 JOIN grupos g ON g.id = mc.grupo_id
		 LEFT JOIN setores s ON s.id = mc.setor_id
		 LEFT JOIN usuarios ua ON ua.id = mc.aberta_por
		 WHERE mc.id = ?`, confID).
		Scan(&id, &gid, &gNome, &sid, &sNome, &data, &st, &apID, &apNome, &abertaEm, &fechadaEm, &obs); err != nil {
		jsonErro(w, http.StatusNotFound, "conferência não encontrada")
		return
	}
	esc, err := a.exigeEscopo(u)
	if err != nil {
		jsonErro(w, http.StatusForbidden, "conta sem grupo definido")
		return
	}
	if esc > 0 && gid != esc {
		jsonErro(w, http.StatusForbidden, "conferência fora do seu escopo")
		return
	}

	rows, rerr := a.st.db.Query(
		`SELECT mci.id, mci.item_id, mi.nome, mi.codigo_patrimonio, COALESCE(mi.sensibilidade, 'convencional'),
		        mci.status, mci.quantidade_esperada, mci.quantidade_conferida,
		        mci.conferido_por, COALESCE(uc.nome_guerra, uc.login, ''),
		        COALESCE(mci.observacao, '')
		 FROM material_conferencia_itens mci
		 JOIN material_itens mi ON mi.id = mci.item_id
		 LEFT JOIN usuarios uc ON uc.id = mci.conferido_por
		 WHERE mci.conferencia_id = ?
		 ORDER BY mi.nome`, confID)
	if rerr != nil {
		jsonErro(w, http.StatusInternalServerError, rerr.Error())
		return
	}
	defer rows.Close()
	var itens []map[string]any
	for rows.Next() {
		var ciID, itemID int64
		var cpID *int64
		var itemNome, codPat, sens, st, obsStr, cpNome string
		var qtdEsp, qtdConf int64
		if err := rows.Scan(&ciID, &itemID, &itemNome, &codPat, &sens, &st, &qtdEsp, &qtdConf, &cpID, &cpNome, &obsStr); err == nil {
			var cpIDVal int64
			if cpID != nil {
				cpIDVal = *cpID
			}
			itens = append(itens, map[string]any{
				"id":                   ciID,
				"item_id":              itemID,
				"item_nome":            itemNome,
				"nome":                 itemNome,
				"codigo_patrimonio":    codPat,
				"sensibilidade":        sens,
				"status":               st,
				"quantidade_esperada":  qtdEsp,
				"quantidade_conferida": qtdConf,
				"conferido_por":        cpIDVal,
				"conferido_por_nome":   cpNome,
				"observacao":           obsStr,
			})
		}
	}
	var sidVal int64
	if sid != nil {
		sidVal = *sid
	}
	jsonOK(w, map[string]any{
		"id":              id,
		"grupo_id":        gid,
		"grupo_nome":      gNome,
		"setor_id":        sidVal,
		"setor_nome":      sNome,
		"data":            data,
		"status":          st,
		"aberta_por":      apID,
		"aberta_por_nome": apNome,
		"aberta_em":       abertaEm,
		"fechada_em":      fechadaEm,
		"observacao":      obs,
		"itens":           itens,
	})
}

// -----------------------------------------------------------------
// hMaterialConferenciaBipar
// -----------------------------------------------------------------
func (a *App) hMaterialConferenciaBipar(w http.ResponseWriter, r *http.Request) {
	confID, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || confID <= 0 {
		jsonErro(w, http.StatusBadRequest, "id inválido")
		return
	}
	u := usuarioDoCtx(r)
	var confGrupoID int64
	if err := a.st.db.QueryRow(`SELECT grupo_id FROM material_conferencias WHERE id = ?`, confID).Scan(&confGrupoID); err != nil {
		jsonErro(w, http.StatusNotFound, "conferência não encontrada")
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
	var req struct {
		ItemID              int64  `json:"item_id"`
		CodigoPatrimonio    string `json:"codigo_patrimonio"`
		Status              string `json:"status"`
		QuantidadeEsperada  int64  `json:"quantidade_esperada"`
		QuantidadeConferida int64  `json:"quantidade_conferida"`
		Observacao          string `json:"observacao"`
	}
	if err := decodificar(r, &req); err != nil {
		jsonErro(w, http.StatusBadRequest, "JSON inválido: "+err.Error())
		return
	}
	if req.ItemID <= 0 && req.CodigoPatrimonio == "" {
		jsonErro(w, http.StatusBadRequest, "item_id ou codigo_patrimonio é obrigatório")
		return
	}
	if req.ItemID <= 0 && req.CodigoPatrimonio != "" {
		// Lookup por código de patrimônio
		if err := a.st.db.QueryRow(`SELECT id FROM material_itens WHERE codigo_patrimonio = ?`, req.CodigoPatrimonio).Scan(&req.ItemID); err != nil {
			jsonErro(w, http.StatusNotFound, "item não encontrado pelo código de patrimônio")
			return
		}
	}
	if req.Status == "" {
		req.Status = "presente"
	}
	if req.QuantidadeEsperada <= 0 {
		req.QuantidadeEsperada = 1
	}
	if req.QuantidadeConferida <= 0 {
		req.QuantidadeConferida = req.QuantidadeEsperada
	}
	if _, err := a.st.db.Exec(
		`INSERT INTO material_conferencia_itens (conferencia_id, item_id, status, quantidade_esperada, quantidade_conferida, conferido_por, observacao)
		 VALUES (?, ?, ?, ?, ?, ?, ?)
		 ON CONFLICT(conferencia_id, item_id) DO UPDATE SET
		     status = excluded.status,
		     quantidade_conferida = excluded.quantidade_conferida,
		     conferido_por = excluded.conferido_por,
		     observacao = excluded.observacao,
		     conferido_em = strftime('%Y-%m-%dT%H:%M:%fZ','now')`,
		confID, req.ItemID, req.Status, req.QuantidadeEsperada, req.QuantidadeConferida, u.ID,
		req.Observacao); err != nil {
		jsonErro(w, http.StatusInternalServerError, err.Error())
		return
	}
	jsonOK(w, map[string]any{"ok": true})
}

// -----------------------------------------------------------------
// hMaterialConferenciaFechar
// -----------------------------------------------------------------
func (a *App) hMaterialConferenciaFechar(w http.ResponseWriter, r *http.Request) {
	confID, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || confID <= 0 {
		jsonErro(w, http.StatusBadRequest, "id inválido")
		return
	}
	u := usuarioDoCtx(r)
	var confGrupoID int64
	if err := a.st.db.QueryRow(`SELECT grupo_id FROM material_conferencias WHERE id = ?`, confID).Scan(&confGrupoID); err != nil {
		jsonErro(w, http.StatusNotFound, "conferência não encontrada")
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
	_, err = a.st.db.Exec(
		`UPDATE material_conferencias SET status = 'fechada', fechada_por = ?, fechada_em = strftime('%Y-%m-%dT%H:%M:%fZ','now')
		 WHERE id = ? AND status = 'aberta'`, u.ID, confID)
	if err != nil {
		jsonErro(w, http.StatusInternalServerError, err.Error())
		return
	}
	jsonOK(w, map[string]any{"ok": true})
}

// -----------------------------------------------------------------
// hMaterialConferenciaPDF — relatório de conferência de material via PDF
// -----------------------------------------------------------------
func (a *App) hMaterialConferenciaPDF(w http.ResponseWriter, r *http.Request) {
	if a.reservaAtivo() {
		jsonErro(w, http.StatusLocked, "módulo em reserva (indisponível nesta instalação)")
		return
	}
	confID, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || confID <= 0 {
		jsonErro(w, http.StatusBadRequest, "id inválido")
		return
	}
	u := usuarioDoCtx(r)

	var p ProntoMaterialPDF
	p.ID = confID
	var confGrupoID int64
	var sid *int64
	var apID int64
	var fpID *int64
	var gNome, sNome, st, data, apNome, fpNome, abertaEm, fechadaEm, obs string

	err = a.st.db.QueryRow(`
		SELECT mc.id, mc.grupo_id, COALESCE(g.nome, '1ª Cia (Geral)'), mc.setor_id, COALESCE(s.nome, 'Carga Geral'),
		       mc.data, mc.status, mc.aberta_por, COALESCE(ua.nome_guerra, ua.login, ''),
		       mc.aberta_em, mc.fechada_por, COALESCE(uf.nome_guerra, uf.login, ''),
		       COALESCE(mc.fechada_em, ''), COALESCE(mc.observacao, '')
		FROM material_conferencias mc
		JOIN grupos g ON g.id = mc.grupo_id
		LEFT JOIN setores s ON s.id = mc.setor_id
		LEFT JOIN usuarios ua ON ua.id = mc.aberta_por
		LEFT JOIN usuarios uf ON uf.id = mc.fechada_por
		WHERE mc.id = ?`, confID).Scan(
		&p.ID, &confGrupoID, &gNome, &sid, &sNome,
		&data, &st, &apID, &apNome,
		&abertaEm, &fpID, &fpNome,
		&fechadaEm, &obs)
	if err != nil {
		jsonErro(w, http.StatusNotFound, "conferência não encontrada")
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
	p.GrupoNome = gNome
	p.SetorNome = sNome
	p.Data = data
	p.Status = st
	p.AbertaPorNome = apNome
	p.AbertaEm = abertaEm
	p.FechadaPorNome = fpNome
	p.FechadaEm = fechadaEm
	p.Observacao = obs

	rows, rerr := a.st.db.Query(`
		SELECT mci.item_id, mi.nome, mi.codigo_patrimonio, COALESCE(mc.nome, 'Geral'),
		       mci.quantidade_esperada, mci.quantidade_conferida, mci.status,
		       COALESCE(uc.nome_guerra, uc.login, '—'), COALESCE(mci.observacao, '')
		FROM material_conferencia_itens mci
		JOIN material_itens mi ON mi.id = mci.item_id
		LEFT JOIN material_categorias mc ON mc.id = mi.categoria_id
		LEFT JOIN usuarios uc ON uc.id = mci.conferido_por
		WHERE mci.conferencia_id = ?
		ORDER BY mi.nome`, confID)
	if rerr != nil {
		jsonErro(w, http.StatusInternalServerError, rerr.Error())
		return
	}
	defer rows.Close()

	totais := map[string]int{"total": 0, "presente": 0, "acautelado": 0, "manutencao": 0, "ausente": 0, "nao_conferido": 0}
	for rows.Next() {
		var it ProntoMaterialItemPDF
		if err := rows.Scan(&it.ItemID, &it.Nome, &it.CodigoPatrimonio, &it.CategoriaNome,
			&it.QuantidadeEsperada, &it.QuantidadeConferida, &it.Status,
			&it.ConferidoPorNome, &it.Observacao); err == nil {
			totais["total"]++
			if it.QuantidadeConferida == 0 {
				totais["nao_conferido"]++
			} else {
				totais[it.Status]++
			}
			p.Itens = append(p.Itens, it)
		}
	}
	p.Totais = totais

	login := "Sistema"
	if u != nil {
		login = u.Login
	}
	pdfBytes, err := a.gerarProntoMaterialPDF(p, login)
	if err != nil {
		jsonErro(w, http.StatusInternalServerError, "erro ao gerar pronto de material em PDF: "+err.Error())
		return
	}

	w.Header().Set("Content-Type", "application/pdf")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`inline; filename="pronto_material_%d.pdf"`, confID))
	w.Header().Set("Content-Length", strconv.Itoa(len(pdfBytes)))
	_, _ = w.Write(pdfBytes)
}

func (a *App) hMaterialCautelaReciboPDF(w http.ResponseWriter, r *http.Request) {
	if a.reservaAtivo() {
		jsonErro(w, http.StatusLocked, "módulo em reserva (indisponível nesta instalação)")
		return
	}
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id <= 0 {
		jsonErro(w, http.StatusBadRequest, "ID inválido")
		return
	}
	u := usuarioDoCtx(r)
	escopo, err := a.exigeEscopo(u)
	if err != nil {
		jsonErro(w, http.StatusForbidden, "conta sem grupo definido")
		return
	}

	var rec ReciboCautelaPDF
	rec.ID = id

	var itemGrupoID int64
	q := `SELECT mc.data_saida, COALESCE(mc.data_devolucao, ''), COALESCE(mc.obs_saida, ''), COALESCE(mc.obs_devolucao, ''), mc.status,
	             mi.nome, mi.codigo_patrimonio, COALESCE(mi.numero_serie, '—'), mi.grupo_id,
	             COALESCE(cat.nome, 'Geral'), COALESCE(NULLIF(mi.sensibilidade, ''), 'convencional'),
	             p.nome_guerra, p.nome_completo, COALESCE(s.nome, 'Indefinido'), COALESCE(fu.nome, 'Indefinida'), COALESCE(g.nome, 'Geral'),
	             COALESCE(NULLIF(ue.nome_guerra,''), NULLIF(ue.nome_completo,''), '—'), COALESCE(NULLIF(ur.nome_guerra,''), NULLIF(ur.nome_completo,''), '—')
	      FROM material_cautelas mc
	      JOIN material_itens mi ON mi.id = mc.item_id
	      LEFT JOIN material_categorias cat ON cat.id = mi.categoria_id
	      JOIN pessoas p ON p.id = mc.pessoa_id
	      LEFT JOIN setores s ON s.id = p.setor_id
	      LEFT JOIN funcoes fu ON fu.id = p.funcao_id
	      LEFT JOIN grupos g ON g.id = p.grupo_id
	      JOIN usuarios ue ON ue.id = mc.responsavel_entrega_id
	      LEFT JOIN usuarios ur ON ur.id = mc.responsavel_recebimento_id
	      WHERE mc.id = ?`

	err = a.st.db.QueryRow(q, id).Scan(
		&rec.DataSaida, &rec.DataDevolucao, &rec.ObsSaida, &rec.ObsDevolucao, &rec.Status,
		&rec.ItemNome, &rec.CodigoPatrimonio, &rec.NumeroSerie, &itemGrupoID,
		&rec.CategoriaNome, &rec.Sensibilidade,
		&rec.PessoaNomeGuerra, &rec.PessoaCompleto, &rec.PessoaSetor, &rec.PessoaFuncao, &rec.PessoaGrupo,
		&rec.ResponsavelSaida, &rec.ResponsavelDev,
	)
	if err != nil {
		jsonErro(w, http.StatusNotFound, "cautela não encontrada")
		return
	}

	if escopo > 0 && itemGrupoID != escopo {
		jsonErro(w, http.StatusForbidden, "cautela fora do seu escopo")
		return
	}

	pdf, err := a.gerarReciboCautelaPDF(rec, u.Login)
	if err != nil {
		jsonErro(w, http.StatusInternalServerError, "falha ao gerar recibo de cautela: "+err.Error())
		return
	}

	a.st.Auditoria(&u.ID, "exportar", "recibo_cautela", &id, rec.CodigoPatrimonio, ipDe(r))
	w.Header().Set("Cache-Control", "no-store, no-cache, must-revalidate")
	w.Header().Set("Pragma", "no-cache")
	w.Header().Set("Content-Type", "application/pdf")
	w.Header().Set("Content-Disposition", fmt.Sprintf("inline; filename=recibo_cautela_%d.pdf", id))
	_, _ = w.Write(pdf)
}

func (a *App) hMaterialInventarioPDF(w http.ResponseWriter, r *http.Request) {
	if a.reservaAtivo() {
		jsonErro(w, http.StatusLocked, "módulo em reserva (indisponível nesta instalação)")
		return
	}
	u := usuarioDoCtx(r)
	escopo, err := a.exigeEscopo(u)
	if err != nil {
		jsonErro(w, http.StatusForbidden, "conta sem grupo definido")
		return
	}

	var inv InventarioRelatorioPDF
	inv.Totais = map[string]int{"total": 0, "disponivel": 0, "acautelado": 0, "manutencao": 0, "baixado": 0}

	var grupoNome string
	if escopo > 0 {
		_ = a.st.db.QueryRow(`SELECT nome FROM grupos WHERE id = ?`, escopo).Scan(&grupoNome)
	} else {
		grupoNome = "Carga Geral Institucional"
	}
	inv.Grupo = grupoNome

	q := `SELECT mi.id, mi.codigo_patrimonio, mi.nome, COALESCE(cat.nome, 'Geral'),
	             COALESCE(mi.numero_serie, '—'), mi.status,
	             COALESCE(p.nome_guerra, '—') AS responsavel
	      FROM material_itens mi
	      LEFT JOIN material_categorias cat ON cat.id = mi.categoria_id
	      LEFT JOIN material_cautelas mc ON mc.item_id = mi.id AND mc.status = 'ativa'
	      LEFT JOIN pessoas p ON p.id = mc.pessoa_id
	      WHERE 1=1`
	var args []any
	if escopo > 0 {
		q += ` AND mi.grupo_id = ?`
		args = append(args, escopo)
	}
	q += ` ORDER BY mi.status = 'acautelado' DESC, cat.nome ASC, mi.nome ASC`

	rows, err := a.st.db.Query(q, args...)
	if err != nil {
		jsonErro(w, http.StatusInternalServerError, "falha ao buscar inventário: "+err.Error())
		return
	}
	defer rows.Close()

	for rows.Next() {
		var id int64
		var cod, nome, cat, numSerie, st, resp string
		if rows.Scan(&id, &cod, &nome, &cat, &numSerie, &st, &resp) == nil {
			inv.Totais["total"]++
			if _, ok := inv.Totais[st]; ok {
				inv.Totais[st]++
			}
			inv.Itens = append(inv.Itens, map[string]any{
				"id":                id,
				"codigo_patrimonio": cod,
				"nome":              nome,
				"categoria_nome":    cat,
				"numero_serie":      numSerie,
				"status":            st,
				"responsavel_atual": resp,
			})
		}
	}

	pdf, err := a.gerarInventarioMaterialPDF(inv, u.Login)
	if err != nil {
		jsonErro(w, http.StatusInternalServerError, "falha ao gerar PDF de inventário: "+err.Error())
		return
	}

	a.st.Auditoria(&u.ID, "exportar", "inventario_pdf", nil, grupoNome, ipDe(r))
	w.Header().Set("Cache-Control", "no-store, no-cache, must-revalidate")
	w.Header().Set("Pragma", "no-cache")
	w.Header().Set("Content-Type", "application/pdf")
	w.Header().Set("Content-Disposition", "inline; filename=inventario_material.pdf")
	_, _ = w.Write(pdf)
}

func (a *App) hMaterialCategoriasList(w http.ResponseWriter, r *http.Request) {
	u := usuarioDoCtx(r)
	escopo, err := a.exigeEscopo(u)
	if err != nil {
		jsonErro(w, http.StatusForbidden, "conta sem grupo definido")
		return
	}
	rows, err := a.st.db.Query(
		`SELECT id, COALESCE(grupo_id, 0), nome, ativo
		 FROM material_categorias
		 WHERE (grupo_id IS NULL OR grupo_id = ? OR ? = 0)
		 ORDER BY nome`, escopo, escopo)
	if err != nil {
		jsonErro(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer rows.Close()
	var lista []map[string]any
	for rows.Next() {
		var id, gid int64
		var nome string
		var ativo int
		if rows.Scan(&id, &gid, &nome, &ativo) == nil {
			lista = append(lista, map[string]any{
				"id": id, "grupo_id": gid, "nome": nome, "ativo": ativo == 1,
			})
		}
	}
	jsonOK(w, map[string]any{"categorias": lista})
}

func (a *App) hMaterialCategoriasAdd(w http.ResponseWriter, r *http.Request) {
	u := usuarioDoCtx(r)
	esc, err := a.exigeEscopo(u)
	if err != nil {
		jsonErro(w, http.StatusForbidden, "conta sem grupo definido")
		return
	}
	var req struct {
		ID    int64  `json:"id"`
		Nome  string `json:"nome"`
		Ativo *bool  `json:"ativo"`
	}
	if err := decodificar(r, &req); err != nil || strings.TrimSpace(req.Nome) == "" {
		jsonErro(w, http.StatusBadRequest, "Nome da categoria é obrigatório")
		return
	}
	req.Nome = strings.TrimSpace(req.Nome)
	ativo := 1
	if req.Ativo != nil && !*req.Ativo {
		ativo = 0
	}
	if req.ID > 0 {
		var catGrupoID sql.NullInt64
		if err := a.st.db.QueryRow(`SELECT grupo_id FROM material_categorias WHERE id = ?`, req.ID).Scan(&catGrupoID); err != nil {
			jsonErro(w, http.StatusNotFound, "categoria não encontrada")
			return
		}
		if !catGrupoID.Valid || catGrupoID.Int64 == 0 {
			if u.Papel != "admin" {
				jsonErro(w, http.StatusForbidden, "apenas admin pode editar categoria global")
				return
			}
		} else {
			if esc > 0 && catGrupoID.Int64 != esc {
				jsonErro(w, http.StatusForbidden, "categoria fora do seu escopo")
				return
			}
		}
		_, err := a.st.db.Exec(`UPDATE material_categorias SET nome = ?, ativo = ? WHERE id = ?`, req.Nome, ativo, req.ID)
		if err != nil {
			jsonErro(w, http.StatusInternalServerError, err.Error())
			return
		}
		jsonOK(w, map[string]any{"ok": true, "id": req.ID})
		return
	}
	res, err := a.st.db.Exec(`INSERT INTO material_categorias (grupo_id, nome, ativo) VALUES (?, ?, ?)`, u.GrupoID, req.Nome, ativo)
	if err != nil {
		jsonErro(w, http.StatusInternalServerError, err.Error())
		return
	}
	nid, _ := res.LastInsertId()
	jsonOK(w, map[string]any{"ok": true, "id": nid})
}

func (a *App) hMaterialCategoriasDel(w http.ResponseWriter, r *http.Request) {
	u := usuarioDoCtx(r)
	esc, err := a.exigeEscopo(u)
	if err != nil {
		jsonErro(w, http.StatusForbidden, "conta sem grupo definido")
		return
	}
	idStr := r.PathValue("id")
	id, _ := strconv.ParseInt(idStr, 10, 64)
	if id <= 0 {
		jsonErro(w, http.StatusBadRequest, "ID inválido")
		return
	}
	var catGrupoID sql.NullInt64
	if err := a.st.db.QueryRow(`SELECT grupo_id FROM material_categorias WHERE id = ?`, id).Scan(&catGrupoID); err != nil {
		jsonErro(w, http.StatusNotFound, "categoria não encontrada")
		return
	}
	if !catGrupoID.Valid || catGrupoID.Int64 == 0 {
		if u.Papel != "admin" {
			jsonErro(w, http.StatusForbidden, "apenas admin pode excluir categoria global")
			return
		}
	} else {
		if esc > 0 && catGrupoID.Int64 != esc {
			jsonErro(w, http.StatusForbidden, "categoria fora do seu escopo")
			return
		}
	}
	var count int
	_ = a.st.db.QueryRow(`SELECT COUNT(*) FROM material_itens WHERE categoria_id = ?`, id).Scan(&count)
	if count > 0 {
		_, _ = a.st.db.Exec(`UPDATE material_categorias SET ativo = 0 WHERE id = ?`, id)
		jsonOK(w, map[string]any{"ok": true, "desativado": true})
		return
	}
	_, err = a.st.db.Exec(`DELETE FROM material_categorias WHERE id = ?`, id)
	if err != nil {
		jsonErro(w, http.StatusInternalServerError, err.Error())
		return
	}
	jsonOK(w, map[string]any{"ok": true})
}

func (a *App) hMaterialItensList(w http.ResponseWriter, r *http.Request) {
	u := usuarioDoCtx(r)
	escopo, err := a.exigeEscopo(u)
	if err != nil {
		jsonErro(w, http.StatusForbidden, "conta sem grupo definido")
		return
	}
	statusQ := r.URL.Query().Get("status")
	catQ := r.URL.Query().Get("categoria_id")
	garagemQ := r.URL.Query().Get("garagem")

	q := `
		SELECT mi.id, mi.grupo_id, COALESCE(g.nome, ''), mi.categoria_id, COALESCE(mc.nome, 'Sem Categoria'),
		       mi.nome, mi.codigo_patrimonio, COALESCE(mi.numero_serie, ''), mi.status, COALESCE(mi.observacao, ''),
		       mi.criado_em, COALESCE(mi.nivel_sensibilidade, 'padrao'),
		       COALESCE(mi.sensibilidade, 'convencional'), COALESCE(mi.quantidade, 1),
		       COALESCE((SELECT SUM(mc.quantidade) FROM material_cautelas mc WHERE mc.item_id = mi.id AND mc.status = 'ativa'), 0),
		       caut.id, caut.pessoa_id, p.nome_guerra, p.nome_completo, caut.data_saida, COALESCE(caut.obs_saida, ''),
		       ue.login,
		       mi.setor_id, COALESCE(s.nome, ''),
		       mv.item_id, COALESCE(mv.placa, ''), COALESCE(mv.renavam, ''),
		       mv.padrinho_titular_id, COALESCE(pt.nome_guerra, ''),
		       mv.padrinho_substituto_id, COALESCE(ps.nome_guerra, ''),
		       COALESCE(mv.hodometro_atual, 0), COALESCE(mv.combustivel_atual, 'cheio')
		FROM material_itens mi
		LEFT JOIN material_categorias mc ON mc.id = mi.categoria_id
		LEFT JOIN grupos g ON g.id = mi.grupo_id
		LEFT JOIN setores s ON s.id = mi.setor_id
		LEFT JOIN material_viaturas mv ON mv.item_id = mi.id
		LEFT JOIN pessoas pt ON pt.id = mv.padrinho_titular_id
		LEFT JOIN pessoas ps ON ps.id = mv.padrinho_substituto_id
		LEFT JOIN material_cautelas caut ON caut.item_id = mi.id AND caut.status = 'ativa'
		LEFT JOIN pessoas p ON p.id = caut.pessoa_id
		LEFT JOIN usuarios ue ON ue.id = caut.responsavel_entrega_id
		WHERE (? = 0 OR mi.grupo_id = ?)`
	args := []any{escopo, escopo}

	if statusQ != "" {
		q += ` AND mi.status = ?`
		args = append(args, statusQ)
	}
	if catQ != "" {
		if cid, err := strconv.ParseInt(catQ, 10, 64); err == nil && cid > 0 {
			q += ` AND mi.categoria_id = ?`
			args = append(args, cid)
		}
	}
	if garagemQ == "1" {
		q += ` AND (mi.categoria_id IN (SELECT id FROM material_categorias WHERE LOWER(nome) LIKE '%viatur%') OR mv.item_id IS NOT NULL)`
	}
	q += ` ORDER BY mi.status, mc.nome, mi.nome`

	rows, err := a.st.db.Query(q, args...)
	if err != nil {
		jsonErro(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer rows.Close()

	var lista []map[string]any
	for rows.Next() {
		var id, gid int64
		var catID, setorID, mvItemID, padTitID, padSubID *int64
		var gNome, catNome, nome, cod, numSerie, status, obs, criadoEm, sens, sensibilidade, setorNome string
		var mvPlaca, mvRenavam, padTitNome, padSubNome, mvCombustivel string
		var mvHodometro int64
		var quantidade, qtdAcautelada int
		var cautID, pesID *int64
		var pNomeGuerra, pNomeCompleto, dtSaida, obsSaida, opEntrega *string
		if err := rows.Scan(&id, &gid, &gNome, &catID, &catNome, &nome, &cod, &numSerie, &status, &obs, &criadoEm, &sens,
			&sensibilidade, &quantidade, &qtdAcautelada,
			&cautID, &pesID, &pNomeGuerra, &pNomeCompleto, &dtSaida, &obsSaida, &opEntrega,
			&setorID, &setorNome,
			&mvItemID, &mvPlaca, &mvRenavam,
			&padTitID, &padTitNome,
			&padSubID, &padSubNome,
			&mvHodometro, &mvCombustivel); err == nil {

			dispQtd := quantidade - qtdAcautelada
			if dispQtd < 0 {
				dispQtd = 0
			}

			item := map[string]any{
				"id":                    id,
				"grupo_id":              gid,
				"grupo_nome":            gNome,
				"setor_id":              setorID,
				"setor_nome":            setorNome,
				"categoria_id":          catID,
				"categoria_nome":        catNome,
				"nome":                  nome,
				"codigo_patrimonio":     cod,
				"numero_serie":          numSerie,
				"status":                status,
				"observacao":            obs,
				"criado_em":             criadoEm,
				"nivel_sensibilidade":   sens,
				"sensibilidade":         sensibilidade,
				"quantidade":            quantidade,
				"quantidade_acautelada": qtdAcautelada,
				"quantidade_disponivel": dispQtd,
			}
			if cautID != nil {
				item["cautela_ativa"] = map[string]any{
					"id":                   *cautID,
					"pessoa_id":            pesID,
					"pessoa_nome_guerra":   pNomeGuerra,
					"pessoa_nome_completo": pNomeCompleto,
					"data_saida":           dtSaida,
					"obs_saida":            obsSaida,
					"responsavel_entrega":  opEntrega,
				}
			}
			if mvItemID != nil {
				item["viatura"] = map[string]any{
					"placa":                    mvPlaca,
					"renavam":                  mvRenavam,
					"padrinho_titular_id":      padTitID,
					"padrinho_titular_nome":    padTitNome,
					"padrinho_substituto_id":   padSubID,
					"padrinho_substituto_nome": padSubNome,
					"hodometro_atual":          mvHodometro,
					"combustivel_atual":        mvCombustivel,
				}
			}
			lista = append(lista, item)
		}
	}
	jsonOK(w, map[string]any{"itens": lista})
}

func (a *App) hMaterialItensSave(w http.ResponseWriter, r *http.Request) {
	u := usuarioDoCtx(r)
	esc, err := a.exigeEscopo(u)
	if err != nil {
		jsonErro(w, http.StatusForbidden, "conta sem grupo definido")
		return
	}
	var req struct {
		ID                   int64  `json:"id"`
		GrupoID              *int64 `json:"grupo_id"`
		SetorID              *int64 `json:"setor_id"`
		CategoriaID          *int64 `json:"categoria_id"`
		Nome                 string `json:"nome"`
		CodigoPatrimonio     string `json:"codigo_patrimonio"`
		Patrimonio           string `json:"patrimonio"`
		NumeroSerie          string `json:"numero_serie"`
		Status               string `json:"status"`
		Observacao           string `json:"observacao"`
		NivelSensibilidade   string `json:"nivel_sensibilidade"`
		Sensibilidade        string `json:"sensibilidade"`
		Quantidade           int    `json:"quantidade"`
		Placa                string `json:"placa"`
		Renavam              string `json:"renavam"`
		PadrinhoTitularID    *int64 `json:"padrinho_titular_id"`
		PadrinhoSubstitutoID *int64 `json:"padrinho_substituto_id"`
		HodometroAtual       int64  `json:"hodometro_atual"`
		CombustivelAtual     string `json:"combustivel_atual"`
	}
	if err := decodificar(r, &req); err != nil || strings.TrimSpace(req.Nome) == "" {
		jsonErro(w, http.StatusBadRequest, "Nome é obrigatório")
		return
	}
	if strings.TrimSpace(req.CodigoPatrimonio) == "" && strings.TrimSpace(req.Patrimonio) != "" {
		req.CodigoPatrimonio = strings.TrimSpace(req.Patrimonio)
	}

	// Validação de SetorID se informado
	var setorVal *int64
	if req.SetorID != nil && *req.SetorID > 0 {
		var count int
		if err := a.st.db.QueryRow(`SELECT COUNT(*) FROM setores WHERE id = ?`, *req.SetorID).Scan(&count); err != nil || count == 0 {
			jsonErro(w, http.StatusBadRequest, "setor inválido")
			return
		}
		setorVal = req.SetorID
	}

	// v1.5: Normalização de Sensibilidade (apenas 'convencional' e 'controlado')
	if req.Sensibilidade == "" {
		if req.NivelSensibilidade == "sensivel" || req.NivelSensibilidade == "restrito" {
			req.Sensibilidade = "controlado"
		} else {
			req.Sensibilidade = "convencional"
		}
	}
	if req.Sensibilidade == "controlado" {
		req.Quantidade = 1
		if strings.TrimSpace(req.CodigoPatrimonio) == "" {
			jsonErro(w, http.StatusBadRequest, "Código de Patrimônio é obrigatório para material controlado")
			return
		}
	} else {
		req.Sensibilidade = "convencional"
		if req.Quantidade <= 0 {
			req.Quantidade = 1
		}
		if strings.TrimSpace(req.CodigoPatrimonio) == "" {
			req.CodigoPatrimonio = fmt.Sprintf("MAT-%d", time.Now().UnixNano()%100000000)
		}
	}

	grupoID := int64(0)
	if u.GrupoID != nil {
		grupoID = *u.GrupoID
	}
	// Fix P0/P1-2: grupo do CORPO só é honrado para ADMIN (gestão global). Para
	// gerente/operador é SILENCIOSAMENTE IGNORADO — o front legitamente ecoa o
	// grupo do item na edição (views_material.js), mas um corpo forjado apontando
	// outro grupo nunca vira alvo; o escopo do UPDATE + RowsAffected protegem o resto.
	if req.GrupoID != nil && *req.GrupoID > 0 && u.Papel == "admin" {
		grupoID = *req.GrupoID
	}
	if grupoID <= 0 {
		_ = a.st.db.QueryRow(`SELECT id FROM grupos ORDER BY id LIMIT 1`).Scan(&grupoID)
	}
	if grupoID <= 0 {
		resG, errG := a.st.db.Exec(`INSERT INTO grupos (nome, codigo) VALUES ('1ª Cia (Geral)', ?)`, gerarCodigoGrupo())
		if errG == nil {
			grupoID, _ = resG.LastInsertId()
		}
	}
	if grupoID <= 0 {
		jsonErro(w, http.StatusBadRequest, "Grupo é obrigatório")
		return
	}
	if req.Status == "" {
		req.Status = "disponivel"
	}
	if req.NivelSensibilidade == "" {
		req.NivelSensibilidade = "padrao"
	}

	salvarViaturaSeNecessario := func(itemID int64) error {
		temViatura := strings.TrimSpace(req.Placa) != "" ||
			strings.TrimSpace(req.Renavam) != "" ||
			(req.PadrinhoTitularID != nil && *req.PadrinhoTitularID > 0) ||
			(req.PadrinhoSubstitutoID != nil && *req.PadrinhoSubstitutoID > 0) ||
			req.HodometroAtual > 0 ||
			strings.TrimSpace(req.CombustivelAtual) != ""

		if !temViatura {
			return nil
		}

		comb := strings.TrimSpace(req.CombustivelAtual)
		switch comb {
		case "cheio", "3/4", "1/2", "1/4":
			// ok
		default:
			comb = "cheio"
		}

		var padTit, padSub *int64
		if req.PadrinhoTitularID != nil && *req.PadrinhoTitularID > 0 {
			padTit = req.PadrinhoTitularID
		}
		if req.PadrinhoSubstitutoID != nil && *req.PadrinhoSubstitutoID > 0 {
			padSub = req.PadrinhoSubstitutoID
		}

		_, err := a.st.db.Exec(`
			INSERT INTO material_viaturas (item_id, placa, renavam, padrinho_titular_id,
			       padrinho_substituto_id, hodometro_atual, combustivel_atual, atualizado_em)
			VALUES (?, ?, ?, ?, ?, ?, ?, strftime('%Y-%m-%dT%H:%M:%fZ','now'))
			ON CONFLICT(item_id) DO UPDATE SET
				placa = excluded.placa,
				renavam = excluded.renavam,
				padrinho_titular_id = excluded.padrinho_titular_id,
				padrinho_substituto_id = excluded.padrinho_substituto_id,
				hodometro_atual = excluded.hodometro_atual,
				combustivel_atual = excluded.combustivel_atual,
				atualizado_em = strftime('%Y-%m-%dT%H:%M:%fZ','now')`,
			itemID, strings.TrimSpace(req.Placa), strings.TrimSpace(req.Renavam), padTit, padSub, req.HodometroAtual, comb)
		return err
	}

	if req.ID > 0 {
		resIt, err := a.st.db.Exec(`
			UPDATE material_itens
			SET setor_id = ?, categoria_id = ?, nome = ?, codigo_patrimonio = ?, numero_serie = ?, status = ?, observacao = ?, nivel_sensibilidade = ?, sensibilidade = ?, quantidade = ?
			WHERE id = ? AND (? = 0 OR grupo_id = ?)`,
			setorVal, req.CategoriaID, req.Nome, req.CodigoPatrimonio, req.NumeroSerie, req.Status, req.Observacao, req.NivelSensibilidade, req.Sensibilidade, req.Quantidade, req.ID, esc, grupoID)
		if err != nil {
			jsonErro(w, http.StatusInternalServerError, err.Error())
			return
		}
		// Fix P1-2: 200 sem efeito escondia edição fora do escopo — agora 404 honesto.
		if n, _ := resIt.RowsAffected(); n == 0 {
			jsonErro(w, http.StatusNotFound, "item não encontrado no seu escopo")
			return
		}
		if err := salvarViaturaSeNecessario(req.ID); err != nil {
			jsonErro(w, http.StatusInternalServerError, err.Error())
			return
		}
		a.st.Auditoria(&u.ID, "editar", "material_itens", &req.ID, req.Nome+" ("+req.CodigoPatrimonio+")", ipDe(r))
		jsonOK(w, map[string]any{"ok": true, "id": req.ID})
		return
	}

	res, err := a.st.db.Exec(`
		INSERT INTO material_itens (grupo_id, setor_id, categoria_id, nome, codigo_patrimonio, numero_serie, status, observacao, nivel_sensibilidade, sensibilidade, quantidade)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		grupoID, setorVal, req.CategoriaID, req.Nome, req.CodigoPatrimonio, req.NumeroSerie, req.Status, req.Observacao, req.NivelSensibilidade, req.Sensibilidade, req.Quantidade)
	if err != nil {
		jsonErro(w, http.StatusInternalServerError, err.Error())
		return
	}
	newID, _ := res.LastInsertId()
	if err := salvarViaturaSeNecessario(newID); err != nil {
		jsonErro(w, http.StatusInternalServerError, err.Error())
		return
	}
	a.st.Auditoria(&u.ID, "criar", "material_itens", &newID, req.Nome+" ("+req.CodigoPatrimonio+")", ipDe(r))
	jsonOK(w, map[string]any{"ok": true, "id": newID})
}

func (a *App) hMaterialItensDel(w http.ResponseWriter, r *http.Request) {
	u := usuarioDoCtx(r)
	idStr := r.PathValue("id")
	id, _ := strconv.ParseInt(idStr, 10, 64)
	if id <= 0 {
		jsonErro(w, http.StatusBadRequest, "ID inválido")
		return
	}

	var itemGrupoID int64
	var itemStatus, itemNome, codPatrimonio string
	err := a.st.db.QueryRow(`SELECT COALESCE(grupo_id,0), status, nome, codigo_patrimonio FROM material_itens WHERE id = ?`, id).
		Scan(&itemGrupoID, &itemStatus, &itemNome, &codPatrimonio)
	if err != nil {
		jsonErro(w, http.StatusNotFound, "Item não encontrado")
		return
	}

	esc, err := a.exigeEscopo(u)
	if err != nil {
		jsonErro(w, http.StatusForbidden, "conta sem grupo definido")
		return
	}
	if esc > 0 && itemGrupoID > 0 && itemGrupoID != esc {
		jsonErro(w, http.StatusForbidden, "Você não tem permissão para alterar itens de outro grupo")
		return
	}

	modo := r.URL.Query().Get("modo")

	// Modo "baixar" (desincorporar/aposentar patrimônio mantendo histórico)
	if modo == "baixar" {
		if itemStatus == "acautelado" {
			jsonErro(w, http.StatusBadRequest, "Não é possível baixar um item acautelado. Realize a devolução primeiro.")
			return
		}
		_, err = a.st.db.Exec(`UPDATE material_itens SET status = 'baixado' WHERE id = ?`, id)
		if err != nil {
			jsonErro(w, http.StatusInternalServerError, err.Error())
			return
		}
		a.st.Auditoria(&u.ID, "baixar_patrimonio", "material_itens", &id, fmt.Sprintf("%s (%s)", itemNome, codPatrimonio), ipDe(r))
		jsonOK(w, map[string]any{"ok": true, "acao": "baixado"})
		return
	}

	// Exclusão definitiva (remove item, histórico de cautelas e anexos em transação atômica)
	if itemStatus == "acautelado" {
		jsonErro(w, http.StatusBadRequest, "Não é possível excluir um item que está acautelado no momento. Realize a devolução primeiro.")
		return
	}

	tx, err := a.st.db.Begin()
	if err != nil {
		jsonErro(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer tx.Rollback()

	// 1. Apagar anexos de cautelas deste item
	_, err = tx.Exec(`DELETE FROM material_cautela_anexos WHERE cautela_id IN (SELECT id FROM material_cautelas WHERE item_id = ?)`, id)
	if err != nil {
		jsonErro(w, http.StatusInternalServerError, "Falha ao limpar anexos: "+err.Error())
		return
	}

	// 2. Apagar cautelas deste item
	_, err = tx.Exec(`DELETE FROM material_cautelas WHERE item_id = ?`, id)
	if err != nil {
		jsonErro(w, http.StatusInternalServerError, "Falha ao limpar cautelas: "+err.Error())
		return
	}

	// 3. Apagar o item em si
	_, err = tx.Exec(`DELETE FROM material_itens WHERE id = ?`, id)
	if err != nil {
		jsonErro(w, http.StatusInternalServerError, "Falha ao excluir item: "+err.Error())
		return
	}

	if err := tx.Commit(); err != nil {
		jsonErro(w, http.StatusInternalServerError, err.Error())
		return
	}

	a.st.Auditoria(&u.ID, "excluir", "material_itens", &id, fmt.Sprintf("%s (%s)", itemNome, codPatrimonio), ipDe(r))
	jsonOK(w, map[string]any{"ok": true, "acao": "excluido"})
}

func (a *App) hMaterialCautelar(w http.ResponseWriter, r *http.Request) {
	u := usuarioDoCtx(r)
	var req struct {
		ItemID     int64  `json:"item_id"`
		PessoaID   int64  `json:"pessoa_id"`
		Quantidade int    `json:"quantidade"`
		ObsSaida   string `json:"obs_saida"`
		Anexos     []struct {
			NomeArquivo string `json:"nome_arquivo"`
			TipoMIME    string `json:"tipo_mime"`
			Tamanho     int64  `json:"tamanho"`
			DadosBase64 string `json:"dados_base64"`
		} `json:"anexos"`
	}
	if err := decodificar(r, &req); err != nil || req.ItemID <= 0 || req.PessoaID <= 0 {
		jsonErro(w, http.StatusBadRequest, "Item e Pessoa são obrigatórios para cautela")
		return
	}
	if req.Quantidade <= 0 {
		req.Quantidade = 1
	}

	// Fix cia-F1: anexos INLINE do cautelar gravavam mime/nome CRUS, bypassando a
	// allowlist do endpoint dedicado (stored XSS latente no banco — BUGS_s3 S1-B1).
	// Mesma regra do hMaterialAnexoAdd: valida o slice INTEIRO ANTES de abrir a tx
	// (400 sem efeito colateral); dentro da tx, grava direto — erro ali = rollback.
	for i := range req.Anexos {
		anexo := &req.Anexos[i]
		if strings.TrimSpace(anexo.NomeArquivo) == "" || strings.TrimSpace(anexo.DadosBase64) == "" {
			continue // entrada incompleta: ignorada, como antes
		}
		mime := strings.ToLower(strings.TrimSpace(anexo.TipoMIME))
		switch mime {
		case "application/pdf", "image/png", "image/jpeg", "image/webp":
			// permitido
		default:
			jsonErro(w, http.StatusBadRequest, "tipo não permitido (use PDF, PNG, JPEG ou WEBP)")
			return
		}
		nome := sanitizarNomeArquivo(anexo.NomeArquivo)
		if nome == "" {
			jsonErro(w, http.StatusBadRequest, "nome de arquivo inválido")
			return
		}
		anexo.TipoMIME = mime
		anexo.NomeArquivo = nome
	}

	tx, err := a.st.db.Begin()
	if err != nil {
		jsonErro(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer tx.Rollback()

	var statusAtual, itemSensibilidade string
	var itemNome, codPatrimonio string
	var itemQtd int
	err = tx.QueryRow(`SELECT status, nome, codigo_patrimonio, COALESCE(sensibilidade, 'convencional'), COALESCE(quantidade, 1) FROM material_itens WHERE id = ?`, req.ItemID).Scan(&statusAtual, &itemNome, &codPatrimonio, &itemSensibilidade, &itemQtd)
	if err != nil {
		jsonErro(w, http.StatusNotFound, "Item não encontrado")
		return
	}
	if statusAtual != "disponivel" {
		jsonErro(w, http.StatusBadRequest, fmt.Sprintf("Item '%s' não está disponível (status atual: %s)", itemNome, statusAtual))
		return
	}

	if itemSensibilidade == "controlado" {
		req.Quantidade = 1
	} else {
		var somaAtiva int
		_ = tx.QueryRow(`SELECT COALESCE(SUM(quantidade), 0) FROM material_cautelas WHERE item_id = ? AND status = 'ativa'`, req.ItemID).Scan(&somaAtiva)
		disp := itemQtd - somaAtiva
		if req.Quantidade > disp {
			jsonErro(w, http.StatusBadRequest, fmt.Sprintf("Quantidade solicitada (%d) maior que o saldo disponível na reserva (%d)", req.Quantidade, disp))
			return
		}
	}

	// Fix P1-1: cautelar exige item do PRÓPRIO escopo (o id do corpo era aceito cru).
	// USA tx: o handler já segura a conexão única — query no pool aqui = deadlock.
	esc, err := a.exigeEscopo(u)
	if err != nil {
		jsonErro(w, http.StatusForbidden, "conta sem grupo definido")
		return
	}
	if esc > 0 {
		var itemGrupo int64
		if err := tx.QueryRow(`SELECT COALESCE(grupo_id,0) FROM material_itens WHERE id = ?`, req.ItemID).Scan(&itemGrupo); err != nil || itemGrupo != esc {
			jsonErro(w, http.StatusForbidden, "item fora do seu escopo")
			return
		}
	}

	dataSaida := time.Now().UTC().Format("2006-01-02T15:04:05.000Z")
	res, err := tx.Exec(`
		INSERT INTO material_cautelas (item_id, pessoa_id, responsavel_entrega_id, data_saida, obs_saida, status, quantidade)
		VALUES (?, ?, ?, ?, ?, 'ativa', ?)`,
		req.ItemID, req.PessoaID, u.ID, dataSaida, req.ObsSaida, req.Quantidade)
	if err != nil {
		jsonErro(w, http.StatusInternalServerError, err.Error())
		return
	}
	cautelaID, _ := res.LastInsertId()

	if itemSensibilidade == "controlado" {
		_, err = tx.Exec(`UPDATE material_itens SET status = 'acautelado' WHERE id = ?`, req.ItemID)
	} else {
		var somaPos int
		_ = tx.QueryRow(`SELECT COALESCE(SUM(quantidade), 0) FROM material_cautelas WHERE item_id = ? AND status = 'ativa'`, req.ItemID).Scan(&somaPos)
		if somaPos >= itemQtd {
			_, err = tx.Exec(`UPDATE material_itens SET status = 'acautelado' WHERE id = ?`, req.ItemID)
		} else {
			_, err = tx.Exec(`UPDATE material_itens SET status = 'disponivel' WHERE id = ?`, req.ItemID)
		}
	}
	if err != nil {
		jsonErro(w, http.StatusInternalServerError, err.Error())
		return
	}

	for _, anexo := range req.Anexos {
		if strings.TrimSpace(anexo.NomeArquivo) != "" && strings.TrimSpace(anexo.DadosBase64) != "" {
			// Já validado/sanitizado antes da tx (cia-F1); falha aqui = rollback.
			if _, err := tx.Exec(`
				INSERT INTO material_cautela_anexos (cautela_id, nome_arquivo, tipo_mime, tamanho, dados_base64)
				VALUES (?, ?, ?, ?, ?)`,
				cautelaID, anexo.NomeArquivo, anexo.TipoMIME, anexo.Tamanho, anexo.DadosBase64); err != nil {
				jsonErro(w, http.StatusInternalServerError, err.Error())
				return
			}
		}
	}

	if err := tx.Commit(); err != nil {
		jsonErro(w, http.StatusInternalServerError, err.Error())
		return
	}
	a.st.Auditoria(&u.ID, "cautelar", "material_cautelas", &cautelaID,
		fmt.Sprintf("item=%s (%s) qtd=%d pessoa_id=%d anexos=%d", itemNome, codPatrimonio, req.Quantidade, req.PessoaID, len(req.Anexos)), ipDe(r))
	jsonOK(w, map[string]any{"ok": true, "cautela_id": cautelaID})
}

func (a *App) hMaterialDevolver(w http.ResponseWriter, r *http.Request) {
	u := usuarioDoCtx(r)
	var req struct {
		CautelaID    *int64 `json:"cautela_id"`
		ItemID       *int64 `json:"item_id"`
		ObsDevolucao string `json:"obs_devolucao"`
		Quantidade   int    `json:"quantidade"`
	}
	if err := decodificar(r, &req); err != nil || (req.CautelaID == nil && req.ItemID == nil) {
		jsonErro(w, http.StatusBadRequest, "Informe cautela_id ou item_id para devolução")
		return
	}

	tx, err := a.st.db.Begin()
	if err != nil {
		jsonErro(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer tx.Rollback()

	var cautelaID int64
	var itemID int64
	var cautelaQtd int
	var cautelaPessoa int64
	var cautelaRespEnt int64
	var cautelaDataSaida string
	var cautelaObsSaida string
	if req.CautelaID != nil && *req.CautelaID > 0 {
		cautelaID = *req.CautelaID
		err = tx.QueryRow(`
			SELECT item_id, pessoa_id, responsavel_entrega_id, data_saida, COALESCE(obs_saida,''), COALESCE(quantidade, 1)
			FROM material_cautelas WHERE id = ? AND status = 'ativa'`, cautelaID).
			Scan(&itemID, &cautelaPessoa, &cautelaRespEnt, &cautelaDataSaida, &cautelaObsSaida, &cautelaQtd)
	} else if req.ItemID != nil && *req.ItemID > 0 {
		itemID = *req.ItemID
		err = tx.QueryRow(`
			SELECT id, pessoa_id, responsavel_entrega_id, data_saida, COALESCE(obs_saida,''), COALESCE(quantidade, 1)
			FROM material_cautelas WHERE item_id = ? AND status = 'ativa' ORDER BY id DESC LIMIT 1`, itemID).
			Scan(&cautelaID, &cautelaPessoa, &cautelaRespEnt, &cautelaDataSaida, &cautelaObsSaida, &cautelaQtd)
	}
	if err != nil {
		jsonErro(w, http.StatusNotFound, "Cautela ativa não encontrada para este item")
		return
	}

	// Fix P1-1: devolução por ID cru exigia escopo — a cautela precisa ser do grupo
	// do próprio item, e o item do escopo do usuário. USA tx (conexão já presa:
	// query no pool aqui = deadlock, pego pela suíte).
	{
		esc, err := a.exigeEscopo(u)
		if err != nil {
			jsonErro(w, http.StatusForbidden, "conta sem grupo definido")
			return
		}
		var itemGrupo int64
		if err := tx.QueryRow(`SELECT COALESCE(grupo_id,0) FROM material_itens WHERE id = ?`, itemID).Scan(&itemGrupo); err == nil {
			if esc > 0 && itemGrupo != esc {
				jsonErro(w, http.StatusForbidden, "item fora do seu escopo")
				return
			}
		}
	}

	dataDevolucao := time.Now().UTC().Format("2006-01-02T15:04:05.000Z")
	if req.Quantidade <= 0 || req.Quantidade >= cautelaQtd {
		// Devolução integral
		_, err = tx.Exec(`
			UPDATE material_cautelas
			SET status = 'devolvida', data_devolucao = ?, responsavel_recebimento_id = ?, obs_devolucao = ?
			WHERE id = ?`,
			dataDevolucao, u.ID, req.ObsDevolucao, cautelaID)
		if err != nil {
			jsonErro(w, http.StatusInternalServerError, err.Error())
			return
		}
	} else {
		// Devolução parcial (ex.: devolvendo 3 de 10)
		_, err = tx.Exec(`UPDATE material_cautelas SET quantidade = quantidade - ? WHERE id = ?`, req.Quantidade, cautelaID)
		if err != nil {
			jsonErro(w, http.StatusInternalServerError, err.Error())
			return
		}
		_, err = tx.Exec(`
			INSERT INTO material_cautelas (item_id, pessoa_id, responsavel_entrega_id, responsavel_recebimento_id, data_saida, data_devolucao, obs_saida, obs_devolucao, status, quantidade)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, 'devolvida', ?)`,
			itemID, cautelaPessoa, cautelaRespEnt, u.ID, cautelaDataSaida, dataDevolucao, cautelaObsSaida, req.ObsDevolucao, req.Quantidade)
		if err != nil {
			jsonErro(w, http.StatusInternalServerError, err.Error())
			return
		}
	}

	var itemSensibilidade string
	var itemQtd int
	_ = tx.QueryRow(`SELECT COALESCE(sensibilidade, 'convencional'), COALESCE(quantidade, 1) FROM material_itens WHERE id = ?`, itemID).Scan(&itemSensibilidade, &itemQtd)
	var somaAtiva int
	_ = tx.QueryRow(`SELECT COALESCE(SUM(quantidade), 0) FROM material_cautelas WHERE item_id = ? AND status = 'ativa'`, itemID).Scan(&somaAtiva)
	if somaAtiva < itemQtd {
		_, err = tx.Exec(`UPDATE material_itens SET status = 'disponivel' WHERE id = ?`, itemID)
		if err != nil {
			jsonErro(w, http.StatusInternalServerError, err.Error())
			return
		}
	}

	if err := tx.Commit(); err != nil {
		jsonErro(w, http.StatusInternalServerError, err.Error())
		return
	}
	a.st.Auditoria(&u.ID, "devolver", "material_cautelas", &cautelaID,
		fmt.Sprintf("item_id=%d qtd=%d obs=%s", itemID, req.Quantidade, req.ObsDevolucao), ipDe(r))
	jsonOK(w, map[string]any{"ok": true, "cautela_id": cautelaID})
}

func (a *App) hMaterialCautelasList(w http.ResponseWriter, r *http.Request) {
	u := usuarioDoCtx(r)
	escopo, err := a.exigeEscopo(u)
	if err != nil {
		jsonErro(w, http.StatusForbidden, "conta sem grupo definido")
		return
	}
	statusQ := r.URL.Query().Get("status")
	pessoaQ := r.URL.Query().Get("pessoa_id")
	itemQ := r.URL.Query().Get("item_id")

	q := `
		SELECT mc.id, mc.item_id, mi.nome, mi.codigo_patrimonio,
		       COALESCE(mc.pessoa_id, 0), COALESCE(p.nome_guerra, ''), COALESCE(p.nome_completo, ''),
		       COALESCE(mc.responsavel_entrega_id, 0), COALESCE(ue.login, ''),
		       COALESCE(mc.responsavel_recebimento_id, 0), COALESCE(ur.login, ''),
		       mc.data_saida, COALESCE(mc.data_devolucao, ''),
		       COALESCE(mc.obs_saida, ''), COALESCE(mc.obs_devolucao, ''),
		       mc.status, COALESCE(mc.quantidade, 1), COALESCE(mi.sensibilidade, 'convencional')
		FROM material_cautelas mc
		JOIN material_itens mi ON mi.id = mc.item_id
		LEFT JOIN pessoas p ON p.id = mc.pessoa_id
		LEFT JOIN usuarios ue ON ue.id = mc.responsavel_entrega_id
		LEFT JOIN usuarios ur ON ur.id = mc.responsavel_recebimento_id
		WHERE (? = 0 OR mi.grupo_id = ?)`
	args := []any{escopo, escopo}

	if statusQ != "" {
		q += ` AND mc.status = ?`
		args = append(args, statusQ)
	}
	if pessoaQ != "" {
		if pid, err := strconv.ParseInt(pessoaQ, 10, 64); err == nil && pid > 0 {
			q += ` AND mc.pessoa_id = ?`
			args = append(args, pid)
		}
	}
	if itemQ != "" {
		if itm, err := strconv.ParseInt(itemQ, 10, 64); err == nil && itm > 0 {
			q += ` AND mc.item_id = ?`
			args = append(args, itm)
		}
	}
	q += ` ORDER BY mc.id DESC LIMIT 200`

	rows, err := a.st.db.Query(q, args...)
	if err != nil {
		jsonErro(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer rows.Close()

	var lista []map[string]any
	for rows.Next() {
		var cid, iid, pid, respEnt, respRec int64
		var iNome, iCod, pGuerra, pCompleto, loginEnt, loginRec, dtSaida, dtDev, obsS, obsD, st, sens string
		var mcQtd int
		if err := rows.Scan(&cid, &iid, &iNome, &iCod, &pid, &pGuerra, &pCompleto,
			&respEnt, &loginEnt, &respRec, &loginRec, &dtSaida, &dtDev, &obsS, &obsD, &st, &mcQtd, &sens); err == nil {
			lista = append(lista, map[string]any{
				"id":                      cid,
				"item_id":                 iid,
				"item_nome":               iNome,
				"codigo_patrimonio":       iCod,
				"pessoa_id":               pid,
				"pessoa_nome_guerra":      pGuerra,
				"pessoa_nome_completo":    pCompleto,
				"responsavel_entrega_id":  respEnt,
				"responsavel_entrega":     loginEnt,
				"responsavel_recebimento": loginRec,
				"data_saida":              dtSaida,
				"data_devolucao":          dtDev,
				"obs_saida":               obsS,
				"obs_devolucao":           obsD,
				"status":                  st,
				"quantidade":              mcQtd,
				"sensibilidade":           sens,
			})
		}
	}
	jsonOK(w, map[string]any{"cautelas": lista})
}

func escopoCautelaID(db *sql.DB, cautelaID int64) (int64, error) {
	var itemGrupo int64
	err := db.QueryRow(`SELECT COALESCE(mi.grupo_id,0)
		FROM material_cautelas mc JOIN material_itens mi ON mi.id = mc.item_id
		WHERE mc.id = ?`, cautelaID).Scan(&itemGrupo)
	return itemGrupo, err
}

func (a *App) hMaterialAnexoAdd(w http.ResponseWriter, r *http.Request) {
	u := usuarioDoCtx(r)
	cautelaIDStr := r.PathValue("id")
	cautelaID, _ := strconv.ParseInt(cautelaIDStr, 10, 64)
	if cautelaID <= 0 {
		jsonErro(w, http.StatusBadRequest, "ID da cautela inválido")
		return
	}

	var req struct {
		NomeArquivo string `json:"nome_arquivo"`
		TipoMIME    string `json:"tipo_mime"`
		Tamanho     int64  `json:"tamanho"`
		DadosBase64 string `json:"dados_base64"`
	}
	if err := decodificar(r, &req); err != nil || strings.TrimSpace(req.NomeArquivo) == "" || strings.TrimSpace(req.DadosBase64) == "" {
		jsonErro(w, http.StatusBadRequest, "Nome do arquivo e dados em base64 são obrigatórios")
		return
	}
	// Fix cia-F7: cautela inexistente era 500 FK cru — pre-check resolve (404) e
	// junto com o escopo (mesmo bloco de antes, agora via helper cia-F2).
	itemGrupo, err := escopoCautelaID(a.st.db, cautelaID)
	if err != nil {
		jsonErro(w, http.StatusNotFound, "Cautela não encontrada")
		return
	}
	esc, err := a.exigeEscopo(u)
	if err != nil {
		jsonErro(w, http.StatusForbidden, "conta sem grupo definido")
		return
	}
	if esc > 0 && itemGrupo != esc {
		jsonErro(w, http.StatusForbidden, "cautela fora do seu escopo")
		return
	}
	// Fix P1-3: teto REAL de anexo — o LimitReader de 1 MB corta o JSON inteiro;
	// base64 cresce ~4/3, então o DECODED útil máximo aqui é ~600 KB.
	const maxAnexoBase64 = 800 * 1024 // 800 KB de base64 ≈ 600 KB de arquivo
	if len(req.DadosBase64) > maxAnexoBase64 {
		jsonErro(w, http.StatusRequestEntityTooLarge, "anexo acima do teto (máx. ~600 KB)")
		return
	}
	// Fix P1-1 (rodada 04/10): allowlist de MIME na entrada — anexo com tipo livre
	// (text/html, image/svg+xml…) servido pela origem é stored XSS (CSP não salva:
	// tem unsafe-inline). Fora da allowlist → 400.
	mime := strings.ToLower(strings.TrimSpace(req.TipoMIME))
	switch mime {
	case "application/pdf", "image/png", "image/jpeg", "image/webp":
		// permitido
	default:
		jsonErro(w, http.StatusBadRequest, "tipo não permitido (use PDF, PNG, JPEG ou WEBP)")
		return
	}
	nome := sanitizarNomeArquivo(req.NomeArquivo)
	if nome == "" {
		jsonErro(w, http.StatusBadRequest, "nome de arquivo inválido")
		return
	}
	res, err := a.st.db.Exec(`
		INSERT INTO material_cautela_anexos (cautela_id, nome_arquivo, tipo_mime, tamanho, dados_base64)
		VALUES (?, ?, ?, ?, ?)`,
		cautelaID, nome, mime, req.Tamanho, req.DadosBase64)
	if err != nil {
		jsonErro(w, http.StatusInternalServerError, err.Error())
		return
	}
	newID, _ := res.LastInsertId()
	a.st.Auditoria(&u.ID, "anexar_documento", "material_cautela_anexos", &newID,
		fmt.Sprintf("cautela=%d arquivo=%s", cautelaID, nome), ipDe(r))
	jsonOK(w, map[string]any{"ok": true, "id": newID, "nome_arquivo": nome})
}

func sanitizarNomeArquivo(nome string) string {
	nome = strings.ReplaceAll(nome, "\\", "/")
	nome = filepath.Base(nome)
	nome = strings.Map(func(r rune) rune {
		if r == '"' || r == '\'' || r < 0x20 || r == 0x7f {
			return -1
		}
		return r
	}, nome)
	nome = strings.TrimSpace(nome)
	if len(nome) > 120 {
		nome = nome[:120]
	}
	return nome
}

func (a *App) hMaterialAnexoList(w http.ResponseWriter, r *http.Request) {
	u := usuarioDoCtx(r)
	cautelaIDStr := r.PathValue("id")
	cautelaID, _ := strconv.ParseInt(cautelaIDStr, 10, 64)
	if cautelaID <= 0 {
		jsonErro(w, http.StatusBadRequest, "ID da cautela inválido")
		return
	}
	// Fix cia-F2 (IDOR): listagem de anexos sem checagem de escopo vazava
	// metadados de cautela alheia.
	if !a.cautelaNoEscopo(u, w, cautelaID) {
		return
	}
	rows, err := a.st.db.Query(`
		SELECT id, cautela_id, nome_arquivo, tipo_mime, tamanho, criado_em
		FROM material_cautela_anexos
		WHERE cautela_id = ?
		ORDER BY id`, cautelaID)
	if err != nil {
		jsonErro(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer rows.Close()
	var lista []map[string]any
	if lista == nil {
		lista = make([]map[string]any, 0) // fix cia-F9: nil marshaliza null — contrato front
	}
	for rows.Next() {
		var id, cid, tam int64
		var nome, mime, criada string
		if rows.Scan(&id, &cid, &nome, &mime, &tam, &criada) == nil {
			lista = append(lista, map[string]any{
				"id":           id,
				"cautela_id":   cid,
				"nome_arquivo": nome,
				"tipo_mime":    mime,
				"tamanho":      tam,
				"criado_em":    criada,
			})
		}
	}
	jsonOK(w, map[string]any{"anexos": lista})
}

func (a *App) hMaterialAnexoGet(w http.ResponseWriter, r *http.Request) {
	u := usuarioDoCtx(r)
	anexoIDStr := r.PathValue("id")
	anexoID, _ := strconv.ParseInt(anexoIDStr, 10, 64)
	if anexoID <= 0 {
		jsonErro(w, http.StatusBadRequest, "ID inválido")
		return
	}
	// Fix cia-F2 (IDOR): download de anexo sem escopo entregava BYTES do PDF
	// do grupo alheio. Escopo via cautela_id da tabela de anexos.
	var cautelaID int64
	if err := a.st.db.QueryRow(`SELECT cautela_id FROM material_cautela_anexos WHERE id = ?`, anexoID).Scan(&cautelaID); err != nil {
		jsonErro(w, http.StatusNotFound, "Documento anexo não encontrado")
		return
	}
	if !a.cautelaNoEscopo(u, w, cautelaID) {
		return
	}
	var nome, mime, b64 string
	err := a.st.db.QueryRow(`
		SELECT nome_arquivo, tipo_mime, dados_base64
		FROM material_cautela_anexos WHERE id = ?`, anexoID).Scan(&nome, &mime, &b64)
	if err != nil {
		jsonErro(w, http.StatusNotFound, "Documento anexo não encontrado")
		return
	}

	// Remove data URL prefix if present (e.g. data:image/png;base64,...)
	if idx := strings.Index(b64, ","); idx != -1 {
		b64 = b64[idx+1:]
	}

	dados, err := base64.StdEncoding.DecodeString(b64)
	if err != nil {
		jsonErro(w, http.StatusInternalServerError, "Falha ao decodificar arquivo")
		return
	}

	if mime == "" {
		mime = "application/octet-stream"
	}
	// Fix P1-1 defesa em profundidade (rodada 04/10): o download é superfície própria —
	// o banco pode ter sido poblado por outro caminho com mime hostil (legado). Só
	// servimos mime da allowlist; fora dela (ou vazio), octet-stream + attachment
	// NUNCA inline — stored XSS na própria origem morre aqui independente do upload.
	switch mime {
	case "application/pdf", "image/png", "image/jpeg", "image/webp":
		// mime confiável
	default:
		mime = "application/octet-stream"
	}
	w.Header().Set("Content-Type", mime)
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", sanitizarNomeArquivo(nome)))
	w.Header().Set("Content-Length", strconv.Itoa(len(dados)))
	_, _ = w.Write(dados)
}

func (a *App) hMaterialAnexoDel(w http.ResponseWriter, r *http.Request) {
	u := usuarioDoCtx(r)
	anexoIDStr := r.PathValue("id")
	anexoID, _ := strconv.ParseInt(anexoIDStr, 10, 64)
	if anexoID <= 0 {
		jsonErro(w, http.StatusBadRequest, "ID inválido")
		return
	}
	// Fix cia-F2 (IDOR): DELETE sem escopo permitia DESTRUIÇÃO cross-group
	// (op_b apagou anexo alheio no play test). Validar ANTES do DELETE.
	var cautelaID int64
	if err := a.st.db.QueryRow(`SELECT cautela_id FROM material_cautela_anexos WHERE id = ?`, anexoID).Scan(&cautelaID); err != nil {
		jsonErro(w, http.StatusNotFound, "Documento anexo não encontrado")
		return
	}
	if !a.cautelaNoEscopo(u, w, cautelaID) {
		return
	}
	res, err := a.st.db.Exec(`DELETE FROM material_cautela_anexos WHERE id = ?`, anexoID)
	if err != nil {
		jsonErro(w, http.StatusInternalServerError, err.Error())
		return
	}
	if n, _ := res.RowsAffected(); n == 0 {
		jsonErro(w, http.StatusNotFound, "Documento anexo não encontrado")
		return
	}
	a.st.Auditoria(&u.ID, "excluir_anexo", "material_cautela_anexos", &anexoID, "", ipDe(r))
	jsonOK(w, map[string]any{"ok": true})
}

func (a *App) hMaterialItemQRCode(w http.ResponseWriter, r *http.Request) {
	u := usuarioDoCtx(r)
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id <= 0 {
		jsonErro(w, http.StatusBadRequest, "ID de material inválido")
		return
	}
	var codPatrimonio, nome string
	var gid int64
	err = a.st.db.QueryRow(`SELECT codigo_patrimonio, nome, grupo_id FROM material_itens WHERE id = ?`, id).Scan(&codPatrimonio, &nome, &gid)
	if err != nil {
		jsonErro(w, http.StatusNotFound, "Material não encontrado")
		return
	}
	esc, err := a.exigeEscopo(u)
	if err != nil {
		jsonErro(w, http.StatusForbidden, "conta sem grupo definido")
		return
	}
	if esc > 0 && gid != esc {
		jsonErro(w, http.StatusForbidden, "Acesso restrito ao grupo")
		return
	}
	payload := fmt.Sprintf("sci://m:%d:%s", id, codPatrimonio)
	qr, err := GerarQRCode(payload)
	if err != nil {
		jsonErro(w, http.StatusInternalServerError, "Erro ao gerar QR Code: "+err.Error())
		return
	}
	if r.URL.Query().Get("format") == "svg" {
		w.Header().Set("Content-Type", "image/svg+xml")
		_ = qr.RenderSVG(w, 256)
		return
	}
	pngData, err := qr.RenderPNG(8, 4)
	if err != nil {
		jsonErro(w, http.StatusInternalServerError, "Erro ao renderizar PNG: "+err.Error())
		return
	}
	w.Header().Set("Content-Type", "image/png")
	w.Header().Set("Content-Length", strconv.Itoa(len(pngData)))
	_, _ = w.Write(pngData)
}

func (a *App) hMaterialEtiquetasLotePDF(w http.ResponseWriter, r *http.Request) {
	if a.reservaAtivo() {
		jsonErro(w, http.StatusLocked, "módulo em reserva (indisponível nesta instalação)")
		return
	}
	u := usuarioDoCtx(r)
	escopo, err := a.exigeEscopo(u)
	if err != nil {
		jsonErro(w, http.StatusForbidden, "conta sem grupo definido")
		return
	}

	idsParam := r.URL.Query().Get("ids")
	var idList []int64
	if idsParam != "" {
		for _, s := range strings.Split(idsParam, ",") {
			s = strings.TrimSpace(s)
			if id, err := strconv.ParseInt(s, 10, 64); err == nil && id > 0 {
				idList = append(idList, id)
			}
		}
	}

	q := `SELECT mi.id, mi.nome, mi.codigo_patrimonio, COALESCE(cat.nome, 'Geral'),
	             COALESCE(mi.numero_serie, ''), COALESCE(mi.nivel_sensibilidade, 'padrao'),
	             COALESCE(mi.tipo_material, ''), COALESCE(mi.classe_material, '')
	      FROM material_itens mi
	      LEFT JOIN material_categorias cat ON cat.id = mi.categoria_id
	      WHERE 1=1`
	var args []any
	if escopo > 0 {
		q += ` AND mi.grupo_id = ?`
		args = append(args, escopo)
	}
	if len(idList) > 0 {
		ph := strings.TrimSuffix(strings.Repeat("?,", len(idList)), ",")
		q += ` AND mi.id IN (` + ph + `)`
		for _, id := range idList {
			args = append(args, id)
		}
	}
	q += ` ORDER BY mi.codigo_patrimonio ASC, mi.nome ASC`

	rows, err := a.st.db.Query(q, args...)
	if err != nil {
		jsonErro(w, http.StatusInternalServerError, "falha ao buscar itens de material: "+err.Error())
		return
	}
	defer rows.Close()

	var itens []MaterialItemEtiqueta
	for rows.Next() {
		var it MaterialItemEtiqueta
		if err := rows.Scan(&it.ID, &it.Nome, &it.CodigoPatrimonio, &it.CategoriaNome,
			&it.NumeroSerie, &it.NivelSensibilidade, &it.TipoMaterial, &it.ClasseMaterial); err == nil {
			itens = append(itens, it)
		}
	}

	if len(itens) == 0 {
		jsonErro(w, http.StatusNotFound, "nenhum item selecionado ou encontrado")
		return
	}

	pdfBytes, err := a.gerarEtiquetasLotePDF(itens)
	if err != nil {
		jsonErro(w, http.StatusInternalServerError, "erro ao gerar etiquetas em PDF: "+err.Error())
		return
	}

	w.Header().Set("Content-Type", "application/pdf")
	w.Header().Set("Content-Disposition", `inline; filename="etiquetas_material_lote.pdf"`)
	w.Header().Set("Content-Length", strconv.Itoa(len(pdfBytes)))
	_, _ = w.Write(pdfBytes)
}

// authMaterial: middleware de acesso ao módulo Material.
// Se a reserva operacional estiver ativa, responde 423 Locked.
// Permite gerente, operador E encarregado/auxiliar de material
// (designação por chave 'enc_material'). Admin mantém o comportamento de reservaAuth.
func (a *App) authMaterial(next http.HandlerFunc) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if a.reservaAtivo() {
			jsonErro(w, http.StatusLocked, "módulo em reserva operacional")
			return
		}
		a.auth(false, func(w http.ResponseWriter, r *http.Request) {
			u := usuarioDoCtx(r)
			if u == nil {
				jsonErro(w, http.StatusUnauthorized, "não autenticado")
				return
			}
			if u.Papel == "gerente" || u.Papel == "operador" || a.ehEncarregadoDeMaterial(u) {
				next(w, r)
				return
			}
			http.Error(w, `{"erro":"papel sem acesso a esta área"}`, http.StatusForbidden)
		}).ServeHTTP(w, r)
	})
}

// ---------- rotas rotasMaterial ----------
func (a *App) rotasMaterial() {
	m := a.mux

	// Módulo de Material e Cautelas (v1.0)
	m.Handle("GET /api/material/categorias", a.authMaterial(a.hMaterialCategoriasList))
	m.Handle("POST /api/material/categorias", a.authMaterial(a.hMaterialCategoriasAdd))
	m.Handle("DELETE /api/material/categorias/{id}", a.authMaterial(a.hMaterialCategoriasDel))
	m.Handle("GET /api/material/itens", a.authMaterial(a.hMaterialItensList))
	m.Handle("POST /api/material/itens", a.authMaterial(a.hMaterialItensSave))
	m.Handle("DELETE /api/material/itens/{id}", a.authMaterial(a.hMaterialItensDel))
	m.Handle("GET /api/material/itens/{id}/qr", a.authMaterial(a.hMaterialItemQRCode))
	m.Handle("GET /api/material/etiquetas-lote.pdf", a.authMaterial(a.hMaterialEtiquetasLotePDF))
	// v1.5.4-D2 (R-2): os 3 PDFs do módulo que estavam auth(false) (com checagem
	// parcial interna) passam pelo MESMO guard das rotas de dados — authMaterial
	// (gerente/operador/enc_material; admin 403; 423 em reserva). O recorte de
	// escopo segue dentro de cada handler (exigeEscopo + objeto no escopo).
	m.Handle("GET /api/material/inventario/pdf", a.authMaterial(a.hMaterialInventarioPDF))
	m.Handle("POST /api/material/cautelar", a.authMaterial(a.hMaterialCautelar))
	m.Handle("POST /api/material/devolver", a.authMaterial(a.hMaterialDevolver))
	m.Handle("GET /api/material/cautelas", a.authMaterial(a.hMaterialCautelasList))
	// v1.5.4-D2 (R-2): recibo de cautela sai do auth(false) pelado — mesmo
	// guard do módulo (o handler já exige cautela no escopo do solicitante).
	m.Handle("GET /api/material/cautelas/{id}/recibo.pdf", a.authMaterial(a.hMaterialCautelaReciboPDF))
	m.Handle("POST /api/material/cautelas/{id}/anexos", a.authMaterial(a.hMaterialAnexoAdd))
	m.Handle("GET /api/material/cautelas/{id}/anexos", a.authMaterial(a.hMaterialAnexoList))
	m.Handle("GET /api/material/anexos/{id}", a.authMaterial(a.hMaterialAnexoGet))
	m.Handle("DELETE /api/material/anexos/{id}", a.authMaterial(a.hMaterialAnexoDel))

	// Material v1.5 (feat/v1.5-evolucao): responsáveis, anexos por item, comentários, conferência diária
	m.Handle("GET /api/material/responsaveis", a.authMaterial(a.hMaterialResponsaveisList))
	m.Handle("POST /api/material/responsaveis", a.authMaterial(a.hMaterialResponsaveisSave))
	m.Handle("GET /api/material/itens/{id}/anexos", a.authMaterial(a.hMaterialItemAnexosList))
	m.Handle("POST /api/material/itens/{id}/anexos", a.authMaterial(a.hMaterialItemAnexoAdd))
	m.Handle("GET /api/material/item-anexos/{anexo_id}", a.authMaterial(a.hMaterialItemAnexoGet))
	m.Handle("DELETE /api/material/item-anexos/{anexo_id}", a.authMaterial(a.hMaterialItemAnexoDel))
	m.Handle("GET /api/material/itens/{id}/comentarios", a.authMaterial(a.hMaterialItemComentariosList))
	m.Handle("POST /api/material/itens/{id}/comentarios", a.authMaterial(a.hMaterialItemComentarioAdd))
	m.Handle("GET /api/material/conferencias", a.authMaterial(a.hMaterialConferenciasList))
	m.Handle("POST /api/material/conferencias", a.authMaterial(a.hMaterialConferenciasList))
	m.Handle("POST /api/material/conferencias/iniciar", a.authMaterial(a.hMaterialConferenciaIniciar))
	m.Handle("GET /api/material/conferencias/{id}", a.authMaterial(a.hMaterialConferenciaGet))
	m.Handle("POST /api/material/conferencias/{id}/bipar", a.authMaterial(a.hMaterialConferenciaBipar))
	m.Handle("POST /api/material/conferencias/{id}/fechar", a.authMaterial(a.hMaterialConferenciaFechar))
	// v1.5.4-D2 (R-2): o "pronto.pdf" era a pior lacuna (auth(false) sem guard
	// de papel) — agora pede o guard do módulo; escopo da conferência segue
	// cobrado dentro do handler (grupo do solicitante).
	m.Handle("GET /api/material/conferencias/{id}/pronto.pdf", a.authMaterial(a.hMaterialConferenciaPDF))
}
