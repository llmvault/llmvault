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
	return &model.ToolFilter{
		Allow: normalizeOptionalStrings(payload.Allow),
		Deny:  normalizeOptionalStrings(payload.Deny),
	}
}

func normalizeToolFilter(filter *model.ToolFilter) *model.ToolFilter {
	if filter == nil || (filter.Allow == nil && filter.Deny == nil) {
		return nil
	}
	return &model.ToolFilter{
		Allow: normalizeStringsPreservingNil(filter.Allow),
		Deny:  normalizeStringsPreservingNil(filter.Deny),
	}
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
