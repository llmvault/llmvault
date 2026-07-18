package model

import (
	"time"

	"github.com/google/uuid"
)

// TeamExternalResourceRoute deterministically maps one provider resource from
// a connected account to an agent in a team. ConnectionID identifies the
// provider/workspace; resource type and key are provider-defined, stable
// identifiers (for example, slack_channel/C123).
type TeamExternalResourceRoute struct {
	ID           uuid.UUID  `gorm:"type:uuid;primaryKey;default:gen_random_uuid()"`
	OrgID        uuid.UUID  `gorm:"type:uuid;not null;index"`
	Org          Org        `gorm:"foreignKey:OrgID;constraint:OnDelete:CASCADE"`
	TeamID       uuid.UUID  `gorm:"type:uuid;not null;index"`
	Team         Team       `gorm:"foreignKey:TeamID;constraint:OnDelete:CASCADE"`
	ConnectionID uuid.UUID  `gorm:"type:uuid;not null;uniqueIndex:idx_team_external_resource_routes_resource,priority:1"`
	Connection   Connection `gorm:"foreignKey:ConnectionID;constraint:OnDelete:CASCADE"`
	AgentID      uuid.UUID  `gorm:"type:uuid;not null;index"`
	Agent        Agent      `gorm:"foreignKey:AgentID;constraint:OnDelete:RESTRICT"`
	ResourceType string     `gorm:"type:text;not null;uniqueIndex:idx_team_external_resource_routes_resource,priority:2"`
	ResourceKey  string     `gorm:"type:text;not null;uniqueIndex:idx_team_external_resource_routes_resource,priority:3"`
	ResourceName string     `gorm:"type:text;not null;default:''"`
	ResourceURL  string     `gorm:"type:text;not null;default:''"`
	Metadata     JSON       `gorm:"type:jsonb;not null;default:'{}'"`
	CreatedBy    *uuid.UUID `gorm:"type:uuid"`
	Creator      *User      `gorm:"foreignKey:CreatedBy;constraint:OnDelete:SET NULL"`
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

func (TeamExternalResourceRoute) TableName() string { return "team_external_resource_routes" }
