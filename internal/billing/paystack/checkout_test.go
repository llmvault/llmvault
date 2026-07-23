package paystack

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"

	"github.com/usehivy/hivy/internal/billing"
)

func TestCreateDepositSendsCurrencyAmountAndMetadata(t *testing.T) {
	purchaseID := uuid.MustParse("11111111-1111-1111-1111-111111111111")
	orgID := uuid.MustParse("22222222-2222-2222-2222-222222222222")
	var body initializeRequest
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/transaction/initialize" || r.Method != http.MethodPost {
			t.Fatalf("request = %s %s", r.Method, r.URL.Path)
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"status": true,
			"message": "Authorization URL created",
			"data": {
				"authorization_url": "https://checkout.paystack.test/access_test",
				"access_code": "access_test",
				"reference": "11111111-1111-1111-1111-111111111111"
			}
		}`))
	}))
	defer server.Close()

	provider := New(Config{SecretKey: "sk_test_scrubbed"})
	provider.client.baseURL = server.URL
	provider.client.http = server.Client()
	session, err := provider.CreateDeposit(context.Background(), billing.DepositIntent{
		PurchaseID: purchaseID, OrgID: orgID, CustomerEmail: "buyer@example.com",
		AmountMinor: 560, Currency: billing.CurrencyUSD, Channels: []string{"card"},
		Metadata: map[string]string{
			"org_id": orgID.String(), "purchase_id": purchaseID.String(), "pack_id": "usd_5",
		},
	})
	if err != nil {
		t.Fatalf("CreateDeposit: %v", err)
	}
	if session.Reference != purchaseID.String() || session.AccessCode != "access_test" {
		t.Fatalf("session = %#v", session)
	}
	if body.Email != "buyer@example.com" || body.Amount != 560 || body.Currency != "USD" {
		t.Fatalf("initialize body = %#v", body)
	}
	if len(body.Channels) != 1 || body.Channels[0] != "card" || body.Reference != purchaseID.String() {
		t.Fatalf("initialize routing body = %#v", body)
	}
	var metadata map[string]string
	if err := json.Unmarshal([]byte(body.Metadata), &metadata); err != nil {
		t.Fatalf("decode metadata: %v", err)
	}
	if metadata["org_id"] != orgID.String() || metadata["purchase_id"] != purchaseID.String() || metadata["pack_id"] != "usd_5" {
		t.Fatalf("metadata = %#v", metadata)
	}
}

func TestCreateDepositReturnsStructuredProviderRejection(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{
			"status": false,
			"message": "Currency not supported by merchant",
			"type": "validation_error"
		}`))
	}))
	defer server.Close()

	provider := New(Config{SecretKey: "sk_test_scrubbed"})
	provider.client.baseURL = server.URL
	provider.client.http = server.Client()
	_, err := provider.CreateDeposit(context.Background(), billing.DepositIntent{
		PurchaseID: uuid.New(), CustomerEmail: "buyer@example.com",
		AmountMinor: 560, Currency: billing.CurrencyUSD,
	})
	var providerErr *billing.ProviderRequestError
	if !errors.As(err, &providerErr) {
		t.Fatalf("error = %v, want ProviderRequestError", err)
	}
	if providerErr.StatusCode != http.StatusForbidden || providerErr.Type != "validation_error" {
		t.Fatalf("provider error = %#v", providerErr)
	}
}
