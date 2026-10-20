package compose

import (
	"context"
	"os/exec"
	"runtime"
	"strings"
	"testing"
)

func TestNewDistroPadrao(t *testing.T) {
	casos := []struct {
		entrada  string
		esperado string
	}{
		{"", "Ubuntu-24.04"},
		{"   ", "Ubuntu-24.04"},
		{"Debian", "Debian"},
		{"Ubuntu-22.04", "Ubuntu-22.04"},
	}

	for _, c := range casos {
		if got := New(c.entrada).Distro; got != c.esperado {
			t.Errorf("New(%q).Distro = %q; esperado %q", c.entrada, got, c.esperado)
		}
	}
}

// TestMontarArgumentos confere a linha de comando montada, sem executar nada.
//
// E o contrato com a CLI do compose: -f sempre absoluto e traduzido para o
// WSL, e --project-directory apontando para a pasta do proprio yml, para que
// caminho relativo dentro do arquivo resolva a partir dela.
func TestMontarArgumentos(t *testing.T) {
	pularSeNaoForWindows(t)
	r := New("Ubuntu-24.04")

	cmd := r.montar(context.Background(), `C:\Sistemas\app\docker-compose.yml`, "", "up", "-d")

	esperado := []string{
		"wsl.exe", "-d", "Ubuntu-24.04", "--exec",
		"docker", "compose",
		"-f", "/mnt/c/Sistemas/app/docker-compose.yml",
		"--project-directory", "/mnt/c/Sistemas/app",
		"up", "-d",
	}
	compararArgs(t, cmd.Args, esperado)

	if !strings.HasSuffix(cmd.Path, "wsl.exe") && cmd.Path != "wsl.exe" {
		t.Errorf("cmd.Path = %q; esperado terminar em wsl.exe", cmd.Path)
	}
}

func TestMontarComProjectName(t *testing.T) {
	pularSeNaoForWindows(t)
	r := New("Debian")

	cmd := r.montar(context.Background(), `D:\app\compose.yml`, "meuprojeto", "ps")

	esperado := []string{
		"wsl.exe", "-d", "Debian", "--exec",
		"docker", "compose",
		"-f", "/mnt/d/app/compose.yml",
		"--project-directory", "/mnt/d/app",
		"-p", "meuprojeto",
		"ps",
	}
	compararArgs(t, cmd.Args, esperado)
}

func TestMontarSemProjectNameNaoEnviaFlagP(t *testing.T) {
	pularSeNaoForWindows(t)
	cmd := New("Ubuntu-24.04").montar(context.Background(), `C:\a\compose.yml`, "", "down")
	for _, a := range cmd.Args {
		if a == "-p" {
			t.Fatalf("flag -p nao deveria aparecer sem project name: %v", cmd.Args)
		}
	}
}

// Sem --exec o wsl.exe passa a linha de comando por um shell da distro, que
// expande $VAR e $(...) nos argumentos (caminho do yml, nome do projeto).
func TestMontarUsaExecSemShell(t *testing.T) {
	pularSeNaoForWindows(t)
	r := New("Ubuntu")
	cmd := r.montar(context.Background(), `C:\pasta\a$(echo x)\docker-compose.yml`, "proj$USER", "up")

	args := cmd.Args[1:]
	esperado := []string{
		"-d", "Ubuntu", "--exec", "docker", "compose",
		"-f", "/mnt/c/pasta/a$(echo x)/docker-compose.yml",
		"--project-directory", "/mnt/c/pasta/a$(echo x)",
		"-p", "proj$USER", "up",
	}
	if len(args) != len(esperado) {
		t.Fatalf("args = %q, esperava %q", args, esperado)
	}
	for i := range esperado {
		if args[i] != esperado[i] {
			t.Errorf("args[%d] = %q, esperava %q", i, args[i], esperado[i])
		}
	}
}

// TestMontarForaDoWindowsChamaDockerDireto: em Linux nao ha WSL entre o painel
// e o daemon, e o caminho do yml ja e POSIX. Passar pelo wsl.exe ali seria
// chamar um executavel que nem existe.
func TestMontarForaDoWindowsChamaDockerDireto(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("cobre o ramo nao-Windows")
	}
	r := New("Ubuntu-24.04")

	cmd := r.montar(context.Background(), "/home/dev/app/docker-compose.yml", "meuprojeto", "up", "-d")

	esperado := []string{
		"docker", "compose",
		"-f", "/home/dev/app/docker-compose.yml",
		"--project-directory", "/home/dev/app",
		"-p", "meuprojeto",
		"up", "-d",
	}
	compararArgs(t, cmd.Args, esperado)

	for _, a := range cmd.Args {
		if strings.Contains(a, "wsl") {
			t.Errorf("argumento %q menciona WSL fora do Windows: %v", a, cmd.Args)
		}
	}
}

