package handler

import (
	"crypto/hmac"
	"crypto/sha512"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"

	"github.com/google/uuid"

	"github.com/usehivy/hivy/internal/billing/purchase"
	"github.com/usehivy/hivy/internal/logging"
)

const maxPaystackWebhookBytes = 1 << 20

type PaystackWebhookHandler struct {
	secretKey string
	purchases *purchase.Service
}

func NewPaystackWebhookHandler(secretKey string, purchases *purchase.Service) *PaystackWebhookHandler {
	return &PaystackWebhookHandler{secretKey: secretKey, purchases: purchases}
}

type paystackWebhookEnvelope struct {
	Event string `json:"event"`
	Data  struct {
		Metadata json.RawMessage `json:"metadata"`
	} `json:"data"`
}

// Handle verifies Paystack's signature and credits successful purchases.
// @Summary Receive Paystack payment events
// @Tags internal
// @Accept json
// @Produce json
// @Success 200 {object} statusResponse
// @Failure 400 {object} errorResponse
// @Failure 401 {object} errorResponse
// @Failure 500 {object} errorResponse
// @Router /internal/webhooks/paystack [post]
func (h *PaystackWebhookHandler) Handle(w http.ResponseWriter, r *http.Request) {
	if h == nil || h.secretKey == "" || h.purchases == nil {
		writeJSON(w, http.StatusNotFound, errorResponse{Error: "not found"})
		return
	}
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxPaystackWebhookBytes))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "invalid webhook body"})
		return
	}
	if !validPaystackSignature(body, r.Header.Get("x-paystack-signature"), h.secretKey) {
		writeJSON(w, http.StatusUnauthorized, errorResponse{Error: "invalid webhook signature"})
		return
	}
	var event paystackWebhookEnvelope
	if err := json.Unmarshal(body, &event); err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "invalid webhook body"})
		return
	}
	if event.Event != "charge.success" {
		writeJSON(w, http.StatusOK, statusResponse{Status: "ignored"})
		return
	}
	metadata := paystackWebhookMetadata(event.Data.Metadata)
	orgID, orgErr := uuid.Parse(metadata["org_id"])
	purchaseID, purchaseErr := uuid.Parse(metadata["purchase_id"])
	if orgErr != nil || purchaseErr != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "invalid payment metadata"})
		return
	}
	if _, err := h.purchases.Verify(r.Context(), orgID, purchaseID); err != nil {
		logging.FromContext(r.Context()).ErrorContext(r.Context(), "process Paystack webhook", "error", err, "purchase_id", purchaseID)
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "failed to process payment"})
		return
	}
	writeJSON(w, http.StatusOK, statusResponse{Status: "processed"})
}

func validPaystackSignature(body []byte, signature, secret string) bool {
	provided, err := hex.DecodeString(signature)
	if err != nil {
		return false
	}
	mac := hmac.New(sha512.New, []byte(secret))
	_, _ = mac.Write(body)
	return hmac.Equal(provided, mac.Sum(nil))
}

func paystackWebhookMetadata(raw json.RawMessage) map[string]string {
	var metadata map[string]string
	if json.Unmarshal(raw, &metadata) == nil {
		return metadata
	}
	var encoded string
	if json.Unmarshal(raw, &encoded) == nil {
		_ = json.Unmarshal([]byte(encoded), &metadata)
	}
	return metadata
}
