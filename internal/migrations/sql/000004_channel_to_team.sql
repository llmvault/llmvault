-- +goose Up

-- Reconcile databases created before channel ownership was replaced by teams.
-- Every additive operation is idempotent because a fresh database receives the
-- current schema from 000001 before it reaches this migration.

ALTER TABLE public.agents
    ADD COLUMN IF NOT EXISTS memory_mission text;

ALTER TABLE public.agent_directives
    ADD COLUMN IF NOT EXISTS agent_id uuid;
ALTER TABLE public.agent_memories
    ADD COLUMN IF NOT EXISTS agent_id uuid;
ALTER TABLE public.agent_observations
    ADD COLUMN IF NOT EXISTS agent_id uuid;
ALTER TABLE public.memory_suppressions
    ADD COLUMN IF NOT EXISTS agent_id uuid;
ALTER TABLE public.sessions
    ADD COLUMN IF NOT EXISTS team_id uuid;
ALTER TABLE public.sheets
    ADD COLUMN IF NOT EXISTS team_id uuid;
ALTER TABLE public.apps
    ADD COLUMN IF NOT EXISTS team_id uuid;

ALTER TABLE public.agent_triggers
    ADD COLUMN IF NOT EXISTS resource_type text DEFAULT ''::text NOT NULL,
    ADD COLUMN IF NOT EXISTS resource_key text DEFAULT ''::text NOT NULL,
    ADD COLUMN IF NOT EXISTS resource_name text DEFAULT ''::text NOT NULL;

-- The legacy Slack workspace ID used the name team_id. Preserve it as
-- slack_team_id before adding the Kubernetes-era team UUID.
-- +goose StatementBegin
DO $$
DECLARE
    legacy_team_type text;
BEGIN
    SELECT data_type
      INTO legacy_team_type
      FROM information_schema.columns
     WHERE table_schema = 'public'
       AND table_name = 'slack_thread_events'
       AND column_name = 'team_id';

    IF legacy_team_type = 'text' THEN
        IF NOT EXISTS (
            SELECT 1 FROM information_schema.columns
             WHERE table_schema = 'public'
               AND table_name = 'slack_thread_events'
               AND column_name = 'slack_team_id'
        ) THEN
            ALTER TABLE public.slack_thread_events RENAME COLUMN team_id TO slack_team_id;
        ELSE
            UPDATE public.slack_thread_events
               SET slack_team_id = team_id
             WHERE slack_team_id = '' AND team_id <> '';
            ALTER TABLE public.slack_thread_events DROP COLUMN team_id;
        END IF;
    END IF;

    IF EXISTS (
        SELECT 1 FROM information_schema.columns
         WHERE table_schema = 'public'
           AND table_name = 'slack_thread_events'
           AND column_name = 'channel_resolved_at'
    ) THEN
        IF NOT EXISTS (
            SELECT 1 FROM information_schema.columns
             WHERE table_schema = 'public'
               AND table_name = 'slack_thread_events'
               AND column_name = 'route_resolved_at'
        ) THEN
            ALTER TABLE public.slack_thread_events RENAME COLUMN channel_resolved_at TO route_resolved_at;
        ELSE
            UPDATE public.slack_thread_events
               SET route_resolved_at = COALESCE(route_resolved_at, channel_resolved_at);
            ALTER TABLE public.slack_thread_events DROP COLUMN channel_resolved_at;
        END IF;
    END IF;
END $$;
-- +goose StatementEnd

ALTER TABLE public.slack_thread_events
    ADD COLUMN IF NOT EXISTS slack_team_id text DEFAULT ''::text NOT NULL,
    ADD COLUMN IF NOT EXISTS team_id uuid,
    ADD COLUMN IF NOT EXISTS agent_id uuid,
    ADD COLUMN IF NOT EXISTS route_resolved_at timestamp with time zone;

CREATE TABLE IF NOT EXISTS public.agent_memory_digests (
    agent_id uuid NOT NULL,
    org_id uuid NOT NULL,
    content text NOT NULL,
    observation_count integer DEFAULT 0 NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT agent_memory_digests_pkey PRIMARY KEY (agent_id),
    CONSTRAINT fk_agent_memory_digests_agent FOREIGN KEY (agent_id) REFERENCES public.agents(id) ON DELETE CASCADE,
    CONSTRAINT fk_agent_memory_digests_org FOREIGN KEY (org_id) REFERENCES public.orgs(id) ON DELETE CASCADE
);

