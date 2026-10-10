# SCI — Auditoria Completa de Codebase, Arquitetura, Banco de Dados e Roadmap

**Data:** 2026-10-10 · **Base:** `main` @ `608e5ef` (v1.5.3) · **Método:** leitura direta do código + 4 frentes de auditoria paralelas (banco, segurança, backend, frontend) + execução de build/testes. Achados marcados **[VERIFICADO]** foram conferidos linha a linha no código; os demais vêm das frentes de auditoria com referência de arquivo.

---

## 1. Resultados de testes e build (executados nesta auditoria)

| Verificação | Resultado |
|---|---|
| `go vet ./...` | ✅ limpo |
| `go build` | ✅ limpo |
| `go test -count=1 ./...` | ✅ **ok — 236 testes, 188s** |
| `go test -cover` | ✅ **74,6% de cobertura de statements** |
| `go test -race` | ⚠️ não executável nesta máquina Windows (exige cgo/gcc); rodar no host Linux, onde o projeto já o usa |

**Qualidade dos testes:** integração HTTP de verdade (`httptest` contra o mux), dirigida por personas (gerente/operador/chefe/encarregado), com casos positivos, negativos e de regressão, e asserções diretas no banco (`onda_1010_gap1_test.go` é bom exemplo). Cada teste cria um SQLite temporário e roda a cadeia inteira de migrações — isolamento total e as migrações são exercitadas 236× por execução da suíte.

---

## 2. Veredito executivo

O SCI é um projeto **muito acima da média para o porte e o contexto** (ferramenta militar interna, binário único, equipe mínima, 337 commits em 13 dias): criptografia e sessões corretas, SQL 100% parametrizado, sanitizador server-side, backup com VACUUM INTO + sha256 + MANIFEST, documentação e CHANGELOG disciplinados, 236 testes com 74,6% de cobertura, zero TODO/FIXME pendurado no código.

**Porém, a auditoria encontrou 3 defeitos de classe P0** — nenhum pego pela suíte de testes — concentrados exatamente onde o código cresceu mais rápido que a arquitetura:

1. **Bypass de escopo para contas sem grupo** (`escopo == -1` cai no ramo "vê tudo") — vazamento de dados de pessoal entre grupos.
2. **XSS armazenado via `foto_base64`** — gravação sem validação + renderização sem escape + CSP que não protege.
3. **Importação de backup estruturalmente quebrada** — o swap nunca renomeia o arquivo importado para `sci.db`; a importação se perde no restart; migrações pós-v12 não rodam na reabertura.

Há ainda um conjunto consistente de P1 (NUKE de grupo viola FKs; `hUsuarioPapelDel` fail-open para chefe_setor; fotos de pessoal sem checagem de escopo; Drive servindo HTML inline) e dívida estrutural conhecida (autorização pulverizada, CI que não roda os próprios testes, migrações encadeadas à mão).

A própria suíte é a maior aliada para fechar esses buracos rapidamente: o custo de cada fix é baixo e o harness de teste por persona já existe.

---

## 3. Achados P0 (verificados)

### P0-1 · Bypass de escopo: contas sem grupo leem dados de TODOS os grupos **[VERIFICADO]**

- `escopoDoUsuario` (server_grupos.go:202-213): admin → `0`; não-admin sem grupo → **`-1`** ("sem acesso a dados de grupo" — intenção declarada).
- Mas dezenas de handlers bifurcam apenas em `if escopo > 0 { filtra } else { SEM filtro }` — o `else` foi escrito para o admin (`0`) e **engole o `-1`**.
- Exemplo verificado ponta a ponta: `GET /api/conferencias` → `a.auth(false, ...)` (server_conferencia.go:1584,1599) → `hConferenciaList` (server_conferencia.go:836-855) ramifica só em `escopo > 0` → conta sem grupo recebe a lista de **todos** os grupos.
- O código já conhece o perigo e o corrigiu em pontos isolados (hEfetivoAtual — "conta corrompida sem grupo: não vê ninguém", server_relatorios.go:36-39; hUsuariosList, server_pessoal.go:880-886) — **metade dos handlers foi corrigida, metade não**.
- Alcance: contas criadas pelo admin nascem sem grupo por padrão (server_pessoal.go:1164-1165, 1220-1222). Endpoints afetados incluem ficha/comentários/PDF/QR de pessoas (PII completa: telefone, endereço, tipo sanguíneo, nascimento, foto — server_pessoal.go:407-508), relatórios/registros (server_relatorios.go:79-132), cautelas e anexos de material (server_material.go:190-298, 1739-1771), SSE de conferência (server_realtime.go:88-99), `/api/grupos` (server_grupos.go:382) e exports (`filtroGrupoSQL` devolve `""` para `escopo <= 0`, helpers.go:112-117).
- **Correção sugerida:** guarda central `if esc < 0 → 403` (ou fazer `escopoDoUsuario` retornar um tipo que force o tratamento), varredura de todos os `esc > 0`/`esc <= 0`/`? <= 0`, e um teste de matriz rota×papel com conta sem grupo.

