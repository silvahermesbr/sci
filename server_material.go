package main

import (
	"encoding/base64"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// =====================================================================
// MÓDULO DE MATERIAL, RESERVA E CAUTELAS (v1.0)
// =====================================================================

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
	setorQ := r.URL.Query().Get("setor_id")
	garagemQ := r.URL.Query().Get("garagem")

	q := `
		SELECT mi.id, mi.grupo_id, COALESCE(g.nome, ''), mi.categoria_id, COALESCE(mc.nome, 'Sem Categoria'),
		       mi.nome, mi.codigo_patrimonio, COALESCE(mi.numero_serie, ''), mi.status, COALESCE(mi.observacao, ''),
		       mi.criado_em, COALESCE(mi.nivel_sensibilidade, 'padrao'),
		       COALESCE(mi.sensibilidade, 'convencional'), COALESCE(mi.quantidade, 1),
		       COALESCE((SELECT SUM(mc.quantidade) FROM material_cautelas mc WHERE mc.item_id = mi.id AND mc.status = 'ativa'), 0),
		       caut.id, caut.pessoa_id, p.nome_guerra, p.nome_completo, caut.data_saida, COALESCE(caut.obs_saida, ''),
		       ue.login,
		       mi.setor_id, COALESCE(st.nome, 'Carga Geral'),
		       mv.placa, mv.renavam, mv.padrinho_titular_id, COALESCE(pt.nome_guerra, ''),
		       mv.padrinho_substituto_id, COALESCE(ps.nome_guerra, ''),
		       COALESCE(mv.hodometro_atual, 0), COALESCE(mv.combustivel_atual, 'cheio')
		FROM material_itens mi
		LEFT JOIN material_categorias mc ON mc.id = mi.categoria_id
		LEFT JOIN grupos g ON g.id = mi.grupo_id
		LEFT JOIN setores st ON st.id = mi.setor_id
		LEFT JOIN material_viaturas mv ON mv.item_id = mi.id
		LEFT JOIN pessoas pt ON pt.id = mv.padrinho_titular_id
		LEFT JOIN pessoas ps ON ps.id = mv.padrinho_substituto_id
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
	if setorQ != "" {
		if sid, err := strconv.ParseInt(setorQ, 10, 64); err == nil {
			if sid == 0 {
				q += ` AND (mi.setor_id IS NULL OR mi.setor_id = 0)`
			} else if sid > 0 {
				q += ` AND mi.setor_id = ?`
				args = append(args, sid)
			}
		}
	}
	if garagemQ == "1" || garagemQ == "true" {
		q += ` AND (LOWER(mc.nome) LIKE '%viatura%' OR LOWER(mc.nome) LIKE '%veiculo%' OR mv.item_id IS NOT NULL)`
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
		var catID, setorID *int64
		var gNome, catNome, nome, cod, numSerie, status, obs, criadoEm, sens, sensibilidade, setorNome string
		var quantidade, qtdAcautelada int
		var cautID, pesID *int64
		var pNomeGuerra, pNomeCompleto, dtSaida, obsSaida, opEntrega *string
		var mvPlaca, mvRenavam *string
		var mvPadTitID, mvPadSubID *int64
		var mvPadTitNome, mvPadSubNome string
		var mvHodometro int
		var mvCombustivel string

		if err := rows.Scan(&id, &gid, &gNome, &catID, &catNome, &nome, &cod, &numSerie, &status, &obs, &criadoEm, &sens,
			&sensibilidade, &quantidade, &qtdAcautelada,
			&cautID, &pesID, &pNomeGuerra, &pNomeCompleto, &dtSaida, &obsSaida, &opEntrega,
			&setorID, &setorNome,
			&mvPlaca, &mvRenavam, &mvPadTitID, &mvPadTitNome, &mvPadSubID, &mvPadSubNome,
			&mvHodometro, &mvCombustivel); err == nil {

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
				"setor_id":              setorID,
				"setor_nome":            setorNome,
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
			if mvPlaca != nil || mvRenavam != nil || mvPadTitID != nil || mvPadSubID != nil || mvHodometro > 0 {
				item["viatura"] = map[string]any{
					"placa":                  strVal(mvPlaca),
					"renavam":                strVal(mvRenavam),
					"padrinho_titular_id":    mvPadTitID,
					"padrinho_titular_nome":  mvPadTitNome,
					"padrinho_substituto_id": mvPadSubID,
					"padrinho_substituto_nome": mvPadSubNome,
					"hodometro_atual":        mvHodometro,
					"combustivel_atual":      mvCombustivel,
				}
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

func strVal(s *string) string {
	if s == nil {
		return ""
	}
	return *s
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
		SetorID            *int64 `json:"setor_id"`
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
		// Campos específicos de Viatura / Garagem
		Placa                string `json:"placa"`
		Renavam              string `json:"renavam"`
		PadrinhoTitularID    *int64 `json:"padrinho_titular_id"`
		PadrinhoSubstitutoID *int64 `json:"padrinho_substituto_id"`
		HodometroAtual       int    `json:"hodometro_atual"`
		CombustivelAtual     string `json:"combustivel_atual"`
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

	salvarViatura := func(itemID int64) {
		if strings.TrimSpace(req.Placa) != "" || strings.TrimSpace(req.Renavam) != "" || req.PadrinhoTitularID != nil || req.PadrinhoSubstitutoID != nil || req.HodometroAtual > 0 {
			comb := req.CombustivelAtual
			if comb == "" {
				comb = "cheio"
			}
			_, _ = a.st.db.Exec(`
				INSERT INTO material_viaturas (item_id, placa, renavam, padrinho_titular_id, padrinho_substituto_id, hodometro_atual, combustivel_atual, atualizado_em)
				VALUES (?, ?, ?, ?, ?, ?, ?, strftime('%Y-%m-%dT%H:%M:%fZ','now'))
				ON CONFLICT(item_id) DO UPDATE SET
					placa = excluded.placa,
					renavam = excluded.renavam,
					padrinho_titular_id = excluded.padrinho_titular_id,
					padrinho_substituto_id = excluded.padrinho_substituto_id,
					hodometro_atual = excluded.hodometro_atual,
					combustivel_atual = excluded.combustivel_atual,
					atualizado_em = excluded.atualizado_em`,
				itemID, strings.TrimSpace(req.Placa), strings.TrimSpace(req.Renavam), req.PadrinhoTitularID, req.PadrinhoSubstitutoID, req.HodometroAtual, comb)
		}
	}

	if req.ID > 0 {
		resIt, err := a.st.db.Exec(`
			UPDATE material_itens
			SET categoria_id = ?, setor_id = ?, nome = ?, codigo_patrimonio = ?, numero_serie = ?, status = ?, observacao = ?, nivel_sensibilidade = ?, sensibilidade = ?, quantidade = ?
			WHERE id = ? AND (? <= 0 OR grupo_id = ?)`,
			req.CategoriaID, req.SetorID, req.Nome, req.CodigoPatrimonio, req.NumeroSerie, req.Status, req.Observacao, req.NivelSensibilidade, req.Sensibilidade, req.Quantidade, req.ID, escopoDoUsuario(u), grupoID)
		if err != nil {
			jsonErro(w, http.StatusInternalServerError, err.Error())
			return
		}
		if n, _ := resIt.RowsAffected(); n == 0 {
			jsonErro(w, http.StatusNotFound, "item não encontrado no seu escopo")
			return
		}
		salvarViatura(req.ID)
		a.st.Auditoria(&u.ID, "editar", "material_itens", &req.ID, req.Nome+" ("+req.CodigoPatrimonio+")", ipDe(r))
		jsonOK(w, map[string]any{"ok": true, "id": req.ID})
		return
	}

	res, err := a.st.db.Exec(`
		INSERT INTO material_itens (grupo_id, setor_id, categoria_id, nome, codigo_patrimonio, numero_serie, status, observacao, nivel_sensibilidade, sensibilidade, quantidade)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		grupoID, req.SetorID, req.CategoriaID, req.Nome, req.CodigoPatrimonio, req.NumeroSerie, req.Status, req.Observacao, req.NivelSensibilidade, req.Sensibilidade, req.Quantidade)
	if err != nil {
		jsonErro(w, http.StatusInternalServerError, err.Error())
		return
	}
	newID, _ := res.LastInsertId()
	salvarViatura(newID)
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
			mime := anexo.TipoMIME
			if mime == "" {
				mime = "application/octet-stream"
			}
			_, _ = tx.Exec(`
				INSERT INTO material_cautela_anexos (cautela_id, nome_arquivo, tipo_mime, tamanho, dados_base64)
				VALUES (?, ?, ?, ?, ?)`,
				cautelaID, anexo.NomeArquivo, mime, anexo.Tamanho, anexo.DadosBase64)
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

