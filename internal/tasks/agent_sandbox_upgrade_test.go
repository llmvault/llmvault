package tasks

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/hibiken/asynq"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/agentruntime"
	"github.com/usehivy/hivy/internal/config"
	"github.com/usehivy/hivy/internal/crypto"
	"github.com/usehivy/hivy/internal/enqueue"
	"github.com/usehivy/hivy/internal/model"
	"github.com/usehivy/hivy/internal/sandbox"
)

func TestAgentSandboxUpgradeSkipsRuntimeDrainWhenAlwaysOnSessionsIdle(t *testing.T) {
	harness := newAgentSandboxUpgradeHarness(t)
	org, agent, channel := seedAgentSandboxUpgradeFixture(t, harness.db, "always_on")
	oldSandbox := seedAgentSandboxUpgradeSandbox(t, harness, org.ID, agent.ID, "old-runtime")
	seedAgentSandboxUpgradeSession(t, harness.db, org.ID, channel.ID, agent.ID, oldSandbox.ID, model.SessionAgentTurnIdle)
	upgrade := seedAgentSandboxUpgradeRow(t, harness.db, org.ID, agent.ID, oldSandbox.ID)

	if err := harness.handler.Handle(t.Context(), agentSandboxUpgradeTask(t, upgrade.ID, agent.ID)); err != nil {
		t.Fatalf("handle upgrade: %v", err)
	}

	harness.runtime.assertDrainCalls(t, 0, 0)
	assertAgentSandboxUpgradeSucceeded(t, harness.db, upgrade.ID)
	assertAgentSandboxUpgradeActiveSandbox(t, harness.db, org.ID, agent.ID, oldSandbox.ID)
	if len(harness.provider.stopped) != 1 || harness.provider.stopped[0] != oldSandbox.ExternalID {
		t.Fatalf("stopped=%v want old sandbox %s", harness.provider.stopped, oldSandbox.ExternalID)
	}
}

func TestAgentSandboxUpgradeDrainsWhenAnyAlwaysOnSessionActive(t *testing.T) {
	harness := newAgentSandboxUpgradeHarness(t)
	org, agent, channel := seedAgentSandboxUpgradeFixture(t, harness.db, "always_on")
	oldSandbox := seedAgentSandboxUpgradeSandbox(t, harness, org.ID, agent.ID, "old-runtime")
	seedAgentSandboxUpgradeSession(t, harness.db, org.ID, channel.ID, agent.ID, oldSandbox.ID, model.SessionAgentTurnActive)
	upgrade := seedAgentSandboxUpgradeRow(t, harness.db, org.ID, agent.ID, oldSandbox.ID)

	if err := harness.handler.Handle(t.Context(), agentSandboxUpgradeTask(t, upgrade.ID, agent.ID)); err != nil {
		t.Fatalf("handle upgrade: %v", err)
	}

	harness.runtime.assertDrainCalls(t, 1, 1)
	assertAgentSandboxUpgradeSucceeded(t, harness.db, upgrade.ID)
}

func TestAgentSandboxUpgradeContinuesWhenDrainSignalFailsTwice(t *testing.T) {
	harness := newAgentSandboxUpgradeHarness(t)
	harness.runtime.failNextDrainPOSTs(2)
	org, agent, channel := seedAgentSandboxUpgradeFixture(t, harness.db, "always_on")
	oldSandbox := seedAgentSandboxUpgradeSandbox(t, harness, org.ID, agent.ID, "old-runtime")
	seedAgentSandboxUpgradeSession(t, harness.db, org.ID, channel.ID, agent.ID, oldSandbox.ID, model.SessionAgentTurnActive)
	upgrade := seedAgentSandboxUpgradeRow(t, harness.db, org.ID, agent.ID, oldSandbox.ID)

	if err := harness.handler.Handle(t.Context(), agentSandboxUpgradeTask(t, upgrade.ID, agent.ID)); err != nil {
		t.Fatalf("handle upgrade: %v", err)
	}

	harness.runtime.assertDrainCalls(t, 2, 0)
	assertAgentSandboxUpgradeSucceeded(t, harness.db, upgrade.ID)
	assertAgentSandboxUpgradeActiveSandbox(t, harness.db, org.ID, agent.ID, oldSandbox.ID)
	if len(harness.provider.stopped) != 1 || harness.provider.stopped[0] != oldSandbox.ExternalID {
		t.Fatalf("stopped=%v want old sandbox %s", harness.provider.stopped, oldSandbox.ExternalID)
	}
}

