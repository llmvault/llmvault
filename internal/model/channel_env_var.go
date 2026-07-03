package model

import (
	"time"

	"github.com/google/uuid"
)

// ChannelEnvVar is a user-supplied environment variable scoped to a single
// channel. The value is stored AES-256-GCM encrypted. Name is the user's chosen
// name (uppercase, without the __ENV__ injection prefix); it is unique per
// channel. At runtime config push, each var for the session's channel is
// injected as __ENV__<Name>, which the sandbox runtime strips before exposing
// the clean name to the workload.
type ChannelEnvVar struct {
	ID             uuid.UUID `gorm:"type:uuid;primaryKey;default:gen_random_uuid()"`
	OrgID          uuid.UUID `gorm:"type:uuid;not null;index"`
	ChannelID      uuid.UUID `gorm:"type:uuid;not null;index"`
	Channel        Channel   `gorm:"foreignKey:ChannelID;constraint:OnDelete:CASCADE"`
	Name           string    `gorm:"type:text;not null"`
	EncryptedValue []byte    `gorm:"type:bytea;not null"`
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

func (ChannelEnvVar) TableName() string { return "channel_env_vars" }
