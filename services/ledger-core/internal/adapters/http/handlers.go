// Package httpapi is the HTTP adapter of the ledger engine: request parsing,
// wire views per the OpenAPI contract, and the router. Business rules live in
// internal/app and internal/domain.
package httpapi

import (
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/ASanchezT85/ledgercore/libs/go/httpx"
	"github.com/ASanchezT85/ledgercore/libs/go/ident"
	"github.com/ASanchezT85/ledgercore/libs/go/money"

	"github.com/ASanchezT85/ledgercore/services/ledger-core/internal/app"
	"github.com/ASanchezT85/ledgercore/services/ledger-core/internal/domain"
)

// idempotencyReplayHeader marks responses served from the idempotency store.
const idempotencyReplayHeader = "X-Idempotent-Replay"

type handler struct {
	svc *app.Service
}

// ---- helpers ------------------------------------------------------------------

func (h *handler) tenant(w http.ResponseWriter, r *http.Request) (uuid.UUID, bool) {
	tenantID, ok := ident.TenantFromContext(r.Context())
	if !ok || tenantID == uuid.Nil {
		httpx.WriteError(w, r, http.StatusUnauthorized, httpx.CodeUnauthorized, "request has no tenant context")
		return uuid.Nil, false
	}
	return tenantID, true
}

func pathID(w http.ResponseWriter, r *http.Request) (uuid.UUID, bool) {
	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		httpx.WriteError(w, r, http.StatusBadRequest, httpx.CodeValidationFailed, "path id must be a valid UUID")
		return uuid.Nil, false
	}
	return id, true
}

// parsePage reads the uniform ?limit=/?cursor= parameters. The returned
// app.Page asks the store for limit+1 rows so the handler can tell whether a
// next page exists without a second query; the second return value is the
// page size actually exposed to the client.
func parsePage(w http.ResponseWriter, r *http.Request) (app.Page, int, bool) {
	pr, ok := httpx.ParsePage(w, r)
	if !ok {
		return app.Page{}, 0, false
	}
	return app.Page{Cursor: pr.Cursor, Limit: pr.FetchLimit()}, pr.Limit, true
}

func optionalUUID(w http.ResponseWriter, r *http.Request, raw, name string) (*uuid.UUID, bool) {
	if raw == "" {
		return nil, true
	}
	id, err := uuid.Parse(raw)
	if err != nil {
		httpx.WriteError(w, r, http.StatusBadRequest, httpx.CodeValidationFailed, name+" must be a valid UUID")
		return nil, false
	}
	return &id, true
}

// writeErr maps application/domain errors onto the API error contract.
// Unexpected errors are logged with detail but returned opaque, so internals
// (SQL, stack traces) never leak to clients.
func writeErr(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, app.ErrNotFound):
		httpx.WriteError(w, r, http.StatusNotFound, httpx.CodeNotFound, "resource not found")
	case errors.Is(err, app.ErrIdempotencyConflict):
		httpx.WriteError(w, r, http.StatusConflict, httpx.CodeIdempotencyConflict, "idempotency key reused with a different request payload")
	case errors.Is(err, app.ErrConflict):
		httpx.WriteError(w, r, http.StatusConflict, httpx.CodeConflict, "a resource with those unique attributes already exists")
	case errors.Is(err, app.ErrInvalidState):
		httpx.WriteError(w, r, http.StatusConflict, httpx.CodeConflict, trimSentinel(err.Error()))
	case errors.Is(err, app.ErrInsufficientFunds):
		httpx.WriteError(w, r, http.StatusUnprocessableEntity, httpx.CodeInsufficientFunds, trimSentinel(err.Error()))
	case errors.Is(err, domain.ErrUnbalanced), errors.Is(err, money.ErrOverflow):
		httpx.WriteError(w, r, http.StatusUnprocessableEntity, httpx.CodeUnbalancedTransaction, err.Error())
	case errors.Is(err, domain.ErrTooFewPostings),
		errors.Is(err, domain.ErrNonPositiveAmount),
		errors.Is(err, domain.ErrInvalidDirection),
		errors.Is(err, money.ErrInvalidAsset),
		errors.Is(err, app.ErrValidation):
		httpx.WriteError(w, r, http.StatusBadRequest, httpx.CodeValidationFailed, trimSentinel(err.Error()))
	default:
		slog.Error("unhandled request error",
			"error", err,
			"method", r.Method,
			"path", r.URL.Path,
			"request_id", httpx.RequestIDFromContext(r.Context()),
		)
		httpx.WriteError(w, r, http.StatusInternalServerError, httpx.CodeInternal, "an internal error occurred")
	}
}

