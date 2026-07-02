package sheets

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/model"
)

// Tool names for the agent-facing sheets MCP tool group (plan §1).
const (
	toolSheetCreate     = "sheet_create"
	toolSheetList       = "sheet_list"
	toolSheetDescribe   = "sheet_describe"
	toolSheetManage     = "sheet_manage"
	toolRowsQuery       = "rows_query"
	toolRowsWrite       = "rows_write"
	toolSheetImportCSV  = "sheet_import_csv"
	toolSheetOperations = "sheet_operations"
)

// SheetsPluginSlug is the plugin whose per-agent install gates this tool
// group. Installing the plugin grants both the skill (existing machinery)
// and these tools.
const SheetsPluginSlug = "sheets"

const (
	// sheetToolTimeout bounds every tool call's service work.
	sheetToolTimeout = 20 * time.Second
	// sheetImportStartTimeout bounds import-job creation (includes the
	// enqueue hop once Phase 4 wires the Asynq enqueuer).
	sheetImportStartTimeout = 5 * time.Second
)

// NewToolsFunc returns the sheets ToolsFunc. It registers the eight sheets
// tools on the MCP server ONLY when the calling token is an agent proxy for
// an active agent that has the `sheets` plugin installed
// (agent_plugin_installs), mirroring the conditional-gating precedent of
// agents.NewToolsFunc/agentBuilderEnabled. Registering nothing when the
// plugin is absent keeps the tool surface out of un-enrolled agents' prompts
// entirely.
func NewToolsFunc(svc *Service) func(server *mcp.Server, token *model.Token) {
	return func(server *mcp.Server, token *model.Token) {
		if server == nil || svc == nil || svc.db == nil || !sheetToolAgentProxy(token) {
			return
		}
		agentID, err := sheetToolAgentID(token)
		if err != nil {
			return
		}
		ctx, cancel := context.WithTimeout(context.Background(), sheetToolTimeout)
		defer cancel()
		var agent model.Agent
		if err := svc.db.WithContext(ctx).
			Where("id = ? AND org_id = ? AND status <> ?", agentID, token.OrgID, "archived").
			First(&agent).Error; err != nil {
			return
		}
		if !sheetsPluginInstalled(ctx, svc.db, token.OrgID, agentID) {
			return
		}
		registerSheetCreate(server, svc, token, agentID)
		registerSheetList(server, svc, token)
		registerSheetDescribe(server, svc, token)
		registerSheetManage(server, svc, token, agentID)
		registerRowsQuery(server, svc, token)
		registerRowsWrite(server, svc, token, agentID)
		registerSheetImportCSV(server, svc, token, agentID)
		registerSheetOperations(server, svc, token, agentID)
	}
}

// sheetsPluginInstalled reports whether the sheets plugin is installed for
// this agent: an agent_plugin_installs row joined to the active plugin with
// slug "sheets", scoped to the caller's org.
func sheetsPluginInstalled(ctx context.Context, db *gorm.DB, orgID, agentID uuid.UUID) bool {
	var count int64
	err := db.WithContext(ctx).Model(&model.AgentPluginInstall{}).
		Joins("JOIN plugins ON plugins.id = agent_plugin_installs.plugin_id").
		Where("agent_plugin_installs.agent_id = ? AND agent_plugin_installs.org_id = ? AND plugins.slug = ? AND plugins.status = ?",
			agentID, orgID, SheetsPluginSlug, model.PluginStatusActive).
		Count(&count).Error
	return err == nil && count > 0
}

func sheetToolAgentProxy(token *model.Token) bool {
	if token == nil || token.Meta == nil {
		return false
	}
	tokenType, _ := token.Meta[model.TokenMetaType].(string)
	return tokenType == model.TokenTypeAgentProxy
}

func sheetToolAgentID(token *model.Token) (uuid.UUID, error) {
	agentIDText, _ := token.Meta[model.TokenMetaAgentID].(string)
	agentID, err := uuid.Parse(strings.TrimSpace(agentIDText))
	if err != nil || agentID == uuid.Nil {
		return uuid.Nil, fmt.Errorf("agent proxy token is missing agent_id")
	}
	return agentID, nil
}

