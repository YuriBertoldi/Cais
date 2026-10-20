// Comando cais: painel web local para gerenciar o Docker, servido em
// loopback e aberto no navegador padrao.
//
// Substitui a interface do Docker Desktop, cujo uso exige assinatura paga em
// empresas acima dos limites do Docker Subscription Service Agreement. O motor
// fica a cargo do Docker Engine (Apache 2.0) instalado no WSL2.
package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"syscall"
	"time"

	"cais/internal/compose"
	"cais/internal/config"
	"cais/internal/docker"
	"cais/internal/handler"
	"cais/internal/store"
)

// versao e sobrescrita no build via -ldflags "-X main.versao=...".
var versao = "dev"

func main() {
	if err := run(); err != nil {
		slog.Error("falha ao iniciar o painel", "erro", err)
		// O executavel distribuido e compilado com -H=windowsgui e nao tem
		// console: sem este aviso, um duplo-clique que falha nao mostraria
		// absolutamente nada ao usuario.
		avisarFalhaFatal(err)
		os.Exit(1)
	}
}

func run() error {
	cfg := config.Load()

	destino, fecharLog := destinoDoLog(cfg.CaminhoDB)
	defer fecharLog()

	log := slog.New(slog.NewTextHandler(destino, &slog.HandlerOptions{Level: slog.LevelInfo}))
	slog.SetDefault(log)

	// Instancia unica: se a porta padrao ja estiver ocupada pelo proprio
	// painel, abre o navegador nela e encerra. Evita dois processos
	// escrevendo no mesmo SQLite por causa de um duplo-clique.
	ln, err := escutar(cfg.Porta)
	if err != nil {
		var ocupada *erroPortaOcupada
		if errors.As(err, &ocupada) {
			url := fmt.Sprintf("http://127.0.0.1:%d/", cfg.Porta)
			log.Info("painel ja esta rodando; abrindo a instancia existente", "url", url)
			if cfg.AbrirNavegador {
				abrirNavegador(url, log)
			}
			return nil
		}
		return err
	}
	defer ln.Close()

	porta := ln.Addr().(*net.TCPAddr).Port

	db, err := store.Open(cfg.CaminhoDB)
	if err != nil {
		return err
	}
	// O SQLite em WAL deixa arquivos -wal/-shm; fechar faz o checkpoint.
	defer db.Close()
	log.Info("banco local pronto", "caminho", cfg.CaminhoDB)

	// O painel sobe mesmo sem daemon: quem acabou de instalar precisa ver a
	// tela de diagnostico, nao um processo que se recusa a iniciar.
	dk, err := docker.New(docker.Options{Host: cfg.DockerHost})
	if err != nil {
		log.Warn("cliente Docker nao configurado", "erro", err, "host", cfg.DockerHost)
		dk = nil
	}

	h, err := handler.New(handler.Deps{
		DB:         db,
		Docker:     dk,
		Compose:    compose.New(cfg.WSLDistro),
		FS:         assetFS(cfg.Dev),
		Dev:        cfg.Dev,
		Porta:      porta,
		Versao:     versao,
		Log:        log,
		DockerHost: cfg.DockerHost,
		WSLDistro:  cfg.WSLDistro,
	})
	if err != nil {
		return err
	}

	mux := http.NewServeMux()
	// /healthz fica fora dos middlewares: e o que permite detectar instancia
	// propria na porta antes mesmo de haver token CSRF.
	mux.HandleFunc("GET /healthz", h.Healthz)
	mux.Handle("/", h.Routes())

	srv := &http.Server{
		Handler:           mux,
		ReadHeaderTimeout: 10 * time.Second,
		// Sem WriteTimeout de proposito: SSE e WebSocket sao conexoes longas
		// e seriam cortados no meio.
		IdleTimeout: 120 * time.Second,
	}

	url := fmt.Sprintf("http://127.0.0.1:%d/", porta)
	log.Info("cais iniciado", "url", url, "versao", versao, "dev", cfg.Dev)

	erros := make(chan error, 1)
	go func() {
		if err := srv.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
			erros <- err
		}
	}()

	if cfg.AbrirNavegador {
		abrirNavegador(url, log)
	}

	parar := make(chan os.Signal, 1)
	signal.Notify(parar, os.Interrupt, syscall.SIGTERM)

	select {
	case err := <-erros:
		return err
	case <-parar:
		log.Info("encerrando o painel")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return srv.Shutdown(ctx)
}

