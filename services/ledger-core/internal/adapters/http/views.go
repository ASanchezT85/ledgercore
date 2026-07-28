package httpapi

import (
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/ledgercore/ledgercore/libs/go/money"

	"github.com/ledgercore/ledgercore/services/ledger-core/internal/domain"
)

// Wire representations following contracts/openapi/ledger.v1.yaml: amounts
// are string-encoded int64 minor units and normal_balance uses DEBIT/CREDIT.

type moneyView struct {
	Asset  string `json:"asset"`
	Amount string `json:"amount"`
}

func newMoneyView(a money.Amount) moneyView {
	return moneyView{Asset: a.Asset, Amount: strconv.FormatInt(a.Units, 10)}
}

// moneyBody is the request shape of a monetary amount.
type moneyBody struct {
	Asset  string `json:"asset"`
	Amount string `json:"amount"`
}

func (m moneyBody) toAmount() (money.Amount, error) {
	units, err := strconv.ParseInt(strings.TrimSpace(m.Amount), 10, 64)
	if err != nil {
		return money.Amount{}, err
	}
	return money.Amount{Asset: m.Asset, Units: units}, nil
}

func normalBalanceOut(nb domain.NormalBalance) string {
	if nb == domain.NormalDebit {
		return string(domain.DirectionDebit)
	}
	return string(domain.DirectionCredit)
}

func normalBalanceIn(s string) (domain.NormalBalance, bool) {
	switch strings.ToUpper(s) {
	case string(domain.DirectionDebit):
		return domain.NormalDebit, true
	case string(domain.DirectionCredit):
		return domain.NormalCredit, true
	}
	return "", false
}

type ledgerView struct {
	ID          uuid.UUID         `json:"id"`
	Name        string            `json:"name"`
	Description string            `json:"description,omitempty"`
	Environment string            `json:"environment"`
	Metadata    map[string]string `json:"metadata"`
	CreatedAt   time.Time         `json:"created_at"`
}

func newLedgerView(l domain.Ledger) ledgerView {
	return ledgerView{
		ID:          l.ID,
		Name:        l.Name,
		Description: l.Description,
		Environment: l.Environment,
		Metadata:    orEmpty(l.Metadata),
		CreatedAt:   l.CreatedAt,
	}
}

type accountView struct {
	ID            uuid.UUID         `json:"id"`
	LedgerID      uuid.UUID         `json:"ledger_id"`
	Name          string            `json:"name"`
	Type          string            `json:"type"`
	NormalBalance string            `json:"normal_balance"`
	Status        string            `json:"status"`
	Metadata      map[string]string `json:"metadata"`
	CreatedAt     time.Time         `json:"created_at"`
}

func newAccountView(a domain.Account) accountView {
	return accountView{
		ID:            a.ID,
		LedgerID:      a.LedgerID,
		Name:          a.Name,
		Type:          string(a.Type),
		NormalBalance: normalBalanceOut(a.NormalBalance),
		Status:        a.Status,
		Metadata:      orEmpty(a.Metadata),
		CreatedAt:     a.CreatedAt,
	}
}

type balanceView struct {
	Asset     string `json:"asset"`
	Exponent  int    `json:"exponent"`
	Posted    string `json:"posted"`
	Pending   string `json:"pending"`
	Available string `json:"available"`
	Held      string `json:"held"`
	Debits    string `json:"posted_debits"`
	Credits   string `json:"posted_credits"`
	Version   int64  `json:"version"`
}

func newBalanceView(b domain.Balance, nb domain.NormalBalance) balanceView {
	return balanceView{
		Asset:     b.Asset,
		Exponent:  domain.AssetExponent(b.Asset),
		Posted:    strconv.FormatInt(b.PostedNet(nb), 10),
		Pending:   strconv.FormatInt(b.PendingNet(nb), 10),
		Available: strconv.FormatInt(b.Available(nb), 10),
		Held:      strconv.FormatInt(b.Held, 10),
		Debits:    strconv.FormatInt(b.PostedDebits, 10),
		Credits:   strconv.FormatInt(b.PostedCredits, 10),
		Version:   b.Version,
	}
}

type postingView struct {
	ID        uuid.UUID `json:"id"`
	AccountID uuid.UUID `json:"account_id"`
	Direction string    `json:"direction"`
	Amount    moneyView `json:"amount"`
}

type transactionView struct {
	ID             uuid.UUID         `json:"id"`
	LedgerID       uuid.UUID         `json:"ledger_id"`
	IdempotencyKey string            `json:"idempotency_key"`
	Reference      string            `json:"reference,omitempty"`
	Description    string            `json:"description,omitempty"`
	Status         string            `json:"status"`
	Postings       []postingView     `json:"postings"`
	Metadata       map[string]string `json:"metadata"`
	ReversedBy     *uuid.UUID        `json:"reversed_by"`
	Reverses       *uuid.UUID        `json:"reverses"`
	EffectiveAt    time.Time         `json:"effective_at"`
	CreatedAt      time.Time         `json:"created_at"`
	PostedAt       *time.Time        `json:"posted_at"`
}

