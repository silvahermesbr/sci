# Relatório de Auditoria de Escopos — Módulo de Pessoal (SCI)

**Data:** 08/10/2026  
**Referência:** Ordem do Diretor 08/10 — Isolamento Rígido de Escopo e Multi-tenancy por Grupo  
**Alvo:** Rotas registradas em `rotasPessoal` ([server_pessoal.go:1531](file:///c:/Users/hermes/Documents/programming/wt_frenteA_pessoal/server_pessoal.go#L1531))

---

## 1. Sumário Executivo

Foi realizada a auditoria exaustiva de todas as rotas e manipuladores (*handlers*) vinculados à gestão de pessoal e usuários. O objetivo principal foi verificar a conformidade com a regra estrita de isolamento: **dados de pessoal nunca cruzam grupos/unidades sem autorização expressa (papel `admin` ou árvore de subordinação quando previsto)**.

**Resultado da Auditoria:** **NENHUM FURO REAL DE ESCOPO IDENTIFICADO.**  
Todos os endpoints aplicam filtragem no SQL (no fio) ou validação rigorosa prévia/pós-consulta com bloqueio `403 Forbidden` / `404 Not Found`. Nenhuma informação privada ou restrita vaza entre grupos distintos.

---

## 2. Auditoria Detalhada por Família de Rotas

### 2.1 Pessoas (CRUD, Ficha, Apresentação, Modificações, QR e PDF)

| Rota | Método | Handler | Mecanismo de Isolamento | Veredito |
|---|---|---|---|---|
| `/api/pessoas` | GET | `hPessoasList` | `a.pessoasTodas(escopoDoUsuario(u))` aplica `WHERE grupo_id = ?` quando `escopo > 0`. A auditoria de modificações é restrita às pessoas retornadas. | Conforme |
| `/api/pessoas` | POST | `hPessoasAdd` | Envelopado em `guardaGestaoPessoal`. Força `grupo_id = u.GrupoID` para gerente, encarregado e auxiliar. Valida se a função pertence ao grupo via `funcaoValidaParaPessoa`. | Conforme |
| `/api/pessoas/{id}` | PATCH | `hPessoasEdit` | Envelopado em `guardaGestaoPessoal`. Consulta `grupo_id` da pessoa: se `u.Papel != "admin"` e `gid != escopo`, retorna `403`. Valida `atribuiFuncaoPessoal` (chave `enc_pessoal`) e `funcaoValidaParaPessoa`. | Conforme |
| `/api/pessoas/{id}` | DELETE | `hPessoaExcluir` | Envelopado em `guardaGestaoPessoal`. Se `u.Papel != "admin"`, valida se a pessoa pertence ao grupo do usuário antes de desativar/excluir (`403` caso contrário). | Conforme |
| `/api/pessoas/{id}/ficha` | GET | `hPessoaFicha` | Filtro SQL direto na query (`AND p.grupo_id = ?` quando `escopo > 0`). Retorna `404` se a pessoa não for do grupo. | Conforme |
| `/api/pessoas/{id}/pdf` | GET | `hPessoaPDF` | Consulta dados da pessoa e valida `if escopo > 0 && (gid == nil || *gid != escopo) { 403 }`. Relatórios de presença, cautelas e escalas só são gerados após a validação. | Conforme |
| `/api/pessoas/{id}/qr` | GET | `hPessoaQRCode` | Obtém `grupo_id` da pessoa e checa `if esc > 0 && (gid == nil || *gid != esc) { 403 }`. Geração SVG/PNG é bloqueada para outros grupos. | Conforme |
| `/api/pessoas/{id}/modificacoes` | GET | `hPessoaModificacoes` | Consulta `grupo_id` da pessoa na tabela `pessoas` e valida `gid == esc` para não-admins (`403` se for de outro grupo). | Conforme |
| `/api/pessoas/apresentacao` | GET | `hPessoaApresentacaoGet` | SQL direto: `WHERE p.grupo_id = ?` quando `esc > 0 && u.Papel != "admin"`. | Conforme |
| `/api/pessoas/{id}/apresentacao` | POST | `hPessoaApresentacaoSet` | Envelopado em `guardaGestaoPessoal`. Valida `grupo_id` da pessoa antes do upsert (`403` se pertencer a outro grupo). | Conforme |

### 2.2 Usuários (Contas, Senhas, Fotos, Papéis e Mover)

| Rota | Método | Handler | Mecanismo de Isolamento | Veredito |
|---|---|---|---|---|
| `/api/usuarios` | GET | `hUsuariosList` | Bloqueia contas sem papel e sem grupo (`403`). Admin vê todas; Gerente vê próprio + subordinados; Operador/Chefe vê somente o próprio grupo. Hash de senhas omitido para papéis não-administrativos. | Conforme |
| `/api/usuarios` | POST | `hUsuariosAdd` | Operador não cria contas (`403`). Gerente cria apenas `chefe_setor` no próprio grupo. Chefe cria apenas `operador` no seu setor. Encarregado/Auxiliar criam apenas `operador`/`chefe_setor` no próprio grupo. | Conforme |
| `/api/usuarios/{id}` | PATCH | `hUsuarioEdit` | Envelopado em `guardaGestaoPessoal`. Encarregado/Auxiliar só editam operadores/chefes do próprio grupo (`403` se alvo for gerente/admin ou outro grupo). | Conforme |
| `/api/usuarios/{id}` | DELETE | `hUsuarioExcluir` | Gerente só exclui operador/chefe do próprio grupo (`403` fora). Operador/chefe bloqueados. | Conforme |
| `/api/usuarios/{id}/senha` | POST | `hUsuarioSenha` | Exclusivo admin e gerente. Gerente limitado a operadores e chefes do próprio grupo (`403` caso contrário). | Conforme |
| `/api/usuarios/{id}/foto` | GET | `hUsuarioFotoGet` | Recuperação de avatar público/autenticado por ID. | Conforme |
| `/api/usuarios/{id}/mover` | PATCH | `hMoverConta` | Exclusivo admin e gerente. Gerente restrito à sua própria árvore hierárquica (origem e destino devem estar na árvore de subordinação). | Conforme |
| `/api/perfil` | GET / PATCH | `hPerfilGet` / `hPerfilSet` | Operação restrita ao próprio usuário autenticado (`u.ID`). | Conforme |
| `/api/usuarios/{id}/papeis` | POST / DELETE | `hUsuarioPapelAdd` / `hUsuarioPapelDel` | Gerente restrito a gerenciar papéis de operador/chefe_setor em seu próprio grupo e subordinados ativos. | Conforme |
| `/api/sessao/contexto` | POST | `hMudarContexto` | Validação estrita de que o `papel_id` pertence ao usuário logado (`u.ID`). | Conforme |

---

## 3. Análise de Estilo: SQL no Fio vs. Validação no Handler

- **SQL no Fio (`hPessoaFicha`, `hPessoasList`, `hPessoaApresentacaoGet`):**
  A cláusula `WHERE grupo_id = ?` é incorporada à consulta SQL. Quando o registro pertence a outro grupo, a consulta retorna zero linhas, disparando `404 Not Found`. Não há tráfego de dados indevidos do banco para a memória do servidor.
- **Validação no Handler (`hPessoaPDF`, `hPessoaQRCode`, `hPessoaModificacoes`, `hPessoasEdit`):**
  A consulta busca a entidade por `id` e o handler inspeciona o campo `grupo_id`, retornando `403 Forbidden` ou `404 Not Found` antes de qualquer processamento ou emissão de dados sensíveis.
- **Julgamento:**
  Ambos os estilos cumprem integralmente a garantia de confidencialidade e multi-tenancy. Seguindo a diretriz de **proibição de refactors cosméticos sem furo comprovado**, a arquitetura atual foi preservada integralmente.

---

## 4. Conclusão

O subsistema de pessoal apresenta isolamento mecânico robusto em todas as camadas de controle e persistência. Nenhuma modificação em código de produção foi necessária para fechamento de escopo nesta auditoria.
