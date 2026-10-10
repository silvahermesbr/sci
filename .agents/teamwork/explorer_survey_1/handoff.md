# Relatório de Investigação Técnica — Onda v1.5.4 (R1: P0s e R2: Bugs de Campo)

**Data**: 2026-10-10  
**Agente**: `teamwork_preview_explorer_1`  
**Escopo**: Auditoria e Análise Read-Only dos Itens R1 (A, B, C) e R2 (D1, D2, D3)  
**Status**: Investigação Concluída — Handoff Hard  

---

## 1. Observation (Observações Diretas do Código)

### Item A (R-1): Escopo `-1` (Contas Sem Grupo) e Verificações Frouxas `esc > 0`

- **Definição de Escopo**: Em `server_grupos.go:202-213`:
  ```go
  func escopoDoUsuario(u *Usuario) int64 {
      if u == nil {
          return -1
      }
      if u.Papel == "admin" {
          return 0
      }
      if u.GrupoID == nil {
          return -1 // gerente/operador sem grupo = sem acesso a dados de grupo
      }
      return *u.GrupoID
  }
  ```
  Contas válidas autenticadas cujo `GrupoID` é nulo recebem escopo `-1`.

- **Falha em Listagem de Grupos**: Em `server_grupos.go:219-222` e `:382-385`:
  ```go
  func (a *App) hGruposList(w http.ResponseWriter, r *http.Request) {
      escopo := escopoDoUsuario(usuarioDoCtx(r))
      jsonOK(w, a.gruposComCodigo(escopo))
  }
  // server_grupos.go:382:
  if escopo > 0 && id != escopo {
      continue
  }
  ```
  Quando `escopo == -1`, a condição `escopo > 0 && id != escopo` é FALSA. O loop não pula nenhum grupo. Usuários sem grupo recebem a lista completa de todos os grupos do sistema, incluindo seus códigos de convite (`codigo`), vínculos e contagens.

- **Falha em Listagem de Conferências**: Em `server_conferencia.go:836-855` (`hConferenciaList`):
  ```go
  if r.URL.Query().Get("arq") == "1" {
      if escopo > 0 {
          rows, err = a.st.db.Query(q+` WHERE c.grupo_id = ? ...`, escopo)
      } else {
          rows, err = a.st.db.Query(q + ` WHERE c.arquivada_em IS NOT NULL ...`)
      }
  } else {
      de, ate := a.periodoPadrao(r)
      if escopo > 0 {
          rows, err = a.st.db.Query(q+` WHERE c.grupo_id = ? ...`, escopo, de, ate)
      } else {
          rows, err = a.st.db.Query(q+` WHERE c.arquivada_em IS NULL AND c.data BETWEEN ? AND ? ...`, de, ate)
      }
  }
  ```
  Quando `escopo == -1`, `escopo > 0` é falso; o código cai no ramo `else` desenhado para o admin (`0`) e executa a query sem filtro `WHERE c.grupo_id = ?`, retornando conferências de todos os grupos.

- **Falha em Detalhe e PDF de Conferência**: Em `server_conferencia.go:961-968` (`hConferenciaGet`) e `server_conferencia.go:1130-1137` (`hConferenciaPDF`):
  ```go
  if esc := escopoDoUsuario(uCtx); esc > 0 {
      var gid int64
      qerr := a.st.db.QueryRow(`SELECT COALESCE(grupo_id,0) FROM conferencias WHERE id = ?`, id).Scan(&gid)
      if qerr != nil || (gid != esc && !int64Contem(a.gruposSubordinadosAtivos(esc), gid)) {
          jsonErro(w, http.StatusForbidden, "conferência de outro grupo")
          return
      }
  }
  ```
  Quando `esc == -1`, o bloco de checagem é inteiramente pulado. Qualquer conta sem grupo acessa dados detalhados e gera PDF de qualquer conferência.

- **Falha em Fechamento, Arquivamento e Descarte de Conferência**:
  - `server_conferencia.go:553-560` (`hConferenciaFechar`): `if esc := escopoDoUsuario(u); esc > 0 { ... }`. Gerente com `esc == -1` fecha conferência de qualquer grupo.
  - `server_conferencia.go:754-762` (`hConferenciaArquivar`): `if esc := escopoDoUsuario(u); esc > 0 { ... } else if u.Papel != "admin" && u.Papel != "gerente" { 403 }`. Gerente com `esc == -1` arquiva conferência de qualquer grupo.
  - `server_conferencia.go:654` e `:712` (`hConferenciaSetorConcluir` e `hConferenciaSetorReabrir`): `if esc := escopoDoUsuario(u); esc > 0 && confGrupoID != esc { 403 }`. Pula o bloqueio quando `esc == -1`.
  - `server_conferencia.go:1436` e `:1489` (`hComentariosAdd` e `hComentariosList`): `if esc := escopoDoUsuario(u); esc > 0 { ... }`. Pula a validação quando `esc == -1`.
  - `server_realtime.go:88-99` (`hConferenciaStream` SSE): `if esc := escopoDoUsuario(u); esc > 0 { ... }`. Conexão SSE aberta para qualquer conferência sem validação de grupo quando `esc == -1`.

