// Package config resolve a configuracao de inicializacao do painel:
// flags de linha de comando, caminho do banco local e porta de escuta.
package config

import (
	"flag"
	"os"
	"path/filepath"
)

// PortaPadrao e a porta fixa tentada antes de cair para porta livre dinamica.
const PortaPadrao = 45678

// NomeApp e usado para montar o diretorio de dados em %LOCALAPPDATA%.
const NomeApp = "Cais"

// Config reune tudo que o main precisa para montar as dependencias.
type Config struct {
	// Dev faz os templates e estaticos serem lidos do disco em vez do binario,
	// com reparse a cada request.
	Dev bool
	// Porta preferida. 0 significa "qualquer porta livre".
	Porta int
	// AbrirNavegador controla se o navegador padrao e aberto no startup.
	AbrirNavegador bool
	// DockerHost e o endpoint da Engine API.
	DockerHost string
	// WSLDistro e a distro usada para executar o docker compose.
	WSLDistro string
	// CaminhoDB e o arquivo SQLite de estado local.
	CaminhoDB string
}

// Load le as flags e devolve a configuracao resolvida.
func Load() Config {
	var c Config
	flag.BoolVar(&c.Dev, "dev", false, "le templates/estaticos do disco e reparseia a cada request")
	flag.IntVar(&c.Porta, "porta", PortaPadrao, "porta de escuta (0 = qualquer porta livre)")
	navegador := flag.Bool("no-browser", false, "nao abrir o navegador no startup")
	flag.StringVar(&c.DockerHost, "docker-host", "tcp://127.0.0.1:2375", "endpoint da Engine API: tcp://host:porta ou wsl://distro (dial-stdio, sem porta TCP)")
	flag.StringVar(&c.WSLDistro, "wsl-distro", "Ubuntu-24.04", "distro WSL usada para executar o docker compose")
	flag.StringVar(&c.CaminhoDB, "db", "", "caminho do SQLite (padrao: %LOCALAPPDATA%\\Cais\\painel.db)")
	flag.Parse()

	c.AbrirNavegador = !*navegador
	if c.CaminhoDB == "" {
		c.CaminhoDB = CaminhoDBPadrao()
	}
	return c
}

// DirDados devolve o caminho do diretorio de dados do painel. So monta a
// string: quem cria o diretorio e o store.Open, no caminho do banco.
//
// Usa LOCALAPPDATA de proposito, nunca Roaming: em perfil de dominio o .db e os
// arquivos -wal/-shm do SQLite seriam sincronizados no logoff, o que corrompe o
// banco e deixa o logoff lento.
func DirDados() string {
	base := os.Getenv("LOCALAPPDATA")
	if base == "" {
		var err error
		if base, err = os.UserCacheDir(); err != nil {
			base = os.TempDir()
		}
	}
	return filepath.Join(base, NomeApp)
}

// CaminhoDBPadrao devolve o caminho do SQLite no diretorio de dados.
func CaminhoDBPadrao() string {
	return filepath.Join(DirDados(), "painel.db")
}
