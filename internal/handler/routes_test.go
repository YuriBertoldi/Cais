package handler

import (
	"database/sql"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"cais/internal/compose"
	"cais/internal/models"
	"cais/internal/store"
)

// handlerCompleto monta o painel inteiro contra os templates e estaticos reais
// do repositorio, com banco descartavel e SEM cliente Docker.
//
// Docker nil e proposital: e o estado de quem acabou de instalar e ainda nao
// tem daemon. Toda pagina precisa renderizar assim mesmo, e nenhum teste toca
// o daemon nem chama o wsl.exe.
func handlerCompleto(t *testing.T) (*Handler, *sql.DB) {
	t.Helper()

	db, err := store.Open(filepath.Join(t.TempDir(), "painel.db"))
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	h, err := New(Deps{
		DB:         db,
		FS:         os.DirFS(raizDoRepo(t)),
		Porta:      portaTeste,
		Versao:     "testes",
		Log:        slog.New(slog.NewTextHandler(io.Discard, nil)),
		Docker:     nil,
		Compose:    compose.New(""),
		DockerHost: "tcp://127.0.0.1:2375",
		WSLDistro:  "Ubuntu-24.04",
	})
	if err != nil {
		t.Fatalf("handler.New: %v", err)
	}
	return h, db
}

// raizDoRepo devolve o diretorio que contem templates/ e static/.
func raizDoRepo(t *testing.T) string {
	t.Helper()

	raiz, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatalf("resolver raiz: %v", err)
	}
	if _, err := os.Stat(filepath.Join(raiz, "templates", "base.html")); err != nil {
		t.Fatalf("templates/base.html nao encontrado a partir de %s: %v", raiz, err)
	}
	return raiz
}

// TestNewParseiaTodosOsTemplates: um erro de sintaxe em qualquer .html da
// lista `pages` derruba o painel no startup. Falhar aqui e mais barato.
func TestNewParseiaTodosOsTemplates(t *testing.T) {
	h, _ := handlerCompleto(t)

	for _, p := range pages {
		if h.tmpls[p] == nil {
			t.Errorf("template %q nao foi parseado", p)
		}
	}
}

// TestPaginasRenderizam exercita cada pagina de ponta a ponta: rota,
// middlewares, consulta ao banco e execucao do template.
func TestPaginasRenderizam(t *testing.T) {
	h, db := handlerCompleto(t)

	// Dados para as telas nao ficarem todas no caminho vazio.
	if err := store.CreateStack(db, models.Stack{
		Nome:       "app-de-teste",
		CaminhoYML: filepath.Join(t.TempDir(), "nao-existe", "docker-compose.yml"),
	}); err != nil {
		t.Fatalf("CreateStack: %v", err)
	}
	if err := store.LogAction(db, models.Action{
		AlvoTipo: "container", AlvoNome: "web", Acao: "start", Resultado: "ok", DuracaoMS: 42,
	}); err != nil {
		t.Fatalf("LogAction: %v", err)
	}

	rotas := []string{
		"/", // sem daemon, a raiz serve a tela de diagnostico
		"/containers",
		"/containers/novo",
		"/containers/abc123/logs",
		"/stacks",
		"/images",
		"/volumes",
		"/acoes",
		"/configuracoes",
	}

	rt := h.Routes()
	for _, rota := range rotas {
		t.Run(rota, func(t *testing.T) {
			w := httptest.NewRecorder()
			rt.ServeHTTP(w, reqLocal(http.MethodGet, rota))

			if w.Code != http.StatusOK {
				t.Fatalf("status = %d; esperado 200. Corpo: %s", w.Code, w.Body.String())
			}
			if ct := w.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/html") {
				t.Errorf("Content-Type = %q; esperado text/html", ct)
			}
			corpo := w.Body.String()
			if !strings.Contains(corpo, "<!DOCTYPE") && !strings.Contains(corpo, "<!doctype") {
				t.Errorf("corpo nao parece uma pagina completa: %.120s", corpo)
			}
			// O template base nao pode deixar acao de escrita sem o token.
			if !strings.Contains(corpo, h.csrf) {
				t.Error("token CSRF ausente do HTML renderizado")
			}
		})
	}
}

