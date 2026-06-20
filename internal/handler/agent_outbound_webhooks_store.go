package handler

import (
	"context"
	"encoding/json"
	"fmt"

	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/model"
	"github.com/usehivy/hivy/internal/precontext"
	"github.com/usehivy/hivy/internal/runtimeevents"
)

// storeAndMaybeEnqueue persists the event and triggers follow-up work. It errors
// only when a revenue/reply-bearing record is lost (so the caller returns 5xx and
// the outbox redelivers); best-effort side effects are captured to Sentry.
func (h *AgentOutboundWebhookHandler) storeAndMaybeEnqueue(ctx context.Context, sb *model.Sandbox, event *agentOutboundEvent) error {
	payload := map[string]any{}
	if len(event.Payload) > 0 {
		if err := json.Unmarshal(event.Payload, &payload); err != nil {
			captureAgentWebhookIngest(ctx, "decode_event_payload", sb, event, "", "", err)
		}
	}
	sessionID := stringValue(payload, "session_id")
	source := agentEventSource(payload)
	if event.EventType == runtimeevents.EventModelUsage {
		if err := h.recordRuntimeModelUsageGeneration(ctx, sb, event, payload); err != nil {
			// Sole billing record for runtime LLM usage; surface the failure for a 5xx
			// so the outbox retries (deterministic ID makes the retry idempotent).
			captureAgentWebhookIngest(ctx, "record_runtime_generation", sb, event, sessionID, source, err)
			return fmt.Errorf("record runtime model usage: %w", err)
		}
	}
	if event.EventType == "skill.synced" {
		if err := h.syncSkillEvent(ctx, sb, payload); err != nil {
			captureAgentWebhookIngest(ctx, "sync_skill", sb, event, sessionID, source, err)
		}
	}
	if event.EventType == "session.created" {
		session, createdSession, err := h.ensureRuntimeSession(ctx, sb, sessionID, source, payload)
		if err != nil {
			captureAgentWebhookIngest(ctx, "ensure_session", sb, event, sessionID, source, err)
			return fmt.Errorf("ensure runtime session: %w", err)
		}
		if createdSession {
			precontext.InvalidateSessions(ctx, h.preloadCache, session.OrgID, session.AgentID)
		}
		return nil
	}
	if !shouldStoreRuntimeSessionEvent(event.EventType) {
		return nil
	}
	session, createdSession, err := h.ensureRuntimeSession(ctx, sb, sessionID, source, payload)
	if err != nil {
		captureAgentWebhookIngest(ctx, "ensure_session", sb, event, sessionID, source, err)
		return fmt.Errorf("ensure runtime session: %w", err)
	}
	if createdSession {
		precontext.InvalidateSessions(ctx, h.preloadCache, session.OrgID, session.AgentID)
	}
	stored, ok := sessionEventFromOutbound(sb, event, payload, session.ID, sessionID)
	if !ok {
		captureAgentWebhookIngest(ctx, "drop_missing_sandbox_owner", sb, event, sessionID, source, fmt.Errorf("agent sandbox missing org_id or agent_id"))
		return nil
	}
	if h.writer != nil {
		// Buffered path: the writer's retrying drain provides durability; the ack
		// doesn't block on the async write but a DB error no longer drops the row.
		h.writer.Write(ctx, stored)
	} else {
		// Synchronous path: the ack must depend on the durable write, so a failure
		// returns 5xx. The (sandbox_id, event_id) dedupe makes redelivery idempotent.
		err := h.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
			if err := tx.Clauses(sessionEventOnConflict()).Create(&stored).Error; err != nil {
				return err
			}
			if err := syncAgentScheduleEvent(tx, stored); err != nil {
				captureSessionEventFailure(ctx, "sync_schedule", stored, err)
			}
			return nil
		})
		if err != nil {
			captureSessionEventFailure(ctx, "store_memory_event", stored, err)
			return fmt.Errorf("store session event: %w", err)
		}
		precontext.InvalidateSessions(ctx, h.preloadCache, stored.OrgID, stored.AgentID)
	}
	if event.EventType == "session.completed" {
		h.markSessionEnded(ctx, session.ID, event.At)
	}
	if event.EventType == runtimeevents.EventTurnCompleted || event.EventType == runtimeevents.EventTurnFailed {
		h.completeSessionTurn(ctx, session, event.EventType, payload)
	}
	return nil
}
