# Handoff Report — Especificação Rigorosa da Onda v1.5.4 (SCI)

**Agente**: `teamwork_preview_spec_miner_1` (Specification Investigator / Miner)  
**Data**: 2026-10-10  
**Contexto**: Levantamento e mineração exaustiva de requisitos, interfaces, comportamentos observados, regras de aceite e casos de borda da Onda v1.5.4 do SCI a partir das fontes autoritativas (`docs/INSTRUCOES_AGENTES.md`, `docs/ARQUITETURA.md`, `docs/ROADMAP.md`, `CHANGELOG.md`, `docs/auditoria-completa-1010.md`, `docs/auditoria-escopos-pessoal.md` e código Go/JS).

---

## Features Discovered

| # | Category | Feature | Description | Inputs | Outputs | Error Behavior | Discovered Via |
|---|---|---|---|---|---|---|---|
| F-01 | R1 (P0) | Fechamento da classe de escopo `-1` (R-1) | Impedir que contas sem grupo acessem dados de outras unidades em ~35 rotas de dados | Sessão de usuário com `u.GrupoID == nil` e papel != `admin` (`escopo == -1`) | `403 Forbidden` (`{"ok":false,"erro":"conta sem grupo definido"}`) ou lista vazia onde aplicável | 403 Forbidden imediato via guarda central `exigeEscopo()` / eliminação de bifurcação frouxa `esc > 0` | `docs/ROADMAP.md:50-56`, `docs/ARQUITETURA.md:61-64,507`, `server_grupos.go:202-213`, `helpers.go:112-117` |
| F-02 | R1 (P0) | Blindagem de `foto_base64` contra Stored XSS (R-3) | Validação rigorosa de payload Base64/MIME e teto de tamanho no backend; escape estrito nos sinks do frontend | Payload JSON no `PATCH /api/perfil` e `PATCH /api/usuarios/{id}` contendo `foto_base64` | `200 OK` (`{"ok":true}`) para payload válido de imagem | `400 Bad Request` se prefixo != `data:image/(png\|jpeg\|webp);base64,`, base64 inválido ou tamanho > 512 KB | `docs/ROADMAP.md:58-63`, `docs/ARQUITETURA.md:141,509`, `server_pessoal.go:343,994-1003`, `web/core.js:580`, `web/views_gestao.js:548,2535` |
| F-03 | R1 (P0) | Swap e Importação Atômica de Backup (R-4) | Substituição real por rename atômico para `sci.db`, limpeza correta de WAL/SHM, execução da cadeia completa de migrações (v2..v43) e rollback seguro | `POST /api/admin/backup/importar` (multipart/form-data com campo `arquivo`) | `200 OK` (`{"ok":true,"schema":43,"seguranca":"backups/..."}`) | `400 Bad Request` (arquivo não-SQLite, integridade corrompida, schema > binário) / `500` com restauração do banco original | `docs/ROADMAP.md:65-70`, `docs/ARQUITETURA.md:123,363,510`, `server_admin.go:190-279`, `store.go:1348-1403` |
| F-04 | R2 (Bug Campo) | Erradicação de Chefe-Zumbi de Setor (D1 / R-12) | Remoção de fallbacks legados em `chefeComandaSetor` e invalidação de `usuarios.setor_id` ao destituir ou substituir chefia | Chamada a `POST /api/grupos/{id}/destituir_chefe` ou `POST /api/grupos/{id}/nomear_chefe` | `200 OK` confirmando nomeação/destituição | Usuário destituído recebe `403 Forbidden` imediato ao tentar concluir/reabrir/marcar presença do setor | `docs/ROADMAP.md:74-83`, `docs/ARQUITETURA.md:80-87,518`, `onda_0510_conf_escopo.go:145-164`, `onda_0510_escalas.go:280-385` |
| F-05 | R2 (Bug Campo) | Blindagem de Rotas de Exportação/PDF (D2 / R-2) | Aplicação de guardas de papel/módulo (`reservaAuth`, `authMaterial`, checagem de escopo rígida) em todas as rotas de dados e PDFs | Requisição HTTP para PDFs de escalas, material e conferência | PDF binário com headers `Content-Type: application/pdf` e escopo validado | `403 Forbidden` se solicitante não tiver papel/escopo autorizado; `423 Locked` se módulo em reserva | `docs/ROADMAP.md:85-94`, `docs/ARQUITETURA.md:88-93,508`, `server_escalas.go:1703,1716-1718`, `server_material.go:2184,2188,2209` |
| F-06 | R2 (Bug Campo) | Unificação de Ordenação por Antiguidade e Aviso `sem_tag` (D3 / R-7) | Ordenação uniforme via `COALESCE` unindo as 3 fontes (`p.funcao_id`, `u2.funcao_id`, `up2.funcao_id`); resposta de militares excluídos ao iniciar | `POST /api/conferencia/iniciar` com `funcao_ids`; `GET /api/conferencia/{id}`; pré-fechamento | Resposta de iniciar com `sem_tag: [nomes]`; conferência ordenada por antiguidade respeitando as 3 fontes | `400 Bad Request` se tags inválidas / `403 Forbidden` fora do escopo | `docs/ROADMAP.md:96-109`, `docs/ARQUITETURA.md:97-104,183,513`, `onda_0910_conf_antiguidade.go:119-202`, `server_conferencia.go:519,985` |
| F-07 | R3 (P1) | Proteção de Escopo em Fotos de Usuário (R-5) | Restringir `GET /api/usuarios/{id}/foto` ao próprio grupo ou árvore subordinada ativa do solicitante (LGPD) | `GET /api/usuarios/{id}/foto` autenticado | Imagem binária (`image/jpeg`, `image/png`, `image/webp`) | `403 Forbidden` se o alvo pertencer a outro grupo fora da hierarquia; `404` se foto inexistente | `docs/ROADMAP.md:115`, `docs/ARQUITETURA.md:257,511`, `server_pessoal.go:1015-1047` |
| F-08 | R3 (P1) | Cascata Completa no NUKE de Grupos e Limpeza de FKs (R-6, R-15) | Incluir todas as tabelas dependentes v1.5 na exclusão atômica (`chefe_setores`, `funcao_membros`, `avisos`, `material_conferencias`, `escala_modelos`, `conferencia_escalas`, etc.) | `DELETE /api/grupos/{id}?nuke=1` e `DELETE /api/conferencia/arquivada/{id}` com senha admin | `200 OK` com contadores de remoção e integridade referencial mantida | Rollback limpo com `500` se houver falha; zero violações de constraint FK | `docs/ROADMAP.md:118-121`, `docs/ARQUITETURA.md:357-359,512,521`, `server_grupos.go:140-186`, `server_conferencia.go:776-805` |
| F-09 | R3 (P1) | Restrição de Arquivamento e Descarte de Conferência (R-8) | Alinhar autorização de arquivar/descartar estritamente a gerente e encarregado de pessoal designado (bloquear operador) | `POST /api/conferencia/{id}/arquivar` e `DELETE /api/conferencia/{id}` | `200 OK` confirmando arquivamento/descarte | `403 Forbidden` quando chamado por operador ou sem papel/função autorizada | `docs/ROADMAP.md:126-127`, `docs/ARQUITETURA.md:201,514`, `server_conferencia.go:741-762,882-903` |
| F-10 | R3 (P1) | Isolamento de Setor em `hMudarContexto` (R-9) | Validar que o `setor_id` pertence ao grupo do papel ativo e não reescrever a coluna global `usuarios.setor_id` | `POST /api/sessao/contexto` com `{papel_id, setor_id}` | `200 OK` com dados do usuário na nova sessão | `400 Bad Request` se o setor não pertencer ao grupo do papel ativo | `docs/ROADMAP.md:113-114`, `docs/ARQUITETURA.md:90-91,243,515`, `mensagens.go:17-80` |
| F-11 | R3 (P1) | Allowlist de Solicitante e Alvo em `hUsuarioPapelDel/Add` (R-10) | Bloquear fail-open em `hUsuarioPapelDel` (chefe_setor não remove papéis) e validar que o alvo pertence à árvore do gerente | `POST/DELETE /api/usuarios/{id}/papeis[/{pid}]` | `200 OK` confirmando alteração de papel | `403 Forbidden` se solicitante for operador ou chefe de setor, ou se alvo for de outro grupo | `docs/ROADMAP.md:112`, `docs/ARQUITETURA.md:244,516`, `mensagens.go:84-138,233-278` |
| F-12 | R3 (P1) | Blindagem MIME e Content-Disposition no Drive (R-11) | Allowlist restrita de MIME para inline (`image/*`, `application/pdf`), sanitização de filename e cabeçalho `nosniff` | `GET /api/drive/download/{id}?inline=1` e upload multipart | Stream de dados com headers seguros (`Content-Disposition`, `X-Content-Type-Options: nosniff`) | Força `Content-Disposition: attachment` para tipos executáveis/HTML | `docs/ROADMAP.md:116-117`, `docs/ARQUITETURA.md:329,517`, `drive.go:701-707,764-774` |
| F-13 | R3 (P1) | Hierarquia de Mensageria por Papel (R-14) | Impedir que contas sem grupo enviem mensagens globais e garantir validação de regras de envio para todos os papéis | `POST /api/mensagens/enviar` com destinatários | `200 OK` confirmando envio de mensagem | `403 Forbidden` para contas sem grupo ou operadores tentando enviar fora do grupo/cadeia | `docs/ROADMAP.md:124-125`, `docs/ARQUITETURA.md:159,315,520`, `mensagens.go:640-678` |
| F-14 | R3 (P1) | Invalidação de Sessões na Troca de Senha (R-16) | Revogar sessões ativas no banco ao trocar senha via `hTrocarSenha` ou `hUsuarioSenha` | `POST /api/senha` ou `POST /api/usuarios/{id}/senha` | `200 OK` confirmando alteração de senha | Sessões antigas passam a receber `401 Unauthorized` imediatamente | `docs/ROADMAP.md:128`, `docs/ARQUITETURA.md:105,242,522`, `server_pessoal.go:147-183` |
| F-15 | R4 (CI) | Fortalecimento do `ci.sh` e Higiene de Repositório | Execução obrigatória de `go vet ./...` e `go test -count=1 ./...` no CI; expurgo de `dados.tar.gz` da árvore | Execução de `bash ci.sh` em ambiente Linux/CI | Saída limpa `CI VERDE` após suíte de testes completa | Interrupção imediata (`exit 1`) caso qualquer teste ou `vet` falhe | `docs/ROADMAP.md:130-132`, `docs/ARQUITETURA.md:534`, `ci.sh` |
| F-16 | R5 (Gov) | Governança e Atualização Documental | Atualização cruzada de `docs/ARQUITETURA.md`, `docs/ROADMAP.md`, `CHANGELOG.md` e bump único de cache-bust | Alterações de código e documentação | Documentos sincronizados sem drift | N/A (revisão de conformidade no gate de saída) | `docs/INSTRUCOES_AGENTES.md:27-35`, `docs/ROADMAP.md:224-233`, `docs/ARQUITETURA.md:8-12` |

