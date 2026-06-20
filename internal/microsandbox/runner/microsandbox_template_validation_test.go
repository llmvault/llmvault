package runner

import (
	"testing"

	"github.com/usehivy/hivy/internal/agentruntime"
)

func TestTemplateValidationEnvUsesPersistentRuntimeDBPath(t *testing.T) {
	env := templateValidationEnv(CreateTemplateRequest{}, "validation-sbx")
	if got := env[agentruntime.AgentEnvDBPath]; got != agentruntime.AgentRuntimeDBPath {
		t.Fatalf("runtime db path = %q, want %q", got, agentruntime.AgentRuntimeDBPath)
	}
}
