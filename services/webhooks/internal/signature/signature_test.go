package signature

import (
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"
)

// Known vectors computed with an independent HMAC-SHA256 implementation
// (Python's stdlib hmac/hashlib, not the code under test). They freeze the
// signing scheme: any change that breaks these breaks every client SDK.
//
// Recomputed when the secret prefix moved from "whsec_" to "lcwh_": the secret
// IS the HMAC key, so changing it necessarily changes every signature. These
// tests caught that, which is what a known-answer test is for.
var knownVectors = []struct {
	name   string
	secret string
	t      int64
	body   string
	hexSig string
}{
	{
		name:   "json body",
		secret: "lcwh_8Zt5xVcbXkO2qWm1nJp3rTyUuIoPaSdF",
		t:      1700000000,
		body:   `{"hello":"world"}`,
		hexSig: "58aa98444a24e97e8d9fa484e79ed0062fb53048d2597e27ee8c79d3a6bf58a3",
	},
	{
		name:   "empty object",
		secret: "lcwh_00000000000000000000000000000000",
		t:      1234567890,
		body:   `{}`,
		hexSig: "8e96e7220c92ab23d735e9fe35f56053037536afee89c643df77cf5d8cb3d812",
	},
	{
		name:   "empty body",
		secret: "lcwh_abc",
		t:      1700000001,
		body:   "",
		hexSig: "18e3a3997f57d9948422c5cfb8c42ac2852a173b6cc97e3a0a7144134d352246",
	},
}

func TestSignKnownVectors(t *testing.T) {
	for _, v := range knownVectors {
		t.Run(v.name, func(t *testing.T) {
			got := Sign(v.secret, v.t, []byte(v.body))
			if got != v.hexSig {
				t.Fatalf("Sign(%q, %d, %q) = %s, want %s", v.secret, v.t, v.body, got, v.hexSig)
			}
		})
	}
}

func TestHeaderFormat(t *testing.T) {
	v := knownVectors[0]
	got := Header(v.secret, v.t, []byte(v.body))
	want := fmt.Sprintf("t=%d,v1=%s", v.t, v.hexSig)
	if got != want {
		t.Fatalf("Header = %s, want %s", got, want)
	}
}

func TestVerifyAcceptsValidSignature(t *testing.T) {
	for _, v := range knownVectors {
		t.Run(v.name, func(t *testing.T) {
			header := Header(v.secret, v.t, []byte(v.body))
			now := time.Unix(v.t, 0).Add(time.Minute)
			if err := VerifyAt(v.secret, header, []byte(v.body), now, DefaultTolerance); err != nil {
				t.Fatalf("VerifyAt rejected a valid signature: %v", err)
			}
		})
	}
}

func TestVerifyRejectsTamperedBody(t *testing.T) {
	v := knownVectors[0]
	header := Header(v.secret, v.t, []byte(v.body))
	now := time.Unix(v.t, 0)
	tampered := []byte(`{"hello":"w0rld"}`)
	if err := VerifyAt(v.secret, header, tampered, now, DefaultTolerance); !errors.Is(err, ErrSignatureMismatch) {
		t.Fatalf("want ErrSignatureMismatch for tampered body, got %v", err)
	}
}

func TestVerifyRejectsTamperedTimestamp(t *testing.T) {
	v := knownVectors[0]
	// Signature was computed for v.t but the header claims v.t+1.
	header := fmt.Sprintf("t=%d,v1=%s", v.t+1, Sign(v.secret, v.t, []byte(v.body)))
	now := time.Unix(v.t, 0)
	if err := VerifyAt(v.secret, header, []byte(v.body), now, DefaultTolerance); !errors.Is(err, ErrSignatureMismatch) {
		t.Fatalf("want ErrSignatureMismatch for tampered timestamp, got %v", err)
	}
}

func TestVerifyRejectsWrongSecret(t *testing.T) {
	v := knownVectors[0]
	header := Header(v.secret, v.t, []byte(v.body))
	now := time.Unix(v.t, 0)
	if err := VerifyAt("lcwh_other", header, []byte(v.body), now, DefaultTolerance); !errors.Is(err, ErrSignatureMismatch) {
		t.Fatalf("want ErrSignatureMismatch for wrong secret, got %v", err)
	}
}

func TestVerifyRejectsStaleTimestamp(t *testing.T) {
	v := knownVectors[0]
	header := Header(v.secret, v.t, []byte(v.body))
	now := time.Unix(v.t, 0).Add(DefaultTolerance + time.Second)
	if err := VerifyAt(v.secret, header, []byte(v.body), now, DefaultTolerance); !errors.Is(err, ErrTimestampTooOld) {
		t.Fatalf("want ErrTimestampTooOld, got %v", err)
	}
}

func TestVerifyToleranceDisabled(t *testing.T) {
	v := knownVectors[0]
	header := Header(v.secret, v.t, []byte(v.body))
	// Ten years later, tolerance 0 disables the freshness check.
	now := time.Unix(v.t, 0).Add(10 * 365 * 24 * time.Hour)
	if err := VerifyAt(v.secret, header, []byte(v.body), now, 0); err != nil {
		t.Fatalf("tolerance 0 must skip freshness check, got %v", err)
	}
}

func TestVerifyAcceptsAnyMatchingV1(t *testing.T) {
	v := knownVectors[0]
	valid := Sign(v.secret, v.t, []byte(v.body))
	header := fmt.Sprintf("t=%d,v1=%s,v1=%s", v.t, strings.Repeat("0", 64), valid)
	now := time.Unix(v.t, 0)
	if err := VerifyAt(v.secret, header, []byte(v.body), now, DefaultTolerance); err != nil {
		t.Fatalf("VerifyAt must accept when any v1 matches, got %v", err)
	}
}

func TestVerifyIgnoresUnknownSchemes(t *testing.T) {
	v := knownVectors[0]
	header := Header(v.secret, v.t, []byte(v.body)) + ",v2=deadbeef"
	now := time.Unix(v.t, 0)
	if err := VerifyAt(v.secret, header, []byte(v.body), now, DefaultTolerance); err != nil {
		t.Fatalf("unknown schemes must be ignored, got %v", err)
	}
}

func TestVerifyMalformedHeaders(t *testing.T) {
	v := knownVectors[0]
	now := time.Unix(v.t, 0)
	cases := []string{
		"",
		"t=notanumber,v1=abc",
		"v1=abc",       // missing t
		"t=1700000000", // missing v1
		"totally-not-a-header",
	}
	for _, h := range cases {
		if err := VerifyAt(v.secret, h, []byte(v.body), now, DefaultTolerance); !errors.Is(err, ErrInvalidHeader) {
			t.Fatalf("header %q: want ErrInvalidHeader, got %v", h, err)
		}
	}
}
