package sandbox

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/config"
	"github.com/usehivy/hivy/internal/crypto"
	"github.com/usehivy/hivy/internal/model"
)

func TestWarmPoolClaimUsesRequestedRuntimeImage(t *testing.T) {
	db := connectSandboxTestDB(t)
	encKey := sandboxTestSymmetricKey(t)
	provider := warmSlotImageTestProvider{id: "warm-image-provider-" + uuid.NewString()[:8]}
	cfg := &config.Config{SandboxesRuntimeImageTag: "v3.4.0-amd64"}
	pool := &WarmPool{db: db, provider: provider, encKey: encKey, cfg: cfg}

	defaultImage := AgentRuntimeImageRef(cfg, model.SandboxImageDefault)
	developerImage := AgentRuntimeImageRef(cfg, model.SandboxImageDeveloper)
	defaultSlotID := seedWarmSlot(t, db, encKey, provider.id, defaultImage, "default-external")
	developerSlotID := seedWarmSlot(t, db, encKey, provider.id, developerImage, "developer-external")
	sandboxID := seedClaimTargetSandbox(t, db, encKey, provider.id)

	claimed, err := pool.Claim(context.Background(), model.SandboxWarmSlotModeAgent, developerImage, sandboxID)
	if err != nil {
		t.Fatalf("claim developer warm slot: %v", err)
	}
	if claimed.ID != developerSlotID || claimed.ExternalID != "developer-external" {
		t.Fatalf("claimed slot = (%s,%s), want developer slot (%s,developer-external)", claimed.ID, claimed.ExternalID, developerSlotID)
	}

	var defaultSlot model.SandboxWarmSlot
	if err := db.First(&defaultSlot, "id = ?", defaultSlotID).Error; err != nil {
		t.Fatalf("load default slot: %v", err)
	}
	if defaultSlot.Status != model.SandboxWarmSlotStatusWarm {
		t.Fatalf("default slot status = %q, want warm", defaultSlot.Status)
	}
}

func TestWarmPoolReconcileMaintainsSeparateRuntimeImagePools(t *testing.T) {
	db := connectSandboxTestDB(t)
	provider := &warmSlotImageTestProvider{id: "warm-reconcile-provider-" + uuid.NewString()[:8]}
	cfg := &config.Config{
		SandboxWarmPoolAgentSize: 1,
		SandboxesRuntimeImageTag: "v3.4.0-amd64",
		RailwayRuntimePort:       AgentSandboxPort,
	}
	pool := &WarmPool{db: db, provider: provider, encKey: sandboxTestSymmetricKey(t), cfg: cfg}

	defaultImage := AgentRuntimeImageRef(cfg, model.SandboxImageDefault)
	developerImage := AgentRuntimeImageRef(cfg, model.SandboxImageDeveloper)
	if _, err := pool.Reconcile(context.Background(), model.SandboxWarmSlotModeAgent, defaultImage, nil); err != nil {
		t.Fatalf("reconcile default image: %v", err)
	}
	if _, err := pool.Reconcile(context.Background(), model.SandboxWarmSlotModeAgent, developerImage, nil); err != nil {
		t.Fatalf("reconcile developer image: %v", err)
	}

	var slots []model.SandboxWarmSlot
	if err := db.Where("provider_id = ? AND mode = ?", provider.id, model.SandboxWarmSlotModeAgent).Find(&slots).Error; err != nil {
		t.Fatalf("load warm slots: %v", err)
	}
	counts := map[string]int{}
	for _, slot := range slots {
		counts[slot.RuntimeImage]++
	}
	if counts[defaultImage] != 1 {
		t.Fatalf("default warm slot count = %d, want 1", counts[defaultImage])
	}
	if counts[developerImage] != 1 {
		t.Fatalf("developer warm slot count = %d, want 1", counts[developerImage])
	}
}

func seedClaimTargetSandbox(t *testing.T, db *gorm.DB, encKey *crypto.SymmetricKey, providerID string) uuid.UUID {
	t.Helper()
	encrypted, err := encKey.EncryptString("runtime-secret")
	if err != nil {
		t.Fatalf("encrypt sandbox runtime secret: %v", err)
	}
	sb := model.Sandbox{
		ID:                     uuid.New(),
		ProviderID:             providerID,
		ExternalID:             "claim-target",
		RuntimeURL:             "https://claim-target.example",
		EncryptedRuntimeSecret: encrypted,
		Status:                 "creating",
	}
	if err := db.Create(&sb).Error; err != nil {
		t.Fatalf("create claim target sandbox: %v", err)
	}
	return sb.ID
}

