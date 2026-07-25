// Package app contains the application layer of the webhooks service: the
// storage ports (implemented by adapters) and the use cases orchestrating the
// domain model. It is transport-agnostic — HTTP and NATS adapters call in.
package app

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/ledgercore/ledgercore/libs/go/events"
	"github.com/ledgercore/ledgercore/libs/go/httpx"
	"github.com/ledgercore/ledgercore/services/webhooks/internal/domain"
)

// ---- Ports ------------------------------------------------------------------

// SubscriptionStore persists webhook subscriptions, always tenant-scoped.
type SubscriptionStore interface {
	Create(ctx context.Context, sub domain.Subscription) error
	List(ctx context.Context, tenantID uuid.UUID) ([]domain.Subscription, error)
	Get(ctx context.Context, tenantID, id uuid.UUID) (domain.Subscription, error)
	// Update applies a partial patch; nil fields are left untouched.
	Update(ctx context.Context, tenantID, id uuid.UUID, url *string, eventTypes []string, active *bool) (domain.Subscription, error)
	UpdateSecret(ctx context.Context, tenantID, id uuid.UUID, secret string) error
	ListActive(ctx context.Context, tenantID uuid.UUID) ([]domain.Subscription, error)
}

// DeliveryStore persists webhook deliveries, always tenant-scoped.
type DeliveryStore interface {
	// InsertPending inserts deliveries idempotently:
	// ON CONFLICT (subscription_id, event_id) DO NOTHING.
	InsertPending(ctx context.Context, tenantID uuid.UUID, ds []domain.Delivery) error
	ListDeliveries(ctx context.Context, tenantID uuid.UUID, f DeliveryFilter) ([]domain.Delivery, string, error)
	// Requeue resets a failed/dead delivery to pending with a fresh attempt
	// budget. Returns domain.ErrConflict if the delivery is not retryable.
	Requeue(ctx context.Context, tenantID, id uuid.UUID) (domain.Delivery, error)
}

// DeliveryFilter narrows a delivery listing. Zero values mean "no filter".
type DeliveryFilter struct {
	Status         string
	SubscriptionID *uuid.UUID
	Cursor         httpx.Cursor
	Limit          int
}

// ---- Service ------------------------------------------------------------------

// Service wires the use cases to the storage ports.
type Service struct {
	subs SubscriptionStore
	dels DeliveryStore
}

// NewService builds the application service.
func NewService(subs SubscriptionStore, dels DeliveryStore) *Service {
	return &Service{subs: subs, dels: dels}
}

// SubscriptionPatch is the partial update accepted by UpdateSubscription.
// Nil fields are not modified.
type SubscriptionPatch struct {
	URL        *string
	EventTypes []string
	Active     *bool
}

// CreateSubscription registers a webhook endpoint and returns the plaintext
// signing secret exactly once. requireHTTPS is true for live-environment
// callers.
func (s *Service) CreateSubscription(ctx context.Context, tenantID uuid.UUID, requireHTTPS bool, rawURL string, eventTypes []string) (domain.Subscription, error) {
	if err := domain.ValidateEndpointURL(rawURL, requireHTTPS); err != nil {
		return domain.Subscription{}, err
	}
	eventTypes = domain.NormalizeEventTypes(eventTypes)
	if err := domain.ValidateEventTypes(eventTypes); err != nil {
		return domain.Subscription{}, err
	}
	secret, err := domain.NewSecret()
	if err != nil {
		return domain.Subscription{}, err
	}
	sub := domain.Subscription{
		ID:         uuid.New(),
		TenantID:   tenantID,
		URL:        rawURL,
		Secret:     secret,
		EventTypes: eventTypes,
		Active:     true,
		CreatedAt:  time.Now().UTC(),
	}
	if err := s.subs.Create(ctx, sub); err != nil {
		return domain.Subscription{}, fmt.Errorf("app: create subscription: %w", err)
	}
	return sub, nil
}

// ListSubscriptions returns every subscription of the tenant.
func (s *Service) ListSubscriptions(ctx context.Context, tenantID uuid.UUID) ([]domain.Subscription, error) {
	return s.subs.List(ctx, tenantID)
}

