package model

import (
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type DatabaseConnection struct {
	ID             uuid.UUID  `gorm:"type:uuid;primaryKey;default:gen_random_uuid()"`
	OrgID          uuid.UUID  `gorm:"type:uuid;not null;index"`
	Org            Org        `gorm:"foreignKey:OrgID;constraint:OnDelete:CASCADE"`
	Provider       string     `gorm:"type:varchar(32);not null;index"`
	DisplayName    string     `gorm:"type:text;not null;default:''"`
	Name           string     `gorm:"type:text;not null;default:''"`
	Slug           string     `gorm:"type:text;not null;default:'';index"`
	NeedsName      bool       `gorm:"not null;default:false"`
	EncryptedDSN   []byte     `gorm:"type:bytea;not null"`
	WrappedDEK     []byte     `gorm:"type:bytea;not null"`
	SchemaSnapshot RawJSON    `gorm:"type:jsonb;not null;default:'{}'"`
	AccessPolicy   JSON       `gorm:"type:jsonb;not null;default:'{}'"`
	RevokedAt      *time.Time `gorm:"type:timestamptz"`
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

func (DatabaseConnection) TableName() string { return "database_connections" }

func (c *DatabaseConnection) BeforeCreate(_ *gorm.DB) error {
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
