package model

import (
	"time"

	"github.com/google/uuid"
	"github.com/lib/pq"
)

// Skill is a reusable prompt + references bundle that agents can invoke. A
// skill with OrgID=nil belongs to a global catalog plugin. A custom skill keeps
// its org for tenant integrity and inherits its team visibility exclusively
// from its team-owned Plugin.
type Skill struct {
	ID          uuid.UUID  `gorm:"type:uuid;primaryKey;default:gen_random_uuid()"`
	PluginID    *uuid.UUID `gorm:"type:uuid;index"`
	Plugin      *Plugin    `gorm:"foreignKey:PluginID;constraint:OnDelete:CASCADE"`
	OrgID       *uuid.UUID `gorm:"type:uuid;index"`
	Org         *Org       `gorm:"foreignKey:OrgID;constraint:OnDelete:CASCADE"`
	PublisherID *uuid.UUID `gorm:"type:uuid;index"`
	Publisher   *User      `gorm:"foreignKey:PublisherID;constraint:OnDelete:SET NULL"`

	Slug        string  `gorm:"not null;index"`
	Name        string  `gorm:"not null"`
	Description *string `gorm:"type:text"`
	// HumanDescription is user-facing display copy. Description remains the
	// agent-facing description used when compiling runtime config.
	HumanDescription *string `gorm:"type:text"`
	Category         string  `gorm:"not null;default:'';size:64;index"`

	// SourceType is "inline" (content authored in the UI) or "git" (hydrated from a repo).
	SourceType  string  `gorm:"not null"`
	RepoURL     *string `gorm:"type:text"`
	RepoSubpath *string `gorm:"type:text"`
	RepoRef     string  `gorm:"not null;default:'main'"`

	Bundle            RawJSON `gorm:"type:jsonb;not null;default:'{}'"`
	HydratedCommitSHA *string `gorm:"type:text"`
	HydratedAt        *time.Time
	HydrationError    *string `gorm:"type:text"`

	Tags           pq.StringArray `gorm:"type:text[];default:'{}'"`
	IntegrationIDs pq.StringArray `gorm:"type:text[];default:'{}'"`
	InstallCount   int            `gorm:"not null;default:0"`
	Featured       bool           `gorm:"not null;default:false;index"`
	Hidden         bool           `gorm:"not null;default:false;index"`
	VerifiedAt     *time.Time

	// Status is draft, published, or archived.
	Status string `gorm:"not null;default:'draft';index"`

	// PublicSkillID references the cloned public skill when this org skill is
	// promoted into the global library.
	PublicSkillID *uuid.UUID `gorm:"type:uuid;index"`

	// OriginSkillID references the original org-scoped skill that was cloned
	// to create this public skill. Only set on public (OrgID=nil) skills.
	OriginSkillID *uuid.UUID `gorm:"type:uuid;index"`
	// OriginOrgID is the org that published this public skill.
	OriginOrgID *uuid.UUID `gorm:"type:uuid"`
	// PublishedAt is when this skill was made public.
	PublishedAt *time.Time

	CreatedAt time.Time
	UpdatedAt time.Time
}

func (Skill) TableName() string { return "skills" }

const (
	SkillSourceInline = "inline"
	SkillSourceGit    = "git"

	SkillStatusDraft     = "draft"
	SkillStatusPublished = "published"
	SkillStatusArchived  = "archived"
)
