// Package postgres implements the persistence port of the ledger engine on
// PostgreSQL: pgx repositories, embedded goose migrations and the
// transactional outbox writes. Everything runs inside the "ledger" schema
// with tenant RLS enforced via pgxutil.WithTenantTx.
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

//go:embed migrations/*.sql
var migrationsFS embed.FS

// Migrate creates the "ledger" schema if needed and applies the embedded
// goose migrations. The pool must have search_path=ledger (pgxutil.NewPool),
// so both the goose version table and every object land in this service's
// schema and never touch another service's.
func Migrate(ctx context.Context, pool *pgxpool.Pool) error {
	// Check existence first: CREATE SCHEMA IF NOT EXISTS still requires the
	// CREATE privilege on the database even when the schema already exists,
	// and the app role deliberately lacks it (schemas are provisioned by
	// infra/postgres/init/01-init.sql).
	var exists bool
	if err := pool.QueryRow(ctx, "SELECT EXISTS (SELECT 1 FROM pg_namespace WHERE nspname = 'ledger')").Scan(&exists); err != nil {
		return fmt.Errorf("postgres: check schema ledger: %w", err)
	}
	if !exists {
		if _, err := pool.Exec(ctx, "CREATE SCHEMA ledger"); err != nil {
			return fmt.Errorf("postgres: create schema ledger: %w", err)
		}
	}

	sub, err := fs.Sub(migrationsFS, "migrations")
	if err != nil {
		return fmt.Errorf("postgres: sub fs: %w", err)
	}

	// database/sql handle backed by the same pgx pool (search_path included).
	// Closing it returns connections to the pool without closing the pool.
	db := stdlib.OpenDBFromPool(pool)
	defer db.Close()

	provider, err := goose.NewProvider(goose.DialectPostgres, db, sub)
	if err != nil {
		return fmt.Errorf("postgres: goose provider: %w", err)
	}
	results, err := provider.Up(ctx)
	if err != nil {
		return fmt.Errorf("postgres: apply migrations: %w", err)
	}
	for _, r := range results {
		if r.Error != nil {
			return fmt.Errorf("postgres: migration %s failed: %w", r.Source.Path, r.Error)
		}
	}
	return nil
}
