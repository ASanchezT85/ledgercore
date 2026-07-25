package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/ledgercore/ledgercore/libs/go/events"
	"github.com/ledgercore/ledgercore/libs/go/pgxutil"

	"github.com/ledgercore/ledgercore/services/reconciliation/internal/app"
	"github.com/ledgercore/ledgercore/services/reconciliation/internal/domain"
)

// Store implements app.Store over a pgx pool whose search_path is pinned to
// the recon schema. All table names below are unqualified on purpose.
type Store struct {
	pool *pgxpool.Pool
}

// NewStore wraps the pool.
func NewStore(pool *pgxpool.Pool) *Store { return &Store{pool: pool} }

// Within opens a tenant transaction (SET LOCAL app.tenant_id -> RLS applies)
// and runs fn against a transaction-scoped store.
func (s *Store) Within(ctx context.Context, tenantID uuid.UUID, fn func(app.TxStore) error) error {
	return pgxutil.WithTenantTx(ctx, s.pool, tenantID, func(tx pgx.Tx) error {
		return fn(&txStore{tx: tx})
	})
}

// InsertMirrorEntries idempotently stores mirror postings coming from
// ledger.transaction.posted events. Conflicts on the deterministic posting id
// are ignored, so event redeliveries are harmless.
func (s *Store) InsertMirrorEntries(ctx context.Context, tenantID uuid.UUID, entries []domain.MirrorEntry) error {
	if len(entries) == 0 {
		return nil
	}
	return pgxutil.WithTenantTx(ctx, s.pool, tenantID, func(tx pgx.Tx) error {
		batch := &pgx.Batch{}
		for _, e := range entries {
			batch.Queue(`
				INSERT INTO ledger_entries_mirror
					(id, tenant_id, transaction_id, account_id, direction, amount, asset, reference, posted_at)
				VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
				ON CONFLICT (id) DO NOTHING`,
				e.ID, e.TenantID, e.TransactionID, e.AccountID, e.Direction, e.Amount, e.Asset, e.Reference, e.PostedAt,
			)
		}
		results := tx.SendBatch(ctx, batch)
		defer results.Close()
		for range entries {
			if _, err := results.Exec(); err != nil {
				return fmt.Errorf("postgres: insert mirror entry: %w", err)
			}
		}
		return nil
	})
}

// DrainOutbox publishes up to limit pending outbox rows and marks them
// published, inside a system transaction (the outbox spans tenants). Rows are
// locked with FOR UPDATE SKIP LOCKED so concurrent pollers never double-send.
// If publishing fails mid-batch, rows already sent stay marked (the commit
// still happens) and the error is returned for logging; unpublished rows are
// retried on the next tick. JetStream deduplicates by envelope id anyway.
func (s *Store) DrainOutbox(ctx context.Context, pub events.Publisher, limit int) (int, error) {
	published := 0
	var pubErr error
	err := pgxutil.WithSystemTx(ctx, s.pool, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `
			SELECT id, envelope
			FROM outbox
			WHERE published_at IS NULL
			ORDER BY created_at
			LIMIT $1
			FOR UPDATE SKIP LOCKED`, limit)
		if err != nil {
			return fmt.Errorf("postgres: select outbox: %w", err)
		}
		type pending struct {
			id   uuid.UUID
			body []byte
		}
		var batch []pending
		for rows.Next() {
			var p pending
			if err := rows.Scan(&p.id, &p.body); err != nil {
				rows.Close()
				return fmt.Errorf("postgres: scan outbox row: %w", err)
			}
			batch = append(batch, p)
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return fmt.Errorf("postgres: iterate outbox: %w", err)
		}

		for _, p := range batch {
			var env events.Envelope
			if err := json.Unmarshal(p.body, &env); err != nil {
				// Poison row: mark it published so it never blocks the queue,
				// and surface the error to the poller log.
				pubErr = fmt.Errorf("postgres: outbox row %s is not a valid envelope: %w", p.id, err)
			} else if err := pub.Publish(env); err != nil {
				// Broker unavailable: stop here, keep the row pending.
				pubErr = err
				break
			}
			if _, err := tx.Exec(ctx, `UPDATE outbox SET published_at = now() WHERE id = $1`, p.id); err != nil {
				return fmt.Errorf("postgres: mark outbox published: %w", err)
			}
			published++
		}
		return nil
	})
	if err != nil {
		return 0, err
	}
	return published, pubErr
}

// ---- txStore -----------------------------------------------------------------

