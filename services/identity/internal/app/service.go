// Package app contains the identity service use cases: tenant registry
// management, API key lifecycle and JWT issuance. It depends only on the
// domain package and on repository ports implemented by adapters.
package app

import (
	"context"
	"crypto/subtle"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/ledgercore/ledgercore/services/identity/internal/domain"
)

// DefaultScopes are the scopes granted to every token issued from an API key.
var DefaultScopes = []string{"ledger:read", "ledger:write"}

// TokenTTL is the lifetime of issued access tokens.
const TokenTTL = 15 * time.Minute

// ErrInvalidCredentials is returned for any authentication failure during
// token issuance. It is deliberately opaque: callers must not learn whether
// the key exists, is revoked, or belongs to a suspended tenant.
var ErrInvalidCredentials = errors.New("invalid credentials")

// ValidationError marks bad input; the HTTP adapter maps it to 400.
type ValidationError struct{ Msg string }

func (e ValidationError) Error() string { return e.Msg }

// ---- Ports ------------------------------------------------------------------

// TenantRepository persists the system-level tenant registry.
type TenantRepository interface {
	CreateTenant(ctx context.Context, t domain.Tenant) error
	GetTenant(ctx context.Context, id uuid.UUID) (domain.Tenant, error)
	ListTenants(ctx context.Context) ([]domain.Tenant, error)
}

// APIKeyRepository persists API keys (hash + prefix only, never plaintext).
type APIKeyRepository interface {
	CreateAPIKey(ctx context.Context, k domain.APIKey) error
	FindAPIKeysByPrefix(ctx context.Context, prefix string) ([]domain.APIKey, error)
	RevokeAPIKey(ctx context.Context, id uuid.UUID) error
}

// SigningKeyRepository persists the JWT signing keys.
type SigningKeyRepository interface {
	InsertSigningKey(ctx context.Context, k domain.SigningKey) error
	ListActiveSigningKeys(ctx context.Context) ([]domain.SigningKey, error)
}

// ---- Service ------------------------------------------------------------------

// Service implements every identity use case.
type Service struct {
	tenants TenantRepository
	keys    APIKeyRepository
	signing SigningKeyRepository
	issuer  *TokenIssuer
	now     func() time.Time
}

// NewService wires the use cases with their ports.
func NewService(tenants TenantRepository, keys APIKeyRepository, signing SigningKeyRepository, issuer *TokenIssuer) *Service {
	return &Service{
		tenants: tenants,
		keys:    keys,
		signing: signing,
		issuer:  issuer,
		now:     func() time.Time { return time.Now().UTC() },
	}
}

// EnsureSigningKey returns the newest active signing key, generating and
// persisting one if none exists yet. Called once at startup.
//
// TODO(kms): in production the key must be created inside a KMS/HSM and the
// service should sign via the KMS API; this database bootstrap is dev-only.
func EnsureSigningKey(ctx context.Context, repo SigningKeyRepository) (domain.SigningKey, error) {
	existing, err := repo.ListActiveSigningKeys(ctx)
	if err != nil {
		return domain.SigningKey{}, fmt.Errorf("app: list signing keys: %w", err)
	}
	if len(existing) > 0 {
		slog.Info("using existing signing key", "kid", existing[0].Kid)
		return existing[0], nil
	}
	key, err := domain.GenerateSigningKey()
	if err != nil {
		return domain.SigningKey{}, err
	}
	if err := repo.InsertSigningKey(ctx, key); err != nil {
		return domain.SigningKey{}, fmt.Errorf("app: persist signing key: %w", err)
	}
	slog.Info("generated new signing key", "kid", key.Kid, "algorithm", key.Algorithm)
	return key, nil
}

// ---- Tenants ------------------------------------------------------------------

// CreateTenant registers a new tenant in the system-level registry.
func (s *Service) CreateTenant(ctx context.Context, name, slug string) (domain.Tenant, error) {
	name = strings.TrimSpace(name)
	if name == "" || len(name) > 255 {
		return domain.Tenant{}, ValidationError{Msg: "name is required and must be at most 255 characters"}
	}
	if slug == "" {
		// Slug is optional in the API (see infra/README.md): derive it
		// from the name when omitted.
		slug = slugify(name)
	}
	if !domain.ValidSlug(slug) {
		return domain.Tenant{}, ValidationError{Msg: "slug must be lowercase alphanumerics separated by single hyphens (max 100 chars)"}
	}
	id, err := uuid.NewV7()
	if err != nil {
		return domain.Tenant{}, fmt.Errorf("app: generate tenant id: %w", err)
	}
	tenant := domain.Tenant{
		ID:        id,
		Name:      name,
		Slug:      slug,
		Status:    domain.TenantStatusActive,
		CreatedAt: s.now(),
	}
	if err := s.tenants.CreateTenant(ctx, tenant); err != nil {
		return domain.Tenant{}, err
	}
	return tenant, nil
}

