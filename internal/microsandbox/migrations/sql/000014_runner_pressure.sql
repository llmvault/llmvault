-- +goose Up
ALTER TABLE microsandbox_runners ADD COLUMN IF NOT EXISTS cpu_utilization double precision NOT NULL DEFAULT 0;
ALTER TABLE microsandbox_runners ADD COLUMN IF NOT EXISTS load1 double precision NOT NULL DEFAULT 0;
ALTER TABLE microsandbox_runners ADD COLUMN IF NOT EXISTS runnable_processes bigint NOT NULL DEFAULT 0;
ALTER TABLE microsandbox_runners ADD COLUMN IF NOT EXISTS starting_operations bigint NOT NULL DEFAULT 0;
ALTER TABLE microsandbox_runners ADD COLUMN IF NOT EXISTS reported_running_sandboxes bigint NOT NULL DEFAULT 0;

-- +goose Down
ALTER TABLE microsandbox_runners DROP COLUMN IF EXISTS reported_running_sandboxes;
ALTER TABLE microsandbox_runners DROP COLUMN IF EXISTS starting_operations;
ALTER TABLE microsandbox_runners DROP COLUMN IF EXISTS runnable_processes;
ALTER TABLE microsandbox_runners DROP COLUMN IF EXISTS load1;
ALTER TABLE microsandbox_runners DROP COLUMN IF EXISTS cpu_utilization;
