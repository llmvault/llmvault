# Harness Codex Study: Runtime and Tooling Strategy for Hivy

Date: 2026-06-19

Scope: this report studies Codex's Rust harness and tool runtime, then translates the lessons into concrete improvements for the Hivy Rust runtime under `/Users/bahdcoder/code/usehivy.com/sandboxes/runtime`. The goal is a harness that makes open-weight and weaker tool-calling models substantially more reliable.

## Sources Studied

Codex areas studied:

- `codex-rs/tools`: shared tool abstractions, tool specs, schema handling, deferred discovery, executor traits, tool outputs.
- `codex-rs/core/src/tools`: tool registry, router, planning, shell tools, `apply_patch`, MCP, image view, planning, context tools, plugin tools, multi-agent tools, and hosted tools.
- `codex-rs/core/src/unified_exec`: process manager, PTY/pipe execution, stdin sessions, output buffering, cancellation, streaming, and process lifecycle.
- `codex-rs/apply-patch`: freeform patch grammar, parser, shell invocation detection, verification, application, and committed diff tracking.
- `codex-rs/core/src/context_manager`, `compact.rs`, and `session/turn.rs`: append-only history, normalization, bounded context injection, compaction, prompt building, and turn loop behavior.
- `codex-rs/protocol/src/openai_models.rs`: model capability metadata that gates tool choice and context behavior.

Hivy areas studied:

- `sandboxes/runtime/crates/domain/src/tool_specs.rs`: current built-in tool catalog.
- `sandboxes/runtime/crates/tools/src/*`: `bash`, `read_file`, `write_file`, `edit_file`, path policy, truncation, process registry, and diff helpers.
- `sandboxes/runtime/crates/agent/src/runner.rs`: turn loop, tool execution, repair path, MCP merging, history loading, and prompt rendering.
- `sandboxes/runtime/crates/agent/src/request_builder.rs` and `model_client.rs`: OpenAI-compatible request shape, streaming, usage, thinking deltas, and tool-call accumulation.
- `sandboxes/runtime/crates/agent/src/compaction.rs`: token estimation and deterministic compaction.
- `sandboxes/runtime/crates/safety/src/*`: JSON repair, XML tool repair, thinking guard, overthinking guard, repeat detection.
- `sandboxes/runtime/crates/mcp/src/lib.rs`: MCP connection, tool discovery, prefixing, and dispatch.
- `internal/registry/registry.go` and `internal/agentruntime/compile_defaults.go`: curated model metadata and current fixed runtime defaults.

Five parallel read-only agents were used to split the study: Codex tool architecture, Codex exec runtime, Codex file editing, Codex context/session loop, and current Hivy gaps.

## Executive Summary

Hivy already has useful foundations: Rust tool traits, built-in `bash`, `read_file`, `write_file`, `edit_file`, skills, MCP, streaming SSE parsing, XML tool repair, JSON repair, thinking stripping, overthinking detection, repeat detection, persisted history, compaction, and background subagent tasks.

The main gap is not one missing function. The main gap is that Hivy does not yet have a Codex-class harness shape:

1. Model capability metadata is not compiled into a runtime profile that controls context limits, tool exposure, parallel calls, request dialect, repair policy, and retry behavior.
2. Tools are exposed as one flat JSON function list. Codex separates model-visible specs, dispatch routing, dynamic tools, deferred tools, hidden compatibility tools, and runtime outputs.
3. Shell execution is too coarse. Hivy has `bash` plus background polling; Codex has a session-oriented `exec_command` and `write_stdin` contract with PTY/pipe choice, yield time, output caps, process ids, and robust cleanup.
4. File tools are better than raw shell, but Hivy still needs first-class search/list/patch primitives. Codex's patch pipeline is safer than full overwrite or ad hoc edit replacement because it parses, verifies, previews, applies, and records committed deltas.
5. Context management must become model-window-aware before each request. Current Hivy defaults use large static budgets and optional compaction; open-weight models need conservative preflight budgeting and context-length recovery.
6. Tool-call repair should be explicit. Hivy repairs malformed JSON and XML in several places, but some unrecoverable cases are silently weakened to `{}` or filtered away. Open-weight models need bounded feedback about malformed calls so they can self-correct.

The best direction is not to copy Codex line-for-line. The right direction is to copy its architecture: model profiles, small direct tool set, deferred discovery, typed tool runtime, robust exec sessions, patch-first editing, bounded context fragments, hard output caps, strict lifecycle events, and evals that measure tool-call success.

## Current Hivy Baseline

Good existing pieces:

- `JsonTool` trait and `ToolDefinition` are a reasonable minimal abstraction.
- `read_file` supports offsets and limits, normalizes numeric strings, tracks files read, detects images, and caps output to 2000 lines or 50 KB.
- `edit_file` requires prior read, requires unique `old_text`, rejects overlapping edits, preserves BOM and line endings, and returns a unified diff.
- `write_file` uses a per-file mutation lock and size caps.
- `bash` supports timeout, output truncation, deny patterns, env passthrough, and background execution.
- The agent runner validates missing required arguments and feeds model-readable tool errors back into history.
- JSON repair accepts common weak-model mistakes: trailing commas, single quotes, unquoted keys, Python `None`/`True`/`False`, unbalanced braces, and unterminated strings.
- XML repair can recover text-encoded tool calls.
- Thinking and overthinking controls already exist.
- MCP discovery and prefixed MCP tool dispatch already exist.
- Skills are progressively disclosed through `skills_list` and `skill_view`.

Important gaps:

- `internal/registry` has model context and capability metadata, but `compile_defaults.go` still emits generic defaults: OpenAI-compatible provider, max output 8192, input budget 180000, low reasoning.
- Rust `ModelConfig` only knows generic OpenAI-compatible fields. It does not carry provider dialect, tool reliability, parallel policy, context window, cache policy, or repair settings.
- `request_builder.rs` always emits chat-completions style JSON and always sets `parallel_tool_calls: true` when tools exist.
- Context compaction uses a hardcoded 128k fallback window and is optional. Context-length errors are classified as non-fallback.
- `bash` is a one-shot command plus a separate background status registry. It does not expose PTY sessions, stdin writes, yield time, process termination, or stable polling semantics to the model.
- Background bash keeps only a 10 KB tail and has no head/tail retained transcript.
- `read_file` allows absolute process-readable paths by default. That is convenient, but unsafe as a default for agentic coding unless gated by policy.
- `write_file` overwrites existing files without requiring a version hash or prior read.
- `edit_file` is good for small replacements but does not support a multi-file patch grammar, move/delete/add operations, or exact committed deltas.
- There is no first-class `rg`/search or file listing tool. Models must use shell for discovery.
- MCP tools are all merged into the visible tool set. There is no deferred `tool_search` equivalent.
- Tool outputs are raw JSON strings in model history. There is no per-tool output contract with model-facing text, telemetry preview, structured event payload, and code-mode result.
- Repeat detection only catches identical calls. It does not catch semantic loops such as `cat file`, `sed file`, `read_file same file with slightly different limits`, or repeated failing edits with tiny argument changes.

## Codex Principles To Copy

### 1. Tool Specs And Runtimes Are Separate

Codex separates:

- model-facing `ToolSpec`
- execution-side `ToolExecutor`
- `ToolRouter` that normalizes model output
- `ToolRegistry` that dispatches, hooks, telemetry, and lifecycle events
- per-tool output conversion back into model input

Hivy should add the same split. Today `JsonTool` bundles definition and execution. That is simple, but it makes it hard to support deferred loading, hidden compatibility names, tool families, per-tool parallel safety, and model-specific presentation.

Target shape:

```rust
pub enum HarnessToolSpec {
    Function(FunctionToolSpec),
    Namespace(NamespaceToolSpec),
    Freeform(FreeformToolSpec),
    Deferred(DeferredToolSpec),
}

pub trait ToolExecutor: Send + Sync {
    fn id(&self) -> ToolId;
    fn spec(&self) -> HarnessToolSpec;
    fn exposure(&self) -> ToolExposure;
    fn parallelism(&self) -> ToolParallelism;
    async fn run(&self, call: ToolCall, ctx: ToolRuntimeContext) -> ToolResult;
}

pub enum ToolExposure {
    Direct,
    Deferred,
    HiddenDispatchOnly,
    DirectModelOnly,
}
```

Use this to keep Hivy's public tool surface small while retaining runtime compatibility.

### 2. Model Metadata Must Drive The Harness

Codex has a `ModelInfo` layer with context window, truncation policy, shell tool type, apply-patch tool type, parallel tool support, image support, hosted search support, tool mode, and compaction threshold.

Hivy already has model metadata in Go. Convert that into a Rust-facing `RuntimeModelProfile` at agent compile time.

Recommended fields:

```rust
pub struct RuntimeModelProfile {
    pub provider: ProviderDialect,
    pub model_id: String,
    pub family: Option<String>,
    pub open_weights: bool,
    pub context_window_tokens: u32,
    pub effective_context_window_percent: f32,
    pub max_output_tokens: u32,
    pub supports_native_tools: bool,
    pub native_tool_reliability: ToolReliability,
    pub supports_parallel_tools: bool,
    pub default_parallel_tools: bool,
    pub supports_vision: bool,
    pub supports_reasoning_effort: bool,
    pub reasoning_field: Option<ReasoningFieldDialect>,
    pub supports_prompt_cache: bool,
    pub cache_policy: CachePolicy,
    pub stream_dialect: StreamDialect,
    pub tool_argument_repair: RepairPolicy,
    pub xml_tool_repair: RepairPolicy,
    pub context_length_retry: ContextLengthRetryPolicy,
    pub preferred_tool_surface: ToolSurfaceMode,
}
```

