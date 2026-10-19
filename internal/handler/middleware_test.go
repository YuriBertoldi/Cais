package handler

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

const portaTeste = 45678

// handlerMinimo monta um Handler sem banco, sem Docker e sem templates --
// suficiente para exercitar os middlewares isoladamente.
func handlerMinimo() *Handler {
	return &Handler{
		Deps: Deps{
			Porta: portaTeste,
			Log:   slog.New(slog.NewTextHandler(io.Discard, nil)),
		},
		csrf: "token-de-teste",
	}
}

// okHandler marca que a requisicao passou pelo middleware.
func okHandler(passou *bool) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		*passou = true
		w.WriteHeader(http.StatusOK)
	})
}

// reqLocal monta uma requisicao que ja chega do loopback, como a do navegador
// na maquina do usuario. O httptest.NewRequest usa 192.0.2.1 por padrao, que
// seria barrado pela checagem de origem.
func reqLocal(metodo, alvo string) *http.Request {
	r := httptest.NewRequest(metodo, alvo, nil)
	r.Host = "127.0.0.1:45678"
	r.RemoteAddr = "127.0.0.1:55555"
	return r
}

// ============================================================
// LocalhostOnly
// ============================================================

// TestLocalhostOnlyHost cobre a defesa contra DNS rebinding: um dominio do
// atacante que resolva para 127.0.0.1 chega aqui com o Host dele.
func TestLocalhostOnlyHost(t *testing.T) {
	casos := []struct {
		nome   string
		host   string
		aceita bool
	}{
		{"loopback ipv4", "127.0.0.1:45678", true},
		{"nome localhost", "localhost:45678", true},
		{"loopback ipv6", "[::1]:45678", true},
		{"dominio do atacante", "evil.com", false},
		{"dominio com a porta certa", "evil.com:45678", false},
		{"loopback na porta errada", "127.0.0.1:1234", false},
		{"loopback sem porta", "127.0.0.1", false},
		{"host vazio", "", false},
		{"outro ip local", "192.168.0.10:45678", false},
		{"subdominio de localhost", "a.localhost:45678", false},
	}

	for _, c := range casos {
		t.Run(c.nome, func(t *testing.T) {
			h := handlerMinimo()
			var passou bool
			w := httptest.NewRecorder()

			r := reqLocal(http.MethodGet, "/")
			r.Host = c.host

			h.LocalhostOnly(okHandler(&passou)).ServeHTTP(w, r)

			if passou != c.aceita {
				t.Errorf("Host %q: passou = %v; esperado %v", c.host, passou, c.aceita)
			}
			if !c.aceita && w.Code != http.StatusForbidden {
				t.Errorf("Host %q: status = %d; esperado 403", c.host, w.Code)
			}
		})
	}
}

// TestLocalhostOnlyRemoteAddr e defesa em profundidade: o listener ja escuta
// so em loopback, mas um proxy mal configurado poderia furar isso.
func TestLocalhostOnlyRemoteAddr(t *testing.T) {
	casos := []struct {
		nome   string
		remote string
		aceita bool
	}{
		{"loopback ipv4", "127.0.0.1:55555", true},
		{"loopback ipv6", "[::1]:55555", true},
		{"outro endereco de loopback", "127.0.0.5:55555", true},
		{"rede local", "192.168.0.10:55555", false},
		{"internet", "203.0.113.7:55555", false},
	}

	for _, c := range casos {
		t.Run(c.nome, func(t *testing.T) {
			h := handlerMinimo()
			var passou bool
			w := httptest.NewRecorder()

			r := reqLocal(http.MethodGet, "/")
			r.RemoteAddr = c.remote

			h.LocalhostOnly(okHandler(&passou)).ServeHTTP(w, r)

			if passou != c.aceita {
				t.Errorf("RemoteAddr %q: passou = %v; esperado %v", c.remote, passou, c.aceita)
			}
		})
	}
}

// TestLocalhostOnlyOrigin impede que uma aba qualquer dispare requests contra
// o painel.
func TestLocalhostOnlyOrigin(t *testing.T) {
	casos := []struct {
		nome   string
		origin string
		aceita bool
	}{
		{"sem origin", "", true},
		{"origin do proprio painel", "http://127.0.0.1:45678", true},
		{"origin localhost", "http://localhost:45678", true},
		{"origin ipv6", "http://[::1]:45678", true},
		{"origin de outro site", "http://evil.com", false},
		{"origin https de outro site", "https://evil.com", false},
		{"origin na porta errada", "http://127.0.0.1:3000", false},
		{"origin null", "null", false},
		{"origin malformada", "://", false},
	}

	for _, c := range casos {
		t.Run(c.nome, func(t *testing.T) {
			h := handlerMinimo()
			var passou bool
			w := httptest.NewRecorder()

			r := reqLocal(http.MethodPost, "/containers/abc/start")
			if c.origin != "" {
				r.Header.Set("Origin", c.origin)
			}

			h.LocalhostOnly(okHandler(&passou)).ServeHTTP(w, r)

			if passou != c.aceita {
				t.Errorf("Origin %q: passou = %v; esperado %v", c.origin, passou, c.aceita)
			}
		})
	}
}

