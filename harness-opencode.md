# Hivy Rust Runtime Harness Improvements From Opencode

This report studies opencode's harness, built-in tools, tool registry, session loop, shell runner, subagents, permissions, compaction, and model-provider adaptation, then maps the strongest ideas onto Hivy's Rust runtime.

The goal is not to copy opencode line-for-line. The goal is to make Hivy's harness much better for open-weight models: weaker schema following, weaker tool-call stability, more malformed JSON, repeated calls, long thinking-only loops, provider-specific quirks, and less reliable native tool use.

## Study Scope

Parallel exploration covered:

- Opencode built-in tools and active tool registry: `../opencode/packages/opencode/src/tool/*`.
- Opencode shell and process execution paths: legacy shell, V2 bash, manual shell, PTY.
- Opencode session architecture: durable V2 runner, legacy processor, compaction, permissions, subagents.
- Hivy current Rust runtime: `sandboxes/runtime/crates/{domain,tools,agent,runtime,mcp,storage}`.

Key opencode references:

- Active tool contract: `../opencode/packages/opencode/src/tool/tool.ts`.
- Active registry: `../opencode/packages/opencode/src/tool/registry.ts`.
- Session tool wrapping: `../opencode/packages/opencode/src/session/tools.ts`.
- Legacy processor: `../opencode/packages/opencode/src/session/processor.ts`.
- Legacy run loop: `../opencode/packages/opencode/src/session/prompt.ts`.
- V2 session runner: `../opencode/packages/core/src/session/runner/llm.ts`.
- V2 run coordinator: `../opencode/packages/core/src/session/run-coordinator.ts`.
- V2 tool registry: `../opencode/packages/core/src/tool/registry.ts`.
- Hivy built-in specs: `sandboxes/runtime/crates/domain/src/tool_specs.rs`.
- Hivy built-in tool wiring: `sandboxes/runtime/crates/tools/src/lib.rs`.
- Hivy model loop: `sandboxes/runtime/crates/agent/src/runner.rs`.
- Hivy OpenAI-compatible stream client: `sandboxes/runtime/crates/agent/src/model_client.rs`.

## Executive Summary

Hivy already has several useful open-weight recovery mechanisms: OpenAI-compatible SSE streaming, JSON repair, XML tool repair, thinking filtering, repeated tool-call rejection, subagents, request-user-input, update-plan, skills, MCP tools, and durable event storage.

The biggest gap is that Hivy's core harness still makes models use too many blunt tools. It has `bash`, `read_file`, `write_file`, `edit_file`, status checks, skills, subagents, conversation search, and MCP, but it does not have first-class source `glob`, source `grep`, directory listing, diagnostics/LSP, webfetch, websearch, apply_patch, todo, or a model-facing invalid-tool repair sink. This forces open-weight models into bash pipelines, which is exactly where they are most likely to quote paths badly, overrun output limits, miss context, or perform unsafe file operations.

The second biggest gap is runtime enforcement. Hivy declares limits such as tool timeout, input token budget, allowed read roots, write atomicity, and bash sandboxing, but several of these are not fully enforced. A great harness must make the correct path the easiest path for a weak model and make unsafe or unbounded behavior structurally hard.

The top priorities are:

1. Add first-class source discovery tools: `list_dir`, `glob`, `grep`.
2. Fix filesystem safety: enforce read roots, make read-before-edit per session/turn, store file freshness, implement atomic writes.
3. Upgrade `read_file` output for model usability: directory mode, numbered lines, continuation hints, binary/media handling, path suggestions.
4. Put every tool behind one registry settlement layer: schema validation, timeout/cancel, permission, output truncation, metadata, attachments, and tool-result replay.
5. Replace bash as the discovery/editing fallback with structured tools; keep bash for commands, tests, builds, and package managers.
6. Build a command supervisor for foreground/background bash with process groups, cwd correctness, streaming spool files, hard timeouts, and durable status.
7. Add post-edit feedback: formatter hooks and diagnostics/LSP summaries after `edit_file` and `write_file`.
8. Add provider/model profiles for open-weight models: schema simplification, native-vs-XML tool strategy, parallel tool-call gating, retry behavior, and sampling controls.
9. Split Hivy's large model loop into provider stream, tool executor, context builder, compactor, and session scheduler.
10. Create conformance tests that simulate weak-model failures: malformed tool JSON, XML tool calls in text, duplicate calls, dangling tool calls, long bash output, timeout, stale edits, and context overflow.

## Current Hivy Tool Surface

From `sandboxes/runtime/crates/domain/src/tool_specs.rs`, Hivy tool specs currently include:

- `bash`
- `read_file`
- `write_file`
- `cron`
- `subagent_task`
- `check_bash_status`
- `wake`
- `skills_list`
- `skills_view`
- `skills_manage`
- `search_sessions`
- `request_user_input`
- `update_plan`

From `sandboxes/runtime/crates/tools/src/lib.rs`, the built-in local filesystem/process layer wires:

- `bash`
- `read_file`
- `write_file`
- `edit_file` when write support is configured

From `sandboxes/runtime/crates/agent/src/rig_tool_registry.rs`, agent/meta tools are wired separately:

