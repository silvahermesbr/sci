# Project: Onda Completa v1.5.4 (SCI Go)

## Architecture
O SCI é um sistema monolítico em Go com frontend SPA Vanilla JS (embutido via `//go:embed web`).
O armazenamento é SQLite local em modo WAL gerenciado por `*Store` (`store.go`).
A autenticação e autorização utilizam sessões em banco com `Usuario` no contexto HTTP, papéis (`admin`, `gerente`, `chefe_setor`, `operador`, etc.) e controle de escopo multitenant (`GrupoID`):
- `escopo == 0`: administrador global (acesso a todos os grupos).
- `escopo > 0`: usuário vinculado ao grupo `escopo` (e subunidades subordinadas quando aplicável).
- `escopo == -1`: conta válida autenticada sem grupo atribuído (`u.GrupoID == nil`). **MANDATÓRIO**: Contas com `escopo == -1` NUNCA devem ter acesso a dados ou entidades de grupo. Devem receber `403 Forbidden` imediato em rotas de dados ou listas vazias seguras.

## Feature Inventory
| # | Feature | Description | Milestone | Source |
|---|---------|-------------|-----------|--------|
| F-01 | R-1 (Item A) | Fechar escopo `-1` (contas sem grupo) em todas as rotas de dados via guarda central `exigeEscopo()` e remoção de bifurcações `esc > 0` | M1 (R1) | Survey F-01 |
| F-02 | R-2 (Item B) | Blindar `foto_base64` contra XSS armazenado (validação MIME/base64/512KB no backend e escape rigoroso nos sinks do frontend) | M1 (R1) | Survey F-02 |
| F-03 | R-3/R-4 (Item C) | Swap atômico de backup para `sci.db`, limpeza correta de WAL/SHM, cadeia completa de migrações v2..v43 e rollback seguro | M1 (R1) | Survey F-03 |
| F-04 | D1 (Chefe-zumbi) | Erradicar chefe-zumbi: remover fallbacks de `chefeComandaSetor` e invalidar `usuarios.setor_id` ao destituir chefia de setor | M2 (R2) | Survey F-04 |
| F-05 | D2 (Rotas PDF/Dados) | Blindar rotas de exportação/PDF/dados com guardas de módulo e papel (`reservaAuth`, `authMaterial`, checagem de escopo rígida) | M2 (R2) | Survey F-05 |
| F-06 | D3 (Antiguidade/Tags) | Unificar ordenação da conferência usando a mesma expressão `COALESCE` para todas as fontes de tags e retornar `sem_tag` em `hConferenciaIniciar` | M2 (R2) | Survey F-06 |
| F-07 | R-5 (Foto LGPD) | Restringir `GET /api/usuarios/{id}/foto` ao próprio grupo/subordinados e remover cabeçalhos de cache público | M3 (R3) | Survey F-07 |
| F-08 | R-6/R-15 (Cascata/NUKE) | Cascata atômica completa no NUKE de grupos (incluindo as 10 tabelas dependentes v1.5), exclusão de setores e limpeza de `conferencia_escalas` | M3 (R3) | Survey F-08 |
| F-09 | R-8 (Conf Arquivar/Descartar) | Restringir arquivamento e descarte de conferência estritamente a gerente e encarregado de pessoal designado (bloquear operador) | M3 (R3) | Survey F-09 |
| F-10 | R-9 (Contexto Setor) | Validar em `hMudarContexto` que `setor_id` pertence ao grupo do papel ativo e não reescrever a coluna global `usuarios.setor_id` | M3 (R3) | Survey F-10 |
| F-11 | R-10 (Papéis Fail-Open) | Bloquear fail-open em `hUsuarioPapelDel` (allowlist estrita: admin/gerente) e validar escopo do alvo em `hUsuarioPapelAdd` | M3 (R3) | Survey F-11 |
| F-12 | R-11 (Drive MIME/Headers) | Allowlist estrita de MIME para inline (`image/*`, `application/pdf`), sanitização de filename em `Content-Disposition` e `nosniff` | M3 (R3) | Survey F-12 |
| F-13 | R-14 (Mensagens Hierarquia) | Validar hierarquia de envio em `hMensagensEnviar` mesmo para contas sem grupo ou operadores | M3 (R3) | Survey F-13 |
| F-14 | R-16 (Sessões Troca Senha) | Revogar sessões ativas no banco ao trocar senha em `hTrocarSenha` e `hUsuarioSenha` | M3 (R3) | Survey F-14 |
| F-15 | R4 (CI & Limpeza) | Atualizar `ci.sh` com `go vet ./...` e `go test -count=1 ./...`; remover `dados.tar.gz` e bancos espúrios | M4 (R4) | Survey F-15 |
| F-16 | R5 (Governança/Docs) | Atualizar `docs/ARQUITETURA.md`, `docs/ROADMAP.md`, `CHANGELOG.md` e realizar bump de cache-bust para `v371` | M5 (R5) | Survey F-16 |

