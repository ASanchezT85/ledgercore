package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/ASanchezT85/ledgercore/libs/go/events"
	"github.com/ASanchezT85/ledgercore/libs/go/pgxutil"
	"github.com/ASanchezT85/ledgercore/services/identity/internal/domain"
)

// ---- Sandbox signup (atomic provisioning) ------------------------------------

// ProvisionSandboxTenant creates the tenant, its sandbox API key and the
// signup record in ONE transaction, enforcing the global per-day cap with a
// serialized count (the signups table is locked for the check+insert window
// so concurrent signups cannot race past the limit). Nothing is persisted if
// any step fails.
func (s *Store) ProvisionSandboxTenant(ctx context.Context, email string, t domain.Tenant, k domain.APIKey, dailyLimit int) error {
	return pgxutil.WithSystemTx(ctx, s.pool, func(tx pgx.Tx) error {
		// Serialize the daily-cap check against concurrent signups.
		if _, err := tx.Exec(ctx, `LOCK TABLE sandbox_signups IN SHARE ROW EXCLUSIVE MODE`); err != nil {
			return fmt.Errorf("postgres: lock sandbox_signups: %w", err)
		}
		var todays int
		if err := tx.QueryRow(ctx,
			`SELECT count(*) FROM sandbox_signups WHERE created_at >= date_trunc('day', now())`,
		).Scan(&todays); err != nil {
			return fmt.Errorf("postgres: count today's signups: %w", err)
		}
		if todays >= dailyLimit {
			return domain.ErrSignupLimitReached
		}

		if _, err := tx.Exec(ctx,
			`INSERT INTO tenants (id, name, slug, status, created_at, expires_at) VALUES ($1, $2, $3, $4, $5, $6)`,
			t.ID, t.Name, t.Slug, t.Status, t.CreatedAt, t.ExpiresAt,
		); err != nil {
			if isUnique(err, "tenants_slug_key") {
				return domain.ErrSlugConflict
			}
			return fmt.Errorf("postgres: insert sandbox tenant: %w", err)
		}
		if _, err := tx.Exec(ctx,
			`INSERT INTO api_keys (id, tenant_id, environment, name, key_prefix, secret_hash, created_at)
			 VALUES ($1, $2, $3, $4, $5, $6, $7)`,
			k.ID, k.TenantID, k.Environment, k.Name, k.KeyPrefix, k.SecretHash, k.CreatedAt,
		); err != nil {
			return fmt.Errorf("postgres: insert sandbox api key: %w", err)
		}
		if _, err := tx.Exec(ctx,
			`INSERT INTO sandbox_signups (email, tenant_id, created_at) VALUES ($1, $2, $3)`,
			email, t.ID, t.CreatedAt,
		); err != nil {
			if isUnique(err, "sandbox_signups_pkey") {
				return domain.ErrEmailTaken
			}
			return fmt.Errorf("postgres: insert sandbox signup: %w", err)
		}
		return nil
	})
}

func isUnique(err error, constraint string) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == uniqueViolation && pgErr.ConstraintName == constraint
}

// ---- Sweeper ------------------------------------------------------------------

// ExpiredSandboxTenants returns sandbox tenants past their TTL that have not
// been announced yet (status still active or suspended).
func (s *Store) ExpiredSandboxTenants(ctx context.Context, now time.Time) ([]domain.Tenant, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT t.id, t.name, t.slug, t.status, t.created_at, t.expires_at
		FROM tenants t
		WHERE t.expires_at IS NOT NULL
		  AND t.expires_at < $1
		  AND t.status IN ('active', 'suspended')
		ORDER BY t.expires_at`, now)
	if err != nil {
		return nil, fmt.Errorf("postgres: select expired sandbox tenants: %w", err)
	}
	defer rows.Close()
	var out []domain.Tenant
	for rows.Next() {
		var t domain.Tenant
		if err := rows.Scan(&t.ID, &t.Name, &t.Slug, &t.Status, &t.CreatedAt, &t.ExpiresAt); err != nil {
			return nil, fmt.Errorf("postgres: scan expired tenant: %w", err)
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

// MarkTenantPurging flips the tenant to purging AND enqueues the expiry
// envelope in the outbox, in one transaction. Idempotent: if the tenant is
// no longer active/suspended nothing happens (no duplicate event).
func (s *Store) MarkTenantPurging(ctx context.Context, env events.Envelope) error {
	payload, err := json.Marshal(env)
	if err != nil {
		return fmt.Errorf("postgres: marshal expiry envelope: %w", err)
	}
	return pgxutil.WithSystemTx(ctx, s.pool, func(tx pgx.Tx) error {
		tag, err := tx.Exec(ctx,
			`UPDATE tenants SET status = $1 WHERE id = $2 AND status IN ('active', 'suspended')`,
			domain.TenantStatusPurging, env.TenantID,
		)
		if err != nil {
			return fmt.Errorf("postgres: mark tenant purging: %w", err)
		}
		if tag.RowsAffected() == 0 {
			return nil // already announced by a concurrent sweep
		}
		if _, err := tx.Exec(ctx,
			`INSERT INTO outbox (tenant_id, topic, envelope) VALUES ($1, $2, $3)`,
			env.TenantID, env.Type, payload,
		); err != nil {
			return fmt.Errorf("postgres: enqueue expiry event: %w", err)
		}
		return nil
	})
}

// PurgeableTenants returns purging tenants whose expiry is older than the
// grace period (expires_at < now - grace).
func (s *Store) PurgeableTenants(ctx context.Context, cutoff time.Time) ([]uuid.UUID, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id FROM tenants WHERE status = $1 AND expires_at < $2`,
		domain.TenantStatusPurging, cutoff,
	)
	if err != nil {
		return nil, fmt.Errorf("postgres: select purgeable tenants: %w", err)
	}
	defer rows.Close()
	var ids []uuid.UUID
	for rows.Next() {
		var id uuid.UUID
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("postgres: scan purgeable tenant: %w", err)
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

// FinalizeTenantPurge deletes the tenant's API keys and marks it purged, in
// one transaction. Idempotent by the status guard.
func (s *Store) FinalizeTenantPurge(ctx context.Context, tenantID uuid.UUID) (int64, error) {
	var deletedKeys int64
	err := pgxutil.WithSystemTx(ctx, s.pool, func(tx pgx.Tx) error {
		tag, err := tx.Exec(ctx,
			`UPDATE tenants SET status = $1 WHERE id = $2 AND status = $3`,
			domain.TenantStatusPurged, tenantID, domain.TenantStatusPurging,
		)
		if err != nil {
			return fmt.Errorf("postgres: mark tenant purged: %w", err)
		}
		if tag.RowsAffected() == 0 {
			return nil
		}
		del, err := tx.Exec(ctx, `DELETE FROM api_keys WHERE tenant_id = $1`, tenantID)
		if err != nil {
			return fmt.Errorf("postgres: delete tenant api keys: %w", err)
		}
		deletedKeys = del.RowsAffected()
		return nil
	})
	return deletedKeys, err
}
