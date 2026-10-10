# Handoff Report: Survey of R3 (P1 Guards), R4 (CI & Cleanliness), and Test Suite Assessment

**Agent**: `teamwork_preview_explorer_2` (Codebase Explorer)  
**Date**: 2026-10-10  
**Target Repository**: `c:\Users\hermes\Documents\programming\sci`  
**Working Directory**: `c:\Users\hermes\Documents\programming\sci\.agents\teamwork\explorer_survey_2`  

---

## 1. Observation

### 1.1 R3 P1 Guards

#### A. R-5: Photo Scope Validation (`hUsuarioFotoGet`)
- **File & Lines**: `server_pessoal.go:1015-1047`, route registered at `server_pessoal.go:1586`.
- **Verbatim Route**:
  ```go
  m.Handle("GET /api/usuarios/{id}/foto", a.auth(false, a.hUsuarioFotoGet))
  ```
- **Verbatim Handler**:
  ```go
  func (a *App) hUsuarioFotoGet(w http.ResponseWriter, r *http.Request) {
  	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
  	if err != nil {
  		jsonErro(w, http.StatusBadRequest, "id inválido")
  		return
  	}
  	var foto string
  	err = a.st.db.QueryRow(`SELECT COALESCE(foto_base64,'') FROM usuarios WHERE id = ?`, id).Scan(&foto)
  	if err != nil || foto == "" {
  		http.NotFound(w, r)
  		return
  	}
  ...
  	w.Header().Set("Content-Type", mime)
  	w.Header().Set("Cache-Control", "public, max-age=3600")
  	w.WriteHeader(http.StatusOK)
  	_, _ = w.Write(raw)
  }
  ```
- **Observed Behavior**:
  - `hUsuarioFotoGet` extracts `{id}` and immediately executes `SELECT COALESCE(foto_base64,'') FROM usuarios WHERE id = ?` without inspecting the authenticated user (`usuarioDoCtx(r)`).
  - Any authenticated user in any group can download the 1x1 photo avatar of any other user in any group by enumerating IDs (`/api/usuarios/1/foto`, `/api/usuarios/2/foto`, etc.).
  - Cache header is `public, max-age=3600`, which exposes personal biometric data to shared intermediary caches.

---

#### B. R-6 & R-15: Cascade Deletion, Sector Deletion, and NUKE Handling
- **Files & Lines**:
  - `server_grupos.go:140-186` (NUKE in `hGrupoExcluir`)
  - `ordem_0610_setores.go:25-130` (`hSetorExcluir`)
  - `server_conferencia.go:785-816` (`excluirArquivada`)
  - `store.go:2214-2224` (v32 `funcao_membros`), `:2248-2256` (v33 `conferencia_escalas`), `:2468-2475` (v35 `chefe_setores`), `:2578-2610` (v36 `material_conferencias`), `:1986-2025` (v27 `escala_modelos`, `setor_sugestoes`), `:1565-1595` (v22 `avisos`).
- **Verbatim NUKE Query List (`server_grupos.go:144-173`)**:
  ```go
  for _, q := range []string{
  	`UPDATE auditoria SET usuario_id = NULL WHERE usuario_id IN (SELECT id FROM usuarios WHERE grupo_id = ?)`,
  	`DELETE FROM presencas WHERE pessoa_id IN (SELECT id FROM pessoas WHERE grupo_id = ?)`,
  	`DELETE FROM comentarios WHERE pessoa_id IN (SELECT id FROM pessoas WHERE grupo_id = ?)`,
  	`DELETE FROM conferencias WHERE grupo_id = ?`,
  	`DELETE FROM escala_pessoas WHERE pessoa_id IN (SELECT id FROM pessoas WHERE grupo_id = ?)`,
  	`DELETE FROM escala_turnos WHERE grupo_id = ?`,
  	`DELETE FROM escala_tipos WHERE grupo_id = ?`,
  	`DELETE FROM material_cautelas WHERE item_id IN (SELECT id FROM material_itens WHERE grupo_id = ?)`,
  	`DELETE FROM material_itens WHERE grupo_id = ?`,
  	`DELETE FROM material_categorias WHERE grupo_id = ?`,
  	`DELETE FROM setores WHERE pai_id IS NOT NULL AND grupo_id = ?`,
  	`DELETE FROM funcoes WHERE pai_id IS NOT NULL AND grupo_id = ?`,
  	`DELETE FROM tags WHERE grupo_id = ?`,
  	`DELETE FROM destinos WHERE grupo_id = ?`,
  	`DELETE FROM setores WHERE grupo_id = ?`,
  	`DELETE FROM funcoes WHERE grupo_id = ?`,
  	`DELETE FROM conferencia_tipos WHERE grupo_id = ?`,
  }
  ```
