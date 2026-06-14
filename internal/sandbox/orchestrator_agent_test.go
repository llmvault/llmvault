package sandbox

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/usehivy/hivy/internal/agentruntime"
	"github.com/usehivy/hivy/internal/model"
)

func TestCreateAgentSandbox_ClonesSelectedGitHubProfileRepositories(t *testing.T) {
	orch, provider, db := setupOrchestrator(t)
	provider.endpointOverride = agentRuntimeEndpoint(t)
	orch.cfg.SandboxesRuntimeBaseImage = "ghcr.io/usehivy/hivy-sandboxes-runtime:test-v1"

	org := createTestOrg(t, db)
	cred := createTestCred(t, db, org.ID)
	agent := createTestAgent(t, db, org.ID, cred.ID)
	agent.IsManaged = true
	agent.Resources = model.JSON{
		"github": map[string]any{
			"repository": []any{
				map[string]any{"id": "123456", "name": "api", "full_name": "octo-org/api"},
				map[string]any{"id": "789012", "name": "web", "full_name": "octo-org/web"},
			},
		},
	}
	if err := db.Save(&agent).Error; err != nil {
		t.Fatalf("save agent: %v", err)
	}

	var commands []string
	provider.executeCommandFn = func(_ context.Context, _ string, command string) (string, error) {
		commands = append(commands, command)
		return "", nil
	}

	sb, err := orch.CreateAgentSandbox(context.Background(), &agent, agentStartupSecrets())
	if err != nil {
		t.Fatalf("CreateAgentSandbox: %v", err)
	}
	t.Cleanup(func() { db.Where("id = ?", sb.ID).Delete(&model.Sandbox{}) })

	if len(commands) != 3 {
		t.Fatalf("commands = %#v, want mkdir plus two repository clone commands", commands)
	}
	if commands[0] != "mkdir -p '/home/daytona/repos'" {
		t.Fatalf("mkdir command = %q, want quoted agent repo dir", commands[0])
	}
	wantFragments := []string{
		"git clone --depth=1 'https://github.com/octo-org/api.git' '/home/daytona/repos/api'",
		"git clone --depth=1 'https://github.com/octo-org/web.git' '/home/daytona/repos/web'",
	}
	for i, fragment := range wantFragments {
		if !strings.Contains(commands[i+1], fragment) {
			t.Fatalf("clone command %d = %q, want fragment %q", i+1, commands[i+1], fragment)
		}
	}
	for _, command := range commands {
		if strings.Contains(command, "123456") || strings.Contains(command, "789012") {
			t.Fatalf("clone command used numeric GitHub repository id: %q", command)
		}
	}
}

func TestCreateAgentSandbox_NoGitHubSelectionSkipsRepositoryClone(t *testing.T) {
	tests := []struct {
		name      string
		resources model.JSON
	}{
		{name: "no resources"},
		{name: "empty selection", resources: model.JSON{"github": map[string]any{"repository": []any{}}}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			orch, provider, db := setupOrchestrator(t)
			provider.endpointOverride = agentRuntimeEndpoint(t)
			orch.cfg.SandboxesRuntimeBaseImage = "ghcr.io/usehivy/hivy-sandboxes-runtime:test-v1"

			org := createTestOrg(t, db)
			cred := createTestCred(t, db, org.ID)
			agent := createTestAgent(t, db, org.ID, cred.ID)
			agent.IsManaged = true
			if err := db.Save(&agent).Error; err != nil {
				t.Fatalf("save agent: %v", err)
			}
			agent.Resources = tt.resources
			if err := db.Save(&agent).Error; err != nil {
				t.Fatalf("save agent resources: %v", err)
			}

			var commands []string
			provider.executeCommandFn = func(_ context.Context, _ string, command string) (string, error) {
				commands = append(commands, command)
				return "", nil
			}

			sb, err := orch.CreateAgentSandbox(context.Background(), &agent, agentStartupSecrets())
			if err != nil {
				t.Fatalf("CreateAgentSandbox: %v", err)
			}
			t.Cleanup(func() { db.Where("id = ?", sb.ID).Delete(&model.Sandbox{}) })

			if len(commands) != 0 {
				t.Fatalf("commands = %#v, want no repository clone commands", commands)
			}
		})
	}
}

