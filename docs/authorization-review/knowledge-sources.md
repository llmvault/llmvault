# Authorization — Knowledge Sources (RAG)

## 1. Overview

A **knowledge source** (RAG source) is an org-level ingestion unit: a website crawl or an integration-backed connector (Notion, Linear, GitHub, Slack) that pulls potentially sensitive org content into the vector store (Qdrant). Content is retrieved via `POST /v1/rag/search` and via the agent MCP tool `search_knowledge_base`.

Resources involved:
- `rag_sources` — the org-owned source rows (org-scoped, `org_id`). This is a **catalog resource** in the _MODEL.md sense.
- `channel_rag_sources` — the grant join table (`org_id`, `channel_id`, `rag_source_id`) that says "agents in this channel may search this source." This is **composition** (channel ↔ org-resource).
- `connections` / integrations — the credential a source ingests through (org-scoped, validated on create).

Principals:
- **Org Admin/Owner** — create/edit/delete sources and grant them to channels. Per the operator's policy, **both** create and channel-grant are Org-Admin-only.
- **Member / channel owner** — should only *use* granted sources in channels they can reach.
- **API key / automated actor** — org-wide reach, gated by scope, not human role.

Retrieval scoping (`/v1/rag/search`) was just fixed to be channel-scoped for non-manager members (deny-by-default; a member only reaches sources granted to channels they can use). **Per instructions this read-scoping is not re-flagged as a leak** — but §4 shows a *write* path that lets a member widen their own usable set, defeating that fix.

## 2. Backend endpoint inventory

Reads mount under `/rag` with `ResolveUser`; mutations are wrapped in a `RequireOrgAdmin` group (`cmd/server/serve_routes_v1_rag.go:23-48`). The channel-side grant lives under the `channels` scope group (`cmd/server/serve_routes_v1.go:146-169`).

| Method | Path | Handler (file:line) | Mutates/Reads | CURRENT gate | Correct? |
|---|---|---|---|---|---|
| GET | `/v1/rag/integrations` | `rag_integrations.go` ListIntegrations | Reads | any org member (ResolveUser) | ✅ |
| GET | `/v1/rag/connections/{cid}/scopes` | `rag_sources_scope.go:38` ListConnectionScopes | Reads | any org member | ⚠️ leaks connection scope metadata to members |
| GET | `/v1/rag/sources` | `rag_sources_read.go:28` List | Reads | any org member, **org-wide (no channel scope)** | ⚠️ metadata leak (see §4-4) |
| GET | `/v1/rag/sources/{id}` | `rag_sources_read.go` Get | Reads | any org member, org-scoped | ⚠️ same class |
| GET | `/v1/rag/sources/{id}/attempts` `/{attempt_id}` | `rag_sources_attempts.go:26,82` | Reads | any org member | ⚠️ ingest logs visible to members |
| GET | `/v1/rag/sources/{id}/channels` | `rag_sources_channels.go:36` GetSourceChannels | Reads | member → visible channels only; manager/APIkey → all (`actorSeesOrgWide`) | ✅ |
| GET | `/v1/rag/sources/{id}/documents` | `rag_sources_documents.go:47` (search handler) | Reads | any org member, org-scoped | ⚠️ doc listing not channel-scoped |
| POST | `/v1/rag/search` | `rag_search.go:62` Search | Reads | member → channel-scoped usable sources; manager/APIkey → org-wide | ✅ (just-shipped fix; not re-flagged) |
| POST | `/v1/rag/sources` | `rag_sources_create.go:42` Create | Mutates | **RequireOrgAdmin** | ✅ catalog create = org admin |
| PATCH | `/v1/rag/sources/{id}` | `rag_sources_update.go` Update | Mutates | RequireOrgAdmin | ✅ |
| DELETE | `/v1/rag/sources/{id}` | `rag_sources_delete.go:23` Delete | Mutates | RequireOrgAdmin | ✅ |
| PUT | `/v1/rag/sources/{id}/channels` | `rag_sources_channels.go:98` SetSourceChannels | Mutates (grant) | **RequireOrgAdmin**; validates channels ∈ org only | ✅ grant = org admin (matches operator policy) |
| POST | `/v1/rag/sources/{id}/sync` | `rag_sources_sync.go` TriggerSync | Mutates | RequireOrgAdmin | ✅ |
| POST | `/v1/rag/sources/{id}/prune` | `rag_sources.go` TriggerPrune | Mutates | RequireOrgAdmin | ✅ |
| POST | `/v1/rag/website/discover-sections` | `rag_website_discovery.go` | Mutates (crawl probe) | RequireOrgAdmin | ✅ |
| GET | `/v1/channels/{id}/rag-sources` | `channels_rag_sources.go:41` ListChannelRAGSources | Reads | `authorizeChannel(view)` — usable channel | ✅ |
| **PUT** | **`/v1/channels/{id}/rag-sources`** | **`channels_rag_sources.go:77` SetChannelRAGSources** | **Mutates (grant)** | **`authorizeChannel(manage)` = `canManageChannel` = apiKey \|\| org manager \|\| `channel_members.role=="owner"`** | **❌ non-admin channel owner can grant any org source** |

