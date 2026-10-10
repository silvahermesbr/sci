# SCI — MAPA DE ARQUITETURA (fonte de verdade operacional)

> **Gerado em 2026-10-10** a partir de auditoria completa do código (`main` @ `608e5ef`, schema v43).
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
  - `confAuth` = papéis {gerente, operador} + encarregado/auxiliar de pessoal; admin **403**.
  - `confMarcarAuth` = confAuth + chefe_setor.
  - `confPDFAuth` (server_conferencia.go, v1.5.4-D2) = papéis da conferência {gerente, operador,
    enc_pessoal/auxiliar}; `chefe_setor` SÓ se a conferência envolve setor que AINDA comanda
    (`conferenciaEnvolveSetorComandado`: `conferencia_setores` × `chefe_setores` — fonte única
    D-1); admin **403**. O recorte de escopo (exigeEscopo + grupo/subordinados) segue no handler.
  - `guardaGestaoPessoal` — admin, gerente, ou encarregado/auxiliar de pessoal designado.
  - `authMaterial` — gerente, operador, ou encarregado de material designado; admin **403**; 423 se `MODO_RESERVA=1`.
  - `reservaAuth` — {gerente, operador, chefe_setor}; admin 403; 423 se `MODO_RESERVA=1`.
- **Escopo** (`escopoDoUsuario`, server_grupos.go:202): **0** = admin (vê tudo) · **N** = grupo da
  sessão · **-1** = conta sem grupo (**deveria ver nada** — ver defeito R-1: handlers que só testam
  `esc > 0` deixam o `-1` no ramo sem filtro). O escopo vem do **papel ativo da sessão**
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
| **Papéis de sistema** | `usuario_papeis` (`admin\|gerente\|operador\|chefe_setor`) | Acesso base a módulos (papel ATIVO da sessão) | `escopoDoUsuario`, todas as guardas, `/api/me` |
| **Cadeiras (funções de grupo)** | `funcao_membros` × `funcoes` tipo='grupo' (chave `enc_pessoal`/`enc_material`, titular/auxiliar) | Poderes espelhados: enc_pessoal→fechar conferência, gerir pessoal; enc_material→módulo material | `podeGestaoPessoal`, `ehEncarregadoDePessoal/Material`, `authMaterial`, `gestorPessoal/Material` (front) |
| **Chefias de setor** | `chefe_setores` (1 linha por setor — `setor_id` UNIQUE, v35) | Comando do setor p/ conferência (concluir/reabrir/marcar) | `chefeComandaSetor` (**FONTE ÚNICA** — D-2 executada na v1.5.4-D1, ver R-12), `setorAtivoComandado` (onda_v154_d1.go), `hCatalogoList`, `hSetoresAgregado` |

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
- Designação de cadeira **não re-chaveia sessão**: `hFuncaoMembrosSet` só sincroniza
  `usuario_papeis.funcao_id` (display). O poder novo vale a partir do próximo `/api/me`.
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
ativo; NULL (designado puro) cai no `usuarios.*`. Conta **sem grupo** = escopo `-1`. O front espelha
via `/api/me` (`ME`, `funcoes_grupo[].chave`, `papeis[]`) — o dropdown de contexto grava
`POST /api/sessao/contexto` (guarda R-9: setor dentro do grupo do papel ativo; chefe só setor
comandado; admin livre).
**PROPOSTA D-5 (efeito já aplicado na v1.5.4-E1, decisão final pendente de comando):** o CONTEXTO
da sessão é `sessoes.setor_ativo_id` (sobrepõe o cadastro em `UsuarioDaSessao`); `usuarios.setor_id`
é CADASTRO/exibição — não é mais reescrito pela troca de contexto, e continuam lendo-o como
fallback de cadastro: `hMe`/`hLogin` (setor resolvido p/ o front), setor do OPERADOR em
`guardaSetorNaMarcar`/`setorDoUsuario` e os resíduos já anotados no D-5 (designação de escala do
chefe, `hSetorPreFechamento`). O COMANDO de chefia segue exclusivamente em `chefe_setores` (D-2).

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
4. Migrações: cadeia v2…v43 chamada à mão em `AbrirStore` (store.go:56-175) — **`ReabrirComArquivo`
   só executa até v12** (store.go:1372-1402) — defeito R-4; registro único `[]func()` planejado na v1.5.5.

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
⚠ = comportamento divergente entre front e servidor (ver registro de defeitos).

| Módulo (rotas front) | admin | gerente | operador | chefe_setor | enc_pessoal (designado) | enc_material (designado) | conta sem grupo (-1) |
|---|---|---|---|---|---|---|---|
| **Conferência** `#/hoje` `#/conferencia` | ✗ (leitura vazia) | ✓ total | ✓ lançar/iniciar; fechar ✗ | ✓ setor próprio (concluir/reabrir/marcar) | ✓ total **inclui fechar** | ✗ | ⚠ **vaza tudo** (R-1) |
| **Pessoal** `#/pessoal` | ✗ (só via admin p/ catálogo) | ✓ | ✗ | ✗ | ✓ (efetivo/funções/setores) | ✗ | ✗ (403 nas escritas; ⚠ leituras R-1) |
| **Material** `#/material` | ✗ | ✓ | ⚠ ✓ API (front esconde) | ✗ | ✗ | ✓ | ⚠ **vaza tudo** (R-1) |
| **Grupos/gerenciar** `#/grupos` | ✗ (área admin separada) | ✓ (próprio grupo) | ✗ | ✗ | ✗ | ✗ | ⚠ lista grupos (R-1) |
| **Admin/config** `#/admin` `#/configuracoes` | ✓ | ✗ | ✗ | ✗ | ✗ | ✗ | ✗ |
| **Relatórios** `#/relatorios` | ✓ global | ✓ árvore | ⚠ API sem guarda de papel | ✗ (front) | ✓ | ✗ | ⚠ **vaza tudo** (R-1) |
| **Mensagens** `#/mensagens` | ✓ (caixa própria) | ✓ | ⚠ front esconde; API ok (escopo R-14) | ✓ | ✓ | ✓ | ✗ não envia (R-14 corrigido) |
| **Drive** `#/drive` | ✗ (doutrina) | ✓ | ⚠ front esconde; API por ACL | ✓ | ✓ | ✓ | por ACL |
| **Mural** `#/avisos` | ✓ publica global | ✓ publica | lê + ciência + comentário | idem | idem | idem | lê se tiver grupo |
| **Escalas/Calendário/Consciência** | dormentes (rotas front bloqueadas; APIs vivas — ver §10) | | | | | | |