func TestCreateAgentSandbox_RepositoryCloneFailureMarksSandboxError(t *testing.T) {
	orch, provider, db := setupOrchestrator(t)
	provider.endpointOverride = agentRuntimeEndpoint(t)
	orch.cfg.SandboxesRuntimeBaseImage = "ghcr.io/usehivy/hivy-sandboxes-runtime:test-v1"

	org := createTestOrg(t, db)
	cred := createTestCred(t, db, org.ID)
	agent := createTestAgent(t, db, org.ID, cred.ID)
	agent.IsManaged = true
	agent.Resources = model.JSON{
		"github": map[string]any{
			"repository": []any{
				map[string]any{"id": "123456", "name": "api", "full_name": "octo-org/api"},
			},
		},
	}
	if err := db.Save(&agent).Error; err != nil {
		t.Fatalf("save agent: %v", err)
	}

	provider.executeCommandFn = func(_ context.Context, _ string, command string) (string, error) {
		if strings.Contains(command, "git clone ") {
			return "", errors.New("clone failed")
		}
		return "", nil
	}

	sb, err := orch.CreateAgentSandbox(context.Background(), &agent, agentStartupSecrets())
	if err == nil {
		t.Fatal("CreateAgentSandbox succeeded, want repository clone failure")
	}
	if sb != nil {
		t.Fatalf("sandbox return = %#v, want nil on failure", sb)
	}

	var stored model.Sandbox
	if err := db.Where("agent_id = ?", agent.ID).Order("created_at DESC").First(&stored).Error; err != nil {
		t.Fatalf("load stored sandbox: %v", err)
	}
	if stored.Status != "error" {
		t.Fatalf("stored sandbox status = %q, want error", stored.Status)
	}
	if stored.ErrorMessage == nil || !strings.Contains(*stored.ErrorMessage, "repository cloning failed") {
		t.Fatalf("stored sandbox error_message = %v, want repository cloning failure", stored.ErrorMessage)
	}
}

func TestRestartAgentSandbox_UsesProviderRestartWhenAvailable(t *testing.T) {
	orch, provider, db := setupOrchestrator(t)
	provider.endpointOverride = agentRuntimeEndpoint(t)
	encryptedSecret, err := orch.encKey.EncryptString("restart-runtime-secret")
	if err != nil {
		t.Fatalf("encrypt runtime secret: %v", err)
	}
	sb := model.Sandbox{
		ID:                     uuid.New(),
		ExternalID:             "restartable-sandbox",
		EncryptedRuntimeSecret: encryptedSecret,
		Status:                 string(StatusRunning),
		ProviderID:             ProviderDaytona,
	}
	if err := db.Create(&sb).Error; err != nil {
		t.Fatalf("create sandbox: %v", err)
	}
	provider.registerSandbox(sb.ExternalID, StatusRunning)

	if err := orch.RestartAgentSandbox(context.Background(), &sb); err != nil {
		t.Fatalf("RestartAgentSandbox: %v", err)
	}
	if !reflect.DeepEqual(provider.restartedIDs, []string{"restartable-sandbox"}) {
		t.Fatalf("restartedIDs = %#v", provider.restartedIDs)
	}
	if len(provider.stoppedIDs) != 0 {
		t.Fatalf("stoppedIDs = %#v, want provider restart path", provider.stoppedIDs)
	}

	var stored model.Sandbox
	if err := db.First(&stored, "id = ?", sb.ID).Error; err != nil {
		t.Fatalf("load sandbox: %v", err)
	}
	if stored.Status != string(StatusRunning) || stored.RuntimeURL == "" {
		t.Fatalf("stored sandbox status/url = %q/%q", stored.Status, stored.RuntimeURL)
	}
}

func agentRuntimeEndpoint(t *testing.T) string {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/healthz" {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("ok"))
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	t.Cleanup(srv.Close)
	return srv.URL
}

func agentStartupSecrets() *agentruntime.StartupSecrets {
	return &agentruntime.StartupSecrets{
		ProxyToken: "ptok-test",
	}
}
