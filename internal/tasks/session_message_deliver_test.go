package tasks

import (
	"errors"
	"testing"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/model"
)

func TestRuntimeMessageFromEventRendersStructuredAttachmentsAndComments(t *testing.T) {
	sessionID := uuid.New()
	msg := runtimeMessageFromEvent(
		model.Session{ID: sessionID},
		model.SessionEvent{
			Payload: model.JSON{
				"text": "Please review this",
				"attachments": []any{
					map[string]any{
						"filename":             "screen.png",
						"asset_url":            "https://api.example.test/assets/screen.png",
						"content_type":         "image/png",
						"rendered_description": "Primary category: Product UI",
					},
				},
				"code_line_comments": []any{
					map[string]any{
						"path":         "apps/web/lib/diffs-theme.ts",
						"display_path": "apps/web/lib/diffs-theme.ts",
						"line_number":  float64(148),
						"side":         "additions",
						"body":         "Use the HeroUI token here.",
					},
				},
			},
		},
		nil,
	)

	want := `Please review this

<attachment name="screen.png" url="https://api.example.test/assets/screen.png" mime_type="image/png">
<description>
Primary category: Product UI
</description>
</attachment>

Code comments to address:

1. apps/web/lib/diffs-theme.ts:R148
   Use the HeroUI token here.`
	if msg.Text != want {
		t.Fatalf("runtime text mismatch\nwant:\n%s\n\ngot:\n%s", want, msg.Text)
	}
	if msg.Raw["text"] != "Please review this" {
		t.Fatalf("raw text was not preserved: %#v", msg.Raw["text"])
	}
	if len(msg.Attachments) != 1 {
		t.Fatalf("attachments len=%d, want 1", len(msg.Attachments))
	}
}

func TestLoadRuntimeSandboxDoesNotReuseAgentRuntimeForPerSessionWithoutAttachedSandbox(t *testing.T) {
	db := connectTestDB(t)
	org, agent, channel := seedSessionRuntimeSelectionFixture(t, db, "per_session")
	existing := seedSessionRuntimeSelectionSandbox(t, db, org.ID, agent.ID)
	session := seedSessionRuntimeSelectionSession(t, db, org.ID, channel.ID, agent.ID, nil)

	handler := &SessionMessageDeliverHandler{db: db}
	loaded, err := handler.loadRuntimeSandbox(t.Context(), session, &agent)

	if !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatalf("loadRuntimeSandbox error = %v, want record not found", err)
	}
	if loaded != nil {
		t.Fatalf("loadRuntimeSandbox returned sandbox %s, want nil", loaded.ID)
	}

	attached := seedSessionRuntimeSelectionSession(t, db, org.ID, channel.ID, agent.ID, &existing.ID)
	loaded, err = handler.loadRuntimeSandbox(t.Context(), attached, &agent)
	if err != nil {
		t.Fatalf("load attached per-session sandbox: %v", err)
	}
	if loaded.ID != existing.ID {
		t.Fatalf("loaded sandbox = %s, want attached sandbox %s", loaded.ID, existing.ID)
	}
}

func TestLoadRuntimeSandboxReusesAgentRuntimeForAlwaysOn(t *testing.T) {
	db := connectTestDB(t)
	org, agent, channel := seedSessionRuntimeSelectionFixture(t, db, "always_on")
	existing := seedSessionRuntimeSelectionSandbox(t, db, org.ID, agent.ID)
	session := seedSessionRuntimeSelectionSession(t, db, org.ID, channel.ID, agent.ID, nil)

	handler := &SessionMessageDeliverHandler{db: db}
	loaded, err := handler.loadRuntimeSandbox(t.Context(), session, &agent)
	if err != nil {
		t.Fatalf("load always-on sandbox: %v", err)
	}
	if loaded.ID != existing.ID {
		t.Fatalf("loaded sandbox = %s, want always-on sandbox %s", loaded.ID, existing.ID)
	}
}

func seedSessionRuntimeSelectionFixture(t *testing.T, db *gorm.DB, strategy string) (model.Org, model.Agent, model.Channel) {
	t.Helper()
	org := model.Org{Name: "session-runtime-" + uuid.NewString()[:8], Active: true}
	if err := db.Create(&org).Error; err != nil {
		t.Fatalf("create org: %v", err)
	}
	agent := model.Agent{
		OrgID:           &org.ID,
		Name:            "runtime-agent-" + uuid.NewString()[:8],
		SandboxStrategy: strategy,
		Model:           "test-model",
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
		Name:           "runtime-channel-" + uuid.NewString()[:8],
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
		_ = db.Where("org_id = ?", org.ID).Delete(&model.Channel{}).Error
		_ = db.Where("org_id = ?", org.ID).Delete(&model.Agent{}).Error
		_ = db.Where("id = ?", org.ID).Delete(&model.Org{}).Error
	})
	return org, agent, channel
}

func seedSessionRuntimeSelectionSandbox(t *testing.T, db *gorm.DB, orgID, agentID uuid.UUID) model.Sandbox {
	t.Helper()
	sandbox := model.Sandbox{
		OrgID:                  &orgID,
		AgentID:                &agentID,
		ProviderID:             "test",
		ExternalID:             "external-" + uuid.NewString()[:8],
		RuntimeURL:             "http://runtime-" + uuid.NewString()[:8] + ".test",
		EncryptedRuntimeSecret: []byte("encrypted"),
		Status:                 "running",
	}
	if err := db.Create(&sandbox).Error; err != nil {
		t.Fatalf("create sandbox: %v", err)
	}
	return sandbox
}

func seedSessionRuntimeSelectionSession(t *testing.T, db *gorm.DB, orgID, channelID, agentID uuid.UUID, sandboxID *uuid.UUID) model.Session {
	t.Helper()
	session := model.Session{
		OrgID:             orgID,
		ChannelID:         channelID,
		AgentID:           agentID,
		SandboxID:         sandboxID,
		Model:             "test-model",
		AccessMode:        "full",
		ReasoningEffort:   "high",
		Source:            "web",
		SourceResourceKey: uuid.NewString(),
		Name:              "runtime-selection",
		Status:            "active",
		AgentTurnStatus:   model.SessionAgentTurnIdle,
		IntegrationScopes: model.JSON{},
	}
	if err := db.Create(&session).Error; err != nil {
		t.Fatalf("create session: %v", err)
	}
	return session
}
