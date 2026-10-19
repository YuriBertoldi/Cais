// Package compose executa o Docker Compose.
//
// O Compose nao existe na Engine API -- e uma CLI. E ele precisa rodar DENTRO
// da distro WSL, nao pelo Windows: um daemon Linux nao entende caminho
// Windows, entao `docker compose` chamado do Windows quebra bind mount
// relativo de forma silenciosa. O `up` passa limpo e o container sobe com o
// diretorio vazio, o que so aparece como erro em tempo de execucao da
// aplicacao dentro do container.
package compose

import (
	"path/filepath"
	"regexp"
	"strings"
)

// reUnidade casa "C:\" ou "c:/" no inicio do caminho.
var reUnidade = regexp.MustCompile(`^([a-zA-Z]):[\\/]`)

// ToWSLPath converte um caminho Windows para o equivalente montado no WSL.
//
//	C:\Sistemas\git\projeto  ->  /mnt/c/Sistemas/git/projeto
//
// Caminhos que ja sao POSIX passam inalterados.
func ToWSLPath(win string) string {
	p := strings.TrimSpace(win)
	if p == "" {
		return ""
	}
	if strings.HasPrefix(p, "/") {
		return p // ja e POSIX
	}

	m := reUnidade.FindStringSubmatch(p)
	if m == nil {
		// Caminho relativo ou UNC: so normaliza as barras.
		return filepath.ToSlash(p)
	}

	unidade := strings.ToLower(m[1])
	resto := filepath.ToSlash(p[len(m[0]):])
	return "/mnt/" + unidade + "/" + resto
}
