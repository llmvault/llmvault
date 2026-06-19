package handler

import "testing"

func TestDefaultCredentialAuthScheme_ElevenLabs(t *testing.T) {
	if got := defaultCredentialAuthScheme("elevenlabs"); got != "xi-api-key" {
		t.Fatalf("defaultCredentialAuthScheme(elevenlabs) = %q, want xi-api-key", got)
	}
	if !validCredentialAuthScheme("xi-api-key") {
		t.Fatal("xi-api-key should be a valid credential auth scheme")
	}
}