The two grant paths write the **same** `channel_rag_sources` rows but enforce **different** gates. The source-side (`PUT /rag/sources/{id}/channels`) is org-admin-only; the channel-side (`PUT /channels/{id}/rag-sources`) accepts a channel "owner." This split-brain is the core defect.

## 3. Frontend screens & actions

| Screen (path) | Action | Calls | UI gated by role today? | Should be |
|---|---|---|---|---|
| `app/w/settings/knowledge/page.tsx` | List sources | GET `/rag/sources` | **No** — nav shows to all members (`settings/layout.tsx`, `_components/nav.tsx` have no role filter) | Org-admin-only screen |
| `app/w/settings/knowledge/new/page.tsx` | Create source, then grant to channels | POST `/rag/sources` → PUT `/rag/sources/{id}/channels` | No UI gate; backend 403s non-admins | Org Admin |
| `app/w/settings/knowledge/[id]/edit/page.tsx` | Edit source + channel set | PATCH `/rag/sources/{id}` + PUT `/rag/sources/{id}/channels` | No UI gate; backend 403s non-admins | Org Admin |
| `app/w/settings/knowledge/[id]/documents/page.tsx` | View indexed docs/attempts | GET documents/attempts | No | Org Admin |
| `app/w/settings/channels/[id]/page.tsx` → `_knowledge-sources-tab.tsx` | Toggle any org source on/off for a channel | **PUT `/channels/{id}/rag-sources`** | **No UI gate** — tab shown to any channel viewer; toggles succeed for channel owners | Org Admin |

The channel "Knowledge sources" tab lists **all** org sources (`GET /rag/sources`, page_size 100) and lets a channel owner switch any of them on for the channel. This is the UI surface of the §4-1 escalation, and it also exposes every source name to any member who can open a channel's settings.

## 4. Ambiguities & lapses (ranked)

**1. HIGH → effectively CRITICAL: a non-admin can grant any org knowledge source to a channel and then read its content.**
`POST /channels` is open to any JWT under the `channels` scope group; the creator is written as `channel_members.role = "owner"` (`channels_mutation.go:120-124`). `PUT /channels/{id}/rag-sources` gates on `canManageChannel` (`channels_auth.go:58-60`) = `apiKey || isOrgManager || memberRole=="owner"`. So any member can: (a) create a channel → become its owner, (b) grant *any* org source to it (`SetChannelRAGSources` validates only `source ∈ org`, `channels_rag_sources.go:107-118`), (c) call `/rag/search`, where `usableRagSourceIDs` (`channel_access.go:104`) now returns that source because it is granted to a channel they can use. This **defeats the just-shipped search read-scoping through a write path**, exposing Notion/Linear/GitHub/Slack/website content the member was never granted. Blast radius: full RAG corpus, any member. This directly **contradicts the operator's claim** that "granting to channels can only be done by org admins" — true for the source-side route, false for the channel-side route.

**2. RESOLVED (not a lapse): source-side grant is org-admin-only, which matches policy.** `PUT /rag/sources/{id}/channels` requires org admin. Per the operator's explicit instruction, granting an existing source to a channel is an **Org Admin** action — so this route is already correct. It centralizes composition on org admins by design; that is the intended policy, not a gap. The defect to fix is the *other* grant route (§4-1), which does **not** enforce the org-admin gate.

**3. N/A: no team boundary needed on org-admin grants.** `SetSourceChannels` lets an org admin grant a source to *any* channel in the org regardless of team. That is correct and intended: grants are Org-Admin-only and an org admin is authorized across every team, so there is no team principal to scope against.

