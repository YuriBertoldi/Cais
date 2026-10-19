# Roda localmente os mesmos gates do pipeline (.github/workflows/ci.yml).
#
# Uso:
#   .\scripts\validar.ps1
#   .\scripts\validar.ps1 -Cobertura   # abre o relatorio de cobertura no navegador
#
# A ideia e nunca descobrir no CI algo que daria para ver aqui em um minuto.
[CmdletBinding()]
param(
    [switch]$Cobertura
)

$ErrorActionPreference = "Stop"
$raiz = Split-Path -Parent $PSScriptRoot
Push-Location $raiz

$falhas = @()

function Etapa {
    param([string]$Nome, [scriptblock]$Acao)

    Write-Host ""
    Write-Host "== $Nome" -ForegroundColor Cyan
    try {
        & $Acao
        Write-Host "   OK" -ForegroundColor Green
    }
    catch {
        Write-Host "   FALHOU: $($_.Exception.Message)" -ForegroundColor Red
        $script:falhas += $Nome
    }
}

try {
    Etapa "Formatacao (gofmt)" {
        $foraDoPadrao = gofmt -l .
        if ($foraDoPadrao) {
            $foraDoPadrao | ForEach-Object { Write-Host "   $_" -ForegroundColor Yellow }
            throw "rode: gofmt -w ."
        }
    }

    Etapa "Dependencias (go mod tidy)" {
        & go mod tidy
        if ($LASTEXITCODE -ne 0) { throw "go mod tidy falhou" }
        $pendentes = git status --porcelain go.mod go.sum
        if ($pendentes) { throw "go.mod/go.sum mudaram; revise e comite o resultado" }
    }

    Etapa "Analise estatica (go vet)" {
        & go vet ./...
        if ($LASTEXITCODE -ne 0) { throw "go vet apontou problemas" }
    }

    # As aspas em "-flag=valor" nao sao decorativas: sem elas o PowerShell
    # quebra o argumento no "=" e o go recebe dois tokens.
    Etapa "Testes" {
        & go test ./... -count=1 "-covermode=atomic" "-coverprofile=cobertura.out"
        if ($LASTEXITCODE -ne 0) { throw "testes falharam" }
    }

    Etapa "Cobertura total" {
        $linhas = & go tool cover "-func=cobertura.out"
        if ($LASTEXITCODE -ne 0) { throw "nao foi possivel ler a cobertura" }
        Write-Host "   $($linhas | Select-Object -Last 1)"
    }

    # O detector de corrida precisa de CGO e de um compilador C. Em maquina de
    # dev sem mingw isso simplesmente nao esta disponivel -- o CI cobre.
    if (Get-Command gcc -ErrorAction SilentlyContinue) {
        Etapa "Detector de corrida" {
            $env:CGO_ENABLED = "1"
            try {
                & go test ./... -count=1 -race
                if ($LASTEXITCODE -ne 0) { throw "detector de corrida acusou problema" }
            }
            finally {
                Remove-Item Env:\CGO_ENABLED -ErrorAction SilentlyContinue
            }
        }
    }
    else {
        Write-Host ""
        Write-Host "== Detector de corrida" -ForegroundColor Cyan
        Write-Host "   PULADO: gcc nao encontrado (o CI roda esta etapa)." -ForegroundColor DarkGray
    }

    Etapa "Compilacao" {
        & go build -o "$env:TEMP\cais-validacao.exe" .
        if ($LASTEXITCODE -ne 0) { throw "go build falhou" }
        Remove-Item "$env:TEMP\cais-validacao.exe" -ErrorAction SilentlyContinue
    }

    if ($Cobertura) {
        & go tool cover -html=cobertura.out
    }

    Write-Host ""
    if ($falhas.Count -gt 0) {
        Write-Host "Validacao reprovada em: $($falhas -join ', ')" -ForegroundColor Red
        exit 1
    }
    Write-Host "Validacao completa: tudo passou." -ForegroundColor Green
}
finally {
    Pop-Location
}
