package httpapi

// R-006 / R-007 authorization + audience tests for the webhooks HTTP surface.
//
// These exercise the real middleware stack (ident.RequireAuthConfig +
// ident.RequireScope) end to end against a locally served JWKS, with fake
// stores so no database is required. They prove:
//   - a token missing webhooks:write is 403 on write routes,
//   - a token with no webhooks scopes is 403 on read routes,
//   - a token minted for a different audience is 401,
//   - a correctly scoped token clears auth (not 401/403),
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

	"github.com/ledgercore/ledgercore/libs/go/httpx"
	"github.com/ledgercore/ledgercore/libs/go/ident"
	"github.com/ledgercore/ledgercore/services/webhooks/internal/app"
	"github.com/ledgercore/ledgercore/services/webhooks/internal/config"
	"github.com/ledgercore/ledgercore/services/webhooks/internal/domain"
)

// fakeSubs satisfies app.SubscriptionStore with empty/no-op behavior.
type fakeSubs struct{}

func (fakeSubs) Create(context.Context, domain.Subscription) error { return nil }
func (fakeSubs) List(context.Context, uuid.UUID, int, httpx.Cursor) ([]domain.Subscription, error) {
	return nil, nil
}
func (fakeSubs) Get(context.Context, uuid.UUID, uuid.UUID) (domain.Subscription, error) {
	return domain.Subscription{}, domain.ErrNotFound
}
func (fakeSubs) Update(context.Context, uuid.UUID, uuid.UUID, *string, []string, *bool) (domain.Subscription, error) {
	return domain.Subscription{}, domain.ErrNotFound
}
func (fakeSubs) RotateSecret(context.Context, uuid.UUID, uuid.UUID, string, time.Time) error {
	return nil
}
func (fakeSubs) ListActive(context.Context, uuid.UUID) ([]domain.Subscription, error) {
	return nil, nil
}

// fakeDels satisfies app.DeliveryStore with empty/no-op behavior.
type fakeDels struct{}

func (fakeDels) InsertPending(context.Context, uuid.UUID, []domain.Delivery) error { return nil }
func (fakeDels) ListDeliveries(context.Context, uuid.UUID, app.DeliveryFilter) ([]domain.Delivery, string, error) {
	return nil, "", nil
}
func (fakeDels) Requeue(context.Context, uuid.UUID, uuid.UUID) (domain.Delivery, error) {
	return domain.Delivery{}, domain.ErrNotFound
}

func newAuthzHarness(t *testing.T) (http.Handler, func(scopes []string, aud []string) string) {
	t.Helper()
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	const kid = "webhooks-authz-key"
	jwks := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"keys": []map[string]string{{
				"kty": "OKP", "crv": "Ed25519", "kid": kid,
				"x": base64.RawURLEncoding.EncodeToString(pub),
			}},
		})
	})).URL

	svc := app.NewService(fakeSubs{}, fakeDels{})
	handler := NewHandler(svc, nil, config.Config{JWKSURL: jwks})

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

func TestWebhooksAuthorization(t *testing.T) {
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
		tok := mint([]string{ident.ScopeWebhooksRead}, goodAud)
		if code := call(http.MethodPost, "/v1/webhook-subscriptions", tok); code != http.StatusForbidden {
			t.Fatalf("status = %d, want 403", code)
		}
	})

	t.Run("read route without webhooks scopes is 403", func(t *testing.T) {
		tok := mint([]string{ident.ScopeLedgerRead}, goodAud)
		if code := call(http.MethodGet, "/v1/webhook-subscriptions", tok); code != http.StatusForbidden {
			t.Fatalf("status = %d, want 403", code)
		}
	})

	t.Run("wrong audience is 401", func(t *testing.T) {
		tok := mint([]string{ident.ScopeWebhooksRead}, []string{"some-other-service"})
		if code := call(http.MethodGet, "/v1/webhook-subscriptions", tok); code != http.StatusUnauthorized {
			t.Fatalf("status = %d, want 401", code)
		}
	})

	t.Run("correctly scoped read token clears auth", func(t *testing.T) {
		tok := mint([]string{ident.ScopeWebhooksRead}, goodAud)
		code := call(http.MethodGet, "/v1/webhook-subscriptions", tok)
		if code == http.StatusUnauthorized || code == http.StatusForbidden {
			t.Fatalf("correctly scoped token rejected: status = %d", code)
		}
	})

	t.Run("no token is 401", func(t *testing.T) {
		if code := call(http.MethodGet, "/v1/webhook-subscriptions", ""); code != http.StatusUnauthorized {
			t.Fatalf("status = %d, want 401", code)
		}
	})
}