// sheetToolActor resolves the mutation Actor: the calling agent plus the
// runtime-injected _hivy_session_id (recorded as source_session_id on
// created sheets/rows/operations). The session, when present, must belong to
// the caller's org.
func (s *Service) sheetToolActor(ctx context.Context, token *model.Token, agentID uuid.UUID, rawSessionID string) (Actor, error) {
	actor := Actor{AgentID: &agentID}
	sessionIDText := strings.TrimSpace(rawSessionID)
	if sessionIDText == "" {
		return actor, nil
	}
	sessionID, err := uuid.Parse(sessionIDText)
	if err != nil || sessionID == uuid.Nil {
		return actor, fmt.Errorf("_hivy_session_id must be a valid UUID")
	}
	var count int64
	if err := s.db.WithContext(ctx).Model(&model.Session{}).
		Where("id = ? AND org_id = ?", sessionID, token.OrgID).
		Count(&count).Error; err != nil || count == 0 {
		return actor, fmt.Errorf("session not found in this org")
	}
	actor.SessionID = &sessionID
	return actor, nil
}

// --- shared plumbing ---------------------------------------------------------

func sheetToolContext(ctx context.Context) (context.Context, context.CancelFunc) {
	return context.WithTimeout(ctx, sheetToolTimeout)
}

func decodeSheetToolArgs(req *mcp.CallToolRequest, dst any) *mcp.CallToolResult {
	if req == nil || req.Params.Arguments == nil {
		return nil // no arguments is valid for optional-only payloads
	}
	if err := json.Unmarshal(req.Params.Arguments, dst); err != nil {
		return sheetToolError("invalid arguments")
	}
	return nil
}

func sheetToolError(msg string) *mcp.CallToolResult {
	return &mcp.CallToolResult{
		Content: []mcp.Content{&mcp.TextContent{Text: "Error: " + msg}},
		IsError: true,
	}
}

func sheetToolJSON(v any) (*mcp.CallToolResult, error) {
	b, err := json.Marshal(v)
	if err != nil {
		return sheetToolError("failed to serialize response"), nil
	}
	return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: string(b)}}}, nil
}

func parseSheetToolUUID(raw, name string) (uuid.UUID, *mcp.CallToolResult) {
	id, err := uuid.Parse(strings.TrimSpace(raw))
	if err != nil || id == uuid.Nil {
		return uuid.Nil, sheetToolError(name + " must be a valid UUID")
	}
	return id, nil
}

// --- response shaping --------------------------------------------------------

func sheetObject(sheet *model.Sheet) map[string]any {
	return map[string]any{
		"id":          sheet.ID.String(),
		"slug":        sheet.Slug,
		"name":        sheet.Name,
		"description": sheet.Description,
		"icon":        sheet.Icon,
		"created_at":  sheet.CreatedAt,
		"updated_at":  sheet.UpdatedAt,
	}
}

func fieldObject(field *model.SheetField) map[string]any {
	return map[string]any{
		"id":      field.ID,
		"name":    field.Name,
		"type":    field.Type,
		"options": map[string]any(field.Options),
	}
}

// fieldsLegend maps field IDs to names/types/options so agents can translate
// `data` payloads (always keyed by field ID) back to human columns.
func fieldsLegend(fields []model.SheetField) []map[string]any {
	out := make([]map[string]any, 0, len(fields))
	for i := range fields {
		out = append(out, fieldObject(&fields[i]))
	}
	return out
}

func rowObjects(rows []model.SheetRow) []map[string]any {
	out := make([]map[string]any, 0, len(rows))
	for i := range rows {
		out = append(out, map[string]any{
			"id":   rows[i].ID.String(),
			"data": map[string]any(rows[i].Data),
		})
	}
	return out
}

// sheetStructureObject shapes a sheet with its pages, field legends, and row
// counts — the shared response of sheet_create and sheet_describe.
func sheetStructureObject(structure *SheetStructure, rowCounts map[uuid.UUID]int64) map[string]any {
	pages := make([]map[string]any, 0, len(structure.Pages))
	for i := range structure.Pages {
		page := &structure.Pages[i]
		pages = append(pages, map[string]any{
			"id":        page.Page.ID.String(),
			"name":      page.Page.Name,
			"row_count": rowCounts[page.Page.ID],
			"fields":    fieldsLegend(page.Fields),
		})
	}
	return map[string]any{
		"sheet": sheetObject(&structure.Sheet),
		"pages": pages,
	}
}
