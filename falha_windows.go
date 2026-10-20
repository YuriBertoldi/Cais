package main

import (
	"golang.org/x/sys/windows"
)

// avisarFalhaFatal mostra uma caixa de mensagem quando o painel nao consegue
// subir.
//
// O executavel distribuido e compilado com -H=windowsgui para nao deixar uma
// janela de console aberta. Sem console, um erro no startup -- banco sem
// permissao de escrita, por exemplo -- nao apareceria em lugar nenhum: o
// duplo-clique simplesmente nao faria nada. A caixa fecha essa lacuna.
//
// O texto cita o arquivo de log porque a mensagem do erro raramente basta
// sozinha para o usuario resolver.
func avisarFalhaFatal(err error) {
	texto := "O Cais nao conseguiu iniciar.\n\n" + err.Error() +
		"\n\nO detalhe fica registrado em " + NomeArquivoLog +
		", na mesma pasta do banco."

	titulo, _ := windows.UTF16PtrFromString("Cais")
	corpo, erroConv := windows.UTF16PtrFromString(texto)
	if erroConv != nil {
		return // texto impossivel de converter: nada a fazer aqui
	}
	// MB_OK | MB_ICONERROR. O erro de MessageBox e ignorado de proposito:
	// este ja e o caminho de falha, nao ha para onde escalar.
	_, _ = windows.MessageBox(0, corpo, titulo, windows.MB_OK|windows.MB_ICONERROR)
}