- conversation search
- background bash status
- skills
- subagents
- wake
- request user input
- update plan
- MCP-wrapped tools

The missing core coding tools are:

- source file `glob`
- source content `grep`
- directory `list_dir` or `read_file` directory mode
- `stat` or metadata tool
- `todo_write`
- `webfetch`
- `websearch`
- LSP/diagnostics tools
- `apply_patch`
- invalid-tool repair tool
- richer question tool distinct from generic user input

## Opencode's Harness Principles Worth Copying

### 1. Tool result envelope, not raw strings

Opencode tools return:

- `title`: compact user-facing summary.
- `metadata`: structured UI/event data.
- `output`: model-facing text.
- `attachments`: first-class media/data payloads.

See `../opencode/packages/opencode/src/tool/tool.ts`.

Hivy should make every `JsonTool` return a stable envelope:

```rust
pub struct ToolOutcome {
    pub title: String,
    pub model_output: String,
    pub metadata: serde_json::Value,
    pub attachments: Vec<ToolAttachment>,
    pub truncation: Option<TruncationInfo>,
}
```

Do not force all structure into a JSON string returned to the model. The model needs a clear textual result; the UI and event stream need structured metadata; future multimodal models need attachments.

### 2. Central tool settlement layer

Opencode validates input schemas, runs permission asks, executes tools, emits metadata updates, truncates output, and normalizes errors at the tool wrapper/registry boundary.

Hivy currently calls tools directly in `sandboxes/runtime/crates/agent/src/runner.rs` with `tool.call(...).await`, and configured `tool_call_timeout_seconds` is not enforced there. This should be fixed by introducing a `ToolExecutor` that is the only path for tool execution.

The Hivy `ToolExecutor` should enforce:

- schema decode before execution
- missing required argument detection
- unknown/stale tool rejection
- per-tool timeout
- cancellation propagation
- max tool calls per turn
- max consecutive tool errors
- permission decision
- output byte/line cap
- full-output spool file when truncated
- attachment normalization
- event emission for start, metadata, result, error, cancel
- model-facing retry hints for safe argument errors

### 3. Provider-specific tool schema normalization

Opencode normalizes schemas before sending them to models: inline refs, simplify optional/null, add integer safe ranges, and transform schemas per provider.

Open-weight and OpenAI-compatible providers vary wildly. Hivy should add a `ToolSchemaProfile`:

- `native_tools`: send OpenAI-compatible `tools`.
- `xml_tools`: describe tools in prompt and parse XML/text calls.
- `json_only`: one JSON object per assistant turn.
- `single_tool_only`: disable parallel tool calls.
- `simple_schema`: remove refs, unions, oneOf/anyOf, defaults, nullable ambiguity.
- `strict_schema`: for providers that actually honor JSON schema.

This should be selected by model profile, not hard-coded globally.

### 4. One provider turn, then explicit tool settlement

Opencode's V2 runner is the better architecture target: durable input admission, one provider turn, then explicit tool execution and continuation. Avoid letting a provider SDK execute tools internally.

Hivy already has a custom SSE client, which is good. Keep that. Strengthen it with a normalized provider stream trait:

```rust
enum ProviderEvent {
    TextDelta(String),
    ThinkingDelta(String),
    ToolCallDelta(ToolCallDelta),
    ToolCalls(Vec<ToolCall>),
    Usage(Usage),
    Done(FinishReason),
}
```

Then make the model loop consume only normalized events.

### 5. Replay hygiene

Opencode carefully ensures historical tool calls always have corresponding tool results. Pending, interrupted, or errored tools are replayed as explicit tool-result errors, not dangling calls.

Hivy should do the same in history reconstruction:

- Never replay an assistant tool call without a tool result.
- If a tool was cancelled, replay a tool result saying it was cancelled.
- If a tool was interrupted by session cancellation, replay that as a structured tool error.
- If a previous tool registration is stale or missing, do not execute it; return a stale-tool error result.

This matters because many OpenAI-compatible runtimes reject message histories with dangling tool calls.

## Core Tool Recommendations

### `read_file`

Current Hivy behavior:

- Reads only files.
- Returns JSON with `path`, `content`, truncation flags, total lines/bytes, offset/limit.
- Truncates to 2000 lines / 50 KB.
- Tracks files as read in a shared `files_read` set.
- Detects images by extension but only returns a note.
- `ReadFileConfig.allowed_roots` exists, but `resolve_read_path` currently ignores it.

Opencode behavior to copy:

- Same tool can read files and list directories.
- Absolute path in output.
- XML-ish model output with path/type/content tags.
- Numbered lines in file output.
- Offset/limit continuation hints.
- Directory listing sorted, directories suffixed with `/`, total count, pagination hints.
- Binary detection by extension and byte heuristic.
- Images/PDFs returned as attachments.
- Not-found suggestions from sibling directory entries.
- Per-line max length so one huge line does not destroy context.
- LSP warm/touch after file reads.
- Optional instruction reminders from nearby instruction files.

Recommended Hivy changes:

