package sheets

import (
	"context"
	"strings"

	"github.com/google/uuid"
	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/usehivy/hivy/internal/model"
)

// sheet_manage actions (cron-style dispatch keeps the tool count down).
var sheetManageActions = []string{
	"rename_sheet", "archive_sheet",
	"create_page", "rename_page", "archive_page",
	"add_field", "update_field", "archive_field",
}

type sheetManageArgs struct {
	Action        string         `json:"action"`
	SheetID       string         `json:"sheet_id"`
	PageID        string         `json:"page_id"`
	FieldID       string         `json:"field_id"`
	Name          *string        `json:"name"`
	Description   *string        `json:"description"`
	Type          *string        `json:"type"`
	Options       map[string]any `json:"options"`
	HivySessionID string         `json:"_hivy_session_id"`
}

func registerSheetManage(server *mcp.Server, svc *Service, token *model.Token, agentID uuid.UUID) {
	server.AddTool(&mcp.Tool{
		Name:        toolSheetManage,
		Description: "Manage sheet structure after creation, dispatched by action: rename_sheet | archive_sheet | create_page | rename_page | archive_page | add_field | update_field | archive_field. Renaming a field is update_field with a new name (cells key by field ID, so renames never touch data). Archive actions retire things without destroying row data.",
		InputSchema: map[string]any{
			"type":                 "object",
			"additionalProperties": false,
			"properties": map[string]any{
				"action":      map[string]any{"type": "string", "enum": sheetManageActions, "description": "The structure change to perform."},
				"sheet_id":    map[string]any{"type": "string", "description": "Sheet UUID for rename_sheet, archive_sheet, create_page."},
				"page_id":     map[string]any{"type": "string", "description": "Page UUID for rename_page, archive_page, add_field."},
				"field_id":    map[string]any{"type": "string", "description": "Field ID (fld_…) for update_field, archive_field."},
				"name":        map[string]any{"type": "string", "description": "New name for rename_sheet, create_page, rename_page, add_field, update_field."},
				"description": map[string]any{"type": "string", "description": "New description for rename_sheet."},
				"type":        map[string]any{"type": "string", "enum": FieldTypes(), "description": "Field type for add_field (required) or update_field (optional type change)."},
				"options":     map[string]any{"type": "object", "description": "Field options for add_field/update_field: {\"choices\": [...]} for select/multi_select, {\"target_page_id\": \"…\"} for relation."},
			},
			"required": []string{"action"},
		},
	}, func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		var args sheetManageArgs
		if errResult := decodeSheetToolArgs(req, &args); errResult != nil {
			return errResult, nil
		}
		return handleSheetManage(ctx, svc, token, agentID, args)
	})
}

func handleSheetManage(ctx context.Context, svc *Service, token *model.Token, agentID uuid.UUID, args sheetManageArgs) (*mcp.CallToolResult, error) {
	tctx, cancel := sheetToolContext(ctx)
	defer cancel()
	actor, err := svc.sheetToolActor(tctx, token, agentID, args.HivySessionID)
	if err != nil {
		return sheetToolError(err.Error()), nil
	}
	switch strings.ToLower(strings.TrimSpace(args.Action)) {
	case "rename_sheet":
		return manageRenameSheet(tctx, svc, token, actor, args)
	case "archive_sheet":
		return manageArchiveSheet(tctx, svc, token, actor, args)
	case "create_page":
		return manageCreatePage(tctx, svc, token, actor, args)
	case "rename_page":
		return manageRenamePage(tctx, svc, token, actor, args)
	case "archive_page":
		return manageArchivePage(tctx, svc, token, actor, args)
	case "add_field":
		return manageAddField(tctx, svc, token, actor, args)
	case "update_field":
		return manageUpdateField(tctx, svc, token, actor, args)
	case "archive_field":
		return manageArchiveField(tctx, svc, token, actor, args)
	default:
		return sheetToolError("action must be one of " + strings.Join(sheetManageActions, ", ")), nil
	}
}

func manageRenameSheet(ctx context.Context, svc *Service, token *model.Token, actor Actor, args sheetManageArgs) (*mcp.CallToolResult, error) {
	sheetID, errResult := parseSheetToolUUID(args.SheetID, "sheet_id")
	if errResult != nil {
		return errResult, nil
	}
	if errResult := sheetToolGuardResult(svc.SheetInTeam(ctx, token.OrgID, actor.TeamID, sheetID)); errResult != nil {
		return errResult, nil
	}
	if args.Name == nil && args.Description == nil {
		return sheetToolError("rename_sheet requires name and/or description"), nil
	}
	sheet, err := svc.UpdateSheet(ctx, token.OrgID, sheetID, UpdateSheetRequest{
		Name:        args.Name,
		Description: args.Description,
	})
	if err != nil {
		return sheetToolError(err.Error()), nil
	}
	return sheetToolJSON(map[string]any{"sheet": sheetObject(sheet)})
}

func manageArchiveSheet(ctx context.Context, svc *Service, token *model.Token, actor Actor, args sheetManageArgs) (*mcp.CallToolResult, error) {
	sheetID, errResult := parseSheetToolUUID(args.SheetID, "sheet_id")
	if errResult != nil {
		return errResult, nil
	}
	if errResult := sheetToolGuardResult(svc.SheetInTeam(ctx, token.OrgID, actor.TeamID, sheetID)); errResult != nil {
		return errResult, nil
	}
	if err := svc.ArchiveSheet(ctx, token.OrgID, sheetID); err != nil {
		return sheetToolError(err.Error()), nil
	}
	return sheetToolJSON(map[string]any{"archived": true, "sheet_id": sheetID.String()})
}