## Milestones
| # | Name | Scope | Dependencies | Status |
|---|------|-------|-------------|--------|
| M1 | P0s de Segurança e Integridade (R1) | F-01 (Escopo -1), F-02 (XSS foto_base64), F-03 (Backup atômico/migrações) | Survey concluído | PLANNED |
| M2 | Bugs de Campo Relatados (R2) | F-04 (Chefe-zumbi), F-05 (Rotas PDF/Dados), F-06 (Antiguidade/Tags COALESCE & sem_tag) | M1 | PLANNED |
| M3 | Guardas Remanescentes P1 (R3) | F-07 (R-5), F-08 (R-6/R-15), F-09 (R-8), F-10 (R-9), F-11 (R-10), F-12 (R-11), F-13 (R-14), F-14 (R-16) | M1, M2 | PLANNED |
| M4 | Fortalecimento de CI e Limpeza (R4) | F-15 (ci.sh e expurgo de dados.tar.gz) | M1, M2, M3 | PLANNED |
| M5 | Documentação e Governança (R5) | F-16 (ARQUITETURA.md, ROADMAP.md, CHANGELOG.md, cache-bust bump) | M1..M4 | PLANNED |
| M6 | Aceite Final e Verificação Integral | go vet ./..., go build, go test -count=1 ./... 100% verde + auditoria forense | M1..M5 | PLANNED |

## Interface Contracts
### Central Scope Guard (`helpers.go` / `server_grupos.go`)
- `func (a *App) exigeEscopo(u *Usuario) (int64, error)`:
  - Se `u == nil`: retorna `(-1, err)` -> `401 Unauthorized`
  - Se `u.Papel == "admin"`: retorna `(0, nil)`
  - Se `u.GrupoID == nil`: retorna `(-1, ErrContaSemGrupo)` -> handlers retornam `403 Forbidden` (`{"ok":false,"erro":"conta sem grupo definido"}`)
  - Caso contrário: retorna `(*u.GrupoID, nil)`
- `filtroGrupoSQL(escopo int64, alias string) string`:
  - Se `escopo == 0`: retorna `""`
  - Se `escopo > 0`: retorna ` AND alias.grupo_id = ?`
  - Se `escopo < 0`: retorna ` AND 1 = 0` (bloqueio rigoroso fail-closed)
- `filtroGrupoArgs(escopo int64) []any`:
  - Se `escopo <= 0`: retorna `nil`
  - Se `escopo > 0`: retorna `[]any{escopo}`

### Foto Base64 Validation (`server_pessoal.go`)
- `validarFotoBase64(s string) (mime string, data []byte, err error)`:
  - Tamanho <= 512 * 1024 bytes (raw base64 string)
  - Prefixo obrigatório: `data:image/(png|jpeg|webp);base64,`
  - Decodificação Base64 rigorosa
  - Magic bytes: PNG (`\x89PNG`), JPEG (`\xFF\xD8\xFF`), WebP (`RIFF...WEBP`)

### Backup Swap & Migrations (`server_admin.go` & `store.go`)
- `ReabrirComArquivo(caminho string) error`:
  - Executa cadeia de migrações unificada idêntica a `AbrirStore` (`migrar()` até `migrarV43()`).
  - Em caso de erro, garante rollback para o arquivo de segurança anterior preservando o nome canônico `sci.db`.
  - Remoção limpa de arquivos `-wal` e `-shm` pré-existentes.

### Chefe de Setor Authority (`onda_0510_conf_escopo.go` & `onda_0510_escalas.go`)
- `chefeComandaSetor(u *Usuario, setorID int64) bool`:
  - Autoridade ÚNICA via `SELECT COUNT(*) FROM chefe_setores WHERE usuario_id = ? AND setor_id = ?`.
  - Eliminação total dos fallbacks legados `u.SetorID` e `pessoas.setor_id`.
- `hGrupoDestituirChefe`:
  - Remove linha de `chefe_setores`.
  - Invalida `UPDATE usuarios SET setor_id = NULL WHERE id = req.UsuarioID AND setor_id = req.SetorID`.
- `hGrupoNomearChefe`:
  - Se o setor já tinha chefe anterior A, invalida `UPDATE usuarios SET setor_id = NULL WHERE id = A AND setor_id = req.SetorID`.

## Code Layout
- Backend handlers: `server_grupos.go`, `server_conferencia.go`, `server_pessoal.go`, `server_material.go`, `server_escalas.go`, `server_relatorios.go`, `server_admin.go`, `server_realtime.go`, `mensagens.go`, `drive.go`, `helpers.go`, `relatorio.go`, `onda_*.go`.
- Store e Migrações: `store.go`, `store_migracoes.go` (se aplicável).
- Frontend: `web/core.js`, `web/views_gestao.js`, `web/ui_helpers.js`, `web/index.html`, `web/lazy.js`.
- Scripts & Config: `ci.sh`, `main.go`.
- Testes: `*_test.go` na raiz do repositório.
