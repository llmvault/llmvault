package tasks

import (
	"context"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/model"
)

func loadAgentSandboxByID(ctx context.Context, db *gorm.DB, orgID, agentID, sandboxID uuid.UUID) (*model.Sandbox, error) {
	var sb model.Sandbox
	if err := db.WithContext(ctx).
		Where("id = ? AND org_id = ? AND agent_id = ?", sandboxID, orgID, agentID).
		First(&sb).Error; err != nil {
		return nil, err
	}
	return &sb, nil
}
