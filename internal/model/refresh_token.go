package model

import (
	"time"

	"github.com/google/uuid"
)

type RefreshToken struct {
	ID        uuid.UUID `gorm:"type:uuid;primaryKey;default:gen_random_uuid()"`
	UserID    uuid.UUID `gorm:"type:uuid;not null;index"`
	TokenHash string    `gorm:"not null;uniqueIndex"` // SHA-256 of the token string
	ExpiresAt time.Time `gorm:"not null"`
	RevokedAt *time.Time
	CreatedAt time.Time

	// Grace window for single-use rotation. When this token is rotated, the
	// replacement pair is stored here so a concurrent presentation of the same
	// single-use token within the grace window receives the same rotated pair
	// instead of being force-logged-out.
	ReplacedAt             *time.Time
	ReplacedByAccessToken  string
	ReplacedByRefreshToken string
}

func (RefreshToken) TableName() string { return "refresh_tokens" }
