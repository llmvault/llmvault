# Integrations Triggers Plan — GitHub & Linear

Status: in progress. First implementation target: **GitHub mentions on issues and pull requests**.
Decided 2026-07-03.

## Goal

Curated, hand-crafted trigger automations for the platforms real software engineers live in —
GitHub and Linear — following the pattern established by Slack reaction triggers. Each trigger
is a deliberate product surface with its own install form, its own delivery code, and its own
default instruction playbook. No generic trigger builder, no workflow engine.

## Philosophy (locked decisions)

- **Curated triggers only.** ~3–4 high-value triggers per provider, each with unique manual
  handling. `global/triggers/README.md` already codifies this: "Provider-specific delivery code
  owns event handling."
- **Prompt-driven behavior.** The agent acts by loading the provider's tools (plugin-gated Nango
  proxy actions) and following the trigger's instructions. Shipped templates are editable
  defaults; the user's prompt is the real control surface. We trust the model.
- **No auto-posting of final text** (unlike Slack thread replies). Comments on tickets/PRs are
  deliberate artifacts the agent creates via tools when the playbook says to.
- **No auto-mounting of tools.** If a trigger's playbook says "comment on the issue" but the
  target agent has no Linear/GitHub plugin, the action fails and the user fixes the agent's
  config. Predictability over magic. (Verified: `AgentTrigger.ConnectionID` is used only for
  webhook matching — session tools come exclusively from `agent_plugin_installs`, resolved per
  org+provider in `internal/connectionaccess/access.go`.)
- **No cross-provider session aliasing.** PR ↔ Linear-issue linkage lives in prompts and
  artifacts ("include the issue URL in the PR body", "read the linked issue first"), not in a
  mapping table.
- **Everything stays on Nango.** OAuth + webhook forwarding, including Linear agent webhooks.
- **Multi-step recipes are playbooks, not workflows.** "Comment plan → move ticket → implement →
  open PR → comment link" is an instructions template (eventually a skill the instructions
  reference), executed by the model with tools.

## Existing primitives (what we build on)

- `AgentTrigger` (`internal/model/agent_trigger.go`) — trigger config: `TriggerType` (webhook/http),
  `ConnectionID`, `ChannelID`, `TriggerKeys`, `TriggerKey`/`TriggerValue`, `Conditions`
  (`TriggerMatch`), `Instructions`, `SourceSlug`.
- `AgentTriggerDelivery` — per-event dedup via unique `(trigger_id, delivery_id)`.
- Generic webhook path: `POST /internal/webhooks/nango` → `dispatchWebhookEvent`
  (`internal/handler/nango_webhooks_dispatch.go`) → event inference → asynq
  `AgentTriggerDispatchTask` → match triggers by `(org, connection, enabled, webhook,
  trigger_keys && event_keys)` → conditions → `deliver` → `compileMessage` →
  `findOrCreateTriggerSession` → runtime.
- Slack-style dedicated path: `handleSlackForward` / `handleSlackReactionForward` →
  `slackworkflow.Claim*` (event-ledger dedup) → dedicated asynq task → context fetch →
  hand-crafted prompt → shared session/deliver flow.
- Deterministic sessions: UUIDv5 of a resource key; lookup by
  `(org, agent, channel, source, source_resource_key, status=active)`.
- Install templates: `global/triggers/<slug>/` (`trigger.json` + `instructions.md`), loaded by
  `internal/automationcatalog`, served to the install UI. Currently only `slack-reaction`.
- `/v1/triggers` CRUD (`internal/handler/triggers.go`) — currently hard-restricted to
  `provider=slack` + `trigger_key=reaction_added`. Each new curated trigger relaxes this
  deliberately, one template at a time.
- Event catalog `internal/mcp/catalog/providers/*.triggers.json` — still the runtime authority
  for the *generic* dispatch path (`HasTriggers`, key validation, refs, resource-key templates).
  Being phased out in favor of `global/triggers/` templates + dedicated delivery code; new
  curated triggers should not extend it unless they ride the generic dispatcher.

## Core platform changes (shared across providers)

### 1. Resource-scoped sessions

All events on the same external resource (issue, PR) must land in the same session, regardless
of which trigger fired. Change `stableTriggerSessionID` (`internal/tasks/agent_trigger_conversation.go`)
to drop the trigger ID from the hash: key on `(channelID, resourceKey)` (org/agent are already
part of the session lookup). Resource keys are canonical per provider:

- GitHub: `github:<owner/repo>:issue:<number>` — PRs are issues in GitHub's model, so PR
  conversation comments and issue comments unify naturally.
- Linear (agent events): `linear:<organizationId>:agent-session:<agentSessionId>` — Linear's
  agent sessions already provide per-issue continuity.

Payoff: human-in-the-loop approval is free. The agent comments a plan and ends its turn; the
approving reply arrives as a new event on the same resource → same session → the agent continues
with full context. The conversation is the state machine.

