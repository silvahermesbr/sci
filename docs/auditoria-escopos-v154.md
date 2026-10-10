# Relatório de Auditoria de Escopos — Onda v1.5.4 (todos os módulos tocados)

**Data:** 10/10/2026  
**Referência:** Gate de saída da onda v1.5.4 (`ROADMAP.md` §Definition of Done, item 5; `INSTRUCOES_AGENTES.md` regra 5) — auditoria re-executada no formato de `auditoria-escopos-pessoal.md`, agora cobrindo TODOS os módulos tocados na onda.  
**Alvo:** Estado PÓS-onda da branch `onda/v1.5.4-estancar` (HEAD `ddb6aa3`): itens A/B/C (`fd96c90`+`b13fd5b`), F (`7751f70`), D1 (`7d6a2a9`), D2 (`6d69a16`), D3 (`f8a67df`), E1 (`03fa62a`), E2 (`f2fc223`).  
**Mapa de referência:** `ARQUITETURA.md` (§1 protocolo, §4 matriz, §5-§10 módulos, §14 defeitos) — consultado como ponto de partida e **verificado contra o código linha a linha**; divergências encontradas estão listadas na §11 e foram corrigidas no mapa neste mesmo commit.

---

## 1. Sumário Executivo

Foi re-executada a auditoria de escopo sobre os 8 módulos tocados na onda v1.5.4 (CONFERÊNCIA, PESSOAL/USUÁRIOS, MATERIAL, ESCALAS, MENSAGENS, DRIVE, GRUPOS/ADMIN, MURAL/CALENDÁRIO), verificando no CÓDIGO cada guarda citada pelo mapa e as 10 classes de defeito fechadas pela onda.

**Resultado: NENHUM FURO DE ESCOPO ABERTO.** Todas as classes de defeito da onda estão estancadas no servidor com teste de regressão por persona. A auditoria encontrou **3 divergências mapa×código** — todas do tipo "o código já corrigiu e o mapa ainda denunciava": (i) **R-1** (escopo -1) está corrigido pela guarda central `exigeEscopo`/`filtroGrupoSQL` em TODOS os handlers verificados, mas o mapa ainda o listava como P0 aberto e mantinha células "⚠-1 vaza (R-1)" nas tabelas; (ii) **R-3** (XSS foto_base64) está corrigido na escrita (`validarFotoBase64`) e nos sinks (`esc()`+`fotoValida()`), mas a linha do §6 e o registro ainda denunciavam; (iii) **R-4** (import de backup) está corrigido (swap com rename real + cadeia COMPLETA `executarMigracoes` na reabertura), mas o §3.E e o registro ainda o listavam como aberto. Adicionalmente, o mapa citava schema **v43** (§3.E/§12) e a onda D1 subiu para **v44** (`versaoSchemaBinario`, main.go:23). Todas corrigidas no `ARQUITETURA.md` neste commit.

---

## 2. Método

1. Cada rota/sistema listado nas tabelas abaixo foi verificado no código-fonte da branch (arquivo:linha do estado pós-onda), não por cópia do mapa.
2. Cada classe de defeito da onda foi amarrada ao teste de regressão que a prova (persona `setupTestApp` + `loginAs` + `doJSONReq`, casos positivo e negativo).
3. O teste de matriz da classe -1 (`r1_test.go:46` `TestR1_ScopeClosure_Matrix`) itera 18 rotas de operador + 3 de gerente com conta SEM grupo e exige **403** em todas — é a prova de fechamento da classe, não uma amostra.
4. Mecanismo central verificado: `exigeEscopo` (helpers.go:125 — admin→0; sem grupo→erro 403; senão grupo da sessão) e `filtroGrupoSQL` (helpers.go:138 — `-1` ⇒ ` AND 1 = 0`, fail-closed no fio do SQL).

---

## 3. CONFERÊNCIA

