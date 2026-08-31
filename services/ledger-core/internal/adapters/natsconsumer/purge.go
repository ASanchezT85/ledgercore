// Package natsconsumer hosts the ledger-core JetStream consumers. Today it
// contains only the sandbox purge consumer: on identity.tenant.expired it
// deletes every row the expired tenant owns in the ledger schema.
//
// LC-004: the purge no longer opens the append-only maintenance hatch with the
// runtime role. It calls the SECURITY DEFINER function
// ledger.purge_expired_sandbox_tenant, owned by the restricted ledgercore_maint
// role (see migration 0005). That function is the only sanctioned path allowed
// to bypass the append-only guards; the runtime role cannot bypass them on its
// own. The call still runs inside one tenant-scoped transaction (RLS via
// SET LOCAL app.tenant_id) and is idempotent (a redelivery deletes nothing).
package natsconsumer

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/nats-io/nats.go"

	"github.com/ASanchezT85/ledgercore/libs/go/events"
	"github.com/ASanchezT85/ledgercore/libs/go/pgxutil"
)

// PurgeDurable identifies the durable purge consumer, so redeploys resume
// from the last acknowledged event.
const PurgeDurable = "ledger-purge"

const purgeTimeout = 60 * time.Second

// StartPurgeConsumer subscribes durably to identity.tenant.expired on the
// LEDGERCORE stream and purges the ledger schema for each expired tenant.
func StartPurgeConsumer(ctx context.Context, nc *nats.Conn, pool *pgxpool.Pool) (*nats.Subscription, error) {
	return events.SubscribeTenantExpired(ctx, nc, PurgeDurable, purgeTimeout,
		func(ctx context.Context, tenantID uuid.UUID) error {
			deleted, err := PurgeTenant(ctx, pool, tenantID)
			if err != nil {
				return err
			}
			slog.Info(fmt.Sprintf("tenant purge complete: %d rows deleted", deleted),
				"schema", "ledger", "tenant_id", tenantID, "rows_deleted", deleted)
			return nil
		})
}

// PurgeTenant deletes every ledger-schema row of one tenant by invoking the
// sanctioned SECURITY DEFINER function ledger.purge_expired_sandbox_tenant
// (owned by ledgercore_maint) inside a single tenant-scoped transaction. The
// function refuses tenants that own live ledgers and is the only path allowed
// to bypass the append-only guards. It returns the number of rows deleted and
// is idempotent. Exported for tests.
func PurgeTenant(ctx context.Context, pool *pgxpool.Pool, tenantID uuid.UUID) (int64, error) {
	var deleted int64
	err := pgxutil.WithTenantTx(ctx, pool, tenantID, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT purge_expired_sandbox_tenant($1)`, tenantID).Scan(&deleted)
	})
	if err != nil {
		return 0, fmt.Errorf("purge tenant %s: %w", tenantID, err)
	}
	return deleted, nil
}
