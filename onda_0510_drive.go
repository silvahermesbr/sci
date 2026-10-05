/* SCI — Onda 05/10: mover/copiar arquivo, propriedades e modelo de acesso do Drive.

   1. MOVER / COPIAR — POST /api/drive/arquivos/{id}/mover|copiar, corpo {pasta_id}:
      mover = UPDATE drive_arquivos SET pasta_id (sem copiar bytes);
      copiar = NOVO item com CÓPIA FÍSICA própria em dados/drive (nome_armazenado é
      UNIQUE no schema — reutilizar o físico exigiria migração; itens ficam
      independentes: excluir um não afeta o outro). Grants de compartilhamento NÃO
      migram para a cópia (herança do documento original permanece no original).
   2. MODELO DE ACESSO (item 4 da ordem — verificado e alinhado, sem reinventar):
      - ARQUIVO = item isolado com identificador único (id); nome_armazenado UNIQUE
        é o nome do físico em disco — 1 item = 1 físico.
      - ACESSO = lista em drive_compartilhamentos (alvo_usuario_id / alvo_papel_id /
        alvo_grupo_id, criado_em, pode_editar).
      - GET /api/drive/compartilhamentos devolve também criado_em_fmt (Brasília) e
        alimenta a TABELA DE ACESSOS no modal de PROPRIEDADES do front.
*/

package main

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// driveFmtDataHora: RFC3339 (UTC real do banco) → "dd/mm/aaaa hh:mm" em Brasília.
func driveFmtDataHora(s string) string {
	if strings.TrimSpace(s) == "" {
		return ""
	}
	loc, err := time.LoadLocation("America/Sao_Paulo")
	if err != nil {
		return s
	}
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		return s
	}
	return t.In(loc).Format("02/01/2006 15:04")
}

// destinoValidoDrive: checagem de permissão de ESCRITA no destino (0 = raiz do próprio grupo).
func destinoValidoDrive(a *App, u *Usuario, pastaID int64) bool {
	if pastaID == 0 {
		return u.GrupoID != nil
	}
	ok, _, err := a.checarAcessoPasta(u, pastaID, true)
	return err == nil && ok
}

// destinoDriveJSON: nome do destino para o payload de resposta/auditoria.
func destinoDriveJSON(a *App, pastaID int64) map[string]any {
	destino := map[string]any{"pasta_id": pastaID, "nome": ""}
	if pastaID > 0 {
		var nome string
		if err := a.st.db.QueryRow(`SELECT nome FROM drive_pastas WHERE id = ?`, pastaID).Scan(&nome); err == nil {
			destino["nome"] = nome
		}
	}
	return destino
}

// POST /api/drive/arquivos/{id}/mover — muda a pasta do arquivo (registro único, sem cópia física).
func (a *App) hDriveArquivoMover(w http.ResponseWriter, r *http.Request) {
	a.driveMoverCopiar(w, r, false)
}

// POST /api/drive/arquivos/{id}/copiar — NOVO item com cópia física própria em dados/drive.
func (a *App) hDriveArquivoCopiar(w http.ResponseWriter, r *http.Request) {
	a.driveMoverCopiar(w, r, true)
}

// copiarFisicoDrive: duplica o arquivo físico em dados/drive com nome único novo
// (mesmo padrão do upload: nanotimestamp + hex + ext). Stream, sem carregar em RAM.
// Devolve o NOME novo e os bytes efetivamente gravados — o INSERT registra o
// tamanho REAL do físico, nunca o metadado antigo do item de origem.
func copiarFisicoDrive(a *App, nomeArmazenado, ext string) (string, int64, error) {
	if len(ext) > 10 {
		ext = ""
	}
	novo := fmt.Sprintf("%d_%s%s", time.Now().UnixNano(), hexAleatorioDrive(8), ext)
	src, err := os.Open(filepath.Join(a.pastaFisicaDrive(), nomeArmazenado))
	if err != nil {
		return "", 0, err
	}
	defer src.Close()
	dst, err := os.Create(filepath.Join(a.pastaFisicaDrive(), novo))
	if err != nil {
		return "", 0, err
	}
	n, errC := io.Copy(dst, src)
	if errC != nil {
		_ = dst.Close()
		_ = os.Remove(filepath.Join(a.pastaFisicaDrive(), novo))
		return "", 0, errC
	}
	// Close em disco: sem ele (ou falhando), bytes ficam no buffer e o físico
	// pode nascer truncado/ausente — gravidade real da gravação é o Close.
	if errF := dst.Close(); errF != nil {
		_ = os.Remove(filepath.Join(a.pastaFisicaDrive(), novo))
		return "", 0, errF
	}
	return novo, n, nil
}

