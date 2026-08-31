package app

import (
	"strconv"
	"time"

	"github.com/google/uuid"

	"github.com/ASanchezT85/ledgercore/libs/go/money"

	"github.com/ASanchezT85/ledgercore/services/ledger-core/internal/domain"
)

// Event payloads matching contracts/events/*.schema.json. They are built by
// the persistence adapter inside the business transaction and written to the
// outbox table; the poller publishes them to NATS later.

// EventMoney is the wire representation of a monetary amount: string-encoded
// int64 minor units, per the platform money contract.
type EventMoney struct {
	Asset  string `json:"asset"`
	Amount string `json:"amount"`
}

// NewEventMoney converts a money.Amount into its wire shape.
func NewEventMoney(a money.Amount) EventMoney {
	return EventMoney{Asset: a.Asset, Amount: strconv.FormatInt(a.Units, 10)}
}

// EventPosting is one transaction leg inside an event payload.
type EventPosting struct {
	AccountID uuid.UUID  `json:"account_id"`
	Direction string     `json:"direction"`
	Amount    EventMoney `json:"amount"`
}

// TransactionPostedEvent is the payload of ledger.transaction.posted.
type TransactionPostedEvent struct {
	TransactionID  uuid.UUID         `json:"transaction_id"`
	LedgerID       uuid.UUID         `json:"ledger_id"`
	IdempotencyKey string            `json:"idempotency_key"`
	Description    string            `json:"description,omitempty"`
	Postings       []EventPosting    `json:"postings"`
	Metadata       map[string]string `json:"metadata,omitempty"`
	PostedAt       time.Time         `json:"posted_at"`
}

// NewTransactionPostedEvent builds the posted payload from a domain transaction.
func NewTransactionPostedEvent(t domain.Transaction, postedAt time.Time) TransactionPostedEvent {
	postings := make([]EventPosting, len(t.Postings))
	for i, p := range t.Postings {
		postings[i] = EventPosting{
			AccountID: p.AccountID,
			Direction: string(p.Direction),
			Amount:    NewEventMoney(p.Amount),
		}
	}
	return TransactionPostedEvent{
		TransactionID:  t.ID,
		LedgerID:       t.LedgerID,
		IdempotencyKey: t.IdempotencyKey,
		Description:    t.Description,
		Postings:       postings,
		Metadata:       t.Metadata,
		PostedAt:       postedAt,
	}
}

// TransactionReversedEvent is the payload of ledger.transaction.reversed.
type TransactionReversedEvent struct {
	TransactionID         uuid.UUID `json:"transaction_id"`
	ReversalTransactionID uuid.UUID `json:"reversal_transaction_id"`
	LedgerID              uuid.UUID `json:"ledger_id"`
	Reason                string    `json:"reason,omitempty"`
	ReversedAt            time.Time `json:"reversed_at"`
}

// HoldCreatedEvent is the payload of ledger.hold.created.
type HoldCreatedEvent struct {
	HoldID         uuid.UUID  `json:"hold_id"`
	LedgerID       uuid.UUID  `json:"ledger_id"`
	AccountID      uuid.UUID  `json:"account_id"`
	IdempotencyKey string     `json:"idempotency_key,omitempty"`
	Amount         EventMoney `json:"amount"`
	ExpiresAt      time.Time  `json:"expires_at"`
	CreatedAt      time.Time  `json:"created_at"`
}

// HoldCapturedEvent is the payload of ledger.hold.captured.
type HoldCapturedEvent struct {
	HoldID         uuid.UUID   `json:"hold_id"`
	LedgerID       uuid.UUID   `json:"ledger_id"`
	AccountID      uuid.UUID   `json:"account_id"`
	CapturedAmount EventMoney  `json:"captured_amount"`
	ReleasedAmount *EventMoney `json:"released_amount,omitempty"` // remainder on partial capture
	TransactionID  *uuid.UUID  `json:"transaction_id"`            // linked posted transaction, when provided
	CapturedAt     time.Time   `json:"captured_at"`
}

// HoldReleasedEvent is the payload of ledger.hold.released.
type HoldReleasedEvent struct {
	HoldID     uuid.UUID  `json:"hold_id"`
	LedgerID   uuid.UUID  `json:"ledger_id"`
	AccountID  uuid.UUID  `json:"account_id"`
	Amount     EventMoney `json:"amount"`
	Reason     string     `json:"reason"` // "released" | "expired"
	ReleasedAt time.Time  `json:"released_at"`
}
