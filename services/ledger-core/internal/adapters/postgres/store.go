package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ledgercore/ledgercore/libs/go/events"
	"github.com/ledgercore/ledgercore/libs/go/httpx"
	"github.com/ledgercore/ledgercore/libs/go/money"
	"github.com/ledgercore/ledgercore/libs/go/pgxutil"

	"github.com/ledgercore/ledgercore/services/ledger-core/internal/app"
	"github.com/ledgercore/ledgercore/services/ledger-core/internal/domain"
)

// Store implements app.Store on PostgreSQL. Every method opens a single
// tenant-scoped transaction (RLS via SET LOCAL app.tenant_id), so multi-step
// operations are atomic and cross-tenant access is impossible.
type Store struct {
	pool *pgxpool.Pool
}

// Compile-time check that Store satisfies the application port.
var _ app.Store = (*Store)(nil)

// NewStore wraps a pgx pool (search_path=ledger) as the persistence adapter.
func NewStore(pool *pgxpool.Pool) *Store {
	return &Store{pool: pool}
}

// ---- shared helpers -----------------------------------------------------------

// mapErr converts PostgreSQL error classes into application sentinels.
func mapErr(err error) error {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		switch pgErr.Code {
		case "23505": // unique_violation
			return fmt.Errorf("%w: %s", app.ErrConflict, pgErr.ConstraintName)
		case "23503": // foreign_key_violation
			return fmt.Errorf("%w: %s", app.ErrNotFound, pgErr.ConstraintName)
		case "23514": // check_violation
			return fmt.Errorf("%w: %s", app.ErrValidation, pgErr.ConstraintName)
		}
	}
	return err
}

func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}

// meta normalizes nil metadata so jsonb columns always store an object.
func meta(m map[string]string) map[string]string {
	if m == nil {
		return map[string]string{}
	}
	return m
}

// cursorArgs turns a keyset cursor into SQL arguments (NULLs on first page).
func cursorArgs(c httpx.Cursor) (any, any) {
	if c.IsZero() {
		return nil, nil
	}
	return c.CreatedAt, c.ID
}

func normalizeLimit(limit int) int {
	switch {
	case limit <= 0:
		return 25
	case limit > 100:
		return 100
	default:
		return limit
	}
}

// insertOutbox serializes the event envelope and appends it to the outbox
// inside the caller's transaction: same commit as the business change.
func insertOutbox(ctx context.Context, tx pgx.Tx, topic string, tenantID uuid.UUID, payload any) error {
	env, err := events.NewEnvelope(topic, tenantID, payload)
	if err != nil {
		return err
	}
	body, err := json.Marshal(env)
	if err != nil {
		return fmt.Errorf("marshal envelope: %w", err)
	}
	_, err = tx.Exec(ctx,
		`INSERT INTO outbox (tenant_id, topic, envelope) VALUES ($1, $2, $3)`,
		tenantID, topic, body)
	if err != nil {
		return fmt.Errorf("insert outbox: %w", err)
	}
	return nil
}

// ---- ledgers ------------------------------------------------------------------

// CreateLedger inserts a ledger.
func (s *Store) CreateLedger(ctx context.Context, l domain.Ledger) error {
	err := pgxutil.WithTenantTx(ctx, s.pool, l.TenantID, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `
			INSERT INTO ledgers (id, tenant_id, name, description, environment, metadata, created_at)
			VALUES ($1, $2, $3, $4, $5, $6, $7)`,
			l.ID, l.TenantID, l.Name, l.Description, l.Environment, meta(l.Metadata), l.CreatedAt)
		return err
	})
	return mapErr(err)
}

const ledgerColumns = `id, tenant_id, name, description, environment, metadata, created_at`

func scanLedger(row pgx.Row) (domain.Ledger, error) {
	var l domain.Ledger
	err := row.Scan(&l.ID, &l.TenantID, &l.Name, &l.Description, &l.Environment, &l.Metadata, &l.CreatedAt)
	return l, err
}

// GetLedger fetches one ledger; RLS guarantees tenant scoping.
func (s *Store) GetLedger(ctx context.Context, tenantID, id uuid.UUID) (domain.Ledger, error) {
	var out domain.Ledger
	err := pgxutil.WithTenantTx(ctx, s.pool, tenantID, func(tx pgx.Tx) error {
		l, err := scanLedger(tx.QueryRow(ctx,
			`SELECT `+ledgerColumns+` FROM ledgers WHERE id = $1`, id))
		if errors.Is(err, pgx.ErrNoRows) {
			return fmt.Errorf("ledger %s: %w", id, app.ErrNotFound)
		}
		out = l
		return err
	})
	return out, err
}

// ListLedgers pages through ledgers, newest first.
func (s *Store) ListLedgers(ctx context.Context, tenantID uuid.UUID, page app.Page) ([]domain.Ledger, error) {
	cAt, cID := cursorArgs(page.Cursor)
	var out []domain.Ledger
	err := pgxutil.WithTenantTx(ctx, s.pool, tenantID, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `
			SELECT `+ledgerColumns+`
			FROM ledgers
			WHERE ($1::timestamptz IS NULL OR (created_at, id) < ($1, $2::uuid))
			ORDER BY created_at DESC, id DESC
			LIMIT $3`, cAt, cID, normalizeLimit(page.Limit))
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			l, err := scanLedger(rows)
			if err != nil {
				return err
			}
			out = append(out, l)
		}
		return rows.Err()
	})
	return out, err
}

// ---- accounts -----------------------------------------------------------------

