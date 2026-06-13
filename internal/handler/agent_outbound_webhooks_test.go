package handler

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/model"
	skillpkg "github.com/usehivy/hivy/internal/skills"
	"github.com/usehivy/hivy/internal/testdb"
)

func TestAgentOutboundMemoryCheckpoints(t *testing.T) {
	if !shouldTriggerAgentMemoryCheckpoint("session.created") {
		t.Fatal("session creation should schedule delayed retain")
	}
	for _, eventType := range []string{"user.message.received", "agent.message.sent", "agent.stream.token", "error.model"} {
		if shouldTriggerAgentMemoryCheckpoint(eventType) {
			t.Fatalf("%s should not directly trigger retain", eventType)
		}
	}
}

func TestShouldStoreAgentSessionEvent_KeepsConversationTimeline(t *testing.T) {
	for _, eventType := range []string{
		"user.message.received",
		"agent.stream.token",
		"agent.stream.thinking",
		"agent.tool.call",
		"agent.tool.result",
		"agent.message.sent",
		"error.model",
		"skill.synced",
	} {
		if !shouldStoreAgentSessionEvent(eventType) {
			t.Fatalf("%s should be stored", eventType)
		}
	}
	for _, eventType := range []string{
		"session.created",
		"tool.invoked",
		"agent.final_message",
		"agent.run.turn.started",
		"agent.run.model.request.started",
		"agent.run.model.usage",
		"agent.run.turn.completed",
	} {
		if shouldStoreAgentSessionEvent(eventType) {
			t.Fatalf("%s should be skipped", eventType)
		}
	}
}

func TestAgentSessionEventFromOutbound_StoresTimelineEventTypes(t *testing.T) {
	orgID := uuid.New()
	agentID := uuid.New()
	sandboxID := uuid.New()
	agentSessionID := uuid.New()
	eventAt := time.Date(2026, 5, 13, 9, 30, 0, 0, time.UTC)
	sb := &model.Sandbox{ID: sandboxID, OrgID: &orgID, AgentID: &agentID}
	for _, eventType := range []string{
		"user.message.received",
		"agent.stream.token",
		"agent.stream.thinking",
		"agent.tool.call",
		"agent.tool.result",
		"agent.message.sent",
		"error.model",
	} {
		payload := map[string]any{
			"session_id": "slack-session-1",
			"source":     "slack",
			"text":       "api_key=sk-secret should still be persisted for session sync",
		}
		body, err := json.Marshal(payload)
		if err != nil {
			t.Fatalf("marshal payload: %v", err)
		}
		stored, ok := agentSessionEventFromOutbound(sb, &agentOutboundEvent{
			EventType: eventType,
			Payload:   body,
			At:        eventAt,
		}, payload, agentSessionID, "slack-session-1")
		if !ok {
			t.Fatalf("%s was not stored", eventType)
		}
		if stored.EventType != eventType || stored.AgentSessionID != agentSessionID || stored.SessionID != "slack-session-1" || stored.Source != "slack" {
			t.Fatalf("stored event mismatch: %#v", stored)
		}
	}
}

func TestAgentSessionEventFromOutbound_StoresEventWithoutSessionID(t *testing.T) {
	orgID := uuid.New()
	agentID := uuid.New()
	sandboxID := uuid.New()
	sb := &model.Sandbox{ID: sandboxID, OrgID: &orgID, AgentID: &agentID}
	payload := map[string]any{"source": "system"}
	body, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}
	stored, ok := agentSessionEventFromOutbound(sb, &agentOutboundEvent{
		EventType: "config.applied",
		Payload:   body,
		At:        time.Now().UTC(),
	}, payload, uuid.New(), "")
	if !ok {
		t.Fatal("event without session id should still be stored")
	}
	if stored.SessionID != "" || stored.EventType != "config.applied" {
		t.Fatalf("stored event mismatch: %#v", stored)
	}
}

func TestAgentEventSource_SanitizesFutureGateways(t *testing.T) {
	source := agentEventSource(map[string]any{"source": "WhatsApp Business"})
	if source != "whatsapp-business" {
		t.Fatalf("source = %q", source)
	}
	if agentEventSource(map[string]any{}) != "manual" {
		t.Fatal("missing source should fall back to manual")
	}
}

func TestShouldDeliverGatewayRuntimeFinal_UsesSessionSourceAndSkipsDirectSlackGateway(t *testing.T) {
	gatewaySession := &model.AgentSession{Source: "gateway"}
	if !shouldDeliverGatewayRuntimeFinal(gatewaySession, map[string]any{"source": "cron", "text": "wake result"}) {
		t.Fatal("gateway-backed resumed turn should be delivered")
	}
	if shouldDeliverGatewayRuntimeFinal(gatewaySession, map[string]any{"source": "gateway", "provider": "slack"}) {
		t.Fatal("direct Slack gateway turn should remain handled by the stream worker")
	}
	if shouldDeliverGatewayRuntimeFinal(&model.AgentSession{Source: "cron"}, map[string]any{"text": "wake result"}) {
		t.Fatal("non-gateway session should not deliver through gateway adapter")
	}
}

func TestPayloadLooksSensitive(t *testing.T) {
	if !payloadLooksSensitive(map[string]any{"text": "api_key=sk-secret"}) {
		t.Fatal("expected secret-looking payload to be rejected")
	}
	if payloadLooksSensitive(map[string]any{"text": "The team requires rollback notes."}) {
		t.Fatal("ordinary payload should not be rejected")
	}
}

