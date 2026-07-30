// Package ident provides authentication primitives shared by all LedgerCore
// services: JWT (EdDSA) validation against the identity service JWKS, the
// canonical Claims type, and context helpers.
package ident

import (
	"context"
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/ledgercore/ledgercore/libs/go/httpx"
)

// Environment values carried in tokens.
const (
	EnvSandbox = "sandbox"
	EnvLive    = "live"
)

// Scopes are the authorization capabilities a token carries. Authentication
// (a valid signature) is not authorization: routes gate on these via
// RequireScope, and the identity service mints tokens with a bounded subset
// (never the wildcard, which only dev auth-disabled mode injects).
//
// Convention: "<service>:read" for safe/read-only operations, "<service>:write"
// for state-changing ones. Deny-by-default — a route with no matching scope
// on the token yields 403.
const (
	// ScopeWildcard grants every scope. Reserved for dev auth-disabled mode;
	// it is never issued by the identity service.
	ScopeWildcard = "*"

	ScopeLedgerRead    = "ledger:read"
	ScopeLedgerWrite   = "ledger:write"
	ScopeReconRead     = "reconciliation:read"
	ScopeReconWrite    = "reconciliation:write"
	ScopeWebhooksRead  = "webhooks:read"
	ScopeWebhooksWrite = "webhooks:write"
)

// Service audiences. A token issued by identity carries every service
// audience in its "aud" claim; each service validates that its own audience
// is present (see AuthConfig.ExpectedAudience). This rejects tokens minted
// for a different audience set (e.g. a future admin API).
const (
	AudienceLedgerCore     = "ledger-core"
	AudienceReconciliation = "reconciliation"
	AudienceWebhooks       = "webhooks"
)

// DefaultAudiences is the audience set stamped on every issued token. Kept as
// a function so callers cannot mutate a shared slice.
func DefaultAudiences() []string {
	return []string{AudienceLedgerCore, AudienceReconciliation, AudienceWebhooks}
}

// DefaultIssuer is the "iss" claim the identity service stamps and every
// service verifies.
const DefaultIssuer = "ledgercore-identity"

// DefaultClockSkew bounds the tolerated clock drift when validating exp/iat/nbf.
const DefaultClockSkew = 60 * time.Second

// Claims is the authenticated identity attached to every request.
type Claims struct {
	Subject     string
	TenantID    uuid.UUID
	Environment string // "sandbox" | "live"
	Scopes      []string
}

// HasScope reports whether the token carries the given scope. The wildcard
// scope "*" (dev auth-disabled mode only) satisfies any check.
func (c Claims) HasScope(scope string) bool {
	for _, s := range c.Scopes {
		if s == scope || s == ScopeWildcard {
			return true
		}
	}
	return false
}

type ctxKey struct{}

// ClaimsFromContext returns the claims injected by RequireAuth.
func ClaimsFromContext(ctx context.Context) (Claims, bool) {
	c, ok := ctx.Value(ctxKey{}).(Claims)
	return c, ok
}

// TenantFromContext returns the tenant id injected by RequireAuth.
func TenantFromContext(ctx context.Context) (uuid.UUID, bool) {
	c, ok := ClaimsFromContext(ctx)
	if !ok {
		return uuid.Nil, false
	}
	return c.TenantID, true
}

// ContextWithClaims returns a child context carrying the claims.
// Exported for tests and internal service-to-service calls.
func ContextWithClaims(ctx context.Context, c Claims) context.Context {
	return context.WithValue(ctx, ctxKey{}, c)
}

// tokenClaims is the JWT payload shape issued by the identity service.
type tokenClaims struct {
	TenantID    string   `json:"tenant_id"`
	Environment string   `json:"env"`
	Scopes      []string `json:"scopes"`
	jwt.RegisteredClaims
}

// jwk is the subset of RFC 7517 needed for Ed25519 (OKP) keys.
type jwk struct {
	Kty string `json:"kty"`
	Crv string `json:"crv"`
	Kid string `json:"kid"`
	X   string `json:"x"`
}

type jwks struct {
	Keys []jwk `json:"keys"`
}

// jwksCache fetches and caches the JWKS document, refreshing it in the
// background so request latency never pays for a fetch after warm-up.
type jwksCache struct {
	url    string
	client *http.Client

	mu        sync.RWMutex
	keys      map[string]ed25519.PublicKey
	fetchedAt time.Time

	refreshEvery time.Duration
	once         sync.Once
}

