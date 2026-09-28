#!/bin/bash
# E2E do SCI em produção — roda DENTRO do host, via curl. Códigos HTTP por passo.
B="http://127.0.0.1:10003"
CJ=$(mktemp)
code() { curl -s -o /dev/null -w '%{http_code}' "$@"; }
H='Content-Type: application/json'
X='X-SCI: 1'
echo "login:      $(code -c "$CJ" -X POST $B/api/login -H "$H" -H "$X" -d '{"login":"admin","senha":"C3f@Sci2026!"}')"
echo "setor:      $(code -b "$CJ" -X POST $B/api/catalogo/setores -H "$H" -H "$X" -d '{"nome":"E2E"}')"
echo "pessoa:     $(code -b "$CJ" -X POST $B/api/pessoas -H "$H" -H "$X" -d '{"nome_guerra":"TESTE","nome_completo":"Teste E2E","status":"ativo"}')"
echo "abrir:      $(code -b "$CJ" -X POST $B/api/formatura/abrir -H "$H" -H "$X" -d '{}')"
echo "confirmar:  $(code -b "$CJ" -X POST $B/api/formatura/confirmar -H "$H" -H "$X" -d '{"lancamentos":[{"pessoa_id":1,"situacao":"presente"}]}')"
echo "semanal:    $(code -b "$CJ" "$B/api/presenca/periodo")"
echo "pdf:        $(code -b "$CJ" "$B/api/relatorio.pdf")"
echo "csv:        $(code -b "$CJ" "$B/api/export/pessoas")"
echo "backup:     $(code -b "$CJ" -X POST $B/api/backup -H "$H" -H "$X" -d '{}')"
echo "guarda401:  $(code "$B/api/formatura/hoje")"
echo "raiz_html:  $(code "$B/")"
rm -f "$CJ"
