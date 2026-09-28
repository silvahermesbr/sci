package main

// Servidor HTTP do SCI: rotas da API, backup definitivo, front embutido (SPA).

import (
	"crypto/sha256"
	"database/sql"
	"embed"
	"encoding/csv"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
)

//go:embed web
var webEmbed embed.FS

type App struct {
	st        *Store
	lim       *Limiter
	mux       *http.ServeMux
	horaLocal *time.Location
	omTitulo  string
}

func NovaApp(st *Store) *App {
	loc, err := time.LoadLocation("America/Sao_Paulo")
	if err != nil {
		loc = time.UTC
	}
	titulo := os.Getenv("SCI_OM_TITULO")
	if titulo == "" {
		titulo = "SCI — RELATÓRIO DE PRESENÇA"
	}
	a := &App{st: st, lim: NovoLimiter(), mux: http.NewServeMux(), horaLocal: loc, omTitulo: titulo}
	a.rotas()
	return a
}

func env(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

// ---------- helpers JSON ----------

func jsonOK(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_ = json.NewEncoder(w).Encode(v)
}

func jsonErro(w http.ResponseWriter, status int, msg string) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"erro": msg})
}

func decodificar(r *http.Request, v any) error {
	defer r.Body.Close()
	body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	if err != nil {
		return err
	}
	return json.Unmarshal(body, v)
}

// ---------- backup definitivo ----------

// backupAgora cria backups/sci_YYYYMMDD_HHMMSS.db consistente (VACUUM INTO),
// com sha256 + linha no MANIFEST.txt. Todos os dados vivem em arquivo.
func (a *App) backupAgora() (arquivo, shaHex string, err error) {
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
	raw, err := os.ReadFile(alvo)
	if err != nil {
		return
	}
	h := sha256.Sum256(raw)
	shaHex = hex.EncodeToString(h[:])
	if err = os.WriteFile(alvo+".sha256", []byte(shaHex+"  "+nome+"\n"), 0o640); err != nil {
		return
	}
	f, err2 := os.OpenFile(filepath.Join(dir, "MANIFEST.txt"), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o640)
	if err2 == nil {
		fmt.Fprintf(f, "%s\t%s\t%s\t%d bytes\n", time.Now().UTC().Format(time.RFC3339), nome, shaHex, len(raw))
		f.Close()
	}
	return nome, shaHex, nil
}

// backupFlag registra falha de backup em FLAG_BACKUP.txt (visível no watchdog).
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

// ---------- migracao v2 (senhas opcionais por usuário) ----------

func (s *Store) migrarV2() error {
	rows, err := s.db.Query(`PRAGMA table_info(usuarios)`)
	if err != nil {
		return err
	}
	defer rows.Close()
	tem := false
	for rows.Next() {
		var cid int
		var nome, tipo string
		var notNull int
		var dflt any
		var pk int
		if err := rows.Scan(&cid, &nome, &tipo, &notNull, &dflt, &pk); err != nil {
			return err
		}
		if nome == "senhas" {
			tem = true
		}
	}
	rows.Close()
	if tem {
		return nil
	}
	_, err = s.db.Exec(`ALTER TABLE usuarios ADD COLUMN senhas TEXT NOT NULL DEFAULT '[]'`)
	return err
}

// ---------- rotas ----------

func (a *App) rotas() {
	m := a.mux
	m.HandleFunc("GET /api/health", a.hHealth)
	m.HandleFunc("POST /api/login", a.hLogin)
	m.Handle("GET /api/me", a.auth(false, a.hMe))
	m.Handle("POST /api/logout", a.auth(false, a.hLogout))
	m.Handle("POST /api/senha", a.auth(false, a.hTrocarSenha))

	m.Handle("GET /api/conferencia/hoje", a.auth(false, a.hConferenciaHoje))
	m.Handle("POST /api/conferencia/iniciar", a.auth(false, a.hConferenciaIniciar))
	m.Handle("POST /api/conferencia/fechar", a.auth(false, a.hConferenciaFechar))
	m.Handle("GET /api/conferencia/lista", a.auth(false, a.hConferenciaList))
	m.Handle("GET /api/conferencia/{id}", a.auth(false, a.hConferenciaGet))
	m.Handle("GET /api/conferencia/{id}/relatorio.pdf", a.auth(false, a.hConferenciaPDF))

	m.Handle("GET /api/presenca/periodo", a.auth(false, a.hPresencaPeriodo))
	m.Handle("GET /api/conferencias", a.auth(false, a.hConferenciaList))

	m.Handle("GET /api/catalogo/{t}", a.auth(false, a.hCatalogoList))
	m.Handle("POST /api/catalogo/{t}", a.auth(true, a.hCatalogoAdd))
	m.Handle("DELETE /api/catalogo/{t}/{id}", a.auth(true, a.hCatalogoDel))

	m.Handle("GET /api/pessoas", a.auth(false, a.hPessoasList))
	m.Handle("POST /api/pessoas", a.auth(true, a.hPessoasAdd))
	m.Handle("PATCH /api/pessoas/{id}", a.auth(true, a.hPessoasEdit))

	m.Handle("GET /api/usuarios", a.auth(true, a.hUsuariosList))
	// criação é validada DENTRO do handler (admin cria qualquer; gerente cria operador do próprio grupo)
	m.Handle("POST /api/usuarios", a.auth(false, a.hUsuariosAdd))
	m.Handle("POST /api/usuarios/{id}/senha", a.auth(true, a.hUsuarioSenha))
	m.Handle("GET /api/grupos", a.auth(false, a.hGruposList))
	m.Handle("POST /api/grupos", a.auth(true, a.hGruposAdd))
	m.Handle("POST /api/comentarios", a.auth(false, a.hComentariosAdd))
	m.Handle("GET /api/comentarios/{id}", a.auth(false, a.hComentariosList))

	m.Handle("GET /api/relatorio", a.auth(false, a.hRelatorioJSON))
	m.Handle("GET /api/relatorio.pdf", a.auth(false, a.hRelatorioPDF))
	m.Handle("GET /api/export/{t}", a.auth(false, a.hExportCSV))

	m.Handle("POST /api/backup", a.auth(true, a.hBackup))
	m.Handle("GET /api/backup/download", a.auth(true, a.hBackupDownload))

	m.Handle("GET /", http.HandlerFunc(a.hSPA))
}

// ---------- handlers básicos ----------

func (a *App) hHealth(w http.ResponseWriter, _ *http.Request) {
	jsonOK(w, map[string]any{"ok": true, "hora": time.Now().Format(time.RFC3339)})
}

func (a *App) hLogin(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Login string `json:"login"`
		Senha string `json:"senha"`
	}
	if err := decodificar(r, &req); err != nil || req.Login == "" || req.Senha == "" {
		jsonErro(w, http.StatusBadRequest, "login e senha obrigatórios")
		return
	}
	u, err := a.validarCredenciais(req.Login, req.Senha, ipDe(r))
	if err != nil {
		jsonErro(w, http.StatusUnauthorized, err.Error())
		return
	}
	tok, expira, err := a.st.CriarSessao(u.ID, ttlSessao)
	if err != nil {
		jsonErro(w, http.StatusInternalServerError, "falha ao criar sessão")
		return
	}
	http.SetCookie(w, &http.Cookie{
		Name: cookieSessao, Value: tok, Path: "/", HttpOnly: true,
		SameSite: http.SameSiteLaxMode, MaxAge: int(ttlSessao.Seconds()),
	})
	jsonOK(w, map[string]any{"usuario": u, "expira": expira})
}

