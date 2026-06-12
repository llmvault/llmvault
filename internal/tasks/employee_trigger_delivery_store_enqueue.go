package tasks

import (
	"context"
	"fmt"

	"github.com/usehivy/hivy/internal/employeeruntime"
	"github.com/usehivy/hivy/internal/logging"
	"github.com/usehivy/hivy/internal/model"
)

func (h *EmployeeTriggerDispatchHandler) enqueueStoreDelivery(ctx context.Context, payload EmployeeTriggerDispatchPayload, trigger model.EmployeeTrigger, conv *model.EmployeeSession, compiled compiledTriggerMessage, resp *employeeruntime.HTTPMessageResponse) {
	if h.enqueuer == nil {
		logging.CaptureWithFields(ctx, fmt.Errorf("employee trigger delivery store enqueue skipped: enqueuer is nil"), triggerStoreEnqueueFields(payload, trigger, conv, compiled, resp))
		return
	}
	if resp == nil {
		resp = &employeeruntime.HTTPMessageResponse{}
	}
	task, opts, err := NewEmployeeTriggerStoreDeliveryTask(EmployeeTriggerStoreDeliveryPayload{
		OrgID:                 trigger.OrgID,
		EmployeeID:            trigger.EmployeeID,
		TriggerID:             trigger.ID,
		ConnectionID:          trigger.ConnectionID,
		DeliveryID:            payload.DeliveryID,
		EventKey:              eventKey(payload.EventType, payload.EventAction),
		ResourceKey:           compiled.ResourceKey,
		ConversationID:        conv.ID,
		RuntimeConversationID: conv.RuntimeConversationID,
		RuntimeSessionID:      resp.SessionID,
		RuntimeStreamID:       resp.StreamID,
		RuntimeTraceID:        resp.TraceID,
		RuntimeTurnID:         resp.TurnID,
		PayloadJSON:           payload.PayloadJSON,
	})
	if err != nil {
		logging.CaptureWithFields(ctx, fmt.Errorf("build employee trigger delivery store task: %w", err), triggerStoreEnqueueFields(payload, trigger, conv, compiled, resp))
		return
	}
	if _, err := h.enqueuer.EnqueueContext(ctx, task, opts...); err != nil {
		logging.CaptureWithFields(ctx, fmt.Errorf("enqueue employee trigger delivery store task: %w", err), triggerStoreEnqueueFields(payload, trigger, conv, compiled, resp))
	}
}

func triggerStoreEnqueueFields(payload EmployeeTriggerDispatchPayload, trigger model.EmployeeTrigger, conv *model.EmployeeSession, compiled compiledTriggerMessage, resp *employeeruntime.HTTPMessageResponse) map[string]any {
	fields := map[string]any{
		"org_id":                  trigger.OrgID.String(),
		"employee_id":             trigger.EmployeeID.String(),
		"trigger_id":              trigger.ID.String(),
		"delivery_id":             payload.DeliveryID,
		"event_key":               eventKey(payload.EventType, payload.EventAction),
		"resource_key":            compiled.ResourceKey,
		"runtime_conversation_id": "",
		"runtime_session_id":      "",
	}
	if conv != nil {
		fields["conversation_id"] = conv.ID.String()
		fields["runtime_conversation_id"] = conv.RuntimeConversationID
	}
	if resp != nil {
		fields["runtime_session_id"] = resp.SessionID
	}
	return fields
}
