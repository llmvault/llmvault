package model

import (
	"time"

	"github.com/google/uuid"
)

type AgentChannel struct {
	ID        uuid.UUID `gorm:"type:uuid;primaryKey;default:gen_random_uuid()"`
	OrgID     uuid.UUID `gorm:"type:uuid;not null;index:idx_agent_channels_org_agent,priority:1"`
	Org       Org       `gorm:"foreignKey:OrgID;constraint:OnDelete:CASCADE"`
	AgentID   uuid.UUID `gorm:"type:uuid;not null;index:idx_agent_channels_org_agent,priority:2;uniqueIndex:idx_agent_channels_agent_channel,priority:1"`
	Agent     Agent     `gorm:"foreignKey:AgentID;constraint:OnDelete:CASCADE"`
	ChannelID uuid.UUID `gorm:"type:uuid;not null;index;uniqueIndex:idx_agent_channels_agent_channel,priority:2"`
	Channel   Channel   `gorm:"foreignKey:ChannelID;constraint:OnDelete:CASCADE"`
	CreatedAt time.Time
}

func (AgentChannel) TableName() string { return "agent_channels" }
