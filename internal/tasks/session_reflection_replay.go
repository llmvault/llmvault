package tasks

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/model"
	"github.com/usehivy/hivy/internal/trigger/hivy"
)

// ReflectionMemoryCandidate exposes the reflection candidate shape to the
// replay harness (cmd/reflect-replay) without duplicating it.
type ReflectionMemoryCandidate = reflectionMemoryCandidate

// ReflectionReplayRun is one replayed reflection: the exact production
// transcript and prompt inputs plus the memories the pipeline would keep.
type ReflectionReplayRun struct {
	SessionID    uuid.UUID
	SessionName  string
	AgentName    string
	EventCount   int
	Transcript   string
	Existing     string
	SystemPrompt string
	RawResponse  string
	Memories     []ReflectionMemoryCandidate
}

// ReplaySessionReflection renders the reflection transcript for a session
// exactly as production does (same event window, same transcript renderer,
// same prompt builder, same normalization and cap) and runs the extraction
// prompt against it. It never writes memories or touches reflection state.
func ReplaySessionReflection(ctx context.Context, db *gorm.DB, client hivy.CompletionClient, modelID string, sessionID uuid.UUID) (*ReflectionReplayRun, error) {
	if db == nil {
		return nil, fmt.Errorf("replay requires a database connection")
	}
	h := &SessionReflectionHandler{db: db}
	var session model.Session
	if err := db.WithContext(ctx).First(&session, "id = ?", sessionID).Error; err != nil {
		return nil, fmt.Errorf("load session %s: %w", sessionID, err)
	}
	latest, ok, err := loadLatestReflectableEvent(db.WithContext(ctx), sessionID)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, fmt.Errorf("session %s has no reflectable events", sessionID)
	}
	// Same window a fresh reflection state would process: everything up to
	// the latest durable event, capped at the production event limit.
	claim := sessionReflectionClaim{
		Session:        session,
		ThroughEventID: latest.ID,
		ThroughEventAt: latest.EventAt,
	}
	events, err := h.loadEvents(ctx, claim)
	if err != nil {
		return nil, err
	}
	userNames := h.loadReflectionUserNames(ctx, session, events)
	transcript, _ := renderSessionReflectionTranscript(session, "", events, userNames)
	existing := h.loadExistingMemories(ctx, session.ID)
	agentMission := h.loadAgentMission(ctx, session)
	run := &ReflectionReplayRun{
		SessionID:    session.ID,
		SessionName:  session.Name,
		AgentName:    "",
		EventCount:   len(events),
		Transcript:   transcript,
		Existing:     existing,
		SystemPrompt: buildSessionReflectionSystemPrompt(agentMission),
	}
	if client == nil {
		return run, nil
	}
	result, raw, err := generateSessionReflection(ctx, client, modelID, reflectionModelConfigFromEnv().Temperature, transcript, existing, agentMission)
	run.RawResponse = raw
	if err != nil {
		return run, err
	}
	run.Memories = result.Memories
	return run, nil
}

// RecentReflectableSessionIDs lists the most recently active sessions that
// have at least one durable event, newest first.
func RecentReflectableSessionIDs(ctx context.Context, db *gorm.DB, limit int) ([]uuid.UUID, error) {
	if limit <= 0 {
		limit = 5
	}
	type row struct {
		SessionID uuid.UUID
	}
	var rows []row
	err := db.WithContext(ctx).Raw(`
SELECT s.id AS session_id
FROM sessions s
JOIN LATERAL (
	SELECT event_at
	FROM session_events
	WHERE session_id = s.id
		AND (durability = 'durable' OR durability = '')
	ORDER BY event_at DESC, id DESC
	LIMIT 1
) latest ON true
ORDER BY latest.event_at DESC
LIMIT ?`, limit).Scan(&rows).Error
	if err != nil {
		return nil, fmt.Errorf("list recent reflectable sessions: %w", err)
	}
	out := make([]uuid.UUID, 0, len(rows))
	for _, r := range rows {
		out = append(out, r.SessionID)
	}
	return out, nil
}