func (a *App) hMe(w http.ResponseWriter, r *http.Request) {
	jsonOK(w, map[string]any{"usuario": usuarioDoCtx(r)})
}

func (a *App) hLogout(w http.ResponseWriter, r *http.Request) {
	if c, err := r.Cookie(cookieSessao); err == nil {
		a.st.EncerrarSessao(c.Value)
	}
	http.SetCookie(w, &http.Cookie{Name: cookieSessao, Value: "", Path: "/", MaxAge: -1})
	jsonOK(w, map[string]bool{"ok": true})
}

// hTrocarSenha: cada conta troca a PRÓPRIA senha (exige a atual).
func (a *App) hTrocarSenha(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Atual string `json:"atual"`
		Nova  string `json:"nova"`
	}
	if err := decodificar(r, &req); err != nil || req.Atual == "" || req.Nova == "" {
		jsonErro(w, http.StatusBadRequest, "atual e nova obrigatórias")
		return
	}
	if len(req.Nova) < 8 {
		jsonErro(w, http.StatusBadRequest, "nova senha: mínimo 8 caracteres")
		return
	}
	u := usuarioDoCtx(r)
	var hash string
	if err := a.st.db.QueryRow(`SELECT senha_hash FROM usuarios WHERE id = ?`, u.ID).Scan(&hash); err != nil {
		jsonErro(w, http.StatusInternalServerError, err.Error())
		return
	}
	if !verificaSenha(req.Atual, hash) {
		jsonErro(w, http.StatusUnauthorized, "senha atual incorreta")
		return
	}
	novo, err := hashSenha(req.Nova)
	if err != nil {
		jsonErro(w, http.StatusInternalServerError, err.Error())
		return
	}
	if _, err := a.st.db.Exec(`UPDATE usuarios SET senha_hash = ? WHERE id = ?`, novo, u.ID); err != nil {
		jsonErro(w, http.StatusInternalServerError, err.Error())
		return
	}
	a.st.Auditoria(&u.ID, "trocar_senha", "usuarios", &u.ID, "", ipDe(r))
	jsonOK(w, map[string]bool{"ok": true})
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

// hBackupDownload: entrega o arquivo .db para download no navegador (ordem Tenente 28/09).
var reBackupNome = regexp.MustCompile(`^sci_[0-9]{8}_[0-9]{6}(_[0-9]{2})?\.db$`)

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

// hUsuarioSenha: admin redefine a senha de qualquer conta (ordem Tenente 28/09).
func (a *App) hUsuarioSenha(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		jsonErro(w, http.StatusBadRequest, "id inválido")
		return
	}
	var req struct {
		Senha string `json:"senha"`
	}
	if err = decodificar(r, &req); err != nil || len(req.Senha) < 8 {
		jsonErro(w, http.StatusBadRequest, "senha: mínimo 8 caracteres")
		return
	}
	hash, err := hashSenha(req.Senha)
	if err != nil {
		jsonErro(w, http.StatusInternalServerError, err.Error())
		return
	}
	res, err := a.st.db.Exec(`UPDATE usuarios SET senha_hash = ? WHERE id = ?`, hash, id)
	if err != nil {
		jsonErro(w, http.StatusInternalServerError, err.Error())
		return
	}
	if n, _ := res.RowsAffected(); n == 0 {
		jsonErro(w, http.StatusNotFound, "usuário inexistente")
		return
	}
	u := usuarioDoCtx(r)
	a.st.Auditoria(&u.ID, "redefinir_senha", "usuarios", &id, "", ipDe(r))
	jsonOK(w, map[string]bool{"ok": true})
}

// ---------- conferências (ciclo completo: iniciar → gravar → fechar → lista) ----------

const tipoConferenciaPadrao = "Conferência de pessoal"

func (a *App) tipoPadraoID() (int64, string, error) {
	var id int64
	var nome string
	err := a.st.db.QueryRow(
		`SELECT id, nome FROM conferencia_tipos WHERE nome = ? AND ativo = 1`, tipoConferenciaPadrao).
		Scan(&id, &nome)
	return id, nome, err
}

// hConferenciaHoje: contexto para a área de conferência — a conferência ABERTA (se houver)
// + o efetivo ativo + os lançamentos dela.
func (a *App) hConferenciaHoje(w http.ResponseWriter, r *http.Request) {
	u := usuarioDoCtx(r)
	escopo := escopoDoUsuario(u)
	var f struct {
		ID       int64
		Status   string
		Data     string
		CriadaEm string
	}
	var err error
	if escopo > 0 {
		err = a.st.db.QueryRow(
			`SELECT id, status, data, criado_em FROM conferencias
			 WHERE status = 'aberta' AND grupo_id = ? ORDER BY id DESC LIMIT 1`, escopo).
			Scan(&f.ID, &f.Status, &f.Data, &f.CriadaEm)
	} else {
		err = a.st.db.QueryRow(
			`SELECT id, status, data, criado_em FROM conferencias
			 WHERE status = 'aberta' ORDER BY id DESC LIMIT 1`).
			Scan(&f.ID, &f.Status, &f.Data, &f.CriadaEm)
	}
	var form *map[string]any
	if err == nil {
		estados := map[int64]map[string]any{}
		rows, e := a.st.db.Query(
			`SELECT pessoa_id, situacao, destino_id, COALESCE(observacao,'')
			 FROM presencas WHERE conferencia_id = ?`, f.ID)
		if e == nil {
			for rows.Next() {
				var pid int64
				var sit string
				var did *int64
				var obs string
				if rows.Scan(&pid, &sit, &did, &obs) == nil {
					estados[pid] = map[string]any{"situacao": sit, "destino_id": did, "observacao": obs}
				}
			}
			rows.Close()
		}
		form = &map[string]any{"id": f.ID, "status": f.Status, "data": f.Data,
			"criada_em": f.CriadaEm, "estados": estados}
	}
	jsonOK(w, map[string]any{"conferencia": form, "pessoas": a.pessoasAtivas()})
}

type lancamentoReq struct {
	PessoaID   int64   `json:"pessoa_id"`
	Situacao   string  `json:"situacao"`
	DestinoID  *int64  `json:"destino_id"`
	TagID      *int64  `json:"tag_id"`
	Observacao string  `json:"observacao"`
}

