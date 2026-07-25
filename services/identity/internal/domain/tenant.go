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