1. Fix `resolve_read_path` to enforce `ReadFileConfig.allowed_roots`.
2. Make read tracking per session and preferably per turn. Do not use a process-global set.
3. Store read freshness: path, canonical path, mtime, size, and content hash of the range or full file.
4. Add directory mode to `read_file`, or add a separate `list_dir`. For weak models, a separate `list_dir` is easier to understand.
5. Add model output format:

```xml
<file>
<path>/abs/path/src/main.rs</path>
<content>
1: fn main() {
2:     println!("hello");
3: }
</content>
</file>
```

6. Keep JSON metadata for UI, but make the model-facing result textual and line-addressable.
7. Add `offset` and `limit` continuation hints directly in the model text:

```text
<notice>File continues after line 2000. Call read_file with offset=2001.</notice>
```

8. Add binary/media behavior:

- text: line-numbered text
- image: attachment plus short model text
- PDF: text extraction if available, attachment otherwise
- binary unknown: refuse to dump bytes; return metadata and suggest bash/file-specific tools

9. Add path suggestions:

```text
File not found: /repo/src/serer.rs
Did you mean?
- /repo/src/server.rs
- /repo/src/serve.rs
```

10. Add maximum line length, for example 2000 chars, so minified files do not dominate context.
11. Add `read_many` only if needed later. First make one read highly reliable.

Why this helps open-weight models:

- Line numbers make edit targeting much more reliable.
- Directory mode avoids `ls` via bash.
- Suggestions recover spelling mistakes without another model turn.
- Attachments keep multimodal support out of ad hoc JSON strings.
- Per-turn read freshness prevents stale edits.

### `list_dir`

Opencode folds directory listing into `read`. Hivy should consider a separate `list_dir` because open-weight models benefit from narrower tools.

Proposed schema:

```json
{
  "path": "absolute or workspace-relative directory",
  "offset": 1,
  "limit": 200
}
```

Output:

```xml
<directory>
<path>/repo/src</path>
<entries>
agent/
tools/
main.rs
lib.rs
</entries>
<notice>Showing 200 of 438 entries. Continue with offset=201.</notice>
</directory>
```

Implementation details:

- Enforce read roots.
- Sort directories before files or simple lexicographic order, but be consistent.
- Include symlink marker if relevant.
- Cap total entries.
- Return absolute paths in metadata, compact names in model text.
- Never recursively list by default.

### `glob`

Hivy currently forces filename discovery through bash/find/rg. Opencode has a dedicated `glob` capped at 100 results.

Add a native `glob` tool using `ignore`, `globset`, or ripgrep-compatible walking:

Schema:

```json
{
  "pattern": "**/*.rs",
  "path": "/repo"
}
```

Behavior:

- `path` defaults to workspace root.
- `path` must be a directory under allowed roots.
- Respect `.gitignore` by default.
- Include hidden files only with an explicit option if you add one.
- Return absolute paths.
- Cap at 100 results initially.
- If truncated, say so honestly and advise narrowing the pattern.

Why this is P0:

- Weak models are bad at composing portable `find` commands.
- The model should not need bash just to discover files.
- A structured glob tool makes tool use cheaper and safer.

### `grep`

Opencode has a dedicated `grep` tool that groups results by file, includes line numbers, supports include filters, and caps output.

Add `grep` or `search_files`:

Schema:

```json
{
  "pattern": "SessionRunner",
  "path": "/repo",
  "include": "*.rs",
  "literal": false
}
```

Behavior:

- Use `ripgrep` as an implementation detail or a Rust search library.
- Default regex mode is fine, but add `literal` for weak models.
- Group by file:

```text
/repo/src/runner.rs:
117: pub struct RigAgentRunner {
455: // XML tool call repair
```

- Cap at 100 matching lines or a byte cap.
- Never claim to know total matches when truncated unless the implementation counts them.
- Prefer absolute paths.
- Enforce read roots.

Why this is P0:

- It removes a huge amount of brittle bash.
- It gives line numbers that feed directly into `read_file` and `edit_file`.

### `edit_file`

Current Hivy behavior:

- Requires read-before-edit, but the read set is shared too broadly.
- Preserves BOM and line endings.
- Applies multiple exact text replacements against the original content.
- Rejects overlaps.
- Uses simple fuzzy matching for Unicode punctuation/nbsp/dash normalization.
- Returns a unified diff.

Opencode behavior to copy:

- Per-file lock.
- Exact replacement first.
- Layered fallback matchers:
  - line-trimmed
  - block-anchor
  - whitespace-normalized
  - indentation-flexible
  - escape-normalized
  - trimmed-boundary
  - context-aware
  - multi-occurrence handling
- Guard against dangerously loose matches.
- Permission with diff metadata.
- Format after edit.
- Publish file watcher events.
- Touch LSP and return diagnostics.

Recommended Hivy changes:

1. Make read-before-edit session-scoped and turn-aware.
2. Store file hash/mtime at read time and require the current file still matches before editing, unless the edit includes an explicit stale override.
3. Add `replace_all` for safe renames.
4. Keep exact match as the primary path.
5. Add fallback matching in this order:
   - normalized line endings
   - Unicode punctuation normalization
   - trim trailing whitespace per line
   - indentation-flexible match
   - context-window match using nearby unchanged lines
