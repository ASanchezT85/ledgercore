// Package domain holds the identity service's pure business types and rules:
// tenants, API keys and JWT signing keys. It has no knowledge of HTTP,
// PostgreSQL or any other infrastructure concern.
package domain

import (
	"errors"
	"regexp"
	"time"

	"github.com/google/uuid"
)

// Tenant statuses.
const (
	TenantStatusActive    = "active"
	TenantStatusSuspended = "suspended"
	// TenantStatusPurging: sandbox TTL elapsed; expiry announced to the
	// platform, grace period running before credentials are deleted.
	TenantStatusPurging = "purging"
	// TenantStatusPurged: grace elapsed; API keys deleted, downstream
	// schemas purged. Terminal state kept for audit.
	TenantStatusPurged = "purged"
)

// API key environments. They mirror libs/go/ident.EnvSandbox / EnvLive.
const (
	EnvironmentSandbox = "sandbox"
	EnvironmentLive    = "live"
)

// Sentinel errors shared across layers. Adapters translate infrastructure
// failures (pgx.ErrNoRows, unique violations) into these.
var (
	ErrNotFound     = errors.New("resource not found")
	ErrSlugConflict = errors.New("slug is already taken")
	// ErrEmailTaken: the email already claimed its one sandbox signup.
	ErrEmailTaken = errors.New("email already has a sandbox tenant")
	// ErrSignupLimitReached: the global signups-per-day cap was hit.
	ErrSignupLimitReached = errors.New("daily sandbox signup limit reached")
)

// Tenant is a customer of the platform. The tenant registry is a system-level
// table: it is the root of the multi-tenant model, so it cannot itself be
// scoped by tenant (RLS does not apply — see migration 0001).
type Tenant struct {
	ID        uuid.UUID
	Name      string
	Slug      string
	Status    string
	CreatedAt time.Time
	// ExpiresAt is the sandbox TTL; nil means the tenant never expires.
	ExpiresAt *time.Time
}

// Active reports whether the tenant may authenticate and operate.
func (t Tenant) Active() bool { return t.Status == TenantStatusActive }

var slugPattern = regexp.MustCompile(`^[a-z0-9]+(?:-[a-z0-9]+)*$`)

// MaxSlugLength bounds tenant slugs.
const MaxSlugLength = 100

// ValidSlug reports whether s is a well-formed tenant slug:
// lowercase alphanumerics separated by single hyphens, at most 100 chars.
func ValidSlug(s string) bool {
	return s != "" && len(s) <= MaxSlugLength && slugPattern.MatchString(s)
}

// ValidEnvironment reports whether s is a known API key environment.
func ValidEnvironment(s string) bool {
	return s == EnvironmentSandbox || s == EnvironmentLive
}
