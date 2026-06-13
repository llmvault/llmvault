# Agent Runtime Migration Plan

**Status:** Companion plan, backend/runtime first
**Date:** 2026-06-13
**Scope:** Remove the `employee` and `specialist` concepts from the agent runtime stack before the wider v2 domain migration. Frontend work is explicitly out of scope.

---

## 1. Goal

The runtime should have exactly one actor concept: **agent**.

Today the control plane and sandbox runtime mix several concepts:

- Go control-plane runtime packages are named `employeeruntime`, `employeeprompts`, and `employeesandbox`.
- Runtime startup/env/token/webhook contracts still use `employee_id`, `HIVY_EMPLOYEE_ID`, `employee_proxy`, and `/internal/employees/...`.
- The Rust runtime config has `RuntimeMode::{Employee, Specialist}`, `specialist_profile`, `sub_agents`, delegate tools, and delegate scheduler paths.
- There are two runtime image variants: normal runtime and specialist runtime.

Target state:

- One runtime package vocabulary: `agent`.
- One runtime image.
- One warm-pool mode: `agent`.
- No specialist mode, specialist profile, specialist sandbox, or specialist runtime image. Subagents remain first-class, but they are agents, not specialists. Legacy delegate wording/plumbing should be renamed or replaced with subagent/agent-task terminology. The existing specialist Dockerfile is preserved by renaming it to `Dockerfile.developers` and treating it as a selectable developer workspace template image, not a runtime mode.
- Hivy is just an agent. It has access to all tools; its product behavior is prompt/policy: it may research, inspect, and write scripts to gather information, but it does not complete and deliver implementation work itself. Later v2 handoff moves work to another agent.

No backwards compatibility is required.

---

## 2. Runtime Contract

### 2.1 Agent definition JSON

The compiled runtime config should become:

```json
{
  "agent": { "name": "Hivy", "description": "..." },
  "system_prompt": {},
  "model": {},
  "multimodal_model": {},
  "limits": {},
  "context": {},
  "tools": [],
  "mcp_servers": [],
  "skills": [],
  "outbound_channels": [],
  "sub_agents": {},
  "safety": {}
}
```

Delete from the runtime schema:

- `mode`
- `specialist_profile`
- delegate-specific config fields

Keep normal tool config, MCP servers, skills, memory, outbound channels, subagents, safety settings, schedules/wake tools, and local session storage.

`sub_agents` must stay in `AgentDefinition`. A subagent is a nested agent definition or agent reference keyed by a stable local name. It must use the same agent-shaped contract as the parent: prompt, model, tools, MCP servers, skills, limits, safety, and optional further subagents. It must not use `specialist_profile`, runtime `mode`, specialist catalogs, or specialist image selection.

### 2.2 Environment variables

Rename the runtime env contract:

| Old | New |
|---|---|
| `HIVY_EMPLOYEE_ID` | `HIVY_AGENT_ID` |
| `HIVY_EMPLOYEE_SQLITE_BACKUP_MAX_BYTES` | `HIVY_AGENT_SQLITE_BACKUP_MAX_BYTES` |
| `HIVY_SANDBOX_WARM_POOL_EMPLOYEE_SIZE` | `HIVY_SANDBOX_WARM_POOL_AGENT_SIZE` |

Delete:

- `HIVY_RUNTIME_MODE`
- `HIVY_SANDBOXES_RUNTIME_SPECIALIST_IMAGE`
- `HIVY_SANDBOX_WARM_POOL_SPECIALIST_SIZE`

Keep model, proxy, org, sandbox, git, drive, integration, and Sentry env vars.

### 2.3 Control-plane callbacks

Rename callback paths and payload fields:

- `/internal/webhooks/employee/{sandboxID}` -> `/internal/webhooks/agent/{sandboxID}`
- `/internal/employees/{agentID}/sqlite-backup/*` -> `/internal/agents/{agentID}/sqlite-backup/*`
- `employee_id` -> `agent_id`
- `employee_session_id` -> `session_id`
- token meta `type=employee_proxy` -> `type=agent_proxy`
- token meta `runtime_mode` deleted

Runtime event names such as `agent.message.sent`, `agent.run.model.usage`, and `agent.tool.invoked` already use the right domain language and should stay.

---

## 3. Workstreams

### R1 - Go Runtime Rename

Mechanical rename:

- `internal/employeeruntime` -> `internal/agentruntime`
- `internal/employeeprompts` -> `internal/agentprompts`
- `internal/employeesandbox` -> `internal/agentsandbox`
- `EmployeeDefinition` -> `AgentDefinition`
- `EmployeeEnv*` -> `AgentEnv*`
- `DefaultEmployeeModel` -> `DefaultAgentModel`
- `TokenTypeEmployeeProxy` -> `TokenTypeAgentProxy`
- `TokenMetaEmployeeID` -> `TokenMetaAgentID`
- `TokenHarnessEmployeeSandbox` -> `TokenHarnessAgentSandbox`

The rename should compile before behavior changes where possible, but no alias layer is required.

### R2 - Delete Specialist Runtime Surface

Remove from Go runtime compilation and startup:

- `PrepareSpecialistStartup`
- `MintSpecialistProxyToken`
- `CompileSpecialist*`
- `specialist_model_config.go`
- `specialists_prompt.go`
- specialist catalog dependency from runtime compile deps
- specialist runtime token modes and token metadata
- specialist runtime image arguments in gateway/uploads/sandbox services

Hivy should no longer receive a specialist catalog or specialist launch tool. Future assignment is W10 `handoff_to_agent()`, and runtime-local subagent work should be expressed as subagent/agent-task execution, not as specialist delegation.

### R3 - Rust Runtime Schema Cleanup

In `sandboxes/runtime`:

- Delete `RuntimeMode` and `SpecialistProfile` from `crates/domain/src/agent_definition.rs`.
- Keep `sub_agents` in `AgentDefinition` and make sure nested definitions use the same specialist-free agent schema.
- Replace `ToolSpec::Delegate`, `ToolSpec::CheckDelegatedStatus`, `DelegateConfig`, and generated OpenAPI schema with subagent/agent-task equivalents. The capability stays; the specialist/delegate vocabulary does not.
- Rename delegate tool registration in `crates/agent/src/rig_tool_registry.rs` to subagent/agent-task registration.
- Rename delegate session stream creation from runtime state/handler setup to subagent task stream creation.
- Rename delegate-result handling, parent-turn waiting, force-fail delegates, and delegate status logic in `crates/runtime/src/handler.rs` to subagent task/result handling.
- Replace `CronJobSource::Delegate` and delegate-only columns/queries with subagent-task naming, or reset runtime SQLite migrations to a clean baseline with subagent-task fields.

Keep ordinary cron/wake scheduling. That is a user-facing scheduling capability, separate from subagent task execution.

### R4 - Single Runtime Image + Developer Template

Delete the specialist runtime image path, but preserve the Dockerfile as a template:

- rename `sandboxes/runtime/Dockerfile.specialist` -> `sandboxes/runtime/Dockerfile.developers`
- `sandbox-runtime-specialist-*` Make targets
- `cmd/buildtemplates sandbox-runtime-specialist`
- `hivy-sandboxes-runtime-specialist` image registration
- specialist warm-pool mode and tests

Keep the runtime image name as the sandbox runtime image; this is not an employee-named artifact. `Dockerfile.developers` belongs to the agent workspace template catalog users/admins can pick from when configuring an agent, alongside future template images.

### R5 - Agent Sandbox Startup

Rename the main sandbox creation path:

- `CreateEmployeeSandbox` -> `CreateAgentSandbox`
- `buildEmployeeSandboxName` -> `buildAgentSandboxName`
- `employeeSandboxEnvVars` -> `agentSandboxEnvVars`
- labels use `agent_id`
- warm claim mode is `agent`

This work prepares W9 but does not have to implement per-session snapshot forking yet. Each agent will eventually have an admin-built workspace snapshot; this plan makes the runtime contract agent-shaped so W9 can build on it cleanly.

### R6 - Runtime Webhook Ingestion

Rename outbound handling:

- `EmployeeOutboundWebhookHandler` -> `AgentOutboundWebhookHandler`
- `EmployeeEventWriter` -> `AgentEventWriter`
- employee SQLite backup handlers -> agent SQLite backup handlers
- memory-retain task payloads and generation tags use agent/session names

Delete specialist callback handling and specialist task fallback paths. Runtime event persistence should target the new `sessions`/`session_events` naming once W4 lands; before W4, keep the minimal renamed control-plane shape needed for compile-green backend tests.

### R7 - Hivy Prompt Policy

Replace the current engineering-coordinator/delegation prompt with a Hivy agent prompt:

- Hivy has all tools available.
- Hivy may inspect systems, query data, browse, read, and write scripts to gather facts.
- Hivy does not perform and deliver implementation work as the final worker.
- For work that should be performed by another agent, Hivy should explain the needed assignment now and later use W10 `handoff_to_agent()` when available.
- Do not encode this as runtime tool restrictions.

### R8 - Full Agent Runtime E2E Harness

Add one high-signal E2E test that proves the renamed runtime works as an agent runtime, not just as a set of renamed packages.

The test should run against the real sandbox runtime and real OpenRouter credentials, but replace the production control plane with an in-process mock control plane. The mock control plane should expose the agent callback and backup endpoints needed by the runtime:

- `/internal/webhooks/agent/{sandboxID}` for outbound runtime events
- `/internal/agents/{agentID}/sqlite-backup/*` for backup upload/list/download flows
- any token/session bootstrap endpoints the runtime needs to start the task

The fixture should seed:

- one organization
- one agent
- one configured subagent under the parent agent definition
- one agent workspace/snapshot reference
- one session
- a coding-task prompt
- a custom MCP server dedicated to this test
- a compact fixture repository with tests the agent can modify and run

The coding task should intentionally require the agent to exercise the full configured tool surface. It should instruct the agent to:

- inspect the fixture repository
- invoke the configured subagent for one bounded helper step if the subagent tool is available in this phase
- call the custom MCP server tool and incorporate its returned value into the code change
- edit source files
- run tests
- run at least one research/gathering script or shell command
- emit progress messages while working
- request or trigger a SQLite backup path if backup is runtime-driven
- finish with a concise result message

