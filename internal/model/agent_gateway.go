package model

import (
	"time"

	"github.com/google/uuid"
)

type AgentGatewayRoute struct {
	ID uuid.UUID `gorm:"type:uuid;primaryKey;default:gen_random_uuid()"`

	OrgID uuid.UUID `gorm:"type:uuid;not null;index"`
	Org   Org       `gorm:"foreignKey:OrgID;constraint:OnDelete:CASCADE"`

	AgentID uuid.UUID `gorm:"type:uuid;not null;index"`
	Agent   Agent     `gorm:"foreignKey:AgentID;constraint:OnDelete:CASCADE"`

	ConnectionID *uuid.UUID  `gorm:"type:uuid;index"`
	Connection   *Connection `gorm:"foreignKey:ConnectionID;constraint:OnDelete:SET NULL"`

	Provider string `gorm:"not null;size:128;index"`
	Name     string `gorm:"type:text;not null;default:''"`
	Enabled  bool   `gorm:"not null;default:true;index"`
	Config   JSON   `gorm:"type:jsonb;not null;default:'{}'"`

	CreatedAt time.Time
	UpdatedAt time.Time
	RevokedAt *time.Time `gorm:"index"`
}

func (AgentGatewayRoute) TableName() string { return "agent_gateway_routes" }

type AgentGatewayEvent struct {
	ID uuid.UUID `gorm:"type:uuid;primaryKey;default:gen_random_uuid()"`

	OrgID   uuid.UUID  `gorm:"type:uuid;not null;index"`
	AgentID uuid.UUID  `gorm:"type:uuid;not null;index"`
	RouteID *uuid.UUID `gorm:"type:uuid;index"`

	AgentSessionID *uuid.UUID    `gorm:"type:uuid;index"`
	AgentSession   *AgentSession `gorm:"foreignKey:AgentSessionID;constraint:OnDelete:SET NULL"`

	Provider              string  `gorm:"not null;size:128;index"`
	ExternalMessageID     string  `gorm:"type:text;not null;default:''"`
	DedupeKey             string  `gorm:"type:text;not null;default:'';index"`
	ThreadKey             string  `gorm:"type:text;not null;default:'';index"`
	ChannelID             string  `gorm:"type:text;not null;default:''"`
	ThreadID              string  `gorm:"type:text;not null;default:''"`
	SenderID              string  `gorm:"type:text;not null;default:''"`
	Status                string  `gorm:"not null;default:'received';size:32;index"`
	Error                 string  `gorm:"type:text;not null;default:''"`
	RuntimeConversationID string  `gorm:"type:text;not null;default:''"`
	RuntimeSessionID      string  `gorm:"type:text;not null;default:''"`
	RuntimeStreamID       string  `gorm:"type:text;not null;default:''"`
	RuntimeTraceID        string  `gorm:"type:text;not null;default:''"`
	RuntimeTurnID         string  `gorm:"type:text;not null;default:''"`
	Payload               RawJSON `gorm:"type:jsonb;not null;default:'{}'"`

	ReceivedAt  time.Time `gorm:"not null;index"`
	ProcessedAt *time.Time
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

func (AgentGatewayEvent) TableName() string { return "agent_gateway_events" }

type AgentGatewayDelivery struct {
	ID uuid.UUID `gorm:"type:uuid;primaryKey;default:gen_random_uuid()"`

	OrgID          uuid.UUID  `gorm:"type:uuid;not null;index"`
	AgentID        uuid.UUID  `gorm:"type:uuid;not null;index"`
	RouteID        *uuid.UUID `gorm:"type:uuid;index"`
	AgentSessionID uuid.UUID  `gorm:"type:uuid;not null;index"`

	Provider         string  `gorm:"not null;size:128;index"`
	DedupeKey        string  `gorm:"type:text;not null;default:'';index"`
	RuntimeSessionID string  `gorm:"type:text;not null;default:''"`
	RuntimeTraceID   string  `gorm:"type:text;not null;default:''"`
	RuntimeTurnID    string  `gorm:"type:text;not null;default:''"`
	ThreadKey        string  `gorm:"type:text;not null;default:'';index"`
	ChannelID        string  `gorm:"type:text;not null;default:''"`
	ThreadID         string  `gorm:"type:text;not null;default:''"`
	ResponseText     string  `gorm:"type:text;not null;default:''"`
	ProviderHandles  RawJSON `gorm:"type:jsonb;not null;default:'[]'"`
	Status           string  `gorm:"not null;default:'sent';size:32;index"`
	Error            string  `gorm:"type:text;not null;default:''"`

	CreatedAt time.Time
	UpdatedAt time.Time
}

func (AgentGatewayDelivery) TableName() string { return "agent_gateway_deliveries" }
