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
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/model"
	"github.com/usehivy/hivy/internal/pluginresolve"
)

// Tool names for the agent-facing apps MCP tool group (apps plan §6). The
// names, args, and return shapes are the contract documented in
// global/plugins/apps/skills/apps/SKILL.md — change them together.
const (
	toolAppCreate   = "app_create"
	toolAppPublish  = "app_publish"
	toolAppStatus   = "app_status"
	toolAppLogs     = "app_logs"
	toolAppRollback = "app_rollback"
)

// AppsPluginSlug is the plugin whose per-agent install gates this tool group.
// Installing the plugin grants both the apps skill (existing machinery) and
// these tools, mirroring the sheets plugin gate.
const AppsPluginSlug = "apps"

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

// NewToolsFunc returns the apps ToolsFunc. It registers the five app tools on
// the MCP server ONLY when the calling token is an agent proxy for an active
// agent whose team-resolved effective plugin set includes the `apps` plugin,
// mirroring sheets.NewToolsFunc. Registering nothing when the plugin is
// absent keeps the tool surface out of un-enrolled agents' prompts entirely.
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
		if !PluginInstalled(ctx, svc.db, agent) {
			return
		}
		registerAppCreate(server, svc, token, agentID)
		registerAppPublish(server, svc, token, agentID)
		registerAppStatus(server, svc, token)
		registerAppLogs(server, svc, token)
		registerAppRollback(server, svc, token)
	}
}

// PluginInstalled reports whether the apps plugin is in the agent's effective
// plugin set (team grants ∪ auto-install ∪ default-agent). Exported because the
// preview-env endpoint gates on the same entitlement (handler.AppPreviewEnv).
func PluginInstalled(ctx context.Context, db *gorm.DB, agent model.Agent) bool {
	has, err := pluginresolve.AgentHasPluginSlug(ctx, db, agent, AppsPluginSlug)
	return err == nil && has
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

// appToolSession resolves the agent's current channel and session from the
// runtime-injected _hivy_session_id. The value is server-controlled and cannot
// be forged by the model. A session is mandatory: apps are channel-scoped and
// the channel is derived from session.ChannelID (a NOT NULL column, so every
// session has one). Returns (channelID, sessionID, error).
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
		Select("id", "channel_id").
		Where("id = ? AND org_id = ?", sessionID, token.OrgID).
		First(&session).Error; err != nil {
		return uuid.Nil, uuid.Nil, fmt.Errorf("session not found in this org")
	}
	return session.ChannelID, sessionID, nil
}

// appForSession loads one app scoped to the caller's org AND session channel.
// A cross-channel app returns the same "not found" as a missing one, so an
// agent cannot probe another channel's apps (sheets channelGuard convention).
func (s *Service) appForSession(ctx context.Context, token *model.Token, channelID uuid.UUID, rawAppID string) (*model.App, *mcp.CallToolResult) {
	appID, errResult := parseAppToolUUID(rawAppID, "app_id")
	if errResult != nil {
		return nil, errResult
	}
	app, err := s.GetApp(ctx, token.OrgID, appID)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return nil, appToolError("app not found in this channel")
		}
		return nil, appToolError(err.Error())
	}
	if app.ChannelID != channelID {
		return nil, appToolError("app not found in this channel")
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
