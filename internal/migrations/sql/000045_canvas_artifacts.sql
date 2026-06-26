-- +goose Up
ALTER TABLE canvas_projects
    ADD COLUMN IF NOT EXISTS slug text NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS description text NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS archived_at timestamptz;

CREATE UNIQUE INDEX IF NOT EXISTS idx_canvas_projects_org_slug_active
    ON canvas_projects(org_id, slug)
    WHERE archived_at IS NULL AND slug <> '';
CREATE INDEX IF NOT EXISTS idx_canvas_projects_org_updated_active
    ON canvas_projects(org_id, updated_at DESC)
    WHERE archived_at IS NULL;
CREATE INDEX IF NOT EXISTS idx_canvas_projects_archived_at
    ON canvas_projects(archived_at);

CREATE TABLE canvas_artifacts (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id uuid NOT NULL REFERENCES orgs(id) ON DELETE CASCADE,
    canvas_project_id uuid NOT NULL REFERENCES canvas_projects(id) ON DELETE CASCADE,
    slug text NOT NULL,
    type text NOT NULL,
    name text NOT NULL DEFAULT '',
    manifest_json jsonb NOT NULL DEFAULT '{}'::jsonb,
    source_session_id uuid REFERENCES sessions(id) ON DELETE SET NULL,
    created_by_agent_id uuid REFERENCES agents(id) ON DELETE SET NULL,
    created_by_user_id uuid REFERENCES users(id) ON DELETE SET NULL,
    archived_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT canvas_artifacts_type_check CHECK (type IN ('web_page', 'presentation'))
);

CREATE UNIQUE INDEX idx_canvas_artifacts_project_slug_active
    ON canvas_artifacts(canvas_project_id, slug)
    WHERE archived_at IS NULL;
CREATE INDEX idx_canvas_artifacts_org_project
    ON canvas_artifacts(org_id, canvas_project_id)
    WHERE archived_at IS NULL;
CREATE INDEX idx_canvas_artifacts_source_session
    ON canvas_artifacts(source_session_id)
    WHERE source_session_id IS NOT NULL;
CREATE INDEX idx_canvas_artifacts_archived_at
    ON canvas_artifacts(archived_at);

CREATE TABLE canvas_artifact_files (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id uuid NOT NULL REFERENCES orgs(id) ON DELETE CASCADE,
    canvas_artifact_id uuid NOT NULL REFERENCES canvas_artifacts(id) ON DELETE CASCADE,
    path text NOT NULL,
    role text NOT NULL DEFAULT '',
    content_type text NOT NULL DEFAULT '',
    object_key text NOT NULL,
    size_bytes bigint NOT NULL DEFAULT 0,
    sha256 text NOT NULL DEFAULT '',
    archived_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT canvas_artifact_files_size_check CHECK (size_bytes >= 0)
);

CREATE UNIQUE INDEX idx_canvas_artifact_files_artifact_path_active
    ON canvas_artifact_files(canvas_artifact_id, path)
    WHERE archived_at IS NULL;
CREATE INDEX idx_canvas_artifact_files_object_key
    ON canvas_artifact_files(object_key);
CREATE INDEX idx_canvas_artifact_files_org
    ON canvas_artifact_files(org_id);
CREATE INDEX idx_canvas_artifact_files_archived_at
    ON canvas_artifact_files(archived_at);

-- +goose Down
DROP TABLE IF EXISTS canvas_artifact_files;
DROP TABLE IF EXISTS canvas_artifacts;
DROP INDEX IF EXISTS idx_canvas_projects_archived_at;
DROP INDEX IF EXISTS idx_canvas_projects_org_updated_active;
DROP INDEX IF EXISTS idx_canvas_projects_org_slug_active;
ALTER TABLE canvas_projects
    DROP COLUMN IF EXISTS archived_at,
    DROP COLUMN IF EXISTS description,
    DROP COLUMN IF EXISTS slug;
