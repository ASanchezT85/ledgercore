// Package app contains the application services (use cases) of the ledger
// engine. It validates inputs against the domain rules and orchestrates the
// persistence port; it knows nothing about HTTP or SQL.
package app

import (
	"context"
	"fmt"
	"sort"
	"time"

	"github.com/google/uuid"

	"github.com/ASanchezT85/ledgercore/libs/go/money"

	"github.com/ASanchezT85/ledgercore/services/ledger-core/internal/domain"
)

// DefaultHoldTTL is how long a hold lives when the client does not provide
// expires_at.
const DefaultHoldTTL = 7 * 24 * time.Hour

// Service exposes every ledger use case.
type Service struct {
	store Store
	now   func() time.Time
}

// NewService builds the application service on top of a persistence port.
func NewService(store Store) *Service {
	return &Service{store: store, now: func() time.Time { return time.Now().UTC() }}
}

func newID() (uuid.UUID, error) {
	id, err := uuid.NewV7()
	if err != nil {
		return uuid.Nil, fmt.Errorf("generate id: %w", err)
	}
	return id, nil
}

// ---- Ledgers ------------------------------------------------------------------

// CreateLedgerInput carries the fields to create a ledger.
type CreateLedgerInput struct {
	Name        string
	Description string
	Environment string // from the token claims: "sandbox" | "live"
	Metadata    map[string]string
}

// CreateLedger creates a ledger in the caller's environment.
func (s *Service) CreateLedger(ctx context.Context, tenantID uuid.UUID, in CreateLedgerInput) (domain.Ledger, error) {
	if in.Name == "" || len(in.Name) > 255 {
		return domain.Ledger{}, fmt.Errorf("%w: name must be 1-255 characters", ErrValidation)
	}
	id, err := newID()
	if err != nil {
		return domain.Ledger{}, err
	}
	l := domain.Ledger{
		ID:          id,
		TenantID:    tenantID,
		Name:        in.Name,
		Description: in.Description,
		Environment: in.Environment,
		Metadata:    in.Metadata,
		CreatedAt:   s.now(),
	}
	if err := s.store.CreateLedger(ctx, l); err != nil {
		return domain.Ledger{}, err
	}
	return l, nil
}

// GetLedger fetches one ledger by id.
func (s *Service) GetLedger(ctx context.Context, tenantID, id uuid.UUID) (domain.Ledger, error) {
	return s.store.GetLedger(ctx, tenantID, id)
}

// ListLedgers pages through the tenant's ledgers, newest first.
func (s *Service) ListLedgers(ctx context.Context, tenantID uuid.UUID, page Page) ([]domain.Ledger, error) {
	return s.store.ListLedgers(ctx, tenantID, page)
}

// ---- Accounts -----------------------------------------------------------------

// CreateAccountInput carries the fields to create an account.
type CreateAccountInput struct {
	LedgerID      uuid.UUID
	Name          string
	Type          domain.AccountType
	NormalBalance domain.NormalBalance
	Metadata      map[string]string
}

// CreateAccount creates an account inside an existing ledger. The account
// name doubles as its unique path within the ledger.
func (s *Service) CreateAccount(ctx context.Context, tenantID uuid.UUID, in CreateAccountInput) (domain.Account, error) {
	if in.Name == "" || len(in.Name) > 255 {
		return domain.Account{}, fmt.Errorf("%w: name must be 1-255 characters", ErrValidation)
	}
	if !in.Type.Valid() {
		return domain.Account{}, fmt.Errorf("%w: type must be one of asset|liability|equity|revenue|expense", ErrValidation)
	}
	if !in.NormalBalance.Valid() {
		return domain.Account{}, fmt.Errorf("%w: normal_balance must be DEBIT or CREDIT", ErrValidation)
	}
	if in.LedgerID == uuid.Nil {
		return domain.Account{}, fmt.Errorf("%w: ledger_id is required", ErrValidation)
	}
	id, err := newID()
	if err != nil {
		return domain.Account{}, err
	}
	a := domain.Account{
		ID:            id,
		TenantID:      tenantID,
		LedgerID:      in.LedgerID,
		Name:          in.Name,
		Path:          in.Name,
		Type:          in.Type,
		NormalBalance: in.NormalBalance,
		Status:        "active",
		Metadata:      in.Metadata,
		CreatedAt:     s.now(),
	}
	if err := s.store.CreateAccount(ctx, a); err != nil {
		return domain.Account{}, err
	}
	return a, nil
}

