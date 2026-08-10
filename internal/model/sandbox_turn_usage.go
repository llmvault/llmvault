package model

import (
	"time"

	"github.com/google/uuid"
)

// SandboxTurnUsage is the durable, monotonically increasing compute duration
// for one runtime turn. Pricing and vCPU are snapshotted from the session.
type SandboxTurnUsage struct {
	OrgID                uuid.UUID `gorm:"type:uuid;not null;index"`
	SessionID            uuid.UUID `gorm:"type:uuid;primaryKey"`
	TurnID               string    `gorm:"primaryKey"`
	SandboxVCPU          int       `gorm:"column:sandbox_vcpu;not null"`
	PricingVersion       int       `gorm:"not null"`
	CreditsPerVCPUMinute int       `gorm:"column:credits_per_vcpu_minute;not null"`
	StartedAt            time.Time
	ObservedThrough      time.Time
	EndedAt              *time.Time
	ActiveMilliseconds   int64 `gorm:"not null;default:0"`
	CreatedAt            time.Time
	UpdatedAt            time.Time
}

func (SandboxTurnUsage) TableName() string { return "sandbox_turn_usage" }