6. Require high confidence for fuzzy matches:
   - one unique candidate
   - replacement span not wildly larger than requested old text
   - normalized distance below threshold
   - no match inside generated/binary/minified huge line unless exact
7. Return a clear error for ambiguity:

```json
{
  "error": "old_text matched 4 locations",
  "hint": "include more surrounding lines from the most recent read_file output"
}
```

8. Run formatter hooks after successful edit.
9. Run diagnostics after successful edit and include compact errors in the model output.
10. Keep the unified diff in metadata and optionally in model output.

Important caution:

Do not over-copy opencode's fuzzy edit behavior without stronger freshness guards. Open-weight models can produce approximate old text often. Fuzzy matching is useful, but it must be conservative.

### `write_file`

Current Hivy behavior:

- Writes content to a path after writable path resolution.
- Creates parent directories.
- Uses per-file lock.
- Enforces max size.
- `WriteFileConfig.atomic` exists but local write uses direct `tokio::fs::write`.
- No read-before-write requirement.
- No diff preview/permission metadata.
- No formatter/diagnostics.

Opencode behavior to copy:

- For existing files, read old content and compute a diff.
- Preserve BOM.
- Ask edit permission with diff metadata.
- Write, format, sync BOM.
- Publish file watcher events.
- Touch LSP and return diagnostics.

Recommended Hivy changes:

1. Enforce read-before-write for existing files, or strongly prefer `edit_file`.
2. Implement `atomic`:
   - write temp file in same directory
   - fsync temp file if appropriate
   - rename over target
   - fsync parent directory on Unix where available
3. Preserve BOM when overwriting an existing text file.
4. Preserve line endings where reasonable, or make conversion explicit.
5. Compute diff against old content and emit it as metadata.
6. Ask permission using diff metadata when permissions are enabled.
7. Run formatter hooks.
8. Run diagnostics.
9. Publish a file-changed event for UI/watchers.
10. Return output that tells the model whether diagnostics remain.

Model-facing output should be simple:

```text
Wrote /repo/src/main.rs (1428 bytes).

Diagnostics:
- src/main.rs:12:5 error[E0425]: cannot find value `foo`
```

### `apply_patch`

Opencode exposes `apply_patch` only for model/provider cases where patch-style editing is more reliable than `edit`/`write`. For other models, it hides `apply_patch` and uses edit/write.

For Hivy, add `apply_patch` as a model-profile-gated tool, not universally.

Use it when:

- The model is strong at exact diff grammar.
- The task touches multiple files.
- You need add/update/delete/move in one atomic plan.

Avoid it when:

- The model is a weaker open-weight model that often corrupts patch syntax.
- The model already struggles with JSON arguments.

Implementation details:

- Parse patch before applying.
- Derive file changes.
- Enforce read/write roots per changed path.
- Ask permission with total diff and per-file additions/deletions.
- Apply with per-file locks.
- Format changed files.
- Run diagnostics.
- Return structured per-file results.

### `bash`

Current Hivy behavior:

- Foreground bash uses `bash -lc`, configured workdir, stdin null, stdout/stderr captured separately, timeout, max output.
- Background bash uses `ProcessRegistry`, but does not set cwd.
- Deny patterns are substring matches.
- `BashConfig.sandbox` exists but local execution is not sandboxed.
- No parser-based permission extraction.
- No per-call workdir.
- No full-output spool file from byte zero.
- Background output can be clobbered because stdout and stderr tasks update shared output differently.

Opencode behavior to copy:

- `description` argument required so the model states why it is running the command.
- Per-call `workdir`.
- Shell-specific prompt guidance.
- Parse shell with tree-sitter bash/powershell.
- Detect path-affecting commands and external directories.
- Ask permission based on command pattern/arity and external directory.
- Merge environment through plugin hooks.
- Stream output to metadata while running.
- Truncate preview but spill full output to disk.
- Race exit/abort/timeout and add metadata for timeout/abort.
- Kill process tree.

Recommended Hivy changes:

1. Add args:

```json
{
  "command": "cargo test -p tools",
  "description": "Run the tools crate tests after editing read_file",
  "workdir": "/repo/sandboxes/runtime",
  "timeout_seconds": 120,
  "run_in_background": false
}
```

2. Make `workdir` explicit and validated against allowed roots.
3. Fix background cwd immediately.
4. Build one `CommandSupervisor` used by foreground bash, background bash, and future PTY:
   - process group on Unix
   - Job Object on Windows
   - stdin null by default
   - deterministic env policy
   - streaming stdout/stderr readers
   - bounded preview
   - full spool file
   - timeout and hard-kill timeout
   - cancellation token
   - exit status and signal
5. Preserve stdout/stderr ordering if possible. If not possible, label channels clearly.
6. Store background output in a file or durable output table, not only an in-memory `DashMap`.
7. Use IDs with randomness or monotonic sequence, not timestamp millis only.
8. Add parser-based risk extraction:
   - detect `rm`, `mv`, `cp`, `chmod`, `chown`, `git clean`, `git reset`, package managers, network commands
   - detect path arguments outside workspace
   - detect dynamic expansions and ask broader permission
