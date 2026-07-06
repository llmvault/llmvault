package memory

import (
	"context"
	"errors"
	"strings"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/model"
)

// ChannelMission returns the channel's memory mission, "" when unset. A
// missing channel also yields "" with no error so callers on the extraction
// path degrade to the base guidelines instead of failing the run.
func ChannelMission(ctx context.Context, db *gorm.DB, channelID uuid.UUID) (string, error) {
	var row struct {
		MemoryMission *string
	}
	err := db.WithContext(ctx).
		Model(&model.Channel{}).
		Select("memory_mission").
		Where("id = ?", channelID).
		Take(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	if row.MemoryMission == nil {
		return "", nil
	}
	return strings.TrimSpace(*row.MemoryMission), nil
}
