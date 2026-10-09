# Execução da Fase 1: Fundação de Identidade (Concluída)

A primeira fase do Plano Mestre v1.2 já está implementada, compilada e rodando no seu host local. O foco foi a transição para a **Autenticação Única**, dividindo o papel dos militares entre *Registros Planos* (sem acesso) e *Usuários* de forma fluida.

## Modificações Realizadas:

1. **Alteração do Schema e Migrações (`store.go`)**
   - Adicionado o campo `precisa_setup` (default `1`) na tabela de usuários de forma retroativa para que usuários criados anteriormente tenham que obrigatoriamente se re-identificar no padrão V1.2.
   - O objeto `UsuarioDaSessao` foi atualizado para trafegar essa flag do banco de dados para os *handlers* da API.

2. **Interceptação de Segurança (`auth.go`)**
   - O middleware de autenticação principal agora avalia a flag `u.PrecisaSetup`.
   - Se o usuário precisar de setup, **todas as rotas da intranet são bloqueadas** com HTTP 403 (exceção feita apenas para as rotas `/api/logout` e `/api/setup`). 

3. **Novo Endpoint de Resolução (`server.go`)**
   - Criada a rota `POST /api/setup`.
   - Essa rota aceita um payload `{ novo_login, nova_senha }` validando e hasheando a nova credencial de acesso. A rota automaticamente converte o login fornecido para formato CPF/IDT e zera a flag de `precisa_setup`, inserindo na trilha de auditoria ("login atualizado").

4. **Criação Rápida e Senha Padrão (`views_gestao.js` & `server.go`)**
   - No painel de administração (Aba "Efetivo e Acessos" > "Novo Usuário"), a senha não é mais um campo obrigatório. 
   - Modificado o *Controller* (`hUsuariosAdd`) para assumir a string `"sci"` como senha caso nenhuma seja fornecida, e o novo usuário *sempre* entra com `precisa_setup = 1` por padrão.

5. **Modal Force-Capture (`core.js`)**
   - A função mestre de `fetch` do Frontend (`window.api`) agora captura em tempo real qualquer pacote recusado pelo servidor que contenha `req_setup: true`.
   - Quando ativado, a rotina dispara a função `window.showSetupModal()`, um modal **Infechável** e **Tela-cheia** ordenando: "Atualização Obrigatória (v1.2). Para continuar, você precisa atualizar seu Login para um Nº de Identificação Único e cadastrar uma nova senha."

## Próximos Passos
Por motivos de estabilidade e segurança, dividi a execução do Plano Mestre em suas respectivas fases — afinal, injetar o *Drive Local*, *Avisos* e o *Calendário* de uma única vez em cima de um sistema vivo poderia causar danos irreversíveis ao banco de dados.

O `sci.exe` já foi reiniciado. Abra a URL local e verifique a nova imposição de login! Quando aprovar a Fase 1, partiremos direto para a **Fase 2 (Fórum de Avisos e Despachos Internos)**.