// CreateAccount inserts an account after checking that its ledger exists.
func (s *Store) CreateAccount(ctx context.Context, a domain.Account) error {
	err := pgxutil.WithTenantTx(ctx, s.pool, a.TenantID, func(tx pgx.Tx) error {
		var one int
		err := tx.QueryRow(ctx, `SELECT 1 FROM ledgers WHERE id = $1`, a.LedgerID).Scan(&one)
		if errors.Is(err, pgx.ErrNoRows) {
			return fmt.Errorf("ledger %s: %w", a.LedgerID, app.ErrNotFound)
		}
		if err != nil {
			return err
		}
		_, err = tx.Exec(ctx, `
			INSERT INTO accounts (id, tenant_id, ledger_id, name, path, type, normal_balance, status, metadata, created_at)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)`,
			a.ID, a.TenantID, a.LedgerID, a.Name, a.Path, string(a.Type), string(a.NormalBalance),
			a.Status, meta(a.Metadata), a.CreatedAt)
		return err
	})
	return mapErr(err)
}

const accountColumns = `id, tenant_id, ledger_id, name, path, type, normal_balance, status, metadata, created_at`

func scanAccount(row pgx.Row) (domain.Account, error) {
	var a domain.Account
	var typ, nb string
	err := row.Scan(&a.ID, &a.TenantID, &a.LedgerID, &a.Name, &a.Path, &typ, &nb, &a.Status, &a.Metadata, &a.CreatedAt)
	a.Type = domain.AccountType(typ)
	a.NormalBalance = domain.NormalBalance(nb)
	return a, err
}

func getAccountTx(ctx context.Context, tx pgx.Tx, id uuid.UUID) (domain.Account, error) {
	a, err := scanAccount(tx.QueryRow(ctx,
		`SELECT `+accountColumns+` FROM accounts WHERE id = $1`, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Account{}, fmt.Errorf("account %s: %w", id, app.ErrNotFound)
	}
	return a, err
}

// GetAccount fetches one account.
func (s *Store) GetAccount(ctx context.Context, tenantID, id uuid.UUID) (domain.Account, error) {
	var out domain.Account
	err := pgxutil.WithTenantTx(ctx, s.pool, tenantID, func(tx pgx.Tx) error {
		a, err := getAccountTx(ctx, tx, id)
		out = a
		return err
	})
	return out, err
}

// ListAccounts pages through accounts, newest first, optionally filtered by
// ledger and type.
func (s *Store) ListAccounts(ctx context.Context, tenantID uuid.UUID, filter app.AccountFilter, page app.Page) ([]domain.Account, error) {
	cAt, cID := cursorArgs(page.Cursor)
	var ledgerID any
	if filter.LedgerID != nil {
		ledgerID = *filter.LedgerID
	}
	var typ any
	if filter.Type != nil {
		typ = string(*filter.Type)
	}
	var out []domain.Account
	err := pgxutil.WithTenantTx(ctx, s.pool, tenantID, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `
			SELECT `+accountColumns+`
			FROM accounts
			WHERE ($1::uuid IS NULL OR ledger_id = $1)
			  AND ($2::text IS NULL OR type = $2)
			  AND ($3::timestamptz IS NULL OR (created_at, id) < ($3, $4::uuid))
			ORDER BY created_at DESC, id DESC
			LIMIT $5`, ledgerID, typ, cAt, cID, normalizeLimit(page.Limit))
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			a, err := scanAccount(rows)
			if err != nil {
				return err
			}
			out = append(out, a)
		}
		return rows.Err()
	})
	return out, err
}

// GetBalances returns the account plus one Balance row per asset.
func (s *Store) GetBalances(ctx context.Context, tenantID, accountID uuid.UUID) (domain.Account, []domain.Balance, error) {
	var account domain.Account
	var balances []domain.Balance
	err := pgxutil.WithTenantTx(ctx, s.pool, tenantID, func(tx pgx.Tx) error {
		a, err := getAccountTx(ctx, tx, accountID)
		if err != nil {
			return err
		}
		account = a
		rows, err := tx.Query(ctx, `
			SELECT asset, posted_debits, posted_credits, pending_debits, pending_credits, held, version
			FROM account_balances
			WHERE account_id = $1
			ORDER BY asset`, accountID)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var b domain.Balance
			if err := rows.Scan(&b.Asset, &b.PostedDebits, &b.PostedCredits,
				&b.PendingDebits, &b.PendingCredits, &b.Held, &b.Version); err != nil {
				return err
			}
			balances = append(balances, b)
		}
		return rows.Err()
	})
	return account, balances, err
}

// ListEntries pages through the postings that hit an account, newest first.
func (s *Store) ListEntries(ctx context.Context, tenantID, accountID uuid.UUID, asset string, page app.Page) ([]domain.Entry, error) {
	cAt, cID := cursorArgs(page.Cursor)
	var assetArg any
	if asset != "" {
		assetArg = asset
	}
	var out []domain.Entry
	err := pgxutil.WithTenantTx(ctx, s.pool, tenantID, func(tx pgx.Tx) error {
		if _, err := getAccountTx(ctx, tx, accountID); err != nil {
			return err
		}
		rows, err := tx.Query(ctx, `
			SELECT id, transaction_id, account_id, direction, asset, amount, created_at
			FROM postings
			WHERE account_id = $1
			  AND ($2::varchar IS NULL OR asset = $2)
			  AND ($3::timestamptz IS NULL OR (created_at, id) < ($3, $4::uuid))
			ORDER BY created_at DESC, id DESC
			LIMIT $5`, accountID, assetArg, cAt, cID, normalizeLimit(page.Limit))
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var e domain.Entry
			var dir string
			var units int64
			var assetCode string
			if err := rows.Scan(&e.ID, &e.TransactionID, &e.AccountID, &dir, &assetCode, &units, &e.CreatedAt); err != nil {
				return err
			}
			e.Direction = domain.Direction(dir)
			e.Amount = money.Amount{Asset: assetCode, Units: units}
			out = append(out, e)
		}
		return rows.Err()
	})
	return out, err
}

// ---- balance application --------------------------------------------------------

const upsertPostedBalance = `
	INSERT INTO account_balances (account_id, tenant_id, asset, posted_debits, posted_credits, version)
	VALUES ($1, $2, $3, $4, $5, 1)
	ON CONFLICT (account_id, asset) DO UPDATE SET
		posted_debits  = account_balances.posted_debits  + EXCLUDED.posted_debits,
		posted_credits = account_balances.posted_credits + EXCLUDED.posted_credits,
		version        = account_balances.version + 1,
		updated_at     = now()`