The custom MCP server should be deterministic and test-owned. It should return a value that can be asserted in the final repository state, such as a generated requirement, a token, or a fixture-specific constant. The test should fail if the final code does not prove the MCP result was actually used.

The mock control plane should record every runtime event, webhook payload, backup request, model-usage event, tool invocation, tool result, and final message. Assertions should cover:

- the seeded agent starts with `agent_id`, not `employee_id`
- the runtime config contains `sub_agents`
- parent and nested subagent definitions contain no `mode`, `specialist_profile`, delegate tools, or specialist fields
- output is streamed during the task, not only emitted at completion
- each enabled test tool has both an invocation event and a terminal result/error event
- subagent task execution is recorded with agent/subagent terminology, not specialist/delegate terminology
- the custom MCP server was connected, called, and its result affected the code
- the fixture repo tests pass after the agent change
- OpenRouter model usage is reported with provider/model/token metadata
- outbound webhooks use `/internal/webhooks/agent/{sandboxID}` and carry `agent_id`
- SQLite backup upload/list/download behavior works against the mock control plane
- no recorded payload contains `employee_id`, `employee_session_id`, `specialist`, `delegate`, or `runtime_mode`
- the final agent message references the completed coding task and the test-observed result

Make this a single acceptance test with a clear name, for example `TestAgentRuntimeCodingTaskE2E`. It should be opt-in for local/manual/CI environments with real credentials, but it is the strongest verification gate for the migration. Suggested required env:

- `OPENROUTER_API_KEY`
- `HIVY_E2E_OPENROUTER_MODEL`
- `HIVY_AGENT_RUNTIME_E2E=1`

The test should emit enough structured artifacts on failure to debug the runtime without rerunning blindly: captured event log, final workspace diff, MCP server calls, mock control-plane requests, runtime logs, backup metadata, and model usage.

---

## 4. Implementation Order

1. **Runtime contract rename:** Go package/type/env/token rename plus Rust `AgentDefinition` schema cleanup. Compile green with the current single-agent behavior.
2. **Specialist deletion:** remove specialist compile paths, specialist prompt/tool surface, specialist runtime image registration, specialist warm-pool mode, and specialist task plumbing. Rename `Dockerfile.specialist` to `Dockerfile.developers` as a retained workspace template image.
3. **Subagent preservation in Rust:** keep `sub_agents` in `AgentDefinition`, remove specialist schema fields, and rename legacy delegate tools, scheduler paths, stream/result handling, and tests to subagent/agent-task terminology.
4. **Agent sandbox startup:** rename sandbox creation, env injection, labels, backup endpoints, and runtime webhook endpoints to agent naming.
5. **Hivy prompt replacement:** install the new all-tools concierge/research prompt with no specialist references.
6. **Generated clients/docs:** regenerate runtime OpenAPI/client code and backend OpenAPI after route/schema rename.
7. **Runtime E2E harness:** add the seeded-agent coding-task E2E with mock control plane, configured subagent, custom MCP server, real OpenRouter credentials, streaming assertions, tool assertions, subagent assertions, and SQLite backup assertions.
8. **Grep gates:** enforce no runtime/control-plane `employee`, `specialist`, or legacy `delegate` references outside deliberate historical docs during this phase.

---

## 5. Verification

Minimum checks for this phase:

- `go test ./internal/agentruntime ./internal/agentsandbox ./internal/sandbox ./internal/handler ./internal/gateway ./internal/tasks`
- `cargo test --manifest-path sandboxes/runtime/Cargo.toml`
- `make sandbox-runtime-openapi`
- `make generate-sandbox-runtime-client`
- build the single runtime image successfully
- `HIVY_AGENT_RUNTIME_E2E=1 OPENROUTER_API_KEY=... HIVY_E2E_OPENROUTER_MODEL=... go test ./... -run TestAgentRuntimeCodingTaskE2E`
- grep gate: no `specialist` in runtime/control-plane code
- grep gate: no legacy `delegate` vocabulary in runtime/control-plane code after it is renamed to subagent/agent-task terminology
- grep gate: no `employee` in runtime/control-plane code except approved compatibility-free historical docs during the transition

The old specialist/delegation tests should be deleted or rewritten with the code they cover. Replacement tests should assert that a compiled agent config has `sub_agents` support, no mode/specialist/delegate fields, runtime env uses `HIVY_AGENT_ID`, outbound events/webhooks carry `agent_id`, and the full E2E test proves a real seeded agent can complete a coding task through streamed output, tool usage, subagent task execution, custom MCP, mock control-plane webhooks, and SQLite backups.

---

## 6. Hand-off To The Main V2 Plan

After this runtime plan lands, the main v2 architecture plan can proceed with:

- baseline DB rewrite using `agents`, `channels`, `sessions`, `session_events`, and `session_message_queue`
- agent CRUD
- per-agent admin-built workspace snapshots
- first-class channel/session APIs
- server-side realtime ingestion
- Slack custom bot binding in a new table
- W10 `handoff_to_agent()` as transfer, not delegation
