package mcpserver

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/access"
	"github.com/usehivy/hivy/internal/agentschedule"
	"github.com/usehivy/hivy/internal/model"
)

func addCronTool(server *mcp.Server, token *model.Token, db *gorm.DB) {
	if server == nil {
		return
	}
	agent := callingProxyAgent(token, db)
	if agent == nil || !agent.IsDefault {
		return
	}
	server.AddTool(&mcp.Tool{
		Name:        "cron",
		Description: "Create, list, update, pause, resume, and cancel recurring cron jobs. By default the job belongs to the calling agent; pass agent_id to manage jobs for another agent in the same organization. All times are UTC.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"action": map[string]any{
					"type":        "string",
					"description": "One of create, list, update, pause, resume, cancel.",
				},
				"agent_id": map[string]any{
					"type":        "string",
					"description": "Optional UUID of the agent this job belongs to. Defaults to the calling agent. Must be an agent in the same organization.",
				},
				"job_id": map[string]any{
					"type":        "string",
					"description": "Cron job id for update, pause, resume, or cancel.",
				},
				"task_prompt": map[string]any{
					"type":        "string",
					"description": "Prompt to send when the cron job runs.",
				},
				"description": map[string]any{
					"type":        "string",
					"description": "Short human-readable description.",
				},
				"interval_seconds": map[string]any{
					"type":        "integer",
					"description": "Recurring interval in seconds. Mutually exclusive with cron_expression.",
				},
				"cron_expression": map[string]any{
					"type":        "string",
					"description": "Standard 5-field cron expression. Mutually exclusive with interval_seconds.",
				},
				"repeat_count": map[string]any{
					"type":        "integer",
					"description": "Optional maximum number of runs.",
				},
			},
			"required": []string{"action"},
		},
	}, func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		args, err := decodeCronToolArgs(req)
		if err != nil {
			return cronToolError(err.Error()), nil
		}
		return handleCronTool(ctx, db, agent, args)
	})
}

type cronToolArgs struct {
	Action          string  `json:"action"`
	AgentID         string  `json:"agent_id"`
	JobID           string  `json:"job_id"`
	TaskPrompt      string  `json:"task_prompt"`
	Description     string  `json:"description"`
	IntervalSeconds *int64  `json:"interval_seconds"`
	CronExpression  *string `json:"cron_expression"`
	RepeatCount     *int64  `json:"repeat_count"`
	HivySessionID   string  `json:"_hivy_session_id"`
	HivyActorUserID string  `json:"_hivy_actor_user_id"`
}

func decodeCronToolArgs(req *mcp.CallToolRequest) (cronToolArgs, error) {
	var args cronToolArgs
	if req == nil || req.Params.Arguments == nil {
		return args, fmt.Errorf("arguments are required")
	}
	if err := json.Unmarshal(req.Params.Arguments, &args); err != nil {
		return args, fmt.Errorf("invalid arguments")
	}
	args.Action = strings.ToLower(strings.TrimSpace(args.Action))
	return args, nil
}

