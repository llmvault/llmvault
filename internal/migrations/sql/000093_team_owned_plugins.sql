-- +goose Up
-- Custom plugins are team-owned. There is no production compatibility burden
-- for the old org-owned shape, so remove any pre-release rows that cannot be
-- assigned to a team unambiguously.
DELETE FROM public.plugins WHERE org_id IS NOT NULL;

ALTER TABLE public.plugins
    ADD COLUMN team_id uuid;

DROP INDEX public.idx_plugins_org_slug;

CREATE INDEX idx_plugins_team_id ON public.plugins USING btree (team_id);

CREATE UNIQUE INDEX idx_plugins_team_slug
    ON public.plugins USING btree (team_id, slug)
    WHERE (team_id IS NOT NULL);

CREATE UNIQUE INDEX idx_teams_id_org_id
    ON public.teams USING btree (id, org_id);

ALTER TABLE ONLY public.plugins
    ADD CONSTRAINT plugins_team_org_fkey
    FOREIGN KEY (team_id, org_id) REFERENCES public.teams(id, org_id)
    ON DELETE CASCADE;

ALTER TABLE ONLY public.plugins
    ADD CONSTRAINT plugins_scope_check CHECK (
        (org_id IS NULL AND team_id IS NULL)
        OR
        (org_id IS NOT NULL AND team_id IS NOT NULL)
    );
