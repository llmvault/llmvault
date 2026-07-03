-- +goose Up
-- The alias's public URL, resolved by AppURL() in preference to the sandbox
-- endpoint. Populated on deploy when the provider supports aliases
-- (microsandbox: https://{alias}.{previewBaseDomain}); stays '' for
-- docker/local, which keep plain sandbox-endpoint URLs. Distinct from the
-- alias stem (apps.alias) so the resolved production URL survives redeploys.
ALTER TABLE apps ADD COLUMN alias_url text NOT NULL DEFAULT '';

-- +goose Down
ALTER TABLE apps DROP COLUMN IF EXISTS alias_url;
