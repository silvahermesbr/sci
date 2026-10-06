package main

import (
	crand "crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// ---------- Drive Local: Infraestrutura de Armazenamento Físico & Compartilhamento ----------

func (a *App) pastaFisicaDrive() string {
	dir := filepath.Join(a.st.dataDir, "drive")
	_ = os.MkdirAll(dir, 0o750)
	return dir
}

func hexAleatorioDrive(n int) string {
	b := make([]byte, n)
	_, _ = crand.Read(b)
	return hex.EncodeToString(b)
}

// checarAcessoPasta valida se o usuário possui acesso à pasta (leitura ou edição).
func (a *App) checarAcessoPasta(u *Usuario, pastaID int64, precisaEdicao bool) (bool, int64, error) {
	if u.Papel == "admin" {
		return false, 0, fmt.Errorf("administrador não possui acesso ao drive operacional")
	}
	if pastaID == 0 {
		// Raiz do Drive
		if u.GrupoID != nil {
			return true, *u.GrupoID, nil
		}
		return false, 0, fmt.Errorf("sem grupo associado")
	}

	var grupoID int64
	var paiID *int64
	var autorUsuarioID int64
	err := a.st.db.QueryRow(`
		SELECT grupo_id, pai_id, autor_usuario_id 
		FROM drive_pastas WHERE id = ?
	`, pastaID).Scan(&grupoID, &paiID, &autorUsuarioID)
	if err != nil {
		return false, 0, err
	}

	if u.ID == autorUsuarioID {
		return true, grupoID, nil
	}

	// Doutrina Militar: Gerente tem controle do próprio grupo e subordinados
	if u.Papel == "gerente" && u.GrupoID != nil {
		if grupoID == *u.GrupoID || int64Contem(a.gruposSubordinadosAtivos(*u.GrupoID), grupoID) {
			return true, grupoID, nil
		}
	}

	// Checar compartilhamento direto na pasta
	var podeEditar int
	var qArgs []any
	q := `SELECT pode_editar FROM drive_compartilhamentos WHERE pasta_id = ? AND (`
	qArgs = append(qArgs, pastaID)
	conds := []string{"alvo_usuario_id = ?"}
	qArgs = append(qArgs, u.ID)
	if u.PapelAtivoID != nil {
		conds = append(conds, "alvo_papel_id = ?")
		qArgs = append(qArgs, *u.PapelAtivoID)
	}
	if u.GrupoID != nil {
		conds = append(conds, "alvo_grupo_id = ?")
		qArgs = append(qArgs, *u.GrupoID)
	}
	q += strings.Join(conds, " OR ") + `) ORDER BY pode_editar DESC LIMIT 1`

	errComp := a.st.db.QueryRow(q, qArgs...).Scan(&podeEditar)
	if errComp == nil {
		if !precisaEdicao || podeEditar == 1 {
			return true, grupoID, nil
		}
	}

	// Mesma Unidade: Operador tem leitura padrão do grupo
	if u.GrupoID != nil && *u.GrupoID == grupoID {
		if !precisaEdicao {
			return true, grupoID, nil
		}
	}

	// Onda 05/10 (dupla chave ADITIVA — DELIB-0010 B): a pasta é POSSESSO da
	// função que o usuário EXERCE (funcao_membros) → leitura garantida. Ex-
	// titular perde este braço (herança Q3) sem perder o que é seu (autor/
	// grants permanecem). Edição NÃO vem pela função (só leitura).
	if a.acessoViaFuncaoPasta(u, pastaID, precisaEdicao) {
		return true, grupoID, nil
	}

	// Herança de pasta pai
	if paiID != nil && *paiID > 0 {
		return a.checarAcessoPasta(u, *paiID, precisaEdicao)
	}

	return false, grupoID, nil
}

// checarAcessoArquivo valida se o usuário tem permissão sobre o arquivo físico.
func (a *App) checarAcessoArquivo(u *Usuario, arquivoID int64, precisaEdicao bool) (bool, map[string]any, error) {
	var pastaID *int64
	var grupoID int64
	var nomeOriginal, nomeArmazenado, tipo string
	var tamanho int64
	var autorUsuarioID, autorPapelID int64
	var criadoEm string

	err := a.st.db.QueryRow(`
		SELECT pasta_id, grupo_id, nome_original, nome_armazenado, tipo, tamanho, 
		       autor_usuario_id, autor_papel_id, criado_em
		FROM drive_arquivos WHERE id = ?
	`, arquivoID).Scan(&pastaID, &grupoID, &nomeOriginal, &nomeArmazenado, &tipo, &tamanho,
		&autorUsuarioID, &autorPapelID, &criadoEm)
	if err != nil {
		return false, nil, err
	}

	meta := map[string]any{
		"id":              arquivoID,
		"pasta_id":        pastaID,
		"grupo_id":        grupoID,
		"nome_original":   nomeOriginal,
		"nome_armazenado": nomeArmazenado,
		"tipo":            tipo,
		"tamanho":         tamanho,
		"autor_usuario_id": autorUsuarioID,
		"autor_papel_id":  autorPapelID,
		"criado_em":       criadoEm,
	}

	if u.Papel == "admin" {
		return false, nil, fmt.Errorf("administrador não possui acesso ao drive operacional")
	}
	if u.ID == autorUsuarioID {
		return true, meta, nil
	}

	// Gerente da Unidade ou Unidade Superior
	if u.Papel == "gerente" && u.GrupoID != nil {
		if grupoID == *u.GrupoID || int64Contem(a.gruposSubordinadosAtivos(*u.GrupoID), grupoID) {
			return true, meta, nil
		}
	}

	// Compartilhamento direto no arquivo
	var podeEditar int
	var qArgs []any
	q := `SELECT pode_editar FROM drive_compartilhamentos WHERE arquivo_id = ? AND (`
	qArgs = append(qArgs, arquivoID)
	conds := []string{"alvo_usuario_id = ?"}
	qArgs = append(qArgs, u.ID)
	if u.PapelAtivoID != nil {
		conds = append(conds, "alvo_papel_id = ?")
		qArgs = append(qArgs, *u.PapelAtivoID)
	}
	if u.GrupoID != nil {
		conds = append(conds, "alvo_grupo_id = ?")
		qArgs = append(qArgs, *u.GrupoID)
	}
	q += strings.Join(conds, " OR ") + `) ORDER BY pode_editar DESC LIMIT 1`

	errComp := a.st.db.QueryRow(q, qArgs...).Scan(&podeEditar)
	if errComp == nil {
		if !precisaEdicao || podeEditar == 1 {
			return true, meta, nil
		}
	}

	// Verificação via pasta ascendente se houver
	if pastaID != nil && *pastaID > 0 {
		ok, _, errP := a.checarAcessoPasta(u, *pastaID, precisaEdicao)
		if errP == nil && ok {
			return true, meta, nil
		}
	}

	// Onda 05/10 (dupla chave ADITIVA — DELIB-0010 B): arquivo é POSSESSO da
	// função que o usuário EXERCE → leitura garantida (herança Q3 p/ sucessor;
	// ex-titular perde SÓ este braço). Edição NÃO vem pela função (só leitura).
	if a.acessoViaFuncaoArquivo(u, arquivoID, precisaEdicao) {
		return true, meta, nil
	}

	// Onda 05/10 (item 4): ARQUIVO é item isolado — acesso = lista em
	// drive_compartilhamentos (usuário/papel/grupo) + autor + gerência.
	// A antiga cortesia "mesma unidade lê por padrão" foi removida: membro do
	// grupo SEM grant não vê nem baixa o arquivo (hDriveItens filtra por aqui;
	// download/anexos/herança de pasta não dependem desta regra).

	return false, meta, nil
}

// ---------- Handlers HTTP do Drive Local ----------

// GET /api/drive/itens?pasta_id={id}&compartilhados={0|1}
// DEPRECATADO na onda 05/10: a rota usa a.hDriveItens (onda_0510_drive_itens.go),
// com visibilidade decidida por item pelas funções canônicas de acesso. Corpo
// legado mantido como referência histórica — NÃO editar.
func (a *App) hDriveItensLegado(w http.ResponseWriter, r *http.Request) {
	u := usuarioDoCtx(r)
	if u.Papel == "admin" {
		jsonErro(w, http.StatusForbidden, "administrador não possui acesso ao drive operacional")
		return
	}
	pIDStr := r.URL.Query().Get("pasta_id")
	soCompartilhados := r.URL.Query().Get("compartilhados") == "1"

	var pastaID int64
	if pIDStr != "" && pIDStr != "0" {
		pid, err := strconv.ParseInt(pIDStr, 10, 64)
		if err == nil {
			pastaID = pid
		}
	}

	if pastaID > 0 {
		ok, _, err := a.checarAcessoPasta(u, pastaID, false)
		if err != nil || !ok {
			jsonErro(w, http.StatusForbidden, "sem permissão para acessar esta pasta")
			return
		}
	}

	// Breadcrumbs
	breadcrumbs := []map[string]any{
		{"id": 0, "nome": "Meu Drive"},
	}
	if pastaID > 0 {
		var curID int64 = pastaID
		var trilha []map[string]any
		for curID > 0 {
			var nome string
			var paiID *int64
			err := a.st.db.QueryRow(`SELECT nome, pai_id FROM drive_pastas WHERE id = ?`, curID).Scan(&nome, &paiID)
			if err != nil {
				break
			}
			trilha = append([]map[string]any{{"id": curID, "nome": nome}}, trilha...)
			if paiID != nil && *paiID > 0 {
				curID = *paiID
			} else {
				curID = 0
			}
		}
		breadcrumbs = append(breadcrumbs, trilha...)
	}

	type PastaItem struct {
		ID             int64  `json:"id"`
		Nome           string `json:"nome"`
		PaiID          *int64 `json:"pai_id"`
		GrupoID        int64  `json:"grupo_id"`
		AutorNome      string `json:"autor_nome"`
		CriadoEm       string `json:"criado_em"`
		QtdItens       int    `json:"qtd_itens"`
		Compartilhada  bool   `json:"compartilhada"`
		PodeEditar     bool   `json:"pode_editar"`
	}

	type ArquivoItem struct {
		ID             int64  `json:"id"`
		PastaID        *int64 `json:"pasta_id"`
		NomeOriginal   string `json:"nome_original"`
		Tipo           string `json:"tipo"`
		Tamanho        int64  `json:"tamanho"`
		CriadoEm       string `json:"criado_em"`
		AutorNome      string `json:"autor_nome"`
		Compartilhado  bool   `json:"compartilhado"`
		PodeEditar     bool   `json:"pode_editar"`
	}

	pastas := []PastaItem{}
	arquivos := []ArquivoItem{}

	if soCompartilhados {
		// Modo 'Compartilhados Comigo' (estilo Google Drive)
		qArgs := []any{u.ID}
		conds := []string{"dc.alvo_usuario_id = ?"}
		if u.PapelAtivoID != nil {
			conds = append(conds, "dc.alvo_papel_id = ?")
			qArgs = append(qArgs, *u.PapelAtivoID)
		}
		if u.GrupoID != nil {
			conds = append(conds, "dc.alvo_grupo_id = ?")
			qArgs = append(qArgs, *u.GrupoID)
		}
		filtroComp := strings.Join(conds, " OR ")

		// Pastas compartilhadas
		pRows, err := a.st.db.Query(`
			SELECT DISTINCT dp.id, dp.nome, dp.pai_id, dp.grupo_id, dp.criado_em,
			       COALESCE(NULLIF(u.nome_guerra,''), NULLIF(u.nome_completo,''), '—'),
			       dc.pode_editar
			FROM drive_compartilhamentos dc
			JOIN drive_pastas dp ON dp.id = dc.pasta_id
			LEFT JOIN usuarios u ON u.id = dp.autor_usuario_id
			WHERE (` + filtroComp + `)
			ORDER BY dp.nome ASC
		`, qArgs...)
		if err == nil {
			defer pRows.Close()
			for pRows.Next() {
				var p PastaItem
				var pEdit int
				_ = pRows.Scan(&p.ID, &p.Nome, &p.PaiID, &p.GrupoID, &p.CriadoEm, &p.AutorNome, &pEdit)
				p.Compartilhada = true
				p.PodeEditar = (pEdit == 1)
				pastas = append(pastas, p)
			}
		}

		// Arquivos compartilhados
		aRows, errA := a.st.db.Query(`
			SELECT DISTINCT da.id, da.pasta_id, da.nome_original, da.tipo, da.tamanho, da.criado_em,
			       COALESCE(NULLIF(u.nome_guerra,''), NULLIF(u.nome_completo,''), '—'),
			       dc.pode_editar
			FROM drive_compartilhamentos dc
			JOIN drive_arquivos da ON da.id = dc.arquivo_id
			LEFT JOIN usuarios u ON u.id = da.autor_usuario_id
			WHERE (` + filtroComp + `)
			ORDER BY da.criado_em DESC
		`, qArgs...)
		if errA == nil {
			defer aRows.Close()
			for aRows.Next() {
				var ar ArquivoItem
				var pEdit int
				_ = aRows.Scan(&ar.ID, &ar.PastaID, &ar.NomeOriginal, &ar.Tipo, &ar.Tamanho, &ar.CriadoEm, &ar.AutorNome, &pEdit)
				ar.Compartilhado = true
				ar.PodeEditar = (pEdit == 1)
				arquivos = append(arquivos, ar)
			}
		}
	} else {
		// Listagem normal de pasta ou raiz
		var pQuery string
		var pArgs []any
		if pastaID == 0 {
			pQuery = `
				SELECT dp.id, dp.nome, dp.pai_id, dp.grupo_id, dp.criado_em,
				       COALESCE(NULLIF(u.nome_guerra,''), NULLIF(u.nome_completo,''), '—'),
				       (SELECT COUNT(*) FROM drive_pastas sp WHERE sp.pai_id = dp.id) + 
				       (SELECT COUNT(*) FROM drive_arquivos sa WHERE sa.pasta_id = dp.id) AS qtd_itens,
				       (SELECT COUNT(*) FROM drive_compartilhamentos dc WHERE dc.pasta_id = dp.id) AS qtd_comp,
				       dp.autor_usuario_id
				FROM drive_pastas dp
				LEFT JOIN usuarios u ON u.id = dp.autor_usuario_id
				WHERE dp.pai_id IS NULL AND dp.grupo_id = ?
				ORDER BY dp.nome ASC
			`
			if u.GrupoID != nil {
				pArgs = append(pArgs, *u.GrupoID)
			} else {
				pArgs = append(pArgs, 0)
			}
		} else {
			pQuery = `
				SELECT dp.id, dp.nome, dp.pai_id, dp.grupo_id, dp.criado_em,
				       COALESCE(NULLIF(u.nome_guerra,''), NULLIF(u.nome_completo,''), '—'),
				       (SELECT COUNT(*) FROM drive_pastas sp WHERE sp.pai_id = dp.id) + 
				       (SELECT COUNT(*) FROM drive_arquivos sa WHERE sa.pasta_id = dp.id) AS qtd_itens,
				       (SELECT COUNT(*) FROM drive_compartilhamentos dc WHERE dc.pasta_id = dp.id) AS qtd_comp,
				       dp.autor_usuario_id
				FROM drive_pastas dp
				LEFT JOIN usuarios u ON u.id = dp.autor_usuario_id
				WHERE dp.pai_id = ?
				ORDER BY dp.nome ASC
			`
			pArgs = append(pArgs, pastaID)
		}

		var subs []int64
		if u.Papel == "gerente" && u.GrupoID != nil {
			subs = a.gruposSubordinadosAtivos(*u.GrupoID)
		}

		pRows, err := a.st.db.Query(pQuery, pArgs...)
		if err == nil {
			for pRows.Next() {
				var p PastaItem
				var nComp int
				var autorUID int64
				_ = pRows.Scan(&p.ID, &p.Nome, &p.PaiID, &p.GrupoID, &p.CriadoEm, &p.AutorNome, &p.QtdItens, &nComp, &autorUID)
				p.Compartilhada = nComp > 0
				p.PodeEditar = (u.ID == autorUID)
				if !p.PodeEditar && u.Papel == "gerente" && u.GrupoID != nil && (*u.GrupoID == p.GrupoID || int64Contem(subs, p.GrupoID)) {
					p.PodeEditar = true
				}
				pastas = append(pastas, p)
			}
			pRows.Close()
		}

		// Listar arquivos
		var aQuery string
		var aArgs []any
		if pastaID == 0 {
			aQuery = `
				SELECT da.id, da.pasta_id, da.nome_original, da.tipo, da.tamanho, da.criado_em,
				       COALESCE(NULLIF(u.nome_guerra,''), NULLIF(u.nome_completo,''), '—'),
				       (SELECT COUNT(*) FROM drive_compartilhamentos dc WHERE dc.arquivo_id = da.id) AS qtd_comp,
				       da.autor_usuario_id, da.grupo_id
				FROM drive_arquivos da
				LEFT JOIN usuarios u ON u.id = da.autor_usuario_id
				WHERE da.pasta_id IS NULL AND da.grupo_id = ?
				ORDER BY da.criado_em DESC
			`
			if u.GrupoID != nil {
				aArgs = append(aArgs, *u.GrupoID)
			} else {
				aArgs = append(aArgs, 0)
			}
		} else {
			aQuery = `
				SELECT da.id, da.pasta_id, da.nome_original, da.tipo, da.tamanho, da.criado_em,
				       COALESCE(NULLIF(u.nome_guerra,''), NULLIF(u.nome_completo,''), '—'),
				       (SELECT COUNT(*) FROM drive_compartilhamentos dc WHERE dc.arquivo_id = da.id) AS qtd_comp,
				       da.autor_usuario_id, da.grupo_id
				FROM drive_arquivos da
				LEFT JOIN usuarios u ON u.id = da.autor_usuario_id
				WHERE da.pasta_id = ?
				ORDER BY da.criado_em DESC
			`
			aArgs = append(aArgs, pastaID)
		}

		aRows, errA := a.st.db.Query(aQuery, aArgs...)
		if errA == nil {
			for aRows.Next() {
				var ar ArquivoItem
				var nComp int
				var autorUID int64
				var arGrupoID int64
				_ = aRows.Scan(&ar.ID, &ar.PastaID, &ar.NomeOriginal, &ar.Tipo, &ar.Tamanho, &ar.CriadoEm, &ar.AutorNome, &nComp, &autorUID, &arGrupoID)
				ar.Compartilhado = nComp > 0
				ar.PodeEditar = (u.ID == autorUID)
				if !ar.PodeEditar && u.Papel == "gerente" && u.GrupoID != nil && (*u.GrupoID == arGrupoID || int64Contem(subs, arGrupoID)) {
					ar.PodeEditar = true
				}
				arquivos = append(arquivos, ar)
			}
			aRows.Close()
		}
	}

	podeEditarAtual, _, _ := a.checarAcessoPasta(u, pastaID, true)

	jsonOK(w, map[string]any{
		"pasta_id":         pastaID,
		"breadcrumbs":      breadcrumbs,
		"pastas":           pastas,
		"arquivos":         arquivos,
		"pode_editar":      podeEditarAtual,
		"so_compartilhados": soCompartilhados,
	})
}

// POST /api/drive/pastas - Criar Pasta
func (a *App) hDrivePastasAdd(w http.ResponseWriter, r *http.Request) {
	u := usuarioDoCtx(r)
	if u.Papel == "admin" {
		jsonErro(w, http.StatusForbidden, "administrador não possui acesso ao drive operacional")
		return
	}
	var req struct {
		Nome    string `json:"nome"`
		PastaID *int64 `json:"pasta_id"`
		GrupoID *int64 `json:"grupo_id"`
	}
	if err := decodificar(r, &req); err != nil || strings.TrimSpace(req.Nome) == "" {
		jsonErro(w, http.StatusBadRequest, "nome da pasta é obrigatório")
		return
	}
	req.Nome = strings.TrimSpace(req.Nome)

	var grupoID int64
	if req.PastaID != nil && *req.PastaID > 0 {
		ok, gID, err := a.checarAcessoPasta(u, *req.PastaID, true)
		if err != nil || !ok {
			jsonErro(w, http.StatusForbidden, "sem permissão para criar pasta neste diretório")
			return
		}
		grupoID = gID
	} else {
		if u.GrupoID != nil {
			grupoID = *u.GrupoID
		} else {
			jsonErro(w, http.StatusBadRequest, "usuário sem grupo associado para criar pasta raiz")
			return
		}
	}

	res, err := a.st.db.Exec(`
		INSERT INTO drive_pastas (nome, grupo_id, pai_id, autor_usuario_id, autor_papel_id, funcao_id)
		VALUES (?, ?, ?, ?, ?, ?)
	`, req.Nome, grupoID, req.PastaID, u.ID, u.PapelAtivoID, a.funcaoTitularNoGrupo(u, grupoID))
	if err != nil {
		jsonErro(w, http.StatusInternalServerError, "falha ao criar pasta: "+err.Error())
		return
	}
	id, _ := res.LastInsertId()

	a.st.Auditoria(&u.ID, "drive_criar_pasta", "drive_pastas", &id, req.Nome, ipDe(r))
	jsonOK(w, map[string]any{"id": id, "nome": req.Nome, "grupo_id": grupoID})
}

// PATCH /api/drive/pastas/{id} - Renomear Pasta
func (a *App) hDrivePastasEdit(w http.ResponseWriter, r *http.Request) {
	u := usuarioDoCtx(r)
	if u.Papel == "admin" {
		jsonErro(w, http.StatusForbidden, "administrador não possui acesso ao drive operacional")
		return
	}
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id <= 0 {
		jsonErro(w, http.StatusBadRequest, "id inválido")
		return
	}

	ok, _, errAcesso := a.checarAcessoPasta(u, id, true)
	if errAcesso != nil || !ok {
		jsonErro(w, http.StatusForbidden, "sem permissão para editar esta pasta")
		return
	}

	var req struct {
		Nome string `json:"nome"`
	}
	if err := decodificar(r, &req); err != nil || strings.TrimSpace(req.Nome) == "" {
		jsonErro(w, http.StatusBadRequest, "novo nome é obrigatório")
		return
	}
	req.Nome = strings.TrimSpace(req.Nome)

	_, err = a.st.db.Exec(`UPDATE drive_pastas SET nome = ? WHERE id = ?`, req.Nome, id)
	if err != nil {
		jsonErro(w, http.StatusInternalServerError, "falha ao renomear: "+err.Error())
		return
	}

	a.st.Auditoria(&u.ID, "drive_renomear_pasta", "drive_pastas", &id, req.Nome, ipDe(r))
	jsonOK(w, map[string]any{"id": id, "nome": req.Nome})
}

// DELETE /api/drive/pastas/{id} - Excluir Pasta e Arquivos Físicos Recursivamente
func (a *App) hDrivePastasDel(w http.ResponseWriter, r *http.Request) {
	u := usuarioDoCtx(r)
	if u.Papel == "admin" {
		jsonErro(w, http.StatusForbidden, "administrador não possui acesso ao drive operacional")
		return
	}
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id <= 0 {
		jsonErro(w, http.StatusBadRequest, "id inválido")
		return
	}

	ok, _, errAcesso := a.checarAcessoPasta(u, id, true)
	if errAcesso != nil || !ok {
		jsonErro(w, http.StatusForbidden, "sem permissão para excluir esta pasta")
		return
	}

	// Buscar todos os arquivos físicos descendentes para exclusão segura do disco
	var buscarArquivosDescendentes func(pID int64) []string
	buscarArquivosDescendentes = func(pID int64) []string {
		var nomes []string
		rows, err := a.st.db.Query(`SELECT nome_armazenado FROM drive_arquivos WHERE pasta_id = ?`, pID)
		if err == nil {
			for rows.Next() {
				var n string
				_ = rows.Scan(&n)
				nomes = append(nomes, n)
			}
			rows.Close()
		}
		subRows, errS := a.st.db.Query(`SELECT id FROM drive_pastas WHERE pai_id = ?`, pID)
		if errS == nil {
			var subs []int64
			for subRows.Next() {
				var s int64
				_ = subRows.Scan(&s)
				subs = append(subs, s)
			}
			subRows.Close()
			for _, s := range subs {
				nomes = append(nomes, buscarArquivosDescendentes(s)...)
			}
		}
		return nomes
	}

	arquivosFisicos := buscarArquivosDescendentes(id)

	// Excluir registro no banco (CASCADE remove subpastas, arquivos e compartilhamentos)
	_, err = a.st.db.Exec(`DELETE FROM drive_pastas WHERE id = ?`, id)
	if err != nil {
		jsonErro(w, http.StatusInternalServerError, "falha ao remover pasta do banco: "+err.Error())
		return
	}

	// Excluir arquivos físicos do disco
	pastaFisica := a.pastaFisicaDrive()
	for _, n := range arquivosFisicos {
		_ = os.Remove(filepath.Join(pastaFisica, n))
	}

	a.st.Auditoria(&u.ID, "drive_excluir_pasta", "drive_pastas", &id, fmt.Sprintf("%d arquivos físicos removidos", len(arquivosFisicos)), ipDe(r))
	jsonOK(w, map[string]any{"ok": true, "arquivos_removidos": len(arquivosFisicos)})
}

// POST /api/drive/upload - Upload Físico de Arquivo para ./dados/drive
func (a *App) hDriveUpload(w http.ResponseWriter, r *http.Request) {
	u := usuarioDoCtx(r)
	if u.Papel == "admin" {
		jsonErro(w, http.StatusForbidden, "administrador não possui acesso ao drive operacional")
		return
	}

	// Limite de 128 MB por upload multipart
	if err := r.ParseMultipartForm(128 << 20); err != nil {
		jsonErro(w, http.StatusBadRequest, "falha ao processar upload: tamanho excessivo ou formato incorreto")
		return
	}

	file, header, err := r.FormFile("arquivo")
	if err != nil {
		jsonErro(w, http.StatusBadRequest, "campo 'arquivo' ausente")
		return
	}
	defer file.Close()

	var pastaID *int64
	pIDStr := r.FormValue("pasta_id")
	if pIDStr != "" && pIDStr != "0" {
		pid, err := strconv.ParseInt(pIDStr, 10, 64)
		if err == nil && pid > 0 {
			pastaID = &pid
		}
	}

	var grupoID int64
	if pastaID != nil {
		ok, gID, err := a.checarAcessoPasta(u, *pastaID, true)
		if err != nil || !ok {
			jsonErro(w, http.StatusForbidden, "sem permissão para enviar arquivo para esta pasta")
			return
		}
		grupoID = gID
	} else {
		if u.GrupoID != nil {
			grupoID = *u.GrupoID
		} else {
			jsonErro(w, http.StatusBadRequest, "usuário sem grupo associado para envio na raiz")
			return
		}
	}

	nomeOriginal := header.Filename
	if nomeOriginal == "" {
		nomeOriginal = "sem_nome"
	}
	// Sanitizar extensão e gerar identificador único físico
	ext := filepath.Ext(nomeOriginal)
	if len(ext) > 10 {
		ext = ""
	}
	nomeArmazenado := fmt.Sprintf("%d_%s%s", time.Now().UnixNano(), hexAleatorioDrive(8), ext)

	pastaFisica := a.pastaFisicaDrive()
	caminhoCompleto := filepath.Join(pastaFisica, nomeArmazenado)

	dst, err := os.Create(caminhoCompleto)
	if err != nil {
		jsonErro(w, http.StatusInternalServerError, "falha ao criar arquivo no disco local: "+err.Error())
		return
	}
	defer dst.Close()

	tam, err := io.Copy(dst, file)
	if err != nil {
		_ = os.Remove(caminhoCompleto)
		jsonErro(w, http.StatusInternalServerError, "falha ao gravar conteúdo físico: "+err.Error())
		return
	}

	tipoMime := header.Header.Get("Content-Type")
	if tipoMime == "" {
		tipoMime = mime.TypeByExtension(ext)
		if tipoMime == "" {
			tipoMime = "application/octet-stream"
		}
	}

	res, err := a.st.db.Exec(`
		INSERT INTO drive_arquivos (pasta_id, grupo_id, nome_original, nome_armazenado, tipo, tamanho, autor_usuario_id, autor_papel_id, funcao_id)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, pastaID, grupoID, nomeOriginal, nomeArmazenado, tipoMime, tam, u.ID, u.PapelAtivoID, a.funcaoTitularNoGrupo(u, grupoID))
	if err != nil {
		_ = os.Remove(caminhoCompleto)
		jsonErro(w, http.StatusInternalServerError, "falha ao cadastrar metadados do arquivo: "+err.Error())
		return
	}
	arqID, _ := res.LastInsertId()

	a.st.Auditoria(&u.ID, "drive_upload", "drive_arquivos", &arqID, fmt.Sprintf("%s (%d bytes)", nomeOriginal, tam), ipDe(r))

	jsonOK(w, map[string]any{
		"id":            arqID,
		"nome_original": nomeOriginal,
		"tamanho":       tam,
		"tipo":          tipoMime,
		"pasta_id":      pastaID,
		"grupo_id":      grupoID,
	})
}

// GET /api/drive/download/{id} - Download ou Visualização em Stream Físico
func (a *App) hDriveDownload(w http.ResponseWriter, r *http.Request) {
	u := usuarioDoCtx(r)
	if u.Papel == "admin" {
		jsonErro(w, http.StatusForbidden, "administrador não possui acesso ao drive operacional")
		return
	}
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id <= 0 {
		jsonErro(w, http.StatusBadRequest, "id inválido")
		return
	}

	ok, meta, errAcesso := a.checarAcessoArquivo(u, id, false)
	if errAcesso != nil || !ok {
		jsonErro(w, http.StatusForbidden, "sem permissão para baixar este arquivo")
		return
	}

	nomeArmazenado := meta["nome_armazenado"].(string)
	nomeOriginal := meta["nome_original"].(string)
	tipo := meta["tipo"].(string)
	tamanho := meta["tamanho"].(int64)

	caminhoFisico := filepath.Join(a.pastaFisicaDrive(), nomeArmazenado)
	f, err := os.Open(caminhoFisico)
	if err != nil {
		jsonErro(w, http.StatusNotFound, "arquivo físico não encontrado no servidor")
		return
	}
	defer f.Close()

	inline := r.URL.Query().Get("inline") == "1"
	disp := "attachment"
	if inline {
		disp = "inline"
	}

	w.Header().Set("Content-Type", tipo)
	w.Header().Set("Content-Disposition", fmt.Sprintf(`%s; filename="%s"`, disp, nomeOriginal))
	w.Header().Set("Content-Length", strconv.FormatInt(tamanho, 10))

	http.ServeContent(w, r, nomeOriginal, time.Now(), f)
}

// PATCH /api/drive/arquivos/{id} - Renomear Arquivo
func (a *App) hDriveArquivosEdit(w http.ResponseWriter, r *http.Request) {
	u := usuarioDoCtx(r)
	if u.Papel == "admin" {
		jsonErro(w, http.StatusForbidden, "administrador não possui acesso ao drive operacional")
		return
	}
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id <= 0 {
		jsonErro(w, http.StatusBadRequest, "id inválido")
		return
	}

	ok, _, errAcesso := a.checarAcessoArquivo(u, id, true)
	if errAcesso != nil || !ok {
		jsonErro(w, http.StatusForbidden, "sem permissão para renomear este arquivo")
		return
	}

	var req struct {
		Nome string `json:"nome"`
	}
	if err := decodificar(r, &req); err != nil || strings.TrimSpace(req.Nome) == "" {
		jsonErro(w, http.StatusBadRequest, "novo nome é obrigatório")
		return
	}
	novoNome := strings.TrimSpace(req.Nome)

	_, err = a.st.db.Exec(`UPDATE drive_arquivos SET nome_original = ? WHERE id = ?`, novoNome, id)
	if err != nil {
		jsonErro(w, http.StatusInternalServerError, "falha ao atualizar: "+err.Error())
		return
	}

	a.st.Auditoria(&u.ID, "drive_renomear_arquivo", "drive_arquivos", &id, novoNome, ipDe(r))
	jsonOK(w, map[string]any{"id": id, "nome_original": novoNome})
}

// DELETE /api/drive/arquivos/{id} - Excluir Arquivo Físico e Registro
func (a *App) hDriveArquivosDel(w http.ResponseWriter, r *http.Request) {
	u := usuarioDoCtx(r)
	if u.Papel == "admin" {
		jsonErro(w, http.StatusForbidden, "administrador não possui acesso ao drive operacional")
		return
	}
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id <= 0 {
		jsonErro(w, http.StatusBadRequest, "id inválido")
		return
	}

	ok, meta, errAcesso := a.checarAcessoArquivo(u, id, true)
	if errAcesso != nil || !ok {
		jsonErro(w, http.StatusForbidden, "sem permissão para excluir este arquivo")
		return
	}

	nomeArmazenado := meta["nome_armazenado"].(string)

	_, err = a.st.db.Exec(`DELETE FROM drive_arquivos WHERE id = ?`, id)
	if err != nil {
		jsonErro(w, http.StatusInternalServerError, "falha ao remover registro: "+err.Error())
		return
	}

	caminhoFisico := filepath.Join(a.pastaFisicaDrive(), nomeArmazenado)
	_ = os.Remove(caminhoFisico)

	a.st.Auditoria(&u.ID, "drive_excluir_arquivo", "drive_arquivos", &id, meta["nome_original"].(string), ipDe(r))
	jsonOK(w, map[string]any{"ok": true})
}

// POST /api/drive/compartilhar - Conceder Permissão Estilo Google Drive
func (a *App) hDriveCompartilhar(w http.ResponseWriter, r *http.Request) {
	u := usuarioDoCtx(r)
	if u.Papel == "admin" {
		jsonErro(w, http.StatusForbidden, "administrador não possui acesso ao drive operacional")
		return
	}

	var req struct {
		PastaID       *int64 `json:"pasta_id"`
		ArquivoID     *int64 `json:"arquivo_id"`
		AlvoTipo      string `json:"alvo_tipo"` // "grupo", "usuario", "papel"
		AlvoID        int64  `json:"alvo_id"`
		PodeEditar    bool   `json:"pode_editar"`
	}
	if err := decodificar(r, &req); err != nil || (req.PastaID == nil && req.ArquivoID == nil) || req.AlvoID <= 0 {
		jsonErro(w, http.StatusBadRequest, "parâmetros de compartilhamento inválidos")
		return
	}

	if req.PastaID != nil {
		ok, _, err := a.checarAcessoPasta(u, *req.PastaID, true)
		if err != nil || !ok {
			jsonErro(w, http.StatusForbidden, "sem permissão para compartilhar esta pasta")
			return
		}
	} else if req.ArquivoID != nil {
		ok, _, err := a.checarAcessoArquivo(u, *req.ArquivoID, true)
		if err != nil || !ok {
			jsonErro(w, http.StatusForbidden, "sem permissão para compartilhar este arquivo")
			return
		}
	}

	var alvoGrupoID *int64
	var alvoUsuarioID *int64
	var alvoPapelID *int64

	switch req.AlvoTipo {
	case "grupo":
		alvoGrupoID = &req.AlvoID
	case "usuario":
		alvoUsuarioID = &req.AlvoID
	case "papel":
		alvoPapelID = &req.AlvoID
	default:
		jsonErro(w, http.StatusBadRequest, "alvo_tipo deve ser 'grupo', 'usuario' ou 'papel'")
		return
	}

	podeEdVal := 0
	if req.PodeEditar {
		podeEdVal = 1
	}

	res, err := a.st.db.Exec(`
		INSERT INTO drive_compartilhamentos (pasta_id, arquivo_id, alvo_grupo_id, alvo_usuario_id, alvo_papel_id, pode_editar)
		VALUES (?, ?, ?, ?, ?, ?)
	`, req.PastaID, req.ArquivoID, alvoGrupoID, alvoUsuarioID, alvoPapelID, podeEdVal)
	if err != nil {
		jsonErro(w, http.StatusInternalServerError, "falha ao registrar compartilhamento: "+err.Error())
		return
	}
	compId, _ := res.LastInsertId()

	a.st.Auditoria(&u.ID, "drive_compartilhar", "drive_compartilhamentos", &compId,
		fmt.Sprintf("alvo=%s:%d pode_editar=%v", req.AlvoTipo, req.AlvoID, req.PodeEditar), ipDe(r))

	jsonOK(w, map[string]any{"id": compId, "ok": true})
}

// GET /api/drive/compartilhamentos?pasta_id={id}&arquivo_id={id} - Listar Permissões Ativas
// Onda 05/10: devolve também criado_em_fmt (Brasília) — fonte da tabela de acessos
// nas propriedades; e é exigido acesso de LEITURA ao item (antes só checava o alvo dado).
func (a *App) hDriveCompartilhamentosList(w http.ResponseWriter, r *http.Request) {
	u := usuarioDoCtx(r)
	if u.Papel == "admin" {
		jsonErro(w, http.StatusForbidden, "administrador não possui acesso ao drive operacional")
		return
	}
	pStr := r.URL.Query().Get("pasta_id")
	aStr := r.URL.Query().Get("arquivo_id")

	var pastaID, arquivoID *int64
	if pStr != "" {
		pid, _ := strconv.ParseInt(pStr, 10, 64)
		if pid > 0 {
			pastaID = &pid
			ok, _, err := a.checarAcessoPasta(u, pid, false)
			if err != nil || !ok {
				jsonErro(w, http.StatusForbidden, "sem permissão")
				return
			}
		}
	}
	if aStr != "" {
		aid, _ := strconv.ParseInt(aStr, 10, 64)
		if aid > 0 {
			arquivoID = &aid
			ok, _, err := a.checarAcessoArquivo(u, aid, false)
			if err != nil || !ok {
				jsonErro(w, http.StatusForbidden, "sem permissão")
				return
			}
		}
	}

	if pastaID == nil && arquivoID == nil {
		jsonErro(w, http.StatusBadRequest, "informe pasta_id ou arquivo_id")
		return
	}

	type CompItem struct {
		ID          int64  `json:"id"`
		AlvoTipo    string `json:"alvo_tipo"`
		AlvoNome    string `json:"alvo_nome"`
		PodeEditar  bool   `json:"pode_editar"`
		CriadoEm    string `json:"criado_em"`
		CriadoEmFmt string `json:"criado_em_fmt"`
	}

	var itens []CompItem

	var q string
	var arg int64
	if pastaID != nil {
		q = `
			SELECT dc.id, dc.pode_editar, dc.criado_em,
			       CASE 
			         WHEN dc.alvo_grupo_id IS NOT NULL THEN 'grupo'
			         WHEN dc.alvo_usuario_id IS NOT NULL THEN 'usuario'
			         WHEN dc.alvo_papel_id IS NOT NULL THEN 'papel'
			         ELSE 'outro'
			       END as alvo_tipo,
			       COALESCE(g.nome, u.nome_guerra, u.login, up.papel, '—') as alvo_nome
			FROM drive_compartilhamentos dc
			LEFT JOIN grupos g ON g.id = dc.alvo_grupo_id
			LEFT JOIN usuarios u ON u.id = dc.alvo_usuario_id
			LEFT JOIN usuario_papeis up ON up.id = dc.alvo_papel_id
			WHERE dc.pasta_id = ?
		`
		arg = *pastaID
	} else {
		q = `
			SELECT dc.id, dc.pode_editar, dc.criado_em,
			       CASE 
			         WHEN dc.alvo_grupo_id IS NOT NULL THEN 'grupo'
			         WHEN dc.alvo_usuario_id IS NOT NULL THEN 'usuario'
			         WHEN dc.alvo_papel_id IS NOT NULL THEN 'papel'
			         ELSE 'outro'
			       END as alvo_tipo,
			       COALESCE(g.nome, u.nome_guerra, u.login, up.papel, '—') as alvo_nome
			FROM drive_compartilhamentos dc
			LEFT JOIN grupos g ON g.id = dc.alvo_grupo_id
			LEFT JOIN usuarios u ON u.id = dc.alvo_usuario_id
			LEFT JOIN usuario_papeis up ON up.id = dc.alvo_papel_id
			WHERE dc.arquivo_id = ?
		`
		arg = *arquivoID
	}

	rws, err := a.st.db.Query(q, arg)
	if err == nil {
		defer rws.Close()
		for rws.Next() {
			var it CompItem
			var pEd int
			_ = rws.Scan(&it.ID, &pEd, &it.CriadoEm, &it.AlvoTipo, &it.AlvoNome)
			it.PodeEditar = pEd == 1
			it.CriadoEmFmt = driveFmtDataHora(it.CriadoEm)
			itens = append(itens, it)
		}
	}

	jsonOK(w, map[string]any{"compartilhamentos": itens})
}

// DELETE /api/drive/compartilhamentos/{id} - Revogar Compartilhamento
func (a *App) hDriveCompartilhamentosDel(w http.ResponseWriter, r *http.Request) {
	u := usuarioDoCtx(r)
	if u.Papel == "admin" {
		jsonErro(w, http.StatusForbidden, "administrador não possui acesso ao drive operacional")
		return
	}
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id <= 0 {
		jsonErro(w, http.StatusBadRequest, "id inválido")
		return
	}

	var pastaID, arquivoID *int64
	err = a.st.db.QueryRow(`SELECT pasta_id, arquivo_id FROM drive_compartilhamentos WHERE id = ?`, id).Scan(&pastaID, &arquivoID)
	if err != nil {
		jsonErro(w, http.StatusNotFound, "compartilhamento não encontrado")
		return
	}

	if pastaID != nil {
		ok, _, errP := a.checarAcessoPasta(u, *pastaID, true)
		if errP != nil || !ok {
			jsonErro(w, http.StatusForbidden, "sem permissão para revogar compartilhamento desta pasta")
			return
		}
	} else if arquivoID != nil {
		ok, _, errA := a.checarAcessoArquivo(u, *arquivoID, true)
		if errA != nil || !ok {
			jsonErro(w, http.StatusForbidden, "sem permissão para revogar compartilhamento deste arquivo")
			return
		}
	}

	_, err = a.st.db.Exec(`DELETE FROM drive_compartilhamentos WHERE id = ?`, id)
	if err != nil {
		jsonErro(w, http.StatusInternalServerError, "falha ao revogar: "+err.Error())
		return
	}

	a.st.Auditoria(&u.ID, "drive_revogar_compartilhamento", "drive_compartilhamentos", &id, "", ipDe(r))
	jsonOK(w, map[string]any{"ok": true})
}
