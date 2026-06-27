package model

import (
	"time"

	"github.com/google/uuid"
	"github.com/lib/pq"
)

type AgentCatalog struct {
	ID                 uuid.UUID      `gorm:"type:uuid;primaryKey;default:gen_random_uuid()"`
	Slug               string         `gorm:"not null;uniqueIndex"`
	Name               string         `gorm:"not null"`
	Description        string         `gorm:"type:text;not null;default:''"`
	Category           string         `gorm:"not null;default:'';size:64;index"`
	AvatarURL          string         `gorm:"type:text;not null;default:''"`
	Developer          string         `gorm:"not null;default:'Hivy'"`
	Official           bool           `gorm:"not null;default:false"`
	IsDefault          bool           `gorm:"not null;default:false;index"`
	Model              string         `gorm:"not null;default:''"`
	AvailableModels    pq.StringArray `gorm:"type:text[];not null;default:'{}'"`
	SandboxImage       string         `gorm:"type:text;not null;default:'default'"`
	Instructions       string         `gorm:"type:text;not null;default:''"`
	Tools              JSON           `gorm:"type:jsonb;not null;default:'{}'"`
	SubAgents          RawJSON        `gorm:"type:jsonb;not null;default:'{}'"`
	RequiredPlugins    pq.StringArray `gorm:"type:text[];default:'{}'"`
	RecommendedPlugins pq.StringArray `gorm:"type:text[];default:'{}'"`
	Manifest           RawJSON        `gorm:"type:jsonb;not null;default:'{}'"`
	SourceHash         string         `gorm:"not null;default:''"`
	Status             string         `gorm:"not null;default:'active';index"`
	CreatedAt          time.Time
	UpdatedAt          time.Time
}

func (AgentCatalog) TableName() string { return "agent_catalog" }

const (
	AgentCatalogStatusActive   = "active"
	AgentCatalogStatusArchived = "archived"
)

type AgentCatalogSubAgent struct {
	Name          string       `json:"name"`
	Description   string       `json:"description"`
	Model         string       `json:"model,omitempty"`
	Tools         JSON         `json:"tools,omitempty"`
	McpToolFilter *ToolFilter  `json:"mcp_tool_filter,omitempty"`
	SkillFilter   *SkillFilter `json:"skill_filter,omitempty"`
	Instructions  string       `json:"instructions"`
}

type ToolFilter struct {
	Allow []string `json:"allow"`
	Deny  []string `json:"deny"`
}

type SkillFilter struct {
	Allow []string `json:"allow"`
}