// pularSeNaoForWindows guarda os testes que afirmam a linha de comando com
// wsl.exe e caminho traduzido -- ambos so valem no alvo Windows.
func pularSeNaoForWindows(t *testing.T) {
	t.Helper()
	if runtime.GOOS != "windows" {
		t.Skip("a linha de comando com wsl.exe so vale no Windows")
	}
}

func compararArgs(t *testing.T, got, esperado []string) {
	t.Helper()
	if len(got) != len(esperado) {
		t.Fatalf("args = %v (%d); esperado %v (%d)", got, len(got), esperado, len(esperado))
	}
	for i := range esperado {
		if got[i] != esperado[i] {
			t.Errorf("args[%d] = %q; esperado %q (completo: %v)", i, got[i], esperado[i], got)
		}
	}
}

// TestResumir cobre o corte da saida do compose para caber numa mensagem de
// tela: so as ultimas linhas interessam, e o total e limitado.
func TestResumir(t *testing.T) {
	casos := []struct {
		nome     string
		entrada  string
		esperado string
	}{
		{"linha unica passa inteira", "erro: porta ocupada", "erro: porta ocupada"},
		{"quatro linhas sao mantidas", "a\nb\nc\nd", "a | b | c | d"},
		{"mais de quatro mantem as ultimas", "1\n2\n3\n4\n5\n6", "3 | 4 | 5 | 6"},
		{"vazio", "", ""},
	}

	for _, c := range casos {
		t.Run(c.nome, func(t *testing.T) {
			if got := resumir(c.entrada); got != c.esperado {
				t.Errorf("resumir(%q) = %q; esperado %q", c.entrada, got, c.esperado)
			}
		})
	}
}

func TestResumirTruncaEm300(t *testing.T) {
	longa := strings.Repeat("x", 500)

	got := resumir(longa)

	if len(got) != 303 {
		t.Fatalf("len = %d; esperado 303 (300 + reticencias)", len(got))
	}
	if !strings.HasSuffix(got, "...") {
		t.Errorf("esperado sufixo \"...\"; obtido %q", got[len(got)-10:])
	}
}

// TestExecutarUneStdoutEStderr cobre a juncao dos dois fluxos sem envolver o
// WSL: erro de compose traz a mensagem util em stderr, e sem unir os fluxos o
// usuario veria apenas "exit status 1".
//
// Usa o cmd.exe como processo de apoio, nao o wsl.exe -- o teste e sobre o
// tratamento da saida, nao sobre o compose.
func TestExecutarUneStdoutEStderr(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("usa cmd.exe como processo de apoio")
	}
	r := New("Ubuntu-24.04")
	ctx := context.Background()

	t.Run("sucesso devolve a saida aparada", func(t *testing.T) {
		cmd := exec.CommandContext(ctx, "cmd", "/c", "echo tudo certo")

		saida, err := r.executar(ctx, cmd)

		if err != nil {
			t.Fatalf("executar: %v", err)
		}
		if saida != "tudo certo" {
			t.Errorf("saida = %q; esperado \"tudo certo\"", saida)
		}
	})

	t.Run("falha traz a mensagem do stderr", func(t *testing.T) {
		cmd := exec.CommandContext(ctx, "cmd", "/c", "echo falhou feio 1>&2 & exit /b 1")

		saida, err := r.executar(ctx, cmd)

		if err == nil {
			t.Fatal("esperado erro")
		}
		if !strings.Contains(err.Error(), "falhou feio") {
			t.Errorf("erro = %q; esperado conter a mensagem do stderr", err)
		}
		if !strings.Contains(saida, "falhou feio") {
			t.Errorf("saida = %q; esperado conter a mensagem do stderr", saida)
		}
	})

	t.Run("falha silenciosa ainda vira erro", func(t *testing.T) {
		cmd := exec.CommandContext(ctx, "cmd", "/c", "exit /b 3")

		if _, err := r.executar(ctx, cmd); err == nil {
			t.Fatal("esperado erro mesmo sem saida")
		} else if !strings.Contains(err.Error(), "compose falhou") {
			t.Errorf("erro = %q; esperado mencionar que o compose falhou", err)
		}
	})
}

func TestServicoRodando(t *testing.T) {
	casos := []struct {
		estado   string
		esperado bool
	}{
		{"running", true},
		{"exited", false},
		{"paused", false},
		{"", false},
		{"Running", false}, // o compose emite minusculo; comparacao e exata
	}

	for _, c := range casos {
		if got := (Servico{State: c.estado}).Rodando(); got != c.esperado {
			t.Errorf("Servico{State:%q}.Rodando() = %v; esperado %v", c.estado, got, c.esperado)
		}
	}
}
