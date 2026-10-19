package docker

import (
	"bufio"
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"net/url"
	"strconv"
	"time"
)

// Fluxo identifica a origem de um pedaco do log.
type Fluxo byte

const (
	FluxoStdin  Fluxo = 0
	FluxoStdout Fluxo = 1
	FluxoStderr Fluxo = 2
)

// Nome devolve o rotulo usado na interface.
func (f Fluxo) Nome() string {
	if f == FluxoStderr {
		return "err"
	}
	return "out"
}

// LogOptions configura a leitura do log de um container.
type LogOptions struct {
	Follow     bool
	Timestamps bool
	Tail       string // "100", "all"
	Since      time.Time
}

// Logs abre o stream de log de um container.
//
// O chamador fecha o ReadCloser. Para interromper um follow, cancele o
// context -- fechar o corpo sozinho pode bloquear ate o proximo byte chegar.
func (c *Client) Logs(ctx context.Context, id string, o LogOptions) (io.ReadCloser, error) {
	q := url.Values{}
	q.Set("stdout", "1")
	q.Set("stderr", "1")
	if o.Follow {
		q.Set("follow", "1")
	}
	if o.Timestamps {
		q.Set("timestamps", "1")
	}
	if o.Tail != "" {
		q.Set("tail", o.Tail)
	}
	if !o.Since.IsZero() {
		q.Set("since", strconv.FormatInt(o.Since.Unix(), 10))
	}
	return c.stream(ctx, "/containers/"+id+"/logs", q)
}

// cabecalhoFrame e o tamanho do cabecalho de multiplexacao do Docker.
const cabecalhoFrame = 8

// DemuxFrames le o stream de log e entrega cada pedaco ja separado por fluxo.
//
// Quando o container NAO tem TTY, o Docker multiplexa stdout e stderr num
// unico stream, prefixando cada pedaco com 8 bytes:
//
//	[tipo][0][0][0][tamanho uint32 big-endian]
//
// Repassar esse stream cru para a tela produz lixo binario a cada 8 bytes --
// e o erro classico de quem implementa isso pela primeira vez.
//
// Com TTY o stream e cru, sem cabecalho, porque nao ha o que separar: os dois
// fluxos ja chegam combinados no pseudo-terminal.
func DemuxFrames(r io.Reader, tty bool, fn func(Fluxo, []byte) error) error {
	br := bufio.NewReaderSize(r, 64<<10)

	if tty {
		buf := make([]byte, 32<<10)
		for {
			n, err := br.Read(buf)
			if n > 0 {
				if e := fn(FluxoStdout, buf[:n]); e != nil {
					return e
				}
			}
			if err != nil {
				return finalizar(err)
			}
		}
	}

	cab := make([]byte, cabecalhoFrame)
	for {
		if _, err := io.ReadFull(br, cab); err != nil {
			return finalizar(err)
		}

		fluxo := Fluxo(cab[0])
		tamanho := binary.BigEndian.Uint32(cab[4:8])
		if tamanho == 0 {
			continue
		}
		// Guarda contra um cabecalho corrompido pedir uma alocacao absurda.
		if tamanho > 16<<20 {
			return fmt.Errorf("frame de log inválido: %d bytes", tamanho)
		}

		payload := make([]byte, tamanho)
		if _, err := io.ReadFull(br, payload); err != nil {
			return finalizar(err)
		}
		if err := fn(fluxo, payload); err != nil {
			return err
		}
	}
}

// finalizar converte o fim natural do stream em sucesso.
func finalizar(err error) error {
	if err == io.EOF || err == io.ErrUnexpectedEOF {
		return nil
	}
	return err
}
