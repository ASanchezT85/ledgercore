// Package outbox drains the identity outbox table and publishes the stored
// envelopes to NATS JetStream. Same pattern as ledger-core: rows are written
// inside the business transaction (see Store.MarkTenantPurging) and this
// poller is the only component that talks to the broker, so a NATS outage
// never fails a sweep.
package outbox

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ASanchezT85/ledgercore/libs/go/events"
	"github.com/ASanchezT85/ledgercore/libs/go/pgxutil"
)

// DefaultInterval is how often the poller wakes up.
const DefaultInterval = 500 * time.Millisecond

// batchSize is the maximum number of events drained per tick.
const batchSize = 100

// Poller periodically publishes unpublished outbox rows.
type Poller struct {
	pool     *pgxpool.Pool
	pub      events.Publisher
	interval time.Duration
}

// NewPoller builds a poller. A zero interval falls back to DefaultInterval.
func NewPoller(pool *pgxpool.Pool, pub events.Publisher, interval time.Duration) *Poller {
	if interval <= 0 {
		interval = DefaultInterval
	}
	return &Poller{pool: pool, pub: pub, interval: interval}
}

// Run drains the outbox on every tick until the context is canceled.
func (p *Poller) Run(ctx context.Context) {
	ticker := time.NewTicker(p.interval)
	defer ticker.Stop()
	slog.Info("identity outbox poller started", "interval", p.interval.String())
	for {
		select {
		case <-ctx.Done():
			slog.Info("identity outbox poller stopped")
			return
		case <-ticker.C:
			n, err := p.drainOnce(ctx)
			if err != nil && ctx.Err() == nil {
				slog.Error("identity outbox drain failed", "error", err)
			}
			if n > 0 {
				slog.Debug("identity outbox events published", "count", n)
			}
		}
	}
}

type outboxRow struct {
	id       int64
	envelope []byte
}

// drainOnce publishes up to batchSize pending events inside a system
// transaction. FOR UPDATE SKIP LOCKED makes concurrent instances cooperate;
// the JetStream MsgId (the event id) dedupes crash-window redeliveries.
func (p *Poller) drainOnce(ctx context.Context) (int, error) {
	published := 0
	var publishErr error
	err := pgxutil.WithSystemTx(ctx, p.pool, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `
			SELECT id, envelope
			FROM outbox
			WHERE published_at IS NULL
			ORDER BY id
			LIMIT $1
			FOR UPDATE SKIP LOCKED`, batchSize)
		if err != nil {
			return fmt.Errorf("select outbox: %w", err)
		}
		pending := make([]outboxRow, 0, batchSize)
		for rows.Next() {
			var r outboxRow
			if err := rows.Scan(&r.id, &r.envelope); err != nil {
				rows.Close()
				return fmt.Errorf("scan outbox row: %w", err)
			}
			pending = append(pending, r)
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return fmt.Errorf("iterate outbox rows: %w", err)
		}
		if len(pending) == 0 {
			return nil
		}

		done := make([]int64, 0, len(pending))
		for _, r := range pending {
			var env events.Envelope
			if err := json.Unmarshal(r.envelope, &env); err != nil {
				slog.Error("identity outbox row has malformed envelope", "id", r.id, "error", err)
				continue
			}
			if err := p.pub.Publish(env); err != nil {
				publishErr = fmt.Errorf("publish outbox row %d: %w", r.id, err)
				break
			}
			done = append(done, r.id)
		}
		if len(done) > 0 {
			if _, err := tx.Exec(ctx,
				`UPDATE outbox SET published_at = now() WHERE id = ANY($1)`, done); err != nil {
				return fmt.Errorf("mark outbox published: %w", err)
			}
			published = len(done)
		}
		return nil
	})
	if err != nil {
		return published, err
	}
	return published, publishErr
}