- **Verbatim Foreign Keys Blocking NUKE (SQLite `FOREIGN KEY constraint failed`)**:
  1. `conferencia_escalas`: `conferencia_id INTEGER NOT NULL REFERENCES conferencias(id)` without CASCADE (`store.go:2250`).
  2. `chefe_setores`: `grupo_id REFERENCES grupos(id)`, `setor_id UNIQUE REFERENCES setores(id)`, `usuario_id REFERENCES usuarios(id)` without CASCADE (`store.go:2470-2472`).
  3. `funcao_membros`: `funcao_id REFERENCES funcoes(id)`, `grupo_id REFERENCES grupos(id)`, `usuario_id REFERENCES usuarios(id)` without CASCADE (`store.go:2216-2218`).
  4. `avisos`: `grupo_id REFERENCES grupos(id)`, `autor_usuario_id REFERENCES usuarios(id)`, `autor_papel_id REFERENCES usuario_papeis(id)` without CASCADE (`store.go:1569-1571`).
  5. `material_conferencias`: `grupo_id REFERENCES grupos(id)`, `setor_id REFERENCES setores(id)`, `aberta_por REFERENCES usuarios(id)` without CASCADE (`store.go:2580-2584`).
  6. `escala_modelos`: `grupo_id REFERENCES grupos(id)` without CASCADE (`store.go:1988`).
  7. `setor_sugestoes`: `grupo_id REFERENCES grupos(id)`, `autor_id REFERENCES usuarios(id)` without CASCADE (`store.go:2015-2017`).
  8. `grupo_vinculos`: `superior_id REFERENCES grupos(id)`, `subordinado_id REFERENCES grupos(id)` (`store.go:470-471`).
  9. `usuario_papeis`: `grupo_id REFERENCES grupos(id)` (`store.go:1454`).
  10. `mensagens`: `remetente_papel_id REFERENCES usuario_papeis(id)`, `grupo_id REFERENCES grupos(id)` (`store.go:1465`).
- **Verbatim Sector Deletion (`ordem_0610_setores.go:96-119`)**:
  ```go
  tx.Exec(`UPDATE pessoas SET setor_id = NULL ... WHERE setor_id = ?`, id)
  tx.Exec(`UPDATE usuarios SET setor_id = NULL WHERE setor_id = ?`, id)
  tx.Exec(`DELETE FROM setores WHERE id = ?`, id)
  ```
  - `chefe_setores` has `setor_id INTEGER NOT NULL UNIQUE REFERENCES setores(id)` without CASCADE (`store.go:2471`).
  - `material_itens` has `setor_id INTEGER REFERENCES setores(id)` without CASCADE (`store.go:2512`).
  - `material_conferencias` has `setor_id INTEGER REFERENCES setores(id)` without CASCADE (`store.go:2581`).
  - If a sector has an appointed chief in `chefe_setores`, items assigned in `material_itens`, or past material conferences in `material_conferencias`, `DELETE FROM setores` aborts with `FOREIGN KEY constraint failed`.
- **Verbatim `excluirArquivada` (R-15, `server_conferencia.go:799-803`)**:
  ```go
  for _, q := range []string{
  	`DELETE FROM comentarios WHERE conferencia_id = ?`,
  	`DELETE FROM presencas WHERE conferencia_id = ?`,
  	`DELETE FROM conferencias WHERE id = ?`,
  }
  ```
  - Compare with `hConferenciaDescartar` (`server_conferencia.go:914-919`), which includes:
    ```go
    `DELETE FROM comentarios WHERE conferencia_id = ?`,
    `DELETE FROM presencas WHERE conferencia_id = ?`,
    `DELETE FROM conferencia_escalas WHERE conferencia_id = ?`,
    `DELETE FROM conferencias WHERE id = ?`,
    ```
  - `excluirArquivada` omits `DELETE FROM conferencia_escalas WHERE conferencia_id = ?`, leaving `conferencia_escalas` blocking deletion or becoming orphaned.

---

#### C. R-8: Archive/Discard Conference Role Guard
- **Files & Lines**:
  - `server_conferencia.go:741-774` (`hConferenciaArquivar`)
  - `server_conferencia.go:882-930` (`hConferenciaDescartar`)
  - `server_conferencia.go:1576, 1586` (routes wrapped with `confAuth`)
  - `onda_0510_conf_escopo.go:133-137` (`authConf` definition)
- **Verbatim Route & Middleware**:
  ```go
  m.Handle("POST /api/conferencia/{id}/arquivar", confAuth(a.hConferenciaArquivar))
  m.Handle("DELETE /api/conferencia/{id}", confAuth(a.hConferenciaDescartar))
  
  // onda_0510_conf_escopo.go:135
  func (a *App) authConf(prox http.HandlerFunc) http.Handler {
  	return a.authConfCom([]string{"gerente", "operador"}, prox)
  }
  ```
- **Verbatim Handlers Scope Checks**:
  In `hConferenciaArquivar`:
  ```go
  if esc := escopoDoUsuario(u); esc > 0 {
  	if gid == nil || *gid != esc {
  		jsonErro(w, http.StatusForbidden, "conferência de outro grupo")
  		return
  	}
  } else if u.Papel != "admin" && u.Papel != "gerente" {
  	jsonErro(w, http.StatusForbidden, "sem acesso")
  	return
  }
  ```
  In `hConferenciaDescartar`:
  ```go
  if esc := escopoDoUsuario(u); esc > 0 {
  	if gid == nil || *gid != esc {
  		jsonErro(w, http.StatusForbidden, "conferência de outro grupo")
  		return
  	}
  } else if u.Papel != "admin" {
  	jsonErro(w, http.StatusForbidden, "sem acesso")
  	return
  }
  ```