// GetTenant fetches a tenant by id.
func (s *Service) GetTenant(ctx context.Context, id uuid.UUID) (domain.Tenant, error) {
	return s.tenants.GetTenant(ctx, id)
}

// ListTenants returns every tenant.
func (s *Service) ListTenants(ctx context.Context) ([]domain.Tenant, error) {
	return s.tenants.ListTenants(ctx)
}

// ---- API keys ------------------------------------------------------------------

// CreateAPIKey mints a new API key for a tenant. The returned string is the
// plaintext secret, shown exactly once; only its hash and prefix are stored.
func (s *Service) CreateAPIKey(ctx context.Context, tenantID uuid.UUID, environment, name string) (domain.APIKey, string, error) {
	name = strings.TrimSpace(name)
	if name == "" || len(name) > 255 {
		return domain.APIKey{}, "", ValidationError{Msg: "name is required and must be at most 255 characters"}
	}
	if !domain.ValidEnvironment(environment) {
		return domain.APIKey{}, "", ValidationError{Msg: "environment must be sandbox or live"}
	}
	if _, err := s.tenants.GetTenant(ctx, tenantID); err != nil {
		return domain.APIKey{}, "", err // domain.ErrNotFound -> 404 at the edge
	}
	secret, err := domain.NewAPIKeySecret(environment)
	if err != nil {
		return domain.APIKey{}, "", err
	}
	id, err := uuid.NewV7()
	if err != nil {
		return domain.APIKey{}, "", fmt.Errorf("app: generate api key id: %w", err)
	}
	key := domain.APIKey{
		ID:          id,
		TenantID:    tenantID,
		Environment: environment,
		Name:        name,
		KeyPrefix:   domain.PrefixOf(secret),
		SecretHash:  domain.HashSecret(secret),
		CreatedAt:   s.now(),
	}
	if err := s.keys.CreateAPIKey(ctx, key); err != nil {
		return domain.APIKey{}, "", err
	}
	return key, secret, nil
}

// RevokeAPIKey revokes a key. Revoking an already-revoked key is a no-op.
func (s *Service) RevokeAPIKey(ctx context.Context, id uuid.UUID) error {
	return s.keys.RevokeAPIKey(ctx, id)
}

// ---- Token issuance ------------------------------------------------------------------

// dummyHash keeps the hash-compare cost uniform when the prefix matches no
// stored key, so "unknown prefix" is not distinguishable by timing.
var dummyHash = domain.HashSecret("lk_sandbox_timing_equalizer_dummy_secret")

// IssueToken exchanges a plaintext API key for a short-lived EdDSA JWT.
// Every failure mode returns ErrInvalidCredentials.
func (s *Service) IssueToken(ctx context.Context, secret string) (IssuedToken, error) {
	if !strings.HasPrefix(secret, "lk_") || len(secret) <= domain.KeyPrefixLength {
		return IssuedToken{}, ErrInvalidCredentials
	}
	candidates, err := s.keys.FindAPIKeysByPrefix(ctx, domain.PrefixOf(secret))
	if err != nil {
		return IssuedToken{}, fmt.Errorf("app: find api key: %w", err)
	}
	var match *domain.APIKey
	for i := range candidates {
		if candidates[i].VerifySecret(secret) {
			match = &candidates[i]
			break
		}
	}
	if match == nil {
		// Equalize timing with the found-key path (constant-time compare).
		subtle.ConstantTimeCompare(domain.HashSecret(secret), dummyHash)
		return IssuedToken{}, ErrInvalidCredentials
	}
	if match.Revoked() {
		slog.Info("token denied: api key revoked", "api_key_id", match.ID)
		return IssuedToken{}, ErrInvalidCredentials
	}
	tenant, err := s.tenants.GetTenant(ctx, match.TenantID)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return IssuedToken{}, ErrInvalidCredentials
		}
		return IssuedToken{}, fmt.Errorf("app: load tenant: %w", err)
	}
	if !tenant.Active() {
		slog.Info("token denied: tenant not active", "tenant_id", tenant.ID, "status", tenant.Status)
		return IssuedToken{}, ErrInvalidCredentials
	}
	return s.issuer.Issue(match.ID, tenant.ID, match.Environment, DefaultScopes)
}

// slugify derives a URL-safe slug from a display name: lowercase
// alphanumerics separated by single hyphens, max 100 chars.
func slugify(name string) string {
	var b strings.Builder
	prevHyphen := true // avoid a leading hyphen
	for _, r := range strings.ToLower(name) {
		switch {
		case (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9'):
			b.WriteRune(r)
			prevHyphen = false
		default:
			if !prevHyphen {
				b.WriteByte('-')
				prevHyphen = true
			}
		}
	}
	s := strings.Trim(b.String(), "-")
	if len(s) > 100 {
		s = strings.Trim(s[:100], "-")
	}
	return s
}
