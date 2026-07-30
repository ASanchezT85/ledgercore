package postgres

// LC-002/LC-014 anti-leak RLS contract test for the recon schema.
//
// All recon business tables carry FORCE ROW LEVEL SECURITY + tenant_isolation
// (USING + WITH CHECK) from 0001_init.sql. This test drives the real Store
// API as an intruder tenant and asserts it can neither read nor write another
// tenant's data (recon.outbox is intentionally system-level and excluded).
//
// Requires LEDGERCORE_TEST_DATABASE_URL and a NOBYPASSRLS role (skips
// otherwise).

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/ledgercore/ledgercore/libs/go/httpx"
	"github.com/ledgercore/ledgercore/libs/go/pgxutil"
	"github.com/ledgercore/ledgercore/services/reconciliation/internal/app"
	"github.com/ledgercore/ledgercore/services/reconciliation/internal/domain"
)

func TestReconRLSCrossTenantIsolation(t *testing.T) {
	url := os.Getenv("LEDGERCORE_TEST_DATABASE_URL")
	if url == "" {
		t.Skip("LEDGERCORE_TEST_DATABASE_URL not set; skipping integration test")
	}
	ctx := context.Background()

	if err := Migrate(ctx, url); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	pool, err := pgxutil.NewPool(ctx, url, "recon")
	if err != nil {
		t.Fatalf("pool: %v", err)
	}
	defer pool.Close()

	var bypass bool
	if err := pool.QueryRow(ctx,
		"SELECT rolsuper OR rolbypassrls FROM pg_roles WHERE rolname = current_user").Scan(&bypass); err != nil {
		t.Fatalf("check role: %v", err)
	}
	if bypass {
		t.Skip("connected role bypasses RLS; run as ledgercore_app (NOBYPASSRLS) to verify isolation")
	}

	store := NewStore(pool)
	tenantA := uuid.New()
	intruder := uuid.New()

	// Tenant A owns a source, an import, one external transaction, a run and a
	// discrepancy.
	src := domain.Source{ID: uuid.New(), TenantID: tenantA, Name: "rls-" + uuid.NewString(), Kind: domain.SourceBank, CreatedAt: time.Now().UTC()}
	imp := domain.Import{ID: uuid.New(), TenantID: tenantA, SourceID: src.ID, Filename: "s.csv", Status: domain.ImportProcessed, RowCount: 1, CreatedAt: time.Now().UTC()}
	ext := domain.ExternalTransaction{ID: uuid.New(), TenantID: tenantA, ImportID: imp.ID, SourceID: src.ID, ExternalRef: "r1", Amount: 100, Asset: "USD", Direction: domain.DirectionDebit, OccurredAt: time.Now().UTC(), MatchStatus: domain.MatchUnmatched}
	run := domain.Run{ID: uuid.New(), TenantID: tenantA, SourceID: src.ID, Status: domain.RunCompleted, StartedAt: time.Now().UTC()}
	disc := domain.Discrepancy{ID: uuid.New(), TenantID: tenantA, RunID: run.ID, Kind: domain.KindMissingInternal, Details: map[string]any{"x": "y"}, Status: domain.DiscrepancyOpen, CreatedAt: time.Now().UTC()}

	if err := store.Within(ctx, tenantA, func(tx app.TxStore) error {
		if err := tx.InsertSource(ctx, src); err != nil {
			return err
		}
		if err := tx.InsertImport(ctx, imp); err != nil {
			return err
		}
		if err := tx.InsertExternalTransactions(ctx, []domain.ExternalTransaction{ext}); err != nil {
			return err
		}
		if err := tx.InsertRun(ctx, run); err != nil {
			return err
		}
		return tx.InsertDiscrepancy(ctx, disc)
	}); err != nil {
		t.Fatalf("seed tenant A: %v", err)
	}

	t.Run("intruder reads return empty / not-found", func(t *testing.T) {
		if err := store.Within(ctx, intruder, func(tx app.TxStore) error {
			if _, err := tx.GetSource(ctx, src.ID); err != domain.ErrNotFound {
				t.Errorf("GetSource: want ErrNotFound, got %v", err)
			}
			if _, err := tx.GetRun(ctx, run.ID); err != domain.ErrNotFound {
				t.Errorf("GetRun: want ErrNotFound, got %v", err)
			}
			if _, err := tx.GetDiscrepancy(ctx, disc.ID); err != domain.ErrNotFound {
				t.Errorf("GetDiscrepancy: want ErrNotFound, got %v", err)
			}
			if got, err := tx.ListSources(ctx, 50, httpx.Cursor{}); err != nil || len(got) != 0 {
				t.Errorf("ListSources: want 0 rows nil err, got %d rows err=%v", len(got), err)
			}
			if got, err := tx.ListUnmatchedExternals(ctx, src.ID); err != nil || len(got) != 0 {
				t.Errorf("ListUnmatchedExternals: want 0 rows, got %d err=%v", len(got), err)
			}
			if got, err := tx.SourceSummaries(ctx); err != nil || len(got) != 0 {
				t.Errorf("SourceSummaries: want 0 rows, got %d err=%v", len(got), err)
			}
			return nil
		}); err != nil {
			t.Fatalf("intruder tx: %v", err)
		}
	})

	t.Run("intruder cannot write into tenant A space (WITH CHECK)", func(t *testing.T) {
		// Inserting a row stamped with tenant A's id while running under the
		// intruder context must be blocked by WITH CHECK.
		alien := domain.Source{ID: uuid.New(), TenantID: tenantA, Name: "alien-" + uuid.NewString(), Kind: domain.SourceBank, CreatedAt: time.Now().UTC()}
		err := store.Within(ctx, intruder, func(tx app.TxStore) error {
			return tx.InsertSource(ctx, alien)
		})
		if err == nil {
			_, _ = pool.Exec(ctx, "DELETE FROM sources WHERE id = $1", alien.ID)
			t.Fatal("intruder inserted a source for tenant A; WITH CHECK not enforced")
		}
	})

	t.Run("tenant A still sees its own data", func(t *testing.T) {
		if err := store.Within(ctx, tenantA, func(tx app.TxStore) error {
			if _, err := tx.GetSource(ctx, src.ID); err != nil {
				t.Errorf("tenant A lost its source: %v", err)
			}
			return nil
		}); err != nil {
			t.Fatalf("tenant A tx: %v", err)
		}
	})
}
