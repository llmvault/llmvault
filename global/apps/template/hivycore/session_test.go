package hivycore

import (
	"encoding/base64"
	"strings"
	"testing"
	"time"
)

func testSession(t *testing.T) Session {
	t.Helper()
	return Session{
		UserID:     "6a4f0e0a-1111-4f4f-8888-aaaaaaaaaaaa",
		UserName:   "Ada Lovelace",
		UserEmail:  "ada@example.com",
		UserAvatar: "https://cdn.example.com/ada.png",
		OrgID:      "7b5f1f1b-2222-4f4f-9999-bbbbbbbbbbbb",
		OrgName:    "Analytical Engines",
		Role:       "admin",
		ExpiresAt:  time.Now().Add(SessionTTL).Unix(),
	}
}

func TestSessionRoundTrip(t *testing.T) {
	codec, err := newSessionCodec("test-secret-value")
	if err != nil {
		t.Fatalf("newSessionCodec: %v", err)
	}
	want := testSession(t)

	value, err := codec.encrypt(want)
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}
	got, err := codec.decrypt(value)
	if err != nil {
		t.Fatalf("decrypt: %v", err)
	}
	if *got != want {
		t.Fatalf("round trip mismatch: got %+v want %+v", *got, want)
	}
}

func TestSessionTamperRejected(t *testing.T) {
	codec, err := newSessionCodec("test-secret-value")
	if err != nil {
		t.Fatalf("newSessionCodec: %v", err)
	}
	value, err := codec.encrypt(testSession(t))
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}

	raw, err := base64.StdEncoding.DecodeString(value)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	raw[len(raw)-1] ^= 0x01 // flip one ciphertext bit
	if _, err := codec.decrypt(base64.StdEncoding.EncodeToString(raw)); err == nil {
		t.Fatal("tampered cookie decrypted successfully")
	}
}

func TestSessionWrongKeyRejected(t *testing.T) {
	codecA, _ := newSessionCodec("secret-a")
	codecB, _ := newSessionCodec("secret-b")
	value, err := codecA.encrypt(testSession(t))
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}
	if _, err := codecB.decrypt(value); err == nil {
		t.Fatal("cookie decrypted under a different secret")
	}
}

func TestSessionExpiredRejected(t *testing.T) {
	codec, _ := newSessionCodec("test-secret-value")
	expired := testSession(t)
	expired.ExpiresAt = time.Now().Add(-time.Minute).Unix()
	value, err := codec.encrypt(expired)
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}
	if _, err := codec.decrypt(value); err == nil || !strings.Contains(err.Error(), "expired") {
		t.Fatalf("expired session accepted (err=%v)", err)
	}
}

func TestSessionGarbageRejected(t *testing.T) {
	codec, _ := newSessionCodec("test-secret-value")
	for _, value := range []string{"", "not-base64!!", base64.StdEncoding.EncodeToString([]byte("short"))} {
		if _, err := codec.decrypt(value); err == nil {
			t.Fatalf("garbage cookie %q decrypted successfully", value)
		}
	}
}

func TestJTIGuard(t *testing.T) {
	guard := newJTIGuard(time.Minute, 3)
	now := time.Now()
	guard.now = func() time.Time { return now }

	if !guard.firstUse("a") {
		t.Fatal("first use of a rejected")
	}
	if guard.firstUse("a") {
		t.Fatal("replay of a accepted")
	}

	// Capacity eviction: b, c, d push a out.
	guard.firstUse("b")
	guard.firstUse("c")
	guard.firstUse("d")
	if guard.firstUse("d") {
		t.Fatal("replay of d accepted after eviction churn")
	}

	// TTL expiry: after the window, d is forgotten (token would be expired
	// by then anyway — the JWT exp check is the real gate).
	now = now.Add(2 * time.Minute)
	if !guard.firstUse("d") {
		t.Fatal("expired jti record still blocking")
	}
}
