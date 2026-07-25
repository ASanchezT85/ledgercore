// Package signature implements the LedgerCore webhook signing scheme and is
// the reference implementation for client SDKs.
//
// Every webhook POST carries the header:
//
//	X-LedgerCore-Signature: t=<unix seconds>,v1=<hex hmac-sha256>
//
// where the MAC input is "<t>." followed by the raw request body, keyed with
// the subscription secret (whsec_...). Receivers must recompute the MAC over
// the exact bytes they received and compare in constant time, and should
// reject timestamps outside their tolerance window to prevent replay.
package signature

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// HeaderName is the canonical signature header.
const HeaderName = "X-LedgerCore-Signature"

// SchemeV1 identifies the current signing scheme inside the header.
const SchemeV1 = "v1"

// DefaultTolerance is the recommended replay-protection window.
const DefaultTolerance = 5 * time.Minute

// Verification errors. They are sentinel values so SDK-style callers can
// branch on the failure reason.
var (
	ErrInvalidHeader     = errors.New("signature: malformed signature header")
	ErrTimestampTooOld   = errors.New("signature: timestamp outside tolerance")
	ErrSignatureMismatch = errors.New("signature: no matching v1 signature")
)

// Sign computes the hex-encoded HMAC-SHA256 of "<t>.<body>" keyed with secret.
func Sign(secret string, t int64, body []byte) string {
	return hex.EncodeToString(mac(secret, t, body))
}

// Header builds the full signature header value: "t=<t>,v1=<signature>".
func Header(secret string, t int64, body []byte) string {
	return fmt.Sprintf("t=%d,%s=%s", t, SchemeV1, Sign(secret, t, body))
}

// Verify checks a received header against the body using time.Now as the
// reference clock. A tolerance <= 0 disables the timestamp freshness check.
func Verify(secret, header string, body []byte, tolerance time.Duration) error {
	return VerifyAt(secret, header, body, time.Now(), tolerance)
}

// VerifyAt is Verify with an explicit reference clock, for deterministic
// tests. It accepts the header if ANY v1 entry matches (this allows secret
// rotation windows where senders sign with two secrets).
func VerifyAt(secret, header string, body []byte, now time.Time, tolerance time.Duration) error {
	ts, candidates, err := parseHeader(header)
	if err != nil {
		return err
	}
	if tolerance > 0 {
		diff := now.Sub(time.Unix(ts, 0))
		if diff < 0 {
			diff = -diff
		}
		if diff > tolerance {
			return ErrTimestampTooOld
		}
	}
	expected := mac(secret, ts, body)
	for _, c := range candidates {
		raw, err := hex.DecodeString(c)
		if err != nil {
			continue
		}
		// hmac.Equal is a constant-time comparison.
		if hmac.Equal(raw, expected) {
			return nil
		}
	}
	return ErrSignatureMismatch
}

func mac(secret string, t int64, body []byte) []byte {
	h := hmac.New(sha256.New, []byte(secret))
	h.Write([]byte(strconv.FormatInt(t, 10)))
	h.Write([]byte("."))
	h.Write(body)
	return h.Sum(nil)
}

// parseHeader extracts the timestamp and every v1 signature candidate from a
// header of the form "t=123,v1=abc[,v1=def...]".
func parseHeader(header string) (int64, []string, error) {
	var (
		ts         int64
		haveTS     bool
		candidates []string
	)
	for _, part := range strings.Split(header, ",") {
		key, value, found := strings.Cut(strings.TrimSpace(part), "=")
		if !found {
			return 0, nil, ErrInvalidHeader
		}
		switch key {
		case "t":
			parsed, err := strconv.ParseInt(value, 10, 64)
			if err != nil {
				return 0, nil, ErrInvalidHeader
			}
			ts = parsed
			haveTS = true
		case SchemeV1:
			candidates = append(candidates, value)
		default:
			// Ignore unknown keys for forward compatibility (e.g. v2).
		}
	}
	if !haveTS || len(candidates) == 0 {
		return 0, nil, ErrInvalidHeader
	}
	return ts, candidates, nil
}
