package handler

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
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
		Columns:     []clause.Column{{Name: "sandbox_id"}, {Name: "event_id"}},
		TargetWhere: clause.Where{Exprs: []clause.Expression{gorm.Expr("event_id <> ?", "")}},
		DoNothing:   true,
	}
}

func (h *AgentOutboundWebhookHandler) ensureAgentSession(ctx context.Context, sb *model.Sandbox, sessionID, source string, payload map[string]any) (*model.AgentSession, bool, error) {
	if sb.OrgID == nil || sb.AgentID == nil {
		return nil, false, fmt.Errorf("agent sandbox missing org_id or agent_id")
	}
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return nil, false, fmt.Errorf("runtime session_id is required for agent session events")
	}
	if source == "" {
		source = agentEventSource(payload)
	}
	session := model.AgentSession{}
	scope := model.AgentSession{
		OrgID:                 *sb.OrgID,
		AgentID:               *sb.AgentID,
		SandboxID:             sb.ID,
		RuntimeConversationID: sessionID,
	}
	attrs := model.AgentSession{
		Source:            source,
		SourceResourceKey: agentSessionSourceResourceKey(payload, sessionID),
		Status:            "active",
		IntegrationScopes: model.JSON{},
	}
	result := h.db.WithContext(ctx).Where(&scope).Attrs(attrs).FirstOrCreate(&session)
	if result.Error != nil {
		return nil, false, fmt.Errorf("upsert agent session: %w", result.Error)
	}
	return &session, result.RowsAffected > 0, nil
}

func (h *AgentOutboundWebhookHandler) markAgentSessionEnded(ctx context.Context, sessionID uuid.UUID, at time.Time) {
	if h == nil || h.db == nil || sessionID == uuid.Nil {
		return
	}
	endedAt := at.UTC()
	if endedAt.IsZero() {
		endedAt = h.now().UTC()
	}
	if err := h.db.WithContext(ctx).Model(&model.AgentSession{}).
		Where("id = ?", sessionID).
		Updates(map[string]any{"status": "ended", "ended_at": endedAt}).Error; err != nil {
		logging.Capture(ctx, fmt.Errorf("agent outbound webhook: mark session ended: %w", err))
	}
}

func agentSessionSourceResourceKey(payload map[string]any, fallback string) string {
	return firstNonEmpty(
		stringValue(payload, "source_resource_key"),
		stringValue(payload, "thread_ts"),
		stringValue(payload, "channel"),
		stringValue(payload, "conversation_id"),
		stringValue(payload, "chat_id"),
		fallback,
	)
}

func agentSessionEventFromOutbound(sb *model.Sandbox, event *agentOutboundEvent, payload map[string]any, agentSessionID uuid.UUID, sessionID string) (model.AgentSessionEvent, bool) {
	if sb.OrgID == nil || sb.AgentID == nil {
		return model.AgentSessionEvent{}, false
	}
	stored := model.AgentSessionEvent{
		OrgID:          *sb.OrgID,
		AgentID:        *sb.AgentID,
		SandboxID:      sb.ID,
		AgentSessionID: agentSessionID,
		SessionID:      sessionID,
		EventID:        agentSessionEventID(payload),
		EventType:      event.EventType,
		Source:         agentEventSource(payload),
		Mode:           stringValueDefault(payload, "mode", "agent"),
		SequenceNumber: int64Value(payload, "sequence"),
		Payload:        model.RawJSON(event.Payload),
		EventAt:        event.At.UTC(),
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

func agentSessionEventID(payload map[string]any) string {
	return firstNonEmpty(stringValue(payload, "event_id"), stringValue(payload, "id"))
}

func deriveSessionEventID(sandboxID uuid.UUID, event *agentOutboundEvent, sessionID string) string {
	sum := sha256.Sum256(fmt.Appendf(nil, "%s|%s|%s|%d|%s",
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