func seedWarmSlot(t *testing.T, db *gorm.DB, encKey *crypto.SymmetricKey, providerID, runtimeImage, externalID string) uuid.UUID {
	t.Helper()
	encrypted, err := encKey.EncryptString("runtime-secret")
	if err != nil {
		t.Fatalf("encrypt runtime secret: %v", err)
	}
	slot := model.SandboxWarmSlot{
		ProviderID:             providerID,
		Mode:                   model.SandboxWarmSlotModeAgent,
		Status:                 model.SandboxWarmSlotStatusWarm,
		ExternalID:             externalID,
		EndpointURL:            "https://" + externalID + ".example",
		RuntimeImage:           runtimeImage,
		RuntimePort:            AgentSandboxPort,
		EncryptedRuntimeSecret: encrypted,
	}
	if err := db.Create(&slot).Error; err != nil {
		t.Fatalf("create warm slot: %v", err)
	}
	return slot.ID
}

type warmSlotImageTestProvider struct {
	id      string
	creates int
}

func (p warmSlotImageTestProvider) ID() string { return p.id }

func (p warmSlotImageTestProvider) Validate(context.Context) error { return nil }

func (p warmSlotImageTestProvider) RuntimeLayout() RuntimeLayout {
	return RuntimeLayout{AgentRepoDir: "/workspace/repos", WorkspaceRepoDir: "/workspace/repos"}
}

func (p *warmSlotImageTestProvider) CreateWarmSlot(_ context.Context, opts WarmSlotCreateOpts) (*WarmSlotInfo, error) {
	p.creates++
	return &WarmSlotInfo{
		ExternalID:  fmt.Sprintf("warm-%d-%s", p.creates, uuid.NewString()[:8]),
		EndpointURL: fmt.Sprintf("https://warm-%d.example", p.creates),
		RuntimePort: opts.RuntimePort,
	}, nil
}

func (p warmSlotImageTestProvider) CreateSandbox(context.Context, CreateSandboxOpts) (*SandboxInfo, error) {
	return nil, ErrUnsupported
}

func (p warmSlotImageTestProvider) StartSandbox(context.Context, string) error { return nil }

func (p warmSlotImageTestProvider) StopSandbox(context.Context, string) error { return nil }

func (p warmSlotImageTestProvider) ArchiveSandbox(context.Context, string) error { return nil }

func (p warmSlotImageTestProvider) DeleteSandbox(context.Context, string) error { return nil }

func (p warmSlotImageTestProvider) GetStatus(context.Context, string) (SandboxStatus, error) {
	return StatusRunning, nil
}

func (p warmSlotImageTestProvider) GetEndpoint(_ context.Context, _ string, _ int) (string, error) {
	return "https://warm.example", nil
}

func (p warmSlotImageTestProvider) BuildTemplate(context.Context, TemplateBuildRequest) (string, error) {
	return "", ErrUnsupported
}

func (p warmSlotImageTestProvider) BuildTemplateWithLogs(context.Context, TemplateBuildRequest, func(string)) (string, error) {
	return "", ErrUnsupported
}

func (p warmSlotImageTestProvider) GetTemplateStatus(context.Context, string) (*TemplateBuildStatus, error) {
	return nil, ErrUnsupported
}

func (p warmSlotImageTestProvider) GetTemplateLogs(context.Context, string) (string, error) {
	return "", ErrUnsupported
}

func (p warmSlotImageTestProvider) DeleteTemplate(context.Context, string) error { return nil }

func (p warmSlotImageTestProvider) SetAutoStop(context.Context, string, int) error { return nil }

func (p warmSlotImageTestProvider) SetAutoArchive(context.Context, string, int) error { return nil }

func (p warmSlotImageTestProvider) ExecuteCommand(context.Context, string, string) (string, error) {
	return "", ErrUnsupported
}

func (p warmSlotImageTestProvider) ExecuteCommandWithTimeout(context.Context, string, string, time.Duration) (string, error) {
	return "", ErrUnsupported
}

func (p warmSlotImageTestProvider) GetResourceUsage(context.Context, string) (*ResourceUsage, error) {
	return nil, ErrUnsupported
}
