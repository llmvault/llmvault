package purchase

import (
	"errors"
	"testing"

	"github.com/usehivy/hivy/internal/billing"
)

func TestCreditsForSubtotalUSD(t *testing.T) {
	svc := &Service{}
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

func TestCreditsForSubtotalNGNSnapshotsFixedRate(t *testing.T) {
	svc := &Service{}
	credits, rate, err := svc.creditsForSubtotal(billing.CurrencyNGN, 1_450_000)
	if err != nil {
		t.Fatalf("creditsForSubtotal: %v", err)
	}
	if credits != 10_000 {
		t.Fatalf("credits = %d, want 10000", credits)
	}
	if rate == nil || *rate != NGNMinorPerUSD {
		t.Fatalf("NGN rate snapshot = %v, want %d", rate, NGNMinorPerUSD)
	}
}

func TestCreditsForSubtotalRejectsAmountsTooSmallForOneCredit(t *testing.T) {
	svc := &Service{}
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
	svc := &Service{}
	packs := svc.Packs()
	if len(packs) != 10 {
		t.Fatalf("packs = %d, want 10", len(packs))
	}
	usdFive, ok := findPack("usd_5", billing.CurrencyUSD)
	if !ok || usdFive.SubtotalMinor != 500 {
		t.Fatalf("USD $5 pack = %#v, found = %v", usdFive, ok)
	}
	ngnEquivalent, ok := findPack("ngn_7250", billing.CurrencyNGN)
	if !ok || ngnEquivalent.SubtotalMinor != 725_000 {
		t.Fatalf("NGN equivalent pack = %#v, found = %v", ngnEquivalent, ok)
	}
	usdCredits, _, err := svc.creditsForSubtotal(usdFive.Currency, usdFive.SubtotalMinor)
	if err != nil {
		t.Fatalf("USD pack credits: %v", err)
	}
	ngnCredits, _, err := svc.creditsForSubtotal(ngnEquivalent.Currency, ngnEquivalent.SubtotalMinor)
	if err != nil {
		t.Fatalf("NGN pack credits: %v", err)
	}
	if usdCredits != 5_000 || ngnCredits != usdCredits {
		t.Fatalf("equivalent pack credits: USD=%d NGN=%d", usdCredits, ngnCredits)
	}
	var usdQuote, ngnQuote PackQuote
	for _, quote := range packs {
		switch quote.ID {
		case "usd_5":
			usdQuote = quote
		case "ngn_7250":
			ngnQuote = quote
		}
	}
	if usdQuote.FeeMinor != 60 || usdQuote.TotalMinor != 560 {
		t.Fatalf("USD $5 quote = %#v", usdQuote)
	}
	if ngnQuote.FeeMinor != 87_000 || ngnQuote.TotalMinor != 812_000 {
		t.Fatalf("NGN ₦7,250 quote = %#v", ngnQuote)
	}
	if _, ok := findPack("usd_10", billing.CurrencyNGN); ok {
		t.Fatal("USD pack must not be purchasable from an NGN account")
	}
}

func TestResolvePurchaseAmountAcceptsCustomSubtotal(t *testing.T) {
	customSubtotal := int64(1_234)
	packID, subtotal, err := resolvePurchaseAmount("", &customSubtotal, billing.CurrencyUSD)
	if err != nil {
		t.Fatalf("resolve custom amount: %v", err)
	}
	if packID != CustomPackID || subtotal != customSubtotal {
		t.Fatalf("custom purchase = (%q, %d), want (%q, %d)", packID, subtotal, CustomPackID, customSubtotal)
	}
}

func TestResolvePurchaseAmountRequiresPackOrCustomAmount(t *testing.T) {
	customSubtotal := int64(500)
	if _, _, err := resolvePurchaseAmount("usd_5", &customSubtotal, billing.CurrencyUSD); !errors.Is(err, ErrInvalidAmount) {
		t.Fatalf("pack plus custom amount error = %v, want ErrInvalidAmount", err)
	}
	if _, _, err := resolvePurchaseAmount("", nil, billing.CurrencyUSD); !errors.Is(err, ErrInvalidPack) {
		t.Fatalf("missing amount error = %v, want ErrInvalidPack", err)
	}
}

func TestCheckoutChannelsAreCurrencyAware(t *testing.T) {
	usdChannels := checkoutChannels(billing.CurrencyUSD)
	if len(usdChannels) != 1 || usdChannels[0] != "card" {
		t.Fatalf("USD channels = %#v, want card only", usdChannels)
	}
	if ngnChannels := checkoutChannels(billing.CurrencyNGN); len(ngnChannels) != 0 {
		t.Fatalf("NGN channels = %#v, want Paystack defaults", ngnChannels)
	}
}

const maxInt64 = int64(^uint64(0) >> 1)
