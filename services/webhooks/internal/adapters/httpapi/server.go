// Package httpapi exposes the webhooks management REST API plus the health
// endpoints, following the platform conventions: stdlib ServeMux with method
// patterns, ident.RequireAuth on /v1, and the shared error contract.
package httpapi

import (
	"context"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ledgercore/ledgercore/libs/go/httpx"
	"github.com/ledgercore/ledgercore/libs/go/ident"
	"github.com/ledgercore/ledgercore/services/webhooks/internal/app"
	"github.com/ledgercore/ledgercore/services/webhooks/internal/config"
)

// server carries the handler dependencies.
type server struct {
	svc  *app.Service
	pool *pgxpool.Pool
}

// NewHandler assembles the full HTTP handler: health endpoints (public) and
// the /v1 API behind authentication.
func NewHandler(svc *app.Service, pool *pgxpool.Pool, cfg config.Config) http.Handler {
	s := &server{svc: svc, pool: pool}

	api := http.NewServeMux()
	api.HandleFunc("POST /v1/webhook-subscriptions", s.createSubscription)
	api.HandleFunc("GET /v1/webhook-subscriptions", s.listSubscriptions)
	api.HandleFunc("GET /v1/webhook-subscriptions/{id}", s.getSubscription)
	api.HandleFunc("PATCH /v1/webhook-subscriptions/{id}", s.updateSubscription)
	api.HandleFunc("POST /v1/webhook-subscriptions/{id}/rotate-secret", s.rotateSecret)
	api.HandleFunc("GET /v1/webhook-deliveries", s.listDeliveries)
	api.HandleFunc("POST /v1/webhook-deliveries/{id}/retry", s.retryDelivery)

	auth := ident.RequireAuth(cfg.JWKSURL, cfg.AuthDisabled)

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
