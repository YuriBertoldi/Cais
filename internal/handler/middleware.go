package handler

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"html/template"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"cais/internal/docker"
	"cais/internal/models"
)

// funcMap reune os helpers disponiveis nos templates.
var funcMap = template.FuncMap{
	"hora": func(t time.Time) string {
		if t.IsZero() {
			return "-"
		}
		return t.Format("02/01/2006 15:04:05")
	},
	"duracao": func(ms int64) string {
		if ms < 1000 {
			return fmt.Sprintf("%d ms", ms)
		}
		return fmt.Sprintf("%.1f s", float64(ms)/1000)
	},
	"bytes": docker.FormatarBytes,
	// A Engine API devolve o id com o prefixo do algoritmo ("sha256:abc...").
	// Na tela interessa o mesmo recorte que o `docker images` mostra.
	"idcurto": func(id string) string {
		s := strings.TrimPrefix(id, "sha256:")
		if len(s) > 12 {
			return s[:12]
		}
		return s
	},
}

func tokenAleatorio() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// ============================================================
// Middlewares
// ============================================================

// LocalhostOnly rejeita requests cujo Host nao seja o loopback na porta do
// painel, e requests com Origin de outra origem.
//
// Isso fecha duas portas de uma vez. O Host mata DNS rebinding: um dominio
// controlado pelo atacante que resolva para 127.0.0.1 chegaria aqui com
// Host "evil.com" e e barrado. O Origin impede que uma aba qualquer dispare
// requests contra o painel.
func (h *Handler) LocalhostOnly(next http.Handler) http.Handler {
	permitidos := map[string]bool{
		fmt.Sprintf("127.0.0.1:%d", h.Porta): true,
		fmt.Sprintf("localhost:%d", h.Porta): true,
		fmt.Sprintf("[::1]:%d", h.Porta):     true,
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !permitidos[r.Host] {
			h.Log.Warn("request com Host nao permitido", "host", r.Host, "remote", r.RemoteAddr)
			http.Error(w, "Host nao permitido.", http.StatusForbidden)
			return
		}

		// Defesa em profundidade: o listener ja escuta so em loopback, mas um
		// proxy mal configurado poderia furar isso.
		if ip, _, err := net.SplitHostPort(r.RemoteAddr); err == nil {
			if addr := net.ParseIP(ip); addr != nil && !addr.IsLoopback() {
				h.Log.Warn("request de origem nao-loopback", "remote", r.RemoteAddr)
				http.Error(w, "Acesso permitido apenas localmente.", http.StatusForbidden)
				return
			}
		}

		if o := r.Header.Get("Origin"); o != "" {
			u, err := url.Parse(o)
			if err != nil || !permitidos[u.Host] {
				h.Log.Warn("request com Origin nao permitida", "origin", o)
				http.Error(w, "Origin nao permitida.", http.StatusForbidden)
				return
			}
		}

		next.ServeHTTP(w, r)
	})
}

// CSRF exige o token do processo em toda requisicao que nao seja de leitura,
// e tambem nos streams (SSE/WebSocket), onde o token vai na query string.
func (h *Handler) CSRF(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		leitura := r.Method == http.MethodGet || r.Method == http.MethodHead || r.Method == http.MethodOptions
		stream := strings.HasPrefix(r.URL.Path, "/api/") || strings.HasPrefix(r.URL.Path, "/ws/")

		if leitura && !stream {
			next.ServeHTTP(w, r)
			return
		}

		informado := r.URL.Query().Get("csrf")
		if informado == "" {
			informado = r.Header.Get("X-CSRF-Token")
		}
		if informado == "" {
			// FormValue consome o corpo; so chamamos quando realmente preciso.
			informado = r.FormValue("csrf")
		}

		if subtle.ConstantTimeCompare([]byte(informado), []byte(h.csrf)) != 1 {
			h.Log.Warn("token CSRF invalido", "path", r.URL.Path, "metodo", r.Method)
			http.Error(w, "Token de seguranca invalido. Recarregue a pagina.", http.StatusForbidden)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// Recover evita que um panic em um handler derrube o processo inteiro.
func (h *Handler) Recover(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if v := recover(); v != nil {
				h.Log.Error("panic em handler", "path", r.URL.Path, "valor", v)
				http.Error(w, "Erro interno do painel.", http.StatusInternalServerError)
			}
		}()
		next.ServeHTTP(w, r)
	})
}

// ============================================================
// Flash (mensagem de resultado da ultima acao)
// ============================================================

const cookieFlash = "painel_flash"

func setFlash(w http.ResponseWriter, tipo, msg string) {
	v := base64.URLEncoding.EncodeToString([]byte(tipo + "|" + msg))
	http.SetCookie(w, &http.Cookie{
		Name:     cookieFlash,
		Value:    v,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteStrictMode,
		MaxAge:   30,
	})
}

func lerFlash(w http.ResponseWriter, r *http.Request) models.Flash {
	c, err := r.Cookie(cookieFlash)
	if err != nil || c.Value == "" {
		return models.Flash{}
	}
	// Consome: a mensagem nao deve sobreviver ao proximo reload.
	http.SetCookie(w, &http.Cookie{Name: cookieFlash, Value: "", Path: "/", MaxAge: -1})

	b, err := base64.URLEncoding.DecodeString(c.Value)
	if err != nil {
		return models.Flash{}
	}
	tipo, msg, ok := strings.Cut(string(b), "|")
	if !ok {
		return models.Flash{}
	}
	return models.Flash{Tipo: tipo, Msg: msg}
}
