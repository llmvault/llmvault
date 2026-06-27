-- +goose Up
CREATE TABLE canvas_projects (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id uuid NOT NULL REFERENCES orgs(id) ON DELETE CASCADE,
    name text NOT NULL DEFAULT '',
    created_by_agent_id uuid REFERENCES agents(id) ON DELETE SET NULL,
    created_by_user_id uuid REFERENCES users(id) ON DELETE SET NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX idx_canvas_projects_org_id
    ON canvas_projects(org_id);

-- +goose Down
DROP TABLE IF EXISTS canvas_projects;
