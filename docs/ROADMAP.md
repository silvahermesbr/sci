# SCI — Roadmap Mestre (pós v1.5.3)

> Atualizado em **2026-10-10** após auditoria completa (ver `docs/auditoria-completa-1010.md`).
> Base: `main` @ `608e5ef` · schema **v43** · 236 testes verdes · cobertura 74,6%.
>
> **Companion obrigatório:** [`ARQUITETURA.md`](ARQUITETURA.md) — o mapa de todos os endpoints
> (~185 rotas), guardas, escopos, tabelas, efeitos colaterais e **registro de defeitos conhecidos
> (R-1…R-28)** referenciado por este roadmap. **Nenhuma onda entra em execução sem consultar o mapa.**
> Este roadmap responde "o que e por quê"; o mapa responde "onde e o que mais quebra".
> **Agente começando agora? Leia primeiro [`INSTRUCOES_AGENTES.md`](INSTRUCOES_AGENTES.md).**

---

## Como usar este documento

1. Cada onda/módulo tem **escopo fechado, causa-raiz referenciada (arquivo:linha) e critério de aceite**.
2. Uma onda só é considerada concluída com o **gate de saída** (ver §Definition of Done).
3. Bugs descobertos no caminho não entram na onda corrente: viram item da onda de correções seguinte
   (exceto P0 de segurança/vazamento, que entra imediatamente).
4. A ordem das ondas **não é negociável** sem decisão explícita: v1.5.4 estanca os P0 antes de qualquer
   feature nova — não existe justificativa de negócio para feature sobre fundação furada.

---

## Estado atual (resumo da auditoria 10/10)

**Saudável:** binário único autocontido; SQLite WAL+FULL+FK-on; Argon2id OWASP; sessões hasheadas;
SQL 100% parametrizado; sanitizador server-side; backup `VACUUM INTO`+sha256+MANIFEST; 236 testes
HTTP-level por persona; documentação disciplinada.

**Doente (detalhe em `auditoria-completa-1010.md`):**
- 3 P0 verificados: bypass de escopo p/ contas sem grupo (`-1`); XSS armazenado via `foto_base64`;
  import de backup quebrado (swap sem rename + migrações só até v12 na reabertura).
- 3 bugs de campo relatados pelo comando (causa-raiz abaixo em v1.5.4): chefe-zumbi de setor;
  acesso a módulos fora da esfera via URL/API; conferência por antiguidade desconsiderando as tags.
- Classe P1: NUKE viola FKs; `hUsuarioPapelDel` fail-open; fotos sem escopo; Drive serve HTML inline;
  **ci.sh não roda `go test`**; migrações encadeadas à mão (divergência já ocorrida).
- Estrutural: autorização pulverizada (6 middlewares + 103 prólogos); 3 sistemas paralelos de
  designação (papéis, cadeiras, chefias) sem documento único; módulos dormentes custando binário.

---

# ONDAS DE EXECUÇÃO

## v1.5.4 — Estancar (P0 + bugs de campo) — prioridade ALTA, esforço ~2-3 dias

Nada de feature. Cada item abaixo tem causa-raiz já apontada — a correção é pequena; o que falta
é a disciplina de tocar todos os pontos listados no `ARQUITETURA.md`.

### A. Fechar a classe escopo `-1` (P0 + bug "acesso via URL")
- **Causa-raiz:** `escopoDoUsuario` (server_grupos.go:202) devolve `-1` p/ conta sem grupo, mas
  ~20 handlers bifurcam só `if escopo > 0` → o `-1` cai no ramo sem filtro (ver mapa, coluna Escopo).
- **Correção:** guarda central (middleware ou função `exigeEscopo()`) + varredura de TODOS os ramos
  `esc > 0` / `esc <= 0` / `? <= 0` do mapa.
- **Aceite:** teste de matriz — conta sem grupo recebe 403/JSON vazio em TODAS as rotas de dados
  (o teste itera a tabela de rotas do `ARQUITETURA.md`).

### B. Blidar `foto_base64` (P0)
- **Causa-raiz:** gravação sem validação (server_pessoal.go:343 e hPerfilSet :994-1003) →
  `<img src="${...}">` sem escape (core.js:580, views_gestao.js:548/:2535) com CSP `unsafe-inline`.
- **Correção:** validar prefixo `data:image/(png|jpeg|webp);base64,` + decodificar + teto ~512 KB no
  servidor; `esc()` nos sinks.
- **Aceite:** teste com payload `x" onerror=` rejeitado no PATCH e inócuo na listagem.

### C. Consertar import de backup (P0)
- **Causa-raiz:** server_admin.go:259-274 (nunca renomeia `.novo`→`sci.db`; remove WAL do arquivo
  errado) + store.go:1372-1402 (reabre migrando só até v12) + rollback reabrindo o próprio import.