---

## Edge Cases

| # | Feature | Input | Observed Behavior |
|---|---|---|---|
| E-01 | R-1 (Escopo -1) | Conta criada sem grupo acessa `GET /api/pessoas`, `GET /api/conferencias`, `GET /api/material/itens` | Sem a correção: `escopo == -1` cai no ramo `else` (sem filtro), vazando todos os militares e conferências do banco. Com a correção: retorna `403 Forbidden` imediatamente. |
| E-02 | R-1 (Escala Limpar Dia) | Conta com `escopo == -1` chama `POST /api/escalas/limpar-dia` com data válida | Sem a correção: SQL executa `(? <= 0 OR grupo_id = ?)` com `-1`, apagando turnos de escala de TODOS os grupos na data. Com a correção: bloqueado com 403 Forbidden. |
| E-03 | R-2 (XSS foto_base64) | Payload `x" onerror="alert(document.cookie)"` no `FotoBase64` do `hPerfilSet` | Sem a correção: salvo cru no banco e renderizado em `<img src="${...}">` sem escape, disparando JavaScript. Com a correção: rejeitado no backend com 400 Bad Request; sinks usam `esc()`. |
| E-04 | R-2 (Teto foto_base64) | Payload válido de imagem Base64 com 513 KB | Rejeitado no backend com 400 Bad Request por ultrapassar o teto estrito de 512 KB. |
| E-05 | R-4 (Import Backup Legado) | Upload de banco SQLite válido com schema v10 | Sem a correção: `ReabrirComArquivo` migra somente até v12, deixando tabelas de v13..v43 inexistentes. Com a correção: executa a cadeia completa até schema v43. |
| E-06 | R-4 (Import Crash/Falha) | Falha de integridade ou DDL durante a reabertura do arquivo importado | Sem a correção: rollback tenta reabrir o próprio arquivo defeituoso `sci.db.novo`. Com a correção: restaura o backup de segurança original preservando o caminho original. |
| E-07 | D1 (Chefe-Zumbi) | Usuário A comanda Setor 1; Gerente nomeia Usuário B para Setor 1 | Sem a correção: `usuarios.setor_id` de A permanece apontando para Setor 1; `chefeComandaSetor` retorna `true` pelo fallback 1. Com a correção: `usuarios.setor_id` de A é limpo e fallback é removido; A perde comandos de Setor 1 imediatamente. |
| E-08 | D1 (Multi-Chefia) | Usuário comanda Setores 1 e 2; destitui comando de Setor 1 | Destituição remove apenas o registro do Setor 1 em `chefe_setores`. Como ainda possui Setor 2, o papel `chefe_setor` em `usuario_papeis` é preservado, mas comandos de Setor 1 são revogados. |
| E-09 | D2 (PDFs de Escalas/Material) | Operador do Grupo A acessa URL `/api/material/conferencias/{id_do_grupo_b}/pronto.pdf` | Sem a correção: rota possui `auth(false)` e query SQL busca por ID sem checar grupo, entregando o PDF. Com a correção: rota envelopada em `authMaterial` com validação de escopo, retornando 403. |
| E-10 | D3 (Antiguidade na Conta) | Militar vinculado a uma função de antiguidade via `usuarios.funcao_id` ou `usuario_papeis.funcao_id`, mas sem `pessoas.funcao_id` | Sem a correção: `fu.antiguidade` é NULL, caindo em `COALESCE(fu.antiguidade, 999)` no final da lista. Com a correção: `COALESCE(fu.antiguidade, fu_u.antiguidade, fu_up.antiguidade, 999)` posiciona o militar na ordem exata de precedência. |
| E-11 | D3 (Militares Sem Tag) | Grupo possui 10 militares ativos; conferência iniciada filtrando 2 tags que cobrem apenas 6 militares | Ao iniciar, o backend calcula a diferença do efetivo ativo do grupo e devolve `sem_tag: [nomes]` com os 4 militares excluídos, permitindo que a interface emita alerta ao operador. |
| E-12 | R-5 (Foto LGPD) | Operador do Grupo A tenta baixar avatar `/api/usuarios/{id_grupo_b}/foto` | Sem a correção: endpoint autenticado devolve a imagem sem checagem de grupo. Com a correção: validação de escopo bloqueia com 403 Forbidden. |
| E-13 | R-6 (NUKE de Grupo Rico) | Executar NUKE em grupo contendo chefias designadas, avisos no mural, conferências de material e modelos de escala | Sem a correção: `DELETE FROM grupos` aborta com `FOREIGN KEY constraint failed`. Com a correção: cascata atômica prévia limpa todas as tabelas dependentes v1.5 com sucesso. |
| E-14 | R-8 (Descarte por Operador) | Operador do grupo tenta chamar `DELETE /api/conferencia/{id}` em conferência aberta | Sem a correção: `confAuth` autoriza operador e handler permite descarte. Com a correção: validação de papel bloqueia operador com 403 Forbidden (exclusivo para gerente ou encarregado de pessoal). |
| E-15 | R-9 (Setor de Outro Grupo) | Usuário com papel ativo no Grupo A chama `hMudarContexto` enviando `setor_id` pertencente ao Grupo B | Sem a correção: handler valida apenas `setor.ativo = 1` e aceita a troca. Com a correção: validação exige `setor.grupo_id == papel_ativo.grupo_id`, retornando 400 Bad Request. |
| E-16 | R-10 (Fail-Open de Papéis) | Chefe de setor chama `DELETE /api/usuarios/{id}/papeis/{pid}` | Sem a correção: código só bloqueia operador (`u.Papel == "operador"`), permitindo que chefe de setor remova qualquer papel. Com a correção: allowlist estrita permite apenas admin e gerente. |
| E-17 | R-11 (Drive HTML Inline) | Upload de arquivo `exploit.html` baixado com `?inline=1` | Sem a correção: servidor envia `Content-Type: text/html` com `inline`, executando script na origem do SCI. Com a correção: tipos fora da allowlist são forçados a `Content-Disposition: attachment` com `X-Content-Type-Options: nosniff`. |
| E-18 | R-16 (Troca de Senha) | Atacante possui cookie de sessão roubado; vítima troca sua senha via `/api/senha` | Sem a correção: hash é atualizado mas `sessoes` permanecem ativas. Com a correção: sessões anteriores são deletadas do banco, forçando reautenticação imediata. |

