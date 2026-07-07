-- +goose Up
-- +goose StatementBegin
ALTER TABLE integrations
    ADD COLUMN bot_handle text NOT NULL DEFAULT '';
-- +goose StatementEnd

-- +goose StatementBegin
-- Remove legacy GitHub providers. Their connections cascade via
-- fk_connections_integration (ON DELETE CASCADE).
DELETE FROM integrations
WHERE provider IN ('github-app-oauth', 'github-pat', 'github');
-- +goose StatementEnd

-- +goose StatementBegin
-- Backfill bot handles for the two surviving GitHub App providers so the
-- manifests remain the source of truth on the next seed as well.
UPDATE integrations SET bot_handle = 'usehivy'
WHERE provider = 'github-app';
-- +goose StatementEnd

-- +goose StatementBegin
UPDATE integrations SET bot_handle = 'usehivy-reviews'
WHERE provider = 'github-app-code-reviews';
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
ALTER TABLE integrations
    DROP COLUMN bot_handle;
-- +goose StatementEnd
