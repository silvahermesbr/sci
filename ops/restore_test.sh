#!/bin/bash
# SCI — restore: valida um backup abrindo-o e contando tabelas/presenças.
# Uso: bash ops/restore_test.sh dados/backups/sci_YYYYMMDD_HHMMSS.db
BK="$1"
[ -f "$BK" ] || { echo "uso: $0 <arquivo .db do backup>"; exit 1; }
sha_ok="n/a"
[ -f "$BK.sha256" ] && sha_ok=$(sha256sum "$BK" | awk '{print $1}') && \
  esperado=$(awk '{print $1}' "$BK.sha256") && \
  [ "$sha_ok" = "$esperado" ] && sha_ok="OK" || sha_ok="DIVERGE"
echo "sha256: $sha_ok"
cp "$BK" /tmp/sci_restore_probe.db
SCI_DATA_DIR=/tmp/sci_restore_probe_dir sci restore-probe 2>/dev/null || \
  python3 - "$BK" <<'EOF'
import sqlite3, sys
con = sqlite3.connect(sys.argv[1])
tabs = [r[0] for r in con.execute("SELECT name FROM sqlite_master WHERE type='table'")]
print("integrity:", con.execute("PRAGMA integrity_check").fetchone()[0])
print("tabelas:", len(tabs), "| presencas:", con.execute("SELECT COUNT(*) FROM presencas").fetchone()[0],
      "| pessoas:", con.execute("SELECT COUNT(*) FROM pessoas").fetchone()[0])
print("RESTORE OK" if len(tabs) >= 12 else "RESTORE SUSPEITO")
con.close()
EOF
rm -f /tmp/sci_restore_probe.db
