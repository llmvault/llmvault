package mcpserver

import (
	"context"
	"strings"

	"github.com/google/uuid"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/access"
	"github.com/usehivy/hivy/internal/agentschedule"
	"github.com/usehivy/hivy/internal/model"
)

// callingProxyAgent resolves the calling agent for an agent-proxy token.
// Returns nil when the token is not an agent-proxy token or the agent cannot
// be resolved; callers must then skip tool registration. Shared by the cron,
// create_http_trigger, and list_channels tools.
func callingProxyAgent(token *model.Token, db *gorm.DB) *model.Agent {
	if token == nil || db == nil || token.Meta == nil {
		return nil
	}
	if tokenType, _ := token.Meta[model.TokenMetaType].(string); tokenType != model.TokenTypeAgentProxy {
		return nil
	}
	agentIDText, _ := token.Meta[model.TokenMetaAgentID].(string)
	agentID, err := uuid.Parse(strings.TrimSpace(agentIDText))
	if err != nil || agentID == uuid.Nil {
		return nil
	}
	var agent model.Agent
	if err := db.Where("id = ? AND org_id = ? AND status <> ?", agentID, token.OrgID, "archived").First(&agent).Error; err != nil {
		return nil
	}
	return &agent
}

// addListChannelsTool registers list_channels for agent-proxy tokens. It lets
// agents translate provider channel references (Slack C0XXXX ids, channel
// names) into the Hivy channel UUIDs required by cron and create_http_trigger.
func addListChannelsTool(server *mcp.Server, token *model.Token, db *gorm.DB) {
	if server == nil {
		return
	}
	agent := callingProxyAgent(token, db)
	if agent == nil {
		return
	}
	server.AddTool(&mcp.Tool{
		Name: "list_channels",
		Description: "List the organization's channels and their HIVY channel UUIDs. " +
			"The returned id is the value to use as channel_id in the cron and create_http_trigger tools. " +
			"Provider channel ids (like Slack C0XXXX) are NOT valid channel_id values — use this tool to translate them: " +
			"externally linked channels include external_provider, external_resource_name, and external_resource_key " +
			"(e.g. the Slack C0XXXX id) so you can map a provider channel to the right HIVY UUID. " +
			"Only channels the calling agent is allowed to use are returned.",
		InputSchema: map[string]any{
			"type":       "object",
			"properties": map[string]any{},
		},
	}, func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		return handleListChannels(ctx, db, agent, actorUserIDFromRequest(req))
	})
}

func handleListChannels(ctx context.Context, db *gorm.DB, agent *model.Agent, actorRaw string) (*mcp.CallToolResult, error) {
	if agent == nil || agent.OrgID == nil {
		return cronToolError("agent context is missing an organization"), nil
	}
	orgID := *agent.OrgID
	// When a human is behind the turn, only surface channels they can use; a
	// non-manager must not learn the names/keys of channels they aren't in.
	// Automated runs (no actor) keep the agent-scoped view.
	actor, err := access.Resolve(ctx, db, orgID, actorRaw)
	if err != nil {
		return cronToolError(err.Error()), nil
	}
	restricted, err := agentschedule.RestrictedChannelIDs(ctx, db, orgID, agent.ID)
	if err != nil {
		return cronToolError(err.Error()), nil
	}
	var channels []model.Channel
	if err := db.WithContext(ctx).
		Where("org_id = ? AND archived_at IS NULL", orgID).
		Order("name ASC").
		Find(&channels).Error; err != nil {
		return cronToolError("list channels: " + err.Error()), nil
	}
	out := make([]map[string]any, 0, len(channels))
	for _, channel := range channels {
		if restricted != nil {
			if _, ok := restricted[channel.ID]; !ok {
				continue
			}
		}
		if actor != nil && !actor.CanUseChannel(ctx, db, channel) {
			continue
		}
		entry := map[string]any{
			"id":         channel.ID.String(),
			"name":       channel.Name,
			"kind":       channel.Kind,
			"visibility": channel.Visibility,
			"is_default": channel.IsDefault,
			"is_system":  channel.Kind == "system",
		}
		if channel.ExternalProvider != "" {
			entry["external_provider"] = channel.ExternalProvider
		}
		if channel.ExternalResourceName != "" {
			entry["external_resource_name"] = channel.ExternalResourceName
		}
		if channel.ExternalResourceKey != "" {
			entry["external_resource_key"] = channel.ExternalResourceKey
		}
		out = append(out, entry)
	}
	return cronToolJSON(map[string]any{"channels": out, "total": len(out)})
}
