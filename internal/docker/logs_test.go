package docker

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"
)

// frame monta um pedaco do stream multiplexado do Docker:
//
//	[tipo][0][0][0][tamanho uint32 big-endian][payload]
func frame(f Fluxo, payload string) []byte {
	cab := make([]byte, cabecalhoFrame)
	cab[0] = byte(f)
	binary.BigEndian.PutUint32(cab[4:8], uint32(len(payload)))
	return append(cab, payload...)
}

type pedaco struct {
	fluxo Fluxo
	texto string
}

func coletar(t *testing.T, r io.Reader, tty bool) []pedaco {
	t.Helper()
	var out []pedaco
	if err := DemuxFrames(r, tty, func(f Fluxo, p []byte) error {
		out = append(out, pedaco{f, string(p)})
		return nil
	}); err != nil {
		t.Fatalf("DemuxFrames: %v", err)
	}
	return out
}

// TestDemuxFramesSeparaFluxos cobre o caso sem TTY, em que stdout e stderr
// chegam multiplexados num unico stream. Repassar o stream cru colocaria lixo
// binario na tela a cada 8 bytes.
func TestDemuxFramesSeparaFluxos(t *testing.T) {
	var b bytes.Buffer
	b.Write(frame(FluxoStdout, "linha normal\n"))
	b.Write(frame(FluxoStderr, "deu ruim\n"))
	b.Write(frame(FluxoStdout, "seguindo\n"))

	got := coletar(t, &b, false)

	esperado := []pedaco{
		{FluxoStdout, "linha normal\n"},
		{FluxoStderr, "deu ruim\n"},
		{FluxoStdout, "seguindo\n"},
	}
	if len(got) != len(esperado) {
		t.Fatalf("obtidos %d pedacos; esperado %d", len(got), len(esperado))
	}
	for i := range esperado {
		if got[i] != esperado[i] {
			t.Errorf("pedaco %d = %+v; esperado %+v", i, got[i], esperado[i])
		}
	}
}

func TestDemuxFramesIgnoraFrameVazio(t *testing.T) {
	var b bytes.Buffer
	b.Write(frame(FluxoStdout, ""))
	b.Write(frame(FluxoStdout, "util"))
	b.Write(frame(FluxoStderr, ""))

	got := coletar(t, &b, false)

	if len(got) != 1 || got[0].texto != "util" {
		t.Errorf("pedacos = %+v; esperado apenas {out, \"util\"}", got)
	}
}

func TestDemuxFramesStreamVazio(t *testing.T) {
	if got := coletar(t, bytes.NewReader(nil), false); len(got) != 0 {
		t.Errorf("pedacos = %+v; esperado nenhum", got)
	}
}

// TestDemuxFramesProtegeContraTamanhoAbsurdo cobre a guarda contra cabecalho
// corrompido pedir uma alocacao gigante.
func TestDemuxFramesProtegeContraTamanhoAbsurdo(t *testing.T) {
	cab := make([]byte, cabecalhoFrame)
	cab[0] = byte(FluxoStdout)
	binary.BigEndian.PutUint32(cab[4:8], 17<<20) // acima do limite de 16 MB

	err := DemuxFrames(bytes.NewReader(cab), false, func(Fluxo, []byte) error { return nil })

	if err == nil {
		t.Fatal("esperado erro para frame acima do limite")
	}
	if !strings.Contains(err.Error(), "inválido") {
		t.Errorf("erro = %q; esperado mencionar frame invalido", err)
	}
}

// TestDemuxFramesStreamTruncado documenta a escolha de encerrar em silencio:
// o fim abrupto do stream e o que acontece quando o container morre no meio
// do follow, e nao deve virar erro na tela.
func TestDemuxFramesStreamTruncado(t *testing.T) {
	completo := frame(FluxoStdout, "mensagem completa")
	truncado := completo[:len(completo)-5]

	err := DemuxFrames(bytes.NewReader(truncado), false, func(Fluxo, []byte) error { return nil })

	if err != nil {
		t.Errorf("DemuxFrames com stream truncado = %v; esperado nil", err)
	}
}

