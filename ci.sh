# CI local do SCI — GATE DE VERDADE (v1.5.4-F, fecha R-28): vet + build + test, todos obrigatorios.
# Qualquer etapa que falhe = exit 1 (CI vermelho, o merge nao passa). Rodar antes de qualquer deploy.
# NOTA -race: `go test -race ./...` exige cgo/gcc e e Linux-only — NAO executa aqui (a suíte roda
# sem -race em qualquer host). No host Linux rodar `go test -race ./...` + este script (`bash ci.sh`,
# que tambem e Linux-only pelo PATH do go) — ver docs/INSTRUCOES_AGENTES.md.
set -e
export PATH=/opt/data/.local/go/bin:$PATH
cd "$(dirname "$0")"

# 1) vet: erro estatico e bloqueio
go vet ./... || { echo "vet FALHOU"; exit 1; }
echo "vet OK"

# 2) build: prova o binario (hardening de release: CGO off, trimpath, ldflags -s -w)
CGO_ENABLED=0 go build -trimpath -ldflags "-s -w" -o /tmp/sci_ci ./... || { echo "build FALHOU"; exit 1; }
echo "build OK"

# 3) teste: suíte inteira, sem cache (-count=1), ~190s — falha = exit 1
go test -count=1 ./... || { echo "test FALHOU"; exit 1; }
echo "test OK"

# 4) smoke: sobe em porta alta, health, backup CLI, derruba
SCI_DATA_DIR=/tmp/sci_ci_dados SCI_PORT=14098 SCI_ADMIN_SENHA=ci /tmp/sci_ci &
PID=$!
# start frio com migrações novas passa de 1s — sondar até 10s (lição v365: corrida do sleep fixo)
HEALTH=falhou
for i in $(seq 1 20); do
  sleep 0.5
  if curl -s -o /dev/null http://127.0.0.1:14098/api/health; then HEALTH=ok; break; fi
done
[ "$HEALTH" = "ok" ] && echo "health OK" || { echo "health FALHOU"; kill $PID; exit 1; }
kill $PID
SCI_DATA_DIR=/tmp/sci_ci_dados /tmp/sci_ci backup && echo "backup CLI OK"
rm -rf /tmp/sci_ci_dados
echo "CI VERDE"
