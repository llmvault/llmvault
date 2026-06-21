# Hivy Runtime Harness Improvements From OMO

This document distills the useful harness ideas from `oh-my-openagent` / OMO and maps them onto the Rust runtime in `sandboxes/runtime`. The target is not to copy OMO line-for-line. The target is to move as much correctness as possible out of prompts and into typed runtime services, so open-weight models can succeed even when they produce weak JSON, repeat tool calls, lose context, overthink, or do not follow OpenAI-compatible semantics exactly.

The core thesis: a great agent harness is a set of state machines, bounded tools, context packers, recovery supervisors, and evidence gates. Prompts should describe intent. The runtime should enforce behavior.

## Study Scope

OMO areas studied:

- Startup and lifecycle: `packages/omo-opencode/src/testing/create-plugin-module.ts`, `plugin-interface.ts`, `create-hooks.ts`, `plugin/event.ts`.
- Tool registry and every native/gated/team tool: `tools/`, `plugin/tool-registry*.ts`, `features/background-agent/`, `features/team-mode/`.
- Hashline editing: `packages/hashline-core/`, `hooks/hashline-read-enhancer/`, `tools/hashline-edit/`.
- LSP runtime: `packages/lsp-core/`, `packages/lsp-tools-mcp/`, `packages/lsp-daemon/`.
- Prompt dispatch gate: `packages/utils/src/prompt-async-gate*`.
- Model routing and fallback: `packages/model-core/`, `hooks/model-fallback/`, `hooks/runtime-fallback/`.
- Agents, categories, skills, rules, and keyword routing: `agents/`, `skills-loader-core/`, `rules-engine/`, `agents-md-core/`, `keyword-detector/`.
- Compaction, continuation, and todo preservation: `hooks/todo-continuation-enforcer/`, `hooks/compaction-context-injector/`, `hooks/compaction-todo-preserver/`.
- QA culture: root `AGENTS.md`, `opencode-qa`, `codex-qa`, evidence requirements.

Hivy areas studied:

- Domain config: `sandboxes/runtime/crates/domain/src/agent_definition.rs`, `model_config.rs`, `tool_specs.rs`, `subagent.rs`, `mcp_specs.rs`, `skill_specs.rs`.
- Agent loop: `sandboxes/runtime/crates/agent/src/runner.rs`, `model_client.rs`, `request_builder.rs`, `history.rs`, `compaction.rs`, `rig_tool_registry.rs`.
- Tools: `sandboxes/runtime/crates/tools/src/read.rs`, `write.rs`, `edit.rs`, `bash.rs`, `path.rs`, `process_registry.rs`.
- Runtime/session: `sandboxes/runtime/crates/runtime/src/handler.rs`, `session_coordinator.rs`, `subagent_worker.rs`, `wake_timer.rs`.
- Storage: `sandboxes/runtime/crates/storage/migrations/001_init.sql`, `storage/src/repos.rs`, SQLite write gateway.
- Safety: `sandboxes/runtime/crates/safety/src/json_repair.rs`, `xml_tool_repair.rs`, `repeat_detector.rs`, `thinking_guard.rs`.
- Skills, MCP, streaming, observability: `skills/src/lib.rs`, `mcp/src/lib.rs`, `api/src/session_stream.rs`, `outbound/`.

## Current Hivy Baseline

Hivy is not starting from zero. The current Rust runtime already has several important ingredients:

- A real crate split: `domain`, `agent`, `tools`, `runtime`, `storage`, `safety`, `skills`, `mcp`, `api`, `outbound`.
- Live config snapshots via `ConfigStore` using `ArcSwap`, so turns capture stable config and updates apply on the next turn.
- Durable SQLite sessions/events/idempotency/outbox/subagent tables.
- SSE stream replay with sequence numbers and trace/turn/run IDs.
- XML tool-call repair, JSON argument repair, thinking stripping, overthinking detection, repeated tool-call rejection, stream idle timeout, empty-response recovery, cutoff recovery, and provider fallback.
- Skills as first-class filesystem artifacts with catalog, view, manage, linked supporting files, and pinned skill protection.
- Subagents as first-class runtime work, not just prompt prose.
- Read-before-edit and exact unique replacement in the file edit tool.

The main open-weight problem is that the runtime still assumes too much OpenAI-compatible behavior:

- `ModelConfig` only has `OpenaiCompatible` with `base_url`, `model_id`, `api_key_env`, temperature, max tokens, reasoning effort, headers, and a single nested fallback (`domain/src/model_config.rs:109`).
- `request_builder.rs` always emits `stream: true`, `stream_options.include_usage`, OpenAI `tools`, `parallel_tool_calls: true`, `max_completion_tokens`, and `reasoning_effort`.
- `model_client.rs` expects Chat Completions SSE and `[DONE]`, then repairs around failures.
- `read_file` returns plain content, not stable edit anchors.
- `write_file` overwrites existing files directly. `WriteFileConfig.atomic` exists but is not implemented as a distinct atomic-write mode.
- `resolve_read_path` ignores `ReadFileConfig.allowed_roots`, while write path policy is stronger.
- `SessionCoordinator` has in-memory turn queueing and cancellation, but not a durable internal prompt reservation system.
- Context packing loads up to 1000 model-history events and uses char-count compaction, rather than a deterministic budgeted packer.
- Subagent tasks have only `queued`, `running`, `completed`, `failed`, with limited lease/retry metadata.
- MCP servers are shared registry entries, not optionally per-session isolated clients.

