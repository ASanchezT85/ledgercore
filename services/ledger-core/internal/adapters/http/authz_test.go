package httpapi

// R-006 / R-007 authorization tests for the ledger-core HTTP surface.
//
// These exercise the real middleware stack (ident.RequireAuthConfig with the
// ledger-core audience + ident.RequireScope) end to end against a locally
// served JWKS, with a no-op app.Store so no database is required. They prove:
//   - a token without ledger scopes is 403 on read routes (deny by default),
//   - a read-only token is 403 on write routes,
//   - a token minted for a different audience is 401,
//   - a correctly scoped token clears auth (neither 401 nor 403),
//   - no token is 401.

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"

	"github.com/ASanchezT85/ledgercore/libs/go/ident"
	"github.com/ASanchezT85/ledgercore/libs/go/money"

	"github.com/ASanchezT85/ledgercore/services/ledger-core/internal/app"
	"github.com/ASanchezT85/ledgercore/services/ledger-core/internal/domain"
)

// fakeStore satisfies app.Store with zero-value/no-op behavior. The negative
// authz cases (401/403) are rejected by the middleware before any handler
// runs, so the store is never touched there; the positive case only needs the
// call not to be 401/403.
type fakeStore struct{}

func (fakeStore) CreateLedger(context.Context, domain.Ledger) error { return nil }
func (fakeStore) GetLedger(context.Context, uuid.UUID, uuid.UUID) (domain.Ledger, error) {
	return domain.Ledger{}, nil
}
func (fakeStore) ListLedgers(context.Context, uuid.UUID, app.Page) ([]domain.Ledger, error) {
	return nil, nil
}
func (fakeStore) CreateAccount(context.Context, domain.Account) error { return nil }
func (fakeStore) GetAccount(context.Context, uuid.UUID, uuid.UUID) (domain.Account, error) {
	return domain.Account{}, nil
}
func (fakeStore) ListAccounts(context.Context, uuid.UUID, app.AccountFilter, app.Page) ([]domain.Account, error) {
	return nil, nil
}
func (fakeStore) GetBalances(context.Context, uuid.UUID, uuid.UUID) (domain.Account, []domain.Balance, error) {
	return domain.Account{}, nil, nil
}
func (fakeStore) ListEntries(context.Context, uuid.UUID, uuid.UUID, string, app.Page) ([]domain.Entry, error) {
	return nil, nil
}
func (fakeStore) CreateTransaction(context.Context, domain.Transaction) (domain.Transaction, bool, error) {
	return domain.Transaction{}, false, nil
}
func (fakeStore) GetTransaction(context.Context, uuid.UUID, uuid.UUID) (domain.Transaction, error) {
	return domain.Transaction{}, nil
}
func (fakeStore) ListTransactions(context.Context, uuid.UUID, app.TransactionFilter, app.Page) ([]domain.Transaction, error) {
	return nil, nil
}
func (fakeStore) PostTransaction(context.Context, uuid.UUID, uuid.UUID, time.Time) (domain.Transaction, error) {
	return domain.Transaction{}, nil
}
func (fakeStore) ReverseTransaction(context.Context, uuid.UUID, uuid.UUID, string, string, time.Time) (domain.Transaction, bool, error) {
	return domain.Transaction{}, false, nil
}
func (fakeStore) CreateHold(context.Context, domain.Hold) (domain.Hold, bool, error) {
	return domain.Hold{}, false, nil
}
func (fakeStore) CaptureHold(context.Context, uuid.UUID, uuid.UUID, *money.Amount, *uuid.UUID, time.Time) (domain.Hold, error) {
	return domain.Hold{}, nil
}
func (fakeStore) ReleaseHold(context.Context, uuid.UUID, uuid.UUID, time.Time) (domain.Hold, error) {
	return domain.Hold{}, nil
}
func (fakeStore) GetHold(context.Context, uuid.UUID, uuid.UUID) (domain.Hold, error) {
	return domain.Hold{}, nil
}
func (fakeStore) TrialBalance(context.Context, uuid.UUID, uuid.UUID, *time.Time) (domain.TrialBalance, error) {
	return domain.TrialBalance{}, nil
}
func (fakeStore) Statement(context.Context, uuid.UUID, uuid.UUID, time.Time, time.Time, app.Page) (domain.Statement, error) {
	return domain.Statement{}, nil
}
func (fakeStore) ProviderAccountBalances(context.Context, uuid.UUID) ([]domain.ProviderAccountBalance, error) {
	return nil, nil
}
func (fakeStore) VerifyBalances(context.Context, uuid.UUID, uuid.UUID) ([]domain.BalanceDiscrepancy, error) {
	return nil, nil
}

func newAuthzHarness(t *testing.T) (http.Handler, func(scopes []string, aud []string) string) {
	t.Helper()
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	const kid = "ledger-authz-key"
	jwks := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"keys": []map[string]string{{
				"kty": "OKP", "crv": "Ed25519", "kid": kid,
				"x": base64.RawURLEncoding.EncodeToString(pub),
			}},
		})
	}))
	t.Cleanup(jwks.Close)

	svc := app.NewService(fakeStore{})
	dbPing := func(context.Context) error { return nil }
	handler := NewRouter(svc, dbPing, jwks.URL, false)

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

func TestLedgerAuthorization(t *testing.T) {
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
		tok := mint([]string{ident.ScopeLedgerRead}, goodAud)
		if code := call(http.MethodPost, "/v1/ledgers", tok); code != http.StatusForbidden {
			t.Fatalf("status = %d, want 403", code)
		}
	})

	t.Run("read route without ledger scopes is 403", func(t *testing.T) {
		tok := mint([]string{ident.ScopeReconRead}, goodAud)
		if code := call(http.MethodGet, "/v1/ledgers", tok); code != http.StatusForbidden {
			t.Fatalf("status = %d, want 403", code)
		}
	})

	t.Run("wrong audience is 401", func(t *testing.T) {
		tok := mint([]string{ident.ScopeLedgerRead}, []string{ident.AudienceReconciliation})
		if code := call(http.MethodGet, "/v1/ledgers", tok); code != http.StatusUnauthorized {
			t.Fatalf("status = %d, want 401", code)
		}
	})

	t.Run("correctly scoped read token clears auth", func(t *testing.T) {
		tok := mint([]string{ident.ScopeLedgerRead}, goodAud)
		code := call(http.MethodGet, "/v1/ledgers", tok)
		if code == http.StatusUnauthorized || code == http.StatusForbidden {
			t.Fatalf("correctly scoped token rejected: status = %d", code)
		}
	})

	t.Run("no token is 401", func(t *testing.T) {
		if code := call(http.MethodGet, "/v1/ledgers", ""); code != http.StatusUnauthorized {
			t.Fatalf("status = %d, want 401", code)
		}
	})
}
