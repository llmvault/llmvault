package e2e

// Payloads copied VERBATIM from the shipped skill at
// global/plugins/sheets/skills/sheets/SKILL.md (Core Workflow / CSV Import /
// Mistakes & Recovery). Placeholder IDs are swapped for real ones at runtime;
// the JSON shape is untouched. internal/sheets/mcptools_contract_test.go pins
// the full contract — this e2e drives the same documented payloads through
// the production BuildServer stack against the live compose stack.
const (
	sheetsE2ESkillSheetCreate = `{
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
	sheetsE2ESkillRowsInsert = `{
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
	sheetsE2ESkillRowsQuery = `{
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
	sheetsE2ESkillImportStart = `{
  "page_id": "9a2d4e7f-…",
  "object_key": "pub/e/…/imports/leads.csv",
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
	sheetsE2ESkillOpsList   = `{ "action": "list", "page_id": "9a2d4e7f-…" }`
	sheetsE2ESkillOpsRevert = `{ "action": "revert", "operation_id": "b82fd4c1-…" }`
)

var sheetsE2EToolNames = []string{
	"sheet_create", "sheet_list", "sheet_describe", "sheet_manage",
	"rows_query", "rows_write", "sheet_import_csv", "sheet_operations",
}
