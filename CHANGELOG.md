# SCI — Changelog Oficial

## [v1.6.0] — Não lançada

### 🎭 Onda v1.6.0-contextos — cadeiras viram CONTEXTO, operador é de setor (F1–F7)
- **F1 — fundação (`5184b4a`):** migração **v45** (`migrarV45`, `onda_v160_contextos.go`) — REBUILD
  de `usuario_papeis` com o CHECK estendido (`+enc_pessoal`/`+enc_material`, ids preservados),
  MATERIALIZAÇÃO das designações de cadeira em linhas de papel (INSERT OR IGNORE pela UNIQUE) e
  RE-KEY das sessões de designado puro; `versaoSchemaBinario` → **45**; dropdown de contexto resolve
  os contextos novos (`rotuloPapel` com rótulo próprio).
- **F2 — poder segue o contexto ativo (`515dd1b`):** `ehEncarregadoDePessoal/Material` passam a ler
  `u.Papel` (linha materializada) — o PODER vem do CONTEXTO ATIVO da sessão, não da designação
  implícita; cargo sem acréscimo de poder; menu/portões do front por contexto (`enc_pessoal` →
  conferência+pessoal+relatórios, `enc_material` → só material).
- **F3 — designação sincroniza com o papel (`66f7fe5`):** `hFuncaoMembrosSet` materializa a linha de
  papel da cadeira e `hFuncaoMembrosDel` desmaterializa quando não resta designação, ambos com
  RE-KEY das sessões presas (fim do "chefe-zumbi de cadeira"); fan-out de notificação de aviso
  inclui os contextos enc.
- **F4 — extinção do operador de grupo (`b2afdee`):** OPERADOR nasce OBRIGATORIAMENTE com setor do
  grupo (`hUsuariosAdd`/`hUsuarioPapelAdd` — herda o do chefe criador ou setor explícito) e a guarda
  central `exigeSetorOperador` bloqueia contas legadas sem setor nos portões `authConfCom`,
  `authMaterial`, `reservaAuth` e nos handlers de dados de relatórios (403 claro, sem
  auto-adivinhação); `usuarios.setor_id` = cadastro do operador e fonte do recorte (D-5).
- **F5 — contas SEM FUNÇÃO (`14e5c75`):** papel `sem_funcao` (fora do CHECK de `usuario_papeis` — é
  ausência de contexto, não papel de linha) com escopo central NEGADO
  (`papelTemEscopoDeDados` em `escopoDoUsuario`/`exigeEscopo`/`podeVerMural`); login aterrissa na
  página de bloqueio dedicada `#/bloqueio` (`ViewBloqueioSemFuncao` — só perfil/senha/sair).
- **F6 — leitura de conferência com guarda (`edad628`):** nova porta de papel `confLeituraAuth` nas
  leituras do módulo (hoje/estado/lista/conferencias/{id}/funcoes-antiguidade/stream) — contexto
  `enc_material` **403 nem leitura**; admin mantém leitura vazia.
- **F7 — material setor-bound + furos de conferência (`765f9c6`, merge-fix `79ad356`):**
  `setorEscopoMaterial` recorta o módulo Material ao PRÓPRIO setor do operador (leituras, escritas,
  PDFs, conferências de material — iniciar força o setor, Carga Geral fica fora; categorias:
  escrita só gerente/enc_material) e os furos de conferência fechados: operador só conclui/reabre/lê
  pré-fechamento do próprio setor de cadastro.
