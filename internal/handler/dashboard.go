package handler

import (
	"net/http"

	"cais/internal/docker"
	"cais/internal/models"
	"cais/internal/store"
)

// HandleDashboard renderiza a pagina inicial com os cards consolidados.
//
// Sem daemon nao ha nada para consolidar -- todo card do dashboard sairia
// zerado, o que parece "nenhum container" em vez de "nao consegui perguntar".
// Nesse caso a pagina inicial vira a tela de diagnostico, que e justamente o
// que quem acabou de instalar precisa ver.
func (h *Handler) HandleDashboard(w http.ResponseWriter, r *http.Request) {
	base := h.base(w, r, "dashboard", "Dashboard")
	h.preencherDaemon(r.Context(), &base)

	if !base.DockerOK {
		h.renderErroDocker(w, r, base)
		return
	}

	p := models.DashboardPage{BasePage: base}

	acoes, err := store.ListActions(h.DB, 10)
	if err != nil {
		h.Log.Error("listar acoes", "erro", err)
	}
	p.UltimasAcoes = acoes

	// Daqui para baixo o daemon respondeu: o desvio acima ja tratou o contrario.
	if cts, err := h.Docker.ContainerList(r.Context(), true); err != nil {
		h.Log.Error("contar containers", "erro", err)
	} else {
		for _, c := range cts {
			if c.Rodando() {
				p.ContainersRodando++
			} else {
				p.ContainersParados++
			}
		}
	}

	if imgs, err := h.Docker.ImageList(r.Context()); err != nil {
		h.Log.Error("contar imagens", "erro", err)
	} else {
		p.TotalImagens = len(imgs)
		var soma int64
		for _, i := range imgs {
			soma += i.Size
		}
		p.EspacoImagens = docker.FormatarBytes(soma)
	}

	if vols, err := h.Docker.VolumeList(r.Context()); err != nil {
		h.Log.Error("contar volumes", "erro", err)
	} else {
		p.TotalVolumes = len(vols)
	}

	h.render(w, r, "dashboard", p)
}

// renderErroDocker mostra a tela de diagnostico no lugar do dashboard.
//
// A lista de diagnostico so afirma o que o painel de fato observou -- o
// endpoint tentado e a resposta recebida -- seguida das causas comuns em ordem
// de probabilidade. Os comandos de verificacao ficam no proprio template.
func (h *Handler) renderErroDocker(w http.ResponseWriter, r *http.Request, base models.BasePage) {
	base.Title = "Sem conexão com o Docker"

	erro := base.DockerErro
	if erro == "" {
		erro = "nenhuma mensagem devolvida."
	}

	h.render(w, r, "erro-docker", models.ErroDockerPage{
		BasePage: base,
		Host:     h.DockerHost,
		Distro:   h.WSLDistro,
		Diagnostico: []string{
			"O painel tentou consultar a versão do Engine em " + h.DockerHost + ".",
			"Resposta: " + erro,
			"Causa mais comum: a distro WSL está parada. O WSL desliga a VM depois de alguns minutos ociosa.",
			"Em seguida: o serviço docker está parado dentro da distro.",
			"Por último: o daemon está de pé, mas sem expor a porta TCP — o que não é o padrão do Docker Engine e exige o drop-in do systemd.",
		},
	})
}
