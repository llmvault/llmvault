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

// actorCanAccessCronSchedule reports whether the human behind the turn may see
// or manage a schedule, gating on the schedule's channel. A nil actor (an
// automated trigger/cron/system run) and org managers are unrestricted, exactly
// as the create path treats them; a regular member may only reach a schedule
// whose channel they can use.
func actorCanAccessCronSchedule(ctx context.Context, db *gorm.DB, actor *access.Actor, schedule model.AgentSchedule) bool {
	if actor == nil || actor.IsOrgManager() {
		return true
	}
	channelID, err := uuid.Parse(strings.TrimSpace(schedule.Channel))
	if err != nil || channelID == uuid.Nil {
		return false
	}
	allowed, err := actor.CanUseChannelID(ctx, db, channelID)
	return err == nil && allowed
}

// enforceActorCronSchedule blocks a manage action (update/pause/resume/cancel)
// on a schedule whose channel the acting human cannot use. It is a no-op for a
// nil actor (automated run) or an org manager. The returned *mcp.CallToolResult,
// when non-nil, is a user-facing error the agent should relay verbatim.
func enforceActorCronSchedule(ctx context.Context, db *gorm.DB, actor *access.Actor, agent *model.Agent, jobID string) *mcp.CallToolResult {
	if actor == nil || actor.IsOrgManager() {
		return nil
	}
	schedule, err := agentschedule.LoadForAgent(ctx, db, agent, jobID)
	if err != nil {
		return cronToolError(err.Error())
	}
	if !actorCanAccessCronSchedule(ctx, db, actor, *schedule) {
		return cronToolError("Not allowed: you can only manage cron jobs in channels you belong to. " +
			"The person you're acting for is not a member of this job's channel (it may be restricted to a team they're not on). " +
			"Ask an organization admin or a member of that channel to manage it.")
	}
	return nil
}