func TestDemuxFramesPropagaErroDoCallback(t *testing.T) {
	var b bytes.Buffer
	b.Write(frame(FluxoStdout, "primeira"))
	b.Write(frame(FluxoStdout, "segunda"))

	parar := errors.New("cliente desconectou")
	var chamadas int

	err := DemuxFrames(&b, false, func(Fluxo, []byte) error {
		chamadas++
		return parar
	})

	if !errors.Is(err, parar) {
		t.Errorf("erro = %v; esperado %v", err, parar)
	}
	if chamadas != 1 {
		t.Errorf("callback chamado %d vezes; esperado parar na primeira", chamadas)
	}
}

// TestDemuxFramesComTTY cobre o caso com TTY: o stream e cru, sem cabecalho,
// porque os dois fluxos ja chegam combinados no pseudo-terminal.
func TestDemuxFramesComTTY(t *testing.T) {
	entrada := "saida crua do terminal\nsem cabecalho de 8 bytes\n"

	got := coletar(t, strings.NewReader(entrada), true)

	if len(got) == 0 {
		t.Fatal("nenhum pedaco entregue")
	}
	var montado strings.Builder
	for _, p := range got {
		if p.fluxo != FluxoStdout {
			t.Errorf("fluxo = %v; com TTY tudo vem como stdout", p.fluxo)
		}
		montado.WriteString(p.texto)
	}
	if montado.String() != entrada {
		t.Errorf("conteudo = %q; esperado %q", montado.String(), entrada)
	}
}

func TestDemuxFramesComTTYPropagaErroDoCallback(t *testing.T) {
	parar := errors.New("parar")

	err := DemuxFrames(strings.NewReader("qualquer coisa"), true, func(Fluxo, []byte) error {
		return parar
	})

	if !errors.Is(err, parar) {
		t.Errorf("erro = %v; esperado %v", err, parar)
	}
}

func TestFluxoNome(t *testing.T) {
	casos := []struct {
		fluxo    Fluxo
		esperado string
	}{
		{FluxoStdout, "out"},
		{FluxoStderr, "err"},
		{FluxoStdin, "out"}, // stdin nao aparece em log; cai no rotulo neutro
	}

	for _, c := range casos {
		if got := c.fluxo.Nome(); got != c.esperado {
			t.Errorf("Fluxo(%d).Nome() = %q; esperado %q", c.fluxo, got, c.esperado)
		}
	}
}

// TestLogsMontaQuery confere a traducao de LogOptions para a query da Engine API.
func TestLogsMontaQuery(t *testing.T) {
	desde := time.Date(2026, 10, 19, 12, 0, 0, 0, time.UTC)

	casos := []struct {
		nome     string
		opts     LogOptions
		esperado url.Values
	}{
		{
			"padrao",
			LogOptions{},
			url.Values{"stdout": {"1"}, "stderr": {"1"}},
		},
		{
			"follow com tail",
			LogOptions{Follow: true, Tail: "200"},
			url.Values{"stdout": {"1"}, "stderr": {"1"}, "follow": {"1"}, "tail": {"200"}},
		},
		{
			"com timestamps e since",
			LogOptions{Timestamps: true, Since: desde},
			url.Values{
				"stdout": {"1"}, "stderr": {"1"}, "timestamps": {"1"},
				"since": {"1792411200"}, // 2026-10-19T12:00:00Z
			},
		},
	}

	for _, c := range casos {
		t.Run(c.nome, func(t *testing.T) {
			var recebida url.Values
			var caminho string
			cli := clienteFake(t, func(w http.ResponseWriter, r *http.Request) {
				recebida, caminho = r.URL.Query(), r.URL.Path
				w.Write([]byte("ok"))
			})

			rc, err := cli.Logs(context.Background(), "abc", c.opts)
			if err != nil {
				t.Fatalf("Logs: %v", err)
			}
			defer rc.Close()

			if caminho != "/containers/abc/logs" {
				t.Errorf("caminho = %q", caminho)
			}
			if len(recebida) != len(c.esperado) {
				t.Fatalf("query = %v; esperado %v", recebida, c.esperado)
			}
			for k, v := range c.esperado {
				if recebida.Get(k) != v[0] {
					t.Errorf("query[%s] = %q; esperado %q", k, recebida.Get(k), v[0])
				}
			}
		})
	}
}

func TestLogsPropagaErroHTTP(t *testing.T) {
	c := clienteFake(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte(`{"message":"No such container: abc"}`))
	})

	rc, err := c.Logs(context.Background(), "abc", LogOptions{})
	if err == nil {
		rc.Close()
		t.Fatal("esperado erro")
	}
	if err.Error() != "No such container: abc" {
		t.Errorf("erro = %q", err)
	}
}