### P0-2 · XSS armazenado via `foto_base64` **[VERIFICADO]**

- Escrita sem qualquer validação: `UPDATE usuarios SET foto_base64 = ?` com só `TrimSpace` (server_pessoal.go:343 no caminho de edição de usuário; hPerfilSet idem em server_pessoal.go:994-1003).
- Sinks sem escape: `<img src="${usuario.foto_base64}">` em web/core.js:580 (sidebar de todos), web/views_gestao.js:548 e :2535 (listas de contas vistas por gerente/admin), web/ui_helpers.js:251.
- Payload `x" onerror="<js>"` via PATCH de perfil próprio dispara na sessão de quem abrir a lista de contas → efeito: download do banco (`/api/backup/download`), NUKE, escalada.
- A CSP **não salva**: `script-src 'self' 'unsafe-inline'` (main.go:111-117 — o próprio comentário admite a limitação).
- **Correção sugerida:** validar no servidor prefixo `data:image/(png|jpeg|webp);base64,` + decodificar + teto de tamanho; `esc()` nos sinks; (médio prazo) migrar handlers inline → CSP com nonce.

### P0-3 · Importação de backup quebrada em 3 pontos **[VERIFICADO]**

`hBackupImportar` (server_admin.go:190-279) + `ReabrirComArquivo` (store.go:1348-1403):

1. **O swap nunca completa:** o banco importado vira `sci.db.novo` e `s.arquivo` passa a apontar para ele — **nenhum código renomeia `.novo` → `sci.db`** (grep confirma: única referência é a criação em server_admin.go:260). No restart, `AbrirStore` abre o `sci.db` original → **a importação inteira se perde silenciosamente**. O teste existente só prova o estado in-process.
2. **Migrações até v12:** `ReabrirComArquivo` re-executa v2…v12 e para (store.go:1372-1402). Importar backup legado válido (schema ≤12, aceito pela validação) deixa o banco sem 30+ versões de schema → 500s em drive/escalas/material/mensagens.
3. **Rollback incorreto:** em falha, `ReabrirComArquivo(a.st.arquivo)` (server_admin.go:269) reabre o **próprio arquivo importado** — `s.arquivo` já foi mutado para o novo caminho antes das migrações (store.go:1354). E `os.Remove(arquivo+"-wal"/"-shm")` (server_admin.go:273-274) roda sobre o caminho **novo** (o da conexão recém-aberta), não sobre o antigo.
- **Correção sugerida:** renomear para `sci.db` dentro do swap (com remoção do WAL/SHM **antigos** antes da reabertura), rodar a cadeia completa de migrações (ver P1-6: registry único), capturar o caminho original antes de mutar `s.arquivo`, e adicionar teste que fecha/reabre o Store após import.

---

## 4. Achados P1

### P1-1 · NUKE de grupo viola FKs das tabelas v1.5 **[VERIFICADO]**

