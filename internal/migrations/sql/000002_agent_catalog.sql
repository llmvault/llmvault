-- +goose Up
CREATE TABLE public.agent_catalog (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    slug text NOT NULL,
    name text NOT NULL,
    description text DEFAULT ''::text NOT NULL,
    category text DEFAULT ''::text NOT NULL,
    avatar_url text DEFAULT ''::text NOT NULL,
    developer text DEFAULT 'Hivy'::text NOT NULL,
    official boolean DEFAULT false NOT NULL,
    is_default boolean DEFAULT false NOT NULL,
    model text DEFAULT ''::text NOT NULL,
    instructions text DEFAULT ''::text NOT NULL,
    required_plugins text[] DEFAULT '{}'::text[] NOT NULL,
    recommended_plugins text[] DEFAULT '{}'::text[] NOT NULL,
    manifest jsonb DEFAULT '{}'::jsonb NOT NULL,
    source_hash text DEFAULT ''::text NOT NULL,
    status text DEFAULT 'active'::text NOT NULL,
    created_at timestamp with time zone,
    updated_at timestamp with time zone,
    sub_agents jsonb DEFAULT '{}'::jsonb NOT NULL,
    sandbox_image text DEFAULT 'default'::text NOT NULL,
    tools jsonb DEFAULT '{}'::jsonb NOT NULL,
    default_reasoning_effort text DEFAULT ''::text NOT NULL,
    auto_load_skills jsonb DEFAULT '[]'::jsonb NOT NULL,
    CONSTRAINT agent_catalog_sandbox_image_valid CHECK ((sandbox_image = ANY (ARRAY['default'::text, 'developer'::text])))
);

ALTER TABLE ONLY public.agent_catalog
    ADD CONSTRAINT agent_catalog_pkey PRIMARY KEY (id);

CREATE INDEX idx_agent_catalog_default ON public.agent_catalog USING btree (is_default);

CREATE UNIQUE INDEX idx_agent_catalog_slug ON public.agent_catalog USING btree (slug);

CREATE INDEX idx_agent_catalog_status ON public.agent_catalog USING btree (status);

-- +goose Down
DROP TABLE IF EXISTS public.agent_catalog CASCADE;
