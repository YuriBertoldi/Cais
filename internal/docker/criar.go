package docker

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

// ============================================================
// Volumes
// ============================================================

// VolumeCreate cria um volume nomeado.
func (c *Client) VolumeCreate(ctx context.Context, nome, driver string) (Volume, error) {
	if driver == "" {
		driver = "local"
	}
	body := map[string]any{"Name": nome, "Driver": driver}

	var v Volume
	err := c.do(ctx, http.MethodPost, "/volumes/create", nil, body, &v)
	return v, err
}

// ============================================================
// Imagens
// ============================================================

// ProgressoPull e uma linha do stream de progresso do pull.
type ProgressoPull struct {
	Status    string `json:"status"`
	ID        string `json:"id"`
	Progresso string `json:"progress"`
	Erro      string `json:"error"`
}

// ImagePull baixa uma imagem, entregando o progresso linha a linha.
//
// A Engine API responde em NDJSON: um objeto JSON por linha, emitido conforme
// cada camada avanca. A funcao so retorna quando o download termina ou o
// context e cancelado.
func (c *Client) ImagePull(ctx context.Context, ref string, fn func(ProgressoPull)) error {
	imagem, tag := separarTag(ref)

	q := url.Values{}
	q.Set("fromImage", imagem)
	q.Set("tag", tag)

	req, err := c.montarReq(ctx, http.MethodPost, "/images/create", q, nil)
	if err != nil {
		return err
	}
	// Sem timeout: baixar uma imagem grande leva minutos.
	cli := &http.Client{Transport: c.http.Transport}
	resp, err := cli.Do(req)
	if err != nil {
		return fmt.Errorf("iniciar download: %w", err)
	}
	defer resp.Body.Close()

	if err := erroDaResposta(resp); err != nil {
		return err
	}

	sc := bufio.NewScanner(resp.Body)
	sc.Buffer(make([]byte, 0, 64<<10), 1<<20)
	for sc.Scan() {
		linha := strings.TrimSpace(sc.Text())
		if linha == "" {
			continue
		}
		var p ProgressoPull
		if err := json.Unmarshal([]byte(linha), &p); err != nil {
			continue
		}
		// O daemon devolve HTTP 200 e so depois reporta a falha no corpo --
		// tipicamente "manifest unknown" para uma tag que nao existe.
		if p.Erro != "" {
			return fmt.Errorf("%s", p.Erro)
		}
		fn(p)
	}
	if err := sc.Err(); err != nil && err != io.EOF {
		return err
	}
	return nil
}

// separarTag divide "nginx:1.25" em ("nginx", "1.25").
//
// Cuidado com registry que traz porta: em "localhost:5000/app" os dois pontos
// pertencem ao host, nao a tag. Por isso a busca e pelo ultimo ":" e so vale
// se nao houver "/" depois dele.
//
// Referencia por digest ("nginx@sha256:abc...") tem o proprio separador. O "@"
// vem antes do ":" do algoritmo, entao a regra do ultimo ":" partiria no lugar
// errado e pediria ao daemon a tag "abc..." da imagem "nginx@sha256". O digest
// inteiro, com o prefixo do algoritmo, vai no lugar da tag -- e o que a Engine
// API espera.
func separarTag(ref string) (string, string) {
	ref = strings.TrimSpace(ref)

	// i > 0 e i < len-1 descartam "@tudo" e "nginx@", que nao sao referencias
	// validas e nao devem virar um pull de imagem sem nome ou sem tag.
	if i := strings.LastIndex(ref, "@"); i > 0 && i < len(ref)-1 {
		return ref[:i], ref[i+1:]
	}

	i := strings.LastIndex(ref, ":")
	if i < 0 || strings.Contains(ref[i:], "/") {
		return ref, "latest"
	}
	return ref[:i], ref[i+1:]
}

// ============================================================
// Containers
// ============================================================

// PortaMapeada e uma publicacao de porta pedida na criacao.
type PortaMapeada struct {
	Host      string // porta no host
	Container string // porta dentro do container
	Protocolo string // tcp | udp
}

// NovoContainer reune o que a tela de criacao coleta.
type NovoContainer struct {
	Nome    string
	Imagem  string
	Env     []string // "CHAVE=valor"
	Portas  []PortaMapeada
	Volumes []string // "nome:/caminho" ou "/host:/caminho"
	Restart string   // no | always | unless-stopped | on-failure
	Cmd     []string
}

// ContainerCreate cria o container e devolve o id. Nao inicia.
func (c *Client) ContainerCreate(ctx context.Context, n NovoContainer) (string, error) {
	expostas := map[string]struct{}{}
	bindings := map[string][]map[string]string{}

	for _, p := range n.Portas {
		proto := p.Protocolo
		if proto == "" {
			proto = "tcp"
		}
		chave := p.Container + "/" + proto
		expostas[chave] = struct{}{}
		bindings[chave] = append(bindings[chave], map[string]string{
			// Publica so em loopback: um container de desenvolvimento nao
			// deveria ficar exposto na rede da empresa sem alguem pedir.
			"HostIp":   "127.0.0.1",
			"HostPort": p.Host,
		})
	}

	restart := n.Restart
	if restart == "" {
		restart = "no"
	}

	hostConfig := map[string]any{
		"PortBindings":  bindings,
		"RestartPolicy": map[string]any{"Name": restart},
	}
	if len(n.Volumes) > 0 {
		hostConfig["Binds"] = n.Volumes
	}

	body := map[string]any{
		"Image":        n.Imagem,
		"ExposedPorts": expostas,
		"HostConfig":   hostConfig,
	}
	if len(n.Env) > 0 {
		body["Env"] = n.Env
	}
	if len(n.Cmd) > 0 {
		body["Cmd"] = n.Cmd
	}

	q := url.Values{}
	if n.Nome != "" {
		q.Set("name", n.Nome)
	}

	var out struct {
		ID       string   `json:"Id"`
		Warnings []string `json:"Warnings"`
	}
	if err := c.do(ctx, http.MethodPost, "/containers/create", q, body, &out); err != nil {
		return "", err
	}
	return out.ID, nil
}

// ImageExiste informa se a imagem ja esta local.
func (c *Client) ImageExiste(ctx context.Context, ref string) bool {
	return c.do(ctx, http.MethodGet, "/images/"+url.PathEscape(ref)+"/json", nil, nil, nil) == nil
}
