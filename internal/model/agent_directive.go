package model

import (
	"time"

	"github.com/google/uuid"
)

const (
	// DirectiveSourceUserPinned marks a directive written by hand.
	DirectiveSourceUserPinned = "user-pinned"
	// DirectiveSourceExtractedConfirmed marks a directive promoted from an
	// extracted observation on explicit user confirmation (pin).
	DirectiveSourceExtractedConfirmed = "extracted-confirmed"
)

// AgentDirective is a hard rule injected verbatim into every agent prompt in
// scope — never ranked or trimmed, so the bar for existence is explicit human
// action. ChannelID scopes it: set = that channel only, NULL = org-wide.
type AgentDirective struct {
	ID              uuid.UUID  `gorm:"type:uuid;primaryKey;default:gen_random_uuid()"`
	OrgID           uuid.UUID  `gorm:"type:uuid;not null;index"`
	Org             Org        `gorm:"foreignKey:OrgID;constraint:OnDelete:CASCADE"`
	ChannelID       *uuid.UUID `gorm:"type:uuid;index"`
	Channel         *Channel   `gorm:"foreignKey:ChannelID;constraint:OnDelete:CASCADE"`
	Content         string     `gorm:"type:text;not null"`
	CreatedByUserID *uuid.UUID `gorm:"type:uuid"`
	CreatedBy       *User      `gorm:"foreignKey:CreatedByUserID;constraint:OnDelete:SET NULL"`
	Source          string     `gorm:"type:text;not null;default:'user-pinned'"`
	Active          bool       `gorm:"not null;default:true"`
	// DeletedAt soft-deletes the rule. Content is immutable, so deleted rows
	// are kept verbatim as history — "which rules were in force at time T"
	// stays answerable for as-of temporal audits. Every read path filters
	// deleted_at IS NULL.
	DeletedAt *time.Time
	CreatedAt time.Time
	UpdatedAt time.Time
}

func (AgentDirective) TableName() string { return "agent_directives" }