const upsertPendingBalance = `
	INSERT INTO account_balances (account_id, tenant_id, asset, pending_debits, pending_credits, version)
	VALUES ($1, $2, $3, $4, $5, 1)
	ON CONFLICT (account_id, asset) DO UPDATE SET
		pending_debits  = account_balances.pending_debits  + EXCLUDED.pending_debits,
		pending_credits = account_balances.pending_credits + EXCLUDED.pending_credits,
		version         = account_balances.version + 1,
		updated_at      = now()`

const movePendingToPosted = `
	UPDATE account_balances SET
		posted_debits   = posted_debits   + $1,
		posted_credits  = posted_credits  + $2,
		pending_debits  = pending_debits  - $1,
		pending_credits = pending_credits - $2,
		version         = version + 1,
		updated_at      = now()
	WHERE account_id = $3 AND asset = $4`

func debitCredit(p domain.Posting) (debit, credit int64) {
	if p.Direction == domain.DirectionDebit {
		return p.Amount.Units, 0
	}
	return 0, p.Amount.Units
}

// applyBalances upserts account_balances for every posting using the given
// statement (posted or pending columns), inside the business transaction.
func applyBalances(ctx context.Context, tx pgx.Tx, tenantID uuid.UUID, postings []domain.Posting, stmt string) error {
	batch := &pgx.Batch{}
	for _, p := range postings {
		d, c := debitCredit(p)
		batch.Queue(stmt, p.AccountID, tenantID, p.Amount.Asset, d, c)
	}
	return tx.SendBatch(ctx, batch).Close()
}

// promoteBalances moves draft (pending) amounts into posted for every posting.
func promoteBalances(ctx context.Context, tx pgx.Tx, postings []domain.Posting) error {
	batch := &pgx.Batch{}
	for _, p := range postings {
		d, c := debitCredit(p)
		batch.Queue(movePendingToPosted, d, c, p.AccountID, p.Amount.Asset)
	}
	return tx.SendBatch(ctx, batch).Close()
}

// ---- transactions ----------------------------------------------------------------

const transactionColumns = `id, tenant_id, ledger_id, reference, description, idempotency_key,
	status, effective_at, posted_at, reversed_by_id, reverses_id, metadata, created_at`

func scanTransaction(row pgx.Row) (domain.Transaction, error) {
	var t domain.Transaction
	var status string
	err := row.Scan(&t.ID, &t.TenantID, &t.LedgerID, &t.Reference, &t.Description, &t.IdempotencyKey,
		&status, &t.EffectiveAt, &t.PostedAt, &t.ReversedByID, &t.ReversesID, &t.Metadata, &t.CreatedAt)
	t.Status = domain.TransactionStatus(status)
	return t, err
}

func loadPostings(ctx context.Context, tx pgx.Tx, transactionIDs []uuid.UUID) (map[uuid.UUID][]domain.Posting, error) {
	rows, err := tx.Query(ctx, `
		SELECT id, transaction_id, account_id, direction, asset, amount
		FROM postings
		WHERE transaction_id = ANY($1)
		ORDER BY created_at, id`, transactionIDs)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make(map[uuid.UUID][]domain.Posting, len(transactionIDs))
	for rows.Next() {
		var p domain.Posting
		var txID uuid.UUID
		var dir, asset string
		var units int64
		if err := rows.Scan(&p.ID, &txID, &p.AccountID, &dir, &asset, &units); err != nil {
			return nil, err
		}
		p.Direction = domain.Direction(dir)
		p.Amount = money.Amount{Asset: asset, Units: units}
		out[txID] = append(out[txID], p)
	}
	return out, rows.Err()
}

func loadTransactionTx(ctx context.Context, tx pgx.Tx, id uuid.UUID, forUpdate bool) (domain.Transaction, error) {
	q := `SELECT ` + transactionColumns + ` FROM transactions WHERE id = $1`
	if forUpdate {
		q += ` FOR UPDATE`
	}
	t, err := scanTransaction(tx.QueryRow(ctx, q, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Transaction{}, fmt.Errorf("transaction %s: %w", id, app.ErrNotFound)
	}
	if err != nil {
		return domain.Transaction{}, err
	}
	postings, err := loadPostings(ctx, tx, []uuid.UUID{id})
	if err != nil {
		return domain.Transaction{}, err
	}
	t.Postings = postings[id]
	return t, nil
}

func insertTransactionTx(ctx context.Context, tx pgx.Tx, t domain.Transaction) error {
	_, err := tx.Exec(ctx, `
		INSERT INTO transactions (id, tenant_id, ledger_id, reference, description, idempotency_key,
			status, effective_at, posted_at, reversed_by_id, reverses_id, metadata, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)`,
		t.ID, t.TenantID, t.LedgerID, t.Reference, t.Description, t.IdempotencyKey,
		string(t.Status), t.EffectiveAt, t.PostedAt, t.ReversedByID, t.ReversesID, meta(t.Metadata), t.CreatedAt)
	if err != nil {
		return err
	}
	batch := &pgx.Batch{}
	for _, p := range t.Postings {
		batch.Queue(`
			INSERT INTO postings (id, tenant_id, transaction_id, account_id, direction, asset, amount, created_at)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`,
			p.ID, t.TenantID, t.ID, p.AccountID, string(p.Direction), p.Amount.Asset, p.Amount.Units, t.CreatedAt)
	}
	return tx.SendBatch(ctx, batch).Close()
}

// checkAccountsInLedger verifies that every posting account exists and
// belongs to the transaction's ledger.
func checkAccountsInLedger(ctx context.Context, tx pgx.Tx, ledgerID uuid.UUID, postings []domain.Posting) error {
	ids := make([]uuid.UUID, 0, len(postings))
	seen := make(map[uuid.UUID]bool, len(postings))
	for _, p := range postings {
		if !seen[p.AccountID] {
			seen[p.AccountID] = true
			ids = append(ids, p.AccountID)
		}
	}
	rows, err := tx.Query(ctx, `SELECT id, ledger_id FROM accounts WHERE id = ANY($1)`, ids)
	if err != nil {
		return err
	}
	defer rows.Close()
	found := make(map[uuid.UUID]uuid.UUID, len(ids))
	for rows.Next() {
		var id, lid uuid.UUID
		if err := rows.Scan(&id, &lid); err != nil {
			return err
		}
		found[id] = lid
	}
	if err := rows.Err(); err != nil {
		return err
	}
	for _, id := range ids {
		lid, ok := found[id]
		if !ok {
			return fmt.Errorf("account %s: %w", id, app.ErrNotFound)
		}
		if lid != ledgerID {
			return fmt.Errorf("%w: account %s belongs to another ledger", app.ErrValidation, id)
		}
	}
	return nil
}

func storeIdempotentResponse(ctx context.Context, tx pgx.Tx, tenantID uuid.UUID, key string, statusCode int, t domain.Transaction) error {
	body, err := json.Marshal(t)
	if err != nil {
		return fmt.Errorf("marshal idempotent response: %w", err)
	}
	_, err = tx.Exec(ctx, `
		INSERT INTO idempotency_keys (tenant_id, idempotency_key, status_code, response)
		VALUES ($1, $2, $3, $4)`, tenantID, key, statusCode, body)
	return err
}

// lookupIdempotent returns the stored transaction for a key, if any.
func lookupIdempotentTx(ctx context.Context, tx pgx.Tx, key string) (domain.Transaction, bool, error) {
	var raw []byte
	err := tx.QueryRow(ctx,
		`SELECT response FROM idempotency_keys WHERE idempotency_key = $1`, key).Scan(&raw)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Transaction{}, false, nil
	}
	if err != nil {
		return domain.Transaction{}, false, err
	}
	var t domain.Transaction
	if err := json.Unmarshal(raw, &t); err != nil {
		return domain.Transaction{}, false, fmt.Errorf("unmarshal stored response: %w", err)
	}
	return t, true, nil
}

