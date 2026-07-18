package mcpserver

import (
	"encoding/json"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/usehivy/hivy/internal/model"
)

func cronScheduleResponse(row model.AgentSchedule) map[string]any {
	out := map[string]any{
		"job_id":           row.RuntimeJobID,
		"status":           row.Status,
		"schedule_kind":    row.ScheduleKind,
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
