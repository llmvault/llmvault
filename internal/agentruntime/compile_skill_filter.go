package agentruntime

import (
	"context"
	"encoding/json"
	"sort"
	"strings"

	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/model"
)

type toolFilterJSON struct {
	Allow *[]string `json:"allow"`
	Deny  *[]string `json:"deny"`
}

// ResolveAgentMCPToolFilter is the single policy path for both runtime config
// compilation and the JTI-scoped Hivy MCP server. It always returns an
// allow-list: every MCP capability other than the universal skill_view tool
// must be explicitly granted by the catalog or agent configuration.
func ResolveAgentMCPToolFilter(ctx context.Context, db *gorm.DB, agent *model.Agent) *model.ToolFilter {
	if agent == nil {
		return compileMCPToolFilter(nil)
	}
	if agent.AgentCatalog != nil {
		if filter := mcpToolFilterFromCatalogManifest(agent.AgentCatalog.Manifest); filter != nil {
			return filter
		}
	}
	if db != nil && agent.AgentCatalogID != nil {
		var catalog model.AgentCatalog
		if err := db.WithContext(ctx).
			Select("manifest").
			Where("id = ? AND status = ?", *agent.AgentCatalogID, model.AgentCatalogStatusActive).
			First(&catalog).Error; err == nil {
			if filter := mcpToolFilterFromCatalogManifest(catalog.Manifest); filter != nil {
				return filter
			}
		}
	}
	// User-created agents carry their own MCP tool filter. A nil filter has
	// allow-all semantics in the runtime, so the compiler must never emit nil
	// for an agent that has not explicitly granted MCP capabilities.
	return compileMCPToolFilter(agent.McpToolFilter)
}

func resolveAgentMCPToolFilter(ctx context.Context, db *gorm.DB, agent *model.Agent) *model.ToolFilter {
	return ResolveAgentMCPToolFilter(ctx, db, agent)
}

// compileMCPToolFilter applies the platform's deny-by-default policy. The
// runtime treats nil as unrestricted, so the compiler never emits nil; an
// otherwise empty capability set receives only universal skill_view below.
func compileMCPToolFilter(filter *model.ToolFilter) *model.ToolFilter {
	return normalizeToolFilter(filter)
}

func mcpToolFilterFromCatalogManifest(raw model.RawJSON) *model.ToolFilter {
	if strings.TrimSpace(string(raw)) == "" {
		return nil
	}
	var payload struct {
		McpToolFilter *toolFilterJSON `json:"mcp_tool_filter"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil || payload.McpToolFilter == nil {
		return nil
	}
	return toolFilterFromPayload(payload.McpToolFilter)
}

func toolFilterFromPayload(payload *toolFilterJSON) *model.ToolFilter {
	if payload == nil || (payload.Allow == nil && payload.Deny == nil) {
		return nil
	}
	// Route catalog manifest filters through the same normalization choke point
	// as user-created and sub-agent filters so the explicit allow-list policy is
	// identical at every entry point.
	return normalizeToolFilter(&model.ToolFilter{
		Allow: normalizeOptionalStrings(payload.Allow),
		Deny:  normalizeOptionalStrings(payload.Deny),
	})
}

// normalizeToolFilter is the single choke point every compiled MCP tool filter
// flows through: user-created agent filters, catalog manifest filters, and
// sub-agent filters. It deliberately compiles every input to an allow-list. A
// legacy deny-only filter therefore grants no optional MCP capability: deny
// rules can only subtract from capabilities that were explicitly allowed.
func normalizeToolFilter(filter *model.ToolFilter) *model.ToolFilter {
	allow := []string{}
	deny := []string{}
	if filter != nil {
		allow = normalizeStringsPreservingNil(filter.Allow)
		deny = normalizeStringsPreservingNil(filter.Deny)
	}
	denied := make(map[string]bool, len(deny))
	for _, id := range deny {
		denied[id] = true
	}
	filtered := make([]string, 0, len(allow))
	for _, id := range allow {
		if !denied[id] {
			filtered = append(filtered, id)
		}
	}
	return applyReadOnlyMCPToolFloor(&model.ToolFilter{Allow: filtered})
}

// applyReadOnlyMCPToolFloor adds the small universal MCP surface to every
// compiled filter. Today that surface is only skill_view: the available-skill
// inventory is already rendered into the system prompt, so skills_list is not
// needed, and channel/automation tools stay explicit Hivy-default grants.
func applyReadOnlyMCPToolFloor(filter *model.ToolFilter) *model.ToolFilter {
	if filter == nil {
		return filter
	}
	present := make(map[string]bool, len(filter.Allow))
	for _, id := range filter.Allow {
		present[id] = true
	}
	changed := false
	for _, id := range model.ReadOnlyMCPToolFloor {
		if present[id] {
			continue
		}
		filter.Allow = append(filter.Allow, id)
		present[id] = true
		changed = true
	}
	if changed {
		sort.Strings(filter.Allow)
	}
	return filter
}

func normalizeOptionalStrings(values *[]string) []string {
	if values == nil {
		return nil
	}
	return normalizeStringsPreservingNil(*values)
}

func normalizeStringsPreservingNil(values []string) []string {
	if values == nil {
		return nil
	}
	out := make([]string, 0, len(values))
	seen := map[string]bool{}
	for _, name := range values {
		name = strings.TrimSpace(name)
		if name == "" || seen[name] {
			continue
		}
		seen[name] = true
		out = append(out, name)
	}
	sort.Strings(out)
	return out
}
