# LIMPEZA — Codebase Cleanup Report

## APAGADOS (prova de mortidão)

### 1. `web/views_admin.js` — APAGADO no commit 79e89ec
- **Prova**: rota `#/admin` mapeia para `web/views_gestao.js` no `HASH_ARQUIVO` do `lazy.js`. Zero referências a `views_admin.js` em `index.html`, `lazy.js`, `core.js` ou qualquer `views_*.js`.
- **Carga dinâmica**: ausente do mapa `HASH_ARQUIVO` e `DEP_VIEW`.
- **Duplicação**: funcionalidade de admin já integrada em `views_gestao.js`.

### 2. `web/views_grupos.js` — APAGADO no commit 79e89ec
- **Prova**: rota `#/grupos` mapeia para `web/views_gestao.js` no `HASH_ARQUIVO`. Zero referências a `views_grupos.js` no código.
- **Carga dinâmica**: ausente do mapa `HASH_ARQUIVO` e `DEP_VIEW`.
- **Duplicação**: funcionalidade de grupos já integrada em `views_gestao.js`.

### 3. `web/views_pessoal.js` — APAGADO (commit anterior, fora do branch atual)
- **Prova**: zero referências a `views_pessoal` em `web/index.html`, `web/lazy.js`, `web/core.js` ou qualquer `web/views_*.js`. Ausente do `HASH_ARQUIVO`.
- **Histórico**: removido no commit `44025f9` (fatia de `views_gestao.js` em admin/grupos/pessoal/perfil).

### 4. `web/views_perfil.js` — APAGADO nesta rodada
- **Prova**:
  - Rota `#/perfil` mapeia para `/views_gestao.js` no `HASH_ARQUIVO` do `lazy.js`.
  - `grep -r 'views_perfil'` sobre todo o repo: **zero resultados**.
  - `grep '#/perfil' web/lazy.js` → `'/views_gestao.js'` (não `views_perfil.js`).
  - Arquivo continha comentário `/* [FATIADO da views_gestao.js] */` — era um fragmento da modularização nunca ativado.
  - `views_gestao.js` (carregado dinamicamente) já define `window.ViewPerfil` completo e funcional.

## MANTIDOS (decisão fundamentada)

### Views mantidas (referenciadas no HASH_ARQUIVO do lazy.js)
| Arquivo | Rotas | Decisão |
|---------|-------|---------|
| `web/views_avisos.js` | `#/avisos` | Mantido — referência viva |
| `web/views_calendario.js` | `#/calendario` | Mantido — referência viva |
| `web/views_conf.js` | `#/hoje`, `#/conferencia`, `#/relatorios` | Mantido — referência viva |
| `web/views_config.js` | `#/configuracoes` | Mantido — referência viva |
| `web/views_consciencia.js` | `#/consciencia` | Mantido — referência viva |
| `web/views_drive.js` | `#/drive` | Mantido — referência viva |
| `web/views_escalas.js` | `#/escalas` | Mantido — referência viva |
| `web/views_gestao.js` | `#/admin`, `#/grupos`, `#/perfil` | Mantido — referência viva, contém as 3 views fundidas |
| `web/views_material.js` | `#/material` | Mantido — referência viva |
| `web/views_mensagens.js` | `#/mensagens`, `#/despachos` | Mantido — referência viva |

Arquivos auxiliares mantidos (todos referenciados):
- `web/core.js` — eager load no `index.html`, shell/router/helpers
- `web/lazy.js` — loader dinâmico de views
- `web/editor_rico.js` — dependência de `views_conf.js` e `views_mensagens.js`
- `web/ui_helpers.js` — helpers de UI
- `web/style.css` — eager load no `index.html`

### Reservas absolutas (não tocar — outras rodadas paralelas atuam nelas)
- `web/index.html`, `web/lazy.js`, `web/views_gestao.js`
- `onda_0510_c2.go`, `server_pessoal.go`, `onda_0610_pessoal.go`
- `onda_0510_conf_escopo.go`, `store.go`, `main.go`

## CANDIDATOS-PENDENTES (classificados, não apagados)

### Scripts soltos na raiz
| Arquivo | Tipo | Observação |
|---------|------|------------|
| `ci.sh` | Shell script | Script de CI na raiz — referenciado? Funcional? Não verificado. |

### Scripts em `ops/` (apenas listados — não avaliados nesta rodada)
- `backup_diario.sh` — backup operacional
- `build_prod.ps1` — build Windows PowerShell
- `build_prod.sh` — build Linux shell
- `carga_conferencia.py` — carga de dados Python
- `e2e_conf_estado.sh` — teste E2E shell
- `e2e_escalas.sh` — teste E2E shell
- `e2e_producao.sh` — teste E2E produção
- `e2e_rascunho.sh` — teste E2E rascunho
- `restore_test.sh` — restore test
- `verifica_v7.py` — verificação Python
- `watchdog_sci.sh` — watchdog

### Documentos .md na raiz (fora do escopo desta limpeza)
`CHANGELOG.md`, `HANDOFF.md`, `HANDOFF_FRONTEND.md`, `PLANO_DECISOES.md`, `README.md`, `ROADMAP.md`, `TECHNICAL_PLAN.md`, `bugfixes_report.md`, `v1_2_fase1_report.md`

### Artefatos de build (.gitignored)
- `sci` (binário compilado, 21MB) — gitignorado, presente no working tree

## LGPD

Nenhum arquivo `.db` ou `.log` encontrado no working tree. Nada a denunciar.

## Validação

`go build ./...` e `go test ./...` executados após as remoções. Resultados documentados no commit.