- **Observed Behavior**:
  - `confAuth` allows roles `{"gerente", "operador"}` and designated encarregado/auxiliar de pessoal.
  - When `esc > 0`, an `operador` passes `hConferenciaArquivar` and `hConferenciaDescartar`.
  - In contrast, `hConferenciaFechar` (`server_conferencia.go:524`) and `hConferenciaDespachar` (`onda_despacho.go:167`) explicitly enforce:
    ```go
    if u.Papel != "gerente" && !a.ehEncarregado(u) && !a.ehAuxiliarDePessoal(u) {
        jsonErro(w, http.StatusForbidden, "...")
        return
    }
    ```
  - An operator can archive closed conferences or discard (delete) open conferences, violating doctrine where archiving/discarding is restricted to `gerente` and `encarregado/auxiliar de pessoal`.

---

#### D. R-9: `hMudarContexto` Scope and Role Validation
- **File & Lines**: `mensagens.go:30-80`
- **Verbatim Handler**:
  ```go
  // Se setor_id foi passado, validar
  if req.SetorID != nil && *req.SetorID > 0 {
  	var setorValido int
  	_ = a.st.db.QueryRow(`SELECT COUNT(*) FROM setores WHERE id = ? AND ativo = 1`, *req.SetorID).Scan(&setorValido)
  	if setorValido == 0 {
  		jsonErro(w, http.StatusBadRequest, "setor inválido ou inativo")
  		return
  	}
  } else if papel == "chefe_setor" && grupoID != nil {
  	var firstSid int64
  	if e := a.st.db.QueryRow(`SELECT cs.setor_id FROM chefe_setores cs JOIN setores s ON s.id = cs.setor_id WHERE cs.usuario_id = ? AND cs.grupo_id = ? ORDER BY s.nome ASC LIMIT 1`, u.ID, *grupoID).Scan(&firstSid); e == nil {
  		req.SetorID = &firstSid
  	}
  }
  
  // Atualizar a sessão ativa
  h := sha256.Sum256([]byte(c.Value))
  hash := hex.EncodeToString(h[:])
  _, err = a.st.db.Exec(`UPDATE sessoes SET papel_ativo_id = ?, setor_ativo_id = ? WHERE token_hash = ?`, req.PapelID, req.SetorID, hash)
  if err != nil {
  	jsonErro(w, http.StatusInternalServerError, "falha ao atualizar contexto da sessão")
  	return
  }
  if req.SetorID != nil && *req.SetorID > 0 {
  	_, _ = a.st.db.Exec(`UPDATE usuarios SET setor_id = ? WHERE id = ?`, *req.SetorID, u.ID)
  }
  ```
- **Observed Behavior**:
  1. `SELECT COUNT(*) FROM setores WHERE id = ? AND ativo = 1` checks if the sector exists in the system, but does NOT check if `setor_id` belongs to `grupoID` of the active `usuario_papeis`. A user can select a sector belonging to a completely different group.
  2. If the active role is `chefe_setor`, it does NOT verify whether the user actually commands that sector in `chefe_setores` when `req.SetorID` is provided explicitly.
  3. `UPDATE usuarios SET setor_id = ? WHERE id = ?` mutates the user's permanent account `usuarios.setor_id` whenever they change active session context, corrupting user profiles and enabling zombie command powers.

---

#### E. R-10: `hUsuarioPapelDel` and `hUsuarioPapelAdd` Permissions
- **File & Lines**:
  - `mensagens.go:84-140` (`hUsuarioPapelAdd`)
  - `mensagens.go:240-285` (`hUsuarioPapelDel`)
- **Verbatim `hUsuarioPapelDel` (`mensagens.go:245-270`)**:
  ```go
  u := usuarioDoCtx(r)
  if u.Papel == "operador" {
  	jsonErro(w, http.StatusForbidden, "operador não gerencia papéis")
  	return
  }
  ...
  // Se o gerente está excluindo, garantir que o papel pertence ao seu grupo e é operador
  if u.Papel == "gerente" {
  	var pGrupoID *int64
  	var pPapel string
  	err := a.st.db.QueryRow(`SELECT grupo_id, papel FROM usuario_papeis WHERE id = ? AND usuario_id = ?`, papelID, usuarioID).Scan(&pGrupoID, &pPapel)
  	dentroDaArvore := func(g int64) bool { return g == *u.GrupoID || int64Contem(a.gruposSubordinadosAtivos(*u.GrupoID), g) }
  	if err != nil || (pPapel != "operador" && pPapel != "chefe_setor") || pGrupoID == nil || u.GrupoID == nil || !dentroDaArvore(*pGrupoID) {
  		jsonErro(w, http.StatusForbidden, "permissão insuficiente para remover este papel")
  		return
  	}
  }
  ...
  _, err = a.st.db.Exec(`DELETE FROM usuario_papeis WHERE id = ? AND usuario_id = ?`, papelID, usuarioID)
  ```
