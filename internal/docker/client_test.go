package docker

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// clienteFake aponta um Client real para um servidor de teste que finge ser o
// daemon. O transporte do Client disca sempre no endereco fixo, entao basta
// passar o host do httptest.
func clienteFake(t *testing.T, h http.HandlerFunc) *Client {
	t.Helper()

	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)

	c, err := New(Options{
		Host:    "tcp://" + strings.TrimPrefix(srv.URL, "http://"),
		Timeout: 5 * time.Second,
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return c
}

func TestNewEscolheTransportePeloPrefixo(t *testing.T) {
	casos := []struct {
		host      string
		descricao string
		erro      bool
	}{
		{"", "tcp://127.0.0.1:2375", false},
		{"tcp://10.0.0.5:2376", "tcp://10.0.0.5:2376", false},
		{"wsl://Ubuntu-24.04", "wsl://Ubuntu-24.04", false},
		{"wsl://", "wsl://(distro padrao)", false},
		{"unix:///var/run/docker.sock", "unix:///var/run/docker.sock", false},
		{"http://127.0.0.1:2375", "", true},
	}
	for _, c := range casos {
		cli, err := New(Options{Host: c.host})
		if c.erro {
			if err == nil {
				t.Errorf("host %q: esperava erro", c.host)
			}
			continue
		}
		if err != nil {
			t.Errorf("host %q: erro inesperado: %v", c.host, err)
			continue
		}
		if got := cli.Descricao(); got != c.descricao {
			t.Errorf("host %q: descricao = %q, esperava %q", c.host, got, c.descricao)
		}
	}
}

// TestNewHostNaoSuportado completa a tabela acima com as formas que um usuario
// vindo de outra ferramenta tentaria: o npipe do Docker Desktop e o host cru,
// sem esquema. Aceitar qualquer uma delas daria um cliente que nunca conecta.
func TestNewHostNaoSuportado(t *testing.T) {
	casos := []string{
		"npipe:////./pipe/docker_engine",
		"127.0.0.1:2375",
		"ssh://usuario@maquina",
	}

	for _, host := range casos {
		if _, err := New(Options{Host: host}); err == nil {
			t.Errorf("New(%q) nao devolveu erro", host)
		}
	}
}

// TestUnixDialerDescricao: o caminho do socket entra inteiro na descricao,
// inclusive a barra que sobra do "unix://" + "/var/run/...".
func TestUnixDialerDescricao(t *testing.T) {
	casos := []struct{ caminho, esperado string }{
		{"/var/run/docker.sock", "unix:///var/run/docker.sock"},
		{"/home/dev/.docker/desktop/docker.sock", "unix:///home/dev/.docker/desktop/docker.sock"},
	}

	for _, c := range casos {
		if got := (&unixDialer{caminho: c.caminho}).Descricao(); got != c.esperado {
			t.Errorf("Descricao() = %q; esperado %q", got, c.esperado)
		}
	}
}

// TestUnixDialerSocketInexistente garante que a falha vem do dial, e nao de um
// panico por caminho vazio.
func TestUnixDialerSocketInexistente(t *testing.T) {
	d := &unixDialer{caminho: filepath.Join(t.TempDir(), "nao-existe.sock")}

	conn, err := d.DialContext(context.Background())

	if err == nil {
		conn.Close()
		t.Fatal("esperado erro para socket inexistente")
	}
}

func TestNewTimeoutPadrao(t *testing.T) {
	c, err := New(Options{})
	if err != nil {
		t.Fatalf("New com host vazio: %v", err)
	}
	if c.http.Timeout != 30*time.Second {
		t.Errorf("Timeout = %v; esperado 30s", c.http.Timeout)
	}
}

func TestNewHostComEspacos(t *testing.T) {
	c, err := New(Options{Host: "  tcp://10.0.0.1:2375  "})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if got := c.Descricao(); got != "tcp://10.0.0.1:2375" {
		t.Errorf("Descricao() = %q; esperado tcp://10.0.0.1:2375", got)
	}
}

// TestPingNegociaVersao cobre o motivo do Ping existir: a primeira chamada vai
// sem prefixo de versao e as seguintes passam a usar a versao negociada.
func TestPingNegociaVersao(t *testing.T) {
	var caminhos []string
	c := clienteFake(t, func(w http.ResponseWriter, r *http.Request) {
		caminhos = append(caminhos, r.URL.Path)
		if strings.HasSuffix(r.URL.Path, "/version") {
			w.Write([]byte(`{"Version":"27.1.1","ApiVersion":"1.46","Os":"linux","Arch":"amd64"}`))
			return
		}
		w.Write([]byte(`[]`))
	})

	v, err := c.Ping(context.Background())
	if err != nil {
		t.Fatalf("Ping: %v", err)
	}
	if v.Version != "27.1.1" || v.APIVersion != "1.46" {
		t.Errorf("Version = %+v; esperado 27.1.1 / 1.46", v)
	}
	if caminhos[0] != "/version" {
		t.Errorf("primeiro caminho = %q; esperado /version sem prefixo", caminhos[0])
	}

	if _, err := c.ContainerList(context.Background(), false); err != nil {
		t.Fatalf("ContainerList: %v", err)
	}
	if caminhos[1] != "/v1.46/containers/json" {
		t.Errorf("segundo caminho = %q; esperado prefixo /v1.46", caminhos[1])
	}
}

// TestPingComFalhaPreservaVersaoAnterior evita que uma queda momentanea do
// daemon degrade o cliente para rotas sem versao.
func TestPingComFalhaPreservaVersaoAnterior(t *testing.T) {
	var falhar bool
	c := clienteFake(t, func(w http.ResponseWriter, r *http.Request) {
		if falhar {
			http.Error(w, "indisponivel", http.StatusServiceUnavailable)
			return
		}
		w.Write([]byte(`{"ApiVersion":"1.46"}`))
	})

	if _, err := c.Ping(context.Background()); err != nil {
		t.Fatalf("primeiro Ping: %v", err)
	}
	falhar = true
	if _, err := c.Ping(context.Background()); err == nil {
		t.Fatal("segundo Ping deveria falhar")
	}
	if c.apiVersion != "1.46" {
		t.Errorf("apiVersion = %q; esperado continuar 1.46 apos falha", c.apiVersion)
	}
}

// TestPingSemApiVersionMantemSemPrefixo cobre daemon que nao informa a versao.
func TestPingSemApiVersionMantemSemPrefixo(t *testing.T) {
	c := clienteFake(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Write([]byte(`{"Version":"20.10.0"}`))
	})

	if _, err := c.Ping(context.Background()); err != nil {
		t.Fatalf("Ping: %v", err)
	}
	if c.prefixo() != "" {
		t.Errorf("prefixo() = %q; esperado vazio", c.prefixo())
	}
}

