package sheets

import (
	"context"
	"strings"

	"github.com/google/uuid"
	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/usehivy/hivy/internal/model"
)

// --- sheet_import_csv --------------------------------------------------------

type sheetImportCSVArgs struct {
	Action        string         `json:"action"`
	PageID        string         `json:"page_id"`
	ObjectKey     string         `json:"object_key"`
	Options       map[string]any `json:"options"`
	JobID         string         `json:"job_id"`
	HivySessionID string         `json:"_hivy_session_id"`
}

func registerSheetImportCSV(server *mcp.Server, svc *Service, token *model.Token, agentID uuid.UUID) {
	server.AddTool(&mcp.Tool{
		Name:        toolSheetImportCSV,
		Description: "Import a CSV from the agent drive into a page. action start (the default) takes page_id, object_key (the drive upload's key), and options {has_header, delimiter, field_mapping, create_fields} and returns a job_id; the import runs asynchronously. action status takes job_id and returns status (pending|running|completed|failed), processed_rows, total_rows, and error — poll it until completed before reporting success. For fewer than ~500 rows use rows_write batches instead.",
		InputSchema: map[string]any{
			"type":                 "object",
			"additionalProperties": false,
			"properties": map[string]any{
				"action":     map[string]any{"type": "string", "enum": []string{"start", "status"}, "description": "start begins an import (default when omitted); status polls a job."},
				"page_id":    map[string]any{"type": "string", "description": "UUID of the target page (start)."},
				"object_key": map[string]any{"type": "string", "description": "Object key returned by the drive upload, e.g. pub/e/{agentID}/imports/leads.csv; org-owned pub/o/… keys are accepted too (start)."},
				"options": map[string]any{
					"type":        "object",
					"description": "Import options (start).",
					"properties": map[string]any{
						"has_header":    map[string]any{"type": "boolean", "description": "First row is a header row."},
						"delimiter":     map[string]any{"type": "string", "description": "Column delimiter; defaults to a comma."},
						"field_mapping": map[string]any{"type": "object", "description": "CSV column name → existing field ID (fld_…)."},
						"create_fields": map[string]any{"type": "boolean", "description": "Create fields from the header with type inference instead of mapping."},
					},
				},
				"job_id": map[string]any{"type": "string", "description": "Import job UUID (status)."},
			},
		},
	}, func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		var args sheetImportCSVArgs
		if errResult := decodeSheetToolArgs(req, &args); errResult != nil {
			return errResult, nil
		}
		return handleSheetImportCSV(ctx, svc, token, agentID, args)
	})
}

func handleSheetImportCSV(ctx context.Context, svc *Service, token *model.Token, agentID uuid.UUID, args sheetImportCSVArgs) (*mcp.CallToolResult, error) {
	switch strings.ToLower(strings.TrimSpace(args.Action)) {
	case "", "start":
		return importCSVStart(ctx, svc, token, agentID, args)
	case "status":
		return importCSVStatus(ctx, svc, token, args)
	default:
		return sheetToolError("action must be start or status"), nil
	}
}

func importCSVStart(ctx context.Context, svc *Service, token *model.Token, agentID uuid.UUID, args sheetImportCSVArgs) (*mcp.CallToolResult, error) {
	tctx, cancel := context.WithTimeout(ctx, sheetImportStartTimeout)
	defer cancel()
	pageID, errResult := parseSheetToolUUID(args.PageID, "page_id")
	if errResult != nil {
		return errResult, nil
	}
	if strings.TrimSpace(args.ObjectKey) == "" {
		return sheetToolError("object_key is required (upload the CSV to the agent drive first)"), nil
	}
	actor, err := svc.sheetToolActor(tctx, token, agentID, args.HivySessionID)
	if err != nil {
		return sheetToolError(err.Error()), nil
	}
	if errResult := sheetToolGuardResult(svc.PageInChannel(tctx, token.OrgID, actor.ChannelID, pageID)); errResult != nil {
		return errResult, nil
	}
	job, err := svc.CreateImportJob(tctx, token.OrgID, pageID, CreateImportJobRequest{
		ObjectKey: args.ObjectKey,
		Options:   model.JSON(args.Options),
	}, actor)
	if err != nil {
		return sheetToolError(err.Error()), nil
	}
	return sheetToolJSON(map[string]any{
		"job":  importJobObject(job),
		"note": "The import runs asynchronously. Poll sheet_import_csv with action \"status\" and this job_id until status is completed.",
	})
}

func importCSVStatus(ctx context.Context, svc *Service, token *model.Token, args sheetImportCSVArgs) (*mcp.CallToolResult, error) {
	tctx, cancel := sheetToolContext(ctx)
	defer cancel()
	jobID, errResult := parseSheetToolUUID(args.JobID, "job_id")
	if errResult != nil {
		return errResult, nil
	}
	channelID, errResult := sheetToolChannelResult(tctx, svc, token, args.HivySessionID)
	if errResult != nil {
		return errResult, nil
	}
	if errResult := sheetToolGuardResult(svc.ImportJobInChannel(tctx, token.OrgID, channelID, jobID)); errResult != nil {
		return errResult, nil
	}
	job, err := svc.GetImportJob(tctx, token.OrgID, jobID)
	if err != nil {
		return sheetToolError(err.Error()), nil
	}
	return sheetToolJSON(map[string]any{"job": importJobObject(job)})
}

