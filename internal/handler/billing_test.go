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