// TestLocalhostOnlyAcompanhaAPorta: quando a porta preferida esta ocupada o
// painel sobe numa porta dinamica, e a allow-list precisa seguir junto.
func TestLocalhostOnlyAcompanhaAPorta(t *testing.T) {
	h := handlerMinimo()
	h.Porta = 51234

	var passou bool
	r := reqLocal(http.MethodGet, "/")
	r.Host = "127.0.0.1:51234"
	h.LocalhostOnly(okHandler(&passou)).ServeHTTP(httptest.NewRecorder(), r)
	if !passou {
		t.Error("Host com a porta dinamica deveria ser aceito")
	}

	passou = false
	r2 := reqLocal(http.MethodGet, "/")
	r2.Host = "127.0.0.1:45678"
	h.LocalhostOnly(okHandler(&passou)).ServeHTTP(httptest.NewRecorder(), r2)
	if passou {
		t.Error("Host com a porta antiga nao deveria ser aceito")
	}
}

// ============================================================
// CSRF
// ============================================================

func TestCSRFLiberaLeitura(t *testing.T) {
	for _, metodo := range []string{http.MethodGet, http.MethodHead, http.MethodOptions} {
		h := handlerMinimo()
		var passou bool

		h.CSRF(okHandler(&passou)).ServeHTTP(httptest.NewRecorder(), reqLocal(metodo, "/containers"))

		if !passou {
			t.Errorf("%s em pagina de leitura deveria passar sem token", metodo)
		}
	}
}

func TestCSRFExigeTokenEmEscrita(t *testing.T) {
	casos := []struct {
		nome   string
		ajusta func(*http.Request)
		aceita bool
	}{
		{"sem token", func(*http.Request) {}, false},
		{
			"token no cabecalho",
			func(r *http.Request) { r.Header.Set("X-CSRF-Token", "token-de-teste") },
			true,
		},
		{
			"token na query",
			func(r *http.Request) { r.URL.RawQuery = "csrf=token-de-teste" },
			true,
		},
		{
			"token no formulario",
			func(r *http.Request) {
				r.Body = io.NopCloser(strings.NewReader("csrf=token-de-teste&nome=x"))
				r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
			},
			true,
		},
		{
			"token errado",
			func(r *http.Request) { r.Header.Set("X-CSRF-Token", "outro-token") },
			false,
		},
		{
			"token vazio",
			func(r *http.Request) { r.Header.Set("X-CSRF-Token", "") },
			false,
		},
		{
			"prefixo do token nao basta",
			func(r *http.Request) { r.Header.Set("X-CSRF-Token", "token") },
			false,
		},
	}

	for _, c := range casos {
		t.Run(c.nome, func(t *testing.T) {
			h := handlerMinimo()
			var passou bool
			w := httptest.NewRecorder()

			r := reqLocal(http.MethodPost, "/containers/abc/start")
			c.ajusta(r)

			h.CSRF(okHandler(&passou)).ServeHTTP(w, r)

			if passou != c.aceita {
				t.Errorf("passou = %v; esperado %v", passou, c.aceita)
			}
			if !c.aceita && w.Code != http.StatusForbidden {
				t.Errorf("status = %d; esperado 403", w.Code)
			}
		})
	}
}

// TestCSRFExigeTokenEmStreams: EventSource e WebSocket nao mandam cabecalho
// proprio, entao o token viaja na query string -- e um GET sob /api/ ou /ws/
// nao pode ser tratado como leitura livre.
func TestCSRFExigeTokenEmStreams(t *testing.T) {
	casos := []struct {
		nome   string
		alvo   string
		aceita bool
	}{
		{"stream de api sem token", "/api/containers/abc/logs/stream", false},
		{"stream de api com token", "/api/containers/abc/logs/stream?csrf=token-de-teste", true},
		{"stream de websocket sem token", "/ws/qualquer", false},
		{"stream de websocket com token", "/ws/qualquer?csrf=token-de-teste", true},
		{"pagina normal segue livre", "/containers", true},
		{"stream com token errado", "/api/x?csrf=errado", false},
	}

	for _, c := range casos {
		t.Run(c.nome, func(t *testing.T) {
			h := handlerMinimo()
			var passou bool

			h.CSRF(okHandler(&passou)).ServeHTTP(httptest.NewRecorder(), reqLocal(http.MethodGet, c.alvo))

			if passou != c.aceita {
				t.Errorf("GET %s: passou = %v; esperado %v", c.alvo, passou, c.aceita)
			}
		})
	}
}

