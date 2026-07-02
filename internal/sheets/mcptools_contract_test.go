package sheets

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

// The payloads below are copied VERBATIM from the shipped skill at
// global/plugins/sheets/skills/sheets/SKILL.md (Core Workflow, Filter AST,
// CSV Import, Mistakes & Recovery). The skill is the payload contract: if a
// schema change breaks this test, the SKILL.md must change in the same
// commit. Placeholder IDs ("5f0b6c1e-…", "fld_8k2mx1q9", …) are swapped for
// real ones at runtime; the JSON shape is untouched.
const (
	skillSheetList   = `{}`
	skillSheetCreate = `{
  "name": "SaaS Competitor Research",
  "description": "Competitors and outreach leads for the Q3 positioning project.",
  "pages": [
    {
      "name": "Competitors",
      "fields": [
        { "name": "Company", "type": "text", "options": {} },
        { "name": "Website", "type": "url", "options": {} },
        { "name": "Contact Email", "type": "email", "options": {} },
        { "name": "Tier", "type": "select", "options": { "choices": ["direct", "adjacent", "aspirational"] } },
        { "name": "Employees", "type": "number", "options": {} },
        { "name": "Notes", "type": "long_text", "options": {} }
      ]
    }
  ]
}`
	skillSheetDescribe = `{ "sheet_id": "5f0b6c1e-…" }`
	skillRowsInsert    = `{
  "page_id": "9a2d4e7f-…",
  "action": "insert",
  "rows": [
    {
      "data": {
        "fld_8k2mx1q9": "Acme Corp",
        "fld_3n7pw2rt": "https://acme.example.com",
        "fld_6q1zv8mk": "founders@acme.example.com",
        "fld_2j9xc4hb": "direct",
        "fld_7t5ry1ns": 220
      }
    },
    {
      "data": {
        "fld_8k2mx1q9": "Bolt Analytics",
        "fld_3n7pw2rt": "https://bolt.example.io",
        "fld_2j9xc4hb": "adjacent",
        "fld_7t5ry1ns": 45
      }
    }
  ]
}`
	skillRowsUpdate = `{
  "page_id": "9a2d4e7f-…",
  "action": "update",
  "rows": [
    { "id": "c31a9d20-…", "data": { "fld_2j9xc4hb": "direct" } }
  ]
}`
	skillRowsDelete = `{ "page_id": "9a2d4e7f-…", "action": "delete", "ids": ["c31a9d20-…", "d47b0e31-…"] }`
	skillRowsQuery  = `{
  "page_id": "9a2d4e7f-…",
  "filter": {
    "and": [
      { "field": "fld_2j9xc4hb", "op": "eq", "value": "direct" },
      { "field": "fld_7t5ry1ns", "op": "gte", "value": 50 }
    ]
  },
  "sorts": [{ "field": "fld_7t5ry1ns", "direction": "desc" }],
  "limit": 100
}`
	skillManageAddField    = `{ "action": "add_field", "page_id": "9a2d4e7f-…", "name": "Status", "type": "select", "options": { "choices": ["new", "contacted", "qualified", "lost"] } }`
	skillManageCreatePage  = `{ "action": "create_page", "sheet_id": "5f0b6c1e-…", "name": "Deals" }`
	skillManageUpdateField = `{ "action": "update_field", "field_id": "fld_2j9xc4hb", "options": { "choices": ["new", "contacted", "qualified", "won", "lost"] } }`
	skillFilterGoodOr      = `{ "and": [ { "field": "fld_7t5ry1ns", "op": "gte", "value": 50 }, { "or": [ { "field": "fld_2j9xc4hb", "op": "eq", "value": "direct" }, { "field": "fld_2j9xc4hb", "op": "eq", "value": "adjacent" } ] } ] }`
	skillFilterGoodEq      = `{ "and": [{ "field": "fld_2j9xc4hb", "op": "eq", "value": "qualified" }] }`
	skillFilterGoodIn      = `{ "and": [{ "field": "fld_2j9xc4hb", "op": "in", "value": ["new", "contacted"] }] }`
	skillFilterGoodEmpty   = `{ "and": [{ "field": "fld_6q1zv8mk", "op": "is_empty" }] }`
	skillFilterBadName     = `{ "and": [{ "field": "Status", "op": "eq", "value": "qualified" }] }`
	skillFilterBadSymbol   = `{ "and": [ { "field": "fld_7t5ry1ns", "op": ">=", "value": 50 } ] }`
	skillFilterBadIn       = `{ "and": [{ "field": "fld_2j9xc4hb", "op": "in", "value": "new, contacted" }] }`
	skillFilterBadNull     = `{ "and": [{ "field": "fld_6q1zv8mk", "op": "eq", "value": null }] }`
	skillSearch            = `{ "page_id": "9a2d4e7f-…", "search": "acme", "limit": 25 }`
	skillImportStart       = `{
  "page_id": "9a2d4e7f-…",
  "object_key": "pub/o/…/imports/leads.csv",
  "options": {
    "has_header": true,
    "delimiter": ",",
    "field_mapping": {
      "Company": "fld_8k2mx1q9",
      "Website": "fld_3n7pw2rt",
      "Employees": "fld_7t5ry1ns"
    }
  }
}`
	skillImportStatus = `{ "action": "status", "job_id": "b3f8c2d1-…" }`
	skillOpsList      = `{ "action": "list", "page_id": "9a2d4e7f-…" }`
	skillOpsRevert    = `{ "action": "revert", "operation_id": "b82fd4c1-…" }`
)