// CreateTransaction persists the transaction atomically: idempotency check,
// domain rows, balance application, outbox event and idempotency record all
// commit together or not at all.
func (s *Store) CreateTransaction(ctx context.Context, t domain.Transaction) (domain.Transaction, bool, error) {
	var out domain.Transaction
	replayed := false
	err := pgxutil.WithTenantTx(ctx, s.pool, t.TenantID, func(tx pgx.Tx) error {
		// 1. Idempotency first: an existing key returns the stored response.
		stored, ok, err := lookupIdempotentTx(ctx, tx, t.IdempotencyKey)
		if err != nil {
			return err
		}
		if ok {
			out, replayed = stored, true
			return nil
		}
		// 2. Referential checks.
		var one int
		err = tx.QueryRow(ctx, `SELECT 1 FROM ledgers WHERE id = $1`, t.LedgerID).Scan(&one)
		if errors.Is(err, pgx.ErrNoRows) {
			return fmt.Errorf("ledger %s: %w", t.LedgerID, app.ErrNotFound)
		}
		if err != nil {
			return err
		}
		if err := checkAccountsInLedger(ctx, tx, t.LedgerID, t.Postings); err != nil {
			return err
		}
		// 3. Transaction + postings.
		if err := insertTransactionTx(ctx, tx, t); err != nil {
			return err
		}
		// 4. Balances + outbox when posted; pending when draft.
		if t.Status == domain.TransactionPosted {
			if err := applyBalances(ctx, tx, t.TenantID, t.Postings, upsertPostedBalance); err != nil {
				return err
			}
			postedAt := t.CreatedAt
			if t.PostedAt != nil {
				postedAt = *t.PostedAt
			}
			payload := app.NewTransactionPostedEvent(t, postedAt)
			if err := insertOutbox(ctx, tx, events.TopicTransactionPosted, t.TenantID, payload); err != nil {
				return err
			}
		} else {
			if err := applyBalances(ctx, tx, t.TenantID, t.Postings, upsertPendingBalance); err != nil {
				return err
			}
		}
		// 5. Idempotency record with the response snapshot.
		if err := storeIdempotentResponse(ctx, tx, t.TenantID, t.IdempotencyKey, 201, t); err != nil {
			return err
		}
		out = t
		return nil
	})
	if err != nil {
		// Concurrent duplicate: another request with the same key won the
		// race. Retry the lookup once and serve the stored response.
		if isUniqueViolation(err) {
			var stored domain.Transaction
			var ok bool
			lerr := pgxutil.WithTenantTx(ctx, s.pool, t.TenantID, func(tx pgx.Tx) error {
				var innerErr error
				stored, ok, innerErr = lookupIdempotentTx(ctx, tx, t.IdempotencyKey)
				return innerErr
			})
			if lerr == nil && ok {
				return stored, true, nil
			}
			return domain.Transaction{}, false, mapErr(err)
		}
		return domain.Transaction{}, false, mapErr(err)
	}
	return out, replayed, nil
}

// GetTransaction fetches one transaction with its postings.
func (s *Store) GetTransaction(ctx context.Context, tenantID, id uuid.UUID) (domain.Transaction, error) {
	var out domain.Transaction
	err := pgxutil.WithTenantTx(ctx, s.pool, tenantID, func(tx pgx.Tx) error {
		t, err := loadTransactionTx(ctx, tx, id, false)
		out = t
		return err
	})
	return out, err
}