---

## 1. Observation (O que foi observado)

A análise foi conduzida através de inspeção direta de arquivos fonte, documentos de arquitetura e histórico de testes:

1. **Escopo -1 e Filtros Frouxos (R-1 / P0-1):**
   - `server_grupos.go:202-213`:
     ```go
     func escopoDoUsuario(u *Usuario) int64 {
         if u == nil { return -1 }
         if u.Papel == "admin" { return 0 }
         if u.GrupoID == nil { return -1 }
         return *u.GrupoID
     }
     ```
   - `helpers.go:112-117`:
     ```go
     func filtroGrupoSQL(escopo int64, alias string) string {
         if escopo <= 0 { return "" }
         return ` AND ` + alias + `.grupo_id = ?`
     }
     ```
     *Fato verificado*: Para `escopo == -1`, `escopo <= 0` é verdadeiro, retornando `""` e eliminando o filtro de grupo em consultas de exportação e relatórios.
   - `server_conferencia.go:836-855`: Em `hConferenciaList`, a ramificação é estritamente `if escopo > 0 { ... } else { ... }`, fazendo com que contas com `escopo == -1` caiam no `else` e recebam conferências de todos os grupos.

2. **Injeção em `foto_base64` (R-3 / P0-2):**
   - `server_pessoal.go:343` e `server_pessoal.go:994-1003`: O campo `req.FotoBase64` é gravado diretamente via `UPDATE usuarios SET foto_base64 = ?` com apenas `strings.TrimSpace`.
   - `web/core.js:580`: `<img src="${usuario.foto_base64}" class="sidebar-avatar-img" alt="Foto">` (sem escape).
   - `web/views_gestao.js:548`: `<img src="${u.foto_base64}" style="width:100%; height:100%; object-fit:cover">` (sem escape).
   - `web/views_gestao.js:2535`: `<img src="${fotoAtual}" alt="Foto 1x1">` (sem escape).
   - `main.go:111-117`: CSP global define `script-src 'self' 'unsafe-inline'`, permitindo a execução de manipuladores como `onerror`.

