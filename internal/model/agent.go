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
	// Type is "agent" for top-level agents and "subagent" for a sub-agent owned
	// by ParentAgentID. Sub-agents are excluded from top-level agent listings.
	Type          string     `gorm:"type:text;not null;default:'agent'"`
	ParentAgentID *uuid.UUID `gorm:"type:uuid;index"`
	// Name is unique per org among top-level agents (partial unique index
	// idx_agents_org_name WHERE parent_agent_id IS NULL) and unique within a
	// parent for sub-agents (idx_agents_parent_name).
	Name                string           `gorm:"type:text;not null"`
	Description         *string          `gorm:"type:text;not null;default:''"`
	AvatarURL           *string          `gorm:"type:text"`
	Category            *string          `gorm:"-"`
	Icon                string           `gorm:"type:text;not null;default:''"`
	IsDefault           bool             `gorm:"not null;default:false;index"`
	SandboxImage        string           `gorm:"type:text;not null;default:'default'"`
	SandboxSize         string           `gorm:"type:text;not null;default:'small'"`
	WorkspaceSnapshotID *uuid.UUID       `gorm:"type:uuid"`
	SandboxTemplateID   *uuid.UUID       `gorm:"type:uuid"`
	SandboxTemplate     *SandboxTemplate `gorm:"foreignKey:SandboxTemplateID;constraint:OnDelete:SET NULL"`

	Instructions     *string        `gorm:"type:text"`
	Model            string         `gorm:"not null"`
	ImageModel       string         `gorm:"type:text;not null;default:''"`
	VectorImageModel string         `gorm:"type:text;not null;default:''"`
	Tools            JSON           `gorm:"type:jsonb;not null;default:'{}'"`
	McpServers       RawJSON        `gorm:"type:jsonb;not null;default:'[]'"`
	// McpToolFilter gates MCP tools (allow/deny). NULL = all MCP tools allowed.
	McpToolFilter *ToolFilter `gorm:"type:jsonb;serializer:json"`
	Skills        JSON        `gorm:"type:jsonb;not null;default:'{}'"`
	Integrations  JSON        `gorm:"-"`
	RuntimeConfig JSON        `gorm:"column:runtime_config;type:jsonb;not null;default:'{}'"`
	Permissions   JSON        `gorm:"type:jsonb;not null;default:'{}'"`
	Resources     JSON        `gorm:"type:jsonb;not null;default:'{}'"`

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

// SandboxToolDefinition describes a tool/service that can be enabled in a sandbox.
type SandboxToolDefinition struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
}

// ValidSandboxTools is the canonical list of sandbox tools the platform supports.
var ValidSandboxTools = []SandboxToolDefinition{
	{ID: "chrome", Name: "Chrome browser", Description: "Headless Chrome for web scraping, testing, and browser automation via agent-browser."},
}

// validSandboxToolIDs is a set for fast validation lookups.
var validSandboxToolIDs = func() map[string]bool {
	result := make(map[string]bool, len(ValidSandboxTools))
	for _, tool := range ValidSandboxTools {
		result[tool.ID] = true
	}
	return result
}()

// ValidateSandboxTools checks that all provided tool IDs are recognized.
// Returns the first invalid ID, or empty string if all are valid.
func ValidateSandboxTools(tools []string) string {
	for _, tool := range tools {
		if !validSandboxToolIDs[tool] {
			return tool
		}
	}
	return ""
}

// BuiltInToolDefinition describes a built-in tool that can be granted to an agent.
type BuiltInToolDefinition struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Category    string `json:"category"`
	Locked      bool   `json:"locked"` // true = cannot be toggled off by the user
}

