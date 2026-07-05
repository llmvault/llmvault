package handler

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"

	"gorm.io/driver/postgres"

	"github.com/usehivy/hivy/internal/model"
	"github.com/usehivy/hivy/internal/runtimestream"
	"github.com/usehivy/hivy/internal/testdb"
)

func subagentDetailEvent(parentSessionID, childSessionID uuid.UUID, jobID string) runtimestream.Event {
	return runtimestream.Event{
		SessionID:  parentSessionID.String(),
		RuntimeSeq: 42,
		EventID:    "evt-subagent-42",
		EventType:  "tool_call_completed",
		Durability: runtimestream.DurabilityDurable,
		TurnID:     "turn-1",
		Payload: map[string]any{
			"scope": "subagent",
			"text":  "child produced output",
			"subagent": map[string]any{
				"job_id":            jobID,
				"agent_name":        "researcher",
				"parent_session_id": parentSessionID.String(),
				"child_session_id":  childSessionID.String(),
			},
		},
		OccurredAt: time.Date(2026, 7, 5, 12, 0, 0, 0, time.UTC),
	}
}

// TestEnrichRuntimeEventKeepsSubagentDetailParentScoped confirms the ingress path
// does not special-case, reject, or reroute scope="subagent" detail events: with a
// bound (parent) session the event stays under the parent id and its scope/subagent
// payload survives enrichment + Validate().
func TestEnrichRuntimeEventKeepsSubagentDetailParentScoped(t *testing.T) {
	ctx := context.Background()
	orgID := uuid.New()
	agentID := uuid.New()
	sandboxID := uuid.New()
	parentSessionID := uuid.New()
	childSessionID := uuid.New()
	jobID := "job-" + uuid.NewString()[:8]

	h := &RuntimeStreamIngressHandler{}
	sb := &model.Sandbox{ID: sandboxID, OrgID: &orgID, AgentID: &agentID}
	boundSession := &model.Session{ID: parentSessionID, OrgID: orgID, AgentID: agentID, SandboxID: &sandboxID}

	event := subagentDetailEvent(parentSessionID, childSessionID, jobID)
	accepted, err := h.enrichRuntimeEvent(ctx, sb, boundSession, &event)
	if err != nil {
		t.Fatalf("enrichRuntimeEvent rejected a durable subagent detail event: %v", err)
	}
	if accepted.SessionID != parentSessionID.String() {
		t.Fatalf("accepted session_id = %s, want parent %s (must not reroute to child)", accepted.SessionID, parentSessionID)
	}
	if accepted.OrgID != orgID.String() || accepted.AgentID != agentID.String() {
		t.Fatalf("accepted org/agent = %s/%s, want %s/%s", accepted.OrgID, accepted.AgentID, orgID, agentID)
	}
	if accepted.Durability != runtimestream.DurabilityDurable {
		t.Fatalf("accepted durability = %q, want durable", accepted.Durability)
	}
	if scope, _ := accepted.Payload["scope"].(string); scope != "subagent" {
		t.Fatalf("accepted payload.scope = %v, want subagent", accepted.Payload["scope"])
	}
	sub, ok := accepted.Payload["subagent"].(map[string]any)
	if !ok {
		t.Fatalf("accepted payload.subagent missing: %#v", accepted.Payload["subagent"])
	}
	if got, _ := sub["job_id"].(string); got != jobID {
		t.Fatalf("accepted payload.subagent.job_id = %v, want %s", sub["job_id"], jobID)
	}
	if got, _ := sub["child_session_id"].(string); got != childSessionID.String() {
		t.Fatalf("accepted payload.subagent.child_session_id = %v, want %s", sub["child_session_id"], childSessionID)
	}
}

