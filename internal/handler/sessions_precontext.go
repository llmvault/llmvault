package handler

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/uuid"

	"github.com/usehivy/hivy/internal/logging"
	"github.com/usehivy/hivy/internal/model"
	"github.com/usehivy/hivy/internal/precontext"
)

func (h *SessionHandler) buildInitialSessionContext(ctx context.Context, session model.Session, actor *uuid.UUID, text string, payload model.JSON) []string {
	if h == nil || h.preContext == nil {
		return nil
	}
	req := initialSessionContextRequest(session, actor, text, payload)
	req.ChannelID = session.ChannelID
	var ch model.Channel
	if err := h.db.WithContext(ctx).Select("expose_org_memories").First(&ch, "id = ?", session.ChannelID).Error; err == nil {
		req.IncludeOrgMemories = ch.ExposeOrgMemories
	}
	sections, err := h.preContext.Build(ctx, req)
	if err != nil {
		logging.Capture(ctx, fmt.Errorf("build initial session context: %w", err))
		return nil
	}
	return nonemptySessionContextSections(sections)
}

func initialSessionContextRequest(session model.Session, actor *uuid.UUID, text string, payload model.JSON) precontext.Request {
	if strings.TrimSpace(text) == "" && payload != nil {
		text, _ = payload["text"].(string)
	}
	user := ""
	display := ""
	if payload != nil {
		user, _ = payload["user"].(string)
		display, _ = payload["user_display_name"].(string)
	}
	if strings.TrimSpace(user) == "" && actor != nil {
		user = actor.String()
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

func nonemptySessionContextSections(sections []string) []string {
	out := make([]string, 0, len(sections))
	for _, section := range sections {
		if section = strings.TrimSpace(section); section != "" {
			out = append(out, section)
		}
	}
	return out
}
