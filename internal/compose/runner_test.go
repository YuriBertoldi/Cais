package compose

import (
	"context"
	"testing"
)

// Sem --exec o wsl.exe passa a linha de comando por um shell da distro, que
// expande $VAR e $(...) nos argumentos (caminho do yml, nome do projeto).
func TestMontarUsaExecSemShell(t *testing.T) {
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

func TestToWSLPath(t *testing.T) {
	casos := map[string]string{
		`C:\Sistemas\git\projeto`: "/mnt/c/Sistemas/git/projeto",
		`d:/dados/x.yml`:          "/mnt/d/dados/x.yml",
		"/home/u/x.yml":           "/home/u/x.yml",
		"":                        "",
	}
	for entrada, esperado := range casos {
		if got := ToWSLPath(entrada); got != esperado {
			t.Errorf("ToWSLPath(%q) = %q, esperava %q", entrada, got, esperado)
		}
	}
}