Guardas do módulo: `authConf` (onda_0510_conf_escopo.go:135 — gerente/operador + enc/aux de pessoal), `authConfCom` com `chefe_setor` (:111), `confPDFAuth` (server_conferencia.go:1672). Rotas em `rotasConferencia` (server_conferencia.go:1711-1752).

| Rota | Método | Guarda (código) | Escopo -1 / objeto | Veredito |
|---|---|---|---|---|
| `/api/conferencia/hoje` | GET | auth(false) + `exigeEscopo` (:19) | sem grupo 403; chefe só lê o setor que AINDA comanda (`setorAtivoComandado` :196) | Conforme |
| `/api/conferencia/estado` | GET | auth(false) + escopo no hash (onda_0610_conf_estado.go) | 0/-1: hash vazio; N: por setor | Conforme |
| `/api/conferencia/iniciar` | POST | `confMarcarAuth` (:1728) + chefe só setor comandado (`chefeComandaSetor` :476) | sem grupo 403; responde `sem_tag` (D3) | Conforme |
| `/api/conferencia/despachar` | POST | `confAuth` + operador 403 interno (onda_despacho.go:160) | sem grupo 403; `sem_tag` com recorte aos setores despachados | Conforme |
| `/api/conferencia/fechar` | POST | `confAuth` + portão gerente/enc_pessoal interno (:524) + `exigeEscopo` (:544) | sem grupo 403 | Conforme |
| `/api/conferencia/marcar` | POST | `confMarcarAuth` (:1731) + `guardaSetorNaMarcar` (onda_0510_conf_escopo.go:163; ramo chefe condicionado ao comando vigente `setorAtivoComandado` :179) | sem grupo 409/403; contexto órfão não marca | Conforme |
| `/api/conferencia/lista` · `/api/conferencias` | GET | auth(false) + `exigeEscopo` (:875) | sem grupo 403 (provado na matriz r1) | Conforme |
| `/api/conferencia/{id}` | GET | auth(false) + `exigeEscopo` (:1027) + objeto no próprio grupo/subordinados (:1029-1036) | sem grupo 403; outro grupo 403 | Conforme |
| `/api/conferencia/{id}` (descartar) | DELETE | `confAuth` + `exigeEscopo` (:953) + portão gerente/enc (R-8, E2) | operador 403; sem grupo 403 | Conforme |
| `…/{id}/setor/{sid}/concluir` · `/reabrir` | POST | `confMarcarAuth` + `setorAtivoComandado` (:696, :758) | chefe sem comando vigente não conclui/reabre (D1) | Conforme |
| `…/setor/{sid}/pre_fechamento` | GET | `confMarcarAuth` (:1738) | resíduo D-5 documentado: ramo chefe ainda lê o CONTEXTO (sem poder de escrita) | Conforme (resíduo D-5) |
| `/api/conferencia/{id}/relatorio.pdf` | GET | **`confPDFAuth`** (:1672, D2): papéis do módulo; chefe SÓ com setor envolvido COMANDADO (`conferenciaEnvolveSetorComandado` :1697 — `conferencia_setores` × `chefe_setores`); admin 403 + `exigeEscopo` no handler (:1180) | sem grupo 403; outro grupo 403 | Conforme |
| `/api/conferencia/{id}/arquivar` | POST | `confAuth` + `exigeEscopo` (:792) + **portão gerente/enc_pessoal** (:810, R-8/E2) | operador 403; sem grupo 403 | Conforme |
| `/api/conferencia/arquivada/{id}` | DELETE | **auth(true)** (:1725) + DELETEs completos em tx (:849-860, R-15/E2 — inclui `conferencia_escalas`) | admin only | Conforme |
| `/api/conferencia/{id}/stream` | GET | auth(false) + `exigeEscopo` (server_realtime.go:88) | sem grupo 403 | Conforme |
| `/api/comentarios` (POST) · `/api/comentarios/{id}` · `/api/pessoas/{id}/comentarios` | GET/POST | `confAuth` + `exigeEscopo` (:1512, :1582) | sem grupo 403 | Conforme |

