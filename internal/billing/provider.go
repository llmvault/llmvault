// Package billing contains the provider-agnostic credit ledger and one-time
// deposit contracts.
package billing

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
)

type Currency string

const (
	CurrencyUSD Currency = "USD"
	CurrencyNGN Currency = "NGN"
)

func (c Currency) IsValid() bool { return c == CurrencyUSD || c == CurrencyNGN }

var (
	ErrUnsupportedCurrency = errors.New("billing: currency not supported by this provider")
	ErrOrgMismatch         = errors.New("billing: transaction org does not match expected org")
	ErrPurchaseMismatch    = errors.New("billing: transaction purchase does not match expected purchase")
)

type DepositIntent struct {
	PurchaseID    uuid.UUID
	OrgID         uuid.UUID
	CustomerEmail string
	AmountMinor   int64
	Currency      Currency
	Metadata      map[string]string
	Channels      []string
}

type SavedPaymentCharge struct {
	PurchaseID        uuid.UUID
	OrgID             uuid.UUID
	AuthorizationCode string
	CustomerEmail     string
	AmountMinor       int64
	Currency          Currency
	Metadata          map[string]string
}

type DepositSession struct {
	URL        string
	AccessCode string
	Reference  string
}

type ResolveDepositRequest struct {
	Reference          string
	ExpectedOrgID      uuid.UUID
	ExpectedPurchaseID uuid.UUID
}

type PaymentStatus string

const (
	PaymentPending  PaymentStatus = "pending"
	PaymentPaid     PaymentStatus = "paid"
	PaymentFailed   PaymentStatus = "failed"
	PaymentReversed PaymentStatus = "reversed"
)

type DepositResult struct {
	Status          PaymentStatus
	Reference       string
	PaidAt          *time.Time
	PaidAmountMinor int64
	Currency        Currency
	Metadata        map[string]string
	CustomerEmail   string
	Authorization   *PaymentAuthorization
}

type PaymentAuthorization struct {
	AuthorizationCode string  `json:"authorization_code"`
	Bin               string  `json:"bin"`
	CardType          string  `json:"card_type"`
	Last4             string  `json:"last4"`
	ExpMonth          string  `json:"exp_month"`
	ExpYear           string  `json:"exp_year"`
	Bank              string  `json:"bank"`
	Channel           string  `json:"channel"`
	Signature         string  `json:"signature"`
	Reusable          bool    `json:"reusable"`
	CountryCode       string  `json:"country_code"`
	Brand             string  `json:"brand"`
	AccountName       *string `json:"account_name"`
}

// Provider initializes and verifies one-time credit deposits.
type Provider interface {
	Name() string
	CreateDeposit(ctx context.Context, intent DepositIntent) (*DepositSession, error)
	ChargeSavedPayment(ctx context.Context, charge SavedPaymentCharge) (*DepositSession, error)
	ResolveDeposit(ctx context.Context, req ResolveDepositRequest) (*DepositResult, error)
}
