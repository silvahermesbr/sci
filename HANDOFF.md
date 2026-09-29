# HANDOFF — FIM DE RODADA 29/09/2026 (SCI v9.16.11)

> **Sessão encerrada pelo Tenente.** Este documento registra o estado completo do projeto,
> o que foi entregue na rodada e as pendências/orientações para as próximas rodadas.
> **Documento complementar:** `HANDOFF_FRONTEND.md` (contratos de API, telas e doutrina — continua válido).

---

## 1. ESTADO EM PRODUÇÃO (10003)

- **Versão:** v9.16.11 (commit `b1e9ec7`), binário sha `e4e37a57…`, `?v=225`
- **Health:** 200 · CI VERDE · GitHub sincronizado (main)
- **Front:** modular (core.js + views_conf.js + views_gestao.js + style.css), cache-bust `?v=225`,
  estáticos com `Cache-Control: no-store` (navegador nunca mais segura JS velho)
- **Schema do banco:** v12 (última migração: `antiguidade` nos catálogos de organização)
- **Dados:** estrutura real do Tenente — **3º B Com GE (id 3) > Cia Com (id 4) > 3º Pel Com (id 1)**,
  56 militares no 3º Pel, contas admin/tenhermes/3bcomge/ciacom
- **Resíduo conhecido:** grupo **"Cia Parcial" (id 2)** — resíduo de teste da Aimi com 1 pessoa
  inativa (57) e várias conferências FECHADAS (imutáveis por doutrina). Não excluído de propósito.
  Se o Tenente quiser removê-lo: exigiria exceção doutrinária ou novo reset total (com backup pré).

## 2. ENTREGUE NESTA RODADA (resumo por tema)

- **Herança de catálogos RECURSIVA em toda a cadeia** (3º Pel herda de Cia e do Bde);
  aba Tags separa **HERDADO (read only)** × **DO MEU GRUPO** (gerenciável)
- **Catálogos em LISTA** com **EDITAR** (PATCH `/api/catalogo/{t}/{id}`) e **EXCLUIR**;
  herdados são somente leitura (403)
- **Antiguidade de catálogos** definida por **DRAG & DROP** (modal "⚖ Definir antiguidade"),
  coluna `antiguidade` (schema v12), ordenação hierárquica (caminho pai>filho) — **sem alfabética achatada**
- **Relatórios:** ordenação **FUNÇÃO > SETOR > alfabética** (função maior que setor); colunas
  **ORD + Função** antes do nome; **sem "Por"** e **sem colunas de convocações/% válidas** (tela e PDF)
- **Estado Atual do efetivo** no topo de Relatórios/Dashboard: estado = última conferência do militar;
  alertas ● verde (hoje) / ● amarelo (1+ dia) / ● vermelho (1+ semana ou nunca); detalhe só no PDF
- **Conferência:** #/hoje = só LISTAS (abertas c/ botão ABRIR; fechadas = só PDF); edição em
  `#/conferencia?id=N`; **múltiplas conferências simultâneas**; **salvamento parcial** (✅ e situação
  gravam na hora); **carry over** (nova conf herda situação/destino se última conf ≤ 1 dia; 2+ dias
  reseta p/ presente); **checkbox ✅ sempre zerado** em conferência nova (verificado=0 no carry);
  **descartar conferência** (modal duplo, apaga parciais; fechada nunca); busca com **debounce**
  preservando foco; FECHAR religa handlers a cada re-render
- **Gestão:** edição em lote c/ **overlay "Processando…"** (disco girando → ✅ Pronto! verde /
  ❌ vermelho com o erro real) em toda ação de escrita; coluna **ATIVO** (ativos 1º + alfabética);
  **INDEFINIDO** p/ militar sem setor/função; toggle de grupos em **todos os níveis** c/ efetivo
  recursivo por nó
- **Infra de front:** estáticos **NO-STORE** (fim do cache de JS velho), burger menu mobile
  (drawer esquerdo → X, título central, dropdown usuário à direita, acima de tudo ao abrir)

## 3. BUGS RAIZ CORRIGIDOS (aprender com eles)

1. **Scan de colunas**: SELECT sem a coluna nova → `rows.Scan` falha em silêncio → API devolve
   `[]` (parecia "não carrega"). SEMPRE alinhar SELECT/cols/Scan.
2. **MAX de string para "última"**: `'…#9' > '…#13'` lexicográfico → escolhia conferência errada.
   Usar ROW_NUMBER PARTITION BY. (fix no carry over)
3. **Herança só 1 nível**: gruposSuperioresAtivos agora sobe a cadeia inteira (loop de ancestrais).
4. **Stacking context**: drawer dentro de header sticky ficava atrás de tudo → reanexar ao body ao abrir.
5. **Modal**: `abrirModal()` devolve OBJETO `{fechar, mask, modal}` — ligar handlers no `.mask`/`.modal`.
6. **Cache do navegador**: NO-STORE nos estáticos + `?v=N` (cache causou checks fantasmas e JS velho).

## 4. PENDÊNCIAS / PRÓXIMAS RODADAS (sugestões do estado atual)

- [ ] **Resíduo "Cia Parcial"**: decidir exclusão excepcional ou ignore permanente
- [ ] **Redesign visual**: protótipo aprovado em estrutura; polimento fino (ícones SVG padronizados,
      animações, tipografia) pode seguir em nova rodada — estrutura modular já pronta
- [ ] **Hierarquia de setores**: preencher `pai_id` dos setores reais do Tenente (hoje sem setor
      definido → "INDEFINIDO") para o agrupamento hierárquico dos relatórios ficar completo
- [ ] **Mobile**: conferência em tela pequena (lista de militares) — testar em aparelho real
- [ ] **Antiguidade de catálogos**: hoje por categoria; avaliar arrastar direto na lista (sem modal)
- [ ] **Pendências externas ao SCI** (não esquecer): `agenda_visita --go` (NC), cápsula EMS f2

## 5. ORIENTAÇÕES PERMANENTES (next session)

- **Deploy SEM WIPE**: backup pré → kill por PID exato de `ss -tlnp` → sftp sci.new → mv → setsid
  → validar. **PROIBIDO** `deploy_v9_sci.py`/`pos_deploy_sci.py` (apagam o banco!)
- **WATCHDOG no host** (`ops/watchdog_sci.sh`) ressuscita binário velho se o sci morrer — durante
  deploy, pausar o watchdog, subir, e reativar
- **GitHub push 500 intermitente** (remote rejected Internal Server Error): apenas repetir o push
- **Pool pós-IMPORT de backup**: conexões antigas podem dar 500 FK falso → restart resolve
- **Testes**: instância efêmera local (SCI_DATA_DIR=/tmp/x SCI_PORT=14xxx), um cookie jar por
  persona, header `X-SCI: 1` em toda escrita, `node --check` nos JS, `ci.sh` VERDE antes de commit
- **Rótulos oficiais**: "Conferência de pessoal" (nunca "formatura"), status Aberta/Fechada,
  situações Presente/Atraso/Falta/Justificada, horários SEMPRE Brasília
