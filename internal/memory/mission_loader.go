package memory

import (
	"context"
	"errors"
	"strings"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/model"
)

// ChannelMission returns the channel's effective memory mission: the stored
// per-channel mission when set, otherwise the curated template for the
// channel's category ("" for 'general', which has none). The fallback keeps
// extraction and the settings UI consistent — an unset mission means "follow
// the category default", not "no mission" (clearing the mission in the UI
// resets to the default). A missing channel yields "" with no error so
// callers on the extraction path degrade to the base guidelines instead of
// failing the run. The lookup is scoped by org_id as well as the channel id so
// it upholds the tenancy invariant (every channel query carries org_id) rather
// than relying on the channel UUID alone.
func ChannelMission(ctx context.Context, db *gorm.DB, orgID, channelID uuid.UUID) (string, error) {
	var row struct {
		MemoryMission *string
		Category      string
	}
	err := db.WithContext(ctx).
		Model(&model.Channel{}).
		Select("memory_mission", "category").
		Where("id = ? AND org_id = ?", channelID, orgID).
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
