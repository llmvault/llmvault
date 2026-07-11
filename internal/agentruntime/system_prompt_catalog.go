package agentruntime

import (
	"context"
	"strings"

	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/model"
)

// effectiveAgentInstructions resolves an agent's system prompt with a strict,
// non-blanking precedence:
//
//  1. agent.Instructions — a user fork; authoritative once set.
//  2. Live catalog instructions for a clone without a prompt override.
//  3. agent.InstructionsSnapshot when its catalog is no longer active.
//  4. Live catalog fallback for legacy rows without a snapshot.
//
// The function never returns empty as long as any layer has content, so a
// blanked/archived catalog can no longer strand a clone with no prompt.
func effectiveAgentInstructions(ctx context.Context, db *gorm.DB, agent *model.Agent) string {
	if agent == nil {
		return ""
	}
	if agent.Instructions != nil {
		if instructions := strings.TrimSpace(*agent.Instructions); instructions != "" {
			return instructions
		}
	}
	if agent.AgentCatalogID != nil {
		if instructions := liveCatalogInstructions(ctx, db, agent); instructions != "" {
			return instructions
		}
	}
	if agent.InstructionsSnapshot != nil {
		if instructions := strings.TrimSpace(*agent.InstructionsSnapshot); instructions != "" {
			return instructions
		}
	}
	return liveCatalogInstructions(ctx, db, agent)
}

func liveCatalogInstructions(ctx context.Context, db *gorm.DB, agent *model.Agent) string {
	if agent == nil {
		return ""
	}
	if agent.AgentCatalog != nil {
		if instructions := strings.TrimSpace(agent.AgentCatalog.Instructions); instructions != "" {
			return instructions
		}
	}
	if agent.AgentCatalogID == nil || db == nil {
		return ""
	}
	var catalog model.AgentCatalog
	err := db.WithContext(ctx).
		Where("id = ? AND status = ?", *agent.AgentCatalogID, model.AgentCatalogStatusActive).
		First(&catalog).Error
	if err != nil {
		return ""
	}
	return strings.TrimSpace(catalog.Instructions)
}
