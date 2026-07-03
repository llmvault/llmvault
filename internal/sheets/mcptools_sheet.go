package sheets

import (
	"context"
	"strings"

	"github.com/google/uuid"
	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/usehivy/hivy/internal/model"
)

// --- sheet_create ------------------------------------------------------------

type sheetCreateFieldArgs struct {
	Name    string         `json:"name"`
	Type    string         `json:"type"`
	Options map[string]any `json:"options"`
}

type sheetCreatePageArgs struct {
	Name   string                 `json:"name"`
	Fields []sheetCreateFieldArgs `json:"fields"`
}

type sheetCreateArgs struct {
	Name          string                `json:"name"`
	Description   string                `json:"description"`
	Pages         []sheetCreatePageArgs `json:"pages"`
	HivySessionID string                `json:"_hivy_session_id"`
}

func registerSheetCreate(server *mcp.Server, svc *Service, token *model.Token, agentID uuid.UUID) {
	server.AddTool(&mcp.Tool{
		Name:        toolSheetCreate,
		Description: "Create a new sheet with optional pages (tabs) and typed fields (columns). Returns the sheet, page IDs, and generated field IDs — capture the field IDs, every rows_write/rows_query data payload is keyed by them. Check sheet_list first and reuse an existing sheet when one fits; never create duplicates.",
		InputSchema: map[string]any{
			"type":                 "object",
			"additionalProperties": false,
			"properties": map[string]any{
				"name":        map[string]any{"type": "string", "description": "Sheet display name, e.g. \"SaaS Competitor Research\"."},
				"description": map[string]any{"type": "string", "description": "Short description of what the sheet holds."},
				"pages": map[string]any{
					"type":        "array",
					"description": "Pages (tabs) to create inline. Omit for a single default page.",
					"maxItems":    MaxPagesPerSheet,
					"items": map[string]any{
						"type":                 "object",
						"additionalProperties": false,
						"properties": map[string]any{
							"name":   map[string]any{"type": "string", "description": "Page (tab) name."},
							"fields": sheetCreateFieldsSchema(),
						},
						"required": []string{"name"},
					},
				},
			},
			"required": []string{"name"},
		},
	}, func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		var args sheetCreateArgs
		if errResult := decodeSheetToolArgs(req, &args); errResult != nil {
			return errResult, nil
		}
		return handleSheetCreate(ctx, svc, token, agentID, args)
	})
}

func sheetCreateFieldsSchema() map[string]any {
	return map[string]any{
		"type":        "array",
		"description": "Typed columns for the page.",
		"maxItems":    MaxFieldsPerPage,
		"items": map[string]any{
			"type":                 "object",
			"additionalProperties": false,
			"properties": map[string]any{
				"name": map[string]any{"type": "string", "description": "Column display name."},
				"type": map[string]any{"type": "string", "enum": FieldTypes(), "description": "Field type."},
				"options": map[string]any{
					"type":        "object",
					"description": "Type-specific options: {\"choices\": [...]} for select/multi_select, {\"target_page_id\": \"…\"} for relation.",
				},
			},
			"required": []string{"name", "type"},
		},
	}
}

func handleSheetCreate(ctx context.Context, svc *Service, token *model.Token, agentID uuid.UUID, args sheetCreateArgs) (*mcp.CallToolResult, error) {
	tctx, cancel := sheetToolContext(ctx)
	defer cancel()
	if strings.TrimSpace(args.Name) == "" {
		return sheetToolError("name is required"), nil
	}
	actor, err := svc.sheetToolActor(tctx, token, agentID, args.HivySessionID)
	if err != nil {
		return sheetToolError(err.Error()), nil
	}
	req := CreateSheetRequest{Name: args.Name, Description: args.Description}
	for _, page := range args.Pages {
		spec := PageSpec{Name: page.Name}
		for _, field := range page.Fields {
			spec.Fields = append(spec.Fields, FieldSpec{
				Name:    field.Name,
				Type:    field.Type,
				Options: model.JSON(field.Options),
			})
		}
		req.Pages = append(req.Pages, spec)
	}
	structure, err := svc.CreateSheet(tctx, token.OrgID, req, actor)
	if err != nil {
		return sheetToolError(err.Error()), nil
	}
	return sheetToolJSON(sheetStructureObject(structure, nil))
}

// --- sheet_list --------------------------------------------------------------

type sheetListArgs struct {
	Search        string `json:"search"`
	Limit         int    `json:"limit"`
	HivySessionID string `json:"_hivy_session_id"`
}

func registerSheetList(server *mcp.Server, svc *Service, token *model.Token) {
	server.AddTool(&mcp.Tool{
		Name:        toolSheetList,
		Description: "List this channel's sheets with their pages and row counts, newest-updated first. Sheets are scoped to the channel you are working in. Use it before sheet_create to reuse an existing sheet, and to find the sheet/page IDs for other tools.",
		InputSchema: map[string]any{
			"type":                 "object",
			"additionalProperties": false,
			"properties": map[string]any{
				"search": map[string]any{"type": "string", "description": "Optional case-insensitive substring match on sheet names."},
				"limit":  map[string]any{"type": "integer", "minimum": 1, "maximum": QueryLimitMCP, "description": "Max sheets to return (clamped to 100)."},
			},
		},
	}, func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		var args sheetListArgs
		if errResult := decodeSheetToolArgs(req, &args); errResult != nil {
			return errResult, nil
		}
		return handleSheetList(ctx, svc, token, args)
	})
}

