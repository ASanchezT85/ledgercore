// Package domain contains the pure double-entry ledger model: entities,
// invariants and rules. It has no infrastructure dependencies — only the
// shared money primitives and uuid.
package domain

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/ledgercore/ledgercore/libs/go/money"
)

// ---- Enumerations -----------------------------------------------------------

// AccountType is the accounting classification of an account.
type AccountType string

// Account types.
const (
	AccountAsset     AccountType = "asset"
	AccountLiability AccountType = "liability"
	AccountEquity    AccountType = "equity"
	AccountRevenue   AccountType = "revenue"
	AccountExpense   AccountType = "expense"
)

// Valid reports whether t is one of the known account types.
func (t AccountType) Valid() bool {
	switch t {
	case AccountAsset, AccountLiability, AccountEquity, AccountRevenue, AccountExpense:
		return true
	}
	return false
}

// NormalBalance is the side on which an account naturally grows.
type NormalBalance string

// Normal balance sides.
const (
	NormalDebit  NormalBalance = "debit"
	NormalCredit NormalBalance = "credit"
)

// Valid reports whether n is a known normal balance side.
func (n NormalBalance) Valid() bool { return n == NormalDebit || n == NormalCredit }

// Direction is the side of a posting.
type Direction string

// Posting directions.
const (
	DirectionDebit  Direction = "DEBIT"
	DirectionCredit Direction = "CREDIT"
)

// Valid reports whether d is DEBIT or CREDIT.
func (d Direction) Valid() bool { return d == DirectionDebit || d == DirectionCredit }

// Opposite returns the mirrored direction (DEBIT <-> CREDIT).
func (d Direction) Opposite() Direction {
	if d == DirectionDebit {
		return DirectionCredit
	}
	return DirectionDebit
}

// TransactionStatus is the lifecycle state of a transaction.
type TransactionStatus string

// Transaction lifecycle: draft -> posted -> reversed.
const (
	TransactionDraft    TransactionStatus = "draft"
	TransactionPosted   TransactionStatus = "posted"
	TransactionReversed TransactionStatus = "reversed"
)

// HoldStatus is the lifecycle state of a hold.
type HoldStatus string

// Hold lifecycle states.
const (
	HoldActive   HoldStatus = "active"
	HoldCaptured HoldStatus = "captured"
	HoldReleased HoldStatus = "released"
	HoldExpired  HoldStatus = "expired"
)

// ---- Entities ---------------------------------------------------------------

// Ledger is an isolated chart of accounts within a tenant.
type Ledger struct {
	ID          uuid.UUID         `json:"id"`
	TenantID    uuid.UUID         `json:"tenant_id"`
	Name        string            `json:"name"`
	Description string            `json:"description,omitempty"`
	Environment string            `json:"environment"` // "sandbox" | "live"
	Metadata    map[string]string `json:"metadata,omitempty"`
	CreatedAt   time.Time         `json:"created_at"`
}

// Account holds balances per asset and receives postings.
type Account struct {
	ID            uuid.UUID         `json:"id"`
	TenantID      uuid.UUID         `json:"tenant_id"`
	LedgerID      uuid.UUID         `json:"ledger_id"`
	Name          string            `json:"name"`
	Path          string            `json:"path"` // colon-namespaced, unique within the ledger
	Type          AccountType       `json:"type"`
	NormalBalance NormalBalance     `json:"normal_balance"`
	Status        string            `json:"status"` // "active" for now
	Metadata      map[string]string `json:"metadata,omitempty"`
	CreatedAt     time.Time         `json:"created_at"`
}

// Posting is one leg of a transaction: a signed movement on an account.
type Posting struct {
	ID        uuid.UUID    `json:"id"`
	AccountID uuid.UUID    `json:"account_id"`
	Direction Direction    `json:"direction"`
	Amount    money.Amount `json:"amount"`
}

