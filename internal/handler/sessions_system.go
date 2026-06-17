package handler

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/logging"
	"github.com/usehivy/hivy/internal/model"
)

type systemSessionRequest struct {
	OrgID             uuid.UUID
	ChannelID         uuid.UUID
	Agent             model.Agent
	Text              string
	Name              string
	Source            string
	SourceID          *uuid.UUID
	SourceResourceKey string
	Raw               model.JSON
}

func (h *SessionHandler) createSystemSession(ctx context.Context, req systemSessionRequest) (*model.Session, *model.SessionEvent, bool, error) {
	if h == nil || h.db == nil {
		return nil, nil, false, fmt.Errorf("session handler not configured")
	}
	text := strings.TrimSpace(req.Text)
	if text == "" {
		return nil, nil, false, fmt.Errorf("text is required")
	}
	if req.OrgID == uuid.Nil {
		return nil, nil, false, fmt.Errorf("org_id is required")
	}
	if req.ChannelID == uuid.Nil {
		return nil, nil, false, fmt.Errorf("channel_id is required")
	}
	if req.Agent.ID == uuid.Nil {
		return nil, nil, false, fmt.Errorf("agent is required")
	}
	source := defaultString(strings.TrimSpace(req.Source), "system")
	resourceKey := strings.TrimSpace(req.SourceResourceKey)
	if resourceKey != "" {
		var existing model.Session
		err := h.db.WithContext(ctx).
			Where("org_id = ? AND source = ? AND source_resource_key = ? AND status = ?",
				req.OrgID, source, resourceKey, "active").
			First(&existing).Error
		if err == nil {
			return &existing, nil, false, nil
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil, false, fmt.Errorf("load existing system session: %w", err)
		}
	}

	sessionID := uuid.New()
	name := strings.TrimSpace(req.Name)
	autoName := name == ""
	if name == "" {
		name = webSessionName(text)
	}
	reasoningEffort, _ := normalizeSessionReasoningEffort("")
	session := model.Session{
		ID:                sessionID,
		OrgID:             req.OrgID,
		ChannelID:         req.ChannelID,
		AgentID:           req.Agent.ID,
		SandboxID:         h.bestEffortSandboxIDForContext(ctx, req.OrgID, req.Agent),
		Model:             defaultString(strings.TrimSpace(req.Agent.Model), ""),
		AccessMode:        "full",
		ReasoningEffort:   reasoningEffort,
		Source:            source,
		SourceID:          req.SourceID,
		SourceResourceKey: defaultString(resourceKey, sessionID.String()),
		Name:              name,
		Status:            "active",
		AgentTurnStatus:   model.SessionAgentTurnIdle,
		IntegrationScopes: model.JSON{},
	}
	var event model.SessionEvent
	err := h.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&session).Error; err != nil {
			return err
		}
		var err error
		raw := req.Raw
		event, err = h.createUserMessageEvent(tx, &session, nil, text, normalizeJSONPtr(&raw))
		return err
	})
	if err != nil {
		return nil, nil, false, fmt.Errorf("create system session: %w", err)
	}
	if _, err := h.dispatchOrQueueSessionDelivery(ctx, session.ID); err != nil {
		return nil, nil, true, fmt.Errorf("queue system session delivery: %w", err)
	}
	if autoName {
		if err := h.enqueueSessionName(ctx, session.ID); err != nil {
			logging.FromContext(ctx).WarnContext(ctx, "enqueue system session name task failed", "session_id", session.ID, "error", err)
		}
	}
	return &session, &event, true, nil
}
