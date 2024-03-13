package infrastructure

import (
	"database/sql"
	"embed"
	"fmt"

	"github.com/pressly/goose/v3"
)

// MigrateSQLDatabase migrates the database, using the dbmate library
// NOTE: assumes that the migrations are in a folder called "migrations"
func MigrateSQLDatabase(domain, dialect string, db *sql.DB, fs embed.FS) error {
	goose.SetBaseFS(fs)
	if err := goose.SetDialect(dialect); err != nil {
		return fmt.Errorf("failed to set dialect: %w", err)
	}

	goose.SetTableName(domain + "_goose_db_version")

	if err := goose.Up(db, "migrations"); err != nil {
		return fmt.Errorf("failed to migrate database: %w", err)
	}

	return nil
}
