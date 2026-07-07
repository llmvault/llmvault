package memory

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"github.com/google/uuid"

	"github.com/usehivy/hivy/internal/model"
)

type memorySearchArgs struct {
	Query         string   `json:"query"`
	Tags          []string `json:"tags"`
	IncludeFacts  bool     `json:"include_facts"`
	HivySessionID string   `json:"_hivy_session_id"`
}

// memoryToolContext is the per-call scope for the regular agent tools. Every
// call runs inside a session, and the session's channel is the only memory
// scope the agent may read: sessionChannelID plus (when the channel allows
// it) org-wide memories. Agents never write memory through tools.
type memoryToolContext struct {
	OrgID             uuid.UUID
	AgentID           uuid.UUID
	SessionID         uuid.UUID
	ChannelID         uuid.UUID
	ExposeOrgMemories bool
	UserID            *uuid.UUID
}

var memoryToolTagRE = regexp.MustCompile(`^[a-z0-9]([a-z0-9-]{0,62}[a-z0-9])?$`)

func memoryToolAgentProxy(token *model.Token) bool {
	if token == nil || token.Meta == nil {
		return false
	}
	tokenType, _ := token.Meta[model.TokenMetaType].(string)
	return tokenType == model.TokenTypeAgentProxy
}

func memoryToolAgentID(token *model.Token) (uuid.UUID, error) {
	agentIDText, _ := token.Meta[model.TokenMetaAgentID].(string)
	agentID, err := uuid.Parse(strings.TrimSpace(agentIDText))
	if err != nil || agentID == uuid.Nil {
		return uuid.Nil, fmt.Errorf("agent proxy token is missing agent_id")
	}
	return agentID, nil
}

// memoryToolContext resolves the session-derived scope. A valid session is
// required: the session names the channel, and the channel's ExposeOrgMemories
// flag decides whether org-wide memories fold into the agent's recall.
func (s *Service) memoryToolContext(ctx context.Context, token *model.Token, agentID uuid.UUID, rawSessionID string) (memoryToolContext, error) {
	toolCtx := memoryToolContext{OrgID: token.OrgID, AgentID: agentID}
	sessionIDText := strings.TrimSpace(rawSessionID)
	if sessionIDText == "" {
		return toolCtx, fmt.Errorf("_hivy_session_id is required to scope memories to this channel")
	}
	sessionID, err := uuid.Parse(sessionIDText)
	if err != nil || sessionID == uuid.Nil {
		return toolCtx, fmt.Errorf("_hivy_session_id must be a valid UUID")
	}
	var session model.Session
	if err := s.cfg.DB.WithContext(ctx).
		Where("id = ? AND org_id = ? AND agent_id = ?", sessionID, token.OrgID, agentID).
		First(&session).Error; err != nil {
		return toolCtx, fmt.Errorf("session not found for this agent")
	}
	var channel model.Channel
	if err := s.cfg.DB.WithContext(ctx).
		Select("id", "expose_org_memories").
		Where("id = ? AND org_id = ?", session.ChannelID, token.OrgID).
		First(&channel).Error; err != nil {
		return toolCtx, fmt.Errorf("session channel not found")
	}
	toolCtx.SessionID = sessionID
	toolCtx.ChannelID = channel.ID
	toolCtx.ExposeOrgMemories = channel.ExposeOrgMemories
	if session.CreatedBy != nil && *session.CreatedBy != uuid.Nil {
		toolCtx.UserID = session.CreatedBy
	}
	return toolCtx, nil
}

// searchScope is the read scope for this channel: the channel itself, folding
// in org-wide memories only when the channel exposes them.
func (toolCtx memoryToolContext) searchScope() ChannelScope {
	return ChannelScope{ChannelID: &toolCtx.ChannelID, IncludeOrgMemories: toolCtx.ExposeOrgMemories}
}