- **Falha em Pessoal (PII completo)**:
  - `helpers.go:165-171` (`pessoasTodas`, chamado por `hPessoasList` em `server_pessoal.go:589`):
    ```go
    if escopo > 0 {
        rows, err = a.st.db.Query(q+` WHERE p.grupo_id = ? ...`, escopo)
    } else {
        rows, err = a.st.db.Query(q + ` ...`)
    }
    ```
    Retorna todo o efetivo do banco para `esc == -1`.
  - `server_pessoal.go:407-413` (`hPessoaFicha`): `if escopo > 0 { q += " AND p.grupo_id = ?" }`. Retorna ficha de pessoa de qualquer grupo.
  - `server_pessoal.go:433-439` (`hPessoaComentarios`): `if escopo > 0 { ... }`. Retorna histórico de comentários de qualquer pessoa.
  - `server_pessoal.go:505-508` (`hPessoaPDF`): `if escopo > 0 && (gid == nil || *gid != escopo) { 403 }`. Emite PDF com dados pessoais (endereço, telefone, sangue, nascimento) para qualquer militar.
  - `server_pessoal.go:1364-1367` (`hPessoaQRCode`): `if esc := escopoDoUsuario(u); esc > 0 && (gid == nil || *gid != esc) { 403 }`. Gera QR Code sem restrição de grupo.

- **Falha em Material e Cautelas**:
  - `server_material.go:1115` (`hMaterialItensList`):
    `WHERE (? <= 0 OR mi.grupo_id = ?)` com parâmetros `(escopo, escopo)`. Para `escopo == -1`, `-1 <= 0` é verdadeiro, retornando todos os itens de todos os grupos.
  - `server_material.go:1752` (`hMaterialCautelasList`):
    `WHERE (? <= 0 OR mi.grupo_id = ?)`. Para `escopo == -1`, retorna todas as cautelas de todos os grupos.
  - `server_material.go:904-923` (`hMaterialInventarioPDF`):
    `if escopo > 0 { q += " AND mi.grupo_id = ?"; args = append(args, escopo) }`. Gera PDF do inventário geral do sistema para `esc == -1`.
  - `server_material.go:145, 190, 267, 298, 325, 372, 563, 646, 714, 873, 1845, 2049`:
    Todos usam o padrão `if esc := escopoDoUsuario(u); esc > 0 && itemGrupo != esc { 403 }`.

- **Falha em Escalas e Relatórios/Export**:
  - `helpers.go:112-124`:
    ```go
    func filtroGrupoSQL(escopo int64, alias string) string {
        if escopo <= 0 {
            return ""
        }
        return ` AND ` + alias + `.grupo_id = ?`
    }
    func filtroGrupoArgs(escopo int64) []any {
        if escopo <= 0 {
            return nil
        }
        return []any{escopo}
    }
    ```
    Utilizado em `server_relatorios.go:356, 375, 391` (`hExportCSV`). Para `escopo <= 0` (inclusive `-1`), `filtroGrupoSQL` retorna string vazia e argumentos nulos, exportando a totalidade de pessoas, presenças e conferências sem filtro de grupo.
  - `relatorio.go:1431, 1460, 1502, 1567, 1601, 1645` (`hExportarDados`):
    Todos usam `if escopo > 0 { q += " AND ...grupo_id = ?"; args = append(args, escopo) }`.
  - `server_escalas.go:1110-1112` (`hEscalasLimparDia`):
    `DELETE FROM escala_turnos WHERE (? <= 0 OR grupo_id = ?) AND data_inicio LIKE ?` com `escopo, escopo`. Se chamado por conta com `esc == -1`, `-1 <= 0` apaga todos os turnos de todas as subunidades da data especificada.
  - `server_escalas.go:665, 966, 1276, 1292, 1391, 1493, 1507`:
    Todas as queries contêm `(? <= 0 OR grupo_id = ?)`.

---

### Item B (R-2 / R-3): `foto_base64` no Backend e Frontend

