// Package orgtier owns permanent organization capacity unlocks and their
// scarce-resource limits.
package orgtier

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/model"
)

const (
	Tier1 = 1
	Tier2 = 2
	Tier3 = 3
	Tier4 = 4

	creditsPerUSD = int64(1_000)
)

var (
	ErrConcurrentSessions    = errors.New("org tier concurrent session limit reached")
	ErrSandboxSizeNotAllowed = errors.New("org tier sandbox size is not allowed")
	ErrKnowledgeStorageLimit = errors.New("org tier knowledge storage limit reached")
)

type Limits struct {
	Tier                  int
	ConcurrentSessions    int
	MaxSandboxSize        string
	KnowledgeStorageBytes int64
}

var limitsByTier = map[int]Limits{
	Tier1: {Tier: Tier1, ConcurrentSessions: 1, MaxSandboxSize: "nano", KnowledgeStorageBytes: 1 << 30},
	Tier2: {Tier: Tier2, ConcurrentSessions: 2, MaxSandboxSize: "small", KnowledgeStorageBytes: 3 << 30},
	Tier3: {Tier: Tier3, ConcurrentSessions: 5, MaxSandboxSize: "medium", KnowledgeStorageBytes: 5 << 30},
	Tier4: {Tier: Tier4, ConcurrentSessions: 10, MaxSandboxSize: "large", KnowledgeStorageBytes: 10 << 30},
}

var sandboxSizeRank = map[string]int{
	"nano":   1,
	"small":  2,
	"medium": 3,
	"large":  4,
}

func LimitsForTier(tier int) Limits {
	if limits, ok := limitsByTier[tier]; ok {
		return limits
	}
	return limitsByTier[Tier1]
}

// TierForLifetimeCredits converts USD-normalized purchased credits into the
// highest permanent tier the org has earned.
func TierForLifetimeCredits(credits int64) int {
	switch {
	case credits >= 500*creditsPerUSD:
		return Tier4
	case credits >= 250*creditsPerUSD:
		return Tier3
	case credits >= 100*creditsPerUSD:
		return Tier2
	default:
		return Tier1
	}
}

func ValidateSandboxSize(tier int, size string) error {
	size = strings.ToLower(strings.TrimSpace(size))
	rank, ok := sandboxSizeRank[size]
	if !ok {
		return ErrSandboxSizeNotAllowed
	}
	maxRank := sandboxSizeRank[LimitsForTier(tier).MaxSandboxSize]
	if rank > maxRank {
		return ErrSandboxSizeNotAllowed
	}
	return nil
}

// EffectiveSandboxSize resolves a custom template's resource size when one is
// selected. Template resources override the agent's configured sandbox size at
// runtime, so every tier gate must validate this effective value.
func EffectiveSandboxSize(
	ctx context.Context,
	db *gorm.DB,
	orgID uuid.UUID,
	configuredSize string,
	templateID *uuid.UUID,
) (string, error) {
	size := model.NormalizeTemplateSize(configuredSize)
	if templateID == nil {
		return size, nil
	}
	if db == nil {
		return "", fmt.Errorf("load effective sandbox template: database is required")
	}
	var tmpl model.SandboxTemplate
	if err := db.WithContext(ctx).
		Where("id = ? AND (org_id = ? OR org_id IS NULL)", *templateID, orgID).
		First(&tmpl).Error; err != nil {
		return "", fmt.Errorf("load effective sandbox template: %w", err)
	}
	if strings.TrimSpace(tmpl.Size) != "" {
		size = model.NormalizeTemplateSize(tmpl.Size)
	}
	return size, nil
}

// PromoteForCompletedDeposits atomically raises an org to the highest tier
// earned by all completed deposits. GREATEST makes the update monotonic even if
// a purchase is later refunded or reversed.
func PromoteForCompletedDeposits(ctx context.Context, tx *gorm.DB, orgID uuid.UUID) error {
	if tx == nil {
		return fmt.Errorf("promote org tier: database is required")
	}
	var lifetimeCredits int64
	if err := tx.WithContext(ctx).Model(&model.CreditPurchase{}).
		Where("org_id = ? AND status = ?", orgID, model.CreditPurchaseCredited).
		Select("COALESCE(SUM(credits), 0)").Scan(&lifetimeCredits).Error; err != nil {
		return fmt.Errorf("sum completed org deposits: %w", err)
	}
	tier := TierForLifetimeCredits(lifetimeCredits)
	res := tx.WithContext(ctx).Model(&model.Org{}).
		Where("id = ?", orgID).
		Update("capacity_tier", gorm.Expr("GREATEST(capacity_tier, ?)", tier))
	if res.Error != nil {
		return fmt.Errorf("promote org tier: %w", res.Error)
	}
	if res.RowsAffected != 1 {
		return fmt.Errorf("promote org tier: org not found")
	}
	return nil
}

func lockOrg(ctx context.Context, tx *gorm.DB, orgID uuid.UUID) (model.Org, error) {
	if err := tx.WithContext(ctx).
		Exec("SELECT pg_advisory_xact_lock(hashtextextended(?, 0))", orgID.String()).Error; err != nil {
		return model.Org{}, fmt.Errorf("lock org capacity: %w", err)
	}
	var org model.Org
	if err := tx.WithContext(ctx).Where("id = ?", orgID).First(&org).Error; err != nil {
		return model.Org{}, fmt.Errorf("lock org capacity: %w", err)
	}
	return org, nil
}

func loadOrg(ctx context.Context, db *gorm.DB, orgID uuid.UUID) (model.Org, error) {
	var org model.Org
	if err := db.WithContext(ctx).Where("id = ?", orgID).First(&org).Error; err != nil {
		return model.Org{}, fmt.Errorf("load org capacity: %w", err)
	}
	return org, nil
}
