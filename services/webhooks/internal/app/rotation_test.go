package app

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/ASanchezT85/ledgercore/libs/go/httpx"
	"github.com/ASanchezT85/ledgercore/services/webhooks/internal/domain"
	"github.com/ASanchezT85/ledgercore/services/webhooks/internal/signature"
)

// fakeSubs implements SubscriptionStore in memory for rotation tests.
type fakeSubs struct {
	sub domain.Subscription
}

func (f *fakeSubs) Create(_ context.Context, s domain.Subscription) error { f.sub = s; return nil }
func (f *fakeSubs) List(context.Context, uuid.UUID, int, httpx.Cursor) ([]domain.Subscription, error) {
	return []domain.Subscription{f.sub}, nil
}
func (f *fakeSubs) Get(context.Context, uuid.UUID, uuid.UUID) (domain.Subscription, error) {
	return f.sub, nil
}
func (f *fakeSubs) Update(_ context.Context, _, _ uuid.UUID, _ *string, _ []string, _ *bool) (domain.Subscription, error) {
	return f.sub, nil
}
func (f *fakeSubs) RotateSecret(_ context.Context, _, _ uuid.UUID, secret string, prevExpiresAt time.Time) error {
	old := f.sub.Secret
	f.sub.PreviousSecret = &old
	f.sub.PreviousSecretExpiresAt = &prevExpiresAt
	f.sub.Secret = secret
	return nil
}
func (f *fakeSubs) ListActive(context.Context, uuid.UUID) ([]domain.Subscription, error) {
	return []domain.Subscription{f.sub}, nil
}

func TestRotateSecretGracePeriod(t *testing.T) {
	subs := &fakeSubs{sub: domain.Subscription{
		ID:       uuid.New(),
		TenantID: uuid.New(),
		Secret:   "lcwh_old0000000000000000000000000000",
	}}
	svc := NewService(subs, nil)

	oldSecret := subs.sub.Secret
	newSecret, prevExpires, err := svc.RotateSecret(context.Background(), subs.sub.TenantID, subs.sub.ID)
	if err != nil {
		t.Fatalf("rotate: %v", err)
	}
	if !strings.HasPrefix(newSecret, domain.SecretPrefix) || newSecret == oldSecret {
		t.Fatalf("bad new secret %q", newSecret)
	}
	if until := time.Until(prevExpires); until < domain.RotationGrace-time.Minute || until > domain.RotationGrace+time.Minute {
		t.Fatalf("grace deadline %v not ~%v away", prevExpires, domain.RotationGrace)
	}

	now := time.Now().UTC()
	// Inside the grace window both secrets sign.
	secrets := subs.sub.SigningSecrets(now)
	if len(secrets) != 2 || secrets[0] != newSecret || secrets[1] != oldSecret {
		t.Fatalf("signing secrets during grace = %v", secrets)
	}
	// A delivery signed now verifies against the OLD secret (receiver that
	// has not yet switched)...
	body := []byte(`{"hello":"world"}`)
	header := signature.HeaderMulti(secrets, now.Unix(), body)
	if err := signature.VerifyAt(oldSecret, header, body, now, signature.DefaultTolerance); err != nil {
		t.Fatalf("old secret must verify during grace: %v", err)
	}
	// ...and against the new one.
	if err := signature.VerifyAt(newSecret, header, body, now, signature.DefaultTolerance); err != nil {
		t.Fatalf("new secret must verify: %v", err)
	}

	// After the 24h grace the old secret is no longer used for signing, so a
	// receiver still pinned to it stops verifying.
	after := now.Add(domain.RotationGrace + time.Minute)
	secrets = subs.sub.SigningSecrets(after)
	if len(secrets) != 1 || secrets[0] != newSecret {
		t.Fatalf("signing secrets after grace = %v", secrets)
	}
	header = signature.HeaderMulti(secrets, after.Unix(), body)
	if err := signature.VerifyAt(oldSecret, header, body, after, signature.DefaultTolerance); err == nil {
		t.Fatal("old secret must NOT verify after the grace window")
	}
}
