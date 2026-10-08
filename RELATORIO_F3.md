# Relatório Técnico de Implementação — Frente 3 (Novo Modelo de Encarregados)

## 1. Visão Geral e Contexto Executivo

Este documento consolida a implementação do **Novo Modelo de Encarregados** no Sistema de Controle Interno (SCI), conforme diretrizes estritas do dono do sistema.

### Princípios do Modelo Implementado:
1. **Funções por Designação**: Gerente, Encarregado de Pessoal e Encarregado de Material são funções vinculadas ao grupo da sessão através de designações (1 titular e N auxiliares por grupo). Não são itens soltos de catálogo/tags.
2. **Igualdade Registral vs Poder Operacional**: O titular da cadeira detém a indicação registral (`👑`), enquanto auxiliares detêm **exatamente os mesmos poderes operacionais** da função.
3. **Escopo de Módulos**:
   - **Gerente**: Acesso irrestrito a todos os módulos do grupo (Avisos, Conferência, Email, Drive, Pessoal, Material, Gerenciar Grupo, Relatórios). É a autoridade exclusiva para designar titulares e auxiliares.
   - **Encarregado de Pessoal (Titular & Auxiliar)**: Acesso total à Conferência e ao módulo Pessoal (Efetivo, Funções em modo leitura, Setores). **Não tem acesso** ao módulo Gerenciar Grupo (`#/grupos`) nem ao módulo Material (`#/material`). Não pode designar nem remover funções.
   - **Encarregado de Material (Titular & Auxiliar)**: Módulo Material desbloqueado e ativo (Balcão, Inventário, Garagem, Histórico). Acesso mantido à Conferência do grupo. **Não tem acesso** a Pessoal nem a Gerenciar Grupo.
4. **Anti-Escalação de Privilégios (Regra de Ferro)**:
   - O poder decorre exclusivamente da chave imutável (`enc_pessoal`, `enc_material`), gravada na tabela `funcoes` com índice único parcial (`idx_funcoes_chave`).
   - A API impede alteração da coluna `chave` via requisições de catálogo (`PATCH`).
   - Renomear itens arbitrários do catálogo (ex.: "Soldado EV" renomeado para "Encarregado de Pessoal") confere **zero poderes**.

---

## 2. Matriz de Permissões e Acessos

Legenda das células:
- **200**: Acesso liberado no backend e módulo visível na interface.
- **403**: Acesso negado com HTTP 403 Forbidden pelo backend.
- **Oculto**: Módulo/rota invisível na navegação ou redirecionado para rota padrão/sem-módulo.

| Papel / Função | Conferência (`#/hoje`) | Pessoal (`#/pessoal`) | Material (`#/material`) | Gerenciar Grupo (`#/grupos`) | Relatórios (`#/relatorios`) | Mural (`#/avisos`) | Designar Funções (`POST /api/grupo/funcoes/membros`) |
| :--- | :---: | :---: | :---: | :---: | :---: | :---: | :---: |
| **Admin** (global sem grupo) | Oculto / 403 | 200 (Global) | 403 / Oculto | Oculto / 403 | Oculto | 200 | 200 |
| **Gerente** | 200 | 200 | 200 | 200 | 200 | 200 | 200 |
| **Encarregado de Pessoal (Titular)** | 200 | 200 | 403 / Oculto | 403 / Oculto | 200 | Oculto | 403 (Leitura apenas) |
| **Encarregado de Pessoal (Auxiliar)** | 200 | 200 | 403 / Oculto | 403 / Oculto | 200 | Oculto | 403 (Leitura apenas) |
| **Encarregado de Material (Titular)** | 200 | 403 / Oculto | 200 | 403 / Oculto | Oculto | Oculto | 403 |
| **Encarregado de Material (Auxiliar)** | 200 | 403 / Oculto | 200 | 403 / Oculto | Oculto | Oculto | 403 |
| **Chefe de Setor** | 200 | 403 / Oculto | 403 / Oculto | 403 / Oculto | Oculto | 200 | 403 |
| **Operador** | 200 | 403 / Oculto | 403 / Oculto | 403 / Oculto | Oculto | 200 | 403 |
| **Usuário Comum / Sem Função** | Oculto / 403 | Oculto / 403 | Oculto / 403 | Oculto / 403 | Oculto | Oculto | 403 |

---

## 3. Resumo dos Testes e Validação da Suíte

### 3.1. Testes Específicos do Modelo (Frente 1 & Frente 3)
Suíte `onda_f3_modelo_test.go` e `onda_f3_e2e_personas_test.go`:
- **`TestF3E2EPersonasEMatrizPermissoes`**: Validação de ponta a ponta das 6 personas (Admin, Gerente, Encarregado Pessoal Titular/Auxiliar, Encarregado Material Titular/Auxiliar, Operador/Chefe) e proteção contra bypasses (renomeação maliciosa e criação forçada de grupo no catálogo). (**PASS**)
- **`TestF3Migracao38IdempotenteEBackfill`**: Validação da migração v38 com colunas `tipo` e `chave`, índice único parcial e preenchimento idempotente. (**PASS**)
- **`TestF3AuxiliarEncPessoalTemPoderGestaoPessoal`**: Validação de paridade total de poder para auxiliares. (**PASS**)
- **`TestF3EncMaterialAcessaMaterialENaoPessoal`**: Validação de isolamento do encarregado de material. (**PASS**)
- **`TestF3EncarregadoNaoDesignaMembros`**: Validação de bloqueio 403 ao tentar designar membros sem ser gerente/admin. (**PASS**)
- **`TestF3AntiEscalacaoRenomearItemAntiguidade`**: Validação contra ataques de renomeação de itens no catálogo. (**PASS**)
- **`TestF3ApiMeTrazFuncoesGrupo`**: Validação do contrato de `/api/me` sem N+1. (**PASS**)
- **`TestF3PatchCatalogoNaoAlteraChaveEBloqueiaNaoAdmin`**: Imutabilidade da coluna chave via API. (**PASS**)
- **`TestF3SegundaLinhaMesmaChaveViolaIndiceSemPanico`**: Validação da restrição de integridade do SQLite. (**PASS**)

