-- +goose Up
-- Source-aware channel name uniqueness.

DROP INDEX IF EXISTS idx_channels_org_name;

CREATE UNIQUE INDEX IF NOT EXISTS idx_channels_org_source_name
    ON channels USING btree (org_id, origin, external_provider, external_workspace_key, external_resource_type, name)
    WHERE archived_at IS NULL;
