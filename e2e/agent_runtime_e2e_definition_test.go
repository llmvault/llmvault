package e2e

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type agentRuntimeE2EAgentManifest struct {
	Name        string                                     `json:"name"`
	Description string                                     `json:"description"`
	SubAgents   map[string]agentRuntimeE2ESubagentManifest `json:"sub_agents"`
}

type agentRuntimeE2ESubagentManifest struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Model       string `json:"model"`
}

func agentRuntimeE2EDefinition(t *testing.T, trace *agentRuntimeE2ETrace, fixtureURL, proxyURL, controlPlaneURL, sandboxID string) map[string]any {
	t.Helper()
	hakaree := loadAgentRuntimeE2EHakareeManifest(t)
	modelID := strings.TrimSpace(os.Getenv("HIVY_AGENT_RUNTIME_E2E_MODEL"))
	if modelID == "" {
		modelID = "deepseek/deepseek-v4-flash"
	}
	modelProfile := "deepseek"
	if strings.Contains(modelID, "glm") || strings.Contains(modelID, "z-ai") {
		modelProfile = "glm"
	} else if strings.Contains(modelID, "kimi") || strings.Contains(modelID, "moonshot") {
		modelProfile = "kimi"
	} else if strings.Contains(modelID, "minimax") {
		modelProfile = "minimax"
	} else if strings.Contains(modelID, "mimo") {
		modelProfile = "mimo"
	} else if strings.Contains(modelID, "qwen") {
		modelProfile = "qwen"
	}
	var canonicalModelID string
	if idx := strings.LastIndex(modelID, "/"); idx >= 0 && idx+1 < len(modelID) {
		canonicalModelID = modelID[idx+1:]
	} else {
		canonicalModelID = modelID
	}
	trace.Logf("definition", "model_id=%s fixture_url=%s proxy_url=%s control_plane_url=%s", modelID, fixtureURL, proxyURL, controlPlaneURL)
	model := map[string]any{
		"provider":           "openai_compatible",
		"base_url":           strings.TrimRight(proxyURL, "/") + "/v1",
		"model_id":           modelID,
		"canonical_model_id": canonicalModelID,
		"provider_id":        "openrouter",
		"upstream_model_id":  modelID,
		"model_profile":      modelProfile,
		"capabilities": map[string]any{
			"reasoning":         true,
			"tool_call":         true,
			"structured_output": true,
		},
		"api_key_env":       "HIVY_PROXY_API_KEY",
		"temperature":       0,
		"max_output_tokens": 4096,
		"extra_headers": map[string]string{
			"HTTP-Referer": "https://usehivy.com",
			"X-Title":      "Hivy",
		},
	}
	definition := map[string]any{
		"agent": map[string]any{
			"name":        hakaree.Name,
			"description": hakaree.Description + " Flagship runtime E2E test mode.",
		},
		"system_prompt": map[string]any{
			"cacheable_segments": []any{
				map[string]any{
					"type": "static_text",
					"config": map[string]any{
						"title": "Hakaree Runtime E2E Contract",
						"content": strings.Join([]string{
							"You are Hakaree running inside a temporary Hivy runtime workspace for the flagship runtime E2E.",
							"This is a test session. Every agent and subagent must minimize token use, avoid broad real work, avoid external research unless explicitly requested by a numbered step, and stop as soon as the requested markers are produced.",
							"Tool coverage is part of this task. Use every tool named by the user.",
							"Do not invent the token or requirement phrases. Retrieve them from fixture_requirements.",
							"Use exact, minimal file edits. If a tool returns an id for a required follow-up, use that exact id.",
							"Subagent orchestration coverage is part of this task. Start codebase-explorer, librarian, and oracle in the same tool-call batch, then use each subagent_task tool result directly.",
							"Do not poll subagent status. The subagent_task tool returns the completed subagent result.",
						}, "\n"),
					},
				},
			},
			"dynamic_segments": []any{
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
		"model": model,
		"limits": map[string]any{
			"max_turns_per_session":     120,
			"input_token_budget":        100000,
			"output_token_budget":       12000,
			"tool_call_timeout_seconds": 120,
		},
		"context": map[string]any{
			"max_history_events": 20,
		},
		"tools":             agentRuntimeE2ETools(),
		"mcp_servers":       []any{map[string]any{"transport": "streamable_http", "name": "fixture", "url": fixtureURL, "headers": map[string]string{}}},
		"skills":            []any{},
		"outbound_channels": []any{map[string]any{"name": "control-plane", "type": "webhook", "url": controlPlaneURL + "/internal/webhooks/agent/" + sandboxID, "secret_env": "HIVY_RUNTIME_SECRET"}},
		"sub_agents":        agentRuntimeE2ESubagents(t, hakaree, model),
		"safety":            map[string]any{},
	}
	trace.Logf("definition", "agent=%s tools=%d sub_agents=%d outbound_channels=%d", hakaree.Name, len(agentRuntimeE2ETools()), len(agentRuntimeE2EHakareeSubagents), 1)
	return definition
}

func loadAgentRuntimeE2EHakareeManifest(t *testing.T) agentRuntimeE2EAgentManifest {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(repoRootFromE2E(t), "global", "agents", "hakaree", "agent.json"))
	if err != nil {
		t.Fatalf("read hakaree agent manifest: %v", err)
	}
	var manifest agentRuntimeE2EAgentManifest
	if err := json.Unmarshal(raw, &manifest); err != nil {
		t.Fatalf("parse hakaree agent manifest: %v", err)
	}
	if strings.TrimSpace(manifest.Name) == "" {
		t.Fatal("hakaree manifest has empty name")
	}
	for _, agent := range agentRuntimeE2EHakareeSubagents {
		if _, ok := manifest.SubAgents[agent]; !ok {
			t.Fatalf("hakaree manifest missing %q subagent", agent)
		}
	}
	return manifest
}

func agentRuntimeE2ETools() []any {
	writeConfig := map[string]any{"allowed_roots": []string{}, "max_file_size_bytes": 1048576, "deny_globs": []string{}, "atomic": true}
	searchConfig := map[string]any{"allowed_roots": []string{}, "deny_globs": []string{}, "max_results": 120, "max_output_bytes": 262144, "timeout_seconds": 20, "include_hidden": false, "respect_gitignore": true, "follow_symlinks": false, "enable_content_indexing": true}
	return []any{
		map[string]any{"type": "builtin.bash", "config": map[string]any{"workdir": ".", "timeout_seconds": 90, "max_output_bytes": 1048576, "deny_patterns": []string{"rm -rf /", "mkfs", "shutdown", "reboot"}, "env_passthrough": []string{"PATH", "HOME"}, "sandbox": "process_isolated"}},
		map[string]any{"type": "builtin.read_file", "config": map[string]any{"allowed_roots": []string{}, "max_file_size_bytes": 1048576, "deny_globs": []string{}}},
		map[string]any{"type": "builtin.write_file", "config": writeConfig},
		map[string]any{"type": "builtin.file_search", "config": searchConfig},
		map[string]any{"type": "builtin.glob", "config": searchConfig},
		map[string]any{"type": "builtin.grep", "config": searchConfig},
		map[string]any{"type": "builtin.multi_grep", "config": searchConfig},
		map[string]any{"type": "builtin.apply_patch", "config": map[string]any{"allowed_roots": []string{}, "max_file_size_bytes": 1048576, "deny_globs": []string{}, "atomic": true}},
		map[string]any{"type": "builtin.lsp", "config": map[string]any{"enabled": true, "allowed_roots": []string{}, "timeout_seconds": 20}},
		map[string]any{"type": "builtin.subagent_task", "config": map[string]any{"agents": agentRuntimeE2EHakareeSubagents}},
		map[string]any{"type": "builtin.check_bash_status"},
		map[string]any{"type": "builtin.search_sessions"},
	}
}

func agentRuntimeE2ESubagents(t *testing.T, manifest agentRuntimeE2EAgentManifest, model map[string]any) map[string]any {
	t.Helper()
	out := map[string]any{}
	for _, key := range agentRuntimeE2EHakareeSubagents {
		subagent := manifest.SubAgents[key]
		out[key] = agentRuntimeE2ESubagent(model, key, subagent.Name, subagent.Description)
	}
	return out
}

func agentRuntimeE2ESubagent(model map[string]any, key, name, description string) map[string]any {
	if strings.TrimSpace(name) == "" {
		name = key
	}
	if strings.TrimSpace(description) == "" {
		description = "Hakaree subagent participating in the flagship runtime E2E."
	}
	prompt := strings.Join([]string{
		"You are Hakaree subagent `" + key + "` running in FLAGSHIP_E2E_TEST_SESSION mode.",
		"This is not real user work. Keep token use minimal, do not explore beyond the parent goal, and do not perform external or destructive actions.",
		"When invoked, perform exactly two or three cheap local tool calls requested by the parent goal, then stop.",
		"Your final response must be exactly: " + agentRuntimeE2ESubagentMarkers[key],
	}, "\n")
	return map[string]any{
		"agent":         map[string]any{"name": name, "description": description + " E2E test mode."},
		"system_prompt": map[string]any{"cacheable_segments": []any{map[string]any{"type": "static_text", "config": map[string]any{"title": "Contract", "content": prompt}}}, "dynamic_segments": []any{}},
		"model":         model,
		"limits":        map[string]any{"max_turns_per_session": 8, "input_token_budget": 12000, "output_token_budget": 500, "tool_call_timeout_seconds": 30},
		"context":       map[string]any{},
		"tools":         agentRuntimeE2ESubagentTools(), "mcp_servers": []any{}, "skills": []any{}, "outbound_channels": []any{}, "sub_agents": map[string]any{}, "safety": map[string]any{},
	}
}

func agentRuntimeE2ESubagentTools() []any {
	searchConfig := map[string]any{"allowed_roots": []string{}, "deny_globs": []string{}, "max_results": 20, "max_output_bytes": 32768, "timeout_seconds": 10, "include_hidden": false, "respect_gitignore": true, "follow_symlinks": false, "enable_content_indexing": true}
	return []any{
		map[string]any{"type": "builtin.read_file", "config": map[string]any{"allowed_roots": []string{}, "max_file_size_bytes": 262144, "deny_globs": []string{}}},
		map[string]any{"type": "builtin.file_search", "config": searchConfig},
		map[string]any{"type": "builtin.glob", "config": searchConfig},
		map[string]any{"type": "builtin.grep", "config": searchConfig},
		map[string]any{"type": "builtin.multi_grep", "config": searchConfig},
		map[string]any{"type": "builtin.lsp", "config": map[string]any{"enabled": true, "allowed_roots": []string{}, "timeout_seconds": 10}},
	}
}
