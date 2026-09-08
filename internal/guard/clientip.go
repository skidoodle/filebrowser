package guard

import (
	"net/http"
	"net/netip"
	"slices"
	"strings"
)

// ClientIP resolves the real client address, honoring X-Forwarded-For only
// through the configured trusted proxy CIDRs. Untrusted clients cannot
// spoof an IP by setting the header themselves.
func ClientIP(r *http.Request, trusted []netip.Prefix) netip.Addr {
	remote, err := netip.ParseAddrPort(r.RemoteAddr)
	if err != nil {
		return netip.Addr{}
	}
	client := remote.Addr().Unmap()

	xff := r.Header.Get("X-Forwarded-For")
	if xff == "" {
		return client
	}
	parts := strings.Split(xff, ",")

	// Walk from the right: each hop must be a trusted proxy for the next
	// (leftward) entry to be believed.
	for _, part := range slices.Backward(parts) {
		if !isTrusted(client, trusted) {
			break
		}
		candidate, err := netip.ParseAddr(strings.TrimSpace(part))
		if err != nil {
			break
		}
		client = candidate.Unmap()
	}
	return client
}

func isTrusted(addr netip.Addr, trusted []netip.Prefix) bool {
	if !addr.IsValid() {
		return false
	}
	for _, p := range trusted {
		if p.Contains(addr) {
			return true
		}
	}
	return false
}

// TrustedFor reports whether the direct peer is a configured proxy.
func TrustedFor(r *http.Request, trusted []netip.Prefix) bool {
	remote, err := netip.ParseAddrPort(r.RemoteAddr)
	if err != nil {
		return false
	}
	return isTrusted(remote.Addr().Unmap(), trusted)
}
