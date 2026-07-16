package model

import (
	"time"

	"github.com/google/uuid"
	"github.com/lib/pq"
)

// Skill is a reusable prompt + references bundle that agents can invoke.
// Org skills have an org and no team; an explicit TeamSkillGrant makes them
// available to a team. Team skills have both an org and team and are available
// to that team's agents by ownership.
type Skill struct {
	ID          uuid.UUID  `gorm:"type:uuid;primaryKey;default:gen_random_uuid()"`
	OrgID       *uuid.UUID `gorm:"type:uuid;index"`
	Org         *Org       `gorm:"foreignKey:OrgID;constraint:OnDelete:CASCADE"`
	TeamID      *uuid.UUID `gorm:"type:uuid;index"`
	Team        *Team      `gorm:"foreignKey:TeamID;constraint:OnDelete:CASCADE"`
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

	Tags   pq.StringArray `gorm:"type:text[];default:'{}'"`
	Hidden bool           `gorm:"not null;default:false;index"`

	// Status is draft, published, or archived.
	Status string `gorm:"not null;default:'draft';index"`

	CreatedAt time.Time
	UpdatedAt time.Time
}

func (Skill) TableName() string { return "skills" }

// TeamSkillGrant makes an org skill available to every agent on a team.
// Team-owned skills need no grant.
type TeamSkillGrant struct {
	ID        uuid.UUID  `gorm:"type:uuid;primaryKey;default:gen_random_uuid()"`
	OrgID     uuid.UUID  `gorm:"type:uuid;not null;index"`
	Org       Org        `gorm:"foreignKey:OrgID;constraint:OnDelete:CASCADE"`
	TeamID    uuid.UUID  `gorm:"type:uuid;not null;uniqueIndex:team_skill_grants_team_skill_unique,priority:1"`
	Team      Team       `gorm:"foreignKey:TeamID;constraint:OnDelete:CASCADE"`
	SkillID   uuid.UUID  `gorm:"type:uuid;not null;uniqueIndex:team_skill_grants_team_skill_unique,priority:2"`
	Skill     Skill      `gorm:"foreignKey:SkillID;constraint:OnDelete:CASCADE"`
	GrantedBy *uuid.UUID `gorm:"type:uuid"`
	User      *User      `gorm:"foreignKey:GrantedBy;constraint:OnDelete:SET NULL"`
	CreatedAt time.Time
}

func (TeamSkillGrant) TableName() string { return "team_skill_grants" }

const (
	ConnectionKindIntegration = "integration"
	ConnectionKindDatabase    = "database"
)

const (
	SkillSourceInline = "inline"
	SkillSourceGit    = "git"

	SkillStatusDraft     = "draft"
	SkillStatusPublished = "published"
	SkillStatusArchived  = "archived"
)