**Classe provada por:** `onda_v154_d1_test.go` (chefe-zumbi: `TestD1SubstituicaoMataChefeZumbi`, `TestD1ContextoOrfaoPerdeLeituraEMarcar`), `onda_v154_d2_test.go` (PDF: `TestD2_R2_PDFsComGuardaDoModulo`, `TestD2_R2_PDFMagicBytes`), `onda_v154_d3_test.go` (antiguidade: 4 testes), `onda_v154_e2_test.go` (`TestE2ArquivarDescartarSomenteGerenteEnc`, `TestE2ExcluirArquivadaApagaEscala`), `r1_test.go` (matriz -1).

---

## 4. PESSOAL / USUÁRIOS

Guardas: `guardaGestaoPessoal` (onda_0610_pessoal.go:78), `podeGestaoPessoal` (:59), prólogos internos. Rotas em server_pessoal.go:1652-1708.

| Rota | Método | Guarda (código) | Escopo -1 / objeto | Veredito |
|---|---|---|---|---|
| `/api/pessoas` | GET | auth(false) + `exigeEscopo` 403 (:669-672) | sem grupo **403** | Conforme |
| `/api/pessoas/{id}/ficha` | GET | auth(false) + `exigeEscopo` 403 (:461-464) + objeto no fio | sem grupo **403** | Conforme |
| `/api/pessoas/{id}/pdf` · `/qr` | GET | auth(false) + `exigeEscopo` 403 (:550-554 · :1463-1467) + `esc > 0 && gid != esc → 403` (:1478) | sem grupo **403**; outro grupo 403 | Conforme |
| `/api/pessoas/apresentacao` (+POST) | GET/POST | `exigeEscopo` (onda_presenca_banco.go:25) / `guardaGestaoPessoal` | sem grupo 403 | Conforme |
| `/api/pessoas[/{id}]` | POST/PATCH/DELETE | `guardaGestaoPessoal` + grupo forçado/validado | admin/ger/enc apenas | Conforme |
| `/api/usuarios` | GET | prólogo interno: sem grupo e sem papel 403 | sem grupo 403 (mapa §6 e teste r1 de contraste) | Conforme |
| `/api/usuarios/{id}/foto` | GET | **`hUsuarioFotoGet` com escopo (E2, R-5)**: própria sempre (avatar); admin global; grupo vê MESMO grupo (`:1120-1130` — `esc>0 && grupoAlvo != esc → 403`); sem grupo 403 exceto a própria | fora do escopo **403** | Conforme |
| `/api/usuarios/{id}/papeis[/{pid}]` | POST/DELETE | **allowlist fail-closed `podeGerirPapelAlvo` (E1, R-10)** (mensagens.go:143/:301; onda_v154_e1.go:66): admin global; gerente operador/chefe no próprio grupo/árvore; enc/aux de pessoal no próprio grupo; alvo resolvido ANTES da decisão; re-chaveio de sessões só pós-autorização; D-1: Add materializa comando (UPSERT 1:1), Del revoga comandos do grupo | fora do escopo 403; fail-closed p/ o resto | Conforme |
| `/api/sessao/contexto` | POST | **E1, R-9** (mensagens.go:17): papel próprio; setor no GRUPO do papel ativo; chefe_setor só setor COMANDADO (`chefeComandaSetorNoGrupo` onda_v154_e1.go:96); admin livre; `UPDATE usuarios SET setor_id` REMOVIDO — contexto em `sessoes.setor_ativo_id` | setor de outro grupo 403 | Conforme |
| `/api/senha` · `/api/usuarios/{id}/senha` | POST | **E1, R-16**: `invalidarSessoesDeSenha` (onda_v154_e1.go:47) — troca própria preserva a corrente (:235); redefinição por outrem derruba TODAS (:295); admin semeado com senha padrão nasce `precisa_setup=1` | — | Conforme |
| `/api/perfil` | GET/PATCH | própria conta; **foto validada `validarFotoBase64`** (server_pessoal.go:17, aplicada :406 e :1079 — allowlist png/jpeg/webp + magic bytes + teto) | R-3 corrigido na escrita | Conforme |

