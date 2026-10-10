// server_admin.go — extraído de server.go (onda de modularização; recorte puro).

package main

import (
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

func (a *App) backupAgora() (arquivo, shaHex string, err error) {
	backupMu.Lock()
	defer backupMu.Unlock()
	dir := a.st.dataDir + string(os.PathSeparator) + "backups"
	if err = os.MkdirAll(dir, 0o750); err != nil {
		return
	}
	nome := "sci_" + time.Now().Format("20060102_150405") + ".db"
	alvo := filepath.Join(dir, nome)
	// colisão no mesmo segundo (ex.: backup de boot + manual): sufixo determinístico
	for i := 1; ; i++ {
		if _, e := os.Stat(alvo); os.IsNotExist(e) {
			break
		}
		nome = fmt.Sprintf("sci_%s_%02d.db", time.Now().Format("20060102_150405"), i)
		alvo = filepath.Join(dir, nome)
	}
	_, err = a.st.db.Exec(`VACUUM INTO ?`, alvo)
	if err != nil {
		return
	}
	// checkpoint: mantém o -wal enxuto (revisão TAKEDA §2/SHORYU §1.2)
	_, _ = a.st.db.Exec(`PRAGMA wal_checkpoint(TRUNCATE)`)
	fDB, err := os.Open(alvo)
	if err != nil {
		return
	}
	defer fDB.Close()

	h := sha256.New()
	tamanho, err := io.Copy(h, fDB)
	if err != nil {
		return
	}
	shaHex = hex.EncodeToString(h.Sum(nil))

	if err = os.WriteFile(alvo+".sha256", []byte(shaHex+"  "+nome+"\n"), 0o640); err != nil {
		return
	}
	fManifest, err2 := os.OpenFile(filepath.Join(dir, "MANIFEST.txt"), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o640)
	if err2 == nil {
		fmt.Fprintf(fManifest, "%s\t%s\t%s\t%d bytes\n", time.Now().UTC().Format(time.RFC3339), nome, shaHex, tamanho)
		fManifest.Close()
	}
	return nome, shaHex, nil
}

func (a *App) backupFlag(acao string, motivo any) {
	msg := fmt.Sprintf("%s — backup falhou em %s: %v\n", time.Now().UTC().Format(time.RFC3339), acao, motivo)
	_ = os.WriteFile(filepath.Join(a.st.dataDir, "FLAG_BACKUP.txt"), []byte(msg), 0o640)
}

func (a *App) backupAssincrono(acao string) {
	go func() {
		if _, _, err := a.backupAgora(); err != nil {
			a.backupFlag(acao, err)
		} else {
			_ = os.Remove(filepath.Join(a.st.dataDir, "FLAG_BACKUP.txt"))
		}
	}()
}

func (a *App) hHealth(w http.ResponseWriter, _ *http.Request) {
	jsonOK(w, map[string]any{"ok": true, "hora": time.Now().Format(time.RFC3339)})
}

func (a *App) hBackup(w http.ResponseWriter, r *http.Request) {
	nome, sha, err := a.backupAgora()
	if err != nil {
		a.backupFlag("manual:"+usuarioDoCtx(r).Login, err)
		jsonErro(w, http.StatusInternalServerError, "backup falhou: "+err.Error())
		return
	}
	_ = os.Remove(filepath.Join(a.st.dataDir, "FLAG_BACKUP.txt"))
	a.st.Auditoria(&usuarioDoCtx(r).ID, "backup", "banco", nil, nome, ipDe(r))
	jsonOK(w, map[string]string{"arquivo": "backups/" + nome, "sha256": sha})
}

func (a *App) hBackupDownload(w http.ResponseWriter, r *http.Request) {
	nome := r.URL.Query().Get("nome")
	if !reBackupNome.MatchString(nome) {
		jsonErro(w, http.StatusBadRequest, "nome de backup inválido")
		return
	}
	caminho := filepath.Join(a.st.dataDir, "backups", nome)
	if _, err := os.Stat(caminho); err != nil {
		jsonErro(w, http.StatusNotFound, "backup não encontrado")
		return
	}
	u := usuarioDoCtx(r)
	a.st.Auditoria(&u.ID, "baixar_backup", "banco", nil, nome, ipDe(r))
	w.Header().Set("Content-Disposition", "attachment; filename="+nome)
	http.ServeFile(w, r, caminho)
}

func (a *App) hAdminSistemaMetricas(w http.ResponseWriter, r *http.Request) {
	metricas := coletarMetricas(a.st.dataDir)
	jsonOK(w, metricas)
}

func (a *App) hAdminVinculoSet(w http.ResponseWriter, r *http.Request) {
	var req struct {
		SuperiorID    int64 `json:"superior_id"`
		SubordinadoID int64 `json:"subordinado_id"`
	}
	if err := decodificar(r, &req); err != nil || req.SuperiorID == 0 || req.SubordinadoID == 0 {
		jsonErro(w, http.StatusBadRequest, "superior_id e subordinado_id obrigatórios")
		return
	}
	if req.SuperiorID == req.SubordinadoID {
		jsonErro(w, http.StatusBadRequest, "um grupo não se subordina a si mesmo")
		return
	}
	var sup, sub, reverso int
	if a.st.db.QueryRow(`SELECT COUNT(*) FROM grupos WHERE id = ?`, req.SuperiorID).Scan(&sup) != nil || sup == 0 ||
		a.st.db.QueryRow(`SELECT COUNT(*) FROM grupos WHERE id = ?`, req.SubordinadoID).Scan(&sub) != nil || sub == 0 {
		jsonErro(w, http.StatusNotFound, "grupo inexistente")
		return
	}
	_ = a.st.db.QueryRow(`SELECT COUNT(*) FROM grupo_vinculos WHERE superior_id = ? AND subordinado_id = ?`,
		req.SubordinadoID, req.SuperiorID).Scan(&reverso)
	if reverso > 0 {
		jsonErro(w, http.StatusConflict, "vínculo reverso já existe — não é possível inverter")
		return
	}
	if _, err := a.st.db.Exec(`INSERT INTO grupo_vinculos (superior_id, subordinado_id, criado_por_superior, criado_por_subordinado)
		VALUES (?, ?, 1, 1) ON CONFLICT(superior_id, subordinado_id)
		DO UPDATE SET criado_por_superior = 1, criado_por_subordinado = 1`,
		req.SuperiorID, req.SubordinadoID); err != nil {
		jsonErro(w, http.StatusInternalServerError, err.Error())
		return
	}
	u := usuarioDoCtx(r)
	a.st.Auditoria(&u.ID, "vincular", "grupo_vinculos", nil,
		fmt.Sprintf("admin: sup=%d sub=%d", req.SuperiorID, req.SubordinadoID), ipDe(r))
	jsonOK(w, map[string]any{"ok": true})
}

func (a *App) hAdminVinculoRem(w http.ResponseWriter, r *http.Request) {
	var req struct {
		SuperiorID    int64 `json:"superior_id"`
		SubordinadoID int64 `json:"subordinado_id"`
	}
	_ = decodificar(r, &req)
	// aceita também query string (DELETE da UI)
	if req.SuperiorID == 0 {
		req.SuperiorID, _ = strconv.ParseInt(r.URL.Query().Get("superior_id"), 10, 64)
	}
	if req.SubordinadoID == 0 {
		req.SubordinadoID, _ = strconv.ParseInt(r.URL.Query().Get("subordinado_id"), 10, 64)
	}
	if req.SuperiorID == 0 || req.SubordinadoID == 0 {
		jsonErro(w, http.StatusBadRequest, "superior_id e subordinado_id obrigatórios")
		return
	}
	res, err := a.st.db.Exec(`DELETE FROM grupo_vinculos WHERE superior_id = ? AND subordinado_id = ?`,
		req.SuperiorID, req.SubordinadoID)
	if err != nil {
		jsonErro(w, http.StatusInternalServerError, err.Error())
		return
	}
	if n, _ := res.RowsAffected(); n == 0 {
		jsonErro(w, http.StatusNotFound, "vínculo inexistente")
		return
	}
	u := usuarioDoCtx(r)
	a.st.Auditoria(&u.ID, "desvincular", "grupo_vinculos", nil,
		fmt.Sprintf("admin: sup=%d sub=%d", req.SuperiorID, req.SubordinadoID), ipDe(r))
	jsonOK(w, map[string]bool{"ok": true})
}

func (a *App) hBackupImportar(w http.ResponseWriter, r *http.Request) {
	// mapear usuário → sessão depois do swap morre? Não: sessões vivem NO banco,
	// restauradas junto com o arquivo importado.
	if err := r.ParseMultipartForm(64 << 20); err != nil {
		jsonErro(w, http.StatusBadRequest, "upload inválido (multipart/form-data, campo 'arquivo')")
		return
	}
	f, _, err := r.FormFile("arquivo")
	if err != nil {
		jsonErro(w, http.StatusBadRequest, "campo 'arquivo' ausente")
		return
	}
	defer f.Close()
	tmp, err := os.CreateTemp(a.st.dataDir, "import_*.db")
	if err != nil {
		jsonErro(w, http.StatusInternalServerError, err.Error())
		return
	}
	tmpNome := tmp.Name()
	defer os.Remove(tmpNome)
	if _, err = io.Copy(tmp, f); err != nil {
		tmp.Close()
		jsonErro(w, http.StatusInternalServerError, "falha ao gravar upload: "+err.Error())
		return
	}
	tmp.Close()
	// 1) magic SQLite: "SQLite format 3\x00"
	cab := make([]byte, 16)
	fh, err := os.Open(tmpNome)
	if err != nil {
		jsonErro(w, http.StatusInternalServerError, err.Error())
		return
	}
	_, err = io.ReadFull(fh, cab)
	fh.Close()
	if err != nil || string(cab) != "SQLite format 3\x00" {
		jsonErro(w, http.StatusBadRequest, "arquivo não é um banco SQLite — importação rejeitada")
		return
	}
	// 2) abre o upload em cópia privativa e roda integrity_check + checagem de versão
	checagem, err := sql.Open("sqlite", "file:"+tmpNome+"?_pragma=foreign_keys(0)&_txlock=immediate")
	if err != nil {
		jsonErro(w, http.StatusBadRequest, "banco ilegível: "+err.Error())
		return
	}
	defer checagem.Close()
	var integridade string
	if err = checagem.QueryRow(`PRAGMA integrity_check`).Scan(&integridade); err != nil || integridade != "ok" {
		jsonErro(w, http.StatusBadRequest, "integrity_check falhou ("+integridade+") — importação rejeitada")
		return
	}
	var versaoArq int
	if err = checagem.QueryRow(`SELECT COALESCE(MAX(versao),0) FROM schema_migrations`).Scan(&versaoArq); err != nil {
		jsonErro(w, http.StatusBadRequest, "arquivo sem schema_migrations — não é um banco do SCI")
		return
	}
	versaoBin := versaoSchemaBinario
	if versaoArq > versaoBin {
		jsonErro(w, http.StatusBadRequest,
			fmt.Sprintf("backup mais novo que o sistema (schema %d > %d) — importação rejeitada", versaoArq, versaoBin))
		return
	}
	checagem.Close()
	// 3) backup de segurança do estado ATUAL
	nomeSeg, _, err := a.backupAgora()
	if err != nil {
		jsonErro(w, http.StatusInternalServerError, "backup de segurança falhou — nada alterado: "+err.Error())
		return
	}
	segPath := filepath.Join(a.st.dataDir, "backups", nomeSeg)

	// 4) swap atômico para sci.db: fecha pool → backup temporário de sci.db → rename direto para sci.db → limpa wal/shm → reabre+migra
	canonicalDB := filepath.Join(a.st.dataDir, "sci.db")
	bakPath := canonicalDB + ".bak"

	// Fecha o pool atual para liberar locks de arquivo (crucial no Windows)
	if err = a.st.Close(); err != nil {
		jsonErro(w, http.StatusInternalServerError, "falha ao fechar banco atual: "+err.Error())
		return
	}

	_ = os.Remove(bakPath)
	if err = os.Rename(canonicalDB, bakPath); err != nil {
		// Se rename falhar, tenta copiar e remover
		if err = copiarArquivoDisco(canonicalDB, bakPath); err == nil {
			_ = os.Remove(canonicalDB)
		}
	}

	_ = os.Remove(canonicalDB)
	if err = os.Rename(tmpNome, canonicalDB); err != nil {
		// Falha ao mover o novo banco para sci.db -> rollback imediato
		if _, e := os.Stat(bakPath); e == nil {
			_ = os.Rename(bakPath, canonicalDB)
		} else {
			_ = copiarArquivoDisco(segPath, canonicalDB)
		}
		_ = a.st.ReabrirComArquivo(canonicalDB)
		jsonErro(w, http.StatusInternalServerError, "preparar swap falhou — nada alterado: "+err.Error())
		return
	}

	// Limpa WAL e SHM órfãos
	_ = os.Remove(canonicalDB + "-wal")
	_ = os.Remove(canonicalDB + "-shm")
	_ = os.Remove(canonicalDB + ".novo")
	_ = os.Remove(bakPath + "-wal")
	_ = os.Remove(bakPath + "-shm")

	// Reabre e executa a cadeia de migrações v2..v43 unificada
	if err = a.st.ReabrirComArquivo(canonicalDB); err != nil {
		// Rollback seguro em caso de falha de integridade ou migração
		if a.st.db != nil {
			_ = a.st.db.Close()
		}
		_ = os.Remove(canonicalDB)
		_ = os.Remove(canonicalDB + "-wal")
		_ = os.Remove(canonicalDB + "-shm")

		if _, e := os.Stat(bakPath); e == nil {
			_ = os.Rename(bakPath, canonicalDB)
		} else {
			_ = copiarArquivoDisco(segPath, canonicalDB)
		}
		_ = os.Remove(canonicalDB + "-wal")
		_ = os.Remove(canonicalDB + "-shm")
		_ = a.st.ReabrirComArquivo(canonicalDB)

		jsonErro(w, http.StatusInternalServerError, "swap falhou na migração — banco restaurado ao estado anterior: "+err.Error())
		return
	}

	// Sucesso: remove o .bak temporário
	_ = os.Remove(bakPath)

	var schemaFinal int
	_ = a.st.db.QueryRow(`SELECT COALESCE(MAX(versao),0) FROM schema_migrations`).Scan(&schemaFinal)

	u := usuarioDoCtx(r)
	a.st.Auditoria(&u.ID, "importar_backup", "banco", nil,
		fmt.Sprintf("schema=%d seguranca=%s", schemaFinal, nomeSeg), ipDe(r))
	jsonOK(w, map[string]any{"ok": true, "schema": schemaFinal, "seguranca": "backups/" + nomeSeg})
}

func copiarArquivoDisco(origem, destino string) error {
	in, err := os.Open(origem)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.Create(destino)
	if err != nil {
		return err
	}
	defer out.Close()
	if _, err = io.Copy(out, in); err != nil {
		return err
	}
	return out.Sync()
}

func (a *App) hConfiguracoesGet(w http.ResponseWriter, _ *http.Request) {
	// FIX P0-2 (revisão DEV-L 30/09): a rota é PÚBLICA (o login precisa dos rótulos e
	// do tema antes da sessão) — só as chaves de aparência/termos saem. Chaves internas
	// (MODO_RESERVA e futuras flags) NUNCA vazem por aqui.
	publicas := map[string]bool{
		"NOME_SISTEMA": true, "SUBTITULO_SISTEMA": true, "TITULO_ORGANIZACAO": true,
		"ROTULO_GRUPO": true, "ROTULO_SETOR": true, "ROTULO_FUNCAO": true,
		"ROTULO_PESSOA": true, "ROTULO_IDENTIFICADOR": true,
		"COR_PRIMARIA": true, "COR_PRIMARIA_CLARO": true, "COR_PRIMARIA_ESCURO": true,
		"CAUTELA_PRAZO_PADRAO_HORAS": true, "WEBHOOK_ATRASOS_URL": true,
	}
	rows, err := a.st.db.Query(`SELECT chave, valor FROM configuracoes`)
	if err != nil {
		jsonErro(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer rows.Close()
	configs := map[string]string{}
	for rows.Next() {
		var k, v string
		if rows.Scan(&k, &v) == nil && publicas[k] {
			configs[k] = v
		}
	}
	jsonOK(w, map[string]any{"configuracoes": configs})
}

func (a *App) hConfiguracoesSet(w http.ResponseWriter, r *http.Request) {
	u := usuarioDoCtx(r)
	if u.Papel != "admin" {
		jsonErro(w, http.StatusForbidden, "Apenas o Administrador pode alterar configurações globais")
		return
	}
	var req map[string]string
	if err := decodificar(r, &req); err != nil {
		jsonErro(w, http.StatusBadRequest, "Corpo inválido")
		return
	}
	tx, err := a.st.db.Begin()
	if err != nil {
		jsonErro(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer tx.Rollback()

	for k, v := range req {
		k = strings.TrimSpace(k)
		if k == "" {
			continue
		}
		_, err = tx.Exec(`
			INSERT INTO configuracoes (chave, valor, atualizado_em)
			VALUES (?, ?, strftime('%Y-%m-%dT%H:%M:%fZ','now'))
			ON CONFLICT(chave) DO UPDATE SET valor = excluded.valor, atualizado_em = excluded.atualizado_em`,
			k, v)
		if err != nil {
			jsonErro(w, http.StatusInternalServerError, err.Error())
			return
		}
	}
	if err := tx.Commit(); err != nil {
		jsonErro(w, http.StatusInternalServerError, err.Error())
		return
	}
	a.st.Auditoria(&u.ID, "editar_configuracoes", "configuracoes", nil, fmt.Sprintf("chaves=%d", len(req)), ipDe(r))
	jsonOK(w, map[string]any{"ok": true})
}

func (a *App) hNotificacoesHub(w http.ResponseWriter, r *http.Request) {
	u := usuarioDoCtx(r)
	escopo, err := a.exigeEscopo(u)
	if err != nil {
		jsonErro(w, http.StatusForbidden, "conta sem grupo definido")
		return
	}

	prazoHoras := 24
	var cfgPrazo string
	_ = a.st.db.QueryRow(`SELECT valor FROM configuracoes WHERE chave = 'CAUTELA_PRAZO_PADRAO_HORAS'`).Scan(&cfgPrazo)
	if p, err := strconv.Atoi(cfgPrazo); err == nil && p > 0 {
		prazoHoras = p
	}

	q := `SELECT mc.id, mc.item_id, mi.nome, mi.codigo_patrimonio, mc.pessoa_id, p.nome_guerra,
	             mc.data_saida, COALESCE(mi.nivel_sensibilidade, 'padrao'),
	             ROUND((strftime('%s', 'now') - strftime('%s', mc.data_saida)) / 3600.0, 1) as horas_fora
	      FROM material_cautelas mc
	      JOIN material_itens mi ON mi.id = mc.item_id
	      JOIN pessoas p ON p.id = mc.pessoa_id
	      WHERE mc.status = 'ativa'
	        AND (strftime('%s', 'now') - strftime('%s', mc.data_saida)) > (? * 3600)`
	args := []any{prazoHoras}
	if escopo > 0 {
		q += ` AND mi.grupo_id = ?`
		args = append(args, escopo)
	}
	q += ` ORDER BY mc.data_saida ASC LIMIT 100`

	rows, err := a.st.db.Query(q, args...)
	var atrasadas []map[string]any
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var cid, iid, pid int64
			var iNome, iCod, pGuerra, dts, sens string
			var horas float64
			if rows.Scan(&cid, &iid, &iNome, &iCod, &pid, &pGuerra, &dts, &sens, &horas) == nil {
				atrasadas = append(atrasadas, map[string]any{
					"cautela_id":          cid,
					"item_id":             iid,
					"item_nome":           iNome,
					"codigo_patrimonio":   iCod,
					"pessoa_id":           pid,
					"pessoa_nome_guerra":  pGuerra,
					"data_saida":          dts,
					"nivel_sensibilidade": sens,
					"horas_em_uso":        horas,
				})
			}
		}
	}

	// Despachos e Avisos Pendentes (v1.2 Fase 2)
	despachosPendentes := 0
	if u.PapelAtivoID != nil && *u.PapelAtivoID > 0 {
		_ = a.st.db.QueryRow(`
			SELECT COUNT(*)
			FROM mensagem_destinatarios md
			JOIN mensagens m ON m.id = md.mensagem_id
			WHERE md.destinatario_papel_id = ? AND m.exige_resposta = 1 AND md.respondido_em IS NULL AND md.excluida = 0`,
			*u.PapelAtivoID).Scan(&despachosPendentes)
	}

	avisosPendentes := 0
	if escopo > 0 {
		_ = a.st.db.QueryRow(`
			SELECT COUNT(*)
			FROM avisos a
			WHERE a.grupo_id = ?
			  AND NOT EXISTS (SELECT 1 FROM aviso_cientes ac WHERE ac.aviso_id = a.id AND ac.usuario_id = ?)`,
			escopo, u.ID).Scan(&avisosPendentes)
	}

	jsonOK(w, map[string]any{
		"prazo_horas":         prazoHoras,
		"total_atrasadas":     len(atrasadas),
		"cautelas_atrasadas":  atrasadas,
		"despachos_pendentes": despachosPendentes,
		"avisos_pendentes":    avisosPendentes,
		"total_geral":         len(atrasadas) + despachosPendentes + avisosPendentes,
	})
}

// ---------- rotas rotasAdmin ----------
func (a *App) rotasAdmin() {
	m := a.mux
	m.HandleFunc("GET /api/health", a.hHealth)

	// Fórum & Mural de Avisos Gerenciais (v1.2 Fase 2)
	m.Handle("GET /api/avisos", a.auth(false, a.hAvisosList))
	m.Handle("POST /api/avisos", a.auth(false, a.hAvisosAdd))
	m.Handle("DELETE /api/avisos/{id}", a.auth(false, a.hAvisosDel))
	m.Handle("POST /api/avisos/{id}/ciente", a.auth(false, a.hAvisosCiente))
	m.Handle("POST /api/avisos/{id}/comentar", a.auth(false, a.hAvisosComentar))
	m.Handle("GET /api/avisos/{id}/detalhes", a.auth(false, a.hAvisosDetalhes))
	m.Handle("POST /api/avisos/{id}/repostar", a.auth(false, a.hAvisosRepostar))
	m.Handle("GET /api/notificacoes", a.auth(false, a.hNotificacoesHub))

	m.Handle("POST /api/backup", a.auth(true, a.hBackup))
	m.Handle("POST /api/backup/importar", a.auth(true, a.hBackupImportar)) // R9
	m.Handle("GET /api/backup/download", a.auth(true, a.hBackupDownload))
	m.Handle("GET /api/admin/sistema/metricas", a.auth(true, a.hAdminSistemaMetricas))

	// Módulo de Configurações e White-Label (v1.0)
	m.HandleFunc("GET /api/configuracoes", a.hConfiguracoesGet)
	m.Handle("POST /api/configuracoes", a.auth(true, a.hConfiguracoesSet))
}
