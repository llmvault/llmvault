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
	Name              string           `gorm:"type:text;not null"`
	Description       *string          `gorm:"type:text;not null;default:''"`
	AvatarURL         *string          `gorm:"type:text"`
	Category          *string          `gorm:"-"`
	Icon              string           `gorm:"type:text;not null;default:''"`
	IsDefault         bool             `gorm:"not null;default:false;index"`
	SandboxImage      string           `gorm:"type:text;not null;default:'default'"`
	SandboxSize       string           `gorm:"type:text;not null;default:'small'"`
	SandboxTemplateID *uuid.UUID       `gorm:"type:uuid"`
	SandboxTemplate   *SandboxTemplate `gorm:"foreignKey:SandboxTemplateID;constraint:OnDelete:SET NULL"`

	Instructions *string `gorm:"type:text"`
	// InstructionsSnapshot is a fallback copy of the catalog prompt. An unedited
	// clone prefers the live catalog; a user-authored Instructions value wins.
	InstructionsSnapshot *string `gorm:"column:instructions_snapshot;type:text"`
	Model                string  `gorm:"not null"`
	// DefaultReasoningEffort seeds new sessions (low|medium|high) when the caller
	// does not pass an explicit reasoning_effort. Empty means unset.
	DefaultReasoningEffort string `gorm:"column:default_reasoning_effort;type:text"`
	// AutoLoadSkills lists skills the runtime preloads into new sessions before
	// the first model call (normalized {name, files} object form).
	AutoLoadSkills   AutoLoadSkills `gorm:"column:auto_load_skills;type:jsonb;not null;default:'[]'"`
	ImageModel       string         `gorm:"type:text;not null;default:''"`
	VectorImageModel string         `gorm:"type:text;not null;default:''"`
	Tools            JSON           `gorm:"type:jsonb;not null;default:'{}'"`
	McpServers       RawJSON        `gorm:"type:jsonb;not null;default:'[]'"`
	// McpToolFilter records requested grants for managed MCP capabilities. The
	// runtime compiler adds the universal floor; generated connection MCP servers
	// use ConnectionMCPToolDeny instead.
	McpToolFilter *ToolFilter `gorm:"type:jsonb;serializer:json"`
	// ConnectionMCPToolDeny is keyed by concrete connection UUID. Users can opt
	// individual generated MCP tools out without affecting another instance of
	// the same provider.
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