- **Verbatim `hUsuarioPapelAdd` (`mensagens.go:113-135`)**:
  ```go
  if u.Papel == "operador" || u.Papel == "chefe_setor" {
  	jsonErro(w, http.StatusForbidden, "operador ou chefe de setor não gerencia papéis")
  	return
  }
  if u.Papel == "gerente" {
  	if papel != "operador" && papel != "chefe_setor" {
  		jsonErro(w, http.StatusForbidden, "gerente só pode atribuir papel de operador ou chefe de setor")
  		return
  	}
  ...
  	dentroDaArvore := func(g int64) bool { return g == *u.GrupoID || int64Contem(a.gruposSubordinadosAtivos(*u.GrupoID), g) }
  	if req.GrupoID != nil && *req.GrupoID != *u.GrupoID {
  		if !dentroDaArvore(*req.GrupoID) {
  			jsonErro(w, http.StatusForbidden, "gerente só pode atribuir papel no próprio grupo ou em subordinados")
  			return
  		}
  	} else {
  		req.GrupoID = u.GrupoID
  	}
  }
  ```
- **Observed Behavior**:
  - In `hUsuarioPapelDel`, authorization is a blacklist checking only `if u.Papel == "operador"`. Any other role (such as `chefe_setor`) bypasses the `operador` check, does not enter the `if u.Papel == "gerente"` block, and falls straight into deleting any role of any user in the system (`DELETE FROM usuario_papeis WHERE id = ? AND usuario_id = ?`) and re-keying sessions!
  - In `hUsuarioPapelAdd`, a `gerente`'s scope is checked only against `req.GrupoID`, but not against target `usuarioID` (`SELECT COALESCE(grupo_id,0) FROM usuarios WHERE id = ?`). A gerente could assign roles to users from outside their organizational tree.

---

#### F. R-11: Drive MIME Allowlist, Inline Headers, and Access
- **File & Lines**: `drive.go:701-707`, `764-774`
- **Verbatim Upload Handler (`drive.go:701-707`)**:
  ```go
  tipoMime := header.Header.Get("Content-Type")
  if tipoMime == "" {
  	tipoMime = mime.TypeByExtension(ext)
  	if tipoMime == "" {
  		tipoMime = "application/octet-stream"
  	}
  }
  ```
- **Verbatim Download Handler (`drive.go:764-774`)**:
  ```go
  inline := r.URL.Query().Get("inline") == "1"
  disp := "attachment"
  if inline {
  	disp = "inline"
  }
  
  w.Header().Set("Content-Type", tipo)
  w.Header().Set("Content-Disposition", fmt.Sprintf(`%s; filename="%s"`, disp, nomeOriginal))
  w.Header().Set("Content-Length", strconv.FormatInt(tamanho, 10))
  
  http.ServeContent(w, r, nomeOriginal, time.Now(), f)
  ```
- **Observed Behavior**:
  - In `hDriveUpload`, any user can upload an arbitrary file with client-supplied MIME `text/html`, `image/svg+xml`, `application/xhtml+xml`, or executable content.
  - In `hDriveDownload`, `inline=1` serves this arbitrary `Content-Type` with `disp = "inline"`, causing the browser to render HTML/SVG with active scripts inside the application origin (stored XSS).
  - `Content-Disposition` interpolates unescaped `nomeOriginal` without `sanitizarNomeArquivo` or `%q` quoting.
  - In contrast, the Material module (`server_material.go:1991-2001`) enforces:
    ```go
    switch mime {
    case "application/pdf", "image/png", "image/jpeg", "image/webp":
    default:
        mime = "application/octet-stream"
    }
    w.Header().Set("Content-Type", mime)
    w.Header().Set("X-Content-Type-Options", "nosniff")
    w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", sanitizarNomeArquivo(nome)))
    ```

---

#### G. R-14: Messaging Role Hierarchy and Recipient Validation
- **File & Lines**: `mensagens.go:640-678` (`hMensagensEnviar`)
- **Verbatim Handler**:
  ```go
  // Validação de Hierarquia de Envio:
  // Operador só pode enviar para o mesmo grupo, para seu gerente, ou responder a quem enviou (pai_id).
  if u.Papel == "operador" && u.GrupoID != nil {
  	var permitidoPara []int64
  	// Busca quem o operador pode enviar
  	rowsP, _ := a.st.db.Query(`
  		SELECT id FROM usuario_papeis
  		WHERE grupo_id = ? OR papel = 'admin' OR (grupo_id = ? AND papel = 'gerente')`,
  		*u.GrupoID, *u.GrupoID)
  	if rowsP != nil {
  		for rowsP.Next() {
  			var pID int64
  			if rowsP.Scan(&pID) == nil {
  				permitidoPara = append(permitidoPara, pID)
  			}
  		}
  		rowsP.Close()
  	}
  ...
  	for _, dID := range req.DestinatarioPapelIDs {
  		if !mapaPerm[dID] {
  			jsonErro(w, http.StatusForbidden, "operadores só podem enviar mensagens para membros da própria equipe ou gerência")
  			return
  		}
  	}
  }
  ```
- **Observed Behavior**:
  - The hierarchy check is guarded by `if u.Papel == "operador" && u.GrupoID != nil`.
  - An operator without a group (`u.GrupoID == nil`) completely skips validation and can send messages to any role in any group in the system.
  - `chefe_setor` has no validation and can send messages to any group/role.
  - Users with no group should be forbidden from sending messages.

