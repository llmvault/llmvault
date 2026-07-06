package model

import (
	"time"

	"github.com/google/uuid"
)

// MemorySuppression records the content fingerprint of a memory the user
// deleted. Consolidation checks these fingerprints before creating an
// observation so deleted content cannot be resurrected by later facts.
// ChannelID scopes the suppression: set = that channel, NULL = org-wide.
type MemorySuppression struct {
	ID                 uuid.UUID  `gorm:"type:uuid;primaryKey;default:gen_random_uuid()"`
	OrgID              uuid.UUID  `gorm:"type:uuid;not null"`
	Org                Org        `gorm:"foreignKey:OrgID;constraint:OnDelete:CASCADE"`
	ChannelID          *uuid.UUID `gorm:"type:uuid"`
	Channel            *Channel   `gorm:"foreignKey:ChannelID;constraint:OnDelete:CASCADE"`
	ContentFingerprint string     `gorm:"type:text;not null"`
	CreatedAt          time.Time
}

func (MemorySuppression) TableName() string { return "memory_suppressions" }
