-- +goose Up
-- Grace window for single-use refresh-token rotation. When a token is rotated,
-- the replacement pair is recorded on the revoked row so a concurrent
-- presentation of the same single-use token (from another tab, the proxy, the
-- stream-token route, or another instance) can be handed the same rotated pair
-- within the grace window instead of being force-logged-out.
ALTER TABLE refresh_tokens
    ADD COLUMN replaced_at timestamp with time zone,
    ADD COLUMN replaced_by_access_token text,
    ADD COLUMN replaced_by_refresh_token text;

-- +goose Down
ALTER TABLE refresh_tokens
    DROP COLUMN IF EXISTS replaced_by_refresh_token,
    DROP COLUMN IF EXISTS replaced_by_access_token,
    DROP COLUMN IF EXISTS replaced_at;