A lista de DELETEs do modo NUKE (server_grupos.go:144-186) cobre o mundo v1.0, mas omite tabelas com FK `NO ACTION` para o que é apagado: `chefe_setores.setor_id`, `funcao_membros.funcao_id/grupo_id/usuario_id`, `avisos.grupo_id/autor_papel_id` (+ `aviso_cientes/comentarios`), `material_conferencias.grupo_id`, `escala_modelos.grupo_id`, `setor_sugestoes.grupo_id`. Resultado: `FOREIGN KEY constraint failed` dentro da tx → 500 com rollback. **Qualquer grupo que já tenha publicado um aviso no mural (habilitado para todos) ou designado chefe/função não pode mais ser nukeado.** O `TestNukeGrupo` não popula essas tabelas, logo o CI não vê. Fix: completar a lista + teste com grupo "rico".

### P1-2 · `hUsuarioPapelDel` fail-open para chefe_setor **[VERIFICADO]**

mensagens.go:246-278: bloqueia `operador` e constrange `gerente`; **chefe_setor e qualquer outro papel caem direto no DELETE** — removem papel de qualquer usuário/grupo e ainda re-chaveiam sessões alheias (`UPDATE sessoes SET papel_ativo_id…`, :275). Espelho: `hUsuarioPapelAdd` não valida que o alvo pertence à árvore do gerente. Fix: allowlist de solicitantes + validação de escopo do alvo nos dois handlers.

### P1-3 · Fotos de usuários sem escopo **[VERIFICADO]**

`GET /api/usuarios/{id}/foto` (server_pessoal.go:1015-1045, rota :1586): qualquer autenticado enumera e baixa fotos 1x1 de **qualquer grupo** (LGPD). A auditoria interna do projeto (docs/auditoria-escopos-pessoal.md) marcou esse endpoint "Conforme" com a justificativa "avatar público" — frágil para dado pessoal de militar. Fix: mesma checagem de grupo das fichas.

### P1-4 · Drive serve HTML inline (2ª via de XSS)

Upload sem allowlist de MIME (drive.go:701-707) e `?inline=1` serve o Content-Type salvo (drive.go:764-774) → arquivo `text/html` compartilhado executa na origem do SCI. O módulo Material faz o correto (allowlist + `attachment` + nosniff, server_material.go:1859-1866, 1991-1999) — replicar no Drive.

### P1-5 · CI não roda os testes **[VERIFICADO]**

`ci.sh` = build + health + backup CLI. Os 236 testes (190s) **não gateiam nada** — com ~26 commits/dia, regressão só é pega se o dev rodar local. Maior alavanca de robustez pelo menor esforço do projeto: adicionar `go vet` + `go test -count=1 ./...` ao ci.sh.

### P1-6 · Migrações: cadeia manual, divergência já ocorrida **[VERIFICADO]**

`AbrirStore` encadeia ~40 `if err := s.migrarVn()` à mão (store.go:56-175) e `ReabrirComArquivo` divergiu (para em v12). Numeração já colidiu em produção (v42→v43, store.go:173). Os 4 rebuilds com FK OFF (v4/v17/v24/v25) **não são transacionais**: crash entre `DROP` e `RENAME` deixa tabela inexistente e pode travar o boot no `foreign_key_check`. Fix: registry `[]func() error` + loop (30 linhas), envolver rebuilds em tx.

### P1-7 · Escritas multi-statement fora de transação

Pior caso: `hMensagensEnviar` (mensagens.go:691-718) — INSERT mensagem + loop de destinatários + UPDATE com `_, _ =` engolido: falha parcial = **mensagem sem destinatário, remetente acha que enviou**. Idem `criarConferenciaBase` (onda_despacho.go:66-135), `hGrupoDestituirChefe` (onda_0510_escalas.go:316).

---

## 5. Banco de dados — auditoria detalhada

### 5.1 O que está certo (e é acima da média)

- **SQLite bem configurado** (store.go:40-54): WAL + `busy_timeout(5000)` + `foreign_keys(1)` + `synchronous(FULL)` + `_txlock=immediate`, `SetMaxOpenConns(1)` — escritor serializado, zero SQLITE_BUSY, e o pragma de FK vale para a única conexão.
- **FKs declaradas e enforcement real**; índices parciais únicos cravando regras de negócio (1 gerente/grupo em usuario_papeis; 1 conferência de material aberta por grupo — v41; titular único em funcao_membros).
- **Backup é a parte mais madura do projeto**: `VACUUM INTO` consistente → checkpoint → sha256 streaming → sidecar + MANIFEST; dispara no boot, em fechamentos, manual e via CLI `sci backup` para cron; colisão de nome serializada por mutex (reproduzida com -race); import valida magic + integrity_check + versão de schema.
- `uq_pessoas_identidade` (índice único de expressão) para dedup de efetivo por grupo.

