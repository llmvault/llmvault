package mcpserver

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/agentschedule"
	"github.com/usehivy/hivy/internal/model"
)

func addCronTool(server *mcp.Server, token *model.Token, db *gorm.DB) {
	if server == nil || token == nil || db == nil || token.Meta == nil {
		return
	}
	if tokenType, _ := token.Meta[model.TokenMetaType].(string); tokenType != model.TokenTypeAgentProxy {
		return
	}
	agentIDText, _ := token.Meta[model.TokenMetaAgentID].(string)
	agentID, err := uuid.Parse(strings.TrimSpace(agentIDText))
	if err != nil || agentID == uuid.Nil {
		return
	}
	var agent model.Agent
	if err := db.Where("id = ? AND org_id = ? AND status <> ?", agentID, token.OrgID, "archived").First(&agent).Error; err != nil {
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
				"channel_id": map[string]any{
					"type":        "string",
					"description": "Optional channel UUID for the scheduled run. Defaults to the org system channel.",
				},
			},
			"required": []string{"action"},
		},
	}, func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		args, err := decodeCronToolArgs(req)
		if err != nil {
			return cronToolError(err.Error()), nil
		}
		return handleCronTool(ctx, db, &agent, args)
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
	ChannelID       string  `json:"channel_id"`
	HivySessionID   string  `json:"_hivy_session_id"`
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
	agent := callingAgent
	if strings.TrimSpace(args.AgentID) != "" {
		target, errResult := resolveCronAgent(ctx, db, callingAgent, args.AgentID)
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
			Description:     args.Description,
			TaskPrompt:      args.TaskPrompt,
			ChannelID:       args.ChannelID,
			IntervalSeconds: args.IntervalSeconds,
			CronExpression:  expr,
			RepeatCount:     args.RepeatCount,
		}
		var (
			schedule *model.AgentSchedule
			err      error
		)
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
		out := make([]map[string]any, 0, len(rows))
		for _, row := range rows {
			out = append(out, cronScheduleResponse(row))
		}
		return cronToolJSON(map[string]any{"jobs": out, "total": len(out)})
	case "update":
		if strings.TrimSpace(args.JobID) == "" {
			return cronToolError("job_id is required"), nil
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
		if strings.TrimSpace(args.ChannelID) != "" {
			update.ChannelID = &args.ChannelID
		}
		schedule, err := agentschedule.Update(ctx, db, agent, args.JobID, update)
		if err != nil {
			return cronToolError(err.Error()), nil
		}
		return cronToolJSON(map[string]any{"job": cronScheduleResponse(*schedule)})
	case "pause":
		return cronToolSetStatus(ctx, db, agent, args.JobID, agentschedule.StatusPaused)
	case "resume":
		return cronToolSetStatus(ctx, db, agent, args.JobID, agentschedule.StatusActive)
	case "cancel":
		return cronToolSetStatus(ctx, db, agent, args.JobID, agentschedule.StatusCancelled)
	default:
		return cronToolError("action must be one of create, list, update, pause, resume, cancel"), nil
	}
}

// resolveCronAgent returns the target agent for a cron action, validating it
// belongs to the calling agent's organization. Returns the calling agent when
// agent_id matches it.
func resolveCronAgent(ctx context.Context, db *gorm.DB, callingAgent *model.Agent, idText string) (*model.Agent, *mcp.CallToolResult) {
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
	return &target, nil
}

func cronToolSetStatus(ctx context.Context, db *gorm.DB, agent *model.Agent, jobID, status string) (*mcp.CallToolResult, error) {
	if strings.TrimSpace(jobID) == "" {
		return cronToolError("job_id is required"), nil
	}
	schedule, err := agentschedule.SetStatus(ctx, db, agent, jobID, status)
	if err != nil {
		return cronToolError(err.Error()), nil
	}
	return cronToolJSON(map[string]any{"job": cronScheduleResponse(*schedule)})
}

func cronScheduleResponse(row model.AgentSchedule) map[string]any {
	out := map[string]any{
		"job_id":           row.RuntimeJobID,
		"status":           row.Status,
		"schedule_kind":    row.ScheduleKind,
		"channel_id":       row.Channel,
		"description":      row.Description,
		"task_prompt":      row.TaskPrompt,
		"interval_seconds": row.IntervalSeconds,
		"cron_expression":  row.CronExpression,
		"repeat_count":     row.RepeatCount,
		"repeat_completed": row.RepeatCompleted,
		"next_run_at":      row.NextRunAt,
		"last_run_at":      row.LastRunAt,
		"last_status":      row.LastStatus,
		"last_error":       row.LastError,
		"created_at":       row.CreatedAt,
		"updated_at":       row.UpdatedAt,
	}
	if row.SourceSlug != "" {
		out["source_slug"] = row.SourceSlug
	}
	return out
}

func cronToolError(msg string) *mcp.CallToolResult {
	return &mcp.CallToolResult{
		Content: []mcp.Content{&mcp.TextContent{Text: "Error: " + msg}},
		IsError: true,
	}
}

func cronToolJSON(v any) (*mcp.CallToolResult, error) {
	b, err := json.Marshal(v)
	if err != nil {
		return cronToolError("failed to serialize response"), nil
	}
	return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: string(b)}}}, nil
}
