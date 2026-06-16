package agentruntime

import (
	"context"
	"strings"

	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/model"
)

func effectiveAgentInstructions(ctx context.Context, db *gorm.DB, agent *model.Agent) string {
	if agent == nil {
		return ""
	}
	if agent.Instructions != nil {
		if instructions := strings.TrimSpace(*agent.Instructions); instructions != "" {
			return instructions
		}
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