**4. MEDIUM: source metadata reads are org-wide, not channel-scoped.** `GET /rag/sources`, `/{id}`, `/attempts`, `/documents` return all org sources/ingest logs to any member. This is a *metadata/doc* leak distinct from the search *content* leak that was fixed — a member can enumerate every source name, config (scope selections), connection linkage, and per-run error logs. Lower severity than §4-1 but should be scoped to the member's usable sources (reuse `usableRagSourceIDs`) or gated to admins.

**5. LOW: frontend does not mirror the backend gate.** The Knowledge settings section and the channel Knowledge tab render for all members with no role check, while the org-admin backend returns 403. Cosmetic today (backend is authoritative) but the channel tab is the live surface for §4-1, and the `teams/page.tsx` pattern (`activeOrg?.role === "owner"|"admin"`) already exists to gate it.

## 5. Recommendation

Per-action target principal + mechanism:

| Action | Target principal | Enforcement |
|---|---|---|
| Create / update / delete source; sync / prune; website discover | **Org Admin** | Keep `RequireOrgAdmin` group — already correct. |
| Grant existing source ↔ channel (both routes) | **Org Admin** | Single shared predicate `IsOrgManager()`; **remove the `channel_members.role=="owner"` shortcut** for this action. |
| Read source metadata / attempts / documents | Org Admin, or member scoped to usable sources | Scope reads with `usableRagSourceIDs` / `actorSeesOrgWide`, mirroring `/search`. |
| Search / use in channel | Member of a channel the source is granted to | Already correct (`/search` + MCP tool). |

Concrete steps, in priority order:

1. **Close §4-1 now (no schema change).** Change `SetChannelRAGSources` so granting an org resource is **not** available to a plain channel owner. Gate `PUT /channels/{id}/rag-sources` behind `IsOrgManager()` (org owner/admin) — the same org-admin check the source-side route already uses — so both grant paths require org admin. This immediately eliminates the escalation. **Unify both grant paths on one predicate (`IsOrgManager()`)** so they can't diverge again. Note channel-member "owner" should still manage channel membership/agents per existing rules — only the *org-resource grant* action moves up to Org Admin.

2. **Scope the metadata reads (§4-4).** Apply the `actorSeesOrgWide` / `usableRagSourceIDs` pattern to `List`, `Get`, `ListAttempts`, `ListDocuments` so non-managers see only sources reachable through their usable channels.

3. **Mirror gates in the frontend.** Hide the Knowledge settings nav section and disable the channel Knowledge-tab toggles unless `activeOrg.role ∈ {owner, admin}`. Reuse the existing `activeOrg?.role` gate from `settings/teams/page.tsx`. Backend stays authoritative.

**On the operator's "org-admin only for both":** this is the confirmed policy and this doc endorses it — both source creation/edit/delete and channel-grant are **Org-Admin-only**. Create, edit, delete, sync, prune, and the source-side grant route already enforce it. The single live hole is the channel-side grant route (`PUT /channels/{id}/rag-sources`), which today accepts a channel owner via `canManageChannel`; the fix is to enforce the same org-admin gate there. (We considered and rejected delegating grants to team members/team-admins — grants stay Org Admin per operator policy.)

## 6. Deviations from the baseline model

- **No deviation on grants.** The baseline deletes the Team Admin role, and per operator policy knowledge-source grants are Org-Admin-only. There is no team principal involved in granting, so questions about team ownership or team-scoped grants do not arise here. Channel-level `channel_members.role="owner"` is a *different, wrong* axis for org-resource grants and must not be used for them — that is the §4-1 hole.
- **Team-less channels are a non-issue for grants.** Because grants require Org Admin (authorized org-wide), it does not matter whether a channel has `team_id IS NULL` — no team-level delegation exists to fall back from.

## 7. Open questions for the operator

1. **Metadata visibility:** should non-admin members see the *list* of source names/configs at all, or only sources granted to channels they can use? (Drives §4-4 scope.)

*(Resolved: channel-grant policy is **Org-Admin-only** for both grant routes — confirmed by the operator, so the earlier "Org Admin only vs Team Admin + Org Admin" and "team owner vs team admin" questions no longer apply. Team-less channels need no special handling since grants are Org-Admin-only.)*
