package cache

import (
	"testing"
	"time"
)

// TestRevokedSet_TTLBounded verifies the in-memory revoked set evicts entries
// once their TTL lapses, instead of growing for the process lifetime (P2-38).
func TestRevokedSet_TTLBounded(t *testing.T) {
	inv := NewInvalidator(nil, nil, nil, nil)

	// A live entry is reported revoked.
	inv.markRevoked("live", time.Hour)
	if !inv.IsTokenLocallyRevoked("live") {
		t.Fatal("freshly revoked JTI should be reported revoked")
	}

	// An already-lapsed entry is treated as absent and lazily evicted on read.
	inv.markRevoked("stale", -time.Second) // non-positive ttl is capped, so set directly
	inv.revokedMu.Lock()
	inv.revokedSet["stale"] = time.Now().Add(-time.Minute)
	inv.revokedMu.Unlock()

	if inv.IsTokenLocallyRevoked("stale") {
		t.Fatal("expired revocation entry should not be reported revoked")
	}
	inv.revokedMu.RLock()
	_, present := inv.revokedSet["stale"]
	inv.revokedMu.RUnlock()
	if present {
		t.Fatal("expired entry should be lazily evicted on read")
	}
}

// TestRevokedSet_Sweep verifies the opportunistic sweep drops lapsed entries
// while retaining live ones.
func TestRevokedSet_Sweep(t *testing.T) {
	inv := NewInvalidator(nil, nil, nil, nil)

	inv.markRevoked("keep", time.Hour)
	inv.revokedMu.Lock()
	inv.revokedSet["drop1"] = time.Now().Add(-time.Minute)
	inv.revokedSet["drop2"] = time.Now().Add(-time.Hour)
	inv.revokedMu.Unlock()

	inv.sweepRevoked()

	inv.revokedMu.RLock()
	defer inv.revokedMu.RUnlock()
	if _, ok := inv.revokedSet["keep"]; !ok {
		t.Fatal("sweep evicted a live entry")
	}
	if len(inv.revokedSet) != 1 {
		t.Fatalf("sweep left %d entries, want 1 (lapsed entries not dropped)", len(inv.revokedSet))
	}
}

// TestRevokedSet_TTLCapped verifies a TTL longer than the ceiling is capped.
func TestRevokedSet_TTLCapped(t *testing.T) {
	inv := NewInvalidator(nil, nil, nil, nil)
	before := time.Now().Add(revokedEntryMaxTTL)
	inv.markRevoked("jti", 100*time.Hour)
	after := time.Now().Add(revokedEntryMaxTTL)

	inv.revokedMu.RLock()
	expiry := inv.revokedSet["jti"]
	inv.revokedMu.RUnlock()
	if expiry.Before(before) || expiry.After(after.Add(time.Second)) {
		t.Fatalf("expiry %v not capped to ~revokedEntryMaxTTL window [%v,%v]", expiry, before, after)
	}
}
