package httpapi

import (
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/ASanchezT85/ledgercore/libs/go/httpx"
	"github.com/ASanchezT85/ledgercore/libs/go/ident"

	"github.com/ASanchezT85/ledgercore/services/reconciliation/internal/app"
	"github.com/ASanchezT85/ledgercore/services/reconciliation/internal/domain"
)

// maxImportBytes caps a CSV upload at 10 MiB.
const maxImportBytes = 10 << 20

// tenant extracts the authenticated tenant. RequireAuth guarantees it exists
// on /v1 routes; the guard is defensive.
func tenant(w http.ResponseWriter, r *http.Request) (uuid.UUID, bool) {
	id, ok := ident.TenantFromContext(r.Context())
	if !ok || id == uuid.Nil {
		httpx.WriteError(w, r, http.StatusUnauthorized, httpx.CodeUnauthorized, "request has no tenant context")
		return uuid.Nil, false
	}
	return id, true
}

// writeServiceError maps use-case errors to the platform error contract.
func writeServiceError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, domain.ErrNotFound):
		httpx.WriteError(w, r, http.StatusNotFound, httpx.CodeNotFound, "resource not found")
	case errors.Is(err, app.ErrValidation):
		httpx.WriteError(w, r, http.StatusBadRequest, httpx.CodeValidationFailed, err.Error())
	default:
		slog.Error("request failed",
			"error", err,
			"path", r.URL.Path,
			"request_id", httpx.RequestIDFromContext(r.Context()),
		)
		httpx.WriteError(w, r, http.StatusInternalServerError, httpx.CodeInternal, "an internal error occurred")
	}
}

func pathUUID(w http.ResponseWriter, r *http.Request) (uuid.UUID, bool) {
	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		httpx.WriteError(w, r, http.StatusBadRequest, httpx.CodeValidationFailed, "path id must be a valid UUID")
		return uuid.Nil, false
	}
	return id, true
}

// ---- wire DTOs ---------------------------------------------------------------

type sourceJSON struct {
	ID        uuid.UUID `json:"id"`
	Name      string    `json:"name"`
	Kind      string    `json:"kind"`
	CreatedAt time.Time `json:"created_at"`
}

func toSourceJSON(s domain.Source) sourceJSON {
	return sourceJSON{ID: s.ID, Name: s.Name, Kind: string(s.Kind), CreatedAt: s.CreatedAt}
}

type importJSON struct {
	ID        uuid.UUID `json:"id"`
	SourceID  uuid.UUID `json:"source_id"`
	Filename  string    `json:"filename"`
	Status    string    `json:"status"`
	RowCount  int       `json:"row_count"`
	Error     string    `json:"error,omitempty"`
	CreatedAt time.Time `json:"created_at"`
}

func toImportJSON(i domain.Import) importJSON {
	return importJSON{
		ID: i.ID, SourceID: i.SourceID, Filename: i.Filename,
		Status: string(i.Status), RowCount: i.RowCount, Error: i.Error, CreatedAt: i.CreatedAt,
	}
}

type runJSON struct {
	ID             uuid.UUID  `json:"id"`
	SourceID       uuid.UUID  `json:"source_id"`
	Status         string     `json:"status"`
	MatchedCount   int        `json:"matched_count"`
	UnmatchedCount int        `json:"unmatched_count"`
	StartedAt      time.Time  `json:"started_at"`
	FinishedAt     *time.Time `json:"finished_at,omitempty"`
}

func toRunJSON(r domain.Run) runJSON {
	return runJSON{
		ID: r.ID, SourceID: r.SourceID, Status: string(r.Status),
		MatchedCount: r.MatchedCount, UnmatchedCount: r.UnmatchedCount,
		StartedAt: r.StartedAt, FinishedAt: r.FinishedAt,
	}
}

type discrepancyJSON struct {
	ID        uuid.UUID      `json:"id"`
	RunID     uuid.UUID      `json:"run_id"`
	Kind      string         `json:"kind"`
	Details   map[string]any `json:"details"`
	Status    string         `json:"status"`
	CreatedAt time.Time      `json:"created_at"`
}

func toDiscrepancyJSON(d domain.Discrepancy) discrepancyJSON {
	details := d.Details
	if details == nil {
		details = map[string]any{}
	}
	return discrepancyJSON{
		ID: d.ID, RunID: d.RunID, Kind: string(d.Kind),
		Details: details, Status: string(d.Status), CreatedAt: d.CreatedAt,
	}
}

type summaryJSON struct {
	SourceID          uuid.UUID `json:"source_id"`
	SourceName        string    `json:"source_name"`
	Matched           int       `json:"matched"`
	Unmatched         int       `json:"unmatched"`
	Disputed          int       `json:"disputed"`
	OpenDiscrepancies int       `json:"open_discrepancies"`
}

// ---- handlers ------------------------------------------------------------------

func (s *Server) createSource(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := tenant(w, r)
	if !ok {
		return
	}
	var body struct {
		Name string `json:"name"`
		Kind string `json:"kind"`
	}
	if err := httpx.DecodeJSON(w, r, &body); err != nil {
		httpx.WriteError(w, r, http.StatusBadRequest, httpx.CodeValidationFailed, err.Error())
		return
	}
	src, err := s.svc.CreateSource(r.Context(), tenantID, body.Name, domain.SourceKind(body.Kind))
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusCreated, toSourceJSON(src))
}

