package handler

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"cais/internal/docker"
	"cais/internal/models"
)

// HandleLogs renderiza o visualizador de log de um container.
func (h *Handler) HandleLogs(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	base := h.base(w, r, "containers", "Logs")
	h.preencherDaemon(r.Context(), &base)

	p := models.LogsPage{BasePage: base, ID: id, Nome: id}
	if base.DockerOK {
		if d, err := h.Docker.ContainerInspect(r.Context(), id); err == nil {
			p.Nome = d.Nome()
			p.Rodando = d.State.Running
			p.Imagem = d.Config.Image
		} else {
			p.Erro = err.Error()
		}
	}
	p.Title = "Logs — " + p.Nome
	h.render(w, r, "logs", p)
}

// eventoLog e um pedaco de log ja pronto para o navegador.
type eventoLog struct {
	Fluxo string `json:"f"` // out | err
	Texto string `json:"t"`
}

// HandleLogsStream transmite o log de um container por SSE.
//
// SSE e nao WebSocket porque o fluxo e de mao unica: so o servidor fala. O
// EventSource do navegador ainda reconecta sozinho se a conexao cair, o que
// seria codigo manual num WebSocket.
func (h *Handler) HandleLogsStream(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming não suportado", http.StatusInternalServerError)
		return
	}
	if h.Docker == nil {
		http.Error(w, "sem conexão com o Docker", http.StatusServiceUnavailable)
		return
	}

	// Saber se o container tem TTY decide se o stream vem multiplexado.
	det, err := h.Docker.ContainerInspect(r.Context(), id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}

	tail := r.URL.Query().Get("tail")
	if tail == "" {
		tail = "200"
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	// Impede buffering por proxy intermediario; sem isto o log chega em
	// blocos grandes em vez de linha a linha.
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)
	flusher.Flush()

	ctx := r.Context()
	corpo, err := h.Docker.Logs(ctx, id, docker.LogOptions{
		Follow:     true,
		Timestamps: r.URL.Query().Get("timestamps") == "1",
		Tail:       tail,
	})
	if err != nil {
		enviarSSE(w, flusher, "erro", err.Error())
		return
	}
	defer corpo.Close()

	// Heartbeat: mantem a conexao viva em containers silenciosos e permite
	// ao servidor perceber que o cliente sumiu.
	pararPing := make(chan struct{})
	defer close(pararPing)
	linhas := make(chan eventoLog, 256)

	go func() {
		defer close(linhas)
		_ = docker.DemuxFrames(corpo, det.Config.Tty, func(f docker.Fluxo, p []byte) error {
			for _, l := range strings.Split(strings.TrimRight(string(p), "\n"), "\n") {
				select {
				case linhas <- eventoLog{Fluxo: f.Nome(), Texto: l}:
				case <-ctx.Done():
					return ctx.Err()
				}
			}
			return nil
		})
	}()

	ticker := time.NewTicker(20 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			// Comentario SSE: ignorado pelo EventSource, mas trafega bytes.
			fmt.Fprint(w, ": ping\n\n")
			flusher.Flush()
		case ev, aberto := <-linhas:
			if !aberto {
				enviarSSE(w, flusher, "fim", "")
				return
			}
			// Serializado como JSON de UMA linha: um \n cru no meio do
			// payload encerraria o evento SSE no lugar errado.
			b, err := json.Marshal(ev)
			if err != nil {
				continue
			}
			fmt.Fprintf(w, "data: %s\n\n", b)
			flusher.Flush()
		}
	}
}

func enviarSSE(w http.ResponseWriter, f http.Flusher, evento, dado string) {
	fmt.Fprintf(w, "event: %s\ndata: %s\n\n", evento, dado)
	f.Flush()
}
