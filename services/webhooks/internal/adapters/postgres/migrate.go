package postgres

import (
	"context"
	"embed"
	"fmt"
	"io/fs"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
)

// SchemaName is the Postgres schema owned by the webhooks service. The pool
// pins search_path to it, so all statements here are schema-local.
const SchemaName = "webhooks"

//go:embed migrations/*.sql
var migrationsFS embed.FS

// Migrate runs the embedded goose migrations against the service schema.
// It first ensures the schema exists, because goose creates its version
// table through the pool whose search_path already points at it.
func Migrate(ctx context.Context, pool *pgxpool.Pool) error {
	// Check existence first: CREATE SCHEMA IF NOT EXISTS still requires the
	// CREATE privilege on the database even when the schema already exists,
	// and the app role deliberately lacks it (schemas are provisioned by
	// infra/postgres/init/01-init.sql).
	var exists bool
	if err := pool.QueryRow(ctx, "SELECT EXISTS (SELECT 1 FROM pg_namespace WHERE nspname = $1)", SchemaName).Scan(&exists); err != nil {
		return fmt.Errorf("postgres: check schema %s: %w", SchemaName, err)
	}
	if !exists {
		if _, err := pool.Exec(ctx, "CREATE SCHEMA "+SchemaName); err != nil {
			return fmt.Errorf("postgres: ensure schema %s: %w", SchemaName, err)
		}
	}

	sub, err := fs.Sub(migrationsFS, "migrations")
	if err != nil {
		return fmt.Errorf("postgres: open embedded migrations: %w", err)
	}

	// stdlib.OpenDBFromPool wraps the pgx pool as database/sql for goose.
	// Closing the wrapper releases its connections back to the pool without
	// closing the pool itself.
	db := stdlib.OpenDBFromPool(pool)
	defer db.Close()

	provider, err := goose.NewProvider(goose.DialectPostgres, db, sub)
	if err != nil {
		return fmt.Errorf("postgres: goose provider: %w", err)
	}
	if _, err := provider.Up(ctx); err != nil {
		return fmt.Errorf("postgres: run migrations: %w", err)
	}
	return nil
}
