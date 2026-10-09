# SCI — Sistema de Controle Interno (3º B Com GE)

Controle digital de **presença por conferências** (pessoal e material) da OM.
Um binário Go com frontend embutido e banco SQLite em arquivo: **sem Docker, sem npm,
sem serviços externos**. Um processo, um arquivo de dados, backup integrado.

## Stack

- **Backend:** Go puro (servidor HTTP + API + migrações de schema no binário).
- **Banco:** SQLite em arquivo (`SCI_DATA_DIR`), modo WAL, migrations versionadas
  (`schema_migrations`, schema atual **v41**).
- **Frontend:** embutido no binário (pasta `web/`), servido pelo próprio Go.

## Como rodar

```bash
SCI_PORT=10003 SCI_DATA_DIR=./dados ./sci
```

- Primeiro boot cria **admin/admin** — troque a senha no botão "Senha" (mín. 8 caracteres).
- Variáveis: `SCI_PORT` (padrão 10003), `SCI_DATA_DIR` (padrão `./dados`).

## Build e CI

```bash
go build ./...        # compila
go test ./...         # suíte completa de testes
bash ci.sh            # build + health + backup (o gate de CI do projeto)
```

## Estrutura

```
*.go            backend na raiz (servidor, módulos, migrações, testes _test.go)
web/            frontend embutido (JS puro, sem build)
ops/            E2E de produção, watchdog e scripts de operação
docs/           handoffs, plano de decisões, roadmap, relatório de limpeza
docs/historico/ relatórios históricos de ondas/fases encerradas
ci.sh           gate de CI (build + health + backup)
```

## Módulos

- **Conferência** por setor e por **antiguidade**, com pré-fechamento e relatório PDF.
- **Pessoal** — efetivo com funções e **encarregados** (cadeiras com chave: Gerente,
  Enc. Pessoal, Enc. Material).
- **Material** — itens por setor, viaturas, checklist de conferência, pronto PDF.
- **Grupos** e **Relatórios** (simples e detalhado, por período).
- **Drive local** (anexos), **Email interno** e **Mural de avisos**.

## Segurança dos dados (LGPD)

O diretório `dados/` contém dados pessoais de militares e **NUNCA vai ao repositório**
(já coberto pelo `.gitignore`). Backups ficam fora do controle de versão e usam
`VACUUM INTO` + sha256 + MANIFEST.
