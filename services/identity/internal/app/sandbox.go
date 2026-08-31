package app

import (
	"context"
	"fmt"
	"log/slog"
	"net/mail"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/ASanchezT85/ledgercore/libs/go/events"
	"github.com/ASanchezT85/ledgercore/services/identity/internal/domain"
)

// Sandbox lifecycle constants (blueprint §14, Fase 1a).
const (
	// SandboxTTL is how long a self-service sandbox tenant lives.
	SandboxTTL = 14 * 24 * time.Hour
	// PurgeGrace is the window between announcing expiry
	// (identity.tenant.expired, status=purging) and deleting credentials.
	PurgeGrace = 24 * time.Hour
	// DefaultSignupsPerDay is the global daily cap when
	// LEDGERCORE_SANDBOX_SIGNUPS_PER_DAY is unset.
	DefaultSignupsPerDay = 20
)

// SandboxRepository is the persistence port for signup and sweeping;
// *postgres.Store implements it.
type SandboxRepository interface {
	ProvisionSandboxTenant(ctx context.Context, email string, t domain.Tenant, k domain.APIKey, dailyLimit int) error
	ExpiredSandboxTenants(ctx context.Context, now time.Time) ([]domain.Tenant, error)
	MarkTenantPurging(ctx context.Context, env events.Envelope) error
	PurgeableTenants(ctx context.Context, cutoff time.Time) ([]uuid.UUID, error)
	FinalizeTenantPurge(ctx context.Context, tenantID uuid.UUID) (int64, error)
}

// SandboxService implements the self-service sandbox use cases.
type SandboxService struct {
	repo       SandboxRepository
	dailyLimit int
	now        func() time.Time
}

// NewSandboxService wires the sandbox use cases. dailyLimit <= 0 falls back
// to DefaultSignupsPerDay.
func NewSandboxService(repo SandboxRepository, dailyLimit int) *SandboxService {
	if dailyLimit <= 0 {
		dailyLimit = DefaultSignupsPerDay
	}
	return &SandboxService{
		repo:       repo,
		dailyLimit: dailyLimit,
		now:        func() time.Time { return time.Now().UTC() },
	}
}

// SandboxSignup is the result of a successful signup. Secret is the API key
// plaintext, shown exactly once.
type SandboxSignup struct {
	Tenant domain.Tenant
	Key    domain.APIKey
	Secret string
}

// Signup provisions a sandbox tenant + API key atomically: if anything
// fails, nothing is persisted. Limits enforced in the same transaction:
// one signup per email (forever) and a global daily cap.
func (s *SandboxService) Signup(ctx context.Context, email, companyName string) (SandboxSignup, error) {
	email = strings.ToLower(strings.TrimSpace(email))
	companyName = strings.TrimSpace(companyName)
	if email == "" || len(email) > 320 {
		return SandboxSignup{}, ValidationError{Msg: "email is required (max 320 characters)"}
	}
	if addr, err := mail.ParseAddress(email); err != nil || addr.Address != email {
		return SandboxSignup{}, ValidationError{Msg: "email must be a plain valid address"}
	}
	if companyName == "" || len(companyName) > 255 {
		return SandboxSignup{}, ValidationError{Msg: "company_name is required (max 255 characters)"}
	}

	tenantID, err := uuid.NewV7()
	if err != nil {
		return SandboxSignup{}, fmt.Errorf("app: generate tenant id: %w", err)
	}
	// Derive the slug from the company name plus a short unique suffix so
	// two companies with the same name never collide.
	base := slugify(companyName)
	if base == "" {
		base = "sandbox"
	}
	suffix := strings.ToLower(strings.ReplaceAll(tenantID.String(), "-", ""))[:6]
	slug := base
	if len(slug) > domain.MaxSlugLength-7 {
		slug = strings.Trim(slug[:domain.MaxSlugLength-7], "-")
	}
	slug = slug + "-" + suffix

	now := s.now()
	expires := now.Add(SandboxTTL)
	tenant := domain.Tenant{
		ID:        tenantID,
		Name:      companyName,
		Slug:      slug,
		Status:    domain.TenantStatusActive,
		CreatedAt: now,
		ExpiresAt: &expires,
	}

	secret, err := domain.NewAPIKeySecret(domain.EnvironmentSandbox)
	if err != nil {
		return SandboxSignup{}, err
	}
	keyID, err := uuid.NewV7()
	if err != nil {
		return SandboxSignup{}, fmt.Errorf("app: generate api key id: %w", err)
	}
	sandboxScopes := make([]string, len(DefaultScopes))
	copy(sandboxScopes, DefaultScopes)
	key := domain.APIKey{
		ID:          keyID,
		TenantID:    tenantID,
		Environment: domain.EnvironmentSandbox,
		Name:        "sandbox signup key",
		KeyPrefix:   domain.PrefixOf(secret),
		SecretHash:  domain.HashSecret(secret),
		Scopes:      sandboxScopes,
		CreatedAt:   now,
	}

	if err := s.repo.ProvisionSandboxTenant(ctx, email, tenant, key, s.dailyLimit); err != nil {
		return SandboxSignup{}, err
	}
	slog.Info("sandbox tenant provisioned",
		"tenant_id", tenant.ID, "slug", tenant.Slug, "expires_at", expires)
	return SandboxSignup{Tenant: tenant, Key: key, Secret: secret}, nil
}

// ---- Sweeper ------------------------------------------------------------------

// TenantExpiredPayload is the data carried by identity.tenant.expired.
type TenantExpiredPayload struct {
	TenantID  uuid.UUID `json:"tenant_id"`
	Slug      string    `json:"slug"`
	ExpiresAt time.Time `json:"expires_at"`
}

// Sweep runs one TTL pass: announce newly expired sandbox tenants (outbox +
// status=purging) and finalize the ones whose grace period elapsed
// (status=purged + api keys deleted). Idempotent by the status guards.
func (s *SandboxService) Sweep(ctx context.Context) error {
	now := s.now()

	expired, err := s.repo.ExpiredSandboxTenants(ctx, now)
	if err != nil {
		return err
	}
	for _, t := range expired {
		env, err := events.NewEnvelope(events.TopicTenantExpired, t.ID, TenantExpiredPayload{
			TenantID:  t.ID,
			Slug:      t.Slug,
			ExpiresAt: *t.ExpiresAt,
		})
		if err != nil {
			return err
		}
		if err := s.repo.MarkTenantPurging(ctx, env); err != nil {
			return err
		}
		slog.Info("sandbox tenant expired: purge announced",
			"tenant_id", t.ID, "slug", t.Slug, "expired_at", t.ExpiresAt)
	}

	purgeable, err := s.repo.PurgeableTenants(ctx, now.Add(-PurgeGrace))
	if err != nil {
		return err
	}
	for _, id := range purgeable {
		deleted, err := s.repo.FinalizeTenantPurge(ctx, id)
		if err != nil {
			return err
		}
		slog.Info("sandbox tenant purged: credentials deleted",
			"tenant_id", id, "api_keys_deleted", deleted)
	}
	return nil
}

// RunSweeper sweeps immediately and then on every tick until ctx is done.
func (s *SandboxService) RunSweeper(ctx context.Context, interval time.Duration) {
	if interval <= 0 {
		interval = time.Hour
	}
	slog.Info("sandbox TTL sweeper started", "interval", interval.String())
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		if err := s.Sweep(ctx); err != nil && ctx.Err() == nil {
			slog.Error("sandbox sweep failed", "error", err)
		}
		select {
		case <-ctx.Done():
			slog.Info("sandbox TTL sweeper stopped")
			return
		case <-ticker.C:
		}
	}
}