3. **Importação de Backup (R-4 / P0-3):**
   - `server_admin.go:260-274`:
     - O arquivo importado é renomeado para `novo := a.st.arquivo + ".novo"`.
     - Nunca é executado `os.Rename(novo, a.st.arquivo)`. Ao reiniciar o serviço, o binário abre `sci.db` original, perdendo a importação.
     - Em caso de erro na reabertura, linha 269 executa `a.st.ReabrirComArquivo(a.st.arquivo)`, que tenta reabrir `novo` (pois `s.arquivo` já havia sido alterado em `store.go:1354`).
   - `store.go:1372-1402`: `ReabrirComArquivo` chama explicitamente apenas até `s.migrarV12()`, ignorando as migrações v13 a v43.

4. **Chefe-Zumbi de Setor (D1 / R-12):**
   - `onda_0510_conf_escopo.go:153-155`:
     ```go
     if u.SetorID != nil && *u.SetorID == setorID {
         return true
     }
     ```
   - `onda_0510_escalas.go:315`: Comentário e código confirmam: `"usuarios.setor_id NÃO é mexido aqui"`.
   - Ao nomear chefe para um setor (`hGrupoNomearChefe`), o anterior perde o registro em `chefe_setores`, mas mantém `usuarios.setor_id` preenchido, continuando a autorizar comandos via fallback.

