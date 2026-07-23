package handler

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/billing"
	"github.com/usehivy/hivy/internal/billing/purchase"
	"github.com/usehivy/hivy/internal/logging"
	"github.com/usehivy/hivy/internal/middleware"
	"github.com/usehivy/hivy/internal/model"
)

type BillingHandler struct {
	db        *gorm.DB
	purchases *purchase.Service
	credits   *billing.CreditsService
}

func NewBillingHandler(db *gorm.DB, purchases *purchase.Service, credits *billing.CreditsService) *BillingHandler {
	return &BillingHandler{db: db, purchases: purchases, credits: credits}
}

type billingAccountResponse struct {
	Balance             int64                `json:"balance"`
	FeeBasisPoints      int64                `json:"fee_basis_points"`
	SupportedCurrencies []string             `json:"supported_currencies"`
	Packs               []creditPackResponse `json:"packs"`
}

type creditPackResponse struct {
	ID             string `json:"id"`
	Currency       string `json:"currency"`
	SubtotalMinor  int64  `json:"subtotal_minor"`
	FeeBasisPoints int64  `json:"fee_basis_points"`
	FeeMinor       int64  `json:"fee_minor"`
	TotalMinor     int64  `json:"total_minor"`
	Credits        int64  `json:"credits"`
}

// GetAccount returns the org's permanent credit balance and deposit settings.
// @Summary Get billing account
// @Tags billing
// @Produce json
// @Success 200 {object} billingAccountResponse
// @Failure 401 {object} errorResponse
// @Failure 500 {object} errorResponse
// @Security BearerAuth
// @Router /v1/billing/account [get]
func (h *BillingHandler) GetAccount(w http.ResponseWriter, r *http.Request) {
	org, ok := middleware.OrgFromContext(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, errorResponse{Error: "missing org context"})
		return
	}
	balance, err := h.credits.Balance(org.ID)
	if err != nil {
		logging.FromContext(r.Context()).ErrorContext(r.Context(), "load billing balance", "error", err)
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "failed to load billing account"})
		return
	}
	packs := h.purchases.Packs()
	packResponses := make([]creditPackResponse, 0, len(packs))
	for _, pack := range packs {
		packResponses = append(packResponses, creditPackResponse{
			ID: pack.ID, Currency: string(pack.Currency), SubtotalMinor: pack.SubtotalMinor,
			FeeBasisPoints: pack.FeeBasisPoints, FeeMinor: pack.FeeMinor,
			TotalMinor: pack.TotalMinor, Credits: pack.Credits,
		})
	}
	writeJSON(w, http.StatusOK, billingAccountResponse{
		Balance:             balance,
		FeeBasisPoints:      purchase.DepositFeeBasisPoints,
		SupportedCurrencies: []string{string(billing.CurrencyUSD), string(billing.CurrencyNGN)},
		Packs:               packResponses,
	})
}

type createCreditPurchaseRequest struct {
	Currency          string  `json:"currency"`
	PackID            string  `json:"pack_id"`
	IdempotencyKey    string  `json:"idempotency_key"`
	PaymentMethodID   *string `json:"payment_method_id,omitempty"`
	SavePaymentMethod bool    `json:"save_payment_method"`
}

type creditPurchaseResponse struct {
	ID                string `json:"id"`
	Status            string `json:"status"`
	Currency          string `json:"currency"`
	SubtotalMinor     int64  `json:"subtotal_minor"`
	FeeBasisPoints    int64  `json:"fee_basis_points"`
	FeeMinor          int64  `json:"fee_minor"`
	TotalMinor        int64  `json:"total_minor"`
	Credits           int64  `json:"credits"`
	FXMinorPerUSD     *int64 `json:"fx_minor_per_usd,omitempty"`
	ProviderReference string `json:"provider_reference,omitempty"`
	AccessCode        string `json:"access_code,omitempty"`
	CheckoutURL       string `json:"checkout_url,omitempty"`
	PaidAt            string `json:"paid_at,omitempty"`
	CreditedAt        string `json:"credited_at,omitempty"`
	CreatedAt         string `json:"created_at"`
	PackID            string `json:"pack_id"`
	PaymentMethodID   string `json:"payment_method_id,omitempty"`
}

// CreatePurchase creates a pending Paystack deposit and returns popup details.
// @Summary Create credit purchase
// @Tags billing
// @Accept json
// @Produce json
// @Param body body createCreditPurchaseRequest true "Credit purchase"
// @Success 201 {object} creditPurchaseResponse
// @Failure 400 {object} errorResponse
// @Failure 401 {object} errorResponse
// @Failure 409 {object} errorResponse
// @Failure 500 {object} errorResponse
// @Security BearerAuth
// @Router /v1/billing/purchases [post]
func (h *BillingHandler) CreatePurchase(w http.ResponseWriter, r *http.Request) {
	org, ok := middleware.OrgFromContext(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, errorResponse{Error: "missing org context"})
		return
	}
	var body createCreditPurchaseRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "invalid request body"})
		return
	}
	userID, ok := currentRequestUserID(r.Context())
	if !ok || userID == nil {
		writeJSON(w, http.StatusUnauthorized, errorResponse{Error: "missing user context"})
		return
	}
	var user model.User
	if err := h.db.WithContext(r.Context()).Where("id = ?", *userID).First(&user).Error; err != nil {
		logging.FromContext(r.Context()).ErrorContext(r.Context(), "load billing user", "error", err)
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "failed to create credit purchase"})
		return
	}
	var paymentMethodID *uuid.UUID
	if body.PaymentMethodID != nil {
		parsed, err := uuid.Parse(*body.PaymentMethodID)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, errorResponse{Error: "invalid payment method id"})
			return
		}
		paymentMethodID = &parsed
	}
	result, err := h.purchases.Create(r.Context(), purchase.CreateInput{
		OrgID:             org.ID,
		UserID:            *userID,
		Email:             user.Email,
		Currency:          billing.Currency(body.Currency),
		PackID:            body.PackID,
		IdempotencyKey:    body.IdempotencyKey,
		PaymentMethodID:   paymentMethodID,
		SavePaymentMethod: body.SavePaymentMethod,
	})
	if err != nil {
		h.writePurchaseError(w, r, err)
		return
	}
	resp := purchaseDTO(*result.Purchase)
	resp.AccessCode = result.Session.AccessCode
	resp.CheckoutURL = result.Session.URL
	writeJSON(w, http.StatusCreated, resp)
}