- **Correção:** swap com rename real; reabertura pela cadeia COMPLETA (item E); rollback pelo caminho
  original capturado antes da mutação de `s.arquivo`.
- **Aceite:** teste que importa, fecha o Store, reabre e verifica persistência + schema.

### D. Corrigir os 3 bugs de campo (relatos do comando)

**D1 — "2 chefes de setor simultâneos" (chefe-zumbi):**
- **Causa-raiz:** `chefeComandaSetor` (onda_0510_conf_escopo.go:145-164) aceita fallbacks
  `usuarios.setor_id` e `pessoas.setor_id`; `hGrupoNomearChefe` (onda_0510_escalas.go:297) escreve
  `usuarios.setor_id` como efeito colateral e NADA o limpa na destituição/troca (documentado em
  hGrupoDestituirChefe: "usuarios.setor_id NÃO é mexido aqui"). UI lê só `chefe_setores`
  (server_catalogo.go:253-257) → chefe antigo conserva poderes invisíveis.
- **Correção:** remover os fallbacks de `chefeComandaSetor` (migração one-shot que materializa
  comandos legados em `chefe_setores` antes), OU invalidar `usuarios.setor_id` ao destituir;
  decisão registrada no `ARQUITETURA.md` §Chefias. Teste: nomear A, trocar p/ B, A perde
  concluir/reabrir/marcar do setor IMEDIATAMENTE.

**D2 — "Acesso a módulos fora da esfera via URL":**
- **Causa-raiz:** gates do `rotear()` são cosméticos (ok, por design); o buraco real é servidor:
  (i) classe `-1` (item A); (ii) rotas de PDF/dados com `a.auth(false)` e sem checagem interna de
  papel/escopo — `/api/escalas/pdf`, `/api/escalas/relatorio-dia.pdf`, `/api/escalas/minhas`
  (server_escalas.go:1703,1716-1718), PDFs de material com checagem parcial (server_material.go:2184,
  2188, 2209), `/api/conferencia/{id}/relatorio.pdf` (fall-through `-1`).
- **Correção:** cada rota de PDF ganha o mesmo guard do módulo (reservaAuth p/ escalas; authMaterial/
  escopo p/ material; escopo estrito p/ conferência). Regra nova no mapa: **nenhuma rota de dados
  sem guard declarado na tabela**.
- **Aceite:** teste por persona: operador/sem-grupo recebe 403 em todos os PDFs de outro escopo.

**D3 — "Antiguidade desconsidera as tags do grupo":**
- **Causa-raiz (3 partes):** (i) o filtro casa a pessoa por 3 fontes (`p.funcao_id` OU
  `usuarios.funcao_id` OU `usuario_papeis.funcao_id`, onda_0910_conf_antiguidade.go:132-134), mas a
  ORDENAÇÃO usa só `p.funcao_id` (`ORDER BY COALESCE(fu.antiguidade,999)`) e o relatório na tela
  (`hConferenciaGet`, server_conferencia.go:985) não ordena por antiguidade NUNCA — só
  `situacao, nome`; (ii) militar sem vínculo com a tag em nenhuma das 3 fontes é excluído em
  silêncio (lista "some"); (iii) não há herança de grupo superior (por design 09/10 — documentar,
  pois grupos acostumados com catálogo herdado esperam herança).
- **Correção:** unificar fonte de ordenação (mesma expressão COALESCE das 3 fontes do filtro) em
  `hConferenciaGet`, pré-fechamento e PDFs; ao iniciar conferência por antiguidade, responder
  `sem_tag: [nomes]` (militares do grupo fora do filtro) p/ o front avisar; decidir/documentar
  política de herança.
- **Aceite:** teste com militar tagueado só na CONTA (não na pessoa) aparece na ordem correta;
  teste de aviso de excluídos.

### E. Correções de guarda remanescentes (P1 de segurança — IDs R-x do `ARQUITETURA.md` §14)
- `hUsuarioPapelDel/Add`: allowlist de solicitantes + escopo do alvo (R-10; mensagens.go:112-138, 246-278).
- `hMudarContexto`: setor precisa pertencer ao grupo do papel ativo; parar de reescrever
  `usuarios.setor_id` global (R-9; mensagens.go:44-67).
- `GET /api/usuarios/{id}/foto`: checagem de escopo (R-5; server_pessoal.go:1015).
- Drive: allowlist de MIME p/ `inline` (ou `attachment`+nosniff sempre) e Content-Disposition
  sanitizado (R-11; drive.go:701-707, 764-774).
