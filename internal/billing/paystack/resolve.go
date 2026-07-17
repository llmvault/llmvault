package paystack

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"time"

	"github.com/google/uuid"

	"github.com/usehivy/hivy/internal/billing"
)

type verifyTransactionResponse struct {
	Reference string     `json:"reference"`
	Status    string     `json:"status"`
	Amount    int64      `json:"amount"`
	Currency  string     `json:"currency"`
	PaidAt    *time.Time `json:"paid_at"`
	// Paystack returns metadata in one of three shapes — null, the
	// number 0 (their placeholder), or a JSON object — so we keep it
	// raw and decode by inspection.
	Metadata      json.RawMessage              `json:"metadata"`
	Authorization billing.PaymentAuthorization `json:"authorization"`
	Customer      struct {
		Email string `json:"email"`
	} `json:"customer"`
}

// ResolveDeposit calls /transaction/verify/:reference and returns the
// normalized result. The purchase service verifies amount and currency.
func (p *Provider) ResolveDeposit(ctx context.Context, req billing.ResolveDepositRequest) (*billing.DepositResult, error) {
	if req.Reference == "" {
		return nil, fmt.Errorf("paystack resolve: empty reference")
	}

	var tx verifyTransactionResponse
	if err := p.client.do(ctx, http.MethodGet, "/transaction/verify/"+url.PathEscape(req.Reference), nil, &tx); err != nil {
		return nil, fmt.Errorf("verify transaction: %w", err)
	}

	metadata := parseMetadata(tx.Metadata)

	// Defense-in-depth: reject a reference whose transaction was initialised for
	// a different org. Without this a member could reuse another org's valid
	// paid reference to credit a purchase they did not fund.
	if req.ExpectedOrgID != uuid.Nil {
		if metadata["org_id"] != req.ExpectedOrgID.String() {
			return nil, fmt.Errorf("%w: reference %q", billing.ErrOrgMismatch, req.Reference)
		}
	}
	if req.ExpectedPurchaseID != uuid.Nil && metadata["purchase_id"] != req.ExpectedPurchaseID.String() {
		return nil, fmt.Errorf("%w: reference %q", billing.ErrPurchaseMismatch, req.Reference)
	}

	result := &billing.DepositResult{
		Status:          mapTransactionStatus(tx.Status),
		PaidAt:          tx.PaidAt,
		PaidAmountMinor: tx.Amount,
		Currency:        billing.Currency(tx.Currency),
		Reference:       tx.Reference,
		Metadata:        metadata,
		CustomerEmail:   tx.Customer.Email,
	}
	if tx.Authorization.AuthorizationCode != "" {
		result.Authorization = &tx.Authorization
	}
	return result, nil
}

// parseMetadata decodes the metadata Paystack echoes back from the
// /transaction/initialize call. Their API uses three shapes here — null,
// the literal number 0 (their "no metadata" placeholder), or a JSON
// object — so we accept all of them and ignore unknown shapes.
func parseMetadata(raw json.RawMessage) map[string]string {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 || bytes.Equal(trimmed, []byte("null")) || bytes.Equal(trimmed, []byte("0")) {
		return nil
	}
	var m map[string]string
	if err := json.Unmarshal(trimmed, &m); err == nil {
		return m
	}
	var encoded string
	if err := json.Unmarshal(trimmed, &encoded); err != nil {
		return nil
	}
	if err := json.Unmarshal([]byte(encoded), &m); err != nil {
		return nil
	}
	return m
}

func mapTransactionStatus(s string) billing.PaymentStatus {
	switch s {
	case "success":
		return billing.PaymentPaid
	case "failed", "abandoned":
		return billing.PaymentFailed
	case "reversed":
		return billing.PaymentReversed
	}
	return billing.PaymentPending
}