### 5.2 Inventário

~48 tabelas + views, 69 índices, schema v43. DDL distribuído em 3 lugares — sendo um deles **fora do versionamento**: `conferencia_despachos` e `pessoas_apresentacao` são criadas lazy por `sync.Once` no primeiro handler (onda_despacho_schema.go, onda_presenca_banco_schema.go) — não entram em `schema_migrations` e, se o CREATE falha, o `once.Do` nunca repete (degradação permanente até restart).

### 5.3 Lacunas

| Item | Detalhe |
|---|---|
| **Índices faltando em caminhos quentes** | `auditoria(entidade, registro_id)` — `hPessoasList` roda `GROUP BY` sobre a auditoria inteira a cada listagem (server_pessoal.go:599-608); só existe `idx_auditoria_em`. `conferencias(grupo_id, data)` — varreduras por data em listas/marcação/estado. `mensagem_destinatarios(mensagem_id)` — full-scan no CASCADE. |
| **ON DELETE inconsistente** | CASCADE nos filhos "possuídos", NO ACTION nas FKs de referência — aceitável pela imutabilidade histórica, **exceto** porque o NUKE não cobre todas (P1-1). |
| **Datas 100% TEXT** | Mistura de formatos timestamp (`%fZ` com millis, RFC3339 Go sem millis, `"…:05.000Z"`) — nenhuma comparação atual cruza errada, mas é bomba-relógio para o próximo `>` entre colunas de procedências diferentes. |
| **Blobs base64 no banco** | `material_cautela_anexos.dados_base64`, `material_item_anexos`, `usuarios.foto_base64` — inflação de 33%, cada backup copia tudo; a doutrina do Drive (bytes em disco) não foi aplicada ao módulo Material. |
| **Auditoria sem retenção** | Append-only na prática (bom), mas cresce para sempre, o INSERT tem erro engolido (`_, _ =`) e o scan sem índice bloqueia a única conexão do pool. |
| **Colunas duplicadas** | `material_itens.nivel_sensibilidade` (v19) e `sensibilidade` (v29) coexistem; a primeira defasa após backfill. |
| **Backup sem rotação** | Crescimento ilimitado no mesmo disco do banco; off-box só manual. |

### 5.4 Avaliação do `SetMaxOpenConns(1)`

Correto para o objetivo (volume ínfimo, escritor serializado, `_txlock=immediate` elimina deadlock de upgrade). Custo: **leituras também serializam** — uma query lenta congela a API inteira. Os pontos que seguram a conexão: o GROUP BY de auditoria do `hPessoasList`, o export SQLite e o `VACUUM INTO`. Com os índices acima + contexto nas queries, o risco fica contido. A varredura por queries aninhadas (o deadlock histórico do `hUsuariosList`) **não encontrou novas ocorrências** no pool principal — a lição foi internalizada (padrão `executorSQL`, comentários de doutrina em 5+ pontos).

---

## 6. Backend — arquitetura

**Forte:** roteamento moderno (ServeMux Go 1.22, `{ Método /rota/{id} }`, ~200 endpoints), recuperação de panic + CSP global (main.go:108-127), SSE correto (hub RWMutex, buffer com drop-on-full, heartbeat, cleanup por contexto — server_realtime.go), watchdog SLA com webhook, shutdown gracioso, `sci backup` CLI.

**Dívida estrutural:**