// ValidBuiltInTools is the canonical list of all built-in tools available in Runtime.
// Used for: the frontend tool picker, permission key validation, and forge tool mocking.
var ValidBuiltInTools = []BuiltInToolDefinition{
	{ID: "Read", Name: "Read file", Description: "Read file contents with optional line range and hash-based caching.", Category: "filesystem"},
	{ID: "write", Name: "Write file", Description: "Create or overwrite a file with new content.", Category: "filesystem"},
	{ID: "edit", Name: "Edit file", Description: "Apply targeted edits to a file using search-and-replace.", Category: "filesystem"},
	{ID: "multiedit", Name: "Multi-edit file", Description: "Apply multiple edits to a single file in one call.", Category: "filesystem"},
	{ID: "apply_patch", Name: "Apply patch", Description: "Apply a multi-file patch using the runtime patch format.", Category: "filesystem"},
	{ID: "file_search", Name: "File search", Description: "Fuzzy file path search using FFF typo-tolerant ranking.", Category: "filesystem"},
	{ID: "glob", Name: "Glob", Description: "Find files matching a glob pattern using the FFF index.", Category: "filesystem"},
	{ID: "grep", Name: "Grep", Description: "Search file contents using FFF plain, regex, or fuzzy matching.", Category: "filesystem"},
	{ID: "multi_grep", Name: "Multi grep", Description: "Search file contents for several patterns in one FFF pass.", Category: "filesystem"},
	{ID: "AstGrep", Name: "AstGrep", Description: "Structural code search using ast-grep patterns (syntax-aware match/rewrite).", Category: "filesystem"},
	{ID: "LS", Name: "List directory", Description: "List files and directories at a given path.", Category: "filesystem"},

	{ID: "bash", Name: "Bash", Description: "Execute shell commands and return output.", Category: "shell"},

	{ID: "web_fetch", Name: "Fetch URL", Description: "Fetch content from a URL and convert to markdown, text, or HTML.", Category: "web"},
	{ID: "web_search", Name: "Web search", Description: "Search the web and return results with titles, descriptions, and URLs.", Category: "web"},
	{ID: "web_crawl", Name: "Crawl website", Description: "Crawl a website following links from a starting URL.", Category: "web"},
	{ID: "web_get_links", Name: "Get links", Description: "Extract all links from a webpage.", Category: "web"},
	{ID: "web_screenshot", Name: "Screenshot", Description: "Take a screenshot of a webpage as base64-encoded PNG.", Category: "web"},
	{ID: "web_transform", Name: "Transform HTML", Description: "Convert HTML to markdown or plain text without HTTP requests.", Category: "web"},

	{ID: "batch", Name: "Batch", Description: "Execute multiple independent tool calls concurrently.", Category: "orchestration"},

	{ID: "todowrite", Name: "Write tasks", Description: "Create and manage a structured task list for the current session.", Category: "tasks"},
	{ID: "todoread", Name: "Read tasks", Description: "Read the current task list with statuses and priorities.", Category: "tasks"},

	{ID: "journal_write", Name: "Write journal", Description: "Write an entry to the persistent conversation journal.", Category: "journal"},
	{ID: "journal_read", Name: "Read journal", Description: "Read all journal entries including checkpoint summaries.", Category: "journal"},

	{ID: "ping_me_back_in", Name: "Ping me back", Description: "Schedule a delayed self-reminder. After the specified seconds, the agent receives a notification with a custom message. Returns a ping ID for cancellation.", Category: "scheduling"},
	{ID: "cancel_ping_me_back", Name: "Cancel ping", Description: "Cancel a pending ping by its ID (returned by ping_me_back_in).", Category: "scheduling"},

	{ID: "lsp", Name: "LSP", Description: "Language Server Protocol operations for code navigation and diagnostics.", Category: "code_intelligence"},
	{ID: "skill", Name: "Skill", Description: "Execute a skill within the conversation.", Category: "code_intelligence"},
}

// validBuiltInToolIDs is a set for fast validation lookups.
var validBuiltInToolIDs = func() map[string]bool {
	result := make(map[string]bool, len(ValidBuiltInTools))
	for _, tool := range ValidBuiltInTools {
		result[tool.ID] = true
	}
	return result
}()

// BuiltInToolIDs returns just the ID strings from the registry.
func BuiltInToolIDs() []string {
	ids := make([]string, len(ValidBuiltInTools))
	for index, tool := range ValidBuiltInTools {
		ids[index] = tool.ID
	}
	return ids
}

// ValidatePermissionKeys checks that all keys in a permissions map are valid built-in tool IDs.
// Returns the first invalid key, or empty string if all are valid.
func ValidatePermissionKeys(permissions map[string]string) string {
	for key := range permissions {
		if !validBuiltInToolIDs[key] {
			return key
		}
	}
	return ""
}

// IsValidPermissionKey reports whether key refers to a known built-in tool ID.
func IsValidPermissionKey(key string) bool {
	return validBuiltInToolIDs[key]
}

// RuntimeBuiltInToolIDs are the canonical built-in tool identifiers understood by
// the Rust runtime AgentDefinition.tools schema.
var RuntimeBuiltInToolIDs = []string{
	"bash",
	"read_file",
	"write_file",
	"file_search",
	"glob",
	"grep",
	"multi_grep",
	"apply_patch",
	"lsp",
	"subagent_task",
	"check_bash_status",
	"search_sessions",
	"request_user_input",
	"update_plan",
}

// BaselineRuntimeToolIDs is the subset of RuntimeBuiltInToolIDs every
// top-level agent needs to operate its sandbox: shell, file read/write/edit,
// search, planning, session search, and asking the user. The agent-builder
// MCP tools always grant these to parent agents and only expose the remaining
// runtime tools for selection. Sub-agents are exempt so read-only sub-agents
// stay expressible.
var BaselineRuntimeToolIDs = []string{
	"bash",
	"check_bash_status",
	"read_file",
	"write_file",
	"apply_patch",
	"file_search",
	"glob",
	"grep",
	"multi_grep",
	"update_plan",
	"search_sessions",
	"request_user_input",
}

// ReadOnlyMCPToolFloor lists read-only MCP tools that an MCP tool filter allow
// list must never lock out: skills are unusable without the skill tools, and
// the cron / create_http_trigger tools are unusable without list_channels to
// resolve Hivy channel UUIDs. An explicit deny still wins.
var ReadOnlyMCPToolFloor = []string{
	"skills_list",
	"skill_view",
	"list_channels",
}

var validRuntimeBuiltInToolIDs = func() map[string]bool {
	result := make(map[string]bool, len(RuntimeBuiltInToolIDs))
	for _, id := range RuntimeBuiltInToolIDs {
		result[id] = true
	}
	return result
}()

func IsValidRuntimeBuiltInToolID(key string) bool {
	return validRuntimeBuiltInToolIDs[key]
}
