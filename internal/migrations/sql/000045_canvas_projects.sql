-- +goose Up
CREATE TABLE public.canvas_projects (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    org_id uuid NOT NULL,
    name text DEFAULT ''::text NOT NULL,
    created_by_agent_id uuid,
    created_by_user_id uuid,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    slug text DEFAULT ''::text NOT NULL,
    description text DEFAULT ''::text NOT NULL,
    archived_at timestamp with time zone
);

ALTER TABLE ONLY public.canvas_projects
    ADD CONSTRAINT canvas_projects_pkey PRIMARY KEY (id);

CREATE INDEX idx_canvas_projects_archived_at ON public.canvas_projects USING btree (archived_at);

CREATE INDEX idx_canvas_projects_org_id ON public.canvas_projects USING btree (org_id);

CREATE UNIQUE INDEX idx_canvas_projects_org_slug_active ON public.canvas_projects USING btree (org_id, slug) WHERE ((archived_at IS NULL) AND (slug <> ''::text));

CREATE INDEX idx_canvas_projects_org_updated_active ON public.canvas_projects USING btree (org_id, updated_at DESC) WHERE (archived_at IS NULL);

ALTER TABLE ONLY public.canvas_projects
    ADD CONSTRAINT canvas_projects_created_by_agent_id_fkey FOREIGN KEY (created_by_agent_id) REFERENCES public.agents(id) ON DELETE SET NULL;

ALTER TABLE ONLY public.canvas_projects
    ADD CONSTRAINT canvas_projects_created_by_user_id_fkey FOREIGN KEY (created_by_user_id) REFERENCES public.users(id) ON DELETE SET NULL;

ALTER TABLE ONLY public.canvas_projects
    ADD CONSTRAINT canvas_projects_org_id_fkey FOREIGN KEY (org_id) REFERENCES public.orgs(id) ON DELETE CASCADE;
