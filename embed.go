package main

import (
	"embed"
	"io/fs"
	"os"
)

//go:embed all:templates all:static
var embedded embed.FS

// assetFS devolve a raiz de onde templates/ e static/ sao lidos.
//
// Em modo dev le do disco, para que editar um .html ou .css nao exija
// recompilar. No binario distribuido le do embed, que e o que permite
// entregar um .exe unico, sem pasta de apoio ao lado.
func assetFS(dev bool) fs.FS {
	if dev {
		return os.DirFS(".")
	}
	return embedded
}
