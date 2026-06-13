package security

import (
	"strings"
	"testing"
)

func TestGeneratePreviewPasswordShape(t *testing.T) {
	password := GeneratePreviewPassword()
	parts := strings.Split(password, "-")
	if len(parts) != 3 {
		t.Fatalf("password %q has %d parts, want 3", password, len(parts))
	}
	for _, part := range parts {
		if part == "" || strings.ToLower(part) != part {
			t.Fatalf("password part %q must be lowercase non-empty", part)
		}
	}
}

func TestEncryptStringRoundTrip(t *testing.T) {
	encrypted, err := EncryptString("test-key", "amber-linen-river")
	if err != nil {
		t.Fatal(err)
	}
	if string(encrypted) == "amber-linen-river" {
		t.Fatal("ciphertext must not contain plaintext")
	}
	decrypted, err := DecryptString("test-key", encrypted)
	if err != nil {
		t.Fatal(err)
	}
	if decrypted != "amber-linen-river" {
		t.Fatalf("decrypted = %q", decrypted)
	}
}

func TestDecryptStringRejectsWrongKey(t *testing.T) {
	encrypted, err := EncryptString("test-key", "amber-linen-river")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := DecryptString("wrong-key", encrypted); err == nil {
		t.Fatal("expected wrong key to fail")
	}
}