9. Keep bash prompt guidance strict:
   - use `workdir`, not `cd`
   - quote paths
   - do not use bash for reading/writing/searching when structured tools exist
   - do not pipe to `head`/`tail` just to avoid output; output is already captured
   - use parallel tool calls for independent commands only if model profile supports it
10. Add live tests:
   - timeout kills child and grandchild
   - huge single output chunk is capped
   - invalid UTF-8 does not corrupt history
   - background process reports correct cwd
   - cancellation drains readers
   - stderr/stdout race does not lose output

### `check_bash_status`

Current Hivy has it, but background process state is process-local and output handling is weak.

Improve it:

- Return `running`, `exit_code`, `timed_out`, `started_at`, `finished_at`, `cwd`, `command`, preview, output_spool_path.
- Allow `offset`/`limit` or `tail_bytes` for long output.
- Evict old jobs only after durable settlement and retention.
- Add cancel support as a separate `cancel_bash` or status arg if needed.
- Inject completion events into the session if a background job finishes while the model is still active.

### `todo_write`

Opencode has a durable `todowrite` tool for multi-step work. Hivy has `update_plan`, which is good for UI planning, but a todo tool is still useful for long coding runs and subagent orchestration.

Add either:

- strengthen `update_plan` into durable todos, or
- add `todo_write` with structured state.

Recommended schema:

```json
{
  "todos": [
    { "id": "read-runtime", "content": "Inspect runtime handler", "status": "completed", "priority": "high" },
    { "id": "fix-bash", "content": "Fix background bash cwd", "status": "in_progress", "priority": "high" }
  ]
}
```

Rules:

- exactly one `in_progress` unless no work is active
- preserve IDs across updates
- persist per session
- publish todo-updated events
- expose compact UI separate from model output

### `subagent_task`

Current Hivy has subagents, which is already important. Opencode's task tool adds:

- child sessions
- inherited but constrained permissions
- foreground and background modes
- `task_id` reuse
- synthetic parent result injection
- clear XML-ish task envelopes

Recommended Hivy changes:

1. Treat every subagent as a durable child activity with its own event log.
2. Derive permissions from parent:
   - inherit denies
   - deny `subagent_task` by default unless explicitly allowed
   - deny write/edit/apply_patch unless subagent type allows it
3. Add budgets:
   - max turns
   - max tool calls
   - max wall time
   - max input/output tokens
4. Add background completion injection:

```text
<task id="abc" state="completed">
<summary>Inspected shell runner and found cwd bug.</summary>
<result>...</result>
</task>
```

5. Add task status polling with full result, partial summary, and errors.
6. Make task prompts include exact output contract, otherwise weak models return broad prose.
7. Keep subagent contexts fresh, but pass explicit file paths and known facts.

### `skills_list`, `skills_view`, `skills_manage`

Hivy already has skills. Opencode's `skill` tool does two things worth copying:

- Skill bodies are loaded on demand instead of bloating every prompt.
- Tool output includes base path and a small sample of files, helping the model know where references live.

Recommended changes:

- Keep skill catalog in dynamic prompt as short summaries only.
- Use `skills_view` to load full instructions.
- Include `base_dir` in metadata and model output.
- Include a bounded file sample, excluding the main skill file.
- Add skill-load permission if skills can execute or expose sensitive files.

### `webfetch`

Hivy does not appear to have a first-class webfetch tool in the core runtime.

Add it because models should not use bash/curl for ordinary reading:

Schema:

```json
{
  "url": "https://example.com/docs",
  "timeout_seconds": 30
}
```

Behavior:

- allow only `http` and `https`
- ask permission by URL/domain
- cap response bytes, for example 5 MB
- set reasonable Accept headers
- convert HTML to readable markdown/text
- strip scripts/styles
- return images as attachments
- return content type and final URL in metadata
- protect against local network fetches unless explicitly allowed

For open-weight models, webfetch should return concise text. Do not dump raw HTML unless requested.

### `websearch`

Opencode has websearch provider gating. Hivy should add websearch only if product requirements need it. When added:

- Make it provider-backed, not model-constructed browser queries.
- Permission by query or domain policy.
- Return title, URL, snippet, date when available.
- Cap results.
- Keep search separate from fetch. The model should fetch selected pages after search.

### LSP and diagnostics

Opencode's edit/write tools touch LSP and append compact diagnostics. This is one of the highest-value coding harness features.

Hivy should add a diagnostics layer:

- Start language servers lazily by workspace/language.
- Touch files after read/edit/write.
- After edit/write, collect diagnostics for changed files.
- Optionally collect a small number of related diagnostics from other files.
- Return only errors by default, max 20 per file.
- Include line/column and message.
- If LSP fails, emit a structured diagnostic-source error instead of silently returning empty diagnostics.

Add tools later:

- `diagnostics`
- `definition`
- `references`
- `hover`
- `symbols`

But the P0 value is automatic diagnostics after file mutation.

### `question` / `request_user_input`

Hivy already has `request_user_input`. Opencode's `question` tool is structured: one or more questions, options, and answer summaries.

