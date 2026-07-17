package paystack

import (
	"context"
	_ "embed"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"

	"github.com/usehivy/hivy/internal/billing"
)

//go:embed testdata/charge_authorization_success.json
var chargeAuthorizationFixture []byte

//go:embed testdata/verify_transaction_card.json
var verifyTransactionFixture []byte

func TestChargeSavedPaymentSendsServerOwnedAmountAndCurrency(t *testing.T) {
	purchaseID := uuid.MustParse("11111111-1111-1111-1111-111111111111")
	var body chargeAuthorizationRequest
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/transaction/charge_authorization" || r.Method != http.MethodPost {
			t.Fatalf("request = %s %s", r.Method, r.URL.Path)
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(chargeAuthorizationFixture)
	}))
	defer server.Close()

	provider := New(Config{SecretKey: "sk_test_scrubbed"})
	provider.client.baseURL = server.URL
	provider.client.http = server.Client()
	session, err := provider.ChargeSavedPayment(context.Background(), billing.SavedPaymentCharge{
		PurchaseID: purchaseID, AuthorizationCode: "AUTH_test_reusable",
		CustomerEmail: "buyer@example.com", AmountMinor: 1100,
		Currency: billing.CurrencyUSD,
		Metadata: map[string]string{"purchase_id": purchaseID.String()},
	})
	if err != nil {
		t.Fatalf("ChargeSavedPayment: %v", err)
	}
	if session.Reference != purchaseID.String() {
		t.Fatalf("reference = %q", session.Reference)
	}
	if body.Amount != 1100 || body.Currency != "USD" || body.Reference != purchaseID.String() {
		t.Fatalf("charge body = %#v", body)
	}
	var metadata map[string]string
	if err := json.Unmarshal([]byte(body.Metadata), &metadata); err != nil || metadata["purchase_id"] != purchaseID.String() {
		t.Fatalf("metadata = %q, err = %v", body.Metadata, err)
	}
}

func TestResolveDepositReturnsReusableAuthorization(t *testing.T) {
	purchaseID := uuid.MustParse("11111111-1111-1111-1111-111111111111")
	orgID := uuid.MustParse("22222222-2222-2222-2222-222222222222")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(verifyTransactionFixture)
	}))
	defer server.Close()

	provider := New(Config{SecretKey: "sk_test_scrubbed"})
	provider.client.baseURL = server.URL
	provider.client.http = server.Client()
	result, err := provider.ResolveDeposit(context.Background(), billing.ResolveDepositRequest{
		Reference: purchaseID.String(), ExpectedOrgID: orgID, ExpectedPurchaseID: purchaseID,
	})
	if err != nil {
		t.Fatalf("ResolveDeposit: %v", err)
	}
	if result.Authorization == nil || !result.Authorization.Reusable || result.Authorization.Signature != "SIG_test_card" {
		t.Fatalf("authorization = %#v", result.Authorization)
	}
	if result.CustomerEmail != "buyer@example.com" || result.PaidAmountMinor != 1100 || result.Currency != billing.CurrencyUSD {
		t.Fatalf("result = %#v", result)
	}
}

func TestParseMetadataAcceptsPaystackJSONString(t *testing.T) {
	metadata := parseMetadata(json.RawMessage(`"{\"org_id\":\"22222222-2222-2222-2222-222222222222\"}"`))
	if metadata["org_id"] != "22222222-2222-2222-2222-222222222222" {
		t.Fatalf("metadata = %#v", metadata)
	}
}
