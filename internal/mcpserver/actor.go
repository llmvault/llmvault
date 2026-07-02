package mcpserver

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/google/uuid"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/access"
)

// actorUserIDFromRequest extracts the runtime-injected `_hivy_actor_user_id`
// from a tool call. The field is never part of the tool's advertised schema —
// the Rust runtime injects it (like `_hivy_session_id`) so the model cannot see
// or set it. Empty means the run has no human actor (trigger/cron/system).
func actorUserIDFromRequest(req *mcp.CallToolRequest) string {
	if req == nil || req.Params.Arguments == nil {
		return ""
	}
	var probe struct {
		ActorUserID string `json:"_hivy_actor_user_id"`
	}
	_ = json.Unmarshal(req.Params.Arguments, &probe)
	return probe.ActorUserID
}

// enforceActorChannelAccess blocks binding a scheduled run / trigger to an
// explicit channel the acting user cannot use. It is a no-op when there is no
// human actor (automated runs keep their agent-scoped behavior) or the actor is
// an org manager. The returned *mcp.CallToolResult, when non-nil, is a
// user-facing error the agent should relay verbatim.
func enforceActorChannelAccess(ctx context.Context, db *gorm.DB, actor *access.Actor, channelID uuid.UUID) *mcp.CallToolResult {
	if actor == nil || actor.IsOrgManager() {
		return nil
	}
	allowed, err := actor.CanUseChannelID(ctx, db, channelID)
	if err != nil {
		return cronToolError("could not verify your access to that channel: " + err.Error())
	}
	if !allowed {
		return cronToolError("Not allowed: you can only target channels you belong to. " +
			"The person you're acting for is not a member of that channel (it may be restricted to a team they're not on). " +
			"Pick a channel they belong to, or ask an organization admin or a member of that channel to set this up.")
	}
	return nil
}

// enforceActorChannelArg is enforceActorChannelAccess for a raw channel_id
// argument. An empty value (falls back to the org's managed system channel) is
// not gated; a malformed value is reported clearly. Returns nil when allowed.
func enforceActorChannelArg(ctx context.Context, db *gorm.DB, actor *access.Actor, raw string) *mcp.CallToolResult {
	raw = strings.TrimSpace(raw)
	if raw == "" || actor == nil || actor.IsOrgManager() {
		return nil
	}
	channelID, err := uuid.Parse(raw)
	if err != nil || channelID == uuid.Nil {
		return cronToolError("channel_id must be a valid HIVY channel UUID (use list_channels to find it)")
	}
	return enforceActorChannelAccess(ctx, db, actor, channelID)
}
