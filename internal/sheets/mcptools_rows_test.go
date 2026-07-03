package sheets

import (
	"fmt"
	"testing"
)

// TestSheetToolsRowsAndOperations drives rows_write, rows_query (filters,
// sorts, search, cursor, resolve_relations, clamps), sheet_import_csv, and
// sheet_operations end to end.
func TestSheetToolsRowsAndOperations(t *testing.T) {
	fixture, client := setupSheetTools(t)
	ctx := fixture.toolCtx()
	leadsPage := fixture.leads.Page.ID.String()
	nameField := fixture.fieldByName(t, fixture.leads, "Name").ID
	scoreField := fixture.fieldByName(t, fixture.leads, "Score").ID
	statusField := fixture.fieldByName(t, fixture.leads, "Status").ID

	// Insert.
	inserted := callSheetTool(t, ctx, client, toolRowsWrite, map[string]any{
		"page_id": leadsPage, "action": "insert",
		"rows": []map[string]any{
			{"data": map[string]any{nameField: "Acme", scoreField: 90, statusField: "qualified"}},
			{"data": map[string]any{nameField: "Bolt", scoreField: 40, statusField: "new"}},
		},
	})
	if inserted["inserted"].(float64) != 2 {
		t.Fatalf("inserted = %v, want 2", inserted["inserted"])
	}
	ids := rowIDs(t, inserted)

	// Update is a partial merge: only the sent key changes.
	callSheetTool(t, ctx, client, toolRowsWrite, map[string]any{
		"page_id": leadsPage, "action": "update",
		"rows": []map[string]any{{"id": ids[1], "data": map[string]any{statusField: "qualified"}}},
	})
	row := fixture.reloadRow(t, mustUUID(t, ids[1]))
	if row.Data[nameField] != "Bolt" || row.Data[statusField] != "qualified" {
		t.Fatalf("partial merge failed: %#v", row.Data)
	}

	// Query: filter + sorts [{field,direction}] + search.
	queried := callSheetTool(t, ctx, client, toolRowsQuery, map[string]any{
		"page_id": leadsPage,
		"filter":  map[string]any{"and": []map[string]any{{"field": statusField, "op": "eq", "value": "qualified"}}},
		"sorts":   []map[string]any{{"field": scoreField, "direction": "desc"}},
	})
	rows := responseRows(t, queried)
	if len(rows) != 2 || rows[0]["data"].(map[string]any)[nameField] != "Acme" {
		t.Fatalf("filtered/sorted query mismatch: %#v", rows)
	}
	if len(queried["fields_legend"].([]any)) == 0 {
		t.Fatalf("query response missing fields_legend")
	}
	searched := callSheetTool(t, ctx, client, toolRowsQuery, map[string]any{"page_id": leadsPage, "search": "acme"})
	if len(responseRows(t, searched)) != 1 {
		t.Fatalf("search returned %d rows, want 1", len(responseRows(t, searched)))
	}

	// Error paths surface as IsError results.
	assertSheetToolError(t, ctx, client, toolRowsQuery, map[string]any{
		"page_id": leadsPage,
		"filter":  map[string]any{"and": []map[string]any{{"field": "fld_nope00000", "op": "eq", "value": "x"}}},
	}, "unknown field")
	assertSheetToolError(t, ctx, client, toolRowsQuery, map[string]any{
		"page_id": leadsPage,
		"filter":  map[string]any{"and": []map[string]any{{"field": scoreField, "op": ">=", "value": 1}}},
	}, "not supported")
	assertSheetToolError(t, ctx, client, toolRowsQuery, map[string]any{
		"page_id": leadsPage, "sorts": []map[string]any{{"field": scoreField, "direction": "sideways"}},
	}, "asc or desc")
	assertSheetToolError(t, ctx, client, toolRowsWrite, map[string]any{
		"page_id": leadsPage, "action": "upsert", "rows": []map[string]any{{"data": map[string]any{}}},
	}, "action must be one of")

	// Batch limit: 101 rows is rejected, never truncated.
	tooMany := make([]map[string]any, 0, MaxRowsPerWriteMCP+1)
	for i := 0; i <= MaxRowsPerWriteMCP; i++ {
		tooMany = append(tooMany, map[string]any{"data": map[string]any{nameField: fmt.Sprintf("Bulk %d", i)}})
	}
	assertSheetToolError(t, ctx, client, toolRowsWrite, map[string]any{
		"page_id": leadsPage, "action": "insert", "rows": tooMany,
	}, "limit exceeded")

	// Query limit clamp: fill the companies page past 100 rows, ask for 5000,
	// get exactly QueryLimitMCP plus a cursor that walks the rest.
	companiesPage := fixture.companies.Page.ID.String()
	companyField := fixture.fieldByName(t, fixture.companies, "Company").ID
	bulk := make([]map[string]any, 0, MaxRowsPerWriteMCP)
	for i := 0; i < MaxRowsPerWriteMCP; i++ {
		bulk = append(bulk, map[string]any{"data": map[string]any{companyField: fmt.Sprintf("Co %03d", i)}})
	}
	callSheetTool(t, ctx, client, toolRowsWrite, map[string]any{"page_id": companiesPage, "action": "insert", "rows": bulk})
	callSheetTool(t, ctx, client, toolRowsWrite, map[string]any{
		"page_id": companiesPage, "action": "insert",
		"rows": []map[string]any{
			{"data": map[string]any{companyField: "Tail 1"}},
			{"data": map[string]any{companyField: "Tail 2"}},
		},
	})
	page1 := callSheetTool(t, ctx, client, toolRowsQuery, map[string]any{"page_id": companiesPage, "limit": 5000})
	if len(responseRows(t, page1)) != QueryLimitMCP {
		t.Fatalf("clamped query returned %d rows, want %d", len(responseRows(t, page1)), QueryLimitMCP)
	}
	cursor, _ := page1["next_cursor"].(string)
	if cursor == "" {
		t.Fatalf("clamped query returned no next_cursor")
	}
	page2 := callSheetTool(t, ctx, client, toolRowsQuery, map[string]any{"page_id": companiesPage, "cursor": cursor})
	if len(responseRows(t, page2)) != 2 || page2["next_cursor"].(string) != "" {
		t.Fatalf("cursor page mismatch: %d rows, cursor %q", len(responseRows(t, page2)), page2["next_cursor"])
	}

	// resolve_relations hydrates linked rows into {id,label} chips.
	relField := fixture.createRelationField(t, fixture.companies.Page.ID)
	companyID := rowIDs(t, page2)[0]
	callSheetTool(t, ctx, client, toolRowsWrite, map[string]any{
		"page_id": leadsPage, "action": "update",
		"rows": []map[string]any{{"id": ids[0], "data": map[string]any{relField.ID: []string{companyID}}}},
	})
	hydrated := callSheetTool(t, ctx, client, toolRowsQuery, map[string]any{
		"page_id": leadsPage, "search": "acme", "resolve_relations": true,
	})
	relations := hydrated["relations"].(map[string]any)
	if relations[companyID] == nil || relations[companyID].(map[string]any)["label"] == "" {
		t.Fatalf("relation not hydrated: %#v", relations)
	}

	// Delete archives; operations list shows it newest-first; revert restores.
	deleted := callSheetTool(t, ctx, client, toolRowsWrite, map[string]any{
		"page_id": leadsPage, "action": "delete", "ids": []string{ids[0]},
	})
	if deleted["deleted"].(float64) != 1 {
		t.Fatalf("deleted = %v, want 1", deleted["deleted"])
	}
	if fixture.reloadRow(t, mustUUID(t, ids[0])).ArchivedAt == nil {
		t.Fatalf("delete did not archive row %s", ids[0])
	}
	opsOut := callSheetTool(t, ctx, client, toolSheetOperations, map[string]any{"action": "list", "page_id": leadsPage})
	ops := opsOut["operations"].([]any)
	if len(ops) == 0 || ops[0].(map[string]any)["type"] != OpTypeRowsDelete {
		t.Fatalf("operations list mismatch: %#v", ops)
	}
	opID := ops[0].(map[string]any)["id"].(string)
	if ops[0].(map[string]any)["actor_agent_id"] != fixture.agent.ID.String() {
		t.Fatalf("operation actor mismatch: %#v", ops[0])
	}
	callSheetTool(t, ctx, client, toolSheetOperations, map[string]any{"action": "revert", "operation_id": opID})
	if fixture.reloadRow(t, mustUUID(t, ids[0])).ArchivedAt != nil {
		t.Fatalf("revert did not un-archive row %s", ids[0])
	}
	assertSheetToolError(t, ctx, client, toolSheetOperations, map[string]any{
		"action": "revert", "operation_id": opID,
	}, "already reverted")

	// CSV import: start (default action) + status; foreign keys rejected.
	jobOut := callSheetTool(t, ctx, client, toolSheetImportCSV, map[string]any{
		"page_id":    leadsPage,
		"object_key": OrgAttachmentPrefix(fixture.org.ID) + "imports/leads.csv",
		"options":    map[string]any{"has_header": true, "create_fields": true},
	})
	job := jobOut["job"].(map[string]any)
	if job["status"] != ImportStatusPending {
		t.Fatalf("import job status = %v, want pending (no enqueuer wired)", job["status"])
	}
	statusOut := callSheetTool(t, ctx, client, toolSheetImportCSV, map[string]any{
		"action": "status", "job_id": job["job_id"].(string),
	})
	if statusOut["job"].(map[string]any)["status"] != ImportStatusPending {
		t.Fatalf("import status mismatch: %#v", statusOut)
	}
	assertSheetToolError(t, ctx, client, toolSheetImportCSV, map[string]any{
		"page_id":    leadsPage,
		"object_key": OrgAttachmentPrefix(fixture.otherOrg.ID) + "imports/leads.csv",
	}, "not owned by this org")
}
