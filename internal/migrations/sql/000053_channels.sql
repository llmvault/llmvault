-- +goose Up
CREATE TABLE public.channels (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    org_id uuid NOT NULL,
    name text NOT NULL,
    description text DEFAULT ''::text NOT NULL,
    kind text DEFAULT 'standard'::text NOT NULL,
    visibility text DEFAULT 'public'::text NOT NULL,
    default_agent_id uuid NOT NULL,
    is_default boolean DEFAULT false NOT NULL,
    origin text DEFAULT 'native'::text NOT NULL,
    external_provider text DEFAULT ''::text NOT NULL,
    external_connection_id uuid,
    external_workspace_key text DEFAULT ''::text NOT NULL,
    external_resource_type text DEFAULT 'channel'::text NOT NULL,
    external_resource_key text DEFAULT ''::text NOT NULL,
    external_resource_name text DEFAULT ''::text NOT NULL,
    external_resource_url text DEFAULT ''::text NOT NULL,
    external_metadata jsonb DEFAULT '{}'::jsonb NOT NULL,
    created_by uuid,
    archived_at timestamp with time zone,
    created_at timestamp with time zone,
    updated_at timestamp with time zone,
    team_id uuid,
    image_model text DEFAULT ''::text NOT NULL,
    vector_image_model text DEFAULT ''::text NOT NULL,
    expose_org_memories boolean DEFAULT true NOT NULL,
    category text DEFAULT 'general'::text NOT NULL,
    memory_mission text
);

ALTER TABLE ONLY public.channels
    ADD CONSTRAINT channels_pkey PRIMARY KEY (id);

CREATE INDEX idx_channels_archived_at ON public.channels USING btree (archived_at);

CREATE INDEX idx_channels_default_agent_id ON public.channels USING btree (default_agent_id);

CREATE INDEX idx_channels_external_connection_id ON public.channels USING btree (external_connection_id);

CREATE INDEX idx_channels_external_provider ON public.channels USING btree (org_id, external_provider);

CREATE INDEX idx_channels_is_default ON public.channels USING btree (is_default);

CREATE UNIQUE INDEX idx_channels_org_external_resource ON public.channels USING btree (org_id, external_provider, external_workspace_key, external_resource_type, external_resource_key) WHERE (external_resource_key <> ''::text);

CREATE UNIQUE INDEX idx_channels_org_source_name ON public.channels USING btree (org_id, COALESCE(team_id, '00000000-0000-0000-0000-000000000000'::uuid), origin, external_provider, external_workspace_key, external_resource_type, name) WHERE (archived_at IS NULL);

CREATE INDEX idx_channels_origin ON public.channels USING btree (origin);

CREATE INDEX idx_channels_team_id ON public.channels USING btree (team_id);

ALTER TABLE ONLY public.channels
    ADD CONSTRAINT fk_channels_creator FOREIGN KEY (created_by) REFERENCES public.users(id) ON DELETE SET NULL;

ALTER TABLE ONLY public.channels
    ADD CONSTRAINT fk_channels_default_agent FOREIGN KEY (default_agent_id) REFERENCES public.agents(id) ON DELETE RESTRICT;

ALTER TABLE ONLY public.channels
    ADD CONSTRAINT fk_channels_external_connection FOREIGN KEY (external_connection_id) REFERENCES public.connections(id) ON DELETE SET NULL;

ALTER TABLE ONLY public.channels
    ADD CONSTRAINT fk_channels_org FOREIGN KEY (org_id) REFERENCES public.orgs(id) ON DELETE CASCADE;

ALTER TABLE ONLY public.channels
    ADD CONSTRAINT fk_channels_team FOREIGN KEY (team_id) REFERENCES public.teams(id) ON DELETE SET NULL;
