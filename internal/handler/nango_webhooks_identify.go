package handler

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"

	"github.com/usehivy/hivy/internal/model"
)

func (h *NangoWebhookHandler) identify(ctx context.Context, wh *nangoWebhook) *webhookContext {
	var connection model.Connection
	err := h.db.Preload("Integration").
		Where("nango_connection_id = ? AND revoked_at IS NULL", wh.ConnectionID).
		Order("created_at DESC").
		First(&connection).Error
	if err != nil {
		return nil
	}

	return &webhookContext{
		orgID:      connection.OrgID,
		connection: &connection,
	}
}

func verifyNangoSignature(body []byte, secret string, signature string) bool {
	// Fail closed: without a configured webhook secret we cannot verify the HMAC,
	// so we cannot trust the caller. Proceeding would let an attacker forge a
	// valid X-Nango-Hmac-Sha256 header by computing HMAC with an empty key over
	// their chosen body, enabling forged connection-state / Slack inbound events
	// for arbitrary orgs. Mirrors the employee-outbound webhook hardening.
	if secret == "" {
		return false
	}
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	expected := hex.EncodeToString(mac.Sum(nil))
	return hmac.Equal([]byte(expected), []byte(signature))
}