// TestEnrichRuntimeEventResolvesParentSessionByDBLookup exercises the HandleWS
// path (no bound session): the durable subagent detail event arrives with
// SessionID = the PARENT session, so the org/agent-scoped lookup resolves it and
// the "session not found for sandbox" error must NOT fire.
func TestEnrichRuntimeEventResolvesParentSessionByDBLookup(t *testing.T) {
	ctx := context.Background()
	db := connectRuntimeIngressTestDB(t)

	orgID := uuid.New()
	agentID := uuid.New()
	sandboxID := uuid.New()
	channelID := uuid.New()
	parentSessionID := uuid.New()
	childSessionID := uuid.New()
	jobID := "job-" + uuid.NewString()[:8]

	seedRuntimeIngressParent(t, db, orgID, agentID, channelID, parentSessionID)

	h := &RuntimeStreamIngressHandler{db: db}
	sb := &model.Sandbox{ID: sandboxID, OrgID: &orgID, AgentID: &agentID}

	event := subagentDetailEvent(parentSessionID, childSessionID, jobID)
	accepted, err := h.enrichRuntimeEvent(ctx, sb, nil, &event)
	if err != nil {
		t.Fatalf("enrichRuntimeEvent failed to resolve parent-scoped subagent event: %v", err)
	}
	if accepted.SessionID != parentSessionID.String() {
		t.Fatalf("accepted session_id = %s, want parent %s", accepted.SessionID, parentSessionID)
	}
	if accepted.OrgID != orgID.String() || accepted.AgentID != agentID.String() {
		t.Fatalf("accepted org/agent = %s/%s, want %s/%s", accepted.OrgID, accepted.AgentID, orgID, agentID)
	}
	if scope, _ := accepted.Payload["scope"].(string); scope != "subagent" {
		t.Fatalf("accepted payload.scope = %v, want subagent", accepted.Payload["scope"])
	}
}

func connectRuntimeIngressTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	dsn := testdb.DatabaseURL("DATABASE_URL", "HIVY_DATABASE_URL", "TEST_DATABASE_URL")
	database, err := gorm.Open(postgres.Open(dsn), &gorm.Config{Logger: gormlogger.Discard})
	if err != nil {
		t.Fatalf("connect postgres: %v", err)
	}
	sqlDB, _ := database.DB()
	sqlDB.SetMaxOpenConns(3)
	sqlDB.SetMaxIdleConns(1)
	testdb.ApplyMigrations(t, database)
	t.Cleanup(func() { sqlDB.Close() })
	return database
}

func seedRuntimeIngressParent(t *testing.T, db *gorm.DB, orgID, agentID, channelID, sessionID uuid.UUID) {
	t.Helper()
	if err := db.Create(&model.Org{ID: orgID, Name: "ingress-org-" + orgID.String()[:8], Active: true}).Error; err != nil {
		t.Fatalf("create org: %v", err)
	}
	if err := db.Create(&model.Agent{
		ID:            agentID,
		OrgID:         &orgID,
		Name:          "Agent-" + agentID.String()[:8],
		Model:         "test-model",
		Tools:         model.JSON{},
		McpServers:    model.RawJSON("[]"),
		Skills:        model.JSON{},
		RuntimeConfig: model.JSON{},
		Permissions:   model.JSON{},
		Resources:     model.JSON{},
		Status:        "active",
	}).Error; err != nil {
		t.Fatalf("create agent: %v", err)
	}
	if err := db.Create(&model.Channel{
		ID:             channelID,
		OrgID:          orgID,
		Name:           "chan-" + channelID.String()[:8],
		Kind:           "standard",
		Visibility:     "public",
		DefaultAgentID: agentID,
		Origin:         "native",
	}).Error; err != nil {
		t.Fatalf("create channel: %v", err)
	}
	if err := db.Create(&model.Session{
		ID:        sessionID,
		OrgID:     orgID,
		ChannelID: channelID,
		AgentID:   agentID,
		Model:     "test-model",
		Source:    "web",
		Name:      "parent session",
		Status:    "active",
	}).Error; err != nil {
		t.Fatalf("create parent session: %v", err)
	}
	t.Cleanup(func() {
		db.Where("session_id = ?", sessionID).Delete(&model.SessionEvent{})
		db.Where("id = ?", sessionID).Delete(&model.Session{})
		db.Where("id = ?", channelID).Delete(&model.Channel{})
		db.Where("id = ?", agentID).Delete(&model.Agent{})
		db.Where("id = ?", orgID).Delete(&model.Org{})
	})
}