// Transaction is a balanced set of postings.
type Transaction struct {
	ID             uuid.UUID         `json:"id"`
	TenantID       uuid.UUID         `json:"tenant_id"`
	LedgerID       uuid.UUID         `json:"ledger_id"`
	Reference      string            `json:"reference,omitempty"`
	Description    string            `json:"description,omitempty"`
	IdempotencyKey string            `json:"idempotency_key"`
	Status         TransactionStatus `json:"status"`
	EffectiveAt    time.Time         `json:"effective_at"`
	PostedAt       *time.Time        `json:"posted_at,omitempty"`
	ReversedByID   *uuid.UUID        `json:"reversed_by,omitempty"` // set on the original when reversed
	ReversesID     *uuid.UUID        `json:"reverses,omitempty"`    // set on the reversal, points at the original
	Metadata       map[string]string `json:"metadata,omitempty"`
	Postings       []Posting         `json:"postings"`
	CreatedAt      time.Time         `json:"created_at"`
}

// Entry is a posting as seen from an account's statement: the leg plus its
// transaction context.
type Entry struct {
	ID            uuid.UUID    `json:"id"`
	TransactionID uuid.UUID    `json:"transaction_id"`
	AccountID     uuid.UUID    `json:"account_id"`
	Direction     Direction    `json:"direction"`
	Amount        money.Amount `json:"amount"`
	CreatedAt     time.Time    `json:"created_at"`
}

// Hold reserves funds on an account until captured or released.
type Hold struct {
	ID                    uuid.UUID         `json:"id"`
	TenantID              uuid.UUID         `json:"tenant_id"`
	LedgerID              uuid.UUID         `json:"ledger_id"`
	AccountID             uuid.UUID         `json:"account_id"`
	IdempotencyKey        string            `json:"idempotency_key"`
	Amount                money.Amount      `json:"amount"`
	Status                HoldStatus        `json:"status"`
	ExpiresAt             time.Time         `json:"expires_at"`
	CapturedTransactionID *uuid.UUID        `json:"captured_transaction_id,omitempty"`
	Metadata              map[string]string `json:"metadata,omitempty"`
	CreatedAt             time.Time         `json:"created_at"`
}

// Balance is the per-asset running balance of an account, maintained
// transactionally alongside postings.
type Balance struct {
	Asset          string `json:"asset"`
	PostedDebits   int64  `json:"posted_debits"`
	PostedCredits  int64  `json:"posted_credits"`
	PendingDebits  int64  `json:"pending_debits"`
	PendingCredits int64  `json:"pending_credits"`
	Held           int64  `json:"held"`
	Version        int64  `json:"version"`
}

// PostedNet returns the net posted balance signed by the normal balance side.
func (b Balance) PostedNet(nb NormalBalance) int64 {
	if nb == NormalDebit {
		return b.PostedDebits - b.PostedCredits
	}
	return b.PostedCredits - b.PostedDebits
}

// PendingNet returns the net pending (draft) balance signed by the normal
// balance side.
func (b Balance) PendingNet(nb NormalBalance) int64 {
	if nb == NormalDebit {
		return b.PendingDebits - b.PendingCredits
	}
	return b.PendingCredits - b.PendingDebits
}

// Available is what can be spent right now: posted net minus active holds.
func (b Balance) Available(nb NormalBalance) int64 {
	return b.PostedNet(nb) - b.Held
}

// StatementEntry is one line of an account statement: a posting with its
// transaction context and the running balance after it.
type StatementEntry struct {
	PostingID     uuid.UUID
	TransactionID uuid.UUID
	Reference     string
	Direction     Direction
	Amount        money.Amount
	EffectiveAt   time.Time
	// Running is the balance after this entry, signed by the account's
	// normal balance side.
	Running int64
}

// AssetBalance is a per-asset signed balance snapshot.
type AssetBalance struct {
	Asset string
	Units int64 // signed by the account's normal balance side
}

// Statement is the account statement over a period: opening balance before
// From, the entries within [From, To] with running balances, and the closing
// balance at To.
type Statement struct {
	Account Account
	From    time.Time
	To      time.Time
	Opening []AssetBalance
	Closing []AssetBalance
	Entries []StatementEntry
}

