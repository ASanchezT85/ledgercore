package app

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/ledgercore/ledgercore/libs/go/httpx"
	"github.com/ledgercore/ledgercore/libs/go/ident"
	"github.com/ledgercore/ledgercore/services/identity/internal/domain"
)

// ---- In-memory fakes ---------------------------------------------------------

type fakeStore struct {
	mu      sync.Mutex
	tenants map[uuid.UUID]domain.Tenant
	keys    map[uuid.UUID]domain.APIKey
	signing []domain.SigningKey
}

func newFakeStore() *fakeStore {
	return &fakeStore{
		tenants: map[uuid.UUID]domain.Tenant{},
		keys:    map[uuid.UUID]domain.APIKey{},
	}
}

func (f *fakeStore) CreateTenant(_ context.Context, t domain.Tenant) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, existing := range f.tenants {
		if existing.Slug == t.Slug {
			return domain.ErrSlugConflict
		}
	}
	f.tenants[t.ID] = t
	return nil
}

func (f *fakeStore) GetTenant(_ context.Context, id uuid.UUID) (domain.Tenant, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	t, ok := f.tenants[id]
	if !ok {
		return domain.Tenant{}, domain.ErrNotFound
	}
	return t, nil
}

func (f *fakeStore) ListTenants(_ context.Context, _ int, _ httpx.Cursor) ([]domain.Tenant, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]domain.Tenant, 0, len(f.tenants))
	for _, t := range f.tenants {
		out = append(out, t)
	}
	return out, nil
}

func (f *fakeStore) CreateAPIKey(_ context.Context, k domain.APIKey) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.keys[k.ID] = k
	return nil
}

func (f *fakeStore) FindAPIKeysByPrefix(_ context.Context, prefix string) ([]domain.APIKey, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []domain.APIKey
	for _, k := range f.keys {
		if k.KeyPrefix == prefix {
			out = append(out, k)
		}
	}
	return out, nil
}

func (f *fakeStore) RevokeAPIKey(_ context.Context, id uuid.UUID) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	k, ok := f.keys[id]
	if !ok {
		return domain.ErrNotFound
	}
	if k.RevokedAt == nil {
		now := time.Now().UTC()
		k.RevokedAt = &now
		f.keys[id] = k
	}
	return nil
}

func (f *fakeStore) InsertSigningKey(_ context.Context, k domain.SigningKey) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.signing = append(f.signing, k)
	return nil
}

func (f *fakeStore) ListActiveSigningKeys(_ context.Context) ([]domain.SigningKey, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []domain.SigningKey
	for _, k := range f.signing {
		if k.Active {
			out = append(out, k)
		}
	}
	return out, nil
}

func (f *fakeStore) UpdateSigningKeyPrivatePEM(_ context.Context, kid uuid.UUID, privateKeyPEM string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	for i, k := range f.signing {
		if k.Kid == kid {
			f.signing[i].PrivateKeyPEM = privateKeyPEM
			return nil
		}
	}
	return domain.ErrNotFound
}

// ---- Helpers ---------------------------------------------------------

func newTestService(t *testing.T) (*Service, *fakeStore) {
	t.Helper()
	store := newFakeStore()
	signingKey, err := EnsureSigningKey(context.Background(), store, nil)
	if err != nil {
		t.Fatalf("EnsureSigningKey: %v", err)
	}
	issuer, err := NewTokenIssuer(signingKey, TokenTTL)
	if err != nil {
		t.Fatalf("NewTokenIssuer: %v", err)
	}
	return NewService(store, store, store, issuer), store
}

// ---- Tests ---------------------------------------------------------

func TestEnsureSigningKeyIsIdempotent(t *testing.T) {
	store := newFakeStore()
	first, err := EnsureSigningKey(context.Background(), store, nil)
	if err != nil {
		t.Fatal(err)
	}
	second, err := EnsureSigningKey(context.Background(), store, nil)
	if err != nil {
		t.Fatal(err)
	}
	if first.Kid != second.Kid {
		t.Errorf("second call generated a new key: %s != %s", first.Kid, second.Kid)
	}
	if len(store.signing) != 1 {
		t.Errorf("stored %d keys, want 1", len(store.signing))
	}
}