For Hivy:

- Keep `request_user_input` as the main human-clarification tool.
- Add schema support for multiple concise questions only if UI supports it.
- Add auto-resolution/defaults for non-blocking questions if product UX needs it.
- Model output after the user answers should summarize the answer in a tool result so replay is deterministic.

### `invalid` tool

Opencode uses an invalid hidden tool and repair behavior so malformed or unknown tool calls become model-actionable feedback.

Hivy needs a model-facing invalid-call path:

- Unknown tool: return a tool result explaining available tool names.
- Bad casing: optionally repair `Read_File` to `read_file` by lowercasing if unique.
- Invalid JSON: repair with `JsonRepair` if safe; otherwise return a structured error.
- Missing required args: return exact missing fields and a corrected example.
- Tool unavailable for this model/profile: say why and suggest allowed alternatives.

Do not let unknown tool calls disappear. Weak models need explicit correction.

## Harness Architecture Roadmap

### P0: Safety and tool usability

Implement these first:

1. Enforce `ReadFileConfig.allowed_roots`.
2. Make read-before-edit state per session/turn, not global.
3. Add file freshness guards to `edit_file`.
4. Implement `WriteFileConfig.atomic`.
5. Enforce `tool_call_timeout_seconds` around every tool call.
6. Add max tool calls per turn and max wall-clock per turn.
7. Add first-class `list_dir`, `glob`, and `grep`.
8. Fix background bash cwd.
9. Add command supervisor with process group kill and output spool files.
10. Add central `ToolExecutor` result envelope and truncation.

These are the changes most likely to improve open-weight model reliability immediately.

### P1: Coding feedback loop

1. Upgrade `read_file` output with line numbers, directory mode, path suggestions, binary/media handling.
2. Add formatter hooks after edit/write.
3. Add diagnostics summaries after edit/write.
4. Add robust but conservative edit fallback matching.
5. Add diff metadata and permission previews for write/edit.
6. Add `todo_write` or make `update_plan` durable and strict.
7. Add invalid-tool correction path.
8. Add replay hygiene for cancelled/interrupted/stale tools.

### P2: Provider profiles and subagents

1. Add model profiles:
   - native OpenAI-compatible tools
   - XML/text tool repair
   - JSON-only tool calls
   - no parallel tools
   - simplified schema
   - strict schema
2. Gate `parallel_tool_calls` by model profile instead of always sending it.
3. Gate `apply_patch` by model profile.
4. Add subagent budgets and durable child activities.
5. Add background task completion injection.
6. Add webfetch and maybe websearch.
7. Add LSP navigation tools.

### P3: Long-term harness quality

1. Split `RigAgentRunner::run_turn` into modules:
   - `TurnBuilder`
   - `ProviderStream`
   - `ToolExecutor`
   - `ContextBudget`
   - `Compactor`
   - `TurnPersistence`
   - `SessionScheduler`
2. Add per-session actors:
   - one active drain per session
   - explicit resume joins existing run
   - prompt wakeups coalesce
   - different sessions run concurrently
   - shell/manual execution queues separately from provider turns
3. Add durable background job ownership and restart recovery.
4. Add snapshot/generation IDs to tool registries to reject stale tool calls.
5. Add OpenAPI/SDK contract tests before changing external API shapes.

## Open-Weight Model Strategy

Open-weight models need a harness that assumes:

- They may emit tool calls as XML/text even when native tools are available.
- They may send malformed JSON.
- They may repeat identical broken tool calls.
- They may call tools with wrong casing.
- They may omit required args.
- They may use bash when a safer tool exists.
- They may produce long internal thinking and no usable answer.
- They may ignore subtle system instructions.
- They may not respect complex JSON Schema.

Hivy already has some recovery mechanisms. Improve them with model profiles.

### Profile fields

Recommended `ModelHarnessProfile`:

```rust
pub struct ModelHarnessProfile {
    pub native_tools: bool,
    pub xml_tool_repair: bool,
    pub json_repair: bool,
    pub parallel_tool_calls: bool,
    pub max_tool_calls_per_turn: usize,
    pub strict_tool_schema: bool,
    pub simplified_tool_schema: bool,
    pub tool_name_case_repair: bool,
    pub repeat_detection: RepeatPolicy,
    pub thinking_strip: bool,
    pub thinking_only_retries: usize,
    pub cutoff_retries: usize,
    pub send_temperature: bool,
    pub send_top_p: bool,
    pub send_reasoning_effort: bool,
}
```

Use conservative defaults for open-weight local models:

- `native_tools = true` only if the endpoint actually supports them.
- `xml_tool_repair = true`.
- `json_repair = true`.
- `parallel_tool_calls = false`.
- `simplified_tool_schema = true`.
- `max_tool_calls_per_turn = 8`.
- `thinking_only_retries = 2`.
- `cutoff_retries = 2`.

### Schema simplification

For weak models:

- No nested unions.
- No `oneOf` or `anyOf`.
- No nullable optional ambiguity.
- Prefer strings, booleans, numbers, arrays of objects.
- Use short field names only when obvious.
- Put operational constraints in descriptions and runtime validation.
- Snapshot-test the final schema sent to the provider.

