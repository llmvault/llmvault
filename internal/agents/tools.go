// Package agents holds the reusable agent create/update service shared by the
// HTTP handlers and the agent-builder MCP tools, plus the strict tool/skill/
// plugin routing and validation those callers depend on.
package agents

import (
	"fmt"
	"sort"
	"strings"

	"github.com/usehivy/hivy/internal/model"
)

// AssignableMCPTools is the canonical set of MCP tools an agent-builder caller
// may grant to a parent or sub-agent via the shared `tools` array. Values here
// route into the agent's McpToolFilter.Allow list rather than the runtime
// Tools map.
//
// IMPORTANT: this list must stay in sync with the MCP tools actually registered
// on the "hivy" MCP server (see internal/spider, internal/memory, internal/skills,
// internal/rag, internal/mcpserver/cron_tool.go, internal/handler/images_mcp.go).
// The privileged agent-builder tools (create_agent, update_agent) and the
// read-only list_org_plugins are intentionally NOT assignable.
var AssignableMCPTools = []string{
	"web_search",
	"web_fetch",
	"generate_image",
	"generate_vector_image",
	"search_memories",
	"retain_memory",
	"forget_memory",
	"skills_list",
	"skill_view",
	"search_knowledge_base",
	"cron",
}

var assignableMCPToolSet = func() map[string]bool {
	out := make(map[string]bool, len(AssignableMCPTools))
	for _, id := range AssignableMCPTools {
		out[id] = true
	}
	return out
}()

// AssignableToolIDs returns the full union of runtime built-in tool ids and
// assignable MCP tool ids, in a stable order (runtime tools first, then MCP
// tools). It is the exact set used for the `tools.items.enum` in the MCP input
// schemas so agents cannot hallucinate tool names.
func AssignableToolIDs() []string {
	out := make([]string, 0, len(model.RuntimeBuiltInToolIDs)+len(AssignableMCPTools))
	out = append(out, model.RuntimeBuiltInToolIDs...)
	out = append(out, AssignableMCPTools...)
	return out
}

// toolEnumValues returns AssignableToolIDs as []any for a JSON schema enum.
func toolEnumValues() []any {
	ids := AssignableToolIDs()
	out := make([]any, len(ids))
	for i, id := range ids {
		out[i] = id
	}
	return out
}

// SplitTools routes a provided list of tool identifiers into the runtime Tools
// map ({id:true}) and the MCP allow-list, validating every value against the
// canonical union. On an unknown value it returns a helpful error naming the
// offending value and listing every allowed tool.
func SplitTools(tools []string) (runtime model.JSON, mcpAllow []string, err error) {
	runtime = model.JSON{}
	mcpSeen := map[string]bool{}
	for _, raw := range tools {
		id := strings.TrimSpace(raw)
		if id == "" {
			continue
		}
		switch {
		case model.IsValidRuntimeBuiltInToolID(id):
			runtime[id] = true
		case assignableMCPToolSet[id]:
			if !mcpSeen[id] {
				mcpSeen[id] = true
				mcpAllow = append(mcpAllow, id)
			}
		default:
			return nil, nil, fmt.Errorf(
				"unknown tool %q: allowed tools are: %s",
				id, strings.Join(AssignableToolIDs(), ", "),
			)
		}
	}
	sort.Strings(mcpAllow)
	return runtime, mcpAllow, nil
}
