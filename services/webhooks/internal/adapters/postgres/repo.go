// Package postgres is the storage adapter of the webhooks service. All
// tenant-scoped statements run through pgxutil.WithTenantTx so the RLS
// policies apply; the dispatcher claim spans tenants and uses a system
// transaction (the worker role must own the tables or hold BYPASSRLS).
package postgres

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/ledgercore/ledgercore/libs/go/httpx"
	"github.com/ledgercore/ledgercore/libs/go/pgxutil"
	"github.com/ledgercore/ledgercore/services/webhooks/internal/app"
	"github.com/ledgercore/ledgercore/services/webhooks/internal/domain"
	"github.com/ledgercore/ledgercore/services/webhooks/internal/keycrypt"
)

// Repo implements app.SubscriptionStore, app.DeliveryStore and the
// dispatcher store against the webhooks schema.
type Repo struct {
	pool *pgxpool.Pool
	// cipher encrypts the signing secret (and previous_secret) at rest with
	// envelope AES-256-GCM. Nil in dev when LEDGERCORE_MASTER_KEY is unset:
	// secrets are then stored/read as plaintext (WARN at startup).
	cipher *keycrypt.Cipher
}

// NewRepo builds the repository on top of a pool pinned to the webhooks schema.
// cipher may be nil (dev only), in which case secrets are stored in plaintext.
func NewRepo(pool *pgxpool.Pool, cipher *keycrypt.Cipher) *Repo {
	return &Repo{pool: pool, cipher: cipher}
}

const subscriptionColumns = "id, tenant_id, url, secret, event_types, active, created_at, previous_secret, previous_secret_expires_at"

// encryptSecret seals a plaintext signing secret bound to the subscription id.
// A nil cipher (dev) stores the value verbatim.
func (r *Repo) encryptSecret(plaintext string, subID uuid.UUID) (string, error) {
	if r.cipher == nil || plaintext == "" {
		return plaintext, nil
	}
	return r.cipher.Encrypt(plaintext, subID.String())
}

// decryptSecret opens a stored secret bound to the subscription id. Legacy
// plaintext values (no enc:v1: prefix) pass through unchanged.
func (r *Repo) decryptSecret(stored string, subID uuid.UUID) (string, error) {
	if stored == "" {
		return "", nil
	}
	if !keycrypt.IsEncrypted(stored) {
		return stored, nil
	}
	return r.cipher.Decrypt(stored, subID.String())
}

func (r *Repo) scanSubscription(row pgx.Row) (domain.Subscription, error) {
	var s domain.Subscription
	err := row.Scan(&s.ID, &s.TenantID, &s.URL, &s.Secret, &s.EventTypes, &s.Active, &s.CreatedAt,
		&s.PreviousSecret, &s.PreviousSecretExpiresAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Subscription{}, domain.ErrNotFound
	}
	if err != nil {
		return domain.Subscription{}, fmt.Errorf("postgres: scan subscription: %w", err)
	}
	if s.Secret, err = r.decryptSecret(s.Secret, s.ID); err != nil {
		return domain.Subscription{}, fmt.Errorf("postgres: decrypt secret: %w", err)
	}
	if s.PreviousSecret != nil {
		plain, err := r.decryptSecret(*s.PreviousSecret, s.ID)
		if err != nil {
			return domain.Subscription{}, fmt.Errorf("postgres: decrypt previous secret: %w", err)
		}
		s.PreviousSecret = &plain
	}
	return s, nil
}

// ---- app.SubscriptionStore -----------------------------------------------------

// Create inserts a new subscription.
func (r *Repo) Create(ctx context.Context, sub domain.Subscription) error {
	secret, err := r.encryptSecret(sub.Secret, sub.ID)
	if err != nil {
		return fmt.Errorf("postgres: encrypt secret: %w", err)
	}
	return pgxutil.WithTenantTx(ctx, r.pool, sub.TenantID, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `
			INSERT INTO subscriptions (id, tenant_id, url, secret, event_types, active, created_at)
			VALUES ($1, $2, $3, $4, $5, $6, $7)`,
			sub.ID, sub.TenantID, sub.URL, secret, sub.EventTypes, sub.Active, sub.CreatedAt)
		return err
	})
}

