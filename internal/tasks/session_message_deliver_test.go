package tasks

import (
	"errors"
	"testing"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/model"
	"github.com/usehivy/hivy/internal/sandbox"
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
						"short_description":    "A UI screenshot.",
						"important_details":    "Shows the primary action.",
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
				"artifact_comments": []any{
					map[string]any{
						"artifact_name": "Homepage",
						"selector":      "hero",
						"body":          "Make the heading more concrete.",
					},
				},
			},
		},
	)

	want := `Please review this

<attachment name="screen.png" url="https://api.example.test/assets/screen.png" mime_type="image/png">
<important_details>
Shows the primary action.
</important_details>
<full_description>
Primary category: Product UI
</full_description>
<short_description>
A UI screenshot.
</short_description>
</attachment>

Code comments to address:

1. apps/web/lib/diffs-theme.ts:R148
   Use the HeroUI token here.

Artifact comments to address:

1. Homepage @ hero
   Make the heading more concrete.`
	if msg.Text != want {
		t.Fatalf("runtime text mismatch\nwant:\n%s\n\ngot:\n%s", want, msg.Text)
	}
	if len(msg.Attachments) != 0 {
		t.Fatalf("attachments len=%d, want 0 because attachments are compiled into text", len(msg.Attachments))
	}
}

func TestLoadRuntimeSandboxDoesNotReuseAgentRuntimeForSessionWithoutAttachedSandbox(t *testing.T) {
	db := connectTestDB(t)
	org, agent, channel := seedSessionRuntimeSelectionFixture(t, db, "session")
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
		t.Fatalf("load attached session sandbox: %v", err)
	}
	if loaded.ID != existing.ID {
		t.Fatalf("loaded sandbox = %s, want attached sandbox %s", loaded.ID, existing.ID)
	}
}

func TestClaimNextSkipsDrainingSessionSandbox(t *testing.T) {
	db := connectTestDB(t)
	org, agent, channel := seedSessionRuntimeSelectionFixture(t, db, "session")
	existing := seedSessionRuntimeSelectionSandbox(t, db, org.ID, agent.ID)
	if err := db.Model(&existing).Update("status", string(sandbox.StatusDraining)).Error; err != nil {
		t.Fatalf("mark sandbox draining: %v", err)
	}
	session := seedSessionRuntimeSelectionSession(t, db, org.ID, channel.ID, agent.ID, &existing.ID)
	queue := model.SessionMessageQueue{
		OrgID:          org.ID,
		SessionID:      session.ID,
		MessageText:    "hold until replacement",
		MessagePayload: model.JSON{"text": "hold until replacement"},
		SequenceNumber: 1,
		Status:         "pending",
	}
	if err := db.Create(&queue).Error; err != nil {
		t.Fatalf("create queued message: %v", err)
	}

	handler := &SessionMessageDeliverHandler{db: db}
	claimed, err := handler.claimNext(t.Context(), session.ID)
	if !errors.Is(err, ErrSessionRuntimeDraining) {
		t.Fatalf("claimNext error = %v, want ErrSessionRuntimeDraining", err)
	}
	if claimed != nil {
		t.Fatalf("claimNext returned row while draining: %+v", claimed)
	}
	var storedQueue model.SessionMessageQueue
	if err := db.First(&storedQueue, "id = ?", queue.ID).Error; err != nil {
		t.Fatalf("reload queue: %v", err)
	}
	if storedQueue.Status != "pending" || storedQueue.AttemptCount != 0 {
		t.Fatalf("queue mutated while draining: status=%s attempts=%d", storedQueue.Status, storedQueue.AttemptCount)
	}
	var storedSession model.Session
	if err := db.First(&storedSession, "id = ?", session.ID).Error; err != nil {
		t.Fatalf("reload session: %v", err)
	}
	if storedSession.AgentTurnStatus != model.SessionAgentTurnIdle {
		t.Fatalf("session turn status=%s, want idle", storedSession.AgentTurnStatus)
	}
}

func seedSessionRuntimeSelectionFixture(t *testing.T, db *gorm.DB, strategy string) (model.Org, model.Agent, model.Channel) {
	t.Helper()
	org := model.Org{Name: "session-runtime-" + uuid.NewString()[:8], Active: true}
	if err := db.Create(&org).Error; err != nil {
		t.Fatalf("create org: %v", err)
	}
	team := model.Team{OrgID: org.ID, Name: "session-runtime-team-" + uuid.NewString()[:8]}
	if err := db.Create(&team).Error; err != nil {
		t.Fatalf("create team: %v", err)
	}
	agent := model.Agent{
		OrgID:         &org.ID,
		TeamID:        team.ID,
		Name:          "runtime-agent-" + uuid.NewString()[:8],
		Model:         "test-model",
		Tools:         model.JSON{},
		McpServers:    model.RawJSON("[]"),
		Skills:        model.JSON{},
		RuntimeConfig: model.JSON{},
		Permissions:   model.JSON{},
		Resources:     model.JSON{},
		Status:        "active",
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
