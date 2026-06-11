package proxy

import (
	"context"
	"errors"
	"net"
	"net/http"
	"time"
)

// NewTransport creates an http.Transport optimized for proxying to LLM providers.
//
// The DialContext re-checks every resolved address against the disallowed-IP
// list and pins the connection to a validated IP. This closes the DNS-rebinding
// TOCTOU window between ValidateBaseURL (which resolves the host once) and the
// actual dial (which would otherwise re-resolve independently): a short-TTL
// attacker domain that passed validation pointing at a public IP cannot be
// re-resolved to 169.254.169.254/10.x/127.0.0.1 at connection time.
//
// AllowLoopback (set in tests) bypasses the IP checks but keeps the dial logic.
func NewTransport() *http.Transport {
	dialer := &net.Dialer{
		Timeout:   5 * time.Second,
		KeepAlive: 30 * time.Second,
	}
	return &http.Transport{
		DialContext:           guardedDialContext(dialer),
		MaxIdleConns:          200,
		MaxIdleConnsPerHost:   50,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   5 * time.Second,
		ResponseHeaderTimeout: 120 * time.Second,
		DisableCompression:    true,
	}
}

// guardedDialContext returns a DialContext that resolves the host once, rejects
// the dial if any candidate address is disallowed, and connects to a validated
// IP literal so the kernel does not re-resolve the hostname.
func guardedDialContext(dialer *net.Dialer) func(ctx context.Context, network, address string) (net.Conn, error) {
	return func(ctx context.Context, network, address string) (net.Conn, error) {
		host, port, err := net.SplitHostPort(address)
		if err != nil {
			return nil, err
		}

		// IP literal: validate and dial directly.
		if ip := net.ParseIP(host); ip != nil {
			if !AllowLoopback && isDisallowedIP(ip) {
				return nil, errors.New("proxy: dial to disallowed address blocked")
			}
			return dialer.DialContext(ctx, network, address)
		}

		// Resolve the host once and validate every candidate before connecting.
		ips, err := net.DefaultResolver.LookupIP(ctx, ipNetworkFor(network), host)
		if err != nil {
			return nil, err
		}
		if len(ips) == 0 {
			return nil, errors.New("proxy: host did not resolve to any address")
		}

		var lastErr error
		for _, ip := range ips {
			if !AllowLoopback && isDisallowedIP(ip) {
				return nil, errors.New("proxy: dial to disallowed address blocked")
			}
			// Pin the connection to the validated IP literal so there is no
			// second, unvalidated resolution.
			conn, err := dialer.DialContext(ctx, network, net.JoinHostPort(ip.String(), port))
			if err != nil {
				lastErr = err
				continue
			}
			return conn, nil
		}
		if lastErr == nil {
			lastErr = errors.New("proxy: no dialable address")
		}
		return nil, lastErr
	}
}

// ipNetworkFor maps a dial network to the address family LookupIP should resolve.
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
