-- +goose Up
-- Apps (apps plan §0): agent-built web applications bound to exactly ONE
-- sheet. The app record stores its microsandbox alias stem, current sandbox,
-- and the AES-256-GCM (SandboxEncKey) encrypted runtime secret that
-- authenticates the app backend to the internal app API — the same pattern as
-- sandboxes.encrypted_runtime_secret.
CREATE TABLE apps (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id uuid NOT NULL REFERENCES orgs(id) ON DELETE CASCADE,
    channel_id uuid NOT NULL REFERENCES channels(id) ON DELETE CASCADE,
    sheet_id uuid NOT NULL REFERENCES sheets(id) ON DELETE CASCADE,
    slug text NOT NULL,
    name text NOT NULL,
    description text NOT NULL DEFAULT '',
    icon text NOT NULL DEFAULT '',
    alias text NOT NULL DEFAULT '',
    sandbox_id uuid REFERENCES sandboxes(id) ON DELETE SET NULL,
    encrypted_app_secret bytea NOT NULL,
    active_version_id uuid,
    status text NOT NULL DEFAULT 'draft',
    template_version text NOT NULL DEFAULT '',
    created_by_agent_id uuid REFERENCES agents(id) ON DELETE SET NULL,
    created_by_user_id uuid REFERENCES users(id) ON DELETE SET NULL,
    source_session_id uuid REFERENCES sessions(id) ON DELETE SET NULL,
    archived_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT apps_status_check
        CHECK (status IN ('draft', 'deploying', 'running', 'stopped', 'failed'))
);

CREATE UNIQUE INDEX idx_apps_org_slug_active
    ON apps(org_id, slug)
    WHERE archived_at IS NULL;
CREATE INDEX idx_apps_channel_updated_active
    ON apps(channel_id, updated_at DESC)
    WHERE archived_at IS NULL;
CREATE INDEX idx_apps_sheet_active
    ON apps(sheet_id)
    WHERE archived_at IS NULL;

-- Immutable version history: object keys follow the canvas-artifact layout
-- (pub/o/{orgID}/apps/{appSlug}/{sha256}/...), new content = new key, rows are
-- soft-archived for pruning and never overwritten.
CREATE TABLE app_versions (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    app_id uuid NOT NULL REFERENCES apps(id) ON DELETE CASCADE,
    org_id uuid NOT NULL REFERENCES orgs(id) ON DELETE CASCADE,
    source_object_key text NOT NULL,
    bundle_object_key text NOT NULL,
    source_sha256 text NOT NULL,
    bundle_sha256 text NOT NULL,
    source_bytes bigint NOT NULL DEFAULT 0,
    bundle_bytes bigint NOT NULL DEFAULT 0,
    notes text NOT NULL DEFAULT '',
    template_version text NOT NULL DEFAULT '',
    created_by_agent_id uuid REFERENCES agents(id) ON DELETE SET NULL,
    source_session_id uuid REFERENCES sessions(id) ON DELETE SET NULL,
    archived_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX idx_app_versions_app_created_active
    ON app_versions(app_id, created_at DESC)
    WHERE archived_at IS NULL;
CREATE INDEX idx_app_versions_org
    ON app_versions(org_id);

-- Circular FK, added once both tables exist. ON DELETE SET NULL so pruning a
-- version row can never strand an app behind a dangling pointer; app code
-- keeps active_version_id consistent on deploy/rollback.
ALTER TABLE apps ADD CONSTRAINT fk_apps_active_version
    FOREIGN KEY (active_version_id) REFERENCES app_versions(id) ON DELETE SET NULL;

-- +goose Down
ALTER TABLE apps DROP CONSTRAINT IF EXISTS fk_apps_active_version;
DROP TABLE IF EXISTS app_versions;
DROP TABLE IF EXISTS apps;
