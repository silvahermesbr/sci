#!/usr/bin/env bash
# E2E — onda Escalas (05/10): painel ESCALA da conferência + nomeação de chefe.
# Cenário: admin provisiona contas e vincula grupo → gerente cria setores e
# NOMEIA chefe de setor → gerente designa chefe+operador na escala da
# conferência → login do CHEFE → designa operador do SEU setor (200), recebe
# 403 em operador de outro setor, remove designação (200; repetir 404).
# Contratos provados: /api/usuarios expõe setor_id (filtro do chefe no front),
# GET /api/conferencia/{id} devolve "escala" com nome_guerra/papel.
# Knobs: SCI_PORT=14215 · SCI_DATA_DIR=/opt/data/tmp_smoke_esc · binário em
# diretório PRÓPRIO (nunca rm -rf do dir que contém o servidor em teste).
set -u
PORT=${SCI_PORT:-14215}
DATA=${SCI_DATA_DIR:-/opt/data/tmp_smoke_esc}
BIN=${SCI_BIN:-/opt/data/tmp_smoke_esc_bin/sci_ci}
REPO=$(cd "$(dirname "$0")/.." && pwd)

if [ ! -x "$BIN" ]; then
  mkdir -p "$(dirname "$BIN")"
  (cd "$REPO" && CGO_ENABLED=0 go build -o "$BIN" .) || { echo "FALHA no build"; exit 1; }
fi

# ghost de smoke anterior: matar pelo PID do MESMO binário antes de limpar
# (^ âncora: o cmdline do servidor É o caminho; sem ela o pgrep casa o próprio
# shell invocador — que carrega o caminho em SCI_BIN — e o script se mata)
for p in $(pgrep -f "^$BIN" 2>/dev/null); do kill "$p" 2>/dev/null; sleep 0.3; done
rm -rf "$DATA"

SCI_DATA_DIR="$DATA" SCI_PORT=$PORT "$BIN" > /opt/data/tmp_smoke_esc.log 2>&1 &
PID=$!
trap 'kill $PID 2>/dev/null' EXIT

B="http://127.0.0.1:$PORT"
for i in $(seq 1 60); do curl -s --max-time 2 -o /dev/null "$B/api/health" && break; sleep 0.5; done

AJ=/opt/data/tmp_smoke_esc_admin.jar
GJ=/opt/data/tmp_smoke_esc_ger.jar
CJ=/opt/data/tmp_smoke_esc_chefe.jar
rm -f "$AJ" "$GJ" "$CJ"