---

#### H. R-16: Session Invalidation on Password Change and Admin Seed Setup
- **Files & Lines**:
  - `server_pessoal.go:147-181` (`hTrocarSenha`)
  - `server_pessoal.go:183-237` (`hUsuarioSenha`)
  - `store.go:237-244` (`sessoes` table definition)
  - `store.go:992-995` (`SeedIfEmpty`)
  - `auth.go:224-227` (`auth` middleware setup enforcement)
- **Verbatim `hTrocarSenha` (`server_pessoal.go:175`)**:
  ```go
  if _, err := a.st.db.Exec(`UPDATE usuarios SET senha_hash = ? WHERE id = ?`, novo, u.ID); err != nil {
  	jsonErro(w, http.StatusInternalServerError, err.Error())
  	return
  }
  ```
- **Verbatim `hUsuarioSenha` (`server_pessoal.go:226`)**:
  ```go
  res, err := a.st.db.Exec(`UPDATE usuarios SET senha_hash = ? WHERE id = ?`, novoHash, id)
  ```
- **Verbatim `SeedIfEmpty` (`store.go:992-995`)**:
  ```go
  if _, err := s.db.Exec(
  	`INSERT INTO usuarios (login, senha_hash, papel, precisa_setup) VALUES ('admin', ?, 'admin', 0)`, hash); err != nil {
  	return err
  }
  ```
- **Verbatim `auth` middleware (`auth.go:224-227`)**:
  ```go
  if u.PrecisaSetup && r.URL.Path != "/api/setup" && r.URL.Path != "/api/me" && r.URL.Path != "/api/logout" {
  	http.Error(w, `{"erro":"atualização de cadastro obrigatória", "req_setup": true}`, http.StatusForbidden)
  	return
  }
  ```
- **Observed Behavior**:
  - Changing password via `hTrocarSenha` or resetting via `hUsuarioSenha` updates `senha_hash` but never touches the `sessoes` table (`DELETE FROM sessoes WHERE usuario_id = ?`). Existing sessions remain valid indefinitely.
  - In `SeedIfEmpty`, default admin is seeded with `precisa_setup = 0`, bypassing mandatory password change on initial boot. When set to `1`, `auth.go` already blocks all access except `/api/setup`, `/api/me`, and `/api/logout`.

---

### 1.2 R4 CI & Cleanliness

#### A. Inspection of `ci.sh`
- **File & Lines**: `ci.sh:1-21`
- **Verbatim `ci.sh` Content**:
  ```sh
  # CI local do SCI — build + smoke mínimo (roda antes de qualquer deploy)
  set -e
  export PATH=/opt/data/.local/go/bin:$PATH
  cd "$(dirname "$0")"
  CGO_ENABLED=0 go build -trimpath -ldflags "-s -w" -o /tmp/sci_ci ./...
  echo "build OK"
  # smoke: sobe em porta alta, health, backup CLI, derruba
  SCI_DATA_DIR=/tmp/sci_ci_dados SCI_PORT=14098 SCI_ADMIN_SENHA=ci /tmp/sci_ci &
  PID=$!
  # start frio com migrações novas passa de 1s — sondar até 10s (lição v365: corrida do sleep fixo)
  HEALTH=falhou
  for i in $(seq 1 20); do
    sleep 0.5
    if curl -s -o /dev/null http://127.0.0.1:14098/api/health; then HEALTH=ok; break; fi
  done
  [ "$HEALTH" = "ok" ] && echo "health OK" || { echo "health FALHOU"; kill $PID; exit 1; }
  kill $PID
  SCI_DATA_DIR=/tmp/sci_ci_dados /tmp/sci_ci backup && echo "backup CLI OK"
  rm -rf /tmp/sci_ci_dados
  echo "CI VERDE"
  ```
- **Observed Behavior**:
  - `ci.sh` only builds the binary and runs an ad-hoc smoke test.
  - `ci.sh` does NOT run `go vet ./...`.
  - `ci.sh` does NOT run `go test -count=1 ./...`. None of the 236 unit/integration tests gate deployment!

#### B. Spurious Databases and Artifacts
- **Files Found in Filesystem**:
  1. `dados.tar.gz` (260,835 bytes) in repository root (`c:\Users\hermes\Documents\programming\sci\dados.tar.gz`). Contains real production dump with real military personnel PII and Argon2 password hashes.
  2. `dados/sci.db` (798,720 bytes) in `dados/` directory.
  3. `dados/backups/sci_*.db` (14 separate database backup files totaling several megabytes).
- **Git Status**:
  - Ignored by `.gitignore` lines 9-14 (`dados/`, `*.db`, `*.tar.gz`), but present on disk and posing serious security and privacy risks if packaged or exposed.

---

### 1.3 Test Suite Assessment

- **Existing Tests**: 69 test files (`*_test.go`), 236 test functions.
- **Statement Coverage**: 74.6% statements.
- **Static Analysis Execution**:
  ```powershell
  go vet ./...
  ```
  Result: Clean exit (code 0).
