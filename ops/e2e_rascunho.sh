#!/usr/bin/env bash
# E2E — RASCUNHO PERMANENTE da conferência (ordem Tenente 30/09).
# Cenário: iniciar conferência → lançar situação/observação (autosave /marcar, o que o
# front manda a cada toque) → trocar DESTINO (novo handler do drop-down grava na hora)
# → "fechar a página" → RECARREGAR (novo GET /conferencia/hoje) → TUDO lá.
set -u
PORT=14123
DATA=$(mktemp -d /tmp/sci_e2e_rasc.XXXX)
SCI_DATA_DIR="$DATA" SCI_PORT=$PORT /tmp/sci_ci > /tmp/sci_e2e_rasc.log 2>&1 &
PID=$!
trap 'kill $PID 2>/dev/null; rm -rf "$DATA" /tmp/e2e_rasc_*' EXIT

B="http://127.0.0.1:$PORT"
for i in $(seq 1 30); do curl -s -o /dev/null "$B/api/health" && break; sleep 0.5; done

AJ=/tmp/e2e_rasc_admin; GJ=/tmp/e2e_rasc_ger
jqid() { python3 -c 'import sys,json
o=json.load(sys.stdin)
if isinstance(o,dict):
    if "id" in o: print(o["id"]); raise SystemExit
raise SystemExit("sem id: "+json.dumps(o))'; }

curl -sf -c "$AJ" -H 'Content-Type: application/json' -d '{"login":"admin","senha":"admin"}' "$B/api/login" > /dev/null
# grupo nasce COM gerente (contrato do POST /api/grupos)
RESP=$(curl -sf -b "$AJ" -H 'X-SCI: 1' -H 'Content-Type: application/json' \
  -d '{"nome":"Cia Rascunho E2E","login":"gere2e","senha":"gere2e123","nome_guerra":"GER E2E"}' "$B/api/grupos")
echo "$RESP" | python3 -c 'import sys,json; d=json.load(sys.stdin); print("grupo criado:", d)' | head -1
GID=$(echo "$RESP" | jqid)
curl -sf -c "$GJ" -H 'Content-Type: application/json' -d '{"login":"gere2e","senha":"gere2e123"}' "$B/api/login" > /dev/null

PPID_=$(curl -sf -b "$GJ" -H 'X-SCI: 1' -H 'Content-Type: application/json' -d '{"nome_guerra":"RASCUNHO","nome_completo":"Rascunho E2E"}' "$B/api/pessoas" | jqid)
CID=$(curl -sf -b "$GJ" -H 'X-SCI: 1' -H 'Content-Type: application/json' -d '{"local":"E2E"}' "$B/api/conferencia/iniciar" | jqid)

# 1º toque do operador: falta + observação (autosave)
curl -sf -b "$GJ" -H 'X-SCI: 1' -H 'Content-Type: application/json' \
  -d "{\"pessoa_id\":$PPID_,\"situacao\":\"falta\",\"observacao\":\"lançado na terça à noite\",\"verificado\":true}" \
  "$B/api/conferencia/marcar?id=$CID" > /dev/null
# 2º toque: vira justificada COM destino (o novo handler do drop-down grava na hora)
DID=$(curl -sf -b "$GJ" -H 'X-SCI: 1' -H 'Content-Type: application/json' -d '{"nome":"Destino Rascunho"}' "$B/api/catalogo/destinos" | jqid)
curl -sf -b "$GJ" -H 'X-SCI: 1' -H 'Content-Type: application/json' \
  -d "{\"pessoa_id\":$PPID_,\"situacao\":\"justificada\",\"destino_id\":$DID,\"observacao\":\"mudou o destino\",\"verificado\":true}" \
  "$B/api/conferencia/marcar?id=$CID" > /dev/null

# "FECHOU A PÁGINA" → RECARGA: novo GET é exatamente o que o browser faz ao reabrir
curl -sf -b "$GJ" "$B/api/conferencia/hoje?id=$CID" | PID="$PPID_" DID="$DID" python3 -c '
import sys, json, os
d = json.load(sys.stdin)
e = (d["conferencia"]["estados"] or {}).get(str(os.environ["PID"]))
assert e, "estado NÃO persistiu (rascunho perdido!): " + json.dumps(d["conferencia"]["estados"])
assert e["situacao"] == "justificada", e
assert str(e["destino_id"]) == os.environ["DID"], e
assert e["observacao"] == "mudou o destino", e
assert e["verificado"] in (1, True), e
print("RASCUNHO PERMANENTE OK — estado sobreviveu à recarga:", e)
'
echo "E2E RASCUNHO: VERDE"