## Design Principles To Steal From OMO

1. Prefer runtime state over conversational memory.
   Durable tasks, todos, evidence, reservations, model attempts, and tool outputs should outlive a context window.

2. Prefer small explicit tools over permissive shell usage.
   The model should choose among bounded operations. The runtime should do path checks, output caps, locks, repair, retries, and state updates.

3. Treat internal prompt injection as a dangerous write.
   Any runtime-generated follow-up, subagent result, wake, monitor alert, fallback retry, or continuation must go through one dispatch gate.

4. Route work by category before selecting a model.
   Open-weight models need sharper contracts: planner, explorer, code-edit, test-fix, reviewer, visual, writing, quick, deep.

5. Late-load context.
   Skills, AGENTS.md, rules, README context, and MCP capabilities should be discovered cheaply and loaded only when relevant.

6. Preserve tool-call pairs.
   Message history must never retain assistant tool calls without matching tool results, or tool results without their call.

7. Make verification an artifact, not a statement.
   Evidence files, stream captures, command output, screenshots, HTTP dumps, and DB receipts should be first-class.

8. Bound every loop.
   Model retries, repeated tool calls, background tasks, subagents, monitor injections, continuations, and compactions need cooldowns, caps, and terminal states.

## P0 Roadmap

These are the highest-leverage upgrades for open-weight reliability.

1. Add model capability profiles and request/stream adapters.
2. Replace plain text editing with hashline-anchored file reads and edits.
3. Add a durable `PromptGate` on top of `SessionCoordinator`.
4. Build a deterministic context packer with explicit budgets and message repair.
5. Add first-class search/glob/LSP tools and a max-tool trimming policy.
6. Upgrade subagents into a real background job runtime with leases, concurrency, stale detection, and parent wake delivery.
7. Add an open-weight eval and evidence harness that proves the runtime survives weak model behavior.

## 1. Model Capability Profiles

Current Hivy model config is too thin for open-weight models. Add a typed capability layer in `domain/src/model_config.rs` and pass it through `agent/src/request_builder.rs` and `agent/src/model_client.rs`.

Add:

```rust
pub struct ModelCapabilities {
    pub provider_family: ProviderFamily,
    pub endpoint_protocol: EndpointProtocol,
    pub context_window_tokens: u32,
    pub default_output_tokens: u32,
    pub max_output_tokens: u32,
    pub tokenizer: Option<String>,
    pub chat_template: Option<ChatTemplateKind>,
    pub supports_native_tools: bool,
    pub supports_parallel_tool_calls: bool,
    pub supports_tool_choice: bool,
    pub supports_json_schema: bool,
    pub supports_response_format: bool,
    pub supports_reasoning_effort: bool,
    pub supports_temperature: bool,
    pub supports_top_p: bool,
    pub max_tokens_param: MaxTokensParam,
    pub stream_done_required: bool,
    pub usage_expected: bool,
    pub thinking_delta_fields: Vec<String>,
    pub tool_call_format: ToolCallFormat,
    pub malformed_tool_recovery: ToolRecoveryMode,
    pub first_token_timeout_secs: u64,
    pub stream_idle_timeout_secs: u64,
    pub health_cooldown_secs: u64,
}
```

Key enums:

- `ProviderFamily`: `OpenAi`, `OpenRouter`, `Vllm`, `Ollama`, `LlamaCpp`, `Tgi`, `Sglang`, `AnthropicCompat`, `GeminiCompat`, `Custom`.
- `EndpointProtocol`: `OpenAiChatCompletions`, `OpenAiResponses`, `OllamaChat`, `VllmChat`, `TextGenerationInference`, `CustomHttp`.
- `MaxTokensParam`: `MaxCompletionTokens`, `MaxTokens`, `NumPredict`, `MaxNewTokens`, `None`.
- `ToolCallFormat`: `NativeOpenAi`, `JsonObjectInText`, `XmlToolCall`, `ReActText`, `NoTools`.
- `ToolRecoveryMode`: `Strict`, `RepairJson`, `ExtractXml`, `PromptRetry`, `SingleToolOnly`.

Runtime behavior:

- `request_builder` must not always send `parallel_tool_calls: true`. Only send it when `supports_parallel_tool_calls`.
- Only send `reasoning_effort` when supported.
- Choose the correct max-token parameter per backend.
- Disable unsupported `temperature` and `top_p` rather than hoping the backend ignores them.
- For weak local models, optionally force `parallel_tool_calls = false` and cap the tool list.
- For models without native tool calling, render a strict tool-call envelope in the prompt and parse it in `model_client`.
- Treat no usage as normal when `usage_expected = false`.
- Treat missing `[DONE]` as either fatal or recoverable based on `stream_done_required`.
- Read thinking fields from the capability profile, not a hardcoded list.

Add a `ModelAttempt` record:

```rust
pub struct ModelAttempt {
    pub provider: String,
    pub model: String,
    pub capabilities_id: String,
    pub endpoint_url: String,
    pub sampling: SamplingParams,
    pub fallback_rank: u32,
    pub reason: ModelAttemptReason,
}
```

Store attempts in memory during a turn and emit durable run events:

- `model_attempt_started`
- `model_attempt_failed`
- `model_attempt_fallback_selected`
- `model_attempt_succeeded`
- `model_capability_param_stripped`
- `model_stream_protocol_deviation`

Why this matters: open-weight endpoints differ in the exact parameters they accept, whether they stream usage, whether they send `[DONE]`, how they express reasoning, and whether native tool calling is actually usable. OMO normalizes model settings before dispatch; Hivy should make that a core Rust contract.