func TestIntegration_AgentSkillSync_UpsertsSkillAndAttachesAgent(t *testing.T) {
	db := connectAgentSkillSyncTestDB(t)
	org1 := model.Org{Name: "skill-sync-" + uuid.NewString()}
	org2 := model.Org{Name: "skill-sync-" + uuid.NewString()}
	if err := db.Create(&org1).Error; err != nil {
		t.Fatalf("create org1: %v", err)
	}
	if err := db.Create(&org2).Error; err != nil {
		t.Fatalf("create org2: %v", err)
	}
	t.Cleanup(func() {
		db.Where("id IN ?", []uuid.UUID{org1.ID, org2.ID}).Delete(&model.Org{})
	})
	agent1 := model.Agent{OrgID: &org1.ID, Name: "Aria", Model: "test"}
	agent2 := model.Agent{OrgID: &org2.ID, Name: "Aria", Model: "test"}
	if err := db.Create(&agent1).Error; err != nil {
		t.Fatalf("create agent1: %v", err)
	}
	if err := db.Create(&agent2).Error; err != nil {
		t.Fatalf("create agent2: %v", err)
	}
	global := model.Skill{Slug: "debug-deploys", Name: "debug-deploys", SourceType: model.SkillSourceInline, RepoRef: "main", Status: model.SkillStatusPublished}
	if err := db.Create(&global).Error; err != nil {
		t.Fatalf("create global skill with same slug: %v", err)
	}
	t.Cleanup(func() {
		db.Unscoped().Delete(&global)
	})

	h := NewAgentOutboundWebhookHandler(db, nil, nil)
	sb1 := &model.Sandbox{ID: uuid.New(), OrgID: &org1.ID, AgentID: &agent1.ID}
	sb2 := &model.Sandbox{ID: uuid.New(), OrgID: &org2.ID, AgentID: &agent2.ID}
	payload := map[string]any{
		"action":      "create",
		"name":        "debug-deploys",
		"description": "Debug deploy failures.",
		"tags":        []string{"deploy", "debug"},
		"content":     "---\nname: debug-deploys\ndescription: Debug deploy failures.\n---\n\n# Debug\nCheck logs first.",
		"files":       map[string]string{"references/errors.md": "# Errors"},
	}
	if err := h.syncSkillEvent(t.Context(), sb1, payload); err != nil {
		t.Fatalf("sync skill org1: %v", err)
	}
	var skill model.Skill
	if err := db.Where("org_id = ? AND slug = ?", org1.ID, "debug-deploys").First(&skill).Error; err != nil {
		t.Fatalf("load org skill: %v", err)
	}
	if skill.Status != model.SkillStatusPublished {
		t.Fatalf("status = %q", skill.Status)
	}
	var links int64
	db.Model(&model.AgentSkill{}).Where("agent_id = ? AND skill_id = ?", agent1.ID, skill.ID).Count(&links)
	if links != 1 {
		t.Fatalf("agent skill links = %d", links)
	}
	var bundle skillpkg.Bundle
	if err := json.Unmarshal(skill.Bundle, &bundle); err != nil {
		t.Fatalf("decode bundle: %v", err)
	}
	if bundle.Content != "\n# Debug\nCheck logs first." || bundle.Files["references/errors.md"] != "# Errors" {
		t.Fatalf("unexpected bundle: %#v", bundle)
	}

	payload["action"] = "patch"
	payload["content"] = "---\nname: debug-deploys\n---\n\n# Debug\nCheck deployment logs first."
	if err := h.syncSkillEvent(t.Context(), sb1, payload); err != nil {
		t.Fatalf("sync update: %v", err)
	}
	if err := db.First(&skill, "id = ?", skill.ID).Error; err != nil {
		t.Fatalf("reload updated skill: %v", err)
	}
	if !strings.Contains(string(skill.Bundle), "Check deployment logs first.") {
		t.Fatalf("bundle did not update: %s", string(skill.Bundle))
	}
	if err := h.syncSkillEvent(t.Context(), sb2, payload); err != nil {
		t.Fatalf("same slug in second org should be allowed: %v", err)
	}
	var org2Skill model.Skill
	if err := db.Where("org_id = ? AND slug = ?", org2.ID, "debug-deploys").First(&org2Skill).Error; err != nil {
		t.Fatalf("load org2 skill: %v", err)
	}
	if org2Skill.ID == skill.ID {
		t.Fatal("org2 skill reused org1 skill")
	}

	if err := h.syncSkillEvent(t.Context(), sb1, map[string]any{"action": "delete", "name": "debug-deploys", "deleted": true}); err != nil {
		t.Fatalf("sync delete: %v", err)
	}
	db.Model(&model.AgentSkill{}).Where("agent_id = ? AND skill_id = ?", agent1.ID, skill.ID).Count(&links)
	if links != 0 {
		t.Fatalf("agent link should be detached, got %d", links)
	}
	if err := db.First(&model.Skill{}, "id = ?", skill.ID).Error; err != nil {
		t.Fatalf("skill should remain after detach-only delete: %v", err)
	}
}

func connectAgentSkillSyncTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	dsn := testdb.DatabaseURL("DATABASE_URL", "HIVY_DATABASE_URL", "TEST_DATABASE_URL")
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("connect postgres: %v", err)
	}
	sqlDB, _ := db.DB()
	sqlDB.SetMaxOpenConns(3)
	sqlDB.SetMaxIdleConns(1)
	testdb.ApplyMigrations(t, db)
	t.Cleanup(func() { sqlDB.Close() })
	return db
}
