---
name: canvas
description: Use when creating, opening, inspecting, editing, exporting, or collaborating on Hivy Canvas design files from a sandbox using the built-in `canvas` CLI, the isolated Canvas browser session, or Canvas MCP tools.
---

# Hivy Canvas

Use the built-in `canvas` CLI to work in Hivy-managed Canvas files from the sandbox. The CLI hides authentication, Canvas session setup, and MCP protocol details. From the agent's perspective, Canvas work is a sequence of CLI commands; from the user's perspective, the file updates live in their Canvas iframe/browser.

## Table of Contents

1. [Operating model](#operating-model)
2. [Core rules](#core-rules)
3. [Safety-first workflow](#safety-first-workflow)
4. [Prompt and design quality guardrails](#prompt-and-design-quality-guardrails)
5. [Workflow references](#workflow-references)
6. [CLI commands](#cli-commands)
7. [Standard Canvas workflows](#standard-canvas-workflows)
8. [Available MCP tools](#available-mcp-tools)
9. [Canvas Plugin API patterns](#canvas-plugin-api-patterns)
   1. [Always check connection first](#1-always-check-connection-first)
   2. [Core API reference](#2-core-api-reference)
   3. [Critical API gotchas](#3-critical-api-gotchas)
   4. [Page management API](#4-page-management-api)
   5. [Library creation API](#5-library-creation-api)
   6. [Token API](#6-token-api)
   7. [Font and typography constraints](#7-font-and-typography-constraints)
   8. [Visual effects](#8-visual-effects-fills-gradients-blur-and-glassmorphism)
   9. [Storage global](#9-storage-global-cross-call-state)
   10. [Idempotency helpers](#10-idempotency-helpers)
   11. [Design system discovery](#11-design-system-discovery)
   12. [Board positioning](#12-board-positioning)
   13. [CSS export](#13-css-export)
   14. [Interactions, flows, and animations](#14-interactions-flows-and-animations)
   15. [Validation patterns](#15-validation-patterns)
   16. [Platform layout templates](#16-platform-layout-templates)
   17. [Default design tokens](#17-default-design-tokens-fallback)
   18. [Component checklists](#18-component-checklists)
10. [Troubleshooting](#troubleshooting)

## Operating model

- `canvas init` opens an isolated browser session named `canvas`.
- The browser tab must stay open on the design file for plugin-backed MCP tools to work.
- `canvas file switch` navigates that browser tab to the target Canvas file/page.
- `canvas mcp <tool>` calls the deployed MCP server, which routes tool calls to the plugin in the open browser tab.
- `canvas project create` and `canvas file create` call Hivy runtime endpoints for the current agent and org.

## Core rules

- Start Canvas work with `canvas doctor`, then `canvas init`.
- Before modifying a design with MCP, ensure a Canvas file is open in the `canvas` browser session.
- Before choosing or changing colors, typography, logo usage, voice, or any brand-sensitive visual direction, run `canvas brands list` and inspect the relevant org brand with `canvas brands view <brand-id>`. If the org has no brands and a palette is needed, choose one from the brief or product context, persist it with `canvas brands create`, and tell the user what you saved.
- Call `canvas mcp high_level_overview --json '{}'` once before using `execute_code`.
- Follow the safety-first workflow: READ, PLAN, WRITE, VERIFY. Do not skip steps.
- Use `canvas file current` before destructive or large edits to confirm the active file/page.
- Prefer `execute_code` for structured design edits and `export_shape` for visual verification.
- Return or store shape ids from `execute_code`; use those ids for later edits and exports.
- Keep the open browser tab focused on the file the agent is editing. If the user asks to switch tasks/files, call `canvas file switch`.
- For visual QA, always export the page or target board with `export_shape`, save the MCP response in `/tmp`, decode it to a `/tmp/*.png` or `/tmp/*.svg` file, then call `read_file` on that image file to inspect the rendered content before deciding the design is correct.
- For durable exported assets, use the Drive skill after exporting to a local file.

## Safety-first workflow

This workflow adapts the best process rules from `ar27111994/penpot-mcp`'s MIT-licensed `SKILL.md` for Hivy Canvas. Always work in this order:

```text
1. READ   -> Inspect, list, analyze. Never skip.
2. PLAN   -> Describe intended changes before applying.
3. WRITE  -> Use small atomic `execute_code` batches.
4. VERIFY -> Structural read after each write, then exported-image inspection with `read_file`.
```

### READ

Before changing a file:

- Run `canvas file current` to confirm the active file and page.
- Run `canvas mcp high_level_overview --json '{}'` once per task.
- Run `canvas brands list` before brand-sensitive design work. If a default brand exists, inspect it with `canvas brands view <brand-id>` and use its `logos`, `colors`, `typography`, and `voice` as the source of truth. If there are multiple brands and no default, use `request_user_input` to choose one. If no brand exists and the design needs a palette, choose a defensible starting palette from the brief, audience, product category, or existing file context, create a brand with `canvas brands create`, and tell the user what was created. If context is too thin to choose responsibly, use `request_user_input` for one focused direction question.
- Use `execute_code` to list the relevant pages, boards, components, colors, typographies, token sets, and existing interactions.
- Prefer structured JSON returns over prose when reading file state.
- Store large discovered structures in `storage` only when they are needed across calls, and always read them with a fallback because `storage` can reset.

Starter reads:

```bash
canvas mcp execute_code --json '{"code":"return {page: penpot.currentPage?.name, pages: penpotUtils.getPages(), structure: penpotUtils.shapeStructure(penpot.root, 2)};"}'
```

```bash
canvas mcp execute_code --json '{"code":"return {components: penpot.library.local.components.map(c => c.name), colors: penpot.library.local.colors.map(c => ({name: c.name, color: c.color})), typographies: penpot.library.local.typographies.map(t => ({name: t.name, fontFamilies: t.fontFamilies, fontSize: t.fontSize, fontWeight: t.fontWeight})), tokenSets: penpot.library.local.tokens.sets.map(s => s.name)};"}'
```

### PLAN

Before writing:

- Describe the exact target file/page, boards, shapes, tokens, components, or interactions you will change.
- State the batch plan, including the maximum number of shape operations per `execute_code` call.
- State the verification plan: structural read after each batch plus `export_shape` -> `/tmp` decode -> `read_file` for visual QA.
- For destructive, broad, or ambiguous changes, ask for approval before writing.
- Do not invent colors, tokens, components, copy, font weights, or product decisions when the file, user brief, or selected org brand does not provide enough context. When no org brand exists, a self-selected palette is allowed only if it is persisted with `canvas brands create`; otherwise ask a focused question or mark missing values as TODO.

### WRITE

When writing:

- Use `execute_code` for one logical unit at a time.
- Keep write calls small: about 5-10 shape operations per call.
- Never switch page and write in the same `execute_code` call. Use one call to `penpot.openPage(page)`, then a later call to read/write the new current page.
- Use idempotent helpers such as `ensurePage`, `ensureBoard`, `ensureColor`, `ensureTypography`, `ensureSet`, and `addToken`.
- Return ids, names, counts, and changed properties from every write call.
- Avoid logging unless it materially helps debugging. Never log the same object you return.

### VERIFY

After every write batch:

- Run a structural read with `execute_code` to confirm the expected page, boards, layer names, dimensions, token counts, component counts, interactions, and changed properties.
- Verify no accidental duplicate boards/components/styles were created.
- For visual work, run the mandatory image inspection loop:
  1. `canvas mcp export_shape --json ... > /tmp/<name>.json`
  2. Decode the image response to `/tmp/<name>.png` or `/tmp/<name>.svg`
  3. `read_file /tmp/<name>.png`
  4. Inspect the rendered content, then iterate if needed
- Do not claim visual correctness until the decoded export has been inspected with `read_file`.
- Treat `export_shape` as visual evidence only. If export fails, verify structurally and report that visual export could not be completed.

## Prompt and design quality guardrails

Use these guardrails for substantial Canvas work, especially design systems, prototyping, and design-to-code tasks.

### Agent role

Act as a Senior Product Designer and Design Systems Engineer who understands Canvas Plugin API constraints, WCAG accessibility, tokens, components, prototyping, and design-to-code handoff. Work from file evidence and the user brief. Do not make product decisions without input.

### Canvas ruleset

```text
SOURCE: Hivy Canvas MCP only
NO_GUESSING: true
IF_MISSING: mark TODO or ask one focused question
OUTPUT: deterministic, stable ordering
PREFER: structured data over prose for reads and audits
BATCH_LIMIT: 5-10 shape operations per execute_code call
PAGE_SWITCH: separate call from writes
IDEMPOTENCY: always check before creating
STORAGE: use storage for cross-call state only when useful; always provide fallback
STRUCTURAL_VERIFY: required after each write batch
VISUAL_VERIFY: export_shape -> /tmp decode -> read_file
```

### Negative constraints

- Do not override colors, typography, logo usage, or brand voice from an existing org brand. Always check `canvas brands list` before deciding these values. If the brand list is empty, any self-selected palette must be saved with `canvas brands create` instead of remaining an untracked one-off file choice.
- Do not use font weights unless they are confirmed available for that font family.
- Do not switch page and write in the same `execute_code` call.
- Do not create duplicate colors, typographies, token sets, components, or boards. Always check first.
- Do not use `export_shape` as the only verification. Use structural API checks, then exported-image `read_file` inspection for visual QA.
- Do not use appearance-only layer names such as `rectangle-23` or `blue-box` for new work. Use functional names.

### Design file quality rules

- Choose a clear page strategy: domain-based pages such as `Foundations`, `Mobile`, `Desktop`, or atomic-level pages such as `Tokens`, `Primitives`, `Components`, `Patterns`.
- Place related boards left-to-right in flow order. Use larger gaps for separate journeys.
- Name flow entry boards `/flows/[journey]-start`; name overlay boards `overlay/[name]`.
- Name layers by function, such as `background`, `icon-close`, `label-primary`, `button-submit`, and `card-plan`.
- Use component names with path structure, such as `button/primary/default`, `form/input/text/focus`, and `nav/tab/active`.
- Use an 8px spacing base unless the existing file has a different explicit system.
- Use Flex/Grid layout instead of invisible rectangles for spacing.
- Maintain WCAG AA contrast for text and minimum touch targets: 44px for iOS and 48dp for Android.
- Keep handoff developer-readable: semantic layer names, tokens/components as the source of truth, no duplicate styles, and clear open questions.

## Workflow references

Load these bundled references only when the task needs the workflow. They come from `ar27111994/penpot-mcp`'s MIT-licensed reference set and use "Penpot MCP" terminology; translate tool calls to Hivy's `canvas mcp <tool> --json ...` wrapper.

- [references/prototyping-workflows.md](references/prototyping-workflows.md): Use for flow setup, screen naming, interactions, overlays, drawers, modal wiring, animation choices, and prototype audits.
- [references/design-system-workflows.md](references/design-system-workflows.md): Use for design-system creation, token architecture, token migration, component library management, palette updates, and cross-file consistency audits.
- [references/design-to-code-workflows.md](references/design-to-code-workflows.md): Use for screen-to-code handoff, React component extraction, token export, component-code mapping, layout extraction, asset export, and design-code drift checks.

## CLI commands

### `canvas --help`

Use when you need the exact built-in command list.

```bash
canvas --help
```

Expected output:

```text
canvas controls a Hivy-managed Canvas session.

Usage:
  canvas doctor
  canvas init
  canvas project list
  canvas project create --name "Project"
  canvas file list
  canvas file create --name "File" --project-id <canvas-project-id>
  canvas file switch <canvas-file-id> [--page-id <page-id>]
  canvas file current
  canvas brands list
  canvas brands view <brand-id>
  canvas brands create --name "Brand" [--json '{"colors":{...}}']
  canvas brands update <brand-id> --json '{"description":"..."}'
  canvas mcp <tool> --json '{"key":"value"}'
```

### `canvas --version`

Use when debugging whether the sandbox image includes the expected CLI build.

```bash
canvas --version
```

Example output:

```text
canvas v3.7.0-30-g636a0c9ef (636a0c9ef)
```

### `canvas doctor`

Use first. It validates that Canvas is ready in the sandbox.

```bash
canvas doctor
```

Success:

```text
canvas runtime ok
```

Failure example:

```text
canvas: canvas runtime not ready: missing browser
```

### `canvas init`

Use once at the start of a Canvas task. It opens the isolated browser session, logs in through `/api/hivy/session`, and lands on the file encoded in the JWT when a file is present.

```bash
canvas init
```

Example output:

```text
✓ Smoke File 20260621103236 - Canvas
  https://canvas.usehivy.com/#/workspace?team-id=...&file-id=...
```

### `canvas project list`

Use when you need to find an existing Canvas project id.

```bash
canvas project list
```

Example output:

```json
{
  "projects": [
    {
      "project_id": "9dde6c38-4045-40f0-90a1-67f1ee0a3888",
      "name": "Marketing Site Redesign"
    }
  ]
}
```

### `canvas project create --name "..."`

Use when the task needs a new Canvas project. This calls Hivy's internal runtime endpoint for the current agent and org.

```bash
canvas project create --name "Marketing Site Redesign"
```

Example output:

```json
{
  "project_id": "9dde6c38-4045-40f0-90a1-67f1ee0a3888",
  "team_id": "449e5360-172e-4daa-8773-a75a55ec425d",
  "name": "Marketing Site Redesign"
}
```

Use `project_id` for later Canvas CLI calls and browser navigation.

### `canvas file list`

Use when you need to find an existing Canvas file id or workspace URL.

```bash
canvas file list
```

Example output:

```json
{
  "files": [
    {
      "file_id": "76ac2286-c18c-4683-bf03-7af5399c8297",
      "project_id": "9dde6c38-4045-40f0-90a1-67f1ee0a3888",
      "page_id": "275b6902-d932-8159-8008-35a5b2619ab0",
      "name": "Homepage Concept",
      "project_name": "Marketing Site Redesign",
      "workspace_url": "https://canvas.usehivy.com/#/workspace?team-id=...&file-id=...&page-id=..."
    }
  ]
}
```

### `canvas file create --name "..." --project-id <id>`

Use when the task needs a new design file in an existing Canvas project. Pass the `project_id` returned by `canvas project create`.

```bash
canvas file create --name "Homepage Concept" --project-id 9dde6c38-4045-40f0-90a1-67f1ee0a3888
```

Example output:

```json
{
  "file_id": "76ac2286-c18c-4683-bf03-7af5399c8297",
  "project_id": "9dde6c38-4045-40f0-90a1-67f1ee0a3888",
  "page_id": "275b6902-d932-8159-8008-35a5b2619ab0",
  "team_id": "449e5360-172e-4daa-8773-a75a55ec425d",
  "name": "Homepage Concept",
  "workspace_url": "https://canvas.usehivy.com/#/workspace?team-id=...&file-id=...&page-id=..."
}
```

For agent work, use `canvas file switch` with `file_id` and `page_id`.

### `canvas file switch <file-id> [--page-id <page-id>]`

Use whenever the agent needs to edit a different file or page, or immediately after file creation to ensure the browser tab owns MCP for that file.

```bash
canvas file switch 76ac2286-c18c-4683-bf03-7af5399c8297 --page-id 275b6902-d932-8159-8008-35a5b2619ab0
```

Example output:

```text
✓ Homepage Concept - Canvas
  https://canvas.usehivy.com/#/workspace?file-id=76ac2286-c18c-4683-bf03-7af5399c8297&team-id=449e5360-172e-4daa-8773-a75a55ec425d&page-id=275b6902-d932-8159-8008-35a5b2619ab0
```

If the file was created through `canvas file create`, pass the returned `file_id`.

### `canvas brands list`

Use before any brand-sensitive design choice. This calls Hivy's runtime brand endpoint for the current agent org and returns active org brands.

```bash
canvas brands list
```

Example output:

```json
{
  "data": [
    {
      "id": "4f87ff34-9bf0-4273-8614-c00f8d973937",
      "name": "Hivy",
      "slug": "hivy",
      "is_default": true,
      "colors": {"tokens": [{"id": "brand-blue", "value": "#2463eb"}]},
      "typography": {},
      "voice": {},
      "logos": {}
    }
  ],
  "has_more": false
}
```

Use the `is_default` brand when present. If multiple brands exist and no default is present, use `request_user_input` to choose the brand before applying colors, typography, logo usage, or voice. If the list is empty and the work needs a palette, choose a defensible starter palette from the brief or file context, create it with `canvas brands create`, and mention the new brand in the user-facing summary.

### `canvas brands view <brand-id>`

Use after `canvas brands list` to inspect the selected brand in full before applying its values.

```bash
canvas brands view 4f87ff34-9bf0-4273-8614-c00f8d973937
```

Example output:

```json
{
  "brand": {
    "id": "4f87ff34-9bf0-4273-8614-c00f8d973937",
    "name": "Hivy",
    "is_default": true,
    "logos": {},
    "colors": {},
    "typography": {},
    "voice": {},
    "source": {"version": 1, "origin": "manual"}
  }
}
```

When a brand exists, treat its `logos`, `colors`, `typography`, and `voice` as the source of truth. Do not randomly choose a palette or type style. If required values are missing from the selected brand, ask with `request_user_input` or mark the specific missing values as TODO in Canvas notes.

### `canvas brands create --name "..."`

Use when the org has no brands and you choose a starter palette for the design, or when the user asks to create a brand record. Name the brand from the product, project, or organization when available; if this is the first brand, mark it as default. Include chosen color tokens when you selected a palette, and include typography or voice values only when you selected them intentionally. Tell the user that you created the brand and summarize the saved palette.

```bash
canvas brands create --name "Hivy" --default --json '{"colors":{"tokens":[{"id":"brand-blue","value":"#2463eb"}]}}'
```

### `canvas brands update <brand-id> --json "..."`

Use only when the user asks to update the org brand or explicitly approves updating it. Do not silently overwrite brand values while working on a design file.

```bash
canvas brands update 4f87ff34-9bf0-4273-8614-c00f8d973937 --json '{"description":"Updated brand notes"}'
```

### `canvas file current`

Use before MCP edits and before reporting a file link. It prints the CLI's tracked state plus the currently open browser URL.

```bash
canvas file current
```

Example output:

```json
{
  "file_id": "76ac2286-c18c-4683-bf03-7af5399c8297",
  "page_id": "275b6902-d932-8159-8008-35a5b2619ab0",
  "workspace_url": "https://canvas.usehivy.com/#/workspace?file-id=76ac2286-c18c-4683-bf03-7af5399c8297&team-id=449e5360-172e-4daa-8773-a75a55ec425d&page-id=275b6902-d932-8159-8008-35a5b2619ab0",
  "updated_at": "2026-06-21T09:45:38Z"
}
```

### `canvas mcp <tool> --json '{...}'`

Use for all MCP tool calls. Always pass valid JSON to `--json`. The CLI initializes a Streamable HTTP MCP session for each command and calls the named tool.

```bash
canvas mcp high_level_overview --json '{}'
```

Typical response envelope:

```json
{
  "id": "1782035022339467161",
  "jsonrpc": "2.0",
  "result": {
    "content": [
      {
        "type": "text",
        "text": "..."
      }
    ]
  }
}
```

Error when the browser file is not connected to MCP:

```text
canvas: POST https://canvas.usehivy.com/mcp/stream?userToken=<redacted> failed with 500: No plugin instance connected
```

Fix by running:

```bash
canvas init
canvas file switch <canvas-file-id> --page-id <page-id>
canvas mcp high_level_overview --json '{}'
```

## Standard Canvas workflows

### New design file

```bash
canvas doctor
canvas init
project_json="$(canvas project create --name "New Product Flow")"
project_id="$(printf '%s' "$project_json" | jq -r .project_id)"
file_json="$(canvas file create --name "Checkout Flow" --project-id "$project_id")"
file_id="$(printf '%s' "$file_json" | jq -r .file_id)"
page_id="$(printf '%s' "$file_json" | jq -r '.page_id // empty')"
canvas file switch "$file_id" --page-id "$page_id"
canvas mcp high_level_overview --json '{}'
```

### Open an existing file and inspect it

```bash
canvas doctor
canvas init
canvas file switch <canvas-file-id> --page-id <page-id>
canvas file current
canvas mcp high_level_overview --json '{}'
canvas mcp execute_code --json '{"code":"return penpotUtils.shapeStructure(penpot.root, 2);"}'
```

### Create a simple board, rectangle, and text

```bash
canvas mcp execute_code --json "$(jq -nc --arg code '
const board = penpot.createBoard();
board.name = "Landing Page Hero";
board.x = 120;
board.y = 120;
board.resize(1440, 900);
board.fills = [{ fillColor: "#FFFFFF", fillOpacity: 1 }];

const card = penpot.createRectangle();
card.name = "Hero CTA panel";
card.x = 220;
card.y = 220;
card.resize(520, 280);
card.borderRadius = 16;
card.fills = [{ fillColor: "#14B8A6", fillOpacity: 1 }];
board.appendChild(card);

const title = penpot.createText("Ship design work live");
if (title) {
  title.name = "Hero headline";
  title.x = 260;
  title.y = 260;
  title.fontSize = "48";
  title.fills = [{ fillColor: "#111827", fillOpacity: 1 }];
  board.appendChild(title);
}

return { boardId: board.id, cardId: card.id, titleId: title ? title.id : null };
' '{code:$code}')"
```

### Export current page as PNG and save locally

```bash
canvas mcp export_shape --json '{"shapeId":"page","format":"png","mode":"shape"}' > /tmp/page-export.json
node -e '
const fs = require("fs");
const response = JSON.parse(fs.readFileSync("/tmp/page-export.json", "utf8"));
const image = response.result.content.find((entry) => entry.type === "image");
fs.writeFileSync("/tmp/page-export.png", Buffer.from(image.data, "base64"));
'
```

Then inspect the rendered export:

```text
read_file /tmp/page-export.png
```

## Available MCP tools

These are the tools registered by the multi-user Canvas MCP server that Hivy Canvas uses by default:

- `high_level_overview`
- `penpot_api_info`
- `execute_code`
- `export_shape`

### `high_level_overview`

Source tool description:

```text
Returns basic high-level instructions on the usage of Canvas-related tools and the Canvas Plugin API. If you have already read the high-level overview, do not call this tool again.
```

Input schema:

```json
{}
```

Use once per design task before `execute_code`. It returns Canvas Plugin API guidance and utility conventions such as `penpotUtils.shapeStructure`, `penpotUtils.findShapeById`, and board/layout best practices.

Samples:

```bash
# 1. Read the overview at the start of a design task.
canvas mcp high_level_overview --json '{}'
```

```bash
# 2. Save the overview locally if you need to grep it during a long task.
canvas mcp high_level_overview --json '{}' > /tmp/canvas-overview.json
jq -r '.result.content[0].text' /tmp/canvas-overview.json | sed -n '1,80p'
```

```bash
# 3. Confirm the tool responds before attempting plugin-backed mutations.
canvas mcp high_level_overview --json '{}' \
  | jq -r '.result.content[0].text | split("\n")[0]'
```

### `penpot_api_info`

Source tool description:

```text
Retrieves Canvas Plugin API documentation for types and their members. Read the high-level overview first.
```

Input schema:

```json
{
  "type": "string, required, API type name",
  "member": "string, optional, member name"
}
```

Use to inspect exact Canvas Plugin API members before writing `execute_code`. Prefer this over guessing property names.

Samples:

```bash
# 1. Inspect the root API type.
canvas mcp penpot_api_info --json '{"type":"Penpot"}'
```

```bash
# 2. Inspect board capabilities before creating containers.
canvas mcp penpot_api_info --json '{"type":"Board"}'
```

```bash
# 3. Inspect rectangle fill/stroke APIs.
canvas mcp penpot_api_info --json '{"type":"Rectangle","member":"fills"}'
```

```bash
# 4. Inspect text sizing before editing labels.
canvas mcp penpot_api_info --json '{"type":"Text","member":"fontSize"}'
```

```bash
# 5. Inspect a specific method before layout work.
canvas mcp penpot_api_info --json '{"type":"Board","member":"appendChild"}'
```

### `execute_code`

Source tool description:

```text
Executes JavaScript code in the Canvas plugin context.
IMPORTANT: Before using this tool, make sure you have read the high-level overview and know which Canvas Plugin API functionality is necessary and how to use it.
You have access to the exact plugin globals `penpot`, `penpotUtils`, and `storage`.
`storage` is an object in which arbitrary data can be stored, simply by adding a new attribute; stored attributes can be referenced in future calls to this tool, so any intermediate results that could come in handy later should be stored in `storage` instead of just a fleeting variable; you can also store functions and thus build up a library).
Think of the code being executed as the body of a function: The tool call returns whatever you return in the applicable `return` statement, if any. You can return arbitrary JS objects; no need to apply JSON.stringify.
If an exception occurs, the exception's message will be returned to you.
Any output that you generate via the `console` object will be returned to you separately; so you may use it to track what your code is doing, but you should *only* do so only if there is an ACTUAL NEED for this! VERY IMPORTANT: Don't use logging prematurely! NEVER log the data you are returning, as you will otherwise receive it twice!
VERY IMPORTANT: In general, try a simple approach first, and only if it fails, try more complex code that involves handling different cases (in particular error cases) and that applies logging.
```

Input schema:

```json
{
  "code": "string, required, JavaScript code to execute in the plugin context"
}
```

Use for reading and mutating the active Canvas file. The code runs as a function body; use `return ...` for data you need back.

Samples:

```bash
# 1. Inspect the current page and shallow shape tree.
canvas mcp execute_code --json '{"code":"return {page: penpot.currentPage?.name, structure: penpotUtils.shapeStructure(penpot.root, 2)};"}'
```

```bash
# 2. Create a board with text and return ids for follow-up edits.
canvas mcp execute_code --json "$(jq -nc --arg code '
const board = penpot.createBoard();
board.name = "Feature Comparison";
board.x = 100;
board.y = 100;
board.resize(960, 540);
board.fills = [{ fillColor: "#FFFFFF", fillOpacity: 1 }];

const heading = penpot.createText("Compare plans");
if (heading) {
  heading.name = "Heading";
  heading.x = 140;
  heading.y = 140;
  heading.fontSize = "40";
  heading.fills = [{ fillColor: "#111827", fillOpacity: 1 }];
  board.appendChild(heading);
}

storage.lastBoardId = board.id;
return {boardId: board.id, headingId: heading ? heading.id : null};
' '{code:$code}')"
```

```bash
# 3. Find a shape by name and update its fill.
canvas mcp execute_code --json "$(jq -nc --arg code '
const shape = penpotUtils.findShape((item) => item.name === "Hero CTA panel");
if (!shape) throw new Error("Shape not found");
shape.fills = [{ fillColor: "#2563EB", fillOpacity: 1 }];
return {id: shape.id, name: shape.name};
' '{code:$code}')"
```

```bash
# 4. Create a card grid using boards and rectangles.
canvas mcp execute_code --json "$(jq -nc --arg code '
const board = penpot.createBoard();
board.name = "Three Feature Cards";
board.x = 80;
board.y = 80;
board.resize(1200, 420);
board.fills = [{ fillColor: "#F9FAFB", fillOpacity: 1 }];

const cards = [];
for (let i = 0; i < 3; i++) {
  const card = penpot.createRectangle();
  card.name = `Feature Card ${i + 1}`;
  card.x = 130 + i * 360;
  card.y = 150;
  card.resize(300, 180);
  card.borderRadius = 12;
  card.fills = [{ fillColor: "#FFFFFF", fillOpacity: 1 }];
  card.strokes = [{ strokeColor: "#E5E7EB", strokeStyle: "solid", strokeWidth: 1, strokeAlignment: "center" }];
  board.appendChild(card);
  cards.push(card.id);
}
return {boardId: board.id, cardIds: cards};
' '{code:$code}')"
```

```bash
# 5. Store helpers in MCP storage for reuse in later execute_code calls.
canvas mcp execute_code --json "$(jq -nc --arg code '
storage.findByName = (name) => penpotUtils.findShape((shape) => shape.name === name);
return {stored: ["findByName"]};
' '{code:$code}')"
```

### `export_shape`

Source tool description:

```text
Exports a shape or shape image fill from the active Canvas design to a PNG or SVG image.
```

Capture image data from the MCP JSON response and write it to a sandbox file if needed.

Input schema:

```json
{
  "shapeId": "string, required. Use a shape id, 'selection', or 'page'",
  "format": "png or svg, optional, default png",
  "mode": "shape or fill, optional, default shape"
}
```

Use for visual verification and PNG/SVG exports of a specific board, selection, image fill, or the current page.

Mandatory visual QA workflow:

1. Export the page or target board with `export_shape` and save the JSON response to `/tmp/<descriptive-name>.json`.
2. Decode the MCP image response into `/tmp/<descriptive-name>.png` or `/tmp/<descriptive-name>.svg`.
3. Call `read_file` on that decoded image path and inspect the actual rendered content.
4. If anything is visually wrong, edit with `execute_code`, then repeat export, decode, and `read_file`.
5. Do not report that the design is visually correct until a decoded export has been inspected with `read_file`.

Samples:

```bash
# 1. Export the whole current page as PNG.
canvas mcp export_shape --json '{"shapeId":"page","format":"png","mode":"shape"}' > /tmp/page-export.json
```

```bash
# 2. Export a known board/shape id as PNG.
canvas mcp export_shape --json '{"shapeId":"3966c9f4-d99a-804d-8008-35a83b52cffa","format":"png","mode":"shape"}' > /tmp/board-export.json
```

```bash
# 3. Export the current selection as SVG.
canvas mcp export_shape --json '{"shapeId":"selection","format":"svg","mode":"shape"}' \
  | jq -r '.result.content[0].text // .result.content[0].data'
```

```bash
# 4. Decode a PNG response to a local file.
canvas mcp export_shape --json '{"shapeId":"page","format":"png","mode":"shape"}' > /tmp/export.json
node -e '
const fs = require("fs");
const response = JSON.parse(fs.readFileSync("/tmp/export.json", "utf8"));
const image = response.result.content.find((entry) => entry.type === "image");
fs.writeFileSync("/tmp/export.png", Buffer.from(image.data, "base64"));
'
```

After decoding, inspect the rendered image with the file reader:

```text
read_file /tmp/export.png
```

```bash
# 5. Export only the raw image fill for a shape that has an image fill.
canvas mcp export_shape --json '{"shapeId":"<image-filled-shape-id>","format":"png","mode":"fill"}' > /tmp/fill-export.json
```

## Canvas Plugin API patterns

This section adapts the Penpot MCP `penpot-api-patterns.md` reference from `ar27111994/penpot-mcp` for Hivy Canvas. The upstream content is MIT licensed and describes direct MCP tool calls such as `mcp__penpot__execute_code`; in Hivy Canvas, use the CLI wrapper instead:

```bash
canvas mcp high_level_overview --json '{}'
canvas mcp penpot_api_info --json '{"type":"Penpot"}'
canvas mcp execute_code --json '{"code":"return penpotUtils.shapeStructure(penpot.root);"}'
canvas mcp export_shape --json '{"shapeId":"page","format":"png","mode":"shape"}'
```

For multi-line JavaScript, wrap the code with `jq` so shell quoting does not corrupt the payload:

```bash
canvas mcp execute_code --json "$(jq -nc --arg code '
const board = penpot.createBoard();
board.name = "Example";
board.resize(400, 300);
return { id: board.id, name: board.name };
' '{code:$code}')"
```

### 1. Always check connection first

Use one of these commands before relying on plugin-backed operations:

```bash
canvas mcp high_level_overview --json '{}'
canvas mcp penpot_api_info --json '{"type":"Penpot"}'
```

- Succeeds: server and browser plugin bridge are connected. Proceed to the task.
- Fails: ask, "The Canvas MCP server doesn't appear connected. Is the Canvas browser session open? I can help troubleshoot or guide setup."
- Only walk through setup if the user confirms Canvas is not initialized.

### 2. Core API reference

All JavaScript snippets in this section run inside Hivy Canvas with `canvas mcp execute_code --json '{"code":"..."}'`.

#### Read operations

```javascript
// Page structure (current page)
penpotUtils.shapeStructure(penpot.root);
penpotUtils.findShapes((s) => s.type === "text", penpot.root); // predicate-based
penpotUtils.findShapes((s) => s.name.startsWith("icon"), penpot.root);
penpotUtils.findShapes(() => true, penpot.root); // all shapes on current page
penpotUtils.findShape((s) => s.name === "target"); // singular, returns first match

// Find by ID
penpotUtils.findShapeById("uuid-string");

// Library reads
penpot.library.local.components;
penpot.library.local.colors;
penpot.library.local.typographies;
penpot.library.local.tokens; // TokenCatalog
penpot.library.local.tokens.sets; // TokenSet[]
penpot.library.local.tokens.themes; // TokenTheme[]

// File context
penpot.currentPage; // Page | null
penpot.currentFile; // File | null
penpot.root; // root Shape of current page

// Interactions on a shape
const board = penpotUtils.findShape((s) => s.name === "Home");
board.interactions; // Interaction[]
```

#### Create shape operations

```javascript
const board = penpot.createBoard();
const rect = penpot.createRectangle();
const text = penpot.createText("Hello"); // returns Text | null
if (!text) return { error: "createText returned null" };
const ellipse = penpot.createEllipse();

// Layout on a container
board.addFlexLayout();
board.addGridLayout();

// Z-ordering (NOT appendChild for ordering)
parent.insertChild(0, shape); // index 0 = bottom of stack
parent.appendChild(shape); // appends without z-order control
```

#### Modify shape operations

```javascript
// SIZE: always resize(), never direct assignment
shape.resize(400, 300);
shape.width = 400; // wrong: read-only

// POSITION
// Top-level shapes (boards on page root): direct assignment works
board.x = 100;
board.y = 200;
// Parented shapes: use utility
penpotUtils.setParentXY(shape, 100, 200);

// Text grow behavior: reset after every resize
text.resize(200, 0);
text.growType = "auto-height"; // "auto-width" | "auto-height" | "fixed"

// Fills (solid color)
shape.fills = [{ fillColor: "#3451B2", fillOpacity: 1 }];

// Typography on a text layer
text.fontFamily = "Inter";
text.fontSize = String(16); // always string for safety
text.fontWeight = "400"; // always string
text.lineHeight = String(1.5); // always string
text.letterSpacing = "0";

// Flex layout
const layout = board.addFlexLayout();
layout.dir = "row"; // "row" | "column" | "row-reverse" | "column-reverse"
layout.gap = 16;
layout.padding = { top: 16, right: 16, bottom: 16, left: 16 };
layout.justifyContent = "center"; // "start" | "center" | "end" | "space-between"
layout.alignItems = "center"; // "start" | "center" | "end" | "stretch"

// Visual effects
shape.opacity = 0.9;
shape.blendMode = "multiply"; // "normal" | "multiply" | "screen" | "overlay" | ...
shape.borderRadius = 20;
shape.hidden = false;
```

#### Clone and remove

```javascript
const clone = shape.clone(); // clones to same parent
shape.remove(); // removes shape from current page
interaction.remove(); // removes specific interaction
```

#### Plugin data (persistent metadata per shape/page/library)

```javascript
// Shape-level
shape.setPluginData("my-key", "my-value");
shape.getPluginData("my-key");
shape.getPluginDataKeys();

// Library-level (file-wide metadata)
penpot.library.local.setPluginData("spec", JSON.stringify(payload));
penpot.library.local.getPluginData("spec");

// Page-level
page.setPluginData("role", "foundations");

// Shared across plugins (namespace required)
shape.setSharedPluginData("design-system", "token", "color.primary.500");
```

#### Community plugin boundaries

The Canvas MCP server runs through the Canvas plugin and exposes the Penpot Plugin API. Do not assume it can list installed community plugins, install plugins, launch another plugin, or call another plugin's private UI/API. The documented plugin API supports file content, libraries, comments, user data, and plugin/shared plugin data according to the active plugin's manifest permissions; it is not a general installed-plugin automation bus.

Safe coordination pattern:

1. Use MCP to inspect the file first.
2. If a task appears to need a community plugin, check only for file-visible evidence such as generated layers, library assets, comments, or namespaced shared plugin data.
3. If the user provides an installed-plugin inventory, treat it as user-provided context and choose a plugin only when the task clearly maps to that plugin's stated capability.
4. If the plugin must run, ask the user to confirm the exact installed plugin and whether they want it used.
5. Ask the user to run that plugin manually when it requires its own UI or permissions, then re-inspect the file with MCP.
6. Never browse, search, or install plugins from an agent loop unless the user explicitly requested plugin discovery or installation.

### 3. Critical API gotchas

| Property / Behaviour | Status | Correct Approach |
| --- | --- | --- |
| `shape.width` / `shape.height` | Read-only | `shape.resize(w, h)` |
| `shape.parentX` / `shape.parentY` | Read-only | `penpotUtils.setParentXY(shape, x, y)` |
| `shape.x` / `shape.y` for parented shapes | Read-only | `penpotUtils.setParentXY(shape, x, y)` |
| `shape.x` / `shape.y` for root-level shapes | Works | Direct assignment OK for top-level boards |
| Z-ordering via `appendChild` | Ignores order | `insertChild(index, shape)` |
| `penpot.createText(...)` | Nullable | Check result before resize/style calls |
| Text clips after `resize()` | Reset required | Set `growType` after every `text.resize()` |
| Flex children order | Reversed | For column: last inserted = visually top |
| Page switch + write in same call | Writes to wrong page | Two calls: switch page, then write |
| Large batch writes | Silent timeout/partial risk | Max about 10 ops per call; verify after |
| `export_shape` HTTP error | Unreliable | Verify structurally; do not rely only on export |
| Library `fontSize` | Must be string | `"16"` not `16` for library typographies |
| Shadow `color` field | Color object | `{ color: "#hex", opacity: 0.15 }` not `{r,g,b,a}` |
| `LibraryColor.color` | Different from fill | `color.color = "#hex"` not `color.fillColor` |

#### Flex children reversal

```javascript
// Column: last inserted = visually top
container.insertChild(0, footer); // bottom
container.insertChild(1, content); // middle
container.insertChild(2, header); // top; counter-intuitive
```

#### Text resize and growType: always pair

```javascript
const label = penpot.createText("Button Label");
if (!label) return { error: "Text creation failed" };

label.resize(120, 0);
label.growType = "auto-height"; // must follow every resize
label.fontSize = "14"; // string
label.fontWeight = "500"; // string
```

#### Batch size discipline

```javascript
for (let i = 0; i < shapes.length; i += 5) {
  const batch = shapes.slice(i, i + 5);
  // process batch
  // return partial results so caller can verify before continuing
}
```

### 4. Page management API

#### Read pages

```javascript
// List all pages (returns {id, name}[]; lightweight)
const pages = penpotUtils.getPages(); // [{id, name}]

// Get Page object by name/id (full Page API)
const pageByName = penpotUtils.getPageByName("Mobile"); // Page | null
const pageById = penpotUtils.getPageById("uuid"); // Page | null

// Current page
const currentPage = penpot.currentPage; // Page | null
```

#### Page-scoped shape search (preferred over root search)

```javascript
// Criteria-based search on a specific page; cleaner than predicate on root
const page = penpotUtils.getPageByName("Mobile");
if (!page) return { error: "Page not found", pageName: "Mobile" };

const boards = page.findShapes({ type: "board" });
const named = page.findShapes({ name: "Header" });
const like = page.findShapes({ nameLike: "btn-" });
// Type options: "board" | "rectangle" | "ellipse" | "text" | "group" | "path" | "image" | "boolean" | "svg-raw"
```

#### Create and navigate pages

```javascript
// Create a new page
const newPage = penpot.createPage();
newPage.name = "Foundations";

// Navigate to a page: separate this call from writes that follow
penpot.openPage(page); // pass Page object
penpot.openPage(page.id); // or page id string
// After openPage, penpot.root and penpot.currentPage reflect the new page
```

#### Move shapes between pages

```javascript
// Access root of any page and append to it
const targetPage = penpotUtils.getPageByName("Mobile");
targetPage.root.appendChild(board); // moves board to that page's root
```

#### Idempotent ensurePage helper

```javascript
function ensurePage(name) {
  const existing = penpotUtils.getPageByName(name);
  if (existing) {
    penpot.openPage(existing);
    return penpot.currentPage;
  }
  const page = penpot.createPage();
  page.name = name;
  penpot.openPage(page);
  return page;
}
```

#### Page flows (prototype entry points) via API

```javascript
// Flows can be created via API (Page.createFlow)
const page = penpot.currentPage;
const entryBoard = penpotUtils.findShape(
  (s) => s.name === "/flows/onboarding-start",
);
const flow = page.createFlow("Onboarding", entryBoard);

// Remove a flow
page.removeFlow(flow);

// List existing flows
page.flows; // Flow[]
```

#### Ruler guides

```javascript
page.addRulerGuide("vertical", 320); // "horizontal" | "vertical"
page.addRulerGuide("horizontal", 64, board); // optional: board-scoped guide
page.removeRulerGuide(guide);
page.rulerGuides; // RulerGuide[]
```

### 5. Library creation API

#### Create library color

```javascript
function ensureColor(name, hex) {
  const existing = penpot.library.local.colors.find((c) => c.name === name);
  if (existing) return existing;
  const color = penpot.library.local.createColor();
  color.name = name; // supports path: "Brand/Primary"
  color.color = hex; // .color, not .fillColor
  return color;
}

// Access
color.name; // "Brand/Primary"
color.color; // "#RRGGBB"; the hex value passed in
color.path; // path segment before last "/"
```

#### Create library typography

```javascript
function ensureTypography(
  name,
  fontFamilies,
  fontWeight,
  fontSize,
  lineHeight,
  letterSpacing,
  textTransform,
) {
  const existing = penpot.library.local.typographies.find(
    (t) => t.name === name,
  );
  if (existing) return existing;
  const typo = penpot.library.local.createTypography();
  typo.name = name;
  typo.fontFamilies = fontFamilies; // fontFamilies, not fontFamily
  typo.fontWeight = fontWeight; // string: "700"
  typo.fontSize = fontSize; // string: "16"; must be string
  typo.lineHeight = lineHeight; // string: "1.5"
  typo.letterSpacing = letterSpacing; // string: "0.02"
  typo.fontStyle = "normal"; // "normal" | "italic" | null
  if (textTransform) typo.textTransform = textTransform; // "uppercase" | "capitalize" | "lowercase"
  return typo;
}

// Apply to a text shape
typo.applyToText(textShape); // apply typography style to whole text
typo.applyToTextRange(range); // apply to a text range
typo.setFont(font, variant); // set font + variant from Fonts API
```

#### Create library component

```javascript
function ensureComponent(name, shapes) {
  if (penpot.library.local.components.some((c) => c.name === name)) return null;
  const component = penpot.library.local.createComponent(shapes); // shapes: Shape[]
  component.name = name; // supports path: "category/component/variant"
  return component;
}

// Typical pattern: clone shape, position it off-canvas, then create component
const clone = sourceShape.clone();
clone.x = 3400;
clone.y = 0;
const component = penpot.library.local.createComponent([clone]);
component.name = "category/component/variant";
```

### 6. Token API

The full W3C DTCG-compatible token system is accessible via `penpot.library.local.tokens`.

#### TokenCatalog

```javascript
const catalog = penpot.library.local.tokens;
catalog.sets; // TokenSet[] in precedence order
catalog.themes; // TokenTheme[]
catalog.addSet({ name: "brand/base" }); // create TokenSet
catalog.addTheme({ group: "Theme", name: "Light" }); // create TokenTheme
catalog.getSetById(id); // TokenSet | undefined
catalog.getThemeById(id); // TokenTheme | undefined
```

#### TokenSet

```javascript
const set = catalog.addSet({ name: "brand/base" });
set.name; // string; supports / for grouping
set.active; // boolean
set.tokens; // Token[]
set.toggleActive(); // activate / deactivate

// Add token to set
const token = set.addToken({
  type: "color", // TokenType, see below
  name: "color.brand.primary", // dot-path
  value: "#RRGGBB", // TokenValueString; can reference: "{color.base.500}"
});

// TokenType values:
// "color" | "dimension" | "spacing" | "typography" | "shadow" | "opacity"
// "borderRadius" | "borderWidth" | "fontWeights" | "fontSizes" | "fontFamilies"
// "letterSpacing" | "textDecoration" | "textCase" | "number" | "sizing"

set.duplicate(); // clone set
set.remove(); // delete set
```

#### TokenTheme

```javascript
const theme = catalog.addTheme({ group: "Theme", name: "Dark" });
theme.group; // "Theme"
theme.name; // "Dark"
theme.active; // boolean
theme.activeSets; // TokenSet[]
theme.addSet(set); // add a TokenSet to this theme
theme.removeSet(set); // remove
theme.toggleActive(); // activate theme
theme.duplicate();
theme.remove();
```

#### Full token setup example (production pattern)

```javascript
function ensureSet(name) {
  return (
    penpot.library.local.tokens.sets.find((s) => s.name === name) ||
    penpot.library.local.tokens.addSet({ name })
  );
}
function addToken(set, type, name, value) {
  return (
    set.tokens.find((t) => t.name === name && t.type === type) ||
    set.addToken({ type, name, value: String(value) })
  );
}
function ensureTheme(group, name, sets) {
  const existing = penpot.library.local.tokens.themes.find(
    (t) => t.group === group && t.name === name,
  );
  const theme =
    existing || penpot.library.local.tokens.addTheme({ group, name });
  sets.forEach((set) => {
    if (!theme.activeSets.some((a) => a.name === set.name)) theme.addSet(set);
  });
  return theme;
}

const base = ensureSet("brand/base");
addToken(base, "color", "color.brand.primary", "#RRGGBB"); // replace with your brand color
addToken(base, "spacing", "spacing.md", "16");
addToken(base, "borderRadius", "radius.md", "8");
addToken(base, "borderRadius", "radius.lg", "16");
addToken(base, "opacity", "opacity.overlay", "0.8");

const light = ensureSet("theme/light");
addToken(light, "color", "color.bg.default", "#F5F5F5");
addToken(light, "color", "color.text.primary", "{color.neutral.900}"); // reference

const dark = ensureSet("theme/dark");
addToken(dark, "color", "color.bg.default", "#121212");

// Activate base + light by default
if (!base.active) base.toggleActive();
if (!light.active) light.toggleActive();

ensureTheme("Theme", "Light", [base, light]);
ensureTheme("Theme", "Dark", [base, dark]);
```

### 7. Font and typography constraints

#### Font weight must match installed variants

```javascript
// Always discover installed typographies before using a font family
const typos = penpot.library.local.typographies;
const interWeights = typos
  .filter((t) => t.fontFamilies === "Inter")
  .map((t) => t.fontWeight);
// Only use weights confirmed in interWeights array

// Library typographies use fontFamilies, not fontFamily
typo.fontFamilies = "Inter 28pt";
```

#### Library fontSize must be string

```javascript
typo.fontSize = "16"; // string required for library typographies
text.fontSize = "16"; // also safest for text layers
text.fontSize = 16; // also works on text layers, but string is safer
```

#### fontId stale after typography update

Known API limitation: updating `fontFamily`/`fontWeight` on a library typography does not update the internal `fontId` field. Rendered text layers use correct IDs. Do not attempt to patch `fontId` manually. Workaround: delete and recreate the typography style.

### 8. Visual effects: fills, gradients, blur and glassmorphism

#### Solid fill

```javascript
shape.fills = [{ fillColor: "#RRGGBB", fillOpacity: 1 }];
```

#### Linear gradient fill

```javascript
shape.fills = [
  {
    fillColorGradient: {
      type: "linear",
      startX: 0.5,
      startY: 0, // 0-1 normalized
      endX: 0.5,
      endY: 1,
      width: 1,
      stops: [
        { color: "#RRGGBB", opacity: 1, offset: 0 },
        { color: "#RRGGBB", opacity: 0, offset: 1 },
      ],
    },
  },
];
```

#### Radial gradient fill

```javascript
shape.fills = [
  {
    fillColorGradient: {
      type: "radial",
      startX: 0.5,
      startY: 0.5,
      endX: 1,
      endY: 0.5,
      width: 0.5,
      stops: [
        { color: "#FFFFFF", opacity: 0.2, offset: 0 },
        { color: "#FFFFFF", opacity: 0, offset: 1 },
      ],
    },
  },
];
```

#### Image fill from URL

`uploadMediaUrl(name, url)` fetches from the URL via the Canvas server. Only use URLs from trusted sources; never pass user-supplied or agent-generated URLs without validation.

```javascript
// uploadMediaUrl is async; must use await
const imageData = await penpot.uploadMediaUrl(
  "image-name",
  "https://example.com/image.jpg",
);
shape.fills = [{ fillOpacity: 1, fillImage: imageData }];
```

#### Blur (layer-blur only)

```javascript
shape.blurs = [{ type: "layer-blur", value: 20, hidden: false }];
// shape.blurs is an array
// type "layer-blur" is the only supported type
```

#### Shadow

```javascript
shape.shadows = [
  {
    style: "drop-shadow", // "drop-shadow" | "inner-shadow"
    offsetX: 0,
    offsetY: 8,
    blur: 32,
    spread: 0,
    color: { color: "#000000", opacity: 0.08 }, // .color + .opacity, not r/g/b/a
    hidden: false,
  },
];
```

#### Glassmorphism recipe (20px standard)

```javascript
// Semi-transparent surface + blur + subtle border + shadow
function glassPanel(
  parent,
  name,
  x,
  y,
  w,
  h,
  surfaceColor = "#FFFFFF",
  surfaceOpacity = 0.8,
  blurValue = 20,
  borderRadius = 16,
) {
  const panel = penpot.createRectangle();
  panel.name = name;
  panel.resize(w, h);
  panel.borderRadius = borderRadius;
  panel.fills = [{ fillColor: surfaceColor, fillOpacity: surfaceOpacity }];
  panel.blurs = [{ type: "layer-blur", value: blurValue, hidden: false }];
  panel.shadows = [
    {
      style: "drop-shadow",
      offsetX: 0,
      offsetY: 4,
      blur: 16,
      spread: 0,
      color: { color: "#000000", opacity: 0.08 },
      hidden: false, // adjust color/opacity to match your palette
    },
  ];
  parent.appendChild(panel);
  penpotUtils.setParentXY(panel, x, y);
  return panel;
}
```

#### Strokes

```javascript
shape.strokes = [
  {
    strokeColor: "#2e3434",
    strokeOpacity: 1,
    strokeStyle: "solid", // "solid" | "dashed" | "dotted" | "mixed" | "none"
    strokeWidth: 2,
    strokeAlignment: "center", // "center" | "inner" | "outer"
  },
];
```

### 9. storage Global: Cross-Call State

The `storage` object persists across all `execute_code` calls within a single MCP session. Use it to share large data structures (design tokens, color palettes, component lists) between calls without re-computing or re-passing them.

```javascript
// Call 1: store design system data
const DS = { colors: { primary: "#RRGGBB" }, typography: [] };
storage.designSystem = DS;
return { stored: true, colorCount: Object.keys(DS.colors).length };
```

```javascript
// Call 2+: retrieve from storage
const fallback = { colors: {}, typography: [] };
const DS = storage.designSystem || fallback; // fallback if session reset
const C = DS.colors;

// Pattern for processing queues
storage.shapesToProcess = allShapes.map((s) => s.id);
storage.processed = [];

// Later call:
const id = storage.shapesToProcess.shift();
const shape = penpotUtils.findShapeById(id);
storage.processed.push(id);
return {
  remaining: storage.shapesToProcess.length,
  done: storage.processed.length,
};
```

`storage` is session-scoped and resets when the MCP server restarts. Always use `|| fallback` when reading from storage.

### 10. Idempotency helpers

Always use idempotent helpers for design system construction. Calls will often be retried after partial failures.

```javascript
// Idempotent set creation
function ensureSet(name) {
  return (
    penpot.library.local.tokens.sets.find((s) => s.name === name) ||
    penpot.library.local.tokens.addSet({ name })
  );
}

// Idempotent token addition
function addToken(set, type, name, value) {
  return (
    set.tokens.find((t) => t.name === name && t.type === type) ||
    set.addToken({ type, name, value: String(value) })
  );
}

// Idempotent color creation
function ensureColor(name, hex) {
  return (
    penpot.library.local.colors.find((c) => c.name === name) ||
    (() => {
      const c = penpot.library.local.createColor();
      c.name = name;
      c.color = hex;
      return c;
    })()
  );
}

// Idempotent typography creation
function ensureTypography(
  name,
  fontFamilies,
  weight,
  size,
  lineHeight,
  letterSpacing,
) {
  return (
    penpot.library.local.typographies.find((t) => t.name === name) ||
    (() => {
      const t = penpot.library.local.createTypography();
      t.name = name;
      t.fontFamilies = fontFamilies;
      t.fontWeight = weight;
      t.fontSize = size;
      t.lineHeight = lineHeight;
      t.letterSpacing = letterSpacing;
      return t;
    })()
  );
}

// Idempotent page creation + navigation
function ensurePage(name) {
  const existing = penpotUtils.getPageByName(name);
  if (existing) {
    penpot.openPage(existing);
    return penpot.currentPage;
  }
  const page = penpot.createPage();
  page.name = name;
  penpot.openPage(page);
  return page;
}

// Idempotent board creation
function ensureBoard(name, x, y, w, h, fill = "#F5F5F5") {
  const existing = penpotUtils.findShape(
    (s) => s.type === "board" && s.name === name,
  );
  if (existing) return existing;
  const board = penpot.createBoard();
  board.name = name;
  board.resize(w, h);
  board.x = x;
  board.y = y;
  board.fills = [{ fillColor: fill, fillOpacity: 1 }];
  return board;
}

// Clear boards by name prefix (useful for re-runs)
function clearBoards(prefix) {
  penpotUtils
    .findShapes(
      (s) => s.type === "board" && s.name.startsWith(prefix),
      penpot.root,
    )
    .forEach((s) => s.remove());
}
```

### 11. Design system discovery

Run this before any design work:

```javascript
const allShapes = penpotUtils.findShapes(() => true, penpot.root);

// Colors in use on current page
const colors = new Set();
allShapes.forEach((s) => {
  if (s.fills)
    s.fills.forEach((f) => {
      if (f.fillColor) colors.add(f.fillColor);
    });
  if (s.strokes)
    s.strokes.forEach((st) => {
      if (st.strokeColor) colors.add(st.strokeColor);
    });
});

// Library assets
const components = penpot.library.local.components;
const colorStyles = penpot.library.local.colors;
const typographies = penpot.library.local.typographies;
const catalog = penpot.library.local.tokens;

// Prototype coverage
const boardsWithInteractions = allShapes
  .filter((s) => s.type === "board" && s.interactions?.length > 0)
  .map((s) => ({ name: s.name, count: s.interactions.length }));

// All pages
const pages = penpotUtils.getPages().map((p) => {
  const page = penpotUtils.getPageByName(p.name);
  return {
    name: p.name,
    boardCount: page ? page.findShapes({ type: "board" }).length : 0,
  };
});

return {
  pages,
  uniqueColorCount: colors.size,
  colorStyleCount: colorStyles.length,
  componentCount: components.length,
  typographyCount: typographies.length,
  tokenSetCount: catalog.sets.length,
  tokenThemeCount: catalog.themes.length,
  boardsWithInteractions,
  textStyleSample: [
    ...new Set(
      allShapes
        .filter((s) => s.type === "text")
        .map((s) => `${s.fontFamily} ${s.fontSize}/${s.fontWeight}`),
    ),
  ].slice(0, 10),
};
```

### 12. Board positioning

#### On current page

```javascript
// Find rightmost edge for next board
const boards = penpotUtils.findShapes((s) => s.type === "board", penpot.root);
let nextX = 0;
const GAP = 100;
boards.forEach((b) => {
  const edge = b.x + b.width;
  if (edge + GAP > nextX) nextX = edge + GAP;
});

const newBoard = penpot.createBoard();
newBoard.resize(375, 812);
newBoard.x = nextX; // direct assignment OK for root-level boards
newBoard.y = 0;
return { placedAt: { x: nextX, y: 0 } };
```

#### Across multiple pages

```javascript
// Example: replace page names and canvas sizes to match your project
const pageDefs = [
  { name: "Foundations", w: 1600, h: 900 },
  { name: "Mobile", w: 1440, h: 900 },
  { name: "Desktop", w: 1600, h: 900 },
];
// NOTE: create all pages first, then write boards in separate calls per page
```

Conventions:

- 100px gap: related screens in the same flow.
- 200px+ gap: separate flows or sections.
- Wireframes left, final design right.

### 13. CSS export

```javascript
const selection = penpot.selection;
if (!selection || selection.length === 0) return "No shape selected";
const css = penpot.generateStyle(selection[0], {
  type: "css",
  includeChildren: true,
});
return css;
```

`export_shape` (raster/SVG file) may fail with HTTP errors in remote MCP. Always verify structurally via API. Export is best-effort.

### 14. Interactions, flows and animations

#### Interaction model

```typescript
interface Interaction {
  trigger: Trigger; // "click" | "mouse-enter" | "mouse-leave" | "after-delay"
  delay?: number | null; // ms; only for "after-delay"
  action: Action;
  remove(): void;
}
type Action =
  | NavigateTo
  | OpenOverlay
  | ToggleOverlay
  | CloseOverlay
  | PreviousScreen
  | OpenUrl;
```

#### Animation types

```typescript
{ type: "dissolve", duration: 300, easing?: "linear" | "ease" | "ease-in" | "ease-out" | "ease-in-out" }
{ type: "slide", way: "in" | "out", direction: "left" | "right" | "up" | "down", duration: 300, easing?: ... }
{ type: "push", direction: "left" | "right" | "up" | "down", duration: 300, easing?: ... }
```

#### Add interactions

```javascript
const home = penpotUtils.findShape((s) => s.name === "Home");
const detail = penpotUtils.findShape((s) => s.name === "Detail");

// Navigate with animation
home.addInteraction("click", {
  type: "navigate-to",
  destination: detail,
  animation: { type: "dissolve", duration: 300, easing: "ease-in-out" },
});

// After-delay (splash screen)
home.addInteraction(
  "after-delay",
  { type: "navigate-to", destination: detail },
  2000,
);

// Open overlay
const modal = penpotUtils.findShape((s) => s.name === "overlay/confirm-delete");
home.addInteraction("click", {
  type: "open-overlay",
  destination: modal,
  position: "center",
  closeWhenClickOutside: true,
  addBackgroundOverlay: true,
  animation: { type: "dissolve", duration: 200 },
});
```

#### Create prototype flow via API

```javascript
// Flows (prototype entry points) can be created directly via Page API
const page = penpot.currentPage;
const entryBoard = penpotUtils.findShape(
  (s) => s.name === "/flows/onboarding-start",
);
page.createFlow("Onboarding", entryBoard);
// page.flows returns all Flow objects on the page
```

#### Animation duration guide

| Duration | Use |
| --- | --- |
| 100ms | Subtle state change such as toggle or checkbox |
| 200ms | Component transition such as modal open |
| 300ms | Screen navigation |
| 400ms+ | Deliberate transitions such as onboarding or hero moments |

### 15. Validation patterns

```javascript
const allBoards = penpotUtils.findShapes(
  (s) => s.type === "board",
  penpot.root,
);
const allBoardNames = new Set(allBoards.map((b) => b.name));

// Accessibility checks
const tinyText = penpotUtils.findShapes(
  (s) => s.type === "text" && Number(s.fontSize) < 12,
  penpot.root,
);
const hardCodedFills = penpotUtils.findShapes(
  (s) => s.fills?.some((f) => f.fillColor && !f.fillColorRefId),
  penpot.root,
);

// Naming checks
const autoNamed = penpotUtils.findShapes(
  (s) => /^(Rectangle|Ellipse|Text|Group|Frame|Board)\s*\d+$/.test(s.name),
  penpot.root,
);

// Prototype coverage
const unwiredBoards = allBoards
  .filter((b) => !b.interactions?.length)
  .map((b) => b.name);
const brokenInteractions = allBoards
  .flatMap((b) =>
    (b.interactions || []).map((i) => ({
      source: b.name,
      dest: i.action.destination?.name,
      broken:
        i.action.destination && !allBoardNames.has(i.action.destination.name),
    })),
  )
  .filter((i) => i.broken);

// Token coverage
const tokenSetNames = penpot.library.local.tokens.sets.map((s) => s.name);

// Nesting depth
function getDepth(shape, d = 0) {
  if (!shape.children?.length) return d;
  return Math.max(...shape.children.map((c) => getDepth(c, d + 1)));
}

return {
  tinyTextCount: tinyText.length,
  hardCodedFillCount: hardCodedFills.length,
  autoNamedCount: autoNamed.length,
  unwiredBoardCount: unwiredBoards.length,
  unwiredBoards,
  brokenInteractions,
  tokenSetNames,
  maxNestingDepth: getDepth(penpot.root),
};
```

### 16. Platform layout templates

#### Mobile (375 x 812)

```text
┌─────────────────────────────┐
│ Status Bar          (44px)  │
├─────────────────────────────┤
│ Header / Nav        (56px)  │
├─────────────────────────────┤
│ Content (scrollable) 16px H │
├─────────────────────────────┤
│ Bottom Nav / CTA    (84px)  │
└─────────────────────────────┘
```

#### Tablet (768 x 1024)

- Content max-width: 680px centered; side margins: 44px.
- Common layout: 2-column grid or navigation rail + content.
- Overlay max-width: about 50% of screen.

#### Desktop Dashboard (1440 x 900)

```text
┌──────┬──────────────────────────────────┐
│Sidebar│ Header                  (64px)  │
│ 240px ├──────────────────────────────────┤
│       │ Content Grid (4-col, gap 24px)   │
└──────┴──────────────────────────────────┘
```

#### Create platform boards

```javascript
const platforms = [
  { name: "Mobile", w: 375, h: 812 },
  { name: "Tablet", w: 768, h: 1024 },
  { name: "Desktop", w: 1440, h: 900 },
];
let x = 0;
platforms.forEach(({ name, w, h }) => {
  const b = penpot.createBoard();
  b.name = name;
  b.resize(w, h);
  b.x = x;
  b.y = 0;
  x += w + 100;
});
```

### 17. Default design tokens (fallback)

Only use these when there is no existing design system. Always prefer discovered tokens.

#### Spacing (8px base)

| Token | Value | Usage |
| --- | --- | --- |
| `spacing-xs` | 4px | Tight inline |
| `spacing-sm` | 8px | Related elements |
| `spacing-md` | 16px | Default padding |
| `spacing-lg` | 24px | Section spacing |
| `spacing-xl` | 32px | Major sections |
| `spacing-2xl` | 48px | Page-level |

#### Typography scale

| Level | Size | Weight |
| --- | --- | --- |
| Display | 48-64px | 700 |
| H1 | 32-40px | 700 |
| H2 | 24-28px | 600 |
| H3 | 20-22px | 600 |
| Body | 16px | 400 |
| Small | 14px | 400 |
| Caption | 12px | 400 |

#### Semantic colors

| Role | Example |
| --- | --- |
| Primary | Brand-defined |
| Success | #22C55E range |
| Warning | #F59E0B range |
| Error | #EF4444 range |

#### Border radius

| Token | Value | Usage |
| --- | --- | --- |
| `radius-sm` | 4px | Inputs, tags |
| `radius-md` | 8px | Cards |
| `radius-lg` | 16px | Panels |
| `radius-full` | 9999px | Pills, avatars |
| `radius-overlay` | 20px | Overlays, glass panels |

### 18. Component checklists

#### Buttons

- [ ] Min touch target 44 x 44px (iOS) / 48 x 48dp (Android)
- [ ] States: default, hover, active, disabled, loading
- [ ] WCAG AA contrast (3:1 large, 4.5:1 small text)
- [ ] Consistent border-radius

#### Form inputs

- [ ] Label above input (never placeholder-only)
- [ ] States: default, focus, error, disabled
- [ ] Error adjacent to field; min height 44px

#### Navigation

- [ ] Active state indicated; max 7 +/- 2 items
- [ ] Touch targets 48px minimum on mobile

#### Cards

- [ ] Clear hierarchy; hover/focus if interactive; empty state defined

#### Prototype checklist

- [ ] Flow entry boards named `/flows/*` and defined via API or Prototype panel
- [ ] All click targets have interactions
- [ ] No broken interaction destinations
- [ ] Overlay boards prefixed `overlay/`
- [ ] Animation durations match content weight (100/200/300ms)

#### Pre-handoff review

- [ ] Visual hierarchy unambiguous
- [ ] All spacing from token scale
- [ ] Body text >= 16px; all text WCAG AA contrast
- [ ] Loading / empty / error states designed
- [ ] All layers semantically named
- [ ] No hard-coded colors or spacing
- [ ] Interactions wired and verified

## Troubleshooting

### `/mcp` returns 405

Use `canvas mcp ...` rather than hand-rolling MCP HTTP requests. Do not rewrite the MCP path manually.

### JSON-RPC parse error

The CLI should send JSON-RPC ids as strings. If you see `Invalid JSON-RPC message`, check the CLI version and update the sandbox image.

### No plugin instance connected

The MCP server cannot find a matching browser plugin for the MCP token.

```bash
canvas init
canvas file switch <canvas-file-id> --page-id <page-id>
canvas mcp high_level_overview --json '{}'
```

### Export works but the user needs a shareable URL

Decode the MCP image response to a local file, then use the Drive skill to upload it. Do not paste local `/tmp/...` paths to the user as if they can open them outside the sandbox.

### `canvas project create` or `canvas file create` fails

Check `canvas doctor`, then report the HTTP error. Do not try to bypass the CLI with backend credentials.
