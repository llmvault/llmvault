package middleware

import (
	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/billing"
	"github.com/usehivy/hivy/internal/model"
	"github.com/usehivy/hivy/internal/observe"
	"github.com/usehivy/hivy/internal/registry"
)

func calculateCost(reg *registry.Registry, providerID, modelID string, usage observe.UsageData) float64 {
	if reg == nil || providerID == "" || modelID == "" {
		return 0
	}
	cost, err := billing.EstimateCostUSD(reg, providerID, modelID,
		int64(usage.InputTokens), int64(usage.OutputTokens), int64(usage.CachedTokens))
	if err != nil {
		return 0
	}
	return cost
}

// extractAttribution reads token.meta to extract user_id and tags. Populated
// into the Generation row for observability filtering.
func extractAttribution(db *gorm.DB, jti string, gen *model.Generation) {
	var token model.Token
	if err := db.Select("meta").Where("jti = ?", jti).First(&token).Error; err != nil {
		return
	}
	if token.Meta == nil {
		return
	}
	if user, ok := token.Meta["user"].(string); ok {
		gen.UserID = user
	}
	if tags, ok := token.Meta["tags"].([]any); ok {
		for _, t := range tags {
			if s, ok := t.(string); ok {
				gen.Tags = append(gen.Tags, s)
			}
		}
	}
	gen.SessionID = sessionIDForSandbox(db, token.Meta)
}

// sessionIDForSandbox resolves the session that owns the token's sandbox.
// Sandboxes are provisioned one-per-session at session create, so the
// sandbox_id in an agent-proxy token identifies exactly one session.
func sessionIDForSandbox(db *gorm.DB, meta model.JSON) *uuid.UUID {
	raw, ok := meta[model.TokenMetaSandboxID].(string)
	if !ok {
		return nil
	}
	sandboxID, err := uuid.Parse(raw)
	if err != nil || sandboxID == uuid.Nil {
		return nil
	}
	var session model.Session
	if err := db.Select("id").Where("sandbox_id = ?", sandboxID).Take(&session).Error; err != nil {
		return nil
	}
	return &session.ID
}
