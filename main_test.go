package main

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// TestDestinoDoLogGravaAoLadoDoBanco: o executavel do Windows e compilado com
// -H=windowsgui e nao tem console, entao o arquivo e a unica trilha que sobra.
// Ele acompanha o -db para que apontar o banco leve o log junto.
func TestDestinoDoLogGravaAoLadoDoBanco(t *testing.T) {
	dir := t.TempDir()
	caminhoDB := filepath.Join(dir, "painel.db")

	w, fechar := destinoDoLog(caminhoDB)
	if _, err := w.Write([]byte("linha de teste\n")); err != nil {
		t.Fatalf("escrever no log: %v", err)
	}
	fechar()

	conteudo, err := os.ReadFile(filepath.Join(dir, NomeArquivoLog))
	if err != nil {
		t.Fatalf("ler o log: %v", err)
	}
	if !strings.Contains(string(conteudo), "linha de teste") {
		t.Errorf("log = %q; esperado conter a linha escrita", conteudo)
	}
}

// TestDestinoDoLogCriaODiretorio cobre a primeira execucao, em que a pasta de
// dados ainda nao existe.
func TestDestinoDoLogCriaODiretorio(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "ainda", "nao", "existe")

	w, fechar := destinoDoLog(filepath.Join(dir, "painel.db"))
	w.Write([]byte("oi\n"))
	fechar()

	if _, err := os.Stat(filepath.Join(dir, NomeArquivoLog)); err != nil {
		t.Errorf("log nao foi criado: %v", err)
	}
}

// TestDestinoDoLogAcumula: cada execucao acrescenta, nao sobrescreve. Um
// painel que falha no startup e e reaberto nao pode apagar a pista anterior.
func TestDestinoDoLogAcumula(t *testing.T) {
	caminhoDB := filepath.Join(t.TempDir(), "painel.db")

	for _, linha := range []string{"primeira\n", "segunda\n"} {
		w, fechar := destinoDoLog(caminhoDB)
		w.Write([]byte(linha))
		fechar()
	}

	conteudo, err := os.ReadFile(filepath.Join(filepath.Dir(caminhoDB), NomeArquivoLog))
	if err != nil {
		t.Fatalf("ler o log: %v", err)
	}
	for _, esperado := range []string{"primeira", "segunda"} {
		if !strings.Contains(string(conteudo), esperado) {
			t.Errorf("log = %q; esperado conter %q", conteudo, esperado)
		}
	}
}

// TestDestinoDoLogCaiParaStdout: nao conseguir abrir o arquivo nao pode
// impedir o painel de subir -- ele so perde a trilha em disco.
func TestDestinoDoLogCaiParaStdout(t *testing.T) {
	// Um arquivo comum no lugar onde o diretorio precisaria existir.
	arquivo := filepath.Join(t.TempDir(), "arquivo.txt")
	if err := os.WriteFile(arquivo, []byte("x"), 0o644); err != nil {
		t.Fatalf("preparar arquivo: %v", err)
	}

	w, fechar := destinoDoLog(filepath.Join(arquivo, "sub", "painel.db"))
	defer fechar()

	if w != os.Stdout {
		t.Errorf("destino = %T; esperado cair para os.Stdout", w)
	}
}

func TestComandoNavegador(t *testing.T) {
	const url = "http://127.0.0.1:45678/"

	cmd := comandoNavegador(url)

	if runtime.GOOS == "windows" {
		// rundll32, e nao "cmd /c start": o start trata o primeiro argumento
		// entre aspas como titulo da janela e nao abre a URL.
		if !strings.Contains(cmd.Path, "rundll32") {
			t.Errorf("cmd.Path = %q; esperado rundll32", cmd.Path)
		}
		if len(cmd.Args) < 3 || cmd.Args[1] != "url.dll,FileProtocolHandler" {
			t.Errorf("args = %q; esperado o FileProtocolHandler", cmd.Args)
		}
	} else if !strings.Contains(cmd.Path, "xdg-open") {
		t.Errorf("cmd.Path = %q; esperado xdg-open fora do Windows", cmd.Path)
	}

	if cmd.Args[len(cmd.Args)-1] != url {
		t.Errorf("ultimo argumento = %q; esperado a URL", cmd.Args[len(cmd.Args)-1])
	}
}

// TestErroPortaOcupada cobre a mensagem que distingue "e o proprio painel" de
// "e outro programa" na escolha da porta.
func TestErroPortaOcupada(t *testing.T) {
	err := &erroPortaOcupada{porta: 45678}

	if !strings.Contains(err.Error(), "45678") {
		t.Errorf("erro = %q; esperado citar a porta", err)
	}
}
