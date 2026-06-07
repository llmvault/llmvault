package specialisttasks

import (
	"context"
	"strconv"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/usehivy/hivy/internal/model"
)

func registerTimelineTool(server *mcp.Server, service *Service, token *model.Token) {
	server.AddTool(&mcp.Tool{
		Name: "specialist_task_timeline",
		Description: `Fetch a paginated timeline of recorded events for a specialist task.

Use this when specialist_task_status is too compact and you need the specialist's message/tool-call history. Pass exactly the task_id returned by specialist_launch_task. Use limit and offset for pagination.`,
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"task_id": map[string]any{
					"type":        "string",
					"description": "UUID returned by specialist_launch_task.",
				},
				"limit": map[string]any{
					"type":        "integer",
					"description": "Maximum number of timeline events to return, default 20, max 100.",
				},
				"offset": map[string]any{
					"type":        "integer",
					"description": "Zero-based event offset for pagination.",
				},
			},
			"required": []string{"task_id"},
		},
	}, func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		var params struct {
			TaskID string `json:"task_id"`
			Limit  any    `json:"limit"`
			Offset any    `json:"offset"`
		}
		decodeArgs(req, &params)
		taskID, toolErr := parseUUIDField("task_id", params.TaskID)
		if toolErr != nil {
			return toolErrorJSON(toolErr), nil
		}
		limit, toolErr := parseOptionalIntField("limit", params.Limit)
		if toolErr != nil {
			return toolErrorJSON(toolErr), nil
		}
		offset, toolErr := parseOptionalIntField("offset", params.Offset)
		if toolErr != nil {
			return toolErrorJSON(toolErr), nil
		}
		resp, toolErr := service.Timeline(ctx, token, taskID, limit, offset)
		if toolErr != nil {
			return toolErrorJSON(toolErr), nil
		}
		return toolJSON(resp)
	})
}

func parseOptionalIntField(name string, value any) (int, *ToolError) {
	switch typed := value.(type) {
	case nil:
		return 0, nil
	case float64:
		if typed < 0 || typed != float64(int(typed)) {
			return 0, newToolError("invalid_"+name, name+" must be a non-negative integer.", "The provided numeric argument was negative or fractional.", false, "Pass "+name+" as a non-negative JSON integer or numeric string.")
		}
		return int(typed), nil
	case string:
		typed = strings.TrimSpace(typed)
		if typed == "" {
			return 0, nil
		}
		parsed, err := strconv.Atoi(typed)
		if err != nil {
			return 0, wrapToolError("invalid_"+name, name+" must be a non-negative integer.", err, false, "Pass "+name+" as a non-negative JSON integer or numeric string.")
		}
		if parsed < 0 {
			return 0, newToolError("invalid_"+name, name+" must be a non-negative integer.", "The provided numeric string was negative.", false, "Pass "+name+" as a non-negative JSON integer or numeric string.")
		}
		return parsed, nil
	default:
		return 0, newToolError("invalid_"+name, name+" must be a non-negative integer.", "The provided argument had an unsupported type.", false, "Pass "+name+" as a non-negative JSON integer or numeric string.")
	}
}
