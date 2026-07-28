// Package httpx contains the HTTP building blocks shared by every service:
// middlewares (request id, panic recovery, structured logging, dev CORS),
// JSON helpers with the platform error contract, and keyset pagination
// cursors.
package httpx

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
)

// ---- Request ID -----------------------------------------------------------

type requestIDKey struct{}

// RequestIDHeader is the canonical request id header.
const RequestIDHeader = "X-Request-Id"

// RequestIDFromContext returns the request id set by the RequestID middleware.
func RequestIDFromContext(ctx context.Context) string {
	id, _ := ctx.Value(requestIDKey{}).(string)
	return id
}

// RequestID assigns each request a unique id (or propagates the incoming
// X-Request-Id) and echoes it back in the response headers.
func RequestID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := r.Header.Get(RequestIDHeader)
		if id == "" || len(id) > 128 {
			id = newRequestID()
		}
		w.Header().Set(RequestIDHeader, id)
		ctx := context.WithValue(r.Context(), requestIDKey{}, id)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func newRequestID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return uuid.NewString()
	}
	return hex.EncodeToString(b[:])
}

// ---- Recover ----------------------------------------------------------------

// Recover converts panics into a 500 JSON error and logs the stack via slog.
func Recover(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if rec := recover(); rec != nil {
				if rec == http.ErrAbortHandler { //nolint:errorlint // sentinel comparison per net/http docs
					panic(rec)
				}
				slog.Error("panic recovered",
					"panic", fmt.Sprint(rec),
					"method", r.Method,
					"path", r.URL.Path,
					"request_id", RequestIDFromContext(r.Context()),
				)
				WriteError(w, r, http.StatusInternalServerError, CodeInternal, "an internal error occurred")
			}
		}()
		next.ServeHTTP(w, r)
	})
}

// ---- Logger -----------------------------------------------------------------

type statusRecorder struct {
	http.ResponseWriter
	status int
	bytes  int
}

func (s *statusRecorder) WriteHeader(code int) {
	s.status = code
	s.ResponseWriter.WriteHeader(code)
}

func (s *statusRecorder) Write(b []byte) (int, error) {
	if s.status == 0 {
		s.status = http.StatusOK
	}
	n, err := s.ResponseWriter.Write(b)
	s.bytes += n
	return n, err
}

// Logger emits one structured log line per request: method, path, status,
// duration and request id.
func Logger(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rec := &statusRecorder{ResponseWriter: w}
		next.ServeHTTP(rec, r)
		if rec.status == 0 {
			rec.status = http.StatusOK
		}
		slog.Info("http request",
			"method", r.Method,
			"path", r.URL.Path,
			"status", rec.status,
			"duration_ms", time.Since(start).Milliseconds(),
			"bytes", rec.bytes,
			"request_id", RequestIDFromContext(r.Context()),
		)
	})
}

// ---- CORS (dev) ---------------------------------------------------------------

// CORSDev is a permissive CORS middleware for local development only.
// Production traffic goes through the gateway, which owns CORS there.
func CORSDev(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		if origin != "" {
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Vary", "Origin")
			w.Header().Set("Access-Control-Allow-Credentials", "true")
			w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type, X-Tenant-Id, X-Request-Id, Idempotency-Key")
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
		}
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// ---- Error catalog ---------------------------------------------------------------

// Canonical, stable error codes of the platform API. Every 4xx/5xx response
// of every service uses one of these (documented in docs/errores-api.md).
const (
	CodeValidationFailed      = "validation_failed"      // 400: malformed or semantically invalid input
	CodeInvalidCursor         = "invalid_cursor"         // 400: malformed pagination cursor
	CodeUnauthorized          = "unauthorized"           // 401: missing/invalid credentials or tenant context
	CodeForbidden             = "forbidden"              // 403: authenticated but not allowed
	CodeNotFound              = "not_found"              // 404: resource does not exist within the tenant
	CodeConflict              = "conflict"               // 409: uniqueness or lifecycle-state conflict
	CodeIdempotencyConflict   = "idempotency_conflict"   // 409: idempotency key reused with a different payload
	CodeUnbalancedTransaction = "unbalanced_transaction" // 422: postings do not balance per asset
	CodeInsufficientFunds     = "insufficient_funds"     // 422: available balance cannot cover the operation
	CodeRateLimited           = "rate_limited"           // 429: caller exceeded a rate/usage limit
	CodeServiceUnavailable    = "service_unavailable"    // 503: dependency (e.g. database) unavailable
	CodeInternal              = "internal"               // 500: unexpected error, details only in logs
)

// ---- JSON helpers ---------------------------------------------------------------

// errorBody is the platform-wide API error contract:
// {"error":{"code","message","request_id"}}.
type errorBody struct {
	Error struct {
		Code      string `json:"code"`
		Message   string `json:"message"`
		RequestID string `json:"request_id"`
	} `json:"error"`
}

// WriteJSON serializes v with the given status. Encoding failures are logged;
// headers are already sent at that point so the body may be truncated.
func WriteJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if v == nil {
		return
	}
	if err := json.NewEncoder(w).Encode(v); err != nil {
		slog.Error("write json response", "error", err)
	}
}

// WriteError writes the standard error shape
// {"error":{"code","message","request_id"}}. The request id comes from the
// RequestID middleware via r's context.
func WriteError(w http.ResponseWriter, r *http.Request, status int, code, msg string) {
	var body errorBody
	body.Error.Code = code
	body.Error.Message = msg
	if r != nil {
		body.Error.RequestID = RequestIDFromContext(r.Context())
	}
	WriteJSON(w, status, body)
}

