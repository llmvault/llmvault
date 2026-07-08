-- +goose Up
CREATE TABLE public.skills (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    org_id uuid,
    publisher_id uuid,
    slug text NOT NULL,
    name text NOT NULL,
    description text,
    category character varying(64) DEFAULT ''::character varying NOT NULL,
    source_type text NOT NULL,
    repo_url text,
    repo_subpath text,
    repo_ref text DEFAULT 'main'::text NOT NULL,
    bundle jsonb DEFAULT '{}'::jsonb NOT NULL,
    hydrated_commit_sha text,
    hydrated_at timestamp with time zone,
    hydration_error text,
    tags text[] DEFAULT '{}'::text[],
    integration_ids text[] DEFAULT '{}'::text[],
    install_count bigint DEFAULT 0 NOT NULL,
    featured boolean DEFAULT false NOT NULL,
    hidden boolean DEFAULT false NOT NULL,
    verified_at timestamp with time zone,
    status text DEFAULT 'draft'::text NOT NULL,
    public_skill_id uuid,
    origin_skill_id uuid,
    origin_org_id uuid,
    published_at timestamp with time zone,
    created_at timestamp with time zone,
    updated_at timestamp with time zone,
    plugin_id uuid,
    human_description text
);

ALTER TABLE ONLY public.skills
    ADD CONSTRAINT skills_pkey PRIMARY KEY (id);

CREATE INDEX idx_skills_category ON public.skills USING btree (category);

CREATE INDEX idx_skills_featured ON public.skills USING btree (featured);

CREATE INDEX idx_skills_hidden ON public.skills USING btree (hidden);

CREATE INDEX idx_skills_org_id ON public.skills USING btree (org_id);

CREATE INDEX idx_skills_origin_skill_id ON public.skills USING btree (origin_skill_id);

CREATE INDEX idx_skills_plugin_id ON public.skills USING btree (plugin_id);

CREATE INDEX idx_skills_public_skill_id ON public.skills USING btree (public_skill_id);

CREATE INDEX idx_skills_publisher_id ON public.skills USING btree (publisher_id);

CREATE INDEX idx_skills_slug ON public.skills USING btree (slug);

CREATE INDEX idx_skills_status ON public.skills USING btree (status);

ALTER TABLE ONLY public.skills
    ADD CONSTRAINT fk_skills_org FOREIGN KEY (org_id) REFERENCES public.orgs(id) ON DELETE CASCADE;

ALTER TABLE ONLY public.skills
    ADD CONSTRAINT fk_skills_plugin FOREIGN KEY (plugin_id) REFERENCES public.plugins(id) ON DELETE CASCADE;

ALTER TABLE ONLY public.skills
    ADD CONSTRAINT fk_skills_publisher FOREIGN KEY (publisher_id) REFERENCES public.users(id) ON DELETE SET NULL;

-- +goose Down
DROP TABLE IF EXISTS public.skills CASCADE;
