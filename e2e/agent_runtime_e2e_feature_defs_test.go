package e2e

import (
	"os"
	"strings"
	"testing"
)

func agentRuntimeE2EModel(t *testing.T, proxyURL string) map[string]any {
	t.Helper()
	modelID := strings.TrimSpace(os.Getenv("HIVY_AGENT_RUNTIME_E2E_MODEL"))
	if modelID == "" {
		modelID = "openai/gpt-4o-mini"
	}
	return map[string]any{
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
}

func agentRuntimeFeatureDefinition(t *testing.T, proxyURL, controlPlaneURL, sandboxID, name, prompt string, tools, mcpServers, skills []any) map[string]any {
	t.Helper()
	model := agentRuntimeE2EModel(t, proxyURL)
	return map[string]any{
		"agent": map[string]any{
			"name":        name,
			"description": "Executes a focused backend runtime E2E.",
		},
		"system_prompt": map[string]any{
			"cacheable_segments": []any{map[string]any{"type": "static_text", "config": map[string]any{
				"title":   "Runtime E2E Contract",
				"content": prompt,
			}}},
			"dynamic_segments": []any{
				map[string]any{"type": "dynamic_context", "config": map[string]any{"title": "Runtime Context", "preamble": "", "item_template": "{content}"}},
				map[string]any{"type": "mcp_tools", "config": map[string]any{"title": "Configured project tools", "preamble": "Use configured MCP tools when the user names them.", "item_template": "- {name}"}},
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
		"context": map[string]any{"max_history_events": 30},
		"tools":   tools, "mcp_servers": mcpServers, "skills": skills,
		"outbound_channels": []any{map[string]any{"name": "control-plane", "type": "webhook", "url": controlPlaneURL + "/internal/webhooks/agent/" + sandboxID, "secret_env": "HIVY_RUNTIME_SECRET"}},
		"sub_agents":        map[string]any{},
		"safety":            map[string]any{},
	}
}

func agentRuntimeFileTools() []any {
	writeConfig := map[string]any{"allowed_roots": []string{}, "max_file_size_bytes": 1048576, "deny_globs": []string{}, "atomic": true}
	return []any{
		map[string]any{"type": "builtin.bash", "config": map[string]any{"workdir": ".", "timeout_seconds": 90, "max_output_bytes": 1048576, "deny_patterns": []string{"rm -rf /", "mkfs", "shutdown", "reboot"}, "env_passthrough": []string{"PATH", "HOME"}, "sandbox": "process_isolated"}},
		map[string]any{"type": "builtin.read_file", "config": map[string]any{"allowed_roots": []string{}, "max_file_size_bytes": 1048576, "deny_globs": []string{}}},
		map[string]any{"type": "builtin.write_file", "config": writeConfig},
		map[string]any{"type": "builtin.check_bash_status"},
		map[string]any{"type": "builtin.search_sessions"},
	}
}

func agentRuntimeSkillsTools() []any {
	return append(agentRuntimeFileTools(),
		map[string]any{"type": "builtin.skills_list"},
		map[string]any{"type": "builtin.skill_view"},
		map[string]any{"type": "builtin.skill_manage"},
	)
}
