# HANDOFF — SCI Frontend Redesign (para a Seção de Frontend)

> **STATUS: CONCLUÍDO (29/09/2026, commit d70452a).** O redesign foi implementado e está em
> produção (v200: core.js + views_conf.js + views_gestao.js + style.css novo). Este documento
> permanece como referência de contratos/telas/doutrina para manutenção futura.

> **Missão (original):** redesenhar o frontend do SCI (protótipo funcional → produto profissional).
> **Regra de OURO:** o backend (Go) e a API **NÃO mudam**. Todo o trabalho é em `web/`.
> **Estado do código no momento deste handoff:** commit `a413857`, tag `v9.11.2`, produção
> em `10.10.0.5:10003` (v9.11.2, `app.js?v=109`). Banco RESETADO e zerado (fresh start).

---

## 1. Stack e arquitetura ATUAL (o que você vai encontrar)

- **Vanilla JS**, sem build-step, sem framework. Arquivo único: `web/app.js` (~700 linhas).
- `web/index.html`: casca (topbar, `#app`, `#toasts`, dropdown de usuário).
- `web/style.css`: tema escuro/verde-militar atual (custom properties em `:root`).
- **Hash routing** (`location.hash`), funções `view*()` por tela, re-render total a cada
  navegação (`rotear()` no final do app.js).
- API via helper `api(path, opts)` — injeta `X-SCI: 1` em métodos de escrita (anti-CSRF),
  cookie de sessão `sci_sessao` (HttpOnly), 401 → volta ao login.
- Cache-busting: `?v=N` em `index.html` (a cada deploy, incrementar N).
- **Embed:** tudo é servido DENTRO do binário Go via `embed.FS` (`web/` inteiro). Não há
  servidor de assets separado.

**RESTRIÇÕES HARD:**
1. **NÃO alterar** `server.go`, `auth.go`, `store.go`, `relatorio.go` (backend congelado).
2. **NÃO mudar contratos de API** (rotas/payloads) — a lista completa está abaixo.
3. **Manter** o helper `api()`, o fluxo de login/logout, hash routing e cache-busting
   (podem ser refatorados internamente, mas o comportamento permanece).
4. **Rótulos oficiais:** "Conferência de pessoal" (nunca "formatura"); status só
   **Aberta/Fechada**; situação: Presente/Atraso/Falta/Justificada; papéis ADMIN/GERENTE/OPERADOR.
5. UI profissional: **sem textos-meta** (nada de "só o admin pode" etc.), sem emojis em
   botões de ação críticos (💬/✅/✕ podem virar ícones de biblioteca).

## 2. Papéis e navegação (congelado)

| Papel | Abas (ordem) | Homepage pós-login |
|---|---|---|
| **admin** | DASHBOARD · ADMIN · RELATÓRIOS | `#/admin` |
| **gerente** | CONFERÊNCIA · RELATÓRIOS · GERENCIAR | `#/hoje` |
| **operador** | CONFERÊNCIA · RELATÓRIOS | `#/hoje` |

- Admin **não vê** conferências (nem lista) — backend retorna 403; catálogos 403; sem "Meu usuário".
- Dropdown do nome (topo direito): **Meu usuário** (não-admin) · **Mudar senha** · **Logout**.

## 3. Telas existentes (paridade obrigatória — nada pode sumir)

### #/hoje — CONFERÊNCIA (gerente/operador)
- Se **sem conferência aberta**: card "Nenhuma conferência aberta" + botão ▶ Iniciar
  (sem campo de nome — ID é automático).
- Se **aberta**: banner `Conferência #ID · aberta em DATA às HH:MM`, barra fixa com busca
  por nome + contador verificados + ✕ FECHAR CONFERÊNCIA (confirma; **recarrega a página**).
- Lista agrupada por setor; cada militar: checkbox verificação ✅ · nome+completo+função ·
  **DROP-DOWN de situação** (Presente/Atraso/Falta/Justificada — NÃO cíclico) ·
  select de destino (só quando justificada) · 💬 comentários (append-only por conferência).
