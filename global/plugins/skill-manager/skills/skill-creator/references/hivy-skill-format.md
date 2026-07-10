# Hivy skill format — the create_skill / update_skill contract

## How a skill reaches an agent

1. You publish a skill into an org plugin with `create_skill`.
2. An org owner/admin enables that plugin for a team (team settings). Every agent on the team then inherits the plugin by default. A user can disable an optional inherited plugin for one agent in Agent details; no MCP tool attaches a plugin directly to an agent.
3. The agent's system prompt lists the skill as one `name: description` line, and `skills_list` returns it.
4. When the agent calls `skill_view(name)`, the platform composes a SKILL.md (generated frontmatter + your content) and **materializes the whole bundle into `.skills/<slug>/` in the agent's workspace**: `SKILL.md` plus every supporting file at its declared path.

Consequences for authoring:

- The reading agent accesses supporting files at `.skills/<slug>/references/...`, `.skills/<slug>/scripts/...` etc. Write instructions with those paths: "run `bash .skills/<slug>/scripts/validate.sh`".
- The `description` is the ONLY thing an agent sees before deciding to load the skill. It must state the triggering situations concretely.
- Frontmatter is generated from your fields (name, description, category, tags). Your `content` must NOT begin with `---` — create_skill rejects it.

## create_skill fields

| Field | Required | Notes |
|---|---|---|
| `plugin_slug` | yes | An org-owned plugin (from `create_org_plugin`). Global catalog plugins are rejected. |
| `name` | yes | Display name, max 120 chars. The slug is generated from it (lowercase, hyphens) and never changes afterward. |
| `description` | yes | Trigger text, max 1024 chars. Start with "Use when…". |
| `content` | yes | SKILL.md body in markdown, no frontmatter, max 256 KB. |
| `human_description` | no | User-facing copy for the UI. |
| `category` | no | Shown in `skills_list`; agents can filter by it. |
| `tags` | no | Lowercase kebab-case. |
| `required_environment_variables` | no | The injected names the skill's instructions read (org variables: `HIVY_ORG_<NAME>`). Surfaced in `skill_view` so agents and users know what must be set. |
| `files` | no | Object of relative path → content. Max 32 files, 256 KB each, 1 MB total including content. |

**File paths** must be clean relative paths under exactly one of: `references/`, `templates/`, `scripts/`, `assets/`. No `..`, no leading `/`, no backslashes. Anything else is rejected — the sandbox materializer enforces the same allow-list.

## update_skill

A true patch: only provided fields change. `files` REPLACES the entire file set when provided — to add one file, resend all of them. The slug never changes even if `name` does. Updating an archived skill republishes it.

## Writing a good description (trigger text)

Bad: `"Helps with sales proposals."` — no agent knows when to load it.

Good: `"Use when the user asks to draft, review, or format a sales proposal or quote — covers the org's proposal template, pricing table format, and approval checklist."`

Name the verbs and nouns a request would contain. If the skill should also trigger proactively ("when you are about to X"), say that too.

## Writing good content

- Open with the skill's contract: what it guarantees when followed.
- Prefer numbered workflows over prose. Put exact commands and API payloads inline.
- Push long material (API references, style guides, large templates) into `references/` files and tell the reader when to open each — the content body should stay loadable at a glance.
- Scripts must be self-contained: use tools preinstalled in the sandbox (see `references/sandbox-environment.md`) or install into the project directory at run time with an explicit step.
- State required env vars early, with the `HIVY_ORG_` prefix, and what to do when they are missing (send the user to workspace settings — do not fail silently).

## Environment variables

Org-level variables are managed by users in workspace settings (tool responses include `environment_settings_url`). A variable saved as `NAME` is injected into every sandbox in the org as `HIVY_ORG_NAME`. Values are write-only — nobody can read them back from the API, and they must never appear in skill content, files, memory, or chat. Skills declare the injected names in `required_environment_variables` and read them from the environment at run time, e.g. `curl -H "Authorization: Bearer $HIVY_ORG_STRIPE_API_KEY" ...`.