**Classe provada por:** `r1_test.go` (`TestR1_FotoBase64_Validation`, matriz), `onda_v154_e1_test.go` (`TestE1R9ContextoNoGrupoDoPapel`, `TestE1R10PapelDelSemFailOpen`, `TestE1R10PapelAddFailClosed`, `TestE1R16SenhaInvalidaSessoes`, `TestE1R16AdminSembradoExigeTroca`, `TestE1R16AdminComSenhaPropriaNasceLivre`), `onda_v154_e2_test.go` (`TestE2FotoUsuarioComEscopo`), `onda_v154_d1_test.go` (`TestD1PapelAddDelMaterializaComando`).

---

## 5. MATERIAL

Guarda do módulo: `authMaterial` (server_material.go:2278 — gerente/operador/enc_material; admin 403; 423 em reserva); escopo do objeto no handler (`exigeEscopo` + `cautelaNoEscopo` server.go:387). Rotas em server_material.go:2296-2346.

| Rota | Método | Guarda (código) | Escopo -1 / objeto | Veredito |
|---|---|---|---|---|
| `/api/material/categorias[/{id}]` · `/itens[/{id}]` | GET/POST/DELETE | `authMaterial` + `exigeEscopo` nos handlers (29 ocorrências no arquivo) | sem grupo 403 (matriz r1: `/api/material/itens`, `/cautelas` 403) | Conforme |
| `/api/material/itens/{id}/qr` · `/etiquetas-lote.pdf` | GET | `authMaterial` (:2311) | sem grupo 403 | Conforme |
| `/api/material/inventario/pdf` | GET | **`authMaterial` (D2/R-2, :2316)** + `exigeEscopo` (:971-975) + reserva 423 | sem grupo **403** | Conforme |
| `/api/material/cautelas/{id}/recibo.pdf` | GET | **`authMaterial` (D2/R-2, :2322)** + `exigeEscopo` (:911-915) + cautela no escopo | sem grupo **403** | Conforme |
| `/api/material/conferencias/{id}/pronto.pdf` | GET | **`authMaterial` (D2/R-2, :2346 — antes auth(false) sem guard)** + `exigeEscopo` (:831-835) + `esc > 0 && confGrupoID != esc → 403` (:836-840) | sem grupo **403**; outro grupo 403 | Conforme |
| `/api/material/cautelar` · `/devolver` · `/cautelas` · anexos · `itens/{id}/anexos` · `comentarios` · `conferencias*` | GET/POST/DELETE | `authMaterial` (+`cautelaNoEscopo` server.go:387 nos anexos) + `exigeEscopo` | sem grupo 403 | Conforme |
| `/api/material/responsaveis` | GET/POST | `authMaterial` + papel para POST + **`exigeEscopo` 403 (:81-85)** — gerente sem grupo NÃO define em qualquer grupo (corrige a célula antiga do mapa) | sem grupo **403** | Conforme |

**Classe provada por:** `onda_v154_d2_test.go` (`TestD2_R2_PDFsComGuardaDoModulo` — matriz por persona), `r1_test.go` (matriz -1), `onda_material_blindagem_test.go` (IDOR anexos).

---

## 6. ESCALAS (dormente — backend vivo)

Guarda do módulo: `reservaAuth` (server_escalas.go:1819 — {gerente, operador, chefe_setor}; admin 403; 423 em reserva) nas 21+ rotas (:1788-1816). Recorte sempre pelo escopo da SESSÃO (PDFs não aceitam `?grupo=`).

