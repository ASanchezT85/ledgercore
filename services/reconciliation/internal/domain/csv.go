package domain

import (
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/ASanchezT85/ledgercore/libs/go/money"
)

// DefaultCSVExponent is the fallback decimal exponent for assets not covered
// by DefaultAssetExponent. Amounts like "10.50" become 1050 minor units.
const DefaultCSVExponent = 2

// ExponentFunc resolves the decimal exponent (number of minor-unit places)
// for a given asset code. Making the exponent per-asset avoids the old bug of
// assuming 2 for every currency: JPY has 0, most fiat 2, some 3. v2 will read
// these from a per-tenant asset registry; DefaultAssetExponent is the bounded
// stand-in.
type ExponentFunc func(asset string) int

// assetExponents holds the exponents that differ from the default of 2.
var assetExponents = map[string]int{
	"JPY": 0, "KRW": 0, "CLP": 0, "VND": 0, "ISK": 0,
	"BHD": 3, "KWD": 3, "OMR": 3, "TND": 3,
}

// DefaultAssetExponent returns the conventional minor-unit exponent for an
// asset, falling back to DefaultCSVExponent when the asset is unknown.
func DefaultAssetExponent(asset string) int {
	if e, ok := assetExponents[strings.ToUpper(asset)]; ok {
		return e
	}
	return DefaultCSVExponent
}

// FixedExponent returns an ExponentFunc that ignores the asset and always
// returns n. Useful for tests and single-currency statements.
func FixedExponent(n int) ExponentFunc {
	return func(string) int { return n }
}

// StatementRow is one parsed and normalized row of an imported CSV statement.
type StatementRow struct {
	ExternalRef string
	AmountUnits int64 // minor units, via money.ParseUnits — never floats
	Asset       string
	// Direction is DEBIT/CREDIT when the optional "direction" column is
	// present, or "" when it is absent (see ExternalTransaction.Direction).
	Direction  string
	OccurredAt time.Time
	Raw        map[string]string // original CSV values for audit (raw JSONB)
}

// csvColumns are the required statement columns, in canonical order.
var csvColumns = [4]string{"external_ref", "amount", "asset", "occurred_at"}

// ParseStatementCSV parses a statement with header
// external_ref,amount,asset,occurred_at (any column order, case-insensitive)
// plus an OPTIONAL "direction" column (debit/credit). amount is a decimal
// string converted to minor units using the per-asset exponent from exp;
// occurred_at is RFC 3339. The first invalid row aborts the parse with an
// error naming the row, so a failed import stores an actionable detail
// message.
func ParseStatementCSV(r io.Reader, exp ExponentFunc) ([]StatementRow, error) {
	if exp == nil {
		exp = DefaultAssetExponent
	}
	cr := csv.NewReader(r)
	cr.TrimLeadingSpace = true

	header, err := cr.Read()
	if errors.Is(err, io.EOF) {
		return nil, errors.New("csv: file is empty, expected header external_ref,amount,asset,occurred_at")
	}
	if err != nil {
		return nil, fmt.Errorf("csv: read header: %w", err)
	}

	colIdx := make(map[string]int, len(header))
	for i, col := range header {
		colIdx[strings.ToLower(strings.TrimSpace(col))] = i
	}
	for _, col := range csvColumns {
		if _, ok := colIdx[col]; !ok {
			return nil, fmt.Errorf("csv: missing required column %q", col)
		}
	}

	var rows []StatementRow
	rowNum := 1 // header is row 1
	for {
		rec, err := cr.Read()
		if errors.Is(err, io.EOF) {
			break
		}
		rowNum++
		if err != nil {
			return nil, fmt.Errorf("csv: row %d: %w", rowNum, err)
		}

		field := func(col string) string {
			i := colIdx[col]
			if i < len(rec) {
				return strings.TrimSpace(rec[i])
			}
			return ""
		}

		ref := field("external_ref")
		if ref == "" {
			return nil, fmt.Errorf("csv: row %d: external_ref is required", rowNum)
		}
		if len(ref) > 255 {
			return nil, fmt.Errorf("csv: row %d: external_ref exceeds 255 characters", rowNum)
		}

		asset := field("asset")
		if asset == "" {
			return nil, fmt.Errorf("csv: row %d: asset is required", rowNum)
		}
		if err := money.ValidateAssetCode(asset); err != nil {
			return nil, fmt.Errorf("csv: row %d: %w", rowNum, err)
		}

		rawAmount := field("amount")
		units, err := money.ParseUnits(rawAmount, exp(asset))
		if err != nil {
			return nil, fmt.Errorf("csv: row %d: amount %q: %w", rowNum, rawAmount, err)
		}

		rawOccurred := field("occurred_at")
		occurredAt, err := time.Parse(time.RFC3339, rawOccurred)
		if err != nil {
			return nil, fmt.Errorf("csv: row %d: occurred_at %q is not RFC 3339", rowNum, rawOccurred)
		}

		// Optional direction column. Absent -> "" (matches either side).
		direction := ""
		rawDirection := ""
		if _, ok := colIdx["direction"]; ok {
			rawDirection = field("direction")
			switch strings.ToUpper(rawDirection) {
			case "":
				direction = ""
			case DirectionDebit:
				direction = DirectionDebit
			case DirectionCredit:
				direction = DirectionCredit
			default:
				return nil, fmt.Errorf("csv: row %d: direction %q must be debit or credit", rowNum, rawDirection)
			}
		}

		raw := map[string]string{
			"external_ref": ref,
			"amount":       rawAmount,
			"asset":        asset,
			"occurred_at":  rawOccurred,
		}
		if rawDirection != "" {
			raw["direction"] = rawDirection
		}
		rows = append(rows, StatementRow{
			ExternalRef: ref,
			AmountUnits: units,
			Asset:       asset,
			Direction:   direction,
			OccurredAt:  occurredAt.UTC(),
			Raw:         raw,
		})
	}
	return rows, nil
}
