// Comando gerar-icone desenha a marca do Cais e grava docs/cais.ico.
//
// Existe para o repositorio nao depender de ImageMagick nem de pacote npm so
// para produzir um icone: a marca sao quatro poligonos e um retangulo
// arredondado, e desenhar isso com a stdlib sai mais barato do que carregar
// um rasterizador de SVG.
//
// O .ico so precisa ser regerado quando a marca mudar. Depois de rodar,
// converta-o no recurso que o linker do Go embute no .exe:
//
//	go run ./scripts/gerar-icone
//	go run github.com/akavel/rsrc@latest -ico docs/cais.ico -arch amd64 -o cais_windows_amd64.syso
//
// O sufixo _windows_amd64 no .syso nao e enfeite: o go aplica a ele as mesmas
// regras de restricao por nome de arquivo dos .go, e sem isso o recurso
// entraria tambem na compilacao para Linux, que nao sabe o que fazer com ele.
package main

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"log"
	"os"
	"path/filepath"
)

// A geometria e a mesma de faviconSVG, em internal/handler/routes.go, no
// viewBox 24x24 do SVG original. Mudou la, muda aqui.
const (
	lado = 24.0 // lado do viewBox
	raio = 6.0  // o rx do retangulo arredondado
)

// As cores sao o padrao da interface: AccentPadrao e o TextoSobre dele.
// O icone do executavel e fixo, diferente do favicon, que acompanha a cor
// escolhida pelo usuario -- o Windows le o recurso do .exe uma vez, no disco.
var (
	corFundo = color.NRGBA{R: 0x24, G: 0x96, B: 0xed, A: 0xff}
	corMarca = color.NRGBA{R: 0xff, G: 0xff, B: 0xff, A: 0xff}
)

type ponto struct{ x, y float64 }

// face e um dos tres lados visiveis do cubo, com a opacidade que o SVG usa
// para dar a impressao de volume.
var faces = []struct {
	pontos    []ponto
	opacidade float64
}{
	{[]ponto{{12, 5.2}, {18.6, 8.7}, {12, 12.2}, {5.4, 8.7}}, 1.00},
	{[]ponto{{4.9, 9.4}, {11.5, 12.9}, {11.5, 19.4}, {4.9, 15.9}}, 0.62},
	{[]ponto{{19.1, 9.4}, {12.5, 12.9}, {12.5, 19.4}, {19.1, 15.9}}, 0.82},
}

// tamanhos sao os que o Windows escolhe conforme o contexto: 16 na barra de
// titulo, 32 na barra de tarefas, 256 na visualizacao grande do Explorer.
var tamanhos = []int{16, 32, 48, 64, 128, 256}

// superAmostragem e o fator de desenho antes da reducao. Serrilhado aparece
// muito no 16x16, que e justamente onde o icone mais e visto.
const superAmostragem = 4

func main() {
	var imagens []*image.NRGBA
	for _, t := range tamanhos {
		imagens = append(imagens, desenhar(t))
	}

	destino := filepath.Join("docs", "cais.ico")
	if err := gravarICO(destino, imagens); err != nil {
		log.Fatalf("gravar %s: %v", destino, err)
	}

	info, err := os.Stat(destino)
	if err != nil {
		log.Fatalf("conferir %s: %v", destino, err)
	}
	fmt.Printf("%s gravado: %d tamanhos, %d bytes\n", destino, len(imagens), info.Size())
}

// desenhar produz a marca no tamanho pedido, com fundo transparente fora do
// retangulo arredondado.
func desenhar(tamanho int) *image.NRGBA {
	s := tamanho * superAmostragem
	grande := image.NewNRGBA(image.Rect(0, 0, s, s))
	escala := float64(s) / lado

	for py := range s {
		for px := range s {
			// Centro do pixel, de volta para as coordenadas do viewBox.
			x := (float64(px) + 0.5) / escala
			y := (float64(py) + 0.5) / escala

			if !dentroDoRetanguloArredondado(x, y) {
				continue // fora da marca: fica transparente
			}
			cor := corFundo
			for _, f := range faces {
				if dentroDoPoligono(x, y, f.pontos) {
					cor = sobrepor(corMarca, f.opacidade, cor)
				}
			}
			grande.SetNRGBA(px, py, cor)
		}
	}
	return reduzir(grande, tamanho)
}

// dentroDoRetanguloArredondado reproduz o rect rx=6 do SVG.
func dentroDoRetanguloArredondado(x, y float64) bool {
	if x < 0 || y < 0 || x > lado || y > lado {
		return false
	}
	// Centro do arco do canto mais proximo, quando o ponto esta na quina.
	cx, cy := x, y
	switch {
	case x < raio:
		cx = raio
	case x > lado-raio:
		cx = lado - raio
	default:
		return true // fora das faixas de canto: o lado reto ja decide
	}
	switch {
	case y < raio:
		cy = raio
	case y > lado-raio:
		cy = lado - raio
	default:
		return true
	}
	dx, dy := x-cx, y-cy
	return dx*dx+dy*dy <= raio*raio
}

