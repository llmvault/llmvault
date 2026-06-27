# Canvas Artifact System Prototype

## Goal

Build a first-party Canvas artifact system for UseHivey agents. This is a new system and is not based on Penpot. Penpot should remain in place until the prototype is tested, then the Penpot-specific code can be deleted and replaced.

The prototype should let per-session sandbox agents create, validate, verify, sync, and list Canvas projects and artifacts. The user-facing app should preview real interactive HTML artifacts and let users attach artifact comments to messages sent to the agent.

## Settled Decisions

### Scope

- Canvas is available only for per-session agents.
- Persisted Canvas projects and artifacts live globally in the org, not inside a session.
- A session can be passed as optional context when the UI wants to filter to artifacts touched by a session or request a live preview from that session's sandbox.
- The artifact workspace in the sandbox is `/workspace/canvas`.
- Agents edit artifact files directly in the sandbox filesystem.
- The control plane stores Canvas projects, artifacts, and artifact file metadata.
- Artifact file contents are stored in S3/object storage, not Postgres.
- No revision model for the prototype.
- No `canvas_artifact_comments` table for the prototype.
- User comments are not written into artifact files.
- User comments are attached to session messages, similar to existing code line comments.

### Supported Artifact Types

Support two artifact types first:

- `web_page`: a responsive HTML page. This can represent a landing page, dashboard, settings page, app screen, or other page-like interface.
- `presentation`: a responsive slide deck made of one or more HTML slide files.

Both types are treated as responsive. The manifest dimensions represent the default viewport shown first, not a hard rendering limit. The frontend can offer desktop, tablet, and mobile view options.

### Sandbox File Layout

Use a simple project/artifact folder structure:

```text
/workspace/canvas/
  projects/
    project-slug/
      project.json
      artifacts/
        artifact-slug/
          artifact.json
          index.html
        deck-slug/
          artifact.json
          slides/
            001.html
            002.html
```

For `web_page`, the primary file is usually `index.html`.

For `presentation`, each slide is an HTML file listed in `artifact.json`.

### Manifest Shape

Use JSON in the sandbox. Project and artifact manifests should already be JSON so the CLI can validate and send them to the backend without a format conversion step. The backend should validate the payload and store canonical JSON in `manifest_json`.

Example `web_page` artifact:

```json
{
  "schema_version": 1,
  "kind": "hivy.canvas.artifact",
  "id": "settings-page-v1",
  "type": "web_page",
  "name": "Settings page",
  "default_viewport": {
    "width": 1440,
    "height": 1200
  },
  "files": [
    {
      "path": "index.html",
      "role": "entrypoint"
    }
  ]
}
```

Example `presentation` artifact:

```json
{
  "schema_version": 1,
  "kind": "hivy.canvas.artifact",
  "id": "investor-deck",
  "type": "presentation",
  "name": "Investor deck",
  "default_viewport": {
    "width": 1920,
    "height": 1080
  },
  "slides": [
    {
      "id": "cover",
      "name": "Cover",
      "file": "slides/001.html"
    },
    {
      "id": "problem",
      "name": "Problem",
      "file": "slides/002.html"
    }
  ]
}
```

### CLI Commands

Add new commands under the `canvas` CLI:

```sh
canvas project create --name "Project name"
canvas project list

canvas artifact create --project <project-id-or-slug> --type web_page --name "Artifact name"
canvas artifact create --project <project-id-or-slug> --type presentation --name "Deck name"
canvas artifact list --project <project-id-or-slug>
canvas artifact validate <artifact-path>
canvas artifact verify <artifact-path>
canvas artifact sync <artifact-path>
```

Expected behavior:

- `project create` calls the control plane and creates a DB-backed project.
- `project list` calls the control plane and lists org-scoped projects visible to the runtime agent.
- `artifact create` scaffolds `/workspace/canvas/projects/.../artifacts/...`.
- `artifact validate` validates JSON manifest shape, paths, supported type, required HTML structure, and required data attributes.
- `artifact verify` performs stronger local static checks against the generated HTML. It should not require Playwright or a browser runtime in the prototype.
- `artifact sync` sends artifact metadata and files to the control plane.
- `artifact list` calls the control plane and lists artifacts for a project.

Runtime CLI auth should follow the existing runtime-secret pattern: `HIVY_CONTROL_PLANE_URL`, `HIVY_AGENT_ID`, and `HIVY_RUNTIME_SECRET`.

### Approved API Endpoints

Use org-scoped public endpoints for the product UI because Canvas projects and artifacts live globally in the org. Use optional `session_id` query parameters only when the UI needs session context, such as filtering to artifacts touched in a session or previewing from an active session sandbox. Use runtime-authenticated internal endpoints for the sandbox CLI and watcher.

#### Public Org Endpoints

