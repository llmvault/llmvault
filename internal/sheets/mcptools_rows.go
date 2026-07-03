package sheets

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/usehivy/hivy/internal/model"
)

// --- rows_query --------------------------------------------------------------

type sheetSortArgs struct {
	Field     string `json:"field"`
	Direction string `json:"direction"`
}

type rowsQueryArgs struct {
	PageID           string          `json:"page_id"`
	Filter           *Filter         `json:"filter"`
	Sorts            []sheetSortArgs `json:"sorts"`
	Search           string          `json:"search"`
	Cursor           string          `json:"cursor"`
	Limit            int             `json:"limit"`
	ResolveRelations bool            `json:"resolve_relations"`
	HivySessionID    string          `json:"_hivy_session_id"`
}

func registerRowsQuery(server *mcp.Server, svc *Service, token *model.Token) {
	server.AddTool(&mcp.Tool{
		Name:        toolRowsQuery,
		Description: "Query a page's rows with a filter AST, sorts, free-text search, and cursor paging. Returns {rows: [{id, data}], fields_legend, next_cursor}; data is keyed by field ID. Results are capped at 100 rows per call — when next_cursor is non-empty, pass it back as cursor and keep going until it is empty. Pass resolve_relations to hydrate relation cells into {id, label} chips.",
		InputSchema: map[string]any{
			"type":                 "object",
			"additionalProperties": false,
			"properties": map[string]any{
				"page_id": map[string]any{"type": "string", "description": "UUID of the page to query."},
				"filter": map[string]any{
					"type":        "object",
					"description": "Filter AST: nested {\"and\": [...]} / {\"or\": [...]} groups over {field, op, value} conditions. field is a field ID (fld_…, never a display name); op is one of eq neq contains not_contains starts_with gt gte lt lte is_empty is_not_empty in. Use is_empty/is_not_empty (no value) for emptiness; `in` takes an array value.",
				},
				"sorts": map[string]any{
					"type":        "array",
					"description": "Sort order. field is a field ID, or the special keys position / created_at. Field sorts disable cursor paging.",
					"maxItems":    4,
					"items": map[string]any{
						"type":                 "object",
						"additionalProperties": false,
						"properties": map[string]any{
							"field":     map[string]any{"type": "string", "description": "Field ID (fld_…), or position / created_at."},
							"direction": map[string]any{"type": "string", "enum": []string{"asc", "desc"}, "description": "Sort direction; defaults to asc."},
						},
						"required": []string{"field"},
					},
				},
				"search":            map[string]any{"type": "string", "description": "Case-insensitive substring match across the whole row — use instead of building an `or` over every text field."},
				"cursor":            map[string]any{"type": "string", "description": "Opaque cursor from a previous call's next_cursor."},
				"limit":             map[string]any{"type": "integer", "minimum": 1, "maximum": QueryLimitMCP, "description": "Rows per page, clamped to 100."},
				"resolve_relations": map[string]any{"type": "boolean", "description": "Hydrate relation cells into {id, label} pairs in the relations map."},
			},
			"required": []string{"page_id"},
		},
	}, func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		var args rowsQueryArgs
		if errResult := decodeSheetToolArgs(req, &args); errResult != nil {
			return errResult, nil
		}
		return handleRowsQuery(ctx, svc, token, args)
	})
}

