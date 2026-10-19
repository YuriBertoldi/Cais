package handler

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// AccentPadrao e o azul oficial do Docker.
const AccentPadrao = "#2496ed"

// chaveAccent e a chave da cor de destaque na tabela settings.
const chaveAccent = "accent"

// Paleta e a lista de cores oferecidas na tela de configuracoes.
//
// Todas foram escolhidas com saturacao parecida, para que o painel mantenha o
// mesmo peso visual qualquer que seja a escolha.
var Paleta = []CorPreset{
	{"Azul Docker", "#2496ed"},
	{"Índigo", "#6366f1"},
	{"Violeta", "#8b5cf6"},
	{"Turquesa", "#14b8a6"},
	{"Esmeralda", "#10b981"},
	{"Âmbar", "#f59e0b"},
	{"Coral", "#f43f5e"},
	{"Grafite", "#64748b"},
}

// CorPreset e uma opcao pronta da paleta.
type CorPreset struct {
	Nome string
	Hex  string
}

// reHex aceita apenas #RRGGBB.
//
// A validacao e de seguranca, nao de usabilidade: o valor e interpolado dentro
// de um bloco <style>, entao qualquer coisa fora deste formato seria uma porta
// de injecao de CSS.
var reHex = regexp.MustCompile(`^#[0-9a-fA-F]{6}$`)

// CorValida normaliza a cor informada, caindo no padrao quando invalida.
func CorValida(s string) string {
	s = strings.TrimSpace(strings.ToLower(s))
	if !reHex.MatchString(s) {
		return AccentPadrao
	}
	return s
}

// rgb decompoe um hex ja validado.
func rgb(hex string) (r, g, b int) {
	v, _ := strconv.ParseUint(hex[1:], 16, 32)
	return int(v>>16) & 0xff, int(v>>8) & 0xff, int(v) & 0xff
}

// Escurecer devolve uma variante mais escura, usada em hover e gradiente.
func Escurecer(hex string, fator float64) string {
	r, g, b := rgb(hex)
	f := func(c int) int {
		v := int(float64(c) * fator)
		if v < 0 {
			return 0
		}
		if v > 255 {
			return 255
		}
		return v
	}
	return fmt.Sprintf("#%02x%02x%02x", f(r), f(g), f(b))
}

// TextoSobre devolve preto ou branco, o que tiver mais contraste sobre a cor.
//
// Sem isto, uma cor clara como o âmbar ficaria com texto branco por cima no
// icone da marca, praticamente ilegivel.
func TextoSobre(hex string) string {
	r, g, b := rgb(hex)
	// Luminancia relativa aproximada (coeficientes ITU-R BT.601).
	lum := (0.299*float64(r) + 0.587*float64(g) + 0.114*float64(b)) / 255
	if lum > 0.6 {
		return "#101828"
	}
	return "#ffffff"
}
