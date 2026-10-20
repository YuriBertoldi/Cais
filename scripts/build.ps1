# Compila o cais como executavel unico.
#
# Uso:
#   .\scripts\build.ps1
#   .\scripts\build.ps1 -Versao 0.2.0
#   .\scripts\build.ps1 -Plataforma linux
#   .\scripts\build.ps1 -Plataforma ambos -Versao 0.2.0
[CmdletBinding()]
param(
    [string]$Versao = "dev",

    [ValidateSet("windows", "linux", "ambos")]
    [string]$Plataforma = "windows",

    [ValidateSet("amd64", "arm64")]
    [string]$Arquitetura = "amd64",

    # Vazio usa o nome padrao de cada alvo. So faz sentido com uma plataforma.
    [string]$Saida = ""
)

$ErrorActionPreference = "Stop"
$raiz = Split-Path -Parent $PSScriptRoot
Push-Location $raiz

function Compilar {
    param([string]$SO, [string]$Arch, [string]$Destino)

    # -trimpath remove caminhos absolutos da maquina do binario.
    # -s -w descartam tabela de simbolos e DWARF: o binario fica bem menor e
    # nada aqui depende de stack trace com numero de linha em producao.
    $ldflags = "-s -w -X main.versao=$Versao"

    if ($SO -eq "windows") {
        # -H=windowsgui tira o subsistema de console: sem ele, o duplo-clique
        # deixa uma janela preta aberta durante toda a vida do painel. O preco
        # e que stdout nao vai para lugar nenhum -- por isso o painel tambem
        # grava cais.log ao lado do banco, e erros de startup viram caixa de
        # mensagem (falha_windows.go).
        $ldflags += " -H=windowsgui"
    }

    Write-Host "Compilando cais $Versao para $SO/$Arch ..." -ForegroundColor Cyan

    $env:GOOS = $SO
    $env:GOARCH = $Arch
    try {
        & go build -trimpath -ldflags $ldflags -o $Destino .
        if ($LASTEXITCODE -ne 0) { throw "go build falhou com codigo $LASTEXITCODE" }
    }
    finally {
        Remove-Item Env:\GOOS, Env:\GOARCH -ErrorAction SilentlyContinue
    }

    $mb = [math]::Round((Get-Item $Destino).Length / 1MB, 1)
    Write-Host "OK: $Destino ($mb MB)" -ForegroundColor Green
}

try {
    if ($Saida -and $Plataforma -eq "ambos") {
        throw "-Saida nao vale com -Plataforma ambos: cada alvo tem o proprio nome"
    }

    # Objetos nomeados em vez de pares posicionais: com array de array, o
    # PowerShell desfaz um nivel de aninhamento no foreach e o indice passa a
    # devolver a coisa errada, em silencio.
    $alvos = @()
    if ($Plataforma -in @("windows", "ambos")) {
        $alvos += [pscustomobject]@{ SO = "windows"; Destino = "cais.exe" }
    }
    if ($Plataforma -in @("linux", "ambos")) {
        $alvos += [pscustomobject]@{ SO = "linux"; Destino = "cais-linux-$Arquitetura" }
    }

    foreach ($alvo in $alvos) {
        $destino = if ($Saida) { $Saida } else { $alvo.Destino }
        Compilar -SO $alvo.SO -Arch $Arquitetura -Destino $destino
    }

    Write-Host ""
    Write-Host "O executavel e autossuficiente: templates e estaticos estao embutidos." -ForegroundColor DarkGray
    Write-Host "Para distribuir, basta copiar o arquivo." -ForegroundColor DarkGray

    if ($Plataforma -ne "windows") {
        Write-Host ""
        Write-Host "No Linux o painel fala com o daemon pelo socket Unix" -ForegroundColor DarkGray
        Write-Host "(unix:///var/run/docker.sock, o padrao) e roda o compose direto," -ForegroundColor DarkGray
        Write-Host "sem WSL. Lembre do bit de execucao: chmod +x" -ForegroundColor DarkGray
    }
}
finally {
    Pop-Location
}