func handleCronTool(ctx context.Context, db *gorm.DB, callingAgent *model.Agent, args cronToolArgs) (*mcp.CallToolResult, error) {
	// By default the job belongs to the calling agent; agent_id targets another
	// agent in the same org (validated here).
	orgID := uuid.Nil
	if callingAgent != nil && callingAgent.OrgID != nil {
		orgID = *callingAgent.OrgID
	}
	actor, err := access.Resolve(ctx, db, orgID, args.HivyActorUserID)
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
	switch args.Action {
	case "create":
		expr := ""
		if args.CronExpression != nil {
			expr = strings.TrimSpace(*args.CronExpression)
		}
		input := agentschedule.CreateInput{
			JobID:           args.JobID,
			CreatedByUserID: cronScheduleCreator(actor),
			Description:     args.Description,
			TaskPrompt:      args.TaskPrompt,
			IntervalSeconds: args.IntervalSeconds,
			CronExpression:  expr,
			RepeatCount:     args.RepeatCount,
		}
		var schedule *model.AgentSchedule
		if agent.ID == callingAgent.ID {
			// Self-scheduling: tie the schedule to the caller's session.
			schedule, err = agentschedule.CreateFromSession(ctx, db, agent, args.HivySessionID, input)
		} else {
			// Cross-agent: no caller session for the target agent.
			schedule, err = agentschedule.Create(ctx, db, agent, input)
		}
		if err != nil {
			return cronToolError(err.Error()), nil
		}
		return cronToolJSON(map[string]any{"job": cronScheduleResponse(*schedule)})
	case "list":
		rows, err := agentschedule.List(ctx, db, agent)
		if err != nil {
			return cronToolError(err.Error()), nil
		}
		// A non-manager actor only sees schedules owned by agents in their teams.
		// nil-actor (automated) and managers see all.
		out := make([]map[string]any, 0, len(rows))
		for _, row := range rows {
			if !actorCanAccessCronSchedule(ctx, db, actor, row) {
				continue
			}
			out = append(out, cronScheduleResponse(row))
		}
		return cronToolJSON(map[string]any{"jobs": out, "total": len(out)})
	case "update":
		if strings.TrimSpace(args.JobID) == "" {
			return cronToolError("job_id is required"), nil
		}
		if errResult := enforceActorCronSchedule(ctx, db, actor, agent, args.JobID); errResult != nil {
			return errResult, nil
		}
		update := agentschedule.UpdateInput{
			IntervalSeconds: args.IntervalSeconds,
			CronExpression:  args.CronExpression,
			RepeatCount:     args.RepeatCount,
		}
		if strings.TrimSpace(args.Description) != "" {
			update.Description = &args.Description
		}
		if strings.TrimSpace(args.TaskPrompt) != "" {
			update.TaskPrompt = &args.TaskPrompt
		}
		schedule, err := agentschedule.Update(ctx, db, agent, args.JobID, update)
		if err != nil {
			return cronToolError(err.Error()), nil
		}
		return cronToolJSON(map[string]any{"job": cronScheduleResponse(*schedule)})
	case "pause":
		return cronToolSetStatus(ctx, db, actor, agent, args.JobID, agentschedule.StatusPaused)
	case "resume":
		return cronToolSetStatus(ctx, db, actor, agent, args.JobID, agentschedule.StatusActive)
	case "cancel":
		return cronToolSetStatus(ctx, db, actor, agent, args.JobID, agentschedule.StatusCancelled)
	default:
		return cronToolError("action must be one of create, list, update, pause, resume, cancel"), nil
	}
}

func cronScheduleCreator(actor *access.Actor) *uuid.UUID {
	if actor == nil || actor.UserID == uuid.Nil {
		return nil
	}
	id := actor.UserID
	return &id
}

// resolveCronAgent returns the target agent for a cron action, validating it
// belongs to the calling agent's organization. Returns the calling agent when
// agent_id matches it.
func resolveCronAgent(ctx context.Context, db *gorm.DB, callingAgent *model.Agent, actor *access.Actor, idText string) (*model.Agent, *mcp.CallToolResult) {
	id, err := uuid.Parse(strings.TrimSpace(idText))
	if err != nil || id == uuid.Nil {
		return nil, cronToolError("agent_id must be a valid UUID")
	}
	if callingAgent == nil || callingAgent.OrgID == nil {
		return nil, cronToolError("agent context is missing an organization")
	}
	if id == callingAgent.ID {
		return callingAgent, nil
	}
	var target model.Agent
	if err := db.WithContext(ctx).
		Where("id = ? AND org_id = ? AND status <> ?", id, *callingAgent.OrgID, "archived").
		First(&target).Error; err != nil {
		return nil, cronToolError("agent not found in this organization")
	}
	if actor != nil && !actor.IsOrgManager() {
		member, err := actor.IsTeamMember(ctx, db, target.TeamID)
		if err != nil {
			return nil, cronToolError("could not verify your access to that agent: " + err.Error())
		}
		if !member {
			return nil, cronToolError("Not allowed: you can only manage agents on teams you belong to. " +
				"Ask a member of that team or an organization admin to set this up.")
		}
	}
	return &target, nil
}

func cronToolSetStatus(ctx context.Context, db *gorm.DB, actor *access.Actor, agent *model.Agent, jobID, status string) (*mcp.CallToolResult, error) {
	if strings.TrimSpace(jobID) == "" {
		return cronToolError("job_id is required"), nil
	}
	if errResult := enforceActorCronSchedule(ctx, db, actor, agent, jobID); errResult != nil {
		return errResult, nil
	}
	schedule, err := agentschedule.SetStatus(ctx, db, agent, jobID, status)
	if err != nil {
		return cronToolError(err.Error()), nil
	}
	return cronToolJSON(map[string]any{"job": cronScheduleResponse(*schedule)})
}
