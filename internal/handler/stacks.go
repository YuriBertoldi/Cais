package handler

import (
	"context"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"cais/internal/models"
	"cais/internal/store"
)

// HandleStacks lista as stacks compose cadastradas e o estado de cada uma.
func (h *Handler) HandleStacks(w http.ResponseWriter, r *http.Request) {
	base := h.base(w, r, "stacks", "Stacks")
	h.preencherDaemon(r.Context(), &base)

	p := models.StacksPage{BasePage: base}

	stacks, err := store.ListStacks(h.DB)
	if err != nil {
		h.Log.Error("listar stacks", "erro", err)
		p.Erro = err.Error()
		h.render(w, r, "stacks", p)
		return
	}

	for _, s := range stacks {
		item := models.StackItem{Stack: s}

		// Arquivo apagado ou movido depois do cadastro e o caso mais comum
		// de "stack que nao sobe"; e barato detectar e dizer isso na tela.
		if _, err := os.Stat(s.CaminhoYML); err != nil {
			item.Ausente = true
		} else if base.DockerOK {
			servicos, err := h.Compose.Ps(r.Context(), s.CaminhoYML, s.ProjectName)
			if err != nil {
				item.Erro = err.Error()
			} else {
				item.Servicos = servicos
				for _, sv := range servicos {
					if sv.Rodando() {
						item.Rodando++
					}
				}
			}
		}
		p.Stacks = append(p.Stacks, item)
	}

	h.render(w, r, "stacks", p)
}

// HandleCriarStack cadastra uma stack a partir do caminho de um compose.
func (h *Handler) HandleCriarStack(w http.ResponseWriter, r *http.Request) {
	caminho := strings.TrimSpace(r.FormValue("caminho_yml"))
	nome := strings.TrimSpace(r.FormValue("nome"))
	projeto := strings.TrimSpace(r.FormValue("project_name"))

	if caminho == "" {
		setFlash(w, "erro", "Informe o caminho do docker-compose.yml.")
		http.Redirect(w, r, "/stacks", http.StatusSeeOther)
		return
	}
	if _, err := os.Stat(caminho); err != nil {
		setFlash(w, "erro", "Arquivo não encontrado: "+caminho)
		http.Redirect(w, r, "/stacks", http.StatusSeeOther)
		return
	}
	if nome == "" {
		// Sem nome informado, o da pasta e o rotulo mais util.
		nome = filepath.Base(filepath.Dir(caminho))
	}

	// Valida de verdade: se o compose nao consegue ler o arquivo, cadastrar
	// so adiaria o erro para a hora do "subir".
	if _, err := h.Compose.Validar(r.Context(), caminho); err != nil {
		setFlash(w, "erro", "Compose inválido: "+err.Error())
		http.Redirect(w, r, "/stacks", http.StatusSeeOther)
		return
	}

	err := store.CreateStack(h.DB, models.Stack{
		Nome:        nome,
		CaminhoYML:  caminho,
		ProjectName: projeto,
	})
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE") {
			setFlash(w, "erro", "Essa stack já está cadastrada.")
		} else {
			h.Log.Error("cadastrar stack", "erro", err)
			setFlash(w, "erro", "Não foi possível cadastrar: "+err.Error())
		}
	} else {
		setFlash(w, "sucesso", "Stack "+nome+" cadastrada.")
	}
	http.Redirect(w, r, "/stacks", http.StatusSeeOther)
}

// HandleExcluirStack remove o cadastro, sem tocar nos containers.
func (h *Handler) HandleExcluirStack(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err := store.DeleteStack(h.DB, id); err != nil {
		setFlash(w, "erro", "Não foi possível remover o cadastro.")
	} else {
		setFlash(w, "sucesso", "Cadastro removido. Os containers não foram alterados.")
	}
	http.Redirect(w, r, "/stacks", http.StatusSeeOther)
}

// HandleStackAcao executa up, down, restart ou pull sobre uma stack.
func (h *Handler) HandleStackAcao(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(r.PathValue("id"), 10, 64)
	acao := r.PathValue("acao")

	s, err := store.GetStack(h.DB, id)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	var fn func(context.Context) error
	switch acao {
	case "up":
		rebuild := r.FormValue("rebuild") != ""
		fn = func(ctx context.Context) error {
			_, e := h.Compose.Up(ctx, s.CaminhoYML, s.ProjectName, rebuild)
			return e
		}
	case "down":
		fn = func(ctx context.Context) error {
			_, e := h.Compose.Down(ctx, s.CaminhoYML, s.ProjectName, false)
			return e
		}
	case "restart":
		fn = func(ctx context.Context) error { _, e := h.Compose.Restart(ctx, s.CaminhoYML, s.ProjectName); return e }
	case "pull":
		fn = func(ctx context.Context) error { _, e := h.Compose.Pull(ctx, s.CaminhoYML, s.ProjectName); return e }
	default:
		http.Error(w, "Ação não suportada.", http.StatusBadRequest)
		return
	}

	h.executar(w, r, models.Action{
		AlvoTipo: "stack",
		AlvoID:   strconv.FormatInt(s.ID, 10),
		AlvoNome: s.Nome,
		Acao:     acao,
	}, "/stacks", fn)
}