// GetAccount fetches one account by id.
func (s *Service) GetAccount(ctx context.Context, tenantID, id uuid.UUID) (domain.Account, error) {
	return s.store.GetAccount(ctx, tenantID, id)
}

// ListAccounts pages through the tenant's accounts, newest first.
func (s *Service) ListAccounts(ctx context.Context, tenantID uuid.UUID, filter AccountFilter, page Page) ([]domain.Account, error) {
	return s.store.ListAccounts(ctx, tenantID, filter, page)
}

// GetBalances returns the account plus one Balance row per asset it has
// touched. Available is derived from the account's normal balance.
func (s *Service) GetBalances(ctx context.Context, tenantID, accountID uuid.UUID) (domain.Account, []domain.Balance, error) {
	return s.store.GetBalances(ctx, tenantID, accountID)
}

// ListEntries pages through the postings that hit an account, newest first.
func (s *Service) ListEntries(ctx context.Context, tenantID, accountID uuid.UUID, asset string, page Page) ([]domain.Entry, error) {
	if asset != "" {
		if err := money.ValidateAssetCode(asset); err != nil {
			return nil, fmt.Errorf("%w: %v", ErrValidation, err)
		}
	}
	return s.store.ListEntries(ctx, tenantID, accountID, asset, page)
}

// ---- Transactions ----------------------------------------------------------------

// PostingInput is one leg of a transaction as submitted by the client.
type PostingInput struct {
	AccountID uuid.UUID
	Direction domain.Direction
	Amount    money.Amount
}

// CreateTransactionInput carries the fields to create a transaction.
type CreateTransactionInput struct {
	LedgerID       uuid.UUID
	IdempotencyKey string
	Reference      string
	Description    string
	Status         domain.TransactionStatus // draft | posted (default posted)
	EffectiveAt    *time.Time
	Metadata       map[string]string
	Postings       []PostingInput
}

// CreateTransaction validates the double-entry invariant and persists the
// transaction atomically (postings + balances + outbox event + idempotency
// record in one database transaction). The bool result reports an idempotent
// replay.
func (s *Service) CreateTransaction(ctx context.Context, tenantID uuid.UUID, in CreateTransactionInput) (domain.Transaction, bool, error) {
	if in.IdempotencyKey == "" || len(in.IdempotencyKey) > 255 {
		return domain.Transaction{}, false, fmt.Errorf("%w: idempotency_key must be 1-255 characters", ErrValidation)
	}
	if in.LedgerID == uuid.Nil {
		return domain.Transaction{}, false, fmt.Errorf("%w: ledger_id is required", ErrValidation)
	}
	status := in.Status
	if status == "" {
		status = domain.TransactionPosted
	}
	if status != domain.TransactionDraft && status != domain.TransactionPosted {
		return domain.Transaction{}, false, fmt.Errorf("%w: status must be draft or posted", ErrValidation)
	}
	if len(in.Postings) > 1000 {
		return domain.Transaction{}, false, fmt.Errorf("%w: at most 1000 postings per transaction", ErrValidation)
	}

	now := s.now()
	id, err := newID()
	if err != nil {
		return domain.Transaction{}, false, err
	}
	t := domain.Transaction{
		ID:             id,
		TenantID:       tenantID,
		LedgerID:       in.LedgerID,
		Reference:      in.Reference,
		Description:    in.Description,
		IdempotencyKey: in.IdempotencyKey,
		Status:         status,
		EffectiveAt:    now,
		Metadata:       in.Metadata,
		CreatedAt:      now,
		Postings:       make([]domain.Posting, len(in.Postings)),
	}
	if in.EffectiveAt != nil {
		t.EffectiveAt = in.EffectiveAt.UTC()
		t.EffectiveAtProvided = true
	}
	if status == domain.TransactionPosted {
		postedAt := now
		t.PostedAt = &postedAt
	}
	for i, p := range in.Postings {
		pid, err := newID()
		if err != nil {
			return domain.Transaction{}, false, err
		}
		t.Postings[i] = domain.Posting{
			ID:        pid,
			AccountID: p.AccountID,
			Direction: p.Direction,
			Amount:    p.Amount,
		}
	}

	// The central invariant: per asset, debits == credits.
	if err := domain.ValidateBalanced(t.Postings); err != nil {
		return domain.Transaction{}, false, err
	}
	return s.store.CreateTransaction(ctx, t)
}

