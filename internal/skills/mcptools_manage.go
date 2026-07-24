package skills

import (
	"context"
	"regexp"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/access"
	"github.com/usehivy/hivy/internal/model"
)

const (
	toolCreateSkill  = "create_skill"
	toolUpdateSkill  = "update_skill"
	toolArchiveSkill = "archive_skill"

	maxSkillContentBytes   = 256 * 1024
	maxSkillFileBytes      = 256 * 1024
	maxSkillTotalBytes     = 1024 * 1024
	maxSkillFiles          = 32
	maxSkillNameLen        = 120
	maxSkillDescriptionLen = 1024
)

var envVarNamePattern = regexp.MustCompile(`^[A-Z][A-Z0-9_]*$`)

func resolveSkillManagerTeam(ctx context.Context, db *gorm.DB, token *model.Token, rawActorUserID, action string) (*model.Agent, *access.Actor, *mcp.CallToolResult) {
	if token == nil {
		return nil, nil, skillToolError("missing agent token")
	}
	agentID, err := skillToolAgentID(token)
	if err != nil {
		return nil, nil, skillToolError(err.Error())
	}
	agent, err := loadActiveAgent(ctx, db, token.OrgID, agentID)
	if err != nil {
		return nil, nil, skillToolError("calling agent not found")
	}
	actor, err := access.Resolve(ctx, db, token.OrgID, rawActorUserID)
	if err != nil {
		return nil, nil, skillToolError(err.Error())
	}
	if actor == nil {
		return nil, nil, skillToolError("Not allowed: " + action + " must be done on behalf of a team member, but this run has no human actor.")
	}
	ok, err := actor.CanManageTeamResource(ctx, db, agent.TeamID)
	if err != nil {
		return nil, nil, skillToolError(err.Error())
	}
	if !ok {
		return nil, nil, skillToolError("Not allowed: " + action + " requires membership in the calling agent's team.")
	}
	return agent, actor, nil
}

func registerSkillManagerTools(server *mcp.Server, db *gorm.DB, token *model.Token, frontendURL string) {
	registerCreateSkillTool(server, db, token, frontendURL)
	registerUpdateSkillTool(server, db, token, frontendURL)
	registerArchiveSkillTool(server, db, token)
}