### Tool descriptions

Every tool description should include:

- when to use it
- when not to use it
- required path shape
- output truncation behavior
- safety caveats
- one short example

For weak models, separate tools beat overloaded tools. `list_dir`, `glob`, `grep`, and `read_file` are easier than one huge filesystem tool.

## Context and Compaction

Opencode has two useful ideas:

- Core V2: durable history compaction around sessions.
- Legacy: richer pruning of old tool outputs while keeping recent tail turns and summaries.

Hivy gaps:

- `input_token_budget` appears not fully enforced.
- `max_history_events` is hard-coded in history loading rather than driven by config.
- Compaction should be token-aware, not character-only.

Recommendations:

1. Enforce input token budget before provider call.
2. Use model-specific token estimation when available; use conservative fallback otherwise.
3. Keep recent turns intact.
4. Prune old large tool outputs first.
5. Replace old tool output bodies with placeholders:

```text
[Old tool result content cleared. Full output stored at tool-output://...]
```

6. Preserve:
   - user requests
   - assistant decisions
   - file paths changed
   - diffs or summaries of diffs
   - diagnostics
   - unresolved todos
   - subagent summaries
7. Keep full tool output in a durable store with retention.
8. Add compaction events so debugging can explain why context changed.

## Permissions

Opencode has rich permission asks with metadata and durable "allow always" rules. Its V2 and legacy permission dialects differ, but the core idea is strong.

Hivy should normalize internally around:

```rust
enum PermissionAction {
    Read,
    Write,
    Edit,
    Execute,
    Network,
    SpawnSubagent,
    UseMcpTool,
}

struct PermissionRequest {
    action: PermissionAction,
    resource: String,
    tool: String,
    metadata: serde_json::Value,
}
```

For tool-specific metadata:

- read: canonical path, root classification
- write/edit/apply_patch: unified diff, additions/deletions, file list
- bash: parsed command, risk class, path arguments, external dirs
- grep/glob: pattern, root path
- webfetch: URL, host, content type
- subagent: subagent type, model, permissions, budget
- MCP: server, tool, args summary

Rules:

- Saved approvals should be project-scoped.
- Reject should cancel pending requests for that session if appropriate.
- "Allow always" should store exact resource patterns, not broad accidental strings.
- Permission prompts should show diffs/previews, not raw JSON args.

## Event Model and Observability

For a great harness, debugging must be first-class.

Every turn should emit:

- turn started
- model request started
- provider endpoint selected
- text delta
- thinking delta if visible
- tool call started
- tool metadata updated
- tool result
- tool error
- tool timeout
- tool cancelled
- model usage
- compaction event
- retry event
- final message

Every tool result should store:

- session id
- turn id
- tool call id
- tool name
- args hash
- start/end time
- duration
- status
- output preview
- output store pointer
- attachment metadata
- permission decision

Add a debug export command later that reconstructs a full turn, including provider request after redaction.

## Concrete File-Level Recommendations For Hivy

### `sandboxes/runtime/crates/domain/src/tool_specs.rs`

Add specs:

- `ListDir`
- `Glob`
- `Grep`
- `TodoWrite`
- `WebFetch`
- `Diagnostics`
- `ApplyPatch` gated by profile

Add config:

- `max_results`
- `include_hidden`
- `respect_gitignore`
- `network_allowlist`
- `diagnostics_enabled`
- `formatter_enabled`

Tighten existing specs:

- `BashConfig.sandbox` should either be implemented or renamed to avoid false security.
- `ReadFileConfig.allowed_roots` must be enforced.
- `WriteFileConfig.atomic` must be enforced.
- Add per-tool timeout defaults.

### `sandboxes/runtime/crates/tools/src/path.rs`

Fix:

- `resolve_read_path` must call the same root policy as write, with read-specific defaults.
- Canonicalization should handle non-existent write targets by canonicalizing nearest existing parent.
- Symlinks must not escape allowed roots after canonicalization.
- Return a structured `ResolvedPath` with:
  - original input
  - absolute path
  - canonical path if exists
  - root classification
  - existed bool

### `sandboxes/runtime/crates/tools/src/read.rs`

Implement:

- directory mode or delegate to `list_dir`
- line-numbered model output
- path suggestions
- binary sniffing
- image/PDF attachments
- per-line cap
- session-scoped read tracking with freshness
- better continuation hints

### `sandboxes/runtime/crates/tools/src/edit.rs`

Implement:

- read freshness guard
- `replace_all`
- conservative fallback matchers
- formatter hook
- diagnostics hook
- diff metadata
- clearer ambiguity errors

### `sandboxes/runtime/crates/tools/src/write.rs`

Implement:

- atomic writes
- read-before-write for existing files
- BOM and line ending preservation
- diff metadata
- formatter hook
- diagnostics hook
- watcher event

### `sandboxes/runtime/crates/tools/src/bash.rs`

Implement:

- `description`
- per-call `workdir`
- parser-based permission extraction
- sandbox enforcement or explicit no-sandbox naming
- output spool pointer
- foreground and background use the same supervisor

