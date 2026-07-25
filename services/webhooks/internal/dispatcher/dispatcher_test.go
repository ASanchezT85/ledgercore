package dispatcher

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/ledgercore/ledgercore/services/webhooks/internal/domain"
	"github.com/ledgercore/ledgercore/services/webhooks/internal/signature"
)

// fakeStore records the outcome writes performed by the dispatcher.
type fakeStore struct {
	mu        sync.Mutex
	delivered []struct {
		id   uuid.UUID
		code int
	}
	failed []struct {
		id       uuid.UUID
		attempts int
		code     *int
		lastErr  string
		next     time.Time
		dead     bool
	}
}

func (f *fakeStore) ClaimDue(context.Context, int, time.Duration) ([]domain.ClaimedDelivery, error) {
	return nil, nil
}

func (f *fakeStore) MarkDelivered(_ context.Context, _ uuid.UUID, id uuid.UUID, code int) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.delivered = append(f.delivered, struct {
		id   uuid.UUID
		code int
	}{id, code})
	return nil
}

func (f *fakeStore) MarkFailed(_ context.Context, _ uuid.UUID, id uuid.UUID, attempts int, code *int, lastErr string, next time.Time, dead bool) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.failed = append(f.failed, struct {
		id       uuid.UUID
		attempts int
		code     *int
		lastErr  string
		next     time.Time
		dead     bool
	}{id, attempts, code, lastErr, next, dead})
	return nil
}

func claimed(url, secret string) domain.ClaimedDelivery {
	return domain.ClaimedDelivery{
		ID:             uuid.New(),
		TenantID:       uuid.New(),
		SubscriptionID: uuid.New(),
		EventID:        uuid.New(),
		EventType:      "ledger.transaction.posted",
		Payload:        []byte(`{"id":"evt","type":"ledger.transaction.posted"}`),
		Attempts:       0,
		URL:            url,
		Secret:         secret,
	}
}

func TestSendSetsHeadersAndValidSignature(t *testing.T) {
	const secret = "whsec_8Zt5xVcbXkO2qWm1nJp3rTyUuIoPaSdF"
	var (
		gotBody []byte
		gotHdr  http.Header
	)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotHdr = r.Header.Clone()
		buf := make([]byte, r.ContentLength)
		_, _ = r.Body.Read(buf)
		gotBody = buf
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	c := claimed(srv.URL, secret)
	status, err := Send(context.Background(), srv.Client(), c)
	if err != nil {
		t.Fatalf("Send: %v", err)
	}
	if status != http.StatusOK {
		t.Fatalf("status = %d, want 200", status)
	}
	if got := gotHdr.Get("Content-Type"); got != "application/json" {
		t.Fatalf("Content-Type = %q", got)
	}
	if got := gotHdr.Get("X-LedgerCore-Event-Id"); got != c.EventID.String() {
		t.Fatalf("X-LedgerCore-Event-Id = %q, want %q", got, c.EventID)
	}
	if got := gotHdr.Get("X-LedgerCore-Event-Type"); got != c.EventType {
		t.Fatalf("X-LedgerCore-Event-Type = %q, want %q", got, c.EventType)
	}
	sig := gotHdr.Get(signature.HeaderName)
	if sig == "" {
		t.Fatal("missing signature header")
	}
	if err := signature.Verify(secret, sig, gotBody, signature.DefaultTolerance); err != nil {
		t.Fatalf("received signature does not verify against received body: %v", err)
	}
	// Verify must fail with the wrong secret.
	if err := signature.Verify("whsec_wrong", sig, gotBody, signature.DefaultTolerance); err == nil {
		t.Fatal("signature verified with the wrong secret")
	}
}

func TestProcessMarksDeliveredOn2xx(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusAccepted)
	}))
	defer srv.Close()

	store := &fakeStore{}
	d := New(store)
	d.client = srv.Client()

	c := claimed(srv.URL, "whsec_x")
	d.process(context.Background(), c)

	if len(store.delivered) != 1 || len(store.failed) != 0 {
		t.Fatalf("delivered=%d failed=%d, want 1/0", len(store.delivered), len(store.failed))
	}
	if store.delivered[0].code != http.StatusAccepted {
		t.Fatalf("recorded code = %d, want 202", store.delivered[0].code)
	}
}

func TestProcessSchedulesBackoffOnFailure(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	store := &fakeStore{}
	d := New(store)
	d.client = srv.Client()

	c := claimed(srv.URL, "whsec_x")
	c.Attempts = 1 // second attempt
	before := time.Now().UTC()
	d.process(context.Background(), c)

	if len(store.failed) != 1 {
		t.Fatalf("failed=%d, want 1", len(store.failed))
	}
	f := store.failed[0]
	if f.attempts != 2 {
		t.Fatalf("attempts = %d, want 2", f.attempts)
	}
	if f.dead {
		t.Fatal("attempt 2 of 5 must not be dead")
	}
	if f.code == nil || *f.code != http.StatusInternalServerError {
		t.Fatalf("last_status_code = %v, want 500", f.code)
	}
	// Backoff after the 2nd failed attempt is 5 minutes.
	wantNext := before.Add(domain.Backoff(2))
	if f.next.Before(wantNext.Add(-time.Minute)) || f.next.After(wantNext.Add(time.Minute)) {
		t.Fatalf("next_attempt_at = %v, want ~%v", f.next, wantNext)
	}
}

func TestProcessMarksDeadAfterMaxAttempts(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
	}))
	defer srv.Close()

	store := &fakeStore{}
	d := New(store)
	d.client = srv.Client()

	c := claimed(srv.URL, "whsec_x")
	c.Attempts = domain.MaxAttempts - 1 // this is the final attempt
	d.process(context.Background(), c)

	if len(store.failed) != 1 {
		t.Fatalf("failed=%d, want 1", len(store.failed))
	}
	if !store.failed[0].dead {
		t.Fatalf("attempt %d must tombstone the delivery", domain.MaxAttempts)
	}
}

func TestProcessRecordsTransportError(t *testing.T) {
	store := &fakeStore{}
	d := New(store)
	// Nothing listens here; connection is refused immediately.
	c := claimed("http://127.0.0.1:1", "whsec_x")
	d.process(context.Background(), c)

	if len(store.failed) != 1 {
		t.Fatalf("failed=%d, want 1", len(store.failed))
	}
	f := store.failed[0]
	if f.code != nil {
		t.Fatalf("transport errors must not record a status code, got %v", *f.code)
	}
	if f.lastErr == "" {
		t.Fatal("transport errors must record last_error")
	}
}
