package tasks

import (
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/hibiken/asynq"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/model"
	"github.com/usehivy/hivy/internal/sandbox"
)

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

func assertAgentSandboxUpgradeInPlaceSandbox(t *testing.T, db *gorm.DB, orgID, agentID uuid.UUID, oldSandbox model.Sandbox) {
	t.Helper()
	var sandboxes []model.Sandbox
	if err := db.Where("org_id = ? AND agent_id = ?", orgID, agentID).Order("created_at ASC").Find(&sandboxes).Error; err != nil {
		t.Fatalf("load sandboxes: %v", err)
	}
	if len(sandboxes) != 1 {
		t.Fatalf("sandboxes=%d want existing sandbox only", len(sandboxes))
	}
	got := sandboxes[0]
	if got.ID != oldSandbox.ID {
		t.Fatalf("sandbox id=%s want %s", got.ID, oldSandbox.ID)
	}
	if got.ExternalID != oldSandbox.ExternalID {
		t.Fatalf("external id=%q want %q", got.ExternalID, oldSandbox.ExternalID)
	}
	if got.RuntimeURL != oldSandbox.RuntimeURL {
		t.Fatalf("runtime url=%q want %q", got.RuntimeURL, oldSandbox.RuntimeURL)
	}
	if got.Status != string(sandbox.StatusRunning) {
		t.Fatalf("status=%q want running", got.Status)
	}
}