func (a *App) driveMoverCopiar(w http.ResponseWriter, r *http.Request, copia bool) {
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

	var req struct {
		PastaID int64 `json:"pasta_id"`
	}
	if err := decodificar(r, &req); err != nil || req.PastaID < 0 {
		jsonErro(w, http.StatusBadRequest, "pasta_id inválido")
		return
	}

	ok, meta, errAcesso := a.checarAcessoArquivo(u, id, true)
	if errAcesso != nil || !ok {
		jsonErro(w, http.StatusForbidden, "sem permissão para alterar este arquivo")
		return
	}
	if !destinoValidoDrive(a, u, req.PastaID) {
		jsonErro(w, http.StatusForbidden, "sem permissão para usar a pasta de destino")
		return
	}

	acao := "drive_mover_arquivo"
	novoPastaID := req.PastaID
	var novoID int64
	if copia {
		acao = "drive_copiar_arquivo"
		// Cópia FÍSICA própria: nome_armazenado é UNIQUE no schema — cada item
		// referencia 1 físico próprio em dados/drive (sem migração necessária).
		ext := filepath.Ext(meta["nome_original"].(string))
		novoNomeArm, nBytes, errC := copiarFisicoDrive(a, meta["nome_armazenado"].(string), ext)
		if errC != nil {
			jsonErro(w, http.StatusInternalServerError, "falha ao copiar o arquivo físico: "+errC.Error())
			return
		}
		res, errI := a.st.db.Exec(`
			INSERT INTO drive_arquivos (pasta_id, grupo_id, nome_original, nome_armazenado, tipo, tamanho, autor_usuario_id, autor_papel_id)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		`, nullInt64(novoPastaID), meta["grupo_id"], meta["nome_original"], novoNomeArm,
			meta["tipo"], nBytes, u.ID, u.PapelAtivoID)
		if errI != nil {
			_ = os.Remove(filepath.Join(a.pastaFisicaDrive(), novoNomeArm))
			jsonErro(w, http.StatusInternalServerError, "falha ao copiar arquivo: "+errI.Error())
			return
		}
		novoID, _ = res.LastInsertId()
	} else {
		res, errU := a.st.db.Exec(`UPDATE drive_arquivos SET pasta_id = ? WHERE id = ?`, nullInt64(novoPastaID), id)
		if errU != nil {
			jsonErro(w, http.StatusInternalServerError, "falha ao mover arquivo: "+errU.Error())
			return
		}
		if n, _ := res.RowsAffected(); n == 0 {
			jsonErro(w, http.StatusNotFound, "arquivo não encontrado")
			return
		}
		novoID = id
	}

	a.st.Auditoria(&u.ID, acao, "drive_arquivos", &novoID,
		fmt.Sprintf("%s → pasta %d", meta["nome_original"], novoPastaID), ipDe(r))

	jsonOK(w, map[string]any{
		"id":       novoID,
		"pasta_id": novoPastaID,
		"destino":  destinoDriveJSON(a, novoPastaID),
	})
}

// nullInt64: 0 vira NULL (raiz), senão o próprio valor.
func nullInt64(v int64) any {
	if v == 0 {
		return nil
	}
	return v
}

