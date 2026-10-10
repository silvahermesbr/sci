# Original User Request

## 2026-10-10T15:09:40Z

Executar a onda completa v1.5.4 (estancar P0s, bugs de campo e correções de segurança P1) no sistema SCI em Go, em conformidade estrita com `docs/INSTRUCOES_AGENTES.md`, `docs/ARQUITETURA.md` e `docs/ROADMAP.md`.

Working directory: c:\Users\hermes\Documents\programming\sci
Integrity mode: development

## Requirements

### R1. Estancar os 3 P0s de Segurança e Integridade
- **Item A (R-1):** Fechar a classe de escopo `-1` (contas sem grupo) em todas as rotas de dados através de guarda centralizada e eliminação de bifurcações frouxas `esc > 0`.
- **Item B (R-2):** Blindar `foto_base64` contra XSS armazenado validando payload MIME/base64 e teto no backend, com escape rigoroso no frontend.
- **Item C (R-3 / R-4):** Corrigir importação de backup (`server_admin.go` e `store.go`) garantindo swap com rename atômico para `sci.db`, limpeza correta de WAL e reabertura pela cadeia completa de migrações com rollback seguro.

### R2. Resolver os 3 Bugs de Campo Relatados pelo Comando
- **Item D1 (Chefe-zumbi):** Corrigir `chefeComandaSetor` e invalidação de `usuarios.setor_id` ao destituir chefia de setor.
- **Item D2 (Acesso via URL/API):** Proteger rotas de exportação/PDF e endpoints de dados garantindo que nenhuma rota opere sem checagem de escopo/papel compatível com o módulo.
- **Item D3 (Antiguidade e tags):** Unificar ordenação da conferência usando a mesma expressão `COALESCE` para todas as fontes de tags, sinalizando militares fora do filtro na resposta.

### R3. Resolver Guardas Remanescentes P1
- Aplicar correções de R-5 (foto escopo), R-6/R-15 (cascade de exclusão e NUKE), R-8 (arquivar/descartar conferência), R-9 (`hMudarContexto`), R-10 (`hUsuarioPapelDel/Add`), R-11 (Drive MIME/inline), R-14 (mensageria por papéis) e R-16 (invalidação de sessões na troca de senha).

### R4. Fortalecimento de CI
- Atualizar `ci.sh` para executar os testes e verificações estáticas obrigatórias do projeto, e remover bancos/artefatos espúrios da árvore.

### R5. Documentação e Governança
- Atualizar a tabela de rotas e colunas de escopo/guarda no `docs/ARQUITETURA.md` para cada endpoint tocado.
- Atualizar o status e caixas de seleção em `docs/ROADMAP.md` e registrar entradas correspondentes no `CHANGELOG.md`.

## Acceptance Criteria

### Testes e Verificação
- [ ] Testes de regressão novos cobrindo casos positivos e negativos para cada item corrigido (A, B, C, D1, D2, D3 e E).
- [ ] Teste de matriz de escopo comprovando que conta com `esc == -1` recebe 403 / JSON vazio em rotas de dados.
- [ ] `go vet ./...` executa sem apontamentos.
- [ ] `go build` compila sem erros.
- [ ] `go test -count=1 ./...` passa 100% verde em toda a suíte.

### Integridade do Sistema
- [ ] Nenhuma rota permite bypass de escopo por conta sem grupo ou operadores fora de sua esfera.
- [ ] Import de backup restaura fielmente o estado do banco e reabre com migrações íntegras.
- [ ] Destituição de chefia revoga imediatamente permissões de comando de setor.
- [ ] Zero regressões nos testes existentes do repositório.
