package agents

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/model"
	pluginstore "github.com/usehivy/hivy/internal/plugins"
)

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
			OrgID:          &orgIDCopy,
			Type:           model.AgentTypeSubAgent,
			Name:           name,
			Description:    &desc,
			Instructions:   &instructions,
			Model:          subModel,
			Tools:          tools,
			McpServers:     model.RawJSON("[]"),
			McpToolFilter:  subAgentFilter(in.McpAllow, in.McpDeny),
			Skills:         skills,
			AutoLoadSkills: append(model.AutoLoadSkills(nil), in.AutoLoadSkills...),
			Permissions:    model.JSON{},
			Resources:      model.JSON{},
			RuntimeConfig:  model.JSON{},
			SandboxImage:   model.SandboxImageDefault,
			SandboxSize:    model.DefaultAgentSandboxSize,
			Status:         "active",
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
