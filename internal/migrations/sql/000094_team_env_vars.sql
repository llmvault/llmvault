-- +goose Up
DROP TABLE public.channel_env_vars;

CREATE TABLE public.team_env_vars (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    org_id uuid NOT NULL,
    team_id uuid NOT NULL,
    name text NOT NULL,
    encrypted_value bytea NOT NULL,
    description text DEFAULT ''::text NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL
);

ALTER TABLE ONLY public.team_env_vars
    ADD CONSTRAINT team_env_vars_pkey PRIMARY KEY (id);

CREATE INDEX idx_team_env_vars_team ON public.team_env_vars USING btree (team_id);

CREATE UNIQUE INDEX idx_team_env_vars_team_name ON public.team_env_vars USING btree (team_id, name);

CREATE INDEX idx_team_env_vars_org ON public.team_env_vars USING btree (org_id);

ALTER TABLE ONLY public.team_env_vars
    ADD CONSTRAINT team_env_vars_team_org_fkey
    FOREIGN KEY (team_id, org_id) REFERENCES public.teams(id, org_id) ON DELETE CASCADE;
