# Progress — teamwork_preview_explorer_1

Last visited: 2026-10-10T15:27:00Z

## Status
- [x] Initialized DISPATCH.md, BRIEFING.md, progress.md
- [x] Read docs/INSTRUCOES_AGENTES.md, docs/ARQUITETURA.md, docs/ROADMAP.md, docs/auditoria-completa-1010.md
- [x] Investigate R1 Item A (R-1): Scope `esc == -1`, loose `esc > 0` checks, central guard
  - Traced `escopoDoUsuario` in `server_grupos.go:202`
  - Located ~35 handlers with loose `esc > 0`, `esc <= 0`, `? <= 0` fall-throughs
  - Identified defense-in-depth strategy: central guard `exigeEscopo`, `filtroGrupoSQL` with `AND 1=0`, SQL `? = 0`
- [x] Investigate R1 Item B (R-2): `foto_base64` MIME, size ceiling, frontend escaping/sanitization
  - Backend write sinks: `server_pessoal.go:343` and `server_pessoal.go:994-1003`
  - Frontend unescaped sinks: `core.js:580`, `views_gestao.js:548, 683, 759, 2535, 2637, 2707`, `ui_helpers.js:251, 386, 462`
  - Validation requirements: data URI prefix, base64 decode, 512 KB limit, image magic bytes
- [x] Investigate R1 Item C (R-3 / R-4): Backup import, atomic rename, WAL cleanup, migration chain reopening, rollback
  - `server_admin.go:259-274`: never renames `.novo` -> `sci.db`, deletes WAL of active connection, broken rollback
  - `store.go:1372-1402`: `ReabrirComArquivo` only runs up to v12, omitting v13-v43
  - Defined unified migration runner and atomic rename swap sequence
- [x] Investigate R2 Item D1: `chefeComandaSetor`, `usuarios.setor_id`, sector chief demotion
  - `onda_0510_conf_escopo.go:145-164`: fallbacks to `u.SetorID` and `pessoas.setor_id`
  - `onda_0510_escalas.go:297, 315`: `hGrupoDestituirChefe` never clears `usuarios.setor_id`
  - `onda_0510_itens79.go:175-184`: `hSetoresAgregado` counts chiefs via `usuarios.setor_id` instead of `chefe_setores`
- [x] Investigate R2 Item D2: Export/PDF/data routes missing scope/role guards
  - `server_escalas.go:1703, 1716-1718`: `/api/escalas/pdf`, `/api/escalas/relatorio-dia.pdf`, `/api/escalas/minhas` use `auth(false)`
  - `server_material.go:2184, 2188, 2209`: `/api/material/inventario/pdf`, `/api/material/cautelas/{id}/recibo.pdf`, `/api/material/conferencias/{id}/pronto.pdf`
  - `server_conferencia.go:1593`: `/api/conferencia/{id}/relatorio.pdf`
  - `server_relatorios.go:414-424`: entire reports and export suite on `auth(false)`
  - `server_pessoal.go:468, 1350`: `/api/pessoas/{id}/pdf`, `/api/pessoas/{id}/qr`
- [x] Investigate R2 Item D3: Conference ordering logic, COALESCE for tags, filtering/signaling
  - `onda_0910_conf_antiguidade.go:183-201`: `hSetorPreFechamento` orders only by `fu.antiguidade`, ignoring `fu_u` and `fu_up`
  - `server_conferencia.go:985`: `hConferenciaGet` orders only by `situacao, p.nome_guerra`, ignoring seniority
  - `server_conferencia.go:1053`: `montarLancamentosPDFConferencia` orders only by `cf2.caminho` on `p.funcao_id`
  - Silent exclusion: `hConferenciaIniciar` and `hConferenciaDespachar` do not return `sem_tag`
- [x] Compile comprehensive handoff.md
- [x] Send handoff message to parent
