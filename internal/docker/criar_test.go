package docker

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"reflect"
	"testing"
)

// TestSepararTag cobre a armadilha do registry com porta: em
// "localhost:5000/app" os dois pontos pertencem ao host, nao a tag.
func TestSepararTag(t *testing.T) {
	casos := []struct {
		entrada string
		imagem  string
		tag     string
	}{
		{"nginx", "nginx", "latest"},
		{"nginx:1.25", "nginx", "1.25"},
		{"library/nginx:1.25", "library/nginx", "1.25"},
		{"localhost:5000/app", "localhost:5000/app", "latest"},
		{"localhost:5000/app:v2", "localhost:5000/app", "v2"},
		{"registry.local:8443/time/app:dev", "registry.local:8443/time/app", "dev"},
		{"  nginx:1.25  ", "nginx", "1.25"},
		{"", "", "latest"},

		// Referencia por digest: o separador e o "@", e o digest vai inteiro
		// no lugar da tag.
		{
			"nginx@sha256:a1b2c3d4e5f6",
			"nginx", "sha256:a1b2c3d4e5f6",
		},
		{
			"localhost:5000/app@sha256:a1b2c3",
			"localhost:5000/app", "sha256:a1b2c3",
		},
		{
			"registry.local:8443/time/app@sha256:ff00",
			"registry.local:8443/time/app", "sha256:ff00",
		},

		// Malformadas: nao viram pull de imagem sem nome nem sem tag.
		{"nginx@", "nginx@", "latest"},
		{"@sha256:abc", "@sha256", "abc"},
	}

	for _, c := range casos {
		img, tag := separarTag(c.entrada)
		if img != c.imagem || tag != c.tag {
			t.Errorf("separarTag(%q) = (%q, %q); esperado (%q, %q)", c.entrada, img, tag, c.imagem, c.tag)
		}
	}
}

func TestVolumeCreateDriverPadrao(t *testing.T) {
	var corpo map[string]any
	c := clienteFake(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/volumes/create" || r.Method != http.MethodPost {
			t.Errorf("%s %s; esperado POST /volumes/create", r.Method, r.URL.Path)
		}
		json.NewDecoder(r.Body).Decode(&corpo)
		w.Write([]byte(`{"Name":"dados","Driver":"local"}`))
	})

	v, err := c.VolumeCreate(context.Background(), "dados", "")
	if err != nil {
		t.Fatalf("VolumeCreate: %v", err)
	}
	if corpo["Driver"] != "local" {
		t.Errorf("Driver enviado = %v; esperado local", corpo["Driver"])
	}
	if corpo["Name"] != "dados" {
		t.Errorf("Name enviado = %v; esperado dados", corpo["Name"])
	}
	if v.Name != "dados" {
		t.Errorf("volume devolvido = %+v", v)
	}
}

func TestVolumeCreateDriverInformado(t *testing.T) {
	var corpo map[string]any
	c := clienteFake(t, func(w http.ResponseWriter, r *http.Request) {
		json.NewDecoder(r.Body).Decode(&corpo)
		w.Write([]byte(`{}`))
	})

	if _, err := c.VolumeCreate(context.Background(), "dados", "nfs"); err != nil {
		t.Fatalf("VolumeCreate: %v", err)
	}
	if corpo["Driver"] != "nfs" {
		t.Errorf("Driver = %v; esperado nfs", corpo["Driver"])
	}
}