5. **Rotas Desprotegidas de PDF e Dados (D2 / R-2):**
   - `server_escalas.go:1703,1716,1717,1718`: Registradas com `a.auth(false, ...)` em vez de `a.reservaAuth(...)`.
   - `server_material.go:2184,2188,2209`: Registradas com `a.auth(false, ...)`. Em particular, `hMaterialConferenciaPDF` (`server_material.go:731-764`) busca conferência por ID sem filtrar grupo ou verificar permissão de material.
   - `server_conferencia.go:1095-1135`: `hConferenciaPDF` testa apenas `if esc := escopoDoUsuario(uCtx); esc > 0`, permitindo bypass para `esc == -1`.

6. **Ordenação de Antiguidade e Tags (D3 / R-7):**
   - `onda_0910_conf_antiguidade.go:201`: `ORDER BY COALESCE(fu.antiguidade, 999), p.nome_guerra` ignora `fu_u.antiguidade` e `fu_up.antiguidade`.
   - `server_conferencia.go:985`: `hConferenciaGet` ordena por `ORDER BY pr.situacao, p.nome_guerra`, sem suporte a antiguidade.
   - `server_conferencia.go:519`: `hConferenciaIniciar` responde apenas `{"id": id, "data": data, "conferencia_id": id}`, omitindo lista de militares fora do filtro.