- Mudar p/ falta/justificada abre modal (justificada EXIGE destino).
- **Histórico embutido** abaixo: filtro por ID + duas tabelas (Abertas; Fechadas) com
  colunas **ID · Status · Horário · Data · Grupo · Operador · Lanç. · Relatório(PDF)**.

### #/relatorios — RELATÓRIOS (todos)
- Filtro de escopo: admin = todos os grupos; gerente = **Meu grupo + subordinados /
  Meu grupo (somente) / cada subordinado individual**.
- Abas de modo: **Dia (default) · Semana · Ano · Período livre**.
- Render: resumo (caixas: conferências, efetivo, presentes, atrasos, faltas, justificadas,
  faltas tot., % válidas (P+A), % ef. pronto) + botão ABRIR PDF + tabela de conferências
  do período + tabela de efetivo **em ordem alfabética** (Antig. · Nome · Função(ID) ·
  Setor · **Grupo** · Pres · Atraso · Falta · Just — **SEM coluna %**).

### #/admin — ADMINISTRAÇÃO (só admin) — 3 sub-abas
- **usuarios:** SOMENTE tabela (ID · Login · Papel · Grupo · Status · Criada · Ações:
  🔑senha · ➡mover · excluir) + filtros (login/grupo/status). SEM criação de conta
  (admin não se cria; gerentes nascem com grupo; operadores nascem no Gerenciar).
- **grupos:** árvore NESTED colapsável (raiz-only toggle) com **efetivo total recursivo**
  (total + próprio), gerente de cada grupo; criar grupo EXIGE gerente no ato
  (login+senha+nome de guerra); trocar gerente (anterior vira operador, nunca vago);
  subordinação por select + remover; **Excluir grupo** com modal (normal p/ vazio;
  FORÇADA pede senha de admin — remove contas+pessoas; grupo com conferências NUNCA).
- **backup:** gerar backup (download .db), IMPORTAR (upload → valida → backup de segurança
  automático → swap; falha = rejeita sem tocar o banco).

### #/grupos — GERENCIAR (gerente) — 4 sub-abas
- **pessoal:** form de militar + **adição em lote** (textarea `guerra;completo;setor;função`
  com exemplo) + banco de pessoal (checkbox por linha, **editar em lote** — modal único
  aplica setor/função/status —, excluir individual e em lote; com histórico → desativa).
- **tags:** catálogos do grupo — **Setores · Funções · Tags · Destinos** (adicionar,
  ✕ excluir, ⏸ desativar). Herança só DESCE (leitura do que os de cima criaram; nunca sobe).
- **grupos:** MEU GRUPO + árvore READ ONLY com efetivo recursivo.
- **operadores:** criar operador (acima da tabela) + tabela (senha · mover · excluir) + filtro.

### #/perfil — MEU USUÁRIO (não-admin; abre do dropdown)
- Login/Função/Grupo/Setor/Função (readonly) + Nome de guerra/Completo editáveis.

### #/hoje (admin) — DASHBOARD
- Leitura: painel da semana (mesmo render de relatório), sem ferramentas.

## 4. Contratos de API (NÃO MUDAR — resumo operacional)