// TestDashboardSemDaemonMostraDiagnostico: todo card do dashboard sairia
// zerado sem daemon, o que parece "nenhum container" em vez de "nao consegui
// perguntar". A raiz precisa servir a tela de diagnostico.
func TestDashboardSemDaemonMostraDiagnostico(t *testing.T) {
	h, _ := handlerCompleto(t) // Docker: nil
	w := httptest.NewRecorder()

	h.Routes().ServeHTTP(w, reqLocal(http.MethodGet, "/"))

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d; esperado 200. Corpo: %s", w.Code, w.Body.String())
	}
	corpo := w.Body.String()

	for _, trecho := range []string{
		"Sem conexão com o Docker",
		h.DockerHost, // o endpoint tentado
		h.WSLDistro,  // a distro configurada, usada nos comandos de verificacao
		"wsl -l -v",  // as verificacoes rapidas do template
	} {
		if !strings.Contains(corpo, trecho) {
			t.Errorf("tela de diagnostico sem %q", trecho)
		}
	}
	// Nao pode ser o dashboard disfarcado.
	if strings.Contains(corpo, "Últimas ações") {
		t.Error("a raiz renderizou o dashboard em vez do diagnostico")
	}
}

// TestDashboardRenderizaComDados executa o template do dashboard direto: com
// o desvio para a tela de diagnostico, nenhuma rota alcanca esse template sem
// um daemon de verdade, e sem isto ele deixaria de ser exercitado.
func TestDashboardRenderizaComDados(t *testing.T) {
	h, _ := handlerCompleto(t)
	w := httptest.NewRecorder()

	h.render(w, reqLocal(http.MethodGet, "/"), "dashboard", models.DashboardPage{
		BasePage: models.BasePage{
			Title: "Dashboard", Active: "dashboard", Accent: AccentPadrao,
			DockerOK: true, EngineVersao: "27.1.1",
		},
		ContainersRodando: 3,
		ContainersParados: 1,
		TotalImagens:      12,
		TotalVolumes:      4,
		EspacoImagens:     "2.1 GB",
		UltimasAcoes: []models.Action{
			{AlvoNome: "web", Acao: "start", Resultado: "ok", DuracaoMS: 120, CriadoEm: time.Now()},
		},
	})

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d; esperado 200. Corpo: %s", w.Code, w.Body.String())
	}
	if corpo := w.Body.String(); !strings.Contains(corpo, "2.1 GB") {
		t.Error("o dashboard nao exibiu o espaco consolidado de imagens")
	}
}

// TestRenderEmModoDevEhSeguroSobConcorrencia exercita o reparse a cada
// request: em modo dev o mapa de templates e trocado enquanto outras
// requisicoes o leem. Vale rodar com -race.
func TestRenderEmModoDevEhSeguroSobConcorrencia(t *testing.T) {
	h, _ := handlerCompleto(t)
	h.Dev = true

	rt := h.Routes()
	pronto := make(chan struct{})

	for range 8 {
		go func() {
			defer func() { pronto <- struct{}{} }()
			w := httptest.NewRecorder()
			rt.ServeHTTP(w, reqLocal(http.MethodGet, "/containers"))
			if w.Code != http.StatusOK {
				t.Errorf("status = %d; esperado 200", w.Code)
			}
		}()
	}
	for range 8 {
		<-pronto
	}
}

func TestRenderDeTemplateNaoRegistrado(t *testing.T) {
	h, _ := handlerCompleto(t)
	w := httptest.NewRecorder()

	h.render(w, reqLocal(http.MethodGet, "/"), "inexistente", nil)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("status = %d; esperado 500", w.Code)
	}
}

