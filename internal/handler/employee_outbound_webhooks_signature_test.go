package handler

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"testing"

	"github.com/google/uuid"

	"github.com/usehivy/hivy/internal/model"
)

// P1-2: HMAC verification must fail closed when no encryption key is configured.
// Returning true there would let anonymous callers forge session events / usage
// for any sandbox ID.
func TestVerifySignatureFailsClosedWithoutEncKey(t *testing.T) {
	h := NewEmployeeOutboundWebhookHandler(nil, nil, nil)
	sb := &model.Sandbox{ID: uuid.New()}
	body := []byte(`{"event_type":"agent.run.model.usage"}`)

	// Even a "correctly shaped" signature (any value) must be rejected because
	// the server has no key with which to verify the HMAC.
	if h.verifySignature(context.Background(), sb, body, "sha256=deadbeef") {
		t.Fatal("verifySignature returned true with nil encryption key (fail-open SSRF/forgery)")
	}
	if h.verifySignature(context.Background(), sb, body, "") {
		t.Fatal("verifySignature returned true with nil encryption key and empty signature")
	}
}

// Sanity check that with a real key, a valid signature still verifies and an
// invalid one is rejected (guards against an over-broad fail-closed change).
func TestVerifySignatureWithEncKey(t *testing.T) {
	key := outboundWebhookTestSymmetricKey(t)
	const secret = "runtime-secret-value"
	encrypted, err := key.EncryptString(secret)
	if err != nil {
		t.Fatalf("encrypt secret: %v", err)
	}

	h := NewEmployeeOutboundWebhookHandler(nil, key, nil)
	sb := &model.Sandbox{ID: uuid.New(), EncryptedRuntimeSecret: encrypted}
	body := []byte(`{"event_type":"agent.message.sent"}`)

	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	valid := hex.EncodeToString(mac.Sum(nil))

	if !h.verifySignature(context.Background(), sb, body, "sha256="+valid) {
		t.Fatal("expected valid signature to verify")
	}
	if h.verifySignature(context.Background(), sb, body, "sha256=00") {
		t.Fatal("expected invalid signature to be rejected")
	}
}
