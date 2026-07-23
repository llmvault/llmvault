package model

import (
	"time"

	"github.com/google/uuid"
	"github.com/lib/pq"
)

const (
	OnboardingStepTeam        = "team"
	OnboardingStepConnections = "connections"
	OnboardingStepWelcome     = "welcome"
	OnboardingStepComplete    = "complete"
)

type Org struct {
	ID             uuid.UUID      `gorm:"type:uuid;primaryKey;default:gen_random_uuid()"`
	Name           string         `gorm:"not null;uniqueIndex"`
	RateLimit      int            `gorm:"not null;default:1000"`
	Active         bool           `gorm:"not null;default:true"`
	OnboardingStep string         `gorm:"not null;default:'complete';size:32"`
	AllowedOrigins pq.StringArray `gorm:"type:text[]"`

	// BYOK reports whether the org runs agents on its own LLM credentials.
	// When false, agents fall back to platform-owned system credentials.
	BYOK bool `gorm:"not null;default:false"`

	// CapacityTier is a permanent org unlock promoted by lifetime completed
	// deposits. It never decreases automatically.
	CapacityTier int `gorm:"not null;default:1"`

	// LogoURL is a CDN-served URL to the org's square logo. Stored as the
	// asset_url returned from POST /v1/uploads/sign with asset_type=org_logo.
	// Empty string when no logo is set.
	LogoURL string `gorm:"not null;default:''"`

	Website     string `gorm:"not null;default:'';size:500"`
	Description string `gorm:"type:text;not null;default:''"`

	PromptCompany string `gorm:"type:text;not null;default:''"`

	SandboxExposedPorts pq.Int64Array `gorm:"type:integer[];not null;default:'{3000,5173,8000,8080}'"`

	CreatedAt time.Time
	UpdatedAt time.Time
}

func (Org) TableName() string { return "orgs" }
