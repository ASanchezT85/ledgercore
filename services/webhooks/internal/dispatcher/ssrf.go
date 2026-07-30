package dispatcher

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"syscall"
	"time"
)

// SSRF hardening for the delivery worker.
//
// Webhook endpoints are attacker-controlled URLs. Without a network policy the
// dispatcher would happily POST to loopback, RFC1918, link-local (including the
// cloud metadata endpoint 169.254.169.254) or other reserved ranges, turning
// the service into a server-side request forgery pivot into the internal
// network.
//
// The guard validates the *actual IP a connection is opened to*, via the
// dialer Control hook, which runs for the initial request AND for every
// redirect hop after DNS resolution. Validating the connect-time IP (not just
// the URL) closes the TOCTOU / DNS-rebinding gap where a hostname resolves to a
// public IP at check time and an internal IP at connect time. Redirects are
// additionally capped, and the transport keeps strict timeouts.

const (
	// maxRedirects bounds how many redirect hops a delivery may follow. Each
	// hop is still IP-validated by the dialer Control hook.
	maxRedirects = 3
	// dialTimeout bounds the TCP connect per hop.
	dialTimeout = 5 * time.Second
	// tlsTimeout bounds the TLS handshake per hop.
	tlsTimeout = 5 * time.Second
)

// errBlockedIP is returned by the dialer when a destination resolves to a
// non-public address.
type errBlockedIP struct{ ip string }

func (e errBlockedIP) Error() string {
	return fmt.Sprintf("dispatcher: refusing to connect to blocked address %s (SSRF guard)", e.ip)
}

// isBlockedIP reports whether ip is one the dispatcher must never connect to:
// loopback, private (RFC1918 / fc00::/7), link-local (169.254/16 incl. the
// cloud metadata address, fe80::/10), CGNAT (100.64/10), multicast,
// interface-local, and the unspecified address. Anything that is not a plain
// global-unicast address is rejected.
func isBlockedIP(ip net.IP) bool {
	if ip == nil {
		return true
	}
	if ip.IsLoopback() ||
		ip.IsPrivate() ||
		ip.IsLinkLocalUnicast() ||
		ip.IsLinkLocalMulticast() ||
		ip.IsInterfaceLocalMulticast() ||
		ip.IsMulticast() ||
		ip.IsUnspecified() {
		return true
	}
	// Carrier-grade NAT 100.64.0.0/10 (RFC 6598): not covered by IsPrivate.
	if v4 := ip.To4(); v4 != nil {
		if v4[0] == 100 && v4[1] >= 64 && v4[1] <= 127 {
			return true
		}
	}
	// Belt and braces: only allow addresses classified as global unicast.
	return !ip.IsGlobalUnicast()
}

// safeControl is the net.Dialer Control hook. address is "host:port" with host
// already resolved to a literal IP, so validating it here vets the exact peer
// the socket connects to.
func safeControl(_, address string, _ syscall.RawConn) error {
	host, _, err := net.SplitHostPort(address)
	if err != nil {
		return fmt.Errorf("dispatcher: cannot parse dial address %q: %w", address, err)
	}
	ip := net.ParseIP(host)
	if ip == nil {
		// Control always receives a resolved IP literal; a non-IP here is
		// unexpected, so fail closed.
		return errBlockedIP{ip: host}
	}
	if isBlockedIP(ip) {
		return errBlockedIP{ip: ip.String()}
	}
	return nil
}

// newSafeClient builds the hardened HTTP client used by the dispatcher: a
// per-hop IP allow-check, capped redirects and strict timeouts.
func newSafeClient(timeout time.Duration) *http.Client {
	dialer := &net.Dialer{
		Timeout:   dialTimeout,
		KeepAlive: 30 * time.Second,
		Control:   safeControl,
	}
	transport := &http.Transport{
		DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			return dialer.DialContext(ctx, network, addr)
		},
		TLSHandshakeTimeout:   tlsTimeout,
		ResponseHeaderTimeout: timeout,
		ExpectContinueTimeout: 1 * time.Second,
		MaxIdleConns:          100,
		IdleConnTimeout:       90 * time.Second,
		ForceAttemptHTTP2:     true,
	}
	return &http.Client{
		Timeout:   timeout,
		Transport: transport,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= maxRedirects {
				return fmt.Errorf("dispatcher: stopped after %d redirects", maxRedirects)
			}
			return nil
		},
	}
}