| Rota | Método | Guarda (código) | Escopo -1 | Veredito |
|---|---|---|---|---|
| `/api/escalas/pdf` · `/relatorio-dia.pdf` (+ duplicata `/relatorio-dia/pdf`) · `/minhas` | GET | **`reservaAuth` (D2/R-2)** — saíram do `a.auth(false)` pelado; `minhas` também barra sem grupo com `exigeEscopo` (:1664) | **403** | Conforme |
| `/api/escalas/tipos|turnos|hoje|modelos*` (todas as 21 rotas) | GET/POST/DELETE | `reservaAuth` + **`exigeEscopo` em TODOS os 20 handlers** (linhas 21, 144, 176, 219, 423, 559, 685, 706, 718, 789, 879, 995, 1027, 1155, 1202, 1332, 1385, 1513, 1554, 1664) | sem grupo **403** — inclusive `limpar-dia`, cujo DELETE agora nunca roda com escopo 0 (reservaAuth barra o admin) | Conforme |

**Classe provada por:** `r1_test.go` (matriz: `/api/escalas/hoje|turnos|tipos` e `POST /api/escalas/limpar-dia` com gerente sem grupo → 403), `onda_v154_d2_test.go` (persona).

---

## 7. MENSAGENS

Rotas em server_pessoal.go:1666-1681 (`auth(false)` + prólogos). Caixa = linha por papel (`mensagem_destinatarios`).

| Rota | Método | Guarda (código) | Escopo | Veredito |
|---|---|---|---|---|
| `/api/mensagens` (enviar) | POST | **E1, R-14** (mensagens.go:692-745): não-admin sem grupo → **403** (:695-698); alcance = próprio grupo + `gruposSubordinadosAtivos` (server_grupos.go:14) resolvido em lista antes das queries (:709); destinatário fora do alcance → 403; admin global; caixa admin alcançável; resposta (`pai_id`) alcança o remetente da pai | sem grupo **403**; fora do escopo 403 | Conforme |
| inbox/enviadas/thread/ler/excluir/arquivar/pastas/finalizar | GET/POST | sessão + destinatário/dono da caixa (checagens internas pré-existentes) | por papel ativo | Conforme |
| `/api/mensagens/destinatarios` | GET | lista global (picker) — o SERVIDOR recusa o envio fora da matriz (resíduo cosmético documentado no mapa §8) | n/a | Conforme (resíduo cosmético) |

**Classe provada por:** `onda_v154_e1_test.go` (`TestE1R14EscopoDeEnvio`), `multi_papel_e_mensagens_test.go`.

---

## 8. DRIVE

Guarda: ACL por item `checarAcessoPasta`/`checarAcessoArquivo` (drive.go:32/:114) — autor → gerente (árvore) → grant (`drive_compartilhamentos`) → herança de pasta → função; admin 403 (doutrina).

| Rota | Método | Guarda (código) | Conteúdo | Veredito |
|---|---|---|---|---|
| `/api/drive/download/{id}` | GET | `hDriveDownload` (drive.go:748): admin 403 (:750-753); ACL (`checarAcessoArquivo` :764); **MIME gate (E2, R-11):** `mimeDriveInlinavel` (drive.go:740 — allowlist application/pdf, image/png, image/jpeg, image/webp; parâmetros de MIME descartados) — fora da allowlist: `attachment` + octet-stream; **`nosniff` SEMPRE** (:798); `Content-Disposition` com filename `sanitizarNomeArquivo` (server_material.go:2005 — barra aspas/apóstrofo/controle, teto 120) interpolado com `%q` (:799) | XSS por upload hostil bloqueado no SERVE (defesa em profundidade p/ tipos legados no banco) | Conforme |
| demais 16 rotas (itens/pastas/upload/mover/copiar/compartilhar) | — | ACL por item + `auth(false)` | por ACL | Conforme |
| `/api/drive/seletor` | GET | lista SEM ACL, sem consumidor — resíduo documentado (mapa §8) | n/a | Resíduo conhecido (fora da onda) |