### `sandboxes/runtime/crates/tools/src/process_registry.rs`

Fix:

- set cwd for background processes
- use non-colliding IDs
- avoid stdout/stderr output clobbering
- store output durably
- use process groups
- add cancel
- distinguish running, exited, timed out, cancelled, failed-to-start

### `sandboxes/runtime/crates/tools/src/operations.rs`

Improve:

- chronological output or channel-labelled merged output
- invalid UTF-8 handling
- process tree kill
- flush/drain semantics after timeout
- temp-file atomic writes

### `sandboxes/runtime/crates/agent/src/runner.rs`

Refactor gradually:

- introduce `ToolExecutor` first without large behavior changes
- enforce tool timeout around `tool.call`
- add max tool calls per turn
- move XML repair and repeat detection into named modules
- move tool dispatch into executor
- move history budgeting into context builder
- gate `parallel_tool_calls` by profile

### `sandboxes/runtime/crates/agent/src/request_builder.rs`

Change:

- Do not always send `parallel_tool_calls: true`.
- Apply provider/model schema profile.
- Omit unsupported params per provider.
- Add option for tool-choice control if providers support it.
- Support non-OpenAI-compatible providers behind traits later.

### `sandboxes/runtime/crates/agent/src/model_client.rs`

Keep:

- SSE parsing with UTF-8 carry.
- idle timeout.
- usage parsing.
- JSON repair/tool accumulator.
- fallback endpoints for retryable failures.

Improve:

- expose raw provider finish reason in events.
- make invalid JSON SSE frames observable under debug.
- model profile should classify provider quirks.
- add tests for split UTF-8, CRLF SSE boundaries, malformed tool args, stream end without `[DONE]`.

## Evaluation Suite

Build evals around failure modes, not only happy paths.

### Filesystem evals

- model finds a file by glob without bash
- model searches content by grep without bash
- model reads directory, then file, then edits correctly
- edit rejects stale file after external modification
- edit handles CRLF and BOM
- edit rejects ambiguous old text
- write creates new file atomically
- write existing file requires read or returns strong guidance
- read denies path outside allowed roots
- symlink escape is denied

### Bash evals

- command runs in requested cwd
- background command runs in requested cwd
- timeout kills child and grandchild
- large output spools full output and returns bounded preview
- stderr/stdout are not lost
- invalid UTF-8 is represented safely
- denied dangerous command produces model-actionable error

### Open-weight model evals

- malformed JSON tool call repaired
- XML tool call in assistant text extracted
- native empty-args tool call repaired from XML text
- wrong tool casing repaired
- repeated identical invalid call rejected
- thinking-only response retried
- cutoff response reprompted
- unknown tool returns invalid-tool guidance
- no dangling tool calls after cancellation

### Subagent evals

- foreground subagent returns result
- background subagent completion injects result
- subagent cannot write when denied
- subagent budget stops runaway work
- parent can poll status by task id

### Diagnostics evals

- edit introduces compile/type error and diagnostics appear
- write fixes error and diagnostics clear
- LSP failure emits diagnostic-source warning instead of silent empty result

## Suggested Implementation Order

1. Fix safety bugs:
   - enforce read roots
   - session-scoped read tracking
   - atomic writes
   - tool timeouts
   - background bash cwd
2. Add source tools:
   - `list_dir`
   - `glob`
   - `grep`
3. Add central `ToolExecutor` envelope and truncation.
4. Upgrade `read_file` output.
5. Upgrade edit/write with freshness, formatter, diagnostics.
6. Build command supervisor and output spool store.
7. Add provider/tool profiles for open-weight models.
8. Add invalid-tool correction path.
9. Add durable todo/subagent improvements.
10. Add webfetch/LSP navigation/apply_patch as profile-gated advanced tools.

## What Not To Copy Blindly

- Do not port opencode's legacy `SessionPrompt` as one giant Rust module. It is feature-rich but too monolithic.
- Do not expose fuzzy edit matching without freshness and confidence guards.
- Do not expose `apply_patch` to every model. Gate it by profile.
- Do not rely on shell prompts to keep models safe. Enforce safety in runtime.
- Do not imply bash has persistent cwd if every call starts a new process.
- Do not make LSP failures look like "no diagnostics". Report diagnostic source failures.
- Do not make background jobs process-local only if product UX expects recovery after restart.

## Final Target State

The ideal Hivy harness should make this the normal model workflow:

1. `list_dir` or `glob` to discover files.
2. `grep` to find symbols/usages.
3. `read_file` to inspect exact context with line numbers.
4. `edit_file` for surgical changes.
5. `write_file` only for new files or intentional full rewrites.
6. formatter and diagnostics run automatically.
7. `bash` runs tests/builds with bounded, spooled output.
8. repeat/cutoff/malformed tool behavior is repaired or rejected with clear guidance.
9. history replay remains valid for every provider.
10. subagents run as durable, permission-scoped child activities.

That is the path to a much stronger open-weight harness: fewer opportunities for the model to improvise unsafe shell commands, more structured feedback after every action, and runtime enforcement for the cases where model instructions are not enough.