// TestErroDaRespostaUsaMensagemDoDaemon garante que a mensagem util do daemon
// chegue a tela, em vez de um "HTTP 404" generico.
func TestErroDaRespostaUsaMensagemDoDaemon(t *testing.T) {
	casos := []struct {
		nome     string
		status   int
		corpo    string
		esperado string
	}{
		{"json com message", 404, `{"message":"No such container: abc"}`, "No such container: abc"},
		{"corpo em texto puro", 500, "falha interna", "falha interna"},
		{"corpo vazio", 503, "", "daemon respondeu HTTP 503"},
		{"json sem message", 409, `{"outro":"x"}`, `{"outro":"x"}`},
	}

	for _, c := range casos {
		t.Run(c.nome, func(t *testing.T) {
			cli := clienteFake(t, func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(c.status)
				w.Write([]byte(c.corpo))
			})

			err := cli.ContainerStart(context.Background(), "abc")
			if err == nil {
				t.Fatal("esperado erro")
			}
			if err.Error() != c.esperado {
				t.Errorf("erro = %q; esperado %q", err.Error(), c.esperado)
			}
		})
	}
}

func TestRespostaDeSucessoNaoViraErro(t *testing.T) {
	// 204 No Content e a resposta normal de start/stop.
	c := clienteFake(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})

	if err := c.ContainerStart(context.Background(), "abc"); err != nil {
		t.Errorf("ContainerStart: %v", err)
	}
}