// ListTransactions pages through transactions with their postings, newest first.
func (s *Store) ListTransactions(ctx context.Context, tenantID uuid.UUID, filter app.TransactionFilter, page app.Page) ([]domain.Transaction, error) {
	cAt, cID := cursorArgs(page.Cursor)
	var ledgerID any
	if filter.LedgerID != nil {
		ledgerID = *filter.LedgerID
	}
	var status any
	if filter.Status != nil {
		status = string(*filter.Status)
	}
	var out []domain.Transaction
	err := pgxutil.WithTenantTx(ctx, s.pool, tenantID, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `
			SELECT `+transactionColumns+`
			FROM transactions
			WHERE ($1::uuid IS NULL OR ledger_id = $1)
			  AND ($2::text IS NULL OR status = $2)
			  AND ($3::timestamptz IS NULL OR (created_at, id) < ($3, $4::uuid))
			ORDER BY created_at DESC, id DESC
			LIMIT $5`, ledgerID, status, cAt, cID, normalizeLimit(page.Limit))
		if err != nil {
			return err
		}
		defer rows.Close()
		ids := make([]uuid.UUID, 0, 32)
		for rows.Next() {
			t, err := scanTransaction(rows)
			if err != nil {
				return err
			}
			out = append(out, t)
			ids = append(ids, t.ID)
		}
		if err := rows.Err(); err != nil {
			return err
		}
		if len(ids) == 0 {
			return nil
		}
		postings, err := loadPostings(ctx, tx, ids)
		if err != nil {
			return err
		}
		for i := range out {
			out[i].Postings = postings[out[i].ID]
		}
		return nil
	})
	return out, err
}

// PostTransaction transitions draft -> posted: moves pending balances to
// posted and emits ledger.transaction.posted. Already-posted transactions are
// returned unchanged (idempotent).
func (s *Store) PostTransaction(ctx context.Context, tenantID, id uuid.UUID, now time.Time) (domain.Transaction, error) {
	var out domain.Transaction
	err := pgxutil.WithTenantTx(ctx, s.pool, tenantID, func(tx pgx.Tx) error {
		t, err := loadTransactionTx(ctx, tx, id, true)
		if err != nil {
			return err
		}
		switch t.Status {
		case domain.TransactionPosted:
			out = t // idempotent no-op
			return nil
		case domain.TransactionReversed:
			return fmt.Errorf("%w: transaction is reversed and cannot be posted", app.ErrInvalidState)
		}
		if _, err := tx.Exec(ctx, `
			UPDATE transactions SET status = 'posted', posted_at = $2 WHERE id = $1`, id, now); err != nil {
			return err
		}
		if err := promoteBalances(ctx, tx, t.Postings); err != nil {
			return err
		}
		t.Status = domain.TransactionPosted
		t.PostedAt = &now
		payload := app.NewTransactionPostedEvent(t, now)
		if err := insertOutbox(ctx, tx, events.TopicTransactionPosted, t.TenantID, payload); err != nil {
			return err
		}
		out = t
		return nil
	})
	return out, mapErr(err)
}

// ReverseTransaction creates and posts the mirror transaction, marks the
// original reversed and emits ledger.transaction.reversed — atomically.
func (s *Store) ReverseTransaction(ctx context.Context, tenantID, id uuid.UUID, idempotencyKey, reason string, now time.Time) (domain.Transaction, error) {
	var out domain.Transaction
	err := pgxutil.WithTenantTx(ctx, s.pool, tenantID, func(tx pgx.Tx) error {
		original, err := loadTransactionTx(ctx, tx, id, true)
		if err != nil {
			return err
		}
		revID, err := uuid.NewV7()
		if err != nil {
			return fmt.Errorf("generate reversal id: %w", err)
		}
		rev, err := domain.Reverse(original, revID, now)
		if err != nil {
			switch {
			case errors.Is(err, domain.ErrAlreadyReversed):
				return fmt.Errorf("%w: transaction is already reversed", app.ErrInvalidState)
			case errors.Is(err, domain.ErrNotPosted):
				return fmt.Errorf("%w: only posted transactions can be reversed", app.ErrInvalidState)
			}
			return err
		}
		if idempotencyKey != "" {
			rev.IdempotencyKey = idempotencyKey
		}
		rev.Description = "Reversal of " + original.IdempotencyKey
		if reason != "" {
			rev.Description += ": " + reason
		}
		if err := insertTransactionTx(ctx, tx, rev); err != nil {
			return err
		}
		if err := applyBalances(ctx, tx, rev.TenantID, rev.Postings, upsertPostedBalance); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `
			UPDATE transactions SET status = 'reversed', reversed_by_id = $2 WHERE id = $1`, id, rev.ID); err != nil {
			return err
		}
		if err := storeIdempotentResponse(ctx, tx, rev.TenantID, rev.IdempotencyKey, 201, rev); err != nil {
			return err
		}
		payload := app.TransactionReversedEvent{
			TransactionID:         original.ID,
			ReversalTransactionID: rev.ID,
			LedgerID:              original.LedgerID,
			Reason:                reason,
			ReversedAt:            now,
		}
		if err := insertOutbox(ctx, tx, events.TopicTransactionReversed, rev.TenantID, payload); err != nil {
			return err
		}
		out = rev
		return nil
	})
	return out, mapErr(err)
}

// ---- holds --------------------------------------------------------------------

const holdColumns = `id, tenant_id, ledger_id, account_id, idempotency_key, asset, amount,
	status, expires_at, captured_transaction_id, metadata, created_at`

func scanHold(row pgx.Row) (domain.Hold, error) {
	var h domain.Hold
	var status, asset string
	var units int64
	err := row.Scan(&h.ID, &h.TenantID, &h.LedgerID, &h.AccountID, &h.IdempotencyKey, &asset, &units,
		&status, &h.ExpiresAt, &h.CapturedTransactionID, &h.Metadata, &h.CreatedAt)
	h.Status = domain.HoldStatus(status)
	h.Amount = money.Amount{Asset: asset, Units: units}
	return h, err
}