func newTransactionView(t domain.Transaction) transactionView {
	postings := make([]postingView, len(t.Postings))
	for i, p := range t.Postings {
		postings[i] = postingView{
			ID:        p.ID,
			AccountID: p.AccountID,
			Direction: string(p.Direction),
			Amount:    newMoneyView(p.Amount),
		}
	}
	return transactionView{
		ID:             t.ID,
		LedgerID:       t.LedgerID,
		IdempotencyKey: t.IdempotencyKey,
		Reference:      t.Reference,
		Description:    t.Description,
		Status:         string(t.Status),
		Postings:       postings,
		Metadata:       orEmpty(t.Metadata),
		ReversedBy:     t.ReversedByID,
		Reverses:       t.ReversesID,
		EffectiveAt:    t.EffectiveAt,
		CreatedAt:      t.CreatedAt,
		PostedAt:       t.PostedAt,
	}
}

type entryView struct {
	ID            uuid.UUID `json:"id"`
	TransactionID uuid.UUID `json:"transaction_id"`
	AccountID     uuid.UUID `json:"account_id"`
	Direction     string    `json:"direction"`
	Amount        moneyView `json:"amount"`
	CreatedAt     time.Time `json:"created_at"`
}

func newEntryView(e domain.Entry) entryView {
	return entryView{
		ID:            e.ID,
		TransactionID: e.TransactionID,
		AccountID:     e.AccountID,
		Direction:     string(e.Direction),
		Amount:        newMoneyView(e.Amount),
		CreatedAt:     e.CreatedAt,
	}
}

type holdView struct {
	ID             uuid.UUID         `json:"id"`
	LedgerID       uuid.UUID         `json:"ledger_id"`
	AccountID      uuid.UUID         `json:"account_id"`
	IdempotencyKey string            `json:"idempotency_key"`
	Amount         moneyView         `json:"amount"`
	Status         string            `json:"status"`
	TransactionID  *uuid.UUID        `json:"transaction_id"`
	ExpiresAt      time.Time         `json:"expires_at"`
	Metadata       map[string]string `json:"metadata"`
	CreatedAt      time.Time         `json:"created_at"`
}

func newHoldView(h domain.Hold) holdView {
	return holdView{
		ID:             h.ID,
		LedgerID:       h.LedgerID,
		AccountID:      h.AccountID,
		IdempotencyKey: h.IdempotencyKey,
		Amount:         newMoneyView(h.Amount),
		Status:         string(h.Status),
		TransactionID:  h.CapturedTransactionID,
		ExpiresAt:      h.ExpiresAt,
		Metadata:       orEmpty(h.Metadata),
		CreatedAt:      h.CreatedAt,
	}
}

type trialBalanceRowView struct {
	AccountID   uuid.UUID `json:"account_id"`
	AccountName string    `json:"account_name"`
	AccountType string    `json:"account_type"`
	Asset       string    `json:"asset"`
	Debits      string    `json:"debits"`
	Credits     string    `json:"credits"`
	Net         string    `json:"net"`
}

type trialBalanceTotalView struct {
	Asset    string `json:"asset"`
	Debits   string `json:"debits"`
	Credits  string `json:"credits"`
	Balanced bool   `json:"balanced"`
}

type trialBalanceView struct {
	LedgerID uuid.UUID               `json:"ledger_id"`
	AsOf     time.Time               `json:"as_of"`
	Rows     []trialBalanceRowView   `json:"rows"`
	Totals   []trialBalanceTotalView `json:"totals"`
}

func newTrialBalanceView(tb domain.TrialBalance) trialBalanceView {
	out := trialBalanceView{
		LedgerID: tb.LedgerID,
		AsOf:     tb.AsOf,
		Rows:     make([]trialBalanceRowView, len(tb.Rows)),
		Totals:   make([]trialBalanceTotalView, len(tb.Totals)),
	}
	for i, r := range tb.Rows {
		out.Rows[i] = trialBalanceRowView{
			AccountID:   r.AccountID,
			AccountName: r.AccountName,
			AccountType: string(r.AccountType),
			Asset:       r.Asset,
			Debits:      strconv.FormatInt(r.Debits, 10),
			Credits:     strconv.FormatInt(r.Credits, 10),
			Net:         strconv.FormatInt(r.Debits-r.Credits, 10),
		}
	}
	for i, t := range tb.Totals {
		out.Totals[i] = trialBalanceTotalView{
			Asset:    t.Asset,
			Debits:   strconv.FormatInt(t.Debits, 10),
			Credits:  strconv.FormatInt(t.Credits, 10),
			Balanced: t.Balanced,
		}
	}
	return out
}

