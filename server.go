package main

// Servidor HTTP do SCI: rotas da API, backup definitivo, front embutido (SPA).

import (
	"bytes"
	"crypto/sha256"
	"database/sql"
	"embed"
	"encoding/base64"
	"encoding/csv"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log"
	"math"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
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
	a.iniciarWatchdogSLA()
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
	return json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(v)
}

// ---------- backup definitivo ----------

// backupMu serializa o par "checar colisão de nome → VACUUM INTO" dentro do
// processo. Sem ele, 2 backups no mesmo segundo (POST /api/backup simultâneos ou
// backup assíncrono de fechamento × manual) passam os dois pelo os.Stat e o segundo
// VACUUM INTO morre com "output file already exists" (F4, -race: reproduzido).
var backupMu sync.Mutex

// backupAgora cria backups/sci_YYYYMMDD_HHMMSS.db consistente (VACUUM INTO),
// com sha256 + linha no MANIFEST.txt. Todos os dados vivem em arquivo.
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
	m.HandleFunc("GET /api/ping", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"ok":true,"timestamp":"` + time.Now().UTC().Format(time.RFC3339) + `"}`))
	})
	m.HandleFunc("GET /api/health", a.hHealth)
	m.HandleFunc("POST /api/login", a.hLogin)
	m.Handle("POST /api/setup", a.auth(false, a.hAuthSetup))
	m.Handle("GET /api/me", a.auth(false, a.hMe))
	m.Handle("POST /api/logout", a.auth(false, a.hLogout))
	m.Handle("POST /api/senha", a.auth(false, a.hTrocarSenha))

	// Contexto de Sessão (Multi-Funções)
	m.Handle("POST /api/sessao/contexto", a.auth(false, a.hMudarContexto))

	// Gestão de Papéis Múltiplos
	m.Handle("POST /api/usuarios/{id}/papeis", a.auth(false, a.hUsuarioPapelAdd))
	m.Handle("DELETE /api/usuarios/{id}/papeis/{papel_id}", a.auth(false, a.hUsuarioPapelDel))

	// Módulo de Mensageria Interna por Função & Despachos (v1.2 Fase 2)
	m.Handle("GET /api/mensagens/inbox", a.auth(false, a.hMensagensInbox))
	m.Handle("GET /api/mensagens/enviadas", a.auth(false, a.hMensagensEnviadas))
	m.Handle("POST /api/mensagens", a.auth(false, a.hMensagensEnviar))
	m.Handle("POST /api/mensagens/{id}/ler", a.auth(false, a.hMensagensMarcarLida))
	m.Handle("POST /api/mensagens/{id}/excluir", a.auth(false, a.hMensagensExcluir))
	m.Handle("POST /api/mensagens/{id}/arquivar", a.auth(false, a.hMensagensArquivar))
	m.Handle("POST /api/mensagens/{id}/desarquivar", a.auth(false, a.hMensagensDesarquivar))
	m.Handle("GET /api/mensagens/pastas", a.auth(false, a.hMensagensPastasList))
	m.Handle("POST /api/mensagens/pastas", a.auth(false, a.hMensagensPastasAdd))
	m.Handle("DELETE /api/mensagens/pastas/{id}", a.auth(false, a.hMensagensPastasDel))
	m.Handle("POST /api/mensagens/{id}/mover-pasta", a.auth(false, a.hMensagensMoverPasta))
	m.Handle("GET /api/mensagens/{id}/thread", a.auth(false, a.hMensagensThread))
	m.Handle("POST /api/mensagens/{id}/responder", a.auth(false, a.hMensagensResponderThread))
	m.Handle("POST /api/mensagens/{id}/finalizar", a.auth(false, a.hMensagensFinalizar))
	m.Handle("GET /api/mensagens/contador", a.auth(false, a.hMensagensContador))
	m.Handle("GET /api/mensagens/destinatarios", a.auth(false, a.hMensagensDestinatarios))

	// Fórum & Mural de Avisos Gerenciais (v1.2 Fase 2)
	m.Handle("GET /api/avisos", a.auth(false, a.hAvisosList))
	m.Handle("POST /api/avisos", a.auth(false, a.hAvisosAdd))
	m.Handle("DELETE /api/avisos/{id}", a.auth(false, a.hAvisosDel))
	m.Handle("POST /api/avisos/{id}/ciente", a.auth(false, a.hAvisosCiente))
	m.Handle("POST /api/avisos/{id}/comentar", a.auth(false, a.hAvisosComentar))
	m.Handle("GET /api/avisos/{id}/detalhes", a.auth(false, a.hAvisosDetalhes))
	m.Handle("POST /api/avisos/{id}/repostar", a.auth(false, a.hAvisosRepostar))

	// Módulo de Drive Local (v1.2 Fase 3)
	m.Handle("GET /api/drive/itens", a.auth(false, a.hDriveItens))
	m.Handle("GET /api/drive/seletor", a.auth(false, a.hDriveSeletor))
	m.Handle("POST /api/drive/pastas", a.auth(false, a.hDrivePastasAdd))
	m.Handle("PATCH /api/drive/pastas/{id}", a.auth(false, a.hDrivePastasEdit))
	m.Handle("DELETE /api/drive/pastas/{id}", a.auth(false, a.hDrivePastasDel))
	m.Handle("POST /api/drive/upload", a.auth(false, a.hDriveUpload))
	m.Handle("GET /api/drive/download/{id}", a.auth(false, a.hDriveDownload))
	m.Handle("PATCH /api/drive/arquivos/{id}", a.auth(false, a.hDriveArquivosEdit))
	m.Handle("DELETE /api/drive/arquivos/{id}", a.auth(false, a.hDriveArquivosDel))
	m.Handle("POST /api/drive/compartilhar", a.auth(false, a.hDriveCompartilhar))
	m.Handle("GET /api/drive/compartilhamentos", a.auth(false, a.hDriveCompartilhamentosList))
	m.Handle("DELETE /api/drive/compartilhamentos/{id}", a.auth(false, a.hDriveCompartilhamentosDel))

	// Módulo de Calendário Operacional & Mesh (v1.2 Fase 3 / Nextcloud dynamic calendars)
	m.Handle("GET /api/calendarios", a.auth(false, a.hCalendariosList))
	m.Handle("POST /api/calendarios", a.auth(false, a.hCalendariosAdd))
	m.Handle("DELETE /api/calendarios/{id}", a.auth(false, a.hCalendariosDel))
	m.Handle("POST /api/calendarios/{id}/compartilhar", a.auth(false, a.hCalendariosCompartilhar))
	m.Handle("GET /api/calendarios/{id}/compartilhamentos", a.auth(false, a.hCalendariosCompartilhamentosList))
	m.Handle("GET /api/calendario/visao", a.auth(false, a.hCalendarioVisao))
	m.Handle("POST /api/calendario/eventos", a.auth(false, a.hCalendarioEventosSave))
	m.Handle("DELETE /api/calendario/eventos/{id}", a.auth(false, a.hCalendarioEventosDel))
	m.Handle("POST /api/calendario/compartilhar", a.auth(false, a.hCalendarioCompartilhar))
	m.Handle("GET /api/calendario/compartilhamentos", a.auth(false, a.hCalendarioCompartilhamentosList))
	m.Handle("DELETE /api/calendario/compartilhamentos/{id}", a.auth(false, a.hCalendarioCompartilhamentosDel))

	// abas de conferência/presença: GERENTE e OPERADOR apenas (R2/R11 — admin tem nav própria)
	confAuth := func(h http.HandlerFunc) http.Handler { return a.authPapeis([]string{"gerente", "operador"}, h) }
	confMarcarAuth := func(h http.HandlerFunc) http.Handler {
		return a.authPapeis([]string{"gerente", "operador", "chefe_setor"}, h)
	}

	// Arquivo de conferências + filtro de período (ordem Tenente 30/09):
	// FECHADAS × ARQUIVADAS; gerente arquiva, admin-only exclui arquivada.
	m.Handle("POST /api/conferencia/{id}/arquivar", confAuth(a.hConferenciaArquivar))
	m.Handle("DELETE /api/conferencia/arquivada/{id}", a.auth(true, a.hConferenciaExcluirArquivada))

	// Busca individual nos relatórios (ordem Tenente 30/09): registros por pessoa+período
	// e filtros complexos multi-seleção (TAG × período em OU).
	m.Handle("GET /api/relatorio/registros", a.auth(false, a.hRegistrosBusca))
	m.Handle("GET /api/relatorio/tags", a.auth(false, a.hTagsDisponiveis))
	m.Handle("GET /api/pessoas/{id}/ficha", a.auth(false, a.hPessoaFicha))
	m.Handle("GET /api/conferencia/hoje", a.auth(false, a.hConferenciaHoje))
	m.Handle("POST /api/conferencia/iniciar", confAuth(a.hConferenciaIniciar))
	m.Handle("POST /api/conferencia/fechar", confAuth(a.hConferenciaFechar))
	m.Handle("POST /api/conferencia/marcar", confMarcarAuth(a.hConferenciaMarcar))
	m.Handle("GET /api/conferencia/lista", a.auth(false, a.hConferenciaList))
	m.Handle("GET /api/conferencia/{id}", a.auth(false, a.hConferenciaGet))
	m.Handle("DELETE /api/conferencia/{id}", confAuth(a.hConferenciaDescartar))
	m.Handle("POST /api/conferencia/{id}/setor/{setor_id}/concluir", confMarcarAuth(a.hConferenciaSetorConcluir))
	m.Handle("POST /api/conferencia/{id}/setor/{setor_id}/reabrir", confMarcarAuth(a.hConferenciaSetorReabrir))
	m.Handle("GET /api/conferencia/{id}/relatorio.pdf", a.auth(false, a.hConferenciaPDF))

	m.Handle("GET /api/efetivo_atual", a.auth(false, a.hEfetivoAtual)) // todos os papéis: admin vê todos, demais veem o escopo
	m.Handle("GET /api/presenca/periodo", a.auth(false, a.hPresencaPeriodo))
	m.Handle("GET /api/conferencias", a.auth(false, a.hConferenciaList))

	m.Handle("GET /api/catalogo/{t}", a.auth(false, a.hCatalogoList))
	m.Handle("POST /api/catalogo/{t}", a.auth(false, a.hCatalogoAdd))
	m.Handle("DELETE /api/catalogo/{t}/{id}", a.auth(false, a.hCatalogoDel))
	m.Handle("PATCH /api/catalogo/{t}/{id}/pai", a.auth(false, a.hCatalogoReparentar))
	m.Handle("PATCH /api/catalogo/{t}/{id}", a.auth(false, a.hCatalogoEditar))

	m.Handle("GET /api/pessoas", a.auth(false, a.hPessoasList))
	m.Handle("POST /api/pessoas", a.auth(false, a.hPessoasAdd))
	m.Handle("PATCH /api/pessoas/{id}", a.auth(false, a.hPessoasEdit))
	m.Handle("DELETE /api/pessoas/{id}", a.auth(false, a.hPessoaExcluir)) // v9.7: admin/gerente excluem (com histórico → desativa)
	m.Handle("GET /api/pessoas/{id}/qr", a.auth(false, a.hPessoaQRCode))
	m.Handle("GET /api/pessoas/{id}/pdf", a.auth(false, a.hPessoaPDF))
	m.Handle("DELETE /api/grupos/{id}", a.auth(true, a.hGrupoExcluir)) // v9.7: só admin, só grupo vazio

	m.Handle("GET /api/usuarios", a.auth(false, a.hUsuariosList)) // admin: todas; gerente/operador: do próprio grupo (v9.4)
	// criação é validada DENTRO do handler (admin cria qualquer; gerente cria operador do próprio grupo)
	m.Handle("POST /api/usuarios", a.auth(false, a.hUsuariosAdd))
	m.Handle("GET /api/operadores-do-setor", a.auth(false, a.hOperadoresDoSetor))
	m.Handle("POST /api/operadores-do-setor", a.auth(false, a.hOperadoresDoSetor))
	m.Handle("PATCH /api/usuarios/{id}", a.auth(false, a.hUsuarioEdit))
	m.Handle("DELETE /api/usuarios/{id}", a.auth(false, a.hUsuarioExcluir))   // R6; gerente só operador do próprio grupo (v9.4)
	m.Handle("POST /api/usuarios/{id}/senha", a.auth(false, a.hUsuarioSenha)) // admin: qualquer; gerente: operador do próprio grupo (v9.4)
	m.Handle("GET /api/usuarios/{id}/foto", a.auth(false, a.hUsuarioFotoGet))
	m.Handle("GET /api/grupos", a.auth(false, a.hGruposList))
	m.Handle("GET /api/grupos/arvore", a.auth(false, a.hArvoreGrupos)) // v9.4: árvore nested (admin: floresta; gerente: do próprio)
	m.Handle("POST /api/grupos", a.auth(true, a.hGruposAdd))           // R7: exige gerente no ato
	m.Handle("GET /api/grupos/{id}/gerente", a.auth(true, a.hGrupoGerenteGet))
	m.Handle("POST /api/grupos/{id}/trocar-gerente", a.auth(true, a.hGrupoTrocarGerente))
	m.Handle("PATCH /api/grupos/{id}", a.auth(false, a.hGrupoUpdate))
	m.Handle("POST /api/admin/grupos/vinculo", a.auth(true, a.hAdminVinculoSet))   // R8
	m.Handle("DELETE /api/admin/grupos/vinculo", a.auth(true, a.hAdminVinculoRem)) // R8
	m.Handle("GET /api/vinculos", a.auth(false, a.hVinculoList))
	m.Handle("POST /api/vinculos", a.auth(false, a.hVinculoAdd)) // legado: fora da UI (R12)
	m.Handle("GET /api/perfil", a.auth(false, a.hPerfilGet))
	m.Handle("PATCH /api/perfil", a.auth(false, a.hPerfilSet))
	m.Handle("PATCH /api/usuarios/{id}/mover", a.auth(false, a.hMoverConta)) // v9.5: admin qualquer; gerente dentro da própria árvore
	m.Handle("POST /api/comentarios", confAuth(a.hComentariosAdd))
	m.Handle("GET /api/comentarios/{id}", confAuth(a.hComentariosList))
	m.Handle("GET /api/pessoas/{id}/comentarios", confAuth(a.hPessoaComentarios))

	m.Handle("GET /api/relatorio", a.auth(false, a.hRelatorioJSON))
	m.Handle("GET /api/relatorio.pdf", a.auth(false, a.hRelatorioPDF))
	m.Handle("GET /api/export/{t}", a.auth(false, a.hExportCSV))
	m.Handle("GET /api/export", a.auth(false, a.hExportarDados))
	m.Handle("GET /api/notificacoes", a.auth(false, a.hNotificacoesHub))

	m.Handle("POST /api/backup", a.auth(true, a.hBackup))
	m.Handle("POST /api/backup/importar", a.auth(true, a.hBackupImportar)) // R9
	m.Handle("GET /api/backup/download", a.auth(true, a.hBackupDownload))
	m.Handle("GET /api/admin/sistema/metricas", a.auth(true, a.hAdminSistemaMetricas))

	// Módulos ESCALA e MATERIAL EM RESERVA (ordem Tenente 30/09): fora do frontend e
	// Módulos de Escalas e Material desbloqueados para apreciação (respeita MODO_RESERVA=1 se configurado)
	// Fix P1 (fail-closed): erro de banco => módulo em reserva (423), JAMais liberado; delega ao helper único.
	reservaAuth := func(next http.HandlerFunc) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if a.reservaAtivo() {
				jsonErro(w, http.StatusLocked, "módulo em reserva operacional")
				return
			}
			a.authPapeis([]string{"gerente", "operador", "chefe_setor"}, next).ServeHTTP(w, r)
		})
	}

	// Módulo de Escalas e Serviços Integrados (v1.0) — EM RESERVA (ordem Tenente 30/09)
	m.Handle("GET /api/escalas/tipos", reservaAuth(a.hEscalasTiposList))
	m.Handle("POST /api/escalas/tipos", reservaAuth(a.hEscalasTiposAdd))
	m.Handle("DELETE /api/escalas/tipos/{id}", reservaAuth(a.hEscalasTiposDel))
	m.Handle("GET /api/escalas/turnos", reservaAuth(a.hEscalasTurnosList))
	m.Handle("POST /api/escalas/turnos", reservaAuth(a.hEscalasTurnosSave))
	m.Handle("GET /api/escalas/hoje", reservaAuth(a.hEscalasHoje))
	m.Handle("GET /api/escalas/pdf", a.auth(false, a.hEscalasPDF))

	// Escalas 2.0 (v1.5) — Modelos, Fases, Delegação e Minhas Escalas
	m.Handle("GET /api/escalas/modelos", reservaAuth(a.hEscalasModelosList))
	m.Handle("POST /api/escalas/modelos", reservaAuth(a.hEscalasModelosSave))
	m.Handle("GET /api/escalas/modelos/{id}", reservaAuth(a.hEscalasModelosGet))
	m.Handle("DELETE /api/escalas/modelos/{id}", reservaAuth(a.hEscalasModelosDel))
	m.Handle("POST /api/escalas/aplicar-modelo", reservaAuth(a.hEscalasAplicarModelo))
	m.Handle("POST /api/escalas/limpar-dia", reservaAuth(a.hEscalasLimparDia))
	m.Handle("POST /api/escalas/turnos/{id}/alocar", reservaAuth(a.hEscalasTurnoAlocar))
	m.Handle("POST /api/escalas/turnos/{id}/delegar", reservaAuth(a.hEscalasTurnoDelegar))
	m.Handle("GET /api/escalas/turnos/{id}/candidatos", reservaAuth(a.hEscalasTurnoCandidatos))
	m.Handle("PATCH /api/escalas/fase", reservaAuth(a.hEscalasAlterarFase))
	m.Handle("GET /api/escalas/relatorio-dia.pdf", a.auth(false, a.hEscalasRelatorioDiaPDF))
	m.Handle("GET /api/escalas/relatorio-dia/pdf", a.auth(false, a.hEscalasRelatorioDiaPDF))
	m.Handle("GET /api/escalas/minhas", a.auth(false, a.hEscalasMinhas))

	// Módulo de Material e Cautelas (v1.0) — EM RESERVA (ordem Tenente 30/09)
	m.Handle("GET /api/material/categorias", reservaAuth(a.hMaterialCategoriasList))
	m.Handle("POST /api/material/categorias", reservaAuth(a.hMaterialCategoriasAdd))
	m.Handle("DELETE /api/material/categorias/{id}", reservaAuth(a.hMaterialCategoriasDel))
	m.Handle("GET /api/material/itens", reservaAuth(a.hMaterialItensList))
	m.Handle("POST /api/material/itens", reservaAuth(a.hMaterialItensSave))
	m.Handle("DELETE /api/material/itens/{id}", reservaAuth(a.hMaterialItensDel))
	m.Handle("GET /api/material/itens/{id}/qr", reservaAuth(a.hMaterialItemQRCode))
	m.Handle("GET /api/material/etiquetas-lote.pdf", reservaAuth(a.hMaterialEtiquetasLotePDF))
	m.Handle("GET /api/material/inventario/pdf", a.auth(false, a.hMaterialInventarioPDF))
	m.Handle("POST /api/material/cautelar", reservaAuth(a.hMaterialCautelar))
	m.Handle("POST /api/material/devolver", reservaAuth(a.hMaterialDevolver))
	m.Handle("GET /api/material/cautelas", reservaAuth(a.hMaterialCautelasList))
	m.Handle("GET /api/material/cautelas/{id}/recibo.pdf", a.auth(false, a.hMaterialCautelaReciboPDF))
	m.Handle("POST /api/material/cautelas/{id}/anexos", reservaAuth(a.hMaterialAnexoAdd))
	m.Handle("GET /api/material/cautelas/{id}/anexos", reservaAuth(a.hMaterialAnexoList))
	m.Handle("GET /api/material/anexos/{id}", reservaAuth(a.hMaterialAnexoGet))
	m.Handle("DELETE /api/material/anexos/{id}", reservaAuth(a.hMaterialAnexoDel))

	// Workflow Setorial (v1.5) — Sugestões e Aprovações por Chefe de Setor
	m.Handle("GET /api/setores/sugestoes", a.auth(false, a.hSetorSugestoesList))
	m.Handle("POST /api/setores/sugestoes", a.auth(false, a.hSetorSugestoesAdd))
	m.Handle("POST /api/setores/sugestoes/{id}/avaliar", a.auth(false, a.hSetorSugestoesAvaliar))

	// Consciência Situacional (v1.5) — Comando e Visão Geral Consolidada
	// Consciência Situacional: módulo em ACERVO para versão futura (ordem Diretor
	// 05/10/26: acesso removido across the board). Rota DESLIGADA (404 — mesmo
	// tratamento de rota inexistente); handler e código preservados para o religamento.
	m.HandleFunc("GET /api/consciencia/resumo", func(w http.ResponseWriter, r *http.Request) { http.NotFound(w, r) })

	// Módulo de Configurações e White-Label (v1.0)
	m.HandleFunc("GET /api/configuracoes", a.hConfiguracoesGet)
	m.Handle("POST /api/configuracoes", a.auth(true, a.hConfiguracoesSet))

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
		SameSite: http.SameSiteStrictMode, MaxAge: int(ttlSessao.Seconds()),
	})
	jsonOK(w, map[string]any{"usuario": u, "expira": expira})
}

func (a *App) hAuthSetup(w http.ResponseWriter, r *http.Request) {
	u := usuarioDoCtx(r)
	if !u.PrecisaSetup {
		jsonErro(w, http.StatusBadRequest, "usuário não precisa de setup")
		return
	}
	var req struct {
		NovoLogin string `json:"novo_login"`
		NovaSenha string `json:"nova_senha"`
	}
	if err := decodificar(r, &req); err != nil || req.NovoLogin == "" || req.NovaSenha == "" {
		jsonErro(w, http.StatusBadRequest, "identificação e nova senha são obrigatórios")
		return
	}
	if len(req.NovaSenha) < 8 {
		jsonErro(w, http.StatusBadRequest, "a nova senha deve ter no mínimo 8 caracteres")
		return
	}

	hash, err := hashSenha(req.NovaSenha)
	if err != nil {
		jsonErro(w, http.StatusInternalServerError, "falha ao processar senha")
		return
	}

	req.NovoLogin = strings.ToLower(strings.TrimSpace(req.NovoLogin))

	_, err = a.st.db.Exec(`UPDATE usuarios SET login = ?, senha_hash = ?, precisa_setup = 0 WHERE id = ?`, req.NovoLogin, hash, u.ID)
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE constraint") {
			jsonErro(w, http.StatusConflict, "este número de identificação já está em uso por outro usuário")
			return
		}
		jsonErro(w, http.StatusInternalServerError, "falha ao salvar novos dados")
		return
	}
	a.st.Auditoria(&u.ID, "setup_concluido", "usuarios", &u.ID, "login atualizado", ipDe(r))
	jsonOK(w, map[string]string{"msg": "Cadastro atualizado com sucesso. Faça o login novamente."})
}

func (a *App) hMe(w http.ResponseWriter, r *http.Request) {
	jsonOK(w, map[string]any{"usuario": usuarioDoCtx(r)})
}

func (a *App) hLogout(w http.ResponseWriter, r *http.Request) {
	if c, err := r.Cookie(cookieSessao); err == nil {
		a.st.EncerrarSessao(c.Value)
	}
	http.SetCookie(w, &http.Cookie{Name: cookieSessao, Value: "", Path: "/", HttpOnly: true, SameSite: http.SameSiteStrictMode, MaxAge: -1})
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

func (a *App) hAdminSistemaMetricas(w http.ResponseWriter, r *http.Request) {
	metricas := coletarMetricas(a.st.dataDir)
	jsonOK(w, metricas)
}

// hUsuarioSenha: admin redefine a senha de qualquer conta; GERENTE redefine a de
// OPERADOR do próprio grupo (v9.4 — aba Gerenciar). Senha de gerente só admin muda.
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
	solicitante := usuarioDoCtx(r)
	if solicitante.Papel == "gerente" {
		var alvoPapel string
		var alvoGrupo int64
		if err := a.st.db.QueryRow(`SELECT papel, COALESCE(grupo_id,0) FROM usuarios WHERE id = ?`, id).Scan(&alvoPapel, &alvoGrupo); err != nil {
			jsonErro(w, http.StatusNotFound, "usuário inexistente")
			return
		}
		if (alvoPapel != "operador" && alvoPapel != "chefe_setor") || solicitante.GrupoID == nil || alvoGrupo != *solicitante.GrupoID {
			jsonErro(w, http.StatusForbidden, "gerente só redefine senha de membros do próprio grupo")
			return
		}
	}
	var hashAtual string
	if err := a.st.db.QueryRow(`SELECT senha_hash FROM usuarios WHERE id = ?`, id).Scan(&hashAtual); err != nil {
		jsonErro(w, http.StatusInternalServerError, err.Error())
		return
	}
	_ = hashAtual // leitura apenas para validar existência da conta
	novoHash, err := hashSenha(req.Senha)
	if err != nil {
		jsonErro(w, http.StatusInternalServerError, err.Error())
		return
	}
	res, err := a.st.db.Exec(`UPDATE usuarios SET senha_hash = ? WHERE id = ?`, novoHash, id)
	if err != nil {
		jsonErro(w, http.StatusInternalServerError, err.Error())
		return
	}
	if n, _ := res.RowsAffected(); n == 0 {
		jsonErro(w, http.StatusNotFound, "usuário inexistente")
		return
	}
	a.st.Auditoria(&solicitante.ID, "redefinir_senha", "usuarios", &id, "", ipDe(r))
	jsonOK(w, map[string]bool{"ok": true})
}

func (a *App) hUsuarioEdit(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		jsonErro(w, http.StatusBadRequest, "id inválido")
		return
	}
	var req struct {
		NomeGuerra     *string `json:"nome_guerra"`
		NomeCompleto   *string `json:"nome_completo"`
		DataNascimento *string `json:"data_nascimento"`
		TipoSanguineo  *string `json:"tipo_sanguineo"`
		Telefone       *string `json:"telefone"`
		Email          *string `json:"email"`
		Endereco       *string `json:"endereco"`
		FotoBase64     *string `json:"foto_base64"`
		GrupoID        *int64  `json:"grupo_id"`
		FuncaoID       *int64  `json:"funcao_id"`
		SetorID        *int64  `json:"setor_id"`
		Ativo          *bool   `json:"ativo"`
	}
	if err = decodificar(r, &req); err != nil {
		jsonErro(w, http.StatusBadRequest, "JSON inválido")
		return
	}
	solicitante := usuarioDoCtx(r)
	if solicitante.Papel == "operador" || solicitante.Papel == "chefe_setor" {
		jsonErro(w, http.StatusForbidden, "usuário sem permissão para editar outros usuários")
		return
	}
	if solicitante.Papel == "gerente" {
		var alvoPapel string
		var alvoGrupo int64
		if err := a.st.db.QueryRow(`SELECT papel, COALESCE(grupo_id,0) FROM usuarios WHERE id = ?`, id).Scan(&alvoPapel, &alvoGrupo); err != nil {
			jsonErro(w, http.StatusNotFound, "usuário inexistente")
			return
		}
		if (alvoPapel != "operador" && alvoPapel != "chefe_setor") || solicitante.GrupoID == nil || alvoGrupo != *solicitante.GrupoID {
			jsonErro(w, http.StatusForbidden, "gerente só edita membros do próprio grupo")
			return
		}
	}

	if req.NomeGuerra != nil {
		_, _ = a.st.db.Exec(`UPDATE usuarios SET nome_guerra = ? WHERE id = ?`, strings.TrimSpace(*req.NomeGuerra), id)
		var pID *int64
		_ = a.st.db.QueryRow(`SELECT pessoa_id FROM usuarios WHERE id = ?`, id).Scan(&pID)
		if pID != nil {
			_, _ = a.st.db.Exec(`UPDATE pessoas SET nome_guerra = ? WHERE id = ?`, strings.TrimSpace(*req.NomeGuerra), *pID)
		}
	}
	if req.NomeCompleto != nil {
		_, _ = a.st.db.Exec(`UPDATE usuarios SET nome_completo = ? WHERE id = ?`, strings.TrimSpace(*req.NomeCompleto), id)
		var pID *int64
		_ = a.st.db.QueryRow(`SELECT pessoa_id FROM usuarios WHERE id = ?`, id).Scan(&pID)
		if pID != nil {
			_, _ = a.st.db.Exec(`UPDATE pessoas SET nome_completo = ? WHERE id = ?`, strings.TrimSpace(*req.NomeCompleto), *pID)
		}
	}
	if req.DataNascimento != nil {
		_, _ = a.st.db.Exec(`UPDATE usuarios SET data_nascimento = ? WHERE id = ?`, strings.TrimSpace(*req.DataNascimento), id)
	}
	if req.TipoSanguineo != nil {
		_, _ = a.st.db.Exec(`UPDATE usuarios SET tipo_sanguineo = ? WHERE id = ?`, strings.TrimSpace(*req.TipoSanguineo), id)
	}
	if req.Telefone != nil {
		_, _ = a.st.db.Exec(`UPDATE usuarios SET telefone = ? WHERE id = ?`, strings.TrimSpace(*req.Telefone), id)
	}
	if req.Email != nil {
		_, _ = a.st.db.Exec(`UPDATE usuarios SET email = ? WHERE id = ?`, strings.TrimSpace(*req.Email), id)
	}
	if req.Endereco != nil {
		_, _ = a.st.db.Exec(`UPDATE usuarios SET endereco = ? WHERE id = ?`, strings.TrimSpace(*req.Endereco), id)
	}
	if req.FotoBase64 != nil {
		_, _ = a.st.db.Exec(`UPDATE usuarios SET foto_base64 = ? WHERE id = ?`, strings.TrimSpace(*req.FotoBase64), id)
	}
	if req.GrupoID != nil {
		gid := *req.GrupoID
		if gid <= 0 {
			_, _ = a.st.db.Exec(`UPDATE usuarios SET grupo_id = NULL WHERE id = ?`, id)
		} else {
			_, _ = a.st.db.Exec(`UPDATE usuarios SET grupo_id = ? WHERE id = ?`, gid, id)
			// Adiciona ou preserva papel no grupo
			_, _ = a.st.db.Exec(`INSERT OR IGNORE INTO usuario_papeis (usuario_id, grupo_id, papel) VALUES (?, ?, 'operador')`, id, gid)
		}
	}
	if req.FuncaoID != nil {
		_, _ = a.st.db.Exec(`UPDATE usuarios SET funcao_id = ? WHERE id = ?`, req.FuncaoID, id)
		var pID *int64
		_ = a.st.db.QueryRow(`SELECT pessoa_id FROM usuarios WHERE id = ?`, id).Scan(&pID)
		if pID != nil {
			_, _ = a.st.db.Exec(`UPDATE pessoas SET funcao_id = ? WHERE id = ?`, req.FuncaoID, *pID)
		}
	}
	if req.SetorID != nil {
		_, _ = a.st.db.Exec(`UPDATE usuarios SET setor_id = ? WHERE id = ?`, req.SetorID, id)
		var pID *int64
		_ = a.st.db.QueryRow(`SELECT pessoa_id FROM usuarios WHERE id = ?`, id).Scan(&pID)
		if pID != nil {
			_, _ = a.st.db.Exec(`UPDATE pessoas SET setor_id = ? WHERE id = ?`, req.SetorID, *pID)
		}
	}
	if req.Ativo != nil {
		at := 0
		if *req.Ativo {
			at = 1
		}
		_, _ = a.st.db.Exec(`UPDATE usuarios SET ativo = ? WHERE id = ?`, at, id)
	}
	a.st.Auditoria(&solicitante.ID, "editar_usuario", "usuarios", &id, "", ipDe(r))
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
	// v9.14.2: ?id=N abre conferência específica (várias simultâneas); senão a mais recente
	idQ := r.URL.Query().Get("id")
	qHoje := `SELECT id, status, data, criado_em FROM conferencias
		 WHERE status = 'aberta' AND grupo_id = ?`
	argsHoje := []any{escopo}
	if idQ != "" {
		if cid, e := strconv.ParseInt(idQ, 10, 64); e == nil {
			qHoje += ` AND id = ?`
			argsHoje = append(argsHoje, cid)
		}
	}
	qHoje += ` ORDER BY id DESC LIMIT 1`
	err = a.st.db.QueryRow(qHoje, argsHoje...).
		Scan(&f.ID, &f.Status, &f.Data, &f.CriadaEm)
	var form *map[string]any
	if err == nil {
		estados := map[int64]map[string]any{}
		rows, e := a.st.db.Query(
			`SELECT pessoa_id, situacao, destino_id, COALESCE(observacao,''), verificado
			 FROM presencas WHERE conferencia_id = ?`, f.ID)
		if e == nil {
			for rows.Next() {
				var pid int64
				var sit string
				var did *int64
				var obs string
				var verificado int
				if rows.Scan(&pid, &sit, &did, &obs, &verificado) == nil {
					estados[pid] = map[string]any{"situacao": sit, "destino_id": did, "observacao": obs,
						"verificado": verificado == 1}
				}
			}
			rows.Close()
		}
		form = &map[string]any{"id": f.ID, "status": f.Status, "data": f.Data,
			"criada_em": f.CriadaEm, "estados": estados}
	}

	var setoresStatus []map[string]any
	if f.ID > 0 {
		qSetores := `
			SELECT s.id, s.nome, COALESCE(s.sigla, ''),
			       COALESCE(cs.status, 'nao_iniciada'),
			       cs.concluido_por, COALESCE(u.nome_guerra, u.login, ''), cs.concluido_em,
			       COUNT(DISTINCT p.id) AS total_efetivo,
			       COUNT(DISTINCT CASE WHEN pr.verificado = 1 THEN p.id ELSE NULL END) AS total_verificados
			FROM setores s
			JOIN pessoas p ON p.setor_id = s.id AND p.status = 'ativo' AND (? <= 0 OR p.grupo_id = ?)
			LEFT JOIN conferencia_setores cs ON cs.setor_id = s.id AND cs.conferencia_id = ?
			LEFT JOIN usuarios u ON u.id = cs.concluido_por
			LEFT JOIN presencas pr ON pr.conferencia_id = ? AND pr.pessoa_id = p.id
			WHERE s.ativo = 1 AND (? <= 0 OR s.grupo_id = ? OR s.grupo_id IS NULL)
			GROUP BY s.id, s.nome, s.sigla, cs.status, cs.concluido_por, u.nome_guerra, u.login, cs.concluido_em
			ORDER BY s.nome ASC`
		sRows, sErr := a.st.db.Query(qSetores, escopo, escopo, f.ID, f.ID, escopo, escopo)
		if sErr == nil {
			for sRows.Next() {
				var sid int64
				var sNome, sSigla, sStatus, concNome string
				var concPor *int64
				var concEm *string
				var totEf, totVer int
				if sRows.Scan(&sid, &sNome, &sSigla, &sStatus, &concPor, &concNome, &concEm, &totEf, &totVer) == nil {
					item := map[string]any{
						"setor_id":           sid,
						"setor_nome":         sNome,
						"setor_sigla":        sSigla,
						"status":             sStatus,
						"total_efetivo":      totEf,
						"total_pessoas":      totEf,
						"total_verificados":  totVer,
						"verificados":        totVer,
						"concluido_por_id":   concPor,
						"concluido_por_nome": concNome,
						"concluido_em":       concEm,
					}
					setoresStatus = append(setoresStatus, item)
				}
			}
			sRows.Close()
		}
	}
	if setoresStatus == nil {
		setoresStatus = []map[string]any{}
	}

	dataHoje := time.Now().In(a.horaLocal).Format("2006-01-02")
	if f.Data != "" {
		dataHoje = f.Data
	}
	escalados := a.escaladosNaData(escopo, dataHoje)
	tHoje, _ := time.Parse("2006-01-02", dataHoje)
	dataOntem := tHoje.AddDate(0, 0, -1).Format("2006-01-02")
	escaladosOntem := a.escaladosNaData(escopo, dataOntem)
	jsonOK(w, map[string]any{
		"conferencia":     form,
		"setores_status":  setoresStatus,
		"pessoas":         a.pessoasAtivas(escopo),
		"escalados":       escalados,
		"escalados_ontem": escaladosOntem,
	})
}

type lancamentoReq struct {
	PessoaID   int64  `json:"pessoa_id"`
	Situacao   string `json:"situacao"`
	DestinoID  *int64 `json:"destino_id"`
	TagID      *int64 `json:"tag_id"`
	Observacao string `json:"observacao"`
	Verificado bool   `json:"verificado"` // ordem Tenente 30/09: ✅ no fechamento decide NÃO VERIFICADO
}

// hConferenciaMarcar (v9.13, ordem Tenente 29/09): salvamento PARCIAL — grava imediatamente
// o estado de UM militar na conferência ABERTA do escopo. Reload volta ao ponto (o GET hoje
// já devolve estados). Imutabilidade: conferência fechada rejeita.
func (a *App) hConferenciaMarcar(w http.ResponseWriter, r *http.Request) {
	u := usuarioDoCtx(r)
	escopo := escopoDoUsuario(u)
	var req struct {
		PessoaID   int64   `json:"pessoa_id"`
		Situacao   string  `json:"situacao"`
		DestinoID  *int64  `json:"destino_id"`
		Observacao *string `json:"observacao"`
		Verificado *bool   `json:"verificado"`
	}
	if err := decodificar(r, &req); err != nil || req.PessoaID == 0 {
		jsonErro(w, http.StatusBadRequest, "pessoa_id obrigatório")
		return
	}
	var confID int64
	var status string
	qMark := `SELECT id, status FROM conferencias
		WHERE status = 'aberta' AND grupo_id = ?`
	argsMark := []any{escopo}
	if idQ := r.URL.Query().Get("id"); idQ != "" {
		if cid, e := strconv.ParseInt(idQ, 10, 64); e == nil {
			qMark += ` AND id = ?`
			argsMark = append(argsMark, cid)
		}
	}
	qMark += ` ORDER BY id DESC LIMIT 1`
	err := a.st.db.QueryRow(qMark, argsMark...).Scan(&confID, &status)
	if err != nil {
		jsonErro(w, http.StatusConflict, "nenhuma conferência aberta")
		return
	}
	// pessoa precisa pertencer ao escopo
	var n int
	_ = a.st.db.QueryRow(`SELECT COUNT(*) FROM pessoas WHERE id = ? AND grupo_id = ? AND status='ativo'`,
		req.PessoaID, escopo).Scan(&n)
	if n == 0 {
		jsonErro(w, http.StatusForbidden, "pessoa fora do seu escopo")
		return
	}

	// Regra de Setor: Chefe de Setor só tira falta/presença do seu próprio setor
	if u.Papel == "chefe_setor" {
		var setorChefe *int64
		if u.SetorID != nil {
			setorChefe = u.SetorID
		} else if u.PessoaID != nil {
			_ = a.st.db.QueryRow(`SELECT setor_id FROM pessoas WHERE id = ?`, *u.PessoaID).Scan(&setorChefe)
		}
		if setorChefe == nil {
			jsonErro(w, http.StatusForbidden, "chefe de setor sem setor atribuído no cadastro")
			return
		}
		var setorPessoa *int64
		_ = a.st.db.QueryRow(`SELECT setor_id FROM pessoas WHERE id = ?`, req.PessoaID).Scan(&setorPessoa)
		if setorPessoa == nil || *setorPessoa != *setorChefe {
			jsonErro(w, http.StatusForbidden, "chefe de setor só pode lançar presença para militares do seu próprio setor")
			return
		}
	}
	if req.Situacao == "" {
		// "desmarcar" o check (ordem Tenente 30/09): REMOVE a verificação — grava
		// verificado=0 no lançamento existente (antes o uncheck não persistia e o ✅
		// ressuscitava no reload); sem lançamento, nada a gravar.
		var jahExiste int
		_ = a.st.db.QueryRow(`SELECT COUNT(*) FROM presencas WHERE conferencia_id = ? AND pessoa_id = ?`,
			confID, req.PessoaID).Scan(&jahExiste)
		if jahExiste > 0 {
			_, _ = a.st.db.Exec(`UPDATE presencas SET verificado = 0, alterado_por = ?, alterado_em = strftime('%Y-%m-%dT%H:%M:%fZ','now')
				WHERE conferencia_id = ? AND pessoa_id = ?`, u.ID, confID, req.PessoaID)
		}
		jsonOK(w, map[string]any{"ok": true, "gravado": jahExiste > 0})
		return
	}
	switch req.Situacao {
	case "presente", "atraso", "falta", "justificada", "nao_verificado":
	default:
		jsonErro(w, http.StatusBadRequest, "situação inválida")
		return
	}
	if req.Situacao == "justificada" && req.DestinoID == nil {
		jsonErro(w, http.StatusBadRequest, "justificada exige destino")
		return
	}
	// v1.5: Estados de presente, falta e atraso não têm destino (destino zerado); somente justificada aceita destino.
	if req.Situacao != "justificada" {
		req.DestinoID = nil
	}

	// v1.5: Caso o estado mude em relação ao anterior, a observação é resetada (salvo nova observação explícita).
	var sitAnt, obsAnt string
	_ = a.st.db.QueryRow(`SELECT situacao, COALESCE(observacao,'') FROM presencas WHERE conferencia_id = ? AND pessoa_id = ?`,
		confID, req.PessoaID).Scan(&sitAnt, &obsAnt)
	if sitAnt != "" && sitAnt != req.Situacao && req.Observacao == nil {
		vazio := ""
		req.Observacao = &vazio
	}
	// v9.16.4: ✅ de verificação persiste separado (carry over inicia zerado)
	verificadoFlag := 0
	if req.Verificado != nil && *req.Verificado {
		verificadoFlag = 1
	}
	// v9.15.3: marcado_em em UTC REAL (exibição converte p/ Brasília)
	marcadoEm := time.Now().UTC().Format("2006-01-02T15:04:05.000Z")
	_, err = a.st.db.Exec(`INSERT INTO presencas (conferencia_id, pessoa_id, situacao, destino_id, observacao, marcado_por, marcado_em, verificado)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(conferencia_id, pessoa_id) DO UPDATE SET
		  situacao = excluded.situacao,
		  destino_id = excluded.destino_id,
		  observacao = excluded.observacao,
		  alterado_por = excluded.marcado_por,
		  alterado_em = excluded.marcado_em,
		  verificado = excluded.verificado`,
		confID, req.PessoaID, req.Situacao, req.DestinoID, req.Observacao, u.ID, marcadoEm, verificadoFlag)
	if err != nil {
		jsonErro(w, http.StatusInternalServerError, err.Error())
		return
	}

	// v1.5: Atualização automática da conferência setorial para 'em_andamento'
	var pSetorID *int64
	_ = a.st.db.QueryRow(`SELECT setor_id FROM pessoas WHERE id = ?`, req.PessoaID).Scan(&pSetorID)
	if pSetorID != nil && *pSetorID > 0 {
		_, _ = a.st.db.Exec(`
			INSERT INTO conferencia_setores (conferencia_id, setor_id, status)
			VALUES (?, ?, 'em_andamento')
			ON CONFLICT(conferencia_id, setor_id) DO UPDATE SET
			  status = 'em_andamento'
		`, confID, *pSetorID)
	}

	a.st.Auditoria(&u.ID, "marcar_parcial", "presencas", &confID,
		"pessoa "+fmt.Sprintf("%d", req.PessoaID)+" → "+req.Situacao, ipDe(r))
	jsonOK(w, map[string]any{"ok": true, "gravado": true, "conferencia_id": confID})
}

// hEfetivoAtual (v9.15, ordem Tenente 29/09): estado ATUAL de cada militar = estado na ÚLTIMA
// conferência em que foi lançado (não agregação de período). Usado no dashboard de relatórios
// com alertas de frescor: >1 dia = amarelo; >1 semana = vermelho (calculado no cliente pela data).
func (a *App) hEfetivoAtual(w http.ResponseWriter, r *http.Request) {
	u := usuarioDoCtx(r)
	escopo := escopoDoUsuario(u)
	q := `
		SELECT p.id, p.nome_guerra, COALESCE(s.nome,''), COALESCE(fu.nome,''),
		       COALESCE((SELECT g.nome FROM grupos g WHERE g.id = p.grupo_id),'—'),
		       ult.data, ult.situacao, ult.conferencia_id
		FROM pessoas p
		LEFT JOIN setores s ON s.id = p.setor_id
		LEFT JOIN funcoes fu ON fu.id = p.funcao_id
		LEFT JOIN (
			SELECT pr.pessoa_id, c.data, pr.situacao, pr.conferencia_id,
			       ROW_NUMBER() OVER (PARTITION BY pr.pessoa_id ORDER BY c.data DESC, c.id DESC) rn
			FROM presencas pr
			JOIN conferencias c ON c.id = pr.conferencia_id AND c.status = 'fechada'
		) ult ON ult.pessoa_id = p.id AND ult.rn = 1
		WHERE p.status = 'ativo'`
	var args []any
	if escopo > 0 {
		q += ` AND p.grupo_id = ?`
		args = append(args, escopo)
	} else if escopo == -1 {
		// Conta corrompida sem grupo: não vê ninguém
		jsonOK(w, map[string]any{"pessoas": []map[string]any{}, "hoje": time.Now().In(a.horaLocal).Format("2006-01-02")})
		return
	}
	q += ` ORDER BY COALESCE(s.nome,''), p.nome_guerra`
	rows, err := a.st.db.Query(q, args...)
	if err != nil {
		jsonErro(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer rows.Close()
	out := []map[string]any{}
	for rows.Next() {
		var id int64
		var ng, setor, funcao, grupo string
		var data, sit *string
		var confID *int64
		if rows.Scan(&id, &ng, &setor, &funcao, &grupo, &data, &sit, &confID) == nil {
			m := map[string]any{"id": id, "nome_guerra": ng, "setor": setor, "funcao": funcao, "grupo": grupo}
			if data != nil {
				m["ultima_data"] = *data
				m["situacao"] = *sit
				m["conferencia_id"] = *confID
			} else {
				m["ultima_data"] = nil
				m["situacao"] = nil
			}
			out = append(out, m)
		}
	}
	jsonOK(w, map[string]any{"pessoas": out, "hoje": time.Now().In(a.horaLocal).Format("2006-01-02")})
}

// pessoasAtivas(escopo): escopo 0 = todas (admin); N = só do grupo N.
func (a *App) pessoasAtivas(escopo int64) []map[string]any {
	q := `
		SELECT p.id, p.nome_guerra, p.nome_completo, COALESCE(s.nome,''),
		       COALESCE(NULLIF(fu.nome,''), COALESCE(NULLIF(fu_u.nome,''), COALESCE(NULLIF(fu_up.nome,''), ''))),
		       p.setor_id
		FROM pessoas p
		LEFT JOIN setores s ON s.id = p.setor_id
		LEFT JOIN funcoes fu ON fu.id = p.funcao_id
		LEFT JOIN usuarios u2 ON u2.pessoa_id = p.id
		LEFT JOIN funcoes fu_u ON fu_u.id = u2.funcao_id
		LEFT JOIN usuario_papeis up2 ON up2.usuario_id = u2.id AND (up2.grupo_id = p.grupo_id OR up2.grupo_id IS NULL)
		LEFT JOIN funcoes fu_up ON fu_up.id = up2.funcao_id
		WHERE p.status = 'ativo'`
	var rows *sql.Rows
	var err error
	if escopo > 0 {
		rows, err = a.st.db.Query(q+` AND p.grupo_id = ?
			ORDER BY COALESCE(s.nome,''), p.nome_guerra`, escopo)
	} else if escopo == 0 {
		rows, err = a.st.db.Query(q + `
			ORDER BY COALESCE(s.nome,''), p.nome_guerra`)
	} else {
		return []map[string]any{}
	}
	if err != nil {
		return []map[string]any{}
	}
	defer rows.Close()
	out := []map[string]any{}
	for rows.Next() {
		var id int64
		var ng, nc, setor, funcao string
		var setorID *int64
		if rows.Scan(&id, &ng, &nc, &setor, &funcao, &setorID) == nil {
			item := map[string]any{
				"id": id, "nome_guerra": ng, "nome_completo": nc, "setor": setor, "funcao": funcao,
			}
			if setorID != nil {
				item["setor_id"] = *setorID
			} else {
				item["setor_id"] = nil
			}
			out = append(out, item)
		}
	}
	return out
}

// hConferenciaIniciar: cria a conferência com data/hora de AGORA (Brasília) e a deixa ABERTA.
func (a *App) hConferenciaIniciar(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Local       string `json:"local"`
		Nome        string `json:"nome"`        // ordem 04/10: modal NOVA CONFERÊNCIA pede nome
		PrazoFinal  string `json:"prazo_final"` // horário-limite p/ pronto da conferência
		Encarregado *int64 `json:"encarregado_usuario_id"` // encarregado de pessoal
	}
	_ = decodificar(r, &req)
	// ordem Tenente (28/09): data/hora são coletadas do relógio — horário de Brasília
	// REGRA (28/09): ADMIN NÃO inicia conferência — só gerente/operador de grupo.
	u := usuarioDoCtx(r)
	if u.Papel == "admin" {
		jsonErro(w, http.StatusForbidden, "o admin não inicia conferências — quem inicia é o gerente/operador de um grupo")
		return
	}
	// ordem 04/10 (Fase G): conferência é iniciada por GERENTE DE GRUPO.
	if u.Papel != "gerente" {
		jsonErro(w, http.StatusForbidden, "a conferência é iniciada pelo gerente do grupo")
		return
	}
	if u.GrupoID == nil {
		jsonErro(w, http.StatusForbidden, "conta sem grupo definido")
		return
	}
	grupoID := *u.GrupoID
	data := time.Now().In(a.horaLocal).Format("2006-01-02")
	// ordem 04/10: a NOVA CONFERÊNCIA é nomeada pelo MODAL (nome+prazo; cancelar
	// descarta, despachar cria). Na API, chamadas sem nome (testes/integrações)
	// recebem o nome padrão — o front nunca envia vazio.
	if strings.TrimSpace(req.Nome) == "" {
		req.Nome = tipoConferenciaPadrao + " — " + data
	}
	tipoID, _, err := a.tipoPadraoID()
	if err != nil {
		jsonErro(w, http.StatusInternalServerError, "tipo '"+tipoConferenciaPadrao+"' inexistente")
		return
	}
	// v9.14.2 (ordem Tenente 29/09): MÚLTIPLAS conferências abertas simultâneas por grupo
	// (o bloqueio anterior de "1 aberta por grupo" foi removido). Cada conferência tem o
	// próprio ID; o /hoje (edição) usa ?id= quando informado, senão a mais recente.
	var id int64
	// v9.15.3: banco grava UTC REAL (sufixo Z verdadeiro); EXIBIÇÃO converte p/ Brasília
	criadoEm := time.Now().UTC().Format("2006-01-02T15:04:05.000Z")
	res, e := a.st.db.Exec(
		`INSERT INTO conferencias (data, tipo_id, local, grupo_id, criado_por, criado_em, nome, prazo_final, encarregado_usuario_id) VALUES (?,?,?,?,?,?,?,?,?)`,
		data, tipoID, req.Local, grupoID, u.ID, criadoEm,
		strings.TrimSpace(req.Nome), strings.TrimSpace(req.PrazoFinal), req.Encarregado)
	if e != nil {
		jsonErro(w, http.StatusInternalServerError, e.Error())
		return
	}
	id, _ = res.LastInsertId()

	// v9.16.3 (ordem Tenente 29/09): CARRY OVER — a nova conferência herda o estado da última
	// conferência fechada de cada militar, com regra de frescor:
	//   última conf hoje ou ontem  → mantém situação + destino (serviço de ontem continua)
	//   última conf há 2+ dias     → reseta para 'presente' (ex.: sexta → segunda)
	//   nunca conferido            → nada a herdar (default 'presente')
	// Herda APENAS pessoas ativas do grupo.
	// FIX P0-1 (revisão DEV-L 30/09): fechamento da subquery ROW_NUMBER foi apagado na
	// v1.0 e o erro morria no `_, _ =` — toda conferência nova nascia SEM herdar estado.
	if _, err := a.st.db.Exec(`INSERT INTO presencas
		(conferencia_id, pessoa_id, situacao, destino_id, observacao, marcado_por, marcado_em, verificado)
		SELECT ?, pr.pessoa_id, pr.situacao, pr.destino_id, pr.observacao, ?, strftime('%Y-%m-%dT%H:%M:%fZ','now'), 0
		FROM presencas pr
		JOIN conferencias c ON c.id = pr.conferencia_id AND c.status = 'fechada'
		JOIN (SELECT pessoa_id, conferencia_id FROM (
		      SELECT p2.pessoa_id, p2.conferencia_id,
		             ROW_NUMBER() OVER (PARTITION BY p2.pessoa_id ORDER BY c2.data DESC, c2.id DESC) rn
		      FROM presencas p2 JOIN conferencias c2 ON c2.id = p2.conferencia_id AND c2.status='fechada'
		      ) WHERE rn = 1) u2 ON u2.pessoa_id = pr.pessoa_id AND u2.conferencia_id = c.id
		WHERE julianday(?) - julianday(c.data) <= 1
		  AND pr.pessoa_id IN (SELECT id FROM pessoas WHERE grupo_id = ? AND status = 'ativo')`,
		id, u.ID, data, grupoID); err != nil {
		log.Printf("sci carry-over conf %d: %v", id, err) // nunca mais silencioso
	}

	// v1.0: MOTOR INTELIGENTE — Se houver militares escalados na data (módulo Escalas),
	// garante o destino 'Serviço de Escala' e pré-associa na conferência como Justificada/Serviço.
	var destServicoID int64
	_ = a.st.db.QueryRow(`SELECT id FROM destinos WHERE (grupo_id = ? OR grupo_id IS NULL) AND LOWER(nome) LIKE '%serviço%' AND ativo = 1 ORDER BY grupo_id DESC LIMIT 1`, grupoID).Scan(&destServicoID)
	if destServicoID == 0 {
		// cria destino de serviço automaticamente se não existir
		resD, errD := a.st.db.Exec(`INSERT INTO destinos (grupo_id, nome, ativo) VALUES (?, 'Serviço de Escala', 1)`, grupoID)
		if errD == nil {
			destServicoID, _ = resD.LastInsertId()
		}
	}
	if destServicoID > 0 {
		// FIX P1-1 (revisão DEV-L 30/09): erro do motor NUNCA silencioso
		if _, err := a.st.db.Exec(`
			INSERT INTO presencas (conferencia_id, pessoa_id, situacao, destino_id, observacao, marcado_por, marcado_em, verificado)
			SELECT ?, ep.pessoa_id, 'justificada', ?, 'Escala: ' || etp.nome, ?, strftime('%Y-%m-%dT%H:%M:%fZ','now'), 0
			FROM escala_pessoas ep
			JOIN escala_turnos et ON et.id = ep.turno_id
			JOIN escala_tipos etp ON etp.id = et.tipo_id
			JOIN pessoas pes ON pes.id = ep.pessoa_id AND pes.status = 'ativo' AND pes.grupo_id = ?
			WHERE substr(et.data_inicio, 1, 10) <= ? AND substr(COALESCE(NULLIF(et.data_fim, ''), et.data_inicio), 1, 10) >= ?
			ON CONFLICT(conferencia_id, pessoa_id) DO UPDATE SET
			  situacao = 'justificada',
			  destino_id = excluded.destino_id,
			  observacao = excluded.observacao,
			  alterado_por = excluded.marcado_por,
			  alterado_em = strftime('%Y-%m-%dT%H:%M:%fZ','now')
		`, id, destServicoID, u.ID, grupoID, data, data); err != nil {
			log.Printf("sci motor-escalas conf %d: %v", id, err)
		}
	}

	// v1.5: Inicialização das conferências setoriais individuais
	_, _ = a.st.db.Exec(`
		INSERT OR IGNORE INTO conferencia_setores (conferencia_id, setor_id, status)
		SELECT ?, s.id, 'nao_iniciada'
		FROM setores s
		WHERE (s.grupo_id = ? OR s.grupo_id IS NULL)
		  AND s.ativo = 1
		  AND s.id IN (SELECT DISTINCT setor_id FROM pessoas WHERE grupo_id = ? AND status = 'ativo' AND setor_id IS NOT NULL)
	`, id, grupoID, grupoID)

	a.st.Auditoria(&u.ID, "iniciar", "conferencias", &id, "data="+data+" (carry over e escalas aplicados)", ipDe(r))
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
		case "presente", "atraso", "falta", "justificada", "nao_verificado":
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
		// ordem Tenente 30/09: conferência NÃO fecha com membro "presente" implícito —
		// quem está sem ✅ (verificado=0 no corpo do front) grava NAO_VERIFICADO
		// (pune no % de presença como tudo que não é presente). Destino cai fora:
		// NÃO VERIFICADO não tem destino (lançamento efetivo não aconteceu).
		if !l.Verificado {
			l.Situacao = "nao_verificado"
			l.DestinoID = nil
		}
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
		time.Now().UTC().Format("2006-01-02T15:04:05.000Z"), req.ID); err != nil {
		jsonErro(w, http.StatusInternalServerError, err.Error())
		return
	}
	_, _ = tx.Exec(`UPDATE conferencia_setores SET status = 'concluida' WHERE conferencia_id = ? AND status != 'concluida'`, req.ID)
	if err = tx.Commit(); err != nil {
		jsonErro(w, http.StatusInternalServerError, err.Error())
		return
	}
	a.st.Auditoria(&u.ID, "fechar", "conferencias", &req.ID,
		fmt.Sprintf("lancamentos=%d", gravados), ipDe(r))
	a.backupAssincrono("fechar") // zero-perda: cópia consistente a cada fechamento
	jsonOK(w, map[string]any{"conferencia_id": req.ID, "gravados": gravados})
}

// hConferenciaSetorConcluir: conclui a conferência setorial (status = 'concluida')
func (a *App) hConferenciaSetorConcluir(w http.ResponseWriter, r *http.Request) {
	u := usuarioDoCtx(r)
	cid, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || cid <= 0 {
		jsonErro(w, http.StatusBadRequest, "id de conferência inválido")
		return
	}
	sid, err := strconv.ParseInt(r.PathValue("setor_id"), 10, 64)
	if err != nil || sid <= 0 {
		jsonErro(w, http.StatusBadRequest, "setor_id inválido")
		return
	}

	var confStatus string
	var confGrupoID int64
	if err := a.st.db.QueryRow(`SELECT status, COALESCE(grupo_id, 0) FROM conferencias WHERE id = ?`, cid).Scan(&confStatus, &confGrupoID); err != nil {
		jsonErro(w, http.StatusNotFound, "conferência não encontrada")
		return
	}
	if confStatus != "aberta" {
		jsonErro(w, http.StatusBadRequest, "conferência já está fechada")
		return
	}
	if esc := escopoDoUsuario(u); esc > 0 && confGrupoID != esc {
		jsonErro(w, http.StatusForbidden, "conferência fora do seu escopo")
		return
	}

	if u.Papel == "chefe_setor" {
		var setorChefe *int64
		if u.SetorID != nil {
			setorChefe = u.SetorID
		} else if u.PessoaID != nil {
			_ = a.st.db.QueryRow(`SELECT setor_id FROM pessoas WHERE id = ?`, *u.PessoaID).Scan(&setorChefe)
		}
		if setorChefe == nil || *setorChefe != sid {
			jsonErro(w, http.StatusForbidden, "chefe de setor só pode concluir seu próprio setor")
			return
		}
	}

	concluidoEm := time.Now().UTC().Format("2006-01-02T15:04:05.000Z")
	_, err = a.st.db.Exec(`
		INSERT INTO conferencia_setores (conferencia_id, setor_id, status, concluido_por, concluido_em)
		VALUES (?, ?, 'concluida', ?, ?)
		ON CONFLICT(conferencia_id, setor_id) DO UPDATE SET
		  status = 'concluida',
		  concluido_por = excluded.concluido_por,
		  concluido_em = excluded.concluido_em
	`, cid, sid, u.ID, concluidoEm)
	if err != nil {
		jsonErro(w, http.StatusInternalServerError, err.Error())
		return
	}

	a.st.Auditoria(&u.ID, "concluir_setor", "conferencia_setores", &sid, fmt.Sprintf("conf_id=%d setor_id=%d", cid, sid), ipDe(r))
	jsonOK(w, map[string]any{"ok": true, "status": "concluida", "concluido_em": concluidoEm})
}

// hConferenciaSetorReabrir: reabre a conferência setorial (status = 'em_andamento')
func (a *App) hConferenciaSetorReabrir(w http.ResponseWriter, r *http.Request) {
	u := usuarioDoCtx(r)
	cid, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || cid <= 0 {
		jsonErro(w, http.StatusBadRequest, "id de conferência inválido")
		return
	}
	sid, err := strconv.ParseInt(r.PathValue("setor_id"), 10, 64)
	if err != nil || sid <= 0 {
		jsonErro(w, http.StatusBadRequest, "setor_id inválido")
		return
	}

	var confStatus string
	var confGrupoID int64
	if err := a.st.db.QueryRow(`SELECT status, COALESCE(grupo_id, 0) FROM conferencias WHERE id = ?`, cid).Scan(&confStatus, &confGrupoID); err != nil {
		jsonErro(w, http.StatusNotFound, "conferência não encontrada")
		return
	}
	if confStatus != "aberta" {
		jsonErro(w, http.StatusBadRequest, "conferência já está fechada")
		return
	}
	if esc := escopoDoUsuario(u); esc > 0 && confGrupoID != esc {
		jsonErro(w, http.StatusForbidden, "conferência fora do seu escopo")
		return
	}

	if u.Papel == "chefe_setor" {
		var setorChefe *int64
		if u.SetorID != nil {
			setorChefe = u.SetorID
		} else if u.PessoaID != nil {
			_ = a.st.db.QueryRow(`SELECT setor_id FROM pessoas WHERE id = ?`, *u.PessoaID).Scan(&setorChefe)
		}
		if setorChefe == nil || *setorChefe != sid {
			jsonErro(w, http.StatusForbidden, "chefe de setor só pode reabrir seu próprio setor")
			return
		}
	}

	_, err = a.st.db.Exec(`
		UPDATE conferencia_setores
		SET status = 'em_andamento', concluido_por = NULL, concluido_em = NULL
		WHERE conferencia_id = ? AND setor_id = ?
	`, cid, sid)
	if err != nil {
		jsonErro(w, http.StatusInternalServerError, err.Error())
		return
	}

	a.st.Auditoria(&u.ID, "reabrir_setor", "conferencia_setores", &sid, fmt.Sprintf("conf_id=%d setor_id=%d", cid, sid), ipDe(r))
	jsonOK(w, map[string]any{"ok": true, "status": "em_andamento"})
}

// ====================== ARQUIVO DE CONFERÊNCIAS (ordem Tenente 30/09) ======================
// FECHADAS × ARQUIVADAS: `conferencias.arquivada_em` (migração v18) marca a arquivada.
// Arquivar = botão do gerente (só conferência FECHADA); excluir arquivada = só ADMIN,
// apaga conferência + presenças + comentários ( NUKE pontual do arquivo).

// migração v18 (ordem Tenente 30/09): arquivo de conferências (arquivada_em) +
// tag_id nos comentários (TAGs do catálogo nos comentários da busca individual)
func (s *Store) migrarV18() error {
	var v int
	_ = s.db.QueryRow(`SELECT versao FROM schema_migrations WHERE versao = 18`).Scan(&v)
	if v == 18 {
		return nil
	}
	if !s.colunaExiste("conferencias", "arquivada_em") {
		if _, err := s.db.Exec(`ALTER TABLE conferencias ADD COLUMN arquivada_em TEXT`); err != nil {
			if !strings.Contains(err.Error(), "duplicate column") {
				return fmt.Errorf("migração v18: %w", err)
			}
		}
	}
	if !s.colunaExiste("comentarios", "tag_id") {
		if _, err := s.db.Exec(`ALTER TABLE comentarios ADD COLUMN tag_id INTEGER REFERENCES tags(id)`); err != nil {
			if !strings.Contains(err.Error(), "duplicate column") {
				return fmt.Errorf("migração v18 comentarios: %w", err)
			}
		}
	}
	if _, err := s.db.Exec(`CREATE INDEX IF NOT EXISTS idx_conferencias_arq ON conferencias(arquivada_em)`); err != nil {
		return fmt.Errorf("migração v18 idx: %w", err)
	}
	return s.marcarVersao(18)
}

// hConferenciaArquivar (ordem Tenente 30/09): FECHADA → ARQUIVADA. Gerente do grupo.
func (a *App) hConferenciaArquivar(w http.ResponseWriter, r *http.Request) {
	u := usuarioDoCtx(r)
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		jsonErro(w, http.StatusBadRequest, "id inválido")
		return
	}
	var gid *int64
	var status string
	if e := a.st.db.QueryRow(`SELECT grupo_id, status FROM conferencias WHERE id = ?`, id).Scan(&gid, &status); e != nil {
		jsonErro(w, http.StatusNotFound, "conferência inexistente")
		return
	}
	if esc := escopoDoUsuario(u); esc > 0 {
		if gid == nil || *gid != esc {
			jsonErro(w, http.StatusForbidden, "conferência de outro grupo")
			return
		}
	} else if u.Papel != "admin" && u.Papel != "gerente" {
		jsonErro(w, http.StatusForbidden, "sem acesso")
		return
	}
	if status != "fechada" {
		jsonErro(w, http.StatusConflict, "só conferência FECHADA pode ser arquivada")
		return
	}
	if _, err := a.st.db.Exec(`UPDATE conferencias SET arquivada_em = strftime('%Y-%m-%dT%H:%M:%fZ','now') WHERE id = ?`, id); err != nil {
		jsonErro(w, http.StatusInternalServerError, err.Error())
		return
	}
	a.st.Auditoria(&u.ID, "arquivar", "conferencias", &id, "", ipDe(r))
	a.backupAssincrono("arquivar")
	jsonOK(w, map[string]any{"ok": true, "arquivada": id})
}

// hConferenciaExcluirArquivada (ordem Tenente 30/09): apaga ARQUIVADA — só ADMIN.
// É a única via de destruição de uma conferência fechada (o NUKE de grupo é o outro caso).
func (a *App) hConferenciaExcluirArquivada(w http.ResponseWriter, r *http.Request) {
	u := usuarioDoCtx(r) // a.auth(true) já garantiu papel admin
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		jsonErro(w, http.StatusBadRequest, "id inválido")
		return
	}
	var gid *int64
	var arq *string
	if e := a.st.db.QueryRow(`SELECT grupo_id, arquivada_em FROM conferencias WHERE id = ?`, id).Scan(&gid, &arq); e != nil {
		jsonErro(w, http.StatusNotFound, "conferência inexistente")
		return
	}
	if arq == nil {
		jsonErro(w, http.StatusConflict, "conferência não está arquivada — arquive antes de excluir")
		return
	}
	tx, err := a.st.db.Begin()
	if err != nil {
		jsonErro(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer tx.Rollback()
	for _, q := range []string{
		`DELETE FROM comentarios WHERE conferencia_id = ?`,
		`DELETE FROM presencas WHERE conferencia_id = ?`,
		`DELETE FROM conferencias WHERE id = ?`,
	} {
		if _, e := tx.Exec(q, id); e != nil {
			jsonErro(w, http.StatusInternalServerError, e.Error())
			return
		}
	}
	if err = tx.Commit(); err != nil {
		jsonErro(w, http.StatusInternalServerError, err.Error())
		return
	}
	a.st.Auditoria(&u.ID, "excluir_arquivada", "conferencias", &id, "", ipDe(r))
	a.backupAssincrono("excluir_arquivada")
	jsonOK(w, map[string]any{"ok": true, "excluida": id})
}

// hRegistrosBusca (ordem Tenente 30/09): busca INDIVIDUAL nos relatórios.
// ?pessoa=ID&de=&ate=  → todos os lançamentos da pessoa no período (data, conf, local,
// situação, destino, observação, tags).
// ?filtros=[{"tag":1,"de":"...","ate":"..."},...] → multi-seleção em OU: qualquer regra
// casada entra no resultado (ex.: TAG A na última semana OU TAG B no último mês).
func (a *App) hRegistrosBusca(w http.ResponseWriter, r *http.Request) {
	u := usuarioDoCtx(r)
	escopo := escopoDoUsuario(u)
	de, ate := a.periodoPadrao(r)
	filtrosJSON := r.URL.Query().Get("filtros")
	pessoaQ := r.URL.Query().Get("pessoa")

	cond := []string{"f.status='fechada'", "f.data BETWEEN ? AND ?"}
	args := []any{de, ate}
	if escopo > 0 {
		ft := a.filtroArvore(escopo, "f")
		cond = append(cond, strings.TrimPrefix(ft.clause, " AND "))
		args = append(args, ft.args...)
	}
	var pessoaID int64
	if pessoaQ != "" {
		p, err := strconv.ParseInt(pessoaQ, 10, 64)
		if err != nil || p <= 0 {
			jsonErro(w, http.StatusBadRequest, "pessoa inválida")
			return
		}
		pessoaID = p
		cond = append(cond, "pr.pessoa_id = ?")
		args = append(args, pessoaID)
	}
	where := " WHERE " + strings.Join(cond, " AND ")

	if filtrosJSON != "" {
		// modo filtros: regras (tag × período) em OU; retorna REGISTROS DE COMENTÁRIO
		var regras []struct {
			Tag int64  `json:"tag"`
			De  string `json:"de"`
			Ate string `json:"ate"`
		}
		if err := json.Unmarshal([]byte(filtrosJSON), &regras); err != nil || len(regras) == 0 {
			jsonErro(w, http.StatusBadRequest, "filtros inválidos")
			return
		}
		ou := []string{}
		oargs := []any{}
		for _, rg := range regras {
			if rg.Tag <= 0 || rg.De == "" || rg.Ate == "" {
				jsonErro(w, http.StatusBadRequest, "cada filtro exige tag, de e ate")
				return
			}
			ou = append(ou, "(t.tag_id = ? AND f.data BETWEEN ? AND ?)")
			oargs = append(oargs, rg.Tag, rg.De, rg.Ate)
		}
		q := `
		SELECT f.data, f.id, p.nome_guerra, COALESCE(tt.nome,''), t.comentario,
		       COALESCE(NULLIF(u.nome_guerra,''), u.login), t.criado_em, t.tag_id
		FROM comentarios t
		JOIN tags tt ON tt.id = t.tag_id
		JOIN conferencias f ON f.id = t.conferencia_id
		JOIN pessoas p ON p.id = t.pessoa_id
		JOIN usuarios u ON u.id = t.operador_id
		WHERE ( ` + strings.Join(ou, " OR ") + ` ) AND f.status='fechada'`
		qargs := oargs
		if escopo > 0 {
			ft := a.filtroArvore(escopo, "f")
			q += ft.clause
			qargs = append(qargs, ft.args...)
		}
		q += ` ORDER BY f.data DESC, f.id DESC, t.ordem DESC LIMIT 400`
		rows, err := a.st.db.Query(q, qargs...)
		if err != nil {
			jsonErro(w, http.StatusInternalServerError, err.Error())
			return
		}
		defer rows.Close()
		out := []map[string]any{}
		for rows.Next() {
			var data, nome, tag, comentario, por, em string
			var confID, tagID int64
			if rows.Scan(&data, &confID, &nome, &tag, &comentario, &por, &em, &tagID) == nil {
				out = append(out, map[string]any{
					"data": data, "conferencia_id": confID, "pessoa": nome, "tag": tag,
					"tag_id": tagID, "comentario": comentario, "operador": por,
					"datahora": em, "tipo": "comentario",
				})
			}
		}
		jsonOK(w, map[string]any{"registros": out, "modo": "filtros"})
		return
	}

	// modo padrão: lançamentos (com a TAG associada) da pessoa no período
	q := `
	SELECT f.data, f.id, COALESCE(f.local,''), pr.situacao, COALESCE(d.nome,''),
	       COALESCE(pr.observacao,''), COALESCE(tg.nome,''), pr.id
	FROM presencas pr
	JOIN conferencias f ON f.id = pr.conferencia_id
	LEFT JOIN destinos d ON d.id = pr.destino_id
	LEFT JOIN tags tg ON tg.id = pr.tag_id` + where + `
	ORDER BY f.data DESC, f.id DESC LIMIT 400`
	if pessoaID == 0 {
		jsonErro(w, http.StatusBadRequest, "informe pessoa=ID (ou filtros=...)")
		return
	}
	rows, err := a.st.db.Query(q, args...)
	if err != nil {
		jsonErro(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer rows.Close()
	out := []map[string]any{}
	for rows.Next() {
		var data, local, sit, destino, obs string
		var tags *string
		var confID, prID int64
		if rows.Scan(&data, &confID, &local, &sit, &destino, &obs, &tags, &prID) == nil {
			tagsOut := ""
			if tags != nil {
				tagsOut = *tags
			}
			out = append(out, map[string]any{
				"data": data, "conferencia_id": confID, "local": local, "situacao": sit,
				"destino": destino, "observacao": obs, "tags": tagsOut, "presenca_id": prID,
			})
		}
	}
	// ficha mínima da pessoa para o modal
	var nome, nomeCompleto string
	var setor, funcao *string
	_ = a.st.db.QueryRow(`SELECT p.nome_guerra, COALESCE(p.nome_completo,''),
		s.nome, fu.nome FROM pessoas p
		LEFT JOIN setores s ON s.id = p.setor_id
		LEFT JOIN funcoes fu ON fu.id = p.funcao_id WHERE p.id = ?`, pessoaID).
		Scan(&nome, &nomeCompleto, &setor, &funcao)
	ficha := map[string]any{"id": pessoaID, "nome_guerra": nome, "nome_completo": nomeCompleto,
		"setor": setor, "funcao": funcao}
	jsonOK(w, map[string]any{"pessoa": ficha, "registros": out, "de": de, "ate": ate})
}

// hTagsDisponiveis: TAGs visíveis no escopo (próprias + herdadas) para montar os filtros.
func (a *App) hTagsDisponiveis(w http.ResponseWriter, r *http.Request) {
	u := usuarioDoCtx(r)
	escopo := escopoDoUsuario(u)
	var rows *sql.Rows
	var err error
	if escopo > 0 {
		ids := append([]int64{escopo}, a.gruposSuperioresAtivos(escopo)...)
		marks := strings.TrimRight(strings.Repeat("?,", len(ids)), ",")
		args := make([]any, len(ids))
		for i, v := range ids {
			args[i] = v
		}
		rows, err = a.st.db.Query(`SELECT id, nome FROM tags WHERE ativo = 1 AND grupo_id IN (`+marks+`) ORDER BY nome`, args...)
	} else {
		rows, err = a.st.db.Query(`SELECT id, nome FROM tags WHERE ativo = 1 ORDER BY nome`)
	}
	if err != nil {
		jsonErro(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer rows.Close()
	out := []map[string]any{}
	for rows.Next() {
		var id int64
		var nome string
		if rows.Scan(&id, &nome) == nil {
			out = append(out, map[string]any{"id": id, "nome": nome})
		}
	}
	jsonOK(w, out)
}

// hPessoaFicha (ordem Tenente 30/09): dados pessoais para o modal da busca individual.
func (a *App) hPessoaFicha(w http.ResponseWriter, r *http.Request) {
	u := usuarioDoCtx(r)
	escopo := escopoDoUsuario(u)
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		jsonErro(w, http.StatusBadRequest, "id inválido")
		return
	}
	q := `SELECT p.id, p.nome_guerra, COALESCE(p.nome_completo,''),
		COALESCE(s.nome,'INDEFINIDO'), COALESCE(fu.nome,'INDEFINIDO'), COALESCE(g.nome,'—'),
		p.status
		FROM pessoas p
		LEFT JOIN setores s ON s.id = p.setor_id
		LEFT JOIN funcoes fu ON fu.id = p.funcao_id
		LEFT JOIN grupos g ON g.id = p.grupo_id
		WHERE p.id = ?`
	if escopo > 0 {
		q += ` AND p.grupo_id = ?`
	}
	args := []any{id}
	if escopo > 0 {
		args = append(args, escopo)
	}
	var fid int64
	var ng, nc, setor, funcao, grupo, status string
	if e := a.st.db.QueryRow(q, args...).Scan(&fid, &ng, &nc, &setor, &funcao, &grupo, &status); e != nil {
		jsonErro(w, http.StatusNotFound, "pessoa não encontrada no seu escopo")
		return
	}
	jsonOK(w, map[string]any{"id": fid, "nome_guerra": ng, "nome_completo": nc,
		"setor": setor, "funcao": funcao, "grupo": grupo, "status": status})
}

// hPessoaComentarios: histórico completo de comentários de uma pessoa (para a ficha individual).
func (a *App) hPessoaComentarios(w http.ResponseWriter, r *http.Request) {
	u := usuarioDoCtx(r)
	escopo := escopoDoUsuario(u)
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		jsonErro(w, http.StatusBadRequest, "id inválido")
		return
	}
	// Validação de escopo: o usuário só vê comentários de quem ele enxerga na ficha
	if escopo > 0 {
		var check int
		if e := a.st.db.QueryRow(`SELECT 1 FROM pessoas WHERE id = ? AND grupo_id = ?`, id, escopo).Scan(&check); e != nil {
			jsonErro(w, http.StatusNotFound, "pessoa não encontrada no seu escopo")
			return
		}
	}
	q := `
		SELECT c.id, c.conferencia_id, COALESCE(t.nome,''), c.comentario,
		       COALESCE(NULLIF(u.nome_guerra,''), u.login), c.criado_em
		FROM comentarios c
		LEFT JOIN tags t ON t.id = c.tag_id
		LEFT JOIN usuarios u ON u.id = c.operador_id
		WHERE c.pessoa_id = ?
		ORDER BY c.criado_em DESC, c.ordem DESC`
	rows, err := a.st.db.Query(q, id)
	if err != nil {
		jsonErro(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer rows.Close()
	out := []map[string]any{}
	for rows.Next() {
		var cid, confID int64
		var tag, comentario, op, data string
		if rows.Scan(&cid, &confID, &tag, &comentario, &op, &data) == nil {
			out = append(out, map[string]any{
				"id": cid, "conferencia_id": confID, "tag": tag,
				"comentario": comentario, "operador": op, "datahora": data,
			})
		}
	}
	jsonOK(w, out)
}

// hConferenciaList: conferências DO ESCOPO (grupo não vê grupo; admin vê todas).
func (a *App) hConferenciaList(w http.ResponseWriter, r *http.Request) {
	u := usuarioDoCtx(r)
	escopo := escopoDoUsuario(u)
	q := `
		SELECT c.id, c.data, COALESCE(c.hora,''), COALESCE(c.local,''), c.status,
		       COALESCE(NULLIF(u.nome_guerra,''), u.login), c.criado_em, c.fechada_em,
		       (SELECT COUNT(*) FROM presencas p WHERE p.conferencia_id = c.id) AS lanc,
		       COALESCE(c.grupo_id,0), COALESCE((SELECT g.nome FROM grupos g WHERE g.id = c.grupo_id),'—'),
		       c.arquivada_em,
		       COALESCE(c.nome,''), COALESCE(c.prazo_final,''),
		       COALESCE((SELECT COALESCE(NULLIF(u2.nome_completo,''), NULLIF(u2.nome_guerra,''), u2.login)
		                 FROM usuarios u2 WHERE u2.id = c.encarregado_usuario_id),'')
		FROM conferencias c
		LEFT JOIN usuarios u ON u.id = c.criado_por`
	var rows *sql.Rows
	var err error
	// ordem Tenente 30/09: aba CONFERÊNCIAS tem filtro de período (padrão: últimos 7 dias);
	// arquivadas vêm SÓ na aba ARQUIVO (?arq=1, sem limite de período)
	if r.URL.Query().Get("arq") == "1" {
		if escopo > 0 {
			rows, err = a.st.db.Query(q+` WHERE c.grupo_id = ? AND c.arquivada_em IS NOT NULL
				ORDER BY c.data DESC, c.id DESC`, escopo)
		} else {
			rows, err = a.st.db.Query(q + ` WHERE c.arquivada_em IS NOT NULL
				ORDER BY c.data DESC, c.id DESC`)
		}
	} else {
		de, ate := a.periodoPadrao(r)
		if escopo > 0 {
			rows, err = a.st.db.Query(q+` WHERE c.grupo_id = ? AND c.arquivada_em IS NULL
				AND c.data BETWEEN ? AND ?
				ORDER BY c.data DESC, c.id DESC`, escopo, de, ate)
		} else {
			rows, err = a.st.db.Query(q+` WHERE c.arquivada_em IS NULL
				AND c.data BETWEEN ? AND ?
				ORDER BY c.data DESC, c.id DESC`, de, ate)
		}
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
		var arquivada *string
		var nome, prazo, encarregado string
		if rows.Scan(&id, &data, &hora, &local, &status, &criado, &criadaEm, &fechada, &lanc, &grupoID, &grupoNome, &arquivada, &nome, &prazo, &encarregado) == nil {
			out = append(out, map[string]any{
				"id": id, "data": data, "hora": hora, "local": local, "status": status,
				"criado_por": criado, "criada_em": criadaEm, "fechada_em": fechada, "lancamentos": lanc,
				"grupo_id": grupoID, "grupo": grupoNome, "arquivada_em": arquivada,
				"nome": nome, "prazo_final": prazo, "encarregado_nome": encarregado,
			})
		}
	}
	jsonOK(w, out)
}

// hConferenciaDescartar (v9.14.4, ordem Tenente 29/09): DESCARTAR conferência ABERTA —
// apaga a conferência, presenças parciais e comentários dela. Conferência FECHADA é
// histórico imutável → 409. Escopo: só o grupo dono.
func (a *App) hConferenciaDescartar(w http.ResponseWriter, r *http.Request) {
	u := usuarioDoCtx(r)
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		jsonErro(w, http.StatusBadRequest, "id inválido")
		return
	}
	var gid *int64
	var status string
	if e := a.st.db.QueryRow(`SELECT grupo_id, status FROM conferencias WHERE id = ?`, id).Scan(&gid, &status); e != nil {
		jsonErro(w, http.StatusNotFound, "conferência inexistente")
		return
	}
	if esc := escopoDoUsuario(u); esc > 0 {
		if gid == nil || *gid != esc {
			jsonErro(w, http.StatusForbidden, "conferência de outro grupo")
			return
		}
	} else if u.Papel != "admin" {
		jsonErro(w, http.StatusForbidden, "sem acesso")
		return
	}
	if status != "aberta" {
		jsonErro(w, http.StatusConflict, "conferência fechada é histórico — não pode ser descartada")
		return
	}
	tx, err := a.st.db.Begin()
	if err != nil {
		jsonErro(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer tx.Rollback()
	for _, q := range []string{
		`DELETE FROM comentarios WHERE conferencia_id = ?`,
		`DELETE FROM presencas WHERE conferencia_id = ?`,
		`DELETE FROM conferencias WHERE id = ?`,
	} {
		if _, e := tx.Exec(q, id); e != nil {
			jsonErro(w, http.StatusInternalServerError, e.Error())
			return
		}
	}
	if err = tx.Commit(); err != nil {
		jsonErro(w, http.StatusInternalServerError, err.Error())
		return
	}
	a.st.Auditoria(&u.ID, "descartar", "conferencias", &id, "conferência aberta descartada", ipDe(r))
	jsonOK(w, map[string]any{"ok": true, "descartada": id})
}

// hCatalogoReparentar (v9.16, ordem Tenente 29/09): hierarquia via DRAG & DROP — muda o pai
// de um item de catálogo. Regras: só gerente; item e novo pai do grupo do gerente (herdados
// de cima não se movem); pai não pode ser descendente do item (anti-ciclo); profundidade ≤ 8.
func (a *App) hCatalogoReparentar(w http.ResponseWriter, r *http.Request) {
	t, err := tabelaDeCatalogo(r.PathValue("t"))
	if err != nil {
		jsonErro(w, http.StatusBadRequest, err.Error())
		return
	}
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		jsonErro(w, http.StatusBadRequest, "id inválido")
		return
	}
	u := usuarioDoCtx(r)
	if u == nil || u.Papel != "gerente" {
		jsonErro(w, http.StatusForbidden, "gestão de catálogos é exclusiva do gerente")
		return
	}
	esc := escopoDoUsuario(u)
	var req struct {
		PaiID       *int64 `json:"pai_id"`
		Antiguidade *int   `json:"antiguidade"`
	}
	if err = decodificar(r, &req); err != nil {
		jsonErro(w, http.StatusBadRequest, "JSON inválido")
		return
	}
	// item precisa ser do grupo do gerente (dono)
	var donoGrupo int64
	if e := a.st.db.QueryRow(`SELECT COALESCE(grupo_id,0) FROM `+t+` WHERE id = ?`, id).Scan(&donoGrupo); e != nil {
		jsonErro(w, http.StatusNotFound, "item inexistente")
		return
	}
	if esc > 0 && donoGrupo != esc {
		jsonErro(w, http.StatusForbidden, "item herdado de grupo superior — não pode ser movido")
		return
	}
	if req.PaiID != nil {
		// pai válido: existe, do mesmo grupo, não é o próprio item
		if *req.PaiID == id {
			jsonErro(w, http.StatusBadRequest, "um item não pode ser pai de si mesmo")
			return
		}
		var paiDono int64
		if e := a.st.db.QueryRow(`SELECT COALESCE(grupo_id,0) FROM `+t+` WHERE id = ?`, *req.PaiID).Scan(&paiDono); e != nil || paiDono != donoGrupo {
			jsonErro(w, http.StatusForbidden, "pai de outro grupo")
			return
		}
		// anti-ciclo: subir a ancestralidade do novo pai; se encontrar o item → ciclo
		pai := *req.PaiID
		prof := 0
		for pai != 0 && prof < 16 {
			var avo *int64
			if e := a.st.db.QueryRow(`SELECT pai_id FROM `+t+` WHERE id = ?`, pai).Scan(&avo); e != nil {
				break
			}
			if avo != nil && *avo == id {
				jsonErro(w, http.StatusBadRequest, "não é possível: o item é ancestral do novo pai (ciclo)")
				return
			}
			if avo == nil {
				break
			}
			pai = *avo
			prof++
		}
		// profundidade resultante do item ≤ 8
		prof = 1
		pai = *req.PaiID
		for pai != 0 && prof <= 8 {
			var avo *int64
			if e := a.st.db.QueryRow(`SELECT pai_id FROM `+t+` WHERE id = ?`, pai).Scan(&avo); e != nil || avo == nil {
				break
			}
			pai = *avo
			prof++
		}
		if prof > 8 {
			jsonErro(w, http.StatusBadRequest, "profundidade máxima da hierarquia é 8")
			return
		}
	}
	if req.Antiguidade != nil {
		if _, err = a.st.db.Exec(`UPDATE `+t+` SET antiguidade = ? WHERE id = ?`, *req.Antiguidade, id); err != nil {
			jsonErro(w, http.StatusInternalServerError, err.Error())
			return
		}
		a.st.Auditoria(&u.ID, "antiguidade", t, &id, fmt.Sprintf("posição %d", *req.Antiguidade), ipDe(r))
		jsonOK(w, map[string]any{"ok": true, "id": id, "antiguidade": *req.Antiguidade})
		return
	}
	if _, err = a.st.db.Exec(`UPDATE `+t+` SET pai_id = ? WHERE id = ?`, req.PaiID, id); err != nil {
		jsonErro(w, http.StatusInternalServerError, err.Error())
		return
	}
	a.st.Auditoria(&u.ID, "reparentar", t, &id, "novo pai", ipDe(r))
	jsonOK(w, map[string]any{"ok": true, "id": id, "pai_id": req.PaiID})
}

// hCatalogoEditar (v9.16.11, ordem Tenente 29/09): EDITAR item do catálogo (nome, sigla, cor).
// Só gerente; item do próprio grupo (herdados são read only).
func (a *App) hCatalogoEditar(w http.ResponseWriter, r *http.Request) {
	t, err := tabelaDeCatalogo(r.PathValue("t"))
	if err != nil {
		jsonErro(w, http.StatusBadRequest, err.Error())
		return
	}
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		jsonErro(w, http.StatusBadRequest, "id inválido")
		return
	}
	u := usuarioDoCtx(r)
	if u == nil || (u.Papel != "gerente" && u.Papel != "admin") {
		jsonErro(w, http.StatusForbidden, "gestão de catálogos é exclusiva do gerente ou administrador")
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
	if esc := escopoDoUsuario(u); esc > 0 {
		var donoGrupo int64
		if e := a.st.db.QueryRow(`SELECT COALESCE(grupo_id,0) FROM `+t+` WHERE id = ?`, id).Scan(&donoGrupo); e != nil {
			jsonErro(w, http.StatusNotFound, "item não encontrado")
			return
		}
		subordinados := a.gruposSubordinadosAtivos(esc)
		ehSubordinado := false
		for _, sub := range subordinados {
			if sub == donoGrupo {
				ehSubordinado = true
				break
			}
		}
		if donoGrupo != esc && !ehSubordinado {
			jsonErro(w, http.StatusForbidden, "item herdado de grupo superior ou global — somente leitura")
			return
		}
	}
	switch t {
	case "setores":
		_, err = a.st.db.Exec(`UPDATE setores SET nome = ?, sigla = NULLIF(?,'') WHERE id = ?`, nome, req.Sigla, id)
	case "tags":
		_, err = a.st.db.Exec(`UPDATE tags SET nome = ?, cor = NULLIF(?,'') WHERE id = ?`, nome, req.Cor, id)
	default:
		_, err = a.st.db.Exec(`UPDATE `+t+` SET nome = ? WHERE id = ?`, nome, id)
	}
	if err != nil {
		jsonErro(w, http.StatusBadRequest, "não atualizado (duplicado?): "+err.Error())
		return
	}
	a.st.Auditoria(&u.ID, "editar", t, &id, nome, ipDe(r))
	jsonOK(w, map[string]any{"ok": true, "id": id, "nome": nome})
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
		SELECT p.nome_guerra, COALESCE(s.nome,'INDEFINIDO'), pr.situacao,
		       COALESCE(d.nome,''), COALESCE(pr.observacao,''), COALESCE(u.login,''), COALESCE(pr.marcado_em,''),
		       COALESCE(NULLIF(fu.nome,''), COALESCE(NULLIF(fu_u.nome,''), COALESCE(NULLIF(fu_up.nome,''), '—'))),
		       COALESCE(pr.verificado, 0)
		FROM presencas pr
		JOIN pessoas p ON p.id = pr.pessoa_id
		LEFT JOIN setores s ON s.id = p.setor_id
		LEFT JOIN funcoes fu ON fu.id = p.funcao_id
		LEFT JOIN usuarios u2 ON u2.pessoa_id = p.id
		LEFT JOIN funcoes fu_u ON fu_u.id = u2.funcao_id
		LEFT JOIN usuario_papeis up2 ON up2.usuario_id = u2.id AND (up2.grupo_id = p.grupo_id OR up2.grupo_id IS NULL)
		LEFT JOIN funcoes fu_up ON fu_up.id = up2.funcao_id
		LEFT JOIN destinos d ON d.id = pr.destino_id
		LEFT JOIN usuarios u ON u.id = pr.marcado_por
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
		var ng, setor, sit, destino, obs, por, em, funcao string
		var verificado int
		if rows.Scan(&ng, &setor, &sit, &destino, &obs, &por, &em, &funcao, &verificado) == nil {
			lanc = append(lanc, map[string]any{
				"nome_guerra": ng, "funcao": funcao, "setor": setor, "situacao": sit,
				"destino": destino, "observacao": obs, "marcado_por": por, "marcado_em": em,
				"verificado": verificado == 1,
			})
			cont[sit]++
		}
	}
	cMap := map[string]any{
		"id": id, "data": data, "status": status, "hora": hora, "local": local,
		"criada_em": criadaEm, "fechada_em": fechada, "criado_por": criadoPor,
	}
	res := map[string]any{
		"id": id, "data": data, "status": status, "hora": hora, "local": local,
		"criada_em": criadaEm, "fechada_em": fechada, "criado_por": criadoPor,
		"conferencia": cMap,
		"lancamentos": lanc,
		"presencas":   lanc,
		"resumo": map[string]any{
			"presentes": cont["presente"], "atrasos": cont["atraso"],
			"faltas": cont["falta"], "justificadas": cont["justificada"],
		},
	}
	jsonOK(w, res)
}

func (a *App) montarLancamentosPDFConferencia(id int64, filtro string) ([]map[string]any, map[string]int, error) {
	rows, e := a.st.db.Query(`
		WITH RECURSIVE cam_setor(id, caminho) AS (
		  SELECT id, nome FROM setores WHERE pai_id IS NULL
		  UNION ALL
		  SELECT f.id, cs.caminho || ' > ' || f.nome FROM setores f JOIN cam_setor cs ON f.pai_id = cs.id
		),
		cam_funcao(id, caminho) AS (
		  SELECT id, nome FROM funcoes WHERE pai_id IS NULL
		  UNION ALL
		  SELECT f.id, cf.caminho || ' > ' || f.nome FROM funcoes f JOIN cam_funcao cf ON f.pai_id = cf.id
		)
		SELECT p.nome_guerra, COALESCE(NULLIF(s.sigla,''), s.nome, 'INDEFINIDO'), pr.situacao,
		       CASE WHEN pr.situacao = 'justificada' THEN COALESCE(d.nome,'') ELSE '' END AS destino, COALESCE(pr.observacao,''), u.login,
		       COALESCE(NULLIF(fu.nome,''), COALESCE(NULLIF(fu_u.nome,''), COALESCE(NULLIF(fu_up.nome,''), '—'))),
		       COALESCE(cf2.caminho,'~sem função'), COALESCE(cs2.caminho,'~sem setor')
		FROM presencas pr
		JOIN pessoas p ON p.id = pr.pessoa_id
		LEFT JOIN setores s ON s.id = p.setor_id
		LEFT JOIN cam_setor cs2 ON cs2.id = s.id
		LEFT JOIN funcoes fu ON fu.id = p.funcao_id
		LEFT JOIN cam_funcao cf2 ON cf2.id = fu.id
		LEFT JOIN usuarios u2 ON u2.pessoa_id = p.id
		LEFT JOIN funcoes fu_u ON fu_u.id = u2.funcao_id
		LEFT JOIN usuario_papeis up2 ON up2.usuario_id = u2.id AND (up2.grupo_id = p.grupo_id OR up2.grupo_id IS NULL)
		LEFT JOIN funcoes fu_up ON fu_up.id = up2.funcao_id
		LEFT JOIN destinos d ON d.id = pr.destino_id
		JOIN usuarios u ON u.id = pr.marcado_por
		WHERE pr.conferencia_id = ?
		ORDER BY COALESCE(cf2.caminho,'~sem função'), COALESCE(cs2.caminho,'~sem setor'), p.nome_guerra COLLATE NOCASE`, id)
	if e != nil {
		return nil, nil, e
	}
	defer rows.Close()
	lanc := []map[string]any{}
	resumo := map[string]int{"presentes": 0, "atrasos": 0, "faltas": 0, "justificadas": 0, "nao_verificados": 0}
	ord := 0
	for rows.Next() {
		var ng, setor, sit, destino, obs, por, funcao, camF, camS string
		if rows.Scan(&ng, &setor, &sit, &destino, &obs, &por, &funcao, &camF, &camS) == nil {
			switch sit {
			case "presente":
				resumo["presentes"]++
			case "atraso":
				resumo["atrasos"]++
			case "falta":
				resumo["faltas"]++
			case "nao_verificado":
				resumo["nao_verificados"]++
			case "justificada":
				resumo["justificadas"]++
			}
			if filtro == "faltas" && sit != "falta" {
				continue
			}
			if filtro == "justificados" && sit != "justificada" {
				continue
			}
			ord++
			lanc = append(lanc, map[string]any{
				"ord": ord, "nome_guerra": ng, "funcao": funcao, "setor": setor, "situacao": sit,
				"destino": destino, "observacao": obs,
			})
		}
	}
	return lanc, resumo, nil
}

// hConferenciaPDF: relatório próprio — SOMENTE de conferência fechada (ordem Tenente 28/09).
func (a *App) hConferenciaPDF(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		jsonErro(w, http.StatusBadRequest, "id inválido")
		return
	}
	filtro := r.URL.Query().Get("filtro")
	if filtro != "" && filtro != "todos" && filtro != "faltas" && filtro != "justificados" {
		jsonErro(w, http.StatusBadRequest, "filtro inválido (todos|faltas|justificados)")
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
	lanc, resumo, e := a.montarLancamentosPDFConferencia(id, filtro)
	if e != nil {
		jsonErro(w, http.StatusInternalServerError, e.Error())
		return
	}
	u := usuarioDoCtx(r)
	selo := ""
	if filtro == "faltas" {
		selo = "_SO_FALTAS"
	} else if filtro == "justificados" {
		selo = "_SO_JUSTIFICADOS"
	}
	var fechadoPorNome string
	_ = a.st.db.QueryRow(`
		SELECT COALESCE(NULLIF(u.nome_completo,''), NULLIF(u.nome_guerra,''), u.login)
		FROM conferencias c
		JOIN usuarios u ON u.id = COALESCE(c.fechada_por, c.criado_por)
		WHERE c.id = ?`, id).Scan(&fechadoPorNome)

	// "Iniciada por" também por NOME COMPLETO (ordem Diretor 04/10): o login
	// nunca aparece no PDF, nem no banner nem na assinatura.
	var criadoPorNome string
	_ = a.st.db.QueryRow(`
		SELECT COALESCE(NULLIF(u.nome_completo,''), NULLIF(u.nome_guerra,''), u.login)
		FROM conferencias c
		JOIN usuarios u ON u.id = c.criado_por
		WHERE c.id = ?`, id).Scan(&criadoPorNome)

	// Assinatura por NOME COMPLETO (ordem Diretor 04/10): o PDF nunca mostra o
	// login — quem assina é identificado pelo nome completo (fallback nome de
	// guerra) e, quando houver, a função específica do grupo logo abaixo.
	var quemAssina string
	_ = a.st.db.QueryRow(`
		SELECT COALESCE(NULLIF(u2.nome_completo,''), NULLIF(u2.nome_guerra,''), u2.login)
		FROM usuarios u2 WHERE u2.id = ?`, u.ID).Scan(&quemAssina)
	if strings.TrimSpace(quemAssina) == "" {
		quemAssina = u.Login
	}
	var funcaoAssina string
	_ = a.st.db.QueryRow(`
		SELECT f.nome FROM usuarios u2
		JOIN funcoes f ON f.id = u2.funcao_id AND f.grupo_id IS NOT DISTINCT FROM u2.grupo_id
		WHERE u2.id = ?`, u.ID).Scan(&funcaoAssina)

	cp := ConferenciaPDF{
		ID: id, Data: data, Status: status, CriadaEm: criadaEm, FechadaEm: fechada,
		CriadoPor: criadoPorNome, GeradoPor: quemAssina, Resumo: resumo, Lancamentos: lanc,
		Filtro: filtro, FechadoPorNome: fechadoPorNome, FuncaoGeradoPor: funcaoAssina,
	}
	pdf, err := a.gerarConferenciaPDF(cp)
	if err != nil {
		jsonErro(w, http.StatusInternalServerError, "falha ao gerar PDF: "+err.Error())
		return
	}
	a.st.Auditoria(&u.ID, "exportar", "conferencia", &id, "pdf", ipDe(r))
	w.Header().Set("Cache-Control", "no-store, no-cache, must-revalidate")
	w.Header().Set("Pragma", "no-cache")
	w.Header().Set("Content-Type", "application/pdf")
	w.Header().Set("Content-Disposition",
		fmt.Sprintf("inline; filename=SCI_conferencia_%s_%d%s.pdf", data, id, selo))
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
		WITH ultima_presenca AS (
		  SELECT p.situacao,
		         ROW_NUMBER() OVER(PARTITION BY p.pessoa_id ORDER BY f.data DESC, f.hora DESC, f.id DESC) as rn
		  FROM presencas p JOIN conferencias f ON f.id = p.conferencia_id`+filtro+`
		)
		SELECT COUNT(*),
		       COALESCE(SUM(situacao='presente'),0), COALESCE(SUM(situacao='atraso'),0),
		       COALESCE(SUM(situacao='falta'),0), COALESCE(SUM(situacao='justificada'),0),
		       COALESCE(SUM(situacao='nao_verificado'),0)
		FROM ultima_presenca WHERE rn = 1`,
		args...).Scan(&b.TotalLanc, &b.Presentes, &b.Atrasos, &b.Faltas, &b.Justificadas, &b.NaoVerificados)
	// "efetivo pronto" = presentes SEM ressalva (ordem do Tenente, 28/09)
	// "efetivo pronto" = presentes SEM ressalva (ordem do Tenente, 28/09)
	_ = a.st.db.QueryRow(`
		WITH ultima_presenca AS (
		  SELECT p.situacao, p.observacao,
		         ROW_NUMBER() OVER(PARTITION BY p.pessoa_id ORDER BY f.data DESC, f.hora DESC, f.id DESC) as rn
		  FROM presencas p JOIN conferencias f ON f.id = p.conferencia_id`+filtro+`
		)
		SELECT COALESCE(SUM(situacao='presente' AND (observacao IS NULL OR observacao = '')),0)
		FROM ultima_presenca WHERE rn = 1`,
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
	b.TotalFaltas = b.Faltas + b.Justificadas + b.NaoVerificados // ordem Tenente 30/09: NÃO VERIFICADO pune como falta
	if denom > 0 {
		b.PctGeral = round1(100 * float64(validas) / float64(denom))
		b.PctPronto = round1(100 * float64(b.PresentesPuros) / float64(denom))
		b.PctPresencaEstrita = round1(100 * float64(b.PresentesPuros) / float64(denom))
	}

	rows, err := a.st.db.Query(`
		WITH ultima_presenca AS (
		  SELECT pr.situacao, p.setor_id,
		         ROW_NUMBER() OVER(PARTITION BY pr.pessoa_id ORDER BY f.data DESC, f.hora DESC, f.id DESC) as rn
		  FROM presencas pr
		  JOIN pessoas p ON p.id = pr.pessoa_id
		  JOIN conferencias f ON f.id = pr.conferencia_id
		  WHERE f.status='fechada' AND f.data BETWEEN ? AND ?`+a.clSetor(escopo)+`
		)
		SELECT COALESCE(s.nome,'INDEFINIDO') AS setor_nome,
		       COALESCE(SUM(upr.situacao IN ('presente','atraso')),0) AS pres,
		       COALESCE(SUM(upr.situacao='falta'),0) AS faltas,
		       COALESCE(SUM(upr.situacao='justificada'),0) AS just,
		       COUNT(upr.situacao) AS lanc
		FROM ultima_presenca upr
		LEFT JOIN setores s ON s.id = upr.setor_id
		WHERE upr.rn = 1
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
		WITH ultima_presenca AS (
		  SELECT pr.situacao, pr.destino_id,
		         ROW_NUMBER() OVER(PARTITION BY pr.pessoa_id ORDER BY f.data DESC, f.hora DESC, f.id DESC) as rn
		  FROM presencas pr
		  JOIN conferencias f ON f.id = pr.conferencia_id
		  WHERE f.status='fechada' AND f.data BETWEEN ? AND ?`+a.clSetor(escopo)+`
		)
		SELECT COALESCE(d.nome,'(sem destino)'), COUNT(*)
		FROM ultima_presenca upr
		LEFT JOIN destinos d ON d.id = upr.destino_id
		WHERE upr.rn = 1 AND upr.situacao IN ('falta','justificada')
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
	// v9.7 (ordem Tenente): relatórios organizados por ANTIGUIDADE DE FUNÇÃO
	// (funcao_id menor = mais antigo), depois alfabetica. ID da função visível no relatório.
	// v9.11.1 (ordem Tenente 29/09): efetivo do relatório em ORDEM ALFABÉTICA (por hora).
	// v9.17 (ordem Tenente 29/09): TODOS os relatórios ordenam por HIERARQUIA DE SETOR
	// (árvore pai>filho, raiz antes, alfabético por nível), depois HIERARQUIA DE FUNÇÃO
	// (mesma regra), depois nome de guerra. Caminho construído com CTE recursiva.
	rows, err = a.st.db.Query(`
		WITH RECURSIVE cam_setor(id, caminho) AS (
		  SELECT id, nome FROM setores WHERE pai_id IS NULL
		  UNION ALL
		  SELECT f.id, cs.caminho || ' > ' || f.nome FROM setores f JOIN cam_setor cs ON f.pai_id = cs.id
		),
		cam_funcao(id, caminho) AS (
		  SELECT id, nome FROM funcoes WHERE pai_id IS NULL
		  UNION ALL
		  SELECT f.id, cf.caminho || ' > ' || f.nome FROM funcoes f JOIN cam_funcao cf ON f.pai_id = cf.id
		),
		ultima_presenca AS (
		  SELECT pr.pessoa_id, pr.situacao,
		         ROW_NUMBER() OVER(PARTITION BY pr.pessoa_id ORDER BY f.data DESC, f.hora DESC, f.id DESC) as rn
		  FROM presencas pr
		  JOIN conferencias f ON f.id = pr.conferencia_id
		  WHERE f.status='fechada' AND f.data BETWEEN ? AND ? `+ftP2.clause+`
		)
		SELECT p.id, p.nome_guerra, COALESCE(s.nome,'INDEFINIDO'),
		       CASE WHEN upr.situacao IN ('presente','atraso') THEN 1 ELSE 0 END,
		       CASE WHEN upr.situacao = 'atraso' THEN 1 ELSE 0 END,
		       CASE WHEN upr.situacao = 'falta' THEN 1 ELSE 0 END,
		       CASE WHEN upr.situacao = 'justificada' THEN 1 ELSE 0 END,
		       CASE WHEN upr.situacao = 'nao_verificado' THEN 1 ELSE 0 END,
		       CASE WHEN upr.situacao IS NOT NULL THEN 1 ELSE 0 END,
		       COALESCE(p.funcao_id, u2.funcao_id, up2.funcao_id),
		       COALESCE(NULLIF(fu.nome,''), COALESCE(NULLIF(fu_u.nome,''), COALESCE(NULLIF(fu_up.nome,''), ''))),
		       ROW_NUMBER() OVER (ORDER BY COALESCE(cf2.caminho,'~sem função'),
		                                  COALESCE(cs2.caminho,'~sem setor'),
		                                  p.nome_guerra COLLATE NOCASE) AS antig,
		       COALESCE(g2.nome,'—')
		FROM pessoas p
		LEFT JOIN setores s ON s.id = p.setor_id
		LEFT JOIN cam_setor cs2 ON cs2.id = s.id
		LEFT JOIN funcoes fu ON fu.id = p.funcao_id
		LEFT JOIN cam_funcao cf2 ON cf2.id = fu.id
		LEFT JOIN usuarios u2 ON u2.pessoa_id = p.id
		LEFT JOIN funcoes fu_u ON fu_u.id = u2.funcao_id
		LEFT JOIN usuario_papeis up2 ON up2.usuario_id = u2.id AND (up2.grupo_id = p.grupo_id OR up2.grupo_id IS NULL)
		LEFT JOIN funcoes fu_up ON fu_up.id = up2.funcao_id
		LEFT JOIN grupos g2 ON g2.id = p.grupo_id
		LEFT JOIN ultima_presenca upr ON upr.pessoa_id = p.id AND upr.rn = 1
		WHERE p.status='ativo'`+a.filtroArvore(escopo, "p").clause+`
		ORDER BY COALESCE(cf2.caminho,'~sem função'),
		         COALESCE(cs2.caminho,'~sem setor'),
		         p.nome_guerra COLLATE NOCASE`,
		append(append([]any{de, ate}, ftP2.args...), a.filtroArvore(escopo, "p").args...)...)
	if err == nil {
		for rows.Next() {
			var r PessoaStat
			var funcaoID *int64
			var antig int
			if rows.Scan(&r.ID, &r.NomeGuerra, &r.Setor, &r.Presencas, &r.Atrasos,
				&r.Faltas, &r.Justificadas, &r.NaoVerificados, &r.Lancados, &funcaoID, &r.Funcao, &antig, &r.Grupo) == nil {
				r.FuncaoID = funcaoID
				r.Antiguidade = antig
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
		       COALESCE(SUM(pr.situacao IN ('falta','nao_verificado')),0)
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
	// v9.15.2: relatório sempre fresco — sem cache HTTP
	w.Header().Set("Cache-Control", "no-store, no-cache, must-revalidate")
	w.Header().Set("Pragma", "no-cache")
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
	// v9.15.2 (ordem Tenente): relatório SEMPRE on demand — proibir cache do navegador
	w.Header().Set("Cache-Control", "no-store, no-cache, must-revalidate")
	w.Header().Set("Pragma", "no-cache")
	w.Header().Set("Content-Type", "application/pdf")
	w.Header().Set("Content-Disposition",
		fmt.Sprintf("inline; filename=SCI_relatorio_%s_%s.pdf", de, ate))
	_, _ = w.Write(pdf)
}

func (a *App) hPessoaPDF(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id <= 0 {
		jsonErro(w, http.StatusBadRequest, "ID inválido")
		return
	}
	u := usuarioDoCtx(r)
	escopo := escopoDoUsuario(u)

	var f FichaPessoalPDF
	f.ID = id

	var gid *int64
	var setor, funcao, grupo string
	qP := `SELECT p.nome_guerra, p.nome_completo, p.status, p.grupo_id,
	              COALESCE(s.nome, 'Indefinido'), COALESCE(fu.nome, 'Indefinida'), COALESCE(g.nome, 'Geral'),
	              COALESCE(u.data_nascimento, ''), COALESCE(u.tipo_sanguineo, ''),
	              COALESCE(u.telefone, ''), COALESCE(u.email, ''), COALESCE(u.endereco, ''),
	              COALESCE(u.foto_base64, '')
	       FROM pessoas p
	       LEFT JOIN setores s ON s.id = p.setor_id
	       LEFT JOIN funcoes fu ON fu.id = p.funcao_id
	       LEFT JOIN grupos g ON g.id = p.grupo_id
	       LEFT JOIN usuarios u ON u.pessoa_id = p.id
	       WHERE p.id = ?`

	err = a.st.db.QueryRow(qP, id).Scan(
		&f.NomeGuerra, &f.NomeCompleto, &f.Status, &gid,
		&setor, &funcao, &grupo,
		&f.DataNascimento, &f.TipoSanguineo,
		&f.Telefone, &f.Email, &f.Endereco, &f.FotoBase64,
	)
	if err != nil {
		jsonErro(w, http.StatusNotFound, "pessoa não encontrada")
		return
	}

	if escopo > 0 && (gid == nil || *gid != escopo) {
		jsonErro(w, http.StatusForbidden, "pessoa fora do seu escopo")
		return
	}

	f.Setor = setor
	f.Funcao = funcao
	f.Grupo = grupo

	_ = a.st.db.QueryRow(`
		SELECT COUNT(*),
		       COALESCE(SUM(situacao = 'presente'), 0),
		       COALESCE(SUM(situacao = 'atraso'), 0),
		       COALESCE(SUM(situacao = 'falta'), 0),
		       COALESCE(SUM(situacao = 'justificada'), 0)
		FROM presencas WHERE pessoa_id = ?`, id).Scan(
		&f.TotalConfs, &f.Presencas, &f.Atrasos, &f.Faltas, &f.Justificadas,
	)
	if f.TotalConfs > 0 {
		f.PctPresenca = 100.0 * float64(f.Presencas+f.Atrasos) / float64(f.TotalConfs)
	}

	rowsC, errC := a.st.db.Query(`
		SELECT mc.id, mi.nome, mi.codigo_patrimonio, mc.data_saida, COALESCE(u.nome_guerra, u.login)
		FROM material_cautelas mc
		JOIN material_itens mi ON mi.id = mc.item_id
		JOIN usuarios u ON u.id = mc.responsavel_entrega_id
		WHERE mc.pessoa_id = ? AND mc.status = 'ativa'
		ORDER BY mc.id DESC LIMIT 10`, id)
	if errC == nil {
		for rowsC.Next() {
			var cid int64
			var itemNome, codPat, dtSaida, resp string
			if rowsC.Scan(&cid, &itemNome, &codPat, &dtSaida, &resp) == nil {
				f.CautelasAtivas = append(f.CautelasAtivas, map[string]any{
					"id":                  cid,
					"item_nome":           itemNome,
					"codigo_patrimonio":   codPat,
					"data_saida":          dtSaida,
					"responsavel_entrega": resp,
				})
			}
		}
		rowsC.Close()
	}

	rowsE, errE := a.st.db.Query(`
		SELECT et.data_inicio, et.data_fim, etp.nome, COALESCE(ep.funcao_escala, '')
		FROM escala_pessoas ep
		JOIN escala_turnos et ON et.id = ep.turno_id
		JOIN escala_tipos etp ON etp.id = et.tipo_id
		WHERE ep.pessoa_id = ?
		ORDER BY et.data_inicio DESC LIMIT 8`, id)
	if errE == nil {
		for rowsE.Next() {
			var dtIni, dtFim, tNome, fEsc string
			if rowsE.Scan(&dtIni, &dtFim, &tNome, &fEsc) == nil {
				f.Escalas = append(f.Escalas, map[string]any{
					"data_inicio":   dtIni,
					"data_fim":      dtFim,
					"tipo_nome":     tNome,
					"funcao_escala": fEsc,
				})
			}
		}
		rowsE.Close()
	}

	pdf, err := a.gerarFichaPessoalPDF(f, u.Login)
	if err != nil {
		jsonErro(w, http.StatusInternalServerError, "falha ao gerar ficha em PDF: "+err.Error())
		return
	}

	a.st.Auditoria(&u.ID, "exportar", "ficha_pessoal", &id, f.NomeGuerra, ipDe(r))
	w.Header().Set("Cache-Control", "no-store, no-cache, must-revalidate")
	w.Header().Set("Pragma", "no-cache")
	w.Header().Set("Content-Type", "application/pdf")
	w.Header().Set("Content-Disposition", fmt.Sprintf("inline; filename=ficha_%s_%d.pdf", f.NomeGuerra, id))
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
	             COALESCE(ue.nome_guerra, ue.login), COALESCE(ur.nome_guerra, COALESCE(ur.login, '—'))
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

func (a *App) hEscalasPDF(w http.ResponseWriter, r *http.Request) {
	if a.reservaAtivo() {
		jsonErro(w, http.StatusLocked, "módulo em reserva (indisponível nesta instalação)")
		return
	}
	u := usuarioDoCtx(r)
	escopo := escopoDoUsuario(u)

	mes := strings.TrimSpace(r.URL.Query().Get("mes"))
	de := strings.TrimSpace(r.URL.Query().Get("de"))
	ate := strings.TrimSpace(r.URL.Query().Get("ate"))

	if mes != "" && de == "" && ate == "" {
		de = mes + "-01"
		ate = mes + "-31"
	}
	if de == "" || ate == "" {
		hoje := time.Now().In(a.horaLocal)
		de = hoje.Format("2006-01") + "-01"
		ate = hoje.Format("2006-01") + "-31"
	}

	var esc EscalasRelatorioPDF
	esc.Periodo = fmt.Sprintf("%s a %s", de, ate)

	var grupoNome string
	if escopo > 0 {
		_ = a.st.db.QueryRow(`SELECT nome FROM grupos WHERE id = ?`, escopo).Scan(&grupoNome)
	} else {
		grupoNome = "Todas as Subunidades"
	}
	esc.Grupo = grupoNome

	qTurnos := `SELECT et.id, et.data_inicio, et.data_fim, etp.nome, COALESCE(et.observacao, ''), COALESCE(g.nome, '')
	            FROM escala_turnos et
	            JOIN escala_tipos etp ON etp.id = et.tipo_id
	            LEFT JOIN grupos g ON g.id = et.grupo_id
	            WHERE substr(et.data_inicio, 1, 10) <= ? AND substr(et.data_fim, 1, 10) >= ?`
	args := []any{ate, de}
	if escopo > 0 {
		qTurnos += ` AND et.grupo_id = ?`
		args = append(args, escopo)
	}
	qTurnos += ` ORDER BY et.data_inicio ASC, et.id ASC`

	rows, err := a.st.db.Query(qTurnos, args...)
	if err != nil {
		jsonErro(w, http.StatusInternalServerError, "falha ao buscar turnos: "+err.Error())
		return
	}

	var turnoIDs []int64
	for rows.Next() {
		var tid int64
		var dIni, dFim, tNome, obs, gNome string
		if rows.Scan(&tid, &dIni, &dFim, &tNome, &obs, &gNome) == nil {
			turnoIDs = append(turnoIDs, tid)
			esc.Turnos = append(esc.Turnos, map[string]any{
				"id":          tid,
				"data_inicio": dIni,
				"data_fim":    dFim,
				"tipo_nome":   tNome,
				"observacao":  obs,
				"grupo_nome":  gNome,
				"militares":   "",
			})
		}
	}
	rows.Close()

	if len(turnoIDs) > 0 {
		ph := strings.TrimSuffix(strings.Repeat("?,", len(turnoIDs)), ",")
		qP := fmt.Sprintf(`
			SELECT ep.turno_id, p.nome_guerra, COALESCE(ep.funcao_escala, '')
			FROM escala_pessoas ep
			JOIN pessoas p ON p.id = ep.pessoa_id
			WHERE ep.turno_id IN (%s)
			ORDER BY p.nome_guerra ASC`, ph)

		argsT := make([]any, len(turnoIDs))
		for i, id := range turnoIDs {
			argsT[i] = id
		}

		pRows, pErr := a.st.db.Query(qP, argsT...)
		if pErr == nil {
			pPorTurno := map[int64][]string{}
			for pRows.Next() {
				var tid int64
				var ng, fEsc string
				if pRows.Scan(&tid, &ng, &fEsc) == nil {
					info := ng
					if fEsc != "" {
						info += " (" + fEsc + ")"
					}
					pPorTurno[tid] = append(pPorTurno[tid], info)
				}
			}
			pRows.Close()

			for i := range esc.Turnos {
				tid := esc.Turnos[i]["id"].(int64)
				if mils, ok := pPorTurno[tid]; ok {
					esc.Turnos[i]["militares"] = strings.Join(mils, ", ")
				}
			}
		}
	}

	pdf, err := a.gerarEscalasPDF(esc, u.Login)
	if err != nil {
		jsonErro(w, http.StatusInternalServerError, "falha ao gerar PDF de escalas: "+err.Error())
		return
	}

	a.st.Auditoria(&u.ID, "exportar", "escalas_pdf", nil, de+" a "+ate, ipDe(r))
	w.Header().Set("Cache-Control", "no-store, no-cache, must-revalidate")
	w.Header().Set("Pragma", "no-cache")
	w.Header().Set("Content-Type", "application/pdf")
	w.Header().Set("Content-Disposition", "inline; filename=escala_servico.pdf")
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
	// v9.16.8: TODA a cadeia de superiores (recursivo) — herança vale para qualquer vínculo
	// ativo, em todos os níveis (ex.: 3º Pel → Cia → Bde).
	visita := map[int64]bool{gid: true}
	fila := []int64{gid}
	var out []int64
	for len(fila) > 0 {
		atual := fila[0]
		fila = fila[1:]
		rows, err := a.st.db.Query(`SELECT superior_id FROM grupo_vinculos WHERE subordinado_id = ?`, atual)
		if err != nil {
			continue
		}
		for rows.Next() {
			var sup int64
			if rows.Scan(&sup) == nil && !visita[sup] {
				visita[sup] = true
				out = append(out, sup)
				fila = append(fila, sup)
			}
		}
		rows.Close()
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
	var args []any
	q := `WITH RECURSIVE cam(id, caminho) AS (
		  SELECT id, nome FROM ` + t + ` WHERE pai_id IS NULL
		  UNION ALL
		  SELECT f.id, cm.caminho || ' > ' || f.nome FROM ` + t + ` f JOIN cam cm ON f.pai_id = cm.id
		)
		SELECT ` + t + `.id, ` + t + `.nome` + extra + `, ` + t + `.pai_id, ` + t + `.ativo, ` + t + `.grupo_id, ` + t + `.antiguidade
		FROM ` + t + ` LEFT JOIN cam ON cam.id = ` + t + `.id`
	if esc := escopoDoUsuario(usuarioDoCtx(r)); esc > 0 {
		// Doutrina: todos os membros da hierarquia têm visibilidade das tags de superiores, do próprio grupo e de subordinados
		ids := append([]int64{esc}, a.gruposSuperioresAtivos(esc)...)
		ids = append(ids, a.gruposSubordinadosAtivos(esc)...)
		ph := strings.TrimSuffix(strings.Repeat("?,", len(ids)), ",")
		args = make([]any, len(ids))
		for i, id := range ids {
			args[i] = id
		}
		q += ` WHERE ` + t + `.grupo_id IS NULL OR ` + t + `.grupo_id IN (` + ph + `)`
	}
	q += ` ORDER BY cam.caminho, antiguidade, nome`
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
	cols = append(cols, "pai_id", "ativo", "grupo_id", "antiguidade")
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
		Nome    string `json:"nome"`
		Sigla   string `json:"sigla"`
		Cor     string `json:"cor"`
		GrupoID *int64 `json:"grupo_id"`
	}
	if err = decodificar(r, &req); err != nil || strings.TrimSpace(req.Nome) == "" {
		jsonErro(w, http.StatusBadRequest, "nome obrigatório")
		return
	}
	nome := strings.TrimSpace(req.Nome)
	u := usuarioDoCtx(r)
	if u == nil || (u.Papel != "gerente" && u.Papel != "admin") {
		jsonErro(w, http.StatusForbidden, "gestão de catálogos é exclusiva do gerente ou administrador")
		return
	}
	var grupoID any
	if u.Papel == "admin" && req.GrupoID != nil {
		// Fix P2: admin só cria catálogo em grupo EXISTENTE (antes gravava FK cru
		// de grupo inexistente e quebrava herança de catálogos em silêncio).
		if *req.GrupoID <= 0 {
			jsonErro(w, http.StatusBadRequest, "grupo_id inválido")
			return
		}
		var n int
		if err := a.st.db.QueryRow(`SELECT COUNT(*) FROM grupos WHERE id = ?`, *req.GrupoID).Scan(&n); err != nil || n == 0 {
			jsonErro(w, http.StatusBadRequest, "grupo inexistente")
			return
		}
		grupoID = *req.GrupoID
	} else if u.GrupoID != nil {
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
	if u == nil || (u.Papel != "gerente" && u.Papel != "admin") {
		jsonErro(w, http.StatusForbidden, "gestão de catálogos é exclusiva do gerente ou administrador")
		return
	}
	if esc := escopoDoUsuario(u); esc > 0 {
		var donoGrupo int64
		if err := a.st.db.QueryRow(`SELECT COALESCE(grupo_id,0) FROM `+t+` WHERE id = ?`, id).Scan(&donoGrupo); err != nil {
			jsonErro(w, http.StatusNotFound, "item não encontrado")
			return
		}
		subordinados := a.gruposSubordinadosAtivos(esc)
		ehSubordinado := false
		for _, sub := range subordinados {
			if sub == donoGrupo {
				ehSubordinado = true
				break
			}
		}
		if donoGrupo != esc && !ehSubordinado {
			jsonErro(w, http.StatusForbidden, "catálogo herdado de grupo superior ou global — somente leitura")
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
		SELECT p.id, p.nome_guerra, p.nome_completo, p.setor_id,
		       COALESCE(p.funcao_id, u2.funcao_id, up2.funcao_id), p.status,
		       COALESCE(s.nome,''),
		       COALESCE(NULLIF(fu.nome,''), COALESCE(NULLIF(fu_u.nome,''), COALESCE(NULLIF(fu_up.nome,''), ''))),
		       COALESCE(g.nome,'')
		FROM pessoas p
		LEFT JOIN setores s ON s.id = p.setor_id
		LEFT JOIN funcoes fu ON fu.id = p.funcao_id
		LEFT JOIN usuarios u2 ON u2.pessoa_id = p.id
		LEFT JOIN funcoes fu_u ON fu_u.id = u2.funcao_id
		LEFT JOIN usuario_papeis up2 ON up2.usuario_id = u2.id AND (up2.grupo_id = p.grupo_id OR up2.grupo_id IS NULL)
		LEFT JOIN funcoes fu_up ON fu_up.id = up2.funcao_id
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

// hPessoaExcluir (v9.7): admin/gerente EXCLUEM pessoa do banco de pessoal.
// Gerente: só do próprio grupo. Com histórico em conferências (FK presencas) →
// desativa (status='inativo') e responde {desativado:true}; sem histórico → DELETE físico.
func (a *App) hPessoaExcluir(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		jsonErro(w, http.StatusBadRequest, "id inválido")
		return
	}
	u := usuarioDoCtx(r)
	var grupoID *int64
	if err := a.st.db.QueryRow(`SELECT grupo_id FROM pessoas WHERE id = ?`, id).Scan(&grupoID); err != nil {
		jsonErro(w, http.StatusNotFound, "pessoa inexistente")
		return
	}
	if esc := escopoDoUsuario(u); esc > 0 {
		if grupoID == nil || *grupoID != esc {
			jsonErro(w, http.StatusForbidden, "pessoa de outro grupo")
			return
		}
	}
	var lanc int
	_ = a.st.db.QueryRow(`SELECT COUNT(*) FROM presencas WHERE pessoa_id = ?`, id).Scan(&lanc)
	if lanc > 0 {
		if _, err = a.st.db.Exec(`UPDATE pessoas SET status='inativo',
		 atualizado_em=strftime('%Y-%m-%dT%H:%M:%fZ','now') WHERE id=?`, id); err != nil {
			jsonErro(w, http.StatusInternalServerError, err.Error())
			return
		}
		a.st.Auditoria(&u.ID, "desativar", "pessoas", &id, "histórico preservado (lançamentos)", ipDe(r))
		jsonOK(w, map[string]bool{"desativado": true})
		return
	}
	if _, err = a.st.db.Exec(`DELETE FROM pessoas WHERE id = ?`, id); err != nil {
		jsonErro(w, http.StatusInternalServerError, err.Error())
		return
	}
	a.st.Auditoria(&u.ID, "excluir", "pessoas", &id, "", ipDe(r))
	jsonOK(w, map[string]bool{"ok": true})
}

// hGrupoExcluir (v9.7): admin EXCLUI grupo — vazio (sem pessoas/contas/vínculos/histórico).
// v9.10: modo FORÇADO (?forcar=1 + senha de admin no corpo) exclui grupo INTEIRO mesmo
// com contas e pessoas: contas do grupo são EXCLUÍDAS, pessoas também, subordinação do
// grupo é removida. Histórico de conferências continua blindado — grupo com conferências
// gravadas NÃO é excluído nem forçado.
// v1.1 (ordem Tenente 30/09): MODO NUKE (?nuke=1 + senha de admin) — exclusão FORÇADA
// TOTAL: apaga TUDO do grupo, INCLUSIVE o histórico de conferências (presenças,
// comentários), catálogos e dados dos módulos em reserva. Reversível? NÃO. A dupla
// confirmação é no FRONT; o servidor prova autoridade com a senha de admin.
func (a *App) hGrupoExcluir(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		jsonErro(w, http.StatusBadRequest, "id inválido")
		return
	}
	u := usuarioDoCtx(r)
	var nome string
	if err := a.st.db.QueryRow(`SELECT nome FROM grupos WHERE id = ?`, id).Scan(&nome); err != nil {
		jsonErro(w, http.StatusNotFound, "grupo inexistente")
		return
	}
	forcar := r.URL.Query().Get("forcar") == "1"
	nuke := r.URL.Query().Get("nuke") == "1"
	if forcar || nuke {
		var req struct {
			Senha string `json:"senha"`
		}
		if err := decodificar(r, &req); err != nil || req.Senha == "" {
			jsonErro(w, http.StatusBadRequest, "senha de admin obrigatória para exclusão forçada")
			return
		}
		var hash string
		if err := a.st.db.QueryRow(`SELECT senha_hash FROM usuarios WHERE id = ?`, u.ID).Scan(&hash); err != nil ||
			!verificaSenha(req.Senha, hash) {
			modo := "forcar"
			if nuke {
				modo = "nuke"
			}
			a.st.Auditoria(&u.ID, "excluir_grupo_negado", "grupos", &id, "senha incorreta ("+modo+")", ipDe(r))
			jsonErro(w, http.StatusUnauthorized, "senha de admin incorreta — exclusão negada")
			return
		}
	}
	var pessoas, contas, vinculos, confs int
	_ = a.st.db.QueryRow(`SELECT
		(SELECT COUNT(*) FROM pessoas WHERE grupo_id = ?),
		(SELECT COUNT(*) FROM usuarios WHERE grupo_id = ? AND ativo = 1),
		(SELECT COUNT(*) FROM grupo_vinculos WHERE superior_id = ? OR subordinado_id = ?),
		(SELECT COUNT(*) FROM conferencias WHERE grupo_id = ?)`,
		id, id, id, id, id).Scan(&pessoas, &contas, &vinculos, &confs)
	if confs > 0 && !nuke {
		jsonErro(w, http.StatusConflict, fmt.Sprintf("grupo tem %d conferência(s) no histórico — exclusão negada (imutabilidade histórica); exclusão TOTAL exige o MODO NUKE com dupla confirmação", confs))
		return
	}
	if !forcar && !nuke {
		if pessoas > 0 {
			jsonErro(w, http.StatusConflict, fmt.Sprintf("grupo tem %d pessoa(s) no banco de pessoal — mova ou exclua antes (ou use exclusão forçada com senha)", pessoas))
			return
		}
		if contas > 0 {
			jsonErro(w, http.StatusConflict, fmt.Sprintf("grupo tem %d conta(s) ativa(s) — mova ou exclua as contas antes (ou use exclusão forçada com senha)", contas))
			return
		}
		if vinculos > 0 {
			jsonErro(w, http.StatusConflict, "grupo tem vínculos de subordinação — remova-os antes (painel de grupos)")
			return
		}
	}
	tx, err := a.st.db.Begin()
	if err != nil {
		jsonErro(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer tx.Rollback()
	if _, err = tx.Exec(`DELETE FROM grupo_vinculos WHERE superior_id = ? OR subordinado_id = ?`, id, id); err != nil {
		jsonErro(w, http.StatusInternalServerError, err.Error())
		return
	}
	if nuke {
		// MODO NUKE (ordem Tenente 30/09): apaga TODO o rastro do grupo. Ordem respeita
		// FKs: histórico → vínculos com módulos → catálogos do grupo. Backup automático
		// é gravado logo após o commit (backupAssincrono no fim do handler).
		for _, q := range []string{
			// 0) auditoria dos usuários do grupo: vínculo anulado (rastro forense
			// preservado — auditoria.usuario_id não tem CASCADE)
			`UPDATE auditoria SET usuario_id = NULL WHERE usuario_id IN (SELECT id FROM usuarios WHERE grupo_id = ?)`,
			// 1) histórico de conferências do grupo (presenças/comentários caem por CASCADE)
			`DELETE FROM presencas WHERE pessoa_id IN (SELECT id FROM pessoas WHERE grupo_id = ?)`,
			`DELETE FROM comentarios WHERE pessoa_id IN (SELECT id FROM pessoas WHERE grupo_id = ?)`,
			`DELETE FROM conferencias WHERE grupo_id = ?`,
			// 2) módulos em reserva (escala/material) ligados ao grupo ou ao pessoal dele
			`DELETE FROM escala_pessoas WHERE pessoa_id IN (SELECT id FROM pessoas WHERE grupo_id = ?)`,
			`DELETE FROM escala_turnos WHERE grupo_id = ?`,
			`DELETE FROM escala_tipos WHERE grupo_id = ?`,
			`DELETE FROM material_cautelas WHERE item_id IN (SELECT id FROM material_itens WHERE grupo_id = ?)`,
			`DELETE FROM material_itens WHERE grupo_id = ?`,
			`DELETE FROM material_categorias WHERE grupo_id = ?`,
			// 3) catálogos de organização do grupo (referências já apagadas acima).
			// setores/funções: 2 passes — filho (pai_id) antes do pai, self-FK exige
			`DELETE FROM setores WHERE pai_id IS NOT NULL AND grupo_id = ?`,
			`DELETE FROM funcoes WHERE pai_id IS NOT NULL AND grupo_id = ?`,
			`DELETE FROM tags WHERE grupo_id = ?`,
			`DELETE FROM destinos WHERE grupo_id = ?`,
			`DELETE FROM setores WHERE grupo_id = ?`,
			`DELETE FROM funcoes WHERE grupo_id = ?`,
			`DELETE FROM conferencia_tipos WHERE grupo_id = ?`,
		} {
			if _, err = tx.Exec(q, id); err != nil {
				jsonErro(w, http.StatusInternalServerError, "nuke: "+err.Error())
				return
			}
		}
	}
	if _, err = tx.Exec(`DELETE FROM usuarios WHERE grupo_id = ?`, id); err != nil {
		jsonErro(w, http.StatusInternalServerError, err.Error())
		return
	}
	if _, err = tx.Exec(`DELETE FROM pessoas WHERE grupo_id = ?`, id); err != nil {
		jsonErro(w, http.StatusInternalServerError, err.Error())
		return
	}
	if _, err = tx.Exec(`DELETE FROM grupos WHERE id = ?`, id); err != nil {
		jsonErro(w, http.StatusInternalServerError, err.Error())
		return
	}
	if err = tx.Commit(); err != nil {
		jsonErro(w, http.StatusInternalServerError, err.Error())
		return
	}
	detalhe := "vazio"
	if nuke {
		detalhe = fmt.Sprintf("NUKE: %d conta(s), %d pessoa(s) e %d conferência(s) APAGADAS", contas, pessoas, confs)
	} else if forcar {
		detalhe = fmt.Sprintf("FORÇADA: %d conta(s) e %d pessoa(s) removidas", contas, pessoas)
	}
	a.st.Auditoria(&u.ID, "excluir", "grupos", &id, nome+" ["+detalhe+"]", ipDe(r))
	jsonOK(w, map[string]any{"ok": true, "forçada": forcar || nuke, "nuke": nuke,
		"contas_removidas": contas, "pessoas_removidas": pessoas, "conferencias_removidas": confs})
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
	// v9.4 (ordem Tenente): status de pessoal só ATIVO ou INATIVO
	if req.Status != "ativo" && req.Status != "inativo" {
		jsonErro(w, http.StatusBadRequest, "status só pode ser ativo ou inativo")
		return
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
	// v9.7: safeguard anti-duplicata — mesma pessoa (guerra+completo, case-insensitive,
	// trim) não nasce duas vezes no mesmo grupo (índice único do schema v7 é a 2ª barreira)
	var dup int
	if err := a.st.db.QueryRow(`SELECT COUNT(*) FROM pessoas
		WHERE grupo_id IS ? AND LOWER(TRIM(nome_guerra)) = LOWER(TRIM(?)) AND LOWER(TRIM(nome_completo)) = LOWER(TRIM(?))`,
		grupoID, strings.TrimSpace(req.NomeGuerra), strings.TrimSpace(req.NomeCompleto)).Scan(&dup); err == nil && dup > 0 {
		jsonErro(w, http.StatusConflict, "pessoa já cadastrada neste grupo (mesmo nome de guerra e nome completo)")
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
	// v9.4 (ordem Tenente): status de pessoal só ATIVO ou INATIVO
	if req.Status != "ativo" && req.Status != "inativo" {
		jsonErro(w, http.StatusBadRequest, "status só pode ser ativo ou inativo")
		return
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
	if req.FuncaoID != nil {
		_, _ = a.st.db.Exec(`UPDATE usuarios SET funcao_id = ? WHERE pessoa_id = ?`, req.FuncaoID, id)
	}
	if req.SetorID != nil {
		_, _ = a.st.db.Exec(`UPDATE usuarios SET setor_id = ? WHERE pessoa_id = ?`, req.SetorID, id)
	}
	a.st.Auditoria(&u.ID, "alterar", "pessoas", &id, req.NomeGuerra, ipDe(r))
	jsonOK(w, map[string]bool{"ok": true})
}

// ---------- usuários ----------

func (a *App) hUsuariosList(w http.ResponseWriter, r *http.Request) {
	u := usuarioDoCtx(r)
	escopo := escopoDoUsuario(u)
	// ordem 04/10 (Grupos): gerente vê o PRÓPRIO grupo + GRUPOS SUBORDINADOS
	// (admin continua vendo todos). Operador/chefe seguem só no próprio grupo.
	var subordinados []int64
	if u.Papel == "gerente" && escopo > 0 {
		subordinados = a.gruposSubordinadosAtivos(escopo)
	}
	rows, err := a.st.db.Query(
		`SELECT id, login, papel, pessoa_id, COALESCE(grupo_id,0), ativo, criado_em, senhas,
		        COALESCE(nome_guerra,''), COALESCE(nome_completo,''),
		        COALESCE(data_nascimento,''), COALESCE(tipo_sanguineo,''),
		        COALESCE(telefone,''), COALESCE(email,''),
		        COALESCE(endereco,''), COALESCE(foto_base64,'')
		 FROM usuarios ORDER BY id`)
	if err != nil {
		jsonErro(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer rows.Close()
	out := []map[string]any{}
	for rows.Next() {
		var id, grupoID int64
		var login, papel, criado, senhas, nomeGuerra, nomeCompleto string
		var dataNasc, tipoSang, tel, email, endereco, foto string
		var pessoaID *int64
		var ativo int
		if rows.Scan(&id, &login, &papel, &pessoaID, &grupoID, &ativo, &criado, &senhas, &nomeGuerra, &nomeCompleto,
			&dataNasc, &tipoSang, &tel, &email, &endereco, &foto) == nil {
			// escopo: admin vê tudo; gerente vê o PRÓPRIO grupo + subordinados
			// (ordem 04/10); operador/chefe só contas do próprio grupo, SEM
			// dados de sessão/senha (v9.4).
			mostrar := escopo <= 0 || int64(escopo) == grupoID
			if !mostrar && len(subordinados) > 0 {
				for _, sid := range subordinados {
					if sid == grupoID {
						mostrar = true
						break
					}
				}
			}
			if !mostrar {
				continue
			}
			item := map[string]any{
				"id": id, "login": login, "papel": papel, "pessoa_id": pessoaID,
				"grupo_id": grupoID, "ativo": ativo == 1, "criado_em": criado,
				"nome_guerra": nomeGuerra, "nome_completo": nomeCompleto,
				"data_nascimento": dataNasc, "tipo_sanguineo": tipoSang,
				"telefone": tel, "email": email, "endereco": endereco, "foto_base64": foto,
			}
			if u.Papel != "operador" {
				item["senhas"] = json.RawMessage(senhas)
			}
			out = append(out, item)
		}
	}
	rows.Close()

	for i := range out {
		id := out[i]["id"].(int64)
		papeis, _ := a.st.PapeisDoUsuario(id)
		out[i]["papeis"] = papeis
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
	_ = a.st.db.QueryRow(`SELECT COALESCE(nome_guerra,''), COALESCE(nome_completo,''),
		COALESCE(data_nascimento,''), COALESCE(tipo_sanguineo,''), COALESCE(telefone,''),
		COALESCE(email,''), COALESCE(endereco,''), COALESCE(foto_base64,'')
		FROM usuarios WHERE id = ?`, u.ID).Scan(
		&u.NomeGuerra, &u.NomeCompleto,
		&u.DataNascimento, &u.TipoSanguineo, &u.Telefone,
		&u.Email, &u.Endereco, &u.FotoBase64)

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

// hPerfilSet: o próprio usuário atualiza seu perfil completo (dados cadastrais e foto 1x1).
func (a *App) hPerfilSet(w http.ResponseWriter, r *http.Request) {
	var req struct {
		NomeGuerra     string `json:"nome_guerra"`
		NomeCompleto   string `json:"nome_completo"`
		DataNascimento string `json:"data_nascimento"`
		TipoSanguineo  string `json:"tipo_sanguineo"`
		Telefone       string `json:"telefone"`
		Email          string `json:"email"`
		Endereco       string `json:"endereco"`
		FotoBase64     string `json:"foto_base64"`
	}
	if err := decodificar(r, &req); err != nil || strings.TrimSpace(req.NomeGuerra) == "" || strings.TrimSpace(req.NomeCompleto) == "" {
		jsonErro(w, http.StatusBadRequest, "nome de guerra e nome completo obrigatórios")
		return
	}
	u := usuarioDoCtx(r)
	if _, err := a.st.db.Exec(`UPDATE usuarios SET
		nome_guerra = ?, nome_completo = ?,
		data_nascimento = ?, tipo_sanguineo = ?,
		telefone = ?, email = ?, endereco = ?,
		foto_base64 = ?
		WHERE id = ?`,
		strings.TrimSpace(req.NomeGuerra), strings.TrimSpace(req.NomeCompleto),
		strings.TrimSpace(req.DataNascimento), strings.TrimSpace(req.TipoSanguineo),
		strings.TrimSpace(req.Telefone), strings.TrimSpace(req.Email), strings.TrimSpace(req.Endereco),
		strings.TrimSpace(req.FotoBase64), u.ID); err != nil {
		jsonErro(w, http.StatusInternalServerError, err.Error())
		return
	}
	if u.PessoaID != nil {
		_, _ = a.st.db.Exec(`UPDATE pessoas SET nome_guerra = ?, nome_completo = ? WHERE id = ?`,
			strings.TrimSpace(req.NomeGuerra), strings.TrimSpace(req.NomeCompleto), *u.PessoaID)
	}
	a.st.Auditoria(&u.ID, "editar", "usuarios", &u.ID, "perfil próprio", ipDe(r))
	jsonOK(w, map[string]bool{"ok": true})
}

// hUsuarioFotoGet: entrega o stream de imagem da foto de perfil 1x1 do usuário.
func (a *App) hUsuarioFotoGet(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		jsonErro(w, http.StatusBadRequest, "id inválido")
		return
	}
	var foto string
	err = a.st.db.QueryRow(`SELECT COALESCE(foto_base64,'') FROM usuarios WHERE id = ?`, id).Scan(&foto)
	if err != nil || foto == "" {
		http.NotFound(w, r)
		return
	}
	partes := strings.Split(foto, ",")
	b64 := foto
	mime := "image/jpeg"
	if len(partes) == 2 {
		b64 = partes[1]
		if strings.Contains(partes[0], "image/png") {
			mime = "image/png"
		} else if strings.Contains(partes[0], "image/webp") {
			mime = "image/webp"
		}
	}
	raw, err := base64.StdEncoding.DecodeString(b64)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", mime)
	w.Header().Set("Cache-Control", "public, max-age=3600")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(raw)
}

// hMoverConta (v9.5): MOVE conta entre grupos. Admin: qualquer origem → qualquer
// destino. Gerente: origem = próprio grupo ou subordinado; destino = próprio grupo
// ou subordinado (dentro da hierarquia dele). Nunca move admin. Gerente não pode
// deixar seu grupo sem gerente (vira operador no destino ou bloqueia se for o único).
func (a *App) hMoverConta(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		jsonErro(w, http.StatusBadRequest, "id inválido")
		return
	}
	var req struct {
		GrupoID int64 `json:"grupo_id"`
	}
	if err := decodificar(r, &req); err != nil || req.GrupoID == 0 {
		jsonErro(w, http.StatusBadRequest, "grupo_id obrigatório")
		return
	}
	u := usuarioDoCtx(r)
	var alvoPapel string
	var origem int64
	if err := a.st.db.QueryRow(`SELECT papel, COALESCE(grupo_id,0) FROM usuarios WHERE id = ?`, id).Scan(&alvoPapel, &origem); err != nil {
		jsonErro(w, http.StatusNotFound, "usuário inexistente")
		return
	}
	if alvoPapel == "admin" {
		jsonErro(w, http.StatusForbidden, "conta admin não é movida")
		return
	}
	dentroDaArvore := func(g int64) bool { return g == *u.GrupoID || int64Contem(a.gruposSubordinadosAtivos(*u.GrupoID), g) }
	if u.Papel == "gerente" {
		if u.GrupoID == nil {
			jsonErro(w, http.StatusForbidden, "gerente sem grupo definido")
			return
		}
		if !dentroDaArvore(origem) || !dentroDaArvore(req.GrupoID) {
			jsonErro(w, http.StatusForbidden, "mover só dentro da sua hierarquia (próprio grupo + subordinados)")
			return
		}
	}
	// destino precisa existir
	var existe int
	if a.st.db.QueryRow(`SELECT COUNT(*) FROM grupos WHERE id = ?`, req.GrupoID).Scan(&existe) != nil || existe == 0 {
		jsonErro(w, http.StatusNotFound, "grupo de destino inexistente")
		return
	}
	if origem == req.GrupoID {
		jsonOK(w, map[string]bool{"ok": true}) // nada a fazer
		return
	}
	// gerente que se move: se for o único do grupo de origem, rebaixa a operador no destino
	vaiRebaixar := false
	if alvoPapel == "gerente" {
		var outros int
		_ = a.st.db.QueryRow(`SELECT COUNT(*) FROM usuarios WHERE grupo_id = ? AND papel = 'gerente' AND id <> ? AND ativo = 1`,
			origem, id).Scan(&outros)
		if outros == 0 {
			vaiRebaixar = true
		}
	}
	tx, err := a.st.db.Begin()
	if err != nil {
		jsonErro(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer tx.Rollback()
	if vaiRebaixar {
		if _, err = tx.Exec(`UPDATE usuarios SET grupo_id = ?, papel = 'operador' WHERE id = ?`, req.GrupoID, id); err != nil {
			jsonErro(w, http.StatusInternalServerError, err.Error())
			return
		}
	} else {
		if _, err = tx.Exec(`UPDATE usuarios SET grupo_id = ? WHERE id = ?`, req.GrupoID, id); err != nil {
			jsonErro(w, http.StatusInternalServerError, err.Error())
			return
		}
	}
	if err = tx.Commit(); err != nil {
		jsonErro(w, http.StatusInternalServerError, err.Error())
		return
	}
	a.st.Auditoria(&u.ID, "mover", "usuarios", &id,
		fmt.Sprintf("origem=%d destino=%d rebaixado=%v", origem, req.GrupoID, vaiRebaixar), ipDe(r))
	jsonOK(w, map[string]any{"ok": true, "rebaixado": vaiRebaixar})
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

// hArvoreGrupos (v9.4): árvore NESTED dos grupos. Admin: floresta completa a partir
// das raízes. Gerente/operador: próprio grupo como raiz + subordinados (recursivo).
// READ ONLY — subordinação só no painel admin.
// v9.10: efetivo RECURSIVO — cada nó carrega o efetivo próprio + soma de toda a
// subárvore (o painel mostra o total agregado; ao expandir, o valor se disseca por nível).
func (a *App) hArvoreGrupos(w http.ResponseWriter, r *http.Request) {
	u := usuarioDoCtx(r)
	rows, err := a.st.db.Query(`
		SELECT g.id, g.nome, COALESCE(g.codigo,''),
		       (SELECT COUNT(*) FROM pessoas p WHERE p.grupo_id = g.id AND p.status = 'ativo'),
		       (SELECT COUNT(*) FROM usuarios us WHERE us.grupo_id = g.id AND us.ativo = 1)
		FROM grupos g ORDER BY g.nome`)
	if err != nil {
		jsonErro(w, http.StatusInternalServerError, err.Error())
		return
	}
	type GrupoN struct {
		ID             int64     `json:"id"`
		Nome           string    `json:"nome"`
		Codigo         string    `json:"codigo"`
		Efetivo        int       `json:"efetivo"`         // próprio (sem subordinados)
		EfetivoTotal   int       `json:"efetivo_total"`   // recursivo: próprio + subárvore
		Contas         int       `json:"contas"`          // próprias (sem subordinados)
		ContasTotal    int       `json:"contas_total"`    // recursivo: próprias + subárvore
		SubgruposTotal int       `json:"subgrupos_total"` // total de grupos subordinados na subárvore
		Filhos         []*GrupoN `json:"filhos"`
	}
	nos := map[int64]*GrupoN{}
	filhosDe := map[int64][]int64{}
	paiDe := map[int64]int64{}
	defer rows.Close()
	for rows.Next() {
		var id int64
		var nome, cod string
		var efetivo, contas int
		if err := rows.Scan(&id, &nome, &cod, &efetivo, &contas); err != nil {
			continue
		}
		nos[id] = &GrupoN{ID: id, Nome: nome, Codigo: cod, Efetivo: efetivo, Contas: contas, Filhos: []*GrupoN{}}
	}
	linhas, err := a.st.db.Query(`SELECT superior_id, subordinado_id FROM grupo_vinculos
		WHERE criado_por_superior = 1 AND criado_por_subordinado = 1`)
	if err != nil {
		jsonErro(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer linhas.Close()
	for linhas.Next() {
		var sup, sub int64
		if linhas.Scan(&sup, &sub) == nil {
			filhosDe[sup] = append(filhosDe[sup], sub)
			paiDe[sub] = sup
		}
	}
	var montar func(id int64, profundidade int) *GrupoN
	montar = func(id int64, profundidade int) *GrupoN {
		no := nos[id]
		if no == nil || profundidade > 32 {
			return nil
		}
		no.EfetivoTotal = no.Efetivo
		no.ContasTotal = no.Contas
		no.SubgruposTotal = 0
		for _, filho := range filhosDe[id] {
			if f := montar(filho, profundidade+1); f != nil {
				no.Filhos = append(no.Filhos, f)
				no.EfetivoTotal += f.EfetivoTotal
				no.ContasTotal += f.ContasTotal
				no.SubgruposTotal += 1 + f.SubgruposTotal
			}
		}
		return no
	}
	raizes := []int64{}
	for id := range nos {
		if paiDe[id] == 0 {
			raizes = append(raizes, id)
		}
	}
	if u != nil && u.Papel != "admin" && u.GrupoID != nil {
		// gerente/operador: árvore enraizada no PRÓPRIO grupo
		if raiz := montar(*u.GrupoID, 0); raiz != nil {
			jsonOK(w, []*GrupoN{raiz})
			return
		}
		jsonOK(w, []*GrupoN{})
		return
	}
	out := []*GrupoN{}
	for _, id := range raizes {
		if no := montar(id, 0); no != nil {
			out = append(out, no)
		}
	}
	jsonOK(w, out)
}

// hComentariosAdd: comentário append-only sobre pessoa em conferência (ordem Tenente).
func (a *App) hComentariosAdd(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ConferenciaID int64  `json:"conferencia_id"`
		PessoaID      int64  `json:"pessoa_id"`
		Comentario    string `json:"comentario"`
		TagID         *int64 `json:"tag_id"` // ordem Tenente 30/09: TAGs do catálogo nos comentários
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
	// TAG (ordem Tenente 30/09): se informada, precisa existir e ser visível no escopo
	if req.TagID != nil && *req.TagID > 0 {
		var n int
		var q2 string
		args2 := []any{*req.TagID}
		if esc := escopoDoUsuario(u); esc > 0 {
			ids := append([]int64{esc}, a.gruposSuperioresAtivos(esc)...)
			marks := strings.TrimRight(strings.Repeat("?,", len(ids)), ",")
			q2 = `SELECT COUNT(*) FROM tags WHERE id = ? AND ativo = 1 AND (grupo_id IS NULL OR grupo_id IN (` + marks + `))`
			for _, v := range ids {
				args2 = append(args2, v)
			}
		} else {
			q2 = `SELECT COUNT(*) FROM tags WHERE id = ? AND ativo = 1`
		}
		_ = a.st.db.QueryRow(q2, args2...).Scan(&n)
		if n == 0 {
			jsonErro(w, http.StatusBadRequest, "tag inválida")
			return
		}
	}
	res, err := a.st.db.Exec(
		`INSERT INTO comentarios (ordem, conferencia_id, pessoa_id, operador_id, comentario, tag_id) VALUES (?,?,?,?,?,?)`,
		ord, req.ConferenciaID, req.PessoaID, u.ID, strings.TrimSpace(req.Comentario), req.TagID)
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
		SELECT c.ordem, c.criado_em, p.nome_guerra, u.login, c.comentario, COALESCE(tt.nome,'')
		FROM comentarios c
		JOIN pessoas p ON p.id = c.pessoa_id
		JOIN usuarios u ON u.id = c.operador_id
		LEFT JOIN tags tt ON tt.id = c.tag_id
		WHERE c.conferencia_id = ? ORDER BY c.ordem`, id)
	if err != nil {
		jsonErro(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer rows.Close()
	out := []map[string]any{}
	for rows.Next() {
		var ord int64
		var em, nome, por, comentario, tag string
		if rows.Scan(&ord, &em, &nome, &por, &comentario, &tag) == nil {
			out = append(out, map[string]any{
				"ordem": ord, "datahora": em, "pessoa": nome,
				"operador": por, "comentario": comentario, "tag": tag,
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
		FuncaoID *int64 `json:"funcao_id"`
		SetorID  *int64 `json:"setor_id"`
	}
	if err := decodificar(r, &req); err != nil || req.Login == "" {
		jsonErro(w, http.StatusBadRequest, "identificação obrigatória")
		return
	}
	precisaSetup := 1
	if req.Senha == "" {
		req.Senha = "sci"
	} else if req.Senha != "sci" {
		precisaSetup = 0
	}
	u := usuarioDoCtx(r)
	papel := strings.ToLower(strings.TrimSpace(req.Papel))
	switch papel {
	case "admin", "gerente", "operador", "chefe_setor":
	default:
		jsonErro(w, http.StatusBadRequest, "papel inválido (admin | gerente | operador | chefe_setor)")
		return
	}
	// Hierarquia de criação (ordem Diretor 04/10 — "Criação de usuários" e
	// "Sistema de operadores"):
	// - ADMIN cria qualquer papel; usuários criados pelo admin NÃO têm grupo
	//   (exceto gerente, que exige unidade).
	// - GERENTE cria CHEFE_SETOR no próprio grupo; gerente NÃO cria operador —
	//   ele seleciona os chefes de setor do grupo, e os chefes selecionam os
	//   operadores dentre os usuários do seu setor.
	// - CHEFE_SETOR promove a OPERADOR somente usuário DO SEU SETOR.
	// - OPERADOR não cria conta nenhuma.
	// A liberação do menu de gestão no front é atrelada à FUNÇÃO de encarregado
	// de pessoal (catálogo do grupo); a defesa dura aqui é por PAPEL.
	if u.Papel == "operador" {
		jsonErro(w, http.StatusForbidden, "operador não cria contas")
		return
	}
	if u.Papel != "admin" {
		if u.GrupoID == nil {
			jsonErro(w, http.StatusForbidden, "gerente sem grupo definido")
			return
		}
		switch {
		case u.Papel == "gerente":
			if papel != "chefe_setor" {
				jsonErro(w, http.StatusForbidden, "gerente seleciona chefes de setor; operadores são designados pelos chefes")
				return
			}
			req.GrupoID = u.GrupoID // força o próprio grupo, ignore o que vier no corpo
		case u.Papel == "chefe_setor":
			if papel != "operador" {
				jsonErro(w, http.StatusForbidden, "chefe de setor só designa operadores")
				return
			}
			req.GrupoID = u.GrupoID
		default:
			jsonErro(w, http.StatusForbidden, "sem permissão para criar contas")
			return
		}
	}
	if papel == "admin" {
		req.GrupoID = nil // admin é global
	}
	// ordem 04/10: DEFAULT dos usuários criados pelo admin é SEM grupo — o
	// modal do admin não envia grupo. Se o corpo trouxer grupo explícito
	// (provisionamento/seed), o admin pode vinculá-lo deliberadamente.
	// CHEFE_SETOR designa OPERADOR: a conta herda o SETOR do chefe (é o
	// vínculo que define o escopo de atuação); gerente/admin podem informar
	// funcao_id/setor_id explícitos (catálogos do grupo) na criação.
	if u.Papel == "chefe_setor" && papel == "operador" && req.SetorID == nil {
		var setorChefe *int64
		_ = a.st.db.QueryRow(`SELECT setor_id FROM usuarios WHERE id = ?`, u.ID).Scan(&setorChefe)
		req.SetorID = setorChefe
	}
	if req.SetorID != nil && *req.SetorID <= 0 {
		req.SetorID = nil
	}
	if req.FuncaoID != nil && *req.FuncaoID <= 0 {
		req.FuncaoID = nil
	}
	// ordem 04/10 (Fase G): função informada deve pertencer ao catálogo do
	// grupo de vinculação (global = grupo NULL serve a todos). Gerente não
	// atravessa grupos atribuindo função alheia.
	if req.FuncaoID != nil {
		var fGrupo *int64
		_ = a.st.db.QueryRow(`SELECT grupo_id FROM funcoes WHERE id = ?`, *req.FuncaoID).Scan(&fGrupo)
		alvo := req.GrupoID
		if fGrupo != nil && (alvo == nil || *fGrupo != *alvo) {
			jsonErro(w, http.StatusForbidden, "função não pertence ao catálogo do grupo")
			return
		}
	}
	if papel == "gerente" && (req.GrupoID == nil || *req.GrupoID <= 0) {
		jsonErro(w, http.StatusBadRequest, "gerente deve obrigatoriamente estar vinculado a uma unidade")
		return
	}
	hash, err := hashSenha(req.Senha)
	if err != nil {
		jsonErro(w, http.StatusInternalServerError, err.Error())
		return
	}
	res, err := a.st.db.Exec(
		`INSERT INTO usuarios (login, senha_hash, papel, pessoa_id, grupo_id, funcao_id, setor_id, precisa_setup) VALUES (?,?,?,?,?,?,?,?)`,
		strings.ToLower(strings.TrimSpace(req.Login)), hash, papel, req.PessoaID, req.GrupoID, req.FuncaoID, req.SetorID, precisaSetup)
	if err != nil {
		jsonErro(w, http.StatusBadRequest, "não criado (duplicado?): "+err.Error())
		return
	}
	id, _ := res.LastInsertId()
	var funcaoID *int64
	if req.FuncaoID != nil {
		funcaoID = req.FuncaoID // função informada no create (ordem 04/10)
	} else if req.PessoaID != nil {
		_ = a.st.db.QueryRow(`SELECT funcao_id FROM pessoas WHERE id = ?`, *req.PessoaID).Scan(&funcaoID)
	}
	_, _ = a.st.db.Exec(`INSERT OR IGNORE INTO usuario_papeis (usuario_id, grupo_id, papel, funcao_id) VALUES (?,?,?,?)`,
		id, req.GrupoID, papel, funcaoID)

	a.st.Auditoria(&u.ID, "criar", "usuarios", &id, req.Login+" ("+papel+")", ipDe(r))
	jsonOK(w, map[string]any{"id": id})
}

// hOperadoresDoSetor (ordem 04/10 — Fase G3): o CHEFE DE SETOR não cria
// operadores — SELECIONA dentre as contas presentes no SEU setor. GET lista
// os candidatos do setor; POST designa a conta como operador (INSERT do papel
// na mesma linha, sem mudar o papel principal da conta).
func (a *App) hOperadoresDoSetor(w http.ResponseWriter, r *http.Request) {
	u := usuarioDoCtx(r)
	if u.Papel != "chefe_setor" {
		jsonErro(w, http.StatusForbidden, "somente chefe de setor opera este endpoint")
		return
	}
	if u.GrupoID == nil {
		jsonErro(w, http.StatusForbidden, "chefe de setor sem grupo definido")
		return
	}
	var setorID *int64
	_ = a.st.db.QueryRow(`SELECT setor_id FROM usuarios WHERE id = ?`, u.ID).Scan(&setorID)
	if setorID == nil {
		jsonErro(w, http.StatusBadRequest, "chefe de setor sem setor vinculado")
		return
	}

	if r.Method == http.MethodGet {
		rows, err := a.st.db.Query(`
			SELECT u.id, u.login, COALESCE(u.nome_guerra,''), COALESCE(u.nome_completo,''),
			       COALESCE(u.funcao_id,0), COALESCE(f.nome,''),
			       EXISTS(SELECT 1 FROM usuario_papeis up WHERE up.usuario_id = u.id AND up.grupo_id = ? AND up.papel = 'operador')
			FROM usuarios u
			LEFT JOIN funcoes f ON f.id = u.funcao_id
			WHERE u.grupo_id = ? AND u.setor_id = ? AND u.ativo = 1
			ORDER BY u.login`, *u.GrupoID, *u.GrupoID, *setorID)
		if err != nil {
			jsonErro(w, http.StatusInternalServerError, err.Error())
			return
		}
		defer rows.Close()
		out := []map[string]any{}
		for rows.Next() {
			var id, fid int64
			var login, ng, nc, fnome string
			var ehOp int
			if rows.Scan(&id, &login, &ng, &nc, &fid, &fnome, &ehOp) == nil {
				out = append(out, map[string]any{
					"id": id, "login": login, "nome_guerra": ng, "nome_completo": nc,
					"funcao_id": fid, "funcao_nome": fnome, "eh_operador": ehOp == 1,
				})
			}
		}
		jsonOK(w, out)
		return
	}

	// POST: designar {login} (conta do setor) como operador
	var req struct {
		Login string `json:"login"`
	}
	if err := decodificar(r, &req); err != nil || req.Login == "" {
		jsonErro(w, http.StatusBadRequest, "login obrigatório")
		return
	}
	login := strings.ToLower(strings.TrimSpace(req.Login))
	var id int64
	var grupo, setor *int64
	err := a.st.db.QueryRow(`SELECT id, grupo_id, setor_id FROM usuarios WHERE login = ? AND ativo = 1`, login).
		Scan(&id, &grupo, &setor)
	if err != nil {
		jsonErro(w, http.StatusNotFound, "conta não encontrada")
		return
	}
	if grupo == nil || *grupo != *u.GrupoID || setor == nil || *setor != *setorID {
		jsonErro(w, http.StatusForbidden, "conta não pertence ao seu setor")
		return
	}
	if _, err := a.st.db.Exec(`INSERT OR IGNORE INTO usuario_papeis (usuario_id, grupo_id, papel) VALUES (?, ?, 'operador')`, id, *u.GrupoID); err != nil {
		jsonErro(w, http.StatusInternalServerError, err.Error())
		return
	}
	a.st.Auditoria(&u.ID, "designar_operador", "usuarios", &id, login, ipDe(r))
	jsonOK(w, map[string]any{"ok": true, "id": id})
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
		w.Header().Set("Cache-Control", "no-store, must-revalidate")
	case strings.HasSuffix(caminho, ".js"):
		w.Header().Set("Content-Type", "text/javascript; charset=utf-8")
		w.Header().Set("Cache-Control", "no-store, must-revalidate")
	case strings.HasSuffix(caminho, ".css"):
		w.Header().Set("Content-Type", "text/css; charset=utf-8")
		w.Header().Set("Cache-Control", "no-store, must-revalidate")
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
	// v9.4: GERENTE exclui OPERADOR/CHEFE_SETOR do próprio grupo; operador/chefe não exclui ninguém.
	if eu.Papel == "operador" || eu.Papel == "chefe_setor" {
		jsonErro(w, http.StatusForbidden, "somente admin e gerente excluem contas")
		return
	}
	if eu.Papel == "gerente" {
		var alvoPapel string
		var alvoGrupo int64
		if err := a.st.db.QueryRow(`SELECT papel, COALESCE(grupo_id,0) FROM usuarios WHERE id = ?`, id).Scan(&alvoPapel, &alvoGrupo); err != nil {
			jsonErro(w, http.StatusNotFound, "usuário inexistente")
			return
		}
		if (alvoPapel != "operador" && alvoPapel != "chefe_setor") || eu.GrupoID == nil || alvoGrupo != *eu.GrupoID {
			jsonErro(w, http.StatusForbidden, "gerente só exclui membros do próprio grupo")
			return
		}
	}
	if papel == "gerente" {
		var gid int64
		if err := a.st.db.QueryRow(`SELECT COALESCE(grupo_id,0) FROM usuarios WHERE id = ?`, id).Scan(&gid); err != nil {
			jsonErro(w, http.StatusNotFound, "usuário inexistente")
			return
		}
		if gid == 0 {
			jsonErro(w, http.StatusConflict, "gerente sem grupo — mova a conta antes de excluir")
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

// hGruposAdd: cria grupo. Criação envolve apenas a escolha de um novo nome.
// Parâmetros de login/senha/nome_guerra são opcionais (para compatibilidade legada).
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

	cod := gerarCodigoGrupo()
	for tenta := 0; tenta < 8; tenta++ {
		var existe int
		_ = a.st.db.QueryRow(`SELECT COUNT(*) FROM grupos WHERE codigo = ?`, cod).Scan(&existe)
		if existe == 0 {
			break
		}
		cod = gerarCodigoGrupo()
	}

	req.Login = strings.ToLower(strings.TrimSpace(req.Login))
	criarGerente := req.Login != "" && len(req.Senha) >= 8

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

	var uid int64
	if criarGerente {
		var existeLogin int
		_ = tx.QueryRow(`SELECT COUNT(*) FROM usuarios WHERE login = ?`, req.Login).Scan(&existeLogin)
		if existeLogin > 0 {
			jsonErro(w, http.StatusBadRequest, "login já existe")
			return
		}
		hash, err := hashSenha(req.Senha)
		if err != nil {
			jsonErro(w, http.StatusInternalServerError, err.Error())
			return
		}
		ng := strings.TrimSpace(req.NomeGuerra)
		if ng == "" {
			ng = req.Login
		}
		precisaSetup := 1
		if req.Senha != "" && req.Senha != "sci" {
			precisaSetup = 0
		}
		res2, err := tx.Exec(`INSERT INTO usuarios (login, senha_hash, papel, grupo_id, nome_guerra, nome_completo, precisa_setup)
			VALUES (?,?,?,?,?,?,?)`,
			req.Login, hash, "gerente", gid, ng, ng, precisaSetup)
		if err != nil {
			jsonErro(w, http.StatusBadRequest, "gerente não criado: "+err.Error())
			return
		}
		uid, _ = res2.LastInsertId()
		_, _ = tx.Exec(`INSERT OR IGNORE INTO usuario_papeis (usuario_id, grupo_id, papel) VALUES (?,?,?)`,
			uid, gid, "gerente")
	}

	if err = tx.Commit(); err != nil {
		jsonErro(w, http.StatusInternalServerError, err.Error())
		return
	}
	u := usuarioDoCtx(r)
	det := req.Nome + " [" + cod + "]"
	if criarGerente {
		det += " gerente=" + req.Login
	}
	a.st.Auditoria(&u.ID, "criar", "grupos", &gid, det, ipDe(r))
	resp := map[string]any{"id": gid, "codigo": cod}
	if uid > 0 {
		resp["gerente_id"] = uid
	}
	jsonOK(w, resp)
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

// hGrupoTrocarGerente (R7): define/promove uma conta a gerente do grupo; qualquer gerente
// anterior vira operador. O usuário selecionado herda a função/papel de Gerente.
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
		jsonErro(w, http.StatusBadRequest, "login do novo gerente obrigatório (ou __REMOVE__ para vagar)")
		return
	}
	req.Login = strings.ToLower(strings.TrimSpace(req.Login))

	removeGerente := req.Login == "__remove__"

	var uid int64
	var papel string
	var uGrupoID *int64

	if !removeGerente {
		if err := a.st.db.QueryRow(`SELECT id, papel, grupo_id FROM usuarios WHERE login = ? AND ativo = 1`,
			req.Login).Scan(&uid, &papel, &uGrupoID); err != nil {
			jsonErro(w, http.StatusNotFound, "conta não encontrada")
			return
		}
		if papel == "gerente" && uGrupoID != nil && *uGrupoID == gid {
			jsonErro(w, http.StatusBadRequest, "esta conta já é o gerente deste grupo")
			return
		}
	}

	tx, err := a.st.db.Begin()
	if err != nil {
		jsonErro(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer tx.Rollback()

	// Rebaixa qualquer gerente anterior do grupo para operador
	if removeGerente {
		_, _ = tx.Exec(`UPDATE usuarios SET papel = 'operador' WHERE grupo_id = ? AND papel = 'gerente' AND ativo = 1`, gid)
		_, _ = tx.Exec(`UPDATE usuario_papeis SET papel = 'operador' WHERE grupo_id = ? AND papel = 'gerente'`, gid)
	} else {
		_, _ = tx.Exec(`UPDATE usuarios SET papel = 'operador' WHERE grupo_id = ? AND papel = 'gerente' AND id <> ? AND ativo = 1`, gid, uid)
		_, _ = tx.Exec(`UPDATE usuario_papeis SET papel = 'operador' WHERE grupo_id = ? AND papel = 'gerente' AND usuario_id <> ?`, gid, uid)

		// Promove o usuário selecionado a Gerente e vincula ao grupo
		if _, err = tx.Exec(`UPDATE usuarios SET papel = 'gerente', grupo_id = ? WHERE id = ?`, gid, uid); err != nil {
			jsonErro(w, http.StatusInternalServerError, err.Error())
			return
		}

		// Atualiza ou insere o papel em usuario_papeis (herança de função)
		var papelID int64
		errP := tx.QueryRow(`SELECT id FROM usuario_papeis WHERE usuario_id = ? AND grupo_id = ?`, uid, gid).Scan(&papelID)
		if errP == nil {
			_, _ = tx.Exec(`UPDATE usuario_papeis SET papel = 'gerente' WHERE id = ?`, papelID)
		} else {
			_, _ = tx.Exec(`INSERT INTO usuario_papeis (usuario_id, grupo_id, papel) VALUES (?, ?, 'gerente')`, uid, gid)
		}
	}

	if err = tx.Commit(); err != nil {
		jsonErro(w, http.StatusInternalServerError, err.Error())
		return
	}
	u := usuarioDoCtx(r)
	a.st.Auditoria(&u.ID, "definir_gerente", "grupos", &gid, "novo="+req.Login, ipDe(r))
	jsonOK(w, map[string]any{"ok": true, "gerente": req.Login})
}

// hGrupoUpdate: atualiza nome e código da unidade/grupo (admin em qualquer; gerente no seu próprio grupo ou subordinados).
func (a *App) hGrupoUpdate(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		jsonErro(w, http.StatusBadRequest, "id inválido")
		return
	}
	u := usuarioDoCtx(r)
	if u == nil {
		jsonErro(w, http.StatusUnauthorized, "não autenticado")
		return
	}
	if u.Papel != "admin" {
		if u.Papel != "gerente" || u.GrupoID == nil || !a.st.EhSubordinado(*u.GrupoID, id) {
			jsonErro(w, http.StatusForbidden, "sem permissão para alterar este grupo")
			return
		}
	}

	var req struct {
		Nome   *string `json:"nome"`
		Codigo *string `json:"codigo"`
	}
	if err := decodificar(r, &req); err != nil {
		jsonErro(w, http.StatusBadRequest, "payload inválido: "+err.Error())
		return
	}

	var nomeAtual, codigoAtual string
	if err := a.st.db.QueryRow(`SELECT nome, COALESCE(codigo,'') FROM grupos WHERE id = ?`, id).Scan(&nomeAtual, &codigoAtual); err != nil {
		jsonErro(w, http.StatusNotFound, "grupo inexistente")
		return
	}

	novoNome := nomeAtual
	if req.Nome != nil {
		trimmed := strings.TrimSpace(*req.Nome)
		if trimmed == "" {
			jsonErro(w, http.StatusBadRequest, "nome do grupo não pode ser vazio")
			return
		}
		var existenteID int64
		if err := a.st.db.QueryRow(`SELECT id FROM grupos WHERE LOWER(nome) = LOWER(?) AND id <> ?`, trimmed, id).Scan(&existenteID); err == nil {
			jsonErro(w, http.StatusBadRequest, "já existe outro grupo com este nome")
			return
		}
		novoNome = trimmed
	}

	novoCodigo := codigoAtual
	if req.Codigo != nil {
		trimmedCod := strings.ToUpper(strings.TrimSpace(*req.Codigo))
		if trimmedCod != "" {
			var existenteID int64
			if err := a.st.db.QueryRow(`SELECT id FROM grupos WHERE UPPER(codigo) = ? AND id <> ?`, trimmedCod, id).Scan(&existenteID); err == nil {
				jsonErro(w, http.StatusBadRequest, "já existe outro grupo com este código")
				return
			}
		}
		novoCodigo = trimmedCod
	}

	if _, err := a.st.db.Exec(`UPDATE grupos SET nome = ?, codigo = ? WHERE id = ?`, novoNome, novoCodigo, id); err != nil {
		jsonErro(w, http.StatusInternalServerError, "erro ao atualizar grupo: "+err.Error())
		return
	}

	detalhes := fmt.Sprintf("nome=%s codigo=%s", novoNome, novoCodigo)
	a.st.Auditoria(&u.ID, "editar", "grupos", &id, detalhes, ipDe(r))

	jsonOK(w, map[string]any{
		"ok":     true,
		"id":     id,
		"nome":   novoNome,
		"codigo": novoCodigo,
	})
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

// =====================================================================
// MÓDULO DE ESCALAS E SERVIÇOS (v1.0)
// =====================================================================

func (a *App) escaladosNaData(grupoID int64, data string) []map[string]any {
	q := `SELECT ep.pessoa_id, et.id, et.tipo_id, COALESCE(etp.nome, ''), COALESCE(ep.funcao_escala, ''), et.data_inicio, et.data_fim,
	             p.nome_guerra, p.nome_completo, COALESCE(s.nome, ''), COALESCE(f.nome, '')
	      FROM escala_pessoas ep
	      JOIN escala_turnos et ON et.id = ep.turno_id
	      JOIN escala_tipos etp ON etp.id = et.tipo_id
	      JOIN pessoas p ON p.id = ep.pessoa_id
	      LEFT JOIN setores s ON s.id = p.setor_id
	      LEFT JOIN funcoes f ON f.id = p.funcao_id
	      WHERE (? <= 0 OR et.grupo_id = ?)
	        AND substr(et.data_inicio, 1, 10) <= ? AND substr(COALESCE(NULLIF(et.data_fim, ''), et.data_inicio), 1, 10) >= ?
	      ORDER BY et.id, p.nome_guerra`
	rows, err := a.st.db.Query(q, grupoID, grupoID, data, data)
	if err != nil {
		return []map[string]any{}
	}
	defer rows.Close()
	var res []map[string]any
	for rows.Next() {
		var pid, tid, tipoID int64
		var tipoNome, funcaoEscala, dtIni, dtFim, ng, nc, setor, funcao string
		if err := rows.Scan(&pid, &tid, &tipoID, &tipoNome, &funcaoEscala, &dtIni, &dtFim, &ng, &nc, &setor, &funcao); err == nil {
			res = append(res, map[string]any{
				"pessoa_id":     pid,
				"turno_id":      tid,
				"tipo_id":       tipoID,
				"tipo_nome":     tipoNome,
				"funcao_escala": funcaoEscala,
				"data_inicio":   dtIni,
				"data_fim":      dtFim,
				"nome_guerra":   ng,
				"nome_completo": nc,
				"setor":         setor,
				"funcao":        funcao,
			})
		}
	}
	return res
}

func (a *App) hEscalasTiposList(w http.ResponseWriter, r *http.Request) {
	u := usuarioDoCtx(r)
	escopo := escopoDoUsuario(u)
	rows, err := a.st.db.Query(
		`SELECT id, COALESCE(grupo_id, 0), nome, COALESCE(descricao, ''), ativo, criado_em
		 FROM escala_tipos
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
		var nome, desc, criada string
		var ativo int
		if rows.Scan(&id, &gid, &nome, &desc, &ativo, &criada) == nil {
			lista = append(lista, map[string]any{
				"id": id, "grupo_id": gid, "nome": nome, "descricao": desc,
				"ativo": ativo == 1, "criado_em": criada,
			})
		}
	}
	jsonOK(w, map[string]any{"tipos": lista})
}

func (a *App) hEscalasTiposAdd(w http.ResponseWriter, r *http.Request) {
	u := usuarioDoCtx(r)
	var req struct {
		ID        int64  `json:"id"`
		Nome      string `json:"nome"`
		Descricao string `json:"descricao"`
		Ativo     *bool  `json:"ativo"`
	}
	if err := decodificar(r, &req); err != nil || strings.TrimSpace(req.Nome) == "" {
		jsonErro(w, http.StatusBadRequest, "Nome do tipo de escala é obrigatório")
		return
	}
	req.Nome = strings.TrimSpace(req.Nome)
	ativo := 1
	if req.Ativo != nil && !*req.Ativo {
		ativo = 0
	}
	if req.ID > 0 {
		_, err := a.st.db.Exec(`UPDATE escala_tipos SET nome = ?, descricao = ?, ativo = ? WHERE id = ?`,
			req.Nome, req.Descricao, ativo, req.ID)
		if err != nil {
			jsonErro(w, http.StatusInternalServerError, err.Error())
			return
		}
		a.st.Auditoria(&u.ID, "editar", "escala_tipos", &req.ID, req.Nome, ipDe(r))
		jsonOK(w, map[string]any{"ok": true, "id": req.ID})
		return
	}
	res, err := a.st.db.Exec(`INSERT INTO escala_tipos (grupo_id, nome, descricao, ativo) VALUES (?, ?, ?, ?)`,
		u.GrupoID, req.Nome, req.Descricao, ativo)
	if err != nil {
		jsonErro(w, http.StatusInternalServerError, err.Error())
		return
	}
	newID, _ := res.LastInsertId()
	a.st.Auditoria(&u.ID, "criar", "escala_tipos", &newID, req.Nome, ipDe(r))
	jsonOK(w, map[string]any{"ok": true, "id": newID})
}

func (a *App) hEscalasTiposDel(w http.ResponseWriter, r *http.Request) {
	u := usuarioDoCtx(r)
	idStr := r.PathValue("id")
	id, _ := strconv.ParseInt(idStr, 10, 64)
	if id <= 0 {
		jsonErro(w, http.StatusBadRequest, "ID inválido")
		return
	}
	var turnosCount int
	_ = a.st.db.QueryRow(`SELECT COUNT(*) FROM escala_turnos WHERE tipo_id = ?`, id).Scan(&turnosCount)
	if turnosCount > 0 {
		_, _ = a.st.db.Exec(`UPDATE escala_tipos SET ativo = 0 WHERE id = ?`, id)
		jsonOK(w, map[string]any{"ok": true, "desativado": true})
		return
	}
	_, err := a.st.db.Exec(`DELETE FROM escala_tipos WHERE id = ?`, id)
	if err != nil {
		jsonErro(w, http.StatusInternalServerError, err.Error())
		return
	}
	a.st.Auditoria(&u.ID, "excluir", "escala_tipos", &id, "", ipDe(r))
	jsonOK(w, map[string]any{"ok": true})
}

// --- Escalas 2.0: Helpers de Permanência, Descanso e Faixa de Posto/Graduação ---

type InfoDescanso struct {
	Nivel        string  `json:"nivel"` // "critico", "alerta", "atencao", "ok"
	HorasFolga   float64 `json:"horas_folga"`
	Mensagem     string  `json:"mensagem"`
	Conflito     bool    `json:"conflito"`
	ConflitoErro string  `json:"conflito_erro"`
}

func parseDataHoraTurno(s string) (time.Time, error) {
	s = strings.TrimSpace(s)
	formatos := []string{
		"2006-01-02T15:04:05",
		"2006-01-02T15:04",
		"2006-01-02 15:04:05",
		"2006-01-02 15:04",
		time.RFC3339,
	}
	for _, f := range formatos {
		if t, err := time.Parse(f, s); err == nil {
			return t, nil
		}
	}
	return time.Time{}, fmt.Errorf("formato de data/hora inválido: %s", s)
}

func (a *App) validarDescansoEscala(pessoaID int64, turnoID int64, dataInicioStr, dataFimStr string) InfoDescanso {
	info := InfoDescanso{Nivel: "ok", HorasFolga: 999, Mensagem: "Descanso adequado"}
	tIni, err1 := parseDataHoraTurno(dataInicioStr)
	tFim, err2 := parseDataHoraTurno(dataFimStr)
	if err1 != nil || err2 != nil {
		return info
	}

	rows, err := a.st.db.Query(`
		SELECT et.id, etp.nome, et.data_inicio, COALESCE(NULLIF(et.data_fim, ''), et.data_inicio)
		FROM escala_pessoas ep
		JOIN escala_turnos et ON et.id = ep.turno_id
		JOIN escala_tipos etp ON etp.id = et.tipo_id
		WHERE ep.pessoa_id = ? AND et.id != ?
	`, pessoaID, turnoID)
	if err != nil {
		return info
	}
	defer rows.Close()

	minFolga := 999999.0
	temOutro := false

	for rows.Next() {
		var oID int64
		var oNome, oIniStr, oFimStr string
		if rows.Scan(&oID, &oNome, &oIniStr, &oFimStr) == nil {
			oIni, e1 := parseDataHoraTurno(oIniStr)
			oFim, e2 := parseDataHoraTurno(oFimStr)
			if e1 != nil || e2 != nil {
				continue
			}

			// 1. Verificação estrita de sobreposição simultânea:
			// Dois intervalos [A_ini, A_fim] e [B_ini, B_fim] colidem se A_ini < B_fim E A_fim > B_ini
			if tIni.Before(oFim) && tFim.After(oIni) {
				info.Conflito = true
				info.ConflitoErro = fmt.Sprintf("Militar já escalado simultaneamente no posto '%s' (%s às %s)",
					oNome, oIni.Format("15:04"), oFim.Format("15:04"))
				info.Nivel = "conflito"
				info.HorasFolga = 0
				info.Mensagem = info.ConflitoErro
				return info
			}

			// 2. Cálculo do descanso (intervalo de folga entre escalas):
			var gap float64 = -1
			if !tIni.Before(oFim) { // este turno é após o outro
				gap = tIni.Sub(oFim).Hours()
			} else if !oIni.Before(tFim) { // este turno é antes do outro
				gap = oIni.Sub(tFim).Hours()
			}

			if gap >= 0 {
				temOutro = true
				if gap < minFolga {
					minFolga = gap
				}
			}
		}
	}

	if !temOutro {
		info.Nivel = "ok"
		info.HorasFolga = 999
		info.Mensagem = "Sem outros serviços próximos registrados"
		return info
	}

	info.HorasFolga = math.Round(minFolga*10) / 10
	if minFolga < 24.0 {
		info.Nivel = "critico"
		info.Mensagem = fmt.Sprintf("🔴 Alerta Crítico: Folga de apenas %.1fh (< 24h) em relação a outro serviço", info.HorasFolga)
	} else if minFolga < 48.0 {
		info.Nivel = "alerta"
		info.Mensagem = fmt.Sprintf("🟠 Alerta: Folga de %.1fh (< 48h) em relação a outro serviço", info.HorasFolga)
	} else if minFolga < 72.0 {
		info.Nivel = "atencao"
		info.Mensagem = fmt.Sprintf("🟡 Atenção: Folga de %.1fh (< 72h) em relação a outro serviço", info.HorasFolga)
	} else {
		info.Nivel = "ok"
		info.Mensagem = fmt.Sprintf("🟢 Descanso adequado (%.1fh)", info.HorasFolga)
	}

	return info
}

func (a *App) obterFuncoesOrdenadas(grupoID int64) []int64 {
	rows, err := a.st.db.Query(`
		SELECT id FROM funcoes 
		WHERE ativo = 1 AND (grupo_id IS NULL OR grupo_id = ?)
		ORDER BY antiguidade ASC, id ASC`, grupoID)
	if err != nil {
		return nil
	}
	defer rows.Close()
	var ids []int64
	for rows.Next() {
		var id int64
		if rows.Scan(&id) == nil {
			ids = append(ids, id)
		}
	}
	return ids
}

func (a *App) verificarFaixaPostoGrad(funcaoID *int64, minID *int64, maxID *int64, funcoesOrdenadas []int64) bool {
	if (minID == nil || *minID <= 0) && (maxID == nil || *maxID <= 0) {
		return true
	}
	if funcaoID == nil || *funcaoID <= 0 {
		return false
	}
	fID := *funcaoID

	posMap := make(map[int64]int)
	for i, id := range funcoesOrdenadas {
		posMap[id] = i
	}

	pPos, okP := posMap[fID]
	if !okP {
		return false
	}

	hasMin := minID != nil && *minID > 0
	hasMax := maxID != nil && *maxID > 0

	if hasMin && hasMax {
		minPos, okMin := posMap[*minID]
		maxPos, okMax := posMap[*maxID]
		if okMin && okMax {
			startPos := minPos
			endPos := maxPos
			if minPos > maxPos {
				startPos = maxPos
				endPos = minPos
			}
			return pPos >= startPos && pPos <= endPos
		}
	}

	if hasMin {
		minPos, okMin := posMap[*minID]
		if okMin && pPos < minPos {
			return false
		}
	}

	if hasMax {
		maxPos, okMax := posMap[*maxID]
		if okMax && pPos > maxPos {
			return false
		}
	}

	return true
}

func (a *App) hEscalasTurnosList(w http.ResponseWriter, r *http.Request) {
	u := usuarioDoCtx(r)
	escopo := escopoDoUsuario(u)
	de := r.URL.Query().Get("de")
	ate := r.URL.Query().Get("ate")
	mes := r.URL.Query().Get("mes")
	dataQ := r.URL.Query().Get("data")

	var condEscopo string
	var args []any
	if escopo <= 0 {
		condEscopo = "1=1"
	} else {
		// Subordinados também enxergam escalas dos grupos superiores (v1.5)
		superiores := a.gruposSuperioresAtivos(escopo)
		gids := append([]int64{escopo}, superiores...)
		ph := strings.TrimSuffix(strings.Repeat("?,", len(gids)), ",")
		condEscopo = fmt.Sprintf("(et.grupo_id IN (%s) OR et.grupo_delegado_id = ?)", ph)
		for _, gid := range gids {
			args = append(args, gid)
		}
		args = append(args, escopo)
	}

	q := fmt.Sprintf(`SELECT et.id, et.grupo_id, et.tipo_id, etp.nome, et.data_inicio, et.data_fim, COALESCE(et.observacao,''),
	             COALESCE(u.login,''), et.criado_em, COALESCE(g.nome, ''),
	             COALESCE(et.fase, 'aberto'), et.modelo_id, et.grupo_delegado_id, COALESCE(et.status_delegacao, 'proprio'),
	             COALESCE(gd.nome, ''), et.posto_grad_min_id, et.posto_grad_max_id,
	             COALESCE(fgmin.nome, ''), COALESCE(fgmax.nome, '')
	      FROM escala_turnos et
	      JOIN escala_tipos etp ON etp.id = et.tipo_id
	      LEFT JOIN usuarios u ON u.id = et.criado_por
	      LEFT JOIN grupos g ON g.id = et.grupo_id
	      LEFT JOIN grupos gd ON gd.id = et.grupo_delegado_id
	      LEFT JOIN funcoes fgmin ON fgmin.id = et.posto_grad_min_id
	      LEFT JOIN funcoes fgmax ON fgmax.id = et.posto_grad_max_id
	      WHERE %s`, condEscopo)

	if mes != "" {
		q += ` AND (et.data_inicio LIKE ? OR et.data_fim LIKE ?)`
		args = append(args, mes+"%", mes+"%")
	} else if de != "" && ate != "" {
		q += ` AND et.data_fim >= ? AND et.data_inicio <= ?`
		args = append(args, de, ate)
	} else if dataQ != "" {
		q += ` AND (et.data_inicio LIKE ? OR (et.data_inicio <= ? AND et.data_fim >= ?))`
		args = append(args, dataQ+"%", dataQ+"T23:59:59", dataQ+"T00:00:00")
	}
	q += ` ORDER BY et.data_inicio DESC, et.id DESC`

	rows, err := a.st.db.Query(q, args...)
	if err != nil {
		jsonErro(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer rows.Close()

	type TurnoItem struct {
		ID                int64            `json:"id"`
		GrupoID           int64            `json:"grupo_id"`
		GrupoNome         string           `json:"grupo_nome"`
		TipoID            int64            `json:"tipo_id"`
		TipoNome          string           `json:"tipo_nome"`
		DataInicio        string           `json:"data_inicio"`
		DataFim           string           `json:"data_fim"`
		Observacao        string           `json:"observacao"`
		CriadoPor         string           `json:"criado_por"`
		CriadoEm          string           `json:"criado_em"`
		Fase              string           `json:"fase"`
		ModeloID          *int64           `json:"modelo_id"`
		GrupoDelegadoID   *int64           `json:"grupo_delegado_id"`
		StatusDelegacao   string           `json:"status_delegacao"`
		GrupoDelegadoNome string           `json:"grupo_delegado_nome"`
		PostoGradMinID    *int64           `json:"posto_grad_min_id"`
		PostoGradMaxID    *int64           `json:"posto_grad_max_id"`
		PostoGradMinNome  string           `json:"posto_grad_min_nome"`
		PostoGradMaxNome  string           `json:"posto_grad_max_nome"`
		Pessoas           []map[string]any `json:"pessoas"`
	}
	turnos := []TurnoItem{}
	var turnoIDs []any
	for rows.Next() {
		var t TurnoItem
		if rows.Scan(&t.ID, &t.GrupoID, &t.TipoID, &t.TipoNome, &t.DataInicio, &t.DataFim, &t.Observacao, &t.CriadoPor, &t.CriadoEm, &t.GrupoNome,
			&t.Fase, &t.ModeloID, &t.GrupoDelegadoID, &t.StatusDelegacao, &t.GrupoDelegadoNome,
			&t.PostoGradMinID, &t.PostoGradMaxID, &t.PostoGradMinNome, &t.PostoGradMaxNome) == nil {
			t.Pessoas = []map[string]any{}
			turnos = append(turnos, t)
			turnoIDs = append(turnoIDs, t.ID)
		}
	}
	rows.Close()

	if len(turnoIDs) > 0 {
		ph := strings.TrimSuffix(strings.Repeat("?,", len(turnoIDs)), ",")
		qP := fmt.Sprintf(`
			SELECT ep.turno_id, ep.pessoa_id, COALESCE(ep.funcao_escala, ''),
			       p.nome_guerra, p.nome_completo, COALESCE(s.nome, ''), COALESCE(fu.nome, '')
			FROM escala_pessoas ep
			JOIN pessoas p ON p.id = ep.pessoa_id
			LEFT JOIN setores s ON s.id = p.setor_id
			LEFT JOIN funcoes fu ON fu.id = p.funcao_id
			WHERE ep.turno_id IN (%s)
			ORDER BY p.nome_guerra`, ph)
		pRows, pErr := a.st.db.Query(qP, turnoIDs...)
		if pErr == nil {
			pessoasPorTurno := map[int64][]map[string]any{}
			for pRows.Next() {
				var tid, pid int64
				var fEscala, ng, nc, setor, funcao string
				if pRows.Scan(&tid, &pid, &fEscala, &ng, &nc, &setor, &funcao) == nil {
					pessoasPorTurno[tid] = append(pessoasPorTurno[tid], map[string]any{
						"pessoa_id":     pid,
						"funcao_escala": fEscala,
						"nome_guerra":   ng,
						"nome_completo": nc,
						"setor":         setor,
						"funcao":        funcao,
					})
				}
			}
			pRows.Close()
			for i := range turnos {
				if pes, ok := pessoasPorTurno[turnos[i].ID]; ok {
					turnos[i].Pessoas = pes
				}
			}
		}
	}
	jsonOK(w, map[string]any{"turnos": turnos})
}

func (a *App) hEscalasTurnosSave(w http.ResponseWriter, r *http.Request) {
	u := usuarioDoCtx(r)
	if u.Papel != "admin" && u.GrupoID == nil {
		jsonErro(w, http.StatusForbidden, "Usuário sem grupo definido")
		return
	}
	var req struct {
		ID             int64  `json:"id"`
		GrupoID        *int64 `json:"grupo_id"`
		TipoID         int64  `json:"tipo_id"`
		DataInicio     string `json:"data_inicio"`
		DataFim        string `json:"data_fim"`
		Observacao     string `json:"observacao"`
		PostoGradMinID *int64 `json:"posto_grad_min_id"`
		PostoGradMaxID *int64 `json:"posto_grad_max_id"`
		Pessoas        *[]struct {
			PessoaID     int64  `json:"pessoa_id"`
			FuncaoEscala string `json:"funcao_escala"`
		} `json:"pessoas"`
	}
	if err := decodificar(r, &req); err != nil || req.TipoID <= 0 || req.DataInicio == "" || req.DataFim == "" {
		jsonErro(w, http.StatusBadRequest, "Tipo de escala e datas de início/fim são obrigatórios")
		return
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
		jsonErro(w, http.StatusBadRequest, "Grupo é obrigatório para o turno de serviço")
		return
	}

	// Validação de sobreposição para pessoas alocadas
	if req.Pessoas != nil {
		for _, p := range *req.Pessoas {
			if p.PessoaID > 0 {
				desc := a.validarDescansoEscala(p.PessoaID, req.ID, req.DataInicio, req.DataFim)
				if desc.Conflito {
					jsonErro(w, http.StatusBadRequest, desc.ConflitoErro)
					return
				}
			}
		}
	}

	tx, err := a.st.db.Begin()
	if err != nil {
		jsonErro(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer tx.Rollback()

	var turnoID = req.ID
	if turnoID > 0 {
		resUpd, err := tx.Exec(`
			UPDATE escala_turnos
			SET tipo_id = ?, data_inicio = ?, data_fim = ?, observacao = ?, posto_grad_min_id = ?, posto_grad_max_id = ?
			WHERE id = ? AND (? = 0 OR grupo_id = ?)`,
			req.TipoID, req.DataInicio, req.DataFim, req.Observacao, req.PostoGradMinID, req.PostoGradMaxID,
			turnoID, escopoDoUsuario(u), grupoID)
		if err != nil {
			jsonErro(w, http.StatusInternalServerError, err.Error())
			return
		}
		if n, _ := resUpd.RowsAffected(); n == 0 {
			jsonErro(w, http.StatusNotFound, "turno não encontrado no seu escopo")
			return
		}
		if req.Pessoas != nil {
			_, _ = tx.Exec(`DELETE FROM escala_pessoas WHERE turno_id = ?`, turnoID)
		}
	} else {
		res, err := tx.Exec(`
			INSERT INTO escala_turnos (grupo_id, tipo_id, data_inicio, data_fim, observacao, criado_por, posto_grad_min_id, posto_grad_max_id)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
			grupoID, req.TipoID, req.DataInicio, req.DataFim, req.Observacao, u.ID, req.PostoGradMinID, req.PostoGradMaxID)
		if err != nil {
			jsonErro(w, http.StatusInternalServerError, err.Error())
			return
		}
		turnoID, _ = res.LastInsertId()
	}

	if req.Pessoas != nil {
		for _, p := range *req.Pessoas {
			if p.PessoaID > 0 {
				_, _ = tx.Exec(`
					INSERT INTO escala_pessoas (turno_id, pessoa_id, funcao_escala)
					VALUES (?, ?, ?)`, turnoID, p.PessoaID, p.FuncaoEscala)
			}
		}
	}

	if err := tx.Commit(); err != nil {
		jsonErro(w, http.StatusInternalServerError, err.Error())
		return
	}
	nPessoas := 0
	if req.Pessoas != nil {
		nPessoas = len(*req.Pessoas)
	}
	a.st.Auditoria(&u.ID, "salvar_turno", "escala_turnos", &turnoID,
		fmt.Sprintf("tipo=%d de=%s ate=%s pessoas=%d", req.TipoID, req.DataInicio, req.DataFim, nPessoas), ipDe(r))
	jsonOK(w, map[string]any{"ok": true, "id": turnoID})
}

func (a *App) hEscalasTurnosDel(w http.ResponseWriter, r *http.Request) {
	u := usuarioDoCtx(r)
	idStr := r.PathValue("id")
	id, _ := strconv.ParseInt(idStr, 10, 64)
	if id <= 0 {
		jsonErro(w, http.StatusBadRequest, "ID inválido")
		return
	}
	escopo := escopoDoUsuario(u)
	res, err := a.st.db.Exec(`DELETE FROM escala_turnos WHERE id = ? AND (? <= 0 OR grupo_id = ?)`, id, escopo, escopo)
	if err != nil {
		jsonErro(w, http.StatusInternalServerError, err.Error())
		return
	}
	ra, _ := res.RowsAffected()
	if ra == 0 {
		jsonErro(w, http.StatusNotFound, "Turno não encontrado ou sem permissão")
		return
	}
	a.st.Auditoria(&u.ID, "excluir", "escala_turnos", &id, "", ipDe(r))
	jsonOK(w, map[string]any{"ok": true})
}

func (a *App) hEscalasHoje(w http.ResponseWriter, r *http.Request) {
	u := usuarioDoCtx(r)
	escopo := escopoDoUsuario(u)
	dataHoje := time.Now().In(a.horaLocal).Format("2006-01-02")
	escalados := a.escaladosNaData(escopo, dataHoje)
	jsonOK(w, map[string]any{"data": dataHoje, "escalados": escalados})
}

// =====================================================================
// ESCALAS 2.0 — MODELOS, APLICAÇÃO, DELEGAÇÃO E MINHAS ESCALAS (v1.5)
// =====================================================================

func (a *App) hEscalasModelosList(w http.ResponseWriter, r *http.Request) {
	u := usuarioDoCtx(r)
	escopo := escopoDoUsuario(u)
	q := `SELECT em.id, em.nome, COALESCE(em.descricao,''), em.ativo, em.criado_em,
	             (SELECT COUNT(*) FROM escala_modelo_postos emp WHERE emp.modelo_id = em.id) AS total_postos,
	             (SELECT COUNT(*) FROM escala_modelo_aptos ema WHERE ema.modelo_id = em.id) AS total_aptos
	      FROM escala_modelos em
	      WHERE (em.grupo_id = ? OR ? <= 0)
	      ORDER BY em.nome ASC`
	rows, err := a.st.db.Query(q, escopo, escopo)
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
		var id int64
		var nome, desc, criadoEm string
		var ativo, postos, aptos int
		if rows.Scan(&id, &nome, &desc, &ativo, &criadoEm, &postos, &aptos) == nil {
			lista = append(lista, map[string]any{
				"id":           id,
				"nome":         nome,
				"descricao":    desc,
				"ativo":        ativo == 1,
				"criado_em":    criadoEm,
				"total_postos": postos,
				"total_aptos":  aptos,
			})
		}
	}
	jsonOK(w, map[string]any{"modelos": lista})
}

func (a *App) hEscalasModelosGet(w http.ResponseWriter, r *http.Request) {
	u := usuarioDoCtx(r)
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id <= 0 {
		jsonErro(w, http.StatusBadRequest, "ID inválido")
		return
	}
	var mod struct {
		ID        int64  `json:"id"`
		GrupoID   int64  `json:"grupo_id"`
		Nome      string `json:"nome"`
		Descricao string `json:"descricao"`
		Ativo     bool   `json:"ativo"`
	}
	var ativoInt int
	err = a.st.db.QueryRow(`SELECT id, grupo_id, nome, COALESCE(descricao,''), ativo FROM escala_modelos WHERE id = ?`, id).
		Scan(&mod.ID, &mod.GrupoID, &mod.Nome, &mod.Descricao, &ativoInt)
	if err != nil {
		jsonErro(w, http.StatusNotFound, "Modelo não encontrado")
		return
	}
	mod.Ativo = ativoInt == 1

	// Fix cia-F8: o DETALHE não era escopado — bravo lia o modelo completo do
	// alpha com PII (nome_completo dos aptos) enquanto a LISTAGEM é escopada.
	// Mesmo guard de dono do Save (cia-F3): admin (escopo<=0) livre.
	var modeloGrupo int64
	if err := a.st.db.QueryRow(`SELECT COALESCE(grupo_id,0) FROM escala_modelos WHERE id = ?`, id).Scan(&modeloGrupo); err != nil {
		jsonErro(w, http.StatusNotFound, "Modelo não encontrado")
		return
	}
	if esc := escopoDoUsuario(u); esc > 0 && modeloGrupo != esc {
		jsonErro(w, http.StatusForbidden, "modelo fora do seu escopo")
		return
	}

	// Postos
	pRows, _ := a.st.db.Query(`
		SELECT emp.id, emp.tipo_id, etp.nome, emp.hora_inicio, emp.hora_fim, emp.quantidade, emp.ordem,
		       emp.posto_grad_min_id, emp.posto_grad_max_id,
		       COALESCE(fgmin.nome, ''), COALESCE(fgmax.nome, '')
		FROM escala_modelo_postos emp
		JOIN escala_tipos etp ON etp.id = emp.tipo_id
		LEFT JOIN funcoes fgmin ON fgmin.id = emp.posto_grad_min_id
		LEFT JOIN funcoes fgmax ON fgmax.id = emp.posto_grad_max_id
		WHERE emp.modelo_id = ?
		ORDER BY emp.ordem ASC, emp.id ASC`, id)
	var postos []map[string]any
	if postos == nil {
		postos = make([]map[string]any, 0) // fix cia-F9: nil marshaliza null — contrato front
	}
	if pRows != nil {
		defer pRows.Close()
		for pRows.Next() {
			var pid, tid int64
			var tnome, hi, hf string
			var qtd, ord int
			var pgMinID, pgMaxID *int64
			var pgMinNome, pgMaxNome string
			if pRows.Scan(&pid, &tid, &tnome, &hi, &hf, &qtd, &ord, &pgMinID, &pgMaxID, &pgMinNome, &pgMaxNome) == nil {
				postos = append(postos, map[string]any{
					"id":                  pid,
					"tipo_id":             tid,
					"tipo_nome":           tnome,
					"hora_inicio":         hi,
					"hora_fim":            hf,
					"quantidade":          qtd,
					"ordem":               ord,
					"posto_grad_min_id":   pgMinID,
					"posto_grad_max_id":   pgMaxID,
					"posto_grad_min_nome": pgMinNome,
					"posto_grad_max_nome": pgMaxNome,
				})
			}
		}
	}

	// Aptos
	aRows, _ := a.st.db.Query(`
		SELECT ema.pessoa_id, p.nome_guerra, p.nome_completo, COALESCE(s.nome,''), COALESCE(fu.nome, '')
		FROM escala_modelo_aptos ema
		JOIN pessoas p ON p.id = ema.pessoa_id
		LEFT JOIN setores s ON s.id = p.setor_id
		LEFT JOIN funcoes fu ON fu.id = p.funcao_id
		WHERE ema.modelo_id = ?
		ORDER BY p.nome_guerra ASC`, id)
	var aptos []map[string]any
	if aptos == nil {
		aptos = make([]map[string]any, 0) // fix cia-F9: nil marshaliza null — contrato front
	}
	if aRows != nil {
		defer aRows.Close()
		for aRows.Next() {
			var pid int64
			var ng, nc, setor, funcao string
			if aRows.Scan(&pid, &ng, &nc, &setor, &funcao) == nil {
				aptos = append(aptos, map[string]any{
					"pessoa_id":     pid,
					"nome_guerra":   ng,
					"nome_completo": nc,
					"setor":         setor,
					"funcao":        funcao,
				})
			}
		}
	}

	jsonOK(w, map[string]any{
		"modelo": mod,
		"postos": postos,
		"aptos":  aptos,
	})
}

func (a *App) hEscalasModelosSave(w http.ResponseWriter, r *http.Request) {
	u := usuarioDoCtx(r)
	escopo := escopoDoUsuario(u)
	if escopo <= 0 {
		jsonErro(w, http.StatusBadRequest, "Usuário deve pertencer a um grupo")
		return
	}
	var req struct {
		ID        int64  `json:"id"`
		Nome      string `json:"nome"`
		Descricao string `json:"descricao"`
		Postos    []struct {
			TipoID         int64  `json:"tipo_id"`
			HoraInicio     string `json:"hora_inicio"`
			HoraFim        string `json:"hora_fim"`
			Quantidade     int    `json:"quantidade"`
			Ordem          int    `json:"ordem"`
			PostoGradMinID *int64 `json:"posto_grad_min_id"`
			PostoGradMaxID *int64 `json:"posto_grad_max_id"`
		} `json:"postos"`
		AptosIDs []int64 `json:"aptos_ids"`
	}
	if err := decodificar(r, &req); err != nil || strings.TrimSpace(req.Nome) == "" {
		jsonErro(w, http.StatusBadRequest, "Nome do modelo é obrigatório")
		return
	}

	tx, err := a.st.db.Begin()
	if err != nil {
		jsonErro(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer tx.Rollback()

	modeloID := req.ID
	if modeloID > 0 {
		// Fix cia-F3: com id alheio, o UPDATE (escopado) não fazia nada (200
		// fantasma) mas os DELETE+INSERT de postos/aptos SEM filtro de grupo
		// REESCREVIAM o modelo da vítima. Checar dono antes de qualquer
		// DELETE/INSERT: dono = grupo_id do modelo == escopo do usuário (admin
		// escopo<=0 livre). A partir daqui os DELETEs operam sobre modelo próprio.
		var modeloGrupo int64
		if err := tx.QueryRow(`SELECT COALESCE(grupo_id,0) FROM escala_modelos WHERE id = ?`, modeloID).Scan(&modeloGrupo); err != nil {
			jsonErro(w, http.StatusNotFound, "Modelo não encontrado")
			return
		}
		if escopo > 0 && modeloGrupo != escopo {
			jsonErro(w, http.StatusForbidden, "modelo fora do seu escopo")
			return
		}
		ra, err := tx.Exec(`UPDATE escala_modelos SET nome = ?, descricao = ? WHERE id = ? AND grupo_id = ?`,
			req.Nome, req.Descricao, modeloID, escopo)
		if err != nil {
			jsonErro(w, http.StatusInternalServerError, err.Error())
			return
		}
		if n, _ := ra.RowsAffected(); n == 0 {
			jsonErro(w, http.StatusNotFound, "Modelo não encontrado no seu escopo")
			return
		}
		_, _ = tx.Exec(`DELETE FROM escala_modelo_postos WHERE modelo_id = ?`, modeloID)
		_, _ = tx.Exec(`DELETE FROM escala_modelo_aptos WHERE modelo_id = ?`, modeloID)
	} else {
		res, err := tx.Exec(`INSERT INTO escala_modelos (grupo_id, nome, descricao, ativo) VALUES (?, ?, ?, 1)`,
			escopo, req.Nome, req.Descricao)
		if err != nil {
			jsonErro(w, http.StatusInternalServerError, err.Error())
			return
		}
		modeloID, _ = res.LastInsertId()
	}

	for _, p := range req.Postos {
		qtd := p.Quantidade
		if qtd <= 0 {
			qtd = 1
		}
		hi := p.HoraInicio
		if hi == "" {
			hi = "07:00"
		}
		hf := p.HoraFim
		if hf == "" {
			hf = "07:00"
		}
		_, err = tx.Exec(`
			INSERT INTO escala_modelo_postos (modelo_id, tipo_id, hora_inicio, hora_fim, quantidade, ordem, posto_grad_min_id, posto_grad_max_id)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
			modeloID, p.TipoID, hi, hf, qtd, p.Ordem, p.PostoGradMinID, p.PostoGradMaxID)
		if err != nil {
			jsonErro(w, http.StatusInternalServerError, err.Error())
			return
		}
	}

	for _, pid := range req.AptosIDs {
		_, _ = tx.Exec(`INSERT OR IGNORE INTO escala_modelo_aptos (modelo_id, pessoa_id) VALUES (?, ?)`, modeloID, pid)
	}

	if err := tx.Commit(); err != nil {
		jsonErro(w, http.StatusInternalServerError, err.Error())
		return
	}

	jsonOK(w, map[string]any{"id": modeloID, "ok": true})
}

func (a *App) hEscalasModelosDel(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id <= 0 {
		jsonErro(w, http.StatusBadRequest, "ID inválido")
		return
	}
	u := usuarioDoCtx(r)
	escopo := escopoDoUsuario(u)
	// Fix cia-F5: modelo referenciado por turnos explodia em 500 FK cru
	// (e ficava permanentemente indeletável). Pre-check: turnos apontando o
	// modelo → 409 com instrução; sem turnos → DELETE normal (dono/escopo).
	var turnos int
	if err := a.st.db.QueryRow(`SELECT COUNT(*) FROM escala_turnos WHERE modelo_id = ?`, id).Scan(&turnos); err != nil {
		jsonErro(w, http.StatusInternalServerError, err.Error())
		return
	}
	if turnos > 0 {
		jsonErro(w, http.StatusConflict, "modelo aplicado em turnos; exclua os turnos primeiro")
		return
	}
	res, err := a.st.db.Exec(`DELETE FROM escala_modelos WHERE id = ? AND (? <= 0 OR grupo_id = ?)`, id, escopo, escopo)
	if err != nil {
		jsonErro(w, http.StatusInternalServerError, err.Error())
		return
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		jsonErro(w, http.StatusNotFound, "Modelo não encontrado")
		return
	}
	jsonOK(w, map[string]any{"ok": true})
}

func (a *App) hEscalasAplicarModelo(w http.ResponseWriter, r *http.Request) {
	u := usuarioDoCtx(r)
	escopo := escopoDoUsuario(u)
	var req struct {
		ModeloID int64  `json:"modelo_id"`
		Data     string `json:"data"` // YYYY-MM-DD
		GrupoID  *int64 `json:"grupo_id"`
	}
	if err := decodificar(r, &req); err != nil || req.ModeloID <= 0 || req.Data == "" {
		jsonErro(w, http.StatusBadRequest, "modelo_id e data são obrigatórios")
		return
	}
	if escopo <= 0 && req.GrupoID != nil && *req.GrupoID > 0 {
		escopo = *req.GrupoID
	}
	if escopo <= 0 {
		_ = a.st.db.QueryRow(`SELECT COALESCE(grupo_id, 0) FROM escala_modelos WHERE id = ?`, req.ModeloID).Scan(&escopo)
	}
	if escopo <= 0 {
		_ = a.st.db.QueryRow(`SELECT id FROM grupos ORDER BY id LIMIT 1`).Scan(&escopo)
	}

	// Buscar postos do modelo com faixas de posto/graduação
	pRows, err := a.st.db.Query(`
		SELECT tipo_id, hora_inicio, hora_fim, quantidade, posto_grad_min_id, posto_grad_max_id
		FROM escala_modelo_postos
		WHERE modelo_id = ?
		ORDER BY ordem ASC, id ASC`, req.ModeloID)
	if err != nil {
		jsonErro(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer pRows.Close()

	type postoDef struct {
		tipoID   int64
		hi, hf   string
		qtd      int
		pgMinID  *int64
		pgMaxID  *int64
	}
	var postos []postoDef
	for pRows.Next() {
		var p postoDef
		if pRows.Scan(&p.tipoID, &p.hi, &p.hf, &p.qtd, &p.pgMinID, &p.pgMaxID) == nil {
			postos = append(postos, p)
		}
	}
	pRows.Close()

	if len(postos) == 0 {
		jsonErro(w, http.StatusBadRequest, "o modelo selecionado não possui postos cadastrados")
		return
	}

	tBase, errDate := time.Parse("2006-01-02", req.Data)
	if errDate != nil {
		jsonErro(w, http.StatusBadRequest, "formato de data inválido (esperado YYYY-MM-DD)")
		return
	}

	tx, err := a.st.db.Begin()
	if err != nil {
		jsonErro(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer tx.Rollback()

	// Fix cia-F4: reaplicar o modelo na MESMA data duplicava turnos idênticos
	// (BUGS_s2 B3: 2 turnos, mesma data/tipo/modelo, sem aviso). MENOR mudança
	// segura = bloquear: turnos do MESMO modelo nesta data → 409 (o chefe tem
	// "limpar-dia" para substituir por decisão própria).
	var jaExistem int
	if err := tx.QueryRow(`SELECT COUNT(*) FROM escala_turnos
		WHERE modelo_id = ? AND substr(data_inicio,1,10) = ?`, req.ModeloID, req.Data).Scan(&jaExistem); err != nil {
		jsonErro(w, http.StatusInternalServerError, err.Error())
		return
	}
	if jaExistem > 0 {
		jsonErro(w, http.StatusConflict, "turnos já existentes para esta data (modelo já aplicado); use limpar-dia antes de reaplicar")
		return
	}

	criados := 0
	for _, p := range postos {
		dataIni := req.Data + "T" + p.hi + ":00"
		dataFimDia := req.Data
		if p.hf <= p.hi {
			dataFimDia = tBase.AddDate(0, 0, 1).Format("2006-01-02")
		}
		dataFim := dataFimDia + "T" + p.hf + ":00"

		for q := 0; q < p.qtd; q++ {
			_, err = tx.Exec(`
				INSERT INTO escala_turnos (grupo_id, tipo_id, data_inicio, data_fim, modelo_id, fase, status_delegacao, criado_por, posto_grad_min_id, posto_grad_max_id)
				VALUES (?, ?, ?, ?, ?, 'aberto', 'proprio', ?, ?, ?)`,
				escopo, p.tipoID, dataIni, dataFim, req.ModeloID, u.ID, p.pgMinID, p.pgMaxID)
			if err != nil {
				jsonErro(w, http.StatusInternalServerError, err.Error())
				return
			}
			criados++
		}
	}

	if err := tx.Commit(); err != nil {
		jsonErro(w, http.StatusInternalServerError, err.Error())
		return
	}

	a.st.Auditoria(&u.ID, "aplicar_modelo", "escala_turnos", &req.ModeloID,
		fmt.Sprintf("data=%s turnos_criados=%d grupo=%d", req.Data, criados, escopo), ipDe(r))
	jsonOK(w, map[string]any{"ok": true, "turnos_criados": criados, "fase": "aberto"})
}

func (a *App) hEscalasLimparDia(w http.ResponseWriter, r *http.Request) {
	u := usuarioDoCtx(r)
	escopo := escopoDoUsuario(u)
	var req struct {
		Data    string `json:"data"` // YYYY-MM-DD
		GrupoID *int64 `json:"grupo_id"`
	}
	if err := decodificar(r, &req); err != nil || req.Data == "" {
		jsonErro(w, http.StatusBadRequest, "data obrigatória")
		return
	}
	if escopo <= 0 && req.GrupoID != nil && *req.GrupoID > 0 {
		escopo = *req.GrupoID
	}

	res, err := a.st.db.Exec(`
		DELETE FROM escala_turnos
		WHERE (? <= 0 OR grupo_id = ?) AND data_inicio LIKE ?`,
		escopo, escopo, req.Data+"%")
	if err != nil {
		jsonErro(w, http.StatusInternalServerError, err.Error())
		return
	}
	n, _ := res.RowsAffected()
	a.st.Auditoria(&u.ID, "limpar_dia", "escala_turnos", nil,
		fmt.Sprintf("data=%s grupo=%d removidos=%d", req.Data, escopo, n), ipDe(r))
	jsonOK(w, map[string]any{"ok": true, "removidos": n})
}

func (a *App) hEscalasTurnoAlocar(w http.ResponseWriter, r *http.Request) {
	turnoID, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || turnoID <= 0 {
		jsonErro(w, http.StatusBadRequest, "ID do turno inválido")
		return
	}
	var req struct {
		PessoaID     *int64 `json:"pessoa_id"` // se nil, desocupa o posto
		FuncaoEscala string `json:"funcao_escala"`
	}
	if err := decodificar(r, &req); err != nil {
		jsonErro(w, http.StatusBadRequest, "requisição inválida")
		return
	}

	u := usuarioDoCtx(r)
	escopo := escopoDoUsuario(u)

	var turno struct {
		ID              int64
		GrupoID         int64
		TipoID          int64
		DataInicio      string
		DataFim         string
		ModeloID        *int64
		GrupoDelegadoID *int64
		PostoGradMinID  *int64
		PostoGradMaxID  *int64
	}
	err = a.st.db.QueryRow(`
		SELECT id, grupo_id, tipo_id, data_inicio, data_fim, modelo_id, grupo_delegado_id, posto_grad_min_id, posto_grad_max_id
		FROM escala_turnos WHERE id = ?`, turnoID).
		Scan(&turno.ID, &turno.GrupoID, &turno.TipoID, &turno.DataInicio, &turno.DataFim,
			&turno.ModeloID, &turno.GrupoDelegadoID, &turno.PostoGradMinID, &turno.PostoGradMaxID)
	if err != nil {
		jsonErro(w, http.StatusNotFound, "Turno não encontrado")
		return
	}

	// Permissão: admin, próprio grupo dono do turno, grupo delegado, ou grupo subordinado visualizando escala superior
	podeAlocar := false
	if u.Papel == "admin" || turno.GrupoID == escopo {
		podeAlocar = true
	} else if turno.GrupoDelegadoID != nil && *turno.GrupoDelegadoID == escopo {
		podeAlocar = true
	} else {
		superiores := a.gruposSuperioresAtivos(escopo)
		if int64Contem(superiores, turno.GrupoID) {
			podeAlocar = true
		}
	}
	if !podeAlocar {
		jsonErro(w, http.StatusForbidden, "Você não tem permissão para gerenciar este posto")
		return
	}

	var alertaDescanso InfoDescanso
	if req.PessoaID != nil && *req.PessoaID > 0 {
		pid := *req.PessoaID

		// 1. Aptos enforcement se turno oriundo de modelo com aptos definidos
		// Nota: Se o posto for delegado para um subgrupo, o subgrupo escala membros da sua própria fração
		ehDelegadoParaSub := turno.GrupoDelegadoID != nil && *turno.GrupoDelegadoID != turno.GrupoID
		if !ehDelegadoParaSub && turno.ModeloID != nil && *turno.ModeloID > 0 {
			var countAptos int
			_ = a.st.db.QueryRow(`SELECT count(*) FROM escala_modelo_aptos WHERE modelo_id = ?`, *turno.ModeloID).Scan(&countAptos)
			if countAptos > 0 {
				var estaApto int
				_ = a.st.db.QueryRow(`SELECT count(*) FROM escala_modelo_aptos WHERE modelo_id = ? AND pessoa_id = ?`, *turno.ModeloID, pid).Scan(&estaApto)
				if estaApto == 0 {
					jsonErro(w, http.StatusBadRequest, "O militar selecionado não está na lista de habilitados/aptos desta escala")
					return
				}
			}
		}

		// 2. Faixa de Posto/Graduação (mínima e máxima)
		if turno.PostoGradMinID != nil || turno.PostoGradMaxID != nil {
			var pFuncaoID *int64
			_ = a.st.db.QueryRow(`SELECT funcao_id FROM pessoas WHERE id = ?`, pid).Scan(&pFuncaoID)
			ord := a.obterFuncoesOrdenadas(turno.GrupoID)
			if !a.verificarFaixaPostoGrad(pFuncaoID, turno.PostoGradMinID, turno.PostoGradMaxID, ord) {
				jsonErro(w, http.StatusBadRequest, "O militar não atende à faixa de Posto/Graduação definida para este posto")
				return
			}
		}

		// 3. Algoritmo de conferência de permanência: sobreposição simultânea e descanso
		alertaDescanso = a.validarDescansoEscala(pid, turno.ID, turno.DataInicio, turno.DataFim)
		if alertaDescanso.Conflito {
			jsonErro(w, http.StatusBadRequest, alertaDescanso.ConflitoErro)
			return
		}
	}

	tx, err := a.st.db.Begin()
	if err != nil {
		jsonErro(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer tx.Rollback()

	_, _ = tx.Exec(`DELETE FROM escala_pessoas WHERE turno_id = ?`, turnoID)

	if req.PessoaID != nil && *req.PessoaID > 0 {
		_, err = tx.Exec(`INSERT INTO escala_pessoas (turno_id, pessoa_id, funcao_escala) VALUES (?, ?, ?)`,
			turnoID, *req.PessoaID, req.FuncaoEscala)
		if err != nil {
			jsonErro(w, http.StatusInternalServerError, err.Error())
			return
		}
		_, _ = tx.Exec(`UPDATE escala_turnos SET status_delegacao = 'preenchido' WHERE id = ?`, turnoID)
	} else {
		_, _ = tx.Exec(`UPDATE escala_turnos SET status_delegacao = 'proprio' WHERE id = ?`, turnoID)
	}

	if err := tx.Commit(); err != nil {
		jsonErro(w, http.StatusInternalServerError, err.Error())
		return
	}

	jsonOK(w, map[string]any{
		"ok":              true,
		"alerta_descanso": alertaDescanso,
	})
}

func (a *App) hEscalasTurnoDelegar(w http.ResponseWriter, r *http.Request) {
	turnoID, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || turnoID <= 0 {
		jsonErro(w, http.StatusBadRequest, "ID do turno inválido")
		return
	}
	var req struct {
		GrupoDelegadoID *int64 `json:"grupo_delegado_id"` // se nil, revoga delegação
	}
	if err := decodificar(r, &req); err != nil {
		jsonErro(w, http.StatusBadRequest, "requisição inválida")
		return
	}

	u := usuarioDoCtx(r)
	escopo := escopoDoUsuario(u)

	if req.GrupoDelegadoID != nil && *req.GrupoDelegadoID > 0 {
		subs := a.gruposSubordinadosAtivos(escopo)
		if !int64Contem(subs, *req.GrupoDelegadoID) {
			jsonErro(w, http.StatusForbidden, "o grupo destino não é subordinado direto ou ativo do seu grupo")
			return
		}
		ra, err := a.st.db.Exec(`
			UPDATE escala_turnos
			SET grupo_delegado_id = ?, status_delegacao = 'delegado'
			WHERE id = ? AND (? <= 0 OR grupo_id = ?)`,
			*req.GrupoDelegadoID, turnoID, escopo, escopo)
		if err != nil {
			jsonErro(w, http.StatusInternalServerError, err.Error())
			return
		}
		// Fix P1 (rodada 04/10): 200 fantasma — UPDATE com WHERE escopo TEM que checar
		// RowsAffected==0 → 404 (lição bd6a7af: 200-sem-efeito esconde falha silenciosa)
		if n, _ := ra.RowsAffected(); n == 0 {
			jsonErro(w, http.StatusNotFound, "turno não encontrado no seu escopo")
			return
		}
	} else {
		ra, err := a.st.db.Exec(`
			UPDATE escala_turnos
			SET grupo_delegado_id = NULL, status_delegacao = 'proprio'
			WHERE id = ? AND (? <= 0 OR grupo_id = ?)`,
			turnoID, escopo, escopo)
		if err != nil {
			jsonErro(w, http.StatusInternalServerError, err.Error())
			return
		}
		if n, _ := ra.RowsAffected(); n == 0 {
			jsonErro(w, http.StatusNotFound, "turno não encontrado no seu escopo")
			return
		}
	}

	jsonOK(w, map[string]any{"ok": true})
}

func (a *App) hEscalasTurnoCandidatos(w http.ResponseWriter, r *http.Request) {
	turnoID, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || turnoID <= 0 {
		jsonErro(w, http.StatusBadRequest, "ID do turno inválido")
		return
	}
	u := usuarioDoCtx(r)
	escopo := escopoDoUsuario(u)

	var turno struct {
		ID              int64  `json:"id"`
		GrupoID         int64  `json:"grupo_id"`
		TipoID          int64  `json:"tipo_id"`
		TipoNome        string `json:"tipo_nome"`
		DataInicio      string `json:"data_inicio"`
		DataFim         string `json:"data_fim"`
		ModeloID        *int64 `json:"modelo_id"`
		GrupoDelegadoID *int64 `json:"grupo_delegado_id"`
		PostoGradMinID  *int64 `json:"posto_grad_min_id"`
		PostoGradMaxID  *int64 `json:"posto_grad_max_id"`
	}
	err = a.st.db.QueryRow(`
		SELECT et.id, et.grupo_id, et.tipo_id, etp.nome, et.data_inicio, et.data_fim,
		       et.modelo_id, et.grupo_delegado_id, et.posto_grad_min_id, et.posto_grad_max_id
		FROM escala_turnos et
		JOIN escala_tipos etp ON etp.id = et.tipo_id
		WHERE et.id = ?`, turnoID).
		Scan(&turno.ID, &turno.GrupoID, &turno.TipoID, &turno.TipoNome, &turno.DataInicio, &turno.DataFim,
			&turno.ModeloID, &turno.GrupoDelegadoID, &turno.PostoGradMinID, &turno.PostoGradMaxID)
	if err != nil {
		jsonErro(w, http.StatusNotFound, "Turno não encontrado")
		return
	}

	// Permissão
	podeVer := false
	if u.Papel == "admin" || turno.GrupoID == escopo {
		podeVer = true
	} else if turno.GrupoDelegadoID != nil && *turno.GrupoDelegadoID == escopo {
		podeVer = true
	} else {
		superiores := a.gruposSuperioresAtivos(escopo)
		if int64Contem(superiores, turno.GrupoID) {
			podeVer = true
		}
	}
	if !podeVer {
		jsonErro(w, http.StatusForbidden, "Acesso não autorizado a este turno")
		return
	}

	// Grupo do qual os militares serão alocados:
	grupoMilitares := escopo
	if grupoMilitares <= 0 {
		grupoMilitares = turno.GrupoID
	}

	// Carregar aptos do modelo (se houver e não for delegado a subgrupo)
	aptosSet := make(map[int64]bool)
	temFiltroAptos := false
	ehDelegadoParaSub := turno.GrupoDelegadoID != nil && *turno.GrupoDelegadoID != turno.GrupoID
	if !ehDelegadoParaSub && turno.ModeloID != nil && *turno.ModeloID > 0 {
		aRows, aErr := a.st.db.Query(`SELECT pessoa_id FROM escala_modelo_aptos WHERE modelo_id = ?`, *turno.ModeloID)
		if aErr == nil {
			for aRows.Next() {
				var pid int64
				if aRows.Scan(&pid) == nil {
					aptosSet[pid] = true
					temFiltroAptos = true
				}
			}
			aRows.Close()
		}
	}

	// Carregar funcoes ordenadas para validação de faixa
	funcoesOrd := a.obterFuncoesOrdenadas(grupoMilitares)

	// Consultar militares ativos do grupo
	pRows, err := a.st.db.Query(`
		SELECT p.id, p.nome_guerra, p.nome_completo, p.funcao_id, COALESCE(fu.nome, ''), COALESCE(s.nome, '')
		FROM pessoas p
		LEFT JOIN funcoes fu ON fu.id = p.funcao_id
		LEFT JOIN setores s ON s.id = p.setor_id
		WHERE p.status = 'ativo' AND (? <= 0 OR p.grupo_id = ?)
		ORDER BY fu.antiguidade ASC, p.nome_guerra ASC`, grupoMilitares, grupoMilitares)
	if err != nil {
		jsonErro(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer pRows.Close()

	type CandidatoItem struct {
		ID                  int64        `json:"id"`
		NomeGuerra          string       `json:"nome_guerra"`
		NomeCompleto        string       `json:"nome_completo"`
		FuncaoID            *int64       `json:"funcao_id"`
		Funcao              string       `json:"funcao"`
		Setor               string       `json:"setor"`
		Apto                bool         `json:"apto"`
		CompativelPostoGrad bool         `json:"compativel_posto_grad"`
		Descanso            InfoDescanso `json:"descanso"`
	}

	var candidatos []CandidatoItem
	for pRows.Next() {
		var c CandidatoItem
		if pRows.Scan(&c.ID, &c.NomeGuerra, &c.NomeCompleto, &c.FuncaoID, &c.Funcao, &c.Setor) == nil {
			if temFiltroAptos {
				c.Apto = aptosSet[c.ID]
			} else {
				c.Apto = true
			}
			c.CompativelPostoGrad = a.verificarFaixaPostoGrad(c.FuncaoID, turno.PostoGradMinID, turno.PostoGradMaxID, funcoesOrd)
			candidatos = append(candidatos, c)
		}
	}
	pRows.Close()

	for i := range candidatos {
		candidatos[i].Descanso = a.validarDescansoEscala(candidatos[i].ID, turno.ID, turno.DataInicio, turno.DataFim)
	}

	jsonOK(w, map[string]any{
		"turno":      turno,
		"candidatos": candidatos,
	})
}

func (a *App) hEscalasAlterarFase(w http.ResponseWriter, r *http.Request) {
	u := usuarioDoCtx(r)
	escopo := escopoDoUsuario(u)
	var req struct {
		Data string `json:"data"` // YYYY-MM-DD
		Fase string `json:"fase"` // aberto | preenchido | aprovado | publicado
	}
	if err := decodificar(r, &req); err != nil || req.Data == "" || req.Fase == "" {
		jsonErro(w, http.StatusBadRequest, "data e fase são obrigatórios")
		return
	}
	switch req.Fase {
	case "aberto", "preenchido", "aprovado", "publicado":
	default:
		jsonErro(w, http.StatusBadRequest, "fase inválida (aberto | preenchido | aprovado | publicado)")
		return
	}

	res, err := a.st.db.Exec(`
		UPDATE escala_turnos
		SET fase = ?
		WHERE grupo_id = ? AND data_inicio LIKE ?`,
		req.Fase, escopo, req.Data+"%")
	if err != nil {
		jsonErro(w, http.StatusInternalServerError, err.Error())
		return
	}
	n, _ := res.RowsAffected()
	// Fix cia-F6: 200 fantasma {"atualizados":0} para não-dono (mesmo padrão do
	// fix de delegar, bd6a7af) — UPDATE sem match TEM que recusar.
	if n == 0 {
		jsonErro(w, http.StatusNotFound, "escala não encontrada no seu escopo")
		return
	}
	jsonOK(w, map[string]any{"ok": true, "atualizados": n, "fase": req.Fase})
}

func (a *App) hEscalasRelatorioDiaPDF(w http.ResponseWriter, r *http.Request) {
	u := usuarioDoCtx(r)
	escopo := escopoDoUsuario(u)
	data := r.URL.Query().Get("data")
	if data == "" {
		data = time.Now().In(a.horaLocal).Format("2006-01-02")
	}

	var grupoNome string
	if escopo > 0 {
		_ = a.st.db.QueryRow(`SELECT nome FROM grupos WHERE id = ?`, escopo).Scan(&grupoNome)
	}
	if grupoNome == "" {
		grupoNome = "Comando Geral"
	}

	var fase string = "aberto"
	_ = a.st.db.QueryRow(`
		SELECT COALESCE(fase, 'aberto')
		FROM escala_turnos
		WHERE (grupo_id = ? OR ? <= 0) AND data_inicio LIKE ?
		ORDER BY id DESC LIMIT 1`, escopo, escopo, data+"%").Scan(&fase)

	q := `SELECT et.id, etp.nome, et.data_inicio, COALESCE(NULLIF(et.data_fim, ''), et.data_inicio),
	             COALESCE(p.nome_guerra, ''), COALESCE(p.nome_completo, ''),
	             COALESCE(s.nome, ''), COALESCE(et.status_delegacao, 'proprio'), COALESCE(gd.nome, ''),
	             COALESCE(fu.nome, '')
	      FROM escala_turnos et
	      JOIN escala_tipos etp ON etp.id = et.tipo_id
	      LEFT JOIN escala_pessoas ep ON ep.turno_id = et.id
	      LEFT JOIN pessoas p ON p.id = ep.pessoa_id
	      LEFT JOIN funcoes fu ON fu.id = p.funcao_id
	      LEFT JOIN setores s ON s.id = p.setor_id
	      LEFT JOIN grupos gd ON gd.id = et.grupo_delegado_id
	      WHERE (et.grupo_id = ? OR ? <= 0)
	        AND (et.data_inicio LIKE ? OR (substr(et.data_inicio, 1, 10) <= ? AND substr(COALESCE(NULLIF(et.data_fim, ''), et.data_inicio), 1, 10) >= ?))
	      ORDER BY et.data_inicio ASC, et.id ASC`

	rows, err := a.st.db.Query(q, escopo, escopo, data+"%", data, data)
	if err != nil {
		jsonErro(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer rows.Close()

	var turnosPDF []EscalaTurnoPDF
	for rows.Next() {
		var id int64
		var posto, di, df, ng, nc, setor, stDeleg, gDeleg, fuNome string
		if rows.Scan(&id, &posto, &di, &df, &ng, &nc, &setor, &stDeleg, &gDeleg, &fuNome) == nil {
			horario := ""
			if len(di) >= 16 && len(df) >= 16 {
				horario = di[11:16] + " às " + df[11:16]
			}
			origem := setor
			if stDeleg == "delegado" {
				if gDeleg != "" {
					origem = "Delegado: " + gDeleg
				} else {
					origem = "Delegado"
				}
			}
			militarNomeCompleto := nc
			militarNomeGuerra := ng
			if fuNome != "" {
				if militarNomeGuerra != "" {
					militarNomeGuerra = fuNome + " " + militarNomeGuerra
				}
				if militarNomeCompleto != "" {
					militarNomeCompleto = fuNome + " " + militarNomeCompleto
				}
			}
			turnosPDF = append(turnosPDF, EscalaTurnoPDF{
				ID:            id,
				PostoNome:     posto,
				Horario:       horario,
				MilitarNome:   militarNomeCompleto,
				MilitarGuerra: militarNomeGuerra,
				SetorOuOrigem: origem,
				Status:        stDeleg,
			})
		}
	}

	pdfData, err := a.gerarEscalaDiaPDF(EscalaDiaPDF{
		Data:      data,
		GrupoNome: grupoNome,
		Fase:      fase,
		GeradoPor: u.Login,
		Turnos:    turnosPDF,
	})
	if err != nil {
		jsonErro(w, http.StatusInternalServerError, "erro ao gerar PDF de escala: "+err.Error())
		return
	}

	w.Header().Set("Content-Type", "application/pdf")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`inline; filename="escala_%s.pdf"`, data))
	w.Header().Set("Content-Length", strconv.Itoa(len(pdfData)))
	_, _ = w.Write(pdfData)
}

func (a *App) hEscalasMinhas(w http.ResponseWriter, r *http.Request) {
	u := usuarioDoCtx(r)
	if u.PessoaID == nil {
		jsonOK(w, map[string]any{
			"escalas_aptas":   []any{},
			"proximos_turnos": []any{},
			"historico":       []any{},
		})
		return
	}
	pid := *u.PessoaID
	hoje := time.Now().In(a.horaLocal).Format("2006-01-02")

	// Escalas aptas
	mRows, _ := a.st.db.Query(`
		SELECT em.id, em.nome, COALESCE(em.descricao,''), COALESCE(g.nome,'')
		FROM escala_modelo_aptos ema
		JOIN escala_modelos em ON em.id = ema.modelo_id
		LEFT JOIN grupos g ON g.id = em.grupo_id
		WHERE ema.pessoa_id = ? AND em.ativo = 1
		ORDER BY em.nome ASC`, pid)
	var escalasAptas []map[string]any
	if mRows != nil {
		defer mRows.Close()
		for mRows.Next() {
			var id int64
			var nome, desc, gNome string
			if mRows.Scan(&id, &nome, &desc, &gNome) == nil {
				escalasAptas = append(escalasAptas, map[string]any{
					"id":         id,
					"nome":       nome,
					"descricao":  desc,
					"grupo_nome": gNome,
				})
			}
		}
	}

	// Próximos turnos escalados
	pRows, _ := a.st.db.Query(`
		SELECT et.id, etp.nome, et.data_inicio, et.data_fim, COALESCE(ep.funcao_escala,''), COALESCE(et.fase,'aberto'), COALESCE(g.nome,'')
		FROM escala_pessoas ep
		JOIN escala_turnos et ON et.id = ep.turno_id
		JOIN escala_tipos etp ON etp.id = et.tipo_id
		LEFT JOIN grupos g ON g.id = et.grupo_id
		WHERE ep.pessoa_id = ? AND et.data_fim >= ?
		ORDER BY et.data_inicio ASC LIMIT 20`, pid, hoje)
	var proximosTurnos []map[string]any
	if pRows != nil {
		defer pRows.Close()
		for pRows.Next() {
			var id int64
			var posto, di, df, fEscala, fase, gNome string
			if pRows.Scan(&id, &posto, &di, &df, &fEscala, &fase, &gNome) == nil {
				proximosTurnos = append(proximosTurnos, map[string]any{
					"turno_id":      id,
					"posto_nome":    posto,
					"data_inicio":   di,
					"data_fim":      df,
					"funcao_escala": fEscala,
					"fase":          fase,
					"grupo_nome":    gNome,
				})
			}
		}
	}

	// Histórico recente (passados)
	hRows, _ := a.st.db.Query(`
		SELECT et.id, etp.nome, et.data_inicio, et.data_fim, COALESCE(ep.funcao_escala,''), COALESCE(g.nome,'')
		FROM escala_pessoas ep
		JOIN escala_turnos et ON et.id = ep.turno_id
		JOIN escala_tipos etp ON etp.id = et.tipo_id
		LEFT JOIN grupos g ON g.id = et.grupo_id
		WHERE ep.pessoa_id = ? AND et.data_fim < ?
		ORDER BY et.data_inicio DESC LIMIT 20`, pid, hoje)
	var historico []map[string]any
	if hRows != nil {
		defer hRows.Close()
		for hRows.Next() {
			var id int64
			var posto, di, df, fEscala, gNome string
			if hRows.Scan(&id, &posto, &di, &df, &fEscala, &gNome) == nil {
				historico = append(historico, map[string]any{
					"turno_id":      id,
					"posto_nome":    posto,
					"data_inicio":   di,
					"data_fim":      df,
					"funcao_escala": fEscala,
					"grupo_nome":    gNome,
				})
			}
		}
	}

	jsonOK(w, map[string]any{
		"escalas_aptas":   escalasAptas,
		"proximos_turnos": proximosTurnos,
		"historico":       historico,
	})
}

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

// =====================================================================
// MÓDULO DE CONFIGURAÇÕES E WHITE-LABEL (v1.0)
// =====================================================================

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

// hConfiguracoesGet / hConfiguracoesSet (v1.0): White-Label. MODO_RESERVA NUNCA
// entra pela API de escrita (chave interna de ativação dos módulos em reserva).
func (a *App) reservaAtivo() bool {
	var v string
	// Fix P1: erro REAL de banco => fail-closed (em reserva). Flag AUSENTE
	// (sql.ErrNoRows) é estado válido por desenho — banco zerado nasce com
	// módulos ativos (contrato provado pela suíte: admin leva 403, não 423).
	if err := a.st.db.QueryRow(`SELECT valor FROM configuracoes WHERE chave = 'MODO_RESERVA'`).Scan(&v); err != nil && err != sql.ErrNoRows {
		return true
	}
	return v == "1"
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

// =====================================================================
// ANEXOS E DOCUMENTOS ESCANEADOS DE CAUTELAS (v1.0)
// =====================================================================

// escopoCautelaID (fix cia-F2): devolve o grupo dono da cautela via JOIN
// cautela→item (material_itens.grupo_id). ErrSQLNoRows = cautela não existe;
// outro erro = falha de leitura. Usar APENAS fora de tx (pool).
func escopoCautelaID(db *sql.DB, cautelaID int64) (int64, error) {
	var itemGrupo int64
	err := db.QueryRow(`SELECT COALESCE(mi.grupo_id,0)
		FROM material_cautelas mc JOIN material_itens mi ON mi.id = mc.item_id
		WHERE mc.id = ?`, cautelaID).Scan(&itemGrupo)
	return itemGrupo, err
}

// cautelaNoEscopo (fix cia-F2): verdadeiro se a cautela é acessível ao usuário
// (admin escopo<=0 vê tudo; conta sem grupo -1 não vê nada). Em recusa, já
// responde 403 (fora do escopo) ou 404 (inexistente) e devolve false.
// Pool apenas — nada de tx aqui (lição bd6a7af).
func (a *App) cautelaNoEscopo(u *Usuario, w http.ResponseWriter, cautelaID int64) bool {
	itemGrupo, err := escopoCautelaID(a.st.db, cautelaID)
	if err != nil {
		jsonErro(w, http.StatusNotFound, "Cautela não encontrada")
		return false
	}
	if esc := escopoDoUsuario(u); esc > 0 && itemGrupo != esc {
		jsonErro(w, http.StatusForbidden, "cautela fora do seu escopo")
		return false
	}
	return true
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

// sanitizarNomeArquivo (Fix P1-1): basename (barras normalizadas), sem aspas nem
// caracteres de controle (Content-DispositionInjection), teto de 120 chars.
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

// =====================================================================
// QR CODE, CONSCIÊNCIA SITUACIONAL & WATCHDOG SLA (v19)
// =====================================================================

func (a *App) hPessoaQRCode(w http.ResponseWriter, r *http.Request) {
	u := usuarioDoCtx(r)
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id <= 0 {
		jsonErro(w, http.StatusBadRequest, "ID de militar inválido")
		return
	}
	var nomeGuerra string
	var gid *int64
	err = a.st.db.QueryRow(`SELECT nome_guerra, grupo_id FROM pessoas WHERE id = ?`, id).Scan(&nomeGuerra, &gid)
	if err != nil {
		jsonErro(w, http.StatusNotFound, "Militar não encontrado")
		return
	}
	if esc := escopoDoUsuario(u); esc > 0 && (gid == nil || *gid != esc) {
		jsonErro(w, http.StatusForbidden, "Acesso restrito ao grupo")
		return
	}
	payload := fmt.Sprintf("sci://p:%d:%s", id, nomeGuerra)
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

func (a *App) hNotificacoesHub(w http.ResponseWriter, r *http.Request) {
	u := usuarioDoCtx(r)
	escopo := escopoDoUsuario(u)

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

func (a *App) iniciarWatchdogSLA() {
	go func() {
		ticker := time.NewTicker(30 * time.Minute)
		for range ticker.C {
			a.verificarAtrasosSLA()
		}
	}()
}

func (a *App) verificarAtrasosSLA() {
	var webhookURL, cfgPrazo string
	_ = a.st.db.QueryRow(`SELECT valor FROM configuracoes WHERE chave = 'WEBHOOK_ATRASOS_URL'`).Scan(&webhookURL)
	// Fix P1-5: webhook é comando de SAÍDA — URL só http/https com host; falhas
	// LOGADAS (antes engolidas: o alerta prometido nunca disparava sem pista).
	if webhookURL != "" {
		if u, err := url.Parse(webhookURL); err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
			log.Printf("SLA webhook: WEBHOOK_ATRASOS_URL inválida (%q) — alerta não disparado", webhookURL)
			webhookURL = ""
		}
	}
	if webhookURL == "" {
		return
	}

	prazoHoras := 24
	_ = a.st.db.QueryRow(`SELECT valor FROM configuracoes WHERE chave = 'CAUTELA_PRAZO_PADRAO_HORAS'`).Scan(&cfgPrazo)
	if p, err := strconv.Atoi(cfgPrazo); err == nil && p > 0 {
		prazoHoras = p
	}

	q := `SELECT mc.id, mi.nome, mi.codigo_patrimonio, p.nome_guerra, mc.data_saida,
	             ROUND((strftime('%s', 'now') - strftime('%s', mc.data_saida)) / 3600.0, 1) as horas_fora
	      FROM material_cautelas mc
	      JOIN material_itens mi ON mi.id = mc.item_id
	      JOIN pessoas p ON p.id = mc.pessoa_id
	      WHERE mc.status = 'ativa'
	        AND (strftime('%s', 'now') - strftime('%s', mc.data_saida)) > (? * 3600)
	      ORDER BY mc.data_saida ASC LIMIT 50`

	rows, err := a.st.db.Query(q, prazoHoras)
	if err != nil {
		return
	}
	defer rows.Close()

	var itens []map[string]any
	for rows.Next() {
		var cid int64
		var iNome, iCod, pGuerra, dts string
		var horas float64
		if rows.Scan(&cid, &iNome, &iCod, &pGuerra, &dts, &horas) == nil {
			itens = append(itens, map[string]any{
				"cautela_id":        cid,
				"item":              iNome,
				"codigo_patrimonio": iCod,
				"responsavel":       pGuerra,
				"saida":             dts,
				"horas_em_aberto":   horas,
			})
		}
	}
	rows.Close() // FIXED: Explicitly close rows to free the single DB connection before HTTP call

	if len(itens) == 0 {
		return
	}

	payload := map[string]any{
		"sistema":        "SCI",
		"evento":         "ALERTA_CAUTELAS_ATRASADAS",
		"total_atrasos":  len(itens),
		"prazo_config_h": prazoHoras,
		"disparado_em":   time.Now().UTC().Format(time.RFC3339),
		"cautelas":       itens,
	}
	corpo, err := json.Marshal(payload)
	if err != nil {
		return
	}

	client := &http.Client{Timeout: 5 * time.Second}
	req, err := http.NewRequest("POST", webhookURL, bytes.NewReader(corpo))
	if err != nil {
		log.Printf("SLA webhook: falha ao montar requisição: %v", err)
		return
	}
	req.Header.Set("Content-Type", "application/json")
	resp, errPost := client.Do(req)
	if errPost != nil {
		log.Printf("SLA webhook: POST falhou para %s: %v", webhookURL, errPost)
		return
	}
	if resp != nil {
		if resp.StatusCode >= 400 {
			log.Printf("SLA webhook: alvo respondeu %d", resp.StatusCode)
		}
		_ = resp.Body.Close()
	}
}

// =====================================================================
// WORKFLOW SETORIAL E CONSCIÊNCIA SITUACIONAL (v1.5)
// =====================================================================

func (a *App) hSetorSugestoesList(w http.ResponseWriter, r *http.Request) {
	u := usuarioDoCtx(r)
	escopo := escopoDoUsuario(u)
	if escopo <= 0 && u.Papel != "admin" {
		jsonErro(w, http.StatusForbidden, "usuário sem grupo definido")
		return
	}

	setorFiltro := strings.TrimSpace(r.URL.Query().Get("setor"))
	statusFiltro := strings.TrimSpace(r.URL.Query().Get("status"))

	q := `SELECT s.id, s.grupo_id, COALESCE(g.nome, ''), s.setor_tipo, s.autor_id,
	             COALESCE(u_aut.nome_guerra, u_aut.login), s.tipo_acao, s.dados_json,
	             s.status, s.aprovado_por, COALESCE(u_apr.nome_guerra, u_apr.login, ''),
	             COALESCE(s.aprovado_em, ''), COALESCE(s.justificativa, ''), s.criado_em
	      FROM setor_sugestoes s
	      JOIN grupos g ON g.id = s.grupo_id
	      JOIN usuarios u_aut ON u_aut.id = s.autor_id
	      LEFT JOIN usuarios u_apr ON u_apr.id = s.aprovado_por
	      WHERE 1=1`
	var args []any

	if escopo > 0 {
		q += ` AND s.grupo_id = ?`
		args = append(args, escopo)
	}
	if setorFiltro != "" {
		q += ` AND s.setor_tipo = ?`
		args = append(args, setorFiltro)
	}
	if statusFiltro != "" {
		q += ` AND s.status = ?`
		args = append(args, statusFiltro)
	}
	q += ` ORDER BY s.id DESC LIMIT 100`

	rows, err := a.st.db.Query(q, args...)
	if err != nil {
		jsonErro(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer rows.Close()

	var lista []map[string]any
	for rows.Next() {
		var id, gid, autorID int64
		var gNome, sTipo, autorNome, tipoAcao, dadosJSON, status, aprovadorNome, aprovadoEm, just, criadoEm string
		var aprovadorID *int64
		if errScan := rows.Scan(&id, &gid, &gNome, &sTipo, &autorID, &autorNome, &tipoAcao,
			&dadosJSON, &status, &aprovadorID, &aprovadorNome, &aprovadoEm, &just, &criadoEm); errScan == nil {
			var dados map[string]any
			_ = json.Unmarshal([]byte(dadosJSON), &dados)
			lista = append(lista, map[string]any{
				"id":             id,
				"grupo_id":       gid,
				"grupo_nome":     gNome,
				"setor_tipo":     sTipo,
				"autor_id":       autorID,
				"autor_nome":     autorNome,
				"tipo_acao":      tipoAcao,
				"dados":          dados,
				"dados_json":     dadosJSON,
				"status":         status,
				"aprovado_por":   aprovadorID,
				"aprovador_nome": aprovadorNome,
				"aprovado_em":    aprovadoEm,
				"justificativa":  just,
				"criado_em":      criadoEm,
			})
		}
	}
	if lista == nil {
		lista = []map[string]any{}
	}
	jsonOK(w, map[string]any{"sugestoes": lista})
}

func (a *App) hSetorSugestoesAdd(w http.ResponseWriter, r *http.Request) {
	u := usuarioDoCtx(r)
	escopo := escopoDoUsuario(u)
	if escopo <= 0 {
		jsonErro(w, http.StatusForbidden, "usuário sem grupo operacional")
		return
	}

	var req struct {
		SetorTipo string         `json:"setor_tipo"` // 'comando', 'pessoal', 'material'
		TipoAcao  string         `json:"tipo_acao"`
		Dados     map[string]any `json:"dados"`
	}
	if err := decodificar(r, &req); err != nil {
		jsonErro(w, http.StatusBadRequest, "corpo inválido")
		return
	}

	req.SetorTipo = strings.ToLower(strings.TrimSpace(req.SetorTipo))
	if req.SetorTipo != "comando" && req.SetorTipo != "pessoal" && req.SetorTipo != "material" {
		jsonErro(w, http.StatusBadRequest, "setor_tipo inválido (deve ser 'comando', 'pessoal' ou 'material')")
		return
	}
	if strings.TrimSpace(req.TipoAcao) == "" {
		jsonErro(w, http.StatusBadRequest, "tipo_acao obrigatório")
		return
	}
	// Fix P1-2 (rodada 04/10): allowlist de tipo_acao na ENTRADA — a sugestão guarda
	// dados_json autoral que vira escrita no banco na aprovação; só os dois tipos
	// conhecidos por aplicarEfeitoSugestao podem entrar na fila.
	req.TipoAcao = strings.ToLower(strings.TrimSpace(req.TipoAcao))
	if req.TipoAcao != "alterar_status_militar" && req.TipoAcao != "atualizar_item_material" {
		jsonErro(w, http.StatusBadRequest, "tipo_acao não permitido (deve ser 'alterar_status_militar' ou 'atualizar_item_material')")
		return
	}

	dj, err := json.Marshal(req.Dados)
	if err != nil {
		jsonErro(w, http.StatusBadRequest, "falha ao serializar dados")
		return
	}

	res, err := a.st.db.Exec(`
		INSERT INTO setor_sugestoes (grupo_id, setor_tipo, autor_id, tipo_acao, dados_json, status)
		VALUES (?, ?, ?, ?, ?, 'pendente')`,
		escopo, req.SetorTipo, u.ID, req.TipoAcao, string(dj))
	if err != nil {
		jsonErro(w, http.StatusInternalServerError, "falha ao salvar sugestão: "+err.Error())
		return
	}

	id, _ := res.LastInsertId()
	a.st.Auditoria(&u.ID, "sugestao_criar", "setor_sugestoes", &id, fmt.Sprintf("setor=%s acao=%s", req.SetorTipo, req.TipoAcao), ipDe(r))

	jsonOK(w, map[string]any{
		"ok":         true,
		"id":         id,
		"mensagem":   "Sugestão encaminhada com sucesso para apreciação do Chefe de Setor",
		"status":     "pendente",
		"setor_tipo": req.SetorTipo,
	})
}

// idDeJSON converte id vindo de dados_json (float64/int64/string) para int64; 0 = inválido.
func idDeJSON(v any) int64 {
	switch n := v.(type) {
	case float64:
		return int64(n)
	case int64:
		return n
	case int:
		return int64(n)
	case json.Number:
		i, _ := n.Int64()
		return i
	case string:
		i, err := strconv.ParseInt(strings.TrimSpace(n), 10, 64)
		if err != nil {
			return 0
		}
		return i
	}
	return 0
}

// executorSQL cobre *sql.DB e *sql.Tx (mesma superfície Exec/QueryRow).
// Lição bd6a7af: dentro de transação, consulta no pool (`a.st.db`) com SQLite de
// conexão única = DEADLOCK — o efeito da sugestão lê e escreve pela PRÓPRIA tx.
type executorSQL interface {
	Exec(query string, args ...any) (sql.Result, error)
	QueryRow(query string, args ...any) *sql.Row
}

// hSetorSugestoesAvaliar aplica a chancela do chefe sobre uma sugestão de setor.
// FIX P1 (rodada 04/10): a aplicação de efeitos REVALIDA o escopo do alvo (o grupo
// da sugestão tem que conter a pessoa/item) e usa ALLOWLIST de status — antes,
// dados_json autoral virava UPDATE sem escopo em pessoas/material_itens (escrita
// cross-group) e qualquer string virava "novo status".
func (a *App) aplicarEfeitoSugestao(tipoAcao string, dados map[string]any, escopo int64) (int64, string, error) {
	return aplicarEfeitoSugestaoTx(a.st.db, tipoAcao, dados, escopo)
}

func aplicarEfeitoSugestaoTx(ex executorSQL, tipoAcao string, dados map[string]any, escopo int64) (int64, string, error) {
	if tipoAcao == "alterar_status_militar" {
		pID := idDeJSON(dados["pessoa_id"])
		novoStatus, _ := dados["novo_status"].(string)
		novoStatus = strings.ToLower(strings.TrimSpace(novoStatus))
		if pID <= 0 || novoStatus == "" {
			return 0, "", fmt.Errorf("dados incompletos (pessoa_id/novo_status)")
		}
		if novoStatus != "ativo" && novoStatus != "inativo" {
			return 0, "", fmt.Errorf("status de militar não permitido: %q", novoStatus)
		}
		var gid int64
		if err := ex.QueryRow(`SELECT COALESCE(grupo_id,0) FROM pessoas WHERE id = ?`, pID).Scan(&gid); err != nil {
			return 0, "", fmt.Errorf("militar %d não encontrado", pID)
		}
		if escopo > 0 && gid != escopo {
			return 0, "", fmt.Errorf("militar %d fora do grupo da sugestão", pID)
		}
		if escopo <= 0 {
			return 0, "", fmt.Errorf("sem grupo operacional para aplicar alteração de militar")
		}
		if _, err := ex.Exec(`UPDATE pessoas SET status = ? WHERE id = ?`, novoStatus, pID); err != nil {
			return 0, "", err
		}
		return pID, "militar", nil
	}
	if tipoAcao == "atualizar_item_material" {
		itemID := idDeJSON(dados["item_id"])
		novoStatus, _ := dados["status"].(string)
		novoStatus = strings.ToLower(strings.TrimSpace(novoStatus))
		if itemID <= 0 || novoStatus == "" {
			return 0, "", fmt.Errorf("dados incompletos (item_id/status)")
		}
		if novoStatus != "disponivel" && novoStatus != "acautelado" && novoStatus != "manutencao" && novoStatus != "baixado" {
			return 0, "", fmt.Errorf("status de item não permitido: %q", novoStatus)
		}
		var gid int64
		if err := ex.QueryRow(`SELECT COALESCE(grupo_id,0) FROM material_itens WHERE id = ?`, itemID).Scan(&gid); err != nil {
			return 0, "", fmt.Errorf("item %d não encontrado", itemID)
		}
		if escopo > 0 && gid != escopo {
			return 0, "", fmt.Errorf("item %d fora do grupo da sugestão", itemID)
		}
		if escopo <= 0 {
			return 0, "", fmt.Errorf("sem grupo operacional para aplicar alteração de item")
		}
		if _, err := ex.Exec(`UPDATE material_itens SET status = ? WHERE id = ?`, novoStatus, itemID); err != nil {
			return 0, "", err
		}
		return itemID, "item", nil
	}
	return 0, "", fmt.Errorf("tipo de ação desconhecido: %q", tipoAcao)
}

func (a *App) hSetorSugestoesAvaliar(w http.ResponseWriter, r *http.Request) {
	u := usuarioDoCtx(r)
	escopo := escopoDoUsuario(u)

	// Apenas chefes (gerente, chefe_setor ou admin) podem avaliar sugestões
	if u.Papel != "admin" && u.Papel != "gerente" && u.Papel != "chefe_setor" {
		jsonErro(w, http.StatusForbidden, "Apenas o Chefe de Setor ou Gerente pode aprovar/rejeitar sugestões")
		return
	}

	sugID, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || sugID <= 0 {
		jsonErro(w, http.StatusBadRequest, "ID da sugestão inválido")
		return
	}

	var req struct {
		Acao          string `json:"acao"` // 'aprovar' ou 'rejeitar'
		Justificativa string `json:"justificativa"`
	}
	if err := decodificar(r, &req); err != nil {
		jsonErro(w, http.StatusBadRequest, "corpo inválido")
		return
	}

	req.Acao = strings.ToLower(strings.TrimSpace(req.Acao))
	if req.Acao != "aprovar" && req.Acao != "rejeitar" {
		jsonErro(w, http.StatusBadRequest, "ação inválida (deve ser 'aprovar' ou 'rejeitar')")
		return
	}

	var gid int64
	var statusAtual, sTipo, tipoAcao, dadosJSON string
	err = a.st.db.QueryRow(`
		SELECT grupo_id, status, setor_tipo, tipo_acao, dados_json
		FROM setor_sugestoes WHERE id = ?`, sugID).Scan(&gid, &statusAtual, &sTipo, &tipoAcao, &dadosJSON)
	if err != nil {
		jsonErro(w, http.StatusNotFound, "sugestão não encontrada")
		return
	}

	if escopo > 0 && gid != escopo && u.Papel != "admin" {
		jsonErro(w, http.StatusForbidden, "sugestão fora do seu grupo")
		return
	}
	if statusAtual != "pendente" {
		jsonErro(w, http.StatusConflict, "sugestão já foi avaliada anteriormente ("+statusAtual+")")
		return
	}

	agora := time.Now().UTC().Format(time.RFC3339)
	novoStatus := "rejeitado"
	if req.Acao == "aprovar" {
		novoStatus = "aprovado"
	}

	// Fix P1-2 (rodada 04/10): transação única — efeito + chancela nascem e morrem juntos.
	// Em erro de efeito a sugestão NÃO vira 'aprovada' (fica pendente) e o motivo volta
	// ao chefe (409). Guard TOCTOU: WHERE status='pendente' + RowsAffected==0 → 409
	// 'já avaliada' (avaliação concorrente não reaplica efeito nem sobrescreve chancela).
	tx, err := a.st.db.Begin()
	if err != nil {
		jsonErro(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer tx.Rollback()

	// Se aprovado, o resultado oficial fica em nome do Chefe de Setor que aprovou!
	// WHERE status='pendente' mata o TOCTOU: se avaliada concorrentemente, RowsAffected=0.
	ra, err := tx.Exec(`
		UPDATE setor_sugestoes
		SET status = ?, aprovado_por = ?, aprovado_em = ?, justificativa = ?
		WHERE id = ? AND status = 'pendente'`, novoStatus, u.ID, agora, req.Justificativa, sugID)
	if err != nil {
		jsonErro(w, http.StatusInternalServerError, "falha ao atualizar status: "+err.Error())
		return
	}
	if n, _ := ra.RowsAffected(); n == 0 {
		jsonErro(w, http.StatusConflict, "sugestão já avaliada")
		return
	}

	// Se aprovado, aplicar a ação correspondente no banco com autoria do Chefe.
	// Escopo do alvo + allowlist revalidados AQUI (momento da aplicação) — não confiar
	// no checkpoint da criação. Falha de efeito → rollback: sugestão segue pendente.
	if req.Acao == "aprovar" {
		var dados map[string]any
		_ = json.Unmarshal([]byte(dadosJSON), &dados)
		if _, _, err := aplicarEfeitoSugestaoTx(tx, tipoAcao, dados, escopo); err != nil {
			jsonErro(w, http.StatusConflict, "falha ao aplicar efeito da sugestão: "+err.Error())
			return
		}
	}
	if err := tx.Commit(); err != nil {
		jsonErro(w, http.StatusInternalServerError, "falha ao concluir avaliação: "+err.Error())
		return
	}

	chefeNome := u.NomeGuerra
	if chefeNome == "" {
		chefeNome = u.NomeCompleto
	}
	if chefeNome == "" {
		chefeNome = u.Login
	}

	a.st.Auditoria(&u.ID, "sugestao_"+novoStatus, "setor_sugestoes", &sugID,
		fmt.Sprintf("avaliado por chefe=%s justificativa=%s", chefeNome, req.Justificativa), ipDe(r))

	jsonOK(w, map[string]any{
		"ok":             true,
		"id":             sugID,
		"status":         novoStatus,
		"aprovador_id":   u.ID,
		"aprovador_nome": chefeNome,
		"aprovado_em":    agora,
		"mensagem":       fmt.Sprintf("Sugestão %s com sucesso com chancela oficial de %s", novoStatus, chefeNome),
	})
}

func (a *App) hConscienciaResumo(w http.ResponseWriter, r *http.Request) {
	u := usuarioDoCtx(r)
	escopo := escopoDoUsuario(u)

	// Buscar dados do próprio grupo e subordinados imediatos/recursivos
	var grupoPrincipalNome string
	if escopo > 0 {
		_ = a.st.db.QueryRow(`SELECT nome FROM grupos WHERE id = ?`, escopo).Scan(&grupoPrincipalNome)
	} else {
		grupoPrincipalNome = "Comando Geral / Todas as Unidades"
	}

	hoje := time.Now().In(a.horaLocal).Format("2006-01-02")
	ontem := time.Now().In(a.horaLocal).AddDate(0, 0, -1).Format("2006-01-02")

	// Determinar grupos no escopo
	var gruposIDs []int64
	if escopo > 0 {
		gruposIDs = append([]int64{escopo}, a.gruposSubordinadosAtivos(escopo)...)
	} else {
		rows, _ := a.st.db.Query(`SELECT id FROM grupos ORDER BY nome`)
		if rows != nil {
			defer rows.Close()
			for rows.Next() {
				var gid int64
				if rows.Scan(&gid) == nil {
					gruposIDs = append(gruposIDs, gid)
				}
			}
		}
	}

	// Métricas agregadas
	var totalEfetivo, totalPresentes, totalEscaladosHoje, totalMateriais, totalCautelasAbertas, totalConferenciasFechadas int

	type SubordinadoResumo struct {
		ID                   int64  `json:"id"`
		Nome                 string `json:"nome"`
		Efetivo              int    `json:"efetivo"`
		PresentesHoje        int    `json:"presentes_hoje"`
		FaltasHoje           int    `json:"faltas_hoje"`
		StatusConferencia    string `json:"status_conferencia"` // 'fechada', 'aberta', 'pendente'
		ConferenciaID        *int64 `json:"conferencia_id,omitempty"`
		MateriaisAcautelados int    `json:"materiais_acautelados"`
	}

	var listaSub []SubordinadoResumo

	for _, gid := range gruposIDs {
		var gNome string
		var ef int
		_ = a.st.db.QueryRow(`SELECT nome FROM grupos WHERE id = ?`, gid).Scan(&gNome)
		_ = a.st.db.QueryRow(`SELECT COUNT(*) FROM pessoas WHERE grupo_id = ? AND status = 'ativo'`, gid).Scan(&ef)
		totalEfetivo += ef

		// Conferência de hoje do grupo
		var confID int64
		var confStatus string
		var presentes, faltas int
		errConf := a.st.db.QueryRow(`
			SELECT id, status FROM conferencias
			WHERE grupo_id = ? AND data = ?
			ORDER BY id DESC LIMIT 1`, gid, hoje).Scan(&confID, &confStatus)

		statusConf := "pendente"
		var pConfID *int64
		if errConf == nil {
			statusConf = confStatus
			pConfID = &confID
			if confStatus == "fechada" {
				totalConferenciasFechadas++
			}
			_ = a.st.db.QueryRow(`SELECT COUNT(*) FROM presencas WHERE conferencia_id = ? AND situacao = 'presente'`, confID).Scan(&presentes)
			_ = a.st.db.QueryRow(`SELECT COUNT(*) FROM presencas WHERE conferencia_id = ? AND situacao IN ('falta', 'justificada')`, confID).Scan(&faltas)
		}
		totalPresentes += presentes

		// Materiais acautelados no grupo
		var acautelados int
		_ = a.st.db.QueryRow(`
			SELECT COUNT(*) FROM material_cautelas mc
			JOIN material_itens mi ON mi.id = mc.item_id
			WHERE mi.grupo_id = ? AND mc.status = 'ativa'`, gid).Scan(&acautelados)
		totalCautelasAbertas += acautelados

		var totalMatGrupo int
		_ = a.st.db.QueryRow(`SELECT COUNT(*) FROM material_itens WHERE grupo_id = ?`, gid).Scan(&totalMatGrupo)
		totalMateriais += totalMatGrupo

		listaSub = append(listaSub, SubordinadoResumo{
			ID:                   gid,
			Nome:                 gNome,
			Efetivo:              ef,
			PresentesHoje:        presentes,
			FaltasHoje:           faltas,
			StatusConferencia:    statusConf,
			ConferenciaID:        pConfID,
			MateriaisAcautelados: acautelados,
		})
	}

	// Escalados de hoje em todas as unidades do escopo
	if len(gruposIDs) > 0 {
		ph := strings.TrimSuffix(strings.Repeat("?,", len(gruposIDs)), ",")
		args := make([]any, len(gruposIDs)+1)
		for i, g := range gruposIDs {
			args[i] = g
		}
		args[len(gruposIDs)] = hoje
		_ = a.st.db.QueryRow(`
			SELECT COUNT(DISTINCT ep.pessoa_id)
			FROM escala_pessoas ep
			JOIN escala_turnos et ON et.id = ep.turno_id
			WHERE et.grupo_id IN (`+ph+`) AND substr(et.data_inicio, 1, 10) <= ? AND substr(et.data_fim, 1, 10) >= ?`,
			append(args, hoje)...).Scan(&totalEscaladosHoje)
	}

	// Sugestões pendentes
	var sugestoesPendentes int
	if escopo > 0 {
		_ = a.st.db.QueryRow(`SELECT COUNT(*) FROM setor_sugestoes WHERE grupo_id = ? AND status = 'pendente'`, escopo).Scan(&sugestoesPendentes)
	} else {
		_ = a.st.db.QueryRow(`SELECT COUNT(*) FROM setor_sugestoes WHERE status = 'pendente'`).Scan(&sugestoesPendentes)
	}

	jsonOK(w, map[string]any{
		"grupo_id":                  escopo,
		"grupo_nome":                grupoPrincipalNome,
		"data_hoje":                 hoje,
		"data_ontem":                ontem,
		"total_efetivo":             totalEfetivo,
		"total_presentes_hoje":      totalPresentes,
		"total_escalados_hoje":      totalEscaladosHoje,
		"total_materiais":           totalMateriais,
		"total_cautelas_ativas":     totalCautelasAbertas,
		"conferencias_fechadas":     totalConferenciasFechadas,
		"sugestoes_pendentes":       sugestoesPendentes,
		"subordinados":              listaSub,
	})
}

