# BRIEFING — 2026-10-10T15:11:00Z

## Mission
Execute Onda Completa v1.5.4 in the SCI Go system (P0 security fixes, field bugs, P1 guards, CI hardening, and governance).

## 🔒 My Identity
- Archetype: teamwork_preview_orchestrator
- Roles: orchestrator, user_liaison, human_reporter, successor
- Working directory: c:\Users\hermes\Documents\programming\sci\.agents\teamwork\teamwork_preview_orchestrator_1
- Original parent: sentinel
- Original parent conversation ID: bcfd13a3-a791-4533-b31f-1492e75dc70c

## 🔒 My Workflow
- **Pattern**: Project Pattern
- **Scope document**: c:\Users\hermes\Documents\programming\sci\PROJECT.md
1. **Decompose**: Survey full scope via 3 Explorers/Spec Miners, merge findings, decompose into Milestones (R1 to R5).
2. **Dispatch & Execute**:
   - Direct (iteration loop): For each milestone: Explorer(s) -> Worker -> Reviewers (2) + Challengers (2) + Auditor -> Gate check.
3. **On failure**: Retry -> Replace -> Skip (if non-critical) -> Redistribute -> Redesign.
4. **Succession**: At 16 spawns, write handoff.md, spawn successor.
- **Work items**:
  1. Survey and Scope Mapping [done]
  2. R1: 3 P0s de Segurança e Integridade (M1) [in-progress]
  3. R2: 3 Bugs de Campo Relatados pelo Comando (M2) [pending]
  4. R3: Guardas Remanescentes P1 (M3) [pending]
  5. R4: Fortalecimento de CI (M4) [pending]
  6. R5: Documentação e Governança (M5) [pending]
  7. Final Acceptance & Verification (M6) [pending]
- **Current phase**: 1 (Milestone M1 Execution)
- **Current focus**: Milestone M1 (P0 Security: Scope -1, XSS foto_base64, Atomic Backup Import/WAL)

## 🔒 Key Constraints
- NEVER write, modify, or create source code files directly.
- NEVER run build/test commands yourself — require workers to do so.
- NEVER investigate or explore the problem at the code level — dispatch Explorers for technical investigation.
- Audit is BINARY VETO — violation means failure, no exceptions.
- Never reuse a subagent after it has delivered its handoff — always spawn fresh.

## Current Parent
- Conversation ID: bcfd13a3-a791-4533-b31f-1492e75dc70c
- Updated: 2026-10-10T15:10:48Z

## Key Decisions Made
- Project Orchestrator initialized.
- Survey phase initiated with 3 parallel agents to thoroughly map specs in docs/INSTRUCOES_AGENTES.md, docs/ARQUITETURA.md, docs/ROADMAP.md and current codebase status.

## Team Roster
| Agent | Type | Work Item | Status | Conv ID |
|-------|------|-----------|--------|---------|
| spec_miner_survey_1 | teamwork_preview_spec_miner | Survey Docs & Specs | completed | 4011b303-7c00-4e8b-8f40-40b29389f716 |
| explorer_survey_1 | teamwork_preview_explorer | Survey R1 (P0s) & R2 (Field bugs) | completed | 5b42481f-3943-46c4-8b69-02f5396e9cfd |
| explorer_survey_2 | teamwork_preview_explorer | Survey R3 (P1s) & R4/CI | completed | 3fb7557f-518c-4bf1-8a47-f17a3fa2388b |
| worker_m1 | teamwork_preview_worker | Implement M1 P0 Security & Integrity | in-progress | 3f7fb3f2-9c77-4ec3-9007-c0d99868bcbe |

## Succession Status
- Succession required: no
- Spawn count: 4 / 16
- Pending subagents: 3f7fb3f2-9c77-4ec3-9007-c0d99868bcbe
- Predecessor: none
- Successor: not yet spawned

## Active Timers
- Heartbeat cron: 2c71df87-2cd2-441b-8edf-cef8253c7857/task-12
- Safety timer: none

## Artifact Index
- ORIGINAL_REQUEST.md — c:\Users\hermes\Documents\programming\sci\.agents\teamwork\ORIGINAL_REQUEST.md
- DISPATCH.md — c:\Users\hermes\Documents\programming\sci\.agents\teamwork\teamwork_preview_orchestrator_1\DISPATCH.md
- progress.md — c:\Users\hermes\Documents\programming\sci\.agents\teamwork\teamwork_preview_orchestrator_1\progress.md
- BRIEFING.md — c:\Users\hermes\Documents\programming\sci\.agents\teamwork\teamwork_preview_orchestrator_1\BRIEFING.md
- PROJECT.md — c:\Users\hermes\Documents\programming\sci\PROJECT.md
