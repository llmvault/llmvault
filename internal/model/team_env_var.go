package model

import (
	"time"

	"github.com/google/uuid"
)

// TeamEnvVar is a user-supplied environment variable shared by every channel
// owned by a team. Values are stored AES-256-GCM encrypted and are never
// returned by the HTTP API.
type TeamEnvVar struct {
	ID             uuid.UUID `gorm:"type:uuid;primaryKey;default:gen_random_uuid()"`
	OrgID          uuid.UUID `gorm:"type:uuid;not null;index"`
	TeamID         uuid.UUID `gorm:"type:uuid;not null;index"`
	Team           Team      `gorm:"foreignKey:TeamID,OrgID;references:ID,OrgID;constraint:OnDelete:CASCADE"`
	Name           string    `gorm:"type:text;not null"`
	EncryptedValue []byte    `gorm:"type:bytea;not null"`
	Description    string    `gorm:"type:text;not null;default:''"`
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

func (TeamEnvVar) TableName() string { return "team_env_vars" }
