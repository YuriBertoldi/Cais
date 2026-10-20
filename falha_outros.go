//go:build !windows

package main

// avisarFalhaFatal nao faz nada fora do Windows.
//
// A caixa de mensagem existe por causa do -H=windowsgui, que tira o console do
// executavel do Windows. Em Linux o binario roda no terminal e o erro ja
// aparece ali, alem de ir para o arquivo de log.
func avisarFalhaFatal(error) {}