func (a *App) pessoasAtivas() []map[string]any {
	rows, err := a.st.db.Query(`
		SELECT p.id, p.nome_guerra, p.nome_completo, COALESCE(s.nome,''), COALESCE(fu.nome,'')
		FROM pessoas p
		LEFT JOIN setores s ON s.id = p.setor_id
		LEFT JOIN funcoes fu ON fu.id = p.funcao_id
		WHERE p.status = 'ativo'
		ORDER BY COALESCE(s.nome,''), p.nome_guerra`)
	if err != nil {
		return []map[string]any{}
	}
	defer rows.Close()
	out := []map[string]any{}
	for rows.Next() {
		var id int64
		var ng, nc, setor, funcao string
		if rows.Scan(&id, &ng, &nc, &setor, &funcao) == nil {
			out = append(out, map[string]any{
				"id": id, "nome_guerra": ng, "nome_completo": nc, "setor": setor, "funcao": funcao,
			})
		}
	}
	return out
}

// hConferenciaIniciar: cria a conferência (data = hoje ou a informada) e a deixa ABERTA.
func (a *App) hConferenciaIniciar(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Local string `json:"local"`
		Data  string `json:"data"` // data de início (opcional; default hoje)
	}
	_ = decodificar(r, &req)
	hoje := time.Now().In(a.horaLocal).Format("2006-01-02")
	data := req.Data
	if data == "" {
		data = hoje
	}
	if len(data) != 10 || data[4] != '-' || data[7] != '-' {
		jsonErro(w, http.StatusBadRequest, "data inválida (AAAA-MM-DD)")
		return
	}
	tipoID, _, err := a.tipoPadraoID()
	if err != nil {
		jsonErro(w, http.StatusInternalServerError, "tipo '"+tipoConferenciaPadrao+"' inexistente")
		return
	}
	u := usuarioDoCtx(r)
	escopo := escopoDoUsuario(u)
	// já existe conferência aberta NO ESCOPO? (admin: global; grupo: do grupo)
	var abertaID int64
	qAberta := `SELECT id FROM conferencias WHERE status = 'aberta'`
	if escopo > 0 {
		qAberta += ` AND grupo_id = ?`
	}
	qAberta += ` LIMIT 1`
	if escopo > 0 {
		if e := a.st.db.QueryRow(qAberta, escopo).Scan(&abertaID); e == nil {
			jsonErro(w, http.StatusConflict, "já existe uma conferência aberta do seu grupo (feche-a antes de iniciar outra)")
			return
		}
	} else if e := a.st.db.QueryRow(qAberta).Scan(&abertaID); e == nil {
		jsonErro(w, http.StatusConflict, "já existe uma conferência aberta (feche-a antes de iniciar outra)")
		return
	}
	// várias por dia: permitido (ordem Tenente); UNIQUE(data,tipo) foi removida na v4
	var id int64
	grupoID := escopo // operador/gerente → próprio grupo; admin → NULL (global)
	var res sql.Result
	var e error
	if escopo > 0 {
		res, e = a.st.db.Exec(
			`INSERT INTO conferencias (data, tipo_id, local, grupo_id, criado_por) VALUES (?,?,?,?,?)`,
			data, tipoID, req.Local, grupoID, u.ID)
	} else {
		res, e = a.st.db.Exec(
			`INSERT INTO conferencias (data, tipo_id, local, criado_por) VALUES (?,?,?,?)`,
			data, tipoID, req.Local, u.ID)
	}
	if e != nil {
		jsonErro(w, http.StatusInternalServerError, e.Error())
		return
	}
	id, _ = res.LastInsertId()
	a.st.Auditoria(&u.ID, "iniciar", "conferencias", &id, "data="+data, ipDe(r))
	jsonOK(w, map[string]any{"id": id, "data": data})
}

