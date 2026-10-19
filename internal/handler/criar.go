package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"cais/internal/docker"
	"cais/internal/models"
	"cais/internal/store"
)

// ============================================================
// Volume
// ============================================================

// HandleVolumeCriar cria um volume nomeado.
func (h *Handler) HandleVolumeCriar(w http.ResponseWriter, r *http.Request) {
	nome := strings.TrimSpace(r.FormValue("nome"))
	if nome == "" {
		setFlash(w, "erro", "Informe o nome do volume.")
		http.Redirect(w, r, "/volumes", http.StatusSeeOther)
		return
	}

	h.executar(w, r, models.Action{
		AlvoTipo: "volume", AlvoID: nome, AlvoNome: nome, Acao: "create",
	}, "/volumes", func(ctx context.Context) error {
		_, err := h.Docker.VolumeCreate(ctx, nome, strings.TrimSpace(r.FormValue("driver")))
		return err
	})
}

// ============================================================
// Imagem
// ============================================================

// HandleImagePull baixa uma imagem transmitindo o progresso.
//
// E um POST lido por fetch() + ReadableStream, nao SSE: o EventSource do
// navegador so faz GET, e baixar imagem muda estado -- nao cabe num GET.
// A resposta sai em NDJSON, uma linha de progresso por vez.
func (h *Handler) HandleImagePull(w http.ResponseWriter, r *http.Request) {
	ref := strings.TrimSpace(r.FormValue("ref"))
	if ref == "" {
		http.Error(w, "informe a imagem", http.StatusBadRequest)
		return
	}
	if h.Docker == nil {
		http.Error(w, "sem conexão com o Docker", http.StatusServiceUnavailable)
		return
	}
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming não suportado", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/x-ndjson")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)
	flusher.Flush()

	enviar := func(v any) {
		b, _ := json.Marshal(v)
		fmt.Fprintf(w, "%s\n", b)
		flusher.Flush()
	}

	inicio := time.Now()
	err := h.Docker.ImagePull(r.Context(), ref, func(p docker.ProgressoPull) {
		enviar(map[string]string{
			"status": p.Status,
			"id":     p.ID,
			"prog":   p.Progresso,
		})
	})

	acao := models.Action{
		AlvoTipo: "image", AlvoID: ref, AlvoNome: ref, Acao: "pull",
		DuracaoMS: time.Since(inicio).Milliseconds(),
	}
	if err != nil {
		acao.Resultado, acao.Erro = "erro", err.Error()
		enviar(map[string]string{"erro": err.Error()})
	} else {
		acao.Resultado = "ok"
		enviar(map[string]string{"fim": ref})
	}
	if e := store.LogAction(h.DB, acao); e != nil {
		h.Log.Error("gravar historico do pull", "erro", e)
	}
}

// ============================================================
// Container
// ============================================================

// HandleContainerNovo mostra o formulario de criacao.
func (h *Handler) HandleContainerNovo(w http.ResponseWriter, r *http.Request) {
	base := h.base(w, r, "containers", "Novo container")
	h.preencherDaemon(r.Context(), &base)

	p := models.NovoContainerPage{BasePage: base}
	if base.DockerOK {
		if imgs, err := h.Docker.ImageList(r.Context()); err == nil {
			for _, i := range imgs {
				if t := i.Tag(); t != "<none>" {
					p.Imagens = append(p.Imagens, t)
				}
			}
		}
		if vols, err := h.Docker.VolumeList(r.Context()); err == nil {
			for _, v := range vols {
				p.Volumes = append(p.Volumes, v.Name)
			}
		}
	}
	// Pre-preenche a imagem quando o usuario veio da tela de imagens.
	p.ImagemEscolhida = r.URL.Query().Get("imagem")
	h.render(w, r, "container-novo", p)
}

// HandleContainerCriar cria e inicia o container.
func (h *Handler) HandleContainerCriar(w http.ResponseWriter, r *http.Request) {
	if h.Docker == nil {
		setFlash(w, "erro", "Sem conexão com o Docker Engine.")
		http.Redirect(w, r, "/containers/novo", http.StatusSeeOther)
		return
	}

	n := docker.NovoContainer{
		Nome:    strings.TrimSpace(r.FormValue("nome")),
		Imagem:  strings.TrimSpace(r.FormValue("imagem")),
		Restart: r.FormValue("restart"),
		Env:     linhasNaoVazias(r.FormValue("env")),
		Volumes: linhasNaoVazias(r.FormValue("volumes")),
		Cmd:     strings.Fields(r.FormValue("cmd")),
	}
	if n.Imagem == "" {
		setFlash(w, "erro", "Informe a imagem.")
		http.Redirect(w, r, "/containers/novo", http.StatusSeeOther)
		return
	}

	portas, err := parsePortas(r.FormValue("portas"))
	if err != nil {
		setFlash(w, "erro", err.Error())
		http.Redirect(w, r, "/containers/novo", http.StatusSeeOther)
		return
	}
	n.Portas = portas

	rotulo := n.Nome
	if rotulo == "" {
		rotulo = n.Imagem
	}

	h.executar(w, r, models.Action{
		AlvoTipo: "container", AlvoNome: rotulo, Acao: "create",
	}, "/containers", func(ctx context.Context) error {
		id, err := h.Docker.ContainerCreate(ctx, n)
		if err != nil {
			return err
		}
		// Criar sem iniciar deixaria o usuario com um container "created"
		// sem entender por que nao subiu. Iniciar aqui e o esperado.
		if err := h.Docker.ContainerStart(ctx, id); err != nil {
			return fmt.Errorf("container criado, mas não iniciou: %w", err)
		}
		return nil
	})
}

// parsePortas interpreta uma porta por linha, no formato "host:container".
//
// Aceita tambem "8080" sozinho, assumindo a mesma porta dos dois lados, e
// sufixo de protocolo em "53:53/udp".
func parsePortas(texto string) ([]docker.PortaMapeada, error) {
	var out []docker.PortaMapeada
	for _, linha := range linhasNaoVazias(texto) {
		proto := "tcp"
		if i := strings.LastIndex(linha, "/"); i >= 0 {
			proto = strings.ToLower(strings.TrimSpace(linha[i+1:]))
			linha = linha[:i]
			if proto != "tcp" && proto != "udp" {
				return nil, fmt.Errorf("protocolo inválido em %q: use tcp ou udp", linha)
			}
		}

		host, cont, achou := strings.Cut(linha, ":")
		host = strings.TrimSpace(host)
		if !achou {
			cont = host
		}
		cont = strings.TrimSpace(cont)

		if !somenteDigitos(host) || !somenteDigitos(cont) {
			return nil, fmt.Errorf("porta inválida: %q (use host:container, ex. 8080:80)", linha)
		}
		out = append(out, docker.PortaMapeada{Host: host, Container: cont, Protocolo: proto})
	}
	return out, nil
}

func somenteDigitos(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

// linhasNaoVazias quebra um textarea em linhas, descartando as em branco.
func linhasNaoVazias(s string) []string {
	var out []string
	for _, l := range strings.Split(s, "\n") {
		if l = strings.TrimSpace(l); l != "" {
			out = append(out, l)
		}
	}
	return out
}
