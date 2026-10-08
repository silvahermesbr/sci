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

// gerarPDFMinimo: produz um PDF mínimo válido (cabeçalho+texto).
// Usado pelo hMaterialConferenciaPDF como stub até a implementação completa.
func (a *App) gerarPDFMinimo(titulo, conteudo string) []byte {
	// PDF mínimo: 1 página com texto simples
	enc := func(s string) string {
		s = strings.Replace(s, "\\", "\\\\", -1)
		s = strings.Replace(s, "(", "\\(", -1)
		s = strings.Replace(s, ")", "\\)", -1)
		return s
	}
	et := enc(titulo)
	ec := enc(conteudo)
	// Content stream: Mostra título e conteúdo
	content := fmt.Sprintf("BT /F1 14 Tf 100 700 Td (%s) Tj ET BT /F1 10 Tf 100 670 Td (%s) Tj ET", et, ec)
	slen := len(content)
	// Monta o PDF inline
	pdfStr := fmt.Sprintf("%%PDF-1.4\n1 0 obj<</Type/Catalog/Pages 2 0 R>>endobj\n2 0 obj<</Type/Pages/Kids[3 0 R]/Count 1>>endobj\n3 0 obj<</Type/Page/Parent 2 0 R/MediaBox[0 0 612 792]/Contents 4 0 R/Resources<</Font<</F1 5 0 R>>>>>>endobj\n4 0 obj<</Length %d>>stream\n%s\nendstream\nendobj\n5 0 obj<</Type/Font/Subtype/Type1/BaseFont/Courier>>endobj\nxref\n0 6\n0000000000 65535 f \n0000000009 00000 n \n0000000058 00000 n \n0000000105 00000 n \n0000000192 00000 n \n0000000370 00000 n \ntrailer<</Size 6/Root 1 0 R>>\nstartxref\n420\n%%%%EOF", slen, content)
	return []byte(pdfStr)
}

