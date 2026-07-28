package httpapi

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/google/uuid"

	"github.com/ledgercore/ledgercore/libs/go/httpx"
	"github.com/ledgercore/ledgercore/libs/go/ident"
	"github.com/ledgercore/ledgercore/services/webhooks/internal/app"
	"github.com/ledgercore/ledgercore/services/webhooks/internal/domain"
)

// ---- DTOs -----------------------------------------------------------------

// subscriptionDTO is the API shape of a subscription. Secret is present only
// in create/rotate responses; it is never readable afterwards.
type subscriptionDTO struct {
	ID         uuid.UUID `json:"id"`
	URL        string    `json:"url"`
	EventTypes []string  `json:"event_types"`
	Active     bool      `json:"active"`
	CreatedAt  time.Time `json:"created_at"`
	Secret     string    `json:"secret,omitempty"`
}

func toSubscriptionDTO(s domain.Subscription, includeSecret bool) subscriptionDTO {
	dto := subscriptionDTO{
		ID:         s.ID,
		URL:        s.URL,
		EventTypes: s.EventTypes,
		Active:     s.Active,
		CreatedAt:  s.CreatedAt,
	}
	if includeSecret {
		dto.Secret = s.Secret
	}
	return dto
}

type deliveryDTO struct {
	ID             uuid.UUID       `json:"id"`
	SubscriptionID uuid.UUID       `json:"subscription_id"`
	EventID        uuid.UUID       `json:"event_id"`
	EventType      string          `json:"event_type"`
	Payload        json.RawMessage `json:"payload"`
	Status         string          `json:"status"`
	Attempts       int             `json:"attempts"`
	NextAttemptAt  time.Time       `json:"next_attempt_at"`
	LastStatusCode *int            `json:"last_status_code"`
	LastError      *string         `json:"last_error"`
	CreatedAt      time.Time       `json:"created_at"`
	DeliveredAt    *time.Time      `json:"delivered_at"`
}

func toDeliveryDTO(d domain.Delivery) deliveryDTO {
	return deliveryDTO{
		ID:             d.ID,
		SubscriptionID: d.SubscriptionID,
		EventID:        d.EventID,
		EventType:      d.EventType,
		Payload:        d.Payload,
		Status:         d.Status,
		Attempts:       d.Attempts,
		NextAttemptAt:  d.NextAttemptAt,
		LastStatusCode: d.LastStatusCode,
		LastError:      d.LastError,
		CreatedAt:      d.CreatedAt,
		DeliveredAt:    d.DeliveredAt,
	}
}

// ---- Helpers ---------------------------------------------------------------

// requestIdentity extracts the tenant and whether https must be enforced
// (live environment). RequireAuth guarantees the claims are present.
func requestIdentity(r *http.Request) (uuid.UUID, bool, bool) {
	claims, ok := ident.ClaimsFromContext(r.Context())
	if !ok {
		return uuid.Nil, false, false
	}
	return claims.TenantID, claims.Environment == ident.EnvLive, true
}

func pathID(w http.ResponseWriter, r *http.Request) (uuid.UUID, bool) {
	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		httpx.WriteError(w, r, http.StatusBadRequest, httpx.CodeValidationFailed, "id must be a valid UUID")
		return uuid.Nil, false
	}
	return id, true
}

// writeServiceError maps application/domain errors onto the API contract.
func writeServiceError(w http.ResponseWriter, r *http.Request, err error) {
	var ve domain.ValidationError
	switch {
	case errors.As(err, &ve):
		httpx.WriteError(w, r, http.StatusBadRequest, httpx.CodeValidationFailed, ve.Error())
	case errors.Is(err, domain.ErrNotFound):
		httpx.WriteError(w, r, http.StatusNotFound, httpx.CodeNotFound, "resource not found")
	case errors.Is(err, domain.ErrConflict):
		httpx.WriteError(w, r, http.StatusConflict, httpx.CodeConflict, "delivery is not in a retryable state (only failed or dead)")
	default:
		slog.Error("request failed",
			"method", r.Method, "path", r.URL.Path,
			"request_id", httpx.RequestIDFromContext(r.Context()), "error", err)
		httpx.WriteError(w, r, http.StatusInternalServerError, httpx.CodeInternal, "an internal error occurred")
	}
}

// ---- Subscriptions ---------------------------------------------------------------

func (s *server) createSubscription(w http.ResponseWriter, r *http.Request) {
	tenantID, requireHTTPS, ok := requestIdentity(r)
	if !ok {
		httpx.WriteError(w, r, http.StatusUnauthorized, httpx.CodeUnauthorized, "authentication required")
		return
	}
	var req struct {
		URL        string   `json:"url"`
		EventTypes []string `json:"event_types"`
	}
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		httpx.WriteError(w, r, http.StatusBadRequest, httpx.CodeValidationFailed, err.Error())
		return
	}
	sub, err := s.svc.CreateSubscription(r.Context(), tenantID, requireHTTPS, req.URL, req.EventTypes)
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	// The plaintext secret is returned exactly once, here.
	httpx.WriteJSON(w, http.StatusCreated, toSubscriptionDTO(sub, true))
}

