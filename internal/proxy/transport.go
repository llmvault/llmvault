package proxy

import (
	"context"
	"errors"
	"net"
	"net/http"
	"time"
)

// NewTransport creates an http.Transport for LLM providers whose DialContext
// re-checks and pins the resolved IP, closing the DNS-rebinding TOCTOU window so
// a short-TTL attacker domain can't re-resolve to a metadata/private IP at dial.
// AllowLoopback (tests) bypasses checks.
// RetryTransport wraps an http.RoundTripper to retry 503 responses with backoff.
type RetryTransport struct {
	Inner       http.RoundTripper
	MaxRetries  int
	InitialWait time.Duration
}

func (rt *RetryTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	maxRetries := rt.MaxRetries
	if maxRetries <= 0 {
		maxRetries = 2
	}
	wait := rt.InitialWait
	if wait <= 0 {
		wait = 500 * time.Millisecond
	}
	var resp *http.Response
	var err error
	for attempt := 0; attempt <= maxRetries; attempt++ {
		resp, err = rt.Inner.RoundTrip(req)
		if err != nil || resp.StatusCode != http.StatusServiceUnavailable {
			return resp, err
		}
		if attempt < maxRetries {
			resp.Body.Close()
			select {
			case <-time.After(wait):
			case <-req.Context().Done():
				return nil, req.Context().Err()
			}
			wait *= 2
		}
	}
	return resp, err
}

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
// disallowed candidates, and connects to the validated IP literal (no re-resolve).
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
			// Pin to the validated IP literal so there is no second resolution.
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
