// Package app orchestrates the reconciliation use cases over the storage
// port. All persistence for a use case happens inside one tenant-scoped
// transaction (Store.Within), so runs, imports and their side effects
// (discrepancies, outbox events) commit or roll back atomically.
package app

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"strconv"
	"time"

	"github.com/google/uuid"
	"github.com/ledgercore/ledgercore/libs/go/events"

	"github.com/ledgercore/ledgercore/libs/go/httpx"
	"github.com/ledgercore/ledgercore/services/reconciliation/internal/domain"
)

// ErrValidation marks client errors (HTTP 400). Wrap it with %w.
var ErrValidation = errors.New("validation failed")

func invalid(format string, args ...any) error {
	return fmt.Errorf("%w: %s", ErrValidation, fmt.Sprintf(format, args...))
}

// TxStore is the storage port scoped to one open tenant transaction.
type TxStore interface {
	InsertSource(ctx context.Context, s domain.Source) error
	ListSources(ctx context.Context, limit int, cursor httpx.Cursor) ([]domain.Source, error)
	GetSource(ctx context.Context, id uuid.UUID) (domain.Source, error)

	InsertImport(ctx context.Context, imp domain.Import) error
	InsertExternalTransactions(ctx context.Context, txs []domain.ExternalTransaction) error

	ListUnmatchedExternals(ctx context.Context, sourceID uuid.UUID) ([]domain.ExternalTransaction, error)
	ListMirrorEntries(ctx context.Context) ([]domain.MirrorEntry, error)
	MarkExternalMatched(ctx context.Context, externalID, entryID uuid.UUID) error
	MarkExternalDisputed(ctx context.Context, externalID uuid.UUID) error

	InsertRun(ctx context.Context, run domain.Run) error
	UpdateRun(ctx context.Context, run domain.Run) error
	GetRun(ctx context.Context, id uuid.UUID) (domain.Run, error)

	InsertDiscrepancy(ctx context.Context, d domain.Discrepancy) error
	ListDiscrepancies(ctx context.Context, status domain.DiscrepancyStatus, limit int, cursor httpx.Cursor) ([]domain.Discrepancy, error)
	GetDiscrepancy(ctx context.Context, id uuid.UUID) (domain.Discrepancy, error)
	UpdateDiscrepancy(ctx context.Context, d domain.Discrepancy) error

	SourceSummaries(ctx context.Context) ([]domain.SourceSummary, error)

	InsertOutboxEnvelope(ctx context.Context, env events.Envelope) error
}

// Store is the storage port. Within opens a tenant-scoped transaction
// (SET LOCAL app.tenant_id, so RLS applies) and runs fn inside it.
type Store interface {
	Within(ctx context.Context, tenantID uuid.UUID, fn func(TxStore) error) error
}

// Service implements the reconciliation use cases.
type Service struct {
	store Store
	log   *slog.Logger
}

// NewService builds a Service. logger must not be nil.
func NewService(store Store, logger *slog.Logger) *Service {
	return &Service{store: store, log: logger}
}

// CreateSource registers an external source for the tenant.
func (s *Service) CreateSource(ctx context.Context, tenantID uuid.UUID, name string, kind domain.SourceKind) (domain.Source, error) {
	if name == "" || len(name) > 255 {
		return domain.Source{}, invalid("name is required and must be at most 255 characters")
	}
	if !domain.ValidSourceKind(kind) {
		return domain.Source{}, invalid("kind must be one of bank, psp, provider, manual")
	}
	src := domain.Source{
		ID:        newID(),
		TenantID:  tenantID,
		Name:      name,
		Kind:      kind,
		CreatedAt: time.Now().UTC(),
	}
	err := s.store.Within(ctx, tenantID, func(tx TxStore) error {
		return tx.InsertSource(ctx, src)
	})
	if err != nil {
		return domain.Source{}, err
	}
	return src, nil
}

// ListSources returns a keyset page of the tenant's sources, newest first.
func (s *Service) ListSources(ctx context.Context, tenantID uuid.UUID, limit int, cursor httpx.Cursor) ([]domain.Source, error) {
	var out []domain.Source
	err := s.store.Within(ctx, tenantID, func(tx TxStore) error {
		var err error
		out, err = tx.ListSources(ctx, limit, cursor)
		return err
	})
	return out, err
}

