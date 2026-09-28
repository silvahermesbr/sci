# SCI — Sistema de Controle Interno (3º B Com GE)

Controle digital de presença em formaturas. **1 binário Go + SQLite em arquivo + front embutido** — sem Docker, sem serviços externos, sem build-step no front.

## Rodar

```bash
SCI_PORT=10003 SCI_DATA_DIR=./dados SCI_ADMIN_SENHA=<senha> ./sci
```

- `SCI_PORT` (padrão 10003) · `SCI_DATA_DIR` (padrão `./dados`) · `SCI_ADMIN_SENHA` (padrão `sci12345` — TROCAR no 1º boot; seed só roda com banco vazio) · `SCI_OM_TITULO` (cabeçalho do PDF).
- Login: `admin` + a senha do seed. Novas contas em **Admin → usuários** (admin ou usuário).
- Front: qualquer navegador na rede local (tablet na formatura, desktop do admin).

## Build (host de dev = host de deploy)

```bash
CGO_ENABLED=0 go build -trimpath -ldflags "-s -w" -o sci ./...
```

Toolchain no home do host (sem sudo): Go 1.27 em `/opt/data/.local/go/bin`.

## Zero perda de dados — doutrina

1. **Tudo em arquivo**: banco `dados/sci.db` (WAL + `synchronous=FULL` + FK ON), sessões em tabela (sobrevivem a restart), auditoria em tabela.
2. **Backup automático**: a cada formatura confirmada, no boot, via `POST /api/backup` (admin) e via CLI — `VACUUM INTO` (cópia consistente com o servidor no ar) → `backups/sci_YYYYMMDD_HHMMSS.db` + `.sha256` + linha no `MANIFEST.txt`. Falha vira `dados/FLAG_BACKUP.txt` (visível).
3. **Backup diário (cron)**: `ops/backup_diario.sh` (retém 15 dias). Entrada sugerida:
   `30 3 * * * /opt/data/workspace/projetos/sci/app/ops/backup_diario.sh >> ~/projetos/sci/backup.log 2>&1`
4. **Teste de restore (mensal)**: `bash ops/restore_test.sh dados/backups/sci_<ts>.db` — valida sha256 + integrity + contagens.
5. **Portabilidade**: export CSV em `Pessoas/Relatórios` (`/api/export/{pessoas|presencas|formaturas}`) + o próprio `.db` é SQLite padrão.

## Operação no host (10.10.0.5)

```bash
# subir (sobrevive à sessão):
setsid nohup env SCI_PORT=10003 SCI_DATA_DIR=$HOME/projetos/sci/dados SCI_ADMIN_SENHA=... \
  $HOME/projetos/sci/sci >> $HOME/projetos/sci/server.log 2>&1 < /dev/null &
# watchdog (3 falhas -> mata por PID exato e re-levanta):
setsid nohup bash /opt/data/workspace/projetos/sci/app/ops/watchdog_sci.sh \
  >> $HOME/projetos/sci/watchdog.log 2>&1 < /dev/null &
# parar: SEMPRE por PID exato (nunca pkill -f):
PID=$(ss -tlnp | grep :10003 | grep -oP 'pid=\K\d+' | head -1); kill "$PID"
```

## Regras de negócio cravadas no MVP

- Lançamento **exceção-first**: todos presentes por padrão; toque cicla `presente → atraso → falta → justificada`; falta/justificada **exige destino** (catálogo do admin).
- `CONFIRMAR` grava N tickets em **1 transação** e fecha a formatura; re-confirmar no mesmo dia faz **upsert** (correção), nunca duplica (`UNIQUE formatura+pessoa`).
- % de presença = (presentes + atrasos) ÷ convocações; **justificada fora da razão**; tabela ordenada **piores primeiro**.
- Nada é apagado: correção é UPDATE com `alterado_por/em`; exclusão de catálogo = desativação.
- 1 formatura do mesmo tipo por dia (`UNIQUE data+tipo`).

## Decisões pendentes do Tenente

Ver `../SCI_brainstorm_consolidado.pdf` seção 5 (13 itens — status da pessoa, prazo de correção, horário de corte, LGPD etc.). O MVP foi construído com os defaults prudentes documentados lá.

## Estrutura

```
app/  main.go (boot, CLI backup) · server.go (rotas, backup, export)
      store.go (schema, pragmas, sessões) · auth.go (argon2id, rate-limit)
      relatorio.go (A4 fpdf) · web/ (index.html, app.js, style.css — embed)
ops/  backup_diario.sh · watchdog_sci.sh · restore_test.sh
```