- **Backend — Ausência de Validação na Escrita**:
  - `server_pessoal.go:343` (`hUsuarioAtualizar`):
    ```go
    if req.FotoBase64 != nil {
        _, _ = a.st.db.Exec(`UPDATE usuarios SET foto_base64 = ? WHERE id = ?`, strings.TrimSpace(*req.FotoBase64), id)
    }
    ```
  - `server_pessoal.go:994-1003` (`hPerfilSet`):
    ```go
    if _, err := a.st.db.Exec(`UPDATE usuarios SET ... foto_base64 = ? WHERE id = ?`,
        ..., strings.TrimSpace(req.FotoBase64), u.ID); err != nil {
    ```
  Em ambos os pontos, o backend apenas aplica `strings.TrimSpace`. Não há validação de prefixo MIME (`data:image/...`), validação de decodificação base64, checagem de bytes mágicos de imagem, nem teto de tamanho.

- **Frontend — Sinks de Renderização Sem Escape**:
  - `web/core.js:580` (barra lateral do usuário logado):
    ```javascript
    ${usuario && usuario.foto_base64 ? `<img src="${usuario.foto_base64}" class="sidebar-avatar-img" alt="Foto">` : `<svg...`}
    ```
  - `web/views_gestao.js:548` (tabela de listagem de contas):
    ```javascript
    ${u.foto_base64 ? `<img src="${u.foto_base64}" style="width:100%; height:100%; object-fit:cover">` : `<svg...`}
    ```
  - `web/views_gestao.js:683`:
    ```javascript
    ${fotoAtual ? `<img src="${fotoAtual}" style="width:100%; height:100%; object-fit:cover">` : `<svg...`}
    ```
  - `web/views_gestao.js:759`:
    ```javascript
    fotoBox.innerHTML = `<img src="${fotoAtual}" style="width:100%; height:100%; object-fit:cover">`;
    ```
  - `web/views_gestao.js:2535` (tela de perfil):
    ```javascript
    ${fotoAtual ? `<img src="${fotoAtual}" alt="Foto 1x1">` : `<svg...`}
    ```
  - `web/views_gestao.js:2637`:
    ```javascript
    fotoBox.innerHTML = `<img src="${fotoAtual}" alt="Foto 1x1">`;
    ```
  - `web/views_gestao.js:2707`:
    ```javascript
    sbAvatar.innerHTML = `<img src="${fotoAtual}" class="sidebar-avatar-img" alt="Foto">`;
    ```
  - `web/ui_helpers.js:251, 386, 462`: Cópia espelhada sem escape.
  
  **Política CSP Permissiva**: Em `main.go:111-117`, o cabeçalho `Content-Security-Policy` define:
  ```
  default-src 'self'; script-src 'self' 'unsafe-inline'; style-src 'self' 'unsafe-inline'; ...
  ```
  Como `script-src` permite `'unsafe-inline'`, qualquer payload em atributo HTML como `x" onerror="<código js>"` executa código JavaScript sem bloqueio do navegador.

---

### Item C (R-3 / R-4): Importação de Backup, Swap Atômico, WAL e Cadeia de Migrações

- **Código Atual da Importação**: Em `server_admin.go:259-274` (`hBackupImportar`):
  ```go
  // 4) swap atômico: fecha pool → remove alvo+wal/shm → rename → reabre+migra
  novo := a.st.arquivo + ".novo"
  _ = os.Remove(novo)
  if err = os.Rename(tmpNome, novo); err != nil {
      jsonErro(w, http.StatusInternalServerError, "preparar swap falhou — nada alterado: "+err.Error())
      return
  }
  // o defer os.Remove(tmpNome) vira no-op (arquivo renomeado)
  if err = a.st.ReabrirComArquivo(novo); err != nil {
      // rollback: volta o arquivo de segurança para o lugar
      _ = a.st.ReabrirComArquivo(a.st.arquivo)
      jsonErro(w, http.StatusInternalServerError, "swap falhou — banco reaberto no estado anterior: "+err.Error())
      return
  }
  _ = os.Remove(a.st.arquivo + "-wal")
  _ = os.Remove(a.st.arquivo + "-shm")
  ```