- **Test Execution Harness (`v1_test.go:15-36`, `helpers_test.go:1-60`)**:
  - Tests use real HTTP integration against `app.mux` via `httptest.NewServer` / `httptest.NewRecorder`.
  - `setupTestApp(t)` creates a temporary SQLite file in `os.MkdirTemp`, runs all migrations (v2...v43), seeds default admin, and registers full router.
  - Multi-persona testing helpers: `loginAs(t, app, login, senha)`, `loginAsPapel(t, app, st, login, papel, ...)`.
  - HTTP requests helper: `doJSONReq(app, method, path, body, cookie)`, `doRawReqH(...)`.
  - Database verification helper: direct `st.db.QueryRow` and `st.db.Exec` assertions.

---

## 2. Logic Chain

```
[Observation: server_pessoal.go:1022 queries foto_base64 without inspecting caller context]
    ├──> Anyone can query any user ID (/api/usuarios/{id}/foto)
    └──> [Requirement R-5]: Add caller scope check: allow if caller.ID == targetID, or caller.Papel == "admin", or caller's scope contains target user's grupo_id (including subordinates for manager); reject scope -1 with 403; change Cache-Control to private.

[Observation: server_grupos.go:144-173 NUKE omits tables with NO ACTION FKs: chefe_setores, funcao_membros, avisos, material_conferencias, escala_modelos, setor_sugestoes, conferencia_escalas, grupo_vinculos, usuario_papeis, mensagens]
    ├──> NUKE on groups with existing mural notices, sector chiefs, scale models, or material conferences crashes with SQLite FK violation
    └──> [Requirement R-6]: Expand NUKE delete transaction in topological foreign key dependency order; add TestNukeGrupoRico.

[Observation: ordem_0610_setores.go:108 deletes setor without deleting/nullifying chefe_setores, material_itens, material_conferencias]
    ├──> Sector deletion fails if sector has designated chief or material items/conferences
    └──> [Requirement R-6 Sector]: Add DELETE FROM chefe_setores WHERE setor_id = ? and UPDATE material_itens/material_conferencias SET setor_id = NULL.

[Observation: server_conferencia.go:799-803 excluirArquivada omits conferencia_escalas]
    ├──> Deleting archived conference fails if scale guard rows exist
    └──> [Requirement R-15]: Add DELETE FROM conferencia_escalas WHERE conferencia_id = ? to excluirArquivada (matching hConferenciaDescartar).

[Observation: server_conferencia.go:741 hConferenciaArquivar and :882 hConferenciaDescartar wrapped in confAuth allow operador when esc > 0]
    ├──> Operator can archive closed conferences and discard open conferences
    └──> [Requirement R-8]: Add explicit role guard: if u.Papel != "gerente" && !a.ehEncarregado(u) && !a.ehAuxiliarDePessoal(u) -> 403. Also block esc <= 0 accounts.

[Observation: mensagens.go:44 hMudarContexto validates sector exists globally, does not check group/role match, and overwrites usuarios.setor_id]
    ├──> User can switch to sectors from unrelated groups; account setor_id is permanently overwritten
    └──> [Requirement R-9]: Validate sector belongs to group of active role (and to chefe_setores for chefe_setor); remove UPDATE usuarios SET setor_id.

[Observation: mensagens.go:246 hUsuarioPapelDel only blacklists operador; jefe_setor bypasses check and deletes any role]
    ├──> Fail-open authorization allows chefe_setor to delete roles and re-key sessions globally
    └──> [Requirement R-10]: Replace blacklist with allowlist (admin or gerente); for gerente, ensure target user belongs to manager's tree. In hUsuarioPapelAdd, validate target user scope.

[Observation: drive.go:701-707 accepts any MIME; :764 serves inline with unsanitized filename]
    ├──> File upload and inline rendering enables stored XSS and header injection
    └──> [Requirement R-11]: Implement MIME allowlist for inline serving (pdf, png, jpeg, webp); default all others to attachment; sanitize filename with sanitizarNomeArquivo and %q; set X-Content-Type-Options: nosniff.

[Observation: mensagens.go:642 checks operator hierarchy only if u.GrupoID != nil]
    ├──> Operators without group can message anyone anywhere; jefe_setor has no restrictions
    └──> [Requirement R-14]: Enforce that accounts without group cannot send messages; constrain operador and chefe_setor to same group/manager/admin.

[Observation: server_pessoal.go:175 and :226 do not delete sessoes; store.go:993 seeds admin with precisa_setup=0]
    ├──> Compromised sessions stay active after password changes; default admin is never forced to change credentials
    └──> [Requirement R-16]: Execute DELETE FROM sessoes WHERE usuario_id = ? on password changes/resets; set precisa_setup = 1 in SeedIfEmpty.

[Observation: ci.sh:5-20 only runs build + curl health check; no go vet or go test]
    ├──> Broken tests or vet regressions pass CI silently
    └──> [Requirement R-4 CI]: Add go vet ./... and go test -count=1 ./... into ci.sh before smoke test.

[Observation: dados.tar.gz and dados/ contain real PII dumps and backup DBs]
    ├──> Violates repository cleanliness and privacy standards
    └──> [Requirement R-4 Cleanliness]: Remove dados.tar.gz, dados/sci.db, dados/backups/*.db from filesystem.
```

