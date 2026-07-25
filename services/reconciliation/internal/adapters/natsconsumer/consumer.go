// Package natsconsumer feeds the local ledger mirror from
// ledger.transaction.posted events (event-carried state transfer). The
// reconciliation service never reads the ledger schema: this consumer is its
// only source of ledger truth.
package natsconsumer

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strconv"
	"time"

	"github.com/google/uuid"
	"github.com/ledgercore/ledgercore/libs/go/events"
	"github.com/nats-io/nats.go"

	"github.com/ledgercore/ledgercore/services/reconciliation/internal/domain"
)

// DurableName identifies the JetStream durable consumer, so redeploys resume
// from the last acknowledged event.
const DurableName = "recon-mirror"

// postingNamespace is the fixed UUIDv5 namespace used to derive a
// deterministic id per posting (transaction_id + posting index). The same
// event redelivered always produces the same ids, and the mirror insert is
// ON CONFLICT DO NOTHING — that pair is the idempotency guarantee.
var postingNamespace = uuid.MustParse("a2c1f0b4-8d3e-4f6a-9b7c-1e5d2a8c4f60")

// MirrorWriter is the persistence port the consumer writes through.
type MirrorWriter interface {
	InsertMirrorEntries(ctx context.Context, tenantID uuid.UUID, entries []domain.MirrorEntry) error
}

// Payload shapes of ledger.transaction.posted
// (contracts/events/ledger.transaction.posted.schema.json).
type moneyPayload struct {
	Asset  string `json:"asset"`
	Amount string `json:"amount"` // minor units, string-encoded
}

type postingPayload struct {
	AccountID uuid.UUID    `json:"account_id"`
	Direction string       `json:"direction"`
	Amount    moneyPayload `json:"amount"`
}

type transactionPostedPayload struct {
	TransactionID  uuid.UUID         `json:"transaction_id"`
	LedgerID       uuid.UUID         `json:"ledger_id"`
	IdempotencyKey string            `json:"idempotency_key"`
	Description    string            `json:"description"`
	Postings       []postingPayload  `json:"postings"`
	Metadata       map[string]string `json:"metadata"`
	PostedAt       time.Time         `json:"posted_at"`
}

// Start subscribes durably to ledger.transaction.posted on the LEDGERCORE
// stream. Malformed events are terminated (never redelivered); persistence
// failures are NAKed so JetStream redelivers them.
func Start(ctx context.Context, nc *nats.Conn, writer MirrorWriter, log *slog.Logger) (*nats.Subscription, error) {
	js, err := nc.JetStream()
	if err != nil {
		return nil, fmt.Errorf("natsconsumer: jetstream context: %w", err)
	}

	sub, err := js.Subscribe(events.TopicTransactionPosted, func(msg *nats.Msg) {
		handle(ctx, msg, writer, log)
	},
		nats.Durable(DurableName),
		nats.ManualAck(),
		nats.AckExplicit(),
		nats.DeliverAll(),
		nats.BindStream(events.StreamName),
	)
	if err != nil {
		return nil, fmt.Errorf("natsconsumer: subscribe %s: %w", events.TopicTransactionPosted, err)
	}
	log.Info("mirror consumer started",
		"topic", events.TopicTransactionPosted,
		"durable", DurableName,
	)
	return sub, nil
}

func handle(ctx context.Context, msg *nats.Msg, writer MirrorWriter, log *slog.Logger) {
	var env events.Envelope
	if err := json.Unmarshal(msg.Data, &env); err != nil {
		log.Error("mirror consumer: malformed envelope, terminating message", "error", err)
		_ = msg.Term()
		return
	}
	if env.Type != events.TopicTransactionPosted {
		_ = msg.Ack()
		return
	}
	if env.TenantID == uuid.Nil {
		log.Error("mirror consumer: envelope without tenant, terminating", "event_id", env.ID)
		_ = msg.Term()
		return
	}

	var payload transactionPostedPayload
	if err := json.Unmarshal(env.Data, &payload); err != nil {
		log.Error("mirror consumer: malformed payload, terminating", "event_id", env.ID, "error", err)
		_ = msg.Term()
		return
	}

	entries := make([]domain.MirrorEntry, 0, len(payload.Postings))
	for i, p := range payload.Postings {
		units, err := strconv.ParseInt(p.Amount.Amount, 10, 64)
		if err != nil {
			log.Error("mirror consumer: posting amount is not integer minor units, terminating",
				"event_id", env.ID, "amount", p.Amount.Amount, "error", err)
			_ = msg.Term()
			return
		}
		entries = append(entries, domain.MirrorEntry{
			ID:            PostingID(payload.TransactionID, i),
			TenantID:      env.TenantID,
			TransactionID: payload.TransactionID,
			AccountID:     p.AccountID,
			Direction:     p.Direction,
			Amount:        units,
			Asset:         p.Amount.Asset,
			Reference:     payload.IdempotencyKey,
			PostedAt:      payload.PostedAt,
		})
	}

	if err := writer.InsertMirrorEntries(ctx, env.TenantID, entries); err != nil {
		log.Error("mirror consumer: persist failed, NAKing for redelivery",
			"event_id", env.ID, "error", err)
		_ = msg.Nak()
		return
	}
	_ = msg.Ack()
}

// PostingID derives the deterministic mirror row id for one posting of a
// transaction. Exported for tests.
func PostingID(transactionID uuid.UUID, index int) uuid.UUID {
	return uuid.NewSHA1(postingNamespace, []byte("ledgercore:posting:"+transactionID.String()+"#"+strconv.Itoa(index)))
}