### 3.2. Suíte Completa de Regressão
- **Comando executado**: `go test ./... -vet=off -timeout 900s -count=1`
- **Resultado**: `ok sci 75.199s` (100% dos testes passaram sem nenhuma falha).
- **Verificação de sintaxe de Frontend**:
  - `web/core.js`: `node --check` aprovado (exit 0).
  - `web/views_gestao.js`: `node --check` aprovado (exit 0).
  - `web/views_pessoal.js`: `node --check` aprovado (exit 0).
  - `web/views_material.js`: `node --check` aprovado (exit 0).
  - `web/lazy.js`: `node --check` aprovado (exit 0).
- **Cache-Busting**: Atualizado de `v=359` para `v=360` em todas as 4 referências do `web/index.html` e no `CACHEBUST` de `web/lazy.js`.

---

## 4. Roteiro Passo a Passo para Teste no Navegador

Para homologação visual e operacional no navegador, execute os passos abaixo:

### Passo 1: Iniciar o Servidor
No terminal, execute o binário ou rode diretamente em Go:
```bash
go run .
```
Abra o navegador em `http://localhost:8080` (ou na porta configurada).

### Passo 2: Acessar como Gerente
1. Faça login com um usuário de papel `gerente` (ex.: conta de gerência do grupo).
2. Verifique a Sidebar:
   - Devem constar: **Mural de Avisos**, **Conferência**, **Email Interno**, **Drive Local**, **Pessoal**, **Material**, **Gerenciar Grupo** e **Relatórios**.
3. Acesse o módulo **Material** (`#/material`):
   - O módulo abre normalmente (Balcão Express, Inventário & Carga, Garagem & Frota).
4. Acesse o módulo **Pessoal** (`#/pessoal`) -> aba **Funções**:
   - Note o formulário no topo: campo de texto + botão `+ Nova função de grupo`.
   - Na tabela de funções, note os botões de `Designar` (select de conta + select titular/auxiliar) e os botões `remover`.
   - Crie uma designação: selecione uma conta de operador e designe como `auxiliar` de "Encarregado de Pessoal".
   - Designe outra conta como `titular` de "Encarregado de Material".

### Passo 3: Acessar como Encarregado de Pessoal (Titular ou Auxiliar)
1. Faça logout e login com a conta designada para "Encarregado de Pessoal".
2. Verifique a Sidebar:
   - Devem constar exclusivamente: **Conferência**, **Pessoal** e **Meu Perfil**.
   - Note que **Gerenciar Grupo** (`#/grupos`) e **Material** (`#/material`) **NÃO** aparecem.
3. Teste digitar diretamente na URL `#/grupos`:
   - O sistema redireciona automaticamente para a tela inicial (`#/hoje`).
4. Teste digitar diretamente na URL `#/material`:
   - O sistema exibe o aviso de módulo não disponível.
5. Acesse o módulo **Pessoal** (`#/pessoal`) -> aba **Funções**:
   - A tabela é exibida em **modo somente leitura**:
     - Indica quem é o titular com coroa `👑` e quem são os auxiliares.
     - **Não** exibe formulário de criação de função.
     - **Não** exibe botões `Designar` nem botões `remover`.
     - Exibe o banner informativo discreto: *"A designação de funções é realizada pelo gerente do grupo."*
6. Na aba **Efetivo**, confirme que o encarregado/auxiliar consegue cadastrar e editar militares normalmente com poderes plenos de gestão de pessoal.

### Passo 4: Acessar como Encarregado de Material
1. Faça logout e login com a conta designada para "Encarregado de Material".
2. Verifique a Sidebar:
   - Devem constar exclusivamente: **Conferência**, **Material** e **Meu Perfil**.
   - Note que **Pessoal** (`#/pessoal`) e **Gerenciar Grupo** (`#/grupos`) **NÃO** aparecem.
3. Acesse o módulo **Material** (`#/material`):
   - Módulo carrega normalmente com acesso total aos controles de material e carga.
4. Teste digitar diretamente na URL `#/pessoal`:
   - O sistema redireciona automaticamente para `#/hoje`.

### Passo 5: Teste de Anti-Escalação no Catálogo
1. Com uma conta de operador ou chefe de setor, tente chamar a API de catálogo renomeando uma antiguidade para "Encarregado de Pessoal".
2. Mesmo que o nome da função no banco contenha "Encarregado de Pessoal", a conta **não** recebe papel nem privilégios de encarregado, pois a coluna `chave` permanece `NULL` e a detecção é feita exclusivamente pela chave de integridade do sistema.
