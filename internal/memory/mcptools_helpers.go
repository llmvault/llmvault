package memory

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"github.com/google/uuid"

	"github.com/usehivy/hivy/internal/model"
)

type memoryToolTarget struct {
	Owner      string `json:"owner"`
	Visibility string `json:"visibility"`
	AgentID    string `json:"agent_id"`
}

type memorySearchArgs struct {
	Query         string           `json:"query"`
	Target        memoryToolTarget `json:"target"`
	Tags          []string         `json:"tags"`
	HivySessionID string           `json:"_hivy_session_id"`
}

type memoryRetainArgs struct {
	Content       string           `json:"content"`
	Target        memoryToolTarget `json:"target"`
	Tags          []string         `json:"tags"`
	Metadata      model.JSON       `json:"metadata"`
	HivySessionID string           `json:"_hivy_session_id"`
}

type memoryForgetArgs struct {
	MemoryID      string `json:"memory_id"`
	Reason        string `json:"reason"`
	UserApproved  bool   `json:"user_approved"`
	HivySessionID string `json:"_hivy_session_id"`
}

type memoryToolContext struct {
	OrgID     uuid.UUID
	AgentID   uuid.UUID
	SessionID *uuid.UUID
	UserID    *uuid.UUID
}

type resolvedMemoryTarget struct {
	Scope      string
	UserID     *uuid.UUID
	AgentID    *uuid.UUID
	Visibility string
	Response   map[string]any
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

func (s *Service) memoryToolContext(ctx context.Context, token *model.Token, agentID uuid.UUID, rawSessionID string) (memoryToolContext, error) {
	toolCtx := memoryToolContext{OrgID: token.OrgID, AgentID: agentID}
	var agent model.Agent
	if err := s.cfg.DB.WithContext(ctx).
		Where("id = ? AND org_id = ? AND status <> ?", agentID, token.OrgID, "archived").
		First(&agent).Error; err != nil {
		return toolCtx, fmt.Errorf("agent is not active in this org")
	}
	sessionIDText := strings.TrimSpace(rawSessionID)
	if sessionIDText == "" {
		return toolCtx, nil
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
	toolCtx.SessionID = &sessionID
	if session.CreatedBy != nil && *session.CreatedBy != uuid.Nil {
		toolCtx.UserID = session.CreatedBy
	}
	return toolCtx, nil
}

func normalizeMemorySearchTarget(target memoryToolTarget, toolCtx memoryToolContext) (resolvedMemoryTarget, error) {
	owner := strings.ToLower(strings.TrimSpace(target.Owner))
	if owner == "" {
		owner = memoryToolOwnerBoth
	}
	visibility := normalizeMemoryVisibility(target.Visibility, AgentVisibilityBoth)
	if visibility != AgentVisibilityAllAgents && visibility != AgentVisibilityThisAgent && visibility != AgentVisibilityBoth {
		return resolvedMemoryTarget{}, fmt.Errorf("target.visibility must be all_agents, this_agent, or both")
	}
	out := resolvedMemoryTarget{
		Visibility: visibility,
		Response:   memoryToolTargetResponse(owner, visibility),
	}
	switch owner {
	case memoryToolOwnerBoth:
		out.UserID = toolCtx.UserID
	case memoryToolOwnerOrg:
		out.Scope = model.AgentMemoryScopeOrg
	case memoryToolOwnerUser:
		if toolCtx.UserID == nil {
			return out, fmt.Errorf("user memory search requires _hivy_session_id for a session created by a user")
		}
		out.Scope = model.AgentMemoryScopeUser
		out.UserID = toolCtx.UserID
	default:
		return out, fmt.Errorf("target.owner must be org, user, or both")
	}
	return out, nil
}

func (s *Service) normalizeMemoryRetainTarget(ctx context.Context, target memoryToolTarget, toolCtx memoryToolContext) (resolvedMemoryTarget, error) {
	owner := strings.ToLower(strings.TrimSpace(target.Owner))
	visibility := normalizeMemoryVisibility(target.Visibility, "")
	agentIDText := strings.TrimSpace(target.AgentID)
	if owner != memoryToolOwnerOrg && owner != memoryToolOwnerUser {
		return resolvedMemoryTarget{}, fmt.Errorf("target.owner must be org or user")
	}
	if visibility != "" && agentIDText != "" {
		return resolvedMemoryTarget{}, fmt.Errorf("the target cannot include both visibility and agent_id (pick one)")
	}
	if agentIDText != "" {
		return s.normalizeMemoryRetainAgentTarget(ctx, owner, agentIDText, toolCtx)
	}
	if visibility != AgentVisibilityAllAgents && visibility != AgentVisibilityThisAgent {
		return resolvedMemoryTarget{}, fmt.Errorf("target.visibility must be all_agents or this_agent")
	}
	out := resolvedMemoryTarget{
		Visibility: visibility,
		Response:   memoryToolTargetResponse(owner, visibility),
	}
	if owner == memoryToolOwnerOrg {
		out.Scope = model.AgentMemoryScopeOrg
	} else {
		if toolCtx.UserID == nil {
			return out, fmt.Errorf("user memory target requires _hivy_session_id for a session created by a user")
		}
		out.Scope = model.AgentMemoryScopeUser
		out.UserID = toolCtx.UserID
	}
	if visibility == AgentVisibilityThisAgent {
		out.AgentID = &toolCtx.AgentID
	}
	return out, nil
}

func (s *Service) normalizeMemoryRetainAgentTarget(ctx context.Context, owner, agentIDText string, toolCtx memoryToolContext) (resolvedMemoryTarget, error) {
	if owner != memoryToolOwnerOrg {
		return resolvedMemoryTarget{}, fmt.Errorf("target.agent_id requires target.owner org")
	}
	targetAgentID, err := uuid.Parse(agentIDText)
	if err != nil || targetAgentID == uuid.Nil {
		return resolvedMemoryTarget{}, fmt.Errorf("target.agent_id must be a valid UUID")
	}
	var count int64
	if err := s.cfg.DB.WithContext(ctx).Model(&model.Agent{}).
		Where("id = ? AND org_id = ? AND status <> ?", targetAgentID, toolCtx.OrgID, "archived").
		Count(&count).Error; err != nil {
		return resolvedMemoryTarget{}, fmt.Errorf("validate target agent: %w", err)
	}
	if count == 0 {
		return resolvedMemoryTarget{}, fmt.Errorf("target.agent_id must be an active agent in this org")
	}
	response := memoryToolTargetResponse(memoryToolOwnerOrg, AgentVisibilityThisAgent)
	response["agent_id"] = targetAgentID.String()
	return resolvedMemoryTarget{
		Scope:      model.AgentMemoryScopeOrg,
		AgentID:    &targetAgentID,
		Visibility: AgentVisibilityThisAgent,
		Response:   response,
	}, nil
}

func normalizeMemoryVisibility(raw, fallback string) string {
	value := strings.ToLower(strings.TrimSpace(raw))
	if value == "" {
		return fallback
	}
	return value
}

// canForget decides whether the calling agent may archive mem. The user-scope
// rule applies to everyone, supervisor included: personal memories may only be
// forgotten in a session for that same user. Memories bound to another agent
// are only forgettable by a supervisor (see orgMemoriesEnabled) carrying
// explicit user approval.
func (toolCtx memoryToolContext) canForget(mem model.AgentMemory, supervisor, userApproved bool) error {
	if mem.Scope == model.AgentMemoryScopeUser {
		if toolCtx.UserID == nil || mem.UserID == nil || *toolCtx.UserID != *mem.UserID {
			return fmt.Errorf("forget_memory cannot archive another user's memory")
		}
	}
	if mem.AgentID == nil || *mem.AgentID == toolCtx.AgentID {
		return nil
	}
	if !supervisor {
		return fmt.Errorf("forget_memory cannot archive another agent's memory")
	}
	if !userApproved {
		return fmt.Errorf("this memory belongs to another agent: show the user its content and owning agent, get their explicit confirmation, then retry with user_approved true")
	}
	return nil
}
