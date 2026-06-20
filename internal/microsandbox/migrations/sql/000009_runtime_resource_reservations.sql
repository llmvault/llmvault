-- +goose Up
-- +goose StatementBegin
ALTER TABLE microsandbox_runners
    ALTER COLUMN disk_overcommit SET DEFAULT 4;

UPDATE microsandbox_runners AS r
SET
    reserved_cpu = COALESCE((
        SELECT SUM(cpu)
        FROM microsandbox_sandboxes AS s
        WHERE s.runner_id = r.id
          AND s.status IN ('creating', 'running')
    ), 0),
    reserved_memory_mb = COALESCE((
        SELECT SUM(memory_mb)
        FROM microsandbox_sandboxes AS s
        WHERE s.runner_id = r.id
          AND s.status IN ('creating', 'running')
    ), 0),
    reserved_disk_gb = COALESCE((
        SELECT SUM(disk_gb)
        FROM microsandbox_sandboxes AS s
        WHERE s.runner_id = r.id
          AND s.status IN ('creating', 'running', 'stopped')
    ), 0);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DO $$ BEGIN RAISE EXCEPTION 'microsandbox down migration is intentionally unsupported; reset or restore the microsandbox database instead'; END $$;
-- +goose StatementEnd
