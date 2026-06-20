package sandbox

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"

	"github.com/usehivy/hivy/internal/agentruntime"
	"github.com/usehivy/hivy/internal/config"
	"github.com/usehivy/hivy/internal/model"
)

func TestCreateAgentSandboxUsesReadySandboxTemplateExternalID(t *testing.T) {
	db := connectSandboxTestDB(t)
	orgID := uuid.New()
	org := model.Org{ID: orgID, Name: "Sandbox Template Test", RateLimit: 1000, Active: true}
	if err := db.Create(&org).Error; err != nil {
		t.Fatalf("create org: %v", err)
	}
	externalID := "tpl_custom123"
	tmpl := model.SandboxTemplate{
		ID:            uuid.New(),
		OrgID:         &orgID,
		Name:          "Custom Runtime",
		Slug:          fmt.Sprintf("custom-runtime-%s", uuid.NewString()[:8]),
		Size:          "large",
		ProviderID:    ProviderMicrosandbox,
		ExternalID:    &externalID,
		BuildStatus:   "ready",
		BuildCommands: "apt-get install -y openjdk-21-jdk",
		Tags:          model.JSON{},
		Config:        model.JSON{},
	}
	if err := db.Create(&tmpl).Error; err != nil {
		t.Fatalf("create template: %v", err)
	}
	agent := model.Agent{
		ID:                uuid.New(),
		OrgID:             &orgID,
		Name:              "Template Agent",
		SandboxStrategy:   "per_session",
		SandboxTemplateID: &tmpl.ID,
		SandboxSize:       "small",
		Model:             "gpt-5.4",
		Status:            "active",
		Tools:             model.JSON{},
		McpServers:        model.RawJSON("[]"),
		Skills:            model.JSON{},
		RuntimeConfig:     model.JSON{},
		Permissions:       model.JSON{},
		Resources:         model.JSON{},
	}
	if err := db.Create(&agent).Error; err != nil {
		t.Fatalf("create agent: %v", err)
	}
	t.Cleanup(func() {
		db.Where("agent_id = ?", agent.ID).Delete(&model.Sandbox{})
		db.Where("id = ?", agent.ID).Delete(&model.Agent{})
		db.Where("id = ?", tmpl.ID).Delete(&model.SandboxTemplate{})
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

	provider := &agentCreateProvider{endpoint: runtime.URL}
	orch := NewOrchestrator(db, provider, sandboxTestSymmetricKey(t), &config.Config{
		SandboxProviderID:         ProviderMicrosandbox,
		SandboxesRuntimeBaseImage: "ghcr.io/usehivy/hivy-sandboxes-runtime:v3.3.0-amd64",
		APIWebhookBaseURL:         "https://api.example",
		ProxyHost:                 "https://proxy.example",
	})

	created, err := orch.CreateAgentSandbox(context.Background(), &agent, &agentruntime.StartupSecrets{ProxyToken: "proxy-token"})
	if err != nil {
		t.Fatalf("CreateAgentSandbox: %v", err)
	}
	if created.SandboxTemplateID == nil || *created.SandboxTemplateID != tmpl.ID {
		t.Fatalf("sandbox template id = %v, want %s", created.SandboxTemplateID, tmpl.ID)
	}
	if created.SnapshotID == nil || *created.SnapshotID != externalID {
		t.Fatalf("snapshot/template ref = %v, want %s", created.SnapshotID, externalID)
	}
	if len(provider.created) != 1 {
		t.Fatalf("provider creates = %d, want 1", len(provider.created))
	}
	opts := provider.created[0]
	if opts.TemplateRef != externalID {
		t.Fatalf("template ref = %q, want %q", opts.TemplateRef, externalID)
	}
	if opts.CPU != 4 || opts.Memory != 8 || opts.Disk != 40 {
		t.Fatalf("resources = cpu:%d memory:%d disk:%d, want 4/8/40", opts.CPU, opts.Memory, opts.Disk)
	}
}
