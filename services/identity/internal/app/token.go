package app

import (
	"context"
	"crypto/ed25519"
	"encoding/base64"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"

	"github.com/ledgercore/ledgercore/services/identity/internal/domain"
)

// IssuedToken is the OAuth2-style response of POST /v1/auth/token.
type IssuedToken struct {
	AccessToken string `json:"access_token"`
	TokenType   string `json:"token_type"`
	ExpiresIn   int    `json:"expires_in"`
}

// tokenClaims is the JWT payload. The claim names (tenant_id, env, scopes)
// are the platform contract defined by libs/go/ident.RequireAuth — every
// service validates tokens with that middleware, so the issuer must match it
// exactly.
type tokenClaims struct {
	TenantID    string   `json:"tenant_id"`
	Environment string   `json:"env"`
	Scopes      []string `json:"scopes"`
	jwt.RegisteredClaims
}

// TokenIssuer signs EdDSA JWTs with the active signing key.
type TokenIssuer struct {
	kid string
	key ed25519.PrivateKey
	ttl time.Duration
	now func() time.Time
}

// NewTokenIssuer builds an issuer from a persisted signing key.
func NewTokenIssuer(key domain.SigningKey, ttl time.Duration) (*TokenIssuer, error) {
	if key.Algorithm != domain.AlgorithmEdDSA {
		return nil, fmt.Errorf("app: unsupported signing algorithm %q", key.Algorithm)
	}
	priv, err := key.Ed25519PrivateKey()
	if err != nil {
		return nil, err
	}
	return &TokenIssuer{
		kid: key.Kid.String(),
		key: priv,
		ttl: ttl,
		now: func() time.Time { return time.Now().UTC() },
	}, nil
}

// Issue signs a token for the given API key / tenant pair.
func (i *TokenIssuer) Issue(apiKeyID, tenantID uuid.UUID, environment string, scopes []string) (IssuedToken, error) {
	now := i.now()
	claims := tokenClaims{
		TenantID:    tenantID.String(),
		Environment: environment,
		Scopes:      scopes,
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   "api_key:" + apiKeyID.String(),
			Issuer:    "ledgercore-identity",
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(i.ttl)),
			ID:        uuid.NewString(),
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodEdDSA, claims)
	token.Header["kid"] = i.kid
	signed, err := token.SignedString(i.key)
	if err != nil {
		return IssuedToken{}, fmt.Errorf("app: sign token: %w", err)
	}
	return IssuedToken{
		AccessToken: signed,
		TokenType:   "Bearer",
		ExpiresIn:   int(i.ttl.Seconds()),
	}, nil
}

// ---- JWKS ------------------------------------------------------------------

// JWK is an RFC 7517 Ed25519 (OKP) public key entry.
type JWK struct {
	Kty string `json:"kty"`
	Crv string `json:"crv"`
	Kid string `json:"kid"`
	X   string `json:"x"`
	Alg string `json:"alg"`
	Use string `json:"use"`
}

// JWKSDocument is the /.well-known/jwks.json response body.
type JWKSDocument struct {
	Keys []JWK `json:"keys"`
}

// JWKS builds the public key set from every active signing key.
func (s *Service) JWKS(ctx context.Context) (JWKSDocument, error) {
	keys, err := s.signing.ListActiveSigningKeys(ctx)
	if err != nil {
		return JWKSDocument{}, fmt.Errorf("app: list signing keys: %w", err)
	}
	doc := JWKSDocument{Keys: make([]JWK, 0, len(keys))}
	for _, k := range keys {
		pub, err := k.Ed25519PublicKey()
		if err != nil {
			return JWKSDocument{}, fmt.Errorf("app: decode public key %s: %w", k.Kid, err)
		}
		doc.Keys = append(doc.Keys, JWK{
			Kty: "OKP",
			Crv: "Ed25519",
			Kid: k.Kid.String(),
			X:   base64.RawURLEncoding.EncodeToString(pub),
			Alg: domain.AlgorithmEdDSA,
			Use: "sig",
		})
	}
	return doc, nil
}
