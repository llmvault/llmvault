-- +goose Up
-- Composer uploads can now happen before a session (and its sandbox) exists:
-- the new-chat screen uploads attachments first and references them in the
-- session create call. Those assets have no sandbox to attach to.
ALTER TABLE agent_assets ALTER COLUMN sandbox_id DROP NOT NULL;

-- +goose Down
DELETE FROM agent_assets WHERE sandbox_id IS NULL;
ALTER TABLE agent_assets ALTER COLUMN sandbox_id SET NOT NULL;