## 2. Proactive And Reactive Model Recovery

Keep two separate systems:

- Proactive routing: choose the right model and settings before the request.
- Reactive recovery: respond to request errors, stream failures, context overflow, malformed tool calls, thinking-only loops, empty responses, and repeated invalid tool calls.

Hivy already has some reactive recovery in `runner.rs`. Split it into `agent/src/recovery.rs` so it becomes testable and profile-aware.

Add recovery classes:

- `HttpRateLimit`
- `HttpOverloaded`
- `HttpBilling`
- `HttpAuth`
- `ContextLength`
- `InvalidRequest`
- `TransportTimeout`
- `HeaderTimeout`
- `StreamIdleTimeout`
- `StreamEndedWithoutDone`
- `NoEvents`
- `ThinkingOnly`
- `EmptyAssistant`
- `CutOff`
- `MalformedToolCall`
- `RepeatedToolCall`
- `ToolSchemaViolation`
- `BackendColdStart`
- `BackendGpuOom`

Recovery policy:

- Retry transport/rate/server/overload with bounded backoff.
- Fall back to the next `ModelAttempt` for overload, timeout, cold start, and GPU OOM.
- Do not fall back on malformed harness request until the request is normalized.
- On context overflow, compact/repack and retry once before falling back.
- On malformed tool output, repair once, then feed a structured correction message.
- On repeated bad tool call, return a tool result explaining the repeated signature and force a different action.
- On thinking-only responses, retry without injecting a new message for the first attempts; only inject a hard stop after repeated failures.
- Preserve raw provider error payloads as durable evidence, but redact secrets.

The current `JsonRepair` returns `{}` on unrecoverable malformed input. Replace that with:

```rust
pub enum RepairOutcome {
    Unchanged(Value),
    Repaired { value: Value, repairs: Vec<RepairKind>, raw: String },
    Failed { raw: String, error: String },
}
```

Do not silently turn impossible JSON into `{}`. That hides the model failure and creates generic missing-argument errors.

## 3. Hashline File Reads And Edits

This is the highest-value tool improvement.

Current Hivy:

- `read_file` returns plain content with truncation metadata.
- `edit_file` requires prior read and exact unique `old_text`.
- This helps, but open-weight models still hallucinate snippets, lose whitespace, and replace the wrong repeated block.

OMO's hashline pattern:

- Every read line is returned as `LINE#ID|content`.
- `ID` is a short hash derived from normalized line content plus line number.
- Edit operations must cite the original line/hash anchors.
- The edit engine validates hashes against the current file.
- Stale anchors produce a useful mismatch response with updated nearby anchors.
- Edits are sorted bottom-up, deduped, overlap-checked, and applied to the original snapshot.

Add a `HashlineFs` layer in `tools`:

```rust
pub struct LineAnchor {
    pub line: u32,
    pub hash: String,
    pub content: String,
}

pub enum HashlineEditOp {
    Replace { start: LineAnchorRef, end: Option<LineAnchorRef>, lines: Vec<String> },
    Append { after: LineAnchorRef, lines: Vec<String> },
    Prepend { before: LineAnchorRef, lines: Vec<String> },
}
```

New `read_file` output:

```json
{
  "path": "/workspace/src/main.rs",
  "content": "1#AB|fn main() {\\n2#KC|    println!(\"hi\");\\n3#DF|}",
  "anchor_format": "LINE#HASH|content",
  "truncated": false,
  "total_lines": 3
}
```

New `edit_file` arguments:

```json
{
  "path": "src/main.rs",
  "edits": [
    {
      "op": "replace",
      "start": "2#KC",
      "end": "2#KC",
      "lines": ["    println!(\"hello\");"]
    }
  ]
}
```

Implementation details:

- Preserve UTF-8 BOM and original CRLF/LF.
- Reject overlapping edits.
- Apply bottom-up.
- Treat identical duplicate edits as one edit and report `deduped_edits`.
- Reject no-op edits unless explicitly allowed.
- Return unified diff, bytes written, anchors consumed, and updated anchors around changed ranges.
- For create-new-file, allow unanchored create only if the target does not exist.
- For existing files, `write_file` should refuse overwrite unless `overwrite_existing=true` and the file was read in the same session. Prefer `edit_file`.
- Invalidate read permissions/anchors for other sessions when a file is written.
- Keep a per-session read-anchor cache keyed by canonical path and file mtime/content hash.

This changes the file editing problem from "model remembers exact strings" to "model references stable runtime facts".

## 4. Tool Registry And Tool Surface

OMO's tool registry builds always-on tools, gated tools, team tools, and MCP tools, then trims low-priority tools if a provider has a tool-count cap. Hivy should add the same policy around `ToolSpec`.

Add a `ToolRegistry` service:

- Registers factories by tool name.
- Applies config gates.
- Applies role/agent allowlists and denylists.
- Normalizes JSON schemas.
- Sorts tools deterministically.
- Trims low-priority tools when `ModelCapabilities.max_tool_count` is set.
- Emits `tool_registry_built` with tool count, trimmed tools, and role.

Suggested tool priorities:

1. Keep: `read_file`, `edit_file`, `bash`, `update_plan`, `request_user_input`.
2. Keep for coding roles: `grep`, `glob`, LSP diagnostics/navigation.
3. Keep for autonomy: `subagent_task`, `wake`.
4. Optional: skills, MCP, session search.
5. Trim first: visual, monitor, team admin, long-tail MCPs.

