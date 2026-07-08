-- +goose Up
CREATE TABLE public.teams (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    org_id uuid NOT NULL,
    name text NOT NULL,
    description text DEFAULT ''::text NOT NULL,
    created_by uuid,
    archived_at timestamp with time zone,
    created_at timestamp with time zone,
    updated_at timestamp with time zone
);

ALTER TABLE ONLY public.teams
    ADD CONSTRAINT teams_pkey PRIMARY KEY (id);

CREATE INDEX idx_teams_archived_at ON public.teams USING btree (archived_at);

CREATE INDEX idx_teams_org_id ON public.teams USING btree (org_id);

CREATE UNIQUE INDEX idx_teams_org_name_active ON public.teams USING btree (org_id, name) WHERE (archived_at IS NULL);

ALTER TABLE ONLY public.teams
    ADD CONSTRAINT fk_teams_creator FOREIGN KEY (created_by) REFERENCES public.users(id) ON DELETE SET NULL;

ALTER TABLE ONLY public.teams
    ADD CONSTRAINT fk_teams_org FOREIGN KEY (org_id) REFERENCES public.orgs(id) ON DELETE CASCADE;

-- +goose Down
DROP TABLE IF EXISTS public.teams CASCADE;
