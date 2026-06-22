package agentruntime

import (
	"encoding/json"
	"strings"
	"testing"
)

// the debug config payload must never leak the runtime secret, sensitive
// env values, or MCP Authorization headers when logged.
func TestRedactConfigUpdateRequestStripsSecrets(t *testing.T) {
	body := ConfigUpdateRequest{
		RuntimeSecret: "super-secret-runtime-value",
		RuntimeEnv: map[string]string{
			AgentEnvProxyAPIKey:      "proxy-token-abc",   // catalog Sensitive
			AgentEnvSlackToken:       "xoxb-real-token",   // catalog Sensitive
			"CUSTOM_API_SECRET":      "leak-me",           // heuristic Sensitive
			"DATABASE_PASSWORD":      "hunter2",           // heuristic Sensitive
			AgentEnvAgentBaseURL:     "https://llm.local", // not sensitive
			"FEATURE_FLAG_SOMETHING": "on",                // not sensitive
		},
		Definition: &AgentDefinition{
			McpServers: []any{
				map[string]any{
					"name": "hivy",
					"url":  "https://mcp.example/abc",
					"headers": map[string]any{
						"Authorization": "Bearer real-jwt-here",
						"X-Trace":       "keep-this",
					},
				},
			},
		},
		Workspace: &WorkspaceConfig{
			Repos: []WorkspaceRepoConfig{
				{
					ID:       "usehivy/hivy",
					Name:     "hivy",
					FullName: "usehivy/hivy",
					CloneURL: "https://token@example.com/usehivy/hivy.git",
					Depth:    1,
				},
			},
		},
	}

	out := redactConfigUpdateRequest(body)
	raw, err := json.Marshal(out)
	if err != nil {
		t.Fatalf("marshal redacted: %v", err)
	}
	dump := string(raw)

	for _, secret := range []string{
		"super-secret-runtime-value",
		"proxy-token-abc",
		"xoxb-real-token",
		"leak-me",
		"hunter2",
		"Bearer real-jwt-here",
		"https://token@example.com/usehivy/hivy.git",
	} {
		if strings.Contains(dump, secret) {
			t.Fatalf("redacted payload still contains secret %q: %s", secret, dump)
		}
	}

	for _, keep := range []string{"https://llm.local", "on", "keep-this", "https://mcp.example/abc"} {
		if !strings.Contains(dump, keep) {
			t.Fatalf("redacted payload dropped non-sensitive value %q: %s", keep, dump)
		}
	}
	for _, keep := range []string{"usehivy/hivy", "hivy"} {
		if !strings.Contains(dump, keep) {
			t.Fatalf("redacted payload dropped safe repo value %q: %s", keep, dump)
		}
	}

	if body.RuntimeSecret != "super-secret-runtime-value" {
		t.Fatal("redaction mutated the original RuntimeSecret")
	}
	if body.RuntimeEnv[AgentEnvSlackToken] != "xoxb-real-token" {
		t.Fatal("redaction mutated the original RuntimeEnv")
	}
	servers, _ := body.Definition.McpServers[0].(map[string]any)
	headers, _ := servers["headers"].(map[string]any)
	if headers["Authorization"] != "Bearer real-jwt-here" {
		t.Fatal("redaction mutated the original MCP Authorization header")
	}
}