// List returns a keyset page of the tenant's subscriptions, newest first.
func (r *Repo) List(ctx context.Context, tenantID uuid.UUID, limit int, cursor httpx.Cursor) ([]domain.Subscription, error) {
	q := "SELECT " + subscriptionColumns + ` FROM subscriptions
		WHERE tenant_id = $1
		  AND ($2::timestamptz IS NULL OR (created_at, id) < ($2, $3::uuid))
		ORDER BY created_at DESC, id DESC
		LIMIT $4`
	var cAt, cID any
	if !cursor.IsZero() {
		cAt, cID = cursor.CreatedAt, cursor.ID
	}
	return r.querySubscriptions(ctx, tenantID, q, tenantID, cAt, cID, limit)
}

// ListActive returns only active subscriptions of the tenant.
func (r *Repo) ListActive(ctx context.Context, tenantID uuid.UUID) ([]domain.Subscription, error) {
	q := "SELECT " + subscriptionColumns + " FROM subscriptions WHERE tenant_id = $1 AND active ORDER BY created_at, id"
	return r.querySubscriptions(ctx, tenantID, q, tenantID)
}

func (r *Repo) querySubscriptions(ctx context.Context, tenantID uuid.UUID, q string, args ...any) ([]domain.Subscription, error) {
	var out []domain.Subscription
	err := pgxutil.WithTenantTx(ctx, r.pool, tenantID, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, q, args...)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			s, err := r.scanSubscription(rows)
			if err != nil {
				return err
			}
			out = append(out, s)
		}
		return rows.Err()
	})
	if err != nil {
		return nil, fmt.Errorf("postgres: list subscriptions: %w", err)
	}
	return out, nil
}

// Get returns one subscription of the tenant.
func (r *Repo) Get(ctx context.Context, tenantID, id uuid.UUID) (domain.Subscription, error) {
	var out domain.Subscription
	err := pgxutil.WithTenantTx(ctx, r.pool, tenantID, func(tx pgx.Tx) error {
		var err error
		out, err = r.scanSubscription(tx.QueryRow(ctx,
			"SELECT "+subscriptionColumns+" FROM subscriptions WHERE id = $1 AND tenant_id = $2", id, tenantID))
		return err
	})
	return out, err
}

// Update applies a partial patch; nil fields are left untouched.
func (r *Repo) Update(ctx context.Context, tenantID, id uuid.UUID, url *string, eventTypes []string, active *bool) (domain.Subscription, error) {
	var out domain.Subscription
	err := pgxutil.WithTenantTx(ctx, r.pool, tenantID, func(tx pgx.Tx) error {
		var (
			sets []string
			args []any
		)
		if url != nil {
			args = append(args, *url)
			sets = append(sets, fmt.Sprintf("url = $%d", len(args)))
		}
		if eventTypes != nil {
			args = append(args, eventTypes)
			sets = append(sets, fmt.Sprintf("event_types = $%d", len(args)))
		}
		if active != nil {
			args = append(args, *active)
			sets = append(sets, fmt.Sprintf("active = $%d", len(args)))
		}
		if len(sets) == 0 {
			var err error
			out, err = r.scanSubscription(tx.QueryRow(ctx,
				"SELECT "+subscriptionColumns+" FROM subscriptions WHERE id = $1 AND tenant_id = $2", id, tenantID))
			return err
		}
		args = append(args, id, tenantID)
		q := fmt.Sprintf(
			"UPDATE subscriptions SET %s WHERE id = $%d AND tenant_id = $%d RETURNING %s",
			strings.Join(sets, ", "), len(args)-1, len(args), subscriptionColumns)
		var err error
		out, err = r.scanSubscription(tx.QueryRow(ctx, q, args...))
		return err
	})
	return out, err
}

