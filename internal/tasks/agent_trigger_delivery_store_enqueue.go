package tasks

import (
	"context"
	"fmt"

	"github.com/usehivy/hivy/internal/agentruntime"
	"github.com/usehivy/hivy/internal/logging"
	"github.com/usehivy/hivy/internal/model"
)

func (h *AgentTriggerDispatchHandler) enqueueStoreDelivery(ctx context.Context, payload AgentTriggerDispatchPayload, trigger model.AgentTrigger, session *model.Session, compiled compiledTriggerMessage, resp *agentruntime.HTTPMessageResponse) {
	if h.enqueuer == nil {
		logging.CaptureWithFields(ctx, fmt.Errorf("agent trigger delivery store enqueue skipped: enqueuer is nil"), triggerStoreEnqueueFields(payload, trigger, session, compiled, resp))
		return
	}
	if resp == nil {
		resp = &agentruntime.HTTPMessageResponse{}
	}
	task, opts, err := NewAgentTriggerStoreDeliveryTask(AgentTriggerStoreDeliveryPayload{
		OrgID:            trigger.OrgID,
		AgentID:          trigger.AgentID,
		TriggerID:        trigger.ID,
		ConnectionID:     trigger.ConnectionID,
		DeliveryID:       payload.DeliveryID,
		EventKey:         eventKey(payload.EventType, payload.EventAction),
		ResourceKey:      compiled.ResourceKey,
		SessionID:        session.ID,
		RuntimeSessionID: resp.SessionID,
		RuntimeStreamID:  resp.StreamID,
		RuntimeTraceID:   resp.TraceID,
		RuntimeTurnID:    resp.TurnID,
		PayloadJSON:      payload.PayloadJSON,
	})
	if err != nil {
		logging.CaptureWithFields(ctx, fmt.Errorf("build agent trigger delivery store task: %w", err), triggerStoreEnqueueFields(payload, trigger, session, compiled, resp))
		return
	}
	if _, err := h.enqueuer.EnqueueContext(ctx, task, opts...); err != nil {
		logging.CaptureWithFields(ctx, fmt.Errorf("enqueue agent trigger delivery store task: %w", err), triggerStoreEnqueueFields(payload, trigger, session, compiled, resp))
	}
}

func triggerStoreEnqueueFields(payload AgentTriggerDispatchPayload, trigger model.AgentTrigger, session *model.Session, compiled compiledTriggerMessage, resp *agentruntime.HTTPMessageResponse) map[string]any {
	fields := map[string]any{
		"org_id":             trigger.OrgID.String(),
		"agent_id":           trigger.AgentID.String(),
		"trigger_id":         trigger.ID.String(),
		"delivery_id":        payload.DeliveryID,
		"event_key":          eventKey(payload.EventType, payload.EventAction),
		"resource_key":       compiled.ResourceKey,
		"runtime_session_id": "",
	}
	if session != nil {
		fields["session_id"] = session.ID.String()
	}
	if resp != nil {
		fields["runtime_session_id"] = resp.SessionID
	}
	return fields
}
