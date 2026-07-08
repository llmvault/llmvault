-- +goose Up
-- +goose StatementBegin
-- Org-scope the preview-alias namespace. Previously an alias row was keyed on
-- the hostname stem alone with no owner, so Org B claiming Org A's app slug
-- would last-write-wins repoint https://{slug}.{PreviewBaseDomain} at Org B's
-- sandbox (hostname/content takeover). We record the owning org (derived from
-- the claiming sandbox) so the control plane can reject a cross-org repoint.
ALTER TABLE microsandbox_aliases
    ADD COLUMN IF NOT EXISTS org_id text NOT NULL DEFAULT '';

-- Backfill org_id from the currently-mapped sandbox so existing aliases keep
-- their owner and are protected by the cross-org guard immediately.
UPDATE microsandbox_aliases a
SET org_id = s.org_id
FROM microsandbox_sandboxes s
WHERE a.sandbox_id = s.id
  AND a.org_id = '';

CREATE INDEX IF NOT EXISTS idx_microsandbox_aliases_org_id
    ON microsandbox_aliases (org_id);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DO $$ BEGIN RAISE EXCEPTION 'microsandbox down migration is intentionally unsupported; reset or restore the microsandbox database instead'; END $$;
-- +goose StatementEnd
