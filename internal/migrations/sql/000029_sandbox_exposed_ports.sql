-- +goose Up
ALTER TABLE orgs
    ADD COLUMN IF NOT EXISTS sandbox_exposed_ports integer[] NOT NULL DEFAULT '{3000,5173,8000,8080}';

ALTER TABLE sandboxes
    ADD COLUMN IF NOT EXISTS exposed_ports integer[] NOT NULL DEFAULT '{3000,5173,8000,8080}';

-- +goose Down
ALTER TABLE sandboxes
    DROP COLUMN IF EXISTS exposed_ports;

ALTER TABLE orgs
    DROP COLUMN IF EXISTS sandbox_exposed_ports;
