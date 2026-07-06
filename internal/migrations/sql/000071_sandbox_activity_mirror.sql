-- +goose Up
-- Idle signals for app sandboxes (which have no session): last_app_activity_at
-- from the app's activity ping, last_gateway_activity_at mirrored from the
-- control plane as the fallback. Auto-sleep stops an app when both go stale.
ALTER TABLE sandboxes ADD COLUMN last_app_activity_at timestamptz;
ALTER TABLE sandboxes ADD COLUMN last_gateway_activity_at timestamptz;

-- +goose Down
ALTER TABLE sandboxes DROP COLUMN IF EXISTS last_app_activity_at;
ALTER TABLE sandboxes DROP COLUMN IF EXISTS last_gateway_activity_at;
