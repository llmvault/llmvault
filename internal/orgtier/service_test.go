package orgtier

import (
	"errors"
	"testing"
)

func TestTierForLifetimeCreditsUsesPermanentDepositThresholds(t *testing.T) {
	tests := []struct {
		credits int64
		want    int
	}{
		{credits: 0, want: Tier1},
		{credits: 99_999, want: Tier1},
		{credits: 100_000, want: Tier2},
		{credits: 250_000, want: Tier3},
		{credits: 500_000, want: Tier4},
	}
	for _, tt := range tests {
		if got := TierForLifetimeCredits(tt.credits); got != tt.want {
			t.Fatalf("TierForLifetimeCredits(%d) = %d, want %d", tt.credits, got, tt.want)
		}
	}
}

func TestLimitsForTierUsesConservativeCapacityProgression(t *testing.T) {
	tests := []struct {
		tier               int
		concurrentSessions int
		knowledgeStorageGB int64
	}{
		{tier: Tier1, concurrentSessions: 1, knowledgeStorageGB: 1},
		{tier: Tier2, concurrentSessions: 2, knowledgeStorageGB: 3},
		{tier: Tier3, concurrentSessions: 5, knowledgeStorageGB: 5},
		{tier: Tier4, concurrentSessions: 10, knowledgeStorageGB: 10},
	}
	for _, tt := range tests {
		limits := LimitsForTier(tt.tier)
		if limits.ConcurrentSessions != tt.concurrentSessions {
			t.Fatalf("tier %d concurrent sessions = %d, want %d", tt.tier, limits.ConcurrentSessions, tt.concurrentSessions)
		}
		if got := limits.KnowledgeStorageBytes >> 30; got != tt.knowledgeStorageGB {
			t.Fatalf("tier %d knowledge storage = %d GB, want %d GB", tt.tier, got, tt.knowledgeStorageGB)
		}
	}
}

func TestSandboxSizeLimitsAndXlargeDisabled(t *testing.T) {
	if err := ValidateSandboxSize(Tier1, "nano"); err != nil {
		t.Fatalf("tier 1 nano: %v", err)
	}
	if err := ValidateSandboxSize(Tier1, "small"); !errors.Is(err, ErrSandboxSizeNotAllowed) {
		t.Fatalf("tier 1 small error = %v", err)
	}
	if err := ValidateSandboxSize(Tier4, "large"); err != nil {
		t.Fatalf("tier 4 large: %v", err)
	}
	if err := ValidateSandboxSize(Tier4, "xlarge"); !errors.Is(err, ErrSandboxSizeNotAllowed) {
		t.Fatalf("tier 4 xlarge error = %v", err)
	}
}
