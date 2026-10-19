package store

import (
	"database/sql"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"cais/internal/models"
)

// bancoTemp abre um SQLite descartavel, ja migrado.
func bancoTemp(t *testing.T) *sql.DB {
	t.Helper()

	caminho := filepath.Join(t.TempDir(), "painel.db")
	db, err := Open(caminho)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

// TestOpenCriaDiretorio: o caminho padrao fica em %LOCALAPPDATA%\Cais, que
// nao existe na primeira execucao.
func TestOpenCriaDiretorio(t *testing.T) {
	caminho := filepath.Join(t.TempDir(), "sub", "pasta", "painel.db")

	db, err := Open(caminho)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer db.Close()

	if _, err := os.Stat(caminho); err != nil {
		t.Errorf("arquivo do banco nao foi criado: %v", err)
	}
}

func TestOpenComDiretorioInvalido(t *testing.T) {
	// Um arquivo comum no lugar onde o diretorio precisaria existir.
	arquivo := filepath.Join(t.TempDir(), "arquivo.txt")
	if err := os.WriteFile(arquivo, []byte("x"), 0o644); err != nil {
		t.Fatalf("preparar arquivo: %v", err)
	}

	db, err := Open(filepath.Join(arquivo, "painel.db"))
	if err == nil {
		db.Close()
		t.Fatal("esperado erro ao criar diretorio sobre um arquivo")
	}
}

func TestOpenAplicaSchema(t *testing.T) {
	db := bancoTemp(t)

	tabelas := []string{"settings", "compose_stacks", "favoritos", "action_log", "schema_migrations"}
	for _, nome := range tabelas {
		var existe string
		err := db.QueryRow(
			`SELECT name FROM sqlite_master WHERE type='table' AND name = ?`, nome).Scan(&existe)
		if err != nil {
			t.Errorf("tabela %q ausente: %v", nome, err)
		}
	}
}

// TestMigrationsSaoIdempotentes: toda abertura do painel roda as migrations;
// rodar duas vezes nao pode duplicar registro nem falhar.
func TestMigrationsSaoIdempotentes(t *testing.T) {
	db := bancoTemp(t)

	for i := range 3 {
		if err := RunMigrations(db); err != nil {
			t.Fatalf("RunMigrations na %da vez: %v", i+1, err)
		}
	}

	var aplicadas int
	if err := db.QueryRow(`SELECT COUNT(*) FROM schema_migrations`).Scan(&aplicadas); err != nil {
		t.Fatalf("contar migrations: %v", err)
	}
	if aplicadas != len(migrations) {
		t.Errorf("migrations registradas = %d; esperado %d", aplicadas, len(migrations))
	}
}

func TestReabrirBancoPreservaDados(t *testing.T) {
	caminho := filepath.Join(t.TempDir(), "painel.db")

	db, err := Open(caminho)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if err := SetSetting(db, "accent", "#10b981"); err != nil {
		t.Fatalf("SetSetting: %v", err)
	}
	db.Close()

	db2, err := Open(caminho)
	if err != nil {
		t.Fatalf("reabrir: %v", err)
	}
	defer db2.Close()

	if got := GetSetting(db2, "accent", "#2496ed"); got != "#10b981" {
		t.Errorf("accent apos reabrir = %q; esperado #10b981", got)
	}
}

// ============================================================
// Settings
// ============================================================

func TestGetSettingDevolvePadraoQuandoAusente(t *testing.T) {
	db := bancoTemp(t)

	if got := GetSetting(db, "inexistente", "padrao"); got != "padrao" {
		t.Errorf("GetSetting = %q; esperado \"padrao\"", got)
	}
}

func TestSetSettingGravaESobrescreve(t *testing.T) {
	db := bancoTemp(t)

	if err := SetSetting(db, "accent", "#6366f1"); err != nil {
		t.Fatalf("SetSetting: %v", err)
	}
	if got := GetSetting(db, "accent", "#2496ed"); got != "#6366f1" {
		t.Errorf("GetSetting = %q; esperado #6366f1", got)
	}

	// O ON CONFLICT precisa atualizar, nao estourar chave duplicada.
	if err := SetSetting(db, "accent", "#f43f5e"); err != nil {
		t.Fatalf("SetSetting sobrescrevendo: %v", err)
	}
	if got := GetSetting(db, "accent", "#2496ed"); got != "#f43f5e" {
		t.Errorf("GetSetting apos sobrescrita = %q; esperado #f43f5e", got)
	}

	var linhas int
	db.QueryRow(`SELECT COUNT(*) FROM settings WHERE chave = 'accent'`).Scan(&linhas)
	if linhas != 1 {
		t.Errorf("linhas de accent = %d; esperado 1", linhas)
	}
}

func TestSetSettingAceitaValorVazio(t *testing.T) {
	db := bancoTemp(t)

	if err := SetSetting(db, "chave", ""); err != nil {
		t.Fatalf("SetSetting: %v", err)
	}
	if got := GetSetting(db, "chave", "padrao"); got != "" {
		t.Errorf("GetSetting = %q; esperado vazio (o valor gravado, nao o padrao)", got)
	}
}

// ============================================================
// Stacks
// ============================================================

func TestCreateEListStacks(t *testing.T) {
	db := bancoTemp(t)

	entrada := []models.Stack{
		{Nome: "zeta", CaminhoYML: `C:\z\compose.yml`, Ordem: 1},
		{Nome: "alfa", CaminhoYML: `C:\a\compose.yml`, Ordem: 1},
		{Nome: "primeira", CaminhoYML: `C:\p\compose.yml`, Ordem: 0},
	}
	for _, s := range entrada {
		if err := CreateStack(db, s); err != nil {
			t.Fatalf("CreateStack %q: %v", s.Nome, err)
		}
	}

	stacks, err := ListStacks(db)
	if err != nil {
		t.Fatalf("ListStacks: %v", err)
	}

	// ORDER BY ordem, nome.
	esperado := []string{"primeira", "alfa", "zeta"}
	if len(stacks) != len(esperado) {
		t.Fatalf("obtidas %d stacks; esperado %d", len(stacks), len(esperado))
	}
	for i, nome := range esperado {
		if stacks[i].Nome != nome {
			t.Errorf("posicao %d = %q; esperado %q", i, stacks[i].Nome, nome)
		}
	}
	if stacks[0].CriadoEm.IsZero() {
		t.Error("CriadoEm ficou zerado; o datetime('now') do SQLite nao foi interpretado")
	}
}

func TestListStacksVazio(t *testing.T) {
	stacks, err := ListStacks(bancoTemp(t))
	if err != nil {
		t.Fatalf("ListStacks: %v", err)
	}
	if len(stacks) != 0 {
		t.Errorf("obtidas %d stacks; esperado 0", len(stacks))
	}
}

func TestCreateStackPreservaCamposOpcionais(t *testing.T) {
	db := bancoTemp(t)

	original := models.Stack{
		Nome:          "app",
		CaminhoYML:    `C:\app\docker-compose.yml`,
		ProjectName:   "meuapp",
		EnvFile:       `C:\app\.env`,
		PastaTrabalho: `C:\app`,
		Ordem:         7,
	}
	if err := CreateStack(db, original); err != nil {
		t.Fatalf("CreateStack: %v", err)
	}

	stacks, err := ListStacks(db)
	if err != nil {
		t.Fatalf("ListStacks: %v", err)
	}
	s := stacks[0]

	if s.ProjectName != original.ProjectName || s.EnvFile != original.EnvFile ||
		s.PastaTrabalho != original.PastaTrabalho || s.Ordem != original.Ordem {
		t.Errorf("stack recuperada = %+v; esperado %+v", s, original)
	}
	if s.ID == 0 {
		t.Error("ID nao foi atribuido")
	}
}

// TestCreateStackRejeitaCaminhoDuplicado: o UNIQUE em caminho_yml e o que a
// tela usa para dizer "essa stack ja esta cadastrada".
func TestCreateStackRejeitaCaminhoDuplicado(t *testing.T) {
	db := bancoTemp(t)

	s := models.Stack{Nome: "app", CaminhoYML: `C:\app\compose.yml`}
	if err := CreateStack(db, s); err != nil {
		t.Fatalf("primeiro CreateStack: %v", err)
	}

	err := CreateStack(db, models.Stack{Nome: "outro nome", CaminhoYML: `C:\app\compose.yml`})
	if err == nil {
		t.Fatal("esperado erro de chave duplicada")
	}
	if !strings.Contains(strings.ToUpper(err.Error()), "UNIQUE") {
		t.Errorf("erro = %q; o handler depende da palavra UNIQUE para a mensagem de tela", err)
	}
}

func TestGetStack(t *testing.T) {
	db := bancoTemp(t)

	if err := CreateStack(db, models.Stack{Nome: "app", CaminhoYML: `C:\app\compose.yml`}); err != nil {
		t.Fatalf("CreateStack: %v", err)
	}
	stacks, _ := ListStacks(db)

	s, err := GetStack(db, stacks[0].ID)
	if err != nil {
		t.Fatalf("GetStack: %v", err)
	}
	if s.Nome != "app" || s.CaminhoYML != `C:\app\compose.yml` {
		t.Errorf("stack = %+v", s)
	}
}

func TestGetStackInexistente(t *testing.T) {
	if _, err := GetStack(bancoTemp(t), 999); err == nil {
		t.Fatal("esperado erro para id inexistente")
	}
}

func TestDeleteStack(t *testing.T) {
	db := bancoTemp(t)

	if err := CreateStack(db, models.Stack{Nome: "app", CaminhoYML: `C:\app\compose.yml`}); err != nil {
		t.Fatalf("CreateStack: %v", err)
	}
	stacks, _ := ListStacks(db)

	if err := DeleteStack(db, stacks[0].ID); err != nil {
		t.Fatalf("DeleteStack: %v", err)
	}

	restantes, _ := ListStacks(db)
	if len(restantes) != 0 {
		t.Errorf("sobraram %d stacks; esperado 0", len(restantes))
	}

	// Apagar duas vezes nao e erro: o DELETE simplesmente nao casa nada.
	if err := DeleteStack(db, stacks[0].ID); err != nil {
		t.Errorf("DeleteStack repetido: %v", err)
	}
}

// ============================================================
// Historico de acoes
// ============================================================

func TestLogActionEListActions(t *testing.T) {
	db := bancoTemp(t)

	acoes := []models.Action{
		{AlvoTipo: "container", AlvoID: "abc", AlvoNome: "web", Acao: "start", Resultado: "ok", DuracaoMS: 120},
		{AlvoTipo: "image", AlvoID: "sha256:1", AlvoNome: "nginx", Acao: "remove", Resultado: "erro", Erro: "em uso"},
		{AlvoTipo: "stack", AlvoID: "1", AlvoNome: "app", Acao: "up", Resultado: "ok", DuracaoMS: 5000},
	}
	for _, a := range acoes {
		if err := LogAction(db, a); err != nil {
			t.Fatalf("LogAction: %v", err)
		}
	}

	lidas, err := ListActions(db, 10)
	if err != nil {
		t.Fatalf("ListActions: %v", err)
	}
	if len(lidas) != 3 {
		t.Fatalf("obtidas %d acoes; esperado 3", len(lidas))
	}

	// Mais recentes primeiro.
	if lidas[0].Acao != "up" || lidas[2].Acao != "start" {
		t.Errorf("ordem = %q, %q, %q; esperado up, remove, start",
			lidas[0].Acao, lidas[1].Acao, lidas[2].Acao)
	}
	if lidas[1].Erro != "em uso" || lidas[1].Resultado != "erro" {
		t.Errorf("acao com erro = %+v", lidas[1])
	}
	if lidas[2].DuracaoMS != 120 {
		t.Errorf("DuracaoMS = %d; esperado 120", lidas[2].DuracaoMS)
	}
	if lidas[0].CriadoEm.IsZero() {
		t.Error("CriadoEm ficou zerado")
	}
	if lidas[0].ID == 0 {
		t.Error("ID nao foi atribuido")
	}
}

func TestListActionsAplicaLimite(t *testing.T) {
	db := bancoTemp(t)

	for range 60 {
		if err := LogAction(db, models.Action{AlvoTipo: "system", Acao: "prune", Resultado: "ok"}); err != nil {
			t.Fatalf("LogAction: %v", err)
		}
	}

	casos := []struct {
		limite   int
		esperado int
	}{
		{10, 10},
		{60, 60},
		{100, 60}, // nao inventa linhas
		{0, 50},   // limite invalido cai no padrao
		{-1, 50},
	}

	for _, c := range casos {
		lidas, err := ListActions(db, c.limite)
		if err != nil {
			t.Fatalf("ListActions(%d): %v", c.limite, err)
		}
		if len(lidas) != c.esperado {
			t.Errorf("ListActions(%d) devolveu %d; esperado %d", c.limite, len(lidas), c.esperado)
		}
	}
}

func TestListActionsVazio(t *testing.T) {
	lidas, err := ListActions(bancoTemp(t), 10)
	if err != nil {
		t.Fatalf("ListActions: %v", err)
	}
	if len(lidas) != 0 {
		t.Errorf("obtidas %d acoes; esperado 0", len(lidas))
	}
}

// TestLogActionComCamposVazios: a maioria das acoes nao preenche AlvoID nem
// Erro, e o schema tem DEFAULT ” para isso. Nenhum campo pode virar NULL,
// senao o Scan em string quebra na leitura.
func TestLogActionComCamposVazios(t *testing.T) {
	db := bancoTemp(t)

	if err := LogAction(db, models.Action{AlvoTipo: "system", Acao: "prune", Resultado: "ok"}); err != nil {
		t.Fatalf("LogAction: %v", err)
	}

	lidas, err := ListActions(db, 1)
	if err != nil {
		t.Fatalf("ListActions: %v", err)
	}
	if lidas[0].AlvoID != "" || lidas[0].AlvoNome != "" || lidas[0].Erro != "" {
		t.Errorf("campos opcionais = %+v; esperado strings vazias", lidas[0])
	}
}