func TestAgentSandboxUpgradeTaskMarksPerSessionAgentFailed(t *testing.T) {
	harness := newAgentSandboxUpgradeHarness(t)
	org, agent, _ := seedAgentSandboxUpgradeFixture(t, harness.db, "per_session")
	oldSandbox := seedAgentSandboxUpgradeSandbox(t, harness, org.ID, agent.ID, "old-runtime")
	upgrade := seedAgentSandboxUpgradeRow(t, harness.db, org.ID, agent.ID, oldSandbox.ID)

	if err := harness.handler.Handle(t.Context(), agentSandboxUpgradeTask(t, upgrade.ID, agent.ID)); err != nil {
		t.Fatalf("handle upgrade: %v", err)
	}

	var stored model.AgentSandboxUpgrade
	if err := harness.db.First(&stored, "id = ?", upgrade.ID).Error; err != nil {
		t.Fatalf("load upgrade: %v", err)
	}
	if stored.Status != model.AgentSandboxUpgradeStatusFailed {
		t.Fatalf("status=%s want failed", stored.Status)
	}
	if stored.ErrorMessage == nil || *stored.ErrorMessage != "per-session agents do not use sandbox upgrades" {
		t.Fatalf("error=%v", stored.ErrorMessage)
	}
	if len(harness.provider.created) != 0 {
		t.Fatalf("created replacement sandboxes=%d, want 0", len(harness.provider.created))
	}
}

