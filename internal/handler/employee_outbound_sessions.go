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

// employeeSessionEventOnConflict deduplicates inserts against the partial unique
// index on (sandbox_id, event_id), so an outbox redelivery is skipped. TargetWhere
// mirrors the index predicate so Postgres matches the partial index.
func employeeSessionEventOnConflict() clause.OnConflict {
	return clause.OnConflict{
		Columns:     []clause.Column{{Name: "sandbox_id"}, {Name: "event_id"}},
		TargetWhere: clause.Where{Exprs: []clause.Expression{gorm.Expr("event_id <> ?", "")}},
		DoNothing:   true,
	}
}

func (h *EmployeeOutboundWebhookHandler) ensureEmployeeSession(ctx context.Context, sb *model.Sandbox, sessionID, source string, payload map[string]any, specialistTask *model.SpecialistTask) (*model.EmployeeSession, bool, error) {
	if sb.OrgID == nil || sb.EmployeeID == nil {
		return nil, false, fmt.Errorf("employee sandbox missing org_id or employee_id")
	}
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return nil, false, fmt.Errorf("runtime session_id is required for employee session events")
	}
	if specialistTask != nil && specialistTask.ConversationID != nil {
		var session model.EmployeeSession
		if err := h.db.WithContext(ctx).
			Where("id = ? AND org_id = ? AND employee_id = ?", *specialistTask.ConversationID, *sb.OrgID, *sb.EmployeeID).
			First(&session).Error; err != nil {
			return nil, false, fmt.Errorf("load specialist employee session: %w", err)
		}
		return &session, false, nil
	}
	if specialistTask != nil {
		var session model.EmployeeSession
		err := h.db.WithContext(ctx).
			Where("org_id = ? AND employee_id = ? AND runtime_conversation_id = ?", specialistTask.OrgID, specialistTask.EmployeeID, specialistTask.EmployeeSessionID).
			Order("created_at DESC").
			First(&session).Error
		if err == nil {
			if updateErr := h.db.WithContext(ctx).Model(specialistTask).Update("conversation_id", session.ID).Error; updateErr != nil {
				return nil, false, fmt.Errorf("backfill specialist employee session: %w", updateErr)
			}
			return &session, false, nil
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, false, fmt.Errorf("load specialist employee session: %w", err)
		}
	}
	if source == "" {
		source = employeeEventSource(payload)
	}
	session := model.EmployeeSession{}
	scope := model.EmployeeSession{
		OrgID:                 *sb.OrgID,
		EmployeeID:            *sb.EmployeeID,
		SandboxID:             sb.ID,
		RuntimeConversationID: sessionID,
	}
	attrs := model.EmployeeSession{
		Source:            source,
		SourceResourceKey: employeeSessionSourceResourceKey(payload, sessionID),
		Status:            "active",
		IntegrationScopes: model.JSON{},
	}
	result := h.db.WithContext(ctx).Where(&scope).Attrs(attrs).FirstOrCreate(&session)
	if result.Error != nil {
		return nil, false, fmt.Errorf("upsert employee session: %w", result.Error)
	}
	return &session, result.RowsAffected > 0, nil
}

func (h *EmployeeOutboundWebhookHandler) markEmployeeSessionEnded(ctx context.Context, sessionID uuid.UUID, at time.Time) {
	if h == nil || h.db == nil || sessionID == uuid.Nil {
		return
	}
	endedAt := at.UTC()
	if endedAt.IsZero() {
		endedAt = h.now().UTC()
	}
	if err := h.db.WithContext(ctx).Model(&model.EmployeeSession{}).
		Where("id = ?", sessionID).
		Updates(map[string]any{"status": "ended", "ended_at": endedAt}).Error; err != nil {
		logging.Capture(ctx, fmt.Errorf("employee outbound webhook: mark session ended: %w", err))
	}
}

func employeeSessionSourceResourceKey(payload map[string]any, fallback string) string {
	return firstNonEmpty(
		stringValue(payload, "source_resource_key"),
		stringValue(payload, "thread_ts"),
		stringValue(payload, "channel"),
		stringValue(payload, "conversation_id"),
		stringValue(payload, "chat_id"),
		fallback,
	)
}

func employeeSessionEventFromOutbound(sb *model.Sandbox, event *employeeOutboundEvent, payload map[string]any, employeeSessionID uuid.UUID, sessionID string) (model.EmployeeSessionEvent, bool) {
	if sb.OrgID == nil || sb.EmployeeID == nil {
		return model.EmployeeSessionEvent{}, false
	}
	stored := model.EmployeeSessionEvent{
		OrgID:             *sb.OrgID,
		EmployeeID:        *sb.EmployeeID,
		SandboxID:         sb.ID,
		EmployeeSessionID: employeeSessionID,
		SessionID:         sessionID,
		EventID:           employeeSessionEventID(payload),
		EventType:         event.EventType,
		Source:            employeeEventSource(payload),
		Mode:              stringValueDefault(payload, "mode", "employee"),
		SequenceNumber:    int64Value(payload, "sequence"),
		Payload:           model.RawJSON(event.Payload),
		EventAt:           event.At.UTC(),
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

func employeeSessionEventID(payload map[string]any) string {
	return firstNonEmpty(stringValue(payload, "event_id"), stringValue(payload, "id"))
}

func deriveSessionEventID(sandboxID uuid.UUID, event *employeeOutboundEvent, sessionID string) string {
	sum := sha256.Sum256(fmt.Appendf(nil, "%s|%s|%s|%d|%s",
		sandboxID.String(),
		sessionID,
		event.EventType,
		event.At.UTC().UnixNano(),
		event.Payload,
	))
	return "evt_rt_" + hex.EncodeToString(sum[:16])
}

func shouldStoreEmployeeSessionEvent(eventType string) bool {
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

func shouldTriggerEmployeeMemoryCheckpoint(eventType string) bool {
	return eventType == "session.created"
}
