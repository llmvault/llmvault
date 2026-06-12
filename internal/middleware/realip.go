package middleware

import (
	"net"
	"net/http"
	"strings"
)

// Headers a reverse proxy uses to convey the originating client IP. A malicious client can spoof
// them, so they are only honoured when the peer is a trusted proxy.
const (
	trueClientIPHeader  = "True-Client-IP"
	xRealIPHeader       = "X-Real-IP"
	xForwardedForHeader = "X-Forwarded-For"
)

// RealIP sets r.RemoteAddr to the client IP, trusting forwarding headers only
// when the peer is inside a trusted-proxy CIDR (unlike chi's RealIP, which trusts
// them unconditionally and lets any client spoof its source IP).
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

// realIPFromRequest derives the client IP from forwarding headers for a trusted
// peer, else returns "".
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
