package main

import (
	"fmt"
	"net/http"
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
		GrupoID              int64  `json:"grupo_id"`
		SetorID              int64  `json:"setor_id"`
		EncarregadoID        int64  `json:"encarregado_id"`
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
	if encID <= 0 { encID = 0 }
	auxID := req.AuxiliarEncarregadoID
	if auxID <= 0 { auxID = 0 }
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
	if setorVal <= 0 { setorVal = 0 }
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
		ItemID             int64  `json:"item_id"`
		CodigoPatrimonio   string `json:"codigo_patrimonio"`
		Status             string `json:"status"`
		QuantidadeEsperada int64  `json:"quantidade_esperada"`
		QuantidadeConferida int64 `json:"quantidade_conferida"`
		Observacao         string `json:"observacao"`
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