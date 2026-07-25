// Package pgxutil provides the shared PostgreSQL plumbing: pool construction
// pinned to a service schema, and transaction helpers that enforce the
// multi-tenant RLS contract (SET LOCAL app.tenant_id).
package pgxutil

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// NewPool creates a pgx pool whose connections have search_path fixed to the
// given schema. Every service owns exactly one schema and must never touch
// another service's schema.
func NewPool(ctx context.Context, url, searchPath string) (*pgxpool.Pool, error) {
	cfg, err := pgxpool.ParseConfig(url)
	if err != nil {
		return nil, fmt.Errorf("pgxutil: parse database url: %w", err)
	}
	if searchPath != "" {
		if cfg.ConnConfig.RuntimeParams == nil {
			cfg.ConnConfig.RuntimeParams = map[string]string{}
		}
		cfg.ConnConfig.RuntimeParams["search_path"] = searchPath
	}
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("pgxutil: create pool: %w", err)
	}
	return pool, nil
}

// WithTenantTx runs fn inside a transaction scoped to the given tenant.
// It executes SET LOCAL app.tenant_id via set_config so every RLS policy
// (USING tenant_id = current_setting('app.tenant_id')::uuid) applies for the
// duration of the transaction. set_config with is_local=true is parameterized,
// so no literal quoting is involved. Commit on success, rollback otherwise.
func WithTenantTx(ctx context.Context, pool *pgxpool.Pool, tenantID uuid.UUID, fn func(pgx.Tx) error) error {
	if tenantID == uuid.Nil {
		return fmt.Errorf("pgxutil: tenant id must not be nil")
	}
	return withTx(ctx, pool, func(tx pgx.Tx) error {
		// set_config(..., true) == SET LOCAL: the setting dies with the tx.
		if _, err := tx.Exec(ctx, "SELECT set_config('app.tenant_id', $1, true)", tenantID.String()); err != nil {
			return fmt.Errorf("pgxutil: set tenant context: %w", err)
		}
		return fn(tx)
	})
}

// WithSystemTx runs fn inside a transaction without tenant context. Use it
// only for system-level tables that are exempt from RLS (e.g. the tenant
// registry in identity) and for outbox drains that span tenants.
func WithSystemTx(ctx context.Context, pool *pgxpool.Pool, fn func(pgx.Tx) error) error {
	return withTx(ctx, pool, fn)
}

func withTx(ctx context.Context, pool *pgxpool.Pool, fn func(pgx.Tx) error) error {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("pgxutil: begin: %w", err)
	}
	defer func() {
		// No-op if the tx was already committed.
		_ = tx.Rollback(ctx)
	}()
	if err := fn(tx); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("pgxutil: commit: %w", err)
	}
	return nil
}
