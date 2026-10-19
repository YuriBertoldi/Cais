package handler

import (
	"net/http"

	"cais/internal/docker"
	"cais/internal/models"
	"cais/internal/store"
)

// HandleDashboard renderiza a pagina inicial com os cards consolidados.
func (h *Handler) HandleDashboard(w http.ResponseWriter, r *http.Request) {
	base := h.base(w, r, "dashboard", "Dashboard")
	h.preencherDaemon(r.Context(), &base)

	p := models.DashboardPage{BasePage: base}

	acoes, err := store.ListActions(h.DB, 10)
	if err != nil {
		h.Log.Error("listar acoes", "erro", err)
	}
	p.UltimasAcoes = acoes

	if base.DockerOK {
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
	}

	h.render(w, r, "dashboard", p)
}