type txStore struct {
	tx pgx.Tx
}

var _ app.TxStore = (*txStore)(nil)

func notFound(err error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.ErrNotFound
	}
	return err
}

func (t *txStore) InsertSource(ctx context.Context, s domain.Source) error {
	_, err := t.tx.Exec(ctx, `
		INSERT INTO sources (id, tenant_id, name, kind, created_at)
		VALUES ($1, $2, $3, $4, $5)`,
		s.ID, s.TenantID, s.Name, string(s.Kind), s.CreatedAt)
	if err != nil {
		return fmt.Errorf("postgres: insert source: %w", err)
	}
	return nil
}

func (t *txStore) ListSources(ctx context.Context) ([]domain.Source, error) {
	rows, err := t.tx.Query(ctx, `
		SELECT id, tenant_id, name, kind, created_at
		FROM sources ORDER BY created_at, id`)
	if err != nil {
		return nil, fmt.Errorf("postgres: list sources: %w", err)
	}
	defer rows.Close()

	out := []domain.Source{}
	for rows.Next() {
		var s domain.Source
		var kind string
		if err := rows.Scan(&s.ID, &s.TenantID, &s.Name, &kind, &s.CreatedAt); err != nil {
			return nil, fmt.Errorf("postgres: scan source: %w", err)
		}
		s.Kind = domain.SourceKind(kind)
		out = append(out, s)
	}
	return out, rows.Err()
}

func (t *txStore) GetSource(ctx context.Context, id uuid.UUID) (domain.Source, error) {
	var s domain.Source
	var kind string
	err := t.tx.QueryRow(ctx, `
		SELECT id, tenant_id, name, kind, created_at
		FROM sources WHERE id = $1`, id).
		Scan(&s.ID, &s.TenantID, &s.Name, &kind, &s.CreatedAt)
	if err != nil {
		return domain.Source{}, notFound(err)
	}
	s.Kind = domain.SourceKind(kind)
	return s, nil
}

func (t *txStore) InsertImport(ctx context.Context, imp domain.Import) error {
	_, err := t.tx.Exec(ctx, `
		INSERT INTO imports (id, tenant_id, source_id, filename, status, row_count, error, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`,
		imp.ID, imp.TenantID, imp.SourceID, imp.Filename, string(imp.Status), imp.RowCount, imp.Error, imp.CreatedAt)
	if err != nil {
		return fmt.Errorf("postgres: insert import: %w", err)
	}
	return nil
}

func (t *txStore) InsertExternalTransactions(ctx context.Context, txs []domain.ExternalTransaction) error {
	if len(txs) == 0 {
		return nil
	}
	batch := &pgx.Batch{}
	for _, x := range txs {
		raw, err := json.Marshal(x.Raw)
		if err != nil {
			return fmt.Errorf("postgres: marshal raw row: %w", err)
		}
		batch.Queue(`
			INSERT INTO external_transactions
				(id, tenant_id, import_id, source_id, external_ref, amount, asset, occurred_at, raw, match_status, matched_entry_id)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9::jsonb, $10, $11)`,
			x.ID, x.TenantID, x.ImportID, x.SourceID, x.ExternalRef, x.Amount, x.Asset, x.OccurredAt, raw, string(x.MatchStatus), x.MatchedEntryID)
	}
	results := t.tx.SendBatch(ctx, batch)
	defer results.Close()
	for range txs {
		if _, err := results.Exec(); err != nil {
			return fmt.Errorf("postgres: insert external transaction: %w", err)
		}
	}
	return nil
}

func (t *txStore) ListUnmatchedExternals(ctx context.Context, sourceID uuid.UUID) ([]domain.ExternalTransaction, error) {
	rows, err := t.tx.Query(ctx, `
		SELECT id, tenant_id, import_id, source_id, external_ref, amount, asset, occurred_at, match_status, matched_entry_id
		FROM external_transactions
		WHERE source_id = $1 AND match_status = 'unmatched'
		ORDER BY occurred_at, id`, sourceID)
	if err != nil {
		return nil, fmt.Errorf("postgres: list unmatched externals: %w", err)
	}
	defer rows.Close()

	out := []domain.ExternalTransaction{}
	for rows.Next() {
		var x domain.ExternalTransaction
		var status string
		if err := rows.Scan(&x.ID, &x.TenantID, &x.ImportID, &x.SourceID, &x.ExternalRef,
			&x.Amount, &x.Asset, &x.OccurredAt, &status, &x.MatchedEntryID); err != nil {
			return nil, fmt.Errorf("postgres: scan external transaction: %w", err)
		}
		x.MatchStatus = domain.MatchStatus(status)
		out = append(out, x)
	}
	return out, rows.Err()
}

