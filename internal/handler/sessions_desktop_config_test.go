package handler

import (
	"testing"

	"github.com/usehivy/hivy/internal/agentruntime"
)

func TestNormalizeDesktopRuntimeConfigRewritesDockerHostURLs(t *testing.T) {
	update := agentruntime.ConfigUpdateRequest{
		RuntimeEnv: map[string]string{
			"HIVY_CONTROL_PLANE_URL": "http://host.docker.internal:8080",
			"HIVY_AGENT_MODEL":       "host.docker.internal-model",
		},
		Definition: &agentruntime.AgentDefinition{
			Model: agentruntime.ModelConfig{BaseURL: "http://host.docker.internal:18082/v1"},
			McpServers: []any{map[string]any{
				"url": "http://host.docker.internal:8081/token",
			}},
			OutboundChannels: []any{map[string]any{
				"url": "ws://host.docker.internal:8080/sessions/{session_id}/events",
			}},
		},
	}

	normalizeDesktopRuntimeConfig(&update)

	if got := update.RuntimeEnv["HIVY_RUNTIME_MODE"]; got != "desktop" {
		t.Fatalf("runtime mode = %q", got)
	}
	if got := update.RuntimeEnv["HIVY_CONTROL_PLANE_URL"]; got != "http://127.0.0.1:8080" {
		t.Fatalf("control plane URL = %q", got)
	}
	if got := update.RuntimeEnv["HIVY_AGENT_MODEL"]; got != "host.docker.internal-model" {
		t.Fatalf("non-URL value changed to %q", got)
	}
	if got := update.Definition.Model.BaseURL; got != "http://127.0.0.1:18082/v1" {
		t.Fatalf("model URL = %q", got)
	}
	server := update.Definition.McpServers[0].(map[string]any)
	if got := server["url"]; got != "http://127.0.0.1:8081/token" {
		t.Fatalf("MCP URL = %q", got)
	}
	channel := update.Definition.OutboundChannels[0].(map[string]any)
	if got := channel["url"]; got != "ws://127.0.0.1:8080/sessions/{session_id}/events" {
		t.Fatalf("outbound URL = %q", got)
	}
}
