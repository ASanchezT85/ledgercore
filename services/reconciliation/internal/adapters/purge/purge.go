// Package purge deletes every recon-schema row of a sandbox tenant when
// identity announces its expiry (identity.tenant.expired). Unlike the ledger
// schema there is no append-only hatch to open: recon tables are plain
// mutable state. Deletes run in ONE tenant-scoped transaction, child-first
// so no FK is violated, and are idempotent (a redelivery deletes nothing).
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

	"github.com/ASanchezT85/ledgercore/libs/go/events"
	"github.com/ASanchezT85/ledgercore/libs/go/pgxutil"
)

// Durable identifies this service's purge consumer on the LEDGERCORE stream.
const Durable = "recon-purge"

const purgeTimeout = 60 * time.Second

// statements delete child tables before their parents.
var statements = []struct {
	table string
	sql   string
}{
	{"discrepancies", `DELETE FROM discrepancies WHERE tenant_id = $1`},
	{"reconciliation_runs", `DELETE FROM reconciliation_runs WHERE tenant_id = $1`},
	{"external_transactions", `DELETE FROM external_transactions WHERE tenant_id = $1`},
	{"imports", `DELETE FROM imports WHERE tenant_id = $1`},
	{"sources", `DELETE FROM sources WHERE tenant_id = $1`},
	{"ledger_entries_mirror", `DELETE FROM ledger_entries_mirror WHERE tenant_id = $1`},
	{"outbox", `DELETE FROM outbox WHERE tenant_id = $1`},
}

// Start subscribes durably to identity.tenant.expired and purges the recon
// schema for each expired tenant.
func Start(ctx context.Context, nc *nats.Conn, pool *pgxpool.Pool) (*nats.Subscription, error) {
	return events.SubscribeTenantExpired(ctx, nc, Durable, purgeTimeout,
		func(ctx context.Context, tenantID uuid.UUID) error {
			counts, err := Tenant(ctx, pool, tenantID)
			if err != nil {
				return err
			}
			total := int64(0)
			args := []any{"schema", "recon", "tenant_id", tenantID}
			for _, st := range statements {
				args = append(args, st.table, counts[st.table])
				total += counts[st.table]
			}
			slog.Info(fmt.Sprintf("tenant purge complete: %d rows deleted", total), args...)
			return nil
		})
}

// Tenant deletes every recon-schema row of one tenant in a single
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
