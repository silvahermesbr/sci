# SCI — Instruções para os Próximos Agentes (despacho pós-auditoria 10/10)

> **Leia isto antes de escrever qualquer linha de código.**
> Você foi despachado para executar o roadmap. Este arquivo é a ordem de serviço; os documentos
> abaixo são o terreno. Eles foram gerados por auditoria completa do código em `main` @ `608e5ef`
> (v1.5.3, schema v43) — tudo aqui foi verificado linha a linha com referência `arquivo:linha`.

## Ordem de leitura obrigatória

1. **[`ARQUITETURA.md`](ARQUITETURA.md)** — o mapa de TODO o sistema: ~185 endpoints com guarda,
   escopo, tabelas e efeitos colaterais; doutrinas transversais; matriz papel×módulo; checklists de
   impacto; **registro de defeitos R-1…R-28**. Consulte a linha da rota que você vai mexer E a seção
   "Se você alterar este módulo, verifique também" ANTES de codar, e ATUALIZE-AS depois.
2. **[`ROADMAP.md`](ROADMAP.md)** — o plano: ondas v1.5.4 (estancar P0s + bugs de campo), v1.5.5
   (consolidação), v1.6 (módulos M1-M8). Cada item tem causa-raiz citada e critério de aceite.
3. [`auditoria-completa-1010.md`](auditoria-completa-1010.md) — contexto do diagnóstico (opcional,
   para entender o "porquê" dos P0s).

## Regras de execução (inegociáveis)

1. **Ordem das ondas é fixa.** Nada de feature antes de v1.5.4 completa. Um P0 aberto não negocia
   com prazo de negócio. Dentro da v1.5.4, a ordem sugerida é A → B → C → D1/D2/D3 → E → F (mas A
   primeiro: a classe de escopo `-1` é a que mais vaza dados).
2. **Um item do roadmap = uma branch = um PR/merge.** Padrão do repo: `onda/v1.5.4-a-escopo` etc.,
   merge para `main` só com o gate de saída verde. Mensagens de commit no padrão do histórico
   (`feat(gap1) …`, `fix(conf) …`, `docs: …`), em português, escopo no sujeito.
3. **Protocolo de alteração** (ARQUITETURA.md §1): antes de codar, leia a rota no mapa + seção de
   impacto do módulo; depois de codar, **atualize o mapa no mesmo commit**. Mudou doutrina de papel →
   os 3 pontos do `web/core.js` (menu `montarShell`, portões do `rotear()`, `rotaInicial()`) mudam
   JUNTOS com o servidor — um lado sem o outro é o bug que originou esta auditoria.
4. **Todo fix ganha teste de regressão** no padrão do repo (persona: `setupTestApp` + `loginAs` +
   `doJSONReq`, casos positivo E negativo). Sem teste, o item não fecha.
5. **Gate de saída de toda onda:** `go vet` + `go build` + `go test -count=1 ./...` verdes;
   CHANGELOG atualizado (com número de onda e escopo); cache-bust se `web/` mudou (bump ÚNICO por
   onda, nunca rebaixar o número); ARQUITETURA.md atualizado; roadmap marcado.

## Regras técnicas que já quebraram o sistema antes (não repita)

- **Pool SQLite = 1 conexão**: nunca `Query/QueryRow/Exec` no pool com `rows.Next()` aberto
  (deadlock). Dentro de transação, TODO acesso pela tx (`executorSQL`). Multi-escrita atômica → tx.
- **Escopo tem 3 valores**: 0=admin (tudo), -1=sem grupo (**nada**), N=grupo. Todo novo `if esc > 0`
  precisa do ramo `-1` tratado — a ausência disso é o P0 R-1.
- **Migração nova**: número sequencial único (já colidiu v42/v43), registrar na cadeia do
  `AbrirStore` **e** no `ReabrirComArquivo` (hoje divergem — R-4 corrige com registry único).
- **Fuso**: tudo que exibe hora usa `America/Sao_Paulo` (`App.horaLocal`/`fmtDataBR`).
- **Frontend renderiza rich text cru**: a sanitização é server-side na escrita. Novo campo de texto
  → sanitizar na entrada (sanitiza.go) e usar `esc()` do core.js no template.
- **Nunca** comitar: `dados/`, `*.db`, binários, `dados.tar.gz` (o .gitignore cobre; confira
  `git status` antes do commit).
- **Doutrina do projeto**: proibição de refactor cosmético sem furo comprovado (consta da auditoria
  interna v1.3). Corrija o defeito; não reescreva o que funciona.

## Ambiente e verificação

- Máquina dev Windows: `go vet ./...`, `go build`, `go test -count=1 ./...` (suíte ~190s, 236
  testes, cobertura 74,6%). `-race` exige cgo/gcc — **rodar no host Linux**, junto com `bash ci.sh`
  (que é Linux-only e, até a v1.5.4-F, não roda testes — atualize-o no item F).
- Cada teste cria SQLite temporário com a cadeia de migrações inteira — quebra de migração falha
  a suíte TODA (bom sinal, não um bug).
- `ops/carga_conferencia.py` é o harness de carga pós-mudança na conferência (p50/p95).

## Primeira missão sugerida (v1.5.4-A)

Fechar a classe de escopo `-1` (R-1): guarda central `esc < 0 → 403` + varredura de TODOS os ramos
`esc > 0`/`esc <= 0`/`? <= 0` listados na coluna Escopo das tabelas do ARQUITETURA.md + teste de
matriz (conta sem grupo contra todas as rotas de dados). Aceite e detalhes: ROADMAP.md §v1.5.4-A.
Ao terminar, marque o item no ROADMAP, atualize a coluna Escopo das linhas tocadas no mapa e siga
para o item B.

## Decisões que NÃO são suas

D-1 a D-5 (ROADMAP, final) são decisões de comando (herança de antiguidade, fallbacks de chefia,
acesso a escalas, anexos em disco, semântica de `usuarios.setor_id`). Implemente o que o roadmap
mandar; se uma onda depender de uma D-x pendente, pare e registre a pergunta no PR — não decida
sozinho.
