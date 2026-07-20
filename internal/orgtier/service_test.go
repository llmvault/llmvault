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
