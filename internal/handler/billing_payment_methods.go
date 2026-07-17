package handler

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/usehivy/hivy/internal/middleware"
	"github.com/usehivy/hivy/internal/model"
)

type billingPaymentMethodResponse struct {
	ID          string `json:"id"`
	CardType    string `json:"card_type"`
	Last4       string `json:"last4"`
	ExpMonth    string `json:"exp_month"`
	ExpYear     string `json:"exp_year"`
	Bank        string `json:"bank"`
	CountryCode string `json:"country_code"`
}

type billingPaymentMethodsResponse struct {
	PaymentMethods []billingPaymentMethodResponse `json:"payment_methods"`
}

// ListPaymentMethods lists reusable Paystack cards without secret fields.
// @Summary List billing payment methods
// @Tags billing
// @Produce json
// @Success 200 {object} billingPaymentMethodsResponse
// @Failure 401 {object} errorResponse
// @Security BearerAuth
// @Router /v1/billing/payment-methods [get]
func (h *BillingHandler) ListPaymentMethods(w http.ResponseWriter, r *http.Request) {
	org, ok := middleware.OrgFromContext(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, errorResponse{Error: "missing org context"})
		return
	}
	userID, ok := currentRequestUserID(r.Context())
	if !ok || userID == nil {
		writeJSON(w, http.StatusUnauthorized, errorResponse{Error: "missing user context"})
		return
	}
	methods, err := h.purchases.ListPaymentMethods(r.Context(), org.ID, *userID)
	if err != nil {
		h.writePurchaseError(w, r, err)
		return
	}
	out := make([]billingPaymentMethodResponse, 0, len(methods))
	for _, method := range methods {
		out = append(out, paymentMethodDTO(method))
	}
	writeJSON(w, http.StatusOK, billingPaymentMethodsResponse{PaymentMethods: out})
}

// DeletePaymentMethod permanently removes a saved Paystack authorization from Hivy.
// @Summary Remove billing payment method
// @Tags billing
// @Produce json
// @Param id path string true "Payment method ID"
// @Success 200 {object} statusResponse
// @Failure 400 {object} errorResponse
// @Failure 404 {object} errorResponse
// @Security BearerAuth
// @Router /v1/billing/payment-methods/{id} [delete]
func (h *BillingHandler) DeletePaymentMethod(w http.ResponseWriter, r *http.Request) {
	org, ok := middleware.OrgFromContext(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, errorResponse{Error: "missing org context"})
		return
	}
	userID, ok := currentRequestUserID(r.Context())
	if !ok || userID == nil {
		writeJSON(w, http.StatusUnauthorized, errorResponse{Error: "missing user context"})
		return
	}
	methodID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "invalid payment method id"})
		return
	}
	if err := h.purchases.DeletePaymentMethod(r.Context(), org.ID, *userID, methodID); err != nil {
		h.writePurchaseError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, statusResponse{Status: "deleted"})
}

func paymentMethodDTO(method model.BillingPaymentMethod) billingPaymentMethodResponse {
	return billingPaymentMethodResponse{
		ID: method.ID.String(), CardType: method.CardType, Last4: method.Last4,
		ExpMonth: method.ExpMonth, ExpYear: method.ExpYear, Bank: method.Bank,
		CountryCode: method.CountryCode,
	}
}
