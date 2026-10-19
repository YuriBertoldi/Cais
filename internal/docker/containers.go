package docker

import (
	"context"
	"net/http"
	"net/url"
	"sort"
	"strconv"
)

// Ping confere se o daemon responde e negocia a versao da API.
//
// Chamar antes de qualquer outra coisa: sem isso o cliente fala com a rota sem
// prefixo de versao, que funciona mas fica a merce de mudanca de default.
func (c *Client) Ping(ctx context.Context) (Version, error) {
	var v Version
	// Deliberadamente sem prefixo: e a chamada que descobre qual usar.
	anterior := c.apiVersion
	c.apiVersion = ""
	err := c.do(ctx, http.MethodGet, "/version", nil, nil, &v)
	if err != nil {
		c.apiVersion = anterior
		return v, err
	}
	if v.APIVersion != "" {
		c.apiVersion = v.APIVersion
	}
	return v, nil
}

// ContainerList lista containers. Com all=false traz apenas os em execucao.
//
// A ordem e estavel (rodando primeiro, depois por nome) para que a tela nao
// embaralhe linhas a cada atualizacao automatica.
func (c *Client) ContainerList(ctx context.Context, all bool) ([]Container, error) {
	q := url.Values{}
	if all {
		q.Set("all", "1")
	}
	var out []Container
	if err := c.do(ctx, http.MethodGet, "/containers/json", q, nil, &out); err != nil {
		return nil, err
	}

	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Rodando() != out[j].Rodando() {
			return out[i].Rodando()
		}
		return out[i].Nome() < out[j].Nome()
	})
	return out, nil
}

// ContainerInspect devolve o detalhe completo de um container.
func (c *Client) ContainerInspect(ctx context.Context, id string) (ContainerDetail, error) {
	var d ContainerDetail
	err := c.do(ctx, http.MethodGet, "/containers/"+id+"/json", nil, nil, &d)
	return d, err
}

// ContainerStart inicia um container parado.
func (c *Client) ContainerStart(ctx context.Context, id string) error {
	return c.do(ctx, http.MethodPost, "/containers/"+id+"/start", nil, nil, nil)
}

// ContainerStop para um container, aguardando timeoutSeg antes do SIGKILL.
func (c *Client) ContainerStop(ctx context.Context, id string, timeoutSeg int) error {
	q := url.Values{}
	if timeoutSeg > 0 {
		q.Set("t", strconv.Itoa(timeoutSeg))
	}
	return c.do(ctx, http.MethodPost, "/containers/"+id+"/stop", q, nil, nil)
}

// ContainerRestart reinicia um container.
func (c *Client) ContainerRestart(ctx context.Context, id string, timeoutSeg int) error {
	q := url.Values{}
	if timeoutSeg > 0 {
		q.Set("t", strconv.Itoa(timeoutSeg))
	}
	return c.do(ctx, http.MethodPost, "/containers/"+id+"/restart", q, nil, nil)
}

// ContainerPause suspende os processos do container.
func (c *Client) ContainerPause(ctx context.Context, id string) error {
	return c.do(ctx, http.MethodPost, "/containers/"+id+"/pause", nil, nil, nil)
}

// ContainerUnpause retoma um container pausado.
func (c *Client) ContainerUnpause(ctx context.Context, id string) error {
	return c.do(ctx, http.MethodPost, "/containers/"+id+"/unpause", nil, nil, nil)
}

// ContainerRemove remove um container. comVolumes apaga tambem os volumes
// anonimos associados -- nunca os nomeados.
func (c *Client) ContainerRemove(ctx context.Context, id string, force, comVolumes bool) error {
	q := url.Values{}
	if force {
		q.Set("force", "1")
	}
	if comVolumes {
		q.Set("v", "1")
	}
	return c.do(ctx, http.MethodDelete, "/containers/"+id, q, nil, nil)
}
