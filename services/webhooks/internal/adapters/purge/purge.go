// Package purge deletes every webhooks-schema row of a sandbox tenant when
// identity announces its expiry (identity.tenant.expired). The tables are
// plain mutable state (no append-only hatch). Deletes run in ONE
// tenant-scoped transaction, child-first, and are idempotent.
package purge

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/nats-io/nats.go"

	"github.com/ledgercore/ledgercore/libs/go/events"
	"github.com/ledgercore/ledgercore/libs/go/pgxutil"
)

// Durable identifies this service's purge consumer on the LEDGERCORE stream.
const Durable = "webhooks-purge"

const purgeTimeout = 60 * time.Second

// statements delete child tables before their parents (deliveries reference
// subscriptions; the explicit order keeps the intent visible even though the
// FK is ON DELETE CASCADE).
var statements = []struct {
	table string
	sql   string
}{
	{"deliveries", `DELETE FROM deliveries WHERE tenant_id = $1`},
	{"subscriptions", `DELETE FROM subscriptions WHERE tenant_id = $1`},
}

// Start subscribes durably to identity.tenant.expired and purges the
// webhooks schema for each expired tenant.
func Start(ctx context.Context, nc *nats.Conn, pool *pgxpool.Pool) (*nats.Subscription, error) {
	return events.SubscribeTenantExpired(ctx, nc, Durable, purgeTimeout,
		func(ctx context.Context, tenantID uuid.UUID) error {
			counts, err := Tenant(ctx, pool, tenantID)
			if err != nil {
				return err
			}
			total := int64(0)
			args := []any{"schema", "webhooks", "tenant_id", tenantID}
			for _, st := range statements {
				args = append(args, st.table, counts[st.table])
				total += counts[st.table]
			}
			slog.Info(fmt.Sprintf("tenant purge complete: %d rows deleted", total), args...)
			return nil
		})
}

// Tenant deletes every webhooks-schema row of one tenant in a single
// tenant-scoped transaction. Exported for tests.
func Tenant(ctx context.Context, pool *pgxpool.Pool, tenantID uuid.UUID) (map[string]int64, error) {
	counts := make(map[string]int64, len(statements))
	err := pgxutil.WithTenantTx(ctx, pool, tenantID, func(tx pgx.Tx) error {
		for _, st := range statements {
			tag, err := tx.Exec(ctx, st.sql, tenantID)
			if err != nil {
				return fmt.Errorf("purge %s: %w", st.table, err)
			}
			counts[st.table] = tag.RowsAffected()
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return counts, nil
}
