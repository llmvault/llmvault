package agents

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/lib/pq"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/model"
	pluginstore "github.com/usehivy/hivy/internal/plugins"
)

// ErrDuplicateName is returned by CreateAgent/UpdateAgent when the agent (or a
// sub-agent) name collides with an existing row. Callers map it to their own
// conflict semantics (HTTP 409, MCP error text).
var ErrDuplicateName = errors.New("agent name already exists")

// ErrAgentNotFound is returned by UpdateAgent when the target agent is not an
// active agent owned by the org.
var ErrAgentNotFound = errors.New("agent not found")

// Deps carries the collaborators the create/update core needs without pulling
// in the HTTP handler. ValidateModel checks a model id is selectable for the
// org (credentials + text output); DefaultModel is the fallback model.
type Deps struct {
	DB            *gorm.DB
	DefaultModel  string
	ValidateModel func(ctx context.Context, orgID uuid.UUID, modelID string) error
	// Models is the set of canonical model ids an agent-builder caller may
	// select (the full agent-selectable catalog minus the scribe model). It is
	// used verbatim as the `model` schema enum and to reject unknown models with
	// a helpful message. Empty disables model selection (falls back to default).
	Models []string
}

func (d Deps) validateModel(ctx context.Context, orgID uuid.UUID, modelID string) error {
	if d.ValidateModel == nil {
		return nil
	}
	return d.ValidateModel(ctx, orgID, modelID)
}

// SubAgentInput is one sub-agent supplied to CreateAgent/UpdateAgent. Tools and
// McpAllow are already resolved by the caller (the HTTP handler passes its
// normalized values; the MCP layer routes its string enum through SplitTools).
// Skills is the sub-agent's resolved Skills jsonb.
type SubAgentInput struct {
	Name         string
	Description  string
	Instructions string
	Model        string // "" inherits the parent model
	Tools        model.JSON
	McpAllow     []string
	McpDeny      []string
	Skills       model.JSON
}

// CreateInput is the resolved payload for CreateAgent. All tool/skill/plugin
// values are pre-validated and normalized by the caller.
type CreateInput struct {
	Name         string
	Description  string
	Instructions string
	Model        string // "" uses Deps.DefaultModel

	Tools         model.JSON
	McpToolFilter *model.ToolFilter
	Skills        model.JSON

	// PluginIDs are additional org-installed plugins to enable on the agent
	// beyond the auto-installed set.
	PluginIDs []uuid.UUID

	SubAgents []SubAgentInput
}

// UpdateInput is the resolved patch for UpdateAgent. Nil pointers mean "leave
// unchanged"; a non-nil pointer replaces that field. SubAgents is applied as a
// delete+recreate when non-nil.
type UpdateInput struct {
	Name         *string
	Description  *string
	Instructions *string
	Model        *string // non-nil sets the agent's model
	Status       *string

	Tools         *model.JSON
	McpToolFilter *model.ToolFilter // non-nil replaces (nil pointer = leave; empty filter = clear)
	SetMcpFilter  bool              // when true, McpToolFilter (even nil) is written
	Skills        *model.JSON

	// SetPlugins, when true, replaces the agent's non-locked plugin installs
	// with PluginIDs.
	SetPlugins bool
	PluginIDs  []uuid.UUID

	SubAgents *[]SubAgentInput
}

// CreateAgent persists a top-level agent, its sub-agents, requested plugin
// installs, and the auto-installed plugin set, in one transaction. It mirrors
// the HTTP create handler's defaults so both paths behave identically.
func CreateAgent(ctx context.Context, deps Deps, orgID uuid.UUID, in CreateInput) (*model.Agent, error) {
	if deps.DB == nil {
		return nil, fmt.Errorf("agents: nil DB")
	}
	name := strings.TrimSpace(in.Name)
	if name == "" {
		return nil, fmt.Errorf("name is required")
	}
	modelID := strings.TrimSpace(in.Model)
	if modelID == "" {
		modelID = deps.DefaultModel
	}
	if err := deps.validateModel(ctx, orgID, modelID); err != nil {
		return nil, err
	}

	subRows, err := buildSubAgentRows(ctx, deps, orgID, modelID, in.SubAgents)
	if err != nil {
		return nil, err
	}

	tools := in.Tools
	if tools == nil {
		tools = model.JSON{}
	}
	if len(subRows) > 0 {
		tools["subagent_task"] = true
	}

	// Permissions and sandbox tools default to the canonical "all enabled" set,
	// matching the HTTP create handler.
	permissions := model.JSON{}
	for _, id := range model.BuiltInToolIDs() {
		permissions[id] = true
	}
	sandboxTools := make([]string, 0, len(model.ValidSandboxTools))
	for _, tool := range model.ValidSandboxTools {
		sandboxTools = append(sandboxTools, tool.ID)
	}

	desc := strings.TrimSpace(in.Description)
	instructions := strings.TrimSpace(in.Instructions)
	skills := in.Skills
	if skills == nil {
		skills = model.JSON{}
	}
	orgIDCopy := orgID
	agent := model.Agent{
		OrgID:         &orgIDCopy,
		Name:          name,
		Description:   &desc,
		Instructions:  &instructions,
		IsDefault:     false,
		SandboxImage:  model.SandboxImageDefault,
		SandboxSize:   model.DefaultAgentSandboxSize,
		Model:         modelID,
		Tools:         tools,
		McpToolFilter: in.McpToolFilter,
		McpServers:    model.RawJSON("[]"),
		Skills:        skills,
		Permissions:   permissions,
		Resources:     model.JSON{},
		RuntimeConfig: model.JSON{},
		SandboxTools:  pq.StringArray(sandboxTools),
		Status:        "active",
	}

	if err := deps.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&agent).Error; err != nil {
			return err
		}
		for i := range subRows {
			subRows[i].ParentAgentID = &agent.ID
			if err := tx.Create(&subRows[i]).Error; err != nil {
				return err
			}
		}
		if err := pluginstore.EnsureAutoInstalledForAgent(ctx, tx, orgID, agent.ID); err != nil {
			return err
		}
		return attachPlugins(ctx, tx, orgID, agent.ID, in.PluginIDs)
	}); err != nil {
		return nil, mapWriteError(err)
	}
	agent.OrgID = &orgIDCopy
	return &agent, nil
}
