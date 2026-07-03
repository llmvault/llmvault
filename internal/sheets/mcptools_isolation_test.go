package sheets

import (
	"testing"

	"github.com/google/uuid"

	"github.com/usehivy/hivy/internal/model"
)

// TestSheetToolsOrgIsolation proves an agent-proxy token for org A can
// neither see nor touch org B's sheets through ANY of the eight tools
// (KI-07 class). The fixture's otherOrg sheet/page/row are the targets.
func TestSheetToolsOrgIsolation(t *testing.T) {
	fixture, client := setupSheetTools(t)
	ctx := fixture.toolCtx()
	otherSheet := fixture.otherSheet.Sheet.ID.String()
	otherPage := fixture.otherPage.Page.ID.String()
	otherField := fixture.otherPage.Fields[0].ID
	otherRow := fixture.otherRow.ID.String()

	// Seed an import job and grab an operation in the other org directly.
	otherJob, err := fixture.svc.CreateImportJob(ctx, fixture.otherOrg.ID, fixture.otherPage.Page.ID, CreateImportJobRequest{
		ObjectKey: OrgAttachmentPrefix(fixture.otherOrg.ID) + "imports/secret.csv",
	}, Actor{})
	if err != nil {
		t.Fatalf("seed other-org import job: %v", err)
	}
	otherOps, err := fixture.svc.ListOperations(ctx, fixture.otherOrg.ID, fixture.otherPage.Page.ID, 1)
	if err != nil || len(otherOps) == 0 {
		t.Fatalf("seed other-org operation: %v (%d ops)", err, len(otherOps))
	}

	// 1. sheet_list never returns the other org's sheet.
	listed := callSheetTool(t, ctx, client, toolSheetList, map[string]any{})
	for _, item := range listed["sheets"].([]any) {
		if item.(map[string]any)["id"] == otherSheet {
			t.Fatalf("sheet_list leaked other org's sheet %s", otherSheet)
		}
	}

	// 2. sheet_describe.
	assertSheetToolError(t, ctx, client, toolSheetDescribe, map[string]any{"sheet_id": otherSheet}, "not found")

	// 3. sheet_manage — every entity reference is org-checked.
	assertSheetToolError(t, ctx, client, toolSheetManage, map[string]any{
		"action": "rename_sheet", "sheet_id": otherSheet, "name": "Hijacked",
	}, "not found")
	assertSheetToolError(t, ctx, client, toolSheetManage, map[string]any{
		"action": "archive_sheet", "sheet_id": otherSheet,
	}, "not found")
	assertSheetToolError(t, ctx, client, toolSheetManage, map[string]any{
		"action": "add_field", "page_id": otherPage, "name": "X", "type": "text",
	}, "not found")
	assertSheetToolError(t, ctx, client, toolSheetManage, map[string]any{
		"action": "update_field", "field_id": otherField, "name": "X",
	}, "not found")
	assertSheetToolError(t, ctx, client, toolSheetManage, map[string]any{
		"action": "archive_field", "field_id": otherField,
	}, "not found")

	// 4. rows_query.
	assertSheetToolError(t, ctx, client, toolRowsQuery, map[string]any{"page_id": otherPage}, "not found")

	// 5. rows_write — insert, update, and delete against the other page/row.
	assertSheetToolError(t, ctx, client, toolRowsWrite, map[string]any{
		"page_id": otherPage, "action": "insert",
		"rows": []map[string]any{{"data": map[string]any{otherField: "intruder"}}},
	}, "not found")
	assertSheetToolError(t, ctx, client, toolRowsWrite, map[string]any{
		"page_id": otherPage, "action": "update",
		"rows": []map[string]any{{"id": otherRow, "data": map[string]any{otherField: "intruder"}}},
	}, "not found")
	assertSheetToolError(t, ctx, client, toolRowsWrite, map[string]any{
		"page_id": otherPage, "action": "delete", "ids": []string{otherRow},
	}, "not found")
	// Cross-org row IDs through the caller's OWN page are invisible too.
	assertSheetToolError(t, ctx, client, toolRowsWrite, map[string]any{
		"page_id": fixture.leads.Page.ID.String(), "action": "update",
		"rows": []map[string]any{{"id": otherRow, "data": map[string]any{fixture.leads.Fields[0].ID: "x"}}},
	}, "not found")

	// 6. sheet_import_csv — start on their page, status on their job.
	assertSheetToolError(t, ctx, client, toolSheetImportCSV, map[string]any{
		"page_id": otherPage, "object_key": OrgAttachmentPrefix(fixture.org.ID) + "imports/x.csv",
	}, "not found")
	assertSheetToolError(t, ctx, client, toolSheetImportCSV, map[string]any{
		"action": "status", "job_id": otherJob.ID.String(),
	}, "not found")

	// 7. sheet_operations — list their page, revert their operation.
	assertSheetToolError(t, ctx, client, toolSheetOperations, map[string]any{
		"action": "list", "page_id": otherPage,
	}, "not found")
	assertSheetToolError(t, ctx, client, toolSheetOperations, map[string]any{
		"action": "revert", "operation_id": otherOps[0].ID.String(),
	}, "not found")

	// 8. sheet_create always lands in the caller's org.
	created := callSheetTool(t, ctx, client, toolSheetCreate, map[string]any{"name": "Isolation " + uuid.NewString()})
	createdID := mustUUID(t, created["sheet"].(map[string]any)["id"].(string))
	var sheet model.Sheet
	if err := fixture.db.First(&sheet, "id = ?", createdID).Error; err != nil {
		t.Fatalf("load created sheet: %v", err)
	}
	if sheet.OrgID != fixture.org.ID {
		t.Fatalf("sheet_create landed in org %s, want %s", sheet.OrgID, fixture.org.ID)
	}
	if sheet.CreatedByAgentID == nil || *sheet.CreatedByAgentID != fixture.agent.ID {
		t.Fatalf("sheet_create provenance mismatch: %#v", sheet.CreatedByAgentID)
	}

	// The other org's row is untouched by all of the above.
	reloaded := fixture.reloadRow(t, fixture.otherRow.ID)
	if reloaded.ArchivedAt != nil || reloaded.Data[otherField] != "classified" {
		t.Fatalf("other org's row was modified: %#v", reloaded)
	}
}
