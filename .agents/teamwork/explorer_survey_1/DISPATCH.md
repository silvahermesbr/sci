## 2026-10-10T15:12:30Z

You are teamwork_preview_explorer_1, a Codebase Explorer specializing in P0 Security and Field Bugs.
Your working directory is: c:\Users\hermes\Documents\programming\sci\.agents\teamwork\explorer_survey_1

MANDATORY FIRST STEP:
Read the Original Request: c:\Users\hermes\Documents\programming\sci\.agents\teamwork\ORIGINAL_REQUEST.md

Objective:
Investigate the codebase (Go backend and frontend files) for R1 (P0s) and R2 (Field bugs):
1. R1 Item A (R-1): Locate all data handlers/routes checking scope (`esc`), where loose `esc > 0` checks exist, where accounts with `esc == -1` (no group) currently bypass or need central guard. Identify the central guard mechanism or where it should be implemented.
2. R1 Item B (R-2): Locate `foto_base64` handling in backend (validation, MIME checking, size ceiling) and frontend display (escaping/sanitization against stored XSS).
3. R1 Item C (R-3 / R-4): Locate backup import logic in `server_admin.go`, `store.go`, database file management, WAL cleanup, migration chain reopening, and rollback mechanisms.
4. R2 Item D1: Locate `chefeComandaSetor`, `usuarios.setor_id`, and sector chief demotion logic.
5. R2 Item D2: Locate export/PDF/data routes lacking adequate scope/role verification.
6. R2 Item D3: Locate conference ordering logic, COALESCE expressions for tags, and filtering/signaling of personnel.

For each item, identify:
- Exact files and line numbers
- Current implementation and flaws
- Expected fix strategy and architectural implications
- Existing tests and new tests needed

Boundaries:
- Read-only! Do NOT modify any source code files.
- Write your comprehensive report to: c:\Users\hermes\Documents\programming\sci\.agents\teamwork\explorer_survey_1\handoff.md
- Update c:\Users\hermes\Documents\programming\sci\.agents\teamwork\explorer_survey_1\progress.md as you work.
- When finished, send a message to the orchestrator (caller) referencing your handoff.