// RotateSecret installs a new signing secret, keeping the old one as
// previous_secret until prevExpiresAt so in-flight receivers keep verifying.
func (r *Repo) RotateSecret(ctx context.Context, tenantID, id uuid.UUID, secret string, prevExpiresAt time.Time) error {
	enc, err := r.encryptSecret(secret, id)
	if err != nil {
		return fmt.Errorf("postgres: encrypt secret: %w", err)
	}
	// previous_secret = secret copies the already-encrypted current value
	// (same subscription-id AAD), so it stays decryptable during the grace
	// window; secret = $2 installs the freshly encrypted new value.
	return pgxutil.WithTenantTx(ctx, r.pool, tenantID, func(tx pgx.Tx) error {
		tag, err := tx.Exec(ctx, `
			UPDATE subscriptions
			SET previous_secret = secret,
			    previous_secret_expires_at = $1,
			    secret = $2
			WHERE id = $3 AND tenant_id = $4`, prevExpiresAt, enc, id, tenantID)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return domain.ErrNotFound
		}
		return nil
	})
}

// PurgeExpiredPreviousSecrets clears previous secrets whose grace window has
// passed, across all tenants (system transaction). Returns the rows cleaned.
func (r *Repo) PurgeExpiredPreviousSecrets(ctx context.Context) (int64, error) {
	var n int64
	err := pgxutil.WithSystemTx(ctx, r.pool, func(tx pgx.Tx) error {
		tag, err := tx.Exec(ctx, `
			UPDATE subscriptions
			SET previous_secret = NULL, previous_secret_expires_at = NULL
			WHERE previous_secret IS NOT NULL AND previous_secret_expires_at <= now()`)
		if err != nil {
			return err
		}
		n = tag.RowsAffected()
		return nil
	})
	if err != nil {
		return 0, fmt.Errorf("postgres: purge expired previous secrets: %w", err)
	}
	return n, nil
}

// ReencryptPlaintextSecrets migrates any subscription whose secret (or
// previous_secret) is still stored in plaintext to the encrypted-at-rest
// format (LC-008). It is a no-op when no cipher is configured (dev). Runs in a
// system transaction because it spans tenants; each secret is sealed with the
// subscription-id AAD. Returns the number of rows re-encrypted.
func (r *Repo) ReencryptPlaintextSecrets(ctx context.Context) (int64, error) {
	if r.cipher == nil {
		return 0, nil
	}
	var n int64
	err := pgxutil.WithSystemTx(ctx, r.pool, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `
			SELECT id, secret, previous_secret FROM subscriptions
			WHERE secret NOT LIKE 'enc:v1:%'
			   OR (previous_secret IS NOT NULL AND previous_secret NOT LIKE 'enc:v1:%')`)
		if err != nil {
			return err
		}
		type pending struct {
			id     uuid.UUID
			secret string
			prev   *string
		}
		var todo []pending
		for rows.Next() {
			var p pending
			if err := rows.Scan(&p.id, &p.secret, &p.prev); err != nil {
				rows.Close()
				return err
			}
			todo = append(todo, p)
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return err
		}
		for _, p := range todo {
			encSecret := p.secret
			if !keycrypt.IsEncrypted(encSecret) {
				if encSecret, err = r.cipher.Encrypt(encSecret, p.id.String()); err != nil {
					return err
				}
			}
			var encPrev *string
			if p.prev != nil {
				v := *p.prev
				if !keycrypt.IsEncrypted(v) {
					if v, err = r.cipher.Encrypt(v, p.id.String()); err != nil {
						return err
					}
				}
				encPrev = &v
			}
			tag, err := tx.Exec(ctx,
				"UPDATE subscriptions SET secret = $1, previous_secret = $2 WHERE id = $3",
				encSecret, encPrev, p.id)
			if err != nil {
				return err
			}
			n += tag.RowsAffected()
		}
		return nil
	})
	if err != nil {
		return 0, fmt.Errorf("postgres: reencrypt plaintext secrets: %w", err)
	}
	return n, nil
}

// ---- app.DeliveryStore -------------------------------------------------------------

const deliveryColumns = "id, tenant_id, subscription_id, event_id, event_type, payload, status, attempts, next_attempt_at, last_status_code, last_error, created_at, delivered_at"

