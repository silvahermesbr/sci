package main

// Servidor HTTP do SCI: rotas da API, backup definitivo, front embutido (SPA).

import (
	"database/sql"
	"embed"
	"io/fs"
	"net/http"
	"os"
	"regexp"
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
	confHub   *HubConferencia
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
	a := &App{st: st, lim: NovoLimiter(), mux: http.NewServeMux(), horaLocal: loc, omTitulo: titulo, confHub: NovoHubConferencia()}
	a.rotas()
	a.iniciarWatchdogSLA()
	return a
}

// ---------- helpers JSON ----------

// ---------- backup definitivo ----------

// backupMu serializa o par "checar colisão de nome → VACUUM INTO" dentro do
// processo. Sem ele, 2 backups no mesmo segundo (POST /api/backup simultâneos ou
// backup assíncrono de fechamento × manual) passam os dois pelo os.Stat e o segundo
// VACUUM INTO morre com "output file already exists" (F4, -race: reproduzido).
var backupMu sync.Mutex

// backupAgora cria backups/sci_YYYYMMDD_HHMMSS.db consistente (VACUUM INTO),
// com sha256 + linha no MANIFEST.txt. Todos os dados vivem em arquivo.

// backupFlag registra falha de backup em FLAG_BACKUP.txt (visível no watchdog).

// ---------- migracao v2 (senhas opcionais por usuário) ----------

// ---------- rotas ----------