- **Material por NÍVEL de acesso + termos Cautelar/Descautelar (onda v1.6.0-contextos):** doutrina
  de nível no módulo Material — gerente/enc_material gerem o grupo inteiro; **chefe de setor**
  adiciona/cria/atualiza o material do PRÓPRIO setor (leitura recortada; item alheio 404 honesto do
  P1-2) e NÃO exclui nem dá baixa (403 "exclusão e baixa de material é ato do gerente ou
  encarregado de material" — cobre também a baixa patrimonial `?modo=baixar`); **operador APENAS
  cautelar/descautelar** (o 200 que deixava o operador cruando item no próprio setor foi INVERTIDO
  para 403; cautela/descautela próprias provadas em 200); chefe MANTÉM cautela/descautela do
  próprio setor (interpretação registrada nos handlers). Rótulos, mensagens e relatórios impressos
  trocam "Acautelar/Acautelado(s)/Devolver" por "Cautelar/Cautelado(s)/Descautelar" — o ENUM
  gravado `status='acautelado'` (CHECKs store.go) e os campos `data_devolucao`/`obs_devolucao` são
  esquema e NÃO mudam. Front: `#/material` já aberto a chefe/operador (menu+portão do checkpoint)
  e views_material.js esconde os controles fora do nível (operador não vê cadastrar/editar/excluir;
  chefe não vê excluir/baixar; gerente/enc vêem tudo). R-32 ✅ no §14 da ARQUITETURA.

---

## [v1.5.4] — Não lançada

### 🔒 Onda v1.5.4 — D2: rotas de PDF/dados com o mesmo guard do módulo (R-2)
- **Causa:** rotas de dados/PDF registradas com `a.auth(false)` sem guarda de papel/escopo — os
  portões do `rotear()` no front são cosméticos por design e o servidor era a parte furada:
  `/api/escalas/pdf`, `/api/escalas/relatorio-dia.pdf` (+ duplicata `/relatorio-dia/pdf`) e
  `/api/escalas/minhas`; os PDFs de material `inventario/pdf`, `cautelas/{id}/recibo.pdf` e
  `conferencias/{id}/pronto.pdf`; e `/api/conferencia/{id}/relatorio.pdf` (sem guarda de papel).
- **Correção:** escalas exigem `reservaAuth` (gerente/operador/chefe_setor; admin 403; `minhas`
  também barra conta sem grupo — a abertura ampla segue PENDENTE p/ M5, decisão D-3); material
  exige `authMaterial` (gerente/operador/enc_material; escopo do objeto segue no handler);
  conferência exige a nova guarda `confPDFAuth` — papéis do módulo (gerente/operador/enc_pessoal/
  auxiliar; admin proibido) e chefe_setor somente quando a conferência envolve setor que AINDA
  comanda (`conferenciaEnvolveSetorComandado`, fonte única `chefe_setores` — D-1). Doutrina nova
  registrada no `ARQUITETURA.md` §3.I: **nenhuma rota de dados sem guard declarado na tabela**.
- **Regressão:** `onda_v154_d2_test.go` (matriz por persona — gerente/operador do próprio grupo
  200; conta sem grupo 403 em todas; admin 403; chefe só com setor comandado; operador de outro
  grupo 403 nos objetos alheios e PDF do próprio grupo sem vazamento, provado pelo texto do PDF).

### 🪜 Onda v1.5.4 — D3: antiguidade com ordenação unificada nas 3 fontes da tag (R-7)
- **D3 (R-7):** antiguidade com ordenação unificada nas 3 fontes da tag (pessoa → conta → papel, fonte SQL única `exprAntiguidadeTresFontes` em `onda_0910_conf_antiguidade.go`) na listagem do `/hoje`, pré-fechamento, relatório em tela e PDF de conferência; `iniciar`/`despachar` respondem `sem_tag:[nomes]` (quem ficou fora do filtro por não ter a tag em nenhuma fonte, com recorte aos setores despachados); pré-fechamento voltou a listar os não-marcados (`nao_verificado` — linha NULL era descartada no Scan); sem herança de catálogo entre grupos segue by design (decisão D-1 pendente). Regressão: `onda_v154_d3_test.go`; front do `sem_tag` pendente.

### 🛡️ Onda v1.5.4 — E1: guardas de autenticação (R-9/R-10/R-14/R-16)
- **R-9 — contexto de sessão (`POST /api/sessao/contexto`):** o setor passado precisa pertencer ao
  GRUPO do papel ativo; chefe_setor só assume setor que AINDA comanda (`chefe_setores` — fonte única
  desde a D1); admin segue global. O `UPDATE usuarios SET setor_id` global foi REMOVIDO: o contexto
  vive em `sessoes.setor_ativo_id` (`UsuarioDaSessao` o sobrepõe); `usuarios.setor_id` vira cadastro
  (proposta D-5, decisão pendente). Regressão: `onda_v154_e1_test.go` (outro grupo 403, setor não
  comandado 403, comandado 200 sem reescrita do cadastro, admin livre).
- **R-10 — gestão de papéis (`POST|DELETE /api/usuarios/{id}/papeis[/{pid}]`):** fim do fail-open do
  Del — allowlist de solicitantes fail-closed (`podeGerirPapelAlvo`): admin global; gerente
  operador/chefe_setor no próprio grupo/árvore; enc/aux de pessoal (o cargo manda, v367) no próprio
  grupo; alvo resolvido ANTES de qualquer decisão, e o re-chaveio de sessões do alvo só ocorre com
  remoção legítima. Trava "não remover o único papel" mantida.
- **R-14 — hierarquia de envio (`POST /api/mensagens`):** ninguém envia para fora do escopo do papel
  ativo (próprio grupo + subordinados ativos — `gruposSubordinadosAtivos`); conta sem grupo não envia
  a ninguém (403); admin segue global; caixa admin continua destino alcançável e a resposta a
  despacho (`pai_id`) alcança o remetente da mensagem pai. Matriz de envio documentada no
  ARQUITETURA §8.
- **R-16 — senha/sessões:** troca de senha (própria ou por gerente/admin) invalida as sessões da
  conta — na troca pela própria conta a sessão CORRENTE é preservada (quem trocou não cai; decisão
  no ARQUITETURA §3.C); redefinição por outrem derruba todas. Admin semeado no 1º boot com a senha
  PADRÃO `admin` nasce com troca obrigatória (`precisa_setup=1` — gate central do middleware auth,
  doutrina reusada); deploy com `SCI_ADMIN_SENHA` própria não nasce bloqueado.
- Regressão completa em `onda_v154_e1_test.go` (7 testes, persona positivo/negativo); testes de
  mensageria anteriores às guardas atualizados à nova doutrina (vínculos de árvore explícitos).

### 🛡️ Onda v1.5.4 — E2: escopo e conteúdo (R-5/R-6/R-8/R-11/R-15/R-21)
- **Escopo e conteúdo fechados:** foto de usuário com escopo (própria/avatar 200, admin global, grupo vê grupo, 403 fora — R-5); drive só serve `inline` na allowlist de MIME dos anexos de material (PDF/PNG/JPEG/WEBP), resto `attachment`+octet-stream+`nosniff` SEMPRE e filename sanitizado no SERVE (R-11); NUKE de grupo, exclusão de setor e de conferência arquivada completam os DELETEs de FKs NO ACTION em transação (conferencia_escalas, mural com origens de repost, escala_modelos+postos+aptos, material_conferencias+itens, chefe_setores, funcao_membros, grupo_setor_responsaveis, setor_sugestoes, usuario_papeis, funcao_id/setor_id de cadastro) — grupo/setor "rico" sem erro e sem órfãos, e a exclusão de setor revoga comando+papel (doutrina D1) remanejando o material (R-6/R-15); detalhes/ciente de aviso e compartilhamentos de calendário só no escopo do objeto (R-21, resíduo: comentários de aviso); arquivar/descartar conferência viram ato de gerente ou encarregado de pessoal (R-8). Regressão: `onda_v154_e2_test.go` (8 testes persona, positivo e negativo).

### 🧟 Onda v1.5.4 — D1: mata o "chefe-zumbi" (R-12) — chefe_setores vira fonte única
- **Causa:** A nomeado chefe do setor S conservava poderes invisíveis depois de substituído por B
  (ou destituído): o COMANDO (linha em `chefe_setores`) saía, mas o CONTEXTO (`usuarios.setor_id`,
  jamais limpo na troca/destituição) continuava autorizando via fallbacks do `chefeComandaSetor`
  (`usuarios.setor_id` → `pessoas.setor_id`) — "2 chefes de setor simultâneos". Fábrica adicional:
  `hUsuarioPapelAdd` gravava o papel chefe_setor + `usuarios.setor_id` SEM linha em
  `chefe_setores`; `hSetoresAgregado` lia fonte divergente da UI de catálogo.
- **Correção (decisão D-2):** `chefe_setores` é a FONTE ÚNICA do comando — fallbacks removidos;
  novo `setorAtivoComandado` condiciona o contexto da sessão ao comando VIGENTE (concluir setor,
  reabrir setor, marcar presença e leitura do efetivo no `/hoje`: contexto órfão não lê nem lança);
  `hUsuarioPapelAdd` (chefe_setor + setor_id) materializa o comando (UPSERT 1:1 — chefe anterior
  perde a linha) e `hUsuarioPapelDel` do papel revoga os comandos do grupo; `hSetoresAgregado` lê
  `chefe_setores`. Regressão em `onda_v154_d1_test.go` (substituição 1:1, add/del de papel,
  contexto órfão — positivo e negativo por persona).
- **Migração v44 (schema 43 → 44):** materializa os comandos legados ainda pendentes (fontes
  `usuarios.setor_id` → `pessoas.setor_id`; INSERT OR IGNORE — a UNIQUE(setor_id) preserva o
  comando vigente; idempotente). `versaoSchemaBinario` subiu junto (44) — sem isso o próprio
  backup do binário seria rejeitado no import. Resíduo documentado (proposta D-5):
  `usuarios.setor_id` deixa de ser fonte de autorização e vira contexto/cadastro exibido;
  designação de escala do chefe e pre_fechamento ainda leem o contexto (sem poder de conferência).

### 🚪 Onda v1.5.4 — Gate de saída: cache-bust v371, auditoria de escopo, higiene
- **Cache-bust v371** (4 refs `index.html` + `CACHEBUST` do `lazy.js` — `web/core.js`, `web/ui_helpers.js` e `web/views_gestao.js` mudaram no `fd96c90`); **auditoria de escopos pós-onda criada** (`docs/auditoria-escopos-v154.md` — 8 módulos × classes provadas; divergências mapa×código R-1/R-3/R-4 e schema v44 corrigidas no `ARQUITETURA.md`); higiene: `.claude/` no `.gitignore`.

---

## [v1.5.3] — 2026-10-10

### 🔐 Onda 10/10 — Frente A (Gap 1): designação pelo encarregado de pessoal
- **Backend (`/api/grupo/funcoes/membros`):** o encarregado de pessoal agora designa/remove membros **apenas na cadeira `enc_material`** do próprio grupo (titular/auxiliar); a própria cadeira `enc_pessoal` e demais funções continuam restritas ao gerente/admin (403). Operador sem a função segue bloqueado (regressão coberta em `onda_1010_gap1_test.go`, 6 casos).
- **GET devolve `chave`:** o enunciado de funções agora inclui o campo `chave` (revisão CEO — a trava visual do front dependia do nome da função, frágil com white-label).
- **Frontend:** portão do módulo Conferência (`#/hoje`, `#/conferencia`) liberado para gerente, operador, chefe de setor e encarregado de pessoal; encarregado de material puro e conta sem função são barrados (menu sem CONFERÊNCIA para `enc_material`). A aba FUNÇÕES do Pessoal espelha a trava do servidor: controles desabilitados fora da cadeira `enc_material` para não-gerentes.
- **Cache-bust v370** (4 refs `index.html` + `CACHEBUST`).

---

## [v1.5.2] — 2026-10-10

### 🔀 Onda de consolidação — merge total das frentes em `main`
- **Merge total:** as 14 branches do repositório (Frentes F1/F2/F3, fixes e integrações) estão contidas em `main`; operação registrada com prova em `docs/historico/onda_consolidacao_1010.md`. Schema binário permanece **43** (nenhuma migração nesta onda).
- **Pessoal — UX das abas EFETIVO/CHEFIAS (`fix/pessoal-front-ux`):** ordenação por cabeçalho + paginação (20/página) nas tabelas de Efetivo e Chefias, além de auditoria UX documentada (`docs/auditoria-ux-pessoal.md`). Revisão CEO pré-merge manteve o contrato "autocontido" de `views_pessoal.js` — helpers locais `pesTabelaControles/pesOrdenar` no lugar dos globais `window.tabelaControles`, garantindo ordenação/paginação também navegando **direto** a `#/pessoal` (os globais só existiam com `views_gestao.js` previamente carregada).
- **Cache-bust v369:** 4 refs em `web/index.html` + `CACHEBUST` em `web/lazy.js` (doutrina: bump único por onda, nunca rebaixar o número).

---

## [v1.5.0] — 2026-10-02

### 🚀 Novidades & Módulos Principais
- **Módulo de Escalas 2.0 (`/api/escalas/*`, `#/escalas`):**
  - **Templates de Escala (`escala_modelos`, `escala_modelo_postos`, `escala_modelo_aptos`):** Criação e parametrização de modelos repetitivos de serviço contendo postos e lista de militares aptos via checkboxes.
  - **Aplicação e Limpeza Diária:** Botões "Aplicar Escala" para clonar o template para a data selecionada e "Limpar Escala do Dia" com modal de confirmação.
  - **Ciclo de Vida em 4 Fases:** `aberto` -> `preenchido` -> `aprovado` -> `publicado`.
  - **Delegação Inter-Grupos:** Possibilidade de alocar membro do próprio grupo ou delegar o preenchimento do posto para subunidade subordinada com atualização em tempo real.
  - **Relatório Diário Oficial em PDF (`/api/escalas/relatorio-dia.pdf`):** Documento formal com cabeçalho, fase, postos, militares alocados e campo de assinatura.
  - **Aba "Minhas Escalas" (`/api/escalas/minhas`):** Visão dedicada do militar com escalas em que está apto, próximos serviços e histórico.

- **Consciência Situacional do Grupo (`/api/consciencia/resumo`, `#/consciencia`):**
  - Painel consolidado do Comando monitorando subunidades subordinadas (efetivo pronto, conferências abertas/fechadas, cautelas e escalas).
  - Consulta rápida instantânea de militares no banco de pessoal com abertura direta de ficha e PDF.
  - Fila de apreciação e sugestões setoriais integradas.

- **Workflow Setorial de Sugestão & Aprovação (`/api/setores/sugestoes/*`):**
  - Estruturação dos três setores operacionais: **Comando**, **Pessoal** e **Material**.
  - Auxiliares operam sob workflow de sugestão; ao ser aprovada pelo Chefe de Setor/Gerente, a ação é executada e o resultado oficial é registrado em nome do Chefe.

- **Etiquetas de Material em Lote (`/api/material/etiquetas-lote.pdf`):**
  - Impressão otimizada de 10 etiquetas por folha A4 (grid 2x5) com QR Code individual, código de patrimônio e dados da subunidade.

- **Módulo de Conferências & Calendário (Ajustes e Correções):**
  - **Inspeção de Arquivo:** Visualização detalhada e filtros de registros de conferências fechadas e arquivadas.
  - **Regra de Observação e Destino:** Reset automático de observação de carry-over caso a situação do militar se altere; destino zerado para presente, atraso e falta (preservado apenas para falta justificada).
  - **Badges Visuais:** Sinalização de militar escalado hoje (`📅🔴`) e pós-escala (`📅🟡`).
  - **PDF de Conferência:** Abreviação do nome do setor para evitar truncamentos e bloco de assinatura física centralizado.
  - **Anti-Duplicidade de Calendário:** Índice único e atualização idempotente de permissões ao compartilhar calendário.

### 🎨 Design System & Frontend
- **Dropdown Estilizado Obsidian Glassmorphism (`window.criarDropdown`, `.sci-dropdown`):** Menu suspenso com fundo translúcido escuro, blur acrílico, chevron dinâmico e suporte a fechamento por clique externo/Escape.
- **Viewport Mobile & Tablet Edge-to-Edge:** Barra `#mobileBar` e viewport tocando 100% das bordas da tela sem folgas ou margens flutuantes. Rolagem horizontal refinada nas abas (`.abas`) para evitar empilhamento em telas compactas.

---

## [v1.0.0] — 2026-09-29

### 🚀 Novidades & Módulos Principais
- **Módulo de Escalas & Serviços Integrados (`/api/escalas/*`, `#/escalas`):**
  - Cadastro de Tipos de Serviço / Postos configuráveis (Oficial de Dia, Adjunto, Guarda, Sentinela, etc.).
  - Grade mensal e diária de turnos com seleção múltipla de militares e funções de posto.
  - **Motor Inteligente de Integração com Conferência:** Militares escalados no dia são automaticamente pré-preenchidos como *Justificada (Serviço de Escala)* ao abrir nova conferência.
  - Painel de visualização de efetivo de serviço ativo hoje.

- **Módulo de Material, Reserva & Cautelas com Anexos (`/api/material/*`, `#/material`):**
  - Inventário completo de bens, armamento, viaturas, chaves, rádios e equipamentos com controle de patrimônio e número de série.
  - **Modal Expresso de Cautela ("⚡ Iniciar Nova Cautela")** acessível diretamente do topo e das tabelas.
  - **Sistema Ilimitado de Anexos & Digitalizações:** Permite anexar múltiplos PDFs, fotos e fichas assinadas escaneadas no ato da cautela ou a posteriori.
  - Gerenciador de documentos anexos com download inline (`/api/material/anexos/{id}`) e visualização.
  - Operações completas de devolução e baixa patrimonial.
  - **Exclusão Definitiva Atômica:** Opção de excluir o bem e limpar seu histórico e anexos com integridade referencial em transação atômica.
  - Filtro inteligente de status no inventário (Ativos, Disponíveis, Acautelados, Manutenção, Baixados).

- **Módulo de Configurações Globais & White-Label (`/api/configuracoes`, `#/configuracoes`):**
  - Painel administrativo para customização de identidade visual, nome da instituição/organização, rótulos de pessoal/gerente e cores.
  - Armazenamento em chave-valor no SQLite (`configuracoes`).

- **Explorador Hierárquico Interativo de Grupos & Subgrupos (`#/admin`):**
  - Visualização recursiva em árvore em qualquer nível de profundidade (Comando › Brigada › Batalhão › Companhia › Pelotão › etc.).
  - **Modo de Foco com Breadcrumbs:** Navegação focada no grupo clicado com trilha de ancestrais interativa.
  - **Cálculo Recursivo de Efetivo:** Total de pessoal somando todos os subgrupos dependentes (`Efetivo Total: X | Próprio: Y`).
  - Ações diretas por unidade: Criar Subgrupo Subordinado, Trocar Gerente, Mudar Subordinação, Auditar Contas e Excluir.

### 🎨 Design System & Identidade Visual
- **Tema Obsidian Slate Dark:** Gradientes modernos escuros profundos (`#070a0f` → `#0d131f`) com cartões em glassmorphism fosco (`backdrop-filter: blur(12px)`).
- **Tipografia & Contraste:** Textos com alto contraste, chips semitransparentes com bordas refinadas.
- **Responsividade Mobile:** Drawer menu lateral com overlay, alvos de toque mínimos de 44px e tabelas fluidas.

### 🛡️ Segurança & Arquitetura
- **Guarda de Acesso por Escopo:**
  - Os módulos de **Escalas** e **Material** são restritos aos papéis de grupo (`gerente` e `operador`). O papel **Admin** (sistêmico) é bloqueado com `403 Forbidden` e redirecionado para a gestão global (`#/admin`).
- **Persistência & Migrações:**
  - Atualização para Schema v16 (tabelas `escala_tipos`, `escala_turnos`, `escala_pessoas`, `material_categorias`, `material_itens`, `material_cautelas`, `material_cautela_anexos`, `configuracoes`).
  - Zero dependência externa de runtime (Go + SQLite puro compilado em binário único autocontido).

### 🧪 Testes Automatizados
- Cobertura completa de migrações, sementes padrão, integração inteligente de escalas na conferência, ciclo de vida de materiais/anexos, baixas, exclusão definitiva e white-label (`v1_test.go`).
