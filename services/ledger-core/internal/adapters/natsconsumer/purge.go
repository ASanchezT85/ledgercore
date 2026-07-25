// Package natsconsumer hosts the ledger-core JetStream consumers. Today it
// contains only the sandbox purge consumer: on identity.tenant.expired it
// deletes every row the expired tenant owns in the ledger schema.
//
// This is the ONLY application code path allowed to use the
// ledger.allow_maintenance hatch (blueprint §14, Fase 1a): the deletes run in
// one tenant-scoped transaction with SET LOCAL, in strict child-first order,
// and are idempotent (a redelivery finds nothing to delete and acks).
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

	"github.com/ledgercore/ledgercore/libs/go/events"
	"github.com/ledgercore/ledgercore/libs/go/pgxutil"
)

// PurgeDurable identifies the durable purge consumer, so redeploys resume
// from the last acknowledged event.
const PurgeDurable = "ledger-purge"

const purgeTimeout = 60 * time.Second

// purgeStatements deletes tenant data child-first so no FK is ever violated.
// Every statement is scoped by tenant_id; RLS (tenant tx) enforces the same
// scope a second time.
var purgeStatements = []struct {
	table string
	sql   string
}{
	{"postings", `DELETE FROM postings WHERE tenant_id = $1`},
	{"idempotency_keys", `DELETE FROM idempotency_keys WHERE tenant_id = $1`},
	{"holds", `DELETE FROM holds WHERE tenant_id = $1`},
	{"account_balances", `DELETE FROM account_balances WHERE tenant_id = $1`},
	{"transactions", `DELETE FROM transactions WHERE tenant_id = $1`},
	{"accounts", `DELETE FROM accounts WHERE tenant_id = $1`},
	{"ledgers", `DELETE FROM ledgers WHERE tenant_id = $1`},
	{"outbox", `DELETE FROM outbox WHERE tenant_id = $1`},
}

// StartPurgeConsumer subscribes durably to identity.tenant.expired on the
// LEDGERCORE stream and purges the ledger schema for each expired tenant.
func StartPurgeConsumer(ctx context.Context, nc *nats.Conn, pool *pgxpool.Pool) (*nats.Subscription, error) {
	return events.SubscribeTenantExpired(ctx, nc, PurgeDurable, purgeTimeout,
		func(ctx context.Context, tenantID uuid.UUID) error {
			counts, err := PurgeTenant(ctx, pool, tenantID)
			if err != nil {
				return err
			}
			logPurge("ledger", tenantID, counts)
			return nil
		})
}

func logPurge(schema string, tenantID uuid.UUID, counts map[string]int64) {
	total := int64(0)
	args := make([]any, 0, 2*len(counts)+4)
	args = append(args, "schema", schema, "tenant_id", tenantID)
	for _, st := range purgeStatements {
		args = append(args, st.table, counts[st.table])
		total += counts[st.table]
	}
	slog.Info(fmt.Sprintf("tenant purge complete: %d rows deleted", total), args...)
}

// PurgeTenant deletes every ledger-schema row of one tenant in a single
// tenant-scoped transaction with the maintenance hatch enabled. It returns
// deleted row counts per table and is idempotent. Exported for tests.
func PurgeTenant(ctx context.Context, pool *pgxpool.Pool, tenantID uuid.UUID) (map[string]int64, error) {
	counts := make(map[string]int64, len(purgeStatements))
	err := pgxutil.WithTenantTx(ctx, pool, tenantID, func(tx pgx.Tx) error {
		// SET LOCAL: the hatch dies with this transaction.
		if _, err := tx.Exec(ctx, `SET LOCAL ledger.allow_maintenance = '1'`); err != nil {
			return fmt.Errorf("enable maintenance hatch: %w", err)
		}
		for _, st := range purgeStatements {
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
