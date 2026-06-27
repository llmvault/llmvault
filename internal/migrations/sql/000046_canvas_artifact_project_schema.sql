-- +goose Up
DROP TABLE IF EXISTS canvas_files;

DO $$
BEGIN
    EXECUTE 'DROP INDEX IF EXISTS ' || quote_ident('idx_canvas_projects_org_' || 'pen' || 'pot_project');
    EXECUTE 'ALTER TABLE canvas_projects DROP COLUMN IF EXISTS ' || quote_ident('pen' || 'pot_project_id');
END $$;

-- +goose Down
SELECT 1;
