---
name: sheets
description: Use when work produces structured or tabular data the user should keep, browse, or edit — leads, research results, inventories, comparisons — or when the user asks for a sheet, database, table, tracker, or spreadsheet. This is the execution skill for the sheet MCP tools: sheet creation with typed fields, batch row writes, filter-AST queries, CSV import, relations, attachments, and operation revert.
---

# Sheets

Use this skill whenever work produces structured, row-shaped output the user will revisit: lead lists, research results, scraped data, inventories, comparisons, trackers. Sheets are first-party, channel-scoped storage that users browse, filter, and edit live in the Sheets panel of the web app. A sheet belongs to the channel the session runs in — `sheet_list`, `sheet_create`, and every read or write operate within that channel, and you only see and touch sheets in it.

Prefer a sheet over a markdown table or a loose CSV file for anything the user will keep, browse, edit, or ask about later. A markdown table in chat is disposable; a sheet is durable, queryable, and editable by the user. Only use inline markdown tables for small throwaway summaries inside a reply.

## Concepts

The hierarchy is **sheet → pages → fields → rows**:

- A **sheet** is the container (e.g. "Q3 Lead Research"). It has a name, description, and one or more pages.
- A **page** is a tab inside a sheet with its own fields and rows (e.g. "Competitors" and "Deals" as two pages of one sheet).
- A **field** is a typed column. Every field has a stable ID like `fld_8k2mx1q9` (`fld_` + random base36), a display name, a type, and type-specific `options`.
- A **row** stores cell values in a `data` object **keyed by field ID, never by field name**: `{"fld_8k2mx1q9": "Acme"}`. Field names are display labels; renaming a column never touches row data.

Because payloads key by field ID, you cannot write or filter correctly from memory of column names. **Always call `sheet_describe` first** to get the legend mapping field IDs to names/types/options, and use those IDs in every `rows_write` and `rows_query` payload. `rows_query` also returns a `fields_legend` with each result so you can translate data back to human-readable columns.

**Views** are saved lenses (filters, sorts, hidden columns) that belong to users. Do not create, modify, or rely on views — work with pages, fields, and rows only.

## Core Workflow

Run this loop for sheet work:

1. **`sheet_list`** — see what already exists in this channel. Reuse an existing sheet when one fits the work; never create a duplicate "Leads v2" when "Leads" exists in the channel.

```json
{}
```

2. **`sheet_create`** — only when no existing sheet fits. Define properly typed fields up front. A leads/competitors example:

```json
{
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
}
```

The response returns the sheet ID, page IDs, and the generated field IDs — capture them; you need the field IDs for every write.

3. **`sheet_describe`** — before any write or query against a sheet you did not just create in this session, fetch the full structure (pages, fields with id/name/type/options, row counts):

```json
{ "sheet_id": "5f0b6c1e-…" }
```

4. **`rows_write`** with `action: "insert"` — batches of **at most 100 rows per call**. Split larger sets into multiple calls. `data` keys are field IDs from the describe legend:

```json
{
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
}
```

Updates take `[{id, data}]` and merge **only the keys you send** (partial merge — other cells are untouched). Deletes take an array of row IDs and archive them:

```json
{
  "page_id": "9a2d4e7f-…",
  "action": "update",
  "rows": [
    { "id": "c31a9d20-…", "data": { "fld_2j9xc4hb": "direct" } }
  ]
}
```

```json
{ "page_id": "9a2d4e7f-…", "action": "delete", "ids": ["c31a9d20-…", "d47b0e31-…"] }
```

5. **`rows_query`** — filter, sort, search, and page through rows. Returns `{rows: [{id, data}], fields_legend, next_cursor}`; the limit is clamped to 100, so follow `next_cursor` until it is empty when you need everything:

```json
{
  "page_id": "9a2d4e7f-…",
  "filter": {
    "and": [
      { "field": "fld_2j9xc4hb", "op": "eq", "value": "direct" },
      { "field": "fld_7t5ry1ns", "op": "gte", "value": 50 }
    ]
  },
  "sorts": [{ "field": "fld_7t5ry1ns", "direction": "desc" }],
  "limit": 100
}
```

6. **`sheet_manage`** — schema changes after creation, dispatched by `action` (`rename_sheet | archive_sheet | create_page | rename_page | archive_page | add_field | update_field | archive_field`):

```json
{ "action": "add_field", "page_id": "9a2d4e7f-…", "name": "Status", "type": "select", "options": { "choices": ["new", "contacted", "qualified", "lost"] } }
```

```json
{ "action": "create_page", "sheet_id": "5f0b6c1e-…", "name": "Deals" }
```

```json
{ "action": "update_field", "field_id": "fld_2j9xc4hb", "options": { "choices": ["new", "contacted", "qualified", "won", "lost"] } }
```

There is no `rename_field` action — renaming a field is `update_field` with a new `name` (safe at any time: cells key by field ID, so renames never touch data). Use `archive_field` / `archive_page` / `archive_sheet` to retire things without destroying row data.

