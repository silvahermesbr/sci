# SCI — Roadmap (pós v1.5)

> Atualizado em 2026-10-06. Branch de referência: `feat/v1.5-evolucao` (`0fdfb39`).
> Pendências detalhadas e contexto técnico: ver seção **"Estado Atual"** em `HANDOFF.md`.

## Ordem de módulos acordada com o usuário

1. Mural de Avisos
2. Relatórios *(Gerente)*
3. Mensagens
4. Drive
5. Conferências *(Gerente / Encarregado de Pessoal / Chefe de Setor / Operadores)*
6. Gerenciar Grupo *(Gerente)*
7. Pessoal *(Encarregado de Pessoal)*
8. Material *(Encarregado de Material / Chefes de Setor)*
9. … outros módulos

Calendário, Escalas e Consciência Situacional estão **ocultos** e voltam em versões futuras.

---

## v1.5.1 — Funções e permissões (prioridade ALTA)

**Objetivo:** fechar a divisão de responsabilidades do grupo.

- [ ] **Aba "Funções" em Gerenciar Grupo (P1):** o gerente seleciona, entre os usuários do próprio grupo, o Encarregado de Pessoal, o Encarregado de Material e os respectivos auxiliares.
  - Backend: garantir que o gerente só atribua/remova essas funções dentro do próprio escopo.
  - Aceite: atribuir e remover pelo UI reflete no dropdown de contexto do usuário afetado sem precisar de F5 no próximo login.
- [ ] **Módulo "Pessoal" com rota própria (P2):** extrair o Banco de Pessoal de `#/grupos` para `#/pessoal`, acessível ao gerente e ao encarregado de pessoal.
- [ ] **Menu filtrado por papel ativo (P3):** cada item aparece apenas para os papéis da lista acima.
- [ ] **Bloqueio das rotas ocultas (P5):** `#/calendario`, `#/escalas` e `#/consciencia` redirecionam para a rota inicial enquanto estiverem fora do escopo.
- [ ] **Testes de regressão (P6):** papéis presentes na resposta de `/api/login`; gerente editando membro do grupo com papel base diferente de operador; usuário fora do escopo continua bloqueado (403).

**Critério de saída:** `go vet`, `go build` e `go test` verdes; checklist manual por papel (gerente, enc. pessoal, enc. material, chefe de setor, operador) validando o menu e o acesso.

## v1.5.2 — Estabilização para produção (prioridade MÉDIA)

- [ ] Revisão de permissões no backend para os novos papéis (`encarregado_pessoal`, `encarregado_material` e auxiliares) em todos os endpoints de Pessoal e Material — o frontend esconder um item não é controle de acesso.
- [ ] Auditoria de consultas aninhadas com `*sql.Rows` aberto (risco de deadlock com `SetMaxOpenConns(1)`), adicionando um teste com timeout curto para os endpoints de listagem.
- [ ] Smoke test de deploy: binário limpo, banco novo, seed do admin, criação de grupo, primeiro gerente e primeira conferência.
- [ ] Atualizar `CHANGELOG.md` e `README.md` com o fluxo de funções.

## v1.6 — Reintrodução de módulos (prioridade BAIXA / futuro)

- [ ] **Consciência Situacional:** corrigir o modal de conferência que transborda lateralmente (P4), revisar responsividade e reexibir no menu.
- [ ] **Escalas e Calendário:** revalidar com a nova divisão de funções antes de reexibir.
- [ ] Itens herdados do roadmap v1.1 ainda abertos: notificações/alertas de cautelas em atraso e visão gráfica de escalas.