func TestCreateTenantValidation(t *testing.T) {
	svc, _ := newTestService(t)
	ctx := context.Background()

	var verr ValidationError
	if _, err := svc.CreateTenant(ctx, "", "acme"); !errors.As(err, &verr) {
		t.Errorf("empty name: got %v, want ValidationError", err)
	}
	if _, err := svc.CreateTenant(ctx, "Acme", "Not A Slug!"); !errors.As(err, &verr) {
		t.Errorf("bad slug: got %v, want ValidationError", err)
	}

	tenant, err := svc.CreateTenant(ctx, "Acme Payments", "acme-payments")
	if err != nil {
		t.Fatal(err)
	}
	if tenant.Status != domain.TenantStatusActive {
		t.Errorf("status = %q, want active", tenant.Status)
	}

	if _, err := svc.CreateTenant(ctx, "Acme Clone", "acme-payments"); !errors.Is(err, domain.ErrSlugConflict) {
		t.Errorf("duplicate slug: got %v, want ErrSlugConflict", err)
	}
}

func TestCreateAPIKeyStoresOnlyHash(t *testing.T) {
	svc, store := newTestService(t)
	ctx := context.Background()

	tenant, err := svc.CreateTenant(ctx, "Acme", "acme")
	if err != nil {
		t.Fatal(err)
	}
	key, secret, err := svc.CreateAPIKey(ctx, tenant.ID, domain.EnvironmentSandbox, "ci")
	if err != nil {
		t.Fatal(err)
	}
	if secret == "" || key.KeyPrefix != domain.PrefixOf(secret) {
		t.Errorf("prefix %q does not match secret %q", key.KeyPrefix, secret)
	}
	stored := store.keys[key.ID]
	if string(stored.SecretHash) == secret {
		t.Error("plaintext secret was stored")
	}
	if !stored.VerifySecret(secret) {
		t.Error("stored hash does not verify the returned secret")
	}

	if _, _, err := svc.CreateAPIKey(ctx, uuid.New(), domain.EnvironmentSandbox, "ci"); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("unknown tenant: got %v, want ErrNotFound", err)
	}
	var verr ValidationError
	if _, _, err := svc.CreateAPIKey(ctx, tenant.ID, "prod", "ci"); !errors.As(err, &verr) {
		t.Errorf("bad environment: got %v, want ValidationError", err)
	}
}

// TestIssueTokenRoundTripAgainstOwnJWKS proves the whole loop: the service
// issues a JWT, serves its own JWKS, and libs/go/ident.RequireAuth (the
// middleware every other service uses) accepts the token and reconstructs
// the exact claims.
func TestIssueTokenRoundTripAgainstOwnJWKS(t *testing.T) {
	svc, _ := newTestService(t)
	ctx := context.Background()

	tenant, err := svc.CreateTenant(ctx, "Acme", "acme")
	if err != nil {
		t.Fatal(err)
	}
	key, secret, err := svc.CreateAPIKey(ctx, tenant.ID, domain.EnvironmentLive, "prod-key")
	if err != nil {
		t.Fatal(err)
	}

	token, err := svc.IssueToken(ctx, secret)
	if err != nil {
		t.Fatalf("IssueToken: %v", err)
	}
	if token.TokenType != "Bearer" {
		t.Errorf("token_type = %q, want Bearer", token.TokenType)
	}
	if token.ExpiresIn != int(TokenTTL.Seconds()) {
		t.Errorf("expires_in = %d, want %d", token.ExpiresIn, int(TokenTTL.Seconds()))
	}

	// Serve the JWKS the way the real service does.
	jwksSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		doc, err := svc.JWKS(r.Context())
		if err != nil {
			t.Errorf("JWKS: %v", err)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		httpx.WriteJSON(w, http.StatusOK, doc)
	}))
	defer jwksSrv.Close()

	// Validate with the shared middleware, exactly like ledger-core would.
	var got ident.Claims
	var authenticated bool
	protected := ident.RequireAuth(jwksSrv.URL, false)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got, authenticated = ident.ClaimsFromContext(r.Context())
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/protected", nil)
	req.Header.Set("Authorization", "Bearer "+token.AccessToken)
	rec := httptest.NewRecorder()
	protected.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("middleware rejected the token: status %d, body %s", rec.Code, rec.Body.String())
	}
	if !authenticated {
		t.Fatal("claims missing from context")
	}
	if got.TenantID != tenant.ID {
		t.Errorf("tenant id = %s, want %s", got.TenantID, tenant.ID)
	}
	if got.Environment != domain.EnvironmentLive {
		t.Errorf("environment = %q, want live", got.Environment)
	}
	if got.Subject != "api_key:"+key.ID.String() {
		t.Errorf("subject = %q, want api_key:%s", got.Subject, key.ID)
	}
	for _, scope := range DefaultScopes {
		if !got.HasScope(scope) {
			t.Errorf("missing scope %q in %v", scope, got.Scopes)
		}
	}
}