func handleRowsQuery(ctx context.Context, svc *Service, token *model.Token, args rowsQueryArgs) (*mcp.CallToolResult, error) {
	tctx, cancel := sheetToolContext(ctx)
	defer cancel()
	pageID, errResult := parseSheetToolUUID(args.PageID, "page_id")
	if errResult != nil {
		return errResult, nil
	}
	channelID, errResult := sheetToolChannelResult(tctx, svc, token, args.HivySessionID)
	if errResult != nil {
		return errResult, nil
	}
	if errResult := sheetToolGuardResult(svc.PageInChannel(tctx, token.OrgID, channelID, pageID)); errResult != nil {
		return errResult, nil
	}
	sorts, err := sheetToolSorts(args.Sorts)
	if err != nil {
		return sheetToolError(err.Error()), nil
	}
	result, err := svc.QueryRows(tctx, token.OrgID, pageID, Query{
		Filter:           args.Filter,
		Search:           args.Search,
		Sorts:            sorts,
		Cursor:           args.Cursor,
		Limit:            args.Limit,
		ResolveRelations: args.ResolveRelations,
	}, QueryLimitMCP)
	if err != nil {
		return sheetToolError(err.Error()), nil
	}
	out := map[string]any{
		"rows":          rowObjects(result.Rows),
		"fields_legend": fieldsLegend(result.Fields),
		"next_cursor":   result.NextCursor,
		"total":         len(result.Rows),
	}
	if result.Relations != nil {
		out["relations"] = result.Relations
	}
	return sheetToolJSON(out)
}

func sheetToolSorts(args []sheetSortArgs) ([]Sort, error) {
	if len(args) == 0 {
		return nil, nil
	}
	out := make([]Sort, 0, len(args))
	for _, sort := range args {
		field := strings.TrimSpace(sort.Field)
		if field == "" {
			return nil, fmt.Errorf("sorts entries require a field")
		}
		switch strings.ToLower(strings.TrimSpace(sort.Direction)) {
		case "", "asc":
			out = append(out, Sort{Field: field})
		case "desc":
			out = append(out, Sort{Field: field, Desc: true})
		default:
			return nil, fmt.Errorf("sort direction must be asc or desc")
		}
	}
	return out, nil
}

// --- rows_write --------------------------------------------------------------

type rowWriteItemArgs struct {
	ID   string         `json:"id"`
	Data map[string]any `json:"data"`
}

type rowsWriteArgs struct {
	PageID        string             `json:"page_id"`
	Action        string             `json:"action"`
	Rows          []rowWriteItemArgs `json:"rows"`
	IDs           []string           `json:"ids"`
	HivySessionID string             `json:"_hivy_session_id"`
}

func registerRowsWrite(server *mcp.Server, svc *Service, token *model.Token, agentID uuid.UUID) {
	server.AddTool(&mcp.Tool{
		Name:        toolRowsWrite,
		Description: "Write rows on a page in batches of at most 100 per call. action insert takes rows [{data}]; action update takes rows [{id, data}] and merges ONLY the keys you send (partial merge — other cells untouched); action delete takes ids [...] and archives them. data is keyed by field ID (fld_… from sheet_describe), never by field name. Every call is one revertible operation — see sheet_operations.",
		InputSchema: map[string]any{
			"type":                 "object",
			"additionalProperties": false,
			"properties": map[string]any{
				"page_id": map[string]any{"type": "string", "description": "UUID of the page to write to."},
				"action":  map[string]any{"type": "string", "enum": []string{"insert", "update", "delete"}, "description": "Write mode."},
				"rows": map[string]any{
					"type":        "array",
					"description": "For insert: [{data}]. For update: [{id, data}]. data maps field IDs to cell values.",
					"maxItems":    MaxRowsPerWriteMCP,
					"items": map[string]any{
						"type":                 "object",
						"additionalProperties": false,
						"properties": map[string]any{
							"id":   map[string]any{"type": "string", "description": "Row UUID (update only)."},
							"data": map[string]any{"type": "object", "description": "Cell values keyed by field ID, e.g. {\"fld_8k2mx1q9\": \"Acme\"}."},
						},
						"required": []string{"data"},
					},
				},
				"ids": map[string]any{
					"type":        "array",
					"description": "Row UUIDs to archive (delete only).",
					"maxItems":    MaxRowsPerWriteMCP,
					"items":       map[string]any{"type": "string"},
				},
			},
			"required": []string{"page_id", "action"},
		},
	}, func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		var args rowsWriteArgs
		if errResult := decodeSheetToolArgs(req, &args); errResult != nil {
			return errResult, nil
		}
		return handleRowsWrite(ctx, svc, token, agentID, args)
	})
}

