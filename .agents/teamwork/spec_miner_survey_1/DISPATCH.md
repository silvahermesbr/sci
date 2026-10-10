## 2026-10-10T15:12:30Z
From: 2c71df87-2cd2-441b-8edf-cef8253c7857
Role: teamwork_preview_spec_miner_1

You are teamwork_preview_spec_miner_1, an authoritative Specification Investigator.
Your working directory is: c:\Users\hermes\Documents\programming\sci\.agents\teamwork\spec_miner_survey_1

MANDATORY FIRST STEP:
Read the Original Request: c:\Users\hermes\Documents\programming\sci\.agents\teamwork\ORIGINAL_REQUEST.md

Objective:
Extract and document the complete, rigorous specification of Onda v1.5.4 from the project's documentation:
- docs/INSTRUCOES_AGENTES.md
- docs/ARQUITETURA.md
- docs/ROADMAP.md
- CHANGELOG.md (and any other relevant docs in docs/)

Extract precise requirements, acceptance criteria, and exact rules for:
1. R1: The 3 P0s of Security and Integrity
   - Item A (R-1): Scope -1 (accounts without group) across all data routes. Centralized guard, elimination of loose `esc > 0` checks.
   - Item B (R-2): `foto_base64` sanitization / stored XSS protection (MIME/base64 validation, size limits in backend, strict escaping in frontend).
   - Item C (R-3 / R-4): Atomic backup import (`server_admin.go`, `store.go`) with rename to sci.db, proper WAL cleanup, migration chain re-execution, and safe rollback.
2. R2: Field Bugs Reported by Command
   - Item D1: Zombie chief (`chefeComandaSetor` & invalidation of `usuarios.setor_id` when demoting sector chief).
   - Item D2: Protection of export/PDF/data routes against missing scope/role checks.
   - Item D3: Unification of conference ordering using identical COALESCE expression for all tag sources, signaling out-of-filter personnel in response.
3. R3: Remaining P1 Guards
   - R-5 (photo scope), R-6/R-15 (cascade deletion and NUKE), R-8 (archive/discard conference), R-9 (`hMudarContexto`), R-10 (`hUsuarioPapelDel/Add`), R-11 (Drive MIME/inline), R-14 (role messaging), R-16 (session invalidation on password change).
4. R4: CI Fortification (`ci.sh`, artifact cleanup).
5. R5: Documentation & Governance requirements (`docs/ARQUITETURA.md`, `docs/ROADMAP.md`, `CHANGELOG.md`).

Boundaries:
- Read-only! Do NOT write or modify any project code or doc files.
- Write your comprehensive findings to: c:\Users\hermes\Documents\programming\sci\.agents\teamwork\spec_miner_survey_1\handoff.md
- Update c:\Users\hermes\Documents\programming\sci\.agents\teamwork\spec_miner_survey_1\progress.md as you work.
- When finished, send a message to the orchestrator (caller) confirming completion and referencing the handoff path.