7. **Guardas P1 Remanescentes:**
   - `server_pessoal.go:1015-1045`: `hUsuarioFotoGet` busca foto por ID sem validar grupo do solicitante (R-5).
   - `server_grupos.go:144-173`: NUKE omite `chefe_setores`, `funcao_membros`, `avisos`, `material_conferencias`, `escala_modelos`, `setor_sugestoes`, `grupo_setor_responsaveis` (R-6).
   - `server_conferencia.go:799-803`: `hConferenciaExcluirArquivada` omite `conferencia_escalas` (R-15).
   - `server_conferencia.go:741,882`: `hConferenciaArquivar` e `hConferenciaDescartar` permitem execução por operador (R-8).
   - `mensagens.go:44-50,67`: `hMudarContexto` aceita setor de qualquer grupo e sobrescreve globalmente `usuarios.setor_id` (R-9).
   - `mensagens.go:246-249`: `hUsuarioPapelDel` bloqueia apenas operador, permitindo fail-open para `chefe_setor` (R-10).
   - `drive.go:764-774`: `hDriveDownload` permite servir qualquer Content-Type como `inline` sem `nosniff` e com header `Content-Disposition` sem sanitização (R-11).
   - `mensagens.go:642`: `hMensagensEnviar` condiciona restrição de operador a `u.GrupoID != nil`, deixando operador sem grupo sem restrições (R-14).
   - `server_pessoal.go:175,210`: `hTrocarSenha` e `hUsuarioSenha` alteram `senha_hash` sem invalidar registros em `sessoes` (R-16).

8. **CI e Artefatos:**
   - `ci.sh`: Executa apenas build e health/smoke simples; omite `go vet` e `go test`.
   - `dados.tar.gz`: Presente na raiz do repositório (arquivo de 217 KB contendo cópia do banco).