This profile should be compiled from:

- curated Go registry model limits and flags
- provider-specific defaults
- Hivy eval data
- per-agent override settings

It should control:

- request body fields
- parallel tool calls
- direct vs deferred tool exposure
- native JSON tools vs XML fallback
- max tool schema budget
- compaction threshold
- retry and repair policy
- vision attachment handling
- output token cap

### 3. Keep Direct Tools Small, Make Everything Else Searchable

Codex supports `Direct`, `Deferred`, `DirectModelOnly`, and `Hidden` tools. Deferred tools are discoverable through `tool_search`.

Open-weight models degrade when they see too many tools. Hivy should expose a small direct set by default:

- `read_file`
- `search`
- `list_files`
- `apply_patch`
- `exec_command`
- `write_stdin`
- `update_plan`
- `tool_search`

Everything else can be deferred:

- MCP tools
- skills management
- session search
- wake
- subagent tools
- browser tools
- image generation or hosted web tools
- plugin/tool install tools

For weak models, prefer even fewer direct tools:

- `read_file`
- `search`
- `apply_patch`
- `exec_command`
- `tool_search`

### 4. Schema Size Is A Runtime Budget

Codex normalizes and compacts schemas because tool schemas can dominate prompt tokens. Hivy should add:

- schema byte budget per request
- schema token budget per model profile
- stripped descriptions for deferred tools
- schema deduplication for repeated MCP shapes
- max depth for schemas
- max enum values displayed
- stable sorted tool order

Suggested defaults:

- direct tool schema budget: 8k to 16k tokens depending on model
- deferred search metadata budget: 2k to 4k tokens
- individual schema max: 4k bytes before compaction
- MCP tool max visible directly: 8 to 12 tools; defer the rest

### 5. Tool Outputs Need A Contract

Codex's outputs have separate forms:

- model-facing output
- telemetry preview
- structured event payload
- post-tool hook payload
- code-mode JSON result where relevant

Hivy should stop treating every tool result as just `serde_json::Value.to_string()` in history. Add a `ToolOutput` trait or enum:

```rust
pub struct ToolOutput {
    pub model_text: String,
    pub event_json: serde_json::Value,
    pub telemetry_preview: String,
    pub changed_files: Vec<PathBuf>,
    pub tokens_before_truncation: Option<u32>,
    pub truncated: bool,
}
```

This gives each tool a stable model contract without losing structured runtime observability.

## Tool Catalog Recommendations

### `read_file`

Keep Hivy's current `read_file`, but tighten and extend it.

Current strengths:

- line offset and limit
- output metadata
- UTF-8 enforcement
- file-read tracking for edits
- image detection
- numeric string repair for offsets and limits

Changes:

- Make absolute reads policy-controlled. Default to workspace root and explicit allowed roots. Do not allow arbitrary process-readable absolute paths by default.
- Return line numbers or an option to include them. Models edit more accurately when reads include line ranges.
- Add `start_line`/`end_line` aliases in addition to `offset`/`limit` for weak-model friendliness.
- Include `sha256` or `version` in every successful read. Require that version for destructive overwrites and optionally for patches.
- Add `max_bytes` as a request parameter clamped by policy, not only a fixed 50 KB output cap.
- Use head/tail truncation for very large files when no range is provided, not head-only. Head-only hides the most recent errors in logs and generated files.
- Add a specific binary-file response with MIME and size rather than just UTF-8 error.
- Reset or scope `files_read` by turn/session intentionally. Current long-lived read tracking can allow stale blind edits much later.

Recommended model output shape:

```json
{
  "path": "...",
  "version": "sha256:...",
  "range": {"start": 1, "end": 200},
  "total_lines": 1200,
  "total_bytes": 98542,
  "truncated": true,
  "content": "..."
}
```

### `search` / `rg`

This is the biggest missing day-to-day coding tool. Codex leans on shell `rg`, but Hivy should expose first-class search because open-weight models are worse at composing safe shell search commands.

Add `search`:

```json
{
  "query": "string",
  "path": "optional relative path",
  "glob": "optional glob",
  "case_sensitive": false,
  "regex": false,
  "context_lines": 2,
  "max_matches": 100,
  "max_files": 50
}
```

Implementation details:

- Prefer `ripgrep` when installed, with a Rust fallback using `ignore` + `grep` crates.
- Always respect `.gitignore` by default.
- Hide binary files by default.
- Return file path, line number, line text, and optional context.
- Cap matches and bytes hard.
- Include a note when results are truncated and how to narrow.
- Support literal mode by default; require `regex=true` for regex.
- Keep this parallel-safe and read-only.

### `list_files`

Add a first-class file listing tool:

```json
{
  "path": ".",
  "depth": 2,
  "glob": "*.rs",
  "include_hidden": false,
  "max_entries": 200
}
```

Return:

- sorted entries
- type: file, dir, symlink
- size for files
- truncation metadata

This avoids weak models using `find .` with unbounded output.