**Page vs new sheet:** add a page when it is the same project but a different entity (Competitors + Deals + Contacts in one research sheet). Create a new sheet when it is a different project or the user would look for it separately.

## CSV Import

Rule of thumb by size:

- **Fewer than ~500 rows:** skip CSV entirely — insert with `rows_write` in batches of ≤100. It is simpler, synchronous, and each batch is individually revertible.
- **~500 rows or more:** write a CSV in the sandbox, upload it to the agent drive, then start an async import.

Large-file walkthrough:

1. Write the CSV to a sandbox file with a header row.
2. Upload it via the drive endpoint (see the drive skill) and capture the `key` from the JSON response:

```bash
object_key=$(
  curl -fsS --retry 3 -X PUT \
    -H "Authorization: Bearer $HIVY_DRIVE_UPLOAD_BEARER" \
    -H "Content-Type: text/csv" \
    --upload-file ./leads.csv \
    "$HIVY_DRIVE_UPLOAD_URL/imports/leads.csv" | jq -r '.key'
)
```

3. Call `sheet_import_csv` with the target page and the object key exactly as the drive returned it — your drive keys look like `pub/e/{agentID}/…` and are accepted directly (org-owned `pub/o/{orgID}/…` keys work too). Map columns to existing field IDs with `field_mapping`, or set `create_fields: true` to let the importer create fields with type inference (number/date/checkbox/email/url detection, falling back to `text`):

```json
{
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
}
```

The tool returns a `job_id` and the import runs asynchronously (statuses: `pending → running → completed | failed`). Poll `sheet_import_csv` with `action: "status"` until it finishes:

```json
{ "action": "status", "job_id": "b3f8c2d1-…" }
```

The status response carries `status`, `processed_rows`, `total_rows`, and `error`. Do not report the import as done until status is `completed`; if it is `failed`, surface the `error` to the user. As a secondary check, a finished import also appears in `sheet_operations` `action: "list"` as a `csv_import` operation with its row count, and `sheet_describe` shows the updated row counts. CSV uploads are capped at 50 MB; pages are soft-capped at 200,000 rows, and an import beyond that fails fast with a clear error.

## Field Types Reference

All 12 field types, the value shape to send in `data`, and how the service coerces:

| Type | Cell value | `options` example | Coercion / validation |
|---|---|---|---|
| `text` | string | `{}` | Short strings; ≤16 KB like every cell |
| `long_text` | string | `{}` | Multi-paragraph text; still plain text, no HTML blobs |
| `number` | number | `{"format": "decimal"}` | Numeric strings like `"42"` are coerced; non-numeric values are rejected |
| `checkbox` | boolean | `{}` | Send `true`/`false`; truthy strings are coerced |
| `select` | string | `{"choices": ["new", "qualified", "lost"]}` | Must match a defined choice; ≤500 choices per field |
| `multi_select` | array of strings | `{"choices": ["saas", "fintech", "ai"]}` | Each entry must match a defined choice |
| `date` | ISO 8601 string | `{}` | Send `"2026-07-02"` or `"2026-07-02T09:30:00Z"`; parsed to a timestamp |
| `url` | string | `{}` | Validated as a URL |
| `email` | string | `{}` | Validated as an email address |
| `phone` | string | `{}` | Stored as text; keep a consistent format like E.164 |
| `attachment` | array of object keys | `{}` | Org-owned keys (`pub/o/{orgID}/…`) or your own drive uploads (`pub/e/{agentID}/…`); other orgs' keys are rejected; ≤10 per cell, ≤25 MB per file |
| `relation` | array of row UUIDs | `{"target_page_id": "…"}` | Every ID must be a live row on the target page in the same org; ≤100 links per cell |

### Relations — worked example

Link deals to competitor rows. The target page lives in the field's `options.target_page_id`:

```json
{ "action": "add_field", "page_id": "<deals-page-id>", "name": "Competitor", "type": "relation", "options": { "target_page_id": "<competitors-page-id>" } }
```

Query competitor rows first to get their UUIDs, then write the link as an array of row IDs:

```json
{
  "page_id": "<deals-page-id>",
  "action": "insert",
  "rows": [
    { "data": { "fld_dealname1": "Acme renewal", "fld_relcomp99": ["c31a9d20-…"] } }
  ]
}
```

When reading, pass `resolve_relations: true` to `rows_query`. This does **not** rewrite the cells in place — each row's `data` still holds bare UUIDs. Instead the response carries a separate top-level `relations` map of `{id, label}` pairs (the label comes from the target page's display field); join `data`'s UUIDs against it to render human-readable links. Relation filtering supports `contains` (linked to row X), `is_empty`, and `is_not_empty` only — you cannot filter through a relation into the target row's fields (e.g. "where linked company's status = qualified" is not supported; query the target page separately and use the resulting row IDs).

### Attachments — worked example