// MaxBodyBytes is the request body limit enforced by DecodeJSON.
const MaxBodyBytes = 1 << 20 // 1 MiB

// DecodeJSON decodes the request body into dst, enforcing a 1 MiB limit and
// rejecting unknown fields and trailing garbage. Errors are safe to expose to
// clients.
func DecodeJSON(w http.ResponseWriter, r *http.Request, dst any) error {
	r.Body = http.MaxBytesReader(w, r.Body, MaxBodyBytes)
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()

	if err := dec.Decode(dst); err != nil {
		var maxBytesErr *http.MaxBytesError
		var syntaxErr *json.SyntaxError
		var typeErr *json.UnmarshalTypeError
		switch {
		case errors.As(err, &maxBytesErr):
			return fmt.Errorf("request body exceeds %d bytes", MaxBodyBytes)
		case errors.Is(err, io.EOF):
			return errors.New("request body is empty")
		case errors.Is(err, io.ErrUnexpectedEOF):
			return errors.New("malformed JSON: unexpected end of body")
		case errors.As(err, &syntaxErr):
			return fmt.Errorf("malformed JSON at offset %d", syntaxErr.Offset)
		case errors.As(err, &typeErr):
			return fmt.Errorf("invalid type for field %q", typeErr.Field)
		case strings.HasPrefix(err.Error(), "json: unknown field "):
			field := strings.TrimPrefix(err.Error(), "json: unknown field ")
			return fmt.Errorf("unknown field %s", field)
		default:
			return errors.New("invalid JSON body")
		}
	}
	if dec.More() {
		return errors.New("request body must contain a single JSON object")
	}
	return nil
}

// ---- Keyset pagination cursor ----------------------------------------------------

// Cursor is an opaque keyset-pagination cursor over (created_at, id).
// The wire form is base64url of "<RFC3339Nano>|<uuid>".
type Cursor struct {
	CreatedAt time.Time
	ID        uuid.UUID
}

// Encode returns the opaque wire representation of the cursor.
func (c Cursor) Encode() string {
	raw := c.CreatedAt.UTC().Format(time.RFC3339Nano) + "|" + c.ID.String()
	return base64.RawURLEncoding.EncodeToString([]byte(raw))
}

// DecodeCursor parses a wire cursor. An empty string yields a zero cursor
// (first page) with no error.
func DecodeCursor(s string) (Cursor, error) {
	if s == "" {
		return Cursor{}, nil
	}
	raw, err := base64.RawURLEncoding.DecodeString(s)
	if err != nil {
		return Cursor{}, errors.New("httpx: invalid cursor encoding")
	}
	parts := strings.SplitN(string(raw), "|", 2)
	if len(parts) != 2 {
		return Cursor{}, errors.New("httpx: invalid cursor format")
	}
	ts, err := time.Parse(time.RFC3339Nano, parts[0])
	if err != nil {
		return Cursor{}, errors.New("httpx: invalid cursor timestamp")
	}
	id, err := uuid.Parse(parts[1])
	if err != nil {
		return Cursor{}, errors.New("httpx: invalid cursor id")
	}
	return Cursor{CreatedAt: ts, ID: id}, nil
}

// IsZero reports whether the cursor is the first-page (empty) cursor.
func (c Cursor) IsZero() bool {
	return c.CreatedAt.IsZero() && c.ID == uuid.Nil
}

// ---- Uniform pagination -----------------------------------------------------------

// Pagination limits shared by every collection endpoint of the platform:
// ?limit= defaults to DefaultLimit and is clamped to MaxLimit.
const (
	DefaultLimit = 50
	MaxLimit     = 200
)

// PageRequest is a parsed, validated pagination request.
type PageRequest struct {
	Limit  int // requested page size, in [1, MaxLimit]
	Cursor Cursor
}

// FetchLimit is the number of rows the storage layer should fetch: one more
// than the page size, so the presence of a next page is known without a
// second query.
func (p PageRequest) FetchLimit() int { return p.Limit + 1 }

// ParsePage reads ?limit= and ?cursor= from the request. On invalid input it
// writes the standard 400 error (validation_failed or invalid_cursor) and
// returns ok=false. Limits above MaxLimit are clamped, not rejected.
func ParsePage(w http.ResponseWriter, r *http.Request) (PageRequest, bool) {
	page := PageRequest{Limit: DefaultLimit}
	if raw := r.URL.Query().Get("limit"); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil || n < 1 {
			WriteError(w, r, http.StatusBadRequest, CodeValidationFailed,
				"limit must be a positive integer")
			return PageRequest{}, false
		}
		if n > MaxLimit {
			n = MaxLimit
		}
		page.Limit = n
	}
	cursor, err := DecodeCursor(r.URL.Query().Get("cursor"))
	if err != nil {
		WriteError(w, r, http.StatusBadRequest, CodeInvalidCursor,
			"cursor is malformed; use the next_cursor value returned by a previous page")
		return PageRequest{}, false
	}
	page.Cursor = cursor
	return page, true
}

// Window trims a fetched slice of FetchLimit() rows down to the page size and
// computes the next cursor. It returns nil when there are no more results, so
// clients never need an extra empty page. keyOf extracts the keyset cursor of
// a row.
func Window[T any](items []T, limit int, keyOf func(T) Cursor) ([]T, *string) {
	if len(items) <= limit {
		return items, nil
	}
	items = items[:limit]
	c := keyOf(items[len(items)-1]).Encode()
	return items, &c
}

// ListResponse is the uniform collection envelope: {"data":[...],
// "next_cursor": "..." | null}.
type ListResponse[T any] struct {
	Data       []T     `json:"data"`
	NextCursor *string `json:"next_cursor"`
}