### `apply_patch`

This should become Hivy's primary edit tool. Keep `edit_file` as a simple compatibility tool, but prefer patch for multi-file code changes.

Codex's key design:

1. The model emits a narrow freeform grammar, not arbitrary shell.
2. Runtime parses the patch before any mutation.
3. Runtime resolves and verifies paths against current filesystem state.
4. Runtime computes a diff preview from actual files.
5. Runtime checks sandbox/write policy per path.
6. Runtime applies the patch.
7. Runtime records committed deltas and whether they are exact.

Recommended supported operations:

- add file
- delete file
- update file
- move file

Recommended Hivy grammar can copy Codex's shape:

```text
*** Begin Patch
*** Add File: path
+new line
*** Update File: path
@@
-old line
+new line
*** Delete File: path
*** End Patch
```

Required behavior:

- Reject raw patch bodies not wrapped in the tool grammar.
- Reject path traversal.
- Include move destinations in write permission checks.
- Refuse ambiguous update hunks.
- Validate delete/update content against the current file.
- Preserve final newline behavior predictably.
- Apply writes with temp-file and atomic rename where possible.
- Return changed files, bytes written, diff, and exact/inexact flag.
- If any partial write may have happened, mark the delta inexact and force the next turn to recompute diff from disk.

Do not trust the model's patch text as the preview. Compute preview from disk.

### `write_file`

Keep it, but demote it.

Current Hivy behavior overwrites existing files. That is high-risk for weaker models.

Changes:

- For new files, allow write with size cap.
- For existing files, require either:
  - prior `read_file` in the current turn or recent scoped session, plus matching `version`
  - or explicit `overwrite_existing: true` with an approval/policy path
- Return unified diff for overwrites.
- Honor the existing `atomic` config. Currently config has `atomic`, but local operations use `tokio::fs::write`, which is not atomic.
- Default writable roots should be workspace only. `/tmp`, `/var/tmp`, and `$HOME` should be opt-in roots per agent or tool profile.

### `edit_file`

Keep Hivy's current `edit_file`, but treat it as a convenience patch builder:

- Add optional `version` from `read_file`.
- Return the exact old/new diff and changed byte ranges.
- Reject edits after stale reads when file version changed.
- Add a maximum replacement count per call.
- Add better failure diagnostics: not found, ambiguous, fuzzy match used, overlap.
- Consider internally translating `edit_file` to `apply_patch` once patch exists.

### `exec_command` And `write_stdin`

Replace model-facing `bash` with Codex-style unified exec tools.

Current Hivy:

- `bash(command, timeout_seconds, run_in_background)`
- `check_bash_status(process_id)`
- no stdin
- no PTY
- no yield/poll model
- no explicit process lifecycle output

Target:

`exec_command`:

```json
{
  "cmd": "string",
  "workdir": "optional path",
  "tty": false,
  "yield_time_ms": 10000,
  "max_output_tokens": 10000,
  "shell": "optional shell",
  "login": true
}
```

`write_stdin`:

```json
{
  "session_id": 1234,
  "chars": "string",
  "yield_time_ms": 1000,
  "max_output_tokens": 10000
}
```

Behavior details to copy from Codex:

- `yield_time_ms` is not the process timeout. It controls when the tool returns.
- Pipe mode is default and closes stdin. Models should request `tty=true` for interactive programs.
- `write_stdin` should only work for live interactive sessions, except Ctrl-C for non-PTY cancellation.
- Return process/session id when still running.
- Allow empty `write_stdin` as a poll with a longer minimum wait.
- Keep process output in a retained head/tail buffer, not tail-only.
- Use process groups and kill descendants on timeout/session cleanup.
- Cap live processes, for example 64 total, and evict old exited processes first.
- Protect the most recent running processes from eviction.
- Emit model-readable failures. Sandbox denial should be a tool result the model can learn from, not only a runtime error.

Recommended response text should be stable:

```text
Chunk ID: abc123
Wall time: 2.1450 seconds
Process running with session ID 1204
Original token count: 18421
Output:
...
```

For Hivy JSON, keep the same fields:

```json
{
  "chunk_id": "abc123",
  "session_id": 1204,
  "running": true,
  "exit_code": null,
  "wall_time_seconds": 2.145,
  "original_token_count": 18421,
  "truncated": true,
  "output": "..."
}
```

### Output Buffering

Codex has a strong pattern:

- retain up to 1 MiB in a head/tail buffer
- stream UI deltas separately from model-facing output
- cap deltas and preserve UTF-8 boundaries
- wait briefly after process exit for trailing output
- truncate model output by tokens, not only bytes

Recommended Hivy defaults:

- retained transcript: 1 MiB head/tail
- model output default: 10k tokens
- UI delta max: 8 KB to 32 KB
- immediate command yield default: 10 seconds
- min non-empty stdin wait: 250 ms
- min empty poll wait: 5 seconds
- max initial yield: 30 seconds
- background process hard timeout: agent/tool policy, not `yield_time_ms`
- post-exit trailing output grace: 50 ms to 100 ms

