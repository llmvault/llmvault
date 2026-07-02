package agents

import (
	"context"
	"errors"
	"fmt"
	"sort"
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
		OrgID:           &orgIDCopy,
		Name:            name,
		Description:     &desc,
		Instructions:    &instructions,
		IsDefault:       false,
		SandboxImage:    model.SandboxImageDefault,
		SandboxSize:     model.DefaultAgentSandboxSize,
		Model:           modelID,
		AvailableModels: pq.StringArray{modelID},
		Tools:           tools,
		McpToolFilter:   in.McpToolFilter,
		McpServers:      model.RawJSON("[]"),
		Skills:          skills,
		Permissions:     permissions,
		Resources:       model.JSON{},
		RuntimeConfig:   model.JSON{},
		SandboxTools:    pq.StringArray(sandboxTools),
		Status:          "active",
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

// UpdateAgent applies a true patch to an org-owned agent. Only non-nil fields
// change; provided arrays replace. Sub-agents are delete+recreate when
// provided. Verifies org ownership.
func UpdateAgent(ctx context.Context, deps Deps, orgID, agentID uuid.UUID, in UpdateInput) (*model.Agent, error) {
	if deps.DB == nil {
		return nil, fmt.Errorf("agents: nil DB")
	}
	var agent model.Agent
	if err := deps.DB.WithContext(ctx).
		Where("id = ? AND org_id = ? AND status <> ?", agentID, orgID, "archived").
		First(&agent).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrAgentNotFound
		}
		return nil, err
	}

	updates := map[string]any{}
	if in.Name != nil {
		name := strings.TrimSpace(*in.Name)
		if name == "" {
			return nil, fmt.Errorf("name cannot be empty")
		}
		if agent.IsDefault && name != agent.Name {
			return nil, fmt.Errorf("default agent cannot be renamed")
		}
		updates["name"] = name
		agent.Name = name
	}
	if in.Description != nil {
		value := strings.TrimSpace(*in.Description)
		updates["description"] = value
		agent.Description = &value
	}
	if in.Instructions != nil {
		value := strings.TrimSpace(*in.Instructions)
		updates["instructions"] = value
		agent.Instructions = &value
	}
	if in.Status != nil {
		status := strings.TrimSpace(*in.Status)
		if status != "active" && status != "archived" {
			return nil, fmt.Errorf("status must be active or archived")
		}
		updates["status"] = status
		agent.Status = status
	}
	if in.Tools != nil {
		value := *in.Tools
		if value == nil {
			value = model.JSON{}
		}
		updates["tools"] = value
		agent.Tools = value
	}
	if in.SetMcpFilter {
		updates["mcp_tool_filter"] = in.McpToolFilter
		agent.McpToolFilter = in.McpToolFilter
	}
	if in.Skills != nil {
		value := *in.Skills
		if value == nil {
			value = model.JSON{}
		}
		updates["skills"] = value
		agent.Skills = value
	}

	var subRows []model.Agent
	if in.SubAgents != nil {
		var err error
		subRows, err = buildSubAgentRows(ctx, deps, orgID, agent.Model, *in.SubAgents)
		if err != nil {
			return nil, err
		}
	}

	if err := deps.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if len(updates) > 0 {
			if err := tx.Model(&model.Agent{}).
				Where("id = ? AND org_id = ?", agent.ID, orgID).
				Updates(updates).Error; err != nil {
				return err
			}
		}
		if in.SubAgents != nil {
			if err := tx.Where("parent_agent_id = ? AND type = ?", agent.ID, model.AgentTypeSubAgent).
				Delete(&model.Agent{}).Error; err != nil {
				return err
			}
			for i := range subRows {
				subRows[i].ParentAgentID = &agent.ID
				if err := tx.Create(&subRows[i]).Error; err != nil {
					return err
				}
			}
		}
		if in.SetPlugins {
			if err := replacePlugins(ctx, tx, orgID, agent.ID, in.PluginIDs); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		return nil, mapWriteError(err)
	}
	return &agent, nil
}

// BuildSubAgentRows validates sub-agent inputs and returns unsaved agent rows
// (ParentAgentID left nil), so the HTTP handler and the create/update service
// share one implementation. Supports skills (Skills jsonb) in addition to
// tools and the MCP filter.
func BuildSubAgentRows(ctx context.Context, deps Deps, orgID uuid.UUID, parentModel string, inputs []SubAgentInput) ([]model.Agent, error) {
	return buildSubAgentRows(ctx, deps, orgID, parentModel, inputs)
}