**Princípios da matriz:** (1) admin é papel de infraestrutura — **bloqueado** de dados operacionais
de grupo (exceções: relatórios globais, excluir arquivadas, NUKE); (2) escopo N vê próprio grupo e,
onde indicado, subordinados ativos (vínculo bilateral); (3) contas sem grupo (-1) não deveriam ver
nada — hoje vazam (R-1).

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

| Método | Rota | Handler (arq:linha) | Guarda | Papéis | Escopo 0/-1/N | W | R | Notas |
|---|---|---|---|---|---|---|---|---|
| GET | /api/conferencia/hoje | server_conferencia.go:16 | auth(false) | qualquer | 0+admin: vazio; ⚠-1: escalados de todos; N: conf aberta | DDL lazy CD | conferencias, presencas, CD, CF, CS, escalas | modo+funcoes_filtro; pessoas na escada de antiguidade unificada (v1.5.4-D3/R-7); corte por setor ativo/despachos — chefe só lê o setor que AINDA comanda (setorAtivoComandado, v1.5.4-D1/R-12; contexto órfão → pessoas vazias) |
| GET | /api/conferencia/estado | onda_0610_conf_estado.go:23 | auth(false) | qualquer | 0/-1: hash vazio; N: por setor | — | idem | SHA-1/setor; base do tick 2s |
| POST | /api/conferencia/iniciar | server_conferencia.go:434 | confMarcarAuth + internas :454-484 | ger/op*/chefe*/enc* | sem grupo 403 | conferencias, presencas(carry+escalas), CS, CF, CD | pessoas, escalas | valida funcao_ids do grupo ANTES (:498); auditoria; chefe só inicia setor que comanda (chefeComandaSetor — fonte única chefe_setores, D-1); modo antiguidade responde `sem_tag:[nomes]` (v1.5.4-D3/R-7) |
| POST | /api/conferencia/despachar | onda_despacho.go:160 | confAuth (+operador 403 interno) | ger/enc | sem grupo 403 | +CD | idem | exige ≥1 setor; modo antiguidade responde `sem_tag` com recorte aos setores despachados (v1.5.4-D3/R-7) |
| POST | /api/conferencia/fechar | server_conferencia.go:522 | confAuth + :524 | **só gerente/enc_pessoal** | ⚠-1 fecha de OUTRO grupo (R-1) | presencas upsert, conferencias, CS (tx) | conferencias | sem ✅→nao_verificado; **backupAssíncrono** + broadcast SSE |
| POST | /api/conferencia/marcar | server_conferencia.go:306 | confMarcarAuth + guardaSetorNaMarcar | ger/enc: grupo; chefe/op: setor próprio | -1: 409 | presencas, CS | pessoas | só justificada tem destino; troca reset obs; SSE; ramo chefe exige comando VIGENTE (setorAtivoComandado, v1.5.4-D1/R-12) |
| GET | /api/conferencia/lista · /api/conferencias | server_conferencia.go:818 (:1584/:1599) | auth(false) | qualquer | ⚠-1: **todas** (R-1) | — | conferencias | ?arq=1 arquivo; senão 7 dias |
| GET | /api/conferencia/{id} | server_conferencia.go:933 | auth(false) | qualquer | 0: qualquer; ⚠-1: qualquer (R-1); N: própria+subordinados | — | tudo | relatório na tela; modo antiguidade ordena pela escada (antiguidade unificada das 3 fontes, v1.5.4-D3/R-7); modo setores mantém situacao/nome |
| DELETE | /api/conferencia/{id} | server_conferencia.go:882 | confAuth | ger/op/enc | ⚠-1 (R-1) | comentarios, presencas, CE, conferencias | — | só ABERTA; sem backup |
| POST | /api/conferencia/{id}/setor/{sid}/concluir | server_conferencia.go:631 | confMarcarAuth | chefe: setor ativo COMANDADO (setorAtivoComandado, v1.5.4-D1/R-12); ger/op/enc | ⚠-1 (R-1) | CS | — | exige aberta |
| POST | /api/conferencia/{id}/setor/{sid}/reabrir | server_conferencia.go:689 | idem | idem | ⚠-1 (R-1) | CS | — | |
| GET | …/setor/{sid}/pre_fechamento | onda_0910:138 | confMarcarAuth | idem | ⚠-1 (R-1) | — | pessoas+CF | ordena pela antiguidade UNIFICADA das 3 fontes da tag (v1.5.4-D3/R-7); não-marcados saem como nao_verificado (COALESCE situacao — antes a linha NULL era descartada no Scan); ramo chefe ainda lê o CONTEXTO sem conferir comando (resíduo D-5) |
| GET | /api/conferencia/funcoes-antiguidade | onda_0910:296 | auth(false) | qualquer | ≤0: vazio+aviso | — | funcoes | picker; tags DO grupo |
| GET | /api/conferencia/{id}/relatorio.pdf | server_conferencia.go:1141 | confPDFAuth + escopo interno :1181 | ger/op/enc/aux; **chefe só c/ setor envolvido COMANDADO** (conferenciaEnvolveSetorComandado); admin 403 (v1.5.4-D2/R-2) | -1: 403; N: própria+subordinados | — | tudo | só FECHADA; filtros; assinatura; modo antiguidade sai na escada (mesma ordem do relatório em tela, v1.5.4-D3/R-7) |
| POST | /api/conferencia/{id}/arquivar | server_conferencia.go:741 | confAuth | ⚠ doc diz gerente; op/enc passam (R-8) | ⚠-1 (R-1) | conferencias | — | só FECHADA; backup |
| DELETE | /api/conferencia/arquivada/{id} | server_conferencia.go:776 | **auth(true)** | admin | — | comentarios, presencas, conferencias | — | ⚠ CE fica órfã (R-15) |
| POST | /api/conferencia/{id}/escala | onda_0510_escalas.go:88 | auth(false)+guardaEscala | ger(chefe/op do grupo); chefe(só op do setor) | gid==esc | CE | usuarios | escala de guarda da conferência |
| DELETE | …/escala/{usuario_id} | onda_0510_escalas.go:140 | idem | idem | idem | CE | — | |
| GET | /api/conferencia/{id}/stream | server_realtime.go:68 | auth(false) | qualquer | ⚠-1 (R-1) | — (hub mem) | conferencias | SSE; sem consumidor front |
| POST | /api/comentarios | server_conferencia.go:1422 | confAuth | ger/op/enc | ⚠-1 (R-1) | comentarios | tags | tag validada grupo+superiores |
| GET | /api/comentarios/{id} · /api/pessoas/{id}/comentarios | :1481 · server_pessoal.go:424 | confAuth | idem | ⚠-1 (R-1) | — | comentarios | histórico |

