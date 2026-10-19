// Package docker fala com a Engine API do Docker por HTTP, sem o SDK oficial.
//
// A escolha e deliberada: o SDK traz cerca de 60 modulos transitivos e 20-25 MB
// ao binario para cobrir uma API inteira, enquanto o painel usa ~15 endpoints e
// um punhado de campos. O que o SDK resolveria e que nao e trivial -- o hijack
// da conexao e a demultiplexacao do stream de logs -- sao poucas linhas,
// escritas uma vez, e precisamos de controle sobre elas de qualquer forma.
package docker

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Dialer abstrai o transporte ate o daemon.
//
// Existe para que a variante sem porta TCP (relay pelo socket Unix via
// "wsl -e socat") possa ser plugada sem tocar em nenhum handler.
type Dialer interface {
	DialContext(ctx context.Context) (net.Conn, error)
	Descricao() string
}

// tcpDialer conversa com o daemon por TCP, tipicamente 127.0.0.1:2375.
type tcpDialer struct{ addr string }

func (d *tcpDialer) DialContext(ctx context.Context) (net.Conn, error) {
	var dl net.Dialer
	return dl.DialContext(ctx, "tcp", d.addr)
}

func (d *tcpDialer) Descricao() string { return "tcp://" + d.addr }

// Client e um cliente da Engine API.
type Client struct {
	dialer Dialer
	http   *http.Client

	// apiVersion e negociada no Ping e prefixa todas as rotas, para o painel
	// nao quebrar quando o daemon for mais novo ou mais velho que o esperado.
	apiVersion string
}

// Options configura o cliente.
type Options struct {
	// Host no formato tcp://host:porta.
	Host string
	// Timeout das chamadas normais. Streams ignoram este valor.
	Timeout time.Duration
}

// New monta o cliente a partir do host configurado.
func New(o Options) (*Client, error) {
	host := strings.TrimSpace(o.Host)
	if host == "" {
		host = "tcp://127.0.0.1:2375"
	}
	var d Dialer
	switch {
	case strings.HasPrefix(host, "tcp://"):
		d = &tcpDialer{addr: strings.TrimPrefix(host, "tcp://")}
	case strings.HasPrefix(host, "wsl://"):
		d = &wslDialer{distro: strings.TrimPrefix(host, "wsl://")}
	default:
		return nil, fmt.Errorf("host %q nao suportado: use tcp://host:porta ou wsl://distro", host)
	}

	if o.Timeout <= 0 {
		o.Timeout = 30 * time.Second
	}

	return &Client{
		dialer: d,
		http: &http.Client{
			Timeout: o.Timeout,
			Transport: &http.Transport{
				DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
					return d.DialContext(ctx)
				},
			},
		},
	}, nil
}

// Descricao devolve o endpoint em uso, para exibir em telas de diagnostico.
func (c *Client) Descricao() string { return c.dialer.Descricao() }

// do executa uma chamada JSON contra a Engine API.
//
// out pode ser nil quando a resposta nao interessa.
func (c *Client) do(ctx context.Context, metodo, caminho string, q url.Values, body, out any) error {
	var corpo io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("serializar corpo: %w", err)
		}
		corpo = bytes.NewReader(b)
	}

	req, err := c.montarReq(ctx, metodo, caminho, q, corpo)
	if err != nil {
		return err
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("chamar %s %s: %w", metodo, caminho, err)
	}
	defer resp.Body.Close()

	if err := erroDaResposta(resp); err != nil {
		return err
	}
	if out == nil {
		io.Copy(io.Discard, resp.Body)
		return nil
	}
	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		return fmt.Errorf("decodificar resposta de %s: %w", caminho, err)
	}
	return nil
}