// ImportStatement parses a CSV statement and stores it. Row errors mark the
// import failed with the row-level detail and store no transactions; the
// import record itself is always created so failures are visible.
func (s *Service) ImportStatement(ctx context.Context, tenantID, sourceID uuid.UUID, filename string, csvBody io.Reader) (domain.Import, error) {
	if sourceID == uuid.Nil {
		return domain.Import{}, invalid("source_id is required")
	}
	if filename == "" {
		filename = "statement.csv"
	}

	rows, parseErr := domain.ParseStatementCSV(csvBody, domain.DefaultCSVExponent)

	imp := domain.Import{
		ID:        newID(),
		TenantID:  tenantID,
		SourceID:  sourceID,
		Filename:  filename,
		CreatedAt: time.Now().UTC(),
	}

	err := s.store.Within(ctx, tenantID, func(tx TxStore) error {
		if _, err := tx.GetSource(ctx, sourceID); err != nil {
			return err
		}
		if parseErr != nil {
			imp.Status = domain.ImportFailed
			imp.Error = parseErr.Error()
			imp.RowCount = 0
			return tx.InsertImport(ctx, imp)
		}
		imp.Status = domain.ImportProcessed
		imp.RowCount = len(rows)
		if err := tx.InsertImport(ctx, imp); err != nil {
			return err
		}
		txs := make([]domain.ExternalTransaction, 0, len(rows))
		for _, row := range rows {
			txs = append(txs, domain.ExternalTransaction{
				ID:          newID(),
				TenantID:    tenantID,
				ImportID:    imp.ID,
				SourceID:    sourceID,
				ExternalRef: row.ExternalRef,
				Amount:      row.AmountUnits,
				Asset:       row.Asset,
				OccurredAt:  row.OccurredAt,
				Raw:         row.Raw,
				MatchStatus: domain.MatchUnmatched,
			})
		}
		return tx.InsertExternalTransactions(ctx, txs)
	})
	if err != nil {
		return domain.Import{}, err
	}
	return imp, nil
}

// StartRun executes the naive matcher for a source synchronously, inside one
// tenant transaction: match updates, discrepancies, outbox events and run
// counters all commit together.
func (s *Service) StartRun(ctx context.Context, tenantID, sourceID uuid.UUID) (domain.Run, error) {
	if sourceID == uuid.Nil {
		return domain.Run{}, invalid("source_id is required")
	}

	run := domain.Run{
		ID:        newID(),
		TenantID:  tenantID,
		SourceID:  sourceID,
		Status:    domain.RunRunning,
		StartedAt: time.Now().UTC(),
	}

	err := s.store.Within(ctx, tenantID, func(tx TxStore) error {
		if _, err := tx.GetSource(ctx, sourceID); err != nil {
			return err
		}
		if err := tx.InsertRun(ctx, run); err != nil {
			return err
		}

		externals, err := tx.ListUnmatchedExternals(ctx, sourceID)
		if err != nil {
			return err
		}
		mirror, err := tx.ListMirrorEntries(ctx)
		if err != nil {
			return err
		}

		result := domain.MatchNaive(externals, mirror)

		for _, m := range result.Matches {
			if err := tx.MarkExternalMatched(ctx, m.ExternalID, m.EntryID); err != nil {
				return err
			}
		}
		for _, f := range result.Findings {
			if f.Kind == domain.KindAmountMismatch {
				if err := tx.MarkExternalDisputed(ctx, f.External.ID); err != nil {
					return err
				}
			}
			disc := discrepancyFromFinding(run, f)
			if err := tx.InsertDiscrepancy(ctx, disc); err != nil {
				return err
			}
			env, err := discrepancyEnvelope(run, disc, f)
			if err != nil {
				return err
			}
			if err := tx.InsertOutboxEnvelope(ctx, env); err != nil {
				return err
			}
		}

		now := time.Now().UTC()
		run.Status = domain.RunCompleted
		run.MatchedCount = len(result.Matches)
		run.UnmatchedCount = len(result.Findings)
		run.FinishedAt = &now
		return tx.UpdateRun(ctx, run)
	})
	if err != nil {
		return domain.Run{}, err
	}
	return run, nil
}

// GetRun returns one run of the tenant.
func (s *Service) GetRun(ctx context.Context, tenantID, id uuid.UUID) (domain.Run, error) {
	var out domain.Run
	err := s.store.Within(ctx, tenantID, func(tx TxStore) error {
		var err error
		out, err = tx.GetRun(ctx, id)
		return err
	})
	return out, err
}

// Report returns the per-source reconciliation summary.
func (s *Service) Report(ctx context.Context, tenantID uuid.UUID) ([]domain.SourceSummary, error) {
	var out []domain.SourceSummary
	err := s.store.Within(ctx, tenantID, func(tx TxStore) error {
		var err error
		out, err = tx.SourceSummaries(ctx)
		return err
	})
	return out, err
}

// ListDiscrepancies lists a keyset page of discrepancies, newest first,
// optionally filtered by status.
func (s *Service) ListDiscrepancies(ctx context.Context, tenantID uuid.UUID, status string, limit int, cursor httpx.Cursor) ([]domain.Discrepancy, error) {
	filter := domain.DiscrepancyStatus(status)
	if status != "" && !domain.ValidDiscrepancyStatus(filter) {
		return nil, invalid("status must be one of open, investigating, resolved")
	}
	var out []domain.Discrepancy
	err := s.store.Within(ctx, tenantID, func(tx TxStore) error {
		var err error
		out, err = tx.ListDiscrepancies(ctx, filter, limit, cursor)
		return err
	})
	return out, err
}