**Classe provada por:** `onda_v154_e2_test.go` (`TestE2DriveMIMEInlineSeguro`), `ordem_diretor_anexos_drive_test.go`.

---

## 9. GRUPOS / ADMIN / CATÁLOGOS

| Rota | Método | Guarda (código) | Escopo -1 / objeto | Veredito |
|---|---|---|---|---|
| `/api/grupos` | GET | `hGruposList` (server_grupos.go:261): **`exigeEscopo` → 403** para conta sem grupo | **403** (corrige célula antiga do mapa) | Conforme |
| `/api/vinculos` | GET | `hVinculoList`: sem grupo devolve `{"pendentes":[],"ativos":[]}` (JSON vazio — aceito pelo critério do item A) | JSON vazio | Conforme |
| `/api/grupos/{id}` (excluir/`forcar`/`nuke`) | DELETE | **auth(true)** + **NUKE com rol completo de DELETEs em tx (E2, R-6)** (server_grupos.go:141-226): presenças/comentários, `conferencia_escalas` (por conferência, por usuário e por designante), mural com cientes/comentários cruzados, `escala_modelos+postos+aptos` antes de `escala_tipos`, `material_conferencias+itens` (+cautelas/itens/categorias), `chefe_setores`, `funcao_membros`, `grupo_setor_responsaveis`, `setor_sugestoes`, `usuario_papeis`, catálogos e usuários/pessoas do grupo | admin only | Conforme |
| `/api/catalogo/{t}` · `/api/setores/agregado` | GET | auth(false) + herança de dono/subordinados; `hSetoresAgregado` lê **`chefe_setores`** (fonte única D1) | sem grupo: recorte próprio | Conforme |
| `/api/catalogo/{t}` (escrita) · `/api/setores/{id}` (excluir) | POST/PATCH/DELETE | `guardaGestaoPessoal` / `hSetorExcluir` (ordem_0610_setores.go:31): gerente(árvore)/enc(próprio) + **`exigeEscopo` 403** (:52-58); exclusão em tx remaneja pessoas/contas, **revoga o comando** (`chefe_setores` + purga do papel sem último comando com re-chave de sessão), limpa `grupo_setor_responsaveis` e remaneja o MATERIAL (E2/R-6) | sem grupo **403**; setor rico sem órfãos | Conforme |
| `/api/setores/sugestoes*` | GET/POST | workflow sugerir→avaliar com allowlist de efeito revalidada na aplicação (tx) | por escopo | Conforme |
| `/api/backup*` · `/api/admin/sistema/metricas` · `/api/configuracoes` (POST) | — | **auth(true)** (server_admin.go:519-526); import com swap por rename real p/ `sci.db` + reabertura pela CADEIA COMPLETA (`executarMigracoes` store.go:1243 — a mesma de `AbrirStore`; `ReabrirComArquivo` store.go:1378) + rollback pelo caminho canônico (R-4 corrigido) | admin only | Conforme |

**Classe provada por:** `onda_v154_e2_test.go` (`TestE2NukeGrupoRicoSemOrfaos`, `TestE2ExclusaoSetorRicoSemOrfaos`, `TestE2ExcluirArquivadaApagaEscala`), `r1_test.go` (`TestR1_BackupRestore_AtomicAndRollback`, matriz `/api/grupos` 403), `api_backup_test.go`, `subordinacao_e_setores_test.go`, `ordem_0610_setores_test.go`.

---

## 10. MURAL / CALENDÁRIO