### Tool Inventory To Add Or Upgrade

| OMO tool family | Hivy equivalent | Recommendation |
| --- | --- | --- |
| `grep` | missing | Add `grep` backed by `rg`, with modes `content`, `files_with_matches`, `count`, include glob, path, head limit, timeout, and truncation metadata. |
| `glob` | missing | Add `glob` backed by `globwalk` or `ignore`, sorted by mtime, capped to 100 results, with path root. |
| `read_file` | exists | Add hashline anchors and enforce `allowed_roots`. |
| `edit` hashline | `edit_file` exists | Replace exact string edits with hashline edits. Keep exact-string patch as a fallback only. |
| `write_file` | exists | Split into `create_file` and guarded overwrite. Honor `atomic`. Refuse blind overwrite by default. |
| `bash` | exists | Convert sandbox string to enum, enforce env policy, add shell-read guard, stronger deny policy, unique process IDs. |
| `check_bash_status` | exists | Add output cursor, since repeated full output encourages loops. |
| `session_list/read/search/info` | partial `search_sessions` | Add session transcript tools with bounded slices and todo/task context. |
| `background_output/cancel` | partial process registry | Add generic job runtime with output cursors and cancellation for bash, subagents, monitors, and future jobs. |
| `task` delegation | `subagent_task` exists | Add categories, sync/background modes, model fallback chains, skills, max depth, and verification contracts. |
| `call_omo_agent` | missing | Add narrow direct-consultant tool for read-only explorer/reviewer agents. |
| `skill` | `skills_list/view/manage` exists | Keep catalog-first. Add triggers, restrictions, and skill capability summaries. |
| `skill_mcp` | missing | Add skill-embedded MCP clients keyed per session/skill/server. |
| `look_at` | partial multimodal attachments | Add explicit visual inspection tool with recursion disabled and MIME sniffing. |
| `interactive_bash` | missing | Optional PTY/tmux tool for long interactive processes. Keep state outside tmux. |
| `monitor_*` | missing | Add bounded monitor jobs that inject filtered output only through `PromptGate`. |
| `task_create/get/list/update` | `update_plan` exists | Add durable task list separate from visible plan. |
| `team_*` | missing | Add `TeamRuntime` later, backed by SQLite tables. |
| `lsp_*` | missing | Add LSP daemon or runtime LSP manager for diagnostics, definitions, references, symbols, rename. |

### Bash Hardening

Current `BashConfig.sandbox` is a string and not meaningfully enforced. Replace it with:

```rust
pub enum BashSandbox {
    ProcessIsolated,
    WorkspaceOnly,
    NoNetwork,
    Disabled,
}
```

Add:

- AST-ish command policy for obvious file reads. If the model runs `cat`, `head`, `tail`, `sed -n`, or `awk` for simple reads, warn or reject and ask it to use `read_file` so it gets anchors.
- Command allow/deny policy based on parsed first command where possible, not only substring matching.
- Process group kill for timeouts and cancellation.
- Unique process IDs with nanoid/UUID, not timestamp only.
- Output cursors: `check_bash_status(process_id, cursor)` returns only new lines plus `next_cursor`.
- Per-session process limit and global process limit.
- Redaction for env values in emitted command logs.

## 5. LSP As First-Class Runtime Tooling

OMO's LSP is a huge quality boost because it gives the model semantic facts without asking it to grep blindly.

Add tools:

- `lsp_status`
- `lsp_diagnostics`
- `lsp_goto_definition`
- `lsp_find_references`
- `lsp_symbols`
- `lsp_prepare_rename`
- `lsp_rename`
- `lsp_install_decision`

Recommended architecture:

- Start with an in-process `LspManager` crate if that is faster.
- Move to a daemon/proxy once startup cost becomes a problem.
- Key clients by workspace root and language server.
- Use ref counts and idle reaping.
- Bound initialization timeout.
- Apply workspace edits only inside allowed roots.
- Store install decisions so the model does not repeatedly ask for the same language server setup.

Open-weight impact: LSP tools reduce the amount of reasoning the model must do. Instead of inferring symbol relationships from text search, the harness returns definitions, references, and diagnostics.

## 6. PromptGate: Durable Internal Dispatch

OMO's most important lifecycle lesson is that internal prompt injection must be centralized. Hivy already has `SessionCoordinator` for in-memory session queueing. Keep it, but add `PromptGate` as the only API for runtime-generated follow-ups.

Routes that must use `PromptGate`:

- Subagent completion notifications.
- Wake timers.
- Future monitor output.
- Continuation prompts.
- Fallback retry prompts.
- Team mailbox live delivery.
- External reply correlation.
- Any system-generated "please continue" message.

Add table:

```sql
CREATE TABLE session_prompt_reservations (
    session_id TEXT PRIMARY KEY,
    source TEXT NOT NULL,
    semantic_key TEXT NOT NULL,
    mode TEXT NOT NULL,
    state TEXT NOT NULL,
    created_at TEXT NOT NULL,
    expires_at TEXT NOT NULL,
    payload_json TEXT NOT NULL
);

CREATE TABLE session_prompt_dedupe (
    session_id TEXT NOT NULL,
    semantic_key TEXT NOT NULL,
    created_at TEXT NOT NULL,
    PRIMARY KEY (session_id, semantic_key)
);
```

API shape:

```rust
pub struct InternalPrompt {
    pub session_id: SessionId,
    pub source: InternalPromptSource,
    pub text: String,
    pub semantic_key: String,
    pub queue_behavior: QueueBehavior,
    pub mode: DispatchMode,
    pub post_dispatch_hold_ms: u64,
    pub timeout_ms: u64,
}

pub enum QueueBehavior {
    DeferUntilIdle,
    CoalesceBySemanticKey,
    DropIfBusy,
    ForceQueue,
}
```

Gate semantics:

- Reserve per session before dispatch.
- Reject or coalesce duplicate semantic keys for a hold window.
- Check active session state.
- Check latest assistant/tool state so tool-call pairs are not interrupted.
- If busy, queue/defer according to behavior.
- Dispatch via inbound channel only after the reservation is owned.
- Hold the reservation briefly after dispatch to collapse duplicate idle/error edges.
- Release on accepted terminal state, explicit abort, timeout, or failed enqueue.
- Persist enough state to recover after restart.

Add CI/static audit:

- Ban direct `inbound_sink.send(...)` from runtime modules except inside `PromptGate` and external inbound handlers.
- Ban ad hoc "system instruction" continuation injection outside `PromptGate`.
- Require explicit queue behavior.
- Require `post_dispatch_hold_ms > 0`.

## 7. Deterministic Context Packing

Current Hivy builds messages in `runner.rs`:

- Cacheable system prompt.
- Dynamic system prompt.
- Loaded model history with hardcoded limit 1000.
- Current user message and images.
- Optional char-estimate compaction.

For open-weight models, add a deterministic packer:

```rust
pub struct ContextPacker {
    pub token_counter: Arc<dyn TokenCounter>,
    pub budget: ContextBudget,
    pub rules_engine: Arc<RulesEngine>,
    pub skill_store: SkillStore,
}
```

Budget tiers:

1. Required system identity and safety.
2. Current user turn and attachments.
3. Pending tool-call/result pairs.
4. Recent messages.
5. Active plan/todos/tasks.
6. Relevant file rules and AGENTS.md blocks.
7. Loaded skills.
8. Memory entries.
9. Session search snippets.
10. Old summaries.

Packing rules:

- Always keep assistant tool calls and matching tool results together.
- Never keep orphaned tool results.
- If a tool result is too large, replace it with a structured summary and a pointer.
- Keep the last N real user turns protected.
- Deduplicate repeated rule/skill/context blocks.
- Use model-specific token counters where possible; fall back to conservative chars/token.
- Emit `context_pack_built` with tokens by tier, dropped tiers, and message count.
- Add tests that small context profiles drop low-priority context before dropping recent turns.

## 8. AGENTS.md, README, And Rules Injection

OMO injects local instructions at the point of file use. Hivy should do the same.

Add a `rules` or `context_rules` crate:

- Discover `AGENTS.md` up from the file path to workspace root.
- Discover local README files near the file.
- Support project/user rule dirs later, for example `.hivy/rules`, `.agents/rules`, `.cursor/rules`, `.github/instructions`.
- Support frontmatter: `alwaysApply`, `globs`, `paths`, `description`, `priority`.
- Cache injected paths per session.
- Clear cache on compaction and file delete.
- Path containment must be strict.
- Truncate rule output with explicit metadata.

Injection points:

- After `read_file`.
- Before `edit_file` or `write_file`.
- Before `bash` if command operates on a path.
- During context packing for currently active file set.

Do not dump all rules into the initial prompt. Open-weight models benefit from fewer, more relevant instructions.

## 9. Compaction, Todos, And Continuation

Hivy already has `compaction.rs`, but it should preserve runtime state, not only summarize text.

Add `CompactionCheckpoint`:

```rust
pub struct CompactionCheckpoint {
    pub session_id: SessionId,
    pub compaction_epoch: u64,
    pub model_attempt: ModelAttempt,
    pub available_tools: Vec<String>,
    pub active_plan: Option<UpdatePlanPayload>,
    pub active_tasks: Vec<TaskState>,
    pub active_subagents: Vec<String>,
    pub pending_questions: Vec<String>,
    pub loaded_skills: Vec<String>,
    pub recent_file_anchors: Vec<FileAnchorSnapshot>,
}
```

Before compaction:

- Capture model/tool config.
- Capture plan/todos/tasks.
- Capture active subagent and background job IDs.
- Capture loaded skills and relevant rules.
- Capture current file anchors if recent edits are pending.

After compaction:

- Restore checkpoint as a high-priority context block.
- Keep a compaction epoch in events so duplicate autocontinuations are ignored.
- Preserve plan state against bootstrapping overwrites.

Continuation policy:

- Only continue when incomplete work exists.
- Do not continue when a user question is pending.
- Do not continue while background jobs or subagents are active unless the continuation is specifically to check them.
- Cool down continuations per session.
- Cap continuation failures.
- Respect user stop/interruption.
- Dispatch via `PromptGate`.

## 10. Plans, Tasks, Evidence

Hivy has `update_plan`, but OMO shows that serious work needs separate durable task/evidence state.

Keep visible plan simple. Add durable task/evidence tables:

```sql
CREATE TABLE runtime_tasks (
    id TEXT PRIMARY KEY,
    session_id TEXT NOT NULL,
    title TEXT NOT NULL,
    details TEXT NOT NULL,
    state TEXT NOT NULL,
    owner TEXT,
    priority INTEGER NOT NULL DEFAULT 0,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    completed_at TEXT
);

CREATE TABLE runtime_evidence (
    id TEXT PRIMARY KEY,
    session_id TEXT NOT NULL,
    task_id TEXT,
    kind TEXT NOT NULL,
    path TEXT,
    command TEXT,
    summary TEXT NOT NULL,
    payload_json TEXT NOT NULL,
    created_at TEXT NOT NULL
);
```