// trimSentinel drops the "app: ..." sentinel prefix from user-facing messages.
func trimSentinel(msg string) string {
	for _, prefix := range []string{"app: invalid state: ", "app: insufficient funds: ", "app: validation failed: "} {
		if strings.HasPrefix(msg, prefix) {
			return strings.TrimPrefix(msg, prefix)
		}
	}
	return msg
}

// ---- ledgers ------------------------------------------------------------------

type createLedgerRequest struct {
	Name        string            `json:"name"`
	Description string            `json:"description"`
	Metadata    map[string]string `json:"metadata"`
}

func (h *handler) createLedger(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.tenant(w, r)
	if !ok {
		return
	}
	var req createLedgerRequest
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		httpx.WriteError(w, r, http.StatusBadRequest, httpx.CodeValidationFailed, err.Error())
		return
	}
	claims, _ := ident.ClaimsFromContext(r.Context())
	l, err := h.svc.CreateLedger(r.Context(), tenantID, app.CreateLedgerInput{
		Name:        req.Name,
		Description: req.Description,
		Environment: claims.Environment,
		Metadata:    req.Metadata,
	})
	if err != nil {
		writeErr(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusCreated, newLedgerView(l))
}

func (h *handler) listLedgers(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.tenant(w, r)
	if !ok {
		return
	}
	page, limit, ok := parsePage(w, r)
	if !ok {
		return
	}
	ledgers, err := h.svc.ListLedgers(r.Context(), tenantID, page)
	if err != nil {
		writeErr(w, r, err)
		return
	}
	ledgers, next := httpx.Window(ledgers, limit, func(l domain.Ledger) httpx.Cursor {
		return httpx.Cursor{CreatedAt: l.CreatedAt, ID: l.ID}
	})
	resp := listResponse[ledgerView]{Data: make([]ledgerView, len(ledgers)), NextCursor: next}
	for i, l := range ledgers {
		resp.Data[i] = newLedgerView(l)
	}
	httpx.WriteJSON(w, http.StatusOK, resp)
}

