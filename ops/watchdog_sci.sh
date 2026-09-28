#!/bin/bash
# SCI — watchdog: 3 falhas seguidas em /api/health -> mata por PID exato e sobe de novo.
# Início (SOBREVIVE à sessão — padrão validado no host):
#   setsid nohup bash ops/watchdog_sci.sh >> ~/projetos/sci/watchdog.log 2>&1 < /dev/null &
# NÃO duplicar: o lock abaixo garante 1 instância por porta.
PORTA="${SCI_PORT:-10003}"
DIR="${SCI_DATA_DIR:-$HOME/projetos/sci/dados}"
BIN="$HOME/projetos/sci/sci"
LOCK="/tmp/sci_watchdog_${PORTA}.lock"
[ -f "$LOCK" ] && kill -0 "$(cat "$LOCK")" 2>/dev/null && exit 0   # já há watchdog
echo $$ > "$LOCK"
trap 'rm -f "$LOCK"' EXIT

falhas=0
while true; do
  curl -s -o /dev/null "http://127.0.0.1:${PORTA}/api/health"
  if [ $? -eq 0 ]; then
    falhas=0
  else
    falhas=$((falhas+1))
    echo "$(date '+%F %T') falha $falhas/3 na porta $PORTA"
    if [ "$falhas" -ge 3 ]; then
      # matar por PID EXATO (nunca pkill -f — casa com o próprio shell)
      PID=$(ss -tlnp | grep ":${PORTA}" | grep -oP 'pid=\K\d+' | head -1)
      [ -n "$PID" ] && kill -9 "$PID" && echo "morto PID $PID"
      sleep 1
      setsid nohup env SCI_PORT="$PORTA" SCI_DATA_DIR="$DIR" "$BIN" \
        >> "$DIR/../server.log" 2>&1 < /dev/null &
      echo "$(date '+%F %T') SCI re Levantado na porta $PORTA"
      falhas=0
    fi
  fi
  sleep 30
done
