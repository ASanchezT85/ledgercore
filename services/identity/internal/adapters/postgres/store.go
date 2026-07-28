package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ledgercore/ledgercore/services/identity/internal/domain"
)

// uniqueViolation is the PostgreSQL SQLSTATE for unique constraint errors.
const uniqueViolation = "23505"

// Store implements the app repository ports over a pgx pool whose
// search_path is pinned to the "identity" schema. All identity tables are
// system-level (see migration 0001), so queries run without tenant context.
type Store struct {
	pool *pgxpool.Pool
}

// NewStore wraps the pool.
func NewStore(pool *pgxpool.Pool) *Store { return &Store{pool: pool} }

// ---- Tenants ----------------------------------------------------------------

// CreateTenant inserts a tenant; a duplicate slug maps to domain.ErrSlugConflict.
func (s *Store) CreateTenant(ctx context.Context, t domain.Tenant) error {
	_, err := s.pool.Exec(ctx,
		`INSERT INTO tenants (id, name, slug, status, created_at, expires_at) VALUES ($1, $2, $3, $4, $5, $6)`,
		t.ID, t.Name, t.Slug, t.Status, t.CreatedAt, t.ExpiresAt,
	)
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == uniqueViolation {
		return domain.ErrSlugConflict
	}
	if err != nil {
		return fmt.Errorf("postgres: insert tenant: %w", err)
	}
	return nil
}

// GetTenant fetches one tenant by id.
func (s *Store) GetTenant(ctx context.Context, id uuid.UUID) (domain.Tenant, error) {
	var t domain.Tenant
	err := s.pool.QueryRow(ctx,
		`SELECT id, name, slug, status, created_at, expires_at FROM tenants WHERE id = $1`,
		id,
	).Scan(&t.ID, &t.Name, &t.Slug, &t.Status, &t.CreatedAt, &t.ExpiresAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Tenant{}, domain.ErrNotFound
	}
	if err != nil {
		return domain.Tenant{}, fmt.Errorf("postgres: get tenant: %w", err)
	}
	return t, nil
}

// ListTenants returns every tenant ordered by creation time.
func (s *Store) ListTenants(ctx context.Context) ([]domain.Tenant, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id, name, slug, status, created_at, expires_at FROM tenants ORDER BY created_at, id`,
	)
	if err != nil {
		return nil, fmt.Errorf("postgres: list tenants: %w", err)
	}
	defer rows.Close()

	var out []domain.Tenant
	for rows.Next() {
		var t domain.Tenant
		if err := rows.Scan(&t.ID, &t.Name, &t.Slug, &t.Status, &t.CreatedAt, &t.ExpiresAt); err != nil {
			return nil, fmt.Errorf("postgres: scan tenant: %w", err)
		}
		out = append(out, t)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("postgres: iterate tenants: %w", err)
	}
	return out, nil
}

// ---- API keys ----------------------------------------------------------------

// CreateAPIKey inserts a key (hash + prefix only).
func (s *Store) CreateAPIKey(ctx context.Context, k domain.APIKey) error {
	_, err := s.pool.Exec(ctx,
		`INSERT INTO api_keys (id, tenant_id, environment, name, key_prefix, secret_hash, created_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7)`,
		k.ID, k.TenantID, k.Environment, k.Name, k.KeyPrefix, k.SecretHash, k.CreatedAt,
	)
	if err != nil {
		return fmt.Errorf("postgres: insert api key: %w", err)
	}
	return nil
}

// FindAPIKeysByPrefix returns every key sharing the lookup prefix (collisions
// are possible in principle; the caller disambiguates by hash).
func (s *Store) FindAPIKeysByPrefix(ctx context.Context, prefix string) ([]domain.APIKey, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id, tenant_id, environment, name, key_prefix, secret_hash, created_at, revoked_at
		 FROM api_keys WHERE key_prefix = $1`,
		prefix,
	)
	if err != nil {
		return nil, fmt.Errorf("postgres: find api keys by prefix: %w", err)
	}
	defer rows.Close()

	var out []domain.APIKey
	for rows.Next() {
		var k domain.APIKey
		if err := rows.Scan(&k.ID, &k.TenantID, &k.Environment, &k.Name, &k.KeyPrefix, &k.SecretHash, &k.CreatedAt, &k.RevokedAt); err != nil {
			return nil, fmt.Errorf("postgres: scan api key: %w", err)
		}
		out = append(out, k)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("postgres: iterate api keys: %w", err)
	}
	return out, nil
}