func (h *handler) getLedger(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.tenant(w, r)
	if !ok {
		return
	}
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	l, err := h.svc.GetLedger(r.Context(), tenantID, id)
	if err != nil {
		writeErr(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, newLedgerView(l))
}

// ---- accounts -----------------------------------------------------------------

type createAccountRequest struct {
	LedgerID      uuid.UUID         `json:"ledger_id"`
	Name          string            `json:"name"`
	Type          string            `json:"type"`
	NormalBalance string            `json:"normal_balance"`
	Metadata      map[string]string `json:"metadata"`
}

func (h *handler) createAccount(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.tenant(w, r)
	if !ok {
		return
	}
	var req createAccountRequest
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		httpx.WriteError(w, r, http.StatusBadRequest, httpx.CodeValidationFailed, err.Error())
		return
	}
	nb, valid := normalBalanceIn(req.NormalBalance)
	if !valid {
		httpx.WriteError(w, r, http.StatusBadRequest, httpx.CodeValidationFailed, "normal_balance must be DEBIT or CREDIT")
		return
	}
	a, err := h.svc.CreateAccount(r.Context(), tenantID, app.CreateAccountInput{
		LedgerID:      req.LedgerID,
		Name:          req.Name,
		Type:          domain.AccountType(strings.ToLower(req.Type)),
		NormalBalance: nb,
		Metadata:      req.Metadata,
	})
	if err != nil {
		writeErr(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusCreated, newAccountView(a))
}

func (h *handler) listAccounts(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.tenant(w, r)
	if !ok {
		return
	}
	page, limit, ok := parsePage(w, r)
	if !ok {
		return
	}
	filter := app.AccountFilter{}
	ledgerID, ok := optionalUUID(w, r, r.URL.Query().Get("ledger_id"), "ledger_id")
	if !ok {
		return
	}
	filter.LedgerID = ledgerID
	if raw := r.URL.Query().Get("type"); raw != "" {
		t := domain.AccountType(strings.ToLower(raw))
		if !t.Valid() {
			httpx.WriteError(w, r, http.StatusBadRequest, httpx.CodeValidationFailed, "type must be one of asset|liability|equity|revenue|expense")
			return
		}
		filter.Type = &t
	}
	accounts, err := h.svc.ListAccounts(r.Context(), tenantID, filter, page)
	if err != nil {
		writeErr(w, r, err)
		return
	}
	accounts, next := httpx.Window(accounts, limit, func(a domain.Account) httpx.Cursor {
		return httpx.Cursor{CreatedAt: a.CreatedAt, ID: a.ID}
	})
	resp := listResponse[accountView]{Data: make([]accountView, len(accounts)), NextCursor: next}
	for i, a := range accounts {
		resp.Data[i] = newAccountView(a)
	}
	httpx.WriteJSON(w, http.StatusOK, resp)
}

func (h *handler) getAccount(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.tenant(w, r)
	if !ok {
		return
	}
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	a, err := h.svc.GetAccount(r.Context(), tenantID, id)
	if err != nil {
		writeErr(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, newAccountView(a))
}

func (h *handler) getBalances(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.tenant(w, r)
	if !ok {
		return
	}
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	account, balances, err := h.svc.GetBalances(r.Context(), tenantID, id)
	if err != nil {
		writeErr(w, r, err)
		return
	}
	data := make([]balanceView, len(balances))
	for i, b := range balances {
		data[i] = newBalanceView(b, account.NormalBalance)
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"data": data})
}

func (h *handler) listEntries(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.tenant(w, r)
	if !ok {
		return
	}
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	page, limit, ok := parsePage(w, r)
	if !ok {
		return
	}
	entries, err := h.svc.ListEntries(r.Context(), tenantID, id, r.URL.Query().Get("asset"), page)
	if err != nil {
		writeErr(w, r, err)
		return
	}
	entries, next := httpx.Window(entries, limit, func(e domain.Entry) httpx.Cursor {
		return httpx.Cursor{CreatedAt: e.CreatedAt, ID: e.ID}
	})
	resp := listResponse[entryView]{Data: make([]entryView, len(entries)), NextCursor: next}
	for i, e := range entries {
		resp.Data[i] = newEntryView(e)
	}
	httpx.WriteJSON(w, http.StatusOK, resp)
}

// ---- transactions ----------------------------------------------------------------

type postingRequest struct {
	AccountID uuid.UUID `json:"account_id"`
	Direction string    `json:"direction"`
	Amount    moneyBody `json:"amount"`
}

type createTransactionRequest struct {
	LedgerID       uuid.UUID         `json:"ledger_id"`
	IdempotencyKey string            `json:"idempotency_key"`
	Reference      string            `json:"reference"`
	Description    string            `json:"description"`
	Status         string            `json:"status"`
	EffectiveAt    *time.Time        `json:"effective_at"`
	Metadata       map[string]string `json:"metadata"`
	Postings       []postingRequest  `json:"postings"`
}

func (h *handler) createTransaction(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.tenant(w, r)
	if !ok {
		return
	}
	var req createTransactionRequest
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		httpx.WriteError(w, r, http.StatusBadRequest, httpx.CodeValidationFailed, err.Error())
		return
	}
	if req.IdempotencyKey == "" {
		// The Idempotency-Key header is an accepted alternative to the body field.
		req.IdempotencyKey = r.Header.Get("Idempotency-Key")
	}
	in := app.CreateTransactionInput{
		LedgerID:       req.LedgerID,
		IdempotencyKey: req.IdempotencyKey,
		Reference:      req.Reference,
		Description:    req.Description,
		Status:         domain.TransactionStatus(req.Status),
		EffectiveAt:    req.EffectiveAt,
		Metadata:       req.Metadata,
		Postings:       make([]app.PostingInput, len(req.Postings)),
	}
	for i, p := range req.Postings {
		amount, err := p.Amount.toAmount()
		if err != nil {
			httpx.WriteError(w, r, http.StatusBadRequest, httpx.CodeValidationFailed,
				"postings["+strconv.Itoa(i)+"].amount.amount must be a string-encoded integer")
			return
		}
		in.Postings[i] = app.PostingInput{
			AccountID: p.AccountID,
			Direction: domain.Direction(strings.ToUpper(p.Direction)),
			Amount:    amount,
		}
	}
	t, replayed, err := h.svc.CreateTransaction(r.Context(), tenantID, in)
	if err != nil {
		writeErr(w, r, err)
		return
	}
	status := http.StatusCreated
	if replayed {
		w.Header().Set(idempotencyReplayHeader, "true")
		status = http.StatusOK
	}
	httpx.WriteJSON(w, status, newTransactionView(t))
}

