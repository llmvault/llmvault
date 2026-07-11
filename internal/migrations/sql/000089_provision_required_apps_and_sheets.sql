-- +goose Up
-- Apps and Sheets were previously auto-installed for every agent. Preserve
-- those capabilities for existing teams that already run catalog agents which
-- explicitly require them, while making the plugins team-managed for everyone
-- else.
WITH required_team_plugins AS (
    SELECT DISTINCT agents.org_id, agents.team_id, plugins.id AS plugin_id
    FROM agents
    JOIN agent_catalog ON agent_catalog.id = agents.agent_catalog_id
    JOIN plugins ON plugins.slug = ANY(agent_catalog.required_plugins)
    WHERE agents.status <> 'archived'
      AND plugins.org_id IS NULL
      AND plugins.status = 'active'
      AND plugins.slug IN ('apps', 'sheets')
)
INSERT INTO org_plugin_installs (id, org_id, plugin_id, created_at, updated_at)
SELECT gen_random_uuid(), required_team_plugins.org_id, required_team_plugins.plugin_id, NOW(), NOW()
FROM required_team_plugins
WHERE NOT EXISTS (
    SELECT 1
    FROM org_plugin_installs installs
    WHERE installs.org_id = required_team_plugins.org_id
      AND installs.plugin_id = required_team_plugins.plugin_id
      AND installs.revoked_at IS NULL
);

WITH required_team_plugins AS (
    SELECT DISTINCT agents.org_id, agents.team_id, plugins.id AS plugin_id
    FROM agents
    JOIN agent_catalog ON agent_catalog.id = agents.agent_catalog_id
    JOIN plugins ON plugins.slug = ANY(agent_catalog.required_plugins)
    WHERE agents.status <> 'archived'
      AND plugins.org_id IS NULL
      AND plugins.status = 'active'
      AND plugins.slug IN ('apps', 'sheets')
)
INSERT INTO team_plugins (id, org_id, team_id, plugin_id, created_at)
SELECT gen_random_uuid(), required_team_plugins.org_id, required_team_plugins.team_id, required_team_plugins.plugin_id, NOW()
FROM required_team_plugins
WHERE NOT EXISTS (
    SELECT 1
    FROM team_plugins grants
    WHERE grants.team_id = required_team_plugins.team_id
      AND grants.plugin_id = required_team_plugins.plugin_id
);