func manageCreatePage(ctx context.Context, svc *Service, token *model.Token, actor Actor, args sheetManageArgs) (*mcp.CallToolResult, error) {
	sheetID, errResult := parseSheetToolUUID(args.SheetID, "sheet_id")
	if errResult != nil {
		return errResult, nil
	}
	if errResult := sheetToolGuardResult(svc.SheetInTeam(ctx, token.OrgID, actor.TeamID, sheetID)); errResult != nil {
		return errResult, nil
	}
	name := ""
	if args.Name != nil {
		name = *args.Name
	}
	if strings.TrimSpace(name) == "" {
		return sheetToolError("create_page requires a name"), nil
	}
	structure, err := svc.CreatePage(ctx, token.OrgID, sheetID, PageSpec{Name: name})
	if err != nil {
		return sheetToolError(err.Error()), nil
	}
	return sheetToolJSON(map[string]any{"page": map[string]any{
		"id":     structure.Page.ID.String(),
		"name":   structure.Page.Name,
		"fields": fieldsLegend(structure.Fields),
	}})
}

func manageRenamePage(ctx context.Context, svc *Service, token *model.Token, actor Actor, args sheetManageArgs) (*mcp.CallToolResult, error) {
	pageID, errResult := parseSheetToolUUID(args.PageID, "page_id")
	if errResult != nil {
		return errResult, nil
	}
	if errResult := sheetToolGuardResult(svc.PageInTeam(ctx, token.OrgID, actor.TeamID, pageID)); errResult != nil {
		return errResult, nil
	}
	if args.Name == nil || strings.TrimSpace(*args.Name) == "" {
		return sheetToolError("rename_page requires a name"), nil
	}
	page, err := svc.UpdatePage(ctx, token.OrgID, pageID, UpdatePageRequest{Name: args.Name})
	if err != nil {
		return sheetToolError(err.Error()), nil
	}
	return sheetToolJSON(map[string]any{"page": map[string]any{"id": page.ID.String(), "name": page.Name}})
}

func manageArchivePage(ctx context.Context, svc *Service, token *model.Token, actor Actor, args sheetManageArgs) (*mcp.CallToolResult, error) {
	pageID, errResult := parseSheetToolUUID(args.PageID, "page_id")
	if errResult != nil {
		return errResult, nil
	}
	if errResult := sheetToolGuardResult(svc.PageInTeam(ctx, token.OrgID, actor.TeamID, pageID)); errResult != nil {
		return errResult, nil
	}
	if err := svc.ArchivePage(ctx, token.OrgID, pageID); err != nil {
		return sheetToolError(err.Error()), nil
	}
	return sheetToolJSON(map[string]any{"archived": true, "page_id": pageID.String()})
}

func manageAddField(ctx context.Context, svc *Service, token *model.Token, actor Actor, args sheetManageArgs) (*mcp.CallToolResult, error) {
	pageID, errResult := parseSheetToolUUID(args.PageID, "page_id")
	if errResult != nil {
		return errResult, nil
	}
	if errResult := sheetToolGuardResult(svc.PageInTeam(ctx, token.OrgID, actor.TeamID, pageID)); errResult != nil {
		return errResult, nil
	}
	name, fieldType := "", ""
	if args.Name != nil {
		name = *args.Name
	}
	if args.Type != nil {
		fieldType = *args.Type
	}
	if strings.TrimSpace(name) == "" || strings.TrimSpace(fieldType) == "" {
		return sheetToolError("add_field requires name and type"), nil
	}
	field, err := svc.CreateField(ctx, token.OrgID, pageID, FieldSpec{
		Name:    name,
		Type:    fieldType,
		Options: model.JSON(args.Options),
	}, actor)
	if err != nil {
		return sheetToolError(err.Error()), nil
	}
	return sheetToolJSON(map[string]any{"field": fieldObject(field)})
}

func manageUpdateField(ctx context.Context, svc *Service, token *model.Token, actor Actor, args sheetManageArgs) (*mcp.CallToolResult, error) {
	fieldID := strings.TrimSpace(args.FieldID)
	if !ValidFieldID(fieldID) {
		return sheetToolError("field_id must be a field ID like fld_8k2mx1q9"), nil
	}
	if errResult := sheetToolGuardResult(svc.FieldInTeam(ctx, token.OrgID, actor.TeamID, fieldID)); errResult != nil {
		return errResult, nil
	}
	req := UpdateFieldRequest{Name: args.Name, Type: args.Type}
	if args.Options != nil {
		options := model.JSON(args.Options)
		req.Options = &options
	}
	if req.Name == nil && req.Type == nil && req.Options == nil {
		return sheetToolError("update_field requires name, type, and/or options"), nil
	}
	field, err := svc.UpdateField(ctx, token.OrgID, fieldID, req, actor)
	if err != nil {
		return sheetToolError(err.Error()), nil
	}
	return sheetToolJSON(map[string]any{"field": fieldObject(field)})
}

func manageArchiveField(ctx context.Context, svc *Service, token *model.Token, actor Actor, args sheetManageArgs) (*mcp.CallToolResult, error) {
	fieldID := strings.TrimSpace(args.FieldID)
	if !ValidFieldID(fieldID) {
		return sheetToolError("field_id must be a field ID like fld_8k2mx1q9"), nil
	}
	if errResult := sheetToolGuardResult(svc.FieldInTeam(ctx, token.OrgID, actor.TeamID, fieldID)); errResult != nil {
		return errResult, nil
	}
	if err := svc.ArchiveField(ctx, token.OrgID, fieldID, actor); err != nil {
		return sheetToolError(err.Error()), nil
	}
	return sheetToolJSON(map[string]any{"archived": true, "field_id": fieldID})
}