// SignRaw converts a raw debit-minus-credit sum into a balance signed by the
// account's normal side: debit-normal accounts grow with debits, credit-normal
// accounts grow with credits.
func SignRaw(nb NormalBalance, raw int64) int64 {
	if nb == NormalDebit {
		return raw
	}
	return -raw
}

// RunningBalance composes the opening raw sum (debits-credits before the
// period) with the cumulative raw sum up to a statement line, signed by the
// account's normal balance.
func RunningBalance(nb NormalBalance, openingRaw, cumulativeRaw int64) int64 {
	return SignRaw(nb, openingRaw+cumulativeRaw)
}

// ---- Provider positions ------------------------------------------------------

// ProviderAccountBalance is one account/asset aggregate used to compute
// counterparty (provider) positions. Raw sums come from account_balances.
type ProviderAccountBalance struct {
	AccountID     uuid.UUID
	Path          string
	Type          AccountType
	Metadata      map[string]string
	Asset         string
	PostedDebits  int64
	PostedCredits int64
}

// ProviderAssetPosition is the net position against one provider in one asset.
type ProviderAssetPosition struct {
	Asset string
	// Net is the signed raw sum (debits - credits) across the provider's
	// accounts: positive means the counterparty holds value for us
	// (they_owe_us), negative means we owe them.
	Net      int64
	Accounts []uuid.UUID
}

// ProviderPosition groups the per-asset positions of one provider.
type ProviderPosition struct {
	Provider  string
	Positions []ProviderAssetPosition
}

// ProviderOf extracts the counterparty name of an account. metadata.provider
// wins as an explicit override; otherwise the account path must follow the
// convention <type>:provider:<name>:... (":" or "/" both accepted as
// separators, e.g. "assets:provider:thunes:usd" or
// "liabilities/provider/dlocal/mxn"). The bool reports whether the account
// identifies a provider at all.
func ProviderOf(path string, metadata map[string]string) (string, bool) {
	if p := strings.TrimSpace(metadata["provider"]); p != "" {
		return strings.ToLower(p), true
	}
	segs := strings.FieldsFunc(path, func(r rune) bool { return r == ':' || r == '/' })
	for i := 0; i+1 < len(segs); i++ {
		if strings.EqualFold(segs[i], "provider") && segs[i+1] != "" {
			return strings.ToLower(segs[i+1]), true
		}
	}
	return "", false
}

// TrialBalanceRow is one account/asset aggregate of the trial balance.
type TrialBalanceRow struct {
	AccountID   uuid.UUID
	AccountName string
	AccountType AccountType
	Asset       string
	Debits      int64
	Credits     int64
}

// TrialBalanceTotal is the per-asset grand total of the trial balance.
type TrialBalanceTotal struct {
	Asset    string
	Debits   int64
	Credits  int64
	Balanced bool
}

// TrialBalance is the ledger-wide accounting sanity report.
type TrialBalance struct {
	LedgerID uuid.UUID
	AsOf     time.Time
	Rows     []TrialBalanceRow
	Totals   []TrialBalanceTotal
}

// ---- Asset registry ----------------------------------------------------------

// DefaultAssetExponent is used for assets not present in the registry.
const DefaultAssetExponent = 2

// assetExponents maps asset codes to their display exponent (decimal places).
// Amounts are always stored in minor units; the exponent only matters at the
// presentation boundary.
var assetExponents = map[string]int{
	"USD": 2, "EUR": 2, "MXN": 2, "DOP": 2, "GTQ": 2, "COP": 2, "PEN": 2,
	"JPY": 0, "CLP": 0,
	"USDC": 6, "USDT": 6,
	"BTC": 8,
}

// AssetExponent returns the display exponent for an asset code.
func AssetExponent(code string) int {
	if exp, ok := assetExponents[code]; ok {
		return exp
	}
	return DefaultAssetExponent
}

// ---- Rules -------------------------------------------------------------------