// GetTransaction fetches one transaction with its postings.
func (s *Service) GetTransaction(ctx context.Context, tenantID, id uuid.UUID) (domain.Transaction, error) {
	return s.store.GetTransaction(ctx, tenantID, id)
}

// ListTransactions pages through transactions, newest first.
func (s *Service) ListTransactions(ctx context.Context, tenantID uuid.UUID, filter TransactionFilter, page Page) ([]domain.Transaction, error) {
	return s.store.ListTransactions(ctx, tenantID, filter, page)
}

// PostTransaction transitions a draft to posted, applying balances and
// emitting ledger.transaction.posted.
//
// R-008 note — naturally idempotent by design: post carries no request payload
// beyond the transaction id, and the operation is a pure state-machine
// transition (draft->posted). Re-posting an already-posted transaction is a
// no-op that returns it unchanged (see the store), so a retry can never
// double-apply balances or emit a duplicate event. There is therefore no
// idempotency key or request fingerprint to store: the transaction row itself
// is the idempotency record.
func (s *Service) PostTransaction(ctx context.Context, tenantID, id uuid.UUID) (domain.Transaction, error) {
	return s.store.PostTransaction(ctx, tenantID, id, s.now())
}

// ReverseTransactionInput carries the optional reversal fields.
type ReverseTransactionInput struct {
	IdempotencyKey string // defaults to "reversal-of:<id>"
	Reason         string
}

// ReverseTransaction creates and posts the mirror transaction, marks the
// original as reversed and emits ledger.transaction.reversed. It is idempotent
// by reversal idempotency key (R-008); the bool result reports an idempotent
// replay.
func (s *Service) ReverseTransaction(ctx context.Context, tenantID, id uuid.UUID, in ReverseTransactionInput) (domain.Transaction, bool, error) {
	if len(in.IdempotencyKey) > 255 {
		return domain.Transaction{}, false, fmt.Errorf("%w: idempotency_key must be at most 255 characters", ErrValidation)
	}
	if len(in.Reason) > 1024 {
		return domain.Transaction{}, false, fmt.Errorf("%w: reason must be at most 1024 characters", ErrValidation)
	}
	return s.store.ReverseTransaction(ctx, tenantID, id, in.IdempotencyKey, in.Reason, s.now())
}

// ---- Holds --------------------------------------------------------------------

// CreateHoldInput carries the fields to create a hold.
type CreateHoldInput struct {
	AccountID      uuid.UUID
	LedgerID       uuid.UUID // optional; validated against the account when set
	IdempotencyKey string
	Amount         money.Amount
	ExpiresAt      *time.Time
	Metadata       map[string]string
}