### Shell Environment

Codex forces non-interactive, model-friendly defaults. Hivy should do the same:

- `NO_COLOR=1`
- `TERM=dumb`
- `PAGER=cat`
- `GIT_PAGER=cat`
- `CLICOLOR=0`
- stable `LANG`/`LC_ALL`
- `HIVY_CI=1`
- `HIVY_SESSION_ID`
- `HIVY_TURN_ID`

Preserve only approved env vars by default. Avoid leaking all process env into tools.

### `request_permissions`

If Hivy has no approval UI, do not fake Codex's nuanced approval flow. Add a simple policy enum:

- `deny`
- `sandbox`
- `allow_workspace`
- `allow_all`

If there is an approval UI, key approvals by:

- environment id
- canonical command
- cwd
- tty
- requested network
- requested read/write roots
- tool id

For file writes, key approvals per path and include move destinations.

### `view_image`

Hivy currently tells `read_file` callers that image files are not inlined and relies on multimodal forwarding when available.

Add `view_image`:

- path input
- detail level: `low`, `high`, `original` when model supports it
- sandboxed read
- MIME validation
- size cap and resizing
- model output as image content item when supported
- fallback text description when not supported

For non-vision open-weight models, wire the existing Go image-description path into Rust turn ingestion so images become bounded textual evidence instead of being skipped.

### `update_plan`

Hivy already has `update_plan` and validates duplicate `in_progress`. Keep it.

Codex lesson:

- plan is UI/runtime state, not sacred reasoning state
- return only a short acknowledgement to the model
- enforce at most one in-progress item
- reject empty steps
- do not over-inject plans into model context

### `get_context_remaining` And `new_context`

Add Codex-style context tools:

- `get_context_remaining`: return estimated tokens used, remaining, model window, effective threshold, and next compaction threshold.
- `new_context`: explicitly start a fresh context window without trying to carry everything forward.

Open-weight models often need visibility into context pressure. This prevents them from reading huge files or dumping logs near the limit.

### `tool_search`

Add a BM25 or simple lexical search over deferred tools:

Input:

```json
{"query": "browser screenshot local app", "limit": 8}
```

Output:

- matching tool names
- short descriptions
- schemas or load tokens for the next request

Use it for:

- MCP tools
- skills
- browser tools
- app-specific tools
- rare admin tools
- hosted tools

For weak models, `tool_search` is better than showing 40 tools directly.

### MCP Tools

Hivy already connects MCP servers and prefixes tools as `{server}_{raw}`.

Improvements:

- Add direct/deferred exposure for MCP tools.
- Preserve raw MCP identity internally while presenting sanitized stable model names.
- Include server name and raw name in telemetry.
- Respect MCP `readOnlyHint` or a Hivy-side allowlist to decide parallel safety.
- Cap MCP output before history insertion.
- Add MCP resource tools if useful: `list_mcp_resources`, `list_mcp_resource_templates`, `read_mcp_resource`.
- Consider exposing MCP resources as context sources before letting the model call arbitrary external tools.

### Skills

Hivy's skills system is already aligned with Codex's progressive disclosure:

- `skills_list` returns cheap metadata.
- `skill_view` loads full skill content.

Improvements:

- Make skills discoverable through `tool_search` or a unified discovery index.
- Add token budgets for skill summaries and full skill bodies.
- Track which skills were viewed in the turn for telemetry and evals.
- Do not load linked files automatically unless a skill explicitly instructs it and the model requests them.

### Subagents

Hivy has background subagent tasks, but the tool contract is too thin.

Current tool asks for:

- `agent`
- `goal`

Codex-class contract should add:

- task title
- token budget
- wall time budget
- max depth
- allowed tools
- result schema
- artifact expectation
- context propagation policy
- media propagation policy
- join mode: wait first, wait all, poll status
- cancellation
- explicit result consumption

Recommended tool family:

- `spawn_agent`
- `send_agent_input`
- `wait_agent`
- `close_agent`
- `list_agents`

If keeping one `subagent_task` tool, expand schema:

```json
{
  "agent": "explorer",
  "goal": "...",
  "context": "bounded explicit context to pass",
  "max_tokens": 20000,
  "timeout_seconds": 600,
  "expected_output": "findings with file references",
  "allow_tools": ["read_file", "search"],
  "propagate_attachments": false
}
```

For open-weight models, avoid recursive delegation by default. Parent sessions can delegate; subagents should not get subagent tools unless explicitly configured.

### Hosted Web, Browser, And Image Generation

Codex gates hosted `web_search` and `image_generation` by provider/model features. Hivy should do the same.

Do not expose these tools directly unless:

- selected model or provider supports them
- user policy allows network
- task needs them
- schema budget allows them

Browser tools should be deferred and loaded only when the task is UI/browser testing.

### Code Mode