- NUKE/hSetorExcluir/excluirArquivada: completar DELETEs — `chefe_setores`, `funcao_membros`,
  `avisos`+`aviso_cientes/comentarios`, `material_conferencias`+itens, `escala_modelos`+postos+aptos,
  `setor_sugestoes`, `grupo_setor_responsaveis`, `conferencia_escalas` (R-6, R-15) + teste com grupo
  "rico" (server_grupos.go:144-186).
- Mural/calendário: leituras por id sem checar grupo do objeto — `hAvisosDetalhes/Ciente`,
  `GET /api/calendarios/{id}/compartilhamentos` (R-21).
- Mensagens: hierarquia de envio — operador SEM grupo (hoje envia a qualquer papel de qualquer
  grupo) e revisão geral de quem pode enviar para quem (R-14; mensagens.go:640-678).
- Arquivar/descartar conferência: alinhar código à doutrina (só gerente/enc; hoje operador passa)
  (R-8; server_conferencia.go:741/:882).
- Troca de senha invalida sessões + admin/admin com troca obrigatória no 1º boot (R-16).

### F. CI passa a gatear de verdade
- ci.sh: `go vet` + `go test -count=1 ./...` (190s) obrigatórios; `-race` no host Linux.
- Remover `dados.tar.gz` (banco real) da árvore de trabalho.

**Gate de saída v1.5.4:** todos os itens com teste de regressão; matriz rota×papel verde; auditoria
de escopo re-executada (mesmo formato de `docs/auditoria-escopos-pessoal.md`, agora cobrindo TODOS
os módulos); `ARQUITETURA.md` atualizado com cada mudança; CHANGELOG v1.5.4.

---

## v1.5.5 — Consolidação estrutural — prioridade MÉDIA-ALTA, esforço ~1 semana

Objetivo: reduzir o custo de CADA onda futura. Sem isso, v1.5.4 volta a apodrecer em 2 semanas.

1. **Autorização central:** tabela de política por rota no `rotas*()` (rota → guard declarativo).
   Os 6 middlewares viram políticas nomeadas; os 103 prólogos inline viram chamadas à política.
   O `ARQUITETURA.md` passa a ser GERADO desta tabela (fim do drift mapa×código).
2. **Registry de migrações:** `[]func() error` único (mata a divergência reopen); rebuilds v4/v17/v24/v25
   transacionais; DDL lazy (`conferencia_despachos`, `pessoas_apresentacao`) entra no versionamento.
3. **Índices de caminho quente:** `auditoria(entidade, registro_id)`, `conferencias(grupo_id, data)`,
   `mensagem_destinatarios(mensagem_id)`; retenção da auditoria (ex.: >24 meses → arquivo anual).
4. **Higiene de erros:** matar os 163 `jsonErro(w,500,err.Error())` (mensagem genérica + log interno
   com request-id); unificar `http.Error`×`jsonErro` no middleware de auth.
5. **Transações onde falta:** `hMensagensEnviar` (mensagens.go:691-718), `criarConferenciaBase`
   (onda_despacho.go:66-135), `hGrupoDestituirChefe` (onda_0510_escalas.go:316).
6. **Sessões/senha:** invalidar sessões na troca de senha; forçar troca do admin/admin no 1º boot;
   `Secure` no cookie condicional a TLS.
7. **Frontend:** decidir destino do `ui_helpers.js` (carregar e apagar as 3 cópias, ou deletar);
   unificar `esc()` (a cópia de views_avisos.js não escapa apóstrofo e ignora fuso);
   `EditorRico.sanitizaHTML` na renderização (defesa-em-profundidade); cache por hash de conteúdo
   no boot Go + `immutable` (fim do no-store + bump manual de `?v=`).
8. **`go mod tidy`**; módulos dormentes JS fora do embed (`web/acervo/`).

**Gate de saída v1.5.5:** tabela de política cobrindo 100% das rotas; `go test` no CI; zero
`err.Error()` em resposta; bench da suíte < 200s.

---

## v1.6 — Módulos: reativação e evolução (ordem acordada com o comando)

Cada módulo abaixo depende do v1.5.4 A/D2 (matriz de autorização) estar verde. **Critério geral de
reativação de módulo dormente:** (1) guardas revalidadas contra a política central; (2) rotas
desbloqueadas no `rotear()` e no menu; (3) e2e da máquina persona re-executado; (4) carga
(`ops/carga_conferencia.py` como modelo) sem regressão de p95.

### M1 — Conferência de Pessoal (evolução contínua)
- Estado: núcleo maduro (setor + antiguidade + despachos + pré-fechamento + PDF + SSE + estado-hash).
- Lacunas: D3 (antiguidade), UX de conferências simultâneas por setor, exportação de ano (PLANO_DECISOES
  item 7 — rota `/api/export/ano/{ano}` prometida e inexistente).