func handleRowsWrite(ctx context.Context, svc *Service, token *model.Token, agentID uuid.UUID, args rowsWriteArgs) (*mcp.CallToolResult, error) {
	tctx, cancel := sheetToolContext(ctx)
	defer cancel()
	pageID, errResult := parseSheetToolUUID(args.PageID, "page_id")
	if errResult != nil {
		return errResult, nil
	}
	actor, err := svc.sheetToolActor(tctx, token, agentID, args.HivySessionID)
	if err != nil {
		return sheetToolError(err.Error()), nil
	}
	if errResult := sheetToolGuardResult(svc.PageInChannel(tctx, token.OrgID, actor.ChannelID, pageID)); errResult != nil {
		return errResult, nil
	}
	switch strings.ToLower(strings.TrimSpace(args.Action)) {
	case "insert":
		return rowsWriteInsert(tctx, svc, token, pageID, actor, args.Rows)
	case "update":
		return rowsWriteUpdate(tctx, svc, token, pageID, actor, args.Rows)
	case "delete":
		return rowsWriteDelete(tctx, svc, token, pageID, actor, args.IDs)
	default:
		return sheetToolError("action must be one of insert, update, delete"), nil
	}
}

func rowsWriteInsert(ctx context.Context, svc *Service, token *model.Token, pageID uuid.UUID, actor Actor, items []rowWriteItemArgs) (*mcp.CallToolResult, error) {
	if len(items) == 0 {
		return sheetToolError("insert requires a non-empty rows array"), nil
	}
	inserts := make([]RowInsert, 0, len(items))
	for _, item := range items {
		if item.ID != "" {
			return sheetToolError("insert rows must not include id (use action update to change existing rows)"), nil
		}
		if len(item.Data) == 0 {
			return sheetToolError("every insert row requires a data object keyed by field ID"), nil
		}
		inserts = append(inserts, RowInsert{Data: item.Data})
	}
	rows, err := svc.InsertRows(ctx, token.OrgID, pageID, inserts, MaxRowsPerWriteMCP, actor)
	if err != nil {
		return sheetToolError(err.Error()), nil
	}
	return sheetToolJSON(map[string]any{"action": "insert", "inserted": len(rows), "rows": rowObjects(rows)})
}

func rowsWriteUpdate(ctx context.Context, svc *Service, token *model.Token, pageID uuid.UUID, actor Actor, items []rowWriteItemArgs) (*mcp.CallToolResult, error) {
	if len(items) == 0 {
		return sheetToolError("update requires a non-empty rows array"), nil
	}
	updates := make([]RowUpdate, 0, len(items))
	for _, item := range items {
		rowID, errResult := parseSheetToolUUID(item.ID, "row id")
		if errResult != nil {
			return errResult, nil
		}
		if len(item.Data) == 0 {
			return sheetToolError("every update row requires a data object with the keys to change"), nil
		}
		updates = append(updates, RowUpdate{ID: rowID, Data: item.Data})
	}
	rows, err := svc.UpdateRows(ctx, token.OrgID, pageID, updates, MaxRowsPerWriteMCP, actor)
	if err != nil {
		return sheetToolError(err.Error()), nil
	}
	return sheetToolJSON(map[string]any{"action": "update", "updated": len(rows), "rows": rowObjects(rows)})
}

func rowsWriteDelete(ctx context.Context, svc *Service, token *model.Token, pageID uuid.UUID, actor Actor, rawIDs []string) (*mcp.CallToolResult, error) {
	if len(rawIDs) == 0 {
		return sheetToolError("delete requires a non-empty ids array"), nil
	}
	ids := make([]uuid.UUID, 0, len(rawIDs))
	for _, raw := range rawIDs {
		rowID, errResult := parseSheetToolUUID(raw, "row id")
		if errResult != nil {
			return errResult, nil
		}
		ids = append(ids, rowID)
	}
	deleted, err := svc.DeleteRows(ctx, token.OrgID, pageID, ids, MaxRowsPerWriteMCP, actor)
	if err != nil {
		return sheetToolError(err.Error()), nil
	}
	return sheetToolJSON(map[string]any{"action": "delete", "deleted": deleted})
}
