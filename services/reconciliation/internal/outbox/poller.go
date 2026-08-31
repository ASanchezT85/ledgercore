// Package outbox runs the poller goroutine that drains the transactional
// outbox into NATS JetStream. Events are inserted into the outbox inside the
// same business transaction that produced them; this poller is the only
// component that talks to the broker for publishing.
package outbox

import (
	"context"
	"log/slog"
	"time"

	"github.com/ASanchezT85/ledgercore/libs/go/events"
)

// DefaultInterval is how often the poller checks for pending rows.
const DefaultInterval = time.Second

// DefaultBatchSize is how many rows are drained per tick.
const DefaultBatchSize = 100

// Drainer publishes pending outbox rows and marks them published.
type Drainer interface {
	DrainOutbox(ctx context.Context, pub events.Publisher, limit int) (int, error)
}

// Run loops until ctx is cancelled. Errors are logged and retried on the next
// tick; the poller never crashes the service.
func Run(ctx context.Context, d Drainer, pub events.Publisher, interval time.Duration, log *slog.Logger) {
	if interval <= 0 {
		interval = DefaultInterval
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	log.Info("outbox poller started", "interval", interval.String())
	for {
		select {
		case <-ctx.Done():
			log.Info("outbox poller stopped")
			return
		case <-ticker.C:
		}
		n, err := d.DrainOutbox(ctx, pub, DefaultBatchSize)
		if err != nil {
			log.Warn("outbox drain failed; will retry", "published", n, "error", err)
			continue
		}
		if n > 0 {
			log.Info("outbox drained", "published", n)
		}
	}
}
