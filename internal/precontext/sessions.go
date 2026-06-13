package precontext

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/model"
)

func (s *Service) fetchSessionsSection(ctx context.Context, req Request) (string, error) {
	if s.cfg.DB == nil || req.OrgID == uuid.Nil || req.AgentID == uuid.Nil {
		return "", nil
	}
	sessions, err := s.loadRecentSessions(ctx, req)
	if err != nil {
		return "", err
	}
	if len(sessions) == 0 {
		return "", nil
	}
	ids := make([]uuid.UUID, 0, len(sessions))
	for _, session := range sessions {
		ids = append(ids, session.ID)
	}
	var events []model.SessionEvent
	if err := s.cfg.DB.WithContext(ctx).
		Where("session_id IN ? AND event_type IN ?", ids, []string{"user.message.received", "agent.message.sent"}).
		Order("event_at ASC, created_at ASC").
		Find(&events).Error; err != nil {
		return "", fmt.Errorf("load recent session events: %w", err)
	}
	eventsBySession := map[uuid.UUID][]model.SessionEvent{}
	for _, event := range events {
		eventsBySession[event.SessionID] = append(eventsBySession[event.SessionID], event)
	}
	var lines []string
	for _, session := range sessions {
		summary := formatSessionSummary(session, eventsBySession[session.ID])
		if summary != "" {
			lines = append(lines, summary)
		}
	}
	return section("## Recent sessions", strings.Join(lines, "\n"), SessionsBudgetBytes), nil
}

func (s *Service) loadRecentSessions(ctx context.Context, req Request) ([]model.Session, error) {
	query := s.cfg.DB.WithContext(ctx).
		Where("org_id = ? AND agent_id = ? AND status <> ?", req.OrgID, req.AgentID, "error")
	if req.CurrentSessionID != uuid.Nil {
		query = query.Where("id <> ?", req.CurrentSessionID)
	}
	var sessions []model.Session
	if err := query.Order("updated_at DESC, created_at DESC").Limit(10).Find(&sessions).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, nil
		}
		return nil, fmt.Errorf("load recent sessions: %w", err)
	}
	return sessions, nil
}

func formatSessionSummary(session model.Session, events []model.SessionEvent) string {
	user, modelReply := latestUserAndModel(events)
	if user == "" && modelReply == "" {
		return ""
	}
	name := cleanText(session.Name)
	if name == "" {
		name = cleanText(session.SourceResourceKey)
	}
	if name == "" {
		name = session.ID.String()
	}
	line := "- " + trimToBytes(name, 96)
	if timestamp := sessionTimestamp(session); timestamp != "" {
		line += " [" + timestamp + "]"
	}
	if user != "" {
		line += "\n  User: " + trimToBytes(user, 180)
	}
	if modelReply != "" {
		line += "\n  Hivy: " + trimToBytes(modelReply, 180)
	}
	return trimToBytes(line, 430)
}

func sessionTimestamp(session model.Session) string {
	when := session.UpdatedAt
	if when.IsZero() {
		when = session.CreatedAt
	}
	if when.IsZero() {
		return ""
	}
	return when.UTC().Format(time.RFC3339)
}

func latestUserAndModel(events []model.SessionEvent) (string, string) {
	user := ""
	modelReply := ""
	for _, event := range events {
		payload := map[string]any(event.Payload)
		text := firstString(payload["text"], payload["message"], payload["content"], payload["markdown"])
		if text == "" {
			continue
		}
		switch event.EventType {
		case "user.message.received":
			user = text
		case "agent.message.sent":
			modelReply = text
		}
	}
	return user, modelReply
}