```
POST /api/login {login,senha} → {usuario{papel,grupo_id,...}} + cookie
GET  /api/me · POST /api/logout · POST /api/senha {atual,nova}
GET  /api/conferencia/hoje → {conferencia|null{id,data,criada_em,estados{pid:{situacao,destino_id,observacao}}}, pessoas[{id,nome_guerra,nome_completo,setor,funcao}]}
POST /api/conferencia/iniciar {local?} · POST /api/conferencia/fechar {id,lancamentos[{pessoa_id,situacao,destino_id,observacao}]}
GET  /api/conferencia/lista → [{id,data,local,status,criado_por,criada_em,fechada_em,lancamentos,grupo_id,grupo}]
GET  /api/conferencia/{id}/relatorio.pdf (só fechada)
GET  /api/pessoas → {pessoas[...]} · POST /api/pessoas · PATCH /api/pessoas/{id} · DELETE /api/pessoas/{id} (com histórico → {desativado:true})
GET  /api/catalogo/{setores|funcoes|tags|destinos|conferencia_tipos|status_pessoal} → [{id,nome,pai_id,ativo,...}]
POST /api/catalogo/{t} {nome,sigla?,cor?} · DELETE /api/catalogo/{t}/{id}?modo=desativar
GET  /api/usuarios (admin: todos; gerente/operador: próprio grupo; operador sem 'senhas')
POST /api/usuarios {login,senha,papel} · DELETE /api/usuarios/{id} · POST /api/usuarios/{id}/senha {senha}
PATCH /api/usuarios/{id}/mover {grupo_id}
GET  /api/grupos · GET /api/grupos/arvore (nested: [{id,nome,codigo,efetivo,efetivo_total,contas,filhos[...]}])
POST /api/grupos {nome,login,senha,nome_guerra} · POST /api/grupos/{id}/trocar-gerente {login}
POST /api/admin/grupos/vinculo {superior_id,subordinado_id} · DELETE idem ?superior_id&subordinado_id
DELETE /api/grupos/{id}[?forcar=1 + body {senha}]
GET  /api/relatorio?de&ate&grupo · GET /api/relatorio.pdf?de&ate&grupo
GET  /api/perfil · PATCH /api/perfil · GET /api/backup + download · POST /api/backup/importar (multipart 'arquivo')
```
Erros JSON: `{"erro":"mensagem"}`. Escrita exige `X-SCI: 1` + Origin da própria origem.

## 5. Doutrina visual (decisões do Tenente — NÃO reabrir)

- Gráficos/proporções podem ser COLORIDOS; **tabelas e textos em P&B**.
- Estética desejada: **moderna, profissional, militar contido** (protótipo atual é
  verde-escuro básico). Sugerido: dark padrão + acento verde-militar + tipografia limpa,
  espaçamento generoso, micro-interações discretas. Ícones via biblioteca leve (ex.:
  Lucide inline) — sem CDN pesado (sistema roda em rede local/VPN lenta: **perf importa**,
  ver §7).
- Responsivo mínimo (uso em desktop; mobile é bônus).

## 6. Critérios de aceite (checklist da Seção)

- [ ] Paridade 100% com §3 (nenhuma função perdida) — testar CADA tela com admin E gerente E operador.
- [ ] Zero chamada nova/alterada de API.
- [ ] `node --check web/app.js` limpo; sem CDN externo; assets locais.
- [ ] Cache-bust incrementado (`?v=200+`).
- [ ] Smoke E2E manual nas 3 personas (admin/gerente/operador) contra instância efêmera local
      (SCI_DATA_DIR=/tmp/hf_d SCI_PORT=14080) ANTES de qualquer deploy.
- [ ] ci.sh VERDE · commit · push (GIT_SSH_COMMAND='ssh -F /opt/data/.secrets/github_tutela_ssh_config').
- [ ] Deploy em produção SEM WIPE (padrão deploy_v92_sci.py: backup pré → kill por PID
      exato de `ss -tlnp` → mv sci.new sci → setsid nohup → validar em NOVA conexão).
      **PROIBIDO** deploy_v9_sci.py/pos_deploy_sci.py (fazem wipe de dados!).
- [ ] Pós-deploy: health 200, v=200+, login das 3 personas, conferência abrir/fechar,
      relatório gerar, backup gerar.

## 7. Pitfalls que já custaram sessão (não repetir)

- **Cache do navegador** do usuário manteve JS velho → sempre incrementar `?v=`.
- `pkill -f` casa com o próprio `bash -c` — matar processo por **PID exato de ss -tlnp**.
- Instância de teste órfã segurando a porta → falso resultado; conferir porta livre antes.
- Cookie jar compartilhado entre personas → falso 200/403; **um jar por persona**.
- `p.setor_id` pode vir AUSENTE (não null) no JSON de pessoas — cuidado com undefined.
- Deploy scripts antigos (`deploy_v9_sci.py`, `pos_deploy_sci.py`) **APAGAM O BANCO**.

## 8. Estado do banco (fresh start)

Banco de produção **resetado e zerado** (só conta admin/admin). A Seção NÃO precisa de
dados para desenvolver (usar instância efêmera local com seed manual). Qualquer dado que
criar em produção durante testes: limpar ao final (mesmo procedimento de reset, com backup pré).
