package model

import (
	"time"

	"github.com/google/uuid"
)

const (
	SlackThreadEventDirectionInbound  = "inbound"
	SlackThreadEventDirectionOutbound = "outbound"

	SlackThreadEventStatusReceived   = "received"
	SlackThreadEventStatusIgnored    = "ignored"
	SlackThreadEventStatusEnqueued   = "enqueued"
	SlackThreadEventStatusProcessing = "processing"
	SlackThreadEventStatusCompleted  = "completed"
	SlackThreadEventStatusFailed     = "failed"
	SlackThreadEventStatusSent       = "sent"
)

type SlackThreadEvent struct {
	ID                    uuid.UUID            `gorm:"type:uuid;primaryKey;default:gen_random_uuid()"`
	OrgID                 uuid.UUID            `gorm:"type:uuid;not null;index"`
	Org                   Org                  `gorm:"foreignKey:OrgID;constraint:OnDelete:CASCADE"`
	ConnectionID          uuid.UUID            `gorm:"type:uuid;not null;index"`
	Connection            Connection           `gorm:"foreignKey:ConnectionID;constraint:OnDelete:CASCADE"`
	ResolvedTeamID        *uuid.UUID           `gorm:"column:team_id;type:uuid;index"`
	ResolvedTeam          *Team                `gorm:"foreignKey:ResolvedTeamID;constraint:OnDelete:SET NULL"`
	AgentID               *uuid.UUID           `gorm:"type:uuid;index"`
	Agent                 *Agent               `gorm:"foreignKey:AgentID;constraint:OnDelete:SET NULL"`
	TriggerID             *uuid.UUID           `gorm:"type:uuid;index"`
	Trigger               *AgentTrigger        `gorm:"foreignKey:TriggerID;constraint:OnDelete:SET NULL"`
	SessionID             *uuid.UUID           `gorm:"type:uuid;index"`
	Session               *Session             `gorm:"foreignKey:SessionID;constraint:OnDelete:SET NULL"`
	SessionEventID        *uuid.UUID           `gorm:"type:uuid"`
	SessionEvent          *SessionEvent        `gorm:"foreignKey:SessionEventID;constraint:OnDelete:SET NULL"`
	SessionMessageQueueID *uuid.UUID           `gorm:"type:uuid"`
	SessionMessageQueue   *SessionMessageQueue `gorm:"foreignKey:SessionMessageQueueID;constraint:OnDelete:SET NULL"`
	SlackTeamID           string               `gorm:"column:slack_team_id;type:text;not null;default:''"`
	SlackChannelID        string               `gorm:"type:text;not null"`
	ThreadTS              string               `gorm:"type:text;not null"`
	MessageTS             string               `gorm:"type:text;not null"`
	MessageAt             time.Time            `gorm:"not null"`
	EventID               string               `gorm:"type:text;not null;default:''"`
	EventType             string               `gorm:"type:text;not null"`
	Direction             string               `gorm:"type:text;not null"`
	SenderID              string               `gorm:"type:text;not null;default:''"`
	Text                  string               `gorm:"type:text;not null;default:''"`
	Status                string               `gorm:"type:text;not null;default:'received'"`
	IgnoreReason          string               `gorm:"type:text;not null;default:''"`
	SlackReplyTS          string               `gorm:"type:text;not null;default:''"`
	RuntimeStreamID       string               `gorm:"type:text;not null;default:''"`
	RuntimeTurnID         string               `gorm:"type:text;not null;default:''"`
	Error                 string               `gorm:"type:text;not null;default:''"`
	Raw                   JSON                 `gorm:"type:jsonb;not null;default:'{}'"`
	ReceivedAt            time.Time            `gorm:"not null;default:now()"`
	StatusSetAt           *time.Time
	EnqueuedAt            *time.Time
	JobStartedAt          *time.Time
	RouteResolvedAt       *time.Time `gorm:"column:route_resolved_at"`
	SessionResolvedAt     *time.Time
	RuntimePostedAt       *time.Time
	FinalReceivedAt       *time.Time
	SlackReplySentAt      *time.Time
	CompletedAt           *time.Time
	FailedAt              *time.Time
	CreatedAt             time.Time
	UpdatedAt             time.Time
}

func (SlackThreadEvent) TableName() string { return "slack_thread_events" }
