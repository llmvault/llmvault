package agentschedule

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/model"
)

func ListForOrg(ctx context.Context, db *gorm.DB, orgID uuid.UUID) ([]model.AgentSchedule, error) {
	if db == nil {
		return nil, fmt.Errorf("database is required")
	}
	var rows []model.AgentSchedule
	if err := db.WithContext(ctx).
		Preload("Agent").
		Where("org_id = ? AND cancelled_at IS NULL", orgID).
		Order("created_at DESC").
		Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("list schedules: %w", err)
	}
	return rows, nil
}

// LoadByID loads a single schedule by its UUID within an org, with the owning
// Agent preloaded. Used by the REST get/update/delete endpoints.
func LoadByID(ctx context.Context, db *gorm.DB, orgID, id uuid.UUID) (*model.AgentSchedule, error) {
	if db == nil {
		return nil, fmt.Errorf("database is required")
	}
	var schedule model.AgentSchedule
	err := db.WithContext(ctx).
		Preload("Agent").
		Where("id = ? AND org_id = ?", id, orgID).
		First(&schedule).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, fmt.Errorf("schedule not found")
	}
	if err != nil {
		return nil, fmt.Errorf("load schedule: %w", err)
	}
	return &schedule, nil
}