Codex has code-mode tools for nested tool calls from JavaScript. Hivy does not need this first.

Recommendation:

- Do not implement code mode until the basic harness is strong.
- If added later, restrict it to strong models and local trusted tasks.
- Nested tool calls must route through the same registry and approval system, not bypass it.

## Context And Turn Loop Improvements

### Append-Only History

Codex normal history is append-only. It only replaces history during compaction, rollback, or sanitization.

Hivy should preserve this invariant:

- assistant tool-call item is recorded
- matching tool result is recorded
- missing tool outputs are synthesized as aborted/error outputs before the next request
- orphan tool results are removed or repaired before request
- unsupported images are stripped or summarized before request

This prevents provider 400s and context drift.

### Normalize Before Every Request

Before sending to any provider:

- ensure every assistant tool call has a tool result
- ensure every tool result has a previous assistant tool call
- strip or summarize unsupported modalities
- cap all tool outputs
- cap dynamic context and memory fragments
- preserve cacheable system prompt prefix
- sort tools stably
- apply provider-specific request dialect

### Context Budgeting

Hivy should stop using a static 180k budget for all models.

Use:

```text
effective_window = model.context_window_tokens * model.effective_context_window_percent
reserved_output = model.max_output_tokens
reserved_tool_slack = profile.tool_slack_tokens
compaction_threshold = effective_window - reserved_output - reserved_tool_slack
```

Suggested effective percentages:

- strong long-context hosted models: 0.85
- strong open-weight models: 0.75
- weaker tool-call models: 0.60 to 0.70
- models with unreliable context adherence: 0.50

Add context-length recovery:

1. If provider returns context length error, classify it as recoverable once.
2. Aggressively compact or drop old tool outputs.
3. Retry the same request once.
4. If still failing, return a model-readable failure and ask for narrowing.

### Typed Context Fragments

Codex requires contextual injections to be typed fragments with hard caps.

Hivy should make these typed:

- dynamic context
- memory context
- skill catalog
- MCP tools list
- image descriptions
- preloaded files
- environment/policy state
- subagent results

Each fragment should have:

- role
- marker/wrapper
- token/byte cap
- rendering method
- stable identity for cache/diffing

Avoid unbounded strings in system prompts.

### Tool Output Truncation

Hivy currently truncates individual read/bash outputs, but history-level truncation should also exist.

Add a final history insertion cap:

- cap any single tool result to profile max, for example 8k to 16k tokens
- keep metadata outside truncation
- record original size
- use head/tail for command output
- use start/range metadata for file reads
- for diffs, cap by file and hunk count

## Open-Weight Model Optimizations

### Tool Surface Modes

Use model profiles to choose mode:

1. `NativeJsonStrict`: strong tool models. Use function tools, JSON schema, direct plus deferred tools.
2. `NativeJsonLoose`: common open-weight mode. Use simple schemas, repair JSON, small direct tool set, no parallel by default.
3. `XmlToolFallback`: models that emit XML/text tools better than native tool calls. Use XML repair and concise tool instructions.
4. `ShellAndPatchMinimal`: weak coding models. Direct tools only: search, read, patch, exec, plan.

### Simple Names Beat Fancy Names

For weak models, prefer:

- `read_file`
- `search`
- `list_files`
- `apply_patch`
- `exec_command`
- `write_stdin`

Avoid:

- deeply nested namespaces
- many optional fields
- ambiguous booleans
- overlapping tools that do the same thing

If using namespaced tools internally, present simple aliases externally.

### Disable Parallel Calls Unless Proven

Hivy currently sets `parallel_tool_calls: true` whenever tools exist.

Change this to:

- profile default false for open-weight models
- true only for models that pass evals
- per-tool parallel safety enforced by runtime lock
- write tools exclusive
- shell sessions exclusive per process but read/search can parallelize

Open-weight models often issue parallel read/edit combinations that race or miss context.

### Repair Must Be Explicit

Current JSON repair returns `{}` on failure. That can turn a malformed destructive call into a missing-argument path, but it loses the evidence.

Change repair result to:

```rust
pub enum ToolArgParseResult {
    Valid(serde_json::Value),
    Repaired { value: serde_json::Value, raw: String, repair_kind: RepairKind },
    Failed { raw: String, error: String },
}
```

On failure:

- do not call the tool
- send a bounded tool error back to the model
- include tool name, missing required args, and a short raw snippet
- increment repair-failure telemetry
- after N failures, switch tool surface mode or stop

Do not silently filter empty tool names without recording a model-visible or telemetry event.

### Repeat Detection Should Become Semantic

Current exact repeat detection is useful but shallow.

Add semantic loop guards:

- repeated reads of same file/range
- repeated searches with equivalent query
- repeated shell commands after success
- repeated edit failures on same file and same old text
- repeated `ls/find/rg` broad scans after truncation
- repeated missing-argument calls
- repeated empty responses or thinking-only turns