func scanDelivery(row pgx.Row) (domain.Delivery, error) {
	var d domain.Delivery
	err := row.Scan(&d.ID, &d.TenantID, &d.SubscriptionID, &d.EventID, &d.EventType, &d.Payload,
		&d.Status, &d.Attempts, &d.NextAttemptAt, &d.LastStatusCode, &d.LastError, &d.CreatedAt, &d.DeliveredAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Delivery{}, domain.ErrNotFound
	}
	if err != nil {
		return domain.Delivery{}, fmt.Errorf("postgres: scan delivery: %w", err)
	}
	return d, nil
}

// InsertPending inserts deliveries idempotently per (subscription, event).
func (r *Repo) InsertPending(ctx context.Context, tenantID uuid.UUID, ds []domain.Delivery) error {
	return pgxutil.WithTenantTx(ctx, r.pool, tenantID, func(tx pgx.Tx) error {
		batch := &pgx.Batch{}
		for _, d := range ds {
			batch.Queue(`
				INSERT INTO deliveries (id, tenant_id, subscription_id, event_id, event_type, payload, status, attempts, next_attempt_at, created_at)
				VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
				ON CONFLICT (subscription_id, event_id) DO NOTHING`,
				d.ID, d.TenantID, d.SubscriptionID, d.EventID, d.EventType, d.Payload,
				d.Status, d.Attempts, d.NextAttemptAt, d.CreatedAt)
		}
		return tx.SendBatch(ctx, batch).Close()
	})
}

// ListDeliveries returns a keyset-paginated page of deliveries plus the
// next cursor.
func (r *Repo) ListDeliveries(ctx context.Context, tenantID uuid.UUID, f app.DeliveryFilter) ([]domain.Delivery, string, error) {
	where := []string{"tenant_id = $1"}
	args := []any{tenantID}
	if f.Status != "" {
		args = append(args, f.Status)
		where = append(where, fmt.Sprintf("status = $%d", len(args)))
	}
	if f.SubscriptionID != nil {
		args = append(args, *f.SubscriptionID)
		where = append(where, fmt.Sprintf("subscription_id = $%d", len(args)))
	}
	if !f.Cursor.IsZero() {
		args = append(args, f.Cursor.CreatedAt, f.Cursor.ID)
		where = append(where, fmt.Sprintf("(created_at, id) < ($%d, $%d)", len(args)-1, len(args)))
	}
	args = append(args, f.Limit+1)
	q := fmt.Sprintf(
		"SELECT %s FROM deliveries WHERE %s ORDER BY created_at DESC, id DESC LIMIT $%d",
		deliveryColumns, strings.Join(where, " AND "), len(args))

	var out []domain.Delivery
	err := pgxutil.WithTenantTx(ctx, r.pool, tenantID, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, q, args...)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			d, err := scanDelivery(rows)
			if err != nil {
				return err
			}
			out = append(out, d)
		}
		return rows.Err()
	})
	if err != nil {
		return nil, "", fmt.Errorf("postgres: list deliveries: %w", err)
	}

	next := ""
	if len(out) > f.Limit {
		out = out[:f.Limit]
		last := out[len(out)-1]
		next = httpx.Cursor{CreatedAt: last.CreatedAt, ID: last.ID}.Encode()
	}
	return out, next, nil
}

// Requeue resets a failed/dead delivery to pending with a fresh attempt budget.
func (r *Repo) Requeue(ctx context.Context, tenantID, id uuid.UUID) (domain.Delivery, error) {
	var out domain.Delivery
	err := pgxutil.WithTenantTx(ctx, r.pool, tenantID, func(tx pgx.Tx) error {
		var err error
		out, err = scanDelivery(tx.QueryRow(ctx, `
			UPDATE deliveries
			SET status = 'pending', attempts = 0, next_attempt_at = now()
			WHERE id = $1 AND tenant_id = $2 AND status IN ('failed', 'dead')
			RETURNING `+deliveryColumns, id, tenantID))
		if errors.Is(err, domain.ErrNotFound) {
			// Distinguish "does not exist" from "not retryable".
			var status string
			probe := tx.QueryRow(ctx,
				"SELECT status FROM deliveries WHERE id = $1 AND tenant_id = $2", id, tenantID)
			if probeErr := probe.Scan(&status); probeErr == nil {
				return fmt.Errorf("%w: delivery is %s, only failed or dead deliveries can be retried",
					domain.ErrConflict, status)
			}
			return domain.ErrNotFound
		}
		return err
	})
	return out, err
}

