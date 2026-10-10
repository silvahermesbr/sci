# BRIEFING — 2026-10-10T15:27:00Z

## Mission
Investigate Go backend and frontend for R1 (P0 Security/Integrity: R-1 esc==-1, R-2 foto_base64 XSS/MIME, R-3/R-4 backup import/rollback) and R2 (Field Bugs: D1 chefe-zumbi, D2 export/PDF routes auth, D3 conference ordering/COALESCE tags).

## 🔒 My Identity
- Archetype: explorer
- Roles: Codebase Explorer specializing in P0 Security and Field Bugs
- Working directory: c:\Users\hermes\Documents\programming\sci\.agents\teamwork\explorer_survey_1
- Original parent: 2c71df87-2cd2-441b-8edf-cef8253c7857
- Milestone: v1.5.4 investigation

## 🔒 Key Constraints
- Read-only investigation — do NOT modify any source code files
- Focus on R1 (Items A, B, C) and R2 (Items D1, D2, D3)
- Deliver comprehensive handoff.md following 5-component protocol
- Update progress.md regularly

## Current Parent
- Conversation ID: 2c71df87-2cd2-441b-8edf-cef8253c7857
- Updated: 2026-10-10T15:13:00Z

## Investigation State
- **Explored paths**: `docs/ARQUITETURA.md`, `docs/ROADMAP.md`, `docs/INSTRUCOES_AGENTES.md`, `docs/auditoria-completa-1010.md`, `server_grupos.go`, `server_conferencia.go`, `server_pessoal.go`, `server_material.go`, `server_escalas.go`, `server_relatorios.go`, `server_catalogo.go`, `server_realtime.go`, `server_admin.go`, `store.go`, `helpers.go`, `onda_0510_conf_escopo.go`, `onda_0510_escalas.go`, `onda_0510_itens79.go`, `onda_0910_conf_antiguidade.go`, `onda_despacho.go`, `web/core.js`, `web/views_gestao.js`, `web/views_conf.js`, `ci.sh`.
- **Key findings**:
  1. R1-A (R-1): `escopoDoUsuario` returns `-1`. Loose `esc > 0`, `esc <= 0`, and SQL `? <= 0` allow unassigned users to bypass filters and access cross-unit data across ~35 handlers. Fix requires central guard (`exigeEscopo` / middleware), `filtroGrupoSQL` returning `AND 1=0` on `esc < 0`, and converting SQL `? <= 0` to `? = 0`.
  2. R1-B (R-2): Backend writes `foto_base64` without MIME, decoding, or size checks (`server_pessoal.go:343, 994-1003`). Frontend renders raw `<img src="${...}">` in `core.js:580`, `views_gestao.js:548, 683, 759, 2535, 2637, 2707`. Fix requires backend validation (data URI prefix, base64 decode, 512KB ceiling, image magic bytes) and `esc()` on all frontend sinks.
  3. R1-C (R-3/R-4): Backup import (`server_admin.go:259-274`) renames to `sci.db.novo` but never swaps with `sci.db`, deletes WAL of active connection, has broken rollback. `ReabrirComArquivo` (`store.go:1372-1402`) only migrates up to v12, omitting v13-v43. Fix requires atomic swap sequence, rollback with original backup, and single unified migration runner.
  4. R2-D1: `chefeComandaSetor` (`onda_0510_conf_escopo.go:145-164`) maintains legacy fallback to `usuarios.setor_id`. `hGrupoDestituirChefe` never clears `usuarios.setor_id`. `hSetoresAgregado` counts chiefs via `usuarios.setor_id`, displaying 2 chiefs. Fix: make `chefe_setores` the single source of truth, invalidate `usuarios.setor_id` on replacement/demotion, and align `hSetoresAgregado`.
  5. R2-D2: Multiple data and PDF export routes use `auth(false)` without role or scope checks (`/api/escalas/pdf`, `/api/escalas/relatorio-dia.pdf`, `/api/escalas/minhas`, `/api/material/inventario/pdf`, `/api/material/cautelas/{id}/recibo.pdf`, `/api/material/conferencias/{id}/pronto.pdf`, `/api/conferencia/{id}/relatorio.pdf`, `/api/relatorio*`, `/api/export*`, `/api/pessoas/{id}/pdf`, `/api/pessoas/{id}/qr`). Fix: apply module-appropriate guards (`reservaAuth`, `authMaterial`, `confAuth`, `guardaGestaoPessoal`).
  6. R2-D3: `hSetorPreFechamento` orders only by `fu.antiguidade` (ignoring `fu_u` and `fu_up`); `hConferenciaGet` orders by `situacao, nome` (never seniority); `montarLancamentosPDFConferencia` joins hierarchy only on `p.funcao_id`. Non-matching personnel are silently dropped when starting conferences without returning `sem_tag`. Fix: unified `COALESCE` across all 3 sources in all sorting queries, and return `sem_tag: []string` on conference start.
- **Unexplored areas**: None within the assigned R1/R2 scope.

## Key Decisions Made
- Structured exhaustive 5-component handoff report detailing exact lines, flaws, architectural implications, fix strategies, and comprehensive test matrix.

## Artifact Index
- `handoff.md` — Comprehensive 5-component handoff report
- `progress.md` — Liveness and progress tracking
- `DISPATCH.md` — Incoming dispatch log
- `BRIEFING.md` — Persistent situational awareness
