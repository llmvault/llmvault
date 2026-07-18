package model

import (
	"time"

	"github.com/google/uuid"
)

const (
	AgentEmailDirectionInbound  = "inbound"
	AgentEmailDirectionOutbound = "outbound"
	AgentEmailStatusReceived    = "received"
	AgentEmailStatusQueued      = "queued"
	AgentEmailStatusSent        = "sent"
	AgentEmailStatusFailed      = "failed"
)

// AgentEmailThread is the durable correlation boundary between RFC email and
// a Hivy session. ReplyToken is an opaque fallback when clients omit headers.
type AgentEmailThread struct {
	ID            uuid.UUID  `gorm:"type:uuid;primaryKey;default:gen_random_uuid()"`
	OrgID         uuid.UUID  `gorm:"type:uuid;not null;index"`
	AgentID       uuid.UUID  `gorm:"type:uuid;not null;index"`
	SessionID     *uuid.UUID `gorm:"type:uuid;index"`
	RootMessageID string     `gorm:"type:text;not null;default:''"`
	ReplyToken    string     `gorm:"type:text;not null;uniqueIndex"`
	LastMessageAt time.Time  `gorm:"not null"`
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

func (AgentEmailThread) TableName() string { return "agent_email_threads" }

// AgentEmailMessage keeps the normalized message and the Resend identifiers
// needed for delivery state, RFC threading, and idempotency.
type AgentEmailMessage struct {
	ID            uuid.UUID `gorm:"type:uuid;primaryKey;default:gen_random_uuid()"`
	OrgID         uuid.UUID `gorm:"type:uuid;not null;index"`
	AgentID       uuid.UUID `gorm:"type:uuid;not null;index"`
	ThreadID      uuid.UUID `gorm:"type:uuid;not null;index"`
	Direction     string    `gorm:"type:text;not null"`
	Status        string    `gorm:"type:text;not null;default:'received'"`
	ResendEmailID string    `gorm:"type:text;not null;default:'';uniqueIndex:idx_agent_email_messages_resend_id,where:resend_email_id <> ''"`
	MessageID     string    `gorm:"type:text;not null;default:'';index"`
	InReplyTo     string    `gorm:"type:text;not null;default:''"`
	References    RawJSON   `gorm:"type:jsonb;not null;default:'[]'"`
	FromAddress   string    `gorm:"type:text;not null;default:''"`
	ToAddresses   RawJSON   `gorm:"type:jsonb;not null;default:'[]'"`
	CCAddresses   RawJSON   `gorm:"type:jsonb;not null;default:'[]'"`
	Subject       string    `gorm:"type:text;not null;default:''"`
	TextBody      string    `gorm:"type:text;not null;default:''"`
	HTMLBody      string    `gorm:"type:text;not null;default:''"`
	Headers       RawJSON   `gorm:"type:jsonb;not null;default:'{}'"`
	ProviderAt    time.Time `gorm:"not null"`
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

func (AgentEmailMessage) TableName() string { return "agent_email_messages" }

// AgentEmailWebhookReceipt makes Resend's at-least-once delivery safe before
// expensive content retrieval is scheduled.
type AgentEmailWebhookReceipt struct {
	SvixID        string  `gorm:"type:text;primaryKey"`
	EventType     string  `gorm:"type:text;not null"`
	ResendEmailID string  `gorm:"type:text;not null;default:'';index"`
	Payload       RawJSON `gorm:"type:jsonb;not null;default:'{}'"`
	ProcessedAt   *time.Time
	CreatedAt     time.Time
}

func (AgentEmailWebhookReceipt) TableName() string { return "agent_email_webhook_receipts" }
