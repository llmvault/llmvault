package mcpserver

import (
	"strings"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/model"
)

// callingProxyAgent resolves the active agent for a proxy token.
func callingProxyAgent(token *model.Token, db *gorm.DB) *model.Agent {
	if token == nil || db == nil || token.Meta == nil {
		return nil
	}
	if tokenType, _ := token.Meta[model.TokenMetaType].(string); tokenType != model.TokenTypeAgentProxy {
		return nil
	}
	agentIDText, _ := token.Meta[model.TokenMetaAgentID].(string)
	agentID, err := uuid.Parse(strings.TrimSpace(agentIDText))
	if err != nil || agentID == uuid.Nil {
		return nil
	}
	var agent model.Agent
	if err := db.Where("id = ? AND org_id = ? AND status <> ?", agentID, token.OrgID, "archived").First(&agent).Error; err != nil {
		return nil
	}
	return &agent
}
