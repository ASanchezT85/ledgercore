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

// MatchNaive is the deliberately NAIVE v1 matcher, documented as such.
//
// v1 semantics:
//   - An external transaction matches a mirror entry when external_ref equals
//     the mirror reference AND amount AND asset are identical. Posting
//     direction (DEBIT/CREDIT) is ignored in v1.
//   - Each mirror entry backs at most one match: it is consumed on first use.
//   - external_ref present in the mirror but with a different amount or asset
//     on every free candidate -> amount_mismatch finding.
//   - external_ref absent from the mirror -> missing_internal finding.
//
// Left for v2 (out of scope here, see README):
//   - Mirror entries with no external counterpart (missing_external): the
//     mirror is not scoped per source yet, so flagging them would report every
//     posting of other rails as missing.
//   - duplicate detection (same external_ref appearing twice in a statement).
func MatchNaive(externals []ExternalTransaction, mirror []MirrorEntry) MatchResult {
	byRef := make(map[string][]int, len(mirror))
	for i := range mirror {
		byRef[mirror[i].Reference] = append(byRef[mirror[i].Reference], i)
	}
	consumed := make([]bool, len(mirror))

	var res MatchResult
	for _, ext := range externals {
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
			if mirror[i].Amount == ext.Amount && mirror[i].Asset == ext.Asset {
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
					"external %q reports %d %s but ledger mirror has %d %s for the same reference",
					ext.ExternalRef, ext.Amount, ext.Asset, entry.Amount, entry.Asset,
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
