# SCI — MAPA DE ARQUITETURA (fonte de verdade operacional)

> **Gerado em 2026-10-10** a partir de auditoria completa do código (`main` @ `608e5ef`, schema v43).
> **Atualizado no gate de saída da v1.5.4** (schema v44, branch `onda/v1.5.4-estancar`): auditoria de
> escopos re-executada e divergências mapa×código resolvidas — ver `auditoria-escopos-v154.md`.
> **Atualizado no gate de saída da v1.6.0-contextos** (schema v45, branch `onda/v1.6.0-contextos`):
> doutrina dos CONTEXTOS — cadeiras materializam linha de papel, poder segue o contexto ativo,
> operador é de setor e 'sem_funcao' não tem escopo (§3.A/§3.B/§4).
> Este documento é o **mapa a consultar ANTES e DEPOIS de qualquer alteração**: todo endpoint do
> sistema está listado com sua guarda, escopo, tabelas e efeitos colaterais, e cada módulo tem sua
> lista de "se você alterar aqui, verifique também".
>
> **REGRA DE MANUTENÇÃO (obrigatória):** nenhuma onda/commit altera rota, guarda, tabela ou doutrina
> sem atualizar a linha/seção correspondente deste documento **no mesmo commit**. Um mapa desatualizado
> é pior que nenhum mapa — ele mente com autoridade. Se este documento divergir do código em algo que
> você está vendo, trate a divergência como bug (ou do código ou do mapa) e resolva na mesma onda.

---

## Índice

