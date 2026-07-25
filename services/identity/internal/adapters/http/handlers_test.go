package httpadapter

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/ledgercore/ledgercore/services/identity/internal/app"
	"github.com/ledgercore/ledgercore/services/identity/internal/domain"
)

const testAdminToken = "test-admin-token"

// stubService is a hand-rolled IdentityService for handler-level tests.
type stubService struct {
	tenant domain.Tenant
	token  app.IssuedToken
	jwks   app.JWKSDocument
	err    error
}

func (s *stubService) CreateTenant(context.Context, string, string) (domain.Tenant, error) {
	return s.tenant, s.err
}
func (s *stubService) GetTenant(context.Context, uuid.UUID) (domain.Tenant, error) {
	return s.tenant, s.err
}
func (s *stubService) ListTenants(context.Context) ([]domain.Tenant, error) {
	return []domain.Tenant{s.tenant}, s.err
}
func (s *stubService) CreateAPIKey(context.Context, uuid.UUID, string, string) (domain.APIKey, string, error) {
	return domain.APIKey{}, "lk_sandbox_secret", s.err
}
func (s *stubService) RevokeAPIKey(context.Context, uuid.UUID) error { return s.err }
func (s *stubService) IssueToken(context.Context, string) (app.IssuedToken, error) {
	return s.token, s.err
}
func (s *stubService) JWKS(context.Context) (app.JWKSDocument, error) { return s.jwks, s.err }

type okPinger struct{}

func (okPinger) Ping(context.Context) error { return nil }

func newTestHandler(svc IdentityService) http.Handler {
	return New(svc, okPinger{}, testAdminToken).Handler()
}

func do(t *testing.T, h http.Handler, method, path, body string, headers map[string]string) *httptest.ResponseRecorder {
	t.Helper()
	var req *http.Request
	if body == "" {
		req = httptest.NewRequest(method, path, nil)
	} else {
		req = httptest.NewRequest(method, path, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func TestAdminEndpointsRequireToken(t *testing.T) {
	h := newTestHandler(&stubService{})

	// No token.
	rec := do(t, h, http.MethodPost, "/v1/tenants", `{"name":"A","slug":"a"}`, nil)
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("without token: status %d, want 401", rec.Code)
	}
	// Wrong token.
	rec = do(t, h, http.MethodGet, "/v1/tenants", "", map[string]string{"X-Admin-Token": "wrong"})
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("wrong token: status %d, want 401", rec.Code)
	}
	// Correct token.
	rec = do(t, h, http.MethodGet, "/v1/tenants", "", map[string]string{"X-Admin-Token": testAdminToken})
	if rec.Code != http.StatusOK {
		t.Errorf("correct token: status %d, want 200; body %s", rec.Code, rec.Body.String())
	}
}

func TestAdminDisabledWhenTokenUnset(t *testing.T) {
	h := New(&stubService{}, okPinger{}, "").Handler()
	rec := do(t, h, http.MethodGet, "/v1/tenants", "", map[string]string{"X-Admin-Token": ""})
	if rec.Code != http.StatusServiceUnavailable {
		t.Errorf("admin with empty configured token: status %d, want 503", rec.Code)
	}
}

func TestCreateTenantHandler(t *testing.T) {
	now := time.Now().UTC()
	stub := &stubService{tenant: domain.Tenant{
		ID: uuid.New(), Name: "Acme", Slug: "acme", Status: domain.TenantStatusActive, CreatedAt: now,
	}}
	h := newTestHandler(stub)

	rec := do(t, h, http.MethodPost, "/v1/tenants", `{"name":"Acme","slug":"acme"}`,
		map[string]string{"X-Admin-Token": testAdminToken})
	if rec.Code != http.StatusCreated {
		t.Fatalf("status %d, want 201; body %s", rec.Code, rec.Body.String())
	}
	var got map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got["slug"] != "acme" || got["status"] != "active" {
		t.Errorf("unexpected body: %v", got)
	}
}

func TestValidationErrorMapsTo400(t *testing.T) {
	stub := &stubService{err: app.ValidationError{Msg: "bad input"}}
	h := newTestHandler(stub)
	rec := do(t, h, http.MethodPost, "/v1/tenants", `{"name":"","slug":""}`,
		map[string]string{"X-Admin-Token": testAdminToken})
	if rec.Code != http.StatusBadRequest {
		t.Errorf("status %d, want 400", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "validation_error") {
		t.Errorf("body %s missing validation_error code", rec.Body.String())
	}
}

func TestIssueTokenHandler(t *testing.T) {
	stub := &stubService{token: app.IssuedToken{AccessToken: "jwt", TokenType: "Bearer", ExpiresIn: 900}}
	h := newTestHandler(stub)

	rec := do(t, h, http.MethodPost, "/v1/auth/token", `{"api_key":"lk_sandbox_x"}`, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d, want 200; body %s", rec.Code, rec.Body.String())
	}
	var got app.IssuedToken
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.ExpiresIn != 900 || got.TokenType != "Bearer" {
		t.Errorf("unexpected token response: %+v", got)
	}

	// Missing api_key -> 400.
	rec = do(t, h, http.MethodPost, "/v1/auth/token", `{"api_key":""}`, nil)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("empty api_key: status %d, want 400", rec.Code)
	}

	// Invalid credentials -> 401 with the platform error shape.
	stub.err = app.ErrInvalidCredentials
	rec = do(t, h, http.MethodPost, "/v1/auth/token", `{"api_key":"lk_sandbox_bad"}`, nil)
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("invalid credentials: status %d, want 401", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "invalid_credentials") {
		t.Errorf("body %s missing invalid_credentials code", rec.Body.String())
	}
}

func TestJWKSIsPublic(t *testing.T) {
	stub := &stubService{jwks: app.JWKSDocument{Keys: []app.JWK{{Kty: "OKP", Crv: "Ed25519", Kid: "k1", X: "x"}}}}
	h := newTestHandler(stub)
	rec := do(t, h, http.MethodGet, "/.well-known/jwks.json", "", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d, want 200", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), `"OKP"`) {
		t.Errorf("body %s missing OKP key", rec.Body.String())
	}
}

func TestHealthEndpoints(t *testing.T) {
	h := newTestHandler(&stubService{})
	if rec := do(t, h, http.MethodGet, "/healthz", "", nil); rec.Code != http.StatusOK {
		t.Errorf("healthz: status %d, want 200", rec.Code)
	}
	if rec := do(t, h, http.MethodGet, "/readyz", "", nil); rec.Code != http.StatusOK {
		t.Errorf("readyz: status %d, want 200", rec.Code)
	}
}

func TestRevokeAPIKeyHandler(t *testing.T) {
	stub := &stubService{}
	h := newTestHandler(stub)
	id := uuid.NewString()

	rec := do(t, h, http.MethodDelete, "/v1/api-keys/"+id, "", map[string]string{"X-Admin-Token": testAdminToken})
	if rec.Code != http.StatusNoContent {
		t.Errorf("revoke: status %d, want 204", rec.Code)
	}

	rec = do(t, h, http.MethodDelete, "/v1/api-keys/not-a-uuid", "", map[string]string{"X-Admin-Token": testAdminToken})
	if rec.Code != http.StatusBadRequest {
		t.Errorf("bad id: status %d, want 400", rec.Code)
	}

	stub.err = domain.ErrNotFound
	rec = do(t, h, http.MethodDelete, "/v1/api-keys/"+id, "", map[string]string{"X-Admin-Token": testAdminToken})
	if rec.Code != http.StatusNotFound {
		t.Errorf("missing key: status %d, want 404", rec.Code)
	}
}
