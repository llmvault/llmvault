-- +goose Up
UPDATE public.agents
SET team_id = (
    SELECT id FROM public.teams t
    WHERE t.org_id = agents.org_id
    ORDER BY t.created_at ASC
    LIMIT 1
)
WHERE team_id IS NULL;

ALTER TABLE ONLY public.agents
    DROP CONSTRAINT agents_team_id_fkey;

ALTER TABLE ONLY public.agents
    ADD CONSTRAINT agents_team_id_fkey FOREIGN KEY (team_id) REFERENCES public.teams(id) ON DELETE RESTRICT;

ALTER TABLE public.agents
    ALTER COLUMN team_id SET NOT NULL;
