package mcpserver

import (
	"context"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/access"
	"github.com/usehivy/hivy/internal/agentschedule"
	"github.com/usehivy/hivy/internal/model"
)

// actorCanAccessCronSchedule reports whether the human behind the turn may see
// or manage a schedule, gating on its owning agent's team. A nil actor (an
// automated trigger/cron/system run) and org managers are unrestricted, exactly
// as the create path treats them; a regular member may only reach a schedule
// whose team they can use.
func actorCanAccessCronSchedule(ctx context.Context, db *gorm.DB, actor *access.Actor, schedule model.AgentSchedule) bool {
	if actor == nil || actor.IsOrgManager() {
		return true
	}
	var agent model.Agent
	if err := db.WithContext(ctx).Select("team_id").Where("id = ? AND org_id = ?", schedule.AgentID, schedule.OrgID).First(&agent).Error; err != nil {
		return false
	}
	allowed, err := actor.IsTeamMember(ctx, db, agent.TeamID)
	return err == nil && allowed
}

// enforceActorCronSchedule blocks a manage action (update/pause/resume/cancel)
// on a schedule whose agent team the acting human cannot use. It is a no-op for a
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
		return cronToolError("Not allowed: you can only manage cron jobs for agents on teams you belong to. " +
			"Ask an organization admin or a member of that team to manage it.")
	}
	return nil
}
