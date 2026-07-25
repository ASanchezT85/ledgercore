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
)

// Environment values carried in tokens.
const (
	EnvSandbox = "sandbox"
	EnvLive    = "live"
)

// Claims is the authenticated identity attached to every request.
type Claims struct {
	Subject     string
	TenantID    uuid.UUID
	Environment string // "sandbox" | "live"
	Scopes      []string
}

// HasScope reports whether the token carries the given scope.
func (c Claims) HasScope(scope string) bool {
	for _, s := range c.Scopes {
		if s == scope {
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

// RequireAuth returns a middleware that authenticates every request.
//
// Normal mode: expects "Authorization: Bearer <jwt>", validates the EdDSA
// signature against the JWKS at jwksURL and injects Claims into the context.
//
// Disabled mode (dev only, LEDGERCORE_AUTH_DISABLED=true): trusts the
// X-Tenant-Id header and fabricates sandbox claims with full scope.
func RequireAuth(jwksURL string, authDisabled bool) func(http.Handler) http.Handler {
	cache := newJWKSCache(jwksURL)

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if authDisabled {
				raw := r.Header.Get("X-Tenant-Id")
				if raw == "" {
					writeAuthError(w, http.StatusUnauthorized, "missing_tenant", "X-Tenant-Id header is required when auth is disabled")
					return
				}
				tenantID, err := uuid.Parse(raw)
				if err != nil {
					writeAuthError(w, http.StatusUnauthorized, "invalid_tenant", "X-Tenant-Id must be a valid UUID")
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
				writeAuthError(w, http.StatusUnauthorized, "missing_token", "Authorization: Bearer token is required")
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
			}, jwt.WithValidMethods([]string{jwt.SigningMethodEdDSA.Alg()}), jwt.WithExpirationRequired())
			if err != nil || !token.Valid {
				writeAuthError(w, http.StatusUnauthorized, "invalid_token", "token is invalid or expired")
				return
			}

			tenantID, err := uuid.Parse(tc.TenantID)
			if err != nil {
				writeAuthError(w, http.StatusUnauthorized, "invalid_token", "token tenant_id claim is not a UUID")
				return
			}
			if tc.Environment != EnvSandbox && tc.Environment != EnvLive {
				writeAuthError(w, http.StatusUnauthorized, "invalid_token", "token env claim must be sandbox or live")
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

func writeAuthError(w http.ResponseWriter, status int, code, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"error": map[string]string{"code": code, "message": msg},
	})
}
