# SCI — Sistema de Controle Interno (3º B Com GE)

Controle digital de presença por **conferências de pessoal**. 1 binário Go + SQLite em
arquivo + frontend embutido — sem Docker, sem npm, sem serviços externos.

## Rodar

```bash
SCI_PORT=10003 SCI_DATA_DIR=./dados ./sci
```

- Primeiro boot cria `admin/admin` (senha padrão — troque no botão "Senha"; mín. 8).
- `SCI_PORT` (padrão 10003) · `SCI_DATA_DIR` (padrão `./dados`) · `SCI_OM_TITULO`
  (padrão "SCI - Sistema de Controle Interno").

## Fluxo de conferência

1. **Conferência** → iniciar (data + local opcional). A conferência fica **ABERTA**.
2. Lista do efetivo ativo por setor: todo mundo **presente por padrão**; toque no nome
   cicla `presente → falta → atraso → justificada`; falta pede **motivo**, justificada
   pede **destino + motivo**; ✅ marca "verifiquei" (contador n/N na barra).
3. **✕ Fechar conferência** grava tudo em 1 transação e arquiva.
4. **Conferências** (lista agrupada por dia): status Aberta/Fechada, horário de criação
   ou fechamento, operador, **Relatório PDF** (só para fechadas — com horário de
   fechamento, horário de geração e nome do operador).
5. **Relatórios** por dia/semana/mês/período, com **% EFETIVO PRONTO** (só presentes sem
   ressalva) — impressão em preto e branco.

## Zero perda de dados

- Banco 100% em arquivo (WAL + synchronous FULL + FK); sessões e auditoria em tabelas.
- Backup automático: a cada conferência fechada, no boot, via botão (com download no
  navegador), via `sci backup` (cron) — `VACUUM INTO` + sha256 + MANIFEST; falha acende
  FLAG.
- Migrações de schema versionadas no binário (schema_migrations v1..v3).
- Nada é apagado: fechado não se edita; comentários são append-only.

## Admin

- CRUD de efetivo, catálogos (setores, funções, destinos, tags, tipos de conferência),
  contas (usuário/admin) com redefinição de senha, backup.

## Operação no host

```bash
# subir (sobrevive à sessão):
cd ~/projetos/sci && setsid nohup env SCI_PORT=10003 SCI_DATA_DIR=$HOME/projetos/sci/dados \
  ./sci >> server.log 2>&1 < /dev/null &
# watchdog (re-levanta em 3 falhas; cron */5 de reforço):
setsid nohup bash ops/watchdog_sci.sh >> watchdog.log 2>&1 < /dev/null &
```

## Decisões registradas

`../PLANO_DECISOES_2026-09-28.md` (status, comentários pós-fechamento, justificada =
falta justificada, várias conferências/dia, grupos admin→gerente→operador etc.).
