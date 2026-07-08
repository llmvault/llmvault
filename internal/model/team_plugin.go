package model

import (
	"time"

	"github.com/google/uuid"
)

// TeamPlugin provisions a plugin to a team. Teams are the provisioning unit:
// a team's agents may install only the plugins granted to the team here. The
// set of rows for a team is its plugin allowlist. Unique per (team, plugin).
type TeamPlugin struct {
	ID            uuid.UUID  `gorm:"type:uuid;primaryKey;default:gen_random_uuid()"`
	OrgID         uuid.UUID  `gorm:"type:uuid;not null;index:idx_team_plugins_org_team,priority:1"`
	TeamID        uuid.UUID  `gorm:"type:uuid;not null;index:idx_team_plugins_org_team,priority:2;uniqueIndex:team_plugins_team_plugin_unique,priority:1"`
	Team          Team       `gorm:"foreignKey:TeamID;constraint:OnDelete:CASCADE"`
	PluginID      uuid.UUID  `gorm:"type:uuid;not null;uniqueIndex:team_plugins_team_plugin_unique,priority:2"`
	Plugin        Plugin     `gorm:"foreignKey:PluginID;constraint:OnDelete:CASCADE"`
	EnabledBy     *uuid.UUID `gorm:"type:uuid"`
	EnabledByUser *User      `gorm:"foreignKey:EnabledBy;constraint:OnDelete:SET NULL"`
	CreatedAt     time.Time
}

func (TeamPlugin) TableName() string { return "team_plugins" }
