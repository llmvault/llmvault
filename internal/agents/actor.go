package agents

import (
	"context"
	"encoding/json"

	"github.com/google/uuid"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/access"
)

// actorUserIDFromRequest extracts the runtime-injected `_hivy_actor_user_id`
// from a tool call — the trusted identity of the human behind the turn. It is
// never part of the tool schema (the model can't see or set it); empty means an
// automated run with no human actor.
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

// requireOrgManager gates the privileged agent-builder tools on the acting
// human being an org owner/admin. Returning a nil result means allowed.
//
// When there is no human actor (automated trigger/system run, or a deploy where
// the runtime does not yet inject identity) the check is skipped so existing
// flows keep working; the user-facing tightening applies once a real actor is
// present. `action` names what is being attempted, e.g. "creating an agent".
func requireOrgManager(ctx context.Context, db *gorm.DB, orgID uuid.UUID, req *mcp.CallToolRequest, action string) *mcp.CallToolResult {
	actor, err := access.Resolve(ctx, db, orgID, actorUserIDFromRequest(req))
	if err != nil {
		return toolError(err.Error())
	}
	if actor == nil {
		return nil
	}
	if !actor.IsOrgManager() {
		return toolError("Not allowed: " + action + " requires an organization admin or owner. " +
			"The person you're acting for has the role \"" + actor.OrgRole + "\", which can't create or change agents. " +
			"Ask an organization admin or owner to make this change.")
	}
	return nil
}