CREATE TABLE IF NOT EXISTS public.team_external_resource_routes (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    org_id uuid NOT NULL,
    team_id uuid NOT NULL,
    connection_id uuid NOT NULL,
    agent_id uuid NOT NULL,
    resource_type text NOT NULL,
    resource_key text NOT NULL,
    resource_name text DEFAULT ''::text NOT NULL,
    resource_url text DEFAULT ''::text NOT NULL,
    metadata jsonb DEFAULT '{}'::jsonb NOT NULL,
    created_by uuid,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT team_external_resource_routes_pkey PRIMARY KEY (id),
    CONSTRAINT team_external_resource_routes_org_fkey FOREIGN KEY (org_id) REFERENCES public.orgs(id) ON DELETE CASCADE,
    CONSTRAINT team_external_resource_routes_team_fkey FOREIGN KEY (team_id) REFERENCES public.teams(id) ON DELETE CASCADE,
    CONSTRAINT team_external_resource_routes_connection_fkey FOREIGN KEY (connection_id) REFERENCES public.connections(id) ON DELETE CASCADE,
    CONSTRAINT team_external_resource_routes_agent_fkey FOREIGN KEY (agent_id) REFERENCES public.agents(id) ON DELETE RESTRICT,
    CONSTRAINT team_external_resource_routes_created_by_fkey FOREIGN KEY (created_by) REFERENCES public.users(id) ON DELETE SET NULL
);

-- Required by the idempotent route backfill below. The remaining lookup
-- indexes are created after legacy channel columns have been removed.
CREATE UNIQUE INDEX IF NOT EXISTS idx_team_external_resource_routes_resource
    ON public.team_external_resource_routes (connection_id, resource_type, resource_key);

-- Migrate legacy rows only when the old channel table is present. A fresh
-- database has no channels and safely skips this entire block.
-- +goose StatementBegin
DO $$
BEGIN
IF to_regclass('public.channels') IS NULL THEN
    RETURN;
END IF;

-- Carry channel-owned presentation and memory settings to the agent that
-- represented that channel.
UPDATE public.agents AS a
   SET image_model = CASE WHEN a.image_model = '' THEN c.image_model ELSE a.image_model END,
       vector_image_model = CASE WHEN a.vector_image_model = '' THEN c.vector_image_model ELSE a.vector_image_model END,
       memory_mission = COALESCE(a.memory_mission, c.memory_mission)
  FROM public.channels AS c
 WHERE c.default_agent_id = a.id;

-- Channel members became members of the channel's team. Existing team
-- memberships win so no role or deactivation state is overwritten.
IF to_regclass('public.channel_members') IS NOT NULL THEN
    INSERT INTO public.team_members (id, org_id, team_id, user_id, role, created_at, updated_at, deactivated_at)
    SELECT gen_random_uuid(), c.org_id, c.team_id, cm.user_id, cm.role,
           COALESCE(cm.created_at, now()), COALESCE(cm.created_at, now()), cm.deactivated_at
      FROM public.channel_members AS cm
      JOIN public.channels AS c ON c.id = cm.channel_id
     WHERE c.team_id IS NOT NULL
    ON CONFLICT (team_id, user_id) DO NOTHING;
END IF;

UPDATE public.sessions AS s
   SET team_id = COALESCE(
       (SELECT c.team_id FROM public.channels AS c WHERE c.id = s.channel_id),
       (SELECT a.team_id FROM public.agents AS a WHERE a.id = s.agent_id)
   )
 WHERE s.team_id IS NULL;

UPDATE public.sheets AS sh
   SET team_id = COALESCE(
       (SELECT c.team_id FROM public.channels AS c WHERE c.id = sh.channel_id),
       (SELECT a.team_id FROM public.agents AS a WHERE a.id = sh.created_by_agent_id),
       (SELECT s.team_id FROM public.sessions AS s WHERE s.id = sh.source_session_id)
   )
 WHERE sh.team_id IS NULL;

UPDATE public.apps AS app
   SET team_id = COALESCE(
       (SELECT c.team_id FROM public.channels AS c WHERE c.id = app.channel_id),
       (SELECT a.team_id FROM public.agents AS a WHERE a.id = app.created_by_agent_id),
       (SELECT s.team_id FROM public.sessions AS s WHERE s.id = app.source_session_id)
   )
 WHERE app.team_id IS NULL;

