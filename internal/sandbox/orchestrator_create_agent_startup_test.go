package sandbox

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/usehivy/hivy/internal/agentruntime"
	"github.com/usehivy/hivy/internal/config"
	"github.com/usehivy/hivy/internal/model"
)

func TestCreateAgentSandboxPushesStartupProxyToken(t *testing.T) {
	db := connectSandboxTestDB(t)
	orgID := uuid.New()
	org := model.Org{ID: orgID, Name: "Startup Token Test", RateLimit: 1000, Active: true}
	if err := db.Create(&org).Error; err != nil {
		t.Fatalf("create org: %v", err)
	}
	team := model.Team{ID: uuid.New(), OrgID: orgID, Name: "sandbox-team-" + uuid.NewString()[:8]}
	if err := db.Create(&team).Error; err != nil {
		t.Fatalf("create team: %v", err)
	}
	agent := model.Agent{
		ID:            uuid.New(),
		OrgID:         &orgID,
		TeamID:        team.ID,
		Name:          "Startup Token Agent",
		Model:         "gpt-5.4",
		Status:        "active",
		Tools:         model.JSON{},
		McpServers:    model.RawJSON("[]"),
		Skills:        model.JSON{},
		RuntimeConfig: model.JSON{},
		Permissions:   model.JSON{},
		Resources:     model.JSON{},
	}
	if err := db.Create(&agent).Error; err != nil {
		t.Fatalf("create agent: %v", err)
	}
	t.Cleanup(func() {
		db.Where("agent_id = ?", agent.ID).Delete(&model.Sandbox{})
		db.Where("id = ?", agent.ID).Delete(&model.Agent{})
		db.Where("id = ?", team.ID).Delete(&model.Team{})
		db.Where("id = ?", org.ID).Delete(&model.Org{})
	})

	runtime := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/healthz" {
			http.NotFound(w, r)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer runtime.Close()

	var pushedJTI string
	orch := NewOrchestrator(db, &agentCreateProvider{endpoint: runtime.URL}, sandboxTestSymmetricKey(t), &config.Config{
		SandboxProviderID: ProviderMicrosandbox,
		APIWebhookBaseURL: "https://api.example",
		ProxyHost:         "https://proxy.example",
	})
	orch.SetAgentRuntimeConfigPusher(func(_ context.Context, _ *model.Sandbox, push AgentRuntimeConfigPush) error {
		if push.ProxyToken == nil {
			t.Fatal("proxy token was nil")
		}
		pushedJTI = push.ProxyToken.JTI
		return nil
	})
	secrets := &agentruntime.StartupSecrets{
		ProxyToken:    "proxy-token",
		ProxyTokenJTI: uuid.NewString(),
		ProxyExpires:  time.Now().Add(time.Hour),
	}

	if _, err := orch.CreateAgentSandbox(context.Background(), &agent, secrets); err != nil {
		t.Fatalf("CreateAgentSandbox: %v", err)
	}
	if pushedJTI != secrets.ProxyTokenJTI {
		t.Fatalf("pushed jti = %q, want startup jti %q", pushedJTI, secrets.ProxyTokenJTI)
	}
}

func testStartupSecrets() *agentruntime.StartupSecrets {
	return &agentruntime.StartupSecrets{
		ProxyToken:    "proxy-token",
		ProxyTokenJTI: uuid.NewString(),
		ProxyExpires:  time.Now().Add(time.Hour),
	}
}
