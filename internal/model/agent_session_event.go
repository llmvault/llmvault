package model

import (
	"time"

	"github.com/google/uuid"
)

type AgentSessionEvent struct {
	ID             uuid.UUID    `gorm:"type:uuid;primaryKey;default:gen_random_uuid()"`
	OrgID          uuid.UUID    `gorm:"type:uuid;not null;index:idx_agent_session_event_scope"`
	Org            Org          `gorm:"foreignKey:OrgID;constraint:OnDelete:CASCADE"`
	AgentID        uuid.UUID    `gorm:"type:uuid;not null;index:idx_agent_session_event_scope"`
	Agent          Agent        `gorm:"foreignKey:AgentID;constraint:OnDelete:CASCADE"`
	SandboxID      uuid.UUID    `gorm:"type:uuid;not null;index"`
	Sandbox        Sandbox      `gorm:"foreignKey:SandboxID;constraint:OnDelete:CASCADE"`
	AgentSessionID uuid.UUID    `gorm:"type:uuid;not null;index"`
	AgentSession   AgentSession `gorm:"foreignKey:AgentSessionID;constraint:OnDelete:CASCADE"`

	SessionID      string    `gorm:"column:runtime_session_id;not null;index:idx_agent_session_event_scope;size:255"`
	EventID        string    `gorm:"not null;default:'';index;size:255"`
	EventType      string    `gorm:"not null;index;size:128"`
	Source         string    `gorm:"not null;default:'manual';size:128"`
	Mode           string    `gorm:"not null;default:'agent';index;size:64"`
	SequenceNumber int64     `gorm:"not null;default:0;index"`
	Payload        RawJSON   `gorm:"type:jsonb;not null;default:'{}'"`
	EventAt        time.Time `gorm:"not null;index"`

	RetainedAt *time.Time `gorm:"index"`
	CreatedAt  time.Time
}

func (AgentSessionEvent) TableName() string { return "agent_session_events" }