// assertStrictDecode pins skill↔schema key consistency: every key in the
// documented payload must exist on the tool's argument struct.
func assertStrictDecode(t *testing.T, payload string, dst any) {
	t.Helper()
	dec := json.NewDecoder(strings.NewReader(payload))
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		t.Fatalf("SKILL.md payload does not decode into the tool args (schema drift): %v\n%s", err, payload)
	}
}

func TestSheetToolsSkillPayloadContract(t *testing.T) {
	ctx := context.Background()
	fixture, client := setupSheetTools(t)

	assertStrictDecode(t, skillSheetCreate, &sheetCreateArgs{})
	assertStrictDecode(t, skillSheetDescribe, &sheetDescribeArgs{})
	assertStrictDecode(t, skillRowsInsert, &rowsWriteArgs{})
	assertStrictDecode(t, skillRowsUpdate, &rowsWriteArgs{})
	assertStrictDecode(t, skillRowsDelete, &rowsWriteArgs{})
	assertStrictDecode(t, skillRowsQuery, &rowsQueryArgs{})
	assertStrictDecode(t, skillManageAddField, &sheetManageArgs{})
	assertStrictDecode(t, skillManageCreatePage, &sheetManageArgs{})
	assertStrictDecode(t, skillManageUpdateField, &sheetManageArgs{})
	assertStrictDecode(t, skillSearch, &rowsQueryArgs{})
	assertStrictDecode(t, skillImportStart, &sheetImportCSVArgs{})
	assertStrictDecode(t, skillImportStatus, &sheetImportCSVArgs{})
	assertStrictDecode(t, skillOpsList, &sheetOperationsArgs{})
	assertStrictDecode(t, skillOpsRevert, &sheetOperationsArgs{})
	for _, filter := range []string{skillFilterGoodOr, skillFilterGoodEq, skillFilterGoodIn, skillFilterGoodEmpty} {
		assertStrictDecode(t, filter, &Filter{})
	}

	// 1-2. sheet_list, then sheet_create with the documented payload.
	callSheetToolJSON(t, ctx, client, toolSheetList, skillSheetList)
	created := callSheetToolJSON(t, ctx, client, toolSheetCreate, skillSheetCreate)
	page := created["pages"].([]any)[0].(map[string]any)
	fieldByName := map[string]string{}
	for _, raw := range page["fields"].([]any) {
		field := raw.(map[string]any)
		fieldByName[field["name"].(string)] = field["id"].(string)
	}
	replace := strings.NewReplacer(
		"5f0b6c1e-…", created["sheet"].(map[string]any)["id"].(string),
		"9a2d4e7f-…", page["id"].(string),
		"fld_8k2mx1q9", fieldByName["Company"],
		"fld_3n7pw2rt", fieldByName["Website"],
		"fld_6q1zv8mk", fieldByName["Contact Email"],
		"fld_2j9xc4hb", fieldByName["Tier"],
		"fld_7t5ry1ns", fieldByName["Employees"],
		"pub/o/…/", OrgAttachmentPrefix(fixture.org.ID),
	).Replace

	// 3-4. describe, then insert; capture the documented row-id placeholders.
	callSheetToolJSON(t, ctx, client, toolSheetDescribe, replace(skillSheetDescribe))
	inserted := callSheetToolJSON(t, ctx, client, toolRowsWrite, replace(skillRowsInsert))
	ids := rowIDs(t, inserted)
	rowReplace := strings.NewReplacer("c31a9d20-…", ids[0], "d47b0e31-…", ids[1]).Replace

	// 5-6. update (partial merge), then the documented filter+sorts query.
	callSheetToolJSON(t, ctx, client, toolRowsWrite, rowReplace(replace(skillRowsUpdate)))
	queried := callSheetToolJSON(t, ctx, client, toolRowsQuery, replace(skillRowsQuery))
	if len(responseRows(t, queried)) != 1 {
		t.Fatalf("documented query returned %d rows, want 1 (Acme)", len(responseRows(t, queried)))
	}
	filterQuery := func(filter string) map[string]any {
		return callSheetToolJSON(t, ctx, client, toolRowsQuery,
			`{ "page_id": "`+replace("9a2d4e7f-…")+`", "filter": `+replace(filter)+` }`)
	}
	filterQuery(skillFilterGoodOr) // uses the original Tier choices

	// 7. sheet_manage documented payloads (update_field swaps Tier's choices).
	callSheetToolJSON(t, ctx, client, toolSheetManage, replace(skillManageAddField))
	callSheetToolJSON(t, ctx, client, toolSheetManage, replace(skillManageCreatePage))
	callSheetToolJSON(t, ctx, client, toolSheetManage, replace(skillManageUpdateField))

	// 8. Remaining good filters and the search payload.
	filterQuery(skillFilterGoodEq)
	filterQuery(skillFilterGoodIn)
	if got := len(responseRows(t, filterQuery(skillFilterGoodEmpty))); got != 1 {
		t.Fatalf("is_empty filter returned %d rows, want 1 (Bolt has no email)", got)
	}
	if got := len(responseRows(t, callSheetToolJSON(t, ctx, client, toolRowsQuery, replace(skillSearch)))); got != 1 {
		t.Fatalf("documented search returned %d rows, want 1", got)
	}

	// 9. The documented BAD payloads fail as IsError results with guidance.
	badFilter := func(filter, want string) {
		t.Helper()
		var args map[string]any
		payload := `{ "page_id": "` + replace("9a2d4e7f-…") + `", "filter": ` + replace(filter) + ` }`
		if err := json.Unmarshal([]byte(payload), &args); err != nil {
			t.Fatalf("bad-filter payload invalid: %v", err)
		}
		assertSheetToolError(t, ctx, client, toolRowsQuery, args, want)
	}
	badFilter(skillFilterBadName, "unknown field")
	badFilter(skillFilterBadSymbol, "not supported")
	badFilter(skillFilterBadIn, "in expects a non-empty array")
	badFilter(skillFilterBadNull, "requires a value")

	// 10. CSV import start (no action key — start is the default) + status.
	started := callSheetToolJSON(t, ctx, client, toolSheetImportCSV, replace(skillImportStart))
	jobID := started["job"].(map[string]any)["job_id"].(string)
	statusOut := callSheetToolJSON(t, ctx, client, toolSheetImportCSV,
		strings.ReplaceAll(skillImportStatus, "b3f8c2d1-…", jobID))
	if statusOut["job"].(map[string]any)["status"] != ImportStatusPending {
		t.Fatalf("documented status payload mismatch: %#v", statusOut)
	}

	// 11. delete, operations list, and revert with the documented payloads.
	callSheetToolJSON(t, ctx, client, toolRowsWrite, rowReplace(replace(skillRowsDelete)))
	opsOut := callSheetToolJSON(t, ctx, client, toolSheetOperations, replace(skillOpsList))
	deleteOpID := ""
	for _, raw := range opsOut["operations"].([]any) {
		op := raw.(map[string]any)
		if op["type"] == OpTypeRowsDelete {
			deleteOpID = op["id"].(string)
			break
		}
	}
	if deleteOpID == "" {
		t.Fatalf("operations list has no rows_delete entry: %#v", opsOut)
	}
	reverted := callSheetToolJSON(t, ctx, client, toolSheetOperations,
		strings.ReplaceAll(skillOpsRevert, "b82fd4c1-…", deleteOpID))
	if reverted["reverted"] != true {
		t.Fatalf("documented revert payload failed: %#v", reverted)
	}
}
