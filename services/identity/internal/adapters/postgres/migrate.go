// Package postgres implements the identity repositories over pgx and runs
// the goose migrations embedded in this binary.
package postgres

import (
	"context"
	"embed"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
)

//go:embed migrations/*.sql
var migrationsFS embed.FS

// Migrate applies the embedded goose migrations. It is called at startup when
// LEDGERCORE_AUTO_MIGRATE=true. The schema is created up front so goose can
// bootstrap its version table inside it (the pool's search_path is pinned to
// "identity" by pgxutil.NewPool).
func Migrate(ctx context.Context, pool *pgxpool.Pool) error {
	// Check existence first: CREATE SCHEMA IF NOT EXISTS still requires the
	// CREATE privilege on the database even when the schema already exists,
	// and the app role deliberately lacks it (schemas are provisioned by
	// infra/postgres/init/01-init.sql).
	var exists bool
	if err := pool.QueryRow(ctx, "SELECT EXISTS (SELECT 1 FROM pg_namespace WHERE nspname = 'identity')").Scan(&exists); err != nil {
		return fmt.Errorf("postgres: check schema: %w", err)
	}
	if !exists {
		if _, err := pool.Exec(ctx, "CREATE SCHEMA identity"); err != nil {
			return fmt.Errorf("postgres: create schema: %w", err)
		}
	}

	db := stdlib.OpenDBFromPool(pool)
	defer db.Close() // closes the database/sql wrapper, not the pgx pool

	goose.SetBaseFS(migrationsFS)
	goose.SetLogger(goose.NopLogger()) // startup logging is handled via slog
	goose.SetTableName("identity.goose_db_version")
	if err := goose.SetDialect("postgres"); err != nil {
		return fmt.Errorf("postgres: set goose dialect: %w", err)
	}
	if err := goose.UpContext(ctx, db, "migrations"); err != nil {
		return fmt.Errorf("postgres: run migrations: %w", err)
	}
	return nil
}
