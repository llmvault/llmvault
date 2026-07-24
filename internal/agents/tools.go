// Package agents holds the reusable agent create/update service shared by the
// HTTP handlers and the agent-builder MCP tools, plus the strict tool/skill/
// connection and skill routing those callers depend on.
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
// IMPORTANT: this list must stay in sync with the optional MCP tools actually
// registered on the "hivy" MCP server (see internal/webcrawl,
// internal/sheets, internal/apps, and internal/handler/images_mcp.go).
// Baseline parent tools and default-Hivy management tools are intentionally
// not assignable.
var AssignableMCPTools = []string{
	"web_search",
	"web_fetch",
	"web_crawl",
	"generate_image",
	"generate_vector_image",
	"remix_image",
	"vectorize_image",
	"sheet_create",
	"sheet_list",
	"sheet_describe",
	"sheet_manage",
	"rows_query",
	"rows_write",
	"sheet_import_csv",
	"sheet_operations",
	"app_create",
	"app_publish",
	"app_status",
	"app_logs",
	"app_rollback",
	"send_email",
	"email_read",
	"email_search",
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

var baselineParentMCPToolSet = stringSet(model.BaselineParentMCPToolIDs)

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

// parentAssignableMCPTools are the MCP tools a parent agent may opt into.
func parentAssignableMCPTools() []string {
	out := make([]string, 0, len(AssignableMCPTools))
	for _, id := range AssignableMCPTools {
		if baselineParentMCPToolSet[id] {
			continue
		}
		out = append(out, id)
	}
	return out
}

var parentAssignableMCPToolSet = stringSet(parentAssignableMCPTools())

// ParentAssignableToolIDs is the set of tools a top-level (parent) agent may be
// granted via the agent-builder `tools` array: the optional runtime tools plus
// the explicitly selectable MCP tools. Baseline sandbox tools and universal
// skill_view are intentionally excluded, so they never appear in the parent
// schema enum.
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

// SubAgentAssignableToolIDs is the tool catalog available to a sub-agent.
// Drive files belong to the parent sandbox and are intentionally never exposed
// to sub-agents.
func SubAgentAssignableToolIDs() []string {
	out := make([]string, 0, len(model.RuntimeBuiltInToolIDs)+len(AssignableMCPTools)-2)
	for _, id := range model.RuntimeBuiltInToolIDs {
		if id == "drive_upload" || id == "drive_download" {
			continue
		}
		out = append(out, id)
	}
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
