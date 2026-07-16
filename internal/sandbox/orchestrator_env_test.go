package sandbox

import (
	"context"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/usehivy/hivy/internal/agentruntime"
	"github.com/usehivy/hivy/internal/config"
	"github.com/usehivy/hivy/internal/model"
)

func TestAgentSandboxEnvVarsUseAPIWebhookBaseURL(t *testing.T) {
	cfg := &config.Config{
		APIWebhookBaseURL: "http://host.docker.internal:8080",
		ProxyHost:         "http://host.docker.internal:8080",
	}
	agent := &model.Agent{ID: uuid.New()}
	sb := &model.Sandbox{ID: uuid.New()}
	env := agentSandboxEnvVars(context.Background(), nil, cfg, "runtime-secret", sb, uuid.New(), agent, &agentruntime.StartupSecrets{ProxyToken: "proxy-token"}, nil, "", "")

	if got := env[agentruntime.AgentEnvCloudControlPlaneURL]; got != "http://host.docker.internal:8080" {
		t.Fatalf("control plane url = %q", got)
	}
	if got := env[agentruntime.AgentEnvAgentBaseURL]; got != "http://host.docker.internal:8080/v1" {
		t.Fatalf("agent base url = %q", got)
	}
	if got := env[agentruntime.AgentEnvDBPath]; got != agentruntime.AgentRuntimeDBPath {
		t.Fatalf("runtime db path = %q, want %q", got, agentruntime.AgentRuntimeDBPath)
	}
	for _, key := range []string{
		agentruntime.AgentEnvGitCredentialsURL,
		agentruntime.AgentEnvDriveUploadURL,
	} {
		if got := env[key]; !strings.HasPrefix(got, "http://host.docker.internal:8080/") {
			t.Fatalf("%s = %q", key, got)
		}
	}
}

func TestAgentSandboxEnvVarsUseAgentSandboxSentryDSN(t *testing.T) {
	cfg := &config.Config{
		APIWebhookBaseURL:      "https://api.example",
		ProxyHost:              "proxy.example",
		Environment:            "production",
		SentryTracesSampleRate: 0.25,
		AgentSandboxSentryDSN:  "https://agent@example.com/2",
	}
	agent := &model.Agent{ID: uuid.New()}
	sb := &model.Sandbox{ID: uuid.New()}

	env := agentSandboxEnvVars(context.Background(), nil, cfg, "runtime-secret", sb, uuid.New(), agent, &agentruntime.StartupSecrets{ProxyToken: "proxy-token"}, nil, "", "")

	if got := env[agentruntime.AgentEnvSentryDSN]; got != cfg.AgentSandboxSentryDSN {
		t.Fatalf("sentry dsn = %q, want agent sandbox dsn", got)
	}
	if got := env[agentruntime.AgentEnvSentryEnvironment]; got != "production" {
		t.Fatalf("sentry environment = %q", got)
	}
	if got := env[agentruntime.AgentEnvSentryEnableLogs]; got != "true" {
		t.Fatalf("sentry enable logs = %q", got)
	}
}
