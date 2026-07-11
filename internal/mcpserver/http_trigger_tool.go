package mcpserver

import (
	"context"
	"encoding/json"
	"os"
	"strings"

	"github.com/google/uuid"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/access"
	"github.com/usehivy/hivy/internal/agentschedule"
	"github.com/usehivy/hivy/internal/model"
)

// addHTTPTriggerTool registers create_http_trigger for agent-proxy tokens. The
// trigger runs an agent whenever its returned URL is POSTed to. By default it
// targets the calling agent; agent_id targets another agent in the same org.
func addHTTPTriggerTool(server *mcp.Server, token *model.Token, db *gorm.DB) {
	if server == nil {
		return
	}
	callingAgent := callingProxyAgent(token, db)
	if callingAgent == nil || !callingAgent.IsDefault {
		return
	}
	server.AddTool(&mcp.Tool{
		Name:        "create_http_trigger",
		Description: "Create an HTTP trigger that runs an agent whenever its URL is POSTed to. Returns the trigger URL to hand to the user or an external system. By default it runs the calling agent; pass agent_id to target another agent in the same organization.",
		InputSchema: map[string]any{
			"type":                 "object",
			"additionalProperties": false,
			"required":             []string{"instructions"},
			"properties": map[string]any{
				"agent_id": map[string]any{
					"type":        "string",
					"description": "Optional UUID of the agent to run. Defaults to the calling agent. Must be in the same organization.",
				},
				"instructions": map[string]any{
					"type":        "string",
					"description": "What the agent should do each time the trigger fires.",
				},
				"channel_id": map[string]any{
					"type":        "string",
					"description": "Optional HIVY channel UUID (not a Slack/provider channel id) the run's conversation lives in. Use list_channels to find valid ids. Defaults to your team's #general channel.",
				},
				"secret": map[string]any{
					"type":        "string",
					"description": "Optional shared secret. When set, callers must send it (Authorization: Bearer <secret>, X-Api-Key, X-Webhook-Secret, or ?secret=). Recommended.",
				},
			},
		},
	}, func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		var args httpTriggerArgs
		if req == nil || req.Params.Arguments == nil {
			return cronToolError("arguments are required"), nil
		}
		if err := json.Unmarshal(req.Params.Arguments, &args); err != nil {
			return cronToolError("invalid arguments"), nil
		}
		return handleCreateHTTPTrigger(ctx, db, token, callingAgent, args)
	})
}

type httpTriggerArgs struct {
	AgentID         string `json:"agent_id"`
	Instructions    string `json:"instructions"`
	ChannelID       string `json:"channel_id"`
	Secret          string `json:"secret"`
	HivyActorUserID string `json:"_hivy_actor_user_id"`
}

func handleCreateHTTPTrigger(ctx context.Context, db *gorm.DB, token *model.Token, callingAgent *model.Agent, args httpTriggerArgs) (*mcp.CallToolResult, error) {
	instructions := strings.TrimSpace(args.Instructions)
	if instructions == "" {
		return cronToolError("instructions is required"), nil
	}
	actor, err := access.Resolve(ctx, db, token.OrgID, args.HivyActorUserID)
	if err != nil {
		return cronToolError(err.Error()), nil
	}
	agent := callingAgent
	if strings.TrimSpace(args.AgentID) != "" {
		target, errResult := resolveCronAgent(ctx, db, callingAgent, actor, args.AgentID)
		if errResult != nil {
			return errResult, nil
		}
		agent = target
	}

	// Resolve the channel exactly like cron schedules do: an empty channel
	// resolves to the agent's team #general (the team-scoped IsDefault channel);
	// explicit values get org-scoped validation plus the agent-access check.
	// Every new trigger stores an explicit channel.
	channelText, err := agentschedule.ResolveScheduleChannel(ctx, db, token.OrgID, agent.ID, args.ChannelID)
	if err != nil {
		return cronToolError(err.Error()), nil
	}
	resolved, err := uuid.Parse(channelText)
	if err != nil || resolved == uuid.Nil {
		return cronToolError("resolve channel: invalid channel id"), nil
	}
	if errResult := enforceActorChannelAccess(ctx, db, actor, resolved); errResult != nil {
		return errResult, nil
	}
	channelID := &resolved

	trigger := model.AgentTrigger{
		OrgID:        token.OrgID,
		AgentID:      agent.ID,
		TriggerType:  "http",
		ChannelID:    channelID,
		Instructions: instructions,
		Enabled:      true,
	}
	secret := strings.TrimSpace(args.Secret)
	if secret != "" {
		hash, err := bcrypt.GenerateFromPassword([]byte(secret), bcrypt.DefaultCost)
		if err != nil {
			return cronToolError("failed to secure trigger secret"), nil
		}
		trigger.SecretKey = string(hash)
	}
	if err := db.WithContext(ctx).Create(&trigger).Error; err != nil {
		return cronToolError("create trigger: " + err.Error()), nil
	}

	url := strings.TrimRight(triggerBaseURL(), "/") + "/incoming/triggers/" + trigger.ID.String()
	out := map[string]any{
		"trigger_id":      trigger.ID.String(),
		"agent_id":        agent.ID.String(),
		"trigger_type":    "http",
		"url":             url,
		"method":          "POST",
		"secret_required": secret != "",
	}
	if secret != "" {
		out["secret"] = secret
	}
	return cronToolJSON(out)
}

// triggerBaseURL returns the public API base for /incoming/triggers URLs. Read
// from the same env var config uses (HIVY_API_WEBHOOK_BASE_URL) so this tool
// stays self-contained.
func triggerBaseURL() string {
	if v := strings.TrimSpace(os.Getenv("HIVY_API_WEBHOOK_BASE_URL")); v != "" {
		return v
	}
	return "https://api.usehivy.com"
}