// ---- statements ----------------------------------------------------------------

type assetBalanceView struct {
	Asset    string `json:"asset"`
	Exponent int    `json:"exponent"`
	Amount   string `json:"amount"`
}

func newAssetBalanceViews(balances []domain.AssetBalance) []assetBalanceView {
	out := make([]assetBalanceView, len(balances))
	for i, b := range balances {
		out[i] = assetBalanceView{
			Asset:    b.Asset,
			Exponent: domain.AssetExponent(b.Asset),
			Amount:   strconv.FormatInt(b.Units, 10),
		}
	}
	return out
}

type statementEntryView struct {
	ID             uuid.UUID `json:"id"`
	TransactionID  uuid.UUID `json:"transaction_id"`
	Reference      string    `json:"reference,omitempty"`
	Direction      string    `json:"direction"`
	Amount         moneyView `json:"amount"`
	EffectiveAt    time.Time `json:"effective_at"`
	RunningBalance string    `json:"running_balance"`
}

type statementPeriodView struct {
	From time.Time `json:"from"`
	To   time.Time `json:"to"`
}

type statementView struct {
	Account        accountView          `json:"account"`
	Period         statementPeriodView  `json:"period"`
	OpeningBalance []assetBalanceView   `json:"opening_balance"`
	Entries        []statementEntryView `json:"entries"`
	ClosingBalance []assetBalanceView   `json:"closing_balance"`
	NextCursor     *string              `json:"next_cursor"`
}

func newStatementView(st domain.Statement, limit int) statementView {
	out := statementView{
		Account:        newAccountView(st.Account),
		Period:         statementPeriodView{From: st.From, To: st.To},
		OpeningBalance: newAssetBalanceViews(st.Opening),
		ClosingBalance: newAssetBalanceViews(st.Closing),
		Entries:        make([]statementEntryView, len(st.Entries)),
	}
	for i, e := range st.Entries {
		out.Entries[i] = statementEntryView{
			ID:             e.PostingID,
			TransactionID:  e.TransactionID,
			Reference:      e.Reference,
			Direction:      string(e.Direction),
			Amount:         newMoneyView(e.Amount),
			EffectiveAt:    e.EffectiveAt,
			RunningBalance: strconv.FormatInt(e.Running, 10),
		}
	}
	if n := len(st.Entries); n > 0 {
		last := st.Entries[n-1]
		out.NextCursor = nextCursor(n, limit, last.EffectiveAt, last.PostingID)
	}
	return out
}

// ---- provider positions --------------------------------------------------------

type providerAssetPositionView struct {
	Asset     string      `json:"asset"`
	Exponent  int         `json:"exponent"`
	Net       string      `json:"net"`
	Direction string      `json:"direction"` // they_owe_us | we_owe_them | flat
	Accounts  []uuid.UUID `json:"accounts"`
}

type providerPositionView struct {
	Provider  string                      `json:"provider"`
	Positions []providerAssetPositionView `json:"positions"`
}

type providerPositionsResponse struct {
	Data []providerPositionView `json:"data"`
	Hint string                 `json:"hint,omitempty"`
}

// providerPositionsHint guides tenants that do not follow the naming
// convention: without it (or the metadata override) the report is empty.
const providerPositionsHint = "no provider accounts found: name accounts as " +
	"assets:provider:<name>:... (or liabilities:provider:<name>:...) or set metadata.provider on the account"

func positionDirection(net int64) string {
	switch {
	case net > 0:
		return "they_owe_us"
	case net < 0:
		return "we_owe_them"
	default:
		return "flat"
	}
}

func newProviderPositionsView(positions []domain.ProviderPosition) providerPositionsResponse {
	out := providerPositionsResponse{Data: make([]providerPositionView, len(positions))}
	for i, p := range positions {
		v := providerPositionView{
			Provider:  p.Provider,
			Positions: make([]providerAssetPositionView, len(p.Positions)),
		}
		for j, pos := range p.Positions {
			v.Positions[j] = providerAssetPositionView{
				Asset:     pos.Asset,
				Exponent:  domain.AssetExponent(pos.Asset),
				Net:       strconv.FormatInt(pos.Net, 10),
				Direction: positionDirection(pos.Net),
				Accounts:  pos.Accounts,
			}
		}
		out.Data[i] = v
	}
	if len(out.Data) == 0 {
		out.Hint = providerPositionsHint
	}
	return out
}

// listResponse is the standard paginated collection shape.
type listResponse[T any] struct {
	Data       []T     `json:"data"`
	NextCursor *string `json:"next_cursor"`
}

func orEmpty(m map[string]string) map[string]string {
	if m == nil {
		return map[string]string{}
	}
	return m
}
