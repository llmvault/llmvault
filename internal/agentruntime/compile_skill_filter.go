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

func resolveAgentMCPToolFilter(ctx context.Context, db *gorm.DB, agent *model.Agent) *model.ToolFilter {
	if agent == nil {
		return nil
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
	// User-created agents carry their own MCP tool filter.
	return normalizeToolFilter(agent.McpToolFilter)
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
	// (normalizeToolFilter) as user-created and sub-agent filters so the
	// read-only floor is applied once, everywhere.
	return normalizeToolFilter(&model.ToolFilter{
		Allow: normalizeOptionalStrings(payload.Allow),
		Deny:  normalizeOptionalStrings(payload.Deny),
	})
}

// normalizeToolFilter is the single choke point every compiled MCP tool filter
// flows through: user-created agent filters (resolveAgentMCPToolFilter), catalog
// manifest filters (toolFilterFromPayload), and sub-agent filters
// (compile_subagents.go). It sorts/dedupes the allow and deny lists and applies
// the read-only floor.
func normalizeToolFilter(filter *model.ToolFilter) *model.ToolFilter {
	if filter == nil || (filter.Allow == nil && filter.Deny == nil) {
		return nil
	}
	return applyReadOnlyMCPToolFloor(&model.ToolFilter{
		Allow: normalizeStringsPreservingNil(filter.Allow),
		Deny:  normalizeStringsPreservingNil(filter.Deny),
	})
}

// applyReadOnlyMCPToolFloor keeps allow-list MCP tool filters from locking an
// agent out of read-only skill/channel tools. Allow-list agents built by the old
// agent-builder flow (before this floor existed) would otherwise be denied every
// MCP tool not named in their allow list — including list_channels (needed to
// resolve Hivy channel UUIDs for cron / create_http_trigger) and the skill tools
// (skills are unusable without them). For any filter with a NON-empty allow list
// we union in each model.ReadOnlyMCPToolFloor id that is not already explicitly
// denied; an explicit deny is respected as deliberate and still wins. Nil filters
// and pure deny-list filters (a deny list already permits every unnamed tool) are
// left untouched. Output stays sorted and deduped.
func applyReadOnlyMCPToolFloor(filter *model.ToolFilter) *model.ToolFilter {
	if filter == nil || len(filter.Allow) == 0 {
		return filter
	}
	denied := make(map[string]bool, len(filter.Deny))
	for _, id := range filter.Deny {
		denied[id] = true
	}
	present := make(map[string]bool, len(filter.Allow))
	for _, id := range filter.Allow {
		present[id] = true
	}
	changed := false
	for _, id := range model.ReadOnlyMCPToolFloor {
		if denied[id] || present[id] {
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
