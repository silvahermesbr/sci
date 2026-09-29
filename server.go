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
	"log"
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

	// abas de conferência/presença: GERENTE e OPERADOR apenas (R2/R11 — admin tem nav própria)
	confAuth := func(h http.HandlerFunc) http.Handler { return a.authPapeis([]string{"gerente", "operador"}, h) }
	m.Handle("GET /api/conferencia/hoje", confAuth(a.hConferenciaHoje))
	m.Handle("POST /api/conferencia/iniciar", confAuth(a.hConferenciaIniciar))
	m.Handle("POST /api/conferencia/fechar", confAuth(a.hConferenciaFechar))
	m.Handle("GET /api/conferencia/lista", confAuth(a.hConferenciaList))
	m.Handle("GET /api/conferencia/{id}", confAuth(a.hConferenciaGet))
	m.Handle("GET /api/conferencia/{id}/relatorio.pdf", confAuth(a.hConferenciaPDF))

	m.Handle("GET /api/presenca/periodo", confAuth(a.hPresencaPeriodo))
	m.Handle("GET /api/conferencias", confAuth(a.hConferenciaList))

	m.Handle("GET /api/catalogo/{t}", a.auth(false, a.hCatalogoList))
	m.Handle("POST /api/catalogo/{t}", a.auth(false, a.hCatalogoAdd))
	m.Handle("DELETE /api/catalogo/{t}/{id}", a.auth(false, a.hCatalogoDel))

	m.Handle("GET /api/pessoas", a.auth(false, a.hPessoasList))
	m.Handle("POST /api/pessoas", a.auth(false, a.hPessoasAdd))
	m.Handle("PATCH /api/pessoas/{id}", a.auth(false, a.hPessoasEdit))

	m.Handle("GET /api/usuarios", a.auth(true, a.hUsuariosList))
	// criação é validada DENTRO do handler (admin cria qualquer; gerente cria operador do próprio grupo)
	m.Handle("POST /api/usuarios", a.auth(false, a.hUsuariosAdd))
	m.Handle("DELETE /api/usuarios/{id}", a.auth(true, a.hUsuarioExcluir)) // R6
	m.Handle("POST /api/usuarios/{id}/senha", a.auth(true, a.hUsuarioSenha))
	m.Handle("GET /api/grupos", a.auth(false, a.hGruposList))
	m.Handle("POST /api/grupos", a.auth(true, a.hGruposAdd)) // R7: exige gerente no ato
	m.Handle("GET /api/grupos/{id}/gerente", a.auth(true, a.hGrupoGerenteGet))
	m.Handle("POST /api/grupos/{id}/trocar-gerente", a.auth(true, a.hGrupoTrocarGerente))
	m.Handle("POST /api/admin/grupos/vinculo", a.auth(true, a.hAdminVinculoSet))   // R8
	m.Handle("DELETE /api/admin/grupos/vinculo", a.auth(true, a.hAdminVinculoRem)) // R8
	m.Handle("GET /api/vinculos", a.auth(false, a.hVinculoList))
	m.Handle("POST /api/vinculos", a.auth(false, a.hVinculoAdd)) // legado: fora da UI (R12)
	m.Handle("GET /api/perfil", a.auth(false, a.hPerfilGet))
	m.Handle("PATCH /api/perfil", a.auth(false, a.hPerfilSet))
	m.Handle("PATCH /api/usuarios/{id}/mover", a.auth(true, a.hMoverConta))
	m.Handle("POST /api/comentarios", confAuth(a.hComentariosAdd))
	m.Handle("GET /api/comentarios/{id}", confAuth(a.hComentariosList))

	m.Handle("GET /api/relatorio", a.auth(false, a.hRelatorioJSON))
	m.Handle("GET /api/relatorio.pdf", a.auth(false, a.hRelatorioPDF))
	m.Handle("GET /api/export/{t}", a.auth(false, a.hExportCSV))

	m.Handle("POST /api/backup", a.auth(true, a.hBackup))
	m.Handle("POST /api/backup/importar", a.auth(true, a.hBackupImportar)) // R9
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
	// admin não tem grupo: nunca há "conferência do admin" — a área fica em modo leitura
	if escopo == 0 && u.Papel == "admin" {
		jsonOK(w, map[string]any{"conferencia": nil, "pessoas": a.pessoasAtivas(0)})
		return
	}
	var err error
	err = a.st.db.QueryRow(
		`SELECT id, status, data, criado_em FROM conferencias
		 WHERE status = 'aberta' AND grupo_id = ? ORDER BY id DESC LIMIT 1`, escopo).
		Scan(&f.ID, &f.Status, &f.Data, &f.CriadaEm)
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
	jsonOK(w, map[string]any{"conferencia": form, "pessoas": a.pessoasAtivas(escopo)})
}

type lancamentoReq struct {
	PessoaID   int64  `json:"pessoa_id"`
	Situacao   string `json:"situacao"`
	DestinoID  *int64 `json:"destino_id"`
	TagID      *int64 `json:"tag_id"`
	Observacao string `json:"observacao"`
}

