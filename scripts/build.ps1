# Compila o cais como executavel unico para Windows.
#
# Uso:
#   .\scripts\build.ps1
#   .\scripts\build.ps1 -Versao 0.2.0
[CmdletBinding()]
param(
    [string]$Versao = "dev",
    [string]$Saida  = "cais.exe"
)

$ErrorActionPreference = "Stop"
$raiz = Split-Path -Parent $PSScriptRoot
Push-Location $raiz

try {
    Write-Host "Compilando cais $Versao ..." -ForegroundColor Cyan

    # -trimpath remove caminhos absolutos da maquina do binario.
    # -s -w descartam tabela de simbolos e DWARF: o .exe fica bem menor e
    # nada aqui depende de stack trace com numero de linha em producao.
    $ldflags = "-s -w -X main.versao=$Versao"
    & go build -trimpath -ldflags $ldflags -o $Saida .
    if ($LASTEXITCODE -ne 0) { throw "go build falhou com codigo $LASTEXITCODE" }

    $mb = [math]::Round((Get-Item $Saida).Length / 1MB, 1)
    Write-Host "OK: $Saida ($mb MB)" -ForegroundColor Green
    Write-Host ""
    Write-Host "O executavel e autossuficiente: templates e estaticos estao embutidos." -ForegroundColor DarkGray
    Write-Host "Para distribuir, basta copiar o .exe." -ForegroundColor DarkGray
}
finally {
    Pop-Location
}
