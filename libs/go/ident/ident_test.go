package ident

import (
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
)

func echoTenantHandler(t *testing.T) http.Handler {
	t.Helper()
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		claims, ok := ClaimsFromContext(r.Context())
		if !ok {
			t.Fatal("claims missing from context")
		}
		tenant, ok := TenantFromContext(r.Context())
		if !ok || tenant != claims.TenantID {
			t.Fatal("TenantFromContext disagrees with claims")
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(claims.TenantID.String()))
	})
}

func TestRequireAuthDisabledMode(t *testing.T) {
	mw := RequireAuth("http://unused.invalid/jwks.json", true)
	handler := mw(echoTenantHandler(t))

	tenant := uuid.New()
	req := httptest.NewRequest(http.MethodGet, "/v1/accounts", nil)
	req.Header.Set("X-Tenant-Id", tenant.String())
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", rec.Code, rec.Body.String())
	}
	if rec.Body.String() != tenant.String() {
		t.Fatalf("tenant = %q, want %q", rec.Body.String(), tenant)
	}
}

func TestRequireAuthDisabledModeRejectsBadHeader(t *testing.T) {
	mw := RequireAuth("http://unused.invalid/jwks.json", true)
	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("handler should not run")
	}))

	for name, header := range map[string]string{"missing": "", "not a uuid": "nope"} {
		t.Run(name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/v1/accounts", nil)
			if header != "" {
				req.Header.Set("X-Tenant-Id", header)
			}
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)
			if rec.Code != http.StatusUnauthorized {
				t.Fatalf("status = %d, want 401", rec.Code)
			}
			var body struct {
				Error struct {
					Code    string `json:"code"`
					Message string `json:"message"`
				} `json:"error"`
			}
			if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
				t.Fatalf("error body is not JSON: %v", err)
			}
			if body.Error.Code == "" {
				t.Fatal("error code missing")
			}
		})
	}
}

func TestRequireAuthRejectsMissingToken(t *testing.T) {
	mw := RequireAuth("http://unused.invalid/jwks.json", false)
	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("handler should not run")
	}))

	req := httptest.NewRequest(http.MethodGet, "/v1/accounts", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rec.Code)
	}
}

func TestRequireAuthValidatesEdDSAAgainstJWKS(t *testing.T) {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	const kid = "test-key-1"

	jwksSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"keys": []map[string]string{{
				"kty": "OKP",
				"crv": "Ed25519",
				"kid": kid,
				"x":   base64.RawURLEncoding.EncodeToString(pub),
			}},
		})
	}))
	defer jwksSrv.Close()

	tenant := uuid.New()
	makeToken := func(mutate func(c jwt.MapClaims)) string {
		claims := jwt.MapClaims{
			"sub":       "api-key-123",
			"tenant_id": tenant.String(),
			"env":       EnvSandbox,
			"scopes":    []string{"ledger:read", "ledger:write"},
			"exp":       time.Now().Add(time.Hour).Unix(),
			"iat":       time.Now().Unix(),
		}
		if mutate != nil {
			mutate(claims)
		}
		tok := jwt.NewWithClaims(jwt.SigningMethodEdDSA, claims)
		tok.Header["kid"] = kid
		signed, err := tok.SignedString(priv)
		if err != nil {
			t.Fatal(err)
		}
		return signed
	}

	mw := RequireAuth(jwksSrv.URL, false)

	t.Run("valid token passes and injects claims", func(t *testing.T) {
		var got Claims
		handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			got, _ = ClaimsFromContext(r.Context())
			w.WriteHeader(http.StatusOK)
		}))
		req := httptest.NewRequest(http.MethodGet, "/v1/accounts", nil)
		req.Header.Set("Authorization", "Bearer "+makeToken(nil))
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200; body: %s", rec.Code, rec.Body.String())
		}
		if got.TenantID != tenant || got.Subject != "api-key-123" || got.Environment != EnvSandbox {
			t.Fatalf("unexpected claims: %+v", got)
		}
		if !got.HasScope("ledger:write") || got.HasScope("admin") {
			t.Fatalf("unexpected scopes: %+v", got.Scopes)
		}
	})

	t.Run("expired token rejected", func(t *testing.T) {
		handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			t.Fatal("handler should not run")
		}))
		req := httptest.NewRequest(http.MethodGet, "/v1/accounts", nil)
		req.Header.Set("Authorization", "Bearer "+makeToken(func(c jwt.MapClaims) {
			c["exp"] = time.Now().Add(-time.Hour).Unix()
		}))
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("status = %d, want 401", rec.Code)
		}
	})

	t.Run("token signed by another key rejected", func(t *testing.T) {
		_, otherPriv, err := ed25519.GenerateKey(rand.Reader)
		if err != nil {
			t.Fatal(err)
		}
		claims := jwt.MapClaims{
			"sub": "x", "tenant_id": tenant.String(), "env": EnvSandbox,
			"exp": time.Now().Add(time.Hour).Unix(),
		}
		tok := jwt.NewWithClaims(jwt.SigningMethodEdDSA, claims)
		tok.Header["kid"] = kid
		signed, err := tok.SignedString(otherPriv)
		if err != nil {
			t.Fatal(err)
		}
		handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			t.Fatal("handler should not run")
		}))
		req := httptest.NewRequest(http.MethodGet, "/v1/accounts", nil)
		req.Header.Set("Authorization", "Bearer "+signed)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("status = %d, want 401", rec.Code)
		}
	})
}
