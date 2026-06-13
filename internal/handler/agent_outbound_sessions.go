package handler

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/usehivy/hivy/internal/logging"
	"github.com/usehivy/hivy/internal/model"
)

// agentSessionEventOnConflict deduplicates inserts against the partial unique
// index on (sandbox_id, event_id), so an outbox redelivery is skipped. TargetWhere
// mirrors the index predicate so Postgres matches the partial index.
func agentSessionEventOnConflict() clause.OnConflict {
	return clause.OnConflict{
		Columns:     []clause.Column{{Name: "session_id"}, {Name: "event_id"}},
		TargetWhere: clause.Where{Exprs: []clause.Expression{gorm.Expr("event_id IS NOT NULL")}},
		DoNothing:   true,
	}
}

func (h *AgentOutboundWebhookHandler) ensureRuntimeSession(ctx context.Context, sb *model.Sandbox, runtimeSessionID, source string, payload map[string]any) (*model.Session, bool, error) {
	if sb.OrgID == nil || sb.AgentID == nil {
		return nil, false, fmt.Errorf("agent sandbox missing org_id or agent_id")
	}
	runtimeSessionID = strings.TrimSpace(runtimeSessionID)
	if runtimeSessionID == "" {
		return nil, false, fmt.Errorf("runtime session_id is required for session events")
	}
	if source == "" {
		source = agentEventSource(payload)
	}
	sessionID := canonicalSessionUUID(sb.ID, runtimeSessionID)
	var session model.Session
	err := h.db.WithContext(ctx).
		Where("id = ? AND org_id = ? AND agent_id = ?", sessionID, *sb.OrgID, *sb.AgentID).
		First(&session).Error
	if err == nil {
		return &session, false, nil
	}
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, false, fmt.Errorf("load session: %w", err)
	}
	channelID, err := h.ensureRuntimeEventChannel(ctx, sb, source, payload)
	if err != nil {
		return nil, false, err
	}
	session = model.Session{
		ID:                sessionID,
		OrgID:             *sb.OrgID,
		ChannelID:         channelID,
		AgentID:           *sb.AgentID,
		SandboxID:         &sb.ID,
		Source:            source,
		SourceResourceKey: sessionSourceResourceKey(payload, runtimeSessionID),
		Name:              defaultString(stringValue(payload, "session_name"), "Runtime session"),
		Status:            "active",
		IntegrationScopes: model.JSON{},
	}
	if err := h.db.WithContext(ctx).Create(&session).Error; err != nil {
		return nil, false, fmt.Errorf("create session: %w", err)
	}
	return &session, true, nil
}

func (h *AgentOutboundWebhookHandler) ensureRuntimeEventChannel(ctx context.Context, sb *model.Sandbox, source string, payload map[string]any) (uuid.UUID, error) {
	if raw := strings.TrimSpace(stringValue(payload, "channel_id")); raw != "" {
		if id, err := uuid.Parse(raw); err == nil {
			var count int64
			if err := h.db.WithContext(ctx).Model(&model.Channel{}).
				Where("id = ? AND org_id = ? AND archived_at IS NULL", id, *sb.OrgID).
				Count(&count).Error; err != nil {
				return uuid.Nil, fmt.Errorf("load event channel: %w", err)
			}
			if count > 0 {
				return id, nil
			}
		}
	}
	name := "runtime-" + shortUUID(*sb.AgentID)
	channel := model.Channel{}
	scope := model.Channel{OrgID: *sb.OrgID, Origin: "native", Name: name}
	attrs := model.Channel{
		Description:      "Runtime-originated agent activity",
		Kind:             "system",
		Visibility:       "private",
		DefaultAgentID:   *sb.AgentID,
		ExternalMetadata: model.JSON{"source": source},
	}
	if err := h.db.WithContext(ctx).Where(&scope).Attrs(attrs).FirstOrCreate(&channel).Error; err != nil {
		return uuid.Nil, fmt.Errorf("ensure runtime event channel: %w", err)
	}
	return channel.ID, nil
}

