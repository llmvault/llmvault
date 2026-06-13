package model

import (
	"time"

	"github.com/google/uuid"
)

type DatabaseConnection struct {
	ID             uuid.UUID  `gorm:"type:uuid;primaryKey;default:gen_random_uuid()"`
	OrgID          uuid.UUID  `gorm:"type:uuid;not null;index"`
	Org            Org        `gorm:"foreignKey:OrgID;constraint:OnDelete:CASCADE"`
	AgentID        *uuid.UUID `gorm:"type:uuid;index"`
	Agent          *Agent     `gorm:"foreignKey:AgentID;constraint:OnDelete:SET NULL"`
	Provider       string     `gorm:"type:varchar(32);not null;index"`
	DisplayName    string     `gorm:"type:text;not null;default:''"`
	EncryptedDSN   []byte     `gorm:"type:bytea;not null"`
	WrappedDEK     []byte     `gorm:"type:bytea;not null"`
	SchemaSnapshot RawJSON    `gorm:"type:jsonb;not null;default:'{}'"`
	AccessPolicy   JSON       `gorm:"type:jsonb;not null;default:'{}'"`
	RevokedAt      *time.Time `gorm:"type:timestamptz"`
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

func (DatabaseConnection) TableName() string { return "database_connections" }
