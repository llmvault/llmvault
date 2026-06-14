package model

import (
	"time"

	"github.com/google/uuid"
)

type Plugin struct {
	ID          uuid.UUID `gorm:"type:uuid;primaryKey;default:gen_random_uuid()"`
	Slug        string    `gorm:"not null;uniqueIndex"`
	Name        string    `gorm:"not null"`
	Description string    `gorm:"type:text;not null;default:''"`
	Category    string    `gorm:"not null;default:'';size:64;index"`
	Icon        string    `gorm:"not null;default:''"`
	IconColor   string    `gorm:"not null;default:''"`
	Developer   string    `gorm:"not null;default:'Hivy'"`
	Version     string    `gorm:"not null;default:'1'"`
	Status      string    `gorm:"not null;default:'active';index"`
	SourceHash  string    `gorm:"not null;default:''"`
	Manifest    RawJSON   `gorm:"type:jsonb;not null;default:'{}'"`
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

func (Plugin) TableName() string { return "plugins" }

const (
	PluginStatusActive   = "active"
	PluginStatusArchived = "archived"
)

type PluginIntegration struct {
	PluginID  uuid.UUID `gorm:"type:uuid;primaryKey"`
	Plugin    Plugin    `gorm:"foreignKey:PluginID;constraint:OnDelete:CASCADE"`
	Provider  string    `gorm:"not null;primaryKey"`
	Kind      string    `gorm:"not null;default:'integration';primaryKey"`
	Required  bool      `gorm:"not null;default:true"`
	CreatedAt time.Time
}

func (PluginIntegration) TableName() string { return "plugin_integrations" }

const (
	PluginIntegrationKindIntegration = "integration"
	PluginIntegrationKindDatabase    = "database"
)

type OrgPluginInstall struct {
	ID              uuid.UUID  `gorm:"type:uuid;primaryKey;default:gen_random_uuid()"`
	OrgID           uuid.UUID  `gorm:"type:uuid;not null;index"`
	Org             Org        `gorm:"foreignKey:OrgID;constraint:OnDelete:CASCADE"`
	PluginID        uuid.UUID  `gorm:"type:uuid;not null;index"`
	Plugin          Plugin     `gorm:"foreignKey:PluginID;constraint:OnDelete:CASCADE"`
	CreatedByUserID *uuid.UUID `gorm:"type:uuid;index"`
	CreatedByUser   *User      `gorm:"foreignKey:CreatedByUserID;constraint:OnDelete:SET NULL"`
	RevokedAt       *time.Time `gorm:"index"`
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

func (OrgPluginInstall) TableName() string { return "org_plugin_installs" }

type AgentPluginInstall struct {
	OrgID     uuid.UUID `gorm:"type:uuid;not null;index"`
	Org       Org       `gorm:"foreignKey:OrgID;constraint:OnDelete:CASCADE"`
	AgentID   uuid.UUID `gorm:"type:uuid;primaryKey"`
	Agent     Agent     `gorm:"foreignKey:AgentID;constraint:OnDelete:CASCADE"`
	PluginID  uuid.UUID `gorm:"type:uuid;primaryKey"`
	Plugin    Plugin    `gorm:"foreignKey:PluginID;constraint:OnDelete:CASCADE"`
	CreatedAt time.Time
}

func (AgentPluginInstall) TableName() string { return "agent_plugin_installs" }