Attachment cells accept object keys your org owns: org assets (`pub/o/{orgID}/…`, e.g. files uploaded by users through the platform's `sign(sheet_attachment)` flow, which land under `pub/o/{orgID}/sheets/attachments/`) and your own agent drive uploads (`pub/e/{agentID}/…`). To attach a file you produced, upload it via the drive endpoint (see the drive skill), capture the returned `key`, and write that key into the cell — keys belonging to another org or another org's agents are rejected:

```json
{
  "page_id": "9a2d4e7f-…",
  "action": "update",
  "rows": [
    { "id": "c31a9d20-…", "data": { "fld_pitchdeck7": ["pub/e/…/reports/acme-deck.pdf"] } }
  ]
}
```

Never put file contents, base64, or data URIs into a cell — cells hold object keys, the platform serves downloads.

## Filter AST

Filters are a nested AST of `and`/`or` groups over `{field, op, value}` conditions. `field` is always a field ID; `op` is one of `eq neq contains not_contains starts_with gt gte lt lte is_empty is_not_empty in`. Casts follow the field type, so send numbers for number fields and ISO strings for dates.

Good vs bad:

```json
{ "and": [{ "field": "fld_2j9xc4hb", "op": "eq", "value": "qualified" }] }
```
```json
{ "and": [{ "field": "Status", "op": "eq", "value": "qualified" }] }
```
Bad: `field` must be the field ID, never the display name — unknown IDs (including names) are rejected.

```json
{ "and": [ { "field": "fld_7t5ry1ns", "op": "gte", "value": 50 }, { "or": [ { "field": "fld_2j9xc4hb", "op": "eq", "value": "direct" }, { "field": "fld_2j9xc4hb", "op": "eq", "value": "adjacent" } ] } ] }
```
```json
{ "and": [ { "field": "fld_7t5ry1ns", "op": ">=", "value": 50 } ] }
```
Bad: symbols are not ops — use the whitelisted names (`gte`, not `>=`).

```json
{ "and": [{ "field": "fld_2j9xc4hb", "op": "in", "value": ["new", "contacted"] }] }
```
```json
{ "and": [{ "field": "fld_2j9xc4hb", "op": "in", "value": "new, contacted" }] }
```
Bad: `in` takes an array, not a comma-joined string.

```json
{ "and": [{ "field": "fld_6q1zv8mk", "op": "is_empty" }] }
```
```json
{ "and": [{ "field": "fld_6q1zv8mk", "op": "eq", "value": null }] }
```
Bad: emptiness checks use `is_empty` / `is_not_empty`, not `eq null`.

For fuzzy lookups across all columns, use the top-level `search` parameter of `rows_query` (substring match over the whole row) instead of building an `or` over every text field:

```json
{ "page_id": "9a2d4e7f-…", "search": "acme", "limit": 25 }
```

Results are capped at 100 rows per call. When `next_cursor` is non-empty, pass it back as `cursor` in the next call; keep going until it is empty before claiming you have all rows.

## Mistakes & Recovery

Every mutation (one `rows_write` call, one import, one field change) is recorded as an operation on its page. If a batch went wrong — wrong field IDs, bad values, duplicate insert, an import that mapped columns incorrectly — revert it instead of hand-patching:

1. List recent operations for the page (most recent first, with type, row count, and actor):

```json
{ "action": "list", "page_id": "9a2d4e7f-…" }
```

2. Revert the offending operation by ID:

```json
{ "action": "revert", "operation_id": "b82fd4c1-…" }
```

Revert applies the exact inverse: inserts are archived, updates restore prior values, deletes are un-archived, imports archive every imported row. Caveats:

- **Undo is single-level**: a revert is not itself re-undoable. Check the operation list carefully and revert the right operation the first time.
- The log keeps only the most recent operations per page, and older update before-images expire — revert promptly, not at the end of a long session.
- Revert exactly one operation at a time; a 5-batch bad insert is 5 reverts.

## Rules

- Always call `sheet_describe` before writing to or querying a sheet you did not create in this session. Never guess field IDs.
- Key `data` payloads and filters by field ID (`fld_…`), never by field name.
- Batch `rows_write` at ≤100 rows per call. Never loop single-row calls for bulk work; never exceed the cap.
- Reuse existing sheets from `sheet_list` (scoped to this channel); never create a duplicate sheet for the same data.
- Query fresh before bulk updates — humans edit rows in the web app while you work; stale row IDs and values will clobber their edits.
- Archive, don't destroy: deletes are archives, and `archive_field`/`archive_page`/`archive_sheet` are the only removal actions. Only archive things the user asked you to remove.
- No blobs in cells: no HTML, base64, file contents, or JSON dumps. Attachments are drive object keys; text is plain text.
- Respect limits: 16 KB per cell, 128 KB per row, 200 fields per page, 100 pages per sheet, 500 select choices, 10 attachments per cell, 100 relation links per cell, 50 MB CSV uploads.
- Do not touch users' views.
- Confirm async imports completed (operations list + row counts) before reporting success.

## Final Response Checklist

Before reporting completion, state:

- The sheet name and which page you worked in.
- What changed: rows inserted, updated, deleted, or imported (with counts), and any fields or pages added.
- Any operation you reverted and why.
- Where to find it: the user can open the sheet in the **Sheets panel** and edit it directly.
- Any assumptions (field types chosen, select choices defined) the user may want to adjust.
