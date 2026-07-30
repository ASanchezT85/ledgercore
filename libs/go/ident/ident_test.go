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
			"iss":       DefaultIssuer,
			"aud":       DefaultAudiences(),
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
			"sub": "x", "iss": DefaultIssuer, "tenant_id": tenant.String(), "env": EnvSandbox,
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

	// LC-013: issuer, subject and clock-skew hardening on the default
	// middleware (RequireAuth enforces DefaultIssuer).
	reject := func(t *testing.T, mutate func(jwt.MapClaims)) {
		t.Helper()
		handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			t.Fatal("handler should not run")
		}))
		req := httptest.NewRequest(http.MethodGet, "/v1/accounts", nil)
		req.Header.Set("Authorization", "Bearer "+makeToken(mutate))
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("status = %d, want 401", rec.Code)
		}
	}

	t.Run("wrong issuer rejected", func(t *testing.T) {
		reject(t, func(c jwt.MapClaims) { c["iss"] = "someone-else" })
	})
	t.Run("missing issuer rejected", func(t *testing.T) {
		reject(t, func(c jwt.MapClaims) { delete(c, "iss") })
	})
	t.Run("empty subject rejected", func(t *testing.T) {
		reject(t, func(c jwt.MapClaims) { c["sub"] = "" })
	})
	t.Run("missing subject rejected", func(t *testing.T) {
		reject(t, func(c jwt.MapClaims) { delete(c, "sub") })
	})
	t.Run("clock skew beyond leeway rejected", func(t *testing.T) {
		// Expired well past the 60s default leeway.
		reject(t, func(c jwt.MapClaims) { c["exp"] = time.Now().Add(-5 * time.Minute).Unix() })
	})

	// LC-013: audience enforcement via RequireAuthConfig.
	t.Run("audience enforcement", func(t *testing.T) {
		mwAud := RequireAuthConfig(AuthConfig{
			JWKSURL:          jwksSrv.URL,
			ExpectedIssuer:   DefaultIssuer,
			ExpectedAudience: AudienceReconciliation,
		})
		pass := mwAud(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusOK) }))

		// Token carrying the reconciliation audience passes.
		req := httptest.NewRequest(http.MethodGet, "/v1/x", nil)
		req.Header.Set("Authorization", "Bearer "+makeToken(nil))
		rec := httptest.NewRecorder()
		pass.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200 for matching audience, got %d", rec.Code)
		}

		// Token minted for a different audience set is rejected.
		req = httptest.NewRequest(http.MethodGet, "/v1/x", nil)
		req.Header.Set("Authorization", "Bearer "+makeToken(func(c jwt.MapClaims) {
			c["aud"] = []string{"some-other-service"}
		}))
		rec = httptest.NewRecorder()
		pass.ServeHTTP(rec, req)
		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("expected 401 for wrong audience, got %d", rec.Code)
		}
	})
}

// TestRequireScope covers the deny-by-default authorization guard (LC-012).
func TestRequireScope(t *testing.T) {
	newReq := func(scopes []string) *http.Request {
		req := httptest.NewRequest(http.MethodGet, "/v1/x", nil)
		ctx := ContextWithClaims(req.Context(), Claims{
			Subject: "api_key:1", TenantID: uuid.New(), Environment: EnvSandbox, Scopes: scopes,
		})
		return req.WithContext(ctx)
	}
	guarded := func(scope string) http.Handler {
		return RequireScope(scope)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		}))
	}

	t.Run("token with the scope passes", func(t *testing.T) {
		rec := httptest.NewRecorder()
		guarded(ScopeReconWrite).ServeHTTP(rec, newReq([]string{ScopeReconRead, ScopeReconWrite}))
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200", rec.Code)
		}
	})

	t.Run("token without the scope is forbidden", func(t *testing.T) {
		rec := httptest.NewRecorder()
		guarded(ScopeReconWrite).ServeHTTP(rec, newReq([]string{ScopeReconRead}))
		if rec.Code != http.StatusForbidden {
			t.Fatalf("status = %d, want 403", rec.Code)
		}
	})

	t.Run("wildcard satisfies any scope", func(t *testing.T) {
		rec := httptest.NewRecorder()
		guarded(ScopeReconWrite).ServeHTTP(rec, newReq([]string{ScopeWildcard}))
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200", rec.Code)
		}
	})

	t.Run("no claims in context is unauthorized", func(t *testing.T) {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/v1/x", nil)
		guarded(ScopeReconWrite).ServeHTTP(rec, req)
		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("status = %d, want 401", rec.Code)
		}
	})
}
