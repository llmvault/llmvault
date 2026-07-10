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
// on the "hivy" MCP server (see internal/webcrawl, internal/skills,
// internal/rag, internal/mcpserver/cron_tool.go, internal/mcpserver/http_trigger_tool.go,
// internal/mcpserver/list_channels_tool.go, internal/handler/images_mcp.go).
// The privileged agent-builder tools (create_agent, update_agent) and the
// read-only list_org_plugins are intentionally NOT assignable.
var AssignableMCPTools = []string{
	"web_search",
	"web_fetch",
	"generate_image",
	"generate_vector_image",
	"remix_image",
	"skills_list",
	"skill_view",
	"search_knowledge_base",
	"cron",
	"create_http_trigger",
	"list_channels",
}

var assignableMCPToolSet = func() map[string]bool {
	out := make(map[string]bool, len(AssignableMCPTools))
	for _, id := range AssignableMCPTools {
		out[id] = true
	}
	return out
}()

func stringSet(ids []string) map[string]bool {
	out := make(map[string]bool, len(ids))
	for _, id := range ids {
		out[id] = true
	}
	return out
}

var baselineRuntimeToolSet = stringSet(model.BaselineRuntimeToolIDs)

var readOnlyMCPFloorSet = stringSet(model.ReadOnlyMCPToolFloor)

// optionalRuntimeToolIDs are the runtime built-in tools a parent agent may opt
// into: everything in RuntimeBuiltInToolIDs that is not always-granted baseline
// and not subagent_task (which is granted automatically when sub_agents exist).
// Derived by set subtraction so it can never drift from the source lists. In
// practice this is exactly ["lsp"].
func optionalRuntimeToolIDs() []string {
	out := make([]string, 0)
	for _, id := range model.RuntimeBuiltInToolIDs {
		if baselineRuntimeToolSet[id] || id == "subagent_task" {
			continue
		}
		out = append(out, id)
	}
	return out
}

var optionalRuntimeToolSet = stringSet(optionalRuntimeToolIDs())

// parentAssignableMCPTools are the MCP tools a parent agent may opt into: all
// AssignableMCPTools minus the read-only floor (which is always usable and must
// never be listed by the builder).
func parentAssignableMCPTools() []string {
	out := make([]string, 0, len(AssignableMCPTools))
	for _, id := range AssignableMCPTools {
		if readOnlyMCPFloorSet[id] {
			continue
		}
		out = append(out, id)
	}
	return out
}

var parentAssignableMCPToolSet = stringSet(parentAssignableMCPTools())

// ParentAssignableToolIDs is the set of tools a top-level (parent) agent may be
// granted via the agent-builder `tools` array: the optional runtime tools plus
// the parent-assignable MCP tools. Baseline sandbox tools and the read-only
// skill/channel floor are always granted and are intentionally excluded, so they
// never appear in the parent schema enum. Derived from the source lists by set
// subtraction. In practice: lsp, web_search, web_fetch, generate_image,
// generate_vector_image, remix_image, search_knowledge_base,
// cron, create_http_trigger.
func ParentAssignableToolIDs() []string {
	runtime := optionalRuntimeToolIDs()
	mcp := parentAssignableMCPTools()
	out := make([]string, 0, len(runtime)+len(mcp))
	out = append(out, runtime...)
	out = append(out, mcp...)
	return out
}

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

// enumValues converts a list of ids to []any for a JSON schema enum.
func enumValues(ids []string) []any {
	out := make([]any, len(ids))
	for i, id := range ids {
		out[i] = id
	}
	return out
}

// toolEnumValues returns AssignableToolIDs as []any for a JSON schema enum.
func toolEnumValues() []any {
	return enumValues(AssignableToolIDs())
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
