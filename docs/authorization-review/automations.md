# Authorization — Automations (Triggers, Schedules, Webhooks)

## 1. Overview

"Automations" is the surface that makes an agent run **without a human sending a message**. It has three concrete resource types, all backed by the same `agent_triggers` / `agent_schedules` tables and all requiring the same core binding — **(agent × channel × instruction)**:

- **Provider triggers** (`trigger_type = "webhook"`) — fire an agent when an event happens in a connected app (Slack reaction, GitHub issue/PR mention). Config: agent, connection, external resource (repo/Slack channel), the Hivy channel the run happens in, instructions. Curated by a template registry (`triggers_templates.go`).
- **HTTP triggers / webhooks** (`trigger_type = "http"`) — fire an agent from an inbound HTTP `POST /incoming/triggers/{id}`. Config: agent, channel, instructions, optional shared secret (bcrypt-hashed).
- **Schedules / cron** (`agent_schedules`) — fire an agent on a cron expression or interval. Config: agent, optional channel, task prompt, cadence. Also creatable/manageable by agents at runtime via the `cron` MCP tool.

Principals who interact: **Org Admins/Owners** (only role that can create/edit/delete via the REST API today), **Members** (can read visible automations and reach the frontend create forms, but their writes are 403'd), the **HTTP caller** holding a trigger URL + optional secret (non-human, secret axis), and the **automated actor** — an agent creating cron jobs through the `cron` MCP tool on behalf of a human (`_hivy_actor_user_id`).

The crux for this feature: **creating a trigger/schedule is composition + config** — it binds an existing org agent to a channel to run automatically. Per the baseline model that is a **team-member action** (a member of the target channel's team, or Org Admin), but it is currently gated at **Org Admin**. An automation also has unusually high blast radius (an agent that opens PRs or runs unattended on cron), which pulls the other way.

## 2. Backend endpoint inventory

Route block in `cmd/server/serve_routes_v1.go`. The whole agents/triggers/schedules group sits under `RequireAPIKeyScopeOrJWT("agents")` (~line 202); reads are directly under it, writes are in an inner `r.Use(middleware.RequireOrgAdmin(database))` group (~line 250).

| Method | Path | Handler (file:line) | Mutates/Reads | CURRENT gate | Correct? |
|---|---|---|---|---|---|
| GET | `/v1/triggers` | `triggers_read.go` `List` | Reads | `RequireAPIKeyScopeOrJWT("agents")` + in-handler visibility filter | ✅ (just-shipped visibility fix — do not re-flag) |
| GET | `/v1/triggers/{id}` | `triggers_read.go` `Get` | Reads | same + `actorCanAccessTrigger` | ✅ |
| POST | `/v1/triggers` | `triggers.go:100` `Create` → `create`/`createHTTP` | Mutates | `RequireOrgAdmin` + `CanUseChannelID` + `channelagents.Assigned` | ⚠️ over-gated vs model (see §4) |
| PATCH | `/v1/triggers/{id}` | `triggers_update.go:51` `Update` | Mutates | `RequireOrgAdmin` + channel/agent re-check | ⚠️ over-gated |
| DELETE | `/v1/triggers/{id}` | `triggers_delete.go:26` `Delete` | Mutates | `RequireOrgAdmin` + `actorCanAccessTrigger` | ⚠️ over-gated |
| GET | `/v1/schedules` | `schedules.go` `List` | Reads | `RequireAPIKeyScopeOrJWT("agents")` + `actorCanAccessSchedule` filter | ✅ |
| GET | `/v1/schedules/{id}` | `schedules.go` `Get` | Reads | same + access check | ✅ |
| POST | `/v1/schedules` | `schedules_write.go:31` `Create` | Mutates | `RequireOrgAdmin` + `CanUseChannelID` + `channelagents.Assigned` | ⚠️ over-gated |
| PATCH | `/v1/schedules/{id}` | `schedules_write.go:130` `Update` | Mutates | `RequireOrgAdmin` + `actorCanAccessSchedule` + channel re-check | ⚠️ over-gated |
| DELETE | `/v1/schedules/{id}` | `schedules_write.go:~215` `Delete` | Mutates | `RequireOrgAdmin` + `actorCanAccessSchedule` | ⚠️ over-gated |
| POST | `/incoming/triggers/{triggerID}` | `http_trigger.go:64` `Handle` | Mutates (enqueues an agent run) | **NONE** — public route (`serve.go:72`); trigger UUID = bearer, optional bcrypt shared secret | ⚠️ secret axis — see §4 |
| — | `cron` MCP tool | `internal/mcpserver/cron_tool.go` + `cron_tool_access.go` | Mutates schedules | Automated-actor axis: calling agent, `_hivy_actor_user_id` gates `channel_id` via `CanUseChannelID`; org-manager/nil actor unrestricted | ✅ (just-shipped visibility/actor fix — do not re-flag) |
| GET | `/v1/agents/{id}/trigger-deliveries[/{id}]` | `trigger_deliveries.go` | Reads | `RequireAPIKeyScopeOrJWT("agents")` + visibility | ✅ (just-shipped read fix — do not re-flag) |
| GET | `/v1/catalog/integrations/{id}/triggers` | `actions_triggers.go` `ListTriggers` | Reads (static catalog) | public catalog route | ✅ |

### Key enforcement facts verified

- **Write gate is JWT-org-admin ONLY, and excludes API keys.** `RequireOrgAdmin` (`internal/middleware/auth.go:144`) hard-requires `claims.UserID` + an `org_memberships` row with role `owner|admin`. An API key (no UserID) is 403'd even though the outer scope gate `RequireAPIKeyScopeOrJWT("agents")` let it reach the group. So **no API key can create a trigger or schedule** — this is inconsistent with credentials/tokens/database-integrations, which use `RequireOrgAdminOrAPIKey`. (Flag, §4.)
- **Install validation is correct and defense-in-depth.** Even though only admins pass the gate, both create paths still verify the admin `CanUseChannelID(channel)` AND that the agent is `channelagents.Assigned` to that channel (`triggers_helpers.go` `resolveProviderTriggerChannel`/`createHTTP`; `schedules_write.go` `Create`). Provider triggers additionally require the agent to hold the plugin/connection the template's playbook depends on (`validateTriggerAgentPlugin` → `connectionaccess.ResolveAgentProviderAny`). So an installed trigger genuinely can fire (agent is usable in channel + has tooling). This is the model's "agent usable in target channel" requirement, correctly enforced — it just sits behind an org-admin gate rather than the team-membership gate the model calls for.
- **Schedule create allows an empty channel_id** → run defaults to the org system channel (`agentschedule.Create`), which skips the channel-binding check entirely. Same for the cron MCP tool (`channel_id` "Defaults to the org's system channel"). For the REST path this is admin-only so low risk; for the MCP path the actor gate only bites when a `channel_id` IS supplied — a null channel run lands in the system channel unchecked.
- **HTTP-trigger firing is a pure secret axis.** `POST /incoming/triggers/{id}` has no auth middleware. The trigger's unguessable UUID is the bearer token; a shared secret is *optional*. If no `secret_key` was set at create time, **anyone who learns the URL can fire the agent** (subject to redaction of sensitive payload keys). The URL is returned in trigger responses (`httpTriggerWebhookURL`).

## 3. Frontend screens & actions

Directory: `apps/web/app/w/(chat)/automations/`. **No screen or action is role-gated.** The pattern `activeOrg?.role === "owner" || "admin"` exists elsewhere (e.g. `settings/teams/page.tsx:24`) but is absent here, and the sidebar "Automations" entry (`_components/sidebar.tsx:182`) is shown to every member.

| Screen (path) | Action | Calls | UI gated by role today? | Should be |
|---|---|---|---|---|
| `/w/automations` (`page.tsx`) | List connections/schedules/webhooks; shows "Install trigger" / "Add schedule" / "Add webhook trigger" buttons | GET `/v1/triggers`, `/v1/schedules` | **No** | Buttons hidden unless the user could create in ≥1 channel (member of that channel's team, or Org Admin); list visible to members (read is visibility-scoped) |
| `/w/automations/triggers/new` | Install provider trigger | POST `/v1/triggers` | **No** — form reachable by members; submit 403s | Gated (hide/redirect for non-eligible) |
| `/w/automations/triggers/[id]` | Edit/delete provider trigger | PATCH/DELETE `/v1/triggers/{id}` | **No** | Gated |
| `/w/automations/schedules/new` | Create schedule | POST `/v1/schedules` | **No** | Gated |
| `/w/automations/schedules/[id]` | Edit/pause/cancel schedule | PATCH/DELETE `/v1/schedules/{id}` | **No** | Gated |
| `/w/automations/webhooks/new` | Create HTTP trigger + optional secret | POST `/v1/triggers` (`trigger_type:"http"`) | **No** | Gated |
| `/w/automations/webhooks/[id]` | Edit/delete HTTP trigger; view fire URL | PATCH/DELETE `/v1/triggers/{id}` | **No** | Gated |
| `/w/automations/[id]/install` | GitHub App install flow for a trigger | trigger install endpoints | **No** | Gated |

Backend is authoritative (member writes get 403), so this is a **LOW** severity UX mismatch, not a privilege escalation — but it is a real dead-end: a member fills out the whole install form and is rejected only on submit.

## 4. Ambiguities & lapses (ranked)

1. **HTTP triggers with no shared secret = unauthenticated agent invocation (HIGH — secret axis).** `POST /incoming/triggers/{id}` is public; `secret_key` is optional. A trigger created without a secret can be fired by anyone who obtains the URL (it appears in API responses, browser history, logs, front-end state). Blast radius = one full agent run in the bound channel with attacker-controlled payload (sensitive top-level keys are redacted, but the agent still acts on the body). The UUID gives obscurity, not authorization. **Recommend: make a shared secret mandatory** for HTTP triggers (or require signed requests), and treat the fire URL as a secret in the UI.

2. **Create/edit/delete over-gated vs baseline: should be a team-member action (MEDIUM).** Per the org-vs-team principle, binding an *existing* org agent into a *channel* is composition → **member of that channel's team, or Org Admin** (`CanManageTeamResource(channel.TeamID) = actor.IsOrgManager() || actor.IsTeamMember(channel.TeamID)`). Today it's Org Admin only, so it IS over-gated. The install code already computes exactly the predicate needed alongside it (`CanUseChannelID` + `channelagents.Assigned`), so it is *ready* to relax to team-membership — and no migration/role column is required, since `team_members` membership is the grant (see §6). Counterweight: automations have high blast radius (unattended cron, PR-opening agents), which is a legitimate reason one might keep the bar at Org Admin. This is the core decision for the operator (§7).

3. **API keys cannot create automations, unlike peer resources (MEDIUM — inconsistency).** The write group uses `RequireOrgAdmin`, which rejects API keys, while credentials/tokens/database-integrations use `RequireOrgAdminOrAPIKey`. An automation platform that can't be provisioned by an API key is an odd gap; decide deliberately whether key-driven automation setup should be allowed and, if so, switch to `RequireOrgAdminOrAPIKey`.

4. **Null-channel schedules bypass channel binding (LOW→MEDIUM).** A schedule (REST or cron MCP tool) with no `channel_id` runs in the org **system channel**, skipping `CanUseChannelID`/`Assigned`. REST path is admin-only (low risk). The cron MCP path's actor gate (`enforceActorCronSchedule`) only fires when a `channel_id` is supplied and the actor is a non-manager human — a null-channel job created on behalf of a member lands in the system channel unchecked. Worth confirming the system channel's own access model isn't a side door.

5. **Frontend shows create actions to everyone (LOW).** Dead-end forms for members; no escalation because backend 403s. Mirror the eventual backend gate in the UI.

## 5. Recommendation

Per action, target principal + mechanism:

- **Create/Update/Delete trigger (provider + HTTP)** and **Create/Update/Delete schedule** → **member of the target channel's team, or Org Admin.** Enforcement: replace the blanket `RequireOrgAdmin` route group with a per-request predicate evaluated in-handler (the channel isn't known at middleware time). Gate as `CanManageTeamResource(channel.TeamID)` = `actor.IsOrgManager() || actor.IsTeamMember(channel.TeamID)` **AND** the existing `CanUseChannelID` + `channelagents.Assigned` checks (already present — keep them). **No migration or role column is needed** — `team_members` membership is the grant, and `CanManageTeamResource` reads it. The only judgment call is blast radius (see §7): if the operator wants a higher bar than ordinary channel↔agent assignment, keep the `RequireOrgAdmin` gate instead — but that is a deliberate policy choice, not a limitation of the model.
- **Fire HTTP trigger** (`POST /incoming/triggers/{id}`) → **secret axis, not human role.** Make `secret_key` **required** at create time; reject creation without one (or auto-generate and surface once). Keep the bcrypt compare. Optionally add HMAC-signature verification for providers that support it. Surface the URL as a reveal-once secret in the UI.
- **Read triggers/schedules/deliveries** → **Member, visibility-scoped** (already implemented — the just-shipped `actorCanAccessTrigger`/`actorCanAccessSchedule` + visibility subqueries). No change.
- **`cron` MCP tool** → **automated-actor axis** (already implemented — `_hivy_actor_user_id` gates `channel_id`). No change; just close the null-channel/system-channel gap in #4 if the system channel is broadly reachable.
- **API-key writes** → operator decision; if yes, `RequireOrgAdminOrAPIKey` on the write group to match peer resources.
- **Frontend** → gate the "Install/Add" buttons and `.../new` + `.../[id]` edit routes on the same predicate (`CanManageTeamResource` of the target channel's team, i.e. org-manager or team member). Never the only gate.

## 6. Deviations from the baseline model

- **The model is followable immediately — no migration blocks it.** The model's owner for automation create/edit is a member of the channel's team (or Org Admin), enforced by `CanManageTeamResource(channel.TeamID)`. Team membership already lives in `team_members`, so this needs no new role column or migration; the current Org Admin gate is an over-gate that can be relaxed now. (We considered introducing a distinct team-admin tier to sit between member and org-admin and rejected it — the baseline model has no such role.) The only reason NOT to relax it is blast radius, a policy call, not a technical gap.
- **Automations have no single "resource creation vs composition" answer.** Creating a trigger isn't creating a new *resource type* (the agent already exists) — it's composition, which argues for a team-member action. But the composed thing runs unattended with real blast radius, which argues for a higher bar than ordinary channel↔agent assignment. This feature legitimately straddles the org/team line; flagged for the operator rather than forced into one bucket.
- **HTTP-trigger firing is orthogonal to the human role model** — it's a bearer-secret axis (`_MODEL.md`'s "Automated actor / API key" dimension), correctly kept separate.

## 7. Open questions for the operator

1. **Should automation create/manage be a team-member action (composition, per the model) or stay Org Admin because of unattended blast radius?** This is the central decision, and it is purely a policy call — the model says composition-level authority (member of the channel's team, via `CanManageTeamResource`) is enough and requires no migration to adopt. Given an automation can open PRs / run unattended, is that team-membership bar acceptable, or is the blast radius reason to keep the higher Org Admin bar?
2. **Should HTTP-trigger shared secrets be mandatory?** (Recommended yes.) If yes, do we break/deny existing secret-less triggers, or grandfather them with a warning?
3. **Should API keys be able to create triggers/schedules** (switch write gate to `RequireOrgAdminOrAPIKey`), or is automation setup deliberately human-admin-only?
4. **Null-channel automations** land in the org system channel — is that channel's access model acceptable as the default target, or should a channel always be required?
