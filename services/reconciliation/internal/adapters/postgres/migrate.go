// Package postgres implements the storage port over PostgreSQL (schema
// "recon") with pgx, plus the goose migrator and the outbox drain.
package postgres

import (
	"context"
	"embed"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
)

//go:embed migrations/*.sql
var migrationsFS embed.FS

// Migrate creates the recon schema if needed and applies the embedded goose
// migrations. Called at startup when LEDGERCORE_AUTO_MIGRATE=true.
func Migrate(ctx context.Context, databaseURL string) error {
	cfg, err := pgx.ParseConfig(databaseURL)
	if err != nil {
		return fmt.Errorf("postgres: parse database url: %w", err)
	}
	if cfg.RuntimeParams == nil {
		cfg.RuntimeParams = map[string]string{}
	}
	cfg.RuntimeParams["search_path"] = "recon"

	db := stdlib.OpenDB(*cfg)
	defer db.Close()

	// The schema must exist before goose creates its version table in it.
	// Check existence first: CREATE SCHEMA IF NOT EXISTS still requires the
	// CREATE privilege on the database even when the schema already exists,
	// and the app role deliberately lacks it.
	var exists bool
	if err := db.QueryRowContext(ctx, "SELECT EXISTS (SELECT 1 FROM pg_namespace WHERE nspname = 'recon')").Scan(&exists); err != nil {
		return fmt.Errorf("postgres: check schema recon: %w", err)
	}
	if !exists {
		if _, err := db.ExecContext(ctx, "CREATE SCHEMA recon"); err != nil {
			return fmt.Errorf("postgres: create schema recon: %w", err)
		}
	}

	goose.SetBaseFS(migrationsFS)
	goose.SetTableName("recon.goose_db_version")
	if err := goose.SetDialect("postgres"); err != nil {
		return fmt.Errorf("postgres: set goose dialect: %w", err)
	}
	if err := goose.UpContext(ctx, db, "migrations"); err != nil {
		return fmt.Errorf("postgres: run migrations: %w", err)
	}
	return nil
}