func TestRespostaInvalidaDaErroDeDecodificacao(t *testing.T) {
	c := clienteFake(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Write([]byte("isto nao e json"))
	})

	_, err := c.ContainerList(context.Background(), false)
	if err == nil {
		t.Fatal("esperado erro de decodificacao")
	}
	if !strings.Contains(err.Error(), "decodificar") {
		t.Errorf("erro = %q; esperado mencionar decodificacao", err)
	}
}

func TestDaemonInacessivelDaErro(t *testing.T) {
	// Porta de descarte: nada escuta ali.
	c, err := New(Options{Host: "tcp://127.0.0.1:9", Timeout: time.Second})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if _, err := c.Ping(context.Background()); err == nil {
		t.Fatal("esperado erro ao falar com daemon inacessivel")
	}
}

// TestContainerListOrdenacao trava a ordem estavel da listagem: rodando
// primeiro, depois por nome, para a tela nao embaralhar a cada atualizacao.
func TestContainerListOrdenacao(t *testing.T) {
	c := clienteFake(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("all") != "1" {
			t.Errorf("query all = %q; esperado 1", r.URL.Query().Get("all"))
		}
		w.Write([]byte(`[
			{"Id":"3","Names":["/zebra"],"State":"exited"},
			{"Id":"1","Names":["/beta"],"State":"running"},
			{"Id":"2","Names":["/alfa"],"State":"exited"},
			{"Id":"4","Names":["/alfa-rodando"],"State":"running"}
		]`))
	})

	cts, err := c.ContainerList(context.Background(), true)
	if err != nil {
		t.Fatalf("ContainerList: %v", err)
	}

	esperado := []string{"alfa-rodando", "beta", "alfa", "zebra"}
	if len(cts) != len(esperado) {
		t.Fatalf("obtidos %d containers; esperado %d", len(cts), len(esperado))
	}
	for i, nome := range esperado {
		if cts[i].Nome() != nome {
			t.Errorf("posicao %d = %q; esperado %q", i, cts[i].Nome(), nome)
		}
	}
}

func TestContainerListSemAllNaoEnviaQuery(t *testing.T) {
	c := clienteFake(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.RawQuery != "" {
			t.Errorf("query = %q; esperado vazia", r.URL.RawQuery)
		}
		w.Write([]byte(`[]`))
	})

	if _, err := c.ContainerList(context.Background(), false); err != nil {
		t.Fatalf("ContainerList: %v", err)
	}
}