Also extend `SessionMessageQueue` arbitration (currently Slack-only,
`prepareSlackMessageIntent`) to the trigger delivery path, so an event arriving mid-turn queues
instead of colliding.

### 2. Loop protection

Dispatch/delivery-layer actor filter — never prompt-level, since a prompt can't be trusted not
to reply to itself:

- GitHub: skip when `sender.login` is the Hivy app bot (`<slug>[bot]`) — generalize
  `isHivyIdentityValue` (`internal/tasks/agent_trigger_filters.go`) from its check-suite special
  case into a first-class filter.
- Linear: skip when the actor is the app user (`appUserId` / OAuth client actor).
- Backstop: per-resource circuit breaker (max N trigger-initiated turns per resource per hour).

### 3. Assignment model (resource → agent)

Reuse external `Channel` rows + `DefaultAgentID`, exactly like Slack channels:

- GitHub: one channel per repo (`external_resource_type=github_repo`, key `owner/repo`).
- Linear: team → agent primary (every issue has exactly one team), project as optional override.

Resolution order mirrors Slack: existing session's agent > trigger's agent > channel
`DefaultAgentID` > org default agent. Side benefit: repo/team channels appear in the sidebar and
collect all agent activity for that resource.

## Trigger catalog

### GitHub (via generic dispatch — inference already works end-to-end)

1. **Mentioned on an issue or PR** ← FIRST IMPLEMENTATION, see below.
2. **New issue opened** — triage playbook (reproduce, comment analysis, optionally label).
   Default install is label-gated (`hivy` label) to avoid agent runs on every issue of a busy repo.
3. **Review activity on Hivy's PR** (`pull_request_review` changes-requested + review comments) —
   address feedback, push, reply. Resource-scoped sessions route this into the session that
   opened the PR.
4. **CI failed on Hivy's PR** (`check_suite.completed`, failure) — the Hivy-authored filter for
   this event already exists. Read logs, fix, push.

### Linear

Primary path: **`AgentSessionEvent` webhooks** (Linear's recommended agent model — our app is
already registered mentionable/assignable). Linear creates an agent session when Hivy is
mentioned or delegated an issue; `action: "created"` for the first event, `action: "prompted"`
for every subsequent user message, with a `promptContext` blob (issue details, comment thread,
guidance) included. This IS our resource-scoped session model implemented on Linear's side.
Build a dedicated pipeline (decoder + workflow + task, mirroring the Slack files), not the
generic dispatcher.

1. **Issue assigned/delegated to Hivy** (`AgentSessionEvent created` via delegation) — flagship
   playbook: comment ack + plan → move to In Progress → implement → open PR (issue URL in body) →
   comment PR link → move to In Review. Ship autonomous and approval-gated instruction variants.
2. **Reply in a Hivy session** (`AgentSessionEvent prompted`) — pure continuation: feedback,
   approvals, questions land in the existing session.
3. **Mentioned, not assigned** (`AgentSessionEvent created` via mention) — conversational
   playbook: investigate and reply; no state changes; self-assign only if explicitly asked.
4. **Issue moved to a configured status** — classic data-change webhook through the generic
   dispatcher. Requires the Linear inference fix (below). Optional pull-based pickup per team.

### Webhook plumbing facts (researched 2026-07-03)

- Nango forwards provider webhooks as `{from, type:"forward", connectionId, providerConfigKey,
  payload}` **when it can attribute a connection**; unattributable webhooks are forwarded as the
  raw unwrapped payload. Linear agent webhooks are app-scoped, so handle both shapes and fall
  back to resolving the workspace from `payload.organizationId`.
- Linear webhook categories must be enabled on the OAuth app: "Agent session events" (and
  optionally "App notifications" for the legacy `AppUserNotification` granular events:
  `issueAssignedToYou`, `issueCommentMention`, `issueMention`, `issueNewComment`).
- Trust Nango's `X-Nango-Hmac-Sha256` (already verified in `NangoWebhookHandler`); Linear's own
  signature does not survive forwarding.
- **Known gap:** classic Linear webhooks are silently dropped today —
  `inferEventFromHeaders` (`internal/handler/nango_webhooks_infer.go`) only handles GitHub.
  Fix: for Linear, `eventKey = payload.type + "." + payload.action` (e.g. `Issue.update`).
- Empirical validation step before building Linear: mention Hivy in a test workspace and log
  what lands on `/internal/webhooks/nango` (wrapped vs raw, exact payload shapes).

## Verified provider contracts (GitHub, 2026-07-03)

Strict rule for all trigger work: **no contract assumptions** — every payload field,
endpoint behavior, and identity convention is verified against official provider docs
(and the octokit canonical fixtures in `internal/trigger/dispatch/testdata/github/`)
before code is written against it. Tests pin the parsers to the canonical fixtures so
upstream contract drift fails in CI, not production.