// NomeArquivoLog e o log do painel, gravado ao lado do banco.
const NomeArquivoLog = "cais.log"

// destinoDoLog manda o log para o arquivo e, quando ha console, tambem para a
// saida padrao.
//
// O executavel distribuido e compilado com -H=windowsgui justamente para nao
// deixar uma janela de console aberta enquanto o painel roda. O preco e que
// stdout vai para lugar nenhum, entao o arquivo e a unica trilha que sobra
// quando algo da errado. Fica ao lado do banco para que -db leve os dois
// juntos.
func destinoDoLog(caminhoDB string) (io.Writer, func()) {
	caminho := filepath.Join(filepath.Dir(caminhoDB), NomeArquivoLog)

	if err := os.MkdirAll(filepath.Dir(caminho), 0o755); err != nil {
		return os.Stdout, func() {}
	}
	f, err := os.OpenFile(caminho, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		// Sem arquivo, o console ainda serve em execucao interativa.
		return os.Stdout, func() {}
	}
	return io.MultiWriter(os.Stdout, f), func() { f.Close() }
}

// erroPortaOcupada sinaliza que a porta ja esta servida por outra instancia
// deste mesmo painel.
type erroPortaOcupada struct{ porta int }

func (e *erroPortaOcupada) Error() string {
	return fmt.Sprintf("porta %d ja esta em uso por outra instancia do painel", e.porta)
}

// escutar tenta a porta preferida; se estiver ocupada, distingue entre "e o
// proprio painel" e "e outro programa", caindo para porta livre no segundo caso.
//
// O bind e sempre em 127.0.0.1, nunca :porta. O painel conversa com um daemon
// Docker que e root-equivalente; expor isso na rede seria entregar a maquina.
func escutar(preferida int) (net.Listener, error) {
	if preferida > 0 {
		ln, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", preferida))
		if err == nil {
			return ln, nil
		}
		if ehNossaInstancia(preferida) {
			return nil, &erroPortaOcupada{porta: preferida}
		}
		slog.Warn("porta preferida ocupada por outro programa; usando porta livre", "porta", preferida)
	}

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, fmt.Errorf("escutar em loopback: %w", err)
	}
	return ln, nil
}

// ehNossaInstancia consulta /healthz e confere o header de identificacao.
func ehNossaInstancia(porta int) bool {
	c := &http.Client{Timeout: 2 * time.Second}
	resp, err := c.Get(fmt.Sprintf("http://127.0.0.1:%d/healthz", porta))
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	return resp.Header.Get("X-Cais") != ""
}

// abrirNavegador abre a URL no navegador padrao do sistema.
func abrirNavegador(url string, log *slog.Logger) {
	cmd := comandoNavegador(url)
	if err := cmd.Start(); err != nil {
		log.Warn("nao foi possivel abrir o navegador", "erro", err, "url", url)
		return
	}
	go cmd.Wait() // evita processo zumbi
}

// comandoNavegador monta a chamada que delega a URL ao navegador padrao.
//
// No Windows usa rundll32 em vez de "cmd /c start": o start trata o primeiro
// argumento entre aspas como titulo da janela, o que silenciosamente nao abre
// a URL. Fora do Windows, xdg-open e o despachante padrao do freedesktop.
func comandoNavegador(url string) *exec.Cmd {
	if runtime.GOOS == "windows" {
		return exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	}
	return exec.Command("xdg-open", url)
}
