package events

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"github.com/nats-io/nats.go"
)

// SubscribeTenantExpired creates a durable JetStream consumer on
// identity.tenant.expired and invokes purge for every delivery. It owns the
// ack protocol shared by every purge consumer:
//
//   - malformed envelope or nil tenant -> Term (never redelivered)
//   - purge error -> Nak (redelivered; purge implementations MUST be idempotent)
//   - success -> Ack
//
// Each service passes its own durable name so deliveries are tracked
// independently per service.
func SubscribeTenantExpired(ctx context.Context, nc *nats.Conn, durable string, timeout time.Duration, purge func(ctx context.Context, tenantID uuid.UUID) error) (*nats.Subscription, error) {
	js, err := nc.JetStream()
	if err != nil {
		return nil, fmt.Errorf("events: jetstream context: %w", err)
	}
	if timeout <= 0 {
		timeout = 60 * time.Second
	}
	sub, err := js.Subscribe(TopicTenantExpired, func(msg *nats.Msg) {
		var env Envelope
		if err := json.Unmarshal(msg.Data, &env); err != nil {
			slog.Error("tenant purge consumer: malformed envelope, terminating",
				"durable", durable, "error", err)
			_ = msg.Term()
			return
		}
		if env.Type != TopicTenantExpired {
			_ = msg.Ack()
			return
		}
		if env.TenantID == uuid.Nil {
			slog.Error("tenant purge consumer: envelope without tenant, terminating",
				"durable", durable, "event_id", env.ID)
			_ = msg.Term()
			return
		}
		purgeCtx, cancel := context.WithTimeout(ctx, timeout)
		defer cancel()
		if err := purge(purgeCtx, env.TenantID); err != nil {
			slog.Error("tenant purge failed, NAKing for redelivery",
				"durable", durable, "event_id", env.ID, "tenant_id", env.TenantID, "error", err)
			_ = msg.Nak()
			return
		}
		_ = msg.Ack()
	},
		nats.Durable(durable),
		nats.ManualAck(),
		nats.AckExplicit(),
		nats.DeliverAll(),
		nats.AckWait(2*timeout),
		nats.BindStream(StreamName),
	)
	if err != nil {
		return nil, fmt.Errorf("events: subscribe %s (%s): %w", TopicTenantExpired, durable, err)
	}
	slog.Info("tenant purge consumer started", "topic", TopicTenantExpired, "durable", durable)
	return sub, nil
}