func handleSheetList(ctx context.Context, svc *Service, token *model.Token, args sheetListArgs) (*mcp.CallToolResult, error) {
	tctx, cancel := sheetToolContext(ctx)
	defer cancel()
	channelID, errResult := sheetToolChannelResult(tctx, svc, token, args.HivySessionID)
	if errResult != nil {
		return errResult, nil
	}
	sheets, _, err := svc.ListSheets(tctx, token.OrgID, channelID, args.Search, ClampLimit(args.Limit, QueryLimitMCP), "")
	if err != nil {
		return sheetToolError(err.Error()), nil
	}
	pagesBySheet, rowCounts, err := svc.sheetToolPagesIndex(tctx, token.OrgID, sheets)
	if err != nil {
		return sheetToolError(err.Error()), nil
	}
	out := make([]map[string]any, 0, len(sheets))
	for i := range sheets {
		sheet := &sheets[i]
		pages := make([]map[string]any, 0, len(pagesBySheet[sheet.ID]))
		for _, page := range pagesBySheet[sheet.ID] {
			pages = append(pages, map[string]any{
				"id":        page.ID.String(),
				"name":      page.Name,
				"row_count": rowCounts[page.ID],
			})
		}
		obj := sheetObject(sheet)
		obj["pages"] = pages
		out = append(out, obj)
	}
	return sheetToolJSON(map[string]any{"sheets": out, "total": len(out)})
}

// sheetToolPagesIndex loads all active pages for the listed sheets and their
// row counts in two grouped queries, both org-scoped.
func (s *Service) sheetToolPagesIndex(ctx context.Context, orgID uuid.UUID, sheets []model.Sheet) (map[uuid.UUID][]model.SheetPage, map[uuid.UUID]int64, error) {
	pagesBySheet := make(map[uuid.UUID][]model.SheetPage, len(sheets))
	if len(sheets) == 0 {
		return pagesBySheet, map[uuid.UUID]int64{}, nil
	}
	sheetIDs := make([]uuid.UUID, 0, len(sheets))
	for i := range sheets {
		sheetIDs = append(sheetIDs, sheets[i].ID)
	}
	var pages []model.SheetPage
	err := s.db.WithContext(ctx).
		Where("sheet_id IN ? AND org_id = ? AND archived_at IS NULL", sheetIDs, orgID).
		Order("position ASC").Find(&pages).Error
	if err != nil {
		return nil, nil, err
	}
	pageIDs := make([]uuid.UUID, 0, len(pages))
	for i := range pages {
		pagesBySheet[pages[i].SheetID] = append(pagesBySheet[pages[i].SheetID], pages[i])
		pageIDs = append(pageIDs, pages[i].ID)
	}
	rowCounts, err := s.PageRowCounts(ctx, orgID, pageIDs)
	if err != nil {
		return nil, nil, err
	}
	return pagesBySheet, rowCounts, nil
}

// --- sheet_describe ----------------------------------------------------------

type sheetDescribeArgs struct {
	SheetID       string `json:"sheet_id"`
	HivySessionID string `json:"_hivy_session_id"`
}

func registerSheetDescribe(server *mcp.Server, svc *Service, token *model.Token) {
	server.AddTool(&mcp.Tool{
		Name:        toolSheetDescribe,
		Description: "Get the full structure of one sheet: pages, fields (id/name/type/options), and row counts. Call this before writing to or querying any sheet you did not just create — data payloads and filters are keyed by field ID (fld_…), never by field name.",
		InputSchema: map[string]any{
			"type":                 "object",
			"additionalProperties": false,
			"properties": map[string]any{
				"sheet_id": map[string]any{"type": "string", "description": "UUID of the sheet to describe."},
			},
			"required": []string{"sheet_id"},
		},
	}, func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		var args sheetDescribeArgs
		if errResult := decodeSheetToolArgs(req, &args); errResult != nil {
			return errResult, nil
		}
		return handleSheetDescribe(ctx, svc, token, args)
	})
}

func handleSheetDescribe(ctx context.Context, svc *Service, token *model.Token, args sheetDescribeArgs) (*mcp.CallToolResult, error) {
	tctx, cancel := sheetToolContext(ctx)
	defer cancel()
	sheetID, errResult := parseSheetToolUUID(args.SheetID, "sheet_id")
	if errResult != nil {
		return errResult, nil
	}
	channelID, errResult := sheetToolChannelResult(tctx, svc, token, args.HivySessionID)
	if errResult != nil {
		return errResult, nil
	}
	if errResult := sheetToolGuardResult(svc.SheetInChannel(tctx, token.OrgID, channelID, sheetID)); errResult != nil {
		return errResult, nil
	}
	structure, err := svc.GetSheet(tctx, token.OrgID, sheetID)
	if err != nil {
		return sheetToolError(err.Error()), nil
	}
	pageIDs := make([]uuid.UUID, 0, len(structure.Pages))
	for i := range structure.Pages {
		pageIDs = append(pageIDs, structure.Pages[i].Page.ID)
	}
	rowCounts, err := svc.PageRowCounts(tctx, token.OrgID, pageIDs)
	if err != nil {
		return sheetToolError(err.Error()), nil
	}
	return sheetToolJSON(sheetStructureObject(structure, rowCounts))
}
