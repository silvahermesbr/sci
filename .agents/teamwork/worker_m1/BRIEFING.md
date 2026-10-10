# BRIEFING — 2026-10-10T15:30:09Z

## Mission
Implement Milestone M1: The 3 P0s of Security and Integrity (Scope isolation for -1 accounts, foto_base64 XSS hardening, atomic backup restore with migrations and rollback).

## 🔒 My Identity
- Archetype: teamwork_preview_worker_1
- Roles: implementer, qa, specialist
- Working directory: c:\Users\hermes\Documents\programming\sci\.agents\teamwork\worker_m1
- Original parent: 2c71df87-2cd2-441b-8edf-cef8253c7857
- Milestone: M1 (R1: Os 3 P0s de Segurança e Integridade)

## 🔒 Key Constraints
- Genuine implementation only, no cheating or hardcoded test returns.
- Scope guard `esc == -1` (non-admin accounts without group) must fail-closed: return 403 Forbidden on data routes, and `filtroGrupoSQL` must return ` AND 1 = 0`.
- Blind `foto_base64` against stored XSS: strict prefix, base64 validation, magic bytes check, max 512KB limit, frontend escaping.
- Restore backup atomicity: swap/rename to `sci.db`, clean WAL/SHM, run all migrations up to current version, rollback on failure.
- Full regression tests in `r1_test.go`.
- `go vet ./...`, `go build ./...`, and `go test -count=1 ./...` must be 100% green.

## Current Parent
- Conversation ID: 2c71df87-2cd2-441b-8edf-cef8253c7857
- Updated: 2026-10-10T15:30:09Z

## Task Summary
- **What to build**: M1 fixes for Scope -1, foto_base64 sanitization/validation, and Backup restore atomicity + migrations + rollback.
- **Success criteria**: All scope leakages closed; XSS blocked on backend and frontend; backup restore cleanly migrates and rolls back on failure; tests passing.
- **Interface contracts**: PROJECT.md & survey handoffs.
- **Code layout**: Go backend in root package (`*.go`), Frontend in `web/`.

## Key Decisions Made
- [TBD pending survey review]

## Artifact Index
- `.agents/teamwork/worker_m1/DISPATCH.md` — Assignment
- `.agents/teamwork/worker_m1/BRIEFING.md` — Memory & status
- `.agents/teamwork/worker_m1/progress.md` — Heartbeat & steps
- `.agents/teamwork/worker_m1/handoff.md` — Final completion report

## Change Tracker
- **Files modified**: None yet
- **Build status**: Untested
- **Pending issues**: None

## Quality Status
- **Build/test result**: Pending
- **Lint status**: Pending
- **Tests added/modified**: Pending `r1_test.go`
