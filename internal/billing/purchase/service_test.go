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

func TestPercentageCeilAddsTwelvePercent(t *testing.T) {
	fee, err := percentageCeil(1_001, DepositFeeBasisPoints)
	if err != nil {
		t.Fatalf("percentageCeil: %v", err)
	}
	if fee != 121 {
		t.Fatalf("fee = %d, want 121", fee)
	}
}

func TestPercentageCeilRejectsOverflow(t *testing.T) {
	_, err := percentageCeil(maxInt64, DepositFeeBasisPoints)
	if !errors.Is(err, ErrInvalidAmount) {
		t.Fatalf("error = %v, want ErrInvalidAmount", err)
	}
}

func TestCreditPacksEnforceCurrencyMinimumsAndFee(t *testing.T) {
	svc := &Service{ngnMinorPerUSD: 160_000}
	usd := svc.Packs(billing.CurrencyUSD)
	ngn := svc.Packs(billing.CurrencyNGN)
	if len(usd) == 0 || usd[0].SubtotalMinor != 1_000 || usd[0].TotalMinor != 1_120 {
		t.Fatalf("first USD pack = %#v", usd)
	}
	if len(ngn) == 0 || ngn[0].SubtotalMinor != 500_000 || ngn[0].TotalMinor != 560_000 {
		t.Fatalf("first NGN pack = %#v", ngn)
	}
	if _, ok := findPack("usd_10", billing.CurrencyNGN); ok {
		t.Fatal("USD pack must not be purchasable from an NGN account")
	}
}

const maxInt64 = int64(^uint64(0) >> 1)
