package model

import (
	"time"

	"github.com/google/uuid"
)

// AgentPluginOverride records a user's decision to disable one optional
// team-provisioned plugin for one agent. Team plugins remain inherited by
// default; an override only subtracts a currently granted team plugin and
// never affects implicit auto-install or default-agent plugins.
type AgentPluginOverride struct {
	OrgID      uuid.UUID  `gorm:"type:uuid;not null;index"`
	Org        Org        `gorm:"foreignKey:OrgID;constraint:OnDelete:CASCADE"`
	AgentID    uuid.UUID  `gorm:"type:uuid;not null;primaryKey"`
	Agent      Agent      `gorm:"foreignKey:AgentID;constraint:OnDelete:CASCADE"`
	PluginID   uuid.UUID  `gorm:"type:uuid;not null;primaryKey;index"`
	Plugin     Plugin     `gorm:"foreignKey:PluginID;constraint:OnDelete:CASCADE"`
	DisabledBy *uuid.UUID `gorm:"type:uuid"`
	User       *User      `gorm:"foreignKey:DisabledBy;constraint:OnDelete:SET NULL"`
	CreatedAt  time.Time
}

func (AgentPluginOverride) TableName() string { return "agent_plugin_overrides" }