func newJWKSCache(url string) *jwksCache {
	return &jwksCache{
		url:          url,
		client:       &http.Client{Timeout: 10 * time.Second},
		refreshEvery: 5 * time.Minute,
	}
}

// key returns the Ed25519 public key for kid, fetching the JWKS if the cache
// is cold or stale. The background refresher is started on first use.
func (c *jwksCache) key(kid string) (ed25519.PublicKey, error) {
	c.once.Do(func() { go c.refreshLoop() })

	c.mu.RLock()
	k, ok := c.keys[kid]
	fresh := time.Since(c.fetchedAt) < c.refreshEvery
	c.mu.RUnlock()
	if ok && fresh {
		return k, nil
	}
	// Cold cache, unknown kid (possible rotation) or stale: fetch now.
	if err := c.fetch(); err != nil {
		if ok {
			// Serve the stale key rather than failing hard.
			return k, nil
		}
		return nil, err
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	if k, ok := c.keys[kid]; ok {
		return k, nil
	}
	return nil, fmt.Errorf("ident: unknown key id %q", kid)
}

func (c *jwksCache) refreshLoop() {
	ticker := time.NewTicker(c.refreshEvery)
	defer ticker.Stop()
	for range ticker.C {
		if err := c.fetch(); err != nil {
			slog.Warn("jwks background refresh failed", "url", c.url, "error", err)
		}
	}
}

func (c *jwksCache) fetch() error {
	resp, err := c.client.Get(c.url)
	if err != nil {
		return fmt.Errorf("ident: fetch jwks: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("ident: fetch jwks: unexpected status %d", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return fmt.Errorf("ident: read jwks: %w", err)
	}
	var doc jwks
	if err := json.Unmarshal(body, &doc); err != nil {
		return fmt.Errorf("ident: parse jwks: %w", err)
	}
	keys := make(map[string]ed25519.PublicKey, len(doc.Keys))
	for _, k := range doc.Keys {
		if k.Kty != "OKP" || k.Crv != "Ed25519" || k.Kid == "" {
			continue
		}
		raw, err := base64.RawURLEncoding.DecodeString(k.X)
		if err != nil || len(raw) != ed25519.PublicKeySize {
			slog.Warn("jwks: skipping malformed OKP key", "kid", k.Kid)
			continue
		}
		keys[k.Kid] = ed25519.PublicKey(raw)
	}
	if len(keys) == 0 {
		return errors.New("ident: jwks contains no usable Ed25519 keys")
	}
	c.mu.Lock()
	c.keys = keys
	c.fetchedAt = time.Now()
	c.mu.Unlock()
	return nil
}

// AuthConfig configures RequireAuthConfig. Zero values fall back to safe
// defaults (DefaultIssuer, DefaultClockSkew); an empty ExpectedAudience skips
// the audience check for backward compatibility with callers that have not
// opted in yet.
type AuthConfig struct {
	// JWKSURL is the identity service JWKS endpoint used to validate EdDSA
	// signatures.
	JWKSURL string
	// AuthDisabled trusts the X-Tenant-Id header instead of a JWT (dev only).
	AuthDisabled bool
	// ExpectedIssuer, when non-empty, requires the token "iss" to equal it.
	ExpectedIssuer string
	// ExpectedAudience, when non-empty, requires it to appear in the token
	// "aud" claim.
	ExpectedAudience string
	// ClockSkew bounds tolerated drift for exp/iat/nbf. Zero uses
	// DefaultClockSkew.
	ClockSkew time.Duration
}

// RequireAuth returns the default authentication middleware: validates the
// EdDSA signature, requires exp, enforces the platform issuer and a bounded
// clock skew, and requires a non-empty subject. It does NOT enforce an
// audience (callers opt in via RequireAuthConfig). Kept for the many callers
// that pass (jwksURL, authDisabled).
func RequireAuth(jwksURL string, authDisabled bool) func(http.Handler) http.Handler {
	return RequireAuthConfig(AuthConfig{
		JWKSURL:        jwksURL,
		AuthDisabled:   authDisabled,
		ExpectedIssuer: DefaultIssuer,
	})
}

// RequireAuthConfig returns a middleware that authenticates every request per
// cfg.
//
// Normal mode: expects "Authorization: Bearer <jwt>", validates the EdDSA
// signature against the JWKS at cfg.JWKSURL, enforces exp (required), iss,
// aud (when configured), a bounded clock skew and a non-empty subject, then
// injects Claims into the context.
//
// Disabled mode (dev only, cfg.AuthDisabled): trusts the X-Tenant-Id header
// and fabricates sandbox claims with the wildcard scope.
func RequireAuthConfig(cfg AuthConfig) func(http.Handler) http.Handler {
	cache := newJWKSCache(cfg.JWKSURL)
	authDisabled := cfg.AuthDisabled
	skew := cfg.ClockSkew
	if skew <= 0 {
		skew = DefaultClockSkew
	}
	parserOpts := []jwt.ParserOption{
		jwt.WithValidMethods([]string{jwt.SigningMethodEdDSA.Alg()}),
		jwt.WithExpirationRequired(),
		jwt.WithIssuedAt(),
		jwt.WithLeeway(skew),
	}
	if cfg.ExpectedIssuer != "" {
		parserOpts = append(parserOpts, jwt.WithIssuer(cfg.ExpectedIssuer))
	}
	if cfg.ExpectedAudience != "" {
		parserOpts = append(parserOpts, jwt.WithAudience(cfg.ExpectedAudience))
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if authDisabled {
				raw := r.Header.Get("X-Tenant-Id")
				if raw == "" {
					writeAuthError(w, r, http.StatusUnauthorized, "unauthorized", "X-Tenant-Id header is required when auth is disabled")
					return
				}
				tenantID, err := uuid.Parse(raw)
				if err != nil {
					writeAuthError(w, r, http.StatusUnauthorized, "unauthorized", "X-Tenant-Id must be a valid UUID")
					return
				}
				claims := Claims{
					Subject:     "dev",
					TenantID:    tenantID,
					Environment: EnvSandbox,
					Scopes:      []string{"*"},
				}
				next.ServeHTTP(w, r.WithContext(ContextWithClaims(r.Context(), claims)))
				return
			}

			authz := r.Header.Get("Authorization")
			const prefix = "Bearer "
			if !strings.HasPrefix(authz, prefix) {
				writeAuthError(w, r, http.StatusUnauthorized, "unauthorized", "Authorization: Bearer token is required")
				return
			}
			tokenString := strings.TrimSpace(authz[len(prefix):])

			var tc tokenClaims
			token, err := jwt.ParseWithClaims(tokenString, &tc, func(t *jwt.Token) (any, error) {
				kid, _ := t.Header["kid"].(string)
				if kid == "" {
					return nil, errors.New("token header is missing kid")
				}
				return cache.key(kid)
			}, parserOpts...)
			if err != nil || !token.Valid {
				writeAuthError(w, r, http.StatusUnauthorized, "unauthorized", "token is invalid or expired")
				return
			}

			// A signed token with an empty subject carries no principal;
			// reject it rather than attribute actions to "".
			if strings.TrimSpace(tc.Subject) == "" {
				writeAuthError(w, r, http.StatusUnauthorized, "unauthorized", "token subject (sub) is required")
				return
			}

			tenantID, err := uuid.Parse(tc.TenantID)
			if err != nil {
				writeAuthError(w, r, http.StatusUnauthorized, "unauthorized", "token tenant_id claim is not a UUID")
				return
			}
			if tc.Environment != EnvSandbox && tc.Environment != EnvLive {
				writeAuthError(w, r, http.StatusUnauthorized, "unauthorized", "token env claim must be sandbox or live")
				return
			}

			claims := Claims{
				Subject:     tc.Subject,
				TenantID:    tenantID,
				Environment: tc.Environment,
				Scopes:      tc.Scopes,
			}
			next.ServeHTTP(w, r.WithContext(ContextWithClaims(r.Context(), claims)))
		})
	}
}

// RequireScope returns a middleware that authorizes a request against a
// single required scope. It MUST be composed inside RequireAuth (it reads the
// Claims that RequireAuth injects). Missing claims -> 401; present but missing
// the scope -> 403. This is the enforcement half of the scope model: a token
// authenticates a principal, RequireScope decides what that principal may do.
func RequireScope(scope string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			claims, ok := ClaimsFromContext(r.Context())
			if !ok {
				writeAuthError(w, r, http.StatusUnauthorized, httpx.CodeUnauthorized, "authentication is required")
				return
			}
			if !claims.HasScope(scope) {
				writeAuthError(w, r, http.StatusForbidden, httpx.CodeForbidden, "token is missing the required scope "+scope)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// writeAuthError emits the platform error contract via httpx so the shape
// ({"error":{"code","message","request_id"}}) stays single-sourced.
func writeAuthError(w http.ResponseWriter, r *http.Request, status int, code, msg string) {
	httpx.WriteError(w, r, status, code, msg)
}
