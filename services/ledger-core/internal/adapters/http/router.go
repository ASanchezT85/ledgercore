package httpapi

import (
	"context"
	"net/http"
	"time"

	"github.com/ledgercore/ledgercore/libs/go/httpx"
	"github.com/ledgercore/ledgercore/libs/go/ident"

	"github.com/ledgercore/ledgercore/services/ledger-core/internal/app"
)

// NewRouter assembles the full HTTP surface of ledger-core:
//   - /healthz (liveness) and /readyz (Postgres ping) outside auth,
//   - every /v1 route behind ident.RequireAuth (JWT EdDSA against the
//     identity JWKS, or X-Tenant-Id when auth is disabled in dev),
//   - request id, panic recovery and structured logging around everything.
func NewRouter(svc *app.Service, dbPing func(context.Context) error, jwksURL string, authDisabled bool) http.Handler {
	h := &handler{svc: svc}

	api := http.NewServeMux()

	// Ledgers.
	api.HandleFunc("POST /v1/ledgers", h.createLedger)
	api.HandleFunc("GET /v1/ledgers", h.listLedgers)
	api.HandleFunc("GET /v1/ledgers/{id}", h.getLedger)

	// Accounts.
	api.HandleFunc("POST /v1/accounts", h.createAccount)
	api.HandleFunc("GET /v1/accounts", h.listAccounts)
	api.HandleFunc("GET /v1/accounts/{id}", h.getAccount)
	api.HandleFunc("GET /v1/accounts/{id}/balances", h.getBalances)
	api.HandleFunc("GET /v1/accounts/{id}/entries", h.listEntries)

	// Transactions.
	api.HandleFunc("POST /v1/transactions", h.createTransaction)
	api.HandleFunc("GET /v1/transactions", h.listTransactions)
	api.HandleFunc("GET /v1/transactions/{id}", h.getTransaction)
	api.HandleFunc("POST /v1/transactions/{id}/post", h.postTransaction)
	api.HandleFunc("POST /v1/transactions/{id}/reverse", h.reverseTransaction)

	// Holds.
	api.HandleFunc("POST /v1/holds", h.createHold)
	api.HandleFunc("GET /v1/holds/{id}", h.getHold)
	api.HandleFunc("POST /v1/holds/{id}/capture", h.captureHold)
	api.HandleFunc("POST /v1/holds/{id}/release", h.releaseHold)

	// Reports.
	api.HandleFunc("GET /v1/trial-balance", h.trialBalance)
	api.HandleFunc("GET /v1/statements", h.statement)
	api.HandleFunc("GET /v1/provider-positions", h.providerPositions)

	root := http.NewServeMux()
	root.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		httpx.WriteJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})
	root.HandleFunc("GET /readyz", func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
		defer cancel()
		if err := dbPing(ctx); err != nil {
			httpx.WriteError(w, http.StatusServiceUnavailable, "not_ready", "database is unreachable")
			return
		}
		httpx.WriteJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})
	root.Handle("/v1/", ident.RequireAuth(jwksURL, authDisabled)(api))

	var handler http.Handler = root
	// CORS must wrap the auth middleware: browser preflights (OPTIONS) carry
	// no Authorization header and would otherwise die with 401 before the
	// CORS handler could answer them. The console is a cross-origin browser
	// client in every deployment.
	handler = httpx.CORSDev(handler)
	return httpx.RequestID(httpx.Recover(httpx.Logger(handler)))
}
