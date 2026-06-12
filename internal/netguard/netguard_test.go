package netguard

import "testing"

func TestValidateURLRejectsInternalAndMetadata(t *testing.T) {
	prev := AllowLoopback
	AllowLoopback = false
	t.Cleanup(func() { AllowLoopback = prev })

	for _, raw := range []string{
		"http://169.254.169.254/latest/meta-data/",
		"http://127.0.0.1/internal",
		"http://localhost/admin",
		"http://10.0.0.5/secret",
		"http://192.168.1.1/router",
		"http://172.16.0.1/x",
		"http://metadata.google.internal/",
		"http://[::1]/",
		"ftp://example.com/",   // wrong scheme
		"not a url at all ://", // unparseable
	} {
		if err := ValidateURL(raw); err == nil {
			t.Fatalf("expected %q to be rejected", raw)
		}
	}
}

func TestValidateURLAllowsPublicHost(t *testing.T) {
	prev := AllowLoopback
	AllowLoopback = false
	t.Cleanup(func() { AllowLoopback = prev })

	if err := ValidateURL("https://93.184.216.34/path"); err != nil {
		t.Fatalf("expected public IP literal to be allowed, got %v", err)
	}
}

func TestValidateURLAllowLoopbackBypassesIPChecks(t *testing.T) {
	prev := AllowLoopback
	AllowLoopback = true
	t.Cleanup(func() { AllowLoopback = prev })

	if err := ValidateURL("http://127.0.0.1:8080/ok"); err != nil {
		t.Fatalf("AllowLoopback should permit loopback, got %v", err)
	}
	if err := ValidateURL("gopher://127.0.0.1/"); err == nil {
		t.Fatal("non-http scheme must be rejected even with AllowLoopback")
	}
}
