package e2e

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/model"
)

func TestAgentSessionsReflectionE2E(t *testing.T) {
	if os.Getenv("HIVY_AGENT_SESSIONS_E2E") != "1" {
		t.Skip("set HIVY_AGENT_SESSIONS_E2E=1 to run against the live compose stack")
	}
	loadEnv(t)

	ctx, cancel := context.WithTimeout(context.Background(), 35*time.Minute)
	defer cancel()

	apiBase := agentSessionsBaseURL("HIVY_API_BASE_URL", "HIVY_COMPOSE_API_PORT", "8080")
	workerBase := agentSessionsBaseURL("HIVY_WORKER_BASE_URL", "HIVY_COMPOSE_WORKER_HEALTH_PORT", "8090")
	requireAgentSessionsHealthy(t, ctx, apiBase, "api")
	requireAgentSessionsHealthy(t, ctx, workerBase, "worker")
	agentSessionsEnsureSystemAtlasCloudCredential(t)

	runID := strings.ReplaceAll(uuid.NewString(), "-", "")[:12]
	ownerName := "Franz Keller " + runID
	ownerEmail := "agent-sessions-reflection-" + runID + "@example.com"
	finalMarker := "REFLECTION_ACK_" + strings.ToUpper(runID)
	orgMarker := "aurora-" + runID
	sandboxMarker := "orchid-" + runID

	ownerAuth := agentSessionsRegister(t, ctx, apiBase, ownerEmail, "agent-sessions-reflection-e2e-password", ownerName)
	orgID := ownerAuth.Orgs[0].ID
	ownerToken := ownerAuth.AccessToken
	agents := agentSessionsListAgents(t, ctx, apiBase, ownerToken, orgID)
	defaultAgent := findDefaultAgent(t, agents)
	channels := agentSessionsListChannels(t, ctx, apiBase, ownerToken, orgID)
	general := findDefaultGeneralChannel(t, channels, defaultAgent.ID)

	message := strings.Join([]string{
		"Hello. My name is Franz Keller, and I work in the Infrastructure Experience group on the Atlas platform team.",
		"Please keep the following long-lived context in mind for future conversations. You do not need to store anything right now and should not call any tools for this acknowledgement.",
		"- I prefer Python for automation scripts.",
		"- I prefer Go for backend services when the repository is already Go.",
		"- I like concise implementation plans before code changes.",
		"- I prefer Postgres for durable state.",
		"- I prefer lowercase kebab-case tags.",
		"- I want final summaries to be short and direct.",
		"- The organization codename for this initiative is " + orgMarker + ".",
		"- The Atlas platform team labels reflection-related work as memory-reflection.",
		"- For sandbox machines, the runtime control port is 7080.",
		"- Our sandbox base image nickname is " + sandboxMarker + ".",
		"- If environment setup fails, capture the exact missing service name.",
		"- Non-secret runbook IDs like hvy-env-42 are safe to remember.",
		"Reply exactly " + finalMarker + " YES if you understand.",
	}, "\n")
	session := agentSessionsCreateSession(t, ctx, apiBase, ownerToken, orgID, general.ID, message)
	assertAgentSessionsBackendOwnedMutationEvent(t, session.Event)
	sessionID := uuid.MustParse(session.Session.ID)
	ownerID := uuid.MustParse(ownerAuth.User.ID)

	cleanupDB := agentSessionsOpenDB(t)
	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), time.Minute)
		defer cleanupCancel()
		_ = cleanupDB.WithContext(cleanupCtx).Exec(
			"UPDATE agent_memories SET archived_at = now() WHERE source_session_id = ? AND archived_at IS NULL",
			sessionID,
		).Error
	})

	stream := agentSessionsStartSandboxStream(t, ctx, apiBase, ownerToken, orgID, session.Session.ID)
	stream.waitForEvent(t, ctx, 4*time.Minute, func(event runtimeSSEEvent) bool {
		return strings.Contains(event.RawData, finalMarker)
	})
	stream.waitForEvent(t, ctx, time.Minute, func(event runtimeSSEEvent) bool {
		return event.Name == "turn_completed" || event.Name == "done"
	})
	waitForAgentSessionsResponse(t, ctx, apiBase, ownerToken, orgID, session.Session.ID, finalMarker)
	waitForAgentSessionsTurnIdle(t, ctx, sessionID)

	agentSessionsBackdateReflectionEvents(t, ctx, sessionID, time.Now().UTC().Add(-4*time.Minute))
	state := waitForAgentSessionsReflectionComplete(t, ctx, sessionID)
	if state.LastReflectedEventID == nil || state.LastReflectedEventAt == nil {
		t.Fatalf("reflection state did not advance cursor: %+v", state)
	}

	rows := waitForAgentSessionsReflectionMemories(t, ctx, sessionID, ownerID, orgMarker, sandboxMarker)
	t.Logf("reflection memories=%s", summarizeAgentSessionsReflectionRows(rows))
}

