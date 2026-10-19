package docker

import (
	"reflect"
	"testing"
)

func TestContainerNome(t *testing.T) {
	casos := []struct {
		nome     string
		c        Container
		esperado string
	}{
		{"nome com barra inicial", Container{Names: []string{"/web"}}, "web"},
		{"primeiro nome vence", Container{Names: []string{"/web", "/alias"}}, "web"},
		{"nome sem barra", Container{Names: []string{"web"}}, "web"},
		{"sem nomes cai para o id curto", Container{ID: "abcdef0123456789"}, "abcdef012345"},
		{"sem nomes e sem id", Container{}, ""},
		{"nome vazio na lista", Container{Names: []string{""}}, ""},
	}

	for _, c := range casos {
		t.Run(c.nome, func(t *testing.T) {
			if got := c.c.Nome(); got != c.esperado {
				t.Errorf("Nome() = %q; esperado %q", got, c.esperado)
			}
		})
	}
}

func TestContainerIDCurto(t *testing.T) {
	casos := []struct{ id, esperado string }{
		{"abcdef0123456789abcdef", "abcdef012345"},
		{"abcdef012345", "abcdef012345"}, // exatos 12 nao sao cortados
		{"abc", "abc"},
		{"", ""},
	}

	for _, c := range casos {
		if got := (Container{ID: c.id}).IDCurto(); got != c.esperado {
			t.Errorf("IDCurto(%q) = %q; esperado %q", c.id, got, c.esperado)
		}
	}
}

func TestContainerLabelsCompose(t *testing.T) {
	c := Container{Labels: map[string]string{
		"com.docker.compose.project": "meuapp",
		"com.docker.compose.service": "banco",
	}}

	if got := c.Projeto(); got != "meuapp" {
		t.Errorf("Projeto() = %q; esperado %q", got, "meuapp")
	}
	if got := c.Servico(); got != "banco" {
		t.Errorf("Servico() = %q; esperado %q", got, "banco")
	}

	// Container avulso nao tem os labels; ler mapa nil nao pode entrar em panico.
	avulso := Container{}
	if got := avulso.Projeto(); got != "" {
		t.Errorf("Projeto() sem labels = %q; esperado vazio", got)
	}
	if got := avulso.Servico(); got != "" {
		t.Errorf("Servico() sem labels = %q; esperado vazio", got)
	}
}

func TestContainerRodando(t *testing.T) {
	casos := []struct {
		estado   string
		esperado bool
	}{
		{"running", true},
		{"exited", false},
		{"paused", false},
		{"created", false},
		{"restarting", false},
		{"dead", false},
		{"", false},
	}

	for _, c := range casos {
		if got := (Container{State: c.estado}).Rodando(); got != c.esperado {
			t.Errorf("State %q: Rodando() = %v; esperado %v", c.estado, got, c.esperado)
		}
	}
}

// TestPortasPublicadasDeduplicaFamiliaDeEndereco cobre o motivo de existir a
// deduplicacao: a Engine API lista uma entrada por familia de endereco
// (0.0.0.0 e [::]) para o mesmo bind, o que duplicaria toda porta na tela.
func TestPortasPublicadasDeduplicaFamiliaDeEndereco(t *testing.T) {
	c := Container{Ports: []Port{
		{IP: "0.0.0.0", PublicPort: 8080, PrivatePort: 80, Type: "tcp"},
		{IP: "::", PublicPort: 8080, PrivatePort: 80, Type: "tcp"},
	}}

	got := c.PortasPublicadas()

	esperado := []string{"8080→80/tcp"}
	if !reflect.DeepEqual(got, esperado) {
		t.Errorf("PortasPublicadas() = %v; esperado %v", got, esperado)
	}
}

func TestPortasPublicadas(t *testing.T) {
	casos := []struct {
		nome     string
		ports    []Port
		esperado []string
	}{
		{"sem portas", nil, nil},
		{
			"porta exposta mas nao publicada e ignorada",
			[]Port{{PublicPort: 0, PrivatePort: 5432, Type: "tcp"}},
			nil,
		},
		{
			"udp aparece na listagem",
			[]Port{{PublicPort: 53, PrivatePort: 53, Type: "udp"}},
			[]string{"53→53/udp"},
		},
		{
			"ordenacao estavel",
			[]Port{
				{PublicPort: 9000, PrivatePort: 90, Type: "tcp"},
				{PublicPort: 3000, PrivatePort: 30, Type: "tcp"},
			},
			[]string{"3000→30/tcp", "9000→90/tcp"},
		},
		{
			"mistura publicada e nao publicada",
			[]Port{
				{PublicPort: 0, PrivatePort: 9229, Type: "tcp"},
				{PublicPort: 8080, PrivatePort: 80, Type: "tcp"},
			},
			[]string{"8080→80/tcp"},
		},
	}

	for _, c := range casos {
		t.Run(c.nome, func(t *testing.T) {
			if got := (Container{Ports: c.ports}).PortasPublicadas(); !reflect.DeepEqual(got, c.esperado) {
				t.Errorf("PortasPublicadas() = %v; esperado %v", got, c.esperado)
			}
		})
	}
}

