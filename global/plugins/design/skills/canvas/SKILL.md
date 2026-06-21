---
name: canvas
description: Use when creating, opening, inspecting, editing, exporting, or collaborating on Hivy Canvas design files from a sandbox using the built-in `canvas` CLI, the isolated Canvas browser session, or Canvas MCP tools.
---

# Hivy Canvas

Use the built-in `canvas` CLI to work in Hivy-managed Canvas files from the sandbox. The CLI hides authentication, browser setup, and MCP protocol details. From the agent's perspective, Canvas work is a sequence of CLI commands; from the user's perspective, the file updates live in their Canvas iframe/browser.

## Operating model

- `canvas init` opens an isolated browser session named `canvas`.
- The browser tab must stay open on the design file for plugin-backed MCP tools to work.
- `canvas file switch` navigates that browser tab to the target Canvas file/page.
- `canvas mcp <tool>` calls the deployed MCP server, which routes tool calls to the plugin in the open browser tab.
- `canvas project create` and `canvas file create` call Hivy runtime endpoints for the current agent and org.

## Core rules

- Start Canvas work with `canvas doctor`, then `canvas init`.
- Before modifying a design with MCP, ensure a Canvas file is open in the `canvas` browser session.
- Call `canvas mcp high_level_overview --json '{}'` once before using `execute_code`.
- Use `canvas file current` before destructive or large edits to confirm the active file/page.
- Prefer `execute_code` for structured design edits and `export_shape` for visual verification.
- Return or store shape ids from `execute_code`; use those ids for later edits and exports.
- Keep the open browser tab focused on the file the agent is editing. If the user asks to switch tasks/files, call `canvas file switch`.
- For screenshots of the full Canvas UI, use the separate `browser` CLI in session `canvas`.
- For durable exported assets, use the Drive skill after exporting to a local file.

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

Confirm the browser URL:

```bash
browser --session canvas get url
```

Example output:

```text
https://canvas.usehivy.com/#/workspace?team-id=...&file-id=...&page-id=...
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
browser --session canvas wait 5000
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
file /tmp/page-export.png
```

## Production MCP tools

These are the tools registered by the deployed multi-user Canvas MCP server that Hivy Canvas uses by default:

- `high_level_overview`
- `penpot_api_info`
- `execute_code`
- `export_shape`

The Canvas codebase defines additional local/dev-only tools; they are documented later in this skill, but do not assume they are callable in production Canvas.

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

When file-system access is enabled locally, the codebase appends:

```text
Alternatively, you can save it to a file.
```

In deployed Hivy Canvas, file-system access is remote-disabled; do not rely on `filePath`. Capture the image data from the MCP JSON response and write it locally if needed.

Input schema in deployed Canvas:

```json
{
  "shapeId": "string, required. Use a shape id, 'selection', or 'page'",
  "format": "png or svg, optional, default png",
  "mode": "shape or fill, optional, default shape"
}
```