1. [Protocolo de alteração (como usar este mapa)](#1-protocolo-de-alteração)
2. [Convenções de leitura](#2-convenções)
3. [Doutrinas transversais](#3-doutrinas-transversais)
4. [Modelo de autorização — matriz papel × módulo](#4-matriz-de-autorização)
5. Mapas por módulo: [Conferência](#5-conferência) · [Pessoal/Usuários](#6-pessoalusuários) · [Material](#7-material) · [Mensagens/Drive/Mural](#8-mensagens--drive--mural) · [Grupos/Admin/Catálogos/Relatórios](#9-grupos--admin--catálogos--relatórios) · [Escalas/Calendário (dormentes)](#10-escalas--calendário-dormentes)
6. [Mapa do frontend](#11-mapa-do-frontend)
7. [Mapa de dados (tabelas por módulo)](#12-mapa-de-dados)
8. [Checklists de impacto global](#13-checklists-de-impacto-global)
9. [Registro de defeitos conhecidos](#14-registro-de-defeitos-conhecidos)

---

## 1. Protocolo de alteração

**Antes de escrever código:**
1. Localize a rota/tabela no mapa deste documento. Leia a coluna **Guarda**, **Escopo** e **Notas** da
   rota, e a seção **"Se você alterar este módulo, verifique também"** do módulo dela.
2. Se a alteração toca papel/permissão → leia a [§4 matriz](#4-matriz-de-autorização) e a
   [§3.A doutrina dos 3 sistemas de designação](#3a-os-três-sististas-de-designação). Toda mudança de
   doutrina de papel tem **lado servidor E lado cliente** (menu `montarShell`, portões do `rotear()`,
   `rotaInicial()` — três tabelas no `web/core.js`) — os três mudam juntos.
3. Se a alteração toca SQL/schema → leia [§12 mapa de dados](#12-mapa-de-dados) e a regra do
   pool de 1 conexão ([§3.E](#3e-banco-e-concorrência)); nova coluna em tabela lida por PDFs do
   `relatorio.go` exige revisão dos geradores correspondentes.
4. Se a alteração adiciona/remove rota → o registro `m.Handle(...)` precisa de guarda declarada
   (nenhuma rota de dados com `a.auth(false)` "pelado" — doutrina §3.I; classe do R-2 fechada na
   v1.5.4-D2).

**Depois de escrever código:**
5. Atualize a linha da rota e as seções de impacto deste mapa.
6. Teste de regressão por persona (padrão do repo: `setupTestApp` + `loginAs` + `doJSONReq`).
7. `go vet` + `go test -count=1 ./...` + `ci.sh`; CHANGELOG; cache-bust se `web/` mudou.

---

## 2. Convenções

- **Guardas** (auth.go e derivadas):
  - `auth(false)` — qualquer sessão válida (escrita exige `X-SCI:1` + Origin própria — CSRF leve).
  - `auth(true)` — somente admin.
  - `confAuth` = papéis {gerente, operador} + CONTEXTO `enc_pessoal`/auxiliar ATIVO (u.Papel);
    admin **403**; **v1.6.0-F4**: operador sem setor é barrado antes (`exigeSetorOperador`).
  - `confMarcarAuth` = confAuth + chefe_setor (mesma régua do exigeSetorOperador).
  - `confLeituraAuth` (**v1.6.0-F6**, server_conferencia.go:1749) — porta de PAPEL das LEITURAS do
    módulo (hoje/estado/lista/{id}/funcoes-antiguidade/stream): `papelConfAutorizado` (gerente/
    operador/chefe_setor + contexto enc_pessoal) ou admin (leitura vazia); **enc_material 403 — nem
    leitura**. O recorte de escopo (grupo/setor) continua dentro de cada handler.
  - `confPDFAuth` (server_conferencia.go, v1.5.4-D2) = papéis da conferência {gerente, operador,
    contexto enc_pessoal/auxiliar}; `chefe_setor` SÓ se a conferência envolve setor que AINDA comanda
    (`conferenciaEnvolveSetorComandado`: `conferencia_setores` × `chefe_setores` — fonte única
    D-1); admin **403**. O recorte de escopo (exigeEscopo + grupo/subordinados) segue no handler.
  - `guardaGestaoPessoal` — admin, gerente, ou CONTEXTO `enc_pessoal`/auxiliar ATIVO (u.Papel;
    v1.6.0-F2 — a cadeira só dá poder no contexto dela).
  - `authMaterial` — gerente, operador, ou CONTEXTO `enc_material` ATIVO (u.Papel); admin **403**;
    423 se `MODO_RESERVA=1`; **v1.6.0-F4**: operador sem setor 403 (`exigeSetorOperador`);
    **v1.6.0-F7**: handler a handler o recorte `setorEscopoMaterial` aplica o próprio setor ao
    operador (gerente/enc_material ficam com escopo de GRUPO).
  - `reservaAuth` — {gerente, operador, chefe_setor}; admin 403; 423 se `MODO_RESERVA=1`;
    **v1.6.0-F4**: operador sem setor 403.
- **ExigeSetorOperador** (onda_0510_conf_escopo.go:118, v1.6.0-F4): guarda central do "operador é de
  SETOR" — contexto ativo `operador` sem setor resolvido (`u.SetorID` → `pessoas.setor_id`) toma 403
  com a mensagem única "conta sem setor atribuído…". Chamada nos portões de módulo (authConfCom,
  authMaterial, reservaAuth) e nos handlers de relatórios. Ficam FORA, por doutrina: leituras de
  conferência (o corte vazio já protege) e mensagens/mural/drive (comunicação por papel).
- **Escopo** (`escopoDoUsuario`, server_grupos.go:244): **0** = admin (vê tudo) · **N** = grupo da
  sessão · **-1** = conta sem grupo OU papel sem escopo (**não vê nada** — guarda central
  `exigeEscopo` (helpers.go:149) devolve 403 e `filtroGrupoSQL` (helpers.go:167) injeta
  ` AND 1 = 0` no fio do SQL; R-1 corrigido na v1.5.4-A — ver `auditoria-escopos-v154.md`).
  **v1.6.0-F5:** `papelTemEscopoDeDados` (helpers.go:145) nega o `-1` ANTES do grupo — papel ativo
  fora de {admin, gerente, operador, chefe_setor, enc_pessoal, enc_material} ('sem_funcao', vazio,
  desconhecido) NÃO tem escopo de dados nem com grupo. O escopo vem do **papel ativo da sessão**
  (`sessoes.papel_ativo_id` → `usuario_papeis`), não do papel base.
- Nas tabelas: `W` = tabelas escritas, `R` = lidas. Linhas de arquivo seguem `arquivo:linha`.

---

## 3. Doutrinas transversais

### 3.A Os três sistemas de designação (fonte da maioria dos bugs de "2 chefes")

O sistema tem **três camadas independentes de "quem é quem"**, cada uma com tabela, handler e tela
próprios. Toda alteração de poderes deve declarar **em qual camada** mexe e espelhar nas outras quando
couber:

| Camada | Tabela | O que concede | Onde é lida |
|---|---|---|---|
| **Papéis de sistema** | `usuario_papeis` (`admin\|gerente\|operador\|chefe_setor\|enc_pessoal\|enc_material` — CHECK v45) | Acesso base a módulos (papel ATIVO da sessão) | `escopoDoUsuario`, todas as guardas, `/api/me`, dropdown de contexto |
| **Cadeiras (funções de grupo)** | `funcao_membros` × `funcoes` tipo='grupo' (chave `enc_pessoal`/`enc_material`, titular/auxiliar) | **DESIGNAÇÃO — não é poder por si**: materializa/desmaterializa a LINHA DE PAPEL (v45 materializou o legado; sync em `hFuncaoMembrosSet/Del` — F3); o poder é do CONTEXTO ATIVO | `ehEncarregadoDePessoal/Material` (= u.Papel, onda_0510_conf_escopo.go:54/:61), `podeGestaoPessoal`, `authMaterial`, `gestorPessoal/Material` (front) |
| **Chefias de setor** | `chefe_setores` (1 linha por setor — `setor_id` UNIQUE, v35) | Comando do setor p/ conferência (concluir/reabrir/marcar) | `chefeComandaSetor` (**FONTE ÚNICA** — D-2 executada na v1.5.4-D1, ver R-12), `setorAtivoComandado` (onda_v154_d1.go), `hCatalogoList`, `hSetoresAgregado` |

**Doutrina dos CONTEXTOS (v1.6.0 — NOVA, lê antes de tocar papel/cadeira):**
- **A cadeira MATERIALIZA linha em `usuario_papeis`** (migração **v45** materializou as designações
  legadas; desde a F3 a designação nova sincroniza no `hFuncaoMembrosSet` e a remoção desmaterializa
  no `hFuncaoMembrosDel`, ambos com RE-KEY de sessões presas na linha — padrão `hUsuarioPapelDel`).
  `funcao_membros` é a designação; a linha em `usuario_papeis` é o ACESSO/CONTEXTO.
- **O PODER vem do CONTEXTO ATIVO da sessão (`u.Papel`), NÃO da designação implícita**: no contexto
  `operador` quem tem a cadeira é OPERADOR (recorte do próprio setor); no contexto `enc_pessoal` é o
  encarregado (régua de gerente em conferência/pessoal); no contexto `enc_material` só o módulo
  Material (e NEM LEITURA de conferência — confLeituraAuth). Cargos (gerente etc.) NÃO dão acréscimo
  implícito de poder ao contexto de outro papel. Quem tem dois contextos troca no dropdown
  (`POST /api/sessao/contexto`).
- **Operador é de SETOR (v1.6.0-F4 — extinção do operador de grupo):** a criação EXIGE setor do grupo
  (`hUsuariosAdd` :1360 e `hUsuarioPapelAdd`, mensagens.go:216) e a guarda central `exigeSetorOperador`
  bloqueia contas legadas sem setor nos módulos operacionais (403 claro, sem auto-adivinhação).
  `usuarios.setor_id` é o CADASTRO do operador e a FONTE do recorte (`setorDoUsuario` → fallback
  `pessoas.setor_id`; no material, `setorEscopoMaterial`).
- **`sem_funcao` (v1.6.0-F5) é AUSÊNCIA de contexto:** conta de login+senha SEM linha em
  `usuario_papeis` (o CHECK da v45 nem aceita o valor) e SEM escopo de dados
  (`papelTemEscopoDeDados` → `-1` → 403 em tudo). Front: login aterrissa em `#/bloqueio`
  (`ViewBloqueioSemFuncao`); sai do bloqueio por SAIR ou quando a designação futura materializar o
  contexto (próximo login).

**Regras vitais:**
- **DECISÃO D-2 (EXECUTADA na v1.5.4-D1, R-12 "chefe-zumbi"):** `chefe_setores` é a FONTE ÚNICA do
  comando — `chefeComandaSetor` (onda_0510_conf_escopo.go:139) consulta SOMENTE a tabela (os
  fallbacks `usuarios.setor_id`/`pessoas.setor_id` foram REMOVIDOS) e o novo `setorAtivoComandado`
  (onda_v154_d1.go) condiciona o CONTEXTO da sessão ao comando vigente: contexto órfão de chefia
  anterior não conclui, não reabre, não marca e não lê o setor (conclusão da leitura em
  `hConferenciaHoje`). Toda escrita de chefia MATERIALIZA o comando: `hGrupoNomearChefe`,
  `hUsuarioPapelAdd` (chefe_setor + setor_id, UPSERT 1:1 por setor — o chefe anterior perde a
  linha) e a migração **v44** (one-shot que materializou os legados pendentes antes da troca;
  `hUsuarioPapelDel` do papel chefe_setor apaga os comandos do grupo). O nomear/destituir purge o
  papel de quem fica SEM comando, como antes.
- **PROPOSTA D-5 (pendente, decisão de comando):** `usuarios.setor_id` deixa de ser fonte de
  autorização — vira contexto/cadastro EXIBIDO. Resíduos que ainda leem o contexto sem conferir o
  comando: designações de escala do chefe (`guardaEscala`/`validaAlvoEscala`,
  onda_0510_escalas.go:46/:77) e `hSetorPreFechamento` (onda_0910:172) — leitura/designação
  dentro do próprio grupo; o comando real (concluir/reabrir/marcar) já está estancado.
- **Designação de cadeira (v1.6.0-F3, onda_0510_c2.go:104/:226):** `hFuncaoMembrosSet` MATERIALIZA a
  linha de papel da cadeira (`INSERT OR IGNORE … papel = f.chave` — titular/auxiliar da MESMA cadeira
  não colidem pela UNIQUE) e `hFuncaoMembrosDel` DESMATERIALIZA quando não resta designação nenhuma na
  cadeira daquele grupo, com RE-KEY das sessões presas na linha apagada (outra linha do usuário, ou
  `papel_ativo_id` NULL se era a única). Linhas `enc_*` ficam FORA do display-sync de `funcao_id`
  (identidade própria no dropdown). Sem isso a linha órfã reconcederia contexto sem cadeira
  ("chefe-zumbi de cadeira"). O fan-out de notificação de aviso inclui os contextos enc_*
  (mensagens.go:1657).
- **`hMudarContexto` (v1.5.4-E1, R-9 CORRIGIDO):** troca papel/setor ativos DA SESSÃO
  (`sessoes.papel_ativo_id`/`setor_ativo_id`). O setor passado precisa pertencer ao GRUPO do papel
  ativo; `chefe_setor` só assume setor que AINDA comanda (`chefe_setores`, `chefeComandaSetorNoGrupo`
  em onda_v154_e1.go — contexto órfão não entra); admin é global. O antigo
  `UPDATE usuarios SET setor_id` foi REMOVIDO (afetava outras sessões do mesmo usuário) — ver D-5.
- Existe ainda uma 4ª tabela de "responsáveis": `grupo_setor_responsaveis` (encarregado de MATERIAL
  por grupo/setor — módulo Material, server_material.go:37/117). Não confundir com chefia de setor.

### 3.B Papel base × papel ativo × funções · doutrina de contexto
`usuarios.papel` é o papel base; o contexto ativo vive em `sessoes.papel_ativo_id` → `usuario_papeis`
(com `setor_ativo_id`). `UsuarioDaSessao` (store.go:1105) sobrescreve Papel/GrupoID/FuncaoID pelo
ativo; NULL cai no `usuarios.*` — **v1.6.0**: NULL é caso de `sem_funcao`/designado puro (a sessão
legada de cadeira desapareceu: a v45 materializou as linhas enc e re-chaveou as sessões). Conta
**sem grupo** OU **papel sem escopo** = escopo `-1` (`papelTemEscopoDeDados`, §2). O front espelha
via `/api/me` (`ME`, `papeis[]` = TODAS as linhas de `usuario_papeis` — o dropdown de contexto exibe
as cadeiras `enc_pessoal`/`enc_material` com rótulo próprio desde a F1; `rotuloPapel` core.js:112) —
o dropdown grava `POST /api/sessao/contexto` (guarda R-9: setor dentro do grupo do papel ativo; chefe
só setor comandado; admin livre). **Contexto default no login:** `CriarSessaoComPapel` (store.go:936)
escolhe a linha de MENOR id — a de papel de sistema nasce antes da linha enc (migração v45/sync),
então quem tem os dois contextos loga no papel de sistema e troca no dropdown.
**PROPOSTA D-5 (v1.6.0 consolidou o lado do OPERADOR):** o CONTEXTO da sessão é
`sessoes.setor_ativo_id` (sobrepõe o cadastro em `UsuarioDaSessao`); `usuarios.setor_id` é
CADASTRO/exibição — não é reescrito pela troca de contexto, MAS é OBRIGATÓRIO na criação de operador
(hUsuariosAdd/hUsuarioPapelAdd) e é a FONTE do recorte de setor (exigeSetorOperador,
setorEscopoMaterial, guardaSetorNaMarcar, concluir/reabrir/pré-fechamento). Continuam lendo-o como
fallback de cadastro: `hMe`/`hLogin` (setor resolvido p/ o front) e os resíduos já anotados no D-5
(designação de escala do chefe, `hSetorPreFechamento`). O COMANDO de chefia segue exclusivamente em
`chefe_setores` (D-2).

### 3.C Sessões, senhas, CSRF
Cookie `sci_sessao` HttpOnly + SameSite=Strict, TTL 12h; token de 256 bits guardado como **SHA-256**
no banco (store.go:1027). Rate-limit de login: 5 falhas/15min por login+IP, 20 por IP (auth.go:141).
Escrita exige header `X-SCI:1` + Origin contendo o Host. **v1.5.4-E1 (R-16):** troca de senha
(própria, `POST /api/senha`) invalida TODAS as sessões da conta EXCETO a corrente — decisão: quem
autenticou e trocou a própria senha não cai do fluxo em uso; redefinição por OUTREM
(`POST /api/usuarios/{id}/senha`, gerente/admin) derruba TODAS (`invalidarSessoesDeSenha`,
onda_v154_e1.go). Admin semeado no 1º boot com a senha PADRÃO `admin` nasce `precisa_setup=1` —
o gate central do middleware auth já recusa toda operação fora de `/api/setup`, `/api/me` e
`/api/logout` até a troca; seed com `SCI_ADMIN_SENHA` própria não nasce bloqueado. **Lacunas
remanescentes:** sem `Secure` no cookie (HTTP puro na LAN).

### 3.D Ciclo de vida de dados de missão
- **Conferência fechada é imutável** (fechar/marcar/PDF/arquivar/descartar todos guardam status);
  excluir arquivada é admin-only e é a única destruição de histórico fora do NUKE.
- **Carry-over**: ao iniciar, copia situação+destino da última conferência fechada ≤1 dia (falta
  justificada preserva destino) — `criarConferenciaBase` (onda_despacho.go:93-107).
- **Auditoria** (`auditoria`) é append-only; toda mutação sensível registra. `_, _ =` no INSERT
  (erro engolido). Sem índice `(entidade, registro_id)` → `hPessoasList` faz GROUP BY full-scan.

### 3.E Banco e concorrência
SQLite (modernc, puro-Go) — DSN com `busy_timeout(5000)`, `foreign_keys(1)`, WAL,
`synchronous(FULL)`, `_txlock=immediate`; **`SetMaxOpenConns(1)`** (store.go:40-54).
**Regras decorrentes (inegociáveis):**
1. Nunca `Query/QueryRow/Exec` no pool com `*sql.Rows` aberto (deadlock) — colete IDs, feche, consulte.
2. Dentro de transação, TODO acesso pela tx (`executorSQL`, server.go:416) — nunca pelo pool.
3. Multi-escrita que precise ser atômica → `Begin/defer Rollback/Commit` (há pontos fora disso — R-13).
4. Migrações: cadeia v2…v45 unificada em `executarMigracoes` (store.go:1243), chamada por
   `AbrirStore` E por `ReabrirComArquivo` (R-4 corrigido na v1.5.4-A/C); registro único `[]func()`
   planejado na v1.5.5.

### 3.F Frontend embutido
`//go:embed web` servido por `hSPA` com `Cache-Control: no-store` (server.go:303-311) — o `?v=NNN`
de cache-bust é bump manual em `web/index.html` + `CACHEBUST` no `lazy.js` (doutrina atual: bump único
por onda, nunca rebaixar). Views carregadas sob demanda pelo `lazy.js` (mapa hash→arquivo). Router
`rotear()` no core.js é ** cosmético** — o servidor é a guarda real.

### 3.G Tempo real
- **SSE** (`/api/conferencia/{id}/stream`): hub em memória, broadcast em `marcar` e `fechar` —
  **sem consumidor no front** (nenhum EventSource). Sincronização real = **polling de 2s** em
  `/api/conferencia/estado` (hash SHA-1 por setor — contrato também usado por `ops/carga_conferencia.py`).
- **Watchdog SLA** de cautelas (30min → webhook) + sino `/api/notificacoes` (poll 60s).

### 3.H Sanitização/XSS
Rich text sanitizado **no servidor** na escrita (`sanitiza.go`, allowlist b/i/u/br/p/div/h2/h3/
ul/ol/li + text-align); o front renderiza cru — a defesa é unilateral. `esc()` existe no core.js, mas
há 3 cópias divergentes (a de views_avisos.js não escapa apóstrofo). CSP global com
`script-src 'unsafe-inline'` (main.go:111-117) — não protege contra injeção em atributo (R-3/R-11).

### 3.I Nenhuma rota de dados sem guarda declarada (v1.5.4-D2, R-2)
Toda rota de dados/PDF tem, na tabela do seu módulo neste mapa, a **guarda declarada** — o registro
`m.Handle(...)` jamais fica com `a.auth(false)` "pelado" sem uma guarda de papel/escopo
(`reservaAuth`, `authMaterial`, `confPDFAuth`, `authPapeis`, …) e/ou prólogo de escopo
(`exigeEscopo` + teste do objeto no escopo) no handler. Os portões do `rotear()` no front são
cosméticos por design: **o servidor é a guarda real** — um PDF que existe deve declarar quem o lê.
Nova rota sem guarda na tabela = regressão da doutrina (fechou a classe do R-2; o resíduo conhecido
está em R-27/§10 calendário, avaliado na reativação do módulo).

---

## 4. Matriz de autorização

Verdade **do servidor** (o front segue isso no menu/rotear, mas é cosmético). "—" = não aplicável.
Colunas de contexto = **papel ATIVO da sessão** (`u.Papel` — §3.A/§3.B); quem tem duas cadeiras
troca no dropdown e a matriz re-avalia. `sem_função` = conta SEM linha em `usuario_papeis`
(v1.6.0-F5; guarda central `papelTemEscopoDeDados` → `-1`).

| Módulo (rotas front) | admin | gerente | operador (de setor) | chefe_setor | enc_pessoal (contexto) | enc_material (contexto) | sem_função |
|---|---|---|---|---|---|---|---|
| **Conferência** `#/hoje` `#/conferencia` | ✗ (leitura vazia) | ✓ total (grupo) | só próprio setor (lança/inicia/marca/conclui/reabre/pré-fecha); leitura cortada; fechar ✗ | setor comandado (`setorAtivoComandado`) | total (régua gerente) | ✗ TUDO (nem leitura — `confLeituraAuth`) | ✗ 403 |
| **Pessoal** `#/pessoal` | ✗ (só via admin p/ catálogo) | ✓ | ✗ | ✗ | ✓ (efetivo/funções/setores/contas, cria operador C/SETOR e sem_funcao) | ✗ | ✗ 403 |
| **Material** `#/material` | ✗ | ✓ grupo (cadastra/edita/exclui/baixa) | ✓ só cautela/descautela do PRÓPRIO setor (save/del 403; sem setor → 403) | ✓ PRÓPRIO setor: adiciona/edita + cautela/descautela; excluir/baixar ✗ (ato do ger/enc) | ✗ | ✓ grupo | ✗ 403 |
| **Grupos/gerenciar** `#/grupos` | ✗ (área admin separada) | ✓ próprio | ✗ | ✗ | ✗ | ✗ | ✗ 403 |
| **Admin/config** `#/admin` `#/configuracoes` | ✓ | ✗ | ✗ | ✗ | ✗ | ✗ | ✗ |
| **Relatórios** `#/relatorios` | ✓ global | ✓ árvore | ✓ (exige setor; recorte grupo — pendência M1) | ✗ (front) | ✓ | ✗ | ✗ 403 |
| **Mensagens/Mural/Drive** `#/mensagens` `#/avisos` `#/drive` | caixa própria | ✓ | ✓ por papel (grupo; comunicação por papel) | ✓ | ✓ | ✓ | ✗ (exceto Drive por grant prévio — exceção registrada) |
| **Escalas/Calendário/Consciência** (dormente) | ✗ 403 | ✓ grupo | ✓ grupo (D-3; exige setor) | ✓ grupo | ✗ | ✗ | ✗ 403 |
| **Perfil/senha/sair** `#/perfil` | ✓ | ✓ | ✓ | ✓ | ✓ | ✓ | ✓ (só isso + `#/bloqueio`) |

**Princípios da matriz (v1.6.0):** (1) **o poder vem do CONTEXTO ATIVO da sessão** — a cadeira
materializa a linha de papel (v45 + sync F3) e o dropdown troca o contexto; cargo/designação sem
linha não concede nada; (2) **operador é de setor**: criação exige setor e `exigeSetorOperador`/
`setorEscopoMaterial` recortam os dados operacionais ao próprio setor — sem setor = 403 claro;
(3) **`sem_funcao` = só perfil + bloqueio** (`#/bloqueio`): sem linha de papel e sem escopo de dados
(papel fora da lista → `-1` → 403/exceção: Drive por grant prévio); (4) admin é papel de
infraestrutura — **bloqueado** de dados operacionais de grupo (exceções: relatórios globais, leitura
vazia de conferência, excluir arquivadas, NUKE); (5) escopo N vê próprio grupo e, onde indicado,
subordinados ativos (vínculo bilateral); contas sem grupo (-1) seguem sob a guarda central
`exigeEscopo` 403 / `filtroGrupoSQL` `AND 1=0` (v1.5.4-A; R-1 corrigido).

---

## 5. CONFERÊNCIA

Arquivos: `server_conferencia.go`, `onda_0910_conf_antiguidade.go`, `onda_0610_conf_estado.go`,
`onda_despacho.go`(+schema), `server_realtime.go`, PDFs em `relatorio.go`.
Tabelas: `conferencias`, `presencas`, `comentarios`, `conferencia_setores` (CS), `conferencia_despachos`
(CD — **tabela lazy** criada por `sync.Once` fora do versionamento), `conferencia_funcoes` (CF),
`conferencia_escalas` (CE).

**Modos:** `setores` (padrão — efetivo por setor) e `antiguidade` (existe linha em CF). No modo
antiguidade a pessoa entra se `p.funcao_id` OU `usuarios.funcao_id` OU `usuario_papeis.funcao_id` ∈ CF
(`predicadoAntiguidade`, onda_0910); tags válidas = `funcoes.grupo_id = grupo` (v43 matou a
seed global); **sem herança de catálogo de antiguidade entre grupos — by design 09/10** (decisão
D-1, herança de grupo superior, segue PENDENTE de comando p/ M1/M2). **Ordenação (v1.5.4-D3, R-7):**
fonte única SQL em `onda_0910_conf_antiguidade.go` — `filtroAntiguidadeTresFontes` (o próprio
predicado) e `exprAntiguidadeTresFontes` = `COALESCE(fu.antiguidade, fu_u.antiguidade,
fu_up.antiguidade, 999)` (precedência do filtro: pessoa → conta → papel; sem tag = 999, por último)
— consumida via `ordemAntiguidadeTresFontes(colNome)` (antiguidade, nome, id — estável) na listagem
do `/hoje` (`pessoasAtivasOpt` com filtro), no pré-fechamento, no relatório em tela `/{id}` e no PDF
(condicionado ao modo: conferência sem CF mantém as ordens legadas). `POST iniciar`/`despachar`
respondem `sem_tag:[nomes]` — ativos do universo da conferência (grupo; recorte = setores
despachados) fora do filtro por não terem a tag em NENHUMA fonte (front ainda não consome).
Pré-fechamento lista os não-marcados como `nao_verificado` (`COALESCE(pr.situacao,'')` — o Scan
descartava a linha NULL; v1.5.4-D3).

**v1.6.0-contextos:** (F6) as LEITURAS ganharam porta de papel — `confLeituraAuth` (:1749) em
hoje/estado/lista+`/api/conferencias`/{id}/funcoes-antiguidade/stream: passa quem passa em
`papelConfAutorizado` (gerente/operador/chefe_setor + CONTEXTO `enc_pessoal`) e o admin (os handlers
já devolvem leitura vazia); **contexto `enc_material` 403 — nem leitura**; `sem_funcao` 403. (F4) os
portões `confAuth`/`confMarcarAuth` (`authConfCom` :134) barram operador sem setor
(`exigeSetorOperador`). (F2) "enc" passou a ser CONTEXTO: `ehEncarregado*` leem `u.Papel` — o
encarregado no contexto operador é operador (setor), e vice-versa no dropdown. (F7) furos do
operador fechados com ramo espelhando o do chefe: concluir/reabrir e pré-fechamento só o setor de
CADASTRO da conta (`setorDoUsuario`; sem setor → 403 com a mensagem única da onda).

| Método | Rota | Handler (arq:linha) | Guarda | Papéis | Escopo 0/-1/N | W | R | Notas |
|---|---|---|---|---|---|---|---|---|
| GET | /api/conferencia/hoje | server_conferencia.go:16 | **confLeituraAuth** (v1.6.0-F6; rota :1779) | ger/op/chefe/enc_pessoal; admin vazio; **enc_material 403**; sem_funcao 403 | admin: vazio; -1: 403 (v1.5.4-A); N: conf aberta | DDL lazy CD | conferencias, presencas, CD, CF, CS, escalas | modo+funcoes_filtro; pessoas na escada de antiguidade unificada (v1.5.4-D3/R-7); corte por setor ativo/despachos — chefe só lê o setor que AINDA comanda (setorAtivoComandado, v1.5.4-D1/R-12; contexto órfão → pessoas vazias); operador: recorte do próprio contexto |
| GET | /api/conferencia/estado | onda_0610_conf_estado.go:23 | **confLeituraAuth** (F6) | idem hoje | admin: hash vazio; -1: 403 (v1.5.4-A); N: por setor | — | idem | SHA-1/setor; base do tick 2s |
| POST | /api/conferencia/iniciar | server_conferencia.go:447 | confMarcarAuth + internas (+**exigeSetorOperador** F4) | ger/op/chefe/enc_pessoal | sem grupo 403 | conferencias, presencas(carry+escalas), CS, CF, CD | pessoas, escalas | valida funcao_ids do grupo ANTES; auditoria; chefe só inicia setor que comanda (chefeComandaSetor — fonte única chefe_setores, D-1) e **operador só o PRÓPRIO setor** (setorDoUsuario, ramo interno); modo antiguidade responde `sem_tag:[nomes]` (v1.5.4-D3/R-7) |
| POST | /api/conferencia/despachar | onda_despacho.go:160 | confAuth (+operador 403 interno; **exigeSetorOperador** F4) | ger/enc_pessoal | sem grupo 403 | +CD | idem | exige ≥1 setor; modo antiguidade responde `sem_tag` com recorte aos setores despachados (v1.5.4-D3/R-7) |
| POST | /api/conferencia/fechar | server_conferencia.go:542 | confAuth + portão interno | **só gerente/CONTEXTO enc_pessoal** (F2; op/chefe 403) | -1: 403 (v1.5.4-A) | presencas upsert, conferencias, CS (tx) | conferencias | sem ✅→nao_verificado; **backupAssíncrono** + broadcast SSE |
| POST | /api/conferencia/marcar | server_conferencia.go:315 | confMarcarAuth + guardaSetorNaMarcar (:187) | ger/enc_pessoal: grupo; chefe: comando vigente; op: setor de cadastro | -1: 409 | presencas, CS | pessoas | só justificada tem destino; troca reset obs; SSE; ramo chefe exige comando VIGENTE (setorAtivoComandado); ramo operador compara o setor do militar com `setorDoUsuario` (F4) |
| GET | /api/conferencia/lista · /api/conferencias | server_conferencia.go:902 (rotas :1785/:1802) | **confLeituraAuth** (F6) | idem hoje | -1: 403 (v1.5.4-A) | — | conferencias | ?arq=1 arquivo; senão 7 dias |
| GET | /api/conferencia/{id} | server_conferencia.go:1028 | **confLeituraAuth** (F6) | idem hoje | 0: qualquer; -1: 403 (v1.5.4-A); N: própria+subordinados | — | tudo | relatório na tela; modo antiguidade ordena pela escada (antiguidade unificada das 3 fontes, v1.5.4-D3/R-7); modo setores mantém situacao/nome |
| DELETE | /api/conferencia/{id} | server_conferencia.go:969 | confAuth + portão R-8 interno | **só gerente/enc_pessoal** (op 403 — R-8 corrigido v1.5.4-E2) | -1: 403 (v1.5.4-A) | comentarios, presencas, CE, conferencias | — | só ABERTA; sem backup |
| POST | /api/conferencia/{id}/setor/{sid}/concluir | server_conferencia.go:656 | confMarcarAuth (+**exigeSetorOperador** F4) | chefe: setor ativo COMANDADO (setorAtivoComandado, v1.5.4-D1/R-12); ger/enc_pessoal; **operador: só o setor de CADASTRO (ramo v1.6.0-F7 :702 — antes concluía qualquer setor do grupo)** | -1: 403 (v1.5.4-A) | CS | — | exige aberta; sem setor → 403 (mensagem única) |
| POST | /api/conferencia/{id}/setor/{sid}/reabrir | server_conferencia.go:737 | idem | idem; **ramo operador F7 :780** | -1: 403 (v1.5.4-A) | CS | — | |
| GET | …/setor/{sid}/pre_fechamento | onda_0910:200 | confMarcarAuth (+**exigeSetorOperador** F4) | idem; **ramo operador F7 :240 (só o próprio setor)** | -1: 403 (v1.5.4-A) | — | pessoas+CF | ordena pela antiguidade UNIFICADA das 3 fontes da tag (v1.5.4-D3/R-7); não-marcados saem como nao_verificado (COALESCE situacao — antes a linha NULL era descartada no Scan); ramo chefe ainda lê o CONTEXTO sem conferir comando (resíduo D-5) |
| GET | /api/conferencia/funcoes-antiguidade | onda_0910:380 | **confLeituraAuth** (F6) | idem hoje; admin: vazio+aviso | -1: 403 (v1.5.4-A) | — | funcoes | picker; tags DO grupo |
| GET | /api/conferencia/{id}/relatorio.pdf | server_conferencia.go:1207 | confPDFAuth + escopo interno | ger/op/enc_pessoal/aux; **chefe só c/ setor envolvido COMANDADO** (conferenciaEnvolveSetorComandado); admin 403 (v1.5.4-D2/R-2) | -1: 403; N: própria+subordinados | — | tudo | só FECHADA; filtros; assinatura; modo antiguidade sai na escada (mesma ordem do relatório em tela, v1.5.4-D3/R-7) |
| POST | /api/conferencia/{id}/arquivar | server_conferencia.go:808 | confAuth + portão R-8 interno | **só gerente/enc_pessoal** (op 403 — R-8 corrigido v1.5.4-E2) | -1: 403 (v1.5.4-A) | conferencias | — | só FECHADA; backup |
| DELETE | /api/conferencia/arquivada/{id} | server_conferencia.go:856 | **auth(true)** | admin | — | comentarios, presencas, CE, conferencias | — | R-15 corrigido v1.5.4-E2 (sai em tx) |
| POST | /api/conferencia/{id}/escala | onda_0510_escalas.go:88 | auth(false)+guardaEscala | ger(chefe/op do grupo); chefe(só op do setor) | gid==esc | CE | usuarios | escala de guarda da conferência |
| DELETE | …/escala/{usuario_id} | onda_0510_escalas.go:140 | idem | idem | idem | CE | — | |
| GET | /api/conferencia/{id}/stream | server_realtime.go:68 | **confLeituraAuth** (F6; rota :1811) | idem hoje | -1: 403 (v1.5.4-A) | — (hub mem) | conferencias | SSE; sem consumidor front |
| POST | /api/comentarios | server_conferencia.go:1539 | confAuth (**exigeSetorOperador** F4) | ger/op/enc_pessoal | -1: 403 (v1.5.4-A) | comentarios | tags | tag validada grupo+superiores |
| GET | /api/comentarios/{id} · /api/pessoas/{id}/comentarios | :1603 · server_pessoal.go:502 | confAuth | idem | -1: 403 (v1.5.4-A) | — | comentarios | histórico |

**Se você alterar este módulo, verifique também:** enum de situações (muda % presença em TODOS os
relatórios/PDFs — `montarBundle` server_conferencia.go:1203 + server_relatorios.go); `predicadoAntiguidade`
replicado em hoje/estado/pre_fechamento + `pessoasAtivasOpt` (contrato do **hash** do /estado consumido
pelo tick 2s e pelo `ops/carga_conferencia.py`); escalas (badges/pré-fill do iniciar); catálogos
(funções/destinos auto 'Serviço de Escala'/tags); carry-over (colunas de presencas); grupo_vinculos
(herança {id}GET/PDF); SSE+backupAssíncrono (concorrencia_test); migrações v40/v43/v44 e a tabela lazy CD.

**Testes:** conferencia_realtime_test, setores_conferencia_test, v1_5_conferencia_setor_test,
onda_despacho_test, onda_0910_conf_antiguidade_test(+_grupo), onda_v154_d3_test, onda_0510_conf_escopo_test,
onda_0510_escalas_test, contexto_govema_conferencia_test, onda_multichefe_test, multi_setor_chefe_test,
regressao_conferencia_destino_test, concorrencia_test, ordem_diretor_conferencia/0410/pdf_v2, v1_test,
onda_0610_pessoal_test, onda_f3_e2e_personas_test, sci_ordem_0610_frented_test, onda_v154_d2_test,
**onda_v160_f6_test (leituras confLeituraAuth), onda_v160_f7_test (recorte de setor conf+material), onda_v160_migracao_test (v45)**.

---

## 6. PESSOAL/USUÁRIOS

Arquivos: `server_pessoal.go`, `onda_0610_pessoal.go`, `onda_0510_c2.go`, `auth.go`, `helpers.go`,
sessões em `store.go`. Tabelas: `usuarios`, `pessoas`, `usuario_papeis`, `funcoes`, `funcao_membros`,
`sessoes`, `pessoas_apresentacao` (lazy), `auditoria`.

Guardas: `guardaGestaoPessoal` = admin+gerente sempre; **CONTEXTO `enc_pessoal`/auxiliar ATIVO**
(v1.6.0-F2 — `ehEncarregadoDePessoal`/`ehAuxiliarDePessoal` leem `u.Papel`; a cadeira sem o contexto
não dá poder) restrito ao próprio grupo. `podeGestaoPessoal` = mesma regra em função
(onda_0610_pessoal.go:59). Contas: admin cria qualquer (default **sem grupo**); gerente cria
chefe_setor **e sem_funcao** do próprio grupo; chefe promove operador DO SEU SETOR; contexto
enc_pessoal cria operador/chefe_setor/**sem_funcao** do grupo. **v1.6.0-F4: OPERADOR nasce
OBRIGATORIAMENTE com setor do grupo** (`hUsuariosAdd` :1360 — herda o do chefe criador ou setor
explícito de gerente/admin; sem setor = 400) e `hUsuarioPapelAdd` (mensagens.go:216) exige o setor na
atribuição/re-atribuição (grava `usuarios.setor_id`; operador NUNCA ganha comando em
`chefe_setores`). **v1.6.0-F5:** `sem_funcao` (conta bloqueada de login+senha) exige grupo de
vinculação e NÃO ganha linha em `usuario_papeis` (:1412 — o CHECK da v45 nem aceita); não pode ser
criada por chefe nem via `POST /api/usuarios/{id}/papeis` (mensagens.go:127). Senha "sci"/vazia →
`precisa_setup=1` (bloqueia tudo exceto setup/me/logout). **v1.5.4-E1:** admin semeado com a senha
PADRÃO `admin` no 1º boot também nasce `precisa_setup=1` (troca obrigatória; seed com senha própria
não bloqueia).

| Método | Rota | Handler (server_pessoal.go:linha) | Guarda | Papéis | Escopo | W | R | Notas |
|---|---|---|---|---|---|---|---|---|
| POST | /api/login | hLogin :15 (rota :1533) | pública + rate-limit | — | — | sessoes | usuarios | cookie Strict; setores_chefiados |
| POST | /api/setup | hAuthSetup :70 | auth+precisa_setup | conta em setup | — | usuarios | — | troca login+senha |
| GET | /api/me | hMe :110 | auth(false) | qualquer | — | — | várias | contrato do front (ME, funcoes_grupo) |
| POST | /api/logout | hLogout :139 | auth(false) | — | — | sessoes | — | |
| POST | /api/senha | hTrocarSenha :147 | auth(false) | própria | — | usuarios, sessoes | — | **R-16 CORRIGIDO (E1):** invalida as demais sessões da conta; a CORRENTE é preservada (troca pela própria conta — decisão §3.C) |
| POST | /api/sessao/contexto | mensagens.go:17 | papel próprio | qualquer | — | sessoes | — | **R-9 CORRIGIDO (E1):** setor precisa pertencer ao grupo do papel ativo; chefe_setor só setor COMANDADO (`chefeComandaSetorNoGrupo`); admin livre; `usuarios.setor_id` NÃO é mais reescrito (contexto = sessoes.setor_ativo_id, D-5). Dropdown lista TODAS as linhas de `usuario_papeis` — inclui as cadeiras enc_* (v45/F1) |
| POST/DELETE | /api/usuarios/{id}/papeis[/{pid}] | mensagens.go:104/:301 | allowlist `podeGerirPapelAlvo` (E1; **context-bound F2**) | admin (global); ger (operador/chefe na própria árvore); enc/aux de pessoal (operador/chefe do próprio grupo); **fail-closed** p/ o resto | 0/N | usuario_papeis, sessoes, chefe_setores | — | **R-10 CORRIGIDO (E1):** fim do fail-open do Del (chefe/sem-papel barrados; alvo resolvido ANTES da decisão; re-chaveio de sessões do alvo só pós-autorização); trava do único papel mantida; **D-1/R-12**: Add de chefe_setor+setor_id MATERIALIZA o comando (UPSERT 1:1); Del do papel apaga os comandos do grupo. **v1.6.0-F4:** papel operador EXIGE setor do grupo na atribuição (400 sem setor; grava `usuarios.setor_id`, nunca comando); **F5:** `sem_funcao` recusado aqui (400 — ausência de contexto não é papel de linha, mensagens.go:127) |
| GET | /api/pessoas/{id}/ficha | hPessoaFicha :391 | auth(false) | qualquer | -1: 403 (v1.5.4-A) | — | pessoas… | PII |
| GET | /api/pessoas | hPessoasList :587 | auth(false) | qualquer | -1: 403 (v1.5.4-A) | — | pessoas, auditoria | GROUP BY auditoria full-scan |
| GET | /api/pessoas/apresentacao | onda_presenca_banco.go:21 | auth(false) | qualquer | -1: 403 (v1.5.4-A) | (ensure) | lazy table | |
| POST/PATCH/DELETE | /api/pessoas[/{id}] | :685/:759/:642 | guardaGestaoPessoal | adm/ger/enc | 0/N | pessoas | funcoes | grupo forçado; c/ presenças→inativo |
| POST | /api/pessoas/{id}/apresentacao | onda_presenca_banco.go:68 | guarda | idem | 0/N | lazy | | upsert |
| GET | /api/pessoas/{id}/modificacoes | onda_presenca_banco.go:145 | auth(false) | qualquer | -1 403 (ok) | — | auditoria | últimos 30 |
| GET | /api/pessoas/{id}/qr · /pdf | :1350 · :468 | auth(false) | qualquer | -1: 403 (v1.5.4-A) | — | tudo | ficha PII completa |
| GET | /api/usuarios | hUsuariosList :877 | auth(false) | c/ grupo | 0/N(+subord ger); -1 403 | — | usuarios… | col. `senhas` adm/ger/chefe |
| POST | /api/usuarios | hUsuariosAdd :1250 | interna | ver regras | 0/N | usuarios, papeis | — | default sem grupo; **v1.6.0-F4: operador EXIGE setor do grupo** (:1360 — chefe passa o próprio; gerente/admin informam explícito; 400 sem setor); **F5:** aceita `sem_funcao` (admin/gerente/enc_pessoal — nunca chefe; :1386 exige grupo de vinculação; NÃO cria linha em `usuario_papeis`, :1412) |
| PATCH | /api/usuarios/{id} | hUsuarioEdit :302 | guarda | adm/ger/enc | 0/N | usuarios, pessoas | — | espelha pessoa; **v1.6.0-F5:** `sem_funcao` do próprio grupo é editável (reparo pré-designação); gerente/enc NÃO alteram grupo (só admin); ⚠ ramo admin com `grupo_id` insere linha 'operador' genérica no alvo (R-18 ampliado, §14) |
| DELETE | /api/usuarios/{id} | hUsuarioExcluir :1279 | interna | adm;ger | 0/N | usuarios | — | FK→desativa; ⚠ órfãs sessões/papéis (R-17) |
| POST | /api/usuarios/{id}/senha | hUsuarioSenha :183 | interna | adm;ger | 0/N | usuarios, sessoes | — | enc 403; **R-16 (E1):** invalida TODAS as sessões da conta afetada |
| GET | /api/usuarios/{id}/foto | hUsuarioFotoGet :1089 | auth(false) | própria sempre (avatar); admin global; grupo vê grupo | R-5 corrigido v1.5.4-E2 (403 fora) | — | usuarios | cache public 3600 |
| GET/PATCH | /api/perfil | :954/:978 | auth(false) | própria | — | usuarios, pessoas | — | foto_base64 validada na escrita (`validarFotoBase64` — R-3 corrigido v1.5.4-A/B) |
| PATCH | /api/usuarios/{id}/mover | hMoverConta :1049 | interna | adm;ger(árvore) | 0/N | usuarios | — | ⚠ não move usuario_papeis/sessões (R-18) |
| GET/POST/DELETE | /api/grupo/funcoes/membros[/{id}] | onda_0510_c2.go:35/:104/:226 | interna | GET: c/ grupo; SET/DEL: ger/adm; enc**só cadeira enc_material** | 0/N | funcao_membros, usuario_papeis, sessoes | funcoes | onda 10/10; devolve `chave`; **v1.6.0-F3: SET MATERIALIZA a linha de papel da cadeira enc_* (`INSERT OR IGNORE`) e DEL desmaterializa quando não resta designação, com RE-KEY das sessões presas — linhas enc_* fora do display-sync de `funcao_id`** |

**Se você alterar este módulo, verifique também:** contrato `/api/me` (core.js `definirUsuario`,
`rotaInicial`, `gestorPessoal/Material`, `ehEncPessoal`); guardas derivadas (confAuth/authMaterial
leem as mesmas cadeiras); conferência (`hPessoaComentarios` roteada lá); grupos (trocar-gerente,
NUKE); mensagens (destinatários por papel; caixa da função); escalas/material (designações); tests
onda_0610_pessoal, onda_1010_gap1, fix_encarregado(±v367), onda_f2/f3, onda_r1/r2/r3,
multi_papel_e_mensagens, guardas_auth, perfil_e_grupos, **onda_v154_e1 (R-9/R-10/R-16)**,
**onda_v160_f2/f3/f4/f5_test, onda_v160_migracao_test (v45)**.

---

## 7. MATERIAL

Arquivo: `server_material.go` (+PDFs `relatorio.go`, watchdog `server_catalogo.go`, schema store.go
v14/16/27/28/29/36/41). Tabelas: `material_categorias/itens/cautelas/cautela_anexos/tipos/classes/
viaturas/item_anexos/item_comentarios/conferencias/conferencia_itens`, `grupo_setor_responsaveis`.

`authMaterial` (server_material.go:2597): MODO_RESERVA→423; sessão; papel ∈ {gerente, operador,
chefe_setor} OU **CONTEXTO `enc_material` ATIVO** (v1.6.0-F2 — `ehEncarregadoDeMaterial` = u.Papel) →
passa; **admin 403**; **v1.6.0-F4: operador sem setor 403** (`exigeSetorOperador` dentro do
middleware); **v1.6.0-contextos: chefe_setor SEM setor 403** (mesma mensagem do guarda central).
**Doutrina de NÍVEL do módulo (v1.6.0-contextos — MATERIAL por nível de acesso):**
gerente/CONTEXTO enc_material visualizam e gerem TODO o grupo (status quo mantido); **chefe_setor**
visualiza + ADICIONA/CRIA/ATUALIZA o material do PRÓPRIO setor (visto a nível grupo pelo enc) e
NÃO exclui nem dá baixa — 403 "exclusão e baixa de material é ato do gerente ou encarregado de
material" no `hMaterialItensDel`, que cobre TAMBÉM a baixa patrimonial (ela é o ramo
`?modo=baixar` do DELETE de itens; não existe handler de baixa separado); **operador** APENAS
cautelar e descautelar (save/del 403 "operador não tem permissão para cadastrar ou editar
materiais"/"...excluir ou baixar..."); a cautela/descautela do chefe MANTIDAS com recorte do
próprio setor (interpretação registrada em `hMaterialCautelar`/`hMaterialDevolver`).
**v1.6.0-F7 — RECORTE DE SETOR do operador E do chefe (onda_v160_material_setor.go):**
`setorEscopoMaterial` (:34) devolve nil (escopo GRUPO) para gerente/enc_material e o setor de
atuação para operador/chefe_setor (`setorDoUsuario`); `recortaSetorMaterial` (:48) RECUSA
operador/chefe sem setor (403 — mesma mensagem do guarda central; chamar ANTES de abrir tx) e
`setorNoCorte` (:60) exige o objeto (item/cautela — setor do ITEM, não tem setor próprio —
/conferência de material) no próprio setor, **setor NULL = Carga Geral, FORA do recorte**.
Leituras filtram no SQL, escritas checam o objeto na/antes da tx; o `iniciar` de conferência de
material FORÇA o setor do operador/chefe (ignora o do corpo); bipagem/fechamento e PDFs
(pronto/inventário/recibo/etiquetas) idem. **Termos de exibição (v1.6.0-contextos):** rótulos,
mensagens e relatórios impressos dizem **Cautelar/Descautelado/Cautelado** — o valor gravado
`status='acautelado'` é ENUM (CHECK store.go:678/2609) e `data_devolucao`/`obs_devolucao`/
`status='devolvida'` são esquema/valores persistidos: NUNCA renomear identificadores, só o texto
humano. **Categorias:** leitura para todos do módulo;
ESCRITA (add/del) só gerente/CONTEXTO enc_material (:1231/:1293 — o catálogo é da reserva, dimensão
de grupo). Ciclo do item: `disponivel→acautelado→(descautela)→…manutencao/baixado` (CHECK store.go:795);
sensibilidade `controlado` força qtd=1+patrimônio; UNIQUE(grupo, patrimônio); descautela parcial splita
linha; **1 conferência aberta/grupo/setor/dia** (índice parcial v41); watchdog SLA 30min→webhook;
anexos base64 ≤800KB com allowlist MIME+`attachment`+nosniff (o modelo correto — replicar no Drive).

| Método | Rota | Handler (sm:linha) | Guarda | Escopo | W | Notas |
|---|---|---|---|---|---|---|
| GET/POST/DELETE | /api/material/categorias[/{id}] | :1198/:1229/:1291 | authMaterial | leitura ok; **escrita (F7) só gerente/CONTEXTO enc_material** (:1229/:1291); escrita -1: 403 (v1.5.4-A); global só admin | material_categorias | em uso→desativa |
| GET/POST/DELETE | /api/material/itens[/{id}] | :1340/:1483/:1691 | authMaterial | -1: 403 (v1.5.4-A); **níveis (v1.6.0-contextos): operador SÓ cautela/descautela — save/del 403; chefe_setor grava/edita o PRÓPRIO setor (F7 — setor do corpo IGNORADO e forçado ao dele; item de outro setor 404 honesto P1-2); Del (e baixa `?modo=baixar`) só gerente/enc_material — chefe/operador 403** | itens, viaturas | ?status/?categoria/?garagem; exclusão atômica tx |
| GET | /api/material/itens/{id}/qr | :2425 | authMaterial | -1: 403 (v1.5.4-A); **F7: item do próprio setor** | — | `sci://m:{id}:{pat}` |
| GET | /api/material/etiquetas-lote.pdf | :2480 | authMaterial | -1: 403 (v1.5.4-A); **F7: ids ∩ setor do operador** | — | 10/folha A4 |
| GET | /api/material/inventario/pdf | :1108 | **authMaterial** (v1.5.4-D2/R-2) | exigeEscopo; -1: 403; **F7: operador sai só com o próprio setor** | — | reservaAtivo→423 |
| POST | /api/material/cautelar · devolver | :1806 · :1965 | authMaterial | -1: 403 (v1.5.4-A); **F7: item do PRÓPRIO setor (query na tx); chefe_setor MANTÉM cautela/descautela do próprio setor (v1.6.0-contextos — interpretação no handler); operador idem** | cautelas, itens | tx; saldo; parcial split |
| GET | /api/material/cautelas | :2067 | authMaterial | -1: 403 (v1.5.4-A); **F7: recorte pelo setor do ITEM da cautela** | — | LIMIT 200 |
| GET | /api/material/cautelas/{id}/recibo.pdf | :1030 | **authMaterial** (v1.5.4-D2/R-2) | exigeEscopo + cautela no escopo; -1: 403; **F7: recibo só de cautela do próprio setor** | — | 2 vias; auditoria |
| POST/GET | …/cautelas/{id}/anexos · /api/material/anexos/{id} (GET/DEL) | :2168/:2265/:2318/:2385 | authMaterial+cautelaNoEscopo | -1: 403 (v1.5.4-A); **F7: recorte pelo setor do item da cautela** | cautela_anexos | allowlist MIME |
| GET/POST | /api/material/responsaveis | :30/:77 | authMaterial (+papel p/ POST) | POST: sem grupo 403 (exigeEscopo, v1.5.4-A) | grupo_setor_responsaveis | enc material por setor |
| GET/POST | /api/material/itens/{id}/anexos · item-anexos/{aid} (GET/DEL) · itens/{id}/comentarios (GET/POST) | :141/:201/:282/:334/:381/:443 | authMaterial | -1: 403 (v1.5.4-A); **F7: objeto do próprio setor** | item_anexos/comentários | |
| GET/POST | /api/material/conferencias · /iniciar · /{id} · /bipar · /fechar | :498/:552/:672/:781/:870 | authMaterial | -1: 403 (v1.5.4-A); **F7: operador só conf do PRÓPRIO setor (Carga Geral fora); iniciar FORÇA o setor do operador (:569); bipar/fechar conferem o setor** | mat_conf(+itens) | POST /conferencias é rota morta (lista); ⚠ o GET descarta conferências ABERTAS (Scan de `fechada_em` NULL falha — R-29) |
| GET | /api/material/conferencias/{id}/pronto.pdf | :914 | **authMaterial** (v1.5.4-D2/R-2 — antes auth(false) sem guard) | exigeEscopo + conf. do próprio grupo; -1: 403; **F7: só a conferência do próprio setor** | — | reservaAtivo→423 |

**Se você alterar, verifique também:** os 4 geradores de PDF (relatorio.go:861/1095/1231/1709);
watchdog+sino (`server_catalogo.go:560`, `server_admin.go:350`); ficha pessoal (cautelas ativas,
server_pessoal.go:527); NUKE (⚠ não apaga material_conferencias → FK aborta, R-6); etiquetas
(material_tipos/classes); front views_material.js (controles por NÍVEL: operador só
cautela/descautela; chefe sem excluir/baixar — esconde-controle, o 403 nunca é a experiência;
termos Cautelar/Descautelar só em TEXTO) + gates core.js:1305; **recorte de setor do
operador/chefe (`setorEscopoMaterial`/`recortaSetorMaterial` — TOQUE OS DOIS LADOS, porta de papel +
recorte por handler)**; tests onda_material_*,
v1_5_material_*, v15_cia_fixes, fix_encarregado(±v367), regressao_escalas_material, onda_v154_d2_test,
**onda_v160_f7_test**.

---

## 8. MENSAGENS · DRIVE · MURAL

Todos `a.auth(false)` + checagens internas. Arquivos: `mensagens.go` (também abriga
`hMudarContexto`, `hUsuarioPapelAdd/Del` e os handlers do MURAL), `drive.go`, `anexos_drive.go`,
`onda_0510_drive.go`, `onda_0510_funcao.go`, rotas do mural em server_admin.go:436-443.

**MENSAGENS/DESPACHOS** — destinatário = **linha por papel** (`mensagem_destinatarios.destinatario_papel_id`);
trocar contexto troca a caixa. **Hierarquia de envio (v1.5.4-E1, R-14 CORRIGIDO) — matriz de
quem → quem** (`hMensagensEnviar`, mensagens.go:677+):
| Remetente (papel ativo) | Pode enviar para |
|---|---|
| admin | qualquer papel (global — infraestrutura, matriz §4) |
| gerente / chefe_setor / operador / enc_pessoal / enc_material **com grupo** | papéis do PRÓPRIO grupo + de grupos SUBORDINADOS ativos (`gruposSubordinadosAtivos`, vínculo bilateral) + caixa **admin** (destino global de infraestrutura) |
| resposta a despacho (`pai_id`) | + o REMETENTE da mensagem pai (fora da árvore também — a thread é participativa) |
| conta SEM grupo ou SEM função (`sem_funcao`/papel fora da lista — `papelTemEscopoDeDados`, v1.6.0-F5, mensagens.go:671) | **ninguém — 403** (antes: operador sem grupo e gerente/chefe enviavam a qualquer grupo) |

Caixa da Função: mensagens carimbadas com a função titular do remetente aparecem para quem
exerce a função (`da_funcao=1`, não arquivável) — ⚠ mas a **thread** dá 403 para o exercente
(mensagens.go:1068-1072, R-19). Despacho = `tipo='despacho'` + `exige_resposta`; finalizar só pelo
destinatário (1×). Rotas: inbox/enviadas/enviar/ler/excluir/arquivar/desarquivar/pastas
(GET/POST/DEL/mover)/thread/responder/finalizar/contador/destinatários (server_pessoal.go:1547-1562).
⚠ `hMensagensEnviar` sem tx (R-13); `/api/mensagens/destinatarios` lista global (o picker mostra
nomes fora do escopo; o SERVIDOR recusa o envio fora da matriz acima).

**DRIVE** — admin barrado (doutrina). ACL por item (`checarAcessoPasta/Arquivo` drive.go:32-205):
autor → gerente (árvore) → grant direto (`drive_compartilhamentos` alvo usuário/papel/grupo) →
herança da pasta → braço da função (só leitura). Arquivo solto sem grant não é lido por membro do
grupo. Storage em disco `dados/drive/` (`nome_armazenado` UNIQUE; upload multipart ≤128MB; **sem
cota, sem lixeira — DELETE remove o físico**; backup cobre só o banco ⚠ R-20). 17 rotas
(drive.go:1075-1091): itens/da_funcao/seletor(⚠ morto)/pastas CRUD/upload/download(`inline` só
para a allowlist PDF/PNG/JPEG/WEBP — `mimeDriveInlinavel`, resto attachment+nosniff e filename
sanitizado; R-11 corrigido v1.5.4-E2)/mover/copiar/propriedades/arquivo PATCH/DEL/compartilhar
CRUD/arquivo_grupo(gerente).
`/api/drive/seletor` lista sem passar pela ACL e não tem consumidor — remover ou blindar.

**MURAL DE AVISOS** — leitura: admin global ou qualquer papel com grupo E com escopo
(`podeVerMural`, onda_0510_c2.go:25 — **v1.6.0-F5: 'sem_funcao'/papel fora da lista NÃO lê mural nem
com grupo**; ⚠ filtro tautológico mensagens.go:1400); escrita: gerente/admin; detalhes+ciência: quem
vê, COM a checagem de grupo do aviso desde a v1.5.4-E2 (`avisoNoEscopo`, onda_v154_e2.go — R-21
corrigido; ⚠ `hAvisosComentar` seguida sem a checagem — resíduo); excluir: autor ou gerente do grupo
(admin não-autor 403 — assimetria); repostar: só gerente (origem no escopo). **Publicar aviso
dispara**: 1 mensagem de notificação POR papel do grupo (**v1.6.0-F3: a consulta inclui os contextos
`enc_pessoal`/`enc_material` materializados** — o enc puro volta a receber notificação) +
**síntese de `usuario_papeis` para contas que nunca logaram** (mensagens.go:1570-1630,
sem tx) — efeito colateral pesado, mexer com cuidado. Rotas: server_admin.go:436-443 (8 rotas) +
`GET /api/notificacoes` (hub do sino: cautelas atrasadas+despachos+avisos pendentes).

**Se você alterar, verifique também:** `onda_0510_funcao.go` (funçõesExercidas/carimbo — inbox,
drive, escalas); `anexos_drive.go` + seletor do core.js (anexos de mensagens/avisos);
`server_admin.go` hNotificacoesHub (queries duplicadas do contador/mural); contrato papel_ativo
(UsuarioDaSessao); fronts views_mensagens/drive/avisos + CACHEBUST; tests multi_papel_e_mensagens,
ordem_diretor_email_interno/0710_avisos, onda_0510_funcao/drive/itens79, v1_2_fase2/3,
admin_restricao, **onda_v154_e1 (R-14 — matriz de envio)**.

---

## 9. GRUPOS · ADMIN · CATÁLOGOS · RELATÓRIOS

**GRUPOS & HIERARQUIA** (server_grupos.go, rotas :745-765): vínculo **bilateral** (1+1 em
`grupo_vinculos`); `gruposSubordinadosAtivos` = BFS transitivo (⚠ `gruposSuperioresAtivos` NÃO exige
bilateral — assimetria R-22; anti-ciclo só imediato R-23). `escopoDoUsuario` :244 — **v1.6.0-F5:
primeiro o papel (`papelTemEscopoDeDados`): fora da lista → -1, nem com grupo**. nomear/destituir
chefe: gerente ou enc_pessoal, `gid==escopo` (admin 403); substituição 1:1 via `setor_id` UNIQUE;
multi-chefia por usuário. trocar-gerente: admin; rebaixa anterior e **migra grupo_id do alvo**.
excluir grupo: admin (auth(true)); vazio/`forcar`/`nuke` (senha do admin no corpo) — NUKE cobre
desde a v1.5.4-E2 o rol completo de FKs NO ACTION: conferencia_escalas (por conferência E por
usuário), mural (avisos+cientes+comentários, inclusive cruzados, com NULLing de
aviso_origem_id/grupo_origem_id), escala_modelos+postos+aptos (antes do escala_tipos),
material_conferencias+itens, chefe_setores, funcao_membros, grupo_setor_responsaveis,
setor_sugestoes, usuario_papeis do grupo e funcao_id/setor_id de cadastro (R-6 corrigido);
comentário promete backup automático que não existe (R-24). `/api/grupos` e
`/api/vinculos` auth(false) — -1: 403 na `/api/grupos` (`exigeEscopo`, v1.5.4-A) e JSON vazio na
`/api/vinculos` (R-1 corrigido).

**ADMIN** (server_admin.go): backup `VACUUM INTO`+sha256+MANIFEST (mutex; dispara boot/fechamentos/
manual/CLI `sci backup`); **import com 3 bugs verificados** (R-4: sem rename `.novo`→`sci.db`;
reabre migrando só até v12; rollback reabre o próprio import). `GET /api/configuracoes` **público**
com allowlist (⚠ inclui WEBHOOK_ATRASOS_URL — R-25); POST só admin. Métricas admin-only. health público.

**CATÁLOGOS & SETORES** (server_catalogo.go, rotas :1003-1016): allowlist `tabelaDeCatalogo`
(setores/funcoes/destinos/tags/conferencia_tipos/status_pessoal/material_tipos/material_classes).
Leitura auth(false) com herança (próprio+superiores+subordinados); funções só `tipo='antiguidade'`
(v368; cadeiras tipo='grupo' hardcoded v39). Escrita `guardaGestaoPessoal`; editar/excluir dono ou
subordinado; reparentar só dono (anti-ciclo completo, prof ≤8). `hSetorExcluir` (ordem_0610_setores.go):
gerente(árvore)/enc(próprio) — remaneja pessoas/contas p/ SEM SETOR em tx; desde a v1.5.4-E2
(R-6 corrigido) revoga o COMANDO do setor (chefe_setores + purga do papel chefe_setor que perdeu
o último comando no grupo, com re-chave de sessão — doutrina D1), limpa
`grupo_setor_responsaveis` e remaneja o MATERIAL do setor (itens/cautelas/conferências de material
ficam SEM setor, mesma doutrina do pessoal). `hOperadoresDoSetor`: chefe designa operador do
seu setor (valida grupo+setor). `setor_sugestoes`: workflow sugere→avalia (ger/chefe/admin) com
allowlist de efeito revalidada na aplicação (tx, TOCTOU guard) — o padrão-ouro de escrita indireta.
`hSetoresAgregado` lê chefe/qtd_chefes de `chefe_setores` desde a v1.5.4-D1 (antes inferia por
`usuario_papeis`+`usuarios.setor_id` — fonte divergente da UI de catálogo, R-12 corrigido). Envelope
`{funcoes,total}` é **exclusivo do catálogo de funções** (correção 09/10,
`fb32f18`); demais catálogos devolvem array cru — manter esse contrato em qualquer mudança do
`hCatalogoList` (regressão cravada em `fix_catalogo_envelope_test.go`).

**RELATÓRIOS & EXPORT** (server_relatorios.go :414-424 + relatorio.go): todos auth(false); recorte
por `escopoRelatorio` (server_relatorios.go:212 — `exigeEscopo` 403 p/ -1; ?grupo= com 403 fora da
árvore; v1.5.4-A fechou o R-1 da família inteira). **v1.6.0-F4:** os handlers de DADOS
(presenca/periodo, relatorio JSON/PDF, detalhado.pdf — :244/:263/:279/:312) chamam
`exigeSetorOperador` antes do escopo: operador sem setor 403; **com setor, o recorte segue de GRUPO
(pendência M1 — o recorte por setor do operador NÃO está implementado aqui)**. Rotas: registros, tags, efetivo_atual, presenca/periodo, relatorio
(JSON/PDF/detalhado.pdf), export CSV (`/api/export/{t}`), export SQLite/JSON (`/api/export`).
PDFs gerados (relatorio.go): relatório geral :305 · detalhado :2166/:2128 · conferência :547 ·
ficha pessoal :677 · recibo cautela :861 · escalas :1029 · escala do dia :1846 · inventário :1095 ·
pronto material :1231 · etiquetas :1709. Fuso America/Sao_Paulo em tudo (fmtDataBR); ordenação
v9.17 = hierarquia de setor → função → nome. ⚠ rótulo "antiguidade = ID menor" desatualizado (:474).

**Se você alterar, verifique também:** `escopoDoUsuario`/`filtroArvore` (todos os callers de
relatórios e conferência); `grupo_vinculos` (árvore, herança de catálogo, EhSubordinado duplicado);
`chefe_setores` (catálogo, conferência, agregado — FONTE ÚNICA desde a v1.5.4-D1, R-12 corrigido); `funcao_membros`
(**materializa linha de papel — sync F3**; guardas context-bound desde a F2 + front); backup (ReabrirComArquivo, MANIFEST, ci.sh); NUKE × FKs;
views_gestao/pessoal/config + core.js (config pública pré-login); tests perfil_e_grupos,
subordinacao_e_setores, ordem_diretor_hierarquia, api_backup, admin_restricao, v368, sci_ordem_0610_frented.

---

## 10. ESCALAS · CALENDÁRIO (dormentes)

Backend 100% registrado; rotas front bloqueadas em core.js:1106 (`ViewModuloEmDesenvolvimento`).
`web/views_escalas.js` (1453 ln) e `views_calendario.js` (900 ln) embutidos e inalcançáveis.

**ESCALAS** — `reservaAuth` nas 21 rotas (v1.5.4-D2/R-2: `/api/escalas/pdf`,
`/api/escalas/relatorio-dia.pdf` **+** `/pdf` e `/api/escalas/minhas` saíram do `a.auth(false)`
pelado e hoje exigem o MESMO guard do módulo): {ger/op/chefe}, admin 403, 423 se MODO_RESERVA;
**v1.6.0-F4: operador sem setor 403** (`exigeSetorOperador` dentro do guard, server_escalas.go:1827).
**O recorte do operador segue de GRUPO nesta onda** — módulo dormente; o recorte por setor pende do
M5 (decisão D-3 atualizada no roadmap). `escalas/minhas` também barra conta sem grupo (prólogo
`exigeEscopo`); a ABERTURA de `minhas` a todos os papéis quando o módulo reativar segue **PENDENTE
de decisão (D-3)** — não decidida nesta onda. As rotas de PDF de escalas NÃO aceitam parâmetro de
grupo: o recorte é sempre o escopo da SESSÃO (outro papel com grupo lê o PDF do PRÓPRIO grupo — o
teste D2 prova a não-vazamento pelo texto do PDF).
Fases `aberto→preenchido→aprovado→publicado` (**sem máquina de estados** — qualquer→qualquer, em
massa por grupo+data); delegação inter-grupos (subordinados transitivos); modelos
(escala_modelos/postos/aptos com faixa de posto/graduação); InfoDescanso (conflito bloqueante,
folga <24h crítico…). Classe -1 FECHADA: TODOS os handlers exigem `exigeEscopo` (v1.5.4-A — R-1
corrigido) e as rotas `reservaAuth` (D2) — `limpar-dia` não roda com escopo 0 (admin é 403).
**Integração com conferência (reativação muda
comportamento!)**: `criarConferenciaBase` pré-preenche "justificada/Serviço de Escala" para escalados
na data; `escaladosNaData` alimenta badges do hoje (⚠ `escalados_ontem` entregue e nunca renderizado).

**CALENDÁRIO** — 11 rotas auth(false) com 403 admin interno; ACL `checarAcessoEvento`
(autor→gerente→compartilhamento→mesmo grupo). **Duas gerações de API vivas** sobre a mesma tabela
`calendario_compartilhamentos`: `/api/calendarios*` (coleções, v1.3) e `/api/calendario/*` (mesh,
v1.2) — unificar ANTES de reativar (R-27). `GET /api/calendarios/{id}/compartilhamentos` usa desde
a v1.5.4-E2 a mesma régua do compartilhar/revogar (autor do calendário ou gerente do grupo-dono —
R-21 corrigido); `GET /api/calendarios` cria calendário 'Pessoal' no 1º acesso (write-on-GET);
índices únicos anti-dup só para alvo_usuario/grupo de coleções (não evento/papel). Reativação do
calendário sem escalas mostra a seção de escalas sempre vazia.

**Reativação (checklist):** guardas revalidadas contra a matriz §4; rotas desbloqueadas no
`rotear()` + menu; e2e persona re-executado; carga sem regressão; decisões D-3 (roadmap — **D-3
atualizada na v1.6.0: o operador mantém o recorte de GRUPO nas escalas; o recorte por setor pende do
M5**).

---

## 11. Mapa do frontend

`web/index.html` (shell, 4 refs com `?v=NNN`) → `lazy.js` (mapa hash→arquivo + CACHEBUST) → views.
Router `rotear()` (core.js:1135): portões por papel **cosméticos** (§4 é a verdade). Três
tabelas de verdade no core.js que mudam JUNTAS em qualquer doutrina de papel: composição do menu
(`montarShell` :556 — **v1.6.0-F2: menu POR CONTEXTO ATIVO**, sem acréscimo implícito por cargo;
**F5: `sem_funcao` → sidebar vazia**), portões do `rotear()` (:1135 — **F5: qualquer hash de
`sem_funcao` cai em `#/bloqueio`**; **F2: `#/hoje|#/conferencia` aceita `enc_pessoal`,
`#/material` só gerente/`enc_material`, `#/pessoal` e `#/relatorios` gerente/`enc_pessoal`**),
`rotaInicial()` (:126 — **F1/F5: `enc_pessoal`→`#/hoje`, `enc_material`→`#/material`,
`sem_funcao`/vazio→`#/bloqueio`**). O dropdown de contexto lista TODAS as linhas de
`usuario_papeis` (`papeis[]` do /api/me) — **as cadeiras enc_* têm rótulo próprio no `rotuloPapel`**
(core.js:112; cópia sincronizada em views_gestao.js:11) e `papel vazio sem cadeira legada` deriva
`sem_funcao` (F5).

| Rota | View (arquivo) | APIs principais | Portão cliente |
|---|---|---|---|
| #/hoje · #/conferencia | views_conf.js | conferencia/*, /estado (tick 2s), comentarios, escalas | ger/op/chefe/CONTEXTO enc_pessoal (F2; servidor endurece na leitura — confLeituraAuth F6) |
| #/pessoal | views_pessoal.js | pessoas, usuarios, funcoes/membros, nomear_chefe, setores | ger/CONTEXTO enc_pessoal (F2) |
| #/material | views_material.js | material/*, notificacoes | ger/CONTEXTO enc_material (F2) |
| #/grupos | views_gestao.js (ViewGrupos) | usuarios, grupos, vinculos, backup | gerente |
| #/admin · #/configuracoes | views_gestao.js (ViewAdmin) / views_config.js | backup*, configuracoes, metricas | admin |
| #/mensagens · #/despachos | views_mensagens.js | mensagens/*, drive/download | não-operador |
| #/drive | views_drive.js | drive/* | não-operador/não-admin |
| #/avisos | views_avisos.js | avisos/* | todos (menos sem_funcao — F5) |
| #/relatorios | views_conf.js (secção) | relatorio*, registros, tags, export | ger/CONTEXTO enc_pessoal (F2) |
| #/perfil | views_gestao.js (ViewPerfil) | perfil | todos |
| **#/bloqueio** (v1.6.0-F5) | core.js `ViewBloqueioSemFuncao` (:1781) | logout | destino único de `sem_funcao`/papel vazio (sai por SAIR ou pela designação futura + re-login) |
| #/escalas · #/calendario · #/consciencia | — | — | bloqueados (módulos em desenvolvimento) |

 Helpers vivos: `core.js` (api/esc/toast/modais/paginarArray), `views_gestao.js` re-exporta
`tabelaControles`…; ⚠ `ui_helpers.js` (1307 ln) **nunca é carregado** (código morto — decidir destino;
sua `rotuloPapel` :12 NÃO conhece enc_*/sem_funcao — R-32); ⚠ 3 cópias de `esc()` divergentes
(views_avisos sem apóstrofo/fuso).

---

## 12. Mapa de dados

~48 tabelas + views, 69 índices, schema **v45** (const `versaoSchemaBinario`, main.go:23 — v44 da
v1.5.4-D1 materializa chefias legadas; **v45 da v1.6.0-F1** — `migrarV45`, onda_v160_contextos.go:44:
REBUILD de `usuario_papeis` com o CHECK estendido `('admin','gerente','operador','chefe_setor',
'enc_pessoal','enc_material')` preservando ids e colunas (as FKs continuam resolvendo por nome de
tabela; `sem_funcao` fica FORA do CHECK por design — ausência de contexto não é papel), guarda dupla
(`schema_migrations` + DDL em `sqlite_master`, sem transação — PRAGMA foreign_keys é no-op em tx,
padrão v24); MATERIALIZAÇÃO das cadeiras (`funcao_membros` × `funcoes` tipo='grupo' → linha de papel,
INSERT OR IGNORE pela UNIQUE) e RE-KEY das sessões de designado puro para a primeira linha enc).
Migrações: cadeia v2…v45 em `executarMigracoes`
(store.go:1243), usada por `AbrirStore` e `ReabrirComArquivo`; 2 tabelas **lazy** fora do versionamento
(`conferencia_despachos`, `pessoas_apresentacao` — `sync.Once` sem retry). Pragmas: §3.E.

| Domínio | Tabelas | Observações |
|---|---|---|
| Núcleo | grupos, grupo_vinculos, usuarios, pessoas, usuario_papeis, sessoes, auditoria, configuracoes, schema_migrations | sessões guardam SHA-256; auditoria append-only sem índice (entidade,registro_id); `usuario_papeis` CHECK v45 com enc_pessoal/enc_material — **as linhas enc são CONTEXTO (dropdown), não acréscimo de poder (§3.A)**; `sem_funcao` não existe aqui (só em `usuarios.papel`) |
| Conferência | conferencias, presencas, comentarios, tags, destinos, conferencia_setores, conferencia_despachos (lazy), conferencia_funcoes, conferencia_escalas, conferencia_tipos | UNIQUE(conferencia,pessoa); imutabilidade pós-fechamento |
| Organização | setores, funcoes (antiguidade+grupo), funcao_membros, chefe_setores, grupo_setor_responsaveis, setor_sugestoes, status_pessoal | 3+1 camadas de designação (§3.A); `funcao_membros` = designação que MATERIALIZA a linha de papel em `usuario_papeis` (sync F3) |
| Escalas | escala_tipos, turnos, pessoas, modelos, modelo_postos, modelo_aptos | dormente |
| Material | material_categorias/itens/cautelas/cautela_anexos/tipos/classes/viaturas/item_anexos/item_comentarios/conferencias/conferencia_itens | UNIQUE(grupo,patrimônio); índice parcial 1 conf aberta; blobs base64 no banco |
| Mensagens/Drive/Mural | mensagens, mensagem_destinatarios, mensagem_respostas, mensagem_pastas, drive_pastas/arquivos/compartilhamentos, avisos, aviso_cientes, aviso_comentarios | drive: bytes em DISCO `dados/drive/` (fora do backup!) |
| Calendário | calendarios, calendario_eventos, calendario_compartilhamentos | dormente; 2 gerações de API |

---

## 13. Checklists de impacto global

**Alterar papel/permissão de um módulo:**
1. Middleware/guarda do módulo (§5-§10) + prólogos internos dos handlers.
2. As 3 tabelas do core.js (menu, rotear, rotaInicial) + helpers espelhados (gestorPessoal/Material,
   ehEncPessoal/ehEncMaterial — **v1.6.0-F2: leem `ME.papel` (contexto ativo); o fallback por
   nome/funcoes_grupo só sobrevive para papel vazio de sessão legada**).
3. Matriz §4 deste doc + testes persona do módulo.
4. Se toca cadeiras (funcao_membros): **a designação MATERIALIZA linha em `usuario_papeis` (F3) —
   any mudança de sync precisa cobrir materialização + desmaterialização + re-key de sessões**;
   6 guardas servidor (todas context-bound desde a F2) + /api/me (papeis[]/funcoes_grupo) + dropdown
   de contexto.

**Alterar schema:**
1. Migração nova (número sequencial ÚNICO — já colidiu v42/v43) + registro na cadeia E no
   `ReabrirComArquivo` (hoje divergem — R-4).
2. Mapa §12 + queries dos handlers + PDFs de relatorio.go que leem a tabela.
3. NUKE/hSetorExcluir/excluirArquivada (listas de DELETE que precisam cobrir FKs NO ACTION).
4. Testes: setupTestApp re-roda a cadeia inteira 236× — quebra de migração falha TUDO.

**Alterar rota (nova/removida/mudou semântica):**
1. Linha neste doc + lazy.js (se view nova) + index.html (cache-bust) + menu/rotear.
2. Guarda declarada (nenhuma rota de dados "pelada").
3. teste persona + ci.sh.

**Alterar front de view:**
1. Cache-bust (index.html refs + CACHEBUST lazy.js).
2. Contratos /api/me e da view (autocontida p/ deep-link F5 — lição views_pessoal).
3. `esc()` em TODO template novo (usar o do core.js).

---

## 14. Registro de defeitos conhecidos

Prioridade de correção e plano: ver [`ROADMAP.md`](ROADMAP.md) v1.5.4/v1.5.5. Resumo cruzado:

| # | Defeito | Onde (arq:linha) | Classe |
|---|---|---|---|
| R-1 | ~~**Escopo -1 (sem grupo) cai no ramo sem filtro** — ~35 handlers (conferência lista/fechar/arquivar/{id}/pdf/comentários/stream; pessoas ficha/pdf/qr/lista; material quase tudo; relatórios/export inteiros; /api/grupos; escalas limpar-dia apaga TODOS)~~ **CORRIGIDO na v1.5.4-A**: guarda central `exigeEscopo` (helpers.go:125 — conta sem grupo 403; admin 0) + `filtroGrupoSQL` (helpers.go:138 — ` AND 1 = 0` fail-closed no fio) aplicados nos handlers de todos os módulos (conferência, pessoal, material, escalas, relatórios/export, grupos, mural); `/api/vinculos` devolve JSON vazio p/ -1. Auditoria pós-onda (`auditoria-escopos-v154.md`) verificou handler a handler — nenhuma rota de dados deixa o -1 no ramo sem filtro | helpers.go; r1_test.go (matriz de 21 rotas 403) | ✅ |
| R-2 | ~~Rotas de dados `a.auth(false)` sem guarda de papel/escopo (escalas pdf/minhas/relatorio-dia; material pronto.pdf **sem escopo algum**)~~ **CORRIGIDO na v1.5.4-D2**: as 4 rotas de escalas (`/api/escalas/pdf`, `/api/escalas/relatorio-dia.pdf` + duplicata `/relatorio-dia/pdf`, `/api/escalas/minhas`) exigem `reservaAuth` (`minhas` também barra -1 via `exigeEscopo` — abertura ampla pende da D-3/M5); os 3 PDFs de material (`inventario/pdf`, `cautelas/{id}/recibo.pdf`, `conferencias/{id}/pronto.pdf`) exigem `authMaterial`; `/api/conferencia/{id}/relatorio.pdf` exige `confPDFAuth` (papéis do módulo; chefe_setor só com setor envolvido COMANDADO — `conferenciaEnvolveSetorComandado`; admin 403). Doutrina nova: **nenhuma rota de dados sem guard declarado na tabela** (§3.I). Regressão: `onda_v154_d2_test.go` (matriz por persona: gerente/operador do próprio grupo 200; sem-grupo 403 em todas; admin 403; chefe só com setor comandado; operador de outro grupo 403 nos objetos alheios e PDF do próprio grupo sem vazamento) | server_escalas.go:1796-1816; server_material.go:2316-2346; server_conferencia.go:1635-1673 (confPDFAuth + rota :1706) | ✅ |
| R-3 | ~~**XSS armazenado via foto_base64** (gravação sem validação + sinks sem esc + CSP unsafe-inline)~~ **CORRIGIDO na v1.5.4-A/B**: `validarFotoBase64` (server_pessoal.go:17 — allowlist `data:image/(png|jpeg|webp);base64,` + magic bytes + teto ~512 KB) aplicada nas duas escritas (`hUsuarioEdit` :406, `hPerfilSet` :1079); sinks do front com `fotoValida()` + `esc()` (core.js:585, views_gestao.js:548) | server_pessoal.go; r1_test.go (`TestR1_FotoBase64_Validation`) | ✅ |
| R-4 | ~~**Import de backup quebrado** (sem rename .novo→sci.db; reopen só migra até v12; rollback reabre o import)~~ **CORRIGIDO na v1.5.4-A/C**: swap com rename real para `sci.db` canônico (server_admin.go:261-300); `ReabrirComArquivo` (store.go:1378) executa a CADEIA COMPLETA via `executarMigracoes` (store.go:1243 — a mesma de `AbrirStore`, v2…v44); rollback reabre o caminho canônico; `.novo` removido | server_admin.go; store.go; r1_test.go (`TestR1_BackupRestore_AtomicAndRollback`) | ✅ |
| R-5 | ~~Foto de usuário sem escopo (LGPD)~~ **CORRIGIDO na v1.5.4-E2**: a foto própria segue 200 (avatar do perfil); admin (escopo 0) vê tudo; conta de grupo só foto de conta do MESMO grupo; fora disso 403 (conta sem grupo incluída). Regressão: `TestE2FotoUsuarioComEscopo` | server_pessoal.go hUsuarioFotoGet | ✅ |
| R-6 | ~~**NUKE/setor-excluir/excluirArquivada × FKs NO ACTION** (chefe_setores, funcao_membros, avisos, material_conferencias, escala_modelos, setor_sugestoes, CE órfãs…)~~ **CORRIGIDO na v1.5.4-E2**: NUKE com rol completo em tx (conferencia_escalas por conferência/usuario/designante; mural com NULLing de origens e cientes cruzados; escala_modelos+postos+aptos antes do escala_tipos; material_conferencias+itens; chefe_setores; funcao_membros; grupo_setor_responsaveis; setor_sugestoes; usuario_papeis do grupo; funcao_id/setor_id de cadastro NULLados) — teste do grupo "rico" sem erro e sem órfãos; `hSetorExcluir` revoga o comando do setor (chefe_setores + purga do papel chefe_setor sem último comando, re-chave de sessão), limpa grupo_setor_responsaveis e remaneja o material do setor — teste do setor "rico" (`TestE2NukeGrupoRicoSemOrfaos`, `TestE2ExclusaoSetorRicoSemOrfaos`) | server_grupos.go hGrupoExcluir; ordem_0610_setores.go hSetorExcluir | ✅ |
| R-7 | ~~Antiguidade: ordenação ignora fontes u2/up2 e o relatório na tela não ordena por antiguidade; militar sem tag some em silêncio~~ **CORRIGIDO na v1.5.4-D3**: fonte única das expressões SQL em `onda_0910_conf_antiguidade.go` — `filtroAntiguidadeTresFontes` (o predicado do filtro) e `exprAntiguidadeTresFontes` = `COALESCE(fu.antiguidade, fu_u.antiguidade, fu_up.antiguidade, 999)` (precedência pessoa → conta → papel; sem tag = 999, por último), aplicada via `ordemAntiguidadeTresFontes` na listagem do `/hoje` (`pessoasAtivasOpt` com filtro), no pré-fechamento, no relatório em tela `/{id}` e no PDF (`montarLancamentosPDFConferencia`), condicionada ao modo antiguidade (`conferenciaEmModoAntiguidade` — modo setores mantém as ordens legadas); `iniciar`/`despachar` respondem `sem_tag:[nomes]` (`militaresSemTagAntiguidade` — ativos do universo, recorte = setores despachados, fora do filtro por não terem a tag em NENHUMA fonte; `NOT COALESCE(predicado,0)` p/ não perder o sem-tag na lógica tri-estados); bônus da mesma consulta: pré-fechamento voltou a listar os NÃO-marcados (`COALESCE(pr.situacao,'')` — o Scan descartava a linha NULL e o checklist nascia vazio); herança de catálogo segue SEM herança entre grupos (by design 09/10; D-1 pendente). Front do `sem_tag` ainda não consome (pendente) | onda_0910_conf_antiguidade.go; helpers.go (pessoasAtivasOpt); server_conferencia.go (hConferenciaGet/montarLancamentosPDFConferencia/hConferenciaIniciar); onda_despacho.go; onda_v154_d3_test.go | ✅ |
| R-8 | ~~Arquivar/descartar: doc diz gerente, código aceita operador/enc~~ **CORRIGIDO na v1.5.4-E2**: arquivar e descartar exigem gerente ou encarregado de pessoal (mesma régua do fechar); operador 403. Regressão: `TestE2ArquivarDescartarSomenteGerenteEnc` | server_conferencia.go hConferenciaArquivar/hConferenciaDescartar | ✅ |
| R-9 | ~~hMudarContexto aceita setor de qualquer grupo e reescreve usuarios.setor_id global~~ **CORRIGIDO na v1.5.4-E1**: o setor precisa pertencer ao GRUPO do papel ativo; `chefe_setor` só assume setor que AINDA comanda (`chefeComandaSetorNoGrupo` — fonte única `chefe_setores`, D-2); admin é global; o `UPDATE usuarios SET setor_id` global foi REMOVIDO — o contexto vive em `sessoes.setor_ativo_id` (sobrescrito em `UsuarioDaSessao`) e `usuarios.setor_id` fica como cadastro (proposta D-5, decisão pendente) | mensagens.go (hMudarContexto); onda_v154_e1.go; onda_v154_e1_test.go | ✅ |
| R-10 | ~~hUsuarioPapelDel fail-open (chefe remove papéis de qualquer um + re-chaveia sessões)~~ **CORRIGIDO na v1.5.4-E1**: allowlist de solicitantes fail-closed (`podeGerirPapelAlvo` — admin global; gerente operador/chefe_setor no próprio grupo/árvore; enc/aux de pessoal no próprio grupo, doutrina hUsuariosAdd/v367) aplicada no Del E no Add (o Add também deixava passar papel-base não reconhecido com poder de admin); alvo resolvido ANTES de qualquer decisão e re-chaveio de sessões do alvo só pós-autorização; trava "não remover o único papel" mantida | mensagens.go (hUsuarioPapelAdd/Del); onda_v154_e1.go; onda_v154_e1_test.go | ✅ |
| R-11 | ~~Drive serve HTML inline (MIME do cliente) + Content-Disposition sem escapar~~ **CORRIGIDO na v1.5.4-E2**: `inline` só para a allowlist PDF/PNG/JPEG/WEBP (`mimeDriveInlinavel`, mesma lista dos anexos de material); resto attachment + `X-Content-Type-Options: nosniff` SEMPRE + Content-Type octet-stream; filename do Content-Disposition sanitizado (`sanitizarNomeArquivo`). Gate no SERVE (defesa em profundidade p/ tipos hostis legados no banco). Regressão: `TestE2DriveMIMEInlineSeguro` | drive.go hDriveDownload | ✅ |
| R-12 | ~~**Chefe-zumbi**: chefeComandaSetor fallbacks + usuarios.setor_id nunca limpo; hSetoresAgregado usa fonte divergente~~ **CORRIGIDO na onda v1.5.4-D1 (decisão D-2)**: `chefe_setores` vira FONTE ÚNICA — fallbacks `usuarios.setor_id`/`pessoas.setor_id` removidos do `chefeComandaSetor`; novo `setorAtivoComandado` condiciona o CONTEXTO da sessão ao comando vigente (concluir/reabrir/marcar e leitura do `/hoje` — contexto órfão de chefia anterior não autoriza nada); `hUsuarioPapelAdd` (chefe_setor+setor_id) MATERIALIZA o comando (UPSERT 1:1 por setor — chefe anterior perde a linha) e `hUsuarioPapelDel` revoga os comandos do grupo; migração **v44** materializa os legados pendentes (fontes `usuarios.setor_id` → `pessoas.setor_id`, INSERT OR IGNORE — UNIQUE(setor_id) preserva o vigente) e subiu o `versaoSchemaBinario` (44); `hSetoresAgregado` lê `chefe_setores` (igual ao catálogo). Resíduo p/ D-5: designação de escala do chefe e pre_fechamento ainda leem o contexto de cadastro | onda_v154_d1.go (migrarV44 + setorAtivoComandado); onda_0510_conf_escopo.go:139/:179; server_conferencia.go:196/:689/:751; mensagens.go:84/:256; onda_0510_itens79.go:173; onda_v154_d1_test.go | ✅ |
| R-13 | Escritas multi-statement sem tx (hMensagensEnviar, criarConferenciaBase, hAvisosAdd, hGrupoDestituirChefe…) | mensagens.go:691-718 etc. | P1 |
| R-14 | ~~Mensagens: operador sem grupo (e gerente/chefe) enviam a qualquer papel de qualquer grupo~~ **CORRIGIDO na v1.5.4-E1**: ninguém envia fora do escopo do papel ativo (próprio grupo + subordinados ativos — `gruposSubordinadosAtivos`, vínculo bilateral); conta sem grupo não envia a ninguém (403); admin global; caixa admin continua destino alcançável e a resposta (`pai_id`) alcança o remetente da mensagem pai — matriz completa no §8 | mensagens.go (hMensagensEnviar); onda_v154_e1_test.go | ✅ |
| R-15 | ~~excluirArquivada deixa conferencia_escalas órfã (FK sem cascade)~~ **CORRIGIDO na v1.5.4-E2**: o DELETE da conferência arquivada (e o NUKE do grupo, R-6) apaga `conferencia_escalas` na mesma tx, por conferência e por usuário/designante. Regressão: `TestE2ExcluirArquivadaApagaEscala` | server_conferencia.go hConferenciaExcluirArquivada | ✅ |
| R-16 | ~~Troca de senha não invalida sessões; admin/admin sem expiração forçada~~ **CORRIGIDO na v1.5.4-E1**: troca própria (`/api/senha`) invalida as demais sessões e PRESERVA a corrente (decisão: quem trocou não cai do fluxo em uso — §3.C); redefinição por outrem (`/api/usuarios/{id}/senha`) derruba TODAS (`invalidarSessoesDeSenha`); admin semeado no 1º boot com a senha PADRÃO `admin` nasce `precisa_setup=1` — gate central do middleware auth recusa tudo fora de `/api/setup|/api/me|/api/logout` até a troca (seed com `SCI_ADMIN_SENHA` própria não bloqueia) | server_pessoal.go (hTrocarSenha/hUsuarioSenha); store.go (SeedIfEmpty); onda_v154_e1.go; onda_v154_e1_test.go | ✅ |
| R-17 | hUsuarioExcluir não limpa sessoes/papeis/chefe_setores (delete físico deixa órfãs) | server_pessoal.go:1342 | P2 |
| R-18 | hMoverConta não move usuario_papeis/sessões (rebaixamento cosmético). **AMPLIADO na v1.6.0 (F5):** o ramo admin do `hUsuarioEdit` (PATCH /api/usuarios/{id} com `grupo_id`, server_pessoal.go:426) faz `INSERT OR IGNORE INTO usuario_papeis (…, 'operador')` para QUALQUER alvo — para uma conta `sem_funcao` isso CRIA um contexto 'operador' (sem setor: bloqueado pelo `exigeSetorOperador`, mas aparece no dropdown e pode ser assumido no próximo login) | server_pessoal.go:1049-1132 e :420-431 | P2 |
| R-19 | Caixa da Função: exercente vê a mensagem mas leva 403 na thread | mensagens.go:1068-1072 | P2 |
| R-20 | Drive: sem cota, sem lixeira, físico fora do backup | drive.go (design) | P2 |
| R-21 | ~~Mural/Calendário: leituras sem checar grupo do objeto (detalhes/ciente/compartilhamentos)~~ **CORRIGIDO na v1.5.4-E2** (parcial): `hAvisosDetalhes`/`hAvisosCiente` passam pelo `avisoNoEscopo` (onda_v154_e2.go — admin global, grupo da sessão; 403 fora, 404 inexistente) e `GET /api/calendarios/{id}/compartilhamentos` usa a régua do compartilhar/revogar (autor ou gerente do grupo-dono). Resíduo FORA do escopo E2: `hAvisosComentar` ainda não checa o grupo do aviso. Regressão: `TestE2MuralAvisoForaDoEscopo`, `TestE2CalendarioCompartilhamentosEscopo` | mensagens.go hAvisosDetalhes/hAvisosCiente; calendario.go hCalendariosCompartilhamentosList | ✅ (resíduo comentários) |
| R-22 | gruposSuperioresAtivos não exige bilateral (herança sobe por vínculo meio-declarado) | server_grupos.go:45-69 | P2 |
| R-23 | Anti-ciclo de grupo_vinculos só imediato (A→B→C→A passa) | server_grupos.go:279 | P2 |
| R-24 | NUKE promete backup automático inexistente | server_grupos.go:142 | P2 |
| R-25 | GET /api/configuracoes público expõe WEBHOOK_ATRASOS_URL | server_admin.go:285-291 | P2 |
| R-26 | ~~Envelope {funcoes,total} do catálogo tratado como array em 2 views~~ **CORRIGIDO no remoto (fb32f18, 09/10)**: envelope exclusivo do catálogo de funções; demais catálogos voltam a devolver array | server_catalogo.go:328+; fix_catalogo_envelope_test.go | ✅ |
| R-27 | Duplicações de API (calendarios×calendario; relatorio-dia.pdf×/pdf; conferencia/lista×conferencias; POST material/conferencias morto) | ver módulos | P2 |
| R-28 | ~~Ci.sh não roda go test/vet~~; ~~dados.tar.gz com banco real na árvore~~; e2e CDP fora do repo — **CORRIGIDO na v1.5.4-F**: ci.sh gateia vet+build+test; dados.tar.gz movido para fora da árvore; e2e CDP fora do repo segue pendente para M8 | ci.sh; e2e_f2_pessoal.py:17 | P1 processo (parcial ✅) |
| R-29 | **GET /api/material/conferencias descarta conferências ABERTAS**: o Scan de `fechada_em` (string) falha com NULL e a linha é engolida no loop (`if rows.Scan(...) == nil`) — só conferências FECHADAS listam; o front do material hoje recarrega a lista para mostrar a aberta (depende de outro caminho). Provado no F7 (onda_v160_f7_test) | server_material.go:498 (hMaterialConferenciasList) | P2 |
| R-30 | **GET /api/conferencias/hoje (PLURAL) não existe**: não bate com `GET /api/conferencias` (lista) nem com `/api/conferencia/{id}` — cai no catch-all do SPA e devolve 200 com HTML. Testes antigos o usam como prova fraca (200 sem corpo JSON) — trocar por `/api/conferencia/hoje` | rotas server_conferencia.go:1779-1802; onda_f3_e2e_personas_test.go:112 | P2 (teste) |
| R-31 | **Bloco de criação do encarregado em views_pessoal.js é código morto**: `#encCriar` nunca é renderizado (a view não emite o botão); SE reativado, precisa enviar `setor_id` (F4 — o servidor recusa operador sem setor com 400) e tratar `sem_funcao` | web/views_pessoal.js:290 | P3 |
| R-32 | ~~**`ui_helpers.js` (código morto, nunca carregado) tem a 3ª cópia de `rotuloPapel`, DIVERGENTE**: não conhece `enc_pessoal`/`enc_material`/`sem_funcao` (mostraria "OPERADOR").~~ **ALINHADA na v1.6.0-contextos (checkpoint do comando)**: a 3ª cópia agora conhece `chefe_setor`/`encarregado`/`enc_pessoal`/`enc_material`/`sem_funcao` — mesmo vocabulário da cópia de core.js. Residual de higiene M8 (baixar o arquivo de vez ou apagar as cópias) segue aberto como tarefa de limpeza, não como divergência | web/ui_helpers.js:12 | P3 (higiene M8) | ✅ |
| R-33 | ~~**Furo: CONTEXTO enc_material lia a conferência inteira** (hoje/estado/lista/{id}/stream com `a.auth(false)` pelado — sem poder de escrita, lia o efetivo do grupo)~~ **FECHADO na v1.6.0-F6**: porta de papel `confLeituraAuth` (server_conferencia.go:1749) nas leituras — `papelConfAutorizado` + admin (leitura vazia); **enc_material 403 nem leitura**. Regressão: `onda_v160_f6_test.go` | server_conferencia.go | ✅ |
| R-34 | ~~**Furo: operador concluí/reabria/lia pré-fechamento de QUALQUER setor do grupo** (confMarcarAuth só cobra papel)~~ **FECHADO na v1.6.0-F7**: ramo do operador espelhando o do chefe em concluir (:702), reabrir (:780) e pré-fechamento (onda_0910:240) — só o setor de CADASTRO (`setorDoUsuario`); sem setor → 403 (mensagem única da onda). Regressão: `onda_v160_f7_test.go` | server_conferencia.go; onda_0910_conf_antiguidade.go | ✅ |
| R-35 | ~~**Furo: módulo Material sem recorte de setor para o operador** (authMaterial cobra papel e o operador via/editava/apagava/cautelava itens, cautelas, conferências e PDFs do grupo INTEIRO)~~ **FECHADO na v1.6.0-F7**: `setorEscopoMaterial`/`recortaSetorMaterial`/`setorNoCorte` (onda_v160_material_setor.go) aplicam o próprio setor em leituras/escritas/PDFs/conferência de material (Carga Geral — setor NULL — fora do recorte); iniciar força o setor; categorias: escrita ger/enc. Regressão: `onda_v160_f7_test.go` | server_material.go; onda_v160_material_setor.go | ✅ |
| R-36 | ~~**Furo: conferência de material nascia com o setor do CORPO da requisição** (operador informava setor alheio no iniciar/bipar)~~ **FECHADO na v1.6.0-F7**: o iniciar FORÇA o setor do operador (ignora o corpo, server_material.go:569) e bipagem/fechamento/PDF conferem o setor efetivo da conferência (:807/:835/:896/:962). Regressão: `onda_v160_f7_test.go` | server_material.go | ✅ |

---

*Fim do mapa. Atualize-o no mesmo commit de qualquer alteração que ele descreve.*
