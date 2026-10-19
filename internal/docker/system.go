package docker

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"sort"
)

// ImageList lista as imagens locais, maiores primeiro.
func (c *Client) ImageList(ctx context.Context) ([]Image, error) {
	var out []Image
	if err := c.do(ctx, http.MethodGet, "/images/json", nil, nil, &out); err != nil {
		return nil, err
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Size > out[j].Size })
	return out, nil
}

// VolumeList lista os volumes conhecidos pelo daemon.
func (c *Client) VolumeList(ctx context.Context) ([]Volume, error) {
	var env listaVolumes
	if err := c.do(ctx, http.MethodGet, "/volumes", nil, nil, &env); err != nil {
		return nil, err
	}
	sort.SliceStable(env.Volumes, func(i, j int) bool { return env.Volumes[i].Name < env.Volumes[j].Name })
	return env.Volumes, nil
}

// SystemDF devolve o uso de disco consolidado.
//
// Atencao: e uma chamada cara -- o daemon percorre layers e volumes. Nao
// chamar em laco nem a cada atualizacao de tela.
func (c *Client) SystemDF(ctx context.Context) (DiskUsage, error) {
	var d DiskUsage
	err := c.do(ctx, http.MethodGet, "/system/df", nil, nil, &d)
	return d, err
}

// Recuperavel estima o espaco liberavel por uma limpeza geral.
func (d DiskUsage) Recuperavel() int64 {
	var total int64
	for _, bc := range d.BuildCache {
		if !bc.InUse {
			total += bc.Size
		}
	}
	return total
}

// FormatarBytes devolve o tamanho em unidade legivel.
func FormatarBytes(b int64) string {
	const unidade = 1024
	if b < unidade {
		return fmt.Sprintf("%d B", b)
	}
	div, exp := int64(unidade), 0
	for n := b / unidade; n >= unidade; n /= unidade {
		div *= unidade
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(b)/float64(div), "KMGTPE"[exp])
}

// ImageRemove remove uma imagem.
func (c *Client) ImageRemove(ctx context.Context, id string, force bool) error {
	q := url.Values{}
	if force {
		q.Set("force", "1")
	}
	return c.do(ctx, http.MethodDelete, "/images/"+id, q, nil, nil)
}

// VolumeRemove remove um volume.
func (c *Client) VolumeRemove(ctx context.Context, nome string, force bool) error {
	q := url.Values{}
	if force {
		q.Set("force", "1")
	}
	return c.do(ctx, http.MethodDelete, "/volumes/"+nome, q, nil, nil)
}