Use for visual verification, PNG/SVG exports, and screenshots of a specific board or the current page.

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
file /tmp/export.png
```

```bash
# 5. Export only the raw image fill for a shape that has an image fill.
canvas mcp export_shape --json '{"shapeId":"<image-filled-shape-id>","format":"png","mode":"fill"}' > /tmp/fill-export.json
```

## Non-Production Local Canvas MCP Tools

These tools are defined in the Canvas MCP codebase but are not available in the deployed Hivy Canvas MCP server unless the server is running in local development mode. Do not use them for production Canvas work unless you have verified the tool is registered.

### `import_image` local-only

Source tool description:

```text
Imports a pixel image from the local file system into Canvas by creating a Rectangle instance that uses the image as a fill. The rectangle has the image's original proportions by default. Optionally accepts position (x, y) and dimensions (width, height) parameters. If only one dimension is provided, the other is calculated to maintain the image's aspect ratio. Supported formats: JPEG, PNG, GIF, WEBP.
```

Input schema:

```json
{
  "filePath": "string, required, absolute path to image",
  "x": "number, optional",
  "y": "number, optional",
  "width": "positive number, optional",
  "height": "positive number, optional"
}
```

Samples:

```bash
canvas mcp import_image --json '{"filePath":"/workspace/assets/logo.png"}'
```

```bash
canvas mcp import_image --json '{"filePath":"/workspace/assets/hero.webp","x":120,"y":160,"width":640}'
```

```bash
canvas mcp import_image --json '{"filePath":"/workspace/assets/avatar.jpg","x":80,"y":80,"height":128}'
```

### `cljs_repl` devenv-only

Source tool description:

```text
Persistent ClojureScript REPL in the Canvas frontend runtime (via shadow-cljs nREPL). Definitions, requires, and state are preserved across calls. Use it to build up helpers incrementally. Multiple top-level expressions per call are supported; each produces a result line.
```

Input schema:

```json
{
  "code": "string, required, ClojureScript code"
}
```

Samples:

```bash
canvas mcp cljs_repl --json '{"code":"(keys @app.main.store/state)"}'
```

```bash
canvas mcp cljs_repl --json '{"code":"(get-in @app.main.store/state [:current-file-id])"}'
```

```bash
canvas mcp cljs_repl --json '{"code":"(do (require (quote [app.main.store :as st])) (count @st/state))"}'
```

### `import_penpot_file` devenv-only

Source tool description:

```text
Imports a `.penpot` archive into the running Canvas instance from a given URL. The file is imported into the user's Drafts project. Returns the names of the imported files.
```

Input schema:

```json
{
  "url": "string, required, URL of the .penpot file to import"
}
```

Samples:

```bash
canvas mcp import_penpot_file --json '{"url":"https://example.com/sample.penpot"}'
```

```bash
canvas mcp import_penpot_file --json '{"url":"https://storage.example.com/designs/mobile-flow.penpot"}'
```

```bash
canvas mcp import_penpot_file --json '{"url":"https://raw.githubusercontent.com/org/repo/main/fixtures/demo.penpot"}'
```

### `cljs_compiler_output` devenv-only

Source tool description:

```text
Reports the status of the most recent shadow-cljs `:main` build. Use this to diagnose compilation errors when needed. For syntax errors, consider using the clj_check_parentheses tool on the relevant source files.
```

Input schema:

```json
{}
```

Samples:

```bash
canvas mcp cljs_compiler_output --json '{}'
```

```bash
canvas mcp cljs_compiler_output --json '{}' | jq -r '.result.content[0].text'
```

```bash
canvas mcp cljs_compiler_output --json '{}' > /tmp/cljs-compiler-output.json
```

### `clj_check_parentheses` devenv-only

Source tool description:

```text
Analyzes a Clojure/ClojureScript source file for unclosed delimiters and reports the area of interest.
```

Input schema:

```json
{
  "file": "string, required, absolute path to a Clojure/ClojureScript source file"
}
```

Samples:

```bash
canvas mcp clj_check_parentheses --json '{"file":"/workspace/penpot/frontend/src/app/main/ui/workspace.cljs"}'
```

```bash
canvas mcp clj_check_parentheses --json '{"file":"/workspace/penpot/backend/src/app/http/hivy.clj"}'
```

```bash
canvas mcp clj_check_parentheses --json '{"file":"/workspace/penpot/common/src/app/common/types/file.cljc"}'
```

### `read_taiga_issue` devenv-only

Source tool description:

```text
Reads a Canvas issue from the Taiga project tracker, returning its subject, description, status, attachments, and comments.
```

Input schema:

```json
{
  "issueNumber": "positive integer, required, issue number from a Taiga URL"
}
```

Samples:

```bash
canvas mcp read_taiga_issue --json '{"issueNumber":14177}'
```

```bash
canvas mcp read_taiga_issue --json '{"issueNumber":12000}' | jq -r '.result.content[0].text'
```

```bash
canvas mcp read_taiga_issue --json '{"issueNumber":14177}' > /tmp/taiga-issue.json
```

## Browser evidence commands

Use these when you need a screenshot of the live Canvas UI rather than a clean MCP export.

```bash
browser --session canvas get url
browser --session canvas get title
browser --session canvas set viewport 1440 1000
browser --session canvas screenshot /tmp/canvas.png
```

The MCP indicator should be visible in the toolbar when the plugin bridge is active. If MCP calls fail with no connected plugin, reload/switch the file and wait:

```bash
canvas file switch <canvas-file-id> --page-id <page-id>
browser --session canvas wait 5000
browser --session canvas screenshot /tmp/canvas-after-switch.png
```

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
browser --session canvas wait 5000
canvas mcp high_level_overview --json '{}'
```

### Export works but the user needs a shareable URL

Decode the MCP image response to a local file, then use the Drive skill to upload it. Do not paste local `/tmp/...` paths to the user as if they can open them outside the sandbox.

### `canvas project create` or `canvas file create` fails

Check `canvas doctor`, then report the HTTP error. Do not try to bypass the CLI with backend credentials.
