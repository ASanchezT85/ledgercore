package httpx

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestParsePageDefaults(t *testing.T) {
	rec := httptest.NewRecorder()
	page, ok := ParsePage(rec, httptest.NewRequest(http.MethodGet, "/v1/things", nil))
	if !ok {
		t.Fatalf("expected ok, body=%s", rec.Body.String())
	}
	if page.Limit != DefaultLimit {
		t.Fatalf("limit = %d, want %d", page.Limit, DefaultLimit)
	}
	if !page.Cursor.IsZero() {
		t.Fatal("expected zero cursor on first page")
	}
	if page.FetchLimit() != DefaultLimit+1 {
		t.Fatalf("fetch limit = %d, want %d", page.FetchLimit(), DefaultLimit+1)
	}
}

func TestParsePageClampsToMax(t *testing.T) {
	rec := httptest.NewRecorder()
	page, ok := ParsePage(rec, httptest.NewRequest(http.MethodGet, "/v1/things?limit=9999", nil))
	if !ok || page.Limit != MaxLimit {
		t.Fatalf("limit = %d ok=%v, want clamp to %d", page.Limit, ok, MaxLimit)
	}
}

func TestParsePageRejectsBadLimit(t *testing.T) {
	for _, raw := range []string{"0", "-1", "abc", "1.5"} {
		rec := httptest.NewRecorder()
		_, ok := ParsePage(rec, httptest.NewRequest(http.MethodGet, "/v1/things?limit="+raw, nil))
		if ok {
			t.Fatalf("limit=%q accepted, want 400", raw)
		}
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("limit=%q status=%d, want 400", raw, rec.Code)
		}
		var body struct {
			Error struct {
				Code string `json:"code"`
			} `json:"error"`
		}
		_ = json.Unmarshal(rec.Body.Bytes(), &body)
		if body.Error.Code != CodeValidationFailed {
			t.Fatalf("limit=%q code=%q, want %q", raw, body.Error.Code, CodeValidationFailed)
		}
	}
}

func TestParsePageRejectsMalformedCursor(t *testing.T) {
	for _, raw := range []string{"garbage!!", "bm90LWEtY3Vyc29y", "dGltZXxub3QtYS11dWlk"} {
		rec := httptest.NewRecorder()
		_, ok := ParsePage(rec, httptest.NewRequest(http.MethodGet, "/v1/things?cursor="+raw, nil))
		if ok {
			t.Fatalf("cursor=%q accepted, want 400", raw)
		}
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("cursor=%q status=%d, want 400", raw, rec.Code)
		}
		var body struct {
			Error struct {
				Code      string `json:"code"`
				RequestID string `json:"request_id"`
			} `json:"error"`
		}
		_ = json.Unmarshal(rec.Body.Bytes(), &body)
		if body.Error.Code != CodeInvalidCursor {
			t.Fatalf("cursor=%q code=%q, want %q", raw, body.Error.Code, CodeInvalidCursor)
		}
	}
}

type row struct {
	CreatedAt time.Time
	ID        uuid.UUID
}

func rowKey(r row) Cursor { return Cursor{CreatedAt: r.CreatedAt, ID: r.ID} }

func makeRows(n int) []row {
	out := make([]row, n)
	base := time.Now().UTC()
	for i := range out {
		out[i] = row{CreatedAt: base.Add(-time.Duration(i) * time.Minute), ID: uuid.New()}
	}
	return out
}

// The store fetches limit+1 rows; Window must trim and only then emit a
// cursor. A page that came back with <= limit rows is the last one and must
// report next_cursor null WITHOUT the client fetching an extra empty page.
func TestWindow(t *testing.T) {
	t.Run("more pages", func(t *testing.T) {
		rows := makeRows(6) // fetched limit+1 = 6 for limit 5
		page, next := Window(rows, 5, rowKey)
		if len(page) != 5 {
			t.Fatalf("len = %d, want 5", len(page))
		}
		if next == nil {
			t.Fatal("expected non-nil next_cursor")
		}
		c, err := DecodeCursor(*next)
		if err != nil {
			t.Fatalf("emitted cursor does not round-trip: %v", err)
		}
		if !c.CreatedAt.Equal(page[4].CreatedAt) || c.ID != page[4].ID {
			t.Fatalf("cursor points at %v, want last returned row", c)
		}
	})
	t.Run("exactly full last page", func(t *testing.T) {
		rows := makeRows(5) // store returned only 5 of the 6 requested
		page, next := Window(rows, 5, rowKey)
		if len(page) != 5 || next != nil {
			t.Fatalf("len=%d next=%v, want 5 rows and nil cursor", len(page), next)
		}
	})
	t.Run("partial last page", func(t *testing.T) {
		page, next := Window(makeRows(2), 5, rowKey)
		if len(page) != 2 || next != nil {
			t.Fatalf("len=%d next=%v, want 2 rows and nil cursor", len(page), next)
		}
	})
	t.Run("empty", func(t *testing.T) {
		page, next := Window([]row{}, 5, rowKey)
		if len(page) != 0 || next != nil {
			t.Fatalf("want empty page and nil cursor, got len=%d next=%v", len(page), next)
		}
	})
}