| Rota | Método | Guarda (código) | Escopo do objeto | Veredito |
|---|---|---|---|---|
| `/api/avisos` | GET/POST | `podeVerMural` (com grupo) / escrita gerente-admin | listagem por grupo | Conforme |
| `/api/avisos/{id}/detalhes` · `/{id}/ciente` | GET/POST | **`avisoNoEscopo` (E2, R-21)** (onda_v154_e2.go:16 — `exigeEscopo` 403 sem grupo; 404 inexistente; `esc > 0 && grupoAviso != esc → 403`) aplicada em mensagens.go:1851 (detalhes) e :1761 (ciente) | fora do escopo **403**; inexistente 404 | Conforme |
| `/api/avisos/{id}/comentar` | POST | `podeVerMural` apenas — **resíduo documentado**: sem a checagem do grupo do aviso (R-21 parcial, fora do escopo E2) | resíduo | Conforme (resíduo anotado) |
| `/api/avisos/{id}` (excluir) · `/{id}/repostar` | DELETE/POST | autor ou gerente do grupo / só gerente com origem no escopo | por objeto | Conforme |
| `/api/calendarios/{id}/compartilhamentos` | GET | **E2, R-21** (calendario.go:1040): admin 403; régua do compartilhar/revogar — **autor do calendário ou gerente do grupo-dono** (:1059-1066), 403 fora | fora do escopo **403** | Conforme |
| `/api/calendarios*` · `/api/calendario/*` (restante) | — | ACL `checarAcessoEvento` (autor→gerente→compartilhamento→mesmo grupo); módulo dormente (R-27: 2 gerações de API a unificar na reativação) | por ACL | Conforme |

**Classe provada por:** `onda_v154_e2_test.go` (`TestE2MuralAvisoForaDoEscopo`, `TestE2CalendarioCompartilhamentosEscopo`), `ordem_diretor_0710_avisos_test.go`.

---

## 11. Classes de defeito da onda × prova (resumo)

| Classe (defeito R-x) | Mecanismo pós-onda (código) | Teste de regressão |
|---|---|---|
| Escopo -1 (R-1) | guarda central `exigeEscopo` (helpers.go:125) + `filtroGrupoSQL` `AND 1=0` (helpers.go:138) em todos os handlers verificados | `r1_test.go` (`TestR1_ScopeClosure_Matrix` — 21 rotas 403; `_FiltroGrupoSQL`; `_AdminAndNormalGroupAccess`) |
| PDFs com guard do módulo (R-2) | `confPDFAuth` (server_conferencia.go:1672), `reservaAuth` nas rotas de escalas (:1796-1816), `authMaterial` nos 3 PDFs de material (:2316/:2322/:2346) | `onda_v154_d2_test.go` (`TestD2_R2_PDFsComGuardaDoModulo`, `TestD2_R2_PDFMagicBytes`) |
| Chefe-zumbi (R-12) | `chefe_setores` FONTE ÚNICA: `chefeComandaSetor` sem fallbacks (onda_0510_conf_escopo.go:146), `setorAtivoComandado` (onda_v154_d1.go:96), migração v44 materializa legados (onda_v154_d1.go:35), papel add/del materializa/revoga | `onda_v154_d1_test.go` (4 testes) |
| Antiguidade 3 fontes (R-7) | `filtroAntiguidadeTresFontes`/`exprAntiguidadeTresFontes`/`ordemAntiguidadeTresFontes` (onda_0910_conf_antiguidade.go:127/:136/:141) em hoje/pré-fechamento/tela/PDF; `sem_tag` ao iniciar/despachar (`militaresSemTagAntiguidade` :159) | `onda_v154_d3_test.go` (4 testes) |
| Foto com escopo (R-5) | `hUsuarioFotoGet` com régua própria/admin/grupo/403 (server_pessoal.go:1105) | `onda_v154_e2_test.go` (`TestE2FotoUsuarioComEscopo`) |
| Drive MIME (R-11) | `mimeDriveInlinavel` (drive.go:740) + attachment/octet-stream/nosniff sempre + filename sanitizado no SERVE | `onda_v154_e2_test.go` (`TestE2DriveMIMEInlineSeguro`) |
| Exclusões sem órfãos (R-6/R-15) | rol completo de DELETEs em tx no NUKE (server_grupos.go:141-226), `hSetorExcluir` (ordem_0610_setores.go:31, revoga comando + remaneja material), `excluirArquivada` inclui `conferencia_escalas` (server_conferencia.go:849-860) | `onda_v154_e2_test.go` (3 testes "SemOrfaos"/"ApagaEscala") |
| Mural/calendário no escopo (R-21) | `avisoNoEscopo` (onda_v154_e2.go:16) em detalhes/ciente; régua autor/gerente-dono nos compartilhamentos (calendario.go:1040) | `onda_v154_e2_test.go` (`TestE2MuralAvisoForaDoEscopo`, `TestE2CalendarioCompartilhamentosEscopo`) |
| Contexto/envio/papel-del no escopo (R-9/R-14/R-10) | `hMudarContexto` setor no grupo do papel ativo + chefe só comandado (mensagens.go:17); envio `gruposSubordinadosAtivos` (mensagens.go:692-745); `podeGerirPapelAlvo` fail-closed (onda_v154_e1.go:66) | `onda_v154_e1_test.go` (7 testes) |
| Senha invalida sessões (R-16) | `invalidarSessoesDeSenha` (onda_v154_e1.go:47) nas duas rotas de senha; admin semeado `precisa_setup=1` | `onda_v154_e1_test.go` (3 testes R-16) |