// TestRotasAplicamOsMiddlewares confere que a protecao nao ficou de fora do
// encadeamento montado em Routes.
func TestRotasAplicamOsMiddlewares(t *testing.T) {
	h, _ := handlerCompleto(t)
	rt := h.Routes()

	t.Run("host estranho e barrado", func(t *testing.T) {
		r := reqLocal(http.MethodGet, "/")
		r.Host = "evil.com"

		w := httptest.NewRecorder()
		rt.ServeHTTP(w, r)

		if w.Code != http.StatusForbidden {
			t.Errorf("status = %d; esperado 403", w.Code)
		}
	})

	t.Run("post sem csrf e barrado", func(t *testing.T) {
		w := httptest.NewRecorder()
		rt.ServeHTTP(w, reqLocal(http.MethodPost, "/containers/abc/start"))

		if w.Code != http.StatusForbidden {
			t.Errorf("status = %d; esperado 403", w.Code)
		}
	})

	t.Run("stream sem csrf e barrado", func(t *testing.T) {
		w := httptest.NewRecorder()
		rt.ServeHTTP(w, reqLocal(http.MethodGet, "/api/containers/abc/logs/stream"))

		if w.Code != http.StatusForbidden {
			t.Errorf("status = %d; esperado 403", w.Code)
		}
	})
}

// postForm monta um POST ja autenticado com o token do processo.
func postForm(h *Handler, caminho string, campos url.Values) *http.Request {
	if campos == nil {
		campos = url.Values{}
	}
	campos.Set("csrf", h.csrf)

	r := httptest.NewRequest(http.MethodPost, caminho, strings.NewReader(campos.Encode()))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	r.Host = "127.0.0.1:45678"
	r.RemoteAddr = "127.0.0.1:55555"
	return r
}

func TestSalvarAparencia(t *testing.T) {
	h, db := handlerCompleto(t)
	rt := h.Routes()

	w := httptest.NewRecorder()
	rt.ServeHTTP(w, postForm(h, "/configuracoes/aparencia", url.Values{"accent": {"#10b981"}}))

	if w.Code != http.StatusSeeOther {
		t.Fatalf("status = %d; esperado 303. Corpo: %s", w.Code, w.Body.String())
	}
	if got := store.GetSetting(db, "accent", AccentPadrao); got != "#10b981" {
		t.Errorf("accent gravado = %q; esperado #10b981", got)
	}
}

// TestSalvarAparenciaRejeitaCorInvalida: o valor vai para dentro de um bloco
// <style>, entao lixo precisa virar o padrao antes de ser persistido.
func TestSalvarAparenciaRejeitaCorInvalida(t *testing.T) {
	h, db := handlerCompleto(t)
	rt := h.Routes()

	w := httptest.NewRecorder()
	rt.ServeHTTP(w, postForm(h, "/configuracoes/aparencia", url.Values{
		"accent": {"#fff;} body{display:none"},
	}))

	if w.Code != http.StatusSeeOther {
		t.Fatalf("status = %d; esperado 303", w.Code)
	}
	if got := store.GetSetting(db, "accent", "vazio"); got != AccentPadrao {
		t.Errorf("accent gravado = %q; esperado o padrao %q", got, AccentPadrao)
	}
}

func TestAcaoDeContainerNaoSuportada(t *testing.T) {
	h, _ := handlerCompleto(t)
	w := httptest.NewRecorder()

	h.Routes().ServeHTTP(w, postForm(h, "/containers/abc/autodestruir", nil))

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d; esperado 400 (allow-list de acoes)", w.Code)
	}
}

func TestAcaoDeStackNaoSuportada(t *testing.T) {
	h, db := handlerCompleto(t)
	if err := store.CreateStack(db, models.Stack{Nome: "app", CaminhoYML: `C:\x\compose.yml`}); err != nil {
		t.Fatalf("CreateStack: %v", err)
	}
	stacks, _ := store.ListStacks(db)

	w := httptest.NewRecorder()
	h.Routes().ServeHTTP(w, postForm(h, "/stacks/"+itoa(stacks[0].ID)+"/explodir", nil))

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d; esperado 400", w.Code)
	}
}

func TestAcaoEmStackInexistente(t *testing.T) {
	h, _ := handlerCompleto(t)
	w := httptest.NewRecorder()

	h.Routes().ServeHTTP(w, postForm(h, "/stacks/999/up", nil))

	if w.Code != http.StatusNotFound {
		t.Errorf("status = %d; esperado 404", w.Code)
	}
}

