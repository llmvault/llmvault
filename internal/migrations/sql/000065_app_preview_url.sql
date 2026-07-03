-- +goose Up
-- App preview side channel: builder agents run a preview of an app inside
-- their OWN sandbox (GET .../apps/{appID}/preview-env). The server computes
-- the public preview URL from the builder sandbox + requested port and
-- records it here so the platform knows where the latest preview lives.
-- '' / NULL until the first preview registration.
ALTER TABLE apps ADD COLUMN preview_url text NOT NULL DEFAULT '';
ALTER TABLE apps ADD COLUMN preview_registered_at timestamptz;

-- +goose Down
ALTER TABLE apps DROP COLUMN IF EXISTS preview_registered_at;
ALTER TABLE apps DROP COLUMN IF EXISTS preview_url;
