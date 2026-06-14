package handler

import (
	"context"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/model"
)

func agentLockedSkillIDs(ctx context.Context, db *gorm.DB, orgID uuid.UUID, agent *model.Agent) (map[uuid.UUID]bool, error) {
	return map[uuid.UUID]bool{}, nil
}