// TestAcaoSemDockerRegistraFalhaEVolta: sem daemon, a acao nao pode explodir
// -- ela redireciona com a mensagem de erro.
func TestAcaoSemDockerRegistraFalhaEVolta(t *testing.T) {
	h, _ := handlerCompleto(t)
	w := httptest.NewRecorder()

	h.Routes().ServeHTTP(w, postForm(h, "/containers/abc/start", nil))

	if w.Code != http.StatusSeeOther {
		t.Fatalf("status = %d; esperado 303", w.Code)
	}
	if destino := w.Header().Get("Location"); destino != "/containers" {
		t.Errorf("Location = %q; esperado /containers", destino)
	}
}

func TestCriarStackSemCaminho(t *testing.T) {
	h, db := handlerCompleto(t)
	w := httptest.NewRecorder()

	h.Routes().ServeHTTP(w, postForm(h, "/stacks", url.Values{"caminho_yml": {"  "}}))

	if w.Code != http.StatusSeeOther {
		t.Fatalf("status = %d; esperado 303", w.Code)
	}
	stacks, _ := store.ListStacks(db)
	if len(stacks) != 0 {
		t.Errorf("stack foi cadastrada mesmo sem caminho: %+v", stacks)
	}
}

func TestCriarStackComArquivoInexistente(t *testing.T) {
	h, db := handlerCompleto(t)
	w := httptest.NewRecorder()

	ausente := filepath.Join(t.TempDir(), "nao-existe", "docker-compose.yml")
	h.Routes().ServeHTTP(w, postForm(h, "/stacks", url.Values{"caminho_yml": {ausente}}))

	if w.Code != http.StatusSeeOther {
		t.Fatalf("status = %d; esperado 303", w.Code)
	}
	stacks, _ := store.ListStacks(db)
	if len(stacks) != 0 {
		t.Errorf("stack com arquivo inexistente foi cadastrada: %+v", stacks)
	}
}

func TestExcluirStack(t *testing.T) {
	h, db := handlerCompleto(t)
	if err := store.CreateStack(db, models.Stack{Nome: "app", CaminhoYML: `C:\x\compose.yml`}); err != nil {
		t.Fatalf("CreateStack: %v", err)
	}
	stacks, _ := store.ListStacks(db)

	w := httptest.NewRecorder()
	h.Routes().ServeHTTP(w, postForm(h, "/stacks/"+itoa(stacks[0].ID)+"/delete", nil))

	if w.Code != http.StatusSeeOther {
		t.Fatalf("status = %d; esperado 303", w.Code)
	}
	restantes, _ := store.ListStacks(db)
	if len(restantes) != 0 {
		t.Errorf("sobraram %d stacks; esperado 0", len(restantes))
	}
}

func TestVolumeCriarSemNome(t *testing.T) {
	h, _ := handlerCompleto(t)
	w := httptest.NewRecorder()

	h.Routes().ServeHTTP(w, postForm(h, "/volumes/criar", url.Values{"nome": {"  "}}))

	if w.Code != http.StatusSeeOther {
		t.Fatalf("status = %d; esperado 303", w.Code)
	}
	if destino := w.Header().Get("Location"); destino != "/volumes" {
		t.Errorf("Location = %q; esperado /volumes", destino)
	}
}

func TestImagePullSemReferencia(t *testing.T) {
	h, _ := handlerCompleto(t)
	w := httptest.NewRecorder()

	h.Routes().ServeHTTP(w, postForm(h, "/images/pull", url.Values{"ref": {""}}))

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d; esperado 400", w.Code)
	}
}

func TestImagePullSemDocker(t *testing.T) {
	h, _ := handlerCompleto(t)
	w := httptest.NewRecorder()

	h.Routes().ServeHTTP(w, postForm(h, "/images/pull", url.Values{"ref": {"nginx"}}))

	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("status = %d; esperado 503", w.Code)
	}
}

