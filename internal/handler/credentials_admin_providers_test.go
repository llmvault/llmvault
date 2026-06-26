package handler

import (
	"context"
	"encoding/base64"
	"testing"

	"github.com/usehivy/hivy/internal/crypto"
)

func TestDefaultCredentialAuthScheme_ElevenLabs(t *testing.T) {
	if got := defaultCredentialAuthScheme("elevenlabs"); got != "xi-api-key" {
		t.Fatalf("defaultCredentialAuthScheme(elevenlabs) = %q, want xi-api-key", got)
	}
	if !validCredentialAuthScheme("xi-api-key") {
		t.Fatal("xi-api-key should be a valid credential auth scheme")
	}
}

func TestBuildSystemCredentialDefaultsReveProvider(t *testing.T) {
	h := NewCredentialHandler(nil, newCredentialProviderTestKMS(t), nil, nil)

	cred, err := h.buildCredential(context.Background(), nil, createCredentialRequest{
		Label:      "system-reve",
		ProviderID: "reve",
		APIKey:     "reve-test-key",
	})
	if err != nil {
		t.Fatalf("buildCredential returned error: %v", err)
	}
	if cred.ProviderID != "reve" {
		t.Fatalf("ProviderID = %q, want reve", cred.ProviderID)
	}
	if cred.BaseURL != "https://api.reve.com" {
		t.Fatalf("BaseURL = %q, want https://api.reve.com", cred.BaseURL)
	}
	if cred.AuthScheme != "bearer" {
		t.Fatalf("AuthScheme = %q, want bearer", cred.AuthScheme)
	}
	if len(cred.EncryptedKey) == 0 || len(cred.WrappedDEK) == 0 {
		t.Fatal("credential key material was not encrypted")
	}
}

func newCredentialProviderTestKMS(t *testing.T) *crypto.KeyWrapper {
	t.Helper()
	key := make([]byte, 32)
	b64 := base64.StdEncoding.EncodeToString(key)
	kms, err := crypto.NewAEADWrapper(context.Background(), b64, "credential-provider-test")
	if err != nil {
		t.Fatalf("new test KMS: %v", err)
	}
	return kms
}
