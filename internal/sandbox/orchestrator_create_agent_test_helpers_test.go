package sandbox

import (
	"context"
	"encoding/base64"
	"fmt"
	"testing"
	"time"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/crypto"
	"github.com/usehivy/hivy/internal/testdb"
)

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
	endpoint   string
	providerID string
	created    []CreateSandboxOpts
	started    []string
}

func (p *agentCreateProvider) ID() string {
	if p.providerID != "" {
		return p.providerID
	}
	return ProviderMicrosandbox
}

func (p *agentCreateProvider) Validate(context.Context) error { return nil }

func (p *agentCreateProvider) RuntimeLayout() RuntimeLayout {
	return RuntimeLayout{AgentRepoDir: "/workspace/repos", WorkspaceRepoDir: "/workspace/repos"}
}

func (p *agentCreateProvider) CreateSandbox(_ context.Context, opts CreateSandboxOpts) (*SandboxInfo, error) {
	p.created = append(p.created, opts)
	return &SandboxInfo{ExternalID: fmt.Sprintf("external-%d", len(p.created)), Status: StatusRunning}, nil
}

func (p *agentCreateProvider) StartSandbox(_ context.Context, externalID string) error {
	p.started = append(p.started, externalID)
	return nil
}

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

func (p *agentCreateProvider) SetAutoStop(context.Context, string, time.Duration) error { return nil }

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

type warmCapableAgentCreateProvider struct {
	agentCreateProvider
	warmCreated []WarmSlotCreateOpts
}

func (p *warmCapableAgentCreateProvider) UsesWarmPool() bool { return true }

func (p *warmCapableAgentCreateProvider) CreateWarmSlot(_ context.Context, opts WarmSlotCreateOpts) (*WarmSlotInfo, error) {
	p.warmCreated = append(p.warmCreated, opts)
	return &WarmSlotInfo{
		ExternalID:  fmt.Sprintf("warm-external-%d", len(p.warmCreated)),
		EndpointURL: p.endpoint,
		RuntimePort: opts.RuntimePort,
	}, nil
}
