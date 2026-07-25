// Package events defines the event envelope, the canonical topic names and
// the publisher abstraction used by every LedgerCore service.
//
// Services never publish directly from request handlers: they insert the
// serialized envelope into their outbox table inside the business
// transaction, and a poller goroutine drains the outbox through a Publisher.
package events

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// Canonical topic names. Topics are also NATS subjects.
const (
	TopicTransactionPosted   = "ledger.transaction.posted"
	TopicTransactionReversed = "ledger.transaction.reversed"
	TopicHoldCreated         = "ledger.hold.created"
	TopicHoldCaptured        = "ledger.hold.captured"
	TopicHoldReleased        = "ledger.hold.released"
	TopicDiscrepancyDetected = "recon.discrepancy.detected"
	// TopicTenantExpired announces that a sandbox tenant passed its TTL.
	// Every service owning tenant data consumes it and purges its schema.
	TopicTenantExpired = "identity.tenant.expired"
)

// EnvelopeVersion is the current envelope schema version.
const EnvelopeVersion = 1

// Envelope wraps every event published by the platform. Data carries the
// topic-specific payload, described by the JSON Schemas in contracts/events.
type Envelope struct {
	ID         uuid.UUID       `json:"id"`
	Type       string          `json:"type"`
	TenantID   uuid.UUID       `json:"tenant_id"`
	OccurredAt time.Time       `json:"occurred_at"`
	Version    int             `json:"version"`
	Data       json.RawMessage `json:"data"`
}

// NewEnvelope builds an envelope for the given topic, marshaling data as the
// payload. The event id is a new UUIDv7 so ids sort by creation time.
func NewEnvelope(topic string, tenantID uuid.UUID, data any) (Envelope, error) {
	payload, err := json.Marshal(data)
	if err != nil {
		return Envelope{}, fmt.Errorf("events: marshal payload for %s: %w", topic, err)
	}
	id, err := uuid.NewV7()
	if err != nil {
		return Envelope{}, fmt.Errorf("events: generate event id: %w", err)
	}
	return Envelope{
		ID:         id,
		Type:       topic,
		TenantID:   tenantID,
		OccurredAt: time.Now().UTC(),
		Version:    EnvelopeVersion,
		Data:       payload,
	}, nil
}

// Publisher publishes envelopes to their topic. Implementations must be safe
// for concurrent use.
type Publisher interface {
	Publish(env Envelope) error
}
