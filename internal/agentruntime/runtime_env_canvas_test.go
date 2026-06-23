package agentruntime

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/usehivy/hivy/internal/config"
	"github.com/usehivy/hivy/internal/model"
)

type fakeCanvasRuntimeEnvProvider struct{}

func (fakeCanvasRuntimeEnvProvider) AgentRuntimeEnv(context.Context, *model.Agent) (map[string]string, error) {
	return map[string]string{
		AgentEnvPenpotCanvasURL:        "https://canvas.usehivy.com",
		AgentEnvPenpotCanvasTeamID:     "team-id",
		AgentEnvPenpotCanvasProfileID:  "profile-id",
		AgentEnvPenpotCanvasSessionJWT: "jwt",
		AgentEnvPenpotCanvasMCPURL:     "https://canvas.usehivy.com/mcp/stream?userToken=token",
	}, nil
}

type failingCanvasRuntimeEnvProvider struct{}

func (failingCanvasRuntimeEnvProvider) AgentRuntimeEnv(context.Context, *model.Agent) (map[string]string, error) {
	return nil, errors.New("canvas duplicate team")
}

func TestBuildRuntimeEnvWithProxyTokenIncludesCanvasEnv(t *testing.T) {
	orgID := uuid.New()
	agent := &model.Agent{ID: uuid.New(), OrgID: &orgID, Model: DefaultAgentModel}
	token := &ProxyTokenResult{Token: "ptok_test", JTI: "jti"}
	env, err := BuildRuntimeEnvWithProxyToken(context.Background(), CompileDeps{
		Cfg:    &config.Config{},
		Canvas: fakeCanvasRuntimeEnvProvider{},
	}, agent, nil, "runtime-secret", token)
	if err != nil {
		t.Fatalf("BuildRuntimeEnvWithProxyToken: %v", err)
	}
	if env[AgentEnvPenpotCanvasURL] != "https://canvas.usehivy.com" {
		t.Fatalf("canvas url env = %q", env[AgentEnvPenpotCanvasURL])
	}
	if env[AgentEnvPenpotCanvasSessionJWT] != "jwt" {
		t.Fatalf("canvas session jwt env = %q", env[AgentEnvPenpotCanvasSessionJWT])
	}
}

func TestBuildRuntimeEnvWithProxyTokenReturnsCanvasProviderError(t *testing.T) {
	orgID := uuid.New()
	agent := &model.Agent{ID: uuid.New(), OrgID: &orgID, Model: DefaultAgentModel}
	token := &ProxyTokenResult{Token: "ptok_test", JTI: "jti"}
	env, err := BuildRuntimeEnvWithProxyToken(context.Background(), CompileDeps{
		Cfg:    &config.Config{},
		Canvas: failingCanvasRuntimeEnvProvider{},
	}, agent, nil, "runtime-secret", token)
	if err == nil {
		t.Fatalf("BuildRuntimeEnvWithProxyToken unexpectedly succeeded with env %#v", env)
	}
	if !strings.Contains(err.Error(), "canvas duplicate team") {
		t.Fatalf("BuildRuntimeEnvWithProxyToken error = %v", err)
	}
}