// =====================================================================
// ANEXOS E DOCUMENTOS ESCANEADOS DE CAUTELAS (v1.0)
// =====================================================================

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
	// Fix P1-1: anexar em cautela exige escopo do item cautelado.
	{
		var itemGrupo int64
		if err := a.st.db.QueryRow(`SELECT COALESCE(mi.grupo_id,0)
			FROM material_cautelas mc JOIN material_itens mi ON mi.id = mc.item_id
			WHERE mc.id = ?`, cautelaID).Scan(&itemGrupo); err == nil {
			if esc := escopoDoUsuario(u); esc > 0 && itemGrupo != esc {
				jsonErro(w, http.StatusForbidden, "cautela fora do seu escopo")
				return
			}
		}
	}
	// Fix P1-3: teto REAL de anexo — o LimitReader de 1 MB corta o JSON inteiro;
	// base64 cresce ~4/3, então o DECODED útil máximo aqui é ~600 KB.
	const maxAnexoBase64 = 800 * 1024 // 800 KB de base64 ≈ 600 KB de arquivo
	if len(req.DadosBase64) > maxAnexoBase64 {
		jsonErro(w, http.StatusRequestEntityTooLarge, "anexo acima do teto (máx. ~600 KB)")
		return
	}
	mime := req.TipoMIME
	if mime == "" {
		mime = "application/octet-stream"
	}
	res, err := a.st.db.Exec(`
		INSERT INTO material_cautela_anexos (cautela_id, nome_arquivo, tipo_mime, tamanho, dados_base64)
		VALUES (?, ?, ?, ?, ?)`,
		cautelaID, req.NomeArquivo, mime, req.Tamanho, req.DadosBase64)
	if err != nil {
		jsonErro(w, http.StatusInternalServerError, err.Error())
		return
	}
	newID, _ := res.LastInsertId()
	a.st.Auditoria(&u.ID, "anexar_documento", "material_cautela_anexos", &newID,
		fmt.Sprintf("cautela=%d arquivo=%s", cautelaID, req.NomeArquivo), ipDe(r))
	jsonOK(w, map[string]any{"ok": true, "id": newID, "nome_arquivo": req.NomeArquivo})
}

