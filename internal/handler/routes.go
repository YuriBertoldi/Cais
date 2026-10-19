package handler

import (
	"fmt"
	"io/fs"
	"net/http"

	"cais/internal/models"
	"cais/internal/store"
)

// Routes monta o mux e encadeia os middlewares.
//
// A ordem importa: Recover por fora de tudo, LocalhostOnly antes do CSRF
// (nao faz sentido validar token de quem nem deveria alcancar o painel).
func (h *Handler) Routes() http.Handler {
	mux := http.NewServeMux()

	// Estaticos. Servidos do embed em producao, do disco em modo dev.
	sub, err := fs.Sub(h.FS, "static")
	if err != nil {
		panic("subdiretorio static ausente: " + err.Error())
	}
	mux.Handle("GET /static/", http.StripPrefix("/static/", h.cacheEstaticos(http.FileServerFS(sub))))

	// Paginas
	mux.HandleFunc("GET /favicon.svg", h.HandleFavicon)
	mux.HandleFunc("GET /{$}", h.HandleDashboard)
	mux.HandleFunc("GET /containers", h.HandleContainers)
	mux.HandleFunc("GET /containers/novo", h.HandleContainerNovo)
	mux.HandleFunc("GET /containers/{id}/logs", h.HandleLogs)
	mux.HandleFunc("GET /stacks", h.HandleStacks)
	mux.HandleFunc("GET /images", h.HandleImages)
	mux.HandleFunc("GET /volumes", h.HandleVolumes)
	mux.HandleFunc("GET /acoes", h.HandleAcoes)
	mux.HandleFunc("GET /configuracoes", h.HandleConfiguracoes)
	mux.HandleFunc("POST /configuracoes/aparencia", h.HandleSalvarAparencia)

	// Acoes de ciclo de vida. A acao vem na rota e e validada contra a
	// allow-list em acoesContainer -- nada de proxy generico para o daemon.
	mux.HandleFunc("POST /containers/{id}/{acao}", h.HandleContainerAcao)

	mux.HandleFunc("POST /containers/criar", h.HandleContainerCriar)
	mux.HandleFunc("POST /images/pull", h.HandleImagePull)
	mux.HandleFunc("POST /volumes/criar", h.HandleVolumeCriar)

	// Stacks compose. A rota de delete vem antes da generica de acao: o
	// ServeMux do Go 1.22+ resolve por especificidade, nao por ordem, mas
	// manter juntas deixa a intencao explicita.
	mux.HandleFunc("POST /stacks", h.HandleCriarStack)
	mux.HandleFunc("POST /stacks/{id}/delete", h.HandleExcluirStack)
	mux.HandleFunc("POST /stacks/{id}/{acao}", h.HandleStackAcao)

	// Recursos
	mux.HandleFunc("POST /images/{id}/remove", h.HandleImageRemove)
	mux.HandleFunc("POST /volumes/{nome}/remove", h.HandleVolumeRemove)

	// Streams. Ficam sob /api/ para o middleware de CSRF exigir o token
	// tambem aqui -- EventSource e WebSocket nao mandam cabecalho proprio,
	// entao o token viaja na query string.
	mux.HandleFunc("GET /api/containers/{id}/logs/stream", h.HandleLogsStream)

	return h.Recover(h.LocalhostOnly(h.CSRF(mux)))
}

// cacheEstaticos faz o navegador revalidar os estaticos a cada carga.
//
// Arquivos vindos de go:embed tem mtime zero, entao o net/http nao emite
// Last-Modified nem ETag. Sem nenhum validador, o navegador cacheia por
// heuristica propria e continua servindo o CSS antigo depois que o usuario
// troca o .exe por uma versao nova -- exatamente o tipo de bug que faz
// parecer que a atualizacao nao foi aplicada.
//
// "no-cache" nao significa "nao armazene": significa "revalide antes de usar".
// Com o ETag abaixo, a revalidacao devolve 304 enquanto a versao nao muda.
func (h *Handler) cacheEstaticos(next http.Handler) http.Handler {
	etag := `"` + h.Versao + `"`
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("If-None-Match") == etag {
			w.WriteHeader(http.StatusNotModified)
			return
		}
		w.Header().Set("ETag", etag)
		w.Header().Set("Cache-Control", "no-cache")
		next.ServeHTTP(w, r)
	})
}

// faviconSVG e a marca em tamanho de aba, com a cor de destaque aplicada.
//
// E gerado em vez de servido como arquivo estatico porque a cor e escolhida
// pelo usuario; um .svg fixo ficaria fora de sintonia com o resto da interface.
const faviconSVG = `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24">
<rect width="24" height="24" rx="6" fill="%s"/>
<g fill="%s">
<path d="M12 5.2 18.6 8.7 12 12.2 5.4 8.7Z" opacity="1"/>
<path d="M4.9 9.4 11.5 12.9v6.5L4.9 15.9Z" opacity=".62"/>
<path d="M19.1 9.4 12.5 12.9v6.5l6.6-3.5Z" opacity=".82"/>
</g></svg>`

// HandleFavicon serve a marca como favicon SVG.
func (h *Handler) HandleFavicon(w http.ResponseWriter, _ *http.Request) {
	accent := CorValida(store.GetSetting(h.DB, chaveAccent, AccentPadrao))
	svg := fmt.Sprintf(faviconSVG, accent, TextoSobre(accent))

	w.Header().Set("Content-Type", "image/svg+xml")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("ETag", `"`+accent+`"`)
	w.Write([]byte(svg))
}

// Healthz fica fora dos middlewares: e consultado pela propria aplicacao para
// detectar se ja existe uma instancia rodando na porta padrao.
func (h *Handler) Healthz(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("X-Cais", h.Versao)
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("ok"))
}

// base monta a parte comum de toda page data.
func (h *Handler) base(w http.ResponseWriter, r *http.Request, active, title string) models.BasePage {
	accent := CorValida(store.GetSetting(h.DB, chaveAccent, AccentPadrao))
	return models.BasePage{
		Title:        title,
		Active:       active,
		Flash:        lerFlash(w, r),
		CSRF:         h.csrf,
		Versao:       h.Versao,
		Accent:       accent,
		AccentEscuro: Escurecer(accent, 0.80),
		AccentTexto:  TextoSobre(accent),
	}
}