- **Defeitos Identificados na Importação**:
  1. **Ausência de Renomeação para `sci.db`**: O arquivo importado é movido para `novo` (`dados/sci.db.novo`). `ReabrirComArquivo(novo)` é chamado e faz `s.arquivo = novo`. O arquivo `sci.db` original continua intocado em disco. Ao reiniciar o processo, `AbrirStore` abre `filepath.Join(dataDir, "sci.db")` (`store.go:40`), descartando completamente a importação.
  2. **Divergência Crítica na Cadeia de Migrações**: Em `store.go:1372-1402`:
     `ReabrirComArquivo` chama explicitamente: `migrarV2()`, `migrar()`, `migrarV4()`, `migrarV5()`, `migrarV6()`, `migrarV7()`, `migrarV8()`, `migrarV9()`, `migrarV10()`, `migrarV11()`, `migrarV12()` e encerra.
     Por outro lado, `AbrirStore` (`store.go:56-175`) executa de `migrar()` até `migrarV43()`. Ao importar um banco antigo (ex.: schema v12), as 31 migrações de v13 a v43 nunca são aplicadas.
  3. **Rollback Defeituoso**: Em `store.go:1354`, `s.arquivo = novoArquivo` ocorre antes das migrações. Se uma migração falhar, `a.st.arquivo` já contém o caminho do arquivo corrompido. O rollback em `server_admin.go:269` (`a.st.ReabrirComArquivo(a.st.arquivo)`) tenta reabrir o próprio arquivo que acabou de falhar. Além disso, o arquivo de segurança criado em `nomeSeg` (`server_admin.go:254`) nunca é restaurado.
  4. **Corrupção de WAL/SHM**: As linhas 273-274 (`os.Remove(a.st.arquivo + "-wal")`) executam após `ReabrirComArquivo`, ou seja, sobre o arquivo ativo que acabou de ser aberto pela conexão SQLite em modo WAL.

---

### Item D1 (R-12): Chefe-Zumbi, `chefeComandaSetor` e Destituição de Chefia

- **Definição de `chefeComandaSetor`**: Em `onda_0510_conf_escopo.go:145-164`:
  ```go
  func (a *App) chefeComandaSetor(u *Usuario, setorID int64) bool {
      if u == nil || setorID <= 0 {
          return false
      }
      var comandos int
      if e := a.st.db.QueryRow(`SELECT COUNT(*) FROM chefe_setores WHERE usuario_id = ? AND setor_id = ?`, u.ID, setorID).Scan(&comandos); e == nil && comandos > 0 {
          return true
      }
      if u.SetorID != nil && *u.SetorID == setorID {
          return true
      }
      if u.PessoaID != nil && *u.PessoaID > 0 {
          var s *int64
          _ = a.st.db.QueryRow(`SELECT setor_id FROM pessoas WHERE id = ?`, *u.PessoaID).Scan(&s)
          if s != nil && *s == setorID {
              return true
          }
      }
      return false
  }
  ```

- **Nomeação de Chefe**: Em `onda_0510_escalas.go:280-300` (`hGrupoNomearChefe`):
  ```go
  if _, e := tx.Exec(`DELETE FROM chefe_setores WHERE setor_id = ?`, req.SetorID); e != nil { ... }
  if _, e := tx.Exec(`INSERT INTO chefe_setores (grupo_id, setor_id, usuario_id) VALUES (?,?,?)`, escopo, req.SetorID, req.UsuarioID); e != nil { ... }
  ...
  if _, e := tx.Exec(`UPDATE usuarios SET setor_id = ? WHERE id = ?`, req.SetorID, req.UsuarioID); e != nil { ... }
  ```
  Quando o usuário B é nomeado para o setor S, a linha do usuário A em `chefe_setores` é deletada. Porém, `usuarios.setor_id` de A nunca é limpo, permanecendo com o valor de S.

- **Destituição de Chefe**: Em `onda_0510_escalas.go:315` e `:356-399` (`hGrupoDestituirChefe`):
  O comentário na linha 315 registra explicitamente: `// usuarios.setor_id NÃO é mexido aqui.`
  O código remove o registro de `chefe_setores` e eventualmente o papel em `usuario_papeis`, mas não altera `usuarios.setor_id`.
  Como consequência, `chefeComandaSetor(A, S)` continua retornando `true` pela verificação `u.SetorID != nil && *u.SetorID == setorID`.

- **Divergência em `hSetoresAgregado`**: Em `onda_0510_itens79.go:175-184`:
  A contagem e identificação de chefes de setor consulta `usuarios.setor_id` em vez de `chefe_setores`:
  ```sql
  (SELECT COUNT(*) FROM usuarios us2
   JOIN usuario_papeis up2 ON up2.usuario_id = us2.id AND up2.papel = 'chefe_setor' AND up2.grupo_id = s.grupo_id
   WHERE us2.setor_id = s.id AND us2.ativo = 1)
  ```
  Se o usuário A destituído mantém `usuarios.setor_id = s.id` e o novo chefe B também tem `setor_id = s.id`, a contagem retorna `2`, exibindo a anomalia de "2 chefes simultâneos".

