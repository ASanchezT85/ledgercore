package domain

import (
	"strings"
	"testing"
	"time"
)

func TestParseStatementCSVValidRows(t *testing.T) {
	in := strings.Join([]string{
		"external_ref,amount,asset,occurred_at",
		"bank-tx-001,10.50,USD,2026-07-24T12:05:00Z",
		"bank-tx-002,-3.07,USD,2026-07-24T13:00:00Z",
		"bank-tx-003,100,MXN,2026-07-24T14:30:00-05:00",
	}, "\n")

	rows, err := ParseStatementCSV(strings.NewReader(in), FixedExponent(DefaultCSVExponent))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(rows) != 3 {
		t.Fatalf("expected 3 rows, got %d", len(rows))
	}

	if rows[0].ExternalRef != "bank-tx-001" || rows[0].AmountUnits != 1050 || rows[0].Asset != "USD" {
		t.Errorf("row 0 parsed wrong: %+v", rows[0])
	}
	if rows[1].AmountUnits != -307 {
		t.Errorf("row 1: expected -307 units, got %d", rows[1].AmountUnits)
	}
	if rows[2].AmountUnits != 10000 {
		t.Errorf("row 2: expected 10000 units (100 with exponent 2), got %d", rows[2].AmountUnits)
	}
	want := time.Date(2026, 7, 24, 19, 30, 0, 0, time.UTC)
	if !rows[2].OccurredAt.Equal(want) {
		t.Errorf("row 2: occurred_at not normalized to UTC: %v", rows[2].OccurredAt)
	}
	if rows[0].Raw["amount"] != "10.50" {
		t.Errorf("row 0: raw amount not preserved: %v", rows[0].Raw)
	}
}

func TestParseStatementCSVHeaderAnyOrder(t *testing.T) {
	in := "asset,occurred_at,external_ref,amount\nUSD,2026-07-24T12:05:00Z,ref-1,1.23\n"
	rows, err := ParseStatementCSV(strings.NewReader(in), FixedExponent(2))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(rows) != 1 || rows[0].AmountUnits != 123 || rows[0].ExternalRef != "ref-1" {
		t.Fatalf("columns mapped wrong: %+v", rows)
	}
}

func TestParseStatementCSVMalformedAmount(t *testing.T) {
	cases := []string{"10,50", "abc", "1.2.3", "", "1.505"} // 1.505: 3 decimals > exponent 2
	for _, amount := range cases {
		in := "external_ref,amount,asset,occurred_at\nref-1,\"" + amount + "\",USD,2026-07-24T12:05:00Z\n"
		_, err := ParseStatementCSV(strings.NewReader(in), FixedExponent(2))
		if err == nil {
			t.Errorf("amount %q: expected error, got nil", amount)
			continue
		}
		if !strings.Contains(err.Error(), "row 2") {
			t.Errorf("amount %q: error should name the row: %v", amount, err)
		}
	}
}

func TestParseStatementCSVMissingAsset(t *testing.T) {
	// Empty asset value in a row.
	in := "external_ref,amount,asset,occurred_at\nref-1,10.00,,2026-07-24T12:05:00Z\n"
	if _, err := ParseStatementCSV(strings.NewReader(in), FixedExponent(2)); err == nil {
		t.Error("empty asset value: expected error, got nil")
	}

	// Asset column absent from the header.
	in = "external_ref,amount,occurred_at\nref-1,10.00,2026-07-24T12:05:00Z\n"
	_, err := ParseStatementCSV(strings.NewReader(in), FixedExponent(2))
	if err == nil || !strings.Contains(err.Error(), `missing required column "asset"`) {
		t.Errorf("missing asset column: wrong error: %v", err)
	}
}

func TestParseStatementCSVInvalidAssetAndDate(t *testing.T) {
	in := "external_ref,amount,asset,occurred_at\nref-1,10.00,usd,2026-07-24T12:05:00Z\n"
	if _, err := ParseStatementCSV(strings.NewReader(in), FixedExponent(2)); err == nil {
		t.Error("lowercase asset: expected error, got nil")
	}

	in = "external_ref,amount,asset,occurred_at\nref-1,10.00,USD,24/07/2026\n"
	if _, err := ParseStatementCSV(strings.NewReader(in), FixedExponent(2)); err == nil {
		t.Error("non-RFC3339 date: expected error, got nil")
	}
}

func TestParseStatementPerAssetExponent(t *testing.T) {
	// JPY has 0 minor-unit places; USD has 2. With the per-asset resolver a
	// single statement mixing both must not assume 2 for JPY.
	in := strings.Join([]string{
		"external_ref,amount,asset,occurred_at",
		"jpy-1,1000,JPY,2026-07-24T12:00:00Z",
		"usd-1,10.50,USD,2026-07-24T12:00:00Z",
	}, "\n")
	rows, err := ParseStatementCSV(strings.NewReader(in), DefaultAssetExponent)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if rows[0].AmountUnits != 1000 {
		t.Errorf("JPY 1000 with exponent 0 should be 1000 units, got %d", rows[0].AmountUnits)
	}
	if rows[1].AmountUnits != 1050 {
		t.Errorf("USD 10.50 with exponent 2 should be 1050 units, got %d", rows[1].AmountUnits)
	}
	// A JPY amount with decimals is invalid at exponent 0.
	bad := "external_ref,amount,asset,occurred_at\njpy-2,10.5,JPY,2026-07-24T12:00:00Z\n"
	if _, err := ParseStatementCSV(strings.NewReader(bad), DefaultAssetExponent); err == nil {
		t.Error("JPY with decimals should be rejected at exponent 0")
	}
}

func TestParseStatementDirectionColumn(t *testing.T) {
	in := strings.Join([]string{
		"external_ref,amount,asset,occurred_at,direction",
		"d-1,10.00,USD,2026-07-24T12:00:00Z,debit",
		"c-1,10.00,USD,2026-07-24T12:00:00Z,CREDIT",
		"n-1,10.00,USD,2026-07-24T12:00:00Z,",
	}, "\n")
	rows, err := ParseStatementCSV(strings.NewReader(in), FixedExponent(2))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if rows[0].Direction != DirectionDebit || rows[1].Direction != DirectionCredit || rows[2].Direction != "" {
		t.Fatalf("directions parsed wrong: %q %q %q", rows[0].Direction, rows[1].Direction, rows[2].Direction)
	}
	if rows[0].Raw["direction"] != "debit" {
		t.Errorf("raw direction not preserved: %v", rows[0].Raw)
	}

	// Invalid direction value is rejected with a row-scoped error.
	bad := "external_ref,amount,asset,occurred_at,direction\nx,10.00,USD,2026-07-24T12:00:00Z,sideways\n"
	if _, err := ParseStatementCSV(strings.NewReader(bad), FixedExponent(2)); err == nil {
		t.Error("invalid direction should be rejected")
	}
}

func TestParseStatementCSVEmptyFile(t *testing.T) {
	if _, err := ParseStatementCSV(strings.NewReader(""), FixedExponent(2)); err == nil {
		t.Error("empty file: expected error, got nil")
	}
}

func TestParseStatementCSVNoDataRows(t *testing.T) {
	rows, err := ParseStatementCSV(strings.NewReader("external_ref,amount,asset,occurred_at\n"), FixedExponent(2))
	if err != nil {
		t.Fatalf("header-only file should parse: %v", err)
	}
	if len(rows) != 0 {
		t.Fatalf("expected 0 rows, got %d", len(rows))
	}
}
