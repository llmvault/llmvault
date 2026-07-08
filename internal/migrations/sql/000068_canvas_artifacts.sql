-- +goose Up
CREATE TABLE public.canvas_artifacts (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    org_id uuid NOT NULL,
    canvas_project_id uuid NOT NULL,
    slug text NOT NULL,
    type text NOT NULL,
    name text DEFAULT ''::text NOT NULL,
    manifest_json jsonb DEFAULT '{}'::jsonb NOT NULL,
    source_session_id uuid,
    created_by_agent_id uuid,
    created_by_user_id uuid,
    archived_at timestamp with time zone,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT canvas_artifacts_type_check CHECK ((type = ANY (ARRAY['web_page'::text, 'presentation'::text])))
);

ALTER TABLE ONLY public.canvas_artifacts
    ADD CONSTRAINT canvas_artifacts_pkey PRIMARY KEY (id);

CREATE INDEX idx_canvas_artifacts_archived_at ON public.canvas_artifacts USING btree (archived_at);

CREATE INDEX idx_canvas_artifacts_org_project ON public.canvas_artifacts USING btree (org_id, canvas_project_id) WHERE (archived_at IS NULL);

CREATE UNIQUE INDEX idx_canvas_artifacts_project_slug_active ON public.canvas_artifacts USING btree (canvas_project_id, slug) WHERE (archived_at IS NULL);

CREATE INDEX idx_canvas_artifacts_source_session ON public.canvas_artifacts USING btree (source_session_id) WHERE (source_session_id IS NOT NULL);

ALTER TABLE ONLY public.canvas_artifacts
    ADD CONSTRAINT canvas_artifacts_canvas_project_id_fkey FOREIGN KEY (canvas_project_id) REFERENCES public.canvas_projects(id) ON DELETE CASCADE;

ALTER TABLE ONLY public.canvas_artifacts
    ADD CONSTRAINT canvas_artifacts_created_by_agent_id_fkey FOREIGN KEY (created_by_agent_id) REFERENCES public.agents(id) ON DELETE SET NULL;

ALTER TABLE ONLY public.canvas_artifacts
    ADD CONSTRAINT canvas_artifacts_created_by_user_id_fkey FOREIGN KEY (created_by_user_id) REFERENCES public.users(id) ON DELETE SET NULL;

ALTER TABLE ONLY public.canvas_artifacts
    ADD CONSTRAINT canvas_artifacts_org_id_fkey FOREIGN KEY (org_id) REFERENCES public.orgs(id) ON DELETE CASCADE;

ALTER TABLE ONLY public.canvas_artifacts
    ADD CONSTRAINT canvas_artifacts_source_session_id_fkey FOREIGN KEY (source_session_id) REFERENCES public.sessions(id) ON DELETE SET NULL;

-- +goose Down
DROP TABLE IF EXISTS public.canvas_artifacts CASCADE;