func canonicalSessionUUID(sandboxID uuid.UUID, runtimeSessionID string) uuid.UUID {
	if id, err := uuid.Parse(strings.TrimSpace(runtimeSessionID)); err == nil {
		return id
	}
	return uuid.NewSHA1(uuid.NameSpaceURL, []byte("hivy:runtime-session:"+sandboxID.String()+":"+strings.TrimSpace(runtimeSessionID)))
}

func shortUUID(id uuid.UUID) string {
	value := id.String()
	if len(value) < 8 {
		return value
	}
	return value[:8]
}

func (h *AgentOutboundWebhookHandler) markSessionEnded(ctx context.Context, sessionID uuid.UUID, at time.Time) {
	if h == nil || h.db == nil || sessionID == uuid.Nil {
		return
	}
	endedAt := at.UTC()
	if endedAt.IsZero() {
		endedAt = h.now().UTC()
	}
	if err := h.db.WithContext(ctx).Model(&model.Session{}).
		Where("id = ?", sessionID).
		Updates(map[string]any{"status": "ended", "ended_at": endedAt}).Error; err != nil {
		logging.Capture(ctx, fmt.Errorf("agent outbound webhook: mark runtime session ended: %w", err))
	}
}

func sessionSourceResourceKey(payload map[string]any, fallback string) string {
	return firstNonEmpty(
		stringValue(payload, "source_resource_key"),
		stringValue(payload, "thread_ts"),
		stringValue(payload, "channel"),
		stringValue(payload, "chat_id"),
		fallback,
	)
}

func sessionEventFromOutbound(sb *model.Sandbox, event *agentOutboundEvent, payload map[string]any, sessionID uuid.UUID, runtimeSessionID string) (model.SessionEvent, bool) {
	if sb.OrgID == nil || sb.AgentID == nil {
		return model.SessionEvent{}, false
	}
	stored := model.SessionEvent{
		OrgID:            *sb.OrgID,
		AgentID:          *sb.AgentID,
		SandboxID:        &sb.ID,
		SessionID:        sessionID,
		RuntimeSessionID: runtimeSessionID,
		EventID:          sessionEventID(payload),
		EventType:        event.EventType,
		Source:           agentEventSource(payload),
		SequenceNumber:   int64Value(payload, "sequence"),
		Payload:          model.JSON(payload),
		EventAt:          event.At.UTC(),
	}
	if stored.EventID == "" {
		// The runtime stamps no stable event id and `sequence` collides across turns,
		// so the dedupe key is a content hash: stable across redeliveries yet distinct
		// per event. Paired with the (sandbox_id, event_id) index this makes redelivery
		// idempotent.
		stored.EventID = deriveSessionEventID(sb.ID, event, sessionID)
	}
	return stored, true
}

func sessionEventID(payload map[string]any) string {
	return firstNonEmpty(stringValue(payload, "event_id"), stringValue(payload, "id"))
}

func deriveSessionEventID(sandboxID uuid.UUID, event *agentOutboundEvent, sessionID any) string {
	sum := sha256.Sum256(fmt.Appendf(nil, "%s|%v|%s|%d|%s",
		sandboxID.String(),
		sessionID,
		event.EventType,
		event.At.UTC().UnixNano(),
		event.Payload,
	))
	return "evt_rt_" + hex.EncodeToString(sum[:16])
}

func shouldStoreAgentSessionEvent(eventType string) bool {
	switch {
	case eventType == "session.created":
		return false
	case eventType == "tool.invoked":
		return false
	case eventType == "agent.final_message":
		return false
	case strings.HasPrefix(eventType, "agent.run."):
		return false
	default:
		return true
	}
}

func shouldTriggerAgentMemoryCheckpoint(eventType string) bool {
	return eventType == "session.created"
}