The guard should produce model-readable feedback with a suggested different action.

### Prompt Rules Should Be Universal

Do not maintain per-model prompt variants. Use one universal tool discipline:

- inspect before edit
- search before guessing paths
- read exact files before patching
- prefer patch over full overwrite
- use bounded commands
- do not repeat successful commands
- when output is truncated, narrow the query
- when a tool errors, change approach or explain blocker
- keep subagent goals narrow and bounded

Model differences should be runtime policy, not prompt forks.

## Security And Sandbox Strategy

### Path Policy

Current `resolve_read_path` allows arbitrary absolute reads. Current writable roots include workspace, `/tmp`, `/var/tmp`, and `$HOME`.

For agentic coding defaults:

- reads: workspace plus explicit read roots
- writes: workspace only by default
- temp writes: explicit `temp` capability
- home writes: explicit capability
- deny globs always apply after canonicalization
- symlinks resolved before policy checks where possible
- include denied-read roots in sandbox policy so escalation cannot bypass them silently

### Command Policy

Do not rely on substring deny patterns as the safety boundary.

Keep deny patterns as quick rejection, but add:

- sandbox mode
- network mode
- filesystem read/write policy
- approval policy
- command telemetry/classification
- process isolation and cleanup

Command classification should be UI/telemetry only. Safety should come from execution boundaries.

### Sandbox Denial Handling

Codex can run first in sandbox, detect denial, ask or retry with adjusted permissions. If Hivy lacks approval UI, use deterministic policy:

- `sandbox`: fail with model-readable sandbox denial
- `allow_workspace`: retry only if denial is within workspace write policy
- `allow_all`: execute unsandboxed only for trusted configs

Never pretend a command had permission when it did not.

## Implementation Plan

### Phase 0: Measure

Add evals before changing behavior:

- tool-call parse success rate
- malformed JSON repair rate
- empty tool name rate
- missing required args rate
- repeated tool call rate
- context length failures
- read/search/edit sequence success
- compile/test pass rate on coding tasks
- shell timeout/runaway rate
- average tool schema tokens
- average tool output tokens

Create a small suite for:

- read a file and answer a question
- search for symbol and edit one call site
- multi-file patch
- run tests and fix failure
- long command that needs polling
- malformed XML/JSON tool call recovery
- large output truncation
- context overflow recovery

### Phase 1: RuntimeModelProfile

Wire Go registry limits into Rust:

- add profile to compiled agent definition
- derive from `internal/registry` model fields
- pass context window, output cap, reasoning support, modalities, open-weight flag
- add provider dialect and tool mode defaults
- replace static `input_token_budget` and `output_token_budget` defaults when model metadata exists

Affected files:

- `internal/agentruntime/compile_defaults.go`
- `sandboxes/runtime/crates/domain/src/model_config.rs`
- `sandboxes/runtime/crates/agent/src/request_builder.rs`
- `sandboxes/runtime/crates/agent/src/model_client.rs`
- `sandboxes/runtime/crates/agent/src/compaction.rs`

### Phase 2: Tool Router And Exposure

Add:

- tool id
- exposure mode
- parallel safety
- output contract
- hidden compatibility names
- deferred `tool_search`

Keep existing `JsonTool` as an adapter while migrating.

Affected files:

- `sandboxes/runtime/crates/tools/src/lib.rs`
- `sandboxes/runtime/crates/agent/src/runner.rs`
- `sandboxes/runtime/crates/agent/src/rig_tool_registry.rs`
- `sandboxes/runtime/crates/mcp/src/lib.rs`

### Phase 3: Search And File Listing

Add:

- `search`
- `list_files`
- optional `stat_file`

Make them direct for coding agents and parallel-safe.

Affected files:

- `sandboxes/runtime/crates/domain/src/tool_specs.rs`
- new `sandboxes/runtime/crates/tools/src/search.rs`
- new `sandboxes/runtime/crates/tools/src/list.rs`
- `sandboxes/runtime/crates/tools/src/lib.rs`

### Phase 4: Patch Tool

Add `apply_patch` as the preferred edit path.

Implementation pieces:

- parser
- verifier
- path resolver
- sandbox/write policy integration
- atomic writer
- committed delta model
- exact/inexact flag
- diff output
- tests for add/update/delete/move/conflict/partial failure

Affected files:

- new `sandboxes/runtime/crates/tools/src/apply_patch.rs`
- new `sandboxes/runtime/crates/tools/src/patch_parser.rs`
- `sandboxes/runtime/crates/tools/src/diff.rs`
- `sandboxes/runtime/crates/tools/src/operations.rs`
- `sandboxes/runtime/crates/domain/src/tool_specs.rs`

### Phase 5: Unified Exec

Add `exec_command`, `write_stdin`, and `terminate_process`. Keep `bash` as a hidden compatibility alias.

Implementation pieces:

- process ids
- live process registry with cap
- PTY support
- pipe support
- stdin writes
- head/tail transcript
- model output truncation by tokens
- process group cleanup
- stable environment
- yield time and poll behavior

