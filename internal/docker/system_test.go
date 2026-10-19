package docker

import (
	"context"
	"net/http"
	"testing"
)

func TestFormatarBytes(t *testing.T) {
	casos := []struct {
		entrada  int64
		esperado string
	}{
		{0, "0 B"},
		{1, "1 B"},
		{512, "512 B"},
		{1023, "1023 B"},
		{1024, "1.0 KB"},
		{1536, "1.5 KB"},
		{1024 * 1024, "1.0 MB"},
		{1024 * 1024 * 1024, "1.0 GB"},
		{1024 * 1024 * 1024 * 1024, "1.0 TB"},
		{1024 * 1024 * 1024 * 1024 * 1024, "1.0 PB"},
		{1024 * 1024 * 1024 * 1024 * 1024 * 1024, "1.0 EB"},
		{1610612736, "1.5 GB"},
	}

	for _, c := range casos {
		if got := FormatarBytes(c.entrada); got != c.esperado {
			t.Errorf("FormatarBytes(%d) = %q; esperado %q", c.entrada, got, c.esperado)
		}
	}
}

// TestRecuperavel confere que so o build cache ocioso entra na conta: cache em
// uso nao e espaco liberavel por uma limpeza.
func TestRecuperavel(t *testing.T) {
	casos := []struct {
		nome     string
		df       DiskUsage
		esperado int64
	}{
		{"sem build cache", DiskUsage{}, 0},
		{
			"so cache em uso",
			DiskUsage{BuildCache: []struct {
				Size  int64 `json:"Size"`
				InUse bool  `json:"InUse"`
			}{{Size: 500, InUse: true}}},
			0,
		},
		{
			"mistura de ocioso e em uso",
			DiskUsage{BuildCache: []struct {
				Size  int64 `json:"Size"`
				InUse bool  `json:"InUse"`
			}{
				{Size: 100, InUse: false},
				{Size: 900, InUse: true},
				{Size: 50, InUse: false},
			}},
			150,
		},
	}

	for _, c := range casos {
		t.Run(c.nome, func(t *testing.T) {
			if got := c.df.Recuperavel(); got != c.esperado {
				t.Errorf("Recuperavel() = %d; esperado %d", got, c.esperado)
			}
		})
	}
}

// TestImageListOrdenaPorTamanho: a tela de imagens existe para liberar espaco,
// entao a maior precisa aparecer primeiro.
func TestImageListOrdenaPorTamanho(t *testing.T) {
	c := clienteFake(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/images/json" {
			t.Errorf("caminho = %q", r.URL.Path)
		}
		w.Write([]byte(`[
			{"Id":"a","RepoTags":["pequena:1"],"Size":100},
			{"Id":"b","RepoTags":["grande:1"],"Size":9000},
			{"Id":"c","RepoTags":["media:1"],"Size":500}
		]`))
	})

	imgs, err := c.ImageList(context.Background())
	if err != nil {
		t.Fatalf("ImageList: %v", err)
	}

	esperado := []string{"grande:1", "media:1", "pequena:1"}
	for i, tag := range esperado {
		if imgs[i].Tag() != tag {
			t.Errorf("posicao %d = %q; esperado %q", i, imgs[i].Tag(), tag)
		}
	}
}

func TestVolumeListOrdenaPorNome(t *testing.T) {
	c := clienteFake(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/volumes" {
			t.Errorf("caminho = %q", r.URL.Path)
		}
		// A API devolve um envelope, nao um array solto.
		w.Write([]byte(`{"Volumes":[
			{"Name":"zeta","Driver":"local"},
			{"Name":"alfa","Driver":"local"},
			{"Name":"beta","Driver":"local"}
		]}`))
	})

	vols, err := c.VolumeList(context.Background())
	if err != nil {
		t.Fatalf("VolumeList: %v", err)
	}

	esperado := []string{"alfa", "beta", "zeta"}
	if len(vols) != len(esperado) {
		t.Fatalf("obtidos %d volumes; esperado %d", len(vols), len(esperado))
	}
	for i, nome := range esperado {
		if vols[i].Name != nome {
			t.Errorf("posicao %d = %q; esperado %q", i, vols[i].Name, nome)
		}
	}
}

func TestVolumeListVazio(t *testing.T) {
	c := clienteFake(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Write([]byte(`{"Volumes":null}`))
	})

	vols, err := c.VolumeList(context.Background())
	if err != nil {
		t.Fatalf("VolumeList: %v", err)
	}
	if len(vols) != 0 {
		t.Errorf("obtidos %d volumes; esperado 0", len(vols))
	}
}

func TestSystemDF(t *testing.T) {
	c := clienteFake(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/system/df" {
			t.Errorf("caminho = %q", r.URL.Path)
		}
		w.Write([]byte(`{
			"LayersSize": 4096,
			"Images":[{"Size":1000},{"Size":2000}],
			"Volumes":[{"UsageData":{"Size":300}}],
			"BuildCache":[{"Size":70,"InUse":false},{"Size":30,"InUse":true}]
		}`))
	})

	df, err := c.SystemDF(context.Background())
	if err != nil {
		t.Fatalf("SystemDF: %v", err)
	}
	if df.LayersSize != 4096 {
		t.Errorf("LayersSize = %d; esperado 4096", df.LayersSize)
	}
	if len(df.Images) != 2 {
		t.Errorf("Images = %d; esperado 2", len(df.Images))
	}
	if got := df.Recuperavel(); got != 70 {
		t.Errorf("Recuperavel() = %d; esperado 70", got)
	}
}
