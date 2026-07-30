package httpadapter

import (
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/google/uuid"

	"github.com/ledgercore/ledgercore/libs/go/httpx"
	"github.com/ledgercore/ledgercore/services/identity/internal/app"
	"github.com/ledgercore/ledgercore/services/identity/internal/domain"
)

// ---- Response shapes -----------------------------------------------------

type tenantResponse struct {
	ID        uuid.UUID `json:"id"`
	Name      string    `json:"name"`
	Slug      string    `json:"slug"`
	Status    string    `json:"status"`
	CreatedAt time.Time `json:"created_at"`
}

func toTenantResponse(t domain.Tenant) tenantResponse {
	return tenantResponse{ID: t.ID, Name: t.Name, Slug: t.Slug, Status: t.Status, CreatedAt: t.CreatedAt}
}

type apiKeyResponse struct {
	ID          uuid.UUID `json:"id"`
	TenantID    uuid.UUID `json:"tenant_id"`
	Environment string    `json:"environment"`
	Name        string    `json:"name"`
	KeyPrefix   string    `json:"key_prefix"`
	Scopes      []string  `json:"scopes"`
	Secret      string    `json:"secret,omitempty"` // present only on creation
	CreatedAt   time.Time `json:"created_at"`
}

// ---- Error mapping -----------------------------------------------------

func writeServiceError(w http.ResponseWriter, r *http.Request, err error) {
	var verr app.ValidationError
	switch {
	case errors.As(err, &verr):
		httpx.WriteError(w, r, http.StatusBadRequest, httpx.CodeValidationFailed, verr.Error())
	case errors.Is(err, domain.ErrNotFound):
		httpx.WriteError(w, r, http.StatusNotFound, httpx.CodeNotFound, "resource not found")
	case errors.Is(err, domain.ErrSlugConflict):
		httpx.WriteError(w, r, http.StatusConflict, httpx.CodeConflict, "a tenant with that slug already exists")
	case errors.Is(err, domain.ErrEmailTaken):
		httpx.WriteError(w, r, http.StatusConflict, httpx.CodeConflict, "that email already created a sandbox tenant")
	case errors.Is(err, domain.ErrSignupLimitReached):
		httpx.WriteError(w, r, http.StatusTooManyRequests, httpx.CodeRateLimited, "the daily sandbox signup limit was reached; try again tomorrow")
	case errors.Is(err, app.ErrInvalidCredentials):
		httpx.WriteError(w, r, http.StatusUnauthorized, httpx.CodeUnauthorized, "api key is unknown, revoked, or its tenant is not active")
	default:
		slog.Error("identity request failed",
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

// ---- Tenants -----------------------------------------------------

func (s *Server) handleCreateTenant(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name string `json:"name"`
		Slug string `json:"slug"`
	}
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		httpx.WriteError(w, r, http.StatusBadRequest, httpx.CodeValidationFailed, err.Error())
		return
	}
	tenant, err := s.svc.CreateTenant(r.Context(), req.Name, req.Slug)
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusCreated, toTenantResponse(tenant))
}

func (s *Server) handleGetTenant(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r)
	if !ok {
		return
	}
	tenant, err := s.svc.GetTenant(r.Context(), id)
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, toTenantResponse(tenant))
}

func (s *Server) handleListTenants(w http.ResponseWriter, r *http.Request) {
	page, ok := httpx.ParsePage(w, r)
	if !ok {
		return
	}
	tenants, err := s.svc.ListTenants(r.Context(), page.FetchLimit(), page.Cursor)
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	tenants, next := httpx.Window(tenants, page.Limit, func(t domain.Tenant) httpx.Cursor {
		return httpx.Cursor{CreatedAt: t.CreatedAt, ID: t.ID}
	})
	resp := httpx.ListResponse[tenantResponse]{Data: make([]tenantResponse, 0, len(tenants)), NextCursor: next}
	for _, t := range tenants {
		resp.Data = append(resp.Data, toTenantResponse(t))
	}
	httpx.WriteJSON(w, http.StatusOK, resp)
}

// ---- API keys -----------------------------------------------------

func (s *Server) handleCreateAPIKey(w http.ResponseWriter, r *http.Request) {
	var req struct {
		TenantID    string `json:"tenant_id"`
		Environment string `json:"environment"`
		Name        string `json:"name"`
		// Scopes bounds what tokens minted from this key may do; omitted or
		// empty applies app.DefaultScopes. Values outside the allowed set are
		// rejected (400).
		Scopes []string `json:"scopes"`
	}
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		httpx.WriteError(w, r, http.StatusBadRequest, httpx.CodeValidationFailed, err.Error())
		return
	}
	tenantID, err := uuid.Parse(req.TenantID)
	if err != nil {
		httpx.WriteError(w, r, http.StatusBadRequest, httpx.CodeValidationFailed, "tenant_id must be a valid UUID")
		return
	}
	key, secret, err := s.svc.CreateAPIKey(r.Context(), tenantID, req.Environment, req.Name, req.Scopes)
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusCreated, apiKeyResponse{
		ID:          key.ID,
		TenantID:    key.TenantID,
		Environment: key.Environment,
		Name:        key.Name,
		KeyPrefix:   key.KeyPrefix,
		Scopes:      key.Scopes,
		Secret:      secret, // shown exactly once
		CreatedAt:   key.CreatedAt,
	})
}

func (s *Server) handleRevokeAPIKey(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r)
	if !ok {
		return
	}
	if err := s.svc.RevokeAPIKey(r.Context(), id); err != nil {
		writeServiceError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusNoContent, nil)
}

// ---- Sandbox signup -----------------------------------------------------

func (s *Server) handleSandboxSignup(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Email       string `json:"email"`
		CompanyName string `json:"company_name"`
	}
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		httpx.WriteError(w, r, http.StatusBadRequest, httpx.CodeValidationFailed, err.Error())
		return
	}
	signup, err := s.sandbox.Signup(r.Context(), req.Email, req.CompanyName)
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusCreated, map[string]any{
		"tenant_id":  signup.Tenant.ID,
		"slug":       signup.Tenant.Slug,
		"api_key":    signup.Secret, // shown exactly once
		"key_prefix": signup.Key.KeyPrefix,
		"expires_at": signup.Tenant.ExpiresAt,
	})
}

// ---- Auth -----------------------------------------------------

func (s *Server) handleIssueToken(w http.ResponseWriter, r *http.Request) {
	var req struct {
		APIKey string `json:"api_key"`
		// Aliases for the documented request shape (infra/README.md):
		// api_key_secret carries the same plaintext secret; api_key_id is
		// accepted but unnecessary (lookup happens by secret prefix).
		APIKeyID     string `json:"api_key_id"`
		APIKeySecret string `json:"api_key_secret"`
	}
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		httpx.WriteError(w, r, http.StatusBadRequest, httpx.CodeValidationFailed, err.Error())
		return
	}
	if req.APIKey == "" {
		req.APIKey = req.APIKeySecret
	}
	if req.APIKey == "" {
		httpx.WriteError(w, r, http.StatusBadRequest, httpx.CodeValidationFailed, "api_key is required")
		return
	}
	token, err := s.svc.IssueToken(r.Context(), req.APIKey)
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, token)
}

func (s *Server) handleJWKS(w http.ResponseWriter, r *http.Request) {
	doc, err := s.svc.JWKS(r.Context())
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	// Validators cache aggressively (libs/go/ident refreshes every 5 min);
	// a short CDN/browser cache is safe and cheap.
	w.Header().Set("Cache-Control", "public, max-age=300")
	httpx.WriteJSON(w, http.StatusOK, doc)
}