// ValidBuiltInTools is the canonical list of every tool an agent can be granted:
// the Rust runtime's native built-in tools plus the Hivy MCP tools its runtime
// surfaces. Consumed for permission-key validation (ValidatePermissionKeys /
// IsValidPermissionKey) and default permission seeding (BuiltInToolIDs).
//
// Ground truth — keep this list in sync with:
//   - Runtime-native tools ("runtime.*" categories below): the ToolSpec enum in
//     sandboxes/runtime/crates/domain/src/tool_specs.rs (serde `builtin.<id>`),
//     mirrored Go-side by RuntimeBuiltInToolIDs. The runtime's openapi.json also
//     enumerates the same `builtin.*` ids.
//   - MCP tools ("mcp.*" categories below): the curated explicit-grant set
//     AssignableMCPTools in internal/agents/tools.go, registered on the "hivy"
//     MCP server (internal/mcpserver/builder.go and the per-package mcptools.go
//     files). Default-Hivy management and automation tools remain intentionally
//     excluded because only the team's default Hivy agent may use them.
//
// TestValidBuiltInToolsMatchesRuntimeAndMCP pins this to those sources so drift
// surfaces as a deliberate test edit.
var ValidBuiltInTools = []BuiltInToolDefinition{
	// Runtime-native built-in tools (RuntimeBuiltInToolIDs / ToolSpec enum).
	{ID: "bash", Name: "Bash", Description: "Execute shell commands in the sandbox and return their output.", Category: "runtime.shell"},
	{ID: "read_file", Name: "Read file", Description: "Read file contents with an optional line range.", Category: "runtime.filesystem"},
	{ID: "write_file", Name: "Write file", Description: "Create or overwrite a file with new content.", Category: "runtime.filesystem"},
	{ID: "apply_patch", Name: "Apply patch", Description: "Apply a multi-file patch to create, update, or delete files.", Category: "runtime.filesystem"},
	{ID: "lsp", Name: "LSP", Description: "Language Server Protocol operations for code navigation and diagnostics.", Category: "runtime.code_intelligence"},
	{ID: "subagent_task", Name: "Delegate to sub-agent", Description: "Delegate a task to one of the agent's sub-agents.", Category: "runtime.orchestration"},
	{ID: "search_sessions", Name: "Search sessions", Description: "Search prior sessions for relevant context.", Category: "runtime.orchestration"},
	{ID: "request_user_input", Name: "Ask user", Description: "Ask the user a question and wait for their reply.", Category: "runtime.orchestration"},
	{ID: "update_plan", Name: "Update plan", Description: "Create or update a concise visible plan for multi-step work.", Category: "runtime.orchestration"},

	// Hivy MCP tools surfaced to the agent runtime (AssignableMCPTools).
	{ID: "web_search", Name: "Web search", Description: "Search the web and return results with titles, descriptions, and URLs.", Category: "mcp.web"},
	{ID: "web_fetch", Name: "Fetch URL", Description: "Fetch a URL and return its content as markdown, text, or HTML.", Category: "mcp.web"},
	{ID: "web_crawl", Name: "Crawl website", Description: "Crawl a website and return content from multiple pages.", Category: "mcp.web"},
	{ID: "generate_image", Name: "Generate image", Description: "Generate a raster image from a text prompt.", Category: "mcp.image"},
	{ID: "generate_vector_image", Name: "Generate vector image", Description: "Generate an SVG vector image from a text prompt.", Category: "mcp.image"},
	{ID: "remix_image", Name: "Remix image", Description: "Generate a new image from a text prompt and one or more reference images.", Category: "mcp.image"},
	{ID: "vectorize_image", Name: "Vectorize image", Description: "Convert an organization image asset into SVG.", Category: "mcp.image"},
	{ID: "search_knowledge_base", Name: "Search knowledge base", Description: "Search the organization's synced knowledge base (Slack, GitHub, Linear, Notion, websites, and uploaded files).", Category: "mcp.knowledge"},
	{ID: "sheet_create", Name: "Create sheet", Description: "Create a typed sheet.", Category: "mcp.sheets"},
	{ID: "sheet_list", Name: "List sheets", Description: "List sheets available in the current channel.", Category: "mcp.sheets"},
	{ID: "sheet_describe", Name: "Describe sheet", Description: "Inspect a sheet's pages and fields.", Category: "mcp.sheets"},
	{ID: "sheet_manage", Name: "Manage sheet", Description: "Manage sheet pages and fields.", Category: "mcp.sheets"},
	{ID: "rows_query", Name: "Query rows", Description: "Query rows in a sheet page.", Category: "mcp.sheets"},
	{ID: "rows_write", Name: "Write rows", Description: "Create, update, or delete sheet rows.", Category: "mcp.sheets"},
	{ID: "sheet_import_csv", Name: "Import CSV", Description: "Import CSV data into a sheet.", Category: "mcp.sheets"},
	{ID: "sheet_operations", Name: "Sheet operations", Description: "Run bulk operations on a sheet.", Category: "mcp.sheets"},
	{ID: "app_create", Name: "Create app", Description: "Create an app workspace.", Category: "mcp.apps"},
	{ID: "app_publish", Name: "Publish app", Description: "Publish an app.", Category: "mcp.apps"},
	{ID: "app_status", Name: "App status", Description: "Inspect an app's status.", Category: "mcp.apps"},
	{ID: "app_logs", Name: "App logs", Description: "Read app logs.", Category: "mcp.apps"},
	{ID: "app_rollback", Name: "Rollback app", Description: "Roll an app back to a prior version.", Category: "mcp.apps"},
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
	"apply_patch",
	"lsp",
	"subagent_task",
	"search_sessions",
	"request_user_input",
	"update_plan",
}

// BaselineRuntimeToolIDs is the subset of RuntimeBuiltInToolIDs every
// top-level agent needs to operate its sandbox: shell, file read/write/edit,
// planning, session search, and asking the user. Agents use Bash with fd and
// rg for file discovery and content search. The agent-builder
// MCP tools always grant these to parent agents and only expose the remaining
// runtime tools for selection. Sub-agents have their own tool selection.
var BaselineRuntimeToolIDs = []string{
	"bash",
	"read_file",
	"write_file",
	"apply_patch",
	"update_plan",
	"search_sessions",
	"request_user_input",
}

// ReadOnlyMCPToolFloor lists the universal MCP tools. Every compiled agent
// filter includes these tools; all other MCP tools require an explicit allow.
// Skill availability is rendered into the system prompt, so skill_view alone is
// sufficient to load a selected skill.
var ReadOnlyMCPToolFloor = []string{
	"skill_view",
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
