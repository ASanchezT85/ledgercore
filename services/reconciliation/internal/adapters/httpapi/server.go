// Package httpapi exposes the reconciliation REST API using only net/http
// (ServeMux with method patterns). Auth is enforced on /v1 via
// ident.RequireAuth; /healthz and /readyz stay open.
package httpapi

import (
	"context"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/ASanchezT85/ledgercore/libs/go/httpx"
	"github.com/ASanchezT85/ledgercore/libs/go/ident"

	"github.com/ASanchezT85/ledgercore/services/reconciliation/internal/app"
)

// Server holds the handler dependencies.
type Server struct {
	svc  *app.Service
	pool *pgxpool.Pool
}

// NewHandler wires routes and middlewares and returns the root handler.
//
// Auth (LC-013): every /v1 route is behind ident.RequireAuthConfig, which
// validates the EdDSA signature plus iss, aud (this service's audience) and a
// bounded clock skew. Authorization (LC-012): each route additionally
// requires a scope — read scope for GETs, write scope for state changes — so
// a token without the right scope is rejected with 403 (deny by default).
func NewHandler(svc *app.Service, pool *pgxpool.Pool, jwksURL string, authDisabled bool) http.Handler {
	s := &Server{svc: svc, pool: pool}

	read := ident.RequireScope(ident.ScopeReconRead)
	write := ident.RequireScope(ident.ScopeReconWrite)

	api := http.NewServeMux()
	api.Handle("POST /v1/reconciliation/sources", write(http.HandlerFunc(s.createSource)))
	api.Handle("GET /v1/reconciliation/sources", read(http.HandlerFunc(s.listSources)))
	api.Handle("POST /v1/reconciliation/imports", write(http.HandlerFunc(s.createImport)))
	api.Handle("POST /v1/reconciliation/runs", write(http.HandlerFunc(s.createRun)))
	api.Handle("GET /v1/reconciliation/runs/{id}", read(http.HandlerFunc(s.getRun)))
	api.Handle("GET /v1/reconciliation/reports", read(http.HandlerFunc(s.getReports)))
	api.Handle("GET /v1/reconciliation/discrepancies", read(http.HandlerFunc(s.listDiscrepancies)))
	api.Handle("PATCH /v1/reconciliation/discrepancies/{id}", write(http.HandlerFunc(s.patchDiscrepancy)))

	auth := ident.RequireAuthConfig(ident.AuthConfig{
		JWKSURL:          jwksURL,
		AuthDisabled:     authDisabled,
		ExpectedIssuer:   ident.DefaultIssuer,
		ExpectedAudience: ident.AudienceReconciliation,
	})

	root := http.NewServeMux()
	root.HandleFunc("GET /healthz", s.healthz)
	root.HandleFunc("GET /readyz", s.readyz)
	root.Handle("/v1/", auth(api))

	return httpx.RequestID(httpx.Recover(httpx.Logger(httpx.CORSDev(root))))
}

func (s *Server) healthz(w http.ResponseWriter, _ *http.Request) {
	httpx.WriteJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) readyz(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
	defer cancel()
	if err := s.pool.Ping(ctx); err != nil {
		httpx.WriteError(w, r, http.StatusServiceUnavailable, httpx.CodeServiceUnavailable, "database is unreachable")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]string{"status": "ready"})
}
