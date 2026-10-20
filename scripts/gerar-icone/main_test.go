package main

import (
	"encoding/binary"
	"image"
	"image/color"
	"os"
	"path/filepath"
	"testing"
)

// TestDentroDoRetanguloArredondado cobre o recorte da marca: o canto tem que
// ficar de fora, senao o icone vira um quadrado duro em vez de arredondado.
func TestDentroDoRetanguloArredondado(t *testing.T) {
	casos := []struct {
		nome     string
		x, y     float64
		esperado bool
	}{
		{"centro", 12, 12, true},
		{"meio da borda de cima", 12, 0.1, true},
		{"meio da borda da esquerda", 0.1, 12, true},
		{"quina superior esquerda", 0.2, 0.2, false},
		{"quina superior direita", 23.8, 0.2, false},
		{"quina inferior esquerda", 0.2, 23.8, false},
		{"quina inferior direita", 23.8, 23.8, false},
		{"dentro do arco do canto", raio, raio, true},
		{"fora a esquerda", -1, 12, false},
		{"fora abaixo", 12, 25, false},
	}

	for _, c := range casos {
		t.Run(c.nome, func(t *testing.T) {
			if got := dentroDoRetanguloArredondado(c.x, c.y); got != c.esperado {
				t.Errorf("dentroDoRetanguloArredondado(%v, %v) = %v; esperado %v",
					c.x, c.y, got, c.esperado)
			}
		})
	}
}

func TestDentroDoPoligono(t *testing.T) {
	// A face de cima do cubo, um losango em volta de (12, 8.7).
	topo := faces[0].pontos

	casos := []struct {
		nome     string
		x, y     float64
		esperado bool
	}{
		{"centro da face", 12, 8.7, true},
		{"acima da face", 12, 4, false},
		{"abaixo da face", 12, 14, false},
		{"a esquerda da face", 4, 8.7, false},
		{"a direita da face", 20, 8.7, false},
		{"fora da quina do losango", 6, 6, false},
	}

	for _, c := range casos {
		t.Run(c.nome, func(t *testing.T) {
			if got := dentroDoPoligono(c.x, c.y, topo); got != c.esperado {
				t.Errorf("dentroDoPoligono(%v, %v) = %v; esperado %v", c.x, c.y, got, c.esperado)
			}
		})
	}
}

func TestSobrepor(t *testing.T) {
	preto := color.NRGBA{R: 0, G: 0, B: 0, A: 0xff}
	branco := color.NRGBA{R: 0xff, G: 0xff, B: 0xff, A: 0xff}

	casos := []struct {
		nome  string
		alfa  float64
		canal uint8
	}{
		{"opaco devolve a frente", 1.0, 0xff},
		{"transparente devolve o fundo", 0.0, 0x00},
		{"meio a meio", 0.5, 0x80},
	}

	for _, c := range casos {
		t.Run(c.nome, func(t *testing.T) {
			got := sobrepor(branco, c.alfa, preto)
			if got.R != c.canal {
				t.Errorf("R = %#x; esperado %#x", got.R, c.canal)
			}
			if got.A != 0xff {
				t.Errorf("A = %#x; o alfa do fundo deveria ser preservado", got.A)
			}
		})
	}
}

func TestDesenhar(t *testing.T) {
	img := desenhar(32)

	if l := img.Bounds().Dx(); l != 32 {
		t.Fatalf("largura = %d; esperado 32", l)
	}
	if a := img.Bounds().Dy(); a != 32 {
		t.Fatalf("altura = %d; esperado 32", a)
	}

	// O centro cai na face de cima do cubo: opaco e claro.
	centro := img.NRGBAAt(16, 16)
	if centro.A != 0xff {
		t.Errorf("centro com alfa %#x; esperado opaco", centro.A)
	}

	// A quina e recortada pelo raio: tem que sair transparente.
	if quina := img.NRGBAAt(0, 0); quina.A != 0 {
		t.Errorf("quina com alfa %#x; esperado transparente", quina.A)
	}

	// Uma borda reta continua dentro da marca, com a cor de fundo.
	borda := img.NRGBAAt(16, 1)
	if borda.A != 0xff {
		t.Errorf("meio da borda com alfa %#x; esperado opaco", borda.A)
	}
	if borda.R != corFundo.R || borda.G != corFundo.G || borda.B != corFundo.B {
		t.Errorf("meio da borda = %+v; esperado a cor de fundo %+v", borda, corFundo)
	}
}

// TestGravarICOEstrutura confere o container: cabecalho, indice e
// deslocamentos. Um deslocamento errado produz um .ico que o Windows aceita
// em silencio e desenha errado.
func TestGravarICOEstrutura(t *testing.T) {
	// O subdiretorio tambem exercita a criacao da pasta de destino.
	destino := filepath.Join(t.TempDir(), "sub", "teste.ico")
	tam := []int{16, 32}

	var imagens []*image.NRGBA
	for _, s := range tam {
		imagens = append(imagens, desenhar(s))
	}

	if err := gravarICO(destino, imagens); err != nil {
		t.Fatalf("gravarICO: %v", err)
	}

	d, err := os.ReadFile(destino)
	if err != nil {
		t.Fatalf("ler .ico: %v", err)
	}

	if tipo := binary.LittleEndian.Uint16(d[2:4]); tipo != 1 {
		t.Errorf("tipo = %d; esperado 1 (icone)", tipo)
	}
	n := int(binary.LittleEndian.Uint16(d[4:6]))
	if n != len(imagens) {
		t.Fatalf("entradas no indice = %d; esperado %d", n, len(imagens))
	}

	for i, esperado := range tam {
		e := 6 + 16*i
		if l := int(d[e]); l != esperado {
			t.Errorf("entrada %d: largura = %d; esperado %d", i, l, esperado)
		}
		bits := binary.LittleEndian.Uint16(d[e+6 : e+8])
		if bits != 32 {
			t.Errorf("entrada %d: bits por pixel = %d; esperado 32", i, bits)
		}

		tamBloco := binary.LittleEndian.Uint32(d[e+8 : e+12])
		desloc := binary.LittleEndian.Uint32(d[e+12 : e+16])
		if int(desloc)+int(tamBloco) > len(d) {
			t.Fatalf("entrada %d aponta para fora do arquivo", i)
		}
		bloco := d[desloc : desloc+tamBloco]
		if string(bloco[1:4]) != "PNG" {
			t.Errorf("entrada %d nao comeca com assinatura PNG: %q", i, bloco[:8])
		}
	}
}

// TestTamanhoDe256ViraZero: no formato .ico, o byte de dimensao e 0 para 256,
// porque o campo so vai ate 255.
func TestTamanhoDe256ViraZero(t *testing.T) {
	destino := filepath.Join(t.TempDir(), "grande.ico")

	if err := gravarICO(destino, []*image.NRGBA{desenhar(256)}); err != nil {
		t.Fatalf("gravarICO: %v", err)
	}

	d, err := os.ReadFile(destino)
	if err != nil {
		t.Fatalf("ler .ico: %v", err)
	}
	if d[6] != 0 {
		t.Errorf("byte de largura = %d; esperado 0 para 256", d[6])
	}
}
