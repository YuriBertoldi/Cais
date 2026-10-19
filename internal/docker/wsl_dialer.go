package docker

import (
	"context"
	"fmt"
	"io"
	"net"
	"os/exec"
	"sync"
	"time"
)

// wslDialer conversa com o daemon pelo `docker system dial-stdio` dentro de uma
// distro WSL, o mesmo transporte que o Docker usa em ssh://. Nao abre porta
// TCP no daemon: o acesso fica restrito a quem consegue executar o wsl.exe.
type wslDialer struct{ distro string }

func (d *wslDialer) Descricao() string {
	if d.distro == "" {
		return "wsl://(distro padrao)"
	}
	return "wsl://" + d.distro
}

func (d *wslDialer) DialContext(ctx context.Context) (net.Conn, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	var args []string
	if d.distro != "" {
		args = append(args, "-d", d.distro)
	}
	// --exec dispensa o shell da distro: nenhum argumento e reinterpretado.
	args = append(args, "--exec", "docker", "system", "dial-stdio")

	// Sem CommandContext de proposito: o ctx so vale para a abertura, e a
	// conexao precisa sobreviver a ele (logs ao vivo).
	cmd := exec.Command("wsl.exe", args...)
	entrada, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	saida, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("iniciar wsl.exe: %w", err)
	}
	return &processoConn{cmd: cmd, entrada: entrada, saida: saida}, nil
}

// processoConn adapta stdin/stdout de um processo a net.Conn.
type processoConn struct {
	cmd     *exec.Cmd
	entrada io.WriteCloser
	saida   io.ReadCloser

	fechar sync.Once
}

func (c *processoConn) Read(p []byte) (int, error)  { return c.saida.Read(p) }
func (c *processoConn) Write(p []byte) (int, error) { return c.entrada.Write(p) }

func (c *processoConn) Close() error {
	c.fechar.Do(func() {
		c.entrada.Close()
		// dial-stdio encerra sozinho ao fechar o stdin; o Kill cobre o caso de travar.
		feito := make(chan struct{})
		go func() {
			c.cmd.Wait()
			close(feito)
		}()
		select {
		case <-feito:
		case <-time.After(3 * time.Second):
			c.cmd.Process.Kill()
			<-feito
		}
	})
	return nil
}

type enderecoWSL struct{}

func (enderecoWSL) Network() string { return "wsl" }
func (enderecoWSL) String() string  { return "wsl-dial-stdio" }

func (c *processoConn) LocalAddr() net.Addr                { return enderecoWSL{} }
func (c *processoConn) RemoteAddr() net.Addr               { return enderecoWSL{} }
func (c *processoConn) SetDeadline(t time.Time) error      { return nil }
func (c *processoConn) SetReadDeadline(t time.Time) error  { return nil }
func (c *processoConn) SetWriteDeadline(t time.Time) error { return nil }
