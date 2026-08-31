package dispatcher

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestIsBlockedIP(t *testing.T) {
	cases := []struct {
		ip      string
		blocked bool
	}{
		{"127.0.0.1", true},             // loopback
		{"::1", true},                   // loopback v6
		{"10.0.0.5", true},              // RFC1918
		{"172.16.5.4", true},            // RFC1918
		{"192.168.1.1", true},           // RFC1918
		{"169.254.1.1", true},           // link-local
		{"169.254.169.254", true},       // cloud metadata endpoint
		{"fe80::1", true},               // link-local v6
		{"fc00::1", true},               // unique-local v6 (IsPrivate)
		{"224.0.0.1", true},             // multicast
		{"0.0.0.0", true},               // unspecified
		{"100.64.0.1", true},            // CGNAT RFC6598
		{"8.8.8.8", false},              // public
		{"1.1.1.1", false},              // public
		{"2606:4700:4700::1111", false}, // public v6
	}
	for _, c := range cases {
		ip := net.ParseIP(c.ip)
		if ip == nil {
			t.Fatalf("bad test ip %q", c.ip)
		}
		if got := isBlockedIP(ip); got != c.blocked {
			t.Errorf("isBlockedIP(%s) = %v, want %v", c.ip, got, c.blocked)
		}
	}
}

// The hardened client must refuse to connect to a loopback address even though
// the target is a valid, reachable server. This is the same connect-time IP
// check that stops a redirect or DNS-rebind to an internal address.
func TestSafeClientRejectsLoopback(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	client := newSafeClient(2 * time.Second)
	c := claimed(srv.URL, "lcwh_x") // srv.URL is 127.0.0.1:PORT
	_, err := Send(context.Background(), client, c)
	if err == nil {
		t.Fatal("expected the SSRF guard to block a loopback delivery")
	}
	if !strings.Contains(err.Error(), "blocked address") {
		t.Fatalf("expected SSRF guard error, got %v", err)
	}
}

// Every hop (initial request or redirect target) is dialed through the guarded
// transport, so a Location pointing at an internal address is refused at
// connect time. Redirects and the initial request share this exact mechanism,
// which is also what defeats DNS rebinding (the connect-time IP is vetted, not
// just the URL seen at validation time).
func TestSafeClientRejectsInternalTarget(t *testing.T) {
	client := newSafeClient(2 * time.Second)
	_, err := client.Get("http://169.254.169.254/latest/meta-data/")
	if err == nil {
		t.Fatal("expected metadata endpoint to be blocked")
	}
	if !strings.Contains(err.Error(), "blocked address") {
		t.Fatalf("expected SSRF guard error, got %v", err)
	}
}

func TestSafeControlBlocksMetadata(t *testing.T) {
	if err := safeControl("tcp", "169.254.169.254:80", nil); err == nil {
		t.Fatal("safeControl must block the metadata endpoint")
	}
	if err := safeControl("tcp", "8.8.8.8:443", nil); err != nil {
		t.Fatalf("safeControl must allow a public address, got %v", err)
	}
}
