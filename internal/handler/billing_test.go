package handler

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/usehivy/hivy/internal/billing/purchase"
)

func TestWritePurchaseErrorReturnsActionableProviderRejection(t *testing.T) {
	handler := &BillingHandler{}
	request := httptest.NewRequest(http.MethodPost, "/v1/billing/purchases", nil)
	response := httptest.NewRecorder()

	handler.writePurchaseError(response, request, fmt.Errorf("initialize deposit: %w", purchase.ErrPaymentCurrencyUnavailable))

	if response.Code != http.StatusConflict {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusConflict)
	}
	if body := response.Body.String(); !strings.Contains(body, "selected payment currency is unavailable") {
		t.Fatalf("body = %s", body)
	}
}

func TestNormalizeCreateCreditPurchaseRequiresOneAmountSource(t *testing.T) {
	customSubtotal := int64(500)
	for name, body := range map[string]createCreditPurchaseRequest{
		"missing": {},
		"both":    {PackID: "usd_5", SubtotalMinor: &customSubtotal},
	} {
		t.Run(name, func(t *testing.T) {
			response := httptest.NewRecorder()
			if _, ok := normalizeCreateCreditPurchaseForRequest(response, body); ok {
				t.Fatal("normalize request succeeded, want rejection")
			}
			if response.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, want %d", response.Code, http.StatusBadRequest)
			}
		})
	}
}

func TestNormalizeCreateCreditPurchaseAcceptsCustomAmount(t *testing.T) {
	customSubtotal := int64(1_234)
	response := httptest.NewRecorder()
	body, ok := normalizeCreateCreditPurchaseForRequest(response, createCreditPurchaseRequest{
		Currency: " usd ", SubtotalMinor: &customSubtotal, IdempotencyKey: " key ",
	})
	if !ok {
		t.Fatalf("normalize request failed: %s", response.Body.String())
	}
	if body.Currency != "USD" || body.IdempotencyKey != "key" || body.SubtotalMinor == nil || *body.SubtotalMinor != customSubtotal {
		t.Fatalf("normalized body = %#v", body)
	}
}
