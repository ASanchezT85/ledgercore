// Package httpapi exposes the reconciliation REST API using only net/http
// (ServeMux with method patterns). Auth is enforced on /v1 via
// ident.RequireAuth; /healthz and /readyz stay open.
package httpapi

import (
	"context"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/ledgercore/ledgercore/libs/go/httpx"
	"github.com/ledgercore/ledgercore/libs/go/ident"

	"github.com/ledgercore/ledgercore/services/reconciliation/internal/app"
)

// Server holds the handler dependencies.
type Server struct {
	svc  *app.Service
	pool *pgxpool.Pool
}

// NewHandler wires routes and middlewares and returns the root handler.
func NewHandler(svc *app.Service, pool *pgxpool.Pool, jwksURL string, authDisabled bool) http.Handler {
	s := &Server{svc: svc, pool: pool}

	api := http.NewServeMux()
	api.HandleFunc("POST /v1/reconciliation/sources", s.createSource)
	api.HandleFunc("GET /v1/reconciliation/sources", s.listSources)
	api.HandleFunc("POST /v1/reconciliation/imports", s.createImport)
	api.HandleFunc("POST /v1/reconciliation/runs", s.createRun)
	api.HandleFunc("GET /v1/reconciliation/runs/{id}", s.getRun)
	api.HandleFunc("GET /v1/reconciliation/reports", s.getReports)
	api.HandleFunc("GET /v1/reconciliation/discrepancies", s.listDiscrepancies)
	api.HandleFunc("PATCH /v1/reconciliation/discrepancies/{id}", s.patchDiscrepancy)

	root := http.NewServeMux()
	root.HandleFunc("GET /healthz", s.healthz)
	root.HandleFunc("GET /readyz", s.readyz)
	root.Handle("/v1/", ident.RequireAuth(jwksURL, authDisabled)(api))

	return httpx.RequestID(httpx.Recover(httpx.Logger(httpx.CORSDev(root))))
}

func (s *Server) healthz(w http.ResponseWriter, _ *http.Request) {
	httpx.WriteJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) readyz(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
	defer cancel()
	if err := s.pool.Ping(ctx); err != nil {
		httpx.WriteError(w, http.StatusServiceUnavailable, "not_ready", "database is unreachable")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]string{"status": "ready"})
}
