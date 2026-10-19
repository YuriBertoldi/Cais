package store

import (
	"database/sql"
	"time"

	"cais/internal/models"
)

// ListStacks devolve as stacks cadastradas, na ordem definida pelo usuario.
func ListStacks(db *sql.DB) ([]models.Stack, error) {
	rows, err := db.Query(`
		SELECT id, nome, caminho_yml, project_name, env_file, pasta_trabalho, ordem, criado_em
		FROM compose_stacks ORDER BY ordem, nome`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []models.Stack
	for rows.Next() {
		s, err := scanStack(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

// GetStack busca uma stack pelo id.
func GetStack(db *sql.DB, id int64) (models.Stack, error) {
	row := db.QueryRow(`
		SELECT id, nome, caminho_yml, project_name, env_file, pasta_trabalho, ordem, criado_em
		FROM compose_stacks WHERE id = ?`, id)
	return scanStack(row)
}

// CreateStack cadastra uma stack nova.
func CreateStack(db *sql.DB, s models.Stack) error {
	_, err := db.Exec(`
		INSERT INTO compose_stacks (nome, caminho_yml, project_name, env_file, pasta_trabalho, ordem)
		VALUES (?, ?, ?, ?, ?, ?)`,
		s.Nome, s.CaminhoYML, s.ProjectName, s.EnvFile, s.PastaTrabalho, s.Ordem)
	return err
}

// DeleteStack remove o cadastro. Nao toca nos containers nem no arquivo.
func DeleteStack(db *sql.DB, id int64) error {
	_, err := db.Exec(`DELETE FROM compose_stacks WHERE id = ?`, id)
	return err
}

// scanner cobre *sql.Row e *sql.Rows, que compartilham a assinatura de Scan.
type scanner interface{ Scan(dest ...any) error }

func scanStack(sc scanner) (models.Stack, error) {
	var s models.Stack
	var criado string
	err := sc.Scan(&s.ID, &s.Nome, &s.CaminhoYML, &s.ProjectName,
		&s.EnvFile, &s.PastaTrabalho, &s.Ordem, &criado)
	if err != nil {
		return s, err
	}
	if t, err := time.Parse("2006-01-02 15:04:05", criado); err == nil {
		s.CriadoEm = t.UTC().Local()
	}
	return s, nil
}