// TestContainerCreatePublicaSoEmLoopback trava a decisao de seguranca: um
// container de desenvolvimento nao deve ficar exposto na rede da empresa.
func TestContainerCreatePublicaSoEmLoopback(t *testing.T) {
	var corpo struct {
		Image        string              `json:"Image"`
		ExposedPorts map[string]struct{} `json:"ExposedPorts"`
		Env          []string            `json:"Env"`
		Cmd          []string            `json:"Cmd"`
		HostConfig   struct {
			PortBindings  map[string][]map[string]string `json:"PortBindings"`
			Binds         []string                       `json:"Binds"`
			RestartPolicy struct {
				Name string `json:"Name"`
			} `json:"RestartPolicy"`
		} `json:"HostConfig"`
	}
	var nomeQuery string

	c := clienteFake(t, func(w http.ResponseWriter, r *http.Request) {
		nomeQuery = r.URL.Query().Get("name")
		json.NewDecoder(r.Body).Decode(&corpo)
		w.Write([]byte(`{"Id":"novoid","Warnings":[]}`))
	})

	id, err := c.ContainerCreate(context.Background(), NovoContainer{
		Nome:    "meu-app",
		Imagem:  "nginx:1.25",
		Env:     []string{"CHAVE=valor"},
		Volumes: []string{"dados:/var/lib/dados"},
		Cmd:     []string{"nginx", "-g", "daemon off;"},
		Portas: []PortaMapeada{
			{Host: "8080", Container: "80", Protocolo: "tcp"},
			{Host: "5353", Container: "53", Protocolo: "udp"},
		},
	})
	if err != nil {
		t.Fatalf("ContainerCreate: %v", err)
	}

	if id != "novoid" {
		t.Errorf("id = %q; esperado novoid", id)
	}
	if nomeQuery != "meu-app" {
		t.Errorf("query name = %q; esperado meu-app", nomeQuery)
	}

	for _, chave := range []string{"80/tcp", "53/udp"} {
		if _, ok := corpo.ExposedPorts[chave]; !ok {
			t.Errorf("ExposedPorts sem %q: %+v", chave, corpo.ExposedPorts)
		}
	}

	bind := corpo.HostConfig.PortBindings["80/tcp"]
	if len(bind) != 1 {
		t.Fatalf("PortBindings[80/tcp] = %+v; esperado 1 entrada", bind)
	}
	if bind[0]["HostIp"] != "127.0.0.1" {
		t.Errorf("HostIp = %q; esperado 127.0.0.1 (nunca 0.0.0.0)", bind[0]["HostIp"])
	}
	if bind[0]["HostPort"] != "8080" {
		t.Errorf("HostPort = %q; esperado 8080", bind[0]["HostPort"])
	}

	if !reflect.DeepEqual(corpo.HostConfig.Binds, []string{"dados:/var/lib/dados"}) {
		t.Errorf("Binds = %v", corpo.HostConfig.Binds)
	}
	if !reflect.DeepEqual(corpo.Env, []string{"CHAVE=valor"}) {
		t.Errorf("Env = %v", corpo.Env)
	}
	if len(corpo.Cmd) != 3 {
		t.Errorf("Cmd = %v; esperado 3 elementos", corpo.Cmd)
	}
}

func TestContainerCreateRestartPadrao(t *testing.T) {
	var corpo map[string]any
	c := clienteFake(t, func(w http.ResponseWriter, r *http.Request) {
		json.NewDecoder(r.Body).Decode(&corpo)
		w.Write([]byte(`{"Id":"x"}`))
	})

	if _, err := c.ContainerCreate(context.Background(), NovoContainer{Imagem: "nginx"}); err != nil {
		t.Fatalf("ContainerCreate: %v", err)
	}

	hc := corpo["HostConfig"].(map[string]any)
	rp := hc["RestartPolicy"].(map[string]any)
	if rp["Name"] != "no" {
		t.Errorf("RestartPolicy.Name = %v; esperado \"no\"", rp["Name"])
	}
	if _, tem := hc["Binds"]; tem {
		t.Errorf("Binds nao deveria ser enviado sem volumes: %v", hc["Binds"])
	}
	if _, tem := corpo["Env"]; tem {
		t.Errorf("Env nao deveria ser enviado vazio")
	}
	if _, tem := corpo["Cmd"]; tem {
		t.Errorf("Cmd nao deveria ser enviado vazio")
	}
}

func TestContainerCreateProtocoloPadraoTCP(t *testing.T) {
	var corpo map[string]any
	c := clienteFake(t, func(w http.ResponseWriter, r *http.Request) {
		json.NewDecoder(r.Body).Decode(&corpo)
		w.Write([]byte(`{"Id":"x"}`))
	})

	_, err := c.ContainerCreate(context.Background(), NovoContainer{
		Imagem: "nginx",
		Portas: []PortaMapeada{{Host: "8080", Container: "80"}}, // sem protocolo
	})
	if err != nil {
		t.Fatalf("ContainerCreate: %v", err)
	}

	expostas := corpo["ExposedPorts"].(map[string]any)
	if _, ok := expostas["80/tcp"]; !ok {
		t.Errorf("ExposedPorts = %v; esperado chave 80/tcp", expostas)
	}
}

func TestContainerCreateSemNomeNaoEnviaQuery(t *testing.T) {
	c := clienteFake(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.RawQuery != "" {
			t.Errorf("query = %q; esperado vazia", r.URL.RawQuery)
		}
		w.Write([]byte(`{"Id":"x"}`))
	})

	if _, err := c.ContainerCreate(context.Background(), NovoContainer{Imagem: "nginx"}); err != nil {
		t.Fatalf("ContainerCreate: %v", err)
	}
}

func TestContainerCreatePropagaErroDoDaemon(t *testing.T) {
	c := clienteFake(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusConflict)
		w.Write([]byte(`{"message":"Conflict. The container name is already in use"}`))
	})

	id, err := c.ContainerCreate(context.Background(), NovoContainer{Imagem: "nginx", Nome: "web"})
	if err == nil {
		t.Fatal("esperado erro de conflito")
	}
	if id != "" {
		t.Errorf("id = %q; esperado vazio em caso de erro", id)
	}
}

