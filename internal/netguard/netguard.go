// Package netguard provides rebinding-safe SSRF protections: validation of a
// destination URL plus an http.Transport whose DialContext re-checks and pins
// the resolved address. It mirrors the logic in internal/proxy but lives in a
// dependency-free leaf package so callers (e.g. internal/gateway) can reuse it
// without creating an import cycle through internal/proxy's other files.
package netguard

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

var (
	// Disallowed networks (IPv4)
	_, loopback4, _    = net.ParseCIDR("127.0.0.0/8")
	_, linkLocal4, _   = net.ParseCIDR("169.254.0.0/16")
	_, privateA, _     = net.ParseCIDR("10.0.0.0/8")
	_, privateB, _     = net.ParseCIDR("172.16.0.0/12")
	_, privateC, _     = net.ParseCIDR("192.168.0.0/16")
	_, cgNAT, _        = net.ParseCIDR("100.64.0.0/10")
	_, multicast4, _   = net.ParseCIDR("224.0.0.0/4")
	_, reserved4, _    = net.ParseCIDR("240.0.0.0/4")
	_, unspecified4, _ = net.ParseCIDR("0.0.0.0/8")
	// Disallowed networks (IPv6)
	_, loopback6, _    = net.ParseCIDR("::1/128")
	_, linkLocal6, _   = net.ParseCIDR("fe80::/10")
	_, uniqueLocal6, _ = net.ParseCIDR("fc00::/7")
	_, multicast6, _   = net.ParseCIDR("ff00::/8")
	_, unspecified6, _ = net.ParseCIDR("::/128")

	// Explicitly blocked hostnames commonly used for metadata/internal resolution.
	blockedHostnames = map[string]struct{}{
		"localhost":                {},
		"localhost.localdomain":    {},
		"metadata.google.internal": {},
		"metadata":                 {},
	}

	// AllowLoopback can be set to true in tests to allow loopback/private addresses.
	AllowLoopback = false
)

func ipInNets(ip net.IP, nets ...*net.IPNet) bool {
	for _, n := range nets {
		if n.Contains(ip) {
			return true
		}
	}
	return false
}

// IsDisallowedIP reports whether the address falls in a loopback, link-local,
// private, CGNAT, multicast, reserved, or unspecified range.
func IsDisallowedIP(ip net.IP) bool {
	if ip == nil {
		return true
	}
	if ip.To4() != nil {
		return ipInNets(ip, loopback4, linkLocal4, privateA, privateB, privateC, cgNAT, multicast4, reserved4, unspecified4)
	}
	return ipInNets(ip, loopback6, linkLocal6, uniqueLocal6, multicast6, unspecified6)
}

// ValidateURL verifies that the provided URL is http(s), has a hostname, and
// does not resolve to a disallowed range. A resolution error is fatal: allowing
// it through would let an unresolvable (or attacker-timed) host reach the dial
// path, where it could re-resolve to a disallowed address.
func ValidateURL(raw string) error {
	u, err := url.Parse(raw)
	if err != nil {
		return errors.New("invalid url: parse failed")
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return errors.New("invalid url: scheme must be http or https")
	}
	host := u.Hostname()
	if host == "" {
		return errors.New("invalid url: missing host")
	}
	if AllowLoopback {
		return nil
	}
	if _, blocked := blockedHostnames[strings.ToLower(host)]; blocked {
		return errors.New("invalid url: host not allowed")
	}
	if ip := net.ParseIP(host); ip != nil {
		if IsDisallowedIP(ip) {
			return errors.New("invalid url: destination address not allowed")
		}
		return nil
	}
	ips, err := net.LookupIP(host)
	if err != nil {
		return errors.New("invalid url: host resolution failed")
	}
	if len(ips) == 0 {
		return errors.New("invalid url: host did not resolve to any address")
	}
	for _, ip := range ips {
		if IsDisallowedIP(ip) {
			return errors.New("invalid url: destination resolves to a disallowed address")
		}
	}
	return nil
}

// NewTransport returns an http.Transport whose DialContext resolves the host
// once, rejects the dial if any candidate address is disallowed, and pins the
// connection to a validated IP literal so the kernel does not re-resolve the
// hostname (closing the DNS-rebinding TOCTOU window).
func NewTransport() *http.Transport {
	dialer := &net.Dialer{Timeout: 5 * time.Second, KeepAlive: 30 * time.Second}
	return &http.Transport{
		DialContext:           guardedDialContext(dialer),
		MaxIdleConns:          50,
		MaxIdleConnsPerHost:   10,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   5 * time.Second,
		ResponseHeaderTimeout: 30 * time.Second,
	}
}

func guardedDialContext(dialer *net.Dialer) func(ctx context.Context, network, address string) (net.Conn, error) {
	return func(ctx context.Context, network, address string) (net.Conn, error) {
		host, port, err := net.SplitHostPort(address)
		if err != nil {
			return nil, err
		}
		if ip := net.ParseIP(host); ip != nil {
			if !AllowLoopback && IsDisallowedIP(ip) {
				return nil, errors.New("netguard: dial to disallowed address blocked")
			}
			return dialer.DialContext(ctx, network, address)
		}
		ips, err := net.DefaultResolver.LookupIP(ctx, ipNetworkFor(network), host)
		if err != nil {
			return nil, err
		}
		if len(ips) == 0 {
			return nil, errors.New("netguard: host did not resolve to any address")
		}
		var lastErr error
		for _, ip := range ips {
			if !AllowLoopback && IsDisallowedIP(ip) {
				return nil, errors.New("netguard: dial to disallowed address blocked")
			}
			conn, err := dialer.DialContext(ctx, network, net.JoinHostPort(ip.String(), port))
			if err != nil {
				lastErr = err
				continue
			}
			return conn, nil
		}
		if lastErr == nil {
			lastErr = errors.New("netguard: no dialable address")
		}
		return nil, lastErr
	}
}

func ipNetworkFor(network string) string {
	switch network {
	case "tcp4", "udp4":
		return "ip4"
	case "tcp6", "udp6":
		return "ip6"
	default:
		return "ip"
	}
}