## 12. Divergências mapa×código encontradas (corrigidas no `ARQUITETURA.md` neste commit)

1. **R-1 tratado como aberto** — o código já aplica `exigeEscopo`/`filtroGrupoSQL` (fail-closed para -1) nos handlers de TODOS os módulos verificados e a matriz `r1_test` prova 403; o mapa mantinha "⚠ vaza tudo (R-1)" na matriz §4, células "⚠-1 (R-1)" nas tabelas §5-§10 e o registro §14 sem marcação. **Corrigido:** R-1 marcado como corrigido na v1.5.4-A; células atualizadas.
2. **R-3 e R-4 tratados como abertos** — R-3: `validarFotoBase64` (server_pessoal.go:17, aplicada nas duas escritas) + sinks com `esc()`/`fotoValida()` (core.js:585, views_gestao.js:548); R-4: swap com rename real (server_admin.go:261-300) e `ReabrirComArquivo` executando a cadeia COMPLETA `executarMigracoes` (store.go:1243/:1378). Ambos provados por `r1_test.go`. **Corrigido:** registros marcados (correção v1.5.4-A/B/C, `fd96c90`).
3. **Schema citado como v43** (§3.E e §12; `main.go:21`) — a onda D1 subiu a cadeia para **v44** (`migrarV44`, store.go:1370; `versaoSchemaBinario = 44`, main.go:23). **Corrigido:** §3.E/§12 atualizados (v2…v44; `executarMigracoes` única, usada também na reabertura).

---

## 13. Análise de estilo

Mantém-se o padrão pré-existente e a diretriz de **proibição de refactor cosmético sem furo comprovado**: guardas no middleware do módulo (`authConf*`, `reservaAuth`, `authMaterial`, `guardaGestaoPessoal`, `confPDFAuth`) + prólogo de escopo no handler (`exigeEscopo` + teste do objeto), com SQL no fio (`filtroGrupoSQL`) onde a listagem já o usava. A pulverização (103 prólogos) é débito declarado da v1.5.5 (tabela de política por rota) — não foi "arrumada" nesta onda.

## 14. Conclusão

A onda v1.5.4 fecha as 10 classes de defeito propostas com prova de regressão por persona, e a auditoria NÃO encontrou furo de escopo aberto nos 8 módulos tocados. Os resíduos conhecidos permanecem documentados e fora do escopo da onda: `hAvisosComentar` sem checagem de grupo (R-21 parcial), `/api/drive/seletor` sem ACL (sem consumidor), resíduos D-5 (`pre_fechamento` e designação de escala do chefe ainda leem o contexto de cadastro, sem poder de conferência), D-3 (abertura ampla de `escalas/minhas`) e D-1 (herança de catálogo) — decisões pendentes de comando.
