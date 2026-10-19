package handler

import (
	"net/http"

	"cais/internal/config"
	"cais/internal/models"
	"cais/internal/store"
)

// HandleConfiguracoes mostra as preferencias do painel.
func (h *Handler) HandleConfiguracoes(w http.ResponseWriter, r *http.Request) {
	base := h.base(w, r, "configuracoes", "Configurações")
	h.preencherDaemon(r.Context(), &base)

	atual := base.Accent
	opcoes := make([]models.CorOpcao, 0, len(Paleta))
	for _, c := range Paleta {
		opcoes = append(opcoes, models.CorOpcao{
			Nome:        c.Nome,
			Hex:         c.Hex,
			Selecionada: c.Hex == atual,
		})
	}

	h.render(w, r, "configuracoes", models.ConfiguracoesPage{
		BasePage:   base,
		Paleta:     opcoes,
		DockerHost: h.DockerHost,
		WSLDistro:  h.WSLDistro,
		CaminhoDB:  config.CaminhoDBPadrao(),
	})
}

// HandleSalvarAparencia grava a cor de destaque.
func (h *Handler) HandleSalvarAparencia(w http.ResponseWriter, r *http.Request) {
	// CorValida descarta qualquer coisa fora de #RRGGBB. A validacao e de
	// seguranca: o valor vai parar dentro de um bloco <style>.
	cor := CorValida(r.FormValue("accent"))

	if err := store.SetSetting(h.DB, chaveAccent, cor); err != nil {
		h.Log.Error("salvar cor de destaque", "erro", err)
		setFlash(w, "erro", "Não foi possível salvar a cor.")
	} else {
		setFlash(w, "sucesso", "Cor de destaque atualizada.")
	}
	http.Redirect(w, r, "/configuracoes", http.StatusSeeOther)
}
