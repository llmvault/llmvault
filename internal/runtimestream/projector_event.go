package runtimestream

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/usehivy/hivy/internal/model"
)

func sessionEventFromRuntimeEvent(event Event) (model.SessionEvent, error) {
	event.Normalize()
	if err := event.Validate(); err != nil {
		return model.SessionEvent{}, err
	}
	sessionID, err := uuid.Parse(event.SessionID)
	if err != nil {
		return model.SessionEvent{}, fmt.Errorf("parse session id: %w", err)
	}
	orgID, err := uuid.Parse(event.OrgID)
	if err != nil {
		return model.SessionEvent{}, fmt.Errorf("parse org id: %w", err)
	}
	agentID, err := uuid.Parse(event.AgentID)
	if err != nil {
		return model.SessionEvent{}, fmt.Errorf("parse agent id: %w", err)
	}
	var sandboxID *uuid.UUID
	if strings.TrimSpace(event.SandboxID) != "" {
		id, err := uuid.Parse(event.SandboxID)
		if err != nil {
			return model.SessionEvent{}, fmt.Errorf("parse sandbox id: %w", err)
		}
		sandboxID = &id
	}
	var actorUserID *uuid.UUID
	if strings.TrimSpace(event.ActorUserID) != "" {
		id, err := uuid.Parse(event.ActorUserID)
		if err != nil {
			return model.SessionEvent{}, fmt.Errorf("parse actor user id: %w", err)
		}
		actorUserID = &id
	}
	runtimeSeq := event.RuntimeSeq
	payload := model.JSON{}
	for key, value := range event.Payload {
		payload[key] = value
	}
	return model.SessionEvent{
		OrgID:            orgID,
		SessionID:        sessionID,
		AgentID:          agentID,
		SandboxID:        sandboxID,
		RuntimeSessionID: event.SessionID,
		EventID:          event.EventID,
		EventType:        event.EventType,
		ActorUserID:      actorUserID,
		Source:           defaultString(event.Source, "runtime"),
		SequenceNumber:   event.RuntimeSeq,
		RuntimeSeq:       &runtimeSeq,
		RuntimeEventID:   event.EventID,
		TurnID:           event.TurnID,
		SpanID:           event.SpanID,
		Durability:       event.Durability,
		Payload:          payload,
		EventAt:          event.OccurredAt,
	}, nil
}

func runtimeSeqOnConflict() clause.OnConflict {
	return clause.OnConflict{
		Columns:     []clause.Column{{Name: "session_id"}, {Name: "runtime_seq"}},
		TargetWhere: clause.Where{Exprs: []clause.Expression{gorm.Expr("runtime_seq IS NOT NULL")}},
		DoNothing:   true,
	}
}

func defaultString(value, fallback string) string {
	if strings.TrimSpace(value) != "" {
		return strings.TrimSpace(value)
	}
	return fallback
}

func sleepOrDone(ctx context.Context, d time.Duration) {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
	case <-timer.C:
	}
}