1. **Autorização pulverizada:** 6 middlewares de guarda (`auth`, `authPapeis`, `authConf`, `authMaterial`, `reservaAuth`, `guardaGestaoPessoal`) + **103 prólogos inline** de `escopoDoUsuario` — cada revisão de doutrina de papel (4+ em 2 semanas) exige achar todas as cópias; uma esquecida = escalada (já aconteceu: "um OPERADOR do grupo excluía pessoa", server_pessoal.go:1566). Mitigação pragmática: função `exige(perm)` central + tabela de política por rota.
2. **Sem fronteira de camadas:** HTTP → autorização → regra → SQL no mesmo corpo (server_material.go tem 54 chamadas diretas `a.st.db.*`; server_pessoal 92). `store.go` (2948 linhas) é na verdade o arquivo de migrações (~70% DDL).
3. **Erros internos vazando:** 163× `jsonErro(w, 500, err.Error())` manda texto do SQLite para o cliente.
4. **`context.Context` não propagado** (zero `QueryContext`): sem timeout de query — com pool=1, uma query lenta congela tudo.
5. **Arquivos "onda_"** (14+): features partidas entre arquivo de módulo e arquivo de onda (o header de onda_0510_c2.go admite: "a parte 1 vive em mensagens.go"). Recomendação: fold mecânico por domínio mantendo o nome da onda no comentário — a equipe já faz "recortes puros" com disciplina.
6. **Duplicação de API:** duas gerações vivas (`/api/calendarios*` e `/api/calendario/*`; `relatorio-dia.pdf` e `relatorio-dia/pdf` no mesmo handler; `/api/conferencia/lista` e `/api/conferencias`); POST como verbo de ação (`/ler`, `/excluir`, `/fechar`).
7. **go.mod:** todas as dependências marcadas `//indirect` (tidy nunca completado); `google/uuid` requerido mas não importado.

---

## 7. Frontend

**Forte:** carregamento lazy por rota (lazy.js), watchdog de conectividade, polling de notificações, teardown de timers via MutationObserver (raro e bem feito), `esc()` usado ~584×, `api()` consistente (250 call sites).

**Dívida:**

1. **Duplicação pior que o CHANGELOG admite:** `ui_helpers.js` (1307 linhas) é **código morto** — nunca carregado (a fatoração foi abortada a meio); 4 implementações de ordenar/paginar tabela; `formatarTamanho` ×3; **`esc()` ×3 com divergência** (views_avisos.js:12 não escapa apóstrofo e suas datas ignoram o fuso de Brasília que o core aplica).
2. **Cache-busting inócuo:** o servidor manda `Cache-Control: no-store, must-revalidate` para .js/.css (server.go:303-311) — o `?v=370` nunca tem efeito e cada navegação re-baixa tudo. Alternativa zero-build: hash de conteúdo no boot do Go + `immutable`, eliminando o bump manual em 5 pontos (que já foi esquecido em ondas passadas — commit 78b30c0).
3. **Vazamento lento de listeners** em `criarDropdown` (document.addEventListener por instância, sem remoção — core.js:291-293).
4. **Módulos dormente custando binário:** views_escalas/calendario/consciencia (~2.655 linhas) embutidas e inalcançáveis — mover para `web/acervo/` fora do embed.
5. Acessibilidade: modais sem focus trap; `aria-*` quase só no index; contraste `--tx3` 11-12px abaixo de 4.5:1; alvos de toque inline < 44px em botões de tabela.

---

## 8. Segurança — pontos fortes a preservar

Argon2id com parâmetros OWASP e formato PHC (auth.go:76-107) · token de sessão de 256 bits com **SHA-256 no banco** (roubo do DB não expõe sessões) · rate limit de login duplo (login+IP / IP) com auditoria e penalidade · cookie HttpOnly + SameSite=Strict · CSRF por header custom `X-SCI` + checagem de origem · **SQL 100% parametrizado** (varredura completa, incl. ORDER BY e LIKE) · sanitizador server-side de rich text com allowlist · CSP + nosniff em toda resposta · uploads de material com allowlist + attachment · ACL do Drive bem construída · auditoria em todas as mutações sensíveis · nada de dado/binário versionado no git.

Menções: sessões não são invalidadas na troca de senha; HTTP sem TLS e cookie sem `Secure` (LAN); `admin/admin` de fábrica sem expiração forçada (`precisa_setup=0`); `GET /api/configuracoes` público inclui `WEBHOOK_ATRASOS_URL`; `dados.tar.gz` (com o banco REAL: PII + hashes Argon2) está na árvore de trabalho — remover da máquina.

---

## 9. Processo e documentação