func getHoldTx(ctx context.Context, tx pgx.Tx, id uuid.UUID, forUpdate bool) (domain.Hold, error) {
	q := `SELECT ` + holdColumns + ` FROM holds WHERE id = $1`
	if forUpdate {
		q += ` FOR UPDATE`
	}
	h, err := scanHold(tx.QueryRow(ctx, q, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Hold{}, fmt.Errorf("hold %s: %w", id, app.ErrNotFound)
	}
	return h, err
}

// CreateHold reserves funds: checks available balance, inserts the hold and
// bumps account_balances.held in one transaction, then emits
// ledger.hold.created via the outbox.
func (s *Store) CreateHold(ctx context.Context, h domain.Hold) (domain.Hold, bool, error) {
	var out domain.Hold
	replayed := false
	err := pgxutil.WithTenantTx(ctx, s.pool, h.TenantID, func(tx pgx.Tx) error {
		// Idempotent replay.
		existing, err := scanHold(tx.QueryRow(ctx,
			`SELECT `+holdColumns+` FROM holds WHERE idempotency_key = $1`, h.IdempotencyKey))
		if err == nil {
			out, replayed = existing, true
			return nil
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return err
		}

		account, err := getAccountTx(ctx, tx, h.AccountID)
		if err != nil {
			return err
		}
		if h.LedgerID != uuid.Nil && h.LedgerID != account.LedgerID {
			return fmt.Errorf("%w: account %s belongs to another ledger", app.ErrValidation, h.AccountID)
		}
		h.LedgerID = account.LedgerID

		// Lock the balance row and check availability.
		var b domain.Balance
		b.Asset = h.Amount.Asset
		err = tx.QueryRow(ctx, `
			SELECT posted_debits, posted_credits, held
			FROM account_balances
			WHERE account_id = $1 AND asset = $2
			FOR UPDATE`, h.AccountID, h.Amount.Asset).
			Scan(&b.PostedDebits, &b.PostedCredits, &b.Held)
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return err
		}
		available := b.Available(account.NormalBalance)
		if available < h.Amount.Units {
			return fmt.Errorf("%w: available balance %d %s is less than hold amount %d %s",
				app.ErrInsufficientFunds, available, h.Amount.Asset, h.Amount.Units, h.Amount.Asset)
		}

		if _, err := tx.Exec(ctx, `
			INSERT INTO holds (id, tenant_id, ledger_id, account_id, idempotency_key, asset, amount,
				status, expires_at, metadata, created_at, updated_at)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $11)`,
			h.ID, h.TenantID, h.LedgerID, h.AccountID, h.IdempotencyKey, h.Amount.Asset, h.Amount.Units,
			string(h.Status), h.ExpiresAt, meta(h.Metadata), h.CreatedAt); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO account_balances (account_id, tenant_id, asset, held, version)
			VALUES ($1, $2, $3, $4, 1)
			ON CONFLICT (account_id, asset) DO UPDATE SET
				held       = account_balances.held + EXCLUDED.held,
				version    = account_balances.version + 1,
				updated_at = now()`,
			h.AccountID, h.TenantID, h.Amount.Asset, h.Amount.Units); err != nil {
			return err
		}
		payload := app.HoldCreatedEvent{
			HoldID:         h.ID,
			LedgerID:       h.LedgerID,
			AccountID:      h.AccountID,
			IdempotencyKey: h.IdempotencyKey,
			Amount:         app.NewEventMoney(h.Amount),
			ExpiresAt:      h.ExpiresAt,
			CreatedAt:      h.CreatedAt,
		}
		if err := insertOutbox(ctx, tx, events.TopicHoldCreated, h.TenantID, payload); err != nil {
			return err
		}
		out = h
		return nil
	})
	if err != nil {
		if isUniqueViolation(err) {
			// Concurrent duplicate: serve the winner's hold.
			var existing domain.Hold
			lerr := pgxutil.WithTenantTx(ctx, s.pool, h.TenantID, func(tx pgx.Tx) error {
				var innerErr error
				existing, innerErr = scanHold(tx.QueryRow(ctx,
					`SELECT `+holdColumns+` FROM holds WHERE idempotency_key = $1`, h.IdempotencyKey))
				return innerErr
			})
			if lerr == nil {
				return existing, true, nil
			}
		}
		return domain.Hold{}, false, mapErr(err)
	}
	return out, replayed, nil
}

// releaseHeld subtracts the hold amount from account_balances.held.
func releaseHeld(ctx context.Context, tx pgx.Tx, h domain.Hold) error {
	tag, err := tx.Exec(ctx, `
		UPDATE account_balances SET
			held = held - $1,
			version = version + 1,
			updated_at = now()
		WHERE account_id = $2 AND asset = $3`,
		h.Amount.Units, h.AccountID, h.Amount.Asset)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("balance row missing for account %s asset %s", h.AccountID, h.Amount.Asset)
	}
	return nil
}

// CaptureHold frees the held funds and marks the hold captured, optionally
// linking the posted transaction that moved the money. Partial captures
// release the remainder (reflected in the event payload).
func (s *Store) CaptureHold(ctx context.Context, tenantID, id uuid.UUID, amount *money.Amount, transactionID *uuid.UUID, now time.Time) (domain.Hold, error) {
	var out domain.Hold
	err := pgxutil.WithTenantTx(ctx, s.pool, tenantID, func(tx pgx.Tx) error {
		h, err := getHoldTx(ctx, tx, id, true)
		if err != nil {
			return err
		}
		if h.Status != domain.HoldActive {
			return fmt.Errorf("%w: hold is already %s", app.ErrInvalidState, h.Status)
		}
		captured := h.Amount
		if amount != nil {
			if amount.Asset != h.Amount.Asset {
				return fmt.Errorf("%w: capture asset %s does not match hold asset %s",
					app.ErrValidation, amount.Asset, h.Amount.Asset)
			}
			if amount.Units > h.Amount.Units {
				return fmt.Errorf("%w: capture amount %d exceeds hold amount %d",
					app.ErrValidation, amount.Units, h.Amount.Units)
			}
			captured = *amount
		}
		if transactionID != nil {
			var one int
			err := tx.QueryRow(ctx, `SELECT 1 FROM transactions WHERE id = $1`, *transactionID).Scan(&one)
			if errors.Is(err, pgx.ErrNoRows) {
				return fmt.Errorf("transaction %s: %w", *transactionID, app.ErrNotFound)
			}
			if err != nil {
				return err
			}
		}
		if _, err := tx.Exec(ctx, `
			UPDATE holds SET status = 'captured', captured_transaction_id = $2, updated_at = $3
			WHERE id = $1`, id, transactionID, now); err != nil {
			return err
		}
		if err := releaseHeld(ctx, tx, h); err != nil {
			return err
		}
		payload := app.HoldCapturedEvent{
			HoldID:         h.ID,
			LedgerID:       h.LedgerID,
			AccountID:      h.AccountID,
			CapturedAmount: app.NewEventMoney(captured),
			TransactionID:  transactionID,
			CapturedAt:     now,
		}
		if captured.Units < h.Amount.Units {
			remainder := app.NewEventMoney(money.Amount{Asset: h.Amount.Asset, Units: h.Amount.Units - captured.Units})
			payload.ReleasedAmount = &remainder
		}
		if err := insertOutbox(ctx, tx, events.TopicHoldCaptured, h.TenantID, payload); err != nil {
			return err
		}
		h.Status = domain.HoldCaptured
		h.CapturedTransactionID = transactionID
		out = h
		return nil
	})
	return out, mapErr(err)
}

// ReleaseHold frees the reserved funds without moving money.
func (s *Store) ReleaseHold(ctx context.Context, tenantID, id uuid.UUID, now time.Time) (domain.Hold, error) {
	var out domain.Hold
	err := pgxutil.WithTenantTx(ctx, s.pool, tenantID, func(tx pgx.Tx) error {
		h, err := getHoldTx(ctx, tx, id, true)
		if err != nil {
			return err
		}
		if h.Status != domain.HoldActive {
			return fmt.Errorf("%w: hold is already %s", app.ErrInvalidState, h.Status)
		}
		if _, err := tx.Exec(ctx, `
			UPDATE holds SET status = 'released', updated_at = $2 WHERE id = $1`, id, now); err != nil {
			return err
		}
		if err := releaseHeld(ctx, tx, h); err != nil {
			return err
		}
		payload := app.HoldReleasedEvent{
			HoldID:     h.ID,
			LedgerID:   h.LedgerID,
			AccountID:  h.AccountID,
			Amount:     app.NewEventMoney(h.Amount),
			Reason:     "released",
			ReleasedAt: now,
		}
		if err := insertOutbox(ctx, tx, events.TopicHoldReleased, h.TenantID, payload); err != nil {
			return err
		}
		h.Status = domain.HoldReleased
		out = h
		return nil
	})
	return out, mapErr(err)
}

// GetHold fetches one hold.
func (s *Store) GetHold(ctx context.Context, tenantID, id uuid.UUID) (domain.Hold, error) {
	var out domain.Hold
	err := pgxutil.WithTenantTx(ctx, s.pool, tenantID, func(tx pgx.Tx) error {
		h, err := getHoldTx(ctx, tx, id, false)
		out = h
		return err
	})
	return out, err
}

// ---- reports ------------------------------------------------------------------

// TrialBalance aggregates account_balances per account/asset for a ledger and
// computes per-asset totals with the debits == credits sanity check. When asOf
// is set, the state is reconstructed by aggregating postings of transactions
// with effective_at <= asOf (posted or later reversed) instead of reading the
// running account_balances.
func (s *Store) TrialBalance(ctx context.Context, tenantID, ledgerID uuid.UUID, asOf *time.Time) (domain.TrialBalance, error) {
	out := domain.TrialBalance{LedgerID: ledgerID, AsOf: time.Now().UTC()}
	if asOf != nil {
		out.AsOf = *asOf
	}
	err := pgxutil.WithTenantTx(ctx, s.pool, tenantID, func(tx pgx.Tx) error {
		var one int
		err := tx.QueryRow(ctx, `SELECT 1 FROM ledgers WHERE id = $1`, ledgerID).Scan(&one)
		if errors.Is(err, pgx.ErrNoRows) {
			return fmt.Errorf("ledger %s: %w", ledgerID, app.ErrNotFound)
		}
		if err != nil {
			return err
		}
		var rows pgx.Rows
		if asOf == nil {
			rows, err = tx.Query(ctx, `
				SELECT b.account_id, a.name, a.type, b.asset, b.posted_debits, b.posted_credits
				FROM account_balances b
				JOIN accounts a ON a.id = b.account_id
				WHERE a.ledger_id = $1
				ORDER BY a.path, b.asset`, ledgerID)
		} else {
			// Historical reconstruction from postings. A 'reversed' status
			// means the transaction WAS posted and later compensated: its
			// postings still count at any as_of on or after its effective_at
			// (the reversal carries its own, later, effective_at).
			rows, err = tx.Query(ctx, `
				SELECT p.account_id, a.name, a.type, p.asset,
				       COALESCE(SUM(p.amount) FILTER (WHERE p.direction = 'DEBIT'),  0),
				       COALESCE(SUM(p.amount) FILTER (WHERE p.direction = 'CREDIT'), 0)
				FROM postings p
				JOIN transactions t ON t.id = p.transaction_id
				JOIN accounts a ON a.id = p.account_id
				WHERE a.ledger_id = $1
				  AND t.status IN ('posted', 'reversed')
				  AND t.effective_at <= $2
				GROUP BY p.account_id, a.name, a.path, a.type, p.asset
				ORDER BY a.path, p.asset`, ledgerID, *asOf)
		}
		defer rows.Close()
		totals := make(map[string]*domain.TrialBalanceTotal)
		order := make([]string, 0, 4)
		for rows.Next() {
			var r domain.TrialBalanceRow
			var typ string
			if err := rows.Scan(&r.AccountID, &r.AccountName, &typ, &r.Asset, &r.Debits, &r.Credits); err != nil {
				return err
			}
			r.AccountType = domain.AccountType(typ)
			out.Rows = append(out.Rows, r)
			t, ok := totals[r.Asset]
			if !ok {
				t = &domain.TrialBalanceTotal{Asset: r.Asset}
				totals[r.Asset] = t
				order = append(order, r.Asset)
			}
			t.Debits += r.Debits
			t.Credits += r.Credits
		}
		if err := rows.Err(); err != nil {
			return err
		}
		for _, asset := range order {
			t := totals[asset]
			t.Balanced = t.Debits == t.Credits
			out.Totals = append(out.Totals, *t)
		}
		return nil
	})
	return out, err
}

// statementRawSums aggregates the raw (debits - credits) sum per asset over
// the postings of an account within a time filter. SQL does all summing.
func statementRawSums(ctx context.Context, tx pgx.Tx, accountID uuid.UUID, cond string, args ...any) (map[string]int64, error) {
	rows, err := tx.Query(ctx, `
		SELECT p.asset,
		       COALESCE(SUM(CASE WHEN p.direction = 'DEBIT' THEN p.amount ELSE -p.amount END), 0)
		FROM postings p
		JOIN transactions t ON t.id = p.transaction_id
		WHERE p.account_id = $1
		  AND t.status IN ('posted', 'reversed')
		  AND `+cond+`
		GROUP BY p.asset`, append([]any{accountID}, args...)...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make(map[string]int64)
	for rows.Next() {
		var asset string
		var raw int64
		if err := rows.Scan(&asset, &raw); err != nil {
			return nil, err
		}
		out[asset] = raw
	}
	return out, rows.Err()
}

// Statement builds the account statement for [from, to]: the opening balance
// aggregates postings with effective_at < from, entries carry a per-asset
// running balance computed with a SQL window function over the whole period
// (so pagination never breaks the accumulation), and the closing balance is
// opening + the period totals. All sums happen in SQL.
func (s *Store) Statement(ctx context.Context, tenantID, accountID uuid.UUID, from, to time.Time, page app.Page) (domain.Statement, error) {
	cAt, cID := cursorArgs(page.Cursor)
	var out domain.Statement
	err := pgxutil.WithTenantTx(ctx, s.pool, tenantID, func(tx pgx.Tx) error {
		account, err := getAccountTx(ctx, tx, accountID)
		if err != nil {
			return err
		}
		out = domain.Statement{Account: account, From: from, To: to}
		nb := account.NormalBalance

		// Opening: everything strictly before the period.
		openingRaw, err := statementRawSums(ctx, tx, accountID, `t.effective_at < $2`, from)
		if err != nil {
			return err
		}
		// Period totals: closing = opening + period, per asset.
		periodRaw, err := statementRawSums(ctx, tx, accountID,
			`t.effective_at >= $2 AND t.effective_at <= $3`, from, to)
		if err != nil {
			return err
		}

		assets := make(map[string]bool, len(openingRaw)+len(periodRaw))
		for a := range openingRaw {
			assets[a] = true
		}
		for a := range periodRaw {
			assets[a] = true
		}
		names := make([]string, 0, len(assets))
		for a := range assets {
			names = append(names, a)
		}
		sort.Strings(names)
		for _, a := range names {
			out.Opening = append(out.Opening, domain.AssetBalance{
				Asset: a, Units: domain.SignRaw(nb, openingRaw[a])})
			out.Closing = append(out.Closing, domain.AssetBalance{
				Asset: a, Units: domain.RunningBalance(nb, openingRaw[a], periodRaw[a])})
		}

		// Entries: the window runs over the WHOLE period partitioned by
		// asset; the keyset cursor and LIMIT are applied outside, so every
		// page sees the correct cumulative value.
		rows, err := tx.Query(ctx, `
			WITH period AS (
				SELECT p.id, p.transaction_id, t.reference, p.direction, p.asset, p.amount,
				       t.effective_at,
				       SUM(CASE WHEN p.direction = 'DEBIT' THEN p.amount ELSE -p.amount END)
				           OVER (PARTITION BY p.asset ORDER BY t.effective_at, p.id) AS running_raw
				FROM postings p
				JOIN transactions t ON t.id = p.transaction_id
				WHERE p.account_id = $1
				  AND t.status IN ('posted', 'reversed')
				  AND t.effective_at >= $2 AND t.effective_at <= $3
			)
			SELECT id, transaction_id, reference, direction, asset, amount, effective_at, running_raw
			FROM period
			WHERE ($4::timestamptz IS NULL OR (effective_at, id) > ($4, $5::uuid))
			ORDER BY effective_at, id
			LIMIT $6`, accountID, from, to, cAt, cID, normalizeLimit(page.Limit))
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var e domain.StatementEntry
			var dir, asset string
			var units, runningRaw int64
			if err := rows.Scan(&e.PostingID, &e.TransactionID, &e.Reference, &dir, &asset,
				&units, &e.EffectiveAt, &runningRaw); err != nil {
				return err
			}
			e.Direction = domain.Direction(dir)
			e.Amount = money.Amount{Asset: asset, Units: units}
			e.Running = domain.RunningBalance(nb, openingRaw[asset], runningRaw)
			out.Entries = append(out.Entries, e)
		}
		return rows.Err()
	})
	return out, err
}

// ProviderAccountBalances returns the per-account/asset posted sums of every
// asset- or liability-type account of the tenant, straight from
// account_balances (never postings).
func (s *Store) ProviderAccountBalances(ctx context.Context, tenantID uuid.UUID) ([]domain.ProviderAccountBalance, error) {
	var out []domain.ProviderAccountBalance
	err := pgxutil.WithTenantTx(ctx, s.pool, tenantID, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `
			SELECT a.id, a.path, a.type, a.metadata, b.asset, b.posted_debits, b.posted_credits
			FROM account_balances b
			JOIN accounts a ON a.id = b.account_id
			WHERE a.type IN ('asset', 'liability')
			ORDER BY a.path, b.asset`)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var r domain.ProviderAccountBalance
			var typ string
			if err := rows.Scan(&r.AccountID, &r.Path, &typ, &r.Metadata,
				&r.Asset, &r.PostedDebits, &r.PostedCredits); err != nil {
				return err
			}
			r.Type = domain.AccountType(typ)
			out = append(out, r)
		}
		return rows.Err()
	})
	return out, err
}
