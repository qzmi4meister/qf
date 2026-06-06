package store

import (
	"context"
	"database/sql"

	"github.com/pressly/goose/v3"
	"github.com/qf/qf/cp/migrations"
)

// Migrate runs all pending goose migrations against db.
func Migrate(ctx context.Context, db *sql.DB) error {
	goose.SetBaseFS(migrations.FS)
	if err := goose.SetDialect("postgres"); err != nil {
		return err
	}
	return goose.UpContext(ctx, db, ".")
}