These endpoints are called by the web app and use the normal authenticated org context.

```text
GET  /v1/canvas/projects
GET  /v1/canvas/artifacts
GET  /v1/canvas/artifacts/{artifactID}
POST /v1/canvas/artifacts/{artifactID}/preview-url
POST /v1/sessions/{sessionID}/messages
```

Endpoint behavior:

- `GET /v1/canvas/projects` returns org-scoped Canvas projects, including artifact counts and enough metadata for the project picker. It may accept an optional `session_id` query parameter to prioritize or filter projects touched by that session.
- `GET /v1/canvas/artifacts` returns org-scoped artifacts. It should accept optional `project_id`, `project_slug`, and `session_id` query parameters.
- `GET /v1/canvas/artifacts/{artifactID}` returns artifact metadata, manifest JSON, and file metadata. It should not need to return full HTML file content unless the frontend explicitly needs it.
- `POST /v1/canvas/artifacts/{artifactID}/preview-url` returns an opaque iframe URL for direct sandbox rendering. It should accept `session_id` context so the backend can route the preview to that session's active sandbox. The frontend can use the existing read-only sandbox access token flow for this preview path.
- `POST /v1/sessions/{sessionID}/messages` is the existing message endpoint. Extend its payload to accept `artifact_comments` alongside `code_line_comments`.

#### Runtime Agent Endpoints

These endpoints are called by the sandbox CLI and runtime watcher. They are authenticated with `Authorization: Bearer $HIVY_RUNTIME_SECRET` and the `{agentID}` path parameter.

```text
GET  /internal/agents/{agentID}/canvas/projects
POST /internal/agents/{agentID}/canvas/projects
GET  /internal/agents/{agentID}/canvas/artifacts
GET  /internal/agents/{agentID}/canvas/snapshot
POST /internal/agents/{agentID}/canvas/artifacts/sync
```

Endpoint behavior:

- `GET /internal/agents/{agentID}/canvas/projects` powers `canvas project list`.
- `POST /internal/agents/{agentID}/canvas/projects` powers `canvas project create --name ...` and creates an org-scoped DB project.
- `GET /internal/agents/{agentID}/canvas/artifacts` powers `canvas artifact list --project ...`. It should accept `project_id` or `project_slug` as query parameters.
- `GET /internal/agents/{agentID}/canvas/snapshot` powers runtime startup hydration. It returns all non-archived org projects, artifacts, manifests, and artifact file download URLs so the runtime can rebuild `/workspace/canvas` in the background.
- `POST /internal/agents/{agentID}/canvas/artifacts/sync` powers manual sync and watcher sync. It upserts the project if needed, upserts the artifact by org/project/slug or ID, and replaces the stored file set for that artifact.

Suggested snapshot response shape:

```json
{
  "projects": [
    {
      "id": "project-uuid",
      "slug": "project-slug",
      "name": "Project name",
      "artifacts": [
        {
          "id": "artifact-uuid",
          "slug": "settings-page-v1",
          "type": "web_page",
          "name": "Settings page",
          "manifest": {},
          "files": [
            {
              "path": "index.html",
              "role": "entrypoint",
              "content_type": "text/html; charset=utf-8",
              "download_url": "https://object-storage.example/artifact-file",
              "size_bytes": 1234,
              "sha256": "hex-encoded-sha256"
            }
          ]
        }
      ]
    }
  ]
}
```

Suggested sync payload shape:

```json
{
  "session_id": "session-uuid",
  "project": {
    "id": "project-uuid-or-empty",
    "slug": "project-slug",
    "name": "Project name"
  },
  "artifact": {
    "id": "artifact-uuid-or-empty",
    "slug": "settings-page-v1",
    "type": "web_page",
    "name": "Settings page",
    "manifest": {}
  },
  "files": [
    {
      "path": "index.html",
      "role": "entrypoint",
      "content_type": "text/html; charset=utf-8",
      "content": "<!doctype html>...",
      "size_bytes": 1234,
      "sha256": "hex-encoded-sha256"
    }
  ]
}
```

The runtime watcher should also emit Canvas runtime events through the existing runtime event transport. There is no separate watcher event endpoint in the prototype.

### Backend Tables

Prototype tables:

```text
canvas_projects
  id
  org_id
  slug
  name
  description
  created_by_agent_id
  created_by_user_id
  created_at
  updated_at

canvas_artifacts
  id
  org_id
  project_id
  slug
  type
  name
  manifest_json
  created_by_agent_id
  created_by_user_id
  created_at
  updated_at

canvas_artifact_files
  id
  org_id
  artifact_id
  path
  role
  content_type
  object_key
  public_url
  size_bytes
  sha256
  archived_at
  created_at
  updated_at
```

