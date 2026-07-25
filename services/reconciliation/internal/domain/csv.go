package domain

import (
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/ledgercore/ledgercore/libs/go/money"
)

// DefaultCSVExponent is the decimal exponent assumed for statement amounts
// when a source does not declare one. Amounts like "10.50" become 1050 minor
// units. v2 will read the exponent from a per-tenant asset registry.
const DefaultCSVExponent = 2

// StatementRow is one parsed and normalized row of an imported CSV statement.
type StatementRow struct {
	ExternalRef string
	AmountUnits int64 // minor units, via money.ParseUnits — never floats
	Asset       string
	OccurredAt  time.Time
	Raw         map[string]string // original CSV values for audit (raw JSONB)
}

// csvColumns are the required statement columns, in canonical order.
var csvColumns = [4]string{"external_ref", "amount", "asset", "occurred_at"}

// ParseStatementCSV parses a statement with header
// external_ref,amount,asset,occurred_at (any column order, case-insensitive).
// amount is a decimal string converted to minor units with the given
// exponent; occurred_at is RFC 3339. The first invalid row aborts the parse
// with an error naming the row, so a failed import stores an actionable
// detail message.
func ParseStatementCSV(r io.Reader, exponent int) ([]StatementRow, error) {
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
		units, err := money.ParseUnits(rawAmount, exponent)
		if err != nil {
			return nil, fmt.Errorf("csv: row %d: amount %q: %w", rowNum, rawAmount, err)
		}

		rawOccurred := field("occurred_at")
		occurredAt, err := time.Parse(time.RFC3339, rawOccurred)
		if err != nil {
			return nil, fmt.Errorf("csv: row %d: occurred_at %q is not RFC 3339", rowNum, rawOccurred)
		}

		rows = append(rows, StatementRow{
			ExternalRef: ref,
			AmountUnits: units,
			Asset:       asset,
			OccurredAt:  occurredAt.UTC(),
			Raw: map[string]string{
				"external_ref": ref,
				"amount":       rawAmount,
				"asset":        asset,
				"occurred_at":  rawOccurred,
			},
		})
	}
	return rows, nil
}