// CreateHold reserves funds on an account, reducing its available balance
// without moving money. Fails with ErrInsufficientFunds when available is
// below the requested amount. The bool result reports an idempotent replay.
func (s *Service) CreateHold(ctx context.Context, tenantID uuid.UUID, in CreateHoldInput) (domain.Hold, bool, error) {
	if in.IdempotencyKey == "" || len(in.IdempotencyKey) > 255 {
		return domain.Hold{}, false, fmt.Errorf("%w: idempotency_key must be 1-255 characters", ErrValidation)
	}
	if in.AccountID == uuid.Nil {
		return domain.Hold{}, false, fmt.Errorf("%w: account_id is required", ErrValidation)
	}
	if err := money.ValidateAssetCode(in.Amount.Asset); err != nil {
		return domain.Hold{}, false, fmt.Errorf("%w: %v", ErrValidation, err)
	}
	if in.Amount.Units <= 0 {
		return domain.Hold{}, false, fmt.Errorf("%w: amount must be greater than zero", ErrValidation)
	}
	now := s.now()
	expiresAt := now.Add(DefaultHoldTTL)
	expiresAtProvided := false
	if in.ExpiresAt != nil {
		expiresAt = in.ExpiresAt.UTC()
		expiresAtProvided = true
		if !expiresAt.After(now) {
			return domain.Hold{}, false, fmt.Errorf("%w: expires_at must be in the future", ErrValidation)
		}
	}
	id, err := newID()
	if err != nil {
		return domain.Hold{}, false, err
	}
	h := domain.Hold{
		ID:             id,
		TenantID:       tenantID,
		LedgerID:       in.LedgerID, // the store fills/validates it from the account
		AccountID:      in.AccountID,
		IdempotencyKey: in.IdempotencyKey,
		Amount:         in.Amount,
		Status:            domain.HoldActive,
		ExpiresAt:         expiresAt,
		ExpiresAtProvided: expiresAtProvided,
		Metadata:          in.Metadata,
		CreatedAt:         now,
	}
	return s.store.CreateHold(ctx, h)
}

// CaptureHoldInput carries the optional capture fields.
type CaptureHoldInput struct {
	Amount        *money.Amount // defaults to the full hold amount
	TransactionID *uuid.UUID    // optional posted transaction to link
}

// CaptureHold finalizes a hold: the held funds are freed and the hold is
// marked captured, optionally linked to the posted transaction that moved the
// money. A partial capture releases the remainder.
//
// R-008 note — idempotent by the hold state machine, not by a request
// fingerprint. A hold transitions active->captured (or active->released) at
// most once; the store takes FOR UPDATE on the row and rejects any capture or
// release of a non-active hold with ErrInvalidState (surfaced as 409). A
// retried capture therefore can never free the held funds twice or emit a
// duplicate event. Because the hold row's terminal status is itself the
// idempotency record, capture/release deliberately do NOT take a separate
// Idempotency-Key/request_hash: adding one would duplicate a guarantee the
// state machine already provides. (CreateHold, which CAN be replayed with an
// identical request, does carry a fingerprint — see the store.)
func (s *Service) CaptureHold(ctx context.Context, tenantID, id uuid.UUID, in CaptureHoldInput) (domain.Hold, error) {
	if in.Amount != nil {
		if err := money.ValidateAssetCode(in.Amount.Asset); err != nil {
			return domain.Hold{}, fmt.Errorf("%w: %v", ErrValidation, err)
		}
		if in.Amount.Units <= 0 {
			return domain.Hold{}, fmt.Errorf("%w: capture amount must be greater than zero", ErrValidation)
		}
	}
	return s.store.CaptureHold(ctx, tenantID, id, in.Amount, in.TransactionID, s.now())
}

// ReleaseHold frees the reserved funds without moving money.
func (s *Service) ReleaseHold(ctx context.Context, tenantID, id uuid.UUID) (domain.Hold, error) {
	return s.store.ReleaseHold(ctx, tenantID, id, s.now())
}

// GetHold fetches one hold by id.
func (s *Service) GetHold(ctx context.Context, tenantID, id uuid.UUID) (domain.Hold, error) {
	return s.store.GetHold(ctx, tenantID, id)
}

