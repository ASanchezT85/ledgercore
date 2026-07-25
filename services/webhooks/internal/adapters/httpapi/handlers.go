package httpapi

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
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
		httpx.WriteError(w, http.StatusBadRequest, "invalid_request", "id must be a valid UUID")
		return uuid.Nil, false
	}
	return id, true
}

// writeServiceError maps application/domain errors onto the API contract.
func writeServiceError(w http.ResponseWriter, r *http.Request, err error) {
	var ve domain.ValidationError
	switch {
	case errors.As(err, &ve):
		httpx.WriteError(w, http.StatusBadRequest, "invalid_request", ve.Error())
	case errors.Is(err, domain.ErrNotFound):
		httpx.WriteError(w, http.StatusNotFound, "not_found", "resource not found")
	case errors.Is(err, domain.ErrConflict):
		httpx.WriteError(w, http.StatusConflict, "conflict", "delivery is not in a retryable state (only failed or dead)")
	default:
		slog.Error("request failed",
			"method", r.Method, "path", r.URL.Path,
			"request_id", httpx.RequestIDFromContext(r.Context()), "error", err)
		httpx.WriteError(w, http.StatusInternalServerError, "internal_error", "an internal error occurred")
	}
}

// ---- Subscriptions ---------------------------------------------------------------

func (s *server) createSubscription(w http.ResponseWriter, r *http.Request) {
	tenantID, requireHTTPS, ok := requestIdentity(r)
	if !ok {
		httpx.WriteError(w, http.StatusUnauthorized, "missing_token", "authentication required")
		return
	}
	var req struct {
		URL        string   `json:"url"`
		EventTypes []string `json:"event_types"`
	}
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid_request", err.Error())
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
		httpx.WriteError(w, http.StatusUnauthorized, "missing_token", "authentication required")
		return
	}
	subs, err := s.svc.ListSubscriptions(r.Context(), tenantID)
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	out := make([]subscriptionDTO, 0, len(subs))
	for _, sub := range subs {
		out = append(out, toSubscriptionDTO(sub, false))
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"data": out})
}

func (s *server) getSubscription(w http.ResponseWriter, r *http.Request) {
	tenantID, _, ok := requestIdentity(r)
	if !ok {
		httpx.WriteError(w, http.StatusUnauthorized, "missing_token", "authentication required")
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
		httpx.WriteError(w, http.StatusUnauthorized, "missing_token", "authentication required")
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
		httpx.WriteError(w, http.StatusBadRequest, "invalid_request", err.Error())
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
		httpx.WriteError(w, http.StatusUnauthorized, "missing_token", "authentication required")
		return
	}
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	secret, err := s.svc.RotateSecret(r.Context(), tenantID, id)
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	// The new plaintext secret is returned exactly once, here.
	httpx.WriteJSON(w, http.StatusOK, map[string]string{
		"id":     id.String(),
		"secret": secret,
	})
}

// ---- Deliveries ---------------------------------------------------------------------

func (s *server) listDeliveries(w http.ResponseWriter, r *http.Request) {
	tenantID, _, ok := requestIdentity(r)
	if !ok {
		httpx.WriteError(w, http.StatusUnauthorized, "missing_token", "authentication required")
		return
	}
	q := r.URL.Query()

	filter := app.DeliveryFilter{Status: q.Get("status")}

	if raw := q.Get("subscription_id"); raw != "" {
		subID, err := uuid.Parse(raw)
		if err != nil {
			httpx.WriteError(w, http.StatusBadRequest, "invalid_request", "subscription_id must be a valid UUID")
			return
		}
		filter.SubscriptionID = &subID
	}
	cursor, err := httpx.DecodeCursor(q.Get("cursor"))
	if err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid_request", "cursor is not valid")
		return
	}
	filter.Cursor = cursor
	if raw := q.Get("limit"); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil || n <= 0 {
			httpx.WriteError(w, http.StatusBadRequest, "invalid_request", "limit must be a positive integer")
			return
		}
		filter.Limit = n
	}

	deliveries, next, err := s.svc.ListDeliveries(r.Context(), tenantID, filter)
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	out := make([]deliveryDTO, 0, len(deliveries))
	for _, d := range deliveries {
		out = append(out, toDeliveryDTO(d))
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"data":        out,
		"next_cursor": next,
	})
}

func (s *server) retryDelivery(w http.ResponseWriter, r *http.Request) {
	tenantID, _, ok := requestIdentity(r)
	if !ok {
		httpx.WriteError(w, http.StatusUnauthorized, "missing_token", "authentication required")
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