func TestCSRFPreferenciaDeOrigemDoToken(t *testing.T) {
	// Query tem precedencia sobre o cabecalho; basta uma das fontes estar certa.
	h := handlerMinimo()
	var passou bool

	r := reqLocal(http.MethodPost, "/volumes/criar?csrf=token-de-teste")
	r.Header.Set("X-CSRF-Token", "lixo")

	h.CSRF(okHandler(&passou)).ServeHTTP(httptest.NewRecorder(), r)

	if !passou {
		t.Error("token valido na query deveria bastar")
	}
}

// ============================================================
// Recover
// ============================================================

func TestRecoverNaoDerrubaOProcesso(t *testing.T) {
	h := handlerMinimo()
	w := httptest.NewRecorder()

	panico := http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		panic("algo explodiu")
	})

	h.Recover(panico).ServeHTTP(w, reqLocal(http.MethodGet, "/"))

	if w.Code != http.StatusInternalServerError {
		t.Errorf("status = %d; esperado 500", w.Code)
	}
	if !strings.Contains(w.Body.String(), "Erro interno") {
		t.Errorf("corpo = %q; esperado mensagem de erro interno", w.Body.String())
	}
}

func TestRecoverDeixaPassarRequisicaoNormal(t *testing.T) {
	h := handlerMinimo()
	var passou bool
	w := httptest.NewRecorder()

	h.Recover(okHandler(&passou)).ServeHTTP(w, reqLocal(http.MethodGet, "/"))

	if !passou || w.Code != http.StatusOK {
		t.Errorf("passou = %v, status = %d; esperado true/200", passou, w.Code)
	}
}

// ============================================================
// Flash
// ============================================================

func TestFlashIdaEVolta(t *testing.T) {
	w := httptest.NewRecorder()
	setFlash(w, "sucesso", "Container web: start concluído.")

	cookies := w.Result().Cookies()
	if len(cookies) != 1 {
		t.Fatalf("cookies gravados = %d; esperado 1", len(cookies))
	}
	c := cookies[0]
	if c.Name != cookieFlash {
		t.Errorf("nome do cookie = %q; esperado %q", c.Name, cookieFlash)
	}
	if !c.HttpOnly {
		t.Error("cookie de flash deveria ser HttpOnly")
	}
	if c.SameSite != http.SameSiteStrictMode {
		t.Errorf("SameSite = %v; esperado Strict", c.SameSite)
	}

	r := reqLocal(http.MethodGet, "/")
	r.AddCookie(c)

	f := lerFlash(httptest.NewRecorder(), r)
	if f.Tipo != "sucesso" || f.Msg != "Container web: start concluído." {
		t.Errorf("flash lida = %+v", f)
	}
	if f.Vazia() {
		t.Error("flash com mensagem nao deveria ser considerada vazia")
	}
}

// TestFlashEConsumida: a mensagem nao deve sobreviver ao proximo reload.
func TestFlashEConsumida(t *testing.T) {
	grav := httptest.NewRecorder()
	setFlash(grav, "erro", "falhou")

	r := reqLocal(http.MethodGet, "/")
	r.AddCookie(grav.Result().Cookies()[0])

	w := httptest.NewRecorder()
	lerFlash(w, r)

	cookies := w.Result().Cookies()
	if len(cookies) != 1 {
		t.Fatalf("esperado 1 cookie de expiracao; obtidos %d", len(cookies))
	}
	if cookies[0].MaxAge >= 0 {
		t.Errorf("MaxAge = %d; esperado negativo para apagar o cookie", cookies[0].MaxAge)
	}
}

func TestFlashInvalidaOuAusente(t *testing.T) {
	casos := []struct {
		nome      string
		valor     string
		temCookie bool
	}{
		{"sem cookie", "", false},
		{"cookie vazio", "", true},
		{"base64 invalido", "!!!nao-e-base64!!!", true},
		{"conteudo sem separador", "c2VtLXNlcGFyYWRvcg==", true}, // "sem-separador"
	}

	for _, c := range casos {
		t.Run(c.nome, func(t *testing.T) {
			r := reqLocal(http.MethodGet, "/")
			if c.temCookie {
				r.AddCookie(&http.Cookie{Name: cookieFlash, Value: c.valor})
			}

			f := lerFlash(httptest.NewRecorder(), r)

			if !f.Vazia() {
				t.Errorf("flash = %+v; esperado vazia", f)
			}
		})
	}
}

