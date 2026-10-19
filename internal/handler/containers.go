package handler

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"cais/internal/docker"
	"cais/internal/models"
	"cais/internal/store"
)

// timeoutParada e quanto o daemon espera antes do SIGKILL.
const timeoutParada = 10

// HandleContainers lista os containers.
func (h *Handler) HandleContainers(w http.ResponseWriter, r *http.Request) {
	base := h.base(w, r, "containers", "Containers")
	h.preencherDaemon(r.Context(), &base)

	p := models.ContainersPage{BasePage: base}
	if base.DockerOK {
		cts, err := h.Docker.ContainerList(r.Context(), true)
		if err != nil {
			h.Log.Error("listar containers", "erro", err)
			p.Erro = err.Error()
		}
		p.Containers = cts
	}
	h.render(w, r, "containers", p)
}

// acoesContainer mapeia a acao da rota para a chamada da Engine API.
//
// Allow-list explicita: o painel nunca faz proxy cru para o daemon, entao
// qualquer operacao precisa estar nomeada aqui.
var acoesContainer = map[string]func(context.Context, *docker.Client, string) error{
	"start": func(c context.Context, d *docker.Client, id string) error { return d.ContainerStart(c, id) },
	"stop": func(c context.Context, d *docker.Client, id string) error {
		return d.ContainerStop(c, id, timeoutParada)
	},
	"restart": func(c context.Context, d *docker.Client, id string) error {
		return d.ContainerRestart(c, id, timeoutParada)
	},
	"pause":   func(c context.Context, d *docker.Client, id string) error { return d.ContainerPause(c, id) },
	"unpause": func(c context.Context, d *docker.Client, id string) error { return d.ContainerUnpause(c, id) },
	"remove": func(c context.Context, d *docker.Client, id string) error {
		return d.ContainerRemove(c, id, true, false)
	},
}

// HandleContainerAcao executa uma acao de ciclo de vida sobre um container.
func (h *Handler) HandleContainerAcao(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	acao := r.PathValue("acao")

	fn, ok := acoesContainer[acao]
	if !ok {
		http.Error(w, "Ação não suportada.", http.StatusBadRequest)
		return
	}
	if h.Docker == nil {
		setFlash(w, "erro", "Sem conexão com o Docker Engine.")
		http.Redirect(w, r, "/containers", http.StatusSeeOther)
		return
	}

	// O nome e so para o historico ficar legivel; se falhar, segue com o ID.
	nome := id
	if d, err := h.Docker.ContainerInspect(r.Context(), id); err == nil {
		nome = d.Nome()
	}

	h.executar(w, r, models.Action{
		AlvoTipo: "container",
		AlvoID:   id,
		AlvoNome: nome,
		Acao:     acao,
	}, "/containers", func(ctx context.Context) error {
		return fn(ctx, h.Docker, id)
	})
}

// executar roda uma operacao cronometrando, registrando no historico, montando
// a mensagem de resultado e redirecionando.
//
// Concentrar isso aqui mantem cada handler de acao com tres linhas e garante
// que nenhuma operacao escape do action_log.
func (h *Handler) executar(w http.ResponseWriter, r *http.Request, a models.Action, voltarPara string, fn func(context.Context) error) {
	inicio := time.Now()
	err := fn(r.Context())
	a.DuracaoMS = time.Since(inicio).Milliseconds()

	if err != nil {
		a.Resultado = "erro"
		a.Erro = err.Error()
		setFlash(w, "erro", fmt.Sprintf("Falha ao %s %q: %v", a.Acao, a.AlvoNome, err))
	} else {
		a.Resultado = "ok"
		setFlash(w, "sucesso", fmt.Sprintf("%s: %s concluído.", a.AlvoNome, a.Acao))
	}

	if errLog := store.LogAction(h.DB, a); errLog != nil {
		h.Log.Error("gravar no historico", "erro", errLog)
	}
	http.Redirect(w, r, voltarPara, http.StatusSeeOther)
}

// preencherDaemon consulta o daemon e preenche o indicador de conexao.
func (h *Handler) preencherDaemon(ctx context.Context, b *models.BasePage) {
	if h.Docker == nil {
		b.DockerErro = "Cliente Docker não configurado."
		return
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	v, err := h.Docker.Ping(ctx)
	if err != nil {
		b.DockerErro = err.Error()
		return
	}
	b.DockerOK = true
	b.EngineVersao = v.Version
}
