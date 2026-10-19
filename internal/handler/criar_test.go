package handler

import (
	"reflect"
	"strings"
	"testing"

	"cais/internal/docker"
)

func TestParsePortas(t *testing.T) {
	casos := []struct {
		nome     string
		entrada  string
		esperado []docker.PortaMapeada
	}{
		{"vazio", "", nil},
		{"so espacos e quebras", "\n  \n\n", nil},
		{
			"host e container",
			"8080:80",
			[]docker.PortaMapeada{{Host: "8080", Container: "80", Protocolo: "tcp"}},
		},
		{
			"porta sozinha vale dos dois lados",
			"8080",
			[]docker.PortaMapeada{{Host: "8080", Container: "8080", Protocolo: "tcp"}},
		},
		{
			"protocolo udp",
			"53:53/udp",
			[]docker.PortaMapeada{{Host: "53", Container: "53", Protocolo: "udp"}},
		},
		{
			"protocolo em maiusculas",
			"8080:80/TCP",
			[]docker.PortaMapeada{{Host: "8080", Container: "80", Protocolo: "tcp"}},
		},
		{
			"espacos em volta dos numeros",
			"  8080 : 80  ",
			[]docker.PortaMapeada{{Host: "8080", Container: "80", Protocolo: "tcp"}},
		},
		{
			"varias linhas com brancos no meio",
			"8080:80\n\n5432:5432\n  \n53:53/udp",
			[]docker.PortaMapeada{
				{Host: "8080", Container: "80", Protocolo: "tcp"},
				{Host: "5432", Container: "5432", Protocolo: "tcp"},
				{Host: "53", Container: "53", Protocolo: "udp"},
			},
		},
		{
			"porta sozinha com protocolo",
			"53/udp",
			[]docker.PortaMapeada{{Host: "53", Container: "53", Protocolo: "udp"}},
		},
	}

	for _, c := range casos {
		t.Run(c.nome, func(t *testing.T) {
			got, err := parsePortas(c.entrada)
			if err != nil {
				t.Fatalf("parsePortas(%q): %v", c.entrada, err)
			}
			if !reflect.DeepEqual(got, c.esperado) {
				t.Errorf("parsePortas(%q) = %+v; esperado %+v", c.entrada, got, c.esperado)
			}
		})
	}
}

func TestParsePortasInvalidas(t *testing.T) {
	casos := []struct {
		nome    string
		entrada string
		trecho  string
	}{
		{"protocolo desconhecido", "53:53/sctp", "protocolo"},
		{"host nao numerico", "abc:80", "porta inválida"},
		{"container nao numerico", "8080:web", "porta inválida"},
		{"container ausente", "8080:", "porta inválida"},
		{"host ausente", ":80", "porta inválida"},
		{"porta negativa", "-1:80", "porta inválida"},
		{"intervalo de portas", "8000-8010:80", "porta inválida"},
		{"ip no mapeamento", "127.0.0.1:8080:80", "porta inválida"},
		{"erro na segunda linha interrompe tudo", "8080:80\nabc:80", "porta inválida"},
	}

	for _, c := range casos {
		t.Run(c.nome, func(t *testing.T) {
			got, err := parsePortas(c.entrada)
			if err == nil {
				t.Fatalf("parsePortas(%q) = %+v; esperado erro", c.entrada, got)
			}
			if !strings.Contains(err.Error(), c.trecho) {
				t.Errorf("erro = %q; esperado conter %q", err, c.trecho)
			}
			if got != nil {
				t.Errorf("em caso de erro o resultado deve ser nil; obtido %+v", got)
			}
		})
	}
}

func TestSomenteDigitos(t *testing.T) {
	casos := []struct {
		entrada  string
		esperado bool
	}{
		{"8080", true},
		{"0", true},
		{"007", true},
		{"", false},
		{" 80", false},
		{"80a", false},
		{"-1", false},
		{"8.0", false},
		{"8_0", false},
		{"８０８０", false}, // digitos de largura total nao valem
	}

	for _, c := range casos {
		if got := somenteDigitos(c.entrada); got != c.esperado {
			t.Errorf("somenteDigitos(%q) = %v; esperado %v", c.entrada, got, c.esperado)
		}
	}
}

func TestLinhasNaoVazias(t *testing.T) {
	casos := []struct {
		nome     string
		entrada  string
		esperado []string
	}{
		{"vazio", "", nil},
		{"so brancos", "   \n\t\n  ", nil},
		{"linha unica", "CHAVE=valor", []string{"CHAVE=valor"}},
		{
			"descarta linhas em branco no meio",
			"A=1\n\nB=2\n   \nC=3",
			[]string{"A=1", "B=2", "C=3"},
		},
		{
			"remove o retorno de carro do textarea",
			"A=1\r\nB=2\r\n",
			[]string{"A=1", "B=2"},
		},
		{
			"apara espacos de cada linha",
			"  A=1  \n\tB=2\t",
			[]string{"A=1", "B=2"},
		},
		{
			"preserva espaco interno do valor",
			"MSG=bom dia",
			[]string{"MSG=bom dia"},
		},
	}

	for _, c := range casos {
		t.Run(c.nome, func(t *testing.T) {
			if got := linhasNaoVazias(c.entrada); !reflect.DeepEqual(got, c.esperado) {
				t.Errorf("linhasNaoVazias(%q) = %q; esperado %q", c.entrada, got, c.esperado)
			}
		})
	}
}
