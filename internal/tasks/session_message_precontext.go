package tasks

import (
	"context"
	"fmt"
	"strings"

	"github.com/usehivy/hivy/internal/logging"
	"github.com/usehivy/hivy/internal/model"
	"github.com/usehivy/hivy/internal/precontext"
)

func (h *SessionMessageDeliverHandler) WithPreContextBuilder(builder precontext.Builder) *SessionMessageDeliverHandler {
	if h == nil {
		return nil
	}
	h.preContext = builder
	return h
}

func (h *SessionMessageDeliverHandler) commandWithPreContext(ctx context.Context, session model.Session, command SessionMessageCommand) SessionMessageCommand {
	if h == nil || h.preContext == nil {
		return command
	}
	sections, err := h.preContext.Build(ctx, preContextRequest(session, command))
	if err != nil {
		logging.Capture(ctx, fmt.Errorf("build agent precontext: %w", err))
		return command
	}
	if len(sections) == 0 {
		return command
	}
	if command.Payload == nil {
		command.Payload = model.JSON{}
	}
	existing := stringSlice(command.Payload["dynamic_context"])
	command.Payload["dynamic_context"] = append(existing, sections...)
	if err := h.persistCommandPayload(ctx, session, command); err != nil {
		logging.Capture(ctx, fmt.Errorf("persist agent precontext payload: %w", err))
	}
	return command
}

func preContextRequest(session model.Session, command SessionMessageCommand) precontext.Request {
	payload := map[string]any(command.Payload)
	text := strings.TrimSpace(command.Text)
	if text == "" {
		text, _ = payload["text"].(string)
	}
	user, _ := payload["user"].(string)
	display, _ := payload["user_display_name"].(string)
	if strings.TrimSpace(user) == "" && command.ActorUserID != nil {
		user = command.ActorUserID.String()
	}
	return precontext.Request{
		OrgID:            session.OrgID,
		AgentID:          session.AgentID,
		CurrentSessionID: session.ID,
		Text:             strings.TrimSpace(text),
		UserID:           strings.TrimSpace(user),
		UserDisplayName:  strings.TrimSpace(display),
		Source:           strings.TrimSpace(session.Source),
	}
}

func (h *SessionMessageDeliverHandler) persistCommandPayload(ctx context.Context, session model.Session, command SessionMessageCommand) error {
	if h == nil || h.db == nil || command.EventID == nil {
		return nil
	}
	if err := h.db.WithContext(ctx).Model(&model.SessionEvent{}).
		Where("id = ? AND org_id = ?", *command.EventID, session.OrgID).
		Update("payload", command.Payload).Error; err != nil {
		return err
	}
	return h.db.WithContext(ctx).Model(&model.SessionMessageQueue{}).
		Where("session_event_id = ?", *command.EventID).
		Update("message_payload", command.Payload).Error
}
