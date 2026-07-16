package purchase

import (
	"errors"
	"testing"

	"github.com/usehivy/hivy/internal/billing"
)

func TestCreditsForSubtotalUSD(t *testing.T) {
	svc := &Service{ngnMinorPerUSD: 160_000}
	credits, rate, err := svc.creditsForSubtotal(billing.CurrencyUSD, 1_000)
	if err != nil {
		t.Fatalf("creditsForSubtotal: %v", err)
	}
	if credits != 10_000 {
		t.Fatalf("credits = %d, want 10000", credits)
	}
	if rate != nil {
		t.Fatalf("USD rate snapshot = %v, want nil", *rate)
	}
}

func TestCreditsForSubtotalNGNSnapshotsConfiguredRate(t *testing.T) {
	svc := &Service{ngnMinorPerUSD: 160_000}
	credits, rate, err := svc.creditsForSubtotal(billing.CurrencyNGN, 1_600_000)
	if err != nil {
		t.Fatalf("creditsForSubtotal: %v", err)
	}
	if credits != 10_000 {
		t.Fatalf("credits = %d, want 10000", credits)
	}
	if rate == nil || *rate != 160_000 {
		t.Fatalf("NGN rate snapshot = %v, want 160000", rate)
	}
}

func TestCreditsForSubtotalRejectsAmountsTooSmallForOneCredit(t *testing.T) {
	svc := &Service{ngnMinorPerUSD: 160_000}
	_, _, err := svc.creditsForSubtotal(billing.CurrencyNGN, 1)
	if !errors.Is(err, ErrInvalidAmount) {
		t.Fatalf("error = %v, want ErrInvalidAmount", err)
	}
}

func TestPercentageCeilAddsTenPercent(t *testing.T) {
	fee, err := percentageCeil(1_001, DepositFeeBasisPoints)
	if err != nil {
		t.Fatalf("percentageCeil: %v", err)
	}
	if fee != 101 {
		t.Fatalf("fee = %d, want 101", fee)
	}
}

func TestPercentageCeilRejectsOverflow(t *testing.T) {
	_, err := percentageCeil(maxInt64, DepositFeeBasisPoints)
	if !errors.Is(err, ErrInvalidAmount) {
		t.Fatalf("error = %v, want ErrInvalidAmount", err)
	}
}

const maxInt64 = int64(^uint64(0) >> 1)