func TestCriarContainerSemImagem(t *testing.T) {
	h, _ := handlerCompleto(t)
	w := httptest.NewRecorder()

	h.Routes().ServeHTTP(w, postForm(h, "/containers/criar", url.Values{"nome": {"x"}}))

	if w.Code != http.StatusSeeOther {
		t.Fatalf("status = %d; esperado 303", w.Code)
	}
	if destino := w.Header().Get("Location"); destino != "/containers/novo" {
		t.Errorf("Location = %q; esperado /containers/novo", destino)
	}
}

// ============================================================
// Favicon, estaticos e healthz
// ============================================================

// TestFaviconAcompanhaACor: o .svg e gerado porque a cor e escolhida pelo
// usuario; um arquivo fixo ficaria fora de sintonia com a interface.
func TestFaviconAcompanhaACor(t *testing.T) {
	h, db := handlerCompleto(t)
	if err := store.SetSetting(db, "accent", "#f59e0b"); err != nil {
		t.Fatalf("SetSetting: %v", err)
	}

	w := httptest.NewRecorder()
	h.Routes().ServeHTTP(w, reqLocal(http.MethodGet, "/favicon.svg"))

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d; esperado 200", w.Code)
	}
	if ct := w.Header().Get("Content-Type"); ct != "image/svg+xml" {
		t.Errorf("Content-Type = %q; esperado image/svg+xml", ct)
	}
	corpo := w.Body.String()
	if !strings.Contains(corpo, "#f59e0b") {
		t.Errorf("favicon nao usou a cor gravada: %s", corpo)
	}
	// Ambar e claro: o contraste precisa ser com texto escuro.
	if !strings.Contains(corpo, "#101828") {
		t.Errorf("favicon nao aplicou o contraste esperado: %s", corpo)
	}
}

func TestEstaticosRevalidamComETag(t *testing.T) {
	h, _ := handlerCompleto(t)
	rt := h.Routes()

	w := httptest.NewRecorder()
	rt.ServeHTTP(w, reqLocal(http.MethodGet, "/static/app.css"))

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d; esperado 200", w.Code)
	}
	etag := w.Header().Get("ETag")
	if etag != `"testes"` {
		t.Errorf("ETag = %q; esperado a versao do binario", etag)
	}
	if cc := w.Header().Get("Cache-Control"); cc != "no-cache" {
		t.Errorf("Cache-Control = %q; esperado no-cache", cc)
	}

	// Arquivos vindos de go:embed tem mtime zero; sem o ETag o navegador
	// continuaria servindo o CSS antigo depois da troca do .exe.
	w2 := httptest.NewRecorder()
	r2 := reqLocal(http.MethodGet, "/static/app.css")
	r2.Header.Set("If-None-Match", etag)
	rt.ServeHTTP(w2, r2)

	if w2.Code != http.StatusNotModified {
		t.Errorf("status com If-None-Match = %d; esperado 304", w2.Code)
	}
}

func TestEstaticoInexistente(t *testing.T) {
	h, _ := handlerCompleto(t)
	w := httptest.NewRecorder()

	h.Routes().ServeHTTP(w, reqLocal(http.MethodGet, "/static/nao-existe.css"))

	if w.Code != http.StatusNotFound {
		t.Errorf("status = %d; esperado 404", w.Code)
	}
}

// TestHealthz: e o que permite detectar instancia propria na porta antes
// mesmo de haver token CSRF, entao fica fora dos middlewares.
func TestHealthz(t *testing.T) {
	h, _ := handlerCompleto(t)
	w := httptest.NewRecorder()

	// Sem Host de loopback de proposito: o healthz nao passa pelos middlewares.
	h.Healthz(w, httptest.NewRequest(http.MethodGet, "/healthz", nil))

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d; esperado 200", w.Code)
	}
	if w.Body.String() != "ok" {
		t.Errorf("corpo = %q; esperado \"ok\"", w.Body.String())
	}
	if got := w.Header().Get("X-Cais"); got != "testes" {
		t.Errorf("X-Cais = %q; esperado a versao (e o que identifica a instancia)", got)
	}
}

func itoa(i int64) string { return strconv.FormatInt(i, 10) }