**Se você alterar este módulo, verifique também:** enum de situações (muda % presença em TODOS os
relatórios/PDFs — `montarBundle` server_conferencia.go:1203 + server_relatorios.go); `predicadoAntiguidade`
replicado em hoje/estado/pre_fechamento + `pessoasAtivasOpt` (contrato do **hash** do /estado consumido
pelo tick 2s e pelo `ops/carga_conferencia.py`); escalas (badges/pré-fill do iniciar); catálogos
(funções/destinos auto 'Serviço de Escala'/tags); carry-over (colunas de presencas); grupo_vinculos
(herança {id}GET/PDF); SSE+backupAssíncrono (concorrencia_test); migrações v40/v43 e a tabela lazy CD.

**Testes:** conferencia_realtime_test, setores_conferencia_test, v1_5_conferencia_setor_test,
onda_despacho_test, onda_0910_conf_antiguidade_test(+_grupo), onda_v154_d3_test, onda_0510_conf_escopo_test,
onda_0510_escalas_test, contexto_govema_conferencia_test, onda_multichefe_test, multi_setor_chefe_test,
regressao_conferencia_destino_test, concorrencia_test, ordem_diretor_conferencia/0410/pdf_v2, v1_test,
onda_0610_pessoal_test, onda_f3_e2e_personas_test, sci_ordem_0610_frented_test, onda_v154_d2_test.

---

## 6. PESSOAL/USUÁRIOS

Arquivos: `server_pessoal.go`, `onda_0610_pessoal.go`, `onda_0510_c2.go`, `auth.go`, `helpers.go`,
sessões em `store.go`. Tabelas: `usuarios`, `pessoas`, `usuario_papeis`, `funcoes`, `funcao_membros`,
`sessoes`, `pessoas_apresentacao` (lazy), `auditoria`.

Guardas: `guardaGestaoPessoal` = admin+gerente sempre; enc/auxiliar de pessoal (designado na cadeira
`chave='enc_pessoal'`, **qualquer papel** — v367) restrito ao próprio grupo. `podeGestaoPessoal` =
mesma regra em função. Contas: admin cria qualquer (default **sem grupo**); gerente cria chefe_setor;
chefe promove operador do seu setor; enc cria operador/chefe do grupo. Senha "sci"/vazia → `precisa_setup=1`
(bloqueia tudo exceto setup/me/logout). **v1.5.4-E1:** admin semeado com a senha PADRÃO `admin`
no 1º boot também nasce `precisa_setup=1` (troca obrigatória; seed com senha própria não bloqueia).

| Método | Rota | Handler (server_pessoal.go:linha) | Guarda | Papéis | Escopo | W | R | Notas |
|---|---|---|---|---|---|---|---|---|
| POST | /api/login | hLogin :15 (rota :1533) | pública + rate-limit | — | — | sessoes | usuarios | cookie Strict; setores_chefiados |
| POST | /api/setup | hAuthSetup :70 | auth+precisa_setup | conta em setup | — | usuarios | — | troca login+senha |
| GET | /api/me | hMe :110 | auth(false) | qualquer | — | — | várias | contrato do front (ME, funcoes_grupo) |
| POST | /api/logout | hLogout :139 | auth(false) | — | — | sessoes | — | |
| POST | /api/senha | hTrocarSenha :147 | auth(false) | própria | — | usuarios, sessoes | — | **R-16 CORRIGIDO (E1):** invalida as demais sessões da conta; a CORRENTE é preservada (troca pela própria conta — decisão §3.C) |
| POST | /api/sessao/contexto | mensagens.go:17 | papel próprio | qualquer | — | sessoes | — | **R-9 CORRIGIDO (E1):** setor precisa pertencer ao grupo do papel ativo; chefe_setor só setor COMANDADO (`chefeComandaSetorNoGrupo`); admin livre; `usuarios.setor_id` NÃO é mais reescrito (contexto = sessoes.setor_ativo_id, D-5) |
| POST/DELETE | /api/usuarios/{id}/papeis[/{pid}] | mensagens.go:84/:256 | allowlist `podeGerirPapelAlvo` (E1) | admin (global); ger (operador/chefe na própria árvore); enc/aux de pessoal (operador/chefe do próprio grupo); **fail-closed** p/ o resto | 0/N | usuario_papeis, sessoes, chefe_setores | — | **R-10 CORRIGIDO (E1):** fim do fail-open do Del (chefe/sem-papel barrados; alvo resolvido ANTES da decisão; re-chaveio de sessões do alvo só pós-autorização); trava do único papel mantida; **D-1/R-12**: Add de chefe_setor+setor_id MATERIALIZA o comando (UPSERT 1:1); Del do papel apaga os comandos do grupo |
| GET | /api/pessoas/{id}/ficha | hPessoaFicha :391 | auth(false) | qualquer | ⚠-1 vaza (R-1) | — | pessoas… | PII |
| GET | /api/pessoas | hPessoasList :587 | auth(false) | qualquer | ⚠-1 vê tudo (R-1) | — | pessoas, auditoria | GROUP BY auditoria full-scan |
| GET | /api/pessoas/apresentacao | onda_presenca_banco.go:21 | auth(false) | qualquer | ⚠-1 (R-1) | (ensure) | lazy table | |
| POST/PATCH/DELETE | /api/pessoas[/{id}] | :685/:759/:642 | guardaGestaoPessoal | adm/ger/enc | 0/N | pessoas | funcoes | grupo forçado; c/ presenças→inativo |
| POST | /api/pessoas/{id}/apresentacao | onda_presenca_banco.go:68 | guarda | idem | 0/N | lazy | | upsert |
| GET | /api/pessoas/{id}/modificacoes | onda_presenca_banco.go:145 | auth(false) | qualquer | -1 403 (ok) | — | auditoria | últimos 30 |
| GET | /api/pessoas/{id}/qr · /pdf | :1350 · :468 | auth(false) | qualquer | ⚠-1 vaza (R-1) | — | tudo | ficha PII completa |
| GET | /api/usuarios | hUsuariosList :877 | auth(false) | c/ grupo | 0/N(+subord ger); -1 403 | — | usuarios… | col. `senhas` adm/ger/chefe |
| POST | /api/usuarios | hUsuariosAdd :1134 | interna | ver regras | 0/N | usuarios, papeis | — | default sem grupo |
| PATCH | /api/usuarios/{id} | hUsuarioEdit :239 | guarda | adm/ger/enc | 0/N | usuarios, pessoas | — | espelha pessoa |
| DELETE | /api/usuarios/{id} | hUsuarioExcluir :1279 | interna | adm;ger | 0/N | usuarios | — | FK→desativa; ⚠ órfãs sessões/papéis (R-17) |
| POST | /api/usuarios/{id}/senha | hUsuarioSenha :183 | interna | adm;ger | 0/N | usuarios, sessoes | — | enc 403; **R-16 (E1):** invalida TODAS as sessões da conta afetada |
| GET | /api/usuarios/{id}/foto | hUsuarioFotoGet :1015 | auth(false) | qualquer | ⚠ **sem escopo** (R-5) | — | usuarios | cache public 3600 |
| GET/PATCH | /api/perfil | :954/:978 | auth(false) | própria | — | usuarios, pessoas | — | ⚠ foto_base64 sem validação (R-3) |
| PATCH | /api/usuarios/{id}/mover | hMoverConta :1049 | interna | adm;ger(árvore) | 0/N | usuarios | — | ⚠ não move usuario_papeis/sessões (R-18) |
| GET/POST/DELETE | /api/grupo/funcoes/membros[/{id}] | onda_0510_c2.go:29/99/194 | interna | GET: c/ grupo; SET/DEL: ger/adm; enc**só cadeira enc_material** | 0/N | funcao_membros | funcoes | onda 10/10; devolve `chave` |