func (a *App) hMaterialAnexoList(w http.ResponseWriter, r *http.Request) {
	cautelaIDStr := r.PathValue("id")
	cautelaID, _ := strconv.ParseInt(cautelaIDStr, 10, 64)
	if cautelaID <= 0 {
		jsonErro(w, http.StatusBadRequest, "ID da cautela inválido")
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
	anexoIDStr := r.PathValue("id")
	anexoID, _ := strconv.ParseInt(anexoIDStr, 10, 64)
	if anexoID <= 0 {
		jsonErro(w, http.StatusBadRequest, "ID inválido")
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
	w.Header().Set("Content-Type", mime)
	w.Header().Set("Content-Disposition", fmt.Sprintf(`inline; filename="%s"`, nome))
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
	_, err := a.st.db.Exec(`DELETE FROM material_cautela_anexos WHERE id = ?`, anexoID)
	if err != nil {
		jsonErro(w, http.StatusInternalServerError, err.Error())
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

// =====================================================================
// ENCARREGADOS E AUXILIARES DE MATERIAL (v1.5 / v30)
// =====================================================================

func (a *App) hMaterialResponsaveisList(w http.ResponseWriter, r *http.Request) {
	u := usuarioDoCtx(r)
	escopo := escopoDoUsuario(u)

	q := `
		SELECT gsr.grupo_id, g.nome, gsr.setor_id, COALESCE(s.nome, 'Carga Geral'),
		       gsr.encarregado_id, COALESCE(pe.nome_guerra, ''), COALESCE(pe.nome_completo, ''),
		       gsr.auxiliar_encarregado_id, COALESCE(pa.nome_guerra, ''), COALESCE(pa.nome_completo, '')
		FROM grupo_setor_responsaveis gsr
		JOIN grupos g ON g.id = gsr.grupo_id
		LEFT JOIN setores s ON s.id = gsr.setor_id
		LEFT JOIN pessoas pe ON pe.id = gsr.encarregado_id
		LEFT JOIN pessoas pa ON pa.id = gsr.auxiliar_encarregado_id
		WHERE (? <= 0 OR gsr.grupo_id = ?)
		ORDER BY g.nome, s.nome`

	rows, err := a.st.db.Query(q, escopo, escopo)
	if err != nil {
		jsonErro(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer rows.Close()

	var lista []map[string]any
	for rows.Next() {
		var gid int64
		var gNome, sNome, eGuerra, eCompleto, aGuerra, aCompleto string
		var sid, eid, aid *int64
		if rows.Scan(&gid, &gNome, &sid, &sNome, &eid, &eGuerra, &eCompleto, &aid, &aGuerra, &aCompleto) == nil {
			lista = append(lista, map[string]any{
				"grupo_id":                gid,
				"grupo_nome":              gNome,
				"setor_id":                sid,
				"setor_nome":              sNome,
				"encarregado_id":          eid,
				"encarregado_nome_guerra": eGuerra,
				"encarregado_nome":        eCompleto,
				"auxiliar_id":             aid,
				"auxiliar_nome_guerra":    aGuerra,
				"auxiliar_nome":           aCompleto,
			})
		}
	}
	jsonOK(w, map[string]any{"responsaveis": lista})
}

func (a *App) hMaterialResponsaveisSave(w http.ResponseWriter, r *http.Request) {
	u := usuarioDoCtx(r)
	var req struct {
		GrupoID               *int64 `json:"grupo_id"`
		SetorID               *int64 `json:"setor_id"`
		EncarregadoID         *int64 `json:"encarregado_id"`
		AuxiliarEncarregadoID *int64 `json:"auxiliar_encarregado_id"`
	}
	if err := decodificar(r, &req); err != nil {
		jsonErro(w, http.StatusBadRequest, "Dados inválidos")
		return
	}

	gid := int64(0)
	if u.GrupoID != nil {
		gid = *u.GrupoID
	}
	if req.GrupoID != nil && *req.GrupoID > 0 && u.Papel == "admin" {
		gid = *req.GrupoID
	}
	if gid <= 0 {
		_ = a.st.db.QueryRow(`SELECT id FROM grupos ORDER BY id LIMIT 1`).Scan(&gid)
	}

	_, err := a.st.db.Exec(`
		INSERT INTO grupo_setor_responsaveis (grupo_id, setor_id, encarregado_id, auxiliar_encarregado_id, atualizado_em)
		VALUES (?, ?, ?, ?, strftime('%Y-%m-%dT%H:%M:%fZ','now'))
		ON CONFLICT(grupo_id, setor_id) DO UPDATE SET
			encarregado_id = excluded.encarregado_id,
			auxiliar_encarregado_id = excluded.auxiliar_encarregado_id,
			atualizado_em = excluded.atualizado_em`,
		gid, req.SetorID, req.EncarregadoID, req.AuxiliarEncarregadoID)
	if err != nil {
		jsonErro(w, http.StatusInternalServerError, err.Error())
		return
	}

	a.st.Auditoria(&u.ID, "salvar_responsaveis", "grupo_setor_responsaveis", nil, fmt.Sprintf("grupo=%d setor=%v", gid, req.SetorID), ipDe(r))
	jsonOK(w, map[string]any{"ok": true})
}

// =====================================================================
// FICHA DA VIATURA / ITEM: ANEXOS E COMENTÁRIOS (v1.5 / v30)
// =====================================================================

func (a *App) hMaterialItemAnexosList(w http.ResponseWriter, r *http.Request) {
	idStr := r.PathValue("id")
	itemID, _ := strconv.ParseInt(idStr, 10, 64)
	if itemID <= 0 {
		jsonErro(w, http.StatusBadRequest, "ID do item inválido")
		return
	}
	rows, err := a.st.db.Query(`
		SELECT id, item_id, nome_arquivo, tipo_mime, tamanho, criado_em
		FROM material_item_anexos WHERE item_id = ? ORDER BY id DESC`, itemID)
	if err != nil {
		jsonErro(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer rows.Close()
	var lista []map[string]any
	for rows.Next() {
		var id, iid, tam int64
		var nome, mime, criada string
		if rows.Scan(&id, &iid, &nome, &mime, &tam, &criada) == nil {
			lista = append(lista, map[string]any{
				"id": id, "item_id": iid, "nome_arquivo": nome, "tipo_mime": mime, "tamanho": tam, "criado_em": criada,
			})
		}
	}
	jsonOK(w, map[string]any{"anexos": lista})
}

func (a *App) hMaterialItemAnexoAdd(w http.ResponseWriter, r *http.Request) {
	u := usuarioDoCtx(r)
	idStr := r.PathValue("id")
	itemID, _ := strconv.ParseInt(idStr, 10, 64)
	if itemID <= 0 {
		jsonErro(w, http.StatusBadRequest, "ID inválido")
		return
	}
	var req struct {
		NomeArquivo string `json:"nome_arquivo"`
		TipoMIME    string `json:"tipo_mime"`
		Tamanho     int64  `json:"tamanho"`
		DadosBase64 string `json:"dados_base64"`
	}
	if err := decodificar(r, &req); err != nil || strings.TrimSpace(req.NomeArquivo) == "" || strings.TrimSpace(req.DadosBase64) == "" {
		jsonErro(w, http.StatusBadRequest, "Arquivo e conteúdo são obrigatórios")
		return
	}
	mime := req.TipoMIME
	if mime == "" {
		mime = "application/octet-stream"
	}
	res, err := a.st.db.Exec(`
		INSERT INTO material_item_anexos (item_id, nome_arquivo, tipo_mime, tamanho, dados_base64)
		VALUES (?, ?, ?, ?, ?)`, itemID, req.NomeArquivo, mime, req.Tamanho, req.DadosBase64)
	if err != nil {
		jsonErro(w, http.StatusInternalServerError, err.Error())
		return
	}
	nid, _ := res.LastInsertId()
	a.st.Auditoria(&u.ID, "anexar_item", "material_item_anexos", &nid, fmt.Sprintf("item=%d arquivo=%s", itemID, req.NomeArquivo), ipDe(r))
	jsonOK(w, map[string]any{"ok": true, "id": nid})
}

func (a *App) hMaterialItemAnexoGet(w http.ResponseWriter, r *http.Request) {
	idStr := r.PathValue("anexo_id")
	id, _ := strconv.ParseInt(idStr, 10, 64)
	if id <= 0 {
		jsonErro(w, http.StatusBadRequest, "ID inválido")
		return
	}
	var nome, mime, b64 string
	err := a.st.db.QueryRow(`SELECT nome_arquivo, tipo_mime, dados_base64 FROM material_item_anexos WHERE id = ?`, id).Scan(&nome, &mime, &b64)
	if err != nil {
		jsonErro(w, http.StatusNotFound, "Anexo não encontrado")
		return
	}
	if idx := strings.Index(b64, ","); idx != -1 {
		b64 = b64[idx+1:]
	}
	dados, err := base64.StdEncoding.DecodeString(b64)
	if err != nil {
		jsonErro(w, http.StatusInternalServerError, "Falha na decodificação")
		return
	}
	w.Header().Set("Content-Type", mime)
	w.Header().Set("Content-Disposition", fmt.Sprintf(`inline; filename="%s"`, nome))
	w.Header().Set("Content-Length", strconv.Itoa(len(dados)))
	_, _ = w.Write(dados)
}

func (a *App) hMaterialItemAnexoDel(w http.ResponseWriter, r *http.Request) {
	u := usuarioDoCtx(r)
	idStr := r.PathValue("anexo_id")
	id, _ := strconv.ParseInt(idStr, 10, 64)
	if id <= 0 {
		jsonErro(w, http.StatusBadRequest, "ID inválido")
		return
	}
	_, err := a.st.db.Exec(`DELETE FROM material_item_anexos WHERE id = ?`, id)
	if err != nil {
		jsonErro(w, http.StatusInternalServerError, err.Error())
		return
	}
	a.st.Auditoria(&u.ID, "excluir_anexo_item", "material_item_anexos", &id, "", ipDe(r))
	jsonOK(w, map[string]any{"ok": true})
}

func (a *App) hMaterialItemComentariosList(w http.ResponseWriter, r *http.Request) {
	idStr := r.PathValue("id")
	itemID, _ := strconv.ParseInt(idStr, 10, 64)
	if itemID <= 0 {
		jsonErro(w, http.StatusBadRequest, "ID inválido")
		return
	}
	rows, err := a.st.db.Query(`
		SELECT mic.id, mic.item_id, mic.operador_id, u.login, mic.texto, mic.criado_em
		FROM material_item_comentarios mic
		JOIN usuarios u ON u.id = mic.operador_id
		WHERE mic.item_id = ?
		ORDER BY mic.id DESC`, itemID)
	if err != nil {
		jsonErro(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer rows.Close()
	var lista []map[string]any
	for rows.Next() {
		var id, iid, opID int64
		var login, txt, criada string
		if rows.Scan(&id, &iid, &opID, &login, &txt, &criada) == nil {
			lista = append(lista, map[string]any{
				"id":          id,
				"item_id":     iid,
				"operador_id": opID,
				"operador":    login,
				"texto":       txt,
				"criado_em":   criada,
			})
		}
	}
	jsonOK(w, map[string]any{"comentarios": lista})
}

func (a *App) hMaterialItemComentarioAdd(w http.ResponseWriter, r *http.Request) {
	u := usuarioDoCtx(r)
	idStr := r.PathValue("id")
	itemID, _ := strconv.ParseInt(idStr, 10, 64)
	if itemID <= 0 {
		jsonErro(w, http.StatusBadRequest, "ID inválido")
		return
	}
	var req struct {
		Texto string `json:"texto"`
	}
	if err := decodificar(r, &req); err != nil || strings.TrimSpace(req.Texto) == "" {
		jsonErro(w, http.StatusBadRequest, "Texto do comentário é obrigatório")
		return
	}
	res, err := a.st.db.Exec(`
		INSERT INTO material_item_comentarios (item_id, operador_id, texto)
		VALUES (?, ?, ?)`, itemID, u.ID, strings.TrimSpace(req.Texto))
	if err != nil {
		jsonErro(w, http.StatusInternalServerError, err.Error())
		return
	}
	nid, _ := res.LastInsertId()
	a.st.Auditoria(&u.ID, "comentar_item", "material_item_comentarios", &nid, fmt.Sprintf("item=%d", itemID), ipDe(r))
	jsonOK(w, map[string]any{"ok": true, "id": nid})
}

// =====================================================================
// CONFERÊNCIA DIÁRIA DE MATERIAL (CHECK DIÁRIO & PRONTO) (v1.5 / v30)
// =====================================================================

func (a *App) hMaterialConferenciasList(w http.ResponseWriter, r *http.Request) {
	u := usuarioDoCtx(r)
	escopo := escopoDoUsuario(u)

	q := `
		SELECT mc.id, mc.grupo_id, g.nome, mc.setor_id, COALESCE(s.nome, 'Carga Geral'),
		       mc.data, mc.status, mc.aberta_por, uo.login, mc.aberta_em,
		       COALESCE(mc.fechada_por, 0), COALESCE(uf.login, ''), COALESCE(mc.fechada_em, ''),
		       mc.encarregado_id, COALESCE(pe.nome_guerra, ''),
		       mc.auxiliar_id, COALESCE(pa.nome_guerra, ''),
		       COALESCE(mc.observacao, ''),
		       (SELECT COUNT(*) FROM material_conferencia_itens mci WHERE mci.conferencia_id = mc.id) AS total_itens,
		       (SELECT COUNT(*) FROM material_conferencia_itens mci WHERE mci.conferencia_id = mc.id AND mci.status = 'presente') AS itens_presentes,
		       (SELECT COUNT(*) FROM material_conferencia_itens mci WHERE mci.conferencia_id = mc.id AND mci.status = 'acautelado') AS itens_acautelados
		FROM material_conferencias mc
		JOIN grupos g ON g.id = mc.grupo_id
		LEFT JOIN setores s ON s.id = mc.setor_id
		JOIN usuarios uo ON uo.id = mc.aberta_por
		LEFT JOIN usuarios uf ON uf.id = mc.fechada_por
		LEFT JOIN pessoas pe ON pe.id = mc.encarregado_id
		LEFT JOIN pessoas pa ON pa.id = mc.auxiliar_id
		WHERE (? <= 0 OR mc.grupo_id = ?)
		ORDER BY mc.data DESC, mc.id DESC LIMIT 100`

	rows, err := a.st.db.Query(q, escopo, escopo)
	if err != nil {
		jsonErro(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer rows.Close()

	var lista []map[string]any
	for rows.Next() {
		var id, gid, opAberturaID, opFechamentoID int64
		var sid, eid, aid *int64
		var gNome, sNome, data, st, opAberturaLogin, opFechamentoLogin, abertaEm, fechadaEm, eGuerra, aGuerra, obs string
		var totalItens, presentes, acautelados int

		if rows.Scan(&id, &gid, &gNome, &sid, &sNome, &data, &st, &opAberturaID, &opAberturaLogin, &abertaEm,
			&opFechamentoID, &opFechamentoLogin, &fechadaEm, &eid, &eGuerra, &aid, &aGuerra, &obs,
			&totalItens, &presentes, &acautelados) == nil {
			lista = append(lista, map[string]any{
				"id":                      id,
				"grupo_id":                gid,
				"grupo_nome":              gNome,
				"setor_id":                sid,
				"setor_nome":              sNome,
				"data":                    data,
				"status":                  st,
				"aberta_por":              opAberturaLogin,
				"aberta_em":               abertaEm,
				"fechada_por":             opFechamentoLogin,
				"fechada_em":              fechadaEm,
				"encarregado_id":          eid,
				"encarregado_nome_guerra": eGuerra,
				"auxiliar_id":             aid,
				"auxiliar_nome_guerra":    aGuerra,
				"observacao":              obs,
				"total_itens":             totalItens,
				"itens_presentes":         presentes,
				"itens_acautelados":       acautelados,
			})
		}
	}
	jsonOK(w, map[string]any{"conferencias": lista})
}

func (a *App) hMaterialConferenciaGet(w http.ResponseWriter, r *http.Request) {
	idStr := r.PathValue("id")
	id, _ := strconv.ParseInt(idStr, 10, 64)
	if id <= 0 {
		jsonErro(w, http.StatusBadRequest, "ID inválido")
		return
	}

	var conf map[string]any
	{
		var gid, opAberturaID, opFechamentoID int64
		var sid, eid, aid *int64
		var gNome, sNome, data, st, opAberturaLogin, opFechamentoLogin, abertaEm, fechadaEm, eGuerra, aGuerra, obs string
		err := a.st.db.QueryRow(`
			SELECT mc.id, mc.grupo_id, g.nome, mc.setor_id, COALESCE(s.nome, 'Carga Geral'),
			       mc.data, mc.status, mc.aberta_por, uo.login, mc.aberta_em,
			       COALESCE(mc.fechada_por, 0), COALESCE(uf.login, ''), COALESCE(mc.fechada_em, ''),
			       mc.encarregado_id, COALESCE(pe.nome_guerra, ''),
			       mc.auxiliar_id, COALESCE(pa.nome_guerra, ''),
			       COALESCE(mc.observacao, '')
			FROM material_conferencias mc
			JOIN grupos g ON g.id = mc.grupo_id
			LEFT JOIN setores s ON s.id = mc.setor_id
			JOIN usuarios uo ON uo.id = mc.aberta_por
			LEFT JOIN usuarios uf ON uf.id = mc.fechada_por
			LEFT JOIN pessoas pe ON pe.id = mc.encarregado_id
			LEFT JOIN pessoas pa ON pa.id = mc.auxiliar_id
			WHERE mc.id = ?`, id).Scan(&id, &gid, &gNome, &sid, &sNome, &data, &st, &opAberturaID, &opAberturaLogin, &abertaEm,
			&opFechamentoID, &opFechamentoLogin, &fechadaEm, &eid, &eGuerra, &aid, &aGuerra, &obs)
		if err != nil {
			jsonErro(w, http.StatusNotFound, "Conferência não encontrada")
			return
		}
		conf = map[string]any{
			"id":                      id,
			"grupo_id":                gid,
			"grupo_nome":              gNome,
			"setor_id":                sid,
			"setor_nome":              sNome,
			"data":                    data,
			"status":                  st,
			"aberta_por":              opAberturaLogin,
			"aberta_em":               abertaEm,
			"fechada_por":             opFechamentoLogin,
			"fechada_em":              fechadaEm,
			"encarregado_id":          eid,
			"encarregado_nome_guerra": eGuerra,
			"auxiliar_id":             aid,
			"auxiliar_nome_guerra":    aGuerra,
			"observacao":              obs,
		}
	}

	rows, err := a.st.db.Query(`
		SELECT mci.id, mci.item_id, mi.nome, mi.codigo_patrimonio, COALESCE(cat.nome, 'Geral'),
		       mi.sensibilidade, mci.status, mci.quantidade_esperada, mci.quantidade_conferida,
		       COALESCE(u.login, ''), mci.conferido_em, COALESCE(mci.observacao, ''),
		       COALESCE(p.nome_guerra, '') AS cautela_pessoa
		FROM material_conferencia_itens mci
		JOIN material_itens mi ON mi.id = mci.item_id
		LEFT JOIN material_categorias cat ON cat.id = mi.categoria_id
		LEFT JOIN usuarios u ON u.id = mci.conferido_por
		LEFT JOIN material_cautelas mc ON mc.item_id = mi.id AND mc.status = 'ativa'
		LEFT JOIN pessoas p ON p.id = mc.pessoa_id
		WHERE mci.conferencia_id = ?
		ORDER BY mci.status, mi.nome`, id)
	if err != nil {
		jsonErro(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer rows.Close()

	var itens []map[string]any
	for rows.Next() {
		var mciID, itemID int64
		var iNome, iCod, iCat, iSens, iStatus, opLogin, confEm, iObs, cPessoa string
		var qEsp, qConf int
		if rows.Scan(&mciID, &itemID, &iNome, &iCod, &iCat, &iSens, &iStatus, &qEsp, &qConf, &opLogin, &confEm, &iObs, &cPessoa) == nil {
			itens = append(itens, map[string]any{
				"id":                   mciID,
				"item_id":              itemID,
				"nome":                 iNome,
				"codigo_patrimonio":    iCod,
				"categoria_nome":       iCat,
				"sensibilidade":        iSens,
				"status":               iStatus,
				"quantidade_esperada":  qEsp,
				"quantidade_conferida": qConf,
				"conferido_por":        opLogin,
				"conferido_em":         confEm,
				"observacao":           iObs,
				"cautela_responsavel":  cPessoa,
			})
		}
	}
	conf["itens"] = itens
	jsonOK(w, conf)
}

func (a *App) hMaterialConferenciaIniciar(w http.ResponseWriter, r *http.Request) {
	u := usuarioDoCtx(r)
	var req struct {
		GrupoID *int64 `json:"grupo_id"`
		SetorID *int64 `json:"setor_id"`
		Data    string `json:"data"`
	}
	_ = decodificar(r, &req)

	gid := int64(0)
	if u.GrupoID != nil {
		gid = *u.GrupoID
	}
	if req.GrupoID != nil && *req.GrupoID > 0 && u.Papel == "admin" {
		gid = *req.GrupoID
	}
	if gid <= 0 {
		_ = a.st.db.QueryRow(`SELECT id FROM grupos ORDER BY id LIMIT 1`).Scan(&gid)
	}

	data := req.Data
	if data == "" {
		data = time.Now().Format("2006-01-02")
	}

	// Buscar encarregado e auxiliar cadastrados para o grupo/setor
	var eid, aid *int64
	_ = a.st.db.QueryRow(`
		SELECT encarregado_id, auxiliar_encarregado_id
		FROM grupo_setor_responsaveis
		WHERE grupo_id = ? AND (setor_id = ? OR (setor_id IS NULL AND ? IS NULL)) LIMIT 1`, gid, req.SetorID, req.SetorID).Scan(&eid, &aid)

	tx, err := a.st.db.Begin()
	if err != nil {
		jsonErro(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer tx.Rollback()

	res, err := tx.Exec(`
		INSERT INTO material_conferencias (grupo_id, setor_id, data, status, aberta_por, encarregado_id, auxiliar_id)
		VALUES (?, ?, ?, 'aberta', ?, ?, ?)`, gid, req.SetorID, data, u.ID, eid, aid)
	if err != nil {
		jsonErro(w, http.StatusInternalServerError, err.Error())
		return
	}
	confID, _ := res.LastInsertId()

	// Povoar os itens da conferência a partir da carga correspondente
	q := `
		SELECT mi.id, mi.status, COALESCE(mi.quantidade, 1),
		       COALESCE((SELECT SUM(mc.quantidade) FROM material_cautelas mc WHERE mc.item_id = mi.id AND mc.status = 'ativa'), 0)
		FROM material_itens mi
		WHERE mi.grupo_id = ?`
	args := []any{gid}
	if req.SetorID != nil && *req.SetorID > 0 {
		q += ` AND mi.setor_id = ?`
		args = append(args, *req.SetorID)
	} else if req.SetorID != nil && *req.SetorID == 0 {
		q += ` AND (mi.setor_id IS NULL OR mi.setor_id = 0)`
	}

	rows, err := tx.Query(q, args...)
	if err == nil {
		type itemConfInit struct {
			id       int64
			status   string
			qtdTotal int
			qtdCaut  int
		}
		var carregar []itemConfInit
		for rows.Next() {
			var it itemConfInit
			if rows.Scan(&it.id, &it.status, &it.qtdTotal, &it.qtdCaut) == nil {
				carregar = append(carregar, it)
			}
		}
		rows.Close()

		for _, it := range carregar {
			stInicial := "ausente"
			qEsp := it.qtdTotal - it.qtdCaut
			if qEsp < 0 {
				qEsp = 0
			}
			if it.status == "acautelado" || (it.qtdTotal > 0 && it.qtdCaut >= it.qtdTotal) {
				stInicial = "acautelado"
			} else if it.status == "manutencao" {
				stInicial = "manutencao"
			} else if it.status == "baixado" {
				stInicial = "baixado"
			}

			_, _ = tx.Exec(`
				INSERT INTO material_conferencia_itens (conferencia_id, item_id, status, quantidade_esperada, quantidade_conferida)
				VALUES (?, ?, ?, ?, 0)`, confID, it.id, stInicial, qEsp)
		}
	}

	if err := tx.Commit(); err != nil {
		jsonErro(w, http.StatusInternalServerError, err.Error())
		return
	}

	a.st.Auditoria(&u.ID, "iniciar_conferencia_material", "material_conferencias", &confID, fmt.Sprintf("grupo=%d data=%s", gid, data), ipDe(r))
	jsonOK(w, map[string]any{"ok": true, "id": confID})
}

func (a *App) hMaterialConferenciaBipar(w http.ResponseWriter, r *http.Request) {
	u := usuarioDoCtx(r)
	idStr := r.PathValue("id")
	confID, _ := strconv.ParseInt(idStr, 10, 64)
	if confID <= 0 {
		jsonErro(w, http.StatusBadRequest, "ID de conferência inválido")
		return
	}

	var req struct {
		CodigoPatrimonio string `json:"codigo_patrimonio"`
		ItemID           *int64 `json:"item_id"`
		Status           string `json:"status"` // 'presente' default
		Quantidade       int    `json:"quantidade"`
		Observacao       string `json:"observacao"`
	}
	if err := decodificar(r, &req); err != nil {
		jsonErro(w, http.StatusBadRequest, "Dados inválidos")
		return
	}

	var itemID int64
	if req.ItemID != nil && *req.ItemID > 0 {
		itemID = *req.ItemID
	} else if strings.TrimSpace(req.CodigoPatrimonio) != "" {
		cod := strings.TrimSpace(req.CodigoPatrimonio)
		// Caso o leitor venha com "sci://m:123:PAT"
		if strings.HasPrefix(cod, "sci://m:") {
			parts := strings.Split(cod, ":")
			if len(parts) >= 3 {
				if id, err := strconv.ParseInt(parts[2], 10, 64); err == nil && id > 0 {
					itemID = id
				}
			}
		}
		if itemID == 0 {
			_ = a.st.db.QueryRow(`SELECT id FROM material_itens WHERE codigo_patrimonio = ? LIMIT 1`, cod).Scan(&itemID)
		}
	}

	if itemID <= 0 {
		jsonErro(w, http.StatusNotFound, "Material não localizado pelo código informado")
		return
	}

	st := req.Status
	if st == "" {
		st = "presente"
	}
	qtd := req.Quantidade
	if qtd <= 0 {
		qtd = 1
	}

	res, err := a.st.db.Exec(`
		UPDATE material_conferencia_itens
		SET status = ?, quantidade_conferida = ?, conferido_por = ?, conferido_em = strftime('%Y-%m-%dT%H:%M:%fZ','now'), observacao = ?
		WHERE conferencia_id = ? AND item_id = ?`,
		st, qtd, u.ID, req.Observacao, confID, itemID)
	if err != nil {
		jsonErro(w, http.StatusInternalServerError, err.Error())
		return
	}
	if n, _ := res.RowsAffected(); n == 0 {
		// Se o item não estava na lista (ex: item de outro setor trazido pra reserva), adiciona
		_, _ = a.st.db.Exec(`
			INSERT INTO material_conferencia_itens (conferencia_id, item_id, status, quantidade_esperada, quantidade_conferida, conferido_por, conferido_em, observacao)
			VALUES (?, ?, ?, 1, ?, ?, strftime('%Y-%m-%dT%H:%M:%fZ','now'), ?)`,
			confID, itemID, st, qtd, u.ID, req.Observacao)
	}

	var nome, cod string
	_ = a.st.db.QueryRow(`SELECT nome, codigo_patrimonio FROM material_itens WHERE id = ?`, itemID).Scan(&nome, &cod)

	jsonOK(w, map[string]any{"ok": true, "item_id": itemID, "nome": nome, "codigo_patrimonio": cod, "status": st})
}

func (a *App) hMaterialConferenciaFechar(w http.ResponseWriter, r *http.Request) {
	u := usuarioDoCtx(r)
	idStr := r.PathValue("id")
	confID, _ := strconv.ParseInt(idStr, 10, 64)
	if confID <= 0 {
		jsonErro(w, http.StatusBadRequest, "ID inválido")
		return
	}

	var req struct {
		Observacao string `json:"observacao"`
	}
	_ = decodificar(r, &req)

	res, err := a.st.db.Exec(`
		UPDATE material_conferencias
		SET status = 'fechada', fechada_por = ?, fechada_em = strftime('%Y-%m-%dT%H:%M:%fZ','now'), observacao = ?
		WHERE id = ? AND status = 'aberta'`, u.ID, req.Observacao, confID)
	if err != nil {
		jsonErro(w, http.StatusInternalServerError, err.Error())
		return
	}
	if n, _ := res.RowsAffected(); n == 0 {
		jsonErro(w, http.StatusConflict, "Conferência já se encontra fechada ou cancelada")
		return
	}

	a.st.Auditoria(&u.ID, "fechar_conferencia_material", "material_conferencias", &confID, "", ipDe(r))
	jsonOK(w, map[string]any{"ok": true})
}

func (a *App) hMaterialConferenciaPDF(w http.ResponseWriter, r *http.Request) {
	idStr := r.PathValue("id")
	confID, _ := strconv.ParseInt(idStr, 10, 64)
	if confID <= 0 {
		jsonErro(w, http.StatusBadRequest, "ID inválido")
		return
	}

	var grupoNome, setorNome, data, st, abertaEm, fechadaEm, opAbertura, opFechamento, encGuerra, encCompleto, auxGuerra, auxCompleto, obs string
	err := a.st.db.QueryRow(`
		SELECT g.nome, COALESCE(s.nome, 'Carga Geral'), mc.data, mc.status, mc.aberta_em, COALESCE(mc.fechada_em, '—'),
		       uo.login, COALESCE(uf.login, '—'),
		       COALESCE(pe.nome_guerra, '—'), COALESCE(pe.nome_completo, '—'),
		       COALESCE(pa.nome_guerra, '—'), COALESCE(pa.nome_completo, '—'),
		       COALESCE(mc.observacao, '')
		FROM material_conferencias mc
		JOIN grupos g ON g.id = mc.grupo_id
		LEFT JOIN setores s ON s.id = mc.setor_id
		JOIN usuarios uo ON uo.id = mc.aberta_por
		LEFT JOIN usuarios uf ON uf.id = mc.fechada_por
		LEFT JOIN pessoas pe ON pe.id = mc.encarregado_id
		LEFT JOIN pessoas pa ON pa.id = mc.auxiliar_id
		WHERE mc.id = ?`, confID).Scan(
		&grupoNome, &setorNome, &data, &st, &abertaEm, &fechadaEm,
		&opAbertura, &opFechamento,
		&encGuerra, &encCompleto,
		&auxGuerra, &auxCompleto,
		&obs)
	if err != nil {
		jsonErro(w, http.StatusNotFound, "Conferência não encontrada")
		return
	}

	rows, err := a.st.db.Query(`
		SELECT mi.codigo_patrimonio, mi.nome, COALESCE(cat.nome, 'Geral'),
		       mi.sensibilidade, mci.status, mci.quantidade_esperada, mci.quantidade_conferida,
		       COALESCE(p.nome_guerra, '—')
		FROM material_conferencia_itens mci
		JOIN material_itens mi ON mi.id = mci.item_id
		LEFT JOIN material_categorias cat ON cat.id = mi.categoria_id
		LEFT JOIN material_cautelas mc ON mc.item_id = mi.id AND mc.status = 'ativa'
		LEFT JOIN pessoas p ON p.id = mc.pessoa_id
		WHERE mci.conferencia_id = ?
		ORDER BY mci.status, mi.nome`, confID)
	if err != nil {
		jsonErro(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer rows.Close()

	var itens []map[string]any
	totais := map[string]int{"total": 0, "presente": 0, "acautelado": 0, "ausente": 0, "manutencao": 0, "baixado": 0}
	for rows.Next() {
		var cod, nome, cat, sens, itemSt, resp string
		var qEsp, qConf int
		if rows.Scan(&cod, &nome, &cat, &sens, &itemSt, &qEsp, &qConf, &resp) == nil {
			totais["total"]++
			totais[itemSt]++
			itens = append(itens, map[string]any{
				"codigo_patrimonio":    cod,
				"nome":                 nome,
				"categoria":            cat,
				"sensibilidade":        sens,
				"status":               itemSt,
				"quantidade_esperada":  qEsp,
				"quantidade_conferida": qConf,
				"responsavel":          resp,
			})
		}
	}

	pdfBytes, err := a.gerarProntoMaterialPDF(ProntoMaterialDados{
		ConferenciaID: confID,
		GrupoNome:     grupoNome,
		SetorNome:     setorNome,
		Data:          data,
		Status:        st,
		AbertaEm:      abertaEm,
		FechadaEm:     fechadaEm,
		OpAbertura:    opAbertura,
		OpFechamento:  opFechamento,
		Encarregado:   encGuerra,
		Auxiliar:      auxGuerra,
		Totais:        totais,
		Itens:         itens,
	})
	if err != nil {
		jsonErro(w, http.StatusInternalServerError, "Erro ao gerar PDF do Pronto: "+err.Error())
		return
	}

	w.Header().Set("Content-Type", "application/pdf")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`inline; filename="pronto_material_%d.pdf"`, confID))
	w.Header().Set("Content-Length", strconv.Itoa(len(pdfBytes)))
	_, _ = w.Write(pdfBytes)
}