// UpdateDiscrepancy changes the triage status and optionally attaches a
// resolution note (kept inside the details JSONB).
func (s *Service) UpdateDiscrepancy(ctx context.Context, tenantID, id uuid.UUID, status domain.DiscrepancyStatus, resolutionNote string) (domain.Discrepancy, error) {
	if !domain.ValidDiscrepancyStatus(status) {
		return domain.Discrepancy{}, invalid("status must be one of open, investigating, resolved")
	}
	var out domain.Discrepancy
	err := s.store.Within(ctx, tenantID, func(tx TxStore) error {
		d, err := tx.GetDiscrepancy(ctx, id)
		if err != nil {
			return err
		}
		d.Status = status
		if resolutionNote != "" {
			if d.Details == nil {
				d.Details = map[string]any{}
			}
			d.Details["resolution_note"] = resolutionNote
		}
		if err := tx.UpdateDiscrepancy(ctx, d); err != nil {
			return err
		}
		out = d
		return nil
	})
	if err != nil {
		return domain.Discrepancy{}, err
	}
	return out, nil
}

// ---- helpers ----------------------------------------------------------------

// newID returns a UUIDv7 (time-sortable). uuid.NewV7 only fails when the
// system clock/entropy source is broken; fall back to v4 in that case.
func newID() uuid.UUID {
	id, err := uuid.NewV7()
	if err != nil {
		return uuid.New()
	}
	return id
}

func discrepancyFromFinding(run domain.Run, f domain.Finding) domain.Discrepancy {
	details := map[string]any{
		"external_id":     f.External.ID.String(),
		"external_ref":    f.External.ExternalRef,
		"external_amount": strconv.FormatInt(f.External.Amount, 10),
		"asset":           f.External.Asset,
		"detail":          f.Detail,
	}
	if f.Entry != nil {
		details["entry_id"] = f.Entry.ID.String()
		details["entry_transaction_id"] = f.Entry.TransactionID.String()
		details["entry_amount"] = strconv.FormatInt(f.Entry.Amount, 10)
		details["entry_asset"] = f.Entry.Asset
	}
	return domain.Discrepancy{
		ID:        newID(),
		TenantID:  run.TenantID,
		RunID:     run.ID,
		Kind:      f.Kind,
		Details:   details,
		Status:    domain.DiscrepancyOpen,
		CreatedAt: time.Now().UTC(),
	}
}

// discrepancyDetectedPayload matches
// contracts/events/recon.discrepancy.detected.schema.json.
type discrepancyDetectedPayload struct {
	DiscrepancyID uuid.UUID  `json:"discrepancy_id"`
	RunID         uuid.UUID  `json:"run_id"`
	SourceID      uuid.UUID  `json:"source_id"`
	Kind          string     `json:"kind"`
	ExternalID    string     `json:"external_id,omitempty"`
	TransactionID *uuid.UUID `json:"transaction_id,omitempty"`
	Asset         string     `json:"asset"`
	Amount        string     `json:"amount"`
	Detail        string     `json:"detail,omitempty"`
	DetectedAt    time.Time  `json:"detected_at"`
}

// eventKind maps DB discrepancy kinds to the wire kinds of the event
// contract. domain.KindDuplicate has no wire equivalent yet (v2).
func eventKind(k domain.DiscrepancyKind) (string, bool) {
	switch k {
	case domain.KindMissingInternal:
		return "missing_in_ledger", true
	case domain.KindMissingExternal:
		return "missing_in_source", true
	case domain.KindAmountMismatch:
		return "amount_mismatch", true
	default:
		return "", false
	}
}

func discrepancyEnvelope(run domain.Run, disc domain.Discrepancy, f domain.Finding) (events.Envelope, error) {
	kind, ok := eventKind(disc.Kind)
	if !ok {
		return events.Envelope{}, fmt.Errorf("discrepancy kind %q has no event mapping", disc.Kind)
	}
	payload := discrepancyDetectedPayload{
		DiscrepancyID: disc.ID,
		RunID:         run.ID,
		SourceID:      run.SourceID,
		Kind:          kind,
		ExternalID:    f.External.ExternalRef,
		Asset:         f.External.Asset,
		Amount:        strconv.FormatInt(f.External.Amount, 10),
		Detail:        f.Detail,
		DetectedAt:    disc.CreatedAt,
	}
	if f.Entry != nil {
		txID := f.Entry.TransactionID
		payload.TransactionID = &txID
	}
	return events.NewEnvelope(events.TopicDiscrepancyDetected, run.TenantID, payload)
}
