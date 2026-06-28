package model

import (
	"time"

	"github.com/google/uuid"
)

const (
	SessionReflectionStatusIdle    = "idle"
	SessionReflectionStatusRunning = "running"
	SessionReflectionStatusFailed  = "failed"
)

type SessionReflectionState struct {
	SessionID               uuid.UUID  `gorm:"type:uuid;primaryKey"`
	Session                 Session    `gorm:"foreignKey:SessionID;constraint:OnDelete:CASCADE"`
	OrgID                   uuid.UUID  `gorm:"type:uuid;not null;index"`
	Org                     Org        `gorm:"foreignKey:OrgID;constraint:OnDelete:CASCADE"`
	AgentID                 uuid.UUID  `gorm:"type:uuid;not null;index"`
	Agent                   Agent      `gorm:"foreignKey:AgentID;constraint:OnDelete:RESTRICT"`
	LastReflectedEventID    *uuid.UUID `gorm:"type:uuid"`
	LastReflectedEventAt    *time.Time
	LastReflectedRuntimeSeq *int64 `gorm:"type:bigint"`
	Status                  string `gorm:"type:text;not null;default:'idle'"`
	LockedUntil             *time.Time
	LastError               string `gorm:"type:text;not null;default:''"`
	CreatedAt               time.Time
	UpdatedAt               time.Time
}

func (SessionReflectionState) TableName() string { return "session_reflection_states" }