func TestFlashComPipeNaMensagem(t *testing.T) {
	// A mensagem pode conter "|" (vem de erro do daemon); so o primeiro
	// separador conta.
	grav := httptest.NewRecorder()
	setFlash(grav, "erro", "compose falhou | linha 2 | linha 3")

	r := reqLocal(http.MethodGet, "/")
	r.AddCookie(grav.Result().Cookies()[0])

	f := lerFlash(httptest.NewRecorder(), r)
	if f.Tipo != "erro" || f.Msg != "compose falhou | linha 2 | linha 3" {
		t.Errorf("flash = %+v", f)
	}
}

func TestFlashSobreviveACodificacaoDeURL(t *testing.T) {
	// base64.URLEncoding produz "-" e "_", mas nunca caracteres que o
	// http.Cookie precisaria escapar.
	grav := httptest.NewRecorder()
	setFlash(grav, "aviso", "acentuação, çedilha e ~til")

	valor := grav.Result().Cookies()[0].Value
	if _, err := url.QueryUnescape(valor); err != nil {
		t.Fatalf("valor do cookie nao sobrevive a decodificacao: %v", err)
	}

	r := reqLocal(http.MethodGet, "/")
	r.AddCookie(grav.Result().Cookies()[0])

	if f := lerFlash(httptest.NewRecorder(), r); f.Msg != "acentuação, çedilha e ~til" {
		t.Errorf("mensagem = %q; acentuacao foi perdida", f.Msg)
	}
}

// ============================================================
// Token e helpers de template
// ============================================================

func TestTokenAleatorio(t *testing.T) {
	a, err := tokenAleatorio()
	if err != nil {
		t.Fatalf("tokenAleatorio: %v", err)
	}
	if len(a) != 64 {
		t.Errorf("len = %d; esperado 64 (32 bytes em hex)", len(a))
	}

	b, err := tokenAleatorio()
	if err != nil {
		t.Fatalf("tokenAleatorio: %v", err)
	}
	if a == b {
		t.Error("dois tokens consecutivos sairam iguais")
	}
}

func TestFuncMapHora(t *testing.T) {
	hora := funcMap["hora"].(func(time.Time) string)

	if got := hora(time.Time{}); got != "-" {
		t.Errorf("hora(zero) = %q; esperado \"-\"", got)
	}

	d := time.Date(2026, 10, 19, 14, 5, 9, 0, time.Local)
	if got := hora(d); got != "19/10/2026 14:05:09" {
		t.Errorf("hora(%v) = %q; esperado 19/10/2026 14:05:09", d, got)
	}
}

func TestFuncMapDuracao(t *testing.T) {
	duracao := funcMap["duracao"].(func(int64) string)

	casos := []struct {
		ms       int64
		esperado string
	}{
		{0, "0 ms"},
		{1, "1 ms"},
		{999, "999 ms"},
		{1000, "1.0 s"},
		{1500, "1.5 s"},
		{65000, "65.0 s"},
	}

	for _, c := range casos {
		if got := duracao(c.ms); got != c.esperado {
			t.Errorf("duracao(%d) = %q; esperado %q", c.ms, got, c.esperado)
		}
	}
}

// TestFuncMapIDCurto: a Engine API devolve o id com o prefixo do algoritmo,
// mas na tela interessa o mesmo recorte que o `docker images` mostra.
func TestFuncMapIDCurto(t *testing.T) {
	idcurto := funcMap["idcurto"].(func(string) string)

	casos := []struct{ entrada, esperado string }{
		{"sha256:abcdef0123456789abcdef", "abcdef012345"},
		{"abcdef0123456789", "abcdef012345"},
		{"sha256:abc", "abc"},
		{"abc", "abc"},
		{"", ""},
	}

	for _, c := range casos {
		if got := idcurto(c.entrada); got != c.esperado {
			t.Errorf("idcurto(%q) = %q; esperado %q", c.entrada, got, c.esperado)
		}
	}
}

func TestFuncMapBytes(t *testing.T) {
	bytes := funcMap["bytes"].(func(int64) string)

	if got := bytes(1536); got != "1.5 KB" {
		t.Errorf("bytes(1536) = %q; esperado 1.5 KB", got)
	}
}