func importJobObject(job *model.SheetImportJob) map[string]any {
	return map[string]any{
		"job_id":         job.ID.String(),
		"page_id":        job.PageID.String(),
		"status":         job.Status,
		"processed_rows": job.ProcessedRows,
		"total_rows":     job.TotalRows,
		"error":          job.Error,
		"created_at":     job.CreatedAt,
	}
}

// --- sheet_operations --------------------------------------------------------

type sheetOperationsArgs struct {
	Action        string `json:"action"`
	PageID        string `json:"page_id"`
	OperationID   string `json:"operation_id"`
	HivySessionID string `json:"_hivy_session_id"`
}

func registerSheetOperations(server *mcp.Server, svc *Service, token *model.Token, agentID uuid.UUID) {
	server.AddTool(&mcp.Tool{
		Name:        toolSheetOperations,
		Description: "Inspect and undo mutations on a page. action list returns the page's recent operations (type, row_count, actor, reverted_at), newest first. action revert applies one operation's exact inverse by operation_id: inserts are archived, updates restore prior values, deletes are un-archived, imports archive every imported row. Undo is single-level — a revert is not itself re-undoable — so pick the right operation the first time.",
		InputSchema: map[string]any{
			"type":                 "object",
			"additionalProperties": false,
			"properties": map[string]any{
				"action":       map[string]any{"type": "string", "enum": []string{"list", "revert"}, "description": "list recent operations, or revert one."},
				"page_id":      map[string]any{"type": "string", "description": "UUID of the page (list)."},
				"operation_id": map[string]any{"type": "string", "description": "UUID of the operation to revert (revert)."},
			},
			"required": []string{"action"},
		},
	}, func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		var args sheetOperationsArgs
		if errResult := decodeSheetToolArgs(req, &args); errResult != nil {
			return errResult, nil
		}
		return handleSheetOperations(ctx, svc, token, agentID, args)
	})
}

func handleSheetOperations(ctx context.Context, svc *Service, token *model.Token, agentID uuid.UUID, args sheetOperationsArgs) (*mcp.CallToolResult, error) {
	tctx, cancel := sheetToolContext(ctx)
	defer cancel()
	switch strings.ToLower(strings.TrimSpace(args.Action)) {
	case "list":
		return operationsList(tctx, svc, token, args)
	case "revert":
		return operationsRevert(tctx, svc, token, agentID, args)
	default:
		return sheetToolError("action must be list or revert"), nil
	}
}

func operationsList(ctx context.Context, svc *Service, token *model.Token, args sheetOperationsArgs) (*mcp.CallToolResult, error) {
	pageID, errResult := parseSheetToolUUID(args.PageID, "page_id")
	if errResult != nil {
		return errResult, nil
	}
	channelID, errResult := sheetToolChannelResult(ctx, svc, token, args.HivySessionID)
	if errResult != nil {
		return errResult, nil
	}
	if errResult := sheetToolGuardResult(svc.PageInChannel(ctx, token.OrgID, channelID, pageID)); errResult != nil {
		return errResult, nil
	}
	ops, err := svc.ListOperations(ctx, token.OrgID, pageID, OperationRetentionPerPage)
	if err != nil {
		return sheetToolError(err.Error()), nil
	}
	out := make([]map[string]any, 0, len(ops))
	for i := range ops {
		out = append(out, operationObject(&ops[i]))
	}
	return sheetToolJSON(map[string]any{"operations": out, "total": len(out)})
}

func operationsRevert(ctx context.Context, svc *Service, token *model.Token, agentID uuid.UUID, args sheetOperationsArgs) (*mcp.CallToolResult, error) {
	operationID, errResult := parseSheetToolUUID(args.OperationID, "operation_id")
	if errResult != nil {
		return errResult, nil
	}
	actor, err := svc.sheetToolActor(ctx, token, agentID, args.HivySessionID)
	if err != nil {
		return sheetToolError(err.Error()), nil
	}
	if errResult := sheetToolGuardResult(svc.OperationInChannel(ctx, token.OrgID, actor.ChannelID, operationID)); errResult != nil {
		return errResult, nil
	}
	if err := svc.RevertOperation(ctx, token.OrgID, operationID, actor); err != nil {
		return sheetToolError(err.Error()), nil
	}
	return sheetToolJSON(map[string]any{"reverted": true, "operation_id": operationID.String()})
}

func operationObject(op *model.SheetOperation) map[string]any {
	out := map[string]any{
		"id":         op.ID.String(),
		"page_id":    op.PageID.String(),
		"type":       op.Type,
		"row_count":  op.RowCount,
		"created_at": op.CreatedAt,
	}
	if op.ActorAgentID != nil {
		out["actor_agent_id"] = op.ActorAgentID.String()
	}
	if op.ActorUserID != nil {
		out["actor_user_id"] = op.ActorUserID.String()
	}
	if op.RevertedAt != nil {
		out["reverted_at"] = *op.RevertedAt
	}
	return out
}