**Se você alterar este módulo, verifique também:** contrato `/api/me` (core.js `definirUsuario`,
`rotaInicial`, `gestorPessoal/Material`, `ehEncPessoal`); guardas derivadas (confAuth/authMaterial
leem as mesmas cadeiras); conferência (`hPessoaComentarios` roteada lá); grupos (trocar-gerente,
NUKE); mensagens (destinatários por papel; caixa da função); escalas/material (designações); tests
onda_0610_pessoal, onda_1010_gap1, fix_encarregado(±v367), onda_f2/f3, onda_r1/r2/r3,
multi_papel_e_mensagens, guardas_auth, perfil_e_grupos, **onda_v154_e1 (R-9/R-10/R-16)**.

---

## 7. MATERIAL

Arquivo: `server_material.go` (+PDFs `relatorio.go`, watchdog `server_catalogo.go`, schema store.go
v14/16/27/28/29/36/41). Tabelas: `material_categorias/itens/cautelas/cautela_anexos/tipos/classes/
viaturas/item_anexos/item_comentarios/conferencias/conferencia_itens`, `grupo_setor_responsaveis`.

`authMaterial` (server_material.go:2150): MODO_RESERVA→423; sessão; papel ∈ {gerente, operador} OU
enc_material designado → passa; **admin 403**. Ciclo do item: `disponivel→acautelado→(devolução)→…
manutencao/baixado` (CHECK store.go:795); sensibilidade `controlado` força qtd=1+patrimônio;
UNIQUE(grupo, patrimônio); devolução parcial splita linha; **1 conferência aberta/grupo/setor/dia**
(índice parcial v41); watchdog SLA 30min→webhook; anexos base64 ≤800KB com allowlist MIME+
`attachment`+nosniff (o modelo correto — replicar no Drive).

| Método | Rota | Handler (sm:linha) | Guarda | Escopo | W | Notas |
|---|---|---|---|---|---|---|
| GET/POST/DELETE | /api/material/categorias[/{id}] | :967/:994/:1045 | authMaterial | leitura ok; escrita ⚠-1 vaza (R-1); global só admin | material_categorias | em uso→desativa |
| GET/POST/DELETE | /api/material/itens[/{id}] | :1085/:1213/:1399 | authMaterial | ⚠-1 vaza (R-1) | itens, viaturas | ?status/?categoria/?garagem; exclusão atômica tx |
| GET | /api/material/itens/{id}/qr | :2035 | authMaterial | ⚠-1 | — | `sci://m:{id}:{pat}` |
| GET | /api/material/etiquetas-lote.pdf | :2074 | authMaterial | ⚠-1 | — | 10/folha A4 |
| GET | /api/material/inventario/pdf | :892 | **authMaterial** (v1.5.4-D2/R-2) | exigeEscopo; -1: 403 | — | reservaAtivo→423 |
| POST | /api/material/cautelar · devolver | :1484 · :1623 | authMaterial | ⚠-1 (R-1) | cautelas, itens | tx; saldo; parcial split |
| GET | /api/material/cautelas | :1732 | authMaterial | ⚠-1 (R-1) | — | LIMIT 200 |
| GET | /api/material/cautelas/{id}/recibo.pdf | :828 | **authMaterial** (v1.5.4-D2/R-2) | exigeEscopo + cautela no escopo; -1: 403 | — | 2 vias; auditoria |
| POST/GET | …/cautelas/{id}/anexos · /api/material/anexos/{id} (GET/DEL) | :1819/:1902/:1946/:2004 | authMaterial+cautelaNoEscopo | ⚠-1 (R-1) | cautela_anexos | allowlist MIME |
| GET/POST | /api/material/responsaveis | :30/:73 | authMaterial (+papel p/ POST) | POST: ⚠ gerente sem grupo define em qualquer grupo (R-1) | grupo_setor_responsaveis | enc material por setor |
| GET/POST | /api/material/itens/{id}/anexos · item-anexos/{aid} (GET/DEL) · itens/{id}/comentarios (GET/POST) | :133/:178/:244/:281/:313/:360 | authMaterial | ⚠-1 (R-1) | item_anexos/comentários | |
| GET/POST | /api/material/conferencias · /iniciar · /{id} · /bipar · /fechar | :400/:438/:540/:634/:702 | authMaterial | ⚠-1 (R-1) | mat_conf(+itens) | POST /conferencias é rota morta (lista) |
| GET | /api/material/conferencias/{id}/pronto.pdf | :731 | **authMaterial** (v1.5.4-D2/R-2 — antes auth(false) sem guard) | exigeEscopo + conf. do próprio grupo; -1: 403 | — | reservaAtivo→423 |

