package compose

import (
	"runtime"
	"testing"
)

// TestToWSLPath cobre a traducao de caminho Windows -> ponto de montagem WSL.
//
// A conversao e o ponto mais sensivel do pacote: um caminho traduzido errado
// nao quebra o `docker compose`, faz o container subir com bind mount vazio.
func TestToWSLPath(t *testing.T) {
	casos := []struct {
		nome     string
		entrada  string
		esperado string
	}{
		{"unidade maiuscula com barra invertida", `C:\Sistemas\git\projeto`, "/mnt/c/Sistemas/git/projeto"},
		{"unidade minuscula com barra normal", "c:/Sistemas/git", "/mnt/c/Sistemas/git"},
		{"unidade minuscula ate o arquivo", "d:/dados/x.yml", "/mnt/d/dados/x.yml"},
		{"outra unidade ate o arquivo", `D:\x\y\docker-compose.yml`, "/mnt/d/x/y/docker-compose.yml"},
		{"raiz da unidade", `C:\`, "/mnt/c/"},
		{"espacos em volta sao descartados", "  C:\\a\\b  ", "/mnt/c/a/b"},
		{"caminho posix passa inalterado", "/home/dev/app", "/home/dev/app"},
		{"posix com espacos", "  /srv/app  ", "/srv/app"},
		{"vazio", "", ""},
		{"so espacos", "   ", ""},
	}

	for _, c := range casos {
		t.Run(c.nome, func(t *testing.T) {
			if got := ToWSLPath(c.entrada); got != c.esperado {
				t.Errorf("ToWSLPath(%q) = %q; esperado %q", c.entrada, got, c.esperado)
			}
		})
	}
}

// TestToWSLPathDependeDoSO documenta que a normalizacao de barras usa
// filepath.ToSlash, que so trata "\" como separador no Windows. O painel e um
// executavel Windows, entao e la que o comportamento precisa valer.
func TestToWSLPathDependeDoSO(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("normalizacao de barra invertida so vale no Windows")
	}

	casos := []struct {
		nome     string
		entrada  string
		esperado string
	}{
		{"caminho relativo", `pasta\sub\compose.yml`, "pasta/sub/compose.yml"},
		{"caminho UNC", `\\servidor\share\app`, "//servidor/share/app"},
	}

	for _, c := range casos {
		t.Run(c.nome, func(t *testing.T) {
			if got := ToWSLPath(c.entrada); got != c.esperado {
				t.Errorf("ToWSLPath(%q) = %q; esperado %q", c.entrada, got, c.esperado)
			}
		})
	}
}
