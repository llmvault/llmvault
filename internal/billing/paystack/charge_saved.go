package paystack

import (
	"context"
	"fmt"
	"net/http"

	"github.com/usehivy/hivy/internal/billing"
)

type chargeAuthorizationRequest struct {
	AuthorizationCode string `json:"authorization_code"`
	Email             string `json:"email"`
	Amount            int64  `json:"amount"`
	Currency          string `json:"currency"`
	Reference         string `json:"reference"`
	Metadata          string `json:"metadata,omitempty"`
}

type chargeAuthorizationResponse struct {
	Reference        string `json:"reference"`
	Status           string `json:"status"`
	AuthorizationURL string `json:"authorization_url"`
	AccessCode       string `json:"access_code"`
	Paused           bool   `json:"paused"`
}

// ChargeSavedPayment charges a reusable authorization. Paystack can still
// require 2FA, in which case the returned access code resumes that challenge.
func (p *Provider) ChargeSavedPayment(ctx context.Context, charge billing.SavedPaymentCharge) (*billing.DepositSession, error) {
	if charge.AuthorizationCode == "" || charge.CustomerEmail == "" {
		return nil, fmt.Errorf("charge saved payment: missing authorization")
	}
	var response chargeAuthorizationResponse
	err := p.client.do(ctx, http.MethodPost, "/transaction/charge_authorization", chargeAuthorizationRequest{
		AuthorizationCode: charge.AuthorizationCode,
		Email:             charge.CustomerEmail,
		Amount:            charge.AmountMinor,
		Currency:          string(charge.Currency),
		Reference:         charge.PurchaseID.String(),
		Metadata:          encodeMetadata(charge.Metadata),
	}, &response)
	if err != nil {
		return nil, fmt.Errorf("charge authorization: %w", err)
	}
	if response.Reference == "" {
		return nil, fmt.Errorf("charge authorization: empty reference")
	}
	return &billing.DepositSession{
		Reference:  response.Reference,
		URL:        response.AuthorizationURL,
		AccessCode: response.AccessCode,
	}, nil
}