Tools:

- `task_create`
- `task_list`
- `task_get`
- `task_update`
- `record_evidence`

`record_evidence` should validate that referenced artifact paths exist and are non-empty. For command evidence, capture command, cwd, exit code, stdout/stderr tail, and timestamp.

This is the runtime version of "do not trust the agent saying it tested it".

## 11. Delegation, Background Jobs, And Team Runtime

Current Hivy `subagent_task` is a useful base. Upgrade it in layers.

### DelegateRuntime

Add fields:

```json
{
  "goal": "...",
  "agent": "explorer",
  "category": "deep",
  "skills": ["rust-runtime"],
  "run_in_background": true,
  "success_criteria": ["..."],
  "files": ["..."],
  "parent_context": "..."
}
```

Rules:

- Exactly one of `agent` or `category` unless `agent` is explicitly selected.
- Categories define model fallback chain, allowed tools, max context, and verification expectations.
- Subagents have max depth to prevent recursive explosions.
- Parent must not accept subagent completion without evidence or a verifiable result.
- Child result should include summary, changed files, tests, blockers, and artifact paths.

### JobRuntime

Unify background bash, subagents, monitors, and future long jobs under one job runtime:

- `jobs`: id, kind, parent_session_id, child_session_id, provider/model key, state, attempt, lease, heartbeat, created/started/completed.
- `job_outputs`: cursor, content chunk, created_at.
- `job_wakeups`: parent session wake notifications with semantic dedupe keys.

Runtime policies:

- Per-provider and per-model concurrency.
- Queue by concurrency key.
- Stale no-progress timeout.
- Session-gone timeout.
- Max tool calls per child.
- Repeated tool signature circuit breaker.
- Cancel descendants when parent is cancelled.
- Parent wake notification via `PromptGate`.
- Output cursors to avoid repeated full transcript polling.

### TeamRuntime

Build after `DelegateRuntime` is reliable. OMO team mode should become SQLite-backed in Hivy.

Tables:

```sql
CREATE TABLE team_runs (...);
CREATE TABLE team_members (...);
CREATE TABLE team_mailbox_messages (...);
CREATE TABLE team_tasks (...);
CREATE TABLE team_worktrees (...);
```

State machines:

- `TeamRun`: `creating -> active -> shutdown_requested -> deleting -> deleted`; `creating -> failed`; live states can become `orphaned`.
- `TeamMember`: `pending -> running -> idle/running -> completed|errored|cancelled`.
- `TeamTask`: `pending -> claimed -> in_progress -> completed -> deleted`.
- `MailboxMessage`: `unread -> reserved -> processed`; `reserved -> unread` on timeout; optional `dead_letter`.

Rules:

- Validate agent eligibility at team creation.
- No nested teams.
- One worktree per mutating member.
- Atomic task claiming in SQL.
- Durable mailbox first, live injection second.
- Lease and heartbeat every worker.
- Reclaim expired leases at startup.

Open-weight impact: teams let weaker models handle smaller tasks with sharper roles while the harness coordinates state, not the conversation.

## 12. Agents, Roles, And Categories

OMO does not only create agent personas. It enforces roles with tool permissions.

Add `AgentRolePolicy`:

```rust
pub struct AgentRolePolicy {
    pub mode: AgentMode,
    pub allowed_tools: Vec<String>,
    pub denied_tools: Vec<String>,
    pub can_delegate: bool,
    pub can_write_files: bool,
    pub can_use_shell: bool,
    pub max_subagent_depth: u32,
    pub required_planner: Option<String>,
}
```

Suggested built-ins:

- `orchestrator`: main agent, can plan/delegate/edit/test.
- `explorer`: read-only, search/LSP/session tools, no writes.
- `planner`: plan/task tools, no writes.
- `reviewer`: read-only plus test/log inspection, no edits.
- `code_editor`: read/edit/bash/LSP, no delegation by default.
- `test_fixer`: read/edit/bash/LSP/test tools.
- `visual`: image inspection only, no delegation recursion.

Categories should be executable policy:

```rust
pub struct TaskCategory {
    pub name: String,
    pub description: String,
    pub model_chain: Vec<ModelSelector>,
    pub allowed_tools: Vec<String>,
    pub prompt_appendix: String,
    pub max_context_tokens: u32,
    pub verification_required: bool,
    pub evidence_required: bool,
}
```

Open-weight prompt variants:

- Maintain model-family prompt appendices for Llama, Qwen, GLM, Kimi, DeepSeek, MiniMax, Mistral, and generic OpenAI-compatible.
- Use shorter, more literal instructions for weaker models.
- Put exact output sections and stop conditions near the end.
- For models with bad native tools, force single JSON tool envelope.

## 13. Skills And Skill-Embedded MCP

Hivy's `SkillStore` is already a strong base:

- Config skills sync into `.skills`.
- `skills_list` exposes summaries.
- `skill_view` loads full `SKILL.md` or linked files.
- `skill_manage` can create/patch/edit/delete/write/remove.
- Supporting files are restricted to `references`, `templates`, `scripts`, `assets`.

Add:

- Trigger matching that strips code blocks and inline code before keyword detection.
- Per-agent skill restrictions.
- Skill load records in session history so context packer knows what is active.
- Skill readiness checks against runtime env, not process env.
- Skill-embedded MCP metadata in `SKILL.md` frontmatter.
- `skill_mcp` tool that executes exactly one operation: tool, resource, or prompt.
- Per-session key: `${session_id}:${skill_name}:${server_name}`.
- Idle cleanup and reconnect.
- Clean env allowlist for stdio MCP.
- HTTP header placeholder expansion.
- OAuth later if needed.

Do not put all skill content in the base prompt. Catalog first, load only when selected.

## 14. MCP Isolation And Health

Current `McpRegistry` connects shared stdio/HTTP servers and exposes prefixed tools. Improve it before using stateful MCPs heavily.

Add:

- Apply `startup_timeout_seconds` for stdio MCPs.
- Expand env placeholders for stdio env values, not only HTTP headers.
- Redact env values in logs.
- Health state per server: `connecting`, `ready`, `failed`, `restarting`, `disabled`.
- Durable or observable `mcp_server_failed` event on load failure.
- Optional per-session clients for stateful skill MCP.
- Tool-count caps and tool-filter validation before exposing tools to models.

## 15. Message History Repair

Before every model request, run a message preflight:

- Validate role order for the chosen provider.
- Validate assistant tool calls have matching tool result messages.
- Validate tool result IDs match known calls.
- If an old compaction dropped a pair, insert a synthetic tool result with a clear placeholder.
- If a tool result is retained without its assistant call, drop or summarize it.
- Convert unsupported content parts based on model capabilities.
- Enforce provider-specific system message constraints.

This should happen inside the context packer/request adapter, not scattered through `runner.rs`.

## 16. Observability And Canonical Events

Hivy stream observability is already good. Make recovery and harness decisions equally visible.

Persist canonical events for:

- Model attempt start/fail/success/fallback.
- Capability parameter stripping.
- Context pack built and compacted.
- Tool schema validation failure.
- JSON repair success/failure with raw length and repair kinds.
- XML tool-call extraction.
- Thinking-only retry.
- Empty response retry.
- Cutoff retry.
- Repeat tool rejection.
- PromptGate reserve/defer/dispatch/coalesce/drop/release.
- Subagent lease acquired/heartbeat/expired/reclaimed.
- Background job queued/running/stale/cancelled/completed.
- MCP server failed/ready/reloaded.

Keep high-volume token/thinking deltas out of durable storage or sample them. Persist decisions and terminal facts.

## 17. QA And Eval Harness

OMO's most important process lesson: unit tests are not enough for harness code. Hivy already has `make test-agent-sessions-e2e`, which is the right flagship. Add a runtime evidence harness specifically for open-weight behavior.

Create `sandboxes/runtime/scripts/runtime-qa` or an `eval-harness` command that:

- Creates isolated temp workspace and temp runtime home.
- Starts the Rust runtime or runtime Docker image.
- Uses fake OpenAI-compatible servers for deterministic failures.
- Optionally runs a real local model profile when configured.
- Opens the direct sandbox SSE stream.
- Captures all SSE events.
- Captures SQLite row counts before/after where relevant.
- Writes evidence under a predictable directory, for example `.hivy/evidence/<YYYYMMDD>-<slug>/`.

Eval cases:

- Model streams malformed JSON tool arguments.
- Model emits XML tool calls in text.
- Model emits thinking only.
- Model ends stream without `[DONE]`.
- Model sends no usage.
- Model repeats identical tool calls.
- Model asks for a tool that was trimmed.
- Model exceeds context window.
- Model returns assistant tool call without arguments.
- Backend returns 429/503/529.
- Backend stalls before headers.
- Backend stalls mid-stream.
- `read_file` then stale `edit_file` anchors.
- `write_file` blind overwrite attempt.
- Subagent completes after parent already idled.
- Wake fires after process restart.
- MCP startup hangs.
- Tool output exceeds cap.
- Small context model with loaded skills and long history.

Every harness change should have:

- Exact command.
- Exit code.
- SSE transcript.
- Relevant DB/event evidence.
- Why no regression.
- Proof intended behavior landed.

## 18. Concrete Rust Implementation Map

### `crates/domain`

Add:

- `model_capabilities.rs`
- `model_attempt.rs`
- `task_category.rs`
- `role_policy.rs`
- `prompt_gate.rs`
- `runtime_task.rs`
- `team.rs`
- Expanded `SubagentTaskState`.

Modify:

- `model_config.rs`: add capabilities/profile fields and fallback chains.
- `tool_specs.rs`: add grep/glob/LSP/hashline options, tool caps, role gates.
- `agent_definition.rs`: add category/role policies and validation.
- `mcp_specs.rs`: make startup timeout enforced, add env policy.

### `crates/agent`

Split `runner.rs` into:

- `turn_loop.rs`: high-level orchestration.
- `context_packer.rs`: deterministic budgeted packing.
- `request_adapter.rs`: provider/model request shaping.
- `stream_interpreter.rs`: profile-aware SSE/text parsing.
- `recovery.rs`: model and tool recovery decisions.
- `tool_dispatch.rs`: validation, execution, result shaping.
- `message_repair.rs`: tool-call pair repair.

Keep `runner.rs` as a thin coordinator.

### `crates/tools`

Add:

- `hashline.rs`
- `grep.rs`
- `glob.rs`
- `lsp.rs` or LSP MCP bridge
- `policy.rs`
- `tool_registry.rs`

Modify:

- `read.rs`: enforce allowed roots and return anchors.
- `edit.rs`: accept hashline edits.
- `write.rs`: split create/overwrite and honor atomic writes.
- `bash.rs`: sandbox enum, command policy, env policy.
- `process_registry.rs`: UUID IDs, cursors, cancellation, session/global caps.

### `crates/runtime`

Add:

- `prompt_gate.rs`
- `job_runtime.rs`
- `team_runtime.rs`
- `lease_reclaimer.rs`
- `runtime_qa.rs` or script entrypoint.

Modify:

- `session_coordinator.rs`: expose state needed by `PromptGate`, but keep turn queueing.
- `handler.rs`: route subagent/wake/internal messages through `PromptGate`.
- `subagent_worker.rs`: leases, heartbeats, attempts, cancellation, concurrency.
- `wake_timer.rs`: persist wakes instead of only spawning in-memory sleeps.

### `crates/storage`

Add migrations for:

- Prompt reservations and dedupe.
- Runtime tasks and evidence.
- Job attempts/output/leases.
- Wake jobs.
- Team runs/members/mailbox/tasks/worktrees.
- Model attempts or recovery events if not only event payloads.

Tighten:

- Subagent completion should update with `WHERE state IN ('running')` or a valid transition set.
- Add `attempt`, `leased_by`, `lease_expires_at`, `heartbeat_at` to subagent tasks.

### `crates/safety`

Add:

- `RepairOutcome`.
- `ToolCallParser` supporting native, JSON text, XML, and ReAct-like formats.
- Cross-turn repeat signature cache for long sessions.
- Model-family safety profiles.

Modify:

- `json_repair.rs`: stop returning `{}` as if repair succeeded.
- `repeat_detector.rs`: canonicalize JSON by sorted keys before hashing.

### `crates/mcp`

Add:

- Startup timeout.
- Session-scoped clients.
- Env placeholder expansion for stdio.
- Health reporting.
- Idle cleanup.

### `crates/eval-harness`

Use it. This crate should become the deterministic open-weight torture suite, not a placeholder.

## 19. Suggested Build Order

### Phase 1: Model Profiles And Harness Evidence

Acceptance:

- `ModelCapabilities` exists.
- `request_builder` strips unsupported params.
- Fake providers prove no usage, missing `[DONE]`, bad JSON, XML tools, stalls, and context overflow.
- Evidence command writes files under `.hivy/evidence`.

### Phase 2: Hashline Tools

Acceptance:

- `read_file` returns `LINE#HASH` anchors.
- `edit_file` rejects stale anchors and returns updated nearby anchors.
- Existing exact-string edit tests still pass through compatibility path or are migrated.
- Blind overwrite of existing files is blocked by default.

### Phase 3: Context Packer And Message Repair

Acceptance:

- Small context profile tests drop low-priority context deterministically.
- Tool-call/result pairs are never broken.
- Compaction checkpoint preserves plan/tasks/subagents/skills.
- `input_token_budget` and `max_history_events` are actually enforced.

### Phase 4: PromptGate

Acceptance:

- Subagent completion, wake, and future continuation route through one API.
- Duplicate subagent completion notifications coalesce.
- Active tool-call turns are not interrupted.
- Static audit blocks direct internal inbound sends.

### Phase 5: Search, Glob, LSP

Acceptance:

- `grep` and `glob` are bounded and truncation-aware.
- LSP diagnostics and goto definition work in at least Rust/TypeScript.
- Tool registry trims by model capability cap.

### Phase 6: Background Job Runtime

Acceptance:

- Subagents have leases/heartbeats/attempts.
- Parent wake uses `PromptGate`.
- Stale jobs are reclaimed.
- Per-model/provider concurrency is enforced.
- Output cursors prevent polling full logs.

### Phase 7: Teams

Acceptance:

- Team run creates members/tasks/mailbox records.
- Task claim is atomic.
- Mailbox reservation timeout works.
- Worktree per mutating member works.
- No nested teams.

## 20. What Not To Copy Blindly

- OMO uses filesystem JSON for some team state because it adapts to OpenCode. Hivy should use SQLite.
- OMO's OpenCode hook names are adapter-specific. Hivy should implement typed Rust pipeline phases instead.
- OMO has compatibility hacks for OpenCode/Codex. Do not import those unless the Hivy runtime has the same external constraint.
- Do not make a giant abstraction before the state machines are clear. Extract stable primitives after they work.
- Do not solve weak model behavior only with longer prompts. Use schemas, permissions, tool caps, validators, and durable state.

## Final Target Shape

The ideal Hivy harness should look like this:

- `AgentDefinition` declares roles, categories, tools, skills, MCPs, model chains, safety, and budgets.
- `ContextPacker` builds a model-specific prompt from durable state.
- `ModelRouter` selects a `ModelAttempt` using capability profiles and health.
- `RequestAdapter` emits the correct backend protocol.
- `StreamInterpreter` parses native or text tool calls and reasoning according to profile.
- `ToolDispatcher` validates schemas, permissions, paths, caps, and result envelopes.
- `HashlineFs`, `SearchService`, `LspService`, `JobRuntime`, `SkillRuntime`, and `McpRuntime` provide model-friendly capabilities.
- `PromptGate` owns every internal re-entry.
- `RecoverySupervisor` handles retries, fallback, malformed outputs, and loops.
- `CompactionSupervisor` preserves todo/task/model/tool state across small contexts.
- `EvidenceHarness` proves behavior on real runtime paths.

That is the difference between a chat loop with tools and a harness that can make open-weight models feel much stronger than they are.
