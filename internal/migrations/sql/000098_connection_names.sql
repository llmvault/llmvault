-- +goose Up
ALTER TABLE public.connections
    ADD COLUMN name text NOT NULL DEFAULT '',
    ADD COLUMN slug text NOT NULL DEFAULT '',
    ADD COLUMN needs_name boolean NOT NULL DEFAULT false;

ALTER TABLE public.database_connections
    ADD COLUMN name text NOT NULL DEFAULT '',
    ADD COLUMN slug text NOT NULL DEFAULT '',
    ADD COLUMN needs_name boolean NOT NULL DEFAULT false;

-- Backfill stable, human-readable identities. UUID prefixes provide six
-- random hexadecimal characters for additional historical connections.
WITH ranked AS (
    SELECT c.id,
           i.provider,
           row_number() OVER (
               PARTITION BY c.org_id, i.provider
               ORDER BY (c.revoked_at IS NULL) DESC, c.created_at, c.id
           ) AS position
      FROM public.connections c
      JOIN public.integrations i ON i.id = c.integration_id
)
UPDATE public.connections c
   SET name = CASE WHEN ranked.position = 1 THEN ranked.provider ELSE substring(replace(c.id::text, '-', '') FROM 1 FOR 6) END,
       slug = CASE WHEN ranked.position = 1 THEN ranked.provider ELSE substring(replace(c.id::text, '-', '') FROM 1 FOR 6) END,
       needs_name = ranked.position > 1
  FROM ranked
 WHERE ranked.id = c.id;

WITH ranked AS (
    SELECT id,
           provider,
           row_number() OVER (
               PARTITION BY org_id, provider
               ORDER BY (revoked_at IS NULL) DESC, created_at, id
           ) AS position
      FROM public.database_connections
)
UPDATE public.database_connections c
   SET name = CASE WHEN ranked.position = 1 THEN ranked.provider ELSE substring(replace(c.id::text, '-', '') FROM 1 FOR 6) END,
       slug = CASE WHEN ranked.position = 1 THEN ranked.provider ELSE substring(replace(c.id::text, '-', '') FROM 1 FOR 6) END,
       needs_name = ranked.position > 1
  FROM ranked
 WHERE ranked.id = c.id;

DROP INDEX public.idx_database_connections_one_active_provider;

CREATE UNIQUE INDEX idx_connections_active_org_slug
    ON public.connections (org_id, slug)
    WHERE revoked_at IS NULL;

CREATE UNIQUE INDEX idx_database_connections_active_org_slug
    ON public.database_connections (org_id, slug)
    WHERE revoked_at IS NULL;

-- Connection and plugin changes alter the generated MCP server set.
CREATE TRIGGER connections_mcp_config_version
AFTER INSERT OR UPDATE OR DELETE ON public.connections
FOR EACH ROW EXECUTE FUNCTION public.bump_mcp_config_version();

CREATE TRIGGER database_connections_mcp_config_version
AFTER INSERT OR UPDATE OR DELETE ON public.database_connections
FOR EACH ROW EXECUTE FUNCTION public.bump_mcp_config_version();

CREATE TRIGGER org_plugin_installs_mcp_config_version
AFTER INSERT OR UPDATE OR DELETE ON public.org_plugin_installs
FOR EACH ROW EXECUTE FUNCTION public.bump_mcp_config_version();

CREATE TRIGGER team_plugins_mcp_config_version
AFTER INSERT OR UPDATE OR DELETE ON public.team_plugins
FOR EACH ROW EXECUTE FUNCTION public.bump_mcp_config_version();
