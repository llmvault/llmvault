package model

import (
	"time"

	"github.com/google/uuid"
)

type Channel struct {
	ID                   uuid.UUID   `gorm:"type:uuid;primaryKey;default:gen_random_uuid()"`
	OrgID                uuid.UUID   `gorm:"type:uuid;not null;index;uniqueIndex:idx_channels_org_source_name,priority:1,where:archived_at IS NULL;uniqueIndex:idx_channels_org_external_resource,priority:1,where:external_resource_key <> ''"`
	Org                  Org         `gorm:"foreignKey:OrgID;constraint:OnDelete:CASCADE"`
	Name                 string      `gorm:"type:text;not null;uniqueIndex:idx_channels_org_source_name,priority:6,where:archived_at IS NULL"`
	Description          string      `gorm:"type:text;not null;default:''"`
	Kind                 string      `gorm:"type:text;not null;default:'standard'"`
	Visibility           string      `gorm:"type:text;not null;default:'public'"`
	TeamID               *uuid.UUID  `gorm:"type:uuid;index"`
	Team                 *Team       `gorm:"foreignKey:TeamID;constraint:OnDelete:SET NULL"`
	DefaultAgentID       uuid.UUID   `gorm:"type:uuid;not null;index"`
	DefaultAgent         Agent       `gorm:"foreignKey:DefaultAgentID;constraint:OnDelete:RESTRICT"`
	IsDefault            bool        `gorm:"not null;default:false;index"`
	Origin               string      `gorm:"type:text;not null;default:'native';index;uniqueIndex:idx_channels_org_source_name,priority:2,where:archived_at IS NULL"`
	ExternalProvider     string      `gorm:"type:text;not null;default:'';uniqueIndex:idx_channels_org_source_name,priority:3,where:archived_at IS NULL;uniqueIndex:idx_channels_org_external_resource,priority:2,where:external_resource_key <> ''"`
	ExternalConnectionID *uuid.UUID  `gorm:"type:uuid;index"`
	ExternalConnection   *Connection `gorm:"foreignKey:ExternalConnectionID;constraint:OnDelete:SET NULL"`
	ExternalWorkspaceKey string      `gorm:"type:text;not null;default:'';uniqueIndex:idx_channels_org_source_name,priority:4,where:archived_at IS NULL;uniqueIndex:idx_channels_org_external_resource,priority:3,where:external_resource_key <> ''"`
	ExternalResourceType string      `gorm:"type:text;not null;default:'channel';uniqueIndex:idx_channels_org_source_name,priority:5,where:archived_at IS NULL;uniqueIndex:idx_channels_org_external_resource,priority:4,where:external_resource_key <> ''"`
	ExternalResourceKey  string      `gorm:"type:text;not null;default:'';uniqueIndex:idx_channels_org_external_resource,priority:5,where:external_resource_key <> ''"`
	ExternalResourceName string      `gorm:"type:text;not null;default:''"`
	ExternalResourceURL  string      `gorm:"type:text;not null;default:''"`
	ExternalMetadata     JSON        `gorm:"type:jsonb;not null;default:'{}'"`
	CreatedBy            *uuid.UUID  `gorm:"type:uuid"`
	Creator              *User       `gorm:"foreignKey:CreatedBy;constraint:OnDelete:SET NULL"`
	ArchivedAt           *time.Time  `gorm:"index"`
	CreatedAt            time.Time
	UpdatedAt            time.Time
}

func (Channel) TableName() string { return "channels" }

type ChannelMember struct {
	ID        uuid.UUID `gorm:"type:uuid;primaryKey;default:gen_random_uuid()"`
	ChannelID uuid.UUID `gorm:"type:uuid;not null;uniqueIndex:idx_channel_members_channel_user,priority:1"`
	Channel   Channel   `gorm:"foreignKey:ChannelID;constraint:OnDelete:CASCADE"`
	UserID    uuid.UUID `gorm:"type:uuid;not null;uniqueIndex:idx_channel_members_channel_user,priority:2"`
	User      User      `gorm:"foreignKey:UserID;constraint:OnDelete:CASCADE"`
	Role      string    `gorm:"type:text;not null;default:'member'"`
	CreatedAt time.Time
}

func (ChannelMember) TableName() string { return "channel_members" }
