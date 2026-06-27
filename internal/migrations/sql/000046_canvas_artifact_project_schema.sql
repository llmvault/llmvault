-- +goose Up
DROP TABLE IF EXISTS canvas_files;

DROP INDEX IF EXISTS idx_canvas_projects_org_penpot_project;

ALTER TABLE canvas_projects
    DROP COLUMN IF EXISTS penpot_project_id;

-- +goose Down
SELECT 1;
