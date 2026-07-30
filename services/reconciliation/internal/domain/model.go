// Package domain holds the pure reconciliation domain: entities, the CSV
// statement parser and the naive matcher. It has no I/O dependencies so every
// rule in this package is unit-testable without a database.
package domain

import (
	"errors"
	"time"

	"github.com/google/uuid"
)

// ErrNotFound is returned when a requested entity does not exist for the
// tenant. Adapters translate their storage-specific "no rows" errors to it.
var ErrNotFound = errors.New("not found")

// SourceKind classifies an external reconciliation source.
type SourceKind string

// Valid source kinds.
const (
	SourceBank     SourceKind = "bank"
	SourcePSP      SourceKind = "psp"
	SourceProvider SourceKind = "provider"
	SourceManual   SourceKind = "manual"
)

// ValidSourceKind reports whether k is one of the known kinds.
func ValidSourceKind(k SourceKind) bool {
	switch k {
	case SourceBank, SourcePSP, SourceProvider, SourceManual:
		return true
	}
	return false
}

// ImportStatus is the lifecycle state of a statement import.
type ImportStatus string

// Import statuses.
const (
	ImportPending   ImportStatus = "pending"
	ImportProcessed ImportStatus = "processed"
	ImportFailed    ImportStatus = "failed"
)

// MatchStatus is the reconciliation state of an external transaction.
type MatchStatus string

// Match statuses.
const (
	MatchUnmatched MatchStatus = "unmatched"
	MatchMatched   MatchStatus = "matched"
	MatchDisputed  MatchStatus = "disputed"
)

// RunStatus is the lifecycle state of a reconciliation run.
type RunStatus string

// Run statuses.
const (
	RunPending   RunStatus = "pending"
	RunRunning   RunStatus = "running"
	RunCompleted RunStatus = "completed"
	RunFailed    RunStatus = "failed"
)

// DiscrepancyKind classifies a reconciliation discrepancy.
type DiscrepancyKind string

// Discrepancy kinds. KindDuplicate is reserved for the v2 matcher and is
// never produced by MatchNaive.
const (
	KindMissingInternal DiscrepancyKind = "missing_internal"
	KindMissingExternal DiscrepancyKind = "missing_external"
	KindAmountMismatch  DiscrepancyKind = "amount_mismatch"
	KindDuplicate       DiscrepancyKind = "duplicate"
)

// DiscrepancyStatus is the triage state of a discrepancy.
type DiscrepancyStatus string

// Discrepancy statuses.
const (
	DiscrepancyOpen          DiscrepancyStatus = "open"
	DiscrepancyInvestigating DiscrepancyStatus = "investigating"
	DiscrepancyResolved      DiscrepancyStatus = "resolved"
)

// ValidDiscrepancyStatus reports whether s is one of the known statuses.
func ValidDiscrepancyStatus(s DiscrepancyStatus) bool {
	switch s {
	case DiscrepancyOpen, DiscrepancyInvestigating, DiscrepancyResolved:
		return true
	}
	return false
}

// Source is an external party whose statements are reconciled against the
// ledger mirror (a bank, a PSP, a payout provider or a manual upload).
type Source struct {
	ID        uuid.UUID
	TenantID  uuid.UUID
	Name      string
	Kind      SourceKind
	CreatedAt time.Time
}

// Import is one uploaded CSV statement.
type Import struct {
	ID        uuid.UUID
	TenantID  uuid.UUID
	SourceID  uuid.UUID
	Filename  string
	Status    ImportStatus
	RowCount  int
	Error     string
	CreatedAt time.Time
}

// ExternalTransaction is one row of an imported statement, normalized to
// integer minor units (never floats).
type ExternalTransaction struct {
	ID          uuid.UUID
	TenantID    uuid.UUID
	ImportID    uuid.UUID
	SourceID    uuid.UUID
	ExternalRef string
	Amount      int64 // minor units
	Asset       string
	// Direction is DEBIT/CREDIT when the statement declares it (optional CSV
	// column), or "" when it does not. An empty direction matches either side
	// (preserves v1 behavior); a declared direction must equal the mirror
	// entry's posting direction to match (LC-011).
	Direction      string
	OccurredAt     time.Time
	Raw            map[string]string // original CSV values, stored as JSONB
	MatchStatus    MatchStatus
	MatchedEntryID *uuid.UUID
}

// Posting directions shared by mirror entries and (optionally) external rows.
const (
	DirectionDebit  = "DEBIT"
	DirectionCredit = "CREDIT"
)

// MirrorEntry is a local copy of one ledger posting, fed exclusively by
// ledger.transaction.posted events (event-carried state transfer). This
// service never queries the ledger schema directly.
type MirrorEntry struct {
	ID            uuid.UUID
	TenantID      uuid.UUID
	TransactionID uuid.UUID
	AccountID     uuid.UUID
	Direction     string // DEBIT | CREDIT
	Amount        int64  // minor units
	Asset         string
	Reference     string // the transaction idempotency_key
	PostedAt      time.Time
}

// Run is one execution of the matcher for a source.
type Run struct {
	ID             uuid.UUID
	TenantID       uuid.UUID
	SourceID       uuid.UUID
	Status         RunStatus
	MatchedCount   int
	UnmatchedCount int
	StartedAt      time.Time
	FinishedAt     *time.Time
}

// Discrepancy is a mismatch found by a run, kept for triage.
type Discrepancy struct {
	ID        uuid.UUID
	TenantID  uuid.UUID
	RunID     uuid.UUID
	Kind      DiscrepancyKind
	Details   map[string]any
	Status    DiscrepancyStatus
	CreatedAt time.Time
}

// SourceSummary is one row of the reconciliation report read model.
type SourceSummary struct {
	SourceID          uuid.UUID
	SourceName        string
	Matched           int
	Unmatched         int
	Disputed          int
	OpenDiscrepancies int
}
