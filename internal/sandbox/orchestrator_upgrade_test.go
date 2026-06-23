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

type upgradeAgentProvider struct {
	agentCreateProvider
	upgraded []UpgradeSandboxOpts
}

func (p *upgradeAgentProvider) UpgradeSandbox(_ context.Context, _ string, opts UpgradeSandboxOpts) (*SandboxInfo, error) {
	p.upgraded = append(p.upgraded, opts)
	return &SandboxInfo{ExternalID: "external-upgraded", Status: StatusRunning}, nil
}

func TestUpgradeAgentSandboxInPlacePushesRuntimeConfig(t *testing.T) {
	db := connectSandboxTestDB(t)
	orgID := uuid.New()
	org := model.Org{ID: orgID, Name: "Upgrade Config Test", RateLimit: 1000, Active: true}
	if err := db.Create(&org).Error; err != nil {
		t.Fatalf("create org: %v", err)
	}
	agent := model.Agent{
		ID:              uuid.New(),
		OrgID:           &orgID,
		Name:            "Upgrade Agent",
		SandboxStrategy: "per_session",
		Model:           "gpt-5.4",
		Status:          "active",
		Tools:           model.JSON{},
		McpServers:      model.RawJSON("[]"),
		Skills:          model.JSON{},
		RuntimeConfig:   model.JSON{},
		Permissions:     model.JSON{},
		Resources:       model.JSON{},
	}
	if err := db.Create(&agent).Error; err != nil {
		t.Fatalf("create agent: %v", err)
	}
	encKey := sandboxTestSymmetricKey(t)
	encryptedSecret, err := encKey.EncryptString("runtime-secret")
	if err != nil {
		t.Fatalf("encrypt runtime secret: %v", err)
	}
	snapshotID := "ghcr.io/usehivy/hivy-sandboxes-runtime:v1"
	sb := model.Sandbox{
		OrgID:                  &orgID,
		AgentID:                &agent.ID,
		SnapshotID:             &snapshotID,
		ProviderID:             ProviderMicrosandbox,
		ExternalID:             "external-old",
		EncryptedRuntimeSecret: encryptedSecret,
		Status:                 string(StatusRunning),
	}
	if err := db.Create(&sb).Error; err != nil {
		t.Fatalf("create sandbox: %v", err)
	}
	t.Cleanup(func() {
		db.Where("agent_id = ?", agent.ID).Delete(&model.Sandbox{})
		db.Where("id = ?", agent.ID).Delete(&model.Agent{})
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

	provider := &upgradeAgentProvider{agentCreateProvider: agentCreateProvider{endpoint: runtime.URL}}
	orch := NewOrchestrator(db, provider, encKey, &config.Config{
		SandboxProviderID:         ProviderMicrosandbox,
		SandboxesRuntimeBaseImage: "ghcr.io/usehivy/hivy-sandboxes-runtime:v2",
		APIWebhookBaseURL:         "https://api.example",
		ProxyHost:                 "https://proxy.example",
	})
	secrets := &agentruntime.StartupSecrets{
		ProxyToken:    "proxy-token",
		ProxyTokenJTI: uuid.NewString(),
		ProxyExpires:  time.Now().Add(time.Hour),
	}
	var pushedJTI string
	var pushedSandboxID uuid.UUID
	orch.SetAgentRuntimeConfigPusher(func(_ context.Context, sb *model.Sandbox, proxyToken *agentruntime.ProxyTokenResult) error {
		pushedSandboxID = sb.ID
		if proxyToken == nil {
			t.Fatal("proxy token was nil")
		}
		pushedJTI = proxyToken.JTI
		return nil
	})

	upgraded, err := orch.UpgradeAgentSandboxInPlace(context.Background(), &agent, &sb, secrets)
	if err != nil {
		t.Fatalf("UpgradeAgentSandboxInPlace: %v", err)
	}
	if len(provider.upgraded) != 1 {
		t.Fatalf("provider upgrades = %d, want 1", len(provider.upgraded))
	}
	if upgraded.ExternalID != "external-upgraded" {
		t.Fatalf("external id = %q, want external-upgraded", upgraded.ExternalID)
	}
	if pushedSandboxID != sb.ID {
		t.Fatalf("pushed sandbox id = %s, want %s", pushedSandboxID, sb.ID)
	}
	if pushedJTI != secrets.ProxyTokenJTI {
		t.Fatalf("pushed jti = %q, want startup jti %q", pushedJTI, secrets.ProxyTokenJTI)
	}
}
