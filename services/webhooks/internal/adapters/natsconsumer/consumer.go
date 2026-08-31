// Package natsconsumer subscribes to the platform event stream and fans
// every envelope out into pending webhook deliveries.
//
// Two durable push consumers ("webhooks-ledger" on ledger.> and
// "webhooks-recon" on recon.>) cover every topic in the LEDGERCORE stream.
// Handlers ack only after the deliveries are persisted; the UNIQUE
// (subscription_id, event_id) constraint makes redeliveries idempotent.
package natsconsumer

import (
	"context"
	"encoding/json"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"github.com/nats-io/nats.go"

	"github.com/ASanchezT85/ledgercore/libs/go/events"
	"github.com/ASanchezT85/ledgercore/services/webhooks/internal/app"
)

// subscriptionSpec pairs a filter subject with its durable consumer name.
var subscriptionSpecs = []struct {
	subject string
	durable string
}{
	{subject: "ledger.>", durable: "webhooks-ledger"},
	{subject: "recon.>", durable: "webhooks-recon"},
}

const (
	setupRetryEvery = 5 * time.Second
	handleTimeout   = 15 * time.Second
	nakDelay        = 10 * time.Second
)

// Consumer owns the NATS connection and the durable subscriptions.
type Consumer struct {
	url string
	svc *app.Service

	nc *nats.Conn
}

// New builds a consumer; call Start to connect and subscribe.
func New(url string, svc *app.Service) *Consumer {
	return &Consumer{url: url, svc: svc}
}

// Start connects and subscribes in the background, retrying until it
// succeeds or ctx is canceled. The service stays up (and healthy) while NATS
// is unreachable; events buffered in JetStream are consumed once it returns.
func (c *Consumer) Start(ctx context.Context) {
	go func() {
		for {
			err := c.setup()
			if err == nil {
				slog.Info("webhooks NATS consumer subscribed", "url", c.url)
				return
			}
			slog.Warn("webhooks NATS consumer setup failed; retrying",
				"url", c.url, "retry_in", setupRetryEvery.String(), "error", err)
			select {
			case <-ctx.Done():
				return
			case <-time.After(setupRetryEvery):
			}
		}
	}()
}

func (c *Consumer) setup() error {
	nc, err := nats.Connect(c.url,
		nats.Name("webhooks"),
		nats.MaxReconnects(-1),
		nats.ReconnectWait(2*time.Second),
	)
	if err != nil {
		return err
	}
	// NewNATSPublisher is used purely to ensure the LEDGERCORE stream exists
	// with the canonical configuration; this service never publishes.
	if _, err := events.NewNATSPublisher(nc); err != nil {
		nc.Close()
		return err
	}
	js, err := nc.JetStream()
	if err != nil {
		nc.Close()
		return err
	}
	for _, spec := range subscriptionSpecs {
		_, err := js.Subscribe(spec.subject, c.handle,
			nats.Durable(spec.durable),
			nats.BindStream(events.StreamName),
			nats.ManualAck(),
			nats.AckExplicit(),
			nats.AckWait(30*time.Second),
			nats.MaxAckPending(256),
		)
		if err != nil {
			nc.Close()
			return err
		}
	}
	c.nc = nc
	return nil
}

// handle processes one event: fan out to matching subscriptions, then ack.
func (c *Consumer) handle(msg *nats.Msg) {
	var env events.Envelope
	if err := json.Unmarshal(msg.Data, &env); err != nil {
		slog.Error("discarding malformed event envelope", "subject", msg.Subject, "error", err)
		_ = msg.Term() // never processable; do not redeliver
		return
	}
	if env.ID == uuid.Nil || env.TenantID == uuid.Nil || env.Type == "" {
		slog.Error("discarding envelope with missing identity fields", "subject", msg.Subject, "event_id", env.ID)
		_ = msg.Term()
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), handleTimeout)
	defer cancel()
	if err := c.svc.EnqueueEnvelope(ctx, env, msg.Data); err != nil {
		slog.Error("enqueue webhook deliveries failed; will retry",
			"event_id", env.ID, "event_type", env.Type, "tenant_id", env.TenantID, "error", err)
		_ = msg.NakWithDelay(nakDelay)
		return
	}
	_ = msg.Ack()
}

// Close drains the connection, letting in-flight handlers finish.
func (c *Consumer) Close() {
	if c.nc != nil && !c.nc.IsClosed() {
		if err := c.nc.Drain(); err != nil {
			c.nc.Close()
		}
	}
}
