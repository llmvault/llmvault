package sandbox

import (
	"context"
	"net/http"
	"net/http/httptest"
	"slices"
	"testing"

	"github.com/google/uuid"

	"github.com/usehivy/hivy/internal/agentruntime"
	"github.com/usehivy/hivy/internal/config"
	"github.com/usehivy/hivy/internal/model"
)

func TestCreateAgentSandboxUsesConfiguredSandboxSize(t *testing.T) {
	db := connectSandboxTestDB(t)
	orgID := uuid.New()
	org := model.Org{ID: orgID, Name: "Sandbox Size Test", RateLimit: 1000, Active: true}
	if err := db.Create(&org).Error; err != nil {
		t.Fatalf("create org: %v", err)
	}
	agent := model.Agent{
		ID:              uuid.New(),
		OrgID:           &orgID,
		Name:            "Runtime Agent",
		SandboxStrategy: "always_on",
		SandboxSize:     "xlarge",
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

	provider := &agentCreateProvider{endpoint: runtime.URL}
	encKey := sandboxTestSymmetricKey(t)
	cfg := &config.Config{
		SandboxProviderID:         ProviderMicrosandbox,
		SandboxesRuntimeBaseImage: "ghcr.io/usehivy/hivy-sandboxes-runtime:v3.3.0-amd64",
		APIWebhookBaseURL:         "https://api.example",
		ProxyHost:                 "https://proxy.example",
	}
	orch := NewOrchestrator(db, provider, encKey, cfg)

	created, err := orch.CreateAgentSandbox(context.Background(), &agent, &agentruntime.StartupSecrets{ProxyToken: "proxy-token"})
	if err != nil {
		t.Fatalf("CreateAgentSandbox: %v", err)
	}
	wantRuntimeRef := "ghcr.io/usehivy/hivy-sandboxes-runtime:v3.3.0-amd64"
	if created.SnapshotID == nil || *created.SnapshotID != wantRuntimeRef {
		t.Fatalf("snapshot id = %v, want runtime image ref %s", created.SnapshotID, wantRuntimeRef)
	}
	if len(provider.created) != 1 {
		t.Fatalf("provider creates = %d, want 1", len(provider.created))
	}
	opts := provider.created[0]
	if opts.TemplateRef != wantRuntimeRef {
		t.Fatalf("template ref = %q, want %q", opts.TemplateRef, wantRuntimeRef)
	}
	if opts.CPU != 8 || opts.Memory != 16 || opts.Disk != 60 {
		t.Fatalf("resources = cpu:%d memory:%d disk:%d, want 8/16/60", opts.CPU, opts.Memory, opts.Disk)
	}
	if opts.Labels["sandbox_size"] != "xlarge" {
		t.Fatalf("sandbox_size label = %q, want xlarge", opts.Labels["sandbox_size"])
	}
}

func TestCreateAgentSandboxUsesOrgSandboxExposedPorts(t *testing.T) {
	db := connectSandboxTestDB(t)
	orgID := uuid.New()
	org := model.Org{
		ID:                  orgID,
		Name:                "Sandbox Ports Test",
		RateLimit:           1000,
		Active:              true,
		SandboxExposedPorts: model.SandboxExposedPortsInt64Array([]int{9000, 3000, 9000}),
	}
	if err := db.Create(&org).Error; err != nil {
		t.Fatalf("create org: %v", err)
	}
	agent := model.Agent{
		ID:              uuid.New(),
		OrgID:           &orgID,
		Name:            "Preview Agent",
		SandboxStrategy: "always_on",
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
	wantPorts := []int{3000, 9000}
	if len(provider.created) != 1 {
		t.Fatalf("provider creates = %d, want 1", len(provider.created))
	}
	if !slices.Equal(provider.created[0].ExposedPorts, wantPorts) {
		t.Fatalf("provider exposed ports = %v, want %v", provider.created[0].ExposedPorts, wantPorts)
	}
	if !slices.Equal(model.SandboxExposedPortsFromInt64Array(created.ExposedPorts), wantPorts) {
		t.Fatalf("created sandbox exposed ports = %v, want %v", created.ExposedPorts, wantPorts)
	}
}

func TestCreateAgentSandboxUsesAgentSandboxImage(t *testing.T) {
	db := connectSandboxTestDB(t)
	orgID := uuid.New()
	org := model.Org{ID: orgID, Name: "Sandbox Image Test", RateLimit: 1000, Active: true}
	if err := db.Create(&org).Error; err != nil {
		t.Fatalf("create org: %v", err)
	}
	agent := model.Agent{
		ID:              uuid.New(),
		OrgID:           &orgID,
		Name:            "Developer Agent",
		SandboxStrategy: "per_session",
		SandboxImage:    model.SandboxImageDeveloper,
		SandboxSize:     "large",
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

	provider := &agentCreateProvider{endpoint: runtime.URL}
	orch := NewOrchestrator(db, provider, sandboxTestSymmetricKey(t), &config.Config{
		SandboxProviderID:        ProviderMicrosandbox,
		SandboxesRuntimeImageTag: "v3.4.0-amd64",
		APIWebhookBaseURL:        "https://api.example",
		ProxyHost:                "https://proxy.example",
	})

	created, err := orch.CreateAgentSandbox(context.Background(), &agent, &agentruntime.StartupSecrets{ProxyToken: "proxy-token"})
	if err != nil {
		t.Fatalf("CreateAgentSandbox: %v", err)
	}
	wantRuntimeRef := "ghcr.io/usehivy/hivy-sandboxes-runtime-developers:v3.4.0-amd64"
	if created.SnapshotID == nil || *created.SnapshotID != wantRuntimeRef {
		t.Fatalf("snapshot id = %v, want %s", created.SnapshotID, wantRuntimeRef)
	}
	if len(provider.created) != 1 {
		t.Fatalf("provider creates = %d, want 1", len(provider.created))
	}
	opts := provider.created[0]
	if opts.TemplateRef != wantRuntimeRef {
		t.Fatalf("template ref = %q, want %q", opts.TemplateRef, wantRuntimeRef)
	}
	if opts.Labels["sandbox_image"] != model.SandboxImageDeveloper {
		t.Fatalf("sandbox_image label = %q, want developer", opts.Labels["sandbox_image"])
	}
}

func TestCreateAgentSandboxWarmPoolEmptyFallsBackToDirectCreate(t *testing.T) {
	db := connectSandboxTestDB(t)
	orgID := uuid.New()
	org := model.Org{ID: orgID, Name: "Warm Fallback Test", RateLimit: 1000, Active: true}
	if err := db.Create(&org).Error; err != nil {
		t.Fatalf("create org: %v", err)
	}
	agent := model.Agent{
		ID:              uuid.New(),
		OrgID:           &orgID,
		Name:            "Warm Agent",
		SandboxStrategy: "per_session",
		SandboxImage:    model.SandboxImageDefault,
		SandboxSize:     "small",
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

	created, err := orch.CreateAgentSandbox(context.Background(), &agent, &agentruntime.StartupSecrets{ProxyToken: "proxy-token"})
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
