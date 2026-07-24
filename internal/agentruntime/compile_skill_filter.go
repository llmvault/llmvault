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
// allow-list for managed MCP capabilities. Connection MCP servers carry their
// own deny filters and are not governed by this global filter.
func ResolveAgentMCPToolFilter(ctx context.Context, db *gorm.DB, agent *model.Agent) *model.ToolFilter {
	if agent == nil {
		return applyAgentInboxEmailTools(compileMCPToolFilter(nil), false)
	}
	hasInbox := strings.TrimSpace(agent.EmailInboxLocalPart) != ""
	if agent.AgentCatalog != nil {
		if filter := mcpToolFilterFromCatalogManifest(agent.AgentCatalog.Manifest); filter != nil {
			return applyAgentInboxEmailTools(filter, hasInbox)
		}
	}
	if db != nil && agent.AgentCatalogID != nil {
		var catalog model.AgentCatalog
		if err := db.WithContext(ctx).
			Select("manifest").
			Where("id = ? AND status = ?", *agent.AgentCatalogID, model.AgentCatalogStatusActive).
			First(&catalog).Error; err == nil {
			if filter := mcpToolFilterFromCatalogManifest(catalog.Manifest); filter != nil {
				return applyAgentInboxEmailTools(filter, hasInbox)
			}
		}
	}
	// User-created agents carry their own MCP tool filter. A nil filter has
	// allow-all semantics in the runtime, so the compiler must never emit nil
	// for an agent that has not explicitly granted MCP capabilities.
	return applyAgentInboxEmailTools(compileMCPToolFilter(agent.McpToolFilter), hasInbox)
}

func resolveAgentMCPToolFilter(ctx context.Context, db *gorm.DB, agent *model.Agent) *model.ToolFilter {
	return ResolveAgentMCPToolFilter(ctx, db, agent)
}

// compileMCPToolFilter applies the platform's deny-by-default policy. The
// runtime treats nil as unrestricted, so the compiler never emits nil; an
// otherwise empty optional set still receives the universal parent surface.
func compileMCPToolFilter(filter *model.ToolFilter) *model.ToolFilter {
	return applyMCPToolFloor(normalizeToolFilter(filter), model.BaselineParentMCPToolIDs)
}

// compileSubAgentMCPToolFilter applies the sub-agent-specific universal floor.
// In particular, Drive search is parent-scoped alongside Drive upload/download.
func compileSubAgentMCPToolFilter(filter *model.ToolFilter, hasInbox bool) *model.ToolFilter {
	compiled := applyMCPToolFloor(normalizeToolFilter(filter), model.SubAgentReadOnlyMCPToolFloor)
	if compiled == nil {
		return nil
	}
	compiled = applyAgentInboxEmailTools(compiled, hasInbox)
	parentOnly := make(map[string]bool, len(model.BaselineParentMCPToolIDs))
	for _, id := range model.BaselineParentMCPToolIDs {
		parentOnly[id] = true
	}
	for _, id := range model.SubAgentReadOnlyMCPToolFloor {
		delete(parentOnly, id)
	}
	allow := make([]string, 0, len(compiled.Allow))
	for _, id := range compiled.Allow {
		if !parentOnly[id] {
			allow = append(allow, id)
		}
	}
	compiled.Allow = allow
	return compiled
}

// applyAgentInboxEmailTools makes inbox presence the sole grant for the native
// email capability group. Explicit filters cannot expose unusable email tools
// without an inbox, while a provisioned inbox always exposes the complete group.
func applyAgentInboxEmailTools(filter *model.ToolFilter, hasInbox bool) *model.ToolFilter {
	if filter == nil {
		filter = &model.ToolFilter{Allow: []string{}}
	}
	emailTools := make(map[string]bool, len(model.AgentEmailMCPToolIDs))
	for _, id := range model.AgentEmailMCPToolIDs {
		emailTools[id] = true
	}
	allow := make([]string, 0, len(filter.Allow)+len(model.AgentEmailMCPToolIDs))
	present := make(map[string]bool, len(filter.Allow)+len(model.AgentEmailMCPToolIDs))
	for _, id := range filter.Allow {
		rawID := strings.TrimPrefix(id, "hivy_")
		if emailTools[rawID] {
			continue
		}
		if !present[id] {
			allow = append(allow, id)
			present[id] = true
		}
	}
	if hasInbox {
		for _, id := range model.AgentEmailMCPToolIDs {
			if !present[id] {
				allow = append(allow, id)
				present[id] = true
			}
		}
	}
	sort.Strings(allow)
	filter.Allow = allow
	filter.Deny = nil
	return filter
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
	return applyMCPToolFloor(normalizeToolFilter(&model.ToolFilter{
		Allow: normalizeOptionalStrings(payload.Allow),
		Deny:  normalizeOptionalStrings(payload.Deny),
	}), model.BaselineParentMCPToolIDs)
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
	return &model.ToolFilter{Allow: filtered}
}

// applyMCPToolFloor adds a caller-specific universal MCP surface after optional
// grants and denies have been normalized. Parent and sub-agent floors are kept
// separate because sub-agents execute through their parent's proxy identity.
func applyMCPToolFloor(filter *model.ToolFilter, floor []string) *model.ToolFilter {
	if filter == nil {
		return filter
	}
	present := make(map[string]bool, len(filter.Allow))
	for _, id := range filter.Allow {
		present[id] = true
	}
	changed := false
	for _, id := range floor {
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
