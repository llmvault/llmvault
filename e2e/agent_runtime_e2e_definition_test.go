package e2e

import (
	"os"
	"strings"
	"testing"
)

func agentRuntimeE2EDefinition(t *testing.T, trace *agentRuntimeE2ETrace, fixtureURL, proxyURL, controlPlaneURL, sandboxID string) map[string]any {
	t.Helper()
	modelID := strings.TrimSpace(os.Getenv("HIVY_AGENT_RUNTIME_E2E_MODEL"))
	if modelID == "" {
		modelID = "openai/gpt-4o-mini"
	}
	trace.Logf("definition", "model_id=%s fixture_url=%s proxy_url=%s control_plane_url=%s", modelID, fixtureURL, proxyURL, controlPlaneURL)
	model := map[string]any{
		"provider":          "openai_compatible",
		"base_url":          strings.TrimRight(proxyURL, "/") + "/v1",
		"model_id":          modelID,
		"api_key_env":       "HIVY_PROXY_API_KEY",
		"temperature":       0,
		"max_output_tokens": 4096,
		"reasoning_effort":  "low",
		"extra_headers": map[string]string{
			"HTTP-Referer": "https://usehivy.com",
			"X-Title":      "Hivy agent runtime E2E",
		},
	}
	definition := map[string]any{
		"agent": map[string]any{
			"name":        "Runtime E2E Developer",
			"description": "Executes the flagship backend runtime coding E2E.",
		},
		"system_prompt": map[string]any{
			"cacheable_segments": []any{
				map[string]any{
					"type": "static_text",
					"config": map[string]any{
						"title": "Runtime E2E Contract",
						"content": strings.Join([]string{
							"You are an autonomous coding agent inside a temporary Hivy runtime workspace.",
							"Tool coverage is part of this task. Use every tool named by the user.",
							"Do not invent the token or requirement phrases. Retrieve them from fixture_requirements.",
							"Use exact, minimal file edits. If a tool returns an id, use that id in the matching status tool.",
							"After starting each configured subagent, call check_subagent_task_status once for that job id.",
						}, "\n"),
					},
				},
			},
			"dynamic_segments": []any{
				map[string]any{
					"type": "dynamic_context",
					"config": map[string]any{
						"title":         "Runtime Context",
						"preamble":      "",
						"item_template": "{content}",
					},
				},
				map[string]any{
					"type": "mcp_tools",
					"config": map[string]any{
						"title":         "Configured project tools",
						"preamble":      "Use fixture_requirements to retrieve the E2E token and required phrases.",
						"item_template": "- {name}",
					},
				},
			},
		},
		"model":            model,
		"multimodal_model": nil,
		"limits": map[string]any{
			"max_turns_per_session":     120,
			"input_token_budget":        100000,
			"output_token_budget":       12000,
			"tool_call_timeout_seconds": 120,
		},
		"context": map[string]any{
			"max_history_events": 20,
			"memory":             map[string]any{"entries": []any{}, "token_budget": 2000},
		},
		"tools":             agentRuntimeE2ETools(),
		"mcp_servers":       []any{map[string]any{"transport": "streamable_http", "name": "fixture", "url": fixtureURL, "headers": map[string]string{}}},
		"skills":            []any{},
		"outbound_channels": []any{map[string]any{"name": "control-plane", "type": "webhook", "url": controlPlaneURL + "/internal/webhooks/agent/" + sandboxID, "secret_env": "HIVY_RUNTIME_SECRET"}},
		"sub_agents":        agentRuntimeE2ESubagents(model),
		"safety":            map[string]any{},
	}
	trace.Logf("definition", "tools=%d sub_agents=%d outbound_channels=%d", len(agentRuntimeE2ETools()), len(agentRuntimeE2ESubagents(model)), 1)
	return definition
}

func agentRuntimeE2ETools() []any {
	writeConfig := map[string]any{"allowed_roots": []string{}, "max_file_size_bytes": 1048576, "deny_globs": []string{}, "atomic": true}
	return []any{
		map[string]any{"type": "builtin.bash", "config": map[string]any{"workdir": ".", "timeout_seconds": 90, "max_output_bytes": 1048576, "deny_patterns": []string{"rm -rf /", "mkfs", "shutdown", "reboot"}, "env_passthrough": []string{"PATH", "HOME"}, "sandbox": "process_isolated"}},
		map[string]any{"type": "builtin.read_file", "config": map[string]any{"allowed_roots": []string{}, "max_file_size_bytes": 1048576, "deny_globs": []string{}}},
		map[string]any{"type": "builtin.write_file", "config": writeConfig},
		map[string]any{"type": "builtin.subagent_task", "config": map[string]any{"agents": []string{"planner", "qa", "reviewer"}}},
		map[string]any{"type": "builtin.check_subagent_task_status"},
		map[string]any{"type": "builtin.check_bash_status"},
		map[string]any{"type": "builtin.search_sessions"},
	}
}

func agentRuntimeE2ESubagents(model map[string]any) map[string]any {
	return map[string]any{
		"planner":  agentRuntimeE2ESubagent(model, "Planner", "Reply exactly with PLANNER_SUBAGENT_CONFIRMED and no extra analysis."),
		"qa":       agentRuntimeE2ESubagent(model, "QA", "Reply exactly with QA_SUBAGENT_CONFIRMED and no extra analysis."),
		"reviewer": agentRuntimeE2ESubagent(model, "Reviewer", "Reply exactly with REVIEW_SUBAGENT_CONFIRMED and no extra analysis."),
	}
}

func agentRuntimeE2ESubagent(model map[string]any, name, prompt string) map[string]any {
	return map[string]any{
		"agent":         map[string]any{"name": name, "description": "Verification subagent for the runtime E2E."},
		"system_prompt": map[string]any{"cacheable_segments": []any{map[string]any{"type": "static_text", "config": map[string]any{"title": "Contract", "content": prompt}}}, "dynamic_segments": []any{}},
		"model":         model, "multimodal_model": nil,
		"limits":  map[string]any{"max_turns_per_session": 5, "input_token_budget": 20000, "output_token_budget": 1000, "tool_call_timeout_seconds": 30},
		"context": map[string]any{"memory": map[string]any{"entries": []any{}, "token_budget": 1000}},
		"tools":   []any{}, "mcp_servers": []any{}, "skills": []any{}, "outbound_channels": []any{}, "sub_agents": map[string]any{}, "safety": map[string]any{},
	}
}
