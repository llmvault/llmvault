package paystack

import (
	"context"
	"fmt"

	"github.com/usehivy/hivy/internal/billing"
)

type initializeRequest struct {
	Email       string            `json:"email"`
	Amount      int64             `json:"amount"`
	Currency    string            `json:"currency,omitempty"`
	Channels    []string          `json:"channels,omitempty"`
	CallbackURL string            `json:"callback_url,omitempty"`
	Metadata    map[string]string `json:"metadata,omitempty"`
}

type initializeResponse struct {
	AuthorizationURL string `json:"authorization_url"`
	AccessCode       string `json:"access_code"`
	Reference        string `json:"reference"`
}

// CreateDeposit initialises a Paystack transaction the browser will resume
// via PaystackPop.resumeTransaction(access_code). Paystack chooses the
// one-time payment channels available for the account and currency.
func (p *Provider) CreateDeposit(ctx context.Context, intent billing.DepositIntent) (*billing.DepositSession, error) {
	req := initializeRequest{
		Email:       intent.CustomerEmail,
		Amount:      intent.AmountMinor,
		Currency:    string(intent.Currency),
		CallbackURL: intent.CallbackURL,
		Metadata:    intent.Metadata,
	}
	var resp initializeResponse
	if err := p.client.do(ctx, "POST", "/transaction/initialize", req, &resp); err != nil {
		return nil, fmt.Errorf("initialize transaction: %w", err)
	}
	if resp.AuthorizationURL == "" {
		return nil, fmt.Errorf("paystack returned empty authorization_url")
	}
	return &billing.DepositSession{
		URL:        resp.AuthorizationURL,
		AccessCode: resp.AccessCode,
		Reference:  resp.Reference,
	}, nil
}
