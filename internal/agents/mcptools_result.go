package agents

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/google/uuid"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/model"
	"github.com/usehivy/hivy/internal/pluginresolve"
)

// --- output ------------------------------------------------------------------

func agentResultJSON(ctx context.Context, db *gorm.DB, agent *model.Agent, frontendURL string, skillSlugs []string, runtime model.JSON, mcpAllow []string) (*mcp.CallToolResult, error) {
	// Load the persisted plugin/skill/tool state so update responses reflect the
	// stored values, not just the request.
	pluginSlugs, err := agentPluginSlugs(ctx, db, agent)
	if err != nil {
		return toolError(err.Error()), nil
	}
	skillsOut := agentSkillSlugs(agent)
	toolsOut := agentToolIDs(agent)
	subs, err := agentSubAgents(ctx, db, agent.ID)
	if err != nil {
		return toolError(err.Error()), nil
	}
	return toolJSON(map[string]any{
		"agent": map[string]any{
			"id":         agent.ID.String(),
			"name":       agent.Name,
			"status":     agent.Status,
			"model":      agent.Model,
			"plugins":    pluginSlugs,
			"skills":     skillsOut,
			"tools":      toolsOut,
			"sub_agents": subs,
		},
		"url": agentURL(frontendURL, agent.ID),
	})
}

func agentPluginSlugs(ctx context.Context, db *gorm.DB, agent *model.Agent) ([]string, error) {
	slugs, err := pluginresolve.EffectivePluginSlugs(ctx, db, *agent)
	if err != nil {
		return nil, fmt.Errorf("load agent plugins: %w", err)
	}
	if slugs == nil {
		slugs = []string{}
	}
	return slugs, nil
}

func agentSkillSlugs(agent *model.Agent) []string {
	filter := skillFilterAllow(agent.Skills)
	if filter == nil {
		return []string{}
	}
	sort.Strings(filter)
	return filter
}

// agentToolIDs reports the tool ids to echo back to the agent-builder, made
// round-trip-safe against the schema for the agent's kind. For a sub-agent
// (full enum) it reports every enabled runtime tool plus the filter allow list.
// For a parent it reports only parent-assignable ids: the optional runtime tools
// actually enabled (baseline and subagent_task are auto-granted and omitted) and
// the parent-assignable MCP grants derived from the filter — so the returned
// list can be re-sent to update_agent without tripping the parent enum.
func agentToolIDs(agent *model.Agent) []string {
	if agent.Type == model.AgentTypeSubAgent {
		return subAgentToolIDs(agent)
	}
	return parentToolIDs(agent)
}

func subAgentToolIDs(agent *model.Agent) []string {
	out := make([]string, 0, len(agent.Tools))
	for id, enabled := range agent.Tools {
		if b, ok := enabled.(bool); ok && b {
			out = append(out, id)
		}
	}
	if agent.McpToolFilter != nil {
		out = append(out, agent.McpToolFilter.Allow...)
	}
	sort.Strings(out)
	return out
}

func parentToolIDs(agent *model.Agent) []string {
	out := make([]string, 0)
	// Optional runtime tools only (e.g. lsp); baseline + subagent_task omitted.
	for id, enabled := range agent.Tools {
		if b, ok := enabled.(bool); ok && b && optionalRuntimeToolSet[id] {
			out = append(out, id)
		}
	}
	out = append(out, parentGrantedMCPTools(agent.McpToolFilter)...)
	sort.Strings(out)
	return out
}

// parentGrantedMCPTools derives the parent-assignable MCP tools a parent agent
// currently has from its explicit allow-list. A missing or deny-only legacy
// filter grants nothing: the compiler is deny-by-default.
func parentGrantedMCPTools(filter *model.ToolFilter) []string {
	if filter == nil || len(filter.Allow) == 0 {
		return []string{}
	}
	out := make([]string, 0, len(filter.Allow))
	for _, id := range filter.Allow {
		if parentAssignableMCPToolSet[id] {
			out = append(out, id)
		}
	}
	return out
}

func agentSubAgents(ctx context.Context, db *gorm.DB, parentID uuid.UUID) ([]map[string]any, error) {
	var rows []model.Agent
	if err := db.WithContext(ctx).
		Where("parent_agent_id = ? AND type = ? AND status <> ?", parentID, model.AgentTypeSubAgent, "archived").
		Order("created_at ASC").Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("load sub-agents: %w", err)
	}
	out := make([]map[string]any, 0, len(rows))
	for _, row := range rows {
		out = append(out, map[string]any{"id": row.ID.String(), "name": row.Name})
	}
	return out, nil
}

// skillFilterAllow extracts the allow list from the agent Skills jsonb using
// the {"skill_filter":{"allow":[...]}} shape (with a top-level "allow"
// fallback), matching skills.skillFilterFromAgentSkills.
func skillFilterAllow(skills model.JSON) []string {
	if len(skills) == 0 {
		return nil
	}
	if raw, ok := skills["skill_filter"]; ok {
		return allowListFromAny(raw)
	}
	if _, ok := skills["allow"]; ok {
		return allowListFromAny(map[string]any(skills))
	}
	return nil
}

func allowListFromAny(raw any) []string {
	body, err := json.Marshal(raw)
	if err != nil {
		return nil
	}
	var payload struct {
		Allow []string `json:"allow"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil
	}
	return payload.Allow
}

func agentURL(frontendURL string, agentID uuid.UUID) string {
	base := strings.TrimRight(strings.TrimSpace(frontendURL), "/")
	return base + "/w/settings/agents/edit/" + agentID.String()
}

// --- helpers -----------------------------------------------------------------

func allowFilter(allow []string) *model.ToolFilter {
	return &model.ToolFilter{Allow: append([]string{}, allow...)}
}

// mergeBaselineRuntime unions the always-granted baseline sandbox tools into a
// parent agent's runtime tool map so a top-level agent can never end up without
// the core sandbox tools (the incident: an agent that picked only skill tools
// got an empty runtime map and zero built-ins).
func mergeBaselineRuntime(runtime model.JSON) {
	for _, id := range model.BaselineRuntimeToolIDs {
		runtime[id] = true
	}
}

// parentAllowFilter builds the MCP filter for a parent agent as an explicit
// allow-list. Plugin installation controls availability, never implicit tool
// authorization; a Sheets or Apps plugin only exposes a tool the parent chose.
func parentAllowFilter(mcpAllow []string) *model.ToolFilter {
	return allowFilter(mcpAllow)
}

// unionReadOnlyFloor adds the universal MCP floor to a sub-agent allow list so
// it can still load the skills listed in its system prompt.
func unionReadOnlyFloor(allow []string) []string {
	seen := stringSet(allow)
	out := append([]string(nil), allow...)
	for _, id := range model.ReadOnlyMCPToolFloor {
		if !seen[id] {
			seen[id] = true
			out = append(out, id)
		}
	}
	sort.Strings(out)
	return out
}

// hasActiveSubAgents reports whether the parent agent still has active sub-agent
// rows, used to decide whether a tools-only update should keep subagent_task.
func hasActiveSubAgents(ctx context.Context, db *gorm.DB, parentID uuid.UUID) (bool, error) {
	var count int64
	if err := db.WithContext(ctx).Model(&model.Agent{}).
		Where("parent_agent_id = ? AND type = ? AND status = ?", parentID, model.AgentTypeSubAgent, "active").
		Count(&count).Error; err != nil {
		return false, fmt.Errorf("count sub-agents: %w", err)
	}
	return count > 0, nil
}
