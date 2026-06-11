package middleware_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/usehivy/hivy/internal/middleware"
)

func captureRemoteAddr(t *testing.T, trusted []string, peer string, headers map[string]string) string {
	t.Helper()
	var got string
	h := middleware.RealIP(trusted)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = r.RemoteAddr
	}))
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = peer
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	h.ServeHTTP(httptest.NewRecorder(), req)
	return got
}

func TestRealIP_UntrustedPeerIgnoresSpoofedHeaders(t *testing.T) {
	trusted := []string{"127.0.0.0/8"}
	got := captureRemoteAddr(t, trusted, "203.0.113.9:54321", map[string]string{
		"True-Client-IP":  "10.0.0.1",
		"X-Real-IP":       "10.0.0.2",
		"X-Forwarded-For": "10.0.0.3",
	})
	if got != "203.0.113.9:54321" {
		t.Fatalf("untrusted peer must keep its socket address, got %q", got)
	}
}

func TestRealIP_TrustedPeerHonoursTrueClientIP(t *testing.T) {
	trusted := []string{"127.0.0.0/8"}
	got := captureRemoteAddr(t, trusted, "127.0.0.1:443", map[string]string{
		"True-Client-IP": "198.51.100.7",
	})
	if got != "198.51.100.7" {
		t.Fatalf("trusted peer must honour True-Client-IP, got %q", got)
	}
}

func TestRealIP_TrustedPeerHonoursXRealIP(t *testing.T) {
	trusted := []string{"127.0.0.0/8"}
	got := captureRemoteAddr(t, trusted, "127.0.0.1:443", map[string]string{
		"X-Real-IP": "198.51.100.8",
	})
	if got != "198.51.100.8" {
		t.Fatalf("trusted peer must honour X-Real-IP, got %q", got)
	}
}

func TestRealIP_TrustedPeerUsesFirstXForwardedForHop(t *testing.T) {
	trusted := []string{"127.0.0.0/8"}
	got := captureRemoteAddr(t, trusted, "127.0.0.1:443", map[string]string{
		"X-Forwarded-For": "198.51.100.9, 10.0.0.1, 127.0.0.1",
	})
	if got != "198.51.100.9" {
		t.Fatalf("expected left-most X-Forwarded-For hop, got %q", got)
	}
}

func TestRealIP_NoTrustedCIDRsFailsClosed(t *testing.T) {
	// With no parseable trusted ranges, forwarding headers are never honoured,
	// even from loopback.
	got := captureRemoteAddr(t, nil, "127.0.0.1:443", map[string]string{
		"True-Client-IP": "198.51.100.10",
	})
	if got != "127.0.0.1:443" {
		t.Fatalf("with no trusted CIDRs all headers must be ignored, got %q", got)
	}
}

func TestRealIP_TrustedPeerNoHeadersKeepsPeer(t *testing.T) {
	trusted := []string{"127.0.0.0/8"}
	got := captureRemoteAddr(t, trusted, "127.0.0.1:443", nil)
	if got != "127.0.0.1:443" {
		t.Fatalf("expected unchanged peer when no headers present, got %q", got)
	}
}
