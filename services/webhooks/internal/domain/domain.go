// Package domain holds the pure business model of the webhooks service:
// subscriptions, deliveries, event-type matching, the retry/backoff policy,
// endpoint validation and secret generation. It performs no I/O.
package domain

import (
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"net/url"
	"strings"
	"time"

	"github.com/google/uuid"
)

// ---- Errors -----------------------------------------------------------------

// ErrNotFound is returned when a tenant-scoped resource does not exist.
var ErrNotFound = errors.New("resource not found")

// ErrConflict is returned when an operation is not valid for the current
// state of the resource (e.g. retrying a delivery that is not failed/dead).
var ErrConflict = errors.New("conflict with resource state")

// ValidationError describes invalid caller input. Its message is safe to
// return to API clients.
type ValidationError struct{ msg string }

func (e ValidationError) Error() string { return e.msg }

// Invalidf builds a ValidationError with a formatted message.
func Invalidf(format string, args ...any) error {
	return ValidationError{msg: fmt.Sprintf(format, args...)}
}

// ---- Event types --------------------------------------------------------------

// Wildcard subscribes an endpoint to every event type.
const Wildcard = "*"

// KnownEventTypes mirrors the canonical topic constants in libs/go/events.
// The list is duplicated here so the domain package does not import the
// events package; a unit test asserts both lists stay in sync.
var KnownEventTypes = []string{
	"ledger.transaction.posted",
	"ledger.transaction.reversed",
	"ledger.hold.created",
	"ledger.hold.captured",
	"ledger.hold.released",
	"recon.discrepancy.detected",
}

// Matches reports whether a subscription with the given event_types should
// receive an event of the given type. The wildcard "*" matches everything.
func Matches(eventTypes []string, eventType string) bool {
	for _, t := range eventTypes {
		if t == Wildcard || t == eventType {
			return true
		}
	}
	return false
}

// ValidateEventTypes checks that the list is non-empty and every entry is
// either the wildcard or a known event type.
func ValidateEventTypes(eventTypes []string) error {
	if len(eventTypes) == 0 {
		return Invalidf("event_types must contain at least one entry")
	}
	for _, t := range eventTypes {
		if t == Wildcard {
			continue
		}
		known := false
		for _, k := range KnownEventTypes {
			if t == k {
				known = true
				break
			}
		}
		if !known {
			return Invalidf("unknown event type %q; allowed: %s or %q",
				t, strings.Join(KnownEventTypes, ", "), Wildcard)
		}
	}
	return nil
}

// NormalizeEventTypes removes duplicates while preserving order.
func NormalizeEventTypes(eventTypes []string) []string {
	seen := make(map[string]struct{}, len(eventTypes))
	out := make([]string, 0, len(eventTypes))
	for _, t := range eventTypes {
		if _, ok := seen[t]; ok {
			continue
		}
		seen[t] = struct{}{}
		out = append(out, t)
	}
	return out
}

// ---- Endpoint URL ---------------------------------------------------------------

// ValidateEndpointURL checks that raw is an absolute http(s) URL. When
// requireHTTPS is true (live environment) only https endpoints are accepted.
func ValidateEndpointURL(raw string, requireHTTPS bool) error {
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return Invalidf("url must be an absolute http(s) URL")
	}
	switch u.Scheme {
	case "https":
	case "http":
		if requireHTTPS {
			return Invalidf("url must use https in the live environment")
		}
	default:
		return Invalidf("url scheme must be http or https")
	}
	return nil
}

// ---- Secrets ---------------------------------------------------------------------

// SecretPrefix identifies LedgerCore webhook secrets (Stripe-style whsec_).
const SecretPrefix = "whsec_"

const (
	secretAlphabet = "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz"
	secretLength   = 32
)

// NewSecret generates a webhook signing secret: "whsec_" followed by 32
// base62 characters drawn from crypto/rand (~190 bits of entropy).
// Rejection-free: rand.Int is uniform over the alphabet size.
func NewSecret() (string, error) {
	var b strings.Builder
	b.Grow(len(SecretPrefix) + secretLength)
	b.WriteString(SecretPrefix)
	alphabetSize := big.NewInt(int64(len(secretAlphabet)))
	for i := 0; i < secretLength; i++ {
		n, err := rand.Int(rand.Reader, alphabetSize)
		if err != nil {
			return "", fmt.Errorf("domain: generate secret: %w", err)
		}
		b.WriteByte(secretAlphabet[n.Int64()])
	}
	return b.String(), nil
}

// ---- Retry policy -----------------------------------------------------------------

// MaxAttempts is the number of delivery attempts before a delivery is dead.
const MaxAttempts = 5

// backoffSchedule holds the wait after the Nth failed attempt (1-based).
var backoffSchedule = []time.Duration{
	1 * time.Minute,
	5 * time.Minute,
	30 * time.Minute,
	2 * time.Hour,
	12 * time.Hour,
}

// Backoff returns the wait before the next attempt after `attempts` failed
// attempts (1-based). Out-of-range values clamp to the schedule bounds.
func Backoff(attempts int) time.Duration {
	if attempts < 1 {
		attempts = 1
	}
	if attempts > len(backoffSchedule) {
		attempts = len(backoffSchedule)
	}
	return backoffSchedule[attempts-1]
}

// IsDead reports whether a delivery with the given attempt count must be
// marked dead instead of being rescheduled.
func IsDead(attempts int) bool { return attempts >= MaxAttempts }

// ---- Delivery statuses ---------------------------------------------------------------

// Delivery lifecycle: pending -> delivered, or pending -> failed (retrying)
// -> dead after MaxAttempts. A manual retry re-enqueues failed/dead as pending.
const (
	StatusPending   = "pending"
	StatusDelivered = "delivered"
	StatusFailed    = "failed"
	StatusDead      = "dead"
)

// ValidStatus reports whether s is a known delivery status.
func ValidStatus(s string) bool {
	switch s {
	case StatusPending, StatusDelivered, StatusFailed, StatusDead:
		return true
	}
	return false
}

// ---- Entities -------------------------------------------------------------------------

// Subscription is a tenant-owned webhook endpoint registration.
type Subscription struct {
	ID         uuid.UUID
	TenantID   uuid.UUID
	URL        string
	Secret     string // signing secret; never exposed after creation/rotation
	EventTypes []string
	Active     bool
	CreatedAt  time.Time
}

// Delivery is one event fanned out to one subscription.
type Delivery struct {
	ID             uuid.UUID
	TenantID       uuid.UUID
	SubscriptionID uuid.UUID
	EventID        uuid.UUID
	EventType      string
	Payload        json.RawMessage
	Status         string
	Attempts       int
	NextAttemptAt  time.Time
	LastStatusCode *int
	LastError      *string
	CreatedAt      time.Time
	DeliveredAt    *time.Time
}

// ClaimedDelivery is a delivery leased by the dispatcher worker, joined with
// the endpoint data it needs to perform the HTTP POST.
type ClaimedDelivery struct {
	ID             uuid.UUID
	TenantID       uuid.UUID
	SubscriptionID uuid.UUID
	EventID        uuid.UUID
	EventType      string
	Payload        []byte
	Attempts       int
	URL            string
	Secret         string
}
