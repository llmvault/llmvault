-- +goose Up
-- Attribute API keys to the user who created them. Nullable because
-- API-key-authenticated callers (machine-to-machine) have no human creator, and
-- pre-existing keys predate attribution. ON DELETE SET NULL so removing a user
-- does not delete the keys they minted — the key keeps working, it just loses
-- its creator attribution.
ALTER TABLE api_keys ADD COLUMN created_by uuid REFERENCES users(id) ON DELETE SET NULL;
CREATE INDEX idx_api_keys_created_by ON api_keys USING btree (created_by);

-- +goose Down
DROP INDEX IF EXISTS idx_api_keys_created_by;
ALTER TABLE api_keys DROP COLUMN IF EXISTS created_by;
