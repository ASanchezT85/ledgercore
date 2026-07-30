package app

import (
	"context"
	"time"

	"github.com/google/uuid"

	"github.com/ledgercore/ledgercore/libs/go/httpx"
	"github.com/ledgercore/ledgercore/libs/go/money"

	"github.com/ledgercore/ledgercore/services/ledger-core/internal/domain"
)

// Page is a keyset-pagination request: an opaque (created_at, id) cursor plus
// a page size. Listings return rows strictly older than the cursor, newest
// first.
type Page struct {
	Cursor httpx.Cursor
	Limit  int
}

// AccountFilter narrows account listings.
type AccountFilter struct {
	LedgerID *uuid.UUID
	Type     *domain.AccountType
}

// TransactionFilter narrows transaction listings.
type TransactionFilter struct {
	LedgerID *uuid.UUID
	Status   *domain.TransactionStatus
}

// Store is the persistence port of the ledger engine. Every method is
// tenant-scoped and runs inside a single database transaction with RLS
// enforced (pgxutil.WithTenantTx); multi-step methods (transaction creation,
// balance application, outbox writes) are all-or-nothing.
type Store interface {
	CreateLedger(ctx context.Context, l domain.Ledger) error
	GetLedger(ctx context.Context, tenantID, id uuid.UUID) (domain.Ledger, error)
	ListLedgers(ctx context.Context, tenantID uuid.UUID, page Page) ([]domain.Ledger, error)

	CreateAccount(ctx context.Context, a domain.Account) error
	GetAccount(ctx context.Context, tenantID, id uuid.UUID) (domain.Account, error)
	ListAccounts(ctx context.Context, tenantID uuid.UUID, filter AccountFilter, page Page) ([]domain.Account, error)
	GetBalances(ctx context.Context, tenantID, accountID uuid.UUID) (domain.Account, []domain.Balance, error)
	ListEntries(ctx context.Context, tenantID, accountID uuid.UUID, asset string, page Page) ([]domain.Entry, error)

	// CreateTransaction persists a validated transaction. The bool result is
	// true when the idempotency key had already been used: the stored original
	// transaction is returned instead and nothing is written.
	CreateTransaction(ctx context.Context, t domain.Transaction) (domain.Transaction, bool, error)
	GetTransaction(ctx context.Context, tenantID, id uuid.UUID) (domain.Transaction, error)
	ListTransactions(ctx context.Context, tenantID uuid.UUID, filter TransactionFilter, page Page) ([]domain.Transaction, error)
	PostTransaction(ctx context.Context, tenantID, id uuid.UUID, now time.Time) (domain.Transaction, error)
	ReverseTransaction(ctx context.Context, tenantID, id uuid.UUID, idempotencyKey, reason string, now time.Time) (domain.Transaction, error)

	// CreateHold reserves funds. The bool result is true on idempotent replay.
	CreateHold(ctx context.Context, h domain.Hold) (domain.Hold, bool, error)
	CaptureHold(ctx context.Context, tenantID, id uuid.UUID, amount *money.Amount, transactionID *uuid.UUID, now time.Time) (domain.Hold, error)
	ReleaseHold(ctx context.Context, tenantID, id uuid.UUID, now time.Time) (domain.Hold, error)
	GetHold(ctx context.Context, tenantID, id uuid.UUID) (domain.Hold, error)

	// TrialBalance reports the current state (from account_balances) when
	// asOf is nil, or reconstructs the historical state by aggregating
	// postings with effective_at <= asOf.
	TrialBalance(ctx context.Context, tenantID, ledgerID uuid.UUID, asOf *time.Time) (domain.TrialBalance, error)

	// Statement builds the account statement for [from, to]: opening balance
	// before from, paged entries with running balances, and closing balance.
	Statement(ctx context.Context, tenantID, accountID uuid.UUID, from, to time.Time, page Page) (domain.Statement, error)

	// ProviderAccountBalances returns the account/asset aggregates of every
	// asset- or liability-type account, read from account_balances.
	ProviderAccountBalances(ctx context.Context, tenantID uuid.UUID) ([]domain.ProviderAccountBalance, error)

	// VerifyBalances recomputes balances from postings and returns the rows
	// where the derived account_balances table drifts from that source.
	VerifyBalances(ctx context.Context, tenantID, ledgerID uuid.UUID) ([]domain.BalanceDiscrepancy, error)
}