---

### Item D2: Rotas de Exportação/PDF/Dados sem Guarda Adequada

- **Rotas com `a.auth(false)` desprovidas de verificação de papel ou escopo**:
  - `server_escalas.go:1703`: `GET /api/escalas/pdf` -> `a.auth(false, a.hEscalasPDF)`. Permite download de escalas sem bloqueio por papel.
  - `server_escalas.go:1716-1717`: `GET /api/escalas/relatorio-dia.pdf` e `/api/escalas/relatorio-dia/pdf` -> `a.auth(false, a.hEscalasRelatorioDiaPDF)`. Não valida `reservaAtivo()`, papel ou escopo restrito.
  - `server_escalas.go:1718`: `GET /api/escalas/minhas` -> `a.auth(false, a.hEscalasMinhas)`. Não valida módulo em reserva nem perfil operacional.
  - `server_material.go:2184`: `GET /api/material/inventario/pdf` -> `a.auth(false, a.hMaterialInventarioPDF)`. Aberto a qualquer usuário autenticado.
  - `server_material.go:2188`: `GET /api/material/cautelas/{id}/recibo.pdf` -> `a.auth(false, a.hMaterialCautelaReciboPDF)`. Não utiliza `authMaterial`.
  - `server_material.go:2209`: `GET /api/material/conferencias/{id}/pronto.pdf` -> `a.auth(false, a.hMaterialConferenciaPDF)`. Aberto a qualquer autenticado; zero validação de grupo ou papel.
  - `server_conferencia.go:1593`: `GET /api/conferencia/{id}/relatorio.pdf` -> `a.auth(false, a.hConferenciaPDF)`.
  - `server_relatorios.go:414-424`:
    - `GET /api/relatorio/registros` -> `a.auth(false, a.hRegistrosBusca)`
    - `GET /api/relatorio/tags` -> `a.auth(false, a.hTagsDisponiveis)`
    - `GET /api/efetivo_atual` -> `a.auth(false, a.hEfetivoAtual)`
    - `GET /api/presenca/periodo` -> `a.auth(false, a.hPresencaPeriodo)`
    - `GET /api/relatorio` -> `a.auth(false, a.hRelatorioJSON)`
    - `GET /api/relatorio.pdf` -> `a.auth(false, a.hRelatorioPDF)`
    - `GET /api/relatorio/detalhado.pdf` -> `a.auth(false, a.hRelatorioDetalhadoPDF)`
    - `GET /api/export/{t}` -> `a.auth(false, a.hExportCSV)`
    - `GET /api/export` -> `a.auth(false, a.hExportarDados)`
    Todas essas rotas expõem inteligência corporativa e dados estratégicos com autenticação básica sem conferência de papel nem barreira contra `esc == -1`.
  - `server_pessoal.go:468` e `:1350`: `GET /api/pessoas/{id}/pdf` e `GET /api/pessoas/{id}/qr`.

---

### Item D3 (R-7): Ordenação por Antiguidade e Tags de Grupo

- **Filtro vs Ordenação no Pré-fechamento**:
  - Em `onda_0910_conf_antiguidade.go:132-134` (`predicadoAntiguidade`), o filtro casa a pessoa se a tag estiver em qualquer uma das três fontes:
    1. `p.funcao_id`
    2. `u2.funcao_id` (`usuarios`)
    3. `up2.funcao_id` (`usuario_papeis`)
  - Porém, em `onda_0910_conf_antiguidade.go:201` (`hSetorPreFechamento`):
    ```go
    q += ` ORDER BY COALESCE(fu.antiguidade, 999), p.nome_guerra`
    ```
    A ordenação lê apenas `fu.antiguidade` (da tabela `pessoas`). Se o militar possui a função atribuída em seu usuário ou papel, `fu.antiguidade` é NULL, sendo avaliado como 999. O militar é jogado para o final da fila de antiguidade.

- **Ausência de Ordenação por Antiguidade na Tela**:
  - Em `server_conferencia.go:985` (`hConferenciaGet`):
    ```sql
    ORDER BY pr.situacao, p.nome_guerra
    ```
    A visualização principal da conferência nunca ordena por antiguidade, mesmo quando a conferência foi iniciada com filtro de antiguidade.

