package teamprovision

import (
	"context"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/usehivy/hivy/internal/model"
)

// EnablePlugin adds pluginID to teamID's plugin allowlist. It is idempotent:
// enabling an already-enabled plugin is a no-op success. It validates that the
// team belongs to orgID, that the plugin is visible to the org (global or
// org-owned), and that the org has installed the plugin. enabledBy records the
// acting user (nil for API-key callers).
//
// Errors: ErrTeamNotFound, ErrPluginNotFound, ErrPluginNotInstalled.
func EnablePlugin(ctx context.Context, db *gorm.DB, orgID, teamID, pluginID uuid.UUID, enabledBy *uuid.UUID) error {
	ok, err := teamInOrg(ctx, db, orgID, teamID)
	if err != nil {
		return err
	}
	if !ok {
		return ErrTeamNotFound
	}
	if err := validatePluginForOrg(ctx, db, orgID, pluginID); err != nil {
		return err
	}
	row := model.TeamPlugin{
		OrgID:     orgID,
		TeamID:    teamID,
		PluginID:  pluginID,
		EnabledBy: enabledBy,
	}
	// DoNothing keeps the original enabled_by/created_at on a repeat enable.
	return db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "team_id"}, {Name: "plugin_id"}},
		DoNothing: true,
	}).Create(&row).Error
}

// DisablePlugin removes pluginID from teamID's plugin allowlist. Idempotent:
// disabling a plugin that is not enabled is a no-op success. It still validates
// the team belongs to orgID so a caller cannot probe other orgs.
func DisablePlugin(ctx context.Context, db *gorm.DB, orgID, teamID, pluginID uuid.UUID) error {
	ok, err := teamInOrg(ctx, db, orgID, teamID)
	if err != nil {
		return err
	}
	if !ok {
		return ErrTeamNotFound
	}
	return db.WithContext(ctx).
		Where("org_id = ? AND team_id = ? AND plugin_id = ?", orgID, teamID, pluginID).
		Delete(&model.TeamPlugin{}).Error
}

// validatePluginForOrg checks the plugin is visible to the org (global or
// org-owned) and installed by the org.
func validatePluginForOrg(ctx context.Context, db *gorm.DB, orgID, pluginID uuid.UUID) error {
	var visible int64
	if err := db.WithContext(ctx).Model(&model.Plugin{}).
		Where("id = ? AND (org_id IS NULL OR org_id = ?)", pluginID, orgID).
		Count(&visible).Error; err != nil {
		return err
	}
	if visible == 0 {
		return ErrPluginNotFound
	}
	var installed int64
	if err := db.WithContext(ctx).Model(&model.OrgPluginInstall{}).
		Where("org_id = ? AND plugin_id = ? AND revoked_at IS NULL", orgID, pluginID).
		Count(&installed).Error; err != nil {
		return err
	}
	if installed == 0 {
		return ErrPluginNotInstalled
	}
	return nil
}
