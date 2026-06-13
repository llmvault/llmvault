package tasks

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/google/uuid"
	"gorm.io/gorm/clause"

	"github.com/usehivy/hivy/internal/gateway"
	"github.com/usehivy/hivy/internal/model"
)

func (s *GatewayStreamDeliveryService) recordDelivery(ctx context.Context, payload GatewayStreamPayload, text string, handles []gateway.MessageHandle, status, errText string) error {
	if s == nil || s.db == nil {
		return nil
	}
	orgID, _ := parseUUID(payload.OrgID)
	agentID, _ := parseUUID(payload.AgentID)
	sessionID, _ := parseUUID(payload.SessionID)
	routeID, _ := parseUUID(payload.RouteID)
	dedupe := deliveryDedupeKey(payload, text)
	row := model.AgentGatewayDelivery{
		OrgID:            orgID,
		AgentID:          agentID,
		RouteID:          routeIDPtr(routeID),
		AgentSessionID:   sessionID,
		Provider:         firstNonEmpty(payload.Provider, sinkProvider(handles)),
		DedupeKey:        dedupe,
		RuntimeSessionID: payload.RuntimeSessionID,
		RuntimeTraceID:   payload.TraceID,
		RuntimeTurnID:    payload.TurnID,
		ThreadKey:        payload.ThreadKey,
		ChannelID:        payload.ChannelID,
		ThreadID:         payload.ThreadID,
		ResponseText:     text,
		ProviderHandles:  handlesJSON(handles),
		Status:           status,
		Error:            errText,
	}
	if len(handles) > 0 {
		row.ChannelID = handles[0].ChannelID
		row.ThreadID = handles[0].ThreadID
	}
	return s.db.WithContext(ctx).Clauses(clause.OnConflict{DoNothing: true}).Create(&row).Error
}

// deliveryDedupeKey derives the stable dedupe identity, preferring the runtime
// trace+turn (stable across retries) and falling back to session + text hash.
func deliveryDedupeKey(payload GatewayStreamPayload, text string) string {
	dedupe := payload.TraceID + ":" + payload.TurnID
	if strings.Trim(dedupe, ":") == "" {
		dedupe = payload.RuntimeSessionID + ":" + hashDeliveryText(text)
	}
	return dedupe
}

// alreadyDelivered reports whether a prior attempt already sent this turn,
// returning the existing row so the caller can reuse its handles/text. Only
// rows with status "sent" count — a prior "failed" row must be retried.
func (s *GatewayStreamDeliveryService) alreadyDelivered(ctx context.Context, payload GatewayStreamPayload, text string) (model.AgentGatewayDelivery, bool) {
	if s == nil || s.db == nil {
		return model.AgentGatewayDelivery{}, false
	}
	dedupe := deliveryDedupeKey(payload, text)
	if strings.Trim(dedupe, ":") == "" {
		return model.AgentGatewayDelivery{}, false
	}
	var row model.AgentGatewayDelivery
	err := s.db.WithContext(ctx).
		Where("dedupe_key = ? AND status = ?", dedupe, "sent").
		Take(&row).Error
	if err != nil {
		return model.AgentGatewayDelivery{}, false
	}
	return row, true
}

func decodeHandles(raw model.RawJSON) []gateway.MessageHandle {
	if len(raw) == 0 {
		return nil
	}
	var handles []gateway.MessageHandle
	if err := json.Unmarshal(raw, &handles); err != nil {
		return nil
	}
	return handles
}

func eventText(event gateway.SSEEvent) string {
	var data struct {
		Text string `json:"text"`
	}
	if err := json.Unmarshal(event.Data, &data); err != nil {
		return ""
	}
	return data.Text
}

func routeIDPtr(id uuid.UUID) *uuid.UUID {
	if id == uuid.Nil {
		return nil
	}
	return &id
}

func handlesJSON(handles []gateway.MessageHandle) model.RawJSON {
	if handles == nil {
		handles = []gateway.MessageHandle{}
	}
	encoded, err := json.Marshal(handles)
	if err != nil {
		return model.RawJSON("[]")
	}
	return model.RawJSON(encoded)
}

func hashDeliveryText(text string) string {
	return gateway.HashExternalGatewaySecret(text)[:16]
}

func sinkProvider(handles []gateway.MessageHandle) string {
	if len(handles) == 0 {
		return ""
	}
	if provider, ok := handles[0].Raw["provider"].(string); ok {
		return provider
	}
	return ""
}