code() { curl -s --max-time 10 -o /dev/null -w '%{http_code}' "$@"; }
jqid() { python3 -c 'import sys,json
o=json.load(sys.stdin)
if isinstance(o,dict) and "id" in o: print(o["id"]); raise SystemExit
raise SystemExit("sem id: "+json.dumps(o))'; }

# --- login admin (seed: admin/admin) ---
C=$(code -c "$AJ" -H 'Content-Type: application/json' -d '{"login":"admin","senha":"admin"}' "$B/api/login")
[ "$C" = 200 ] || { echo "login admin: $C"; exit 1; }

# --- grupo nasce COM gerente (contrato do POST /api/grupos) ---
RESP=$(curl -sf -b "$AJ" -H 'X-SCI: 1' -H 'Content-Type: application/json' \
  -d '{"nome":"Cia Escalas E2E","login":"gere2e","senha":"gere2e123","nome_guerra":"GER E2E"}' "$B/api/grupos")
GID=$(echo "$RESP" | jqid)
curl -sf -c "$GJ" -H 'Content-Type: application/json' -d '{"login":"gere2e","senha":"gere2e123"}' "$B/api/login" > /dev/null

# --- gerente cria o CHEFE (fluxo real 04/10: linha chefe_setor nasce COM grupo;
#     conta criada pelo admin nasce sem grupo e o login pega a 1ª linha de
#     usuario_papeis — chefe sem escopo = 403, artefato de seed, não de produto) ---
UCH=$(curl -sf -b "$GJ" -H 'X-SCI: 1' -H 'Content-Type: application/json' -d '{"login":"chefesc_e2e","senha":"senha123","papel":"chefe_setor"}' "$B/api/usuarios" | jqid)

# --- gerente: setores do catálogo + NOMEIA chefe (setor Alfa) ---
SA=$(curl -sf -b "$GJ" -H 'X-SCI: 1' -H 'Content-Type: application/json' -d '{"nome":"Setor Alfa E2E","sigla":"SAE"}' "$B/api/catalogo/setores" | jqid)
SB=$(curl -sf -b "$GJ" -H 'X-SCI: 1' -H 'Content-Type: application/json' -d '{"nome":"Setor Bravo E2E","sigla":"SBE"}' "$B/api/catalogo/setores" | jqid)
C=$(code -b "$GJ" -H 'X-SCI: 1' -H 'Content-Type: application/json' -d "{\"usuario_id\":$UCH,\"setor_id\":$SA}" "$B/api/grupos/$GID/nomear_chefe")
[ "$C" = 200 ] || { echo "nomear_chefe: $C"; exit 1; }

# --- chefe designa os OPERADORES do seu setor (herdam setor Alfa) ---
curl -sf -c "$CJ" -H 'Content-Type: application/json' -d '{"login":"chefesc_e2e","senha":"senha123"}' "$B/api/login" > /dev/null
UOPA=$(curl -sf -b "$CJ" -H 'X-SCI: 1' -H 'Content-Type: application/json' -d '{"login":"opaesc_e2e","senha":"senha123","papel":"operador"}' "$B/api/usuarios" | jqid)
UOPC=$(curl -sf -b "$CJ" -H 'X-SCI: 1' -H 'Content-Type: application/json' -d '{"login":"opcesc_e2e","senha":"senha123","papel":"operador"}' "$B/api/usuarios" | jqid)
UOPB=$(curl -sf -b "$CJ" -H 'X-SCI: 1' -H 'Content-Type: application/json' -d '{"login":"opbesc_e2e","senha":"senha123","papel":"operador"}' "$B/api/usuarios" | jqid)

# --- nomes de guerra (POST não aceita nome_guerra; PATCH por cima) + Op Bravo → setor Bravo ---
ng() { # ng <id> <nome>
  C=$(code -b "$GJ" -H 'X-SCI: 1' -H 'Content-Type: application/json' -d "{\"nome_guerra\":\"$2\"}" -X PATCH "$B/api/usuarios/$1")
  [ "$C" = 200 ] || { echo "nome_guerra ($1): $C"; exit 1; }
}
ng "$UCH" "Chefe Alfa"; ng "$UOPA" "Op Alfa"; ng "$UOPB" "Op Bravo"; ng "$UOPC" "Op Charlie"
C=$(code -b "$GJ" -H 'X-SCI: 1' -H 'Content-Type: application/json' -d "{\"setor_id\":$SB}" -X PATCH "$B/api/usuarios/$UOPB")
[ "$C" = 200 ] || { echo "setor do opB: $C"; exit 1; }

# --- conferência aberta + gerente designa CHEFE e OPERADOR na escala ---
CID=$(curl -sf -b "$GJ" -H 'X-SCI: 1' -H 'Content-Type: application/json' -d '{"local":"E2E Escalas"}' "$B/api/conferencia/iniciar" | jqid)
C=$(code -b "$GJ" -H 'X-SCI: 1' -H 'Content-Type: application/json' -d "{\"usuario_id\":$UCH,\"papel_na_escala\":\"chefe\"}" "$B/api/conferencia/$CID/escala")
[ "$C" = 200 ] || { echo "ger designa chefe: $C"; exit 1; }
C=$(code -b "$GJ" -H 'X-SCI: 1' -H 'Content-Type: application/json' -d "{\"usuario_id\":$UOPA,\"papel_na_escala\":\"operador\"}" "$B/api/conferencia/$CID/escala")
[ "$C" = 200 ] || { echo "ger designa operador: $C"; exit 1; }

# GET da conferência devolve a escala com nome_guerra/papel (ordem: chefe 1º)
curl -sf -b "$GJ" "$B/api/conferencia/$CID" | UCH="$UCH" UOPA="$UOPA" python3 -c '
import sys, json, os
esc = json.load(sys.stdin)["escala"]
assert len(esc) == 2, esc
assert esc[0]["usuario_id"] == int(os.environ["UCH"]) and esc[0]["papel_na_escala"] == "chefe", esc
assert esc[1]["usuario_id"] == int(os.environ["UOPA"]) and esc[1]["papel_na_escala"] == "operador", esc
assert esc[0]["nome_guerra"] == "Chefe Alfa", esc
print("escala no GET OK:", [(e["nome_guerra"], e["papel_na_escala"]) for e in esc])'

# --- login do CHEFE; filtro de candidatos provando setor_id exposto ---
curl -sf -c "$CJ" -H 'Content-Type: application/json' -d '{"login":"chefesc_e2e","senha":"senha123"}' "$B/api/login" > /dev/null
curl -sf -b "$CJ" "$B/api/usuarios" | SA="$SA" SB="$SB" python3 -c '
import sys, json, os
SA, SB = int(os.environ["SA"]), int(os.environ["SB"])
contas = {c["login"]: c for c in json.load(sys.stdin)}
assert contas["chefesc_e2e"]["setor_id"] == SA, "setor_id do chefe não exposto: " + json.dumps(contas["chefesc_e2e"])
meu = [l for l, c in contas.items() if c.get("setor_id") == SA and c["ativo"]]
assert set(meu) == {"chefesc_e2e", "opaesc_e2e", "opcesc_e2e"}, meu
assert contas["opbesc_e2e"]["setor_id"] == SB, "opB fora do setor Bravo"
print("candidatos do chefe (setor Alfa) OK:", meu)'

# chefe designa operador do SEU setor → 200; operador de OUTRO setor → 403
C=$(code -b "$CJ" -H 'X-SCI: 1' -H 'Content-Type: application/json' -d "{\"usuario_id\":$UOPC,\"papel_na_escala\":\"operador\"}" "$B/api/conferencia/$CID/escala")
[ "$C" = 200 ] || { echo "chefe designa operador do setor: $C"; exit 1; }
C=$(code -b "$CJ" -H 'X-SCI: 1' -H 'Content-Type: application/json' -d "{\"usuario_id\":$UOPB,\"papel_na_escala\":\"operador\"}" "$B/api/conferencia/$CID/escala")
[ "$C" = 403 ] || { echo "chefe designa operador de outro setor: esperado 403, veio $C"; exit 1; }

# chefe REMOVE designação (botão remover do painel) → 200; repetir → 404
C=$(code -b "$CJ" -H 'X-SCI: 1' -X DELETE "$B/api/conferencia/$CID/escala/$UOPC")
[ "$C" = 200 ] || { echo "chefe remove designação: $C"; curl -s -b "$CJ" -H 'X-SCI: 1' -X DELETE "$B/api/conferencia/$CID/escala/$UOPC"; echo; exit 1; }
C=$(code -b "$CJ" -H 'X-SCI: 1' -X DELETE "$B/api/conferencia/$CID/escala/$UOPC")
[ "$C" = 404 ] || { echo "remoção repetida: esperado 404, veio $C"; exit 1; }

curl -sf -b "$CJ" "$B/api/conferencia/$CID" | python3 -c '
import sys, json
esc = json.load(sys.stdin)["escala"]
assert len(esc) == 2, esc
print("escala final OK (2 designações após add+del)")'

kill "$PID" 2>/dev/null
wait "$PID" 2>/dev/null
trap - EXIT
echo "E2E ESCALAS: VERDE"
