#!/usr/bin/env bash
# E2E — P4 (ordem 06/10, item 4): SINCRONIZAÇÃO com HASH DE ESTADO POR SETOR.
# Cenário: gerente cria 2 setores + pessoal → abre conferência →
#   E1. GET /api/conferencia/estado devolve {hash_geral, setores:[{setor_id,hash}]}
#       — hash NÃO vazio, um bloco por setor, hash_geral estável em 2 leituras
#       (mesmo estado = mesmo hash, pré-condição do zero-mutação no cliente);
#   E2. marcar 1 militar do setor A muda o hash DO SETOR A e o hash_geral;
#       hash do SETOR B permanece (segregação por setor);
#   E3. concluir o setor B muda o hash do B (status entra no hash);
#   E4. guardas: sem cookie → 401 (mesma barreira do /hoje).
# Contratos provados: /api/conferencia/estado (SHA-1 por setor, UMA query),
# hash determinístico, sensibilidade a lançamento/status/contagem.
# Knobs: SCI_PORT=14216 · SCI_DATA_DIR=/opt/data/tmp_smoke_estado · binário em
# diretório PRÓPRIO (nunca rm -rf do dir que contém o servidor em teste).
set -u
PORT=${SCI_PORT:-14216}
DATA=${SCI_DATA_DIR:-/opt/data/tmp_smoke_estado}
BIN=${SCI_BIN:-/opt/data/tmp_smoke_estado_bin/sci_ci}
REPO=$(cd "$(dirname "$0")/.." && pwd)

if [ ! -x "$BIN" ]; then
  mkdir -p "$(dirname "$BIN")"
  (cd "$REPO" && CGO_ENABLED=0 go build -o "$BIN" .) || { echo "FALHA no build"; exit 1; }
fi

# ghost de smoke anterior: matar pelo PID do MESMO binário antes de limpar
for p in $(pgrep -f "^$BIN" 2>/dev/null); do kill "$p" 2>/dev/null; sleep 0.3; done
rm -rf "$DATA"

SCI_DATA_DIR="$DATA" SCI_PORT=$PORT "$BIN" > /opt/data/tmp_smoke_estado.log 2>&1 &
PID=$!
trap 'kill $PID 2>/dev/null' EXIT

B="http://127.0.0.1:$PORT"
for i in $(seq 1 60); do curl -s --max-time 2 -o /dev/null "$B/api/health" && break; sleep 0.5; done

AJ=/opt/data/tmp_smoke_estado_admin.jar
GJ=/opt/data/tmp_smoke_estado_ger.jar
rm -f "$AJ" "$GJ"