// stream abre uma resposta de longa duracao (logs, eventos, stats). O chamador
// e responsavel por fechar o ReadCloser.
func (c *Client) stream(ctx context.Context, caminho string, q url.Values) (io.ReadCloser, error) {
	req, err := c.montarReq(ctx, http.MethodGet, caminho, q, nil)
	if err != nil {
		return nil, err
	}

	// Streams nao podem herdar o Timeout do cliente: seriam cortados no meio.
	// O controle de vida fica por conta do context.
	cli := &http.Client{Transport: c.http.Transport}
	resp, err := cli.Do(req)
	if err != nil {
		return nil, fmt.Errorf("abrir stream %s: %w", caminho, err)
	}
	if err := erroDaResposta(resp); err != nil {
		resp.Body.Close()
		return nil, err
	}
	return resp.Body, nil
}

func (c *Client) montarReq(ctx context.Context, metodo, caminho string, q url.Values, corpo io.Reader) (*http.Request, error) {
	u := "http://docker" + c.prefixo() + caminho
	if len(q) > 0 {
		u += "?" + q.Encode()
	}
	req, err := http.NewRequestWithContext(ctx, metodo, u, corpo)
	if err != nil {
		return nil, fmt.Errorf("montar request para %s: %w", caminho, err)
	}
	if corpo != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	return req, nil
}

func (c *Client) prefixo() string {
	if c.apiVersion == "" {
		return ""
	}
	return "/v" + c.apiVersion
}

// erroRespostaAPI carrega a mensagem que o daemon devolveu.
type erroRespostaAPI struct {
	Status int
	Msg    string
}

func (e *erroRespostaAPI) Error() string {
	if e.Msg == "" {
		return fmt.Sprintf("daemon respondeu HTTP %d", e.Status)
	}
	return e.Msg
}

func erroDaResposta(resp *http.Response) error {
	if resp.StatusCode < 400 {
		return nil
	}
	var corpo struct {
		Message string `json:"message"`
	}
	b, _ := io.ReadAll(io.LimitReader(resp.Body, 8<<10))
	json.Unmarshal(b, &corpo)
	msg := corpo.Message
	if msg == "" {
		msg = strings.TrimSpace(string(b))
	}
	return &erroRespostaAPI{Status: resp.StatusCode, Msg: msg}
}

// hijack abre uma conexao crua com o daemon, usada por exec e attach.
//
// Fala HTTP na mao porque, depois do upgrade, a conexao deixa de ser HTTP e o
// net/http nao devolve o net.Conn subjacente.
func (c *Client) hijack(ctx context.Context, metodo, caminho string, q url.Values, body any) (net.Conn, *bufio.Reader, error) {
	conn, err := c.dialer.DialContext(ctx)
	if err != nil {
		return nil, nil, fmt.Errorf("conectar ao daemon: %w", err)
	}

	var corpo io.Reader
	var tamanho int
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			conn.Close()
			return nil, nil, fmt.Errorf("serializar corpo: %w", err)
		}
		corpo = bytes.NewReader(b)
		tamanho = len(b)
	}

	u := c.prefixo() + caminho
	if len(q) > 0 {
		u += "?" + q.Encode()
	}

	req, err := http.NewRequest(metodo, "http://docker"+u, corpo)
	if err != nil {
		conn.Close()
		return nil, nil, fmt.Errorf("montar request de hijack: %w", err)
	}
	req.Host = "docker"
	req.Header.Set("Connection", "Upgrade")
	req.Header.Set("Upgrade", "tcp")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
		req.ContentLength = int64(tamanho)
	}

	if err := req.Write(conn); err != nil {
		conn.Close()
		return nil, nil, fmt.Errorf("enviar request de hijack: %w", err)
	}

	br := bufio.NewReader(conn)
	resp, err := http.ReadResponse(br, req)
	if err != nil {
		conn.Close()
		return nil, nil, fmt.Errorf("ler resposta de hijack: %w", err)
	}
	// 101 = upgrade aceito; 200 = daemon antigo que responde sem upgrade.
	if resp.StatusCode != http.StatusSwitchingProtocols && resp.StatusCode != http.StatusOK {
		defer conn.Close()
		return nil, nil, erroDaResposta(resp)
	}
	return conn, br, nil
}
