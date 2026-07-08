-- +goose Up
CREATE TABLE public.agents (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    org_id uuid NOT NULL,
    name text NOT NULL,
    description text DEFAULT ''::text NOT NULL,
    avatar_url text,
    icon text DEFAULT ''::text NOT NULL,
    placeholder text DEFAULT ''::text NOT NULL,
    is_default boolean DEFAULT false NOT NULL,
    workspace_snapshot_id uuid,
    sandbox_template_id uuid,
    instructions text,
    model text NOT NULL,
    tools jsonb DEFAULT '{}'::jsonb NOT NULL,
    mcp_servers jsonb DEFAULT '[]'::jsonb NOT NULL,
    skills jsonb DEFAULT '{}'::jsonb NOT NULL,
    runtime_config jsonb DEFAULT '{}'::jsonb NOT NULL,
    permissions jsonb DEFAULT '{}'::jsonb NOT NULL,
    resources jsonb DEFAULT '{}'::jsonb NOT NULL,
    sandbox_tools text[] DEFAULT '{}'::text[],
    setup_commands text[] DEFAULT '{}'::text[],
    status text DEFAULT 'active'::text NOT NULL,
    last_proxy_token_refreshed_at timestamp with time zone,
    created_at timestamp with time zone,
    updated_at timestamp with time zone,
    agent_catalog_id uuid,
    sandbox_size text DEFAULT 'small'::text NOT NULL,
    sandbox_image text DEFAULT 'default'::text NOT NULL,
    image_model text DEFAULT ''::text NOT NULL,
    vector_image_model text DEFAULT ''::text NOT NULL,
    type text DEFAULT 'agent'::text NOT NULL,
    parent_agent_id uuid,
    mcp_tool_filter jsonb,
    default_reasoning_effort text,
    auto_load_skills jsonb DEFAULT '[]'::jsonb NOT NULL,
    team_id uuid,
    instructions_snapshot text,
    CONSTRAINT agents_sandbox_image_valid CHECK ((sandbox_image = ANY (ARRAY['default'::text, 'developer'::text]))),
    CONSTRAINT agents_sandbox_size_check CHECK ((sandbox_size = ANY (ARRAY['nano'::text, 'small'::text, 'medium'::text, 'large'::text, 'xlarge'::text]))),
    CONSTRAINT agents_type_check CHECK ((type = ANY (ARRAY['agent'::text, 'subagent'::text])))
);

ALTER TABLE ONLY public.agents
    ADD CONSTRAINT agents_pkey PRIMARY KEY (id);

CREATE INDEX idx_agent_org_id ON public.agents USING btree (org_id);

CREATE INDEX idx_agents_agent_catalog_id ON public.agents USING btree (agent_catalog_id);

CREATE INDEX idx_agents_is_default ON public.agents USING btree (is_default);

CREATE UNIQUE INDEX idx_agents_org_agent_catalog_team_active ON public.agents USING btree (org_id, agent_catalog_id, COALESCE(team_id, '00000000-0000-0000-0000-000000000000'::uuid)) WHERE ((agent_catalog_id IS NOT NULL) AND (status <> 'archived'::text));

CREATE UNIQUE INDEX idx_agents_org_name ON public.agents USING btree (org_id, name) WHERE (parent_agent_id IS NULL);

CREATE INDEX idx_agents_parent_agent_id ON public.agents USING btree (parent_agent_id);

CREATE UNIQUE INDEX idx_agents_parent_name ON public.agents USING btree (parent_agent_id, name) WHERE (parent_agent_id IS NOT NULL);

CREATE INDEX idx_agents_team_id ON public.agents USING btree (team_id);

ALTER TABLE ONLY public.agents
    ADD CONSTRAINT agents_team_id_fkey FOREIGN KEY (team_id) REFERENCES public.teams(id) ON DELETE SET NULL;

ALTER TABLE ONLY public.agents
    ADD CONSTRAINT fk_agents_agent_catalog FOREIGN KEY (agent_catalog_id) REFERENCES public.agent_catalog(id) ON DELETE SET NULL;

ALTER TABLE ONLY public.agents
    ADD CONSTRAINT fk_agents_org FOREIGN KEY (org_id) REFERENCES public.orgs(id) ON DELETE CASCADE;

ALTER TABLE ONLY public.agents
    ADD CONSTRAINT fk_agents_parent_agent FOREIGN KEY (parent_agent_id) REFERENCES public.agents(id) ON DELETE CASCADE;

ALTER TABLE ONLY public.agents
    ADD CONSTRAINT fk_agents_sandbox_template FOREIGN KEY (sandbox_template_id) REFERENCES public.sandbox_templates(id) ON DELETE SET NULL;
