package postgres

import (
	"context"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/ASanchezT85/ledgercore/libs/go/events"
	"github.com/ASanchezT85/ledgercore/libs/go/pgxutil"

	"github.com/ASanchezT85/ledgercore/services/reconciliation/internal/app"
	"github.com/ASanchezT85/ledgercore/services/reconciliation/internal/domain"
)

// capturePublisher records published envelopes (implements events.Publisher).
type capturePublisher struct {
	mu   sync.Mutex
	envs []events.Envelope
}

func (p *capturePublisher) Publish(env events.Envelope) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.envs = append(p.envs, env)
	return nil
}

// TestPostgresIntegration exercises migrations, RLS isolation, the mirror
// idempotency and the outbox drain against a real database. It is skipped
// unless LEDGERCORE_TEST_DATABASE_URL is set.
func TestPostgresIntegration(t *testing.T) {
	url := os.Getenv("LEDGERCORE_TEST_DATABASE_URL")
	if url == "" {
		t.Skip("LEDGERCORE_TEST_DATABASE_URL not set; skipping integration test")
	}
	ctx := context.Background()

	if !roleModelProvisioned {
		if err := Migrate(ctx, url); err != nil {
			t.Fatalf("migrate: %v", err)
		}
	}

	pool, err := pgxutil.NewPool(ctx, url, "recon")
	if err != nil {
		t.Fatalf("pool: %v", err)
	}
	defer pool.Close()

	store := NewStore(pool)
	tenantA := uuid.New()
	tenantB := uuid.New()

	src := domain.Source{
		ID: uuid.New(), TenantID: tenantA, Name: "it-" + uuid.NewString(),
		Kind: domain.SourceBank, CreatedAt: time.Now().UTC(),
	}
	if err := store.Within(ctx, tenantA, func(tx app.TxStore) error {
		return tx.InsertSource(ctx, src)
	}); err != nil {
		t.Fatalf("insert source: %v", err)
	}

	t.Run("rls isolation", func(t *testing.T) {
		var bypass bool
		if err := pool.QueryRow(ctx,
			"SELECT rolsuper OR rolbypassrls FROM pg_roles WHERE rolname = current_user").
			Scan(&bypass); err != nil {
			t.Fatalf("check role: %v", err)
		}
		if bypass {
			t.Skip("connected role bypasses RLS (superuser); run as ledgercore_app to verify isolation")
		}
		if err := store.Within(ctx, tenantB, func(tx app.TxStore) error {
			_, err := tx.GetSource(ctx, src.ID)
			if err == nil {
				t.Error("tenant B must not see tenant A sources")
			}
			return nil
		}); err != nil {
			t.Fatalf("tenant B tx: %v", err)
		}
	})

	t.Run("mirror insert is idempotent", func(t *testing.T) {
		entry := domain.MirrorEntry{
			ID: uuid.New(), TenantID: tenantA, TransactionID: uuid.New(),
			AccountID: uuid.New(), Direction: "DEBIT", Amount: 1050, Asset: "USD",
			Reference: "it-ref-" + uuid.NewString(), PostedAt: time.Now().UTC(),
		}
		for i := 0; i < 2; i++ {
			if err := store.InsertMirrorEntries(ctx, tenantA, []domain.MirrorEntry{entry}); err != nil {
				t.Fatalf("insert mirror (attempt %d): %v", i+1, err)
			}
		}
		var count int
		if err := store.Within(ctx, tenantA, func(tx app.TxStore) error {
			entries, err := tx.ListMirrorEntries(ctx)
			if err != nil {
				return err
			}
			for _, e := range entries {
				if e.ID == entry.ID {
					count++
				}
			}
			return nil
		}); err != nil {
			t.Fatalf("list mirror: %v", err)
		}
		if count != 1 {
			t.Fatalf("expected exactly 1 mirror row after double insert, got %d", count)
		}
	})

	t.Run("outbox drain publishes and marks", func(t *testing.T) {
		env, err := events.NewEnvelope(events.TopicDiscrepancyDetected, tenantA, map[string]string{"probe": "it"})
		if err != nil {
			t.Fatalf("envelope: %v", err)
		}
		if err := store.Within(ctx, tenantA, func(tx app.TxStore) error {
			return tx.InsertOutboxEnvelope(ctx, env)
		}); err != nil {
			t.Fatalf("insert outbox: %v", err)
		}

		pub := &capturePublisher{}
		if _, err := store.DrainOutbox(ctx, pub, 1000); err != nil {
			t.Fatalf("drain: %v", err)
		}
		found := false
		for _, e := range pub.envs {
			if e.ID == env.ID {
				found = true
			}
		}
		if !found {
			t.Fatal("drained envelope was not published")
		}

		// A second drain must not republish it.
		pub2 := &capturePublisher{}
		if _, err := store.DrainOutbox(ctx, pub2, 1000); err != nil {
			t.Fatalf("second drain: %v", err)
		}
		for _, e := range pub2.envs {
			if e.ID == env.ID {
				t.Fatal("envelope published twice: published_at mark did not stick")
			}
		}
	})
}