// pessoasAtivas(escopo): escopo 0 = todas (admin); N = só do grupo N.
func (a *App) pessoasAtivas(escopo int64) []map[string]any {
	q := `
		SELECT p.id, p.nome_guerra, p.nome_completo, COALESCE(s.nome,''), COALESCE(fu.nome,'')
		FROM pessoas p
		LEFT JOIN setores s ON s.id = p.setor_id
		LEFT JOIN funcoes fu ON fu.id = p.funcao_id
		WHERE p.status = 'ativo'`
	var rows *sql.Rows
	var err error
	if escopo > 0 {
		rows, err = a.st.db.Query(q+` AND p.grupo_id = ?
			ORDER BY COALESCE(s.nome,''), p.nome_guerra`, escopo)
	} else {
		rows, err = a.st.db.Query(q + `
			ORDER BY COALESCE(s.nome,''), p.nome_guerra`)
	}
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

// hConferenciaIniciar: cria a conferência com data/hora de AGORA (Brasília) e a deixa ABERTA.
func (a *App) hConferenciaIniciar(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Local string `json:"local"`
	}
	_ = decodificar(r, &req)
	// ordem Tenente (28/09): data/hora são coletadas do relógio — horário de Brasília
	// REGRA (28/09): ADMIN NÃO inicia conferência — só gerente/operador de grupo.
	u := usuarioDoCtx(r)
	if u.Papel == "admin" {
		jsonErro(w, http.StatusForbidden, "o admin não inicia conferências — quem inicia é o gerente/operador de um grupo")
		return
	}
	if u.GrupoID == nil {
		jsonErro(w, http.StatusForbidden, "conta sem grupo definido")
		return
	}
	grupoID := *u.GrupoID
	data := time.Now().In(a.horaLocal).Format("2006-01-02")
	tipoID, _, err := a.tipoPadraoID()
	if err != nil {
		jsonErro(w, http.StatusInternalServerError, "tipo '"+tipoConferenciaPadrao+"' inexistente")
		return
	}
	// já existe conferência aberta DO GRUPO?
	var abertaID int64
	if e := a.st.db.QueryRow(
		`SELECT id FROM conferencias WHERE status = 'aberta' AND grupo_id = ? LIMIT 1`, grupoID).
		Scan(&abertaID); e == nil {
		jsonErro(w, http.StatusConflict, "já existe uma conferência aberta do seu grupo (feche-a antes de iniciar outra)")
		return
	}
	// várias por dia: permitido (ordem Tenente); UNIQUE(data,tipo) removida na v4
	var id int64
	res, e := a.st.db.Exec(
		`INSERT INTO conferencias (data, tipo_id, local, grupo_id, criado_por) VALUES (?,?,?,?,?)`,
		data, tipoID, req.Local, grupoID, u.ID)
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
	// REGRA (28/09): só quem pertence ao grupo da conferência a fecha
	if esc := escopoDoUsuario(u); esc > 0 {
		var gid *int64
		qerr := a.st.db.QueryRow(`SELECT grupo_id FROM conferencias WHERE id = ?`, req.ID).Scan(&gid)
		if qerr != nil {
			jsonErro(w, http.StatusNotFound, "conferência inexistente")
			return
		}
		if gid == nil || *gid != esc {
			jsonErro(w, http.StatusForbidden, "conferência de outro grupo")
			return
		}
	}

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

// hConferenciaList: conferências DO ESCOPO (grupo não vê grupo; admin vê todas).
func (a *App) hConferenciaList(w http.ResponseWriter, r *http.Request) {
	u := usuarioDoCtx(r)
	escopo := escopoDoUsuario(u)
	q := `
		SELECT c.id, c.data, COALESCE(c.hora,''), COALESCE(c.local,''), c.status,
		       COALESCE(u.login,''), c.criado_em, c.fechada_em,
		       (SELECT COUNT(*) FROM presencas p WHERE p.conferencia_id = c.id) AS lanc,
		       COALESCE(c.grupo_id,0), COALESCE((SELECT g.nome FROM grupos g WHERE g.id = c.grupo_id),'—')
		FROM conferencias c
		LEFT JOIN usuarios u ON u.id = c.criado_por`
	var rows *sql.Rows
	var err error
	if escopo > 0 {
		rows, err = a.st.db.Query(q+` WHERE c.grupo_id = ?
			ORDER BY c.data DESC, c.id DESC`, escopo)
	} else {
		rows, err = a.st.db.Query(q + `
			ORDER BY c.data DESC, c.id DESC`)
	}
	if err != nil {
		jsonErro(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer rows.Close()
	out := []map[string]any{}
	for rows.Next() {
		var id, grupoID int64
		var data, hora, local, status, criado, criadaEm string
		var fechada *string
		var lanc int
		var grupoNome string
		if rows.Scan(&id, &data, &hora, &local, &status, &criado, &criadaEm, &fechada, &lanc, &grupoID, &grupoNome) == nil {
			out = append(out, map[string]any{
				"id": id, "data": data, "hora": hora, "local": local, "status": status,
				"criado_por": criado, "criada_em": criadaEm, "fechada_em": fechada, "lancamentos": lanc,
				"grupo_id": grupoID, "grupo": grupoNome,
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
	// IDOR + HERANÇA (ordem Tenente 28/09 noite): grupo acessa a PRÓPRIA conferência;
	// superior acessa TAMBÉM as de subordinados com vínculo ativo (relatório fechado).
	uCtx := usuarioDoCtx(r)
	if esc := escopoDoUsuario(uCtx); esc > 0 {
		var gid int64
		qerr := a.st.db.QueryRow(`SELECT COALESCE(grupo_id,0) FROM conferencias WHERE id = ?`, id).Scan(&gid)
		if qerr != nil || (gid != esc && !int64Contem(a.gruposSubordinadosAtivos(esc), gid)) {
			jsonErro(w, http.StatusForbidden, "conferência de outro grupo")
			return
		}
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
	// IDOR + HERANÇA (28/09 noite): superior gera o relatório FECHADO do subordinado.
	uCtx := usuarioDoCtx(r)
	if esc := escopoDoUsuario(uCtx); esc > 0 {
		var gid int64
		qerr := a.st.db.QueryRow(`SELECT COALESCE(grupo_id,0) FROM conferencias WHERE id = ?`, id).Scan(&gid)
		if qerr != nil || (gid != esc && !int64Contem(a.gruposSubordinadosAtivos(esc), gid)) {
			jsonErro(w, http.StatusForbidden, "conferência de outro grupo")
			return
		}
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
	a.st.Auditoria(&u.ID, "exportar", "conferencia", &id, "pdf", ipDe(r))
	w.Header().Set("Content-Type", "application/pdf")
	w.Header().Set("Content-Disposition",
		fmt.Sprintf("inline; filename=SCI_conferencia_%s_%d.pdf", data, id))
	_, _ = w.Write(pdf)
}

// ---------- relatórios ----------

func (a *App) montarBundle(de, ate string, escopo int64) Bundle {
	b := Bundle{De: de, Ate: ate}
	// filtro de escopo (revisão TAKEDA/SHORYU): grupo só agrega o próprio grupo
	// + hierarquia (ordem 28/09 noite): superior agrega subordinados ativos
	filtro := ` WHERE f.status='fechada' AND f.data BETWEEN ? AND ?`
	args := []any{de, ate}
	if escopo > 0 {
		ft := a.filtroArvore(escopo, "f")
		filtro += ft.clause
		args = append(args, ft.args...)
	}
	_ = a.st.db.QueryRow(`
		SELECT COUNT(*),
		       COALESCE(SUM(p.situacao='presente'),0), COALESCE(SUM(p.situacao='atraso'),0),
		       COALESCE(SUM(p.situacao='falta'),0), COALESCE(SUM(p.situacao='justificada'),0)
		FROM presencas p JOIN conferencias f ON f.id = p.conferencia_id`+filtro,
		args...).Scan(&b.TotalLanc, &b.Presentes, &b.Atrasos, &b.Faltas, &b.Justificadas)
	// "efetivo pronto" = presentes SEM ressalva (ordem do Tenente, 28/09)
	_ = a.st.db.QueryRow(`
		SELECT COALESCE(SUM(p.situacao='presente'),0)
		FROM presencas p JOIN conferencias f ON f.id = p.conferencia_id`+filtro,
		args...).Scan(&b.PresentesPuros)
	// FIX S4-P0 (verif5): conferencias precisa do MESMO alias "f" da cláusula de
	// árvore — sem alias, "no such column: f.grupo_id" era engolido pelo `_ =`
	// e convocacoes ficava 0 (zerando todos os % do relatório).
	qConv := `SELECT COUNT(*) FROM conferencias f WHERE f.status='fechada' AND f.data BETWEEN ? AND ?`
	qcArgs := append([]any{de, ate}, a.argsArvore(escopo)...)
	if escopo > 0 {
		qConv += a.clSetor(escopo)
	}
	// erros de contagem-base NUNCA silenciosos (lição v9): logar.
	if err := a.st.db.QueryRow(qConv, qcArgs...).Scan(&b.Convocacoes); err != nil {
		log.Printf("sci relatorio: convocacoes escopo=%d: %v", escopo, err)
	}
	// FIX S4-P1: efetivo ativo conta a ÁRVORE do escopo (mesma base dos lançamentos);
	// antes era só o grupo próprio → denominador misturava escopos.
	qEfetivo := `SELECT COUNT(*) FROM pessoas p WHERE p.status='ativo'`
	var efetArgs []any
	if escopo > 0 {
		ftP := a.filtroArvore(escopo, "p")
		qEfetivo += ftP.clause
		efetArgs = ftP.args
	}
	if err := a.st.db.QueryRow(qEfetivo, efetArgs...).Scan(&b.EfetivoAtivo); err != nil {
		log.Printf("sci relatorio: efetivo escopo=%d: %v", escopo, err)
	}
	// decisão Tenente 28/09: JUSTIFICADA = FALTA justificada → tudo que não é presente
	// pune o % de presença (exibição separa faltas justificadas x não justificadas)
	validas := b.Presentes + b.Atrasos
	denom := b.Convocacoes * b.EfetivoAtivo
	// FIX S4-P1: total_faltas é contagem de lançamentos — independe do denominador;
	// antes ficava 0 junto com os % quando denom=0 (falta real escondida).
	b.TotalFaltas = b.Faltas + b.Justificadas
	if denom > 0 {
		b.PctGeral = round1(100 * float64(validas) / float64(denom))
		b.PctPronto = round1(100 * float64(b.PresentesPuros) / float64(denom))
		b.PctPresencaEstrita = round1(100 * float64(b.PresentesPuros) / float64(denom))
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
		WHERE f.status='fechada' AND f.data BETWEEN ? AND ?`+
		a.clSetor(escopo)+`
		GROUP BY setor_nome ORDER BY pres DESC`, append([]any{de, ate}, a.argsArvore(escopo)...)...)
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
		  AND pr.situacao IN ('falta','justificada')`+
		a.clSetor(escopo)+`
		GROUP BY d.id ORDER BY 2 DESC`, append([]any{de, ate}, a.argsArvore(escopo)...)...)
	if err == nil {
		for rows.Next() {
			var r DestinoStat
			if rows.Scan(&r.Destino, &r.Quantidade) == nil {
				b.PorDestino = append(b.PorDestino, r)
			}
		}
		rows.Close()
	}

	ftP2 := a.filtroArvore(escopo, "f2")
	rows, err = a.st.db.Query(`
		SELECT p.id, p.nome_guerra, COALESCE(s.nome,'Sem setor'),
		       COALESCE(SUM(pr.situacao IN ('presente','atraso')),0),
		       COALESCE(SUM(pr.situacao='atraso'),0),
		       COALESCE(SUM(pr.situacao='falta'),0),
		       COALESCE(SUM(pr.situacao='justificada'),0),
		       COUNT(pr.id)
		FROM pessoas p
		LEFT JOIN setores s ON s.id = p.setor_id
		/* FIX S4-P1 (verif5): escopo filtrado DENTRO do join de presencas —
		   pessoa ativa sem lançamento no período permanece na lista (zeros),
		   e lançamentos fora da árvore não contam (COUNT(pr.id) só vê pr
		   já filtrado; o LEFT JOIN f direto contava linha com f NULL). */
		LEFT JOIN presencas pr ON pr.pessoa_id = p.id
		       AND pr.conferencia_id IN (
		           SELECT f2.id FROM conferencias f2
		           WHERE f2.status='fechada' AND f2.data BETWEEN ? AND ?`+
		ftP2.clause+`
		       )
		WHERE p.status='ativo'`+a.filtroArvore(escopo, "p").clause+`
		GROUP BY p.id ORDER BY 7 DESC, 6 DESC, p.nome_guerra`,
		append(append([]any{de, ate}, ftP2.args...), a.filtroArvore(escopo, "p").args...)...)
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
		WHERE f.data BETWEEN ? AND ?`+
		a.clSetor(escopo)+`
		GROUP BY f.id ORDER BY f.data DESC`, append([]any{de, ate}, a.argsArvore(escopo)...)...)
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

// filtroGrupoSQL/Args: cláusula de escopo por grupo para queries com alias "a"
// (revisão TAKEDA/SHORYU — agregados não podem cruzar grupos).
func filtroGrupoSQL(escopo int64, alias string) string {
	if escopo <= 0 {
		return ""
	}
	return ` AND ` + alias + `.grupo_id = ?`
}
func filtroGrupoArgs(escopo int64) []any {
	if escopo <= 0 {
		return nil
	}
	return []any{escopo}
}

// gruposSubordinadosAtivos: TODOS os descendentes (transitivo) com vínculo bilateral ativo.
func (a *App) gruposSubordinadosAtivos(gid int64) []int64 {
	visitado := map[int64]bool{gid: true}
	fila := []int64{gid}
	out := []int64{}
	for len(fila) > 0 {
		g := fila[0]
		fila = fila[1:]
		rows, err := a.st.db.Query(`SELECT subordinado_id FROM grupo_vinculos
			WHERE superior_id = ? AND criado_por_superior = 1 AND criado_por_subordinado = 1`, g)
		if err != nil {
			break
		}
		for rows.Next() {
			var s int64
			if rows.Scan(&s) == nil && !visitado[s] {
				visitado[s] = true
				out = append(out, s)
				fila = append(fila, s)
			}
		}
		rows.Close()
	}
	return out
}

// filtroArvore: escopo do grupo + descendentes (relatórios sobem pela hierarquia).
// FIX S4: retorna struct nomeada (cláusula + args) — evita erro de compilação
// "multiple-value in single-value context" em uso inline dentro de concatenação.
type treeFilter struct {
	clause string
	args   []any
}

func (a *App) filtroArvore(escopo int64, alias string) treeFilter {
	// FIX S4 (verif5): guarda AQUI — chamadores diretos (query de pessoas do
	// bundle) não podem gerar "grupo_id IN (0)" no escopo global (esvaziava a lista).
	if escopo <= 0 {
		return treeFilter{}
	}
	ids := append([]int64{escopo}, a.gruposSubordinadosAtivos(escopo)...)
	ph := strings.TrimSuffix(strings.Repeat("?,", len(ids)), ",")
	args := make([]any, len(ids))
	for i, id := range ids {
		args[i] = id
	}
	return treeFilter{clause: ` AND ` + alias + `.grupo_id IN (` + ph + `)`, args: args}
}

// clSetor/argsArvore: cláusula+args da ÁRVORE do escopo (grupo + subordinados ativos).
// No escopo do bundle, "f" cobre conferências; para pessoas (alias p) a regra é a mesma
// coluna grupo_id, então a cláusula gerada é idêntica em forma.
func (a *App) clSetor(escopo int64) string {
	if escopo <= 0 {
		return ""
	}
	return a.filtroArvore(escopo, "f").clause
}
func (a *App) argsArvore(escopo int64) []any {
	if escopo <= 0 {
		return nil
	}
	return a.filtroArvore(escopo, "f").args
}

// escopoRelatorio: respeita ?grupo= — admin recorta qualquer grupo; gerente,
// só o próprio ou descendentes.
// FIX S4-P1 (verif5): pedido de grupo FORA da árvore agora responde 403 com
// escopo_aplicado — antes caía em fallback silencioso 200 e mascarava erro.
func (a *App) escopoRelatorio(r *http.Request, u *Usuario) (int64, bool) {
	esc := escopoDoUsuario(u)
	q := r.URL.Query().Get("grupo")
	if esc <= 0 {
		if q == "" {
			return esc, true
		}
		gid, err := strconv.ParseInt(q, 10, 64)
		if err != nil {
			return esc, true
		}
		if esc == 0 { // admin: recorte livre
			return gid, true
		}
		return esc, true
	}
	if q == "" {
		return esc, true
	}
	gid, err := strconv.ParseInt(q, 10, 64)
	if err != nil {
		return esc, true
	}
	if gid == esc || int64Contem(a.gruposSubordinadosAtivos(esc), gid) {
		return gid, true
	}
	return esc, false
}

func int64Contem(lista []int64, v int64) bool {
	for _, x := range lista {
		if x == v {
			return true
		}
	}
	return false
}

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
	u := usuarioDoCtx(r)
	esc, ok := a.escopoRelatorio(r, u)
	if !ok {
		jsonErro(w, http.StatusForbidden, "grupo fora do seu escopo")
		return
	}
	jsonOK(w, a.montarBundle(de, ate, esc))
}

func (a *App) hRelatorioJSON(w http.ResponseWriter, r *http.Request) {
	de, ate := a.periodoPadrao(r)
	u := usuarioDoCtx(r)
	esc, ok := a.escopoRelatorio(r, u)
	if !ok {
		jsonErro(w, http.StatusForbidden, "grupo fora do seu escopo")
		return
	}
	jsonOK(w, a.montarBundle(de, ate, esc))
}

func (a *App) hRelatorioPDF(w http.ResponseWriter, r *http.Request) {
	de, ate := a.periodoPadrao(r)
	u := usuarioDoCtx(r)
	esc, ok := a.escopoRelatorio(r, u)
	if !ok {
		jsonErro(w, http.StatusForbidden, "grupo fora do seu escopo")
		return
	}
	pdf, err := a.gerarRelatorioPDF(a.montarBundle(de, ate, esc))
	if err != nil {
		jsonErro(w, http.StatusInternalServerError, "falha ao gerar PDF: "+err.Error())
		return
	}
	a.st.Auditoria(&u.ID, "exportar", "relatorio", nil, de+" a "+ate, ipDe(r))
	w.Header().Set("Content-Type", "application/pdf")
	w.Header().Set("Content-Disposition",
		fmt.Sprintf("inline; filename=SCI_relatorio_%s_%s.pdf", de, ate))
	_, _ = w.Write(pdf)
}

// ---------- exportação CSV (portabilidade — o dado nunca fica preso) ----------

func (a *App) hExportCSV(w http.ResponseWriter, r *http.Request) {
	t := r.PathValue("t")
	escopo := escopoDoUsuario(usuarioDoCtx(r))
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
			LEFT JOIN funcoes fu ON fu.id=p.funcao_id
			WHERE 1=1`+filtroGrupoSQL(escopo, "p")+` ORDER BY p.id`,
			filtroGrupoArgs(escopo)...)
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
			WHERE 1=1`+filtroGrupoSQL(escopo, "f")+` ORDER BY f.data, p.nome_guerra`,
			filtroGrupoArgs(escopo)...)
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
			JOIN conferencia_tipos ft ON ft.id=f.tipo_id
			WHERE 1=1`+filtroGrupoSQL(escopo, "f")+` ORDER BY f.data`,
			filtroGrupoArgs(escopo)...)
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

// gruposSuperioresAtivos: ids dos grupos superiores com vínculo BILATERAL ativo.
func (a *App) gruposSuperioresAtivos(gid int64) []int64 {
	rows, err := a.st.db.Query(`SELECT superior_id FROM grupo_vinculos
		WHERE subordinado_id = ? AND criado_por_superior = 1 AND criado_por_subordinado = 1`, gid)
	if err != nil {
		return nil
	}
	defer rows.Close()
	var out []int64
	for rows.Next() {
		var id int64
		if rows.Scan(&id) == nil {
			out = append(out, id)
		}
	}
	return out
}

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
	q := `SELECT id, nome` + extra + `, ativo FROM ` + t
	var args []any
	if esc := escopoDoUsuario(usuarioDoCtx(r)); esc > 0 {
		// escopo + HERANÇA (ordem Tenente 28/09 noite): grupo vê os globais (NULL),
		// os do PRÓPRIO grupo e os dos grupos SUPERIORES com vínculo ativo
		ids := append([]int64{esc}, a.gruposSuperioresAtivos(esc)...)
		ph := strings.TrimSuffix(strings.Repeat("?,", len(ids)), ",")
		args = make([]any, len(ids))
		for i, id := range ids {
			args[i] = id
		}
		q += ` WHERE grupo_id IS NULL OR grupo_id IN (` + ph + `)`
	}
	q += ` ORDER BY ativo DESC, nome`
	rows, err := a.st.db.Query(q, args...)
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
	// R4 (v9.3): gestão de catálogos é EXCLUSIVA do GERENTE (operador usa, não gerencia)
	u := usuarioDoCtx(r)
	if u == nil || u.Papel != "gerente" {
		jsonErro(w, http.StatusForbidden, "gestão de catálogos é exclusiva do gerente")
		return
	}
	// catálogo dono: sempre o grupo do gerente (v9.3 — admin não gerencia catálogo)
	var grupoID any
	if u.GrupoID != nil {
		grupoID = *u.GrupoID
	}
	var q string
	switch t {
	case "setores":
		q = `INSERT INTO setores (nome, sigla, grupo_id) VALUES (?, NULLIF(?,''), ?)`
	case "tags":
		q = `INSERT INTO tags (nome, cor, grupo_id) VALUES (?, NULLIF(?,''), ?)`
	default:
		q = `INSERT INTO ` + t + ` (nome, grupo_id) VALUES (?,?)`
	}
	var res interface {
		LastInsertId() (int64, error)
	}
	if t == "setores" || t == "tags" {
		res, err = a.st.db.Exec(q, nome, map[bool]string{true: req.Sigla, false: req.Cor}[t == "setores"], grupoID)
	} else {
		res, err = a.st.db.Exec(q, nome, grupoID)
	}
	if err != nil {
		jsonErro(w, http.StatusBadRequest, "não inserido (duplicado?): "+err.Error())
		return
	}
	id, _ := res.LastInsertId()
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
	// R4 (v9.3): gestão de catálogos é EXCLUSIVA do GERENTE
	if u == nil || u.Papel != "gerente" {
		jsonErro(w, http.StatusForbidden, "gestão de catálogos é exclusiva do gerente")
		return
	}
	if esc := escopoDoUsuario(u); esc > 0 {
		var donoGrupo int64
		if err := a.st.db.QueryRow(`SELECT COALESCE(grupo_id,0) FROM `+t+` WHERE id = ?`, id).Scan(&donoGrupo); err != nil || donoGrupo != esc {
			jsonErro(w, http.StatusForbidden, "catálogo de outro grupo ou global (só o admin)")
			return
		}
	}
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

func (a *App) hPessoasList(w http.ResponseWriter, r *http.Request) {
	u := usuarioDoCtx(r)
	jsonOK(w, map[string]any{"pessoas": a.pessoasTodas(escopoDoUsuario(u))})
}

func (a *App) pessoasTodas(escopo int64) []map[string]any {
	q := `
		SELECT p.id, p.nome_guerra, p.nome_completo, p.setor_id, p.funcao_id, p.status,
		       COALESCE(s.nome,''), COALESCE(fu.nome,''), COALESCE(g.nome,'')
		FROM pessoas p
		LEFT JOIN setores s ON s.id = p.setor_id
		LEFT JOIN funcoes fu ON fu.id = p.funcao_id
		LEFT JOIN grupos g ON g.id = p.grupo_id`
	var rows *sql.Rows
	var err error
	if escopo > 0 {
		rows, err = a.st.db.Query(q+` WHERE p.grupo_id = ?
			ORDER BY p.status, COALESCE(s.nome,''), p.nome_guerra`, escopo)
	} else {
		rows, err = a.st.db.Query(q + `
			ORDER BY p.status, COALESCE(s.nome,''), p.nome_guerra`)
	}
	if err != nil {
		return []map[string]any{}
	}
	defer rows.Close()
	out := []map[string]any{}
	for rows.Next() {
		var id int64
		var setorID, funcaoID *int64 // NULL = sem setor/função (não descartar a linha!)
		var ng, nc, status, setor, funcao, grupo string
		if rows.Scan(&id, &ng, &nc, &setorID, &funcaoID, &status, &setor, &funcao, &grupo) == nil {
			out = append(out, map[string]any{
				"id": id, "nome_guerra": ng, "nome_completo": nc,
				"setor_id": setorID, "funcao_id": funcaoID,
				"setor": setor, "funcao": funcao, "status": status, "grupo": grupo,
			})
		}
	}
	return out
}

func (a *App) hPessoasAdd(w http.ResponseWriter, r *http.Request) {
	var req struct {
		NomeGuerra   string `json:"nome_guerra"`
		NomeCompleto string `json:"nome_completo"`
		SetorID      *int64 `json:"setor_id"`
		FuncaoID     *int64 `json:"funcao_id"`
		Status       string `json:"status"`
		GrupoID      *int64 `json:"grupo_id"`
	}
	if err := decodificar(r, &req); err != nil || req.NomeGuerra == "" || req.NomeCompleto == "" {
		jsonErro(w, http.StatusBadRequest, "nome de guerra e nome completo obrigatórios")
		return
	}
	if req.Status == "" {
		req.Status = "ativo"
	}
	u := usuarioDoCtx(r)
	// REGRA (28/09): admin e gerente cadastram; gerente SEMPRE no próprio grupo
	var grupoID *int64
	if u.Papel == "admin" {
		grupoID = req.GrupoID // admin escolhe o grupo (ou NULL = sem grupo)
	} else if u.Papel == "gerente" {
		if u.GrupoID == nil {
			jsonErro(w, http.StatusForbidden, "gerente sem grupo definido")
			return
		}
		grupoID = u.GrupoID
	} else {
		jsonErro(w, http.StatusForbidden, "somente admin e gerente cadastram pessoal")
		return
	}
	res, err := a.st.db.Exec(
		`INSERT INTO pessoas (nome_guerra, nome_completo, setor_id, funcao_id, grupo_id, status) VALUES (?,?,?,?,?,?)`,
		req.NomeGuerra, req.NomeCompleto, req.SetorID, req.FuncaoID, grupoID, req.Status)
	if err != nil {
		jsonErro(w, http.StatusBadRequest, err.Error())
		return
	}
	id, _ := res.LastInsertId()
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
		NomeGuerra   string `json:"nome_guerra"`
		NomeCompleto string `json:"nome_completo"`
		Status       string `json:"status"`
		SetorID      *int64 `json:"setor_id"`
		FuncaoID     *int64 `json:"funcao_id"`
		GrupoID      *int64 `json:"grupo_id"` // admin pode mover pessoa de grupo
	}
	if err = decodificar(r, &req); err != nil {
		jsonErro(w, http.StatusBadRequest, "JSON inválido")
		return
	}
	if req.Status == "" {
		req.Status = "ativo"
	}
	u := usuarioDoCtx(r)
	// REGRA (28/09): gerente só edita pessoal DO PRÓPRIO grupo (admin edita tudo)
	if esc := escopoDoUsuario(u); esc > 0 {
		var gid *int64
		if err = a.st.db.QueryRow(`SELECT grupo_id FROM pessoas WHERE id = ?`, id).Scan(&gid); err != nil {
			jsonErro(w, http.StatusNotFound, "pessoa inexistente")
			return
		}
		if gid == nil || *gid != esc {
			jsonErro(w, http.StatusForbidden, "pessoa de outro grupo")
			return
		}
	}
	// admin pode mover a pessoa de grupo na edição; gerente nunca altera grupo
	if u.Papel == "admin" {
		if _, err = a.st.db.Exec(`UPDATE pessoas SET nome_guerra=?, nome_completo=?, setor_id=?, funcao_id=?, status=?, grupo_id=?,
		 atualizado_em=strftime('%Y-%m-%dT%H:%M:%fZ','now') WHERE id=?`,
			req.NomeGuerra, req.NomeCompleto, req.SetorID, req.FuncaoID, req.Status, req.GrupoID, id); err != nil {
			jsonErro(w, http.StatusBadRequest, err.Error())
			return
		}
	} else {
		if _, err = a.st.db.Exec(
			`UPDATE pessoas SET nome_guerra=?, nome_completo=?, setor_id=?, funcao_id=?, status=?,
		 atualizado_em=strftime('%Y-%m-%dT%H:%M:%fZ','now') WHERE id=?`,
			req.NomeGuerra, req.NomeCompleto, req.SetorID, req.FuncaoID, req.Status, id); err != nil {
			jsonErro(w, http.StatusBadRequest, err.Error())
			return
		}
	}
	a.st.Auditoria(&u.ID, "alterar", "pessoas", &id, req.NomeGuerra, ipDe(r))
	jsonOK(w, map[string]bool{"ok": true})
}

// ---------- usuários ----------

func (a *App) hUsuariosList(w http.ResponseWriter, r *http.Request) {
	u := usuarioDoCtx(r)
	escopo := escopoDoUsuario(u)
	rows, err := a.st.db.Query(
		`SELECT id, login, papel, pessoa_id, COALESCE(grupo_id,0), ativo, criado_em, senhas
		 FROM usuarios ORDER BY id`)
	if err != nil {
		jsonErro(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer rows.Close()
	out := []map[string]any{}
	for rows.Next() {
		var id, grupoID int64
		var login, papel, criado, senhas string
		var pessoaID *int64
		var ativo int
		if rows.Scan(&id, &login, &papel, &pessoaID, &grupoID, &ativo, &criado, &senhas) == nil {
			// operador só vê contas do PRÓPRIO grupo (admin/gerente gerenciam os seus)
			if escopo > 0 && int64(escopo) != grupoID {
				continue
			}
			out = append(out, map[string]any{
				"id": id, "login": login, "papel": papel, "pessoa_id": pessoaID,
				"grupo_id": grupoID, "ativo": ativo == 1, "criado_em": criado,
				"senhas": json.RawMessage(senhas),
			})
		}
	}
	jsonOK(w, out)
}

// ---------- grupos e escopo (fase GRUPOS — ordem Tenente 28/09) ----------

// escopoDoUsuario: 0 = vê tudo (APENAS admin); -1 = conta SEM grupo (sem acesso a dados
// de grupo — revisão TAKEDA: devolver 0 aqui era escala de privilégio silenciosa).
func escopoDoUsuario(u *Usuario) int64 {
	if u == nil {
		return -1
	}
	if u.Papel == "admin" {
		return 0
	}
	if u.GrupoID == nil {
		return -1 // gerente/operador sem grupo = sem acesso a dados de grupo
	}
	return *u.GrupoID
}

// podeAdministrar: admin sempre; gerente dentro do próprio grupo.
func podeAdministrar(u *Usuario) bool {
	return u != nil && (u.Papel == "admin" || u.Papel == "gerente")
}

// hGruposList / hGruposAdd / hGruposDel: gestão de grupos (só admin cria).
func (a *App) hGruposList(w http.ResponseWriter, r *http.Request) {
	escopo := escopoDoUsuario(usuarioDoCtx(r))
	jsonOK(w, a.gruposComCodigo(escopo))
}

// ---------- hierarquia de grupos (ordem Tenente, 28/09 noite) ----------

// hVinculoAdd: vínculo BILATERAL — superior informa código do subordinado e vice-versa.
// O vínculo só existe quando AMBOS registraram o par (consentimento mútuo).
func (a *App) hVinculoAdd(w http.ResponseWriter, r *http.Request) {
	u := usuarioDoCtx(r)
	var req struct {
		Codigo string `json:"codigo"`
		Lado   string `json:"lado"`  // "superior" = declaro MEU código a um inferior; "subordinado" = o contrário
		Outro  string `json:"outro"` // admin: vincula DOIS códigos de uma vez (fecha o bilateral)
	}
	if err := decodificar(r, &req); err != nil || req.Codigo == "" || (req.Lado != "superior" && req.Lado != "subordinado") {
		jsonErro(w, http.StatusBadRequest, "codigo e lado (superior|subordinado) obrigatórios")
		return
	}
	// ADMIN vincula diretamente dois grupos pelos códigos (ordem Tenente 28/09 noite)
	if u.Papel == "admin" && strings.TrimSpace(req.Outro) != "" {
		var idA, idB int64
		if a.st.db.QueryRow(`SELECT id FROM grupos WHERE codigo = ?`, strings.ToUpper(strings.TrimSpace(req.Codigo))).Scan(&idA) != nil ||
			a.st.db.QueryRow(`SELECT id FROM grupos WHERE codigo = ?`, strings.ToUpper(strings.TrimSpace(req.Outro))).Scan(&idB) != nil {
			jsonErro(w, http.StatusNotFound, "código de grupo inexistente")
			return
		}
		if idA == idB {
			jsonErro(w, http.StatusBadRequest, "um grupo não se vincula a si mesmo")
			return
		}
		if _, err := a.st.db.Exec(`INSERT INTO grupo_vinculos (superior_id, subordinado_id, criado_por_superior, criado_por_subordinado)
			VALUES (?, ?, 1, 1) ON CONFLICT(superior_id, subordinado_id)
			DO UPDATE SET criado_por_superior = 1, criado_por_subordinado = 1`, idA, idB); err != nil {
			jsonErro(w, http.StatusInternalServerError, err.Error())
			return
		}
		a.st.Auditoria(&u.ID, "vincular", "grupo_vinculos", nil,
			fmt.Sprintf("admin: sup=%d sub=%d", idA, idB), ipDe(r))
		jsonOK(w, map[string]any{"vinculado": true})
		return
	}
	if u.GrupoID == nil {
		jsonErro(w, http.StatusForbidden, "conta sem grupo não registra vínculos")
		return
	}
	var outroID int64
	err := a.st.db.QueryRow(`SELECT id FROM grupos WHERE codigo = ?`, strings.ToUpper(strings.TrimSpace(req.Codigo))).Scan(&outroID)
	if err != nil {
		jsonErro(w, http.StatusNotFound, "código de grupo inexistente")
		return
	}
	if outroID == *u.GrupoID {
		jsonErro(w, http.StatusBadRequest, "um grupo não se vincula a si mesmo")
		return
	}
	var supID, subID int64
	if req.Lado == "superior" {
		supID, subID = *u.GrupoID, outroID
	} else {
		supID, subID = outroID, *u.GrupoID
	}
	// proteção contra ciclo imediato A->B->A (v1): se o "subordinado" já é superior do solicitante
	var ciclo int
	_ = a.st.db.QueryRow(`SELECT COUNT(*) FROM grupo_vinculos WHERE superior_id = ? AND subordinado_id = ?`,
		subID, supID).Scan(&ciclo)
	if ciclo > 0 {
		jsonErro(w, http.StatusConflict, "vínculo reverso já existe — não é possível inverter")
		return
	}
	_, err = a.st.db.Exec(`INSERT INTO grupo_vinculos
		(superior_id, subordinado_id, criado_por_superior, criado_por_subordinado)
		VALUES (?,?,?,?)
		ON CONFLICT(superior_id, subordinado_id) DO UPDATE SET
		  criado_por_superior = criado_por_superior | ?,
		  criado_por_subordinado = criado_por_subordinado | ?`,
		supID, subID,
		map[bool]int{true: 1, false: 0}[req.Lado == "superior"],
		map[bool]int{true: 1, false: 0}[req.Lado == "subordinado"],
		map[bool]int{true: 1, false: 0}[req.Lado == "superior"],
		map[bool]int{true: 1, false: 0}[req.Lado == "subordinado"])
	if err != nil {
		jsonErro(w, http.StatusInternalServerError, err.Error())
		return
	}
	var pS, pSb int
	_ = a.st.db.QueryRow(`SELECT criado_por_superior, criado_por_subordinado FROM grupo_vinculos
		WHERE superior_id = ? AND subordinado_id = ?`, supID, subID).Scan(&pS, &pSb)
	a.st.Auditoria(&u.ID, "vincular", "grupo_vinculos", nil,
		fmt.Sprintf("sup=%d sub=%d completo=%v", supID, subID, pS == 1 && pSb == 1), ipDe(r))
	if pS == 1 && pSb == 1 {
		jsonOK(w, map[string]any{"vinculado": true})
	} else {
		jsonOK(w, map[string]any{"vinculado": false,
			"aguardando": "aguardando a confirmação do outro grupo"})
	}
}

// hVinculoList: pendentes e ativos do MEU grupo.
func (a *App) hVinculoList(w http.ResponseWriter, r *http.Request) {
	u := usuarioDoCtx(r)
	if u.GrupoID == nil {
		jsonOK(w, map[string]any{"pendentes": []map[string]any{}, "ativos": []map[string]any{}})
		return
	}
	gid := *u.GrupoID
	rows, err := a.st.db.Query(`
		SELECT gv.superior_id, gs.nome, gv.subordinado_id, gsub.nome,
		       gv.criado_por_superior, gv.criado_por_subordinado
		FROM grupo_vinculos gv
		JOIN grupos gs ON gs.id = gv.superior_id
		JOIN grupos gsub ON gsub.id = gv.subordinado_id
		WHERE gv.superior_id = ? OR gv.subordinado_id = ?`, gid, gid)
	if err != nil {
		jsonErro(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer rows.Close()
	pend, ativos := []map[string]any{}, []map[string]any{}
	for rows.Next() {
		var supID, subID, pS, pSb int64
		var supNome, subNome string
		if rows.Scan(&supID, &supNome, &subID, &subNome, &pS, &pSb) == nil {
			role := "subordinado"
			if supID == gid {
				role = "superior"
			}
			item := map[string]any{"superior": supNome, "subordinado": subNome, "meu_papel": role}
			if pS == 1 && pSb == 1 {
				ativos = append(ativos, item)
			} else {
				pend = append(pend, item)
			}
		}
	}
	jsonOK(w, map[string]any{"pendentes": pend, "ativos": ativos})
}

// hPerfilGet: dados da própria conta (aba Meu usuário).
func (a *App) hPerfilGet(w http.ResponseWriter, r *http.Request) {
	u := usuarioDoCtx(r)
	var grupo string
	if u.GrupoID != nil {
		_ = a.st.db.QueryRow(`SELECT nome FROM grupos WHERE id = ?`, *u.GrupoID).Scan(&grupo)
	}
	var setor, funcao string
	if u.SetorID != nil {
		_ = a.st.db.QueryRow(`SELECT nome FROM setores WHERE id = ?`, *u.SetorID).Scan(&setor)
	}
	if u.FuncaoID != nil {
		_ = a.st.db.QueryRow(`SELECT nome FROM funcoes WHERE id = ?`, *u.FuncaoID).Scan(&funcao)
	}
	jsonOK(w, map[string]any{"usuario": u, "grupo": grupo, "setor": setor, "funcao": funcao})
}

// hPerfilSet: o próprio usuário atualiza seu perfil (função/setor herdam do grupo;
// aqui o usuário apenas mantém nome_guerra e nome_completo).
func (a *App) hPerfilSet(w http.ResponseWriter, r *http.Request) {
	var req struct {
		NomeGuerra   string `json:"nome_guerra"`
		NomeCompleto string `json:"nome_completo"`
	}
	if err := decodificar(r, &req); err != nil || req.NomeGuerra == "" || req.NomeCompleto == "" {
		jsonErro(w, http.StatusBadRequest, "nome de guerra e nome completo obrigatórios")
		return
	}
	u := usuarioDoCtx(r)
	if _, err := a.st.db.Exec(`UPDATE usuarios SET nome_guerra = ?, nome_completo = ? WHERE id = ?`,
		req.NomeGuerra, req.NomeCompleto, u.ID); err != nil {
		jsonErro(w, http.StatusInternalServerError, err.Error())
		return
	}
	a.st.Auditoria(&u.ID, "editar", "usuarios", &u.ID, "perfil próprio", ipDe(r))
	jsonOK(w, map[string]bool{"ok": true})
}

// hMoverConta: admin MOVE gerente/operador entre grupos (conta = credencial; função vem do grupo).
func (a *App) hMoverConta(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		jsonErro(w, http.StatusBadRequest, "id inválido")
		return
	}
	var req struct {
		GrupoID *int64 `json:"grupo_id"`
	}
	if err := decodificar(r, &req); err != nil {
		jsonErro(w, http.StatusBadRequest, "corpo inválido")
		return
	}
	_, err = a.st.db.Exec(`UPDATE usuarios SET grupo_id = ? WHERE id = ?`, req.GrupoID, id)
	if err != nil {
		jsonErro(w, http.StatusInternalServerError, err.Error())
		return
	}
	u := usuarioDoCtx(r)
	a.st.Auditoria(&u.ID, "mover", "usuarios", &id,
		fmt.Sprintf("novo grupo: %v", req.GrupoID), ipDe(r))
	jsonOK(w, map[string]bool{"ok": true})
}

// hGruposAdd movido para a seção v9.3 (R7: exige gerente no ato).

// gruposComCodigo: lista com código + vínculos de hierarquia (nomes resolvidos).
func (a *App) gruposComCodigo(escopo int64) []map[string]any {
	rows, err := a.st.db.Query(`
		SELECT g.id, g.nome, COALESCE(g.codigo,''), g.criado_em,
		       (SELECT COUNT(*) FROM usuarios u WHERE u.grupo_id = g.id) AS contas,
		       (SELECT COUNT(*) FROM pessoas p WHERE p.grupo_id = g.id) AS efetivo,
		       (SELECT GROUP_CONCAT(g2.nome) FROM grupo_vinculos gv
		          JOIN grupos g2 ON g2.id = gv.subordinado_id
		          WHERE gv.superior_id = g.id AND gv.criado_por_superior = 1 AND gv.criado_por_subordinado = 1),
		       (SELECT GROUP_CONCAT(g3.nome) FROM grupo_vinculos gv2
		          JOIN grupos g3 ON g3.id = gv2.superior_id
		          WHERE gv2.subordinado_id = g.id AND gv2.criado_por_superior = 1 AND gv2.criado_por_subordinado = 1),
		       (SELECT GROUP_CONCAT(g2.id) FROM grupo_vinculos gv
		          JOIN grupos g2 ON g2.id = gv.subordinado_id
		          WHERE gv.superior_id = g.id AND gv.criado_por_superior = 1 AND gv.criado_por_subordinado = 1),
		       (SELECT GROUP_CONCAT(g3.id) FROM grupo_vinculos gv2
		          JOIN grupos g3 ON g3.id = gv2.superior_id
		          WHERE gv2.subordinado_id = g.id AND gv2.criado_por_superior = 1 AND gv2.criado_por_subordinado = 1)
		FROM grupos g ORDER BY g.nome`)
	if err != nil {
		return []map[string]any{}
	}
	defer rows.Close()
	out := []map[string]any{}
	for rows.Next() {
		var id int64
		var nome, cod, criado string
		var contas, efetivo int
		var sub, sup, subIDs, supIDs sql.NullString
		if rows.Scan(&id, &nome, &cod, &criado, &contas, &efetivo, &sub, &sup, &subIDs, &supIDs) == nil {
			if escopo > 0 && id != escopo {
				// gerente/operador só enxerga o próprio grupo (sub/superiores chegam pelas colunas)
				continue
			}
			out = append(out, map[string]any{
				"id": id, "nome": nome, "codigo": cod, "criado_em": criado,
				"contas": contas, "efetivo": efetivo,
				"subordinados": nilToSlice(sub), "superiores": nilToSlice(sup),
				"subordinados_ids": nilToInts(subIDs), "superiores_ids": nilToInts(supIDs),
			})
		}
	}
	return out
}

func nilToSlice(ns sql.NullString) []string {
	if !ns.Valid || ns.String == "" {
		return []string{}
	}
	return strings.Split(ns.String, ",")
}

func nilToInts(ns sql.NullString) []int64 {
	out := []int64{}
	for _, p := range nilToSlice(ns) {
		if v, err := strconv.ParseInt(strings.TrimSpace(p), 10, 64); err == nil {
			out = append(out, v)
		}
	}
	return out
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
	// IDOR (revisão TAKEDA/SHORYU): comentário só na conferência DO PRÓPRIO grupo
	if esc := escopoDoUsuario(u); esc > 0 {
		var gid *int64
		if err := a.st.db.QueryRow(`SELECT grupo_id FROM conferencias WHERE id = ?`, req.ConferenciaID).Scan(&gid); err != nil || gid == nil || *gid != esc {
			jsonErro(w, http.StatusForbidden, "conferência de outro grupo")
			return
		}
	}
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
	// IDOR: grupo só lista comentários da própria conferência
	u := usuarioDoCtx(r)
	if esc := escopoDoUsuario(u); esc > 0 {
		var gid *int64
		if qerr := a.st.db.QueryRow(`SELECT grupo_id FROM conferencias WHERE id = ?`, id).Scan(&gid); qerr != nil || gid == nil || *gid != esc {
			jsonErro(w, http.StatusForbidden, "conferência de outro grupo")
			return
		}
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
	case "admin", "gerente", "operador":
	default:
		jsonErro(w, http.StatusBadRequest, "papel inválido (admin | gerente | operador)")
		return
	}
	// hierarquia de criação (v9.3):
	// - ADMIN é o ÚNICO que cria GERENTE (e admin)
	// - GERENTE cria OPERADOR, sempre no PRÓPRIO grupo
	// - OPERADOR COMUM não cria conta nenhuma
	if u.Papel == "operador" {
		jsonErro(w, http.StatusForbidden, "operador não cria contas")
		return
	}
	if u.Papel != "admin" {
		if papel != "operador" {
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

// ---------- v9.3 ----------

// hUsuarioExcluir (R6): admin exclui conta. Regras: nunca a si nem outro admin;
// gerente só se o grupo não ficar sem gerente; com FK (presencas.marcado_por,
// conferencias.criado_por, comentarios.operador_id) -> desativa (ativo=0) e responde
// {desativado:true}; sem FK -> DELETE físico.
func (a *App) hUsuarioExcluir(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		jsonErro(w, http.StatusBadRequest, "id inválido")
		return
	}
	eu := usuarioDoCtx(r)
	if id == eu.ID {
		jsonErro(w, http.StatusBadRequest, "não é possível excluir a própria conta")
		return
	}
	var papel string
	if err := a.st.db.QueryRow(`SELECT papel FROM usuarios WHERE id = ?`, id).Scan(&papel); err != nil {
		jsonErro(w, http.StatusNotFound, "usuário inexistente")
		return
	}
	if papel == "admin" {
		jsonErro(w, http.StatusForbidden, "conta admin não é excluída")
		return
	}
	if papel == "gerente" {
		var gid int64
		var temOutro int
		if err := a.st.db.QueryRow(`SELECT COALESCE(grupo_id,0) FROM usuarios WHERE id = ?`, id).Scan(&gid); err != nil {
			jsonErro(w, http.StatusNotFound, "usuário inexistente")
			return
		}
		if gid == 0 {
			jsonErro(w, http.StatusConflict, "gerente sem grupo — mova a conta antes de excluir")
			return
		}
		if err := a.st.db.QueryRow(`SELECT COUNT(*) FROM usuarios WHERE grupo_id = ? AND papel = 'gerente' AND id <> ? AND ativo = 1`,
			gid, id).Scan(&temOutro); err == nil && temOutro == 0 {
			jsonErro(w, http.StatusConflict, "excluiria o único gerente do grupo — troque o gerente antes")
			return
		}
	}
	var marcou, criou, comentou int
	_ = a.st.db.QueryRow(`SELECT
		(SELECT COUNT(*) FROM presencas WHERE marcado_por = ?),
		(SELECT COUNT(*) FROM conferencias WHERE criado_por = ?),
		(SELECT COUNT(*) FROM comentarios WHERE operador_id = ?)`, id, id, id).Scan(&marcou, &criou, &comentou)
	if marcou+criou+comentou > 0 {
		if _, err := a.st.db.Exec(`UPDATE usuarios SET ativo = 0 WHERE id = ?`, id); err != nil {
			jsonErro(w, http.StatusInternalServerError, err.Error())
			return
		}
		a.st.Auditoria(&eu.ID, "desativar", "usuarios", &id, "histórico preservado (FK)", ipDe(r))
		jsonOK(w, map[string]bool{"desativado": true})
		return
	}
	if _, err := a.st.db.Exec(`DELETE FROM usuarios WHERE id = ?`, id); err != nil {
		jsonErro(w, http.StatusInternalServerError, err.Error())
		return
	}
	a.st.Auditoria(&eu.ID, "excluir", "usuarios", &id, "", ipDe(r))
	jsonOK(w, map[string]bool{"ok": true})
}

// hGruposAdd (R7): criar grupo EXIGE gerente no ato (login+senha+nome de guerra).
// Transacional: grupo + conta gerente nascem juntos — nunca grupo vago.
func (a *App) hGruposAdd(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Nome       string `json:"nome"`
		Login      string `json:"login"`
		Senha      string `json:"senha"`
		NomeGuerra string `json:"nome_guerra"`
	}
	if err := decodificar(r, &req); err != nil || strings.TrimSpace(req.Nome) == "" {
		jsonErro(w, http.StatusBadRequest, "nome do grupo obrigatório")
		return
	}
	if strings.TrimSpace(req.Login) == "" || len(req.Senha) < 8 || strings.TrimSpace(req.NomeGuerra) == "" {
		jsonErro(w, http.StatusBadRequest, "grupo nasce com gerente: login, senha (mín. 8) e nome de guerra obrigatórios")
		return
	}
	req.Login = strings.ToLower(strings.TrimSpace(req.Login))
	var existeLogin int
	_ = a.st.db.QueryRow(`SELECT COUNT(*) FROM usuarios WHERE login = ?`, req.Login).Scan(&existeLogin)
	if existeLogin > 0 {
		jsonErro(w, http.StatusBadRequest, "login já existe")
		return
	}
	hash, err := hashSenha(req.Senha)
	if err != nil {
		jsonErro(w, http.StatusInternalServerError, err.Error())
		return
	}
	cod := gerarCodigoGrupo()
	for tenta := 0; tenta < 8; tenta++ {
		var existe int
		_ = a.st.db.QueryRow(`SELECT COUNT(*) FROM grupos WHERE codigo = ?`, cod).Scan(&existe)
		if existe == 0 {
			break
		}
		cod = gerarCodigoGrupo()
	}
	tx, err := a.st.db.Begin()
	if err != nil {
		jsonErro(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer tx.Rollback()
	res, err := tx.Exec(`INSERT INTO grupos (nome, codigo) VALUES (?,?)`, strings.TrimSpace(req.Nome), cod)
	if err != nil {
		jsonErro(w, http.StatusBadRequest, "grupo não criado (duplicado?): "+err.Error())
		return
	}
	gid, _ := res.LastInsertId()
	res2, err := tx.Exec(`INSERT INTO usuarios (login, senha_hash, papel, grupo_id, nome_guerra, nome_completo)
		VALUES (?,?,?,?,?,?)`,
		req.Login, hash, "gerente", gid, strings.TrimSpace(req.NomeGuerra), strings.TrimSpace(req.NomeGuerra))
	if err != nil {
		jsonErro(w, http.StatusBadRequest, "gerente não criado (login duplicado?): "+err.Error())
		return
	}
	uid, _ := res2.LastInsertId()
	if err = tx.Commit(); err != nil {
		jsonErro(w, http.StatusInternalServerError, err.Error())
		return
	}
	u := usuarioDoCtx(r)
	a.st.Auditoria(&u.ID, "criar", "grupos", &gid, req.Nome+" ["+cod+"] gerente="+req.Login, ipDe(r))
	jsonOK(w, map[string]any{"id": gid, "codigo": cod, "gerente_id": uid})
}

// hGrupoGerenteGet: nome do gerente do grupo (painel admin).
func (a *App) hGrupoGerenteGet(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		jsonErro(w, http.StatusBadRequest, "id inválido")
		return
	}
	var login string
	var tem int
	err = a.st.db.QueryRow(`SELECT login, 1 FROM usuarios WHERE grupo_id = ? AND papel = 'gerente' AND ativo = 1 LIMIT 1`, id).Scan(&login, &tem)
	if err != nil {
		jsonOK(w, map[string]any{"gerente": ""})
		return
	}
	jsonOK(w, map[string]any{"gerente": login})
}

// hGrupoTrocarGerente (R7): promove uma conta do grupo a gerente; o gerente
// anterior vira operador. Garante exatamente 1 gerente ativo por grupo.
func (a *App) hGrupoTrocarGerente(w http.ResponseWriter, r *http.Request) {
	gid, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		jsonErro(w, http.StatusBadRequest, "id inválido")
		return
	}
	var req struct {
		Login string `json:"login"`
	}
	if err = decodificar(r, &req); err != nil || strings.TrimSpace(req.Login) == "" {
		jsonErro(w, http.StatusBadRequest, "login do novo gerente obrigatório")
		return
	}
	req.Login = strings.ToLower(strings.TrimSpace(req.Login))
	var uid int64
	var papel string
	if err := a.st.db.QueryRow(`SELECT id, papel FROM usuarios WHERE login = ? AND grupo_id = ? AND ativo = 1`,
		req.Login, gid).Scan(&uid, &papel); err != nil {
		jsonErro(w, http.StatusNotFound, "conta não encontrada neste grupo")
		return
	}
	if papel == "gerente" {
		jsonErro(w, http.StatusBadRequest, "esta conta já é o gerente")
		return
	}
	var existeGer int
	_ = a.st.db.QueryRow(`SELECT COUNT(*) FROM usuarios WHERE grupo_id = ? AND papel = 'gerente' AND ativo = 1`, gid).Scan(&existeGer)
	if existeGer > 1 {
		jsonErro(w, http.StatusConflict, "grupo com mais de um gerente — corrija antes de trocar")
		return
	}
	tx, err := a.st.db.Begin()
	if err != nil {
		jsonErro(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer tx.Rollback()
	if existeGer == 1 {
		if _, err = tx.Exec(`UPDATE usuarios SET papel = 'operador' WHERE grupo_id = ? AND papel = 'gerente' AND ativo = 1`, gid); err != nil {
			jsonErro(w, http.StatusInternalServerError, err.Error())
			return
		}
	}
	if _, err = tx.Exec(`UPDATE usuarios SET papel = 'gerente' WHERE id = ?`, uid); err != nil {
		jsonErro(w, http.StatusInternalServerError, err.Error())
		return
	}
	if err = tx.Commit(); err != nil {
		jsonErro(w, http.StatusInternalServerError, err.Error())
		return
	}
	u := usuarioDoCtx(r)
	a.st.Auditoria(&u.ID, "trocar_gerente", "grupos", &gid, "novo="+req.Login, ipDe(r))
	jsonOK(w, map[string]any{"ok": true, "gerente": req.Login})
}

// hAdminVinculoSet (R8): subordinação direto do painel admin — grava o vínculo
// completo (criado_por_superior=1 e criado_por_subordinado=1), mesma semântica
// de gruposSubordinadosAtivos.
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

// hAdminVinculoRem (R8): remove a subordinação (painel admin).
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

// hBackupImportar (R9): recebe um .db, valida (magic SQLite + integrity_check +
// schema_migrations do arquivo <= schema_migrations do binário), grava backup de
// segurança (VACUUM INTO timestamp), faz o swap atômico (fechar pool → rename →
// reabrir → migrar). Falha em qualquer passo = rejeição SEM tocar o banco.
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
	// 4) swap atômico: fecha pool → remove alvo+wal/shm → rename → reabre+migra
	novo := a.st.arquivo + ".novo"
	_ = os.Remove(novo)
	if err = os.Rename(tmpNome, novo); err != nil {
		jsonErro(w, http.StatusInternalServerError, "preparar swap falhou — nada alterado: "+err.Error())
		return
	}
	// o defer os.Remove(tmpNome) vira no-op (arquivo renomeado)
	if err = a.st.ReabrirComArquivo(novo); err != nil {
		// rollback: volta o arquivo de segurança para o lugar
		_ = a.st.ReabrirComArquivo(a.st.arquivo)
		jsonErro(w, http.StatusInternalServerError, "swap falhou — banco reaberto no estado anterior: "+err.Error())
		return
	}
	_ = os.Remove(a.st.arquivo + "-wal")
	_ = os.Remove(a.st.arquivo + "-shm")
	u := usuarioDoCtx(r)
	a.st.Auditoria(&u.ID, "importar_backup", "banco", nil,
		fmt.Sprintf("schema=%d seguranca=%s", versaoArq, nomeSeg), ipDe(r))
	jsonOK(w, map[string]any{"ok": true, "schema": versaoArq, "seguranca": "backups/" + nomeSeg})
}
