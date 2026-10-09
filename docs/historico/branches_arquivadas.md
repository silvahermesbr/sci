# Branches arquivadas — operação de limpeza 09/10/2026

Registro da limpeza de branches do repo SCI (`/opt/data/workspace/projetos/sci/app`),
feita na branch `chore/limpeza-geral-0910`. Critério: branch **merged** (conteúdo já
em `main`, provado por `git merge-base --is-ancestor`) → apagar; conteúdo único real
(provado por `git cherry` + `git diff main...branch`) → **preservar**.

## 1ª passada (09/10, antes desta operação) — 9 branches locais merged apagadas

A 1ª passada não deixou registro nominal; os nomes e tips abaixo foram recuperados do
`git reflog` nesta operação. Todos os tips provados ancestrais de `main` nesta data.

| Branch (apagada) | Tip último registrado (reflog) |
|---|---|
| feat/v15-front-polish | 7e411df |
| onda-0510-mensageria | 9786bbe |
| onda-0510-modulos-retry | 9786bbe |
| onda-0510-senha | 9786bbe |
| sci-fila-7-9 | 4b884cf |
| sci-poll-conf | e28303c |
| sci_bugs_0610 | 2cddf28 |
| fix/contexto-setor-conferencia | 0eea546 |
| f2/modulo-pessoal | 6491e6f |

Obs.: `onda-0510-mensageria`, `onda-0510-modulos-retry` e `onda-0510-senha` apontavam
para o mesmo commit (9786bbe); os demais nomes aparecem no reflog como checkouts, o que
coincide com a contagem de 9 branches removidas na 1ª passada.

## 2ª passada (esta operação, 09/10)

### Branches LOCAIS apagadas

| Branch | Tip | Prova |
|---|---|---|
| feat/conferencia-antiguidade-fechamento | 5bd62be | merge-base --is-ancestor → ancestral de main |

### Branches LOCAIS PRESERVADAS (conteúdo único real)

| Branch | Tip | Motivo |
|---|---|---|
| f3/front-acessos | c1b5501 | `cherry` 2 patches '+' não em main (sessão por chave, aba FUNCOES, cache-bust v=360); decisão absorver/arquivar pendente — registrada no MISSAO.md da onda 09/10 |
| mod/f2-gestao | 44025f9 | cherry 2 '+'; refatoração views_gestao.js não absorvida |
| mod/f3-lazy | e387666 | cherry 3 '+'; auditoria/lazy.js não absorvido |
| mod/f1-core | cf1cb90 | cherry 3 '+'; WIP preservado pela CEO |
| mod/t3-modulos | ad2d5f4 | cherry 2 '+'; WIP preservado pela CEO Aimi |

### Branches REMOTAS apagadas (merged em origin/main, provado)

| Branch | Tip | Prova |
|---|---|---|
| origin/f2/material-blindagem | 10aed3f | merge-base --is-ancestor origin/main |
| origin/f2/material-features | 621ab7d | idem |
| origin/f2/modulo-pessoal | 6491e6f | idem |
| origin/f3/backend-modelo | 134fa5c | idem |
| origin/f3/front-acessos | 98cb56f | idem |
| origin/f3/qa-integracao | ab4841c | idem |
| origin/feat/conferencia-antiguidade-fechamento | e79403c | idem |
| origin/fix/contexto-setor-conferencia | 0eea546 | idem |
| origin/fix/pessoal-funcao-por-chave | 93bde1f | idem |
| origin/fix/r2-pessoas-edit | 29ce3ea | idem |
| origin/fix/r3-poderes-designacao | 2ff936c | idem |
| origin/integracao-onda-0910 | 63432de | idem |
| origin/riko/landing | 53d72e3 | idem |

### Branches REMOTAS PRESERVADAS

| Branch | Tip | Motivo |
|---|---|---|
| origin/main | 63432de | principal — intocável |
| origin/fix/pessoal-front-ux | f954db7 | não-merged (conteúdo único real) — preservada |

## Estado final

- Locais: `main`, `chore/limpeza-geral-0910` (esta operação), `fix/encarregado-acesso-modulos`
  e `fix/conf-antiguidade-grupo` (em uso por outros executores — preservadas por ordem),
  `f3/front-acessos`, `mod/f1-core`, `mod/f2-gestao`, `mod/f3-lazy`, `mod/t3-modulos`.
- Remotas: `origin/main` e `origin/fix/pessoal-front-ux`.