func (a *App) rotas() {
	m := a.mux
	m.HandleFunc("GET /api/ping", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"ok":true,"timestamp":"` + time.Now().UTC().Format(time.RFC3339) + `"}`))
	})
	a.rotasAdmin()
	a.rotasPessoal()
	a.rotasGrupos()
	a.rotasDrive()
	a.rotasCalendario()
	a.rotasConferencia()
	a.rotasRelatorios()
	a.rotasCatalogo()
	a.rotasEscalas()
	a.rotasMaterial()

	m.Handle("GET /", http.HandlerFunc(a.hSPA))
}

// ---------- handlers básicos ----------

// hTrocarSenha: cada conta troca a PRÓPRIA senha (exige a atual).

// hBackupDownload: entrega o arquivo .db para download no navegador (ordem Tenente 28/09).
var reBackupNome = regexp.MustCompile(`^sci_[0-9]{8}_[0-9]{6}(_[0-9]{2})?\.db$`)

// hUsuarioSenha: admin redefine a senha de qualquer conta; GERENTE redefine a de
// OPERADOR do próprio grupo (v9.4 — aba Gerenciar). Senha de gerente só admin muda.

// ---------- conferências (ciclo completo: iniciar → gravar → fechar → lista) ----------

const tipoConferenciaPadrao = "Conferência de pessoal"

// hConferenciaHoje: contexto para a área de conferência — a conferência ABERTA (se houver)
// + o efetivo ativo + os lançamentos dela.

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

// hEfetivoAtual (v9.15, ordem Tenente 29/09): estado ATUAL de cada militar = estado na ÚLTIMA
// conferência em que foi lançado (não agregação de período). Usado no dashboard de relatórios
// com alertas de frescor: >1 dia = amarelo; >1 semana = vermelho (calculado no cliente pela data).

// pessoasAtivas(escopo): escopo 0 = todas (admin); N = só do grupo N.

// hConferenciaIniciar: cria a conferência com data/hora de AGORA (Brasília) e a deixa ABERTA.

// hConferenciaFechar: grava os lançamentos e fecha (fechada_em = agora).
// Ordem 06/10 (item 12, NOVA DOUTRINA): FECHAR a conferência do grupo é ato de
// GERENTE ou ENCARREGADO DE PESSOAL — operador e chefe_setor → 403 (o chefe
// conclui/reabre o SEU setor; o operador lança presença). Admin segue proibido.

// hConferenciaSetorConcluir: conclui a conferência setorial (status = 'concluida')

// hConferenciaSetorReabrir: reabre a conferência setorial (status = 'em_andamento')

// ====================== ARQUIVO DE CONFERÊNCIAS (ordem Tenente 30/09) ======================
// FECHADAS × ARQUIVADAS: `conferencias.arquivada_em` (migração v18) marca a arquivada.
// Arquivar = botão do gerente (só conferência FECHADA); excluir arquivada = só ADMIN,
// apaga conferência + presenças + comentários ( NUKE pontual do arquivo).

// migração v18 (ordem Tenente 30/09): arquivo de conferências (arquivada_em) +
// tag_id nos comentários (TAGs do catálogo nos comentários da busca individual)

// hConferenciaArquivar (ordem Tenente 30/09): FECHADA → ARQUIVADA. Gerente do grupo.

// hConferenciaExcluirArquivada (ordem Tenente 30/09): apaga ARQUIVADA — só ADMIN.
// É a única via de destruição de uma conferência fechada (o NUKE de grupo é o outro caso).

// hRegistrosBusca (ordem Tenente 30/09): busca INDIVIDUAL nos relatórios.
// ?pessoa=ID&de=&ate=  → todos os lançamentos da pessoa no período (data, conf, local,
// situação, destino, observação, tags).
// ?filtros=[{"tag":1,"de":"...","ate":"..."},...] → multi-seleção em OU: qualquer regra
// casada entra no resultado (ex.: TAG A na última semana OU TAG B no último mês).

// hTagsDisponiveis: TAGs visíveis no escopo (próprias + herdadas) para montar os filtros.

// hPessoaFicha (ordem Tenente 30/09): dados pessoais para o modal da busca individual.

// hPessoaComentarios: histórico completo de comentários de uma pessoa (para a ficha individual).

// hConferenciaList: conferências DO ESCOPO (grupo não vê grupo; admin vê todas).

// hConferenciaDescartar (v9.14.4, ordem Tenente 29/09): DESCARTAR conferência ABERTA —
// apaga a conferência, presenças parciais e comentários dela. Conferência FECHADA é
// histórico imutável → 409. Escopo: só o grupo dono.

// hCatalogoReparentar (v9.16, ordem Tenente 29/09): hierarquia via DRAG & DROP — muda o pai
// de um item de catálogo. Regras: só gerente; item e novo pai do grupo do gerente (herdados
// de cima não se movem); pai não pode ser descendente do item (anti-ciclo); profundidade ≤ 8.

// hCatalogoEditar (v9.16.11, ordem Tenente 29/09): EDITAR item do catálogo (nome, sigla, cor).
// Só gerente; item do próprio grupo (herdados são read only).

// hConferenciaGet: dados completos de UMA conferência (para o relatório na tela).

// hConferenciaPDF: relatório próprio — SOMENTE de conferência fechada (ordem Tenente 28/09).

// ---------- relatórios ----------

// filtroGrupoSQL/Args: cláusula de escopo por grupo para queries com alias "a"
// (revisão TAKEDA/SHORYU — agregados não podem cruzar grupos).

// gruposSubordinadosAtivos: TODOS os descendentes (transitivo) com vínculo bilateral ativo.

// filtroArvore: escopo do grupo + descendentes (relatórios sobem pela hierarquia).
// FIX S4: retorna struct nomeada (cláusula + args) — evita erro de compilação
// "multiple-value in single-value context" em uso inline dentro de concatenação.
type treeFilter struct {
	clause string
	args   []any
}

// clSetor/argsArvore: cláusula+args da ÁRVORE do escopo (grupo + subordinados ativos).
// No escopo do bundle, "f" cobre conferências; para pessoas (alias p) a regra é a mesma
// coluna grupo_id, então a cláusula gerada é idêntica em forma.

// escopoRelatorio: respeita ?grupo= — admin recorta qualquer grupo; gerente,
// só o próprio ou descendentes.
// FIX S4-P1 (verif5): pedido de grupo FORA da árvore agora responde 403 com
// escopo_aplicado — antes caía em fallback silencioso 200 e mascarava erro.

// hRelatorioDetalhadoPDF (ordem SCI 06/10 — Frente D): PDF por período com
// modo SIMPLES (resumo, igual ao legado) ou DETALHADO (resumo + uma folha por
// grupo: grupo do escopo com coluna SETOR, subordinados em folha própria).
// Rota: GET /api/relatorio/detalhado.pdf?de=&ate=&modo=simples|detalhado[&grupo=]

// ---------- exportação CSV (portabilidade — o dado nunca fica preso) ----------

// ---------- catálogos ----------

// gruposSuperioresAtivos: ids dos grupos superiores com vínculo BILATERAL ativo.

// ---------- pessoas ----------

// hPessoaExcluir (v9.7): admin/gerente EXCLUEM pessoa do banco de pessoal.
// Gerente: só do próprio grupo. Com histórico em conferências (FK presencas) →
// desativa (status='inativo') e responde {desativado:true}; sem histórico → DELETE físico.

// hGrupoExcluir (v9.7): admin EXCLUI grupo — vazio (sem pessoas/contas/vínculos/histórico).
// v9.10: modo FORÇADO (?forcar=1 + senha de admin no corpo) exclui grupo INTEIRO mesmo
// com contas e pessoas: contas do grupo são EXCLUÍDAS, pessoas também, subordinação do
// grupo é removida. Histórico de conferências continua blindado — grupo com conferências
// gravadas NÃO é excluído nem forçado.
// v1.1 (ordem Tenente 30/09): MODO NUKE (?nuke=1 + senha de admin) — exclusão FORÇADA
// TOTAL: apaga TUDO do grupo, INCLUSIVE o histórico de conferências (presenças,
// comentários), catálogos e dados dos módulos em reserva. Reversível? NÃO. A dupla
// confirmação é no FRONT; o servidor prova autoridade com a senha de admin.

// ---------- usuários ----------

// ---------- grupos e escopo (fase GRUPOS — ordem Tenente 28/09) ----------

// escopoDoUsuario: 0 = vê tudo (APENAS admin); -1 = conta SEM grupo (sem acesso a dados
// de grupo — revisão TAKEDA: devolver 0 aqui era escala de privilégio silenciosa).

// podeAdministrar: admin sempre; gerente dentro do próprio grupo.

// hGruposList / hGruposAdd / hGruposDel: gestão de grupos (só admin cria).

// ---------- hierarquia de grupos (ordem Tenente, 28/09 noite) ----------

// hVinculoAdd: vínculo BILATERAL — superior informa código do subordinado e vice-versa.
// O vínculo só existe quando AMBOS registraram o par (consentimento mútuo).

// hVinculoList: pendentes e ativos do MEU grupo.

// hPerfilGet: dados da própria conta (aba Meu usuário).

// hPerfilSet: o próprio usuário atualiza seu perfil completo (dados cadastrais e foto 1x1).

// hUsuarioFotoGet: entrega o stream de imagem da foto de perfil 1x1 do usuário.

// hMoverConta (v9.5): MOVE conta entre grupos. Admin: qualquer origem → qualquer
// destino. Gerente: origem = próprio grupo ou subordinado; destino = próprio grupo
// ou subordinado (dentro da hierarquia dele). Nunca move admin. Gerente não pode
// deixar seu grupo sem gerente (vira operador no destino ou bloqueia se for o único).

// hGruposAdd movido para a seção v9.3 (R7: exige gerente no ato).

// gruposComCodigo: lista com código + vínculos de hierarquia (nomes resolvidos).

// hArvoreGrupos (v9.4): árvore NESTED dos grupos. Admin: floresta completa a partir
// das raízes. Gerente/operador: próprio grupo como raiz + subordinados (recursivo).
// READ ONLY — subordinação só no painel admin.
// v9.10: efetivo RECURSIVO — cada nó carrega o efetivo próprio + soma de toda a
// subárvore (o painel mostra o total agregado; ao expandir, o valor se disseca por nível).

// hComentariosAdd: comentário append-only sobre pessoa em conferência (ordem Tenente).

// hOperadoresDoSetor (ordem 04/10 — Fase G3): o CHEFE DE SETOR não cria
// operadores — SELECIONA dentre as contas presentes no SEU setor. GET lista
// os candidatos do setor; POST designa a conta como operador (INSERT do papel
// na mesma linha, sem mudar o papel principal da conta).

// ---------- SPA ----------

func (a *App) hSPA(w http.ResponseWriter, r *http.Request) {
	sub, err := fs.Sub(webEmbed, "web")
	if err != nil {
		http.Error(w, "assets ausentes", http.StatusInternalServerError)
		return
	}
	caminho := strings.TrimPrefix(r.URL.Path, "/")
	if caminho == "" {
		// Landing pública (ordem do Diretor 08/10/26): a raiz serve a página
		// institucional com botão de login; a SPA continua em qualquer
		// #rota (login incluído). Visitante já autenticado vai direto à SPA.
		if r.URL.Query().Get("login") != "1" {
			if _, err := fs.Stat(sub, "landing.html"); err == nil {
				if ck, err := r.Cookie(cookieSessao); err != nil || ck.Value == "" {
					caminho = "landing.html"
				}
			}
		}
		if caminho == "" {
			caminho = "index.html"
		}
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

// hGruposAdd: cria grupo. Criação envolve apenas a escolha de um novo nome.
// Parâmetros de login/senha/nome_guerra são opcionais (para compatibilidade legada).

// hGrupoGerenteGet: nome do gerente do grupo (painel admin).

// hGrupoTrocarGerente (R7): define/promove uma conta a gerente do grupo; qualquer gerente
// anterior vira operador. O usuário selecionado herda a função/papel de Gerente.

// hGrupoUpdate: atualiza nome e código da unidade/grupo (admin em qualquer; gerente no seu próprio grupo ou subordinados).

// hAdminVinculoSet (R8): subordinação direto do painel admin — grava o vínculo
// completo (criado_por_superior=1 e criado_por_subordinado=1), mesma semântica
// de gruposSubordinadosAtivos.

// hAdminVinculoRem (R8): remove a subordinação (painel admin).

// hBackupImportar (R9): recebe um .db, valida (magic SQLite + integrity_check +
// schema_migrations do arquivo <= schema_migrations do binário), grava backup de
// segurança (VACUUM INTO timestamp), faz o swap atômico (fechar pool → rename →
// reabrir → migrar). Falha em qualquer passo = rejeição SEM tocar o banco.

// =====================================================================
// MÓDULO DE ESCALAS E SERVIÇOS (v1.0)
// =====================================================================

// --- Escalas 2.0: Helpers de Permanência, Descanso e Faixa de Posto/Graduação ---

type InfoDescanso struct {
	Nivel        string  `json:"nivel"` // "critico", "alerta", "atencao", "ok"
	HorasFolga   float64 `json:"horas_folga"`
	Mensagem     string  `json:"mensagem"`
	Conflito     bool    `json:"conflito"`
	ConflitoErro string  `json:"conflito_erro"`
}

// =====================================================================
// ESCALAS 2.0 — MODELOS, APLICAÇÃO, DELEGAÇÃO E MINHAS ESCALAS (v1.5)
// =====================================================================

// =====================================================================
// MÓDULO DE MATERIAL, RESERVA E CAUTELAS (v1.0)
// =====================================================================

// =====================================================================
// MÓDULO DE CONFIGURAÇÕES E WHITE-LABEL (v1.0)
// =====================================================================

// hConfiguracoesGet / hConfiguracoesSet (v1.0): White-Label. MODO_RESERVA NUNCA
// entra pela API de escrita (chave interna de ativação dos módulos em reserva).

// =====================================================================
// ANEXOS E DOCUMENTOS ESCANEADOS DE CAUTELAS (v1.0)
// =====================================================================

// escopoCautelaID (fix cia-F2): devolve o grupo dono da cautela via JOIN
// cautela→item (material_itens.grupo_id). ErrSQLNoRows = cautela não existe;
// outro erro = falha de leitura. Usar APENAS fora de tx (pool).

// cautelaNoEscopo (fix cia-F2): verdadeiro se a cautela é acessível ao usuário
// (admin escopo<=0 vê tudo; conta sem grupo -1 não vê nada). Em recusa, já
// responde 403 (fora do escopo) ou 404 (inexistente) e devolve false.
// Pool apenas — nada de tx aqui (lição bd6a7af).
func (a *App) cautelaNoEscopo(u *Usuario, w http.ResponseWriter, cautelaID int64) bool {
	esc, err := a.exigeEscopo(u)
	if err != nil {
		jsonErro(w, http.StatusForbidden, "conta sem grupo definido")
		return false
	}
	itemGrupo, err := escopoCautelaID(a.st.db, cautelaID)
	if err != nil {
		jsonErro(w, http.StatusNotFound, "Cautela não encontrada")
		return false
	}
	if esc > 0 && itemGrupo != esc {
		jsonErro(w, http.StatusForbidden, "cautela fora do seu escopo")
		return false
	}
	return true
}

// sanitizarNomeArquivo (Fix P1-1): basename (barras normalizadas), sem aspas nem
// caracteres de controle (Content-DispositionInjection), teto de 120 chars.

// =====================================================================
// QR CODE, CONSCIÊNCIA SITUACIONAL & WATCHDOG SLA (v19)
// =====================================================================

// =====================================================================
// WORKFLOW SETORIAL E CONSCIÊNCIA SITUACIONAL (v1.5)
// =====================================================================

// idDeJSON converte id vindo de dados_json (float64/int64/string) para int64; 0 = inválido.

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
