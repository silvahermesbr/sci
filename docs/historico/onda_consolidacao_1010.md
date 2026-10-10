# Onda de consolidação — merge total das branches em `main` (10/10/2026)

Registro da operação que encerrou o modelo de frentes paralelas: **todas as branches
do repositório estão contidas em `main`** e foram apagadas após prova. Complementa
`branches_arquivadas.md` (limpeza 09/10, host Linux) e é a base da entrada
[v1.5.2] do `CHANGELOG.md`.

## Estado encontrado

Estudo pré-operação (`git fetch` + `merge-tree` simulado + `git branch --no-merged`)
revelou que o merge total já estava 93% feito: **13 das 14 branches remotas já eram
ancestrais de `main`** (`69572a3`). A única pendente era `fix/pessoal-front-ux`
(1 commit: `f954db7` — ordenação+paginação EFETIVO/CHEFIAS + auditoria UX).
Sem migrações novas (schema permanece **43**) e sem conflitos textuais.

## Revisão CEO pré-merge

`f954db7` trocava os helpers locais por `window.tabelaControles` (guard `typeof`).
Os globais só existem em runtime se `views_gestao.js` já tiver carregado (rotas
#/admin, #/grupos, #/perfil) — abrindo `#/pessoal` direto, a ordenação/paginação
silenciosamente não ativava, violando o contrato "autocontido" do cabeçalho do
próprio arquivo (`ui_helpers.js`, que também os define, não é carregado por ninguém).

Fix cirúrgico **na branch** antes do merge (`823296f`): as duas chamadas voltaram a
usar os helpers locais `pesTabelaControles` (mesma assinatura), preservando as
melhorias da frente (`tb.closest('table')`, 3ª coluna sem ordenação, paginação 20).
Precedente: `9b0f391` (revisão CEO corrige bug latente que o relatório declarou
inexistente).

## Merge e cache-bust

| Commit | Conteúdo |
|---|---|
| `823296f` | fix(pessoal) CEO — helpers locais (contrato autocontido) |
| `8c250b8` | merge --no-ff `fix/pessoal-front-ux` → `main` (CEO) |
| `78b30c0` | cache-bust v368→**v369** (4 refs `index.html` + `CACHEBUST` `lazy.js`) |

## Prova da limpeza (10/10/2026)

Critério: `git merge-base --is-ancestor origin/<branch> main` → apagar. Main local
em `8c250b8` no momento da prova.

### Branches REMOTAS (14 — todas exceto `main`)

Prova coletada contra main local `8c250b8` antes da exclusão. **Nota factual:** ao
executar a exclusão, 13 das 14 já **não existiam mais no remoto** (a operação 09/10
do host Linux já as havia removido; os trackings deste clone Windows é que estavam
obsoletos). Esta onda apagou de fato do remoto apenas `fix/pessoal-front-ux`
(ainda em `f954db7`) e fez `git fetch --prune` dos 13 trackings obsoletos.

| Branch | Tip | Prova |
|---|---|---|
| origin/f2/material-blindagem | 10aed3f | merge-base --is-ancestor → ancestral de main |
| origin/f2/material-features | 621ab7d | merge-base --is-ancestor → ancestral de main |
| origin/f2/modulo-pessoal | 6491e6f | merge-base --is-ancestor → ancestral de main |
| origin/f3/backend-modelo | 134fa5c | merge-base --is-ancestor → ancestral de main |
| origin/f3/front-acessos | 98cb56f | merge-base --is-ancestor → ancestral de main |
| origin/f3/qa-integracao | ab4841c | merge-base --is-ancestor → ancestral de main |
| origin/feat/conferencia-antiguidade-fechamento | e79403c | merge-base --is-ancestor → ancestral de main |
| origin/fix/contexto-setor-conferencia | 0eea546 | merge-base --is-ancestor → ancestral de main |
| origin/fix/pessoal-front-ux | f954db7 | merge-base --is-ancestor → ancestral de main (via `8c250b8`) — apagada nesta onda |
| origin/fix/pessoal-funcao-por-chave | 93bde1f | merge-base --is-ancestor → ancestral de main |
| origin/fix/r2-pessoas-edit | 29ce3ea | merge-base --is-ancestor → ancestral de main |
| origin/fix/r3-poderes-designacao | 2ff936c | merge-base --is-ancestor → ancestral de main |
| origin/integracao-onda-0910 | 63432de | merge-base --is-ancestor → ancestral de main |
| origin/riko/landing | 53d72e3 | merge-base --is-ancestor → ancestral de main |

### Worktrees auxiliares removidos

| Worktree | Branch | Estado |
|---|---|---|
| `wt_frenteA_pessoal` | fix/pessoal-funcao-por-chave | limpo, merged |
| `wt_frenteB_pessoalfront` | fix/pessoal-front-ux | limpo, merged (via `8c250b8`) |

### Branches LOCAIS apagadas (7)

`f2/material-blindagem`, `f2/material-features`, `f3/backend-modelo`,
`f3/front-acessos`, `f3/qa-integracao`, `fix/pessoal-front-ux`,
`fix/pessoal-funcao-por-chave` — todas merged, `git branch -d` (sem `-D`).
Estado final: somente `main`, local e remoto.

## Resíduos conhecidos (fora do escopo desta onda)

- **Host Linux de produção** (`/opt/data/workspace/projetos/sci/app`): tem locals
  arquivadas pela operação 09/10 (`mod/*`, `chore/limpeza-geral-0910`,
  `fix/encarregado-*`, `fix/conf-antiguidade-grupo`) ausentes neste clone — basta
  `git pull` do `main` novo antes do próximo deploy (`deploy_v92_sci.py`; jamais os
  scripts de deploy que apagam o banco).
- `web/ui_helpers.js` é código morto no `main` (nenhum loader o referencia) —
  candidato a limpeza futura.
- Consolidação maior (mover `tabelaControles` para `core.js` e unificar com as
  cópias de `views_gestao.js`) — deliberadamente adiada.
