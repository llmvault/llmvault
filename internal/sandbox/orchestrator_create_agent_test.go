package sandbox

import (
	"context"
	"encoding/base64"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/agentruntime"
	"github.com/usehivy/hivy/internal/config"
	"github.com/usehivy/hivy/internal/crypto"
	"github.com/usehivy/hivy/internal/model"
	"github.com/usehivy/hivy/internal/testdb"
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
	if created.SnapshotID == nil || *created.SnapshotID != "hivy-sandboxes-runtime-v3-3-0-amd64-xlarge" {
		t.Fatalf("snapshot id = %v, want xlarge runtime snapshot", created.SnapshotID)
	}
	if len(provider.created) != 1 {
		t.Fatalf("provider creates = %d, want 1", len(provider.created))
	}
	opts := provider.created[0]
	if opts.TemplateRef != "hivy-sandboxes-runtime-v3-3-0-amd64-xlarge" {
		t.Fatalf("template ref = %q, want xlarge snapshot alias", opts.TemplateRef)
	}
	if opts.CPU != 8 || opts.Memory != 16 || opts.Disk != 60 {
		t.Fatalf("resources = cpu:%d memory:%d disk:%d, want 8/16/60", opts.CPU, opts.Memory, opts.Disk)
	}
	if opts.Labels["sandbox_size"] != "xlarge" {
		t.Fatalf("sandbox_size label = %q, want xlarge", opts.Labels["sandbox_size"])
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
	wantSnapshot := "hivy-sandboxes-runtime-developers-v3-4-0-amd64-large"
	if created.SnapshotID == nil || *created.SnapshotID != wantSnapshot {
		t.Fatalf("snapshot id = %v, want %s", created.SnapshotID, wantSnapshot)
	}
	if len(provider.created) != 1 {
		t.Fatalf("provider creates = %d, want 1", len(provider.created))
	}
	opts := provider.created[0]
	if opts.TemplateRef != wantSnapshot {
		t.Fatalf("template ref = %q, want %q", opts.TemplateRef, wantSnapshot)
	}
	if opts.Labels["sandbox_image"] != model.SandboxImageDeveloper {
		t.Fatalf("sandbox_image label = %q, want developer", opts.Labels["sandbox_image"])
	}
}

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

func connectSandboxTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	dsn := testdb.DatabaseURL("DATABASE_URL", "HIVY_DATABASE_URL", "TEST_DATABASE_URL")
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("cannot connect to Postgres: %v", err)
	}
	sqlDB, _ := db.DB()
	sqlDB.SetMaxOpenConns(3)
	sqlDB.SetMaxIdleConns(1)
	testdb.ApplyMigrations(t, db)
	t.Cleanup(func() { sqlDB.Close() })
	return db
}

func sandboxTestSymmetricKey(t *testing.T) *crypto.SymmetricKey {
	t.Helper()
	key, err := crypto.NewSymmetricKey(base64.StdEncoding.EncodeToString(make([]byte, 32)))
	if err != nil {
		t.Fatalf("symmetric key: %v", err)
	}
	return key
}

type agentCreateProvider struct {
	endpoint string
	created  []CreateSandboxOpts
}

func (p *agentCreateProvider) ID() string { return ProviderMicrosandbox }

func (p *agentCreateProvider) Validate(context.Context) error { return nil }

func (p *agentCreateProvider) RuntimeLayout() RuntimeLayout {
	return RuntimeLayout{AgentRepoDir: "/workspace/repos", WorkspaceRepoDir: "/workspace/repos"}
}

func (p *agentCreateProvider) CreateSandbox(_ context.Context, opts CreateSandboxOpts) (*SandboxInfo, error) {
	p.created = append(p.created, opts)
	return &SandboxInfo{ExternalID: fmt.Sprintf("external-%d", len(p.created)), Status: StatusRunning}, nil
}

func (p *agentCreateProvider) StartSandbox(context.Context, string) error { return nil }

func (p *agentCreateProvider) StopSandbox(context.Context, string) error { return nil }

func (p *agentCreateProvider) ArchiveSandbox(context.Context, string) error { return nil }

func (p *agentCreateProvider) DeleteSandbox(context.Context, string) error { return nil }

func (p *agentCreateProvider) GetStatus(context.Context, string) (SandboxStatus, error) {
	return StatusRunning, nil
}

func (p *agentCreateProvider) GetEndpoint(context.Context, string, int) (string, error) {
	return p.endpoint, nil
}

func (p *agentCreateProvider) BuildTemplate(context.Context, TemplateBuildRequest) (string, error) {
	return "", nil
}

func (p *agentCreateProvider) BuildTemplateWithLogs(context.Context, TemplateBuildRequest, func(string)) (string, error) {
	return "", nil
}

func (p *agentCreateProvider) GetTemplateStatus(context.Context, string) (*TemplateBuildStatus, error) {
	return &TemplateBuildStatus{State: "ready"}, nil
}

func (p *agentCreateProvider) GetTemplateLogs(context.Context, string) (string, error) {
	return "", nil
}

func (p *agentCreateProvider) DeleteTemplate(context.Context, string) error { return nil }

func (p *agentCreateProvider) SetAutoStop(context.Context, string, int) error { return nil }

func (p *agentCreateProvider) SetAutoArchive(context.Context, string, int) error { return nil }

func (p *agentCreateProvider) ExecuteCommand(context.Context, string, string) (string, error) {
	return "", nil
}

func (p *agentCreateProvider) ExecuteCommandWithTimeout(context.Context, string, string, time.Duration) (string, error) {
	return "", nil
}

func (p *agentCreateProvider) GetResourceUsage(context.Context, string) (*ResourceUsage, error) {
	return &ResourceUsage{}, nil
}