// GetSubscription returns one subscription of the tenant.
func (s *Service) GetSubscription(ctx context.Context, tenantID, id uuid.UUID) (domain.Subscription, error) {
	return s.subs.Get(ctx, tenantID, id)
}

// UpdateSubscription applies a partial patch after validating it.
func (s *Service) UpdateSubscription(ctx context.Context, tenantID uuid.UUID, requireHTTPS bool, id uuid.UUID, patch SubscriptionPatch) (domain.Subscription, error) {
	if patch.URL != nil {
		if err := domain.ValidateEndpointURL(*patch.URL, requireHTTPS); err != nil {
			return domain.Subscription{}, err
		}
	}
	if patch.EventTypes != nil {
		patch.EventTypes = domain.NormalizeEventTypes(patch.EventTypes)
		if err := domain.ValidateEventTypes(patch.EventTypes); err != nil {
			return domain.Subscription{}, err
		}
	}
	return s.subs.Update(ctx, tenantID, id, patch.URL, patch.EventTypes, patch.Active)
}

// RotateSecret replaces the signing secret and returns the new plaintext
// value exactly once.
func (s *Service) RotateSecret(ctx context.Context, tenantID, id uuid.UUID) (string, error) {
	secret, err := domain.NewSecret()
	if err != nil {
		return "", err
	}
	if err := s.subs.UpdateSecret(ctx, tenantID, id, secret); err != nil {
		return "", err
	}
	return secret, nil
}

// ListDeliveries returns a page of deliveries plus an opaque next cursor
// (empty when there are no more pages).
func (s *Service) ListDeliveries(ctx context.Context, tenantID uuid.UUID, f DeliveryFilter) ([]domain.Delivery, string, error) {
	if f.Status != "" && !domain.ValidStatus(f.Status) {
		return nil, "", domain.Invalidf("status must be one of pending, delivered, failed, dead")
	}
	if f.Limit <= 0 {
		f.Limit = 50
	}
	if f.Limit > 200 {
		f.Limit = 200
	}
	return s.dels.ListDeliveries(ctx, tenantID, f)
}

// RetryDelivery re-enqueues a failed or dead delivery.
func (s *Service) RetryDelivery(ctx context.Context, tenantID, id uuid.UUID) (domain.Delivery, error) {
	return s.dels.Requeue(ctx, tenantID, id)
}

// EnqueueEnvelope fans one event envelope out to every active subscription
// of the tenant whose event_types match. rawEnvelope, when non-nil, is stored
// verbatim as the delivery payload; otherwise the envelope is re-marshaled.
// Insertion is idempotent per (subscription, event), so NATS redeliveries are
// harmless.
func (s *Service) EnqueueEnvelope(ctx context.Context, env events.Envelope, rawEnvelope []byte) error {
	if env.ID == uuid.Nil || env.TenantID == uuid.Nil || env.Type == "" {
		return domain.Invalidf("envelope is missing id, tenant_id or type")
	}
	subs, err := s.subs.ListActive(ctx, env.TenantID)
	if err != nil {
		return fmt.Errorf("app: list active subscriptions: %w", err)
	}
	payload := rawEnvelope
	if payload == nil {
		payload, err = json.Marshal(env)
		if err != nil {
			return fmt.Errorf("app: marshal envelope %s: %w", env.ID, err)
		}
	}
	now := time.Now().UTC()
	var ds []domain.Delivery
	for _, sub := range subs {
		if !domain.Matches(sub.EventTypes, env.Type) {
			continue
		}
		ds = append(ds, domain.Delivery{
			ID:             uuid.New(),
			TenantID:       env.TenantID,
			SubscriptionID: sub.ID,
			EventID:        env.ID,
			EventType:      env.Type,
			Payload:        payload,
			Status:         domain.StatusPending,
			NextAttemptAt:  now,
			CreatedAt:      now,
		})
	}
	if len(ds) == 0 {
		return nil
	}
	if err := s.dels.InsertPending(ctx, env.TenantID, ds); err != nil {
		return fmt.Errorf("app: enqueue deliveries for event %s: %w", env.ID, err)
	}
	return nil
}
