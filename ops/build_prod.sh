#!/usr/bin/env bash
# SCI — Pipeline de Build de Produção e Ofuscação (Vendor Lock & Segurança)
# Remove símbolos de depuração (DWARF), caminhos locais e assinaturas de build.
set -euo pipefail

echo "==> Iniciando pipeline de build de produção SCI..."

BIN_DIR="./bin"
mkdir -p "$BIN_DIR"

# Flags de ofuscação do linker Go:
# -s: Omit symbol table
# -w: Omit DWARF debugging information
# -buildid=: Remove deterministic build ID
# -trimpath: Remove todos os caminhos do filesystem do host
LDFLAGS="-s -w -buildid="

echo "==> Compilando binário nativo com ofuscação de símbolos..."
go build -trimpath -ldflags="$LDFLAGS" -o "$BIN_DIR/sci_prod" .

if [ -f "$BIN_DIR/sci_prod" ]; then
    echo "==> Binário gerado com sucesso: $BIN_DIR/sci_prod"
    ls -lh "$BIN_DIR/sci_prod"
fi

echo "==> Concluído com sucesso!"
