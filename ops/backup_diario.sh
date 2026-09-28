#!/bin/bash
# SCI — backup diário via cron do user aimi (subcomando nativo, sem sqlite3 CLI)
# Crontab:  30 3 * * *  /opt/data/workspace/projetos/sci/app/ops/backup_diario.sh >> ~/projetos/sci/backup.log 2>&1
export SCI_DATA_DIR="${SCI_DATA_DIR:-$HOME/projetos/sci/dados}"
BIN="$HOME/projetos/sci/sci"
[ -x "$BIN" ] || BIN=/opt/data/workspace/projetos/sci/app/sci
"$BIN" backup
rc=$?
# FLAG_BACKUP.txt (se existir) = falha visível p/ watchdog; retenção 15 dias
find "$SCI_DATA_DIR/backups" -name 'sci_*.db*' -mtime +15 -delete 2>/dev/null
# rotação simples de log (revisão SHORYU §1.4): >10 MB -> mantém só 1 MB recente
LOG="$SCI_DATA_DIR/../server.log"
[ -f "$LOG" ] || LOG="$HOME/projetos/sci/server.log"
if [ -f "$LOG" ] && [ "$(stat -c%s "$LOG" 2>/dev/null || echo 0)" -gt 10485760 ]; then
  tail -c 1048576 "$LOG" > "$LOG.tmp" && mv "$LOG.tmp" "$LOG"
fi
exit $rc
