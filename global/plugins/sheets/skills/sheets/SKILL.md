---
name: sheets
description: Use when work creates or changes durable tabular data that users should browse, filter, and edit in Hivy Sheets: typed schema, rows, imports, relations, attachments, and operation revert.
---

# Hivy Sheets

Sheets are channel-scoped, durable storage for row-shaped work. Prefer one over a chat table or loose CSV when the user will revisit, filter, or edit the result. The tool schemas supply exact payload shapes; this skill defines the product rules.

## Core rules

- Start with `sheet_list`; reuse a matching sheet instead of creating a duplicate.
- A sheet contains pages, typed fields, and rows. Row data and filters use stable field IDs (`fld_…`), never display names.
- Call `sheet_describe` before querying or mutating a sheet not created in this session. Keep its page and field legend as the source of truth.
- Define appropriate field types when creating a sheet. Do not touch users' saved views.
- Insert or update at most 100 rows per write. Query fresh before a bulk update so a human edit is not overwritten.
- Archive only what the user asks to remove. If an operation is wrong, inspect `sheet_operations` and revert the specific operation instead of hand-patching.
- Cells hold ordinary values or Drive object keys for attachments—never file contents, base64, HTML blobs, or large JSON dumps.

## Standard workflow

1. List sheets and identify the page the work belongs to.
2. Create a typed sheet/page only when nothing fits. Capture returned IDs.
3. Describe the sheet, then use the field IDs for `rows_query`, `rows_write`, relation fields, and import mappings.
4. Write small sets in bounded batches. Use `rows_query` cursors until complete when the task needs all rows.
5. For roughly 500+ rows, write CSV, upload it with the Drive workflow, start `sheet_import_csv`, and wait for `completed`. Otherwise use `rows_write` directly.
6. Verify the resulting row count or query result before reporting completion.

## Important behavior

- Updates are partial merges of supplied cell keys. Filter operations and value types are validated by the tool schema; send field IDs and correctly typed values.
- Relation cells contain row IDs from the target page. Attachment cells contain object keys belonging to the org or the current agent's Drive uploads.
- Import is asynchronous. `running` is not success; surface an import error rather than claiming completion.
- Pages group related entities in one project. Create a separate sheet when users would look for the data as a separate project.

## Handoff

State the sheet and page, what changed and how many rows, any reverted operation, and assumptions such as field types or select choices. Tell the user that the result is available in the Sheets panel.