UPDATE public.agent_directives AS d
   SET agent_id = COALESCE(
       (SELECT c.default_agent_id FROM public.channels AS c WHERE c.id = d.channel_id),
       (SELECT min(a.id::text)::uuid
          FROM public.agents AS a
         WHERE a.org_id = d.org_id
           AND a.type = 'agent'
           AND a.status <> 'archived'
        HAVING count(*) = 1)
   )
 WHERE d.agent_id IS NULL;

UPDATE public.agent_memories AS m
   SET agent_id = COALESCE(
       (SELECT s.agent_id FROM public.sessions AS s WHERE s.id = m.source_session_id),
       (SELECT c.default_agent_id FROM public.channels AS c WHERE c.id = m.channel_id),
       (SELECT min(a.id::text)::uuid
          FROM public.agents AS a
         WHERE a.org_id = m.org_id
           AND a.type = 'agent'
           AND a.status <> 'archived'
        HAVING count(*) = 1)
   )
 WHERE m.agent_id IS NULL;

UPDATE public.agent_observations AS o
   SET agent_id = COALESCE(
       (SELECT min(m.agent_id::text)::uuid
          FROM unnest(o.source_fact_ids) AS fact_id
          JOIN public.agent_memories AS m ON m.id = fact_id
        HAVING count(DISTINCT m.agent_id) = 1),
       (SELECT c.default_agent_id FROM public.channels AS c WHERE c.id = o.channel_id),
       (SELECT min(a.id::text)::uuid
          FROM public.agents AS a
         WHERE a.org_id = o.org_id
           AND a.type = 'agent'
           AND a.status <> 'archived'
        HAVING count(*) = 1)
   )
 WHERE o.agent_id IS NULL;

UPDATE public.memory_suppressions AS ms
   SET agent_id = COALESCE(
       (SELECT c.default_agent_id FROM public.channels AS c WHERE c.id = ms.channel_id),
       (SELECT min(a.id::text)::uuid
          FROM public.agents AS a
         WHERE a.org_id = ms.org_id
           AND a.type = 'agent'
           AND a.status <> 'archived'
        HAVING count(*) = 1)
   )
 WHERE ms.agent_id IS NULL;

UPDATE public.slack_thread_events AS ste
   SET team_id = COALESCE(
           (SELECT c.team_id FROM public.channels AS c WHERE c.id = ste.channel_id),
           (SELECT s.team_id FROM public.sessions AS s WHERE s.id = ste.session_id),
           (SELECT a.team_id
              FROM public.agent_triggers AS t
              JOIN public.agents AS a ON a.id = t.agent_id
             WHERE t.id = ste.trigger_id)
       ),
       agent_id = COALESCE(
           (SELECT c.default_agent_id FROM public.channels AS c WHERE c.id = ste.channel_id),
           (SELECT s.agent_id FROM public.sessions AS s WHERE s.id = ste.session_id),
           (SELECT t.agent_id FROM public.agent_triggers AS t WHERE t.id = ste.trigger_id)
       )
 WHERE ste.team_id IS NULL OR ste.agent_id IS NULL;

UPDATE public.agent_triggers AS t
   SET resource_type = CASE
           WHEN c.external_provider = 'slack' AND c.external_resource_type = 'channel' THEN 'slack_channel'
           ELSE c.external_resource_type
       END,
       resource_key = c.external_resource_key,
       resource_name = COALESCE(NULLIF(c.external_resource_name, ''), c.name)
  FROM public.channels AS c
 WHERE c.id = t.channel_id;

INSERT INTO public.team_external_resource_routes (
    org_id, team_id, connection_id, agent_id, resource_type, resource_key,
    resource_name, resource_url, metadata, created_by, created_at, updated_at
)
SELECT c.org_id,
       c.team_id,
       c.external_connection_id,
       c.default_agent_id,
       CASE
           WHEN c.external_provider = 'slack' AND c.external_resource_type = 'channel' THEN 'slack_channel'
           ELSE c.external_resource_type
       END,
       c.external_resource_key,
       COALESCE(NULLIF(c.external_resource_name, ''), c.name),
       c.external_resource_url,
       c.external_metadata,
       c.created_by,
       COALESCE(c.created_at, now()),
       COALESCE(c.updated_at, now())
  FROM public.channels AS c
 WHERE c.team_id IS NOT NULL
   AND c.external_connection_id IS NOT NULL
   AND c.default_agent_id IS NOT NULL
   AND c.external_resource_type <> ''
   AND c.external_resource_key <> ''
