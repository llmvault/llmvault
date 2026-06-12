package handler

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"testing"
)

// verifyNangoSignature must fail closed on an empty webhook secret: otherwise an
// attacker forges a valid X-Nango-Hmac-Sha256 by HMAC-ing with an empty key,
// enabling forged inbound events for arbitrary orgs.
func TestVerifyNangoSignatureFailsClosedWithoutSecret(t *testing.T) {
	body := []byte(`{"type":"forward","connectionId":"victim-org-conn"}`)

	// An empty-key HMAC the attacker can compute must be rejected.
	forged := func(b []byte) string {
		mac := hmac.New(sha256.New, []byte(""))
		mac.Write(b)
		return hex.EncodeToString(mac.Sum(nil))
	}(body)

	if verifyNangoSignature(body, "", forged) {
		t.Fatal("verifyNangoSignature returned true with empty secret (fail-open forgery)")
	}
	if verifyNangoSignature(body, "", "") {
		t.Fatal("verifyNangoSignature returned true with empty secret and empty signature")
	}
}

// Sanity check that with a real secret a valid signature still verifies and an
// invalid one is rejected (guards against an over-broad fail-closed change).
func TestVerifyNangoSignatureWithSecret(t *testing.T) {
	const secret = "nango-webhook-secret" // #nosec G101 -- test fixture, not a real secret
	body := []byte(`{"type":"forward","connectionId":"conn-123"}`)

	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	valid := hex.EncodeToString(mac.Sum(nil))

	if !verifyNangoSignature(body, secret, valid) {
		t.Fatal("expected valid signature to verify")
	}
	if verifyNangoSignature(body, secret, "deadbeef") {
		t.Fatal("expected invalid signature to be rejected")
	}
	// A signature computed with the wrong (empty) key must not verify against a
	// real configured secret.
	emptyKeySig := func(b []byte) string {
		m := hmac.New(sha256.New, []byte(""))
		m.Write(b)
		return hex.EncodeToString(m.Sum(nil))
	}(body)
	if verifyNangoSignature(body, secret, emptyKeySig) {
		t.Fatal("empty-key signature must not verify against a real secret")
	}
}
