package httpapi

import (
	"context"
	"net/http"
	"time"

	"github.com/ASanchezT85/ledgercore/libs/go/httpx"
	"github.com/ASanchezT85/ledgercore/libs/go/ident"

	"github.com/ASanchezT85/ledgercore/services/ledger-core/internal/app"
)

// NewRouter assembles the full HTTP surface of ledger-core:
//   - /healthz (liveness) and /readyz (Postgres ping) outside auth,
//   - every /v1 route behind ident.RequireAuthConfig (JWT EdDSA against the
//     identity JWKS, validating iss + aud=ledger-core + bounded skew, or
//     X-Tenant-Id when auth is disabled in dev),
//   - each route additionally gated by a scope (LC-012 / R-006): read scope
//     for GETs, write scope for state changes, deny by default,
//   - request id, panic recovery and structured logging around everything.
func NewRouter(svc *app.Service, dbPing func(context.Context) error, jwksURL string, authDisabled bool) http.Handler {
	h := &handler{svc: svc}

	read := ident.RequireScope(ident.ScopeLedgerRead)
	write := ident.RequireScope(ident.ScopeLedgerWrite)

	api := http.NewServeMux()

	// Ledgers.
	api.Handle("POST /v1/ledgers", write(http.HandlerFunc(h.createLedger)))
	api.Handle("GET /v1/ledgers", read(http.HandlerFunc(h.listLedgers)))
	api.Handle("GET /v1/ledgers/{id}", read(http.HandlerFunc(h.getLedger)))

	// Accounts.
	api.Handle("POST /v1/accounts", write(http.HandlerFunc(h.createAccount)))
	api.Handle("GET /v1/accounts", read(http.HandlerFunc(h.listAccounts)))
	api.Handle("GET /v1/accounts/{id}", read(http.HandlerFunc(h.getAccount)))
	api.Handle("GET /v1/accounts/{id}/balances", read(http.HandlerFunc(h.getBalances)))
	api.Handle("GET /v1/accounts/{id}/entries", read(http.HandlerFunc(h.listEntries)))

	// Transactions.
	api.Handle("POST /v1/transactions", write(http.HandlerFunc(h.createTransaction)))
	api.Handle("GET /v1/transactions", read(http.HandlerFunc(h.listTransactions)))
	api.Handle("GET /v1/transactions/{id}", read(http.HandlerFunc(h.getTransaction)))
	api.Handle("POST /v1/transactions/{id}/post", write(http.HandlerFunc(h.postTransaction)))
	api.Handle("POST /v1/transactions/{id}/reverse", write(http.HandlerFunc(h.reverseTransaction)))

	// Holds.
	api.Handle("POST /v1/holds", write(http.HandlerFunc(h.createHold)))
	api.Handle("GET /v1/holds/{id}", read(http.HandlerFunc(h.getHold)))
	api.Handle("POST /v1/holds/{id}/capture", write(http.HandlerFunc(h.captureHold)))
	api.Handle("POST /v1/holds/{id}/release", write(http.HandlerFunc(h.releaseHold)))

	// Reports.
	api.Handle("GET /v1/trial-balance", read(http.HandlerFunc(h.trialBalance)))
	api.Handle("GET /v1/statements", read(http.HandlerFunc(h.statement)))
	api.Handle("GET /v1/provider-positions", read(http.HandlerFunc(h.providerPositions)))

	auth := ident.RequireAuthConfig(ident.AuthConfig{
		JWKSURL:          jwksURL,
		AuthDisabled:     authDisabled,
		ExpectedIssuer:   ident.DefaultIssuer,
		ExpectedAudience: ident.AudienceLedgerCore,
	})

	root := http.NewServeMux()
	root.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		httpx.WriteJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})
	root.HandleFunc("GET /readyz", func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
		defer cancel()
		if err := dbPing(ctx); err != nil {
			httpx.WriteError(w, r, http.StatusServiceUnavailable, httpx.CodeServiceUnavailable, "database is unreachable")
			return
		}
		httpx.WriteJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})
	root.Handle("/v1/", auth(api))

	var handler http.Handler = root
	// CORS must wrap the auth middleware: browser preflights (OPTIONS) carry
	// no Authorization header and would otherwise die with 401 before the
	// CORS handler could answer them. The console is a cross-origin browser
	// client in every deployment.
	handler = httpx.CORSDev(handler)
	return httpx.RequestID(httpx.Recover(httpx.Logger(handler)))
}
