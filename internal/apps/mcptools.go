package apps

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/usehivy/hivy/internal/model"
)

// Tool names for the agent-facing apps MCP tool group.
const (
	toolAppCreate   = "app_create"
	toolAppPublish  = "app_publish"
	toolAppStatus   = "app_status"
	toolAppLogs     = "app_logs"
	toolAppRollback = "app_rollback"
)

const (
	// appToolTimeout bounds quick tool work: DB loads, app_create.
	appToolTimeout = 20 * time.Second
	// appOpsTimeout bounds status/logs calls, which reach into the app
	// sandbox's appd (and implicitly wake a sleeping sandbox).
	appOpsTimeout = 60 * time.Second
	// appPublishTimeout bounds the synchronous publish/rollback pipeline:
	// object copies, sandbox provisioning (boot retry window), bundle
	// download, and app restart can legitimately take a couple of minutes.
	appPublishTimeout = 10 * time.Minute
)

// NewToolsFunc registers the app tools for active agent proxy tokens. The
// runtime MCP allow-list remains authoritative for tool visibility.
func NewToolsFunc(svc *Service) func(server *mcp.Server, token *model.Token) {
	return func(server *mcp.Server, token *model.Token) {
		if server == nil || svc == nil || svc.db == nil || !appToolAgentProxy(token) {
			return
		}
		agentID, err := appToolAgentID(token)
		if err != nil {
			return
		}
		ctx, cancel := context.WithTimeout(context.Background(), appToolTimeout)
		defer cancel()
		var agent model.Agent
		if err := svc.db.WithContext(ctx).
			Where("id = ? AND org_id = ? AND status <> ?", agentID, token.OrgID, "archived").
			First(&agent).Error; err != nil {
			return
		}
		registerAppCreate(server, svc, token, agentID)
		registerAppPublish(server, svc, token, agentID)
		registerAppStatus(server, svc, token)
		registerAppLogs(server, svc, token)
		registerAppRollback(server, svc, token)
	}
}

func appToolAgentProxy(token *model.Token) bool {
	if token == nil || token.Meta == nil {
		return false
	}
	tokenType, _ := token.Meta[model.TokenMetaType].(string)
	return tokenType == model.TokenTypeAgentProxy
}

func appToolAgentID(token *model.Token) (uuid.UUID, error) {
	agentIDText, _ := token.Meta[model.TokenMetaAgentID].(string)
	agentID, err := uuid.Parse(strings.TrimSpace(agentIDText))
	if err != nil || agentID == uuid.Nil {
		return uuid.Nil, fmt.Errorf("agent proxy token is missing agent_id")
	}
	return agentID, nil
}

// appToolSession resolves the agent's current team and session from the
// runtime-injected _hivy_session_id. The value is server-controlled and cannot
// be forged by the model. A session is mandatory: apps are team-scoped and
// the team is derived from session.TeamID. Returns (teamID, sessionID, error).
func (s *Service) appToolSession(ctx context.Context, token *model.Token, rawSessionID string) (uuid.UUID, uuid.UUID, error) {
	sessionIDText := strings.TrimSpace(rawSessionID)
	if sessionIDText == "" {
		return uuid.Nil, uuid.Nil, fmt.Errorf("app tools must be called from within a session")
	}
	sessionID, err := uuid.Parse(sessionIDText)
	if err != nil || sessionID == uuid.Nil {
		return uuid.Nil, uuid.Nil, fmt.Errorf("_hivy_session_id must be a valid UUID")
	}
	var session model.Session
	if err := s.db.WithContext(ctx).
		Select("id", "team_id").
		Where("id = ? AND org_id = ?", sessionID, token.OrgID).
		First(&session).Error; err != nil {
		return uuid.Nil, uuid.Nil, fmt.Errorf("session not found in this org")
	}
	return session.TeamID, sessionID, nil
}

// appForSession loads one app scoped to the caller's org AND session team.
func (s *Service) appForSession(ctx context.Context, token *model.Token, teamID uuid.UUID, rawAppID string) (*model.App, *mcp.CallToolResult) {
	appID, errResult := parseAppToolUUID(rawAppID, "app_id")
	if errResult != nil {
		return nil, errResult
	}
	app, err := s.GetApp(ctx, token.OrgID, appID)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return nil, appToolError("app not found in this team")
		}
		return nil, appToolError(err.Error())
	}
	if app.TeamID != teamID {
		return nil, appToolError("app not found in this team")
	}
	return app, nil
}

// --- shared plumbing ---------------------------------------------------------

func decodeAppToolArgs(req *mcp.CallToolRequest, dst any) *mcp.CallToolResult {
	if req == nil || req.Params.Arguments == nil {
		return nil // no arguments is valid for optional-only payloads
	}
	if err := json.Unmarshal(req.Params.Arguments, dst); err != nil {
		return appToolError("invalid arguments")
	}
	return nil
}

func appToolError(msg string) *mcp.CallToolResult {
	return &mcp.CallToolResult{
		Content: []mcp.Content{&mcp.TextContent{Text: "Error: " + msg}},
		IsError: true,
	}
}

func appToolText(text string) (*mcp.CallToolResult, error) {
	return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: text}}}, nil
}

func appToolJSON(v any) (*mcp.CallToolResult, error) {
	b, err := json.Marshal(v)
	if err != nil {
		return appToolError("failed to serialize response"), nil
	}
	return appToolText(string(b))
}

func parseAppToolUUID(raw, name string) (uuid.UUID, *mcp.CallToolResult) {
	id, err := uuid.Parse(strings.TrimSpace(raw))
	if err != nil || id == uuid.Nil {
		return uuid.Nil, appToolError(name + " must be a valid UUID")
	}
	return id, nil
}
