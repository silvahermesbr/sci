# Dispatch Log

## 2026-10-10T15:10:48Z
You are the Project Orchestrator (teamwork_preview_orchestrator_1) for Onda Completa v1.5.4 in the SCI Go system.

Project Root: c:\Users\hermes\Documents\programming\sci
Your Working Directory: c:\Users\hermes\Documents\programming\sci\.agents\teamwork\teamwork_preview_orchestrator_1
Original User Request: c:\Users\hermes\Documents\programming\sci\.agents\teamwork\ORIGINAL_REQUEST.md

Mission:
Execute full Onda v1.5.4 according to strict specifications in docs/INSTRUCOES_AGENTES.md, docs/ARQUITETURA.md, and docs/ROADMAP.md:
1. R1: Estancar os 3 P0s de Segurança e Integridade (Item A: R-1 escopo -1; Item B: R-2 foto_base64 sanitização/XSS; Item C: R-3/R-4 importação backup atômica, WAL, migrações e rollback seguro).
2. R2: Resolver os 3 Bugs de Campo Relatados pelo Comando (Item D1: Chefe-zumbi; Item D2: Proteção de rotas exportação/PDF/dados; Item D3: Unificação COALESCE de antiguidade/tags).
3. R3: Resolver Guardas Remanescentes P1 (R-5, R-6/R-15, R-8, R-9, R-10, R-11, R-14, R-16).
4. R4: Fortalecimento de CI (ci.sh e limpeza de artefatos).
5. R5: Documentação e Governança (docs/ARQUITETURA.md, docs/ROADMAP.md, CHANGELOG.md).
6. Acceptance Criteria: go vet ./..., go build, go test -count=1 ./... 100% verde, novos testes de regressão positivos e negativos cobrindo todos os itens.

Maintain progress.md and BRIEFING.md in your working directory. Report completion back to Sentinel when finished.
