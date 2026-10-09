# Relatório de Execução — Frente 2: Evolução do Módulo de Material

**Projeto:** SCI — Sistema de Controle Interno  
**Módulo:** Material (v1.5)  
**Branch:** `f2/material-features` (base: `main` @ `d51ac17`)  
**Data:** 08/10/2026  

---

## 1. Sumário Executivo

A Frente 2 do Módulo de Material teve como objetivo concluir a implementação no backend de todas as funcionalidades e contratos que o frontend (`web/views_material.js`) já demandava e prometia, além de deliberar e sanar a dívida técnica relacionada à taxonomia legada.

Em consonância estrita com a regra de coordenação com a **Frente 1 Paralela (Blindagem/Segurança)**:
- Nenhuma permissão, middleware de escopo (`authMaterial`, `escopoDoUsuario`) ou allowlist de handlers foi modificada.
- Todas as validações e travas de segurança existentes foram integralmente preservadas.
- O contrato esperado pelo frontend vanilla JS foi 100% honrado sem necessidade de alterações ou bumps no frontend.

---

## 2. Detalhamento Tarefa a Tarefa

### Tarefa 1 — Persistir a Extensão de Viatura (Backend)
- **Escrita (`hMaterialItensSave`):**
  - O struct de request de itens foi expandido para receber `Placa`, `Renavam`, `PadrinhoTitularID`, `PadrinhoSubstitutoID`, `HodometroAtual` e `CombustivelAtual`.
  - Implementado gatilho que executa `UPSERT` atômico na tabela `material_viaturas` (criada na v36) sempre que qualquer dado específico de viatura for informado não-vazio/não-zero.
  - Normalização do domínio de combustível conforme o select `#mItemCombustivel` do front (`cheio`, `3/4`, `1/2`, `1/4`, com fallback seguro para `cheio`).
  - IDs de padrinhos nulos ou `<= 0` são gravados como `NULL` respeitando a integridade referencial com a tabela `pessoas`.
- **Leitura (`hMaterialItensList`):**
  - Adicionados `LEFT JOIN` com `material_viaturas` e com `pessoas` (`pt` titular e `ps` substituto).
  - A resposta JSON inclui o objeto `"viatura": { placa, renavam, padrinho_titular_id, padrinho_titular_nome, padrinho_substituto_id, padrinho_substituto_nome, hodometro_atual, combustivel_atual }` **exclusivamente** para itens que possuam registro de viatura. Itens comuns não contêm a chave `viatura`.
- **Filtro de Garagem (`?garagem=1`):**
  - A listagem filtra itens pertencentes a categorias com nome semelhante a viaturas (`mi.categoria_id IN (SELECT id FROM material_categorias WHERE LOWER(nome) LIKE '%viatur%')`) ou que possuam registro em `material_viaturas`.

### Tarefa 2 — Persistir `setor_id` do Item
- **Escrita (`hMaterialItensSave`):**
  - Adicionado campo `SetorID *int64` ao payload.
  - Validação estrita de Foreign Key: caso um `setor_id > 0` seja informado e não exista na tabela `setores`, a requisição é rejeitada com status **HTTP 400 ("setor inválido")** em vez de gerar erro 500 no banco.
  - Gravado `setor_id` no `INSERT` e `UPDATE` (`NULL` para itens de Carga Geral da Unidade).
- **Leitura (`hMaterialItensList`):**
  - Realizado `LEFT JOIN setores s ON s.id = mi.setor_id`, expondo `setor_id` (inteiro ou `nil`) e `setor_nome` (string ou vazio, permitindo o fallback do front para `'Carga Geral'`).

### Tarefa 3 — Conferência Nasce com Checklist do Inventário
- **Inicialização (`hMaterialConferenciaIniciar`):**
  - A inicialização da conferência diária agora é executada dentro de uma **transação SQLite atômica**.
  - Após inserir a conferência em `material_conferencias`, a tabela `material_conferencia_itens` é automaticamente populada com todo o inventário do escopo:
    - Itens pertencentes ao `grupo_id` da conferência;
    - Se a conferência for de um setor específico (`setor_id > 0`), são incluídos apenas os itens com `mi.setor_id = ?`;
    - Se a conferência for de Carga Geral (`setor_id <= 0`), são incluídos apenas os itens sem setor (`mi.setor_id IS NULL`);
    - Itens com `status = 'baixado'` são explicitamente excluídos do checklist ativo.
  - **Decisão sobre o CHECK de status:** Como o `CHECK` da tabela `material_conferencia_itens` aceita apenas `('presente', 'acautelado', 'ausente', 'manutencao', 'baixado')` (não havendo `'nao_verificado'`), foi mantido o status inicial `'presente'` com `quantidade_conferida = 0`. Os endpoints de consulta (`GET`) e o gerador de PDF tratam `quantidade_conferida = 0` como item pendente / "ainda não conferido" (exibido como `"—"`).
  - Proteção contra duplicidade: tentativa de abrir uma segunda conferência com status `'aberta'` para o mesmo grupo/setor/data é rejeitada com **HTTP 409 Conflict**.