- **ROADMAP.md desatualizado** (10/06): os itens P1-P6 da v1.5.1 foram entregues (CHANGELOG v1.5.2/v1.5.3) mas seguem `- [ ]`. README diz schema v41; binário = 43. Três fontes de verdade (ROADMAP/HANDOFF/CHANGELOG) sobrepõem-se.
- **CI/ops maduros para o porte** (watchdog com kill por PID exato, cron de backup com retenção 15d, harness de carga com p50/p95) — mas 100% acoplados a paths de uma máquina (`/opt/data/...`, `/home/aimi/...`) e o driver CDP do e2e não está no repo (irreproduzível).
- Cultura documental rara: LIMPEZA.md com prova de mortidade de código, auditorias com veredito por rota, lições de deadlock documentadas in loco.

---

## 10. Roadmap recomendado

### v1.5.4 — Hardening imediato (impacto alto, esforço baixo; nada muda a arquitetura)
1. **Fechar a classe escopo -1** (P0-1): guarda central + varredura + teste de matriz rota×papel com conta sem grupo.
2. **Blindar foto_base64** (P0-2): validação server-side + `esc()` nos 4 sinks.
3. **Consertar import de backup** (P0-3): rename do swap, cadeia completa de migrações (registry `[]func() error`), rollback correto, teste de persistência pós-restart.
4. **Completar o NUKE** (P1-1) + teste com grupo "rico" (aviso + chefe + função + conferência de material + modelo de escala).
5. **Allowlist em hUsuarioPapelDel/Add + escopo do setor em hMudarContexto + escopo no endpoint de foto** (P1-2/P1-3).
6. **Drive: allowlist de MIME p/ inline (ou attachment + nosniff) e Content-Disposition sanitizado** (P1-4).
7. **ci.sh: `go vet` + `go test -count=1 ./...`** (190s) e `-race` no host Linux (P1-5).
8. Remover `dados.tar.gz` da árvore de trabalho.

### v1.5.5 — Consolidação
- Migrações transacionais nos rebuilds; índices `auditoria(entidade,registro_id)` e `conferencias(grupo_id,data)`; política de retenção da auditoria; tx em `hMensagensEnviar`/`criarConferenciaBase`.
- Envelope de erro: parar de vazar `err.Error()` (163 sites) e unificar `http.Error`×`jsonErro` no middleware de auth.
- Sessões: invalidar na troca de senha; forçar troca do admin no primeiro boot; `Secure` condicional.
- Frontend: resolver o destino do ui_helpers.js (carregar de verdade ou apagar); unificar `esc()`; `EditorRico.sanitizaHTML` na renderização (defesa-em-profundidade de graça); cache por hash de conteúdo no lugar de no-store + bump manual.
- `go mod tidy`; decidir destino dos módulos dormentes (`web/acervo/` fora do embed).

### v1.6 — O que já está no ROADMAP (mantido, com ajustes)
- Reintroduzir Consciência/Escalas/Calendário **após** a matriz de autorização cobrir os novos papéis (a lição da onda 10/10).
- Adicionar: observabilidade mínima (access log com request-id), e2e reproduzível no repo (vendor do driver CDP), drill de restauração de backup documentado, procedimento LGPD de exportação/exclusão por titular.
- Extração gradual de pacotes (`internal/store` → `internal/pdf` → `internal/api`) nos "recortes puros" que a equipe já executa — sem urgência, sem rewrite.

---

## 11. Anexo — métricas

| Métrica | Valor |
|---|---|
| Commits | 337 em 13 dias (13 dias de projeto até 10/10) |
| Go não-teste | ~24,4k LOC / 42 arquivos |
| Go testes | ~17,1k LOC / 68 arquivos / **236 funções Test** |
| Frontend | ~22,3k LOC JS/CSS/HTML, sem build |
| Endpoints | ~200 (92 GET, 74 POST, 25 DELETE, 10 PATCH) |
| Schema | v43 · ~48 tabelas · 69 índices |
| Cobertura | 74,6% statements |
| TODO/FIXME reais | 0 |
| Módulos dormentes | ~6,2k LOC (escalas+calendário+consciência, back+front) |