- **Ordenação em PDF de Conferência**:
  - Em `server_conferencia.go:1044-1053` (`montarLancamentosPDFConferencia`):
    ```sql
    LEFT JOIN funcoes fu ON fu.id = p.funcao_id
    LEFT JOIN cam_funcao cf2 ON cf2.id = fu.id
    ...
    ORDER BY COALESCE(cf2.caminho,'~sem função'), COALESCE(cs2.caminho,'~sem setor'), p.nome_guerra COLLATE NOCASE
    ```
    A ordenação do PDF une `cam_funcao` exclusivamente através de `p.funcao_id`. Funções herdadas via usuário ou papel resultam em `~sem função`.

- **Exclusão Silenciosa de Militares Sem Tag**:
  - Em `server_conferencia.go:434-520` (`hConferenciaIniciar`) e `onda_despacho.go:160-218` (`hConferenciaDespachar`), quando `req.FuncaoIDs` é informado, os militares ativos do grupo que não possuem nenhuma das tags selecionadas são excluídos da conferência sem qualquer indicação na resposta da API (`{"id": ..., "data": ...}`). O frontend não recebe lista de militares excluídos para exibir alertas.

---

## 2. Logic Chain (Cadeia Lógica de Inferência)

1. **R1 Item A (Escopo -1)**:
   - *Premissa*: `escopoDoUsuario` retorna `-1` para qualquer conta com `GrupoID == nil` (Obs. Item A).
   - *Mecanismo de Falha*: Handlers de dados verificam `if esc > 0` e assumem que `else` representa o papel `admin` (escopo `0`).
   - *Impacto*: Valores de escopo `-1` tornam `esc > 0` falso, caindo inadvertidamente no ramo global irrestrito.
   - *Consequência*: Uma conta comum sem grupo herda privilégios de visualização de dados equivalentes ao admin em mais de 35 endpoints.

