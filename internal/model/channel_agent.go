package model

import (
	"time"

	"github.com/google/uuid"
)

// ChannelAgent assigns an agent to a channel. A channel may have many assigned
// agents; channels.default_agent_id is always one of them. A row here (or a
// system-kind channel) is what makes it legal to invoke the agent in the
// channel — see internal/channelagents.
type ChannelAgent struct {
	ID          uuid.UUID  `gorm:"type:uuid;primaryKey;default:gen_random_uuid()"`
	OrgID       uuid.UUID  `gorm:"type:uuid;not null;index:idx_channel_agents_org_agent,priority:1"`
	ChannelID   uuid.UUID  `gorm:"type:uuid;not null;uniqueIndex:channel_agents_channel_agent_unique,priority:1"`
	Channel     Channel    `gorm:"foreignKey:ChannelID;constraint:OnDelete:CASCADE"`
	AgentID     uuid.UUID  `gorm:"type:uuid;not null;index:idx_channel_agents_org_agent,priority:2;uniqueIndex:channel_agents_channel_agent_unique,priority:2"`
	Agent       Agent      `gorm:"foreignKey:AgentID;constraint:OnDelete:CASCADE"`
	AddedBy     *uuid.UUID `gorm:"type:uuid"`
	AddedByUser *User      `gorm:"foreignKey:AddedBy;constraint:OnDelete:SET NULL"`
	CreatedAt   time.Time
}

func (ChannelAgent) TableName() string { return "channel_agents" }