Affected files:

- `sandboxes/runtime/crates/tools/src/bash.rs`
- `sandboxes/runtime/crates/tools/src/process_registry.rs`
- `sandboxes/runtime/crates/tools/src/operations.rs`
- new `sandboxes/runtime/crates/tools/src/exec.rs`
- new `sandboxes/runtime/crates/tools/src/head_tail_buffer.rs`

### Phase 6: Context Manager

Move history/context logic out of the large runner into a focused module.

Add:

- model-profile token budget
- pre-request normalization
- tool output truncation on insertion
- missing tool output repair
- orphan tool result repair
- modality stripping/summarization
- aggressive context-length retry
- typed context fragments

Affected files:

- `sandboxes/runtime/crates/agent/src/runner.rs`
- `sandboxes/runtime/crates/agent/src/compaction.rs`
- new `sandboxes/runtime/crates/agent/src/context_manager.rs`
- new `sandboxes/runtime/crates/agent/src/context_fragments.rs`

### Phase 7: Repair Semantics

Replace silent repairs with explicit parse outcomes.

Add:

- malformed-call event
- bounded raw snippet
- tool-call repair telemetry
- failed repair model feedback
- semantic repeat detection
- per-model repair policy

Affected files:

- `sandboxes/runtime/crates/safety/src/json_repair.rs`
- `sandboxes/runtime/crates/safety/src/xml_tool_repair.rs`
- `sandboxes/runtime/crates/safety/src/repeat_detector.rs`
- `sandboxes/runtime/crates/agent/src/model_client.rs`
- `sandboxes/runtime/crates/agent/src/runner.rs`

### Phase 8: Subagent Contracts

Expand subagent task schema and runtime state:

- budgets
- cancellation
- structured result
- context/media propagation
- max depth
- explicit join/wait tools
- result consumption marker

Affected files:

- `sandboxes/runtime/crates/domain/src/subagent.rs`
- `sandboxes/runtime/crates/agent/src/rig_tool_registry.rs`
- `sandboxes/runtime/crates/runtime/src/subagent_worker.rs`

## Priority Checklist

P0:

- Compile `RuntimeModelProfile` from registry metadata and use it in Rust.
- Disable `parallel_tool_calls` by default for open-weight models unless profile says safe.
- Add first-class `search` and `list_files`.
- Add context preflight budgeting and context-length retry after compaction.
- Make unrecoverable JSON/tool-call repair explicit instead of silently `{}` or drop.
- Tighten default read/write roots.

P1:

- Add `apply_patch` and demote full-file overwrites.
- Replace model-facing `bash` with `exec_command`/`write_stdin`.
- Add head/tail output buffering and token-based output caps.
- Add deferred `tool_search` and hide most MCP tools by default.
- Add version hashes to `read_file`, `write_file`, and `edit_file`.
- Add semantic repeat detection.

P2:

- Add `view_image` with non-vision fallback descriptions.
- Add MCP resource tools.
- Expand subagent contracts.
- Split `runner.rs`, `model_client.rs`, and large runtime modules around request loop, context, tool execution, stream parsing, and subagents.
- Add code-mode only after the basic harness is stable.

## What Not To Copy Blindly

- Codex's Responses API details are provider-specific. Hivy can keep chat-completions but needs dialect adapters.
- Codex often uses shell for reads/search because its environment includes `rg` and a powerful shell. Hivy should give open-weight models structured `search` and `list_files`.
- Codex's nuanced approval UI only helps if Hivy has a real approval surface. Otherwise use simple deterministic policy.
- Do not expose all Codex-style tools directly. Weak models need fewer direct tools, not more.
- Do not copy app-server destructive filesystem defaults into model-facing tools. Model-facing delete/remove should be conservative and approval-gated.

## Target End State

The ideal Hivy harness has these properties:

- Model profiles drive every request and tool-surface decision.
- The direct tool set is small, stable, and high-signal.
- Rare tools are discoverable through `tool_search`.
- File discovery uses `search` and `list_files`, not unbounded shell.
- File mutation uses `apply_patch` with verification and committed deltas.
- Shell execution is session-based, pollable, cancellable, bounded, and model-readable.
- Tool outputs are separately optimized for model context, telemetry, and UI events.
- History is append-only, normalized before every request, and compacted before overflow.
- Every injected context fragment has a type and hard cap.
- Repair failures are explicit and teach the model what to fix.
- Parallel tools are gated by model profile and per-tool safety.
- Subagents have budgets and contracts.
- Evals continuously measure whether each open-weight model can read, search, patch, run tests, recover from malformed calls, and finish tasks without loops.

If Hivy implements only one thing from Codex first, implement the model-aware tool router with direct/deferred exposure. It unlocks most of the other improvements: smaller prompts, safer tools, better open-weight behavior, model-specific repair, and cleaner runtime observability.
