package config

import (
	"flag"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestDirDadosUsaLocalAppData garante a escolha deliberada por LOCALAPPDATA:
// em perfil de dominio, Roaming sincronizaria o .db e os arquivos -wal/-shm
// no logoff, corrompendo o banco.
func TestDirDadosUsaLocalAppData(t *testing.T) {
	base := t.TempDir()
	t.Setenv("LOCALAPPDATA", base)

	got := DirDados()

	if esperado := filepath.Join(base, NomeApp); got != esperado {
		t.Errorf("DirDados() = %q; esperado %q", got, esperado)
	}
}

// TestDirDadosSemLocalAppData cobre o fallback: sem a variavel, qualquer
// diretorio serve desde que o nome do app esteja no fim.
func TestDirDadosSemLocalAppData(t *testing.T) {
	t.Setenv("LOCALAPPDATA", "")

	got := DirDados()

	if got == "" {
		t.Fatal("DirDados() devolveu vazio sem LOCALAPPDATA")
	}
	if filepath.Base(got) != NomeApp {
		t.Errorf("DirDados() = %q; esperado terminar em %q", got, NomeApp)
	}
	if !filepath.IsAbs(got) {
		t.Errorf("DirDados() = %q; esperado caminho absoluto", got)
	}
}

func TestCaminhoDBPadrao(t *testing.T) {
	base := t.TempDir()
	t.Setenv("LOCALAPPDATA", base)

	got := CaminhoDBPadrao()

	esperado := filepath.Join(base, NomeApp, "painel.db")
	if got != esperado {
		t.Errorf("CaminhoDBPadrao() = %q; esperado %q", got, esperado)
	}
	if !strings.HasPrefix(got, DirDados()) {
		t.Errorf("CaminhoDBPadrao() = %q; esperado estar dentro de %q", got, DirDados())
	}
}

// comLinhaDeComando troca os argumentos do processo e zera o conjunto global
// de flags, para que Load() possa ser exercitado como no startup real.
func comLinhaDeComando(t *testing.T, args ...string) {
	t.Helper()

	argsOriginais := os.Args
	flagsOriginais := flag.CommandLine

	os.Args = append([]string{"cais.exe"}, args...)
	flag.CommandLine = flag.NewFlagSet("cais.exe", flag.ContinueOnError)
	flag.CommandLine.SetOutput(io.Discard)

	t.Cleanup(func() {
		os.Args = argsOriginais
		flag.CommandLine = flagsOriginais
	})
}

func TestLoadPadroes(t *testing.T) {
	base := t.TempDir()
	t.Setenv("LOCALAPPDATA", base)
	comLinhaDeComando(t)

	c := Load()

	if c.Dev {
		t.Error("Dev = true; esperado false por padrao")
	}
	if c.Porta != PortaPadrao {
		t.Errorf("Porta = %d; esperado %d", c.Porta, PortaPadrao)
	}
	if !c.AbrirNavegador {
		t.Error("AbrirNavegador = false; o padrao e abrir o navegador no startup")
	}
	if c.DockerHost != "tcp://127.0.0.1:2375" {
		t.Errorf("DockerHost = %q", c.DockerHost)
	}
	if c.WSLDistro != "Ubuntu-24.04" {
		t.Errorf("WSLDistro = %q", c.WSLDistro)
	}
	if c.CaminhoDB != filepath.Join(base, NomeApp, "painel.db") {
		t.Errorf("CaminhoDB = %q; esperado o caminho padrao em LOCALAPPDATA", c.CaminhoDB)
	}
}

func TestLoadComFlags(t *testing.T) {
	t.Setenv("LOCALAPPDATA", t.TempDir())
	comLinhaDeComando(t,
		"-dev",
		"-porta", "0",
		"-no-browser",
		"-docker-host", "tcp://10.0.0.1:2375",
		"-wsl-distro", "Debian",
		"-db", `C:\tmp\outro.db`,
	)

	c := Load()

	if !c.Dev {
		t.Error("Dev = false; esperado true com -dev")
	}
	if c.Porta != 0 {
		t.Errorf("Porta = %d; esperado 0 (qualquer porta livre)", c.Porta)
	}
	if c.AbrirNavegador {
		t.Error("AbrirNavegador = true; -no-browser deveria desligar")
	}
	if c.DockerHost != "tcp://10.0.0.1:2375" {
		t.Errorf("DockerHost = %q", c.DockerHost)
	}
	if c.WSLDistro != "Debian" {
		t.Errorf("WSLDistro = %q", c.WSLDistro)
	}
	if c.CaminhoDB != `C:\tmp\outro.db` {
		t.Errorf("CaminhoDB = %q; o -db informado deveria vencer o padrao", c.CaminhoDB)
	}
}

func TestPortaPadrao(t *testing.T) {
	// A porta fixa e o que permite detectar instancia ja aberta antes de
	// subir a segunda. Mudar o valor quebra essa deteccao em quem ja tem o
	// painel aberto, entao a constante fica travada por teste.
	if PortaPadrao != 45678 {
		t.Errorf("PortaPadrao = %d; esperado 45678", PortaPadrao)
	}
}
