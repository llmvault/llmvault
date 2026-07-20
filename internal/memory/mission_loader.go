package memory

import (
	"context"
	"errors"
	"strings"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/model"
)

// AgentMission returns an agent's explicit memory mission or its catalog
// category template. A missing agent degrades to the base extraction prompt.
func AgentMission(ctx context.Context, db *gorm.DB, orgID, agentID uuid.UUID) (string, error) {
	var row struct {
		MemoryMission *string
		Category      string
	}
	err := db.WithContext(ctx).
		Model(&model.Agent{}).
		Joins("LEFT JOIN agent_catalog ON agent_catalog.id = agents.agent_catalog_id").
		Select("agents.memory_mission", "COALESCE(agent_catalog.category, '') AS category").
		Where("agents.id = ? AND agents.org_id = ?", agentID, orgID).
		Take(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	if row.MemoryMission != nil {
		if mission := strings.TrimSpace(*row.MemoryMission); mission != "" {
			return mission, nil
		}
	}
	return MissionTemplate(row.Category), nil
}