// GET /api/drive/arquivos/{id}/propriedades — modal de propriedades (item 3 da ordem):
// nome completo, enviado em (criado_em), autor (JOIN usuarios), tamanho e
// TABELA de acessos (drive_compartilhamentos com alvo resolvido + desde quando).
func (a *App) hDriveArquivoPropriedades(w http.ResponseWriter, r *http.Request) {
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

	// Leitura basta (visualizar propriedades), mesma regra do download.
	ok, _, errAcesso := a.checarAcessoArquivo(u, id, false)
	if errAcesso != nil || !ok {
		jsonErro(w, http.StatusForbidden, "sem permissão para ver as propriedades deste arquivo")
		return
	}

	var pastaID *int64
	var grupoID int64
	var nomeOriginal, tipo, criadoEm string
	var tamanho int64
	var autorNome string

	err = a.st.db.QueryRow(`
		SELECT da.pasta_id, da.grupo_id, da.nome_original, da.tipo, da.tamanho, da.criado_em,
		       COALESCE(NULLIF(u.nome_guerra, ''), u.login, '—')
		FROM drive_arquivos da
		LEFT JOIN usuarios u ON u.id = da.autor_usuario_id
		WHERE da.id = ?
	`, id).Scan(&pastaID, &grupoID, &nomeOriginal, &tipo, &tamanho, &criadoEm, &autorNome)
	if err != nil {
		jsonErro(w, http.StatusNotFound, "arquivo não encontrado")
		return
	}

	// Onda 05/10: o front exibe badge "da função" — devolve a função do item.
	var funcaoID *int64
	_ = a.st.db.QueryRow(`SELECT funcao_id FROM drive_arquivos WHERE id = ?`, id).Scan(&funcaoID)
	var funcaoNome string
	if funcaoID != nil {
		_ = a.st.db.QueryRow(`SELECT nome FROM funcoes WHERE id = ?`, *funcaoID).Scan(&funcaoNome)
	}

	pastaNome := ""
	if pastaID != nil && *pastaID > 0 {
		_ = a.st.db.QueryRow(`SELECT nome FROM drive_pastas WHERE id = ?`, *pastaID).Scan(&pastaNome)
	}

	// TABELA DE ACESSOS: cada grant ativo com alvo resolvido e desde quando.
	type Acesso struct {
		ID           int64  `json:"id"`
		AlvoTipo     string `json:"alvo_tipo"`
		AlvoNome     string `json:"alvo_nome"`
		PodeEditar   bool   `json:"pode_editar"`
		CriadoEm     string `json:"criado_em"`
		CriadoEmFmt  string `json:"criado_em_fmt"`
	}
	acessos := []Acesso{}
	rows, err := a.st.db.Query(`
		SELECT dc.id, dc.pode_editar, dc.criado_em,
		       CASE
		         WHEN dc.alvo_grupo_id IS NOT NULL THEN 'grupo'
		         WHEN dc.alvo_usuario_id IS NOT NULL THEN 'usuario'
		         WHEN dc.alvo_papel_id IS NOT NULL THEN 'papel'
		         ELSE 'outro'
		       END as alvo_tipo,
		       COALESCE(g.nome, NULLIF(u2.nome_guerra, ''), u2.login, up.papel, '—') as alvo_nome
		FROM drive_compartilhamentos dc
		LEFT JOIN grupos g ON g.id = dc.alvo_grupo_id
		LEFT JOIN usuarios u2 ON u2.id = dc.alvo_usuario_id
		LEFT JOIN usuario_papeis up ON up.id = dc.alvo_papel_id
		WHERE dc.arquivo_id = ?
		ORDER BY dc.criado_em ASC
	`, id)
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var ac Acesso
			var pEd int
			if errS := rows.Scan(&ac.ID, &pEd, &ac.CriadoEm, &ac.AlvoTipo, &ac.AlvoNome); errS == nil {
				ac.PodeEditar = pEd == 1
				ac.CriadoEmFmt = driveFmtDataHora(ac.CriadoEm)
				acessos = append(acessos, ac)
			}
		}
	}

	jsonOK(w, map[string]any{
		"id":             id,
		"nome_original":  nomeOriginal,
		"tipo":           tipo,
		"tamanho":        tamanho,
		"criado_em":      criadoEm,
		"criado_em_fmt":  driveFmtDataHora(criadoEm),
		"autor_nome":     autorNome,
		"pasta_id":       pastaID,
		"pasta_nome":     pastaNome,
		"grupo_id":       grupoID,
		"acessos":        acessos,
	})
}
