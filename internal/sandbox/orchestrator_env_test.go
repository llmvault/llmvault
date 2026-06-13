package sandbox

import (
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/usehivy/hivy/internal/agentruntime"
	"github.com/usehivy/hivy/internal/config"
	"github.com/usehivy/hivy/internal/model"
)

func TestEmployeeSandboxEnvVarsUseAPIWebhookBaseURL(t *testing.T) {
	cfg := &config.Config{
		APIWebhookBaseURL: "http://host.docker.internal:8080",
		ProxyHost:         "http://host.docker.internal:8080",
	}
	agent := &model.Employee{ID: uuid.New()}
	sb := &model.Sandbox{ID: uuid.New()}
	env := employeeSandboxEnvVars(cfg, "runtime-secret", sb, uuid.New(), agent, &agentruntime.StartupSecrets{ProxyToken: "proxy-token"}, nil, "", "")

	if got := env[agentruntime.EmployeeEnvCloudControlPlaneURL]; got != "http://host.docker.internal:8080" {
		t.Fatalf("control plane url = %q", got)
	}
	if got := env[agentruntime.EmployeeEnvAgentBaseURL]; got != "http://host.docker.internal:8080/v1" {
		t.Fatalf("agent base url = %q", got)
	}
	for _, key := range []string{
		agentruntime.EmployeeEnvGitCredentialsURL,
		agentruntime.EmployeeEnvDriveUploadURL,
		agentruntime.EmployeeEnvBugsinkURL,
		agentruntime.EmployeeEnvLinearURL,
		agentruntime.EmployeeEnvNotionAPIURL,
	} {
		if got := env[key]; !strings.HasPrefix(got, "http://host.docker.internal:8080/") {
			t.Fatalf("%s = %q", key, got)
		}
	}
}

func TestEmployeeSandboxEnvVarsUseAgentSandboxSentryDSN(t *testing.T) {
	cfg := &config.Config{
		APIWebhookBaseURL:      "https://api.example",
		ProxyHost:              "proxy.example",
		Environment:            "production",
		SentryTracesSampleRate: 0.25,
		AgentSandboxSentryDSN:  "https://agent@example.com/2",
	}
	agent := &model.Employee{ID: uuid.New()}
	sb := &model.Sandbox{ID: uuid.New()}

	env := employeeSandboxEnvVars(cfg, "runtime-secret", sb, uuid.New(), agent, &agentruntime.StartupSecrets{ProxyToken: "proxy-token"}, nil, "", "")

	if got := env[agentruntime.EmployeeEnvSentryDSN]; got != cfg.AgentSandboxSentryDSN {
		t.Fatalf("sentry dsn = %q, want agent sandbox dsn", got)
	}
	if got := env[agentruntime.EmployeeEnvSentryEnvironment]; got != "production" {
		t.Fatalf("sentry environment = %q", got)
	}
	if got := env[agentruntime.EmployeeEnvSentryEnableLogs]; got != "true" {
		t.Fatalf("sentry enable logs = %q", got)
	}
}
