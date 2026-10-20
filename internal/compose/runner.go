package compose

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

// Runner executa o docker compose: no Windows, dentro de uma distro WSL;
// em Linux, direto na maquina. Distro e ignorado fora do Windows.
type Runner struct {
	Distro string
}

// New monta o runner para a distro informada.
func New(distro string) *Runner {
	if strings.TrimSpace(distro) == "" {
		distro = "Ubuntu-24.04"
	}
	return &Runner{Distro: distro}
}

// Servico e uma linha de `docker compose ps`.
type Servico struct {
	Name    string `json:"Name"`
	Service string `json:"Service"`
	State   string `json:"State"`
	Health  string `json:"Health"`
	Image   string `json:"Image"`
}

// Rodando informa se o servico esta de pe.
func (s Servico) Rodando() bool { return s.State == "running" }

// montar prepara a chamada do compose.
//
// No Windows, `wsl -d <distro> --exec docker compose -f <yml> ...`; em Linux,
// `docker compose` direto, porque nao ha WSL entre o painel e o daemon.
//
// O -f vai sempre absoluto e traduzido, e o --project-directory aponta para a
// pasta do proprio yml, para que caminhos relativos do arquivo resolvam a
// partir dela e nao do diretorio onde o painel foi iniciado.
func (r *Runner) montar(ctx context.Context, caminhoYML, projectName string, args ...string) *exec.Cmd {
	yml := ToWSLPath(caminhoYML)
	dir := ToWSLPath(filepath.Dir(caminhoYML))

	compose := []string{"compose", "-f", yml, "--project-directory", dir}
	if projectName != "" {
		compose = append(compose, "-p", projectName)
	}
	compose = append(compose, args...)

	// Fora do Windows nao ha WSL no meio: o painel, a CLI e o daemon estao na
	// mesma maquina, e o caminho do yml ja e POSIX.
	if runtime.GOOS != "windows" {
		return exec.CommandContext(ctx, "docker", compose...)
	}

	base := []string{"-d", r.Distro, "--exec", "docker"}
	return exec.CommandContext(ctx, "wsl.exe", append(base, compose...)...)
}

// executar roda o comando e devolve a saida combinada.
//
// Erro de compose costuma vir em stderr com a mensagem util, entao os dois
// fluxos sao unidos: sem isso o usuario veria apenas "exit status 1".
func (r *Runner) executar(ctx context.Context, cmd *exec.Cmd) (string, error) {
	var buf bytes.Buffer
	cmd.Stdout = &buf
	cmd.Stderr = &buf

	err := cmd.Run()
	saida := strings.TrimSpace(buf.String())
	if err != nil {
		if saida == "" {
			return "", fmt.Errorf("compose falhou: %w", err)
		}
		return saida, fmt.Errorf("%s", resumir(saida))
	}
	return saida, nil
}

// resumir corta a saida para caber numa mensagem de tela.
func resumir(s string) string {
	linhas := strings.Split(s, "\n")
	// As ultimas linhas sao as que trazem o erro de verdade.
	if len(linhas) > 4 {
		linhas = linhas[len(linhas)-4:]
	}
	msg := strings.TrimSpace(strings.Join(linhas, " | "))
	if len(msg) > 300 {
		msg = msg[:300] + "..."
	}
	return msg
}

// Up sobe a stack em segundo plano.
func (r *Runner) Up(ctx context.Context, yml, projeto string, rebuild bool) (string, error) {
	args := []string{"up", "-d", "--remove-orphans"}
	if rebuild {
		args = append(args, "--build")
	}
	return r.executar(ctx, r.montar(ctx, yml, projeto, args...))
}

// Down derruba a stack. comVolumes tambem apaga os volumes nomeados dela.
func (r *Runner) Down(ctx context.Context, yml, projeto string, comVolumes bool) (string, error) {
	args := []string{"down", "--remove-orphans"}
	if comVolumes {
		args = append(args, "-v")
	}
	return r.executar(ctx, r.montar(ctx, yml, projeto, args...))
}

// Restart reinicia os servicos da stack.
func (r *Runner) Restart(ctx context.Context, yml, projeto string) (string, error) {
	return r.executar(ctx, r.montar(ctx, yml, projeto, "restart"))
}

// Pull baixa as imagens declaradas.
func (r *Runner) Pull(ctx context.Context, yml, projeto string) (string, error) {
	return r.executar(ctx, r.montar(ctx, yml, projeto, "pull"))
}

// Ps devolve o estado dos servicos da stack.
//
// Uma stack derrubada nao e erro: devolve lista vazia.
func (r *Runner) Ps(ctx context.Context, yml, projeto string) ([]Servico, error) {
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()

	cmd := r.montar(ctx, yml, projeto, "ps", "--format", "json", "--all")
	var buf bytes.Buffer
	cmd.Stdout = &buf
	cmd.Stderr = &buf
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("%s", resumir(buf.String()))
	}

	// O compose emite um objeto JSON por linha (JSON Lines), nao um array.
	var out []Servico
	for _, linha := range strings.Split(buf.String(), "\n") {
		linha = strings.TrimSpace(linha)
		if linha == "" || !strings.HasPrefix(linha, "{") {
			continue
		}
		var s Servico
		if err := json.Unmarshal([]byte(linha), &s); err == nil {
			out = append(out, s)
		}
	}
	return out, nil
}

// Validar confere se o arquivo e um compose legivel, devolvendo os servicos
// declarados. Usado ao cadastrar uma stack nova.
func (r *Runner) Validar(ctx context.Context, yml string) ([]string, error) {
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()

	saida, err := r.executar(ctx, r.montar(ctx, yml, "", "config", "--services"))
	if err != nil {
		return nil, err
	}
	var servicos []string
	for _, l := range strings.Split(saida, "\n") {
		if l = strings.TrimSpace(l); l != "" {
			servicos = append(servicos, l)
		}
	}
	if len(servicos) == 0 {
		return nil, fmt.Errorf("nenhum serviço declarado no arquivo")
	}
	return servicos, nil
}
