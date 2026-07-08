-- +goose Up
CREATE TABLE public.team_members (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    org_id uuid NOT NULL,
    team_id uuid NOT NULL,
    user_id uuid NOT NULL,
    role text DEFAULT 'member'::text NOT NULL,
    created_at timestamp with time zone,
    updated_at timestamp with time zone,
    deactivated_at timestamp with time zone
);

ALTER TABLE ONLY public.team_members
    ADD CONSTRAINT team_members_pkey PRIMARY KEY (id);

CREATE INDEX idx_team_members_org_user ON public.team_members USING btree (org_id, user_id);

CREATE INDEX idx_team_members_team_id ON public.team_members USING btree (team_id);

CREATE UNIQUE INDEX idx_team_members_team_user ON public.team_members USING btree (team_id, user_id);

ALTER TABLE ONLY public.team_members
    ADD CONSTRAINT fk_team_members_org FOREIGN KEY (org_id) REFERENCES public.orgs(id) ON DELETE CASCADE;

ALTER TABLE ONLY public.team_members
    ADD CONSTRAINT fk_team_members_team FOREIGN KEY (team_id) REFERENCES public.teams(id) ON DELETE CASCADE;

ALTER TABLE ONLY public.team_members
    ADD CONSTRAINT fk_team_members_user FOREIGN KEY (user_id) REFERENCES public.users(id) ON DELETE CASCADE;