ON CONFLICT (connection_id, resource_type, resource_key) DO NOTHING;

IF to_regclass('public.channel_memory_digests') IS NOT NULL THEN
    INSERT INTO public.agent_memory_digests (agent_id, org_id, content, observation_count, updated_at)
    SELECT DISTINCT ON (c.default_agent_id)
           c.default_agent_id, d.org_id, d.content, d.observation_count, d.updated_at
      FROM public.channel_memory_digests AS d
      JOIN public.channels AS c ON c.id = d.channel_id
     WHERE c.default_agent_id IS NOT NULL
     ORDER BY c.default_agent_id, d.updated_at DESC, d.channel_id
    ON CONFLICT (agent_id) DO NOTHING;
END IF;

-- Abort before destructive DDL if any required row could not be mapped.
IF EXISTS (SELECT 1 FROM public.sessions WHERE team_id IS NULL) THEN
    RAISE EXCEPTION 'channel-to-team migration: sessions contain unmapped team IDs';
END IF;
IF EXISTS (SELECT 1 FROM public.sheets WHERE team_id IS NULL) THEN
    RAISE EXCEPTION 'channel-to-team migration: sheets contain unmapped team IDs';
END IF;
IF EXISTS (SELECT 1 FROM public.apps WHERE team_id IS NULL) THEN
    RAISE EXCEPTION 'channel-to-team migration: apps contain unmapped team IDs';
END IF;
IF EXISTS (SELECT 1 FROM public.agent_directives WHERE agent_id IS NULL) THEN
    RAISE EXCEPTION 'channel-to-team migration: agent directives contain unmapped agent IDs';
END IF;
IF EXISTS (SELECT 1 FROM public.agent_memories WHERE agent_id IS NULL) THEN
    RAISE EXCEPTION 'channel-to-team migration: agent memories contain unmapped agent IDs';
END IF;
IF EXISTS (SELECT 1 FROM public.agent_observations WHERE agent_id IS NULL) THEN
    RAISE EXCEPTION 'channel-to-team migration: agent observations contain unmapped agent IDs';
END IF;
IF EXISTS (SELECT 1 FROM public.memory_suppressions WHERE agent_id IS NULL) THEN
    RAISE EXCEPTION 'channel-to-team migration: memory suppressions contain unmapped agent IDs';
END IF;
IF EXISTS (
    SELECT 1
      FROM public.agents
     WHERE is_default AND type = 'agent' AND status <> 'archived'
     GROUP BY team_id
    HAVING count(*) > 1
) THEN
    RAISE EXCEPTION 'channel-to-team migration: multiple active default agents exist for a team';
END IF;
IF EXISTS (
    SELECT 1
      FROM public.memory_suppressions
     GROUP BY org_id, agent_id, content_fingerprint
    HAVING count(*) > 1
) THEN
    RAISE EXCEPTION 'channel-to-team migration: duplicate agent memory suppressions would be created';
END IF;
IF EXISTS (
    SELECT 1
      FROM public.agent_triggers
     WHERE enabled AND trigger_key <> '' AND trigger_value <> ''
     GROUP BY org_id, connection_id, resource_type, resource_key, trigger_key, trigger_value
    HAVING count(*) > 1
) THEN
    RAISE EXCEPTION 'channel-to-team migration: duplicate resource trigger routes would be created';
END IF;

END $$;
-- +goose StatementEnd

-- Remove legacy constraints and indexes before replacing their columns.
ALTER TABLE public.agent_directives DROP CONSTRAINT IF EXISTS agent_directives_channel_id_fkey;
ALTER TABLE public.agent_memories DROP CONSTRAINT IF EXISTS fk_agent_memories_channel;
ALTER TABLE public.agent_observations DROP CONSTRAINT IF EXISTS fk_agent_observations_channel;
ALTER TABLE public.agent_triggers DROP CONSTRAINT IF EXISTS fk_agent_triggers_channel;
ALTER TABLE public.apps DROP CONSTRAINT IF EXISTS apps_channel_id_fkey;
ALTER TABLE public.memory_suppressions
    DROP CONSTRAINT IF EXISTS fk_memory_suppressions_channel,
    DROP CONSTRAINT IF EXISTS memory_suppressions_unique;