**Se você alterar, verifique também:** os 4 geradores de PDF (relatorio.go:861/1095/1231/1709);
watchdog+sino (`server_catalogo.go:560`, `server_admin.go:350`); ficha pessoal (cautelas ativas,
server_pessoal.go:527); NUKE (⚠ não apaga material_conferencias → FK aborta, R-6); etiquetas
(material_tipos/classes); front views_material.js + gates core.js:1147; tests onda_material_*,
v1_5_material_*, v15_cia_fixes, fix_encarregado(±v367), regressao_escalas_material, onda_v154_d2_test.

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
| gerente / chefe_setor / operador / enc_pessoal **com grupo** | papéis do PRÓPRIO grupo + de grupos SUBORDINADOS ativos (`gruposSubordinadosAtivos`, vínculo bilateral) + caixa **admin** (destino global de infraestrutura) |
| resposta a despacho (`pai_id`) | + o REMETENTE da mensagem pai (fora da árvore também — a thread é participativa) |
| conta SEM grupo (qualquer papel ≠ admin) | **ninguém — 403** (antes: operador sem grupo e gerente/chefe enviavam a qualquer grupo) |

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
(drive.go:1075-1091): itens/da_funcao/seletor(⚠ morto)/pastas CRUD/upload/download(+`inline` ⚠
R-11)/mover/copiar/propriedades/arquivo PATCH/DEL/compartilhar CRUD/arquivo_grupo(gerente).
`/api/drive/seletor` lista sem passar pela ACL e não tem consumidor — remover ou blindar.

**MURAL DE AVISOS** — leitura: admin global ou qualquer papel com grupo (⚠ filtro tautológico
mensagens.go:1400); escrita: gerente/admin; ciência+comentário: quem vê (⚠ sem checar grupo do aviso
→ IDOR de leitura/ciência R-21); excluir: autor ou gerente do grupo (admin não-autor 403 — assimetria);
repostar: só gerente (origem no escopo). **Publicar aviso dispara**: 1 mensagem de notificação POR
papel do grupo + **síntese de `usuario_papeis` para contas que nunca logaram** (mensagens.go:1570-1630,
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
bilateral — assimetria R-22; anti-ciclo só imediato R-23). `escopoDoUsuario` :202. nomear/destituir
chefe: gerente ou enc_pessoal, `gid==escopo` (admin 403); substituição 1:1 via `setor_id` UNIQUE;
multi-chefia por usuário. trocar-gerente: admin; rebaixa anterior e **migra grupo_id do alvo**.
excluir grupo: admin (auth(true)); vazio/`forcar`/`nuke` (senha do admin no corpo) — ⚠ NUKE não cobre
`chefe_setores/funcao_membros/avisos/material_conferencias/escala_modelos/setor_sugestoes/grupo_setor_responsaveis`
→ FK aborta (R-6); comentário promete backup automático que não existe (R-24). `/api/grupos` e
`/api/vinculos` auth(false) — ⚠ -1 lista todos (R-1).

**ADMIN** (server_admin.go): backup `VACUUM INTO`+sha256+MANIFEST (mutex; dispara boot/fechamentos/
manual/CLI `sci backup`); **import com 3 bugs verificados** (R-4: sem rename `.novo`→`sci.db`;
reabre migrando só até v12; rollback reabre o próprio import). `GET /api/configuracoes` **público**
com allowlist (⚠ inclui WEBHOOK_ATRASOS_URL — R-25); POST só admin. Métricas admin-only. health público.

**CATÁLOGOS & SETORES** (server_catalogo.go, rotas :1003-1016): allowlist `tabelaDeCatalogo`
(setores/funcoes/destinos/tags/conferencia_tipos/status_pessoal/material_tipos/material_classes).
Leitura auth(false) com herança (próprio+superiores+subordinados); funções só `tipo='antiguidade'`
(v368; cadeiras tipo='grupo' hardcoded v39). Escrita `guardaGestaoPessoal`; editar/excluir dono ou
subordinado; reparentar só dono (anti-ciclo completo, prof ≤8). `hSetorExcluir` (ordem_0610_setores.go):
gerente(árvore)/enc(próprio) — remaneja pessoas/contas p/ SEM SETOR em tx; ⚠ não limpa
`chefe_setores`/`grupo_setor_responsaveis` (FK, R-6). `hOperadoresDoSetor`: chefe designa operador do
seu setor (valida grupo+setor). `setor_sugestoes`: workflow sugere→avalia (ger/chefe/admin) com
allowlist de efeito revalidada na aplicação (tx, TOCTOU guard) — o padrão-ouro de escrita indireta.
`hSetoresAgregado` lê chefe/qtd_chefes de `chefe_setores` desde a v1.5.4-D1 (antes inferia por
`usuario_papeis`+`usuarios.setor_id` — fonte divergente da UI de catálogo, R-12 corrigido). Envelope
`{funcoes,total}` é **exclusivo do catálogo de funções** (correção 09/10,
`fb32f18`); demais catálogos devolvem array cru — manter esse contrato em qualquer mudança do
`hCatalogoList` (regressão cravada em `fix_catalogo_envelope_test.go`).

**RELATÓRIOS & EXPORT** (server_relatorios.go :414-424 + relatorio.go): todos auth(false); recorte
por `escopoRelatorio` (?grupo= com 403 fora da árvore); ⚠ família inteira trata `-1` como global
(R-1; exceção `hEfetivoAtual`). Rotas: registros, tags, efetivo_atual, presenca/periodo, relatorio
(JSON/PDF/detalhado.pdf), export CSV (`/api/export/{t}`), export SQLite/JSON (`/api/export`).
PDFs gerados (relatorio.go): relatório geral :305 · detalhado :2166/:2128 · conferência :547 ·
ficha pessoal :677 · recibo cautela :861 · escalas :1029 · escala do dia :1846 · inventário :1095 ·
pronto material :1231 · etiquetas :1709. Fuso America/Sao_Paulo em tudo (fmtDataBR); ordenação
v9.17 = hierarquia de setor → função → nome. ⚠ rótulo "antiguidade = ID menor" desatualizado (:474).

**Se você alterar, verifique também:** `escopoDoUsuario`/`filtroArvore` (todos os callers de
relatórios e conferência); `grupo_vinculos` (árvore, herança de catálogo, EhSubordinado duplicado);
`chefe_setores` (catálogo, conferência, agregado — FONTE ÚNICA desde a v1.5.4-D1, R-12 corrigido); `funcao_membros`
(poderes espelhados em 6 guardas + front); backup (ReabrirComArquivo, MANIFEST, ci.sh); NUKE × FKs;
views_gestao/pessoal/config + core.js (config pública pré-login); tests perfil_e_grupos,
subordinacao_e_setores, ordem_diretor_hierarquia, api_backup, admin_restricao, v368, sci_ordem_0610_frented.

---

## 10. ESCALAS · CALENDÁRIO (dormentes)

