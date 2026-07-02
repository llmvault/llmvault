-- +goose Up
-- Org-owned plugins: the skill-manager lets an org's default agent group
-- agent-authored skills under plugins owned by the org. Global catalog
-- plugins keep org_id NULL; slug uniqueness becomes per-scope so orgs can
-- pick natural names ("sales", "engineering") without colliding with each
-- other. Global slugs stay globally unique.

ALTER TABLE plugins
    ADD COLUMN org_id uuid;

ALTER TABLE ONLY plugins
    ADD CONSTRAINT fk_plugins_org FOREIGN KEY (org_id) REFERENCES orgs(id) ON DELETE CASCADE;

CREATE INDEX idx_plugins_org_id ON plugins USING btree (org_id);

DROP INDEX idx_plugins_slug;
CREATE UNIQUE INDEX idx_plugins_slug_global ON plugins USING btree (slug) WHERE org_id IS NULL;
CREATE UNIQUE INDEX idx_plugins_org_slug ON plugins USING btree (org_id, slug) WHERE org_id IS NOT NULL;

-- +goose Down
-- +goose StatementBegin
DO $$ BEGIN RAISE EXCEPTION 'down migration is intentionally unsupported; reset or restore the database instead'; END $$;
-- +goose StatementEnd