---

## 3. Caveats

- **Read-Only Verification**: In accordance with the Explorer role boundaries, no production code or filesystem deletion was performed. All line numbers and logic traces were verified against the live repository at commit `608e5ef`.
- **Operating System Context**: The local environment is Windows (PowerShell), while `ci.sh` is a POSIX bash script targeting Linux hosts (`/opt/data/...`). When running CI tests on Linux hosts, `-race` can be executed, but on Windows `-race` is unavailable without gcc/cgo.
- **Scope Hierarchy Helpers**: Functions `a.gruposSubordinadosAtivos(escopo)` and `a.setorIDValidoNoGrupo(sid, gid)` already exist in `helpers.go` / `server_grupos.go` and should be reused directly for guards.

---

## 4. Conclusion & Required Changes Specification

### 4.1 Summary of Exact Changes by Item

| Item | File(s) & Lines | Function / Section | Required Modification |
|---|---|---|---|
| **R-5** | `server_pessoal.go:1015-1047` | `hUsuarioFotoGet` | 1. Check `u := usuarioDoCtx(r)`.<br>2. If `u.ID == id` or `u.Papel == "admin"`: allow.<br>3. If `esc := escopoDoUsuario(u); esc <= 0`: 403.<br>4. Query target user's `grupo_id`: verify `targetGrupo == esc` (or in `gruposSubordinadosAtivos` for gerente). Else 403/404.<br>5. Set `Cache-Control: private, max-age=3600`. |
| **R-6 (NUKE)** | `server_grupos.go:144-173` | `hGrupoExcluir` | Add missing DELETE statements in order before deleting `grupos`: `chefe_setores`, `funcao_membros`, `aviso_cientes`, `aviso_comentarios`, `avisos`, `material_conferencia_itens`, `material_conferencias`, `material_viaturas`, `material_classes`, `material_tipos`, `escala_modelo_postos`, `escala_modelo_aptos`, `escala_modelos`, `setor_sugestoes`, `grupo_setor_responsaveis`, `conferencia_escalas`, `grupo_vinculos`, `mensagem_destinatarios`, `mensagem_respostas`, `mensagem_pastas`, `mensagens`, `usuario_papeis`. |
| **R-6 (Setor)** | `ordem_0610_setores.go:96-119` | `hSetorExcluir` | Inside transaction before `DELETE FROM setores`: add `DELETE FROM chefe_setores WHERE setor_id = ?`, `UPDATE material_itens SET setor_id = NULL WHERE setor_id = ?`, and `UPDATE material_conferencias SET setor_id = NULL WHERE setor_id = ?`. |
| **R-15** | `server_conferencia.go:799-803` | `excluirArquivada` | Add `DELETE FROM conferencia_escalas WHERE conferencia_id = ?` before `DELETE FROM conferencias WHERE id = ?`. |
| **R-8** | `server_conferencia.go:741-774`, `:882-930` | `hConferenciaArquivar`, `hConferenciaDescartar` | In both handlers, enforce: `if u.Papel != "gerente" && !a.ehEncarregado(u) && !a.ehAuxiliarDePessoal(u) { jsonErro(w, http.StatusForbidden, "ato do gerente ou do encarregado de pessoal"); return }`. Ensure `escopoDoUsuario(u) <= 0` also receives 403. |
| **R-9** | `mensagens.go:44-68` | `hMudarContexto` | 1. If `req.SetorID != nil`: query `SELECT COUNT(*) FROM setores WHERE id = ? AND grupo_id = ? AND ativo = 1` matching the role's `grupoID`.<br>2. If `papel == "chefe_setor"`: verify user is in `chefe_setores` for that sector.<br>3. REMOVE `UPDATE usuarios SET setor_id = ? WHERE id = ?` (keep only session update). |
| **R-10** | `mensagens.go:246-270`, `mensagens.go:113-135` | `hUsuarioPapelDel`, `hUsuarioPapelAdd` | 1. In `hUsuarioPapelDel`: replace `if u.Papel == "operador"` blacklist with whitelist: allow ONLY `admin` or `gerente`. If `gerente`, verify target user and target role are in manager's tree.<br>2. In `hUsuarioPapelAdd`: if `gerente`, verify target user (`usuarios.id`) belongs to manager's group or subordinate groups. |
| **R-11** | `drive.go:701-707`, `:764-774` | `hDriveUpload`, `hDriveDownload` | 1. In `hDriveDownload`: allow `inline` ONLY for allowlisted MIME (`application/pdf`, `image/png`, `image/jpeg`, `image/webp`). Force `disp = "attachment"` for all other types.<br>2. Set header `X-Content-Type-Options: nosniff`.<br>3. Format `Content-Disposition` using `fmt.Sprintf("%s; filename=%q", disp, sanitizarNomeArquivo(nomeOriginal))`. |
| **R-14** | `mensagens.go:640-678` | `hMensagensEnviar` | 1. If `u.Papel != "admin" && u.GrupoID == nil`: 403.<br>2. For `operador` and `chefe_setor`: restrict recipients to members of their group, their manager, or admin (or replying to parent).<br>3. For `gerente`: restrict to their group, subordinates, other managers/admins, or parent thread. |
| **R-16** | `server_pessoal.go:175-180`, `:226-235`, `store.go:993` | `hTrocarSenha`, `hUsuarioSenha`, `SeedIfEmpty` | 1. In `hTrocarSenha`: execute `DELETE FROM sessoes WHERE usuario_id = ?`.<br>2. In `hUsuarioSenha`: execute `DELETE FROM sessoes WHERE usuario_id = ?`.<br>3. In `SeedIfEmpty`: insert admin with `precisa_setup = 1`. |
| **R4 (CI)** | `ci.sh:5` | `ci.sh` | Add `go vet ./...` and `go test -count=1 ./...` immediately after build and before starting background server. |
| **R4 (Clean)** | Root & `dados/` | Filesystem cleanup | Delete `dados.tar.gz`, `dados/sci.db`, and `dados/backups/*.db`. |

