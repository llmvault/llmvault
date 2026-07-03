-- +goose Up
-- Sheets become channel-scoped: every sheet belongs to exactly one channel, and
-- its visibility follows that channel's access. The sheets table is empty in
-- every environment, so channel_id is added NOT NULL directly with no backfill.
ALTER TABLE sheets
    ADD COLUMN channel_id uuid NOT NULL REFERENCES channels(id) ON DELETE CASCADE;

-- Hot path is "list this channel's sheets, newest-updated first".
CREATE INDEX idx_sheets_channel_updated_active
    ON sheets(channel_id, updated_at DESC)
    WHERE archived_at IS NULL;

-- +goose Down
DROP INDEX IF EXISTS idx_sheets_channel_updated_active;
ALTER TABLE sheets DROP COLUMN IF EXISTS channel_id;
