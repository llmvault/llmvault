package model

import (
	"time"

	"github.com/google/uuid"
)

// AgentTriggerDelivery records a successful trigger injection into an agent
// runtime. AgentTrigger remains the source of truth for trigger configuration;
// this table stores only per-run correlation data.
type AgentTriggerDelivery struct {
	ID uuid.UUID `gorm:"type:uuid;primaryKey;default:gen_random_uuid()"`

	OrgID uuid.UUID `gorm:"type:uuid;not null;index:idx_trigger_delivery_org_agent_created;index:idx_trigger_delivery_org_agent_session_created,priority:1"`
	Org   Org       `gorm:"foreignKey:OrgID;constraint:OnDelete:CASCADE"`

	AgentID uuid.UUID `gorm:"type:uuid;not null;index:idx_trigger_delivery_org_agent_created;index:idx_trigger_delivery_org_agent_session_created,priority:2"`
	Agent   Agent     `gorm:"foreignKey:AgentID;constraint:OnDelete:CASCADE"`

	TriggerID uuid.UUID    `gorm:"type:uuid;not null;index;uniqueIndex:idx_agent_trigger_deliveries_trigger_delivery,priority:1"`
	Trigger   AgentTrigger `gorm:"foreignKey:TriggerID;constraint:OnDelete:CASCADE"`

	ConnectionID *uuid.UUID  `gorm:"type:uuid;index"`
	Connection   *Connection `gorm:"foreignKey:ConnectionID;constraint:OnDelete:SET NULL"`

	DeliveryID  string `gorm:"type:text;not null;index;uniqueIndex:idx_agent_trigger_deliveries_trigger_delivery,priority:2"`
	EventKey    string `gorm:"type:text;not null;default:'';index"`
	ResourceKey string `gorm:"type:text;not null;default:'';index"`

	SessionID        uuid.UUID `gorm:"type:uuid;not null;index"`
	Session          Session   `gorm:"foreignKey:SessionID;constraint:OnDelete:CASCADE"`
	RuntimeSessionID string    `gorm:"type:text;not null;default:'';index;index:idx_trigger_delivery_org_agent_session_created,priority:3"`
	RuntimeStreamID  string    `gorm:"type:text;not null;default:''"`
	RuntimeTraceID   string    `gorm:"type:text;not null;default:''"`
	RuntimeTurnID    string    `gorm:"type:text;not null;default:''"`

	Payload RawJSON `gorm:"type:jsonb;not null;default:'{}'"`

	CreatedAt time.Time `gorm:"index:idx_trigger_delivery_org_agent_created;index:idx_trigger_delivery_org_agent_session_created,priority:4"`
}

func (AgentTriggerDelivery) TableName() string { return "agent_trigger_deliveries" }