---

## 5. Verification Method

### 5.1 Verification Commands
To be run by the implementer upon executing the fixes:
1. **Static Analysis**:
   ```powershell
   go vet ./...
   ```
   *Expected*: Zero findings / clean exit.
2. **Compilation**:
   ```powershell
   go build ./...
   ```
   *Expected*: Clean exit, zero compiler errors.
3. **Full Test Suite Execution**:
   ```powershell
   go test -count=1 ./...
   ```
   *Expected*: 100% PASS across all 236+ tests.
4. **Local CI Script**:
   ```sh
   ./ci.sh
   ```
   *Expected*: "vet OK", "tests OK", "build OK", "health OK", "backup CLI OK", "CI VERDE".

### 5.2 Required New Regression Tests

1. **`TestR5_FotoEscopo`**:
   - User in Group A attempts `GET /api/usuarios/{id_b}/foto` of User in Group B -> Expect `403 Forbidden` / `404 Not Found`.
   - User with scope `-1` attempts `GET /api/usuarios/{id}/foto` -> Expect `403 Forbidden`.
   - User requests own photo -> Expect `200 OK` with `Cache-Control: private`.
   - Manager requests subordinate photo -> Expect `200 OK`.
2. **`TestR6_NukeGrupoRico`**:
   - Create a group with active mural notice (`avisos`), designated sector chief (`chefe_setores`), role members (`funcao_membros`), scale models (`escala_modelos`), and material conferences (`material_conferencias`).
   - Call `DELETE /api/grupos/{id}?nuke=1` as admin with password.
   - Assert `200 OK`, no FK violation, and all associated tables wiped cleanly.
3. **`TestR6_SetorExcluirComChefeEMaterial`**:
   - Create sector with a row in `chefe_setores` and an item in `material_itens`.
   - Call `DELETE /api/setores/{id}` as manager.
   - Assert `200 OK`, `chefe_setores` cleared, and `material_itens.setor_id` set to NULL.
4. **`TestR15_ExcluirArquivadaComEscala`**:
   - Create closed, archived conference with linked row in `conferencia_escalas`.
   - Call `DELETE /api/conferencia/{id}/arquivada` as manager.
   - Assert `200 OK`, no FK conflict.
5. **`TestR8_ConferenciaArquivarDescartarGuarda`**:
   - As `operador`, call `POST /api/conferencia/{id}/arquivar` -> Expect `403 Forbidden`.
   - As `operador`, call `DELETE /api/conferencia/{id}` -> Expect `403 Forbidden`.
   - As `gerente` or `enc_pessoal`, call endpoints -> Expect `200 OK`.
6. **`TestR9_MudarContextoSetorEscopo`**:
   - Call `POST /api/me/contexto` with `setor_id` belonging to another group -> Expect `400 Bad Request`.
   - Call with valid sector -> verify `sessoes.setor_ativo_id` updated, but `usuarios.setor_id` unchanged.
7. **`TestR10_UsuarioPapelPermissoes`**:
   - As `chefe_setor`, call `DELETE /api/usuarios/{id}/papeis/{pid}` -> Expect `403 Forbidden`.
   - As `gerente`, attempt to add/del role for user in foreign group -> Expect `403 Forbidden`.
8. **`TestR11_DriveMimeInline`**:
   - Upload file with MIME `text/html`.
   - Request `GET /api/drive/download/{id}?inline=1`.
   - Assert `Content-Disposition: attachment; filename="...html"`, `X-Content-Type-Options: nosniff`.
9. **`TestR14_MensagensHierarquia`**:
   - Operator without group attempts to send message -> Expect `403 Forbidden`.
   - Operator attempts to send message to user in another group -> Expect `403 Forbidden`.
10. **`TestR16_InvalidaSessoesTrocaSenha`**:
    - Login as user, obtain session cookie A.
    - Change password via `POST /api/perfil/senha`.
    - Using cookie A, call `GET /api/me` -> Expect `401 Unauthorized` / session destroyed.
    - Verify `SeedIfEmpty` seeds admin with `precisa_setup = 1`.

---

### Invalidation Conditions
- Any failure in existing tests when running `go test -count=1 ./...`.
- Reintroduction of blacklists instead of allowlists in permission checks.
- Failure of NUKE transaction on groups with rich data models.
