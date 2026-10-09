# CI local do SCI — build + smoke mínimo (roda antes de qualquer deploy)
set -e
export PATH=/opt/data/.local/go/bin:$PATH
cd "$(dirname "$0")"
CGO_ENABLED=0 go build -trimpath -ldflags "-s -w" -o /tmp/sci_ci ./...
echo "build OK"
# smoke: sobe em porta alta, health, backup CLI, derruba
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
