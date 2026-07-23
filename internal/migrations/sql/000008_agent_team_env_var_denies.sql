-- +goose Up

CREATE UNIQUE INDEX IF NOT EXISTS idx_team_env_vars_id_org_id
    ON public.team_env_vars (id, org_id);

CREATE TABLE public.agent_team_env_var_denies (
    org_id uuid NOT NULL,
    agent_id uuid NOT NULL,
    team_env_var_id uuid NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT agent_team_env_var_denies_pkey PRIMARY KEY (agent_id, team_env_var_id),
    CONSTRAINT agent_team_env_var_denies_agent_org_fkey
        FOREIGN KEY (agent_id, org_id)
        REFERENCES public.agents(id, org_id)
        ON DELETE CASCADE,
    CONSTRAINT agent_team_env_var_denies_env_org_fkey
        FOREIGN KEY (team_env_var_id, org_id)
        REFERENCES public.team_env_vars(id, org_id)
        ON DELETE CASCADE,
    CONSTRAINT agent_team_env_var_denies_org_fkey
        FOREIGN KEY (org_id)
        REFERENCES public.orgs(id)
        ON DELETE CASCADE
);

CREATE INDEX idx_agent_team_env_var_denies_org
    ON public.agent_team_env_var_denies (org_id);

CREATE INDEX idx_agent_team_env_var_denies_env
    ON public.agent_team_env_var_denies (team_env_var_id);

-- +goose Down

DROP TABLE IF EXISTS public.agent_team_env_var_denies;
DROP INDEX IF EXISTS public.idx_team_env_vars_id_org_id;
