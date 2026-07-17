package handler

import (
	"crypto/hmac"
	"crypto/sha512"
	"encoding/hex"
	"testing"
)

func TestValidPaystackSignature(t *testing.T) {
	body := []byte(`{"event":"charge.success"}`)
	mac := hmac.New(sha512.New, []byte("sk_test_scrubbed"))
	_, _ = mac.Write(body)
	signature := hex.EncodeToString(mac.Sum(nil))
	if !validPaystackSignature(body, signature, "sk_test_scrubbed") {
		t.Fatal("valid signature rejected")
	}
	if validPaystackSignature([]byte(`{"event":"tampered"}`), signature, "sk_test_scrubbed") {
		t.Fatal("tampered body accepted")
	}
}