// Domain rule violations.
var (
	ErrTooFewPostings    = errors.New("domain: a transaction requires at least two postings")
	ErrNonPositiveAmount = errors.New("domain: posting amounts must be greater than zero")
	ErrInvalidDirection  = errors.New("domain: posting direction must be DEBIT or CREDIT")
	ErrUnbalanced        = errors.New("domain: debits do not equal credits")
	ErrNotPosted         = errors.New("domain: only posted transactions can be reversed")
	ErrAlreadyReversed   = errors.New("domain: transaction is already reversed")
)

// ValidateBalanced enforces the central double-entry invariant over a set of
// postings: for EVERY asset, the sum of debits equals the sum of credits.
// It also requires at least two postings, strictly positive amounts, valid
// directions and asset codes, and detects int64 overflow while summing.
func ValidateBalanced(postings []Posting) error {
	if len(postings) < 2 {
		return fmt.Errorf("%w: got %d", ErrTooFewPostings, len(postings))
	}
	debits := make(map[string]money.Amount)
	credits := make(map[string]money.Amount)
	for i, p := range postings {
		if err := money.ValidateAssetCode(p.Amount.Asset); err != nil {
			return fmt.Errorf("posting %d: %w", i, err)
		}
		if p.Amount.Units <= 0 {
			return fmt.Errorf("%w: posting %d has amount %d", ErrNonPositiveAmount, i, p.Amount.Units)
		}
		var side map[string]money.Amount
		switch p.Direction {
		case DirectionDebit:
			side = debits
		case DirectionCredit:
			side = credits
		default:
			return fmt.Errorf("%w: posting %d has %q", ErrInvalidDirection, i, p.Direction)
		}
		current, ok := side[p.Amount.Asset]
		if !ok {
			current = money.Amount{Asset: p.Amount.Asset}
		}
		sum, err := current.Add(p.Amount)
		if err != nil {
			// money.ErrOverflow wrapped: the running sum exceeds int64.
			return fmt.Errorf("posting %d: %w", i, err)
		}
		side[p.Amount.Asset] = sum
	}
	for asset, d := range debits {
		c, ok := credits[asset]
		if !ok || c.Units != d.Units {
			return fmt.Errorf("%w: asset %s has debits %d and credits %d",
				ErrUnbalanced, asset, d.Units, c.Units)
		}
	}
	for asset, c := range credits {
		if _, ok := debits[asset]; !ok {
			return fmt.Errorf("%w: asset %s has credits %d and no debits",
				ErrUnbalanced, asset, c.Units)
		}
	}
	return nil
}

// ReversalReferencePrefix prefixes the reference and default idempotency key
// of reversal transactions.
const ReversalReferencePrefix = "reversal-of:"

// Reverse builds the mirror transaction that compensates a posted original:
// same postings with inverted directions, reference "reversal-of:<id>", and a
// link back to the original. The caller supplies the new transaction id and
// the clock so the function stays deterministic and pure.
func Reverse(original Transaction, reversalID uuid.UUID, now time.Time) (Transaction, error) {
	if original.ReversedByID != nil || original.Status == TransactionReversed {
		return Transaction{}, ErrAlreadyReversed
	}
	if original.Status != TransactionPosted {
		return Transaction{}, fmt.Errorf("%w: status is %q", ErrNotPosted, original.Status)
	}
	originalID := original.ID
	postedAt := now
	rev := Transaction{
		ID:             reversalID,
		TenantID:       original.TenantID,
		LedgerID:       original.LedgerID,
		Reference:      ReversalReferencePrefix + originalID.String(),
		IdempotencyKey: ReversalReferencePrefix + originalID.String(),
		Status:         TransactionPosted,
		EffectiveAt:    now,
		PostedAt:       &postedAt,
		ReversesID:     &originalID,
		CreatedAt:      now,
		Postings:       make([]Posting, len(original.Postings)),
	}
	for i, p := range original.Postings {
		id, err := uuid.NewV7()
		if err != nil {
			return Transaction{}, fmt.Errorf("domain: generate posting id: %w", err)
		}
		rev.Postings[i] = Posting{
			ID:        id,
			AccountID: p.AccountID,
			Direction: p.Direction.Opposite(),
			Amount:    p.Amount,
		}
	}
	return rev, nil
}
