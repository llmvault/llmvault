package memory

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"

	"github.com/google/uuid"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/model"
)

type memoryToolTarget struct {
	Owner      string `json:"owner"`
	Visibility string `json:"visibility"`
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

func normalizeMemoryRetainTarget(target memoryToolTarget, toolCtx memoryToolContext) (resolvedMemoryTarget, error) {
	owner := strings.ToLower(strings.TrimSpace(target.Owner))
	visibility := normalizeMemoryVisibility(target.Visibility, "")
	if owner != memoryToolOwnerOrg && owner != memoryToolOwnerUser {
		return resolvedMemoryTarget{}, fmt.Errorf("target.owner must be org or user")
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

func normalizeMemoryVisibility(raw, fallback string) string {
	value := strings.ToLower(strings.TrimSpace(raw))
	if value == "" {
		return fallback
	}
	return value
}

func (toolCtx memoryToolContext) canForget(mem model.AgentMemory) error {
	if mem.Scope != model.AgentMemoryScopeUser {
		return nil
	}
	if toolCtx.UserID == nil || mem.UserID == nil || *toolCtx.UserID != *mem.UserID {
		return fmt.Errorf("forget_memory cannot archive another user's memory")
	}
	return nil
}

func (s *Service) loadToolMemory(ctx context.Context, orgID, memoryID uuid.UUID) (model.AgentMemory, error) {
	var mem model.AgentMemory
	err := s.cfg.DB.WithContext(ctx).
		Where("id = ? AND org_id = ? AND archived_at IS NULL", memoryID, orgID).
		First(&mem).Error
	return mem, err
}

func memoryToolMetadata(metadata model.JSON, target map[string]any, agentID uuid.UUID) model.JSON {
	out := model.JSON{}
	for key, value := range metadata {
		out[key] = value
	}
	out["source"] = "mcp_memory_tool"
	out["created_by_agent_id"] = agentID.String()
	out["target"] = target
	return out
}

func memoryToolSearchResponses(hits []SearchHit) []map[string]any {
	out := make([]map[string]any, 0, len(hits))
	for _, hit := range hits {
		similarity := hit.Similarity
		out = append(out, memoryToolMemoryResponse(hit.Memory, &similarity))
	}
	return out
}

func memoryToolMemoryResponse(mem model.AgentMemory, similarity *float64) map[string]any {
	out := map[string]any{
		"id":                 mem.ID.String(),
		"content":            mem.Content,
		"tags":               []string(mem.Tags),
		"target":             memoryToolTargetFromMemory(mem),
		"embedding_status":   mem.EmbeddingStatus,
		"embedding_revision": mem.EmbeddingRevision,
		"created_at":         mem.CreatedAt,
		"updated_at":         mem.UpdatedAt,
	}
	if similarity != nil {
		out["similarity"] = *similarity
	}
	return out
}

func memoryToolTargetFromMemory(mem model.AgentMemory) map[string]any {
	owner := mem.Scope
	visibility := AgentVisibilityAllAgents
	if mem.AgentID != nil {
		visibility = AgentVisibilityThisAgent
	}
	return memoryToolTargetResponse(owner, visibility)
}

func memoryToolTargetResponse(owner, visibility string) map[string]any {
	return map[string]any{"owner": owner, "visibility": visibility}
}

func normalizeMemoryToolSearchQuery(raw string) (string, error) {
	query := strings.TrimSpace(raw)
	if query == "" {
		return "", fmt.Errorf("query is required")
	}
	if len(query) > memoryToolQueryMaxChars {
		return "", fmt.Errorf("query must be at most %d characters", memoryToolQueryMaxChars)
	}
	if len(strings.Fields(query)) > memoryToolQueryMaxWords {
		return "", fmt.Errorf("query must be at most %d words", memoryToolQueryMaxWords)
	}
	return query, nil
}

func normalizeMemoryToolTags(values []string) ([]string, error) {
	if len(values) > memoryToolMaxTags {
		return nil, fmt.Errorf("tags must contain at most %d items", memoryToolMaxTags)
	}
	out := make([]string, 0, len(values))
	seen := map[string]bool{}
	for _, raw := range values {
		tag := strings.TrimSpace(raw)
		if tag == "" {
			return nil, fmt.Errorf("tags must be non-empty lowercase kebab-case slugs")
		}
		if !memoryToolTagRE.MatchString(tag) {
			return nil, fmt.Errorf("tags must be lowercase kebab-case slugs, for example project-helio")
		}
		if seen[tag] {
			return nil, fmt.Errorf("tags must be unique")
		}
		seen[tag] = true
		out = append(out, tag)
	}
	return out, nil
}

func memoryToolError(msg string) *mcp.CallToolResult {
	return &mcp.CallToolResult{
		Content: []mcp.Content{&mcp.TextContent{Text: "Error: " + msg}},
		IsError: true,
	}
}

func memoryToolJSON(v any) (*mcp.CallToolResult, error) {
	b, err := json.Marshal(v)
	if err != nil {
		return memoryToolError("failed to serialize response"), nil
	}
	return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: string(b)}}}, nil
}

func memoryToolLoadMessage(err error) string {
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return "memory not found"
	}
	return "failed to load memory: " + err.Error()
}