---

## 2. Logic Chain (Cadeia de Raciocínio)

1. **R1 - P0s:**
   - Se `escopoDoUsuario` retorna `-1` para usuários sem grupo e handlers bifurcam apenas `if esc > 0`, qualquer conta sem grupo assume o comportamento de conta de infraestrutura/admin (`esc == 0`), obtendo visualização irrestrita de dados institucionais. Portanto, uma guarda central `exigeEscopo()` e o fechamento de todas as ramificações frouxas é pré-requisito mandatório para a segurança do sistema.
   - A permissividade no armazenamento de strings base64 aliada à renderização direta de templates HTML permite injeção arbitrária de tags e manipuladores de eventos (`onerror`). A imposição de validação rigorosa de cabeçalho MIME (`data:image/(png|jpeg|webp);base64,`), decodificação estrita e teto de 512 KB no backend, combinada com sanitização nos sinks (`esc()`), elimina o vetor de XSS armazenado.
   - O processo de importação de banco só é resiliente se garantir atomicidade do arquivo em disco (`sci.db`), limpeza prévia de WAL associado, execução idempotente de todas as migrações (v2..v43) e restauração garantida em caso de falha. Sem isso, o banco é descarregado ou corrompido no reinício do processo.

2. **R2 - Bugs de Campo:**
   - O conceito de "chefe-zumbi" decorre da duplicidade de fontes de autoridade: `chefe_setores` (tabela canônica v35) versus `usuarios.setor_id` (coluna legada mantida como efeito colateral). Ao destituir ou substituir uma chefia, a ausência de limpeza de `usuarios.setor_id` somada ao fallback em `chefeComandaSetor` permite que o antigo chefe continue assinando e fechando conferências setoriais. A eliminação dos fallbacks e a invalidação da coluna na destituição restauram o princípio de autoridade única.
   - A exposição de PDFs com `auth(false)` sem checagem de escopo/reserva cria canais laterais de exfiltração de dados que contornam as políticas dos módulos de Escalas, Material e Conferência.
   - O mecanismo de antiguidade precisa de simetria entre o filtro de seleção e o critério de ordenação: militares com função cadastrada no usuário ou no papel devem ser ordenados pela mesma precedência hierárquica. Além disso, a conferência setorial não pode omitir membros do grupo em silêncio; a resposta da criação deve sinalizar a lista de militares fora do filtro (`sem_tag`) para conferência e auditoria.

3. **R3 - Guardas P1:**
   - Todas as entidades com integridade referencial devem ser expurgadas em cascata ordenada dentro da transação de exclusão de grupo (NUKE) para cumprir as restrições de chave estrangeira do SQLite (`foreign_keys(1)`).
   - Privilégios de modificação de credenciais e troca de contexto devem ser estritamente contidos dentro da sessão e do escopo do solicitante, garantindo que sessões comprometidas sejam revogadas na troca de senha e que operadores não acessem áreas restritas a gerentes.

4. **R4 e R5 - Governança e Processo:**
   - O portão de qualidade (`ci.sh`) deve refletir a suíte oficial do repositório (`go test -count=1 ./...` e `go vet ./...`) para prevenir regressões silenciosas.
   - Como o frontend do SCI utiliza empacotamento embutido (`//go:embed web`), qualquer alteração em arquivos `.js` requer bump obrigatório de cache-bust (`web/index.html` e `web/lazy.js`) para evitar que clientes utilizem scripts em cache incompatíveis.

---

## 3. Caveats (Ressalvas)

- **Fator CGO / Race**: O teste com flag `-race` exige compilador C (cgo/gcc), não disponível nativamente no ambiente de desenvolvimento Windows padrão do projeto. O gate oficial com `-race` deve ser validado no host Linux conforme previsto em `docs/INSTRUCOES_AGENTES.md`.
- **Decisões de Comando D-1 a D-5**: Decisões estratégicas pendentes (como herança de catálogo de antiguidade entre grupos superiores ou armazenamento físico de anexos de material em disco) pertencem ao comando e estão planejadas para ondas posteriores (v1.5.5 / v1.6). Na v1.5.4, a regra é manter catálogo sem herança entre grupos e focar estritamente na eliminação dos defeitos mapeados.