// buildSubAgentRows validates sub-agent inputs and returns unsaved agent rows
// (ParentAgentID left nil). Supports skills (Skills jsonb) in addition to tools
// and the MCP filter.
func buildSubAgentRows(ctx context.Context, deps Deps, orgID uuid.UUID, parentModel string, inputs []SubAgentInput) ([]model.Agent, error) {
	if len(inputs) == 0 {
		return nil, nil
	}
	orgIDCopy := orgID
	rows := make([]model.Agent, 0, len(inputs))
	seen := map[string]bool{}
	for _, in := range inputs {
		name := strings.TrimSpace(in.Name)
		if name == "" {
			return nil, fmt.Errorf("sub-agent name is required")
		}
		if seen[name] {
			return nil, fmt.Errorf("duplicate sub-agent name %q", name)
		}
		seen[name] = true

		subModel := strings.TrimSpace(in.Model)
		if subModel == "" {
			subModel = parentModel
		} else if err := deps.validateModel(ctx, orgID, subModel); err != nil {
			return nil, fmt.Errorf("sub-agent %q: %s", name, err.Error())
		}

		tools := in.Tools
		if tools == nil {
			tools = model.JSON{}
		}
		skills := in.Skills
		if skills == nil {
			skills = model.JSON{}
		}
		desc := strings.TrimSpace(in.Description)
		instructions := strings.TrimSpace(in.Instructions)
		rows = append(rows, model.Agent{
			OrgID:           &orgIDCopy,
			Type:            model.AgentTypeSubAgent,
			Name:            name,
			Description:     &desc,
			Instructions:    &instructions,
			Model:           subModel,
			AvailableModels: pq.StringArray{subModel},
			Tools:           tools,
			McpServers:      model.RawJSON("[]"),
			McpToolFilter:   subAgentFilter(in.McpAllow, in.McpDeny),
			Skills:          skills,
			Permissions:     model.JSON{},
			Resources:       model.JSON{},
			RuntimeConfig:   model.JSON{},
			SandboxImage:    model.SandboxImageDefault,
			SandboxSize:     model.DefaultAgentSandboxSize,
			Status:          "active",
		})
	}
	return rows, nil
}

func subAgentFilter(allow, deny []string) *model.ToolFilter {
	allow = dedupeNonEmpty(allow)
	deny = dedupeNonEmpty(deny)
	if len(allow) == 0 && len(deny) == 0 {
		return nil
	}
	return &model.ToolFilter{Allow: allow, Deny: deny}
}

// attachPlugins enables each requested plugin on the agent (idempotent).
func attachPlugins(ctx context.Context, tx *gorm.DB, orgID, agentID uuid.UUID, pluginIDs []uuid.UUID) error {
	for _, pluginID := range dedupeUUIDs(pluginIDs) {
		install := model.AgentPluginInstall{OrgID: orgID, AgentID: agentID, PluginID: pluginID}
		if err := tx.WithContext(ctx).
			Clauses(onConflictDoNothing()).
			Create(&install).Error; err != nil {
			return fmt.Errorf("enable plugin for agent: %w", err)
		}
	}
	return nil
}

// replacePlugins sets the agent's plugin installs to exactly the auto-installed
// set plus the requested plugin ids. It removes any currently-enabled plugin
// that is neither auto-installed nor requested.
func replacePlugins(ctx context.Context, tx *gorm.DB, orgID, agentID uuid.UUID, pluginIDs []uuid.UUID) error {
	want := map[uuid.UUID]bool{}
	for _, id := range pluginIDs {
		want[id] = true
	}
	// Auto-installed plugins are locked on the agent and must never be removed.
	locked, err := lockedPluginIDs(ctx, tx, orgID)
	if err != nil {
		return err
	}
	for id := range locked {
		want[id] = true
	}

	var existing []model.AgentPluginInstall
	if err := tx.WithContext(ctx).Where("org_id = ? AND agent_id = ?", orgID, agentID).Find(&existing).Error; err != nil {
		return err
	}
	have := map[uuid.UUID]bool{}
	for _, install := range existing {
		have[install.PluginID] = true
	}

	for id := range want {
		if have[id] {
			continue
		}
		install := model.AgentPluginInstall{OrgID: orgID, AgentID: agentID, PluginID: id}
		if err := tx.WithContext(ctx).Clauses(onConflictDoNothing()).Create(&install).Error; err != nil {
			return fmt.Errorf("enable plugin for agent: %w", err)
		}
	}
	for id := range have {
		if want[id] {
			continue
		}
		if err := tx.WithContext(ctx).
			Where("org_id = ? AND agent_id = ? AND plugin_id = ?", orgID, agentID, id).
			Delete(&model.AgentPluginInstall{}).Error; err != nil {
			return err
		}
	}
	return nil
}

// lockedPluginIDs returns the active auto-install plugin ids installed for the
// org; these are attached to every agent and must not be removed on update.
func lockedPluginIDs(ctx context.Context, tx *gorm.DB, orgID uuid.UUID) (map[uuid.UUID]bool, error) {
	var installs []model.OrgPluginInstall
	if err := tx.WithContext(ctx).
		Preload("Plugin").
		Where("org_id = ? AND revoked_at IS NULL", orgID).
		Find(&installs).Error; err != nil {
		return nil, err
	}
	out := map[uuid.UUID]bool{}
	for _, install := range installs {
		if pluginstore.PluginAutoInstall(install.Plugin) {
			out[install.PluginID] = true
		}
	}
	return out, nil
}

func mapWriteError(err error) error {
	if isDuplicateKeyError(err) {
		return ErrDuplicateName
	}
	return err
}

func dedupeUUIDs(ids []uuid.UUID) []uuid.UUID {
	seen := map[uuid.UUID]bool{}
	out := make([]uuid.UUID, 0, len(ids))
	for _, id := range ids {
		if id == uuid.Nil || seen[id] {
			continue
		}
		seen[id] = true
		out = append(out, id)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].String() < out[j].String() })
	return out
}