Backend 100% registrado; rotas front bloqueadas em core.js:1106 (`ViewModuloEmDesenvolvimento`).
`web/views_escalas.js` (1453 ln) e `views_calendario.js` (900 ln) embutidos e inalcançáveis.

**ESCALAS** — `reservaAuth` nas 21 rotas (v1.5.4-D2/R-2: `/api/escalas/pdf`,
`/api/escalas/relatorio-dia.pdf` **+** `/pdf` e `/api/escalas/minhas` saíram do `a.auth(false)`
pelado e hoje exigem o MESMO guard do módulo): {ger/op/chefe}, admin 403, 423 se MODO_RESERVA.
`escalas/minhas` também barra conta sem grupo (prólogo `exigeEscopo`); a ABERTURA de `minhas` a
todos os papéis quando o módulo reativar segue **PENDENTE de decisão (D-3)** — não decidida nesta
onda. As rotas de PDF de escalas NÃO aceitam parâmetro de grupo: o recorte é sempre o escopo da
SESSÃO (outro papel com grupo lê o PDF do PRÓPRIO grupo — o teste D2 prova a não-vazamento pelo
texto do PDF).
Fases `aberto→preenchido→aprovado→publicado` (**sem máquina de estados** — qualquer→qualquer, em
massa por grupo+data); delegação inter-grupos (subordinados transitivos); modelos
(escala_modelos/postos/aptos com faixa de posto/graduação); InfoDescanso (conflito bloqueante,
folga <24h crítico…). ⚠ classe -1 em ~8 rotas (pior: `limpar-dia` sem grupo_id **apaga TODOS os
grupos**, server_escalas.go:1109-1112 — R-1). **Integração com conferência (reativação muda
comportamento!)**: `criarConferenciaBase` pré-preenche "justificada/Serviço de Escala" para escalados
na data; `escaladosNaData` alimenta badges do hoje (⚠ `escalados_ontem` entregue e nunca renderizado).

**CALENDÁRIO** — 11 rotas auth(false) com 403 admin interno; ACL `checarAcessoEvento`
(autor→gerente→compartilhamento→mesmo grupo). **Duas gerações de API vivas** sobre a mesma tabela
`calendario_compartilhamentos`: `/api/calendarios*` (coleções, v1.3) e `/api/calendario/*` (mesh,
v1.2) — unificar ANTES de reativar (R-27). ⚠ `GET /api/calendarios/{id}/compartilhamentos` sem
checar posse (R-21); `GET /api/calendarios` cria calendário 'Pessoal' no 1º acesso (write-on-GET);
índices únicos anti-dup só para alvo_usuario/grupo de coleções (não evento/papel). Reativação do
calendário sem escalas mostra a seção de escalas sempre vazia.

**Reativação (checklist):** guardas revalidadas contra a matriz §4; rotas desbloqueadas no
`rotear()` + menu; e2e persona re-executado; carga sem regressão; decisões D-3 (roadmap).

---

## 11. Mapa do frontend

`web/index.html` (shell, 4 refs com `?v=NNN`) → `lazy.js` (mapa hash→arquivo + CACHEBUST) → views.
Router `rotear()` (core.js:1088-1179): portões por papel **cosméticos** (§4 é a verdade). Três
tabelas de verdade no core.js que mudam JUNTAS em qualquer doutrina de papel: composição do menu
(`montarShell` :645-747), portões do `rotear()` (:1099-1173), `rotaInicial()` (:91-99).

| Rota | View (arquivo) | APIs principais | Portão cliente |
|---|---|---|---|
| #/hoje · #/conferencia | views_conf.js | conferencia/*, /estado (tick 2s), comentarios, escalas | ger/op/chefe/ehEncPessoal |
| #/pessoal | views_pessoal.js | pessoas, usuarios, funcoes/membros, nomear_chefe, setores | ger/gestorPessoal |
| #/material | views_material.js | material/*, notificacoes | ger/gestorMaterial |
| #/grupos | views_gestao.js (ViewGrupos) | usuarios, grupos, vinculos, backup | gerente |
| #/admin · #/configuracoes | views_gestao.js (ViewAdmin) / views_config.js | backup*, configuracoes, metricas | admin |
| #/mensagens · #/despachos | views_mensagens.js | mensagens/*, drive/download | não-operador |
| #/drive | views_drive.js | drive/* | não-operador/não-admin |
| #/avisos | views_avisos.js | avisos/* | todos |
| #/relatorios | views_conf.js (secção) | relatorio*, registros, tags, export | ger/gestorPessoal |
| #/perfil | views_gestao.js (ViewPerfil) | perfil | todos |
| #/escalas · #/calendario · #/consciencia | — | — | bloqueados (módulos em desenvolvimento) |

 Helpers vivos: `core.js` (api/esc/toast/modais/paginarArray), `views_gestao.js` re-exporta
`tabelaControles`…; ⚠ `ui_helpers.js` (1307 ln) **nunca é carregado** (código morto — decidir destino);
⚠ 3 cópias de `esc()` divergentes (views_avisos sem apóstrofo/fuso).

---

## 12. Mapa de dados

~48 tabelas + views, 69 índices, schema **v43** (const `versaoSchemaBinario`, main.go:21).
Migrações: cadeia manual v2…v43 em `AbrirStore`; 2 tabelas **lazy** fora do versionamento
(`conferencia_despachos`, `pessoas_apresentacao` — `sync.Once` sem retry). Pragmas: §3.E.

| Domínio | Tabelas | Observações |
|---|---|---|
| Núcleo | grupos, grupo_vinculos, usuarios, pessoas, usuario_papeis, sessoes, auditoria, configuracoes, schema_migrations | sessões guardam SHA-256; auditoria append-only sem índice (entidade,registro_id) |
| Conferência | conferencias, presencas, comentarios, tags, destinos, conferencia_setores, conferencia_despachos (lazy), conferencia_funcoes, conferencia_escalas, conferencia_tipos | UNIQUE(conferencia,pessoa); imutabilidade pós-fechamento |
| Organização | setores, funcoes (antiguidade+grupo), funcao_membros, chefe_setores, grupo_setor_responsaveis, setor_sugestoes, status_pessoal | 3+1 camadas de designação (§3.A) |
| Escalas | escala_tipos, turnos, pessoas, modelos, modelo_postos, modelo_aptos | dormente |
| Material | material_categorias/itens/cautelas/cautela_anexos/tipos/classes/viaturas/item_anexos/item_comentarios/conferencias/conferencia_itens | UNIQUE(grupo,patrimônio); índice parcial 1 conf aberta; blobs base64 no banco |
| Mensagens/Drive/Mural | mensagens, mensagem_destinatarios, mensagem_respostas, mensagem_pastas, drive_pastas/arquivos/compartilhamentos, avisos, aviso_cientes, aviso_comentarios | drive: bytes em DISCO `dados/drive/` (fora do backup!) |
| Calendário | calendarios, calendario_eventos, calendario_compartilhamentos | dormente; 2 gerações de API |

