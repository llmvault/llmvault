package sheets

import (
	"context"
	"testing"

	"github.com/google/uuid"
)

// TestSheetToolsLifecycle drives sheet_create, sheet_list, sheet_describe,
// and every sheet_manage action end to end against the test DB.
func TestSheetToolsLifecycle(t *testing.T) {
	ctx := context.Background()
	fixture, client := setupSheetTools(t)

	name := "Tool Lifecycle " + uuid.NewString()
	created := callSheetTool(t, ctx, client, toolSheetCreate, map[string]any{
		"name":        name,
		"description": "Created via MCP tool test.",
		"pages": []map[string]any{{
			"name": "Targets",
			"fields": []map[string]any{
				{"name": "Company", "type": "text"},
				{"name": "Tier", "type": "select", "options": map[string]any{"choices": []string{"direct", "adjacent"}}},
			},
		}},
	})
	sheetObj := created["sheet"].(map[string]any)
	sheetID := sheetObj["id"].(string)
	pages := created["pages"].([]any)
	if len(pages) != 1 {
		t.Fatalf("created pages = %d, want 1", len(pages))
	}
	pageObj := pages[0].(map[string]any)
	pageID := pageObj["id"].(string)
	fields := pageObj["fields"].([]any)
	if len(fields) != 2 {
		t.Fatalf("created fields = %d, want 2", len(fields))
	}
	firstField := fields[0].(map[string]any)
	if !ValidFieldID(firstField["id"].(string)) {
		t.Fatalf("created field id %q is not a fld_ id", firstField["id"])
	}

	// sheet_list finds the sheet with pages + row counts, scoped to this org.
	listed := callSheetTool(t, ctx, client, toolSheetList, map[string]any{"search": name})
	sheets := listed["sheets"].([]any)
	if len(sheets) != 1 {
		t.Fatalf("sheet_list found %d sheets for %q, want 1", len(sheets), name)
	}
	listedSheet := sheets[0].(map[string]any)
	listedPages := listedSheet["pages"].([]any)
	if len(listedPages) != 1 || listedPages[0].(map[string]any)["row_count"].(float64) != 0 {
		t.Fatalf("sheet_list pages/row_count mismatch: %#v", listedPages)
	}

	// sheet_describe returns the full structure.
	described := callSheetTool(t, ctx, client, toolSheetDescribe, map[string]any{"sheet_id": sheetID})
	descPages := described["pages"].([]any)
	if len(descPages) != 1 || len(descPages[0].(map[string]any)["fields"].([]any)) != 2 {
		t.Fatalf("sheet_describe structure mismatch: %#v", described)
	}

	// rename_sheet.
	renamed := callSheetTool(t, ctx, client, toolSheetManage, map[string]any{
		"action": "rename_sheet", "sheet_id": sheetID, "name": name + " v2", "description": "renamed",
	})
	if renamed["sheet"].(map[string]any)["name"] != name+" v2" {
		t.Fatalf("rename_sheet did not rename: %#v", renamed)
	}

	// create_page + rename_page.
	pageCreated := callSheetTool(t, ctx, client, toolSheetManage, map[string]any{
		"action": "create_page", "sheet_id": sheetID, "name": "Deals",
	})
	newPageID := pageCreated["page"].(map[string]any)["id"].(string)
	pageRenamed := callSheetTool(t, ctx, client, toolSheetManage, map[string]any{
		"action": "rename_page", "page_id": newPageID, "name": "Closed Deals",
	})
	if pageRenamed["page"].(map[string]any)["name"] != "Closed Deals" {
		t.Fatalf("rename_page did not rename: %#v", pageRenamed)
	}

	// add_field, update_field (rename + options), archive_field.
	fieldAdded := callSheetTool(t, ctx, client, toolSheetManage, map[string]any{
		"action": "add_field", "page_id": pageID, "name": "Status", "type": "select",
		"options": map[string]any{"choices": []string{"new", "contacted"}},
	})
	fieldID := fieldAdded["field"].(map[string]any)["id"].(string)
	fieldUpdated := callSheetTool(t, ctx, client, toolSheetManage, map[string]any{
		"action": "update_field", "field_id": fieldID, "name": "Stage",
		"options": map[string]any{"choices": []string{"new", "contacted", "won"}},
	})
	if fieldUpdated["field"].(map[string]any)["name"] != "Stage" {
		t.Fatalf("update_field did not rename: %#v", fieldUpdated)
	}
	callSheetTool(t, ctx, client, toolSheetManage, map[string]any{
		"action": "archive_field", "field_id": fieldID,
	})
	assertSheetToolError(t, ctx, client, toolSheetManage, map[string]any{
		"action": "update_field", "field_id": fieldID, "name": "Gone",
	}, "not found")

	// archive_page and archive_sheet.
	callSheetTool(t, ctx, client, toolSheetManage, map[string]any{"action": "archive_page", "page_id": newPageID})
	callSheetTool(t, ctx, client, toolSheetManage, map[string]any{"action": "archive_sheet", "sheet_id": sheetID})
	assertSheetToolError(t, ctx, client, toolSheetDescribe, map[string]any{"sheet_id": sheetID}, "not found")

	// Bad dispatch values surface as IsError results, not transport errors.
	assertSheetToolError(t, ctx, client, toolSheetManage, map[string]any{"action": "explode"}, "action must be one of")
	assertSheetToolError(t, ctx, client, toolSheetManage, map[string]any{"action": "add_field", "page_id": pageID}, "requires name and type")
	assertSheetToolError(t, ctx, client, toolSheetCreate, map[string]any{"name": ""}, "name is required")
	assertSheetToolError(t, ctx, client, toolSheetCreate, map[string]any{
		"name":  "Bad Field Type " + uuid.NewString(),
		"pages": []map[string]any{{"name": "P", "fields": []map[string]any{{"name": "X", "type": "geo"}}}},
	}, "unknown field type")

	// The fixture org's other sheets remain untouched.
	if _, err := fixture.svc.SheetByID(ctx, fixture.org.ID, fixture.sheet.Sheet.ID); err != nil {
		t.Fatalf("fixture sheet disappeared: %v", err)
	}
}
