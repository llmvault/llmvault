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

// requireTeamManager gates the privileged agent-builder tools on the acting
// human being able to manage the agent's owning team: an org owner/admin always
// may, otherwise the actor must be an active member of teamID. Returning a nil
// result means allowed. This mirrors the REST agent-write path
// (handler.authorizeAgentMutation / resolveAndAuthorizeAgentTeam).
//
// A run with no human actor (automated trigger/schedule/system run) fails
// closed: these tools mutate agents and must not be reachable from an
// externally-triggerable run without a person behind it. A nil/zero teamID is
// manager-only — there is no team to authorize a plain member against, mirroring
// the REST rule that an unassigned agent is a manager action. `action` names
// what is being attempted, e.g. "creating an agent".
func requireTeamManager(ctx context.Context, db *gorm.DB, orgID, teamID uuid.UUID, req *mcp.CallToolRequest, action string) *mcp.CallToolResult {
	actor, err := access.Resolve(ctx, db, orgID, actorUserIDFromRequest(req))
	if err != nil {
		return toolError(err.Error())
	}
	if actor == nil {
		return toolError("Not allowed: " + action + " must be done on behalf of an organization member, " +
			"but this run has no human actor (it was started by an automated trigger or schedule).")
	}
	if actor.IsOrgManager() {
		return nil
	}
	if teamID == uuid.Nil {
		return toolError("Not allowed: " + action + " for an agent with no team requires an organization admin or owner. " +
			"Ask an organization admin or owner to make this change.")
	}
	ok, err := actor.CanManageTeamResource(ctx, db, teamID)
	if err != nil {
		return toolError(err.Error())
	}
	if !ok {
		return toolError("Not allowed: " + action + " requires you to be a member of the agent's team. " +
			"The person you're acting for is not a member of that team. " +
			"Ask a member of the team or an organization admin or owner to make this change.")
	}
	return nil
}