code() { curl -s --max-time 10 -o /dev/null -w '%{http_code}' "$@"; }
jqid() { python3 -c 'import sys,json
o=json.load(sys.stdin)
if isinstance(o,dict) and "id" in o: print(o["id"]); raise SystemExit
raise SystemExit("sem id: "+json.dumps(o))'; }

# E4 — sem sessão: mesma barreira do /hoje (401)
C=$(code "$B/api/conferencia/estado")
[ "$C" = 401 ] || { echo "E4: sem cookie esperado 401, veio $C"; exit 1; }

# --- login admin + grupo COM gerente (contrato do POST /api/grupos) ---
C=$(code -c "$AJ" -H 'Content-Type: application/json' -d '{"login":"admin","senha":"admin"}' "$B/api/login")
[ "$C" = 200 ] || { echo "login admin: $C"; exit 1; }
RESP=$(curl -sf -b "$AJ" -H 'X-SCI: 1' -H 'Content-Type: application/json' \
  -d '{"nome":"Cia Estado E2E","login":"gere6","senha":"gere6123","nome_guerra":"GER ESTADO"}' "$B/api/grupos")
GID=$(echo "$RESP" | jqid)
curl -sf -c "$GJ" -H 'Content-Type: application/json' -d '{"login":"gere6","senha":"gere6123"}' "$B/api/login" > /dev/null

# --- 2 setores + 2 pessoas (1 por setor) ---
SA=$(curl -sf -b "$GJ" -H 'X-SCI: 1' -H 'Content-Type: application/json' -d '{"nome":"Setor Alfa Estado","sigla":"SAE"}' "$B/api/catalogo/setores" | jqid)
SB=$(curl -sf -b "$GJ" -H 'X-SCI: 1' -H 'Content-Type: application/json' -d '{"nome":"Setor Bravo Estado","sigla":"SBE"}' "$B/api/catalogo/setores" | jqid)
PA=$(curl -sf -b "$GJ" -H 'X-SCI: 1' -H 'Content-Type: application/json' -d "{\"nome_guerra\":\"MIL A\",\"nome_completo\":\"Militar Alfa Estado\",\"setor_id\":$SA}" "$B/api/pessoas" | jqid)
PB=$(curl -sf -b "$GJ" -H 'X-SCI: 1' -H 'Content-Type: application/json' -d "{\"nome_guerra\":\"MIL B\",\"nome_completo\":\"Militar Bravo Estado\",\"setor_id\":$SB}" "$B/api/pessoas" | jqid)
[ -n "$PA" ] && [ -n "$PB" ] || { echo "criar pessoas falhou"; exit 1; }

# --- conferência aberta (gerente) ---
CID=$(curl -sf -b "$GJ" -H 'X-SCI: 1' -H 'Content-Type: application/json' -d '{"local":"E2E Estado"}' "$B/api/conferencia/iniciar" | jqid)

# E1 — contrato + estabilidade do hash
curl -sf -b "$GJ" "$B/api/conferencia/estado?id=$CID" | SA="$SA" SB="$SB" python3 -c '
import sys, json, os
d = json.load(sys.stdin)
assert isinstance(d.get("hash_geral"), str) and len(d["hash_geral"]) == 40, d
ids = [s["setor_id"] for s in d["setores"]]
assert set(ids) == {os.environ["SA"], os.environ["SB"]}, d
assert all(len(s["hash"]) == 40 for s in d["setores"]), d
print("E1 OK: hash_geral + 1 bloco por setor (40 hex)")'
H1=$(curl -sf -b "$GJ" "$B/api/conferencia/estado?id=$CID")
H2=$(curl -sf -b "$GJ" "$B/api/conferencia/estado?id=$CID")
[ "$H1" = "$H2" ] || { echo "E1b: hash instável com estado parado:
$H1
$H2"; exit 1; }
echo "E1b OK: hash estável em leituras repetidas (zero-mutação viável)"

# E2 — lançamento no setor A: hash A muda, hash B NÃO
curl -sf -b "$GJ" -H 'X-SCI: 1' -H 'Content-Type: application/json' \
  -d "{\"pessoa_id\":$PA,\"situacao\":\"presente\",\"verificado\":true}" "$B/api/conferencia/marcar" > /dev/null
curl -sf -b "$GJ" "$B/api/conferencia/estado?id=$CID" | H_ANTERIOR="$H1" SA="$SA" SB="$SB" python3 -c '
import sys, json, os
antes = json.loads(os.environ["H_ANTERIOR"])
agora = json.load(sys.stdin)
def bloco(d, sid): return next(s["hash"] for s in d["setores"] if s["setor_id"] == sid)
assert bloco(antes, os.environ["SA"]) != bloco(agora, os.environ["SA"]), "hash do setor A não mudou"
assert bloco(antes, os.environ["SB"]) == bloco(agora, os.environ["SB"]), "hash do setor B mudou sem motivo"
assert antes["hash_geral"] != agora["hash_geral"], "hash_geral não mudou"
print("E2 OK: setor A mudou, setor B intocado (segregação por setor)")'

# E3 — concluir o setor B: status setorial entra no hash
C=$(code -b "$GJ" -H 'X-SCI: 1' -X POST "$B/api/conferencia/$CID/setor/$SB/concluir")
[ "$C" = 200 ] || { echo "concluir setor B: $C"; exit 1; }
curl -sf -b "$GJ" "$B/api/conferencia/estado?id=$CID" | H_ANTERIOR="$H1" SB="$SB" python3 -c '
import sys, json, os
antes = json.loads(os.environ["H_ANTERIOR"])
agora = json.load(sys.stdin)
def bloco(d, sid): return next(s["hash"] for s in d["setores"] if s["setor_id"] == sid)
assert bloco(antes, os.environ["SB"]) != bloco(agora, os.environ["SB"]), "conclusão do setor B não mudou o hash"
print("E3 OK: status setorial entra no hash")'

kill "$PID" 2>/dev/null
wait "$PID" 2>/dev/null
trap - EXIT
echo "E2E CONF-ESTADO: VERDE"