// TestLinksPublicados confere o criterio de "vale oferecer link clicavel":
// e a porta INTERNA que diz qual servico roda ali.
func TestLinksPublicados(t *testing.T) {
	casos := []struct {
		nome     string
		ports    []Port
		esperado []LinkPorta
	}{
		{"sem portas", nil, nil},
		{
			"porta web ganha link",
			[]Port{{PublicPort: 8080, PrivatePort: 80, Type: "tcp"}},
			[]LinkPorta{{Rotulo: "8080→80/tcp", URL: "http://localhost:8080/", Web: true}},
		},
		{
			"postgres nao ganha link",
			[]Port{{PublicPort: 5432, PrivatePort: 5432, Type: "tcp"}},
			[]LinkPorta{{Rotulo: "5432→5432/tcp", Web: false}},
		},
		{
			"porta interna manda, nao a publicada",
			// Postgres publicado numa porta alta continua sem link.
			[]Port{{PublicPort: 15432, PrivatePort: 5432, Type: "tcp"}},
			[]LinkPorta{{Rotulo: "15432→5432/tcp", Web: false}},
		},
		{
			"udp fica de fora",
			[]Port{{PublicPort: 53, PrivatePort: 53, Type: "udp"}},
			nil,
		},
		{
			"nao publicada fica de fora",
			[]Port{{PublicPort: 0, PrivatePort: 80, Type: "tcp"}},
			nil,
		},
		{
			"duplicata de familia de endereco e descartada",
			[]Port{
				{IP: "0.0.0.0", PublicPort: 8080, PrivatePort: 80, Type: "tcp"},
				{IP: "::", PublicPort: 8080, PrivatePort: 80, Type: "tcp"},
			},
			[]LinkPorta{{Rotulo: "8080→80/tcp", URL: "http://localhost:8080/", Web: true}},
		},
		{
			"ordenado pelo rotulo",
			[]Port{
				{PublicPort: 9000, PrivatePort: 90, Type: "tcp"},
				{PublicPort: 3000, PrivatePort: 30, Type: "tcp"},
			},
			[]LinkPorta{
				{Rotulo: "3000→30/tcp", URL: "http://localhost:3000/", Web: true},
				{Rotulo: "9000→90/tcp", URL: "http://localhost:9000/", Web: true},
			},
		},
	}

	for _, c := range casos {
		t.Run(c.nome, func(t *testing.T) {
			if got := (Container{Ports: c.ports}).LinksPublicados(); !reflect.DeepEqual(got, c.esperado) {
				t.Errorf("LinksPublicados() = %+v; esperado %+v", got, c.esperado)
			}
		})
	}
}

// TestLinksPublicadosServicosNaoWeb trava a lista de portas que nao falam
// HTTP: oferecer link para elas so geraria aba de erro.
func TestLinksPublicadosServicosNaoWeb(t *testing.T) {
	naoWeb := []int{5432, 3306, 1433, 27017, 6379, 5672, 9092, 22, 25, 1521}

	for _, p := range naoWeb {
		links := (Container{Ports: []Port{
			{PublicPort: 1000, PrivatePort: p, Type: "tcp"},
		}}).LinksPublicados()

		if len(links) != 1 {
			t.Fatalf("porta %d: esperado 1 link; obtido %d", p, len(links))
		}
		if links[0].Web {
			t.Errorf("porta interna %d marcada como web", p)
		}
		if links[0].URL != "" {
			t.Errorf("porta interna %d recebeu URL %q; esperado vazio", p, links[0].URL)
		}
	}
}

func TestContainerDetailNome(t *testing.T) {
	if got := (ContainerDetail{Name: "/meu-app"}).Nome(); got != "meu-app" {
		t.Errorf("Nome() = %q; esperado %q", got, "meu-app")
	}
	if got := (ContainerDetail{Name: "meu-app"}).Nome(); got != "meu-app" {
		t.Errorf("Nome() = %q; esperado %q", got, "meu-app")
	}
	if got := (ContainerDetail{}).Nome(); got != "" {
		t.Errorf("Nome() = %q; esperado vazio", got)
	}
}

func TestImageTag(t *testing.T) {
	casos := []struct {
		nome     string
		tags     []string
		esperado string
	}{
		{"tag normal", []string{"nginx:1.25"}, "nginx:1.25"},
		{"primeira tag util vence", []string{"nginx:1.25", "nginx:latest"}, "nginx:1.25"},
		{"imagem orfa", []string{"<none>:<none>"}, "<none>"},
		{"sem tags", nil, "<none>"},
		{"tag vazia e pulada", []string{"", "nginx:1"}, "nginx:1"},
		{"orfa seguida de tag real", []string{"<none>:<none>", "app:dev"}, "app:dev"},
	}

	for _, c := range casos {
		t.Run(c.nome, func(t *testing.T) {
			if got := (Image{RepoTags: c.tags}).Tag(); got != c.esperado {
				t.Errorf("Tag() = %q; esperado %q", got, c.esperado)
			}
		})
	}
}