2. **R1 Item B (XSS foto_base64)**:
   - *Premissa*: `hUsuarioAtualizar` e `hPerfilSet` aceitam strings arbitrárias e persistem em `usuarios.foto_base64` (Obs. Item B).
   - *Mecanismo de Falha*: Múltiplos arquivos JS interpolam `${usuario.foto_base64}` diretamente dentro de `<img src="...">` sem a função `esc()`.
   - *Impacto*: O cabeçalho CSP permite `'unsafe-inline'`. Um payload injetado com quebra de atributo (ex.: `x" onerror="...")` executa scripts no contexto de qualquer operador, gerente ou admin que visualize a listagem ou perfil.

3. **R1 Item C (Importação de Backup)**:
   - *Premissa*: `hBackupImportar` copia o upload para `sci.db.novo`, mas nunca executa a substituição física do arquivo `sci.db` (Obs. Item C).
   - *Mecanismo de Falha*: `ReabrirComArquivo` abre o arquivo `.novo`, mas o restart do servidor lê `sci.db`. Além disso, `ReabrirComArquivo` só executa migrações até v12, enquanto a aplicação requer até v43.
   - *Impacto*: O banco volta ao estado anterior após reboot, e backups restaurados em versões antigas resultam em falhas estruturais por tabelas faltantes. O rollback reabre o arquivo corrompido em vez do backup de segurança.

4. **R2 Item D1 (Chefe-Zumbi)**:
   - *Premissa*: `chefeComandaSetor` concede autoridade se `u.SetorID == setorID` mesmo quando o registro em `chefe_setores` foi excluído (Obs. Item D1).
   - *Mecanismo de Falha*: A nomeação de um novo chefe remove o anterior de `chefe_setores`, mas não zera `usuarios.setor_id`. A rotina `hGrupoDestituirChefe` declara explicitamente que não altera `usuarios.setor_id`.
   - *Impacto*: O chefe destituído retém permissões de fechar/concluir/reabrir o setor. Paralelamente, `hSetoresAgregado` lê `usuarios.setor_id` e contabiliza dois chefes ativos simultaneamente para o mesmo setor.

5. **R2 Item D2 (Rotas de Exportação sem Guarda)**:
   - *Premissa*: Rotas de relatórios, dados e PDFs utilizam `a.auth(false)` sem checar papéis compatíveis (`reservaAuth`, `authMaterial`, `confAuth`) (Obs. Item D2).
   - *Mecanismo de Falha*: O bloqueio visual no frontend não impede o consumo direto via HTTP. Handlers internos confiam em verificações parciais ou nulas.
   - *Impacto*: Qualquer usuário autenticado (incluindo operadores ou contas sem grupo) pode extrair inventários de material, escalas em desenvolvimento e prontos de serviço fora de sua alçada.

6. **R2 Item D3 (Antiguidade e Tags)**:
   - *Premissa*: O filtro de seleção avalia 3 origens de funções (`p.funcao_id`, `u2.funcao_id`, `up2.funcao_id`), mas as cláusulas `ORDER BY` avaliam somente `p.funcao_id` (Obs. Item D3).
   - *Mecanismo de Falha*: Militares com graduação definida na conta de usuário recebem antiguidade 999 e caem para o final da lista; a tela principal ignora antiguidade completamente; militares sem tag são filtrados sem aviso.
   - *Impacto*: A conferência desordena a hierarquia militar e omite militares sem transparência para a equipe em campo.

---

## 3. Caveats (Ressalvas e Limitações)

- **Modo Read-Only**: Nenhuma linha de código foi modificada nesta fase. Todas as constatações baseiam-se em análise estática do código-fonte e verificação de suíte de testes existente.
- **Doutrina de Herança de Antiguidade**: A decisão sobre herança de tags de antiguidade de grupos superiores foi fixada na onda de 09/10 como estritamente local (tags pertencentes ao grupo da conferência). Caso o Comando opte por permitir tags herdadas de escalões superiores, a lógica de `validarFuncoesAntiguidadeDoGrupo` demandará expansão adicional.
- **CSP Nonce / Refatoração de Frontend**: A eliminação de `'unsafe-inline'` da CSP exigiria refatoração profunda dos manipuladores de eventos inline em templates HTML do frontend. A blindagem recomendada para a v1.5.4 concentra-se em sanitização/validação severa no backend e uso consistente de `esc()` nos sinks do frontend.

---

## 4. Conclusion & Actionable Fix Strategy (Conclusão e Estratégia de Correção)

### Resumo Executivo das Correções Necessárias

| Item | Arquivos Principais | Problema | Estratégia de Correção |
|---|---|---|---|
| **R1-A (R-1)** | `server_grupos.go`, `server_conferencia.go`, `server_pessoal.go`, `server_material.go`, `server_escalas.go`, `server_relatorios.go`, `helpers.go` | `esc == -1` ignora filtros `esc > 0` e queries `? <= 0` | 1. Criar helper central `exigeEscopo(u)` retornando 403 para `esc < 0`.<br>2. Alterar `filtroGrupoSQL` para retornar ` AND 1=0` quando `escopo < 0`.<br>3. Substituir `(? <= 0 OR ...)` por `(? = 0 OR ...)` em todas as queries SQL.<br>4. Rejeitar ou devolver lista vazia nos ~35 handlers mapeados. |
| **R1-B (R-2)** | `server_pessoal.go:343, 994`, `web/core.js:580`, `web/views_gestao.js:548, 683, 759, 2535, 2637, 2707` | Upload sem validação MIME/tamanho; renderização sem escape (XSS) | 1. Criar `validarFotoBase64(foto string) error` com allowlist de MIME (`png`, `jpeg`, `webp`), decodificação estrita, validação de magic bytes e teto de 512 KB.<br>2. Aplicar `esc()` em todos os atributos `src="${...}"` no frontend. |
| **R1-C (R-3/R-4)** | `server_admin.go:259-274`, `store.go:1348-1403` | Importação não move para `sci.db`; migrações param na v12; rollback incorreto | 1. Implementar registry central `executarMigracoes()` unificando `AbrirStore` e `ReabrirComArquivo`.<br>2. Fechar conexão, limpar WAL antigo, renomear `sci.db` para backup temporário, renomear upload para `sci.db`, reabrir e migrar.<br>3. Em falha, restaurar arquivo anterior e reabrir. |
| **R2-D1** | `onda_0510_conf_escopo.go:145`, `onda_0510_escalas.go:297, 356`, `onda_0510_itens79.go:175` | Fallbacks legados em `chefeComandaSetor`; `usuarios.setor_id` não limpo | 1. Restringir `chefeComandaSetor` para consultar unicamente `chefe_setores`.<br>2. Ao nomear novo chefe ou destituir, atualizar `usuarios.setor_id = NULL` para o chefe destituído.<br>3. Em `hSetoresAgregado`, consultar `chefe_setores` para contagem e nome de chefes. |
| **R2-D2** | `server_escalas.go:1703, 1716-1718`, `server_material.go:2184, 2188, 2209`, `server_relatorios.go:414-424` | Rotas de relatórios/PDFs/dados com `auth(false)` | 1. Proteger rotas de escalas com `reservaAuth`.<br>2. Proteger rotas de material com `authMaterial`.<br>3. Proteger relatórios com verificação explícita de perfil/papel (`confAuth` / `guardaGestaoPessoal`).<br>4. Garantir rejeição para `esc == -1`. |
| **R2-D3** | `onda_0910_conf_antiguidade.go:183-201`, `server_conferencia.go:985, 1038-1053`, `onda_despacho.go:66-156` | Ordenação considera apenas `p.funcao_id`; ausência de ordenação na tela; militares sem tag omitidos sem aviso | 1. Unificar ordenação SQL com `COALESCE(fu.antiguidade, fu_u.antiguidade, fu_up.antiguidade, 999)`.<br>2. Ordenar por antiguidade em `hConferenciaGet`.<br>3. Corrigir junção de `cam_funcao` no PDF.<br>4. Em `hConferenciaIniciar`/`hConferenciaDespachar`, calcular militares ativos do grupo excluídos e devolver no JSON `sem_tag: [nomes]`. |

---

## 5. Verification Method (Método de Verificação Independente)

### Comandos de Compilação e Análise Estática
```bash
# 1. Verificação estática do compilador Go
go vet ./...

# 2. Compilação do binário
go build -o bin/sci_test.exe .

# 3. Execução completa dos testes existentes
go test -count=1 ./...
```

### Novos Testes de Regressão Obrigatórios

1. **Matriz de Escopo `-1` (`escopo_matriz_test.go`)**:
   - Criar conta de operador e gerente sem grupo associado (`GrupoID == nil`).
   - Autenticar e disparar requisições para todos os endpoints de dados:
     - `GET /api/grupos` -> Deve retornar apenas array vazio ou erro 403.
     - `GET /api/conferencias`, `GET /api/conferencia/{id}`, `GET /api/conferencia/{id}/relatorio.pdf` -> 403.
     - `GET /api/pessoas`, `GET /api/pessoas/{id}/ficha`, `GET /api/pessoas/{id}/pdf`, `GET /api/pessoas/{id}/qr` -> 403.
     - `GET /api/material/itens`, `GET /api/material/cautelas`, `GET /api/material/inventario/pdf` -> 403.
     - `GET /api/relatorio`, `GET /api/relatorio.pdf`, `GET /api/export/pessoas` -> 403.
     - `POST /api/escalas/limpar-dia` -> 403 (e comprovar que zero turnos foram apagados).

2. **Blindagem de `foto_base64` (`foto_xss_test.go`)**:
   - `PATCH /api/perfil` com payload `x" onerror="alert(1)"` -> Esperado HTTP 400 Bad Request.
   - `PATCH /api/perfil` com payload `data:text/html;base64,PHNjcmlwdD4...` -> Esperado HTTP 400.
   - `PATCH /api/perfil` com payload de imagem válido > 512 KB -> Esperado HTTP 400.
   - `PATCH /api/perfil` com JPEG válido de 10 KB -> Esperado HTTP 200.
   - Inspeção de código frontend garantindo `esc()` em `web/core.js` e `web/views_gestao.js`.

3. **Ciclo de Importação de Backup e Restart (`backup_swap_test.go`)**:
   - Exportar backup do banco atual via `POST /api/backup`.
   - Modificar dados no banco (adicionar registro marcador).
   - Importar o backup anterior via `POST /api/backup/importar`.
   - Fechar o Store com `st.Close()`.
   - Reabrir o banco com `AbrirStore(st.dataDir)` sobre o arquivo físico `sci.db`.
   - Provar que `sci.db` reflete o estado restaurado (registro marcador ausente) e que a tabela `schema_migrations` contém todas as versões até `versaoSchemaBinario`.

4. **Destituição e Troca de Chefia (`chefe_zumbi_test.go`)**:
   - Nomear usuário A como chefe do Setor 1.
   - Validar que A pode marcar/concluir o Setor 1.
   - Nomear usuário B como chefe do Setor 1.
   - Validar imediatamente que A recebe HTTP 403 ao tentar concluir ou reabrir o Setor 1.
   - Validar que `GET /api/setores/agregado` reporta exatamente 1 chefe para o Setor 1.

5. **Antiguidade e Alerta de Excluídos (`conf_antiguidade_tags_test.go`)**:
   - Cadastrar militar A com tag na pessoa (`p.funcao_id`), militar B com tag apenas no usuário (`u.funcao_id`), e militar C sem tag.
   - Iniciar conferência por antiguidade selecionando a tag de A e B.
   - Validar que a resposta JSON inclui `"sem_tag": ["C"]`.
   - Consultar pré-fechamento e `GET /api/conferencia/{id}` e validar que militar B é ordenado conforme o valor real de antiguidade da tag (e não avaliado como 999).