func TestImagePullEntregaProgresso(t *testing.T) {
	var query string
	c := clienteFake(t, func(w http.ResponseWriter, r *http.Request) {
		query = r.URL.RawQuery
		io.WriteString(w, `{"status":"Pulling from library/nginx","id":"1.25"}`+"\n")
		io.WriteString(w, `{"status":"Downloading","id":"abc","progress":"[===>] 10MB"}`+"\n")
		io.WriteString(w, "\n") // linha em branco e ignorada
		io.WriteString(w, `nao e json`+"\n")
		io.WriteString(w, `{"status":"Download complete","id":"abc"}`+"\n")
	})

	var recebidos []ProgressoPull
	err := c.ImagePull(context.Background(), "nginx:1.25", func(p ProgressoPull) {
		recebidos = append(recebidos, p)
	})
	if err != nil {
		t.Fatalf("ImagePull: %v", err)
	}

	if query != "fromImage=nginx&tag=1.25" {
		t.Errorf("query = %q; esperado fromImage=nginx&tag=1.25", query)
	}
	if len(recebidos) != 3 {
		t.Fatalf("recebidos %d eventos; esperado 3 (linha vazia e lixo sao descartados)", len(recebidos))
	}
	if recebidos[1].Progresso != "[===>] 10MB" {
		t.Errorf("progresso = %q", recebidos[1].Progresso)
	}
}

// TestImagePullPorDigest garante que uma referencia fixada por digest chegue
// ao daemon inteira, e nao partida no ":" do algoritmo.
func TestImagePullPorDigest(t *testing.T) {
	var query url.Values
	c := clienteFake(t, func(w http.ResponseWriter, r *http.Request) {
		query = r.URL.Query()
		io.WriteString(w, `{"status":"Pulling from library/nginx"}`+"\n")
	})

	err := c.ImagePull(context.Background(), "nginx@sha256:a1b2c3", func(ProgressoPull) {})
	if err != nil {
		t.Fatalf("ImagePull: %v", err)
	}

	if got := query.Get("fromImage"); got != "nginx" {
		t.Errorf("fromImage = %q; esperado nginx", got)
	}
	if got := query.Get("tag"); got != "sha256:a1b2c3" {
		t.Errorf("tag = %q; esperado o digest inteiro sha256:a1b2c3", got)
	}
}

// TestImagePullErroNoCorpo cobre a armadilha do pull: o daemon devolve HTTP
// 200 e so depois reporta a falha dentro do stream.
func TestImagePullErroNoCorpo(t *testing.T) {
	c := clienteFake(t, func(w http.ResponseWriter, _ *http.Request) {
		io.WriteString(w, `{"status":"Pulling from library/nginx"}`+"\n")
		io.WriteString(w, `{"error":"manifest unknown"}`+"\n")
		io.WriteString(w, `{"status":"nao deveria chegar aqui"}`+"\n")
	})

	var recebidos int
	err := c.ImagePull(context.Background(), "nginx:naoexiste", func(ProgressoPull) { recebidos++ })

	if err == nil {
		t.Fatal("esperado erro vindo do corpo da resposta")
	}
	if err.Error() != "manifest unknown" {
		t.Errorf("erro = %q; esperado \"manifest unknown\"", err)
	}
	if recebidos != 1 {
		t.Errorf("recebidos %d eventos; esperado 1 (o stream para no erro)", recebidos)
	}
}

func TestImagePullErroHTTP(t *testing.T) {
	c := clienteFake(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`{"message":"falha no registry"}`))
	})

	err := c.ImagePull(context.Background(), "nginx", func(ProgressoPull) {})
	if err == nil || err.Error() != "falha no registry" {
		t.Errorf("erro = %v; esperado \"falha no registry\"", err)
	}
}

func TestImageExiste(t *testing.T) {
	casos := []struct {
		nome     string
		status   int
		esperado bool
	}{
		{"imagem local", http.StatusOK, true},
		{"imagem ausente", http.StatusNotFound, false},
	}

	for _, c := range casos {
		t.Run(c.nome, func(t *testing.T) {
			cli := clienteFake(t, func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(c.status)
				w.Write([]byte(`{}`))
			})
			if got := cli.ImageExiste(context.Background(), "nginx:1.25"); got != c.esperado {
				t.Errorf("ImageExiste() = %v; esperado %v", got, c.esperado)
			}
		})
	}
}

// TestImageExisteEscapaReferencia: a barra de "library/nginx" nao pode virar
// separador de rota.
func TestImageExisteEscapaReferencia(t *testing.T) {
	var caminho string
	c := clienteFake(t, func(w http.ResponseWriter, r *http.Request) {
		caminho = r.URL.EscapedPath()
		w.Write([]byte(`{}`))
	})

	c.ImageExiste(context.Background(), "library/nginx:1.25")

	if caminho != "/images/library%2Fnginx:1.25/json" {
		t.Errorf("caminho = %q; esperado a barra escapada", caminho)
	}
}
