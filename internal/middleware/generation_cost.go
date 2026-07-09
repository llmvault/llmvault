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

func extractAttribution(db *gorm.DB, cache *AttributionCache, jti string, gen *model.Generation) {
	attr, ok := attributionFor(db, cache, jti)
	if !ok {
		return
	}
	gen.UserID = attr.UserID
	if len(attr.Tags) > 0 {
		gen.Tags = append(gen.Tags, attr.Tags...)
	}
	gen.SessionID = attr.SessionID
}

func attributionFor(db *gorm.DB, cache *AttributionCache, jti string) (Attribution, bool) {
	if attr, ok := cache.Get(jti); ok {
		return attr, true
	}
	attr, ok := loadAttribution(db, jti)
	if !ok {
		return Attribution{}, false
	}
	cache.Set(jti, attr)
	return attr, true
}

func loadAttribution(db *gorm.DB, jti string) (Attribution, bool) {
	var token model.Token
	if err := db.Select("meta").Where("jti = ?", jti).First(&token).Error; err != nil {
		return Attribution{}, false
	}
	var attr Attribution
	if token.Meta == nil {
		return attr, true
	}
	if user, ok := token.Meta["user"].(string); ok {
		attr.UserID = user
	}
	if tags, ok := token.Meta["tags"].([]any); ok {
		for _, t := range tags {
			if s, ok := t.(string); ok {
				attr.Tags = append(attr.Tags, s)
			}
		}
	}
	if raw, ok := token.Meta[model.TokenMetaSandboxID].(string); ok {
		attr.SandboxID = raw
	}
	attr.SessionID = sessionIDForSandbox(db, token.Meta)
	return attr, true
}

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
