# SCI — PLANO DE IMPLEMENTAÇÃO (decisões do Tenente, 28/09/26)

> Fonte: respostas do Tenente no chat (28/09/26), transcritas na íntegra.
> Status: PLANO APROVADO — implementação em execução. Deploy do v6 (conferências) fica
> para junto do v7 (decisões + grupos), evitando dois upgrades no mesmo dia.

## 1) STATUS DO MILITAR
- Defaults: **ativo / inativo**, editáveis pelo admin (catálogo "status de pessoal").
- TODO militar deve ter status. **Regra de conferência:** só gera relatório/fecha com
  100% da conferência marcada (o checkbox do Tenente vira obrigatório? NÃO — segue
  permitindo fechar com alerta; a trava forte é: sem lançamento, não fecha).
- **Imutabilidade histórica:** mudanças de status/campanha NÃO alteram relatórios antigos.
  Implementação: relatórios agregam por lançamento; status atual só afeta listas futuras
  (novo campo "desde" no status p/ auditoria).

## 2) VERIFICADOR
- **Qualquer conta usuário** pode iniciar/fechar conferência (ratificado — como está).

## 3) EDIÇÃO PÓS-FECHAMENTO
- **Ninguém edita lançamento fechado** (nem admin).
- **Comentários** em tabela própria: `comentarios (ordem, datahora, pessoa_id,
  conferencia_id, operador_id, comentario)` — append-only, exibidos no relatório da
  conferência e na lista.

## 4) JUSTIFICADA = FALTA JUSTIFICADA
- Tudo que não é presente é FALTA (justificada ou não). Posicionamento separado nos
  relatórios: bloco FALTAS NÃO JUSTIFICADAS x FALTAS JUSTIFICADAS.
- % presença = presentes / total (atraso PASSA A PUNIR o % pronto; o quadro continua
  mostrando as linhas separadas).

## 5) VÁRIAS CONFERÊNCIAS POR DIA + DASHBOARD POR DIA
- Removida a trava 1/dia/tipo (mantém: 1 ABERTA por vez no sistema).
- Lista "Conferências" agrupada por dia (dashboard diário).

## 6) HORÁRIO DE CORTE
- A: sem hora oficial; atraso lançado a dedo pelo operador (como está).

## 7) RETENÇÃO
- B: exportar/arquivar por ano (rota /api/export/ano/{ano} na v7+).

## 8) DESTINOS/CAMPAÑHA — "missão externa/serviço"
- Catálogo de destinos deve nascer com: **Serviço, SSV (saindo de serviço),
  Missão externa, Curso, Hospital, Licença, Trânsito, CMA** (editável).
- "No exterior da OM" é tratado por destino (categoria própria do catálogo), não por
  status novo.

## 8b) MUDANÇA DE DESTINO/STATUS NÃO REESCREVE PASSADO
- Já garantida pelo modelo (relatórios leem lançamentos; catálogo muda só o futuro).

## 9) CALENDÁRIO
- A: só conferências por enquanto.

## 10) CONTAS
- Individual por operador (auditoria de quem conferiu e quem gerou relatório) — como está.

## 11) CABEÇALHO
- **"SCI - Sistema de Controle Interno"** (sem OM), default novo.

## 12) NOTIFICAÇÕES
- A: nenhuma por ora.

## 13) BACKUP EXTERNO
- C: nenhum por ora (backups locais + crons mantidos).

## FASE GRUPOS (planejamento aprovado — executar após o v7)
- **ADMIN:** acesso a tudo; **ÚNICO que cria contas de GERENTE** (entrega login+senha).
- **GERENTE:** cria contas de OPERADOR do seu grupo; edita especificações do grupo
  (nome, setores, destinos, efetivo); faz e fecha conferências do grupo.
- **Multi-grupo:** cada grupo = banco de pessoal próprio + conferências próprias.
- Desenho mínimo: tabela `grupos (id, nome, criado_em)` + `pessoas.grupo_id` +
  `conferencias.grupo_id` + `usuarios.grupo_id` (NULL = admin global);
  papel novo `gerente`; escopo por grupo em todas as queries (WHERE grupo_id = ?).
- Admin vê tudo (sem filtro); gerente vê só o seu grupo; operador do grupo idem gerente
  sem poderes de edição de catálogo/contas.
- Backlog técnico: escolher entre "coluna grupo_id" (mais simples, adotado) vs
  "banco por grupo" (descartado — complica backup/relatórios globais).