---

## 13. Checklists de impacto global

**Alterar papel/permissão de um módulo:**
1. Middleware/guarda do módulo (§5-§10) + prólogos internos dos handlers.
2. As 3 tabelas do core.js (menu, rotear, rotaInicial) + helpers espelhados (gestorPessoal/Material,
   ehEncPessoal — core.js:62 usa ⚠ fallback por NOME).
3. Matriz §4 deste doc + testes persona do módulo.
4. Se toca cadeiras (funcao_membros): 6 guardas servidor + /api/me (funcoes_grupo) + dropdown contexto.

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
| R-1 | **Escopo -1 (sem grupo) cai no ramo sem filtro** — ~35 handlers (conferência lista/fechar/arquivar/{id}/pdf/comentários/stream; pessoas ficha/pdf/qr/lista; material quase tudo; relatórios/export inteiros; /api/grupos; escalas limpar-dia apaga TODOS) | padrão `if esc > 0` — ver coluna Escopo das tabelas | P0 |
| R-2 | ~~Rotas de dados `a.auth(false)` sem guarda de papel/escopo (escalas pdf/minhas/relatorio-dia; material pronto.pdf **sem escopo algum**)~~ **CORRIGIDO na v1.5.4-D2**: as 4 rotas de escalas (`/api/escalas/pdf`, `/api/escalas/relatorio-dia.pdf` + duplicata `/relatorio-dia/pdf`, `/api/escalas/minhas`) exigem `reservaAuth` (`minhas` também barra -1 via `exigeEscopo` — abertura ampla pende da D-3/M5); os 3 PDFs de material (`inventario/pdf`, `cautelas/{id}/recibo.pdf`, `conferencias/{id}/pronto.pdf`) exigem `authMaterial`; `/api/conferencia/{id}/relatorio.pdf` exige `confPDFAuth` (papéis do módulo; chefe_setor só com setor envolvido COMANDADO — `conferenciaEnvolveSetorComandado`; admin 403). Doutrina nova: **nenhuma rota de dados sem guard declarado na tabela** (§3.I). Regressão: `onda_v154_d2_test.go` (matriz por persona: gerente/operador do próprio grupo 200; sem-grupo 403 em todas; admin 403; chefe só com setor comandado; operador de outro grupo 403 nos objetos alheios e PDF do próprio grupo sem vazamento) | server_escalas.go:1796-1816; server_material.go:2316-2346; server_conferencia.go:1635-1673 (confPDFAuth + rota :1706) | ✅ |
| R-3 | **XSS armazenado via foto_base64** (gravação sem validação + sinks sem esc + CSP unsafe-inline) | server_pessoal.go:343/:994; core.js:580; views_gestao.js:548/2535 | P0 |
| R-4 | **Import de backup quebrado** (sem rename .novo→sci.db; reopen só migra até v12; rollback reabre o import) | server_admin.go:259-274; store.go:1372-1402 | P0 |
| R-5 | Foto de usuário sem escopo (LGPD) | server_pessoal.go:1015 | P1 |
| R-6 | **NUKE/setor-excluir/excluirArquivada × FKs NO ACTION** (chefe_setores, funcao_membros, avisos, material_conferencias, escala_modelos, setor_sugestoes, CE órfãs…) | server_grupos.go:144-186; ordem_0610_setores.go; server_conferencia.go:799 | P1 |
| R-7 | ~~Antiguidade: ordenação ignora fontes u2/up2 e o relatório na tela não ordena por antiguidade; militar sem tag some em silêncio~~ **CORRIGIDO na v1.5.4-D3**: fonte única das expressões SQL em `onda_0910_conf_antiguidade.go` — `filtroAntiguidadeTresFontes` (o predicado do filtro) e `exprAntiguidadeTresFontes` = `COALESCE(fu.antiguidade, fu_u.antiguidade, fu_up.antiguidade, 999)` (precedência pessoa → conta → papel; sem tag = 999, por último), aplicada via `ordemAntiguidadeTresFontes` na listagem do `/hoje` (`pessoasAtivasOpt` com filtro), no pré-fechamento, no relatório em tela `/{id}` e no PDF (`montarLancamentosPDFConferencia`), condicionada ao modo antiguidade (`conferenciaEmModoAntiguidade` — modo setores mantém as ordens legadas); `iniciar`/`despachar` respondem `sem_tag:[nomes]` (`militaresSemTagAntiguidade` — ativos do universo, recorte = setores despachados, fora do filtro por não terem a tag em NENHUMA fonte; `NOT COALESCE(predicado,0)` p/ não perder o sem-tag na lógica tri-estados); bônus da mesma consulta: pré-fechamento voltou a listar os NÃO-marcados (`COALESCE(pr.situacao,'')` — o Scan descartava a linha NULL e o checklist nascia vazio); herança de catálogo segue SEM herança entre grupos (by design 09/10; D-1 pendente). Front do `sem_tag` ainda não consome (pendente) | onda_0910_conf_antiguidade.go; helpers.go (pessoasAtivasOpt); server_conferencia.go (hConferenciaGet/montarLancamentosPDFConferencia/hConferenciaIniciar); onda_despacho.go; onda_v154_d3_test.go | ✅ |
| R-8 | Arquivar/descartar: doc diz gerente, código aceita operador/enc | server_conferencia.go:741/:882 | P1 |
| R-9 | ~~hMudarContexto aceita setor de qualquer grupo e reescreve usuarios.setor_id global~~ **CORRIGIDO na v1.5.4-E1**: o setor precisa pertencer ao GRUPO do papel ativo; `chefe_setor` só assume setor que AINDA comanda (`chefeComandaSetorNoGrupo` — fonte única `chefe_setores`, D-2); admin é global; o `UPDATE usuarios SET setor_id` global foi REMOVIDO — o contexto vive em `sessoes.setor_ativo_id` (sobrescrito em `UsuarioDaSessao`) e `usuarios.setor_id` fica como cadastro (proposta D-5, decisão pendente) | mensagens.go (hMudarContexto); onda_v154_e1.go; onda_v154_e1_test.go | ✅ |
| R-10 | ~~hUsuarioPapelDel fail-open (chefe remove papéis de qualquer um + re-chaveia sessões)~~ **CORRIGIDO na v1.5.4-E1**: allowlist de solicitantes fail-closed (`podeGerirPapelAlvo` — admin global; gerente operador/chefe_setor no próprio grupo/árvore; enc/aux de pessoal no próprio grupo, doutrina hUsuariosAdd/v367) aplicada no Del E no Add (o Add também deixava passar papel-base não reconhecido com poder de admin); alvo resolvido ANTES de qualquer decisão e re-chaveio de sessões do alvo só pós-autorização; trava "não remover o único papel" mantida | mensagens.go (hUsuarioPapelAdd/Del); onda_v154_e1.go; onda_v154_e1_test.go | ✅ |
| R-11 | Drive serve HTML inline (MIME do cliente) + Content-Disposition sem escapar | drive.go:701/:764-774 | P1 |
| R-12 | ~~**Chefe-zumbi**: chefeComandaSetor fallbacks + usuarios.setor_id nunca limpo; hSetoresAgregado usa fonte divergente~~ **CORRIGIDO na onda v1.5.4-D1 (decisão D-2)**: `chefe_setores` vira FONTE ÚNICA — fallbacks `usuarios.setor_id`/`pessoas.setor_id` removidos do `chefeComandaSetor`; novo `setorAtivoComandado` condiciona o CONTEXTO da sessão ao comando vigente (concluir/reabrir/marcar e leitura do `/hoje` — contexto órfão de chefia anterior não autoriza nada); `hUsuarioPapelAdd` (chefe_setor+setor_id) MATERIALIZA o comando (UPSERT 1:1 por setor — chefe anterior perde a linha) e `hUsuarioPapelDel` revoga os comandos do grupo; migração **v44** materializa os legados pendentes (fontes `usuarios.setor_id` → `pessoas.setor_id`, INSERT OR IGNORE — UNIQUE(setor_id) preserva o vigente) e subiu o `versaoSchemaBinario` (44); `hSetoresAgregado` lê `chefe_setores` (igual ao catálogo). Resíduo p/ D-5: designação de escala do chefe e pre_fechamento ainda leem o contexto de cadastro | onda_v154_d1.go (migrarV44 + setorAtivoComandado); onda_0510_conf_escopo.go:139/:179; server_conferencia.go:196/:689/:751; mensagens.go:84/:256; onda_0510_itens79.go:173; onda_v154_d1_test.go | ✅ |
| R-13 | Escritas multi-statement sem tx (hMensagensEnviar, criarConferenciaBase, hAvisosAdd, hGrupoDestituirChefe…) | mensagens.go:691-718 etc. | P1 |
| R-14 | ~~Mensagens: operador sem grupo (e gerente/chefe) enviam a qualquer papel de qualquer grupo~~ **CORRIGIDO na v1.5.4-E1**: ninguém envia fora do escopo do papel ativo (próprio grupo + subordinados ativos — `gruposSubordinadosAtivos`, vínculo bilateral); conta sem grupo não envia a ninguém (403); admin global; caixa admin continua destino alcançável e a resposta (`pai_id`) alcança o remetente da mensagem pai — matriz completa no §8 | mensagens.go (hMensagensEnviar); onda_v154_e1_test.go | ✅ |
| R-15 | excluirArquivada deixa conferencia_escalas órfã (FK sem cascade) | server_conferencia.go:799-803 | P2 |
| R-16 | ~~Troca de senha não invalida sessões; admin/admin sem expiração forçada~~ **CORRIGIDO na v1.5.4-E1**: troca própria (`/api/senha`) invalida as demais sessões e PRESERVA a corrente (decisão: quem trocou não cai do fluxo em uso — §3.C); redefinição por outrem (`/api/usuarios/{id}/senha`) derruba TODAS (`invalidarSessoesDeSenha`); admin semeado no 1º boot com a senha PADRÃO `admin` nasce `precisa_setup=1` — gate central do middleware auth recusa tudo fora de `/api/setup|/api/me|/api/logout` até a troca (seed com `SCI_ADMIN_SENHA` própria não bloqueia) | server_pessoal.go (hTrocarSenha/hUsuarioSenha); store.go (SeedIfEmpty); onda_v154_e1.go; onda_v154_e1_test.go | ✅ |
| R-17 | hUsuarioExcluir não limpa sessoes/papeis/chefe_setores (delete físico deixa órfãs) | server_pessoal.go:1342 | P2 |
| R-18 | hMoverConta não move usuario_papeis/sessões (rebaixamento cosmético) | server_pessoal.go:1049-1132 | P2 |
| R-19 | Caixa da Função: exercente vê a mensagem mas leva 403 na thread | mensagens.go:1068-1072 | P2 |
| R-20 | Drive: sem cota, sem lixeira, físico fora do backup | drive.go (design) | P2 |
| R-21 | Mural/Calendário: leituras sem checar grupo do objeto (detalhes/ciente/compartilhamentos) | mensagens.go:1674/:1757; calendario.go:1040 | P1 |
| R-22 | gruposSuperioresAtivos não exige bilateral (herança sobe por vínculo meio-declarado) | server_grupos.go:45-69 | P2 |
| R-23 | Anti-ciclo de grupo_vinculos só imediato (A→B→C→A passa) | server_grupos.go:279 | P2 |
| R-24 | NUKE promete backup automático inexistente | server_grupos.go:142 | P2 |
| R-25 | GET /api/configuracoes público expõe WEBHOOK_ATRASOS_URL | server_admin.go:285-291 | P2 |
| R-26 | ~~Envelope {funcoes,total} do catálogo tratado como array em 2 views~~ **CORRIGIDO no remoto (fb32f18, 09/10)**: envelope exclusivo do catálogo de funções; demais catálogos voltam a devolver array | server_catalogo.go:328+; fix_catalogo_envelope_test.go | ✅ |
| R-27 | Duplicações de API (calendarios×calendario; relatorio-dia.pdf×/pdf; conferencia/lista×conferencias; POST material/conferencias morto) | ver módulos | P2 |
| R-28 | ~~Ci.sh não roda go test/vet~~; ~~dados.tar.gz com banco real na árvore~~; e2e CDP fora do repo — **CORRIGIDO na v1.5.4-F**: ci.sh gateia vet+build+test; dados.tar.gz movido para fora da árvore; e2e CDP fora do repo segue pendente para M8 | ci.sh; e2e_f2_pessoal.py:17 | P1 processo (parcial ✅) |

---

*Fim do mapa. Atualize-o no mesmo commit de qualquer alteração que ele descreve.*