func TestIssueTokenRejectsRevokedKey(t *testing.T) {
	svc, _ := newTestService(t)
	ctx := context.Background()

	tenant, err := svc.CreateTenant(ctx, "Acme", "acme")
	if err != nil {
		t.Fatal(err)
	}
	key, secret, err := svc.CreateAPIKey(ctx, tenant.ID, domain.EnvironmentSandbox, "ci")
	if err != nil {
		t.Fatal(err)
	}

	if _, err := svc.IssueToken(ctx, secret); err != nil {
		t.Fatalf("token before revocation should succeed: %v", err)
	}
	if err := svc.RevokeAPIKey(ctx, key.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.IssueToken(ctx, secret); !errors.Is(err, ErrInvalidCredentials) {
		t.Errorf("revoked key: got %v, want ErrInvalidCredentials", err)
	}
	// Revoking again is a no-op, not an error.
	if err := svc.RevokeAPIKey(ctx, key.ID); err != nil {
		t.Errorf("double revoke: %v", err)
	}
}

func TestIssueTokenRejectsSuspendedTenantAndGarbage(t *testing.T) {
	svc, store := newTestService(t)
	ctx := context.Background()

	tenant, err := svc.CreateTenant(ctx, "Acme", "acme")
	if err != nil {
		t.Fatal(err)
	}
	_, secret, err := svc.CreateAPIKey(ctx, tenant.ID, domain.EnvironmentSandbox, "ci")
	if err != nil {
		t.Fatal(err)
	}

	// Suspend the tenant behind the key.
	store.mu.Lock()
	suspended := store.tenants[tenant.ID]
	suspended.Status = domain.TenantStatusSuspended
	store.tenants[tenant.ID] = suspended
	store.mu.Unlock()

	if _, err := svc.IssueToken(ctx, secret); !errors.Is(err, ErrInvalidCredentials) {
		t.Errorf("suspended tenant: got %v, want ErrInvalidCredentials", err)
	}

	for _, bad := range []string{"", "lk_", "nonsense", "lk_sandbox_wrongsecretwrongsecretwrongsec"} {
		if _, err := svc.IssueToken(ctx, bad); !errors.Is(err, ErrInvalidCredentials) {
			t.Errorf("IssueToken(%q): got %v, want ErrInvalidCredentials", bad, err)
		}
	}
}

func TestJWKSListsActiveKeys(t *testing.T) {
	svc, store := newTestService(t)
	doc, err := svc.JWKS(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(doc.Keys) != 1 {
		t.Fatalf("jwks has %d keys, want 1", len(doc.Keys))
	}
	k := doc.Keys[0]
	if k.Kty != "OKP" || k.Crv != "Ed25519" || k.X == "" {
		t.Errorf("malformed JWK: %+v", k)
	}
	kids := make([]string, 0, len(store.signing))
	for _, sk := range store.signing {
		kids = append(kids, sk.Kid.String())
	}
	if !slices.Contains(kids, k.Kid) {
		t.Errorf("jwks kid %s not among stored kids %v", k.Kid, kids)
	}
}
