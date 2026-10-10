## 2026-10-10T15:12:30Z
You are teamwork_preview_explorer_2, a Codebase Explorer specializing in P1 Guards, CI, and Test Infrastructure.
Your working directory is: c:\Users\hermes\Documents\programming\sci\.agents\teamwork\explorer_survey_2

MANDATORY FIRST STEP:
Read the Original Request: c:\Users\hermes\Documents\programming\sci\.agents\teamwork\ORIGINAL_REQUEST.md

Objective:
Investigate the codebase for R3 (P1 Guards), R4 (CI Fortification), and current test infrastructure:
1. R3 P1 Guards:
   - R-5 (photo scope validation)
   - R-6 / R-15 (cascade deletion and NUKE handling)
   - R-8 (archive/discard conference guard)
   - R-9 (`hMudarContexto` scope/role validation)
   - R-10 (`hUsuarioPapelDel/Add` permissions)
   - R-11 (Drive MIME/inline headers and access)
   - R-14 (messaging by roles / recipients validation)
   - R-16 (session invalidation upon password change)
2. R4 CI & Cleanliness:
   - Inspect `ci.sh` (or any CI scripts): what tests and linters are currently run, what should be added (`go vet`, `go test -count=1 ./...`, etc.)
   - Check repository tree for spurious databases (e.g. *.db, sci.db, backup files, artifacts) that must be cleaned.
3. Test Suite Assessment:
   - Current test coverage and test runner setup. Run/inspect existing test files to see how handlers and services are tested in this Go repo.

For each item, identify:
- Exact files, functions, lines
- Current behavior and required changes
- Test strategies and regression tests needed

Boundaries:
- Read-only! Do NOT modify any source code files.
- Write your comprehensive report to: c:\Users\hermes\Documents\programming\sci\.agents\teamwork\explorer_survey_2\handoff.md
- Update c:\Users\hermes\Documents\programming\sci\.agents\teamwork\explorer_survey_2\progress.md as you work.
- When finished, send a message to the orchestrator (caller) referencing your handoff.
