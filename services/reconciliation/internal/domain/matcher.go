package domain

import (
	"fmt"

	"github.com/google/uuid"
)

// MatchPair links an external transaction to the mirror entry that backs it.
type MatchPair struct {
	ExternalID uuid.UUID
	EntryID    uuid.UUID
}

// Finding is a discrepancy detected by the matcher, before persistence.
// Entry is the closest mirror candidate when one exists (amount_mismatch).
type Finding struct {
	Kind     DiscrepancyKind
	External ExternalTransaction
	Entry    *MirrorEntry
	Detail   string
}

// MatchResult is the outcome of a matcher pass. Every input external
// transaction ends up in exactly one of Matches or Findings.
type MatchResult struct {
	Matches  []MatchPair
	Findings []Finding
}

// MatchNaive is the bounded v1 matcher (LC-011 hardening applied on top of
// the original naive matcher — still not the full v2).
//
// Semantics:
//   - An external transaction matches a mirror entry when external_ref equals
//     the mirror reference AND amount AND asset are identical AND, when the
//     external row declares a Direction, it equals the mirror entry's posting
//     direction. A blank external Direction matches either side (statements
//     that do not carry direction keep v1 behavior).
//   - Duplicate guard: if two external rows in the same batch share an
//     external_ref, only the FIRST is matched normally; each later row with
//     that ref is flagged KindDuplicate (and never silently matched or
//     re-counted as missing). Matching two statement lines to one ledger
//     posting is a reconciliation hazard, so it is surfaced, not swallowed.
//   - Each mirror entry backs at most one match: it is consumed on first use.
//   - external_ref present in the mirror but with a different amount/asset/
//     direction on every free candidate -> amount_mismatch finding.
//   - external_ref absent from the mirror -> missing_internal finding.
//
// Left for v2 (out of scope here, see README):
//   - Mirror entries with no external counterpart (missing_external): the
//     mirror is not scoped per source yet, so flagging them would report every
//     posting of other rails as missing.
//   - per-provider adapters and fuzzy/date-window matching.
func MatchNaive(externals []ExternalTransaction, mirror []MirrorEntry) MatchResult {
	byRef := make(map[string][]int, len(mirror))
	for i := range mirror {
		byRef[mirror[i].Reference] = append(byRef[mirror[i].Reference], i)
	}
	consumed := make([]bool, len(mirror))
	seenRef := make(map[string]bool, len(externals))

	var res MatchResult
	for _, ext := range externals {
		// Duplicate guard: a repeated external_ref within one batch is
		// suspicious — flag it rather than let it consume a second entry or
		// masquerade as missing_internal.
		if seenRef[ext.ExternalRef] {
			res.Findings = append(res.Findings, Finding{
				Kind:     KindDuplicate,
				External: ext,
				Detail: fmt.Sprintf(
					"external_ref %q appears more than once in this statement (%d %s); duplicate rows are not matched automatically",
					ext.ExternalRef, ext.Amount, ext.Asset,
				),
			})
			continue
		}
		seenRef[ext.ExternalRef] = true

		candidates := byRef[ext.ExternalRef]

		matchedIdx := -1
		firstFreeIdx := -1
		for _, i := range candidates {
			if consumed[i] {
				continue
			}
			if firstFreeIdx < 0 {
				firstFreeIdx = i
			}
			if mirror[i].Amount == ext.Amount && mirror[i].Asset == ext.Asset && directionMatches(ext.Direction, mirror[i].Direction) {
				matchedIdx = i
				break
			}
		}

		switch {
		case matchedIdx >= 0:
			consumed[matchedIdx] = true
			res.Matches = append(res.Matches, MatchPair{ExternalID: ext.ID, EntryID: mirror[matchedIdx].ID})
		case firstFreeIdx >= 0:
			entry := mirror[firstFreeIdx]
			res.Findings = append(res.Findings, Finding{
				Kind:     KindAmountMismatch,
				External: ext,
				Entry:    &entry,
				Detail: fmt.Sprintf(
					"external %q reports %d %s %s but ledger mirror has %d %s %s for the same reference",
					ext.ExternalRef, ext.Amount, ext.Asset, directionLabel(ext.Direction),
					entry.Amount, entry.Asset, entry.Direction,
				),
			})
		default:
			res.Findings = append(res.Findings, Finding{
				Kind:     KindMissingInternal,
				External: ext,
				Detail:   fmt.Sprintf("external %q (%d %s) has no matching ledger entry", ext.ExternalRef, ext.Amount, ext.Asset),
			})
		}
	}
	return res
}

// directionMatches reports whether an external row's declared direction is
// compatible with a mirror entry's posting direction. A blank external
// direction is treated as "unspecified" and matches either side.
func directionMatches(external, mirror string) bool {
	return external == "" || external == mirror
}

func directionLabel(dir string) string {
	if dir == "" {
		return "(direction unspecified)"
	}
	return dir
}
