package model

import (
	"time"

	"github.com/google/uuid"
)

type AgentAsset struct {
	ID        uuid.UUID `gorm:"type:uuid;primaryKey;default:gen_random_uuid()"`
	OrgID     uuid.UUID `gorm:"type:uuid;not null;index"`
	Org       Org       `gorm:"foreignKey:OrgID;constraint:OnDelete:CASCADE"`
	AgentID   uuid.UUID `gorm:"type:uuid;not null;index:idx_emp_asset_agent_created,priority:1"`
	Agent     Agent     `gorm:"foreignKey:AgentID;constraint:OnDelete:CASCADE"`
	// SandboxID is nil for assets uploaded before a session exists (new-chat
	// composer attachments).
	SandboxID *uuid.UUID `gorm:"type:uuid"`
	Sandbox   *Sandbox   `gorm:"foreignKey:SandboxID;constraint:OnDelete:CASCADE"`

	Path        string   `gorm:"type:text;not null"`
	Filename    string   `gorm:"type:text;not null"`
	Key         string   `gorm:"type:text;not null;uniqueIndex"`
	PublicURL   string   `gorm:"type:text;not null"`
	ContentType string   `gorm:"type:text;not null"`
	Bytes       int64    `gorm:"not null"`
	Description *RawJSON `gorm:"type:jsonb" json:"description,omitempty"`

	CreatedAt time.Time `gorm:"index:idx_emp_asset_agent_created,priority:2,sort:desc"`
	UpdatedAt time.Time
}

func (AgentAsset) TableName() string { return "agent_assets" }