func waitForAgentSessionsTurnIdle(t *testing.T, ctx context.Context, sessionID uuid.UUID) {
	t.Helper()
	db := agentSessionsOpenDB(t)
	deadline := time.Now().Add(2 * time.Minute)
	type turnRow struct {
		AgentTurnStatus      string `gorm:"column:agent_turn_status"`
		AgentTurnLastOutcome string `gorm:"column:agent_turn_last_outcome"`
	}
	var last turnRow
	for time.Now().Before(deadline) {
		var rows []turnRow
		if err := db.WithContext(ctx).Raw(
			"SELECT agent_turn_status, agent_turn_last_outcome FROM sessions WHERE id = ?",
			sessionID,
		).Scan(&rows).Error; err != nil {
			t.Fatalf("load session turn state: %v", err)
		}
		if len(rows) == 0 {
			t.Fatalf("session disappeared while waiting for idle: %s", sessionID)
		}
		last = rows[0]
		if last.AgentTurnStatus == model.SessionAgentTurnIdle {
			if last.AgentTurnLastOutcome == model.SessionAgentTurnOutcomeFailed {
				t.Fatalf("session turn failed before reflection: %+v", last)
			}
			return
		}
		select {
		case <-ctx.Done():
			t.Fatalf("context expired waiting for session idle: %v", ctx.Err())
		case <-time.After(2 * time.Second):
		}
	}
	t.Fatalf("timed out waiting for session idle: %+v", last)
}

func agentSessionsBackdateReflectionEvents(t *testing.T, ctx context.Context, sessionID uuid.UUID, eventAt time.Time) {
	t.Helper()
	db := agentSessionsOpenDB(t)
	if err := db.WithContext(ctx).Exec(`
UPDATE session_events
SET event_at = ?
WHERE session_id = ?
	AND (durability = 'durable' OR durability = '')`, eventAt, sessionID).Error; err != nil {
		t.Fatalf("backdate session events for reflection: %v", err)
	}
}

func waitForAgentSessionsReflectionComplete(t *testing.T, ctx context.Context, sessionID uuid.UUID) model.SessionReflectionState {
	t.Helper()
	db := agentSessionsOpenDB(t)
	deadline := time.Now().Add(5 * time.Minute)
	var last model.SessionReflectionState
	for time.Now().Before(deadline) {
		var rows []model.SessionReflectionState
		if err := db.WithContext(ctx).Where("session_id = ?", sessionID).Find(&rows).Error; err != nil {
			t.Fatalf("load reflection state: %v", err)
		}
		if len(rows) > 0 {
			last = rows[0]
			if last.Status == model.SessionReflectionStatusFailed && strings.TrimSpace(last.LastError) != "" {
				t.Fatalf("reflection failed: %s", last.LastError)
			}
			if last.Status == model.SessionReflectionStatusIdle && last.LastReflectedEventID != nil {
				return last
			}
		}
		t.Logf("waiting for reflection state session=%s status=%s last_event=%s last_error=%q",
			sessionID, last.Status, reflectionUUIDString(last.LastReflectedEventID), last.LastError)
		select {
		case <-ctx.Done():
			t.Fatalf("context expired waiting for reflection: %v", ctx.Err())
		case <-time.After(3 * time.Second):
		}
	}
	t.Fatalf("timed out waiting for reflection state session=%s status=%s last_event=%s last_error=%q",
		sessionID, last.Status, reflectionUUIDString(last.LastReflectedEventID), last.LastError)
	return model.SessionReflectionState{}
}

type agentSessionsReflectionMemoryRow struct {
	ID                uuid.UUID  `gorm:"column:id"`
	OrgID             uuid.UUID  `gorm:"column:org_id"`
	ChannelID         *uuid.UUID `gorm:"column:channel_id"`
	Content           string     `gorm:"column:content"`
	MemoryFingerprint string     `gorm:"column:memory_fingerprint"`
	TagsCSV           string     `gorm:"column:tags_csv"`
	Metadata          model.JSON `gorm:"column:metadata"`
	SourceSessionID   *uuid.UUID `gorm:"column:source_session_id"`
	SourceEventID     *uuid.UUID `gorm:"column:source_event_id"`
}