func (s *Server) listSources(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := tenant(w, r)
	if !ok {
		return
	}
	page, ok := httpx.ParsePage(w, r)
	if !ok {
		return
	}
	sources, err := s.svc.ListSources(r.Context(), tenantID, page.FetchLimit(), page.Cursor)
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	sources, next := httpx.Window(sources, page.Limit, func(src domain.Source) httpx.Cursor {
		return httpx.Cursor{CreatedAt: src.CreatedAt, ID: src.ID}
	})
	resp := httpx.ListResponse[sourceJSON]{Data: make([]sourceJSON, 0, len(sources)), NextCursor: next}
	for _, src := range sources {
		resp.Data = append(resp.Data, toSourceJSON(src))
	}
	httpx.WriteJSON(w, http.StatusOK, resp)
}

// createImport accepts multipart/form-data with fields:
//   - source_id: UUID of a registered source
//   - file: the CSV statement (external_ref,amount,asset,occurred_at)
func (s *Server) createImport(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := tenant(w, r)
	if !ok {
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxImportBytes)
	if err := r.ParseMultipartForm(4 << 20); err != nil {
		httpx.WriteError(w, r, http.StatusBadRequest, httpx.CodeValidationFailed,
			"body must be multipart/form-data with fields source_id and file (max 10 MiB)")
		return
	}
	defer func() {
		if r.MultipartForm != nil {
			_ = r.MultipartForm.RemoveAll()
		}
	}()

	sourceID, err := uuid.Parse(r.FormValue("source_id"))
	if err != nil {
		httpx.WriteError(w, r, http.StatusBadRequest, httpx.CodeValidationFailed, "source_id must be a valid UUID")
		return
	}
	file, header, err := r.FormFile("file")
	if err != nil {
		httpx.WriteError(w, r, http.StatusBadRequest, httpx.CodeValidationFailed, "multipart field \"file\" is required")
		return
	}
	defer file.Close()

	imp, err := s.svc.ImportStatement(r.Context(), tenantID, sourceID, header.Filename, file)
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusCreated, toImportJSON(imp))
}

func (s *Server) createRun(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := tenant(w, r)
	if !ok {
		return
	}
	var body struct {
		SourceID uuid.UUID `json:"source_id"`
	}
	if err := httpx.DecodeJSON(w, r, &body); err != nil {
		httpx.WriteError(w, r, http.StatusBadRequest, httpx.CodeValidationFailed, err.Error())
		return
	}
	run, err := s.svc.StartRun(r.Context(), tenantID, body.SourceID)
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusCreated, toRunJSON(run))
}

func (s *Server) getRun(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := tenant(w, r)
	if !ok {
		return
	}
	id, ok := pathUUID(w, r)
	if !ok {
		return
	}
	run, err := s.svc.GetRun(r.Context(), tenantID, id)
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, toRunJSON(run))
}

func (s *Server) getReports(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := tenant(w, r)
	if !ok {
		return
	}
	summaries, err := s.svc.Report(r.Context(), tenantID)
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	out := make([]summaryJSON, 0, len(summaries))
	totalMatched, totalUnmatched, totalDisputed, totalOpen := 0, 0, 0, 0
	for _, sum := range summaries {
		out = append(out, summaryJSON{
			SourceID: sum.SourceID, SourceName: sum.SourceName,
			Matched: sum.Matched, Unmatched: sum.Unmatched,
			Disputed: sum.Disputed, OpenDiscrepancies: sum.OpenDiscrepancies,
		})
		totalMatched += sum.Matched
		totalUnmatched += sum.Unmatched
		totalDisputed += sum.Disputed
		totalOpen += sum.OpenDiscrepancies
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"data": out,
		"totals": map[string]int{
			"matched":            totalMatched,
			"unmatched":          totalUnmatched,
			"disputed":           totalDisputed,
			"open_discrepancies": totalOpen,
		},
	})
}

func (s *Server) listDiscrepancies(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := tenant(w, r)
	if !ok {
		return
	}
	page, ok := httpx.ParsePage(w, r)
	if !ok {
		return
	}
	discrepancies, err := s.svc.ListDiscrepancies(r.Context(), tenantID, r.URL.Query().Get("status"), page.FetchLimit(), page.Cursor)
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	discrepancies, next := httpx.Window(discrepancies, page.Limit, func(d domain.Discrepancy) httpx.Cursor {
		return httpx.Cursor{CreatedAt: d.CreatedAt, ID: d.ID}
	})
	resp := httpx.ListResponse[discrepancyJSON]{Data: make([]discrepancyJSON, 0, len(discrepancies)), NextCursor: next}
	for _, d := range discrepancies {
		resp.Data = append(resp.Data, toDiscrepancyJSON(d))
	}
	httpx.WriteJSON(w, http.StatusOK, resp)
}

func (s *Server) patchDiscrepancy(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := tenant(w, r)
	if !ok {
		return
	}
	id, ok := pathUUID(w, r)
	if !ok {
		return
	}
	var body struct {
		Status         string `json:"status"`
		ResolutionNote string `json:"resolution_note"`
	}
	if err := httpx.DecodeJSON(w, r, &body); err != nil {
		httpx.WriteError(w, r, http.StatusBadRequest, httpx.CodeValidationFailed, err.Error())
		return
	}
	d, err := s.svc.UpdateDiscrepancy(r.Context(), tenantID, id, domain.DiscrepancyStatus(body.Status), body.ResolutionNote)
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, toDiscrepancyJSON(d))
}