// TestAcoesDeCicloDeVida confere metodo, caminho e query de cada operacao.
func TestAcoesDeCicloDeVida(t *testing.T) {
	casos := []struct {
		nome    string
		chamar  func(*Client) error
		metodo  string
		caminho string
		query   string
	}{
		{
			"start",
			func(c *Client) error { return c.ContainerStart(context.Background(), "abc") },
			http.MethodPost, "/containers/abc/start", "",
		},
		{
			"stop com timeout",
			func(c *Client) error { return c.ContainerStop(context.Background(), "abc", 10) },
			http.MethodPost, "/containers/abc/stop", "t=10",
		},
		{
			"stop sem timeout",
			func(c *Client) error { return c.ContainerStop(context.Background(), "abc", 0) },
			http.MethodPost, "/containers/abc/stop", "",
		},
		{
			"restart",
			func(c *Client) error { return c.ContainerRestart(context.Background(), "abc", 10) },
			http.MethodPost, "/containers/abc/restart", "t=10",
		},
		{
			"pause",
			func(c *Client) error { return c.ContainerPause(context.Background(), "abc") },
			http.MethodPost, "/containers/abc/pause", "",
		},
		{
			"unpause",
			func(c *Client) error { return c.ContainerUnpause(context.Background(), "abc") },
			http.MethodPost, "/containers/abc/unpause", "",
		},
		{
			"remove com force",
			func(c *Client) error { return c.ContainerRemove(context.Background(), "abc", true, false) },
			http.MethodDelete, "/containers/abc", "force=1",
		},
		{
			"remove com volumes anonimos",
			func(c *Client) error { return c.ContainerRemove(context.Background(), "abc", true, true) },
			http.MethodDelete, "/containers/abc", "force=1&v=1",
		},
		{
			"image remove com force",
			func(c *Client) error { return c.ImageRemove(context.Background(), "sha256:abc", true) },
			http.MethodDelete, "/images/sha256:abc", "force=1",
		},
		{
			"volume remove sem force",
			func(c *Client) error { return c.VolumeRemove(context.Background(), "dados", false) },
			http.MethodDelete, "/volumes/dados", "",
		},
	}

	for _, c := range casos {
		t.Run(c.nome, func(t *testing.T) {
			var metodo, caminho, query string
			cli := clienteFake(t, func(w http.ResponseWriter, r *http.Request) {
				metodo, caminho, query = r.Method, r.URL.Path, r.URL.RawQuery
				w.WriteHeader(http.StatusNoContent)
			})

			if err := c.chamar(cli); err != nil {
				t.Fatalf("chamada: %v", err)
			}
			if metodo != c.metodo {
				t.Errorf("metodo = %q; esperado %q", metodo, c.metodo)
			}
			if caminho != c.caminho {
				t.Errorf("caminho = %q; esperado %q", caminho, c.caminho)
			}
			if query != c.query {
				t.Errorf("query = %q; esperado %q", query, c.query)
			}
		})
	}
}

func TestContainerInspect(t *testing.T) {
	c := clienteFake(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/containers/abc/json" {
			t.Errorf("caminho = %q", r.URL.Path)
		}
		w.Write([]byte(`{
			"Id":"abc","Name":"/web",
			"State":{"Status":"running","Running":true,"ExitCode":0,"Health":{"Status":"healthy"}},
			"Config":{"Image":"nginx:1.25","Tty":true}
		}`))
	})

	d, err := c.ContainerInspect(context.Background(), "abc")
	if err != nil {
		t.Fatalf("ContainerInspect: %v", err)
	}
	if d.Nome() != "web" {
		t.Errorf("Nome() = %q; esperado web", d.Nome())
	}
	if !d.State.Running || !d.Config.Tty {
		t.Errorf("estado = %+v; esperado running com tty", d.State)
	}
	if d.State.Health == nil || d.State.Health.Status != "healthy" {
		t.Errorf("Health = %+v; esperado healthy", d.State.Health)
	}
}

// TestContainerInspectSemHealth cobre container sem healthcheck: o ponteiro
// fica nil e nenhum consumidor pode assumir que ele existe.
func TestContainerInspectSemHealth(t *testing.T) {
	c := clienteFake(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Write([]byte(`{"Id":"abc","Name":"/web","State":{"Status":"exited"}}`))
	})

	d, err := c.ContainerInspect(context.Background(), "abc")
	if err != nil {
		t.Fatalf("ContainerInspect: %v", err)
	}
	if d.State.Health != nil {
		t.Errorf("Health = %+v; esperado nil", d.State.Health)
	}
}

func TestContextCanceladoInterrompeChamada(t *testing.T) {
	c := clienteFake(t, func(w http.ResponseWriter, _ *http.Request) {
		time.Sleep(2 * time.Second)
		w.Write([]byte(`[]`))
	})

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if _, err := c.ContainerList(ctx, false); err == nil {
		t.Fatal("esperado erro com context ja cancelado")
	}
}