// VerifyPurchase resolves Paystack payment and atomically grants credits.
// @Summary Verify credit purchase
// @Tags billing
// @Produce json
// @Param id path string true "Purchase ID"
// @Success 200 {object} creditPurchaseResponse
// @Failure 400 {object} errorResponse
// @Failure 404 {object} errorResponse
// @Failure 409 {object} errorResponse
// @Security BearerAuth
// @Router /v1/billing/purchases/{id}/verify [post]
func (h *BillingHandler) VerifyPurchase(w http.ResponseWriter, r *http.Request) {
	org, ok := middleware.OrgFromContext(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, errorResponse{Error: "missing org context"})
		return
	}
	purchaseID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "invalid purchase id"})
		return
	}
	row, err := h.purchases.Verify(r.Context(), org.ID, purchaseID)
	if err != nil {
		h.writePurchaseError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, purchaseDTO(*row))
}

type creditPurchasesResponse struct {
	Purchases []creditPurchaseResponse `json:"purchases"`
}

// ListPurchases returns the org's most recent deposits.
// @Summary List credit purchases
// @Tags billing
// @Produce json
// @Param limit query int false "Maximum purchases"
// @Success 200 {object} creditPurchasesResponse
// @Security BearerAuth
// @Router /v1/billing/purchases [get]
func (h *BillingHandler) ListPurchases(w http.ResponseWriter, r *http.Request) {
	org, ok := middleware.OrgFromContext(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, errorResponse{Error: "missing org context"})
		return
	}
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	rows, err := h.purchases.List(r.Context(), org.ID, limit)
	if err != nil {
		h.writePurchaseError(w, r, err)
		return
	}
	out := make([]creditPurchaseResponse, 0, len(rows))
	for _, row := range rows {
		out = append(out, purchaseDTO(row))
	}
	writeJSON(w, http.StatusOK, creditPurchasesResponse{Purchases: out})
}

func purchaseDTO(row model.CreditPurchase) creditPurchaseResponse {
	resp := creditPurchaseResponse{
		ID:                row.ID.String(),
		Status:            row.Status,
		Currency:          row.Currency,
		SubtotalMinor:     row.SubtotalMinor,
		FeeBasisPoints:    row.FeeBasisPoints,
		FeeMinor:          row.FeeMinor,
		TotalMinor:        row.TotalMinor,
		Credits:           row.Credits,
		FXMinorPerUSD:     row.FXMinorPerUSD,
		ProviderReference: row.ProviderRef,
		CreatedAt:         row.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
		PackID:            row.PackID,
	}
	if row.PaymentMethodID != nil {
		resp.PaymentMethodID = row.PaymentMethodID.String()
	}
	if row.PaidAt != nil {
		resp.PaidAt = row.PaidAt.Format("2006-01-02T15:04:05Z07:00")
	}
	if row.CreditedAt != nil {
		resp.CreditedAt = row.CreditedAt.Format("2006-01-02T15:04:05Z07:00")
	}
	return resp
}

func (h *BillingHandler) writePurchaseError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, purchase.ErrInvalidAmount), errors.Is(err, purchase.ErrInvalidPack):
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "invalid deposit amount"})
	case errors.Is(err, purchase.ErrInvalidRequestKey):
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "invalid idempotency key"})
	case errors.Is(err, purchase.ErrInvalidCurrency):
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "unsupported billing currency"})
	case errors.Is(err, purchase.ErrNotFound):
		writeJSON(w, http.StatusNotFound, errorResponse{Error: "credit purchase not found"})
	case errors.Is(err, purchase.ErrPaymentPending):
		writeJSON(w, http.StatusConflict, errorResponse{Error: "payment is not complete"})
	case errors.Is(err, purchase.ErrPaymentMismatch):
		writeJSON(w, http.StatusPaymentRequired, errorResponse{Error: "paid amount or currency does not match purchase"})
	case errors.Is(err, purchase.ErrPaymentMethodNotFound):
		writeJSON(w, http.StatusNotFound, errorResponse{Error: "payment method not found"})
	case errors.Is(err, purchase.ErrPaymentMethodUnavailable):
		writeJSON(w, http.StatusConflict, errorResponse{Error: "saved payment method is unavailable"})
	case errors.Is(err, purchase.ErrPaymentCurrencyUnavailable):
		logging.FromContext(r.Context()).ErrorContext(r.Context(), "payment provider rejected credit purchase", "error", err)
		writeJSON(w, http.StatusConflict, errorResponse{Error: "selected payment currency is unavailable"})
	default:
		logging.FromContext(r.Context()).ErrorContext(r.Context(), "credit purchase failed", "error", err)
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "credit purchase failed"})
	}
}