// ---- dispatcher.Store -----------------------------------------------------------------

// ClaimDue leases up to `limit` due deliveries for the dispatcher. Rows are
// locked with FOR UPDATE SKIP LOCKED and their next_attempt_at is pushed
// `lease` into the future, so a crashed worker's claims resurface on their
// own. Runs as a system transaction because the scan spans all tenants.
func (r *Repo) ClaimDue(ctx context.Context, limit int, lease time.Duration) ([]domain.ClaimedDelivery, error) {
	var out []domain.ClaimedDelivery
	err := pgxutil.WithSystemTx(ctx, r.pool, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `
			SELECT d.id, d.tenant_id, d.subscription_id, d.event_id, d.event_type, d.payload, d.attempts, s.url, s.secret,
				       s.previous_secret, s.previous_secret_expires_at
			FROM deliveries d
			JOIN subscriptions s ON s.id = d.subscription_id
			WHERE d.status IN ('pending', 'failed')
			  AND d.next_attempt_at <= now()
			  AND s.active
			ORDER BY d.next_attempt_at
			LIMIT $1
			FOR UPDATE OF d SKIP LOCKED`, limit)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var c domain.ClaimedDelivery
			if err := rows.Scan(&c.ID, &c.TenantID, &c.SubscriptionID, &c.EventID, &c.EventType,
				&c.Payload, &c.Attempts, &c.URL, &c.Secret,
				&c.PreviousSecret, &c.PreviousSecretExpiresAt); err != nil {
				return err
			}
			// Decrypt in memory just to sign the outgoing delivery; the
			// plaintext never touches the DB or the logs.
			if c.Secret, err = r.decryptSecret(c.Secret, c.SubscriptionID); err != nil {
				return fmt.Errorf("decrypt secret for subscription %s: %w", c.SubscriptionID, err)
			}
			if c.PreviousSecret != nil {
				plain, derr := r.decryptSecret(*c.PreviousSecret, c.SubscriptionID)
				if derr != nil {
					return fmt.Errorf("decrypt previous secret for subscription %s: %w", c.SubscriptionID, derr)
				}
				c.PreviousSecret = &plain
			}
			out = append(out, c)
		}
		if err := rows.Err(); err != nil {
			return err
		}
		if len(out) == 0 {
			return nil
		}
		ids := make([]uuid.UUID, len(out))
		for i, c := range out {
			ids[i] = c.ID
		}
		_, err = tx.Exec(ctx,
			"UPDATE deliveries SET next_attempt_at = now() + make_interval(secs => $1) WHERE id = ANY($2)",
			lease.Seconds(), ids)
		return err
	})
	if err != nil {
		return nil, fmt.Errorf("postgres: claim due deliveries: %w", err)
	}
	return out, nil
}

// MarkDelivered finalizes a successful delivery.
func (r *Repo) MarkDelivered(ctx context.Context, tenantID, id uuid.UUID, statusCode int) error {
	return pgxutil.WithTenantTx(ctx, r.pool, tenantID, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `
			UPDATE deliveries
			SET status = 'delivered', delivered_at = now(), last_status_code = $1, last_error = NULL
			WHERE id = $2 AND tenant_id = $3`,
			statusCode, id, tenantID)
		return err
	})
}

// MarkFailed records a failed attempt: either reschedules it (status failed)
// or tombstones it (status dead).
func (r *Repo) MarkFailed(ctx context.Context, tenantID, id uuid.UUID, attempts int, statusCode *int, lastError string, nextAttemptAt time.Time, dead bool) error {
	status := domain.StatusFailed
	if dead {
		status = domain.StatusDead
	}
	return pgxutil.WithTenantTx(ctx, r.pool, tenantID, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `
			UPDATE deliveries
			SET status = $1, attempts = $2, last_status_code = $3, last_error = $4, next_attempt_at = $5
			WHERE id = $6 AND tenant_id = $7`,
			status, attempts, statusCode, lastError, nextAttemptAt, id, tenantID)
		return err
	})
}