// ---- Reports ------------------------------------------------------------------

// TrialBalance aggregates account_balances per account/asset for a ledger and
// verifies that total debits equal total credits per asset. With asOf set, the
// report is reconstructed from postings with effective_at <= asOf instead.
func (s *Service) TrialBalance(ctx context.Context, tenantID, ledgerID uuid.UUID, asOf *time.Time) (domain.TrialBalance, error) {
	if asOf != nil {
		t := asOf.UTC()
		asOf = &t
	}
	return s.store.TrialBalance(ctx, tenantID, ledgerID, asOf)
}

// VerifyBalances recomputes the per-account/asset balances of a ledger from
// its postings and returns the discrepancies against the derived
// account_balances table. An empty result means the derived balances are
// consistent with the postings that back them.
func (s *Service) VerifyBalances(ctx context.Context, tenantID, ledgerID uuid.UUID) ([]domain.BalanceDiscrepancy, error) {
	if ledgerID == uuid.Nil {
		return nil, fmt.Errorf("%w: ledger_id is required", ErrValidation)
	}
	return s.store.VerifyBalances(ctx, tenantID, ledgerID)
}

// StatementDefaultPeriod is the lookback window applied when the client does
// not provide from/to.
const StatementDefaultPeriod = 30 * 24 * time.Hour

// Statement builds an account statement: opening balance before from, paged
// entries within [from, to] with per-asset running balances, and the closing
// balance at to. Zero from/to default to the last 30 days.
func (s *Service) Statement(ctx context.Context, tenantID, accountID uuid.UUID, from, to time.Time, page Page) (domain.Statement, error) {
	if accountID == uuid.Nil {
		return domain.Statement{}, fmt.Errorf("%w: account_id is required", ErrValidation)
	}
	if to.IsZero() {
		to = s.now()
	}
	if from.IsZero() {
		from = to.Add(-StatementDefaultPeriod)
	}
	from, to = from.UTC(), to.UTC()
	if from.After(to) {
		return domain.Statement{}, fmt.Errorf("%w: from must not be after to", ErrValidation)
	}
	return s.store.Statement(ctx, tenantID, accountID, from, to, page)
}

// ProviderPositions aggregates the net position against every counterparty
// following the naming convention <type>:provider:<name>:... (or the
// metadata.provider override) over asset- and liability-type accounts. The
// aggregation source is account_balances, not postings.
func (s *Service) ProviderPositions(ctx context.Context, tenantID uuid.UUID) ([]domain.ProviderPosition, error) {
	rows, err := s.store.ProviderAccountBalances(ctx, tenantID)
	if err != nil {
		return nil, err
	}
	type key struct{ provider, asset string }
	nets := make(map[key]*domain.ProviderAssetPosition)
	providers := make(map[string][]string) // provider -> ordered asset keys
	var order []string
	for _, r := range rows {
		provider, ok := domain.ProviderOf(r.Path, r.Metadata)
		if !ok {
			continue
		}
		k := key{provider, r.Asset}
		pos, ok := nets[k]
		if !ok {
			pos = &domain.ProviderAssetPosition{Asset: r.Asset}
			nets[k] = pos
			if _, seen := providers[provider]; !seen {
				order = append(order, provider)
			}
			providers[provider] = append(providers[provider], r.Asset)
		}
		pos.Net += r.PostedDebits - r.PostedCredits
		pos.Accounts = append(pos.Accounts, r.AccountID)
	}
	sort.Strings(order)
	out := make([]domain.ProviderPosition, 0, len(order))
	for _, provider := range order {
		assets := providers[provider]
		sort.Strings(assets)
		p := domain.ProviderPosition{Provider: provider}
		for _, asset := range assets {
			p.Positions = append(p.Positions, *nets[key{provider, asset}])
		}
		out = append(out, p)
	}
	return out, nil
}
