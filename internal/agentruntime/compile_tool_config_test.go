package agentruntime

import (
	"context"
	"testing"

	"github.com/google/uuid"

	"github.com/usehivy/hivy/internal/config"
	"github.com/usehivy/hivy/internal/model"
)

func TestCompileBashToolPassesCanvasRuntimeEnv(t *testing.T) {
	orgID := uuid.New()
	agent := &model.Agent{
		ID:    uuid.New(),
		OrgID: &orgID,
		Name:  "Canvas Agent",
		Model: DefaultAgentModel,
		Tools: model.JSON{"bash": true},
	}

	def, err := Compile(context.Background(), CompileDeps{Cfg: &config.Config{}}, agent)
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	if len(def.Tools) != 1 {
		t.Fatalf("runtime tools = %#v, want one bash tool", def.Tools)
	}
	config, ok := def.Tools[0]["config"].(map[string]any)
	if !ok {
		t.Fatalf("bash config = %#v", def.Tools[0]["config"])
	}
	passthrough, ok := config["env_passthrough"].([]any)
	if !ok {
		t.Fatalf("bash env passthrough = %#v", config["env_passthrough"])
	}
	for _, key := range []string{
		AgentEnvPenpotCanvasURL,
		AgentEnvPenpotCanvasTeamID,
		AgentEnvPenpotCanvasProfileID,
		AgentEnvPenpotCanvasSessionJWT,
		AgentEnvPenpotCanvasMCPURL,
		AgentEnvCloudControlPlaneURL,
		AgentEnvAgentID,
		AgentEnvRuntimeSecret,
	} {
		if !containsAnyString(passthrough, key) {
			t.Fatalf("bash env passthrough missing %s: %#v", key, passthrough)
		}
	}
}

func TestCompileBashToolAddsCanvasEnvToCustomPassthrough(t *testing.T) {
	orgID := uuid.New()
	agent := &model.Agent{
		ID:    uuid.New(),
		OrgID: &orgID,
		Name:  "Canvas Agent",
		Model: DefaultAgentModel,
		Tools: model.JSON{
			"bash": map[string]any{"env_passthrough": []any{"PATH", "HOME"}},
		},
	}

	def, err := Compile(context.Background(), CompileDeps{Cfg: &config.Config{}}, agent)
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	config := def.Tools[0]["config"].(map[string]any)
	passthrough := config["env_passthrough"].([]any)
	if !containsAnyString(passthrough, AgentEnvPenpotCanvasSessionJWT) {
		t.Fatalf("custom bash env passthrough missing canvas env: %#v", passthrough)
	}
	if !containsAnyString(passthrough, AgentEnvRuntimeSecret) {
		t.Fatalf("custom bash env passthrough missing runtime secret: %#v", passthrough)
	}
}

func containsAnyString(values []any, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}