// RevokeAPIKey marks a key revoked. Revoking twice is a no-op; a missing key
// is domain.ErrNotFound.
func (s *Store) RevokeAPIKey(ctx context.Context, id uuid.UUID) error {
	tag, err := s.pool.Exec(ctx,
		`UPDATE api_keys SET revoked_at = now() WHERE id = $1 AND revoked_at IS NULL`,
		id,
	)
	if err != nil {
		return fmt.Errorf("postgres: revoke api key: %w", err)
	}
	if tag.RowsAffected() > 0 {
		return nil
	}
	// Nothing updated: either already revoked (idempotent success) or absent.
	var exists bool
	if err := s.pool.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM api_keys WHERE id = $1)`, id).Scan(&exists); err != nil {
		return fmt.Errorf("postgres: check api key existence: %w", err)
	}
	if !exists {
		return domain.ErrNotFound
	}
	return nil
}

// ---- Signing keys ----------------------------------------------------------------

// InsertSigningKey persists a signing key pair.
func (s *Store) InsertSigningKey(ctx context.Context, k domain.SigningKey) error {
	_, err := s.pool.Exec(ctx,
		`INSERT INTO signing_keys (kid, private_key_pem, public_key_pem, algorithm, created_at, active)
		 VALUES ($1, $2, $3, $4, $5, $6)`,
		k.Kid, k.PrivateKeyPEM, k.PublicKeyPEM, k.Algorithm, k.CreatedAt, k.Active,
	)
	if err != nil {
		return fmt.Errorf("postgres: insert signing key: %w", err)
	}
	return nil
}

// UpdateSigningKeyPrivatePEM replaces the stored private-key material of an
// existing key. Used at startup to re-encrypt legacy plaintext keys once a
// master key (LEDGERCORE_MASTER_KEY) is configured.
func (s *Store) UpdateSigningKeyPrivatePEM(ctx context.Context, kid uuid.UUID, privateKeyPEM string) error {
	tag, err := s.pool.Exec(ctx,
		`UPDATE signing_keys SET private_key_pem = $2 WHERE kid = $1`,
		kid, privateKeyPEM,
	)
	if err != nil {
		return fmt.Errorf("postgres: update signing key private pem: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return domain.ErrNotFound
	}
	return nil
}

// ListActiveSigningKeys returns active keys, newest first (the first one is
// used for signing; all of them are served in the JWKS).
func (s *Store) ListActiveSigningKeys(ctx context.Context) ([]domain.SigningKey, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT kid, private_key_pem, public_key_pem, algorithm, created_at, active
		 FROM signing_keys WHERE active ORDER BY created_at DESC, kid DESC`,
	)
	if err != nil {
		return nil, fmt.Errorf("postgres: list signing keys: %w", err)
	}
	defer rows.Close()

	var out []domain.SigningKey
	for rows.Next() {
		var k domain.SigningKey
		if err := rows.Scan(&k.Kid, &k.PrivateKeyPEM, &k.PublicKeyPEM, &k.Algorithm, &k.CreatedAt, &k.Active); err != nil {
			return nil, fmt.Errorf("postgres: scan signing key: %w", err)
		}
		out = append(out, k)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("postgres: iterate signing keys: %w", err)
	}
	return out, nil
}
