-- +goose Up
-- Anna's current catalog manifest no longer eager-loads browser command docs,
-- but existing clones retain the install-time snapshot. Clear that stale
-- snapshot so the runtime loads browser and Playwright guidance on demand.
UPDATE agents
SET auto_load_skills = '[]'::jsonb,
    updated_at = NOW()
WHERE agent_catalog_id IN (
    SELECT id
    FROM agent_catalog
    WHERE slug = 'anna-playwright-qa-engineer'
)
  AND auto_load_skills <> '[]'::jsonb;

-- +goose Down
-- The prior eager-load selection varied by installation and is intentionally
-- not reconstructed.
