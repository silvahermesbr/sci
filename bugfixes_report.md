# Alterações e Correções Efetuadas

## BUGS Visuais e Funcionais Corrigidos

1. **Ajustar transbordo (overflow) vertical do modal de ficha de usuário**
   - Foi adicionado rolagem (`overflow-y: auto`) e limite de tamanho base (`max-height: 90vh`) para o modal `modalEditarPerfilUsuario` no painel de gestão para evitar que o modal transbordasse e cortasse os botões em telas menores.

2. **Permitir remover o gerente de qualquer grupo (menos Admin)**
   - Corrigido travamento no `server.go` que impedia excluir ou mover um gerente quando era o único do grupo.
   - Adicionada opção de demitir o gerente atual para as funções de operador na opção de **"Trocar Gerente"** sem obrigatoriamente eleger um substituto, permitindo deixar a gerência vaga.

3. **Aba de conferências não atualizando corretamente**
   - Resolvido um problema de sincronia de tempo com a inicialização dos seletores de datas (`p.dia` era inicializado como `undefined` antes da chamada da API). Como consequência, as requisições estavam buscando dados para o período "NaN", e as conferências recentes não apareciam até que você apertasse manualmente "Aplicar". O carregamento automático já está ajustado.

4. **Limite de 3 comentários em modal de conferência**
   - O modal rápido na janela de conferência agora exibe apenas os 3 comentários mais recentes efetuados sobre o militar (com indicativo textual caso haja mais comentários omitidos).

5. **Sem acesso aos comentários gravados na ficha de usuários**
   - Adicionado novo botão ("Ver Histórico de Comentários") diretamente no Relatório Individual (Busca Individual) que resgata todos os comentários emitidos sobre aquele militar em conferências do passado, associando datas, quem operou o registro e a tag.

6. **Relatório de presença geral com bugs visuais: alinhamento de situações "flutuando"**
   - Corrigido em `relatorio.go` um erro no eixo Y do motor de PDF na hora de renderizar os rótulos de cores da barra segmentada. Eles estavam "flutuando" para cima no PDF ao se iterar sobre os valores de "PROPORÇÃO DE SITUAÇÕES". Agora estão nivelados.

7. **Na aba de relatorios, quantidades refletindo status mais atualizado**
   - A forma como o motor extrai dados (`montarBundle`) no backend foi fundamentalmente alterada. Em relatórios de Efetivo (consolidados e relatórios diários de período), em vez de exibir todas as chamadas *somadas* do militar naquele período de datas (ex: 2 presentes, 2 faltas), ele isola matematicamente o **status na ÚLTIMA conferência daquele período**.
   - Consequentemente, o cálculo global de "Efetivo Pronto", de "Faltas", "Validez por Setor", etc., não é mais poluído por duplicatas de presenças contínuas passadas. Todos os relatórios mostram agora a fotografia instantânea e exata do militar baseado em seu último registro.


## Status

- **Patches de Segurança e Build:** Aplicados e atualizados no binário.
- **Servidor Local:** Reiniciado e disponível em sua porta de teste (`:10003`).

Verifique se todas as correções atenderam de forma satisfatória ao uso no dia a dia. Você também pode visualizar o plano completo para V1.2 no `v1_2_master_plan.md` no painel de artefatos que criei e confirmarmos a transição para as novidades.
