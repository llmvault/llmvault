package model

import (
	"time"

	"github.com/google/uuid"
)

// GitHubPullRequestSession maps a bot-created pull request to the session that
// opened it, so later PR events route back into that same session. The unique
// index on (org_id, repo, pr_number) is defined in migration 000068 — the GORM
// tag here is documentation only (this project applies schema via SQL, not
// AutoMigrate).
type GitHubPullRequestSession struct {
	ID        uuid.UUID  `gorm:"type:uuid;primaryKey;default:gen_random_uuid()"`
	OrgID     uuid.UUID  `gorm:"type:uuid;not null;index"`
	Org       Org        `gorm:"foreignKey:OrgID;constraint:OnDelete:CASCADE"`
	AgentID   *uuid.UUID `gorm:"type:uuid"`
	Repo      string     `gorm:"type:text;not null"` // "owner/repo"
	PRNumber  int        `gorm:"not null"`
	SessionID uuid.UUID  `gorm:"type:uuid;not null;index"`
	Session   Session    `gorm:"foreignKey:SessionID;constraint:OnDelete:CASCADE"`
	HeadRef   string     `gorm:"type:text;not null;default:''"`
	CreatedAt time.Time
	UpdatedAt time.Time
}

func (GitHubPullRequestSession) TableName() string { return "github_pull_request_sessions" }