ALTER TABLE public.sessions DROP CONSTRAINT IF EXISTS fk_sessions_channel;
ALTER TABLE public.sheets DROP CONSTRAINT IF EXISTS sheets_channel_id_fkey;
ALTER TABLE public.slack_thread_events DROP CONSTRAINT IF EXISTS fk_slack_thread_events_channel;

DROP INDEX IF EXISTS public.idx_agent_directives_org_channel;
DROP INDEX IF EXISTS public.idx_agent_memories_org_channel;
DROP INDEX IF EXISTS public.idx_agent_memories_unconsolidated;
DROP INDEX IF EXISTS public.idx_agent_observations_org_channel;
DROP INDEX IF EXISTS public.idx_agent_triggers_channel_id;
DROP INDEX IF EXISTS public.idx_agent_triggers_enabled_key_value;
DROP INDEX IF EXISTS public.idx_apps_channel_updated_active;
DROP INDEX IF EXISTS public.idx_sessions_channel;
DROP INDEX IF EXISTS public.idx_sheets_channel_updated_active;

ALTER TABLE public.agent_directives DROP COLUMN IF EXISTS channel_id;
ALTER TABLE public.agent_memories DROP COLUMN IF EXISTS channel_id;
ALTER TABLE public.agent_observations DROP COLUMN IF EXISTS channel_id;
ALTER TABLE public.agent_triggers DROP COLUMN IF EXISTS channel_id;
ALTER TABLE public.apps DROP COLUMN IF EXISTS channel_id;
ALTER TABLE public.memory_suppressions DROP COLUMN IF EXISTS channel_id;
ALTER TABLE public.sessions DROP COLUMN IF EXISTS channel_id;
ALTER TABLE public.sheets DROP COLUMN IF EXISTS channel_id;
ALTER TABLE public.slack_thread_events DROP COLUMN IF EXISTS channel_id;

ALTER TABLE public.agent_directives ALTER COLUMN agent_id SET NOT NULL;
ALTER TABLE public.agent_memories ALTER COLUMN agent_id SET NOT NULL;
ALTER TABLE public.agent_observations ALTER COLUMN agent_id SET NOT NULL;
ALTER TABLE public.memory_suppressions ALTER COLUMN agent_id SET NOT NULL;
ALTER TABLE public.sessions ALTER COLUMN team_id SET NOT NULL;
ALTER TABLE public.sheets ALTER COLUMN team_id SET NOT NULL;
ALTER TABLE public.apps ALTER COLUMN team_id SET NOT NULL;

