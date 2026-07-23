package purchase

import "github.com/usehivy/hivy/internal/billing"

const CustomPackID = "custom"

type Pack struct {
	ID            string
	Currency      billing.Currency
	SubtotalMinor int64
}

type PackQuote struct {
	ID             string
	Currency       billing.Currency
	SubtotalMinor  int64
	FeeBasisPoints int64
	FeeMinor       int64
	TotalMinor     int64
	Credits        int64
}

var creditPacks = []Pack{
	{ID: "usd_5", Currency: billing.CurrencyUSD, SubtotalMinor: 500},
	{ID: "usd_10", Currency: billing.CurrencyUSD, SubtotalMinor: 1_000},
	{ID: "usd_25", Currency: billing.CurrencyUSD, SubtotalMinor: 2_500},
	{ID: "usd_50", Currency: billing.CurrencyUSD, SubtotalMinor: 5_000},
	{ID: "usd_100", Currency: billing.CurrencyUSD, SubtotalMinor: 10_000},
	{ID: "ngn_5000", Currency: billing.CurrencyNGN, SubtotalMinor: 500_000},
	{ID: "ngn_7250", Currency: billing.CurrencyNGN, SubtotalMinor: 5 * NGNMinorPerUSD},
	{ID: "ngn_10000", Currency: billing.CurrencyNGN, SubtotalMinor: 1_000_000},
	{ID: "ngn_25000", Currency: billing.CurrencyNGN, SubtotalMinor: 2_500_000},
	{ID: "ngn_50000", Currency: billing.CurrencyNGN, SubtotalMinor: 5_000_000},
}

func findPack(id string, currency billing.Currency) (Pack, bool) {
	for _, pack := range creditPacks {
		if pack.ID == id && pack.Currency == currency {
			return pack, true
		}
	}
	return Pack{}, false
}

func resolvePurchaseAmount(packID string, customSubtotalMinor *int64, currency billing.Currency) (string, int64, error) {
	if customSubtotalMinor != nil {
		if packID != "" || *customSubtotalMinor <= 0 {
			return "", 0, ErrInvalidAmount
		}
		return CustomPackID, *customSubtotalMinor, nil
	}
	pack, ok := findPack(packID, currency)
	if !ok {
		return "", 0, ErrInvalidPack
	}
	return pack.ID, pack.SubtotalMinor, nil
}

func (s *Service) Packs() []PackQuote {
	quotes := make([]PackQuote, 0, len(creditPacks))
	for _, pack := range creditPacks {
		credits, _, err := s.creditsForSubtotal(pack.Currency, pack.SubtotalMinor)
		if err != nil {
			continue
		}
		fee, err := percentageCeil(pack.SubtotalMinor, DepositFeeBasisPoints)
		if err != nil {
			continue
		}
		quotes = append(quotes, PackQuote{
			ID:             pack.ID,
			Currency:       pack.Currency,
			SubtotalMinor:  pack.SubtotalMinor,
			FeeBasisPoints: DepositFeeBasisPoints,
			FeeMinor:       fee,
			TotalMinor:     pack.SubtotalMinor + fee,
			Credits:        credits,
		})
	}
	return quotes
}
