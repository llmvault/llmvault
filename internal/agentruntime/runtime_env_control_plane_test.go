package agentruntime

import (
	"testing"

	"github.com/google/uuid"

	"github.com/usehivy/hivy/internal/config"
	"github.com/usehivy/hivy/internal/model"
)

func TestApplyControlPlaneRuntimeEnvInjectsConfiguredMCPHost(t *testing.T) {
	t.Parallel()

	env := map[string]string{AgentEnvTrustedPrivateMCPHosts: "attacker.example"}
	cfg := &config.Config{
		APIWebhookBaseURL: "https://api.usehivy.test",
		MCPBaseURL:        "https://Staging.MCP.usehivy.test:8443/path",
	}
	agent := &model.Agent{ID: uuid.New()}

	ApplyControlPlaneRuntimeEnv(env, cfg, agent, "runtime-secret", ControlPlaneRuntimeEnvOptions{})

	if got, want := env[AgentEnvTrustedPrivateMCPHosts], "staging.mcp.usehivy.test"; got != want {
		t.Fatalf("trusted MCP hosts = %q, want %q", got, want)
	}
}
