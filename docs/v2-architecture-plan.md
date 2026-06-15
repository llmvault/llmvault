# Hivy V2 Architecture Migration Plan

**Status:** Approved direction, pre-implementation
**Date:** 2026-06-12
**Scope:** Clean-slate migration to the multi-agent workspace architecture. The static design exploration under `apps/web/app/w/(chat)/` is the product spec.

---

## 1. Product decisions (locked)

1. **Zero onboarding.** Sign up → land directly in `/w` with a prompt input. No wizard, no Slack/connection/business-profile gates.
2. **Multiple agents, no specialists.** Users CRUD agents. Hivy remains the default agent auto-created for every org — the always-on, org-wide concierge that does **not** write code. Each agent declares a `sandbox_strategy`: `always_on` (one 24/7 sandbox, sessions multiplexed — Hivy) or `per_session` (an isolated sandbox forked per session from the agent's workspace snapshot — the default for agents with repos/workspace tools). The specialist construct (catalog, dispatch tool, specialist sandboxes, `specialist_tasks`) is deleted entirely.
3. **First-class channels, web-first.** Channels are a real domain object usable entirely without any chat connector: any org member can create channels, channels have `public|private` visibility (public = org-discoverable and joinable), org members are added to channels, and channels have a **default agent**. An external app link (Slack, Discord, Microsoft Teams, or an arbitrary connected app) is optional provider/resource metadata on a channel, never its identity.
4. **Sessions live inside channels** and are **multi-user**: org members can be invited into a session and collaborate in real time (presence, shared streaming, queued messages). A session's agent is **mutable** — reassignable by humans and via agent-initiated handoff (see decision 6 and W10).
5. **Artifacts** (apps, drive, canvas, browser, review, terminal, files) become real, backed by the session's sandbox and org storage. Static versions exist in the `/w` right panel today.
6. **Slack routing precedence — static beats dynamic.** (1) A mention of a per-agent custom Slack bot routes to its bound agent (Gumloop model; user supplies their own Slack app credentials). (2) Otherwise the linked channel's default agent takes the session. (3) Otherwise Hivy takes it and may invoke `handoff_to_agent()` — a one-shot, guarded, human-reversible **transfer** (never a delegation; no return path) when it isn't the right agent. See W10.
7. **Rename `employee` → `agent` everywhere.** Not live yet: all goose migrations are rewritten from scratch as a new baseline. No data migration, no backwards compatibility, no phased rollout.

### Competitive research (Notion, Gumloop) — Slack routing

Neither product uses an LLM router for inbound Slack. Both bind statically:

| | Gumloop | Notion Custom Agents |
|---|---|---|
| Binding | **One agent per Slack channel** (standard bot). `/gummie add/remove` to manage. | Per-agent triggers: "when mentioned in channel X". Mentionable per-agent handles via Slack user groups. |
| Multiple agents | Custom Slack app per agent (1:1 bot↔agent), pick agent by which bot you mention | Multiple agents may fire on the same message; each decides via instructions whether to reply (no arbitration) |
| Threads | Slack thread = one conversation | Triggered runs logged in agent Activity; replies in-thread |
| Default/fallback | None | None |

**Our design** (validates decision #3+#6): a Slack channel is *linked* to a Hivy channel; the Hivy channel's **default agent** receives every new session started from that Slack channel. Thread = session (as today). Changing who answers in Slack = changing the channel's default agent, or linking the Slack channel to a different Hivy channel. Gumloop's custom-Slack-app model is **adopted** (per-agent bot handles via user-supplied credentials, W6); Notion-style user-group handles are deferred. Dynamic routing exists only as Hivy's bounded `handoff_to_agent()` fallback — a known pattern (OpenAI Agents SDK handoffs, support-triage products) that both Notion and Gumloop deliberately avoided for predictability reasons, which is why static bindings always take precedence and handoff carries hard guardrails (W10).

A note on comparables: Notion and Gumloop are the right references for routing and agent CRUD but **not for compute** — their agents are thin LLM loops over SaaS connectors with no persistent filesystem. For sandbox architecture the relevant comparables are Devin, Codex, Claude Code cloud, and Cursor background agents, all of which are sandbox-per-task forked from a prepared snapshot. That is the model adopted for `per_session` agents (§2.1, W9).

---

## 2. Target domain model

### 2.1 Core entities and cardinality

```
Org
 ├─ OrgMembership (user, role: owner|admin|member)
 ├─ Agent (N; "Hivy" auto-created at signup; user-CRUDable)
 │    ├─ always_on  → 1 long-lived sandbox; sessions multiplex over it (Hivy)
 │    └─ per_session → workspace snapshot; 1 sandbox forked per session
 ├─ Channel (N; "#general" auto-created at signup; default_agent_id;
 │    │       visibility public|private; optional external app link)
 │    ├─ ChannelMember (org members create/join/are added — fully usable without Slack)
 │    └─ Session (N; channel_id NOT NULL, agent_id NOT NULL **mutable**, created_by)
 │         ├─ SessionParticipant (multi-user; creator + invited members)
 │         ├─ SessionEvent (immutable event log → conversation blocks)
 │         └─ Artifact (apps, canvas, drive files, browser snapshots…)
 └─ Connection / Integration / Skill / Billing (unchanged shape, renamed FKs)
```

Sandbox cardinality, by agent `sandbox_strategy`:

- **`always_on` (Hivy):** one long-lived sandbox, auto-stop exempt (it is the 24/7 Slack/schedule/trigger responder — cold-starting on every ping is unacceptable latency). The control plane owns canonical `session_id`s and posts turns to the runtime `/sessions/{session_id}/messages` API, so always-on agents can safely multiplex Slack, schedule, trigger, web, and custom-channel work without a separate runtime conversation identifier.
- **`per_session` (default for workspace/coding agents):** concurrent sessions sharing one sandbox would share a working tree, git state, ports, and processes — a correctness bug, not just a load problem (the static design's per-session "Copy worktree path" assumes isolation). Each session gets its own sandbox **forked from the agent's workspace snapshot**: repos cloned and `setup_commands` run once at agent setup, snapshotted via the existing `sandbox_templates` build pipeline; sessions boot from the snapshot (warm-claimed where the provider supports it). This is the Devin/Codex/Claude-Code-cloud model. 10-min idle auto-stop keeps cost proportional to *active* use, not session count; a new reaper deletes per-session sandboxes N days after session end so the fleet doesn't grow unbounded. Agent memory survives sandbox churn because hindsight banks are per-agent and control-plane-side, not in the sandbox.
- Specialist sandboxes, specialist runtime mode, warm-pool specialist mode, and the snapshot-repo-based selector filtering in `internal/employeesandbox/selector.go` are all deleted. The old specialist Dockerfile is not deleted outright: `sandboxes/runtime/Dockerfile.specialist` is renamed to `sandboxes/runtime/Dockerfile.developers` and becomes a user-selectable developer workspace template image for agents.

### 2.2 New baseline migrations (goose, rewritten from 000001)

| # | Migration | Tables |
|---|-----------|--------|
| 000001 | extensions | pgcrypto, pg_trgm, session settings |
| 000002 | identity | `users`, `orgs`, `org_memberships`, `org_invites`, `oauth_accounts`, `oauth_exchange_tokens`, `otp_codes`, `password_resets`, `email_verifications`, `refresh_tokens` |
| 000003 | billing | `plans`, `subscriptions`, `subscription_change_quotes`, `credit_ledger_entries`, `usage`, `tool_usages`, `generations` (all current columns incl. idempotency/billing indexes folded in) |
| 000004 | credentials | `credentials`, `tokens`, `api_keys`, `audit_log` |
| 000005 | integrations | `integrations`, `connections`, `database_connections` |
| 000006 | sandboxes | `sandboxes` (+ nullable `session_id` for per-session sandboxes), `sandbox_templates`, `sandbox_warm_slots` (employee/specialist *mode* column → single `agent` mode), `custom_domains` |
| 000007 | agents | `agents`, `agent_skills`, `agent_schedules`, `agent_schedule_runs`, `agent_triggers`, `agent_trigger_deliveries`, `agent_sandbox_upgrades`, `hindsight_banks`, `failed_events` |
| 000008 | channels | `channels`, `channel_members` |
| 000009 | sessions | `sessions`, `session_participants`, `session_events`, `session_message_queue` |
| 000010 | skills | `skills` |
| 000011 | rag | all 11 `rag_*` tables (unchanged) |
| 000012 | cross-domain FKs | all foreign keys |
| 000013 | channel source uniqueness | source-aware channel uniqueness |

**Deleted from the old schema:** old employee/specialist runtime tables and fields, old external route/delivery/event tables, old employee session/event tables, `orgs.onboarded`, and `conversation_assets`. Late-history patch migrations are folded into the baseline definitions.

### 2.3 Schema details for the new tables

```sql
-- agents (renamed employees, minus specialist fields)
CREATE TABLE agents (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id uuid NOT NULL REFERENCES orgs(id) ON DELETE CASCADE,
    name text NOT NULL,                      -- unique per org
    description text NOT NULL DEFAULT '',
    avatar_url text,
    icon text,                               -- agent-grid icon (product-level)
    placeholder text,                        -- composer placeholder text
    is_default boolean NOT NULL DEFAULT false, -- true only for Hivy
    sandbox_strategy text NOT NULL DEFAULT 'per_session', -- always_on|per_session
    workspace_snapshot_id uuid,              -- latest ready snapshot (sandbox_templates)
    model text NOT NULL,
    instructions text,
    tools jsonb NOT NULL DEFAULT '{}',
    mcp_servers jsonb NOT NULL DEFAULT '[]',
    skills jsonb NOT NULL DEFAULT '[]',
    runtime_config jsonb NOT NULL DEFAULT '{}',
    permissions jsonb NOT NULL DEFAULT '{}',
    resources jsonb NOT NULL DEFAULT '{}',
    shared_memory boolean NOT NULL DEFAULT true,
    sandbox_tools text[] NOT NULL DEFAULT '{}',
    sandbox_template_id uuid,
    setup_commands jsonb,
    encrypted_env_vars bytea,
    credential_id uuid,
    harness text NOT NULL DEFAULT 'agent-sandbox',
    status text NOT NULL DEFAULT 'active',   -- draft|active|archived
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (org_id, name)
);

CREATE TABLE channels (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id uuid NOT NULL REFERENCES orgs(id) ON DELETE CASCADE,
    name text NOT NULL,                      -- slug-ish, unique per org + source tuple
    description text NOT NULL DEFAULT '',
    kind text NOT NULL DEFAULT 'standard',   -- standard|personal (see §2.4)
    visibility text NOT NULL DEFAULT 'public', -- public: org-discoverable + joinable
                                               -- private: invite-only
                                               -- (personal channels: private by construction)
    default_agent_id uuid NOT NULL REFERENCES agents(id),
    is_default boolean NOT NULL DEFAULT false, -- #general
    origin text NOT NULL DEFAULT 'native',   -- native|external; intentionally open-ended
    external_provider text NOT NULL DEFAULT '', -- slack|discord|microsoft_teams|custom|...
    external_connection_id uuid REFERENCES connections(id) ON DELETE SET NULL,
    external_workspace_key text NOT NULL DEFAULT '', -- Slack team, Discord guild, Teams tenant/team, etc.
    external_resource_type text NOT NULL DEFAULT 'channel',
    external_resource_key text NOT NULL DEFAULT '',  -- provider channel/conversation/resource id
    external_resource_name text NOT NULL DEFAULT '',
    external_resource_url text NOT NULL DEFAULT '',
    external_metadata jsonb NOT NULL DEFAULT '{}',
    created_by uuid REFERENCES users(id),
    archived_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
);

CREATE UNIQUE INDEX idx_channels_org_source_name
    ON channels (org_id, origin, external_provider, external_workspace_key,
                 external_resource_type, name)
    WHERE archived_at IS NULL;

CREATE UNIQUE INDEX idx_channels_org_external_resource
    ON channels (org_id, external_provider, external_workspace_key,
                 external_resource_type, external_resource_key)
    WHERE external_resource_key <> '';

CREATE TABLE channel_members (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    channel_id uuid NOT NULL REFERENCES channels(id) ON DELETE CASCADE,
    user_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    role text NOT NULL DEFAULT 'member',     -- owner|member
    created_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (channel_id, user_id)
);

CREATE TABLE sessions (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id uuid NOT NULL REFERENCES orgs(id) ON DELETE CASCADE,
    channel_id uuid NOT NULL REFERENCES channels(id),
    agent_id uuid NOT NULL REFERENCES agents(id),  -- MUTABLE: handoff/reassignment (W10)
    sandbox_id uuid REFERENCES sandboxes(id),      -- per-session sandbox for per_session agents
    created_by uuid REFERENCES users(id),          -- NULL for slack/webhook-originated
    model text,                                    -- per-session model override
    access_mode text NOT NULL DEFAULT 'full',      -- full|edits|read  (composer)
    reasoning_effort text NOT NULL DEFAULT 'high', -- low|medium|high  (composer)
    source text NOT NULL DEFAULT 'web',            -- web|slack|email|webhook|schedule|api
    source_id uuid,                                -- e.g. Connection.ID for slack
    source_resource_key text,                      -- e.g. slack channel:thread_ts
    name text NOT NULL DEFAULT '',
    status text NOT NULL DEFAULT 'active',         -- active|ended|error|archived
    integration_scopes jsonb,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    ended_at timestamptz
);
CREATE INDEX idx_sessions_channel ON sessions(channel_id, created_at DESC);
CREATE INDEX idx_sessions_agent ON sessions(org_id, agent_id, created_at DESC);

CREATE TABLE session_participants (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    session_id uuid NOT NULL REFERENCES sessions(id) ON DELETE CASCADE,
    user_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    role text NOT NULL DEFAULT 'collaborator',     -- owner|collaborator
    invited_by uuid REFERENCES users(id),
    joined_at timestamptz,                         -- NULL = invited, not yet joined
    last_seen_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (session_id, user_id)
);

CREATE TABLE session_events (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id uuid NOT NULL,
    session_id uuid NOT NULL REFERENCES sessions(id) ON DELETE CASCADE,
    agent_id uuid NOT NULL,
    sandbox_id uuid,
    runtime_session_id text,
    event_id text,                                 -- runtime idempotency key
    event_type text NOT NULL,   -- user.message | agent.message | agent.thinking
                                -- | agent.tool_call | agent.edits | session.system
                                -- | participant.joined | participant.left | …
    actor_user_id uuid,                            -- which human (multi-user attribution)
    source text NOT NULL DEFAULT 'web',
    sequence_number bigint,
    payload jsonb NOT NULL DEFAULT '{}',
    event_at timestamptz NOT NULL DEFAULT now(),
    retained_at timestamptz
);
CREATE UNIQUE INDEX idx_session_events_idem ON session_events(session_id, event_id)
    WHERE event_id IS NOT NULL;
CREATE INDEX idx_session_events_session ON session_events(session_id, event_at);

CREATE TABLE session_message_queue (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id uuid NOT NULL REFERENCES orgs(id) ON DELETE CASCADE,
    session_id uuid NOT NULL REFERENCES sessions(id) ON DELETE CASCADE,
    session_event_id uuid NOT NULL REFERENCES session_events(id) ON DELETE CASCADE,
    sequence_number bigint NOT NULL,
    status text NOT NULL DEFAULT 'pending', -- pending|leased|delivered|failed|cancelled
    attempt_count integer NOT NULL DEFAULT 0,
    leased_by text,
    leased_until timestamptz,
    delivered_at timestamptz,
    last_error text NOT NULL DEFAULT '',
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (session_id, session_event_id),
    UNIQUE (session_id, sequence_number)
);
CREATE INDEX idx_session_message_queue_claim
    ON session_message_queue(session_id, status, sequence_number);

CREATE TABLE artifacts (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id uuid NOT NULL REFERENCES orgs(id) ON DELETE CASCADE,
    session_id uuid REFERENCES sessions(id) ON DELETE SET NULL,
    agent_id uuid REFERENCES agents(id) ON DELETE SET NULL,
    created_by uuid REFERENCES users(id),
    kind text NOT NULL,            -- app|canvas|document|browser_capture|review|file
    title text NOT NULL DEFAULT '',
    storage text NOT NULL,         -- s3|inline|sandbox
    s3_key text,
    content jsonb,                 -- inline payloads (canvas docs, review diffs)
    preview_url text,
    metadata jsonb NOT NULL DEFAULT '{}',
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX idx_artifacts_session ON artifacts(session_id, created_at DESC);
```

`session_events.actor_user_id` is the multi-user upgrade of the old event table: every human-originated event records *which* member sent it, which is what the static UI's per-author message bubbles, "Running Marcus's request", and queued-message attribution need.

### 2.4 The "every session has a channel" invariant vs. private chats

The static sidebar groups chats under projects/channels, but users will also start quick personal sessions from the `/w` root composer. Two options were considered:

- **(chosen) Personal channels:** at first workspace entry, lazily create a `kind='personal'` channel per member (`name = user slug`, sole member, default agent = Hivy). Sessions started outside any channel context land there. Invariant holds (`channel_id NOT NULL`), the sidebar renders personal sessions under "Chats" and shared channels under "Channels", and inviting someone to a personal session works without moving it (session participants are independent of channel membership — see §4.3 authz).
- Nullable `channel_id` — rejected: every query, authz check, and the Slack mapping would need a null branch forever.

### 2.5 Org bootstrap (replaces onboarding)

`POST /auth/register` transactionally creates: user → org → membership(owner) → **Hivy agent** (`is_default=true`, engineering identity prompt from `internal/agentprompts/`, drive skill) → **#general channel** (`is_default=true`, `default_agent_id=hivy`, creator as member). No `onboarded` flag anywhere. Sandbox creation stays lazy (first message), using the existing warm-pool claim path so the first prompt is fast. Business profile, Slack, and connections all become optional post-signup settings surfaces.

---

## 3. The rename: employee → agent

Blast radius (measured): 13 tables, 10 model files, 77 handler files, 4 main packages (+47 incidental dirs), all `/v1/employees*` routes, API-key scope `"employees"`, 2 env vars, 2 cmd tools, Makefile targets, evals, ~142 test files, ~20 frontend files plus the generated `schema.d.ts`.

Rules:

- **DB:** new baseline migrations use `agents`, `channels`, `sessions`, and `session_events`; sessions are first-class objects reached from channels, not agent-subordinate records.
- **Go:** `internal/model/employee*.go` → `agent*.go` (`Employee` → `Agent`, `EmployeeSession` → `Session`, …); packages `employeeruntime` → `agentruntime`, `employeeprompts` → `agentprompts`, `employeesandbox` → `agentsandbox` (then mostly deleted — see §5.2); handlers `employees_*.go` → `agents_*.go`.
- **API:** `/v1/employees/*` → `/v1/agents/*`; scope `"employees"` → `"agents"`; OpenAPI schemas regenerate, so `schema.d.ts` renames flow into the frontend mechanically.
- **Env/ops:** `HIVY_EMPLOYEE_SQLITE_BACKUP_MAX_BYTES` → `HIVY_AGENT_…`; `HIVY_SANDBOX_WARM_POOL_EMPLOYEE_SIZE` → `HIVY_SANDBOX_WARM_POOL_AGENT_SIZE`; `HIVY_SANDBOX_WARM_POOL_SPECIALIST_SIZE` deleted; `cmd/employee-env-doctor` → `cmd/agent-env-doctor`; `cmd/employee-debug-pack` → `cmd/agent-debug-pack`; ansible/docker references swept.
- **Runtime image:** the specialist runtime image is deleted as a runtime concept; `Dockerfile.specialist` is renamed to `Dockerfile.developers` and kept as a workspace template image users/admins can choose for developer agents. The runtime image keeps its name (it's the *sandbox* runtime, not employee-named) — only env vars passed into it rename.
- The rename is **commit 1 of the program** (mechanical, reviewable in isolation), before any behavioral change.

---

## 4. Backend workstreams

### W1 — Schema reset + rename (foundation)

1. Delete `internal/migrations/sql/0000{01..34}*`; write the 14 baseline migrations of §2.2.
2. Rename models/packages/handlers/routes per §3. Compile-clean, tests renamed and green.
3. Delete specialist code: `internal/specialists/`, `internal/specialisttasks/`, `global/specialists/`, `internal/handler/employees_specialists*.go`, `employee_outbound_specialists.go`, `orchestrator_create_specialist.go`, `CompileSpecialist*` in compile.go, specialist filtering in the sandbox selector, `evals/employee-delegation-v1.yaml`, specialist warm-pool mode.
4. Org bootstrap per §2.5; delete onboarding handlers/gates.

### W2 — Agent CRUD

New/changed endpoints (all org-scoped, JWT or API key scope `agents`):

```
POST   /v1/agents                  create (name, description, model, instructions,
                                   tools, skills, mcp_servers, sandbox_tools, avatar,
                                   icon, placeholder, sandbox_strategy)
GET    /v1/agents                  list (existing, renamed)
GET    /v1/agents/{id}             get (existing, renamed)
PATCH  /v1/agents/{id}             full update (today only model is patchable)
DELETE /v1/agents/{id}             archive (refuse for is_default; end active sessions
                                   or block while sessions active — block, simpler)
POST   /v1/agents/{id}/sync        existing resync, renamed
POST   /v1/agents/{id}/workspace/build   (re)build the workspace snapshot (W9)
GET    /v1/agents/{id}/workspace         snapshot status (building|ready|error, built_at)
…/skills, /sandbox/*               existing, renamed; /specialists/* deleted
GET    /v1/models                  promote from /v1/employees/models (org-level)
```

Notes:
- `is_default` (Hivy) is not deletable/renamable; `sandbox_strategy` is locked to `always_on` for it and Hivy's tool config excludes code/workspace tools (it does not write code — its instructions direct it to hand off coding work, W10).
- Agent creation does **not** create a sandbox; for `per_session` agents it kicks off the workspace snapshot build (W9); the first session forks from it.
- **`description` is a routing signal**: it is what Hivy triages on when deciding handoffs. The agent-creation UI must say so explicitly — handoff quality is bounded by description quality.

### W3 — Channels

Channels are **web-first**: a team that never connects Slack, Discord, Teams, or any other app gets the full experience — create channels, join public ones, organize sessions inside them. External linking is optional generic provider/resource metadata.

Channel display names are unique per source tuple, not org-wide: native `#engineering`, Slack `#engineering`, and Discord `#engineering` can coexist because uniqueness includes `origin`, provider, workspace, and resource type.

```
POST   /v1/channels                       create (name, description, visibility,
                                          default_agent_id) — any org member;
                                          creator becomes channel owner
GET    /v1/channels                       channels the caller is a member of
GET    /v1/channels?discoverable=true     + public channels the caller can join
GET    /v1/channels/{id}                  get (incl. members, default agent)
PATCH  /v1/channels/{id}                  rename, description, visibility, default_agent_id
DELETE /v1/channels/{id}                  archive (block on #general and personal)
POST   /v1/channels/{id}/join             self-join (public channels only)
PUT    /v1/channels/{id}/members/{userID} add member (channel owner/org admin)
DELETE /v1/channels/{id}/members/{userID} remove member / leave
GET    /v1/channels/{id}/sessions         sessions in channel (cursor-paginated)
POST   /v1/channels/{id}/slack-link       link/unlink a Slack channel (see W6)
```

Authz: channel content is visible to channel members + org owners/admins; public channels are *discoverable* (name/description) by all org members but content requires joining. Personal channels are visible only to their user (and sessions shared out of them via participants). Unread counts/notification badges are an explicit fast-follow (per-user read cursors), not v1 — live sidebar updates via the channel pub/sub topic (W5) cover the freshness need initially.

### W4 — Sessions v2

```
POST   /v1/sessions                        create (channel_id, agent_id?, first message,
                                           model?, access_mode?, effort?)
                                           agent_id defaults to channel.default_agent_id
GET    /v1/sessions?channel_id=&agent_id=  list (participant- or channel-scoped)
GET    /v1/sessions/{id}                   get (participants, agent, channel, presence)
PATCH  /v1/sessions/{id}                   rename, move channel, archive,
                                           reassign agent_id (human path of W10 handoff)
POST   /v1/sessions/{id}/messages          send message (any participant)
GET    /v1/sessions/{id}/events            history (cursor-paginated)
PUT    /v1/sessions/{id}/participants/{userID}   invite org member
DELETE /v1/sessions/{id}/participants/{userID}   remove / leave
GET    /v1/sessions/{id}/live              realtime stream (see W5)
POST   /v1/sessions/{id}/presence          heartbeat {viewing: bool, typing: bool}
```

Moved off the agent-subordinate paths (`/v1/employees/{id}/sessions/...`) because sessions are now the primary object reached from channels; the agent is an attribute. Send-message: resolve agent → resolve sandbox by strategy (`always_on`: the agent's long-lived sandbox; `per_session`: the session's own sandbox, forked from the workspace snapshot on session start, resumed if stopped — W9) → `PostHTTPMessage` to runtime → persist `user.message` event (with `actor_user_id`) → runtime streams back.

**Multi-user semantics:**
- Creator becomes `owner` participant. Invitees must be org members (org invite flow already exists and stays for getting people *into* the org).
- Any participant can send messages. The runtime receives the author's display name in message metadata so the agent can attribute ("Marcus asked…").
- Messages sent while a turn is running are **queued**: persisted immediately as `user.message` in `session_events` with a `queued: true` payload flag, broadcast to participants (renders as the static design's dashed "Queued" block), and represented by a `session_message_queue` row that owns mutable delivery state. `session_events` remains the canonical transcript; `session_message_queue` owns FIFO order, lease/retry state, and exactly-once runtime delivery. When the active turn completes, the control plane claims the next queue row by `(session_id, sequence_number)`, delivers the referenced `session_event_id` to the runtime using that event ID as the idempotency key, and marks the queue row delivered/failed without rewriting the transcript event.
- `participant.joined` / `.left` become `session.system` events ("Marcus Lee joined the session").

### W5 — Realtime layer (new `internal/realtime`)

The static design requires: all participants see the same streaming turn live, presence avatars, queued-message visibility, and system events — today's per-requester signed SSE stream can't do any of that.

**Architecture: server-side turn consumption + Redis pub/sub fanout + per-session SSE.**

1. **Turn ingestion moves server-side.** When a message is posted, the control plane calls `POST /sessions/{session_id}/messages`, then opens the returned `/sessions/{session_id}/streams/{stream_id}` URL. Runtime events are translated into `session_events` rows (thinking/tool/edits/message) and published to Redis channel `session:{id}`. One ingestion goroutine per active turn, owned by the instance that accepted the message; on crash, reconnect-and-resume using the runtime stream's replay.
2. **Fanout:** every API instance runs a Redis-subscriber hub; `GET /v1/sessions/{id}/live` is a long-lived SSE response (works through the existing Next.js proxy and `fetch-event-source` client — no WebSocket infra needed) that subscribes the client to that session's topic and replays from a `Last-Event-ID` cursor against `session_events` for seamless reconnect.
3. **Presence:** `POST /presence` heartbeats write Redis keys `presence:{session}:{user}` with 30 s TTL; the hub publishes presence diffs on the same topic. Powers the header avatar stack and "viewing now" popover.
4. **Channel-level topic** (`channel:{id}`) carries lightweight events (session created/renamed/ended) for live sidebar updates.
5. Token-level streaming granularity stays: ingestion republishes token deltas as ephemeral `stream.delta` frames (not persisted; persistence is per consolidated event), so all participants watch the same typing stream.

Slack/webhook sessions flow through the same pipeline — a Slack-originated session is visible (and joinable) in `/w` with zero extra work.

### W6 — External Channel Ingestion

Keep: Nango OAuth + webhook ingestion, provider-specific decode/render adapters, thread dedup/continuation, and reply delivery. Delete the old route/event/delivery abstraction. Every inbound integration presents as a channel source and writes into the same `channels`/`sessions`/`session_events` flow used by web.

**Routing precedence (decision #6):** custom bot binding > channel default agent > Hivy (+ handoff).

External ingestion flow:

1. Decode inbound provider payload into a canonical source tuple: `origin`, `external_provider`, `external_connection_id`, `external_workspace_key`, `external_resource_type`, `external_resource_key`, `thread_key`, sender metadata, and message body.
2. Resolve or create the channel using the source tuple. Slack, Discord, Microsoft Teams, and arbitrary external wrappers all use the same source-aware channel columns, so `#engineering` can exist independently per source.
3. Resolve or create the session using `(channel_id, thread_key)`. New session ⇒ `agent_id = channel.default_agent_id`; existing thread ⇒ preserve `sessions.agent_id` so handoff/reassignment sticks.
4. Deliver user messages via the new session message path, persist runtime output in `session_events`, and use provider-specific reply delivery only as a rendering sink for the same session events.
5. Wrapper callbacks are rebuilt here: an external chatbot/app integrates by registering a connection and presenting its inbound conversations as source-aware channels. No route table, route secrets, or delivery table are reintroduced.

**Per-agent custom Slack bots (Gumloop model, in scope):** users who want `@salesassistant` create a Slack app in their own Slack org and provide its credentials. The schema mostly exists — `integrations.custom_app` is already validated for Slack client_id/secret in `integrations_create.go`. New work: a 1:1 binding *custom Slack connection ↔ agent* (adopt Gumloop's rule: binding a connection to a second agent removes it from the first), and source identification that resolves inbound events to the binding's agent. Mentions of that bot route to that agent regardless of channel; the session still lands in the linked Hivy channel for visibility.

Out of scope for v1 (schema-ready, no work now): Notion-style user-group handles on the main bot, `/hivy` slash commands for channel-agent management.

### W7 — Artifacts

Two distinct things ship under "artifacts":

**(a) Live session views** — windows into the agent's sandbox, real-time, not persisted:

| View | Backing | New backend work |
|---|---|---|
| Terminal | sandbox exec | Runtime endpoint proxy: `POST /v1/sessions/{id}/terminal/exec` + streamed output over the session SSE topic (or a dedicated short-lived stream). Read-only command allowlist honors session `access_mode`. |
| Files | sandbox FS | `GET /v1/sessions/{id}/fs?path=` (list) and `GET …/fs/content?path=` via runtime client; reuse drive `FilesView` rendering. |
| Browser | sandbox preview | `GET /v1/sessions/{id}/preview` → provider `GetEndpoint(externalID, port)` signed URL; iframe it (replaces the static same-origin iframe). |
| Review | turn edits | Runtime already reports edits; ingestion (W5) persists `agent.edits` events with unified diffs in payload; Review view renders the latest accumulated diff set. "Undo" = runtime revert endpoint (stretch). |
| Side chat | second conversation | A child session: same channel + agent, `metadata.parent_session_id`, hidden from sidebar lists. Cheap — sessions are free. |

**(b) Persisted artifacts** (`artifacts` table) — durable outputs: canvas documents (`content` jsonb), generated apps (S3 bundle + `preview_url`, served via existing custom-domain/CDN machinery), drive documents (S3, existing `drive_assets` path), browser captures (screenshot S3 key). Agents create them through new Hivy-MCP tools (`artifact_create/update`) added to the compiled runtime config; humans browse them per-session (right panel) and org-wide (Drive page). `attachments` conversation blocks reference artifact IDs.

V1 ships: Review, Terminal, Files, Browser (live views) + Drive-backed documents and browser captures. Canvas and Apps land as fast-follows on the same `artifacts` substrate.

### W8 — Zero onboarding

- Delete `apps/web/app/onboarding/` and the gates in `app/w/(console)/layout.tsx`; signup redirect already targets `/w`.
- Keep the email-confirmation gate (it's auth, not onboarding).
- Backend: delete onboarding fields/logic from `orgs.go` (`onboarded`, business-profile-sync trigger); business profile remains editable in settings and feeds the agent prompt as today (`prompt_company` stays as an optional org field).

### W9 — Per-session sandboxes & agent workspace snapshots

The biggest *new* engineering item in the program (bigger than handoff). For `per_session` agents:

1. **Snapshot build:** on agent create/update (repos, `setup_commands`, sandbox_tools changed), build the agent's workspace via the existing `sandbox_templates` pipeline — boot a builder sandbox, clone repos, run setup, snapshot, mark ready, store as `agents.workspace_snapshot_id`. Surface build status in the agent UI; sessions cannot start until the first snapshot is ready (clear UX state).
2. **Session start:** fork a sandbox from the snapshot (provider `CreateSandbox` with snapshot ref; warm-claim where supported), set `sandboxes.session_id`, push compiled runtime config, deliver the first message. Target: comparable to today's warm-pool session start.
3. **Lifecycle:** existing 10-min idle auto-stop and 24-h archive apply per session-sandbox; resume on new message to a stopped session. New **reaper**: delete session sandboxes N days (default 7) after `sessions.ended_at`/archive. `always_on` agents keep the current exempt lifecycle.
4. **Snapshot staleness:** repos drift from the snapshot. Policy: sessions always fork the *latest ready* snapshot and `git pull` on boot as a cheap freshness pass; manual + scheduled rebuilds (`POST /workspace/build`); auto-rebuild on agent config change.
5. **Runtime sessions:** one canonical `session_id` per session-sandbox. The control plane's session→sandbox resolution branches on strategy in exactly two places: web send-message and external-channel ingestion.

### W10 — `handoff_to_agent()` and reassignment

A bounded, one-shot **transfer** — never a delegation. There is no return path and no results-back; if a future feature wants the source agent to receive results, that is specialists reborn and should be rejected by default (explicit founder line).

1. **Tool:** `handoff_to_agent(agent_name, brief)` in the Hivy-MCP surface compiled into runtimes. Primary user is Hivy (its instructions: triage; hand off coding/specialized work since it has no code tools), but available to all agents — guardrails make it safe.
2. **Guardrails:** max one handoff per user turn; an agent cannot hand off to the immediately previous agent without an intervening human message (kills A↔B loops); target must be an active agent in the org; if nothing fits, the agent answers best-effort and suggests what agent to create — no forced handoff.
3. **Mechanics:** control plane validates → resolves target sandbox by strategy (forking a per-session sandbox at handoff time if needed, W9) → preserves the same canonical `session_id` → **context transfer** = the structured `brief` + transcript replay compiled from `session_events` (we own the full event log) → updates `sessions.agent_id` and `sandbox_id` → emits a `session.system` handoff event, visible in `/w` and posted to the source channel ("Hivy handed this off to Software Engineer").
4. **Continuity:** same `session_id`, same Slack thread key, same `/w` URL, same participants. Only the answering agent changes.
5. **Human override:** `PATCH /v1/sessions/{id} {agent_id}` runs the same machinery (with an automatic control-plane-authored brief). The agent chip in the session header becomes interactive — this is also the correction path when a handoff was wrong.

---

## 5. Frontend workstreams

The stack is ready (Next 16/React 19, React Query + `openapi-fetch` generated client, server-side token proxy, solid SSE consumption in `lib/sessions/use-session-stream.ts`). The work is replacing static data with real wiring.

### F1 — Domain plumbing
- Regenerate `schema.d.ts` from the new OpenAPI; add typed hooks for agents, channels, sessions, participants, artifacts.
- New `useSessionLive(sessionId)` hook on `/v1/sessions/{id}/live` (adapt the existing reconnect/dedup StreamBuffer — it was built for exactly this).
- Replace `_lib/agents.ts` static `AGENTS`/`MODELS` with `GET /v1/agents` + `GET /v1/models`; replace `_lib/static-data.ts` consumers incrementally per surface.

### F2 — Shell, sidebar, routing
- Sidebar "projects" → **channels** (`GET /v1/channels`) with their sessions; "Chats" section → personal-channel sessions. Routes: `/w` (new session), `/w/channels/[channel]` (exists as a stub), `/w/s/[sessionId]` (deep link).
- **Channel home view** at `/w/channels/[channel]` — the surface that makes channels real without Slack: header (name, description, default-agent chip, member avatars), session feed (cursor-paginated, live-updating via the channel topic), and a composer that starts a session *in this channel* with the default agent preselected (overridable). The static design only renders ChatCanvas here; this view is net-new design work.
- Channel CRUD UI: create channel (name, visibility, default agent picker), browse/join public channels, member management, default-agent switcher.
- `WorkspaceContext` becomes real: session = fetched object; `startSession` → `POST /v1/sessions`; `openChat` → router push.

### F3 — Conversation + composer
- Map `session_events` → `ConversationBlock` (the 13 static block types):

| event_type | block |
|---|---|
| `user.message` | `user` (author from `actor_user_id`) / `queued` if flagged |
| `agent.message` | `assistant` (+ trailing `actions`) |
| `agent.thinking` | `thinking` |
| `agent.tool_call` | `tool` / collapsed into `worked` on turn end |
| `agent.edits` | `edits` / `activity` |
| `session.system`, `participant.*` | `system` |
| `stream.delta` (ephemeral) | streaming tail |
| artifact refs in payloads | `attachments` |

- Composer: agent picker (NewSessionView grid from real agents), model picker (agent's `modelIds`), access mode + effort → persisted session fields sent with `POST /messages`; send/stop wired to real turn lifecycle; queued-send while streaming.
- Presence stack + collaborator attribution from live presence frames; invite-to-session UI on the header presence popover.
- Agent chip in the session header becomes **interactive** (reassign agent → `PATCH /v1/sessions/{id}`); handoff `session.system` events render in the conversation.

### F4 — Right panel artifacts
Wire the five views to W7 endpoints (Review→edits events, Terminal→exec, Browser→preview URL, Files→sandbox FS, Side-chat→child session). Add Drive artifact browsing into the panel; lightbox already handles media.

### F5 — Console consolidation
- **Keep as-is:** connections, usage, credits, teams, drive (org-wide), skills (gains an agent selector).
- **Delete:** `(console)/sessions` (replaced by the new `/w` chat), `knowledge` stub, onboarding.
- **Settings:** keep the static settings modal shell; wire General/Appearance first, fold console settings pages in over time.

---

## 6. Implementation order

No data migration, but dependencies force an order. Each stage = one or a few PRs, tests green throughout.

1. **Reset & rename** (W1): migrations baseline, rename sweep, specialist deletion, org bootstrap, zero-onboarding backend. *Big-bang mechanical commit; everything still works as a single-agent product.*
2. **Agents + channels** (W2, W3): CRUD APIs + bootstrap defaults. Frontend F1 + channel sidebar (F2) behind the existing UI.
3. **Sessions v2 + per-session sandboxes** (W4, W9): new session API on channels/participants; workspace snapshot build + fork-per-session; old session endpoints deleted; `(console)/sessions` temporarily repointed or frozen. W9 lands here because everything downstream (realtime, handoff, live artifact views) assumes the session↔sandbox binding.
4. **Realtime** (W5): ingestion-side streaming, Redis fanout, `/live`, presence. Riskiest workstream alongside W9 — build it before the new UI ships so the UI never wires to the older per-requester stream.
5. **New /w goes real** (F2–F4 + W7 live views): conversation, composer, channel home, right panel, multi-user. Delete static data files. Zero-onboarding frontend (W8) flips on here — signup lands in a fully working `/w`.
6. **External ingestion + handoff** (W6, W10) + persisted artifacts (W7b) + console consolidation (F5). Handoff lands with Slack/external channels because that's where triage matters; web users pick agents explicitly.

Testing: keep the `*.test` harness (renamed); new e2e suites for channel authz, session participation/queueing, realtime reconnect/replay, and the Slack channel-link auto-create path. The evals harness gets a replacement for the deleted delegation eval (multi-agent assignment eval).

---

## 7. Risks & watch items

1. **Server-side turn ingestion (W5.1)** changes who owns the runtime stream. Failure modes: instance dies mid-turn (mitigate: resumable runtime streams + event-cursor replay), double-ingestion on retry (mitigate: `event_id` idempotency index, already in schema).
2. **Queued multi-user messages** need a per-session FIFO with exactly-once delivery into the runtime — keep the user-visible transcript in `session_events`, and keep mutable delivery state in `session_message_queue` with leases, retries, and `session_event_id` idempotency. Do not hold the queue in memory.
3. **Terminal/FS/preview endpoints** widen the sandbox attack surface — every one must enforce session participation + `access_mode`, and the preview URL must be signed/expiring (provider `GetEndpoint` URLs may be long-lived; wrap them).
4. **Sandbox concurrency:** per-session isolation (W9) solves this for workspace agents. For `always_on` agents (Hivy), N sessions still share one sandbox — add a per-agent turn concurrency limit (config) before launch.
4b. **Snapshot pipeline reliability (W9):** session start now depends on a prior successful build; a failed/stale snapshot blocks the agent. Mitigate: clear build-status UX, retry/rebuild affordances, and fall back to cold-boot (clone + setup at session start, slow but functional) when no ready snapshot exists.
4c. **Handoff mis-routing (W10):** a wrong handoff answers confidently in front of a team. Mitigations: static bindings take precedence (handoff only fires from Hivy-defaulted contexts), visible system events, one-click human reassignment, and the description-quality nudge in the agent creation UI. Track handoff accuracy in the evals suite from day one.
5. **Rename sweep** must include ansible/, docker/, Makefile, evals, and the two cmd tools, or ops breaks quietly. Grep gate in CI: `git grep -il employee -- ':!docs/v2-architecture-plan.md'` must come back empty at the end of stage 1 (allowing deliberate exceptions list).
6. **Slack auto-created channels** have no human members initially — they must still be visible to org admins, and the session must be joinable from `/w` (channel visibility rule in W3 covers org admins; consider auto-adding the Slack sender if their Slack identity maps to an org member — stretch).

## 8. Deletion manifest — everything we're completely nuking

Consolidated demolition list. Each item should be *gone* (not deprecated, not flagged off) by the end of the stage noted. CI grep gates at the end of each stage verify nothing lingers.

### 8.1 The specialist construct (stage 1) — the largest demolition

| Layer | What dies |
|---|---|
| DB | `specialist_tasks` table; `employees.attached_specialists`; `employee_session_events.specialist_slug`, `.specialist_task_id`, `.mode` |
| Packages | `internal/specialists/` (catalog), `internal/specialisttasks/` (launch service) |
| Definitions | `global/specialists/` — `software-engineering-specialist/`, `business-research-specialist/` (agent.json + prompt.md) |
| Handlers | `internal/handler/employees_specialists*.go`, `employee_outbound_specialists.go` |
| Sandbox | `internal/sandbox/orchestrator_create_specialist.go`; specialist warm-pool mode in `warm_pool*.go`; specialist-image exclusion filter in `internal/employeesandbox/selector.go` |
| Runtime compile | `CompileSpecialist()` / `CompileSpecialistWithProxyToken()` in `internal/employeeruntime/compile.go`; the `specialist_launch_task` tool from compiled runtime configs |
| Images | `ghcr.io/usehivy/hivy-sandboxes-runtime-specialist` registry image as a runtime image; its specialist-runtime registration in `cmd/buildtemplates/sandbox_runtime.go`. `sandboxes/runtime/Dockerfile.specialist` is renamed to `sandboxes/runtime/Dockerfile.developers` and kept as a developer workspace template image, not deleted. |
| API | `GET/POST/DELETE/PATCH /v1/employees/{id}/specialists/*` |
| Env | `HIVY_SANDBOX_WARM_POOL_SPECIALIST_SIZE` |
| Evals | `evals/employee-delegation-v1.yaml` (replaced later by a handoff-accuracy eval, W10) |
| Tests | every `*specialist*` test file (e.g. `employees_specialists_test.go`) |

Grep gate: `git grep -il specialist` → empty (allowing this file).

### 8.2 Onboarding (stage 1 backend, stage 5 frontend)

- `apps/web/app/onboarding/` — entire directory (page, `use-onboarding.ts` state machine, layout gate).
- `orgs.onboarded` column; the `sync: true` → mark-onboarded logic in `internal/handler/orgs.go`; the onboarded-gate redirect in `apps/web/app/w/(console)/layout.tsx`.
- The product requirements themselves: mandatory Slack connection, minimum-2-integrations, business-profile-to-finish. (Business profile survives as an optional settings field.)

### 8.3 Migration history (stage 1)

- All 34 files in `internal/migrations/sql/0000{01..34}_*.sql`, replaced by the 14-file baseline (§2.2). The folded-away patch history (idempotent backfills, column restores, no-ops like 000020/000021) does not get re-created.

### 8.4 Old session API shape (stage 3)

- All `/v1/employees/{id}/sessions/*` endpoints (list/messages/events/streams) — replaced by `/v1/sessions/*` (W4). The per-requester signed stream URL contract (`StreamURL`/`ResponseStreamURL` handed to the browser) dies with them, replaced by `/v1/sessions/{id}/live` (W5); the browser never talks to a runtime stream again.

### 8.5 Frontend (stage 5)

- `apps/web/app/w/(console)/sessions/` — the current real chat UI (replaced by the new `/w`).
- `apps/web/app/w/(console)/knowledge/` — unimplemented stub.
- `apps/web/app/w/(chat)/_lib/static-data.ts` — the entire mock dataset (conversation, projects, collaborators, terminal lines, patches, file tree).
- Static `AGENTS` / `MODELS` arrays in `_lib/agents.ts` (replaced by `GET /v1/agents` + `GET /v1/models`).
- `_components/chat-input.tsx` (noted unused in the component inventory).

### 8.6 Renamed-away, not deleted (stage 1) — for completeness

`employee` naming everywhere (13 tables, 10 model files, 77 handler files, `internal/employee{runtime,prompts,sandbox}/` packages, `/v1/employees*` routes, `"employees"` API-key scope, `HIVY_EMPLOYEE_*` / `HIVY_SANDBOX_WARM_POOL_EMPLOYEE_SIZE` env vars, `cmd/employee-env-doctor`, `cmd/employee-debug-pack`, Makefile targets, ansible/docker references). Grep gate per §7.5.

### 8.7 Deliberately NOT nuked

To prevent overshoot during demolition: auth/identity (incl. org invites), billing (plans/subscriptions/credits/Paystack), credentials/tokens/api-keys, integrations/connections/Nango, RAG (all 11 tables), skills, sandbox providers (Daytona/Docker/Railway) + warm pool (minus specialist mode) + templates pipeline (W9 builds on it), hindsight memory, drive/S3 storage, the Next.js token proxy, the SSE reconnect/dedup client logic (moves server-side, W5), and the console pages teams/connections/usage/credits/drive.

## 9. Open questions (non-blocking, decide during build)

1. Do channel members get notified (in-app) of new sessions in their channels? Unread badges/read-cursors are an explicit fast-follow (W3); the realtime channel topic already supports live updates.
2. ~~Can a session be reassigned mid-life?~~ **Decided: yes** — handoff (W10) and human reassignment via `PATCH /v1/sessions/{id}`; the agent chip is interactive.
3. Org roles vs channel roles: is `viewer` org role still needed with channel-level membership? Plan keeps owner/admin/member at org level.
4. Apps artifact hosting: reuse `custom_domains` + CDN, or per-app subdomains? Defer to the Apps fast-follow.
5. Reaper retention for per-session sandboxes (default 7 days after session end) — confirm against storage cost once real usage data exists.
