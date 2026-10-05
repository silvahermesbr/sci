package main

// Onda 05/10 — item 4 da ordem: MODELO DE ACESSO do Drive (verificado e alinhado).
//
// ARQUIVO = item isolado com identificador único (drive_arquivos.id);
// nome_armazenado UNIQUE é apenas o nome do físico em dados/drive — a cópia
// (onda_0510_drive.go) cria NOVO item referenciando o MESMO físico.
// ACESSO = lista em drive_compartilhamentos (alvo_usuario_id / alvo_papel_id /
// alvo_grupo_id, criado_em, pode_editar).
//
// hDriveItens (GET /api/drive/itens) devolve EXATAMENTE os itens:
//   próprios (autor) ∪ gerenciados pelo gerente (grupo próprio + subordinados
//   ativos) ∪ compartilhados direto com o usuário/seu papel/seu grupo ∪ dentro
//   de pasta acessível por herança — exatamente o que checarAcessoPasta /
//   checarAcessoArquivo aceitam, porque a visibilidade de cada linha é decidida
//   POR ESSAS FUNÇÕES (a versão anterior filtrava por grupo no SQL + Scan que
//   abortava linhas silenciosamente).
// A raiz passa a mostrar também itens de outro grupo compartilhados direto com
// o usuário (antes só apareciam na aba Compartilhados). Aba ?compartilhados=1
// continua = grants DIRETOS, agora deduplicados por id.

import (
	"net/http"
	"strconv"
	"strings"
)

