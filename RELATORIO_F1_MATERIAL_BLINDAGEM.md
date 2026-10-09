# Relatório de Execução — Frente 1: Blindagem de Segurança e Correção de Bugs de Comportamento no Módulo Material

**Data:** 08/10/2026  
**Branch:** `f2/material-blindagem`  
**Base:** `main` (`d51ac17`)  
**Status do Portão de Entrega:** APROVADO (go build exit 0; go test 100% verde)

---

## 1. Status Tarefa a Tarefa

| Tarefa | Descrição | Status | Detalhes / Divergências |
|---|---|---|---|
| **Tarefa 1** | Anexos de Item (escopo, allowlist MIME, sanitização e teto 800KB) | Feito | Handlers `hMaterialItemAnexosList`, `hMaterialItemAnexoAdd`, `hMaterialItemAnexoGet` e `hMaterialItemAnexoDel` agora conferem escopo via item (`SELECT COALESCE(grupo_id,0)`). `hMaterialItemAnexoAdd` valida allowlist (`application/pdf`, `image/png`, `image/jpeg`, `image/webp` -> 400), aplica `sanitizarNomeArquivo` e teto de 800 KB b64 (413). |
| **Tarefa 2** | Comentários de Item (escopo) | Feito | Handlers `hMaterialItemComentariosList` e `hMaterialItemComentarioAdd` conferem escopo do item (404 se inexistente, 403 se grupo != escopo do usuário quando escopo > 0). `Add` preservado append-only com `u.ID`. |
| **Tarefa 3** | Conferência de Material (escopo em mutantes e detalhe) | Feito | Handlers `hMaterialConferenciaGet`, `hMaterialConferenciaBipar`, `hMaterialConferenciaFechar` e `hMaterialConferenciaIniciar` validam escopo da conferência / grupo informado (403 quando escopo > 0 e divergente). |
| **Tarefa 4** | Porta de Papel e Escopo em Responsáveis (`hMaterialResponsaveisSave`) | Feito | Permitido apenas para `admin`, `gerente` ou designado `ehEncarregadoDeMaterial` (demais -> 403). `enc_material` restrito ao seu próprio grupo (`req.GrupoID = *u.GrupoID`). Gerente e encarregado bloqueados se tentarem gravar em grupo alheio (403). Tratamento de FK com ponteiro nulo para evitar violações em setor/auxiliar 0. |
| **Tarefa 5** | Categorias (escopo no UPDATE e DELETE) | Feito | `hMaterialCategoriasAdd` (edição `ID > 0`) e `hMaterialCategoriasDel` exigem que categorias globais (`grupo_id IS NULL`) só sejam editadas/excluídas por `admin` (403 para gerentes/outros); categorias de grupo só pelo próprio grupo (403 para outros grupos). |
| **Tarefa 6a** | Data da Conferência (formato YYYY-MM-DD) | Feito | Corrigido em `hMaterialConferenciaIniciar` de `Format("YYYY-MM-DD")` literal para `Format("2006-01-02")`. |
| **Tarefa 6b** | UNIQUE real para Conferência (Migração v40) | Feito | Criada migração v40 com índice parcial único `CREATE UNIQUE INDEX IF NOT EXISTS idx_mat_conf_unica ON material_conferencias (grupo_id, COALESCE(setor_id,0), data) WHERE status = 'aberta'`. Handler retorna 409 em caso de conflito. Schema registrado nos 3 pontos padrão (`main.go`, `store.go`, `migrarV40`). |
| **Tarefa 6c** | Recibo de Cautela PDF lê sensibilidade correta | Feito | Em `hMaterialCautelaReciboPDF`, atualizada query para `COALESCE(NULLIF(mi.sensibilidade,''), 'convencional')`. |
| **Tarefa 6d** | Cautelas List LEFT JOIN (doutrina zero-perda) | Feito | Em `hMaterialCautelasList`, alterados `JOIN` internos de pessoas e usuários para `LEFT JOIN` e `COALESCE` nos scans com suporte a campos nulos. |
| **Tarefa 7** | Alinhar Gate `chefe_setor` no middleware `authMaterial` | Feito | Removido `chefe_setor` de `authMaterial`. O acesso é exclusivo a `gerente`, `operador` e `ehEncarregadoDeMaterial`. Testes legados ajustados de acordo. |
| **Tarefa 8** | Nova Suíte de Testes (`onda_material_blindagem_test.go`) | Feito | Criados testes completos com matriz de personas (`admin`, `gerente A`, `gerente B`, `enc_material A`, `operador A`, `chefe_setor A`) cobrindo escopo de anexos, allowlist MIME, sanitização, comentários, conferência (escopo, unicidade 409, formato de data), responsáveis por papel, categorias globais vs grupo e bloqueio de `chefe_setor`. |
| **Tarefa 9** | Relatório de Execução | Feito | Este documento. |

---

## 2. Resultado da Suíte de Testes

### Execução da Suíte Completa
- **Comando:** `go test ./... -vet=off -timeout 900s -count=1`
- **Exit Code:** `0`
- **Tempo Total:** ~103.0 segundos (~1m43s)
- **Status:** PASS (todos os pacotes e testes passaram)

### Build do Binário
- **Comando:** `go build ./...`
- **Exit Code:** `0`

---

## 3. Riscos Residuais e Observações

1. **Frontend / Cache-Busting:** Nenhum arquivo frontend (`web/`) precisou ser modificado nesta frente (as APIs mantiveram contratos JSON compatíveis), não sendo necessário bump de cache-bust.
2. **Índice Parcial SQLite:** O índice parcial com expressão `(grupo_id, COALESCE(setor_id,0), data) WHERE status = 'aberta'` é totalmente suportado pelo SQLite embutido e testado com sucesso para garantir unicidade de conferência aberta no mesmo dia/setor.
3. **Escopo de Administrador em Material:** O módulo de material permanece restrito a usuários com atribuição operacional no grupo (`gerente`, `operador`, `enc_material`), enquanto administradores puros sem papel de grupo recebem 403 nas rotas operacionais conforme arquitetura do SCI.