func (t *txStore) ListMirrorEntries(ctx context.Context) ([]domain.MirrorEntry, error) {
	rows, err := t.tx.Query(ctx, `
		SELECT id, tenant_id, transaction_id, account_id, direction, amount, asset, reference, posted_at
		FROM ledger_entries_mirror
		ORDER BY posted_at, id`)
	if err != nil {
		return nil, fmt.Errorf("postgres: list mirror entries: %w", err)
	}
	defer rows.Close()

	out := []domain.MirrorEntry{}
	for rows.Next() {
		var e domain.MirrorEntry
		if err := rows.Scan(&e.ID, &e.TenantID, &e.TransactionID, &e.AccountID,
			&e.Direction, &e.Amount, &e.Asset, &e.Reference, &e.PostedAt); err != nil {
			return nil, fmt.Errorf("postgres: scan mirror entry: %w", err)
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

func (t *txStore) MarkExternalMatched(ctx context.Context, externalID, entryID uuid.UUID) error {
	_, err := t.tx.Exec(ctx, `
		UPDATE external_transactions
		SET match_status = 'matched', matched_entry_id = $2
		WHERE id = $1`, externalID, entryID)
	if err != nil {
		return fmt.Errorf("postgres: mark external matched: %w", err)
	}
	return nil
}

func (t *txStore) MarkExternalDisputed(ctx context.Context, externalID uuid.UUID) error {
	_, err := t.tx.Exec(ctx, `
		UPDATE external_transactions
		SET match_status = 'disputed'
		WHERE id = $1`, externalID)
	if err != nil {
		return fmt.Errorf("postgres: mark external disputed: %w", err)
	}
	return nil
}

func (t *txStore) InsertRun(ctx context.Context, run domain.Run) error {
	_, err := t.tx.Exec(ctx, `
		INSERT INTO reconciliation_runs
			(id, tenant_id, source_id, status, matched_count, unmatched_count, started_at, finished_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`,
		run.ID, run.TenantID, run.SourceID, string(run.Status),
		run.MatchedCount, run.UnmatchedCount, run.StartedAt, run.FinishedAt)
	if err != nil {
		return fmt.Errorf("postgres: insert run: %w", err)
	}
	return nil
}

func (t *txStore) UpdateRun(ctx context.Context, run domain.Run) error {
	_, err := t.tx.Exec(ctx, `
		UPDATE reconciliation_runs
		SET status = $2, matched_count = $3, unmatched_count = $4, finished_at = $5
		WHERE id = $1`,
		run.ID, string(run.Status), run.MatchedCount, run.UnmatchedCount, run.FinishedAt)
	if err != nil {
		return fmt.Errorf("postgres: update run: %w", err)
	}
	return nil
}

func (t *txStore) GetRun(ctx context.Context, id uuid.UUID) (domain.Run, error) {
	var run domain.Run
	var status string
	err := t.tx.QueryRow(ctx, `
		SELECT id, tenant_id, source_id, status, matched_count, unmatched_count, started_at, finished_at
		FROM reconciliation_runs WHERE id = $1`, id).
		Scan(&run.ID, &run.TenantID, &run.SourceID, &status,
			&run.MatchedCount, &run.UnmatchedCount, &run.StartedAt, &run.FinishedAt)
	if err != nil {
		return domain.Run{}, notFound(err)
	}
	run.Status = domain.RunStatus(status)
	return run, nil
}

func (t *txStore) InsertDiscrepancy(ctx context.Context, d domain.Discrepancy) error {
	details, err := json.Marshal(d.Details)
	if err != nil {
		return fmt.Errorf("postgres: marshal discrepancy details: %w", err)
	}
	_, err = t.tx.Exec(ctx, `
		INSERT INTO discrepancies (id, tenant_id, run_id, kind, details, status, created_at)
		VALUES ($1, $2, $3, $4, $5::jsonb, $6, $7)`,
		d.ID, d.TenantID, d.RunID, string(d.Kind), details, string(d.Status), d.CreatedAt)
	if err != nil {
		return fmt.Errorf("postgres: insert discrepancy: %w", err)
	}
	return nil
}

func scanDiscrepancy(row pgx.Row) (domain.Discrepancy, error) {
	var d domain.Discrepancy
	var kind, status string
	var details []byte
	if err := row.Scan(&d.ID, &d.TenantID, &d.RunID, &kind, &details, &status, &d.CreatedAt); err != nil {
		return domain.Discrepancy{}, err
	}
	d.Kind = domain.DiscrepancyKind(kind)
	d.Status = domain.DiscrepancyStatus(status)
	if len(details) > 0 {
		if err := json.Unmarshal(details, &d.Details); err != nil {
			return domain.Discrepancy{}, fmt.Errorf("postgres: unmarshal discrepancy details: %w", err)
		}
	}
	return d, nil
}

func (t *txStore) ListDiscrepancies(ctx context.Context, status domain.DiscrepancyStatus) ([]domain.Discrepancy, error) {
	query := `
		SELECT id, tenant_id, run_id, kind, details, status, created_at
		FROM discrepancies`
	args := []any{}
	if status != "" {
		query += ` WHERE status = $1`
		args = append(args, string(status))
	}
	query += ` ORDER BY created_at DESC, id`

	rows, err := t.tx.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("postgres: list discrepancies: %w", err)
	}
	defer rows.Close()

	out := []domain.Discrepancy{}
	for rows.Next() {
		d, err := scanDiscrepancy(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

func (t *txStore) GetDiscrepancy(ctx context.Context, id uuid.UUID) (domain.Discrepancy, error) {
	d, err := scanDiscrepancy(t.tx.QueryRow(ctx, `
		SELECT id, tenant_id, run_id, kind, details, status, created_at
		FROM discrepancies WHERE id = $1`, id))
	if err != nil {
		return domain.Discrepancy{}, notFound(err)
	}
	return d, nil
}

func (t *txStore) UpdateDiscrepancy(ctx context.Context, d domain.Discrepancy) error {
	details, err := json.Marshal(d.Details)
	if err != nil {
		return fmt.Errorf("postgres: marshal discrepancy details: %w", err)
	}
	_, err = t.tx.Exec(ctx, `
		UPDATE discrepancies SET status = $2, details = $3::jsonb WHERE id = $1`,
		d.ID, string(d.Status), details)
	if err != nil {
		return fmt.Errorf("postgres: update discrepancy: %w", err)
	}
	return nil
}

func (t *txStore) SourceSummaries(ctx context.Context) ([]domain.SourceSummary, error) {
	rows, err := t.tx.Query(ctx, `
		SELECT
			s.id,
			s.name,
			COUNT(et.id) FILTER (WHERE et.match_status = 'matched'),
			COUNT(et.id) FILTER (WHERE et.match_status = 'unmatched'),
			COUNT(et.id) FILTER (WHERE et.match_status = 'disputed'),
			COALESCE(od.open_count, 0)
		FROM sources s
		LEFT JOIN external_transactions et ON et.source_id = s.id
		LEFT JOIN (
			SELECT r.source_id, COUNT(*) AS open_count
			FROM discrepancies d
			JOIN reconciliation_runs r ON r.id = d.run_id
			WHERE d.status = 'open'
			GROUP BY r.source_id
		) od ON od.source_id = s.id
		GROUP BY s.id, s.name, od.open_count
		ORDER BY s.name`)
	if err != nil {
		return nil, fmt.Errorf("postgres: source summaries: %w", err)
	}
	defer rows.Close()

	out := []domain.SourceSummary{}
	for rows.Next() {
		var sum domain.SourceSummary
		if err := rows.Scan(&sum.SourceID, &sum.SourceName,
			&sum.Matched, &sum.Unmatched, &sum.Disputed, &sum.OpenDiscrepancies); err != nil {
			return nil, fmt.Errorf("postgres: scan summary: %w", err)
		}
		out = append(out, sum)
	}
	return out, rows.Err()
}

func (t *txStore) InsertOutboxEnvelope(ctx context.Context, env events.Envelope) error {
	body, err := json.Marshal(env)
	if err != nil {
		return fmt.Errorf("postgres: marshal envelope: %w", err)
	}
	_, err = t.tx.Exec(ctx, `
		INSERT INTO outbox (id, tenant_id, topic, envelope, created_at)
		VALUES ($1, $2, $3, $4::jsonb, now())`,
		env.ID, env.TenantID, env.Type, body)
	if err != nil {
		return fmt.Errorf("postgres: insert outbox envelope: %w", err)
	}
	return nil
}
