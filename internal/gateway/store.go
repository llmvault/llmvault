package gateway

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/usehivy/hivy/internal/employeesandbox"
	"github.com/usehivy/hivy/internal/model"
)

func (s *Service) loadActiveSessionByRuntimeID(ctx context.Context, runtimeID string) (model.EmployeeSession, bool, error) {
	var session model.EmployeeSession
	err := s.db.WithContext(ctx).
		Where("runtime_conversation_id = ? AND source = ? AND status = ?", runtimeID, Source, "active").
		First(&session).Error
	if err == nil {
		return session, true, nil
	}
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return model.EmployeeSession{}, false, nil
	}
	return model.EmployeeSession{}, false, err
}

func (s *Service) insertInboundEvent(ctx context.Context, route model.EmployeeGatewayRoute, inbound InboundEnvelope) (model.EmployeeGatewayEvent, bool, error) {
	event := model.EmployeeGatewayEvent{
		OrgID:             route.OrgID,
		EmployeeID:        route.EmployeeID,
		RouteID:           routeIDPtr(route.ID),
		Provider:          route.Provider,
		ExternalMessageID: inbound.ExternalMessageID,
		DedupeKey:         inbound.DedupeKey,
		ThreadKey:         inbound.ThreadKey,
		ChannelID:         inbound.ChannelID,
		ThreadID:          inbound.ThreadID,
		SenderID:          inbound.SenderID,
		Status:            "received",
		Payload:           rawJSON(inbound.Raw, "{}"),
		ReceivedAt:        inbound.ReceivedAt.UTC(),
	}
	result := s.db.WithContext(ctx).Clauses(clause.OnConflict{DoNothing: true}).Create(&event)
	if result.Error != nil {
		return event, false, fmt.Errorf("insert gateway event: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		var existing model.EmployeeGatewayEvent
		query := s.db.WithContext(ctx)
		if route.ID == uuid.Nil {
			query = query.Where("route_id IS NULL AND org_id = ? AND dedupe_key = ?", route.OrgID, inbound.DedupeKey)
		} else {
			query = query.Where("route_id = ? AND dedupe_key = ?", route.ID, inbound.DedupeKey)
		}
		if err := query.First(&existing).Error; err != nil {
			return existing, true, err
		}
		// A prior row marked "failed" means delivery to the runtime never
		// succeeded (sandbox waking, runtime restart, post timeout). The provider
		// retries the webhook, so re-claim the row and re-run Send instead of
		// dropping the message as a duplicate. Successful/in-flight rows
		// ("received"/"delivered") stay deduped.
		if existing.Status == "failed" {
			if err := s.db.WithContext(ctx).Model(&model.EmployeeGatewayEvent{}).
				Where("id = ? AND status = ?", existing.ID, "failed").
				Updates(map[string]any{"status": "received", "error": ""}).Error; err != nil {
				return existing, true, fmt.Errorf("reclaim failed gateway event: %w", err)
			}
			existing.Status = "received"
			existing.Error = ""
			return existing, false, nil
		}
		return existing, true, nil
	}
	return event, false, nil
}

func (s *Service) findOrCreateSession(ctx context.Context, route model.EmployeeGatewayRoute, threadKey string) (model.EmployeeSession, string, bool, error) {
	conversationID := stableConversationID(route.ID, threadKey)
	sessionID := runtimeSessionID(conversationID)
	sandbox, err := s.employeeSandboxSelector().MainRuntime(ctx, route.OrgID, route.EmployeeID)
	if err != nil {
		return model.EmployeeSession{}, "", false, fmt.Errorf("load employee sandbox: %w", err)
	}
	sourceID := route.ID
	session := model.EmployeeSession{}
	created := false
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		err := tx.Where("org_id = ? AND employee_id = ? AND source = ? AND source_id = ? AND source_resource_key = ? AND status = ?",
			route.OrgID, route.EmployeeID, Source, route.ID, threadKey, "active").
			First(&session).Error
		if err == nil {
			if session.SandboxID != sandbox.ID {
				if err := tx.Model(&session).Update("sandbox_id", sandbox.ID).Error; err != nil {
					return err
				}
				session.SandboxID = sandbox.ID
			}
			return nil
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		session = model.EmployeeSession{
			OrgID:                 route.OrgID,
			EmployeeID:            route.EmployeeID,
			SandboxID:             sandbox.ID,
			RuntimeConversationID: sessionID,
			Source:                Source,
			SourceID:              &sourceID,
			SourceResourceKey:     threadKey,
			Status:                "active",
			Name:                  "Gateway: " + threadKey,
			IntegrationScopes:     model.JSON{},
		}
		if err := tx.Create(&session).Error; err != nil {
			return err
		}
		created = true
		return nil
	})
	if err != nil {
		return model.EmployeeSession{}, "", false, fmt.Errorf("find or create gateway session: %w", err)
	}
	return session, conversationID, created, nil
}

func (s *Service) employeeSandboxSelector() employeesandbox.Selector {
	return employeesandbox.Selector{
		DB:                     s.db,
		EmployeeRuntimeImage:   s.employeeRuntimeImage,
		SpecialistRuntimeImage: s.specialistRuntimeImage,
	}
}

