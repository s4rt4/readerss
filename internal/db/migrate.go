package db

import (
	"database/sql"
	"embed"
	"io/fs"

	"github.com/pressly/goose/v3"
)

func Migrate(conn *sql.DB, migrations embed.FS) error {
	sub, err := fs.Sub(migrations, "migrations")
	if err != nil {
		return err
	}

	goose.SetBaseFS(sub)
	if err := goose.SetDialect("sqlite3"); err != nil {
		return err
	}

	return goose.Up(conn, ".")
}