-- Add the current foreign-key set without assuming whether 000001 already
-- supplied it.
-- +goose StatementBegin
DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conrelid = 'public.agent_directives'::regclass AND conname = 'agent_directives_agent_id_fkey') THEN
        ALTER TABLE public.agent_directives ADD CONSTRAINT agent_directives_agent_id_fkey FOREIGN KEY (agent_id) REFERENCES public.agents(id) ON DELETE CASCADE;
    END IF;
    IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conrelid = 'public.agent_memories'::regclass AND conname = 'fk_agent_memories_agent') THEN
        ALTER TABLE public.agent_memories ADD CONSTRAINT fk_agent_memories_agent FOREIGN KEY (agent_id) REFERENCES public.agents(id) ON DELETE CASCADE;
    END IF;
    IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conrelid = 'public.agent_observations'::regclass AND conname = 'fk_agent_observations_agent') THEN
        ALTER TABLE public.agent_observations ADD CONSTRAINT fk_agent_observations_agent FOREIGN KEY (agent_id) REFERENCES public.agents(id) ON DELETE CASCADE;
    END IF;
    IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conrelid = 'public.memory_suppressions'::regclass AND conname = 'fk_memory_suppressions_agent') THEN
        ALTER TABLE public.memory_suppressions ADD CONSTRAINT fk_memory_suppressions_agent FOREIGN KEY (agent_id) REFERENCES public.agents(id) ON DELETE CASCADE;
    END IF;
    IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conrelid = 'public.memory_suppressions'::regclass AND conname = 'memory_suppressions_unique') THEN
        ALTER TABLE public.memory_suppressions ADD CONSTRAINT memory_suppressions_unique UNIQUE (org_id, agent_id, content_fingerprint);
    END IF;
    IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conrelid = 'public.sessions'::regclass AND conname = 'fk_sessions_team') THEN
        ALTER TABLE public.sessions ADD CONSTRAINT fk_sessions_team FOREIGN KEY (team_id) REFERENCES public.teams(id) ON DELETE RESTRICT;
    END IF;
    IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conrelid = 'public.sheets'::regclass AND conname = 'sheets_team_id_fkey') THEN
        ALTER TABLE public.sheets ADD CONSTRAINT sheets_team_id_fkey FOREIGN KEY (team_id) REFERENCES public.teams(id) ON DELETE RESTRICT;
    END IF;
    IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conrelid = 'public.apps'::regclass AND conname = 'apps_team_id_fkey') THEN
        ALTER TABLE public.apps ADD CONSTRAINT apps_team_id_fkey FOREIGN KEY (team_id) REFERENCES public.teams(id) ON DELETE RESTRICT;
    END IF;
    IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conrelid = 'public.slack_thread_events'::regclass AND conname = 'fk_slack_thread_events_team') THEN
        ALTER TABLE public.slack_thread_events ADD CONSTRAINT fk_slack_thread_events_team FOREIGN KEY (team_id) REFERENCES public.teams(id) ON DELETE SET NULL;
    END IF;
    IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conrelid = 'public.slack_thread_events'::regclass AND conname = 'fk_slack_thread_events_agent') THEN
        ALTER TABLE public.slack_thread_events ADD CONSTRAINT fk_slack_thread_events_agent FOREIGN KEY (agent_id) REFERENCES public.agents(id) ON DELETE SET NULL;
    END IF;
END $$;
-- +goose StatementEnd

CREATE INDEX IF NOT EXISTS idx_agent_directives_org_agent ON public.agent_directives (org_id, agent_id) WHERE active;
CREATE INDEX IF NOT EXISTS idx_agent_memories_org_agent ON public.agent_memories (org_id, agent_id, created_at DESC) WHERE archived_at IS NULL;
CREATE INDEX IF NOT EXISTS idx_agent_memories_unconsolidated ON public.agent_memories (org_id, agent_id, created_at) WHERE archived_at IS NULL AND consolidated_at IS NULL;
CREATE INDEX IF NOT EXISTS idx_agent_observations_org_agent ON public.agent_observations (org_id, agent_id) WHERE archived_at IS NULL;
CREATE UNIQUE INDEX IF NOT EXISTS idx_agent_triggers_enabled_key_value ON public.agent_triggers (org_id, connection_id, resource_type, resource_key, trigger_key, trigger_value) WHERE enabled = true AND trigger_key <> '' AND trigger_value <> '';
CREATE UNIQUE INDEX IF NOT EXISTS idx_agents_default_team_active ON public.agents (team_id) WHERE is_default = true AND type = 'agent' AND status <> 'archived';
CREATE INDEX IF NOT EXISTS idx_apps_team_updated_active ON public.apps (team_id, updated_at DESC) WHERE archived_at IS NULL;
CREATE INDEX IF NOT EXISTS idx_sessions_team ON public.sessions (team_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_sheets_team_updated_active ON public.sheets (team_id, updated_at DESC) WHERE archived_at IS NULL;
CREATE INDEX IF NOT EXISTS idx_team_external_resource_routes_org ON public.team_external_resource_routes (org_id);
CREATE INDEX IF NOT EXISTS idx_team_external_resource_routes_team ON public.team_external_resource_routes (team_id);
CREATE INDEX IF NOT EXISTS idx_team_external_resource_routes_agent ON public.team_external_resource_routes (agent_id);
CREATE UNIQUE INDEX IF NOT EXISTS idx_team_external_resource_routes_resource ON public.team_external_resource_routes (connection_id, resource_type, resource_key);

-- Drop legacy channel storage only after every mapping, assertion, constraint,
-- and replacement index above has succeeded.
DROP TABLE IF EXISTS public.channel_members;
DROP TABLE IF EXISTS public.channel_memory_digests;
DROP TABLE IF EXISTS public.channels;

-- This is an intentionally irreversible ownership-model migration. Restoring
-- a database backup is safer than attempting to synthesize channels on down.