func waitForAgentSessionsReflectionMemories(t *testing.T, ctx context.Context, sessionID, ownerID uuid.UUID, orgMarker, sandboxMarker string) []agentSessionsReflectionMemoryRow {
	t.Helper()
	db := agentSessionsOpenDB(t)
	deadline := time.Now().Add(2 * time.Minute)
	var last []agentSessionsReflectionMemoryRow
	for time.Now().Before(deadline) {
		rows := loadAgentSessionsReflectionMemoryRows(t, db, sessionID)
		last = rows
		if agentSessionsReflectionRowsReady(rows, ownerID, orgMarker, sandboxMarker) {
			return rows
		}
		select {
		case <-ctx.Done():
			t.Fatalf("context expired waiting for reflection memories: %v", ctx.Err())
		case <-time.After(2 * time.Second):
		}
	}
	t.Fatalf("timed out waiting for reflection memories rows=%s", summarizeAgentSessionsReflectionRows(last))
	return nil
}

func loadAgentSessionsReflectionMemoryRows(t *testing.T, db *gorm.DB, sessionID uuid.UUID) []agentSessionsReflectionMemoryRow {
	t.Helper()
	var rows []agentSessionsReflectionMemoryRow
	if err := db.Raw(`
SELECT id, org_id, channel_id, content, memory_fingerprint,
       array_to_string(tags, ',') AS tags_csv, metadata, source_session_id, source_event_id
FROM agent_memories
WHERE archived_at IS NULL
	AND source_session_id = ?
	AND metadata->>'source' = 'reflection'
ORDER BY created_at ASC`, sessionID).Scan(&rows).Error; err != nil {
		t.Fatalf("load reflection memory rows: %v", err)
	}
	return rows
}

func agentSessionsReflectionRowsReady(rows []agentSessionsReflectionMemoryRow, _ uuid.UUID, orgMarker, sandboxMarker string) bool {
	if len(rows) < 4 {
		return false
	}
	// Reflection memories are all channel-scoped now; verify content markers are
	// present across the channel's rows.
	var pythonOK, actorOK, orgOK, sandboxOK bool
	for _, row := range rows {
		if row.MemoryFingerprint == "" || row.SourceEventID == nil || !strings.Contains(row.TagsCSV, "reflection") {
			return false
		}
		if reflectionMetadataString(row.Metadata, "source") != "reflection" || row.ChannelID == nil {
			return false
		}
		content := strings.ToLower(row.Content)
		pythonOK = pythonOK || strings.Contains(content, "python")
		actor := strings.ToLower(reflectionMetadataString(row.Metadata, "actor_display_name"))
		actorOK = actorOK || strings.Contains(actor, "franz")
		orgOK = orgOK || strings.Contains(content, strings.ToLower(orgMarker))
		sandboxOK = sandboxOK || strings.Contains(content, strings.ToLower(sandboxMarker))
	}
	return pythonOK && actorOK && orgOK && sandboxOK
}

func reflectionMetadataString(metadata model.JSON, key string) string {
	if metadata == nil {
		return ""
	}
	value, _ := metadata[key].(string)
	return strings.TrimSpace(value)
}

func summarizeAgentSessionsReflectionRows(rows []agentSessionsReflectionMemoryRow) string {
	type rowSummary struct {
		ChannelID string `json:"channel_id,omitempty"`
		Content   string `json:"content"`
		Tags      string `json:"tags"`
		EventID   string `json:"source_event_id,omitempty"`
		Actor     string `json:"actor_display_name,omitempty"`
		HasFinger bool   `json:"has_fingerprint"`
	}
	summaries := make([]rowSummary, 0, len(rows))
	for _, row := range rows {
		summaries = append(summaries, rowSummary{
			ChannelID: reflectionUUIDString(row.ChannelID),
			Content:   row.Content,
			Tags:      row.TagsCSV,
			EventID:   reflectionUUIDString(row.SourceEventID),
			Actor:     reflectionMetadataString(row.Metadata, "actor_display_name"),
			HasFinger: row.MemoryFingerprint != "",
		})
	}
	raw, _ := json.Marshal(summaries)
	return string(raw)
}

func reflectionUUIDString(value *uuid.UUID) string {
	if value == nil {
		return ""
	}
	return value.String()
}
