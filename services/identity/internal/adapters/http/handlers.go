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
	Secret      string    `json:"secret,omitempty"` // present only on creation
	CreatedAt   time.Time `json:"created_at"`
}

// ---- Error mapping -----------------------------------------------------

func writeServiceError(w http.ResponseWriter, r *http.Request, err error) {
	var verr app.ValidationError
	switch {
	case errors.As(err, &verr):
		httpx.WriteError(w, http.StatusBadRequest, "validation_error", verr.Error())
	case errors.Is(err, domain.ErrNotFound):
		httpx.WriteError(w, http.StatusNotFound, "not_found", "resource not found")
	case errors.Is(err, domain.ErrSlugConflict):
		httpx.WriteError(w, http.StatusConflict, "slug_conflict", "a tenant with that slug already exists")
	case errors.Is(err, app.ErrInvalidCredentials):
		httpx.WriteError(w, http.StatusUnauthorized, "invalid_credentials", "api key is unknown, revoked, or its tenant is not active")
	default:
		slog.Error("identity request failed",
			"error", err,
			"path", r.URL.Path,
			"request_id", httpx.RequestIDFromContext(r.Context()),
		)
		httpx.WriteError(w, http.StatusInternalServerError, "internal_error", "an internal error occurred")
	}
}

func pathUUID(w http.ResponseWriter, r *http.Request) (uuid.UUID, bool) {
	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid_id", "path id must be a valid UUID")
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
		httpx.WriteError(w, http.StatusBadRequest, "invalid_request", err.Error())
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
	tenants, err := s.svc.ListTenants(r.Context())
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	data := make([]tenantResponse, 0, len(tenants))
	for _, t := range tenants {
		data = append(data, toTenantResponse(t))
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"data": data})
}

// ---- API keys -----------------------------------------------------

func (s *Server) handleCreateAPIKey(w http.ResponseWriter, r *http.Request) {
	var req struct {
		TenantID    string `json:"tenant_id"`
		Environment string `json:"environment"`
		Name        string `json:"name"`
		// Accepted for forward compatibility with the documented API
		// (infra/README.md); issued tokens carry app.DefaultScopes today.
		Scopes []string `json:"scopes"`
	}
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	tenantID, err := uuid.Parse(req.TenantID)
	if err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "validation_error", "tenant_id must be a valid UUID")
		return
	}
	key, secret, err := s.svc.CreateAPIKey(r.Context(), tenantID, req.Environment, req.Name)
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
		httpx.WriteError(w, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	if req.APIKey == "" {
		req.APIKey = req.APIKeySecret
	}
	if req.APIKey == "" {
		httpx.WriteError(w, http.StatusBadRequest, "validation_error", "api_key is required")
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
