# SCI — Pipeline de Build de Produção e Ofuscação (Windows PowerShell)
# Remove símbolos de depuração (DWARF), caminhos locais e assinaturas de build.

$ErrorActionPreference = "Stop"

Write-Host "==> Iniciando pipeline de build de produção SCI..." -ForegroundColor Cyan

$goCmd = Get-Command go -ErrorAction SilentlyContinue
if (-not $goCmd) {
    $tempGo = "C:\Users\hermes\AppData\Local\Temp\go_install\go\bin\go.exe"
    if (Test-Path $tempGo) {
        $goBin = $tempGo
    } else {
        throw "Compilador Go não encontrado no PATH nem no Temp"
    }
} else {
    $goBin = "go"
}

$binDir = ".\bin"
if (-not (Test-Path $binDir)) {
    New-Item -ItemType Directory -Path $binDir | Out-Null
}

$ldflags = "-s -w -buildid="

Write-Host "==> Compilando binário sci.exe com ofuscação de símbolos..." -ForegroundColor Yellow
& $goBin build -trimpath -ldflags="$ldflags" -o "$binDir\sci_prod.exe" .

if (Test-Path "$binDir\sci_prod.exe") {
    $item = Get-Item "$binDir\sci_prod.exe"
    $tamanhoMb = [math]::Round($item.Length / 1MB, 2)
    Write-Host "==> Binário de produção gerado com sucesso: $binDir\sci_prod.exe ($tamanhoMb MB)" -ForegroundColor Green
}

# Também atualiza o sci.exe na raiz para uso imediato
& $goBin build -trimpath -ldflags="$ldflags" -o ".\sci.exe" .
Write-Host "==> Executável sci.exe principal atualizado na raiz com sucesso." -ForegroundColor Green
