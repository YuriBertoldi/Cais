package docker

import (
	"fmt"
	"sort"
)

// Structs minimas da Engine API: so os campos que o painel exibe.
//
// Replicar a modelagem completa do SDK seria milhares de linhas para ganhar
// campos que nenhuma tela usa.

// Version e o retorno de GET /version.
type Version struct {
	Version       string `json:"Version"`
	APIVersion    string `json:"ApiVersion"`
	MinAPIVersion string `json:"MinAPIVersion"`
	Os            string `json:"Os"`
	Arch          string `json:"Arch"`
}

// Port e uma porta publicada de um container.
type Port struct {
	IP          string `json:"IP"`
	PrivatePort int    `json:"PrivatePort"`
	PublicPort  int    `json:"PublicPort"`
	Type        string `json:"Type"`
}

// Container e um item de GET /containers/json.
type Container struct {
	ID      string            `json:"Id"`
	Names   []string          `json:"Names"`
	Image   string            `json:"Image"`
	State   string            `json:"State"`  // running | exited | paused | created | restarting | dead
	Status  string            `json:"Status"` // texto legivel: "Up 2 minutes"
	Created int64             `json:"Created"`
	Ports   []Port            `json:"Ports"`
	Labels  map[string]string `json:"Labels"`
}

// Nome devolve o nome principal, sem a barra inicial que a API usa.
func (c Container) Nome() string {
	if len(c.Names) == 0 {
		return c.IDCurto()
	}
	return trimBarra(c.Names[0])
}

// IDCurto devolve os 12 primeiros caracteres do ID, como o docker ps mostra.
func (c Container) IDCurto() string {
	if len(c.ID) > 12 {
		return c.ID[:12]
	}
	return c.ID
}

// Projeto devolve a stack compose de origem, quando houver.
func (c Container) Projeto() string { return c.Labels["com.docker.compose.project"] }

// Servico devolve o nome do servico dentro da stack compose, quando houver.
func (c Container) Servico() string { return c.Labels["com.docker.compose.service"] }

// Rodando informa se o container esta em execucao.
func (c Container) Rodando() bool { return c.State == "running" }

// PortasPublicadas devolve os mapeamentos legiveis, sem repeticao.
//
// A Engine API lista uma entrada por familia de endereco: um bind em
// 0.0.0.0 e outro em [::] para a mesma porta. Exibir as duas faria toda
// porta aparecer duplicada na tela.
func (c Container) PortasPublicadas() []string {
	vistos := make(map[string]bool, len(c.Ports))
	var out []string
	for _, p := range c.Ports {
		if p.PublicPort == 0 {
			continue // porta exposta mas nao publicada no host
		}
		s := fmt.Sprintf("%d→%d/%s", p.PublicPort, p.PrivatePort, p.Type)
		if vistos[s] {
			continue
		}
		vistos[s] = true
		out = append(out, s)
	}
	sort.Strings(out)
	return out
}

// LinkPorta e uma porta publicada pronta para a tela.
type LinkPorta struct {
	Rotulo string
	URL    string
	Web    bool // true quando vale oferecer o link clicavel
}

// portasNaoWeb sao servicos comuns que respondem em TCP mas nao falam HTTP.
// Oferecer link para eles so geraria aba de erro.
var portasNaoWeb = map[int]bool{
	5432: true, 3306: true, 1433: true, 27017: true, 6379: true,
	5672: true, 9092: true, 22: true, 25: true, 1521: true,
}

// LinksPublicados devolve as portas publicadas, marcando quais valem como
// link HTTP para abrir o servico direto do painel.
func (c Container) LinksPublicados() []LinkPorta {
	vistos := make(map[string]bool, len(c.Ports))
	var out []LinkPorta
	for _, p := range c.Ports {
		if p.PublicPort == 0 || p.Type != "tcp" {
			continue
		}
		rotulo := fmt.Sprintf("%d→%d/%s", p.PublicPort, p.PrivatePort, p.Type)
		if vistos[rotulo] {
			continue
		}
		vistos[rotulo] = true

		// O criterio e a porta INTERNA: e ela que diz qual servico roda ali.
		web := !portasNaoWeb[p.PrivatePort]
		l := LinkPorta{Rotulo: rotulo, Web: web}
		if web {
			l.URL = fmt.Sprintf("http://localhost:%d/", p.PublicPort)
		}
		out = append(out, l)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Rotulo < out[j].Rotulo })
	return out
}

// ContainerDetail e o recorte de GET /containers/{id}/json que o painel usa.
type ContainerDetail struct {
	ID      string `json:"Id"`
	Name    string `json:"Name"`
	Created string `json:"Created"`
	State   struct {
		Status     string `json:"Status"`
		Running    bool   `json:"Running"`
		Paused     bool   `json:"Paused"`
		ExitCode   int    `json:"ExitCode"`
		StartedAt  string `json:"StartedAt"`
		FinishedAt string `json:"FinishedAt"`
		Health     *struct {
			Status string `json:"Status"`
		} `json:"Health"`
	} `json:"State"`
	Config struct {
		Image  string            `json:"Image"`
		Tty    bool              `json:"Tty"`
		Env    []string          `json:"Env"`
		Labels map[string]string `json:"Labels"`
		Cmd    []string          `json:"Cmd"`
	} `json:"Config"`
	Mounts []struct {
		Type        string `json:"Type"`
		Name        string `json:"Name"`
		Source      string `json:"Source"`
		Destination string `json:"Destination"`
		RW          bool   `json:"RW"`
	} `json:"Mounts"`
	NetworkSettings struct {
		Networks map[string]struct {
			IPAddress string `json:"IPAddress"`
		} `json:"Networks"`
	} `json:"NetworkSettings"`
}

// Nome devolve o nome sem a barra inicial.
func (d ContainerDetail) Nome() string { return trimBarra(d.Name) }

// Image e um item de GET /images/json.
type Image struct {
	ID         string   `json:"Id"`
	RepoTags   []string `json:"RepoTags"`
	Size       int64    `json:"Size"`
	Created    int64    `json:"Created"`
	Containers int64    `json:"Containers"`
}

// Tag devolve a primeira tag legivel, ou <none> para imagens orfas.
func (i Image) Tag() string {
	for _, t := range i.RepoTags {
		if t != "" && t != "<none>:<none>" {
			return t
		}
	}
	return "<none>"
}

// Volume e um item de GET /volumes.
type Volume struct {
	Name       string `json:"Name"`
	Driver     string `json:"Driver"`
	Mountpoint string `json:"Mountpoint"`
	CreatedAt  string `json:"CreatedAt"`
	UsageData  *struct {
		Size     int64 `json:"Size"`
		RefCount int64 `json:"RefCount"`
	} `json:"UsageData"`
}

// listaVolumes e o envelope de GET /volumes.
type listaVolumes struct {
	Volumes []Volume `json:"Volumes"`
}

// DiskUsage e o recorte de GET /system/df.
type DiskUsage struct {
	LayersSize int64 `json:"LayersSize"`
	Images     []struct {
		Size int64 `json:"Size"`
	} `json:"Images"`
	Volumes []struct {
		UsageData *struct {
			Size int64 `json:"Size"`
		} `json:"UsageData"`
	} `json:"Volumes"`
	BuildCache []struct {
		Size  int64 `json:"Size"`
		InUse bool  `json:"InUse"`
	} `json:"BuildCache"`
}

func trimBarra(s string) string {
	if len(s) > 0 && s[0] == '/' {
		return s[1:]
	}
	return s
}