func (h *handler) listTransactions(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.tenant(w, r)
	if !ok {
		return
	}
	page, limit, ok := parsePage(w, r)
	if !ok {
		return
	}
	filter := app.TransactionFilter{}
	ledgerID, ok := optionalUUID(w, r, r.URL.Query().Get("ledger_id"), "ledger_id")
	if !ok {
		return
	}
	filter.LedgerID = ledgerID
	if raw := r.URL.Query().Get("status"); raw != "" {
		s := domain.TransactionStatus(strings.ToLower(raw))
		if s != domain.TransactionDraft && s != domain.TransactionPosted && s != domain.TransactionReversed {
			httpx.WriteError(w, r, http.StatusBadRequest, httpx.CodeValidationFailed, "status must be draft, posted or reversed")
			return
		}
		filter.Status = &s
	}
	txs, err := h.svc.ListTransactions(r.Context(), tenantID, filter, page)
	if err != nil {
		writeErr(w, r, err)
		return
	}
	txs, next := httpx.Window(txs, limit, func(t domain.Transaction) httpx.Cursor {
		return httpx.Cursor{CreatedAt: t.CreatedAt, ID: t.ID}
	})
	resp := listResponse[transactionView]{Data: make([]transactionView, len(txs)), NextCursor: next}
	for i, t := range txs {
		resp.Data[i] = newTransactionView(t)
	}
	httpx.WriteJSON(w, http.StatusOK, resp)
}

