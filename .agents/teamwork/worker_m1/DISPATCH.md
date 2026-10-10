## 2026-10-10T15:30:09Z

You are teamwork_preview_worker_1, an expert Go software engineer and security specialist.
Your working directory is: c:\Users\hermes\Documents\programming\sci\.agents\teamwork\worker_m1

MANDATORY FIRST STEP:
Read the Original Request: c:\Users\hermes\Documents\programming\sci\.agents\teamwork\ORIGINAL_REQUEST.md
Also read:
- Project Plan: c:\Users\hermes\Documents\programming\sci\.agents\teamwork\teamwork_preview_orchestrator_1\PROJECT.md
- Specification Survey: c:\Users\hermes\Documents\programming\sci\.agents\teamwork\spec_miner_survey_1\handoff.md
- Codebase Survey: c:\Users\hermes\Documents\programming\sci\.agents\teamwork\explorer_survey_1\handoff.md

Objective:
Implement Milestone M1 (R1: Os 3 P0s de Segurança e Integridade):

1. Item A (R-1): Fechar a classe de escopo `-1` (contas sem grupo) em todas as rotas de dados:
   - Centralizar guarda de escopo em `helpers.go` e `server_grupos.go` (`func (a *App) exigeEscopo(u *Usuario) (int64, error)` ou helper equivalente) retornando erro 403 Forbidden (`{"ok":false,"erro":"conta sem grupo definido"}`) para contas sem grupo (`u.GrupoID == nil` quando `u.Papel != "admin"`).
   - Ajustar `filtroGrupoSQL`: para `escopo < 0`, retornar ` AND 1 = 0` (fail-closed absoluto). Para `escopo == 0`, `""`. Para `escopo > 0`, ` AND alias.grupo_id = ?`.
   - Eliminar todas as bifurcações frouxas `esc > 0` que caíam no ramo `else` desprotegido em: `server_grupos.go` (`hGruposList`), `server_conferencia.go` (`hConferenciaList`, `hConferenciaGet`, `hConferenciaPDF`, `hConferenciaFechar`, `hConferenciaArquivar`, `hConferenciaSetorConcluir`, `hConferenciaSetorReabrir`, `hComentariosAdd`, `hComentariosList`), `server_realtime.go` (`hConferenciaStream`), `server_pessoal.go` (`pessoasTodas`, `hPessoaFicha`, `hPessoaComentarios`, `hPessoaPDF`, `hPessoaQRCode`), `server_material.go` (`hMaterialItensList`, `hMaterialCautelasList`, `hMaterialInventarioPDF`, handlers por item), `server_escalas.go` (`hEscalasLimparDia`, handlers com `? <= 0 OR grupo_id = ?`), `server_relatorios.go` (`hExportCSV`, `hExportarDados`).
   - Contas com `esc == -1` NUNCA devem ver dados de outros grupos nem apagar turnos/dados em consultas `? <= 0`.

2. Item B (R-2): Blindar `foto_base64` contra XSS armazenado:
   - No backend (`server_pessoal.go:343` em `hUsuarioAtualizar` e `:994-1003` em `hPerfilSet`): implementar validação rigorosa (`validarFotoBase64`):
     - Prefixo estrito: `data:image/png;base64,`, `data:image/jpeg;base64,`, `data:image/webp;base64,`
     - Decodificação base64 sem erros
     - Checagem de magic bytes para PNG, JPEG, WebP
     - Teto máximo de tamanho de 512 KB (rejeitar maiores com 400 Bad Request)
   - No frontend: escapar rigorosamente os sinks de renderização em `web/core.js` (linha ~580) e `web/views_gestao.js` (linhas ~548, 683, 759, 2535, 2637, 2707) utilizando `esc(usuario.foto_base64)` e validação segura antes de injetar no DOM.

3. Item C (R-3 / R-4): Corrigir importação de backup (`server_admin.go` e `store.go`):
   - Garantir swap com rename atômico definitivo para `sci.db` (não deixar em `.novo`).
   - Limpeza correta de WAL/SHM associados.
   - Reabertura garantindo execução de toda a cadeia de migrações (v2 até v43 unificadas com `AbrirStore`).
   - Rollback seguro em caso de falha de integridade ou migração, restaurando o backup de segurança para `sci.db`.

4. Testes de Regressão:
   - Criar `r1_test.go` cobrindo casos positivos e negativos para os itens A, B e C:
     - Matriz de escopo comprovando que conta com `esc == -1` recebe 403 Forbidden em rotas de dados.
     - Payload malicioso de XSS (`onerror=...`) e tamanho > 512KB em `foto_base64` rejeitados com 400 Bad Request.
     - Importação de backup restaura fielmente, renomeia para `sci.db` e migra integralmente até v43.

5. Verificação:
   - Executar `go vet ./...`
   - Executar `go build ./...`
   - Executar `go test -count=1 ./...`
   - Garantir 100% dos testes verdes.
