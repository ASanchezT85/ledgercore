package events

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestNewEnvelope(t *testing.T) {
	tenant := uuid.New()
	payload := map[string]any{"transaction_id": uuid.New().String(), "status": "posted"}

	env, err := NewEnvelope(TopicTransactionPosted, tenant, payload)
	if err != nil {
		t.Fatalf("NewEnvelope: %v", err)
	}
	if env.ID == uuid.Nil {
		t.Fatal("envelope id must not be nil")
	}
	if env.ID.Version() != 7 {
		t.Fatalf("envelope id version = %d, want UUIDv7", env.ID.Version())
	}
	if env.Type != TopicTransactionPosted {
		t.Fatalf("type = %q", env.Type)
	}
	if env.TenantID != tenant {
		t.Fatalf("tenant = %s, want %s", env.TenantID, tenant)
	}
	if env.Version != EnvelopeVersion {
		t.Fatalf("version = %d, want %d", env.Version, EnvelopeVersion)
	}
	if time.Since(env.OccurredAt) > time.Minute || env.OccurredAt.Location() != time.UTC {
		t.Fatalf("occurred_at looks wrong: %v", env.OccurredAt)
	}

	var data map[string]any
	if err := json.Unmarshal(env.Data, &data); err != nil {
		t.Fatalf("data is not valid JSON: %v", err)
	}
	if data["status"] != "posted" {
		t.Fatalf("payload lost: %v", data)
	}
}

func TestNewEnvelopeRejectsUnmarshalablePayload(t *testing.T) {
	if _, err := NewEnvelope(TopicHoldCreated, uuid.New(), make(chan int)); err == nil {
		t.Fatal("expected error for unmarshalable payload")
	}
}

func TestEnvelopeMarshalRoundTrip(t *testing.T) {
	orig, err := NewEnvelope(TopicHoldReleased, uuid.New(), map[string]string{"hold_id": "h1"})
	if err != nil {
		t.Fatal(err)
	}

	raw, err := json.Marshal(orig)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	// Wire format uses snake_case field names.
	var wire map[string]any
	if err := json.Unmarshal(raw, &wire); err != nil {
		t.Fatal(err)
	}
	for _, field := range []string{"id", "type", "tenant_id", "occurred_at", "version", "data"} {
		if _, ok := wire[field]; !ok {
			t.Fatalf("wire format missing field %q: %s", field, raw)
		}
	}

	var back Envelope
	if err := json.Unmarshal(raw, &back); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if back.ID != orig.ID || back.Type != orig.Type || back.TenantID != orig.TenantID ||
		back.Version != orig.Version || !back.OccurredAt.Equal(orig.OccurredAt) {
		t.Fatalf("round trip mismatch:\n got %+v\nwant %+v", back, orig)
	}
	if string(back.Data) != string(orig.Data) {
		t.Fatalf("data mismatch: %s vs %s", back.Data, orig.Data)
	}
}

func TestTopicConstants(t *testing.T) {
	want := map[string]string{
		TopicTransactionPosted:   "ledger.transaction.posted",
		TopicTransactionReversed: "ledger.transaction.reversed",
		TopicHoldCreated:         "ledger.hold.created",
		TopicHoldCaptured:        "ledger.hold.captured",
		TopicHoldReleased:        "ledger.hold.released",
		TopicDiscrepancyDetected: "recon.discrepancy.detected",
	}
	for got, expected := range want {
		if got != expected {
			t.Fatalf("topic constant %q != %q", got, expected)
		}
	}
}