func (s *server) listSubscriptions(w http.ResponseWriter, r *http.Request) {
	tenantID, _, ok := requestIdentity(r)
	if !ok {
		httpx.WriteError(w, r, http.StatusUnauthorized, httpx.CodeUnauthorized, "authentication required")
		return
	}
	page, ok := httpx.ParsePage(w, r)
	if !ok {
		return
	}
	subs, err := s.svc.ListSubscriptions(r.Context(), tenantID, page.FetchLimit(), page.Cursor)
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	subs, next := httpx.Window(subs, page.Limit, func(sub domain.Subscription) httpx.Cursor {
		return httpx.Cursor{CreatedAt: sub.CreatedAt, ID: sub.ID}
	})
	resp := httpx.ListResponse[subscriptionDTO]{Data: make([]subscriptionDTO, 0, len(subs)), NextCursor: next}
	for _, sub := range subs {
		resp.Data = append(resp.Data, toSubscriptionDTO(sub, false))
	}
	httpx.WriteJSON(w, http.StatusOK, resp)
}

func (s *server) getSubscription(w http.ResponseWriter, r *http.Request) {
	tenantID, _, ok := requestIdentity(r)
	if !ok {
		httpx.WriteError(w, r, http.StatusUnauthorized, httpx.CodeUnauthorized, "authentication required")
		return
	}
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	sub, err := s.svc.GetSubscription(r.Context(), tenantID, id)
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, toSubscriptionDTO(sub, false))
}

func (s *server) updateSubscription(w http.ResponseWriter, r *http.Request) {
	tenantID, requireHTTPS, ok := requestIdentity(r)
	if !ok {
		httpx.WriteError(w, r, http.StatusUnauthorized, httpx.CodeUnauthorized, "authentication required")
		return
	}
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	var req struct {
		URL        *string  `json:"url"`
		EventTypes []string `json:"event_types"`
		Active     *bool    `json:"active"`
	}
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		httpx.WriteError(w, r, http.StatusBadRequest, httpx.CodeValidationFailed, err.Error())
		return
	}
	sub, err := s.svc.UpdateSubscription(r.Context(), tenantID, requireHTTPS, id, app.SubscriptionPatch{
		URL:        req.URL,
		EventTypes: req.EventTypes,
		Active:     req.Active,
	})
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, toSubscriptionDTO(sub, false))
}

func (s *server) rotateSecret(w http.ResponseWriter, r *http.Request) {
	tenantID, _, ok := requestIdentity(r)
	if !ok {
		httpx.WriteError(w, r, http.StatusUnauthorized, httpx.CodeUnauthorized, "authentication required")
		return
	}
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	secret, prevExpires, err := s.svc.RotateSecret(r.Context(), tenantID, id)
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	// The new plaintext secret is returned exactly once, here. Deliveries are
	// signed with BOTH secrets until previous_secret_expires_at, so the
	// receiver can switch inside the grace window without losing events.
	httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"id":                         id.String(),
		"secret":                     secret,
		"previous_secret_expires_at": prevExpires,
	})
}

// ---- Deliveries ---------------------------------------------------------------------

func (s *server) listDeliveries(w http.ResponseWriter, r *http.Request) {
	tenantID, _, ok := requestIdentity(r)
	if !ok {
		httpx.WriteError(w, r, http.StatusUnauthorized, httpx.CodeUnauthorized, "authentication required")
		return
	}
	q := r.URL.Query()

	filter := app.DeliveryFilter{Status: q.Get("status")}

	if raw := q.Get("subscription_id"); raw != "" {
		subID, err := uuid.Parse(raw)
		if err != nil {
			httpx.WriteError(w, r, http.StatusBadRequest, httpx.CodeValidationFailed, "subscription_id must be a valid UUID")
			return
		}
		filter.SubscriptionID = &subID
	}
	page, ok := httpx.ParsePage(w, r)
	if !ok {
		return
	}
	filter.Cursor = page.Cursor
	filter.Limit = page.Limit

	deliveries, next, err := s.svc.ListDeliveries(r.Context(), tenantID, filter)
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	resp := httpx.ListResponse[deliveryDTO]{Data: make([]deliveryDTO, 0, len(deliveries))}
	if next != "" {
		resp.NextCursor = &next
	}
	for _, d := range deliveries {
		resp.Data = append(resp.Data, toDeliveryDTO(d))
	}
	httpx.WriteJSON(w, http.StatusOK, resp)
}

func (s *server) retryDelivery(w http.ResponseWriter, r *http.Request) {
	tenantID, _, ok := requestIdentity(r)
	if !ok {
		httpx.WriteError(w, r, http.StatusUnauthorized, httpx.CodeUnauthorized, "authentication required")
		return
	}
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	d, err := s.svc.RetryDelivery(r.Context(), tenantID, id)
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, toDeliveryDTO(d))
}
