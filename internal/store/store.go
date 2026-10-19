// Package store cuida do estado local do painel em SQLite: stacks compose
// registradas, favoritos, configuracoes e historico de acoes.
//
// O dialeto e SQLite, nao PostgreSQL: placeholders sao "?", chave e
// INTEGER PRIMARY KEY AUTOINCREMENT e timestamp e TEXT com datetime('now').
package store

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"

	_ "modernc.org/sqlite"
)

// Open abre (criando se preciso) o banco local e aplica as migrations.
func Open(caminho string) (*sql.DB, error) {
	if err := os.MkdirAll(filepath.Dir(caminho), 0o755); err != nil {
		return nil, fmt.Errorf("criar diretorio de dados: %w", err)
	}

	// O DSN do modernc.org/sqlite usa _pragma=nome(valor), diferente do mattn.
	dsn := "file:" + caminho +
		"?_pragma=busy_timeout(5000)" +
		"&_pragma=journal_mode(WAL)" +
		"&_pragma=foreign_keys(1)"

	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("abrir banco: %w", err)
	}

	// O volume de escrita e irrisorio e uma unica conexao elimina
	// "database is locked" sem custo perceptivel.
	db.SetMaxOpenConns(1)

	if err := db.Ping(); err != nil {
		db.Close()
		return nil, fmt.Errorf("ping no banco: %w", err)
	}
	if err := RunMigrations(db); err != nil {
		db.Close()
		return nil, err
	}
	return db, nil
}

// ============================================================
// Migrations
// ============================================================

type migration struct {
	version int
	name    string
	sql     string
}

var migrations = []migration{
	{1, "criar_schema_inicial", sqlSchema},
}

const sqlSchema = `
CREATE TABLE IF NOT EXISTS settings (
  chave TEXT PRIMARY KEY,
  valor TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS compose_stacks (
  id             INTEGER PRIMARY KEY AUTOINCREMENT,
  nome           TEXT NOT NULL,
  caminho_yml    TEXT NOT NULL UNIQUE,
  project_name   TEXT NOT NULL DEFAULT '',
  env_file       TEXT NOT NULL DEFAULT '',
  pasta_trabalho TEXT NOT NULL DEFAULT '',
  ordem          INTEGER NOT NULL DEFAULT 0,
  criado_em      TEXT NOT NULL DEFAULT (datetime('now'))
);

CREATE TABLE IF NOT EXISTS favoritos (
  id        INTEGER PRIMARY KEY AUTOINCREMENT,
  tipo      TEXT NOT NULL,
  ref       TEXT NOT NULL,
  criado_em TEXT NOT NULL DEFAULT (datetime('now')),
  UNIQUE (tipo, ref)
);

CREATE TABLE IF NOT EXISTS action_log (
  id         INTEGER PRIMARY KEY AUTOINCREMENT,
  alvo_tipo  TEXT NOT NULL,
  alvo_id    TEXT NOT NULL DEFAULT '',
  alvo_nome  TEXT NOT NULL DEFAULT '',
  acao       TEXT NOT NULL,
  resultado  TEXT NOT NULL,
  erro       TEXT NOT NULL DEFAULT '',
  duracao_ms INTEGER NOT NULL DEFAULT 0,
  criado_em  TEXT NOT NULL DEFAULT (datetime('now'))
);

CREATE INDEX IF NOT EXISTS idx_action_log_criado_em ON action_log (criado_em DESC);
`

// RunMigrations aplica em ordem as migrations ainda nao registradas.
func RunMigrations(db *sql.DB) error {
	_, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS schema_migrations (
		  version    INTEGER PRIMARY KEY,
		  name       TEXT NOT NULL,
		  aplicada_em TEXT NOT NULL DEFAULT (datetime('now'))
		)`)
	if err != nil {
		return fmt.Errorf("criar schema_migrations: %w", err)
	}

	for _, m := range migrations {
		var existe int
		if err := db.QueryRow(`SELECT COUNT(*) FROM schema_migrations WHERE version = ?`, m.version).Scan(&existe); err != nil {
			return fmt.Errorf("consultar migration %d: %w", m.version, err)
		}
		if existe > 0 {
			continue
		}

		tx, err := db.Begin()
		if err != nil {
			return fmt.Errorf("iniciar migration %d: %w", m.version, err)
		}
		if _, err := tx.Exec(m.sql); err != nil {
			tx.Rollback()
			return fmt.Errorf("aplicar migration %d (%s): %w", m.version, m.name, err)
		}
		if _, err := tx.Exec(`INSERT INTO schema_migrations (version, name) VALUES (?, ?)`, m.version, m.name); err != nil {
			tx.Rollback()
			return fmt.Errorf("registrar migration %d: %w", m.version, err)
		}
		if err := tx.Commit(); err != nil {
			return fmt.Errorf("commit da migration %d: %w", m.version, err)
		}
	}
	return nil
}
