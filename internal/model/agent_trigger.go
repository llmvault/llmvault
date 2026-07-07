package model

import (
	"time"

	"github.com/google/uuid"
	"github.com/lib/pq"
)

// AgentTrigger links an agent to an inbound trigger source.
type AgentTrigger struct {
	ID           uuid.UUID      `gorm:"type:uuid;primaryKey;default:gen_random_uuid()"`
	OrgID        uuid.UUID      `gorm:"type:uuid;not null;index"`
	Org          Org            `gorm:"foreignKey:OrgID;constraint:OnDelete:CASCADE"`
	AgentID      uuid.UUID      `gorm:"type:uuid;not null;index"`
	Agent        Agent          `gorm:"foreignKey:AgentID;constraint:OnDelete:CASCADE"`
	TriggerType  string         `gorm:"not null;default:'webhook';size:32;index"` // webhook or http
	ChannelID    *uuid.UUID     `gorm:"type:uuid;index"`
	Channel      *Channel       `gorm:"foreignKey:ChannelID;constraint:OnDelete:SET NULL"`
	ConnectionID *uuid.UUID     `gorm:"type:uuid;index"`
	Connection   *Connection    `gorm:"foreignKey:ConnectionID;constraint:OnDelete:CASCADE"`
	Name         string         `gorm:"type:text"`                         // optional human label; nullable in DB
	TriggerKeys  pq.StringArray `gorm:"type:text[];not null;default:'{}'"` // e.g. {"issues.opened","issues.reopened"}
	TriggerKey   string         `gorm:"type:text;not null;default:'';index"`
	TriggerValue string         `gorm:"type:text;not null;default:''"`
	Enabled      bool           `gorm:"not null;default:true"`
	SourceSlug   string         `gorm:"type:text;not null;default:'';index"`
	Conditions   RawJSON        `gorm:"type:jsonb"`                    // TriggerMatch JSON
	Instructions string         `gorm:"type:text;not null;default:''"` // per-trigger task instruction
	SecretKey    string         `gorm:"type:text;not null;default:''"` // bcrypt hash for HTTP triggers

	CreatedAt time.Time
	UpdatedAt time.Time
}

func (AgentTrigger) TableName() string { return "agent_triggers" }

// TriggerKeyGitHubIssueMention fires only when Hivy is @mentioned on an issue.
const TriggerKeyGitHubIssueMention = "issue_mention"

// TriggerKeyGitHubPRMention fires only when Hivy is @mentioned on a pull request.
const TriggerKeyGitHubPRMention = "pr_mention"

// TriggerKeyGitHubPROpened fires whenever a pull request is opened — no mention
// required. It drives the code-reviews app's automatic review-on-open feature
// and is deliberately NOT a mention key (IsGitHubMentionKey stays false for it).
const TriggerKeyGitHubPROpened = "pr_opened"

// GitHubPROpenedEventKeys subscribes an auto-review trigger to just the
// pull_request.opened webhook. Unlike the mention keys it does not share
// issue_comment.created, so no issue/PR entity gate is needed.
var GitHubPROpenedEventKeys = []string{
	"pull_request.opened",
}

// GitHubIssueMentionEventKeys subscribes an issue-only mention trigger. It shares
// issue_comment.created with pull requests, so the dispatch gate drops comments
// whose issue is actually a PR (issue.pull_request present).
var GitHubIssueMentionEventKeys = []string{
	"issue_comment.created",
	"issues.opened",
}

// GitHubPRMentionEventKeys subscribes a pull-request-only mention trigger. It
// shares issue_comment.created with issues, so the dispatch gate drops comments
// on plain issues (issue.pull_request absent).
var GitHubPRMentionEventKeys = []string{
	"issue_comment.created",
	"pull_request.opened",
}

// IsGitHubMentionKey reports whether a trigger key is one of the GitHub mention
// keys (issue-only or pull-request-only).
func IsGitHubMentionKey(key string) bool {
	switch key {
	case TriggerKeyGitHubIssueMention, TriggerKeyGitHubPRMention:
		return true
	}
	return false
}

// TriggerMatch defines filtering conditions on the webhook payload.
type TriggerMatch struct {
	Mode       string             `json:"mode"` // "all" (AND) or "any" (OR)
	Conditions []TriggerCondition `json:"conditions"`
}

// TriggerCondition is a single filter rule applied to the webhook payload.
type TriggerCondition struct {
	Path     string `json:"path"`     // dot-path into payload, e.g. "repository.full_name"
	Operator string `json:"operator"` // equals, not_equals, one_of, not_one_of, contains, not_contains, matches, exists, not_exists
	Value    any    `json:"value"`    // string or []string depending on operator
}