func (h *handler) getTransaction(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.tenant(w, r)
	if !ok {
		return
	}
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	t, err := h.svc.GetTransaction(r.Context(), tenantID, id)
	if err != nil {
		writeErr(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, newTransactionView(t))
}

func (h *handler) postTransaction(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.tenant(w, r)
	if !ok {
		return
	}
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	t, err := h.svc.PostTransaction(r.Context(), tenantID, id)
	if err != nil {
		writeErr(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, newTransactionView(t))
}

type reverseTransactionRequest struct {
	IdempotencyKey string `json:"idempotency_key"`
	Reason         string `json:"reason"`
}

func (h *handler) reverseTransaction(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.tenant(w, r)
	if !ok {
		return
	}
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	var req reverseTransactionRequest
	if r.ContentLength != 0 {
		if err := httpx.DecodeJSON(w, r, &req); err != nil {
			httpx.WriteError(w, r, http.StatusBadRequest, httpx.CodeValidationFailed, err.Error())
			return
		}
	}
	if req.IdempotencyKey == "" {
		// The Idempotency-Key header is an accepted alternative to the body field.
		req.IdempotencyKey = r.Header.Get("Idempotency-Key")
	}
	t, replayed, err := h.svc.ReverseTransaction(r.Context(), tenantID, id, app.ReverseTransactionInput{
		IdempotencyKey: req.IdempotencyKey,
		Reason:         req.Reason,
	})
	if err != nil {
		writeErr(w, r, err)
		return
	}
	status := http.StatusCreated
	if replayed {
		w.Header().Set(idempotencyReplayHeader, "true")
		status = http.StatusOK
	}
	httpx.WriteJSON(w, status, newTransactionView(t))
}

// ---- holds --------------------------------------------------------------------

type createHoldRequest struct {
	LedgerID       *uuid.UUID        `json:"ledger_id"`
	AccountID      uuid.UUID         `json:"account_id"`
	IdempotencyKey string            `json:"idempotency_key"`
	Amount         moneyBody         `json:"amount"`
	ExpiresAt      *time.Time        `json:"expires_at"`
	Metadata       map[string]string `json:"metadata"`
}

func (h *handler) createHold(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.tenant(w, r)
	if !ok {
		return
	}
	var req createHoldRequest
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		httpx.WriteError(w, r, http.StatusBadRequest, httpx.CodeValidationFailed, err.Error())
		return
	}
	amount, err := req.Amount.toAmount()
	if err != nil {
		httpx.WriteError(w, r, http.StatusBadRequest, httpx.CodeValidationFailed, "amount.amount must be a string-encoded integer")
		return
	}
	in := app.CreateHoldInput{
		AccountID:      req.AccountID,
		IdempotencyKey: req.IdempotencyKey,
		Amount:         amount,
		ExpiresAt:      req.ExpiresAt,
		Metadata:       req.Metadata,
	}
	if req.LedgerID != nil {
		in.LedgerID = *req.LedgerID
	}
	hold, replayed, err := h.svc.CreateHold(r.Context(), tenantID, in)
	if err != nil {
		writeErr(w, r, err)
		return
	}
	status := http.StatusCreated
	if replayed {
		w.Header().Set(idempotencyReplayHeader, "true")
		status = http.StatusOK
	}
	httpx.WriteJSON(w, status, newHoldView(hold))
}

func (h *handler) getHold(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.tenant(w, r)
	if !ok {
		return
	}
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	hold, err := h.svc.GetHold(r.Context(), tenantID, id)
	if err != nil {
		writeErr(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, newHoldView(hold))
}

type captureHoldRequest struct {
	Amount        *moneyBody `json:"amount"`
	TransactionID *uuid.UUID `json:"transaction_id"`
}

func (h *handler) captureHold(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.tenant(w, r)
	if !ok {
		return
	}
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	var req captureHoldRequest
	if r.ContentLength != 0 {
		if err := httpx.DecodeJSON(w, r, &req); err != nil {
			httpx.WriteError(w, r, http.StatusBadRequest, httpx.CodeValidationFailed, err.Error())
			return
		}
	}
	in := app.CaptureHoldInput{TransactionID: req.TransactionID}
	if req.Amount != nil {
		amount, err := req.Amount.toAmount()
		if err != nil {
			httpx.WriteError(w, r, http.StatusBadRequest, httpx.CodeValidationFailed, "amount.amount must be a string-encoded integer")
			return
		}
		in.Amount = &amount
	}
	hold, err := h.svc.CaptureHold(r.Context(), tenantID, id, in)
	if err != nil {
		writeErr(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, newHoldView(hold))
}

func (h *handler) releaseHold(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.tenant(w, r)
	if !ok {
		return
	}
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	hold, err := h.svc.ReleaseHold(r.Context(), tenantID, id)
	if err != nil {
		writeErr(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, newHoldView(hold))
}

// ---- reports ---------------------------------------------------------------------

// optionalTime parses an RFC 3339 query parameter, zero when absent.
func optionalTime(w http.ResponseWriter, r *http.Request, raw, name string) (time.Time, bool) {
	if raw == "" {
		return time.Time{}, true
	}
	t, err := time.Parse(time.RFC3339, raw)
	if err != nil {
		httpx.WriteError(w, r, http.StatusBadRequest, httpx.CodeValidationFailed, name+" must be an RFC 3339 date-time")
		return time.Time{}, false
	}
	return t, true
}

func (h *handler) trialBalance(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.tenant(w, r)
	if !ok {
		return
	}
	ledgerID, err := uuid.Parse(r.URL.Query().Get("ledger_id"))
	if err != nil {
		httpx.WriteError(w, r, http.StatusBadRequest, httpx.CodeValidationFailed, "ledger_id query parameter is required and must be a UUID")
		return
	}
	var asOf *time.Time
	t, ok := optionalTime(w, r, r.URL.Query().Get("as_of"), "as_of")
	if !ok {
		return
	}
	if !t.IsZero() {
		asOf = &t
	}
	tb, err := h.svc.TrialBalance(r.Context(), tenantID, ledgerID, asOf)
	if err != nil {
		writeErr(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, newTrialBalanceView(tb))
}

func (h *handler) statement(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.tenant(w, r)
	if !ok {
		return
	}
	accountID, err := uuid.Parse(r.URL.Query().Get("account_id"))
	if err != nil {
		httpx.WriteError(w, r, http.StatusBadRequest, httpx.CodeValidationFailed, "account_id query parameter is required and must be a UUID")
		return
	}
	from, ok := optionalTime(w, r, r.URL.Query().Get("from"), "from")
	if !ok {
		return
	}
	to, ok := optionalTime(w, r, r.URL.Query().Get("to"), "to")
	if !ok {
		return
	}
	page, limit, ok := parsePage(w, r)
	if !ok {
		return
	}
	st, err := h.svc.Statement(r.Context(), tenantID, accountID, from, to, page)
	if err != nil {
		writeErr(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, newStatementView(st, limit))
}

func (h *handler) providerPositions(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.tenant(w, r)
	if !ok {
		return
	}
	positions, err := h.svc.ProviderPositions(r.Context(), tenantID)
	if err != nil {
		writeErr(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, newProviderPositionsView(positions))
}
