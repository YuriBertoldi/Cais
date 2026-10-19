package docker

import "testing"

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
		{"unix:///var/run/docker.sock", "", true},
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
