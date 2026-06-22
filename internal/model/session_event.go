package model

import (
	"time"

	"github.com/google/uuid"
)

type SessionEvent struct {
	ID               uuid.UUID  `gorm:"type:uuid;primaryKey;default:gen_random_uuid()"`
	OrgID            uuid.UUID  `gorm:"type:uuid;not null;index"`
	Org              Org        `gorm:"foreignKey:OrgID;constraint:OnDelete:CASCADE"`
	SessionID        uuid.UUID  `gorm:"type:uuid;not null;index"`
	Session          Session    `gorm:"foreignKey:SessionID;constraint:OnDelete:CASCADE"`
	AgentID          uuid.UUID  `gorm:"type:uuid;not null;index"`
	Agent            Agent      `gorm:"foreignKey:AgentID;constraint:OnDelete:RESTRICT"`
	SandboxID        *uuid.UUID `gorm:"type:uuid;index"`
	Sandbox          *Sandbox   `gorm:"foreignKey:SandboxID;constraint:OnDelete:SET NULL"`
	RuntimeSessionID string     `gorm:"type:text"`
	EventID          string     `gorm:"type:text"`
	EventType        string     `gorm:"type:text;not null"`
	ActorUserID      *uuid.UUID `gorm:"type:uuid"`
	Actor            *User      `gorm:"foreignKey:ActorUserID;constraint:OnDelete:SET NULL"`
	Source           string     `gorm:"type:text;not null;default:'web'"`
	SequenceNumber   int64
	RuntimeSeq       *int64    `gorm:"type:bigint"`
	RuntimeEventID   string    `gorm:"type:text"`
	TurnID           string    `gorm:"type:text"`
	SpanID           string    `gorm:"type:text"`
	Durability       string    `gorm:"type:text"`
	Payload          JSON      `gorm:"type:jsonb;not null;default:'{}'"`
	EventAt          time.Time `gorm:"not null"`
	RetainedAt       *time.Time
	CreatedAt        time.Time
}

func (SessionEvent) TableName() string { return "session_events" }

type SessionMessageQueue struct {
	ID               uuid.UUID     `gorm:"type:uuid;primaryKey;default:gen_random_uuid()"`
	OrgID            uuid.UUID     `gorm:"type:uuid;not null;index"`
	SessionID        uuid.UUID     `gorm:"type:uuid;not null;uniqueIndex:idx_session_message_queue_sequence,priority:1;uniqueIndex:idx_session_message_queue_session_event,priority:1"`
	Session          Session       `gorm:"foreignKey:SessionID;constraint:OnDelete:CASCADE"`
	SessionEventID   *uuid.UUID    `gorm:"type:uuid;uniqueIndex:idx_session_message_queue_session_event,priority:2,where:session_event_id IS NOT NULL"`
	SessionEvent     *SessionEvent `gorm:"foreignKey:SessionEventID;constraint:OnDelete:SET NULL"`
	ActorUserID      *uuid.UUID    `gorm:"type:uuid"`
	Actor            *User         `gorm:"foreignKey:ActorUserID;constraint:OnDelete:SET NULL"`
	MessageText      string        `gorm:"type:text;not null;default:''"`
	MessagePayload   JSON          `gorm:"type:jsonb;not null;default:'{}'"`
	Model            string        `gorm:"type:text;not null;default:''"`
	ReasoningEffort  string        `gorm:"type:text;not null;default:''"`
	SequenceNumber   int64         `gorm:"not null;uniqueIndex:idx_session_message_queue_sequence,priority:2"`
	Status           string        `gorm:"type:text;not null;default:'pending'"`
	AttemptCount     int           `gorm:"not null;default:0"`
	LeasedBy         string        `gorm:"type:text"`
	LeasedUntil      *time.Time
	DeliveredAt      *time.Time
	LastError        string `gorm:"type:text;not null;default:''"`
	RuntimeStreamID  string `gorm:"type:text;not null;default:''"`
	RuntimeStreamURL string `gorm:"type:text;not null;default:''"`
	RuntimeTraceID   string `gorm:"type:text;not null;default:''"`
	RuntimeTurnID    string `gorm:"type:text;not null;default:''"`
	CreatedAt        time.Time
	UpdatedAt        time.Time
}

func (SessionMessageQueue) TableName() string { return "session_message_queue" }
