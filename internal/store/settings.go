package store

import "database/sql"

// GetSetting le uma configuracao, devolvendo def quando a chave nao existe.
func GetSetting(db *sql.DB, chave, def string) string {
	var v string
	err := db.QueryRow(`SELECT valor FROM settings WHERE chave = ?`, chave).Scan(&v)
	if err != nil {
		return def
	}
	return v
}

// SetSetting grava (ou sobrescreve) uma configuracao.
func SetSetting(db *sql.DB, chave, valor string) error {
	_, err := db.Exec(`
		INSERT INTO settings (chave, valor) VALUES (?, ?)
		ON CONFLICT (chave) DO UPDATE SET valor = excluded.valor`, chave, valor)
	return err
}
