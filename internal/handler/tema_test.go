package handler

import "testing"

// TestCorValida e um teste de seguranca, nao de usabilidade: a cor e
// interpolada dentro de um bloco <style>, entao tudo fora de #RRGGBB precisa
// cair no padrao em vez de chegar ao HTML.
func TestCorValida(t *testing.T) {
	casos := []struct {
		nome     string
		entrada  string
		esperado string
	}{
		{"hex valido", "#10b981", "#10b981"},
		{"maiusculas sao normalizadas", "#2496ED", "#2496ed"},
		{"espacos em volta", "  #f43f5e  ", "#f43f5e"},
		{"vazio", "", AccentPadrao},
		{"nome de cor CSS", "red", AccentPadrao},
		{"hex curto", "#abc", AccentPadrao},
		{"hex longo demais", "#aabbccdd", AccentPadrao},
		{"sem cerquilha", "2496ed", AccentPadrao},
		{"caractere nao hexadecimal", "#gggggg", AccentPadrao},
		{"injecao de css", "#2496ed;} body{display:none", AccentPadrao},
		{"fechamento de tag style", "#2496ed</style><script>", AccentPadrao},
		{"url em css", "url(http://evil/x)", AccentPadrao},
		{"quebra de linha no meio", "#2496ed\n", AccentPadrao},
		{"expressao", "expression(alert(1))", AccentPadrao},
	}

	for _, c := range casos {
		t.Run(c.nome, func(t *testing.T) {
			if got := CorValida(c.entrada); got != c.esperado {
				t.Errorf("CorValida(%q) = %q; esperado %q", c.entrada, got, c.esperado)
			}
		})
	}
}

func TestEscurecer(t *testing.T) {
	casos := []struct {
		nome     string
		hex      string
		fator    float64
		esperado string
	}{
		{"metade do branco", "#ffffff", 0.5, "#7f7f7f"},
		{"fator usado na interface", "#2496ed", 0.80, "#1c78bd"},
		{"preto continua preto", "#000000", 0.5, "#000000"},
		{"fator 1 nao altera", "#2496ed", 1.0, "#2496ed"},
		{"fator acima de 1 satura em ff", "#808080", 2.0, "#ffffff"},
		{"fator negativo satura em 00", "#2496ed", -1.0, "#000000"},
		{"fator zero zera", "#2496ed", 0, "#000000"},
	}

	for _, c := range casos {
		t.Run(c.nome, func(t *testing.T) {
			if got := Escurecer(c.hex, c.fator); got != c.esperado {
				t.Errorf("Escurecer(%q, %v) = %q; esperado %q", c.hex, c.fator, got, c.esperado)
			}
		})
	}
}

// TestTextoSobre: sem isto, uma cor clara como o ambar ficaria com texto
// branco por cima no icone da marca, praticamente ilegivel.
func TestTextoSobre(t *testing.T) {
	casos := []struct {
		nome     string
		hex      string
		esperado string
	}{
		{"branco pede texto escuro", "#ffffff", "#101828"},
		{"preto pede texto claro", "#000000", "#ffffff"},
		{"azul docker pede texto claro", "#2496ed", "#ffffff"},
		{"ambar pede texto escuro", "#f59e0b", "#101828"},
		{"esmeralda pede texto claro", "#10b981", "#ffffff"},
		{"grafite pede texto claro", "#64748b", "#ffffff"},
		{"amarelo puro pede texto escuro", "#ffff00", "#101828"},
	}

	for _, c := range casos {
		t.Run(c.nome, func(t *testing.T) {
			if got := TextoSobre(c.hex); got != c.esperado {
				t.Errorf("TextoSobre(%q) = %q; esperado %q", c.hex, got, c.esperado)
			}
		})
	}
}

// TestPaletaEhConsistente trava o contrato da paleta oferecida na tela: toda
// cor precisa sobreviver a CorValida, senao a opcao escolhida pelo usuario
// seria silenciosamente trocada pelo padrao.
func TestPaletaEhConsistente(t *testing.T) {
	if len(Paleta) == 0 {
		t.Fatal("Paleta vazia")
	}

	vistosHex := map[string]bool{}
	vistosNome := map[string]bool{}

	for _, c := range Paleta {
		if got := CorValida(c.Hex); got != c.Hex {
			t.Errorf("cor %q da paleta nao sobrevive a CorValida: %q -> %q", c.Nome, c.Hex, got)
		}
		if c.Nome == "" {
			t.Errorf("cor %q sem nome", c.Hex)
		}
		if vistosHex[c.Hex] {
			t.Errorf("hex duplicado na paleta: %q", c.Hex)
		}
		if vistosNome[c.Nome] {
			t.Errorf("nome duplicado na paleta: %q", c.Nome)
		}
		vistosHex[c.Hex], vistosNome[c.Nome] = true, true
	}
}

func TestAccentPadraoEhValido(t *testing.T) {
	if CorValida(AccentPadrao) != AccentPadrao {
		t.Errorf("AccentPadrao %q nao passa na propria validacao", AccentPadrao)
	}
	// A primeira opcao da paleta precisa aparecer selecionada na instalacao nova.
	if Paleta[0].Hex != AccentPadrao {
		t.Errorf("Paleta[0] = %q; esperado coincidir com AccentPadrao %q", Paleta[0].Hex, AccentPadrao)
	}
}

// TestDerivadasDaPaletaNaoEstouram garante que toda cor da paleta gera
// variantes validas, ja que elas tambem vao para dentro do <style>.
func TestDerivadasDaPaletaNaoEstouram(t *testing.T) {
	for _, c := range Paleta {
		escura := Escurecer(c.Hex, 0.80)
		if CorValida(escura) != escura {
			t.Errorf("variante escura de %q = %q; nao e um hex valido", c.Hex, escura)
		}
		texto := TextoSobre(c.Hex)
		if texto != "#101828" && texto != "#ffffff" {
			t.Errorf("TextoSobre(%q) = %q; esperado preto ou branco", c.Hex, texto)
		}
	}
}
