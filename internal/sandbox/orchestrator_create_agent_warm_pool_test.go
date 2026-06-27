package sandbox

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"

	"github.com/usehivy/hivy/internal/config"
	"github.com/usehivy/hivy/internal/model"
)

func TestCreateAgentSandboxWarmPoolEmptyFallsBackToDirectCreate(t *testing.T) {
	db := connectSandboxTestDB(t)
	orgID := uuid.New()
	org := model.Org{ID: orgID, Name: "Warm Fallback Test", RateLimit: 1000, Active: true}
	if err := db.Create(&org).Error; err != nil {
		t.Fatalf("create org: %v", err)
	}
	agent := model.Agent{
		ID:            uuid.New(),
		OrgID:         &orgID,
		Name:          "Warm Agent",
		SandboxImage:  model.SandboxImageDefault,
		SandboxSize:   "small",
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

	provider := &warmCapableAgentCreateProvider{
		agentCreateProvider: agentCreateProvider{endpoint: runtime.URL},
	}
	cfg := &config.Config{
		SandboxProviderID:          ProviderMicrosandbox,
		SandboxWarmPoolDefaultSize: 1,
		SandboxesRuntimeImageTag:   "v3.4.0-amd64",
		APIWebhookBaseURL:          "https://api.example",
		ProxyHost:                  "https://proxy.example",
	}
	orch := NewOrchestrator(db, provider, sandboxTestSymmetricKey(t), cfg)
	var reconciles []WarmPoolProfile
	orch.SetWarmPoolReconciler(func(_ context.Context, providerID string, profile WarmPoolProfile) error {
		if providerID != ProviderMicrosandbox {
			t.Fatalf("provider id = %q, want microsandbox", providerID)
		}
		reconciles = append(reconciles, profile)
		return nil
	})

	created, err := orch.CreateAgentSandbox(context.Background(), &agent, testStartupSecrets())
	if err != nil {
		t.Fatalf("CreateAgentSandbox: %v", err)
	}
	if created.ExternalID != "external-1" {
		t.Fatalf("external id = %q, want direct-created external-1", created.ExternalID)
	}
	if len(provider.created) != 1 {
		t.Fatalf("provider creates = %d, want 1 direct create", len(provider.created))
	}
	if len(reconciles) != 1 {
		t.Fatalf("warm pool reconciles = %d, want 1", len(reconciles))
	}
	if reconciles[0].ImageKind != model.SandboxImageDefault || reconciles[0].SandboxSize != "small" {
		t.Fatalf("reconcile profile = %+v, want default small", reconciles[0])
	}
}