func TestAgentSandboxAutoUpgradeIgnoresPerSessionAgents(t *testing.T) {
	db := connectTestDB(t)
	alwaysOrg, alwaysAgent, _ := seedAgentSandboxUpgradeFixture(t, db, "always_on")
	perOrg, perAgent, _ := seedAgentSandboxUpgradeFixture(t, db, "per_session")
	seedAgentSandboxAutoUpgradeCandidate(t, db, alwaysOrg.ID, alwaysAgent.ID, "always-on")
	seedAgentSandboxAutoUpgradeCandidate(t, db, perOrg.ID, perAgent.ID, "per-session")

	handler := NewAgentSandboxAutoUpgradeHandler(db, agentruntime.CompileDeps{Cfg: &config.Config{}}, &enqueue.MockClient{})
	rows, err := handler.loadOutdatedAgentSandboxes(t.Context(), "new-runtime-image", 100)
	if err != nil {
		t.Fatalf("load outdated sandboxes: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("rows=%d want 1: %+v", len(rows), rows)
	}
	if rows[0].AgentID != alwaysAgent.ID {
		t.Fatalf("candidate agent=%s want always-on agent %s", rows[0].AgentID, alwaysAgent.ID)
	}
}

type agentSandboxUpgradeHarness struct {
	db       *gorm.DB
	encKey   *crypto.SymmetricKey
	runtime  *agentSandboxUpgradeRuntime
	provider *agentSandboxUpgradeProvider
	handler  *AgentSandboxUpgradeHandler
}

func newAgentSandboxUpgradeHarness(t *testing.T) *agentSandboxUpgradeHarness {
	t.Helper()
	db := connectTestDB(t)
	encKey, err := crypto.NewSymmetricKey("AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=")
	if err != nil {
		t.Fatalf("create enc key: %v", err)
	}
	runtime := newAgentSandboxUpgradeRuntime(t)
	cfg := &config.Config{
		SandboxProviderID:        sandbox.ProviderMicrosandbox,
		SandboxesRuntimeImageTag: "upgrade-test-amd64",
		APIWebhookBaseURL:        "https://api.example.test",
		ProxyHost:                "https://proxy.example.test",
		MCPBaseURL:               "https://mcp.example.test",
		JWTSigningKey:            "upgrade-test-signing-key",
	}
	provider := &agentSandboxUpgradeProvider{endpoint: runtime.server.URL}
	orchestrator := sandbox.NewOrchestrator(db, provider, encKey, cfg)
	compileDeps := agentruntime.CompileDeps{
		DB:         db,
		EncKey:     encKey,
		SigningKey: []byte("upgrade-test-signing-key"),
		Cfg:        cfg,
	}
	return &agentSandboxUpgradeHarness{
		db:       db,
		encKey:   encKey,
		runtime:  runtime,
		provider: provider,
		handler:  NewAgentSandboxUpgradeHandler(db, orchestrator, compileDeps, &enqueue.MockClient{}),
	}
}

type agentSandboxUpgradeRuntime struct {
	server                  *httptest.Server
	mu                      sync.Mutex
	configCalls             int
	drainPOSTs              int
	drainGETs               int
	cancelPOSTs             int
	healthzCalls            int
	readyzCalls             int
	failDrainPOSTsRemaining int
}

func newAgentSandboxUpgradeRuntime(t *testing.T) *agentSandboxUpgradeRuntime {
	t.Helper()
	rt := &agentSandboxUpgradeRuntime{}
	rt.server = httptest.NewServer(http.HandlerFunc(rt.handle))
	t.Cleanup(rt.server.Close)
	return rt
}

func (rt *agentSandboxUpgradeRuntime) handle(w http.ResponseWriter, r *http.Request) {
	rt.mu.Lock()
	failDrainPOST := false
	switch {
	case r.Method == http.MethodGet && r.URL.Path == "/healthz":
		rt.healthzCalls++
	case r.Method == http.MethodGet && r.URL.Path == "/readyz":
		rt.readyzCalls++
	case r.Method == http.MethodPut && r.URL.Path == "/config":
		rt.configCalls++
	case r.Method == http.MethodPost && r.URL.Path == "/control/drain":
		rt.drainPOSTs++
		if rt.failDrainPOSTsRemaining > 0 {
			rt.failDrainPOSTsRemaining--
			failDrainPOST = true
		}
	case r.Method == http.MethodGet && r.URL.Path == "/control/drain":
		rt.drainGETs++
	case r.Method == http.MethodPost && r.URL.Path == "/control/drain/cancel":
		rt.cancelPOSTs++
	default:
		rt.mu.Unlock()
		http.NotFound(w, r)
		return
	}
	rt.mu.Unlock()

	if failDrainPOST {
		http.Error(w, "runtime unavailable", http.StatusServiceUnavailable)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	switch r.URL.Path {
	case "/config":
		_ = json.NewEncoder(w).Encode(map[string]any{"env_key_count": 1})
	case "/control/drain", "/control/drain/cancel":
		_ = json.NewEncoder(w).Encode(map[string]any{
			"status":                    "drained",
			"draining":                  true,
			"complete":                  true,
			"active_turns":              0,
			"pending_accepted_messages": 0,
			"pending_outbox_events":     0,
		})
	default:
		_ = json.NewEncoder(w).Encode(map[string]any{"status": "ok"})
	}
}

func (rt *agentSandboxUpgradeRuntime) failNextDrainPOSTs(n int) {
	rt.mu.Lock()
	defer rt.mu.Unlock()
	rt.failDrainPOSTsRemaining = n
}

func (rt *agentSandboxUpgradeRuntime) assertDrainCalls(t *testing.T, wantPOSTs, wantGETs int) {
	t.Helper()
	rt.mu.Lock()
	defer rt.mu.Unlock()
	if rt.drainPOSTs != wantPOSTs || rt.drainGETs != wantGETs {
		t.Fatalf("drain calls post=%d get=%d want post=%d get=%d", rt.drainPOSTs, rt.drainGETs, wantPOSTs, wantGETs)
	}
}

type agentSandboxUpgradeProvider struct {
	endpoint string
	created  []sandbox.CreateSandboxOpts
	stopped  []string
	deleted  []string
}

func (p *agentSandboxUpgradeProvider) ID() string { return sandbox.ProviderMicrosandbox }

func (p *agentSandboxUpgradeProvider) Validate(context.Context) error { return nil }

func (p *agentSandboxUpgradeProvider) RuntimeLayout() sandbox.RuntimeLayout {
	return sandbox.RuntimeLayout{AgentRepoDir: "/workspace/repos", WorkspaceRepoDir: "/workspace/repos"}
}

func (p *agentSandboxUpgradeProvider) CreateSandbox(_ context.Context, opts sandbox.CreateSandboxOpts) (*sandbox.SandboxInfo, error) {
	p.created = append(p.created, opts)
	return &sandbox.SandboxInfo{ExternalID: fmt.Sprintf("upgrade-sandbox-%d", len(p.created)), Status: sandbox.StatusRunning}, nil
}

func (p *agentSandboxUpgradeProvider) StartSandbox(context.Context, string) error { return nil }

func (p *agentSandboxUpgradeProvider) StopSandbox(_ context.Context, externalID string) error {
	p.stopped = append(p.stopped, externalID)
	return nil
}

func (p *agentSandboxUpgradeProvider) ArchiveSandbox(context.Context, string) error { return nil }

func (p *agentSandboxUpgradeProvider) DeleteSandbox(_ context.Context, externalID string) error {
	p.deleted = append(p.deleted, externalID)
	return nil
}

func (p *agentSandboxUpgradeProvider) GetStatus(context.Context, string) (sandbox.SandboxStatus, error) {
	return sandbox.StatusRunning, nil
}

func (p *agentSandboxUpgradeProvider) GetEndpoint(context.Context, string, int) (string, error) {
	return p.endpoint, nil
}

func (p *agentSandboxUpgradeProvider) BuildTemplate(context.Context, sandbox.TemplateBuildRequest) (string, error) {
	return "", nil
}

func (p *agentSandboxUpgradeProvider) BuildTemplateWithLogs(context.Context, sandbox.TemplateBuildRequest, func(string)) (string, error) {
	return "", nil
}

func (p *agentSandboxUpgradeProvider) GetTemplateStatus(context.Context, string) (*sandbox.TemplateBuildStatus, error) {
	return &sandbox.TemplateBuildStatus{State: "ready"}, nil
}

func (p *agentSandboxUpgradeProvider) GetTemplateLogs(context.Context, string) (string, error) {
	return "", nil
}

func (p *agentSandboxUpgradeProvider) DeleteTemplate(context.Context, string) error { return nil }

func (p *agentSandboxUpgradeProvider) SetAutoStop(context.Context, string, int) error { return nil }

func (p *agentSandboxUpgradeProvider) SetAutoArchive(context.Context, string, int) error {
	return nil
}

func (p *agentSandboxUpgradeProvider) ExecuteCommand(context.Context, string, string) (string, error) {
	return "", nil
}

func (p *agentSandboxUpgradeProvider) ExecuteCommandWithTimeout(context.Context, string, string, time.Duration) (string, error) {
	return "", nil
}

func (p *agentSandboxUpgradeProvider) GetResourceUsage(context.Context, string) (*sandbox.ResourceUsage, error) {
	return &sandbox.ResourceUsage{}, nil
}

func seedAgentSandboxUpgradeFixture(t *testing.T, db *gorm.DB, strategy string) (model.Org, model.Agent, model.Channel) {
	t.Helper()
	org := model.Org{Name: "upgrade-org-" + uuid.NewString()[:8], Active: true}
	if err := db.Create(&org).Error; err != nil {
		t.Fatalf("create org: %v", err)
	}
	seedAgentSandboxUpgradeCredential(t, db, org.ID)
	agent := model.Agent{
		OrgID:           &org.ID,
		Name:            "upgrade-agent-" + uuid.NewString()[:8],
		SandboxStrategy: strategy,
		SandboxSize:     model.DefaultAgentSandboxSize,
		Model:           "gpt-4o-mini",
		AvailableModels: []string{"gpt-4o-mini"},
		Tools:           model.JSON{},
		McpServers:      model.RawJSON("[]"),
		Skills:          model.JSON{},
		RuntimeConfig:   model.JSON{},
		Permissions:     model.JSON{},
		Resources:       model.JSON{},
		Status:          "active",
	}
	if err := db.Create(&agent).Error; err != nil {
		t.Fatalf("create agent: %v", err)
	}
	channel := model.Channel{
		OrgID:          org.ID,
		Name:           "upgrade-channel-" + uuid.NewString()[:8],
		Kind:           "standard",
		Visibility:     "public",
		DefaultAgentID: agent.ID,
		Origin:         "native",
	}
	if err := db.Create(&channel).Error; err != nil {
		t.Fatalf("create channel: %v", err)
	}
	t.Cleanup(func() {
		_ = db.Where("org_id = ?", org.ID).Delete(&model.Session{}).Error
		_ = db.Where("org_id = ?", org.ID).Delete(&model.Sandbox{}).Error
		_ = db.Where("org_id = ?", org.ID).Delete(&model.AgentSandboxUpgrade{}).Error
		_ = db.Where("org_id = ?", org.ID).Delete(&model.Channel{}).Error
		_ = db.Where("org_id = ?", org.ID).Delete(&model.Agent{}).Error
		_ = db.Where("org_id = ?", org.ID).Delete(&model.Credential{}).Error
		_ = db.Where("id = ?", org.ID).Delete(&model.Org{}).Error
	})
	return org, agent, channel
}

func seedAgentSandboxUpgradeCredential(t *testing.T, db *gorm.DB, orgID uuid.UUID) {
	t.Helper()
	cred := model.Credential{
		OrgID:        &orgID,
		Label:        "upgrade-openai-" + uuid.NewString()[:8],
		BaseURL:      "https://openai.example.test/v1",
		AuthScheme:   "bearer",
		EncryptedKey: []byte("enc"),
		WrappedDEK:   []byte("dek"),
		ProviderID:   "openai",
	}
	if err := db.Create(&cred).Error; err != nil {
		t.Fatalf("create credential: %v", err)
	}
}

func seedAgentSandboxUpgradeSandbox(t *testing.T, h *agentSandboxUpgradeHarness, orgID, agentID uuid.UUID, externalID string) model.Sandbox {
	t.Helper()
	encrypted, err := h.encKey.EncryptString("runtime-secret-" + uuid.NewString())
	if err != nil {
		t.Fatalf("encrypt runtime secret: %v", err)
	}
	sb := model.Sandbox{
		OrgID:                  &orgID,
		AgentID:                &agentID,
		ProviderID:             sandbox.ProviderMicrosandbox,
		ExternalID:             externalID,
		RuntimeURL:             h.runtime.server.URL,
		EncryptedRuntimeSecret: encrypted,
		Status:                 string(sandbox.StatusRunning),
	}
	if err := h.db.Create(&sb).Error; err != nil {
		t.Fatalf("create sandbox: %v", err)
	}
	return sb
}

func seedAgentSandboxUpgradeSession(t *testing.T, db *gorm.DB, orgID, channelID, agentID, sandboxID uuid.UUID, turnStatus string) model.Session {
	t.Helper()
	session := model.Session{
		OrgID:             orgID,
		ChannelID:         channelID,
		AgentID:           agentID,
		SandboxID:         &sandboxID,
		Model:             "gpt-4o-mini",
		AccessMode:        "full",
		ReasoningEffort:   "low",
		Source:            "web",
		SourceResourceKey: uuid.NewString(),
		Name:              "upgrade-session",
		Status:            "active",
		AgentTurnStatus:   turnStatus,
		IntegrationScopes: model.JSON{},
	}
	if err := db.Create(&session).Error; err != nil {
		t.Fatalf("create session: %v", err)
	}
	return session
}

func seedAgentSandboxUpgradeRow(t *testing.T, db *gorm.DB, orgID, agentID, oldSandboxID uuid.UUID) model.AgentSandboxUpgrade {
	t.Helper()
	upgrade := model.AgentSandboxUpgrade{
		OrgID:        orgID,
		AgentID:      agentID,
		OldSandboxID: &oldSandboxID,
		Status:       model.AgentSandboxUpgradeStatusQueued,
		Phase:        model.AgentSandboxUpgradePhaseQueued,
	}
	if err := db.Create(&upgrade).Error; err != nil {
		t.Fatalf("create upgrade: %v", err)
	}
	return upgrade
}

func seedAgentSandboxAutoUpgradeCandidate(t *testing.T, db *gorm.DB, orgID, agentID uuid.UUID, suffix string) model.Sandbox {
	t.Helper()
	snapshotID := "old-runtime-image-" + suffix
	sb := model.Sandbox{
		OrgID:                  &orgID,
		AgentID:                &agentID,
		SnapshotID:             &snapshotID,
		ProviderID:             sandbox.ProviderMicrosandbox,
		ExternalID:             "auto-upgrade-" + suffix + "-" + uuid.NewString()[:8],
		RuntimeURL:             "http://runtime-" + suffix + ".example.test",
		EncryptedRuntimeSecret: []byte("encrypted"),
		Status:                 string(sandbox.StatusRunning),
	}
	if err := db.Create(&sb).Error; err != nil {
		t.Fatalf("create auto-upgrade sandbox: %v", err)
	}
	return sb
}

func agentSandboxUpgradeTask(t *testing.T, upgradeID, agentID uuid.UUID) *asynq.Task {
	t.Helper()
	task, _, err := NewAgentSandboxUpgradeTask(upgradeID, agentID)
	if err != nil {
		t.Fatalf("build task: %v", err)
	}
	return task
}

func assertAgentSandboxUpgradeSucceeded(t *testing.T, db *gorm.DB, upgradeID uuid.UUID) {
	t.Helper()
	var stored model.AgentSandboxUpgrade
	if err := db.First(&stored, "id = ?", upgradeID).Error; err != nil {
		t.Fatalf("load upgrade: %v", err)
	}
	if stored.Status != model.AgentSandboxUpgradeStatusSucceeded || stored.Phase != model.AgentSandboxUpgradePhaseCompleted {
		message := ""
		if stored.ErrorMessage != nil {
			message = *stored.ErrorMessage
		}
		t.Fatalf("upgrade status=%s phase=%s error=%q", stored.Status, stored.Phase, message)
	}
}

func assertAgentSandboxUpgradeActiveSandbox(t *testing.T, db *gorm.DB, orgID, agentID, oldSandboxID uuid.UUID) {
	t.Helper()
	var sandboxes []model.Sandbox
	if err := db.Where("org_id = ? AND agent_id = ?", orgID, agentID).Order("created_at ASC").Find(&sandboxes).Error; err != nil {
		t.Fatalf("load sandboxes: %v", err)
	}
	if len(sandboxes) != 2 {
		t.Fatalf("sandboxes=%d want old+new", len(sandboxes))
	}
	var running []model.Sandbox
	for _, sb := range sandboxes {
		if sb.Status == string(sandbox.StatusRunning) {
			running = append(running, sb)
		}
	}
	if len(running) != 1 {
		t.Fatalf("running sandboxes=%d all=%+v", len(running), sandboxes)
	}
	if running[0].ID == oldSandboxID {
		t.Fatalf("old sandbox is still active: %+v", running[0])
	}
	if !strings.HasPrefix(running[0].ExternalID, "upgrade-sandbox-") {
		t.Fatalf("new running sandbox external_id=%q", running[0].ExternalID)
	}
}