// dentroDoPoligono usa o numero de cruzamentos: conta quantas arestas um raio
// horizontal partindo do ponto atravessa.
func dentroDoPoligono(x, y float64, pts []ponto) bool {
	dentro := false
	for i, j := 0, len(pts)-1; i < len(pts); j, i = i, i+1 {
		pi, pj := pts[i], pts[j]
		if (pi.y > y) == (pj.y > y) {
			continue // a aresta nao cruza a altura do ponto
		}
		corte := (pj.x-pi.x)*(y-pi.y)/(pj.y-pi.y) + pi.x
		if x < corte {
			dentro = !dentro
		}
	}
	return dentro
}

// sobrepor mistura frente sobre fundo com a opacidade dada. O fundo aqui e
// sempre opaco, entao basta interpolar os canais de cor.
func sobrepor(frente color.NRGBA, alfa float64, fundo color.NRGBA) color.NRGBA {
	mistura := func(f, t uint8) uint8 {
		return uint8(float64(f)*alfa + float64(t)*(1-alfa) + 0.5)
	}
	return color.NRGBA{
		R: mistura(frente.R, fundo.R),
		G: mistura(frente.G, fundo.G),
		B: mistura(frente.B, fundo.B),
		A: fundo.A,
	}
}

// reduzir faz a media de cada bloco de superAmostragem x superAmostragem.
//
// A media e feita com a cor ja multiplicada pelo alfa; sem isso, os pixels
// transparentes da borda arrastariam a cor deles para dentro e o contorno
// arredondado ficaria com um halo escuro.
func reduzir(grande *image.NRGBA, tamanho int) *image.NRGBA {
	out := image.NewNRGBA(image.Rect(0, 0, tamanho, tamanho))
	n := superAmostragem * superAmostragem

	for y := range tamanho {
		for x := range tamanho {
			var somaR, somaG, somaB, somaA float64
			for dy := range superAmostragem {
				for dx := range superAmostragem {
					c := grande.NRGBAAt(x*superAmostragem+dx, y*superAmostragem+dy)
					a := float64(c.A) / 255
					somaR += float64(c.R) * a
					somaG += float64(c.G) * a
					somaB += float64(c.B) * a
					somaA += a
				}
			}
			if somaA == 0 {
				continue // bloco inteiro transparente
			}
			out.SetNRGBA(x, y, color.NRGBA{
				R: uint8(somaR/somaA + 0.5),
				G: uint8(somaG/somaA + 0.5),
				B: uint8(somaB/somaA + 0.5),
				A: uint8(somaA/float64(n)*255 + 0.5),
			})
		}
	}
	return out
}

// gravarICO monta o container .ico com um PNG por tamanho.
//
// PNG dentro de .ico e entendido desde o Windows Vista e evita o BMP de
// cabeca para baixo com mascara AND que o formato original exige.
func gravarICO(caminho string, imagens []*image.NRGBA) error {
	if err := os.MkdirAll(filepath.Dir(caminho), 0o755); err != nil {
		return err
	}

	// Os PNGs sao codificados antes: o indice precisa do tamanho de cada um
	// para calcular os deslocamentos.
	blocos := make([][]byte, len(imagens))
	for i, img := range imagens {
		var b bytes.Buffer
		if err := png.Encode(&b, img); err != nil {
			return fmt.Errorf("codificar PNG %dx%d: %w", img.Bounds().Dx(), img.Bounds().Dx(), err)
		}
		blocos[i] = b.Bytes()
	}

	var buf bytes.Buffer
	escreve := func(v any) {
		if err := binary.Write(&buf, binary.LittleEndian, v); err != nil {
			panic(err) // buffer em memoria nao falha
		}
	}

	// ICONDIR
	escreve(uint16(0))            // reservado
	escreve(uint16(1))            // 1 = icone (2 seria cursor)
	escreve(uint16(len(imagens))) //
	desloc := 6 + 16*len(imagens) // cabecalho + um ICONDIRENTRY por imagem

	for i, img := range imagens {
		lado := img.Bounds().Dx()
		b := byte(lado)
		if lado >= 256 {
			b = 0 // no formato, 0 significa 256
		}
		buf.WriteByte(b)    // largura
		buf.WriteByte(b)    // altura
		buf.WriteByte(0)    // cores na paleta (0 = sem paleta)
		buf.WriteByte(0)    // reservado
		escreve(uint16(1))  // planos de cor
		escreve(uint16(32)) // bits por pixel
		escreve(uint32(len(blocos[i])))
		escreve(uint32(desloc))
		desloc += len(blocos[i])
	}
	for _, b := range blocos {
		buf.Write(b)
	}

	return os.WriteFile(caminho, buf.Bytes(), 0o644)
}
