package model

import (
	"time"

	"github.com/google/uuid"
	"github.com/lib/pq"
)

type Agent struct {
	ID                  uuid.UUID        `gorm:"type:uuid;primaryKey;default:gen_random_uuid()"`
	OrgID               *uuid.UUID       `gorm:"type:uuid;not null;index:idx_agent_org_id;uniqueIndex:idx_agents_org_name,priority:1"`
	Org                 *Org             `gorm:"foreignKey:OrgID;constraint:OnDelete:CASCADE"`
	AgentCatalogID      *uuid.UUID       `gorm:"type:uuid;index"`
	AgentCatalog        *AgentCatalog    `gorm:"foreignKey:AgentCatalogID;constraint:OnDelete:SET NULL"`
	Name                string           `gorm:"type:text;not null;uniqueIndex:idx_agents_org_name,priority:2"`
	Description         *string          `gorm:"type:text;not null;default:''"`
	AvatarURL           *string          `gorm:"type:text"`
	Category            *string          `gorm:"-"`
	Icon                string           `gorm:"type:text;not null;default:''"`
	IsDefault           bool             `gorm:"not null;default:false;index"`
	SandboxStrategy     string           `gorm:"type:text;not null;default:'per_session'"`
	SandboxImage        string           `gorm:"type:text;not null;default:'default'"`
	SandboxSize         string           `gorm:"type:text;not null;default:'small'"`
	WorkspaceSnapshotID *uuid.UUID       `gorm:"type:uuid"`
	SandboxTemplateID   *uuid.UUID       `gorm:"type:uuid"`
	SandboxTemplate     *SandboxTemplate `gorm:"foreignKey:SandboxTemplateID;constraint:OnDelete:SET NULL"`

	Instructions    *string        `gorm:"type:text"`
	Model           string         `gorm:"not null"`
	AvailableModels pq.StringArray `gorm:"type:text[];not null;default:'{}'"`
	Tools           JSON           `gorm:"type:jsonb;not null;default:'{}'"`
	McpServers      RawJSON        `gorm:"type:jsonb;not null;default:'[]'"`
	Skills          JSON           `gorm:"type:jsonb;not null;default:'{}'"`
	Integrations    JSON           `gorm:"-"`
	RuntimeConfig   JSON           `gorm:"column:runtime_config;type:jsonb;not null;default:'{}'"`
	Permissions     JSON           `gorm:"type:jsonb;not null;default:'{}'"`
	Resources       JSON           `gorm:"type:jsonb;not null;default:'{}'"`

	SandboxTools     pq.StringArray `gorm:"type:text[];default:'{}'"` // enabled sandbox tools (e.g. "chrome")
	SetupCommands    pq.StringArray `gorm:"type:text[];default:'{}'"` // shell commands run during sandbox creation
	EncryptedEnvVars []byte         `gorm:"type:bytea"`               // AES-256-GCM encrypted JSON map of env vars

	Status        string `gorm:"not null;default:'active'"` // draft, active, archived
	IsSystem      bool   `gorm:"-"`
	IsManaged     bool   `gorm:"-"`
	ProviderGroup string `gorm:"-"`

	LastMemoryRefreshedAt *time.Time `gorm:"type:timestamptz"`
	MemoryRefreshStatus   string     `gorm:"type:varchar(32);not null;default:''"` // queued, running, succeeded, failed
	MemoryRefreshError    string     `gorm:"type:text;not null;default:''"`

	LastProxyTokenRefreshedAt *time.Time `gorm:"type:timestamptz"`

	CreatedAt time.Time
	UpdatedAt time.Time
}

func (Agent) TableName() string { return "agents" }

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

	{ID: "memory_recall", Name: "Recall memory", Description: "Search long-term memory for relevant context from past conversations.", Category: "memory", Locked: true},
	{ID: "memory_retain", Name: "Retain memory", Description: "Store important information to long-term memory.", Category: "memory", Locked: true},
	{ID: "memory_reflect", Name: "Reflect on memory", Description: "Get a synthesized answer by analyzing full memory.", Category: "memory", Locked: true},
	{ID: "memory_forget", Name: "Forget memory", Description: "Delete one long-term memory document by exact document ID.", Category: "memory", Locked: true},
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
