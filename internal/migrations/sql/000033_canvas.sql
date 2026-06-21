-- +goose Up
CREATE TABLE canvas_projects (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id uuid NOT NULL REFERENCES orgs(id) ON DELETE CASCADE,
    penpot_project_id uuid NOT NULL UNIQUE,
    name text NOT NULL DEFAULT '',
    created_by_agent_id uuid REFERENCES agents(id) ON DELETE SET NULL,
    created_by_user_id uuid REFERENCES users(id) ON DELETE SET NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX idx_canvas_projects_org_penpot_project
    ON canvas_projects(org_id, penpot_project_id);
CREATE INDEX idx_canvas_projects_org_id
    ON canvas_projects(org_id);

CREATE TABLE canvas_files (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id uuid NOT NULL REFERENCES orgs(id) ON DELETE CASCADE,
    canvas_project_id uuid REFERENCES canvas_projects(id) ON DELETE SET NULL,
    penpot_project_id uuid NOT NULL,
    penpot_file_id uuid NOT NULL UNIQUE,
    penpot_page_id uuid,
    name text NOT NULL DEFAULT '',
    created_by_agent_id uuid REFERENCES agents(id) ON DELETE SET NULL,
    created_by_user_id uuid REFERENCES users(id) ON DELETE SET NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX idx_canvas_files_org_penpot_file
    ON canvas_files(org_id, penpot_file_id);
CREATE INDEX idx_canvas_files_org_id
    ON canvas_files(org_id);
CREATE INDEX idx_canvas_files_canvas_project_id
    ON canvas_files(canvas_project_id);
CREATE INDEX idx_canvas_files_penpot_project_id
    ON canvas_files(penpot_project_id);

-- +goose Down
DROP TABLE IF EXISTS canvas_files;
DROP TABLE IF EXISTS canvas_projects;
