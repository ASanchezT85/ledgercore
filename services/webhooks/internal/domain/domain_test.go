package domain_test

import (
	"errors"
	"regexp"
	"testing"
	"time"

	"github.com/ASanchezT85/ledgercore/libs/go/events"
	"github.com/ASanchezT85/ledgercore/services/webhooks/internal/domain"
)

func TestKnownEventTypesStayInSyncWithEventsLib(t *testing.T) {
	want := []string{
		events.TopicTransactionPosted,
		events.TopicTransactionReversed,
		events.TopicHoldCreated,
		events.TopicHoldCaptured,
		events.TopicHoldReleased,
		events.TopicDiscrepancyDetected,
	}
	if len(domain.KnownEventTypes) != len(want) {
		t.Fatalf("KnownEventTypes has %d entries, events lib has %d", len(domain.KnownEventTypes), len(want))
	}
	for i, w := range want {
		if domain.KnownEventTypes[i] != w {
			t.Fatalf("KnownEventTypes[%d] = %q, want %q", i, domain.KnownEventTypes[i], w)
		}
	}
}

func TestMatches(t *testing.T) {
	cases := []struct {
		name       string
		eventTypes []string
		eventType  string
		want       bool
	}{
		{"exact match", []string{"ledger.transaction.posted"}, "ledger.transaction.posted", true},
		{"no match", []string{"ledger.transaction.posted"}, "ledger.hold.created", false},
		{"wildcard matches everything", []string{"*"}, "recon.discrepancy.detected", true},
		{"wildcard among others", []string{"ledger.hold.created", "*"}, "ledger.transaction.reversed", true},
		{"empty list matches nothing", []string{}, "ledger.transaction.posted", false},
		{"nil list matches nothing", nil, "ledger.transaction.posted", false},
		{"prefix is not a match", []string{"ledger.transaction"}, "ledger.transaction.posted", false},
		{"case sensitive", []string{"Ledger.Transaction.Posted"}, "ledger.transaction.posted", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := domain.Matches(c.eventTypes, c.eventType); got != c.want {
				t.Fatalf("Matches(%v, %q) = %v, want %v", c.eventTypes, c.eventType, got, c.want)
			}
		})
	}
}

func TestValidateEventTypes(t *testing.T) {
	if err := domain.ValidateEventTypes(nil); err == nil {
		t.Fatal("empty list must be rejected")
	}
	if err := domain.ValidateEventTypes([]string{"ledger.transaction.posted", "*"}); err != nil {
		t.Fatalf("valid list rejected: %v", err)
	}
	err := domain.ValidateEventTypes([]string{"ledger.transaction.typo"})
	var ve domain.ValidationError
	if !errors.As(err, &ve) {
		t.Fatalf("unknown type must yield ValidationError, got %v", err)
	}
}

func TestNormalizeEventTypes(t *testing.T) {
	got := domain.NormalizeEventTypes([]string{"a", "b", "a", "c", "b"})
	want := []string{"a", "b", "c"}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got %v, want %v", got, want)
		}
	}
}

func TestValidateEndpointURL(t *testing.T) {
	cases := []struct {
		name         string
		url          string
		requireHTTPS bool
		wantErr      bool
	}{
		{"https always ok", "https://client.example.com/hooks", true, false},
		{"http ok in sandbox", "http://localhost:9999/hooks", false, false},
		{"http rejected in live", "http://client.example.com/hooks", true, true},
		{"relative rejected", "/hooks", false, true},
		{"empty rejected", "", false, true},
		{"ftp rejected", "ftp://client.example.com/hooks", false, true},
		{"garbage rejected", "::::", false, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := domain.ValidateEndpointURL(c.url, c.requireHTTPS)
			if (err != nil) != c.wantErr {
				t.Fatalf("ValidateEndpointURL(%q, %v) error = %v, wantErr %v", c.url, c.requireHTTPS, err, c.wantErr)
			}
		})
	}
}

func TestBackoffSchedule(t *testing.T) {
	cases := []struct {
		attempts int
		want     time.Duration
	}{
		{1, 1 * time.Minute},
		{2, 5 * time.Minute},
		{3, 30 * time.Minute},
		{4, 2 * time.Hour},
		{5, 12 * time.Hour},
		// Clamping.
		{0, 1 * time.Minute},
		{-3, 1 * time.Minute},
		{99, 12 * time.Hour},
	}
	for _, c := range cases {
		if got := domain.Backoff(c.attempts); got != c.want {
			t.Fatalf("Backoff(%d) = %v, want %v", c.attempts, got, c.want)
		}
	}
}

func TestIsDead(t *testing.T) {
	if domain.IsDead(domain.MaxAttempts - 1) {
		t.Fatal("delivery must still retry below MaxAttempts")
	}
	if !domain.IsDead(domain.MaxAttempts) {
		t.Fatal("delivery must be dead at MaxAttempts")
	}
}

func TestNewSecretFormat(t *testing.T) {
	pattern := regexp.MustCompile(`^lcwh_[0-9A-Za-z]{32}$`)
	seen := map[string]struct{}{}
	for i := 0; i < 50; i++ {
		s, err := domain.NewSecret()
		if err != nil {
			t.Fatalf("NewSecret: %v", err)
		}
		if !pattern.MatchString(s) {
			t.Fatalf("secret %q does not match %s", s, pattern)
		}
		if _, dup := seen[s]; dup {
			t.Fatalf("NewSecret produced a duplicate: %s", s)
		}
		seen[s] = struct{}{}
	}
}

func TestValidStatus(t *testing.T) {
	for _, s := range []string{"pending", "delivered", "failed", "dead"} {
		if !domain.ValidStatus(s) {
			t.Fatalf("%q must be a valid status", s)
		}
	}
	if domain.ValidStatus("retrying") {
		t.Fatal("unknown status accepted")
	}
}
