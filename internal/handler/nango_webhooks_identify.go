package handler

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"strings"

	"github.com/usehivy/hivy/internal/model"
)

// identify resolves the connection (and hence org) a Nango delivery belongs to.
//
// The nango_connection_id alone is NOT a safe key: two orgs could historically
// hold rows with the same id (client-supplied, unverified), so taking the
// newest ("ORDER BY created_at DESC First") could deliver one org's events into
// another. We now (a) disambiguate by the delivery's providerConfigKey (the
// integration's unique key) when present, and (b) REJECT on zero-or-multiple
// matches rather than guessing. The connection-create verification plus the
// active-uniqueness index on (integration_id, nango_connection_id) keep the
// live match set to at most one.
func (h *NangoWebhookHandler) identify(ctx context.Context, wh *nangoWebhook) *webhookContext {
	connID := strings.TrimSpace(wh.ConnectionID)
	if connID == "" {
		return nil
	}

	q := h.db.WithContext(ctx).Model(&model.Connection{}).
		Where("nango_connection_id = ? AND revoked_at IS NULL", connID)

	if key := strings.TrimSpace(wh.ProviderConfigKey); key != "" {
		var integ model.Integration
		if err := h.db.WithContext(ctx).
			Select("id").
			Where("unique_key = ? AND deleted_at IS NULL", key).
			First(&integ).Error; err != nil {
			return nil
		}
		q = q.Where("integration_id = ?", integ.ID)
	}

	var matches []model.Connection
	if err := q.Limit(2).Find(&matches).Error; err != nil {
		return nil
	}
	if len(matches) != 1 {
		// Zero: unknown connection. Multiple: ambiguous — never guess which org
		// owns the delivery.
		return nil
	}

	connection := matches[0]
	if err := h.db.WithContext(ctx).
		Preload("Integration").
		First(&connection, "id = ?", connection.ID).Error; err != nil {
		return nil
	}
	return &webhookContext{
		orgID:      connection.OrgID,
		connection: &connection,
	}
}

// verifyGitHubSignature256 validates a GitHub webhook's X-Hub-Signature-256
// header ("sha256=<hex>") against the raw request body using the configured
// webhook secret. This is native, provider-side authenticity independent of
// Nango's own HMAC — the exact drift the Nango-only posture cannot detect. Fails
// closed on an empty secret or malformed header.
func verifyGitHubSignature256(body []byte, secret, header string) bool {
	if secret == "" {
		return false
	}
	const prefix = "sha256="
	if !strings.HasPrefix(header, prefix) {
		return false
	}
	provided := strings.TrimPrefix(header, prefix)
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	expected := hex.EncodeToString(mac.Sum(nil))
	return hmac.Equal([]byte(expected), []byte(provided))
}

// verifyWebhookSharedSecret constant-time compares a provided shared secret
// against the configured one. Used for direct/Railway incoming webhooks that
// carry no provider signature — the secret is a stronger bearer than the
// unguessable connection UUID alone. Fails closed on an empty configured secret.
func verifyWebhookSharedSecret(configured, provided string) bool {
	if configured == "" {
		return false
	}
	return hmac.Equal([]byte(configured), []byte(provided))
}

func verifyNangoSignature(body []byte, secret string, signature string) bool {
	// Fail closed: without a webhook secret we can't verify the HMAC, so an
	// attacker could forge a valid empty-key HMAC for arbitrary orgs.
	if secret == "" {
		return false
	}
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	expected := hex.EncodeToString(mac.Sum(nil))
	return hmac.Equal([]byte(expected), []byte(signature))
}
