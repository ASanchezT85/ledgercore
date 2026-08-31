// Package httpapi exposes the webhooks management REST API plus the health
// endpoints, following the platform conventions: stdlib ServeMux with method
// patterns, ident.RequireAuth on /v1, and the shared error contract.
package httpapi

import (
	"context"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ASanchezT85/ledgercore/libs/go/httpx"
	"github.com/ASanchezT85/ledgercore/libs/go/ident"
	"github.com/ASanchezT85/ledgercore/services/webhooks/internal/app"
	"github.com/ASanchezT85/ledgercore/services/webhooks/internal/config"
)

// server carries the handler dependencies.
type server struct {
	svc  *app.Service
	pool *pgxpool.Pool
}

// NewHandler assembles the full HTTP handler: health endpoints (public) and
// the /v1 API behind authentication.
//
// Auth (R-007): every /v1 route is behind ident.RequireAuthConfig, which
// validates the EdDSA signature plus iss, this service's audience ("webhooks")
// and a bounded clock skew — matching reconciliation. Authorization (R-006):
// each route additionally requires a scope — webhooks:read for GETs,
// webhooks:write for state changes — so a token without the right scope is
// rejected with 403 (deny by default).
func NewHandler(svc *app.Service, pool *pgxpool.Pool, cfg config.Config) http.Handler {
	s := &server{svc: svc, pool: pool}

	read := ident.RequireScope(ident.ScopeWebhooksRead)
	write := ident.RequireScope(ident.ScopeWebhooksWrite)

	api := http.NewServeMux()
	api.Handle("POST /v1/webhook-subscriptions", write(http.HandlerFunc(s.createSubscription)))
	api.Handle("GET /v1/webhook-subscriptions", read(http.HandlerFunc(s.listSubscriptions)))
	api.Handle("GET /v1/webhook-subscriptions/{id}", read(http.HandlerFunc(s.getSubscription)))
	api.Handle("PATCH /v1/webhook-subscriptions/{id}", write(http.HandlerFunc(s.updateSubscription)))
	api.Handle("POST /v1/webhook-subscriptions/{id}/rotate-secret", write(http.HandlerFunc(s.rotateSecret)))
	api.Handle("GET /v1/webhook-deliveries", read(http.HandlerFunc(s.listDeliveries)))
	api.Handle("POST /v1/webhook-deliveries/{id}/retry", write(http.HandlerFunc(s.retryDelivery)))

	auth := ident.RequireAuthConfig(ident.AuthConfig{
		JWKSURL:          cfg.JWKSURL,
		AuthDisabled:     cfg.AuthDisabled,
		ExpectedIssuer:   ident.DefaultIssuer,
		ExpectedAudience: ident.AudienceWebhooks,
	})

	root := http.NewServeMux()
	root.HandleFunc("GET /healthz", s.healthz)
	root.HandleFunc("GET /readyz", s.readyz)
	root.Handle("/v1/", auth(api))

	var h http.Handler = root
	h = httpx.Logger(h)
	h = httpx.Recover(h)
	h = httpx.RequestID(h)
	return h
}

// healthz is the liveness probe: the process is up.
func (s *server) healthz(w http.ResponseWriter, _ *http.Request) {
	httpx.WriteJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// readyz is the readiness probe: the database answers a ping.
func (s *server) readyz(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()
	if err := s.pool.Ping(ctx); err != nil {
		httpx.WriteError(w, r, http.StatusServiceUnavailable, httpx.CodeServiceUnavailable, "database is unreachable")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]string{"status": "ready"})
}
