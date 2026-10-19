// Package handler reune os controllers HTTP, o carregamento de templates e os
// middlewares de protecao do painel.
package handler

import (
	"bytes"
	"database/sql"
	"fmt"
	"html/template"
	"io/fs"
	"log/slog"
	"net/http"
	"sync"

	"cais/internal/compose"
	"cais/internal/docker"
)

// pages lista os templates de pagina. Cada um e parseado junto com base.html.
//
// Ao adicionar uma pagina nova, inclua o nome aqui.
var pages = []string{
	"dashboard",
	"containers",
	"container-novo",
	"logs",
	"stacks",
	"images",
	"volumes",
	"acoes",
	"configuracoes",
	"erro-docker",
}

// Deps reune tudo que os handlers precisam. Injetado em New.
type Deps struct {
	DB     *sql.DB
	FS     fs.FS // raiz contendo templates/ e static/
	Dev    bool
	Porta  int
	Versao string
	Log    *slog.Logger

	// Docker pode ser nil: o painel sobe mesmo sem daemon, mostrando a tela
	// de diagnostico em vez de recusar a abrir.
	Docker  *docker.Client
	Compose *compose.Runner

	DockerHost string
	WSLDistro  string
}

// Handler agrupa as dependencias e os templates ja parseados.
type Handler struct {
	Deps

	csrf string // token aleatorio, fixo por processo

	mu    sync.RWMutex
	tmpls map[string]*template.Template
}

// New monta o Handler e faz o parse inicial dos templates.
func New(d Deps) (*Handler, error) {
	if d.Log == nil {
		d.Log = slog.Default()
	}
	tok, err := tokenAleatorio()
	if err != nil {
		return nil, fmt.Errorf("gerar token CSRF: %w", err)
	}

	h := &Handler{Deps: d, csrf: tok}
	if err := h.carregarTemplates(); err != nil {
		return nil, err
	}
	return h, nil
}

// carregarTemplates parseia base.html + cada page em um template independente.
func (h *Handler) carregarTemplates() error {
	m := make(map[string]*template.Template, len(pages))
	for _, p := range pages {
		t, err := template.New("base.html").Funcs(funcMap).ParseFS(h.FS,
			"templates/base.html",
			"templates/"+p+".html",
		)
		if err != nil {
			return fmt.Errorf("parse do template %q: %w", p, err)
		}
		m[p] = t
	}

	h.mu.Lock()
	h.tmpls = m
	h.mu.Unlock()
	return nil
}

// render escreve uma pagina. Em modo dev os templates sao reparseados a cada
// request, para que editar um .html nao exija recompilar.
func (h *Handler) render(w http.ResponseWriter, _ *http.Request, page string, data any) {
	if h.Dev {
		if err := h.carregarTemplates(); err != nil {
			h.erroInterno(w, fmt.Errorf("recarregar templates: %w", err))
			return
		}
	}

	h.mu.RLock()
	t := h.tmpls[page]
	h.mu.RUnlock()

	if t == nil {
		h.erroInterno(w, fmt.Errorf("template %q nao registrado em pages", page))
		return
	}

	// Renderiza em buffer antes de escrever: se o template falhar no meio,
	// o cliente recebe um 500 limpo em vez de HTML truncado.
	var buf bytes.Buffer
	if err := t.ExecuteTemplate(&buf, "base", data); err != nil {
		h.erroInterno(w, fmt.Errorf("executar template %q: %w", page, err))
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	buf.WriteTo(w)
}

func (h *Handler) erroInterno(w http.ResponseWriter, err error) {
	h.Log.Error("erro ao renderizar", "erro", err)
	http.Error(w, "Erro interno do painel. Veja o log no console.", http.StatusInternalServerError)
}
