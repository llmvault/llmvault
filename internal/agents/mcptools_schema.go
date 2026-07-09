package agents

import (
	"fmt"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// --- schemas -----------------------------------------------------------------

// parentToolsArraySchema is the `tools` schema for a top-level agent. Its enum
// is only the optional capabilities: baseline sandbox tools and the read-only
// skill/channel tools are always granted and must not be listed here.
func parentToolsArraySchema() map[string]any {
	return map[string]any{
		"type":        "array",
		"description": "Additional capabilities for the agent. Core sandbox tools (bash, file read/write/edit, search, planning) and the read-only skill/channel tools are ALWAYS granted to top-level agents and must not be listed here. subagent_task is granted automatically when sub_agents are defined.",
		"items": map[string]any{
			"type": "string",
			"enum": enumValues(ParentAssignableToolIDs()),
		},
	}
}

// subAgentToolsArraySchema is the `tools` schema for a sub-agent. Its enum is the
// full union so a read-only sub-agent can be expressed by listing just the
// read-only file tools. Sub-agents receive ONLY the tools listed.
func subAgentToolsArraySchema() map[string]any {
	return map[string]any{
		"type":        "array",
		"description": "Tools the sub-agent may use. A sub-agent gets ONLY what is listed here — e.g. a read-only sub-agent lists just read_file, glob, grep, file_search. An empty list defaults to that read-only set. Each value must be one of the listed runtime or MCP tools.",
		"items": map[string]any{
			"type": "string",
			"enum": toolEnumValues(),
		},
	}
}

func skillsArraySchema(desc string) map[string]any {
	return map[string]any{
		"type":        "array",
		"description": desc,
		"items":       map[string]any{"type": "string"},
	}
}

func subAgentsSchema() map[string]any {
	return map[string]any{
		"type":        "array",
		"description": "Sub-agents the parent can dispatch to via subagent_task.",
		"items": map[string]any{
			"type":                 "object",
			"additionalProperties": false,
			"properties": map[string]any{
				"name":         map[string]any{"type": "string"},
				"description":  map[string]any{"type": "string"},
				"instructions": map[string]any{"type": "string"},
				"skills":       skillsArraySchema("Skill slugs the sub-agent can use."),
				"tools":        subAgentToolsArraySchema(),
			},
			"required": []string{"name"},
		},
	}
}

// modelSchema returns the schema for the optional `model` field. When the model
// catalog is available it is a strict enum so agents cannot hallucinate model
// ids; otherwise a plain string.
func modelSchema(models []string, desc string) map[string]any {
	out := map[string]any{"type": "string", "description": desc}
	if len(models) > 0 {
		enum := make([]any, len(models))
		for i, id := range models {
			enum[i] = id
		}
		out["enum"] = enum
	}
	return out
}

func createAgentSchema(models []string) map[string]any {
	return map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"properties": map[string]any{
			"name":         map[string]any{"type": "string", "description": "The agent's display name."},
			"description":  map[string]any{"type": "string", "description": "Short description of the agent."},
			"instructions": map[string]any{"type": "string", "description": "System instructions for the agent."},
			"model":        modelSchema(models, "Model the agent uses. Omit to use the org default."),
			"skills":       skillsArraySchema("Skill slugs the parent agent can use."),
			"tools":        parentToolsArraySchema(),
			"sub_agents":   subAgentsSchema(),
		},
		"required": []string{"name"},
	}
}

func updateAgentSchema(models []string) map[string]any {
	return map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"properties": map[string]any{
			"agent_id":     map[string]any{"type": "string", "description": "UUID of the agent to update."},
			"name":         map[string]any{"type": "string", "description": "The agent's display name."},
			"description":  map[string]any{"type": "string", "description": "Short description of the agent."},
			"instructions": map[string]any{"type": "string", "description": "System instructions for the agent."},
			"model":        modelSchema(models, "Model the agent uses. Omit to leave unchanged."),
			"status":       map[string]any{"type": "string", "enum": []any{"active", "archived"}, "description": "Agent status."},
			"skills":       skillsArraySchema("Skill slugs the parent agent can use. Replaces the current set."),
			"tools":        parentToolsArraySchema(),
			"sub_agents":   subAgentsSchema(),
		},
		"required": []string{"agent_id"},
	}
}

// checkModelChoice rejects a model id that is not in the assignable catalog,
// returning a helpful IsError result listing the allowed models. An empty model
// is allowed (create defaults to the org default; update leaves it unchanged).
func checkModelChoice(deps Deps, modelID string) *mcp.CallToolResult {
	id := strings.TrimSpace(modelID)
	if id == "" || len(deps.Models) == 0 {
		return nil
	}
	for _, allowed := range deps.Models {
		if allowed == id {
			return nil
		}
	}
	return toolError(fmt.Sprintf("unknown model %q: allowed models are: %s", id, strings.Join(deps.Models, ", ")))
}