---

## 4. Conclusion (Conclusão)

A especificação técnica da Onda v1.5.4 está completamente mapeada, com identificação exata das causas-raízes, arquivos, números de linha, contratos de API e critérios de aceite.

**Diretrizes de Implementação para os Agentes de Execução:**
1. **Frente A (R1 - P0s):** Criar guarda central de escopo e aplicar em todas as rotas de dados; implementar sanitizador de Base64 em `server_pessoal.go` e escapar sinks no frontend; corrigir o pipeline de importação em `server_admin.go` e `store.go`.
2. **Frente B (R2 - Bugs de Campo):** Limpar `usuarios.setor_id` em `hGrupoDestituirChefe` e remover fallbacks em `chefeComandaSetor`; proteger rotas de PDF com as guardas correspondentes (`reservaAuth`, `authMaterial`); unificar `ORDER BY` da antiguidade com `COALESCE` triplo e retornar `sem_tag` em `hConferenciaIniciar`.
3. **Frente C (R3 - P1s):** Aplicar correções de escopo e permissões em fotos, NUKE/FKs, arquivamento/descarte, contexto de setor, papéis, Drive, mensagens e invalidação de sessões.
4. **Frente D (R4 & R5 - CI & Docs):** Atualizar `ci.sh`, remover `dados.tar.gz`, sincronizar `docs/ARQUITETURA.md`, `docs/ROADMAP.md`, `CHANGELOG.md` e realizar bump de cache-bust para `v371`.

---

## 5. Verification Method (Método de Verificação Independente)

Os testes e verificações abaixo devem ser executados na raiz do repositório para validar o aceite completo da Onda v1.5.4:

1. **Compilação e Análise Estática:**
   ```powershell
   go vet ./...
   go build ./...
   ```
   *Critério*: Zero erros ou apontamentos.

2. **Execução Integral da Suíte de Testes:**
   ```powershell
   go test -count=1 ./...
   ```
   *Critério*: Todos os testes verdes (236+ testes existentes mais novos testes de regressão).

3. **Verificações Específicas de Regressão por Item:**
   - **Teste de Matriz de Escopo (Item A / R-1):**
     Executar teste HTTP simulando conta com `escopo == -1` contra todas as rotas de dados listadas em `ARQUITETURA.md`, confirmando `403 Forbidden` em 100% dos casos.
   - **Teste de Blindagem XSS (Item B / R-2):**
     Submeter payload `data:image/png;base64,x" onerror=alert(1)` e `data:text/html;base64,...` em `PATCH /api/perfil`; verificar rejeição com `400 Bad Request`.
   - **Teste de Persistência de Importação (Item C / R-4):**
     Realizar importação de backup legado, fechar a instância do `Store`, reabrir a partir do arquivo em disco e validar persistência de dados e schema v43.
   - **Teste de Destituição de Chefia (Item D1 / R-12):**
     Nomear militar A como chefe de setor, substituí-lo por militar B e comprovar bloqueio `403 Forbidden` imediato em ações de conclusão/reabertura setorial por A.
   - **Teste de PDFs e Permissões (Item D2 / R-2):**
     Testar acesso a `/api/escalas/pdf`, `/api/material/inventario/pdf` e `/api/material/conferencias/{id}/pronto.pdf` por usuários sem permissão e comprovar bloqueio com `403 Forbidden`.
   - **Teste de Ordenação por Antiguidade e Aviso (Item D3 / R-7):**
     Verificar ordenação correta de militar com função vinculada apenas ao usuário/papel; verificar retorno de `sem_tag` ao iniciar conferência com filtro parcial.
   - **Teste de Exclusão NUKE com Grupo Rico (Item E / R-6):**
     Criar grupo completo com avisos, chefes, conferência de material e templates de escala; executar NUKE e verificar deleção sem erro de chave estrangeira.