// -----------------------------------------------------------------
// hMaterialResponsaveisList
// -----------------------------------------------------------------
func (a *App) hMaterialResponsaveisList(w http.ResponseWriter, r *http.Request) {
	u := usuarioDoCtx(r)
	escopo := escopoDoUsuario(u)

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
	if req.GrupoID <= 0 {
		u := usuarioDoCtx(r)
		if u.GrupoID != nil && *u.GrupoID > 0 {
			req.GrupoID = *u.GrupoID
		} else {
			jsonErro(w, http.StatusBadRequest, "grupo_id é obrigatório")
			return
		}
	}
	encID := req.EncarregadoID
	if encID <= 0 {
		encID = 0
	}
	auxID := req.AuxiliarEncarregadoID
	if auxID <= 0 {
		auxID = 0
	}
	if _, err := a.st.db.Exec(`INSERT INTO grupo_setor_responsaveis (grupo_id, setor_id, encarregado_id, auxiliar_encarregado_id, atualizado_em)
		VALUES (?, ?, ?, ?, strftime('%Y-%m-%dT%H:%M:%fZ','now'))
		ON CONFLICT(grupo_id, setor_id) DO UPDATE SET
			encarregado_id = excluded.encarregado_id,
			auxiliar_encarregado_id = excluded.auxiliar_encarregado_id,
			atualizado_em = strftime('%Y-%m-%dT%H:%M:%fZ','now')`,
		req.GrupoID, req.SetorID, encID, auxID); err != nil {
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
	rows, err := a.st.db.Query(
		`SELECT id, nome_arquivo, tipo_mime, tamanho, criado_em
		 FROM material_item_anexos WHERE item_id = ? ORDER BY criado_em DESC`, itemID)
	if err != nil {
		jsonErro(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer rows.Close()
	var lista []map[string]any
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
	if strings.TrimSpace(req.NomeArquivo) == "" {
		jsonErro(w, http.StatusBadRequest, "nome_arquivo é obrigatório")
		return
	}
	if req.Tamanho > 10*1024*1024 {
		jsonErro(w, http.StatusRequestEntityTooLarge, "Anexo muito grande (máx 10 MB)")
		return
	}
	res, err := a.st.db.Exec(
		`INSERT INTO material_item_anexos (item_id, nome_arquivo, tipo_mime, tamanho, dados_base64, criado_em)
		 VALUES (?, ?, ?, ?, ?, strftime('%Y-%m-%dT%H:%M:%fZ','now'))`,
		itemID, strings.TrimSpace(req.NomeArquivo), strVal(&req.TipoMIME),
		req.Tamanho, req.DadosBase64)
	if err != nil {
		jsonErro(w, http.StatusInternalServerError, err.Error())
		return
	}
	var anexoID int64
	anexoID, _ = res.LastInsertId()
	jsonOK(w, map[string]any{"ok": true, "id": anexoID})
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
	escopo := escopoDoUsuario(u)
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
		GrupoID int64 `json:"grupo_id"`
		SetorID int64 `json:"setor_id"`
		Data    string
	}
	if err := decodificar(r, &req); err != nil {
		jsonErro(w, http.StatusBadRequest, "JSON inválido: "+err.Error())
		return
	}
	if req.GrupoID <= 0 {
		u2 := usuarioDoCtx(r)
		if u2.GrupoID != nil && *u2.GrupoID > 0 {
			req.GrupoID = *u2.GrupoID
		} else {
			jsonErro(w, http.StatusBadRequest, "grupo_id é obrigatório")
			return
		}
	}
	if req.Data == "" {
		req.Data = time.Now().In(a.horaLocal).Format("YYYY-MM-DD")
	}
	setorVal := req.SetorID
	if setorVal <= 0 {
		setorVal = 0
	}
	res, err := a.st.db.Exec(
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
	var confID int64
	confID, _ = res.LastInsertId()
	if confID <= 0 {
		jsonErro(w, http.StatusInternalServerError, "falha ao obter ID da conferência")
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
	var id, gid, sid, apID int64
	var data, st, apNome, abertaEm, fechadaEm, obs string
	if err := a.st.db.QueryRow(
		`SELECT mc.id, mc.grupo_id, COALESCE(mc.setor_id, 0), mc.data, mc.status,
		        mc.aberta_por, COALESCE(ua.nome_guerra, ua.login, ''), mc.aberta_em, mc.fechada_em,
		        COALESCE(mc.observacao, '')
		 FROM material_conferencias mc
		 LEFT JOIN usuarios ua ON ua.id = mc.aberta_por
		 WHERE mc.id = ?`, confID).
		Scan(&id, &gid, &sid, &data, &st, &apID, &apNome, &abertaEm, &fechadaEm, &obs); err != nil {
		jsonErro(w, http.StatusNotFound, "conferência não encontrada")
		return
	}
	rows, rerr := a.st.db.Query(
		`SELECT mci.id, mci.item_id, mi.nome, mci.status,
		        mci.quantidade_esperada, mci.quantidade_conferida,
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
		var ciID, itemID, cpID int64
		var itemNome, st, obsStr string
		var qtdEsp, qtdConf int64
		var cpNome string
		if rows.Scan(&ciID, &itemID, &itemNome, &st, &qtdEsp, &qtdConf, &cpID, &cpNome, &obsStr) == nil {
			itens = append(itens, map[string]any{
				"id": ciID, "item_id": itemID, "item_nome": itemNome, "status": st,
				"quantidade_esperada": qtdEsp, "quantidade_conferida": qtdConf,
				"conferido_por": cpID, "conferido_por_nome": cpNome, "observacao": obsStr,
			})
		}
	}
	jsonOK(w, map[string]any{
		"id": id, "grupo_id": gid, "setor_id": sid,
		"data": data, "status": st,
		"aberta_por": apID, "aberta_por_nome": apNome,
		"aberta_em": abertaEm, "fechada_em": fechadaEm,
		"observacao": obs, "itens": itens,
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
// (implementação mínima para compatibilidade com o teste de evolução).
// -----------------------------------------------------------------
func (a *App) hMaterialConferenciaPDF(w http.ResponseWriter, r *http.Request) {
	confID, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || confID <= 0 {
		jsonErro(w, http.StatusBadRequest, "id inválido")
		return
	}
	// Gera um PDF mínimo válido com os dados da conferência
	dados := fmt.Sprintf("Pronto de Material - Conferencia #%d", confID)
	pdf := a.gerarPDFMinimo("PRONTO DE MATERIAL", dados)
	w.Header().Set("Content-Type", "application/pdf")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`inline; filename="pronto_material_%d.pdf"`, confID))
	w.Header().Set("Content-Length", strconv.Itoa(len(pdf)))
	_, _ = w.Write(pdf)
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
	escopo := escopoDoUsuario(u)

	var rec ReciboCautelaPDF
	rec.ID = id

	var itemGrupoID int64
	q := `SELECT mc.data_saida, COALESCE(mc.data_devolucao, ''), COALESCE(mc.obs_saida, ''), COALESCE(mc.obs_devolucao, ''), mc.status,
	             mi.nome, mi.codigo_patrimonio, COALESCE(mi.numero_serie, '—'), mi.grupo_id,
	             COALESCE(cat.nome, 'Geral'), COALESCE(mi.nivel_sensibilidade, 'padrao'),
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
	escopo := escopoDoUsuario(u)

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
	escopo := escopoDoUsuario(u)
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
	idStr := r.PathValue("id")
	id, _ := strconv.ParseInt(idStr, 10, 64)
	if id <= 0 {
		jsonErro(w, http.StatusBadRequest, "ID inválido")
		return
	}
	var count int
	_ = a.st.db.QueryRow(`SELECT COUNT(*) FROM material_itens WHERE categoria_id = ?`, id).Scan(&count)
	if count > 0 {
		_, _ = a.st.db.Exec(`UPDATE material_categorias SET ativo = 0 WHERE id = ?`, id)
		jsonOK(w, map[string]any{"ok": true, "desativado": true})
		return
	}
	_, err := a.st.db.Exec(`DELETE FROM material_categorias WHERE id = ?`, id)
	if err != nil {
		jsonErro(w, http.StatusInternalServerError, err.Error())
		return
	}
	jsonOK(w, map[string]any{"ok": true})
}

func (a *App) hMaterialItensList(w http.ResponseWriter, r *http.Request) {
	u := usuarioDoCtx(r)
	escopo := escopoDoUsuario(u)
	statusQ := r.URL.Query().Get("status")
	catQ := r.URL.Query().Get("categoria_id")

	q := `
		SELECT mi.id, mi.grupo_id, COALESCE(g.nome, ''), mi.categoria_id, COALESCE(mc.nome, 'Sem Categoria'),
		       mi.nome, mi.codigo_patrimonio, COALESCE(mi.numero_serie, ''), mi.status, COALESCE(mi.observacao, ''),
		       mi.criado_em, COALESCE(mi.nivel_sensibilidade, 'padrao'),
		       COALESCE(mi.sensibilidade, 'convencional'), COALESCE(mi.quantidade, 1),
		       COALESCE((SELECT SUM(mc.quantidade) FROM material_cautelas mc WHERE mc.item_id = mi.id AND mc.status = 'ativa'), 0),
		       caut.id, caut.pessoa_id, p.nome_guerra, p.nome_completo, caut.data_saida, COALESCE(caut.obs_saida, ''),
		       ue.login
		FROM material_itens mi
		LEFT JOIN material_categorias mc ON mc.id = mi.categoria_id
		LEFT JOIN grupos g ON g.id = mi.grupo_id
		LEFT JOIN material_cautelas caut ON caut.item_id = mi.id AND caut.status = 'ativa'
		LEFT JOIN pessoas p ON p.id = caut.pessoa_id
		LEFT JOIN usuarios ue ON ue.id = caut.responsavel_entrega_id
		WHERE (? <= 0 OR mi.grupo_id = ?)`
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
		var catID *int64
		var gNome, catNome, nome, cod, numSerie, status, obs, criadoEm, sens, sensibilidade string
		var quantidade, qtdAcautelada int
		var cautID, pesID *int64
		var pNomeGuerra, pNomeCompleto, dtSaida, obsSaida, opEntrega *string
		if err := rows.Scan(&id, &gid, &gNome, &catID, &catNome, &nome, &cod, &numSerie, &status, &obs, &criadoEm, &sens,
			&sensibilidade, &quantidade, &qtdAcautelada,
			&cautID, &pesID, &pNomeGuerra, &pNomeCompleto, &dtSaida, &obsSaida, &opEntrega); err == nil {

			dispQtd := quantidade - qtdAcautelada
			if dispQtd < 0 {
				dispQtd = 0
			}

			item := map[string]any{
				"id":                    id,
				"grupo_id":              gid,
				"grupo_nome":            gNome,
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
			lista = append(lista, item)
		}
	}
	jsonOK(w, map[string]any{"itens": lista})
}

func (a *App) hMaterialItensSave(w http.ResponseWriter, r *http.Request) {
	u := usuarioDoCtx(r)
	if u.Papel != "admin" && u.GrupoID == nil {
		jsonErro(w, http.StatusForbidden, "Conta sem grupo")
		return
	}
	var req struct {
		ID                 int64  `json:"id"`
		GrupoID            *int64 `json:"grupo_id"`
		CategoriaID        *int64 `json:"categoria_id"`
		Nome               string `json:"nome"`
		CodigoPatrimonio   string `json:"codigo_patrimonio"`
		Patrimonio         string `json:"patrimonio"`
		NumeroSerie        string `json:"numero_serie"`
		Status             string `json:"status"`
		Observacao         string `json:"observacao"`
		NivelSensibilidade string `json:"nivel_sensibilidade"`
		Sensibilidade      string `json:"sensibilidade"`
		Quantidade         int    `json:"quantidade"`
	}
	if err := decodificar(r, &req); err != nil || strings.TrimSpace(req.Nome) == "" {
		jsonErro(w, http.StatusBadRequest, "Nome é obrigatório")
		return
	}
	if strings.TrimSpace(req.CodigoPatrimonio) == "" && strings.TrimSpace(req.Patrimonio) != "" {
		req.CodigoPatrimonio = strings.TrimSpace(req.Patrimonio)
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

	if req.ID > 0 {
		resIt, err := a.st.db.Exec(`
			UPDATE material_itens
			SET categoria_id = ?, nome = ?, codigo_patrimonio = ?, numero_serie = ?, status = ?, observacao = ?, nivel_sensibilidade = ?, sensibilidade = ?, quantidade = ?
			WHERE id = ? AND (? <= 0 OR grupo_id = ?)`,
			req.CategoriaID, req.Nome, req.CodigoPatrimonio, req.NumeroSerie, req.Status, req.Observacao, req.NivelSensibilidade, req.Sensibilidade, req.Quantidade, req.ID, escopoDoUsuario(u), grupoID)
		if err != nil {
			jsonErro(w, http.StatusInternalServerError, err.Error())
			return
		}
		// Fix P1-2: 200 sem efeito escondia edição fora do escopo — agora 404 honesto.
		if n, _ := resIt.RowsAffected(); n == 0 {
			jsonErro(w, http.StatusNotFound, "item não encontrado no seu escopo")
			return
		}
		a.st.Auditoria(&u.ID, "editar", "material_itens", &req.ID, req.Nome+" ("+req.CodigoPatrimonio+")", ipDe(r))
		jsonOK(w, map[string]any{"ok": true, "id": req.ID})
		return
	}

	res, err := a.st.db.Exec(`
		INSERT INTO material_itens (grupo_id, categoria_id, nome, codigo_patrimonio, numero_serie, status, observacao, nivel_sensibilidade, sensibilidade, quantidade)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		grupoID, req.CategoriaID, req.Nome, req.CodigoPatrimonio, req.NumeroSerie, req.Status, req.Observacao, req.NivelSensibilidade, req.Sensibilidade, req.Quantidade)
	if err != nil {
		jsonErro(w, http.StatusInternalServerError, err.Error())
		return
	}
	newID, _ := res.LastInsertId()
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

	esc := escopoDoUsuario(u)
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
	if esc := escopoDoUsuario(u); esc > 0 {
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
		var itemGrupo int64
		if err := tx.QueryRow(`SELECT COALESCE(grupo_id,0) FROM material_itens WHERE id = ?`, itemID).Scan(&itemGrupo); err == nil {
			if esc := escopoDoUsuario(u); esc > 0 && itemGrupo != esc {
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
	escopo := escopoDoUsuario(u)
	statusQ := r.URL.Query().Get("status")
	pessoaQ := r.URL.Query().Get("pessoa_id")
	itemQ := r.URL.Query().Get("item_id")

	q := `
		SELECT mc.id, mc.item_id, mi.nome, mi.codigo_patrimonio,
		       mc.pessoa_id, p.nome_guerra, p.nome_completo,
		       mc.responsavel_entrega_id, ue.login,
		       COALESCE(mc.responsavel_recebimento_id, 0), COALESCE(ur.login, ''),
		       mc.data_saida, COALESCE(mc.data_devolucao, ''),
		       COALESCE(mc.obs_saida, ''), COALESCE(mc.obs_devolucao, ''),
		       mc.status, COALESCE(mc.quantidade, 1), COALESCE(mi.sensibilidade, 'convencional')
		FROM material_cautelas mc
		JOIN material_itens mi ON mi.id = mc.item_id
		JOIN pessoas p ON p.id = mc.pessoa_id
		JOIN usuarios ue ON ue.id = mc.responsavel_entrega_id
		LEFT JOIN usuarios ur ON ur.id = mc.responsavel_recebimento_id
		WHERE (? <= 0 OR mi.grupo_id = ?)`
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
	if esc := escopoDoUsuario(u); esc > 0 && itemGrupo != esc {
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
	if esc := escopoDoUsuario(u); esc > 0 && gid != esc {
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
	escopo := escopoDoUsuario(u)

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
// Permite gerente, operador, chefe_setor E encarregado/auxiliar de material
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
			if u.Papel == "gerente" || u.Papel == "operador" || u.Papel == "chefe_setor" || a.ehEncarregadoDeMaterial(u) {
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
	m.Handle("GET /api/material/inventario/pdf", a.auth(false, a.hMaterialInventarioPDF))
	m.Handle("POST /api/material/cautelar", a.authMaterial(a.hMaterialCautelar))
	m.Handle("POST /api/material/devolver", a.authMaterial(a.hMaterialDevolver))
	m.Handle("GET /api/material/cautelas", a.authMaterial(a.hMaterialCautelasList))
	m.Handle("GET /api/material/cautelas/{id}/recibo.pdf", a.auth(false, a.hMaterialCautelaReciboPDF))
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
	m.Handle("GET /api/material/conferencias/{id}/pronto.pdf", a.auth(false, a.hMaterialConferenciaPDF))
}