- **Consulta (`hMaterialConferenciaGet`):**
  - Retorna a lista completa de itens esperados vs conferidos, tratando campos opcionais (`conferido_por`, `observacao`) com suporte correto a valores nulos.

### Tarefa 4 — PDF do Pronto de Material Oficial (Substituição do Stub)
- **Implementação do Gerador (`gerarProntoMaterialPDF` em `relatorio.go`):**
  - Substituído o stub temporário de texto simples (`gerarPDFMinimo`) por um gerador A4 oficial utilizando a biblioteca `github.com/go-pdf/fpdf`.
  - Estrutura completa:
    1. Cabeçalho institucional padronizado (Unidade, Setor, Data, Operador emissor);
    2. Bloco 1: Cartões quantitativos de resumo (Total de Itens, Presentes, Acautelados, Manutenção, Não Conferidos);
    3. Bloco 2: Tabela discriminada dos itens (Patrimônio, Descrição, Qtd Esperada, Qtd Conferida com `"—"` para não conferidos, Situação, Conferente, Observação);
    4. Rodapé institucional com linhas de assinatura para o **Encarregado de Material** e **Gerente / Chefe de Setor**.
  - A função legada `gerarPDFMinimo` foi removida do código por não possuir outros chamadores.

### Tarefa 5 — Dívida Técnica: Taxonomia Tipo/Classe
- **Decisão e Execução:**
  - As tabelas `material_tipos` e `material_classes` (criadas na v27) e as respectivas colunas em `material_itens` permanecem no banco de dados para garantir integridade e compatibilidade de replays/backups.
  - Documentado explicitamente em `store.go` através de comentário explicativo que tais tabelas estão dormentes, sem endpoints ativos de gestão, aguardando definição de produto.
  - Confirmado que o endpoint de listagem de itens `hMaterialItensList` não expõe campos mortos.

### Tarefa 6 — Suíte de Testes
- Criado o arquivo `onda_material_features_test.go` cobrindo detalhadamente:
  - Criação de item com extensão de viatura e persistência em `material_viaturas`;
  - Edição idempotente de viatura (upsert sem duplicação de linhas);
  - Criação de item comum e garantia de ausência de linha em `material_viaturas`;
  - Listagem com objeto `viatura` e nomes de padrinhos;
  - Filtro `?garagem=1`;
  - Associação de `setor_id` e resposta de `setor_nome`;
  - Rejeição HTTP 400 para `setor_id` inexistente;
  - Criação automática de checklist em `material_conferencia_itens` ao abrir conferência;
  - Isolamento de itens por setor e por carga geral;
  - Bloqueio HTTP 409 para conferências abertas duplicadas;
  - Geração do Pronto Diário em PDF (HTTP 200, Content-Type `application/pdf`, cabeçalho `%PDF`, tamanho > 1 KB).

---

## 3. Resultado da Suíte de Testes

- **Comando executado:** `go test ./... -vet=off -timeout 900s -count=1`
- **Compilação:** `go build ./...` — **Exit Code 0**
- **Testes Automatizados:** **100% Aprovados (PASS)**
- **Tempo de Execução:** ~85-95 segundos

---

## 4. Esquema de Banco de Dados e Migrações

- **Versão Atual do Schema:** `v39`
- **Novas Migrações:** Nenhuma nova migração de DDL (`v40` ou `v41`) foi necessária nesta frente, uma vez que as tabelas (`material_viaturas`, `material_conferencias`, `material_conferencia_itens`) e a coluna `material_itens.setor_id` já haviam sido provisionadas na migração `v36`.
- **Compatibilidade com Frente 1:** Caso a Frente 1 paralela crie a migração `v40`, a integração com a Frente 2 ocorrerá de forma limpa e sem conflitos de versão de schema.

---

## 5. Riscos Residuais e Recomendações

1. **Classificação de Categorias para a Garagem:** O filtro `?garagem=1` busca categorias cujo nome contenha o termo `"viatur"` (como a categoria padrão `"Viaturas & Chaves"` / `"Viaturas & Transportes"`) ou itens que tenham registro em `material_viaturas`. Recomenda-se manter a convenção de nomenclatura de categorias na administração do sistema.
2. **Homologação em Produção:** As alterações respeitam integralmente os componentes visuais já existentes no frontend (`web/views_material.js`), assegurando continuidade de uso sem necessidade de flush de cache forçado no cliente.