Verified against docs.github.com (webhook payload reference, REST issues/comments,
apps installations, delivery headers):

- `issue_comment` fires for comments on both issues and PR conversations;
  `issue.pull_request` presence is the documented way to detect the PR case.
- `issue.body` / `pull_request.body` are nullable; `comment.body` not documented
  nullable but handled defensively.
- App bot identity: `<app-slug>[bot]` login (user.type `Bot`). Users mention the app by
  typing `@<slug>` — there is NO mention webhook and NO autocomplete for app bots, so
  detection is body parsing and users must type the handle exactly.
- `performed_via_github_app` is on comment objects but is NOT a safe bot signal:
  human comments posted via GitHub Mobile set it too. Bot detection uses the `[bot]`
  login suffix + `user.type`.
- `GET /repos/{o}/{r}/issues/{n}/comments`: JSON array, **ascending by id only** — no
  `sort`/`direction` on the per-issue endpoint; `per_page` max 100. Recent context
  therefore requires fetching the LAST page; `issue.comments` in the webhook payload
  gives the count to compute it (implemented in `fetchGitHubMentionComments`).
- `X-GitHub-Delivery` GUID is **constant across redeliveries** — ideal dedup key — but
  Nango's forwarding does not document that original provider headers survive. Dedup
  therefore falls back to payload-derived keys for exactly-once events
  (`comment.id` / `issue.id` / `pull_request.id` for created/opened;
  `stableGitHubDeliveryID`), then random. Recurring actions (edited, labeled,
  synchronize) must never get payload-derived keys.
- App configuration required: subscribe to **Issues, Issue comment, Pull request**
  webhook events; permissions **Issues: read+write** (receive + comment back),
  **Pull requests: read** (write if playbooks push/comment on PR-specific surfaces).
- `GET /installation/repositories`: `{total_count, repositories[]}`, `per_page` max
  100. KNOWN LIMITATION: generic resource discovery fetches one page, so the repo
  picker truncates for installations with >100 repos.

Empirical validation still pending (do before calling it production-done): send a real
mention through Nango and confirm (a) whether `x-github-delivery`/`x-github-event`
headers arrive in the forward (the `{data, headers}` unwrap suggests they do, but it is
undocumented), (b) the app's webhook subscriptions actually include issue_comment.

## First implementation: GitHub mentions on issues and PRs

Trigger fires when the Hivy GitHub App is @mentioned in:
- an issue or PR conversation comment (`issue_comment.created` — covers both, GitHub treats PR
  conversation comments as issue comments),
- a new issue body (`issues.opened`),
- a new PR body (`pull_request.opened`).

Design (Slack-reaction pattern, dedicated delivery code):

- **Template** `global/triggers/github-mention/`: `trigger.json`
  (provider github-app, key `mention`, required plugin github) + `instructions.md` default
  playbook: respond in-place via a comment; on a PR you may push to the branch if asked; never
  open new PRs unprompted; be concise; never react with emoji unless asked.
- **Install** via `/v1/triggers`: connection + repo (`external_resource_key = owner/repo`,
  auto-provision the repo channel) + agent + instructions. One trigger row per repo, mirroring
  one-per-Slack-channel.
- **Detection**: in the GitHub branch of the Nango webhook path, for the three event keys, check
  the relevant body for `@<app-slug>` (mention detection is ours — GitHub has no mention
  webhook). App slug derives from the same identity source as `isHivyIdentityValue`.
- **Loop protection**: skip when `sender.login` is the app bot.
- **Dedup**: `agent_trigger_deliveries` unique `(trigger_id, delivery_id)` with the GitHub
  delivery ID.
- **Prompt**: hand-crafted — trigger instructions + repo/issue/PR metadata + the mentioning
  comment + recent thread context fetched via the Nango proxy (like
  `FetchReactionMessageContext` does for Slack).
- **Session**: resource-scoped — `github:<owner/repo>:issue:<n>`; follow-up mentions on the same
  issue/PR continue the session.
- **UI**: `GithubMentionInstallForm` in `_trigger-install-form.tsx` — connection → repo picker →
  agent → instructions.

## Build order

1. ✅ Plan + research (this doc).
2. GitHub mentions end-to-end (backend pipeline, template, API, UI, tests).
3. Resource-scoped session keying generalization + trigger-path message queueing.
4. Actor self-event filter as a shared dispatch filter + circuit breaker.
5. Repo→agent / team→agent assignment channels + UI.
6. Remaining GitHub triggers (new issue, review-on-Hivy-PR, CI-failed).
7. Linear: enable webhook categories, capture real payloads, build the AgentSessionEvent
   pipeline, curated defs 1–3; Linear inference fix + status-moved trigger.
8. Playbooks-as-skills refactor once instruction templates stabilize.
