package httpx

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestWriteError(t *testing.T) {
	rec := httptest.NewRecorder()
	WriteError(rec, http.StatusUnprocessableEntity, "unbalanced_transaction", "debits and credits must balance per asset")

	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
		t.Fatalf("content-type = %q", ct)
	}
	var body struct {
		Error struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("body is not JSON: %v", err)
	}
	if body.Error.Code != "unbalanced_transaction" {
		t.Fatalf("code = %q", body.Error.Code)
	}
	if body.Error.Message == "" {
		t.Fatal("message missing")
	}
}

func TestWriteJSON(t *testing.T) {
	rec := httptest.NewRecorder()
	WriteJSON(rec, http.StatusCreated, map[string]string{"id": "abc"})
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d", rec.Code)
	}
	if got := strings.TrimSpace(rec.Body.String()); got != `{"id":"abc"}` {
		t.Fatalf("body = %q", got)
	}
}

func TestDecodeJSON(t *testing.T) {
	type payload struct {
		Name  string `json:"name"`
		Units int64  `json:"units"`
	}

	tests := []struct {
		name    string
		body    string
		wantErr string // substring; empty means success
	}{
		{"valid", `{"name":"cash","units":100}`, ""},
		{"empty body", ``, "empty"},
		{"malformed", `{"name":`, "malformed JSON"},
		{"unknown field", `{"name":"x","nope":1}`, "unknown field"},
		{"wrong type", `{"name":"x","units":"lots"}`, "invalid type"},
		{"trailing garbage", `{"name":"x"}{"name":"y"}`, "single JSON object"},
		{"not json", `hello`, "malformed JSON"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(tt.body))
			rec := httptest.NewRecorder()
			var dst payload
			err := DecodeJSON(rec, req, &dst)
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("DecodeJSON: %v", err)
				}
				if dst.Name != "cash" || dst.Units != 100 {
					t.Fatalf("decoded = %+v", dst)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("error = %v, want substring %q", err, tt.wantErr)
			}
		})
	}
}

func TestDecodeJSONBodyLimit(t *testing.T) {
	big := `{"name":"` + strings.Repeat("a", MaxBodyBytes) + `"}`
	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(big))
	rec := httptest.NewRecorder()
	var dst struct {
		Name string `json:"name"`
	}
	err := DecodeJSON(rec, req, &dst)
	if err == nil || !strings.Contains(err.Error(), "exceeds") {
		t.Fatalf("error = %v, want body-limit error", err)
	}
}

func TestCursorRoundTrip(t *testing.T) {
	orig := Cursor{
		CreatedAt: time.Date(2026, 7, 24, 12, 30, 45, 123456789, time.UTC),
		ID:        uuid.New(),
	}
	wire := orig.Encode()
	got, err := DecodeCursor(wire)
	if err != nil {
		t.Fatalf("DecodeCursor: %v", err)
	}
	if !got.CreatedAt.Equal(orig.CreatedAt) || got.ID != orig.ID {
		t.Fatalf("round trip mismatch: %+v vs %+v", got, orig)
	}
}

func TestDecodeCursorEmptyAndInvalid(t *testing.T) {
	c, err := DecodeCursor("")
	if err != nil || !c.IsZero() {
		t.Fatalf("empty cursor: %+v, %v", c, err)
	}
	for _, bad := range []string{"!!!", "aGVsbG8", "aGVsbG98bm90LWEtdXVpZA"} {
		if _, err := DecodeCursor(bad); err == nil {
			t.Fatalf("DecodeCursor(%q) should fail", bad)
		}
	}
}

func TestRequestIDMiddleware(t *testing.T) {
	var seen string
	h := RequestID(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen = RequestIDFromContext(r.Context())
	}))

	// Generated when absent.
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))
	if seen == "" || rec.Header().Get(RequestIDHeader) != seen {
		t.Fatalf("request id not generated/echoed: %q vs header %q", seen, rec.Header().Get(RequestIDHeader))
	}

	// Propagated when present.
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set(RequestIDHeader, "req-123")
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if seen != "req-123" {
		t.Fatalf("incoming request id not propagated: %q", seen)
	}
}

func TestRecoverMiddleware(t *testing.T) {
	h := Recover(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		panic("boom")
	}))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "internal_error") {
		t.Fatalf("body = %s", rec.Body.String())
	}
}
