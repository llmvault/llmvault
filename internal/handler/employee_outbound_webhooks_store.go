package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/gateway"
	"github.com/usehivy/hivy/internal/model"
	"github.com/usehivy/hivy/internal/precontext"
)

// storeAndMaybeEnqueue persists the event and triggers follow-up work. It errors
// only when a revenue/reply-bearing record is lost (so the caller returns 5xx and
// the outbox redelivers); best-effort side effects are captured to Sentry.
func (h *EmployeeOutboundWebhookHandler) storeAndMaybeEnqueue(ctx context.Context, sb *model.Sandbox, event *employeeOutboundEvent) error {
	payload := map[string]any{}
	if len(event.Payload) > 0 {
		if err := json.Unmarshal(event.Payload, &payload); err != nil {
			captureEmployeeWebhookIngest(ctx, "decode_event_payload", sb, event, "", "", err)
		}
	}
	sessionID := stringValue(payload, "session_id")
	source := employeeEventSource(payload)
	if _, ok := payload["mode"]; !ok {
		payload["mode"] = "employee"
	}
	if event.EventType == "agent.run.model.usage" {
		if err := h.recordRuntimeModelUsageGeneration(ctx, sb, event, payload); err != nil {
			// Sole billing record for runtime LLM usage; surface the failure for a 5xx
			// so the outbox retries (deterministic ID makes the retry idempotent).
			captureEmployeeWebhookIngest(ctx, "record_runtime_generation", sb, event, sessionID, source, err)
			return fmt.Errorf("record runtime model usage: %w", err)
		}
	}
	if event.EventType == "skill.synced" {
		if err := h.syncSkillEvent(ctx, sb, payload); err != nil {
			captureEmployeeWebhookIngest(ctx, "sync_skill", sb, event, sessionID, source, err)
		}
	}
	if event.EventType == "session.created" {
		session, createdSession, err := h.ensureEmployeeSession(ctx, sb, sessionID, source, payload)
		if err != nil {
			captureEmployeeWebhookIngest(ctx, "ensure_employee_session", sb, event, sessionID, source, err)
			return fmt.Errorf("ensure employee session: %w", err)
		}
		if createdSession {
			precontext.InvalidateSessions(ctx, h.preloadCache, session.OrgID, session.EmployeeID)
			h.enqueueEmployeeMemoryRetain(ctx, sb, session, sessionID, "session_created", "session.created")
		}
		return nil
	}
	if !shouldStoreEmployeeSessionEvent(event.EventType) {
		return nil
	}
	session, createdSession, err := h.ensureEmployeeSession(ctx, sb, sessionID, source, payload)
	if err != nil {
		captureEmployeeWebhookIngest(ctx, "ensure_employee_session", sb, event, sessionID, source, err)
		return fmt.Errorf("ensure employee session: %w", err)
	}
	if createdSession {
		precontext.InvalidateSessions(ctx, h.preloadCache, session.OrgID, session.EmployeeID)
		h.enqueueEmployeeMemoryRetain(ctx, sb, session, sessionID, "session_created", "session.created")
	}
	stored, ok := employeeSessionEventFromOutbound(sb, event, payload, session.ID, sessionID)
	if !ok {
		captureEmployeeWebhookIngest(ctx, "drop_missing_sandbox_owner", sb, event, sessionID, source, fmt.Errorf("employee sandbox missing org_id or employee_id"))
		return nil
	}
	stored.Mode = "employee"
	if h.writer != nil {
		// Buffered path: the writer's retrying drain provides durability; the ack
		// doesn't block on the async write but a DB error no longer drops the row.
		h.writer.Write(ctx, stored)
	} else {
		// Synchronous path: the ack must depend on the durable write, so a failure
		// returns 5xx. The (sandbox_id, event_id) dedupe makes redelivery idempotent.
		err := h.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
			if err := tx.Clauses(employeeSessionEventOnConflict()).Create(&stored).Error; err != nil {
				return err
			}
			if err := syncEmployeeScheduleEvent(tx, stored); err != nil {
				captureEmployeeSessionEventFailure(ctx, "sync_schedule", stored, err)
			}
			return nil
		})
		if err != nil {
			captureEmployeeSessionEventFailure(ctx, "store_memory_event", stored, err)
			return fmt.Errorf("store session event: %w", err)
		}
		precontext.InvalidateSessions(ctx, h.preloadCache, stored.OrgID, stored.EmployeeID)
	}
	if event.EventType == "agent.message.sent" {
		h.enqueueEmployeeMemoryRetain(ctx, sb, session, sessionID, "agent_message_sent", "agent.message.sent")
	}
	if event.EventType == "agent.message.sent" && h.gateway != nil && shouldDeliverGatewayRuntimeFinal(session, payload) {
		if _, err := h.gateway.HandleRuntimeFinal(ctx, gateway.AgentResponse{
			EmployeeSession:  *session,
			RuntimeSessionID: sessionID,
			TraceID:          stringValue(payload, "trace_id"),
			TurnID:           stringValue(payload, "turn_id"),
			ChannelID:        stringValue(payload, "channel_id"),
			ThreadID:         stringValue(payload, "thread_id"),
			Text:             stringValue(payload, "text"),
			Raw:              payload,
		}); err != nil {
			captureEmployeeWebhookIngest(ctx, "gateway_deliver_runtime_final", sb, event, sessionID, source, err)
		}
	}
	if event.EventType == "session.completed" {
		h.markEmployeeSessionEnded(ctx, session.ID, event.At)
	}
	return nil
}

func shouldDeliverGatewayRuntimeFinal(session *model.EmployeeSession, payload map[string]any) bool {
	if session == nil || session.Source != gateway.Source {
		return false
	}
	return !isSlackGatewayEvent(payload)
}

func isSlackGatewayEvent(payload map[string]any) bool {
	return strings.EqualFold(strings.TrimSpace(stringValue(payload, "provider")), gateway.SlackProvider)
}
