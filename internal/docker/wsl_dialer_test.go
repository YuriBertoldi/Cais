package docker

import (
	"context"
	"io"
	"os/exec"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestWSLDialerDescricao(t *testing.T) {
	casos := []struct {
		distro   string
		esperado string
	}{
		{"Ubuntu-24.04", "wsl://Ubuntu-24.04"},
		{"", "wsl://(distro padrao)"},
	}

	for _, c := range casos {
		if got := (&wslDialer{distro: c.distro}).Descricao(); got != c.esperado {
			t.Errorf("distro %q: Descricao() = %q; esperado %q", c.distro, got, c.esperado)
		}
	}
}

// TestWSLDialerNaoAbreProcessoComContextCancelado: cada conexao da API e um
// processo wsl.exe. Se o context ja morreu, nao pode sobrar processo orfao.
func TestWSLDialerNaoAbreProcessoComContextCancelado(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	conn, err := (&wslDialer{distro: "Ubuntu-24.04"}).DialContext(ctx)

	if err == nil {
		conn.Close()
		t.Fatal("esperado erro com context ja cancelado")
	}
	if conn != nil {
		t.Error("nao deveria devolver conexao junto com o erro")
	}
}

// processoConnDeTeste adapta um processo qualquer, sem envolver o wsl.exe: o
// que esta sob teste e a adaptacao de stdin/stdout a net.Conn.
func processoConnDeTeste(t *testing.T, nome string, args ...string) *processoConn {
	t.Helper()

	cmd := exec.Command(nome, args...)
	entrada, err := cmd.StdinPipe()
	if err != nil {
		t.Fatalf("StdinPipe: %v", err)
	}
	saida, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatalf("StdoutPipe: %v", err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	return &processoConn{cmd: cmd, entrada: entrada, saida: saida}
}

func TestProcessoConnLeDoStdoutDoProcesso(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("usa cmd.exe como processo de apoio")
	}
	conn := processoConnDeTeste(t, "cmd", "/c", "echo alo")

	b, err := io.ReadAll(conn)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if got := strings.TrimSpace(string(b)); got != "alo" {
		t.Errorf("leitura = %q; esperado \"alo\"", got)
	}

	if err := conn.Close(); err != nil {
		t.Errorf("Close: %v", err)
	}
}

// TestProcessoConnCloseEIdempotente: o http.Transport fecha a conexao mais de
// uma vez em alguns caminhos de erro, e o segundo Close nao pode entrar em
// panico sobre um processo ja colhido.
func TestProcessoConnCloseEIdempotente(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("usa cmd.exe como processo de apoio")
	}
	conn := processoConnDeTeste(t, "cmd", "/c", "echo alo")

	for i := range 3 {
		if err := conn.Close(); err != nil {
			t.Errorf("Close na %da vez: %v", i+1, err)
		}
	}
}

// TestProcessoConnCumpreOContratoDeNetConn: o http.Transport chama SetDeadline
// nas conexoes que recebe. Devolver erro ali derrubaria toda chamada da API.
func TestProcessoConnCumpreOContratoDeNetConn(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("usa cmd.exe como processo de apoio")
	}
	conn := processoConnDeTeste(t, "cmd", "/c", "echo alo")
	defer conn.Close()

	prazo := time.Now().Add(time.Minute)
	if err := conn.SetDeadline(prazo); err != nil {
		t.Errorf("SetDeadline: %v", err)
	}
	if err := conn.SetReadDeadline(prazo); err != nil {
		t.Errorf("SetReadDeadline: %v", err)
	}
	if err := conn.SetWriteDeadline(prazo); err != nil {
		t.Errorf("SetWriteDeadline: %v", err)
	}

	if got := conn.LocalAddr().Network(); got != "wsl" {
		t.Errorf("LocalAddr().Network() = %q; esperado wsl", got)
	}
	if got := conn.RemoteAddr().String(); got != "wsl-dial-stdio" {
		t.Errorf("RemoteAddr().String() = %q; esperado wsl-dial-stdio", got)
	}
}
