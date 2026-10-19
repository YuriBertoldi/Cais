package store

import (
	"database/sql"
	"time"

	"cais/internal/models"
)

// LogAction grava uma operacao no historico.
func LogAction(db *sql.DB, a models.Action) error {
	_, err := db.Exec(`
		INSERT INTO action_log (alvo_tipo, alvo_id, alvo_nome, acao, resultado, erro, duracao_ms)
		VALUES (?, ?, ?, ?, ?, ?, ?)`,
		a.AlvoTipo, a.AlvoID, a.AlvoNome, a.Acao, a.Resultado, a.Erro, a.DuracaoMS)
	return err
}

// ListActions devolve as ultimas acoes, mais recentes primeiro.
func ListActions(db *sql.DB, limite int) ([]models.Action, error) {
	if limite <= 0 {
		limite = 50
	}
	rows, err := db.Query(`
		SELECT id, alvo_tipo, alvo_id, alvo_nome, acao, resultado, erro, duracao_ms, criado_em
		FROM action_log ORDER BY id DESC LIMIT ?`, limite)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []models.Action
	for rows.Next() {
		var a models.Action
		var criado string
		if err := rows.Scan(&a.ID, &a.AlvoTipo, &a.AlvoID, &a.AlvoNome,
			&a.Acao, &a.Resultado, &a.Erro, &a.DuracaoMS, &criado); err != nil {
			return nil, err
		}
		// datetime('now') do SQLite grava em UTC, sem timezone.
		if t, err := time.Parse("2006-01-02 15:04:05", criado); err == nil {
			a.CriadoEm = t.UTC().Local()
		}
		out = append(out, a)
	}
	return out, rows.Err()
}
