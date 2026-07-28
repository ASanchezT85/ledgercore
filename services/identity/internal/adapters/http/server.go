// Package httpadapter exposes the identity use cases over HTTP using the
// stdlib ServeMux with method patterns and the shared libs/go/httpx
// middlewares and JSON helpers.
package httpadapter

import (
	"context"
	"crypto/subtle"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/ledgercore/ledgercore/libs/go/httpx"
	"github.com/ledgercore/ledgercore/services/identity/internal/app"
	"github.com/ledgercore/ledgercore/services/identity/internal/domain"
)

// IdentityService is the port this adapter drives; *app.Service implements it.
type IdentityService interface {
	CreateTenant(ctx context.Context, name, slug string) (domain.Tenant, error)
	GetTenant(ctx context.Context, id uuid.UUID) (domain.Tenant, error)
	ListTenants(ctx context.Context) ([]domain.Tenant, error)
	CreateAPIKey(ctx context.Context, tenantID uuid.UUID, environment, name string) (domain.APIKey, string, error)
	RevokeAPIKey(ctx context.Context, id uuid.UUID) error
	IssueToken(ctx context.Context, secret string) (app.IssuedToken, error)
	JWKS(ctx context.Context) (app.JWKSDocument, error)
}

// Pinger reports database reachability for /readyz; *pgxpool.Pool satisfies it.
type Pinger interface {
	Ping(ctx context.Context) error
}

// SandboxService is the self-service signup port; *app.SandboxService
// implements it. nil disables the public signup endpoint.
type SandboxService interface {
	Signup(ctx context.Context, email, companyName string) (app.SandboxSignup, error)
}

// Server owns the HTTP wiring of the identity service.
type Server struct {
	svc        IdentityService
	sandbox    SandboxService
	db         Pinger
	adminToken string
}

// New builds the HTTP adapter. adminToken is the shared secret required by
// the bootstrap admin endpoints (X-Admin-Token); empty disables them.
// sandbox may be nil to disable the public signup endpoint.
func New(svc IdentityService, db Pinger, adminToken string) *Server {
	return &Server{svc: svc, db: db, adminToken: adminToken}
}

// WithSandbox enables the public self-service signup endpoint.
func (s *Server) WithSandbox(sandbox SandboxService) *Server {
	s.sandbox = sandbox
	return s
}

// Handler returns the fully wired handler: routes + shared middlewares.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()

	// Public endpoints.
	mux.HandleFunc("GET /healthz", s.handleHealthz)
	mux.HandleFunc("GET /readyz", s.handleReadyz)
	mux.HandleFunc("POST /v1/auth/token", s.handleIssueToken)
	mux.HandleFunc("GET /.well-known/jwks.json", s.handleJWKS)
	if s.sandbox != nil {
		// Public self-service sandbox signup. Abuse controls: per-IP rate
		// limit at the gateway (traefik) plus one-signup-per-email and a
		// global daily cap enforced transactionally by the service.
		mux.HandleFunc("POST /v1/sandbox/signups", s.handleSandboxSignup)
	}

	// Admin (bootstrap) endpoints, gated by X-Admin-Token.
	mux.Handle("POST /v1/tenants", s.admin(s.handleCreateTenant))
	mux.Handle("GET /v1/tenants", s.admin(s.handleListTenants))
	mux.Handle("GET /v1/tenants/{id}", s.admin(s.handleGetTenant))
	mux.Handle("POST /v1/api-keys", s.admin(s.handleCreateAPIKey))
	mux.Handle("DELETE /v1/api-keys/{id}", s.admin(s.handleRevokeAPIKey))

	var h http.Handler = mux
	// Browser clients (the console) call token issuance cross-origin; the
	// shared dev CORS middleware answers preflights and mirrors the origin,
	// matching ledger-core and reconciliation.
	h = httpx.CORSDev(h)
	h = httpx.Logger(h)
	h = httpx.RequestID(h)
	h = httpx.Recover(h)
	return h
}

// admin enforces the X-Admin-Token dev-bootstrap gate with a constant-time
// comparison. If no token is configured, admin endpoints are disabled
// (fail closed) rather than open.
func (s *Server) admin(next http.HandlerFunc) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if s.adminToken == "" {
			httpx.WriteError(w, http.StatusServiceUnavailable, "admin_disabled", "LEDGERCORE_ADMIN_TOKEN is not configured")
			return
		}
		// Accept both the X-Admin-Token header and the documented
		// "Authorization: Bearer <token>" form (see infra/README.md).
		got := r.Header.Get("X-Admin-Token")
		if got == "" {
			if auth := r.Header.Get("Authorization"); strings.HasPrefix(auth, "Bearer ") {
				got = strings.TrimPrefix(auth, "Bearer ")
			}
		}
		if subtle.ConstantTimeCompare([]byte(got), []byte(s.adminToken)) != 1 {
			httpx.WriteError(w, http.StatusUnauthorized, "invalid_admin_token", "admin token is missing or incorrect (X-Admin-Token or Authorization: Bearer)")
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (s *Server) handleHealthz(w http.ResponseWriter, _ *http.Request) {
	httpx.WriteJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) handleReadyz(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()
	if err := s.db.Ping(ctx); err != nil {
		httpx.WriteError(w, http.StatusServiceUnavailable, "db_unavailable", "database ping failed")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]string{"status": "ready"})
}
