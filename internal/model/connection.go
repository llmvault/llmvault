package model

import (
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type Connection struct {
	ID                uuid.UUID   `gorm:"type:uuid;primaryKey;default:gen_random_uuid()"`
	OrgID             uuid.UUID   `gorm:"type:uuid;index"`
	Org               Org         `gorm:"foreignKey:OrgID;constraint:OnDelete:CASCADE"`
	UserID            uuid.UUID   `gorm:"type:uuid;not null;index"`
	User              User        `gorm:"foreignKey:UserID;constraint:OnDelete:CASCADE"`
	IntegrationID     uuid.UUID   `gorm:"type:uuid;not null;index"`
	Integration       Integration `gorm:"foreignKey:IntegrationID;constraint:OnDelete:CASCADE"`
	NangoConnectionID string      `gorm:"not null"`
	Name              string      `gorm:"type:text;not null;default:''"`
	Slug              string      `gorm:"type:text;not null;default:'';index"`
	NeedsName         bool        `gorm:"not null;default:false"`
	Meta              JSON        `gorm:"type:jsonb;default:'{}'"`
	WebhookConfigured *bool       `gorm:"not null;default:true"`
	RevokedAt         *time.Time
	CreatedAt         time.Time
	UpdatedAt         time.Time
}

func (Connection) TableName() string { return "connections" }

// BeforeCreate keeps non-handler writers (tests, imports, maintenance jobs)
// from inserting an empty identity that would collide under the active slug
// index. Product creation paths assign the provider-aware identity first.
func (c *Connection) BeforeCreate(_ *gorm.DB) error {
	if c.ID == uuid.Nil {
		c.ID = uuid.New()
	}
	if c.Slug == "" {
		c.Slug = strings.ReplaceAll(c.ID.String(), "-", "")[:6]
		c.NeedsName = true
	}
	if c.Name == "" {
		c.Name = c.Slug
	}
	return nil
}

// TeamConnectionGrant gives a team access to one concrete connection instance.
// Exactly one of ConnectionID and DatabaseConnectionID is set.
type TeamConnectionGrant struct {
	ID                   uuid.UUID           `gorm:"type:uuid;primaryKey;default:gen_random_uuid()"`
	OrgID                uuid.UUID           `gorm:"type:uuid;not null;index"`
	Org                  Org                 `gorm:"foreignKey:OrgID;constraint:OnDelete:CASCADE"`
	TeamID               uuid.UUID           `gorm:"type:uuid;not null;index"`
	Team                 Team                `gorm:"foreignKey:TeamID;constraint:OnDelete:CASCADE"`
	ConnectionID         *uuid.UUID          `gorm:"type:uuid;index"`
	Connection           *Connection         `gorm:"foreignKey:ConnectionID;constraint:OnDelete:CASCADE"`
	DatabaseConnectionID *uuid.UUID          `gorm:"type:uuid;index"`
	DatabaseConnection   *DatabaseConnection `gorm:"foreignKey:DatabaseConnectionID;constraint:OnDelete:CASCADE"`
	GrantedBy            *uuid.UUID          `gorm:"type:uuid"`
	User                 *User               `gorm:"foreignKey:GrantedBy;constraint:OnDelete:SET NULL"`
	CreatedAt            time.Time
}

func (TeamConnectionGrant) TableName() string { return "team_connection_grants" }
