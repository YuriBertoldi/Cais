// Package models reune as entidades persistidas e os dados de pagina
// consumidos pelos templates.
package models

import (
	"time"

	"cais/internal/compose"
	"cais/internal/docker"
)

// ============================================================
// Entidades persistidas
// ============================================================

// Stack e um docker-compose.yml registrado pelo usuario no painel.
type Stack struct {
	ID            int64
	Nome          string
	CaminhoYML    string // caminho Windows, ex: C:\Sistemas\git\projeto\docker-compose.yml
	ProjectName   string // --project-name; vazio usa o default do compose
	EnvFile       string // opcional
	PastaTrabalho string // --project-directory; vazio usa a pasta do YML
	Ordem         int
	CriadoEm      time.Time
}

// Favorito marca um recurso para aparecer no topo das listagens.
type Favorito struct {
	ID       int64
	Tipo     string // container | image | volume | stack
	Ref      string // id ou nome do recurso
	CriadoEm time.Time
}

// Action e um registro do historico de operacoes executadas pelo painel.
type Action struct {
	ID        int64
	AlvoTipo  string // container | image | volume | stack | system
	AlvoID    string
	AlvoNome  string
	Acao      string // start | stop | remove | prune | up | down | ...
	Resultado string // ok | erro
	Erro      string
	DuracaoMS int64
	CriadoEm  time.Time
}

// ============================================================
// Dados de pagina
// ============================================================

// BasePage e embutida em toda page data e alimenta o template base.
//
// Nao ha CurrentUser: o painel nao tem login. A protecao e de rede (bind em
// loopback) somada a validacao de Host/Origin e ao token CSRF.
type BasePage struct {
	Title        string
	Active       string // item do menu em destaque
	Flash        Flash
	CSRF         string
	DockerOK     bool
	DockerErro   string
	EngineVersao string
	Versao       string

	// Cor de destaque escolhida pelo usuario, e as duas derivadas dela.
	// Todas ja validadas como #RRGGBB antes de chegar ao template.
	Accent       string
	AccentEscuro string
	AccentTexto  string
}

// Flash e a mensagem de resultado da ultima acao, lida de um cookie.
type Flash struct {
	Tipo string // sucesso | erro | aviso
	Msg  string
}

// Vazia informa se nao ha mensagem a exibir.
func (f Flash) Vazia() bool { return f.Msg == "" }

// DashboardPage alimenta templates/dashboard.html.
type DashboardPage struct {
	BasePage
	ContainersRodando int
	ContainersParados int
	TotalImagens      int
	TotalVolumes      int
	EspacoImagens     string
	UltimasAcoes      []Action
}

// ErroDockerPage alimenta templates/erro-docker.html quando o ping ao daemon falha.
type ErroDockerPage struct {
	BasePage
	Host        string
	Distro      string
	Diagnostico []string
}

// ContainersPage alimenta templates/containers.html.
type ContainersPage struct {
	BasePage
	Containers []docker.Container
	Erro       string
}

// ConfiguracoesPage alimenta templates/configuracoes.html.
type ConfiguracoesPage struct {
	BasePage
	Paleta     []CorOpcao
	DockerHost string
	WSLDistro  string
	CaminhoDB  string
}

// CorOpcao e uma cor da paleta, ja sabendo se e a selecionada.
type CorOpcao struct {
	Nome        string
	Hex         string
	Selecionada bool
}

// StackItem e uma stack cadastrada somada ao seu estado atual.
type StackItem struct {
	Stack
	Servicos []compose.Servico
	Rodando  int
	Ausente  bool // o docker-compose.yml nao existe mais no caminho
	Erro     string
}

// Total devolve a quantidade de servicos conhecidos da stack.
func (s StackItem) Total() int { return len(s.Servicos) }

// Ativa informa se ha ao menos um servico de pe.
func (s StackItem) Ativa() bool { return s.Rodando > 0 }

// StacksPage alimenta templates/stacks.html.
type StacksPage struct {
	BasePage
	Stacks []StackItem
	Erro   string
}

// ImagesPage alimenta templates/images.html.
type ImagesPage struct {
	BasePage
	Imagens    []docker.Image
	TotalBytes int64
	Erro       string
}

// VolumesPage alimenta templates/volumes.html.
type VolumesPage struct {
	BasePage
	Volumes []docker.Volume
	Erro    string
}

// NovoContainerPage alimenta templates/container-novo.html.
type NovoContainerPage struct {
	BasePage
	Imagens         []string // imagens locais, para o datalist
	Volumes         []string // volumes existentes, para sugestão
	ImagemEscolhida string   // pré-preenchida quando veio da tela de imagens
}

// LogsPage alimenta templates/logs.html.
type LogsPage struct {
	BasePage
	ID      string
	Nome    string
	Imagem  string
	Rodando bool
	Erro    string
}

// AcoesPage alimenta templates/acoes.html.
type AcoesPage struct {
	BasePage
	Acoes []Action
}
