package models

import (
	"testing"

	"cais/internal/compose"
)

func TestFlashVazia(t *testing.T) {
	casos := []struct {
		nome     string
		flash    Flash
		esperado bool
	}{
		{"zero value", Flash{}, true},
		{"so o tipo preenchido", Flash{Tipo: "erro"}, true},
		{"com mensagem", Flash{Tipo: "sucesso", Msg: "pronto"}, false},
		{"mensagem sem tipo", Flash{Msg: "pronto"}, false},
	}

	for _, c := range casos {
		t.Run(c.nome, func(t *testing.T) {
			if got := c.flash.Vazia(); got != c.esperado {
				t.Errorf("Vazia() = %v; esperado %v", got, c.esperado)
			}
		})
	}
}

func TestStackItemTotalEAtiva(t *testing.T) {
	casos := []struct {
		nome  string
		item  StackItem
		total int
		ativa bool
	}{
		{"stack sem servicos", StackItem{}, 0, false},
		{
			"stack cadastrada mas derrubada",
			StackItem{Servicos: []compose.Servico{{State: "exited"}, {State: "exited"}}},
			2, false,
		},
		{
			"stack parcialmente de pe",
			StackItem{
				Servicos: []compose.Servico{{State: "running"}, {State: "exited"}},
				Rodando:  1,
			},
			2, true,
		},
		{
			"stack inteira de pe",
			StackItem{
				Servicos: []compose.Servico{{State: "running"}},
				Rodando:  1,
			},
			1, true,
		},
	}

	for _, c := range casos {
		t.Run(c.nome, func(t *testing.T) {
			if got := c.item.Total(); got != c.total {
				t.Errorf("Total() = %d; esperado %d", got, c.total)
			}
			if got := c.item.Ativa(); got != c.ativa {
				t.Errorf("Ativa() = %v; esperado %v", got, c.ativa)
			}
		})
	}
}
