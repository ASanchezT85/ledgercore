// Package dispatcher is the delivery worker: it leases due deliveries from
// the store, POSTs the signed event envelope to the subscriber endpoint and
// records the outcome, applying the domain backoff policy.
//
// Semantics are at-least-once: a crash between a successful POST and the
// MarkDelivered write causes a re-send after the lease expires. Receivers
// must deduplicate on X-LedgerCore-Event-Id.
package dispatcher

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/ledgercore/ledgercore/services/webhooks/internal/domain"
	"github.com/ledgercore/ledgercore/services/webhooks/internal/signature"
)

// Store is the persistence port the dispatcher needs.
type Store interface {
	ClaimDue(ctx context.Context, limit int, lease time.Duration) ([]domain.ClaimedDelivery, error)
	MarkDelivered(ctx context.Context, tenantID, id uuid.UUID, statusCode int) error
	MarkFailed(ctx context.Context, tenantID, id uuid.UUID, attempts int, statusCode *int, lastError string, nextAttemptAt time.Time, dead bool) error
}

// Defaults for the worker loop.
const (
	DefaultInterval  = 1 * time.Second
	DefaultBatchSize = 50
	DefaultLease     = 2 * time.Minute
	DefaultTimeout   = 10 * time.Second

	maxErrorLen = 500
)

// Dispatcher drives the delivery loop.
type Dispatcher struct {
	store     Store
	client    *http.Client
	interval  time.Duration
	batchSize int
	lease     time.Duration
}

// New builds a dispatcher with the default HTTP client (10s timeout).
func New(store Store) *Dispatcher {
	return &Dispatcher{
		store:     store,
		client:    &http.Client{Timeout: DefaultTimeout},
		interval:  DefaultInterval,
		batchSize: DefaultBatchSize,
		lease:     DefaultLease,
	}
}

// Run blocks, ticking every interval until ctx is canceled.
func (d *Dispatcher) Run(ctx context.Context) {
	ticker := time.NewTicker(d.interval)
	defer ticker.Stop()
	slog.Info("webhook dispatcher started",
		"interval", d.interval.String(), "batch_size", d.batchSize, "lease", d.lease.String())
	for {
		select {
		case <-ctx.Done():
			slog.Info("webhook dispatcher stopped")
			return
		case <-ticker.C:
			d.tick(ctx)
		}
	}
}

// tick drains everything currently due, batch by batch.
func (d *Dispatcher) tick(ctx context.Context) {
	for {
		claimed, err := d.store.ClaimDue(ctx, d.batchSize, d.lease)
		if err != nil {
			if ctx.Err() == nil {
				slog.Error("claim due deliveries failed", "error", err)
			}
			return
		}
		if len(claimed) == 0 {
			return
		}
		// Deliver the batch concurrently; each POST is bounded by the client
		// timeout, so a full batch finishes well within the lease.
		var wg sync.WaitGroup
		for _, c := range claimed {
			wg.Add(1)
			go func(c domain.ClaimedDelivery) {
				defer wg.Done()
				d.process(ctx, c)
			}(c)
		}
		wg.Wait()
		if len(claimed) < d.batchSize || ctx.Err() != nil {
			return
		}
	}
}

// process performs one delivery attempt and records the outcome.
func (d *Dispatcher) process(ctx context.Context, c domain.ClaimedDelivery) {
	statusCode, sendErr := Send(ctx, d.client, c)

	if sendErr == nil && statusCode >= 200 && statusCode < 300 {
		if err := d.store.MarkDelivered(ctx, c.TenantID, c.ID, statusCode); err != nil {
			slog.Error("mark delivered failed", "delivery_id", c.ID, "error", err)
		}
		return
	}

	attempts := c.Attempts + 1
	var codePtr *int
	lastError := ""
	if sendErr != nil {
		lastError = truncate(sendErr.Error(), maxErrorLen)
	} else {
		codePtr = &statusCode
		lastError = fmt.Sprintf("endpoint returned status %d", statusCode)
	}
	dead := domain.IsDead(attempts)
	next := time.Now().UTC()
	if !dead {
		next = next.Add(domain.Backoff(attempts))
	}
	if err := d.store.MarkFailed(ctx, c.TenantID, c.ID, attempts, codePtr, lastError, next, dead); err != nil {
		slog.Error("mark failed failed", "delivery_id", c.ID, "error", err)
		return
	}
	slog.Warn("webhook delivery attempt failed",
		"delivery_id", c.ID,
		"subscription_id", c.SubscriptionID,
		"event_id", c.EventID,
		"attempts", attempts,
		"dead", dead,
		"error", lastError,
	)
}

// Send POSTs the stored envelope to the subscriber endpoint with the signed
// headers. It returns the HTTP status code (0 on transport error).
func Send(ctx context.Context, client *http.Client, c domain.ClaimedDelivery) (int, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.URL, bytes.NewReader(c.Payload))
	if err != nil {
		return 0, fmt.Errorf("dispatcher: build request: %w", err)
	}
	ts := time.Now().Unix()
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-LedgerCore-Event-Id", c.EventID.String())
	req.Header.Set("X-LedgerCore-Event-Type", c.EventType)
	req.Header.Set(signature.HeaderName, signature.Header(c.Secret, ts, c.Payload))

	resp, err := client.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	// Drain a bounded amount so the connection can be reused.
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
	return resp.StatusCode, nil
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}
