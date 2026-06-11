package proxy

import (
	"context"
	"net"
	"strings"
	"testing"
)

func TestValidateBaseURL_RejectsResolutionFailure(t *testing.T) {
	old := AllowLoopback
	AllowLoopback = false
	defer func() { AllowLoopback = old }()

	// A nonexistent host (.invalid, RFC 2606) must fail validation, not pass.
	err := ValidateBaseURL("https://this-host-does-not-exist.invalid")
	if err == nil {
		t.Fatal("expected resolution failure to be rejected, got nil")
	}
	if !strings.Contains(err.Error(), "resolution failed") {
		t.Fatalf("expected resolution failure error, got %v", err)
	}
}

func TestValidateBaseURL_RejectsDisallowedIPLiteral(t *testing.T) {
	old := AllowLoopback
	AllowLoopback = false
	defer func() { AllowLoopback = old }()

	for _, raw := range []string{
		"http://169.254.169.254/latest/meta-data/",
		"http://127.0.0.1:8080",
		"http://10.1.2.3",
		"https://[::1]",
	} {
		if err := ValidateBaseURL(raw); err == nil {
			t.Fatalf("expected %q to be rejected", raw)
		}
	}
}

func TestGuardedDialContext_BlocksDisallowedIPLiteral(t *testing.T) {
	old := AllowLoopback
	AllowLoopback = false
	defer func() { AllowLoopback = old }()

	dial := guardedDialContext(&net.Dialer{})
	_, err := dial(context.Background(), "tcp", "169.254.169.254:80")
	if err == nil {
		t.Fatal("expected dial to disallowed IP literal to be blocked")
	}
	if !strings.Contains(err.Error(), "disallowed") {
		t.Fatalf("expected disallowed-address error, got %v", err)
	}
}

// fakeResolver lets us simulate DNS rebinding: validation-time resolution
// returns a public IP, but the dial-time resolution returns a metadata IP.
func TestGuardedDialContext_BlocksRebindingToMetadataIP(t *testing.T) {
	old := AllowLoopback
	AllowLoopback = false
	defer func() { AllowLoopback = old }()

	// A hostname that resolves to a disallowed address at dial time must be
	// blocked before any connection is attempted.
	if !isDisallowedIP(net.ParseIP("169.254.169.254")) {
		t.Fatal("expected 169.254.169.254 to be disallowed")
	}

	// The dialer must refuse an IP literal in the disallowed range even when
	// the port would otherwise be dialable.
	dial := guardedDialContext(&net.Dialer{})
	if _, err := dial(context.Background(), "tcp", "10.0.0.5:443"); err == nil {
		t.Fatal("expected private-range dial to be blocked")
	}
}

func TestGuardedDialContext_AllowsLoopbackInTestMode(t *testing.T) {
	old := AllowLoopback
	AllowLoopback = true
	defer func() { AllowLoopback = old }()

	// With AllowLoopback set (test mode) the guarded dialer can reach loopback.
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer ln.Close()

	dial := guardedDialContext(&net.Dialer{})
	conn, err := dial(context.Background(), "tcp", ln.Addr().String())
	if err != nil {
		t.Fatalf("expected loopback dial to succeed in test mode, got %v", err)
	}
	_ = conn.Close()
}
