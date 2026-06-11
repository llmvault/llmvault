package middleware

import (
	"net"
	"net/http"
	"strings"
)

// trueClientIPHeader, xRealIPHeader, and xForwardedForHeader are the headers a
// reverse proxy may use to convey the originating client IP. They are also the
// headers a malicious client can spoof, which is why they are only honoured when
// the immediate peer is a trusted proxy.
const (
	trueClientIPHeader  = "True-Client-IP"
	xRealIPHeader       = "X-Real-IP"
	xForwardedForHeader = "X-Forwarded-For"
)

// RealIP returns middleware that sets r.RemoteAddr to the originating client IP,
// but only trusts client-supplied forwarding headers (True-Client-IP,
// X-Real-IP, X-Forwarded-For) when the immediate connection peer falls inside
// one of the configured trusted-proxy CIDRs.
//
// chi's middleware.RealIP trusts these headers unconditionally, which lets any
// client spoof their source IP and bypass per-IP auth brute-force rate limiting
// and poison audit-log IPs. This extractor closes that hole: untrusted peers
// keep their real socket address, trusted proxies (nginx on loopback, and any
// additional configured hops) get their forwarded IP honoured.
//
// Trusted CIDRs that fail to parse are skipped; if none parse, all forwarding
// headers are ignored (fail closed).
func RealIP(trustedCIDRs []string) func(http.Handler) http.Handler {
	nets := parseCIDRs(trustedCIDRs)

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if ip := realIPFromRequest(r, nets); ip != "" {
				r.RemoteAddr = ip
			}
			next.ServeHTTP(w, r)
		})
	}
}

// realIPFromRequest derives the client IP from forwarding headers when the peer
// is trusted, otherwise returns "" (leaving r.RemoteAddr untouched).
func realIPFromRequest(r *http.Request, trusted []*net.IPNet) string {
	peerHost := r.RemoteAddr
	if h, _, err := net.SplitHostPort(r.RemoteAddr); err == nil {
		peerHost = h
	}
	peerIP := net.ParseIP(peerHost)
	if peerIP == nil || !ipInTrusted(peerIP, trusted) {
		// Untrusted (or unparseable) peer: never honour spoofable headers.
		return ""
	}

	// Prefer True-Client-IP / X-Real-IP (single value set by the trusted proxy),
	// then fall back to the first hop in X-Forwarded-For.
	if v := strings.TrimSpace(r.Header.Get(trueClientIPHeader)); v != "" {
		if ip := net.ParseIP(v); ip != nil {
			return ip.String()
		}
	}
	if v := strings.TrimSpace(r.Header.Get(xRealIPHeader)); v != "" {
		if ip := net.ParseIP(v); ip != nil {
			return ip.String()
		}
	}
	if v := r.Header.Get(xForwardedForHeader); v != "" {
		// Left-most entry is the originating client.
		for _, part := range strings.Split(v, ",") {
			candidate := strings.TrimSpace(part)
			if ip := net.ParseIP(candidate); ip != nil {
				return ip.String()
			}
		}
	}
	return ""
}

func ipInTrusted(ip net.IP, trusted []*net.IPNet) bool {
	for _, n := range trusted {
		if n.Contains(ip) {
			return true
		}
	}
	return false
}

func parseCIDRs(cidrs []string) []*net.IPNet {
	nets := make([]*net.IPNet, 0, len(cidrs))
	for _, c := range cidrs {
		c = strings.TrimSpace(c)
		if c == "" {
			continue
		}
		if _, n, err := net.ParseCIDR(c); err == nil {
			nets = append(nets, n)
		}
	}
	return nets
}