Artifact file contents are uploaded to S3/object storage during sync. `canvas_artifact_files` stores metadata and object references only.

### Runtime Folder Watcher

The sandbox runtime should watch `/workspace/canvas` efficiently.

Responsibilities:

- Detect created, updated, renamed, and deleted artifact files.
- Debounce changes so large writes do not create excessive sync work.
- Validate changed JSON artifact manifests before notifying the control plane.
- Emit runtime stream events for the active session so the frontend can refresh Canvas state.
- Sync changed artifacts to the control plane automatically.
- Hydrate `/workspace/canvas` on runtime startup by fetching all org Canvas projects and artifacts from the backend, then downloading artifact files from their S3/object storage URLs.
- Hydration should run in the background after runtime startup and must not block runtime readiness.
- Hydration downloads should run concurrently with bounded parallelism rather than a slow serial loop.

Suggested runtime event types:

```text
canvas.project.created
canvas.artifact.created
canvas.artifact.updated
canvas.artifact.deleted
canvas.artifact.sync_started
canvas.artifact.synced
canvas.artifact.sync_failed
```

Manual `canvas artifact sync` should use the same backend sync path as the watcher.

Sync events are live/SSE-only signals for UI refresh and iframe reload. They do not need to be durable session events. Project and artifact create/update/archive events should still be emitted so the frontend can update its Canvas lists.

### Preview Model

Artifacts must preview as real interactive HTML in iframes.

The frontend should render one selected artifact at a time with artifact tabs, not an infinite canvas. The iframe should preserve interaction inside the HTML document.

Iframe previews should load directly from the active sandbox. This keeps the interactive preview path fast and avoids upload-to-S3-then-download-to-browser latency.

Sync still uploads artifacts to S3/object storage so the backend has durable artifact files and future sandboxes can hydrate `/workspace/canvas`.

### Comments

Artifact comments are message attachments, not artifact file mutations.

The frontend should collect artifact comment anchors and bodies locally, then attach them to the next session message as a new payload key such as `artifact_comments`.

The agent should receive these comments in prompt context with enough anchor data to update the artifact:

```json
{
  "id": "comment-id",
  "artifact_id": "artifact-id",
  "artifact_name": "Settings page",
  "artifact_type": "web_page",
  "file_path": "index.html",
  "slide_id": null,
  "element_id": "hero-title",
  "selector": "[data-canvas-id=\"hero-title\"]",
  "position": { "x": 420, "y": 180 },
  "body": "Make this headline more direct."
}
```

This mirrors the current code line comment flow conceptually, but should use a separate payload key and separate frontend types.

The artifact validator should require data attributes needed for current-session comment targeting. The required attribute is `data-canvas-id`. Coordinate comments can be fallback context, but the prototype should require element anchors for meaningful semantic blocks.

### End-to-End Test Requirement

Add a flagship E2E test based on the existing agent-session Docker provider flow.

The test should prove:

- The stack runs through Docker Compose.
- A per-session sandbox is provisioned using the Docker provider.
- The runtime has Canvas CLI access.
- `canvas project create` writes to the backend DB through the Go API using runtime-secret auth.
- `canvas project list` reads the created project from the backend.
- `canvas artifact create` scaffolds files under `/workspace/canvas`.
- `canvas artifact validate` passes for generated artifacts.
- `canvas artifact sync` creates or updates backend artifact rows and artifact file rows.
- Runtime watcher changes emit session runtime events.
- The control plane receives sync updates and the frontend-visible API can list synced artifacts.

## Resolved Prototype Decisions

- Preview source: iframe previews load directly from the sandbox.
- Durable file storage: artifact file contents are uploaded to S3/object storage during sync.
- Runtime watcher: ownership is in the Rust sandbox runtime that runs the agent.
- Multi-session sandbox reuse: no special prototype handling. Last write wins is acceptable.
- Sync direction: bidirectional. Runtime hydrates `/workspace/canvas` from backend/S3 on startup and syncs local changes back to the control plane.
- Deletes: mark projects/artifacts/files archived in the database rather than hard deleting rows.
- External assets: allowed. Validation should focus on Canvas manifest requirements, HTML structure, and required data attributes, not blocking remote URLs.
- Comment anchors: require `data-canvas-id` in the prototype.
- Browser verification: no Playwright requirement. `canvas artifact verify` performs static HTML validation.
- Sync event durability: sync events are SSE/live-only and used for frontend refresh/iframe reload.
- Artifact identity: use human-readable slugs derived from names. If a slug exists, append an incrementing suffix such as `-1`, `-2`. Backend UUIDs can exist internally, but the CLI should return enough slug/ID/path information for the agent to continue.
- Concurrent sync behavior: last write wins for the prototype.
- Penpot removal: not part of the prototype implementation. Delete Penpot after the new Canvas flow is tested.
