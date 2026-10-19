package handler

import (
	"context"
	"net/http"
	"strings"

	"cais/internal/models"
	"cais/internal/store"
)

// ============================================================
// Imagens
// ============================================================

// HandleImages lista as imagens locais.
func (h *Handler) HandleImages(w http.ResponseWriter, r *http.Request) {
	base := h.base(w, r, "images", "Imagens")
	h.preencherDaemon(r.Context(), &base)

	p := models.ImagesPage{BasePage: base}
	if base.DockerOK {
		imgs, err := h.Docker.ImageList(r.Context())
		if err != nil {
			h.Log.Error("listar imagens", "erro", err)
			p.Erro = err.Error()
		} else {
			p.Imagens = imgs
			for _, i := range imgs {
				p.TotalBytes += i.Size
			}
		}
	}
	h.render(w, r, "images", p)
}

// HandleImageRemove remove uma imagem.
func (h *Handler) HandleImageRemove(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	nome := strings.TrimSpace(r.FormValue("nome"))
	if nome == "" {
		nome = id
	}

	h.executar(w, r, models.Action{
		AlvoTipo: "image", AlvoID: id, AlvoNome: nome, Acao: "remove",
	}, "/images", func(ctx context.Context) error {
		// force=true porque imagem com mais de uma tag exige o flag; sem ele
		// o daemon recusa com "image is referenced in multiple repositories".
		return h.Docker.ImageRemove(ctx, id, true)
	})
}

// ============================================================
// Volumes
// ============================================================

// HandleVolumes lista os volumes.
func (h *Handler) HandleVolumes(w http.ResponseWriter, r *http.Request) {
	base := h.base(w, r, "volumes", "Volumes")
	h.preencherDaemon(r.Context(), &base)

	p := models.VolumesPage{BasePage: base}
	if base.DockerOK {
		vols, err := h.Docker.VolumeList(r.Context())
		if err != nil {
			h.Log.Error("listar volumes", "erro", err)
			p.Erro = err.Error()
		} else {
			p.Volumes = vols
		}
	}
	h.render(w, r, "volumes", p)
}

// HandleVolumeRemove remove um volume.
//
// Operacao destrutiva e irreversivel: o conteudo do volume e apagado.
func (h *Handler) HandleVolumeRemove(w http.ResponseWriter, r *http.Request) {
	nome := r.PathValue("nome")

	h.executar(w, r, models.Action{
		AlvoTipo: "volume", AlvoID: nome, AlvoNome: nome, Acao: "remove",
	}, "/volumes", func(ctx context.Context) error {
		// force=false de proposito: se o volume estiver em uso, o daemon
		// recusa, e essa recusa e uma protecao, nao um obstaculo.
		return h.Docker.VolumeRemove(ctx, nome, false)
	})
}

// ============================================================
// Historico
// ============================================================

// HandleAcoes mostra o historico completo de operacoes do painel.
func (h *Handler) HandleAcoes(w http.ResponseWriter, r *http.Request) {
	base := h.base(w, r, "acoes", "Histórico")
	h.preencherDaemon(r.Context(), &base)

	acoes, err := store.ListActions(h.DB, 500)
	if err != nil {
		h.Log.Error("listar historico", "erro", err)
	}
	h.render(w, r, "acoes", models.AcoesPage{BasePage: base, Acoes: acoes})
}
