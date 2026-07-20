package model

import (
	"time"

	"github.com/google/uuid"
)

// OrgSessionCapacityReservation protects an org's final concurrent-session
// slots while a sandbox is being provisioned outside the database.
type OrgSessionCapacityReservation struct {
	ID        uuid.UUID `gorm:"type:uuid;primaryKey"`
	OrgID     uuid.UUID `gorm:"type:uuid;not null;index"`
	ExpiresAt time.Time `gorm:"not null;index"`
	CreatedAt time.Time
}

func (OrgSessionCapacityReservation) TableName() string {
	return "org_session_capacity_reservations"
}
