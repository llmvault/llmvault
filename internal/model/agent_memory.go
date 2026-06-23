package model

import (
	"time"

	"github.com/google/uuid"
	"github.com/lib/pq"
)

const (
	AgentMemoryScopeOrg  = "org"
	AgentMemoryScopeUser = "user"

	AgentMemoryEmbeddingPending = "pending"
	AgentMemoryEmbeddingReady   = "ready"
	AgentMemoryEmbeddingFailed  = "failed"
)

type AgentMemory struct {
	ID                uuid.UUID      `gorm:"type:uuid;primaryKey;default:gen_random_uuid()"`
	OrgID             uuid.UUID      `gorm:"type:uuid;not null;index"`
	Org               Org            `gorm:"foreignKey:OrgID;constraint:OnDelete:CASCADE"`
	AgentID           *uuid.UUID     `gorm:"type:uuid;index"`
	Agent             *Agent         `gorm:"foreignKey:AgentID;constraint:OnDelete:SET NULL"`
	UserID            *uuid.UUID     `gorm:"type:uuid;index"`
	User              *User          `gorm:"foreignKey:UserID;constraint:OnDelete:CASCADE"`
	Scope             string         `gorm:"type:text;not null"`
	Content           string         `gorm:"type:text;not null"`
	Tags              pq.StringArray `gorm:"type:text[];not null;default:'{}'"`
	Metadata          JSON           `gorm:"type:jsonb;not null;default:'{}'"`
	EmbeddingModel    string         `gorm:"type:text;not null;default:''"`
	EmbeddingStatus   string         `gorm:"type:text;not null;default:'pending'"`
	EmbeddingRevision int            `gorm:"not null;default:1"`
	EmbeddingError    string         `gorm:"type:text;not null;default:''"`
	EmbeddedAt        *time.Time
	SourceSessionID   *uuid.UUID    `gorm:"type:uuid"`
	SourceSession     *Session      `gorm:"foreignKey:SourceSessionID;constraint:OnDelete:SET NULL"`
	SourceEventID     *uuid.UUID    `gorm:"type:uuid"`
	SourceEvent       *SessionEvent `gorm:"foreignKey:SourceEventID;constraint:OnDelete:SET NULL"`
	CreatedByUserID   *uuid.UUID    `gorm:"type:uuid"`
	CreatedBy         *User         `gorm:"foreignKey:CreatedByUserID;constraint:OnDelete:SET NULL"`
	ArchivedAt        *time.Time
	CreatedAt         time.Time
	UpdatedAt         time.Time
}

func (AgentMemory) TableName() string { return "agent_memories" }