func (s *Service) markEventDelivered(ctx context.Context, eventID, sessionID uuid.UUID, conversationID string, delivery *RuntimeDelivery) error {
	updates := map[string]any{
		"employee_session_id":     sessionID,
		"status":                  "delivered",
		"processed_at":            s.now().UTC(),
		"runtime_conversation_id": conversationID,
		"runtime_session_id":      delivery.SessionID,
		"runtime_stream_id":       delivery.StreamID,
		"runtime_trace_id":        delivery.TraceID,
		"runtime_turn_id":         delivery.TurnID,
	}
	return s.db.WithContext(ctx).Model(&model.EmployeeGatewayEvent{}).Where("id = ?", eventID).Updates(updates).Error
}

func (s *Service) markEventFailed(ctx context.Context, eventID uuid.UUID, err error) error {
	return s.db.WithContext(ctx).Model(&model.EmployeeGatewayEvent{}).
		Where("id = ?", eventID).
		Updates(map[string]any{"status": "failed", "error": err.Error(), "processed_at": s.now().UTC()}).Error
}

func (s *Service) loadDeliveryByDedupe(ctx context.Context, routeID uuid.UUID, dedupe string) (*model.EmployeeGatewayDelivery, bool, error) {
	var delivery model.EmployeeGatewayDelivery
	var err error
	if routeID == uuid.Nil {
		err = s.db.WithContext(ctx).Where("route_id IS NULL AND dedupe_key = ?", dedupe).First(&delivery).Error
	} else {
		err = s.db.WithContext(ctx).Where("route_id = ? AND dedupe_key = ?", routeID, dedupe).First(&delivery).Error
	}
	if err == nil {
		return &delivery, true, nil
	}
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, false, nil
	}
	return nil, false, err
}

func (s *Service) loadLatestEventForSession(ctx context.Context, sessionID uuid.UUID) (model.EmployeeGatewayEvent, bool, error) {
	var event model.EmployeeGatewayEvent
	err := s.db.WithContext(ctx).
		Where("employee_session_id = ?", sessionID).
		Order("received_at DESC, created_at DESC").
		First(&event).Error
	if err == nil {
		return event, true, nil
	}
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return model.EmployeeGatewayEvent{}, false, nil
	}
	return model.EmployeeGatewayEvent{}, false, fmt.Errorf("load latest gateway event: %w", err)
}

// upsertDelivery records the delivery outcome. When a prior delivery row exists
// for the dedupe key (always a "failed" row at this point — successful rows
// short-circuit earlier) it updates that row in place rather than inserting a
// duplicate, which would violate the (route_id, dedupe_key) unique index. This is
// what lets a transient send failure be retried instead of permanently dropping
// the reply under the occupied dedupe key.
func (s *Service) upsertDelivery(ctx context.Context, route model.EmployeeGatewayRoute, session model.EmployeeSession, response AgentResponse, dedupe string, existing *model.EmployeeGatewayDelivery, handles []MessageHandle, status string, errText string) (*model.EmployeeGatewayDelivery, error) {
	if existing == nil {
		return s.insertDelivery(ctx, route, session, response, dedupe, handles, status, errText)
	}

	channelID := response.ChannelID
	threadID := response.ThreadID
	if len(handles) > 0 {
		channelID = handles[0].ChannelID
		threadID = handles[0].ThreadID
	}
	updates := map[string]any{
		"runtime_session_id": response.RuntimeSessionID,
		"runtime_trace_id":   response.TraceID,
		"runtime_turn_id":    response.TurnID,
		"response_text":      response.Text,
		"provider_handles":   handlesJSON(handles),
		"status":             status,
		"error":              errText,
		"channel_id":         channelID,
		"thread_id":          threadID,
	}
	if err := s.db.WithContext(ctx).Model(&model.EmployeeGatewayDelivery{}).
		Where("id = ?", existing.ID).
		Updates(updates).Error; err != nil {
		return nil, fmt.Errorf("update gateway delivery: %w", err)
	}
	existing.RuntimeSessionID = response.RuntimeSessionID
	existing.RuntimeTraceID = response.TraceID
	existing.RuntimeTurnID = response.TurnID
	existing.ResponseText = response.Text
	existing.ProviderHandles = handlesJSON(handles)
	existing.Status = status
	existing.Error = errText
	existing.ChannelID = channelID
	existing.ThreadID = threadID
	return existing, nil
}

func (s *Service) insertDelivery(ctx context.Context, route model.EmployeeGatewayRoute, session model.EmployeeSession, response AgentResponse, dedupe string, handles []MessageHandle, status string, errText string) (*model.EmployeeGatewayDelivery, error) {
	row := model.EmployeeGatewayDelivery{
		OrgID:             route.OrgID,
		EmployeeID:        route.EmployeeID,
		RouteID:           routeIDPtr(route.ID),
		EmployeeSessionID: session.ID,
		Provider:          route.Provider,
		DedupeKey:         dedupe,
		RuntimeSessionID:  response.RuntimeSessionID,
		RuntimeTraceID:    response.TraceID,
		RuntimeTurnID:     response.TurnID,
		ThreadKey:         session.SourceResourceKey,
		ResponseText:      response.Text,
		ProviderHandles:   handlesJSON(handles),
		Status:            status,
		Error:             errText,
		ChannelID:         response.ChannelID,
		ThreadID:          response.ThreadID,
	}
	if len(handles) > 0 {
		row.ChannelID = handles[0].ChannelID
		row.ThreadID = handles[0].ThreadID
	}
	if err := s.db.WithContext(ctx).Create(&row).Error; err != nil {
		return nil, fmt.Errorf("insert gateway delivery: %w", err)
	}
	return &row, nil
}

func routeIDPtr(id uuid.UUID) *uuid.UUID {
	if id == uuid.Nil {
		return nil
	}
	return &id
}
