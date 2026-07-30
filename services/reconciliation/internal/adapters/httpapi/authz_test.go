package httpapi

// LC-012 / LC-013 authorization tests for the reconciliation HTTP surface.
//
// These exercise the real middleware stack (ident.RequireAuthConfig +
// ident.RequireScope) end to end against a locally served JWKS, with a fake
// app.Store so no database is required. They prove:
//   - a token missing reconciliation:write is 403 on write routes,
//   - a token with no reconciliation scopes is 403 on read routes,
//   - a token minted for a different audience is 401,
//   - a correctly scoped token clears auth (not 401/403).

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"

	"github.com/ledgercore/ledgercore/libs/go/events"
	"github.com/ledgercore/ledgercore/libs/go/httpx"
	"github.com/ledgercore/ledgercore/libs/go/ident"
	"github.com/ledgercore/ledgercore/services/reconciliation/internal/app"
	"github.com/ledgercore/ledgercore/services/reconciliation/internal/domain"
)

// fakeTxStore satisfies app.TxStore with empty/no-op behavior.
type fakeTxStore struct{}

func (fakeTxStore) InsertSource(context.Context, domain.Source) error { return nil }
func (fakeTxStore) ListSources(context.Context, int, httpx.Cursor) ([]domain.Source, error) {
	return nil, nil
}
func (fakeTxStore) GetSource(context.Context, uuid.UUID) (domain.Source, error) {
	return domain.Source{}, domain.ErrNotFound
}
func (fakeTxStore) InsertImport(context.Context, domain.Import) error { return nil }
func (fakeTxStore) InsertExternalTransactions(context.Context, []domain.ExternalTransaction) error {
	return nil
}
func (fakeTxStore) ListUnmatchedExternals(context.Context, uuid.UUID) ([]domain.ExternalTransaction, error) {
	return nil, nil
}
func (fakeTxStore) ListMirrorEntries(context.Context) ([]domain.MirrorEntry, error) { return nil, nil }
func (fakeTxStore) MarkExternalMatched(context.Context, uuid.UUID, uuid.UUID) error { return nil }
func (fakeTxStore) MarkExternalDisputed(context.Context, uuid.UUID) error           { return nil }
func (fakeTxStore) InsertRun(context.Context, domain.Run) error                     { return nil }
func (fakeTxStore) UpdateRun(context.Context, domain.Run) error                     { return nil }
func (fakeTxStore) GetRun(context.Context, uuid.UUID) (domain.Run, error) {
	return domain.Run{}, domain.ErrNotFound
}
func (fakeTxStore) InsertDiscrepancy(context.Context, domain.Discrepancy) error { return nil }
func (fakeTxStore) ListDiscrepancies(context.Context, domain.DiscrepancyStatus, int, httpx.Cursor) ([]domain.Discrepancy, error) {
	return nil, nil
}
func (fakeTxStore) GetDiscrepancy(context.Context, uuid.UUID) (domain.Discrepancy, error) {
	return domain.Discrepancy{}, domain.ErrNotFound
}
func (fakeTxStore) UpdateDiscrepancy(context.Context, domain.Discrepancy) error { return nil }
func (fakeTxStore) SourceSummaries(context.Context) ([]domain.SourceSummary, error) {
	return nil, nil
}
func (fakeTxStore) InsertOutboxEnvelope(context.Context, events.Envelope) error { return nil }

type fakeStore struct{}

func (fakeStore) Within(ctx context.Context, _ uuid.UUID, fn func(app.TxStore) error) error {
	return fn(fakeTxStore{})
}

func newAuthzHarness(t *testing.T) (http.Handler, func(scopes []string, aud []string) string) {
	t.Helper()
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	const kid = "recon-authz-key"
	jwks := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"keys": []map[string]string{{
				"kty": "OKP", "crv": "Ed25519", "kid": kid,
				"x": base64.RawURLEncoding.EncodeToString(pub),
			}},
		})
	})).URL
	t.Cleanup(func() {})

	svc := app.NewService(fakeStore{}, slog.Default())
	handler := NewHandler(svc, nil, jwks, false)

	mint := func(scopes []string, aud []string) string {
		claims := jwt.MapClaims{
			"sub":       "api_key:" + uuid.NewString(),
			"iss":       ident.DefaultIssuer,
			"aud":       aud,
			"tenant_id": uuid.NewString(),
			"env":       ident.EnvSandbox,
			"scopes":    scopes,
			"exp":       time.Now().Add(time.Hour).Unix(),
			"iat":       time.Now().Unix(),
		}
		tok := jwt.NewWithClaims(jwt.SigningMethodEdDSA, claims)
		tok.Header["kid"] = kid
		signed, err := tok.SignedString(priv)
		if err != nil {
			t.Fatal(err)
		}
		return signed
	}
	return handler, mint
}

func TestReconAuthorization(t *testing.T) {
	handler, mint := newAuthzHarness(t)
	goodAud := ident.DefaultAudiences()

	call := func(method, path, token string) int {
		req := httptest.NewRequest(method, path, nil)
		if token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		return rec.Code
	}

	t.Run("write route with read-only token is 403", func(t *testing.T) {
		tok := mint([]string{ident.ScopeReconRead}, goodAud)
		if code := call(http.MethodPost, "/v1/reconciliation/runs", tok); code != http.StatusForbidden {
			t.Fatalf("status = %d, want 403", code)
		}
	})

	t.Run("read route without recon scopes is 403", func(t *testing.T) {
		tok := mint([]string{ident.ScopeLedgerRead}, goodAud)
		if code := call(http.MethodGet, "/v1/reconciliation/sources", tok); code != http.StatusForbidden {
			t.Fatalf("status = %d, want 403", code)
		}
	})

	t.Run("wrong audience is 401", func(t *testing.T) {
		tok := mint([]string{ident.ScopeReconRead}, []string{"some-other-service"})
		if code := call(http.MethodGet, "/v1/reconciliation/sources", tok); code != http.StatusUnauthorized {
			t.Fatalf("status = %d, want 401", code)
		}
	})

	t.Run("correctly scoped read token clears auth", func(t *testing.T) {
		tok := mint([]string{ident.ScopeReconRead}, goodAud)
		// GET /sources reaches the handler (fake store -> 200). The point is
		// that it is neither 401 nor 403.
		code := call(http.MethodGet, "/v1/reconciliation/sources", tok)
		if code == http.StatusUnauthorized || code == http.StatusForbidden {
			t.Fatalf("correctly scoped token rejected: status = %d", code)
		}
	})

	t.Run("no token is 401", func(t *testing.T) {
		if code := call(http.MethodGet, "/v1/reconciliation/sources", ""); code != http.StatusUnauthorized {
			t.Fatalf("status = %d, want 401", code)
		}
	})
}
