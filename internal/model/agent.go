package model

import (
	"time"

	"github.com/google/uuid"
	"github.com/lib/pq"
)

type Agent struct {
	ID             uuid.UUID     `gorm:"type:uuid;primaryKey;default:gen_random_uuid()"`
	OrgID          *uuid.UUID    `gorm:"type:uuid;not null;index:idx_agent_org_id"`
	Org            *Org          `gorm:"foreignKey:OrgID;constraint:OnDelete:CASCADE"`
	AgentCatalogID *uuid.UUID    `gorm:"type:uuid;index"`
	AgentCatalog   *AgentCatalog `gorm:"foreignKey:AgentCatalogID;constraint:OnDelete:SET NULL"`
	// TeamID is the owning team. Teams are the provisioning unit: every agent
	// belongs to exactly one team (team primary-authz model). ON DELETE RESTRICT.
	TeamID uuid.UUID `gorm:"type:uuid;not null;index:idx_agents_team_id"`
	Team   *Team     `gorm:"foreignKey:TeamID;constraint:OnDelete:RESTRICT"`
	// Type is "agent" for top-level agents and "subagent" for a sub-agent owned
	// by ParentAgentID. Sub-agents are excluded from top-level agent listings.
	Type          string     `gorm:"type:text;not null;default:'agent'"`
	ParentAgentID *uuid.UUID `gorm:"type:uuid;index"`
	// Name is unique within a parent for sub-agents (idx_agents_parent_name).
	// Top-level agent names are not unique per org (several teams share a "Hivy").
	Name string `gorm:"type:text;not null"`
	// EmailInboxLocalPart is immutable once provisioned. The configured inbox
	// domain is deployment-wide, so only the local part belongs in the DB.
	EmailInboxLocalPart string           `gorm:"column:email_inbox_local_part;type:text;not null;default:'';uniqueIndex:idx_agents_email_inbox_local_part,where:email_inbox_local_part <> ''"`
	Description         *string          `gorm:"type:text;not null;default:''"`
	AvatarURL           *string          `gorm:"type:text"`
	Category            *string          `gorm:"-"`
	Icon                string           `gorm:"type:text;not null;default:''"`
	IsDefault           bool             `gorm:"not null;default:false;index"`
	SandboxImage        string           `gorm:"type:text;not null;default:'default'"`
	SandboxSize         string           `gorm:"type:text;not null;default:'nano'"`
	SandboxTemplateID   *uuid.UUID       `gorm:"type:uuid"`
	SandboxTemplate     *SandboxTemplate `gorm:"foreignKey:SandboxTemplateID;constraint:OnDelete:SET NULL"`

	Instructions *string `gorm:"type:text"`
	// InstructionsSnapshot is a fallback copy of the catalog prompt. An unedited
	// clone prefers the live catalog; a user-authored Instructions value wins.
	InstructionsSnapshot *string `gorm:"column:instructions_snapshot;type:text"`
	Model                string  `gorm:"not null"`
	// DefaultReasoningEffort seeds new sessions (low|medium|high) when the caller
	// does not pass an explicit reasoning_effort. Empty means unset.
	DefaultReasoningEffort string `gorm:"column:default_reasoning_effort;type:text"`
	// AutoLoadSkills lists skills the runtime preloads before the first model
	// call of every turn (normalized {name, files} object form).
	AutoLoadSkills   AutoLoadSkills `gorm:"column:auto_load_skills;type:jsonb;not null;default:'[]'"`
	ImageModel       string         `gorm:"type:text;not null;default:''"`
	VectorImageModel string         `gorm:"type:text;not null;default:''"`
	// MemoryMission steers this agent's extraction and consolidation policy.
	// Empty uses the installed catalog's category template.
	MemoryMission *string `gorm:"type:text"`
	Tools         JSON    `gorm:"type:jsonb;not null;default:'{}'"`
	McpServers    RawJSON `gorm:"type:jsonb;not null;default:'[]'"`
	// McpToolFilter records requested grants for managed MCP capabilities. The
	// runtime compiler adds the universal floor; generated connection MCP servers
	// use ConnectionMCPToolDeny instead.
	McpToolFilter *ToolFilter `gorm:"type:jsonb;serializer:json"`
	// ConnectionMCPToolDeny is keyed by concrete connection UUID. Users can opt
	// individual generated MCP tools out without affecting another instance of
	// the same provider; "*" opts the agent out of the inherited connection.
	ConnectionMCPToolDeny ConnectionMCPToolDeny `gorm:"column:connection_mcp_tool_deny;type:jsonb;not null;default:'{}';serializer:json"`
	Skills                JSON                  `gorm:"type:jsonb;not null;default:'{}'"`
	Integrations          JSON                  `gorm:"-"`
	RuntimeConfig         JSON                  `gorm:"column:runtime_config;type:jsonb;not null;default:'{}'"`
	Permissions           JSON                  `gorm:"type:jsonb;not null;default:'{}'"`
	Resources             JSON                  `gorm:"type:jsonb;not null;default:'{}'"`

	SandboxTools  pq.StringArray `gorm:"type:text[];default:'{}'"` // enabled sandbox tools (e.g. "chrome")
	SetupCommands pq.StringArray `gorm:"type:text[];default:'{}'"` // shell commands run during sandbox creation

	Status        string `gorm:"not null;default:'active'"` // draft, active, archived
	IsSystem      bool   `gorm:"-"`
	IsManaged     bool   `gorm:"-"`
	ProviderGroup string `gorm:"-"`

	LastProxyTokenRefreshedAt *time.Time `gorm:"type:timestamptz"`

	CreatedAt time.Time
	UpdatedAt time.Time
}

func (Agent) TableName() string { return "agents" }

const (
	// AgentTypeAgent is a top-level agent (parent_agent_id IS NULL).
	AgentTypeAgent = "agent"
	// AgentTypeSubAgent is a sub-agent owned by a parent agent via ParentAgentID.
	AgentTypeSubAgent = "subagent"
)