// hConferenciaFechar: grava os lançamentos e fecha (fechada_em = agora).
func (a *App) hConferenciaFechar(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ID          int64           `json:"id"`
		Lancamentos []lancamentoReq `json:"lancamentos"`
	}
	if err := decodificar(r, &req); err != nil {
		jsonErro(w, http.StatusBadRequest, "JSON inválido")
		return
	}
	if req.ID == 0 || len(req.Lancamentos) == 0 {
		jsonErro(w, http.StatusBadRequest, "id e lançamentos obrigatórios")
		return
	}
	for _, l := range req.Lancamentos {
		switch l.Situacao {
		case "presente", "atraso", "falta", "justificada":
		default:
			jsonErro(w, http.StatusBadRequest, "situação inválida: "+l.Situacao)
			return
		}
		if l.Situacao == "justificada" && l.DestinoID == nil {
			jsonErro(w, http.StatusBadRequest, "justificada exige destino")
			return
		}
	}
	u := usuarioDoCtx(r)

	tx, err := a.st.db.Begin()
	if err != nil {
		jsonErro(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer tx.Rollback()
	// a conferência tem que estar ABERTA
	var status string
	if err = tx.QueryRow(`SELECT status FROM conferencias WHERE id = ?`, req.ID).Scan(&status); err != nil {
		jsonErro(w, http.StatusNotFound, "conferência inexistente")
		return
	}
	if status != "aberta" {
		jsonErro(w, http.StatusConflict, "conferência já está fechada")
		return
	}
	gravados := 0
	for _, l := range req.Lancamentos {
		_, err = tx.Exec(`
			INSERT INTO presencas (conferencia_id, pessoa_id, situacao, destino_id, tag_id, observacao, marcado_por)
			VALUES (?,?,?,?,?,?,?)
			ON CONFLICT (conferencia_id, pessoa_id) DO UPDATE SET
			  situacao = excluded.situacao,
			  destino_id = excluded.destino_id,
			  tag_id = excluded.tag_id,
			  observacao = excluded.observacao,
			  marcado_por = excluded.marcado_por,
			  alterado_por = excluded.marcado_por,
			  alterado_em = strftime('%Y-%m-%dT%H:%M:%fZ','now')`,
			req.ID, l.PessoaID, l.Situacao, l.DestinoID, l.TagID, l.Observacao, u.ID)
		if err != nil {
			jsonErro(w, http.StatusInternalServerError, "lançamento falhou (nada gravado): "+err.Error())
			return
		}
		gravados++
	}
	if _, err = tx.Exec(
		`UPDATE conferencias SET status = 'fechada', fechada_em = ? WHERE id = ?`,
		time.Now().UTC().Format(time.RFC3339), req.ID); err != nil {
		jsonErro(w, http.StatusInternalServerError, err.Error())
		return
	}
	if err = tx.Commit(); err != nil {
		jsonErro(w, http.StatusInternalServerError, err.Error())
		return
	}
	a.st.Auditoria(&u.ID, "fechar", "conferencias", &req.ID,
		fmt.Sprintf("lancamentos=%d", gravados), ipDe(r))
	a.backupAssincrono("fechar") // zero-perda: cópia consistente a cada fechamento
	jsonOK(w, map[string]any{"conferencia_id": req.ID, "gravados": gravados})
}

// hConferenciaList: todas as conferências, mais recentes primeiro, com status e horários.
func (a *App) hConferenciaList(w http.ResponseWriter, _ *http.Request) {
	rows, err := a.st.db.Query(`
		SELECT c.id, c.data, COALESCE(c.hora,''), COALESCE(c.local,''), c.status,
		       COALESCE(u.login,''), c.criado_em, c.fechada_em,
		       (SELECT COUNT(*) FROM presencas p WHERE p.conferencia_id = c.id) AS lanc
		FROM conferencias c
		LEFT JOIN usuarios u ON u.id = c.criado_por
		ORDER BY c.data DESC, c.id DESC`)
	if err != nil {
		jsonErro(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer rows.Close()
	out := []map[string]any{}
	for rows.Next() {
		var id int64
		var data, hora, local, status, criado, criadaEm string
		var fechada *string
		var lanc int
		if rows.Scan(&id, &data, &hora, &local, &status, &criado, &criadaEm, &fechada, &lanc) == nil {
			out = append(out, map[string]any{
				"id": id, "data": data, "hora": hora, "local": local, "status": status,
				"criado_por": criado, "criada_em": criadaEm, "fechada_em": fechada, "lancamentos": lanc,
			})
		}
	}
	jsonOK(w, out)
}

// hConferenciaGet: dados completos de UMA conferência (para o relatório na tela).
func (a *App) hConferenciaGet(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		jsonErro(w, http.StatusBadRequest, "id inválido")
		return
	}
	var (
		data, status, criadaEm string
		hora, local            *string
		fechada                *string
		criadoPor              string
	)
	err = a.st.db.QueryRow(`
		SELECT c.data, c.status, c.criado_em, c.hora, c.local, c.fechada_em, COALESCE(u.login,'')
		FROM conferencias c LEFT JOIN usuarios u ON u.id = c.criado_por
		WHERE c.id = ?`, id).
		Scan(&data, &status, &criadaEm, &hora, &local, &fechada, &criadoPor)
	if errors.Is(err, sql.ErrNoRows) {
		jsonErro(w, http.StatusNotFound, "conferência inexistente")
		return
	}
	if err != nil {
		jsonErro(w, http.StatusInternalServerError, err.Error())
		return
	}
	rows, e := a.st.db.Query(`
		SELECT p.nome_guerra, COALESCE(s.nome,'Sem setor'), pr.situacao,
		       COALESCE(d.nome,''), COALESCE(pr.observacao,''), u.login, pr.marcado_em
		FROM presencas pr
		JOIN pessoas p ON p.id = pr.pessoa_id
		LEFT JOIN setores s ON s.id = p.setor_id
		LEFT JOIN destinos d ON d.id = pr.destino_id
		JOIN usuarios u ON u.id = pr.marcado_por
		WHERE pr.conferencia_id = ?
		ORDER BY pr.situacao, p.nome_guerra`, id)
	if e != nil {
		jsonErro(w, http.StatusInternalServerError, e.Error())
		return
	}
	defer rows.Close()
	lanc := []map[string]any{}
	cont := map[string]int{}
	for rows.Next() {
		var ng, setor, sit, destino, obs, por, em string
		if rows.Scan(&ng, &setor, &sit, &destino, &obs, &por, &em) == nil {
			lanc = append(lanc, map[string]any{
				"nome_guerra": ng, "setor": setor, "situacao": sit,
				"destino": destino, "observacao": obs, "marcado_por": por, "marcado_em": em,
			})
			cont[sit]++
		}
	}
	jsonOK(w, map[string]any{
		"id": id, "data": data, "status": status, "hora": hora, "local": local,
		"criada_em": criadaEm, "fechada_em": fechada, "criado_por": criadoPor,
		"lancamentos": lanc,
		"resumo": map[string]any{
			"presentes": cont["presente"], "atrasos": cont["atraso"],
			"faltas": cont["falta"], "justificadas": cont["justificada"],
		},
	})
}

// hConferenciaPDF: relatório próprio — SOMENTE de conferência fechada (ordem Tenente 28/09).
func (a *App) hConferenciaPDF(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		jsonErro(w, http.StatusBadRequest, "id inválido")
		return
	}
	var (
		data, status, criadaEm string
		fechada                *string
		criadoPor              string
	)
	err = a.st.db.QueryRow(`
		SELECT c.data, c.status, c.criado_em, c.fechada_em, COALESCE(u.login,'')
		FROM conferencias c LEFT JOIN usuarios u ON u.id = c.criado_por
		WHERE c.id = ?`, id).
		Scan(&data, &status, &criadaEm, &fechada, &criadoPor)
	if errors.Is(err, sql.ErrNoRows) {
		jsonErro(w, http.StatusNotFound, "conferência inexistente")
		return
	}
	if err != nil {
		jsonErro(w, http.StatusInternalServerError, err.Error())
		return
	}
	if status != "fechada" {
		jsonErro(w, http.StatusConflict, "só é possível gerar relatório de conferência FECHADA")
		return
	}
	rows, e := a.st.db.Query(`
		SELECT p.nome_guerra, COALESCE(s.nome,'Sem setor'), pr.situacao,
		       COALESCE(d.nome,''), COALESCE(pr.observacao,''), u.login
		FROM presencas pr
		JOIN pessoas p ON p.id = pr.pessoa_id
		LEFT JOIN setores s ON s.id = p.setor_id
		LEFT JOIN destinos d ON d.id = pr.destino_id
		JOIN usuarios u ON u.id = pr.marcado_por
		WHERE pr.conferencia_id = ?
		ORDER BY pr.situacao, p.nome_guerra`, id)
	if e != nil {
		jsonErro(w, http.StatusInternalServerError, e.Error())
		return
	}
	defer rows.Close()
	lanc := []map[string]any{}
	resumo := map[string]int{"presentes": 0, "atrasos": 0, "faltas": 0, "justificadas": 0}
	for rows.Next() {
		var ng, setor, sit, destino, obs, por string
		if rows.Scan(&ng, &setor, &sit, &destino, &obs, &por) == nil {
			lanc = append(lanc, map[string]any{
				"nome_guerra": ng, "setor": setor, "situacao": sit,
				"destino": destino, "observacao": obs, "marcado_por": por,
			})
			switch sit {
			case "presente":
				resumo["presentes"]++
			case "atraso":
				resumo["atrasos"]++
			case "falta":
				resumo["faltas"]++
			case "justificada":
				resumo["justificadas"]++
			}
		}
	}
	u := usuarioDoCtx(r)
	cp := ConferenciaPDF{
		ID: id, Data: data, Status: status, CriadaEm: criadaEm, FechadaEm: fechada,
		CriadoPor: criadoPor, GeradoPor: u.Login, Resumo: resumo, Lancamentos: lanc,
	}
	pdf, err := a.gerarConferenciaPDF(cp)
	if err != nil {
		jsonErro(w, http.StatusInternalServerError, "falha ao gerar PDF: "+err.Error())
		return
	}
	a.st.Auditoria(&u.ID, "relatorio_conferencia", "conferencias", &id, data, ipDe(r))
	w.Header().Set("Content-Type", "application/pdf")
	w.Header().Set("Content-Disposition",
		fmt.Sprintf("attachment; filename=SCI_conferencia_%s_%d.pdf", data, id))
	_, _ = w.Write(pdf)
}

// ---------- relatórios ----------

func (a *App) montarBundle(de, ate string) Bundle {
	b := Bundle{De: de, Ate: ate}
	_ = a.st.db.QueryRow(`
		SELECT COUNT(*),
		       COALESCE(SUM(situacao='presente'),0), COALESCE(SUM(situacao='atraso'),0),
		       COALESCE(SUM(situacao='falta'),0), COALESCE(SUM(situacao='justificada'),0)
		FROM presencas p JOIN conferencias f ON f.id = p.conferencia_id
		WHERE f.status='fechada' AND f.data BETWEEN ? AND ?`,
		de, ate).Scan(&b.TotalLanc, &b.Presentes, &b.Atrasos, &b.Faltas, &b.Justificadas)
	// "efetivo pronto" = presentes SEM ressalva (ordem do Tenente, 28/09)
	_ = a.st.db.QueryRow(`
		SELECT COALESCE(SUM(p.situacao='presente'),0)
		FROM presencas p JOIN conferencias f ON f.id = p.conferencia_id
		WHERE f.status='fechada' AND f.data BETWEEN ? AND ?`, de, ate).Scan(&b.PresentesPuros)
	_ = a.st.db.QueryRow(`SELECT COUNT(*) FROM conferencias WHERE status='fechada' AND data BETWEEN ? AND ?`, de, ate).
		Scan(&b.Convocacoes)
	_ = a.st.db.QueryRow(`SELECT COUNT(*) FROM pessoas WHERE status='ativo'`).Scan(&b.EfetivoAtivo)
	// decisão Tenente 28/09: JUSTIFICADA = FALTA justificada → tudo que não é presente
	// pune o % de presença (exibição separa faltas justificadas x não justificadas)
	validas := b.Presentes + b.Atrasos
	denom := b.Convocacoes * b.EfetivoAtivo
	if denom > 0 {
		b.PctGeral = round1(100 * float64(validas) / float64(denom))
		b.PctPronto = round1(100 * float64(b.PresentesPuros) / float64(denom))
		b.PctPresencaEstrita = round1(100 * float64(b.PresentesPuros) / float64(denom))
		b.TotalFaltas = b.Faltas + b.Justificadas
	}

	rows, err := a.st.db.Query(`
		SELECT COALESCE(s.nome,'Sem setor') AS setor_nome,
		       COALESCE(SUM(pr.situacao IN ('presente','atraso')),0) AS pres,
		       COALESCE(SUM(pr.situacao='falta'),0) AS faltas,
		       COALESCE(SUM(pr.situacao='justificada'),0) AS just,
		       COUNT(pr.id) AS lanc
		FROM presencas pr
		JOIN pessoas p ON p.id = pr.pessoa_id
		LEFT JOIN setores s ON s.id = p.setor_id
		JOIN conferencias f ON f.id = pr.conferencia_id
		WHERE f.status='fechada' AND f.data BETWEEN ? AND ?
		GROUP BY setor_nome ORDER BY pres DESC`, de, ate)
	if err == nil {
		for rows.Next() {
			var r SetorStat
			if rows.Scan(&r.Setor, &r.Presencas, &r.Faltas, &r.Justificadas, &r.Lancados) == nil {
				if r.Lancados > 0 {
					r.Pct = round1(100 * float64(r.Presencas) / float64(r.Lancados))
				}
				b.PorSetor = append(b.PorSetor, r)
			}
		}
		rows.Close()
	}

	rows, err = a.st.db.Query(`
		SELECT COALESCE(d.nome,'(sem destino)'), COUNT(*)
		FROM presencas pr
		JOIN conferencias f ON f.id = pr.conferencia_id
		LEFT JOIN destinos d ON d.id = pr.destino_id
		WHERE f.status='fechada' AND f.data BETWEEN ? AND ?
		  AND pr.situacao IN ('falta','justificada')
		GROUP BY d.id ORDER BY 2 DESC`, de, ate)
	if err == nil {
		for rows.Next() {
			var r DestinoStat
			if rows.Scan(&r.Destino, &r.Quantidade) == nil {
				b.PorDestino = append(b.PorDestino, r)
			}
		}
		rows.Close()
	}

	rows, err = a.st.db.Query(`
		SELECT p.id, p.nome_guerra, COALESCE(s.nome,''),
		       COALESCE(SUM(pr.situacao IN ('presente','atraso')),0),
		       COALESCE(SUM(pr.situacao='atraso'),0),
		       COALESCE(SUM(pr.situacao='falta'),0),
		       COALESCE(SUM(pr.situacao='justificada'),0),
		       COUNT(pr.id)
		FROM pessoas p
		LEFT JOIN setores s ON s.id = p.setor_id
		LEFT JOIN presencas pr ON pr.pessoa_id = p.id
		LEFT JOIN conferencias f ON f.id = pr.conferencia_id AND f.status='fechada' AND f.data BETWEEN ? AND ?
		WHERE p.status='ativo'
		GROUP BY p.id ORDER BY 7 DESC, 6 DESC, p.nome_guerra`, de, ate)
	if err == nil {
		for rows.Next() {
			var r PessoaStat
			if rows.Scan(&r.ID, &r.NomeGuerra, &r.Setor, &r.Presencas, &r.Atrasos,
				&r.Faltas, &r.Justificadas, &r.Lancados) == nil {
				if b.Convocacoes > 0 {
					r.Pct = round1(100 * float64(r.Presencas) / float64(b.Convocacoes))
				}
				b.Pessoas = append(b.Pessoas, r)
			}
		}
		rows.Close()
	}

	rows, err = a.st.db.Query(`
		SELECT f.data, COALESCE(ft.nome,''), COALESCE(f.hora,''), f.status,
		       COALESCE(SUM(pr.situacao IN ('presente','atraso')),0),
		       COALESCE(SUM(pr.situacao='falta'),0)
		FROM conferencias f
		JOIN conferencia_tipos ft ON ft.id = f.tipo_id
		LEFT JOIN presencas pr ON pr.conferencia_id = f.id
		WHERE f.data BETWEEN ? AND ?
		GROUP BY f.id ORDER BY f.data DESC`, de, ate)
	if err == nil {
		for rows.Next() {
			var r FormaturaStat
			if rows.Scan(&r.Data, &r.Tipo, &r.Hora, &r.Status, &r.Presentes, &r.Faltas) == nil {
				b.Formaturas = append(b.Formaturas, r)
			}
		}
		rows.Close()
	}
	return b
}

func round1(v float64) float64 { return float64(int(v*10+0.5)) / 10 }

func (a *App) periodoPadrao(r *http.Request) (string, string) {
	hoje := time.Now().In(a.horaLocal)
	de := r.URL.Query().Get("de")
	ate := r.URL.Query().Get("ate")
	if ate == "" {
		ate = hoje.Format("2006-01-02")
	}
	if de == "" {
		de = hoje.AddDate(0, 0, -6).Format("2006-01-02")
	}
	return de, ate
}

func (a *App) hPresencaPeriodo(w http.ResponseWriter, r *http.Request) {
	de, ate := a.periodoPadrao(r)
	jsonOK(w, a.montarBundle(de, ate))
}

func (a *App) hRelatorioJSON(w http.ResponseWriter, r *http.Request) {
	de, ate := a.periodoPadrao(r)
	jsonOK(w, a.montarBundle(de, ate))
}

func (a *App) hRelatorioPDF(w http.ResponseWriter, r *http.Request) {
	de, ate := a.periodoPadrao(r)
	pdf, err := a.gerarRelatorioPDF(a.montarBundle(de, ate))
	if err != nil {
		jsonErro(w, http.StatusInternalServerError, "falha ao gerar PDF: "+err.Error())
		return
	}
	u := usuarioDoCtx(r)
	a.st.Auditoria(&u.ID, "exportar", "relatorio", nil, de+" a "+ate, ipDe(r))
	w.Header().Set("Content-Type", "application/pdf")
	w.Header().Set("Content-Disposition",
		fmt.Sprintf("attachment; filename=SCI_relatorio_%s_%s.pdf", de, ate))
	_, _ = w.Write(pdf)
}

// ---------- exportação CSV (portabilidade — o dado nunca fica preso) ----------

func (a *App) hExportCSV(w http.ResponseWriter, r *http.Request) {
	t := r.PathValue("t")
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", "attachment; filename=sci_"+t+".csv")
	_, _ = w.Write([]byte{0xEF, 0xBB, 0xBF}) // BOM p/ Excel
	cw := csv.NewWriter(w)
	switch t {
	case "pessoas":
		_ = cw.Write([]string{"id", "nome_guerra", "nome_completo", "setor", "funcao", "status", "criado_em"})
		rows, err := a.st.db.Query(`SELECT p.id, p.nome_guerra, p.nome_completo,
			COALESCE(s.nome,''), COALESCE(fu.nome,''), p.status, p.criado_em
			FROM pessoas p LEFT JOIN setores s ON s.id=p.setor_id
			LEFT JOIN funcoes fu ON fu.id=p.funcao_id ORDER BY p.id`)
		if err == nil {
			defer rows.Close()
			for rows.Next() {
				var id int64
				var v [6]string
				if rows.Scan(&id, &v[0], &v[1], &v[2], &v[3], &v[4], &v[5]) == nil {
					_ = cw.Write([]string{strconv.FormatInt(id, 10), v[0], v[1], v[2], v[3], v[4], v[5]})
				}
			}
		}
	case "presencas":
		_ = cw.Write([]string{"data", "conferencia_tipo", "nome_guerra", "situacao", "destino", "marcado_em"})
		rows, err := a.st.db.Query(`SELECT f.data, ft.nome, p.nome_guerra, pr.situacao,
			COALESCE(d.nome,''), pr.marcado_em
			FROM presencas pr JOIN conferencias f ON f.id=pr.conferencia_id
			JOIN conferencia_tipos ft ON ft.id=f.tipo_id
			JOIN pessoas p ON p.id=pr.pessoa_id LEFT JOIN destinos d ON d.id=pr.destino_id
			ORDER BY f.data, p.nome_guerra`)
		if err == nil {
			defer rows.Close()
			for rows.Next() {
				var v [6]string
				if rows.Scan(&v[0], &v[1], &v[2], &v[3], &v[4], &v[5]) == nil {
					_ = cw.Write(v[:])
				}
			}
		}
	case "conferencias":
		_ = cw.Write([]string{"data", "hora", "tipo", "local", "status"})
		rows, err := a.st.db.Query(`SELECT f.data, COALESCE(f.hora,''), ft.nome,
			COALESCE(f.local,''), f.status FROM conferencias f
			JOIN conferencia_tipos ft ON ft.id=f.tipo_id ORDER BY f.data`)
		if err == nil {
			defer rows.Close()
			for rows.Next() {
				var v [5]string
				if rows.Scan(&v[0], &v[1], &v[2], &v[3], &v[4]) == nil {
					_ = cw.Write(v[:])
				}
			}
		}
	default:
		_ = cw.Write([]string{"erro", "entidade inválida"})
	}
	cw.Flush()
}

// ---------- catálogos ----------

func (a *App) hCatalogoList(w http.ResponseWriter, r *http.Request) {
	t, err := tabelaDeCatalogo(r.PathValue("t"))
	if err != nil {
		jsonErro(w, http.StatusBadRequest, err.Error())
		return
	}
	extra := ""
	switch t {
	case "setores":
		extra = ", sigla"
	case "tags":
		extra = ", cor"
	}
	rows, err := a.st.db.Query(`SELECT id, nome` + extra + `, ativo FROM ` + t + ` ORDER BY ativo DESC, nome`)
	if err != nil {
		jsonErro(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer rows.Close()
	out := []map[string]any{}
	cols := []string{"id", "nome"}
	if extra != "" {
		cols = append(cols, strings.TrimPrefix(extra, ", "))
	}
	cols = append(cols, "ativo")
	vals := make([]any, len(cols))
	ptrs := make([]any, len(cols))
	for i := range vals {
		ptrs[i] = &vals[i]
	}
	for rows.Next() {
		if err := rows.Scan(ptrs...); err == nil {
			linha := map[string]any{}
			for i, c := range cols {
				linha[c] = vals[i]
			}
			out = append(out, linha)
		}
	}
	jsonOK(w, out)
}

func (a *App) hCatalogoAdd(w http.ResponseWriter, r *http.Request) {
	t, err := tabelaDeCatalogo(r.PathValue("t"))
	if err != nil {
		jsonErro(w, http.StatusBadRequest, err.Error())
		return
	}
	var req struct {
		Nome  string `json:"nome"`
		Sigla string `json:"sigla"`
		Cor   string `json:"cor"`
	}
	if err = decodificar(r, &req); err != nil || strings.TrimSpace(req.Nome) == "" {
		jsonErro(w, http.StatusBadRequest, "nome obrigatório")
		return
	}
	nome := strings.TrimSpace(req.Nome)
	var q string
	switch t {
	case "setores":
		q = `INSERT INTO setores (nome, sigla) VALUES (?, NULLIF(?,''))`
	case "tags":
		q = `INSERT INTO tags (nome, cor) VALUES (?, NULLIF(?,''))`
	default:
		q = `INSERT INTO ` + t + ` (nome) VALUES (?)`
	}
	var res interface {
		LastInsertId() (int64, error)
	}
	if t == "setores" || t == "tags" {
		res, err = a.st.db.Exec(q, nome, map[bool]string{true: req.Sigla, false: req.Cor}[t == "setores"])
	} else {
		res, err = a.st.db.Exec(q, nome)
	}
	if err != nil {
		jsonErro(w, http.StatusBadRequest, "não inserido (duplicado?): "+err.Error())
		return
	}
	id, _ := res.LastInsertId()
	u := usuarioDoCtx(r)
	a.st.Auditoria(&u.ID, "criar", t, &id, nome, ipDe(r))
	jsonOK(w, map[string]any{"id": id, "nome": nome})
}

func (a *App) hCatalogoDel(w http.ResponseWriter, r *http.Request) {
	t, err := tabelaDeCatalogo(r.PathValue("t"))
	if err != nil {
		jsonErro(w, http.StatusBadRequest, err.Error())
		return
	}
	id, err2 := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err2 != nil {
		jsonErro(w, http.StatusBadRequest, "id inválido")
		return
	}
	u := usuarioDoCtx(r)
	// padrão: EXCLUSÃO real (ordem do Tenente 28/09). Em uso → 409 (FK).
	if r.URL.Query().Get("modo") != "desativar" {
		if _, err = a.st.db.Exec(`DELETE FROM `+t+` WHERE id = ?`, id); err != nil {
			jsonErro(w, http.StatusConflict, "item em uso por lançamentos/cadastros — pode desativar em vez de excluir")
			return
		}
		a.st.Auditoria(&u.ID, "excluir", t, &id, "", ipDe(r))
		jsonOK(w, map[string]bool{"ok": true})
		return
	}
	// alternativa conservadora: desativação (histórico preservado)
	if _, err = a.st.db.Exec(`UPDATE `+t+` SET ativo = 0 WHERE id = ?`, id); err != nil {
		jsonErro(w, http.StatusInternalServerError, err.Error())
		return
	}
	a.st.Auditoria(&u.ID, "desativar", t, &id, "", ipDe(r))
	jsonOK(w, map[string]bool{"ok": true})
}

// ---------- pessoas ----------

func (a *App) hPessoasList(w http.ResponseWriter, _ *http.Request) {
	jsonOK(w, map[string]any{"pessoas": a.pessoasTodas()})
}

func (a *App) pessoasTodas() []map[string]any {
	rows, err := a.st.db.Query(`
		SELECT p.id, p.nome_guerra, p.nome_completo, p.setor_id, p.funcao_id, p.status,
		       COALESCE(s.nome,''), COALESCE(fu.nome,'')
		FROM pessoas p
		LEFT JOIN setores s ON s.id = p.setor_id
		LEFT JOIN funcoes fu ON fu.id = p.funcao_id
		ORDER BY p.status, COALESCE(s.nome,''), p.nome_guerra`)
	if err != nil {
		return []map[string]any{}
	}
	defer rows.Close()
	out := []map[string]any{}
	for rows.Next() {
		var id, setorID, funcaoID int64
		var ng, nc, status, setor, funcao string
		if rows.Scan(&id, &ng, &nc, &setorID, &funcaoID, &status, &setor, &funcao) == nil {
			out = append(out, map[string]any{
				"id": id, "nome_guerra": ng, "nome_completo": nc,
				"setor_id": setorID, "funcao_id": funcaoID,
				"setor": setor, "funcao": funcao, "status": status,
			})
		}
	}
	return out
}

func (a *App) hPessoasAdd(w http.ResponseWriter, r *http.Request) {
	var req struct {
		NomeGuerra    string `json:"nome_guerra"`
		NomeCompleto  string `json:"nome_completo"`
		SetorID       *int64 `json:"setor_id"`
		FuncaoID      *int64 `json:"funcao_id"`
		Status        string `json:"status"`
	}
	if err := decodificar(r, &req); err != nil || req.NomeGuerra == "" || req.NomeCompleto == "" {
		jsonErro(w, http.StatusBadRequest, "nome de guerra e nome completo obrigatórios")
		return
	}
	if req.Status == "" {
		req.Status = "ativo"
	}
	res, err := a.st.db.Exec(
		`INSERT INTO pessoas (nome_guerra, nome_completo, setor_id, funcao_id, status) VALUES (?,?,?,?,?)`,
		req.NomeGuerra, req.NomeCompleto, req.SetorID, req.FuncaoID, req.Status)
	if err != nil {
		jsonErro(w, http.StatusBadRequest, err.Error())
		return
	}
	id, _ := res.LastInsertId()
	u := usuarioDoCtx(r)
	a.st.Auditoria(&u.ID, "criar", "pessoas", &id, req.NomeGuerra, ipDe(r))
	jsonOK(w, map[string]any{"id": id})
}

func (a *App) hPessoasEdit(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		jsonErro(w, http.StatusBadRequest, "id inválido")
		return
	}
	var req struct {
		NomeGuerra    string `json:"nome_guerra"`
		NomeCompleto  string `json:"nome_completo"`
		Status        string `json:"status"`
		SetorID       *int64 `json:"setor_id"`
		FuncaoID      *int64 `json:"funcao_id"`
	}
	if err = decodificar(r, &req); err != nil {
		jsonErro(w, http.StatusBadRequest, "JSON inválido")
		return
	}
	if req.Status == "" {
		req.Status = "ativo"
	}
	_, err = a.st.db.Exec(
		`UPDATE pessoas SET nome_guerra=?, nome_completo=?, setor_id=?, funcao_id=?, status=?,
		 atualizado_em=strftime('%Y-%m-%dT%H:%M:%fZ','now') WHERE id=?`,
		req.NomeGuerra, req.NomeCompleto, req.SetorID, req.FuncaoID, req.Status, id)
	if err != nil {
		jsonErro(w, http.StatusBadRequest, err.Error())
		return
	}
	u := usuarioDoCtx(r)
	a.st.Auditoria(&u.ID, "alterar", "pessoas", &id, req.NomeGuerra, ipDe(r))
	jsonOK(w, map[string]bool{"ok": true})
}

// ---------- usuários ----------

func (a *App) hUsuariosList(w http.ResponseWriter, _ *http.Request) {
	rows, err := a.st.db.Query(
		`SELECT id, login, papel, pessoa_id, ativo, criado_em, senhas FROM usuarios ORDER BY id`)
	if err != nil {
		jsonErro(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer rows.Close()
	out := []map[string]any{}
	for rows.Next() {
		var id int64
		var login, papel, criado, senhas string
		var pessoaID *int64
		var ativo int
		if rows.Scan(&id, &login, &papel, &pessoaID, &ativo, &criado, &senhas) == nil {
			out = append(out, map[string]any{
				"id": id, "login": login, "papel": papel, "pessoa_id": pessoaID,
				"ativo": ativo == 1, "criado_em": criado,
				"senhas": json.RawMessage(senhas),
			})
		}
	}
	jsonOK(w, out)
}

// ---------- grupos e escopo (fase GRUPOS — ordem Tenente 28/09) ----------

// escopoDoUsuario: 0 = vê tudo (admin); N = só o grupo N (gerente/operador).
func escopoDoUsuario(u *Usuario) int64 {
	if u == nil {
		return 0
	}
	if u.Papel == "admin" || u.GrupoID == nil {
		return 0
	}
	return *u.GrupoID
}

// podeAdministrar: admin sempre; gerente dentro do próprio grupo.
func podeAdministrar(u *Usuario) bool {
	return u != nil && (u.Papel == "admin" || u.Papel == "gerente")
}

// hGruposList / hGruposAdd / hGruposDel: gestão de grupos (só admin cria).
func (a *App) hGruposList(w http.ResponseWriter, _ *http.Request) {
	rows, err := a.st.db.Query(`
		SELECT g.id, g.nome, g.criado_em,
		       (SELECT COUNT(*) FROM usuarios u WHERE u.grupo_id = g.id) AS contas,
		       (SELECT COUNT(*) FROM pessoas p WHERE p.grupo_id = g.id) AS efetivo
		FROM grupos g ORDER BY g.nome`)
	if err != nil {
		jsonErro(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer rows.Close()
	out := []map[string]any{}
	for rows.Next() {
		var id int64
		var nome, criado string
		var contas, efetivo int
		if rows.Scan(&id, &nome, &criado, &contas, &efetivo) == nil {
			out = append(out, map[string]any{
				"id": id, "nome": nome, "criado_em": criado, "contas": contas, "efetivo": efetivo,
			})
		}
	}
	jsonOK(w, out)
}

func (a *App) hGruposAdd(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Nome string `json:"nome"`
	}
	if err := decodificar(r, &req); err != nil || strings.TrimSpace(req.Nome) == "" {
		jsonErro(w, http.StatusBadRequest, "nome do grupo obrigatório")
		return
	}
	res, err := a.st.db.Exec(`INSERT INTO grupos (nome) VALUES (?)`, strings.TrimSpace(req.Nome))
	if err != nil {
		jsonErro(w, http.StatusBadRequest, "não criado (duplicado?): "+err.Error())
		return
	}
	id, _ := res.LastInsertId()
	u := usuarioDoCtx(r)
	a.st.Auditoria(&u.ID, "criar", "grupos", &id, req.Nome, ipDe(r))
	jsonOK(w, map[string]any{"id": id})
}

// hComentariosAdd: comentário append-only sobre pessoa em conferência (ordem Tenente).
func (a *App) hComentariosAdd(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ConferenciaID int64  `json:"conferencia_id"`
		PessoaID      int64  `json:"pessoa_id"`
		Comentario    string `json:"comentario"`
	}
	if err := decodificar(r, &req); err != nil || req.ConferenciaID == 0 || req.PessoaID == 0 ||
		strings.TrimSpace(req.Comentario) == "" {
		jsonErro(w, http.StatusBadRequest, "conferencia_id, pessoa_id e comentario obrigatórios")
		return
	}
	u := usuarioDoCtx(r)
	// ordem incremental por conferência
	var ord int64
	_ = a.st.db.QueryRow(`SELECT COALESCE(MAX(ordem),0)+1 FROM comentarios WHERE conferencia_id = ?`,
		req.ConferenciaID).Scan(&ord)
	res, err := a.st.db.Exec(
		`INSERT INTO comentarios (ordem, conferencia_id, pessoa_id, operador_id, comentario) VALUES (?,?,?,?,?)`,
		ord, req.ConferenciaID, req.PessoaID, u.ID, strings.TrimSpace(req.Comentario))
	if err != nil {
		jsonErro(w, http.StatusInternalServerError, err.Error())
		return
	}
	id, _ := res.LastInsertId()
	a.st.Auditoria(&u.ID, "comentar", "comentarios", &id,
		fmt.Sprintf("conf=%d pessoa=%d", req.ConferenciaID, req.PessoaID), ipDe(r))
	jsonOK(w, map[string]any{"id": id, "ordem": ord})
}

func (a *App) hComentariosList(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		jsonErro(w, http.StatusBadRequest, "id inválido")
		return
	}
	rows, err := a.st.db.Query(`
		SELECT c.ordem, c.criado_em, p.nome_guerra, u.login, c.comentario
		FROM comentarios c
		JOIN pessoas p ON p.id = c.pessoa_id
		JOIN usuarios u ON u.id = c.operador_id
		WHERE c.conferencia_id = ? ORDER BY c.ordem`, id)
	if err != nil {
		jsonErro(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer rows.Close()
	out := []map[string]any{}
	for rows.Next() {
		var ord int64
		var em, nome, por, comentario string
		if rows.Scan(&ord, &em, &nome, &por, &comentario) == nil {
			out = append(out, map[string]any{
				"ordem": ord, "datahora": em, "pessoa": nome,
				"operador": por, "comentario": comentario,
			})
		}
	}
	jsonOK(w, out)
}

func (a *App) hUsuariosAdd(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Login    string `json:"login"`
		Senha    string `json:"senha"`
		Papel    string `json:"papel"`
		PessoaID *int64 `json:"pessoa_id"`
		GrupoID  *int64 `json:"grupo_id"`
	}
	if err := decodificar(r, &req); err != nil || req.Login == "" || req.Senha == "" {
		jsonErro(w, http.StatusBadRequest, "login e senha obrigatórios")
		return
	}
	u := usuarioDoCtx(r)
	papel := strings.ToLower(strings.TrimSpace(req.Papel))
	switch papel {
	case "admin", "gerente", "usuario":
	default:
		jsonErro(w, http.StatusBadRequest, "papel inválido (admin | gerente | usuario)")
		return
	}
	// hierarquia de criação (ordem Tenente 28/09):
	// - ADMIN é o ÚNICO que cria GERENTE (e admin)
	// - GERENTE cria OPERADOR, sempre no PRÓPRIO grupo
	if u.Papel != "admin" {
		if papel != "usuario" {
			jsonErro(w, http.StatusForbidden, "somente o admin cria gerentes")
			return
		}
		if u.GrupoID == nil {
			jsonErro(w, http.StatusForbidden, "gerente sem grupo definido")
			return
		}
		req.GrupoID = u.GrupoID // força o próprio grupo, ignore o que vier no corpo
	}
	if papel == "admin" {
		req.GrupoID = nil // admin é global
	}
	hash, err := hashSenha(req.Senha)
	if err != nil {
		jsonErro(w, http.StatusInternalServerError, err.Error())
		return
	}
	res, err := a.st.db.Exec(
		`INSERT INTO usuarios (login, senha_hash, papel, pessoa_id, grupo_id) VALUES (?,?,?,?,?)`,
		strings.ToLower(strings.TrimSpace(req.Login)), hash, papel, req.PessoaID, req.GrupoID)
	if err != nil {
		jsonErro(w, http.StatusBadRequest, "não criado (duplicado?): "+err.Error())
		return
	}
	id, _ := res.LastInsertId()
	a.st.Auditoria(&u.ID, "criar", "usuarios", &id, req.Login+" ("+papel+")", ipDe(r))
	jsonOK(w, map[string]any{"id": id})
}

// ---------- SPA ----------

func (a *App) hSPA(w http.ResponseWriter, r *http.Request) {
	sub, err := fs.Sub(webEmbed, "web")
	if err != nil {
		http.Error(w, "assets ausentes", http.StatusInternalServerError)
		return
	}
	caminho := strings.TrimPrefix(r.URL.Path, "/")
	if caminho == "" {
		caminho = "index.html"
	}
	if _, err := fs.Stat(sub, caminho); err != nil {
		caminho = "index.html" // fallback SPA
	}
	b, err := fs.ReadFile(sub, caminho)
	if err != nil {
		http.Error(w, "não encontrado", http.StatusNotFound)
		return
	}
	switch {
	case strings.HasSuffix(caminho, ".html"):
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "no-cache")
	case strings.HasSuffix(caminho, ".js"):
		w.Header().Set("Content-Type", "text/javascript; charset=utf-8")
		w.Header().Set("Cache-Control", "no-cache")
	case strings.HasSuffix(caminho, ".css"):
		w.Header().Set("Content-Type", "text/css; charset=utf-8")
		w.Header().Set("Cache-Control", "no-cache")
	case strings.HasSuffix(caminho, ".svg"):
		w.Header().Set("Content-Type", "image/svg+xml")
	}
	_, _ = w.Write(b)
}