- Próximos: finalizar D3; modo "por antiguidade" com preview do efetivo ANTES de abrir; API de
  retenção/arquivamento anual.

### M2 — Pessoal & Cadeias de Comando (estabilização)
- Estado: efetivo, funções/cadeiras (enc_pessoal/enc_material), chefias de setor, fichas, QR, PDF.
- Lacunas: 3 sistemas de designação sem visão única (papéis/cadeiras/chefias) — criar tela "Estrutura
  de Comando do Grupo" que mostre as 3 camadas juntas (mata a classe "2 chefes" na percepção);
  campos sensíveis LGPD com política de acesso explícita; histórico de designações em auditoria
  consultável.
- Decisão pendente a registrar: política de herança de catálogo de antiguidade (hoje: sem herança).

### M3 — Material & Cautelas (evolução)
- Lacunas: SLA de atrasos só webhook (sem tela de "cautelas em atraso"); anexos base64 no banco
  (migrar p/ disco como o Drive, com meta no banco); conferência de material × antiguidade? (não se
  aplica — mas conferência por SETOR de material herda as mesmas regras de escopo do mapa).
- Próximos: tela de atrasos; migração de anexos p/ disco; etiquetas QR em lote com filtro por setor.

### M4 — Segurança & Portaria (NOVO — proposal em `roadmap-pessoal-seguranca.md`)
- Visitantes: `sci://v:{id}:{token}` + `responsavel_pessoa_id`; leitura na guarita mostra quem
  autorizou. Veículos por pessoa (`pessoa_veiculos`).
- **Depende de:** M2 estável (fichas) + decisão sobre token offline.
- Aceite: portaria consulta por QR sem login completo (sessão de toque única ou kiosk mode).

### M5 — Escalas (reativação) — depois de M1/M2
- Revalidar `reservaAuth` contra a política central; desfazer duplicação de API
  (`relatorio-dia.pdf` vs `/pdf`); integração conferência (pré-fill/badges) re-testada por persona;
  decidir se `escalas/minhas` fica acessível a todos os papéis (hoje a.auth(false)).

### M6 — Calendário (reativação)
- Unificar as 2 gerações de API (`/api/calendarios*` vs `/api/calendario/*`) ANTES de reexibir;
  compartilhamentos com ACL revisada; só então menu.

### M7 — Consciência Situacional (reativação)
- Corrigir modal transbordando (PENDÊNCIA P4 antiga); escopo do resumo revisado; re-carga.

### M8 — Infra & Qualidade (contínuo)
- Observabilidade: access log com request-id; métricas além de `/api/admin/sistema/metricas`.
- E2E reproduzível: driver CDP dentro do repo (hoje path absoluto do host).
- TLS/terminação documentada (ou túnel) —(cookie `Secure` depende disso).
- Drill de restauração de backup trimestral documentado (import corrigido na v1.5.4 torna o drill
  possível — hoje ele falharia no restart).
- Extração gradual de pacotes `internal/` nos recortes puros (store→pdf→api) — SEM rewrite.

---

## Definition of Done de TODA onda (gate obrigatório)

1. `go vet` + `go build` + `go test -count=1 ./...` verdes (e no CI a partir da v1.5.4-F).
2. Teste de regressão novo para cada comportamento corrigido/criado (padrão persona do repo).
3. **`ARQUITETURA.md` atualizado** — toda rota tocada tem sua linha na tabela do módulo revisada
   (guarda/escopo/tabelas/notas); se a doutrina mudou, a seção "Doutrinas transversais" também.
4. CHANGELOG com hash e escopo; cache-bust se `web/` mudou (doutrina atual) — ver §Cache do mapa.
5. Auditoria de escopo do módulo tocado re-executada (formato `auditoria-escopos-pessoal.md`).
6. Nenhum `- [ ]` deste roadmap marcado sem o item correspondente no CHANGELOG.

---

## Registro de decisões pendentes (cada uma vira item de onda quando decidida)

| # | Decisão | Impacta |
|---|---|---|
| D-1 | Herança de catálogo de antiguidade entre grupos (hoje: NÃO herda) | M1/M2 |
| D-2 | Fallbacks de `chefeComandaSetor`: remover puros ou materializar legados | v1.5.4-D1 |
| D-3 | `escalas/minhas` e PDFs de escala: quem acessa quando o módulo volta | M5 |
| D-4 | Anexos de material: disco (doutrina Drive) vs base64 no banco | M3 |
| D-5 | `usuarios.setor_id`: coluna de cadastro ou fonte de autorização? (hoje: os dois, o que causa o bug D1) | M2 |
