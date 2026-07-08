-- +goose Up
CREATE TABLE public.org_invite_teams (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    org_id uuid NOT NULL,
    org_invite_id uuid NOT NULL,
    team_id uuid NOT NULL,
    created_at timestamp with time zone
);

ALTER TABLE ONLY public.org_invite_teams
    ADD CONSTRAINT org_invite_teams_pkey PRIMARY KEY (id);

CREATE UNIQUE INDEX idx_org_invite_teams_invite_team ON public.org_invite_teams USING btree (org_invite_id, team_id);

CREATE INDEX idx_org_invite_teams_org_id ON public.org_invite_teams USING btree (org_id);

CREATE INDEX idx_org_invite_teams_team_id ON public.org_invite_teams USING btree (team_id);

ALTER TABLE ONLY public.org_invite_teams
    ADD CONSTRAINT fk_org_invite_teams_invite FOREIGN KEY (org_invite_id) REFERENCES public.org_invites(id) ON DELETE CASCADE;

ALTER TABLE ONLY public.org_invite_teams
    ADD CONSTRAINT fk_org_invite_teams_org FOREIGN KEY (org_id) REFERENCES public.orgs(id) ON DELETE CASCADE;

ALTER TABLE ONLY public.org_invite_teams
    ADD CONSTRAINT fk_org_invite_teams_team FOREIGN KEY (team_id) REFERENCES public.teams(id) ON DELETE CASCADE;

-- +goose Down
DROP TABLE IF EXISTS public.org_invite_teams CASCADE;
