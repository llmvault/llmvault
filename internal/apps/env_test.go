package apps

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"regexp"
	"strings"
	"sync"
	"testing"

	"github.com/usehivy/hivy/internal/config"
)

var (
	testRSAKeyOnce sync.Once
	testRSAKeyVal  *rsa.PrivateKey
)

func testRSAKey(t *testing.T) *rsa.PrivateKey {
	t.Helper()
	testRSAKeyOnce.Do(func() {
		key, err := rsa.GenerateKey(rand.Reader, 2048)
		if err != nil {
			t.Fatalf("generate rsa key: %v", err)
		}
		testRSAKeyVal = key
	})
	return testRSAKeyVal
}

func TestDeriveSessionSecretDeterministic(t *testing.T) {
	first, err := DeriveSessionSecret("hvapp_deadbeef")
	if err != nil {
		t.Fatalf("derive: %v", err)
	}
	second, err := DeriveSessionSecret("hvapp_deadbeef")
	if err != nil {
		t.Fatalf("derive: %v", err)
	}
	if first != second {
		t.Fatalf("session secret is not deterministic: %q vs %q", first, second)
	}
	if !regexp.MustCompile(`^[a-f0-9]{64}$`).MatchString(first) {
		t.Fatalf("session secret is not 64 hex chars: %q", first)
	}
	other, err := DeriveSessionSecret("hvapp_other")
	if err != nil {
		t.Fatalf("derive: %v", err)
	}
	if other == first {
		t.Fatal("different app secrets derived the same session secret")
	}
}

// TestEncodeAuthPublicKeyPEM proves the escaped one-line PEM survives the
// exact un-escape + parse the template's hivycore performs and is legal in
// an appd env file (no raw newlines).
func TestEncodeAuthPublicKeyPEM(t *testing.T) {
	key := testRSAKey(t)
	encoded, err := EncodeAuthPublicKeyPEM(&key.PublicKey)
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	if strings.ContainsAny(encoded, "\n\r") {
		t.Fatalf("encoded PEM contains raw newlines (appd env files reject them): %q", encoded)
	}
	// hivycore.parseRSAPublicKey: un-escape then PEM-decode PKIX.
	unescaped := strings.ReplaceAll(encoded, `\n`, "\n")
	block, _ := pem.Decode([]byte(unescaped))
	if block == nil {
		t.Fatal("un-escaped value is not PEM")
	}
	parsed, err := x509.ParsePKIXPublicKey(block.Bytes)
	if err != nil {
		t.Fatalf("parse PKIX: %v", err)
	}
	pub, ok := parsed.(*rsa.PublicKey)
	if !ok {
		t.Fatal("parsed key is not RSA")
	}
	if pub.N.Cmp(key.PublicKey.N) != 0 {
		t.Fatal("round-tripped public key does not match")
	}
}

func TestAppImageRef(t *testing.T) {
	if got, want := AppImageRef(nil), "ghcr.io/usehivy/hivy-app:latest"; got != want {
		t.Fatalf("AppImageRef(nil) = %q, want %q", got, want)
	}
	if got, want := AppImageRef(&config.Config{}), "ghcr.io/usehivy/hivy-app:latest"; got != want {
		t.Fatalf("AppImageRef(empty) = %q, want %q", got, want)
	}
	cfg := &config.Config{SandboxesAppImageTag: "v1.2.3-amd64"}
	if got, want := AppImageRef(cfg), "ghcr.io/usehivy/hivy-app:v1.2.3-amd64"; got != want {
		t.Fatalf("AppImageRef(tagged) = %q, want %q", got, want)
	}
}