func (a *App) hDriveItens(w http.ResponseWriter, r *http.Request) {
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
	breadcrumbs := []map[string]any{{"id": 0, "nome": "Meu Drive"}}
	if pastaID > 0 {
		var trilha []map[string]any
		curID := pastaID
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
		ID            int64  `json:"id"`
		Nome          string `json:"nome"`
		PaiID         *int64 `json:"pai_id"`
		GrupoID       int64  `json:"grupo_id"`
		AutorNome     string `json:"autor_nome"`
		CriadoEm      string `json:"criado_em"`
		QtdItens      int    `json:"qtd_itens"`
		Compartilhada bool   `json:"compartilhada"`
		PodeEditar    bool   `json:"pode_editar"`
	}

	type ArquivoItem struct {
		ID            int64  `json:"id"`
		PastaID       *int64 `json:"pasta_id"`
		NomeOriginal  string `json:"nome_original"`
		Tipo          string `json:"tipo"`
		Tamanho       int64  `json:"tamanho"`
		CriadoEm      string `json:"criado_em"`
		AutorNome     string `json:"autor_nome"`
		Compartilhado bool   `json:"compartilhado"`
		PodeEditar    bool   `json:"pode_editar"`
	}

	pastas := []PastaItem{}
	arquivos := []ArquivoItem{}

	if soCompartilhados {
		// Aba "Compartilhados Comigo": grants DIRETOS (usuário/papel/grupo),
		// deduplicados por id (o mesmo item pode ter mais de um grant).
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

		pRows, err := a.st.db.Query(`
			SELECT dp.id, dp.nome, dp.pai_id, dp.grupo_id, dp.criado_em,
			       COALESCE(u2.nome_guerra, u2.login, '—'), MAX(dc.pode_editar)
			FROM drive_compartilhamentos dc
			JOIN drive_pastas dp ON dp.id = dc.pasta_id
			LEFT JOIN usuarios u2 ON u2.id = dp.autor_usuario_id
			WHERE (` + filtroComp + `)
			GROUP BY dp.id
			ORDER BY dp.nome ASC
		`, qArgs...)
		if err == nil {
			for pRows.Next() {
				var p PastaItem
				var pEdit int
				if errS := pRows.Scan(&p.ID, &p.Nome, &p.PaiID, &p.GrupoID, &p.CriadoEm, &p.AutorNome, &pEdit); errS == nil {
					p.Compartilhada = true
					p.PodeEditar = pEdit == 1
					pastas = append(pastas, p)
				}
			}
			pRows.Close()
		}

		aRows, errA := a.st.db.Query(`
			SELECT da.id, da.pasta_id, da.nome_original, da.tipo, da.tamanho, da.criado_em,
			       COALESCE(u2.nome_guerra, u2.login, '—'), MAX(dc.pode_editar)
			FROM drive_compartilhamentos dc
			JOIN drive_arquivos da ON da.id = dc.arquivo_id
			LEFT JOIN usuarios u2 ON u2.id = da.autor_usuario_id
			WHERE (` + filtroComp + `)
			GROUP BY da.id
			ORDER BY da.criado_em DESC
		`, qArgs...)
		if errA == nil {
			for aRows.Next() {
				var ar ArquivoItem
				var pEdit int
				if errS := aRows.Scan(&ar.ID, &ar.PastaID, &ar.NomeOriginal, &ar.Tipo, &ar.Tamanho, &ar.CriadoEm, &ar.AutorNome, &pEdit); errS == nil {
					ar.Compartilhado = true
					ar.PodeEditar = pEdit == 1
					arquivos = append(arquivos, ar)
				}
			}
			aRows.Close()
		}
	} else {
		// Listagem de pasta/raiz: visibilidade por item decidida pelas funções
		// canônicas de acesso (autor, gerência com subordinados, grant direto,
		// herança de pasta pai, leitura da mesma unidade).
		scopeP := "dp.pai_id IS NULL"
		pArgs := []any{}
		if pastaID > 0 {
			scopeP = "dp.pai_id = ?"
			pArgs = append(pArgs, pastaID)
		}

		pRows, err := a.st.db.Query(`
			SELECT dp.id, dp.nome, dp.pai_id, dp.grupo_id, dp.criado_em,
			       COALESCE(u2.nome_guerra, u2.login, '—'),
			       (SELECT COUNT(*) FROM drive_pastas sp WHERE sp.pai_id = dp.id) +
			       (SELECT COUNT(*) FROM drive_arquivos sa WHERE sa.pasta_id = dp.id) AS qtd_itens,
			       (SELECT COUNT(*) FROM drive_compartilhamentos dc WHERE dc.pasta_id = dp.id) AS qtd_comp
			FROM drive_pastas dp
			LEFT JOIN usuarios u2 ON u2.id = dp.autor_usuario_id
			WHERE ` + scopeP + `
			ORDER BY dp.nome ASC
		`, pArgs...)
		if err == nil {
			// Bufferiza ANTES de checar acesso por item: pool SQLite é de conexão
			// única — query aninhada com rows abertas = deadlock (lição da casa).
			type pastaRow = PastaItem
			var brutas []pastaRow
			for pRows.Next() {
				var p PastaItem
				var nComp int
				if errS := pRows.Scan(&p.ID, &p.Nome, &p.PaiID, &p.GrupoID, &p.CriadoEm, &p.AutorNome, &p.QtdItens, &nComp); errS != nil {
					continue
				}
				p.Compartilhada = nComp > 0
				brutas = append(brutas, p)
			}
			pRows.Close()
			for _, p := range brutas {
				okV, _, _ := a.checarAcessoPasta(u, p.ID, false)
				if !okV {
					continue
				}
				okE, _, _ := a.checarAcessoPasta(u, p.ID, true)
				p.PodeEditar = okE
				pastas = append(pastas, p)
			}
		}

		scopeA := "da.pasta_id IS NULL"
		aArgs := []any{}
		if pastaID > 0 {
			scopeA = "da.pasta_id = ?"
			aArgs = append(aArgs, pastaID)
		}

		aRows, errA := a.st.db.Query(`
			SELECT da.id, da.pasta_id, da.nome_original, da.tipo, da.tamanho, da.criado_em,
			       COALESCE(u2.nome_guerra, u2.login, '—'),
			       (SELECT COUNT(*) FROM drive_compartilhamentos dc WHERE dc.arquivo_id = da.id) AS qtd_comp
			FROM drive_arquivos da
			LEFT JOIN usuarios u2 ON u2.id = da.autor_usuario_id
			WHERE ` + scopeA + `
			ORDER BY da.criado_em DESC
		`, aArgs...)
		if errA == nil {
			type arqRow = ArquivoItem
			var brutas []arqRow
			for aRows.Next() {
				var ar ArquivoItem
				var nComp int
				if errS := aRows.Scan(&ar.ID, &ar.PastaID, &ar.NomeOriginal, &ar.Tipo, &ar.Tamanho, &ar.CriadoEm, &ar.AutorNome, &nComp); errS != nil {
					continue
				}
				ar.Compartilhado = nComp > 0
				brutas = append(brutas, ar)
			}
			aRows.Close()
			for _, ar := range brutas {
				okV, _, _ := a.checarAcessoArquivo(u, ar.ID, false)
				if !okV {
					continue
				}
				okE, _, _ := a.checarAcessoArquivo(u, ar.ID, true)
				ar.PodeEditar = okE
				arquivos = append(arquivos, ar)
			}
		}
	}

	podeEditarAtual, _, _ := a.checarAcessoPasta(u, pastaID, true)

	jsonOK(w, map[string]any{
		"pasta_id":          pastaID,
		"breadcrumbs":       breadcrumbs,
		"pastas":            pastas,
		"arquivos":          arquivos,
		"pode_editar":       podeEditarAtual,
		"so_compartilhados": soCompartilhados,
	})
}
