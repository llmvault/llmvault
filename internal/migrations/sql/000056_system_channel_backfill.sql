-- +goose Up
-- Backfill the private "system" channel for existing orgs. New orgs now get
-- it at bootstrap (alongside #general), and agentschedule / trigger dispatch
-- keep a lazy FirstOrCreate safety net with identical attributes.
--
-- default_agent_id is the org's default agent (agents.is_default = true).
-- Orgs without a default agent are intentionally skipped: channels.
-- default_agent_id is NOT NULL so there is no principled value to store, and
-- the lazy ensureSystemChannel fallback still creates the channel (anchored to
-- whichever agent first needs it) the first time a schedule or trigger runs.
--
-- Idempotent: the WHERE NOT EXISTS predicate matches the partial unique index
-- idx_channels_org_source_name (org_id, origin, external_provider,
-- external_workspace_key, external_resource_type, name) WHERE archived_at IS NULL.
INSERT INTO channels (
    org_id,
    name,
    description,
    kind,
    visibility,
    default_agent_id,
    origin,
    external_metadata,
    created_at,
    updated_at
)
SELECT
    o.id,
    'system',
    'System-managed jobs',
    'system',
    'private',
    a.id,
    'native',
    '{"source": "system"}'::jsonb,
    now(),
    now()
FROM orgs o
JOIN LATERAL (
    SELECT ag.id
    FROM agents ag
    WHERE ag.org_id = o.id
      AND ag.is_default = true
    ORDER BY ag.created_at ASC
    LIMIT 1
) a ON true
WHERE NOT EXISTS (
    SELECT 1
    FROM channels c
    WHERE c.org_id = o.id
      AND c.origin = 'native'
      AND c.external_provider = ''
      AND c.external_workspace_key = ''
      AND c.external_resource_type = 'channel'
      AND c.name = 'system'
      AND c.archived_at IS NULL
);
